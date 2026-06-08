---
owner: Core Engineering
document-type: v2-draft-spec
status: Draft
last-updated: 2026-06-05
canonical-source: docs/internal/specs/v2/01-repository-layout.md
source-concept-sections:
  - Files layout of source and configuration
  - Desired artifact lifecycle
  - URI schemes
  - Schema boundaries and versioning
  - v1 compatibility and migration contract
authority: Draft; non-authoritative until promoted by docs/internal/specs/v2/README.md
---

# v2 repository layout

## Purpose

This spec defines the logical source/config/state layout for v2. It separates
repository-owned desired state from local machine state and temporary run data.

This spec closes the MVP handoff decisions for canonical repository paths,
schema filenames, and local-only state roots. Field-level schemas and runtime
validators remain separate implementation work.

## Status and authority

This is a draft extraction from `../../scope/product-concept-v2.md`. It is not
implementation-authoritative until promoted through `README.md`.

Current v1 behavior remains governed by existing v1 specs and contracts.

## Source map and extraction notes

Extracted from the concept sections covering:

- actual values and where they are stored;
- desired repository paths by scope;
- local state, ledgers, backups, captures, and temp data;
- schema boundaries and artifact URI conventions;
- v1 compatibility adapter.

Deliberate non-decisions:

- retention and pruning policy for ledgers, backups, captures, and caches is
  deferred;
- exact schema field definitions and runtime validators are deferred.

## Terms owned by this spec

- repository root;
- source config;
- profile directory;
- desired directory;
- recipe directory;
- local state directory;
- temporary run directory.

## Normative MVP rules

### Repository-owned data

Repository-owned data is portable and may be committed.

It includes:

- manager config;
- profile layers and stacks;
- selected targets/settings;
- desired artifacts;
- bundled or user-local recipes when intentionally committed;
- migration output when chosen by the user.

Repository-owned data must not include:

- raw live captures unless explicitly converted to desired artifacts;
- local ledgers;
- local backups;
- temporary rendered files;
- secrets unless explicitly permitted by a recipe and redaction policy.

### Local state data

Local state data is machine-local and should not be committed.

It includes:

- machine/user identity records;
- ledgers;
- backups;
- normalized hashes;
- raw captures used during a run;
- temporary render/apply workspaces;
- cache entries.

### Repository root discovery

A v2 repository root is the nearest ancestor directory containing
`dotfiles-manager.v2.yaml`.

`.dotfiles-manager.yaml` is never a v2 root marker. A directory containing only
`.dotfiles-manager.yaml` is treated as v1/legacy input and may be used only by
the legacy adapter or migration command.

If both `dotfiles-manager.v2.yaml` and `.dotfiles-manager.yaml` exist, v2
commands read only `dotfiles-manager.v2.yaml`; the legacy adapter and migration
commands read the v1 file. The files are not merged.

### Canonical MVP path decisions

The MVP uses this repository-owned layout:

```text
<repo>/
  dotfiles-manager.v2.yaml
  profiles/
    stacks/<stack-id>.yaml
    layers/<layer-id>.yaml
  desired/
    shared/-/targets/<target-id>/manifest.yaml
    shared/-/targets/<target-id>/settings.yaml
    shared/-/targets/<target-id>/artifacts/...
    user/<user-id>/targets/<target-id>/manifest.yaml
    user/<user-id>/targets/<target-id>/settings.yaml
    user/<user-id>/targets/<target-id>/artifacts/...
    machine/<machine-id>/targets/<target-id>/manifest.yaml
    machine/<machine-id>/targets/<target-id>/settings.yaml
    machine/<machine-id>/targets/<target-id>/artifacts/...
    machine-user/<machine-id>/<user-id>/targets/<target-id>/manifest.yaml
    machine-user/<machine-id>/<user-id>/targets/<target-id>/settings.yaml
    machine-user/<machine-id>/<user-id>/targets/<target-id>/artifacts/...
  recipes/
    local/<recipe-id>/recipe.yaml
  migrations/
    v1-to-v2/<run-id>/migration-plan.yaml
    v1-to-v2/<run-id>/generated/dotfiles-manager.v2.yaml
    v1-to-v2/<run-id>/generated/profiles/...
    v1-to-v2/<run-id>/generated/desired/...
    v1-to-v2/<run-id>/generated/recipes/local/...
```

`dotfiles-manager.v2.yaml` is the only canonical v2 root config filename.
It must carry `schema: dotfiles-manager.v2.root-config` and
`schemaVersion: 1`. Existing `.dotfiles-manager.yaml` remains v1/legacy adapter
input only and must not be silently parsed as v2.

