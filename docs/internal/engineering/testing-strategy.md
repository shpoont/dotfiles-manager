---
owner: QA / Testing Lead
status: Implementation-ready
last-updated: 2026-02-16
canonical-source: docs/internal/engineering/testing-strategy.md
---

# Testing strategy

This document defines the test structure that validates the specification.

## Test layers

1. **Unit tests**
  - config schema + defaults
  - config source resolution precedence (CLI > env > cwd default)
  - path normalization and `[path]` selection
  - pattern include/exclude logic
  - operation planning and ordering

2. **Integration tests**
   - real filesystem scenarios for deploy/import/status
   - overlapping sync behavior (config order; later sync wins)
   - metadata behavior by contract

3. **Contract tests**
   - JSON envelope and payload schemas
   - stable error codes and deterministic validation order
   - logging contract conformance (channel separation/redaction)

4. **Acceptance tests**
  - full checklist execution (`acceptance-checklist.md`)
  - matrix scenario coverage (`../specs/decision-matrix.md`)

5. **Performance regression tests**
  - dotfiles-sized fixture (~1,000 files)
  - enforce baseline timing thresholds from `technical-requirements.md`
  - fixed hard-threshold pass/fail (no variance window)

## Execution environments

- **Linux test lane**: run inside Docker containers only.
- **macOS test lane**: run on native macOS runners.
- Both lanes must enforce filesystem sandbox guards:
  - all test writes/reads scoped to test-owned temp roots (`t.TempDir()`-style)
  - tests fail if code under test attempts writes outside sandbox roots.

Rationale: Linux containerization gives strong isolation, while native macOS runners are required to validate macOS-specific filesystem behavior.

## Coverage mapping

- Decision-level coverage source: `../specs/decisions.md`
- Scenario-level coverage source: `../specs/decision-matrix.md`
- Exit/error coverage source: `../contracts/validation-errors.md`

## Fixture layout convention

Use repository-root `testdata/` with this structure:

```text
testdata/
  fixtures/
    <scenario>/
      source/
      target/
      .dotfiles-manager.yaml
  expected/
    status/
    deploy/
    import/
    errors/
  logs/
    redaction/
```

Recommended baseline scenarios:
- `minimal`
- `nvim-basic`
- `overlap-syncs`
- `perf-1k`

## Test helper modules

Use internal helper packages to keep tests consistent:

```text
internal/testkit/
  sandbox/   # temp-root guards
  fixtures/  # fixture loading/copy helpers
  cli/       # command runner + stdout/stderr/exit capture
  asserts/   # JSON/log/filesystem assertions
```

## Tooling and thresholds

- Test framework: Go `testing` + `testify`
- Static checks in test pipelines: `go vet`, `staticcheck`, `golangci-lint`
- Coverage gates (required):
  - line: 90%
  - branch: 85%
- Logging-critical branch coverage gate (required):
  - branch: 100% for logging-critical modules/paths (`../contracts/logging-contract.md`)

## Logging-focused required tests

- redaction/masking paths: full branch coverage
- error logging branches (including `DFM_*` codes): full branch coverage
- per-command integration assertions (`status`/`deploy`/`import`):
  - default logging format is human-readable text
  - `--log-format json` emits valid JSON Lines on stderr
  - default logging level is `info`
  - `--log-level` accepts `debug|info|warn|error` and rejects invalid values
  - logs emitted to stderr
  - stdout contract remains valid (including `--json`)
  - no sensitive values in emitted logs

## CI sharding strategy

CI sharding is required for PR and `main` pipelines.

### Shard topology

- Linux uses Docker-only shard jobs:
  1. `linux-unit`
  2. `linux-integration`
  3. `linux-contract`
  4. `linux-performance`
- macOS uses native runner jobs with sandbox guards:
  - `macos-integration` (integration + contract subset required for macOS semantics)

### Shard responsibilities

- `linux-unit`
  - config/schema/defaults
  - path normalization and scope selection
  - include/exclude pattern logic
  - operation planning logic
- `linux-integration`
  - filesystem integration scenarios across deploy/import/status
  - overlap behavior and ordering
  - dry-run no-write guarantees
- `linux-contract`
  - JSON contract tests
  - validation error ordering/codes
  - logging contract checks (stderr channel, format/level behavior, redaction)
  - emits `artifacts/branch-metrics.json` (`branch`, `logging_branch`) for coverage gating
- `linux-performance`
  - dotfiles-sized fixture (~1,000 files)
  - emits `artifacts/perf-metrics.json` and enforces hard thresholds:
  - fixed hard thresholds:
    - `status < 2s`
    - `deploy --dry-run`, `import --dry-run < 3s`
    - `deploy`, `import < 5s` (best-effort; disk-dependent)
- `macos-integration`
  - macOS-specific filesystem/metadata behavior
  - sandbox guard conformance

### Coverage aggregation and gates

- `linux-unit`, `linux-integration`, and `linux-contract` each emit coverage profiles.
- A coverage aggregation job merges shard profiles and is the only source of CI coverage gates.
- This job is the `coverage-aggregation` gate in `ci-cd.md` and runs only after `linux-unit`, `linux-integration`, and `linux-contract` succeed.
- Branch metrics (`branch`, `logging_branch`) are sourced from `linux-contract` artifact `artifacts/branch-metrics.json`.
- Coverage pass criteria:
  - line >= 90%
  - branch >= 85%
  - logging-critical branch == 100% (as defined by `../contracts/logging-contract.md`)
- Coverage artifacts from shard and merged outputs are retained for CI debugging.

Artifact schemas and required keys are defined in `../contracts/ci-artifacts-contract.md`.
