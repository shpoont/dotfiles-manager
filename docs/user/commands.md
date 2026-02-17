# Commands

## Command format

```text
dotfiles-manager [--config <path>] [--log-file <path>] [--log-level <debug|info|warn|error>] status [--json] [path]
dotfiles-manager [--config <path>] [--log-file <path>] [--log-level <debug|info|warn|error>] deploy [--dry-run] [--json] [path]
dotfiles-manager [--config <path>] [--log-file <path>] [--log-level <debug|info|warn|error>] import [--dry-run] [--json] [path]
```

Config is resolved in this order:
1. `--config <path>`
2. `DOTFILES_MANAGER_CONFIG`
3. `./.dotfiles-manager.yaml` in the current working directory

No parent-directory config search is performed.

Log file destination:
- default paths:
  - macOS: `~/Library/Logs/dotfiles-manager/dotfiles-manager.log`
  - Linux: `${XDG_STATE_HOME:-~/.local/state}/dotfiles-manager/dotfiles-manager.log`
- override path with `--log-file <path>`
- logs are always human-readable text
- there is no log format option

Log level:
- default: `info`
- supported: `debug`, `info`, `warn`, `error`
- set with `--log-level <level>`

stderr diagnostics:
- warnings and errors are emitted as human-readable text on stderr
- stdout remains command output (including `--json`)

## `status [--json] [path]`

Reports:
- deploy drift (source → target)
- import drift (target → source on manifest paths)
- incoming unmanaged candidates
- removable unmanaged candidates
- removable missing-manifest candidates

`status` does not write files.

Example text output shape:

```text
sync[0] target=~/.config/nvim source=./source/nvim
deploy[2]
  create       lua/init.lua (file->missing)
import[1]
  update       lua/plugins.lua (file->file)
incoming-unmanaged[0]
remove-unmanaged[0]
remove-missing[1]
  remove       lua/legacy.lua (file)
summary deploy=2 import=1 incoming-unmanaged=0 remove-unmanaged=0 remove-missing=1
```

## `deploy [--dry-run] [--json] [path]`

Behavior:
- copy/update managed content source → target
- replace type mismatches (file/dir/symlink)
- then remove unmanaged target paths matching `on.deploy.remove-unmanaged`
- if remove patterns are empty/missing, no unmanaged paths are removed

`--dry-run` plans and reports operations without writing.

Example text output shape:

```text
sync[0] target=~/.config/nvim source=./source/nvim
copy[2]
  update       lua/init.lua (file)
  create       lua/keymaps.lua (file)
remove-unmanaged[1]
  remove       tmp/old.lua (file)
summary dry-run=true copied=2 remove-unmanaged=1
```

## `import [--dry-run] [--json] [path]`

Behavior:
- update managed content target → source
- optionally add unmanaged target files via `on.import.add-unmanaged.include/exclude`
- optionally remove source paths missing in target via `on.import.remove-missing.include/exclude`
- replace type mismatches (file/dir/symlink)

`--dry-run` plans and reports operations without writing.

Example text output shape:

```text
sync[0] target=~/.config/nvim source=./source/nvim
update-managed[1]
  update       lua/init.lua (file)
add-unmanaged[1]
  add          lua/new-plugin.lua (file)
remove-missing[1]
  remove       lua/legacy.lua (file)
summary dry-run=true updated-managed=1 added-unmanaged=1 removed-missing=1
```

## `[path]` scoping

`[path]` can be absolute, `~`-based, or relative.

A sync is selected only when `[path]` is:
- exactly the sync target, or
- inside the sync target subtree

If `[path]` matches no syncs, command fails.

## Output and exit codes

- text mode prints per-sync sections with exact file operations.
- every sync header uses:
  - `sync[idx] target=~/<target> source=./<source>`
- when `[path]` scopes into a subpath, header appends:
  - `scope=<sync-relative-prefix>`
- actions are explicit words (`create`, `update`, `replace_type`, `add`, `remove`).
- `--json` returns machine-readable output (`schema_version: "2.0"`), with:
  - `syncs[].operations[]` for exact per-file operations
  - command-specific summary counts
- Exit `0` on success (including `status` with drift).
- Non-zero on validation/runtime errors.
- Commands are fail-fast on runtime errors.
