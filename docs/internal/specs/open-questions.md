---
owner: Core Engineering
status: Implementation-ready
last-updated: 2026-02-16
canonical-source: docs/internal/specs/open-questions.md
---

# dotfiles-manager: remaining open questions

Spec status: **implementation-ready**.

Most previously open items are now resolved in:
- `cli-and-config-spec.md` (internal reference summary)
- `decision-matrix.md` (scenario outcomes)
- `decisions.md` (decision rationale)

## Remaining items

- None blocking implementation.

## Non-blocking follow-ups (optional)

- [ ] Add explicit duplicate-write detector for overlapping syncs (diagnostic only).

## Explicitly resolved (kept for traceability)

- Config is YAML-only, unknown keys are errors.
- Config resolution order is `--config` → `DOTFILES_MANAGER_CONFIG` → `./.dotfiles-manager.yaml` (cwd).
- Default config lookup is cwd-only (no parent search).
- Config `source`/`target` are relative-only.
- Lexical path normalization; base escape via `..` is invalid.
- `[path]` accepts absolute, `~`, and relative forms.
- `[path]` must be target or subpath; parent-of-target does not match.
- No match for `[path]` is an error; multiple matches are all processed.
- Source is the manifest and source of truth.
- No special conflict state.
- Deploy remove behavior is pattern-list based (`on.deploy.remove-unmanaged`), copy then remove.
- Import unmanaged-add behavior uses `on.import.add-unmanaged.include/exclude`.
- Import remove-missing behavior uses `on.import.remove-missing.include/exclude`.
- Pattern root is sync-relative, using glob + `**`, `/` separator, OS-dependent case/escaping.
- Overlapping syncs execute in config order; later sync result wins on same final path.
- Commands are fail-fast on runtime errors.
- Symlinks are treated as symlink entries.
- `status`, `deploy`, and `import` support `--json`.
- `deploy` and `import` support `--dry-run`; `status --dry-run` is invalid.
- JSON output contract is defined in `../contracts/json-contract.md`.
- Logging contract is defined in `../contracts/logging-contract.md`.
- Metadata contract is defined in `../contracts/metadata-contract.md`.
- Validation/error codes are defined in `../contracts/validation-errors.md`.
