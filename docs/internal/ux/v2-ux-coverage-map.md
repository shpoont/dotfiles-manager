# v2 UX coverage map and storyboard backlog

Status: issue #171 planning artifact.
Last updated: 2026-06-15.
Scope: UX planning only; no command behavior, renderer, JSON schema, or v1 output changes.
Related issues: #113, #165, #166, #167, #168, #169, #171, #177, #179, #181, #183, #185, #187.
Pro pre-validation: <https://chatgpt.com/c/6a281d17-56ec-83ed-88d8-fc0d345b3b9f?dfm_storyboard=1781306002>.
Reconciliation: #185 marks completed #166/#167/#177/#179/#181/#183 coverage as complete where closure evidence exists. #187 adds restore preview/confirm docs/storyboard coverage only; renderer and behavior implementation remain future work. Custom-authoring, unsupported-app depth, trust-review, lifecycle, and native export/import lanes remain open or blocked.

## Purpose

This document maps the remaining v2 local-settings-manager UX surface after the
accepted safe Git quickstart storyboard in
[`v2-safe-quickstart-output-storyboard.md`](v2-safe-quickstart-output-storyboard.md).
It prevents the UX work from becoming opportunistic by deciding, before
implementation, which journeys need high-fidelity terminal storyboards, which
journeys only need reusable wording rules, and which journeys are future or
blocked lanes that must not be presented as available product support.

The product goal is still simple from the user's perspective: a user should be
able to ask the manager to handle common applications, keep relevant settings in
an external repo folder, inspect diffs, save desired state, apply desired state,
and recover safely. Advanced users should be able to author local recipes, but
the happy path should not require them to learn internal nouns first.

## Relationship to #169 and #165

- #169 is the accepted source of truth for the first safe temporary-HOME
  `git:user.email` quickstart storyboard.
- #165 established the selected-preview default/verbose/JSON output-tier
  contract for `status`, `diff`, `save`, and `apply`.
- This map complements those artifacts. It does **not** reopen #165 unless a
  missing universal output-tier rule would invalidate #165's contract.
- For each row below, the `Blocks #165?` column should remain `no` unless the
  row describes a universal output-tier rule that must exist before the tier
  contract can be trusted.

## Label glossary

### Coverage type

- `must-storyboard` — create a high-fidelity terminal transcript before
  implementation or broad copy changes.
- `rule-only` — define reusable wording and safety rules; no standalone
  transcript is needed unless later implementation exposes ambiguity.
- `transcript-review` — existing or future implementation must be checked by
  persona transcript review, but no new storyboard is required in this issue.
- `future/blocked` — keep a placeholder and safety language only; do not mock as
  available product behavior.

### Priority

- `P0` — required before production-ready v2 UX.
- `P1` — important for first public/beta usability, but not required to close the
  first production-readiness slice.
- `P2` — useful refinement or advanced workflow.

### Support status

- `current supported` — implemented or intended in the current v2 supported
  surface.
- `current internal/advanced` — available for maintainers or advanced users, but
  should not be framed as a basic happy path.
- `deferred` — valid product direction, but not part of the current supported UX
  promise.
- `blocked` — explicitly unavailable until a named blocker is resolved.

### Reviewer personas

- `first-time` — Git-literate first-time user who understands shell/Git but not
  dotfiles-manager.
- `cautious` — cautious non-expert Mac user who can copy commands but worries
  about touching real files.
- `power` — advanced dotfiles/power user or maintainer who cares about
  scriptability, auditability, and diagnostics.

## Universal UX contract coverage

