# End-user manual for dotfiles-manager v2

`dotfiles-manager` v2 is a local settings manager. It helps you save selected
application settings into a repository, apply those saved settings on another
machine or profile, and recover local files from backups if a write does not do
what you expected.

This manual is the best starting point if you want to understand the concepts,
workflows, and safe operating habits before managing real application settings.
If you only want to run the sandbox commands, use
[`getting-started.md`](./getting-started.md). If you need exact command syntax,
use [`commands.md`](./commands.md).

## What it does, in one screen

`dotfiles-manager` v2 manages a deliberately small, reviewed set of local app
settings.

It can:

- inspect supported apps and explain what each recipe is allowed to manage;
- track one supported setting, file, or file tree in a profile;
- save the current machine's selected setting into your saved settings repo;
- apply a saved setting from the repo onto the current machine;
- create local backup records before confirmed apply/restore writes;
- restore previous local files/settings from those backups.

It is not:

- a secrets manager;
- a whole-home-directory sync tool;
- a package manager or plugin installer;
- a general app-account export/import tool;
- a cache, session, history, runtime-state, or credential backup tool;
- a guarantee that running apps will reload after their config files change.

The safest way to learn it is to manage one harmless setting first, usually Git
identity in a temporary `HOME`, then gradually add more supported targets.

## Recommended path for new users

Use this path before touching your real dotfiles:

1. Install or build the binary and verify it has the v2 commands.
2. Run the temporary-`HOME` quickstart.
3. Inspect supported recipes with `recipe list`, `recipe discover`, and `recipe explain`.
4. Add one safe target, ideally `git:user.email`.
5. Preview saving with `save --dry-run`.
6. Confirm saving with `save --yes`.
7. Inspect `status` and `diff`.
8. Preview applying with `apply --dry-run`.
9. Confirm applying with `apply --yes` only when the plan is exactly expected.
10. Learn `backup list`, `backup show`, and `restore --dry-run` before adding
    more apps.

In short:

```text
current machine settings -> save -> saved settings repo
saved settings repo -> apply -> current machine settings
apply/restore writes -> local backup first -> write -> verify
```

## What changes my machine?

Most commands are inspection-only. Writes require explicit confirmation.

| Command or flag | What it can change |
| --- | --- |
| `version`, `recipe list`, `recipe discover`, `recipe explain`, `list`, `status`, `diff`, `backup list`, `backup show` | Read-only inspection. |
| `init` | Creates repo scaffold files and local identity state. |
| `add --dry-run` | Preview only; does not select anything. |
| `add --yes` | Updates the selected profile layer in the repo. It does not read or write app settings. |
| `save --dry-run` | Preview only; does not write desired data. |
| `save --yes` | Writes selected current-machine settings into the saved settings repo. |
| `apply --dry-run` | Preview only; does not write app settings. |
| `apply --yes` | Writes saved settings onto current machine files/settings, with backup records for supported writes. |
| `restore <run-id> --dry-run` | Preview only; does not restore files. |
| `restore <run-id> --yes` | Writes previous local backup payloads back onto current machine files/settings, with backup records for the restore write. |

Rule of thumb: if the command says `MODE: DRY RUN`, it should not write. If you
are about to use `--yes`, read the planned target, scope, subject, live path,
and backup notes first.

## The mental model

There are three places to think about:

1. **Current machine settings** — the files or values an app is using now, such
   as `~/.gitconfig` or `~/.tmux.conf`.
2. **Saved settings repo** — the repository where v2 stores selected desired
   values and artifacts under `desired/`.
3. **Local backups and history** — per-machine state outside the repo, used to
   record writes and keep backup payloads for recovery.

A normal successful lifecycle looks like this:

```text
1. You change an app locally.
2. You run status/diff to inspect what v2 sees.
3. You run save --dry-run to preview saving that selected setting.
4. You run save --yes to store the selected current-machine value in the repo.
5. You commit and push/pull that repo using your normal Git workflow.
6. On another machine or profile, you run apply --dry-run to preview the write.
7. You run apply --yes to write the saved value locally.
8. If the write was wrong, you inspect backups and restore the previous local state.
```

