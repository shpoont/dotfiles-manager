---
owner: Product + Core Engineering
status: Concept / proposal
last-updated: 2026-06-04
canonical-source: docs/internal/scope/product-concept-v2.md
---

# Product concept v2: local settings manager

## Status

This document captures the proposed next product direction for
`dotfiles-manager`. It is **not** a current implementation contract and should
not override the existing v1 specs under `docs/internal/specs/` or
`docs/internal/contracts/` until decisions are explicitly promoted there.

The current product remains a config-driven dotfiles sync tool. This document
proposes a future architecture that generalizes dotfiles into managed local
configuration resources for apps, CLI tools, services, and system features.

## Promotion gate and document conventions

This document can become the source for formal specs only after it contains
implementation-grade contracts for the following areas:

1. CLI behavior: operands, prompts, flags, `--json`, dry-run behavior,
   non-interactive mode, mixed results, and exit codes.
2. Status and conflict state machine: how desired, current, and last-applied
   state become user-visible status groups and allowed actions.
3. Mutation transactions: lock, preview, backup, write, verify, ledger commit,
   rollback limits, restore, and partial failure behavior.
4. Schema boundaries: config, profile, desired manifest, artifact metadata,
   recipe, resource, preview, ledger, and backup metadata.
5. Driver contracts: deterministic read/normalize/diff/apply/verify behavior,
   selector validation, redaction, and error taxonomy.
6. Migration and compatibility: v1 `syncs:` adapter behavior and parity tests.
7. Security, trust, and platform assumptions: local recipe trust, symlink/path
   safety, command execution boundaries, and supported platforms.

YAML and command snippets in this document are **illustrative** unless a section
explicitly says **normative for the MVP**. Illustrative examples show intended
shape and vocabulary; they must not be treated as final schemas until promoted
into `docs/internal/specs/` or `docs/internal/contracts/`.

## Executive summary

The direction is worth pursuing only if the user-facing product stays much
simpler than the internal architecture.

The product story should be:

> Choose the apps and tools to manage. The manager stores their relevant settings
> in a config repo, shows what changed, saves this machine's current settings,
> and applies saved settings onto another machine.

The normal command surface should be small:

```bash
dotfiles-manager add <target...>                 # start managing apps/tools
dotfiles-manager list                            # show managed apps/tools
dotfiles-manager status [target-or-setting]      # show what changed
dotfiles-manager diff [target-or-setting]        # show readable changes
dotfiles-manager save [target-or-setting]        # current machine -> repo
dotfiles-manager apply [target-or-setting]       # repo -> current machine
dotfiles-manager sync                            # guided save/apply flow
```

Use `save` and `apply` as the primary v2 verbs. They are clearer than
`import/export` for normal users, and friendlier than `capture/deploy`.
Compatibility aliases can exist later, but the product story should say:

```text
save  = save this machine's settings into the repo
apply = apply saved repo settings onto this machine
sync  = guided conflict-aware choice between save/apply/skip
```

The default mental model should be:

```text
Managed app/tool
  -> saved settings in the repo
  -> current settings on this machine
  -> status / diff
  -> save or apply
```

Profiles, recipes, resources, drivers, artifacts, ledgers, raw captures,
rendered temporary files, and internal plans remain implementation concepts.
Normal users should not need to understand plist keys, defaults domains, app
support folders, file globs, argv arrays, normalization rules, or app-specific
storage details unless they intentionally enter advanced mode.

The internal architecture can remain richer:

```text
ProfileStack
  -> ResolvedProfile
  -> Target
  -> Recipe
  -> SettingsGroup
  -> Resource
  -> Driver
  -> DesiredArtifact
  -> InternalPlan/ChangePreview
  -> StateLedger
```

The most important safety principle is:

> AI may propose app support. Deterministic, reviewed drivers execute validated
> recipes. Unknown state defaults to do-not-manage.

## End-user convenience requirements

Correctness is not enough. The product should feel like a manager for supported
apps and tools, not like a framework that asks users to model local state.

The convenient happy path should be:

```text
1. Initialize or point the manager at a repo.
2. Add supported apps/tools by name.
3. Accept recommended safe settings.
4. Review status/diff in user-meaningful terms.
5. Save current settings or apply saved settings.
```

Convenience requirements:

- `add` should detect whether the target is installed and show recommended safe
  settings before asking the user to edit config.
- Supported targets should work from target names first: `git`, `nvim`,
  `raycast`, `cobona`. Users should not need recipe IDs, drivers, locations,
  artifact paths, or URI schemes for supported cases.
- The manager should recommend scopes and ask only when the choice is ambiguous.
  Prompts should use labels such as “for me” or “for this machine,” while still
  storing the precise `user` or `machine` scope internally.
- `status` should group work by next action: changed here, ready to apply,
  missing saved settings, conflicts, and blocked items.
- `diff` should be readable when possible and honest when not possible. Opaque
  native exports should show hash/metadata changes and the limitation, not fake
  field-level diffs.
- `save` should create or update the conventional desired artifacts after a
  preview. Users should not need to create the desired directory structure
  manually.
- `apply` should explain what will change, back up first when possible, and give
  a clear recovery pointer.
- If an app must be quit before apply, the manager should say so in user terms,
  offer to quit/reopen when safe, and skip cleanly if the user declines.
- Advanced details must remain accessible through `--verbose`, `--json`,
  `recipe explain`, ledgers, and docs. Hiding details for convenience must not
  hide uncertainty, skipped state, or safety blockers.

A good command should answer three user questions quickly:

```text
What changed?
What can I safely do next?
What will be backed up or skipped?
```

## Current product baseline

`dotfiles-manager` currently synchronizes managed dotfiles between:

- a repository-local source tree, and
- one or more `$HOME`-relative target paths.

The v1 mental model is:

```text
sync entry -> source path -> target path -> status/diff/deploy/import
```

That model is strong for classical dotfiles because it is explicit,
deterministic, and easy to audit. It becomes awkward for modern workstation
state because not every setting is a plain file under a known path. Some state
lives in:

- individual dotfiles such as `~/.zshrc`;
- config directories such as `~/.config/nvim`;
- INI/TOML/YAML/JSON files;
- plist files and CFPreferences/defaults domains;
- app export/import formats;
- launchd plists;
- app support directories;
- local databases;
- account/cloud/session state that should not be managed at all.

The next product step should keep the determinism of v1 while raising the user
abstraction above raw paths.

## Product promise

The product should promise:

- portable, reviewable, user-meaningful local settings management;
- deterministic inspect/status/diff/save/apply flows;
- safe defaults and explicit do-not-manage boundaries;
- clear change previews before writes;
- backups before writes where technically possible;
- visible, editable recipes for advanced users;
- compatibility with classical dotfiles workflows.

The product should not promise:

- full app migration;
- cloning accounts, sessions, histories, licenses, caches, or cloud state;
- universal reverse engineering of every application;
- unattended AI management of local state;
- arbitrary community automation execution;
- replacing Homebrew, Nix, MDM, backup tools, or secret managers.

The messaging should avoid saying “manage the whole app.” Prefer:

> Manage selected settings for this target.

For example, Raycast support should not mean “restore all of Raycast.” Even if
Raycast provides native export/import, support should mean “manage selected
Raycast settings through declared native artifacts,” while explicitly excluding
account, extension secrets, history, cache, and cloud state from normal
settings management.

## User-facing model

### Happy-path flows

The product should be documented around common end-user flows before explaining
profiles, recipes, or artifacts.

First setup on a configured machine:

```bash
dotfiles-manager init
dotfiles-manager add git nvim raycast
dotfiles-manager status
dotfiles-manager save
```

Expected experience:

```text
Added git
  Managing: aliases, includes
  Needs choice: user.email is personal. Save it for this user? [Y/n]
  Skipped by default: credentials

Added nvim
  Managing: config
  Skipped by default: cache, generated files, lazy-lock.json

Added raycast
  Managing: snippets, quicklinks
  Optional: settings-and-data native export is available but opaque.
```

New machine setup:

```bash
dotfiles-manager init --repo <repo-url-or-path>
dotfiles-manager list
dotfiles-manager apply --dry-run
dotfiles-manager apply
```

Expected experience:

```text
Ready to apply:
  git:aliases              shared       6 aliases
  nvim:config              shared       42 files
  raycast:snippets         shared       12 snippets

Needs your choice:
  git:user.email           this user    saved value exists for leon

Will not manage:
  ssh keys, Git credentials, Raycast account, caches, histories
```

After changing settings locally:

```bash
dotfiles-manager status
dotfiles-manager diff git:user.email
dotfiles-manager save git:user.email
```

Expected experience:

```text
Changed on this machine:
  git:user.email           this user    save or discard
  raycast:quicklinks       shared       save or view diff

Blocked:
  raycast:settings-and-data
    Raycast is running. Apply can ask to quit and reopen it.
```

Unsupported app onboarding should also start from the user's words and paths,
not from resource/driver vocabulary. A user should be able to say the app name,
point at a config file or native export command, and let the wizard generate a
validated draft. The draft may expose recipes/resources later, but the first
experience should be guided and testable.

### Core nouns

Keep the normal user vocabulary small. Product docs, onboarding, and ordinary
command output should explain only these nouns:

- **Target** — the app, CLI tool, service, system feature, or file bundle being
  managed. User-facing copy can usually say **app**, **tool**, or **setting**.
- **Setting** — the user-meaningful piece of a target that can be managed, such
  as `git:user.email`, `git:aliases`, `nvim:config`,
  `raycast:snippets`, or `cobona:user-info`. A public setting may map to one
  internal settings group when several technical resources are needed.
- **Scope** — who or what owns the saved setting. The only public scopes are
  `shared`, `user`, `machine`, and `machine-user`.
- **Saved settings** — the versioned desired state in the repository.
- **Current settings** — the state currently present on this machine.
- **Change preview** — the dry-run view shown by `status`, `diff`,
  `save --dry-run`, or `apply --dry-run`.
- **Backup** — local state captured before mutation.

Advanced/internal docs may use these additional nouns:

- **Profile layer** — a named desired-state contribution such as `global`,
  `os/macos`, `role/personal-mac`, `user/leon`,
  `machine/klm.mobile.macbook-pro`, or
  `machine-user/klm.mobile.macbook-pro/leon`. A layer is not a physical
  machine; it contributes to the stack selected for a run on a machine.
- **Profile stack** — the ordered set of profile layers used for one run. The
  stack resolves into one effective profile before diff/apply work starts.
- **Machine context** — the current host/user/platform state where a command is
  running: hostname, username, OS, architecture, installed targets, and actual
  local state. The machine context helps select and evaluate a profile but is not
  itself the profile.
- **Settings group** — an internal recipe boundary used when several technical
  resources implement one public setting. It should not become a mandatory
  concept that every user must learn.
- **Named location** — a recipe-defined live path slot such as `config`,
  `userConfig`, `data`, `state`, `cache`, `snippets`, or `extensions`. Recipes
  provide safe defaults; users may override locations for nonstandard setups.
  Named locations are live machine locations, not repository paths.
- **Desired-state artifact** — the versioned source of truth for actual desired
  settings values. Artifacts may be curated file trees, typed structured
  documents, or sanitized portable app exports.
- **Artifact binding** — the profile-layer link between a selected setting and
  the desired-state artifact that supplies its values. In normal workflows this
  should be inferred by scope and convention.
- **Recipe** — the support definition that knows where a target stores settings,
  which settings are safe, how to read/diff/apply them, and which lifecycle
  precautions are required.
- **Resource** — a recipe implementation unit handled by one driver.
- **Driver** — deterministic code that reads, normalizes, diffs, applies, and
  verifies a resource.
- **State ledger** — local per-run records of observed state, backups, hashes,
  verification, and lifecycle actions.

Use `manage:` in user config rather than forcing the public noun `programs:`.
This avoids the naming problem around whether Finder, Dock, launchd jobs, sshd,
or postgres are “programs.” Documentation can say “apps, tools, services, and
system settings.” Internally, use the term **Target**.

### Naming and reference conventions

Use one public namespace for normal commands and configuration:

```text
<target-id>
<target-id>:<setting-id>
```

The target side identifies the managed app, tool, service, or system setting.
The setting side identifies a logical setting owned by that target.

Public target IDs use kebab-case. Dots are allowed only as namespace
separators, and each dot-separated segment must still be kebab-case:

```text
git
ssh
cobona
visual-studio-code
aws-cli
macos.finder
jetbrains.idea
```

Public setting IDs use dot-separated kebab-case paths:

```text
user.email
user.name
aliases.co
ui.theme
settings-and-data
user-info
```

A public setting reference has this form:

```text
git:user.email
git:aliases.co
cobona:user.email
cobona:user-info
raycast:settings-and-data
```

Rules:

- the left side of `:` is always a target ID;
- the right side is always a logical setting or artifact ID owned by that target;
- dot notation in a setting ID means logical nesting, not necessarily a file path
  or JSON/YAML path;
- use lowercase kebab-case for public IDs;
- do not use camelCase in public IDs;
- do not encode physical filenames in setting IDs unless the file itself is the
  user-meaningful artifact;
- do not expose `desired://`, `target://`, profile-layer names, driver names, or
  recipe paths in normal CLI examples.

User IDs and machine IDs are separate namespaces. They should be stable,
portable identifiers chosen by the user or by local setup, such as `leon` or
`klm.mobile.macbook-pro`. They are not target IDs and do not need to follow the
public target namespace rules exactly, but they should avoid characters that are
unsafe in paths or URIs.

Schema field names, recipe-local resource IDs, and recipe-local named locations
are also separate internal namespaces. The public ID rule does not prohibit
schema fields such as `backupBeforeApply` or internal named locations such as
`userConfig`, but those names should not leak into normal target/setting
references.

Normal CLI examples should use public references:

