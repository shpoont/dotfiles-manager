---
owner: Core Engineering
status: Reference
last-updated: 2026-02-17
canonical-source: docs/internal/specs/cli-and-config-spec.md
---

# CLI and config spec (internal reference)

This document is a practical internal reference for command/config behavior.

Canonical policy decisions are in `decisions.md`.
Contract-level details are in `../contracts/*`.

## Command surface

```text
dotfiles-manager --version
dotfiles-manager version
dotfiles-manager [--config <path>] [--log-file <path>] [--log-level <debug|info|warn|error>] status [--json] [path]
dotfiles-manager [--config <path>] [--log-file <path>] [--log-level <debug|info|warn|error>] deploy [--dry-run] [--json] [path]
dotfiles-manager [--config <path>] [--log-file <path>] [--log-level <debug|info|warn|error>] import [--dry-run] [--json] [path]
```

Rules:
- config resolution order: `--config <path>` → `DOTFILES_MANAGER_CONFIG` → `./.dotfiles-manager.yaml` (cwd)
- default lookup is cwd-only (no parent search)
- default config filename is `.dotfiles-manager.yaml`
- `version` / `--version` bypass config resolution and print version immediately
- `version` does not accept `[path]`, `--json`, or `--dry-run`
- logs are written to platform-default log file path unless overridden with `--log-file`
- no log format flag is supported; logs are human-readable text only
- log level defaults to `info`; supported levels: `debug`, `info`, `warn`, `error`
- warnings/errors are emitted as human-readable stderr diagnostics
- `--dry-run` is valid for `deploy`/`import` only
- `[path]` narrows execution to matching target subpaths (against post-expansion target roots)

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
- `target`: relative to `$HOME` after env expansion
- `source`: relative to config file directory after env expansion
- env placeholders are supported in `target`/`source`: `$VAR`, `${VAR}`
- missing or empty env values are validation errors
- expanded paths are still required to be relative-only and non-escaping
- unknown keys are validation errors

Machine-readable schema:
- `../contracts/config-schema.json` defines the YAML config structure for editor/tooling validation.
- Runtime validation in `internal/config` remains authoritative.

## Behavior summary

- `version`/`--version`: print `dotfiles-manager version <value>` and exit (`dev` for non-release local builds)
- `status`: report drift and candidate sets; unmanaged/missing candidates are pattern-gated
- `deploy`: source -> target; optional unmanaged removal by patterns
- `import`: target -> source; optional unmanaged adds + optional missing deletes by patterns
- with default empty pattern lists, commands evaluate manifest paths only (no broad unmanaged target scan)

## Output model summary

Text mode:
- sync blocks always start with:
  - `sync[idx] target=~/<target> source=./<source>`
  - header uses configured path text (placeholders stay visible if present)
- each command prints only non-empty phase blocks
- summary line prints only non-zero categories
- status uses potential-action phrases (`can create`, `can update`, `can replace type`, `can add`, `can remove`)
- deploy/import use execution verbs (`create`, `update`, `replace_type`, `add`, `remove`)
- no color output by default

JSON mode (`--json`):
- schema version is `3.0`
- each sync has `operations[]` (exact files + phase/action/state)
- summary fields are command-specific aggregate counts with fixed keys (zero values retained)
- status `action` values use the same potential-action wording as text mode
- see full details in `../contracts/json-contract.md`

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
