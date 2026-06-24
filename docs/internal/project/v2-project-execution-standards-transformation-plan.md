---
owner: Project Owner + Work Manager
status: Active transformation plan; Phase 4 execution ongoing
last-updated: 2026-06-24
canonical-source: docs/internal/project/v2-project-execution-standards-transformation-plan.md
related-issues: [209, 217]
---

# v2 Project Execution Standards transformation plan

## Purpose

This plan defines how `dotfiles-manager` v2 will be transformed to follow the
Project Execution Standards. It applies to existing v2 prototype behavior, new
functionality, documentation, GitHub issues, project-board flow, pull requests,
validation, acceptance, and closure.

The goal is not to add bureaucracy. The goal is to make each piece of work
clear, testable, evidence-backed, accepted, and safe to continue from by another
human or AI agent.

## External standard

The canonical execution standard is maintained outside this repository in the
`project-execution-standards` project. This repository adopts that standard
through the tailoring rules in
[`../process/v2-execution-standards-tailoring.md`](../process/v2-execution-standards-tailoring.md)
and the project execution record in
[`v2-reset-execution-record.md`](v2-reset-execution-record.md).

Do not copy the full external standard into this repository. If the external
standard changes, update the tailoring document and project execution record only
where the change affects this project.

## Current baseline

As of 2026-06-24:

- The active product direction is the v2 reset around settings-folder sync.
- GitHub issue #209 is the parent v2 reset issue and remains open until the
  combined reset outcome is accepted.
