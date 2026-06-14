# v2 aggregate save/apply transcript review

Status: pass.
Reviewed on: 2026-06-14.
Issue / PR: #179; PR was not assigned at review time.
Branch / commit: `codex/v2-aggregate-save-apply-179`; commit not assigned at review time.
Reviewer method: external Pro pre-validation plus checked-in persona review; post-validation completed before commit.
Transcript source: `docs/internal/ux/v2-aggregate-save-apply-storyboard.md`.
Commands reviewed: `save --dry-run --user-id leon`, `save --yes --user-id leon`, `apply --dry-run --user-id leon`, `apply --yes --user-id leon`, and `apply --dry-run --json --user-id leon` boundary. Narrow safe-confirm examples with `git:user.email` were reviewed as next-command examples, not as the primary #179 coverage.
Output tiers reviewed: default text, verbose text, and JSON boundary.
Data handling: demo/redacted values only; raw managed values, secrets, credentials, account/session data, symlink targets, and payload bytes are not included.
Out of scope: no CLI implementation, no renderer implementation, no JSON schema changes, no v1 output changes, no native export/import support, no #113 change, no lifecycle automation, and no fake subset command syntax.
Deterministic validation: passed locally with `git diff --check`, markdown relative link checker, and `go test ./...`; this review is judgment evidence, not a replacement for future renderer/snapshot tests.

## Persona findings

### Git-literate first-time user

Result: pass.

- Managed app/setting/file: the transcript names Git, Starship, and Zsh sections with public refs such as `git:user.email` and `starship:add_newline` where they are useful for next commands.
- Change/no-change status: the summary separates would-save/would-apply, already saved/up to date, blocked, unsupported, failed, and skipped/not-applicable states.
- Desired-state location: save output names `desired/user/leon/targets/git/settings.yaml` and `desired/user/leon/targets/starship/settings.yaml` as repo write targets.
- Preview/apply/restore next step: dry-run output gives safe narrow confirm commands; confirmed apply gives `status` and supported restore-preview guidance.
- Unsupported or excluded behavior: Git credential helper is unsupported, credentials are not read or printed, and native export/import remains out of scope.
- Notes: the default transcript does not require internal resource, driver, selector, `desired://`, `state://`, ledger, or backup-artifact knowledge.

### Cautious non-expert Mac user

Result: pass.

- Dry-run/write clarity: both dry-run transcripts explicitly say no files changed; confirmed transcripts distinguish repo desired-state writes from live config writes.
- Backup/restore clarity: apply dry-run says backup would be created before confirmed live writes where required; confirmed apply shows a backup run and restore preview only for the supported restore-preview flow.
- Risk and safety wording: blocked Zsh explains that an unsafe symlink was not read or written; broad apply/save is discouraged until blocked/unsupported/failed/skipped items are resolved.
- Redaction/value exposure: current values, desired values, credential helpers, symlink targets, and payload bytes remain hidden in default and verbose output.
- Notes: the transcript avoids native export/import, lifecycle automation, app restart/reload, package-manager action, and plugin-install claims.

### Advanced dotfiles/power user

Result: pass.

- Verbose diagnostics: verbose output preserves the human summary and appends technical refs, states, actions, diagnostics, resources, drivers, locations, selectors, desired/state URIs, and backup intent.
- JSON/scriptability: JSON boundary shows a JSON-only placeholder and states exact schema is owned elsewhere; no prose is appended to JSON output.
- Audit/backup/ledger clarity: confirmed apply reports live file changes, backup handle, and desired-state repo non-change; confirmed save reports desired-state repo writes and no live-file backup.
- Unsupported/internal-boundary clarity: the storyboard distinguishes unsupported from blocked, failed, and skipped/not applicable, and rejects invented subset syntax.
- Notes: future implementation still needs deterministic renderer/snapshot tests, JSON contract coverage, and redaction regression tests.

## Required-question checklist

| Question | Result | Evidence / note |
| --- | --- | --- |
| What app/setting/file is managed? | pass | Per-app Git, Starship, and Zsh sections name each item. |
| What live file or app location is involved? | pass | Default output names `$HOME/.gitconfig`, `$HOME/.config/starship.toml`, and `$HOME/.zshrc` where useful. |
| Did this command change anything? | pass | Dry runs say no files changed; confirmed save/apply say what was written. |
| Is dry-run/no-write obvious? | pass | `save --dry-run` and `apply --dry-run` both say no files were changed. |
| Where was desired state saved? | pass | Confirmed save shows `desired/user/leon/targets/git/settings.yaml`. |
| What live file would apply or restore touch? | pass | Apply output names `$HOME/.gitconfig [user] email`; restore preview is tied to the backup run. |
| Was/would backup be created? | pass | Apply dry-run previews backup creation; confirmed apply shows backup run; save says no live-file backup was created. |
| How to preview before applying? | pass | `dotfiles-manager apply --dry-run --user-id leon git:user.email` is shown after save. |
| How to undo/restore after apply? | pass | Confirmed apply shows `dotfiles-manager restore --dry-run <backup-run-id>` only because restore preview is supported. |
| What command should run next? | pass | The transcript gives supported narrow commands and says no single broad command can resolve everything. |
| What is unsupported/excluded/blocked? | pass | Credential helper is unsupported; Zsh unsafe symlink is blocked; Starship scan timeout failed; Starship add_newline is skipped for apply. |
| Were raw values/secrets/payloads hidden? | pass | Default and verbose examples explicitly keep all values, symlink targets, and payload bytes hidden. |
| Are verbose diagnostics useful and redacted? | pass | Technical details include refs/diagnostics but hide raw desired value and symlink target. |
| Is JSON stdout JSON-only? | pass | JSON boundary contains a JSON-only placeholder and no prose inside the output block. |
| How many apps/settings were checked and counted? | pass | Counts show 6 settings across 3 apps with all required states. |
| Which app/setting owns each blocked reason? | pass | Reasons are under Git, Starship, or Zsh sections. |
| Did output avoid fake subset commands? | pass | The storyboard permits only public setting refs and explicitly forbids invented syntax. |
| Is partial/blocked state clear? | pass | Partial summaries say blocked, unsupported, failed, and skipped items were not saved/applied. |
| Is `save` versus `apply` write boundary clear? | pass | Save writes desired-state repo files; apply writes live config files and does not change desired-state repo files. |
| Are backup/recovery commands only shown where supported? | pass | Restore preview is shown only for a supported restore-preview flow; otherwise the rule says to show the handle without inventing a command. |

## Required copy changes

None for this storyboard review.

## Closure notes

The storyboard satisfies the #179 transcript-review requirement as a
pre-implementation UX artifact. Project fields: verified separately on GitHub
for #179 before closing the issue. Future renderer work must still add deterministic
text snapshots, JSON contract coverage, and redaction regression tests before
claiming implemented aggregate save/apply behavior.
