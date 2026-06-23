#!/usr/bin/env bash
set -euo pipefail

ROOT="docs/internal/specs/v2/fixtures/partial-many-app-sync"
PYTHON_BIN="${PYTHON:-python3}"

if [[ ! -d "$ROOT" ]]; then
  echo "missing fixture directory: $ROOT" >&2
  exit 1
fi

required_pairs=(
  01-many-app-status.status
  02-many-app-planned-sync.plan
  03-partial-by-app.plan
  04-partial-by-setting-area.plan
  05-partial-by-scalar-setting.plan
  06-mixed-sync-result.execution
)

for stem in "${required_pairs[@]}"; do
  txt="$ROOT/$stem.txt"
  json="$ROOT/$stem.json"
  if [[ ! -f "$txt" ]]; then
    echo "missing partial/many-app text fixture: $txt" >&2
    exit 1
  fi
  if [[ ! -f "$json" ]]; then
    echo "missing partial/many-app JSON fixture: $json" >&2
    exit 1
  fi
  if ! grep -Fq 'Summary:' "$txt"; then
    echo "fixture lacks a Summary section: $txt" >&2
    exit 1
  fi
  if grep -Eiq '\b(repo|repository|driver|resource|resourceId|backup|restore|migration|desired)\b|desired://' "$txt"; then
    echo "fixture uses forbidden public-output vocabulary: $txt" >&2
    grep -Ein '\b(repo|repository|driver|resource|resourceId|backup|restore|migration|desired)\b|desired://' "$txt" >&2 || true
    exit 1
  fi
  if ! grep -Eq 'live settings|stored settings|sync|Changed|Will sync|Cannot sync now|Needs your choice|Up to date' "$txt"; then
    echo "fixture does not use public sync vocabulary: $txt" >&2
    exit 1
  fi
done

for file in "$ROOT/02-many-app-planned-sync.plan.txt"; do
  for heading in 'Will sync:' 'Needs your choice:' 'Cannot sync now:' 'Not selected:'; do
    if ! grep -Fq "$heading" "$file"; then
      echo "fixture does not visibly separate $heading: $file" >&2
      exit 1
    fi
  done
  if grep -Eq '^Skipped:' "$file"; then
    echo "fixture flattens user-facing states under a Skipped heading: $file" >&2
    exit 1
  fi
done
for file in "$ROOT/06-mixed-sync-result.execution.txt"; do
  for heading in 'Changed:' 'Needs your choice:' 'Cannot sync now:' 'Not selected:'; do
    if ! grep -Fq "$heading" "$file"; then
      echo "fixture does not visibly separate $heading: $file" >&2
      exit 1
    fi
  done
  if grep -Eq '^Skipped:' "$file"; then
    echo "fixture flattens user-facing states under a Skipped heading: $file" >&2
    exit 1
  fi
done

"$PYTHON_BIN" - "$ROOT" <<'PY'
import json
import re
import sys
from pathlib import Path

root = Path(sys.argv[1])
required = [
    "01-many-app-status.status",
    "02-many-app-planned-sync.plan",
    "03-partial-by-app.plan",
    "04-partial-by-setting-area.plan",
    "05-partial-by-scalar-setting.plan",
    "06-mixed-sync-result.execution",
]
forbidden_keys = {
    "driver",
    "driverId",
    "resource",
    "resourceId",
    "desiredUri",
    "desiredURI",
    "desiredRelPath",
    "diagnostics",
    "backupRefs",
    "restoreRefs",
}
forbidden_public_value = re.compile(r"\b(repo|repository|driver|resource|resourceId|backup|restore|migration|desired)\b|desired://", re.I)
status_write_keys = {"decision", "wouldWrite", "executableBySync", "result"}
unsafe_states = {"conflict", "blocked", "missing-in-live-settings", "unsupported"}

summary_labels = {
    "Up to date": "upToDate",
    "Changed in live settings": "changedInLiveSettings",
    "Changed in stored settings": "changedInStoredSettings",
    "Needs choice": "needsChoice",
    "Cannot sync now": "cannotSyncNow",
    "Will sync": "willSync",
    "Changed": "changed",
    "Failed": "failed",
    "Not attempted": "notAttempted",
    "Writes to stored settings": "writesToStoredSettings",
    "Writes to live settings": "writesToLiveSettings",
    "Out-of-scope writes": "outOfScopeWrites",
}


def walk(obj):
    if isinstance(obj, dict):
        for key, value in obj.items():
            yield key, value
            yield from walk(value)
    elif isinstance(obj, list):
        for value in obj:
            yield from walk(value)


def parse_summary(path):
    text = path.read_text()
    marker = "Summary:\n"
    if marker not in text:
        raise SystemExit(f"{path}: missing Summary section")
    summary = {}
    for raw in text.split(marker, 1)[1].splitlines():
        if not raw.startswith("  ") or ":" not in raw:
            continue
        label, value = raw.strip().split(":", 1)
        value = value.strip()
        if re.fullmatch(r"-?\d+", value):
            summary[label] = int(value)
        else:
            summary[label] = value
    return summary


