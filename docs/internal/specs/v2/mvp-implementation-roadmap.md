---
owner: Core Engineering
document-type: v2-planning-roadmap
status: Draft
last-updated: 2026-06-05
canonical-source: docs/internal/specs/v2/mvp-implementation-roadmap.md
authority: Non-authoritative planning roadmap; not part of formal specs 00-11
---

# v2 MVP implementation roadmap

## Purpose

This roadmap turns the draft v2 spec package into an implementation sequence for
AI agents and human reviewers.

It is not a formal v2 spec. The formal draft spec set remains:

```text
00-vocabulary.md
01-repository-layout.md
02-cli-contract.md
03-profile-and-scope-resolution.md
04-status-conflict-state-machine.md
05-desired-artifacts-and-uris.md
06-recipe-schema.md
07-driver-interface.md
08-mutation-ledger-backup-restore.md
09-security-redaction-trust.md
10-v1-migration.md
11-mvp-acceptance-tests.md
```

This roadmap should not be listed as an active spec in the formal spec table.
It is a planning bridge for issue creation and tracker cleanup.

## Authority and guardrails

- This roadmap is Draft and non-authoritative until separately accepted.
- V1 behavior remains governed by the existing v1 specs and contracts until
  explicit v2 promotion.
- This roadmap must not imply deletion, rewrite, or reinterpretation of v1
  config.
- Future GitHub issues should be generated from this roadmap only after the
  roadmap exists and is reviewed.
- No new GitHub issues are created in Step 3.

## Tracker reset

The current open GitHub issues were audited on 2026-06-04 against the accepted
v2 concept and draft v2 spec package.

### Delete as obsolete or superseded

These issues are old pre-v2 architecture/resource-driver/app-config fragments.
Their useful intent is now represented by the v2 concept, the 00-11 draft specs,
and this roadmap. They should be deleted and later re-authored as focused,
spec-referenced implementation issues.

```text
#11 Design resource/driver model for managed configuration state
#12 Add read-only resource registry and shadow verification mode
#13 Persist local state snapshots to support 3-way diff and better UX
#14 Implement CFPreferences/defaults selected-key driver
#15 Implement plist-file driver for authored plist files and selected key paths
#16 Implement file-document driver for JSON/YAML/TOML app configs
#17 Add do-not-manage resources and import denylist enforcement
#18 Add launchd and login-item runtime resource support
#19 Add secret-template resource type and manual secret verification
#20 Add AI-agent driver for app-specific configuration discovery and verification
#21 Replace hard-coded new-Mac app config shell blocks with manifest-driven apply/verify
#22 Add CLI/app-specific driver pattern for native exports and semantic commands
#39 Add core export/import artifact lifecycle for resource drivers
```

### Keep open

These issues remain independently valid current-v1 maintenance or UI/performance
work and should not be deleted as part of the v2 tracker reset.

```text
#23 perf: memoize shared-root source and target scans across syncs
#24 perf: cache/precompile pattern matching in planning hot loops
#25 perf: optimize file equality checks with metadata and streaming compare
#26 perf: stream file copy operations instead of full ReadFile/WriteFile
#27 perf: render text and JSON from a shared typed plan without repeated envelope filtering
#28 perf: reduce logging sink overhead for cheap commands without removing default logs
#29 perf: cache sync path expansion and validation during one command execution
#33 ui: standardize diff direction legend and patch orientation wording
#35 ui: add prominent dry-run versus apply safety cues in text output
#36 ui: define canonical phase glossary and cross-command aliases
#38 ui: include build provenance in version output
```

## MVP implementation sequence

### Milestone 0: spec hardening and tracker reset

Goal: turn the draft spec package into a stable implementation surface before
agents start coding.

Agent issues should cover:

1. Add final front matter and promotion metadata for v2 specs.
2. Normalize desired URI and public ID examples across all specs.
3. Implement the canonical schema filenames from `01-repository-layout.md`
   with field-level JSON Schemas.
4. Implement the canonical local state, backup, cache, and temp roots from
   `01-repository-layout.md`; retention policy remains a separate decision.
5. Implement machine/user ID bootstrap, adoption, rename-preview, and collision
   rules from `03-profile-and-scope-resolution.md`.
6. Implement the MVP platform matrix and filesystem gates from
   `09-security-redaction-trust.md`.
7. Implement `recipe.explain` from the CLI/JSON contract as a read-only MVP
   advanced command.
8. Set up Systems Mapping/Evaluation as local process support for design
   review, issue decomposition, and handoff evaluation. This is not v2 runtime
   product behavior and must not be read by the CLI.
9. Set up the local Harbor agent-test harness policy before runtime
   implementation: define the `evals/harbor/` structure, map selected
   agent-facing acceptance scenarios to Harbor candidates, preserve
   deterministic CLI/schema/safety tests as normal automated tests, and
   document the local-private auth boundary.
10. Convert this roadmap into GitHub milestones and agent-sized issues.

Exit gate:

