# v2 configuration and data layout

v2 uses a small control plane in the repository, desired-state artifacts in the
repository, and local state outside the repository.

## Config loading

For v2 workflows, use the v2 root config:

```text
dotfiles-manager.v2.yaml
```

Most v2 examples pass it explicitly:

```bash
dotfiles-manager --config dotfiles-manager.v2.yaml list
```

Some preview commands can find a v2 root when the current directory is inside a
repository containing `dotfiles-manager.v2.yaml`. Mutating commands should be
run from the intended repository and should use explicit `--config` in scripts.

Legacy v1 file sync still uses `.dotfiles-manager.yaml`; see the legacy section
at the end of this file.

## Repository control plane

`dotfiles-manager init` creates this minimal repository scaffold:

```text
dotfiles-manager.v2.yaml
profiles/
  stacks/
    default.yaml
  layers/
    global.yaml
```

Example root config:

```yaml
schema: dotfiles-manager.v2.root-config
schemaVersion: 1
activeProfileStack: default
```

Example profile stack:

```yaml
schema: dotfiles-manager.v2.profile-stack
schemaVersion: 1
profileStack:
  - global
```

Example profile layer selecting one Git value:

```yaml
schema: dotfiles-manager.v2.profile-layer
schemaVersion: 1
selections:
  git:
    settings:
      user.email:
        scope: user
```

A stack is an ordered list of layers. Later layers override earlier selections
for the same `target:setting`. Commands can add extra layers with repeated
`--profile <layer>` flags, so one machine can use multiple profile layers such
as global + work + machine-local overrides.

## Scopes

Scopes choose the desired-state subject. They do not decide the live file path;
the recipe resource and named location decide live paths.

| Scope | Desired subject | Meaning |
| --- | --- | --- |
| `shared` | `desired/shared/-/...` | Same desired value for everyone and every machine. |
| `user` | `desired/user/<user-id>/...` | Same desired value for one logical user across machines. |
| `machine` | `desired/machine/<machine-id>/...` | Same desired value for everyone on one machine. |
| `machine-user` | `desired/machine-user/<machine-id>/<user-id>/...` | Value specific to one logical user on one machine. |

Examples:

```text
desired://shared/-/targets/git/settings#shared.aliases
desired://user/leon/targets/git/settings#user.email
desired://machine/mbp-2026/targets/git/settings#host.name
desired://machine-user/mbp-2026/leon/targets/git/settings#local.theme
```

## Desired data plane

Desired state is stored under `desired/` in the repository.

Selected scalar settings use `settings.yaml`:

```text
desired/user/docs-user/targets/git/settings.yaml
```

Example shape:

```yaml
schema: dotfiles-manager.v2.desired-settings
schemaVersion: 1
values:
  user.email:
    intent: set
    kind: string
    value: first@example.test
```

Whole-file resources use `artifacts/`:

```text
desired/user/docs-user/targets/tmux/artifacts/home.conf
desired/user/docs-user/targets/ssh/artifacts/config
```

File-tree resources use an artifact directory:

```text
desired/user/docs-user/targets/nvim/artifacts/config/
```

Desired artifacts may contain the actual managed bytes. Do not commit secrets,
credentials, private keys, tokens, account exports, generated caches, or runtime
state unless a reviewed recipe explicitly supports that data.

## Named locations and live paths

Recipes define named locations such as:

- `home` — paths under `$HOME`, for example `home:.gitconfig`;
- `config` — the recipe's configured config root. The bundled Starship,
  Neovim, and tmux XDG-style examples default this root to the HOME-relative
  path `~/.config`, for example `config:starship.toml` renders as
  `~/.config/starship.toml` and `config:nvim` renders as `~/.config/nvim`;
- `recipe-defined` — used by low-level custom resources.

A resource combines a named location, a path, a driver, and sometimes a
selector. Example from the Git recipe:

```text
resource=user-email driver=ini-file location=home:.gitconfig selector=[user] email
```

Named locations are intentionally explicit because some programs support
non-default config locations. Current bundled recipes document when non-default
locations are not supported by their default discovery, for example
`STARSHIP_CONFIG`, `ZDOTDIR`, `NVIM_APPNAME`, or process `XDG_CONFIG_HOME`
alternatives. A non-default live root must be represented by an explicit named
location override; setting `XDG_CONFIG_HOME` in the manager process does not
silently broaden bundled writes or discovery.

## Internal URI style

User-facing output may show internal URI schemes:

- `desired://...` points to desired repository artifacts;
- `state://...` points to local state artifacts such as backups or ledger runs;
- `recipe://...` identifies bundled or local recipe metadata.

These URIs are stable references in reports, not filesystem paths by themselves.
Use the matching command (`backup show`, `recipe explain`, `list --json`, etc.)
when you need details.

## Local state

v2 local state is outside the repository by default and is keyed by a stable hash
of the real repository path.

Default local state roots:

```text
macOS: ~/Library/Application Support/dotfiles-manager/v2/<repo-state-id>/
Linux: ${XDG_STATE_HOME:-~/.local/state}/dotfiles-manager/v2/<repo-state-id>/
```

Current state subtrees include:

```text
identity/machine.yaml
identity/users/<local-account-key>.yaml
backups/<run-id>/backup.yaml
backups/<run-id>/payloads/...
ledger/ledger.jsonl
ledger/runs/<run-id>.json
```

Normal backup and ledger metadata is redacted/metadata-oriented. Backup payloads
may contain the actual pre-apply managed bytes so restore can put them back.
Treat the local state root as sensitive.

## Custom local recipes

Advanced users can draft local recipes under:

```text
recipes/local/<target-id>/
```

Synthetic fixtures for local recipe roundtrip tests live under:

```text
recipes/local/<target-id>/fixtures/roundtrip/<fixture-name>/
```

`app create` writes recipe metadata and docs. `app validate` validates recipe
metadata. `app test --roundtrip` uses synthetic fixtures and temporary roots; it
does not touch real app config.

Native export/import and arbitrary scripts are not promoted for general public
use in this tranche. A native or command-backed recipe needs explicit reviewed
metadata and safety behavior before docs should present it as supported.

## Legacy v1 configuration

Existing file-sync workflows use `.dotfiles-manager.yaml` with `syncs[]`:

```yaml
syncs:
  - target: .config/nvim
    source: .config/nvim
```

v1 commands are still available for that config:

```bash
dotfiles-manager status
dotfiles-manager diff
dotfiles-manager deploy --dry-run
dotfiles-manager import --dry-run
```

Use `migrate --dry-run` to inspect a generated v2 migration plan. Plain
`migrate` writes generated output under `migrations/v1-to-v2/<run-id>/` for
review; it does not replace active root v2 files automatically.
