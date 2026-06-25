# v2 restore preview/confirm storyboard

> [!WARNING]
> Superseded by #212. Public backup/restore commands are out of v2 product
> scope. This storyboard is historical only and must not be used as active CLI
> UX, docs, tests, or acceptance evidence.

Status: superseded historical UX artifact after #212.
Last updated: 2026-06-15.
Scope: visual CLI UX only; no command behavior, renderer, JSON schema, v1
output, native export/import, lifecycle automation, or package/app-control
changes.
Related issues: #113, #165, #167, #169, #171, #179, #181, #185, #187.
Pro pre-validation: <https://chatgpt.com/c/6a2f35bb-7c54-83eb-9112-064b179a8c19>
returned `APPROVED WITH REQUIRED CHANGES`; those required constraints are folded
into this storyboard.
Pro post-validation: <https://chatgpt.com/c/6a2f35bb-7c54-83eb-9112-064b179a8c19> returned `APPROVED` after required copy changes.

## Purpose

This storyboard defines the target user experience for previewing and confirming
restore from a v2 local backup. It is source material for later implementation,
renderer, snapshot-test, and command-documentation work; it is not evidence that
the current CLI already prints this exact copy.

A user should be able to read the restore output and answer:

- Which backup run would be read?
- Which live file or app/config location would be restored?
- Did the command change anything?
- Is restore whole-file/artifact recovery or semantic single-setting rollback?
- Was a backup-before-restore recovery handle created?
- How can the user recover the pre-restore live state when that handle exists?
- What is blocked, unsupported, or intentionally hidden?

## Storyboard status and command-surface note

All transcripts below are **target UX storyboard examples**. They are not
observed current CLI output and they do not create command, JSON, handle-format,
or exit-code contracts.

Executable command examples use only command forms exposed by current help:

- `dotfiles-manager restore <run-id> --dry-run [--user-id <id>]`
- `dotfiles-manager restore <run-id> --yes [--user-id <id>]`
- `dotfiles-manager restore <run-id> --non-interactive [--user-id <id>]`
- `dotfiles-manager restore <run-id> --verbose [--dry-run|--yes] [--user-id <id>]`
- `dotfiles-manager backup list [--json|--verbose]`
- `dotfiles-manager backup show <run-id> [--json|--verbose]`
- `dotfiles-manager status [path-or-ref]`
- `dotfiles-manager diff [path-or-ref]`

The transcripts below remain default-text examples. `restore --verbose` is a
technical text tier added by the later #190 implementation work; it must appear
after the same human summary, preserve redaction, and avoid dry-run/future-tense
wording after a confirmed successful restore. This document still avoids restore
JSON examples; it introduces no JSON fields, enum names, object shapes, or shell
exit-code rules.

Run IDs such as `selected-value-20260615T102030Z` and
`restore-20260615T103000Z` are illustrative handles for the storyboard. They are
not canonical handle-format requirements.

## Non-goals and boundaries

- No CLI implementation or behavior changes.
- No renderer implementation.
- No JSON schema, enum, field-name, object-shape, or scripting contract changes.
- No shell exit-code contract changes.
- No v1 output changes.
- No native export/import support and no change to #113.
- No lifecycle automation, app quit/reopen/reload, app server control, plugin
  installation, or package-manager action.
- No app-level package restore or semantic single-value rollback unless a later
  reviewed driver explicitly supports it.
- No fake command syntax that the CLI does not support.
- No raw managed values, unrelated config values, credentials, tokens, account
  or session data, private keys, secrets, symlink targets, or backup payload
  bytes in examples.
- No broad recovery claim when no backup-before-restore handle exists.

## Handle taxonomy

Restore output needs three separate nouns. Default text should name only the
handles that help a user act safely.

| Handle or ref | Meaning | Default text rule | Technical tier rule |
| --- | --- | --- | --- |
| Source backup run handle | The backup run restore reads from, for example `selected-value-20260615T102030Z`. | Show it in default output because the user selected it and may need it again. | May be accompanied by internal metadata in existing technical surfaces. |
| Backup-before-restore recovery handle | A new backup of the live state immediately before confirmed restore, for example `restore-20260615T103000Z`, when it is successfully created. | Show it only when it actually exists. Use it for recovery guidance. | May be accompanied by metadata in backup list/show technical output. |
| Internal artifact refs | Manager-owned refs such as `state://...`, resource IDs, driver IDs, payload relative paths, ledger refs, or artifact refs. | Hide from default output. | May appear in supported technical diagnostics such as `backup list --verbose`, `backup show <run-id> --verbose`, or existing JSON contracts, while preserving redaction. |

The backup-before-restore recovery handle is not the same thing as the source
backup run handle. The source handle is what restore reads; the recovery handle
is what lets the user recover the pre-restore live file if the confirmed restore
changed the wrong thing.

## Restore model

