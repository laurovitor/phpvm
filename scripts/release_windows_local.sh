#!/usr/bin/env bash
set -euo pipefail

if ! command -v gh >/dev/null 2>&1; then
  echo "gh CLI not found"
  exit 1
fi

VERSION="${1:?usage: scripts/release_windows_local.sh <tag> [notes] }"
NOTES="${2:-Manual local build release}"

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
DIST_DIR="$ROOT_DIR/dist"
ZIP="$DIST_DIR/phpvm-windows-amd64.zip"

if [[ ! -f "$ZIP" ]]; then
  echo "Missing $ZIP. Run scripts/build_release_local.sh $VERSION first."
  exit 1
fi

cd "$ROOT_DIR"

git tag -f "$VERSION"
git push -f origin "$VERSION"

gh release create "$VERSION" "$ZIP" \
  --repo laurovitor/phpvm \
  --title "$VERSION" \
  --notes "$NOTES" \
  --draft

echo "Draft release created: $VERSION"
