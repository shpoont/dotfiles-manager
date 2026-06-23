# Commands

## Command format

Common v2 local-settings-manager commands:

```text
dotfiles-manager --version
dotfiles-manager version
dotfiles-manager init [--dry-run] [--json] [--machine-id <id>] [--user-id <id>] [--non-interactive] [--yes]
dotfiles-manager [--config <dotfiles-manager.v2.yaml>] recipe list [--json]
dotfiles-manager [--config <dotfiles-manager.v2.yaml>] recipe discover [target] [--json]
dotfiles-manager [--config <dotfiles-manager.v2.yaml>] recipe explain <target> [--json]
dotfiles-manager [--config <dotfiles-manager.v2.yaml>] add <target> [--setting <id>] [--scope <scope>] [--profile <layer>] [--dry-run] [--yes] [--non-interactive] [--json]
dotfiles-manager [--config <dotfiles-manager.v2.yaml>] list [--json] [--machine-id <id>] [--user-id <id>] [--profile <layer>]
dotfiles-manager [--config <dotfiles-manager.v2.yaml>] status [--json] [--verbose] [--machine-id <id>] [--user-id <id>] [--profile <layer>] [target[:setting]]
dotfiles-manager [--config <dotfiles-manager.v2.yaml>] diff [--json] [--verbose] [--machine-id <id>] [--user-id <id>] [--profile <layer>] [target[:setting]]
dotfiles-manager [--config <dotfiles-manager.v2.yaml>] sync [--yes] [--non-interactive] [--json] [--machine-id <id>] [--user-id <id>] [--profile <layer>] [target[:setting]]
dotfiles-manager [--config <dotfiles-manager.v2.yaml>] save [--dry-run] [--yes] [--json] [--verbose] [--machine-id <id>] [--user-id <id>] [--profile <layer>] [target[:setting]]
dotfiles-manager [--config <dotfiles-manager.v2.yaml>] apply [--dry-run] [--yes] [--json] [--verbose] [--machine-id <id>] [--user-id <id>] [--profile <layer>] [target[:setting]]
dotfiles-manager [--config <dotfiles-manager.v2.yaml>] backup list [--json]
dotfiles-manager [--config <dotfiles-manager.v2.yaml>] backup show <run-id> [--json]
dotfiles-manager [--config <dotfiles-manager.v2.yaml>] restore <run-id> [--dry-run] [--yes] [--json] [--machine-id <id>] [--user-id <id>] [--profile <layer>]
dotfiles-manager [--config <dotfiles-manager.v2.yaml>] app create <target-id> --template <file|selected-value|native-export> ...
dotfiles-manager [--config <dotfiles-manager.v2.yaml>] app validate <target-id> [--json]
dotfiles-manager [--config <dotfiles-manager.v2.yaml>] app test <target-id> --roundtrip [--fixture <name>] [--json]
```

Legacy v1 file-sync commands remain available for `.dotfiles-manager.yaml`:

```text
dotfiles-manager [--config <.dotfiles-manager.yaml>] status [--json] [path]
dotfiles-manager [--config <.dotfiles-manager.yaml>] diff [--json] [--direction <both|deploy|import>] [--context <N>] [--patch] [path]
dotfiles-manager [--config <.dotfiles-manager.yaml>] deploy [--dry-run] [--json] [path]
dotfiles-manager [--config <.dotfiles-manager.yaml>] import [--dry-run] [--json] [path]
dotfiles-manager [--config <.dotfiles-manager.yaml>] migrate [--dry-run] [--json]
```

Config is resolved in this order:
1. `--config <path>`
2. `DOTFILES_MANAGER_CONFIG`
3. `./.dotfiles-manager.yaml` in the current working directory for legacy v1
   commands; v2 commands expect `dotfiles-manager.v2.yaml` and can detect a v2
   root for supported preview paths when no v1 config is present.

No parent-directory search is performed for v1 config discovery.
`version`/`--version` do not require config resolution.

For v2 selected-setting commands, scripts should pass `--config
<dotfiles-manager.v2.yaml>` explicitly. The normal v2 workflow is
`status -> diff -> sync`. `sync` is the primary mutating command. `save` and
`apply` are compatibility aliases for explicit directions:

