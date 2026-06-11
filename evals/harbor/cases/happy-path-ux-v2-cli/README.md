# Harbor case: happy-path UX for the v2 CLI

## Relevant v2 specs

- `docs/internal/scope/product-concept-v2.md`
- `docs/internal/specs/v2/00-vocabulary.md`
- `docs/internal/specs/v2/01-repository-layout.md`
- `docs/internal/specs/v2/02-cli-contract.md`
- `docs/internal/specs/v2/03-profile-and-scope-resolution.md`
- `docs/internal/specs/v2/05-desired-artifacts-and-uris.md`
- `docs/internal/specs/v2/08-mutation-ledger-backup-restore.md`
- `docs/internal/specs/v2/09-security-redaction-trust.md`

## User or reviewer risk evaluated

V2 can become too complicated if agents expose internal nouns before
the user understands the normal path. This case checks whether the
answer explains the convenient path first while still telling the
truth about previews, desired data, trust, backups, ledgers, native
boundaries, and what is not touched automatically.

## Passing answer includes

- a concise normal flow using `init`, `add`, `status`, `diff`,
  `save`, and `apply`;
- clear wording for preview/dry-run and confirmation points;
- explanation of desired artifacts without leaking internal layout
  first;
- safety boundaries for live state, trust, backups, ledgers, native
  export/import, unsupported settings, and secrets;
- a next-step/user-facing tone rather than implementation jargon.

## Failing answer looks like

- starts with schema/profile internals instead of the user task;
- implies `apply --yes` or native import can happen automatically;
- says passwords, tokens, cookies, or sessions are synced by default;
- omits backup/ledger/trust or live-state boundaries;
- treats dry-run as a mutation.

## Deterministic tests complemented

- CLI snapshots for `init`, `add`, `status`, `diff`, `save`, and
  `apply`;
- JSON envelope/exit-code contract tests;
- dry-run no-mutation tests;
- backup and ledger integration tests;
- secret/redaction regression tests.

## Out of scope

- building CLI behavior;
- executing real app operations;
- broad UX copy finalization;
- committing Harbor run outputs.
