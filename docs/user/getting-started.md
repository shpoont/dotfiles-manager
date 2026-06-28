# Getting started with v2

Start with the safe temporary-home workflow. It exercises the real v2 command
surface without touching your real `~/.gitconfig`. The normal v2 workflow is
`status -> diff -> sync`; `save` and `apply` appear only when you choose an
explicit sync direction.

If you have not installed the CLI yet, see
[`install-and-release.md`](./install-and-release.md).

## 1) Verify the binary

Use `dotfiles-manager` if it is installed on `PATH`, or set `DFM` to a local
source build.

```bash
DFM=${DFM:-dotfiles-manager}
"$DFM" version
"$DFM" init --help
"$DFM" sync --help
"$DFM" save --help
"$DFM" apply --help
```

## 2) Safe quickstart using a temporary HOME

This workflow creates a temporary settings folder and a temporary home
directory. The only live file it mutates is the temporary
`$DFM_HOME/.gitconfig`.

```bash
DFM=${DFM:-dotfiles-manager}
DFM_DEMO_ROOT=$(mktemp -d)
DFM_HOME="$DFM_DEMO_ROOT/home"
DFM_SETTINGS_FOLDER="$DFM_DEMO_ROOT/settings"
mkdir -p "$DFM_HOME" "$DFM_SETTINGS_FOLDER"

HOME="$DFM_HOME" git config --global user.email first@example.test
HOME="$DFM_HOME" git config --global user.name "First User"

cd "$DFM_SETTINGS_FOLDER"
HOME="$DFM_HOME" "$DFM" init --machine-id docs-machine --user-id docs-user
```

`init` creates the v2 settings-folder scaffold:

```text
dotfiles-manager.v2.yaml
profiles/stacks/default.yaml
profiles/layers/global.yaml
```

It also writes local identity state under the platform-specific v2 local state
root described in [`configuration.md`](./configuration.md); it does not store
identity under the settings folder by default.

### Inspect available Git support

```bash
HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml list
HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml explain git
```

`explain git` should show `git:user.email` and `git:user.name`, and it should
state that credential sections, signing keys, includes, aliases, and
repository-local `.git/config` are not managed.

### Select one setting

```bash
HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
  add git --setting user.email --scope user --profile global --yes

HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
  list --settings --user-id docs-user
```

The selected setting is `git:user.email`. With `--scope user`, stored settings
are resolved for the logical user id `docs-user`, not for one machine only.

### Preview the first sync into stored settings

The normal mental model is sync between live settings and stored settings. For
the very first copy, there are no stored settings yet, so choose an explicit
direction with the `save` compatibility alias:

```text
save = sync live settings -> stored settings
```

Preview first:

```bash
HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
  status --user-id docs-user git:user.email

HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
  save --dry-run --user-id docs-user git:user.email
```

If the dry run says it would sync the live value to stored settings, confirm the
save:

```bash
HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
  save --yes --user-id docs-user git:user.email
```

The value is now stored in the settings folder at:

```text
desired/user/docs-user/targets/git/settings.yaml
```

That file contains the actual email value because the manager needs an actual
stored value to sync later. Normal command output keeps the raw value redacted.

### Preview the opposite direction

Change the temporary live Git config to create drift, then preview syncing the
stored value back to the temporary live file. Use the `apply` compatibility
alias when you want the explicit stored-settings-to-live-settings direction:

```text
apply = sync stored settings -> live settings
```

```bash
HOME="$DFM_HOME" git config --global user.email changed@example.test

HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
  diff --user-id docs-user git:user.email

HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
  apply --dry-run --user-id docs-user git:user.email

HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
  apply --yes --user-id docs-user git:user.email
```

When `apply --yes` writes live settings, normal output keeps the raw email value
redacted.

Clean up the demo when finished:

```bash
rm -rf "$DFM_DEMO_ROOT"
```

## 3) Use your real Git config

Only do this after the temporary-home workflow is clear. These commands can
read from and write to your real `~/.gitconfig`.

```bash
mkdir -p ~/dotfiles-manager-v2
cd ~/dotfiles-manager-v2

dotfiles-manager init --machine-id <your-machine-id> --user-id <your-user-id>
dotfiles-manager --config dotfiles-manager.v2.yaml explain git

dotfiles-manager --config dotfiles-manager.v2.yaml \
  add git --setting user.email --scope user --profile global --dry-run

dotfiles-manager --config dotfiles-manager.v2.yaml \
  add git --setting user.email --scope user --profile global --yes

dotfiles-manager --config dotfiles-manager.v2.yaml \
  save --dry-run --user-id <your-user-id> git:user.email
```

If the `save --dry-run` output is what you expect, confirm the explicit
live-settings-to-stored-settings sync:

```bash
dotfiles-manager --config dotfiles-manager.v2.yaml \
  save --yes --user-id <your-user-id> git:user.email
```

Before syncing stored settings back to real `~/.gitconfig`, preview first:

```bash
dotfiles-manager --config dotfiles-manager.v2.yaml \
  apply --dry-run --user-id <your-user-id> git:user.email
```

Only confirm `apply --yes` when the stored-settings path, live path, and
selected ref are exactly what you intend:

```bash
dotfiles-manager --config dotfiles-manager.v2.yaml \
  apply --yes --user-id <your-user-id> git:user.email
```

The recommended day-to-day command is `sync`; use `save` or `apply` only when
you need to force one explicit direction.

## 4) Next targets

After Git, inspect other supported apps/tools with:

```bash
dotfiles-manager list
dotfiles-manager search starship
dotfiles-manager explain starship
dotfiles-manager explain zsh
dotfiles-manager explain tmux
dotfiles-manager explain ssh
dotfiles-manager explain nvim
```

Read each app's exclusions before adding it. Do not select files that contain
secrets, private keys, tokens, generated caches, or app/account exports unless a
reviewed recipe explicitly says that item is supported.
