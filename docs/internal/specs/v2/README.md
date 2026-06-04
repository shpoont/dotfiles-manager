---
owner: Core Engineering
status: Draft
last-updated: 2026-06-04
canonical-source: docs/internal/specs/v2/README.md
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

## Promotion rule

A v2 spec becomes implementation-authoritative only after it is written,
reviewed, and explicitly linked from this package index as an active spec.
Until then, existing v1 specs and contracts remain authoritative for current
behavior.

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
