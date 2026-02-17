# User documentation

This section is the user-facing guide for `dotfiles-manager`.

If you are new, start here:
1. [`getting-started.md`](./getting-started.md)
2. [`configuration.md`](./configuration.md)
3. [`commands.md`](./commands.md)
4. [`faq.md`](./faq.md)

## What this tool does

`dotfiles-manager` syncs files between:
- a repository-managed **source** directory (manifest, source of truth), and
- one or more `$HOME`-relative **target** directories.

Main commands:
- `status` (preview)
- `deploy` (source → target)
- `import` (target → source)

## Important baseline

- Config is required for every run, resolved in this order:
  1. `--config <path>`
  2. `DOTFILES_MANAGER_CONFIG`
  3. `./.dotfiles-manager.yaml` in the current working directory
- Config format is YAML.
- Default discovery is current-directory only (no parent-directory search).
- Config JSON Schema is available at `../internal/contracts/config-schema.json` for editor/tooling validation.
- Logs are always written to a log file.
  - macOS default: `~/Library/Logs/dotfiles-manager/dotfiles-manager.log`
  - Linux default: `${XDG_STATE_HOME:-~/.local/state}/dotfiles-manager/dotfiles-manager.log`
  - override with `--log-file <path>`
  - logs are always human-readable text (no log format option)
- Log level defaults to `info`; use `--log-level` to change verbosity.
- Warnings/errors are emitted as human-readable diagnostics on stderr.

For deeper implementation/spec details, see `../internal/README.md`.
