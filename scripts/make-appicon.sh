#!/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SRC="$ROOT/gui/AppIcon-source.png"
MASKED="$ROOT/gui/AppIcon-masked.png"
PNG="$ROOT/gui/AppIcon.png"
ICONSET="$ROOT/gui/AppIcon.iconset"
ICNS="$ROOT/gui/AppIcon.icns"

if [[ ! -f "$SRC" ]]; then
  echo "missing $SRC" >&2
  exit 1
fi

python3 "$ROOT/scripts/mask-appicon.py" "$SRC" "$MASKED"
sips -s format png -z 1024 1024 "$MASKED" --out "$PNG" >/dev/null
rm -rf "$ICONSET"
mkdir -p "$ICONSET"
sips -z 16 16 "$PNG" --out "$ICONSET/icon_16x16.png" >/dev/null
sips -z 32 32 "$PNG" --out "$ICONSET/icon_16x16@2x.png" >/dev/null
sips -z 32 32 "$PNG" --out "$ICONSET/icon_32x32.png" >/dev/null
sips -z 64 64 "$PNG" --out "$ICONSET/icon_32x32@2x.png" >/dev/null
sips -z 128 128 "$PNG" --out "$ICONSET/icon_128x128.png" >/dev/null
sips -z 256 256 "$PNG" --out "$ICONSET/icon_128x128@2x.png" >/dev/null
sips -z 256 256 "$PNG" --out "$ICONSET/icon_256x256.png" >/dev/null
sips -z 512 512 "$PNG" --out "$ICONSET/icon_256x256@2x.png" >/dev/null
sips -z 512 512 "$PNG" --out "$ICONSET/icon_512x512.png" >/dev/null
sips -z 1024 1024 "$PNG" --out "$ICONSET/icon_512x512@2x.png" >/dev/null
iconutil -c icns "$ICONSET" -o "$ICNS"
echo "$ICNS"
