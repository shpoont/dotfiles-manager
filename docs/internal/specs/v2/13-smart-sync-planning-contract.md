---
owner: Product + Core Engineering
document-type: v2-active-behavior-spec
status: Active behavior spec
last-updated: 2026-06-23
canonical-source: docs/internal/specs/v2/13-smart-sync-planning-contract.md
source-issue: 222
authority: Authoritative v2 behavior contract for smart-sync planning and conflict UX only; write execution and confirmations remain out of scope.
---

# v2 smart-sync planning contract

## Purpose

This spec defines the active v2 contract for smart-sync planning. A sync plan is
a read-only explanation of what the manager can safely sync later, what direction
is recommended, what is skipped, and where the user must choose before any write
can happen.

The plan must be concrete enough for #223 to execute later without inventing a
separate write model, but #222 itself must not write anything.

## Authority and boundaries

This is an **Active behavior spec** only for planning.

A planning command may inspect live settings, stored settings, status results,
diff metadata, recipes, settings-folder metadata, and local evidence needed to
explain a safe plan. It must not create, update, delete, sync, initialize,
restore, back up, migrate, repair, normalize in place, or otherwise mutate app
files or the settings folder.

Planning output may present choices. It must not record a winning choice or treat
a choice as confirmed for execution. #223 owns accepted-plan execution,
confirmation, mutation, verification, and stale-plan handling.

`internal/v2/guidedsync` is reusable prototype implementation material only. It
is not public reset-v2 authority until reconciled with this spec because it uses
older public terms and mixes planning with execution behavior.

## Relationship to nearby specs and issues

- `12-status-diff-read-only-contract.md` owns read-only status and diff output.
- This spec owns planning decisions derived from those states.
- #223 owns mutating sync execution and confirmations.
- #224 owns broader partial and many-app UX beyond this required planning set.
- #225 owns `save`/`apply` alias or deprecation policy.
- #212 owns backup/restore removal or quarantine.
- #213/#226 own legacy v1 public-surface policy.

## Public vocabulary requirements

Normal planning output must use these public nouns:

- settings folder;
- live settings;
- stored settings;
- status;
- diff;
- sync;
- conflict;
- plan;
- public refs such as `git:user.email`.

Normal planning output must not require users to understand internal storage or
implementation terms. Internal identifiers may appear only in verbose, JSON,
debug, or authoring contexts.

`save` and `apply` are not public planning actions in #222 examples. Until #225
accepts an alias policy, planning output must describe directions with `sync`.

## Public direction and action names

### Directions

Normal text uses these direction labels exactly:

```text
live settings -> stored settings
stored settings -> live settings
both sides changed
no safe direction
not planned
```

JSON or internal planning data may use stable enum values:

```text
live_to_stored
stored_to_live
both_sides_changed
none
unknown
```

### Public action phrases

Normal text should use these phrases:

```text
Plan: sync live settings to stored settings.
Plan: sync stored settings to live settings.
Needs choice: choose live settings or stored settings before sync can continue.
Skipped: app not available.
Skipped: no stored settings yet.
Skipped: not managed.
Blocked: cannot plan safely.
```

The primary public action is always `sync`. Public examples must not introduce
`save` or `apply` as actions before #225 decides their policy.

## Conservative smart-sync rules

Smart sync is conservative. It may recommend a direction only when one side
changed and the other side still matches the previous safe baseline or equivalent
trusted evidence.

| Input state from status/diff | Planning decision | Direction | Rule |
| --- | --- | --- | --- |
| Up to date | skip | none | Nothing would change. |
| Changed in live settings | write | live_to_stored | Plan a later write to stored settings when policy allows it. |
| Changed in stored settings | write | stored_to_live | Plan a later write to live settings when policy allows it. |
| Conflict | needs_choice | both_sides_changed | Never pick a winner automatically. |
| App not available | skip | none | Do not plan a live write or read retry as part of sync. |
| No stored settings yet | needs_choice | live_to_stored, if live settings exist | Do not silently start storing a new setting; show the safe option and require user intent. |
| Missing in live settings | write or needs_choice | stored_to_live, if safe | May plan a later live write only when recipe and policy allow creating that live setting. |
| Not managed | skip | none | The setting is outside the support boundary. |
| Failed to inspect | blocked | unknown | Do not guess. |

A conflict, missing evidence, unsupported operation, failed inspection, or unsafe
policy must produce `needs_choice`, `skip`, or `blocked`; it must not produce a
planned write.

## Plan model for #223

A sync plan is a stable planning object. #223 must execute only an accepted plan
that matches this model; it must refuse to execute stale, invalid, conflicted,
blocked, failed, or choice-required items.

The model must include these concepts:

