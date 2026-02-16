---
owner: Product + Core Engineering
status: Implementation-ready
last-updated: 2026-02-16
canonical-source: docs/internal/scope/product-scope.md
---

# Product scope

## Purpose

`dotfiles-manager` synchronizes managed dotfiles between:
- a repository-local **source** tree (manifest, source of truth), and
- one or more `$HOME`-relative **target** trees.

## Core commands (scope)

- `status` — preview drift and candidate operations
- `deploy` — apply source -> target
- `import` — apply target -> source within configured rules

## In-scope behavior

- Config-driven sync definitions (`syncs`)
- Path-scoped execution with optional `[path]`
- Pattern-driven unmanaged import and unmanaged removal behavior
- Missing-path import deletion behavior by include/exclude patterns
- Machine-readable JSON output (`--json`)
- Dry-run for mutating commands (`deploy`, `import`)

## Out of scope (currently)

- Locking/multi-process coordination
- Conflict-state model with merge history
- Full end-user UX polish and release-grade packaging docs (user docs are in active draft under `docs/user/`)

## Status

Internal spec is implementation-ready; non-blocking follow-ups are tracked in `../specs/open-questions.md`.
