---
owner: Core Engineering
document-type: v2-draft-spec
status: Draft
last-updated: 2026-06-11
canonical-source: docs/internal/specs/v2/11-mvp-acceptance-tests.md
source-concept-sections:
  - MVP acceptance test matrix
  - Suggested MVP
  - Roadmap decomposition
  - Likely failure modes
authority: Draft; non-authoritative until promoted by docs/internal/specs/v2/README.md
---

# v2 MVP acceptance tests

## Purpose

This spec defines the cross-spec release gate for v2 MVP implementation. It is
not a replacement for per-spec tests. Each spec still owns local acceptance
expectations.

The MVP implementation should not start broad feature work without fixtures that
prove the core model is deterministic, safe, and compatible with v1 dotfile
behavior.

## Status and authority

This is a draft extraction from `../../scope/product-concept-v2.md`. It is not
implementation-authoritative until promoted through `README.md`.

Current v1 behavior remains governed by existing v1 specs and contracts.

## Source map and extraction notes

Extracted from the concept sections covering:

- MVP acceptance matrix;
- suggested MVP;
- roadmap phases;
- failure modes and mitigations;
- v1 parity requirements.

Deliberate non-decisions:

- exact test framework is deferred;
- exact CI job names are deferred;
- final release checklist is deferred.

## Terms owned by this spec

- acceptance fixture;
- release gate;
- parity gate;
- snapshot test;
- safety regression;
- dogfood gate.

## Normative MVP rules

### Cross-spec gate

A v2 MVP release must not be called production-ready unless these areas pass:

| Area | Required fixtures/tests |
| --- | --- |
| CLI | Text and `--json` snapshots for every normal command and exit code. |
| Profile/scope | All four scopes, multi-user machine, multiple profile layers. |
| Identity bootstrap | Machine/user bootstrap, local-account mapping, adoption, rename preview, collision handling, non-interactive missing-identity failures. |
| Status | Every canonical state and target-level aggregation. |
| File driver | Create/update/delete preview, backup, restore, symlink rejection. |
| File-tree driver | Globs, case conflicts, unsafe traversal, metadata policy. |
| Structured drivers | INI/JSON/YAML/TOML selectors, invalid selectors, normalization. |
| Plist/defaults | Selected keys only, read-only defaults, unsupported write attempts. |
| Platform/filesystem | macOS roots, Linux XDG roots, unsupported Windows/unknown OS, unsupported driver gating, case-conflict and permission behavior. |
| Redaction | Display redaction, blocked save, unavailable redaction, safe values. |
| Lifecycle | Running app allowed/warn/blocked, explicit lifecycle target validation, non-interactive and `--yes` manual prompt blocks, bounded/hardened detection, native import-operation lifecycle enforcement, apply-only lifecycle actions, truthful dry-run records, quit declined/failed before backup/write, still-running recheck, reopen failure after verified write. |
| Native export | Diffable export, opaque export, passphrase prompt, size/category limits. |
| Ledger | Last-applied update only after verified success, partial failure recording. |
| Restore | Restore preview, backup-before-restore, unsupported restore path. |
| Migration | v1 `syncs:` parity and generated `custom.files` target. |
| Trust | Untrusted local recipe, recipe broadening write scope, command-IO gate. |

### Identity and platform fixture details

Identity fixtures must prove:

- `init` creates machine and user identity records with repo-visible ID prompts;
- existing identity records reload without prompting;
- non-interactive commands that require missing identity exit with input-required
  exit code `4`;
- `--machine-id` and `--user-id` validate, satisfy non-interactive bootstrap
  when persistence is allowed, and fail if they conflict with existing local
  identity records;
- read-only and dry-run commands do not create identity records;
- hostname or computer-name changes do not automatically rename `machineId`;
- local OS account rename does not automatically rename `userId`;
- machine adoption links local state to an existing repository machine subject
  without moving repository paths;
- user adoption links a local account mapping to an existing repository user
  subject without moving repository paths;
- machine rename previews affected `desired/machine/<old>/...` and
  `desired/machine-user/<old>/<user-id>/...` paths and blocks on destination
  overwrite or case conflict;
- user rename previews affected `desired/user/<old>/...` and
  `desired/machine-user/<machine-id>/<old>/...` paths and blocks on destination
  overwrite or case conflict;
