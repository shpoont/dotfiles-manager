---
owner: Core Engineering
document-type: v2-draft-spec
status: Draft
last-updated: 2026-06-05
canonical-source: docs/internal/specs/v2/00-vocabulary.md
source-concept-sections:
  - Core nouns and relationships
  - User-facing model
  - Scopes and profiles
  - Driver/resource model
  - Open decisions
authority: Draft; non-authoritative until promoted by docs/internal/specs/v2/README.md
---

# v2 vocabulary

## Purpose

This spec defines the core nouns for the v2 product model. It exists so that
implementation specs, GitHub issues, recipes, and AI agents use the same names
for the same concepts.

This file is vocabulary only. Behavior is defined in the other v2 specs.

## Status and authority

This is a draft extraction from `../../scope/product-concept-v2.md`. It is not
implementation-authoritative until promoted through `README.md`.

Current v1 behavior remains governed by the existing v1 specs and contracts.

## Source map and extraction notes

Extracted from the concept sections covering:

- simple end-user model;
- targets, settings, settings groups, resources, drivers, recipes;
- scopes, profile layers, profile stacks, resolved profiles;
- desired artifacts, current state, ledgers, backups;
- support levels and capabilities.

Deliberate non-decisions:

- canonical schema filenames are defined by `01-repository-layout.md`, not this
  vocabulary spec;
- canonical local state paths are defined by `01-repository-layout.md`, not this
  vocabulary spec;
- product rename decisions are outside this spec.

## Terms owned by this spec

### Manager

The application that manages selected settings and files for selected targets.
It should normally be experienced as:

```text
manage these apps/settings for me
show what changed
save from this machine
apply to this machine
```

### Target

A user-facing thing the manager can manage.

Examples:

- `git`
- `nvim`
- `raycast`
- `custom.files`
- `cobona`

A target may contain one or more settings and one or more internal resources.
Users should normally choose targets by name, not by resource path.

### Setting

A named manageable piece of target state.

Examples:

- `git:user.email`
- `nvim:config`
- `raycast:snippets`
- `cobona:user.email`
- `cobona:user-info`

A setting may be a single scalar value, a structured object, a file, a file tree,
or a portable native export. A setting is the normal unit for status, diff, save,
apply, and conflict reporting.

### Public target and setting refs

Normal commands should use public refs, not internal URIs.

Draft public grammar:

```text
target-ref = target-id
setting-ref = target-id ":" setting-id

target-id = lower-name *( "." lower-name | "-" lower-name )
setting-id = lower-name *( "." lower-name | "-" lower-name )
lower-name = lowercase letter or digit followed by lowercase letters, digits,
             hyphens, or underscores
```

Examples:

- `git`
- `git:user.email`
- `nvim:config`
- `raycast:quicklinks`
- `visual-studio-code:settings`
- `cobona:user-info`

Internal URI form is separate. For example, public `cobona:user.email` may map
to internal `target://cobona/user.email` when URI form is needed.

### Settings group

An optional recipe-owned grouping of related settings.

Groups are not first-class user model objects. They may help recipes express
safe defaults, bulk selection, or native export/import groupings. A target may
have no groups.

### Resource

An internal technical unit read or written by a driver.

Examples:

- a file path;
- a file tree rooted at a named location;
- an INI section/key pair;
- a JSON/YAML/TOML path selector;
- a plist key path;
- a native export artifact.

Resources are normally hidden from end users. They appear in verbose output,
recipe authoring, debugging, and implementation specs.

### Driver

Reviewed deterministic code shipped with the manager. A driver owns technical
operations such as detecting, reading, normalizing, diffing, previewing,
backing up, applying, verifying, and restoring a resource.

Recipes choose from available drivers. Downloaded or user-authored recipes must
not inject arbitrary executable logic into drivers.

### Recipe

A declarative support definition for a target. A recipe declares:

- target identity and support metadata;
- settings and optional settings groups;
- named target locations;
- resources and driver bindings;
- selectors;
- lifecycle policy;
- safety and sensitivity policy;
- import/export capability when applicable;
- verification expectations.

Bundled recipes are reviewed with the tool. User-local recipes require explicit
trust before write-capable behavior.

### Named target location

A recipe-defined logical path root with a default and optional user override.

Examples:

- `home`: `~`
- `config`: `~/.config/cobona`
- `support`: `~/Library/Application Support/Raycast`

Named locations prevent recipes from embedding unvalidated arbitrary paths.

### Scope

The portability boundary for desired state.

Public scopes are:

| Scope | Meaning |
| --- | --- |
| `shared` | Same for everyone using the repository. |
| `user` | Same for one logical user across machines. |
| `machine` | Same for one machine across users. |
| `machine-user` | Specific to one user on one machine. |

Scope chooses where desired artifacts live and which profile layer owns a saved
value. Scope does not by itself define the active profile stack.

### Machine

A logical device identity known to the manager. A machine can have multiple OS
users and multiple profile layers.

Machine identity should be stable enough for machine-scoped desired state and
local ledgers. Exact bootstrap and persistence are specified outside this file.

