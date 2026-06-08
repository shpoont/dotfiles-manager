---
owner: Core Engineering
document-type: v2-draft-spec
status: Draft
last-updated: 2026-06-07
canonical-source: docs/internal/specs/v2/09-security-redaction-trust.md
source-concept-sections:
  - Security/privacy/trust model
  - Redaction outcomes
  - Platform/filesystem assumptions
  - Lifecycle policy
  - Native import/export
  - Likely failure modes
authority: Draft; non-authoritative until promoted by docs/internal/specs/v2/README.md
---

# v2 security, redaction, and trust

## Purpose

This spec defines v2 safety, privacy, redaction, trust, lifecycle, and platform
assumptions.

The product must default to not managing risky state. It must be difficult to
leak secrets, corrupt app data, or run untrusted recipe logic accidentally.

## Status and authority

This is a draft extraction from `../../scope/product-concept-v2.md`. It is not
implementation-authoritative until promoted through `README.md`.

Current v1 behavior remains governed by existing v1 specs and contracts.

## Source map and extraction notes

Extracted from the concept sections covering:

- default-deny boundaries;
- redaction outcomes;
- recipe trust;
- command execution boundaries;
- lifecycle policy;
- platform/filesystem assumptions;
- failure modes.

Deliberate non-decisions:

- exact interactive trust-prompt UI is deferred;
- exact secret-detection implementation is deferred.

## Terms owned by this spec

- forbidden state;
- sensitive value;
- redaction outcome;
- trust level;
- lifecycle policy;
- command execution boundary;
- platform assumption;
- safety blocker.

## Normative MVP rules

### Default-deny safety

The manager must not manage these categories by default:

- passwords;
- private keys;
- API tokens;
- account sessions;
- browser cookies;
- cloud account state;
- app caches;
- runtime state;
- logs with sensitive contents;
- opaque app databases;
- workspace-local state;
- TCC/privacy permissions;
- account-bound app history.

Recipes may explicitly mark a setting as allowed only when the value is portable
and the safety policy permits it.

### Sensitivity levels

MVP recipe metadata uses this closed sensitivity set:

| Level | Meaning | Write-safety default |
| --- | --- | --- |
| `low` | Non-sensitive portable value. | allowed with required redaction metadata |
| `personal` | Personal but portable value, such as an identity string. | allowed with required redaction metadata |
| `machine-local` | Value is tied to one machine or OS/user context. | allowed by this metadata gate; profile/scope policy may restrict portability |
| `secret` | Secret-bearing value or credential material. | blocked unless caller context explicitly allows sensitive values |
| `unknown` | Recipe cannot classify the value yet. | blocked unless caller context explicitly allows unknown sensitivity |

Approval state is not stored in recipe YAML. Context-specific approvals belong
to the write-planning context or future trust records.

### Redaction outcomes

Every value that may be displayed or saved should resolve to one redaction
outcome:

| Outcome | Meaning | Save allowed? |
| --- | --- | --- |
| `known-safe` | Recipe/driver proves value is not sensitive. | yes |
| `redacted-for-display` | Value may be saved/applied but hidden in output. | yes, with policy |
| `blocked-save` | Sensitive material would enter desired artifacts. | no |
| `redaction-unavailable` | Opaque/unknown format cannot be inspected. | only with opaque opt-in |

Diff renderers must obey redaction. JSON output must not leak redacted values.

### Recipe trust

Bundled recipes are trusted by the release process. User-local recipes must be
explicitly trusted before write-capable behavior. Untrusted recipes may be
inspect-only or blocked.

Write planning must receive an explicit recipe source context. Empty or unknown
source context fails closed before writes. `bundled` is trusted by the release
process. `local` requires evaluated trust evidence from an external local-state
trust record; caller-set booleans or hashes are not sufficient.

Recipe changes that broaden write scope, add native operations, change
sensitivity, or change lifecycle behavior must require review before writes.

Remote recipe catalog, signed downloads, and update trust are post-MVP.

### Recipe explanation safety

`recipe explain <target>` is read-only metadata output. It must not include
secret values, raw captures, session/account state, native command output,
unredacted sensitive data, or value-bearing defaults. Native operation details
must be summarized without raw argv, environment variables, captured output, or
local paths that may contain secrets.

Untrusted local recipes may be explained as metadata so the user can understand
what would be trusted, but write-capable behavior remains blocked until trust is
established. If a recipe cannot be rendered safely without exposing sensitive
metadata, the CLI must emit `metadata-render-blocked` and exit with safety exit
code `5`.

