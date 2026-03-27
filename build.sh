#!/bin/bash
set -e

# Cross-compile build script

VERSION=${1:-"1.0.0"}
OUTPUT_DIR="dist"
LDFLAGS="-s -w -X main.version=${VERSION}"

mkdir -p "$OUTPUT_DIR"

echo "Building browser-history-manager v${VERSION}..."

# Windows
echo "  windows/amd64..."
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="$LDFLAGS" -o "${OUTPUT_DIR}/browser-history-manager-windows-amd64.exe" .

# macOS
echo "  darwin/arm64..."
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="$LDFLAGS" -o "${OUTPUT_DIR}/browser-history-manager-darwin-arm64" .

echo "  darwin/amd64..."
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags="$LDFLAGS" -o "${OUTPUT_DIR}/browser-history-manager-darwin-amd64" .

# Linux
echo "  linux/amd64..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="$LDFLAGS" -o "${OUTPUT_DIR}/browser-history-manager-linux-amd64" .

echo "  linux/arm64..."
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="$LDFLAGS" -o "${OUTPUT_DIR}/browser-history-manager-linux-arm64" .

echo ""
echo "Build complete:"
ls -lh "${OUTPUT_DIR}/"

