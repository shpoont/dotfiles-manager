---
owner: Work Manager
status: Pro-validated draft for Project Owner acceptance
document-type: v2-reset-audit
issue: 219
last-updated: 2026-06-23
canonical-source: docs/internal/project/v2-reset-audit-issue-219.md
inspected-commit: 1bbb484eb958d5477937da675da76482a43a8845
---

# v2 reset audit for issue #219

## Purpose

This audit compares the current repository state with the accepted v2 reset
model recorded in [`v2-reset-execution-record.md`](v2-reset-execution-record.md).
It is a discovery and sequencing artifact only. It does **not** implement product
behavior and does **not** close any of #210-#216 by itself.

The reset model says v2 should be a local settings manager whose primary
user-facing value is syncing selected settings between live app settings and a
settings storage folder. Git can manage that folder, but Git is optional.
Backup/restore and v1 migration are outside the current v2 product scope.

## Audit baseline

- Inspected commit: `1bbb484eb958d5477937da675da76482a43a8845`
  (`Adopt v2 project execution scaffold`).
- Branch used for the audit: `codex/219-v2-reset-audit`.
- Repository scope: tracked repository files only. The first scan covered Go,
  Markdown, YAML, and JSON. The tightening scan also covered TOML, Ruby, shell,
  text, Makefile, Brewfile, and Gemfile-shaped files. Both scans excluded
  generated or local output directories such as `.git/`, `dist/`, `bin/`,
  `artifacts/`, and the unrelated untracked `docs/presentations/` directory.
- GitHub scope: issue #219 plus reset follow-up issues #210-#216.
- Privacy scope: no live app settings, secrets, home-directory settings, or user
  local state were read.

## Classification model

| Classification | Meaning |
| --- | --- |
| Keep | Fits the reset model and can remain as implementation input. |
| Rename | Concept is mostly right but uses the wrong public noun or direction. |
| Remove | Conflicts with the reset and should leave the active v2 product surface. |
| Hide | Owner-approved internal or quarantined behavior: not advertised in public help/docs, not used as acceptance evidence, unreachable from the happy path, and tracked by an explicit follow-up issue. |
| Defer | Valid topic, but not needed before the next reset gate. |
| Project Owner decision | Needs an explicit owner call before implementation. |

## Commands and evidence collection

```bash
git status --short --branch
git log --oneline -5 --decorate
gh issue view 219 --json number,title,state,body,labels,comments,url,projectItems
for n in 210 211 212 213 214 215 216; do
  gh issue view "$n" --json number,title,state,body,labels,projectItems,url \
    > "/tmp/dfm-issue-$n.json"
done
find . -type f \( -name '*.go' -o -name '*.md' -o -name '*.yaml' \
  -o -name '*.yml' -o -name '*.json' \) \
  -not -path './.git/*' -not -path './dist/*' -not -path './bin/*' \
  -not -path './artifacts/*' -not -path './docs/presentations/*' \
  | sort > /tmp/dfm-audit-files.txt
$HOME/.asdf/shims/go run ./cmd/dotfiles-manager --help > /tmp/dfm-help.txt
$HOME/.asdf/shims/go run ./cmd/dotfiles-manager backup --help > /tmp/dfm-backup-help.txt
$HOME/.asdf/shims/go run ./cmd/dotfiles-manager migrate --help > /tmp/dfm-migrate-help.txt
git diff --name-only
git diff --stat
```

The term scan used the same file filter as above and searched these reset terms:
`repo`, `repository`, `save`, `apply`, `backup`, `restore`, `migration`, `user`,
`profile`, `git repo`, `git repository`, `settings storage`, `storage folder`,
`sync`, `dotfiles repo`, `settings repo`, `desired://`, `native export`,
`native import`, `catalog`, and `tap`.

Summary counts from `/tmp/dfm-audit-term-summary.md`:

