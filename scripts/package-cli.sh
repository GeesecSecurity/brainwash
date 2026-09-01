#!/bin/bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=version.sh
source "$ROOT/scripts/version.sh"
VERSION="$(brainwash_version --release)"
LDFLAGS="$(brainwash_ldflags "$VERSION")"
DIST="$ROOT/dist"
STAGE="$DIST/cli-stage"

cd "$ROOT"
mkdir -p "$DIST"
rm -rf "$STAGE"
mkdir -p "$STAGE"

echo "==> brainwash-cli $VERSION"
echo "==> ldflags $LDFLAGS"

build_one() {
  local goos="$1" goarch="$2" label="$3"
  local name="brainwash-cli"
  if [[ "$goos" == windows ]]; then
    name="brainwash-cli.exe"
  fi
  local tmp="$STAGE/${label}"
  mkdir -p "$tmp"
  echo "==> $label"
  GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 go build -trimpath -ldflags="$LDFLAGS" -o "$tmp/$name" ./cmd/brainwash-cli
  local archive="$DIST/brainwash-cli-${VERSION}-${label}.tar.gz"
  tar -C "$tmp" -czf "$archive" "$name"
  echo "$archive"
}

build_one darwin arm64 macos-arm64
build_one darwin amd64 macos-x64
build_one linux arm64 linux-arm64
build_one linux amd64 linux-x64
build_one windows arm64 windows-arm64
build_one windows amd64 windows-x64

rm -rf "$STAGE"
( cd "$DIST" && shasum -a 256 \
  "brainwash-cli-${VERSION}-macos-arm64.tar.gz" \
  "brainwash-cli-${VERSION}-macos-x64.tar.gz" \
  "brainwash-cli-${VERSION}-linux-arm64.tar.gz" \
  "brainwash-cli-${VERSION}-linux-x64.tar.gz" \
  "brainwash-cli-${VERSION}-windows-arm64.tar.gz" \
  "brainwash-cli-${VERSION}-windows-x64.tar.gz" \
  > "brainwash-cli-${VERSION}-SHA256SUMS.txt" )
echo "==> cli archives ready"
ls -lh "$DIST"/brainwash-cli-"${VERSION}"-*.tar.gz "$DIST"/brainwash-cli-"${VERSION}"-SHA256SUMS.txt
