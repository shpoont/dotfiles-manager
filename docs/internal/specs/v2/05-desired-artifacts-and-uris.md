---
owner: Core Engineering
document-type: v2-draft-spec
status: Draft
last-updated: 2026-06-07
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
- examples for Git, illustrative-only Example Tool, candidate Raycast native state, and file-tree state.

Deliberate non-decisions:

- exact encryption format for opaque artifacts is deferred.
- exact URI escaping rules remain deferred.

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

### Desired artifact paths

Every selected target scope has a desired target directory:

```text
desired/<scope>/<subject>/targets/<target-id>/
```

The canonical files under a target directory are:

```text
manifest.yaml
settings.yaml
artifacts/...
```

`manifest.yaml` is a manager-owned object with:

```yaml
schema: dotfiles-manager.v2.desired-manifest
schemaVersion: 1
```

`settings.yaml` is the default manager-owned structured desired object for
scalar/object settings and uses:

```yaml
schema: dotfiles-manager.v2.desired-settings
schemaVersion: 1
```

Payloads under `artifacts/` may be arbitrary file, file-tree, native-export, or
opaque material. Driver-owned payloads under `artifacts/` do not embed manager
schema fields by default, even when the payload format is YAML or JSON; their
form, schema/version context, hash, and owner context are recorded in
`manifest.yaml`.

### Payload schema context

Manager-owned persisted YAML/JSON objects, including `manifest.yaml` and
`settings.yaml`, must embed:

```yaml
schema: dotfiles-manager.v2.<object-name>
schemaVersion: 1
```

Driver-owned payloads under `artifacts/` do not embed manager schema fields by
default, even when the payload format is YAML or JSON. Their desired manifest
entry records the payload form, driver or recipe owner, payload schema or format
version context, and hash. Concrete manifest field names remain future schema
work.

If a future recipe declares a specific artifact payload as manager-owned instead
of driver-owned, that exception must be explicit in the recipe and the payload
must then carry the manager schema fields above.

### MVP selected-value `settings.yaml`

For MVP selected-value resources (`ini-file`, `json-file`, and `yaml-file`),
the default manager-owned desired artifact is:

```text
desired/<scope>/<subject>/targets/<target-id>/settings.yaml
```

It stores actual desired scalar state in a manager-owned, driver-independent
shape:

```yaml
schema: dotfiles-manager.v2.desired-settings
schemaVersion: 1
values:
  <setting-id>:
    intent: set | delete | unmanaged
    kind: string | bool | number | null # required only for intent: set
    value: ...                          # omitted for delete, unmanaged, and null
```

`intent: set` means the desired state has a scalar value. String and bool values
use normal YAML scalar values. Number values are stored as a JSON-number lexical
string so they are not interpreted through YAML-specific number rules.
`kind: null` has no `value` field.

`intent: delete` is an explicit desired-present state: applying it removes the
selected live value if the selected-value driver supports that operation.

`intent: unmanaged` is an explicit intentionally-unmanaged state. It differs
from a missing `settings.yaml` file or a missing setting entry. Status and diff
logic must be able to distinguish:

- no desired artifact or no setting entry;
- desired present (`set` or `delete`);
- desired intentionally unmanaged.

The selected-value `settings.yaml` helpers are only for selected-value drivers.
They must not persist `file`, `file-tree`, native export/import, or opaque
payload state through this manager-owned scalar format.

Desired-value writes are gated even though #93 is storage-only. A write helper
must receive an explicit write-safety decision with recipe, exact setting
reference, source, trust, and approval flags. It must fail closed before
filesystem mutation when:

- the decision is absent;
- source is empty or unsupported;
- source is `local` without explicit trust;
- the exact setting or resource is not write-capable;
- the exact resource is not a selected-value driver;
- the desired value kind is incompatible with the selected-value driver:
  `ini-file` accepts only `string`, `delete`, and `unmanaged`; `json-file` and
  `yaml-file` accept JSON-compatible scalar values (`string`, `bool`, `number`,
  `null`), `delete`, and `unmanaged`;
- setting or resource sensitivity metadata is missing;
- setting or resource redaction metadata is missing;
- sensitivity is `secret` without explicit sensitive-value approval;
- sensitivity is `unknown` without explicit unknown-sensitivity approval;
- redaction is `blocked-save`;
- redaction is `redaction-unavailable` without explicit opaque-artifact
  approval.

