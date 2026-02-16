---
owner: Core Engineering
status: Reference
last-updated: 2026-02-16
canonical-source: docs/internal/specs/cli-and-config-spec.md
---

# CLI and config spec (internal reference)

This document is a practical internal reference for command/config behavior.

Canonical policy decisions are in `decisions.md`.
Contract-level details are in `../contracts/*`.

## Command surface

```text
dotfiles-manager [--config <path>] [--log-format <text|json>] [--log-level <debug|info|warn|error>] status [--json] [path]
dotfiles-manager [--config <path>] [--log-format <text|json>] [--log-level <debug|info|warn|error>] deploy [--dry-run] [--json] [path]
dotfiles-manager [--config <path>] [--log-format <text|json>] [--log-level <debug|info|warn|error>] import [--dry-run] [--json] [path]
```

Rules:
- config resolution order: `--config <path>` → `DOTFILES_MANAGER_CONFIG` → `./.dotfiles-manager.yaml` (cwd)
- default lookup is cwd-only (no parent search)
- default config filename is `.dotfiles-manager.yaml`
- log format defaults to `text`; `json` is optional for machine parsing
- log level defaults to `info`; supported levels: `debug`, `info`, `warn`, `error`
- `--dry-run` is valid for `deploy`/`import` only
- `[path]` narrows execution to matching target subpaths

## Config surface

```yaml
syncs:
  - target: .config/nvim
    source: .config/nvim
    on:
      deploy:
        remove-unmanaged:
          - '**/*.bak'
      import:
        add-unmanaged:
          include:
            - '**'
          exclude:
            - '**/*.tmp'
        remove-missing:
          include:
            - 'lua/**'
          exclude:
            - 'lua/local/**'
```

Key constraints:
- `target`: relative to `$HOME`
- `source`: relative to config file directory
- config paths are relative-only
- unknown keys are validation errors

## Behavior summary

- `status`: report drift and candidate sets
- `deploy`: source -> target; optional unmanaged removal by patterns
- `import`: target -> source; optional unmanaged adds + optional missing deletes by patterns

Cross-cutting:
- source is manifest/source-of-truth
- fail-fast runtime behavior
- deterministic ordering (config order; later overlapping sync wins)

## Where to read details

- Decision source: `decisions.md`
- Scenario outcomes: `decision-matrix.md`
- JSON output: `../contracts/json-contract.md`
- Metadata: `../contracts/metadata-contract.md`
- Error codes: `../contracts/validation-errors.md`
- Acceptance criteria: `../engineering/acceptance-checklist.md`
- Open follow-ups: `open-questions.md`
