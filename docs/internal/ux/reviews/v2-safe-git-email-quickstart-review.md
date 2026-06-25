# v2 safe Git email quickstart transcript review

> [!WARNING]
> Superseded in part by #212. Findings about public backup/restore commands are
> historical only and must not be used as active v2 product guidance.


Status: pass.
Reviewed on: 2026-06-14.
Issue / PR: #168 review for the #167 safe setup/discover/add/list/backup UX slice; PR was not assigned at review time.
Branch / commit: `codex/v2-ux-review-gate-168`; reviewed baseline includes `main` commit `aaf07c372522cf427f1da367b00312a81c1a6bcf`.
Reviewer method: ChatGPT Pro pre-validation plus checked-in persona review. Pro conversation: <https://chatgpt.com/c/6a2e5e89-61bc-83eb-b148-22a7718eeb6a>.
Transcript source: `internal/app/cli_ux167_test.go` generated transcript assertions, cross-checked against `docs/internal/ux/v2-safe-quickstart-output-storyboard.md`.
Commands reviewed: `init`, `recipe discover git`, `recipe explain git`, `add git --setting user.email --scope user --dry-run --yes`, confirmed `add`, `list`, `save --yes git:user.email`, `apply --yes git:user.email`, `backup list`, and `backup show <run-id>`.
Output tiers reviewed: default text, verbose text, `--json`, and `--json --verbose`.
Data handling: test-only values are treated as sensitive and are asserted absent from human transcripts; credential-helper secret text and backup payload bytes are not printed.
Out of scope: no v1 output work, no native export/import support, no lifecycle automation, no new JSON fields, and no broader app support claim.
Deterministic validation: `internal/app/cli_ux167_test.go` asserts human-readable default phrases, verbose-only diagnostics, JSON parseability, and forbidden secret/value strings.

## Persona findings

### Git-literate first-time user

Result: pass.

- Managed app/setting/file: the transcript identifies Git, `git:user.email`, and `$HOME/.gitconfig` in user terms.
- Change/no-change status: setup/add/list/backup output distinguishes selection, saved desired state, live apply, and no-live-config-change cases.
- Desired-state location: the list and save/apply loop uses user-level desired-state language instead of requiring `desired://` knowledge.
- Preview/apply/restore next step: the default output includes next commands such as previewing save/restore and inspecting drift.
- Unsupported or excluded behavior: `recipe explain git` includes a `Not managed:` section for credentials, signing keys, includes, aliases/arbitrary sections, and repository-local config.
- Notes: the default transcript hides implementation refs such as `state://identity`, `recipe://`, `resource=`, `driver=`, `sourceLayer=`, and `selector=`.

### Cautious non-expert Mac user

Result: pass.

- Dry-run/write clarity: the add dry-run output says profile files will not be changed and live app config was not changed.
- Backup/restore clarity: backup output says backups exist, identifies what can restore, provides preview restore guidance, and says payload contents are stored for restore but not printed.
- Risk and safety wording: output distinguishes repo/profile operations from live app config changes, which is the key safety distinction for this slice.
- Redaction/value exposure: the default and verbose transcripts do not print the seeded private email, desired email, credential-helper secret, or backup payload bytes.
- Notes: this review covers the safe Git email slice only. It does not prove aggregate multi-app output, partial success, native export/import, or lifecycle-sensitive app UX.

### Advanced dotfiles/power user

Result: pass.

- Verbose diagnostics: verbose text includes technical refs useful for debugging, including `state://identity`, `recipe://bundled/git`, `resource=`, `driver=`, `sourceLayer=`, and `selector=`.
- JSON/scriptability: JSON and `--json --verbose` outputs parse as JSON and remain prose-free; verbose mode does not append human text to JSON stdout.
- Audit/backup/ledger clarity: backup list/show expose backup run information in the human tier while keeping internal refs for verbose/JSON.
- Unsupported/internal-boundary clarity: default text avoids requiring internal nouns, while verbose preserves technical details for audit/debugging.
- Notes: this is a transcript gate result, not a replacement for fixture, schema, redaction, or integration tests.

## Required-question checklist

| Question | Result | Evidence / note |
| --- | --- | --- |
| What app/setting/file is managed? | pass | Default output includes Git, `git:user.email`, user email, and `$HOME/.gitconfig`. |
| What live file/location is involved? | pass | Discover/explain output names `$HOME/.gitconfig`. |
| Did the command change anything? | pass | Add dry-run and confirmed add distinguish profile selection from live app config; list shows desired state saved/not saved. |
| Is dry-run no-write obvious? | pass | Add dry-run says no profile files will change and no live app config changed. |
| Where is desired state saved? | pass | List/save flow describes desired state in user-level terms and avoids `desired://` in default output. |
| What would apply/restore touch? | pass | Apply identifies Git user email; backup output identifies what can restore. |
| Was/would backup be created? | pass | Backup list/show output says backups are available and gives backup run context. |
| How to preview before apply? | pass | Default output gives preview/inspect-drift next-command guidance. |
| How to undo/restore after apply? | pass | Backup output gives restore preview guidance and explains payloads are stored for restore. |
| What command should run next? | pass | Add/list/backup output include next commands or next action guidance. |
| What is unsupported/excluded/blocked? | pass | `recipe explain git` has `Not managed:` exclusions for risky Git areas. |
| Were raw values/secrets/payloads hidden? | pass | Tests assert absence of private email, desired email, credential-helper secret, and backup payload text. |
| Are verbose diagnostics useful and redacted? | pass | Verbose includes technical IDs/URIs while still excluding secret/raw values. |
| Is JSON stdout JSON-only? | pass | JSON and `--json --verbose` parse as JSON and do not include human prose snippets. |

## Required copy changes

None for the reviewed #167 slice.

## Closure notes

The safe Git email quickstart transcript satisfies the #168 first completed
review requirement. It is acceptable evidence for the setup/discover/explain/add
and list/backup UX slice, while future aggregate, profile, native export/import,
custom app, and lifecycle-sensitive flows still need their own storyboard or
transcript reviews when implemented.
