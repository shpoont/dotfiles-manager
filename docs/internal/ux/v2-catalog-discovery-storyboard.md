---
owner: Product + Core Engineering
status: Design evidence for #228
last-updated: 2026-06-28
source-issue: 228
related-issues:
  - 214
  - 227
  - 229
  - 252
---

# v2 catalog discovery storyboard

This storyboard is pre-implementation design evidence for #228. It supersedes
older #228 issue-comment transcripts that used `recipe list` as the normal
user discovery command.

The accepted user-facing direction is:

```text
Users add catalogs.
Catalogs provide support for apps/tools.
Users manage apps/tools.
Recipe is an advanced authoring/debugging noun, not the normal discovery noun.
```

## Boundaries

#228 should implement the built-in/local catalog discovery foundation and the
normal discovery/explanation surface that depends on it:

- `list` — show supported apps/tools and whether each is managed;
- `search <query>` — find supported apps/tools across enabled catalogs;
- `explain <app>` — explain app support, provenance, candidates, and write
  authority without requiring the user to understand recipe files;
- `catalog ...` — manage built-in/local catalog sources;
- advanced `recipe ...` commands may remain for authors/debugging, but they are
  not the happy-path discovery surface.

Broader command cleanup belongs to #252 when it is outside #228's catalog
foundation scope, especially:

- final compatibility/deprecation behavior for existing `add`, `recipe list`,
  `recipe explain`, and `app create/validate/test` commands;
- `manage`/`unmanage` command implementation if it is not already needed for the
  #228 catalog/unavailable-source scenarios;
- full normal help reordering beyond the #228 discovery and catalog lifecycle
  changes.

Remote GitHub catalogs are not implemented by #228. The future shape
`catalog add shpoont/custom-recipes` is reserved for #229.

## UX principles

- The normal path is app-first/action-first, not recipe-first.
- `list`, `search`, and `explain` are safe read-only discovery commands.
- Catalogs are an advanced expansion mechanism, similar to Homebrew taps.
- Built-in support is always available, deterministic/offline, and not removable.
- Local catalogs are local-only; adding one does not fetch from the network.
- Adding/disabling/removing a catalog never deletes live app settings or stored
  settings.
- Local recipes are discoverable after machine validation, but they do not
  silently override built-in support.
- Before any operation that can write live settings, the effective source/origin
  and write authority must be visible.
- The word `recipe` can appear in advanced/debug details, JSON, validation
  errors, and authoring docs, but normal users should be able to succeed without
  learning it.

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
dotfiles-manager catalog add <local-path> --name <name>
dotfiles-manager catalog disable <name>
dotfiles-manager catalog enable <name>
dotfiles-manager catalog remove <name>

dotfiles-manager recipe ...   # advanced authoring/debugging only
```

`save` and `apply` remain directional sync aliases:

- `save` = live settings to settings storage folder;
- `apply` = settings storage folder to live settings.

`manage` means enroll an app/tool so it participates in status/diff/sync by
default. `unmanage` means stop tracking it by default. Neither command should
silently overwrite live settings, uninstall apps, delete live settings, or delete
stored settings. #252 owns full command cleanup for those verbs unless #228
needs a minimal form to explain unavailable local-catalog selections.

## Transcript 1: first run/offline built-in discovery

Command:

```bash
dotfiles-manager list
```

Expected output:

```text
Supported apps

  APP           SOURCE     STATE
  custom.files  built-in   not managed
  git           built-in   not managed
  nvim          built-in   not managed
  ssh           built-in   not managed
  starship      built-in   not managed
  tmux          built-in   not managed
  zsh           built-in   not managed

Catalogs:
  built-in  enabled  ships with dotfiles-manager

No live settings were read or changed.
No stored settings were changed.
```

User takeaway: the tool has built-in app support and works offline before any
catalog setup.

## Transcript 2: search supported apps

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
```

User takeaway: search is app/tool-oriented and does not require recipe terms.

## Transcript 3: explain built-in support

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
```

Advanced/verbose output may include `recipe://bundled/git`, catalog IDs,
digests, and source IDs. Default output keeps those details secondary.

