---
owner: Product + Core Engineering
document-type: v2-active-behavior-spec
status: Active behavior spec
last-updated: 2026-06-23
canonical-source: docs/internal/specs/v2/14-smart-sync-execution-contract.md
source-issue: 223
authority: Authoritative v2 behavior contract for mutating smart-sync execution and confirmation UX only; planning remains owned by 13-smart-sync-planning-contract.md, broad end-user UX by issue 224, and directional alias policy by issue 225.
---

# v2 smart-sync execution and confirmation contract

## Purpose

`dotfiles-manager sync` is the normal mutating command that copies accepted
one-sided changes between **live settings** and **stored settings**. It executes
only the safe writes that the current smart-sync plan can prove are ready now.

This document is implementation-authoritative for the mutating execution phase
only. It does not define status derivation, diff rendering, conflict resolution,
or the long-form end-user documentation model.

## Boundaries

In scope for issue #223:

- execute current, accepted smart-sync plan items in either safe direction;
- require clear confirmation before any mutation unless the caller explicitly
  accepts the non-interactive contract;
- produce public text and JSON reports that state changed, skipped, failed, and
  not-attempted counts;
- preserve the read-only preview/diff evidence that explains why each planned
  write was considered safe;
- refuse unsafe, stale, unresolved, blocked, or ambiguous work without partial
  best-effort writes.

Out of scope for this contract:

- conflict resolution choices;
- first-time inclusion of settings that have no stored settings yet;
- backup or restore as a product feature;
- v1 migration;
- Git-specific workflow or assumptions;
- remote catalog trust and write authority;
- broad many-app onboarding UX;
- deciding whether `save` or `apply` remain public aliases.

## Core nouns

- **Live settings** are the current settings used by the app on this computer.
- **Stored settings** are the managed copy in the user's settings folder.
- **Settings folder** is the storage location. It may be versioned with Git, but
  Git is optional and must not be assumed in user-facing execution output.
- **Current plan** is a plan generated from the current inspected state, using
  the smart-sync planning contract in `13-smart-sync-planning-contract.md`.
- **Execution item** is a current plan item considered for mutation by `sync`.

## Executable item rules

A plan item is executable by `sync` only when all of these are true at execution
start:

1. `decision` is `write`;
2. `wouldWrite` is `true`;
3. `executableBySync` is `true`;
4. `choiceRequired` is `false`;
5. `direction` is exactly `live_to_stored` or `stored_to_live`;
6. the item still matches the evidence used to build the current plan;
7. the item is not blocked by app availability, lifecycle, safety, or inspection
   failure;
8. the item has public identifiers sufficient for a human to recognize the app
   and setting before confirming.

Anything else is skipped, refused, or blocked. It must not be silently upgraded
into a write.

## Direction semantics

`live_to_stored` copies the inspected live setting into stored settings.

`stored_to_live` copies the inspected stored setting into live settings.

Implementation may use lower-level directional primitives internally, but public
`sync` help, prompts, normal text output, and stable JSON must describe the work
as syncing between live settings and stored settings.

## Confirmation model

Interactive default:

- if no executable writes exist, do not prompt;
- if executable writes exist, prompt once for the whole accepted write set;
- the prompt must summarize how many settings will change and the direction of
  each write;
- the default answer is no;
- answering no exits without mutation and reports that confirmation was refused.

Non-interactive modes:

- `--yes` means the caller confirms the current executable write set;
- `--non-interactive` without `--yes` refuses if any write would happen;
- `--non-interactive` may still report no-op, skipped, refused, or blocked work;
- there are no per-conflict, per-setting, or inline choice prompts in #223.

## Safety ordering

Execution must validate the whole write set before the first mutation. If any
planned write is stale, no longer matches the evidence, or cannot be executed
safely, no writes are attempted.

After validation succeeds, writes may run sequentially. If one write fails during
mutation, later writes are not attempted unless a later contract explicitly
allows independent continuation. The final report must distinguish:

- changed: mutations completed successfully;
- skipped: items that were not executable by design;
- failed: attempted mutations that failed;
- not attempted: executable writes left untouched because an earlier mutation
  failed.

## Stale-plan refusal

`sync` must not execute an old accepted plan blindly. Before mutation, it must
re-check that the current inspected state still matches the evidence from the
plan that is about to be executed.

If the evidence does not match, `sync` refuses with no writes and tells the user
to check status again. It must not guess which side changed.

## Public output contract

Normal text output must be understandable before reading any specification. It
must include:

- whether sync was accepted, refused, completed, or stopped;
- the app and setting for every executable write shown in the confirmation;
- direction in plain words: `live settings -> stored settings` or
  `stored settings -> live settings`;
- counts for `Changed`, `Skipped`, and `Failed`;
- when relevant, `Not attempted`;
- values hidden by default.

Normal text output must not require users to understand internal storage URIs,
legacy command names, or implementation components.

Example confirmation shape:

```text
Sync plan accepted.
Will sync 2 settings:
- git:user.email: live settings -> stored settings
- starship:config: stored settings -> live settings
Proceed with sync? [y/N]
```

Example completion shape:

```text
Sync complete.
Changed: 2
Skipped: 1
Failed: 0
```

## JSON output contract

Stable JSON output must expose enough structure for scripts without promoting
internal implementation details into product concepts.

Required fields:

- schema name and version;
- selection summary;
- confirmation mode and decision;
- counts for changed, skipped, failed, and not attempted;
- item list with app reference, setting reference, state, decision, direction,
  result, reason code, and redaction flag;
- evidence identifiers or hashes sufficient to diagnose stale-plan refusals.

Stable JSON must not include raw secret values. It also must not expose internal
safety artifacts as user-facing product capabilities.

## Exit behavior

Recommended exit categories for #223 implementation:

- success: no failed attempted writes and no confirmation refusal;
- confirmation required/refused: writes existed but were not confirmed;
- refused unsafe plan: unresolved choices, stale evidence, blocked writes, or
  invalid plan shape prevented mutation;
- partial execution failure: at least one mutation failed after validation.

The exact numeric exit codes may follow the existing preview exit-code package,
but public messages must use the reset-v2 vocabulary in this document.

## Vocabulary floor

Public #223 output and help must use:

- settings folder, not repository terminology;
- live settings and stored settings, not legacy desired/current-only wording;
- sync direction, not legacy directional command names in normal sync output;
- app, setting, profile, and scope where relevant.

Public #223 output and help must not present backup/restore, v1 migration, or
Git as the normal write path.
