---
owner: Core Engineering
document-type: v2-draft-spec
status: Draft
last-updated: 2026-06-05
canonical-source: docs/internal/specs/v2/03-profile-and-scope-resolution.md
source-concept-sections:
  - Scopes
  - Multiple profiles on a machine
  - Profile stack and resolved profile
  - Machine identity bootstrap
  - Example normal user config
authority: Draft; non-authoritative until promoted by docs/internal/specs/v2/README.md
---

# v2 profile and scope resolution

## Purpose

This spec defines how v2 resolves users, machines, scopes, profile layers,
profile stacks, selected targets, and desired artifacts into a command-ready
resolved profile.

It answers the core relationship question: a machine may have multiple users,
and a user/machine may use multiple profile layers.

## Status and authority

This is a draft extraction from `../../scope/product-concept-v2.md`. It is not
implementation-authoritative until promoted through `README.md`.

Current v1 behavior remains governed by existing v1 specs and contracts.

## Source map and extraction notes

Extracted from the concept sections covering:

- public scopes;
- profile layers and stacks;
- machine/user identity;
- desired-state paths;
- user-facing prompts for scope selection.

Deliberate non-decisions:

- exact machine/user ID format is deferred;
- whether named profile stacks beyond the active stack are MVP is deferred.

## Terms owned by this spec

- scope;
- subject;
- machine ID;
- user ID;
- profile layer;
- profile stack;
- resolved profile;
- selection;
- override.

## Normative MVP rules

### Public scopes

| Scope | Subject | Use when |
| --- | --- | --- |
| `shared` | none | Portable settings should be the same for everyone. |
| `user` | user ID | Personal settings follow one logical user across machines. |
| `machine` | machine ID | Hardware, display, host, or device-specific settings. |
| `machine-user` | machine ID + user ID | Local override for one user on one machine. |

Prompts should use plain labels such as:

- Everyone using this repo;
- Me, on all my machines;
- This machine;
- Me on this machine.

The stored values should use the scope codes above.

### Machine and user relationships

1. A machine can have multiple users.
2. A user can appear on multiple machines.
3. A machine can use multiple profile layers.
4. A user on one machine can use multiple profile layers.
5. The same desired artifact must resolve differently only when scope or
   profile override rules say so explicitly.

### Profile and identity file paths

Profile stack and layer files are repository-owned:

```text
profiles/stacks/<stack-id>.yaml
profiles/layers/<layer-id>.yaml
```

`<stack-id>` and `<layer-id>` may be relative profile paths to support names
such as `os/macos`, but they must reject absolute paths, empty segments, `.`,
`..`, backslashes, and traversal before resolving to repository paths.

Machine and user identity records are local-only state:

```text
identity/machine.yaml
identity/users/<local-user>.yaml
```

These local identity files carry the manager schema fields:

```yaml
schema: dotfiles-manager.v2.machine-identity
schemaVersion: 1
```

or:

```yaml
schema: dotfiles-manager.v2.user-identity
schemaVersion: 1
```

Exact bootstrap semantics for creating machine and user IDs remain deferred.

### Profile layers

A profile layer may declare:

- selected targets;
- selected settings;
- desired artifact bindings;
- scope preferences;
- named target location overrides;
- policies;
- recipe trust choices.

A layer must not contain live current state, ledgers, backups, or temporary
captures.

### Profile stack resolution

The manager resolves commands in this order:

1. bootstrap or load machine identity;
2. bootstrap or load user identity;
3. determine active profile stack;
4. append any repeated `--profile <layer>` overrides;
5. validate that all referenced layers exist;
6. merge layers in order according to schema-defined merge rules;
7. resolve selected targets and settings;
8. resolve scope and subject for every setting;
9. resolve desired artifact binding for every selected setting;
10. resolve named target locations and user overrides;
11. validate policies, trust, lifecycle, and safety before writes.

Later layers may override earlier layers only where the schema explicitly allows
it. Unknown keys must fail validation rather than being ignored.

