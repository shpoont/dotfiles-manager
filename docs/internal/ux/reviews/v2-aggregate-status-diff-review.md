# v2 aggregate selected status/diff transcript review

Status: pass.
Reviewed on: 2026-06-14.
Issue / PR: #177; PR was not assigned at review time.
Branch / commit: `codex/v2-aggregate-status-diff-177`; commit not assigned at review time.
Reviewer method: ChatGPT Pro issue/implementation-plan pre-validation plus checked-in persona review. Pro conversation: <https://chatgpt.com/c/6a2ed9fb-e2dc-83ed-9efb-211517db1950>.
Transcript source: `docs/internal/ux/v2-aggregate-status-diff-storyboard.md`.
Commands reviewed: `status --user-id leon`, `diff --user-id leon`, `status --verbose --user-id leon`, and `status --json --user-id leon` boundary.
Output tiers reviewed: default text, verbose text, and JSON boundary.
Data handling: demo/redacted values only; raw managed values, secrets, credentials, account/session data, and payload bytes are not included.
Out of scope: no CLI implementation, no JSON schema changes, no v1 output changes, no native export/import support, no #113 change, no lifecycle automation, and no fake subset command syntax.
Deterministic validation: pending local markdown/link validation in the PR; this review is judgment evidence, not a replacement for future renderer/snapshot tests.

## Persona findings

### Git-literate first-time user

Result: pass.

- Managed app/setting/file: the transcript names Git, Starship, and Zsh sections with public refs where useful, such as `git:user.email` and `starship:add_newline` in next commands.
- Change/no-change status: the summary separates changed, up to date, not saved yet, blocked, unsupported, and failed counts before per-app details.
- Desired-state location: the changed Git item shows the user-level desired path `desired/user/leon/targets/git/settings.yaml`.
- Preview/apply/restore next step: the transcript gives safe next commands for inspecting the changed Git value and saving the unsaved Starship value.
- Unsupported or excluded behavior: Git credential helper is explicitly unsupported and credential values are not read or printed.
- Notes: the default transcript does not require internal resource, driver, selector, `desired://`, or `state://` knowledge.

### Cautious non-expert Mac user

Result: pass.

- Dry-run/write clarity: both `status` and `diff` say they are read-only and changed no files.
- Backup/restore clarity: backup is not relevant because these commands do not write; the transcript does not imply a backup was created.
- Risk and safety wording: blocked Zsh explains that an unsafe symlink was not read or diffed; broad apply is discouraged until blocked/unsupported/failed items are resolved.
- Redaction/value exposure: changed values, live values, credential helpers, raw desired values, and symlink targets remain hidden.
- Notes: the transcript avoids native export/import, lifecycle automation, app restart/reload, package-manager action, and plugin installation claims.

### Advanced dotfiles/power user

Result: pass.

- Verbose diagnostics: verbose output preserves the human summary and appends technical refs, states, actions, diagnostics, resources, drivers, locations, selectors, and URIs.
- JSON/scriptability: JSON boundary says stdout remains one JSON document and no prose is appended; exact schema remains owned elsewhere.
- Audit/backup/ledger clarity: no backup/ledger success is implied for read-only commands; technical diagnostics are tiered into verbose/JSON.
- Unsupported/internal-boundary clarity: the storyboard distinguishes unsupported from blocked and failed, and it rejects invented subset syntax.
- Notes: future implementation still needs deterministic renderer/snapshot tests and JSON contract tests.

## Required-question checklist

| Question | Result | Evidence / note |
| --- | --- | --- |
| What app/setting/file is managed? | pass | Per-app Git, Starship, and Zsh sections name each item. |
| What live file/location is involved? | pass | Default output names `$HOME/.gitconfig`, `$HOME/.config/starship.toml`, and `$HOME/.zshrc` where useful. |
| Did the command change anything? | pass | `status` and `diff` both say read-only/no files changed. |
| Is dry-run/no-write obvious? | pass | The commands are read-only and no-write status is explicit. |
| Where is desired state saved? | pass | Changed Git item shows `desired/user/leon/targets/git/settings.yaml`. |
| What would apply/restore touch? | pass | Diff next step narrows apply preview to `git:user.email`; restore is not implied for read-only commands. |
| Was/would backup be created? | pass | No backup is claimed because no write is performed. |
| How to preview before apply? | pass | `dotfiles-manager apply --dry-run --user-id leon git:user.email` is shown for the one diffable change. |
| How to undo/restore after apply? | pass | Not applicable to this read-only storyboard; no apply has happened. Future apply storyboards must cover restore. |
| What command should run next? | pass | The transcript gives supported narrow commands and says no single broad command can resolve everything. |
| What is unsupported/excluded/blocked? | pass | Credential helper is unsupported; Zsh unsafe symlink is blocked; Starship scan timeout failed. |
| Were raw values/secrets/payloads hidden? | pass | Default and verbose examples explicitly keep all values and payloads hidden. |
| Are verbose diagnostics useful and redacted? | pass | Technical details include refs/diagnostics but hide raw desired values and symlink target. |
| Is JSON stdout JSON-only? | pass | JSON boundary states one JSON document only and no human prose. |
| How many apps/settings were checked and counted? | pass | Counts show 6 settings across 3 apps with all required states. |
| Which app/setting owns each blocked reason? | pass | Reasons are under Git, Starship, or Zsh sections. |
| Did output avoid fake subset commands? | pass | The storyboard permits only public setting refs and explicitly forbids invented syntax. |
| Is partial/blocked state clear? | pass | It says broad apply should wait until blocked, unsupported, and failed items are resolved. |

## Required copy changes

None for this storyboard review.

## Closure notes

The storyboard satisfies the #177 transcript-review requirement as a
pre-implementation UX artifact. Future renderer work must still add deterministic
text snapshots, JSON contract coverage, and redaction regression tests before
claiming implemented aggregate behavior.