- a machine with two local users resolves distinct `machine-user` subjects;
- one logical user can be adopted on two machines for user-scoped desired
  artifacts;
- two local accounts on one machine can map to the same logical user only after
  explicit warning/confirmation;
- one user can use multiple profile layers on one machine with deterministic
  merge order.

Platform fixtures must prove:

- macOS local state, cache, and temp roots match `01-repository-layout.md`;
- Linux local state, cache, and temp roots honor XDG defaults from
  `01-repository-layout.md`;
- Windows and unknown OS targets fail before live reads or writes;
- unsupported platform-specific drivers are reported as unsupported/blocked
  metadata;
- mutating commands fail before live reads/writes for unsupported
  OS/driver/target combinations;
- `recipe explain` can safely describe unsupported platform metadata without
  bootstrapping identity or reading live state;
- path traversal, unsafe symlink traversal, case-conflict, and unsupported
  executable-bit/permission changes are rejected or blocked as specified by
  `09-security-redaction-trust.md`.

### Harbor agent-test mapping

Harbor is a local agent-test harness for selected agent-facing acceptance
scenarios. It complements deterministic tests; it does not replace unit,
integration, contract, fixture, snapshot, safety, or performance tests.

Use Harbor for acceptance criteria where the expected result requires judgment
about process, UX clarity, documentation quality, or agent handoff quality.

Good Harbor candidates:

| Area | Harbor use |
| --- | --- |
| Agent implementation gate | Evaluate whether a generated issue or implementation plan cites exact specs, adds tests first, states out-of-scope boundaries, and preserves v1 behavior. |
| Simple user model | Evaluate whether a proposed v2 issue/design keeps the happy path simple: manage app/config targets, save/diff/import/apply safely, with advanced custom apps optional. |
| Recipe/support explanation | Evaluate whether `recipe explain` or recipe docs make supported settings, unsupported/risky settings, lifecycle behavior, and redaction boundaries clear. |
| Safety/trust planning | Evaluate whether a proposed driver/recipe plan excludes credentials by default, respects trust boundaries, and avoids unsafe writes. |
| Documentation/process handoff | Evaluate whether docs updates preserve canonicality, avoid user-facing internal tooling leakage, and give reviewers enough acceptance criteria. |
| Native export/import review | Evaluate whether opaque/native export support is described with proper opt-in, size/category limits, passphrase handling, and diffability expectations. |

Do not use Harbor as the sole test for deterministic behavior:

| Area | Required normal tests |
| --- | --- |
| CLI commands, flags, prompts, exit codes | Go unit/integration/contract tests and snapshots. |
| JSON schema/envelope behavior | Contract tests and schema fixtures. |
| Path safety, symlink rejection, traversal rejection | Unit/integration safety fixtures. |
| File/file-tree/structured driver diff/apply/restore behavior | Deterministic filesystem fixtures. |
| Ledger, backup, restore, partial failure recording | Integration and contract fixtures. |
| Redaction/secret leakage in output, JSON, logs, artifacts, ledgers, or backups | Deterministic safety regression tests. |
| Performance thresholds | Normal performance regression tests. |

Every Harbor case must identify:

- the relevant v2 spec section;
- the user or reviewer risk being evaluated;
- the rubric or pass/fail expectations;
- the deterministic tests it complements;
- the out-of-scope behavior it must not ask the agent to implement.

Initial local-private suite:

| Case | Release-readiness question |
| --- | --- |
| `evals/harbor/cases/issue-quality-v2-specs` | Are agent-authored implementation issues scoped, spec-referenced, acceptance-testable, v2-only, and safe? |
| `evals/harbor/cases/happy-path-ux-v2-cli` | Is the normal user flow convenient and understandable without hiding preview, desired-data, trust, backup, ledger, live-state, or native boundaries? |
| `evals/harbor/cases/recipe-explain-clarity` | Does support explanation make scopes, named locations, managed/unmanaged settings, optional groups, lifecycle, redaction, and native summaries clear without live reads? |
| `evals/harbor/cases/native-safety-review` | Does native export/import review fail closed around arbitrary commands, secrets/account data, opaque diffs, lifecycle, trust, backup, verification, and opt-in? |