For the supported file-backed restore path, restore is **whole-file/artifact
recovery**. If the backup item came from `$HOME/.gitconfig`, restore writes the
stored backed-up artifact for that file back to `$HOME/.gitconfig`. The output
must not imply that the tool edits only `Git user email` inside the file unless a
later reviewed driver truly supports semantic single-setting rollback.

This matters when a user managed one setting from a file that contains other
settings. Default output should state the file boundary plainly:

```text
Restore type:
  Whole file/artifact restore.
  The backup item came from Git user email, but confirming restore writes the
  backed-up $HOME/.gitconfig artifact. It does not edit only one key inside the
  file.
```

Values remain hidden. The examples may name the setting and file path, but not
the actual email value, other config values in the same file, backup payload
bytes, or internal payload paths.

## Storyboard setup

The examples use the Git quickstart context because it is already familiar in
v2 UX docs and because current commands support public setting refs such as
`git:user.email`. The story assumes the user previously ran a confirmed apply
that created a restorable backup. Later, the live Git config drifted and the user
wants to recover from that prior backup.

Assumed user-facing context:

- User ID: `leon`.
- Managed setting that caused the backup item: `git:user.email`.
- Live app/config location that restore would write: `$HOME/.gitconfig [user]
  email` as part of a whole-file `$HOME/.gitconfig` artifact.
- Source backup run handle selected by the user:
  `selected-value-20260615T102030Z`.

These names are illustrative. They are not a format or naming contract.

## Expected default dry-run transcript

Command:

```bash
dotfiles-manager restore selected-value-20260615T102030Z --dry-run --user-id leon
```

Expected default output:

```text
Restore preview for backup selected-value-20260615T102030Z.

Would restore 1 live file.

Source backup:
  selected-value-20260615T102030Z

Live file that would be written:
  - $HOME/.gitconfig
    Backup item: Git user email
    File area: [user] email

Restore type:
  Whole file/artifact restore.
  Confirming restore would write the backed-up $HOME/.gitconfig artifact.
  It would not edit only Git user email inside the file.

Safety:
  Dry run: no files were changed.
  No backup-before-restore recovery handle was created because this dry run did
  not write live files.
  Values and backup payload contents are hidden.

Next:
  Review the backup metadata:
  dotfiles-manager backup show selected-value-20260615T102030Z

  If this is the backup you want, confirm restore:
  dotfiles-manager restore selected-value-20260615T102030Z --yes --user-id leon
```

User takeaway: restore preview reads the selected backup metadata and tells the
user exactly which live file would be written. It does not change files, create a
recovery handle, print raw values, or promise semantic single-key rollback.

## Expected default confirmed-restore transcript

Command:

```bash
dotfiles-manager restore selected-value-20260615T102030Z --yes --user-id leon
```

Expected default output:

```text
Restore completed.

Restored 1 live file from backup selected-value-20260615T102030Z.

Live file restored:
  - $HOME/.gitconfig
    Backup item: Git user email
    Restore type: whole file/artifact restore

Backup before restore:
  Created recovery handle restore-20260615T103000Z for the live file as it
  existed immediately before this restore.

Safety:
  Values and backup payload contents are hidden.
  Internal artifact refs are hidden from default output.

Next:
  Check current drift for the restored setting:
  dotfiles-manager status git:user.email

  Preview recovery of the pre-restore live file:
  dotfiles-manager restore restore-20260615T103000Z --dry-run --user-id leon
```

User takeaway: confirmed restore changed the named live file, the source backup
run is visible, and the pre-restore live state has its own recovery handle. The
next restore command is shown only because that recovery handle exists.

## Confirmed restore when backup-before-restore cannot be created

If policy requires a backup-before-restore handle and the tool cannot create it,
confirmed restore must fail closed before any live file write. It must not invent
a recovery handle.

Command:

```bash
dotfiles-manager restore selected-value-20260615T102030Z --yes --user-id leon
```

Expected default output:

```text
Restore blocked before writing live files.

Source backup:
  selected-value-20260615T102030Z

Live file that would have been restored:
  - $HOME/.gitconfig

Reason:
  dotfiles-manager could not create the required backup-before-restore recovery
  handle for the current live file.

Safety:
  No files changed.
  No recovery handle was created.
  Values and backup payload contents are hidden.

Next:
  Review available backups:
  dotfiles-manager backup list

  Review the source backup metadata:
  dotfiles-manager backup show selected-value-20260615T102030Z
```

User takeaway: if the safety backup cannot be created, restore does not continue
and the output does not imply that recovery is available.

## Blocked or unsafe restore examples

### Missing or unknown backup run

Command:

```bash
dotfiles-manager restore missing-run --dry-run --user-id leon
```

Expected default output:

```text
Restore blocked: backup run not found.

Requested backup:
  missing-run

Safety:
  No files changed.
  No backup-before-restore recovery handle was created.

Next:
  List available backups:
  dotfiles-manager backup list
```

