---
owner: Core Engineering
status: Canonical
last-updated: 2026-02-17
canonical-source: docs/internal/specs/decisions.md
---

# dotfiles-manager: resolved decisions

This document is the canonical decision log for behavior already agreed.
It complements:
- `decision-matrix.md` (scenario outcomes)
- `open-questions.md` (remaining follow-ups)
- `../contracts/*` (machine/engineering contracts)

## 1) Config and schema

| Decision | Why |
|---|---|
| Config resolution order is strict: `--config <path>` → `DOTFILES_MANAGER_CONFIG` → `./.dotfiles-manager.yaml` in current working directory | Keeps behavior explicit while enabling ergonomic default usage. |
| Default config fallback is current-directory only (no parent search) | Preserves determinism and avoids accidental config pickup. |
| Exactly one config is loaded per run | Avoids ambiguous merges across multiple files. |
| Default filename is `.dotfiles-manager.yaml` | Standardizes fallback discovery behavior. |
| Config format is YAML only | Reduces parser/format complexity. |
| Unknown keys are errors | Prevents silent typos and config drift. |
| Defaults: `on.deploy.remove-unmanaged=[]`, `on.import.add-unmanaged.include=[]`, `on.import.add-unmanaged.exclude=[]`, `on.import.remove-missing.include=[]`, `on.import.remove-missing.exclude=[]` | Safe defaults: no deletes/imports unless explicitly configured. |

## 2) Paths and scope

| Decision | Why |
|---|---|
| `source` and `target` in config are relative-only | Prevents surprising absolute-path writes and keeps configs portable. |
| `source` is relative to config directory; `target` is relative to `$HOME` | Clear, stable roots for both sides. |
| Lexical normalization applies (`.`, `..`, duplicate separators) | Ensures consistent matching and validation. |
| Escaping base roots via `..` is invalid | Prevents traversal outside intended scope. |
| Symlinks are treated as symlink entries (no realpath-based sync semantics) | Matches “treat like git entries” model and avoids hidden path rewrites. |
| CLI `[path]` accepts absolute, `~`-based, and relative forms | Convenient for both shell and scripting usage. |
| `[path]` must equal target root or be inside target subtree | Prevents accidental broad selection from parent paths. |
| If `[path]` matches no syncs: error | Surfaces incorrect scoping immediately. |
| If multiple syncs match: process all | Supports overlap/intended fan-out behavior. |
| Overlapping targets are allowed | Keeps config expressive for nested/specialized syncs. |
| Sync execution order is config declaration order | Provides deterministic behavior for overlaps. |
| Within each sync, path operation order is lexical ascending by normalized sync-relative path | Provides deterministic and testable per-sync behavior. |
| If overlapping syncs mutate the same final path, later sync result wins | Defines deterministic conflict outcome without extra conflict mode. |

## 3) Pattern model

Pattern-based behavior is config-driven only.

| Decision | Why |
|---|---|
| Deploy cleanup key: `on.deploy.remove-unmanaged` | Explicitly scoped to deploy behavior. |
| Import unmanaged-add keys: `on.import.add-unmanaged.include/exclude` | Names import behavior by action and stays explicit. |
| Import remove-missing keys: `on.import.remove-missing.include/exclude` | Names delete-on-missing behavior by action and keeps intent clear. |
| No CLI pattern flags (`--include-unmanaged`, `--exclude-unmanaged`, `--remove-unmanaged`) | Avoids command-line/config precedence confusion. |
| Pattern engine is glob with `**` | Familiar and expressive for file trees. |
| Pattern root is sync-relative path | Same logical path language across source/target sides. |
| Pattern separator is `/` | Cross-platform consistency in config patterns. |
| Case sensitivity is OS-dependent | Aligns with filesystem behavior. |
| Escaping behavior is OS-dependent | Aligns with underlying platform/pattern engine behavior. |
| Include then exclude (exclude wins) | Standard, predictable filtering order. |
| Empty include set means “no candidates” for add-unmanaged/remove-missing include-gated flows | Safe-by-default behavior. |