| Contract area | Current owner | Coverage status | Required rule | Next consumer |
| --- | --- | --- | --- | --- |
| Default text vs `--verbose` vs `--json` | #165, #166, #167, #169 | covered for selected-preview and #167 setup/recipe/list/backup implementation; future lanes remain | Default text is human-first; verbose is default plus technical details; JSON remains stable and prose-free. | future profile/custom/native lanes |
| stdout/stderr behavior | #165, CLI spec | covered at contract level | Command-result text and JSON go to stdout. stderr is for argument parsing or unexpected process-level failures, not required explanations. | completed #166/#167 consumers; future CLI contract issues |
| Redaction and hidden values | #165, #169, security spec | baseline covered; must be checked per future flow | Show existence, file paths, hashes/counts, and redaction reason; never print raw managed values or secret-bearing payloads in default or verbose text. Do not imply general-purpose secret scanning beyond known recipe/policy detections. | completed #166/#167/#168 consumers; future flow reviews |
| Dry-run vs confirmed write language | #169, #165, #166, #179, #181 | covered for selected-preview implementation and aggregate storyboards; future lanes remain | Dry runs always say no files changed. Confirmed writes distinguish repo desired-state writes, live file/app writes, and manager-owned local state writes. | future implementation/profile/custom lanes |
| Backup and restore language | #169, #167, #187, backup spec | backup list/show covered; restore preview/confirm docs/storyboard coverage complete; renderer and exact behavior implementation remain future work | Default backup and restore text says which backup or recovery handle is involved, which live files are affected, whether files changed, and whole-file/artifact restore limits. Internal backup URIs stay out of default text and belong only in supported technical surfaces. | future restore renderer/implementation work |
| Blocked-state language | #165, #169 | baseline covered | Say why the command cannot proceed, confirm no files changed for blocked items, and give a safe next command or diagnostic path. | completed #166/#167 consumers; future blocked lanes |
| No-baseline language | #169, #165, #166 | covered baseline | Replace raw `no-baseline` with a plain review note: this setting has not previously been applied by this tool; review paths before confirming. | completed #166 consumer; future selected-preview regressions |
| Multi-app/multi-setting grouping | #165, #171, #177, #179, #181, #183 | aggregate docs/storyboard coverage complete; renderer implementation remains future work | Aggregate output must show counts, per-app grouping, blocked reasons, safe-to-confirm items, and safe next commands without leaking internal planner labels. | future renderer/production-readiness work |
| Partial success / partial blocked summaries | #171, #181, status-machine spec | docs coverage complete; renderer work remains | Report changed/unchanged/blocked counts; identify succeeded items and skipped/blocked items; return appropriate partial exit semantics without implying blocked items changed. | future renderer/production-readiness work |
| Next-command guidance | #169, #165, #171, #166, #167 | covered for single setting and #167 setup/list/backup flows; aggregate rules defined here | Give one safe next command when one command is safe. If no safe single command exists, say so and give narrower supported commands or resolution steps. | future aggregate/profile/custom/native lanes |
| Profiles/scopes/layers | v2 profile spec, #171 | implementation exists; UX coverage incomplete | Explain active profile/layers and scope in user terms without making groups first-class in the happy path. Raw layer IDs belong in verbose. | future issue from this map |
| Custom app authoring | app authoring issues, #171 | current internal/advanced | Keep separate from happy path. Explain it as an advanced path to add unsupported apps safely with fixture tests. | future issue from this map |
| Lifecycle-sensitive apps | lifecycle spec, #171 | future/blocked UX lane | Do not imply the manager will stop, restart, reload, control app servers, install plugins, or run package-manager actions by default. Default must warn or block unless a future reviewed recipe explicitly supports behavior. | future issue after lifecycle recipe support |
| Native export/import | #113, #171 | blocked | Represent only as blocked/future until #113 has a verified target with reviewed account and secret exclusions. | #113 and later UX issue |

## High-fidelity storyboard backlog

