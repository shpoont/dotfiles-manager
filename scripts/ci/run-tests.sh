#!/usr/bin/env bash
set -euo pipefail

SHARD="${1:?usage: run-tests.sh <shard> [platform] [has_go_mod]}"
PLATFORM="${2:-linux}"
HAS_GO_MOD="${3:-$(bash scripts/ci/check-go-module.sh)}"

mkdir -p artifacts

if [[ "$HAS_GO_MOD" != "true" ]]; then
  echo "No go.mod detected; skipping shard '$SHARD' ($PLATFORM)."
  exit 0
fi

export DFM_TEST_SHARD="$SHARD"
export DFM_TEST_PLATFORM="$PLATFORM"

echo "Running shard '$SHARD' on platform '$PLATFORM'..."

case "$SHARD" in
  unit)
    go test ./... -count=1 -covermode=count -coverprofile="artifacts/coverage-unit.out"
    ;;
  integration)
    go test -tags=integration ./... -count=1 -covermode=count -coverprofile="artifacts/coverage-integration.out"
    ;;
  contract)
    go test -tags=contract ./... -count=1 -covermode=count -coverprofile="artifacts/coverage-contract.out"
    if [[ "$PLATFORM" == "linux" && ! -f artifacts/branch-metrics.json ]]; then
      echo "Contract shard requires artifacts/branch-metrics.json for branch coverage gates." >&2
      echo "Generate this file in contract test harness with: {\"branch\": <num>, \"logging_branch\": <num>}." >&2
      exit 1
    fi
    ;;
  performance)
    bash scripts/ci/assert-performance.sh
    ;;
  *)
    echo "Unsupported shard: $SHARD" >&2
    exit 2
    ;;
esac