```text
save  = sync live settings -> stored settings
apply = sync stored settings -> live settings
```

Mutating `sync`, `save`, `apply`, and `restore` require a v2 root and require
`--yes` before a planned write is performed.

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

Selected-setting output tiers:
- default text for `status`, `diff`, `sync`, and the `save`/`apply`
  directional aliases is human-first: it names the selected setting, says
  whether anything changed, shows user-level live/stored-settings paths, hides
  raw values, and gives a safe next command.
- `--verbose` is currently implemented for v2 selected-setting `status`,
  `diff`, `save`, and `apply` only. It keeps the same default explanation and
  appends technical details such as profile stack, refs, resource/driver/selector,
  planner state/action, desired/state URIs, run ids, and backup refs. It still
  redacts managed values and secret-bearing payload bytes.
- `--json` is the stable scripting output. `--json --verbose` still writes only
  the existing JSON document to stdout; verbose prose is suppressed, not moved
  to stderr.

## `version` and `--version`

Both commands print a single line and exit:

```text
dotfiles-manager version=0.2.0 commit=cd127ba0969c07eba05916004547e0094303f9cb date=2026-06-15T18:06:25Z channel=stable provenance=goreleaser
```

Behavior:
- `dotfiles-manager version` and `dotfiles-manager --version` are equivalent
- they do not load config
- they do not accept `[path]`, `--json`, or `--dry-run`
- GoReleaser release archives print the semantic version, full source commit,
  commit timestamp in UTC RFC3339 `Z` form, `channel=stable` or
  `channel=prerelease`, and `provenance=goreleaser`
- Homebrew release builds use the same fields; `provenance=homebrew-source`
  identifies a formula build from the release source archive
- local non-release builds use the explicit fallback
  `version=dev commit=unknown date=unknown channel=dev provenance=unspecified`

## v2 `init`

`init` creates the v2 settings-folder scaffold and local identity state:

```bash
dotfiles-manager init --machine-id docs-machine --user-id docs-user
```

Settings-folder files:

```text
dotfiles-manager.v2.yaml
profiles/stacks/default.yaml
profiles/layers/global.yaml
```

Identity files are written under the platform-specific v2 local state root and
are reported as `state://identity/...` references. `init --dry-run` previews the
plan; `init --json` emits `dotfiles-manager.v2.init`.

## v2 `list`

`list` shows selected managed settings in the resolved profile:

```bash
dotfiles-manager --config dotfiles-manager.v2.yaml list --user-id docs-user
```

It prints the selected ref, scope, subject, source layer, resource driver, named
location, selector, stored-settings reference, and suggested next commands. Use repeated
`--profile <layer>` flags to preview extra profile layers on top of the active
stack.

## v2 backups and restore

Use backup commands after an `apply --yes` or `restore --yes` creates local
backup evidence:

```bash
dotfiles-manager --config dotfiles-manager.v2.yaml backup list
dotfiles-manager --config dotfiles-manager.v2.yaml backup show <run-id>
dotfiles-manager --config dotfiles-manager.v2.yaml restore <run-id> --dry-run --user-id <user-id>
dotfiles-manager --config dotfiles-manager.v2.yaml restore <run-id> --yes --user-id <user-id>
```

Restore should always be previewed first. For selected values backed by files,
restore rolls back the whole backing file from the backup payload; it is not a
semantic single-value rollback. Backup payload contents are not printed by
`backup list` or `backup show`, but local backup payloads can contain actual
managed bytes.

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
- proposes v2 `custom.files` setting refs, driver, stored artifact binding, and generated file paths
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

## v2 target discovery

`recipe list` remains static bundled metadata. To inspect whether bundled
targets appear installed or configured, use the explicit read-only discovery
command:

```bash
dotfiles-manager recipe discover --json
dotfiles-manager recipe discover git
dotfiles-manager recipe discover ssh --json
```

Discovery never mutates files or app state. It does not read config contents,
stored artifacts, backups, ledgers, profile selections, native export/import
commands, or target runtime state. It only performs PATH command lookups and
lstat-style metadata checks of declared live config paths.

Summary states are `unsupported-platform`, `ambiguous`, `config-present`,
`installed`, `config-missing`, and `not-applicable`. JSON also includes separate
`platformState`, `binaryState`, and `configState` axes plus metadata-only command
and config probes.

