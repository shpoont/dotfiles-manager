# v2 UX transcript review gate

Status: issue #168 process artifact.
Last updated: 2026-06-25.
Scope: v2 CLI UX review only; no command behavior, JSON schema, or v1 output changes.
Related issues: #165, #166, #167, #168, #169, #171.
Pro pre-validation: <https://chatgpt.com/c/6a2e5e89-61bc-83eb-b148-22a7718eeb6a>.

## Purpose

This gate makes v2 command-output work reviewable from the user's point of
view. A CLI change is not UX-ready merely because the code compiles, snapshots
match, or a reviewer understands the implementation. The reviewer must inspect
before/after command transcripts and confirm that a normal user can understand:

- what app, setting, and live file are involved;
- whether the command changed anything;
- what will happen before a confirmed write;
- where desired state lives in user-level terms;
- how to preview, apply, inspect drift, and recover through supported workflows such as settings-folder version history when configured;
- what is explicitly unsupported or intentionally hidden.

The gate is deliberately lightweight. It can be run by ChatGPT Pro, another
agent acting as a persona, or a human reviewer, but the completed review must be
checked into `docs/internal/ux/reviews/` before the issue is closed. PR and
issue comments may link to that checked-in review; they are not a substitute for
it.

## When this gate is required

Run this gate for any v2 change that can alter, introduce, or materially depend
on user-facing CLI text:

- default text output for commands, blockers, summaries, warnings, next steps,
  or confirmations;
- `--verbose` text output or diagnostics;
- output-tier rules that distinguish default text, verbose text, and `--json`;
- high-fidelity storyboards or docs that future implementation will treat as UX
  source material;
- multi-app or aggregate command output, including partial success and blocked
  states;
- recipe explanation/discovery copy and support-boundary copy;
- native export/import UX once a verified target exists.

The gate is optional for purely internal refactors that cannot change command
text, JSON, docs, examples, or UX expectations. If a PR claims the gate is not
required, the PR should say why.

## Non-goals and boundaries

- This gate does not replace deterministic unit, integration, contract,
  snapshot, safety, or redaction tests.
- LLM approval alone is not a release gate. It is evidence that complements
  tests and manual review.
- Do not weaken safety warnings or omit write-risk information to make output
  shorter.
- Do not print raw managed values, unrelated config values, private keys,
  tokens, credentials, account/session data, or internal recovery payload bytes to satisfy
  a clarity question.
- Do not claim native export/import, lifecycle automation, app restart, plugin
  installation, package-manager actions, or unsupported app coverage unless a
  reviewed recipe and implementation actually provide it.
- Do not change `--json` schemas, field names, enum values, refs, or script
  contracts as part of this gate.

## Output-tier expectations

Reviewers must evaluate the output tiers separately.

### Default text

Default text is the human-first path. A first-time user should not need the v2
spec, an internal glossary, or implementation terms to answer the review
questions. Default text may show app names, setting refs when they are useful
for next commands, user-level paths, and settings-folder paths such as
`desired/user/<user-id>/targets/<app>/...`. It should not require understanding
raw planner states, driver IDs, resource IDs, selectors, `desired://`,
`state://`, or internal ledger refs.

### Verbose text

Verbose text is default text plus diagnostics for maintainers and power users.
It may include technical IDs, URIs, source layers, selectors, driver names,
ledger refs, and compatibility metadata. It must keep the same redaction policy
as default text.

### JSON

`--json` is the stable scripting contract. `--json --verbose` must still emit
only the JSON payload on stdout; verbose prose must not be appended. The review
may check that JSON remains parseable and schema-bearing, but copy suggestions
must not require JSON shape changes unless a separate JSON-contract issue owns
that change.

## Required personas

Every full review must include these personas. A reviewer may answer them
separately or use separate subagents, but the completed artifact must preserve
persona-specific findings.

| Persona | What they care about | Typical failure signs |
| --- | --- | --- |
| Git-literate first-time user | Understands shell/Git, but not dotfiles-manager internals. Wants to know what to run next. | Output starts with implementation nouns, hides the managed app/setting, or assumes profile/layer knowledge. |
| Cautious non-expert Mac user | Can copy commands, but worries about touching real files and losing data. | Dry-run/write status is ambiguous, write-risk guidance is unclear, or warnings are removed for brevity. |
| Advanced dotfiles/power user | Wants scriptability, auditability, exact refs in the right tier, and stable automation contracts. | Default is too noisy, verbose lacks diagnostics, or JSON/prose boundaries are mixed. |

## Required review package

A review request should provide enough material for the personas to judge the
actual user experience rather than the code structure alone.

| Field | Required content |
| --- | --- |
| Issue / PR | GitHub issue and PR numbers or links. |
| Branch / commit | Branch name and, when available, the reviewed commit SHA. |
| Transcript source | Actual command transcript, generated test transcript, storyboard, or snapshot path. State whether it is before, after, or both. |
| Commands reviewed | Exact commands or command classes included in the transcript. |
| Output tiers reviewed | Default, verbose, JSON, or a justified subset. |
| Data handling | State whether values are demo values, redacted values, or real values. Real secrets and account/session data must not be included. |
| Reviewer method | Pro conversation, subagent/persona review, human review, Harbor/Codex eval, or mixed method. |
| Deterministic validation | Tests, link checks, schema checks, or snapshots that complement the persona review. |
| Out-of-scope boundaries | Explicitly state important things the transcript must not imply or change. |

