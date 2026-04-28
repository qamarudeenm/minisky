#!/bin/bash

# MiniSky Multi-Platform Build Script
VERSION=$(grep "var Version" pkg/version/version.go | cut -d '"' -f 2)
echo "Building MiniSky v$VERSION..."

# 1. Build UI Assets (Ensures dashboard is embedded)
echo "Compiling UI assets..."
cd ui && npm run build && cd ..

# 2. Build for Linux (Current)
echo "Building for Linux (amd64)..."
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/minisky ./cmd/minisky

# 3. Build for Windows (.exe)
echo "Building for Windows (amd64)..."
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o bin/minisky.exe ./cmd/minisky

# 4. Build for macOS (Intel & Apple Silicon)
echo "Building for macOS (amd64 & arm64)..."
GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -o bin/minisky-darwin-amd64 ./cmd/minisky
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -o bin/minisky-darwin-arm64 ./cmd/minisky

echo "Done! Binaries are available in the 'bin/' directory:"
ls -lh bin/
