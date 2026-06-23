---
owner: Product + Core Engineering
document-type: v2-active-behavior-spec
status: Active behavior spec
last-updated: 2026-06-23
canonical-source: docs/internal/specs/v2/12-status-diff-read-only-contract.md
source-issue: 221
authority: Authoritative v2 behavior contract for read-only status and diff output only; write/sync behavior remains out of scope.
---

# v2 read-only status and diff contract

## Purpose

This spec defines the active v2 contract for read-only `status` and `diff`
commands. It turns the reset vocabulary from `00-vocabulary.md` into concrete
human-first output expectations before any mutating sync behavior is built.

`status` and `diff` answer these questions:

- What selected app/tool settings were checked?
- Do live settings and stored settings match?
- If they differ, which side changed: live settings, stored settings, or both?
- Is an app/tool unavailable or are stored settings missing?
- Did the command write anything? The answer must always be no.

## Authority and boundaries

This is an **Active behavior spec** only for read-only `status` and `diff`.

The commands may inspect live settings and stored settings. They must not create,
update, delete, sync, initialize, restore, back up, migrate, repair, normalize in
place, or otherwise mutate app/tool files or the settings folder.

`04-status-conflict-state-machine.md` remains draft reference material. It may
inform state derivation, but it is not normative for public v2 output because it
still contains prototype-era `desired`/`current` and `save`/`apply` wording.

## Public vocabulary requirements

Normal text output must use the accepted #210 vocabulary:

- settings folder;
- live settings;
- stored settings;
- status;
- diff;
- sync;
- conflict;
- public refs such as `git:user.email`.

Normal text output must not require users to understand these terms:

- repo or repository;
- `desired://` or other internal URI schemes;
- driver;
- resource;
- backup or restore;
- v1 migration;
- profile stack internals.

Verbose, JSON, debug, and authoring contexts may include stable internal IDs
where they are useful, but normal output must be understandable without them.
Raw values must remain hidden unless a recipe explicitly declares them safe for
normal display.

## Command surfaces

The read-only command surface must support these selection levels:

```text
dotfiles-manager status
dotfiles-manager status <app-ref>
dotfiles-manager status <setting-ref>

dotfiles-manager diff
dotfiles-manager diff <app-ref>
dotfiles-manager diff <setting-ref>
```

Examples:

```text
dotfiles-manager status
dotfiles-manager status git
dotfiles-manager status git:user.email

dotfiles-manager diff
dotfiles-manager diff starship
dotfiles-manager diff starship:config
```

The exact flags for identity, settings-folder path, verbosity, JSON output, and
non-interactive operation are owned by later CLI-contract work. This issue owns
only the read-only output contract and fixture expectations.

## Output tiers

### Normal text

Normal text is the default and must be human-first. It should contain:

1. a short heading that states what was checked;
2. clear counts for many-app runs;
3. stable per-app grouping;
4. one line per setting, using public refs;
5. explicit direction labels when differences exist;
6. a no-write statement;
7. safe next-step wording that names direction but does not require final sync
   command syntax.

Normal text may name user-level live locations when helpful, such as
`$HOME/.gitconfig [user] email`, but it must not expose raw values or internal
storage URIs.

### Verbose text

Verbose text must include the same human-first summary as normal text, then may
append technical diagnostics. Verbose text may expose recipe IDs, driver IDs,
resource IDs, internal state codes, internal paths, and internal URIs when useful
for debugging or recipe authoring. It must keep the same redaction policy as
normal text.

### JSON

JSON output is the scripting boundary. This spec requires JSON to use the same
public state meanings and direction concepts as normal text. It does not freeze a
full JSON schema.

A JSON result for `status` or `diff` must still represent:

- command name;
- selection level;
- checked apps/settings;
- per-setting state;
- direction, when a difference exists;
- whether a diff is available;
- whether values were redacted;
- diagnostics for blocked, missing, unsupported, or failed items.

JSON may include internal IDs that normal text hides, but it must not use
internal IDs as the only way to understand direction or status.

## Read-only states

| Condition | Normal status label | Direction label | Diff availability | Meaning |
| --- | --- | --- | --- | --- |
| live settings and stored settings match | Up to date | None | no diff | Nothing changed between the two sides. |
| live settings changed and stored settings still match the previous known baseline | Changed in live settings | live settings -> stored settings | diff available if values are displayable | The app/tool has a local change that can later be copied to the settings folder. |
| stored settings changed and live settings still match the previous known baseline | Changed in stored settings | stored settings -> live settings | diff available if values are displayable | The settings folder has a change that can later be copied to the app/tool. |
| both sides changed or no safe baseline exists | Conflict | both sides changed | diff available if values are displayable | The manager cannot choose a safe direction without a user decision. |
| app/tool is unavailable, missing, or cannot be inspected | App not available | none | no diff | The manager could not inspect live settings for the selected app/tool. |
| setting is selected but no stored settings exist | No stored settings yet | live settings -> stored settings, if live settings exist | no stored-side diff | The settings folder does not yet contain stored settings for this selected setting. |
| stored settings exist but live setting is missing | Missing in live settings | stored settings -> live settings, if safe | no live-side diff | The app/tool does not currently have the selected live setting. |
| setting is explicitly unsupported or excluded | Not managed | none | no diff | The recipe intentionally does not manage this setting. |
| read or parse failed | Failed to inspect | none | no diff | The manager could not determine status safely. |