| Term | Matches | Files | Highest-signal files |
| --- | ---: | ---: | --- |
| `repository` | 212 | 52 | `docs/internal/specs/v2/01-repository-layout.md`, `docs/internal/scope/product-concept-v2.md`, `docs/internal/specs/v2/02-cli-contract.md`, `docs/user/configuration.md` |
| `repo` | 4342 | 134 | Mostly code identifiers such as `RepoRoot`; material public uses are in concept/specs/docs. |
| `save` | 1301 | 87 | `docs/internal/scope/product-concept-v2.md`, `docs/internal/specs/v2/02-cli-contract.md`, `docs/user/commands.md`, selected-preview code/tests. |
| `apply` | 2355 | 136 | `docs/internal/scope/product-concept-v2.md`, `docs/internal/specs/v2/02-cli-contract.md`, selected-live/selected-preview code/tests. |
| `backup` | 2322 | 111 | `internal/v2/ledger/restore.go`, `internal/app/cli_backup_restore.go`, `docs/internal/specs/v2/08-mutation-ledger-backup-restore.md`, user docs. |
| `restore` | 1434 | 81 | `internal/v2/ledger/restore.go`, `internal/app/cli_backup_restore.go`, `docs/internal/ux/v2-restore-preview-confirm-storyboard.md`, user docs. |
| `migration` | 418 | 33 | `docs/internal/specs/v2/10-v1-migration.md`, `internal/v2/migration/*`, `internal/app/cli_migrate_test.go`, user docs. |
| `sync` | 1092 | 101 | `internal/app/cli.go`, v1 deploy/import payload tests, `internal/v2/guidedsync/*`, v2 docs. |
| `user` | 3512 | 177 | `docs/internal/scope/product-concept-v2.md`, selected-preview/list/desired tests, `docs/user/commands.md`, `docs/internal/specs/v2/02-cli-contract.md`. |
| `profile` | 1441 | 94 | `internal/v2/ledger/restore_test.go`, `docs/internal/scope/product-concept-v2.md`, custom-files/addtarget/file-tree tests, `docs/internal/specs/v2/03-profile-and-scope-resolution.md`. |
| mandatory-Git patterns | 342 | 69 | `docs/internal/ux/v2-safe-quickstart-output-storyboard.md`, migration tests, UX storyboards, selected-preview tests, concept doc. |
| legacy-v1 surface patterns | 1773 | 252 | `internal/app/cli.go`, deploy/import/status/diff tests, migration tests, `docs/user/commands.md`, current concept doc. |
| `settings storage` | 3 | 1 | Only the new execution record. |
| `storage folder` | 5 | 2 | New execution record and tailoring doc only. |
| `desired://` | 224 | 37 | Concept/specs, desired URI tests, selected-preview/list/resolution tests. |
| `native export` | 236 | 56 | `internal/v2/nativeexport/*`, concept/specs, UX coverage. |
| `native import` | 40 | 21 | Concept/specs and native-apply code. |
| `catalog` | 45 | 19 | Mostly concept/spec references; no remote catalog implementation. |
| `tap` | 163 | 41 | Mostly Homebrew tap/release files and recipe discovery code; no recipe-tap implementation. |

Primary line-level references used below:

These line references are for inspected commit
`1bbb484eb958d5477937da675da76482a43a8845`. If the referenced files change,
prefer commit-permalink evidence in PR review comments and refresh the line
numbers before closing follow-up issues.

