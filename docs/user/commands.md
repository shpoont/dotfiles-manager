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
dotfiles-manager [--config <dotfiles-manager.v2.yaml>] add <target> [--setting <id>] [--scope <scope>] [--profile <layer>] [--dry-run] [--yes] [--non-interactive] [--json]
dotfiles-manager [--config <dotfiles-manager.v2.yaml>] app create <target-id> --template <file|selected-value|native-export> ...
dotfiles-manager [--config <dotfiles-manager.v2.yaml>] app validate <target-id> [--json]
dotfiles-manager [--config <dotfiles-manager.v2.yaml>] app test <target-id> --roundtrip [--fixture <name>] [--json]
dotfiles-manager recipe list [--json]
dotfiles-manager recipe discover [target] [--json]
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
desired artifacts, backups, ledgers, profile selections, native export/import
commands, or target runtime state. It only performs PATH command lookups and
lstat-style metadata checks of declared live config paths.

Summary states are `unsupported-platform`, `ambiguous`, `config-present`,
`installed`, `config-missing`, and `not-applicable`. JSON also includes separate
`platformState`, `binaryState`, and `configState` axes plus metadata-only command
and config probes.

`custom.files` reports `not-applicable` because it has no app binary or fixed
bundled live config path to discover.

## v2 add: select a supported target

`add` updates the active v2 profile layer so later `status`, `save`, `diff`,
and `apply` know which supported settings to manage. It does **not** import
values, write desired artifacts, write live app files, create backups, or update
ledgers.

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
as `artifact: artifacts/config` so desired state is stored as an artifact
payload. For scalar settings, the profile entry can remain scope-only and later
`save` stores the value in `settings.yaml`.

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
the real desired root, trust records, backups, or ledgers.

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

Then use the same selected-value workflow:

```bash
dotfiles-manager recipe explain starship
dotfiles-manager status --user-id leon starship:add_newline
dotfiles-manager save --dry-run --user-id leon starship:add_newline
dotfiles-manager save --yes --user-id leon starship:add_newline
dotfiles-manager diff --user-id leon starship:add_newline
dotfiles-manager apply --dry-run --user-id leon starship:add_newline
dotfiles-manager apply --yes --user-id leon starship:add_newline
```

For user-scoped Starship settings, `save --yes` writes desired state to:

```text
desired/user/<user>/targets/starship/settings.yaml
```

For example, `--user-id leon` writes
`desired/user/leon/targets/starship/settings.yaml`.

This slice manages only the four root-level TOML keys above. It does not yet
auto-discover `STARSHIP_CONFIG` non-default locations, manage shell init,
install Starship, or manage full-file Starship configuration with comments,
palettes, modules, presets, custom commands, or formatting. TOML selected-key
apply may canonicalize/reformat the file and may not preserve comments. Use a
whole-file recipe resource when you want byte-preserving file management instead
of selected TOML-key management.

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

Then use the same selected file-resource workflow:

```bash
dotfiles-manager recipe explain zsh
dotfiles-manager status --user-id leon zsh:zshrc
dotfiles-manager save --dry-run --user-id leon zsh:zshrc
dotfiles-manager save --yes --user-id leon zsh:zshrc
dotfiles-manager diff --user-id leon zsh:zshrc
dotfiles-manager apply --dry-run --user-id leon zsh:zshrc
dotfiles-manager apply --yes --user-id leon zsh:zshrc
```

For user-scoped Zsh files, `save --yes` writes the desired artifact to:

```text
desired/user/<user>/targets/zsh/artifacts/<setting-id>
```

For example, `--user-id leon` and `zsh:zshrc` write:

```text
desired/user/leon/targets/zsh/artifacts/zshrc
```

The desired artifact contains the raw file bytes because it is the file that
will be applied later. Normal text and JSON output, diffs, ledgers, and backup
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

Then use the same selected file-resource workflow:

```bash
dotfiles-manager recipe explain tmux
dotfiles-manager status --user-id leon tmux:home.conf
dotfiles-manager save --dry-run --user-id leon tmux:home.conf
dotfiles-manager save --yes --user-id leon tmux:home.conf
dotfiles-manager diff --user-id leon tmux:home.conf
dotfiles-manager apply --dry-run --user-id leon tmux:home.conf
dotfiles-manager apply --yes --user-id leon tmux:home.conf
```

For user-scoped tmux files, `save --yes` writes the desired artifact to:

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

The desired artifact contains the raw tmux config bytes because it is the file
that will be applied later. Normal text and JSON output, diffs, ledgers, and
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
  block and do not delete or tombstone desired state;
