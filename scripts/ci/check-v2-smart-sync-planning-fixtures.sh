#!/usr/bin/env bash
set -euo pipefail

ROOT="docs/internal/specs/v2/fixtures/smart-sync-planning"
PYTHON_BIN="${PYTHON:-python3}"

if [[ ! -d "$ROOT" ]]; then
  echo "missing fixture directory: $ROOT" >&2
  exit 1
fi

required_txt=(
  live-to-stored.plan.txt
  stored-to-live.plan.txt
  conflict-needs-choice.plan.txt
  missing-app.plan.txt
  missing-stored-settings.plan.txt
  mixed-many-app.plan.txt
)

for file in "${required_txt[@]}"; do
  path="$ROOT/$file"
  if [[ ! -f "$path" ]]; then
    echo "missing smart-sync planning fixture: $path" >&2
    exit 1
  fi
  if ! grep -Fq 'Planning-only command: no files were changed.' "$path"; then
    echo "fixture does not state planning-only/no-write boundary: $path" >&2
    exit 1
  fi
  if grep -Eiq '\b(repo|repository|driver|resource|backup|restore|migration|save|apply)\b|desired://' "$path"; then
    echo "fixture uses forbidden public-output vocabulary: $path" >&2
    grep -Ein '\b(repo|repository|driver|resource|backup|restore|migration|save|apply)\b|desired://' "$path" >&2 || true
    exit 1
  fi
done

for file in \
  live-to-stored.plan.txt \
  stored-to-live.plan.txt \
  conflict-needs-choice.plan.txt \
  missing-app.plan.txt \
  missing-stored-settings.plan.txt \
  mixed-many-app.plan.txt; do
  path="$ROOT/$file"
  if ! grep -Fq 'Direction:' "$path"; then
    echo "fixture does not make direction explicit: $path" >&2
    exit 1
  fi
done

if ! grep -Fq 'Needs choice:' "$ROOT/conflict-needs-choice.plan.txt"; then
  echo "conflict fixture does not require a user choice" >&2
  exit 1
fi

if grep -Fq 'Plan: sync' "$ROOT/conflict-needs-choice.plan.txt"; then
  echo "conflict fixture incorrectly selects an automatic sync direction" >&2
  exit 1
fi

if grep -Fq 'Plan: sync' "$ROOT/missing-app.plan.txt"; then
  echo "missing-app fixture incorrectly plans a write" >&2
  exit 1
fi

if grep -Fq 'Plan: sync' "$ROOT/missing-stored-settings.plan.txt"; then
  echo "missing-stored-settings fixture incorrectly plans an automatic write" >&2
  exit 1
fi

json_path="$ROOT/mixed-many-app.plan.json"
if [[ ! -f "$json_path" ]]; then
  echo "missing JSON concept fixture: $json_path" >&2
  exit 1
fi

"$PYTHON_BIN" - "$json_path" <<'PY'
import json
import sys
from pathlib import Path

path = Path(sys.argv[1])
data = json.loads(path.read_text())

if data.get("mode") != "planning-only":
    raise SystemExit(f"{path}: mode must be planning-only")
if data.get("noWritePerformed") is not True:
    raise SystemExit(f"{path}: noWritePerformed must be true")
if not data.get("apps"):
    raise SystemExit(f"{path}: apps must be present")

items = data.get("items") or []
if len(items) != 6:
    raise SystemExit(f"{path}: expected 6 items, got {len(items)}")

required_fields = {
    "appRef",
    "settingRef",
    "state",
    "decision",
    "direction",
    "wouldWrite",
    "choiceRequired",
    "executableBySync",
}
for index, item in enumerate(items):
    missing = sorted(required_fields - item.keys())
    if missing:
        raise SystemExit(f"{path}: item {index} missing {missing}")

for item in items:
    decision = item["decision"]
    if item["wouldWrite"] and decision != "write":
        raise SystemExit(f"{path}: only decision=write may have wouldWrite=true")
    if decision != "write" and item["executableBySync"]:
        raise SystemExit(f"{path}: non-write item must not be executable")
    if decision == "needs_choice" and not item["choiceRequired"]:
        raise SystemExit(f"{path}: needs_choice item must require choice")
    if decision == "blocked" and item["direction"] not in {"unknown", "none"}:
        raise SystemExit(f"{path}: blocked item direction must not be executable")
    if item["state"] == "conflict":
        if decision == "write" or item["wouldWrite"] or item["executableBySync"]:
            raise SystemExit(f"{path}: conflict item must not be executable")
    if decision == "write":
        for field in ("writeSource", "writeTarget", "liveAddress", "storedAddress", "writeUnitId"):
            if not item.get(field):
                raise SystemExit(f"{path}: write item {item['settingRef']} missing {field}")

write_items = [item for item in items if item["decision"] == "write"]
if len(write_items) != 2:
    raise SystemExit(f"{path}: expected 2 write items, got {len(write_items)}")
if {item["direction"] for item in write_items} != {"live_to_stored", "stored_to_live"}:
    raise SystemExit(f"{path}: write items must cover both safe directions")
PY

echo "v2 smart-sync planning fixtures ok"
