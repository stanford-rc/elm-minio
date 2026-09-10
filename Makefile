
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
#
# 2026-09-08: the Elm divergences moved out of elm-patch and into the fork
# itself, so a tag only carries them if they were committed before it was cut.
# Every tag listed below predates that move and therefore contains the UPSTREAM
# 5 MiB minimum part size. minio.build.sh verifies each divergence after
# checkout and FAILS the build when one is missing, so these targets now fail by
# design rather than quietly producing a binary that is wrong for Elm. The check
# runs the divergence tests, which those tags do not contain.
#
# They are kept because they remain useful for forensics, where you want the
# source as it was. To build one, drop the verify_divergences call, and know why
# you are doing it. Note also that MinIO does not support downgrading, so
# rebuilding an old release for production was already discouraged.
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