`custom.files` reports `not-applicable` because it has no app binary or fixed
bundled live config path to discover.

## v2 add: select a supported target

`add` updates the active v2 profile layer so later `status`, `diff`, `sync`,
and the directional aliases know which supported settings to manage. It does
**not** import values, write stored settings, write live app files, create
backups, or update ledgers.

Common examples:

```bash
dotfiles-manager add git --yes
dotfiles-manager add ssh --setting config
dotfiles-manager add zsh --setting zshrc --scope user
dotfiles-manager add nvim --dry-run --yes
```

If the active profile stack has multiple layers, pass `--profile <layer>` or
run interactively and choose the layer. `--json`, `--non-interactive`, and
`--yes` never prompt; when a choice is required they return
`add.choice-required` with machine-readable `missingChoices`.

For file and file-tree settings, `add` writes explicit artifact metadata such
as `artifact: artifacts/config` so the setting is stored as an artifact
payload. For scalar settings, the profile entry can remain scope-only and later
sync to stored settings writes the value in `settings.yaml`.

## v2 app authoring: custom local recipes

Advanced users can draft custom local recipes under
`recipes/local/<target-id>/`:

```bash
dotfiles-manager app create local-my-app \
  --template file \
  --from-path ~/.config/my-app/config.yaml \
  --setting config \
  --setting-label "Config file" \
  --scope-default user \
  --lifecycle allowed

dotfiles-manager app validate local-my-app
dotfiles-manager app test local-my-app --roundtrip
```

`app create` writes recipe metadata and documentation only. It does not read
live app files, import values, create trust records, or select the app in a
profile. `app validate` checks recipe metadata without reading live app config.

`app test --roundtrip` runs only synthetic fixtures from:

```text
recipes/local/<target-id>/fixtures/roundtrip/<fixture-name>/
```

It copies fixture data into a temporary directory, maps named recipe locations
to `input/live/locations/<location-id>/...`, and compares results with
`expected/desired/` and `expected/live/`. It does not touch real app config,
the real stored-settings root, trust records, backups, or ledgers.

Supported roundtrip drivers in this tranche are whole-file `file` resources and
selected values backed by `ini-file`, `json-file`, `yaml-file`, `toml-file`, or
`plist-file`. Native export/import and `file-tree` fixtures are validate-only or
unsupported until later implementation work.

## v2 selected settings: Git identity example

The first bundled v2 app recipe is `git`, limited to non-credential identity
settings:

- `git:user.email`
- `git:user.name`

It manages only `~/.gitconfig` `[user] email` and `[user] name`. It does not
manage credential helpers, tokens, signing keys, includes, URL rewrites,
aliases, arbitrary sections, or repository-local `.git/config`.

Select Git with the `add` command:

```bash
dotfiles-manager add git --yes
```

That selects the recommended Git identity settings. To select only one setting,
name it explicitly:

```bash
dotfiles-manager add git --setting user.email
```

The equivalent profile-layer entry is:

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

Then use the sync-first selected-value model. `sync` is primary. When there are
no stored settings yet, use the explicit `save` compatibility alias to choose
the live-settings-to-stored-settings direction after you first preview it.

```bash
dotfiles-manager status --user-id leon git:user.email
dotfiles-manager save --dry-run --user-id leon git:user.email
dotfiles-manager save --yes --user-id leon git:user.email
dotfiles-manager diff --user-id leon git:user.email
dotfiles-manager apply --dry-run --user-id leon git:user.email
dotfiles-manager apply --yes --user-id leon git:user.email
```

When no stored settings exist and the selected live Git value exists,
`save --dry-run` explains in plain language that the current live value can be
synced to stored settings. `--verbose` can add troubleshooting metadata, but it
still keeps the raw value hidden.

`save --yes` copies the selected live value from `~/.gitconfig` into stored
settings for that profile subject. For user-scoped Git settings the
path is:

```text
desired/user/<user>/targets/git/settings.yaml
```

For example, `--user-id leon` writes
`desired/user/leon/targets/git/settings.yaml`.

Inspecting that stored settings file directly can reveal the raw safe identity value
such as an email address or display name. The raw value is stored there because
the manager needs an actual value to sync later. Normal command output,
reports, ledgers, backup metadata, and JSON previews stay redacted.

