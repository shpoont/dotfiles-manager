---
title: v2 production-readiness audit for issue 189
status: validated audit
owner: engineering
last_reviewed: 2026-06-15
issue: https://github.com/shpoont/dotfiles-manager/issues/189
---

# v2 production-readiness audit for issue #189

## Verdict

v2 is **not production-ready for normal users yet**.

Current state is better described as:

| Use level | Verdict | Why |
| --- | --- | --- |
| Manual source-build dogfood | **Yes, with care** | The safe temporary-home Git selected-value path passed with an explicit source-built binary and did not touch real user dotfiles. Existing dogfood evidence also covers several bundled targets in temporary roots. |
| First beta for cautious users | **Not yet** | Two user-visible correctness gaps were found during the audit: confirmed `restore --yes` output still reads like a preview, and `list` can report a setting as saved when only the target artifact exists but that setting is missing. |
| Normal public use from published install path | **Not yet** | The latest published release observed by this audit is `v0.1.11`, published on 2026-02-22. The current v2 docs and command surface should be verified in a new release candidate after the P0 output correctness gaps are fixed. |
| Native export/import app support | **Blocked/future** | #113 remains the explicit blocked/future issue for the first verified native export/import recipe with account and secret exclusions. |

The next work should stay focused on v2 and should be implemented one issue at a
time. Do not restart v1 work as part of this readiness path.

## Evidence summary

Raw logs and transcripts for this audit were kept under ignored local evidence
paths in `.tmp/issue-189/evidence/` and are not committed. They may include
machine-local paths and synthetic fixture values.