The repo stores actual desired data so the manager can apply it later. Normal
CLI output redacts raw selected values, but files under `desired/` and local
backup payloads can contain the actual managed bytes. Even non-secret config can
reveal hostnames, usernames, paths, and internal systems. Review before pushing
to a public repository, and do not intentionally manage secrets unless a reviewed
recipe explicitly supports that exact item.

## Concepts at a glance

The most important concepts are the links between nouns, not the nouns by
themselves:

```text
recipe -> supported target/settings/resources
profile -> which supported settings this repo manages
scope + identity -> where the saved value belongs in desired/
named location -> where the current machine's live file is
save -> live file/value to desired repo
apply -> desired repo to live file/value, with backup first
restore -> backup payload back to live file/value
```

Read command output with this question in mind: "Which supported setting is
selected, where is its desired value stored, and which live file would be read or
written on this machine?"

| Concept | Plain meaning | Example |
| --- | --- | --- |
| Target | The app/config surface v2 knows about. | `git`, `starship`, `tmux` |
| Recipe | The reviewed rule for a target: settings, paths, drivers, lifecycle notes, exclusions. | Bundled `starship` recipe |
| Setting ref | The user-facing name of one manageable setting/resource. | `starship:add_newline` |
| Resource | The backing thing the recipe reads/writes. | `starship.toml` key `add_newline` |
| Driver | The parser/writer used for the backing resource. | `toml-file`, `ini-file`, `file` |
| Selection | A profile entry saying this repo should manage a setting ref. | select `git:user.email` |
| Profile layer | A named group of selections. | `global`, `work` |
| Profile stack | An ordered set of layers used together. | `default` -> `global` |
| Scope | Where the desired value belongs logically. | `user`, `shared`, `machine` |
| Subject | The concrete ID for a scope. | user `alice`, machine `alice-laptop` |
| Named location | A recipe location that maps to a live root on this machine. | `home:.gitconfig`, `config:starship.toml` |
| Desired artifact | The saved value/file/tree in the repo. | `desired/user/alice/.../settings.yaml` |
| Backup record | Local pre-write recovery data for confirmed writes. | `state://backups/<run-id>/...` |

### How the concepts relate

Suppose you want to manage `starship:add_newline`.

1. The **Starship recipe** says this is a supported setting and that it is backed
   by the root TOML key `add_newline` in `config:starship.toml`.
2. The **driver** is `toml-file`, so v2 reads and writes one TOML scalar key.
3. The **named location** `config` resolves to the Starship config root on this
   machine, normally `~/.config`.
4. The **live path** is therefore normally `~/.config/starship.toml`.
5. `add starship --setting add_newline --scope user --profile global --yes`
   writes a **selection** into the `global` profile layer.
6. `--scope user --user-id alice` means the saved value belongs under
   `desired/user/alice/...`.
7. `save --yes --user-id alice starship:add_newline` reads the live TOML value
   and stores it in the repo.
8. `apply --yes --user-id alice starship:add_newline` backs up the current live
   file, writes the saved value back to the live TOML file, and records the run.

### How to read `recipe explain`

`recipe explain <target>` is the safest way to understand a target before
selecting it. For example, `dotfiles-manager recipe explain starship` prints
metadata like:

```text
target: starship
settings:
  starship:add_newline resource=add_newline driver=toml-file scope=user
resources:
  add_newline driver=toml-file location=config path=starship.toml selector=add_newline
safety:
  do not manage: STARSHIP_CONFIG non-default locations
```

Read that as:

- `target: starship` — this recipe is for the Starship app/config surface.
- `starship:add_newline` — this is the setting ref you can pass to `add`,
  `status`, `save`, `diff`, and `apply`.
- `scope=user` — if you do not choose another scope, the saved value is per
  logical user.
- `resource=add_newline` — the setting is backed by the recipe resource named
  `add_newline`.
- `driver=toml-file` — v2 will use the TOML selected-value driver.
- `location=config path=starship.toml` — the live file is the Starship config
  file under the recipe's `config` root.
- `selector=add_newline` — only that root TOML key is selected, not the whole
  file.
- `do not manage` lines — these are hard boundaries. Do not assume excluded
  files, settings, secrets, caches, or non-default locations are safe.

