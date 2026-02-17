---
owner: Engineering Operations
status: Implementation-ready
last-updated: 2026-02-17
canonical-source: docs/internal/engineering/ci-cd.md
---

# CI/CD requirements

## 1) CI platform and branch model

- CI platform: **GitHub Actions**
- Branching model: **trunk-based** (short-lived feature branches -> PR -> `main`)
- CI triggers:
  - pull requests
  - pushes to `main`
  - release tags (`vX.Y.Z`)

## 2) Required PR pipeline gates

All must pass before merge:

1. **Static/quality checks**
   - formatting checks
   - `go vet`
   - `staticcheck`
   - `golangci-lint`

2. **Test matrix**
   - Linux shard jobs (Docker-only):
     - `linux-unit`
     - `linux-integration`
     - `linux-contract`
     - `linux-performance`
   - macOS native lane:
     - `macos-integration` (integration + contract subset required for macOS behavior)
   - Shard definitions and scope are canonical in `testing-strategy.md`.

3. **Contract checks**
   - JSON contract tests
   - validation/error code tests
   - logging contract tests (file destination behavior, stderr diagnostics, redaction)

4. **Performance regression checks**
   - run dotfiles-sized fixture benchmark tests in `linux-performance`
   - enforce fixed hard thresholds from `technical-requirements.md`

5. **Coverage gates**
   - `coverage-aggregation` runs after `linux-unit`, `linux-integration`, and `linux-contract`, merges profiles, and is the only coverage gate job
   - aggregate coverage from `linux-unit`, `linux-integration`, and `linux-contract`
   - line >= 90%
   - branch >= 85%
   - logging-critical branch coverage == 100%

### CI job dependency model

Required merge-gate path:
1. static/quality checks
2. Linux shard matrix + macOS lane
3. coverage aggregation gate
4. final required-check pass

Rules:
- any failed required job blocks merge
- coverage aggregation does not run until all required shard jobs succeed
- coverage and performance gates are mandatory, not informational

## 3) Release pipeline

- Versioning: **Semantic Versioning** (`vX.Y.Z`)
- Release tooling: **GoReleaser**
- Release workflow preflight gate (tag build):
  - static checks
  - linux unit/integration/contract/performance shards
  - coverage aggregation thresholds
- Artifacts:
  - macOS arm64
  - macOS amd64
  - Linux arm64
  - Linux amd64
  - checksums file

## 4) Approval and publish policy

- Any maintainer can trigger a release workflow.
- Publishing requires at least **1 code-owner approval**.

## 5) Rollback policy

If a release is bad:
1. Mark release as deprecated.
2. Publish patched superseding version (`vX.Y.Z+1` patch bump).
3. Do not delete published tags/artifacts; supersede instead.

## 6) Ownership

- CI ownership: Engineering Operations
- Release approval owner(s): code owners

## 7) Workflow layout in repository

Implemented workflow files:
- `.github/workflows/ci.yml`
- `.github/workflows/release.yml`

CI helper scripts:
- `scripts/ci/check-go-module.sh`
- `scripts/ci/install-linters.sh`
- `scripts/ci/run-static-checks.sh`
- `scripts/ci/run-tests.sh`
- `scripts/ci/write-branch-metrics.sh`
- `scripts/ci/docker-shard.sh`
- `scripts/ci/assert-performance.sh`
- `scripts/ci/coverage-aggregate.sh`
- `scripts/ci/final-required-check.sh`
- `scripts/ci/guard-release.sh`

Bootstrap behavior:
- if `go.mod` is absent, CI jobs run and skip Go-specific execution paths without failing
- coverage-aggregation job becomes a no-op in bootstrap mode (completes without enforcing coverage thresholds)
- release workflow skips GoReleaser unless both `go.mod` and `.goreleaser.yml` are present

CI artifact contracts (when `go.mod` exists):
- `linux-unit` -> `artifacts/coverage-unit.out`
- `linux-integration` -> `artifacts/coverage-integration.out`
- `linux-contract` -> `artifacts/coverage-contract.out` and `artifacts/branch-metrics.json`
- `linux-performance` -> `artifacts/perf-metrics.json` (used for threshold enforcement in-shard)

Canonical artifact schemas and gate semantics: `../contracts/ci-artifacts-contract.md`.

## 8) Post-release verification

After a release is published, run smoke checks from release artifacts (not local source build).

Manual procedure:

1. Download release artifact + checksums.
2. Verify artifact checksum.
3. Run binary in isolated temp repo/temp HOME:
   - `dotfiles-manager --help`
   - `dotfiles-manager --version`
   - `dotfiles-manager status`
   - `dotfiles-manager deploy --dry-run`
   - `dotfiles-manager import --dry-run`

### Latest verification log

- Date: 2026-02-17
- Release: `v0.1.1`
- Result: PASS
- Verified assets:
  - `dotfiles-manager_0.1.1_darwin_arm64.tar.gz`
  - `dotfiles-manager_0.1.1_checksums.txt`
- Command smoke-test results:
  - `status` output included sync header (`sync[0] target=... source=...`), only non-empty phase blocks, and summary with only non-zero categories
  - for scoped runs, sync header includes `scope=<prefix>`
  - `deploy --dry-run` output included only non-empty phase blocks (`copy[...]`, `remove-unmanaged[...]`) and concise summary
  - `import --dry-run` output included only non-empty phase blocks (`update-managed[...]`, `add-unmanaged[...]`, `remove-missing[...]`) and concise summary
