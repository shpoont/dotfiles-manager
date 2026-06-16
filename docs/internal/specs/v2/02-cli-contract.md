---
owner: Core Engineering
document-type: v2-draft-spec
status: Draft
last-updated: 2026-06-11
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

### `init` first-time bootstrap contract

The first implemented `init` tranche is a first-time bootstrap and local
identity-connect command:

```text
dotfiles-manager init [--machine-id <id>] [--user-id <id>] [--dry-run] [--yes] [--non-interactive] [--json]
```

It may create only:

- `dotfiles-manager.v2.yaml`;
- `profiles/stacks/default.yaml`;
- `profiles/layers/global.yaml`;
- local-only identity records under the manager state root:
  `identity/machine.yaml` and
  `identity/users/<local-account-key>.yaml`.

It must not create desired artifacts, live target files, backups, ledgers,
trust records, recipes, migration output, or app state. It must not adopt,
rename, repair, migrate, or infer ownership of an existing repository subject.
If no repository scaffold exists, `init` creates the minimal `default` stack
with one `global` profile layer. If a complete scaffold already exists and has
the expected schemas, `init` reports it unchanged. If only part of the scaffold
exists, or an existing scaffold file has the wrong schema, `init` fails
deterministically instead of repairing it.

Identity bootstrap follows `03-profile-and-scope-resolution.md`:

- existing local identity records win;
- explicit `--machine-id` and `--user-id` may create missing identity records;
- conflicting explicit IDs fail rather than overriding existing local records;
- interactive text mode explains that IDs are visible in repository desired
  paths and asks before writing;
- `--yes` may accept safe generated candidates;
- `--json` does **not** imply `--yes`, must not prompt, and exits `4` with
  `missingChoices` when identity approval/input is required.

Generated candidates are lower-case, path-safe strings matching
`[a-z0-9][a-z0-9._-]*`. Hostname/account-derived text is sanitized before it is
shown. If a generated candidate would silently collide with an existing
repository subject directory such as `desired/machine/<id>` or
`desired/user/<id>`, `init --yes` adds a deterministic numeric suffix instead
of treating that as adoption.

`init --json` uses:

```yaml
schema: dotfiles-manager.v2.init
schemaVersion: 1
command: init
runId: init
dryRun: false
summary:
  status: ok | changed | blocked | error
  planned: 0
  written: 0
  unchanged: 0
  blocked: 0
  failed: 0
init:
  activeProfileStack: default
  profileStack: [global]
  repoFiles:
    - kind: root-config | profile-stack | profile-layer
      path: dotfiles-manager.v2.yaml
      action: create | unchanged
  identityFiles:
    - kind: machine | user
      path: state://identity/machine.yaml
      id: safe-public-id
      localAccountKey: safe-local-key
      source: existing | explicit | generated | prompted
      action: create | unchanged
  missingChoices: []
diagnostics: []
error: null
```

Paths in JSON are repository-relative or `state://...` logical paths. The JSON
must not include absolute state-root paths, raw hostnames, raw OS account text,
secret values, timestamps, or run durations.


### `add <target>` profile-selection contract

The first implemented `add` tranche accepts one bundled target per invocation:

```text
dotfiles-manager add <target> [--setting <id>]... [--scope <scope>] [--profile <layer>] [--dry-run] [--yes] [--non-interactive] [--json]
```

It writes profile-layer **selection metadata only**. It must not write desired
values, live target files, backups, ledgers, app state, or trust records. Actual
values are imported later by `save` and deployed later by `apply`.

`add` uses `recipe explain` metadata to identify add-selectable settings and may
use `recipe discover` metadata as an advisory install/config hint. Discovery
must remain read-only and advisory. Unsupported-platform discovery blocks the
add by default; no force flag is part of this tranche.

Profile-layer writes are constrained:

- `--profile` must name an existing layer from the active profile stack only.
- The destination layer must be an existing regular file under
  `profiles/layers/` in the v2 repository root.
- Missing layer files, symlinked layer files, non-regular files, traversal,
  absolute paths, and out-of-repo resolved paths must fail closed.
- Writes must be atomic and preserve the layer file permissions.
- Patching must preserve unrelated YAML content, other targets, other
  selections, and unknown fields. It must not use a lossy struct rewrite that
  drops data unknown to the current implementation.

Conflict checks apply across the full active profile stack, not only the
destination layer. If the selected setting already exists anywhere in the active
stack with the same effective scope and artifact, `add` reports it unchanged
and does not duplicate it in another layer. If it exists with a different scope
or artifact, or the new write would silently shadow another active-layer
selection, `add` blocks with a stable conflict diagnostic.

Artifact writes must match the existing resolver/defaulting behavior. Scalar
settings may use the resolver's scope-only `settings.yaml#<setting>` default.
File, file-tree, native, and opaque settings must write explicit canonical
profile-layer artifact metadata such as `artifact: artifacts/<setting-id>` so
they resolve to desired artifact payload paths rather than settings values.

Prompt rules for `add`:

- `--json`, `--non-interactive`, and `--yes` must never prompt.
- JSON mode must not mix prompt text with JSON output.
- `--yes` may accept recommended recipe settings and recipe default scopes.
- `--yes` must not choose a profile layer when the active stack has multiple
  layers; `--profile` is required in that case.
- When a required choice is missing and prompting is disabled, `add` exits `4`
  with diagnostic code `add.choice-required` and machine-readable
  `missingChoices` entries.
- `--dry-run` performs the same target, profile, setting, scope, and conflict
  validation and emits the same planned changes, but writes nothing.

`add --profile <layer>` is a destination-layer flag, not a profile overlay. It
names the single active profile layer that receives selection metadata.

