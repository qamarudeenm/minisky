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
    INSTALL_DIR="/usr/local/bin"

    # Attempt a password-free install first.
    # On Homebrew-managed macOS, /usr/local/bin is often user-writable.
    # When the script is piped through `curl | sh` there is no real TTY,
    # so an unconditional `sudo` prompt breaks (issue #6).
    if [ -w "$INSTALL_DIR" ]; then
        echo "🚀 Installing '$BIN_OUT' to $INSTALL_DIR (no sudo needed)..."
        mv "./$BIN_OUT" "$INSTALL_DIR/$BIN_OUT"
        chmod +x "$INSTALL_DIR/$BIN_OUT"
    else
        # Fall back to sudo, but warn the user why they're being prompted.
        # If the script was piped through curl, sudo may not be able to read
        # the password from stdin. In that case we install to ~/.local/bin instead.
        if [ -t 0 ]; then
            # stdin is a real terminal — sudo can prompt safely.
            echo "🔑 Admin access needed to install to $INSTALL_DIR (your macOS user password):"
            sudo mv "./$BIN_OUT" "$INSTALL_DIR/$BIN_OUT"
            sudo chmod +x "$INSTALL_DIR/$BIN_OUT"
            echo "✅ Installed to $INSTALL_DIR/$BIN_OUT"
        else
            # No TTY (e.g. curl | sh) — avoid a broken sudo prompt.
            FALLBACK_DIR="$HOME/.local/bin"
            mkdir -p "$FALLBACK_DIR"
            mv "./$BIN_OUT" "$FALLBACK_DIR/$BIN_OUT"
            chmod +x "$FALLBACK_DIR/$BIN_OUT"
            echo "✅ Installed to $FALLBACK_DIR/$BIN_OUT (no sudo required)"
            echo ""
            echo "⚠️  '$FALLBACK_DIR' may not be on your PATH."
            echo "   Add the following line to your shell profile (~/.zshrc or ~/.bash_profile):"
            echo "   export PATH=\"\$HOME/.local/bin:\$PATH\""
            echo "   Then run: source ~/.zshrc  (or open a new terminal)"
            echo ""
            echo "   Alternatively, re-run the installer directly in your terminal (not via curl pipe):"
            echo "   curl -fsSL https://minisky.bmics.com.ng/install.sh -o install.sh && bash install.sh"
        fi
    fi
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
