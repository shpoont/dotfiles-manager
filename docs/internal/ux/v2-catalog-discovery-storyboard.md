---
owner: Product + Core Engineering
status: Recontracted design evidence for #228
last-updated: 2026-07-02
source-issue: 228
related-issues:
  - 214
  - 227
  - 229
  - 252
supersedes:
  - older #228 local-catalog storyboard/transcripts
  - PR #255 superseded local-catalog implementation scope
---

# v2 catalog discovery storyboard

This storyboard is pre-implementation design evidence for the recontracted #228
work item. It supersedes earlier #228 storyboards that exposed internal
pseudo-app targets or catalog lifecycle commands in normal discovery.

Project Owner decisions recorded on 2026-06-30:

```text
Normal discovery should show real supported apps/tools only.
Internal/generated pseudo-app targets are not normal app/tool entries.
Catalog lifecycle commands are removed from #228 and from the normal user path.
The default catalog should be the dotfiles-manager official catalog, not a
user-facing "built-in" catalog.
The official catalog is preconfigured by the app, but it is not presented as
baked into the app.
Catalog list should show useful state such as version and last-updated time.
Remote catalog download/update/add/remove behavior is handled by #229.
```

## Accepted user-facing direction

```text
Users manage apps/tools.
Apps/tools are supported by recipes/support definitions.
The dotfiles-manager official catalog is preconfigured.
Normal catalog output shows the catalog name, version, and last-updated time.
Additional support comes later through official-catalog download/update behavior
and additional remote catalogs (#229).
Recipe remains an advanced authoring/debugging noun, not the normal discovery noun.
```

## Boundaries

#228 should implement the official-catalog discovery baseline and the normal
app/tool-oriented discovery/explanation surface that depends on it:

- `list` — show real supported apps/tools from the current official catalog
  metadata and whether each is managed;
- `search <query>` — find supported apps/tools from the current official
  catalog metadata;
- `explain <app>` — explain official-catalog support, managed settings,
  and important exclusions;
- `catalog list` — show the official catalog with useful state: version and
  last-updated time.

Out of scope for #228 after recontracting:

- internal/generated pseudo-app targets in the normal `list`/`search` happy path;
- public arbitrary-file management under a pseudo-app name;
- local or remote catalog add/list/disable/enable/remove lifecycle;
- local-only app candidates, local/official collisions, or local unavailable
  source states;
- implementing first-run download or update of the official catalog;
- adding, updating, disabling, removing, signing, caching, or integrity-checking
  additional remote catalogs;
- remote write grants or live-write authority;
- full `manage`/`unmanage` command implementation unless separately
  recontracted;
