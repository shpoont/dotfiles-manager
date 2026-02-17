---
owner: Core Engineering
status: Contract v1
last-updated: 2026-02-17
canonical-source: docs/internal/contracts/validation-errors.md
---

# dotfiles-manager: validation and error catalog

This document defines stable error codes and when they are raised.

All runtime errors are fail-fast and return non-zero exit.
When `--json` is set, errors are emitted via JSON envelope (`ok=false`, `error.code`, `error.message`, optional `error.details`).
`version`/`--version` bypass config loading and normally do not traverse this validation pipeline.

## 1) Exit codes

- `0`: command completed successfully
- `1`: any error (validation/config/runtime)

## 2) Error codes

## Config loading

| Code | Trigger | Message template |
|---|---|---|
| `DFM_CONFIG_REQUIRED` | no config source resolved for config-dependent commands (`status`/`deploy`/`import`) from `--config`, `DOTFILES_MANAGER_CONFIG`, or `./.dotfiles-manager.yaml` in cwd | `Config not found: pass --config, set DOTFILES_MANAGER_CONFIG, or create ./.dotfiles-manager.yaml` |
| `DFM_CONFIG_NOT_FOUND` | config path does not exist | `Config file not found: {config_path}` |
| `DFM_CONFIG_NOT_FILE` | config path exists but is not a regular file | `Config path is not a file: {config_path}` |
| `DFM_CONFIG_PARSE` | YAML parse failure | `Failed to parse YAML config: {config_path}` |

## Config schema / values

| Code | Trigger | Message template |
|---|---|---|
| `DFM_CONFIG_SCHEMA_UNKNOWN_KEY` | unknown key present | `Unknown config key: {key_path}` |
| `DFM_CONFIG_SCHEMA_TYPE` | wrong type for key (e.g. string instead of list) | `Invalid type at {key_path}: expected {expected}` |
| `DFM_CONFIG_SCHEMA_REQUIRED` | missing required key | `Missing required key: {key_path}` |
| `DFM_CONFIG_PATH_NOT_RELATIVE` | resolved `syncs[].source` or `syncs[].target` is absolute / starts with `~` / otherwise not relative after placeholder expansion | `Path must be relative: {key_path}` |
| `DFM_CONFIG_PATH_ESCAPE` | normalized config path escapes base via `..` after placeholder expansion | `Path escapes base directory: {key_path}` |
| `DFM_CONFIG_PATH_ENV_VAR_UNDEFINED` | `$VAR`/`${VAR}` placeholder references missing or empty environment variable during expansion | `Environment variable {var} required for path: {key_path}` |

Paths that include `$VAR`/`${VAR}` placeholders are expanded using the runtime environment before the checks above; missing/empty variables raise `DFM_CONFIG_PATH_ENV_VAR_UNDEFINED` and the resulting path is subjected to the usual relative/escape validation.

## CLI path scoping

| Code | Trigger | Message template |
|---|---|---|
| `DFM_FLAG_UNSUPPORTED` | unsupported flag used for command (e.g. `status --dry-run`) | `Flag not supported for command: {flag}` |
| `DFM_FLAG_INVALID_VALUE` | invalid value provided for a supported flag (e.g. `--log-level verbose`) | `Invalid value for {flag}: {value} (expected: {expected})` |
| `DFM_SCOPE_NO_MATCH` | provided `[path]` matches no sync targets | `No sync matched provided path` |
| `DFM_SCOPE_INVALID_PATH` | `[path]` cannot be normalized/resolved | `Invalid path argument: {input_path}` |

## Filesystem/runtime

| Code | Trigger | Message template |
|---|---|---|
| `DFM_IO_READ` | failed read/stat/list operation | `Read failed: {path}` |
| `DFM_IO_WRITE` | failed create/write/copy/chmod/utime operation | `Write failed: {path}` |
| `DFM_IO_REMOVE` | failed remove operation | `Remove failed: {path}` |
| `DFM_TYPE_REPLACE` | failed replacement for file/dir/symlink type mismatch | `Failed to replace path type: {path}` |
| `DFM_METADATA_APPLY` | supported metadata attribute failed to apply | `Failed to apply metadata: {path}` |

## 3) JSON error shape

```json
{
  "ok": false,
  "error": {
    "code": "DFM_CONFIG_SCHEMA_UNKNOWN_KEY",
    "message": "Unknown config key: syncs[0].on.deploy.cleanup",
    "details": {
      "key_path": "syncs[0].on.deploy.cleanup"
    }
  }
}
```

## 4) Validation order (deterministic)

To keep errors deterministic:
1. config source resolution + path checks
2. YAML parse
3. schema + placeholder expansion + path validation (missing/empty env vars raise `DFM_CONFIG_PATH_ENV_VAR_UNDEFINED` before other path checks)
4. `[path]` matching checks
5. runtime filesystem operations

Stop at first failure.

Exception:
- `version` and `--version` short-circuit before config/path/runtime validation.