`Conflict` must not be rendered as if one side wins. It requires later sync
planning or a user decision owned by #222-#225.

## Direction language for `diff`

Every diff must state the comparison direction in words before any diff body.

Required wording pattern:

```text
Comparing live settings with stored settings.
Direction: <direction label>.
```

For stored-side changes, the output must say that stored settings changed. For
live-side changes, the output must say that live settings changed. For conflicts,
the output must say that both sides changed or that the safe direction is
unknown.

`+` and `-` markers may appear in a readable diff body, but they are never
sufficient by themselves. A user must be able to understand direction without
knowing which side plus/minus represents.

When values are sensitive or opaque, the diff must explain the limitation:

```text
Values are hidden for safety. Metadata shows that the two sides differ.
```

## Ordering for deterministic output

Many-app output must be deterministic:

1. app/tool order follows the user's configured selection order when available;
2. settings within an app/tool follow recipe-declared order;
3. if either order is unavailable, sort by public ref lexicographically.

Golden fixtures must not depend on filesystem traversal order, map iteration
order, or nondeterministic diagnostics.

## Exit-code boundary

This issue does not freeze final shell exit codes. That is deferred to the CLI
contract work that implements these renderers.

However, future implementation must follow these constraints:

- read-only status/diff success with no command/runtime failure must be
  distinguishable from validation or internal errors;
- detected differences, conflicts, missing stored settings, and missing live
  settings are valid read-only results, not writes;
- parser/selection/configuration errors must not be hidden as a clean status;
- blocked safety/lifecycle conditions must not be hidden as clean status;
- any adopted exit-code mapping must be tested together with the text/JSON
  fixtures.

Existing v2 constants such as `ExitSuccess`, `ExitChanged`,
`ExitValidation`, `ExitSafetyBlocker`, and `ExitPartial` may be reused if the
future CLI contract accepts them.

## Normal status examples

The checked-in fixtures under `fixtures/status-diff-read-only/` are normative
examples for #221. They are not runtime snapshots yet; implementation issues must
turn them into executable golden tests before claiming the renderer is complete.

| Case | Fixture |
| --- | --- |
| Clean selected setting | [`clean.status.txt`](fixtures/status-diff-read-only/clean.status.txt) |
| Live-only change | [`live-only-change.status.txt`](fixtures/status-diff-read-only/live-only-change.status.txt) |
| Stored-only change | [`stored-only-change.status.txt`](fixtures/status-diff-read-only/stored-only-change.status.txt) |
| Conflict | [`conflict.status.txt`](fixtures/status-diff-read-only/conflict.status.txt) |
| Missing app/tool | [`missing-app.status.txt`](fixtures/status-diff-read-only/missing-app.status.txt) |
| Missing stored settings | [`missing-stored-settings.status.txt`](fixtures/status-diff-read-only/missing-stored-settings.status.txt) |
| Many-app aggregate | [`many-app.status.txt`](fixtures/status-diff-read-only/many-app.status.txt) |

## Normal diff examples

| Case | Fixture |
| --- | --- |
| Live-side change compared with stored settings | [`live-vs-stored.diff.txt`](fixtures/status-diff-read-only/live-vs-stored.diff.txt) |
| Stored-side change compared with live settings | [`stored-vs-live.diff.txt`](fixtures/status-diff-read-only/stored-vs-live.diff.txt) |
| Conflict diff | [`conflict.diff.txt`](fixtures/status-diff-read-only/conflict.diff.txt) |
| JSON status boundary example | [`json.status.json`](fixtures/status-diff-read-only/json.status.json) |

## Fixture rules

Future automated tests or golden snapshots for #221-compatible renderers must:

- compare the normal text fixtures exactly, after normalizing platform-specific
  path separators only when the implementation supports multiple platforms;
- assert that normal text fixtures contain no `desired://`, `driver`, `resource`,
  `backup`, `restore`, `migration`, `repo`, or `repository` wording;
- assert that every changed or conflict fixture contains an explicit `Direction:`
  line;
- assert that every status and diff fixture says `Read-only command: no files
  were changed.`;
- assert that hidden values stay hidden unless the recipe marks them safe;
- cover at least clean, live-only change, stored-only change, conflict, missing
  app/tool, missing stored settings, many-app status, and both diff directions.

The JSON fixture is a boundary example for the required concepts. It does not
freeze final JSON field names unless a later CLI/JSON contract adopts them.

## Non-goals

This spec does not define or implement:

- mutating sync execution;
- smart-sync planning;
- conflict-resolution prompts;
- final `sync` command syntax;
- `save`/`apply` alias or deprecation policy;
- Git status, commits, branches, remotes, or history behavior;
- backup/restore behavior or terminology;
- v1 migration behavior or terminology;
- final JSON schema details;
- shell exit-code finalization;
- production end-user documentation.

## Follow-up ownership

- #222 owns smart-sync planning and conflict UX.
- #223 owns mutating sync execution and confirmations.
- #224 owns partial and many-app UX fixtures beyond this read-only baseline.
- #225 owns `save`/`apply` alias or deprecation policy.
- #212 owns backup/restore removal or quarantine.
- #213/#226 own legacy v1 public-surface policy.
- #216 owns production end-user documentation after accepted behavior exists.
