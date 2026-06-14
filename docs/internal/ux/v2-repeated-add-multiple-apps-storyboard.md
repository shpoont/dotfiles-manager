# v2 repeated add flow for multiple supported apps storyboard

Status: issue #183 pre-implementation UX artifact.
Last updated: 2026-06-14.
Scope: visual CLI UX only; no command behavior, renderer, JSON schema, shell exit-code, v1 output, or native export/import changes.
Related issues: #165, #167, #168, #171, #177, #179, #181, #183.
Pro validation evidence: recorded on #183; no ChatGPT conversation URL is checked into this document.

## Purpose

This storyboard covers the user moment after the safe one-setting quickstart:
“I want dotfiles-manager to manage several supported apps/settings, but I do
not want to learn internal resource groups or driver names first.”

The target user should be able to read the transcript and answer:

- Which apps/settings did I select?
- Did selection save any values into my repo yet?
- Did selection change my live app configs?
- Which data will not be managed?
- What command is safe to run next?
- Is any shown syntax unsupported or future-only?

## Current command contract

The current `add` command accepts one target argument:

```text
dotfiles-manager add <target>
```

It supports repeated or comma-separated `--setting` values for that one target.
The happy path in this storyboard therefore uses repeated `add` commands, one
per supported target.

This is **not** current supported syntax:

```bash
# future / not currently supported
dotfiles-manager add git starship zsh
```

A single-command multi-target add flow would need a separate command-contract
issue before documentation or implementation can show it as supported behavior.

## Non-goals

- No CLI implementation or behavior changes.
- No command-contract change for multi-target `add`.
- No renderer implementation in this issue.
- No JSON schema, field, enum, or scripting contract changes.
- No shell exit-code contract changes.
- No v1 output changes.
- No native export/import support and no change to #113.
- No lifecycle automation, app quit/reopen/reload, plugin installation,
  package-manager action, shell integration control, terminal control, or app
  server control.
- No fake selector syntax, fake app-subset syntax, or fake multi-target `add`
  syntax as a current feature.
- No raw managed values, credentials, keys, tokens, credential helpers, history,
  cache/session data, plugin state, or backup payload bytes in examples.

## Output-tier boundary

Default text output is human-first. It may show app names, public setting refs,
plain-language scope labels, and user-level repo paths such as
`desired/user/<user-id>/targets/git/settings.yaml` when that helps the user
understand where saved desired state will live.

Default text must not require understanding resource IDs, driver IDs, selectors,
location IDs, `desired://` URIs, `state://` URIs, raw planner states, or raw
profile-layer internals.

Verbose text may include technical refs and profile-layer details for debugging,
but it must keep the same redaction policy as default text.

JSON remains the scripting contract. This storyboard does not add JSON fields,
rename fields, change enum values, or define new script behavior.

The default-output examples below reuse the already documented UX concepts from
#167 and the aggregate storyboards. They are UX storyboard copy, not a new JSON
or shell contract.

## Storyboard setup

The transcript uses a temporary HOME and repo so the example never touches a
real Mac home directory. Demo values seed live config files only so the manager
can later preview saving them; command output must not print those raw values.

```bash
DFM=/path/to/dotfiles-manager
DFM_DEMO_ROOT=$(mktemp -d)
DFM_HOME="$DFM_DEMO_ROOT/home"
DFM_REPO="$DFM_DEMO_ROOT/repo"
mkdir -p "$DFM_HOME/.config" "$DFM_REPO"

cat > "$DFM_HOME/.gitconfig" <<'GITCONFIG'
[user]
  email = demo@example.test
  name = Demo User
[credential]
  helper = demo-secret-helper
GITCONFIG

cat > "$DFM_HOME/.config/starship.toml" <<'STARSHIP'
add_newline = true
command_timeout = 500
format = "demo prompt format that should not be printed"
STARSHIP

cd "$DFM_REPO"
```

Safety note: `$DFM_HOME` is a temporary demo home. Do not remove the
`HOME="$DFM_HOME"` prefix when copying commands unless you intend to inspect
or select settings from your real home directory.

## Expected default transcript

### 1. Initialize the v2 workspace

Command:

```bash
HOME="$DFM_HOME" "$DFM" init --yes --machine-id demo-mac --user-id demo-user
```

Expected default output:

```text
Initialized dotfiles-manager v2 workspace.

Repo files:
  Created dotfiles-manager.v2.yaml
  Created profiles/stacks/default.yaml
  Created profiles/layers/global.yaml

Local identity:
  Machine: demo-mac
  User: demo-user

These local identity files are used to keep this machine/user separate from shared repo state.

Summary: 3 repo files created, 2 local identity files created.

Next:
  Discover supported settings:
  dotfiles-manager --config dotfiles-manager.v2.yaml recipe discover
```

User takeaway: the repo and local identity exist. No app config has been saved
or applied yet.

### 2. Select two Git identity settings

Command:

```bash
HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
  add git --setting user.email --setting user.name --scope user --yes
```

Expected default output:

