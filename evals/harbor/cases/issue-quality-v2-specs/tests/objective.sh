#!/usr/bin/env bash
set -euo pipefail

output="${1:-${HARBOR_AGENT_OUTPUT:-/app/solution.md}}"
fail() { echo "objective failed: $*" >&2; exit 1; }
require() {
  local pattern="$1" label="${2:-$1}"
  grep -Eiq -- "$pattern" "$output" || fail "missing $label"
}

[[ -s "$output" ]] || fail "missing non-empty output file: $output"
require 'docs/internal/specs/v2/' 'v2 spec reference'
require 'scope' 'scope section'
require 'out[ -]of[ -]scope|non-goals|non goals' 'out-of-scope/non-goals section'
require 'acceptance criteria|acceptance' 'acceptance criteria'
require 'definition of done|done' 'definition of done'
require 'safety|trust|redaction|lifecycle' 'safety invariant'
require 'v1' 'v1 preservation boundary'
require 'v2' 'v2 implementation boundary'
require 'failing cases?|product/spec gaps?|spec gaps?' 'failure-to-gap mapping'
require 'deterministic|go test|contract test|fixture' 'deterministic test complement'