## Transcript 4: list catalogs before local catalogs exist

Command:

```bash
dotfiles-manager catalog list
```

Expected output:

```text
Catalogs

  built-in  Built in  enabled
    Source: ships with dotfiles-manager
    Updates: with dotfiles-manager releases
    Network: not used
    Removable: no

Local catalogs: none
Remote catalogs: not supported yet

No live settings were read or changed.
```

User takeaway: catalogs are understandable, but not required for the built-in
happy path.

## Transcript 5: add a valid local catalog

Precondition: `~/dotfiles-manager-recipes` is a local folder containing recipe
files for `example-tool` and `git`.

Command:

```bash
dotfiles-manager catalog add ~/dotfiles-manager-recipes --name personal
```

Expected output:

```text
Added local catalog: personal

Source:
  ~/dotfiles-manager-recipes

Validated support:
  example-tool  local support
  git           local candidate; built-in support remains the default

Network: not used
No live settings were read or changed.
No stored settings were changed.
```

User takeaway: adding a local catalog is safe, local-only, and validated before
it becomes active.

## Transcript 6: reject invalid local catalog

Command:

```bash
dotfiles-manager catalog add ~/broken-recipes --name broken
```

Expected output:

```text
Catalog not added: broken

Reason:
  1 support definition failed validation.

Invalid support:
  broken-tool
    - unknown field "dangerousCommand"

No live settings were read or changed.
No stored settings were changed.
```

User takeaway: invalid catalog content fails closed before it affects discovery
or writes.

## Transcript 7: list with built-in and local support

Command:

```bash
dotfiles-manager list
```

Expected output:

```text
Supported apps

  APP           SOURCE              STATE
  custom.files  built-in            not managed
  example-tool  personal            not managed
  git           built-in            not managed
                also in personal; built-in remains default
  nvim          built-in            not managed
  ssh           built-in            not managed
  starship      built-in            not managed
  tmux          built-in            not managed
  zsh           built-in            not managed

Use `dotfiles-manager explain <app>` to see support details and candidates.
No live settings were read or changed.
```

User takeaway: local support appears as app support. Collisions are visible and
safe.

## Transcript 8: explain built-in/local collision

Command:

```bash
dotfiles-manager explain git
```

Expected output:

```text
Git is supported by multiple sources.

Default source:
  built-in support from dotfiles-manager

Other available source:
  local catalog: personal
  Status: candidate only

Why built-in is used:
  Built-in support remains the default unless you explicitly choose another
  source. Local support cannot silently replace built-in support.

Can manage from the default source:
  git:user.email  Git user email
  git:user.name   Git user name

No live values were printed.
No live settings were changed.
```

Verbose/debug output may include the effective recipe origins:

```text
Default: recipe://bundled/git
Candidate: recipe://local/git
```

## Transcript 9: explain local-only support

Command:

```bash
dotfiles-manager explain example-tool
```

Expected output:

```text
Example Tool is supported by a local catalog.

App ID: example-tool
Source: local catalog personal
State: not managed

Can manage:
  example-tool:config  Config file
    Live location: $HOME/.config/example-tool/config.yaml

Before live settings can be changed:
  dotfiles-manager will show this source and the paths it wants to manage.
  Local support requires write approval before it can change live settings.

No live values were printed.
No live settings were changed.
```

User takeaway: the local source is visible in plain language before any write.

## Transcript 10: before-write origin summary for local support

Command:

```bash
dotfiles-manager sync --dry-run example-tool
```

Expected output when local write authority is not yet granted:

```text
Preview sync for example-tool

Source:
  local catalog personal

This support wants to manage:
  example-tool:config
    Live location: $HOME/.config/example-tool/config.yaml
    Stored settings: desired/user/<user-id>/targets/example-tool/...

Result:
  Blocked before write.

Reason:
  Local support requires write approval before dotfiles-manager can change live
  settings for this app.

Next step:
  Review and approve this local support before allowing writes.
  If this version does not include an approval command yet, this app remains
  blocked for live writes.

No live settings were changed.
No stored settings were changed.
```

