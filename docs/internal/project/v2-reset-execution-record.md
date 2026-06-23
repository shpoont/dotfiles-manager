---
owner: Project Owner + Work Manager
status: Active project record
last-updated: 2026-06-23
canonical-source: docs/internal/project/v2-reset-execution-record.md
project-issue: 209
---

# v2 reset execution record

## Source of truth

- Project charter / parent issue: #209
- Standards-adoption scaffold issue: #217
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
- `save` and `apply` are directional sync concepts if retained; they are not the
  main product mental model.
- Backup/restore is out of v2 product scope.
- v1 is historical reference only; no v1 migration roadmap is in scope.
- Current v2 implementation/docs are prototype baseline until audited and
  reaccepted under the reset model.

## Explicit non-goals

- Managing app installation as part of dotfiles-manager.
- Requiring Git for the storage folder.
- A first-class backup/restore workflow for v2.
- v1 migration tooling as part of the current v2 roadmap.
- Remote catalog writes before recipe origin, trust, update, and write-authority
  rules are explicit.
- Treating old closed PR branches as active work branches.

## Active work-item table

| Issue | Type | Tier | Current role | Dependencies / notes |
| --- | --- | --- | --- | --- |
| #209 | Project charter / parent | Project-level Tier 1 | Keep active until reset accepted | Must link this execution record. |
| #217 | Enabler scaffold | Tier 1 | Complete | PR #218 merged; closure recorded in GitHub. |
| #219 | Discovery / audit | Tier 1 | Active Phase 2 reset audit | Produces implementation/docs/tracker audit before product work resumes. |
| #210 | Delivery-design | Tier 1 | Freeze product model and vocabulary | Blocks #211, #216, and most user-facing work after #219 acceptance. |
| #211 | Parent delivery area | Tier 2 overall | Split before implementation | Sync writes are mutating and need stronger evidence. |
| #212 | Scope cleanup | Tier 0/Tier 1 | Remove backup/restore from product surface | Requires decision: remove, hide, or legacy/internal. |
| #213 | Scope cleanup | Tier 0/Tier 1 | Remove v1 migration from v2 roadmap | Keep small; close with docs/spec/tracker evidence. |
| #214 | Discovery then delivery | Discovery/Tier 2 for remote writes | Design catalog/tap and trust model | Remote writes require authority/trust model first. |
| #215 | UX/design then delivery | Tier 1/Tier 2 | Define new-computer bootstrap flow | Homebrew Bundle is example, not dependency. |
| #216 | Documentation delivery | Tier 1 | Docs-first draft then production docs | Production docs require verified real behavior. |

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
| Old v2 prototype docs/code keep driving implementation | Run reset audit before product implementation | Work Manager | Open |
| #211 is too large and mutating | Split design/read-only/writes/partial sync | Work Manager | Open |
| Remote recipe catalogs can execute untrusted writes | Make #214 discovery first with trust/write-authority model | Work Manager | Open |
| Production docs could describe unverified behavior | Separate docs-first draft from verified production docs | Docs owner | Open |
| Branches from closed PRs may be reused accidentally | Enforce branch rule and start from `main` | Executor | Active |

## Evidence index

| Claim / gate | Evidence link | Date | Limits |
| --- | --- | --- | --- |
| Active reset issues are #209-#216 | GitHub issue list read during transformation planning | 2026-06-23 | GitHub state can drift; refresh before mutation. |
| No open PRs before scaffold branch | `gh pr list --state open` read during planning | 2026-06-23 | Refresh before opening new PR. |
| Current branch before scaffold was superseded docs branch | `git status --branch` showed `codex/v2-end-user-docs-207` | 2026-06-23 | Historical state only. |
| Product reset excludes backup/restore, mandatory Git repo, and v1 migration | User decision captured in current reset issues and chat context | 2026-06-23 | Must be promoted into issue contracts/specs. |
| Phase 1 scaffold has a tracking issue | GitHub issue #217 | 2026-06-23 | Must be linked to PR when PR exists. |
| Phase 1 scaffold passed validation | PR #218 checks, local scaffold checks, and Pro review conversation | 2026-06-23 | Validates process scaffold only, not current v2 product behavior. |
| Project Owner accepted the Phase 1 source-of-truth decision | User confirmation: "We are good to go" after explicit acceptance prompt | 2026-06-23 | Acceptance is for #209 + this file as source of truth and PR #218 merge approval. |
| Phase 1 scaffold was merged | PR #218 squash merge `1bbb484eb958d5477937da675da76482a43a8845` | 2026-06-23 | Merge validates process scaffold only. |
| Phase 2 audit artifact exists in draft | [`v2-reset-audit-issue-219.md`](v2-reset-audit-issue-219.md) | 2026-06-23 | Pending Project Owner acceptance and follow-up issue edits/splits. |
| Phase 2 audit received Pro validation | ChatGPT Pro review conversation `https://chatgpt.com/c/6a3a5a21-6bc4-83eb-8449-e08b72d0d267` returned acceptable with no must-fix blockers | 2026-06-23 | Pro validation does not replace Project Owner acceptance or follow-up tracker updates. |

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
- Explicit limit: this acceptance does not validate current v2 runtime behavior;
  Phase 2 must audit current code, docs, tests, and tracker state against the
  reset model before product implementation continues.

## Next gate

Phase 2 reset audit:

- [x] Inspect current v2 code, docs, tests, and tracker state from current
      `main`.
- [x] Identify old-model concepts and mandatory-Git assumptions.
- [x] Classify each finding as keep, rename, remove, hide, defer, or owner
      decision.
- [x] Propose issue updates or splits for #210-#216.
- [x] Record audit evidence with exact commit, commands, and file references.
- [x] Validate audit with Pro.
- [ ] Get Project Owner acceptance of audit classifications and Phase 3
      sequence.
- [ ] Apply or explicitly defer follow-up issue edits/splits.
