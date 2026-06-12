# v2 safe quickstart CLI output storyboard

Status: issue #169 pre-implementation UX artifact.
Last updated: 2026-06-13.
Scope: visual CLI UX only; no command behavior or schema changes.
Related issues: #165, #166, #167, #168, #169.
Pro pre-validation: <https://chatgpt.com/c/6a281d17-56ec-83ed-88d8-fc0d345b3b9f?dfm_storyboard=1781306002>.

## Purpose

This document shows what the default v2 terminal output should feel like before
renderer implementation starts. It is the UX source of truth for the first safe
Git quickstart pass.

The target user should be able to read the terminal transcript and answer:

- What setting is managed?
- What live file is involved?
- Did this command change anything?
- What would change if the confirmed command is run?
- Where is desired state stored?
- Was the actual selected value printed?
- How can the user preview or undo an apply?
- What should the user run next?

## Non-goals

- No product behavior changes.
- No renderer implementation in this issue.
- No JSON contract changes.
- No command semantics changes.
- No new interactive wizard.
- No color, theme, pager, or TUI work.
- No broad redesign of every bundled target.
- No native export/import and no change to #113.
- No public `custom.files` adoption.
- No raw selected values, unrelated config values, secrets, private keys,
  tokens, credentials, or backup payload bytes in examples.

## Current pain examples

Only a few current examples are included here. The storyboard should focus on
what good output looks like, not on cataloging every old line.

### Current `init` is implementation-shaped

```text
init
profile stack: default [global]
repo files:
  root-config action=create path=dotfiles-manager.v2.yaml
identity files:
  machine action=create source=explicit path=state://identity/machine.yaml id=test-machine
summary status=changed planned=5 written=5 unchanged=0 blocked=0 failed=0
```

Problem: the user sees internal nouns before they know what was created or
whether real dotfiles were touched.

### Current `status` exposes internal state labels

```text
selected-value status
profile: global
  git:user.email state=missing-desired desired=missing current=present
    resource=user-email driver=ini-file selector=[user] email
    message: Setting is selected but no desired artifact exists.
summary status=changed changed=1 blocked=0 saved=0 applied=0
```

Problem: `state=missing-desired`, `resource`, `driver`, and `selector` are useful
for debugging, but they are not the user story.

### Current backup output hides the recovery path behind refs

```text
backups=state://backups/selected-value-YYYYMMDDTHHMMSSZ/git_user.email
```

Problem: the user needs to know whether a backup exists and how to preview
restore, not the internal backup URI first.

## Output tier boundaries

### Default text output

Default text is for humans. It should show:

- plain-language command result;
- whether anything changed;
- user-level live paths, such as `$HOME/.gitconfig`;
- user-level repo paths, such as
  `desired/user/<user-id>/targets/git/settings.yaml`;
- redaction statements when values exist but are hidden;
- backup run ids and restore preview commands when relevant;
- one safe next command.

Default text should not require understanding:

- `resource`, `driver`, or `selector`;
- `desired://` or `state://` URIs;
- raw planner states such as `missing-desired`;
- raw actions such as `would-promote`;
- `no-baseline` as an unexplained flag.

### Verbose text output

Verbose text is for debugging and power users. `--verbose` is a global output
flag for v2 commands that emit text; it is not a per-command behavior toggle and
must not change the plan, write safety, backup behavior, or redaction behavior.
It may include:

- setting refs such as `git:user.email`;
- profile stack/layer details;
- resource, driver, selector, and location identifiers;
- desired artifact URIs;
- state and backup URIs;
- planner states and actions;
- ledger/run refs;
- compatibility metadata.

Verbose text must keep the same redaction policy as default output. It may show
technical identifiers, but not raw managed values or secret-bearing payloads.

### JSON output

JSON is the stable scripting contract. This storyboard does not redesign JSON.
Issue #165 must preserve existing JSON schemas, field names, enum values,
refs, and redaction behavior unless a later issue explicitly changes the JSON
contract.