`manifest.yaml` binds public settings to desired artifacts. `settings.yaml` is
the default manager-owned structured desired object for scalar/object settings
and must carry schema/version metadata. `artifacts/` is for file, file-tree,
native-export, opaque, and other payload material. Driver-owned payloads under
`artifacts/` do not embed manager schema fields by default, even when the
payload format is YAML or JSON; their form, schema/version context, hash, and
owner context are recorded in `manifest.yaml`.

Repository-owned migration output is written under
`migrations/v1-to-v2/<run-id>/` by default instead of overwriting active v2
files.

### Path variable safety

`<target-id>` uses the lower-case public ID grammar owned by
`00-vocabulary.md`. `<recipe-id>` uses the same lower-case path-safe ID
components for MVP.

`<machine-id>`, `<user-id>`, and local-state `<local-account-key>` path
segments match `[a-z0-9][a-z0-9._-]*` for MVP paths. Generated identity IDs and
local account keys must be lower-case so repository paths do not depend on
case-sensitive filesystem behavior. `<local-account-key>` is a manager-owned
safe key derived from the local OS account with a disambiguator when needed; it
is not raw OS account text and is not a portable user identity.

`<stack-id>` and `<layer-id>` may be relative profile paths to allow names such
as `os/macos`, but they must reject absolute paths, empty segments, `.`, `..`,
backslashes, and traversal. After validation, the stored file is
`profiles/stacks/<stack-id>.yaml` or `profiles/layers/<layer-id>.yaml`.

### Desired scope directories

Desired artifacts should encode scope before target:

```text
desired/shared/-/targets/<target-id>/...
desired/user/<user-id>/targets/<target-id>/...
desired/machine/<machine-id>/targets/<target-id>/...
desired/machine-user/<machine-id>/<user-id>/targets/<target-id>/...
```

This rule keeps shared, user, machine, and machine-user state visibly separate.
The shared subject is the literal `-` segment.

### Schema file locations

Implementation JSON Schemas live outside the managed dotfiles repository unless
the implementation explicitly vendors them for development. The canonical MVP
schema filenames are:

```text
schemas/v2/root-config.schema.json
schemas/v2/profile-stack.schema.json
schemas/v2/profile-layer.schema.json
schemas/v2/machine-identity.schema.json
schemas/v2/user-identity.schema.json
schemas/v2/desired-manifest.schema.json
schemas/v2/desired-settings.schema.json
schemas/v2/recipe.schema.json
schemas/v2/ledger-entry.schema.json
schemas/v2/run-record.schema.json
schemas/v2/backup-metadata.schema.json
schemas/v2/preview.schema.json
schemas/v2/migration-plan.schema.json
schemas/v2/trust-record.schema.json
```

Every manager-owned persisted YAML/JSON object must carry a fully qualified
schema identifier and version:

```yaml
schema: dotfiles-manager.v2.<object-name>
schemaVersion: 1
```

Examples include `dotfiles-manager.v2.root-config`,
`dotfiles-manager.v2.profile-layer`, and
`dotfiles-manager.v2.desired-manifest`.

### Local state layout

`<repo-state-id>` is the lowercase hexadecimal SHA-256 of `realpath(<repo>)`.
The default local state paths use `<repo-state-id>` so different clones do not
accidentally share local ledgers, backups, captures, or caches.

macOS roots:

```text
~/Library/Application Support/dotfiles-manager/v2/<repo-state-id>/
~/Library/Caches/dotfiles-manager/v2/<repo-state-id>/
${TMPDIR}/dotfiles-manager/v2/<repo-state-id>/<run-id>/
```

Linux roots:

```text
${XDG_STATE_HOME:-~/.local/state}/dotfiles-manager/v2/<repo-state-id>/
${XDG_CACHE_HOME:-~/.cache}/dotfiles-manager/v2/<repo-state-id>/
${TMPDIR:-/tmp}/dotfiles-manager/v2/<repo-state-id>/<run-id>/
```

State subpaths:

```text
identity/machine.yaml
identity/users/<local-account-key>.yaml
ledger/ledger.jsonl
ledger/runs/<run-id>.json
backups/<run-id>/backup.yaml
backups/<run-id>/payloads/...
runs/<run-id>/preview.json
runs/<run-id>/captures/...
trust/trust-record.yaml
```

`trust/trust-record.yaml` is relative to the local state root above, not to the
synced repository root. Trust records authorize user-local recipe writes and
must not be read from or written to versioned repository content by default.

Cache subpaths:

```text
normalized/...
recipe-index/...
```

`cache/normalized/...` contains derived, rebuildable normalization results only.
Authoritative last-applied hashes and verified outcomes live in ledger entries
and run records, not cache.

Temp subpaths:

```text
rendered/...
apply/...
downloads/...
```

Local state, backups, cache, captures, and temp directories must not resolve
inside `<repo>/` by default. They must never resolve inside `desired/` or under
a desired artifact path. An explicit override into the repo, if ever supported,
must be a separate opt-in with validation and must still reject `desired/`,
`profiles/`, `recipes/`, and active migration output paths.

### Compatibility layout

Existing v1 `syncs:` config remains readable through a legacy adapter. A v2
migration may generate v2 config and desired artifact bindings, but it must not
delete or rewrite v1 config by default.

## Derived schema boundaries, not final schemas

This spec owns layout boundaries for these persisted objects:

| Object | Persistence class | Canonical path | Schema owner | Schema file |
| --- | --- | --- | --- | --- |
| Root config | repository | `dotfiles-manager.v2.yaml` | layout/profile specs | `schemas/v2/root-config.schema.json` |
| Profile stack | repository | `profiles/stacks/<stack-id>.yaml` | profile spec | `schemas/v2/profile-stack.schema.json` |
| Profile layer | repository | `profiles/layers/<layer-id>.yaml` | profile spec | `schemas/v2/profile-layer.schema.json` |
| Machine identity | local state | `identity/machine.yaml` | profile spec | `schemas/v2/machine-identity.schema.json` |
| User identity | local state | `identity/users/<local-account-key>.yaml` | profile spec | `schemas/v2/user-identity.schema.json` |
| Desired manifest | repository | `desired/.../targets/<target-id>/manifest.yaml` | artifact spec | `schemas/v2/desired-manifest.schema.json` |
| Desired settings | repository | `desired/.../targets/<target-id>/settings.yaml` | artifact spec | `schemas/v2/desired-settings.schema.json` |
| Desired artifact payload | repository | `desired/.../targets/<target-id>/artifacts/...` | artifact and driver specs | recorded by `manifest.yaml` |
| Recipe | repository or bundled catalog | `recipes/local/<recipe-id>/recipe.yaml` | recipe spec | `schemas/v2/recipe.schema.json` |
| Ledger entry | local state | `ledger/ledger.jsonl` | mutation/ledger spec | `schemas/v2/ledger-entry.schema.json` |
| Run record | local state | `ledger/runs/<run-id>.json` | mutation/ledger spec | `schemas/v2/run-record.schema.json` |
| Backup metadata | local state | `backups/<run-id>/backup.yaml` | mutation/ledger spec | `schemas/v2/backup-metadata.schema.json` |
| Preview | local state | `runs/<run-id>/preview.json` | CLI and mutation specs | `schemas/v2/preview.schema.json` |
| Raw capture | local temp/state | `runs/<run-id>/captures/...` | driver and security specs | recorded by run/preview metadata |
| Trust record | local state outside repository | `<state-root>/trust/trust-record.yaml` | security spec | `schemas/v2/trust-record.schema.json` |
| Migration plan | repository | `migrations/v1-to-v2/<run-id>/migration-plan.yaml` | migration spec | `schemas/v2/migration-plan.schema.json` |

Fields shown in examples are sketches unless the owning spec promotes them.

## Examples

Examples in this layout spec demonstrate canonical repository path shapes.
Field names inside schema-bearing snippets remain sketches unless the owning
spec promotes them.

### User-scoped Git identity

```text
desired/user/leon/targets/git/settings.yaml
```

### Illustrative-only shared Example Tool user info

```text
desired/shared/-/targets/example-tool/artifacts/user-info.json  # illustrative-only
```

### Machine-user override

```text
desired/machine-user/mbp-2026/leon/targets/git/settings.yaml
```

## Errors, blockers, and partial-result behavior

The implementation must reject or block:

- desired paths that escape their scope root;
- duplicate desired artifact bindings for the same resolved setting;
- repository-owned files that would store forbidden secret material;
- local state paths that resolve into the repository unless explicitly allowed;
- local state paths that resolve into `desired/` or under any desired artifact
  path;
- v1 config reinterpretation without explicit adapter/migration path.

Partial results must report which target/setting failed layout validation and
which other items, if any, remain safe to process.

## Acceptance expectations

- Fixtures cover all four public scopes.
- Fixtures prove local ledgers/backups are not written under desired artifacts.
- Migration fixtures preserve existing v1 source and target paths.
- Validation rejects path traversal and duplicate artifact bindings.
- Status and verbose output can map a public setting to its desired path.

## Out of scope

- backup retention policy;
- remote recipe catalog layout;
- cloud synchronization of local state.

## Spec follow-ups / open decisions

- Decide exact retention and cleanup policy for ledgers, backups, and captures.
- Decide exact schema field definitions and runtime validator implementation.
