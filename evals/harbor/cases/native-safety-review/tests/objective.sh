#!/usr/bin/env bash
set -euo pipefail

output="${1:-${HARBOR_AGENT_OUTPUT:-/app/solution.md}}"
fail() { echo "objective failed: $*" >&2; exit 1; }
require() {
  local pattern="$1" label="${2:-$1}"
  grep -Eiq -- "$pattern" "$output" || fail "missing $label"
}

[[ -s "$output" ]] || fail "missing non-empty output file: $output"
require 'native' 'native operation context'
require 'arbitrary command|shell|string command|argv' 'arbitrary command/argv boundary'
require 'secret|token|credential|account' 'secret/account boundary'
require 'opaque|metadata-only|diffability|diffable' 'opaque/diffability boundary'
require 'lifecycle|quit|reopen|running' 'lifecycle boundary'
require 'trust|reviewed|bundled' 'trust/review boundary'
require 'backup' 'backup requirement'
require 'verify|verification|post-import export' 'verification requirement'
require 'fail closed|blocked|exit 5|safety blocker' 'fail-closed behavior'
require 'opt[ -]in|explicit confirmation|--yes' 'explicit opt-in/confirmation'
require 'manager-owned temp|temp copy|temporary' 'manager-owned temp boundary'