```bash
dotfiles-manager save cobona:user.email --scope user
dotfiles-manager save cobona:user-info --scope shared
dotfiles-manager diff raycast:settings-and-data
```

Internal URIs are for recipes, ledgers, debug output, and advanced config. They
are defined later in this document.

### Relationships and cardinality

This is an advanced/implementer-facing model. Normal users should not need to
reason about all of these relationships, but the nouns should have explicit
relationships so the model does not imply that a machine, profile, target, or
setting are the same thing.

```text
Repository
  -> Profile*                         # available desired-state profiles

Machine context
  -> active profile stack             # ordered profile layers for this run
  -> resolved profile                  # deterministic merge result
  -> hostname / user / OS / arch
  -> installed targets and actual local state

Profile layer
  -> Target selection*                # what this layer contributes
  -> artifact binding*                # where desired values come from
  -> policy                           # writes, conflicts, backup, trust
  -> recipe pins / local recipe paths

Target selection
  -> selected Setting* or default safe settings
  -> excluded Setting*
  -> desired-state artifact binding
  -> mode                             # read-only, read-write, inspect-only
  -> named location overrides

Recipe
  -> default named locations
  -> Setting/SettingsGroup*
  -> Resource*
  -> lifecycle policy

Setting/SettingsGroup
  -> Resource*                        # implementation detail

Resource
  -> Driver
```

Cardinality rules for the MVP:

- A repository may contain many profile layers.
- A profile layer may be used by many machines.
- A machine may have many profile layers available. A command/run should resolve
  to exactly one **active profile stack**, which then produces exactly one
  **resolved profile** for that run.
- A profile stack is ordered. Later, more-specific layers may override or extend
  earlier, more-general layers.
- A resolved profile may manage many targets.
- A target may appear in many profile layers with different selected settings,
  location overrides, modes, and policies.
- A target resolves to one recipe for the current platform/version, subject to
  recipe pins and trust policy.
- A setting may bind to one desired-state artifact by convention or explicit
  profile-layer config.
- A setting may be implemented by one or more resources.
- A resource is managed by one driver.
- A change preview is per run: active profile stack + resolved profile +
  desired artifacts + current machine context + command intent + current target
  state.
- Backups and ledger entries are per run and should record the active profile
  stack, resolved profile hash, and machine context used for that run.

Profile-stack selection should be explicit and auditable. The tool may infer a
default stack from hostname, username, OS, architecture, or a local config file,
but it should make the selected stack and merge result auditable in detailed
status, dry-run, apply, and machine-readable output. Users should also be able
to override selection explicitly, for example:

```bash
dotfiles-manager status --profile personal-mac --profile user/leon
dotfiles-manager apply \
  --profile global \
  --profile personal-mac \
  --profile machine-user/klm.mobile.macbook-pro/leon \
  --dry-run
```

Layering should be supported deliberately, not accidentally. A machine matching
multiple profiles is not enough to merge them. The stack must come from an
explicit stack definition or explicit CLI selection, and the resolved profile
should be reproducible.

Recommended layer pattern:

```text
global
  -> os/macos
  -> role/personal-mac
  -> user/leon
  -> machine/klm.mobile.macbook-pro
  -> machine-user/klm.mobile.macbook-pro/leon
```

This preserves today's global + per-machine-user shape while making precedence
visible.

Normal UX should not require users to think in profile algebra. Public output can
translate the resolved stack into four plain scopes:

```text
shared       # portable settings for all users/machines using the profile
user         # settings for this OS user across machines
machine      # settings for this host/device
machine-user # settings for this OS user on this host/device only
```

Use user-facing labels rather than internal layer names:

```text
shared
this user
this machine
this user on this machine
```

When the tool asks for a scope, it should lead with the plain-language label and
show the internal scope only as detail:

| Prompt label | Stored scope | Use when |
| --- | --- | --- |
| Everyone using this repo | `shared` | Portable settings that should be the same everywhere. |
| Me, on all my machines | `user` | Personal identity/preferences that follow one OS user. |
| This machine | `machine` | Display, hardware, host, or device-specific settings. |
| Me on this machine | `machine-user` | Local overrides for one user on one host. |

Recipes should provide a recommended default scope per setting. The CLI should
ask only when the recipe cannot choose safely or when saving a new setting for
the first time.

Scope meanings:

- **shared** — portable settings intended for all users/machines using the
  profile stack;
- **user** — settings specific to the current OS account, portable across that
  user's machines;
- **machine** — settings specific to the current host/device, shared by users on
  that machine only when the recipe and policy allow it;
- **machine-user** — settings specific to the current OS account on the current
  host/device.

For example:

```text
git
  shared: unchanged
  this user: changed
  this machine: unchanged
  this user on this machine: unchanged
```

Advanced commands may expose exact profile-layer names, but the happy path should
say things such as "applying shared settings and settings for
leon@work-laptop" rather than "merging profile stack
global -> os/macos -> role/personal-mac -> machine/...".

Example scope choices:

```text
cobona:user-info
  scope: shared
  reason: same file should apply to all users on all machines

cobona:user.email
  scope: user
  reason: same person may use multiple machines

cobona:window-layout
  scope: machine
  reason: depends on this device's display setup

cobona:local-token-cache
  manage: never
  reason: not managed
```

Merge rules should be conservative and deterministic:

- target selections merge additively unless a later layer explicitly disables a
  target;
- selected settings merge additively unless a later layer explicitly excludes a
  setting;
- for a single-artifact setting, the highest-priority layer wins the artifact
  binding;
- desired artifacts do not merge implicitly. Merge only when the recipe declares
  a deterministic merge strategy;
- `excludeSettings` and `manage: never` win over inclusion;
- named location overrides use later-layer precedence;
- scalar target fields such as `mode` use later-layer precedence, unless that
  would weaken safety;
- safety policies should prefer the stricter value by default; weakening safety
  should require an explicit override field and should still be blocked for
  recipe-forbidden settings;
- recipe pins should not conflict silently. If two layers pin different recipe
  versions for the same target, the command should stop and explain the conflict;
- the resolved profile should be available in `status --verbose` and
  machine-readable output for debugging.

Examples:

- A global layer installs common CLI settings. A machine/user layer
  overrides the `config` named location for `git` and adds machine-specific
  Neovim exclusions.
- Two Macs can both use `personal-mac` while adding different machine/user
  overlays.
- `git` can appear in `global`, `personal-mac`, and
  `user/leon` with different contributions. The
  resolved profile may include `identity` and `aliases`, but no layer should
  manage credentials by default.

### Machine identity bootstrap

`init --repo` must establish local identity before scope-specific artifacts can
be resolved. The identity flow should:

1. Detect OS username, hostname, platform, architecture, and repo path.
2. Propose stable `user-id` and `machine-id` values.
3. Allow the user to override those IDs before any save/apply work.
4. Store local identity outside the desired-state repo unless the user
   intentionally commits a profile layer for it.
5. Show the selected profile stack and scope subjects in `status --verbose`.
6. Stop if multiple existing machine/user subjects could match and no explicit
   choice was made.

Changing a `user-id` or `machine-id` later is a migration, not a rename in
place. The tool should preview which desired paths, profile layers, and ledger
references would be affected.

### Control plane and data plane

`dotfiles-manager` separates profile configuration from desired state.

Profile stacks and profile layers are the **control plane**. They select
targets/settings, bind those settings to desired artifacts, override named target
locations, and configure policy.

Desired-state artifacts are the **data plane**. They contain the actual desired
settings: curated file trees, typed structured documents, or sanitized portable
app artifacts.

A profile layer should not normally contain actual settings values. For example,
a profile may say that this machine manages `git:user.email`, but the desired
email value lives in a desired artifact such as:

```text
desired/user/leon/targets/git/settings.yaml
```

This keeps profile resolution, save/apply behavior, safety review, and versioning
understandable:

```text
Profiles decide what desired state applies.
Desired artifacts contain the desired state.
Recipes translate desired state to and from real programs.
Ledgers and backups record what happened locally.
```

Use internal URI-like references to avoid mixing concepts:

- `desired://...` for repository desired-state artifacts;
- `target://...` for logical target or setting references;
- `state://...` for local ledger, backups, and raw captures;
- `temp://...` for per-operation temporary files;
- `secret://...` for external secret references;
- `recipe://...` for recipe definitions and recipe-owned resources.

Live program paths should normally be named locations such as `config`,
`user-data`, or `extensions`, not URI strings.

### Example user commands

Examples below use the current binary name as a placeholder. If the binary is
later renamed, keep the same public verbs. `init` is a one-time setup/bootstrap
command, not part of the daily management loop.

```bash
dotfiles-manager init
dotfiles-manager add git nvim raycast
dotfiles-manager list
dotfiles-manager status
dotfiles-manager diff
dotfiles-manager save git
dotfiles-manager apply --dry-run
dotfiles-manager apply
dotfiles-manager sync
```

Advanced and authoring commands should be separate from the normal path:

```bash
dotfiles-manager app create mytool
dotfiles-manager app edit mytool
dotfiles-manager app validate mytool
dotfiles-manager app test mytool --roundtrip
dotfiles-manager recipe explain git
dotfiles-manager backup list
dotfiles-manager restore <run-id>
```

Compatibility aliases may exist for v1 or expert workflows, but normal
documentation should prefer:

- `save` for current machine -> repository;
- `apply` for repository -> current machine;
- `sync` only for a guided conflict-aware choice, not a blind two-way merge.

### CLI contract v2

This section is normative for the MVP command surface. It defines behavior that
formal specs should preserve.

Global flags:

| Flag | Meaning |
| --- | --- |
| `--profile <layer>` | Add an explicit profile layer to the active stack. Repeatable. |
| `--scope <scope>` | Choose `shared`, `user`, `machine`, or `machine-user` when saving. |
| `--dry-run` | Do not mutate the repo or live target state. Safe reads are allowed. |
| `--json` | Emit the stable machine-readable result schema. |
| `--non-interactive` | Never prompt. Stop with a specific error if input is required. |
| `--yes` | Accept safe default prompts, but never bypass safety blockers. |
| `--verbose` | Include profile stack, artifact URIs, drivers, and ledger references. |

Common exit codes:

| Code | Meaning |
| --- | --- |
| `0` | Completed with no errors or blocked required work. |
| `1` | General failure or unexpected internal error. |
| `2` | Validation, config, recipe, or schema error. |
| `3` | Changes or conflicts were found when the command was used as a check. |
| `4` | User input required but unavailable in `--non-interactive` mode. |
| `5` | Safety, lifecycle, trust, or secret policy blocked an operation. |
| `6` | Partial success: at least one item succeeded and one item failed/skipped. |

`--json` output should always include:

```yaml
schemaVersion: 1
command: init | add | list | status | diff | save | apply | sync | backup.list | restore | migrate
runId: run-...
profileStack: [global, os/macos, user/leon]
summary:
  status: ok | changed | blocked | partial | error
  changed: 0
  blocked: 0
  applied: 0
  saved: 0
items:
  - ref: git:user.email
    target: git
    setting: user.email
    scope: user
    state: changed-current
    recommendedAction: save
    allowedActions: [diff, save, skip]
    blockedActions: []
    messages: []
ledgerRef: state://ledger/current/targets/git/status/run-...
```

Command contracts:

| Command | Default behavior | Writes? | Notes |
| --- | --- | --- | --- |
| `init` | Create/connect local manager state and IDs. | local only | One-time setup. |
| `add <target...>` | Add supported targets with safe defaults. | config | Does not invent desired values. |
| `list` | Show managed selections. | no | Future flags may list installed/catalog data. |
| `status [ref]` | Compare desired, current, and last-applied state. | no | Groups by next action and blocker. |
| `diff [ref]` | Show readable diff when available. | no | Opaque bundles show metadata/hash only. |
| `save [ref]` | Save changed selected settings. | repo | No operand means all unblocked changes. |
| `apply [ref]` | Apply desired artifacts after preview/backup. | live | Default is safe partial apply. |
| `sync` | Guided save/apply/skip choices for differences. | chosen | Never performs blind automatic two-way merge. |
| `backup list` | List backups from local state. | no | Must support `--json`. |
| `restore <run-id>` | Restore from backup after preview. | live | User-initiated recovery. |
| `migrate` | Generate v2 config from v1 config after preview. | config | Must support `--dry-run` and `--json`. |

Nested CLI commands should use stable dotted command identifiers in JSON
output, for example `backup.list` for `dotfiles-manager backup list`.

`add` decision table:

| State | Behavior |
| --- | --- |
| supported and installed | Add recipe defaults, show managed/skipped settings. |
| supported but not installed | Add only if user confirms; status becomes `missing-current`. |
| unsupported | Offer `app create <target>` wizard; do not create write rules. |
| ambiguous target name | Show candidates and require explicit choice. |
| already managed | Show current selection; no duplicate entry. |
| recipe untrusted | Stop until recipe is validated and enabled. |
| experimental recipe | Require explicit opt-in before adding write-capable settings. |
| no writable settings | Add as read-only/inspect-only or skip. |

`list` default behavior is “managed selections in the active resolved profile.”
Future `list --available`, `list --installed`, or `list --recipes` may be added,
but they should not change the default meaning.

Prompt rules:

- `save` must ask before first saving a setting, choosing a scope, saving an
  opaque artifact, or saving data with sensitivity above `low`.
- `apply` must ask before live writes unless policy and `--yes` allow safe
  defaults. It must still stop on safety, trust, and lifecycle blockers.
- `--dry-run` may read current state, run declared read-only native export, and
  write temporary/local state records, but must not change desired repo artifacts
  or live target state.
- `--non-interactive` must fail with exit code `4` when a prompt would be
  required. It must not silently choose destructive, trust, opaque, or lifecycle
  answers.
- Mixed target results must be represented per item. The command may complete
  safe items and return exit code `6` for partial success.

### Status and preview output

