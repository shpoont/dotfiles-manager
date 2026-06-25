# v2 aggregate selected status/diff storyboard

Status: issue #177 pre-implementation UX artifact.
Last updated: 2026-06-14.
Scope: visual CLI UX only; no command behavior, JSON schema, v1 output, or native export/import changes.
Related issues: #165, #166, #168, #171, #177.
ChatGPT Pro pre-validation: <https://chatgpt.com/c/6a2ed9fb-e2dc-83ed-9efb-211517db1950>.

## Purpose

This storyboard defines what aggregate selected `status` and `diff` output
should feel like when a user has more than one selected app or setting. It is a
pre-implementation UX artifact for future renderer and test work.

The target user should be able to read the aggregate transcript and answer:

- Which apps and settings were checked?
- Which settings changed, are up to date, are not saved yet, are blocked, are
  unsupported, or failed?
- Did this command write anything?
- Which values are hidden or redacted?
- Which app/setting owns each blocked, unsupported, or failed reason?
- What command is safe to run next?
- What is explicitly not supported or not managed?

## Non-goals

- No CLI behavior implementation.
- No renderer implementation.
- No JSON schema, enum, field-name, or scripting contract changes.
- No v1 output changes.
- No native export/import support and no change to #113.
- No lifecycle automation, app restart/reload, plugin installation, or
  package-manager action.
- No fake subset command syntax when the current CLI grammar cannot express the
  subset.
- No raw managed values, unrelated config values, credentials, tokens, account
  or session data, private keys, secrets, or internal recovery payload bytes in examples.

## Storyboard setup

The transcript below assumes a user has selected several settings across
multiple apps. The scenario intentionally includes supported items, an
unsupported/not-managed item, a blocked item, and a failed item so the aggregate
shape is explicit.

Demo state:

| App | Item | Aggregate state | User-facing reason |
| --- | --- | --- | --- |
| Git | `git:user.email` | changed | Live value differs from saved desired state. |
| Git | `git:user.name` | up to date | Live value matches saved desired state. |
| Git | `git:credential.helper` | unsupported | Credential helpers are explicitly not managed. |
| Starship | `starship:add_newline` | not saved yet | Selected and live value exists, but no desired state has been saved. |
| Starship | `starship:scan_timeout` | failed | Saved desired state cannot be read; default output stays user-facing. |
| Zsh | `zsh:zshrc` | blocked | Live path is an unsafe symlink, so no read or diff is attempted. |

The examples use canonical user ID `leon` consistently in commands and
`desired/user/leon/...` paths. They do not print actual values. They do not use
native export/import, app-control, or #113-blocked native behavior as the
scenario.

## Output-tier boundaries for aggregate runs

### Default text

Default text is the human-first tier. It should show:

- one-line scope summary;
- summary counts for changed, up to date, not saved yet, blocked, unsupported,
  and failed;
- read-only/no-write status for `status` and `diff`;
- per-app sections;
- one plain-language line per setting/item;
- user-level live paths or desired-state paths only where they help the next
  action;
- redaction statements when values exist but are hidden;
- one safe next command when one command exists;
- safe alternatives when no single command can express the useful subset.

Default text must not require understanding `resource`, `driver`, `selector`,
`desired://`, `state://`, raw planner states, raw actions, raw ledger refs, or
internal recovery artifact refs.

### Verbose text

Verbose text appends diagnostics after the same human summary. It may include
technical refs, resource IDs, driver IDs, selectors, locations, raw state/action
names, desired/state URIs, and diagnostic codes. It must keep the same redaction
policy as default text.

### JSON

`--json` remains the scripting contract. This storyboard does not redesign JSON.
`--json --verbose` must still write only one JSON document to stdout, with no
human prose appended. Any future JSON shape changes require a separate JSON
contract issue.

## Expected default `status` transcript

Command:

```bash
dotfiles-manager status --user-id leon
```

Expected default output:

```text
6 settings checked across 3 apps.

Summary:
  Changed: 1
  Up to date: 1
  Not saved yet: 1
  Blocked: 1
  Unsupported: 1
  Failed: 1

Git
  - Git user email: differs from saved desired state
    Live file: $HOME/.gitconfig [user] email
    Desired state: desired/user/leon/targets/git/settings.yaml
    Value hidden for safety.

  - Git user name: up to date
    Live file: $HOME/.gitconfig [user] name

  - Git credential helper: unsupported
    Reason: credentials and credential helpers are not managed by the Git recipe.
    No credential value was read or printed.

Starship
  - Add newline: selected, but not saved yet
    Live file: $HOME/.config/starship.toml key add_newline
    Live value exists and is hidden.

  - Scan timeout: failed
    Reason: saved desired state could not be read.
    No live Starship file was changed.

Zsh
  - .zshrc: blocked
    Reason: the live path is an unsafe symlink. dotfiles-manager did not read or
    diff the symlink target.

Read-only command: no files were changed.

Next:
  No single command can safely resolve every item above.

  Inspect the changed Git value:
  dotfiles-manager diff --user-id leon git:user.email

  Preview saving the Starship value that is not saved yet:
  dotfiles-manager save --dry-run --user-id leon starship:add_newline

  Diagnose the failed and blocked items:
  dotfiles-manager status --verbose --user-id leon
```

