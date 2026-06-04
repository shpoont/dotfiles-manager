---
owner: Core Engineering
document-type: v2-draft-spec
status: Draft
last-updated: 2026-06-04
canonical-source: docs/internal/specs/v2/05-desired-artifacts-and-uris.md
source-concept-sections:
  - Desired artifact lifecycle rules
  - Artifact resolution algorithm
  - URI schemes
  - Schema boundaries and versioning
  - Files layout of source and configuration
authority: Draft; non-authoritative until promoted by docs/internal/specs/v2/README.md
---

# v2 desired artifacts and URIs

## Purpose

This spec defines desired artifacts, artifact resolution, artifact lifecycle
rules, and URI schemes used to reference logical manager data.

The goal is to make actual stored values clear without forcing normal users to
edit internal URIs.

## Status and authority

This is a draft extraction from `../../scope/product-concept-v2.md`. It is not
implementation-authoritative until promoted through `README.md`.

Current v1 behavior remains governed by existing v1 specs and contracts.

## Source map and extraction notes

Extracted from the concept sections covering:

- where actual values are stored;
- desired artifact lifecycle;
- URI style such as `desired://`;
- artifact resolution algorithm;
- examples for Git, Cobona, Raycast, and file-tree state.

Deliberate non-decisions:

- exact artifact manifest schema is deferred;
- exact filename conventions are draft;
- exact encryption format for opaque artifacts is deferred.

## Terms owned by this spec

- desired artifact;
- desired manifest;
- artifact binding;
- artifact URI;
- logical URI;
- raw capture;
- rendered payload;
- opaque artifact.

## Normative MVP rules

### Desired artifact forms

A desired artifact may be one of:

- structured scalar/object data;
- a full file;
- a file tree;
- a native portable export;
- an opaque encrypted bundle, only with explicit opt-in.

Desired artifacts are repository-owned if and only if policy permits them to be
committed.

### Desired artifact lifecycle

1. A selected setting may start with no desired artifact.
2. `save` may create a desired artifact after preview and prompts.
3. `apply` must require a desired artifact unless the operation is a restore or
   a driver-supported create from defaults.
4. Desired artifacts must be validated before use.
5. Desired artifacts must not store forbidden secrets.
6. Opaque artifacts require explicit opt-in, metadata, and limited diff.
7. Deleting a desired artifact must be previewed and must not silently delete
   live state.

### Artifact resolution algorithm

For each selected setting:

1. resolve active profile stack;
2. resolve target and setting selection;
3. resolve scope and subject;
4. resolve artifact binding from profile/config/recipe default;
5. validate that the artifact URI is within the expected scope root;
6. validate artifact form against recipe and driver capability;
7. validate sensitivity and redaction policy;
8. return resolved artifact metadata and path for command execution.

Duplicate artifact bindings for the same resolved setting are validation errors
unless a schema-defined override rule selects one deterministically.

### URI schemes

Logical schemes:

| Scheme | Purpose |
| --- | --- |
| `target://` | Logical target or setting reference. |
| `desired://` | Canonical desired-state repository reference. |
| `state://` | Observed state, ledgers, backups, and caches. |
| `temp://` | Ephemeral per-run workspace data. |
| `secret://` | External secret reference, never secret material. |
| `recipe://` | Recipe definition or recipe-owned resource. |

URIs are internal identifiers. Normal user commands should prefer target and
setting refs such as `git:user.email`.

### URI authority rules

`desired://` references must encode scope before target. `state://` and
`temp://` references must not be committed as desired artifacts except as
metadata references where explicitly allowed.

`secret://` may reference an external secret, but it must never contain the
secret value.

Draft desired URI grammar:

```text
desired-uri = "desired://" desired-scope "/" desired-subject
              "/targets/" target-id
              [ "/" desired-kind [ "/" artifact-path ] ]

desired-scope = "shared" | "user" | "machine" | "machine-user"
desired-subject = "-" | user-id | machine-id | machine-id "/" user-id
desired-kind = "manifest" | "settings" | "artifacts"
```

Use `-` as the subject for `shared`, because shared scope has no user or
machine subject.

## Derived schema boundaries, not final schemas

This spec owns the desired manifest and artifact-reference boundary.

Persisted objects:

| Object | Owned here? | Notes |
| --- | --- | --- |
| Desired manifest | yes | Index from settings to artifacts. |
| Artifact URI | yes | Logical grammar and allowed schemes. |
| Artifact metadata | yes | Form, sensitivity, hash, origin, schema version. |
| Artifact payload | partial | Payload shape depends on driver/setting. |
| Opaque metadata | partial | Encryption details deferred. |

Final schemas must define per-artifact schema versions independently from root
config, profile, recipe, ledger, and backup versions.

## Examples

### Git email desired artifact

```yaml
artifact: desired://user/leon/targets/git/settings#user.email
form: structured
setting: git:user.email
```

### Cobona shared JSON

```yaml
artifact: desired://shared/-/targets/cobona/artifacts/user-info.json
form: structured
setting: cobona:user-info
```

### Raycast opaque export

```yaml
artifact: desired://machine/mbp-2026/targets/raycast/artifacts/settings.rayconfig.enc
form: native-export
opaque: true
diff: metadata-only
```

## Errors, blockers, and partial-result behavior

Required artifact errors include:

- missing desired artifact for apply;
- artifact URI outside expected scope;
- artifact form unsupported by recipe/driver;
- duplicate binding;
- forbidden secret detected;
- opaque artifact without opt-in;
- unknown schema version;
- unreadable or corrupt artifact.

Partial commands may continue for other settings if their artifacts are valid
and the command supports partial success.

## Acceptance expectations

- Fixtures cover each public scope path.
- Fixtures cover structured, file, file-tree, and opaque artifact forms.
- Validation rejects `desired://` path traversal and duplicate bindings.
- Opaque diffs display metadata only and require confirmation for save/apply.
- `secret://` fixtures prove secret values are not serialized.

## Out of scope

- final encryption format;
- final schema filenames;
- remote artifact storage;
- cross-machine merge;
- arbitrary templating.

## Spec follow-ups / open decisions

- Decide exact URI grammar and escaping rules.
- Decide exact desired manifest filename and schema.
- Decide final artifact hash and metadata fields.
- Decide exact opaque bundle encryption policy.