| Area | References |
| --- | --- |
| Accepted reset constraints | `docs/internal/project/v2-reset-execution-record.md:27`, `docs/internal/project/v2-reset-execution-record.md:35`, `docs/internal/project/v2-reset-execution-record.md:41`, `docs/internal/project/v2-reset-execution-record.md:46`, `docs/internal/project/v2-reset-execution-record.md:47` |
| Current repo/save/apply concept | `docs/internal/scope/product-concept-v2.md:65`, `docs/internal/scope/product-concept-v2.md:75`, `docs/internal/scope/product-concept-v2.md:84`, `docs/internal/scope/product-concept-v2.md:85`, `docs/internal/scope/product-concept-v2.md:88` |
| Current repository layout spec | `docs/internal/specs/v2/01-repository-layout.md:16`, `docs/internal/specs/v2/01-repository-layout.md:21`, `docs/internal/specs/v2/01-repository-layout.md:52`, `docs/internal/specs/v2/01-repository-layout.md:130`, `docs/internal/specs/v2/01-repository-layout.md:143` |
| Current CLI spec conflicts | `docs/internal/specs/v2/02-cli-contract.md:78`, `docs/internal/specs/v2/02-cli-contract.md:79`, `docs/internal/specs/v2/02-cli-contract.md:81`, `docs/internal/specs/v2/02-cli-contract.md:82`, `docs/internal/specs/v2/02-cli-contract.md:83`, `docs/internal/specs/v2/02-cli-contract.md:378` |
| Backup/restore spec and docs | `docs/internal/specs/v2/08-mutation-ledger-backup-restore.md:16`, `docs/internal/specs/v2/08-mutation-ledger-backup-restore.md:20`, `docs/internal/specs/v2/08-mutation-ledger-backup-restore.md:188`, `README.md:8`, `README.md:20`, `docs/user/getting-started.md:127` |
| v1 migration spec and docs | `docs/internal/specs/v2/10-v1-migration.md:15`, `docs/internal/specs/v2/10-v1-migration.md:83`, `docs/internal/specs/v2/10-v1-migration.md:119`, `docs/user/faq.md:166`, `docs/user/commands.md:267` |
| Profile/scope alignment | `docs/internal/specs/v2/03-profile-and-scope-resolution.md:24`, `docs/internal/specs/v2/03-profile-and-scope-resolution.md:64`, `docs/internal/specs/v2/03-profile-and-scope-resolution.md:80`, `docs/internal/specs/v2/03-profile-and-scope-resolution.md:286`, `docs/internal/specs/v2/03-profile-and-scope-resolution.md:414` |
| Desired URI/value storage | `docs/internal/specs/v2/05-desired-artifacts-and-uris.md:71`, `docs/internal/specs/v2/05-desired-artifacts-and-uris.md:140`, `docs/internal/specs/v2/05-desired-artifacts-and-uris.md:268`, `docs/internal/specs/v2/05-desired-artifacts-and-uris.md:281`, `docs/internal/specs/v2/05-desired-artifacts-and-uris.md:297` |
| Native operation capability | `docs/internal/specs/v2/06-recipe-schema.md:167`, `docs/internal/specs/v2/06-recipe-schema.md:169`, `docs/internal/specs/v2/07-driver-interface.md:171`, `docs/internal/specs/v2/07-driver-interface.md:181`, `internal/v2/nativeops/nativeops.go:142`, `internal/v2/nativeops/nativeops.go:210` |
| Catalog/tap gap and bundled registry | `docs/internal/specs/v2/06-recipe-schema.md:49`, `docs/internal/specs/v2/06-recipe-schema.md:481`, `internal/v2/recipe/registry.go:29`, `internal/v2/recipe/registry.go:155`, `internal/v2/recipe/bundled_runtime.go:20` |

## GitHub tracker state read

Tracker evidence came from authenticated `gh issue view ... --json projectItems`
reads. Public GitHub issue pages may not show the same project metadata, so the
project status in this table should be refreshed with `gh` before any tracker
mutation. During this audit #219 itself was open and in project status
`In Progress`.

| Issue | Current role | State during audit | Project status read | Audit conclusion |
| --- | --- | --- | --- | --- |
| #210 | Product concept and vocabulary reset | Open | Todo | Must be first product-model issue after audit acceptance. |
| #211 | Status/diff/sync primary UX | Open | Todo | Too broad for direct implementation; should be split before writes. |
| #212 | Remove backup/restore from product scope | Open | Todo | Required before production docs and CLI help can be trusted. |
| #213 | Remove v1 migration from v2 roadmap | Open | Todo | Required before v2 roadmap/docs stop implying migration. |
| #214 | Recipe catalog/tap support | Open | Todo | Required design issue; current code has only bundled/local recipe discovery. |
| #215 | New-computer bootstrap flow | Open | Todo | Required UX/design issue after storage-folder vocabulary is fixed. |
| #216 | End-user documentation rewrite | Open | Todo | Should wait for #210-#215 decisions and verified behavior. |

## PR scope check

This audit branch is intentionally documentation/tracker evidence only. Current
working diff scope at audit time:

```text
docs/internal/project/v2-reset-audit-issue-219.md
docs/internal/project/v2-reset-execution-record.md

 docs/internal/project/v2-reset-audit-issue-219.md  | 640 +++++++++++++++++++++
 docs/internal/project/v2-reset-execution-record.md |  22 +-
 2 files changed, 655 insertions(+), 7 deletions(-)
```

No Go runtime, CLI command, recipe, or test behavior is changed by #219. Any
behavior change remains a follow-up issue after Project Owner acceptance.

## Public surface inventory