| Concept | Required meaning |
| --- | --- |
| `planVersion` | Version of the plan format used for executor compatibility. |
| `mode` | Must be `planning-only` for #222 outputs. |
| `selection` | Whether the plan is for all apps, one app, one setting, or a selected internal write unit. |
| `generatedFrom` | Status/diff contract version, recipe/catalog evidence, baseline evidence, and timestamp used to create the plan. |
| `apps[]` | Deterministic app/tool groups for normal and JSON output. |
| `items[]` | Exact planned units, one per setting or internal write unit. |
| `appRef` | Public app/tool ref. |
| `settingRef` | Public setting ref. |
| `label` | Human-readable setting label. |
| `writeUnitId` | Stable internal unit that #223 may execute; normal text may call this a write unit. |
| `liveAddress` | Stable description of where live settings would be read from or written to. |
| `storedAddress` | Stable description of where stored settings would be read from or written to. |
| `state` | One of the #221-aligned planning states. |
| `decision` | `write`, `skip`, `needs_choice`, or `blocked`. |
| `direction` | `live_to_stored`, `stored_to_live`, `both_sides_changed`, `none`, or `unknown`. |
| `wouldWrite` | Boolean; true only for items #223 may execute after acceptance. |
| `writeSource` | `live settings`, `stored settings`, or empty. |
| `writeTarget` | `stored settings`, `live settings`, or empty. |
| `reasonCode` | Stable short reason for tests and later executor behavior. |
| `reason` | Human-readable reason. |
| `choiceRequired` | True when the user must choose before any write can happen. |
| `allowedChoices` | Directions/options the user may choose later; no winner selected in #222. |
| `diffAvailable` | Whether a user can inspect a diff before accepting. |
| `valuesRedacted` | Whether values are hidden for safety. |
| `executableBySync` | True only when #223 can execute the item without replanning. |
| `summaryCounts` | Planned writes by direction, conflicts, skipped, blocked, missing, and failed-inspection diagnostics. Diagnostic counts such as failed inspections may be subcounts of blocked items rather than additive decision totals. |
| `noWritePerformed` | Must be true for every #222 plan. |

#223 must not reinterpret `needs_choice`, `skip`, or `blocked` as executable
writes. It may execute only `decision=write`, `wouldWrite=true`,
`executableBySync=true` items from an accepted, current plan.

## Normal text output requirements

Normal text planning output must include:

1. what was planned;
2. summary counts;
3. deterministic per-app sections;
4. exact public setting refs;
5. exact direction for every planned write;
6. skipped and blocked reasons;
7. conflict choice requirements;
8. value redaction notes when values exist but are hidden;
9. `Planning-only command: no files were changed.`

Normal public text may talk about an app setting. JSON, verbose, and internal
planning data must still include stable write-unit identity, such as
`writeUnitId`, when a setting maps to the exact executor target #223 will need.

Many-app output must separate planned writes from conflicts, skipped items,
missing apps, missing stored settings, and failed inspections.

## Deterministic ordering

Planning output must use deterministic ordering:

1. configured selection order when available;
2. recipe-declared setting order within an app/tool;
3. lexicographic public refs when no configured or recipe order exists.

JSON arrays and text sections must use the same deterministic ordering.

## Missing and conflict behavior

### Missing app/tool

A missing or unavailable app/tool is skipped. The plan must not imply the manager
will install, start, quit, repair, or configure the app/tool.

### Missing stored settings

A selected setting with live settings but no stored settings is not silently
written. The plan may show `live settings -> stored settings` as the safe option,
but the decision is `needs_choice` unless a later issue explicitly accepts an
auto-include policy. In normal text, this case must not say `Plan: sync` because
no automatic write has been selected.

### Conflict

A conflict means both sides changed or the safe direction is unknown. The plan
must show `needs_choice`, list allowed choices, and refuse to choose a winner.

Allowed choices may include:

- choose live settings later;
- choose stored settings later;
- skip.

The plan must not call any one choice recommended unless a later conflict UX
issue accepts a stronger policy.

## JSON/concept boundary

#222 includes a JSON concept fixture to freeze the plan model required by #223.
It is a contract for required concepts, enum meanings, and execution boundaries.
It does not freeze every final field name unless a later CLI/JSON contract adopts
it.

JSON may include internal write-unit IDs and addresses. Normal text must stay
human-first and must not require internal IDs.

## Fixtures

The checked-in fixtures under `fixtures/smart-sync-planning/` are normative
examples for #222. They are not runtime snapshots yet; #223 or later renderer
issues must turn them into executable golden tests before claiming implemented
behavior.

| Case | Fixture |
| --- | --- |
| Safe live-to-stored plan | [`live-to-stored.plan.txt`](fixtures/smart-sync-planning/live-to-stored.plan.txt) |
| Safe stored-to-live plan | [`stored-to-live.plan.txt`](fixtures/smart-sync-planning/stored-to-live.plan.txt) |
| Conflict requiring choice | [`conflict-needs-choice.plan.txt`](fixtures/smart-sync-planning/conflict-needs-choice.plan.txt) |
| Missing app/tool | [`missing-app.plan.txt`](fixtures/smart-sync-planning/missing-app.plan.txt) |
| Missing stored settings | [`missing-stored-settings.plan.txt`](fixtures/smart-sync-planning/missing-stored-settings.plan.txt) |
| Mixed many-app plan | [`mixed-many-app.plan.txt`](fixtures/smart-sync-planning/mixed-many-app.plan.txt) |
| JSON concept fixture | [`mixed-many-app.plan.json`](fixtures/smart-sync-planning/mixed-many-app.plan.json) |

## Fixture validation expectations

Future automated tests or golden snapshots for #222-compatible planning must:

- assert every normal text fixture says `Planning-only command: no files were changed.`;
- assert planned write fixtures include an explicit `Direction:` line;
- assert conflict fixtures include `Needs choice` and do not select a winner;
- assert missing-app and missing-stored-settings fixtures are not executable writes;
- assert normal public fixture text does not contain old or internal product terms;
- assert the JSON concept fixture parses and includes `noWritePerformed: true`;
- assert #223 can identify executable items by `decision`, `wouldWrite`,
  `direction`, and `executableBySync`.

## Non-goals

This spec does not define or implement:

- actual writes;
- accepted-plan execution;
- confirmation prompts;
- conflict-resolution prompts beyond planning copy;
- final CLI flags or command syntax;
- `save`/`apply` alias or deprecation policy;
- Git status, commits, branches, remotes, or history behavior;
- backup/restore behavior or terminology;
- v1 migration behavior or terminology;
- catalog/tap trust and write-authority rules;
- final shell exit-code mapping;
- production end-user documentation.
