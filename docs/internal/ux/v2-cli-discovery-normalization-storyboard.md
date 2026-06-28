---
owner: Product + Core Engineering
status: Design evidence for #252 focused slice
last-updated: 2026-06-28
source-issue: 252
related-issues:
  - 228
related-prs:
  - 253
---

# v2 CLI discovery normalization storyboard

This storyboard is pre-implementation design evidence for the first #252 slice:
make normal discovery app/tool-oriented and flattened so #228 catalog discovery
can use the accepted command model.

The frozen slice is intentionally narrower than all of #252. It covers:

- `dotfiles-manager list` as normal supported-app discovery;
- `dotfiles-manager search <query>` as read-only supported-app search;
- `dotfiles-manager explain <app>` as normal app/tool support explanation;
- an explicit compatibility path for the previous selected-settings `list`
  output.

It does **not** implement `manage`, `unmanage`, catalog lifecycle commands,
remote catalogs, or changes to sync execution semantics.

## Product model

Normal users should think:

```text
dotfiles-manager supports apps/tools.
I can list or search supported apps.
I can explain one app before deciding whether to manage it.
Recipes are implementation details for authors and debugging.
```

Command namespaces:

```text
Normal users:
  list
  search <query>
  explain <app>
  status [<app-or-setting>]
  diff [<app-or-setting>]
  sync [<app-or-setting>]
  save [<app-or-setting>]
  apply [<app-or-setting>]

Advanced support-source users:
  catalog ...

Recipe authors/debuggers:
  recipe ...
```

## Compatibility decision for this slice

The existing runtime already has `dotfiles-manager list`, but it lists selected
managed settings from the active profile. This slice changes the default meaning
of `list` to supported-app discovery, because that is the accepted normal-user
model for #228 and later.

To avoid silently deleting useful existing behavior, the previous selected-
settings list remains available through an explicit compatibility flag:

```bash
dotfiles-manager list --settings
```

Compatibility rules:

- `list` without `--settings` is safe read-only supported-app discovery and does
  not require an initialized settings folder.
- `list --settings` keeps the selected-settings output and requires a v2 settings
  folder, just like the old `list` command.
- `list --settings --json` preserves the previous selected-settings JSON schema
  (`dotfiles-manager.v2.list`) for compatibility.
- `recipe list` remains available as an advanced/debug command, but normal help
  and examples should prefer top-level `list`.

A future #252 follow-up may choose a clearer permanent home for selected-settings
listing if `list --settings` is not the final desired shape.

## Transcript 1: list supported apps before initialization

Command:

```bash
dotfiles-manager list
```

Expected output:

```text
Supported apps

  APP           SOURCE    STATE
  custom.files  built-in  not managed
  git           built-in  not managed
  nvim          built-in  not managed
  ssh           built-in  not managed
  starship      built-in  not managed
  tmux          built-in  not managed
  zsh           built-in  not managed

Use `dotfiles-manager explain <app>` to see what can be managed.
Use `dotfiles-manager list --settings` to list selected managed settings.

No live settings were read or changed.
No stored settings were changed.
```

Design note: because the settings folder is not initialized, every app is shown
as `not managed`. The command must not create local identity or state records.

## Transcript 2: list supported apps with an initialized settings folder

Precondition: `git` has selected settings in the active profile.

Command:

```bash
dotfiles-manager list --user-id leon
```

Expected output:

```text
Supported apps

  APP           SOURCE    STATE
  custom.files  built-in  not managed
  git           built-in  managed
  nvim          built-in  not managed
  ssh           built-in  not managed
  starship      built-in  not managed
  tmux          built-in  not managed
  zsh           built-in  not managed

Managed apps:
  git  2 selected settings

Use `dotfiles-manager status git` to inspect drift for Git.
Use `dotfiles-manager list --settings` to see selected settings.

No live settings were read or changed.
No stored settings were changed.
```

Design note: detecting `managed` may read the settings folder profile metadata
and local identity metadata. It must not read live app settings.

## Transcript 3: compatibility selected-settings list

Command:

```bash
dotfiles-manager list --settings --user-id leon
```

Expected output:

```text
Selected settings

Profile stack: default
Subject: user leon

  git:user.email — User email
    Scope: user
    Stored settings: stored

  git:user.name — User name
    Scope: user
    Stored settings: not stored yet

Inspect drift:
  dotfiles-manager --config dotfiles-manager.v2.yaml diff git:user.email
```

Design note: this is the previous `list` behavior under an explicit flag. It is
allowed to keep the existing exact text/JSON except for any heading or hint
updates needed to explain the compatibility flag.

## Transcript 4: search supported apps

Command:

```bash
dotfiles-manager search git
```

Expected output:

```text
Search results for "git"

  APP  SOURCE    STATE
  git  built-in  not managed

Use `dotfiles-manager explain git` to see what can be managed.
No live settings were read or changed.
No stored settings were changed.
```

Command:

```bash
dotfiles-manager search shell
```

Expected output:

```text
No supported apps found for "shell".

Try:
  dotfiles-manager list

No live settings were read or changed.
No stored settings were changed.
```

Design note: this slice searches app IDs, display names, aliases, and summaries
from existing bundled support metadata. Local-catalog candidates, collisions,
and richer catalog provenance remain #228 work.

## Transcript 5: explain supported app

Command:

```bash
dotfiles-manager explain git
```

Expected output:

