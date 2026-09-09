#!/usr/bin/env bash
# Usage: bash run-mac.sh /absolute/path/to/repository FULL_COMMIT [linux/arm64|linux/amd64]
# Creates only a new sibling run directory. Source is read through git archive.
set -euo pipefail
if [ "$#" -lt 2 ] || [ "$#" -gt 3 ]; then
  echo 'Usage: bash run-mac.sh REPOSITORY FULL_COMMIT [linux/arm64|linux/amd64]' >&2
  exit 2
fi
kit_dir="$(cd "$(dirname "$0")" && pwd -P)"
repo_dir="$(cd "$1" && pwd -P)"
commit="$2"
[[ "$commit" =~ ^[0-9a-f]{40}$ ]] || { echo 'A complete 40-character commit is required.' >&2; exit 2; }
actual="$(git -C "$repo_dir" rev-parse --verify HEAD)"
[ "$actual" = "$commit" ] || { echo "HEAD is $actual; expected $commit" >&2; exit 2; }
[ -z "$(git -C "$repo_dir" status --porcelain)" ] || { echo 'Use a clean dedicated checkout.' >&2; exit 2; }
case "$(uname -m)" in
  arm64|aarch64) platform="${3:-linux/arm64}" ;;
  x86_64) platform="${3:-linux/amd64}" ;;
  *) echo 'Unsupported host architecture.' >&2; exit 2 ;;
esac
case "$platform" in linux/arm64|linux/amd64) ;; *) exit 2 ;; esac
docker info >/dev/null
run_dir="$(mktemp -d "$(dirname "$repo_dir")/topic3-reproduction.XXXXXX")"
mkdir "$run_dir/context" "$run_dir/evidence"
printf 'Evidence directory: %s\n' "$run_dir"
{
  sw_vers
  uname -m
  git --version
  docker version
  docker info --format '{{json .}}'
} > "$run_dir/host-versions.txt"
git -C "$repo_dir" archive --format=tar --output="$run_dir/context/source.tar" "$commit"
tar_sha="$(shasum -a 256 "$run_dir/context/source.tar" | awk '{print $1}')"
tree="$(git -C "$repo_dir" rev-parse "$commit^{tree}")"
printf '{"commit":"%s","tree":"%s","archive_sha256":"%s","container_platform":"%s"}\n' \
  "$commit" "$tree" "$tar_sha" "$platform" > "$run_dir/context/source-identity.json"
cp "$kit_dir/Dockerfile" "$kit_dir/in-container.sh" "$kit_dir/summarize.py" "$run_dir/context/"
tag="topic3-reproduction:${commit:0:12}-${platform##*/}"
docker build --platform "$platform" --progress plain -t "$tag" "$run_dir/context" 2>&1 | tee "$run_dir/build.log"
image_id="$(docker image inspect "$tag" --format '{{.Id}}')"
docker image inspect "$image_id" > "$run_dir/image-inspect.json"
container_name="topic3-$(basename "$run_dir" | tr '[:upper:].' '[:lower:]-')"
printf '%s\n' "$container_name" > "$run_dir/container-name.txt"
# No outbound network during evaluation. The service and fixture share loopback.
# Keep the stopped container and all outputs so a failed run remains inspectable.
set +e
docker run --name "$container_name" --network none --platform "$platform" \
  --mount "type=bind,source=$run_dir/evidence,target=/evidence" "$image_id" \
  2>&1 | tee "$run_dir/run.log"
run_exit=${PIPESTATUS[0]}
set -e
printf '%s\n' "$run_exit" > "$run_dir/exit-code.txt"
printf 'Exit code: %s\nEvidence: %s\n' "$run_exit" "$run_dir"
exit "$run_exit"
