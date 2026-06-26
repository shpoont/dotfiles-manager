---
owner: Core Engineering
document-type: v2-draft-spec
status: Draft
last-updated: 2026-06-26
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

### Runtime secret detection for selected values

MVP selected-value desired writes must run a deterministic local secret-detection
gate after URI, recipe metadata, trust, capability, sensitivity, and redaction
checks pass, and before any filesystem side effects such as creating desired
directories, settings files, temporary files, backups, or ledgers.

The gate scans only value-bearing string `set` intents. `delete`, `unmanaged`,
`null`, boolean, and number intents do not carry secret strings and are not
classified by this gate. The scanner must inspect the original trimmed string
and conservative normalized forms for multiline or escaped secret material such
as private-key headers. Generic high-entropy detection must require an
explicit credential-like public setting/resource context, a minimum length, a
deterministic entropy threshold, and character-class diversity; random-looking
values for non-credential settings such as themes must not be blocked by generic
entropy alone.

The MVP blocked categories include private-key headers, common access-token and
API-key shapes, JWT-like bearer material, and sensitive-context high-entropy
strings. A finding always blocks selected-value desired persistence, even when
recipe metadata says the setting is `personal`, `low`, `known-safe`, or
`redacted-for-display`. Changing recipe metadata must not bypass runtime secret
detection. False positives are handled by changing the selected value, choosing
a different non-secret setting, excluding the setting from management, or a
future explicitly reviewed allow mechanism; there is no automatic local
whitelist or unconfirmed workaround in MVP.

Secret-detection diagnostics are metadata-only. They may include stable pattern
IDs, categories, public setting refs, and schema paths. They must not include
matched substrings, raw values, entropy samples, local secret-bearing paths,
captured output, or arbitrary user-provided metadata that could itself contain
secret material. Text output, JSON output, logs, errors, preview snapshots, and
debug formatting of selected-value containers must remain redaction-safe.

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

Remote recipe catalog runtime implementation, signed downloads, and update
mechanics are deferred to #229; the accepted catalog trust/origin/write-authority
model is owned by `17-catalog-trust-origin-model.md`.

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

Trust records are local-only state, not stored settings and not portable
settings-folder data. They live under a platform local state root outside the
settings folder:

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
must not be written inside the settings folder, stored settings, profile
files, recipe files, or stored artifact payloads. The MVP trust evaluator must
reject a state root that resolves inside the settings folder. It must also
reject symlinked trust state paths when reading or writing trust records,
including symlinked state roots, `trust/` directories, and `trust-record.yaml`
files. A future in-settings-folder local-state override, if ever supported,
requires a separate opt-in design and must still be ignored by normal synced
settings-folder content.

Trust-record fingerprints are metadata-only:

- `contentSHA256` covers the canonical validated recipe object;
- `writeSurfaceSHA256` covers the write-relevant declaration surface only:
  target/schema version, effective write-capable settings/resources,
  capabilities, named locations and defaults, paths, selectors, include/exclude
  globs, sensitivity, redaction, lifecycle, artifact form, scope default, and
  exact native-operation execution metadata, including operation IDs, kind,
  runner, platforms, artifact/diff modes, lifecycle, working directory,
  executable, argv token structure, environment declarations, IO specs,
  timeout, expected exits, capture policies, and redaction policy;
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

If constrained native command IO is implemented for bundled or explicitly trusted
reviewed recipes, it must use:

- argv arrays, not shell strings;
- fixed executable or reviewed command source, never inherited `PATH` lookup;
- validated paths, named locations, and operation-kind-specific IO roots;
- import operations closed to manager-owned `artifact`/`temp` roots only; live
  named-location roots are forbidden through input, output, temp, argv, and
  environment channels;
- non-inherited empty environment plus explicit safe `DFM_` declarations;
- timeout;
- declared input/output files;
- no secret printing;
- verification after import/export.

