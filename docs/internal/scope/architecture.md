---
owner: Core Engineering
status: Implementation-ready
last-updated: 2026-02-16
canonical-source: docs/internal/scope/architecture.md
---

# Architecture overview

This document defines the runtime architecture that implements the approved specs/contracts.

## 1) Architectural principles

- **Contract-first**: behavior is driven by `specs/decisions.md` + `contracts/*`.
- **Deterministic**: config order + lexical path ordering everywhere.
- **Fail-fast**: stop at first validation/runtime failure.
- **Channel separation**: reporter owns stdout, logger owns stderr.
- **Portable core**: macOS/Linux now; platform adapter boundary for future Windows.

## 2) Runtime module boundaries

1. **CLI / command layer**
   - parses global flags (`--config`, `--log-format`, `--log-level`) and command flags
   - dispatches `status`, `deploy`, `import`
   - maps unsupported/invalid flags to stable error codes

2. **Config resolver + validator**
   - resolves config source in order:
     1) `--config`
     2) `DOTFILES_MANAGER_CONFIG`
     3) `./.dotfiles-manager.yaml` (cwd only)
   - parses YAML
   - validates schema/unknown keys/required keys/path constraints
   - enforces relative-only config paths + no base escape (`..`)

3. **Scope resolver**
   - normalizes optional CLI `[path]`
   - resolves to matching syncs only when `[path]` is target-or-subpath
   - rejects no-match/invalid-path cases with `DFM_SCOPE_*`
   - computes sub-scope roots used by planner/executor

4. **Planner**
   - compares source vs target trees per matched sync
   - produces deterministic plan objects for:
     - status drift/candidate sets
     - deploy copy/update + remove-unmanaged
     - import managed updates + add-unmanaged + remove-missing
   - applies include/exclude pattern rules (exclude wins)
   - sorts output paths lexically and preserves sync index order

5. **Executor**
   - executes planner output for `deploy` and `import`
   - applies operation order rules:
     - deploy: copy/update first, remove-unmanaged second
     - import: update/add/remove as defined by planned action lists
   - handles type replacement (file/dir/symlink mismatch)
   - applies metadata policy per `../contracts/metadata-contract.md`
   - no writes in dry-run mode

6. **Reporter**
   - converts plan/result to:
     - human text output
     - JSON envelope per `../contracts/json-contract.md`
   - guarantees deterministic ordering and stable field names
   - emits errors in contract format when `--json` is enabled

7. **Logger**
   - emits runtime logs to stderr only
   - supports text/json formats + log levels
   - enforces redaction policy from `../contracts/logging-contract.md`

## 3) Command data flows

### status

`CLI -> config resolver -> scope resolver -> planner -> reporter`

- no executor write phase
- status-only candidate sets are reported

### deploy

`CLI -> config resolver -> scope resolver -> planner -> executor -> reporter`

- planned copy/update and remove-unmanaged operations
- dry-run skips writes but keeps same planning + reporting logic

### import

`CLI -> config resolver -> scope resolver -> planner -> executor -> reporter`

- planned managed updates + optional unmanaged adds + optional missing removes
- dry-run skips writes but keeps same planning + reporting logic

## 4) Determinism and overlap handling

- Syncs are processed in config declaration order.
- Overlapping syncs are allowed; later sync effects win on the same final path.
- Within each sync/action list, paths are sorted lexically before report/execute.
- Validation order is fixed by `../contracts/validation-errors.md`.

## 5) Error handling boundary

- Validation phase errors stop before any mutation.
- Runtime phase errors are fail-fast and stop command execution immediately.
- Exit codes and error codes are defined only by `../contracts/validation-errors.md`.
- Partial execution reporting follows JSON contract (`summary.partial`) when applicable.

## 6) Platform abstraction boundary (future Windows support)

- Core planner/reporter logic remains platform-agnostic and path-normalized.
- Filesystem + metadata operations are isolated behind adapter interfaces.
- v1 adapters target macOS/Linux behavior.
- Windows support will be added by implementing platform adapters without changing command semantics.

## 7) Selected implementation baseline

- Language/runtime: Go 1.22
- CLI framework: Cobra
- YAML parser: `gopkg.in/yaml.v3`
- Glob engine: `doublestar/v4`
- Logging backend: Go `log/slog`
- Supported OS in v1: macOS + Linux (architect for future Windows support)
