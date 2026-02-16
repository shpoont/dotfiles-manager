#!/usr/bin/env bash
set -euo pipefail

HAS_GO_MOD="${1:-$(bash scripts/ci/check-go-module.sh)}"

if [[ "$HAS_GO_MOD" != "true" ]]; then
  echo "No go.mod detected; skipping macOS sandbox lane."
  exit 0
fi

SANDBOX_ROOT="$(mktemp -d)"
cleanup() {
  chmod -R u+w "$SANDBOX_ROOT" 2>/dev/null || true
  rm -rf "$SANDBOX_ROOT" 2>/dev/null || true
}
trap cleanup EXIT

ORIGINAL_HOME="${HOME:-}"
if [[ -n "$ORIGINAL_HOME" && -d "$ORIGINAL_HOME/.asdf" ]]; then
  export ASDF_DIR="${ASDF_DIR:-$ORIGINAL_HOME/.asdf}"
  export ASDF_DATA_DIR="${ASDF_DATA_DIR:-$ORIGINAL_HOME/.asdf}"
fi

export DFM_SANDBOX_ROOT="$SANDBOX_ROOT"
export HOME="$SANDBOX_ROOT/home"
export TMPDIR="$SANDBOX_ROOT/tmp"
export GOPATH="$SANDBOX_ROOT/gopath"
export GOCACHE="$SANDBOX_ROOT/gocache"
export GOMODCACHE="$SANDBOX_ROOT/gomodcache"
export XDG_CACHE_HOME="$SANDBOX_ROOT/xdg-cache"

mkdir -p \
  "$HOME" \
  "$TMPDIR" \
  "$GOPATH" \
  "$GOCACHE" \
  "$GOMODCACHE" \
  "$XDG_CACHE_HOME"

echo "Running macOS tests with sandbox env rooted at: $DFM_SANDBOX_ROOT"

bash scripts/ci/run-tests.sh integration macos "$HAS_GO_MOD"
bash scripts/ci/run-tests.sh contract macos "$HAS_GO_MOD"