| Flow | User intent | Representative commands | Coverage type | Priority | Support status | Owner / future issue | Personas | Dependencies and safety constraints | Blocks #165? |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Safe Git single-setting quickstart | Try the product safely and understand one managed value. | `init`, `add git`, `status git:user.email`, `save --dry-run`, `save --yes`, `diff`, `apply --dry-run`, `apply --yes`; quickstart-level `restore --dry-run` wording only | `must-storyboard` | P0 | current supported | #169, consumed by #165/#166 | first-time, cautious, power | Temporary HOME examples only; no raw values. Quickstart-level restore wording is covered here; full restore preview/confirm docs/storyboard coverage is owned by #187. | no, complete for quickstart scope |
| Selected-setting status/save/diff/apply copy | Make the implemented selected-preview loop readable by default. | `status`, `diff`, `save`, `apply` with setting or target refs | `must-storyboard` | P0 | current supported | #166, implementation/docs coverage complete | first-time, cautious, power | Must preserve #165 tier contract and redaction. | no, complete |
| Multi-app selected status/diff | Check all selected settings and see grouped drift. | `status`, `diff`, `status git starship zsh` only if supported by selector grammar | `must-storyboard` | P0 | current supported | #177, `v2-aggregate-status-diff-storyboard.md` | first-time, cautious, power | Must show counts, per-app sections, blocked reasons, unsupported/failed states, and safe next command. Do not invent unsupported subset selectors or fake subset commands. | no, storyboard complete |
| Multi-app save all selected settings | Save current live state for several supported settings. | `save --dry-run`, `save --yes`, target/ref narrowed saves | `must-storyboard` | P0 | current supported | #179, `v2-aggregate-save-apply-storyboard.md` | first-time, cautious, power | Must distinguish repo desired-state writes from live writes. Must show which items are safe to confirm. | no, storyboard/docs coverage complete |
| Multi-app apply all selected settings | Apply saved desired state across supported apps. | `apply --dry-run`, `apply --yes`, narrowed applies | `must-storyboard` | P0 | current supported | #179, `v2-aggregate-save-apply-storyboard.md` | first-time, cautious, power | Must show live-write risk, backups, blocked items, and partial success semantics. | no, storyboard/docs coverage complete |
| Partial success / partial blocked aggregate run | Understand that some items changed while others were skipped. | aggregate `save --yes` or `apply --yes` with mixed states | `must-storyboard` | P0 | current supported | #181, final outcome semantics addendum in `v2-aggregate-save-apply-storyboard.md` | cautious, power | Must never imply blocked items changed. Exit-state wording must stay prose-only unless a separate contract defines shell/JSON behavior. | no, docs coverage complete |
| First setup / init | Create a v2 repo and local identity without touching real app configs. | `init --yes --machine-id ... --user-id ...`, `init --json` | `must-storyboard` | P0 | current supported | #167, implementation/docs coverage complete | first-time, cautious | Must distinguish repo files from local manager identity state and live app configs. | no, complete |
| Explain/discover supported apps | Find what can be managed and what is excluded. | `list`, `recipe discover`, `recipe explain git`, `recipe explain ssh` | `must-storyboard` | P0 | current supported | #167, implementation/docs coverage complete | first-time, cautious, power | Must not overclaim unsupported, excluded, or native lanes. | no, complete |
| Add one app or setting | Select a supported target safely. | `add git`, `add git:user.email`, `list` | `must-storyboard` | P0 | current supported | #167, implementation/docs coverage complete | first-time, cautious | Must explain selection vs saved desired value. | no, complete |
| Add several supported apps/settings | Select multiple apps without requiring internal resource groups. | repeated `add <target>` commands, `list`; future multi-target `add` syntax only if explicitly labeled unsupported | `must-storyboard` | P0 | current supported | #183, `v2-repeated-add-multiple-apps-storyboard.md` | first-time, cautious, power | Must be explicit about what is selected, what is not saved yet, what is excluded, and that current `add` accepts one target at a time. | no, storyboard/docs coverage complete |
| List selected settings across apps | Review managed surface at a glance. | `list`, `list --json` | `must-storyboard` | P0 | current supported | #167, implementation/docs coverage complete for default readable list output; #183 covers repeated-add multi-app list context | first-time, cautious, power | Must show selected apps/settings, scopes, and unsupported/blocked markers without internal IDs by default. | no, complete for #167/#183 scope |
| Backup list/show | Find available recovery points after writes. | `backup list`, `backup show <run-id>` | `must-storyboard` | P0 | current supported | #167, implementation/docs coverage complete | cautious, power | Must separate backup run IDs from internal `state://` refs. Must explain whole-file vs semantic restore limits. | no, complete |
| Restore preview/confirm | Preview and run recovery safely. | `restore <run-id> --dry-run`, `restore <run-id> --yes` | `must-storyboard` | P0 | current supported | #187, `v2-restore-preview-confirm-storyboard.md` | first-time, cautious, power | Docs/storyboard coverage only; not implemented restore behavior. Must identify files affected, source backup, dry-run no-write status, confirmed live writes, recovery handle when created, blocked cases, whole-file/artifact limits, and #113/native/lifecycle boundaries. | no, storyboard/docs coverage complete |
| Profiles and scopes basics | Understand shared, user, machine, and machine-user desired state. | `save --scope user`, `save --scope machine-user`, `list --verbose` | `must-storyboard` | P1 | current supported | new issue: `v2 UX: storyboard profiles and scopes` | first-time, power | Must explain value storage without making internal directories the first concept. | no |
| One machine with multiple profiles/layers | Use several profiles on one machine, such as global plus project or role layers. | `--profile work`, `--profile personal`, active stack examples | `must-storyboard` | P1 | current supported | new issue: `v2 UX: storyboard multiple profiles on one machine` | power, cautious | Must clarify that one machine can use multiple profiles/layers and that layers are ordered overlays. | no |
| Unknown profile/user/machine | Recover from identity or profile mismatch. | commands with unknown `--profile`, conflicting `--user-id`/`--machine-id` | `rule-only` | P1 | current supported | future wording issue | cautious, power | Must fail closed and explain how to inspect or initialize identity. | no |
| Custom app authoring and fixture/test roundtrip | Advanced user adds unsupported app safely. | `app create`, `app validate`, `app test --roundtrip`, local recipe trust | `must-storyboard` | P1 | current internal/advanced | future issue: `v2 UX: storyboard custom app authoring` | power | Keep out of happy path. Must require fixture/test roundtrip and trust review. Do not call this an import workflow. | no, future |
| Unsupported app or app not installed | User asks for an app that cannot currently be managed. | `add raycast` if no supported recipe, `recipe explain unknown` | `rule-only` | P0 | deferred / current depending target | future recipe ingestion issues; #167 baseline readable errors only | first-time, cautious | Do not say the manager can manage an unsupported app. If install-state discovery exists, explain it; otherwise do not infer installation from missing config. Suggest supported app list or custom app authoring. | no, future for unsupported-app UX depth |
| Lifecycle-sensitive app warning/block | App may need to be closed before safe write. | apply/save on lifecycle-sensitive target | `future/blocked` | P1 | blocked/deferred | future issue after reviewed lifecycle recipes | cautious, power | Do not imply automatic quit/reopen/reload, app server control, plugin install, or package-manager action. Warn or block unless recipe explicitly supports reviewed behavior. | no |
| Native export/import | Use app-native export/import commands generically. | future `save/apply` through native operation | `future/blocked` | P1 | blocked | #113, then future UX storyboard | cautious, power | Blocked until a verified native target exists. Must exclude account/session/secrets and document limitations. | no |
| Secret/sensitive content blocked | Avoid saving or applying known sensitive content accidentally. | save/apply where recipe or policy detections block | `rule-only` | P0 | current supported | #166/#167 baseline wording complete; future policy-specific UX as needed | cautious, power | Must say what category was blocked without printing the secret. Must not imply arbitrary/general secret scanning. | no, baseline complete |
| App recipe trust review | User runs local recipe for first time or after recipe changes. | `recipe explain`, `app validate`, `save/apply` with local recipe | `rule-only` | P1 | current internal/advanced | future trust UX issue; #167 baseline recipe explain readability only | power, cautious | Must explain trust record and changed local recipe evidence without dumping unsafe payloads. | no, future |
| Agent-eval transcript gate | Reuse persona questions across UX PRs. | Harbor/Codex transcript review tasks | `transcript-review` | P0 | current internal/advanced | #168, `v2-transcript-review-gate.md` | first-time, cautious, power | Must run on before/after transcripts, not only code inspection. Completed reviews must be checked into `reviews/`; issue/PR comments may link to the checked-in review. | no |

