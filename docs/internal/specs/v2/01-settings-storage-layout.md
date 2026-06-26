---
owner: Product + Core Engineering
document-type: v2-draft-spec
status: Draft reset layout source; not a runtime behavior contract
last-updated: 2026-06-26
canonical-source: docs/internal/specs/v2/01-settings-storage-layout.md
source-issue: 210
supersedes: docs/internal/specs/v2/01-repository-layout.md
authority: Reset-v2 storage layout direction for planning and follow-up specs; implementation authority requires later promoted behavior/schema specs.
---

# v2 settings storage layout

## Purpose

This draft replaces the older repository-layout model for reset-v2 planning. It
defines the conceptual layout of a **settings folder**: the folder where selected
stored settings live so they can be compared with and synced to live app
settings.

It is a planning/source-of-truth document for layout vocabulary. It does not
finalize runtime schemas or implement behavior.

## Core principles

1. The public noun is **settings folder**.
2. Git is optional. A settings folder may be a Git repository, but Git is not
   required and not the product model.
3. The settings folder stores actual managed values and payloads. It may contain
   sensitive personal preferences and must be documented that way.
4. Local runtime state, ledgers, caches, temp files, and any owner-approved
   internal safety evidence are not stored in the settings folder by default.
5. v1 migration output and backup/restore workflows are not active v2 layout
   requirements.
6. Internal `desired://` URIs may identify stored settings, but normal commands
   use public refs such as `git:user.email`.

## Settings folder contents

A settings folder may contain:

```text
<settings-folder>/
  dotfiles-manager.v2.yaml
  profiles/
    stacks/<stack-id>.yaml
    layers/<layer-id>.yaml
  stored/
    shared/-/targets/<target-id>/manifest.yaml
    shared/-/targets/<target-id>/settings.yaml
    shared/-/targets/<target-id>/artifacts/...
    user/<user-id>/targets/<target-id>/manifest.yaml
    user/<user-id>/targets/<target-id>/settings.yaml
    user/<user-id>/targets/<target-id>/artifacts/...
    machine/<machine-id>/targets/<target-id>/manifest.yaml
    machine/<machine-id>/targets/<target-id>/settings.yaml
    machine/<machine-id>/targets/<target-id>/artifacts/...
    machine-user/<machine-id>/<user-id>/targets/<target-id>/manifest.yaml
    machine-user/<machine-id>/<user-id>/targets/<target-id>/settings.yaml
    machine-user/<machine-id>/<user-id>/targets/<target-id>/artifacts/...
  recipes/
    local/<target-id>/recipe.yaml
```

The exact path names are unresolved draft examples. Later layout/schema and
implementation issues own the decision to keep `desired/` for compatibility,
adopt `stored/` for clearer vocabulary, or choose another accepted path. The
required product rule is that user-facing copy says stored settings in a settings
folder, not repository-owned desired state.

## Stored settings

For each selected setting, the settings folder stores enough material to compare
and sync with live settings:

- `manifest.yaml` binds public setting refs to stored artifacts and metadata;
- `settings.yaml` stores manager-owned scalar/object values when appropriate;
- `artifacts/` stores file, file-tree, native-export, opaque, or driver-owned
  payload material.

Stored settings may contain actual values. They may contain personal or sensitive
preferences. Normal output must avoid printing raw values unless a recipe marks
them safe.

## Scope directories

Stored settings are grouped by portability boundary:

```text
stored/shared/-/targets/<target-id>/...
stored/user/<user-id>/targets/<target-id>/...
stored/machine/<machine-id>/targets/<target-id>/...
stored/machine-user/<machine-id>/<user-id>/targets/<target-id>/...
```

Normal output should prefer labels:

| Directory scope | Normal label |
| --- | --- |
| `shared` | for everyone |
| `user` | for me |
| `machine` | for this computer |
| `machine-user` | for me on this computer |

## Local runtime state

Local runtime state should live outside the settings folder by default. It may
include:

- local machine/person identity records;
- ledgers and previous sync baselines;
- run records;
- caches;
- temporary render/apply workspaces;
- internal safety evidence if #212 accepts it.

Default roots should be platform-native state/cache/temp directories keyed by a
stable settings-folder identity. Local runtime state must not be required to
travel with the settings folder.

## Recipes and catalogs

The settings folder may contain user-local recipes under `recipes/local/` when
the user intentionally authors or vendors them. Bundled recipes come from the
manager. Remote catalog runtime support is future work and must follow the
#227 trust and origin model in `17-catalog-trust-origin-model.md` before remote
recipes can write live settings.

## Git-optional behavior

The manager may recommend Git for versioning and sharing the settings folder, but
must still support a plain local folder.

Examples should say:

```text
settings folder: ~/Settings
optional: commit this folder with Git if you want history/sharing
```

They should not say:

```text
repo: required config repository
```

## Internal URI policy

Internal URIs may use a storage-oriented scheme such as `desired://` or a later
accepted replacement. They are internal identifiers for verbose/JSON/debug or
recipe-authoring contexts.

Normal users should use public refs:

```text
git:user.email
starship:config
```

## Superseded prototype assumptions

The reset layout does not carry forward these older active requirements:

- mandatory repository root as the product noun;
- repository-owned migration output;
- first-class backup/restore layout;
- v1 `syncs:` legacy adapter as a v2 usability requirement;
- public examples that require Git before local settings can be managed.

## Follow-up decisions

- #211/#221-#225 own status/diff/sync command behavior and JSON/text contracts.
- #212 owns removal or quarantine of backup/restore surfaces and tests.
- #213/#226 own legacy v1 public-surface policy.
- #214/#227-#229 own catalog/tap source, trust, and write authority.
- #215/#230-#231 own new-computer bootstrap behavior.
- Schema filenames and exact path compatibility are implementation decisions for
  later promoted specs.
