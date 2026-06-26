---
title: v2 MVP release-candidate evidence
status: historical-superseded
owner: Engineering Operations
last_reviewed: 2026-06-25
audited_commit: 1849bdab9ae8ad41cbdc7cc2ef00224cba5c6824
---

# v2 MVP release-candidate evidence

> Historical note: this #124 evidence was superseded by
> [`v2-release-candidate-issue-192.md`](./v2-release-candidate-issue-192.md),
> which re-verified the release-candidate surface after the #189/#190/#191
> production-readiness corrections. Keep this file as historical evidence only.
>
> Current reset note: #213/#226 supersede any release-candidate wording in this
> file that treated v1 migration, v1 compatibility, or backup/restore as part
> of the active v2 product promise. The current v2 public surface is
> `status -> diff -> sync`; retained v1 commands are hidden compatibility only.


This document records the release-candidate audit for GitHub issue
[#124](https://github.com/shpoont/dotfiles-manager/issues/124), "v2: mark MVP
release candidate after production-readiness gates pass".

## Verdict

**Release candidate for supported non-native v2 targets.**

This is a scoped verdict. It does **not** claim that every originally discussed
native export/import ambition is implemented, and it does **not** claim that v2
is a general app backup system, secret manager, package manager, public recipe
marketplace, or arbitrary script runner.

The supported release-candidate surface is ready for real user use when users
stay inside the documented supported targets, preview before writes, and treat
desired artifacts/backups as potentially sensitive.

## MVP scope decision

This release-candidate audit scopes the v2 MVP to the supported public local
settings surface:

- bundled selected values;
- bundled whole-file resources;
- bundled file-tree resources;
- safe custom local recipe authoring and fixture-only test flow;
- v1 compatibility and migration where documented;
- backups, restore, ledger/report evidence, dry-run safety, and redaction
  behavior for supported drivers.

Native export/import is excluded from this MVP release-candidate verdict because
[#113](https://github.com/shpoont/dotfiles-manager/issues/113) remains
open/blocked: [#112](https://github.com/shpoont/dotfiles-manager/issues/112)
selected no reviewed native export/import target. Native export/import is not
documented as a general public workflow in the user docs and must not be implied
by this release candidate.

If maintainers decide native export/import is mandatory for MVP despite this
scope decision, the verdict changes to:

> Not release-candidate ready: native export/import remains mandatory and #113
> is blocked.

## Release promise

For the scoped release candidate, a real user can safely use the manager to:

- initialize a v2 repository;
- inspect bundled recipes with `recipe list`, `recipe discover`, and
  `recipe explain`;
- select documented bundled targets with `add`;
- run `status`, `diff`, `save --dry-run`, `save --yes`, `apply --dry-run`, and
  `apply --yes` for supported settings;
- store selected desired values and artifacts in documented `desired/...`
  locations;
- get local backups and ledger records for supported live writes;
- inspect backups with `backup list` and `backup show`;
- preview and execute restore for supported backup payloads;
- use documented supported targets only.

Users are expected to follow the documented dry-run-first workflow; first-time
users should start with the safe temporary-HOME quickstart before applying to
real home configuration.

The scoped release candidate does not promise:

- native export/import;
- account or cloud application export;
- plugin or package manager actions;
- arbitrary script execution;
- public recipe marketplace behavior;
- secret-manager behavior;
- public `custom.files` live adoption beyond documented/internal gate status;
- broad app runtime control such as starting, stopping, restarting, or reloading
  applications unless a future reviewed recipe explicitly supports it.

## Evidence freshness

| Field | Evidence |
| --- | --- |
| Audited commit | `1849bdab9ae8ad41cbdc7cc2ef00224cba5c6824` on `main` |
| Clean local checkout | verified before starting #124 |
| Latest user-doc release tranche | [#123](https://github.com/shpoont/dotfiles-manager/issues/123), [PR #163](https://github.com/shpoont/dotfiles-manager/pull/163), merge commit `1849bdab9ae8ad41cbdc7cc2ef00224cba5c6824` |
| Latest dogfood gate | [#122](https://github.com/shpoont/dotfiles-manager/issues/122), [internal evidence](./v2-issue-122-dogfood-release-gate.md), merge commit `715b3fc55396fdb337f901958615319e36156dd7` |
| Harbor/evaluation setup | [#121](https://github.com/shpoont/dotfiles-manager/issues/121), [`evals/harbor/README.md`](../../../evals/harbor/README.md), `.systems-mapping/` working record |
| Local validation for this audit | `go test ./...` and `git diff --check` must pass before PR |
| PR validation for this audit | [PR #164](https://github.com/shpoont/dotfiles-manager/pull/164) must pass GitHub Actions before #124 is marked Done |
| Merge validation for this audit | after squash merge, #124 closure must cite the merge commit and green CI run |

The audited product baseline is `1849bdab9ae8ad41cbdc7cc2ef00224cba5c6824`;
the #124 documentation/audit commit is recorded in PR #164, and the final
squash-merge commit must be recorded in the issue closure after CI passes.

## Gate matrix

| Gate | Status | Evidence | Decision |
| --- | --- | --- | --- |
| v1 parity / compatibility decision for current dotfile use cases | Passed for scoped MVP through documented v1 compatibility plus v2 supported-target workflows | [#125](https://github.com/shpoont/dotfiles-manager/issues/125), [`docs/user/commands.md`](../../user/commands.md) migration section, `internal/v2/migration`, `internal/app/cli_migrate_test.go` | v1 file-sync behavior remains available as legacy compatibility; v2 provides the supported local-settings workflow for documented targets. This RC does not claim every v1 behavior was reimplemented as a v2 recipe. |
| `custom.files` internal/generated vertical slice | Passed for internal/generated path; public arbitrary adoption deferred | [#122](https://github.com/shpoont/dotfiles-manager/issues/122) custom.files internal dogfood, [`v2-issue-122-dogfood-release-gate.md`](./v2-issue-122-dogfood-release-gate.md), `internal/v2/customfiles`, `internal/v2/dogfood` | Internal generated file/tree apply/restore is evidenced. Public CLI `custom.files` live adoption is not part of this RC promise. |
| Dry-run trustworthiness | Passed for supported drivers | [#94](https://github.com/shpoont/dotfiles-manager/issues/94), [#115](https://github.com/shpoont/dotfiles-manager/issues/115), `internal/v2/selectedpreview`, `internal/v2/preview`, `internal/app/cli_selectedpreview_test.go` | Dry-run-first workflow is required in user docs. |
| Live apply backs up, verifies, and records ledgers | Passed for supported drivers | [#96](https://github.com/shpoont/dotfiles-manager/issues/96), [#115](https://github.com/shpoont/dotfiles-manager/issues/115), `internal/v2/ledger`, `internal/app/cli_selectedpreview_test.go`, [#122 dogfood evidence](./v2-issue-122-dogfood-release-gate.md) | Supported live writes are evidence-recorded and backup-aware. |
| Restore works for supported drivers | Passed for supported drivers | [#115](https://github.com/shpoont/dotfiles-manager/issues/115), `internal/v2/ledger/restore.go`, `internal/app/cli_backup_restore_test.go`, [#122 restore scenarios](./v2-issue-122-dogfood-release-gate.md) | Restore is in the release promise for supported backup payloads. |
| Secret, redaction, and trust tests pass | Passed for scoped MVP | [#90](https://github.com/shpoont/dotfiles-manager/issues/90), [#95](https://github.com/shpoont/dotfiles-manager/issues/95), [#114](https://github.com/shpoont/dotfiles-manager/issues/114), `internal/v2/secretpolicy`, `internal/v2/contentsafety`, `internal/v2/recipe/write_safety_test.go` | Docs warn desired artifacts/backups can contain managed bytes and are not secret-safe by default. |
| Unsupported state fails visibly | Passed for scoped MVP | [#102](https://github.com/shpoont/dotfiles-manager/issues/102), [#103](https://github.com/shpoont/dotfiles-manager/issues/103), [#104](https://github.com/shpoont/dotfiles-manager/issues/104), [#105](https://github.com/shpoont/dotfiles-manager/issues/105), [#122 missing-state scenario](./v2-issue-122-dogfood-release-gate.md) | Unsupported and unsafe cases are documented as blockers, exclusions, warnings, or non-goals. |
| CI passes on clean checkout | Passed for latest merged docs tranche; required for #124 PR | [PR #163 checks](https://github.com/shpoont/dotfiles-manager/pull/163), [`ci-cd.md`](./ci-cd.md) | #124 cannot be marked Done until its own PR CI is green. |
| Dogfood succeeds on non-critical profile/machine | Passed for implemented safe dogfood surface | [#122](https://github.com/shpoont/dotfiles-manager/issues/122), [`v2-issue-122-dogfood-release-gate.md`](./v2-issue-122-dogfood-release-gate.md) | Dogfood supports the scoped non-native RC verdict. |
| CLI text and JSON snapshots | Passed for scoped MVP commands | [#117](https://github.com/shpoont/dotfiles-manager/issues/117), `internal/app/cli_init_list_test.go`, `internal/app/cli_recipe_test.go`, `internal/app/cli_selectedpreview_test.go`, `internal/tests/contract` | User-facing command behavior is covered by tests. |
| Profile/scope resolution | Passed | [#107](https://github.com/shpoont/dotfiles-manager/issues/107), [#117](https://github.com/shpoont/dotfiles-manager/issues/117), `internal/v2/resolution/resolver_test.go`, `internal/v2/addtarget/addtarget_test.go` | All four scopes and extra profile layers are part of documented behavior. |
| Identity bootstrap | Passed | [#117](https://github.com/shpoont/dotfiles-manager/issues/117), `internal/v2/initcmd`, `internal/app/cli_init_list_test.go` | Local identity state is documented under platform-specific state roots. |
| Status and canonical state reporting | Passed | [#94](https://github.com/shpoont/dotfiles-manager/issues/94), [#116](https://github.com/shpoont/dotfiles-manager/issues/116), `internal/v2/status`, `internal/v2/guidedsync`, `internal/app/cli_selectedpreview_test.go` | Status/diff/reporting are included in supported workflow. |
| File driver | Passed | [#142](https://github.com/shpoont/dotfiles-manager/issues/142), [#102](https://github.com/shpoont/dotfiles-manager/issues/102), [#104](https://github.com/shpoont/dotfiles-manager/issues/104), [#105](https://github.com/shpoont/dotfiles-manager/issues/105), `internal/v2/filedriver` | Whole-file Zsh/tmux/SSH resources are supported with documented exclusions. |
| File-tree driver | Passed | [#103](https://github.com/shpoont/dotfiles-manager/issues/103), [#142](https://github.com/shpoont/dotfiles-manager/issues/142), `internal/v2/filetreedriver` | Neovim file-tree support is in scoped release promise. |
| Structured selected-value drivers | Passed | [#84](https://github.com/shpoont/dotfiles-manager/issues/84), [#86](https://github.com/shpoont/dotfiles-manager/issues/86), [#92](https://github.com/shpoont/dotfiles-manager/issues/92), [#98](https://github.com/shpoont/dotfiles-manager/issues/98), [#99](https://github.com/shpoont/dotfiles-manager/issues/99), `internal/v2/inidriver`, `jsondriver`, `yamldriver`, `tomldriver`, `plistdriver` | Git and Starship selected values are supported; other structured drivers exist behind recipes/local authoring. |
| Plist/defaults | Passed for implemented scope | [#99](https://github.com/shpoont/dotfiles-manager/issues/99), [#100](https://github.com/shpoont/dotfiles-manager/issues/100), `internal/v2/plistdriver`, `internal/v2/macosdefaultsdriver` | Defaults writes are not promised as a bundled public target in this RC. |
| Platform/filesystem behavior | Passed through tests and CI lanes | [`testing-strategy.md`](./testing-strategy.md), [`ci-cd.md`](./ci-cd.md), `internal/v2/ledger/store_test.go`, `internal/tests/integration` | macOS/Linux paths and unsupported OS behavior are covered by tests/CI policy. |
| Lifecycle policy | Passed for implemented lifecycle engine and recipe warnings | [#111](https://github.com/shpoont/dotfiles-manager/issues/111), `internal/v2/lifecycle`, recipe explain tests, bundled target docs | The scoped RC does not promise app restart/reload control. |
| Native export/import | Deferred / excluded from scoped MVP RC | [#108](https://github.com/shpoont/dotfiles-manager/issues/108), [#109](https://github.com/shpoont/dotfiles-manager/issues/109), [#110](https://github.com/shpoont/dotfiles-manager/issues/110), [#111](https://github.com/shpoont/dotfiles-manager/issues/111), [#112](https://github.com/shpoont/dotfiles-manager/issues/112), [#113](https://github.com/shpoont/dotfiles-manager/issues/113), [`docs/user/faq.md`](../../user/faq.md) | Generic machinery exists, but no first verified native recipe is shipped. #113 remains open/blocked; native support is not part of this scoped non-native RC verdict. |
| Ledger | Passed | [#96](https://github.com/shpoont/dotfiles-manager/issues/96), [#115](https://github.com/shpoont/dotfiles-manager/issues/115), `internal/v2/ledger/store_test.go`, `internal/v2/ledger/restore_test.go` | Ledger evidence is part of supported apply/restore workflow. |
| Migration | Passed for documented generated-output path | [#125](https://github.com/shpoont/dotfiles-manager/issues/125), `internal/v2/migration`, `internal/app/cli_migrate_test.go`, [`docs/user/faq.md`](../../user/faq.md) | Migration generates reviewable output and does not replace active root files automatically. |
| Trust and local recipe authoring | Passed for scoped local authoring | [#118](https://github.com/shpoont/dotfiles-manager/issues/118), [#119](https://github.com/shpoont/dotfiles-manager/issues/119), [#120](https://github.com/shpoont/dotfiles-manager/issues/120), [#114](https://github.com/shpoont/dotfiles-manager/issues/114), `internal/v2/appauthor`, `recipes/local/<target>/fixtures/roundtrip/...` docs | Local authoring is fixture/test oriented and does not imply a marketplace or arbitrary trusted writes. |
| Harbor agent-eval cases | Present as evaluation aids | [#121](https://github.com/shpoont/dotfiles-manager/issues/121), [`evals/harbor/cases/`](../../../evals/harbor/cases/) | Harbor supports qualitative review but does not replace deterministic tests/CI. |
| User-facing install, recovery, and limitations docs | Passed | [#123](https://github.com/shpoont/dotfiles-manager/issues/123), [`../../user/install-and-release.md`](../../user/install-and-release.md), [`../../user/getting-started.md`](../../user/getting-started.md), [`../../user/configuration.md`](../../user/configuration.md), [`../../user/faq.md`](../../user/faq.md) | Users can install/verify, run the safe first workflow, understand stored data, and recover after apply. |

## Known limitations

The limitations below are intentional for this scoped release candidate:

- Native export/import is not a public supported workflow until #113 is
  unblocked and a reviewed first target ships.
- Desired artifacts and backup payloads can contain actual managed bytes; users
  must not treat a settings repository as secret-safe by default.
- Secrets, credentials, private keys, account exports, caches, generated state,
  and runtime state remain out of scope unless a reviewed recipe explicitly
  supports them.
- `custom.files` is not promised as broad public live adoption; it is covered
  for generated/internal dogfood and must remain explicit and safety-bounded.
- The manager does not install apps, plugins, packages, shell frameworks, or
  editor plugins.
- The manager does not restart, reload, or control applications as a general
  behavior. Recipes may warn or block based on lifecycle policy.
- Restore for selected values backed by files can roll back the whole backing
  file; it is not a semantic single-value rollback.
- Published binaries can lag current docs; users must verify v2 command help
  before following v2 workflows.

## Final release-candidate checklist for #124

Before #124 is marked Done:

- [ ] Pro post-validation approves this audit wording.
- [ ] `go test ./...` passes on the #124 branch.
- [ ] `git diff --check` passes.
- [ ] Markdown local-link check for changed docs passes.
- [ ] Overclaim grep finds no unqualified production-ready/native/#113-complete claims.
- [ ] The #124 PR passes GitHub Actions.
- [ ] The #124 PR is squash-merged.
- [ ] The #124 issue closure cites the merge commit and green CI evidence.
- [ ] #113 remains open/blocked unless a separate explicit native-recipe
      decision changes its status.