If a recipe explanation is unclear, stop at `recipe explain` and do not add the
target yet.

### What a local recipe file looks like

Bundled recipes are shipped with the manager, but advanced users can create
local recipes in the saved settings repo. A local recipe is a YAML file:

```text
recipes/local/<target-id>/recipe.yaml
```

For example, this is the shape of a recipe that manages one YAML value,
`user.email`, inside `~/.config/my-app/config.yaml`:

```yaml
schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: local-my-app
displayName: "Local My App"
supportLevel: experimental
capability: read-write
locations:
  home:
    default: "~"
settings:
  user.email:
    label: "User email"
    supportLevel: experimental
    capability: read-write
    artifactForm: scalar
    sensitivity: personal
    redaction: redacted-for-display
    lifecycle: allowed
    scopeDefault: user
    resource: user-email-value
resources:
  user-email-value:
    driver: yaml-file
    location: home
    path: ".config/my-app/config.yaml"
    capability: read-write
    sensitivity: personal
    redaction: redacted-for-display
    lifecycle: allowed
    selector:
      path:
        - "user"
        - "email"
      createMissing: create
      duplicatePolicy: reject
      deleteKey: reject
```

Read this as:

- `target` is the app/config surface the user will type in commands, for example
  `local-my-app`.
- `settings` defines the public setting refs. In this example the setting ref is
  `local-my-app:user.email`.
- `scopeDefault: user` says the default saved value is per logical user.
- `resource: user-email-value` links the setting to the backing resource.
- `resources` defines the live file, driver, selector, sensitivity, redaction,
  and lifecycle behavior.
- `location: home` plus `path: ".config/my-app/config.yaml"` resolves to
  `~/.config/my-app/config.yaml` on the current machine.
- `driver: yaml-file` means v2 uses the YAML selected-value driver.
- `selector.path: ["user", "email"]` means only `user.email` is managed, not the
  whole file.

You normally do not write this from scratch. Start with `app create`, then read
and edit the generated `recipe.yaml`:

```bash
dotfiles-manager app create local-my-app \
  --template selected-value \
  --from-path ~/.config/my-app/config.yaml \
  --setting user.email \
  --setting-label "User email" \
  --driver yaml-file \
  --selector user.email \
  --scope-default user \
  --lifecycle allowed

dotfiles-manager app validate local-my-app
```

`app create` writes recipe metadata and docs only. It does not read live app
data, save desired values, select the app in a profile, or make the recipe
trusted for arbitrary writes.

## Safe first test in a temporary HOME

The full executable quickstart is in [`getting-started.md`](./getting-started.md).
This section explains why it is safe and what to look for.

The quickstart creates:

```text
/tmp/.../home   # temporary HOME with a disposable .gitconfig
/tmp/.../repo   # temporary saved settings repo
```

It then sets a fake Git email, initializes a repo, selects `git:user.email`,
previews saving, confirms saving, changes the fake live email, previews apply,
confirms apply, and inspects backups.

Good signs in the output:

- `init` says it created `dotfiles-manager.v2.yaml` and profile files.
- `add git --setting user.email ... --yes` says it selected `git:user.email`.
- `status` says the desired value is missing but current value is present.
- `save --dry-run` says `MODE: DRY RUN` and `action=would-promote`.
- `save --yes` shows a `run=...` id and `mutation=verified`.
- `apply --dry-run` says `MODE: DRY RUN` and `action=would-apply` when drift exists.
- `apply --yes` shows `mutation=verified` and a `state://backups/...` reference.

The raw email is not printed in normal command output. It is stored in the repo
under:

```text
desired/user/docs-user/targets/git/settings.yaml
```

That is expected: the manager needs the actual desired value to apply it later.

## Choosing user and machine IDs

`init` records local identity state for this saved settings repo. Choose stable
logical IDs, not secrets or throwaway hostnames. For example:

```bash
--user-id alice
--machine-id alice-laptop
```

Use the same `--user-id` on multiple machines when `--scope user` should share
values across those machines. Use different `--machine-id` values when
`machine` or `machine-user` scoped values should stay separate.

