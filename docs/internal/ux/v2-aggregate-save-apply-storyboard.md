# v2 aggregate save/apply confirmation storyboard

Status: issue #179 pre-implementation UX artifact.
Last updated: 2026-06-14.
Scope: visual CLI UX only; no command behavior, renderer, JSON schema, v1 output, or native export/import changes.
Related issues: #165, #166, #167, #168, #171, #177, #179.

## Purpose

This storyboard defines what aggregate selected `save` and `apply` confirmation
flows should feel like when a user has selected more than one app or setting. It
is a pre-implementation UX artifact for future renderer and snapshot-test work.

The target user should be able to read the transcripts and answer:

- Which apps and settings were checked?
- Is this a dry run or a confirmed write?
- What does `save` read, and what does it write?
- What does `apply` read, and what does it write?
- Which live files, desired-state repo files, or manager-owned state files would
  be written or were written?
- Which items are safe to confirm, skipped, blocked, unsupported, or failed?
- Did a partial confirmed write change only some items?
- Was a backup or recovery handle created before live writes?
- What command is safe to run next?

## Non-goals

- No CLI behavior implementation.
- No renderer implementation.
- No JSON schema, enum, field-name, object-shape, or scripting contract changes.
- No v1 output changes.
- No native export/import support and no change to #113.
- No lifecycle automation, app restart/reload, plugin installation, or
  package-manager action.
- No fake subset command syntax when the current CLI grammar cannot express the
  subset.
- No raw managed values, unrelated config values, credentials, tokens, account
  or session data, private keys, secrets, symlink targets, or backup payload
  bytes in examples.

The storyboard must not describe `save` or `apply` as native export/import,
migration, sync, package transfer, or app-level export/import. Issue #113
remains blocked and only appears here as out of scope.

## Storyboard setup

The transcript below assumes a user has selected several settings across
multiple apps. The scenario intentionally includes safe-to-confirm items,
already-current items, blocked items, unsupported items, failed items, and
skipped/not-applicable items.

Demo state:

| App | Item | Save view | Apply view | User-facing reason |
| --- | --- | --- | --- | --- |
| Git | `git:user.email` | would save | would apply | Live value differs from saved desired state. The user must choose direction. |
| Git | `git:user.name` | already saved | up to date | Live value matches saved desired state. |
| Git | `git:credential.helper` | unsupported | unsupported | Credential helpers are explicitly not managed. |
| Starship | `starship:add_newline` | would save | skipped | Live value exists, but no desired state has been saved yet. |
| Starship | `starship:scan_timeout` | failed | failed | Save cannot read live config; apply cannot read saved desired state. |
| Zsh | `zsh:zshrc` | blocked | blocked | Live path is an unsafe symlink. |

The examples use canonical user ID `leon` consistently in commands and
`desired/user/leon/...` paths. They do not print actual values. They do not use
native export/import, app-control, or #113-blocked native behavior as the
scenario.

The same aggregate setup is used for `save` and `apply`, but failures are
command-appropriate:

- `save` failures come from live-config read or desired-repo write boundaries;
- `apply` failures come from desired-state read or live config/app-location
  write boundaries;
- if a future command needs comparison metadata, that metadata must not be
  presented as the primary source of the write.

## Output-tier boundaries for aggregate writes

### Default text

Default text is the human-first tier. It should show:

- one-line scope summary;
- counts before item details;
- dry-run/no-write or confirmed-write status;
- per-app sections;
- one plain-language line per setting/item;
- explicit read and write boundaries;
- safe-to-confirm items and unsafe/skipped items;
- backup/recovery information for confirmed live writes when applicable;
- partial-success wording that does not imply skipped or blocked items changed;
- one safe next command when one command exists;
- safe alternatives when no single command can express the useful subset.

Default text must not require understanding `resource`, `driver`, `selector`,
`desired://`, `state://`, raw planner states, raw actions, raw ledger refs, or
backup artifact refs.

### Verbose text

Verbose text appends diagnostics after the same human summary. It may include
technical refs, resource IDs, driver IDs, selectors, locations, raw state/action
names, desired/state URIs, backup refs, ledger refs, and diagnostic codes. It
must keep the same redaction policy as default text.

### JSON

`--json` remains the scripting contract. This storyboard does not redesign JSON.
`--json --verbose` must still write one JSON document to stdout with no human
prose appended. Any future JSON shape changes require a separate JSON contract
issue.

## Expected default `save --dry-run` transcript

Command:

```bash
dotfiles-manager save --dry-run --user-id leon
```