- no open concept/spec contradictions;
- v2 issue backlog can be generated without relying on old deleted issues;
- every implementation issue references exact specs and acceptance tests;
- Systems Mapping and Harbor process policies are documented before runtime
  implementation issues begin;
- no live Systems Mapping records, copied Codex auth, generated Harbor jobs,
  aggregate jobs, local Docker build contexts, local images, or generated Harbor
  outputs are tracked;
- v2 implementation issues distinguish deterministic tests from optional
  agent-facing Harbor evaluations.

Current sequencing note:

- after #40/#53, complete #54 before runtime implementation issues such as the
  profile/scope/artifact skeleton and the `custom.files` vertical slice;
- Harbor setup is not a blocker for deterministic test design. It is a separate
  agent-evaluation layer for judgment-heavy process, UX, documentation, and
  handoff criteria.

### Milestone 1: core config, profile, and artifact skeleton

Goal: load and validate the control-plane model without mutating live state.

Agent issues should cover:

1. Root config discovery for v2 without silently reinterpreting v1 config.
2. Profile layer and profile stack schema skeleton.
3. Machine/user identity bootstrap, local-account mapping, adoption, and
   rename-preview skeleton.
4. Target and setting selection model.
5. Scope resolution for `shared`, `user`, `machine`, and `machine-user`.
6. Desired artifact URI and repository path resolution.
7. Schema-version plumbing for config, profiles, manifests, recipes, ledgers,
   and backups.

Relevant specs:

- `00-vocabulary.md`
- `01-repository-layout.md`
- `03-profile-and-scope-resolution.md`
- `05-desired-artifacts-and-uris.md`

Exit gate:

- a resolved profile can be rendered for fixtures;
- duplicate bindings and unsafe paths fail validation;
- v1 config remains governed by the legacy path.

### Milestone 2: custom.files vertical slice

Goal: prove v2 can replace current dotfile behavior through the new engine.

Scope:

- `custom.files` target;
- file driver;
- file-tree driver;
- desired artifacts;
- dry-run preview;
- local ledger skeleton;
- backup before apply where supported.

Agent issues should cover:

1. `custom.files` bundled recipe.
2. `file` driver detect/read/normalize/diff/preview/backup/apply/verify/restore.
3. `file-tree` driver include/exclude globs and metadata policy.
4. Path traversal and unsafe symlink rejection.
5. `init`, `add custom.files`, `list`, `status`, `diff`, `save --dry-run`, and
   `apply --dry-run` for the slice.
6. Real `save` and `apply` for the slice after preview.
7. `backup list` and `restore` for supported file resources.

Relevant specs:

- `02-cli-contract.md`
- `06-recipe-schema.md`
- `07-driver-interface.md`
- `08-mutation-ledger-backup-restore.md`
- `11-mvp-acceptance-tests.md`

Exit gate:

- the first vertical-slice gate in `11-mvp-acceptance-tests.md` passes;
- dry-run mutates nothing;
- apply creates backup where supported;
- restore is verified for supported file resources.

### Milestone 3: status, diff, preview, JSON, and exits

Goal: make command output dependable enough for users, CI, and future agents.

Agent issues should cover:

1. Canonical item state derivation.
2. Target-level status aggregation.
3. Command-neutral no-baseline status wording.
4. Conflict detection using last-applied baseline.
5. Redacted diff rendering.
6. JSON result envelope and item result shape.
7. Exit-code behavior, including partial success.
8. Prompt behavior for interactive, `--yes`, and `--non-interactive` modes.
9. Guided `sync` as save/apply/skip choice, not blind merge.

Relevant specs:

- `02-cli-contract.md`
- `04-status-conflict-state-machine.md`
- `05-desired-artifacts-and-uris.md`
- `09-security-redaction-trust.md`

Exit gate:

- snapshot tests cover text and JSON for every normal command;
- every canonical status state has a fixture;
- `sync` cannot silently merge.

### Milestone 4: mutation ledger, backup, and restore hardening

Goal: make live writes auditable and recoverable.

Agent issues should cover:

1. Local ledger schema and persistence.
2. Backup metadata and payload storage.
3. Ledger commit only after verified success.
4. Partial failure recording.
5. Restore preview.
6. Backup-before-restore.
7. Retention and cleanup policy.
8. Driver compatibility checks for restore.

Relevant specs:

- `04-status-conflict-state-machine.md`
- `08-mutation-ledger-backup-restore.md`
- `09-security-redaction-trust.md`
- `11-mvp-acceptance-tests.md`

Exit gate:

- no unverified success is recorded;
- supported restore works from backup;
- unsupported restore fails clearly.

### Milestone 5: structured drivers and initial bundled recipes

Goal: expand beyond raw files while staying deterministic and safe.

Initial driver issues should cover:

1. `ini-file` driver.
2. `json-file` driver.
3. `yaml-file` driver.
4. `toml-file` driver.
5. `plist-file` driver.
6. `macos-defaults-readonly` driver.
7. Selector validation and normalizer versioning.
8. Driver fixture suite.

Initial recipe issues should cover:

