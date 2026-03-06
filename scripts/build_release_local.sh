#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
DIST_DIR="$ROOT_DIR/dist"
VERSION="${1:-dev}"

rm -rf "$DIST_DIR"
mkdir -p "$DIST_DIR"

# Linux (host arch)
GOOS=linux GOARCH=arm64 go build -ldflags "-s -w -X main.appVersion=$VERSION" -o "$DIST_DIR/phpvm-linux-arm64" ./cmd/phpvm

# Windows amd64
GOOS=windows GOARCH=amd64 go build -ldflags "-s -w -X main.appVersion=$VERSION" -o "$DIST_DIR/phpvm.exe" ./cmd/phpvm
(
  cd "$DIST_DIR"
  zip -q "phpvm-windows-amd64.zip" phpvm.exe
)

cat <<MSG
Built artifacts:
- $DIST_DIR/phpvm-linux-arm64
- $DIST_DIR/phpvm.exe
- $DIST_DIR/phpvm-windows-amd64.zip
MSG
