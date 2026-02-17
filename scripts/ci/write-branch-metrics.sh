#!/usr/bin/env bash
set -euo pipefail

PROFILE_PATH="${1:?usage: write-branch-metrics.sh <coverage-profile> [output-json]>}"
OUTPUT_PATH="${2:-artifacts/branch-metrics.json}"

if [[ ! -f "$PROFILE_PATH" ]]; then
  echo "Missing coverage profile: $PROFILE_PATH" >&2
  exit 1
fi

PYTHON_BIN="${PYTHON_BIN:-python3}"
if ! command -v "$PYTHON_BIN" >/dev/null 2>&1; then
  PYTHON_BIN="python"
fi

"$PYTHON_BIN" - "$PROFILE_PATH" "$OUTPUT_PATH" <<'PY'
import json
import subprocess
import sys
from pathlib import Path

profile = Path(sys.argv[1])
out_path = Path(sys.argv[2])

report = subprocess.check_output(["go", "tool", "cover", f"-func={profile}"], text=True)

branch = None
logging_values = []

for raw_line in report.splitlines():
    line = raw_line.strip()
    if not line:
        continue

    parts = line.split()
    if not parts:
        continue

    if parts[0] == "total:":
        branch = float(parts[-1].rstrip("%"))
        continue

    if len(parts) < 3 or not parts[-1].endswith("%"):
        continue

    location = parts[0]
    function_name = parts[1]
    coverage = float(parts[-1].rstrip("%"))

    path = location
    if ":" in location:
        path = location.rsplit(":", 2)[0]

    is_logging_file = "/internal/logging/" in path
    is_cli_error_logger = path.endswith("/internal/app/cli.go") and function_name == "logCommandError"

    if is_logging_file or is_cli_error_logger:
        logging_values.append(coverage)

if branch is None:
    raise SystemExit("unable to parse total coverage from profile report")

if not logging_values:
    raise SystemExit("unable to compute logging_branch: no logging-critical functions found")

payload = {
    "branch": branch,
    "logging_branch": min(logging_values),
}

out_path.parent.mkdir(parents=True, exist_ok=True)
out_path.write_text(json.dumps(payload, indent=2) + "\n")

print(f"Wrote branch metrics to {out_path}")
print(f"- branch: {payload['branch']:.1f}%")
print(f"- logging_branch: {payload['logging_branch']:.1f}%")
PY
