---
owner: Engineering Operations + Core Engineering
status: Contract v2
last-updated: 2026-02-21
canonical-source: docs/internal/contracts/ci-artifacts-contract.md
---

# CI artifacts contract

This document defines required CI artifact files consumed by workflow gates.

Applies when `go.mod` exists and CI is not in bootstrap-skip mode.

## 1) Branch coverage metrics artifact

Produced by: `linux-contract` shard  
Path: `artifacts/branch-metrics.json`  
Consumed by: `coverage-aggregation`

Implementation note:
- produced from `artifacts/coverage-contract.out` via `scripts/ci/write-branch-metrics.sh`

Required JSON shape:

```json
{
  "branch": 87.5,
  "logging_branch": 100.0
}
```

Rules:
- `branch`: numeric percentage in range `[0,100]`
- `logging_branch`: numeric percentage in range `[0,100]`
- coverage gate thresholds:
  - `branch >= 85`
  - `logging_branch >= 100`
- missing file or missing required keys fails CI.

## 2) Performance metrics artifact

Produced by: `linux-performance` shard  
Path: `artifacts/perf-metrics.json`  
Consumed by: `scripts/ci/assert-performance.sh` inside the performance shard

Required JSON shape:

```json
{
  "status_seconds": 1.25,
  "diff_seconds": 1.40,
  "deploy_dry_run_seconds": 2.10,
  "import_dry_run_seconds": 2.35,
  "deploy_seconds": 3.40,
  "import_seconds": 3.85
}
```

Rules:
- values are numeric durations in seconds
- thresholds (hard pass/fail):
  - `status_seconds <= 2.0`
  - `diff_seconds <= 2.0`
  - `deploy_dry_run_seconds <= 3.0`
  - `import_dry_run_seconds <= 3.0`
  - `deploy_seconds <= 5.0`
  - `import_seconds <= 5.0`
- missing file, missing keys, or threshold violation fails CI.

## 3) Compatibility notes

- Producers may add extra fields.
- Gate consumers only require keys defined in this contract.
- Any breaking key rename/removal requires contract version bump + workflow/script update in same change.
