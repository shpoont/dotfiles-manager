# Agent operating instructions for dotfiles-manager

This repository uses an external Project Execution Standard. Do not treat chat
history, local memory, stale plans, or this file as a substitute for the current
standard and the current work-item contract.

## Required reading before non-trivial work

Before changing files, GitHub issues, project state, or release artifacts, read
these in order:

1. External execution standard:
   `/Users/shpoont/Work/shpoont/project-execution-standards/project-execution-standard.md`
2. Repo-specific v2 tailoring:
   `docs/internal/process/v2-execution-standards-tailoring.md`
3. Current v2 project state:
   GitHub issue #209 and
   `docs/internal/project/v2-reset-execution-record.md`
4. Active GitHub issue contract for the work item.
5. Relevant specs, docs, tests, fixtures, and existing behavior evidence for the
   files or product surface being touched.

The external standard is intentionally not copied or vendored in this repo. If
it is missing, unreadable, or conflicts with local instructions, stop and ask for
a decision. Do not proceed from memory, summaries, or copied fragments.

## Session-start gate readback

For non-trivial work, do not jump directly from a broad user prompt to
implementation, CI repair, merge, release, acceptance, or closure. After the
required reading above and before selecting the next action, produce or record a
short gate readback covering:

- current branch, HEAD, working-tree status, and unrelated untracked files;
- live GitHub issue, PR, check, review, and project state relevant to the work;
- current Project Execution Standards lifecycle phase and requested transition;
- active issue contract, frozen package/version, design evidence, managed
  changes, validation rule, acceptance rule, and closure rule;
- public-surface freeze status and runnable/replayable mock status when the work
  affects CLI, UI, API, documentation-facing, or operator-facing behavior;
- exact next action and explicit no-go actions until the current gate is
  satisfied.

If an open PR already owns the active issue, resume that PR and its current
standards gate. Do not restart from `main`, create competing implementation, or
reinterpret the work unless the issue is explicitly recontracted.

## Lifecycle gate and implementation-start rules

Use the updated external standard's lifecycle gate / gate passport model for
material phase transitions. A broad response such as "continue", "proceed",
"ok", or "looks good" permits only work already inside the current approved
package and role authority. It does not by itself approve a transition into
implementation, validation, acceptance, closure, merge, release, a waiver, a
managed change, or a weakened evidence plan.

Before changing tracked files for implementation, product behavior, non-trivial
process, templates, project state, or release-impacting work, verify that there
is a current implementation-start gate for the active work item. The gate
must identify the source of truth, frozen package/version, public-surface status
where relevant, runnable/replayable mock status where relevant, unresolved
questions, owner decision, allowed next action, and no-go actions.

If the gate is missing, stale, ambiguous, or not scoped to the intended action,
the allowed actions are limited to inspection, drafting/revising evidence,
creating a mock or fixture, asking for the gate decision, updating the issue
contract, or recording a blocker. Do not implement around the missing gate.

For meaningful Tier 1/Tier 2 changes to CLI, UI, API, workflow, help/output,
public documentation, or operator-facing behavior, default to runnable or
replayable pre-implementation usage evidence. Static storyboards or transcripts
are sufficient only when the active issue records why stronger usage evidence is
not applicable or is explicitly waived by the decision owner.

## Source-of-truth precedence

For active v2 reset work, use this precedence:

1. The external Project Execution Standard governs the general execution method.
2. `docs/internal/process/v2-execution-standards-tailoring.md` governs how that
   method is applied in this repository.
3. GitHub issue #209 plus
   `docs/internal/project/v2-reset-execution-record.md` describe current v2
   project state, sequencing, risks, and accepted constraints.
4. The active issue contract governs frozen scope, non-goals, evidence,
   validation, acceptance, and closure for one work item.
5. Relevant specs, contracts, docs, tests, and fixtures govern local behavior
   only when they are not marked stale or superseded by the v2 reset record or
   the active issue contract.

Older v1-oriented docs are historical reference for v2 unless an active issue
contract or the v2 execution record explicitly reaffirms them. If v1 docs,
existing implementation, or tests conflict with the v2 reset direction, record
the conflict in the issue and resolve it through discovery, contract update, or
managed change before implementation.

## Development lifecycle

Use the issue as the unit of work. Do not start implementation from a broad
request such as "continue v2" without selecting or creating a work item.

1. Refresh state: fetch/prune remotes, inspect current GitHub issue/project
   state, and start from current `main` unless explicitly told otherwise.
2. Confirm the work item: type, risk tier, affected users/operators, source of
   truth, scope, non-goals, dependencies, evidence, validation, acceptance, and
   closure rule.
3. Create a fresh issue-linked branch from `main` using
   `codex/<issue-or-topic>-<short-slug>` before making tracked file changes.
4. Choose the pre-implementation path:
   - user-facing behavior: draft docs, UX transcript, examples, or storyboard
     first;
   - settled behavior: write or update tests before implementation;
   - legacy/v1 behavior changes: characterize current behavior before changing
     it;
   - unclear or conflicting behavior: run discovery first and do not implement
     until the contract is clear.
5. Freeze the issue contract before implementation. After freeze, scope,
   behavior, risk, evidence, dependency, deadline, or acceptance changes are
   managed changes and must be recorded.
6. Implement only the frozen scope. Do not patch around unresolved requirements,
   failing evidence, missing standards, or source-of-truth conflicts.
7. Verify the real result with inspectable evidence: tests, command output,
   fixtures, temp-home runs, docs validation, diffs, logs, or other evidence
   appropriate to the issue risk tier.
8. Open a draft PR with the issue link, risk tier, frozen contract, design
   evidence, real-result verification, validation result, docs impact, and any
   exceptions or follow-ups.
9. Use subagent review for PR review by default when available. Use Pro review
   for product, process, UX, or standards-sensitive decisions when useful or
   required by the active issue; if unavailable, record the review limitation in
   the PR.
10. Mark ready and merge only after required validation passes and acceptance is
    recorded or follows a predeclared auto-accept rule.
11. After closure, update #209 and
    `docs/internal/project/v2-reset-execution-record.md` only when project-level
    state, sequencing, risk, accepted constraints, or next action changes.

## Scope and safety rules

The following are current repo-local constraints for the v2 reset. If they
conflict with #209, the v2 execution record, or the active issue contract,
follow the precedence rules above and reconcile the conflict before
implementation.

- Keep backup/restore out of v2 product scope unless the active issue changes
  that decision.
- Treat v1 as historical reference only; do not create v1 migration work unless
  explicitly recontracted.
- Use "settings storage folder" for v2 user-facing storage language unless an
  active issue chooses another term.
- Do not require Git as a product assumption; Git is recommended for versioning
  and sharing, not mandatory.
- Do not mutate live user settings, credentials, external accounts, releases, or
  project-board state without the issue contract and required approval.
- Leave unrelated untracked local files untouched.
