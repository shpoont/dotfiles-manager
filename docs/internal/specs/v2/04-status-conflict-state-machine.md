---
owner: Core Engineering
document-type: v2-draft-spec
status: Draft
last-updated: 2026-06-04
canonical-source: docs/internal/specs/v2/04-status-conflict-state-machine.md
source-concept-sections:
  - Canonical status and conflict state machine
  - Status and preview output
  - Mutation transaction model
  - MVP acceptance test matrix
authority: Draft; non-authoritative until promoted by docs/internal/specs/v2/README.md
---

# v2 status and conflict state machine

## Purpose

This spec defines canonical item states, target-level aggregation, and conflict
derivation from desired, current, and last-applied state.

Status must explain what the user can safely do next. It must not hide
uncertainty.

## Status and authority

This is a draft extraction from `../../scope/product-concept-v2.md`. It is not
implementation-authoritative until promoted through `README.md`.

Current v1 behavior remains governed by existing v1 specs and contracts.

## Source map and extraction notes

Extracted from the concept sections covering:

- canonical status states;
- conflict detection using last-applied hashes;
- target-level severity;
- status output grouped by next action;
- mutation ledger update rules.

Deliberate non-decisions:

- exact text rendering is deferred;
- exact normalized hash format is deferred;
- final JSON schema for item states is deferred.

## Terms owned by this spec

- desired normalized state;
- current normalized state;
- last-applied normalized state;
- canonical item state;
- target-level status;
- conflict;
- no-baseline condition.

## Normative MVP rules

### Inputs

Status is computed from:

- selected setting/ref;
- desired artifact existence and normalized state;
- current live existence and normalized state;
- last-applied normalized state from the ledger;
- recipe version;
- driver version;
- normalizer version;
- resource capability;
- lifecycle state;
- trust and safety policy.

### Canonical item states

| State | Meaning | Recommended action |
| --- | --- | --- |
| `unchanged` | Current normalized state equals desired normalized state. | none |
| `changed-current` | Current differs from desired; baseline matches desired. | save or apply |
| `ready-to-apply` | Desired exists and current is missing/different with no local edit. | apply |
| `missing-desired` | Setting selected but no desired artifact exists. | save or create artifact |
| `missing-current` | Desired exists but live path/value is absent. | apply or skip |
| `conflict` | Desired and current both changed since last successful baseline. | guided sync |
| `opaque-changed` | Opaque artifact hash/metadata differs; readable diff unavailable. | confirm save/apply |
| `blocked-lifecycle` | Target is running or unavailable in a forbidden state. | quit/retry/skip |
| `blocked-safety` | Secret, trust, policy, selector, or recipe rule blocks action. | inspect/fix |
| `unsupported` | No trusted recipe/capability supports the operation. | skip/create recipe |
| `unknown` | State cannot be determined safely. | inspect/verbose |

### Derivation order

1. If recipe validation, trust policy, or selector validation fails, return
   `blocked-safety` before reading current state.
2. If lifecycle forbids the requested read/write while a target is running,
   return `blocked-lifecycle`.
3. If no desired artifact exists for a selected setting, return
   `missing-desired`.
4. If desired exists and current does not, return `missing-current`.
5. If current equals desired, return `unchanged`, even if no ledger exists.
6. If current differs from desired and no last-applied baseline exists, render a
   no-baseline condition:
   - `save` context: `changed-current`;
   - `apply` context: `ready-to-apply`;
   - command-neutral `status`: differs with no previous sync baseline, allowing
     `save`, `apply`, and `diff` when safe.
7. If last-applied equals desired and current differs, return
   `changed-current`.
8. If last-applied equals current and desired differs, return
   `ready-to-apply`.
9. If desired and current both differ from last-applied, return `conflict`.
10. If recipe, driver, or normalizer version changed since last-applied, include
    `needs-recheck` warning and recompute normalized hashes before writes.

### Target-level severity

A target's status is the highest-severity item state in this order:

```text
blocked-safety
blocked-lifecycle
unsupported
conflict
opaque-changed
changed-current
ready-to-apply
missing-desired
missing-current
unknown
unchanged
```

### Guided sync

`sync` presents choices for differences. It may offer save, apply, skip, and
inspect/diff. It must not perform blind automatic two-way merge.

### Opaque state

Opaque artifacts may compare by hash and metadata only. If readable diff is
unavailable, output must say so and require confirmation before save/apply.

## Derived schema boundaries, not final schemas

This spec owns the canonical state-code enum and target aggregation rules.

Persisted or emitted objects:

| Object | Owned here? | Notes |
| --- | --- | --- |
| Item state code | yes | Final enum deferred to JSON schema. |
| Target status aggregation | yes | Severity order defined here. |
| Normalized hash | partial | Hash format owned by driver/ledger specs. |
| Ledger baseline | partial | Ledger schema owned by mutation spec. |
| JSON item result | partial | CLI schema owns envelope. |

## Examples

### Unchanged without ledger

```text
desired = leon@example.com
current = leon@example.com
last-applied = absent
state = unchanged
```

### Ready to apply

```text
desired = leon@example.com
current = old@example.com
last-applied = old@example.com
state = ready-to-apply
```

### Conflict

```text
desired = repo@example.com
current = local@example.com
last-applied = old@example.com
state = conflict
```

### Command-neutral no-baseline status

```text
Changed, no previous sync baseline:
  git:user.email    save / apply / diff
```

## Errors, blockers, and partial-result behavior

Status must distinguish:

- validation blockers;
- lifecycle blockers;
- unsupported operations;
- unknown state;
- conflicts;
- opaque changed state.

Partial status output must show per-item state and not collapse uncertainty into
only a target-level summary.

## Acceptance expectations

- Tests cover every canonical state.
- Tests cover target-level severity ordering.
- Tests cover no-baseline behavior for `status`, `save`, and `apply` contexts.
- Tests cover opaque metadata-only diff.
- Tests cover version-change `needs-recheck` warnings.
- Tests prove `sync` is guided and not automatic merge.

## Out of scope

- final renderer layout;
- final normalized hash algorithm;
- semantic merge for structured settings;
- cross-machine merge.

## Spec follow-ups / open decisions

- Decide final command-neutral status text for no-baseline differences.
- Decide exact `needs-recheck` JSON shape.
- Decide whether target aggregation should expose secondary states in summary.
