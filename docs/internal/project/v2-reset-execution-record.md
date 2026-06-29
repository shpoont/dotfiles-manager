---
owner: Project Owner + Work Manager
status: Active project record
last-updated: 2026-06-29
canonical-source: docs/internal/project/v2-reset-execution-record.md
project-issue: 209
---

# v2 reset execution record

## Source of truth

- Project charter / parent issue: #209
- Standards-adoption scaffold issue: #217
- Latest standards reconciliation issue: #238 (complete)
- Transformation plan:
  [`v2-project-execution-standards-transformation-plan.md`](v2-project-execution-standards-transformation-plan.md)
- Project-specific execution tailoring:
  [`../process/v2-execution-standards-tailoring.md`](../process/v2-execution-standards-tailoring.md)

This file is the repo-editable execution record for the v2 reset. GitHub issues
remain authoritative for individual work-item contracts. This file records the
project-level state, sequencing, risks, and evidence index.

## Project outcome

Deliver a v2 local settings manager whose primary user-facing value is syncing
settings between live app settings and a settings storage folder.

The product should let a user:

1. choose supported apps/tools to manage;
2. inspect status and diffs between live settings and stored settings;
3. sync all or selected settings in the correct direction;
4. handle conflicts and missing apps/settings safely;
5. use a settings storage folder with or without Git;
6. use bundled recipes first and later optional recipe catalogs/taps;
7. set up a new computer by installing apps first and then applying settings.

## Current accepted product constraints

- The core noun is settings storage folder, not mandatory Git repository.
- Git may be recommended for versioning and sharing but is not required.
- The primary UX is status, diff, and sync.
- `save` and `apply` are retained as public compatibility aliases for
  directional sync, not as the primary product model:
  - `save` = sync live settings to stored settings;
  - `apply` = sync stored settings to live settings.
- Backup/restore is out of v2 public product scope. #212 / PR #243 removed
  the public command workflow, user-facing docs/outputs, and v2 acceptance
  dependency; lower-level internal recovery/snapshot/ledger mechanics remain
  implementation details for now.
- v1 is historical reference only; no v1 migration roadmap is in scope. #213
  and #226 are complete; PR #247 hides retained legacy v1 public commands from
  the normal v2 help surface and separates them from v2 product acceptance.
- Current v2 implementation and docs are partially reset-aligned after #210,
  #211, #212, #221-#225, and #213/#226. Parent #211 is accepted and closed,
  but the combined v2 product is not production-accepted until remaining gates
  are completed or explicitly deferred.

## Explicit non-goals

- Managing app installation as part of dotfiles-manager.
- Requiring Git for the storage folder.
- A first-class backup/restore workflow for v2.
- v1 migration tooling as part of the current v2 roadmap.
- Remote catalog writes before recipe origin, trust, update, and write-authority
  rules are explicit.
- Treating old closed PR branches as active work branches.


## Current lifecycle gate status

As of 2026-06-29, the updated external Project Execution Standard adds explicit
lifecycle gate / gate passport, public-surface freeze, implementation-start, and
runnable/replayable usage-evidence requirements. Local process hardening was
completed by #256 / PR #257 so future sessions use the updated standard
operationally.

Current active gate:

- #228 has draft implementation PR #255, but #228 is in recovery gate status,
  not normal ready-to-finish implementation.
- #228 recovery is required because implementation began without a sufficiently
  explicit Project Owner implementation-start gate for the final CLI UX package
  and without a recorded decision that static storyboard/transcript evidence was
  sufficient instead of a runnable/replayable CLI mock.
- PR #255 also has a separate CI coverage failure: `coverage-aggregation` failed
  at `88.9% < 90%`; `final-required-check` failed because coverage failed.

No-go actions until #228 recovery is recorded:

- Do not fix #255 coverage or continue #228 implementation as if the UX gate were
  already satisfied.
- Do not mark PR #255 ready, merge it, request #228 acceptance, or close #228.
- Do not start #229 remote catalog implementation using #228 as accepted.

Allowed #228 recovery actions after #256:

1. create a runnable/replayable CLI mock and compare PR #255 against it;
2. record Project Owner waiver/not-applicable decision for the mock and accept
   the existing storyboard as sufficient design evidence for #228; or
3. recontract #228 before implementation continues.

## Standards maturity snapshot

As of 2026-06-29:

