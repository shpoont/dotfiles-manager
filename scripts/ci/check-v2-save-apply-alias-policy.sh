#!/usr/bin/env bash
set -euo pipefail

GO_BIN="${GO:-go}"
PYTHON_BIN="${PYTHON:-python3}"
SPEC="docs/internal/specs/v2/16-save-apply-alias-policy.md"

if [[ ! -f "$SPEC" ]]; then
  echo "missing save/apply alias policy spec: $SPEC" >&2
  exit 1
fi

require_file_contains() {
  local file="$1"
  local needle="$2"
  if ! grep -Fq "$needle" "$file"; then
    echo "missing required text in $file:" >&2
    echo "  $needle" >&2
    exit 1
  fi
}

require_file_not_contains() {
  local file="$1"
  local needle="$2"
  if grep -Fq "$needle" "$file"; then
    echo "forbidden legacy text in $file:" >&2
    echo "  $needle" >&2
    exit 1
  fi
}

require_file_contains "$SPEC" "save  = sync live settings -> stored settings"
require_file_contains "$SPEC" "apply = sync stored settings -> live settings"
require_file_contains "$SPEC" '"operation": "sync"'
require_file_contains "$SPEC" '"invokedCommand": "save"'
require_file_contains "$SPEC" '"direction": "live_to_stored"'
require_file_contains "docs/internal/specs/v2/README.md" "16-save-apply-alias-policy.md"
require_file_contains "docs/internal/specs/v2/00-vocabulary.md" "save  = sync live settings -> stored settings"
require_file_contains "docs/internal/specs/v2/00-vocabulary.md" "apply = sync stored settings -> live settings"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

ROOT_HELP="$($GO_BIN run ./cmd/dotfiles-manager --help)"
SAVE_HELP="$($GO_BIN run ./cmd/dotfiles-manager save --help)"
APPLY_HELP="$($GO_BIN run ./cmd/dotfiles-manager apply --help)"

export ROOT_HELP SAVE_HELP APPLY_HELP
"$PYTHON_BIN" - <<'PY'
import os
import re

root = os.environ["ROOT_HELP"]
save = os.environ["SAVE_HELP"]
apply = os.environ["APPLY_HELP"]

required_root = [
    "Normal v2 workflow:\n  status -> diff -> sync",
    "sync is the primary v2 action",
    "Compatibility aliases:",
    "save  sync live settings -> stored settings",
    "apply sync stored settings -> live settings",
    "The settings folder is local storage. It may be versioned with Git, but Git is\noptional.",
]
for needle in required_root:
    if needle not in root:
        raise SystemExit(f"root help missing required text: {needle!r}")

try:
    commands_block = root.split("Available Commands:\n", 1)[1].split("\nFlags:", 1)[0]
except IndexError as exc:
    raise SystemExit("root help missing Available Commands block") from exc

positions = {}
for command in ("init", "list", "search", "explain", "add", "status", "diff", "sync", "save", "apply"):
    match = re.search(rf"^  {command}\s", commands_block, re.MULTILINE)
    if not match:
        raise SystemExit(f"root help missing Available Commands row for {command}")
    positions[command] = match.start()
if not (positions["init"] < positions["list"] < positions["search"] < positions["explain"] < positions["add"] < positions["status"] < positions["diff"] < positions["sync"] < positions["save"] < positions["apply"]):
    raise SystemExit("root help must list v2 commands as init/list/search/explain/add/status/diff/sync before save/apply")

checks = [
    (save, "save", "live settings to stored settings", "Preview syncing live settings to stored settings without writing"),
    (apply, "apply", "stored settings to live settings", "Preview syncing stored settings to live settings without writing"),
]
for help_text, command, direction_text, dry_run_flag in checks:
    for needle in (
        "Compatibility alias for directional sync.",
        direction_text,
        "For normal use, run status, then diff, then sync.",
        dry_run_flag,
    ):
        if needle not in help_text:
            raise SystemExit(f"{command} help missing required text: {needle!r}")
    forbidden = ["desired", "repository", "should be taught first"]
    lowered = help_text.lower()
    for word in forbidden:
        if word in lowered:
            raise SystemExit(f"{command} help must not use legacy public word {word!r}")
