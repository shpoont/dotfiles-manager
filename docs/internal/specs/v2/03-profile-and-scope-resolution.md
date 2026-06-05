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
identity/users/<local-account-key>.yaml
```

`<local-account-key>` is the safe local path key defined by
`01-repository-layout.md`. It is not the portable `userId` and is not raw OS
account text.

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

### Identity IDs and repo-visible privacy

Machine IDs and user IDs are logical manager subject IDs. They are not secrets,
but they are repository-visible because they appear in desired paths such as:

```text
desired/machine/<machine-id>/targets/<target-id>/...
desired/user/<user-id>/targets/<target-id>/...
desired/machine-user/<machine-id>/<user-id>/targets/<target-id>/...
```

The MVP canonical grammar for `machineId`, `userId`, and `localAccountKey` is:

```text
[a-z0-9][a-z0-9._-]*
```

Generated defaults must be lower-case. Hostname-derived or account-derived
values are candidates only. Prompts must explain that accepted IDs are visible
in repository desired paths before writing local identity state or desired
artifacts. If a local hostname or account name cannot produce a safe
non-sensitive candidate, the manager should generate an opaque safe candidate
such as `machine-<short-random>` or `user-<short-random>`.

### Identity record semantics

`identity/machine.yaml` stores one stable `machineId` for the local state root.
It may also store non-secret observed local hints, such as previous sanitized
hostnames, to help explain likely host renames. These hints are advisory; they
must not be treated as authentication or proof of hardware identity.

`identity/users/<local-account-key>.yaml` maps one local OS account key to one
stable logical `userId`. It may also store non-secret observed local-account
hints, such as sanitized account names or platform-local account identifiers,
to help explain likely local account renames. These hints are advisory; they
must not be treated as cloud identity, account login, or cross-machine proof.

Existing local identity records win over generated candidates. Once persisted,
the `machineId` and `userId` do not automatically change when hostname,
computer name, OS account name, or local account metadata changes.

### Identity bootstrap precedence

Commands that need a machine or user subject resolve identity in this order:

1. load an existing local identity record;
2. use explicit safe identity input from `--machine-id` and/or `--user-id`
   when those flags are valid for the command;
3. in interactive mode, prompt with a safe default candidate and the resulting
   desired subject path;
4. if `--yes` is supported for the command, accept only a non-sensitive,
   path-safe generated default;
5. in non-interactive mode, exit with input-required exit code `4` when no
   persisted or explicit identity exists.

`init` is the normal command for creating or connecting local identity state.
An interactive non-dry-run command that is otherwise allowed to create local
manager state may offer the same bootstrap before its requested work, but it
must disclose the local identity write first. Read-only commands, dry-run
commands, and `recipe explain` must not create or update identity records; they
may use existing records or explicit transient identity input, otherwise they
must report unresolved identity and fail or continue according to the command's
partial-result contract.

### Machine bootstrap, adoption, rename, and collision

When `identity/machine.yaml` is missing, the default interactive machine ID
candidate is derived from the sanitized local hostname or computer name. If no
safe candidate exists, or if the candidate would conflict by exact or
case-insensitive path comparison, the manager must add a safe disambiguator or
generate an opaque candidate.

Machine adoption links the local state root to an existing repository machine
subject ID. Adoption does not move repository paths. It is appropriate when the
user intentionally wants this local machine state to use existing
`desired/machine/<machine-id>/...` and
`desired/machine-user/<machine-id>/<user-id>/...` artifacts. Adoption requires
explicit confirmation in interactive mode; non-interactive adoption requires a
future explicit documented adoption policy and otherwise exits `4`.

Machine rename changes the logical `machineId`. Rename is a future explicit
rename operation, not an automatic consequence of hostname changes and not part
of normal save/apply prompts. A rename preview must show all affected repository
paths before any move, including:

```text
desired/machine/<old-machine-id>/...
desired/machine-user/<old-machine-id>/<user-id>/...
```

The preview must also check the proposed destination paths:

```text
desired/machine/<new-machine-id>/...
desired/machine-user/<new-machine-id>/<user-id>/...
```

The rename must block on overwrite, duplicate destination, unsafe path,
case-insensitive collision, or ambiguous profile references unless the user
explicitly resolves the conflict. Local ledgers and backups remain historical
evidence and are not rewritten silently.

### User bootstrap, local-account mapping, adoption, rename, and collision

When the current local account has no
`identity/users/<local-account-key>.yaml` record, the default interactive user
ID candidate is derived from the sanitized local account short name. The prompt
should also offer safe reuse of an existing repository `user/<user-id>` subject
when the user wants the same logical user to span multiple machines.

User adoption links the current local account mapping to an existing repository
user subject ID. Adoption does not move repository paths. It is appropriate
when a user wants this local OS account to use existing
`desired/user/<user-id>/...` and
`desired/machine-user/<machine-id>/<user-id>/...` artifacts. Adoption requires
explicit confirmation in interactive mode; non-interactive adoption requires a
future explicit documented adoption policy and otherwise exits `4`.

User rename changes the logical `userId`. Rename is a future explicit rename
operation, not an automatic consequence of local OS account rename and not part
of normal save/apply prompts. A rename preview must show all affected
repository paths before any move, including:

```text
desired/user/<old-user-id>/...
desired/machine-user/<machine-id>/<old-user-id>/...
```

The preview must also check the proposed destination paths:

```text
desired/user/<new-user-id>/...
desired/machine-user/<machine-id>/<new-user-id>/...
```

The rename must block on overwrite, duplicate destination, unsafe path,
case-insensitive collision, or ambiguous profile references unless the user
explicitly resolves the conflict.

Multiple local OS accounts on one machine may map to different logical
`userId` values. Mapping two local OS accounts on the same machine to the same
`userId` is allowed only with explicit confirmation and a warning that both
accounts share user-scoped desired artifacts.

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

1. resolve machine identity according to the identity bootstrap rules;
2. resolve user identity according to the identity bootstrap rules;
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
| Machine identity | yes | `identity/machine.yaml` local state | `schemas/v2/machine-identity.schema.json` | Stable `machineId`, advisory local hints, bootstrap/adoption/rename semantics. |
| User identity | yes | `identity/users/<local-account-key>.yaml` local state | `schemas/v2/user-identity.schema.json` | Stable `userId`, local-account mapping, advisory local hints, bootstrap/adoption/rename semantics. |
| Desired artifact | no | `desired/.../targets/<target-id>/...` | `schemas/v2/desired-manifest.schema.json` and `schemas/v2/desired-settings.schema.json` where applicable | Owned by artifact spec. |
| Recipe default scope | no | `recipes/local/<recipe-id>/recipe.yaml` or bundled recipe catalog | `schemas/v2/recipe.schema.json` | Owned by recipe spec. |

Profile and identity objects use fully qualified schema identifiers such as
`dotfiles-manager.v2.profile-stack` and
`dotfiles-manager.v2.profile-layer`. Identity schemas must implement the
semantic fields required above, including stable `machineId`, stable `userId`,
and local-account mapping. Other field names in examples are sketches until
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
- invalid machine/user ID or local account key;
- missing identity in a non-interactive command that requires it;
- identity adoption requires confirmation;
- identity rename would overwrite or collide with an existing desired path;
- two local account mappings would share one `userId` without confirmation.

A command may continue for independent targets if their resolved profiles are
valid and the command supports partial results.

## Acceptance expectations

- Fixtures cover all four scopes.
- Fixtures cover a machine with two users.
- Fixtures cover one user using two profile layers on one machine.
- Fixtures cover machine identity bootstrap, persisted reload, non-interactive
  missing-identity failure, hostname rename without automatic ID change, machine
  adoption, machine rename preview, and machine destination collision.
- Fixtures cover user identity bootstrap, local-account mapping, persisted
  reload, non-interactive missing-identity failure, local account rename without
  automatic logical user rename, user adoption, user rename preview, and user
  destination collision.
- Fixtures cover two local accounts on one machine, including the explicit
  warning/confirmation path for mapping both accounts to the same logical user.
- Fixtures cover `--profile` repeatability and deterministic ordering.
- Fixtures reject unknown scopes and duplicate artifact bindings.
- Save prompts show plain-language scope choices and resulting desired path.

## Out of scope

- field-level identity schema details beyond the semantic fields required here;
- account login or cloud identity;
- cross-machine merge;
- templating engines;
- persistent secret-manager integration.

## Spec follow-ups / open decisions

- Decide whether multiple named profile stacks are MVP.
- Decide exact merge rules for each profile-layer field.
- Decide how machine-specific values are represented without general templating.