| Evidence | Command or source | Result | Notes |
| --- | --- | --- | --- |
| Source revision | `git rev-parse HEAD` | `1e1eb0c368f4e2f1096a166fd798818f632b1915` | Audit started from clean `main` after #187 was merged. |
| Local Go runtime | `go version`; `go env GOVERSION GOMOD` | `go1.26.0 darwin/arm64`; module `go 1.22` | CI and release workflows use Go `1.22`; local asdf Go was newer but compatible for the observed run. |
| Unit/integration/package tests | `$HOME/.asdf/shims/go test ./...` | Passed | Full package test suite passed locally. |
| Source build | `$HOME/.asdf/shims/go build -o .tmp/issue-189/bin/dotfiles-manager ./cmd/dotfiles-manager` | Passed | Every manual check used this explicit binary path, not an installed `dotfiles-manager` on `PATH`. |
| Version/help discovery | explicit binary `version`, root `--help`, and command `--help` for `init`, `add`, `list`, `status`, `diff`, `save`, `apply`, `backup`, `restore`, `recipe`, `app` | Passed | v2 commands are discoverable in the source-built binary. |
| Local static subset | `gofmt -l`; `$HOME/.asdf/shims/go vet ./...` | Passed | `staticcheck` and `golangci-lint` were not installed locally and were not installed during this audit; CI installs them. |
| CI parity surface | `.github/workflows/ci.yml`; `scripts/ci/*` | Deferred to PR CI, with local subset evidence only | The full CI surface includes `scripts/ci/run-static-checks.sh`, linux unit/integration/contract/performance Docker shards, macOS sandbox lane, coverage aggregation, and final required check. #189 must not be marked Done until the PR CI run is green or any unavailable lane is explicitly recorded. |
| Harbor scaffold validation | `bash evals/harbor/validate.sh` | Passed | Validated 4 committed local-private Harbor case scaffolds. No Harbor auth/Docker run was required for this audit. |
| Latest GitHub release | `gh release list`; `gh release view --json tagName,name,publishedAt,url,assets,isDraft,isPrerelease,targetCommitish` | Latest observed release `v0.1.11`, published 2026-02-22 | Release assets exist for darwin/linux amd64/arm64 plus checksums, but this release should not be assumed to match the current v2 docs. |
| Release workflow surface | `.github/workflows/release.yml`; `.goreleaser.yml` | Present | GoReleaser builds linux/darwin amd64/arm64 archives and release workflow can dispatch the Homebrew tap. |
| Isolated manual smoke path | `.tmp/issue-189/evidence/smoke-git-email-transcript.txt` | Passed functionally; found output bugs | Used temporary `HOME`, `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, and `XDG_CACHE_HOME`. |
| Aggregate spot check | `.tmp/issue-189/evidence/smoke-aggregate-git-transcript.txt` | Passed commands; found `list` state bug | Added `git:user.name` after saving only `git:user.email` and compared `list`, `status`, `diff`, and `save --dry-run`. |

## What is implemented today

The current source-built binary has meaningful v2 implementation, not only
specs. This audit directly smoke-tested the Git selected-value path only. Other
user-documented bundled surfaces are listed here as implemented surfaces because
they have source packages, tests, docs, or prior dogfood evidence, but they were
not all re-smoke-tested by #189. P0-3 must reconcile each documented surface
before beta/public readiness.

- v2 repository bootstrap via `init` with explicit machine/user identity.
- Recipe discovery/explanation for bundled targets.
- `add` and `list` for selected settings/profile layers.
- Selected-value `status`, `diff`, `save`, and `apply` with default text,
  `--verbose`, `--json`, redacted values, desired artifacts, ledgers, and
  backup creation for supported writes.
- `backup list`, `backup show`, and selected-value `restore` mechanics.
- Bundled target support documented for Git, Starship, Zsh, tmux, SSH, Neovim,
  local app authoring, and legacy v1 compatibility, with exclusions.
- Drivers and test packages for file, file-tree, INI, JSON, YAML, TOML, plist,
  macOS defaults, selected-live/selected-value preview, custom files,
  lifecycle, native operation plumbing, migration, app authoring, and dogfood.
- Local-private Harbor case scaffolds for issue quality, happy-path UX, recipe
  explanation clarity, and native safety review.

The implemented safe Git selected-value happy path is usable from a source build
when the user follows the temporary-home or carefully reviewed real-home docs.

## What is still only a spec, storyboard, or future expectation

The following should not be treated as fully delivered product behavior just
because internal docs exist:

| Area | Current status | Release interpretation |
| --- | --- | --- |
| Restore UX storyboard from #187 | Docs/storyboard/review coverage exists | Implementation still needs confirmed restore text to match the storyboard and truthfully distinguish dry-run vs write. |
| Aggregate multi-app UX storyboards from #177/#179/#181/#183 | Storyboards and some aggregate output implementation exist | Current aggregate output is usable enough for dogfood, but not all storyboard refinements are proven. The audit found one setting-level `list` accuracy bug. |
| Profiles/scopes advanced UX | Specs and partial CLI support exist | Needs a future P1 user-facing explanation/storyboard, especially for multiple profiles/layers on one machine. |
| Custom app authoring | Implementation and docs exist for advanced local recipes/fixtures | Should remain advanced/internal until beta docs and safety UX are stronger. |
| Lifecycle-sensitive app management | Specs/tests/plumbing exist | Do not promise automatic stop/reopen/reload behavior until reviewed target recipes and UX exist. |
| Native export/import | Plumbing/spec work exists; #113 remains open | Blocked/future. Do not claim Raycast-like native export/import support is generally available. |
| Public release distribution for current v2 | Release workflow exists; latest observed release is old | Needs a release-candidate verification issue after P0 output correctness fixes. |

## Isolated end-to-end smoke path

The audit ran an end-to-end smoke path with an explicit source-built binary:

```text
DFM=/Users/shpoont/Work/shpoont/dotfiles-manager/.tmp/issue-189/bin/dotfiles-manager
```

The smoke path used temporary roots only:

- temporary `HOME` for `~/.gitconfig`;
- temporary `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, and
  `XDG_CACHE_HOME`;
- temporary repository with `dotfiles-manager.v2.yaml`.

Representative commands included:

```bash
"$DFM" init --machine-id test-machine --user-id test-user --yes
"$DFM" --config dotfiles-manager.v2.yaml recipe explain git
"$DFM" --config dotfiles-manager.v2.yaml add git --setting user.email --scope user --profile global --yes
"$DFM" --config dotfiles-manager.v2.yaml list --user-id test-user
"$DFM" --config dotfiles-manager.v2.yaml status --user-id test-user git:user.email
"$DFM" --config dotfiles-manager.v2.yaml save --dry-run --user-id test-user git:user.email
"$DFM" --config dotfiles-manager.v2.yaml save --yes --user-id test-user git:user.email
"$DFM" --config dotfiles-manager.v2.yaml diff --user-id test-user git:user.email
"$DFM" --config dotfiles-manager.v2.yaml apply --dry-run --user-id test-user git:user.email
"$DFM" --config dotfiles-manager.v2.yaml apply --yes --user-id test-user git:user.email
"$DFM" --config dotfiles-manager.v2.yaml backup list --json
"$DFM" --config dotfiles-manager.v2.yaml backup show <run-id>
"$DFM" --config dotfiles-manager.v2.yaml restore <run-id> --dry-run --user-id test-user
"$DFM" --config dotfiles-manager.v2.yaml restore <run-id> --yes --user-id test-user
```

