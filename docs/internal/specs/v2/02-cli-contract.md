---
owner: Core Engineering
document-type: v2-draft-spec
status: Draft
last-updated: 2026-06-05
canonical-source: docs/internal/specs/v2/02-cli-contract.md
source-concept-sections:
  - CLI contract v2
  - Normal user workflow
  - Status and preview output
  - Canonical status and conflict state machine
  - v1 compatibility and migration contract
authority: Draft; non-authoritative until promoted by docs/internal/specs/v2/README.md
---

# v2 CLI contract

## Purpose

This spec defines the draft v2 command surface, global flags, prompt behavior,
JSON result envelope, and exit-code model.

The CLI should keep the normal path simple: add targets, inspect status, save
current changes, apply desired state, and use guided sync only for choices.

## Status and authority

This is a draft extraction from `../../scope/product-concept-v2.md`. It is not
implementation-authoritative until promoted through `README.md`.

Current v1 commands remain governed by the v1 specs and contracts.

## Source map and extraction notes

Extracted from the concept sections covering:

- normal command surface;
- CLI contract v2;
- prompt rules;
- status/preview output;
- migration behavior;
- acceptance matrix.

Deliberate non-decisions:

- exact CLI result field definitions beyond the required envelope semantics are
  deferred;
- exact text formatting is not final except for required semantic content;
- compatibility aliases are deferred.

## Terms owned by this spec

- command;
- operand/ref;
- global flag;
- prompt;
- non-interactive mode;
- JSON result envelope;
- exit code.

## Normative MVP rules

### Normal commands

The MVP command set is:

| Command | Default behavior | Writes? |
| --- | --- | --- |
| `init` | Create/connect local manager state and IDs. | local only |
| `add <target...>` | Add supported targets with safe defaults. | config |
| `list` | Show managed selections in the active resolved profile. | no |
| `status [ref]` | Compare desired, current, and last-applied state. | no |
| `diff [ref]` | Show readable diffs or opaque metadata. | no |
| `save [ref]` | Save or promote changed selected settings to desired artifacts. | repo |
| `apply [ref]` | Apply desired artifacts to live state after preview/backup. | live |
| `sync` | Guided save/apply/skip choices. | chosen |
| `backup list` | List local backups. | no |
| `restore <run-id>` | Restore from backup after preview. | live |
| `migrate` | Generate v2 config from v1 config after preview. | config |

`sync` must never mean blind automatic two-way merge.

### Advanced authoring commands

Advanced commands may exist outside the normal path:

```text
dotfiles-manager recipe list
dotfiles-manager recipe explain <target>
dotfiles-manager app create <target>
dotfiles-manager app edit <target>
dotfiles-manager app validate <target>
dotfiles-manager app test <target> --roundtrip
```

`recipe list` and `recipe explain <target>` are included in the MVP as read-only
advanced commands. `recipe list` shows static bundled target metadata and does
not resolve active profile selection. `recipe explain` explains target support,
selected settings, settings groups, resources, drivers, lifecycle policy,
redaction behavior, support levels, and capability limits without reading live
target state.

Mutating authoring commands such as `app create`, `app edit`, `app validate`,
and `app test` need their own later contract before implementation.

### `recipe list` read-only contract

`recipe list` emits static bundled registry metadata only. It must not inspect
live apps, desired artifacts, profile selection, app installation state, or
native export/import commands. User-local recipe IDs that collide with bundled
canonical IDs or aliases may produce warning diagnostics, but bundled registry
metadata remains authoritative for the bundled entry.

Text output must include deterministic target rows with target ID, source,
trust status, support level, capability, platform support, and aliases.

JSON output uses `command: recipe.list` and a command-specific object:

```yaml
recipeList:
  targets:
    - id: git
      displayName: Git
      aliases: [gitconfig]
      source: bundled
      recipeRef: recipe://bundled/git
      trustStatus: trusted
      version: "1"
      supportLevel: experimental
      capability: read-write
      platformSupport: unknown
      summary: sketch
  diagnostics: []
```

### `recipe explain <target>` read-only contract

