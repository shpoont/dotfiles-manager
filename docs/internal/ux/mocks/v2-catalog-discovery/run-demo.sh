#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MOCK="${ROOT_DIR}/mock-dotfiles-manager.py"
EXPECTED="${ROOT_DIR}/expected-demo.txt"
PYTHON_BIN="${PYTHON:-python3}"
TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/dfm-v2-catalog-mock.XXXXXX")"
trap 'rm -rf "${TMP_DIR}"' EXIT
export DOTFILES_MANAGER_UX_MOCK_STATE="${TMP_DIR}/state.json"

run_command() {
  local status=0
  printf '$ dotfiles-manager'
  printf ' %q' "$@"
  printf '\n'
  "${PYTHON_BIN}" "${MOCK}" "$@" || status=$?
  if [[ ${status} -ne 0 ]]; then
    printf '[exit %d]\n' "${status}"
  fi
  printf '\n'
}

generate() {
  cat <<'HEADER'
# v2 catalog discovery runnable UX mock
# Work item: #228
# Safety: mock only; no live settings, stored settings, or real catalog folders are read or changed.

HEADER
  run_command list
  run_command search git
  run_command explain git
  run_command catalog list
  run_command catalog add '~/broken-recipes' --name broken
  run_command catalog add '~/dotfiles-manager-recipes' --name personal
  run_command list
  run_command explain git
  run_command explain example-tool
  run_command sync --dry-run example-tool
  run_command catalog disable personal
  run_command list
  cat <<'NOTE'
# Scenario precondition for the next command: example-tool was already managed from personal before that catalog became unavailable.

NOTE
  run_command status example-tool
  run_command catalog enable personal
  run_command catalog remove personal
  run_command catalog add shpoont/custom-recipes
}

emit_normalized_demo() {
  generate | "${PYTHON_BIN}" -c 'import sys; data = sys.stdin.read(); sys.stdout.write(data.rstrip("\n") + "\n")'
}

if [[ "${1:-}" == "--check" ]]; then
  actual="${TMP_DIR}/actual-demo.txt"
  emit_normalized_demo > "${actual}"
  diff -u "${EXPECTED}" "${actual}"
  echo "OK: runnable catalog discovery UX mock output matches expected-demo.txt"
else
  emit_normalized_demo
fi
