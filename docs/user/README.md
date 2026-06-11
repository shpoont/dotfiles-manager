# User documentation

`dotfiles-manager` v2 is a local settings manager. It lets you choose a
supported target, preview the live value or files, save desired state into a
repository, apply that desired state on another machine/profile, and recover
from local backups when a write changes live state.

If you are new, start here:

1. [`install-and-release.md`](./install-and-release.md) — install paths,
   verification, and release safety notes.
2. [`getting-started.md`](./getting-started.md) — safe temporary-home
   quickstart and the real Git-config workflow.
3. [`configuration.md`](./configuration.md) — v2 repository layout, profiles,
   scopes, desired data, and local state.
4. [`commands.md`](./commands.md) — command reference and examples.
5. [`faq.md`](./faq.md) — practical answers, recovery, limitations, and legacy
   compatibility.

## Happy path

The normal v2 workflow is:

```text
install -> init -> discover/explain -> add -> status -> save --dry-run ->
save --yes -> diff/status -> apply --dry-run -> apply --yes -> backup/restore
```

Important safety rule: preview before every write. Commands that write live
state or desired state require an explicit confirmation flag such as `--yes`.

## Current supported surface

All bundled v2 support is still marked `experimental`, but these surfaces are
implemented in the current tranche. Some paths also have internal dogfood
evidence; see the internal engineering docs for gate details.

| Surface | Current support | Stores desired data as | Important exclusions |
| --- | --- | --- | --- |
| Git | Selected non-credential identity values: `git:user.email`, `git:user.name` in `~/.gitconfig` | `desired/<scope>/<subject>/targets/git/settings.yaml` | credentials, signing keys, includes, aliases, repository-local `.git/config` |
| Starship | Selected root TOML values: `add_newline`, `command_timeout`, `follow_symlinks`, `scan_timeout` | `desired/<scope>/<subject>/targets/starship/settings.yaml` | full-file formatting/comments, modules, custom command output, non-default `STARSHIP_CONFIG` |
| Zsh | Selected startup files: `.zshrc`, `.zprofile`, `.zlogin`, `.zlogout` | `desired/<scope>/<subject>/targets/zsh/artifacts/...` | `.zshenv`, history, sessions, plugin-manager state, caches, shell restart/re-source |
| tmux | Explicit user config files: `~/.tmux.conf` or `~/.config/tmux/tmux.conf` | `desired/<scope>/<subject>/targets/tmux/artifacts/...` | sessions, sockets, plugins, generated state, `tmux source-file`, deciding active config |
| SSH | Primary user config file `~/.ssh/config` only | `desired/<scope>/<subject>/targets/ssh/artifacts/config` | private/public keys, certificates, known_hosts, authorized_keys, agents, includes, chmod repair |
| Neovim | Default config tree `~/.config/nvim` on Linux/macOS | `desired/<scope>/<subject>/targets/nvim/artifacts/config/` | plugins, generated state, caches, swap/undo/session files, non-default `NVIM_APPNAME` |
| Local app authoring | Draft and validate local recipes plus synthetic roundtrip fixtures | `recipes/local/<target-id>/...` and fixture trees | no public recipe marketplace; native export/import and arbitrary scripts are not promoted by default |
| Legacy v1 file sync | Existing `.dotfiles-manager.yaml` `status`/`diff`/`deploy`/`import` compatibility | v1 `source` directories | v1 is file sync, not the v2 selected-settings model |

`custom.files` exists as a low-level bundled target and is used by migration and
internal dogfood flows, but public live-write adoption for arbitrary custom file
sets is not the recommended first-user path in this tranche.

## What is stored

The repository stores actual desired state so the manager can apply it later.
Examples:

- selected scalar values can be stored as raw values in
  `desired/user/<user-id>/targets/<target>/settings.yaml`;
- whole-file payloads can be stored under
  `desired/user/<user-id>/targets/<target>/artifacts/<artifact-id>`;
- file-tree payloads can be stored as directories under
  `desired/user/<user-id>/targets/<target>/artifacts/<artifact-id>/`.

Normal command output, JSON previews, reports, ledgers, and backup metadata are
metadata-oriented and redact raw selected values. Desired artifacts and backup
payloads can still contain the actual managed bytes. Treat the repository and
its local backup state as sensitive if you manage sensitive files.

## What not to store

Do not manage secrets, credentials, private keys, tokens, account exports,
generated caches, or application runtime state unless a specific reviewed recipe
explicitly says that item is supported. The current v2 surface is not a secret
manager, package manager, plugin installer, app controller, or general account
backup tool.

## Legacy v1 compatibility

The v1 file-sync commands remain available for existing `.dotfiles-manager.yaml`
configs:

- `status`
- `diff`
- `deploy`
- `import`

Use v2 docs and `dotfiles-manager.v2.yaml` for new local-settings-manager
workflows. Use v1 docs/sections only for existing source/target file syncs.

For deeper implementation/spec details, see [`../internal/README.md`](../internal/README.md).
