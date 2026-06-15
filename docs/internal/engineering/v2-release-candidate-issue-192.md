---
title: v2 release-candidate verification for issue 192
status: release-candidate-evidence
owner: Engineering Operations
last_reviewed: 2026-06-15
issue: https://github.com/shpoont/dotfiles-manager/issues/192
audited_commit: d0b705b0da15a1375a34bdf7e81fb1bd19e6b704
---

# v2 release-candidate verification for issue #192

This document records the non-publishing release-candidate verification for
GitHub issue [#192](https://github.com/shpoont/dotfiles-manager/issues/192),
"v2: verify release-candidate install, distribution, docs, and supported-surface
evidence".

Raw command logs, extracted archives, temporary homes, temporary repositories,
and generated desired/backup state were intentionally kept under ignored
`.tmp/issue-192/` and are not committed.

This #192 change also fixes a test-only repository scan so local ignored release
evidence roots such as `.tmp/issue-192/` and `dist/` do not make `go test ./...`
fail by scanning copied/generated source trees. That test hygiene fix does not
change production behavior.

## Verdict

**Release-candidate evidence passed for the current v2 local-settings-manager
surface, but no beta/public install is ready until an approved pre-release or
release is actually published and verified.**

| Use level | Verdict | Reason |
| --- | --- | --- |
| Manual source-build dogfood | **Passed** | A clean checkout of the audited commit passed `go test ./...`, source build, version/help checks, and safe temporary-home smoke checks. |
| Manual archive-binary dogfood | **Passed** | GoReleaser snapshot archives were built without publishing, checksums verified, the darwin/arm64 archive binary was extracted, and v2 command help plus safe temporary-home surface smokes passed. |
| First beta for cautious users | **Not yet published** | The release-candidate evidence is green, but #192 did not create or publish a pre-release. Beta-ready install wording requires an approved `v0.2.0-rc.1` or later pre-release/release and post-publish artifact verification. |
| Normal public use from published install path | **Not yet** | Latest observed published release remains `v0.1.11` from 2026-02-22. Public-ready wording requires an approved release and release-artifact/Homebrew verification. |
| Native export/import app support | **Deferred / excluded** | #113 remains open/blocked. Native export/import is not part of this release-candidate promise. |

## Release-candidate version and tag plan

Recommended next tag, if maintainers explicitly approve publication in a later
release action: **`v0.2.0-rc.1`**.

No tag was created, pushed, or published for #192.

`v0.2.0-rc.1` is preferred over `v0.1.12` or `v0.1.12-rc.1` because the v2
local-settings-manager workflow is a material surface expansion relative to the
old published `v0.1.x` line. A `v0.2.0` pre-release communicates a scoped v2
candidate without implying a patch-only continuation of the stale `v0.1.11`
release. The project is still pre-1.0, so a minor bump is appropriate for a
larger experimental surface while preserving the ability to publish RC builds.

The GoReleaser snapshot built during #192 reports version
`0.1.11-SNAPSHOT-d0b705b` because no release tag was created. That snapshot
version is evidence of the non-publishing archive build only; it is **not** the
recommended published RC version.

## Install paths and publication boundary

| Install path | #192 status | Notes |
| --- | --- | --- |
| Current source checkout | Verified | Clean checkout tests and source binary help checks passed. This remains the safest path until a new pre-release is published. |
| GoReleaser archive artifacts | Verified as non-publishing snapshot | Snapshot archives were built with `.goreleaser.yml`, checksums verified, and the darwin/arm64 extracted binary was smoke-tested. |
| GitHub release artifacts | Not published in #192 | Requires separate explicit approval to create/push a tag and publish release artifacts. |
| Homebrew tap | Not verified end-to-end in #192 | Workflows require `HOMEBREW_TAP_DISPATCH_TOKEN`; #192 inspected workflow expectations but did not publish a release or prove the protected secret exists. |

## Evidence run summary

| Field | Evidence |
| --- | --- |
| Issue | #192 |
| Audited source commit | `d0b705b0da15a1375a34bdf7e81fb1bd19e6b704` |
| Ignored run root | `.tmp/issue-192/20260615T102901Z` |
| Clean checkout | `.tmp/issue-192/20260615T102901Z/clean-checkout` detached at audited commit |
| Latest observed GitHub release | `v0.1.11`, published 2026-02-22 |
| Live release verification | `gh release list --limit 5 --json tagName,publishedAt,isPrerelease,isDraft,isLatest,name,createdAt` recorded in `.tmp/issue-192/20260615T102901Z/logs/gh_release_list.json` |
| GoReleaser invocation | `go run github.com/goreleaser/goreleaser/v2@latest release --snapshot --clean -f .goreleaser.yml` |
| GoReleaser version | `v2.16.0` |
| GoReleaser config | `.goreleaser.yml` |
| Workflow comparison | Release workflow uses `goreleaser/goreleaser-action@v6` with `version: latest`; local #192 used one-off `go run ...@latest` to avoid system install while matching the configured latest GoReleaser intent and the same config file. |
| Publication mode | Non-publishing snapshot; `--snapshot` implies no artifact publication. |

## Clean-checkout and build checks

| Check | Result | Evidence log |
| --- | --- | --- |
| Create detached clean worktree | Passed | `.tmp/issue-192/20260615T102901Z/logs/git_worktree_add.stdout` |
| `go test ./...` from clean checkout | Passed | `.tmp/issue-192/20260615T102901Z/logs/clean_go_test.stdout` |
| Source build | Passed | `.tmp/issue-192/20260615T102901Z/logs/source_go_build.stdout` |
| Source binary `version` and v2 command help | Passed | `.tmp/issue-192/20260615T102901Z/logs/source_version_help.stdout` |

Source-built version output:

```text
dotfiles-manager version=dev commit=unknown date=unknown channel=dev provenance=unspecified
```

## Snapshot archive verification

| Check | Result | Evidence log |
| --- | --- | --- |
| GoReleaser snapshot build, no publish | Passed | `.tmp/issue-192/20260615T102901Z/logs/goreleaser_snapshot.stdout` |
| Checksums for all archives | Passed | `.tmp/issue-192/20260615T102901Z/logs/goreleaser_checksum_verify.stdout` |
| Extracted darwin/arm64 archive binary `version` and command help | Passed | `.tmp/issue-192/20260615T102901Z/logs/archive_binary_version_help.stdout` |

Snapshot artifacts produced:

```text
dotfiles-manager_0.1.11-SNAPSHOT-d0b705b_darwin_amd64.tar.gz
dotfiles-manager_0.1.11-SNAPSHOT-d0b705b_darwin_arm64.tar.gz
dotfiles-manager_0.1.11-SNAPSHOT-d0b705b_linux_amd64.tar.gz
dotfiles-manager_0.1.11-SNAPSHOT-d0b705b_linux_arm64.tar.gz
dotfiles-manager_0.1.11-SNAPSHOT-d0b705b_checksums.txt
```

Extracted archive binary version output:

```text
dotfiles-manager version=0.1.11-SNAPSHOT-d0b705b commit=unknown date=unknown channel=dev provenance=unspecified
```

## Safe temporary-home archive-binary smokes

All archive-binary smokes used temporary `HOME`, `XDG_CONFIG_HOME`,
`XDG_STATE_HOME`, `XDG_DATA_HOME`, `XDG_CACHE_HOME`, and temporary repositories
under the ignored run root. No real dotfiles were touched.

The surface-smoke script ran 66 archive-binary CLI commands. Summary evidence is
stored in `.tmp/issue-192/20260615T102901Z/surface-smoke-summary.json`; raw
stdout/stderr logs are under `.tmp/issue-192/20260615T102901Z/logs/surface-smoke/`.

The Git smoke explicitly checked both P0 fixes from #190/#191:

- confirmed `restore --yes` output contains `Restore completed.` and does not
  continue to label the confirmed run as a restore preview;
- after only `git:user.email` was saved and `git:user.name` was selected,
  `list --json` reports `git:user.email` as `desiredState.status=saved` and
  `git:user.name` as `desiredState.status=not-saved`.

## Supported-surface evidence matrix

| Surface | Evidence type | Commands/tests run | Result | User-readiness classification | Doc action |
| --- | --- | --- | --- | --- | --- |
| Git selected identity values | Archive-binary live smoke in temp HOME | `init`, `recipe explain git`, `add git --setting user.email`, `status`, `save --dry-run`, `save --yes`, `add git --setting user.name`, `list`, `diff`, `apply --dry-run`, `apply --yes`, `backup list`, `backup show`, `restore --dry-run`, `restore --yes` | Passed | Normal-user candidate after approved pre-release is published and verified | Keep user docs; current docs already limit Git to non-credential `[user]` identity values. |
| Starship selected root TOML values | Archive-binary live smoke in temp XDG config | `recipe explain starship`, `add starship --setting add_newline`, `status`, `save --dry-run`, `save --yes`, `diff`, `apply --dry-run`, `apply --yes` against `starship:add_newline` | Passed | Documented experimental surface; beta candidate after approved pre-release is published and verified | Keep user docs; exclusions for modules/comments/non-default config remain correct. |
| Zsh selected startup files | Archive-binary live smoke in temp HOME | `recipe explain zsh`, `add zsh --setting zshrc`, `status`, `save --dry-run`, `save --yes`, `diff`, `apply --dry-run`, `apply --yes` against `zsh:zshrc` | Passed | Documented experimental whole-file surface; beta candidate after approved pre-release is published and verified | Keep user docs; lifecycle warning/exclusions remain necessary. |
| tmux explicit config files | Archive-binary live smoke in temp HOME | `recipe explain tmux`, `add tmux --setting home.conf`, `status`, `save --dry-run`, `save --yes`, `diff`, `apply --dry-run`, `apply --yes` against `tmux:home.conf` | Passed | Documented experimental whole-file surface; beta candidate after approved pre-release is published and verified | Keep user docs; docs correctly exclude runtime reload/session control. |
| SSH primary user config | Archive-binary live smoke in temp HOME | `recipe explain ssh`, `add ssh --setting config`, `status`, `save --dry-run`, `save --yes`, `diff`, `apply --dry-run`, `apply --yes` against `ssh:config` with synthetic non-secret config | Passed | Documented experimental whole-file surface; beta candidate after approved pre-release is published and verified | Keep user docs; docs correctly exclude keys/certs/known_hosts/includes/chmod repair. |
| Neovim config tree | Archive-binary live smoke in temp XDG config | `recipe explain nvim`, `add nvim --setting config`, `status`, `save --dry-run`, `save --yes`, `diff`, `apply --dry-run`, `apply --yes` against `nvim:config` | Passed | Documented experimental file-tree surface; beta candidate after approved pre-release is published and verified | Keep user docs; docs correctly exclude plugins/generated state/non-default `NVIM_APPNAME`. |
| Local app authoring | Archive-binary synthetic fixture smoke | `app create`, `app validate`, generated documented roundtrip fixture, `app test --roundtrip --fixture basic` | Passed | Advanced-user authoring/testing surface, not a public recipe marketplace | Keep user docs; docs already frame this as local advanced authoring. |
| Legacy v1 file sync compatibility | Archive-binary compact v1 smoke | `.dotfiles-manager.yaml` with one sync; `status`, `diff`, `deploy --dry-run`, `import --dry-run` | Passed | Legacy compatibility surface for existing v1 users | Keep user docs; v1 remains compatibility, not the new v2 settings model. |
| `custom.files` | Focused Go tests and prior dogfood evidence | `go test ./internal/v2/dogfood ./internal/v2/customfiles ./internal/v2/ledger ./internal/v2/selectedlive -count=1`, plus #122 evidence | Passed for internal/generated path | Internal/generated and migration/dogfood surface only; public arbitrary live adoption remains deferred | Keep user docs; docs already say `custom.files` is not the recommended first-user app path. |
| Native export/import | Tracker/docs classification | #113 remains open/blocked; no native recipe selected or verified | Deferred / excluded | Not part of this release-candidate promise | Keep user docs; docs already say Raycast-like native export/import is not a general public workflow. |
| Homebrew tap dispatch | Workflow inspection only | `.github/workflows/release.yml` and `.github/workflows/dispatch-homebrew-tap.yml` inspected; both require `HOMEBREW_TAP_DISPATCH_TOKEN` | Limitation recorded | Not verified until an approved release is published and protected secret is present | Keep install docs warning that published binaries/tap may lag; verify after install. |

## Additional validation

| Check | Result | Evidence log |
| --- | --- | --- |
| Harbor scaffold validation | Passed | `.tmp/issue-192/20260615T102901Z/logs/harbor_validate.stdout` |
| `custom.files` internal dogfood package tests | Passed | `.tmp/issue-192/20260615T102901Z/logs/customfiles_dogfood_go_test.stdout` |
| Legacy v1 archive smoke | Passed | `.tmp/issue-192/20260615T102901Z/logs/legacy-v1/` |
| Current workspace `go test ./...` after #192 test-scan hygiene fix | Passed | `.tmp/issue-192/20260615T102901Z/logs/current_workspace_go_test.stdout` |
| `git diff --check` | Passed | `.tmp/issue-192/20260615T102901Z/logs/git_diff_check.stdout` |

## Documentation reconciliation

No user-facing supported-surface docs were narrowed in #192 because every
normal documented v2 surface received either archive-binary live smoke evidence
or remains explicitly classified as advanced/internal/deferred:

- Git, Starship, Zsh, tmux, SSH, and Neovim passed archive-binary temp-root
  smokes.
- Local app authoring passed archive-binary synthetic fixture smoke and remains
  documented as an advanced local authoring/test workflow.
- Legacy v1 compatibility passed archive-binary dry-run/status smoke and remains
  documented as compatibility for existing configs.
- `custom.files` remains internal/generated; user docs already say it is not the
  recommended first-user path.
- Native export/import remains excluded/deferred while #113 is open.
- Install docs already warn that published releases and Homebrew can lag current
  docs and instruct users to verify v2 command help after installation.

The historical [`v2-mvp-release-candidate.md`](./v2-mvp-release-candidate.md)
was left as a historical #124 evidence artifact and marked as superseded by this
#192 verification because #124 predated the #189/#190/#191 production-readiness
corrections.

## Final readiness statement for #192

If this #192 PR passes CI and is squash-merged, the project may say:

> Release-candidate evidence passed for the current scoped v2
> local-settings-manager surface on source and snapshot archive builds.

The project must **not** say:

> v2 beta/public install is ready.

That stronger claim requires a separate approved release action that publishes
`v0.2.0-rc.1` or later, verifies release artifacts from GitHub, verifies the
Homebrew tap path or records a release limitation, and records post-publish
evidence.

## Acceptance checklist

- [x] Define the release-candidate version/tag plan and intended install paths.
- [x] Run clean-checkout tests on the release-candidate commit.
- [x] Verify built archives expose expected v2 `version` and command help.
- [x] Verify the safe temporary-HOME quickstart passes against the
      release-candidate archive binary.
- [x] Reconcile every user-documented supported surface with release-candidate
      evidence or explicit advanced/internal/deferred classification.
- [x] Explicitly classify Git, Starship, Zsh, tmux, SSH, Neovim, local app
      authoring, legacy v1 compatibility, and `custom.files`.
- [x] Verify Homebrew tap dispatch/token expectations enough to document the
      limitation before publication.
- [ ] Require PR CI green, including static checks, linux shards, macOS sandbox
      lane, coverage aggregation, and final required check, before marking this
      issue Done.
- [x] State explicitly whether the result is manual dogfood, beta-ready, or
      public-ready.
- [x] ChatGPT Pro post-validation is recorded before merge/closure.
