---
owner: Core Engineering
status: Contract v1
last-updated: 2026-02-16
canonical-source: docs/internal/contracts/json-contract.md
---

# dotfiles-manager: JSON contract (`--json`)

This document defines the machine-readable output for:
- `status --json`
- `deploy [--dry-run] --json`
- `import [--dry-run] --json`

This is the implementation contract for v1.

## 1) Common envelope

All `--json` command outputs must be a single JSON object:

```json
{
  "schema_version": "1.0",
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

### Common field rules

- `schema_version`: string, currently `"1.0"`.
- `ok`: `true` on success, `false` on error.
- `dry_run`: `true` only when `--dry-run` is set (deploy/import only); otherwise `false`.
- `command`: one of `status`, `deploy`, `import`.
- `config_path`: absolute path of resolved loaded config (from CLI, env var, or cwd default fallback).
- `path_scope.input`: original CLI `[path]` or `null` if omitted.
- `path_scope.normalized`: normalized absolute path or `null` if omitted.
- `path_scope.matched_sync_indexes`: 0-based indexes into `syncs` config entries.
- `syncs`: command-specific payload array.
- `summary`: command-specific aggregate counts.
- `error`: `null` on success; object on failure.

With `--json`, no non-JSON content should be emitted to stdout.
`status` does not accept `--dry-run` (error code: `DFM_FLAG_UNSUPPORTED`).
`--log-format` affects logs on stderr only and must not alter stdout JSON schema.
`--log-level` affects logs on stderr only and must not alter stdout JSON schema.

## 2) Path and ordering rules

- All per-file/per-dir paths in payload objects are **sync-relative**.
- Path separator in JSON paths is always `/`.
- Arrays of path objects must be sorted lexically by `path`.
- `syncs` array is ordered by execution order (config order, filtered by `[path]` selection).

## 3) Status payload

`status --json` returns:

```json
{
  "syncs": [
    {
      "sync_index": 0,
      "source_root": "/abs/src/.config/nvim",
      "target_root": "/Users/alice/.config/nvim",
      "scope_prefix": "lua",
      "deploy_changes": [
        {
          "path": "lua/init.lua",
          "change": "create",
          "source_type": "file",
          "target_type": "missing"
        }
      ],
      "import_changes": [],
      "incoming_unmanaged": [],
      "removable_unmanaged": [],
      "removable_missing": []
    }
  ],
  "summary": {
    "sync_count": 1,
    "deploy_change_count": 1,
    "import_change_count": 0,
    "incoming_unmanaged_count": 0,
    "removable_unmanaged_count": 0,
    "removable_missing_count": 0
  }
}
```

### Status enums

- `change`: `create` | `update` | `replace_type`
- `source_type` / `target_type`: `file` | `dir` | `symlink` | `missing`

`incoming_unmanaged`, `removable_unmanaged`, `removable_missing` objects use:

```json
{ "path": "...", "type": "file|dir|symlink" }
```

## 4) Deploy payload

`deploy --json` (or `deploy --dry-run --json`) returns:

```json
{
  "syncs": [
    {
      "sync_index": 0,
      "source_root": "/abs/src/.config/nvim",
      "target_root": "/Users/alice/.config/nvim",
      "scope_prefix": "",
      "copied": [
        {
          "path": "init.lua",
          "change": "update",
          "type": "file"
        }
      ],
      "removed_unmanaged": [
        {
          "path": "tmp/old.lua",
          "type": "file"
        }
      ]
    }
  ],
  "summary": {
    "sync_count": 1,
    "copied_count": 1,
    "removed_unmanaged_count": 1
  }
}
```

### Deploy enums

- `change`: `create` | `update` | `replace_type`
- `type`: `file` | `dir` | `symlink`

When `dry_run=true`, `copied` and `removed_unmanaged` represent planned operations (not executed operations).

## 5) Import payload

`import --json` (or `import --dry-run --json`) returns:

```json
{
  "syncs": [
    {
      "sync_index": 0,
      "source_root": "/abs/src/.config/nvim",
      "target_root": "/Users/alice/.config/nvim",
      "scope_prefix": "",
      "updated_manifest": [
        {
          "path": "lua/init.lua",
          "change": "update",
          "type": "file"
        }
      ],
      "added_unmanaged": [
        {
          "path": "lua/new.lua",
          "type": "file"
        }
      ],
      "removed_missing": [
        {
          "path": "lua/legacy.lua",
          "type": "file"
        }
      ]
    }
  ],
  "summary": {
    "sync_count": 1,
    "updated_manifest_count": 1,
    "added_unmanaged_count": 1,
    "removed_missing_count": 1
  }
}
```

### Import enums

- `change`: `create` | `update` | `replace_type`
- `type`: `file` | `dir` | `symlink`

When `dry_run=true`, `updated_manifest`, `added_unmanaged`, and `removed_missing` represent planned operations (not executed operations).

## 6) Error object

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

- `code`: stable machine-readable error code (see `validation-errors.md`)
- `message`: human-readable summary
- `details`: optional structured context

When failure happens after partial work, include in `summary`:

```json
{
  "partial": true
}
```

and include already-applied operations up to failure.
For dry-run failures, `summary.partial=true` indicates partial planning completed before failure.

## 7) Compatibility guarantees

- Additive fields are allowed in `1.x`.
- Existing fields in this contract must not change meaning in `1.x`.
- Breaking changes require `schema_version` major bump.
