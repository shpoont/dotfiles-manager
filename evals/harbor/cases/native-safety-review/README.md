# Harbor case: native export/import safety review

## Relevant v2 specs

- `docs/internal/specs/v2/02-cli-contract.md`
- `docs/internal/specs/v2/04-status-conflict-state-machine.md`
- `docs/internal/specs/v2/06-recipe-schema.md`
- `docs/internal/specs/v2/07-driver-interface.md`
- `docs/internal/specs/v2/08-mutation-ledger-backup-restore.md`
- `docs/internal/specs/v2/09-security-redaction-trust.md`
- `docs/internal/specs/v2/11-mvp-acceptance-tests.md`

## User or reviewer risk evaluated

Native app export/import can leak secrets, capture account-bound
state, hide opaque diffs, corrupt live app data, or become arbitrary
command execution. This case checks whether an agent reviewer rejects
unsafe native proposals and requires a closed, reviewed,
explainable, fail-closed design.

## Passing answer includes

- no arbitrary shell/script execution and no inherited `PATH`, cwd,
  or environment;
- fixed reviewed executable/argv-style typed tokens only;
- secret/account/captured-category exclusions and redaction limits;
- opaque or metadata-only diff handling and explicit opt-in;
- lifecycle detection/quit/reopen boundaries;
- trust and reviewed/bundled operation requirements;
- backup-before-import and post-import verification policy;
- manager-owned temp/input/output roots;
- exit `5` or equivalent safety blocker/fail-closed behavior for
  unsupported, untrusted, secret, lifecycle, backup, import, or
  verification failures.

## Failing answer looks like

- allows `bash -c`, script strings, inherited environment, or `PATH`
  lookup;
- imports without backup or treats backup as best effort;
- captures passwords, tokens, account sessions, cookies, or opaque
  app databases without explicit blocker/opt-in policy;
- runs native operations merely because a recipe declares them;
- treats dry-run native apply as a real import/export run.

## Deterministic tests complemented

- recipe schema validation for native operation fields;
- CLI contract tests for dry-run, confirmation-required exit `4`,
  and safety exit `5`;
- lifecycle blocker fixtures;
- backup/ledger/restore integration tests;
- redaction/secret leakage regression tests;
- command-runner unit tests for env, cwd, argv, IO roots, and blocked
  executables.

## Out of scope

- implementing a native runner;
- verifying current behavior of any real app;
- committing native export payloads, raw transcripts, or local app
  data;
- defining CI/cloud auth for Harbor.