Expected default output:

```text
Save preview checked 6 settings across 3 apps.

Summary:
  Would save to desired state: 2
  Already saved: 1
  Blocked: 1
  Unsupported: 1
  Failed: 1
  Skipped / not applicable: 0

Write boundary:
  This is a dry run. No files were changed.
  Live config files may be read.
  Desired-state repo files would be written only after confirmation.
  Live app/config files would not be changed by save.

Git
  - Git user email: would save current live value to desired state
    Live file read: $HOME/.gitconfig [user] email
    Desired state to write: desired/user/leon/targets/git/settings.yaml
    Value hidden for safety.

  - Git user name: already saved
    Live file read: $HOME/.gitconfig [user] name
    Desired state already matches the live value.

  - Git credential helper: unsupported
    Reason: credentials and credential helpers are not managed by the Git recipe.
    No credential value was read or printed.

Starship
  - Add newline: would save current live value to desired state
    Live file read: $HOME/.config/starship.toml key add_newline
    Desired state to write: desired/user/leon/targets/starship/settings.yaml
    Value hidden for safety.

  - Scan timeout: failed
    Reason: live Starship config could not be read safely.
    No desired-state repo file would be written for this item.
    Re-run with --verbose for diagnostic details.

Zsh
  - .zshrc: blocked
    Reason: the live path is an unsafe symlink. dotfiles-manager did not read or
    save the symlink target.

Safe to confirm:
  dotfiles-manager save --yes --user-id leon git:user.email
  dotfiles-manager save --yes --user-id leon starship:add_newline

Next:
  No single command can safely save every selected item above.
  Confirm only the safe items, or resolve the blocked and failed items first.
```

User takeaway: `save --dry-run` previews repo desired-state writes. It does not
claim live writes, backup creation, or executed success. It names the safe
narrow commands only because those public setting refs are supported.

## Expected default `save --yes` transcript

Command:

```bash
dotfiles-manager save --yes --user-id leon
```

Expected default output:

```text
Save completed with partial success.

Summary:
  Saved to desired state: 2
  Already saved: 1
  Blocked: 1
  Unsupported: 1
  Failed: 1
  Skipped / not applicable: 0

Write boundary:
  Desired-state repo files were written.
  Live app/config files were not changed.
  No live-file backup was created because save did not write live files.
  Blocked, unsupported, and failed items were not saved.

Desired-state repo files written:
  - desired/user/leon/targets/git/settings.yaml
  - desired/user/leon/targets/starship/settings.yaml

Git
  - Git user email: saved current live value to desired state
    Live file read: $HOME/.gitconfig [user] email
    Desired state written: desired/user/leon/targets/git/settings.yaml
    Value hidden for safety.

  - Git user name: already saved
    Live file read: $HOME/.gitconfig [user] name
    Desired state already matched the live value.

  - Git credential helper: unsupported
    Reason: credentials and credential helpers are not managed by the Git recipe.
    No credential value was read or printed.

Starship
  - Add newline: saved current live value to desired state
    Live file read: $HOME/.config/starship.toml key add_newline
    Desired state written: desired/user/leon/targets/starship/settings.yaml
    Value hidden for safety.

  - Scan timeout: failed
    Reason: live Starship config could not be read safely.
    No desired-state repo file was written for this item.

Zsh
  - .zshrc: blocked
    Reason: the live path is an unsafe symlink. dotfiles-manager did not read or
    save the symlink target.

Next:
  Preview what would be applied from the saved desired state:
  dotfiles-manager apply --dry-run --user-id leon

  Diagnose the blocked and failed items:
  dotfiles-manager save --dry-run --verbose --user-id leon
```

User takeaway: a confirmed broad save can partially succeed. It reports the
desired-state repo files that were written and says plainly that live app/config
files were not changed. It does not imply unsupported, blocked, or failed items
were saved.

### Narrow safe-confirm example

If the user only wants to confirm one safe item from the dry run, the output can
use a supported public setting ref:

```bash
dotfiles-manager save --yes --user-id leon git:user.email
```

```text
Saved desired state for 1 setting.

Summary:
  Saved to desired state: 1
  Already saved: 0
  Blocked: 0
  Unsupported: 0
  Failed: 0
  Skipped / not applicable: 0

Write boundary:
  Desired-state repo files were written.
  Live app/config files were not changed.
  No live-file backup was created because save did not write live files.

Git
  - Git user email: saved current live value to desired state
    Live file read: $HOME/.gitconfig [user] email
    Desired state written: desired/user/leon/targets/git/settings.yaml
    Value hidden for safety.

Next:
  Preview what would be applied back to the live Git config:
  dotfiles-manager apply --dry-run --user-id leon git:user.email
```