`add --json` uses:

```yaml
schema: dotfiles-manager.v2.add
schemaVersion: 1
command: add
runId: add-target
dryRun: false
summary:
  status: ok | changed | blocked | error
  planned: 0
  written: 0
  unchanged: 0
  blocked: 0
  failed: 0
add:
  target:
    id: git
    displayName: Git
    recipeRef: recipe://bundled/git
  activeProfileStack: default
  profileStack: [global, work]
  destinationProfileLayer: work
  discovery:
    state: config-present | installed | config-missing | unsupported-platform | ambiguous | not-applicable
    binaryState: installed | missing | ambiguous | not-applicable
    configState: config-present | config-missing | ambiguous | not-applicable
    platformState: supported | unsupported | unknown
  settings:
    - ref: git:user.email
      id: user.email
      label: User email
      scope: user
      scopeLabel: Me on all my machines
      artifact: artifacts/config
      artifactForm: scalar | file | file-tree | native | native-export | opaque
      action: add | unchanged
      sourceLayer: global
      resource:
        id: user-email
        driverId: ini-file
        locationId: home
        path: .gitconfig
      selectorSummary: "[user] email"
      nextActions:
        - dotfiles-manager status git:user.email
        - dotfiles-manager save --dry-run git:user.email
        - dotfiles-manager sync git:user.email
  missingChoices: []
diagnostics: []
error: null
```

`add` text output should stay happy-path oriented. It may mention profile stack,
destination profile layer, target discovery, scope, resource, driver, named
location, selector summary, desired artifact binding, and next actions. It must
not require users to understand internal resource grouping nouns.

### `list` managed-selection contract

`list` is the normal command for showing what the active profile currently
manages:

```text
dotfiles-manager list [--json] [--machine-id <id>] [--user-id <id>] [--profile <layer>]...
```

It lists only managed selections from the active profile stack plus any explicit
repeatable `--profile` overlay layers. It must not show the bundled recipe
catalog; static supported-target discovery remains under `recipe list` and
install/config probing remains under `recipe discover`.

`list` is read-only. It must not create identity records, bootstrap state, read
live target values, write desired artifacts, write backups or ledgers, launch
apps, or run native export/import commands. If identity is missing for a
selected scope, `list` still returns the managed row with
`subject.resolved: false`, a stable `missing` identity list, no desired URI, and
a `partial` summary. Missing identity is not an implicit request to run `init`.

`list --profile <layer>` is an overlay flag, matching `status`, `diff`,
`save`, `apply`, and `sync`: each repeatable value is appended to the active
profile stack for this read-only view. It does not write the named layer.

`list --json` uses:

```yaml
schema: dotfiles-manager.v2.list
schemaVersion: 1
command: list
runId: list-managed
summary:
  status: ok | partial | blocked | error
  targets: 1
  settings: 1
  unresolved: 0
  blocked: 0
  failed: 0
list:
  activeProfileStack: default
  profileStack: [global, work]
  settings:
    - ref: git:user.email
      target:
        id: git
        displayName: Git
        recipeRef: recipe://bundled/git
      setting:
        id: user.email
        label: User email
      scope: user
      scopeLabel: Me on all my machines
      subject:
        resolved: true
        id: leon
        missing: []
      sourceLayer: work
      artifact: artifacts/config
      artifactForm: scalar
      desiredUri: desired://user/leon/targets/git/settings#user.email
      desiredRelPath: desired/user/leon/targets/git/settings.yaml
      resource:
        id: user-email
        driverId: ini-file
        locationId: home
        path: .gitconfig
      selectorSummary: "[user] email"
      nextActions:
        - dotfiles-manager status git:user.email
        - dotfiles-manager save --dry-run git:user.email
        - dotfiles-manager sync git:user.email
diagnostics: []
error: null
```

Rows are sorted deterministically by public setting ref. Paths in JSON are
repository-relative or logical URIs; output must not include live values, secret
values, absolute state-root paths, timestamps, or run durations. Text output
should emphasize target/setting refs, scope, subject resolution, source layer,
named settings location, selector/resource summary, desired URI when resolved,
and next actions.


### `sync [ref]` guided-choice contract

The first implemented `sync` tranche is a v2-only guided flow:

```text
dotfiles-manager sync [ref] [--choice <setting-ref=save|apply|skip>]... [--yes] [--non-interactive] [--json] [--machine-id <id>] [--user-id <id>] [--profile <layer>]...
```

`sync` always starts with a read-only planning phase. The plan lists every
resolved item with current state, recommended action when one is safe to
recommend, allowed choices, selected choice, resource/selector summary, and
diagnostics or blockers. Planning output alone must not mutate desired
artifacts, live files, backups, ledgers, app state, native state, or trust
records.

Interactive text mode prompts for explicit `save`, `apply`, or `skip` choices
only for actionable changed, no-baseline, conflict, missing, or opaque-changed
items. The prompt must say that choosing `save` or `apply` mutates that item.
All prompted choices must be collected and validated before any live mutation.
If any required prompt is unanswered, invalid, interrupted, or unavailable, the
command fails with no writes.

Non-interactive execution requires explicit repeatable `--choice` flags and
`--yes`. `--choice` without `--yes` is a validated plan only and must fail
before mutation when any chosen action would write. Missing required choices in
`--yes` execution fail with no writes. `--json` with no choices is plan-only and
must emit the full machine-readable plan without prompting or writing.

Choice validation is strict and happens before execution:

- unknown setting refs fail;
- duplicate refs fail;
- malformed choices fail;
- choices not allowed for the item's state fail;
- choices for safety-blocked, lifecycle-blocked, or unsupported items fail.