The first live-settings-to-stored-settings sync applies only to the selected
safe Git identity key. Repeat the preview-and-save flow for both
`git:user.email` and `git:user.name` if you want to manage both values.

`apply --yes` syncs the stored value back to `~/.gitconfig` after planning,
backup, write, and verification. The backup is a local whole-file pre-apply
backup under the manager's local state directory; normal output, ledgers, and
backup metadata do not show raw Git config values.

## v2 selected settings: Starship prompt options example

The bundled `starship` recipe manages a small selected-key slice of
`~/.config/starship.toml`:

- `starship:add_newline` (`bool`)
- `starship:follow_symlinks` (`bool`)
- `starship:scan_timeout` (non-negative integer)
- `starship:command_timeout` (non-negative integer)

Select Starship with `add`. For example, to manage one key:

```bash
dotfiles-manager add starship --setting add_newline
```

The equivalent profile-layer entry is:

```yaml
# profiles/layers/global.yaml
schema: dotfiles-manager.v2.profile-layer
schemaVersion: 1
selections:
  starship:
    settings:
      add_newline:
        scope: user
```

Then use the same sync-first selected-value model, using `save`/`apply` only as
explicit directional aliases:

```bash
dotfiles-manager recipe explain starship
dotfiles-manager status --user-id leon starship:add_newline
dotfiles-manager save --dry-run --user-id leon starship:add_newline
dotfiles-manager save --yes --user-id leon starship:add_newline
dotfiles-manager diff --user-id leon starship:add_newline
dotfiles-manager apply --dry-run --user-id leon starship:add_newline
dotfiles-manager apply --yes --user-id leon starship:add_newline
```

For user-scoped Starship settings, `save --yes` writes stored settings to:

```text
desired/user/<user>/targets/starship/settings.yaml
```

For example, `--user-id leon` writes
`desired/user/leon/targets/starship/settings.yaml`.

This slice manages only the four root-level TOML keys above. It does not yet
auto-discover `STARSHIP_CONFIG` or process `XDG_CONFIG_HOME` non-default
locations, manage shell init, install Starship, or manage full-file Starship
configuration with comments, palettes, modules, presets, custom commands, or
formatting. The bundled default live path is the HOME-relative
`~/.config/starship.toml`; a non-default live root requires an explicit named
location override and is not inferred from the manager process environment.
TOML selected-key apply may canonicalize/reformat the file and may not preserve
comments. Use a whole-file recipe resource when you want byte-preserving file
management instead of selected TOML-key management.

## v2 selected settings: Zsh startup file example

The bundled `zsh` recipe manages a small explicit set of whole startup files
under the named `home` location, whose default is `~`:

- `zsh:zshrc` -> `~/.zshrc`
- `zsh:zprofile` -> `~/.zprofile`
- `zsh:zlogin` -> `~/.zlogin`
- `zsh:zlogout` -> `~/.zlogout`

Select Zsh with `add`. For example:

```bash
dotfiles-manager add zsh --setting zshrc
```

The equivalent profile-layer entry is:

```yaml
# profiles/layers/global.yaml
schema: dotfiles-manager.v2.profile-layer
schemaVersion: 1
selections:
  zsh:
    settings:
      zshrc:
        scope: user
        artifact: artifacts/zshrc
```

Then use the same sync-first selected file-resource model, using `save`/`apply`
only as explicit directional aliases:

```bash
dotfiles-manager recipe explain zsh
dotfiles-manager status --user-id leon zsh:zshrc
dotfiles-manager save --dry-run --user-id leon zsh:zshrc
dotfiles-manager save --yes --user-id leon zsh:zshrc
dotfiles-manager diff --user-id leon zsh:zshrc
dotfiles-manager apply --dry-run --user-id leon zsh:zshrc
dotfiles-manager apply --yes --user-id leon zsh:zshrc
```

For user-scoped Zsh files, `save --yes` writes the stored artifact to:

```text
desired/user/<user>/targets/zsh/artifacts/<setting-id>
```

For example, `--user-id leon` and `zsh:zshrc` write:

```text
desired/user/leon/targets/zsh/artifacts/zshrc
```