Most examples pass `--user-id` explicitly so you can see which desired subject is
being read or written. If you omit identity flags, v2 resolves them from the
local identity state recorded for this repo.

## First real workflow: manage one Git value

Use this only after the temporary-`HOME` workflow is clear. These commands read
and may eventually write your real `~/.gitconfig`.

### You are here: you want to save your current Git email into the repo

Create or enter the repo that will store desired settings. If this is a new
folder and you want normal Git review/history, initialize Git yourself;
`dotfiles-manager init` creates the v2 files but does not run `git init`:

```bash
mkdir -p ~/dotfiles-manager-v2
cd ~/dotfiles-manager-v2
git init

dotfiles-manager init --machine-id <your-machine-id> --user-id <your-user-id>
dotfiles-manager --config dotfiles-manager.v2.yaml recipe explain git
```

Read the `git` recipe explanation. It should say the current bundled support is
limited to selected non-credential identity values such as `git:user.email` and
`git:user.name`. It should not claim to manage credentials, signing keys,
includes, aliases, or repository-local `.git/config`.

Select only the email value:

```bash
dotfiles-manager --config dotfiles-manager.v2.yaml \
  add git --setting user.email --scope user --profile global --dry-run

dotfiles-manager --config dotfiles-manager.v2.yaml \
  add git --setting user.email --scope user --profile global --yes
```

`add --yes` changes the profile selection in the repo. It does not save your
email value yet and does not write `~/.gitconfig`.

Preview saving your current value:

```bash
dotfiles-manager --config dotfiles-manager.v2.yaml \
  status --user-id <your-user-id> git:user.email

dotfiles-manager --config dotfiles-manager.v2.yaml \
  save --dry-run --user-id <your-user-id> git:user.email
```

If the dry run shows the selected Git email can be promoted into desired state,
confirm:

```bash
dotfiles-manager --config dotfiles-manager.v2.yaml \
  save --yes --user-id <your-user-id> git:user.email
```

Inspect the repo change before committing it:

```bash
git diff -- desired/user/<your-user-id>/targets/git/settings.yaml
```

For a user-scoped Git email, this file contains the actual email value.

### You are here: you cloned the repo and want to apply saved Git email locally

On another machine or profile, initialize or verify local identity, then preview
apply:

```bash
cd ~/dotfiles-manager-v2

dotfiles-manager --config dotfiles-manager.v2.yaml \
  status --user-id <your-user-id> git:user.email

dotfiles-manager --config dotfiles-manager.v2.yaml \
  apply --dry-run --user-id <your-user-id> git:user.email
```

Only confirm when the selected ref, scope, subject, and live file are the ones
you intend:

```bash
dotfiles-manager --config dotfiles-manager.v2.yaml \
  apply --yes --user-id <your-user-id> git:user.email
```

A confirmed apply should show a run id and, when it writes live state, a
`state://backups/...` reference. Git config changes affect future Git commands;
they do not change old commits.

## Understanding the core terms as they become useful

You do not need to memorize every noun before using the tool. Learn them in this
order.

### Saved settings repo

The repository containing `dotfiles-manager.v2.yaml`, `profiles/`, and
`desired/`. You normally commit this repo with Git and move it between machines
using your usual Git hosting or file sync.

### Current machine settings

The app files or values on the machine where the command runs. Examples:

- `~/.gitconfig`
- `~/.config/starship.toml`
- `~/.zshrc`
- `~/.tmux.conf`
- `~/.ssh/config`
- `~/.config/nvim/`

### Supported target and recipe

A **target** is the supported app or config surface, such as `git`, `starship`,
`zsh`, `tmux`, `ssh`, or `nvim`.

A **recipe** is the reviewed rule that tells `dotfiles-manager` what that target
is allowed to manage, where it lives by default, what driver reads/writes it,
and what must not be managed.

Useful commands:

```bash
dotfiles-manager recipe list
dotfiles-manager recipe discover git
dotfiles-manager recipe explain git
```

| Command | User meaning |
| --- | --- |
| `recipe list` | Shows bundled and available targets. |
| `recipe discover <target>` | Shows what the recipe sees on this machine. |
| `recipe explain <target>` | Shows what the recipe is allowed to manage and exclude. |

