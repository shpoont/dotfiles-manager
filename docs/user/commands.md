# Commands

## Command format

```text
dotfiles-manager --version
dotfiles-manager version
dotfiles-manager [--config <path>] [--log-file <path>] [--log-level <debug|info|warn|error>] status [--json] [path]
dotfiles-manager [--config <path>] [--log-file <path>] [--log-level <debug|info|warn|error>] diff [--json] [--direction <both|deploy|import>] [--context <N>] [--patch] [path]
dotfiles-manager [--config <path>] [--log-file <path>] [--log-level <debug|info|warn|error>] deploy [--dry-run] [--json] [path]
dotfiles-manager [--config <path>] [--log-file <path>] [--log-level <debug|info|warn|error>] import [--dry-run] [--json] [path]
dotfiles-manager [--config <path>] migrate [--dry-run] [--json]
dotfiles-manager [--config <dotfiles-manager.v2.yaml>] status [--json] [--machine-id <id>] [--user-id <id>] [--profile <layer>] [target[:setting]]
dotfiles-manager [--config <dotfiles-manager.v2.yaml>] diff [--json] [--machine-id <id>] [--user-id <id>] [--profile <layer>] [target[:setting]]
dotfiles-manager [--config <dotfiles-manager.v2.yaml>] save [--dry-run] [--yes] [--json] [--machine-id <id>] [--user-id <id>] [--profile <layer>] [target[:setting]]
dotfiles-manager [--config <dotfiles-manager.v2.yaml>] apply [--dry-run] [--yes] [--json] [--machine-id <id>] [--user-id <id>] [--profile <layer>] [target[:setting]]
dotfiles-manager recipe list [--json]
dotfiles-manager recipe explain <target> [--json]
```

Config is resolved in this order:
1. `--config <path>`
2. `DOTFILES_MANAGER_CONFIG`
3. `./.dotfiles-manager.yaml` in the current working directory

No parent-directory config search is performed.
`version`/`--version` do not require config resolution.

For v2 selected-setting commands, `--config` must point at
`dotfiles-manager.v2.yaml`. If no v1 `.dotfiles-manager.yaml` exists and the
current directory is inside a v2 root, `status` and `diff` can auto-detect the
v2 root. Live `save` and `apply` require a v2 root and require `--yes` before a
planned write is performed.

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

## `version` and `--version`

Both commands print a single line and exit:

```text
dotfiles-manager version 0.1.4
```

Behavior:
- `dotfiles-manager version` and `dotfiles-manager --version` are equivalent
- they do not load config
- they do not accept `[path]`, `--json`, or `--dry-run`
- release builds print semantic version
- local non-release builds print `dev`

## `status [--json] [path]`

Reports:
- deploy drift (source → target)
- import drift (target → source on manifest paths)
- incoming unmanaged candidates
- removable unmanaged candidates
- removable missing-manifest candidates

`status` does not write files.

Candidate-set scanning is opt-in:
- unmanaged/removal candidate discovery runs only when related pattern lists are configured
- with default empty pattern lists, `status` compares manifest paths only
- when enabled, discovery starts from literal pattern roots (for example `.codex/skills/**` starts at `.codex/skills`)
- wildcard-first patterns (for example `**/*.tmp`) can still require broad scans

Example text output shape:

```text
reminder: deploy applies source -> target; import applies target -> source
sync[0] target=~/.config/nvim source=./source/nvim
deploy[2] (source -> target)
  can create   lua/init.lua (file->missing)
import[1] (target -> source)
  can update   lua/init.lua (file->file)
hint: same path in deploy/import: lua/init.lua
remove-missing[1]
  can remove   lua/legacy.lua (file)
summary deploy=2 import=1 remove-missing=1
```

## `diff [--json] [--direction <both|deploy|import>] [--context <N>] [--patch] [path]`

Behavior:
- preview-only command (no filesystem writes)
- shows unified patch-style diffs for candidate operations
- default direction is `both`
- `--direction deploy` limits phases to deploy + remove-unmanaged
- `--direction import` limits phases to import + incoming-unmanaged + remove-missing
- `--context <N>` controls unified hunk context lines (default `3`, must be `>= 0`)
- binary/type-change/oversize entries are reported with reason text (no patch body)
- per-file patch body is omitted when it exceeds 1 MiB