`recipe explain <target>` accepts only a public target-ref owned by
`00-vocabulary.md`. It must reject setting refs, settings group refs, resource
refs, driver refs, artifact refs, and internal URIs as unsupported ref kinds.

The command may read:

- bundled recipe metadata;
- user-local recipe metadata, subject to safe parsing and redaction;
- active profile selection metadata when it can be resolved without mutation;
- static driver explanation metadata.

The command must not:

- bootstrap machine/user identity;
- create or update local state, cache, temp files, ledgers, backups, profiles,
  desired artifacts, trust records, or repository files;
- read live app/filesystem values for the target;
- read desired artifact payloads;
- read raw captures, ledger payloads, backup payloads, or native export output;
- run native export/import, driver read, detect, normalize, diff, backup, apply,
  verify, restore, or command-IO operations;
- prompt for consent or trust.

Profile selection reporting is best-effort and non-mutating. If the active
profile cannot be resolved without bootstrapping identity or writing local state,
`recipe explain` must still render safe recipe metadata, mark selection as
`unknown` or `unresolved`, and include a diagnostic instead of failing only for
that reason.

#### Required text output fields

Text output must include these sections when data exists, using clear human
labels and redacted-safe values only:

| Section | Required fields |
| --- | --- |
| Target support | target ref, display name, recipe source, recipe trust status, support level, target capability, platform support. |
| Selection summary | whether the target and settings are selected in the active profile, why they are selected/excluded/defaulted, or `unknown` when profile selection cannot be resolved safely. |
| Settings | public setting ref, label, support level, capability, default scope, artifact form, selection/default-inclusion status, sensitivity/redaction outcome, lifecycle policy, resource binding, driver, diff/apply limitations. |
| Settings groups | group ID, label, purpose, included setting refs, default selection or bulk-selection role, native import/export grouping when declared. |
| Resources and drivers | resource ID, named location ID, relative path or selector shape, driver ID, supported operations, backup/restore support, normalization mode, diff mode. |
| Native import/export | operation kind, reviewed/bundled status, artifact form, opaque/diffability, lifecycle requirement, timeout class, verification summary. |
| Safety and limitations | do-not-manage categories, lifecycle blockers/warnings, redaction limitations, unsupported or blocked settings, trust warnings. |
| Diagnostics | stable diagnostic code, message, and relevant target/ref/source/path only when safe to print. |

Native operation details must be summarized only. Output must not print raw argv,
environment variables, captured output, local paths containing secrets, or
value-bearing defaults.

#### `recipe.explain` JSON output

`recipe.explain --json` uses the existing CLI envelope with
`command: recipe.explain`. The minimum command-specific object is
`recipeExplain`. Field names below are a schema sketch until field-level CLI
schemas are promoted, but the object names are the stable minimum shape:

```yaml
schema: dotfiles-manager.v2.preview
schemaVersion: 1
command: recipe.explain
runId: run-...
summary:
  status: ok | blocked | error
recipeExplain:
  target:
    ref: git
    displayName: Git
    supportLevel: stable | read-only | experimental | deprecated | blocked
    capability: inspect-only | read-only | read-write | import-only | export-only | never
    platformSupport: supported | unsupported | unknown
  recipe:
    source: bundled | local
    recipeRef: recipe://bundled/git
    trustStatus: trusted | untrusted | review-required | unknown
    version: sketch
  selection:
    status: selected | partially-selected | not-selected | unknown | unresolved
    reason: sketch
    profileStack: [global, os/macos]
  settings: []
  settingGroups: []
  resources: []
  drivers: []
  nativeOperations: []
  safety:
    redactionSummary: sketch
    lifecycleSummary: sketch
    trustSummary: sketch
    doNotManage: []
  diagnostics: []
```

`settings[]`, `settingGroups[]`, `resources[]`, `drivers[]`, and
`nativeOperations[]` must contain the same categories required by text output.
`diagnostics[]` entries must include a stable code, message, severity, and safe
ref/source/path context when available.

#### `recipe.explain` diagnostics and exit codes

Stable diagnostic codes for `recipe explain` include:

| Code | Exit | Meaning |
| --- | --- | --- |
| `invalid-ref` | 2 | The operand is not valid public ref syntax. |
| `unsupported-ref-kind` | 2 | The operand is a setting, group, resource, artifact, driver, or URI ref instead of a target-ref. |
| `unknown-target` | 2 | No bundled or local recipe can explain the target. |
| `invalid-recipe` | 2 | Matching recipe metadata fails schema or safety validation. |
| `ambiguous-recipe` | 2 | More than one matching recipe applies and the manager cannot choose safely. |
| `selection-unresolved` | 0 | Recipe metadata rendered, but active profile selection could not be resolved without mutation. |
| `metadata-render-blocked` | 5 | Safety, trust, or redaction policy prevents even metadata explanation from being safely rendered. |
| `internal-error` | 1 | Unexpected implementation failure. |

A successful explanation exits `0`, including unsupported/blocked settings that
can be described safely. The command must not prompt, so it should not return
exit code `4` in normal operation. Exit code `6` is not expected for the single-target MVP form.

### Global flags

| Flag | Meaning |
| --- | --- |
| `--profile <layer>` | Add an explicit profile layer to the active stack. Repeatable. |
| `--scope <scope>` | Choose `shared`, `user`, `machine`, or `machine-user` when saving. |
| `--machine-id <id>` | Explicit machine identity input for bootstrap or transient read-only resolution; must not override an existing local machine identity. |
| `--user-id <id>` | Explicit user identity input for bootstrap or transient read-only resolution; must not override an existing local user identity. |
| `--dry-run` | Do not mutate desired repo artifacts or live target state. |
| `--json` | Emit stable machine-readable result data. |
| `--non-interactive` | Never prompt. Fail if input is required. |
| `--yes` | Accept safe default prompts, never safety blockers. |
| `--verbose` | Include profile stack, artifact URIs, drivers, and ledger refs. |

`--dry-run` may read current state, run declared read-only native export, and
write temporary/local run records. It must not change desired artifacts or live
state.

`--machine-id` and `--user-id` are advanced identity inputs. They must validate
against the identity grammar in `03-profile-and-scope-resolution.md`. If a
local identity record already exists, a conflicting flag value must fail with a
clear adoption/rename diagnostic rather than silently overriding the record.
Read-only and dry-run commands may use these flags only transiently and must not
persist identity records. `init` and other commands that are explicitly allowed
to bootstrap local manager state may persist them after validation and the
prompt/non-interactive rules in `03-profile-and-scope-resolution.md`.

### Ref operands

Public ref grammar is owned by `00-vocabulary.md`. A normal-command ref may
identify:

- a target-ref, such as `git`;
- a setting-ref, such as `git:user.email`.

`recipe explain <target>` accepts a target-ref only. Future narrowed resource,
group, driver, or artifact refs are outside the normal user-facing MVP command
surface and must not be implied by examples here.

Normal docs should prefer public target and setting refs over internal URIs.

### Prompt rules

`save` must prompt before:

- first saving a setting;
- choosing or changing a scope;
- saving an opaque artifact;
- saving data with sensitivity above `low`;
- replacing a desired artifact with no last-applied baseline.

`apply` must prompt before live writes unless policy and `--yes` allow a safe
default. It must still stop on safety, trust, lifecycle, and secret blockers.

`--non-interactive` must fail with exit code `4` if a prompt would be required.
It must not silently choose destructive, trust, opaque, or lifecycle answers.

### Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Completed with no errors or blocked required work. |
| `1` | General failure or unexpected internal error. |
| `2` | Validation, config, recipe, or schema error. |
| `3` | Changes or conflicts were found when used as a check. |
| `4` | User input required but unavailable in non-interactive mode. |
| `5` | Safety, lifecycle, trust, or secret policy blocked an operation. |
| `6` | Partial success: at least one item succeeded and one failed/skipped. |

Mixed target results must be represented per item. Safe items may complete while
blocked items are skipped, returning exit code `6` when appropriate.

## Derived schema boundaries, not final schemas

The JSON result envelope is owned by this spec. When the envelope is persisted
as a preview record, the canonical local-state path is
`runs/<run-id>/preview.json`, the schema identifier is
`dotfiles-manager.v2.preview`, and the schema file is
`schemas/v2/preview.schema.json`. Non-preview `--json` outputs reuse this
manager-owned envelope shape, but exact field-level schemas remain deferred.

