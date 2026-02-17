# Documentation map

This directory is organized by **audience** first, then by **scope/detail level**.

## Audience split

- `docs/user/` — user-facing docs (public/operator guidance).
- `docs/internal/` — internal product/engineering docs (canonical for implementation).

## Scope levels (internal)

1. **High-level scope** (`internal/scope/`)
   - What the tool is, what it is not, and system-level context.
2. **Detailed specifications** (`internal/specs/`)
   - Behavior definitions, decisions, matrix outcomes, open questions.
3. **Contracts** (`internal/contracts/`)
   - Stable machine/engineering contracts (`--json`, config schema, metadata, logging, error codes).
4. **Engineering execution** (`internal/engineering/`)
   - Technical requirements, testing strategy, acceptance criteria, CI/CD requirements.
5. **Process/governance** (`internal/process/`)
   - Documentation governance, ownership, and change workflow.

## Canonical source rules

- Each topic has one canonical document.
- Other docs should link to canonical docs rather than re-stating behavior.
- If a behavior changes, update in this order:
  1) `internal/specs/decisions.md`
  2) related contract docs (`internal/contracts/*`)
  3) matrix/checklists (`internal/specs/decision-matrix.md`, `internal/engineering/acceptance-checklist.md`)
  4) any derived summaries.

For practical navigation:
- Start at `internal/README.md` for implementation work.
- Start at `user/README.md` for end-user usage docs.