- broader compatibility/deprecation behavior for existing `recipe ...` and
  `app ...` authoring/debugging commands (#252 / future focused cleanup).

Remote catalog commands such as `catalog add <owner>/<repo>` are the planned
normal extension path, but they are omitted from the #228 public/mock surface
until #229 implements catalog lifecycle behavior. `catalog update` is likewise
omitted until update behavior is implemented.

## UX principles

- The normal path is app-first/action-first, not recipe-first.
- `list`, `search`, and `explain` are safe read-only discovery commands.
- Do not repeat "nothing was read/changed" safety footers in normal discovery
  output; they add noise for little user value. Reserve explicit safety notices
  and confirmations for commands that inspect or change real settings.
- `explain <app>` describes support and must not read or print live setting
  values.
- Normal users should see the default source as the **official catalog**, not as
  a "built-in catalog".
- Do not present the official catalog as baked into or included with the app.
- `catalog list` should avoid source/cache/network/removable detail in normal
  output; show version and last-updated time.
- Downloading/updating the official catalog and adding additional remote catalogs
  are #229 responsibilities, not #228 responsibilities.
- Normal `list` should not include internal/generated pseudo-app mechanisms.
- If a user wants support for an app that is not in the current official
  catalog, the future path is refreshed official support data or an additional
  remote catalog, not a local catalog tip in default output.
- Local recipe authoring/debugging may exist as an advanced/internal capability,
  but it is not #228 normal-user product scope.
- Before any operation that can write live settings, the effective recipe/source
  and write authority must still be visible in the sync/status surface owned by
  the relevant implementation issue.
- The word `recipe` can appear in advanced/debug details, JSON, validation
  errors, and authoring docs, but normal users should be able to discover support
  without learning it.

## Command vocabulary used by these transcripts

```text
dotfiles-manager list
dotfiles-manager search <query>
dotfiles-manager explain <app>
dotfiles-manager status [<app>...]
dotfiles-manager diff [<app>...]
dotfiles-manager sync [<app>...]
dotfiles-manager save [<app>...]
dotfiles-manager apply [<app>...]

dotfiles-manager catalog list

dotfiles-manager recipe ...                 # advanced authoring/debugging only
```

`save` and `apply` remain directional sync aliases:

- `save` = live settings to settings storage folder;
- `apply` = settings storage folder to live settings.

`manage` means enroll an app/tool so it participates in status/diff/sync by
default. `unmanage` means stop tracking it by default. Neither command should
silently overwrite live settings, uninstall apps, delete live settings, or delete
stored settings. #252 owns full command cleanup for those verbs unless a focused
work item explicitly changes that.

## Transcript 1: list official-catalog apps

Command:

```bash
dotfiles-manager list
```

Expected output:

```text
Supported apps

  APP       CATALOG   STATE
  git       official  not managed
  nvim      official  not managed
  ssh       official  not managed
  starship  official  not managed
  tmux      official  not managed
  zsh       official  not managed

Use `dotfiles-manager explain <app>` to see what can be managed.
```

User takeaway: the tool has real app/tool support from the current official
catalog metadata. Internal/generated pseudo-app targets are intentionally absent
from normal `list`.

## Transcript 2: search supported apps

Command:

```bash
dotfiles-manager search git
```

Expected output:

```text
Search results for "git"

  APP  CATALOG   STATE
  git  official  not managed

Use `dotfiles-manager explain git` to see what can be managed.
```

User takeaway: search is app/tool-oriented and does not require recipe terms.

## Transcript 3: search an unsupported app

Command:

```bash
dotfiles-manager search wezterm
```

Expected output:

```text
No supported apps found for "wezterm".

The current official catalog supports:
  git, nvim, ssh, starship, tmux, zsh

This version searches only the current official catalog.
Future versions may refresh official support data or add remote catalogs.
This version cannot do that yet.
```

User takeaway: there is a future extension path, but the default output does not
push a local catalog workaround.

## Transcript 4: explain official-catalog support

Command:

```bash
dotfiles-manager explain git
```

Expected output:

```text
Git is supported.

App ID: git
Catalog: official
State: not managed

Can manage:
  git:user.email  Git user email
  git:user.name   Git user name

Does not manage:
  credential.helper
  [credential] sections
  include/includeIf expansion

```

Advanced/verbose output may include `recipe://official/git`, catalog IDs,
origin URIs, snapshot digests, and source IDs. Default output keeps those details
secondary.

## Transcript 5: list catalogs

Command:

```bash
dotfiles-manager catalog list
```

Expected output:

```text
Catalogs

Catalogs define app/tool support; they do not store your settings.

  dotfiles-manager/official  active for discovery
    Catalog version: 9f2c7a1
    Catalog updated: 2026-06-30 18:00 UTC
```

User takeaway: users can see which official catalog data they are using without
reading source/cache/network/removable implementation details.

## JSON and advanced-output notes

Text output should be app/tool-oriented. JSON and verbose output may expose
implementation fields needed by tests and advanced users, including:

- `sourceKind`;
- `sourceId`;
- `catalogId`;
- `sourceDisplayName`;
- `originUri`;
- `snapshotDigest`;
- `recipeRef`;
- `recipeDigest`;
- `reviewStatus`;
- `writeAuthority`;
- `selectedBy`.

Unknown source kinds, enum values, capabilities, schemas, path declarations, or
unsafe write surfaces must fail closed.

## Acceptance notes for #228 implementation

A #228 implementation PR should use this storyboard and the runnable mock in
`docs/internal/ux/mocks/v2-catalog-discovery/` as design evidence, then provide
real-result verification with:

- targeted catalog/discovery tests and `go test ./...` where practical;
- temp-home/fixture command output for official-catalog metadata discovery;
- proof that internal/generated pseudo-app targets are not shown as normal
  apps/tools in `list` or `search`;
- proof that local catalog lifecycle is not implemented or promoted in #228;
- proof that remote catalog add/update/write behavior was not introduced by
  #228;
- proof that #228 uses deterministic official catalog metadata and does not
  implement first-run download or update behavior;
- proof that #255's prior local-catalog implementation scope is not treated as
  accepted #228 behavior;
- proof that runtime output and tests are reconciled with the refined design:
  normal discovery should not keep old repeated "nothing was read/changed"
  safety footers.
