---
owner: Project Owner + Work Manager
status: Process tailoring
last-updated: 2026-06-29
canonical-source: docs/internal/process/v2-execution-standards-tailoring.md
---

# v2 execution standards tailoring

This document explains how `dotfiles-manager` v2 applies the external Project
Execution Standards without turning normal development into ceremony.

## Scope

This tailoring applies to all v2 reset work, including:

- product model and vocabulary;
- CLI UX and output;
- syncing live app settings with stored settings;
- recipe schema and catalogs;
- app/native import/export drivers;
- documentation;
- tests, fixtures, validation, release, and acceptance.


## Agent entrypoint

The repository root `AGENTS.md` is the operational entrypoint for agents and
maintainers. It does not replace or summarize the external Project Execution
Standard. Instead, it points to the external standard, this tailoring document,
the v2 execution record, and the active GitHub issue contract in the required
reading order.

If the external standard is unavailable, agents must stop and ask for a decision
instead of proceeding from memory, chat history, or copied fragments.

## Source-of-truth rules

- Project-level source of truth: GitHub issue #209 plus
  `docs/internal/project/v2-reset-execution-record.md`.
- Work-item source of truth: the GitHub issue for that work item.
- Durable product decisions/specs: `docs/internal/scope/`,
  `docs/internal/specs/`, `docs/internal/contracts/`, and `docs/internal/ux/`.
- Evidence that must survive PR comments: `docs/internal/project/` or a future
  `docs/internal/evidence/` location, when needed.
- GitHub Project board: status and ordering only. It is not acceptance evidence.

## Risk-tier defaults for this project

| Work type | Default tier | Notes |
| --- | --- | --- |
| Typos, link updates, tiny reversible docs changes | Tier 0 | Use the compressed checklist. |
| Product vocabulary, docs-first UX, CLI output shape | Tier 1 | Needs design evidence before implementation. |
| Read-only status/diff behavior | Tier 1 | Verify with fixtures/temp-home command output. |
| Writes to live app settings or stored settings | Tier 2 unless narrowly reversible | Needs conservative rules, preview, recovery, real-result verification. |
| Remote recipe catalog trust/update/write authority | Discovery first, then Tier 2 | Do not implement remote writes before trust model. |
| Production end-user docs | Tier 1 | Draft before implementation; production acceptance needs real behavior validation. |
| Release/adoption gates | Tier 1/Tier 2 depending on user impact | Requires project-level integration evidence. |

Do not make every issue Tier 2. Escalate only when the work can mutate real user
settings, introduce trust/security/privacy risk, create irreversible changes, or
make public release claims.

## Lifecycle gate rules

The updated external standard's lifecycle gate / gate passport model is mandatory
for material phase transitions in v2 reset work. Apply it before moving between
discovery, design evidence, freeze, implementation, real-result verification,
validation, acceptance, closure, merge, release, or adoption.

For each non-trivial work item, future sessions must be able to identify:

- current lifecycle phase and requested transition;
- current source of truth and whether it was refreshed;
- active issue and open PR, if any;
- frozen package/version, or why the item is still pre-freeze;
- public-surface freeze status when relevant;
- runnable/replayable mock status when relevant;
- open questions, blockers, waivers, accepted exceptions, or managed changes;
- owner decision needed, allowed next action, and no-go actions.

If a gate cannot be reconstructed from current records, execution is blocked for
implementation and release-impacting work. The agent may inspect, draft evidence,
create a mock, ask for the gate decision, record a blocker, or recontract the
issue.

## Public-surface and runnable-mock rule

`dotfiles-manager` is a CLI-first product, so command shape and output are
product UX. For meaningful Tier 1/Tier 2 changes to CLI commands, help text,
status/diff/sync output, catalog/source vocabulary, recipe/app discovery,
workflow prompts, public docs, or operator instructions, design evidence must
include runnable or replayable usage evidence by default. Examples include:

