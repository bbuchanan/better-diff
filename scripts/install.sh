#!/usr/bin/env sh
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname "$0")" && pwd)"
DEFAULT_SOURCE="$SCRIPT_DIR/better-diff"
if [ ! -f "$DEFAULT_SOURCE" ]; then
  DEFAULT_SOURCE="bin/better-diff"
fi

SOURCE_PATH="${1:-$DEFAULT_SOURCE}"

if [ -z "${PREFIX:-}" ]; then
  OS="$(uname -s)"
  if [ "$OS" = "Linux" ]; then
    printf "Install to ~/.local/bin (recommended) or ~/bin? [~/.local/bin]: "
    read -r answer
    case "$answer" in
      ""|"~/.local/bin") PREFIX="$HOME/.local/bin" ;;
      "~/bin")           PREFIX="$HOME/bin" ;;
      *)                 PREFIX="$answer" ;;
    esac
  else
    PREFIX="$HOME/bin"
  fi
fi

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