PY

for file in \
  README.md \
  docs/user/README.md \
  docs/user/getting-started.md \
  docs/user/commands.md; do
  require_file_contains "$file" "status -> diff -> sync"
done

for file in README.md docs/user/*.md; do
  require_file_not_contains "$file" "action="
  require_file_not_contains "$file" "would-promote"
  require_file_not_contains "$file" "desired state"
  require_file_not_contains "$file" "save --dry-run ->"
  require_file_not_contains "$file" "save --yes ->"
  require_file_not_contains "$file" "saved desired state"
  require_file_not_contains "$file" "saved desired value"
done

require_file_contains "docs/user/commands.md" "save  = sync live settings -> stored settings"
require_file_contains "docs/user/commands.md" "apply = sync stored settings -> live settings"
require_file_contains 'README.md' '`save` = sync live settings -> stored settings'
require_file_contains 'README.md' '`apply` = sync stored settings -> live settings'
require_file_contains 'docs/user/README.md' '`save` = sync live settings -> stored settings'
require_file_contains 'docs/user/README.md' '`apply` = sync stored settings -> live settings'
require_file_contains "docs/user/getting-started.md" "save = sync live settings -> stored settings"
require_file_contains "docs/user/getting-started.md" "apply = sync stored settings -> live settings"
require_file_not_contains "internal/v2/selectedpreview/selectedpreview.go" "Preview syncing stored settings to live settings"
require_file_not_contains "internal/v2/selectedpreview/selectedpreview.go" "apply --dry-run"
require_file_contains "internal/v2/selectedpreview/selectedpreview.go" "Run sync to use the safe direction after reviewing this diff"

BIN="$TMP_DIR/dotfiles-manager"
JSON_HOME="$TMP_DIR/home"
JSON_WORK="$TMP_DIR/work"
"$GO_BIN" build -o "$BIN" ./cmd/dotfiles-manager
mkdir -p "$JSON_HOME" "$JSON_WORK"

(
  cd "$JSON_WORK"
  HOME="$JSON_HOME" "$BIN" init --yes --machine-id ci-machine --user-id ci-user >/dev/null
  HOME="$JSON_HOME" "$BIN" add git --setting git:user.email --scope user --yes >/dev/null
  cat >"$JSON_HOME/.gitconfig" <<'EOF'
[user]
	email = ci@example.test
EOF

  if HOME="$JSON_HOME" "$BIN" --config dotfiles-manager.v2.yaml save --json --user-id ci-user git:user.email >"$TMP_DIR/save-refusal.json"; then
    echo "save without --yes unexpectedly succeeded in alias JSON fixture" >&2
    exit 1
  fi
  HOME="$JSON_HOME" "$BIN" --config dotfiles-manager.v2.yaml save --dry-run --json --user-id ci-user git:user.email >"$TMP_DIR/save-dry-run.json"
  HOME="$JSON_HOME" "$BIN" --config dotfiles-manager.v2.yaml save --dry-run --user-id ci-user git:user.email >"$TMP_DIR/save-dry-run.txt"
  HOME="$JSON_HOME" "$BIN" --config dotfiles-manager.v2.yaml save --yes --json --user-id ci-user git:user.email >"$TMP_DIR/save-confirmed.json"

  cat >"$JSON_HOME/.gitconfig" <<'EOF'
[user]
	email = changed-ci@example.test
EOF

  HOME="$JSON_HOME" "$BIN" --config dotfiles-manager.v2.yaml apply --dry-run --json --user-id ci-user git:user.email >"$TMP_DIR/apply-dry-run.json"
  HOME="$JSON_HOME" "$BIN" --config dotfiles-manager.v2.yaml apply --dry-run --user-id ci-user git:user.email >"$TMP_DIR/apply-dry-run.txt"
  if HOME="$JSON_HOME" "$BIN" --config dotfiles-manager.v2.yaml apply --non-interactive --json --user-id ci-user git:user.email >"$TMP_DIR/apply-refusal.json"; then
    echo "apply --non-interactive without --yes unexpectedly succeeded in alias JSON fixture" >&2
    exit 1
  fi
  HOME="$JSON_HOME" "$BIN" --config dotfiles-manager.v2.yaml apply --yes --json --user-id ci-user git:user.email >"$TMP_DIR/apply-confirmed.json"
)

"$PYTHON_BIN" - \
  "$TMP_DIR/save-refusal.json|save|live_to_stored|false|error" \
  "$TMP_DIR/save-dry-run.json|save|live_to_stored|true|ok" \
  "$TMP_DIR/save-confirmed.json|save|live_to_stored|false|ok" \
  "$TMP_DIR/apply-dry-run.json|apply|stored_to_live|true|ok" \
  "$TMP_DIR/apply-refusal.json|apply|stored_to_live|false|error" \
  "$TMP_DIR/apply-confirmed.json|apply|stored_to_live|false|ok" <<'PY'
import json
import sys

for raw_case in sys.argv[1:]:
    path, invoked, direction, dry_run, outcome = raw_case.split("|")
    with open(path, "r", encoding="utf-8") as handle:
        payload = json.load(handle)
    expected_dry_run = dry_run == "true"
    if payload.get("schema") != "dotfiles-manager.v2.preview":
        raise SystemExit(f"{path}: unexpected schema {payload.get('schema')!r}")
    if payload.get("operation") != "sync":
        raise SystemExit(f"{path}: operation must be sync")
    if payload.get("command") != invoked:
        raise SystemExit(f"{path}: command must remain the invoked alias {invoked!r}")
    if payload.get("invokedCommand") != invoked:
        raise SystemExit(f"{path}: invokedCommand must be {invoked!r}")
    if payload.get("direction") != direction:
        raise SystemExit(f"{path}: direction must be {direction!r}")
    if payload.get("dryRun") is not expected_dry_run:
        raise SystemExit(f"{path}: dryRun must be {expected_dry_run!r}")
    summary = payload.get("summary") or {}
    if outcome == "error":
        if summary.get("status") != "error":
            raise SystemExit(f"{path}: refusal summary status must be error")
        if not isinstance(payload.get("error"), dict):
            raise SystemExit(f"{path}: refusal must include an error object")
    else:
        if payload.get("error") is not None:
            raise SystemExit(f"{path}: non-refusal case must not include an error")
        if summary.get("status") == "error":
            raise SystemExit(f"{path}: non-refusal summary status must not be error")
PY

require_file_contains "$TMP_DIR/save-dry-run.txt" "Command alias:"
require_file_contains "$TMP_DIR/save-dry-run.txt" "save is a compatibility alias for sync."
require_file_contains "$TMP_DIR/save-dry-run.txt" "Sync direction:"
require_file_contains "$TMP_DIR/save-dry-run.txt" "live settings -> stored settings"
require_file_contains "$TMP_DIR/save-dry-run.txt" "Primary command:"
require_file_contains "$TMP_DIR/save-dry-run.txt" "sync"
require_file_contains "$TMP_DIR/apply-dry-run.txt" "Command alias:"
require_file_contains "$TMP_DIR/apply-dry-run.txt" "apply is a compatibility alias for sync."
require_file_contains "$TMP_DIR/apply-dry-run.txt" "Sync direction:"
require_file_contains "$TMP_DIR/apply-dry-run.txt" "stored settings -> live settings"
require_file_contains "$TMP_DIR/apply-dry-run.txt" "Primary command:"
require_file_contains "$TMP_DIR/apply-dry-run.txt" "sync"

echo "v2 save/apply alias policy ok"