| Surface | Current reset conflict | Follow-up owner issue |
| --- | --- | --- |
| Root `README.md` | Presents backup/restore and Git-oriented repository workflow as part of v2. | #210, #212, #216 |
| `docs/user/commands.md` | Mixes selected-value v2 commands with legacy `deploy`, `import`, `migrate`, `backup`, and `restore`. | #211, #212, #213, #216 |
| `docs/user/configuration.md` | Leads with repository/control-plane terminology rather than optional settings storage folder. | #210, #216 |
| Root CLI help | Still exposes `backup`, `restore`, `migrate`, and describes `sync` as experimental. | #211, #212, #213 |
| Internal v2 specs | Contain useful implementation detail but conflict on active nouns, backup/restore, migration, and mandatory-Git assumptions. | #210-#214 |

## Test classification

The test suite is valuable but currently encodes both reusable v2 behavior and
old-model public contracts. The audit classified test families by the reset model
rather than treating all existing passing tests as product acceptance evidence.

| Test family from scan | Evidence count | Reset classification | Required follow-up |
| --- | ---: | --- | --- |
| Status/diff/sync tests | 92 files | Keep as primary behavior input, then rewrite expected text around sync-first UX. | #211 |
| Save/apply selected-preview/live tests | 52 files | Keep internals where they model directional sync, but rename public contract after owner decision. | #211 |
| Desired URI/value tests | 52 files | Keep, with public-output and sensitive-data warnings clarified. | #210/#211 |
| Recipe/native operation tests | 74 files | Keep as driver capability evidence, not as proof of production app support or remote catalog safety. | #214 |
| Backup/restore tests | 46 files | Remove from product acceptance; delete, quarantine, or relabel as internal safety tests after #212 decision. | #212 |
| Migration tests | 6 files | Remove from v2 acceptance; delete, archive, or quarantine after #213 decision. | #213 |
| Broad legacy-v1 compatibility tests | 126 files | Keep only as historical/regression reference unless explicitly assigned to a non-v2 maintenance track. | #213 / owner decision |
| CLI help/golden tests | 106 files | Rewrite after command vocabulary is accepted; current output should not be treated as final UX. | #210-#213 |

## Required term coverage conclusions

- `user` and `profile` are not inherently wrong, but the public copy should avoid
  implying a multi-user account system. The implementation can keep profile
  stacks; normal UX should say `for everyone`, `for me`, `for this machine`, and
  `for me on this machine`.
- Mandatory-Git assumptions are still widespread in specs, storyboards, tests,
  and command examples. #210 must make Git optional in nouns, help, examples,
  JSON contracts, and bootstrap flows.
- Legacy-v1 public surface is broader than migration alone. `deploy`, `import`,
  old status/diff phrasing, v1 config compatibility, and migration all need an
  explicit non-v2 track or removal/hiding decision.
- `settings storage folder` currently exists mostly in the reset execution
  record, which confirms that #210 is a real rewrite gate, not a polish task.

## Executive findings

1. The current repository is a useful prototype baseline, not a production-ready
   v2 product surface under the reset model.
2. The largest conflicts are in public docs, root help, and draft specs: they
   still present `repo`, `save`, `apply`, backups, restore, and v1 migration as
   central concepts.
3. Multiple profile layers, scopes, desired artifacts, named locations, selected
   values, native import/export safety, and recipe authoring contain reusable
   design/implementation work.
4. Backup/restore and v1 migration have substantial code and tests. They should
   not drive the v2 product plan. The remaining decision is whether to delete the
   code now or hide it while replacing public UX/docs.
5. Remote recipe catalogs/taps are not implemented and are only lightly described.
   #214 must define catalog trust and origin before any remote recipe can be used
   for writes.
6. The docs currently explain the prototype better than the reset product. #216
   should not start as a polish pass over the current docs; it should be a rewrite
   after #210-#215 settle the user model.

## Material findings

### F-01: Public product noun still says repository/repo

Classification: Rename.

Evidence:

- `docs/internal/scope/product-concept-v2.md` says the manager stores settings
  in a `config repo` and uses `save = ... into the repo` / `apply = ... repo
  settings` in the executive summary.
- `docs/internal/specs/v2/01-repository-layout.md` is titled `v2 repository
  layout`, owns `repository root`, and describes `repository-owned desired state`.
- `docs/user/configuration.md` starts by saying v2 uses a control plane and
  desired-state artifacts `in the repository`.
- The reset execution record is the only current source that consistently uses
  `settings storage folder`.

Decision:

- Rename public-facing language to `settings storage folder` or an owner-approved
  better noun in #210.
