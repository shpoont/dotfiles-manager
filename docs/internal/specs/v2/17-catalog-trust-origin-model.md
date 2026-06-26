---
owner: Product + Core Engineering
location: docs/internal/specs/v2/17-catalog-trust-origin-model.md
document-type: v2-active-behavior-spec
status: Active behavior spec
last-updated: 2026-06-26
canonical-source: docs/internal/specs/v2/17-catalog-trust-origin-model.md
source-issue: 227
authority: Authoritative v2 catalog/tap trust, recipe origin, update, disabling/removal, provenance, collision, and write-authority model for #214/#227; #228 and #229 implementation must conform to this model.
---

# v2 catalog trust and recipe origin model

## Purpose

This document defines the v2 model for recipe catalogs/taps before remote
catalog implementation starts. It accepts the existing bundled registry as the
first normalized catalog seed, defines how local and future remote recipe
sources are represented, and sets the trust/write-authority rules that must gate
any recipe that can change live settings.

The central rule is:

```text
Catalog availability allows discovery. Recipe write authority allows live writes.
They are separate permissions.
```

A catalog being known, enabled, fetched, or visible to the user is never enough
by itself to let recipes from that catalog write live app settings.

## Boundaries

In scope for issue #227:

- catalog/source identity and state;
- bundled/default catalog definition;
- local and remote catalog behavior at model level;
- effective recipe origin fields visible before writes;
- trust, provenance, update, disabling/removal, collision, and write-authority
  rules;
- command-execution boundary for remote catalogs;
- downstream requirements for #228 and #229.

Out of scope for issue #227:

- implementing catalog commands, storage, network fetching, signature
  verification, or runtime write paths;
- allowing remote recipes to write live settings;
- replacing the local recipe trust-record implementation;
- production end-user documentation;
- backup/restore, v1 migration, or app installation workflows.

## Core concepts

Use four concepts and keep them distinct.

| Concept | Meaning | Authority |
| --- | --- | --- |
| Catalog | Data that lists available recipe candidates. | No write authority by itself. |
| Catalog source / tap | A locally configured origin from which a catalog is discovered, such as bundled, local, or remote. | May allow discovery when enabled. |
| Recipe origin | Manager-resolved provenance for one recipe candidate. | Must be visible before writes. |
| Write grant | Local-only authority allowing a specific recipe origin to write live settings. | Required for non-bundled writes. |

Normal users should not need the word `tap` for the bundled happy path. Public
copy should prefer `recipe source` and `permission to change live settings`.
Advanced/JSON output may expose catalog/source/tap fields.

## Source-of-truth and storage rules

Catalog source state, remote source acceptance, and write grants are local
manager state. They must not be stored in the settings folder as portable synced
settings.

The settings folder may contain user-authored local recipe files under
`recipes/local/`, but that file presence is not write authority. A cloned or
shared settings folder must not be able to grant trust on a new machine.

Stored settings may carry non-authoritative provenance metadata that explains
which recipe last produced or synced them. That provenance is useful for status,
diff, audit, and troubleshooting, but it must never grant catalog trust or live
write authority.

## Catalog source record

Every catalog source resolves to a manager-owned source record. The exact schema
is a #228/#229 implementation detail, but the model requires these fields.

| Field | Required | Meaning |
| --- | --- | --- |
| `sourceId` | yes | Stable local identifier, for example `bundled`, `local`, or `remote:<id>`. |
| `sourceKind` | yes | One of `bundled`, `local`, or `remote`. Unknown kinds fail closed. |
| `catalogId` | yes | Stable catalog identity independent from display name. |
| `displayName` | yes | User-facing source/catalog name. Not a security identifier. |
| `originUri` | yes | Release, file, or remote origin URI. |
| `status` | yes | `candidate`, `enabled`, `disabled`, `blocked`, or `removed`. |
| `sourceAcceptance` | yes | Source-level discovery acceptance: `release-accepted`, `user-accepted`, `review-required`, `rejected`, or `invalid`. |
| `integrityState` | yes | `not-required`, `valid`, `missing`, `invalid`, `expired`, `revoked`, or `unsupported`. |
| `writeDefault` | yes | `allowed` or `denied`. Bundled is allowed; local and remote default denied. |
| `pinnedIdentity` | conditional | Release identity, canonical local path, or remote signing identity. Required when relevant to provenance/trust. |
| `lastSeenManifestDigest` | conditional | Digest of the latest validated manifest. Required for remote once fetched. |
| `lastFetchedAt` | conditional | Fetch timestamp for remote sources. |
| `updatePolicy` | yes | `release-only`, `manual`, or another explicitly accepted policy. |
| `disabledReason` / `blockedReason` | conditional | Metadata-only reason when the source is disabled or blocked. |

