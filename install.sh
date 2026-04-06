#!/bin/bash
set -euo pipefail

# Sauron installer — the all-seeing eye for Claude Code
# Usage: curl -fsSL https://raw.githubusercontent.com/sksareen/sauron/main/install.sh | bash

REPO="sksareen/sauron"
INSTALL_DIR="/usr/local/bin"
BINARY="sauron"

echo "👁  Installing Sauron..."
echo ""

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
    arm64)  SUFFIX="darwin-arm64" ;;
    x86_64) SUFFIX="darwin-amd64" ;;
    *)      echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

# Get latest release
LATEST=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"v?([^"]+)".*/\1/')
if [ -z "$LATEST" ]; then
    echo "No releases found. Building from source..."

    # Fallback: build from source
    if ! command -v go &>/dev/null; then
        echo "Error: Go is required to build from source. Install Go 1.23+ first."
        exit 1
    fi

    TMPDIR=$(mktemp -d)
    git clone --depth 1 "https://github.com/${REPO}.git" "$TMPDIR/sauron"
    cd "$TMPDIR/sauron"
    go build -o "$BINARY" ./cmd/sauron
    sudo mv "$BINARY" "$INSTALL_DIR/$BINARY"
    rm -rf "$TMPDIR"
else
    # Download pre-built binary
    URL="https://github.com/${REPO}/releases/download/v${LATEST}/sauron-${SUFFIX}"
    echo "Downloading sauron v${LATEST} for ${SUFFIX}..."

    TMPFILE=$(mktemp)
    curl -fsSL "$URL" -o "$TMPFILE"
    chmod +x "$TMPFILE"
    sudo mv "$TMPFILE" "$INSTALL_DIR/$BINARY"
fi

echo "Binary installed to $INSTALL_DIR/$BINARY"
echo ""

# Run sauron install (registers LaunchAgent + MCP + CLAUDE.md)
"$INSTALL_DIR/$BINARY" install

echo ""

# Start the daemon
"$INSTALL_DIR/$BINARY" start

echo ""
echo "👁  Sauron is watching."
echo "   Open a new Claude Code session and it will have access to your context."
echo ""
echo "   Try: sauron context"
echo "   Try: sauron clipboard"
echo "   Try: sauron status"