Execution uses deterministic plan order. `skip` is recorded in the `sync`
result and does not invoke live mutation. `save` and `apply` choices may compose
the existing per-setting live mutation path. If one chosen mutation fails,
`sync` stops remaining mutations and reports already executed, failed, skipped,
blocked, and `not_attempted` items explicitly. This tranche does not claim
cross-item atomic rollback; separate underlying run IDs and backup refs must be
visible in the aggregate result when execution happens.

`sync` must never choose save/apply because of a recommendation alone. Conflict,
no-baseline, and opaque-changed states require an explicit user or CLI choice.
Unsupported and blocked states remain blockers; they are not silently skipped
or converted into inferred mutations.

### Advanced authoring commands

Advanced commands may exist outside the normal path:

```text
dotfiles-manager recipe list
dotfiles-manager recipe discover [target]
dotfiles-manager recipe explain <target>
dotfiles-manager app create <target-id>
dotfiles-manager app edit <target-id>
dotfiles-manager app validate <target-id>
dotfiles-manager app test <target-id> --roundtrip
```

`recipe list`, `recipe discover [target]`, and `recipe explain <target>` are
included in the MVP as read-only advanced commands. `recipe list` shows static
bundled target metadata and does not resolve active profile selection or inspect
installation state. `recipe discover` is the explicit opt-in live metadata
probe for target install/config state. `recipe explain` explains target support,
selected settings, settings groups, resources, drivers, lifecycle policy,
redaction behavior, support levels, and capability limits without reading live
target state.

Advanced app-authoring commands are for recipe authors and power users, not the
normal app-management happy path. They define and test local recipe metadata
only. Normal users should still start with `add <target>` for supported bundled
targets.

### `app create <target-id>` local recipe draft contract

`app create` creates or previews a local recipe scaffold:

```text
dotfiles-manager app create <target-id>
  --template file|selected-value|native-export
  [--from-path <path>]
  [--display-name <name>]
  --setting <setting-id>
  --setting-label <label>
  [--driver file|ini-file|json-file|yaml-file|toml-file|plist-file]
  [--selector <selector>]
  --scope-default shared|user|machine|machine-user
  --lifecycle allowed|warn|blocked|ask-to-quit|quit-if-running|block-if-running|reopen-if-stopped-by-tool
  [--dry-run]
  [--json]
```

`<target-id>` is a public target ID using the lower-case grammar from
`00-vocabulary.md`. It is not a setting ref, URI, filesystem path, or display
name. `app create` fails with exit code `2` when `<target-id>` collides with a
bundled canonical target ID or bundled alias, because bundled recipes remain
authoritative and a shadowed local recipe would be misleading.

The command may create only repository files under:

```text
recipes/local/<target-id>/recipe.yaml
recipes/local/<target-id>/README.md
recipes/local/<target-id>/fixtures/README.md
```

It must not add the target to profile layers, write desired artifacts, create
trust records, write ledgers or backups, inspect live values, touch live app
paths, run native commands, or mark the recipe trusted. Existing local recipe
directories fail closed; overwrite, repair, adoption, and merge behavior are
out of scope for this contract.

`--template native-export` means a declarative native-export candidate. It must
not imply arbitrary app integration, arbitrary script execution, or trusted
native operations. A local recipe cannot make native operations executable by
setting `reviewed: true`; future write execution still requires local trust
evidence and write-surface matching from `09-security-redaction-trust.md`.

`--from-path` records only a named-location/path shape. It must not read file
contents. For the MVP authoring surface, paths should be expressible through a
safe named location such as `home`. The command rejects absolute paths outside
allowed roots, path traversal, backslashes, empty path segments, symlink/path
escapes, and any input that would require storing a raw machine-local absolute
path in `recipe.yaml`.

The #119 implementation is deliberately non-interactive. Template-specific
required choices must be supplied as flags; otherwise the command exits `2` with
stable diagnostics. `--yes` and `--non-interactive` are not part of the #119
command surface because there are no prompts to approve. Future interactive
creation may add exit `4` missing-choice behavior without changing the generated
layout.

Template-specific requirements are:

| Template | Required non-interactive fields | Generated validity target |
| --- | --- | --- |
| `file` | `--from-path`, `--setting`, `--setting-label`, `--scope-default`, `--lifecycle` | Validate-ready whole-file draft. Driver is `file`; `--driver` is rejected unless it is also `file`. |
| `selected-value` | `--from-path`, `--driver`, `--selector`, `--setting`, `--setting-label`, `--scope-default`, `--lifecycle` | Validate-ready selected-value draft when the selector parses for the chosen driver. `--driver file` is rejected. |
| `native-export` | `--setting`, `--setting-label`, `--scope-default`, `--lifecycle` | Declarative draft only. The generated native operation has `reviewed: false` and an intentionally invalid placeholder executable, and is expected to fail `app validate` until the author replaces and reviews the metadata. Roundtrip is unsupported. |

For `--selector`, the MVP authoring CLI accepts a dot-separated path of literal
key segments for JSON, YAML, TOML, and plist selected-value drivers. INI
selectors use `section.key`. Escaping, array indexes, wildcards, filters,
JSONPath, jq syntax, and partial expressions are not part of this authoring
contract.

`app create --json` uses:

```yaml
schema: dotfiles-manager.v2.app.create
schemaVersion: 1
command: app.create
runId: app-create
dryRun: false
summary:
  status: changed | ok | blocked | error
  planned: 0
  written: 0
  unchanged: 0
  blocked: 0
  failed: 0
appCreate:
  target:
    id: local-file-demo
    displayName: Local File Demo
    recipeRef: recipe://local/local-file-demo
  template: file | selected-value | native-export
  files:
    - kind: recipe | readme | fixtures-readme
      path: recipes/local/local-file-demo/recipe.yaml
      action: create | planned
  nextActions:
    - dotfiles-manager app validate local-file-demo
diagnostics: []
error: null
```