A recipe or remote manifest must not be allowed to self-declare its effective
`sourceKind`, `sourceId`, `sourceAcceptance`, `integrityState`, or write
authority. Those are computed by the manager from resolver state, release
context, local trust records, signatures, digests, and policy. Unknown `status`,
`sourceAcceptance`, `integrityState`, `signatureState`, `updatePolicy`, or
write-authority values deny writes.

`sourceAcceptance` is source-level permission to use a catalog for discovery. It
is deliberately separate from recipe-level `reviewStatus` and write grants.
A user can accept a remote catalog for discovery while every recipe from that
catalog still remains blocked for live writes.

## Bundled/default catalog

The existing bundled registry is accepted as the candidate seed for the v2
bundled catalog. It should be normalized as:

```yaml
sourceId: bundled
sourceKind: bundled
catalogId: org.dotfiles-manager.bundled
displayName: dotfiles-manager built-in recipes
originUri: app-release://dotfiles-manager/<manager-version>/recipes
status: enabled
sourceAcceptance: release-accepted
integrityState: not-required
writeDefault: allowed
updatePolicy: release-only
```

Current bundled fields remain valid as the runtime seed:

```text
source=bundled
recipeRef=recipe://bundled/<target-id>
trustStatus=trusted
```

The normative rule is that bundled trust comes from the dotfiles-manager release
process, not from recipe data claiming to be trusted. Bundled recipes still must
pass schema, capability, lifecycle, redaction, path, and command-boundary
validation before writes.

Bundled recipe updates happen only through manager releases. Installing or
updating the manager is the user's trust event for bundled recipes.

## Local catalog source

Local recipes in the settings folder are modeled as a local catalog source:

```yaml
sourceId: local
sourceKind: local
catalogId: local.settings-folder
originUri: file://<settings-folder>/recipes/local
status: enabled
sourceAcceptance: review-required
integrityState: not-required
writeDefault: denied
updatePolicy: manual
```

A local recipe candidate resolves to:

```text
recipeRef=recipe://local/<target-id>
sourceKind=local
writeAuthority=requires-approval or allowed when a matching local write grant exists
```

Local recipe writes require local trust evidence outside the settings folder.
The existing local trust-record model remains the implementation input:

- canonical recipe content digest;
- write-surface digest;
- target/schema identity;
- reviewed-native-operation state;
- local state path outside the settings folder.

Editing a local recipe, switching branches, replacing the settings folder, or
changing the write surface invalidates the grant unless the recomputed digest and
write-surface evidence still match exactly.

## Remote catalog source

Remote catalogs are future runtime work for #229. This spec defines the model
that #229 must implement before any remote recipe can write live settings.

Adding, discovering, fetching, or enabling a remote catalog authorizes discovery
only. It does not authorize writes.

Remote source states:

| Status | Meaning | Discover? | Write live settings? |
| --- | --- | --- | --- |
| `candidate` | Suggested or discovered but not accepted. | May be shown as a suggestion. | No. |
| `enabled` | User accepted the catalog identity for discovery. | Yes. | Only if the recipe has write authority. |
| `disabled` | User paused the source. | No normal resolution. | No. |
| `removed` | User forgot the source. | No. | No. |
| `blocked` | Integrity, schema, revocation, or policy failure. | No. | No. |

Remote manifests must have a verifiable publisher identity before recipes from
them can be write-capable. #229 must define the concrete signature format,
identity pinning, key rotation, revocation, and fetch/cache mechanics before
runtime support is accepted.

If a fetched remote manifest declares a `catalogId` that differs from the
locally pinned catalog source record, the source must be blocked for writes and
existing recipe grants from that source must not authorize live writes.

At minimum, remote write-capable use requires all of these:

1. the remote source is `enabled`;
2. the catalog identity is pinned locally;
3. the manifest integrity/signature state is `valid`;
4. the manifest is not expired, revoked, unsupported, or blocked;
5. the recipe package/content digest matches the manifest entry;
6. the recipe has a local write grant for the exact digest and write surface;
7. the recipe origin is displayed before the write;
8. command, path, redaction, lifecycle, platform, and capability checks pass.