```text
Git is supported.

App ID: git
Source: built-in support from dotfiles-manager
State: not managed

Can manage:
  git:user.email  Git user email
  git:user.name   Git user name

Does not manage:
  credential.helper
  [credential] sections
  include/includeIf expansion

Why this source is used:
  Built-in support is the default for Git and is trusted by the
  dotfiles-manager release.

No live values were printed.
No live settings were changed.
No stored settings were changed.
```

Design note: top-level `explain` may internally reuse existing `recipe explain`
metadata, but default text must be app/tool-oriented rather than recipe-oriented.
Local-catalog candidate/collision explanation remains #228 work; this slice may
ship built-in support metadata only.

## Transcript 6: explain unknown app

Command:

```bash
dotfiles-manager explain missing
```

Expected output:

```text
App not supported: missing

Try:
  dotfiles-manager search missing
  dotfiles-manager list

No live settings were read or changed.
No stored settings were changed.
```

Expected behavior:

- exit code `2`;
- JSON/text error code `explain.app.notSupported`;
- do not create local state;
- do not read live settings;
- do not change stored settings.

## Transcript 7: advanced recipe namespace remains available

Command:

```bash
dotfiles-manager recipe list
```

Expected output shape:

```text
recipe list
...
```

Design note: this command remains for recipe authors/debuggers. It should not be
advertised as the normal discovery path in root help or user-facing examples.

## Root help contract

Root help should show the flattened normal discovery commands directly:

```text
Available Commands:
  list       List supported apps/tools
  search     Search supported apps/tools
  explain    Explain support for one app/tool
  status     Show drift and candidate operations
  diff       Show unified patch previews for candidate changes
  sync       Sync safe v2 settings changes between live settings and stored settings
  save       Compatibility alias: sync live settings to stored settings
  apply      Compatibility alias: sync stored settings to live settings
  recipe     Advanced: inspect recipe metadata for authors/debugging
```

The exact Cobra formatting can differ, but tests should verify that root help:

- includes top-level `list`, `search`, and `explain`;
- does not present `recipe list` as the normal discovery path;
- keeps `recipe` wording advanced/debug-oriented if it appears.

## JSON contract

This slice introduces stable minimum JSON contracts for the new normal discovery
commands. Implementations may add fields later, but tests for this slice should
cover at least the fields below.

### `list --json`

```json
{
  "schema": "dotfiles-manager.v2.apps",
  "schemaVersion": 1,
  "command": "list",
  "runId": "app-list",
  "summary": {"status": "ok", "apps": 7, "managed": 1},
  "apps": [
    {
      "id": "git",
      "displayName": "Git",
      "aliases": ["gitconfig"],
      "source": "built-in",
      "state": "managed",
      "selectedSettings": 2,
      "recipeRef": "recipe://bundled/git",
      "trustStatus": "trusted",
      "supportLevel": "experimental",
      "capability": "read-write",
      "platformSupport": "unknown",
      "summary": "Manage selected non-credential Git identity settings."
    }
  ],
  "diagnostics": []
}
```

`state` is `managed` when at least one selected setting exists for the app in
the active profile stack; otherwise it is `not-managed`. `list --json` exits `0`
when no settings folder exists and reports all built-in apps as `not-managed`.

### `search <query> --json`

```json
{
  "schema": "dotfiles-manager.v2.apps",
  "schemaVersion": 1,
  "command": "search",
  "runId": "app-search",
  "query": "git",
  "summary": {"status": "ok", "apps": 1, "managed": 0, "matches": 1},
  "apps": [
    {"id": "git", "displayName": "Git", "source": "built-in"}
  ],
  "diagnostics": []
}
```

A no-match search exits `0`, uses `summary.status: ok`, and returns
`summary.matches: 0` with an empty `apps` array. An empty query is invalid, exits
`2`, and uses error code `search.query.invalid`.

### `explain <app> --json`

```json
{
  "schema": "dotfiles-manager.v2.app",
  "schemaVersion": 1,
  "command": "explain",
  "runId": "app-explain",
  "summary": {"status": "ok", "apps": 1, "managed": 0},
  "app": {
    "id": "git",
    "displayName": "Git",
    "source": "built-in",
    "sourceDescription": "built-in support from dotfiles-manager",
    "state": "not-managed",
    "selectedSettings": 0,
    "recipeRef": "recipe://bundled/git",
    "trustStatus": "trusted",
    "supportLevel": "experimental",
    "capability": "read-write",
    "platformSupport": "unknown",
    "settings": [
      {"ref": "git:user.email", "id": "user.email", "label": "User email"}
    ],
    "doNotManage": ["credential.helper"]
  },
  "diagnostics": []
}
```

Unsupported apps exit `2`, use `summary.status: error`, and use error code
`explain.app.notSupported`. The command must not fall back to recipe-oriented
text for normal output.

### `list --settings --json` compatibility

`list --settings --json` preserves the previous selected-settings JSON contract:

- `schema: dotfiles-manager.v2.list`;
- `command: list`;
- `runId: list-managed`;
- selected settings remain under `list.settings`;
- the command still requires a v2 settings folder and reports the existing
  `list.root.notFound` error when run outside one.

## Acceptance notes for this slice

A PR for this slice should provide real-result evidence for:

- `list` before initialization;
- `list` with managed/not-managed state in a fixture settings folder;
- `list --settings` compatibility behavior;
- `search <query>` match and no-match;
- `explain <app>` supported and unknown-app cases;
- root help showing the normal flattened surface;
- no live-setting mutation during discovery commands;
- no stored-setting mutation during discovery commands;
- no local identity/state creation for `list`, `search`, or top-level `explain`.