If a target or setting is unsupported, expect it to be absent from the recipe
list/explanation or rejected by `add`. Do not force it with a generic file rule
unless you understand the file path, safety metadata, backups, and exclusions.

### Selection

A selection says, "this profile should manage this supported setting or file."
For example, selecting `git:user.email` tells later `status`, `save`, `diff`,
and `apply` to consider that Git email value.

`add` creates selections. `add` does not save current values and does not write
app config.

### Selected value, whole file, and file tree

Supported targets use different shapes:

- **Selected value** — one safe key inside a file, such as Git `user.email`.
- **Whole file** — a full config file, such as a selected `.zshrc` or
  `~/.ssh/config`.
- **File tree** — a directory tree, such as `~/.config/nvim`.

Whole-file and file-tree resources can contain much more data than a selected
value. Read the recipe exclusions before selecting them.

### Profile layer and profile stack

A profile layer is a named set of selections, stored in:

```text
profiles/layers/<layer>.yaml
```

A profile stack is an ordered list of layers, stored in:

```text
profiles/stacks/<stack>.yaml
```

Later layers can override earlier selections for the same target/setting.
Repeated `--profile` flags add profile layers in the order shown; they are not
profile-stack names. This lets one machine use multiple layers, such as a global
layer plus a work layer:

```bash
dotfiles-manager --config dotfiles-manager.v2.yaml \
  list --profile global --profile work --user-id alice
```

The exact profile layering model is documented in
[`configuration.md`](./configuration.md). For first use, keep the default
`global` layer until you need different personal/work or machine-specific
behavior.

### Scope: where a saved value belongs

A scope chooses where desired state is stored in the repo. It does not choose the
live file path; the recipe and named location do that.

| Scope | Use when | Repo subject |
| --- | --- | --- |
| `shared` | Everyone and every machine should get the same saved value. | `desired/shared/-/...` |
| `user` | One logical user should get the same value across machines. | `desired/user/<user-id>/...` |
| `machine` | One machine should get the same value for any user. | `desired/machine/<machine-id>/...` |
| `machine-user` | One logical user on one machine needs a unique value. | `desired/machine-user/<machine-id>/<user-id>/...` |

Examples:

- `shared`: a common Starship prompt behavior for everyone.
- `user`: Alice's Git email across all of Alice's machines.
- `machine`: a machine-specific path or hostname-related setting.
- `machine-user`: Alice on laptop A uses a setting that should not apply to
  Alice on laptop B.

## Profiles and scopes: practical examples

### One user, two machines, same Git email

Use `--scope user` for `git:user.email`:

```bash
dotfiles-manager add git --setting user.email --scope user --profile global --yes
```

Saved data goes under:

```text
desired/user/<user-id>/targets/git/settings.yaml
```

Both machines can resolve the same logical user id and apply the same saved Git
email.

### Same machine, personal and work profile layers

Use one repo with two layers:

```text
profiles/layers/global.yaml
profiles/layers/work.yaml
```

The default stack can keep `global`; work commands can add the work layer:

```bash
dotfiles-manager --config dotfiles-manager.v2.yaml \
  list --profile global --profile work --user-id alice
```

Use this pattern when the same laptop should sometimes manage personal settings
and sometimes work-specific settings.

### Shared value plus machine-specific override

A simple approach is:

- put the common selection in `global` with `scope: shared`;
- put the machine-specific selection or override in a machine layer with
  `scope: machine` or `scope: machine-user`.

Keep overrides rare and obvious. If every machine has many overrides, the repo
becomes harder to reason about.

## Supported targets in the current v2 surface

Bundled support is intentionally narrow. Always read `recipe explain <target>`
before adding a target.