### Trust-record storage

Trust records are local-only state, not repository desired data. They live under
the platform local state root from `01-repository-layout.md`:

```text
<state-root>/trust/trust-record.yaml
```

`trust-record.yaml` carries:

```yaml
schema: dotfiles-manager.v2.trust-record
schemaVersion: 1
localRecipes:
  <target-id>:
    source: local
    target: <target-id>
    schemaVersion: 1
    contentSHA256: <canonical validated recipe hash>
    writeSurfaceSHA256: <canonical write-safety surface hash>
    writeSurface:
      target: <target-id>
      schemaVersion: 1
      capability: read-write
      locations: []
      settings: []
      resources: []
      nativeOperations:
        supported: false
        count: 0
        summary: none-declared-current-schema
    reviewedNativeOperations: false
```

The canonical schema file is `schemas/v2/trust-record.schema.json`. The record
must not be written inside the repository, `desired/`, profile files, recipe
files, or desired artifact payloads. The MVP trust evaluator must reject a
state root that resolves inside the repository. It must also reject symlinked
trust state paths when reading or writing trust records, including symlinked
state roots, `trust/` directories, and `trust-record.yaml` files. A future
in-repo local-state override, if ever supported, requires a separate opt-in
design and must still be ignored by normal synced repository content.

Trust-record fingerprints are metadata-only:

- `contentSHA256` covers the canonical validated recipe object;
- `writeSurfaceSHA256` covers the write-relevant declaration surface only:
  target/schema version, effective write-capable settings/resources,
  capabilities, named locations and defaults, paths, selectors, include/exclude
  globs, sensitivity, redaction, lifecycle, artifact form, scope default, and
  native-operation summary;
- no live files, desired values, raw captures, app data, command output, or
  secrets are read or stored.

Local trust evidence used by write safety must be produced by external
local-state evaluation. `ValidateWriteSafety` must recompute the current recipe
and write-surface fingerprints for the recipe being used and compare them to
private evaluated trust evidence. Naked `Trusted: true`, caller-set hashes, or
evidence from another recipe must fail closed.

Invalidation rules:

- missing local trust record -> `review-required`;
- content hash mismatch -> `review-required`;
- write-surface hash mismatch -> `review-required`;
- new or broadened write-capable metadata -> `review-required` and a broadened
  write-surface diagnostic;
- corrupt or invalid trust record -> blocked;
- unreviewed native operations -> blocked or review-required before writes.

### Command execution boundary

Arbitrary recipe scripts are not allowed in MVP.

If constrained native command IO is implemented for bundled reviewed recipes,
it must use:

- argv arrays, not shell strings;
- fixed executable or reviewed command source;
- validated paths and named locations;
- restricted environment;
- timeout;
- declared input/output files;
- no secret printing;
- verification after import/export.

Unreviewed command-backed save/apply is deferred.

### Lifecycle policy

MVP recipes declare lifecycle behavior at setting or resource level. Resource
lifecycle is required for write-capable resources. Setting lifecycle is
optional but enforced when present. Target/group lifecycle inheritance can be
added later if needed. Supported policy states include:

- `allowed`;
- `warn`;
- `blocked`;
- `ask-to-quit`;
- `quit-if-running`;
- `block-if-running`;
- `reopen-if-stopped-by-tool`.

`blocked` always blocks write planning. `ask-to-quit`, `quit-if-running`,
`block-if-running`, and `reopen-if-stopped-by-tool` also block by default until
a lifecycle engine or caller context explicitly handles lifecycle actions.
`allowed` and `warn` do not block the metadata gate; `warn` must remain a
non-blocking diagnostic.

If the manager stops an app, it should reopen it only when policy and user
confirmation permit that. Non-interactive mode must not silently quit or reopen
apps unless a future explicit safe flag is defined.

### Platform/filesystem assumptions

MVP platform support is explicit and capability-gated:

| Platform | MVP support | Notes |
| --- | --- | --- |
| macOS | supported | Primary supported platform for CLI/local state roots, portable file/config drivers, `plist-file`, and `macos-defaults-readonly` capabilities. |
| Linux | supported for portable targets | Supported for CLI/local state roots and portable `file`, `file-tree`, INI, JSON, YAML, and TOML drivers. macOS-specific targets and drivers are unsupported on Linux. |
| Windows | unsupported | Must fail before live reads or writes. Repository data may still be inspected as plain files by implementation tooling, but v2 runtime commands are not supported. |
| unknown OS | blocked | Must fail before live reads or writes. |