The stored artifact contains the raw file bytes because it is the file that
will be synced later. Normal text and JSON output, diffs, ledgers, and backup
metadata stay metadata-only and do not print raw startup file contents.

Zsh startup files can affect shell startup behavior. For `save` and `apply`
plans, selected Zsh files emit a warning diagnostic:

```text
zsh.risk.shell-startup-file
```

This warning does not block the plan by itself. It is there to make the write
risk visible before `save --yes` or `apply --yes`. `status` and `diff` do not
emit this write warning.

`zsh:zshrc` is the intended future default candidate for `add zsh`.
`zsh:zprofile`, `zsh:zlogin`, and `zsh:zlogout` are opt-in because they affect
login/logout startup behavior.

The bundled recipe deliberately does **not** manage:

- `.zshenv` / `zsh:zshenv` (`zsh.blocked.zshenv`)
- `.zsh_history`, `.zhistory`, or `zsh:history`
- `.zcompdump*` completion dump/cache files, `zsh:cache`, and `zsh:zsh-cache`
- `.zsh_sessions/` session state
- cache directories such as `.cache/` and `.config/zsh/.zcompdump*`
- plugin/generated state such as `.oh-my-zsh`, `.zprezto`, `.zinit`, `.zim`,
  and `.zplug`
- `ZDOTDIR` discovery or non-default Zsh locations
- package/plugin-manager installation, shell restart, or shell re-sourcing
- arbitrary shell-script parsing, secret detection, or semantic analysis

Unsupported refs for these categories block visibly before live reads, so raw
file contents are not printed as part of the block.

## v2 selected settings: tmux config file example

The bundled `tmux` recipe manages explicit whole-file user configuration files
only:

- `tmux:home.conf` -> `~/.tmux.conf`
- `tmux:xdg.conf` -> `~/.config/tmux/tmux.conf`

These are **alternative user config locations** from tmux's own lookup rules,
not two files the manager assumes are both active. The manager syncs the exact
setting you select; it does not decide which file tmux loaded, merge them, or
inspect the running tmux server.

Select the desired tmux config file with `add`:

```bash
dotfiles-manager add tmux --setting home.conf
```

The equivalent profile-layer entry is:

```yaml
# profiles/layers/global.yaml
schema: dotfiles-manager.v2.profile-layer
schemaVersion: 1
selections:
  tmux:
    settings:
      home.conf:
        scope: user
        artifact: artifacts/home.conf
```

Then use the same sync-first selected file-resource model, using `save`/`apply` only as explicit directional aliases:

```bash
dotfiles-manager recipe explain tmux
dotfiles-manager status --user-id leon tmux:home.conf
dotfiles-manager save --dry-run --user-id leon tmux:home.conf
dotfiles-manager save --yes --user-id leon tmux:home.conf
dotfiles-manager diff --user-id leon tmux:home.conf
dotfiles-manager apply --dry-run --user-id leon tmux:home.conf
dotfiles-manager apply --yes --user-id leon tmux:home.conf
```

For user-scoped tmux files, `save --yes` writes the stored artifact to:

```text
desired/user/<user>/targets/tmux/artifacts/<setting-id>
```

For example, `--user-id leon` and `tmux:home.conf` write:

```text
desired/user/leon/targets/tmux/artifacts/home.conf
```

The corresponding URI is:

```text
desired://user/leon/targets/tmux/artifacts/home.conf
```

The stored artifact contains the raw tmux config bytes because it is the file
that will be synced later. Normal text and JSON output, diffs, ledgers, and
backup metadata stay metadata-only and do not print raw tmux config contents.

tmux loads user configuration according to tmux's own lookup rules when the
server starts. Existing servers/sessions may not observe a changed config until
you manually run an appropriate `tmux source-file ...` command or restart tmux.
The manager deliberately does not run `source-file`, restart tmux, or mutate
sessions. For `save` and `apply` plans, selected tmux config files emit this
non-blocking warning diagnostic:

```text
tmux.lifecycle.manual-reload
```

`status` and `diff` do not emit this write warning.

Missing-state behavior is fail-closed in the current whole-file slice:

- if the named location root (`~` for `tmux:home.conf`, `~/.config` for
  `tmux:xdg.conf`) is missing, status/diff/save/apply block rather than
  creating it;
