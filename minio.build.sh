#!/bin/bash
#
# minio.build.sh optionally downloads github.com/stanford-rc/minio and builds a
# minio binary from a given tag.
#
# 2026-09-08: this script no longer patches anything. Every Elm divergence
# now lives in the github.com/stanford-rc/minio fork as ordinary source, so a
# build is a checkout and a compile. See README.md, "Why there is no patch step".

# name of this script
prog=$(basename "${0}");

# working directory we are operating out of
workdir="$(cd "$(dirname "${0}")"; pwd)";

# the fork we build, and where it is checked out to
minio_repo="git@github.com:stanford-rc/minio";
minio_dir="${workdir}/src/minio";

# preflight_check verifies the required commands are available
preflight_check() {
	# go is required
	if ! command -v go &> /dev/null; then
		echo "${0}: go is required" 1>&2;
		return 1;
	fi

	# git is required
	if ! command -v git &> /dev/null; then
		echo "${0}: git is required" 1>&2;
		return 1;
	fi
}

if ! preflight_check; then
	exit 1;
fi

# require minio tag to build e.g., STANFORD.2026-05-12T23-03-22Z
if [ "${#}" != "1" ]; then
	echo "usage: ${prog} <minio_release>" 1>&2;
	echo "e.g., ${prog} STANFORD.2026-05-12T23-03-22Z" 1>&2;
	exit 1;
else
	ELM_RELEASE=${1};
fi

# exit the build script on any failures after this point
set -e

# clone the fork unless a tree was provided for us, per src/README.txt
if [[ ! -d "${minio_dir}" ]]; then
	git clone "${minio_repo}" "${minio_dir}";
fi

# check out the revision we're building
git -C "${minio_dir}" checkout --quiet "${ELM_RELEASE}";

# Verify the Elm divergences are present in the tree we are about to build, by
# RUNNING THE TESTS THAT ASSERT THEM rather than grepping for their source.
#
# This replaces two earlier mechanisms, both of which failed silently.
#
# elm-patch rewrote cmd/utils.go at build time and matched on identifier name,
# reporting nothing when it matched nothing, so a rename would have shipped an
# unmodified 5 MiB binary from a green build.
#
# Its replacement grepped the source for the identifier instead, which is the
# same coupling with the same failure mode, and it broke TWICE on 2026-09-08
# alone: once when the constant became a var, and once on a rename from
# stanfordDefaultMinPartSize to elmDefaultMinPartSize. Both times the build
# reported the divergence ABSENT from a tree that carried it.
#
# A test is the real assertion. It is rename-proof, it survives the value moving
# between a const and a var, and it also catches a divergence that is present but
# wired to the wrong thing, which no grep can see.
#
# Tags predating 2026-09-08 carry the 5 MiB upstream minimum, because the value
# used to be applied at build time, and they do not contain these tests. Those
# tags are kept for forensics and are NOT buildable for production; this check is
# what tells you so, by name, instead of letting a wrong binary out.
verify_divergences() {
	local pkg="./cmd/"
	local tests=(
		TestMinPartSizeIsElmFiveGiB
		TestMinAllowedPartSizeUsesTheElmFloor
	)

	local pattern; pattern="$(IFS='|'; echo "${tests[*]}")";

	echo "${prog}: verifying Elm divergences by running: ${tests[*]}";

	local out;
	if ! out="$(cd "${minio_dir}" && go test "${pkg}" -count=1 -v -run "^(${pattern})\$" 2>&1)"; then
		echo "${out}" | tail -30 1>&2;
		echo "${prog}: divergence tests FAILED in ${ELM_RELEASE}" 1>&2;
		return 1;
	fi

	# A -run pattern that matches nothing exits 0. So require each test to have
	# actually reported PASS; absence means the test is not in this tree, which
	# means the divergence is not either.
	local t missing=0;
	for t in "${tests[@]}"; do
		if echo "${out}" | grep -qE -- "^--- PASS: ${t}"; then
			echo "${prog}:   ${t}: PASS";
		else
			echo "${prog}:   ${t}: NOT PRESENT" 1>&2;
			missing=1;
		fi
	done

	if (( missing )); then
		echo "${prog}: ${ELM_RELEASE} does not carry the Elm divergence tests." 1>&2;
		echo "${prog}: Tags before 2026-09-08 applied the minimum part size at build" 1>&2;
		echo "${prog}: time and do not contain it. Build a STANFORD tag from" 1>&2;
		echo "${prog}: 2026-09-08 or later." 1>&2;
		return 1;
	fi
}

verify_divergences;

# build the minio binary, we keep MINIO_RELEASE=RELEASE to be sure that any
# tooling that requires the original RELEASE.<timestamp> format won't be
# confused.
GOOS=$(go env GOOS);
GOPATH=$(go env GOPATH);
GOARCH=$(go env GOARCH);
RELEASE_TIMESTAMP=$(echo "${ELM_RELEASE}" | sed 's,RELEASE.,,;s,STANFORD.,,;s,T\([0-9]*\)-\([0-9]*\)-\([0-9]*\)Z,T\1:\2:\3Z,');
LDFLAGS=$(set -e; cd "${minio_dir}/buildscripts/" && env MINIO_RELEASE=RELEASE go run gen-ldflags.go "${RELEASE_TIMESTAMP}");
cd "${minio_dir}" && CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} go build -tags kqueue -trimpath --ldflags "${LDFLAGS}" -o minio