- Internal Go identifiers such as `RepoRoot` can remain temporarily if they are
  not exposed to users, but specs and JSON/text output should avoid making Git or
  a repository a requirement.
- Consider renaming `01-repository-layout.md` to a storage-layout spec during #210
  or marking it superseded and creating a replacement.

### F-02: Current concept makes save/apply primary, but reset wants sync primary

Classification: Rename / split.

Evidence:

- `docs/internal/scope/product-concept-v2.md` says the normal command surface is
  `add`, `list`, `status`, `diff`, `save`, and `apply`, and says `sync` is not
  stable happy path.
- `docs/internal/specs/v2/02-cli-contract.md` says the stable path is explicit
  `status`, `save --dry-run`, `save --yes`, `diff`, `apply --dry-run`, and
  `apply --yes`; `sync` is advanced/experimental.
- `internal/app/cli.go` exposes `sync` as `Experimental guided save/apply/skip
  flow for advanced v2 users`.
- The accepted reset model says the first-class feature should be inspecting
  status/diff and syncing selected/all settings in the correct direction.

Decision:

- #211 should make `status`, `diff`, and `sync` the primary UX.
- `save` and `apply` can remain as explicit directional sync aliases or advanced
  commands if the owner approves, but they should not be the main mental model.
- Split #211 into at least: read-only status/diff contract, smart-sync planning,
  write execution/confirmation, and partial/many-app UX fixtures.

### F-03: Backup/restore is a public feature in docs, specs, CLI, and code

Classification: Remove from product surface; Project Owner decision for code
retention vs deletion.

Evidence:

- Root `README.md` says v2 can `recover from local backups` and the current v2
  workflow ends with `backup/restore`.
- `docs/user/README.md`, `docs/user/getting-started.md`, `docs/user/commands.md`,
  and `docs/user/faq.md` document backup and restore as user workflows.
- `docs/internal/specs/v2/02-cli-contract.md` includes `backup list` and
  `restore <run-id>` in the MVP command set.
- `docs/internal/specs/v2/08-mutation-ledger-backup-restore.md` is an entire
  formal draft spec for backup and restore behavior.
- `internal/app/cli.go` registers `backup` and `restore` commands, and the current
  root help shows both commands.
- `internal/app/cli_backup_restore.go`, `internal/app/cli_restore_output.go`, and
  `internal/v2/ledger/restore.go` implement the current public commands.

Decision:

- #212 should remove backup/restore from user docs, happy-path UX, and root help.
- The owner should decide in #212 whether to delete backup/restore command code
  immediately or first hide it while status/diff/sync write behavior is reset.
  There should be no unlabelled public `backup` or `restore` v2 workflow after
  #212.
- If any backup-like behavior remains for implementation safety, #212 must make
  that an explicit internal safety decision with a non-product name and must not
  present it as a user-facing workflow.

### F-04: v1 migration is still treated as a v2 roadmap feature

Classification: Remove from active v2 plan; Project Owner decision for code
retention vs deletion.

Evidence:

- `docs/internal/specs/v2/README.md` says v2 must prove v1 parity and keep
  `syncs:` configs readable through a legacy adapter.
- `docs/internal/specs/v2/01-repository-layout.md` includes `migrations/v1-to-v2`
  as repository-owned layout.
- `docs/internal/specs/v2/10-v1-migration.md` is a full migration spec.
- `docs/internal/specs/v2/mvp-implementation-roadmap.md` includes migration and
  parity gates.
- `internal/app/cli.go` registers `migrate`, `migrate parity`, and
  `migrate promote-preview` commands; current help shows `migrate`.
- `internal/v2/migration/*` contains migration implementation and tests.
- `docs/user/faq.md` has a `How do I migrate v1 file syncs?` section.

Decision:

- #213 should remove migration from active v2 specs/docs/roadmap and root help.
- Useful v1 behavior can remain implementation inspiration only.
- The owner should decide in #213 whether to delete the migration command/code now
  or hide/archive it while v2 sync is implemented. It must not block v2 usability.

### F-05: Current docs are not a valid base for production v2 documentation

Classification: Remove/replace through #216 after product reset decisions.

Evidence:

- Root `README.md` and `docs/user/README.md` present backup/restore in the v2
  happy path.
- `docs/user/commands.md` mixes v2 selected-setting commands with legacy v1
  `deploy`, `import`, and migration commands.
