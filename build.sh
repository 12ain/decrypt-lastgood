#!/usr/bin/env bash
# Cross-compiles static binaries and packages them as zip archives.
# Requires Go 1.22+ and Info-ZIP. No cgo, no third-party modules.
set -euo pipefail

cd "$(dirname "$0")"

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
VERSION="${VERSION#v}"

LDFLAGS="-s -w"
STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT

rm -rf dist
mkdir -p dist
DIST="$(pwd)/dist"

build() {
	local goos=$1 goarch=$2 ext=$3
	local name="dlg_${VERSION}_${goos}_${goarch}"
	local dir="$STAGE/$name"

	printf '  %-16s -> %s.zip\n' "$goos/$goarch" "$name"
	mkdir -p "$dir"

	GOOS=$goos GOARCH=$goarch CGO_ENABLED=0 \
		go build -trimpath -buildvcs=false -ldflags "$LDFLAGS" -o "$dir/dlg$ext" ./cmd/dlg
	cp README.md LICENSE "$dir/"

	(cd "$STAGE" && zip -qr "$DIST/$name.zip" "$name")
}

echo "version: $VERSION"
echo "building:"
build windows amd64 .exe
build windows arm64 .exe
build darwin  arm64 ""
build darwin  amd64 ""
build linux   amd64 ""

if command -v sha256sum >/dev/null 2>&1; then
	SUM=(sha256sum)
elif command -v shasum >/dev/null 2>&1; then
	SUM=(shasum -a 256)
else
	SUM=()
fi

if [ ${#SUM[@]} -gt 0 ]; then
	(cd dist && "${SUM[@]}" ./*.zip >SHA256SUMS)
fi

echo
echo "output:"
ls -lh dist