Paths are repository-relative. JSON and text output must not include raw live
values, raw home-directory paths when a named location can be shown, timestamps,
durations, captured output, secrets, or local state-root paths.

### `app edit <target-id>` editable-path contract

`app edit` is a cross-platform helper that resolves editable local recipe files:

```text
dotfiles-manager app edit <target-id> [--print-path] [--json]
```

It does not launch `$EDITOR` in the MVP contract. Text output should say which
file to edit. `--print-path` is script-friendly and emits only the primary
repository-relative path, normally `recipes/local/<target-id>/recipe.yaml`, with
no prose. `--json` emits structured path metadata.

`app edit` must not validate or modify the recipe, create missing files, create
trust records, inspect live state, or open a platform editor. If the target is
bundled-only, unknown, ambiguous, missing as a local recipe, or unsafe to
resolve, the command fails deterministically.

`app edit --json` uses:

```yaml
schema: dotfiles-manager.v2.app.edit
schemaVersion: 1
command: app.edit
runId: app-edit
summary:
  status: ok | error
  files: 1
  failed: 0
appEdit:
  target:
    id: local-file-demo
    recipeRef: recipe://local/local-file-demo
  editableFiles:
    - kind: recipe
      path: recipes/local/local-file-demo/recipe.yaml
      primary: true
diagnostics: []
error: null
```

### `app validate <target-id>` local recipe validation contract

`app validate` validates local recipe metadata and fixture manifests:

```text
dotfiles-manager app validate <target-id> [--json]
```

It reads recipe metadata and fixture manifests only. It must not read live app
values, run native commands, run drivers against user paths, write temp state
except ordinary parser scratch, create trust records, update ledgers/backups,
write desired artifacts, or mark anything trusted.

Validation covers recipe schema, IDs, named locations, resources, driver
bindings, selectors, scopes, lifecycle declarations, sensitivity/redaction
metadata, native-operation declarations, fixture manifests, and write-surface
trust implications. It also fails closed when the requested `<target-id>` is a
bundled canonical ID or alias, when the loaded recipe's internal `target` does
not match `<target-id>`, or when the local recipe metadata path includes
symlink/path escapes. Untrusted local write-capable recipes are a warning or
advisory, not a validation failure: a recipe can be structurally valid while
remaining blocked for future writes until local trust evidence exists under the
local state root.

Validation exits `2` for invalid schema, IDs, drivers, selectors, named
locations, lifecycle declarations, arbitrary scripts, native-operation metadata,
or fixture manifests. It exits `5` only when metadata cannot be safely read or
rendered, such as unsafe symlink/path situations or redaction-blocked metadata.
It must not use exit `5` merely because the local recipe is not trusted yet.

`app validate --json` uses:

```yaml
schema: dotfiles-manager.v2.app.validate
schemaVersion: 1
command: app.validate
runId: app-validate
summary:
  status: ok | blocked | error | partial
  checked: 0
  warnings: 0
  blocked: 0
  failed: 0
appValidate:
  target:
    id: local-file-demo
    recipeRef: recipe://local/local-file-demo
  trust:
    localTrustState: not-checked
    writeTrustRequired: true
    writeSurfaceFingerprint: redacted-or-stable-fingerprint
  fixtures:
    - name: basic
      state: valid | invalid | blocked | skipped
diagnostics: []
error: null
```

### `app test <target-id> --roundtrip` fixture contract

`app test --roundtrip` tests a local recipe against synthetic fixtures only:

```text
dotfiles-manager app test <target-id> --roundtrip [--fixture <name>] [--json]
```

It must never touch the user's actual config path, desired repository root,
backups, ledgers, trust records, or live app state. The command copies fixture
trees into a manager-owned temp directory, overrides named locations to point at
those temp fixture roots, and runs deterministic driver operations there.

MVP roundtrip support is limited to deterministic `file` resources and
selected-value resources backed by `ini-file`, `json-file`, `yaml-file`,
`toml-file`, or `plist-file`. `file-tree`, native export/import, lifecycle
stop/reopen actions, trust-record reads, live desired-root writes, ledgers, and
backups are explicitly outside this tranche. Native-export candidates are
validate-only and must produce a skipped/no-runnable fixture result; local
recipes must not execute native commands during `app test --roundtrip`.

Roundtrip uses temp-root driver operations after recipe metadata has passed
app-authoring safety checks. It must not expose a user-configurable trust bypass:
fixture simulation is internal to `app test --roundtrip`, does not read or
create local trust records, and does not grant any trust to live save/apply
commands.

Fixture layout is co-located with the local recipe for discoverability:

```text
recipes/local/<target-id>/fixtures/
  README.md
  roundtrip/
    <fixture-name>/
      manifest.yaml
      input/
        live/
        desired/
      expected/
        desired/
        live/
```

`manifest.yaml` is strict YAML with duplicate keys and unknown fields rejected:

```yaml
schema: dotfiles-manager.v2.app.roundtrip-fixture
schemaVersion: 1
target: local-file-demo
name: basic        # optional; when present, must match the directory name
synthetic: true    # required; real copied app data is not allowed
modes: [save, apply] # optional; defaults to both, order-independent
settings: [config]   # optional; defaults to all recipe settings
subjects:            # optional; defaults shown here
  user: fixture-user
  machine: fixture-machine
```

