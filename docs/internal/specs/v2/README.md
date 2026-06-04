---
owner: Core Engineering
document-type: v2-package-index
status: Draft
last-updated: 2026-06-04
canonical-source: docs/internal/specs/v2/README.md
authority: Draft package index; promotion rules only, not current implementation behavior
---

# v2 formal specification package

This package is the staging area for formal `dotfiles-manager` v2
specifications.

The source concept is:

- `../../scope/product-concept-v2.md`

The concept document defines the accepted v2 product direction. The files in
this package extract that direction into implementation-facing specifications.
Once a v2 spec is written and reviewed, agents and humans should implement from
that spec instead of directly from the concept prose.

## Authority and relationship to v1

This package is draft implementation-prep material until explicitly promoted.
It does not override the existing v1 specs, contracts, or implementation
behavior by itself.

Current v1 behavior remains governed by:

- `../cli-and-config-spec.md`
- `../decisions.md`
- `../decision-matrix.md`
- `../../contracts/*`

The v2 implementation must prove v1 parity before replacing the existing
dotfile sync path. Existing `syncs:` configs must remain readable through a
legacy adapter until explicit migration behavior is implemented and accepted.

## Metadata contract

Formal v2 specs in this package must use this front matter shape:

```yaml
owner: Core Engineering
document-type: v2-draft-spec
status: Draft
last-updated: YYYY-MM-DD
canonical-source: docs/internal/specs/v2/<file>.md
source-concept-sections:
  - <concept section name>
authority: Draft; non-authoritative until promoted by docs/internal/specs/v2/README.md
```

Planning artifacts are not formal specs. They must use a distinct
`document-type`, such as `v2-planning-roadmap`, and must not appear in the
formal 00-11 spec table.

## Status taxonomy

| Status | Meaning | Implementation authority |
| --- | --- | --- |
| `Draft` | Extracted or proposed v2 material under review. | no |
| `Active` | Reviewed and explicitly promoted for v2 implementation work. | v2 only |
| `Superseded` | Replaced by another promoted v2 spec or current v1 authority. | no |

There are currently no `Active` v2 specs. All formal specs in this package are
`Draft`.

## Promotion rule

A v2 spec becomes implementation-authoritative only after it is written,
reviewed, and explicitly linked from this package index as an active spec.
Until then, existing v1 specs and contracts remain authoritative for current
behavior.

Promotion requires all of the following:

1. the spec's front matter status changes from `Draft` to `Active`;
2. the spec's authority field states the exact v2 implementation scope it owns;
3. this package index marks the spec `Active` in the formal spec table;
4. the promotion change cites the review or issue that approved promotion;
5. the spec has enough acceptance tests or fixture expectations for agents to
   implement without using the concept prose as the contract;
6. the change explicitly states whether current v1 behavior is unaffected,
   adapted through compatibility, or superseded by a migration gate.

Promotion does not automatically make v2 the current user-facing implementation.
Current v1 behavior remains authoritative until a separate migration/promotion
decision updates the root internal canonicality model.

## Agent implementation rule

AI agents may use Draft v2 specs for planning, issue writing, and prototype
work. They must not claim that a Draft v2 spec defines current production
behavior. Runtime implementation issues should cite the exact v2 specs they use
and should also state how v1 behavior is preserved while v2 is built beside it.

## Planned spec set

| Spec | Status | Purpose |
| --- | --- | --- |
| `00-vocabulary.md` | Draft | Core nouns and relationships. |
| `01-repository-layout.md` | Draft | Source/config/state file layout. |
| `02-cli-contract.md` | Draft | Commands, flags, prompts, JSON, and exit codes. |
| `03-profile-and-scope-resolution.md` | Draft | Profile stacks, scopes, IDs, and resolution. |
| `04-status-conflict-state-machine.md` | Draft | Status states, derivation, conflict handling. |
| `05-desired-artifacts-and-uris.md` | Draft | Desired artifacts, URI schemes, lifecycle rules. |
| `06-recipe-schema.md` | Draft | Recipe format, named locations, settings, support levels. |
| `07-driver-interface.md` | Draft | Driver operations, selectors, normalization, verification. |
| `08-mutation-ledger-backup-restore.md` | Draft | Transactions, ledgers, backups, restore semantics. |
| `09-security-redaction-trust.md` | Draft | Secrets, trust, redaction, lifecycle, threat model. |
| `10-v1-migration.md` | Draft | v1 compatibility, migration preview, rollback. |
| `11-mvp-acceptance-tests.md` | Draft | Fixture matrix and release gates. |


## Planning artifacts

These files are planning aids, not formal specs in the 00-11 set:

| File | Status | Purpose |
| --- | --- | --- |
| `mvp-implementation-roadmap.md` | Draft | MVP implementation sequence and tracker reset plan. |

## Spec extraction rules

Each spec should:

1. preserve the simple end-user model from the concept document;
2. define normative behavior separately from examples;
3. include concrete examples for normal and edge cases;
4. define error/blocker behavior where relevant;
5. include acceptance-test expectations;
6. identify explicit out-of-scope behavior;
7. avoid silently changing current v1 behavior before migration is explicit.

The goal is to turn the concept into small, reviewable implementation contracts
that AI agents can execute without improvising from the long concept document.