Convenient output should be organized by decision, not by internal resource. The
default `status` view should avoid profile algebra and show only enough detail
for the next action:

```text
Changed on this machine:
  git:user.email           this user       save / diff
  raycast:quicklinks       shared          save / diff

Ready to apply from repo:
  nvim:config              shared          apply / diff

Needs attention:
  raycast:settings-and-data this machine   opaque native export; diff limited

Blocked:
  cobona:user-info         shared          app must be closed before apply
```

`diff`, `save --dry-run`, and `apply --dry-run` should answer:

```text
What will change?
Where will it be saved or applied?
What is skipped or blocked?
What backup will be created?
How do I see technical detail if needed?
```

The default output may hide drivers and URIs, but it must not hide uncertainty.
If the tool cannot diff, redact, merge, verify, restart an app, or identify a
setting safely, it should say so plainly and offer the safest next action.

### Canonical status and conflict state machine

This section is normative for MVP status and conflict behavior. The engine should
compute status from normalized desired state, normalized current state,
last-applied state, recipe version, normalizer version, resource capability,
lifecycle state, and trust policy.

Canonical item states:

| State code | Meaning | Recommended action |
| --- | --- | --- |
| `unchanged` | Current normalized state equals desired normalized state; ledger may be absent. | none |
| `changed-current` | Current differs from desired; baseline matches desired. | `save` or `apply` |
| `ready-to-apply` | Desired exists and current is missing or different, with no local divergent edit. | `apply` |
| `missing-desired` | Target/setting is selected but no desired artifact exists. | `save` or create artifact |
| `missing-current` | Desired exists but live target path/value is absent. | `apply` or skip |
| `conflict` | Desired and current both changed since last successful apply/save. | guided `sync` |
| `opaque-changed` | Opaque artifact hash/metadata differs; readable diff unavailable. | save/apply after confirmation |
| `blocked-lifecycle` | Target is running or unavailable in a way recipe forbids. | quit/retry/skip |
| `blocked-safety` | Secret, trust, policy, selector, or recipe rule blocks action. | inspect/fix config |
| `unsupported` | No trusted recipe/capability supports the requested operation. | skip or create recipe |
| `unknown` | State cannot be determined safely. | inspect/verbose |

State derivation rules:

- If recipe validation, trust policy, or selector validation fails, status is
  `blocked-safety` before current state is read.
- If lifecycle forbids the requested read/write while a target is running, status
  is `blocked-lifecycle`.
- If no desired artifact exists for a selected setting, status is
  `missing-desired`; `apply` is blocked and `save` may create the artifact.
- If desired exists and current does not, status is `missing-current`; `apply`
  may create the current state when the driver supports it.
- If current equals desired, status is `unchanged` even if no ledger exists.
- If current differs from desired and there is no last-applied state, status is
  `changed-current` for `save` and `ready-to-apply` for `apply`; prompts must
  explain that no previous sync baseline exists.
- If last-applied equals desired and current differs, status is
  `changed-current`.
- If last-applied equals current and desired differs, status is `ready-to-apply`.
- If desired and current both differ from last-applied, status is `conflict`.
- If recipe version, driver version, or normalizer version changed since
  last-applied, status should include a `needs-recheck` warning and may require
  recomputing normalized hashes before allowing writes.

Target-level status is the highest-severity item state in this order:

```text
blocked-safety
blocked-lifecycle
unsupported
conflict
opaque-changed
changed-current
ready-to-apply
missing-desired
missing-current
unknown
unchanged
```

### Example normal user config

The default config should be convention-based. Users should be able to select
targets and settings without writing profile stacks, artifact URIs, resource
names, driver names, or native paths. Most users should not need to open this
file during the happy path; `add`, `save`, and `sync` can create and update it
through prompts.

```yaml
schemaVersion: 1

manage:
  git: true
  nvim: true
  raycast:
    settings:
      - snippets
      - quicklinks

policies:
  writes: confirm
  backupBeforeApply: true
  onConflict: stop
```

Shorthand rules:

- `manage.<target>: true` selects the recipe's default safe settings for that
  target.
- `settings: [name, ...]` selects named public settings and lets scope and
  convention resolve artifact locations.
- `settings.<name>.artifact: desired://...` is an advanced override, not a
  normal onboarding requirement.
- A setting without an explicit artifact resolves by convention from its scope,
  target ID, and setting ID. If the artifact does not exist, `save` creates it
  after preview and confirmation.

Convention examples:

```text
cobona:user.email --scope user
  -> desired://user/<user-id>/targets/cobona/settings#user.email

cobona:user-info --scope shared
  -> desired://shared/-/targets/cobona/artifacts/user-info.json

nvim:config --scope shared
  -> desired://shared/-/targets/nvim/artifacts/config
```

`add` should make the target visible in config and select the recipe's default
safe settings. It should not invent desired values. The first `save` for a
selected setting captures current machine state into the conventional desired
artifact after a preview.

Status output should default to a simple saved-vs-current view: target/setting,
scope, changed/unchanged/missing/blocked state, and recommended action. Exact
profile stacks, artifact URIs, driver names, and ledger references belong in
`--verbose` or machine-readable output.

### Advanced layered config example

The layered form is for advanced users and implementers who need explicit
profile stacks, location overrides, recipe pins, or artifact bindings. It should
not be required for the happy path.

```yaml
schemaVersion: 1

profileStacks:
  default:
    - global
    - os/macos
    - role/personal-mac
    - user/leon
    - machine/klm.mobile.macbook-pro
    - machine-user/klm.mobile.macbook-pro/leon

profiles:
  global:
    manage:
      git:
        settings:
          aliases:
            artifact: desired://shared/-/targets/git/settings#aliases
          includes:
            artifact: desired://shared/-/targets/git/settings#includes
        excludeSettings:
          - credentials

      zsh:
        artifact: desired://shared/-/targets/zsh/artifacts/rc
      starship:
        artifact: desired://shared/-/targets/starship/artifacts/starship.toml
      nvim:
        settings:
          config:
            artifact: desired://shared/-/targets/nvim/artifacts/config

    policies:
      unknown: ignore
      secrets: never
      writes: confirm
      backupBeforeApply: true
      onConflict: stop
      requireRecipeTrust:
        - bundled
        - local

    recipeLock:
      git:
        source: bundled
        version: "1"
      nvim:
        source: bundled
        version: "1"

  os/macos:
    manage:
      macos.finder:
        settings:
          preferences:
            artifact: desired://shared/-/targets/macos.finder/settings#preferences
        mode: read-only

      iterm2:
        settings:
          preferences:
            artifact: desired://shared/-/targets/iterm2/settings#preferences
        mode: read-only

  role/personal-mac:
    manage:
      visual-studio-code:
        settings:
          settings:
            artifact: desired://shared/-/targets/visual-studio-code/artifacts/settings.json
          keybindings:
            artifact: desired://shared/-/targets/visual-studio-code/artifacts/keybindings.json
          snippets:
            artifact: desired://shared/-/targets/visual-studio-code/artifacts/snippets
          extensions:
            artifact: desired://shared/-/targets/visual-studio-code/settings#extensions
        excludeSettings:
          - account
          - workspace
          - cache
          - extension-secrets

      cursor:
        settings:
          settings:
            artifact: desired://shared/-/targets/cursor/artifacts/settings.json
          keybindings:
            artifact: desired://shared/-/targets/cursor/artifacts/keybindings.json
          snippets:
            artifact: desired://shared/-/targets/cursor/artifacts/snippets
          extensions:
            artifact: desired://shared/-/targets/cursor/settings#extensions
        excludeSettings:
          - account
          - workspace
          - cache
          - extension-secrets

      raycast:
        settings:
          snippets:
            artifact: desired://shared/-/targets/raycast/artifacts/snippets.json
          quicklinks:
            artifact: desired://shared/-/targets/raycast/artifacts/quicklinks.json
        excludeSettings:
          - account
          - cloud-sync
          - extension-secrets
          - ai-chats
          - clipboard-history
          - history
          - cache
          - local-databases

  user/leon:
    manage:
      git:
        settings:
          user.email:
            artifact: desired://user/leon/targets/git/settings#user.email

  machine/klm.mobile.macbook-pro:
    manage:
      git:
        locations:
          config: "~/.config/git/config"

  machine-user/klm.mobile.macbook-pro/leon:
    manage:
      raycast:
        settings:
          settings-and-data:
            artifact: >-
              desired://machine-user/klm.mobile.macbook-pro/leon/targets/raycast/artifacts/settings-and-data.rayconfig
            form: portable-export
            storagePolicy: encrypted-opt-in
            secret: prompt-at-runtime
            applyCategories:
              - settings-aliases-hotkeys
              - store-extensions
              - window-management-layouts

      nvim:
        settings:
          config:
            excludePaths:
              - lazy-lock.json
              - .local/**
              - .cache/**
```

Notes:

- `nvim:config` in the shared layer binds to
  `desired://shared/-/targets/nvim/artifacts/config`. It does not mean
  blind-copy the entire Neovim directory. Recipe defaults still exclude cache,
  generated files, secrets, machine-local state, and other unsafe state.
- Users should not need settings lists for simple targets. Settings selection is
  for targets where the recipe exposes meaningful choices. A single-layer config
  may omit `profileStacks` and `profiles`, but layered configs should make the
  active stack explicit.
- Profile layers should bind settings to artifacts. They should not normally
  store actual desired values inline.
- `excludeSettings` is optional when the recipe already blocks unsafe settings by
  default. It can be useful when users want the profile to document intentional
  exclusions.
- `locations` overrides where the tool looks. Location overrides must not weaken
  safety rules automatically.
- `mode: read-only` is important. Some targets may be inspectable before they
  are safely writable.
- Fields such as `form`, `storagePolicy`, `secret`, and `applyCategories` in the
  example show the desired policy intent. Before this becomes a schema contract,
  decide which of those fields belong on artifact metadata, profile-layer
  bindings, or recipe metadata.
- `onConflict: stop` should be the default. Users should explicitly choose merge
  or overwrite behavior.
- Advanced users may use driver-specific overrides such as `excludePaths`, but
  these should be scoped to the setting that allows them and validated against
  the recipe and driver:

```yaml
manage:
  nvim:
    settings:
      config:
        artifact: desired://shared/-/targets/nvim/artifacts/config
        excludePaths:
          - lazy-lock.json
          - .local/**
          - .cache/**
```

For unsupported programs, the advanced flow should still be guided:

```bash
dotfiles-manager app create mytool
dotfiles-manager app validate mytool
dotfiles-manager app test mytool --roundtrip
dotfiles-manager add mytool
dotfiles-manager save mytool
dotfiles-manager diff mytool
dotfiles-manager apply --dry-run mytool
dotfiles-manager apply mytool
```

The wizard should guide the user through: app name, whether settings are files or
native export/import, live locations or app commands, default scope
(`shared`, `user`, `machine`, or `machine-user`), lifecycle requirements, backup
behavior, validation, and a dry-run test.
Resources and drivers can remain inspectable later, but they should not be
required concepts for basic onboarding.

## Repository layout and desired-state artifacts

Recommended repository layout:

```text
dotfiles/
  dotfiles-manager.yaml

  profiles/
    stacks/
      personal-macbook.yaml
    layers/
      shared/
        global.yaml
      os/
        macos.yaml
      roles/
        personal-mac.yaml
      users/
        leon.yaml
      machines/
        klm.mobile.macbook-pro.yaml
      machine-users/
        klm.mobile.macbook-pro/
          leon.yaml

  desired/
    shared/
      -/
        targets/
          git/
            manifest.yaml
            settings.yaml
          nvim/
            manifest.yaml
            artifacts/
              config/
                init.lua
                lua/
          raycast/
            manifest.yaml
            artifacts/
              snippets.json
              quicklinks.json
          cobona/
            manifest.yaml
            artifacts/
              user-info.json

    user/
      leon/
        targets/
          git/
            manifest.yaml
            settings.yaml
          cobona/
            manifest.yaml
            settings.yaml

    machine/
      klm.mobile.macbook-pro/
        targets/
          git/
            manifest.yaml
            settings.yaml

    machine-user/
      klm.mobile.macbook-pro/
        leon/
          targets/
            nvim/
              manifest.yaml
              settings.yaml
            raycast/
              manifest.yaml
              artifacts/
                settings-and-data.rayconfig

  recipes/
    local/
    overlays/

  docs/
```

Repository paths should encode scope before target. This avoids ambiguity when
the same target has shared settings, user-specific settings, machine-specific
settings, and machine-user settings. The desired-state filesystem layout mirrors
the `desired://` URI shape exactly: `desired://user/leon/...` maps to
`desired/user/leon/...`, and the shared subject is the literal `-` segment.

Desired-state target directories may contain:

- `manifest.yaml` — target metadata for that scope: target ID, recipe ID,
  artifact index, provenance, and compatibility metadata;
- `settings.yaml` — normalized logical settings for that target/scope;
- `artifacts/` — desired files, file trees, native exports, or portable app
  artifacts owned by that target/scope.

Profiles contain control-plane config. Desired artifacts contain actual desired
settings. Recipes contain support definitions. Ledger entries, backups, raw
captures, and rendered temporary files live outside the repo by default.

### Schema boundaries and versioning

This section is normative for the MVP schema split. Each persisted file type has
its own version context; `schemaVersion: 1` is interpreted by file type, not as
a global universal schema.

| File type | Owns | Required in MVP |
| --- | --- | --- |
| Root config | active stacks, target selections, policies | yes |
| Profile stack | ordered profile layer names | yes |
| Profile layer | selected targets/settings, scopes, policies, locations | yes |
| Desired manifest | target/scope metadata and artifact index | yes |
| Desired artifact | actual values/files/portable exports | yes |
| Recipe | target support definition | yes |
| Resource | recipe-owned implementation unit | yes |
| Preview JSON | dry-run/change-preview result | yes |
| Ledger entry | completed run receipt | yes |
| Backup metadata | restore source and verification info | yes |

