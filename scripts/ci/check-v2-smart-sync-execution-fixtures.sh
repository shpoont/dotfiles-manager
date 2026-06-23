#!/usr/bin/env bash
set -euo pipefail

ROOT="docs/internal/specs/v2/fixtures/smart-sync-execution"
PYTHON_BIN="${PYTHON:-python3}"

if [[ ! -d "$ROOT" ]]; then
  echo "missing fixture directory: $ROOT" >&2
  exit 1
fi

required_txt=(
  live-to-stored.execution.txt
  stored-to-live.execution.txt
  confirmation-required.execution.txt
  refused-conflict.execution.txt
  mixed-many-app.execution.txt
  partial-failure.execution.txt
  stale-plan.execution.txt
  folder-tree-review-required.execution.txt
)

for file in "${required_txt[@]}"; do
  path="$ROOT/$file"
  if [[ ! -f "$path" ]]; then
    echo "missing smart-sync execution fixture: $path" >&2
    exit 1
  fi
  if ! grep -Fq 'Changed:' "$path" || ! grep -Fq 'Skipped:' "$path" || ! grep -Fq 'Failed:' "$path"; then
    echo "fixture does not include changed/skipped/failed counts: $path" >&2
    exit 1
  fi
  if grep -Eiq '\b(repo|repository|driver|resource|backup|restore|migration|save|apply)\b|desired://' "$path"; then
    echo "fixture uses forbidden public-output vocabulary: $path" >&2
    grep -Ein '\b(repo|repository|driver|resource|backup|restore|migration|save|apply)\b|desired://' "$path" >&2 || true
    exit 1
  fi
done

for file in \
  live-to-stored.execution.txt \
  stored-to-live.execution.txt \
  mixed-many-app.execution.txt \
  partial-failure.execution.txt; do
  path="$ROOT/$file"
  if ! grep -Fq 'live settings -> stored settings' "$path" && ! grep -Fq 'stored settings -> live settings' "$path"; then
    echo "executing fixture does not make a sync direction explicit: $path" >&2
    exit 1
  fi
done

if ! grep -Fq 'Sync not run: confirmation required.' "$ROOT/confirmation-required.execution.txt"; then
  echo "confirmation fixture does not require confirmation" >&2
  exit 1
fi

if grep -Fq 'Sync complete.' "$ROOT/refused-conflict.execution.txt"; then
  echo "conflict fixture incorrectly completes sync" >&2
  exit 1
fi

if ! grep -Fq 'conflict needs a choice' "$ROOT/refused-conflict.execution.txt"; then
  echo "conflict fixture does not require a choice" >&2
  exit 1
fi

if ! grep -Fq 'Not attempted' "$ROOT/partial-failure.execution.txt"; then
  echo "partial-failure fixture does not report not-attempted work" >&2
  exit 1
fi

if ! grep -Fq 'settings changed since the plan was checked' "$ROOT/stale-plan.execution.txt"; then
  echo "stale-plan fixture does not refuse stale evidence" >&2
  exit 1
fi

if ! grep -Fq 'detailed file-by-file review' "$ROOT/folder-tree-review-required.execution.txt"; then
  echo "folder-tree fixture does not explain the review requirement" >&2
  exit 1
fi

json_path="$ROOT/mixed-many-app.execution.json"
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

if data.get("schema") != "dotfiles-manager.sync-execution.v2":
    raise SystemExit(f"{path}: unexpected schema")
if data.get("schemaVersion") != 1:
    raise SystemExit(f"{path}: schemaVersion must be 1")
if data.get("command") != "sync":
    raise SystemExit(f"{path}: command must be sync")
if data.get("noWritePerformed") is not None:
    raise SystemExit(f"{path}: noWritePerformed belongs to planning fixtures, not execution")

summary = data.get("summary") or {}
for key in ("status", "changed", "skipped", "failed", "notAttempted", "writesToStoredSettings", "writesToLiveSettings", "needsChoice", "blocked"):
    if key not in summary:
        raise SystemExit(f"{path}: summary missing {key}")
if summary["changed"] < 2:
    raise SystemExit(f"{path}: expected at least two changed items")
if summary["writesToStoredSettings"] < 1 or summary["writesToLiveSettings"] < 1:
    raise SystemExit(f"{path}: expected changed items in both directions")

items = data.get("items") or []
if len(items) < 4:
    raise SystemExit(f"{path}: expected at least four items")

required_fields = {
    "targetRef",
    "settingRef",
    "state",
    "decision",
    "direction",
    "wouldWrite",
    "choiceRequired",
    "allowedChoices",
    "result",
    "reasonCode",
    "evidenceId",
    "valuesRedacted",
    "executableBySync",
}
for index, item in enumerate(items):
    missing = sorted(required_fields - item.keys())
    if missing:
        raise SystemExit(f"{path}: item {index} missing {missing}")
    for forbidden in ("backupRefs", "restoreRefs", "repository", "repo", "driver", "resource", "diagnostics"):
        if forbidden in item:
            raise SystemExit(f"{path}: item {index} exposes forbidden key {forbidden}")

changed = [item for item in items if item["result"] == "changed"]
if {item["direction"] for item in changed} != {"live_to_stored", "stored_to_live"}:
    raise SystemExit(f"{path}: changed items must cover both directions")

for item in items:
    if item["decision"] != "write" and item["executableBySync"]:
        raise SystemExit(f"{path}: non-write item must not be executable")
    if item["decision"] == "write" and not item["wouldWrite"]:
        raise SystemExit(f"{path}: write item must declare wouldWrite=true")
    if item["decision"] == "needs_choice" and not item["choiceRequired"]:
        raise SystemExit(f"{path}: choice item must declare choiceRequired=true")
    if item["state"] == "conflict" and item["result"] == "changed":
        raise SystemExit(f"{path}: conflict item must not be changed")
    if item["result"] == "changed" and item["decision"] != "write":
        raise SystemExit(f"{path}: only write decisions can be changed")
PY

echo "v2 smart-sync execution fixtures ok"
