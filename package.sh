#!/bin/bash
# Build Sauron.app and, on macOS, a drag-install DMG.
#
#   ./package.sh              # dist/Sauron.app and dist/Sauron.dmg (macOS)
#   ./package.sh --stage-only # dist/Sauron.app, skip hdiutil
#
# Codesign:
#   CODESIGN_IDENTITY="-"            ad-hoc (default when codesign exists)
#   CODESIGN_IDENTITY="Developer ID Application: Name (TEAMID)"
# Notarize (requires a real Developer ID signature and a notarytool profile):
#   NOTARY_KEYCHAIN_PROFILE=sauron-notary ./package.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
VERSION="${VERSION:-0.2.0}"
APP_NAME="Sauron"
STAGE="${ROOT}/dist/${APP_NAME}.app"
MACOS="${STAGE}/Contents/MacOS"
DMG_PATH="${ROOT}/dist/${APP_NAME}.dmg"
STAGING="$(mktemp -d /tmp/${APP_NAME}.dmg.XXXXXX)"
WORK="$(mktemp -d /tmp/${APP_NAME}.build.XXXXXX)"
STAGE_ONLY=0
if [[ "${1:-}" == "--stage-only" ]]; then
  STAGE_ONLY=1
fi

cleanup() {
  rm -rf "${STAGING}" "${WORK}"
}
trap cleanup EXIT

echo "=== Building ${APP_NAME} ${VERSION} ==="
mkdir -p "${WORK}"
(
  cd "${ROOT}"
  CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags "-X main.version=${VERSION}" -o "${WORK}/sauron-arm64" ./cmd/sauron
  CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags "-X main.version=${VERSION}" -o "${WORK}/sauron-amd64" ./cmd/sauron
)

BIN="${WORK}/Sauron"
if command -v lipo >/dev/null 2>&1; then
  lipo -create -output "${BIN}" "${WORK}/sauron-arm64" "${WORK}/sauron-amd64"
  echo "Universal binary (arm64 + amd64)"
else
  cp "${WORK}/sauron-arm64" "${BIN}"
  echo "lipo not found; app binary is darwin/arm64 only. Run this script on a Mac for a universal build."
fi
chmod 755 "${BIN}"

rm -rf "${STAGE}"
mkdir -p "${MACOS}" "${STAGE}/Contents/Resources"
sed "s/__VERSION__/${VERSION}/g" "${ROOT}/packaging/Info.plist" > "${STAGE}/Contents/Info.plist"
cp "${BIN}" "${MACOS}/Sauron"
chmod 755 "${MACOS}/Sauron"

# Refuse bundles that would make macOS ask for capture or accessibility.
if grep -Ei 'ScreenCapture|Accessibility|AppleEvents|screencapture|NSScreenCapture' "${STAGE}/Contents/Info.plist" "${ROOT}/packaging/entitlements.plist"; then
  echo "Refusing to package: privacy prompt keys found" >&2
  exit 1
fi

echo "=== App bundle: ${STAGE} ==="

sign_app() {
  if ! command -v codesign >/dev/null 2>&1; then
    echo "codesign not available; leaving the app unsigned."
    return 0
  fi
  local identity="${CODESIGN_IDENTITY:--}"
  if [[ "${identity}" == "-" ]]; then
    codesign --force --deep --sign - "${STAGE}"
    echo "Ad-hoc signed (local installs). Gatekeeper still requires a Developer ID signature and notarization for a downloaded DMG."
  else
    codesign --force --deep --options runtime --timestamp \
      --entitlements "${ROOT}/packaging/entitlements.plist" \
      --sign "${identity}" "${STAGE}"
    echo "Signed with ${identity}"
  fi
  codesign --verify --verbose=2 "${STAGE}"
}

sign_app

if [[ "${STAGE_ONLY}" == "1" ]]; then
  echo "Staged only. DMG not created."
  exit 0
fi

if ! command -v hdiutil >/dev/null 2>&1; then
  echo "hdiutil not available (this is not macOS). App bundle is at:"
  echo "  ${STAGE}"
  echo "Run ./package.sh on a Mac to create ${DMG_PATH}."
  exit 0
fi

rm -rf "${STAGING}"
mkdir -p "${STAGING}"
cp -R "${STAGE}" "${STAGING}/"
ln -s /Applications "${STAGING}/Applications"

mkdir -p "${ROOT}/dist"
rm -f "${DMG_PATH}"
hdiutil create \
  -volname "${APP_NAME}" \
  -srcfolder "${STAGING}" \
  -ov \
  -format UDZO \
  "${DMG_PATH}"

if [[ -n "${NOTARY_KEYCHAIN_PROFILE:-}" ]]; then
  if ! command -v xcrun >/dev/null 2>&1; then
    echo "xcrun not available; cannot notarize" >&2
    exit 1
  fi
  echo "=== Submitting for notarization ==="
  xcrun notarytool submit "${DMG_PATH}" \
    --keychain-profile "${NOTARY_KEYCHAIN_PROFILE}" \
    --wait
  xcrun stapler staple "${DMG_PATH}"
  echo "Notarized and stapled."
else
  echo "NOTARY_KEYCHAIN_PROFILE unset; skipping notarization."
fi

echo ""
echo "=== Done ==="
echo "DMG: ${DMG_PATH}"
echo "Open it and drag Sauron to Applications, then open Sauron once."