For v2, write grants are exact-digest and exact-write-surface grants. A remote
recipe update does not inherit live-write authority. Broader policies such as
"trust future updates from this catalog" are future advanced work and are not
the default model.

## Catalog manifest model

The exact schema belongs to #229, but any remote-capable manifest must be data,
not executable plugin code. Required conceptual fields:

| Field | Meaning |
| --- | --- |
| `schemaVersion` | Catalog manifest schema version. Unknown versions fail closed. |
| `catalogId` | Stable publisher/catalog identity. |
| `displayName` | User-facing catalog name. |
| `publisher` | Publisher or owner display name. |
| `manifestVersion` | Manifest version/revision. |
| `generatedAt` / `expiresAt` | Freshness metadata for remote validation. |
| `recipes[]` | Recipe index entries. |
| `signatures[]` | Required for remote catalogs. |

Required recipe index fields:

| Field | Meaning |
| --- | --- |
| `recipeId` | Unique recipe identifier within the catalog. |
| `targetId` | Target app/tool ID. Not globally unique across catalogs. |
| `displayName` | User-facing app/tool name. |
| `recipeVersion` | Recipe version. Not authority by itself. |
| `recipeDigest` | Canonical digest of the recipe package/content. |
| `packageRef` | Catalog-relative or otherwise signed/pinned package reference. |
| `supportLevel` | Support label, not a trust label. |
| `capability` | Declared recipe capability. Unknown values fail closed. |
| `platformSupport` | Supported operating systems/platforms. |
| `declaredReadScopes` | Metadata-only summary of reads. |
| `declaredWriteScopes` | Metadata-only summary of writes. |
| `nativeOperationRefs` / `commandRefs` | References to manager-known reviewed operations only. |
| `minManagerVersion` | Minimum compatible manager version. |
| `provides`, `conflicts`, `replaces` | Optional collision/migration metadata. |
| `deprecation` | Optional deprecation/replacement metadata. |

The canonical digest covers the canonical validated recipe package/content and
write-relevant declarations, not mutable URLs, display names, fetched timestamps,
or untrusted source-provided trust labels.

## Effective recipe origin

Before any operation that can write live settings, the manager must resolve and
show an effective recipe origin object. This object is computed by the manager;
it is not copied from recipe-provided trust metadata.

Required effective fields:

| Field | Meaning |
| --- | --- |
| `targetId` | App/tool being managed. |
| `displayName` | User-facing target name. |
| `recipeRef` | Stable resolved recipe reference. |
| `sourceKind` | `bundled`, `local`, or `remote`. |
| `sourceId` | Local catalog source identifier. |
| `catalogId` | Catalog identity. |
| `sourceDisplayName` | Human-readable recipe source. |
| `originUri` | Release, local path, or remote URL. |
| `recipeId` | Recipe ID inside the source/catalog. |
| `recipeVersion` | Recipe version. |
| `recipeDigest` | Canonical recipe/package digest where applicable. |
| `manifestDigest` | Remote manifest digest where applicable. |
| `signatureState` | `not-required`, `valid`, `missing`, `invalid`, `expired`, or `revoked`. |
| `reviewStatus` | Recipe-level write review: `release-reviewed`, `user-reviewed`, `review-required`, or `unknown`. |
| `writeAuthority` | `allowed`, `requires-approval`, `denied`, or `blocked`. |
| `capability` | Declared capability. |
| `declaredReadScopes` | User-readable summary of reads. |
| `declaredWriteScopes` | User-readable summary of writes. |
| `selectedBy` | `bundled-default`, `user-selected`, `pinned`, or `collision-resolution`. |

Current compatibility fields map into this object:

| Current field | Effective origin mapping |
| --- | --- |
| `source=bundled` | `sourceKind=bundled`, `sourceId=bundled`. |
| `recipeRef=recipe://bundled/<target>` | Bundled recipe reference. |
| `trustStatus=trusted` | Compatibility display for `reviewStatus=release-reviewed` and `writeAuthority=allowed`. |
| `source=local` | `sourceKind=local`, `sourceId=local`. |
| `recipeRef=recipe://local/<target>` | Local recipe reference. |
| `trustStatus=untrusted` / `review-required` | Compatibility display for missing local write grant. |

Remote recipe references should follow this shape unless #229 records a managed
change:

```text
recipe://remote/<catalog-id>/<recipe-id>@<recipe-version>
```

The digest remains the write-authority key. A version string or recipeRef alone
is not enough to grant writes.

## Write grants

A write grant is local-only authority for a non-bundled recipe origin to change
live settings. It must live outside the settings folder and must contain no
secret values.

Minimum write-grant fields:

| Field | Meaning |
| --- | --- |
| `sourceKind` / `sourceId` | Source being granted. |
| `catalogId` | Catalog identity. |
| `recipeRef` | Resolved recipe reference. |
| `targetId` | Target app/tool. |
| `recipeDigest` | Exact recipe/package digest. |
| `writeSurfaceSHA256` | Exact write-relevant surface hash. |
| `declaredWriteScopes` | Metadata-only write summary reviewed by the user. |
| `reviewedNativeOperations` | Whether native operations were reviewed for this grant. |
| `grantedAt` | Grant timestamp. |
| `grantReason` / `reviewSummary` | Metadata-only review explanation. |

Remote grants additionally require the pinned signing identity and manifest
context used for review. Local grants continue to use the existing local trust
record model.

Any change to the recipe digest, write surface, native operation surface,
source identity, catalog identity, pinned signing identity, or declared write
scope invalidates the grant until reviewed again.

## Write-authority decision

A write to live settings is allowed only when all of these are true at planning
and execution time:

1. the recipe has a known `sourceKind`;
2. the source is not empty, unknown, disabled, removed, blocked, invalid,
   expired, or revoked;
3. the recipe origin was resolved by the manager;
4. the recipe digest matches the resolved bundled/local/catalog content;
5. the operation is within declared read/write scopes;
6. schema, platform, support, capability, lifecycle, redaction, path, symlink,
   and command-boundary checks pass;
7. the recipe origin and write surface are visible in the write plan before the
   write;
8. the effective `writeAuthority` is `allowed`.

Source-specific rules:

| Source | Write rule |
| --- | --- |
| `bundled` | Allowed by release trust when all validation gates pass. |
| `local` | Denied until a local write grant/trust record matches the current digest and write surface. |
| `remote` | Denied until the source is enabled, identity-pinned, signature-valid, unexpired, and the exact recipe digest/write surface has a local write grant. |
| empty / unknown | Always denied. |
| disabled / removed / blocked | Always denied. |
| invalid / missing required signature | Always denied. |
| digest or write-surface mismatch | Always denied. |
| unsupported schema/capability/platform | Always denied. |

Non-interactive commands must fail closed instead of prompting, auto-accepting,
or treating `--yes` as trust approval.

## Update rules

### Bundled updates

Bundled recipe updates are delivered through manager releases. Explain/security
output should include bundled version/digest provenance where available, but
bundled write authority remains release-process authority.

### Local updates

Local recipe content or write-surface changes invalidate the existing grant.
The next live write requires review again.

### Remote updates

Remote updates may be fetched for discovery, but they must not silently expand
write authority.

Rules #229 must implement or explicitly recontract before runtime support:

- catalog identity is pinned when accepted;
- remote manifests validate against the pinned identity/signing key;
- key rotation requires proof from the old identity or explicit user acceptance;
- changed recipe digest requires a new write grant;
- expanded read/write scopes require reapproval;
- changed native/command capability requires reapproval;
- invalid signatures, revoked identities, unsafe schemas, or unsupported
  capabilities block the source or manifest update;
- expired manifests cannot authorize remote writes;
- offline cached remote writes are not accepted until a separate offline/cache
  trust policy is designed.

## Disable, remove, and block semantics

| State | Behavior |
| --- | --- |
| `disabled` | Source remains known, is ignored for normal resolution, and existing grants are suspended. Stored settings are untouched. |
| `removed` | Source is forgotten. Related remote source trust state and write grants are deleted or marked unusable. Stored settings are untouched. |
| `blocked` | Manager refuses to use the source because of integrity, schema, identity, revocation, or policy failure. Existing grants are unusable while blocked. |

Commands referencing recipes from disabled, removed, or blocked sources must fail
closed with a stable diagnostic that explains the source state.

## Provenance layers

Use two provenance layers.

Authoritative local source/trust state, outside the settings folder:

```yaml
sourceId: remote:acme-tools
catalogId: tools.acme.catalog
pinnedIdentity: SHA256:...
status: enabled
integrityState: valid
writeGrants: []
```

