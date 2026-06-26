# User documentation

`dotfiles-manager` v2 is a local settings manager. It lets you choose a
supported app/tool, compare live settings with stored settings in a local
settings folder, and sync safe changes between them.

`sync` is the primary v2 command. `save` and `apply` are retained only as
directional compatibility aliases:

- `save` = sync live settings -> stored settings;
- `apply` = sync stored settings -> live settings.

If you are new, start here:

1. [`install-and-release.md`](./install-and-release.md) — install paths,
   verification, and release safety notes.
2. [`getting-started.md`](./getting-started.md) — safe temporary-home
   quickstart and the real Git-config workflow.
3. [`configuration.md`](./configuration.md) — v2 settings-folder layout, profiles,
   scopes, stored settings, and local state.
4. [`commands.md`](./commands.md) — command reference and examples.
5. [`faq.md`](./faq.md) — practical answers, recovery, and limitations.

## Happy path

The normal v2 workflow is:

```text
install -> init -> discover/explain -> add -> status -> diff -> sync
```

Important safety rule: inspect before every write. Commands that write live
settings or stored settings require an explicit confirmation flag such as
`--yes`.

## Current supported surface

All bundled v2 support is still marked `experimental`, but these surfaces are
implemented in the current tranche. Some paths also have internal dogfood
evidence; see the internal engineering docs for gate details.

| Surface | Current support | Stores settings as | Important exclusions |
| --- | --- | --- | --- |
| Git | Selected non-credential identity values: `git:user.email`, `git:user.name` in `~/.gitconfig` | `desired/<scope>/<subject>/targets/git/settings.yaml` in the settings folder | credentials, signing keys, includes, aliases, repository-local `.git/config` |
| Starship | Selected root TOML values: `add_newline`, `command_timeout`, `follow_symlinks`, `scan_timeout` in `~/.config/starship.toml` | `desired/<scope>/<subject>/targets/starship/settings.yaml` in the settings folder | full-file formatting/comments, modules, custom command output, non-default `STARSHIP_CONFIG` or process `XDG_CONFIG_HOME` without explicit location override |
| Zsh | Selected startup files: `.zshrc`, `.zprofile`, `.zlogin`, `.zlogout` | `desired/<scope>/<subject>/targets/zsh/artifacts/...` in the settings folder | `.zshenv`, history, sessions, plugin-manager state, caches, shell restart/re-source |
| tmux | Explicit user config files: `~/.tmux.conf` or `~/.config/tmux/tmux.conf` | `desired/<scope>/<subject>/targets/tmux/artifacts/...` in the settings folder | sessions, sockets, plugins, generated state, `tmux source-file`, deciding active config |
| SSH | Primary user config file `~/.ssh/config` only | `desired/<scope>/<subject>/targets/ssh/artifacts/config` in the settings folder | private/public keys, certificates, known_hosts, authorized_keys, agents, includes, chmod repair |
| Neovim | Default config tree `~/.config/nvim` on Linux/macOS | `desired/<scope>/<subject>/targets/nvim/artifacts/config/` in the settings folder | plugins, generated state, caches, swap/undo/session files, non-default `NVIM_APPNAME` or process `XDG_CONFIG_HOME` without explicit location override |
| Local app authoring | Draft and validate local recipes plus synthetic roundtrip fixtures | `recipes/local/<target-id>/...` and fixture trees | no public recipe marketplace; native export/import and arbitrary scripts are not promoted by default |

`custom.files` exists as a low-level bundled target for internal/dogfood flows,
but public live-write adoption for arbitrary custom file sets is not the
recommended first-user path in this tranche.

## What is stored

The settings folder stores actual managed values and payloads so the manager can
compare them with live settings and sync them later.
Examples:

- selected scalar values can be stored as raw values in
  `desired/user/<user-id>/targets/<target>/settings.yaml`;
- whole-file payloads can be stored under
  `desired/user/<user-id>/targets/<target>/artifacts/<artifact-id>`;
- file-tree payloads can be stored as directories under
  `desired/user/<user-id>/targets/<target>/artifacts/<artifact-id>/`.

Normal command output, JSON previews, reports, and ledgers are metadata-oriented
and redact raw selected values. Stored settings files can still contain the
actual managed bytes. Treat the settings folder as sensitive if you manage
sensitive files.

## What not to store

Do not manage secrets, credentials, private keys, tokens, account exports,
generated caches, or application runtime state unless a specific reviewed recipe
explicitly says that item is supported. The current v2 surface is not a secret
manager, package manager, plugin installer, app controller, or general account
archival tool.

For deeper implementation/spec details, see [`../internal/README.md`](../internal/README.md).