Version rules:

- Config, profile, manifest, recipe, artifact, ledger, and backup schemas must be
  versioned independently.
- Readers may accept older versions only through explicit migration code.
- Unknown fields are errors in normative MVP files unless the schema explicitly
  reserves an extension object.
- Recipe, driver, selector, and normalizer versions must be included in preview
  and ledger entries so status can detect stale hashes.
- A schema migration must be previewable and reversible when it mutates desired
  repo files.

Minimal desired manifest contract:

```yaml
schemaVersion: 1
target: cobona
scope: user | shared | machine | machine-user
subject: "-" | { user: leon } | { machine: host } | { machine: host, user: leon }
recipe: cobona
recipeVersion: "1"
artifacts:
  settings:
    path: settings.yaml
    form: structured | file | file-tree | portable-export
    schema: dotfiles-manager.cobona.settings.v1
settings:
  user.email:
    artifact: settings
    fragment: user.email
```

### Artifact resolution algorithm

Artifact resolution must fail closed on ambiguity. The engine should resolve a
selected setting in this order:

1. Resolve machine context: repo, OS user ID, machine ID, platform, and active
   profile stack.
2. Merge profile layers into one resolved profile.
3. Resolve the target recipe and verify recipe trust, support level, and platform
   compatibility.
4. Resolve the public setting and recommended/default scope.
5. Apply explicit profile-layer artifact binding if present.
6. Otherwise derive the conventional desired URI from scope, subject, target,
   setting, and artifact form.
7. Load the desired target manifest when the desired directory exists.
8. Validate artifact path, form, schema, recipe compatibility, and fragment.
9. Detect duplicate bindings, missing artifacts, unsupported artifact forms, and
   subject mismatches.
10. Stop with a clear error if more than one artifact could supply the same
    setting or if no artifact can be created safely.

Desired manifests are required for MVP target directories. They may be generated
by `save` or migration, but once present they are authoritative metadata for
that target/scope.

### Desired-state artifact forms

A desired-state artifact is the versioned source of truth for one setting or
settings group. Supported forms should be explicit:

1. **`file`** — one curated file that is meaningful as a portable artifact.
2. **`file-tree`** — a curated directory of files, used for classical dotfiles
   such as Neovim config.
3. **`structured`** — a normalized YAML/JSON/TOML document with a recipe-defined
   schema, used for settings such as Git identity or app snippets.
4. **`portable-export`** — an app-native export artifact, used only when the app
   requires that format for import. There are two sub-cases:
   - sanitized portable exports may be committed when the recipe can prove unsafe
     state has been excluded;
   - opaque encrypted portable exports may be stored only with explicit opt-in,
     are not diffable or mergeable, and must declare their secret/passphrase and
     category limitations.

Raw captures and generated rendered files are not desired artifacts. They belong
in local state or temporary operation directories.

Example structured artifact for Git user.email:

```text
desired/user/leon/targets/git/settings.yaml
```

```yaml
user:
  name: Leonid Komarovsky
  email: leon@example.com
```

Example file-tree artifact for Neovim:

```text
desired/shared/-/targets/nvim/artifacts/config/
  init.lua
  lua/
    plugins.lua
```

Example sanitized structured artifact for command-backed app snippets:

```text
desired/shared/-/targets/exampleapp/settings.yaml
```

```yaml
snippets:
  - name: Greeting
    keyword: greet
    text: Hello world
```

### Artifact bindings

A settings selection may bind to a desired artifact. In normal user workflows the
binding should be inferred by scope and convention. Advanced profile config may
bind explicitly:

```yaml
manage:
  nvim:
    settings:
      config:
        artifact: desired://shared/-/targets/nvim/artifacts/config

  git:
    settings:
      user.email:
        artifact: desired://user/leon/targets/git/settings#user.email
```

If a selected setting requires an artifact and no artifact can be resolved by
convention or explicit binding, the tool should ask the user to save, create, or
bind one. It should not silently infer desired values from the current machine.

Desired artifact lifecycle rules:

- `save` may create or update desired artifacts only after preview and
  confirmation.
- Removing a setting from `manage:` does not delete its desired artifact. It
  becomes orphaned until the user runs a cleanup command or confirms deletion.
- Scope changes should move/copy artifacts through a previewable migration, not
  by silently rewriting paths.
- Recipe or setting renames require a migration that records old and new refs.
- Garbage collection must default to dry-run and show every artifact that would
  be deleted.
- Opaque artifacts require explicit confirmation before overwrite or deletion.

### Internal URI schemes

Most users should not need URI schemes. They are for recipes, ledgers, debug
output, lifecycle records, and advanced configuration.

Supported schemes for v2:

| Scheme | Purpose |
| --- | --- |
| `target://` | Logical target or setting reference. |
| `desired://` | Canonical desired-state repository reference. |
| `state://` | Observed state, ledgers, backups, and caches. |
| `temp://` | Ephemeral per-run workspace data. |
| `secret://` | External secret reference; never secret material. |
| `recipe://` | Recipe definition or recipe-owned resource. |

URI grammar:

```text
uri = scheme "://" authority path [ "?" query ] [ "#" fragment ]

scheme = "target" | "desired" | "state" | "temp" | "secret" | "recipe"

target-uri = "target://" target-id [ "/" setting-id ]

desired-uri = "desired://" desired-scope "/" desired-subject
              "/targets/" target-id
              [ "/" desired-kind [ "/" artifact-path ] ]

desired-scope = "shared" | "user" | "machine" | "machine-user"
desired-subject = "-" | user-id | machine-id | machine-id "/" user-id
desired-kind = "manifest" | "settings" | "artifacts"

state-uri = "state://" state-kind "/" state-subject
            "/targets/" target-id [ "/" state-item-path ]

state-kind = "observed" | "ledger" | "backup" | "cache"

temp-uri = "temp://" run-id "/" temp-path
secret-uri = "secret://" provider "/" secret-path [ "#" secret-field ]
recipe-uri = "recipe://" recipe-id [ "/" recipe-path ]
```

Use `-` as the desired subject for `shared`, because shared scope has no user or
machine subject.

`target://` intentionally uses URI path syntax, not the public colon syntax.
For example, public `cobona:user.email` becomes internal
`target://cobona/user.email` when a URI is needed.

Examples:

```text
target://cobona
target://cobona/user.email

desired://shared/-/targets/cobona/artifacts/user-info.json
desired://user/leon/targets/cobona/settings#user.email
desired://machine/klm.mobile.macbook-pro/targets/git/settings
desired://machine-user/klm.mobile.macbook-pro/leon/targets/nvim/settings

state://ledger/current/targets/cobona/save/2026-06-04T10-15-00Z
state://backup/current/targets/cobona/apply/2026-06-04T10-15-00Z/config.yaml

temp://run-20260604-101500/cobona/export/config.yaml

secret://op/personal/cobona/api-token#value
secret://env/COBONA_API_TOKEN

recipe://cobona
recipe://cobona/files/config-yaml
```

The fragment on `desired://.../settings#user.email` identifies a logical setting
inside `settings.yaml`. Do not use fragments for filesystem paths; use path
segments under `artifacts/` for files and file trees.

`secret://` is a reference to secret material managed elsewhere; it must never
contain the secret value. If persistent secret-provider integration is deferred,
recipes that need a passphrase should prompt at runtime instead of writing a
secret reference into the repository.

### Example: Cobona

Cobona is a binary app with two live files:

```text
~/.cobona/config.yaml
~/.cobona/user-info.json
```

The user wants to manage one logical value inside `config.yaml` and one whole
portable file:

```text
cobona:user.email   # value at user.email inside ~/.cobona/config.yaml
cobona:user-info    # portable ~/.cobona/user-info.json artifact
```

Scopes:

```text
cobona:user.email
  scope: user
  reason: same OS user/person may use multiple machines

cobona:user-info
  scope: shared
  reason: same file should apply to all users on all machines
```

Normal CLI flow:

```bash
dotfiles-manager add cobona
dotfiles-manager save cobona:user.email --scope user
dotfiles-manager save cobona:user-info --scope shared
dotfiles-manager diff cobona
dotfiles-manager apply cobona
```

Desired layout after save:

```text
desired/
  user/
    leon/
      targets/
        cobona/
          manifest.yaml
          settings.yaml

  shared/
    -/
      targets/
        cobona/
          manifest.yaml
          artifacts/
            user-info.json
```

User-scoped `settings.yaml` contains the normalized logical value:

```yaml
user:
  email: leon@example.com
```

User-scoped `manifest.yaml` records desired-state metadata, not native live
paths:

```yaml
target: cobona
scope: user
subject:
  user: leon
recipe: cobona
artifacts:
  settings:
    path: settings.yaml
    form: structured
    schema: dotfiles-manager.cobona.settings.v1
settings:
  user.email:
    artifact: settings
    fragment: user.email
```

Shared `manifest.yaml` records that `user-info` is a portable file artifact:

```yaml
target: cobona
scope: shared
subject: "-"
recipe: cobona
artifacts:
  user-info:
    path: artifacts/user-info.json
    form: file
settings:
  user-info:
    artifact: user-info
```

The Cobona recipe owns native live locations and translation rules. A sketch:

```yaml
id: cobona
locations:
  config:
    role: config
    default:
      darwin: "~/.cobona/config.yaml"
      linux: "~/.cobona/config.yaml"
  user-info:
    role: config
    default:
      darwin: "~/.cobona/user-info.json"
      linux: "~/.cobona/user-info.json"

settings:
  user.email:
    artifactType: structured
    resources:
      - cobona-config-email
  user-info:
    artifactType: file
    resources:
      - cobona-user-info-file

resources:
  cobona-config-email:
    driver: yaml-file
    source:
      location: config
    selectors:
      includePaths:
        - user.email

  cobona-user-info-file:
    driver: file
    source:
      location: user-info
```

Internal URI equivalents:

```text
target://cobona
target://cobona/user.email
target://cobona/user-info

desired://user/leon/targets/cobona/settings#user.email
desired://shared/-/targets/cobona/artifacts/user-info.json

recipe://cobona
recipe://cobona/resources/cobona-config-email
```

Important distinction:

- `cobona:user.email` is the public logical setting reference;
- `~/.cobona/config.yaml` is the native source/destination path owned by the
  recipe's `config` named location;
- `desired://user/leon/targets/cobona/settings#user.email` is the internal
  desired-state reference.

Do not use those three forms interchangeably in user-facing docs.

### Local state, ledger, and backups

The ledger and backups should live outside the repository by default, for
example under an XDG or platform-appropriate local state directory:

```text
state://ledger/current/targets/cobona/save/<run-id>
state://backup/current/targets/cobona/apply/<run-id>/config.yaml
state://observed/current/targets/cobona/save/<run-id>/config.yaml
state://cache/current/targets/cobona/discovery/<run-id>
temp://<run-id>/cobona/rendered-inputs/config.yaml
```

The local state area may contain sensitive current-machine material, including
pre-apply backups, raw app captures, rendered app-native inputs, app versions,
verification results, and lifecycle actions taken. It should not be committed.

### Mutation transaction model

This section is normative for MVP writes. `save` and `apply` should be treated
as transactions even when the underlying filesystem/app cannot provide full
atomicity.

Apply phases:

1. Resolve profile, recipe, resources, desired artifacts, and current machine
   context.
2. Validate recipe trust, selector safety, lifecycle policy, and desired artifact
   schemas.
3. Read and normalize current state.
4. Compute preview and request confirmation if needed.
5. Re-check current hashes immediately before writing.
6. Acquire a per-target/resource lock where supported.
7. Create backup unless the recipe declares backup unavailable and policy permits
   writing without it.
8. Render desired state into a temp path when the driver requires it.
9. Apply via atomic replace when possible; otherwise use driver-declared safe
   write behavior.
10. Verify by read-after-write, export-normalize-compare, hash check, or
    recipe-declared verifier.
11. Commit ledger only after verification succeeds or after recording a clearly
    partial failure.
12. Release locks and run any user-approved reopen action.

Save phases:

1. Resolve selection and scope.
2. Read and normalize current state.
3. Detect redaction/blocking conditions.
4. Compute preview against desired artifact if present.
5. Ask for confirmation when creating, overwriting, changing scope, or saving
   sensitive/opaque state.
6. Write desired artifact and manifest through temp files where possible.
7. Record ledger with current hash, desired hash, recipe version, driver version,
   and normalizer version.

Failure semantics:

- A failed apply should not update last-applied hashes for failed items.
- A partially successful apply must record per-item outcomes and return partial
  exit code `6`.
- Automatic rollback is best-effort only and allowed only when the driver and
  backup declare it safe.
- User-initiated `restore <run-id>` is separate from automatic rollback. Restore
  must show a preview, create a new backup before restoring when possible, and
  write a new ledger entry.
- If backup is unavailable and policy requires backup, write is blocked.

Backup metadata should include run ID, target, setting/resource refs, live paths,
artifact refs, source app/version, pre-write hashes, backup paths, sensitivity,
redaction status, and restore capability.

### Ledger commit rules

Ledger entries are local records of what happened, not desired state. A completed
write ledger entry should record:

- command, run ID, timestamp, repo/worktree identity, and active profile stack;
- machine context: user ID, machine ID, platform, architecture, hostname;
- target/setting/resource refs and recipe/driver/normalizer versions;
- desired artifact URI and hash before/after;
- current normalized hash before/after;
- backup refs and restore capability;
- lifecycle actions requested, accepted, skipped, or failed;
- verification result and errors/warnings;
- final per-item status code.

`last-applied` should be updated only for items whose apply operation completed
and verified successfully. `save` may update the desired artifact hash and
current observed hash, but it must not imply that a different machine has applied
that state.

## Internal architecture

### Overview