Functional result:

- `save --yes` stored the actual desired email under
  `desired/user/test-user/targets/git/settings.yaml`.
- `apply --yes` wrote the saved desired value back to the temporary
  `$HOME/.gitconfig` and recorded a local backup run.
- `restore --yes` restored the whole pre-apply backing file from the backup and
  created a backup-before-restore run.
- No real user dotfiles were touched.

Important finding: confirmed restore behavior works functionally in this smoke
path, but the human-readable output remains misleading. See P0-1 below.

## CI parity note

The local audit intentionally did not claim full CI parity from local commands.
Local evidence covers `go test ./...`, `gofmt -l`, `go vet ./...`, source build,
help discovery, Harbor scaffold validation, and manual smoke transcripts. The
repository CI surface is broader:

| CI lane | Local #189 status | Required before #189 Done |
| --- | --- | --- |
| `scripts/ci/run-static-checks.sh` | Partially run only through `gofmt` and `go vet`; `staticcheck` and `golangci-lint` were not installed locally and were not installed during the audit. | PR CI must run the full script after installing linters. |
| Linux unit shard | Not run as `scripts/ci/docker-shard.sh unit`; covered only by local `go test ./...`. | PR CI green. |
| Linux integration shard | Not run as `scripts/ci/docker-shard.sh integration`; covered only by local `go test ./...`. | PR CI green. |
| Linux contract shard | Not run as `scripts/ci/docker-shard.sh contract`; covered only by local `go test ./...`. | PR CI green. |
| Linux performance shard | Not run as `scripts/ci/docker-shard.sh performance`. | PR CI green. |
| macOS sandbox lane | Not run as `scripts/ci/run-macos-sandbox-lane.sh`; audit machine was macOS, but this lane remains a CI gate. | PR CI green. |
| Coverage aggregation | Not run locally. | PR CI green and coverage aggregation successful or explicitly explained. |
| Final required check | Not run locally. | PR CI final required check green before moving #189 Done. |

## P0 release blockers

### P0-1: confirmed restore output is not truthful enough

Observed command:

```bash
"$DFM" --config dotfiles-manager.v2.yaml restore <run-id> --yes --user-id test-user
```

Observed behavior:

- the live file was restored;
- a backup-before-restore run appeared in `backup list --json`;
- the text output still began with `dotfiles-manager v2 restore preview`;
- the item still reported `Result: would-change`;
- the backup line said a backup-before-restore `will be created` instead of
  saying it was created;
- the output did not give a clear confirmed-write outcome or recovery handle.

Why this blocks production readiness:

A restore command is the recovery path. If `--yes` output still looks like a dry
run, a normal user cannot confidently tell whether recovery happened.

Required follow-up issue:

```text
v2: make confirmed restore output truthful and human-first
```

Minimum acceptance:

- `restore --dry-run` clearly says no files changed.
- `restore --yes` clearly says which live files were restored.
- `restore --yes` reports the backup-before-restore run that was actually
  created.
- Text, verbose, and JSON tests cover dry-run, confirmed-write, blocked, and
  no-change restore cases.
- Whole-file/artifact restore limits remain explicit.

### P0-2: `list` can overstate setting-level desired-state coverage

Observed sequence:

1. Save `git:user.email` as desired state.
2. Add `git:user.name` to the same target/profile without saving that setting.
3. Run `list --user-id test-user`.
4. Run `status --user-id test-user`.

Observed behavior:

- `list` reported `git:user.name` as `Desired state: saved`.
- `status` correctly reported `git:user.name` as selected but not saved.

Likely interpretation:

`list` appears to treat the target desired artifact as saved without checking
whether each selected setting exists inside that artifact.

Why this blocks production readiness:

The manager's core promise is selected settings. If the overview says a setting
is saved when it is not, the user may believe a value is protected or deployable
when it is missing.

Required follow-up issue:

```text
v2: report list desired-state status at setting granularity
```

Minimum acceptance:

- `list` reports saved/not-saved per selected setting, not only per target
  artifact.
- Tests cover one target with two selected settings where only one setting is
  present in the desired artifact.
- `list --json` and text output agree.
- Next-command guidance points to the missing setting where possible.

### P0-3: release-candidate install, distribution, docs, and supported-surface evidence must be verified

Observed release state:

- latest observed GitHub release: `v0.1.11`, published 2026-02-22;
- source build from current `main` exposes the expected v2 commands;
- release workflow and GoReleaser config exist;
- user docs correctly warn that published releases may lag current docs.

