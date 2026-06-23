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
| #217 | Enabler scaffold | Tier 1 | Install standards adoption scaffold | Close after scaffold PR is validated and accepted. |
| #210 | Delivery-design | Tier 1 | Freeze product model and vocabulary | Blocks #211, #216, and most user-facing work. |
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

## Next gate

Phase 1 scaffold acceptance:

- [ ] Transformation plan exists.
- [ ] Execution record exists.
- [ ] Tailoring rules exist.
- [ ] Issue and PR templates exist.
- [ ] #209 links the execution record.
- [ ] A standards-adoption issue tracks this scaffold.
- [ ] Project Owner accepts #209 + this file as the v2 reset source of truth.
