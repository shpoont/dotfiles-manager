---
owner: Product + Core Engineering
status: Reset product concept accepted for v2 planning
last-updated: 2026-06-23
canonical-source: docs/internal/scope/product-concept-v2.md
source-issue: 210
---

# Product concept v2: local settings manager

## Status

This document is the reset product concept for v2 planning. It supersedes the
older prototype concept that centered on a mandatory config repository,
`save`/`apply` as the primary mental model, backup/restore as a product
feature, and v1 migration as part of the active roadmap.

This concept is a product/model source of truth. Runtime behavior is still
implemented through focused follow-up issues and promoted specs. No runtime
behavior changes merely because this document exists.

## Product promise

`dotfiles-manager` helps a person manage selected local settings for supported
apps and tools.

The user-facing promise is:

> Choose the apps and settings to manage. The manager stores selected settings in
> a settings folder, shows what differs between live settings and stored
> settings, and syncs all or selected settings in the safe direction.

The product should make it easy to:

1. choose supported apps/tools to manage;
2. inspect status and diffs between live settings and stored settings;
3. sync all apps, one app, or selected settings/resources;
4. handle conflicts and missing apps/settings safely;
5. use the settings folder with or without Git;
6. use bundled recipes first and later optional recipe catalogs/taps;
7. set up a new computer by installing apps first and then applying settings.

## Public mental model

Use **settings folder** in normal user-facing copy. Define it precisely as:

> The local folder where the manager stores selected settings so they can be
> compared with and synced to live app settings.

Use **settings storage folder** only where extra precision helps, such as layout,
path ownership, safety, and Git-optional explanations.

The normal model is:

```text
Supported app/tool
  -> live settings on this computer
  -> stored settings in the settings folder
  -> status / diff
  -> sync in the chosen direction
```

Git is optional. A settings folder may be a Git repository so a user can version
or share it, but Git is not the product noun and not required for local use.

## Core nouns

Normal users should only need these nouns:

| Public noun | Meaning |
| --- | --- |
| App/tool | A supported target whose selected settings can be managed. |
| Setting | A user-meaningful piece of app/tool state, such as `git:user.email`. |
| Live settings | The settings currently present in the app/tool on this computer. |
| Stored settings | The settings captured in the settings folder. |
| Settings folder | The folder that stores selected settings for comparison and sync. |
| Status | A summary of what changed and what can be done safely. |
| Diff | A readable or honest metadata-level explanation of differences. |
| Sync | The action that copies selected settings in a chosen safe direction. |
| Conflict | A case where both sides changed or the safe direction is ambiguous. |
| Catalog | A source of recipes. Bundled recipes come first; remote catalogs come later. |

Advanced/internal docs may also use:

- recipe;
- scope;
- profile layer / profile stack;
- named location;
- desired artifact;
- internal URI;
- resource;
- driver;
- ledger;
- trust policy.

These advanced terms must not be required for the ordinary happy path.

## Live settings, stored settings, and actual values

Live settings are read from app-native locations or app-native commands through a
reviewed recipe/driver.

Stored settings are actual managed values and payloads in the settings folder.
They may be scalar values, structured fragments, files, file trees, or portable
native export payloads. Because stored settings may contain personal or sensitive
preferences, the product must:

- warn users that the settings folder may contain sensitive managed bytes;
- avoid printing raw values in normal output unless the recipe declares them safe;
- block secrets and unsafe values unless a later policy explicitly permits them;
- recommend Git only as optional versioning/sharing, not as a safety substitute.

## Primary workflow

### First use on an existing computer

```bash
dotfiles-manager init --settings-folder ~/Settings
dotfiles-manager add git starship
dotfiles-manager status
dotfiles-manager diff
dotfiles-manager sync
```

Expected experience:

```text
Managing:
  git:user.email       for me              no stored value yet
  starship:config      for everyone        live and stored differ

Suggested next step:
  Review diffs, then sync selected settings into the settings folder.
```

### After local app settings changed

```bash
dotfiles-manager status
dotfiles-manager diff git:user.email
dotfiles-manager sync git:user.email
```

Expected experience:

```text
Changed here:
  git:user.email       for me

Safe action:
  Sync live setting -> settings folder
```

### New computer setup

App installation is outside `dotfiles-manager`. A common flow may use Homebrew
Bundle first, then settings sync:

```bash
brew bundle
dotfiles-manager init --settings-folder ~/Settings
dotfiles-manager status
dotfiles-manager sync --from settings-folder
```

Expected experience:

```text
Ready to apply:
  git:user.email       for me
  starship:config      for everyone

Missing app:
  nvim                 install it or skip it

Will not manage:
  credentials, sessions, caches, histories, generated state
```

Exact command names and flags are owned by #211 and its split issues. The concept
requires the workflow shape; it does not finalize CLI syntax.

## Sync-first command model

`status`, `diff`, and `sync` are the primary UX.

`save` and `apply` may remain as explicit directional sync aliases or advanced
commands only if #225 accepts that policy. They are not the primary product
mental model in this reset.

Directional meanings, if retained:

```text
app/live -> settings folder     previously called save
settings folder -> app/live     previously called apply
```

`sync` must not mean blind two-way merge. It must plan, explain direction, ask on
conflicts, and refuse when the safe direction is unknown.

## Scopes and profiles

The underlying model may keep the four precise scopes:

| Internal scope | Normal label |
| --- | --- |
| `shared` | for everyone |
| `user` | for me |
| `machine` | for this computer |
| `machine-user` | for me on this computer |

Normal output should prefer the labels. Advanced output may expose scope IDs.

A machine can have multiple profile layers, and a person can use more than one
profile layer on the same machine. Profiles are composition tools, not an account
system. The product must not make users learn profile algebra for the supported
happy path.

## Recipes, drivers, and catalogs

A recipe declares how a supported app/tool exposes selected settings. A reviewed
driver performs deterministic reads, diffs, previews, writes, and verification.
Unknown state defaults to do-not-manage.

Native import/export is a recipe-declared capability or reviewed driver behavior,
not arbitrary user scripting. Opaque exports must be honest: show hash/metadata
changes and limitations rather than fake semantic diffs.

Bundled recipes are the default path for common apps. Remote catalogs/taps are a
future expansion area and must wait for explicit origin, trust, update, disable,
and write-authority rules.

## Explicit non-goals for active v2

- Requiring Git for the settings folder.
- Managing app installation.
- Managing whole app state, accounts, sessions, cloud state, histories, caches,
  licenses, credentials, TCC/Keychain state, generated runtime files, or raw app
  containers by default.
- A first-class backup/restore workflow.
- v1 migration as part of the active v2 roadmap.
- Remote recipe writes before catalog trust and write authority are explicit.
- Arbitrary downloaded command execution through recipes.

## Source-of-truth relationship

- Vocabulary is owned by `docs/internal/specs/v2/00-vocabulary.md`.
- Settings-folder layout direction is owned by
  `docs/internal/specs/v2/01-settings-storage-layout.md`.
- The active/superseded spec map is owned by `docs/internal/specs/v2/README.md`.
- Status/diff/sync behavior is owned by #211 and split issues #221-#225.
- Backup/restore removal is owned by #212.
- Legacy v1 public-surface policy is owned by #213 and #226.
- Catalog/tap design is owned by #214 and split issues #227-#229.
- New-computer bootstrap is owned by #215 and split issues #230-#231.
- Production docs rewrite is owned by #216.

## Acceptance expectations for this concept

This concept is acceptable only if follow-up specs and issues no longer treat a
mandatory repository, backup/restore, v1 migration, or `save`/`apply` as the
primary v2 product model.