The output may mention the requested handle because the user typed it. It must
not expose internal state paths or search private state directories in default
text.

### Backup payload unavailable or invalid

Command:

```bash
dotfiles-manager restore selected-value-20260615T102030Z --yes --user-id leon
```

Expected default output:

```text
Restore blocked: backup payload could not be verified.

Source backup:
  selected-value-20260615T102030Z

Live file that would have been restored:
  - $HOME/.gitconfig

Reason:
  The stored backup artifact for this item is unavailable or failed validation.

Safety:
  No files changed.
  No backup-before-restore recovery handle was created.
  Backup payload contents and values were not printed.

Next:
  Review backup metadata:
  dotfiles-manager backup show selected-value-20260615T102030Z

  For technical metadata that the current CLI supports:
  dotfiles-manager backup show selected-value-20260615T102030Z --verbose
```

This does not introduce a new diagnostic command. It names only existing backup
metadata commands.

### Unsupported native export/import or lifecycle target

This is a default-output snippet, not a full command transcript. It exists to
show the blocked wording for a future backup item whose metadata says it would
require native export/import, lifecycle automation, app control, package-manager
action, or unsupported native state. #113 remains blocked and future.

```text
Restore blocked: this backup item cannot be restored automatically.

Source backup:
  selected-value-20260615T102030Z

Blocked item:
  Native app settings

Reason:
  Restoring this item would require native export/import or lifecycle/app-control
  behavior that dotfiles-manager does not support in the current v2 restore
  flow.

Safety:
  No files changed.
  No backup-before-restore recovery handle was created.
  Account data, session data, native payload contents, and internal refs are not
  printed.

Next:
  Use supported file-backed restore items only, or wait for the separate native
  export/import work after #113 is unblocked and reviewed.
```

User takeaway: native export/import and lifecycle-sensitive restore are blocked,
not mocked as available product behavior.

## Default, technical, and JSON boundaries

### Default text

Default restore text should be human-first. It may show:

- source backup run handle;
- backup-before-restore recovery handle when it exists;
- app/setting display names;
- public setting refs when useful for next commands;
- user-level live paths such as `$HOME/.gitconfig`;
- whether the command changed files;
- whether restore is whole-file/artifact recovery;
- safe next commands that already exist.

Default restore text must not require understanding resource IDs, driver IDs,
selectors, `state://`, payload paths, ledger refs, or artifact refs. It must not
print values, credentials, account/session data, symlink targets, or backup
payload bytes.

### Technical text

`restore --verbose`, `backup list --verbose`, and
`backup show <run-id> --verbose` are technical text surfaces. They may show
driver/resource/location identifiers and internal refs after the human summary,
while preserving the same redaction policy. Confirmed successful restore verbose
text must use completed-work wording (`applied`, actual backup-before-restore
handle) rather than preview wording (`would-change`, `will be created`).

### JSON

JSON is the existing scripting contract. This storyboard does not redesign it.
It intentionally provides no JSON examples, field names, enum names, object
shapes, or exit-code expectations. Any JSON contract change requires a separate
issue and tests.

## UX rules extracted from this storyboard

1. Restore dry-run must say `No files changed.`
2. Restore dry-run must show the source backup run handle and the live file or
   app/config location that would be written.
3. Confirmed restore must show which live files changed.
4. Confirmed restore must show a backup-before-restore recovery handle only when
   one was created.
5. Recovery guidance must use the backup-before-restore handle only if it exists.
6. Restore output must distinguish source backup run handles from recovery
   handles.
7. Internal artifact refs belong in supported technical surfaces, not default
   restore output.
8. Whole-file/artifact restore limits must be visible whenever the user might
   expect a semantic single-setting rollback.
9. Blocked restore output must say why restore cannot proceed and must say no
   files changed.
10. Native export/import, lifecycle automation, app control, package-manager
    actions, and unsupported native/package state remain blocked or future until
    separately reviewed and implemented.
11. Values, secrets, account/session data, symlink targets, and backup payload
    bytes must stay hidden in every output tier.
12. JSON examples and scripting contracts must not be introduced by a UX
    storyboard.

## Review and validation expectations for implementation issues

Future implementation work that consumes this storyboard should add deterministic
snapshot or transcript tests for at least:

- known-backup dry-run;
- confirmed restore with backup-before-restore recovery handle;
- missing backup run;
- invalid/unavailable payload;
- unsupported native/lifecycle item;
- backup-before-restore creation failure;
- redaction of values and backup payload bytes;
- JSON schema stability, if JSON output is touched in a later issue.

A transcript review using
[`v2-transcript-review-gate.md`](v2-transcript-review-gate.md) must be checked
in for UX changes before implementation closure. Issue #187's checked-in review
is
[`reviews/v2-restore-preview-confirm-review.md`](reviews/v2-restore-preview-confirm-review.md).