Non-authoritative stored-settings provenance, inside stored settings when useful:

```yaml
targetId: example-tool
lastRecipeRef: recipe://remote/tools.acme.catalog/example-tool@1.2.0
lastRecipeVersion: 1.2.0
lastRecipeDigest: sha256:...
lastSourceKind: remote
lastSourceDisplayName: Acme Tools
lastSyncDirection: live_to_stored
lastSyncAt: 2026-06-26T00:00:00Z
```

The second layer helps explain history. It must not be read as trust evidence.

## Collision and precedence rules

`targetId` is not globally unique. Multiple catalogs may offer recipes for the
same target. `recipeRef` plus digest identifies the candidate that can receive
write authority.

Default resolution:

1. user-pinned `recipeRef` when it remains resolvable and allowed;
2. bundled default for bundled-supported targets;
3. no automatic choice; show local/remote candidates that require explicit
   selection.

Rules:

- a remote recipe must never silently override a bundled recipe;
- a local recipe must never silently override a bundled recipe;
- display names are not security identifiers;
- duplicate recipe IDs inside one catalog are invalid unless #229 defines an
  explicit variant mechanism;
- collisions are visible before writes and require explicit selection when the
  bundled default is not used.

This preserves the current implementation behavior where bundled lookup wins
before local lookup and turns that behavior into the accepted default model.

## Command-execution boundary

Remote catalogs are data sources, not plugins.

Allowed recipe operation references:

- declarative file/config resources using manager-owned drivers;
- manager-provided native adapters;
- constrained import/export operation references whose command template,
  executable, arguments, environment, IO roots, timeout, working directory,
  capture policy, and verification are owned by the manager release or by a
  separately accepted reviewed mechanism.

Remote catalogs and remote recipes must not provide or enable:

- shell strings;
- arbitrary scripts;
- pre-sync or post-sync hooks;
- arbitrary executable paths;
- inherited environment or `PATH` lookup;
- dynamic environment expansion beyond manager-approved metadata;
- network calls during sync;
- plugin loading;
- interpreter invocation such as `bash`, `python`, `node`, `osascript`,
  PowerShell, or equivalent script hosts from recipe data;
- command arguments that can turn a reviewed command into arbitrary execution.

A remote recipe may reference a manager-known operation such as:

```yaml
adapter: builtin.example-tool.settings
operation: export
```

It must not define executable command text such as:

```yaml
command: "example --export && curl ..."
```

Path safety remains mandatory for all sources:

- normalize paths before use;
- reject path traversal;
- reject unsafe symlink traversal;
- validate platform-specific expansion;
- treat destructive writes, deletes, imports, app reloads, native import/export,
  and lifecycle actions as live writes.

Read-side behavior is also safety-relevant. Recipes that can read arbitrary
files or run commands are not harmless just because the command is `status` or
`diff`; read scopes and command constraints apply before reads too.

## User-facing examples

### Bundled recipe before write

Normal users should see this as built-in app support, not as catalog/tap
management.

```text
Target: git
Recipe source: built in with dotfiles-manager
Recipe: recipe://bundled/git
Writes live settings: ~/.gitconfig [user] email, [user] name
Permission: allowed by bundled release
```

### Local recipe before write

```text
Target: example-tool
Recipe source: local recipe in this settings folder
Recipe: recipe://local/example-tool
Writes live settings: ~/.config/example-tool/config.yaml
Permission: review required before this recipe can change live settings
Result: blocked until approved
```

### Remote catalog enabled, recipe not granted

```text
Target: example-tool
Recipe source: remote catalog "Acme Tools" <https://example.com/catalog.json>
Recipe: recipe://remote/tools.acme.catalog/example-tool@1.2.0
Recipe digest: sha256:abcd...
Writes live settings: ~/.config/example-tool/config.yaml
Permission: catalog enabled for discovery; recipe write approval required
Result: blocked until approved
```

### Remote recipe granted

```text
Target: example-tool
Recipe source: remote catalog "Acme Tools" <https://example.com/catalog.json>
Recipe: recipe://remote/tools.acme.catalog/example-tool@1.2.0
Recipe digest: sha256:abcd...
Writes live settings: ~/.config/example-tool/config.yaml
Permission: allowed for this exact recipe digest and write surface
Result: ready to write after the normal sync confirmation
```

### Remote recipe changed after approval