Flag notes:
- `--dry-run` is not supported on `diff` (it is already preview-only)
- in JSON mode, patch text is included only with `--patch`
- `--patch` is not supported without `--json`

Example text output shape:

```text
reminder: deploy diff compares target -> source; import diff compares source -> target
sync[0] target=~/.config/nvim source=./source/nvim
deploy-diff[1] (source -> target)
  path: lua/init.lua (file->file)
--- target/lua/init.lua
+++ source/lua/init.lua
@@ -1 +1 @@
-target
+source
summary deploy-diff=1 unified=1
```

## `deploy [--dry-run] [--json] [path]`

Behavior:
- copy/update managed content source → target
- replace type mismatches (file/dir/symlink)
- then remove unmanaged target paths matching `on.deploy.remove-unmanaged`
- if remove patterns are empty/missing, no unmanaged paths are removed
- with empty remove patterns, deploy does not perform unmanaged target-tree scanning
- with remove patterns present, scanning starts from literal pattern roots when available

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
- unmanaged add/remove-missing candidate discovery is include-gated
- with default empty include lists, import evaluates manifest paths only (no unmanaged target-tree scan)
- with add-unmanaged includes present, scanning starts from literal include roots when available

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

## `migrate [--dry-run] [--json]`

Behavior:
- reads existing v1 `.dotfiles-manager.yaml` `syncs:`
- shows each legacy source and target exactly as configured
- shows expanded source/target paths separately
- proposes v2 `custom.files` setting refs, driver, desired artifact binding, and generated file paths
- does not delete or rewrite the v1 config
- `migrate --dry-run` is preview-only and writes nothing
- plain `migrate` writes only a new migration run directory:

```text
migrations/v1-to-v2/<run-id>/
  migration-plan.yaml
  generated/
    dotfiles-manager.v2.yaml
    profiles/
      stacks/legacy.yaml
      layers/legacy.yaml
    recipes/local/custom.files/recipe.yaml
    desired/user/legacy/targets/custom.files/artifacts/...
```

Plain `migrate` does **not** write active v2 paths at the repository root. It
does not create or replace root-level `dotfiles-manager.v2.yaml`, `profiles/`,
`desired/`, or `recipes/`. The generated files live under the migration run's
`generated/` directory so they can be reviewed, diffed, copied, or promoted by
a later explicit step.

Plain `migrate` also keeps `.dotfiles-manager.yaml` byte-for-byte unchanged.
If any legacy sync cannot be represented safely, migration is blocked and no
final run directory is produced by default.

JSON output uses v2-style field casing (`schemaVersion`, `dryRun`,
`configPath`, `generatedFiles`) rather than the v1 command envelope.

Dry-run text output shape:

```text
MODE: DRY RUN (no writes)
migration run=dry-run config=/repo/.dotfiles-manager.yaml
v1 config action: leave unchanged
v1 command behavior: unchanged

sync[0]
  legacy source: dotfiles/git/.gitconfig
  legacy target: .gitconfig
  expanded source: /repo/dotfiles/git/.gitconfig
  expanded target: /home/user/.gitconfig
  proposed: custom.files:sync-0 driver=file
  artifact binding: desired://user/legacy/targets/custom.files/artifacts/sync-0
  v1 config: leave-unchanged
  result: planned
  generated files:
    migrations/v1-to-v2/dry-run/generated/desired/user/legacy/targets/custom.files/artifacts/sync-0
summary syncs=1 planned=1 blocked=0 files=1 file-trees=0 generated-files=6 status=ok
```

Plain migrate text output uses the same item mapping, but starts with:

```text
MODE: MIGRATE (writes generated output only)
migration run=<run-id> config=/repo/.dotfiles-manager.yaml
output: /repo/migrations/v1-to-v2/<run-id>
```

## v2 selected settings: Git identity example