Omitting `--fixture` runs every fixture directory under `fixtures/roundtrip/` in
lexical order. If there are no fixture directories, the command fails with
`app.test.fixture.none`; it does not auto-create fixture data. `--fixture <name>`
runs exactly one fixture and fails if that fixture is absent or unsafe.

`input/live/` is the simulated live target tree. Live files are addressed by
named recipe locations:

```text
input/live/locations/<location-id>/<resource.path>
expected/live/locations/<location-id>/<resource.path>
```

`input/desired/` is a simulated desired root whose paths mirror canonical
desired layout. Supported roots are:

```text
shared/-/targets/<target-id>/...
user/<fixture-user>/targets/<target-id>/...
machine/<fixture-machine>/targets/<target-id>/...
machine-user/<fixture-machine>/<fixture-user>/targets/<target-id>/...
```

For whole-file resources, desired artifacts are stored under
`artifacts/<setting-id>`. For selected-value resources, settings are stored in
`settings.yaml` and addressed through the corresponding
`desired://.../settings#<setting-id>` URI. `expected/desired/` is the
expected desired tree after a simulated save. `expected/live/` is the expected
live tree after applying the desired fixture.

Fixtures must be synthetic or sanitized. Users must not copy live app data into
fixtures when it contains personal, secret, account-bound, opaque, native-export,
or machine-local payloads. Text output, JSON output, diagnostics, fixture
manifests, and test reports must remain redaction-safe and must not print raw
fixture values by default.

`app test --roundtrip --json` uses:

```yaml
schema: dotfiles-manager.v2.app.test-roundtrip
schemaVersion: 1
command: app.test-roundtrip
runId: app-test-roundtrip
summary:
  status: ok | partial | blocked | error
  cases: 0
  passed: 0
  skipped: 0
  blocked: 0
  failed: 0
appTestRoundtrip:
  target:
    id: local-file-demo
    recipeRef: recipe://local/local-file-demo
  fixtures:
    - name: basic
      status: passed | skipped | blocked | failed
      reason: stable-reason-code
      modes: [save, apply]
      cases:
        - setting: config
          resource: config-file
          driver: file
          save: passed | skipped | blocked | failed
          apply: passed | skipped | blocked | failed
diagnostics: []
error: null
```

### App authoring exit codes and examples

| Command | Exit 0 | Exit 2 | Exit 4 | Exit 5 | Exit 6 |
| --- | --- | --- | --- | --- | --- |
| `app create` | Files created, or valid dry-run plan. | Invalid target ID, collision, existing local recipe, unsupported template, invalid path shape. | Required interactive choices unavailable. | Unsafe repository path, symlink/path escape, safety policy blocker. | Not expected for single-target create. |
| `app edit` | Path metadata printed. | Unknown target, bundled-only target, invalid ref, missing local recipe. | Not expected. | Unsafe recipe path or metadata-render safety block. | Not expected. |
| `app validate` | Structurally valid, including untrusted-local warnings. | Invalid recipe/schema/selector/fixture/native metadata. | Not expected. | Cannot safely read/render metadata. | Optional only if multiple independent fixtures or sections produce mixed results. |
| `app test --roundtrip` | All required runnable cases pass. | Invalid recipe/fixture, missing fixture data, no runnable cases, or all-failing roundtrip mismatch. | Not expected; omitted `--fixture` runs all fixtures. | Fixture path escape, unsafe symlink/special file, attempted native execution, redaction/safety blocker. | Some independent cases pass and others fail/block/skip. |

Recipe-authoring examples use neutral local demo target IDs and are examples
only, not supported bundled apps.

Whole-file recipe draft:

```bash
dotfiles-manager app create local-file-demo \
  --template file \
  --from-path ~/.config/local-file-demo/config.txt \
  --display-name "Local File Demo" \
  --setting config \
  --setting-label "Config file" \
  --scope-default user \
  --lifecycle allowed
```

Selected-value recipe draft:

```bash
dotfiles-manager app create local-yaml-demo \
  --template selected-value \
  --from-path ~/.config/local-yaml-demo/config.yaml \
  --driver yaml-file \
  --selector preferences.theme \
  --setting preferences.theme \
  --setting-label "Theme" \
  --scope-default user \
  --lifecycle allowed
```

Native-export candidate draft:

```bash
dotfiles-manager app create local-native-export-demo \
  --template native-export \
  --display-name "Local Native Export Demo" \
  --setting settings \
  --setting-label "Settings export" \
  --scope-default machine-user \
  --lifecycle blocked
```

The file and selected-value examples create validate-ready recipe scaffolds for
synthetic local fixtures. The native-export example creates declarative candidate
metadata only: it includes placeholder command metadata with `reviewed: false`
and an intentionally invalid executable placeholder, must not be treated as
executable, and is expected to fail `app validate` until a recipe author replaces
and reviews the native metadata.

### `recipe list` read-only contract

`recipe list` emits static bundled registry metadata only. It must not inspect
live apps, desired artifacts, profile selection, app installation state, or
native export/import commands. User-local recipe IDs that collide with bundled
canonical IDs or aliases may produce warning diagnostics, but bundled registry
metadata remains authoritative for the bundled entry.

Text output must include deterministic target rows with target ID, source,
trust status, support level, capability, platform support, and aliases.

### `recipe discover` read-only contract

`recipe discover [target]` is the explicit read-only install/config discovery
surface. Unlike `recipe list`, it may inspect live target-owned metadata, but it
must never read file contents, inspect desired artifacts, resolve active profile
selection, query backups or ledger state, launch apps, run package managers,
call native export/import commands, or execute target binaries. Command probes
are PATH lookups only. Config probes are lstat-style metadata checks of declared
live resource paths only.

