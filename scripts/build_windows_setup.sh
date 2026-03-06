#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
DIST_DIR="$ROOT_DIR/dist"
VERSION="${1:-0.1.1-alpha.local}"

if ! command -v makensis >/dev/null 2>&1; then
  echo "makensis not found. Install NSIS first (apt install nsis)."
  exit 1
fi

if [[ ! -f "$DIST_DIR/phpvm.exe" ]]; then
  echo "Missing $DIST_DIR/phpvm.exe. Run scripts/build_release_local.sh $VERSION first."
  exit 1
fi

makensis \
  -DAPP_VERSION="$VERSION" \
  -DINPUT_EXE="..\\..\\dist\\phpvm.exe" \
  -DOUT_DIR="..\\..\\dist" \
  "$ROOT_DIR/installer/windows/phpvm.nsi"

echo "Built installer: $DIST_DIR/phpvm-setup.exe"