The first bundled v2 app recipe is `git`, limited to non-credential identity
settings:

- `git:user.email`
- `git:user.name`

It manages only `~/.gitconfig` `[user] email` and `[user] name`. It does not
manage credential helpers, tokens, signing keys, includes, URL rewrites,
aliases, arbitrary sections, or repository-local `.git/config`.

Until the v2 `add` command ships, select Git in a profile layer:

```yaml
# profiles/layers/global.yaml
schema: dotfiles-manager.v2.profile-layer
schemaVersion: 1
selections:
  git:
    settings:
      user.email:
        scope: user
```

Then use the existing selected-value commands. `save --yes` is the supported
import/promotion command for selected Git identity values: it copies the
selected live value into v2 desired state after you first preview it.

```bash
dotfiles-manager status --user-id leon git:user.email
dotfiles-manager save --dry-run --user-id leon git:user.email
dotfiles-manager save --yes --user-id leon git:user.email
dotfiles-manager diff --user-id leon git:user.email
dotfiles-manager apply --dry-run --user-id leon git:user.email
dotfiles-manager apply --yes --user-id leon git:user.email
```

When no desired artifact exists and the selected live Git value exists,
`save --dry-run` reports `action=would-promote`. That means the value can be
promoted into managed desired state with `save --yes`; normal CLI output still
omits the raw value.

`save --yes` copies the selected live value from `~/.gitconfig` into the desired
settings artifact for that profile subject. For user-scoped Git settings the
path is:

```text
desired/user/<user>/targets/git/settings.yaml
```

For example, `--user-id leon` writes
`desired/user/leon/targets/git/settings.yaml`.

Inspecting that desired file directly can reveal the raw safe identity value
such as an email address or display name. The raw value is stored there because
the manager needs an actual desired value to apply later. Normal command output,
reports, ledgers, backup metadata, and JSON previews stay redacted.

Promotion applies only to the selected safe Git identity key. Repeat the
preview-and-save flow for both `git:user.email` and `git:user.name` if you want
to manage both values.

`apply --yes` writes the desired value back to `~/.gitconfig` after planning,
backup, write, and verification. The backup is a local whole-file pre-apply
backup under the manager's local state directory; normal output, ledgers, and
backup metadata do not show raw Git config values.

## `[path]` scoping

`[path]` can be absolute, `~`-based, or relative.

A sync is selected only when `[path]` is:
- exactly the sync target, or
- inside the sync target subtree

Target matching uses post-expansion target roots (after `$VAR`/`${VAR}` resolution).

If `[path]` matches no syncs, command fails.

## Output and exit codes

- `version`/`--version` output one line: `dotfiles-manager version <value>`.
- `version`/`--version` exit `0` and do not require config.
- text mode prints per-sync sections with exact file operations.
- status text includes one concise direction reminder line once per run.
- diff text includes one concise direction reminder line once per run.
- every sync header uses:
  - `sync[idx] target=~/<target> source=./<source>`
- sync headers show configured path text (placeholders stay visible if present in config)
- when `[path]` scopes into a subpath, header appends:
  - `scope=<sync-relative-prefix>`
- text mode only prints non-empty phase blocks.
- status phase headers include direction:
  - `deploy[n] (source -> target)`
  - `import[n] (target -> source)`
- diff phase headers include direction context per phase block.
- status prints `hint: same path in deploy/import: ...` when a path appears in both direction blocks.
- text summary line only includes non-zero categories.
- status actions are potential, human-readable phrases (`can create`, `can update`, `can replace type`, `can add`, `can remove`).
- deploy/import actions remain actual execution verbs (`create`, `update`, `replace_type`, `add`, `remove`).
- diff actions remain potential candidate wording and include diff metadata fields (`diff_kind`, labels, patch availability).
- `--json` returns machine-readable output (`schema_version: "4.0"`), with:
  - `syncs[].operations[]` for exact per-file operations
  - command-specific summary counts (fixed key set; zero values retained)
- Exit `0` on success (including `status`/`diff` with drift).
- Non-zero on validation/runtime errors.
- Commands are fail-fast on runtime errors.