## Expected default `apply --dry-run` transcript

Command:

```bash
dotfiles-manager apply --dry-run --user-id leon
```

Expected default output:

```text
Apply preview checked 6 settings across 3 apps.

Summary:
  Would apply to live config: 1
  Already up to date: 1
  Blocked: 1
  Unsupported: 1
  Failed: 1
  Skipped / not applicable: 1

Write boundary:
  This is a dry run. No files were changed.
  Desired state may be read from the repo.
  Live app/config files would be written only after confirmation.
  A backup would be created before confirmed live writes where required.
  Desired-state repo files would not be changed by apply.

Git
  - Git user email: would apply saved desired value to live config
    Desired state read: desired/user/leon/targets/git/settings.yaml
    Live file to write: $HOME/.gitconfig [user] email
    Current and desired values are hidden.
    Backup before write: would back up $HOME/.gitconfig.

  - Git user name: already up to date
    Desired state read: desired/user/leon/targets/git/settings.yaml
    Live file checked: $HOME/.gitconfig [user] name

  - Git credential helper: unsupported
    Reason: credentials and credential helpers are not managed by the Git recipe.
    No credential value was read or printed.

Starship
  - Add newline: skipped / not applicable
    Reason: selected, but no saved desired state exists yet.
    Preview saving the live value first:
    dotfiles-manager save --dry-run --user-id leon starship:add_newline

  - Scan timeout: failed
    Reason: saved desired state could not be read.
    No live Starship file would be changed.
    Re-run with --verbose for diagnostic details.

Zsh
  - .zshrc: blocked
    Reason: the live path is an unsafe symlink. No write would be attempted.

Safe to confirm:
  dotfiles-manager apply --yes --user-id leon git:user.email

Next:
  Resolve blocked, unsupported, failed, and skipped items before running a broad
  apply. No single safe broad command is available for every item above.
```

User takeaway: `apply --dry-run` previews live writes. It does not claim desired
repo writes, backup creation, or executed success. Backup is described as a
future confirmed-write safety step, not as something already created by dry run.

## Expected default `apply --yes` transcript

Command:

```bash
dotfiles-manager apply --yes --user-id leon
```

Expected default output:

```text
Apply completed with partial success.

Summary:
  Applied to live config: 1
  Already up to date: 1
  Blocked: 1
  Unsupported: 1
  Failed: 1
  Skipped / not applicable: 1

Write boundary:
  Live app/config files were changed.
  Desired-state repo files were not changed.
  A backup was created before the live write.
  Blocked, unsupported, failed, and skipped items were not applied.

Live app/config files changed:
  - $HOME/.gitconfig [user] email

Backup created:
  - backup run <backup-run-id> includes $HOME/.gitconfig

Git
  - Git user email: applied saved desired value to live config
    Desired state read: desired/user/leon/targets/git/settings.yaml
    Live file changed: $HOME/.gitconfig [user] email
    Current and desired values are hidden.

  - Git user name: already up to date
    Desired state read: desired/user/leon/targets/git/settings.yaml
    Live file checked: $HOME/.gitconfig [user] name

  - Git credential helper: unsupported
    Reason: credentials and credential helpers are not managed by the Git recipe.
    No credential value was read or printed.

Starship
  - Add newline: skipped / not applicable
    Reason: selected, but no saved desired state exists yet.
    It was not applied.

  - Scan timeout: failed
    Reason: saved desired state could not be read.
    No live Starship file was changed.

Zsh
  - .zshrc: blocked
    Reason: the live path is an unsafe symlink. No write was attempted.

Recovery:
  Preview restoring the backup before running restore:
  dotfiles-manager restore --dry-run <backup-run-id>

Next:
  Check aggregate status:
  dotfiles-manager status --user-id leon

  Resolve blocked, unsupported, failed, and skipped items before running another
  broad apply. No single safe command resolved every selected item above.
```

User takeaway: a confirmed broad apply can partially succeed. It reports the
live files changed, backup handle, and desired-state repo non-change. It does
not imply blocked, unsupported, failed, or skipped items were applied.

The restore preview command appears here because restore preview is an
already-supported flow for backup runs. If a future live write has a backup or
recovery handle but no supported restore-preview command, the transcript must
say that plainly instead of inventing a command.

### Narrow safe-confirm example

