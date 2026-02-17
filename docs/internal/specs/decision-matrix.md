---
owner: Core Engineering
status: Reference
last-updated: 2026-02-17
canonical-source: docs/internal/specs/decision-matrix.md
---

# dotfiles-manager: decision matrix (scenario outcomes)

This file is a **case/outcome reference** for behavior examples.

Canonical rules and rationale live in **`decisions.md`**.

## Terms (for matrix rows)

- **S** = `source` path state
- **T** = `target` path state
- **u+ / u-** = add-unmanaged include/exclude result for import candidates
- **r+ / r-** = deploy remove-unmanaged pattern match result
- **m+ / m-** = import remove-missing include/exclude result

## 1) Command scope matrix (per sync)

| Command | Direction | Base scope | Pattern sets used | Outcome focus |
|---|---|---|---|---|
| `status [--json] [path]` | compare | Manifest + candidates | add-unmanaged include/exclude, remove-unmanaged, remove-missing include/exclude | Reports drift + candidate sets. |
| `deploy [--dry-run] [--json] [path]` | S → T | Manifest paths | remove-unmanaged | Applies copy/remove behavior (or plans only with `--dry-run`). |
| `import [--dry-run] [--json] [path]` | T → S | Manifest paths | add-unmanaged include/exclude, remove-missing include/exclude | Applies import behavior (or plans only with `--dry-run`). |

## 2) `[path]` subset matrix

| Input `[path]` vs sync target | Sync selected? | Effective scope |
|---|---|---|
| exactly target | yes | whole sync |
| inside target subtree | yes | intersected subtree only |
| parent of target | no | n/a |
| unrelated | no | n/a |

If no syncs match: command errors.  
If multiple syncs match: all are processed.
When multiple selected syncs touch the same final path, later sync (config order) wins.

Examples:
- `target: .config/nvim` + `[path]=~/.config/nvim` → selected (full sync)
- `target: .config/nvim` + `[path]=~/.config/nvim/lua` → selected (subtree only)
- `target: .config/nvim` + `[path]=~/.config` → not selected

## 3) Per-path behavior matrix

| Scenario | Deploy | Import | Status |
|---|---|---|---|
| `S exists`, `T missing`, `m+` | copy S→T | delete S | missing-in-target + removable-missing |
| `S exists`, `T missing`, `m-` | copy S→T | keep S | missing-in-target |
| `S exists`, `T exists`, content/type differs | update/replace T from S | update/replace S from T | changed |
| `S missing`, `T exists`, `u+`, `r+` | remove T (after copy phase) | import T→S (new) | incoming-unmanaged + removable-unmanaged |
| `S missing`, `T exists`, `u+`, `r-` | keep T | import T→S (new) | incoming-unmanaged |
| `S missing`, `T exists`, `u-`, `r+` | remove T (after copy phase) | skip | extra-in-target + removable-unmanaged |
| `S missing`, `T exists`, `u-`, `r-` | keep T | skip | extra-in-target |
| both missing | no-op | no-op | no-op |

## 4) Test-oracle notes

- `source` is authoritative; there is no separate conflict state.
- Deploy removal order is copy/update first, remove second.
- Status should include candidate visibility (incoming unmanaged, removable unmanaged, removable missing).
- Status text/json actions use potential wording (`can create`, `can update`, `can replace type`, `can add`, `can remove`).
- Text output suppresses empty phase blocks; text summary omits zero-count categories.
- `--dry-run` for deploy/import uses the same scope and outcome planning, but performs no writes.
