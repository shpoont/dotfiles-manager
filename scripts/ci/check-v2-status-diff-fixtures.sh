#!/usr/bin/env bash
set -euo pipefail

ROOT="docs/internal/specs/v2/fixtures/status-diff-read-only"
PYTHON_BIN="${PYTHON:-python3}"

if [[ ! -d "$ROOT" ]]; then
  echo "missing fixture directory: $ROOT" >&2
  exit 1
fi

required_txt=(
  clean.status.txt
  live-only-change.status.txt
  stored-only-change.status.txt
  conflict.status.txt
  missing-app.status.txt
  missing-stored-settings.status.txt
  many-app.status.txt
  live-vs-stored.diff.txt
  stored-vs-live.diff.txt
  conflict.diff.txt
)

for file in "${required_txt[@]}"; do
  path="$ROOT/$file"
  if [[ ! -f "$path" ]]; then
    echo "missing status/diff fixture: $path" >&2
    exit 1
  fi
  if ! grep -Fq 'Read-only command: no files were changed.' "$path"; then
    echo "fixture does not state read-only/no-write boundary: $path" >&2
    exit 1
  fi
  if grep -Eiq '\b(repo|repository|driver|resource|backup|restore|migration)\b|desired://' "$path"; then
    echo "fixture uses forbidden public-output vocabulary: $path" >&2
    grep -Ein '\b(repo|repository|driver|resource|backup|restore|migration)\b|desired://' "$path" >&2 || true
    exit 1
  fi
done

for file in \
  live-only-change.status.txt \
  stored-only-change.status.txt \
  conflict.status.txt \
  many-app.status.txt \
  live-vs-stored.diff.txt \
  stored-vs-live.diff.txt \
  conflict.diff.txt; do
  path="$ROOT/$file"
  if ! grep -Fq 'Direction:' "$path"; then
    echo "fixture does not make direction explicit: $path" >&2
    exit 1
  fi
done

if [[ ! -f "$ROOT/json.status.json" ]]; then
  echo "missing JSON boundary fixture: $ROOT/json.status.json" >&2
  exit 1
fi

"$PYTHON_BIN" -m json.tool "$ROOT/json.status.json" >/dev/null

echo "v2 status/diff fixtures ok"