Unsupported OS, target, driver, or recipe/platform combinations must be reported
as unsupported or blocked metadata. Mutating commands must fail before live
reads, writes, native operations, lifecycle actions, backups, or ledgers for
the unsupported item. `recipe explain` may still describe unsupported platform
metadata safely because it is metadata-only.

Filesystem and process assumptions:

- local state, cache, and temp roots are the platform-specific roots defined by
  `01-repository-layout.md`;
- no root/sudo writes;
- no writes outside declared named locations;
- no system service-manager mutation in MVP;
- no TCC/privacy automation;
- path traversal is rejected;
- unsafe symlink traversal is rejected;
- case-conflict behavior is tested before repository or live writes;
- app databases are unmanaged unless a reviewed driver says otherwise;
- atomic write/replace behavior is used where supported by the platform and
  filesystem;
- executable bit and permission preservation is allowed only when a driver
  explicitly declares support for that platform; otherwise permission-changing
  apply operations are blocked or reported as unsupported.

## Derived schema boundaries, not final schemas

This spec owns safety policy, redaction, trust, and lifecycle boundaries.

Persisted/emitted objects:

| Object | Owned here? | Canonical path | Schema file | Notes |
| --- | --- | --- | --- | --- |
| Sensitivity policy | yes | recipe/profile policy fields | `schemas/v2/recipe.schema.json` and `schemas/v2/profile-layer.schema.json` where applicable | Field-level sensitivity enum deferred. |
| Redaction outcome | yes | emitted CLI/preview diagnostics | `schemas/v2/preview.schema.json` where persisted | Final enum deferred to JSON schemas. |
| Trust record | yes | `trust/trust-record.yaml` local state | `schemas/v2/trust-record.schema.json` | Trust decisions and reviewed recipe fingerprints. |
| Lifecycle policy | yes | recipe policy fields | `schemas/v2/recipe.schema.json` | Recipe policy shape. |
| Command-IO policy | partial | recipe native-operation fields | `schemas/v2/recipe.schema.json` | Only if included in MVP. |
| Security diagnostics | partial | emitted CLI/preview diagnostics | `schemas/v2/preview.schema.json` where persisted | CLI envelope owns output shape. |

## Examples

Examples use the public target/setting ref grammar owned by
`00-vocabulary.md`. Diagnostic field names and enum values remain sketches until
the owning schemas are promoted.

### Blocked secret

```text
git:credential.helper    blocked-save    credential material must not enter repo
```

### App must be closed

```text
example-tool:user-info    blocked-lifecycle    Example Tool must be closed before apply  # illustrative-only
```

### Opaque native export

```text
raycast:settings-and-data    redaction-unavailable    metadata-only diff; opt-in required
```

## Errors, blockers, and partial-result behavior

Security blockers include:

- secret detected;
- redaction unavailable without opaque opt-in;
- untrusted recipe;
- recipe change broadens write scope;
- unsafe path or symlink;
- lifecycle quit required but not confirmed;
- native command not reviewed or violates command boundary;
- unsupported platform.

Partial commands may continue only for independent items that do not depend on
blocked trust or policy decisions.

## Acceptance expectations

- Secret fixtures are blocked or redacted according to outcome.
- JSON and text diff fixtures prove redacted values are not leaked.
- Untrusted local recipe fixtures block writes.
- Recipe-broadening fixtures require review.
- Lifecycle fixtures cover allowed, warn, blocked, quit declined, and reopen
  failure.
- Path traversal and unsafe symlink fixtures are rejected.
- Platform fixtures cover macOS local-state roots, Linux XDG local-state roots,
  unsupported Windows/unknown OS blocking, unsupported driver gating, and safe
  `recipe explain` metadata for unsupported platforms.
- Filesystem fixtures cover case conflicts and unsupported permission or
  executable-bit changes.
- Native export fixtures cover timeout, opaque output, and verification failure.

## Out of scope

- persistent secret-manager integration beyond runtime prompts;
- TCC automation;
- browser/session migration;
- remote recipe catalog trust;
- root/sudo writes;
- arbitrary scripts.

## Spec follow-ups / open decisions

- Decide exact trust-record invalidation rules.
- Decide whether constrained command IO is included in MVP local recipes or
  deferred.