#228 should show this blocked state and preserve the source/provenance details.
The concrete approval/review command is outside #228 unless the active issue is
explicitly recontracted to include it.

If the exact local write grant already exists, the command may proceed to the
normal dry-run sync preview, but it must still show the local source before any
confirmed write.

## Transcript 11: disable a local catalog

Command:

```bash
dotfiles-manager catalog disable personal
```

Expected output:

```text
Disabled local catalog: personal

No longer available from this catalog:
  example-tool
  git local candidate

Nothing was deleted.
Live app settings were not changed.
Stored settings were not changed.

If a managed app depends on this catalog, it will show as source unavailable
until you enable the catalog, add another source, or stop managing that app.
```

Command:

```bash
dotfiles-manager list
```

Expected output excerpt:

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

Disabled local catalogs:
  personal  2 hidden apps/candidates
```

## Transcript 12: unavailable managed app after disabling/removing source

Precondition: `example-tool` was managed before the `personal` catalog was
disabled or removed.

Command:

```bash
dotfiles-manager status example-tool
```

Expected output:

```text
example-tool: blocked

Reason:
  This app is managed with support from local catalog "personal", but that
  catalog is disabled or removed.

No live app settings were read or changed.
Stored settings were not changed.
Stored settings still exist, if they existed before.

To continue:
  Enable the catalog:
    dotfiles-manager catalog enable personal

  Or add another catalog that supports example-tool.

  Or remove this app from the managed set.
```

#228 should not invent a separate stop-managing command. The normal
`unmanage <app>` command and exact compatibility behavior are tracked by #252.

User takeaway: disabling/removing a catalog is reversible and data-preserving.

## Transcript 13: enable a local catalog

Command:

```bash
dotfiles-manager catalog enable personal
```

Expected output:

```text
Enabled local catalog: personal

Validated support:
  example-tool  local support
  git           local candidate; built-in support remains the default

No live settings were read or changed.
No stored settings were changed.
```

## Transcript 14: remove a local catalog

Command:

```bash
dotfiles-manager catalog remove personal
```

Expected output:

```text
Removed local catalog: personal

Forgotten by dotfiles-manager:
  ~/dotfiles-manager-recipes

Nothing was deleted from that folder.
Live app settings were not changed.
Stored settings were not changed.

Apps that depended on this catalog are now source unavailable until you re-add
this catalog, choose another source, or stop managing those apps.
```

Important distinction:

- `disable` keeps the source record and can be re-enabled by name;
- `remove` forgets the source record but does not delete the catalog folder or
  settings data.

## Transcript 15: remote catalog syntax reserved for #229

Command:

```bash
dotfiles-manager catalog add shpoont/custom-recipes
```

Expected #228 output:

```text
Catalog not added: shpoont/custom-recipes

Reason:
  Remote GitHub catalogs are not supported in this version of dotfiles-manager.

For now, use a local catalog folder:
  dotfiles-manager catalog add ./custom-recipes --name personal

Remote catalog trust, updates, and write gates are planned separately.
No live settings were read or changed.
No stored settings were changed.
```

User takeaway: the future Homebrew-like syntax is reserved, but #228 remains
local/offline.

## JSON and advanced-output notes

Text output should be app/tool-oriented. JSON and verbose output may expose
implementation fields needed by tests and advanced users, including:

- `sourceKind`;
- `sourceId`;
- `catalogId`;
- `sourceDisplayName`;
- `originUri`;
- `recipeRef`;
- `recipeDigest`;
- `reviewStatus`;
- `writeAuthority`;
- `selectedBy`.

Unknown source kinds, enum values, capabilities, schemas, path declarations, or
unsafe write surfaces must fail closed.

## Acceptance notes for #228 implementation

A #228 implementation PR should use this storyboard as design evidence and then
provide real-result verification with:

- targeted catalog/discovery tests and `go test ./...` where practical;
- temp-home/fixture command output for built-in first-run/offline discovery;
- local catalog add/list/disable/enable/remove evidence;
- built-in/local collision evidence;
- unavailable-source status evidence;
- proof that remote catalog/network behavior was not introduced.
