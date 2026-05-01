#!/bin/bash

# MiniSky Universal Installer
# Usage: curl -sSL https://minisky.bmics.com.ng/install.sh | bash

set -e

REPO="qamarudeenm/minisky"
BINARY_NAME="minisky"

# 1. Detect OS and Architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

if [[ "$OS" == mingw* || "$OS" == msys* ]]; then
    OS="windows"
fi

case $ARCH in
    x86_64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

echo "🛰️  Installing MiniSky for $OS/$ARCH..."

# 2. Get latest version from GitHub
RELEASE_JSON=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest")
VERSION=$(printf '%s' "$RELEASE_JSON" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')

if [ -z "$VERSION" ]; then
    echo "❌ Error: Could not detect latest version."
    exit 1
fi

echo "📦 Found version $VERSION"

# 3. Download and Install
EXT="tar.gz"
BIN_OUT="$BINARY_NAME"
if [ "$OS" = "windows" ]; then 
    EXT="zip"
    BIN_OUT="${BINARY_NAME}.exe"
fi

DOWNLOAD_URL="https://github.com/$REPO/releases/download/$VERSION/minisky_${OS}_${ARCH}.${EXT}"
ASSET_NAME="minisky_${OS}_${ARCH}.${EXT}"
DOWNLOAD_URL=$(printf '%s' "$RELEASE_JSON" | grep -o '"browser_download_url": "[^"]*"' | sed -E 's/.*"([^"]+)"/\1/' | grep "/${ASSET_NAME}$" | head -n 1 || true)

if [ -z "$DOWNLOAD_URL" ]; then
    echo "❌ Error: Release $VERSION does not include ${ASSET_NAME}."
    echo "This platform is not published in the latest release yet."
    exit 1
fi

echo "📥 Downloading from $DOWNLOAD_URL..."
curl -fsSL -o "minisky.$EXT" "$DOWNLOAD_URL"

if [ "$EXT" = "tar.gz" ]; then
    tar -xzf "minisky.$EXT" minisky
else
    # Windows/Zip
    unzip -q "minisky.$EXT" "$BIN_OUT"
fi

if [ "$OS" = "windows" ]; then
    echo "✅ MiniSky binary ($BIN_OUT) is ready in the current directory."
    echo "To use it globally, add this folder to your Windows PATH."
else
    echo "🚀 Installing '$BIN_OUT' to /usr/local/bin..."
    sudo mv "./$BIN_OUT" "/usr/local/bin/$BIN_OUT"
    sudo chmod +x "/usr/local/bin/$BIN_OUT"
fi

if [ -f "minisky.$EXT" ]; then
    rm "minisky.$EXT"
fi

# 4. Final check
echo ""
echo "🚀 MiniSky installation process finished!"
if [ "$OS" != "windows" ]; then
    echo "Try running: minisky start"
fi
echo ""
echo "Note: Ensure Docker is running on your machine."
