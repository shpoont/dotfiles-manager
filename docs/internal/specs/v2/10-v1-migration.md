---
owner: Core Engineering
status: Draft
last-updated: 2026-06-04
canonical-source: docs/internal/specs/v2/10-v1-migration.md
source-concept-sections:
  - v1 compatibility and migration contract
  - Suggested MVP
  - Roadmap decomposition
  - Existing v1 specs and contracts
authority: Non-authoritative until promoted
---

# v2 v1 compatibility and migration

## Purpose

This spec defines how v2 remains compatible with current v1 `syncs:` behavior
while migrating users toward the v2 target/settings/profile model.

V2 must prove v1 parity before replacing the existing dotfile sync path.

## Status and authority

This is a draft extraction from `../../scope/product-concept-v2.md`. It is not
implementation-authoritative until promoted through `README.md`.

Current v1 behavior remains governed by:

- `../cli-and-config-spec.md`;
- `../decisions.md`;
- `../decision-matrix.md`;
- `../../contracts/*`.

## Source map and extraction notes

Extracted from the concept sections covering:

- legacy dotfiles compatibility adapter;
- migration command behavior;
- custom.files target;
- v1 parity before replacement;
- migration preview and reversibility.

Deliberate non-decisions:

- exact migration output filename is deferred;
- exact compatibility alias policy is deferred;
- final issue split is deferred to roadmap.

## Terms owned by this spec

- v1 config;
- legacy adapter;
- migration preview;
- `custom.files` target;
- promotion from custom file to known target;
- parity gate;
- compatibility alias.

## Normative MVP rules

### Compatibility rules

1. Existing `syncs:` configs remain readable through a legacy adapter.
2. Existing source and target paths must be preserved exactly unless the user
   confirms a migration.
3. V1 command behavior remains governed by v1 specs until explicit promotion.
4. V1 command aliases may continue to exist, but v2 docs should prefer `save`,
   `apply`, and guided `sync`.
5. V2 must not reinterpret v1 config as v2 config silently.

### Migration output

A migrated v1 entry becomes a `custom.files` target backed by `file` or
`file-tree` resources.

Optional promotion from `custom.files` into known targets such as `git`, `zsh`,
or `nvim` must be a separate previewed step.

### Migration commands

Draft command behavior:

```bash
dotfiles-manager migrate --dry-run
dotfiles-manager migrate
```

`migrate --dry-run` must show:

- each legacy sync;
- source path;
- target path;
- proposed v2 target/setting;
- proposed driver;
- proposed desired artifact binding;
- files/config that would be written;
- behavior that remains unchanged.

### Reversibility

Migration must not delete v1 config by default. It should write new v2 config or
a migration branch/file, then let the user compare behavior.

### Parity gate

Before v2 replaces v1 sync behavior, tests must prove parity for current v1
features including:

- config resolution;
- path expansion and validation;
- status/diff/deploy/import behavior;
- pattern-gated unmanaged handling;
- deterministic ordering;
- JSON output compatibility where required.

## Derived schema boundaries, not final schemas

This spec owns migration metadata boundaries.

Persisted/emitted objects:

| Object | Owned here? | Notes |
| --- | --- | --- |
| Migration preview | yes | Final JSON shape deferred. |
| Generated v2 config | partial | Profile/layout specs own final schema. |
| Legacy adapter state | partial | Implementation detail. |
| Parity report | partial | Acceptance spec owns release gate. |

## Examples

### Legacy sync to custom.files

```text
legacy sync: zshrc
  source: dotfiles/zsh/.zshrc
  target: ~/.zshrc
  v2 target: custom.files:zshrc
  driver: file
  action: generate profile selection and desired artifact binding
```

### Optional promotion

```text
custom.files:gitconfig -> git:identity
preview required; user must confirm
```

## Errors, blockers, and partial-result behavior

Migration errors include:

- invalid v1 config;
- source/target path cannot be represented safely in v2;
- generated desired binding would conflict with existing v2 artifact;
- migration output path already exists and differs;
- parity test failure.

Partial migration may write nothing by default unless the user explicitly chooses
to generate only safe independent entries.

## Acceptance expectations

- Fixtures cover every existing v1 config behavior.
- Migration dry-run writes nothing.
- Migration preserves source and target paths exactly.
- Generated `custom.files` entries work through file/file-tree drivers.
- V1 config remains readable after migration.
- Optional promotion from `custom.files` requires a separate preview.

## Out of scope

- automatic semantic promotion of every file to known target;
- deleting v1 config by default;
- cross-repository migration;
- remote recipe catalog migration;
- changing v1 behavior before explicit v2 promotion.

## Spec follow-ups / open decisions

- Decide exact migration output filename/path.
- Decide v1/v2 compatibility aliases.
- Decide parity report format.
- Decide when v2 docs become authoritative over v1 docs.