`--json --verbose` must remain script-safe: stdout is still only the existing
JSON schema, no verbose prose is appended, and `--verbose` does not add or remove
JSON fields. In JSON mode, technical fields already belong in the JSON contract;
verbose-only explanatory text belongs only to text mode.

### stdout and stderr

- Default text, verbose text, and JSON command results go to stdout.
- Text-mode blockers that are part of the command result go to stdout and still
  include `No files changed.` when no mutation happened.
- JSON mode writes one JSON document to stdout; stderr must not receive verbose
  diagnostics or human explanations that would be needed to understand the
  result.
- stderr is reserved for process-level failures outside the normal command
  report path, such as argument parsing or unexpected runtime failures.

## UX rules extracted from this storyboard

Issue #165 should implement these rules before command-specific wording is
completed in #166 and #167:

1. Default output uses plain English first and technical details second.
2. Every command says whether it changed anything.
3. Dry-run output always says `No files changed.`
4. Non-dry-run output must distinguish repo/state writes from live dotfile
   writes.
5. Confirmed write output says what changed.
6. Confirmed live writes say where the backup is and how to preview restore.
7. `no-baseline` becomes a human review note: `No previous sync baseline exists;
   review before confirming.` The safety uncertainty must stay visible in
   default text even when the raw internal flag is hidden.
8. Blocked output says why it blocked, confirms nothing changed, and gives the
   next safe command.
9. Raw selected values and unrelated config values are not printed.
10. Internal identifiers are hidden from default output unless they are useful
    as secondary details.
11. `--verbose` exposes diagnostics without weakening redaction.
12. `--json` remains stable and technical.

## Storyboard setup

The transcript below assumes the user already built or installed a v2-capable
binary and set up a temporary home and repository. Setup commands may use demo
values, but the manager output must not print the raw selected Git email value.

Safety note: these commands set `HOME="$DFM_HOME"` for the demo. In this
storyboard, `$HOME/.gitconfig` refers to the temporary demo Git config, not the
user's real Mac `~/.gitconfig`. Do not remove the `HOME="$DFM_HOME"` prefix
when copying demo commands unless you intend to use your real home.

```bash
DFM=/path/to/dotfiles-manager
DFM_DEMO_ROOT=$(mktemp -d)
DFM_HOME="$DFM_DEMO_ROOT/home"
DFM_REPO="$DFM_DEMO_ROOT/repo"
mkdir -p "$DFM_HOME" "$DFM_REPO"

# Seed temporary Git config with demo values. These are test inputs, not output
# that dotfiles-manager should print later.
HOME="$DFM_HOME" git config --global user.email <demo-email>
HOME="$DFM_HOME" git config --global user.name <demo-name>

cd "$DFM_REPO"
```

## Expected default transcript

### 1. Initialize the v2 repo

Command:

```bash
HOME="$DFM_HOME" "$DFM" init --yes --machine-id test-machine --user-id test-user
```

Expected default output:

```text
Created a dotfiles-manager v2 settings repo.

Created in this folder:
  - dotfiles-manager.v2.yaml
  - profiles/stacks/default.yaml
  - profiles/layers/global.yaml

Saved local identity outside the repo:
  - machine: test-machine
  - user: test-user

No real dotfiles were changed.

Next:
  HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml recipe explain git
```

User takeaway: the repository scaffold exists, local identity is separate from
the repo, and real dotfiles were not touched.

### 2. Explain Git support

Command:

```bash
HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml recipe explain git
```

Expected default output:

```text
Git settings that dotfiles-manager can manage

Can manage:
  - Git user email
    Live file: $HOME/.gitconfig
    Git key: [user] email

  - Git user name
    Live file: $HOME/.gitconfig
    Git key: [user] name

Not managed:
  - credentials and credential helpers
  - SSH or GPG signing keys
  - include/includeIf expansion
  - aliases and arbitrary Git config sections
  - repository-local .git/config

No files changed.

Next:
  HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
    add git --setting user.email --scope user --profile global --yes
```

User takeaway: only safe selected Git identity settings are in scope; secrets and
broader Git config are not managed.

