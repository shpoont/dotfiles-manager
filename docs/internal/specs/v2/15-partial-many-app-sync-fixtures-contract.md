---
owner: Product + Core Engineering
document-type: v2-behavior-fixture-contract
status: Active behavior spec
authority: Fixture and golden-output contract for partial and many-app sync UX from issue #224; does not introduce new driver or selector implementation authority.
source-issue: 224
last-updated: 2026-06-23
---

# Partial and many-app sync UX fixtures contract

## Purpose

This contract defines the reusable fixture family for many-app and partial-sync
UX. It extends the active status/diff, smart-sync planning, and sync execution
contracts without broadening runtime behavior beyond #224.

The fixtures must make the happy path obvious for a user who wants to manage
many applications at once:

1. see current state across apps;
2. understand which settings are safe to sync;
3. sync all safe changes or a narrow selected subset;
4. understand why missing, unsupported, conflicted, or skipped settings did not
   block unrelated safe sync work.

## Scope

In scope:

- many-app status/dashboard examples;
- many-app planned sync examples;
- partial sync by app;
- partial sync by a public **setting area** within an app;
- partial sync by scalar setting;
- mixed execution results with safe writes and skipped unsafe/unavailable items;
- text and JSON fixtures that can be reused by docs and future implementation
  tests.

Out of scope:

- final CLI selector syntax beyond the fixture `selection` objects;
- new app/driver support;
- backup/restore, v1 migration, or Git-required storage behavior;
- changing #223 execution safety boundaries.

## Public wording rule

The issue uses “resource” for the implementation concern of selecting a
sub-area of an app. Public fixtures must call this a **setting area**. The
fixtures must not expose internal driver, resource id, desired URI/path,
backup/restore, migration, or repository-required vocabulary.

## Required fixture family

Fixtures live in:

`docs/internal/specs/v2/fixtures/partial-many-app-sync/`

Required contract fixtures:

| Fixture | Text | JSON | Purpose |
| --- | --- | --- | --- |
| `01-many-app-status.status` | yes | yes | all-app status/dashboard across clean, changed, conflicted, missing, and unsupported settings |
| `02-many-app-planned-sync.plan` | yes | yes | all-app sync plan with safe writes and skipped unsafe/unavailable items |
| `03-partial-by-app.plan` | yes | yes | app-only selection does not plan unrelated app writes |
| `04-partial-by-setting-area.plan` | yes | yes | public setting-area selection within one app |
| `05-partial-by-scalar-setting.plan` | yes | yes | scalar setting selection does not write neighboring settings |
| `06-mixed-sync-result.execution` | yes | yes | execution result where unrelated safe writes complete while other items are skipped |

## Fixture invariants

Every JSON fixture must:

- parse as JSON;
- include `command`, `selection`, `summary`, and `items`;
- make partial selection scope explicit;
- keep skipped, blocked, missing, unsupported, conflicted, and changed states
  semantically distinct;
- avoid hidden internal diagnostics and internal storage/backend identifiers;
- keep text and JSON aligned on selected scope, write count, skipped count, and
  sync directions.

Read-only status fixtures must not expose write-decision fields such as
`decision`, `wouldWrite`, `executableBySync`, or `result`. They may show the
direction that a later sync plan would use, but they must remain inspection
output.

Planning and execution fixtures must:

- set `wouldWrite=false` and `executableBySync=false` on every skipped,
  conflicted, blocked, missing, unsupported, or not-selected item;
- keep safe unrelated writes executable when another selected or visible item is
  missing, unsupported, conflicted, or skipped;
- visibly separate safe writes, needs-choice items, cannot-sync-now items, and
  not-selected items in text output and JSON summaries.

Partial fixtures must additionally prove that selected scope controls planned
writes:

- app selection plans writes only for that app;
- setting-area selection plans writes only inside that app area;
- scalar setting selection plans only that setting and explicitly reports zero
  out-of-scope writes.

## Validation split

The CI fixture checker owns cheap contract invariants: required files, public
vocabulary, JSON parseability, summary/item consistency, write safety flags,
selection-scope checks, and text/JSON direction coverage.

Go tests should be added only when implementation code changes. They should
cover selector resolution, planner/executor boundaries, and renderer stability,
not every word in these prose fixtures.

End-user documentation may quote or adapt these fixtures, but docs-only examples
are non-normative unless promoted into this fixture directory and checker.