- Phase 1 scaffold adoption is complete (#217 / PR #218).
- Phase 2 audit/discovery is complete for sequencing purposes (#219 / PR #220),
  with remaining product decisions carried by follow-up issues.
- Product model/vocabulary is accepted (#210).
- The focused sync split is complete (#221-#225), including the `save`/`apply`
  alias policy in PR #237. Parent #211 remains open because #212 and #213/#226
  are public-scope gates before production-ready sync acceptance.
- Catalog and bootstrap parent areas remain open through their split children:
  #214 -> #227-#229 and #215 -> #230-#231.
- Production documentation (#216) remains open until accepted behavior and
  examples from remaining gates are available.
- Reconciliation issue #238 closed the source-of-truth drift that appeared after
  several rapid issue closures.
- New work must start from `main` on a fresh issue-linked branch, not from old
  closed-PR branches.

## Compliance definition

The project follows the Project Execution Standards when all of these are true:

1. **One project source of truth exists.** GitHub issue #209 is the project-level
   charter and `docs/internal/project/v2-reset-execution-record.md` is the
   repo-editable project execution record.
2. **Every active work item has a contract.** Each issue states type, risk tier,
   outcome, non-goals, dependencies, evidence requirements, validation rule,
   acceptance rule, and closure requirements at the depth appropriate to its
   risk.
3. **Implementation-last is enforced for meaningful delivery work.** User-facing
   or mutating behavior is not implemented until its issue has a frozen outcome,
   UX/model evidence, and real-result verification plan.
4. **Discovery is explicit.** Unknowns that block safe delivery become discovery
   items with a question, method, evidence, decision owner, stop condition, and
   next decision.
5. **Design evidence and real-result verification are not confused.** Storyboards,
   docs-first drafts, Pro reviews, and mock transcripts are design evidence only;
   completion requires real implementation or artifact verification.
6. **Validation, acceptance, and closure are separate.** A PR or issue is not
   done merely because code exists or tests pass. The validation result,
   acceptor decision, exceptions, and follow-up state must be recorded.
7. **Evidence is inspectable.** Material claims point to tests, command output,
   fixtures, docs validation, review records, or other inspectable evidence with
   enough provenance for another authorized reviewer to understand the decision.
8. **Branch state is controlled.** Work starts from current `main`; old branches
   and closed PR branches are reference only unless explicitly selected for
   cherry-pick or salvage.
9. **User-facing docs are part of design and acceptance.** Draft docs or
   equivalent usage guides are used before implementation for user-facing work,
   then reconciled against real behavior before production documentation is
   accepted.
10. **Project-level integration remains visible.** After each work item closes,
    #209 and the execution record are updated with new evidence, risks,
    dependencies, and next action.

## Transformation phases

### Phase status as of 2026-06-24

| Phase | Status | Evidence / next action |
| --- | --- | --- |
| Phase 1 — Install execution scaffold | Complete | #217 closed; PR #218 merged. |
| Phase 2 — Audit current v2 | Complete for sequencing | #219 closed; PR #220 merged; remaining decisions live in follow-up issues. |
| Phase 3 — Recontract and resequence | Substantially complete | #210 accepted; #211/#214/#215 split into focused children; parent issues remain open where gates remain. |
| Phase 4 — Execute frozen work items | Active | #238 reconciled source-of-truth drift; #212 is the recommended next product issue. |
| Phase 5 — Project-level acceptance | Not started | Requires remaining gates, clean-environment validation, and production docs. |

### Phase 1 — Install the execution scaffold

Objective: make the standard operational without changing product behavior.

Deliverables:

- This transformation plan.
- `docs/internal/project/v2-reset-execution-record.md` as the repo-level project
  record.
- `docs/internal/process/v2-execution-standards-tailoring.md` as the
  project-specific rules for applying the external standard.
- `.github/ISSUE_TEMPLATE/work_item.md` for future work items.
- `.github/PULL_REQUEST_TEMPLATE.md` for evidence-backed PRs.
- Update `docs/internal/README.md` so these artifacts are discoverable.
- GitHub issue #217 tracks this adoption work and is linked from #209.

Completion evidence:

- Files exist in the repository.
- The tracking issue links the files and states this is scaffold-only work.
- #209 identifies itself as the v2 reset charter and links the execution record.
- No product behavior is changed by this phase.

Acceptance:

- Project Owner accepts #209 + `v2-reset-execution-record.md` as the v2 reset
  source of truth.

### Phase 2 — Audit current v2 against the reset and the standard

Objective: establish what current code/docs/tests can be reused and what must be
changed before v2 can be treated as production-directed work.

Deliverables:

- A reset audit report under `docs/internal/project/` or `docs/internal/evidence/`
  with a table of stale concepts and affected files.
- Inventory of old-model concepts: `repo`, `repository`, `save`, `apply`,
  `backup`, `restore`, `migration`, `user`, `profile`, and any mandatory-Git
  assumptions.
- Classification for each finding: keep as internal detail, rename, remove,
  hide, defer, or needs owner decision.
- Proposed issue updates/splits for #210-#216 based on the audit.

Completion evidence:

- Audit command(s) and exact commit/branch inspected are recorded.
- Each material product claim has file references or grep/test evidence.
- Unknowns are converted into discovery work or explicit owner decisions.

Acceptance:

- Project Owner accepts the audit classification and any required product
  decisions before implementation continues.

### Phase 3 — Recontract and resequence the active reset issues

Objective: convert #210-#216 from broad reset tasks into standard-compliant work
items.

Planned issue treatment:

| Issue | Treatment | Risk tier | Notes |
| --- | --- | --- | --- |
| #209 | Project charter / parent issue | Project-level Tier 1 | Keep open until reset outcome is accepted. |
| #210 | Freeze product model and vocabulary | Tier 1 | First delivery-design item. Blocks docs and UX. |
| #211 | Parent for sync UX and implementation | Tier 2 overall | Split into sync UX contract, read-only status/diff, mutating sync writes, and partial sync. |
| #212 | Remove backup/restore from product scope | Tier 0/Tier 1 | Keep small; decide remove/hide/legacy-internal. |
| #213 | Remove v1 migration from v2 roadmap | Tier 0/Tier 1 | Keep small; docs/spec/tracker cleanup. |
| #214 | Recipe catalog/tap support | Discovery, then Tier 1/Tier 2 delivery | Trust/write-authority model before remote catalog writes. |
| #215 | New-computer bootstrap flow | Tier 1/Tier 2 | Split into docs-first UX/fixtures before implementation. |
| #216 | End-user documentation | Tier 1 | Split draft docs-as-design from production verified docs. |

Completion evidence:

- Each active issue has type, risk tier, dependencies, evidence requirements,
  validation, acceptance, and closure sections.
- Oversized items are split or explicitly retained as parent issues.
- Project board statuses match the new sequence.

Acceptance:

- Project Owner accepts the resequenced issue set as the working v2 roadmap.

### Phase 4 — Execute one frozen work item at a time

Objective: implement v2 through small standard-compliant slices.

Default work-item flow:

1. Select the next issue from #209's sequence.
2. Confirm type and risk tier.
3. Fill or refresh the issue contract.
4. Run discovery if needed.
5. Draft user-facing docs, CLI transcript, UX storyboard, state model, or other
   mock surface before implementation for user-facing work.
6. Run a design evidence pass.
7. Freeze the issue package.
8. Create a fresh issue-linked branch from `main`.
9. Implement only the frozen scope.
10. Verify the real result with tests, temp-home fixtures, command output, docs
    validation, or other inspectable evidence.
11. Record validation and acceptance separately.
12. Close, update #209, and replan the next item.

Branch rule:

```text
main -> codex/<issue>-<short-slug> -> PR -> validation -> acceptance -> merge
```

Old branches are reference only. Do not build new work on a closed/superseded PR
branch unless the Project Owner explicitly selects that branch for salvage.

### Phase 5 — Project-level acceptance and release readiness

Objective: accept the combined v2 reset outcome, not just individual issues.

Deliverables:

- Integrated v2 product model and CLI behavior.
- Verified beginner path in a clean temporary environment.
- Production user docs reconciled against real behavior.
- Recipe/catalog trust and bootstrap behavior documented at the correct maturity.
- Release/adoption plan with support path, limitations, and rollback/recovery
  story.

Completion evidence:

- All required reset work items are accepted or explicitly deferred/non-goal.
- Clean-environment validation passes.
- Documentation quality acceptance criteria pass.
- Evidence quality expectations are met for all completion claims.
- Known limitations and future work are separated from completion requirements.

## Current execution plan

The first scaffold slice is complete. Current work is in Phase 4: execute one
frozen work item at a time while keeping #209 and the execution record current.

Current steps:

1. Proceed to #212 as the recommended next product issue, because
   backup/restore is still the clearest public-scope contradiction with the
   reset model.
2. Treat #213/#226 as the paired legacy public-surface gate after or alongside
   #212.
3. Continue catalogs (#227 -> #228/#229), bootstrap (#230 -> #231), and final
   production docs (#216) only after their prerequisite decisions/examples are
   accepted.

## Anti-patterns to avoid

- Treating this transformation as a separate permanent project instead of a way
  to execute v2.
- Making every task Tier 2.
- Starting implementation from old closed-PR branches.
- Letting #211 become a single giant sync rewrite.
- Treating storyboards or docs drafts as proof that the real product works.
- Publishing production docs before real behavior is verified.
- Using Git repository terminology as a required user-facing product model.
- Preserving backup/restore as the v2 safety story.
- Reintroducing v1 migration through docs, tests, or acceptance criteria.
- Implementing remote catalogs before trust, origin, update, and write-authority
  rules are explicit.
- Treating GitHub Project status as validation or acceptance.
- Closing issues without validation, acceptance, and next-state updates.
