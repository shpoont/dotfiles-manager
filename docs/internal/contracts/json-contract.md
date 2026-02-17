---
owner: Core Engineering
status: Contract v2
last-updated: 2026-02-17
canonical-source: docs/internal/contracts/json-contract.md
---

# dotfiles-manager: JSON contract (`--json`)

This document defines machine-readable output for:
- `status --json`
- `deploy [--dry-run] --json`
- `import [--dry-run] --json`

Current schema version is **`2.0`**.

## 1) Common envelope

All `--json` outputs are a single JSON object:

```json
{
  "schema_version": "2.0",
  "ok": true,
  "dry_run": false,
  "command": "status",
  "config_path": "/abs/path/to/.dotfiles-manager.yaml",
  "path_scope": {
    "input": "~/.config/nvim",
    "normalized": "/Users/alice/.config/nvim",
    "matched_sync_indexes": [0]
  },
  "syncs": [],
  "summary": {},
  "error": null
}
```

Rules:
- `schema_version` is currently `"2.0"`.
- `command`: `status` | `deploy` | `import`.
- `dry_run` is valid only for `deploy`/`import`; `status --dry-run` errors with `DFM_FLAG_UNSUPPORTED`.
- `config_path` is the resolved loaded config path (absolute).
- with `--json`, stdout must contain JSON only (logs remain stderr and are unaffected by `--log-format` / `--log-level`).

## 2) Sync payload (shared shape)

Each `syncs[]` entry:

```json
{
  "sync_index": 0,
  "sync": "sync[0] target=~/.config/nvim source=./source/nvim",
  "target": "~/.config/nvim",
  "source": "./source/nvim",
  "source_root": "/abs/source/root",
  "target_root": "/abs/target/root",
  "scope_prefix": "",
  "operations": [],
  "counts": {}
}
```

### `operations[]` item

Common fields:
- `phase`: command-specific phase key.
- `action`: operation verb (`create`, `update`, `replace_type`, `add`, `remove`).
- `state`: `candidate` (status), `planned` (dry-run), or `applied` (non-dry-run successful execution).
- `path`: sync-relative path using `/`.

Optional fields:
- `type`: `file` | `dir` | `symlink`
- `source_type` / `target_type`: `file` | `dir` | `symlink` | `missing` (for status drift entries)

Ordering guarantees:
- `syncs` are in execution order (config order after `[path]` filtering).
- `operations` are deterministic and path-sorted within each produced phase.

## 3) Command-specific phases and counts

### `status --json`

Phases:
- `deploy`
- `import`
- `incoming_unmanaged`
- `remove_unmanaged`
- `remove_missing`

Summary keys:
- `sync_count`
- `deploy_count`
- `import_count`
- `incoming_unmanaged_count`
- `remove_unmanaged_count`
- `remove_missing_count`
- `operation_count`

### `deploy --json`

Phases:
- `copy`
- `remove_unmanaged`

Summary keys:
- `sync_count`
- `copy_count`
- `remove_unmanaged_count`
- `operation_count`

### `import --json`

Phases:
- `update_managed`
- `add_unmanaged`
- `remove_missing`

Summary keys:
- `sync_count`
- `update_managed_count`
- `add_unmanaged_count`
- `remove_missing_count`
- `operation_count`

## 4) Error object and partial results

On failure (`ok=false`, non-zero exit):

```json
{
  "error": {
    "code": "DFM_SCOPE_NO_MATCH",
    "message": "No sync matched provided path",
    "details": {
      "input_path": "~/.config/does-not-exist"
    }
  }
}
```

If work was partially completed before failure:
- `summary.partial = true`
- `syncs` contains completed syncs and/or the partial failed sync subset with only already-applied operations.

## 5) Compatibility policy

- Contract is currently `2.x`.
- Additive fields are allowed in `2.x`.
- Breaking changes require a major schema bump (`3.0`, etc.).