1. `git` selected sections, excluding credentials.
2. `zsh` selected files with warnings for risky files.
3. `nvim` file-tree config.
4. `tmux` config file.
5. `ssh` config-only support, never keys.
6. `starship` TOML config.
7. `raycast` snippets/quicklinks when diffable and optional opaque export opt-in.
8. `iTerm2` experimental/read-only preferences.
9. `macos.finder` and `macos.dock` selected read-only defaults.

Relevant specs:

- `06-recipe-schema.md`
- `07-driver-interface.md`
- `09-security-redaction-trust.md`
- `11-mvp-acceptance-tests.md`

Exit gate:

- structured-driver fixtures pass;
- initial recipes declare support level and capability;
- risky settings are excluded or blocked by default.

### Milestone 6: safety, redaction, trust, and lifecycle hardening

Goal: prevent accidental secret leakage, unsafe writes, and app-state
corruption.

Agent issues should cover:

1. Sensitivity classification.
2. Secret detection and blocked-save behavior.
3. Display redaction in text and JSON.
4. Untrusted local recipe write blockers.
5. Recipe-change broadening review.
6. Lifecycle policies: allowed, warn, blocked, ask-to-quit, reopen.
7. Native export/import safety gates for bundled reviewed support.
8. Opaque artifact opt-in and metadata-only diff.

Relevant specs:

- `06-recipe-schema.md`
- `07-driver-interface.md`
- `09-security-redaction-trust.md`
- `11-mvp-acceptance-tests.md`

Exit gate:

- secret fixtures do not leak;
- untrusted recipes cannot write;
- lifecycle blockers are user-visible and non-interactive safe.

### Milestone 7: v1 migration parity and dogfood release

Goal: prove current dotfile users can move to v2 safely.

Agent issues should cover:

1. Legacy `syncs:` adapter.
2. `migrate --dry-run` preview.
3. `migrate` output generation without deleting v1 config.
4. Generated `custom.files` target parity.
5. Optional promotion from `custom.files` to known targets.
6. V1 command compatibility aliases, if kept.
7. Parity report format.
8. Dogfood profile and non-critical machine run.

Relevant specs:

- `01-repository-layout.md`
- `02-cli-contract.md`
- `10-v1-migration.md`
- `11-mvp-acceptance-tests.md`

Exit gate:

- v1 parity fixtures pass;
- migration is preview-first and reversible;
- dogfood run can apply and restore core CLI/editor settings.

## AI-agent issue authoring rules

Every future implementation issue should include:

1. exact title and milestone;
2. relevant v2 spec files and sections;
3. expected files/modules to modify;
4. deterministic acceptance tests or fixtures to add first, plus any applicable
   Harbor agent-test case or rubric for judgment-heavy process, UX, spec-handoff,
   or support-boundary criteria;
5. out-of-scope boundaries;
6. safety/trust/redaction constraints;
7. dry-run and `--json` expectations where relevant;
8. compatibility notes for v1 behavior;
9. definition of done;
10. reviewer checklist.

Agents should not implement directly from `product-concept-v2.md` after a
relevant spec exists. The concept remains useful for rationale and product
intent, not as a substitute for implementation contracts.

## Future GitHub issue backlog plan

After this roadmap is accepted, create GitHub issues in milestone order. Start
with Milestone 0 and the first vertical slice only. Do not create all future
issues at once.

Recommended initial GitHub batch:

1. Harden v2 spec metadata and promotion state.
2. Finalize v2 schema file locations and local state paths.
3. Implement `recipe.explain` as the read-only MVP advanced command specified
   by the CLI contract.
4. Implement v2 profile/scope/artifact resolution skeleton.
5. Implement `custom.files` recipe and file driver vertical slice.
6. Implement status state derivation fixtures.
7. Implement dry-run preview and JSON envelope snapshots.
8. Implement ledger/backup skeleton for file resources.
9. Implement v1 migration dry-run fixture.

Do not recreate deleted old issues verbatim. Re-author them only when they can
reference the relevant v2 specs and concrete acceptance tests.

## Recipe explain decision

`recipe explain <target>` is included in the MVP as a read-only advanced command
because the accepted concept lists recipe explanation as part of the MVP support
surface.

Implementation must follow the `02-cli-contract.md` recipe-explain contract,
including:

- command row for `recipe explain <target>`;
- JSON command identifier `recipe.explain`;
- read-only behavior and no live state reads;
- output expectations for target support, settings, settings groups, resources,
  drivers, lifecycle, redaction, support levels, capabilities, safety limits,
  and stable diagnostics.

This command is not a normal user happy-path command, but it is important for
advanced users, recipe authors, and AI agents reviewing support boundaries.

## Production-ready definition

The MVP is production-ready only when:

- v1 parity for current dotfile use cases is proven;
- the custom.files vertical slice passes all gates;
- dry-run is trustworthy;
- live apply previews, backs up, verifies, and records ledgers;
- restore works for supported drivers;
- secret/redaction/trust tests pass;
- unsupported state fails visibly;
- CI passes on a clean checkout;
- dogfood succeeds on a non-critical profile/machine.