Discovery output must be deterministic in structure and ordering. It uses
`command: recipe.discover`, `runId: recipe-discover`, `schemaVersion: 1`,
canonical bundled target IDs, stable state strings, and stable diagnostic codes.
Rows are sorted by canonical target ID; command probes, config probes, aliases,
resources, and diagnostics are sorted deterministically. Discovery output must
not include timestamps, durations, random IDs, raw config contents, desired
artifact paths, backup payloads, or ledger payloads.

Summary state values are:

- `unsupported-platform` when the target's declared platform support excludes
  the current OS; discovery skips command/config probes in this state;
- `ambiguous` when a relevant command or config probe cannot be classified
  safely, for example stat permission errors, malformed locations, wrong path
  type, command lookup errors, or a symlink rejected by that resource;
- `config-present` when at least one deduplicated declared config resource path
  exists with the expected metadata type;
- `installed` when a declared command probe is found but no config path exists;
- `config-missing` when relevant config probes are all missing and no command is
  found;
- `not-applicable` for targets with no command/config probes such as
  `custom.files`.

JSON includes separate axes as well as the summary state:

```yaml
command: recipe.discover
discovery:
  targets:
    - id: git
      state: config-present
      platformState: unknown | supported | unsupported
      binaryState: installed | missing | ambiguous | not-applicable
      configState: config-present | config-missing | ambiguous | not-applicable
      commandProbes:
        - kind: command
          command: git
          state: installed | missing | ambiguous
      configProbes:
        - kind: config
          id: home:.gitconfig:file
          locationId: home
          path: .gitconfig
          expectedType: file
          actualType: file
          state: present | missing | ambiguous
          resources: [user-email, user-name]
      diagnostics: []
```

For file resources, discovery expects a regular file. For file-tree resources,
discovery checks only the declared root path and expects a directory. It must not
expand include/exclude globs, recurse into trees, or inspect child paths. It
uses lstat semantics. Resources that reject leaf symlinks, such as SSH config,
return `ambiguous` with a stable symlink diagnostic when the declared path is a
symlink; other symlinks are classified as present without following the target.

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

The reviewed native-operation runner may be invoked only through an explicitly
selected `native-export` resource whose recipe has trusted reviewed operation
metadata. `diff` and `save`/`save --dry-run` may run the reviewed export into
manager-owned temp staging so they can compare opaque payload metadata.
`status` must not run native exports in this tranche. For import-capable native
resources, `apply --dry-run` must not run export/import/verify commands by
default; it validates the desired native artifact and reports the native apply
plan. `apply` without `--yes` stops with confirmation-required exit `4` before
backup/export/import/verification, including `--json` and `--non-interactive`
invocations. `apply --yes` may run native apply only for a
trusted resource with the explicit `pre-apply-export` backup policy and
`post-import-export-hash` verification policy; native trust, lifecycle, backup,
import, and verification failures are safety/execution blockers and use exit
`5`, not confirmation exit `4`. Commands must not invoke native operations
merely because a recipe declares them.

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
    capability: inspect-only | read-only | read-write | export-only | never  # import-only is reserved
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
| `--profile <layer>` | For read/preview commands, add an explicit profile layer to the active stack overlay. Repeatable. For `add`, choose the single destination profile layer to update. |
| `--scope <scope>` | Choose `shared`, `user`, `machine`, or `machine-user` when saving. |
| `--machine-id <id>` | Explicit machine identity input for bootstrap or transient read-only resolution; must not override an existing local machine identity. |
| `--user-id <id>` | Explicit user identity input for bootstrap or transient read-only resolution; must not override an existing local user identity. |
| `--dry-run` | Do not mutate desired repo artifacts or live target state. |
| `--json` | Emit stable machine-readable result data. |
| `--non-interactive` | Never prompt. Fail if input is required. |
| `--yes` | Accept safe default prompts, never safety blockers. |
| `--verbose` | Text-mode technical detail flag. In the current #165 implementation this is wired for selected-preview `status`, `diff`, `save`, and `apply`; later issues may extend it to other v2 commands. |

`--dry-run` may read current state, run a declared read-only native export for
`diff`/`save` on a selected trusted `native-export` resource, and write
manager-owned temporary state. It must not change desired artifacts or live
state. For native apply, dry-run is preview-only by default: it validates the
desired native artifact and policy but does not run backup export, import, or
post-import export. Live native import receives only a manager-owned temp copy of
the desired payload; import operations must not expose live `location` roots
through typed input, output, temp, argv, or environment channels. If a native
export is review-required because it may be
opaque, account-bound, large, or privacy-sensitive, the command must fail closed
before executing that export unless the user has explicitly opted in, such as
with `--yes` where that flag is available.

`--machine-id` and `--user-id` are advanced identity inputs. They must validate
against the identity grammar in `03-profile-and-scope-resolution.md`. If a
local identity record already exists, a conflicting flag value must fail with a
clear adoption/rename diagnostic rather than silently overriding the record.
Read-only and dry-run commands may use these flags only transiently and must not
persist identity records. `init` and other commands that are explicitly allowed
to bootstrap local manager state may persist them after validation and the
prompt/non-interactive rules in `03-profile-and-scope-resolution.md`.


### v2 output tiers for selected-preview commands

Issue #165 defines the common selected-preview output-tier contract for
`status`, `diff`, `save`, and `apply`. It does not finalize every command's
copy, and it does not claim `--verbose` support for `init`, `add`, `list`,
`recipe`, `backup`, `restore`, `sync`, app-authoring commands, or legacy v1
commands until those command renderers are explicitly wired and tested.

Default text output is the human-first contract. It must answer, without
requiring internal schema vocabulary:

- which selected setting(s) were checked;
- whether the command is a dry run or a confirmed write;
- whether anything changed, and whether writes would affect repo desired state,
  live app/config state, or only manager-owned local state;
- the user-level live path or source, such as `$HOME/.gitconfig [user] email`;
- the user-level repo path for desired state, such as
  `desired/user/<user>/targets/git/settings.yaml`;
- whether current/desired values exist while keeping raw values hidden;
- first-run review uncertainty such as no previous baseline in plain language;
- the blocked reason and one safe next command when an item cannot proceed.

Default text must keep internal details out of the main path unless they are the
user-facing ref needed to run the next command. In particular, default text must
not require understanding `resource=`, `driver=`, `selector=`, `desired://`,
`state://`, raw planner states such as `missing-desired`, raw actions such as
`would-promote`, or the raw `no-baseline` flag.

For selected-preview blockers, #165 establishes the baseline plain-language
blocked-output behavior: state why the command cannot proceed, confirm that no
files changed, and provide a safe next command or diagnostic path. #166/#167 may
refine exact command wording later, but they must preserve that baseline.

Verbose text output is default text plus a separated technical-details section.
It is not a behavior toggle and must not change planning, write safety, backup,
restore, lifecycle, trust, or redaction behavior. Verbose text may include
profile stack/layer details, setting refs, desired/state URIs, resource IDs,
driver IDs, location IDs, selectors, raw planner states/actions, run/ledger refs,
backup refs, lifecycle records, and diagnostic codes/messages. Verbose text must
preserve default redaction behavior: it may show technical identifiers, but must
not print raw managed values or secret-bearing payload bytes.

JSON output is the stable machine contract. `--json --verbose` must preserve the
existing JSON schema and stdout shape: JSON mode writes only the existing JSON
document to stdout, and verbose prose/diagnostics are suppressed rather than
appended or moved to stderr. JSON field names, enum values, refs, and redaction
policy are unchanged by #165.

For aggregate selected-preview runs, default text may be less polished until the
#171 coverage map and #166/#167 copy work are complete, but it must still show
changed/unchanged/blocked counts, per-item human summaries, dry-run/write status,
blocked reasons, and a safe next action without leaking raw planner labels into
the main path.

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

Lifecycle prompts are narrower than general write confirmation. `--yes` may
authorize managed `quit-if-running` and `reopen-if-stopped-by-tool` behavior
when the recipe declares a supported lifecycle target. It must not override
`blocked`, `block-if-running`, missing/unsupported lifecycle targets, ambiguous
detection, failed quit, still-running recheck, or unsupported controller
behavior. `ask-to-quit` remains manual: JSON/non-interactive mode must block
instead of pretending the app was closed, `--yes` must block instead of
auto-answering the manual step, and text mode without `--yes` may ask the user
to quit manually then re-check before writing.

Native apply blockers are safety blockers, not prompts. Missing or unsupported
native apply backup/verification policy, untrusted or changed local native
recipe evidence, lifecycle blockers, backup creation failure, import failure,
and post-import verification failure must return safety exit `5` or a
partial-success exit `6` when independent items also succeeded. `--yes`
confirms the reviewed action; it must not override those blockers.

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
command: init | add | list | status | diff | save | apply | sync | backup.list | restore | migrate | recipe.explain | recipe.discover
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
dotfiles-manager add git --yes
dotfiles-manager add nvim --yes
dotfiles-manager add ssh --yes
dotfiles-manager add starship --yes
dotfiles-manager add tmux --yes
dotfiles-manager add zsh --yes
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
with `dotfiles-manager add git --yes` or by an equivalent profile-layer entry.
The bundled Git runtime manages only `~/.gitconfig` `[user] email` and `[user] name`.
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

### Save one tmux config file

For the current MVP tranche, the bundled `tmux` runtime manages only selected
whole-file user config refs:

- `tmux:home.conf` -> `~/.tmux.conf`
- `tmux:xdg.conf` -> `~/.config/tmux/tmux.conf`

These refs are explicit alternatives from tmux's own user-config lookup model.
The manager must not imply that both user config files are loaded together. It
syncs only the selected file and does not decide which file tmux loaded, merge
configs, inspect `config_files`, or validate running server state.

`tmux:home.conf` uses `scopeDefault: user` and the named `home` location with
default `~`. `tmux:xdg.conf` uses `scopeDefault: user` and the named `config`
location with default `~/.config`. Both use `driver: file`, `artifactForm:
file`, `sensitivity: personal`, `redaction: redacted-for-display`, and no
selectors or include/exclude globs.

The desired artifact path for a user-scoped tmux file is:

```text
desired/user/<user>/targets/tmux/artifacts/<setting-id>
```

For example, `--user-id leon` and `tmux:home.conf` write
`desired/user/leon/targets/tmux/artifacts/home.conf`, with URI
`desired://user/leon/targets/tmux/artifacts/home.conf`. It must never be stored
in `settings.yaml`.

The command behavior follows the generic selected whole-file resource path:

- `status` and `diff` read metadata only and must not emit raw config bytes;
- `save --dry-run` previews importing the selected live config file into the
  desired artifact;
- `save --yes` writes the desired artifact and verifies it;
- `apply --dry-run` previews copying the desired artifact back to the selected
  live config file;
- `apply --yes` writes a pre-apply backup, applies the desired artifact, and
  verifies it.

tmux loads user configuration according to its own lookup rules when the server
starts. Existing servers/sessions may require manual reload or restart before
they observe changes. Save/apply planning must emit a non-blocking warning
diagnostic with stable code:

```text
tmux.lifecycle.manual-reload
```

