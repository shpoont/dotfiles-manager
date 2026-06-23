---
owner: Product + Core Engineering
document-type: v2-active-behavior-spec
status: Active behavior spec
last-updated: 2026-06-23
canonical-source: docs/internal/specs/v2/16-save-apply-alias-policy.md
source-issue: 225
authority: Authoritative v2 command policy for save/apply directional aliases; sync behavior remains owned by 13-15 sync contracts.
---

# v2 save/apply alias policy

## Decision

`save` and `apply` remain public, callable compatibility aliases for directional
sync. They are not primary v2 product concepts and must not be taught as
separate beginner workflows.

Accepted meanings:

```text
save  = sync live settings -> stored settings
apply = sync stored settings -> live settings
```

`sync` remains the primary v2 command and mental model.

## Rationale

The owner feedback for the reset model was that save and apply are sync with a
direction. Removing them now would break current users, tests, and examples
without improving the core model enough to justify a breaking change. Keeping
them as primary commands would preserve the old repository/desired-state
workflow and contradict the reset.

The compromise is:

- existing explicit-direction use stays possible;
- help, docs, examples, and normal output make `sync` primary;
- retained aliases always say which direction they sync;
- no removal or hiding happens until a later issue records a versioned
  deprecation/removal plan.

## Public wording rules

Normal help and text output must use:

- compatibility alias;
- directional sync;
- live settings;
- stored settings;
- settings folder;
- primary command: sync.

Normal help and beginner docs must not describe:

- `save` as creating desired artifacts or writing a repository;
- `apply` as applying desired state as a separate product mode;
- `save`/`apply` as the primary happy path;
- Git, backups, restore, migration, drivers, resources, or internal URIs as the
  ordinary alias model.

Acceptable examples:

```text
save [ref]  sync live settings -> stored settings
apply [ref] sync stored settings -> live settings
```

## JSON compatibility

For v2 selected-setting `save`/`apply` JSON, preserve the command the user typed
while exposing the semantic v2 operation:

```json
{
  "command": "save",
  "operation": "sync",
  "invokedCommand": "save",
  "direction": "live_to_stored"
}
```

```json
{
  "command": "apply",
  "operation": "sync",
  "invokedCommand": "apply",
  "direction": "stored_to_live"
}
```

This keeps existing command compatibility while preventing new automation from
learning a separate save/apply product model.

## Deferrals

This policy does not:

- remove or hide `save`/`apply`;
- define a removal timeline;
- redesign selector syntax;
- add conflict-resolution choices;
- change many-app/partial sync behavior;
- settle backup/restore product removal;
- rewrite historical internal prototype documents.
