#!/bin/bash
# Package SauronCapture as a DMG for /Applications install
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
APP_NAME="SauronCapture"
APP_DIR="/tmp/${APP_NAME}_pkg/${APP_NAME}.app"
DMG_NAME="${APP_NAME}.dmg"
DMG_PATH="${SCRIPT_DIR}/../dist/${DMG_NAME}"
STAGING="/tmp/${APP_NAME}_dmg"

echo "=== Building ${APP_NAME} ==="

# Build the binary
mkdir -p "${APP_DIR}/Contents/MacOS" "${APP_DIR}/Contents/Resources"
cp "${SCRIPT_DIR}/Info.plist" "${APP_DIR}/Contents/"

swiftc \
    -O \
    -o "${APP_DIR}/Contents/MacOS/${APP_NAME}" \
    -framework Cocoa \
    -framework Carbon \
    -swift-version 5 \
    "${SCRIPT_DIR}/main.swift"

# Copy app icon
cp "${SCRIPT_DIR}/AppIcon.icns" "${APP_DIR}/Contents/Resources/"

echo "=== App bundle: ${APP_DIR} ==="

# Create DMG staging
rm -rf "${STAGING}"
mkdir -p "${STAGING}"
cp -R "${APP_DIR}" "${STAGING}/"

# Create a symlink to /Applications for drag-install
ln -s /Applications "${STAGING}/Applications"

# Create the DMG
mkdir -p "${SCRIPT_DIR}/../dist"
rm -f "${DMG_PATH}"

hdiutil create \
    -volname "${APP_NAME}" \
    -srcfolder "${STAGING}" \
    -ov \
    -format UDZO \
    "${DMG_PATH}"

# Cleanup
rm -rf "${STAGING}" "/tmp/${APP_NAME}_pkg"

echo ""
echo "=== Done ==="
echo "DMG: ${DMG_PATH}"
echo "Open it and drag SauronCapture to Applications."