### Scope selection on save

When saving a new setting and no explicit scope exists, the manager should:

1. use a recipe default only if it is safe and obvious;
2. otherwise ask the user with plain-language scope labels;
3. show the resulting desired path before writing;
4. store the selected scope in the owning profile layer.

`--scope` may choose a scope for a save operation, but it must not override
safety or policy blockers.

### Location overrides

Named target location overrides belong in profile layers. For example, a user
may override Cobona config location from `~/.cobona` to another declared root.

Overrides must remain inside the recipe's named-location model. Arbitrary
unvalidated write paths are not allowed.

## Derived schema boundaries, not final schemas

This spec owns the profile and scope schema boundary.

Persisted objects:

| Object | Owned here? | Canonical path | Schema file | Notes |
| --- | --- | --- | --- | --- |
| Profile stack | yes | `profiles/stacks/<stack-id>.yaml` | `schemas/v2/profile-stack.schema.json` | Ordered profile layer names. |
| Profile layer | yes | `profiles/layers/<layer-id>.yaml` | `schemas/v2/profile-layer.schema.json` | Selections, scopes, location overrides, policies. |
| Machine identity | partial | `identity/machine.yaml` local state | `schemas/v2/machine-identity.schema.json` | Bootstrap semantics deferred. |
| User identity | partial | `identity/users/<local-user>.yaml` local state | `schemas/v2/user-identity.schema.json` | Bootstrap semantics deferred. |
| Desired artifact | no | `desired/.../targets/<target-id>/...` | `schemas/v2/desired-manifest.schema.json` and `schemas/v2/desired-settings.schema.json` where applicable | Owned by artifact spec. |
| Recipe default scope | no | `recipes/local/<recipe-id>/recipe.yaml` or bundled recipe catalog | `schemas/v2/recipe.schema.json` | Owned by recipe spec. |

Profile and identity objects use fully qualified schema identifiers such as
`dotfiles-manager.v2.profile-stack` and
`dotfiles-manager.v2.profile-layer`. Field names in examples are sketches until
schema work promotes them.

## Examples

Examples use the public target/setting ref grammar owned by
`00-vocabulary.md`. Profile field names remain sketches until the profile
schemas are promoted.

### Active stack

```yaml
profileStack:
  - global
  - os/macos
  - user/leon
  - machine/mbp-2026
  - machine-user/mbp-2026/leon
```

### Cobona mixed scopes

```yaml
selections:
  cobona:
    settings:
      user.email:
        scope: user
      user-info:
        scope: shared
```

### Non-default location

```yaml
locations:
  cobona:
    config:
      path: ~/.config/cobona
```

## Errors, blockers, and partial-result behavior

Resolution errors include:

- missing profile layer;
- duplicate profile layer ID with different content;
- unknown scope;
- missing user ID for `user` scope;
- missing machine ID for `machine` scope;
- missing user or machine ID for `machine-user` scope;
- duplicate artifact binding after merge;
- named target location override outside allowed root;
- ambiguous owning layer for a save.

A command may continue for independent targets if their resolved profiles are
valid and the command supports partial results.

## Acceptance expectations

- Fixtures cover all four scopes.
- Fixtures cover a machine with two users.
- Fixtures cover one user using two profile layers on one machine.
- Fixtures cover `--profile` repeatability and deterministic ordering.
- Fixtures reject unknown scopes and duplicate artifact bindings.
- Save prompts show plain-language scope choices and resulting desired path.

## Out of scope

- final identity bootstrap semantics and field-level identity schema;
- account login or cloud identity;
- cross-machine merge;
- templating engines;
- persistent secret-manager integration.

## Spec follow-ups / open decisions

- Decide exact ID bootstrap rules for machine and user.
- Decide whether multiple named profile stacks are MVP.
- Decide exact merge rules for each profile-layer field.
- Decide how machine-specific values are represented without general templating.
