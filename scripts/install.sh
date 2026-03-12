#!/usr/bin/env sh
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname "$0")" && pwd)"
DEFAULT_SOURCE="$SCRIPT_DIR/better-diff"
if [ ! -f "$DEFAULT_SOURCE" ]; then
  DEFAULT_SOURCE="bin/better-diff"
fi

SOURCE_PATH="${1:-$DEFAULT_SOURCE}"
PREFIX="${PREFIX:-$HOME/bin}"
TARGET_PATH="$PREFIX/better-diff"

if [ ! -f "$SOURCE_PATH" ]; then
  echo "missing binary: $SOURCE_PATH" >&2
  exit 1
fi

mkdir -p "$PREFIX"
cp "$SOURCE_PATH" "$TARGET_PATH"
chmod +x "$TARGET_PATH"

echo "installed $TARGET_PATH"
echo "run: $TARGET_PATH --version"