Draft persisted preview envelope:

```yaml
schema: dotfiles-manager.v2.preview
schemaVersion: 1
command: init | add | list | status | diff | save | apply | sync | backup.list | restore | migrate | recipe.explain
runId: run-...
profileStack: [global, os/macos, user/leon]
summary:
  status: ok | changed | blocked | partial | error
  changed: 0
  blocked: 0
  applied: 0
  saved: 0
items: []
ledgerRef: state://ledger/current/...
```

Nested CLI commands should use stable dotted JSON identifiers, such as
`backup.list`.

Final JSON schemas must define:

- command identifier enum;
- item result shape;
- state code enum;
- allowed/blocked actions;
- message/error format;
- ledger and backup references;
- diff payload and redaction format.

## Examples

Examples in this CLI spec demonstrate command/ref shape and required semantic
content. Field names inside YAML/JSON result snippets remain sketches until the
CLI result schema is promoted.

### Normal setup

```bash
dotfiles-manager init
dotfiles-manager add git nvim starship
dotfiles-manager status
dotfiles-manager apply --dry-run
dotfiles-manager apply
```

### Save one Git value

```bash
dotfiles-manager status --user-id leon git:user.email
dotfiles-manager save --dry-run --user-id leon git:user.email
dotfiles-manager save --yes --user-id leon git:user.email
```

For the current MVP tranche, `git:user.email` and `git:user.name` are selected
through profile YAML before the user-facing `add` command is implemented. The
bundled Git runtime manages only `~/.gitconfig` `[user] email` and `[user] name`.
`save --yes` is the supported import/promotion command for selected Git identity
values: after an explicit selection and `save --dry-run`, it writes the current
safe live value into the desired settings artifact. If the desired artifact is
missing and the live selected value exists, `save --dry-run` must report
`plannedAction: would-promote` and count the item under the existing save summary
category.

The desired artifact path for a user-scoped Git setting is:

```text
desired/user/<user>/targets/git/settings.yaml
```

For example, `--user-id leon` writes
`desired/user/leon/targets/git/settings.yaml`.

The desired artifact stores the raw safe identity value because later apply
needs an actual desired value. Normal text output, JSON previews, reports,
ledgers, and backup metadata must not print raw selected values. Credential
helpers, tokens, signing keys, includes, URL credential rewrites, aliases,
arbitrary sections/keys, and repository-local `.git/config` remain unsupported
and must fail closed if selected explicitly.

If no desired artifact exists and no live selected value exists,
`save --dry-run` must not report `would-promote`; promotion is only for an
existing live selected value. Promotion applies only to the selected safe Git
identity key, so a user must repeat the preview-and-save flow for both
`git:user.email` and `git:user.name` when managing both values. Git
case-insensitive ambiguity such as `[User]` or `Email` must block before
promotion, desired writes, backups, or live mutation.

### Save one Zsh startup file

For the current MVP tranche, the bundled `zsh` runtime manages only selected
whole-file startup refs:

- `zsh:zshrc` -> `~/.zshrc`
- `zsh:zprofile` -> `~/.zprofile`
- `zsh:zlogin` -> `~/.zlogin`
- `zsh:zlogout` -> `~/.zlogout`

All four use `scopeDefault: user` and the named `home` location with default
`~`. The desired artifact path for a user-scoped Zsh file is:

```text
desired/user/<user>/targets/zsh/artifacts/<setting-id>
```

For example, `--user-id leon` and `zsh:zshrc` write
`desired/user/leon/targets/zsh/artifacts/zshrc`.

`save --yes` imports the current live startup file into the desired artifact.
`apply --yes` backs up the live startup file and writes the desired artifact
back to the live path through the generic file-resource command path.

Because these files affect shell startup, save/apply planning must emit a
non-blocking warning diagnostic with stable code:

```text
zsh.risk.shell-startup-file
```

`status` and `diff` must not emit this write warning. `.zshenv`, history files,
completion dumps/caches, cache directories (`zsh:cache` / `zsh:zsh-cache`),
session state, and plugin-manager/generated state must block as unsupported
before live reads and must not print raw file contents.
The Zsh recipe must not parse arbitrary shell scripts, discover `ZDOTDIR`,
restart shells, re-source shells, or install/manage plugin managers.

