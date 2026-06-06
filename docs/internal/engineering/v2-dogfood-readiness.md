---
title: v2 dogfood readiness flow
status: draft
owner: engineering
last_reviewed: 2026-06-06
---

# v2 dogfood readiness flow

This runbook defines the first safe dogfood path for generated v2 migration
output. It is an internal readiness flow, not a user-facing command and not a
release claim by itself.

The goal is to prove that the generated `custom.files` path can move through:

1. migration output generation;
2. parity verification;
3. preview-only apply planning;
4. confirmed apply against non-critical locations;
5. backup and ledger evidence;
6. preview-only restore planning;
7. confirmed restore;
8. restored-state verification.

## Automated safe fixture

The deterministic fixture lives in `internal/v2/dogfood`.

It uses temporary roots only:

- a temporary v1-style repository;
- a temporary dogfood home root;
- a temporary local state root;
- a `.gitconfig`-like file setting;
- a `.config/nvim`-like file-tree setting.

The fixture intentionally does not use real home-directory dotfiles, app
configs, shell secrets, SSH keys, browser state, password-manager state, or
Codex authentication material.

The internal runner requires every live target to pass two checks before any
confirmed apply:

- an explicit caller-provided location-root mapper must choose the dogfood live
  root;
- the resolved live target path must be contained within an explicit allowed
  live-root allowlist.

The runner never falls back to `~` or generated location defaults for dogfood
live writes.

## Required sequence

The dogfood readiness sequence is:

```text
write temp v1 fixture
dotfiles-manager migrate --json equivalent
dotfiles-manager migrate parity --run-dir <run-dir> --json equivalent
resolve generated v2 profile and custom.files recipe
custom.files apply dry-run against temp/non-critical roots
confirmed custom.files apply against temp/non-critical roots
verify backup metadata, run record, and ledger entry
custom.files restore dry-run from apply backup
confirmed custom.files restore from apply backup
verify backup-before-restore metadata, run record, ledger entry, and live state
```

The current implementation uses internal primitives directly instead of a public
CLI command because the v2 user-facing apply/restore command surface is not yet
promoted. This keeps the dogfood gate deterministic and prevents accidental
live writes.

## Failure handling

After any confirmed apply succeeds, the runner must treat the target as dirty
until restore succeeds.

If a later dogfood step fails after confirmed apply, the runner attempts a
best-effort confirmed restore from the apply backup using a separate recovery
run id. The report records whether recovery was attempted and whether it
verified.

If best-effort recovery fails, stop and report the original error plus the
recovery error. Do not hide the failure behind a fallback or mark the dogfood
flow successful.

## Future manual/live dogfood checklist

A future live dogfood run must be explicitly approved and must stay limited to
non-critical, restorable settings. Before live dogfood:

1. Choose a non-critical profile and machine.
2. Enumerate the exact live target paths.
3. Confirm no target contains secrets or user-critical app state.
4. Confirm every live target has a restore strategy.
5. Run migration preview first.
6. Run parity and require `status=ok`.
7. Run apply preview and inspect target paths, changes, and backup policy.
8. Confirm apply only for the listed non-critical targets.
9. Verify backup metadata, run record, and ledger entry.
10. Run restore preview from the apply backup.
11. Confirm restore.
12. Verify the live state returned to the pre-apply state.
13. Save only sanitized evidence in the repository or tracker.

## Out of scope

- optional promotion from `custom.files` to known targets;
- v1 command compatibility aliases;
- replacing v1 behavior;
- native app drivers;
- broad app reverse engineering;
- running Harbor cases as production release gates;
- default writes to real user-critical dotfiles or app configs.

Harbor and Systems Mapping/Evaluation remain internal process/evaluation aids
for judgment-heavy acceptance work. Deterministic dogfood behavior must stay
covered by normal automated tests.
