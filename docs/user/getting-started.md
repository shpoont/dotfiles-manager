# Getting started with v2

Start with the safe temporary-home workflow. It exercises the real v2 command
surface without touching your real `~/.gitconfig`.

If you have not installed the CLI yet, see
[`install-and-release.md`](./install-and-release.md).

## 1) Verify the binary

Use `dotfiles-manager` if it is installed on `PATH`, or set `DFM` to a local
source build.

```bash
DFM=${DFM:-dotfiles-manager}
"$DFM" version
"$DFM" init --help
"$DFM" save --help
"$DFM" apply --help
```

## 2) Safe quickstart using a temporary HOME

This workflow creates a temporary repository and a temporary home directory. The
only live file it mutates is the temporary `$DFM_HOME/.gitconfig`.

```bash
DFM=${DFM:-dotfiles-manager}
DFM_DEMO_ROOT=$(mktemp -d)
DFM_HOME="$DFM_DEMO_ROOT/home"
DFM_REPO="$DFM_DEMO_ROOT/repo"
mkdir -p "$DFM_HOME" "$DFM_REPO"

HOME="$DFM_HOME" git config --global user.email first@example.test
HOME="$DFM_HOME" git config --global user.name "First User"

cd "$DFM_REPO"
HOME="$DFM_HOME" "$DFM" init --machine-id docs-machine --user-id docs-user
```

`init` creates the v2 repository scaffold:

```text
dotfiles-manager.v2.yaml
profiles/stacks/default.yaml
profiles/layers/global.yaml
```

It also writes local identity state under the platform-specific v2 local state
root described in [`configuration.md`](./configuration.md); it does not store
identity under the repository by default.

### Inspect available Git support

```bash
HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml recipe discover git
HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml recipe explain git
```

`recipe explain git` should show `git:user.email` and `git:user.name`, and it
should state that credential sections, signing keys, includes, aliases, and
repository-local `.git/config` are not managed.

### Select one setting

```bash
HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
  add git --setting user.email --scope user --profile global --yes

HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
  list --user-id docs-user
```

The selected setting is `git:user.email`. With `--scope user`, desired state is
resolved for the logical user id `docs-user`, not for one machine only.

### Preview and save desired state

`save` copies the selected live value into desired state. Preview first:

```bash
HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
  status --user-id docs-user git:user.email

HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
  save --dry-run --user-id docs-user git:user.email
```

If the dry run reports `action=would-promote`, confirm the save:

```bash
HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
  save --yes --user-id docs-user git:user.email
```

The desired value is now stored in the repository at:

```text
desired/user/docs-user/targets/git/settings.yaml
```

That file contains the actual email value because the manager needs an actual
desired value to apply later. Normal command output keeps the raw value redacted.

### Preview and apply desired state

Change the temporary live Git config to create drift, then preview and apply the
saved desired value back to the temporary live file:

```bash
HOME="$DFM_HOME" git config --global user.email changed@example.test

HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
  diff --user-id docs-user git:user.email

HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
  apply --dry-run --user-id docs-user git:user.email

HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
  apply --yes --user-id docs-user git:user.email
```

When `apply --yes` writes live state, it records local backup and run-history
records under the v2 local state root. Normal output prints a `run=` id and one
or more `state://backups/...` references without printing the raw email value.

### Inspect backups and preview recovery

List backups:

```bash
HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml backup list
```

Copy the first-column run id that looks like
`selected-value-YYYYMMDDTHHMMSS.NNNNNNNNNZ`, then inspect it:

```bash
RUN_ID=selected-value-YYYYMMDDTHHMMSS.NNNNNNNNNZ
HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml backup show "$RUN_ID"
HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
  restore "$RUN_ID" --dry-run --user-id docs-user
```

Only after the dry run shows the expected live path and change, confirm restore:

```bash
HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
  restore "$RUN_ID" --yes --user-id docs-user
```

For a selected value backed by a file, restore rolls back the whole backing file
from the backup payload. It is not a semantic single-value rollback.

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
dotfiles-manager --config dotfiles-manager.v2.yaml recipe explain git

dotfiles-manager --config dotfiles-manager.v2.yaml \
  add git --setting user.email --scope user --profile global --dry-run

dotfiles-manager --config dotfiles-manager.v2.yaml \
  add git --setting user.email --scope user --profile global --yes

dotfiles-manager --config dotfiles-manager.v2.yaml \
  save --dry-run --user-id <your-user-id> git:user.email
```

If the `save --dry-run` output is what you expect, confirm:

```bash
dotfiles-manager --config dotfiles-manager.v2.yaml \
  save --yes --user-id <your-user-id> git:user.email
```

Before applying to real `~/.gitconfig`, preview first:

```bash
dotfiles-manager --config dotfiles-manager.v2.yaml \
  apply --dry-run --user-id <your-user-id> git:user.email
```

Only confirm `apply --yes` when the desired path, live path, and selected ref
are exactly what you intend:

```bash
dotfiles-manager --config dotfiles-manager.v2.yaml \
  apply --yes --user-id <your-user-id> git:user.email
```

If an apply changes the wrong live value, use `backup list`, `backup show`, and
`restore <run-id> --dry-run` before confirming `restore <run-id> --yes`.

## 4) Next targets

After Git, inspect other bundled targets with:

```bash
dotfiles-manager recipe list
dotfiles-manager recipe explain starship
dotfiles-manager recipe explain zsh
dotfiles-manager recipe explain tmux
dotfiles-manager recipe explain ssh
dotfiles-manager recipe explain nvim
```

Read each recipe's exclusions before adding it. Do not select files that contain
secrets, private keys, tokens, generated caches, or app/account exports unless a
reviewed recipe explicitly says that item is supported.