```text
Product model:
Machine context
  -> active profile stack
    -> resolved profile
      -> Target selection
      -> Settings selection
      -> Named location resolution
      -> Change preview
      -> Apply / backup / verify
```

```text
Internal architecture:
ProfileStack
  -> ResolvedProfile
    -> Target selection and user policy
      -> Recipe resolution
        -> Settings groups
          -> Resources
            -> Driver
              -> read / normalize / diff / preview / apply / verify
                -> StateLedger + backups
```

This model generalizes dotfiles without abandoning them. A dotfile is simply a
file-backed resource managed by a file or file-tree driver. The product model
should stay smaller than the internal architecture.

### Profile

A profile layer is one desired-state contribution. It is not the same thing as a
physical machine. Different machines may share one layer, and one machine should
be able to use multiple layers through an ordered profile stack.

Examples:

- `global`
- `os/macos`
- `personal-mac`
- `work-mac`
- `server`
- `user/leon`
- `machine/klm.mobile.macbook-pro`
- `machine-user/klm.mobile.macbook-pro/leon`

A profile layer contains:

- selected targets;
- selected/excluded settings;
- desired artifact bindings;
- per-target modes such as read-only or read-write;
- named location overrides;
- global safety policies;
- recipe pins and trust requirements;
- optional local recipe paths;
- optional legacy dotfiles adapter configuration during migration.

Profile-stack selection is a separate concern from layer contents. The resolver
may choose a default stack from hostname, username, OS, architecture, or local
configuration, but command output should always make the active stack and
resolved profile visible.

### Target

A target is the canonical thing being managed. Examples:

```text
git
zsh
nvim
iterm2
raycast
klack
macos.finder
macos.dock
launchd.user.some-agent
custom.files
```

Target identity must be separate from display name.

For GUI apps, identity may include:

- display name;
- bundle ID;
- team ID;
- app version and build number;
- install path;
- install source: App Store, Homebrew Cask, direct download, Setapp, unknown;
- macOS version.

For CLI tools, identity may include:

- command name;
- resolved binary path;
- version output;
- package manager source;
- config path candidates.

For services, identity may include:

- launchd label;
- plist path;
- service user;
- binary path;
- relevant ports if needed for diagnostics.

For system features, identity may include:

- defaults domain or system domain;
- OS version;
- hardware/platform constraints.

### Named location

A named location is a symbolic path slot used by recipes and user overrides.
Named locations refer to live target paths on the machine, not desired-state
paths inside the repository. They are recipe/internal identifiers, not public
setting IDs; recipes may use names such as `userConfig` or `workspaceStorage`
when those are clearer for implementers. Examples include:

```text
config
userConfig
data
state
cache
snippets
extensions
```

Recipes should define named locations instead of hardcoding paths throughout
resources. The recipe provides platform-specific defaults; users can override
locations when their setup is nonstandard.

Example user override:

```yaml
manage:
  visual-studio-code:
    settings:
      - settings
      - keybindings
      - snippets
    locations:
      userConfig: "~/Library/Application Support/Code/User"
```

Example recipe locations:

```yaml
locations:
  userConfig:
    role: config
    default:
      darwin: "~/Library/Application Support/Code/User"
      linux: "~/.config/Code/User"
    manageDefault: allowed

  workspaceStorage:
    role: state
    manageDefault: blocked
    reason: "Workspace-local state can contain machine-specific or sensitive data."

  cache:
    role: cache
    manageDefault: blocked
    reason: "Cache is runtime state, not portable settings."
```

Resources should refer to named target locations. Desired artifacts should be
referenced separately through artifact bindings:

```yaml
artifact: desired://user/leon/targets/git/settings#user.email
location: config
```

Resources should refer to named locations:

```yaml
resources:
  visual-studio-code.user-settings:
    driver: json-file
    location: userConfig
    path: settings.json

  visual-studio-code.keybindings:
    driver: json-file
    location: userConfig
    path: keybindings.json
```

Defaults should be conservative:

- config locations may be manageable when the recipe knows the format and
  exclusions;
- data locations require stronger justification;
- cache, runtime, credentials, account, local database, and workspace-local
  locations should be blocked by default;
- user overrides change where the tool looks; they must not weaken safety rules
  automatically.

### Recipe

A recipe answers:

> For this target, on this platform/version, what settings are known,
> which are safe to manage, which are forbidden, which drivers read/write them,
> how are values normalized, and how is success verified?

Recipes should be visible, editable manifests. They should be declarative by
default and should not be arbitrary programs.

Recipe matching should be predicate-based rather than display-name based:

```yaml
matches:
  platform:
    os: macos
    versions: ">=14 <16"
  app:
    bundleId: com.example.SomeApp
    versions: ">=1.0 <2.0"
```

Do not require exact-version recipes for everything. Use compatibility ranges
and mark individual settings as `stable`, `read-only`, `experimental`, or
`deprecated`.

### SettingsGroup

A settings group is an internal recipe boundary for a meaningful setting or
setting family. It is useful when a target has separable categories of state with
different portability, safety, or implementation rules.

Examples:

```text
git:user.email
git:aliases
git:includes
visual-studio-code:settings
visual-studio-code:keybindings
visual-studio-code:snippets
visual-studio-code:extensions
cursor:settings
cursor:keybindings
nvim:config
iterm2:preferences
macos.dock:items
macos.finder:preferences
```

A settings group is **not** required for every user-facing configuration. If a
program has one obvious safe setting, the user should be able to write:

```yaml
manage:
  starship: true
```

and let the recipe select the default safe setting.

A settings group defines:

- title and description;
- default include policy;
- support level;
- safety classification;
- capability: read-only, read-write, import-only, inspect-only, or never;
- resources that implement the setting;
- named locations used by those resources;
- legal user overrides;
- lifecycle policy, when the setting should not be read or written while the
  target is running;
- why a setting is excluded or forbidden, when relevant.

Settings groups should also document unsafe state. Recipes should not only say
what can be managed; they should explicitly say what must not be managed. Unsafe
settings should be blocked by recipes, not merely omitted from examples.

Avoid making settings groups mirror implementation details. A settings group is
not necessarily one file, one resource, one driver, or one path. It is a
user-meaningful boundary backed by whatever internal resources are required.

### Resource

A resource is the technical unit read or written by a driver. Examples:

- `~/.gitconfig` section `[user]`;
- `~/.gitconfig` section `[alias]`;
- `~/.config/nvim` file tree;
- `~/Library/Preferences/com.googlecode.iterm2.plist`;
- `com.apple.finder` defaults domain;
- a future official app export file.

Resources are normally internal. Users should only see them when explaining a
change preview, authoring recipes, or debugging.

### Driver

A driver is reviewed deterministic code shipped with the tool. Drivers own
read/export/normalize/diff/preview/apply/import/verify behavior.

Initial MVP drivers should be deliberately small:

- `file`
- `file-tree`
- `ini-file`
- `json-file`
- `yaml-file`
- `toml-file`
- `plist-file`
- `macos-defaults-readonly`

Write-capable `macos-defaults`, `manual`, and `do-not-manage` should not be
treated as ordinary MVP drivers. `manual` and `do-not-manage` are capability or
support states unless a future spec defines them as pseudo-drivers with no
write behavior.

Command-backed drivers should come later and be heavily restricted.

Downloaded recipes must not contain arbitrary executable logic. Otherwise the
recipe ecosystem becomes a remote code execution and supply-chain problem.

#### Driver interface contract

This section is normative for MVP driver implementation. A write-capable driver
must expose these operations or explicitly declare that an operation is
unsupported:

| Operation | Required behavior | Side effects allowed |
| --- | --- | --- |
| `detect` | Determine whether the live target/resource exists and is readable. | no mutation |
| `readCurrent` | Read live state into a raw capture or structured value. | read/temp only |
| `normalize` | Convert raw/current/desired input into deterministic comparison form. | none |
| `diff` | Compare normalized current and desired state. | none |
| `previewApply` | Describe writes, deletes, creates, backups, and risks. | none |
| `backup` | Capture pre-write restore material where supported. | local state only |
| `apply` | Mutate live state according to desired input. | declared target paths only |
| `verify` | Prove or fail the write result. | read/temp only |
| `restore` | Restore from a compatible backup when supported. | declared target paths only |

Driver requirements:

- `normalize` must be deterministic and versioned.
- `diff` must never reveal values that redaction marks for display redaction.
- `previewApply` must be possible without live mutation.
- `apply` must write only declared target locations and validated paths.
- Drivers must reject path traversal, unsafe symlink traversal, and selectors
  outside the recipe-declared resource boundary.
- Drivers must return typed errors: `not-found`, `permission-denied`,
  `invalid-selector`, `unsafe-path`, `secret-detected`, `lifecycle-blocked`,
  `verification-failed`, `unsupported`, and `internal-error`.
- Read-only drivers must not expose write operations through recipe overrides.

Selector contracts should be per driver:

- `file` accepts one resolved file path under one named location.
- `file-tree` accepts include/exclude globs rooted at one named location; globs
  must not escape the root.
- `ini-file` accepts section/key selectors and must define duplicate-key rules.
- `json-file` and `yaml-file` accept recipe-defined path selectors; no arbitrary
  code or expressions.
- `toml-file` accepts table/key selectors.
- `plist-file` accepts explicit key paths only.
- `macos-defaults-readonly` accepts explicit domain/key selectors and cannot
  write in the MVP.

### Lifecycle policy

Some programs should not be read from or written to while running. A recipe may
define lifecycle policy at the target, settings group, or resource level. The
most specific policy wins.

Use policy vocabulary rather than awkward booleans:

```yaml
lifecycle:
  writeWhileRunning: allowed | warn | blocked
  beforeApply: none | ask-to-quit | quit-if-running | block-if-running
  afterApply: none | reopen-if-stopped-by-tool
```

Meaning:

- `writeWhileRunning: allowed` means the recipe considers writes safe while the
  program is running.
- `writeWhileRunning: warn` means the program may overwrite, ignore, or
  partially reload changes. The change preview should warn before apply.
- `writeWhileRunning: blocked` means the tool should not write while the program
  is running unless the lifecycle policy resolves that state first.
- `beforeApply: ask-to-quit` means the tool asks the user to quit the program
  before writing.
- `beforeApply: quit-if-running` means the tool may stop the program only with
  explicit user permission or an explicit non-interactive flag.
- `beforeApply: block-if-running` means apply fails while the program is running.
- `afterApply: reopen-if-stopped-by-tool` means the tool reopens the program only
  if the tool stopped it during this apply operation.

The tool should not silently kill or restart applications. If it stops a program,
it should record that fact for the apply operation and only reopen programs it
stopped itself.

Lifecycle behavior table:

| Policy state | Interactive behavior | Non-interactive behavior |
| --- | --- | --- |
| `allowed` | proceed | proceed |
| `warn` | show warning and ask before write unless policy allows default | fail unless `--yes` is safe |
| `blocked` | stop and explain | fail with lifecycle block |
| `ask-to-quit` | ask user to quit or let tool request quit | fail with input-required |
| `quit-if-running` | allowed only after explicit confirmation | allowed only with future explicit flag |
| `block-if-running` | skip/stop while running | fail with lifecycle block |
| `reopen-if-stopped-by-tool` | ask before reopening | do not reopen unless explicitly allowed |

The ledger must record whether the app was running, whether the user accepted a
quit/reopen action, whether the app actually exited, and whether reopening
succeeded.

Example:

```yaml
targets:
  visual-studio-code:
    lifecycle:
      writeWhileRunning: warn
      beforeApply: ask-to-quit
      afterApply: reopen-if-stopped-by-tool

    settings:
      settings:
        lifecycle:
          writeWhileRunning: warn

      snippets:
        lifecycle:
          writeWhileRunning: allowed

      workspaceStorage:
        support: blocked
        reason: "Workspace-local state is not portable settings."
```

Lifecycle actions should appear in change previews:

```text
Change preview for visual-studio-code:
- Will update user settings and keybindings.
- Will skip account, workspace storage, cache, and extension secrets.
- Code is currently running.
- Writing while running may be overwritten by the app.
- Before apply: ask to quit Code.
- After apply: reopen Code only if stopped by this tool.
```

### Normalizer

Normalization is a product feature, not a hidden implementation detail. If
normalization is poor, users see noisy diffs. If it is too aggressive, the tool
silently drops important settings. If it mishandles secrets, the tool becomes
unsafe.

Every managed resource needs declared normalization and sensitivity policy.
Normalizers may:

- sort keys;
- canonicalize JSON/YAML/TOML/plist output;
- drop known-noisy keys;
- drop timestamps;
- drop machine IDs;
- strip local paths;
- preserve only selected sections;
- redact sensitive-looking values;
- normalize line endings;
- ignore file metadata.

Normalization should be implemented by drivers and tested with fixtures.

Redaction outcomes must be explicit:

| Outcome | Meaning | Save allowed? |
| --- | --- | --- |
| `known-safe` | Recipe/driver proves the value is not sensitive. | yes |
| `redacted-for-display` | Value can be saved/applied but hidden in output. | yes, with policy |
| `blocked-save` | Sensitive material would enter desired artifacts. | no |
| `redaction-unavailable` | Opaque/unknown format cannot be inspected. | only with opaque opt-in |
| `user-approved-sensitive` | User explicitly approved a sensitive portable value. | only if recipe permits |

Display redaction, artifact redaction, and save blocking are different policies.
The tool must not silently modify desired artifacts while calling that
“redaction” unless the recipe defines deterministic format-aware redaction.

### Change preview and internal plan

The product should expose change previews, not a separate plan-management
workflow in the MVP.

User-facing commands should remain simple:

```bash
dotfiles-manager diff
dotfiles-manager save --dry-run
dotfiles-manager apply --dry-run
dotfiles-manager apply
```

- `diff` should focus on current differences.
- `save --dry-run` should show what current machine settings would be saved into
  the repository without writing repository state.
