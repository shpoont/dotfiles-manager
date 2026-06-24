<p align="center">
  <img src="./assets/logo/logo.png" alt="dotfiles-manager logo" width="512" />
</p>

# dotfiles-manager

`dotfiles-manager` is a local settings manager. The v2 workflow lets you select
supported application settings, compare **live settings** with **stored
settings** in a local settings folder, and sync safe changes between them.

`sync` is the primary v2 command. `save` and `apply` remain available only as
directional compatibility aliases:

- `save` = sync live settings -> stored settings;
- `apply` = sync stored settings -> live settings.

The legacy v1 file-sync workflow (`.dotfiles-manager.yaml` with `status`,
`diff`, `deploy`, and `import`) remains available for existing configs.

---

## Current v2 workflow

```text
install -> init -> recipe discover/explain -> add -> status -> diff -> sync
```

When you must choose a direction explicitly, use the compatibility aliases
`save` and `apply`; do not treat them as separate primary workflows.

Start with the safe temporary-home quickstart before touching real dotfiles:

- [`docs/user/install-and-release.md`](./docs/user/install-and-release.md)
- [`docs/user/getting-started.md`](./docs/user/getting-started.md)

## Current supported surface

Bundled v2 support is experimental and intentionally narrow:

- Git selected non-credential identity values (`git:user.email`,
  `git:user.name`);
- Starship selected root TOML values;
- Zsh, tmux, and SSH selected whole-file resources;
- Neovim config-tree resource;
- local recipe authoring and synthetic roundtrip fixtures for advanced users;
- legacy v1 file-sync compatibility.

Do not manage secrets, credentials, private keys, tokens, account exports,
generated caches, or runtime state unless a reviewed recipe explicitly says that
item is supported.

## Install

For the current checkout:

```bash
git clone https://github.com/shpoont/dotfiles-manager.git
cd dotfiles-manager
go test ./...
go build -o ./bin/dotfiles-manager ./cmd/dotfiles-manager
./bin/dotfiles-manager version
./bin/dotfiles-manager init --help
```

Published releases and the Homebrew tap exist, but a published binary can lag
these docs. After any install, verify v2 command help:

```bash
dotfiles-manager version
dotfiles-manager init --help
dotfiles-manager add --help
dotfiles-manager sync --help
dotfiles-manager save --help
dotfiles-manager apply --help
```

If those v2 commands are missing, build from the current source checkout or use
a newer release.

## Safe v2 Git email example

Use a temporary home first:

```bash
DFM=${DFM:-dotfiles-manager}
DFM_DEMO_ROOT=$(mktemp -d)
DFM_HOME="$DFM_DEMO_ROOT/home"
DFM_SETTINGS_FOLDER="$DFM_DEMO_ROOT/settings"
mkdir -p "$DFM_HOME" "$DFM_SETTINGS_FOLDER"

HOME="$DFM_HOME" git config --global user.email first@example.test
cd "$DFM_SETTINGS_FOLDER"
HOME="$DFM_HOME" "$DFM" init --machine-id docs-machine --user-id docs-user
HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
  add git --setting user.email --scope user --profile global --yes
HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
  status --user-id docs-user git:user.email
```

Continue with the full workflow in
[`docs/user/getting-started.md`](./docs/user/getting-started.md).

## Documentation map

- [`docs/README.md`](./docs/README.md) — full docs map.
- [`docs/user/README.md`](./docs/user/README.md) — user-facing v2 docs.
- [`docs/user/configuration.md`](./docs/user/configuration.md) — profiles,
  scopes, stored settings paths, and local state.
- [`docs/user/commands.md`](./docs/user/commands.md) — command reference.
- [`docs/internal/README.md`](./docs/internal/README.md) — canonical internal
  specs/contracts/engineering docs.