`reviewed: true` is not sufficient for user-local recipes. Bundled recipes may
carry reviewed native metadata because the bundle is code-reviewed with the
manager release. Local recipes with native operations require external trust
evidence whose recipe content and native-operation write surface still match at
execution time, and whose trust record explicitly reviewed native operations.
Without that evidence, native operations are blocked before execution even if
the local YAML says `reviewed: true`.


Native export execution itself can be privacy-sensitive even when it is
read-only. Recipes may mark an export as review-required for opaque,
account-bound, large, or privacy-sensitive payloads. Commands must check that
pre-export gate before executing the native runner. In non-interactive or JSON
flows without an accepted opt-in, the command must return a stable safety
diagnostic instead of running the export.


The runner must reject environment inheritance, implicit working directories,
partial-token interpolation, undeclared IO refs, unsafe executable resolution,
shell/script-host executables, execution-influencing env names, and unbounded
stdout/stderr capture before process execution. Diagnostics and explain output
should summarize native operation metadata only; raw argv, raw `exec` errors,
environment values, local paths, and captured output are not normal user-facing
data.

Unreviewed command-backed save/apply is deferred.


Native apply has an additional trust boundary: the reviewed import operation
must receive only a manager-owned temp copy of the stored payload, never the
settings-folder stored artifact path. Local native recipes must have matching trust
evidence for the exact native-operation write surface, including import and
verify declarations. `--yes` confirms the reviewed action but must not override
trust mismatch, lifecycle handling gaps, missing backup/verification policy,
backup failure, import failure, or verification failure.

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

`blocked` always blocks write planning. `allowed` and `warn` do not block the
metadata gate; `warn` must remain a non-blocking diagnostic.

Running-state policies are fail-closed and require explicit lifecycle target
metadata. The manager must not infer app/process identity from target IDs,
config paths, executable names, or bundled app knowledge. Supported MVP target
detection is exact `process-name` basename matching. Regexes, globs, shell
commands, argv matching, path scanning, arbitrary lifecycle scripts, force-kill,
and app-specific lifecycle hacks are outside the MVP. The default process-name
detector must use a controlled absolute platform process-list command, must not
resolve `ps` through inherited `PATH`, must run with a closed environment, and
must fail closed on timeout or process-list failure.

Live `apply` is the command that may evaluate running state and execute
lifecycle actions. `status`, `diff`, and `save` must not quit or reopen apps.
`block-if-running` detects and blocks if the target is running. `ask-to-quit`
asks the user to quit manually, then re-checks; `--yes` must not answer this
manual prompt and must block with a stable lifecycle diagnostic.
`quit-if-running` requires explicit confirmation and a declared managed quit
capability, then re-checks before writing. `reopen-if-stopped-by-tool` reopens
only when the manager itself stopped the app during the same apply.

Lifecycle action contexts must be bounded. Detection, managed quit, managed
reopen, and recheck must not be able to hang the command indefinitely. In
dry-run/preview reports, actual running-state detection is recorded as
`executed`; only prompt/quit/reopen control actions that would happen later are
recorded as `planned`.

If the manager stops an app, it should attempt reopen even when the later write
fails, and must report both the write result and reopen result. If write
succeeds but reopen fails, the write remains recorded and the run must still
surface the lifecycle failure. Non-interactive and JSON modes must not prompt;
they block with structured diagnostics whenever a lifecycle choice is required.

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

- local state, cache, and temp roots are platform-specific roots outside the
  settings folder;
- no root/sudo writes;
- no writes outside declared named locations;
- no system service-manager mutation in MVP;
- no TCC/privacy automation;
- path traversal is rejected;
- unsafe symlink traversal is rejected;
- case-conflict behavior is tested before settings-folder or live writes;
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
- remote recipe catalog runtime implementation;
- root/sudo writes;
- arbitrary scripts.

## Spec follow-ups / open decisions

- Decide exact trust-record invalidation rules.
- Decide whether constrained command IO is included in MVP local recipes or
  deferred.