## Edge-case UX rule catalog

| Edge case | Default text rule | Verbose/JSON details | Coverage type | Owner |
| --- | --- | --- | --- | --- |
| Missing desired state | Say the setting is selected but no saved desired value exists yet. Offer `save --dry-run <ref>` when live state can be saved. | Raw state such as `missing-desired`, desired URI/path, diagnostic codes. | `rule-only` | #166 complete |
| Live file missing | Say no live value/file was found. If saving that missing state is supported, make that explicit; if applying would create a file and policy forbids it, block. | Driver/resource IDs and current snapshot metadata. | `rule-only` | #166 complete |
| Desired artifact missing or invalid | Say the saved desired value cannot be read or is missing/invalid; no live writes occurred. Offer save or validation command. | Parser error, desired URI, schema path, diagnostic code. | `rule-only` | #166/#167 baseline complete; future invalid-artifact depth if needed |
| App not installed | If install-state discovery exists for that target, say the app/target was not found on this machine and whether selection remains configured. If no install-state check exists, do not infer installation from a missing config file; say only what is known. Do not call it a success. | Detection checks and install-state diagnostics when available. | `rule-only` | #167 baseline complete; future install-state UX depth |
| App has no supported recipe | Say the manager does not currently support that app. Offer supported app list or custom app authoring path. | Candidate recipe lookup details. | `rule-only` | #167 baseline complete; future recipe ingestion |
| Lifecycle-sensitive app | Warn or block with manual next step. Do not promise automatic quit/reopen/reload, app server control, plugin install, or package-manager action unless a reviewed recipe supports it. | Lifecycle target IDs, detector result, action records. | `future/blocked` | future lifecycle UX |
| Secret/sensitive value detected | Say saving/applying is blocked because known recipe/policy detection found sensitive content; identify category only when safe. Do not imply arbitrary secret scanning. | Secret policy diagnostic code and safe category; never raw secret. | `rule-only` | #166/#167 baseline complete; future policy-specific UX |
| Unknown profile | Say the profile layer was not found and no files changed. Offer `list`/profile inspection once supported. | Profile stack resolution details. | `rule-only` | future profile UX |
| Unknown user or machine | Say the command cannot resolve local identity or the provided ID conflicts with local identity. Offer `init` or identity inspection guidance. | Identity record paths and conflict diagnostics. | `rule-only` | #167 baseline complete; future profile UX |
| Conflicting profile layers | Say the same setting is selected or configured differently in multiple active layers and the tool needs a narrower profile/layer decision. | Layer order, raw layer IDs, conflict path. | `rule-only` | future profile UX |
| No previous baseline | Say this setting has not previously been applied by this tool; review paths before confirming. | Raw `no-baseline`, ledger refs, last-applied lookup details. | `rule-only` | #166 complete |
| Backup unavailable | Block confirmed live write if backup is required but unavailable. Say no live files changed. | Backup policy, filesystem error, state path. | `rule-only` | #167 backup baseline complete; future write-policy UX if needed |
| Restore blocked | Say restore cannot proceed, what would be affected, and why it is unsafe or unavailable. | Backup ref, artifact refs, verification details in supported technical surfaces only. | `rule-only` | #187 docs/storyboard coverage complete; future renderer/implementation work |
| Restore is whole-file, not semantic single-value rollback | Say restore will restore the stored file/artifact, not edit only the one selected value, whenever that matters. | Artifact/driver details in supported technical surfaces only. | `rule-only` | #187 docs/storyboard coverage complete; future renderer/implementation work |
| Mixed changed/unchanged/blocked aggregate run | Always show total checked, changed/unchanged/blocked counts, per-app sections, blocked reasons, and safe next command(s). | Raw states/actions per item. | `must-storyboard` | #181 docs coverage complete; future renderer work |
| Native export/import unavailable | Say native export/import is not available yet for this app. Do not show fake export/import success mockups. | #113 blocker, candidate evaluation notes. | `future/blocked` | #113/future native UX |
| Local recipe untrusted or changed | Say local recipe needs review before reading/writing; no target files changed. | Trust record hash, recipe path, validation diagnostics. | `rule-only` | future trust UX; #167 recipe explain baseline only |
| Partial success after confirmed write | Say which items changed, which were skipped/blocked, whether backups were recorded, and what to do next. | Ledger run refs and per-item mutation records in verbose/JSON. Shell exit-code and JSON contract changes require a separate issue. | `must-storyboard` | #181 docs coverage complete |