def load(stem):
    path = root / f"{stem}.json"
    data = json.loads(path.read_text())
    for key, value in walk(data):
        if key in forbidden_keys:
            raise SystemExit(f"{path}: exposes forbidden key {key}")
        if isinstance(value, str) and forbidden_public_value.search(value):
            raise SystemExit(f"{path}: exposes forbidden public vocabulary in value {value!r}")
    for key in ("command", "selection", "summary", "items"):
        if key not in data:
            raise SystemExit(f"{path}: missing {key}")
    if not isinstance(data["items"], list) or not data["items"]:
        raise SystemExit(f"{path}: items must be a non-empty list")
    return path, data


def compare_text_summary(stem, data, expected_labels):
    text_summary = parse_summary(root / f"{stem}.txt")
    for label in expected_labels:
        key = summary_labels[label]
        if label not in text_summary:
            raise SystemExit(f"{stem}.txt: summary missing {label}")
        if key not in data["summary"]:
            raise SystemExit(f"{stem}.json: summary missing {key}")
        if text_summary[label] != data["summary"][key]:
            raise SystemExit(f"{stem}: text summary {label}={text_summary[label]!r} does not match JSON {key}={data['summary'][key]!r}")


def bool_field(item, key, path):
    if key not in item or not isinstance(item[key], bool):
        raise SystemExit(f"{path}: item {item.get('settingRef')} missing boolean {key}")
    return item[key]


def check_write_item(path, item):
    for key in ("targetRef", "settingRef", "state", "decision", "direction", "wouldWrite", "choiceRequired", "allowedChoices", "executableBySync", "valuesRedacted"):
        if key not in item:
            raise SystemExit(f"{path}: item missing {key}: {item}")
    if not isinstance(item["allowedChoices"], list):
        raise SystemExit(f"{path}: allowedChoices must be a list for {item['settingRef']}")
    would_write = bool_field(item, "wouldWrite", path)
    executable = bool_field(item, "executableBySync", path)
    choice = bool_field(item, "choiceRequired", path)
    decision = item["decision"]
    result = item.get("result")
    state = item["state"]
    if decision != "write" and (would_write or executable):
        raise SystemExit(f"{path}: non-write item is executable: {item['settingRef']}")
    if decision == "write" and (not would_write or not executable):
        raise SystemExit(f"{path}: write item is not executable: {item['settingRef']}")
    if choice and executable:
        raise SystemExit(f"{path}: choice item is executable: {item['settingRef']}")
    if state in unsafe_states and executable:
        raise SystemExit(f"{path}: unsafe/unavailable state is executable: {item['settingRef']}")
    if result == "changed" and decision != "write":
        raise SystemExit(f"{path}: changed result without write decision: {item['settingRef']}")


loaded = {}
for stem in required:
    path, data = load(stem)
    loaded[stem] = (path, data)

# Read-only status fixture must stay read-only and non-executable.
path, status = loaded["01-many-app-status.status"]
if status.get("command") != "status" or status.get("readOnly") is not True or status.get("changedFiles") != 0:
    raise SystemExit(f"{path}: status fixture must be read-only status with changedFiles=0")
for item in status["items"]:
    for key in ("targetRef", "settingRef", "status", "direction", "diffAvailable", "valuesRedacted"):
        if key not in item:
            raise SystemExit(f"{path}: status item missing {key}: {item}")
    leaked = status_write_keys & item.keys()
    if leaked:
        raise SystemExit(f"{path}: read-only status item exposes write fields {sorted(leaked)}: {item['settingRef']}")
summary = status["summary"]
items = status["items"]
if summary.get("checkedSettings") != len(items):
    raise SystemExit(f"{path}: checkedSettings must match item count")
expected_status_counts = {
    "upToDate": sum(1 for item in items if item["status"] == "up-to-date"),
    "changedInLiveSettings": sum(1 for item in items if item["status"] == "changed-in-live-settings"),
    "changedInStoredSettings": sum(1 for item in items if item["status"] == "changed-in-stored-settings"),
    "needsChoice": sum(1 for item in items if item["status"] == "conflict"),
    "cannotSyncNow": sum(1 for item in items if item["status"] in {"missing-in-live-settings", "unsupported"}),
}
for key, value in expected_status_counts.items():
    if summary.get(key) != value:
        raise SystemExit(f"{path}: {key} mismatch")
compare_text_summary("01-many-app-status.status", status, ["Up to date", "Changed in live settings", "Changed in stored settings", "Needs choice", "Cannot sync now"])

