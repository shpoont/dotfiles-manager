---
owner: Core Engineering
status: Draft
last-updated: 2026-06-04
canonical-source: docs/internal/specs/v2/01-repository-layout.md
source-concept-sections:
  - Files layout of source and configuration
  - Desired artifact lifecycle
  - URI schemes
  - Schema boundaries and versioning
  - v1 compatibility and migration contract
authority: Non-authoritative until promoted
---

# v2 repository layout

## Purpose

This spec defines the logical source/config/state layout for v2. It separates
repository-owned desired state from local machine state and temporary run data.

Exact filenames may change during formal schema work. The logical boundaries in
this spec should remain stable unless explicitly revised.

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

- exact local state directory and retention policy are spec handoff decisions;
- exact JSON Schema filenames are deferred;
- exact repository root discovery is deferred.

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

### Logical layout

The MVP should use this logical layout:

```text
<repo>/
  .dotfiles-manager.yaml          # root manager config or compatibility config
  dotfiles-manager.v2.yaml        # possible v2 root config, exact name deferred
  profiles/
    stacks/
    layers/
  desired/
    shared/-/targets/
    user/<user-id>/targets/
    machine/<machine-id>/targets/
    machine-user/<machine-id>/<user-id>/targets/
  recipes/
    local/
  docs/internal/specs/v2/
```

The exact v2 root config filename is deferred. The implementation must not
silently reinterpret a v1 `.dotfiles-manager.yaml` as v2 without validation or
migration.

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

### Local state layout

The local state layout is logical until exact paths are decided:

```text
state://identity/...
state://ledger/...
state://backups/...
state://captures/...
state://cache/...
temp://run/<run-id>/...
```

The final implementation may map these URIs to platform-specific directories.
It must not map them into the desired repository by default.

### Compatibility layout

Existing v1 `syncs:` config remains readable through a legacy adapter. A v2
migration may generate v2 config and desired artifact bindings, but it must not
delete or rewrite v1 config by default.

## Derived schema boundaries, not final schemas

This spec owns layout boundaries for these persisted objects:

| Object | Persistence class | Final schema owner |
| --- | --- | --- |
| Root config | repository | config/profile specs |
| Profile stack | repository | profile spec |
| Profile layer | repository | profile spec |
| Desired manifest | repository | artifact spec |
| Desired artifact | repository | artifact spec and driver specs |
| Recipe | repository or bundled catalog | recipe spec |
| Ledger entry | local state | mutation/ledger spec |
| Backup metadata | local state | mutation/ledger spec |
| Raw capture | local temp/state | driver and security specs |

Fields shown in examples are sketches unless the owning spec promotes them.

## Examples

### User-scoped Git identity

```text
desired/user/leon/targets/git/settings.yaml
```

### Shared Cobona user info

```text
desired/shared/-/targets/cobona/artifacts/user-info.json
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

- final schema filenames;
- final local state directory;
- backup retention policy;
- remote recipe catalog layout;
- cloud synchronization of local state.

## Spec follow-ups / open decisions

- Decide final v2 root config filename.
- Decide exact state directory on macOS and Linux.
- Decide whether profiles live under `profiles/` or another repository path.
- Decide exact retention and cleanup policy for ledgers, backups, and captures.
