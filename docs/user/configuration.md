# Configuration guide

`dotfiles-manager` is config-driven. All behavior is defined in YAML.

## Config loading

Config is resolved in strict order:
1. `--config <path>` (highest priority)
2. `DOTFILES_MANAGER_CONFIG=<path>`
3. `./.dotfiles-manager.yaml` in the current working directory (fallback)

No parent-directory config search is performed.
The default config filename is `.dotfiles-manager.yaml`.

## Basic schema

```yaml
syncs:
  - target: .config/nvim
    source: .config/nvim
    on:
      deploy:
        remove-unmanaged:
          - '**/*.bak'
      import:
        add-unmanaged:
          include:
            - '**'
          exclude:
            - '**/*.tmp'
        remove-missing:
          include:
            - 'lua/**'
          exclude:
            - 'lua/local/**'
```

## `syncs[]` keys

Required:
- `target` — relative to `$HOME`
- `source` — relative to config file directory

Optional behavior keys:
- `on.deploy.remove-unmanaged` — patterns for unmanaged target paths removed during `deploy`
- `on.import.add-unmanaged.include` — patterns for target-only files that can be added to source during `import`
- `on.import.add-unmanaged.exclude` — patterns excluded from unmanaged import candidates
- `on.import.remove-missing.include` — patterns for source paths missing in target that can be removed during `import`
- `on.import.remove-missing.exclude` — patterns excluded from missing-delete candidates

## Defaults (when omitted)

- no unmanaged removal on deploy
- no unmanaged import on import
- no delete-on-missing on import

## Pattern behavior

- Glob engine with `**`
- Patterns are sync-relative
- Separator is `/`
- Include is evaluated before exclude (exclude wins)
- Case sensitivity and escaping are OS-dependent

## Path and order behavior

- Config paths are relative-only.
- Path normalization is lexical.
- Overlapping syncs are allowed.
- Syncs run in config order.
- If overlapping syncs mutate the same final path, later sync wins.
