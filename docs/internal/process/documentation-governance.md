---
owner: Documentation Maintainer (TBD)
status: Policy
last-updated: 2026-02-16
canonical-source: docs/internal/process/documentation-governance.md
---

# Documentation governance

## Goal

Keep docs scalable as scope grows by separating audience, scope, and canonical ownership.

## Audience boundary

- `docs/user/*` is user-facing.
- `docs/internal/*` is internal engineering/spec content.

Do not mix user-facing narrative into internal spec docs.

## Canonical ownership

- Behavior source of truth: `../specs/decisions.md`
- Machine contracts: `../contracts/*`
- Scenario mapping: `../specs/decision-matrix.md`
- Acceptance gating: `../engineering/acceptance-checklist.md`

## Update workflow

When behavior changes:
1. update `../specs/decisions.md`
2. update affected contract docs (`../contracts/*`)
3. update matrix/checklist (`../specs/decision-matrix.md`, `../engineering/acceptance-checklist.md`)
4. update summaries (`../specs/cli-and-config-spec.md`, README pointers)

## Placeholder policy

If content does not exist yet:
- create a placeholder file with clear `TBD` sections
- include intended scope and owning team/person if known
- keep links in index docs so missing pieces are visible
