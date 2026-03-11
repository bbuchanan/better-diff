#!/usr/bin/env sh
set -eu

SOURCE_PATH="${1:-bin/better-diff}"
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