| Target | Supported shape | Managed by default | Important exclusions |
| --- | --- | --- | --- |
| `git` | Selected values | `git:user.email`, `git:user.name` in `~/.gitconfig` | Credentials, signing keys, includes, aliases, URL rewrites, repository-local `.git/config`. |
| `starship` | Selected values | Selected root TOML values such as `add_newline`, `command_timeout`, `follow_symlinks`, `scan_timeout` | Modules, comments/formatting preservation, custom command output, unreviewed non-default config roots. |
| `zsh` | Whole files | Selected startup files such as `.zshrc`, `.zprofile`, `.zlogin`, `.zlogout` | `.zshenv`, shell history, sessions, plugin-manager state, caches, automatic shell reload. |
| `tmux` | Whole files | Explicit user config files such as `~/.tmux.conf` or `~/.config/tmux/tmux.conf` | Sessions, sockets, plugins, generated state, automatic `tmux source-file`. |
| `ssh` | Whole file | Primary user `~/.ssh/config` | Private/public keys, certificates, known_hosts, authorized_keys, agents, referenced `Include` target files, chmod repair. `Include` lines inside the primary file may be saved as plain text; the referenced files are not read or managed. |
| `nvim` | File tree | Default config tree `~/.config/nvim` | Plugins, generated state, caches, swap/undo/session files, unreviewed `NVIM_APPNAME` alternatives. |
| Local app authoring | Advanced local recipes | Draft/validate local recipes and synthetic roundtrip fixtures | No public marketplace, no arbitrary trusted scripts, native export/import not generally supported. |
| Legacy v1 compatibility | Existing v1 file sync | `.dotfiles-manager.yaml` workflows | This is compatibility, not the recommended v2 selected-settings model. |

Do not manage secrets, private keys, tokens, account exports, generated caches,
or runtime state unless a reviewed recipe explicitly says that exact item is
supported.

## Managing more apps and settings

### You are here: one setting worked and you want to add more

Add gradually. Prefer one small target or setting at a time:

```bash
dotfiles-manager recipe list
dotfiles-manager recipe explain starship

dotfiles-manager --config dotfiles-manager.v2.yaml \
  add starship --setting add_newline --scope user --profile global --dry-run

dotfiles-manager --config dotfiles-manager.v2.yaml \
  add starship --setting add_newline --scope user --profile global --yes

dotfiles-manager --config dotfiles-manager.v2.yaml \
  status --user-id <your-user-id>
```

Then preview saving all currently selected drift, or name one ref explicitly:

```bash
dotfiles-manager --config dotfiles-manager.v2.yaml \
  save --dry-run --user-id <your-user-id>

dotfiles-manager --config dotfiles-manager.v2.yaml \
  save --dry-run --user-id <your-user-id> starship:add_newline
```

Confirm only after the dry run is understandable:

```bash
dotfiles-manager --config dotfiles-manager.v2.yaml \
  save --yes --user-id <your-user-id> starship:add_newline
```

The same pattern applies before live writes:

```bash
dotfiles-manager --config dotfiles-manager.v2.yaml \
  apply --dry-run --user-id <your-user-id>
```

Do not bulk-adopt every app on day one. For whole-file or file-tree targets,
inspect exactly what will be stored and what the recipe excludes.

### Whole-file example: `.tmux.conf`

A whole-file target saves and applies the whole selected file, not one semantic
key inside it. For example, selecting a tmux config file means a confirmed apply
can overwrite that live config file with the saved artifact.

Safe pattern:

```bash
dotfiles-manager recipe explain tmux

dotfiles-manager --config dotfiles-manager.v2.yaml \
  add tmux --setting home.conf --scope user --profile global --dry-run

dotfiles-manager --config dotfiles-manager.v2.yaml \
  add tmux --setting home.conf --scope user --profile global --yes

dotfiles-manager --config dotfiles-manager.v2.yaml \
  save --dry-run --user-id <your-user-id> tmux:home.conf
```

Before `apply --yes`, close or plan to reload tmux yourself if that matters for
your workflow. The manager writes files; it does not promise to reload running
applications.

### File-tree example: Neovim

The Neovim target manages the config tree, not plugins, caches, swap files,
undo/session files, or generated runtime state. Before selecting it, read:

```bash
dotfiles-manager recipe explain nvim
```

After applying a Neovim config tree, you may need to restart Neovim and run your
own plugin/runtime update process. The manager does not install plugins for you.

## Non-default config locations and named locations

Recipes use named locations to turn a recipe path into a live path. Common names
include:

- `home` for paths under `$HOME`, such as `home:.gitconfig`;
- `config` for the app's default config root, such as `config:starship.toml` or
  `config:nvim`.