- The Project Execution Standards scaffold is adopted (#217 / PR #218).
- Phase 2 audit/discovery is closed (#219 / PR #220). The audit was accepted for
  sequencing through the closing record and follow-up issue splits/edits; this
  does not mean every original #219 checkbox was rewritten in-place.
- Phase 3 resequencing has been substantially executed: broad reset areas were
  split into focused children (#221-#231) where needed.
- Product vocabulary is accepted (#210).
- The focused sync split is complete (#221-#225), #212 has removed the public
  backup/restore contradiction, #213/#226 has separated hidden legacy v1
  commands from the v2 happy path, and parent #211 is accepted/closed for the
  status/diff/sync primary UX gate.
- Reconciliation pass #238 closed the source-of-truth drift introduced by several
  rapid issue closures. The project is scaffold-adopted and is following the
  standards in current work, but it is not yet fully v2-accepted.
- The external standard now has explicit lifecycle gate / gate passport,
  public-surface freeze, implementation-start, and runnable/replayable
  usage-evidence requirements. #256 adapts the local repository process to that
  updated standard and records #228 recovery status.

## Active work-item table

| Issue | Type | Tier | Current role | Dependencies / notes |
| --- | --- | --- | --- | --- |
| #209 | Project charter / parent | Project-level Tier 1 | Open / In Progress | Keep active until the reset outcome is accepted. |
| #217 | Enabler scaffold | Tier 1 | Complete | PR #218 merged; standards scaffold installed. |
| #219 | Discovery / audit | Tier 1 | Complete | Phase 2 audit closed; follow-up splits/edits produced #221-#231. |
| #210 | Delivery-design | Tier 1 | Complete | Product model and vocabulary accepted. |
| #211 | Parent delivery area | Tier 2 overall | Complete | Accepted and closed 2026-06-26 after #221-#225, #212, and #213/#226 completed. |
| #212 | Product-scope cleanup gate | Tier 1 | Complete | PR #243 removed public backup/restore workflow; issue closed 2026-06-25. |
| #213 | Product-scope cleanup gate | Tier 1 | Complete | PR #247 removed v1 migration from the active v2 roadmap/user-facing happy path and closed 2026-06-26. |
| #214 | Parent delivery area | Discovery then Tier 2 for remote writes | Open parent | #227 completed the trust/origin model; use #228 before #229 where built-in/local discovery behavior is needed. Remote writes still require #229 write-gate implementation. |
| #215 | Parent delivery area | Tier 1/Tier 2 | Open parent | Use #230 before #231; Homebrew Bundle remains an example, not a dependency. |
| #216 | Documentation delivery | Tier 1 | Open | Production docs depend on accepted behavior/examples from remaining gates. |
| #226 | Delivery-design cleanup | Tier 1 | Complete | PR #247 hides retained legacy v1 commands from root help, labels direct help as legacy compatibility, and closed 2026-06-26. |
| #227 | Discovery/design | Discovery/Tier 1 | Complete | PR #250 merged; issue accepted and closed 2026-06-26. |
| #228 | Delivery | Tier 1 | Recovery gate / draft PR #255 | Built-in/local catalog discovery implementation exists in draft PR #255, but process recovery is required before code/coverage continuation: choose runnable/replayable mock, explicit waiver, or recontract. |
| #229 | Delivery | Tier 2 | Open child of #214 | Implement remote catalog management with write gates against the accepted #227 model. |
| #230 | UX/design | Tier 1 | Open child of #215 | Specify new-computer UX and output. |
| #231 | Delivery | Tier 2 | Open child of #215 | Implement apply-from-storage flow after sync model/UX is accepted. |
| #238 | Cleanup / enabler | Tier 1 | Complete | PR #239 reconciled #209 and this execution record with live state; no runtime changes. |
| #256 | Cleanup / process hardening | Tier 1 | Complete | PR #257 adapted local AGENTS, tailoring, templates, and execution record to the updated 2026-06-29 Project Execution Standard; no runtime changes. |

## Reconciliation inventory: 2026-06-26

This inventory was generated from live GitHub issue/project state before editing
#209 or this file. It is intentionally conservative: closed child issues prove
child-scope completion, not automatic parent closure.

| Issue | Live state | Role | Completion evidence | #209 representation | Parent/open rationale or next action |
| --- | --- | --- | --- | --- | --- |
| #209 | Open / In Progress | Project charter | Live GitHub issue/project state | Keep open and active | Close only when reset outcome is accepted. |
| #217 | Closed / Done | Phase 1 scaffold | Issue closed 2026-06-23; PR #218 merged | Checked | No further action. |
| #219 | Closed / Done | Phase 2 audit | Issue closed 2026-06-23; PR #220 merged; follow-up splits recorded | Checked with conservative note | Audit/discovery closed for sequencing; product gates remain in follow-up issues. |
| #210 | Closed / Done | Vocabulary/product model | Issue closed 2026-06-23 | Checked | No further action unless later vocabulary drift appears. |
| #211 | Closed / Done | Sync parent | Children #221-#225, #212, and #213/#226 closed; Project Owner accepted #211 closure | Checked | Parent accepted and closed 2026-06-26; catalog work moved through #227 and now continues with #228/#229. |
| #221 | Closed / Done | Sync child | Issue closed 2026-06-23 | Checked under #211 | Read-only status/diff contract complete. |
| #222 | Closed / Done | Sync child | PR #234 merged; issue closed 2026-06-23 | Checked under #211 | Smart-sync planning/conflict UX complete. |
| #223 | Closed / Done | Sync child | PR #235 merged; issue closed 2026-06-23 | Checked under #211 | Mutating sync execution/confirmation complete for that slice. |
| #224 | Closed / Done | Sync child | PR #236 merged; issue closed 2026-06-23 | Checked under #211 | Partial/many-app UX fixtures complete. |
| #225 | Closed / Done | Sync child | PR #237 merged; issue closed 2026-06-24 | Checked under #211 | Save/apply alias policy complete. |
| #212 | Closed / Done | Product-scope gate | PR #243 merged 2026-06-25; issue closed | Checked | Public backup/restore workflow removed; lower-level internal recovery mechanics remain implementation details. |
| #213 | Closed / Done | Product-scope gate | PR #247 merged; issue closed 2026-06-26 | Checked | No further action unless future v1 migration/deprecation work is explicitly reintroduced. |
| #226 | Closed / Done | Child of #213 | PR #247 merged; issue closed 2026-06-26 | Checked under #213 | Retained legacy v1 commands are hidden from normal help and separated from v2 acceptance. |
| #214 | Open / Todo | Catalog parent | #227 closed; #228/#229 open | Keep parent open with children | Built-in/local discovery and remote write-gate implementation remain. |
| #227 | Closed / Done | Catalog child | PR #250 merged; issue closed after Project Owner acceptance | Checked under #214 | Trust/origin/write-authority model complete for downstream implementation. |
| #228 | Open / draft PR | Catalog child | Draft PR #255 exists; CI coverage currently fails at 88.9% < 90%; process recovery gate identified | Recovery under #214 | Before code/coverage continuation, recover implementation-start/public-surface gate by mock, waiver, or recontract. |
| #229 | Open / Todo | Catalog child | Live issue open | Open under #214 | Implement remote catalog management with write gates against the accepted #227 model. |
| #215 | Open / Todo | Bootstrap parent | Children #230-#231 open | Keep parent open with children | Bootstrap must reuse sync model; Homebrew Bundle is example only. |
| #230 | Open / Todo | Bootstrap child | Live issue open | Open under #215 | Specify new-computer UX/output before implementation. |
| #231 | Open / Todo | Bootstrap child | Live issue open | Open under #215 | Implement apply-from-storage after accepted UX/model. |
| #216 | Open / Todo | Production docs | Live issue open | Open | Rewrite after accepted behavior/examples; current docs are not final production docs. |
| #238 | Closed / Done | Standards reconciliation | PR #239 merged; final evidence comment recorded | Checked | No further action; keep #209 and this record current after closures. |
| #256 | Closed / Done | Local process hardening | PR #257 completed local process gate adaptation after external standard update | Checked | No further action; resume at #228 recovery gate before #228 code/coverage work. |

## Open parent rationale

- #209 remains open because it is the project charter and closes only when the
  combined reset outcome is accepted.
- #214 remains open as the catalog parent; #227 settled origin/trust/write
  authority, while #228/#229 implementation remains open.
- #215 remains open as the bootstrap parent; #230 must specify UX/output before
  #231 implementation.
- #216 remains open because production end-user documentation depends on
  accepted behavior and examples from the remaining gates.
- #238 is closed; future work should keep #209 and this record current after each
  work item closes.

## Current branch rule

Before starting any work item:

1. fetch/prune remotes;
2. verify no open PR branch already owns the issue;
3. switch to current `main`;
4. create a fresh branch named `codex/<issue-or-topic>-<short-slug>`;
5. leave untracked unrelated local files untouched;
6. treat old closed-PR branches as reference only unless explicitly selected for
   salvage.

## Current risks

| Risk | Mitigation | Owner | Status |
| --- | --- | --- | --- |
| Source-of-truth records drift behind live tracker state | Keep #209 and this record updated after each closure; #238 reconciled the current drift | Work Manager | Monitoring |
| Backup/restore remains visible as a public v2 scope contradiction | #212 / PR #243 removed the public workflow and accepted outputs | Work Manager | Closed |
| Legacy v1 commands/migration can leak into v2 happy path | #213/#226 / PR #247 hide retained legacy commands from normal help and remove v1 migration from v2 user docs | Work Manager | Closed |
| Remote recipe catalogs can execute untrusted writes | #227 defines the trust/origin/write-authority model; #229 must enforce it before any remote catalog can write live settings | Work Manager | Design complete / implementation open |
| Production docs could describe unverified behavior | Keep #216 open until behavior/examples are accepted | Docs owner | Open |
| Branches from closed PRs may be reused accidentally | Enforce branch rule and start from `main` | Executor | Active |
| Agents may cross lifecycle phases from broad prompts or artifact completion | Use lifecycle gate/passport readback, implementation-start gate, and explicit no-go actions from AGENTS/tailoring/templates | Work Manager / Executor | Mitigated / monitoring |
| Meaningful CLI UX can be implemented from static transcripts without owner-approved runnable/replayable evidence | Default to runnable/replayable CLI usage evidence or record a Project Owner waiver/not-applicable decision | Work Manager / Project Owner | Mitigated / monitoring |
| #228 implementation may continue as if PR #255 were only a CI issue | Record #228 recovery status and require mock/waiver/recontract before coverage cleanup or merge | Work Manager | Active |

## Evidence index

| Claim / gate | Evidence link | Date | Limits |
| --- | --- | --- | --- |
| Active reset issues were initially #209-#216 | GitHub issue list read during transformation planning | 2026-06-23 | Historical baseline; superseded by the 2026-06-26 inventory above. |
| Phase 1 scaffold passed validation | PR #218 checks, local scaffold checks, and Pro review conversation | 2026-06-23 | Validates process scaffold only, not current v2 product behavior. |
| Project Owner accepted the Phase 1 source-of-truth decision | User confirmation: "We are good to go" after explicit acceptance prompt | 2026-06-23 | Acceptance is for #209 + this file as source of truth and PR #218 merge approval. |
| Phase 1 scaffold was merged | PR #218 squash merge `1bbb484eb958d5477937da675da76482a43a8845` | 2026-06-23 | Merge validates process scaffold only. |
| Phase 2 audit artifact exists | [`v2-reset-audit-issue-219.md`](v2-reset-audit-issue-219.md) | 2026-06-23 | Audit evidence; remaining decisions live in follow-up issues. |
| Phase 2 audit received Pro validation | ChatGPT Pro review conversation `https://chatgpt.com/c/6a3a5a21-6bc4-83eb-8449-e08b72d0d267` | 2026-06-23 | Pro validation does not replace issue closure/readback evidence. |
| Phase 2 audit/discovery closed for sequencing | GitHub issue #219 closed / project Done; PR #220 merged | 2026-06-23 | Does not imply all product gates are complete. |
| Vocabulary/product model accepted | GitHub issue #210 closed / project Done | 2026-06-23 | Product wording can still require future cleanup if drift appears. |
| Sync split children completed | GitHub issues #221-#225 closed / project Done; PRs #234-#237 for #222-#225 | 2026-06-24 | Completes focused children, not parent #211 production readiness. |
| Save/apply alias policy accepted | PR #237 squash merge `4f7ca3846c4bb4de5f6d5cc98c91318a70b5e15e`; issue #225 closed | 2026-06-24 | Alias policy only; backup/restore and legacy public-surface cleanup deferred. |
| Standards reconciliation completed | PR #239 squash merge `1e2eafeebe26adbb5cd3bc1ad70ec39a93abd5ec`; issue #238 closed / Done | 2026-06-24 | Records state only; product gates remain open. |
| Public backup/restore workflow removed | PR #243 squash merge `04ba7114fb00479fa736b00850a9aa85e8a55a69`; issue #212 closed; final Pro verdict acceptable | 2026-06-25 | Removes public product surface only; lower-level internal recovery/snapshot/ledger mechanics remain implementation details. |
| Legacy v1 public surface separated from v2 happy path | PR #247 squash merge `3bc34d970358854abaca2491ed1f2ef91f8b325b`; issues #213 and #226 closed after Project Owner acceptance | 2026-06-26 | Retains direct legacy command invocation for compatibility; future deletion, warnings, or formal deprecation require separate explicit issue. |
| Status/diff/sync parent gate accepted | Issue #211 closure comment after Project Owner selected option 1 to accept and close #211 | 2026-06-26 | Parent acceptance only; the combined v2 product remains open until catalog, bootstrap, and production documentation gates are complete or deferred. |
| Catalog trust/origin model accepted | PR #250 squash merge `696e235d82856295c04c31941ca31e63e1935e67`; issue #227 closure record after Project Owner accepted closure | 2026-06-26 | Specifies design and handoff requirements only; #228/#229 still need implementation and runtime verification. |
| External Project Execution Standard updated with lifecycle gates | `/Users/shpoont/Work/shpoont/project-execution-standards/project-execution-standard.md`, last-updated 2026-06-29 | 2026-06-29 | Local repo adaptation tracked by #256; external standard is linked, not copied. |
| #228 recovery gate identified | Draft PR #255 exists; CI coverage failure observed; Project Owner discussion identified missing explicit implementation-start/public-surface mock gate | 2026-06-29 | Recovery decision still pending: runnable/replayable mock, waiver, or recontract. |
| Local lifecycle-gate process hardening completed | PR #257 / issue #256 adapted local AGENTS, tailoring, templates, and execution record to the updated standard | 2026-06-29 | Process-only change; does not validate #228 product behavior. |

## Phase 1 acceptance state

Phase 1 scaffold acceptance:

- [x] Transformation plan exists.
- [x] Execution record exists.
- [x] Tailoring rules exist.
- [x] Issue and PR templates exist.
- [x] #209 links the execution record through the source-of-truth decision.
- [x] A standards-adoption issue tracks this scaffold.
- [x] Pro validation says the scaffold is acceptable for Phase 1.
- [x] Project Owner accepts #209 + this file as the v2 reset source of truth.

Acceptance decision:

- Date: 2026-06-23
- Acceptor: Project Owner
- Decision: accept #209 as the v2 reset project charter and this file as the
  repo-editable v2 reset execution record.
- Scope: Phase 1 process scaffold only.
- Explicit limit: this acceptance does not validate current v2 runtime behavior.

## Phase 2 and Phase 3 state

Phase 2 reset audit/discovery:

- [x] Inspect current v2 code, docs, tests, and tracker state from current
      `main`.
- [x] Identify old-model concepts and mandatory-Git assumptions.
- [x] Classify each finding as keep, rename, remove, hide, defer, or owner
      decision.
- [x] Propose issue updates or splits for #210-#216.
- [x] Record audit evidence with exact commit, commands, and file references.
- [x] Validate audit with Pro.
- [x] Close #219 with audit acceptance and follow-up issue edits/splits.

Phase 3 recontract/resequence:

- [x] #210 vocabulary/product model accepted.
- [x] #211 split into focused sync children #221-#225.
- [x] #214 split into catalog children #227-#229.
- [x] #215 split into bootstrap children #230-#231.
- [x] #213 expanded with legacy public-surface child #226.
- [x] #227 catalog/tap trust and origin model accepted.
- [ ] Close or explicitly keep parent issues based on remaining gates. Current
      decision: #211 is closed; keep #214/#215 open as parent issues for the
      reasons above.

## Current next gate

#256 local process hardening is complete. The current gate is #228 recovery:

- create/review a runnable or replayable CLI mock and compare PR #255 against it;
- or record a Project Owner waiver/not-applicable decision that accepts the
  existing storyboard/static transcript evidence for #228;
- or recontract #228 before implementation continues.

Only after #228 recovery is recorded should the executor fix PR #255 coverage,
rerun validation, request review/acceptance, mark PR #255 ready, merge, or close
#228.

Paired follow-up:

- #230 — specify new-computer UX/output can proceed alongside catalog work when
  the work is kept separate from catalog trust/write-authority decisions and has
  its own public-surface/mock gate.

Do not edit runtime behavior, CLI help, tests, specs, or end-user docs as part
of standards-record maintenance except where necessary to keep the project
source-of-truth records accurate.