## Transcript questions

Each persona must be able to answer these questions from the transcript without
extra explanation from the implementer:

1. What app, setting, or file is being managed?
2. What live file or app location is involved?
3. Did this command change anything? If yes, what changed?
4. If it was a dry run, is it obvious that no files changed?
5. Where was desired state saved, in user-level terms?
6. What live file would be touched by apply?
7. Does the output avoid promising public backup/restore, and does it explain confirmed-write safety without exposing internal recovery evidence?
8. How would the user preview before applying?
9. How would the user inspect drift or recover using supported workflows after applying?
10. What command should the user run next?
11. What is explicitly not supported, not managed, blocked, or excluded?
12. Were raw managed values, unrelated config values, secrets, credentials,
    account/session data, or internal recovery payload bytes printed?
13. For verbose output: are diagnostics available without weakening redaction?
14. For JSON output: is stdout still JSON-only and suitable for scripts?

For aggregate or multi-app transcripts, also answer:

15. How many apps/settings were checked, changed, unchanged, not saved yet,
    blocked, or failed?
16. Which app/setting owns each blocked reason?
17. Did the output avoid fake subset commands or unsupported selector syntax?
18. Is it clear whether a partial success changed some items while others were
    skipped or blocked?

## Result states

Use one of these states for the overall result and for each persona when useful.

| State | Meaning | Merge/closure guidance |
| --- | --- | --- |
| `pass` | The transcript answers the required questions for the relevant personas and tiers. Minor non-blocking wording notes may remain. | May close the UX gate after deterministic validation also passes. |
| `pass with copy changes` | The flow is structurally safe and understandable, but specific copy changes are required before merge or closure. | Do not mark the issue Done until the named copy changes are implemented or explicitly deferred with owner/date. |
| `fail` | A persona cannot answer a required safety, next-step, support-boundary, redaction, or output-tier question from the transcript. | Block merge/closure for the UX issue; revise output or docs and rerun the gate. |

A review can pass for one tier and fail for another. For example, default text
may pass while verbose fails because diagnostics are missing, or default text
may pass while JSON fails because prose was appended to JSON stdout.

## Completed-review template

Copy this template into `docs/internal/ux/reviews/<short-name>.md` and fill it
in for each completed review.

```markdown
# <review title>

Status: <pass | pass with copy changes | fail>
Reviewed on: YYYY-MM-DD
Issue / PR: #NNN / #NNN
Branch / commit: <branch> / <sha or unknown>
Reviewer method: <Pro URL, subagents, human reviewer, Harbor case, or mixed>
Transcript source: <path or link>
Commands reviewed: <list>
Output tiers reviewed: <default | verbose | json | subset with rationale>
Data handling: <demo/redacted/real-safe; no secrets included>
Out of scope: <important non-goals>
Deterministic validation: <tests/checks and results>

## Persona findings

### Git-literate first-time user

Result: <pass | pass with copy changes | fail>

- Managed app/setting/file:
- Change/no-change status:
- Desired-state location:
- Preview/apply/sync next step:
- Unsupported or excluded behavior:
- Notes:

### Cautious non-expert Mac user

Result: <pass | pass with copy changes | fail>

- Dry-run/write clarity:
- Write-safety/recovery clarity:
- Risk and safety wording:
- Redaction/value exposure:
- Notes:

### Advanced dotfiles/power user

Result: <pass | pass with copy changes | fail>

- Verbose diagnostics:
- JSON/scriptability:
- Audit/ledger/internal-boundary clarity:
- Unsupported/internal-boundary clarity:
- Notes:

## Required-question checklist

| Question | Result | Evidence / note |
| --- | --- | --- |
| What app/setting/file is managed? |  |  |
| What live file/location is involved? |  |  |
| Did the command change anything? |  |  |
| Is dry-run no-write obvious? |  |  |
| Where is desired state saved? |  |  |
| What would apply touch? |  |  |
| Is public backup/restore avoided? |  |  |
| How to preview before apply? |  |  |
| How to inspect drift or recover after apply? |  |  |
| What command should run next? |  |  |
| What is unsupported/excluded/blocked? |  |  |
| Were raw values/secrets/payloads hidden? |  |  |
| Are verbose diagnostics useful and redacted? |  |  |
| Is JSON stdout JSON-only? |  |  |

## Required copy changes

- None, or list exact required changes with owner.

## Closure notes

- Why the issue/PR may or may not proceed.
```

## Relationship to Harbor and agent tests

Harbor/Codex agent tests may use this gate as a rubric for judgment-heavy UX
acceptance cases. A Harbor case should cite this document, include a transcript
or transcript fixture, and state which deterministic tests cover the behavior
that Harbor is not allowed to prove. Generated Harbor results remain local test
artifacts unless a reviewer intentionally summarizes them in a checked-in review
or PR comment.
