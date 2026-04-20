#!/bin/bash

# MiniSky Universal Installer
# Usage: curl -sSL https://minisky.dev/install.sh | sh

set -e

REPO="minisky-io/minisky"
BINARY_NAME="minisky"

# 1. Detect OS and Architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case $ARCH in
    x86_64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

echo "🛰️  Installing MiniSky for $OS/$ARCH..."

# 2. Get latest version from GitHub
# (Placeholder: In production, this would use the GitHub API)
VERSION="latest"

# 3. Download and Install
# (Placeholder: This assumes your GitHub Releases follow the naming convention)
# DOWNLOAD_URL="https://github.com/$REPO/releases/download/$VERSION/minisky_${OS}_${ARCH}.tar.gz"

echo "✅ Verified architecture. Proceeding with installation..."

# For the local demo, we'll assume the binary is built locally
if [ -f "./minisky" ]; then
    echo "Installing local 'minisky' binary to /usr/local/bin..."
    sudo mv ./minisky /usr/local/bin/minisky
    sudo chmod +x /usr/local/bin/minisky
else
    echo "❌ Error: minisky binary not found in current directory."
    echo "Please run 'go build -o minisky ./cmd/minisky' first."
    exit 1
fi

# 4. Final check
echo ""
echo "🚀 MiniSky installed successfully!"
echo "Try running: minisky start"
echo ""
echo "Note: Ensure Docker is running on your machine."
