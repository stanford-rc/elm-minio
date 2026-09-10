# Elm MinIO container image

## Overview

This project builds a container image for the MinIO server that Elm runs, from
the Stanford RC fork at https://github.com/stanford-rc/minio.

A build is a checkout and a compile. There is no patch step.

## Why there is no patch step

Until 2026-09-08 this repository carried `elm-patch`, a small Go program that
used the [AST module](https://pkg.go.dev/go/ast) to rewrite MinIO's source at
build time. It existed for one reason, stated in its own documentation: MinIO
upstream made sweeping changes to their codebase, so capturing the *intent* of a
change was more durable than a diff with line numbers and context.

That reason no longer holds, and the mechanism carried a risk that outweighed it.

**There is no upstream any more.** MinIO withdrew from open source and the
`github.com/stanford-rc/minio` fork has no upstream remote. Every change in the
fork now originates with us, so there is no churn for an AST layer to absorb.

**The rewrite could fail silently.** `elm-patch` matched on identifier name and
exited non-zero only when a *file path* matched no pattern. It never checked that
a patch changed anything. A rename or a move of `globalMinPartSize` would have
left the file untouched, and `minio.build.sh` printed

```
diff -u "${patch_target}~1" "${patch_target}" || :
```

for a human to read while discarding the exit status, so an empty diff, meaning
the patch matched nothing, passed. The result would have been a green build
shipping the upstream 5 MiB minimum part size, which is exactly the
many-small-parts tape problem this repository was created to prevent.

**Nothing could test it.** The value production ran did not exist in the fork, so
no test there could pin it. Documentation drift followed, as it does.

So the divergences moved into the fork as ordinary source code, where `git log`,
`git blame`, `gofmt`, `go vet` and `go test` all see them.

## Current divergences from upstream MinIO

Both live in https://github.com/stanford-rc/minio, not here.

| divergence | where | pinned by |
|---|---|---|
| `globalMinPartSize` 5 MiB to **5 GiB**, `MINIO_MIN_PART_SIZE` to override | `cmd/utils.go` | `TestMinPartSizeIsElmFiveGiB`, `TestMinAllowedPartSizeUsesTheElmFloor` |
| multipart write-set enforcement | `cmd/erasure-multipart.go`, `cmd/erasure-object.go` | `cmd/erasure-multipart-*_test.go` |

The minimum part size is raised because Elm's disk tier is archived to tape,
where every part is a separate file consuming an inode and its own metadata. An
object assembled from thousands of small parts is expensive to archive and
expensive to recall. Object and version counts are already quota'd; part size was
the remaining unbounded dimension.

The OIDC/JWT claims patch that `elm-patch` also carried was already unused, since
we no longer use MinIO to manage the login procedure. It was removed with the
rest. If a change to `minio-pkg` is ever needed again it belongs in the
`stanford-rc/minio-pkg` fork, on the same reasoning as above.

### Environment variables

Three variables exist only on this fork. All three are read from the process
environment and are deliberately NOT registered in MinIO's config subsystem, so
they do not appear in `mc admin config`. That is the intent rather than an
oversight: config keys are cluster-wide persisted policy, and two of these are
per-process levers meant to be set on one node and restarted to get back to known
behaviour without a rebuild.

| variable | default | effect | read in |
|---|---|---|---|
| `MINIO_MIN_PART_SIZE` | `5GiB` | Overrides the multipart minimum part size. Accepts any humanized size at or above the 5 MiB S3 minimum and below the maximum object size. An unparseable or out-of-range value is fatal at startup rather than silently ignored. | `cmd/utils.go`, resolved once in `serverHandleEnvVars` |
| `MINIO_MULTIPART_WRITESET` | `on` | `off` restores the upstream write path, skipping write-set enforcement and the commit-set collapse check. A rollback lever, not a policy choice. An unrecoverable part is refused whichever way this is set, because returning 200 for a part that cannot be reconstructed is wrong under any policy. | `cmd/erasure-multipart.go`, read per part and per commit |
| `MINIO_DANGLING_DELETE` | `off` | `off` audits a dangling verdict and declines to act. `on` restores upstream behaviour, where `deleteIfDangling` removes the whole object version. Upstream has no such switch and always deletes. Any value other than `on` or `off` is treated as `off`. | `cmd/erasure-object.go` |

The default for `MINIO_DANGLING_DELETE` is the one worth understanding before
changing it. One part below read quorum in a 203-part object makes the whole
version dangling, and acting on that verdict removes all 203 including the
roughly 200 intact ones. An object in that state needs a person rather than a
heal, so the default is to record the verdict and stop.

## Tags before 2026-09-08 are not buildable for production

This is the accepted cost of the move. `elm-patch` applied the part-size change
to *any* tag, including the 2024 releases listed in the Makefile. Those tags
contain the upstream 5 MiB value in their source, so building one now produces a
binary that is wrong for Elm.

`minio.build.sh` checks for each divergence after checkout and **fails the build**
if one is missing, naming it. Old tags remain buildable for forensics by removing
that check deliberately, which is the point: it is a decision rather than an
accident. Note also that MinIO does not support downgrading, so rebuilding an old
release for production was already discouraged.

## Components

### minio.build.sh

Builds the MinIO binary, normally from within the Docker container.

If `./src/minio` already contains a checked-out copy of
github.com/stanford-rc/minio it is used as-is, otherwise the script clones it.
Cloning needs an ssh key registered with github.com and an ssh-agent holding it,
which is what the Makefile's `--ssh default` and `check-ssh-auth-sock.sh`
arrange. Allowing `./src` to be pre-populated exists to aid development; under
normal operation the tag is checked out as part of an automated build and
discarded once the binary exists.

After checkout the script RUNS THE TESTS that assert each Elm divergence and
fails the build if any of them does not report PASS. A `-run` pattern matching
nothing exits 0, so each test must be seen to have passed; absence means the test
is not in the tree, which means the divergence is not either.

That is the third mechanism for this. The first, elm-patch, rewrote the source and
matched on identifier name. The second grepped the source for the same identifier,
which is the same coupling, and it broke twice on 2026-09-08: once when the value
changed from a const to a var, once on a rename. Both times a tree that carried
the divergence was reported as missing it. A test is rename-proof and also catches
a divergence that is present but wired to the wrong value.

### Dockerfile

Docker build file for running minio.build.sh.

The Dockerfile assumes the caller has ssh credentials for any restricted git
URLs, and that the build was invoked with `--ssh default`; see the Makefile.

`./src` and `./minio.build.sh` are copied into the build container and then
`./minio.build.sh` is run with ssh credentials enabled.

### xlmeta

A reference `xl.meta` decoder library, ported from the `xl-meta` command line
utility. Independent of the build tooling and unaffected by any of the above.

## Makefile

The Makefile is used to kick off the build process.

Ideally you can simply call `make` to build the latest release, or `make
<release>` where `<release>` is a tag from github.com/stanford-rc/minio/tags that has
been added to the `RELEASES` declaration in the Makefile.  If you have bash
command line completion for make enabled you should be able to type

```
make<tab><tab>
```

to get a listing of available tags, e.g.,

```
$ make RELEASE.2024-0<tab><tab>
RELEASE.2024-04-06T05-26-02Z  RELEASE.2024-06-13T22-53-53Z  RELEASE.2024-08-26T15-33-07Z
```

If we look at the contents of the Makefile at the time this document was
written you can see it has a `RELEASES` variable that contains a list of
identifiers pulled from https://github.com/stanford-rc/minio/tags:

The list is kept in descending order so that the default target will be to
build the latest release.

```
# RELEASES holds a list of minio releases we want to be able to build.  The
# identifiers come from https://github.com/stanford-rc/minio/tags and we can
# add new versions as we need them.  Note that MinIO does NOT OFFICIALLY
# SUPPORT DOWNGRADING MinIO.  That means that if you start a newer instance of
# MinIO it may modify existing files and create new files that are binary
# incompatible with older releases.  It's critically important that any
# downgrading process be aware of the changes between the versions, as there is
# no guarantee that incompatible changes won't be introduced to the files MinIO
# manages.
#
# NB: The order of RELEASES is newest to oldest, so that the default target is
# the most recent listed release.
RELEASES=\
	STANFORD.2026-05-12T23-03-22Z \
	RELEASE.2024-08-26T15-33-07Z \
	RELEASE.2024-06-13T22-53-53Z \
	RELEASE.2024-04-06T05-26-02Z \

# Given a github.com/stanford-rc/minio repo tag, call docker build with the arg
# ELM_RELEASE=<release> set, and tagging the resulting image as
# ghcr.io/stanford-rc/elm-minio:<release>.
$(RELEASES):
	./check-ssh-auth-sock.sh && \
	export EPOCH=$$(date --date $$(echo $@ | sed 's,RELEASE.,,;s,STANFORD.,,;s,\(.*T\)\(..\)-\(..\)-\(..\),\1\2:\3:\4,') +%s) && \
	docker \
		build . \
		--debug \
		--progress plain \
		--ssh default \
		--build-arg ELM_RELEASE="$@" \
		--build-arg SOURCE_DATE_EPOCH="$$EPOCH" \
		--tag "ghcr.io/stanford-rc/elm-minio:$@";
```

## Potential Problems 

Don't attempt to downgrade a release of MinIO without understanding the potential impact.

## License

This repository contains build tooling for a modified MinIO binary. The build
tooling and all modifications to MinIO source code are licensed under the
[GNU Affero General Public License v3 (AGPLv3)](LICENSE).

MinIO is copyright MinIO, Inc. and its contributors, and is also licensed under
AGPLv3. This project is not affiliated with or endorsed by MinIO, Inc.
