#!/usr/bin/env bash
set -euo pipefail

output="${1:-${HARBOR_AGENT_OUTPUT:-/app/solution.md}}"
fail() { echo "objective failed: $*" >&2; exit 1; }
require() {
  local pattern="$1" label="${2:-$1}"
  grep -Eiq -- "$pattern" "$output" || fail "missing $label"
}

[[ -s "$output" ]] || fail "missing non-empty output file: $output"
require '(^|[^[:alnum:]_])init([^[:alnum:]_]|$)' 'init step'
require '(^|[^[:alnum:]_])add([^[:alnum:]_]|$)' 'add step'
require '(^|[^[:alnum:]_])status([^[:alnum:]_]|$)' 'status step'
require '(^|[^[:alnum:]_])diff([^[:alnum:]_]|$)' 'diff step'
require '(^|[^[:alnum:]_])save([^[:alnum:]_]|$)' 'save step'
require '(^|[^[:alnum:]_])apply([^[:alnum:]_]|$)' 'apply step'
require 'dry[ -]run|preview' 'preview/dry-run explanation'
require 'desired' 'desired artifact/data explanation'
require 'trust|trusted' 'trust boundary'
require 'backup' 'backup boundary'
require 'ledger|last[ -]applied' 'ledger/last-applied boundary'
require 'live state|live files?|not touched|does not touch' 'live-state boundary'
require 'native|export|import' 'native boundary'
require 'next|happy path|normal path' 'user-oriented flow'