```text
Target: example-tool
Recipe source: remote catalog "Acme Tools"
Recipe: recipe://remote/tools.acme.catalog/example-tool@1.3.0
Previous approved digest: sha256:abcd...
Current digest: sha256:ef01...
Permission: blocked because the recipe changed since approval
Result: review the new version before applying it
```

### Remote source disabled

```text
Target: example-tool
Recipe source: remote catalog "Acme Tools"
Permission: blocked because the catalog source is disabled
Stored settings: unchanged
Result: enable the source and review the recipe before any live write
```

### Remote source blocked

```text
Target: example-tool
Recipe source: remote catalog "Acme Tools"
Permission: blocked because the catalog signature is invalid
Stored settings: unchanged
Result: do not use this recipe until the catalog source is repaired or replaced
```

## JSON example shape

Field names are conceptual until #228/#229 promote runtime schemas.

```json
{
  "recipeOrigin": {
    "targetId": "example-tool",
    "displayName": "Example Tool",
    "recipeRef": "recipe://remote/tools.acme.catalog/example-tool@1.2.0",
    "sourceKind": "remote",
    "sourceId": "remote:acme-tools",
    "catalogId": "tools.acme.catalog",
    "sourceDisplayName": "Acme Tools",
    "originUri": "https://example.com/catalog.json",
    "recipeId": "example-tool",
    "recipeVersion": "1.2.0",
    "recipeDigest": "sha256:abcd...",
    "manifestDigest": "sha256:manifest...",
    "signatureState": "valid",
    "reviewStatus": "review-required",
    "writeAuthority": "requires-approval",
    "capability": "read-write",
    "declaredReadScopes": ["~/.config/example-tool/config.yaml"],
    "declaredWriteScopes": ["~/.config/example-tool/config.yaml"],
    "selectedBy": "user-selected"
  }
}
```

## Required fixture matrix for #228/#229

#228/#229 must turn this model into inspectable fixture scenarios before
claiming implementation acceptance. The minimum matrix is:

| Scenario | Required result |
| --- | --- |
| Bundled default recipe | Source shown as built in; write authority allowed after normal safety checks. |
| Bundled + local collision | Bundled remains default; local candidate is visible but cannot override silently. |
| Local recipe without grant | Candidate/explain visible; live write blocked as review required. |
| Local recipe with matching grant | Live write allowed only for matching digest/write surface after normal confirmation. |
| Remote source enabled but recipe ungranted | Discovery allowed; live write blocked as recipe approval required. |
| Remote recipe with matching grant | Live write allowed only for exact digest/write surface after normal confirmation. |
| Remote recipe digest/write surface changed | Existing grant invalidated; live write blocked pending review. |
| Remote source disabled | Source ignored/suspended; live write blocked; stored settings untouched. |
| Remote invalid signature/revoked/unsafe schema | Source blocked; discovery/write blocked with stable diagnostic. |
| Remote recipe declares executable command text, interpreter invocation, unsafe args, or external hook | Source/recipe fails command-boundary validation; live reads and writes are blocked. |
| Unknown source kind or enum value | Fail closed before live reads or writes. |

## Requirements for #228

#228 must implement built-in/local catalog discovery against this model:

- expose the bundled registry as the normalized bundled catalog/source;
- preserve bundled-first precedence;
- show local recipes as local candidates, not bundled replacements;
- keep local write authority bound to the external local trust record;
- make source/origin fields visible in list/explain/planning output before
  writes;
- fail closed for unknown source kinds.

## Requirements for #229

#229 must not implement remote writes until it specifies and verifies:

- remote source configuration and local state location;
- manifest schema and signature verification;
- identity pinning, key rotation, revocation, expiry, and cache/offline policy;
- exact digest and write-surface canonicalization;
- write-grant storage outside the settings folder;
- disable/remove/block behavior;
- collision/selection UI and non-interactive failure behavior;
- command-execution boundary enforcement;
- blackbox examples showing blocked-by-default remote writes.

Remote catalog discovery without writes may be implemented earlier only if it
cannot grant live write authority and all output makes that limitation explicit.

## Acceptance checklist for #227

- Catalog data model is specified through catalog source records, manifests,
  recipe origins, and write grants.
- Recipe origin is visible before writes through the effective origin object and
  examples.
- Trust/write-authority rules are defined before remote writes are allowed.
- The bundled registry is accepted as the normalized bundled/default catalog
  seed.
- Remote catalogs are data-only and cannot become arbitrary command execution.
