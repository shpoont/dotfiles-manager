# Verdict

Not ready. The draft must become a v2-only, spec-referenced issue
before implementation.

# Corrected issue outline

Scope: implement one v2 tranche from `docs/internal/specs/v2/02-cli-contract.md`
and `docs/internal/specs/v2/06-recipe-schema.md`, with exact files
listed. Out-of-scope: no broad app discovery, no native execution
unless the reviewed native contract is explicitly in scope.

# Acceptance criteria and definition of done

Acceptance criteria include text and JSON contract coverage,
deterministic fixture behavior, and a clear Definition of Done with
validation commands. The issue must state how failing cases map to
product/spec gaps, not to ad-hoc workarounds.

# Safety and v1/v2 boundaries

V1 remains preserved while v2 is built beside it. Safety, trust,
redaction, lifecycle, internal recovery evidence, ledger, and native-operation boundaries
must cite `docs/internal/specs/v2/09-security-redaction-trust.md` and
`docs/internal/specs/v2/11-mvp-acceptance-tests.md`.

# Deterministic tests complemented

Harbor complements deterministic Go test, contract test, schema
fixture, path safety, and redaction tests. Harbor is not the only
release gate.
