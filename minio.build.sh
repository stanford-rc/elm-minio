#!/bin/bash
#
# minio.build.sh optionally downloads github.com/stanford-rc/minio and and
# builds a new minio binary after applying patches defined in the elm-patch
# tool.

# name of this script
prog=$(basename "${0}");

# working directory we are operating out of
workdir="$(cd "$(dirname "${0}")"; pwd)";

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

	# jq is required
	if ! command -v jq &> /dev/null; then
		echo "${0}: jq is required" 1>&2;
		return 1;
	fi
}

if ! preflight_check; then
	exit 1;
fi

# relative source file paths to run elm-patch against, note this is derived
# from the unvesioned paths in the elm-patch/patch.go variable Patches.
declare -a patch_files=(
	"minio/cmd/utils.go"

	# we are no longer using minio's built-in ODIC functionality, so we no
	# longer need to patch the list of supported claims
	#"minio-pkg/policy/condition/keyname.go"
)

# remote repo map for patch files
declare -A patch_repos=(
	["minio/cmd/utils.go"]="git@github.com:stanford-rc/minio"
	["minio-pkg/policy/condition/keyname.go"]="git@github.com:stanford-rc/minio-pkg"
)

# require minio tag to build e.g., RELEASE.2024-01-29T03-56-32Z
if [ "${#}" != "1" ]; then
	echo "usage: ${prog} <minio_release>" 1>&2;
	echo "e.g., ${prog} RELEASE.2024-01-29T03-56-32Z" 1>&2;
	exit 1;
else
	ELM_RELEASE=${1};
fi

# mod_deps returns the go.mod dependencies for a module
# it will produce lines of either
#
# <path> <version>
#
# or
#
# <old_path> <old_version> <new_path> <new_version>
#
# depending on whether or not a replacement was defined for a module.
mod_deps() {
	go -C "${1}" mod edit -json | jq -r '
  # 1. Create a lookup map of replacements: OldPath -> NewModuleObject
  (.Replace // [] | map({(.Old.Path): .New}) | add) as $replacements |

  # 2. Iterate through every required package
  .Require[] |

  # 3. Look up the current package in our replacement map
  $replacements[.Path] as $replacement |

  # emit dep_path dep_version [rep_path rep_version]
  "\(.Path) \(.Version)\(if $replacement then " \($replacement.Path) \($replacement.Version)" else "" end)"
  '
}

# exit the build script on any failures after this point
set -e

# iterate over patch files check out the source tree if it's not provided, keep
# track of trees we pulled
declare -A was_cloned;
for patch_file in "${patch_files[@]}"; do
	patch_repo="${patch_repos["${patch_file}"]}";

	patch_dir=$(basename "${patch_repo}");
	
	if [[ ! -d "src/${patch_dir}" ]]; then
		git clone "${patch_repo}" "src/${patch_dir}";
		was_cloned["${patch_dir}"]="true";
	fi
done

# look for module remappings that we may need to resolve while we're
# determining the tag minio depends on.  The mod_new_to_old associative array
# records the CURRENT NAME of a module and maps it to the PRIOR NAME.  We use
# this mapping to resolve the module version target when we have checked out a
# copy of the module from the repo instead of the user having provided it to
# us.
declare -A mod_new_to_old;
while IFS= read -r line; do
	read -r dep_path dep_version rep_path rep_version  <<<"${line}";
	if [[ -n "${rep_path}" ]]; then
		mod_new_to_old["${rep_path}"]="${dep_path}";
	fi
done < <( mod_deps "src/minio" );

# check out the minio revision we're building
git -C src/minio checkout --quiet "${ELM_RELEASE}";
 
# determine dependencies for this minio revision, override any replacements
# that might ahve been defined in the head
declare -A dep_versions;
while IFS= read -r line; do
	read -r dep_path dep_version rep_path rep_version  <<<"${line}";
	if [[ -n "${rep_path}" ]]; then
		dep_versions["${rep_path}"]="${rep_version}";
	else
		dep_versions["${dep_path}"]="${dep_version}";
	fi
done < <( mod_deps "src/minio" );

# iterate over non-minio trees and if they were cloned check out the required
# tag
for patch_file in "${patch_files[@]}"; do
	patch_repo="${patch_repos["${patch_file}"]}";

	patch_dir=$(basename "${patch_repo}");

	if [[ "${patch_dir}" == "minio" ]]; then
		continue
	fi
		
	if [[ "${was_cloned["${patch_dir}"]}" != "true" ]]; then
		continue
	fi

	read -r mod_path mod_version < <(go -C "src/${patch_dir}" mod edit -json | jq -r '(.Module.Path // "") + " " + (.Module.Version // "")');

	if [[ -n "${mod_new_to_old[${mod_path}]}" ]]; then
		old_path="${mod_new_to_old[${mod_path}]}";
		dep_version="${dep_versions[${old_path}]}";
	else
		dep_version="${dep_versions[${mod_path}]}";
	fi

	if [[ -z "${dep_version}" ]]; then
		echo "unable to determine desired module version for src/${patch_dir}" 1>&2;
		exit 1;
	fi
	
	git -C "src/${patch_dir}" checkout "${dep_version}";
done

# iterate over non-minio trees and update minio to point to it
for patch_file in "${patch_files[@]}"; do
	patch_repo="${patch_repos["${patch_file}"]}";

	patch_dir=$(basename "${patch_repo}");

	if [[ "${patch_dir}" == "minio" ]]; then
		continue
	fi
	
	read -r mod_path mod_version < <(go -C "src/${patch_dir}" mod edit -json | jq -r '(.Module.Path // "") + " " + (.Module.Version // "")');

	echo mod_path=${mod_path}
	echo mod_version=${mod_version}

	if [[ -z "${mod_version}" ]]; then
		go -C src/minio mod edit -replace "${mod_path}=$(pwd)/src/${patch_dir}";
	else
		go -C src/minio mod edit -replace "${mod_path}@${mod_version}=$(pwd)/src/${patch_dir}";
	fi
done

# build the elm-patch tool
go -C "${workdir}/elm-patch" build;
	
# switch to the top level src directory and apply patches
cd "${workdir}/src";
	
"${workdir}"/elm-patch/elm-patch -update -backup "${patch_files[@]}";
	
for patch_target in "${patch_files[@]}"; do
	diff -u "${patch_target}~1" "${patch_target}" || :;
done
	
# build the patched minio binary, we keep MINIO_RELEASE=RELEASE to be sure that
# any tooling that requires the original RELEASE.<timestamp> format won't be
# confused.
GOOS=$(go env GOOS);
GOPATH=$(go env GOPATH);
GOARCH=$(go env GOARCH);
RELEASE_TIMESTAMP=$(echo "${ELM_RELEASE}" | sed 's,RELEASE.,,;s,STANFORD.,,;s,T\([0-9]*\)-\([0-9]*\)-\([0-9]*\)Z,T\1:\2:\3Z,');
LDFLAGS=$(set -e; cd minio/buildscripts/ && env MINIO_RELEASE=RELEASE go run gen-ldflags.go "${RELEASE_TIMESTAMP}");
cd minio && CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} go build -tags kqueue -trimpath --ldflags "${LDFLAGS}" -o minio