Named locations exist because many programs can use non-default config paths.
For example, Starship can use `STARSHIP_CONFIG`, Zsh can use `ZDOTDIR`, and
Neovim can use `NVIM_APPNAME`.

Current bundled recipes do not silently broaden discovery just because the
manager process has a different environment variable. If you use non-default
locations, the recipe needs an explicit reviewed location mapping. This prevents
a command from unexpectedly reading or writing a different config tree.

A useful mental model:

```text
same saved setting name + same profile/scope
can map to different live roots on different machines
only when the recipe/location metadata says that is allowed
```

If a non-default location is important to your setup, do not guess. Run
`recipe explain <target>` and check whether the target documents support for the
location you need.

## Backups and restore

### You are here: something changed and you want to undo it

Stop and inspect before running another write.

List backup runs:

```bash
dotfiles-manager --config dotfiles-manager.v2.yaml backup list
```

Show one run:

```bash
dotfiles-manager --config dotfiles-manager.v2.yaml backup show <run-id>
```

Preview restore:

```bash
dotfiles-manager --config dotfiles-manager.v2.yaml \
  restore <run-id> --dry-run --user-id <your-user-id>
```

Confirm only when the restore preview shows the intended live paths and changes:

```bash
dotfiles-manager --config dotfiles-manager.v2.yaml \
  restore <run-id> --yes --user-id <your-user-id>
```

For selected values backed by files, restore rolls back the whole backing file
from the backup payload. It is not a semantic single-key rollback.

Backups are stored outside the repo by default:

```text
macOS: ~/Library/Application Support/dotfiles-manager/v2/<repo-state-id>/
Linux: ${XDG_STATE_HOME:-~/.local/state}/dotfiles-manager/v2/<repo-state-id>/
```

Backup payloads can contain the previous managed bytes. Treat the local state
root as sensitive.

## Updating saved settings after editing an app locally

### You are here: you changed a setting manually and want the repo to learn it

Use the same save workflow:

```bash
dotfiles-manager --config dotfiles-manager.v2.yaml \
  status --user-id <your-user-id> <target>:<setting>

dotfiles-manager --config dotfiles-manager.v2.yaml \
  diff --user-id <your-user-id> <target>:<setting>

dotfiles-manager --config dotfiles-manager.v2.yaml \
  save --dry-run --user-id <your-user-id> <target>:<setting>
```

If the preview is right:

```bash
dotfiles-manager --config dotfiles-manager.v2.yaml \
  save --yes --user-id <your-user-id> <target>:<setting>

git diff
git add desired profiles dotfiles-manager.v2.yaml
git commit
```

Use your normal Git workflow for committing and sharing the saved settings repo.
The manager does not replace Git review.

## Conflicts and confusing states

Common situations:

- **Selected but no desired data exists.** Run `save --dry-run` if the current
  machine has the value you want to save.
- **Desired data exists but current machine differs.** Run `diff` and
  `apply --dry-run` if you want this machine to adopt the saved value.
- **Current machine has local changes you want to keep.** Run `save --dry-run`
  instead of `apply --yes`.
- **You are not sure which machine/user the value belongs to.** Check the scope,
  `--machine-id`, and `--user-id` before writing.
- **The repo has uncommitted changes.** Review them with Git before saving or
  applying more settings.
- **A previous apply created a backup.** Use `backup list` and `backup show`
  before deciding whether to restore.
- **A selected live file does not exist.** Check `recipe explain <target>` and
  `status`; some targets block save/apply instead of creating new files.
- **Desired data exists but the live file is absent.** Treat `apply --dry-run` as
  the source of truth for whether v2 would create a file or block.
- **A whole-file target exists locally but desired data is missing.** Use
  `save --dry-run` if the current local file is the version you want to store.

When in doubt, prefer read-only commands and dry runs. Do not use `--yes` to
explore.

## Stopping management of a setting

There is not yet a dedicated beginner-friendly "stop managing this setting"
workflow. The current safe approach is manual and should be reviewed:

1. Remove the selection from the relevant profile layer under `profiles/layers/`.
2. Decide whether the existing desired data under `desired/` should remain for
   history or be deleted from the repo.
