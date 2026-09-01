#!/bin/bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=version.sh
source "$ROOT/scripts/version.sh"
VERSION="$(brainwash_version --release)"
# Signing names / team ID are public on the certificate. The notary password
# lives in the Keychain profile, never in this repo.
APP_SIGN="${BRAINWASH_APP_SIGN:-Developer ID Application: Geesec Security (Chengdu) Technology Co., Ltd (UV89MYY936)}"
PKG_SIGN="${BRAINWASH_PKG_SIGN:-Developer ID Installer: Geesec Security (Chengdu) Technology Co., Ltd (UV89MYY936)}"
NOTARY_PROFILE="${BRAINWASH_NOTARY_PROFILE:-brainwash-notary}"
BUNDLE_ID="works.earendil.brainwash"

APP="$ROOT/dist/brainwash.app"
PKG="$ROOT/dist/brainwash-${VERSION}-arm64.pkg"
ENTITLEMENTS="$ROOT/gui/Brainwash.entitlements"

cd "$ROOT"

echo "==> build arm64 helper + GUI"
LDFLAGS="$(brainwash_ldflags "$VERSION")"
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="$LDFLAGS" -o "$ROOT/dist/brainwash-cli" ./cmd/brainwash-cli
( cd "$ROOT/gui" && swift build -c release --arch arm64 --product BrainwashGUI )

bash "$ROOT/scripts/make-appicon.sh"

echo "==> assemble .app"
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
GUI_BIN="$ROOT/gui/.build/arm64-apple-macosx/release/BrainwashGUI"
if [[ ! -x "$GUI_BIN" ]]; then
  echo "arm64 BrainwashGUI not found at $GUI_BIN" >&2
  exit 1
fi
cp "$GUI_BIN" "$APP/Contents/MacOS/BrainwashGUI"
cp "$ROOT/dist/brainwash-cli" "$APP/Contents/MacOS/brainwash-cli"
cp "$ROOT/gui/Info.plist" "$APP/Contents/Info.plist"
brainwash_stamp_plist "$APP/Contents/Info.plist" "$VERSION"
echo "==> version $VERSION"
cp "$ROOT/gui/AppIcon.icns" "$APP/Contents/Resources/AppIcon.icns"
printf 'APPL????' > "$APP/Contents/PkgInfo"
chmod +x "$APP/Contents/MacOS/BrainwashGUI" "$APP/Contents/MacOS/brainwash-cli"

echo "==> architectures"
file "$APP/Contents/MacOS/BrainwashGUI"
file "$APP/Contents/MacOS/brainwash-cli"
for bin in "$APP/Contents/MacOS/BrainwashGUI" "$APP/Contents/MacOS/brainwash-cli"; do
  archs=$(lipo -archs "$bin")
  echo "$bin: $archs"
  if [[ "$archs" != *arm64* ]]; then
    echo "expected arm64 in $bin, got: $archs" >&2
    exit 1
  fi
  if [[ "$archs" == *x86_64* ]]; then
    echo "unexpected x86_64 in $bin" >&2
    exit 1
  fi
done

echo "==> codesign"
codesign --force --options runtime --timestamp --sign "$APP_SIGN" --entitlements "$ENTITLEMENTS" "$APP/Contents/MacOS/brainwash-cli"
codesign --force --options runtime --timestamp --sign "$APP_SIGN" --entitlements "$ENTITLEMENTS" "$APP/Contents/MacOS/BrainwashGUI"
codesign --force --options runtime --timestamp --sign "$APP_SIGN" --entitlements "$ENTITLEMENTS" "$APP"
codesign --verify --strict --deep --verbose=2 "$APP"

echo "==> pkgbuild"
rm -f "$PKG"
productbuild --component "$APP" /Applications --sign "$PKG_SIGN" --timestamp --identifier "$BUNDLE_ID" --version "$VERSION" "$PKG"
pkgutil --check-signature "$PKG"

echo "==> notarize"
xcrun notarytool submit "$PKG" --keychain-profile "$NOTARY_PROFILE" --wait
echo "==> staple"
xcrun stapler staple "$PKG"
xcrun stapler staple "$APP"
spctl --assess --type install -vv "$PKG" || true
echo "PKG $PKG"
echo "APP $APP"
