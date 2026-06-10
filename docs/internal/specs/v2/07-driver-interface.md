---
owner: Core Engineering
document-type: v2-draft-spec
status: Draft
last-updated: 2026-06-10
canonical-source: docs/internal/specs/v2/07-driver-interface.md
source-concept-sections:
  - Driver
  - Driver interface contract
  - Selector contracts
  - Initial MVP drivers
  - Native import/export
authority: Draft; non-authoritative until promoted by docs/internal/specs/v2/README.md
---

# v2 driver interface

## Purpose

This spec defines the draft interface for deterministic drivers. Drivers are the
reviewed code that reads, normalizes, diffs, previews, writes, verifies, and
restores target resources.

Recipes configure drivers. Recipes do not implement driver logic.

## Status and authority

This is a draft extraction from `../../scope/product-concept-v2.md`. It is not
implementation-authoritative until promoted through `README.md`.

Current v1 behavior remains governed by existing v1 specs and contracts.

## Source map and extraction notes

Extracted from the concept sections covering:

- resource/driver model;
- driver operation contract;
- selector validation;
- initial MVP drivers;
- command-backed/native import-export restrictions.

Deliberate non-decisions:

- exact programming interface is deferred;
- exact normalized hash algorithm is deferred;
- final selector grammar per structured driver is deferred.

## Terms owned by this spec

- driver;
- resource;
- selector;
- raw capture;
- normalized value;
- preview;
- verification;
- restore support;
- typed driver error.

## Normative MVP rules

### Driver ownership

Drivers are reviewed deterministic code shipped with the manager. A recipe may
select a driver and provide validated parameters, but it must not override driver
implementation logic.

### Required operations

A write-capable driver must expose these operations or explicitly declare that
an operation is unsupported:

| Operation | Required behavior | Side effects allowed |
| --- | --- | --- |
| `detect` | Determine whether resource exists and is readable. | no mutation |
| `readCurrent` | Read live state into raw capture or structured value. | read/temp only |
| `normalize` | Convert input to deterministic comparison form. | none |
| `diff` | Compare normalized current and desired state. | none |
| `previewApply` | Describe writes, deletes, creates, backups, risks. | none |
| `backup` | Capture pre-write restore material where supported. | local state only |
| `apply` | Mutate live state according to desired input. | declared paths only |
| `verify` | Prove or fail the write result. | read/temp only |
| `restore` | Restore from compatible backup when supported. | declared paths only |

Read-only drivers must not expose writes through recipe overrides.

### Static explanation metadata

Drivers must expose static, redacted-safe explanation metadata separately from
live operations. `recipe explain` may use this metadata to summarize selector
support, operation support, backup/restore support, normalization mode, diff
mode, and limitations without calling `detect`, `readCurrent`, `normalize`,
`diff`, `backup`, `apply`, `verify`, `restore`, or native command operations.

Driver explanation metadata must not include raw captures, live values,
value-bearing defaults, raw command argv, environment variables, or local paths
that may contain secrets. It may identify driver IDs, selector kinds, supported
operation names, and limitation summaries.

### Initial MVP drivers

Initial MVP drivers are:

- `file`;
- `file-tree`;
- `ini-file`;
- `json-file`;
- `yaml-file`;
- `toml-file`;
- `plist-file`;
- `macos-defaults-readonly`;
- `native-export` for reviewed native export artifacts and explicitly declared
  native import/apply flows.

For #110, import-capable `native-export` resources remain `read-write`; the
`import-only` capability is reserved until a dedicated end-to-end capability
pass covers explain, add, resolution, preview, live execution, and write safety.

Write-capable `macos-defaults`, general `command-io`, `manual`, and
`do-not-manage` are not ordinary MVP write drivers unless separately specified.

### Selector contracts

- `file` accepts one resolved file path under one named location.
- `file-tree` accepts include/exclude globs rooted at one named location.
- `ini-file` accepts section/key selectors and duplicate-key rules.
- `json-file` accepts recipe-defined path selectors with no expressions.
- `yaml-file` accepts recipe-defined path selectors with no expressions.
- `toml-file` accepts table/key selectors.
- `plist-file` accepts explicit key paths.
- `macos-defaults-readonly` accepts explicit domain/key selectors and cannot
  write in MVP.

Selectors must not escape recipe-declared resource boundaries.

#### MVP `file-tree` selector and metadata policy

For the MVP `custom.files` vertical slice, a `file-tree` resource has one named
location and one resource `path`. The effective resource root is the named
location root plus that resource path. Include/exclude globs are evaluated only
against slash-relative paths inside this effective resource root. The root
directory itself is implicit and is not represented by an empty relative path.

Default include is `["**"]`. Excludes override includes. If an exclude glob
matches a directory, that directory and its descendants are pruned. Directories
are managed when they match an include and are not excluded, or when they are
required parents of a selected descendant. Empty directories are preserved when
selected by the include/exclude policy.

The MVP case-conflict rule is intentionally stricter than any particular local
filesystem: selected relative paths are rejected before mutation when their
`strings.ToLower` form conflicts. This is a deterministic portability rule, not
a complete Unicode case-folding or filesystem-normalization model.

The MVP metadata policy is content-and-presence only:

- managed: relative path identity, regular-file bytes, regular-file presence,
  and directory presence;
- not managed by diff or verify: mtime, owner, group, xattrs, ACLs,
  app-specific metadata, and file mode bits except for deterministic write
  behavior;
