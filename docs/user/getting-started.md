# Getting started

## 1) Install the CLI

```bash
brew install shpoont/tap/dotfiles-manager
```

Or install with Go:

```bash
go install github.com/shpoont/dotfiles-manager/cmd/dotfiles-manager@latest
```

## 2) Prepare your repo

Create or choose a repo where your managed dotfiles live.

## 3) Create config file

Use YAML (default filename: `.dotfiles-manager.yaml` in your current working directory).

```yaml
syncs:
  - target: .config/nvim
    source: .config/nvim
```

Path rules:
- `target` is relative to `$HOME`
- `source` is relative to the directory containing the config file
- config paths must be relative (no absolute paths)
- config paths must not escape base directories via `..` after normalization

Optional editor schema:
- `docs/internal/contracts/config-schema.json`

## 4) Run status first

```bash
dotfiles-manager status
```

This previews what `deploy` and `import` would do.

## 5) Dry-run deploy/import

```bash
dotfiles-manager deploy --dry-run ~/.config/nvim
dotfiles-manager import --dry-run ~/.config/nvim
```

`--dry-run` is available on `deploy` and `import` only.

## 6) Apply changes

```bash
dotfiles-manager deploy ~/.config/nvim
dotfiles-manager import ~/.config/nvim
```

## 7) Optional overrides for config path

Precedence is:
1. `--config <path>`
2. `DOTFILES_MANAGER_CONFIG`
3. `./.dotfiles-manager.yaml` (current directory fallback)

```bash
export DOTFILES_MANAGER_CONFIG=./.dotfiles-manager.yaml
dotfiles-manager status
```

You can also override with CLI:

```bash
dotfiles-manager --config ./custom-config.yaml status
```

## Notes

- Source is treated as the manifest/source of truth.
- Commands fail fast on runtime errors.
- Add `--json` when you need machine-readable output.
- Logs are written to:
  - macOS: `~/Library/Logs/dotfiles-manager/dotfiles-manager.log`
  - Linux: `${XDG_STATE_HOME:-~/.local/state}/dotfiles-manager/dotfiles-manager.log`
- Use `--log-file <path>` to override the log file destination.
