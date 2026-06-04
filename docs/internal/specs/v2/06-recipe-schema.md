---
owner: Core Engineering
status: Draft
last-updated: 2026-06-04
canonical-source: docs/internal/specs/v2/06-recipe-schema.md
source-concept-sections:
  - Recipe
  - Settings and settings groups
  - Named target locations
  - Native import/export
  - Support levels and capabilities
  - Security/privacy/trust model
authority: Non-authoritative until promoted
---

# v2 recipe schema

## Purpose

This spec defines the draft recipe model for target support. Recipes describe
what a target can manage, where its settings live, how settings map to drivers,
and which safety policies apply.

Recipes should make common apps easy for normal users while giving advanced
users a constrained way to add custom targets.

## Status and authority

This is a draft extraction from `../../scope/product-concept-v2.md`. It is not
implementation-authoritative until promoted through `README.md`.

Current v1 behavior remains governed by existing v1 specs and contracts.

## Source map and extraction notes

Extracted from the concept sections covering:

- recipe-owned target declarations;
- settings, optional settings groups, and resources;
- named locations and user overrides;
- native export/import support;
- lifecycle policy;
- support level and capability;
- trust rules for local and downloaded recipes.

Deliberate non-decisions:

- final recipe YAML/JSON schema is deferred;
- remote recipe catalog is deferred;
- arbitrary recipe scripts are deferred and not MVP.

## Terms owned by this spec

- recipe;
- target ID;
- setting ID;
- settings group;
- named target location;
- resource declaration;
- capability;
- support level;
- lifecycle policy;
- native export/import operation.

## Normative MVP rules

### Recipe responsibilities

A recipe must declare:

- stable target ID;
- display name;
- supported platforms and version constraints where known;
- support level;
- target capability;
- settings and optional settings groups;
- named target locations;
- resource declarations;
- driver bindings and selectors;
- default scope recommendations;
- sensitivity and redaction policy;
- lifecycle policy;
- verification expectations.

A recipe must not contain arbitrary executable code in MVP.

### Settings

Each setting must declare:

- stable setting ID;
- user-facing label;
- support level;
- capability;
- default scope, if safe;
- artifact form;
- resource binding;
- sensitivity classification;
- diff/apply limitations.

A setting may be excluded by default if it is risky, account-bound, machine-local,
opaque, secret-bearing, or too noisy.

### Settings groups

Settings groups are optional. They may define safe defaults, bulk selection, or
native export/import groupings. They must not become required public nouns.

### Named target locations

Every resource path must be expressed through a named target location. A named
location has:

- ID;
- default path or platform-specific default;
- path kind;
- whether user override is allowed;
- allowed driver/resource use.

User overrides are profile-layer data, not recipe mutations.

### Native import/export

Native import/export is a capability shape, not arbitrary shell scripting.

A native operation must declare:

- operation kind: export, import, or both;
- whether it is bundled/reviewed support;
- argv-style command or API contract if command-backed;
- input/output artifact form;
- whether output is diffable or opaque;
- lifecycle requirements;
- timeout and expected exit behavior;
- verification behavior;
- sensitivity and redaction policy.

Unreviewed command-backed save/apply is not MVP. Bundled reviewed support may
include constrained native import/export for apps such as Raycast when the tool
can preserve safety and explain diff limitations.

### Support levels and capabilities

A write-capable setting must declare both support level and capability.

Support levels:

- `stable`;
- `read-only`;
- `experimental`;
- `deprecated`;
- `blocked`.

Capabilities:

- `inspect-only`;
- `read-only`;
- `read-write`;
- `import-only`;
- `export-only`;
- `never`.

### Trust

Bundled recipes are trusted by distribution. User-local recipes require explicit
trust before write-capable behavior. Downloaded recipe catalog support is
post-MVP and requires signed review/update policy before use.

## Derived schema boundaries, not final schemas

This spec owns the recipe schema boundary.

Persisted objects:

| Object | Owned here? | Notes |
| --- | --- | --- |
| Recipe metadata | yes | Target ID, display, support, platforms. |
| Settings declaration | yes | IDs, defaults, sensitivity, capability. |
| Settings group | yes | Optional grouping only. |
| Named location | yes | Defaults and override permission. |
| Resource declaration | partial | Driver spec owns operation semantics. |
| Native operation | partial | Security spec owns command safety. |
| Trust record | partial | Security spec owns trust persistence. |

Final recipe schema must have its own schema version independent from config,
profile, artifact, ledger, and backup schemas.

## Examples

### Cobona recipe sketch

```yaml
target: cobona
settings:
  user.email:
    scopeDefault: user
    resource: configYaml.userEmail
  user-info:
    scopeDefault: shared
    resource: userInfoJson
locations:
  config:
    default: ~/.cobona
resources:
  configYaml.userEmail:
    driver: yaml-file
    location: config
    path: config.yaml
    selector: user.email
  userInfoJson:
    driver: json-file
    location: config
    path: user-info.json
```

### Raycast native export sketch

```yaml
target: raycast
settings:
  settings-and-data:
    capability: export-only
    artifactForm: native-export
    diff: metadata-only
    requiresOptIn: true
```

## Errors, blockers, and partial-result behavior

Recipe validation must reject:

- duplicate target or setting IDs;
- unknown driver IDs;
- selectors unsupported by the driver;
- paths not rooted in named locations;
- write-capable settings without support/capability declarations;
- forbidden resource categories;
- arbitrary scripts in MVP;
- native operations without lifecycle, timeout, and verification policy.

If one setting in a target is invalid, safe independent settings may remain
inspectable only if validation can isolate them without ambiguity.

## Acceptance expectations

- Recipe fixtures validate Git, Zsh, Nvim, Starship, Cobona, and Raycast-like
  examples.
- Invalid selector and unsafe path fixtures are rejected.
- Settings groups can be absent without affecting target support.
- Native export fixtures distinguish diffable from opaque artifacts.
- User-local write-capable recipe fixtures require explicit trust.

## Out of scope

- remote recipe catalog;
- signed downloads;
- arbitrary recipe scripts;
- broad app reverse engineering;
- launchd writes;
- service management beyond plain file resources.

## Spec follow-ups / open decisions

- Decide exact recipe schema filename and format.
- Decide whether constrained `command-io` is included in MVP local recipes.
- Decide exact recipe trust-record storage.
- Decide exact support matrix for bundled initial recipes.
