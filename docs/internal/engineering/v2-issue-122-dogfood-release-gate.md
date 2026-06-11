---
title: v2 issue 122 dogfood release gate evidence
status: evidence
owner: engineering
last_reviewed: 2026-06-11
---

# v2 issue 122 dogfood release gate evidence

This document records the sanitized evidence for GitHub issue #122,
"v2: run end-to-end dogfood release gate on non-critical real targets".

Raw command logs, temp repos, desired artifacts, live fixtures, backups, and
ledgers were intentionally left under ignored `.tmp/dogfood-122` and are not
committed.

## Result

| Field | Value |
| --- | --- |
| Issue | #122 |
| Run status | `passed` |
| Run id | `20260611T171126Z` |
| Worktree root | `/Users/shpoont/Work/shpoont/dotfiles-manager` |
| Ignored run root | `/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z` |
| Scenario count | `7` |
| Passed scenarios | `7` |
| Pro pre-execution validation | `approved-with-changes` |

Overall result: **passed** for the implemented safe dogfood surface.

## Safety boundary

- All public CLI live mutations used per-command temp `HOME` and
  `XDG_CONFIG_HOME` values under `.tmp/dogfood-122/<run-id>`.
- The CLI binary was built once under the real developer home and then run
  with explicit per-command environment overrides for app/config scenarios.
- Every `save --yes`, `apply --yes`, and `restore --yes` was preceded by a
  matching dry-run whose JSON was checked for the expected setting ref and
  paths under the scenario temp root.
- Raw command logs and generated payloads were not committed.
- Native export/import was not exercised because #113 remains blocked by the
  lack of a reviewed native target from #112.

## Scenario matrix

| Scenario | Target / setting | Coverage | Status | Apply run | Restore run | Desired artifact | Notes |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `git` | `git:user.email` | public CLI dogfood | `passed` | `selected-value-20260611T171129.872750000Z` | `restore-20260611T171129.995063000Z` | `desired/user/leon/targets/git/settings.yaml` | restore passed |
| `starship` | `starship:add_newline` | public CLI dogfood | `passed` | `selected-value-20260611T171130.275807000Z` | `` | `desired/user/leon/targets/starship/settings.yaml` | restore not-run |
| `tmux` | `tmux:home.conf` | public CLI dogfood | `passed` | `selected-value-20260611T171131.016360000Z` | `restore-20260611T171131.433131000Z` | `desired/user/leon/targets/tmux/artifacts/home.conf` | restore passed |
| `nvim` | `nvim:config` | public CLI dogfood | `passed` | `selected-value-20260611T171131.914577000Z` | `restore-20260611T171132.136413000Z` | `desired/user/leon/targets/nvim/artifacts/config` | restore passed; cache exclude verified |
| `app-roundtrip` | `local-issue122-roundtrip` | public CLI dogfood | `passed` | `` | `` | `` | fixture inputs unchanged |
| `missing-state` | `tmux:home.conf` | public CLI dogfood | `passed` | `` | `` | `` | missing desired apply blocked; missing live save blocked |
| `custom.files-internal-dogfood` | `custom.files` | internal dogfood readiness gate | `passed` | `` | `` | `` | public CLI live-write adoption not-covered |

## Backup refs

| Scenario | Backup refs |
| --- | --- |
| `git` | `state://backups/selected-value-20260611T171129.872750000Z/git_user.email-user-email` |
| `starship` | `state://backups/selected-value-20260611T171130.275807000Z/starship_add_newline-add_newline` |
| `tmux` | `state://backups/selected-value-20260611T171131.016360000Z/tmux_home.conf-home.conf` |
| `nvim` | `state://backups/selected-value-20260611T171131.914577000Z/nvim_config-config` |
| `app-roundtrip` | n/a |
| `missing-state` | n/a |
| `custom.files-internal-dogfood` | n/a |

## Known gap / coverage classification

`custom.files` is covered here by the existing internal dogfood readiness gate:
generated `custom.files` single-file and file-tree resources, explicit temp
allowed roots, apply/restore, backup, ledger, and redaction checks.