- if the selected live config file is missing, `apply --dry-run` / `apply --yes`
  also block in this slice rather than creating the file or intermediate
  directories;
- if the desired artifact is missing, `apply --dry-run` / `apply --yes` block
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

Then use the same selected file-resource workflow:

```bash
dotfiles-manager recipe explain ssh
dotfiles-manager status --user-id leon ssh:config
dotfiles-manager save --dry-run --user-id leon ssh:config
dotfiles-manager save --yes --user-id leon ssh:config
dotfiles-manager diff --user-id leon ssh:config
dotfiles-manager apply --dry-run --user-id leon ssh:config
dotfiles-manager apply --yes --user-id leon ssh:config
```

For user-scoped SSH config, `save --yes` writes the desired artifact to:

```text
desired/user/<user>/targets/ssh/artifacts/config
```

For example:

```text
desired/user/leon/targets/ssh/artifacts/config
desired://user/leon/targets/ssh/artifacts/config
```

The desired artifact contains the raw SSH config bytes because it is the file
that will be applied later. Normal text and JSON output, diffs, ledgers, and
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
- if live `~/.ssh/config` is missing, save blocks and does not delete desired
  state;
- if live `~/.ssh/config` is missing, apply also blocks rather than creating
  the file or intermediate directories;
- if the desired artifact is missing, apply blocks and does not delete live
  state.

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

Then use the same selected file-tree workflow:

```bash
dotfiles-manager recipe explain nvim
dotfiles-manager status --user-id leon nvim:config
dotfiles-manager save --dry-run --user-id leon nvim:config
dotfiles-manager save --yes --user-id leon nvim:config
dotfiles-manager diff --user-id leon nvim:config
dotfiles-manager apply --dry-run --user-id leon nvim:config
dotfiles-manager apply --yes --user-id leon nvim:config
```

For user-scoped Neovim config, `save --yes` writes the desired artifact tree to:

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

The desired artifact contains the raw managed config files because those are the
files that will be applied later. Normal text and JSON output, diffs, ledgers,
and backup metadata stay metadata-only and do not print raw file contents.

Missing-state behavior is explicit:

- if the named location root (`~/.config` by default) is missing, `status`,
  `diff`, `save`, and `apply` block rather than creating the parent location;
- if `~/.config/nvim` is missing but `~/.config` exists, `status` reports a
  missing live tree;
- `save --dry-run` / `save --yes` block when the live tree is missing and do not
  delete or tombstone an existing desired artifact;
- `apply --dry-run` previews creating the live tree when a desired artifact
  exists;
- `apply --yes` may create `~/.config/nvim`, records an absent-tree backup, and
  verifies the result.

The bundled recipe deliberately does **not** manage Neovim installation, plugin
installation, package-manager actions, runtime RPC, non-default `NVIM_APPNAME`
or `XDG_CONFIG_HOME` locations, semantic Lua/Vimscript validation, or secret
scanning. A missing config tree is **not** treated as proof that Neovim is not
installed.

## v2 selected whole-file and file-tree resources

The v2 selected command flow also supports whole-file and file-tree recipe
resources. These resources are selected by a recipe, not scalar keys inside
`settings.yaml`.

For a selected file or file-tree setting, the default desired artifact is:

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

Normal commands are the same as selected scalar settings:

```bash
dotfiles-manager status --user-id leon test.files:config
dotfiles-manager save --dry-run --user-id leon test.files:config
dotfiles-manager save --yes --user-id leon test.files:config
dotfiles-manager diff --user-id leon test.files:config
dotfiles-manager apply --dry-run --user-id leon test.files:config
dotfiles-manager apply --yes --user-id leon test.files:config
```

`save --yes` copies the current live file bytes or managed tree entries into the
desired artifact after preview. `apply --yes` backs up the live file/tree, writes
the desired artifact to the live path, and verifies the result.

Diff and normal command output are metadata-only for file and file-tree resources
in this slice: they show refs, paths, existence, size/count/hash metadata, change
kind, and backup/ledger refs, but they do not print raw file contents. The
desired artifact itself contains the raw bytes because that is the state to apply
later.

Delete/tombstone behavior is intentionally not supported yet. A missing live file
or tree blocks `save`; it does not remove an existing desired artifact. A missing
desired artifact blocks `apply`; it does not delete the live file/tree. Selected
single-file apply still requires an existing live file so the pre-mutation backup
has a concrete file. Selected file-tree apply may create a missing live tree when
the named location root exists; the backup records the tree as absent.

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
