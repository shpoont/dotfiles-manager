# User documentation

`dotfiles-manager` v2 is a local settings manager. It lets you choose supported
application settings, save desired state into a repository, apply that saved
state on another machine or profile, and recover from local backups when a write
changes current machine settings.

## Start here

This page is a routing page plus a quick safety summary. The standalone user
journey lives in [`manual.md`](./manual.md).

1. [`manual.md`](./manual.md) — end-user manual for concepts, workflows, safety,
   supported targets, profiles/scopes, backups, and recovery.
2. [`getting-started.md`](./getting-started.md) — executable temporary-`HOME`
   quickstart and a real Git-config workflow.
3. [`install-and-release.md`](./install-and-release.md) — install paths,
   version checks, and release safety notes.
4. [`commands.md`](./commands.md) — exact command reference and target examples.
5. [`configuration.md`](./configuration.md) — repository layout, profiles,
   scopes, desired data, named locations, internal URI references, and local
   state.
6. [`faq.md`](./faq.md) — practical answers and recovery notes.

## Stable v2 happy path

```text
install -> init -> recipe discover/explain -> add -> status -> save --dry-run ->
save --yes -> diff/status -> apply --dry-run -> apply --yes -> backup/restore
```

Important safety rule: preview before every write. Commands that write live
state or desired state require explicit confirmation, usually `--yes`.

## Current supported surface

Bundled v2 support is intentionally narrow and still marked experimental. Read
`recipe explain <target>` before selecting a target.

| Surface | Current support | Stores desired data as | Important exclusions |
| --- | --- | --- | --- |
| Git | Selected non-credential identity values: `git:user.email`, `git:user.name` in `~/.gitconfig` | `desired/<scope>/<subject>/targets/git/settings.yaml` | credentials, signing keys, includes, aliases, repository-local `.git/config` |
| Starship | Selected root TOML values: `add_newline`, `command_timeout`, `follow_symlinks`, `scan_timeout` in `~/.config/starship.toml` | `desired/<scope>/<subject>/targets/starship/settings.yaml` | full-file formatting/comments, modules, custom command output, non-default `STARSHIP_CONFIG` without explicit reviewed location support |
| Zsh | Selected startup files: `.zshrc`, `.zprofile`, `.zlogin`, `.zlogout` | `desired/<scope>/<subject>/targets/zsh/artifacts/...` | `.zshenv`, history, sessions, plugin-manager state, caches, automatic shell restart/re-source |
| tmux | Explicit user config files: `~/.tmux.conf` or `~/.config/tmux/tmux.conf` | `desired/<scope>/<subject>/targets/tmux/artifacts/...` | sessions, sockets, plugins, generated state, automatic `tmux source-file` |
| SSH | Primary user config file `~/.ssh/config` only | `desired/<scope>/<subject>/targets/ssh/artifacts/config` | private/public keys, certificates, known_hosts, authorized_keys, agents, referenced `Include` target files, chmod repair |
| Neovim | Default config tree `~/.config/nvim` on Linux/macOS | `desired/<scope>/<subject>/targets/nvim/artifacts/config/` | plugins, generated state, caches, swap/undo/session files, non-default `NVIM_APPNAME` without explicit reviewed location support |
| Local app authoring | Draft and validate local recipes plus synthetic roundtrip fixtures | `recipes/local/<target-id>/...` and fixture trees | no public recipe marketplace; native export/import and arbitrary scripts are not promoted by default |
| Legacy v1 file sync | Existing `.dotfiles-manager.yaml` `status`/`diff`/`deploy`/`import` compatibility | v1 `source` directories | v1 is file sync, not the v2 selected-settings model |

Do not manage secrets, credentials, private keys, tokens, account exports,
generated caches, or runtime state unless a reviewed recipe explicitly says that
exact item is supported.

## What is stored

The repository stores actual desired state so the manager can apply it later.
Examples:

- selected scalar values can be stored as raw values in
  `desired/user/<user-id>/targets/<target>/settings.yaml`;
- whole-file payloads can be stored under
  `desired/user/<user-id>/targets/<target>/artifacts/<artifact-id>`;
- file-tree payloads can be stored as directories under
  `desired/user/<user-id>/targets/<target>/artifacts/<artifact-id>/`.

Normal command output, JSON previews, reports, backup history, and backup
metadata are metadata-oriented and redact raw selected values. Desired artifacts
and backup payloads can still contain the actual managed bytes. Even non-secret config can
reveal hostnames, usernames, paths, and internal systems. Review before pushing
to a public repository, and do not intentionally manage secrets unless a reviewed
recipe explicitly supports that exact item.

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

## Support and safe reporting

For ordinary bugs and documentation feedback, open a GitHub issue with a small,
redacted reproduction. For security-sensitive reports, follow
[`../../SECURITY.md`](../../SECURITY.md). Never paste secrets, private keys,
tokens, private config payloads, full backups, or unreduced logs into public
issues.