Why this blocks normal public use:

A normal user should not have to infer whether Homebrew, GitHub Releases, or
`go install @latest` points to the same behavior described by the current docs.
After P0-1 and P0-2 are fixed, a release-candidate verification issue should
prove the install path end to end and reconcile every user-documented supported
surface before calling v2 ready for beta/public use. That issue should either
attach evidence for each documented surface or narrow the user docs so they do
not over-promise.

Required follow-up issue after P0-1 and P0-2:

```text
v2: verify release-candidate install, distribution, docs, and supported-surface evidence
```

Minimum acceptance:

- define the release candidate version/tag plan;
- run clean-checkout tests on the release candidate commit;
- verify built archives expose expected v2 `version` and command help;
- verify docs match the release candidate binary, including the temporary-home
  quickstart;
- reconcile every user-documented supported surface with release-candidate
  evidence or narrow the docs before beta/public readiness;
- verify Homebrew tap dispatch/token expectations or document the release
  limitation before publication;
- do not publish a release from this issue unless explicitly approved.

## Follow-up issue links

Final issue numbers created after Pro post-validation:

| Sequence | Issue | Status |
| --- | --- | --- |
| 1 | [#190 `v2: make confirmed restore output truthful and human-first`](https://github.com/shpoont/dotfiles-manager/issues/190) | Created; project Status `Todo`, Workflow Status `Ready`. |
| 2 | [#191 `v2: report list desired-state status at setting granularity`](https://github.com/shpoont/dotfiles-manager/issues/191) | Created; project Status `Todo`, Workflow Status `Ready`. |
| 3 | [#192 `v2: verify release-candidate install, distribution, docs, and supported-surface evidence`](https://github.com/shpoont/dotfiles-manager/issues/192) | Created; project Status `Todo`, Workflow Status `Ready`. |

## P1 and future work

These should not block fixing the P0 release blockers above, but they matter for
beta quality and broader adoption:

| Priority | Work | Notes |
| --- | --- | --- |
| P1 | Profiles/scopes user UX | Explain shared/user/machine/machine-user and multiple profile layers on one machine in user-facing examples. |
| P1 | Custom app authoring beta UX | Keep advanced, but provide a guided example and safety checklist for local recipes and fixture roundtrips. |
| P1 | Unsupported-app UX depth | Make unsupported app flows helpful without pretending the manager can manage unsupported apps. |
| P1 | Lifecycle-sensitive app UX | Add reviewed warning/block/reopen wording only when target recipes support it. |
| Future/blocked | Native export/import | Continue to track in #113; do not unblock without a verified target and account/secret exclusions. |
| Future | Broader bundled-app coverage | Add common apps one at a time with reviewed recipes, tests, and UX evidence. |

## User documentation audit

Current user docs are broadly aligned with implemented source-build behavior:

- `docs/user/install-and-release.md` tells users to build from source when docs
  are ahead of public releases and to verify v2 command help.
- `docs/user/getting-started.md` uses a temporary-home Git workflow and clearly
  says desired artifacts store actual values.
- `docs/user/commands.md` describes text/verbose/JSON tiers and v2 command
  grammar.
- `docs/user/README.md` distinguishes supported surfaces from exclusions and
  legacy v1 compatibility.
- `docs/user/faq.md` remains the practical place for limitations and recovery
  expectations.

Doc gaps or corrections after this audit:

- Once P0-1 is fixed, restore docs should be checked against the real confirmed
  output transcript.
- Once P0-2 is fixed, `list` examples should be checked against mixed saved and
  not-saved settings under one target.
- Release docs should not claim public install readiness until P0-3 is complete.

## Harbor and Systems Mapping/Evaluation audit

Harbor:

- `evals/harbor/validate.sh` passed for 4 case scaffolds.
- The Harbor suite is correctly documented as local-private and not a runtime or
  CI dependency.
- This audit did not run Harbor/Codex/RewardKit jobs. That is acceptable for
  #189 because the acceptance criterion was to audit readiness and validate the
  scaffold, not to run local-private agent judging.
- Before a larger beta/release review, Harbor can be used as a judgment aid for
  issue quality, UX clarity, recipe explanation, and native-safety planning.

Systems Mapping/Evaluation:

- `.systems-mapping/working-record.json` exists and was updated with #189 audit
  findings and next action.
- The working record remains an ignored process artifact, not product behavior
  and not release evidence by itself.
- Some older resume text had drifted; the #189 update corrected the active
  resume direction for this audit.

## GitHub tracker reconciliation

### #113

`#113 v2: implement first verified native export/import recipe with account and
secret exclusions` should remain open and blocked/future. It should not block a
v2 release that explicitly excludes native export/import, but it blocks any
claim that Raycast-like native export/import is generically supported.

### #23-#38 v1/current-cli issues

The audit found these open v1/current-cli issues:

| Issue | Classification | Recommendation |
| --- | --- | --- |
| #23 `v1 perf: memoize shared-root source and target scans across syncs` | v1-only performance backlog | Keep parked or close later in a separately approved v1 cleanup. Do not include in v2 path. |
| #24 `v1 perf: cache/precompile pattern matching in planning hot loops` | v1-only performance backlog | Keep parked or close later in a separately approved v1 cleanup. |
| #25 `v1 perf: optimize file equality checks with metadata and streaming compare` | v1-only performance backlog | Keep parked or close later in a separately approved v1 cleanup. |
| #26 `v1 perf: stream file copy operations instead of full ReadFile/WriteFile` | v1-only performance backlog | Keep parked or close later in a separately approved v1 cleanup. |
| #27 `v1 perf: render text and JSON from a shared typed plan without repeated envelope filtering` | v1-only implementation backlog; conceptually related to renderer quality | Do not reuse for v2. If v2 renderer work is needed, open explicit v2 issues. |
| #28 `v1 perf: reduce logging sink overhead for cheap commands without removing default logs` | v1-only performance backlog | Keep parked or close later in a separately approved v1 cleanup. |
| #29 `v1 perf: cache sync path expansion and validation during one command execution` | v1-only performance backlog | Keep parked or close later in a separately approved v1 cleanup. |
| #33 `v1 ui: standardize diff direction legend and patch orientation wording` | v1-only UI backlog | Keep parked; v2 diff wording should be handled through v2-specific issues only. |
| #35 `v1 ui: add prominent dry-run versus apply safety cues in text output` | v1-only UI backlog; thematically related to v2 safety | Do not reuse for v2. P0-1 is the v2 restore-specific safety output issue. |
| #36 `v1 ui: define canonical phase glossary and cross-command aliases` | v1-only UI backlog | Keep parked; v2 glossary/UX is already in v2 specs/docs. |
| #38 `v1 ui: include build provenance in version output` | v1-only UI backlog; release-provenance overlap | Do not reuse for v2 release readiness. Handle v2 release provenance in P0-3 if needed. |

No v1/current-cli issue should be closed or deleted by #189. If the user later
wants tracker cleanup, do it as a separate explicitly approved cleanup action.

## Final next issue sequence

ChatGPT Pro post-validation accepted the P0 sequence, and the tracker items have
now been created. Implement them one issue at a time in this order:

1. [#190 `v2: make confirmed restore output truthful and human-first`](https://github.com/shpoont/dotfiles-manager/issues/190) — fix the recovery-path UX correctness bug first.
2. [#191 `v2: report list desired-state status at setting granularity`](https://github.com/shpoont/dotfiles-manager/issues/191) — fix the selected-setting overview correctness bug second, after #190 and before #192.
3. [#192 `v2: verify release-candidate install, distribution, docs, and supported-surface evidence`](https://github.com/shpoont/dotfiles-manager/issues/192) — after #190 and #191 are merged, verify source/release/Homebrew installation readiness and reconcile every user-documented surface before declaring beta/public readiness.

After those, decide whether the next tranche should be:

- P1 profiles/scopes UX;
- P1 custom app authoring UX;
- first additional bundled app recipe;
- or #113 native export/import if a safe verified target is finally selected.

## Pro validation record

| Stage | Result | Notes |
| --- | --- | --- |
| Issue creation pre-validation | Approved with required changes | Pro required no v1 closures/deletions in #189, an isolated smoke path, and release/distribution evidence. |
| Issue creation post-validation | Approved | #189 tracker state was accepted. |
| Implementation-plan pre-validation | Approved with required changes | Pro required explicit source-built binary path, documented `add` grammar, no follow-up issue creation before post-validation, and concrete release evidence. |
| Audit report post-validation | Approved with required changes | Pro required recording this result, adding final P0 issue numbers/links after creation, adding CI-parity evidence, tightening supported-surface claims, and expanding P0-3 to include docs/supported-surface verification. |
| Final validation retry | Approved with required changes | Pro required strict one-issue-at-a-time sequencing: #191 must follow #190 and precede #192, and the report must state that #190-#192 were created and should be implemented in order. |
| Final validation after sequencing fixes | Approved | Pro answered `APPROVED` and `No required changes before merge.` |