- `docs/user/configuration.md` leads with repository language rather than a
  settings storage folder.
- `docs/user/faq.md` presents profiles/scopes/backups/migration as core concepts.

Decision:

- #216 should be a docs rewrite, not incremental copy editing.
- Interim docs may remain for the prototype, but must be labelled as prototype or
  current experimental behavior until #216.
- Production docs should start with: what live settings are, what the storage
  folder is, how status/diff/sync works, and what is deliberately not managed.

### F-06: Profile layers and scopes are mostly aligned, but user-facing naming is too technical

Classification: Keep with vocabulary simplification.

Evidence:

- `docs/internal/specs/v2/03-profile-and-scope-resolution.md` explicitly states
  that a machine can have multiple users, a user can appear on multiple machines,
  and a machine or user on one machine can use multiple profile layers.
- The same spec defines the four accepted scopes: `shared`, `user`, `machine`,
  and `machine-user`.
- `docs/user/configuration.md` already explains that commands can add extra
  profile layers with repeated `--profile <layer>` flags.

Decision:

- Keep the scope model and multiple profile-layer support.
- #210/#216 should avoid making `users` sound like an account-management product.
  In normal copy, prefer labels such as `for everyone`, `for me`, `for this
  machine`, and `for me on this machine`.
- Keep `profile layer` and `profile stack` mostly as advanced/internal concepts;
  normal users should not need them for the supported happy path.

### F-07: Desired artifacts and actual-value storage are reusable, with URI cleanup needed

Classification: Keep / Rename / Defer exact URI escaping until implementation.

Evidence:

- `docs/internal/specs/v2/05-desired-artifacts-and-uris.md` clearly says selected
  scalar values are stored in `settings.yaml`, file and file-tree payloads are
  stored under `artifacts/`, and payloads can contain actual managed bytes.
- The same spec defines canonical `desired://` URI shapes and says normal user
  commands should use public refs such as `git:user.email` rather than internal
  URIs.
- URI escaping remains deferred, and the parser is specified to fail closed on
  percent-encoding and ambiguous URI forms until escaping is specified.

Decision:

- Keep `desired://` internally.
- #210 should define public wording around actual stored values and make clear
  that the storage folder may contain sensitive managed bytes.
- #210 or #211 should decide whether user-facing output shows `desired://` by
  default or only in verbose/JSON output.
- Exact URI escaping can be deferred only if parsers continue to fail closed.

### F-08: Native import/export has a usable generic shape, but must remain recipe-driver capability

Classification: Keep as implementation capability; Hide from normal public UX
until supported recipes exist.

Evidence:

- `docs/internal/specs/v2/06-recipe-schema.md` models native import/export as a
  capability shape, not arbitrary shell scripting.
- `docs/internal/specs/v2/07-driver-interface.md` describes `native-export` as a
  metadata-only driver and blocks structured semantic diffs unless a later driver
  adds a normalizer.
- `internal/v2/nativeexport/nativeexport.go`, `internal/v2/nativeapply`, and
  `internal/v2/nativeops/nativeops.go` implement reviewed operation metadata,
  bounded command execution, payload hashing, temp inputs, and blocked unsafe
  command shapes.
- `docs/user/faq.md` correctly warns that native export/import is not a general
  public workflow in the current tranche.

Decision:

- Do not create app-specific one-off export/import mechanisms outside the
  reviewed recipe-driver model. If a particular app needs native import/export,
  it should be represented as a recipe-declared capability or a dedicated
  reviewed driver, not ad hoc user scripting.
- #214/#216 should describe native import/export as a recipe-declared driver
  capability, not a user-authored command script.
- Bundled app recipes using native export/import need dedicated evidence,
  lifecycle policy, tests, and user-readable limitations before appearing in
  production docs.

### F-09: Remote recipe catalogs/taps are underspecified and unimplemented

Classification: Defer implementation; #214 design required.

Evidence:

- `docs/internal/specs/v2/06-recipe-schema.md` explicitly says remote recipe
  catalog is deferred and out of scope.
- `internal/v2/recipe/registry.go` implements a static bundled registry for
  `custom.files`, `git`, `nvim`, `ssh`, `starship`, `tmux`, and `zsh`.
- Current term scan found `catalog` mostly in docs and `tap` mostly in Homebrew
  release infrastructure, not as a recipe-catalog product feature.

