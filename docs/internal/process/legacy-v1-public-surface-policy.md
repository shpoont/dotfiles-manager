---
owner: Project Owner + Work Manager
status: Active policy for v2 reset
last-updated: 2026-06-25
canonical-source: docs/internal/process/legacy-v1-public-surface-policy.md
related-issues: [213, 226]
---

# Legacy v1 public-surface policy

This policy records the #213/#226 decision for legacy v1 commands during the v2
reset.

## Decision

The v2 public product surface is:

```text
status -> diff -> sync
```

`save` and `apply` remain public compatibility aliases for explicit directional
sync:

- `save` = sync live settings to stored settings;
- `apply` = sync stored settings to live settings.

v1 migration is not part of the active v2 roadmap.

## Retained hidden compatibility surface

The legacy v1 commands remain implemented for now so existing users and scripts
are not broken by this scope-cleanup issue:

- `deploy`
- `import`
- `migrate`

They are hidden from normal root help and are not taught in the v2 happy path.
Direct command help must identify them as legacy v1 compatibility commands.

Keeping these commands callable does **not** mean v2 promises migration tooling,
v1 compatibility as product value, or a separate v1 maintenance track.

## User-facing documentation rule

Primary user-facing docs must explain the v2 local-settings-manager model:

- settings storage folder, not mandatory Git repository;
- live settings and stored settings;
- `status`, `diff`, and `sync`;
- `save` and `apply` only as explicit directional aliases;
- supported bundled recipes and current exclusions.

Primary user-facing docs must not teach users to start with v1 workflows, v1
config files, or v1 migration.

## Test and evidence classification

Existing legacy tests may remain as compatibility-maintenance coverage. They are
not v2 product acceptance evidence unless a future explicit maintenance-track
issue says so.

For v2 acceptance, evidence should come from v2 commands, v2 fixtures, v2 docs,
and the current settings-folder sync model.

## Future change rule

Deleting legacy implementations, adding runtime warnings, formally deprecating
commands, or creating a v1 maintenance track requires a new explicit issue with
its own owner decision, risk tier, validation, and acceptance evidence.

Do not silently remove or weaken legacy direct invocation as part of unrelated
v2 work.