The concrete suite remains local-private. It must not introduce a runtime CLI
dependency, CI/cloud auth dependency, committed generated Harbor results, or
Docker images/build contexts that contain copied Codex auth. Local verifier
auth may be mounted only as a read-only runtime source and copied inside the
container to a temporary writable verifier `CODEX_HOME` that is removed on
exit, because Codex writes helper state while initializing. The Harbor
validator must fail when copied auth or generated Harbor result artifacts are
present under `evals/harbor/`, so those files cannot accidentally become part
of a review or commit.
Verifier `environment/codex-auth/config.toml` is generated local state, not a
committed source file; committed examples may document non-secret defaults, but
real auth/config generated for a run must be cleaned before validation.

### First vertical slice gate

Before broad app support, one vertical slice must pass:

```text
init
add custom.files
list
status
diff
save --dry-run
apply --dry-run
save
apply
backup list
restore
```

Scope:

- file driver;
- file-tree driver;
- profile stack;
- desired artifacts;
- local ledger;
- backup before apply;
- v1 migration fixture.

### Production-readiness gate

Production readiness requires:

- v1 parity for current dotfile use cases;
- no known secret leakage path in output, JSON, artifacts, ledgers, or backups;
- dry-run previews are trustworthy;
- live apply creates backups where supported;
- restore works for supported drivers;
- unsupported/unsafe cases fail clearly;
- CI passes on clean checkout;
- dogfood run succeeds on a non-critical profile/machine.

### Agent implementation gate

AI agents should receive issues that reference:

- the relevant v2 spec files;
- exact acceptance tests;
- out-of-scope boundaries;
- safety constraints;
- expected files to modify.

Agents should not implement directly from the full concept document once a spec
exists for the area.

## Derived schema boundaries, not final schemas

This spec owns test matrix and release-gate metadata only.

Persisted/emitted objects:

| Object | Owned here? | Notes |
| --- | --- | --- |
| Acceptance matrix | yes | Documentation/test planning. |
| CI result format | no | Existing engineering docs own current CI. |
| Fixture schemas | partial | Test implementation detail. |
| Parity report | partial | Migration spec also references it. |

## Examples

Examples demonstrate acceptance-test intent and issue shape. They do not define
final schema fields unless a referenced owning spec has promoted them.

### Issue acceptance block

```text
Acceptance:
- fixture covers shared/user/machine/machine-user scope
- --json snapshot includes command, profileStack, summary, items, ledgerRef
- unsafe path fixture fails with exit code 2 or 5 as specified
- no live writes occur under --dry-run
```

### Dogfood gate

```text
Dogfood profile: v2-sandbox
Targets: custom.files, git read/write, nvim file-tree
Required: dry-run reviewed, apply backed up, restore tested
```

## Errors, blockers, and partial-result behavior

A release gate fails if:

- a safety fixture leaks a secret;
- dry-run mutates live or desired state;
- apply writes without preview;
- ledger records unverified success;
- v1 parity fixture fails;
- unsupported state is silently treated as managed;
- conflict detection is absent or bolted on after writes.

## Acceptance expectations

This file is satisfied when:

- every row in the matrix maps to automated or explicitly manual tests;
- every implementation issue references relevant tests;
- CI can run the core MVP fixture suite;
- production readiness cannot be claimed without the gate passing;
- identity bootstrap, adoption, rename-preview, collision, and local-account
  mapping behavior has deterministic fixtures;
- platform support and unsupported-platform behavior has deterministic fixtures;
- judgment-heavy v2 acceptance criteria are either mapped to Harbor candidates
  or explicitly kept as manual review;
- deterministic product behavior remains covered by normal automated tests and
  is never accepted solely because a Harbor case passed.

## Out of scope

- final CI implementation;
- exact test framework;
- remote catalog tests;
- AI discovery tests;
- broad app reverse-engineering tests;
- running Harbor cases;
- defining CI/cloud Harbor execution;
- copying or normalizing Codex auth outside the local-private verifier runtime
  setup described above;
- treating Harbor results as production release gates before a separate
  CI/cloud-safe design exists.

## Spec follow-ups / open decisions

- Decide exact test framework and fixture directory layout.
- Decide which gates are automated vs manual in MVP.
- Decide dogfood profile and machine strategy.
- Decide release labels/statuses for GitHub issues.