Decision:

- #214 should define catalog/tap data model, origin, update behavior, trust, and
  write authority before implementation.
- The existing bundled registry can be a candidate built-in catalog seed, but it
  is not yet accepted as official catalog content and is not a multi-catalog/tap
  system.
- Remote catalogs must not become a route for arbitrary command execution.

### F-10: New-computer bootstrap exists only as sketches and is still repo/apply oriented

Classification: Rename / Defer to #215.

Evidence:

- `docs/internal/scope/product-concept-v2.md` includes a new-machine setup sketch
  with `init --repo <repo-url-or-path>` and `apply`.
- `docs/user/getting-started.md` focuses on temporary-home and Git email rather
  than applying all settings from an existing storage folder after app install.
- #215 is already open for the bootstrap flow.

Decision:

- #215 should define a first-run flow using `settings storage folder` vocabulary,
  not mandatory Git.
- Include a Homebrew Bundle example as one app-install path, while keeping app
  installation outside dotfiles-manager's scope.
- Add missing-app and missing-setting UX before implementation.

### F-11: Bundled/common-app support is useful, but still experimental and narrow

Classification: Keep as prototype baseline.

Evidence:

- `internal/v2/recipe/registry.go` lists bundled targets for Git, Neovim, SSH,
  Starship, tmux, Zsh, and `custom.files`.
- `internal/v2/recipe/bundled_runtime.go` provides runtime recipes for those
  bundled targets.
- The recipes are narrow and exclude credentials, generated state, keys, sessions,
  caches, and risky files.

Decision:

- Keep this work as implementation input for the supported-app happy path.
- Do not present every experimental bundled target as production-ready until the
  reset specs/tests/docs are updated and accepted.
- `custom.files` should remain low-level/advanced, not the first normal-user path.

### F-12: Systems Mapping and Harbor process assets are present and should stay process-only

Classification: Keep.

Evidence:

- `.systems-mapping/README.md` exists.
- `evals/harbor/` and several candidate cases exist.
- The reset execution record treats these as process support, not runtime product
  behavior.

Decision:

- Keep these assets as Project Execution Standards support.
- Do not let CLI/runtime read live Systems Mapping records or Harbor auth/output.
- Future issues should distinguish deterministic CLI tests from optional
  agent-facing Harbor evaluations.

### F-13: Legacy v1 public surface is broader than migration

Classification: Remove from v2 public surface or move to explicit legacy
maintenance track; Project Owner decision.

Evidence:

- `docs/user/commands.md` still documents legacy `deploy` and `import` alongside
  v2 selected-setting commands.
- Root command registration still contains legacy command surfaces such as
  `deploy`, `import`, `status`, and `diff` behavior that predates the reset model.
- The required term scan found `legacy-v1 surface patterns` across 252 files,
  including deploy/import payload tests, status/diff payload tests, and current
  CLI text-output tests.
- Migration is only one part of this surface. A v2 user should not have to learn
  which old commands are real v2 workflow, compatibility maintenance, or
  historical reference.

Decision:

- Broaden or split #213 so it covers the full legacy public surface, not only the
  `migrate` command and migration spec.
- If the project keeps v1 commands for existing users, place them on an explicit
  legacy/maintenance track with clear docs and tests, separate from the v2 happy
  path and acceptance evidence.
- Do not use legacy `deploy`/`import` behavior as a substitute for reset-model
  status/diff/sync.

## Recommended sequencing after audit acceptance

### Phase 3A: Product model cleanup before runtime changes

1. Execute #210 first.
   - Rename public nouns from repo/repository to settings storage folder.
   - Decide final public names for storage, live settings, stored settings,
     profiles/layers, and internal URIs.
   - Mark superseded draft specs instead of letting conflicting specs stay active.
   - Publish an active-vs-superseded spec map so implementation agents know which
     documents are authoritative.
2. Then split #211 before implementation.
   - Read-only status/diff contract.
   - Smart-sync planning and choice UX.
   - Mutating sync execution and confirmations.
   - Partial and many-app sync UX/test fixtures.
   - This split is mandatory before mutating sync work starts.

### Phase 3B: Scope cleanup before production docs

3. Execute #212.
   - Remove or hide public backup/restore commands and docs.
   - Preserve only internal safety evidence if explicitly approved.
   - Treat this as a blocker for public-facing happy-path work and mutating
     sync-write implementation, not just production documentation.