User takeaway: the user sees the aggregate counts first, then per-app details.
The unsupported Git credential helper is not treated as merely blocked; it is
outside the recipe's support boundary. The failed Starship item stays
user-facing in default output and does not expose parser internals. The Zsh item
says why it was not read. No command writes anything.

## Expected default `diff` transcript

Command:

```bash
dotfiles-manager diff --user-id leon
```

Expected default output:

```text
Diff checked 6 settings across 3 apps.

Summary:
  Diff available: 1
  Up to date: 1
  Not saved yet: 1
  Blocked: 1
  Unsupported: 1
  Failed: 1

Git
  - Git user email: differs from saved desired state
    Live file: $HOME/.gitconfig [user] email
    Desired state: desired/user/leon/targets/git/settings.yaml
    Current and desired values are hidden.

  - Git user name: up to date

  - Git credential helper: unsupported
    Reason: credentials and credential helpers are not managed by the Git recipe.
    No credential value was read or printed.

Starship
  - Add newline: no diff yet
    Reason: selected, but no saved desired state exists.
    Preview saving the live value first:
    dotfiles-manager save --dry-run --user-id leon starship:add_newline

  - Scan timeout: failed
    Reason: saved desired state could not be read.
    Re-run with --verbose for diagnostic details.

Zsh
  - .zshrc: blocked
    Reason: the live path is an unsafe symlink. No diff was produced.

Read-only command: no files were changed.

Next:
  Preview applying the one diffable Git change:
  dotfiles-manager apply --dry-run --user-id leon git:user.email

  Resolve blocked, unsupported, and failed items before running a broad apply.
```

The final next command is safe because `git:user.email` is a supported public
setting ref. If a future run has multiple ready items but the CLI cannot express
that exact subset safely, the output must not invent syntax such as
`apply --ready-only` or `apply git starship except zsh`. It should instead say
that no single safe command is available and give supported narrower commands or
resolution steps.

## Expected verbose addition

Command:

```bash
dotfiles-manager status --verbose --user-id leon
```

Expected verbose structure:

```text
6 settings checked across 3 apps.

Summary:
  Changed: 1
  Up to date: 1
  Not saved yet: 1
  Blocked: 1
  Unsupported: 1
  Failed: 1

Git
  - Git user email: differs from saved desired state
    Live file: $HOME/.gitconfig [user] email
    Desired state: desired/user/leon/targets/git/settings.yaml
    Value hidden for safety.

  ... same human summary as default ...

Technical details:
  profileStack: default
  activeLayers: global

  git:user.email
    state: changed
    action: inspect-diff
    resource: user-email
    driver: ini-file
    location: home
    selector: [user] email
    desired: desired://user/leon/targets/git/settings.yaml#values.user.email
    live: state://selected-live/git/user.email

  git:credential.helper
    state: unsupported
    diagnostic: selectedpreview.resource.unknown
    reason: unsupported setting; credentials are excluded by recipe policy

  starship:scan_timeout
    state: failed
    diagnostic: selectedvalue.starship.integerTypeUnsupported
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
dotfiles-manager status --json --user-id leon
```

Boundary expectation:

```text
stdout: exactly one JSON document that follows the promoted JSON contract
stderr: empty for normal command-result diagnostics
human prose: none
verbose prose with --json --verbose: none
```

This storyboard intentionally does not show a JSON object, field names, enum
values, or item shape. Exact JSON output remains owned by the promoted JSON
contract and future implementation tests.

## Rules extracted from this storyboard

1. Aggregate output starts with counts before item details.
2. Counts distinguish `blocked`, `unsupported`, and `failed`; they are not the
   same user problem.
3. Per-app sections own their reasons. A blocked Zsh item should not appear as a
   generic top-level blocker with no owner.
4. `status` and `diff` say they are read-only and changed no files.
5. Redacted values are described by existence and relationship, not printed.
6. Default output stays user-facing even for failures; parser, artifact, and
   diagnostic internals belong in verbose/JSON.
7. Verbose output appends technical details after the human summary and keeps
   redaction.
8. JSON output remains JSON-only and schema-owned elsewhere.
9. Next commands must be real supported public commands. If no safe single
   command exists, the output says so instead of inventing syntax.
10. Native export/import, lifecycle automation, plugin installation, and
    package-manager actions must not be implied by aggregate UX examples.
