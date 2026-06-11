# Harbor case: recipe explain clarity

## Relevant v2 specs

- `docs/internal/specs/v2/00-vocabulary.md`
- `docs/internal/specs/v2/02-cli-contract.md`
- `docs/internal/specs/v2/03-profile-and-scope-resolution.md`
- `docs/internal/specs/v2/05-desired-artifacts-and-uris.md`
- `docs/internal/specs/v2/06-recipe-schema.md`
- `docs/internal/specs/v2/07-driver-interface.md`
- `docs/internal/specs/v2/09-security-redaction-trust.md`

## User or reviewer risk evaluated

Recipe explanations can either make support discoverable or expose
confusing internal schema details. This case checks whether the
answer explains what is managed, what is unmanaged, where values live
through named locations, how scopes work, and why settings groups are
optional helper metadata rather than a mandatory public workflow.

## Passing answer includes

- `recipe explain` as a read-only metadata command;
- all four scopes: `shared`, `user`, `machine`, and `machine-user`;
- named locations and profile-layer overrides;
- managed vs unmanaged/unsupported settings;
- optional settings groups that may be absent;
- lifecycle and redaction/sensitivity summaries;
- native export/import summaries without raw argv, env, captures, or
  command execution;
- profile selection as best-effort and non-mutating.

## Failing answer looks like

- says settings groups are required first-class user nouns;
- reads live values or desired payloads to explain support;
- runs native commands from `recipe explain`;
- prints value-bearing defaults, raw argv, environment variables, or
  captured output;
- hides unsupported/risky settings.

## Deterministic tests complemented

- recipe schema fixtures;
- `recipe explain` text/JSON snapshots;
- no-mutation tests for read-only commands;
- redaction and metadata-render-blocked safety tests;
- profile/scope resolution fixtures.

## Out of scope

- final UI copy for every bundled recipe;
- live filesystem or app inspection;
- trust prompts or writes;
- recipe catalog download/update policy.
