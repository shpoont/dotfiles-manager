#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MOCK="${ROOT_DIR}/mock-dotfiles-manager.py"
EXPECTED="${ROOT_DIR}/expected-demo.txt"
PYTHON_BIN="${PYTHON:-python3}"
TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/dfm-v2-catalog-mock.XXXXXX")"
trap 'rm -rf "${TMP_DIR}"' EXIT

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
# Work item: #228 recontracted official-catalog discovery baseline
# Safety: mock only; no live settings, stored settings, real catalog folders, secrets, or network resources are read or changed.

HEADER
  run_command list
  run_command search git
  run_command search wezterm
  run_command explain git
  run_command catalog list
}

emit_normalized_demo() {
  generate | "${PYTHON_BIN}" -c 'import sys; data = sys.stdin.read(); sys.stdout.write(data.rstrip("\n") + "\n")'
}

if [[ "${1:-}" == "--check" ]]; then
  actual="${TMP_DIR}/actual-demo.txt"
  emit_normalized_demo > "${actual}"
  diff -u "${EXPECTED}" "${actual}"
  echo "OK: recontracted catalog discovery UX mock output matches expected-demo.txt"
else
  emit_normalized_demo
fi