- runnable mock CLI;
- scripted fixture transcript;
- golden-output fixture;
- temp-home dry-run;
- executable docs example;
- replayable storyboard with commands, fixtures, and expected output that another
  reviewer can independently rerun or mechanically compare.

Here, "replayable" means executable, scripted, fixture-backed, golden-output
backed, or otherwise independently reproducible. A pasted static transcript,
storyboard, issue comment, Pro review, or subagent review is not enough for
meaningful public-surface implementation unless the issue records
a decision-owner waiver or not-applicable decision. The waiver must state scope,
reason, residual risk, alternative evidence accepted, and what it does not
authorize.

Design acceptance, implementation-start authorization, validation, acceptance,
merge, release, and closure are separate decisions. Pro and subagent reviews are
advisory evidence only; they do not replace Project Owner or delegated acceptor
decisions.

## Minimum issue contract

Each non-trivial work item must identify:

- type: delivery, discovery, cleanup, enabler, or parent;
- risk tier and reason;
- outcome and non-goals;
- affected user/operator;
- dependencies and blockers;
- design evidence required;
- real-result verification required;
- validation criteria;
- acceptor and acceptance rule;
- privacy/authority/recovery considerations;
- closure requirements.

Tier 0 work may use the compressed checklist:

```text
Tier 0 checklist:
- Outcome:
- Quick check:
- Result/evidence:
- Accepted/closed by:
```

## Freeze and implementation-start rule

Meaningful delivery work cannot begin until the issue contract is frozen and an
implementation-start gate is current for the intended action.

A frozen issue has:

- clear outcome and non-goals;
- settled user-facing nouns/output for that slice;
- public-surface shape frozen or explicitly not applicable;
- runnable/replayable usage evidence reviewed, waived, or explicitly not
  applicable where the work affects public/user/operator surfaces;
- evidence plan;
- dependencies resolved, blocked, or explicitly waived;
- validation and acceptance rule;
- branch name or PR plan;
- allowed next action and no-go actions.

After freeze, changes to scope, behavior, risk, evidence, dependencies,
public-surface shape, or runnable/replayable evidence requirements are managed
changes. They must be recorded in the issue or PR instead of silently
implemented.

## Design evidence rule

For user-facing work, design evidence should be the smallest useful
pre-implementation model:

- CLI transcript;
- docs-first guide;
- UX storyboard;
- state machine;
- example config/schema;
- temp-home fixture plan;
- systems pressure-test notes.

Design evidence proves the intended model is coherent. It does not prove that
real behavior works.

## Real-result verification rule

Completion claims require real-result evidence. For this project, preferred
real-result evidence includes:

- `go test ./...` or targeted test output;
- CLI command output from a clean temporary home or fixture storage folder;
- before/after file or settings inspection;
- generated artifact diff;
- docs link check or docs validation output;
- source references for any product/support claim;
- explicit limitations for unverified areas.

## Branch rule

Start every work item from current `main` unless the Project Owner explicitly
chooses a different base.

Before implementation:

```bash
git fetch --prune origin
git switch main
git pull --ff-only origin main
git switch -c codex/<issue-or-topic>-<short-slug>
```

Old local or remote branches from closed PRs are reference only. Do not continue
work from a superseded branch because it may carry obsolete product assumptions.

## PR evidence rule

Every PR should state:

- related issue;
- risk tier;
- frozen contract or issue section used;
- what changed;
- design evidence used;
- real-result verification performed;
- validation result;
- docs impact;
- known exceptions or follow-up issues.

## Closure rule

An issue closes only after:

- linked PRs or artifacts exist;
- real-result evidence is recorded or explicitly waived;
- validation outcome is recorded;
- Project Owner or delegated acceptor accepts, rejects, or accepts with
  exceptions;
- follow-up issues are created for accepted exceptions or future work;
- #209 and the execution record are updated when the result changes project
  sequence, risks, or acceptance state.