This run does **not** claim that public CLI `custom.files` live-write adoption
passed. That surface remains deferred until a user-facing local trust/adoption
flow is promoted.

| Surface | Status | Reason |
| --- | --- | --- |
| `custom.files public CLI live-write adoption` | `not-covered` | Public CLI custom.files live writes are not promoted; local recipe trust/adoption has no user-facing trust command in this tranche. Covered instead by internal dogfood readiness gate with explicit temp allowed roots. |

## Redaction and path-safety checks

- Redaction checks passed: `43`.
- Path-safety checks passed: `276`.
- A separate grep over the retained ignored logs and sanitized evidence found
  no fixture sentinel values in normal stdout/stderr logs or evidence JSON.
- Ledger JSONL, run records, and backup metadata were scanned for fixture
  sentinel labels in the public CLI scenarios and passed.

## Expected non-zero exits

The following commands intentionally exited non-zero as part of fail-closed
missing-state validation and are counted as passed scenario evidence:

- #43 `missing desired apply yes`: exited `5`; expected because apply must block
  when the desired artifact is missing.
- #45 `missing live save yes`: exited `5`; expected because save must block when
  the live config file is missing and must not delete/tombstone desired state.

## Additional validation

After the dogfood run, the full Go test suite was run from the repository root:

```sh
(cd /Users/shpoont/Work/shpoont/dotfiles-manager && /Users/shpoont/.asdf/shims/go test ./...)
```

Exit status: `0`.

## Exact commands run

The following table records the exact command lines and exit statuses. The
`stdoutLog` and `stderrLog` paths are relative to the ignored run root and are
not committed.

