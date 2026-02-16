#!/usr/bin/env bash
set -euo pipefail

mkdir -p artifacts

echo "Running performance tests..."

# Performance tests are expected to both execute threshold-sensitive scenarios
# and write artifacts/perf-metrics.json with measured timings in seconds.
go test -tags=performance ./... -count=1 -run '^TestPerformance|TestPerf|TestBenchmarkThresholds$'

if [[ ! -f artifacts/perf-metrics.json ]]; then
  echo "Missing artifacts/perf-metrics.json." >&2
  echo "Performance shard requires metrics JSON with keys:" >&2
  echo "  status_seconds, deploy_dry_run_seconds, import_dry_run_seconds, deploy_seconds, import_seconds" >&2
  exit 1
fi

PYTHON_BIN="${PYTHON_BIN:-python3}"
if ! command -v "$PYTHON_BIN" >/dev/null 2>&1; then
  PYTHON_BIN="python"
fi

"$PYTHON_BIN" - <<'PY'
import json
import sys
from pathlib import Path

path = Path("artifacts/perf-metrics.json")
data = json.loads(path.read_text())

required = {
    "status_seconds": 2.0,
    "deploy_dry_run_seconds": 3.0,
    "import_dry_run_seconds": 3.0,
    "deploy_seconds": 5.0,
    "import_seconds": 5.0,
}

missing = [k for k in required if k not in data]
if missing:
    print(f"perf-metrics missing keys: {', '.join(missing)}", file=sys.stderr)
    sys.exit(1)

for key, limit in required.items():
    value = float(data[key])
    if value > limit:
        print(f"performance gate failed: {key}={value:.3f}s > {limit:.3f}s", file=sys.stderr)
        sys.exit(1)

print("Performance gates passed.")
for key in required:
    print(f"- {key}: {float(data[key]):.3f}s")
PY
