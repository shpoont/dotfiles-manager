---
owner: Core Engineering
status: Contract v1
last-updated: 2026-02-16
canonical-source: docs/internal/contracts/logging-contract.md
---

# dotfiles-manager: logging contract

This document defines logging requirements for v1.

## 1) Scope

This contract covers internal runtime logging behavior for:
- `status`
- `deploy`
- `import`

It does not replace user-facing command output documentation.

## 2) Output channel contract

- Logs must be emitted to **stderr**.
- Human/JSON command outputs remain on **stdout**.
- For `--json`, stdout must remain valid JSON-only output.

## 3) Log format contract

- Global CLI option: `--log-format <text|json>`
- Default log format: `text`
- `--log-format json` emits one JSON object per line to stderr (JSON Lines)
- Invalid format values must fail with error code `DFM_FLAG_INVALID_VALUE`

## 4) Log level contract

- Global CLI option: `--log-level <debug|info|warn|error>`
- Default log level: `info`
- Log level applies to all commands (`status`, `deploy`, `import`)
- Invalid level values must fail with error code `DFM_FLAG_INVALID_VALUE`

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
- Error messages should contain diagnostic context without leaking sensitive content.

## 7) Coverage requirement (logging-critical)

Logging-critical code must have **100% branch coverage**.

Logging-critical includes:
- redaction/masking helpers
- error logging branches
- level/field gating branches in logging emitters

This is an additional requirement on top of global project coverage thresholds.

## 8) Verification sources

- Test strategy: `../engineering/testing-strategy.md`
- Acceptance checks: `../engineering/acceptance-checklist.md`
- Error codes: `validation-errors.md`