This storage gate does not perform value-content secret scanning, persist trust
records, execute lifecycle actions, inspect live apps, or perform live writes.
Those behaviors belong to later command, secret, trust, lifecycle, and live-write
work.

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

URIs are internal identifiers. Normal user commands should prefer public target
and setting refs such as `git:user.email`; that public grammar is owned by
`00-vocabulary.md`.

### URI authority rules

`desired://` references must encode scope before target. Repository files remain
`manifest.yaml`, `settings.yaml`, and payloads under `artifacts/`; internal
`desired://` URIs intentionally use extensionless manager-object endpoints.

`state://` and `temp://` references must not be committed as desired artifacts
except as metadata references where explicitly allowed.

`secret://` may reference an external secret, but it must never contain the
secret value.

Canonical desired URI shapes for MVP:

```text
desired-manifest-uri = "desired://" desired-scope "/" desired-subject
                       "/targets/" target-id "/manifest"

desired-settings-uri = "desired://" desired-scope "/" desired-subject
                       "/targets/" target-id "/settings"
                       [ "#" setting-id ]

desired-artifact-uri = "desired://" desired-scope "/" desired-subject
                       "/targets/" target-id "/artifacts/" artifact-path

desired-scope = "shared" | "user" | "machine" | "machine-user"
desired-subject = "-" | user-id | machine-id | machine-id "/" user-id
```

Use `-` as the subject for `shared`, because shared scope has no user or
machine subject. The `<scope>`, `<subject>`, `<target-id>`, `<setting-id>`, and
`<artifact-path>` values in URI examples are logical URI segments, not raw
filesystem paths. URI escaping and encoding rules remain deferred.

The MVP URI parser intentionally supports only the canonical shapes above. Until
URI escaping rules are specified, it must fail closed on percent-encoding,
queries, userinfo or host ambiguity, backslashes, empty path segments, `.`,
`..`, absolute paths, fragments on `manifest` or `artifacts` URIs, and unknown
object endpoints.

## Derived schema boundaries, not final schemas

This spec owns the desired manifest and artifact-reference boundary.

Persisted objects:

| Object | Owned here? | Canonical path | Schema file | Notes |
| --- | --- | --- | --- | --- |
| Desired manifest | yes | `desired/.../targets/<target-id>/manifest.yaml` | `schemas/v2/desired-manifest.schema.json` | Index from settings to artifacts. |
| Desired settings | yes | `desired/.../targets/<target-id>/settings.yaml` | `schemas/v2/desired-settings.schema.json` | Default structured desired object for scalar/object settings. |
| Artifact URI | yes | Stored in manifests, profiles, ledgers, and CLI output; not a standalone file. | N/A | Logical grammar and allowed schemes. |
| Artifact metadata | yes | `desired/.../targets/<target-id>/manifest.yaml` | `schemas/v2/desired-manifest.schema.json` | Form, sensitivity, hash, origin, and per-payload schema/version context. |
| Artifact payload | partial | `desired/.../targets/<target-id>/artifacts/...` | Recorded by `manifest.yaml` | Payload shape depends on driver/setting and metadata in `manifest.yaml`. |
| Opaque metadata | partial | `desired/.../targets/<target-id>/manifest.yaml` | `schemas/v2/desired-manifest.schema.json` | Encryption details deferred. |

Desired manifests and desired settings use fully qualified schema identifiers
`dotfiles-manager.v2.desired-manifest` and
`dotfiles-manager.v2.desired-settings`. Final field schemas must define
per-artifact schema versions independently from root config, profile, recipe,
ledger, and backup versions.

## Examples

Examples in this spec demonstrate canonical desired URI, public-ref, and path
shape. YAML field names remain sketches until the desired manifest or desired
settings schema is promoted.

### Git email desired artifact

```yaml
artifact: desired://user/leon/targets/git/settings#user.email
form: structured
setting: git:user.email
```

### Illustrative-only shared JSON

```yaml
artifact: desired://shared/-/targets/example-tool/artifacts/user-info.json
form: structured
setting: example-tool:user-info
```

### Candidate Raycast opaque export

Raycast is a native-app candidate until current export/import support is
verified. This snippet only describes desired artifact shape.

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
- remote artifact storage;
- cross-machine merge;
- arbitrary templating.

## Spec follow-ups / open decisions

- Decide exact URI grammar and escaping rules.
- Decide final artifact hash and metadata fields.
- Decide exact opaque bundle encryption policy.
