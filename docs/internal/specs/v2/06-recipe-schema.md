---
owner: Core Engineering
document-type: v2-draft-spec
status: Draft
last-updated: 2026-06-05
canonical-source: docs/internal/specs/v2/06-recipe-schema.md
source-concept-sections:
  - Recipe
  - Settings and settings groups
  - Named target locations
  - Native import/export
  - Support levels and capabilities
  - Security/privacy/trust model
authority: Draft; non-authoritative until promoted by docs/internal/specs/v2/README.md
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

Local repository recipes are stored at:

```text
recipes/local/<recipe-id>/recipe.yaml
```

`<recipe-id>` is a single path segment matching
`[A-Za-z0-9][A-Za-z0-9._-]*`. A local recipe file must carry:

```yaml
schema: dotfiles-manager.v2.recipe
schemaVersion: 1
```

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

### Explainability metadata

A recipe must provide enough static metadata for `recipe explain <target>` to
render target support without reading live target state. Required explainability
metadata includes:

- target display name, support level, capability, platform support, and recipe
  source/trust context;
- setting labels, support levels, capabilities, default scopes, artifact forms,
  sensitivity/redaction classifications, lifecycle policy, selection defaults,
  resource bindings, driver IDs, and diff/apply limitations;
- settings group IDs, labels, purpose, included setting refs, default selection
  or bulk-selection role, and native import/export grouping where applicable;
- resource IDs, named location IDs, selector shapes, backup/restore support,
  normalization mode, and diff mode;
- native operation summary metadata: operation kind, reviewed/bundled status,
  artifact form, diffability/opacity, lifecycle requirement, timeout class, and
  verification summary.

Write-capable recipes must explain support, capability, lifecycle, trust, and
redaction limits without requiring live reads, desired artifact reads, raw
captures, or command execution. Recipes must not expose secret values,
value-bearing defaults, raw argv, environment variables, or captured output in
explain metadata.

### Settings

Public target and setting ref grammar is owned by `00-vocabulary.md`. Desired
artifact URI and artifact payload schema context are owned by
`05-desired-artifacts-and-uris.md`.

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

| Object | Owned here? | Canonical path | Schema file | Notes |
| --- | --- | --- | --- | --- |
| Recipe metadata | yes | `recipes/local/<recipe-id>/recipe.yaml` or bundled catalog equivalent | `schemas/v2/recipe.schema.json` | Target ID, display, support, platforms. |
| Settings declaration | yes | `recipes/local/<recipe-id>/recipe.yaml#/settings` or bundled catalog equivalent | `schemas/v2/recipe.schema.json` | IDs, defaults, sensitivity, capability. |
| Settings group | yes | `recipes/local/<recipe-id>/recipe.yaml#/settingsGroups` or bundled catalog equivalent | `schemas/v2/recipe.schema.json` | Optional grouping only. |
| Named location | yes | `recipes/local/<recipe-id>/recipe.yaml#/locations` or bundled catalog equivalent | `schemas/v2/recipe.schema.json` | Defaults and override permission. |
| Resource declaration | partial | `recipes/local/<recipe-id>/recipe.yaml#/resources` or bundled catalog equivalent | `schemas/v2/recipe.schema.json` | Driver spec owns operation semantics. |
| Native operation | partial | `recipes/local/<recipe-id>/recipe.yaml#/nativeOperations` or bundled catalog equivalent | `schemas/v2/recipe.schema.json` | Security spec owns command safety. |
| Trust record | no | `trust/trust-record.yaml` local state | `schemas/v2/trust-record.schema.json` | Security spec owns trust persistence. |

The recipe schema uses the fully qualified identifier
`dotfiles-manager.v2.recipe` and has its own version context independent from
config, profile, artifact, ledger, and backup schemas.

## Examples

Recipe examples demonstrate relationship and capability shape. YAML field names
remain sketches until `schemas/v2/recipe.schema.json` is promoted.

### Illustrative-only recipe sketch

`example-tool` is not a bundled target; this snippet only demonstrates schema
relationships for mixed-scope resources.

```yaml
target: example-tool
capability: read-write
settings:
  user.email:
    capability: read-write
    sensitivity: personal
    redaction: redacted-for-display
    scopeDefault: user
    resource: configYaml.userEmail
  user-info:
    capability: read-write
    sensitivity: personal
    redaction: redacted-for-display
    scopeDefault: shared
    resource: userInfoJson
locations:
  config:
    default: ~/.example-tool
resources:
  configYaml.userEmail:
    capability: read-write
    sensitivity: personal
    redaction: redacted-for-display
    lifecycle: allowed
    driver: yaml-file
    location: config
    path: config.yaml
    selector:
      path: [user, email]
  userInfoJson:
    capability: read-write
    sensitivity: personal
    redaction: redacted-for-display
    lifecycle: allowed
    driver: file
    location: config
    path: user-info.json
```

### Candidate Raycast native export sketch

Raycast is a native-app candidate until issue #112 verifies current
export/import support. This snippet only demonstrates capability shape.

```yaml
target: raycast
settings:
  settings-and-data:
    capability: export-only
    sensitivity: unknown
    redaction: redaction-unavailable
    artifactForm: native-export
    diff: metadata-only
```

## Errors, blockers, and partial-result behavior

Recipe validation must reject:

- duplicate target or setting IDs;
- unknown driver IDs;
- selectors unsupported by the driver;
- paths not rooted in named locations;
- write-capable settings or resources without required safety metadata;
- user-local write-capable recipes without caller-provided source/trust context;
- forbidden resource categories;
- arbitrary scripts in MVP;
- native operations without lifecycle, timeout, and verification policy.

If one setting in a target is invalid, safe independent settings may remain
inspectable only if validation can isolate them without ambiguity.

## Acceptance expectations

- Recipe fixtures validate Git, planned/candidate Zsh/Nvim/Starship examples,
  illustrative-only Example Tool examples, and Raycast-like native candidates.
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

- Decide whether constrained `command-io` is included in MVP local recipes.
- Decide exact support matrix for bundled initial recipes.
