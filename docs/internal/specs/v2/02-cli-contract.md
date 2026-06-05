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
| `save [ref]` | Save changed selected settings to desired artifacts. | repo |
| `apply [ref]` | Apply desired artifacts to live state after preview/backup. | live |
| `sync` | Guided save/apply/skip choices. | chosen |
| `backup list` | List local backups. | no |
| `restore <run-id>` | Restore from backup after preview. | live |
| `migrate` | Generate v2 config from v1 config after preview. | config |

`sync` must never mean blind automatic two-way merge.

### Advanced authoring commands

Advanced commands may exist outside the normal path:

```text
dotfiles-manager recipe explain <target>
dotfiles-manager app create <target>
dotfiles-manager app edit <target>
dotfiles-manager app validate <target>
dotfiles-manager app test <target> --roundtrip
```

`recipe explain <target>` is included in the MVP as a read-only advanced command.
It should explain target support, selected settings, resources, drivers,
lifecycle policy, redaction behavior, support levels, and capability limits.

Mutating authoring commands such as `app create`, `app edit`, `app validate`,
and `app test` need their own later contract before implementation.

### Global flags

| Flag | Meaning |
| --- | --- |
| `--profile <layer>` | Add an explicit profile layer to the active stack. Repeatable. |
| `--scope <scope>` | Choose `shared`, `user`, `machine`, or `machine-user` when saving. |
| `--dry-run` | Do not mutate desired repo artifacts or live target state. |
| `--json` | Emit stable machine-readable result data. |
| `--non-interactive` | Never prompt. Fail if input is required. |
| `--yes` | Accept safe default prompts, never safety blockers. |
| `--verbose` | Include profile stack, artifact URIs, drivers, and ledger refs. |

`--dry-run` may read current state, run declared read-only native export, and
write temporary/local run records. It must not change desired artifacts or live
state.

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
dotfiles-manager status git:user.email
dotfiles-manager save git:user.email --scope user
```

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

## Out of scope

- final CLI result field-level schemas;
- shell completion;
- UI/TUI design;
- final authoring-command contracts;
- remote recipe catalog commands;
- user-facing plan commands and persisted plan files.

## Spec follow-ups / open decisions

- Define the exact `recipe explain` text and JSON payload shape.
- Decide exact text rendering for grouped status output.
- Decide compatibility aliases for v1 `deploy` and `import` commands.