## Multi-app default output shape

Aggregate commands should use a stable visual shape even before every exact word
is finalized. The output should be scannable, safe, and action-oriented:

1. One-line scope summary: how many apps/targets and settings/items were checked.
2. Summary counts: changed, unchanged/up-to-date, not saved yet, blocked, and
   failed when applicable.
3. Dry-run or write status: say `Dry run: no files were changed.` or identify
   repo/live/manager-state writes after confirmed commands.
4. Per-app sections with human display names.
5. One bullet per setting/item with plain-language state.
6. Blocked items include one safe reason line.
7. Backup line appears when live writes happened or would happen and backup is
   relevant.
8. Next section gives one safe command when possible.
9. If a safe single command cannot express the subset, the output must not invent
   one or show fake syntax. It should tell the user to resolve blocked items
   first or run specific narrower commands only when those commands are already
   supported by the CLI grammar.
10. `--verbose` adds technical details after the same human summary rather than
    replacing it.

Example shape:

```text
3 apps checked. 4 settings total.

Summary:
  Changed: 1
  Up to date: 1
  Not saved yet: 1
  Blocked: 1

Git
  - Git user email: differs from saved desired state
  - Git user name: up to date

Starship
  - Add newline: selected, but not saved yet

Zsh
  - .zshrc: blocked
    Reason: live file requires review before first apply

Dry run: no files were changed.

Next:
  Preview the ready Git changes only:
  dotfiles-manager apply --dry-run git

Blocked items were not included. Re-run with --verbose for diagnostics.
```

