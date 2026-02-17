---
owner: Engineering Leadership
status: Implementation-ready
last-updated: 2026-02-17
canonical-source: docs/internal/engineering/technical-requirements.md
---

# Technical requirements

This document captures implementation constraints and required engineering standards.

## 1) Runtime and platform

- Supported OS targets (v1): **macOS + Linux**
- Architecture requirement: keep implementation portable for future Windows support
- Filesystem assumptions: POSIX-like semantics required for current contracts
- Path handling baseline: as defined in `../specs/decisions.md`

## 2) Language and framework requirements

- Implementation language: **Go 1.22**
- CLI framework/library: **Cobra**
- Config parsing library: **gopkg.in/yaml.v3**
- Pattern matching library: **github.com/bmatcuk/doublestar/v4**
- Logging backend: **Go standard library `log/slog`**
- Assertions/helpers for tests: **testify**

## 3) Testing and coverage requirements

- Unit tests required for:
  - config parsing/validation
  - path matching/scoping
  - pattern filtering
  - planning logic for status/deploy/import
- Integration tests required for:
  - end-to-end filesystem behavior
  - dry-run behavior (no writes)
  - JSON output contract conformance
- Minimum coverage thresholds (CI required):
  - line coverage: **90%**
  - branch coverage: **85%**
- Logging-critical coverage threshold (CI required):
  - branch coverage: **100%** (see `../contracts/logging-contract.md`)

Execution environment requirements:
- Linux tests run inside Docker containers.
- macOS tests run on native macOS runners with strict filesystem sandboxing.
- Test code must hard-fail on writes outside test-owned temp roots.

CI sharding requirements:
- Linux shards: unit, integration, contract, performance (Docker-only).
- macOS lane: native integration/contract subset for platform semantics.
- Coverage aggregation gates are computed from Linux unit/integration/contract shards and must meet 90% line / 85% branch / 100% logging-critical branch coverage.
- Performance thresholds are enforced as hard pass/fail in the Linux performance shard.
- The `coverage-aggregation` gate behavior is defined in `testing-strategy.md`; PR/main dependency ordering is defined in `ci-cd.md`.
- CI artifact schemas for coverage/performance gates are defined in `../contracts/ci-artifacts-contract.md`.

Reference:
- `testing-strategy.md`
- `acceptance-checklist.md`

Test fixture layout and helper module conventions are defined in `testing-strategy.md`.

## 4) CI/CD requirements

- CI must run on every PR: **required**
- CI platform: **GitHub Actions**
- Required CI stages:
  1) formatting/lint/static checks
  2) sharded test matrix (Linux unit/integration/contract/performance + macOS integration lane)
  3) coverage aggregation gate
  4) performance threshold gate
- Required static tooling:
  - `go vet`
  - `staticcheck`
  - `golangci-lint`
- Merge gate:
  - all CI jobs green
  - coverage thresholds met

Release requirements:
- versioning: **Semantic Versioning (`vX.Y.Z`)**
- release tooling: **GoReleaser**
- artifacts:
  - macOS arm64 / amd64
  - Linux arm64 / amd64
  - checksums file

Reference: `ci-cd.md`

## 5) Performance and reliability targets

- v1 performance regression baseline (dotfiles-sized trees):
  - fixture scale: ~1,000 files
  - `status`: < 2s
  - `deploy --dry-run` / `import --dry-run`: < 3s
  - `deploy` / `import`: < 5s (best-effort; disk-dependent)
- These targets are CI regression guards, not end-user SLA guarantees.
- CI enforcement model: fixed hard-threshold pass/fail (no tolerance window).
- Deterministic output ordering: **required**
- Fail-fast behavior on runtime errors: **required**

## 6) Security and safety

- No path traversal outside declared roots: **required**
- No unintended writes in dry-run mode: **required**
- Error messages must not leak sensitive data: **required**
- Runtime logging must follow `../contracts/logging-contract.md` (file-first logging + stderr diagnostics + redaction).
- Runtime logging destination defaults to platform path and is overridable via `--log-file`.
- Runtime logs are human-readable text only (no log format option).
- Runtime logging default level is `info`; allowed levels are `debug|info|warn|error`.
