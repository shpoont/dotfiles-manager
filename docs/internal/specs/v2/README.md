---
owner: Product + Core Engineering
document-type: v2-package-index
status: Active reset index; not a runtime behavior contract
last-updated: 2026-06-23
canonical-source: docs/internal/specs/v2/README.md
source-issue: 210
authority: Source-of-truth map for active, draft, and superseded v2 planning specs; implementation authority requires explicitly Active behavior specs.
---

# v2 specification package index

## Purpose

This package is the staging area for reset-v2 product and implementation specs.
It tells agents which documents are active planning inputs, which are reusable
drafts, and which older prototype specs are superseded.

No v2 spec is runtime implementation-authoritative unless it is explicitly marked
`Active` for a specific behavior scope. Vocabulary and layout documents may be
active planning sources without implementing behavior.

## Current reset model

v2 is a local settings manager focused on syncing selected **live settings** with
**stored settings** in a **settings folder**.

Required constraints:

- Git is optional; a settings folder may be versioned with Git but does not have
  to be a Git repository.
- `status`, `diff`, and `sync` are the primary UX.
- `save`/`apply` are secondary compatibility aliases for explicit directional
  sync, not separate primary workflows.
- Backup/restore is out of v2 product scope; #212 owns removal or quarantine of
  old backup/restore surfaces.
- v1 migration is out of the active v2 roadmap; #213/#226 own legacy public
  surface policy.
- Remote catalog writes wait for #214/#227 trust and write-authority rules.

## Status taxonomy

| Status | Meaning | Implementation authority |
| --- | --- | --- |
| Active vocabulary/planning source | Accepted noun/model direction for issues/specs/docs. | no runtime behavior by itself |
| Active behavior spec | Reviewed behavior contract for a bounded implementation issue. | yes, for that scope only |
| Draft | Reusable prototype/spec material still needing reset review. | no |
| Superseded | Older prototype material replaced by a reset source or follow-up issue. | no |
| Historical | Preserved background/reference only. | no |

## Active and superseded spec map

| File | Reset status | Role / next owner |
| --- | --- | --- |
| `../../scope/product-concept-v2.md` | Active vocabulary/planning source | Reset product concept for v2; not command behavior authority. |
| `00-vocabulary.md` | Active vocabulary/planning source | Vocabulary source for settings folder, live/stored settings, scopes, internal URI policy, CLI vocabulary floor, and sensitive stored values; not command behavior authority. |
| `01-settings-storage-layout.md` | Draft reset layout source | Replaces repository-layout direction; exact schemas/path compatibility need later promoted specs. |
| `01-repository-layout.md` | Superseded | Historical prototype repository layout; do not use as active reset-v2 authority. |
| `02-cli-contract.md` | Superseded draft pending #211 split | Contains useful examples but still encodes old command model; #221-#225 own the replacement status/diff/sync contracts. |
| `03-profile-and-scope-resolution.md` | Draft reusable input | Keep profile/scope mechanics, but #210 vocabulary labels apply. |
| `04-status-conflict-state-machine.md` | Draft reusable input | State-derivation reference only; it still uses older public wording and is not normative for public reset-v2 output. |
| `05-desired-artifacts-and-uris.md` | Draft reusable input | Keep internal artifact/URI mechanics; normal output must follow #210 internal URI policy. |
| `06-recipe-schema.md` | Draft reusable input | Keep recipe/named-location concepts; catalog/trust updates belong to #214/#227. |
| `07-driver-interface.md` | Draft reusable input | Keep deterministic driver model; native import/export remains reviewed recipe/driver capability. |
| `08-mutation-ledger-backup-restore.md` | Superseded for product scope | Backup/restore is not active v2 product scope; #212 decides delete/quarantine/internal safety evidence. |
| `09-security-redaction-trust.md` | Draft reusable input | Must incorporate sensitive stored-settings wording from #210 before promotion. |
| `10-v1-migration.md` | Superseded for active v2 roadmap | v1 migration is not active v2 scope; #213/#226 own legacy public-surface policy. |
| `11-mvp-acceptance-tests.md` | Draft requiring reset review | Must be reworked so backup/restore, migration, and legacy v1 tests are not v2 acceptance blockers. |
| `12-status-diff-read-only-contract.md` | Active behavior spec | Source of truth for read-only `status` and `diff` output, examples, and golden-fixture expectations from #221. |
| `13-smart-sync-planning-contract.md` | Active behavior spec | Source of truth for smart-sync planning and conflict UX from #222; no write execution authority. |
| `14-smart-sync-execution-contract.md` | Active behavior spec | Source of truth for mutating smart-sync execution, confirmation, refusal, and execution reporting from #223. |
| `15-partial-many-app-sync-fixtures-contract.md` | Active behavior spec | Source of truth for partial and many-app sync UX fixture coverage from #224; no new selector or driver implementation authority. |
| `16-save-apply-alias-policy.md` | Active behavior spec | Source of truth for #225 `save`/`apply` public alias policy; `sync` remains primary. |
| `mvp-implementation-roadmap.md` | Superseded planning artifact | Replaced by #209 execution record, #219 audit, and updated issue set. |

## Promotion rule

A behavior spec becomes implementation-authoritative only after all of these are
true:

1. the spec's front matter says `status: Active behavior spec`;
2. the authority field names the exact behavior scope;
3. this index marks the spec active for that scope;
4. the promotion change cites the approving issue/PR/review;
5. acceptance tests or fixtures are sufficient for implementation agents;
6. reset constraints above are not contradicted.

## Agent implementation rule

Agents may use active planning sources and draft reusable inputs for planning and
PR scope. They must not claim that a draft or superseded spec defines current
runtime behavior.

Before runtime implementation starts, the issue must cite the active behavior
spec or explicitly include the accepted behavior contract.

Older prototype packages such as `internal/v2/guidedsync` may be useful
implementation references, but they are not public reset-v2 authority until they
are reconciled with the active behavior specs in this index.

## Follow-up ownership

- #210 owns vocabulary/source-of-truth cleanup.
- #211 and #221-#225 own status/diff/sync and `save`/`apply` policy.
- #221 specifically owns only the read-only `status` and `diff` behavior
  contract in `12-status-diff-read-only-contract.md`.
- #222 owns only smart-sync planning and conflict UX in
  `13-smart-sync-planning-contract.md`.
- #223 owns only mutating smart-sync execution and confirmation UX in
  `14-smart-sync-execution-contract.md`; #224/#225 remain responsible for
  broader UX and directional alias policy.
- #224 owns only partial and many-app sync UX fixture coverage in
  `15-partial-many-app-sync-fixtures-contract.md`; it does not finalize selector
  spelling or add new app/backend behavior.
- #225 owns only the `save`/`apply` alias policy in
  `16-save-apply-alias-policy.md`; it does not remove the aliases or change
  smart-sync planning/execution semantics.
- #212 owns backup/restore removal or quarantine.
- #213 and #226 own legacy v1 public-surface policy.
- #214 and #227-#229 own catalogs/taps.
- #215 and #230-#231 own new-computer bootstrap.
- #216 owns production end-user docs after accepted behavior exists.