### Save/apply Neovim config tree

For the current MVP tranche, the bundled `nvim` runtime manages one selected
file-tree ref:

- `nvim:config` -> `~/.config/nvim`

The target uses `scopeDefault: user`, a named `config` location with default
`~/.config`, and registry `platformSupport: linux-darwin`. Windows paths are not
claimed by this bundled recipe in this slice.

The desired artifact path for a user-scoped Neovim config tree is:

```text
desired/user/<user>/targets/nvim/artifacts/config
```

For example, `--user-id leon` and `nvim:config` write
`desired/user/leon/targets/nvim/artifacts/config`, with URI
`desired://user/leon/targets/nvim/artifacts/config`. It must never be stored in
`settings.yaml`.

The selected command path for `file-tree` resources is the same generic
filesystem-resource path as selected whole-file resources:

- `status` and `diff` read live and desired tree metadata without printing file
  bytes;
- `save --dry-run` previews copying the managed live tree into the desired
  artifact directory;
- `save --yes` writes the desired artifact directory and verifies it;
- `apply --dry-run` previews copying the desired artifact directory to the live
  tree;
- `apply --yes` writes a pre-apply backup, applies the desired artifact
  directory to the live tree, and verifies it.

Missing-state behavior is normative:

- missing named location root (`~/.config` by default) blocks status/diff/save/apply;
  the manager must not create the parent location root;
- missing live tree with an existing location root is not an install-state
  assertion and must not be described as "Neovim not installed";
- missing live tree blocks save and must not delete/tombstone desired state;
- missing desired artifact blocks apply and must not delete/tombstone live state;
- missing live tree with existing desired artifact is allowed for apply; dry-run
  previews create, live apply records an absent-tree backup, creates the tree,
  and verifies.

The bundled Nvim recipe must exclude generated/risky paths narrowly by default,
including shada, swap, undo, view, session, cache, `.netrwhist`, plugin clone
directories (`pack/**`, `site/pack/**`, `bundle/**`, `plugged/**`), generated
dependency directories (`node_modules`, `.deps`, `.rocks`), and common key
material (`*.pem`, `*.key`, `*.p12`, `*.pfx`, `id_rsa`, `id_ed25519`). It must
not use broad secret/token/temp/backup excludes such as `**/*secret*`,
`**/*token*`, `**/tmp/**`, or `**/backup/**`.

The bundled Nvim recipe must not install Neovim, install/update plugins, run
package-manager actions, use runtime RPC, discover `NVIM_APPNAME` or
`XDG_CONFIG_HOME` alternatives, execute or lint Lua/Vimscript, or perform secret
scanning.

### Command-neutral status with no baseline

```text
Changed, no previous sync baseline:
  git:user.email    this user    save / apply / diff
```

The status output must make clear that both directions are possible only because
there is no trusted last-applied baseline.

## Errors, blockers, and partial-result behavior

Required command errors include:

- unknown command;
- invalid flag for command;
- unknown target or setting ref;
- ambiguous target name;
- prompt required in non-interactive mode;
- safety/trust/lifecycle blocker;
- no diff available for an opaque artifact unless metadata diff is allowed.

Partial results must identify succeeded, skipped, and failed items separately.

## Acceptance expectations

- Snapshot tests cover text and `--json` output for every normal command.
- Exit-code tests cover all codes listed above.
- Prompt tests cover interactive, `--yes`, and `--non-interactive` behavior.
- `sync` tests prove no blind two-way merge occurs.
- JSON output is stable enough for CI and future tooling.
- `recipe explain` snapshots cover text and JSON support, limitations,
  diagnostics, read-only behavior, and redaction boundaries.

## Out of scope

- final CLI result field-level schemas;
- shell completion;
- UI/TUI design;
- final authoring-command contracts;
- remote recipe catalog commands;
- user-facing plan commands and persisted plan files.

## Spec follow-ups / open decisions

- Decide exact text rendering for grouped status output.
- Decide compatibility aliases for v1 `deploy` and `import` commands.
