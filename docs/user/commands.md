# Commands

## Command format

```text
dotfiles-manager [--config <path>] [--log-format <text|json>] [--log-level <debug|info|warn|error>] status [--json] [path]
dotfiles-manager [--config <path>] [--log-format <text|json>] [--log-level <debug|info|warn|error>] deploy [--dry-run] [--json] [path]
dotfiles-manager [--config <path>] [--log-format <text|json>] [--log-level <debug|info|warn|error>] import [--dry-run] [--json] [path]
```

Config is resolved in this order:
1. `--config <path>`
2. `DOTFILES_MANAGER_CONFIG`
3. `./.dotfiles-manager.yaml` in the current working directory

No parent-directory config search is performed.

Log format:
- default: `text`
- optional: `--log-format json`
- logs are emitted on stderr (stdout remains command output, including `--json`)

Log level:
- default: `info`
- supported: `debug`, `info`, `warn`, `error`
- set with `--log-level <level>`

## `status [--json] [path]`

Reports:
- deploy drift (source → target)
- import drift (target → source on manifest paths)
- incoming unmanaged candidates
- removable unmanaged candidates
- removable missing-manifest candidates

`status` does not write files.

## `deploy [--dry-run] [--json] [path]`

Behavior:
- copy/update managed content source → target
- replace type mismatches (file/dir/symlink)
- then remove unmanaged target paths matching `on.deploy.remove-unmanaged`
- if remove patterns are empty/missing, no unmanaged paths are removed

`--dry-run` plans and reports operations without writing.

## `import [--dry-run] [--json] [path]`

Behavior:
- update managed content target → source
- optionally add unmanaged target files via `on.import.add-unmanaged.include/exclude`
- optionally remove source paths missing in target via `on.import.remove-missing.include/exclude`
- replace type mismatches (file/dir/symlink)

`--dry-run` plans and reports operations without writing.

## `[path]` scoping

`[path]` can be absolute, `~`-based, or relative.

A sync is selected only when `[path]` is:
- exactly the sync target, or
- inside the sync target subtree

If `[path]` matches no syncs, command fails.

## Output and exit codes

- `--json` returns machine-readable output.
- Exit `0` on success (including `status` with drift).
- Non-zero on validation/runtime errors.
- Commands are fail-fast on runtime errors.
