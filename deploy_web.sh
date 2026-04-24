#!/bin/bash

# ==============================================================================
# MiniSky Web Deployment Script
# Deploys the static landing page to the remote server.
# ==============================================================================

# Configuration
REMOTE_HOST="bmics_server"
REMOTE_DIR="/var/www/minisky.bmics.com.ng/web" # Updated to include /web based on server structure
SOURCE_DIR="web/"

# 1. Validation
if [ -z "$REMOTE_SERVER_BMICS" ]; then
    echo "❌ Error: REMOTE_SERVER_BMICS environment variable is not set."
    echo "Please set it using: export REMOTE_SERVER_BMICS='your_password'"
    exit 1
fi

if ! command -v sshpass &> /dev/null; then
    echo "❌ Error: sshpass is not installed."
    echo "Install it using: sudo apt update && sudo apt install sshpass"
    exit 1
fi

if ! command -v rsync &> /dev/null; then
    echo "❌ Error: rsync is not installed."
    exit 1
fi

echo "🚀 Starting deployment to $REMOTE_HOST..."
echo "📂 Source: $SOURCE_DIR, install.sh"
echo "🌐 Destination: $REMOTE_DIR"

# 2. Deployment
# We use export SSHPASS and sshpass -e to avoid passing the password via CLI arguments
export SSHPASS="$REMOTE_SERVER_BMICS"

sshpass -e rsync -avz \
    -e "ssh -o StrictHostKeyChecking=no" \
    "$SOURCE_DIR" "install.sh" \
    "$REMOTE_HOST:$REMOTE_DIR"

# 3. Status Check
if [ $? -eq 0 ]; then
    echo "✅ Deployment successful! Visit https://minisky.bmics.com.ng"
else
    echo "❌ Deployment failed. Please check your connection and credentials."
    exit 1
fi
