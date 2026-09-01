#!/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
APP="$ROOT/dist/brainwash.app"
MACOS="$APP/Contents/MacOS"
RES="$APP/Contents/Resources"

cd "$ROOT"
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -o "$ROOT/dist/brainwash-cli" ./cmd/brainwash-cli
( cd "$ROOT/gui" && swift build -c release --arch arm64 --product BrainwashGUI )

bash "$ROOT/scripts/make-appicon.sh"

rm -rf "$APP"
mkdir -p "$MACOS" "$RES"
GUI_BIN="$ROOT/gui/.build/arm64-apple-macosx/release/BrainwashGUI"
if [[ ! -x "$GUI_BIN" ]]; then
  echo "arm64 BrainwashGUI not found at $GUI_BIN" >&2
  exit 1
fi
cp "$GUI_BIN" "$MACOS/BrainwashGUI"
cp "$ROOT/dist/brainwash-cli" "$MACOS/brainwash-cli"
for bin in "$MACOS/BrainwashGUI" "$MACOS/brainwash-cli"; do
  archs=$(lipo -archs "$bin")
  if [[ "$archs" != *arm64* || "$archs" == *x86_64* ]]; then
    echo "expected arm64-only in $bin, got: $archs" >&2
    exit 1
  fi
done
cp "$ROOT/gui/Info.plist" "$APP/Contents/Info.plist"
cp "$ROOT/gui/AppIcon.icns" "$RES/AppIcon.icns"
printf 'APPL????' > "$APP/Contents/PkgInfo"
chmod +x "$MACOS/BrainwashGUI" "$MACOS/brainwash-cli"
echo "$APP"