# Planning fixtures must be planning-only and make plan categories distinct.
for stem in required[1:5]:
    path, data = loaded[stem]
    if data.get("mode") != "planning-only" or data.get("noWritePerformed") is not True:
        raise SystemExit(f"{path}: plan fixture must be planning-only with noWritePerformed=true")
    for item in data["items"]:
        check_write_item(path, item)
        if "planCategory" not in item:
            raise SystemExit(f"{path}: plan item missing planCategory: {item['settingRef']}")
        expected = {"write": "will-sync", "needs_choice": "needs-choice", "blocked": "cannot-sync-now"}.get(item["decision"])
        if expected and item["planCategory"] != expected:
            raise SystemExit(f"{path}: planCategory mismatch for {item['settingRef']}")
    summary = data["summary"]
    if summary.get("willSync") != sum(1 for item in data["items"] if item["decision"] == "write"):
        raise SystemExit(f"{path}: willSync mismatch")
    if summary.get("needsChoice", 0) != sum(1 for item in data["items"] if item["decision"] == "needs_choice"):
        raise SystemExit(f"{path}: needsChoice mismatch")
    if summary.get("cannotSyncNow", 0) != sum(1 for item in data["items"] if item["decision"] == "blocked"):
        raise SystemExit(f"{path}: cannotSyncNow mismatch")
    if summary.get("outOfScopeWrites") != 0:
        raise SystemExit(f"{path}: outOfScopeWrites must be zero")

compare_text_summary("02-many-app-planned-sync.plan", loaded["02-many-app-planned-sync.plan"][1], ["Will sync", "Needs choice", "Cannot sync now", "Writes to stored settings", "Writes to live settings"])
compare_text_summary("03-partial-by-app.plan", loaded["03-partial-by-app.plan"][1], ["Will sync", "Needs choice", "Cannot sync now", "Out-of-scope writes"])
compare_text_summary("04-partial-by-setting-area.plan", loaded["04-partial-by-setting-area.plan"][1], ["Will sync", "Needs choice", "Cannot sync now", "Out-of-scope writes"])
compare_text_summary("05-partial-by-scalar-setting.plan", loaded["05-partial-by-scalar-setting.plan"][1], ["Will sync", "Needs choice", "Cannot sync now", "Out-of-scope writes"])

# Partial scope invariants.
path, data = loaded["03-partial-by-app.plan"]
if data["selection"].get("kind") != "app" or {item["targetRef"] for item in data["items"]} != {data["selection"].get("targetRef")}:
    raise SystemExit(f"{path}: app selection includes out-of-scope items")

path, data = loaded["04-partial-by-setting-area.plan"]
sel_area = data["selection"].get("settingAreaRef")
if data["selection"].get("kind") != "setting-area" or not sel_area:
    raise SystemExit(f"{path}: expected setting-area selection")
if any(item.get("settingAreaRef") != sel_area for item in data["items"]):
    raise SystemExit(f"{path}: setting-area selection includes out-of-scope items")

path, data = loaded["05-partial-by-scalar-setting.plan"]
sel_setting = data["selection"].get("settingRef")
if data["selection"].get("kind") != "setting" or not sel_setting:
    raise SystemExit(f"{path}: expected scalar setting selection")
if [item["settingRef"] for item in data["items"]] != [sel_setting]:
    raise SystemExit(f"{path}: scalar setting selection includes out-of-scope items")

# Mixed execution result proves skipped unsafe items do not block safe writes, while preserving distinct causes.
path, data = loaded["06-mixed-sync-result.execution"]
summary = data["summary"]
items = data["items"]
for item in items:
    check_write_item(path, item)
changed = [item for item in items if item.get("result") == "changed"]
skipped = [item for item in items if item.get("result") == "skipped"]
if data.get("schema") != "dotfiles-manager.sync-execution.v2":
    raise SystemExit(f"{path}: execution fixture must use public sync execution schema")
if summary.get("changed") != len(changed) or summary.get("skipped") != len(skipped):
    raise SystemExit(f"{path}: execution summary mismatch")
if {item["direction"] for item in changed} != {"live_to_stored", "stored_to_live"}:
    raise SystemExit(f"{path}: changed items must cover both directions")
if {item["state"] for item in skipped} != {"conflict", "missing-in-live-settings", "unsupported"}:
    raise SystemExit(f"{path}: mixed skipped states must remain conflict/missing/unsupported")
if {item.get("skipCategory") for item in skipped} != {"needs-choice", "cannot-sync-now"}:
    raise SystemExit(f"{path}: skipped items must preserve needs-choice/cannot-sync-now categories")
if summary.get("needsChoice") != sum(1 for item in skipped if item.get("skipCategory") == "needs-choice"):
    raise SystemExit(f"{path}: needsChoice mismatch")
if summary.get("cannotSyncNow") != sum(1 for item in skipped if item.get("skipCategory") == "cannot-sync-now"):
    raise SystemExit(f"{path}: cannotSyncNow mismatch")
if summary.get("failed") != 0 or summary.get("notAttempted") != 0:
    raise SystemExit(f"{path}: skipped unsafe items must not imply failure or not-attempted writes")
compare_text_summary("06-mixed-sync-result.execution", data, ["Changed", "Needs choice", "Cannot sync now", "Failed", "Not attempted", "Writes to stored settings", "Writes to live settings"])

print("v2 partial/many-app sync fixtures ok")
PY