`status` and `diff` must not emit this write warning. The warning must not
block dry-run or `--yes` execution by itself.

Missing-state behavior is normative for this slice:

- missing named location root blocks status/diff/save/apply;
- missing live config file blocks save and must not delete/tombstone desired
  state;
- missing live config file also blocks apply in this whole-file slice rather
  than creating the config file or intermediate directories;
- missing desired artifact blocks apply and must not delete/tombstone live
  state.

The bundled tmux recipe must not manage system configuration, server sockets,
clients, sessions, windows, panes, runtime state, plugin installation, plugin
clones/caches, generated plugin state such as resurrect/continuum session-save
files, history, logs, pid files, temporary files, manual reload actions
(`tmux source-file`), server restart, session mutation, command parsing,
semantic validation, plugin validation, or secret scanning.

Unsupported tmux refs outside `tmux:home.conf` and `tmux:xdg.conf` must remain
unsupported settings and must not be resolved to filesystem paths or read.

### Save one SSH config file

For the current MVP tranche, the bundled `ssh` runtime manages only one
selected whole-file OpenSSH user config ref:

- `ssh:config` -> `~/.ssh/config`

`ssh:config` uses `scopeDefault: user` and the named `home` location with
default `~`. It uses `driver: file`, `artifactForm: file`, `sensitivity:
personal`, `redaction: redacted-for-display`, `lifecycle: allowed`, no
selectors, and no include/exclude globs. It also enables the opt-in file
content safety policy `ssh-config-obvious-secrets` and a save/apply review
warning. The warning is **not** a lifecycle warning because SSH config changes
do not require the manager to stop ssh, ssh-agent, keychain, hardware tokens, or
sessions.

The desired artifact path for a user-scoped SSH config file is:

```text
desired/user/<user>/targets/ssh/artifacts/config
```

For example, `--user-id leon` and `ssh:config` write
`desired/user/leon/targets/ssh/artifacts/config`, with URI
`desired://user/leon/targets/ssh/artifacts/config`. It must never be stored in
`settings.yaml`.

The command behavior follows the generic selected whole-file resource path, with
additional SSH safety gates:

- `status` and `diff` read metadata only and must not emit raw config bytes;
- `save --dry-run` previews importing the live `~/.ssh/config` file into the
  desired artifact;
- `save --yes` writes the desired artifact and verifies it only after scanning
  the live bytes that would be persisted;
- `apply --dry-run` previews copying the desired artifact back to
  `~/.ssh/config`;
- `apply --yes` scans the desired artifact and the current live file before
  writing the live file or creating the raw pre-apply backup payload.

Save/apply planning must emit this non-blocking content-review warning:

```text
ssh.config.review-required
```

The warning tells users to review `Include`, `IdentityFile`, `CertificateFile`,
`LocalCommand`, `ProxyCommand`, and `Match exec` directives and not to store key
material or tokens inline. `status` and `diff` must not emit this write warning.

The `ssh-config-obvious-secrets` content safety policy must block save/apply
before durable writes or backup creation when it detects obvious excluded
material in the byte stream that would be persisted. Stable blocking diagnostic:

```text
ssh.config.excluded-content
```

Diagnostics must be metadata-only: code, public ref, resource id, operation,
path, detector category, and pattern id are allowed; matched text, surrounding
line content, token prefixes, raw config snippets, and entropy samples are not.
The policy must detect obvious private-key headers, token-like secrets covered
by the generic secret policy, SSH public key lines, OpenSSH certificate key
lines, known_hosts-style lines including hashed hosts, and authorized_keys-style
lines. Normal config directives such as `IdentityFile ~/.ssh/id_ed25519` must
not cause the referenced key file to be read or block merely because the
directive names a key path.

The bundled SSH recipe must reject symlinked `~/.ssh/config` before reading or
writing the target:

```text
ssh.config.symlink-unsupported
```

Missing-state behavior is fail-closed for this slice:

- missing named home location blocks status/diff/save/apply;
- missing live `~/.ssh/config` blocks save and must not delete/tombstone desired
  state;
- missing live `~/.ssh/config` also blocks apply rather than creating the file
  or intermediate directories;
- missing desired artifact blocks apply and must not delete/tombstone live
  state.

The bundled SSH recipe must not walk `~/.ssh`, resolve `Include` files, read
`IdentityFile`, `CertificateFile`, or `UserKnownHostsFile` targets, manage
private keys, public keys, certificates, host keys, `known_hosts`,
`authorized_keys`, ssh-agent/keychain/hardware-token state, sockets, control
sockets, multiplexed connection state, generated state, permission repair, key
generation/import/export, SSH installation, network access, `ssh -G`
validation, or command execution.

Explicit excluded refs such as `ssh:keys`, `ssh:private-keys`,
`ssh:public-keys`, `ssh:identity`, `ssh:known_hosts`, `ssh:known-hosts`,
`ssh:authorized_keys`, `ssh:authorized-keys`, `ssh:agent`, `ssh:sockets`,
`ssh:control-sockets`, `ssh:config.d`, `ssh:includes`, `ssh:certificates`, and
`ssh:host-keys` must return:

```text
ssh.ref.excluded
```

They must not resolve to filesystem paths, list directories, or read files.

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
package-manager actions, use runtime RPC, discover `NVIM_APPNAME` or process
`XDG_CONFIG_HOME` alternatives, execute or lint Lua/Vimscript, or perform secret
scanning. Its bundled `config` location default is the HOME-relative
`~/.config`, so `nvim:config` resolves to `~/.config/nvim` unless an explicit
named location override is supplied. Setting `XDG_CONFIG_HOME` in the manager
process must not change bundled default discovery or silently broaden writes.

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