- write behavior: new directories are created with `0755`, new files are
  created with `0644`, and updates to an existing regular file preserve that
  file's current permission bits while replacing bytes;
- unsupported and rejected before mutation: symlinks, hard-link identity,
  device files, sockets, FIFOs, and other special entries.

### Native-export driver apply contract

`native-export` is metadata-only from the manager's perspective. It does not
semantic-diff internal app settings. Export-only resources support `diff` and
`save` through reviewed export capture and block `apply`. Import-capable
resources support `apply` only when recipe validation has accepted an explicit
import operation and closed native apply policy.

A live native apply transaction must run in this order:

1. validate the desired native artifact and payload hash;
2. copy desired payload to a manager-owned temp input and validate the copy;
3. run a pre-apply export and persist it as local backup;
4. run the reviewed import operation using only the temp input root;
5. run a post-import export;
6. compare post-import payload hash and normalizer to desired;
7. write success/last-applied metadata only after verification passes.

If any step before import fails, import must not run. If import or verification
fails after backup, the run record must retain the backup reference but must not
write a successful ledger entry.

### Driver safety requirements

Drivers must:

- reject path traversal;
- reject unsafe symlink traversal;
- validate selectors before reads/writes;
- preserve redaction rules in diffs;
- preview writes without mutation;
- verify writes after apply when write is supported;
- report unsupported operations explicitly;
- version normalizers.

### Typed driver errors

Drivers must return typed errors including:

- `not-found`;
- `permission-denied`;
- `invalid-selector`;
- `unsafe-path`;
- `secret-detected`;
- `lifecycle-blocked`;
- `verification-failed`;
- `unsupported`;
- `internal-error`.

## Derived schema boundaries, not final schemas

This spec owns driver operation and selector boundaries.

Persisted/emitted objects:

| Object | Owned here? | Notes |
| --- | --- | --- |
| Driver ID enum | yes | Final enum deferred to schemas. |
| Selector syntax | partial | Per-driver grammar needs final specs. |
| Normalized value metadata | partial | Ledger stores hashes/version. |
| Raw capture metadata | partial | Security spec controls retention/redaction. |
| Driver error enum | yes | Final JSON shape deferred. |

## Examples

Examples demonstrate selector, normalizer, and driver contract shapes. YAML
field names remain sketches until the driver-related schemas are promoted.

### INI selector

```yaml
driver: ini-file
path: ~/.gitconfig
selector:
  section: user
  key: email
```

### File-tree selector

```yaml
driver: file-tree
root: ~/.config/nvim
include:
  - '**'
exclude:
  - '**/.git/**'
  - '**/tmp/**'
```

### macOS defaults read-only

```yaml
driver: macos-defaults-readonly
domain: com.apple.finder
key: AppleShowAllFiles
```

## Errors, blockers, and partial-result behavior

Driver errors must be reported per item. A failed driver operation must not
silently convert to `unchanged`.

If one resource fails but other settings are independent and safe, the command
may continue and return partial success.

## Acceptance expectations

- Fixtures cover each initial MVP driver.
- Fixtures prove path traversal and unsafe symlink traversal are rejected.
- Structured-driver fixtures cover invalid selectors and normalization.
- `previewApply` fixtures prove no mutation occurs.
- Backup/restore fixtures cover supported and unsupported restore paths.
- Redaction fixtures prove sensitive values are not leaked in diffs.

## Out of scope

- arbitrary script drivers;
- unreviewed command-backed save/apply;
- write-capable macOS defaults;
- database drivers;
- cloud account/session migration;
- service management.

### Reviewed native command runner boundary

The native runner is a constrained driver-like execution primitive, not a
general scripting interface.

The runner may execute only a declared native operation from reviewed recipe
metadata. It must reject unknown operation IDs, unreviewed operations,
unsupported platforms, missing or unknown recipe source, user-local recipes
without matching external local trust evidence, unsafe executable resolution,
undeclared IO references, implicit cwd, environment inheritance, path
traversal, and capture policies that could print raw stdout or stderr.

Execution uses:

- an argv array;
- a fixed reviewed absolute executable path or future bundled allowlisted
  command source, never inherited `PATH` lookup;
- typed whole-token path placeholders for declared inputs, outputs, and temp
  paths;
- a manager-owned temp working directory;
- a non-inherited empty environment plus explicit safe `DFM_` env declarations;
- a required timeout;
- bounded stdout/stderr policies;
- structured result metadata only.

The runner result may include status, exit code, duration, timeout, byte counts,
hashes, output-limit flags, and declared output IDs. It must not expose raw
captured command output, raw argv containing local/sensitive paths, or inherited
environment values. Execution failure diagnostics must be metadata-only and must
not return raw `exec` errors when those can include local executable paths or
argv details.

For `native-export` resources, the driver-like manager layer must run exports
only into temp staging. The reviewed command receives the staging `payload/`
directory as its artifact root. The manager then validates declared artifact
outputs, rejects symlinks and special files, enforces byte and entry limits,
computes a canonical payload tree hash from sorted relative paths plus per-file
hashes, and writes `metadata.json` beside `payload/` only after validation.
Metadata-only diffs compare payload summaries, not volatile runtime metadata such
as command duration or capture timestamps.


## Spec follow-ups / open decisions

- Define final programming interface for drivers.
- Define exact selector grammar for each structured driver.
- Define normalizer versioning and hash algorithm.
- Decide whether constrained command-backed native support is MVP-only bundled
  support or post-MVP.