The command above is illustrative only when the selector grammar supports it.
If the selector grammar cannot safely express `ready Git changes only`, the
`Next` block must use safer generic wording instead of fake syntax, such as:

```text
Next:
  Resolve the blocked Zsh item first, or run a narrower supported command that
  the CLI already supports, such as:
  dotfiles-manager apply --dry-run git:user.email
```

## Default vs verbose for aggregate runs

Default aggregate output may show:

- app/target display names;
- public target refs or setting refs when they are needed to run the next
  command;
- user-level paths such as `$HOME/.gitconfig` or
  `desired/user/<user>/targets/git/settings.yaml`;
- backup run IDs;
- plain-language state and safety reasons.

Default aggregate output must not require understanding:

- resource IDs, driver IDs, selector IDs, location IDs;
- `desired://` or `state://` URIs;
- raw states/actions such as `missing-desired`, `blocked-safety`,
  `would-promote`, `would-apply`;
- raw ledger refs or backup artifact refs;
- raw lifecycle action records.

Verbose aggregate output should append a technical section with those fields for
power users and debugging while preserving the same redaction policy.

## Implementation sequencing

| Issue | How this map affects it |
| --- | --- |
| #165 | Already complete. It may proceed from #169 and the aggregate baseline in the CLI contract. This map does not block it. |
| #166 | Completed. It consumed the selected-setting `status`/`diff`/`save`/`apply` readability rows plus no-baseline, blocked-state, redaction, and selected-preview wording rules. Aggregate renderer work remains separate. |
| #167 | Completed for setup/init, discover/explain, add/list, backup list/show, baseline unsupported/error wording, and shared restore-related wording. It did **not** close full restore preview/confirm, custom app authoring, unsupported-app recipe ingestion, or trust-review UX depth; those remain future rows. |
| #168 | Completed. Adds `v2-transcript-review-gate.md` and the first checked-in review. Future single-setting and multi-app transcript work must use that gate when command text or output-tier expectations change. |
| #177 | Completed. Aggregate selected `status`/`diff` docs/storyboard coverage is complete; implementation/renderer work remains separate. |
| #179 | Completed. Aggregate selected `save`/`apply` confirmation docs/storyboard coverage is complete; implementation/renderer work remains separate. |
| #181 | Completed. Aggregate final outcome semantics docs coverage is complete; implementation/renderer work remains separate. |
| #183 | Completed. Repeated `add` flow for multiple supported apps/settings docs/storyboard coverage is complete without implying unsupported multi-target syntax. |
| #113 | Remains blocked until a verified native target exists. This map only preserves future UX requirements and must not create native export/import mockups that look available. #187 includes only blocked restore wording for native/lifecycle cases; it does not unblock or implement native export/import. |
| #187 | Restore preview/confirm docs/storyboard coverage is complete in `v2-restore-preview-confirm-storyboard.md` with a checked-in persona review. This is not implemented restore behavior and does not change JSON, v1 output, lifecycle, or native export/import support. |

## Completed storyboard artifacts and remaining backlog