### 3. Select Git user email

Command:

```bash
HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
  add git --setting user.email --scope user --profile global --yes
```

Expected default output:

```text
Selected Git user email for management.

Managed setting:
  Git user email

Live source:
  $HOME/.gitconfig [user] email

Applies to:
  user test-user on all machines

Saved desired value:
  not saved yet

Updated repo configuration:
  profiles/layers/global.yaml

No live Git config was changed.

Next:
  HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
    save --dry-run --user-id test-user git:user.email
```

User takeaway: the setting is selected, but the live Git config was not modified
and no desired value has been saved yet.

### 4. List selected settings

Command:

```bash
HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml list --user-id test-user
```

Expected default output:

```text
Selected settings

Git
  - Git user email
    Scope: user test-user on all machines
    Live source: $HOME/.gitconfig [user] email
    Saved desired value: not saved yet

No files changed.

Next:
  HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
    status --user-id test-user git:user.email
```

User takeaway: Git user email is selected, but saving has not happened.

### 5. Check status before saving

Command:

```bash
HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
  status --user-id test-user git:user.email
```

Expected default output:

```text
Git user email

Status:
  Selected, but not saved to this repo yet.

Live value:
  Found in $HOME/.gitconfig [user] email
  Value hidden for safety.

Saved desired value:
  Not created yet.

No files changed.

Next:
  Preview saving the current live value:
  HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
    save --dry-run --user-id test-user git:user.email
```

User takeaway: there is a live value, but the repo does not yet contain the
saved desired value.

### 6. Preview saving current live value

Command:

```bash
HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
  save --dry-run --user-id test-user git:user.email
```

Expected default output:

```text
Dry run: would save Git user email.

From live file:
  $HOME/.gitconfig [user] email
  Value hidden for safety.

To repo file:
  desired/user/test-user/targets/git/settings.yaml

No files changed.

To confirm:
  HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
    save --yes --user-id test-user git:user.email
```

User takeaway: this command would write to the repo, but dry run did not change
anything.

### 7. Save current live value

Command:

```bash
HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
  save --yes --user-id test-user git:user.email
```

Expected default output:

```text
Saved Git user email as desired state.

Read from:
  $HOME/.gitconfig [user] email
  Value hidden for safety.

Wrote repo file:
  desired/user/test-user/targets/git/settings.yaml

No live Git config was changed.

Next:
  Change the temporary Git email to create drift, then inspect the diff:
  HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
    diff --user-id test-user git:user.email
```

User takeaway: desired state now exists in the repo; the live Git config was only
read, not modified.

### 8. Manually create drift

Command:

```bash
HOME="$DFM_HOME" git config --global user.email <different-demo-email>
```

Expected manager output: none. This is a manual Git command, not a
dotfiles-manager command.

User takeaway: the temporary live Git config now differs from the saved desired
state.

### 9. Show diff after drift

Command:

```bash
HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
  diff --user-id test-user git:user.email
```

Expected default output:

```text
Git user email differs from saved desired state.

Live file:
  $HOME/.gitconfig [user] email
  Current value hidden for safety.

Saved desired value:
  desired/user/test-user/targets/git/settings.yaml
  Desired value hidden for safety.

Diff:
  metadata only; selected values are hidden.

Review note:
  No previous sync baseline exists for this setting.
  This is expected on the first apply; review the paths before confirming.

No files changed.

Next:
  Preview applying the saved desired value:
  HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
    apply --dry-run --user-id test-user git:user.email
```

User takeaway: drift exists, but raw values are not printed.

### 10. Preview applying saved desired value

Command:

```bash
HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
  apply --dry-run --user-id test-user git:user.email
```

Expected default output:

```text
Dry run: would update Git user email.

Would read desired value from:
  desired/user/test-user/targets/git/settings.yaml
  Value hidden for safety.

Would update live file:
  $HOME/.gitconfig [user] email

Backup:
  A local backup of $HOME/.gitconfig would be created before writing.

Review note:
  No previous sync baseline exists for this setting.
  This is expected on the first apply; review the paths before confirming.

No files changed.

To confirm:
  HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
    apply --yes --user-id test-user git:user.email
```