- if the selected live config file is missing, `save --dry-run` / `save --yes`
  block and do not delete or tombstone stored settings;
- if the selected live config file is missing, `apply --dry-run` / `apply --yes`
  also block in this slice rather than creating the file or intermediate
  directories;
- if the stored artifact is missing, `apply --dry-run` / `apply --yes` block
  and do not delete or tombstone live state.

The bundled recipe deliberately does **not** manage:

- system tmux configuration files;
- tmux server sockets, clients, sessions, windows, panes, or runtime state;
- plugin installation, plugin clones, plugin caches, or generated plugin state
  such as resurrect/continuum session-save files;
- history, logs, pid files, temporary files, or arbitrary generated state;
- deciding which alternative user config file tmux loaded;
- manual reload actions such as `tmux source-file`, server restart, or session
  mutation;
- tmux command parsing, semantic validation, plugin validation, or secret
  scanning.

Unsupported tmux refs outside `tmux:home.conf` and `tmux:xdg.conf` are treated
as unsupported settings and must not be resolved to filesystem paths or read.

## v2 selected settings: SSH config file example

The bundled `ssh` recipe manages only the primary OpenSSH user config file:

- `ssh:config` -> `~/.ssh/config`

It does **not** manage keys or the whole `~/.ssh` directory. Select it with
`add`:

```bash
dotfiles-manager add ssh --yes
```

The equivalent profile-layer entry is:

```yaml
# profiles/layers/global.yaml
schema: dotfiles-manager.v2.profile-layer
schemaVersion: 1
selections:
  ssh:
    settings:
      config:
        scope: user
        artifact: artifacts/config
```

Then use the same sync-first selected file-resource model, using `save`/`apply` only as explicit directional aliases:

```bash
dotfiles-manager recipe explain ssh
dotfiles-manager status --user-id leon ssh:config
dotfiles-manager save --dry-run --user-id leon ssh:config
dotfiles-manager save --yes --user-id leon ssh:config
dotfiles-manager diff --user-id leon ssh:config
dotfiles-manager apply --dry-run --user-id leon ssh:config
dotfiles-manager apply --yes --user-id leon ssh:config
```

For user-scoped SSH config, `save --yes` writes the stored artifact to:

```text
desired/user/<user>/targets/ssh/artifacts/config
```

For example:

```text
desired/user/leon/targets/ssh/artifacts/config
desired://user/leon/targets/ssh/artifacts/config
```

The stored artifact contains the raw SSH config bytes because it is the file
that will be synced later. Normal text and JSON output, diffs, ledgers, and
backup metadata stay metadata-only and do not print raw SSH config contents.

For `save` and `apply`, the recipe emits this non-blocking content-review
warning:

```text
ssh.config.review-required
```

Review `Include`, `IdentityFile`, `CertificateFile`, `LocalCommand`,
`ProxyCommand`, and `Match exec` directives before writing. The manager does not
read referenced files, so `IdentityFile ~/.ssh/id_ed25519` is allowed as a
directive, but the key file itself is not managed.

Before save/apply persists raw bytes or creates a raw backup payload, the SSH
recipe scans the bytes being persisted for obvious excluded material. It blocks
with:

```text
ssh.config.excluded-content
```

The diagnostic is metadata-only. It does not print the matched key, token,
line, or config snippet. The scanner catches obvious private-key headers,
token-like secrets, public key lines, OpenSSH certificate key lines,
known_hosts-style lines, and authorized_keys-style lines. It does not parse or
validate full SSH semantics.

Symlinked `~/.ssh/config` is blocked:

```text
ssh.config.symlink-unsupported
```

Missing-state behavior is fail-closed:

- if `~` is missing, status/diff/save/apply block;
- if live `~/.ssh/config` is missing, save blocks and does not delete stored
  settings;
- if live `~/.ssh/config` is missing, apply also blocks rather than creating
  the file or intermediate directories;
- if the stored artifact is missing, apply blocks and does not delete live
  settings.

The bundled recipe deliberately does **not** manage:

- private keys, public keys, key certificates, host keys, `known_hosts`,
  `authorized_keys`, or generated host-key state;
- ssh-agent, keychain, hardware-token state, sockets, control sockets, or
  multiplexed connection state;
