# v2 restore preview/confirm transcript review

> [!WARNING]
> Superseded by #212. This restore review is historical only and must not be
> used as active public v2 product guidance.


Status: pass.
Reviewed on: 2026-06-15.
Issue / PR: #187; PR was not assigned at review time.
Branch / commit: `codex/v2-restore-preview-confirm-187`; commit not assigned at review time.
Reviewer method: external Pro pre-validation, checked-in persona review, and Pro post-validation at <https://chatgpt.com/c/6a2f35bb-7c54-83eb-9112-064b179a8c19>; final re-check returned `APPROVED` after required copy changes.
Transcript source: `docs/internal/ux/v2-restore-preview-confirm-storyboard.md`.
Commands reviewed: `dotfiles-manager restore <run-id> --dry-run --user-id leon`, `dotfiles-manager restore <run-id> --yes --user-id leon`, `dotfiles-manager backup list`, `dotfiles-manager backup show <run-id>`, `dotfiles-manager backup show <run-id> --verbose`, `dotfiles-manager status git:user.email`, and blocked restore snippets that are explicitly non-executable where no supported command surface exists.
Output tiers reviewed: default text, existing backup technical text boundary, and JSON boundary without JSON examples.
Data handling: demo/redacted values only; raw managed values, unrelated config values, credentials, tokens, account/session data, private keys, symlink targets, and backup payload bytes are not included.
Out of scope: no CLI implementation, no renderer implementation, no JSON schema changes, no shell exit-code contract changes, no v1 output changes, no native export/import support, no #113 change, no lifecycle automation, no app quit/reopen/reload, no package-manager or plugin action, and no fake restore verbose command.
Deterministic validation: passed locally on 2026-06-15 with `git diff --check`, a markdown relative-link checker for changed UX docs, grep/sanity checks that #113 remains blocked/future and restore is not overclaimed beyond docs/storyboard coverage, no executable restore verbose-flag examples, no broad one-command reversal claim, no recovery guidance without a backup-before-restore handle, and `$HOME/.asdf/shims/go test ./...`.

## Persona findings

### Git-literate first-time user

Result: pass.

- Managed app/setting/file: the storyboard names Git user email and the live
  file `$HOME/.gitconfig` in user-level terms.
- Change/no-change status: dry-run says no files changed; confirmed restore says
  one live file was restored; blocked examples say no files changed.
- Desired-state location: restore reads backup state rather than desired-state
  repo files, and the storyboard keeps that distinction clear.
- Preview/apply/restore next step: the dry-run shows the supported confirm
  command, confirmed restore shows `status git:user.email`, and recovery uses a
  backup-before-restore handle only when that handle exists.
- Unsupported or excluded behavior: native export/import, lifecycle/app-control,
  package-manager action, and unsupported native state remain blocked/future.
- Notes: the transcript explains that restore is whole-file/artifact recovery,
  not semantic single-setting rollback.

### Cautious non-expert Mac user

Result: pass.

- Dry-run/write clarity: the dry-run transcript explicitly says no files were
  changed and no backup-before-restore handle was created.
- Backup/restore clarity: source backup and backup-before-restore recovery
  handle are separate nouns; the recovery handle appears only in the confirmed
  restore success example.
- Risk and safety wording: the confirmed restore transcript identifies the live
  file restored, and blocked restore stops before live writes when a payload is
  invalid, a backup is missing, a recovery handle cannot be created, or the item
  requires unsupported native/lifecycle behavior.
- Redaction/value exposure: raw email values, unrelated config values, secrets,
  account/session data, symlink targets, and payload bytes are not printed.
- Notes: the wording uses recovery guidance and does not promise recovery when
  no backup-before-restore handle exists.

### Advanced dotfiles/power user

Result: pass.

- Verbose diagnostics: the technical boundary points to existing backup
  `--verbose` surfaces and explicitly avoids executable restore verbose-flag examples.
- JSON/scriptability: the storyboard introduces no JSON examples, schema fields,
  enum names, object shapes, or exit-code behavior.
- Audit/backup/ledger clarity: source backup handles, recovery handles, and
  internal artifact refs are defined separately; internal refs stay out of
  default output.
- Unsupported/internal-boundary clarity: the unsupported native/lifecycle snippet
  is labeled a default-output snippet rather than a command transcript and says
  #113 remains blocked/future.
- Notes: future implementation still needs renderer/snapshot tests, redaction
  regression tests, and any JSON contract tests if JSON output changes later.

## Required-question checklist

| Question | Result | Evidence / note |
| --- | --- | --- |
| What app/setting/file is managed? | pass | Git user email and `$HOME/.gitconfig` are named in the storyboard setup and transcripts. |
| What live file would restore touch? | pass | Dry-run and confirmed restore name `$HOME/.gitconfig`. |
| Did the command change anything? | pass | Dry-run and blocked examples say no files changed; confirmed restore says the live file was restored. |
| Is dry-run/no-write obvious? | pass | The dry-run safety block states no files were changed and no recovery handle was created. |
| Where was desired state saved? | pass | Not applicable to restore; the review confirms restore reads backup state, not desired-state repo files. |
| Was a backup created? | pass | Confirmed restore shows a backup-before-restore recovery handle only when created; blocked examples do not invent one. |
| How to preview before restoring? | pass | The primary preview command is `dotfiles-manager restore <run-id> --dry-run --user-id leon`. |
| How to recover after restore? | pass | Recovery preview uses the backup-before-restore handle only in the successful confirmed restore example. |
| What command should run next? | pass | Default outputs give supported `backup show`, `backup list`, `restore --yes`, `status`, or recovery-preview commands. |
| What is unsupported/excluded/blocked? | pass | Missing run, invalid payload, unavailable recovery backup, native export/import, lifecycle/app-control, and unsupported native state are blocked. |
| Were raw values/secrets/payloads hidden? | pass | The examples do not include raw values, secrets, account/session data, symlink targets, or backup payload bytes. |
| Are technical diagnostics useful and redacted? | pass | Technical refs are confined to supported backup technical surfaces and must preserve redaction. |
| Is JSON stdout JSON-only? | pass | The storyboard contains no JSON examples and states JSON contracts are unchanged. |
| Did output avoid fake commands? | pass | Executable command examples are limited to current help; unsupported native/lifecycle restore is a labeled snippet. |
| Does the storyboard avoid implementation overclaiming? | pass | It states target UX only and docs/storyboard coverage only, not observed output or implemented behavior. |
| Does #113 remain blocked? | pass | Native export/import remains blocked/future and unchanged by #187. |

## Required copy changes

None for this checked-in review after applying Pro pre-validation constraints.

## Closure notes

This review satisfies the #187 transcript-review requirement as a docs-only
restore preview/confirm UX artifact. Future implementation work must still add
deterministic renderer/snapshot tests, redaction regression tests, and any JSON
contract tests if JSON output changes later before claiming implemented restore
behavior.
