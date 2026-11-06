#!/usr/bin/env bash
set -euo pipefail

version="${1:?Usage: build-release.sh TAG OS ARCH [OUTPUT_DIR]}"
target_os="${2:?Missing OS}"
target_arch="${3:?Missing architecture}"
output_dir="${4:-dist}"

if [[ ! "$version" =~ ^[a-zA-Z0-9][a-zA-Z0-9._-]*$ ]]; then
    echo "Tag must contain only letters, numbers, dots, underscores and hyphens." >&2
    exit 1
fi

case "$target_os/$target_arch" in
    windows/amd64|darwin/amd64|darwin/arm64|linux/amd64|linux/arm64) ;;
    *) echo "Unsupported target: $target_os/$target_arch" >&2; exit 1 ;;
esac

service_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
mkdir -p "$output_dir"
output_dir="$(cd "$output_dir" && pwd)"

package_name="lumistem-compiler-${version}-${target_os}-${target_arch}"
package_dir="$(mktemp -d)"
trap 'rm -rf "$package_dir"' EXIT

binary_name="lumistem-compiler"
if [[ "$target_os" == windows ]]; then
    binary_name+=".exe"
fi

cd "$service_dir"
CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" go build \
    -trimpath -buildvcs=false -ldflags="-s -w" \
    -o "$package_dir/$binary_name" ./cmd/server

if [[ "$target_os" == windows ]]; then
    (cd "$package_dir" && zip -q "$output_dir/$package_name.zip" "$binary_name")
else
    tar -czf "$output_dir/$package_name.tar.gz" -C "$package_dir" "$binary_name"
fi
