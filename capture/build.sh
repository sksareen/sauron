#!/bin/bash
# Build SauronCapture.app — lightweight screenshot annotation tool
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
APP_DIR="$HOME/.sauron/SauronCapture.app"

echo "Building SauronCapture..."

# Create app bundle structure
mkdir -p "$APP_DIR/Contents/MacOS" "$APP_DIR/Contents/Resources"
cp "$SCRIPT_DIR/Info.plist" "$APP_DIR/Contents/"
cp "$SCRIPT_DIR/AppIcon.icns" "$APP_DIR/Contents/Resources/" 2>/dev/null || true

# Compile Swift
swiftc \
    -O \
    -o "$APP_DIR/Contents/MacOS/SauronCapture" \
    -framework Cocoa \
    -framework Carbon \
    -swift-version 5 \
    "$SCRIPT_DIR/main.swift"

echo "Built: $APP_DIR"
echo ""
echo "To start: open $APP_DIR"
echo "To auto-start at login: sauron capture --install"
