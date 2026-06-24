---
owner: Product + Core Engineering
document-type: v2-vocabulary-source
status: Active vocabulary source; not a runtime behavior contract
last-updated: 2026-06-23
canonical-source: docs/internal/specs/v2/00-vocabulary.md
source-issue: 210
authority: Authoritative v2 vocabulary for planning, specs, issues, CLI text, examples, and docs; behavior still requires promoted behavior specs.
---

# v2 vocabulary

## Purpose

This file defines the vocabulary for the reset v2 product model. It exists so
specs, GitHub issues, command output, examples, and documentation use the same
nouns consistently.

It owns names and meanings only. It is active as a vocabulary/source-of-truth
document, not as a runtime behavior contract.

## Product vocabulary rule

Normal users should experience the product as a settings manager, not as a Git
repository tool and not as a configuration-modeling framework.

Use these public nouns first:

- settings folder;
- live settings;
- stored settings;
- app/tool;
- setting;
- status;
- diff;
- sync;
- conflict;
- catalog, when explaining where recipes come from.

Avoid these as core public nouns in the reset v2 happy path:

- repo or repository;
- desired-state URI;
- driver;
- resource;
- profile stack;
- backup/restore;
- migration;
- user account.

Advanced/internal specs may still use technical terms where needed, but normal
output must explain what the user can do without requiring those terms.

## Core public terms

### Manager

The `dotfiles-manager` application. It manages selected settings for selected
apps/tools by comparing live settings with stored settings and syncing in a safe
direction.

### App/tool

A supported user-facing thing whose selected settings can be managed.

Internal specs may call this a **target**. Normal copy can say app, tool, or
setting depending on context.

Examples:

- `git`;
- `starship`;
- `custom.files` for advanced/custom file management;
- `example-tool` for illustrative-only examples.

### Setting

A named manageable piece of app/tool state.

A setting may be a scalar value, structured object, file, file tree, opaque
portable native export, or other recipe-declared payload. It is the normal unit
for status, diff, sync, conflict reporting, and partial selection.

Examples:

- `git:user.email`;
- `starship:config`;
- `example-tool:user-info` (illustrative-only).

### Public target and setting refs

Normal commands use public refs, not internal URIs.

```text
target-ref = target-id
setting-ref = target-id ":" setting-id
```

Examples:

```text
git
git:user.email
starship:config
example-tool:user-info
```

Public refs are stable user-facing names. They may map internally to settings
folder paths, recipe resources, drivers, or internal URIs, but those mappings are
not part of the ordinary command surface.

### Settings folder

The public noun for the folder where the manager stores selected settings so
they can be compared with and synced to live app settings.

A settings folder may be versioned with Git, but Git is optional. Do not use
`repo`, `repository`, or `config repo` as the normal product noun.

### Settings storage folder

A precise synonym for settings folder. Use it when extra precision is useful,
especially in layout, path ownership, storage safety, and Git-optional
explanations.

### Live settings

The current settings on this computer, read from app-native locations or
app-native commands through a reviewed recipe/driver.

Live settings may include values from files, directories, plist/defaults, INI,
JSON/YAML/TOML, native export commands, or other supported backends. Unknown or
unsafe live state defaults to unmanaged.

### Stored settings

The actual managed settings stored in the settings folder.

Stored settings may contain scalar values, structured fragments, files, file
trees, or portable native export payloads. They may contain personal or sensitive
preferences. Normal output must not print raw values unless a recipe declares the
value safe for display. Secret or unsafe values must be blocked or governed by a
later explicit policy.

### Status

A read-only summary of how live settings and stored settings compare, grouped by
what the user can safely do next.

### Diff

A read-only explanation of differences. Diffs should be readable where possible
and honest where not possible. Opaque native exports should show metadata/hash
changes and limitations, not fake field-level diffs.

### Sync

The action that copies selected settings in a chosen safe direction.

Directions are:

```text
live settings -> stored settings
stored settings -> live settings
```

`sync` must not mean blind automatic two-way merge. It must plan, explain
direction, ask on conflicts, and refuse when the safe direction is unknown.

### Save and apply

`save` and `apply` are not primary v2 product nouns.

Issue #225 accepted them as public compatibility aliases for explicit
directional sync. They remain secondary to the primary `sync` command.

If retained:

```text
save  = sync live settings -> stored settings
apply = sync stored settings -> live settings
```

### Conflict

A state where both sides changed, the baseline is missing, or the safe direction
is otherwise ambiguous. Conflicts require an explicit user decision or refusal.

### Catalog

A source of recipes.

The bundled/default catalog is the source for common built-in app/tool support.
Optional local or remote catalogs are future expansion points. Catalogs are a
public noun only when users manage recipe sources; they are not a prerequisite
for ordinary bundled-app sync.

Remote catalogs require explicit origin, trust, update, disable/remove, and
write-authority rules before recipes from them can write live settings.

## Scopes and profile language

### Scope

The portability boundary for stored settings.