4. Execute #213.
   - Remove or hide v1 migration commands/specs/docs from the active v2 roadmap.
   - Keep v1 as historical reference/inspiration only.
   - Broaden or split this issue to cover the wider legacy v1 public surface.
   - Treat this as a blocker for public-facing happy-path work and mutating
     sync-write implementation.

### Phase 3C: Expansion model and onboarding

5. Execute #214.
   - Specify official and additional recipe catalogs/taps, origin metadata, update
     behavior, and trust/write-authority rules.
6. Execute #215.
   - Specify new-computer bootstrap from a storage folder after apps are installed.
   - Include Homebrew Bundle as one example, not a dependency.

### Phase 3D: Documentation rewrite

7. Execute #216 only after #210-#215 produce accepted behavior and examples.
   - Rewrite docs around status/diff/sync, storage folder, supported apps,
     conflicts, catalogs, bootstrap, and advanced recipe authoring.
   - Validate against the shared documentation acceptance criteria before opening
     a production docs PR.

## Issue-specific recommendations

| Issue | Recommended update before implementation |
| --- | --- |
| #210 | Add acceptance criteria to supersede/rename `product-concept-v2.md`, `00-vocabulary.md`, and `01-repository-layout.md`; require storage-folder noun, optional Git, actual-value and sensitive-data explanation, internal URI policy, simplified profile wording, and an active-vs-superseded spec map. |
| #211 | Treat as parent and split before implementation. Make sync the primary UX, separate read-only planning from writes, add many-app/partial-sync fixtures, and define save/apply as directional aliases or advanced commands only after owner approval. |
| #212 | Require public CLI/help/docs removal or hiding of backup/restore. Add explicit owner decision: delete code now, or temporarily hide while retaining only approved internal safety evidence. Rename or supersede `08-mutation-ledger-backup-restore.md`. |
| #213 | Broaden or split to cover the full legacy v1 public surface: `migrate`, migration specs, `deploy`/`import`, legacy status/diff language, and v1 compatibility tests. Add explicit owner decision: delete, archive, or maintain on a separate legacy track. |
| #214 | Add a new design artifact for catalog/tap model. Treat the bundled registry as a candidate seed only; remote catalogs require trust/origin/update/write-authority gates before writes. |
| #215 | Define a complete new-computer flow with settings storage folder, app-installed prerequisite, all-app apply preview/confirmation, partial apply, missing-app handling, and Homebrew Bundle example. Depend on #210 vocabulary and #211 sync semantics before writing public docs. |
| #216 | Reframe as production docs rewrite after reset specs/behavior are accepted. Add blocker/dependency check on #210-#215, remove prototype docs from production acceptance, and require docs acceptance criteria review. |

## Project Owner decisions

### Needed to accept #219

Accepting #219 should approve only these planning-level decisions:

1. The classifications above are the right way to interpret the current
   repository against the reset model.
2. Phase 3 should start with #210, then mandatory #211 splitting, while #212 and
   #213 block public happy-path/mutating sync work.
3. Follow-up issues may be edited or split according to the recommendations in
   this audit.

### Deferred to follow-up issues

Accepting #219 does **not** decide:

1. The final public noun (`settings storage folder` vs a shorter noun such as
   `settings folder`) — #210 decides this.
2. Whether `save`/`apply` remain public aliases, advanced commands, or are hidden
   after sync becomes primary — #211 decides this.
3. Whether backup/restore code is deleted now or hidden/internalized first —
   #212 decides this.
4. Whether v1 commands/code/tests are deleted, archived, or maintained on a
   separate legacy track — #213 decides this.
5. Whether the bundled recipe registry becomes official catalog content — #214
   decides this.
6. Whether prototype docs remain published with warnings or are temporarily
   reduced — #216 decides this.

## Acceptance checklist for #219

- [x] Audit method and search terms recorded.
- [x] Exact commit inspected recorded.
- [x] GitHub issue/project state read and summarized.
- [x] Material findings classified as keep, rename, remove, hide, defer, or
      Project Owner decision.
- [x] Recommended updates/splits for #210-#216 recorded.
- [x] No product behavior changes made by this audit.
- [x] Pro review completed on this audit artifact.
- [ ] Project Owner accepts the audit classifications and Phase 3 sequence.
- [ ] Follow-up issue edits/splits are applied or explicitly deferred after owner
      acceptance.