3. Leave the current machine's live app files alone unless you explicitly want
   to edit them yourself.
4. Commit the repo change after review.

If you are unsure, do not delete desired data immediately. Removing a selection
is different from deleting live settings.

## When apps need to be closed or restarted

The manager writes files/settings. It does not promise to safely control every
running app process.

Before applying whole-file or file-tree targets, consider whether the app should
be stopped first. After applying, consider whether it needs a restart or reload:

- Git config affects future Git commands.
- Shell startup files affect new shells; an existing shell may need manual
  sourcing or restart.
- tmux may need manual `source-file`, server restart, or session restart.
- Neovim may need restart and separate plugin/runtime refresh.
- GUI apps may need restart if they cache settings.

A reviewed recipe should document lifecycle notes when a target has special
requirements. If the lifecycle behavior is unclear, preview the write and stop
rather than guessing.

## Advanced and deferred areas

### Custom local recipes

Advanced users can draft local recipes under:

```text
recipes/local/<target-id>/
```

Use this only when you understand the file paths, safety metadata, driver,
backup behavior, and exclusions. `app validate` and `app test --roundtrip` are
for recipe authoring and synthetic fixtures; they are not a public recipe
marketplace and do not make arbitrary scripts safe.

### Native app export/import

Some apps, such as Raycast-like tools, may provide native export/import
commands. v2 does not treat native export/import as generally supported just
because an app has such commands.

A native export/import recipe needs reviewed metadata for:

- exact commands and arguments;
- where exported data is stored;
- what secrets or account data may be included;
- whether the app must be stopped before import;
- how backups and restore work;
- how tests prove the workflow is safe.

Until that exists for a target, assume native export/import is unsupported.

### Experimental guided sync

The `sync` command exists as an experimental advanced shortcut. It is not the
stable beginner happy path and it is not a blind merge. New users should use
explicit `status`, `diff`, `save --dry-run`, `save --yes`, `apply --dry-run`,
and `apply --yes`.

If command output mentions `sync` as a possible next command, you can ignore it
while following this manual.

### Legacy v1 compatibility

Legacy v1 file-sync commands still exist for existing `.dotfiles-manager.yaml`
configs. New v2 users should start with `dotfiles-manager.v2.yaml`, profiles,
scopes, selected settings, desired artifacts, backups, and restore.

See [`configuration.md`](./configuration.md) and [`commands.md`](./commands.md)
when you need v1 compatibility details.

## When you see `desired://`, `state://`, or `recipe://`

CLI output may show internal references:

- `desired://...` points to saved desired repo data;
- `state://...` points to local state such as backups or identity files;
- `recipe://...` points to recipe metadata.

These are stable references in reports, not shell paths to paste into `cd`.
Use the matching command, such as `backup show`, `recipe explain`, `list --json`,
or the repo layout in [`configuration.md`](./configuration.md), to inspect more.

## Before using your real HOME: checklist

Do this before confirmed writes against real app config:

- Verify the binary:

  ```bash
  dotfiles-manager version
  dotfiles-manager init --help
  dotfiles-manager add --help
  dotfiles-manager save --help
  dotfiles-manager apply --help
  ```

- Confirm you are in the intended saved settings repo.
- Use explicit `--config dotfiles-manager.v2.yaml` in scripts.
- Confirm `--machine-id` and `--user-id` are the logical identities you expect.
- Run `recipe explain <target>` and read exclusions.
- Run `status` and `diff` before writes.
- Run `save --dry-run` or `apply --dry-run` before `--yes`.
- Know how to run `backup list`, `backup show`, and `restore --dry-run`.
- Close or prepare to restart apps when the recipe/lifecycle notes require it.
- Commit and review repo changes after saving desired data.

## Where to go next

- Run the executable sandbox: [`getting-started.md`](./getting-started.md).
- Install or verify a release: [`install-and-release.md`](./install-and-release.md).
- Learn repo layout, scopes, profiles, named locations, and local state:
  [`configuration.md`](./configuration.md).
- Look up exact commands and target examples: [`commands.md`](./commands.md).
- Find short answers: [`faq.md`](./faq.md).