Completed P0 storyboard/docs artifacts:

1. `v2 UX: storyboard aggregate selected status and diff` — P0, #177 complete.
2. `v2 UX docs: storyboard aggregate save/apply confirmations` — P0, #179
   docs/storyboard coverage complete.
3. `v2 UX docs: add aggregate final outcome semantics` — P0, #181 docs
   coverage complete.
4. `v2 UX: storyboard repeated add flow for multiple supported apps` — P0,
   #183 docs/storyboard coverage complete.
5. `v2 UX: storyboard restore preview and confirm` — P0, #187
   docs/storyboard coverage complete only; restore renderer/behavior
   implementation remains future work.

Remaining future storyboard/UX lanes to create or reuse issues for:

1. `v2 UX: storyboard profiles and scopes` — P1, covers
   global/user/machine/machine-user storage choices.
2. `v2 UX: storyboard multiple profiles on one machine` — P1, covers ordered
   profile/layer overlays.
3. `v2 UX: storyboard custom app authoring and fixture/test roundtrip` — P1,
   advanced-user flow for local recipes, validation, roundtrip fixtures, and
   trust review.
4. `v2 UX: lifecycle-sensitive app wording rules` — P1 future/blocked until
   reviewed lifecycle recipes exist.
5. `v2 UX: native export/import storyboard for first verified target` — blocked
   by #113.

The backlog deliberately does not ask for one giant mockup document. Each
storyboard should cover a user journey with before/after terminal transcripts and
persona review questions.

## Persona transcript review mapping

Every P0 storyboard or UX implementation PR should include before/after command
transcripts and ask the relevant personas to answer:

- What command ran?
- Did it change files? If yes, which class of files: repo desired state, live
  app/config state, or manager-owned local state?
- Which apps/settings were checked?
- Which values are hidden or redacted?
- Which items are safe to confirm?
- Which items are blocked and why?
- Was a backup created or would one be created?
- What should the user run next?
- What is explicitly not managed or not supported?

Persona emphasis by flow:

| Flow family | Required personas | Extra focus |
| --- | --- | --- |
| Single-setting quickstart | first-time, cautious, power | Basic comprehension, dry-run/write distinction, redaction. |
| Multi-app aggregate runs | first-time, cautious, power | Counts, grouping, partial blockers, safe subset commands. |
| Setup/discovery/add/list | first-time, cautious | No accidental live writes, clear supported/unsupported boundary. |
| Backup/restore | first-time, cautious, power | Recovery confidence, backup IDs, whole-file restore limits, and Git-context first-time comprehension when Git examples are used. |
| Profiles/scopes | first-time, power | Where values are stored and how layers override. |
| Custom app authoring and fixture/test roundtrip | power | Recipe safety, fixture tests, local trust, diagnostics. |
| Lifecycle/native lanes | cautious, power | Blocked/future status and no overclaiming of app control. |

## Out-of-scope guardrails

- No command semantics change should be introduced by UX artifacts.
- No JSON schema or field-name changes should be smuggled through storyboards.
- No v1 output changes.
- No fake support for native export/import before #113 is unblocked.
- No fake support for automatic app quit/reopen/reload unless a reviewed recipe
  and implementation explicitly support it.
- No raw managed values, secrets, tokens, private keys, session state, account
  identifiers, or opaque native payload bytes in examples.
- No resource groups as first-class happy-path concepts unless a later issue
  proves the user value and UX.

## Completion checklist for #171

- [x] Coverage map exists.
- [x] User journeys are listed with coverage type, priority, status, owner, and
  persona mapping.
- [x] High-fidelity storyboard needs are separated from reusable edge-case rules.
- [x] Multi-app/multi-setting aggregate runs are explicitly included.
- [x] Aggregate default-output shape is defined at a high level.
- [x] Profiles/scopes and multiple profiles/layers on one machine are included.
- [x] Custom app authoring is included as an advanced-user flow.
- [x] Lifecycle-sensitive apps and native export/import are future/blocked lanes.
- [x] #165 blocking relationship is explicitly stated.
- [x] Pro/UX review feedback is applied after draft and reconciled again by #185.
- [ ] Remaining future follow-up lanes are explicitly preserved in the listed backlog; not all are created yet.
