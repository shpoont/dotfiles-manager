#!/usr/bin/env bash
set -euo pipefail

if (($# < 4)); then
  echo "usage: coverage-aggregate.sh <unit.cov> <integration.cov> <contract.cov> <branch-metrics.json>" >&2
  exit 2
fi

mkdir -p artifacts

MERGED="artifacts/coverage-merged.out"
SUMMARY="artifacts/coverage-summary.txt"
METRICS_JSON="artifacts/coverage-metrics.json"
BRANCH_METRICS_FILE="$4"

COVERAGE_FILES=("$1" "$2" "$3")
for file in "${COVERAGE_FILES[@]}"; do
  if [[ ! -f "$file" ]]; then
    echo "Missing coverage file: $file" >&2
    exit 1
  fi
done

cp "${COVERAGE_FILES[0]}" "$MERGED"
for file in "${COVERAGE_FILES[@]:1}"; do
  tail -n +2 "$file" >> "$MERGED"
done

line_coverage="$(go tool cover -func="$MERGED" | awk '/^total:/ {gsub("%", "", $3); print $3}')"
if [[ -z "$line_coverage" ]]; then
  echo "Unable to compute total line coverage from $MERGED" >&2
  exit 1
fi

if [[ ! -f "$BRANCH_METRICS_FILE" ]]; then
  echo "Missing branch metrics file: $BRANCH_METRICS_FILE" >&2
  exit 1
fi

PYTHON_BIN="${PYTHON_BIN:-python3}"
if ! command -v "$PYTHON_BIN" >/dev/null 2>&1; then
  PYTHON_BIN="python"
fi

BRANCH_METRICS_FILE="$BRANCH_METRICS_FILE" "$PYTHON_BIN" - <<'PY'
import json
import os
from pathlib import Path

metrics_path = Path(os.environ["BRANCH_METRICS_FILE"])
metrics = json.loads(metrics_path.read_text())
for key in ("branch", "logging_branch"):
    if key not in metrics:
        raise SystemExit(f"branch metrics missing key: {key}")
Path("artifacts/branch_coverage.value").write_text(str(float(metrics["branch"])))
Path("artifacts/logging_branch_coverage.value").write_text(str(float(metrics["logging_branch"])))
PY

branch_coverage="$(cat artifacts/branch_coverage.value)"
logging_branch_coverage="$(cat artifacts/logging_branch_coverage.value)"

LINE_THRESHOLD=90
BRANCH_THRESHOLD=85
LOGGING_BRANCH_THRESHOLD=100

check_threshold() {
  local value="$1"
  local threshold="$2"
  local label="$3"

  if ! awk -v v="$value" -v t="$threshold" 'BEGIN { exit !(v >= t) }'; then
    echo "$label coverage gate failed: ${value}% < ${threshold}%" >&2
    exit 1
  fi
}

check_threshold "$line_coverage" "$LINE_THRESHOLD" "line"
check_threshold "$branch_coverage" "$BRANCH_THRESHOLD" "branch"
check_threshold "$logging_branch_coverage" "$LOGGING_BRANCH_THRESHOLD" "logging-critical branch"

cat > "$SUMMARY" <<EOF
Coverage gates passed.
- line: ${line_coverage}% (threshold: ${LINE_THRESHOLD}%)
- branch: ${branch_coverage}% (threshold: ${BRANCH_THRESHOLD}%)
- logging-critical branch: ${logging_branch_coverage}% (threshold: ${LOGGING_BRANCH_THRESHOLD}%)
EOF

cat > "$METRICS_JSON" <<EOF
{
  "line": ${line_coverage},
  "branch": ${branch_coverage},
  "logging_branch": ${logging_branch_coverage},
  "thresholds": {
    "line": ${LINE_THRESHOLD},
    "branch": ${BRANCH_THRESHOLD},
    "logging_branch": ${LOGGING_BRANCH_THRESHOLD}
  }
}
EOF

if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
  {
    echo "## Coverage gate summary"
    echo
    cat "$SUMMARY"
  } >> "$GITHUB_STEP_SUMMARY"
fi

echo "Coverage aggregation complete."
cat "$SUMMARY"