- `Include` target files, `~/.ssh/config.d` trees, `IdentityFile` targets,
  `CertificateFile` targets, or `UserKnownHostsFile` targets;
- key generation, key import/export, permission repair, SSH installation,
  network access, `ssh -G`, or command execution.

Explicit excluded refs such as `ssh:keys`, `ssh:private-keys`,
`ssh:known_hosts`, `ssh:authorized_keys`, `ssh:agent`, `ssh:sockets`,
`ssh:config.d`, `ssh:includes`, `ssh:certificates`, and `ssh:host-keys` return:

```text
ssh.ref.excluded
```

They are not resolved to filesystem paths, listed, or read.

## v2 selected file-tree resources: Neovim config example

The bundled `nvim` recipe manages the Neovim configuration tree at
`~/.config/nvim` on Linux/macOS. It is a file-tree recipe: it syncs selected
files under that tree, while excluding generated state, plugin clones/caches,
session files, swap/undo/view/shada data, and common key-material filenames.

Select Neovim with `add`:

```bash
dotfiles-manager add nvim --yes
```

The equivalent profile-layer entry is:

```yaml
# profiles/layers/global.yaml
schema: dotfiles-manager.v2.profile-layer
schemaVersion: 1
selections:
  nvim:
    settings:
      config:
        scope: user
        artifact: artifacts/config
```

Then use the same sync-first selected file-tree model, using `save`/`apply` only as explicit directional aliases:

```bash
dotfiles-manager recipe explain nvim
dotfiles-manager status --user-id leon nvim:config
dotfiles-manager save --dry-run --user-id leon nvim:config
dotfiles-manager save --yes --user-id leon nvim:config
dotfiles-manager diff --user-id leon nvim:config
dotfiles-manager apply --dry-run --user-id leon nvim:config
dotfiles-manager apply --yes --user-id leon nvim:config
```

For user-scoped Neovim config, `save --yes` writes the stored artifact tree to:

```text
desired/user/<user>/targets/nvim/artifacts/config
```

For example, `--user-id leon` writes:

```text
desired/user/leon/targets/nvim/artifacts/config
```

The corresponding URI is:

```text
desired://user/leon/targets/nvim/artifacts/config
```

The stored artifact contains the raw managed config files because those are the
files that will be synced later. Normal text and JSON output, diffs, ledgers,
and backup metadata stay metadata-only and do not print raw file contents.

Missing-state behavior is explicit:

- if the named location root (`~/.config` by default) is missing, `status`,
  `diff`, `save`, and `apply` block rather than creating the parent location;
- if `~/.config/nvim` is missing but `~/.config` exists, `status` reports a
  missing live tree;
- `save --dry-run` / `save --yes` block when the live tree is missing and do not
  delete or tombstone an existing stored artifact;
- `apply --dry-run` previews creating the live tree when a stored artifact
  exists;
- `apply --yes` may create `~/.config/nvim`, records an absent-tree backup, and
  verifies the result.

File-tree apply reconciles the whole managed backing tree, not only one semantic
setting. If a managed live path exists under `~/.config/nvim` but is absent from
the stored artifact, `apply --dry-run` reports that pending removal before
confirmation. Text output shows up to 20 removal paths and then points to
`--json`; JSON output includes the full untruncated list in
`items[].fileTree.operations[]` with slash-relative paths, metadata-only entry
kinds, and `planned` / `applied` state. File contents are still hidden.

Restore is also a whole-tree operation for file-tree backups. Preview restore
before confirming: restoring a file-tree backup writes the backed-up managed tree
state for that resource and can remove managed live paths that are not present in
the backup payload.

The bundled recipe deliberately does **not** manage Neovim installation, plugin
installation, package-manager actions, runtime RPC, non-default `NVIM_APPNAME`
or process `XDG_CONFIG_HOME` locations, semantic Lua/Vimscript validation, or
secret scanning. The bundled default live path is the HOME-relative
`~/.config/nvim`; a non-default live root requires an explicit named location
override and is not inferred from the manager process environment. A missing
config tree is **not** treated as proof that Neovim is not installed.

## v2 selected whole-file and file-tree resources

The v2 selected command flow also supports whole-file and file-tree recipe
resources. These resources are selected by a recipe, not scalar keys inside
`settings.yaml`.

