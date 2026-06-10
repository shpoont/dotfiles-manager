---
owner: Core Engineering
document-type: v2-draft-spec
status: Draft
last-updated: 2026-06-10
canonical-source: docs/internal/specs/v2/08-mutation-ledger-backup-restore.md
source-concept-sections:
  - Mutation transaction model
  - Ledger commit rules
  - Local backup store
  - Restore behavior
  - CLI contract v2
authority: Draft; non-authoritative until promoted by docs/internal/specs/v2/README.md
---

# v2 mutation, ledger, backup, and restore

## Purpose

This spec defines the draft mutation transaction model for save, apply, backup,
ledger, partial failure, and restore behavior.

The product must be safe to use on live machines. Every live write needs a
preview, a backup when supported, verification, and an auditable result.

## Status and authority

This is a draft extraction from `../../scope/product-concept-v2.md`. It is not
implementation-authoritative until promoted through `README.md`.

Current v1 behavior remains governed by existing v1 specs and contracts.

## Source map and extraction notes

Extracted from the concept sections covering:

- mutation transaction model;
- ledger updates only after verified success;
- backup store;
- restore previews;
- partial success;
- status/conflict baselines.

Deliberate non-decisions:

- retention and pruning policy is deferred;
- exact ledger, run-record, backup, and preview fields are deferred.

## Terms owned by this spec

- mutation transaction;
- run ID;
- change preview;
- backup;
- ledger entry;
- verified success;
- partial success;
- restore plan.

## Normative MVP rules

### Transaction phases

A mutating command must follow these phases:

1. resolve profile, target, settings, and artifacts;
2. validate recipe, trust, selectors, lifecycle, and safety;
3. read and normalize current state where needed;
4. compute status and conflicts;
5. produce change preview;
6. collect required prompts unless non-interactive mode forbids them;
7. create backup for live writes where supported;
8. apply desired or save current state;
9. verify result;
10. write ledger entry only for verified outcomes;
11. report per-item success, skip, failure, and backup references.

### Save behavior

`save` writes repository desired artifacts. It must:

- preview artifact create/update/delete;
- apply redaction and secret policy before writing;
- record current normalized state as last-applied only after desired write is
  verified;
- avoid writing raw captures unless converted to allowed artifacts.

### Apply behavior

`apply` writes live target state. It must:

- preview live writes;
- evaluate lifecycle policy before backup or live write;
- create backup before live writes where supported;
- respect lifecycle policy;
- verify driver result;
- write last-applied state only after verified success;
- record partial failures.

Lifecycle actions are recorded as metadata-only item records in reports and run
records. Each lifecycle record should include the affected target/setting,
lifecycle target ID/display name, native operation ID when an operation-specific
policy was enforced, phase (`before-write` or `after-write`), action (`detect`,
`prompt`, `quit`, `recheck`, `reopen`, `warn`, or `block`), mode (`planned` or
`executed`), result (`succeeded`, `failed`, `blocked`, `declined`, `skipped`, or
`planned`), running-state summary, whether the manager stopped the app, whether
reopen was attempted, and a stable diagnostic code/message when applicable.
Dry-run records must be truthful: running-state detection is `executed` if the
preview actually checked process state, while future prompt/quit/reopen actions
remain `planned`. It must not include raw command lines, shell output, process
environments, arbitrary PIDs, temp paths, account IDs, or payload content.

If lifecycle blocks before write, no backup or live mutation is allowed. If the
manager stopped an app and the later write fails, the manager should still
attempt the declared reopen and record both failures. If write succeeds but
reopen fails, the run must expose a non-success lifecycle result while preserving
the verified write evidence.

For native apply, backup is not optional in MVP. The accepted policy is exactly
`pre-apply-export`; the manager must run and persist that backup export before
import. The accepted verification policy is exactly `post-import-export-hash`;
the manager must run a post-import export and compare its payload hash and
normalizer to the desired native export artifact. Backup export failure blocks
import. Import or verification failure after backup records a failed run with
backup refs, no successful ledger entry, and no claim that live state matches
desired.

### Ledger commit rules

Ledger and run records are local-only state:

```text
ledger/ledger.jsonl
ledger/runs/<run-id>.json
```

`ledger/ledger.jsonl` stores append-only ledger entries with schema
`dotfiles-manager.v2.ledger-entry` and `schemaVersion: 1`.
`ledger/runs/<run-id>.json` stores the expanded run record with schema
`dotfiles-manager.v2.run-record` and `schemaVersion: 1`.