| # | Label | Exit | Command | stdoutLog | stderrLog |
| ---: | --- | ---: | --- | --- | --- |
| 1 | build dogfood binary | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager && /Users/shpoont/.asdf/shims/go build -o /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager ./cmd/dotfiles-manager)` | `logs/build_dogfood_binary.stdout` | `logs/build_dogfood_binary.stderr` |
| 2 | git status | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/git/repo && HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/git/home' XDG_CONFIG_HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/git/home/.config' /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager status --json --user-id leon git:user.email)` | `logs/git_status.stdout` | `logs/git_status.stderr` |
| 3 | git diff | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/git/repo && HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/git/home' XDG_CONFIG_HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/git/home/.config' /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager diff --json --user-id leon git:user.email)` | `logs/git_diff.stdout` | `logs/git_diff.stderr` |
| 4 | git save dry-run | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/git/repo && HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/git/home' XDG_CONFIG_HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/git/home/.config' /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager save --dry-run --json --user-id leon git:user.email)` | `logs/git_save_dry-run.stdout` | `logs/git_save_dry-run.stderr` |
| 5 | git save yes | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/git/repo && HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/git/home' XDG_CONFIG_HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/git/home/.config' /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager save --yes --json --user-id leon git:user.email)` | `logs/git_save_yes.stdout` | `logs/git_save_yes.stderr` |
| 6 | git diff after drift | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/git/repo && HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/git/home' XDG_CONFIG_HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/git/home/.config' /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager diff --json --user-id leon git:user.email)` | `logs/git_diff_after_drift.stdout` | `logs/git_diff_after_drift.stderr` |
| 7 | git apply dry-run | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/git/repo && HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/git/home' XDG_CONFIG_HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/git/home/.config' /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager apply --dry-run --json --user-id leon git:user.email)` | `logs/git_apply_dry-run.stdout` | `logs/git_apply_dry-run.stderr` |
| 8 | git apply yes | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/git/repo && HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/git/home' XDG_CONFIG_HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/git/home/.config' /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager apply --yes --json --user-id leon git:user.email)` | `logs/git_apply_yes.stdout` | `logs/git_apply_yes.stderr` |
| 9 | git backup show | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/git/repo && HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/git/home' XDG_CONFIG_HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/git/home/.config' /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager backup show selected-value-20260611T171129.872750000Z --json)` | `logs/git_backup_show.stdout` | `logs/git_backup_show.stderr` |
| 10 | git restore dry-run | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/git/repo && HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/git/home' XDG_CONFIG_HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/git/home/.config' /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager restore selected-value-20260611T171129.872750000Z --dry-run --json --user-id leon)` | `logs/git_restore_dry-run.stdout` | `logs/git_restore_dry-run.stderr` |
| 11 | git restore yes | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/git/repo && HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/git/home' XDG_CONFIG_HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/git/home/.config' /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager restore selected-value-20260611T171129.872750000Z --yes --json --user-id leon)` | `logs/git_restore_yes.stdout` | `logs/git_restore_yes.stderr` |
| 12 | starship status | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/starship/repo && HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/starship/home' XDG_CONFIG_HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/starship/home/.config' /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager status --json --user-id leon starship:add_newline)` | `logs/starship_status.stdout` | `logs/starship_status.stderr` |
| 13 | starship diff | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/starship/repo && HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/starship/home' XDG_CONFIG_HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/starship/home/.config' /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager diff --json --user-id leon starship:add_newline)` | `logs/starship_diff.stdout` | `logs/starship_diff.stderr` |
| 14 | starship save dry-run | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/starship/repo && HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/starship/home' XDG_CONFIG_HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/starship/home/.config' /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager save --dry-run --json --user-id leon starship:add_newline)` | `logs/starship_save_dry-run.stdout` | `logs/starship_save_dry-run.stderr` |
| 15 | starship save yes | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/starship/repo && HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/starship/home' XDG_CONFIG_HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/starship/home/.config' /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager save --yes --json --user-id leon starship:add_newline)` | `logs/starship_save_yes.stdout` | `logs/starship_save_yes.stderr` |
| 16 | starship diff after drift | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/starship/repo && HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/starship/home' XDG_CONFIG_HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/starship/home/.config' /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager diff --json --user-id leon starship:add_newline)` | `logs/starship_diff_after_drift.stdout` | `logs/starship_diff_after_drift.stderr` |
| 17 | starship apply dry-run | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/starship/repo && HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/starship/home' XDG_CONFIG_HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/starship/home/.config' /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager apply --dry-run --json --user-id leon starship:add_newline)` | `logs/starship_apply_dry-run.stdout` | `logs/starship_apply_dry-run.stderr` |
| 18 | starship apply yes | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/starship/repo && HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/starship/home' XDG_CONFIG_HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/starship/home/.config' /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager apply --yes --json --user-id leon starship:add_newline)` | `logs/starship_apply_yes.stdout` | `logs/starship_apply_yes.stderr` |
| 19 | tmux status | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/tmux/repo && HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/tmux/home' XDG_CONFIG_HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/tmux/home/.config' /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager status --json --user-id leon tmux:home.conf)` | `logs/tmux_status.stdout` | `logs/tmux_status.stderr` |
| 20 | tmux diff | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/tmux/repo && HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/tmux/home' XDG_CONFIG_HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/tmux/home/.config' /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager diff --json --user-id leon tmux:home.conf)` | `logs/tmux_diff.stdout` | `logs/tmux_diff.stderr` |
| 21 | tmux save dry-run | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/tmux/repo && HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/tmux/home' XDG_CONFIG_HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/tmux/home/.config' /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager save --dry-run --json --user-id leon tmux:home.conf)` | `logs/tmux_save_dry-run.stdout` | `logs/tmux_save_dry-run.stderr` |
| 22 | tmux save yes | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/tmux/repo && HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/tmux/home' XDG_CONFIG_HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/tmux/home/.config' /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager save --yes --json --user-id leon tmux:home.conf)` | `logs/tmux_save_yes.stdout` | `logs/tmux_save_yes.stderr` |
| 23 | tmux diff after drift | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/tmux/repo && HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/tmux/home' XDG_CONFIG_HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/tmux/home/.config' /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager diff --json --user-id leon tmux:home.conf)` | `logs/tmux_diff_after_drift.stdout` | `logs/tmux_diff_after_drift.stderr` |
| 24 | tmux apply dry-run | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/tmux/repo && HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/tmux/home' XDG_CONFIG_HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/tmux/home/.config' /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager apply --dry-run --json --user-id leon tmux:home.conf)` | `logs/tmux_apply_dry-run.stdout` | `logs/tmux_apply_dry-run.stderr` |
| 25 | tmux apply yes | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/tmux/repo && HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/tmux/home' XDG_CONFIG_HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/tmux/home/.config' /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager apply --yes --json --user-id leon tmux:home.conf)` | `logs/tmux_apply_yes.stdout` | `logs/tmux_apply_yes.stderr` |
| 26 | tmux backup show | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/tmux/repo && HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/tmux/home' XDG_CONFIG_HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/tmux/home/.config' /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager backup show selected-value-20260611T171131.016360000Z --json)` | `logs/tmux_backup_show.stdout` | `logs/tmux_backup_show.stderr` |
| 27 | tmux restore dry-run | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/tmux/repo && HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/tmux/home' XDG_CONFIG_HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/tmux/home/.config' /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager restore selected-value-20260611T171131.016360000Z --dry-run --json --user-id leon)` | `logs/tmux_restore_dry-run.stdout` | `logs/tmux_restore_dry-run.stderr` |
| 28 | tmux restore yes | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/tmux/repo && HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/tmux/home' XDG_CONFIG_HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/tmux/home/.config' /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager restore selected-value-20260611T171131.016360000Z --yes --json --user-id leon)` | `logs/tmux_restore_yes.stdout` | `logs/tmux_restore_yes.stderr` |
| 29 | nvim status | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/nvim/repo && HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/nvim/home' XDG_CONFIG_HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/nvim/home/.config' /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager status --json --user-id leon nvim:config)` | `logs/nvim_status.stdout` | `logs/nvim_status.stderr` |
| 30 | nvim diff | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/nvim/repo && HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/nvim/home' XDG_CONFIG_HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/nvim/home/.config' /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager diff --json --user-id leon nvim:config)` | `logs/nvim_diff.stdout` | `logs/nvim_diff.stderr` |
| 31 | nvim save dry-run | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/nvim/repo && HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/nvim/home' XDG_CONFIG_HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/nvim/home/.config' /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager save --dry-run --json --user-id leon nvim:config)` | `logs/nvim_save_dry-run.stdout` | `logs/nvim_save_dry-run.stderr` |
| 32 | nvim save yes | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/nvim/repo && HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/nvim/home' XDG_CONFIG_HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/nvim/home/.config' /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager save --yes --json --user-id leon nvim:config)` | `logs/nvim_save_yes.stdout` | `logs/nvim_save_yes.stderr` |
| 33 | nvim diff after drift | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/nvim/repo && HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/nvim/home' XDG_CONFIG_HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/nvim/home/.config' /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager diff --json --user-id leon nvim:config)` | `logs/nvim_diff_after_drift.stdout` | `logs/nvim_diff_after_drift.stderr` |
| 34 | nvim apply dry-run | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/nvim/repo && HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/nvim/home' XDG_CONFIG_HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/nvim/home/.config' /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager apply --dry-run --json --user-id leon nvim:config)` | `logs/nvim_apply_dry-run.stdout` | `logs/nvim_apply_dry-run.stderr` |
| 35 | nvim apply yes | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/nvim/repo && HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/nvim/home' XDG_CONFIG_HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/nvim/home/.config' /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager apply --yes --json --user-id leon nvim:config)` | `logs/nvim_apply_yes.stdout` | `logs/nvim_apply_yes.stderr` |
| 36 | nvim backup show | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/nvim/repo && HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/nvim/home' XDG_CONFIG_HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/nvim/home/.config' /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager backup show selected-value-20260611T171131.914577000Z --json)` | `logs/nvim_backup_show.stdout` | `logs/nvim_backup_show.stderr` |
| 37 | nvim restore dry-run | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/nvim/repo && HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/nvim/home' XDG_CONFIG_HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/nvim/home/.config' /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager restore selected-value-20260611T171131.914577000Z --dry-run --json --user-id leon)` | `logs/nvim_restore_dry-run.stdout` | `logs/nvim_restore_dry-run.stderr` |
| 38 | nvim restore yes | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/nvim/repo && HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/nvim/home' XDG_CONFIG_HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/nvim/home/.config' /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager restore selected-value-20260611T171131.914577000Z --yes --json --user-id leon)` | `logs/nvim_restore_yes.stdout` | `logs/nvim_restore_yes.stderr` |
| 39 | app roundtrip create | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/app-roundtrip/repo && /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager app create local-issue122-roundtrip --template file --from-path .config/issue122/config.yaml --setting config --setting-label 'Issue 122 config' --scope-default user --lifecycle allowed --json)` | `logs/app_roundtrip_create.stdout` | `logs/app_roundtrip_create.stderr` |
| 40 | app roundtrip validate | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/app-roundtrip/repo && /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager app validate local-issue122-roundtrip --json)` | `logs/app_roundtrip_validate.stdout` | `logs/app_roundtrip_validate.stderr` |
| 41 | app roundtrip test | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/app-roundtrip/repo && /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager app test local-issue122-roundtrip --roundtrip --fixture basic --json)` | `logs/app_roundtrip_test.stdout` | `logs/app_roundtrip_test.stderr` |
| 42 | missing desired apply dry-run | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/missing-state/repo && HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/missing-state/home' XDG_CONFIG_HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/missing-state/home/.config' /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager apply --dry-run --json --user-id leon tmux:home.conf)` | `logs/missing_desired_apply_dry-run.stdout` | `logs/missing_desired_apply_dry-run.stderr` |
| 43 | missing desired apply yes | `5` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/missing-state/repo && HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/missing-state/home' XDG_CONFIG_HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/missing-state/home/.config' /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager apply --yes --json --user-id leon tmux:home.conf)` | `logs/missing_desired_apply_yes.stdout` | `logs/missing_desired_apply_yes.stderr` |
| 44 | missing live save dry-run | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/missing-state/repo && HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/missing-state/home' XDG_CONFIG_HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/missing-state/home/.config' /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager save --dry-run --json --user-id leon tmux:home.conf)` | `logs/missing_live_save_dry-run.stdout` | `logs/missing_live_save_dry-run.stderr` |
| 45 | missing live save yes | `5` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/missing-state/repo && HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/missing-state/home' XDG_CONFIG_HOME='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/missing-state/home/.config' /Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/bin/dotfiles-manager save --yes --json --user-id leon tmux:home.conf)` | `logs/missing_live_save_yes.stdout` | `logs/missing_live_save_yes.stderr` |
| 46 | customfiles dogfood focused go test | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager && TMPDIR='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/go-tmp' /Users/shpoont/.asdf/shims/go test ./internal/v2/dogfood -run TestRunMigrationReadinessProvesGeneratedFileAndTreeApplyRestore -count=1)` | `logs/customfiles_dogfood_focused_go_test.stdout` | `logs/customfiles_dogfood_focused_go_test.stderr` |
| 47 | customfiles dogfood package go tests | `0` | `(cd /Users/shpoont/Work/shpoont/dotfiles-manager && TMPDIR='/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/dogfood-122/20260611T171126Z/go-tmp' /Users/shpoont/.asdf/shims/go test ./internal/v2/dogfood ./internal/v2/customfiles ./internal/v2/ledger ./internal/v2/selectedlive -count=1)` | `logs/customfiles_dogfood_package_go_tests.stdout` | `logs/customfiles_dogfood_package_go_tests.stderr` |

## Interpretation

The implemented v2 surface is safe enough for this issue's dogfood gate over
the exercised non-critical temp targets: bundled selected values, bundled
whole-file resources, bundled file-tree resources, custom app-author fixture
roundtrip, missing-state blockers, backup creation, restore, ledger evidence,
and internal generated `custom.files` apply/restore readiness.

Release readiness is still bounded by the explicit known gap above and by any
other open v2 issues that remain required before the product is called ready for
general use.