## 4) Command semantics

### `status [--json] [path]`

Reports:
- deploy drift (`source → target`)
- import drift (`target → source` over manifest)
- incoming unmanaged candidates
- removable unmanaged candidates
- removable missing-manifest candidates

Output semantics:
- status operation wording is potential/human-readable (`can create`, `can update`, `can replace type`, `can add`, `can remove`)
- phase names stay unchanged (`deploy`, `import`, `incoming-unmanaged`, `remove-unmanaged`, `remove-missing`)

JSON format is defined in `../contracts/json-contract.md`.

### `deploy [--dry-run] [--json] [path]`

- Copy/update only when content differs.
- Type mismatches are replaced to match source type.
- If remove patterns are empty/missing, no unmanaged removal occurs.
- With `[path]`, cleanup/removal applies only in the scoped subtree.
- Order is **copy then remove**.
- Preserve metadata as much as the platform supports.
- With `--dry-run`, plan and report actions but do not mutate filesystem.

### `import [--dry-run] [--json] [path]`

- Base scope: manifest paths only.
- Unmanaged add candidates: target-only + add-unmanaged include match + not add-unmanaged exclude.
- Missing-delete candidates: source-only (missing in target) + remove-missing include match + not remove-missing exclude.
- Default (without remove-missing include patterns): do not delete source entries just because target is missing.
- Type mismatches are replaced to match target type.
- Preserve metadata as much as the platform supports.
- With `--dry-run`, plan and report actions but do not mutate filesystem.

Metadata guarantees and best-effort behavior are defined in `../contracts/metadata-contract.md`.

## 5) Source of truth and conflicts

| Decision | Why |
|---|---|
| Source is the manifest and source of truth | Keeps mental model simple and explicit. |
| No separate conflict state in status/output | Without sync history, conflict inference is ambiguous; direction decides overwrite behavior. |

## 6) Runtime behavior

| Decision | Why |
|---|---|
| Commands are fail-fast on runtime errors | Prevents partial hidden failures. |
| Exit `0` on success (including `status` with drift), non-zero on errors | Conventional CLI semantics. |
| `--json` supported on `status`, `deploy`, and `import` | Machine-readable automation support. |
| `--dry-run` supported on `deploy` and `import`, not `status` | Keeps preview explicit for mutating commands; `status` is already preview-only. |
| Text output suppresses empty phase blocks | Reduces noise and surfaces only actionable work. |
| Text summary includes only non-zero categories | Keeps human output concise. |
| JSON `summary` keeps fixed command-specific keys (including zero values) | Preserves stable machine-readable structure. |
| JSON schema version bumped to `3.0` for status action wording change | Captures breaking contract update explicitly. |
| Runtime logs are written to a platform-default log file | Keeps command output channels clean while preserving diagnostics history. |
| `--log-file <path>` overrides the log file destination | Allows explicit per-run log routing. |
| Logs are human-readable text only (no log format option) | Keeps operator-facing diagnostics simple and consistent. |
| Warnings/errors are emitted as human-readable stderr diagnostics | Keeps failures visible in terminal while avoiding noisy routine logs. |
| Log level defaults to `info` and is configurable via `--log-level` | Provides predictable default verbosity with explicit override. |
| Logging backend is Go `log/slog` | Uses standard-library structured logging with low dependency overhead. |
| Logging-critical paths require 100% branch coverage | Ensures redaction and error-path logging safety is continuously verified. |
| Performance is guarded by regression thresholds for dotfiles-sized fixtures (~1,000 files), not by hard end-user SLA | Keeps performance expectations realistic while preventing accidental slowdowns. |
| No locking | Keeps runtime simple; concurrent-run policy can be revisited if needed. |

Validation/error catalog is defined in `../contracts/validation-errors.md`.
Logging contract is defined in `../contracts/logging-contract.md`.