```text
Selected Git settings.

Selection:
  Selected: git:user.email — User email
    Scope: user — Me on all my machines
    Profile layer: global
  Selected: git:user.name — User name
    Scope: user — Me on all my machines
    Profile layer: global

No live app config was changed.

Next:
  Preview saving the current live value as desired state:
  dotfiles-manager --config dotfiles-manager.v2.yaml save --dry-run git:user.email

Summary: 2 selected settings for git.
```

User takeaway: Git user email and Git user name are selected for management.
This did not copy their current values into the repo and did not modify
`$HOME/.gitconfig`.

Git support boundary shown by surrounding docs and review:

- managed: non-credential user identity settings selected above;
- not managed: credentials, tokens, credential helpers, SSH/GPG signing keys,
  auth state, include/includeIf expansion, aliases, arbitrary Git sections, or
  repo-local `.git/config`.

### 3. Select two Starship prompt settings

Command:

```bash
HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
  add starship --setting add_newline --setting command_timeout --scope user --yes
```

Expected default output:

```text
Selected Starship settings.

Selection:
  Selected: starship:add_newline — Add newline
    Scope: user — Me on all my machines
    Profile layer: global
  Selected: starship:command_timeout — Command timeout
    Scope: user — Me on all my machines
    Profile layer: global

No live app config was changed.

Next:
  Preview saving the current live value as desired state:
  dotfiles-manager --config dotfiles-manager.v2.yaml save --dry-run starship:add_newline

Summary: 2 selected settings for starship.
```

User takeaway: Starship settings are selected independently from Git settings.
Selection still does not save current values as desired state and still does not
modify live app config.

Starship support boundary shown by surrounding docs and review:

- managed: selected prompt-wide TOML scalar options, such as `add_newline` and
  `command_timeout`;
- not managed: arbitrary modules, shell integration lifecycle, plugin or package
  installation, terminal state, unrelated TOML keys, or account/session data.

### 4. Review the selected multi-app surface

Command:

```bash
HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml list --user-id demo-user
```

Expected default output:

```text
Selected settings

Git
  git:user.email — User email
    Scope: user — Me on all my machines
    Subject: demo-user
    Desired state: not saved yet
  git:user.name — User name
    Scope: user — Me on all my machines
    Subject: demo-user
    Desired state: not saved yet

Starship
  starship:add_newline — Add newline
    Scope: user — Me on all my machines
    Subject: demo-user
    Desired state: not saved yet
  starship:command_timeout — Command timeout
    Scope: user — Me on all my machines
    Subject: demo-user
    Desired state: not saved yet

Next:
  Preview saving the current live value:
  dotfiles-manager --config dotfiles-manager.v2.yaml save --dry-run --user-id demo-user git:user.email

Summary: 4 selected settings, 0 unresolved.
```

User takeaway: four settings across two apps are selected. Desired state is not
saved yet for any of them. The user can preview saving one selected setting
without applying anything to live app configs.

### 5. Explain what has not happened yet

The repeated add flow should make this distinction explicit in nearby docs,
help text, or review copy:

```text
Selected for management:
  dotfiles-manager knows these settings should be tracked for this user/profile.

Saved as desired state:
  a later save command writes the current live value for the selected setting
  into the external repo folder. For example, it may write a user-level
  desired-state file under desired/user/demo-user/targets/git/settings.yaml.

Applied to live app config:
  a later apply command writes saved desired state back to live files, with the
  safety and backup behavior covered by the save/apply storyboards.
```

Selection changes manager profile state only. It does not save current live
values as desired state and does not edit live app files.

## Available but not selected in this storyboard

Zsh is a bundled target, but this storyboard does not select it in the main
happy path. If a later storyboard selects Zsh, it should use known supported
startup-file refs only and keep the support boundary visible.

Zsh exclusions include history files, completion caches, session state, plugin
state, generated runtime files, and `.zshenv`. Those exclusions are not “blocked
Zsh support”; they are data the manager intentionally does not manage.

## Safe next-command guidance

The safe next command after repeated add should be a command the current CLI can
run. Examples that are safe when those refs are selected:

```bash
dotfiles-manager --config dotfiles-manager.v2.yaml save --dry-run --user-id demo-user git:user.email
dotfiles-manager --config dotfiles-manager.v2.yaml save --dry-run --user-id demo-user starship:add_newline
dotfiles-manager --config dotfiles-manager.v2.yaml status --user-id demo-user
```

The output must not invent app-subset syntax. If a useful subset cannot be
expressed by current public refs or current aggregate commands, the output
should say so plainly and offer supported narrower commands.

## User-facing completion criteria

A first-time user should leave this flow understanding that:

- the manager can select more than one supported app/settings surface;
- repeated `add` commands are the current way to do that;
- selecting several settings is safe and does not modify live app config;
- selected settings can still show `Desired state: not saved yet`;
- the next safe step is a dry-run save or status/diff review;
- app exclusions are intentional safety boundaries, not hidden failures; and
- native export/import remains unavailable until #113 is resolved.