For a selected file or file-tree setting, the default stored artifact is:

```text
desired/<scope>/<subject>/targets/<target>/artifacts/<setting-id>
```

For example, a user-scoped `test.files:config` setting for `--user-id leon`
uses:

```text
desired/user/leon/targets/test.files/artifacts/config
```

The corresponding URI is:

```text
desired://user/leon/targets/test.files/artifacts/config
```

Normal commands follow the same sync-first model as selected scalar settings:

```bash
dotfiles-manager status --user-id leon test.files:config
dotfiles-manager save --dry-run --user-id leon test.files:config
dotfiles-manager save --yes --user-id leon test.files:config
dotfiles-manager diff --user-id leon test.files:config
dotfiles-manager apply --dry-run --user-id leon test.files:config
dotfiles-manager apply --yes --user-id leon test.files:config
```

`save --yes` copies the current live file bytes or managed tree entries into the
stored artifact after preview. `apply --yes` backs up the live file/tree, writes
the stored artifact to the live path, and verifies the result. For file-tree
resources, apply reconciles the whole managed backing tree: live managed paths
that are absent from the stored artifact are removed after backup.

Diff and normal command output are metadata-only for file and file-tree resources
in this slice: they show refs, paths, existence, size/count/hash metadata, change
kind, and backup/ledger refs, but they do not print raw file contents. The
stored artifact itself contains the raw bytes because that is the state to apply
later.

For file-tree `apply`, text output makes removals explicit before confirmation:
it shows up to 20 removal paths and says when more paths are omitted. Use
`--json` to see the full, untruncated `items[].fileTree.operations[]` list. Each
operation path is slash-relative to the managed file-tree root; operation entries
contain action (`create`, `update`, `remove`), kind (`file`, `directory`), and
state (`planned`, `applied`) only, never file contents or hashes.
The current file-tree driver supports only regular files and directories;
symlinks and other non-regular entries fail closed before operation entries are
emitted.

Delete/tombstone behavior is intentionally not supported yet. A missing live file
or tree blocks `save`; it does not remove an existing stored artifact. A missing
stored artifact blocks `apply`; it does not delete the live file/tree. Selected
single-file apply still requires an existing live file so the pre-mutation backup
has a concrete file. Selected file-tree apply may create a missing live tree when
the named location root exists; the backup records the tree as absent.

File-tree restore is also whole-tree restore from the backup payload. Always run
`restore <run-id> --dry-run` first: confirming restore can remove managed live
paths that are absent from the backup tree being restored.

## v2 sync: safe settings execution

`sync [ref]` checks the current state and runs only safe one-sided settings changes.
It is the normal mutating command for copying accepted changes between live
settings and stored settings.

A safe one-sided change means one side changed and the other side still matches
the previous trusted baseline:

- `live settings -> stored settings` when the app's live settings changed;
- `stored settings -> live settings` when the stored settings changed.

Examples:

```bash
dotfiles-manager status --user-id leon git:user.email
dotfiles-manager diff --user-id leon git:user.email
dotfiles-manager sync --user-id leon git:user.email
dotfiles-manager sync --yes --user-id leon git:user.email
dotfiles-manager sync --non-interactive --yes --json --user-id leon git:user.email
```

Interactive `sync` prompts once for the whole accepted write set. The default
answer is no. Use `--yes` only after reviewing the current status or diff.
`--non-interactive` without `--yes` refuses any write and reports that
confirmation is required.

Conflicts and first-time settings are not changed automatically. `sync` reports
that a choice is needed, leaves values hidden, and does not guess which side
should win.

The settings folder is local storage. It can be versioned and shared with Git,
but Git is not required for `sync`.

## `[path]` scoping

`[path]` can be absolute, `~`-based, or relative.

A sync is selected only when `[path]` is:
- exactly the sync target, or
- inside the sync target subtree

Target matching uses post-expansion target roots (after `$VAR`/`${VAR}` resolution).

If `[path]` matches no syncs, command fails.

## Output and exit codes

- `version`/`--version` output one line:
  `dotfiles-manager version=<version> commit=<sha> date=<utc-rfc3339-z> channel=<stable|prerelease|snapshot|dev> provenance=<source>`.
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