| Internal scope | Normal label | Meaning |
| --- | --- | --- |
| `shared` | for everyone | Same wherever this settings folder is used. |
| `user` | for me | Personal settings that follow one logical person across computers. |
| `machine` | for this computer | Settings for one computer. |
| `machine-user` | for me on this computer | Local personal override for one person on one computer. |

Normal text should prefer the labels. Advanced/JSON output may expose scope IDs.

Scopes do not create app accounts, cloud accounts, or a user-management product.
They only decide where stored settings live and how they are selected.

### Machine

A logical computer identity known to the manager. It is not a hardware serial
number or proof that the same physical hardware is present.

### Logical person / user subject

A logical subject used for `user` and `machine-user` scoped settings. It is not
necessarily the OS account name, app account, or cloud account.

Use `user` in schema/internal contexts when needed, but normal text should say
`for me` or `for me on this computer`.

### Profile layer

A named layer of selections, scopes, policies, values, and location overrides.
Profiles are composition tools for advanced use; they are not an account system.

### Profile stack

An ordered list of profile layers used to resolve the effective settings for a
run. Normal users should not need to understand stack algebra for supported apps.

## Recipe and implementation terms

### Recipe

A reviewed support definition for an app/tool. A recipe declares supported
settings, safe defaults, named locations, resources, drivers, lifecycle policy,
sensitivity policy, and optional native import/export capability.

### Catalog metadata

A source of recipes. The bundled/default catalog comes first. Remote catalogs are
future work and require explicit origin, trust, update, disable/remove, and
write-authority rules before they can write live settings.

### Named location

A recipe-defined logical live path root with a default and optional user override.
Named locations prevent recipes from embedding arbitrary unvalidated paths.

### Resource

An internal technical unit read or written by a driver, such as a file, file
section/key, JSON path, plist key path, file tree, or native export payload.
Resources are not normal user-facing nouns.

### Driver

Reviewed deterministic code shipped with the manager. A driver owns technical
operations such as detecting, reading, normalizing, diffing, previewing, writing,
and verifying resources.

Downloaded or user-authored recipes must not inject arbitrary executable logic
into drivers.

### Desired artifact

Internal term for stored setting material in the settings folder. Desired
artifacts may be scalar settings, files, file trees, opaque native exports, or
metadata binding public settings to payloads.

Normal users should see stored settings and settings folder language instead.

### Internal URI

Internal URI schemes such as `desired://` are implementation/debug identifiers.
Normal text output should not require users to read or type them. They may appear
in verbose, JSON, debug, or authoring contexts when they are the clearest stable
identifier.

Public refs such as `git:user.email` remain the normal command surface.

### Ledger

Local evidence of observed state, previous sync baselines, hashes, run results,
and verification. Ledgers are local state, not the settings folder source of
truth.

### Internal safety evidence

Implementation may keep local pre-write evidence or run records if explicitly
accepted by #212. This must not be presented as a first-class backup/restore
workflow in active v2.

## CLI help and output vocabulary floor

Future CLI contracts, help text, JSON fields, text output, examples, and golden
tests must follow this vocabulary floor unless an issue explicitly supersedes it:

- use settings folder, live settings, stored settings, status, diff, and sync as
  the normal product language;
- describe Git as optional versioning/sharing for a settings folder;
- use user-friendly scope labels (`for everyone`, `for me`,
  `for this computer`, `for me on this computer`) in normal text output;
- keep public refs such as `git:user.email` as the normal selection syntax;
- do not expose `desired://` or other internal URIs except in verbose, JSON,
  debug, or authoring contexts;
- do not present backup/restore, v1 migration, or legacy v1 commands as the
  normal v2 happy path;
- do not print raw stored values in normal output unless the recipe declares
  them safe for display.


## Normative vocabulary rules

1. Public copy says settings folder, live settings, stored settings, status,
   diff, and sync.
2. Git is optional versioning/sharing for a settings folder, not the core noun.
3. Stored settings can contain actual values and sensitive managed bytes.
4. Normal output must not expose raw values, internal URI schemes, resources,
   drivers, profile stacks, or storage paths unless the context is verbose,
   JSON, debug, or authoring.
5. `save`/`apply` are secondary compatibility aliases for directional sync.
6. Backup/restore and v1 migration are not active v2 product nouns.
7. Unknown, unsupported, secret, account-bound, generated, cached, or session
   state defaults to unmanaged.

## Example wording

Preferred normal output:

```text
Changed here:
  git:user.email       for me

Suggested safe action:
  sync live settings -> settings folder
```

Preferred precise/internal wording:

```text
Stored setting:
  ref: git:user.email
  scope: user
  settings folder path: desired/user/leon/targets/git/settings.yaml
  internal URI: desired://user/leon/targets/git/settings#user.email
```

Avoid in normal output:

```text
Save current machine state into repo desired://user/leon/...
Apply repo artifact after backup/restore migration baseline.
```

## Out of scope

- Runtime command behavior.
- Final JSON schema fields.
- Catalog/tap trust implementation.
- Backup/restore policy.
- Legacy v1 public-surface policy.
- Production end-user documentation rewrite.
