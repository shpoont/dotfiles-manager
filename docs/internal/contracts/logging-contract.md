---
owner: Core Engineering
status: Contract v2
last-updated: 2026-02-17
canonical-source: docs/internal/contracts/logging-contract.md
---

# dotfiles-manager: logging contract

This document defines logging requirements for v2.

## 1) Scope

This contract covers internal runtime logging behavior for:
- `status`
- `deploy`
- `import`

It does not replace user-facing command output documentation.

## 2) Output channels and destinations

- Command output remains on **stdout**.
- Warnings and errors are emitted as human-readable diagnostics on **stderr**.
- Runtime logs are written to a **log file** (not stdout/stderr stream logs).
- For `--json`, stdout must remain JSON-only output.

## 3) Default log file paths

- macOS:
  - `~/Library/Logs/dotfiles-manager/dotfiles-manager.log`
- Linux:
  - `${XDG_STATE_HOME:-~/.local/state}/dotfiles-manager/dotfiles-manager.log`

Rules:
- Missing parent directories must be created automatically.
- Log file writes are append-only.
- If the log file cannot be opened/written, the command fails (non-zero).

## 4) CLI controls

- Global CLI option: `--log-file <path>`
  - overrides default platform log file path
- Global CLI option: `--log-level <debug|info|warn|error>`
  - default: `info`
  - invalid values must fail with `DFM_FLAG_INVALID_VALUE`

No log format option is supported; logs are human-readable text only.

## 5) Minimum log semantics

Log events must include, at minimum:
- level (`debug` | `info` | `warn` | `error`)
- component
- message

When available, include contextual fields:
- command
- `dry_run`
- error code (`DFM_*`) for error events

## 6) Redaction and safety

- Logs must not expose secret/sensitive values.
- Any sensitive values that reach logging paths must be redacted before emission.
- Error diagnostics should include actionable context without leaking sensitive content.

## 7) Coverage requirement (logging-critical)

Logging-critical code must have **100% branch coverage**.

Logging-critical includes:
- log file writer initialization/error branches
- redaction/masking helpers
- error logging branches
- level/field gating branches in logging emitters

This is an additional requirement on top of global project coverage thresholds.

## 8) Verification sources

- Test strategy: `../engineering/testing-strategy.md`
- Acceptance checks: `../engineering/acceptance-checklist.md`
- Error codes: `validation-errors.md`
