# Harbor case: issue quality from v2 specs

## Relevant v2 specs

- `docs/internal/specs/v2/README.md`
- `docs/internal/specs/v2/mvp-implementation-roadmap.md`
- `docs/internal/specs/v2/02-cli-contract.md`
- `docs/internal/specs/v2/06-recipe-schema.md`
- `docs/internal/specs/v2/09-security-redaction-trust.md`
- `docs/internal/specs/v2/11-mvp-acceptance-tests.md`

## User or reviewer risk evaluated

A weak agent-written issue can make implementation agents build from
vague concept prose, mix v1 and v2 responsibilities, omit safety
constraints, or ship without deterministic tests. This case checks
whether the reviewer pushes the issue back to an agent-sized,
spec-referenced, testable v2 contract.

## Passing answer includes

- a clear verdict on whether the issue is ready;
- corrected scope, out-of-scope, acceptance criteria, and definition
  of done;
- exact v2 spec references;
- explicit v1 preservation and v2-only boundaries;
- safety invariants for trust, redaction, lifecycle, native
  execution, backups, and ledgers when relevant;
- deterministic tests complemented by the Harbor judgment case;
- a mapping from failing Harbor cases back to product/spec gaps.

## Failing answer looks like

- accepts a vague issue without requiring specs or tests;
- asks the agent to replace or rewrite v1 behavior casually;
- treats Harbor as a replacement for Go/contract/fixture tests;
- omits out-of-scope boundaries or safety blockers;
- claims external tracker mutations were performed.

## Deterministic tests complemented

- Go unit and integration tests for implemented behavior;
- CLI text/JSON contract tests and snapshots;
- schema fixtures;
- redaction, path-safety, backup, ledger, and native-operation
  safety regression tests.

## Out of scope

- creating or editing GitHub issues during the Harbor run;
- implementing product behavior;
- deleting v1 behavior;
- committing generated Harbor jobs, transcripts, or dashboards.
