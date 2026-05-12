# Elm MinIO container image

## Overview

This project puts together the infrastructure for an automated build of a modified MinIO server.
In some cases the MinIO developers did not leave any way to override a default, and we found ourselves needing to change the source code.
Currently there are two examples of this:

1. Originally we found the OpenID Connect (OIDC) code hard-coded supported
   policies and did not provide any way to include custom claims.  Because
   Stanford's use of `eduperson_entitlement` was not one of the standard claims
   MinIO referenced, it wasn't available for use.

2. The tape system backing Elm is sensitive to needing to deal with many small
   files (each file consumes an inode and requires additional metadata to track
   it).  While we have quotas on the number of objects (including versions) we
   don't have any way to enforce the size of the parts that make up multipart
   objects, meaning a user could create an object with many thousands of small
   parts, which would be detrimental to us.

This tool takes advantage of the [AST module](https://pkg.go.dev/go/ast)
provided by the [go programming language](https://go.dev/) to manage code to
make point updates to source trees, effectively rewriting the source code where
needed.

This project leverages the elm-patch tool alongside Docker and git to offer an
automated workflow to:

1. Download a specified RELEASE of MinIO source code
2. Patch variables or otherwise modify the source code
3. Build a new MinIO binary
4. Build a stanford-rc/elm-minio docker image

We no longer need to worry about the OIDC issues described in issue (1) above,
we discarded the idea of using MinIO to manage the login procedure.

We are using elm-patch to address issue (2), changing the minimum part size for
multipart objects.

You may wonder why we aren't just using a diff file and patch tool to apply
changes to the upstream source.  The answer is that the MinIO developers seem
to make fairly sweeping changes to their codebase, and we thought it'd be
easier to maintain this higher level tooling to capture the intent of changes,
rather than relying on tracking line numbering and context to apply changes.

## Components

The automated build is managed through several components:

- *policy.patch* provides a tool to modify the Go source code in the minio/pkg project
- *minio.build.sh* provides a tool to download MinIO, patch the source code, and build a new MinIO server binary.
- *Dockerfile* uses a go container image to run minio.build.sh and then build a modified MinIO server container image.
- *Makefile* calls `docker build` with a https://github.com/stanford-rc/minio/tags RELEASE tag (version identifier).

### elm-patch

This directory contains the source code for a small self-contained Go program that can modify the MinIO source code.  Currently there are two patches (one unused):

1. `patch_jwt_claims.go` which modifies the gihhub.com/minio/pkg/ source tree
   to extend the list of supported JWT claims (this patch is not currently in
   use)

2. `patch_minio_globals.go` which modifies the github.com/stanford-rc/minio source
   tree to change some of the default global variables.

The `patch.go` file lists the filepaths we expect to modify:

```
// Patches maps unix filesystem paths to Patcher
var Patches = map[string]Patcher{
        `minio/cmd/utils.go$`: PatchMinioGlobals(),
        `pkg/policy/condition/keyname.go$`: PatchPkgJWTClaims(),
}
```

This is a regular expression match for a relative UNIX filesystem path, and
they map to instances of the Patcher interface defined in the same file:

```
// Patcher interface implemented by individual implementations to modify the
// minio source code
type Patcher interface {
        Patch(string) (*bytes.Buffer, error)
}
```

Each of the `patch_*.go` files mentioned earlier implement `Patcher` by
accepting a filepath and returning the updated source code:

```
$ ls
ast.go  go.mod  go.sum  main.go  patch.go  patch_jwt_claims.go  patch_minio_globals.go

$ go build

$ ls -l elm-patch
-rwxr-xr-x. 1 jimr jimr 3786751 Feb 21 09:56 elm-patch


$ ls ../src
README.txt

$ git clone https://github.com/stanford-rc/minio.git ../src/minio
Cloning into '../src/minio'...

$ ./elm-patch -update -backup ../src/minio/cmd/utils.go

$ ls -l ../src/minio/cmd/utils.go*
-rw-------. 1 jimr jimr 33447 Feb 21 09:57 ../src/minio/cmd/utils.go
-rw-r--r--. 1 jimr jimr 33445 Feb 21 09:57 ../src/minio/cmd/utils.go~1

$ diff -u ../src/minio/cmd/utils.go{~1,}
--- ../src/minio/cmd/utils.go~1 2025-02-21 09:57:05.462027983 -0600
+++ ../src/minio/cmd/utils.go   2025-02-21 09:57:05.463027998 -0600
@@ -288,8 +288,8 @@
        // using 'curl' and presigned URL.
        globalMaxObjectSize = 5 * humanize.TiByte


-       // Minimum Part size for multipart upload is 5MiB
-       globalMinPartSize = 5 * humanize.MiByte
+       // Minimum Part size for multipart upload is 5GiB",
+       globalMinPartSize = 5 * humanize.GiByte

        // Maximum Part ID for multipart upload is 10000
        // (Acceptable values range from 1 to 10000 inclusive)
```

### minio.build.sh

This script builds MinIO from within a Docker container.  If the ./src
directory contains checked out versions of github.com/stanford-rc/minio and/or
github.com/stanford-rc/minio-pkg then they will be used, otherwise it will use
git to fetch them from github.com.  In order to fetch from github.com you need
to have an ssh key registered with github.com and you need to have set up
ssh-agent with that key registered in it (via ssh-add).

The purpose of allowing ./src to be empty of source trees or not is to aid in
development work.  The intent is that under normal operation we check out the
specific release of MinIO we want to patch as part of an automated build and
then discard it once we've got our custom binary built.

This script then runs elm-patch against any paths defined in the script's
`patch_file` array:

```
# relative source file paths to run elm-patch against
patch_files=(
        "minio/cmd/utils.go"

        # we are no longer using minio's built-in ODIC functionality, so we no
        # longer need to patch the list of supported claims
        #"pkg/policy/condition/keyname.go"
)
```

### Dockerfile

Docker build file for running minio.build.sh.

The Dockerfile assumes that the caller has ssh credentials to access any
restricted git URLs, and that the build was called with the `--ssh default`
arguments (see the Makefile).

The `./src`, `./elm-patch`, and `./minio.build.sh` script are copied into the
build container and then `./minio.build.sh` is run w/ ssh credentials enabled.

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
# the most recent listed RELEASE.
RELEASES=\
	STANFORD.2026-05-12T20-37-47Z \
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

Don't attempt to downgrade a RELEASE of MinIO without understanding the potential impact.

## License

This repository contains build tooling for a modified MinIO binary. The build
tooling and all modifications to MinIO source code are licensed under the
[GNU Affero General Public License v3 (AGPLv3)](LICENSE).

MinIO is copyright MinIO, Inc. and its contributors, and is also licensed under
AGPLv3. This project is not affiliated with or endorsed by MinIO, Inc.