A ledger entry must record:

- run ID;
- timestamp;
- command;
- profile stack;
- target/setting refs;
- desired artifact refs;
- driver and normalizer versions;
- normalized hashes;
- backup refs;
- verification result;
- partial failures and skipped items.

The ledger must not claim success for failed or unverified writes.

### Backup rules

Before live writes, the manager must create a backup when the driver supports it.
If backup is unsupported, the change preview must say so and the policy must
allow proceeding.

Backup material is local state, not desired repository data:

```text
backups/<run-id>/backup.yaml
backups/<run-id>/payloads/...
```

`backup.yaml` stores backup metadata with schema
`dotfiles-manager.v2.backup-metadata` and `schemaVersion: 1`. Driver-specific
restore material lives under `payloads/`.

### Preview and capture records

Previews and raw captures are local-only run records:

```text
runs/<run-id>/preview.json
runs/<run-id>/captures/...
```

`preview.json` uses schema `dotfiles-manager.v2.preview` and
`schemaVersion: 1`. Raw captures are retained only when policy permits;
otherwise they remain in temp storage or are deleted after the command.

### Restore rules

`restore <run-id>` must:

1. locate compatible backup material;
2. preview restore writes;
3. ask for confirmation;
4. create backup-before-restore where supported;
5. restore through the driver;
6. verify restore result;
7. write a restore ledger entry.

Restore must fail clearly when backup material is missing, incompatible, or
unsupported.

## Derived schema boundaries, not final schemas

This spec owns ledger and backup metadata boundaries.

Persisted objects:

| Object | Owned here? | Canonical path | Schema file | Notes |
| --- | --- | --- | --- | --- |
| Ledger entry | yes | `ledger/ledger.jsonl` local state | `schemas/v2/ledger-entry.schema.json` | Append-only verified outcome record. |
| Run record | yes | `ledger/runs/<run-id>.json` local state | `schemas/v2/run-record.schema.json` | Expanded per-run transaction record. |
| Backup metadata | yes | `backups/<run-id>/backup.yaml` local state | `schemas/v2/backup-metadata.schema.json` | Restore material reference and compatibility. |
| Backup payload | partial | `backups/<run-id>/payloads/...` local state | Metadata in `backup.yaml` | Driver-specific payload. |
| Preview JSON | partial | `runs/<run-id>/preview.json` local state | `schemas/v2/preview.schema.json` | CLI envelope owns result shape. |
| Normalized hash | partial | ledger/run records; cache copies are disposable | `schemas/v2/ledger-entry.schema.json` and `schemas/v2/run-record.schema.json` | Driver spec owns normalizer versioning. |

Ledger, backup, preview, config, profile, recipe, and artifact schemas must have
independent schema versions.

`cache/normalized/...` is disposable derived data only. Authoritative
last-applied hashes and verified outcomes live in ledger entries and run
records, not cache.

## Examples

Examples use the public target/setting ref grammar owned by
`00-vocabulary.md`. Ledger, backup, and preview field names remain sketches
until their schemas are promoted.

### Apply transaction sketch

```text
run-123
  preview: update git:user.email
  backup: state://backups/run-123/git/user.email
  apply: write ~/.gitconfig [user] email
  verify: current == desired
  ledger: commit verified last-applied hash
```

### Partial success

```text
applied:
  git:user.email
blocked:
  example-tool:user-info    app must be closed before write  # illustrative-only
exit: 6
```

## Errors, blockers, and partial-result behavior

Mutation blockers include:

- validation failure;
- conflict without user choice;
- lifecycle block;
- trust block;
- secret block;
- backup required but unavailable;
- verification failure;
- restore backup missing or incompatible.

Partial success is allowed only when failed/skipped items are independent and the
command can report exact per-item outcomes.

## Acceptance expectations

- Tests prove `apply --dry-run` mutates nothing.
- Tests prove live `apply` creates backups before writes where supported.
- Tests prove ledgers update only after verified success.
- Tests cover partial success and exit code `6`.
- Tests cover restore preview and backup-before-restore.
- Tests cover unsupported restore paths.

## Out of scope

- final retention policy;
- cloud backup sync;
- transactional atomicity across unrelated apps;
- restoring secrets from secret managers;
- restoring account/session/cache state.

## Spec follow-ups / open decisions

- Decide retention and pruning rules.
- Decide final ledger, run-record, backup, and preview field schemas.
- Decide how much backup metadata is safe to show in normal output.