If the user only wants to confirm one safe item from the dry run, the output can
use a supported public setting ref:

```bash
dotfiles-manager apply --yes --user-id leon git:user.email
```

```text
Applied desired state to 1 live setting.

Summary:
  Applied to live config: 1
  Already up to date: 0
  Blocked: 0
  Unsupported: 0
  Failed: 0
  Skipped / not applicable: 0

Write boundary:
  Live app/config files were changed.
  Desired-state repo files were not changed.
  A backup was created before the live write.

Git
  - Git user email: applied saved desired value to live config
    Desired state read: desired/user/leon/targets/git/settings.yaml
    Live file changed: $HOME/.gitconfig [user] email
    Backup: backup run <backup-run-id> includes $HOME/.gitconfig
    Current and desired values are hidden.

Recovery:
  Preview restoring the backup before running restore:
  dotfiles-manager restore --dry-run <backup-run-id>

Next:
  Check status:
  dotfiles-manager status --user-id leon git:user.email
```

## Expected verbose addition

Command:

```bash
dotfiles-manager apply --dry-run --verbose --user-id leon
```

Expected verbose structure:

```text
Apply preview checked 6 settings across 3 apps.

Summary:
  Would apply to live config: 1
  Already up to date: 1
  Blocked: 1
  Unsupported: 1
  Failed: 1
  Skipped / not applicable: 1

Write boundary:
  This is a dry run. No files were changed.
  Desired state may be read from the repo.
  Live app/config files would be written only after confirmation.
  A backup would be created before confirmed live writes where required.
  Desired-state repo files would not be changed by apply.

... same human summary as default ...

Technical details:
  profileStack: default
  activeLayers: global

  git:user.email
    state: would-apply
    action: apply-live-write-preview
    resource: user-email
    driver: ini-file
    location: home
    selector: [user] email
    desired: desired://user/leon/targets/git/settings.yaml#values.user.email
    live: state://selected-live/git/user.email
    backup: would-create-before-live-write

  git:credential.helper
    state: unsupported
    diagnostic: selectedpreview.resource.unknown
    reason: unsupported setting; credentials are excluded by recipe policy

  starship:add_newline
    state: skipped-not-applicable
    diagnostic: selectedvalue.desired.missing
    desired: desired://user/leon/targets/starship/settings.yaml#values.add_newline

  starship:scan_timeout
    state: failed
    diagnostic: selectedvalue.starship.desiredReadFailed
    desired: desired://user/leon/targets/starship/settings.yaml#values.scan_timeout
    raw desired value: hidden

  zsh:zshrc
    state: blocked-safety
    diagnostic: selectedvalue.files.unsafeSymlink
    live: $HOME/.zshrc
    symlink target: hidden

Verbose output kept values and secret-bearing payloads hidden.
```

Verbose output adds technical details after the same user summary. It does not
replace the summary, does not change behavior, and does not print raw values.

## JSON boundary example

Command:

```bash
dotfiles-manager apply --dry-run --json --user-id leon
```

Boundary placeholder output:

```json
null
```

The `null` document above is not a literal CLI payload and does not define the
real schema. It is a JSON-only placeholder used by this UX storyboard to show
that the output block contains exactly one JSON document with no prose, comments,
banners, markdown explanation, or schema description inside the output. Exact
JSON output remains owned by the promoted JSON contract and future implementation
tests.

## Rules extracted from this storyboard

1. Aggregate save/apply output starts with counts before item details.
2. `save` and `apply` must use different write-boundary copy.
3. Dry-run transcripts explicitly say no files changed and avoid
   executed-success wording.
4. Confirmed transcripts say what was actually written and what was skipped.
5. Partial success must never imply blocked, unsupported, failed, or skipped
   items changed.
6. Backups/recovery handles are tied to confirmed live writes, not repo-only
   saves, unless a future implementation explicitly changes that behavior.
7. Restore preview commands appear only where a supported restore-preview flow
   exists.
8. Counts and per-app grouping stay consistent with the aggregate `status`/`diff`
   storyboard.
9. Redacted values are described by existence and relationship, not printed.
10. Default output stays user-facing even for failures; parser, artifact, and
    diagnostic internals belong in verbose/JSON.
11. Verbose output appends technical details after the human summary and keeps
    redaction.
12. JSON output remains JSON-only and schema-owned elsewhere.
13. Next commands must be real supported public commands. If no safe single
    command exists, the output says so instead of inventing syntax.
14. Native export/import, lifecycle automation, plugin installation, and
    package-manager actions must not be implied by aggregate write UX examples.
