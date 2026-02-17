---
owner: QA / Testing Lead (TBD)
status: Implementation-ready
last-updated: 2026-02-17
canonical-source: docs/internal/engineering/acceptance-checklist.md
---

# dotfiles-manager: acceptance checklist (implementation readiness)

Use this checklist before calling implementation complete.

## A) Config loading and schema

- [ ] `--config` loads config successfully.
- [ ] `DOTFILES_MANAGER_CONFIG` loads config when `--config` is omitted.
- [ ] `./.dotfiles-manager.yaml` in cwd loads when `--config` and env var are absent.
- [ ] config precedence is respected: CLI > env > cwd default.
- [ ] missing all config sources yields `DFM_CONFIG_REQUIRED`.
- [ ] invalid YAML yields `DFM_CONFIG_PARSE`.
- [ ] unknown key yields `DFM_CONFIG_SCHEMA_UNKNOWN_KEY`.
- [ ] absolute `source`/`target` rejected (`DFM_CONFIG_PATH_NOT_RELATIVE`).
- [ ] escaping path via `..` rejected (`DFM_CONFIG_PATH_ESCAPE`).

## B) `[path]` matching and scope

- [ ] `[path]` equal to target selects sync.
- [ ] `[path]` inside target selects sync with scoped subtree.
- [ ] parent-of-target does not match.
- [ ] unrelated path does not match.
- [ ] no matches returns `DFM_SCOPE_NO_MATCH`.
- [ ] overlapping matching syncs are all processed in config order.

## C) Status behavior

- [ ] reports deploy drift.
- [ ] reports import drift.
- [ ] reports incoming unmanaged candidates.
- [ ] reports removable unmanaged candidates.
- [ ] reports removable missing-manifest candidates.
- [ ] drift still exits `0`.

### C1) Matrix rows from `../specs/decision-matrix.md`

- [ ] `S exists`, `T missing`, `m+` -> status includes missing-in-target + removable-missing.
- [ ] `S exists`, `T missing`, `m-` -> status includes missing-in-target only.
- [ ] `S exists`, `T exists`, differs -> status includes changed.
- [ ] `S missing`, `T exists`, `u+`,`r+` -> incoming-unmanaged + removable-unmanaged.
- [ ] `S missing`, `T exists`, `u+`,`r-` -> incoming-unmanaged only.
- [ ] `S missing`, `T exists`, `u-`,`r+` -> extra-in-target + removable-unmanaged.
- [ ] `S missing`, `T exists`, `u-`,`r-` -> extra-in-target only.
- [ ] both missing -> no-op.

## D) Deploy behavior

- [ ] updates/copies only changed manifest paths.
- [ ] type mismatch replaced to source type.
- [ ] with empty remove patterns, unmanaged paths are not removed.
- [ ] with remove patterns, unmanaged matching paths are removed.
- [ ] operation order is copy/update first, remove second.
- [ ] `[path]` limits both copy and removal to scoped subtree.
- [ ] `deploy --dry-run` performs no writes and reports planned operations.

## E) Import behavior

- [ ] base import updates manifest paths only.
- [ ] unmanaged add candidates follow `add-unmanaged` include then exclude.
- [ ] empty `add-unmanaged.include` list imports no unmanaged files.
- [ ] remove-missing candidates follow `remove-missing` include/exclude.
- [ ] without `remove-missing.include` patterns, missing target paths do not delete source.
- [ ] type mismatch replaced to target type.
- [ ] `import --dry-run` performs no writes and reports planned operations.

## F) Metadata behavior

- [ ] mode bits + mtime preserved where supported.
- [ ] unsupported xattr/ACL/atime capabilities do not fail.
- [ ] supported metadata apply failures fail command (`DFM_METADATA_APPLY`).

## G) JSON contract

- [ ] `status --json`, `deploy [--dry-run] --json`, `import [--dry-run] --json` conform to `../contracts/json-contract.md`.
- [ ] output includes common envelope fields.
- [ ] per-sync `operations[]` entries are deterministic and path-sorted within emitted phase order.
- [ ] on error, JSON includes `ok=false` + stable `error.code`.
- [ ] on error after partial work, `summary.partial=true` is present.
- [ ] when dry-run is used, JSON has `dry_run=true`.

## H) Exit and runtime behavior

- [ ] success exits `0`.
- [ ] any validation/runtime error exits `1`.
- [ ] runtime failures are fail-fast.
- [ ] `status --dry-run` fails with `DFM_FLAG_UNSUPPORTED`.

## I) Logging and observability

- [ ] logs are written to platform-default log file path.
- [ ] missing log directory is created automatically.
- [ ] `--log-file` overrides default log path.
- [ ] failure to open/write log file fails command.
- [ ] default logging level is `info`.
- [ ] `--log-level` accepts `debug|info|warn|error`.
- [ ] warning/error diagnostics are emitted on stderr as human-readable text.
- [ ] `--json` stdout output is never polluted by logs.
- [ ] invalid `--log-level` values fail with `DFM_FLAG_INVALID_VALUE`.
- [ ] logging-critical modules/paths meet 100% branch coverage.
- [ ] redaction/masking branches have full branch coverage.
- [ ] error logging branches have full branch coverage.
- [ ] command integration tests confirm no sensitive values are emitted in logs.

## J) Performance regression baseline

- [ ] fixture suite includes a dotfiles-sized tree (~1,000 files).
- [ ] thresholds are enforced as fixed hard pass/fail limits (no tolerance window).
- [ ] `status` completes under 2s on baseline CI runner profile.
- [ ] `deploy --dry-run` and `import --dry-run` complete under 3s.
- [ ] `deploy` and `import` complete under 5s (best-effort, disk-dependent).

## K) Fixture and test harness conventions

- [ ] repository uses `testdata/fixtures/<scenario>/{source,target,.dotfiles-manager.yaml}` convention.
- [ ] expected outputs are organized under `testdata/expected/{status,deploy,import,errors}`.
- [ ] logging/redaction fixtures are organized under `testdata/logs/redaction`.
- [ ] test helper modules follow `internal/testkit/{sandbox,fixtures,cli,asserts}` convention.

## L) CI sharding and gates

- [ ] Linux CI runs shard jobs: `linux-unit`, `linux-integration`, `linux-contract`, `linux-performance`.
- [ ] Linux shard jobs run only inside Docker containers.
- [ ] macOS CI runs native `macos-integration` lane with sandbox guards.
- [ ] unit/integration/contract shards emit coverage profiles.
- [ ] contract shard emits `artifacts/branch-metrics.json` with `branch` and `logging_branch` values.
- [ ] performance shard emits `artifacts/perf-metrics.json` and enforces hard thresholds in-shard.
- [ ] CI artifacts conform to `../contracts/ci-artifacts-contract.md`.
- [ ] coverage aggregation job merges shard profiles and enforces:
  - [ ] line >= 90%
  - [ ] branch >= 85%
  - [ ] logging-critical branch == 100%
- [ ] performance shard enforces fixed hard thresholds from `technical-requirements.md`.
- [ ] any required shard/gate failure blocks merge.
