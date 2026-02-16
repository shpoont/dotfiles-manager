#!/usr/bin/env bash
set -euo pipefail

HAS_GO_MOD="${HAS_GO_MOD:-false}"
STATIC_RESULT="${STATIC_RESULT:-unknown}"
LINUX_RESULT="${LINUX_RESULT:-unknown}"
MACOS_RESULT="${MACOS_RESULT:-unknown}"
COVERAGE_RESULT="${COVERAGE_RESULT:-unknown}"

fail=0

require_success() {
  local name="$1"
  local result="$2"
  if [[ "$result" != "success" ]]; then
    echo "Required job '$name' did not succeed (result=$result)." >&2
    fail=1
  fi
}

require_success "static-checks" "$STATIC_RESULT"
require_success "linux-shards" "$LINUX_RESULT"
require_success "macos-integration" "$MACOS_RESULT"

if [[ "$HAS_GO_MOD" == "true" ]]; then
  if [[ "$COVERAGE_RESULT" != "success" ]]; then
    echo "Required job 'coverage-aggregation' did not succeed (result=$COVERAGE_RESULT)." >&2
    fail=1
  fi

  if [[ ! -f artifacts/coverage-summary.txt ]]; then
    echo "Expected coverage summary artifact is missing." >&2
    fail=1
  else
    echo "Coverage summary:"
    cat artifacts/coverage-summary.txt
  fi
else
  echo "No go.mod detected; coverage gate is skipped in bootstrap mode."
  if [[ "$COVERAGE_RESULT" != "skipped" && "$COVERAGE_RESULT" != "success" ]]; then
    echo "Unexpected coverage result in bootstrap mode: $COVERAGE_RESULT" >&2
    fail=1
  fi
fi

if ((fail)); then
  echo "Final required check failed." >&2
  exit 1
fi

echo "Final required check passed."
