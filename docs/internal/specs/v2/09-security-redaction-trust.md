---
owner: Core Engineering
document-type: v2-draft-spec
status: Draft
last-updated: 2026-06-04
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

- exact trust-record schema is deferred;
- exact secret-detection implementation is deferred;
- exact platform matrix is deferred.

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

### Redaction outcomes

Every value that may be displayed or saved should resolve to one redaction
outcome:

| Outcome | Meaning | Save allowed? |
| --- | --- | --- |
| `known-safe` | Recipe/driver proves value is not sensitive. | yes |
| `redacted-for-display` | Value may be saved/applied but hidden in output. | yes, with policy |
| `blocked-save` | Sensitive material would enter desired artifacts. | no |
| `redaction-unavailable` | Opaque/unknown format cannot be inspected. | only with opaque opt-in |
| `user-approved-sensitive` | User approved a sensitive portable value. | only if recipe permits |

Diff renderers must obey redaction. JSON output must not leak redacted values.

### Recipe trust

Bundled recipes are trusted by the release process. User-local recipes must be
explicitly trusted before write-capable behavior. Untrusted recipes may be
inspect-only or blocked.

Recipe changes that broaden write scope, add native operations, change
sensitivity, or change lifecycle behavior must require review before writes.

Remote recipe catalog, signed downloads, and update trust are post-MVP.

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

Recipes may declare lifecycle behavior at target, settings group, or resource
level. Supported policy states include:

- `allowed`;
- `warn`;
- `blocked`;
- `ask-to-quit`;
- `quit-if-running`;
- `block-if-running`;
- `reopen-if-stopped-by-tool`.

If the manager stops an app, it should reopen it only when policy and user
confirmation permit that. Non-interactive mode must not silently quit or reopen
apps unless a future explicit safe flag is defined.

### Platform/filesystem assumptions

MVP should assume:

- no root/sudo writes;
- no writes outside declared named locations;
- path traversal is rejected;
- unsafe symlink traversal is rejected;
- case-conflict behavior is tested;
- app databases are unmanaged unless a reviewed driver says otherwise;
- platform support matrix is explicit.

## Derived schema boundaries, not final schemas

This spec owns safety policy, redaction, trust, and lifecycle boundaries.

Persisted/emitted objects:

| Object | Owned here? | Notes |
| --- | --- | --- |
| Sensitivity policy | yes | Recipe and profile policy fields. |
| Redaction outcome | yes | Final enum deferred to JSON schemas. |
| Trust record | partial | Exact storage path/format deferred. |
| Lifecycle policy | yes | Recipe policy shape. |
| Command-IO policy | partial | Only if included in MVP. |
| Security diagnostics | partial | CLI envelope owns output shape. |

## Examples

### Blocked secret

```text
git:credential.helper    blocked-save    credential material must not enter repo
```

### App must be closed

```text
cobona:user-info    blocked-lifecycle    Cobona must be closed before apply
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
- Native export fixtures cover timeout, opaque output, and verification failure.

## Out of scope

- persistent secret-manager integration beyond runtime prompts;
- TCC automation;
- browser/session migration;
- remote recipe catalog trust;
- root/sudo writes;
- arbitrary scripts.

## Spec follow-ups / open decisions

- Decide exact sensitivity levels.
- Decide exact trust-record storage and invalidation rules.
- Decide platform support matrix for MVP.
- Decide whether constrained command IO is included in MVP local recipes or
  deferred.