- `apply --dry-run` should show the full change preview without writing target
  state.
- `apply` should show or reference the change preview before confirmation unless
  the user explicitly opts into non-interactive behavior.

Internally, the implementation may produce a plan-like object so that `diff`,
`save --dry-run`, `apply --dry-run`, and `apply` share the same safety
pipeline. That object should remain an implementation detail until there is a
strong product reason to expose it directly.

The internal change-preview object may include:

- target and selected settings;
- desired artifacts used;
- resources to inspect or mutate;
- driver operations;
- named target locations resolved to concrete paths;
- raw captures used during save, when relevant;
- generated/rendered temporary inputs, when relevant;
- normalized before/after values;
- writes, deletes, creates, and skipped changes;
- sensitivity warnings and blockers;
- lifecycle actions, such as asking the user to quit an app;
- backup requirements;
- conflict information;
- risk level and rationale;
- verification steps.

Pros of having an internal plan/change-preview object:

- gives diff, dry-run, and apply one consistent source of truth;
- makes safety checks testable;
- makes warnings and blockers easier to render consistently;
- supports backups and verification before mutation;
- keeps room for future advanced workflows.

Cons:

- can become premature architecture if treated as a product feature too early;
- may create false confidence if the preview cannot fully predict app behavior;
- adds implementation overhead before the CLI surface is stable;
- can confuse users if “plan” becomes another thing they must manage.

Recommendation: implement the internal object only to the extent needed to
support `diff`, `save --dry-run`, `apply --dry-run`, `apply`, backups, lifecycle
checks, and verification. Avoid adding user-facing plan commands or persistent
plan files in
the MVP unless a concrete workflow requires them.

### StateLedger

The v1 product can compare source and target directly. The v2 product needs a
first-class state ledger to track ownership, conflicts, recipe changes, and
verification results.

The ledger should store:

- run ID;
- timestamp;
- active profile stack;
- resolved profile hash;
- machine context such as hostname, user, OS, and architecture;
- target identity;
- setting/resource ID;
- recipe ID and recipe hash;
- driver version;
- normalizer version;
- last applied normalized hash;
- last observed actual hash;
- desired hash;
- backup path/reference;
- verification result;
- conflict markers.

Raw artifacts should be stored only when needed. Sensitive raw exports should be
ephemeral or redacted by default.

## Recipe examples

### Git recipe sketch

```yaml
schemaVersion: 1
id: git
displayName: Git
kind: cli
supportLevel: stable

commands:
    - name: git
      argv: ["git", "--version"]

locations:
  config:
    role: config
    default:
      darwin: "~/.gitconfig"
      linux: "~/.gitconfig"
    manageDefault: allowed

matches:
  platform:
    os: macos
  command:
    name: git

settings:
  user.email:
    title: User identity
    description: Name and email from ~/.gitconfig.
    artifactType: structured
    artifactSchema: dotfiles-manager.git.user-email.v1
    default: false
    resources:
      - gitconfig-user

  aliases:
    title: Aliases
    description: Git command aliases.
    artifactType: structured
    artifactSchema: dotfiles-manager.git.aliases.v1
    default: true
    resources:
      - gitconfig-alias

  includes:
    title: Include rules
    description: Git include/includeIf sections.
    artifactType: structured
    artifactSchema: dotfiles-manager.git.includes.v1
    default: false
    resources:
      - gitconfig-includes

  credentials:
    title: Credentials
    default: false
    manage: never
    reason: May expose tokens, credential helpers, or machine-specific auth.

resources:
  gitconfig-user:
    type: structured-config
    driver: ini-file
    capability: read-write
    sensitivity: personal
    source:
      location: config
    selectors:
      includeSections:
        - user
    normalize:
      sortKeys: true
      preserveComments: false
    safety:
      forbiddenKeys:
        - credential
        - credential.helper
    verify:
      readAfterWrite: true

  gitconfig-alias:
    type: structured-config
    driver: ini-file
    capability: read-write
    sensitivity: low
    source:
      location: config
    selectors:
      includeSections:
        - alias
    normalize:
      sortKeys: true
    verify:
      readAfterWrite: true

  gitconfig-includes:
    type: structured-config
    driver: ini-file
    capability: read-write
    sensitivity: machine-local
    source:
      location: config
    selectors:
      includeSections:
        - include
        - includeIf
    normalize:
      sortKeys: true
    warnings:
      - "includeIf paths may be machine-specific."
```

### Neovim recipe sketch

```yaml
schemaVersion: 1
id: nvim
displayName: Neovim
kind: cli
supportLevel: stable

commands:
    - name: nvim
      argv: ["nvim", "--version"]

locations:
  config:
    role: config
    default:
      darwin: "~/.config/nvim"
      linux: "~/.config/nvim"
    manageDefault: allowed

settings:
  config:
    title: Config files
    artifactType: file-tree
    defaultArtifact: desired://shared/-/targets/nvim/artifacts/config
    default: true
    resources:
      - nvim-config-tree

resources:
  nvim-config-tree:
    type: file-tree
    driver: file-tree
    capability: read-write
    sensitivity: low
    source:
      location: config
    selectors:
      include:
        - "**/*"
      exclude:
        - ".git/**"
        - ".local/**"
        - ".cache/**"
        - "lazy-lock.json"
    normalize:
      text:
        lineEndings: lf
      ignoreFileMetadata: true
    verify:
      compareAfterWrite: true
```

### Unsafe GUI app settings

```yaml
settings:
  account:
    title: Account state
    default: false
    manage: never
    reason: Contains login/session/cloud identity state.

  history:
    title: History
    default: false
    manage: never
    reason: Private, noisy, and not portable.

  cache:
    title: Cache
    default: false
    manage: never
    reason: Non-portable generated state.
```

## Include/exclude policy

Avoid turning include/exclude into a universal mini-language. A universal
selector language would expose too much of the implementation model and would be
hard to validate safely.

Recommended hierarchy:

1. Normal users include or exclude **targets** and **settings**.
2. Simple targets support a one-line form such as `nvim: true` or
   `starship: true`.
3. Settings lists are available only where recipes expose meaningful choices.
4. Advanced users may override named locations, include/exclude globs,
   normalization, or driver-specific behavior, but those overrides should not be
   necessary for supported programs.
5. The engine validates that overrides are legal for the setting, resource, and
   driver.

Unsafe settings should be blocked by recipes, not merely excluded by example
configuration. User config may document exclusions, but safety should not depend
on every user remembering to exclude credentials, account state, cache,
workspace-local state, or local databases.

Good normal shape:

```yaml
manage:
  git:
    settings:
      - user.email
      - aliases
      - includes
  nvim: true
  visual-studio-code:
    settings:
      - settings
      - keybindings
      - snippets
```

Good advanced layered shape when explicit artifact bindings or location
overrides are needed:

```yaml
manage:
  git:
    settings:
      user.email:
        artifact: desired://user/leon/targets/git/settings#user.email
      aliases:
        artifact: desired://shared/-/targets/git/settings#aliases
      includes:
        artifact: desired://shared/-/targets/git/settings#includes
    excludeSettings:
      - credentials

  nvim:
    settings:
      config:
        artifact: desired://shared/-/targets/nvim/artifacts/config

  visual-studio-code:
    settings:
      settings:
        artifact: desired://shared/-/targets/visual-studio-code/artifacts/settings.json
      keybindings:
        artifact: desired://shared/-/targets/visual-studio-code/artifacts/keybindings.json
      snippets:
        artifact: desired://shared/-/targets/visual-studio-code/artifacts/snippets
    locations:
      userConfig: "~/Library/Application Support/Code/User"
    excludeSettings:
      - account
      - workspace
      - cache
      - extension-secrets
```

Advanced path-level overrides should be scoped to settings that allow them:

```yaml
manage:
  nvim:
    settings:
      config:
        artifact: desired://shared/-/targets/nvim/artifacts/config
        excludePaths:
          - lazy-lock.json
          - .local/**
```

Avoid exposing this shape as the default:

```yaml
Raycast:
  exclude:
    files: ...
    plistKeys: ...
    defaultsKeys: ...
    jsonPaths: ...
```

If both `groups` and `settings` exist during migration, document one as a legacy
alias. Prefer `settings` for the v2 user-facing config.

## Save/apply and native import/export model

User-facing `save` means: read this machine's current managed settings and write
or update the repository's saved settings after preview. User-facing `apply`
means: read saved settings from the repository and make this machine match them
after preview, backup, lifecycle handling, and verification.

Use `import` and `export` mostly for app-native mechanisms and advanced docs. A
normal user is not importing or exporting files; they are saving current settings
or applying saved settings.

```text
save  = current machine -> repository
apply = repository -> current machine
sync  = guided conflict-aware save/apply/skip decision
```

Lifecycle handling should be visible but not scary. If a target may overwrite
settings while running, the user-facing prompt should be concrete:

```text
Cobona must be closed before applying cobona:user-info.

Choices:
  1. Ask Cobona to quit, apply, then reopen it
  2. I will quit it myself and retry
  3. Skip Cobona
```

The tool should not silently kill or restart apps. Recipes may support
app-specific quit/reopen behavior, but the user should see what will happen
before live state is changed.

### Generic native import/export support

Some apps already expose their own settings export/import flow. Recipes should
model this as a first-class native capability rather than as raw file scraping.
The transport can vary by app: reviewed code, a fixed CLI argv invocation, a
local API, a supported app command flow, or another deterministic adapter. The
normal CLI should stay the same:

```bash
dotfiles-manager status raycast
dotfiles-manager diff raycast
dotfiles-manager save raycast
dotfiles-manager apply raycast
```

Internal save flow for native apps:

1. Resolve active scope/profile and selected setting.
2. Run the recipe-defined native export into `temp://<run-id>/<target>/capture`.
3. Validate declared outputs, formats, sizes, and app version constraints.
4. Classify outputs as diffable structured artifacts or opaque native bundles.
5. Redact only when the format is safely parseable and the recipe declares rules.
6. Normalize only when deterministic and recipe-defined.
7. Show a save preview.
8. Write the desired artifact under `desired://...` after confirmation.
9. Record hashes, versions, and receipt metadata in local state.

Internal apply flow for native apps:

1. Resolve active scope/profile and selected setting.
2. Validate the desired artifact from `desired://...`.
3. Back up or capture pre-apply state when supported.
4. Render or prepare `temp://<run-id>/<target>/rendered` app-native input if needed.
5. Show an apply preview.
6. Run lifecycle policy, such as asking the user to quit the app when needed.
7. Run the recipe-defined native import/apply operation.
8. Verify by reading/exporting again when possible.
9. Record apply receipt and backup pointer in local state.

Raw captures and rendered temporary files should not be committed. They belong in
`state://observed/...`, `state://backup/...`, or `temp://<run-id>/...`.

### Diffable exports and opaque native bundles

Native outputs should be classified honestly.

**Diffable structured exports** are text-based, stable, and meaningful to compare:
JSON, YAML, TOML, INI, shell config, Git config, VS Code settings JSON, or app
exports with predictable structured files. For these, `status` and `diff` can
show field-level or line-level changes after deterministic normalization.

**Opaque native bundles** are ZIPs, encrypted blobs, proprietary backups, binary
archives, app-specific packages, or databases whose internals the tool cannot
safely inspect. The manager may store and apply these, but it must not pretend to
field-diff, redact, merge, or partially apply them.

For opaque bundles, the tool may:

- store the bundle as an explicit `portable-export` artifact;
- track file size, hash, source app, source machine, and captured categories;
- validate declared type and maximum size;
- require explicit opt-in when contents are encrypted or uninspectable;
- back up before apply;
- apply through the app's native import flow;
- verify by hash or app-provided metadata when possible.

For opaque bundles, the tool must not claim it can:

- show field-level diffs;
- redact unknown secrets inside the bundle;
- merge concurrent changes;
- resolve conflicts inside the bundle;
- explain every changed setting;
- safely apply undeclared subcategories.

Opaque `portable-export` contract:

- saving requires explicit opt-in for the target/setting and each replacement;
- artifact metadata must include source target identity, source app version,
  source machine/user subject, captured category IDs, size, hash, timestamp,
  encryption/passphrase policy, and verification method;
- max size must be declared by the recipe;
- passphrases are runtime prompts in the MVP unless a secret-provider contract
  exists;
- `diff` reports metadata/hash changes only;
- `apply` must confirm the category list and warn that field-level rollback is
  unavailable;
- partial apply is allowed only if the native app exposes declared categories and
  the recipe can verify them.

Example opaque diff output:

```text
$ dotfiles-manager diff raycast

raycast: changed
Readable diff unavailable: Raycast settings include an opaque native bundle.

Saved version:
  desired/machine-user/klm.mobile.macbook-pro/leon/targets/raycast/
    artifacts/settings-and-data.rayconfig
  saved: 2026-06-01 from leon@work-laptop
  sha256: 8a91...

Current machine export:
  exported: just now from this machine
  sha256: d430...

Use `dotfiles-manager save raycast` to save this machine's Raycast settings.
Use `dotfiles-manager apply raycast` to apply the saved Raycast settings.
```

### Guided sync

`sync` should not be a blind two-way merge. It should be a guided command that
turns detected differences into explicit choices. The recommended action should
come first, but the user must still choose before writes happen.

```text
raycast differs

Recommended: save snippets and quicklinks from this machine.

Choices:
  1. Save this machine into the repo
  2. Apply repo settings onto this machine
  3. Save as this-machine settings
  4. Save as me-on-this-machine settings
  5. Skip

Why not automatic merge?
  Raycast settings-and-data is an opaque native export. The manager can store or
  apply it, but cannot safely merge or show field-level diffs.
```

The guided flow should reuse the four public scopes instead of introducing a
separate duplicate-variant concept. If both versions are useful, the user saves
one as a more specific scope, such as `machine` or `machine-user`.

