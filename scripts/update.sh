#!/usr/bin/env sh
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname "$0")" && pwd)"
REPO_ROOT="$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)"
PREFIX="${PREFIX:-$HOME/bin}"

if ! git -C "$REPO_ROOT" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  echo "not a git repository: $REPO_ROOT" >&2
  exit 1
fi

CURRENT_BRANCH="$(git -C "$REPO_ROOT" branch --show-current)"
if [ -z "$CURRENT_BRANCH" ]; then
  echo "detached HEAD; refusing to auto-update" >&2
  exit 1
fi

echo "updating $REPO_ROOT ($CURRENT_BRANCH)"
git -C "$REPO_ROOT" pull --ff-only

echo "building"
make -C "$REPO_ROOT" install PREFIX="$PREFIX"

echo "updated $PREFIX/better-diff"
echo "run: $PREFIX/better-diff --version"
