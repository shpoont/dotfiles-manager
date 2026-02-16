# Getting started

## 1) Prepare your repo

Create or choose a repo where your managed dotfiles live.

## 2) Create config file

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

## 3) Run status first

```bash
dotfiles-manager status
```

This previews what `deploy` and `import` would do.

## 4) Dry-run deploy/import

```bash
dotfiles-manager deploy --dry-run ~/.config/nvim
dotfiles-manager import --dry-run ~/.config/nvim
```

`--dry-run` is available on `deploy` and `import` only.

## 5) Apply changes

```bash
dotfiles-manager deploy ~/.config/nvim
dotfiles-manager import ~/.config/nvim
```

## 6) Optional overrides for config path

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