User takeaway: `apply --yes` would change the live Git config, but dry run did
not. A backup will be created first.

### 11. Apply saved desired value

Command:

```bash
HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
  apply --yes --user-id test-user git:user.email
```

Expected default output:

```text
Updated Git user email.

Read desired value from:
  desired/user/test-user/targets/git/settings.yaml
  Value hidden for safety.

Updated live file:
  $HOME/.gitconfig [user] email

Review note:
  No previous sync baseline existed for this setting.
  This was a first apply; the backup below was still created before writing.

Backup created:
  run id: selected-value-YYYYMMDDTHHMMSSZ
  contains: previous $HOME/.gitconfig

To preview undo:
  HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
    restore selected-value-YYYYMMDDTHHMMSSZ --dry-run --user-id test-user
```

User takeaway: the live file changed, a backup exists, and there is an undo
preview command.

### 12. List backups

Command:

```bash
HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml backup list
```

Expected default output:

```text
Local backups

1 restorable backup run found.

selected-value-YYYYMMDDTHHMMSSZ
  Created: 2026-06-13T00:00:00Z
  Contains: previous $HOME/.gitconfig
  Related setting: Git user email

To inspect:
  HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
    backup show selected-value-YYYYMMDDTHHMMSSZ

To preview undo:
  HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
    restore selected-value-YYYYMMDDTHHMMSSZ --dry-run --user-id test-user

Backup payload contents are not printed.

No files changed.
```

User takeaway: there is one restorable backup and the next recovery commands are
visible.

### 13. Show backup details

Command:

```bash
HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
  backup show selected-value-YYYYMMDDTHHMMSSZ
```

Expected default output:

```text
Backup selected-value-YYYYMMDDTHHMMSSZ

Restore status:
  restorable

Contains backup of:
  $HOME/.gitconfig

Related setting:
  Git user email

Important:
  Restore rolls back the backed-up file, not only one semantic Git key.
  Backup payload contents are not printed.

To preview restore:
  HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
    restore selected-value-YYYYMMDDTHHMMSSZ --dry-run --user-id test-user

No files changed.
```

User takeaway: restore is possible, but it restores the backed-up file payload.

### 14. Preview restore

Command:

```bash
HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
  restore selected-value-YYYYMMDDTHHMMSSZ --dry-run --user-id test-user
```

Expected default output:

```text
Dry run: would restore backup selected-value-YYYYMMDDTHHMMSSZ.

Would restore file:
  $HOME/.gitconfig

From backup:
  previous $HOME/.gitconfig captured before apply

Important:
  Restore would replace the backed-up file payload.
  Values are hidden for safety.

No files changed.

To confirm:
  HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
    restore selected-value-YYYYMMDDTHHMMSSZ --yes --user-id test-user
```

User takeaway: restore is previewed safely and no file changed.

### 15. Confirm restore

Command:

```bash
HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
  restore selected-value-YYYYMMDDTHHMMSSZ --yes --user-id test-user
```

Expected default output:

```text
Restored backup selected-value-YYYYMMDDTHHMMSSZ.

Restored file:
  $HOME/.gitconfig

Important:
  This replaced the live `$HOME/.gitconfig` file from the backup.
  Values are hidden for safety.

Backup before restore created:
  run id: restore-YYYYMMDDTHHMMSSZ
  contains: $HOME/.gitconfig as it existed immediately before restore

To preview undo of this restore:
  HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
    restore restore-YYYYMMDDTHHMMSSZ --dry-run --user-id test-user

Next:
  Check status:
  HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
    status --user-id test-user git:user.email
```

User takeaway: restore wrote the live file and status can be checked next.

## Default versus verbose example

The same command can show different levels without changing behavior.

Command:

```bash
HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
  status --user-id test-user git:user.email
```

Default text:

```text
Git user email

Status:
  Selected, but not saved to this repo yet.

Live value:
  Found in $HOME/.gitconfig [user] email
  Value hidden for safety.

Saved desired value:
  Not created yet.

No files changed.

Next:
  HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
    save --dry-run --user-id test-user git:user.email
```

Verbose text may add technical diagnostics after the default explanation:

```text
Technical details:
  setting: git:user.email
  scope: user
  subject: test-user
  planner state: missing-desired
  desired artifact: desired://user/test-user/targets/git/settings#user.email
  resource: user-email
  driver: ini-file
  location: home:.gitconfig
  selector: [user] email
```

JSON output remains the existing machine contract and is not redesigned by this
storyboard.

### Verbose write example

Command:

```bash
HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
  apply --yes --verbose --user-id test-user git:user.email
```

Verbose text keeps the default explanation and then adds diagnostics:

```text
Technical details:
  run id: selected-value-YYYYMMDDTHHMMSSZ
  ledger run: state://ledger/runs/selected-value-YYYYMMDDTHHMMSSZ
  setting: git:user.email
  scope: user
  subject: test-user
  profile stack: default [global]
  desired artifact: desired://user/test-user/targets/git/settings#user.email
  backup artifact: state://backups/selected-value-YYYYMMDDTHHMMSSZ/git_user.email
  resource: user-email
  driver: ini-file
  location: home:.gitconfig
  selector: [user] email
  planner state before write: changed/no-baseline
  values: redacted
```

### Blocked output mini example

Command:

```bash
HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
  apply --dry-run --user-id test-user git:user.email
```

Expected default output when the setting is selected but desired state is
missing:

```text
Cannot apply Git user email yet.

Reason:
  No saved desired value exists in this repo.

Live file:
  $HOME/.gitconfig [user] email

No files changed.

Next:
  Preview saving the current live value first:
  HOME="$DFM_HOME" "$DFM" --config dotfiles-manager.v2.yaml \
    save --dry-run --user-id test-user git:user.email

For diagnostics:
  Re-run the same command with --verbose.
```

Verbose diagnostics may add `planner state: missing-desired`, the desired URI,
resource id, driver id, and selector while keeping values redacted.

## Persona review questions

A storyboard review passes only if each persona can answer the task questions
from the default transcript without an internal glossary.

Required personas:

1. Git-literate first-time user who understands Git but not dotfiles-manager.
2. Cautious non-expert Mac user who can copy commands but worries about touching
   real files.
3. Advanced dotfiles/power user who wants scriptability and auditability while
   keeping default output readable.

Task questions:

- What setting is managed?
- What live file is managed?
- Did `init` touch real dotfiles?
- Did `save --dry-run` change anything?
- What would `save --yes` write?
- Did `save --yes` change the live Git config?
- Did `apply --dry-run` change anything?
- What would `apply --yes` change?
- Was a backup created or would one be created?
- Was the actual Git email printed?
- Where is the saved desired state?
- How would the user preview undo after apply?
- What should the user run next at each step?
- What is explicitly not managed?

Pass criteria:

- The answer is obvious from default output.
- The persona does not need to know what `resource`, `driver`, `selector`,
  `desired://`, `state://`, or `missing-desired` mean.
- The persona can tell dry runs from confirmed writes.
- The persona can tell repo writes from live-file writes.
- The persona understands that values are hidden for safety.

Fail criteria:

- A persona cannot tell whether a command changed files.
- A persona cannot tell what file would be changed by apply or restore.
- A persona thinks secrets or raw values are printed.
- A persona must read an internal spec to know what to do next.

## Implementation handoff

Issue #165 implementation must not begin until this storyboard is reviewed and
accepted.

Issue #165 should use this storyboard as the UX source of truth for
output-tier rules. Issue #166 should implement the selected-setting save/apply
loop wording. Issue #167 should implement setup, recipe, list, and backup
wording. Issue #168 should turn the persona questions into a reusable transcript
review gate.

This storyboard is intentionally narrow. It should unblock implementation rather
than become a parallel design project.
