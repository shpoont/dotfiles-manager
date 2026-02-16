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
- Logs default to text; use `--log-format json` for machine-readable logs.
- Log level defaults to `info`; use `--log-level` to change verbosity.

For deeper implementation/spec details, see `../internal/README.md`.
