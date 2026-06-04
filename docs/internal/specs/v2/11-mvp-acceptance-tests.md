---
owner: Core Engineering
document-type: v2-draft-spec
status: Draft
last-updated: 2026-06-04
canonical-source: docs/internal/specs/v2/11-mvp-acceptance-tests.md
source-concept-sections:
  - MVP acceptance test matrix
  - Suggested MVP
  - Roadmap decomposition
  - Likely failure modes
authority: Draft; non-authoritative until promoted by docs/internal/specs/v2/README.md
---

# v2 MVP acceptance tests

## Purpose

This spec defines the cross-spec release gate for v2 MVP implementation. It is
not a replacement for per-spec tests. Each spec still owns local acceptance
expectations.

The MVP implementation should not start broad feature work without fixtures that
prove the core model is deterministic, safe, and compatible with v1 dotfile
behavior.

## Status and authority

This is a draft extraction from `../../scope/product-concept-v2.md`. It is not
implementation-authoritative until promoted through `README.md`.

Current v1 behavior remains governed by existing v1 specs and contracts.

## Source map and extraction notes

Extracted from the concept sections covering:

- MVP acceptance matrix;
- suggested MVP;
- roadmap phases;
- failure modes and mitigations;
- v1 parity requirements.

Deliberate non-decisions:

- exact test framework is deferred;
- exact CI job names are deferred;
- final release checklist is deferred.

## Terms owned by this spec

- acceptance fixture;
- release gate;
- parity gate;
- snapshot test;
- safety regression;
- dogfood gate.

## Normative MVP rules

### Cross-spec gate

A v2 MVP release must not be called production-ready unless these areas pass:

| Area | Required fixtures/tests |
| --- | --- |
| CLI | Text and `--json` snapshots for every normal command and exit code. |
| Profile/scope | All four scopes, multi-user machine, multiple profile layers. |
| Status | Every canonical state and target-level aggregation. |
| File driver | Create/update/delete preview, backup, restore, symlink rejection. |
| File-tree driver | Globs, case conflicts, unsafe traversal, metadata policy. |
| Structured drivers | INI/JSON/YAML/TOML selectors, invalid selectors, normalization. |
| Plist/defaults | Selected keys only, read-only defaults, unsupported write attempts. |
| Redaction | Display redaction, blocked save, unavailable redaction, safe values. |
| Lifecycle | Running app allowed/warn/blocked, quit declined, reopen failure. |
| Native export | Diffable export, opaque export, passphrase prompt, size/category limits. |
| Ledger | Last-applied update only after verified success, partial failure recording. |
| Restore | Restore preview, backup-before-restore, unsupported restore path. |
| Migration | v1 `syncs:` parity and generated `custom.files` target. |
| Trust | Untrusted local recipe, recipe broadening write scope, command-IO gate. |

### First vertical slice gate

Before broad app support, one vertical slice must pass:

```text
init
add custom.files
list
status
diff
save --dry-run
apply --dry-run
save
apply
backup list
restore
```

Scope:

- file driver;
- file-tree driver;
- profile stack;
- desired artifacts;
- local ledger;
- backup before apply;
- v1 migration fixture.

### Production-readiness gate

Production readiness requires:

- v1 parity for current dotfile use cases;
- no known secret leakage path in output, JSON, artifacts, ledgers, or backups;
- dry-run previews are trustworthy;
- live apply creates backups where supported;
- restore works for supported drivers;
- unsupported/unsafe cases fail clearly;
- CI passes on clean checkout;
- dogfood run succeeds on a non-critical profile/machine.

### Agent implementation gate

AI agents should receive issues that reference:

- the relevant v2 spec files;
- exact acceptance tests;
- out-of-scope boundaries;
- safety constraints;
- expected files to modify.

Agents should not implement directly from the full concept document once a spec
exists for the area.

## Derived schema boundaries, not final schemas

This spec owns test matrix and release-gate metadata only.

Persisted/emitted objects:

| Object | Owned here? | Notes |
| --- | --- | --- |
| Acceptance matrix | yes | Documentation/test planning. |
| CI result format | no | Existing engineering docs own current CI. |
| Fixture schemas | partial | Test implementation detail. |
| Parity report | partial | Migration spec also references it. |

## Examples

### Issue acceptance block

```text
Acceptance:
- fixture covers shared/user/machine/machine-user scope
- --json snapshot includes command, profileStack, summary, items, ledgerRef
- unsafe path fixture fails with exit code 2 or 5 as specified
- no live writes occur under --dry-run
```

### Dogfood gate

```text
Dogfood profile: v2-sandbox
Targets: custom.files, git read/write, nvim file-tree
Required: dry-run reviewed, apply backed up, restore tested
```

## Errors, blockers, and partial-result behavior

A release gate fails if:

- a safety fixture leaks a secret;
- dry-run mutates live or desired state;
- apply writes without preview;
- ledger records unverified success;
- v1 parity fixture fails;
- unsupported state is silently treated as managed;
- conflict detection is absent or bolted on after writes.

## Acceptance expectations

This file is satisfied when:

- every row in the matrix maps to automated or explicitly manual tests;
- every implementation issue references relevant tests;
- CI can run the core MVP fixture suite;
- production readiness cannot be claimed without the gate passing.

## Out of scope

- final CI implementation;
- exact test framework;
- remote catalog tests;
- AI discovery tests;
- broad app reverse-engineering tests.

## Spec follow-ups / open decisions

- Decide exact test framework and fixture directory layout.
- Decide which gates are automated vs manual in MVP.
- Decide dogfood profile and machine strategy.
- Decide release labels/statuses for GitHub issues.