### User

A logical user identity known to the manager. A user can appear on multiple
machines. A machine can have multiple users.

A user is not necessarily the same as a local POSIX account name, although a
local account may be used as a default bootstrap hint.

### Profile layer

A named layer of desired selections, values, policies, and location overrides.

Examples:

- `global`
- `os/macos`
- `user/leon`
- `machine/mbp-2026`
- `machine-user/mbp-2026/leon`
- `work`
- `personal`

A machine may use multiple profile layers. A user on the same machine may also
use multiple profile layers. Profile layers are ordered into a profile stack.

### Profile stack

An ordered list of profile layers used to resolve desired state for a run.
Later layers override or refine earlier layers where the schema permits.

A machine can have multiple named stacks, such as `default`, `work`, or
`travel`, if the implementation supports them. The MVP must at least support a
deterministic active stack.

### Resolved profile

The result of evaluating the active profile stack, selected targets, selected
settings, scopes, desired artifacts, policies, and named-location overrides.

Commands operate against a resolved profile, not against raw profile layers.

### Desired artifact

Repository-side desired state for a setting or resource.

Examples:

- a scalar value for `git:user.email`;
- a JSON/YAML/TOML fragment;
- a file;
- a file tree;
- an encrypted opaque native export.

Desired artifacts are not always human-editable. Opaque artifacts require
explicit policy and metadata-only diff behavior.

### Current state

Live state read from the current machine, user, or application through a driver.
Current state may be normalized before comparison.

### Last-applied state

The normalized state recorded in the local ledger after a verified successful
save or apply. It is used for conflict detection. It is not the desired artifact
itself.

### Change preview

A dry-run description of intended reads, writes, creates, deletes, backups,
skips, blockers, risks, and verification steps.

Any live mutation must be previewable before it is performed.

### Ledger

Local state recording verified command results, normalized hashes, versions,
backup references, partial failures, and restore material references.

The ledger is not the repository source of truth. It is local evidence used for
status, conflict detection, audit, and restore.

### Backup

Local pre-mutation restore material captured before live writes where supported.
Backups are referenced from ledgers and restore previews.

### Support level

Recipe stability and support classification.

| Support level | Meaning |
| --- | --- |
| `stable` | Tested for declared versions/platforms. |
| `read-only` | Safe to inspect/status/diff only. |
| `experimental` | Available with warnings and explicit opt-in for writes. |
| `deprecated` | Existing support remains temporarily. |
| `blocked` | Known unsafe or unsupported. |

### Capability

What operations a target or setting supports.

| Capability | Meaning |
| --- | --- |
| `inspect-only` | Detect/explain only. |
| `read-only` | Read/status/diff, no write. |
| `read-write` | Save and apply through deterministic drivers. |
| `import-only` | Apply through native import, cannot safely save. |
| `export-only` | Save through native export, cannot safely apply. |
| `never` | Explicit do-not-manage boundary. |

## Normative MVP rules

1. End-user commands should speak in targets, settings, scopes, and actions.
2. Resources, drivers, raw captures, ledgers, and URI internals should be hidden
   unless verbose/debug/authoring output is requested.
3. Every managed setting must resolve to one target, one scope, and one desired
   artifact binding before save/apply behavior is allowed.
4. A machine may have multiple users, and a machine/user may use multiple
   profile layers.
5. Settings groups are optional recipe structure, not required public nouns.
6. Unknown or unsupported target state must default to unmanaged rather than
   being guessed.

## Derived schema boundaries, not final schemas

This vocabulary spec owns names and meanings only. It does not define final
schemas.

Schema-bearing specs must use these terms consistently when defining config,
profile, recipe, artifact, ledger, backup, and preview objects.

## Examples

### Git email

```text
target: git
setting: user.email
scope: user
desired artifact: desired://user/leon/targets/git/settings#user.email
resource: ~/.gitconfig [user] email
driver: ini-file
```

### Cobona mixed scopes

```text
cobona:user.email    scope=user         resource=~/.cobona/config.yaml user.email
cobona:user-info     scope=shared       resource=~/.cobona/user-info.json
```

## Errors, blockers, and partial-result behavior

Vocabulary errors should surface as validation errors in downstream specs:

- unknown target ID;
- unknown setting ID;
- unsupported scope;
- ambiguous target name;
- profile stack cannot be resolved;
- desired artifact binding is missing or duplicated.

## Acceptance expectations

- Documentation and command output use the same nouns consistently.
- Normal output does not require users to know drivers or resources.
- Verbose output can map public settings to resources and drivers.
- Tests include at least one machine with multiple users and one user with
  multiple profile layers.

## Out of scope

- final schema field definitions and runtime validators;
- local state retention and cleanup policy;
- product rename;
- remote recipe catalog terminology;
- AI discovery vocabulary beyond draft-recipe concepts.

## Spec follow-ups / open decisions

- Decide exact machine and user ID bootstrap format.
- Decide whether named profile stacks are MVP or post-MVP.
- Decide final product name and compatibility aliases.