### Minimum native recipe metadata

A native import/export recipe should declare enough metadata for deterministic
execution, validation, and user messaging:

- target identity and supported platforms;
- detection rules;
- supported settings/categories;
- artifact type: `file-tree`, `structured`, or `portable-export`;
- diff support: supported/unsupported and why;
- native save/export operation with declared outputs;
- native apply/import operation with declared inputs;
- output formats, expected paths, and max sizes;
- deterministic normalization rules when applicable;
- format-aware redaction or an explicit `redaction: unavailable` reason;
- blocked settings/categories such as accounts, credentials, cache, history, and
  local databases;
- lifecycle requirements, such as `ask-to-quit-before-apply`;
- backup policy;
- verification strategy;
- known noise such as timestamps or machine IDs;
- whether the artifact is shared, machine-specific, user-specific, or selectable.

Profile layers may select settings and bind artifacts. They must not define
arbitrary commands.

### Custom app authoring UX

Unsupported apps should start from a wizard, not from a blank recipe file:

```bash
dotfiles-manager app create mytool
```

Shortest safe wizard:

```text
App name: mytool

Where are the settings?
  1. A file or folder path
  2. A native export/import command
  3. Both

Path or command to inspect:
  ~/.mytool/config.yaml

What should be managed?
  1. Whole file/folder
  2. Selected values
  3. Let the manager suggest safe settings from a sample

What is the default scope?
  1. Shared
  2. This user
  3. This machine
  4. This user on this machine
  5. Ask per setting

Does the app need to be closed before apply?
  1. No
  2. Yes, ask before apply
  3. Yes, ask to close and reopen it

Test now?
  1. Save from this machine
  2. Apply to a sandbox/dry-run target
  3. Skip
```

Generated app support must pass validation before it can run:

```bash
dotfiles-manager app validate mytool
dotfiles-manager app test mytool --roundtrip
```

AI may help draft this app support from user-provided paths, sample config, or
app documentation. The output is still a draft recipe that must validate and run
in dry-run/test mode before it can write to live settings.

Safety defaults for custom apps:

- never overwrite without backup;
- never follow unsafe symlinks by default;
- never run arbitrary shell strings;
- never store secrets unless the app definition explicitly marks them as safe,
  encrypted, external secret references, or excluded;
- classify opaque artifacts as opaque before `diff`, `save`, or `apply`;
- require explicit user approval before enabling write/apply behavior.

## Command-backed behavior

Recipes should be declarative by default. Command-backed behavior is an
advanced/internal execution class for native app support and trusted custom app
support; it is not a normal user concept.

The normal user sees `save` and `apply`. The recipe/driver layer may implement
those operations with app-native export/import commands, APIs, reviewed code, or
direct file drivers.

### Command-IO driver

Apps that expose import/export commands or APIs should use a constrained
`command-io` driver family. This is a driver type, not arbitrary shell hooks in
profile config.

Recipes declare command-backed resources. The driver enforces safety. A recipe
may declare:

- fixed argv arrays, never shell strings;
- typed token substitution only, such as `{{location.appCli}}` or
  `{{temp.capturePath}}`; no general template language;
- declared input and output files;
- declared output format and max size;
- timeout behavior;
- allowlisted environment;
- redaction and block rules before artifacts are written;
- normalization schema;
- lifecycle policy;
- verification strategy such as export-normalize-compare.

Profile layers may select the setting and bind the desired artifact. They must
not define arbitrary commands.

Example resource sketch:

```yaml
resources:
  exampleapp.snippets.command:
    driver: command-io

    export:
      argv:
        - "{{location.appCli}}"
        - settings
        - export
        - --scope
        - snippets
        - --format
        - json
        - --output
        - "{{temp.capturePath}}"
      outputs:
        - path: "{{temp.capturePath}}"
          format: json
          maxBytes: 1048576
      redact:
        blockJsonPaths:
          - "$..token"
          - "$..secret"
          - "$..credential"
        onMatch: fail-save
      normalize:
        schema: dotfiles-manager.exampleapp.snippets.v1
        dropFields:
          - id
          - createdAt
          - updatedAt
          - lastUsedAt
          - deviceId

    import:
      render:
        fromArtifact: "{{artifact.desired}}"
        to: "{{temp.renderedPath}}"
        format: json
      argv:
        - "{{location.appCli}}"
        - settings
        - import
        - --scope
        - snippets
        - --input
        - "{{temp.renderedPath}}"

    verify:
      strategy: export-normalize-compare
```


### Safe class: built-in command driver

The tool ships reviewed code for a known app or system API. The recipe only
configures the reviewed driver. The driver owns execution policy, redaction,
timeouts, and verification.

### Restricted class: declarative command invocation

Allowed only for bundled, signed, or local trusted recipes. Rules:

- argv array only; no shell interpolation;
- fixed executable path or resolved trusted binary;
- no pipes;
- no `curl`, `wget`, or network by default;
- declared input/output files only;
- declared environment only;
- separate read and write capabilities;
- timeout required;
- exit-code contract required;
- stdout/stderr handling declared;
- redaction declared;
- write command requires explicit user confirmation.

### Deferred class: arbitrary scripts

Arbitrary scripts are post-MVP and must not be part of the initial local recipe
contract. Even local user-authored scripts can bypass deterministic previews,
backup policy, redaction, and path safety. If script support is ever added, it
needs a separate trust model, sandbox story, prompt contract, and test matrix.
Arbitrary scripts must never be accepted from a public recipe catalog.

## AI-assisted recipe discovery

AI should be an authoring assistant, not a trusted runtime executor.

A future flow may look like:

```bash
dotfiles-manager recipe draft SomeApp --ai
```

The output should be a draft recipe plus explanation. It should not write local
configuration.

Safety boundaries:

- AI may inspect metadata and known safe locations only.
- AI may not read arbitrary file contents by default.
- AI receives redacted samples unless the user explicitly opts in.
- AI may not execute commands.
- AI may not propose write-capable recipes without human review.
- AI-generated recipes start as read-only, export-only, or do-not-manage.
- AI-generated recipes are marked untrusted.
- AI-generated recipes cannot be submitted upstream without fixtures and review.
- Secrets, account state, histories, caches, databases, cookies, tokens,
  keychains, private keys, and cloud sync state default to do-not-manage.

A bounded discovery flow:

1. Identify the app/tool/service.
2. Show detected identities and candidate config locations.
3. Classify candidates as likely config, cache, account, history, secret, or
   unknown.
4. Propose settings, using settings groups only where they add clarity.
5. Propose a read-only recipe first.
6. Let the user save a sample normalized output.
7. Only later allow write/apply support after review.

The AI should not crawl the whole home directory. Give it a bounded inventory:

- bundle metadata;
- known app support directory names;
- predefined config path candidates;
- file names without contents;
- plist keys with sensitive values redacted;
- sizes, modified times, and extensions.

## Recipe trust and distribution

Trust levels:

- **bundled** — shipped with the binary; highest trust;
- **official-catalog** — signed recipe metadata, pinned by hash;
- **local** — user-authored or repo-authored recipe;
- **generated** — AI draft, untrusted until reviewed;
- **rejected** — explicit do-not-manage decision.

Profiles should pin recipe versions and hashes:

```yaml
recipeLock:
  git:
    source: bundled
    id: git
    version: 1.2.0
    sha256: "..."
  iterm2:
    source: catalog
    id: iterm2
    version: 0.4.1
    sha256: "..."
```

Recipe updates should never silently change write behavior. If an update adds a
managed resource, changes a normalizer, changes selectors, or upgrades a driver
capability from read-only to read-write, the user should see an explicit recipe
migration change preview.

Community recipes should require:

- schema validation;
- no executable code;
- normalization fixtures;
- declared unsafe resources;
- declared app identity matching;
- review for write-capable resources;
- support level: stable, read-only, experimental, or deprecated.

## Platform and filesystem assumptions

The MVP should declare its platform boundary explicitly. Recommended MVP scope:
macOS first, with Linux path conventions allowed in schemas but not required for
initial parity. Windows, root/sudo writes, service management, launchd writes,
and live database editing are deferred.

Filesystem rules:

- `~` expands to the active OS user's home directory only.
- Named locations must resolve to absolute paths before driver execution.
- Relative paths inside recipes are resolved under one named location, never the
  process working directory.
- Drivers must reject path traversal outside their named-location root.
- Symlinks are not followed for writes unless the recipe and policy explicitly
  allow that exact symlink behavior.
- File permissions should be preserved for file/file-tree writes when safe;
  permission changes must be previewed.
- Extended attributes, ACLs, and file ownership are not portable desired state in
  the MVP unless a driver explicitly supports and previews them.
- Atomic replace should be used for single-file writes when possible. If atomic
  writes are unavailable, the driver must say so in the preview.
- Case-sensitivity differences must be detected for file-tree resources.
- Permission failures should be surfaced as blockers, not solved with implicit
  sudo prompts.

Local state paths should follow platform conventions such as XDG state/cache on
Linux and `~/Library/Application Support` or `~/Library/Caches` on macOS. The
exact local state directory remains a spec decision, but it must be outside the
desired-state repo by default.

## Security, privacy, and trust model

The safety model should be testable against concrete threats:

| Threat | Required mitigation |
| --- | --- |
| Secret leakage into repo | default deny, redaction, blocked-save states, fixtures |
| Remote recipe execution | no arbitrary code in downloaded recipes |
| Malicious local recipe | explicit trust, validation, no arbitrary scripts in MVP |
| Path traversal or symlink attack | named-location roots and driver path validation |
| Command injection | argv arrays only, typed token substitution, no shell strings |
| App database corruption | do not manage live databases by default |
| Account/session migration | account, history, cache, and cloud state blocked by recipes |
| Opaque bundle misuse | encrypted-bundle opt-in, metadata-only diff, explicit apply |
| AI-generated unsafe recipe | AI drafts only; validation and trust promotion required |

Local recipe trust rules for the MVP:

- Bundled recipes are trusted by the shipped tool version.
- User-local recipes are trusted only after validation and explicit local
  enablement.
- Local recipes may use declarative built-in drivers.
- Local recipes must not execute arbitrary scripts in the MVP.
- Local recipes may use constrained `command-io` only if the argv, timeout,
  environment, input/output, and no-shell contract is implemented and enabled
  explicitly.
- Any recipe change that broadens write scope, enables command-IO, changes
  selectors, or weakens safety should require review before apply.

AI may draft recipes, selectors, explanations, and fixtures. AI must not be a
runtime executor, must not silently read broad local state, and must not produce
write-capable recipes without deterministic validation and explicit trust
promotion.

## Safety rules

The tool should enforce these defaults:

- Default deny unknown state.
- No secrets by default.
- No account/session state by default.
- No histories by default.
- No caches by default.
- No browser cookies, app tokens, keychains, SSH private keys, API keys,
  licenses, or credential stores.
- No root/sudo writes in the MVP.
- No arbitrary shell in downloaded recipes.
- No network access from drivers unless explicitly supported and trusted.
- No raw app exports committed by default.
- No backups, ledgers, captures, or rendered temp files committed by default.
- No profile-defined arbitrary commands.
- No writes without a change preview.
- Back up before writes where technically possible.
- Do not edit live databases directly.
- Warn or block when an app must be quit before reading/writing.
- Every write-capable resource must have a verifier.
- Every recipe must declare sensitivity and capability per setting.
- Every diff must redact sensitive-looking values.
- Recipe updates must be reviewed before changing managed scope.

## Non-goals

This product should not become:

- a package manager;
- a Homebrew replacement;
- a Nix/Home Manager clone;
- an MDM system;
- a full machine backup tool;
- a secret manager;
- a cloud sync service;
- a migration assistant for all app data;
- a tool for cloning app accounts, sessions, caches, histories, or licenses;
- a generic automation runner.

## Relationship to classical dotfiles

Classical dotfiles should be generalized, not abandoned.

Internally:

```text
dotfile path -> file resource -> file driver
dotfile directory -> file-tree resource -> file-tree driver
```

Product-wise, keep the existing v1 workflow until the v2 engine reaches parity.
Do not force every dotfiles user to adopt recipes immediately.

Migration path:

```text
legacy .dotfiles-manager.yaml syncs
  -> generated custom.files target
  -> file/file-tree resources
  -> optional mapping into known targets like zsh, nvim, git
```

A user should be able to keep a file-oriented config and later evolve to a
higher-level target/settings config.

### v1 compatibility and migration contract

The v2 engine must prove v1 parity before replacing the existing dotfile sync
path. Migration should be preview-first and reversible.

Compatibility rules:

- Existing `syncs:` configs remain readable through a legacy adapter.
- Existing source and target paths must be preserved exactly unless the user
  confirms a migration.
- v1 command aliases may continue to exist, but v2 docs should prefer
  `save`/`apply`.
- A migrated v1 entry becomes a `custom.files` target backed by `file` or
  `file-tree` resources.
- Optional promotion from `custom.files` into known targets such as `git`,
  `zsh`, or `nvim` must be a separate previewed step.

Migration command behavior:

```bash
dotfiles-manager migrate --dry-run
dotfiles-manager migrate
```

`migrate --dry-run` should show:

```text
legacy sync: zshrc
  source: dotfiles/zsh/.zshrc
  target: ~/.zshrc
  v2 target: custom.files:zshrc
  driver: file
  action: generate profile selection and desired artifact binding
```

Migration must not delete v1 config by default. It should write new v2 config or
a migration branch/file, then let the user compare behavior.

Parity acceptance tests before defaulting to v2:

- v1 status/diff/deploy/import scenarios have equivalent v2 behavior.
- v1 ignored/missing path behavior is preserved or explicitly changed with a
  migration warning.
- v1 file and directory syncs round-trip through `custom.files`.
- Existing dotfile users can continue without learning recipes.

## Raycast and GUI app guidance

Raycast is a good example of native app import/export support rather than raw
file reverse engineering.

Normal users should see the same simple flow as for any supported app:

```bash
dotfiles-manager add raycast
dotfiles-manager status raycast
dotfiles-manager diff raycast
dotfiles-manager save raycast
dotfiles-manager apply raycast
```

The Raycast recipe should model at least two artifact classes:

1. **Structured JSON exports** for settings that Raycast can export separately,
   such as snippets and quicklinks. These can become normal desired artifacts:

   ```text
   desired/shared/-/targets/raycast/artifacts/snippets.json
   desired/shared/-/targets/raycast/artifacts/quicklinks.json
   ```

2. **Native `.rayconfig` exports** for Raycast's own settings-and-data bundle.
   This is an opaque, encrypted, app-native portable export. It can be useful for
   restoring settings, aliases, hotkeys, store extensions, or window layouts, but
   it is not a normal diffable settings file. Because window layouts and some
   app-local details may be display- or machine-dependent, the default example
   stores this bundle at machine-user scope:

   ```text
   desired/machine-user/<machine-id>/<user-id>/targets/raycast/artifacts/settings-and-data.rayconfig
   ```

The `.rayconfig` path must therefore be explicit and conservative:

- classify it as `portable-export`;
- require `storagePolicy: encrypted-opt-in` before storing it in a repository;
- keep the Raycast export passphrase in a secret manager when secret support is
  enabled, or prompt at runtime in the MVP;
- use recipe-defined stable category IDs, for example
  `settings-aliases-hotkeys`, `store-extensions`, and
  `window-management-layouts`; verify exact IDs against Raycast's supported
  import/export interface before turning them into a schema contract;
- show that the artifact may contain categories beyond ordinary settings;
- use Raycast's selective import command flow to apply only declared categories;
- treat account, cloud sync, extension secrets, AI chats, clipboard history,
  history, cache, and local databases as outside normal settings management;
- do not claim field-level previews, merges, or redaction for opaque encrypted
  exports.

For `.rayconfig`, `diff` should explain that the bundle changed but readable diff
is unavailable. `save` can replace the saved bundle after confirmation. `apply`
can apply the saved bundle through Raycast's native import flow after backup and
category confirmation.

This means Raycast can be supported through native export/import without making
Raycast a precedent for arbitrary GUI-app state cloning.

This same guarded approach applies to iTerm2, Klack, and other GUI apps. The
system should support them, but the MVP should prove safety and state handling
with boring, deterministic resources first.

## Support levels and capabilities

Support level and capability are separate axes.

Support levels:

| Support level | Meaning | Write/apply allowed? |
| --- | --- | --- |
| `stable` | Tested and supported for declared versions/platforms. | yes, if capability allows |
| `read-only` | Safe to inspect/status/diff only. | no |
| `experimental` | Available with warnings and weaker compatibility guarantees. | only with explicit opt-in |
| `deprecated` | Existing support remains temporarily; new saves may be blocked. | limited |
| `blocked` | Known unsafe or unsupported. | no |

Capabilities:

| Capability | Meaning |
| --- | --- |
| `inspect-only` | Can detect and explain, but not read values. |
| `read-only` | Can read/status/diff, but not write. |
| `read-write` | Can save and apply through deterministic driver behavior. |
| `import-only` | Can apply through native import, but cannot safely save. |
| `export-only` | Can save through native export, but cannot safely apply. |
| `never` | Explicit do-not-manage boundary. |

A write-capable setting should declare both support level and capability.
Examples: `supportLevel: stable` plus `capability: read-write`, or
`supportLevel: experimental` plus `capability: export-only`.

## MVP acceptance test matrix

MVP implementation should not start without fixture-based acceptance criteria.
At minimum, tests should cover:

| Area | Required fixtures/tests |
| --- | --- |
| CLI | text and `--json` snapshots for every normal command and exit code |
| Status state machine | each canonical status code and target-level aggregation |
| File driver | create/update/delete preview, backup, restore, symlink rejection |
| File-tree driver | include/exclude globs, case conflicts, unsafe traversal, metadata policy |
| Structured drivers | INI/JSON/YAML/TOML selectors, invalid selectors, normalization fixtures |
| Plist/defaults | selected keys only, read-only macOS defaults, unsupported write attempts |
| Redaction | display redaction, blocked save, redaction unavailable, known-safe values |
| Lifecycle | running app allowed/warn/blocked, quit declined, reopen failure |
| Native export | diffable export, opaque export, passphrase prompt, size/category limits |
| Ledger | last-applied update only after verified success, partial failure recording |
| Restore | restore preview, backup-before-restore, unsupported restore path |
| Migration | v1 `syncs:` parity and generated `custom.files` target |
| Trust | untrusted local recipe, recipe change broadening write scope, command-IO gate |

## Suggested MVP

The MVP should validate the abstraction without building the whole dream.

Build:

- normal command surface: `add`, `list`, `status`, `diff`, `save`, `apply`, and
  guided `sync`;
- installed-target discovery and recommended safe defaults for bundled targets;
- user-facing status grouped by next action, blocker, and recommendation;
- guided scope prompts using plain-language labels and recipe defaults;
- profile stack/layer schema with explicit artifact bindings;
- optional settings selection for complex targets such as Git, VS Code/Cursor,
  iTerm2, and Raycast;
- target catalog;
- bundled and user-local recipes only;
- no remote recipe catalog in the MVP;
- desired-state artifact conventions and schema validation;
- recipe-defined named target locations with user overrides;
- conservative default exclusions for secrets, credentials, account state, cache,
  runtime state, workspace-local state, and opaque local databases;
- change-preview pipeline and dry-run rendering;
- lifecycle checks and friendly quit/reopen prompts for programs that may
  overwrite settings while running;
- local state ledger;
- local backup store;
- conflict detection using last-applied hashes;
- secret redaction;
- recipe explain command;
- legacy dotfiles compatibility adapter;
- file driver;
- file-tree driver;
- INI driver;
- JSON driver;
- YAML driver;
- TOML driver;
- plist-file driver;
- macos-defaults-readonly driver for selected read-only defaults;
- constrained native import/export capability for bundled, reviewed app support.

Initial bundled recipes:

- `git` — selected `~/.gitconfig` sections, excluding credentials;
- `zsh` — `~/.zshrc`, `~/.zprofile`, optional `~/.zshenv` with warnings;
- `nvim` — `~/.config/nvim` file tree;
- `tmux` — `~/.tmux.conf`;
- `ssh` — `~/.ssh/config` only, never keys;
- `starship` — `~/.config/starship.toml`;
- `custom.files` — legacy file/file-tree resources;
- `raycast` — native snippets/quicklinks when diffable; optional opaque
  `.rayconfig` only with explicit encrypted-bundle opt-in;
- `iTerm2` — experimental/read-only preferences;
- `macos.finder` and `macos.dock` — selected read-only defaults first.

Defer:

- user-facing plan commands;
- persisted plan files;
- exposing resources and drivers in normal workflows;
- AI discovery;
- remote recipe catalog;
- signed downloads;
- unreviewed command-backed save/apply for custom or catalog recipes until the
  constrained native/command IO safety model is implemented;
- arbitrary recipe scripts;
- launchd writes;
- postgres/nginx service management beyond plain file resources;
- general app reverse engineering;
- treating Raycast `.rayconfig` exports as diffable or mergeable settings;
- cross-machine merge;
- persistent secret manager integration beyond runtime prompts.

Success criterion:

> A user can set up a new Mac's core CLI/editor environment from target names
> and recommended settings, without editing URIs or recipes, while still getting
> deterministic diffs, change previews, backups, and conflict detection.

## Roadmap decomposition

### Phase 0: rename and framing

- Decide whether the product remains `dotfiles-manager` or gets a broader name.
- Keep compatibility commands even if the binary is eventually renamed.
- Define product vocabulary: profile layer, profile stack, resolved profile,
  target, setting, desired artifact, named target location, recipe, change
  preview, ledger.
- Write safety principles and non-goals.
- Define migration story from the existing dotfiles model.

### Phase 1: core engine and change-preview pipeline

- Implement profile stack/layer schema v1.
- Implement desired artifact binding and resolution.
- Implement target registry.
- Implement recipe schema v1.
- Implement settings selection model for complex targets.
- Implement named target location resolution and user overrides.
- Implement lifecycle policy checks.
- Implement driver interface.
- Implement shared change-preview pipeline for `diff`, `save --dry-run`,
  `apply --dry-run`, and `apply`.
- Implement state ledger.
- Implement backup store.
- Implement redacted diff renderer.
- Implement schema validation.
- Implement recipe explain output.

Success criterion: a recipe can declare file resources, the engine can produce a
change preview, and the user can understand what will happen.

### Phase 2: file/dotfile parity

- Implement file driver.
- Implement file-tree driver.
- Implement save/status/diff/apply for files.
- Implement conflict detection using last-applied hashes.
- Implement pre-apply backup.
- Implement legacy dotfiles config adapter.
- Implement migration command from old config to profile/resources.

Success criterion: current `dotfiles-manager` functionality works through the
new engine.

### Phase 3: structured config drivers

- Implement INI driver.
- Implement JSON driver.
- Implement YAML driver.
- Implement TOML driver.
- Implement plist-file driver.
- Implement normalizer fixtures.
- Implement selector validation per driver.
- Implement secret detection/redaction pass.

Success criterion: git, starship, tmux, nvim, and simple plist-backed apps are
manageable without custom code.

### Phase 4: bundled recipes

- Add git recipe.
- Add zsh recipe.
- Add nvim recipe.
- Add tmux recipe.
- Add ssh config-only recipe.
- Add starship recipe.
- Add custom.files recipe.
- Add Raycast snippets/quicklinks recipe support and optional opaque `.rayconfig`
  support behind encrypted-bundle opt-in.
- Add experimental iTerm2 recipe.
- Add experimental macOS Finder/Dock read-only recipes.
- Add recipe test fixtures.

Success criterion: a user can set up a new Mac's core CLI environment from
names and settings.

### Phase 5: safety hardening

- Implement sensitivity classifications.
- Implement forbidden resource declarations.
- Implement redacted diffs.
- Implement app-running checks.
- Implement permission diagnostics.
- Implement change-preview risk levels.
- Implement recipe trust levels.
- Implement recipe lock file.
- Implement update review flow.

Success criterion: the tool is difficult to misuse accidentally.

### Phase 6: local app/recipe authoring

- Implement `app create`.
- Implement `app validate`.
- Implement `app test --roundtrip` with fixtures.
- Implement `recipe explain`.
- Implement local recipe override path.
- Implement generated recipe skeletons from known driver templates.

Success criterion: advanced users can author recipes without learning the whole
engine internals.

### Phase 7: catalog/distribution

- Define catalog format.
- Implement signed recipe index.
- Implement recipe download.
- Implement recipe pinning.
- Implement recipe update diff.
- Implement trust prompts.
- Implement support levels: stable, read-only, experimental, deprecated.
- Implement community contribution checklist.

Success criterion: remote recipes are useful without becoming an execution-risk
channel.

### Phase 8: AI-assisted discovery

- Implement bounded inventory collector.
- Implement redaction layer for discovery.
- Implement AI draft recipe command.
- Implement candidate resource classifier.
- Implement draft recipe review UI.
- Implement read-only generated recipe mode.
- Implement fixture generation from sanitized samples.
- Implement submit-PR flow.

Success criterion: AI helps create recipes but cannot silently read secrets or
execute writes.

### Phase 9: carefully expand beyond bundled reviewed support

- Evaluate broader native/command-backed save/apply support beyond bundled,
  reviewed recipes.
- Harden the no-shell argv contract with timeout/env/path/network restrictions.
- Add launchd read-only recipe.
- Add launchd write support only after a strong verification model.
- Evaluate service configs like postgres/nginx as file resources first.

Success criterion: expansion happens by capability and safety evidence, not by
hype.

## Likely failure modes

The product may fail if:

- it promises “restore my apps” while only restoring selected settings;
- recipes become too powerful and turn into supply-chain execution units;
- AI discovery becomes magical but unreliable;
- diffs are noisy because normalization is weak;
- normalization silently drops important state;
- secrets leak into the repository;
- the recipe ecosystem lacks tests, owners, compatibility bounds, and support
  levels;
- classical dotfiles users feel displaced;
- the tool manages live app databases or opaque app support folders;
- conflict detection is bolted on later instead of built into the core model.

The mitigation is to keep the MVP narrow, deterministic, explainable, and
compatible with existing file-based workflows.

## Open decisions

The main product direction in this document is intentionally decided: normal
verbs are `save`/`apply`/guided `sync`; public setting references use
`<target-id>:<setting-id>`; public scopes are `shared`, `user`, `machine`, and
`machine-user`; and desired-state repository paths encode scope before target.

Spec handoff decisions to resolve in formal specs/issues:

1. Product name: keep `dotfiles-manager`, rename, or support a compatibility
   alias?
2. Exact local state directory, backup directory, and retention policy.
3. Exact schema filenames and JSON Schema locations for config, profiles,
   manifests, recipes, previews, ledgers, and backups.
4. Exact support matrix for macOS-only MVP versus macOS+Linux MVP.
5. How to represent machine-specific values without turning the tool into a
   templating engine.
6. Exact selector grammar per structured driver.
7. Whether constrained `command-io` is included in MVP local recipes or deferred
   until after file/structured drivers ship.
8. Which GitHub issues/projects should own the roadmap phases.

## Bottom line

Do not build an “AI-powered universal Mac state manager.”

Build a deterministic local settings manager where users add apps/tools, see
what changed, save current settings into a repo, and apply saved settings onto a
machine. Internally, this can be backed by visible recipes, safe built-in
drivers, clear change previews, backups, conflict detection, and strong
default-deny safety.

That product can grow into AI-assisted recipe discovery and broader app support
later. The foundation should be boring, auditable, and safe.
