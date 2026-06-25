---
owner: Documentation Maintainer (TBD)
status: Reference
last-updated: 2026-06-25
canonical-source: docs/internal/README.md
---

# Internal documentation

This is the canonical internal documentation index for `dotfiles-manager`.

For agent/operator workflow, start at the repository root `AGENTS.md`. That file
references the external Project Execution Standard and defines the required
reading order for active work.

## 1) High-level scope

- `scope/product-scope.md` — product scope, goals, non-goals
- `scope/architecture.md` — architecture overview (current + planned)
- `scope/product-concept-v2.md` — proposed future product concept and architecture

## 2) Detailed specs

- `specs/cli-and-config-spec.md` — current v1 command/config behavior reference
- `specs/decisions.md` — current v1 canonical decisions and rationale
- `specs/decision-matrix.md` — current v1 scenario outcomes/test oracle
- `specs/open-questions.md` — current v1 remaining non-blocking follow-ups
- `specs/v2/README.md` — draft v2 formal specification package and promotion rules

## 3) Contracts

- `contracts/config-schema.json` — JSON Schema for `.dotfiles-manager.yaml`
- `contracts/json-contract.md` — `--json` output contract
- `contracts/metadata-contract.md` — metadata guarantees
- `contracts/validation-errors.md` — error catalog + validation order
- `contracts/logging-contract.md` — logging behavior and coverage contract
- `contracts/ci-artifacts-contract.md` — CI-produced artifact schemas for
  coverage/performance gates

## 4) Engineering docs

- `engineering/technical-requirements.md` — language/framework/testing/CI requirements
- `engineering/testing-strategy.md` — test strategy and suite structure
- `engineering/acceptance-checklist.md` — implementation readiness checklist
- `engineering/ci-cd.md` — CI/CD requirements and release policy
- `engineering/v2-mvp-release-candidate.md` — scoped v2 MVP
  release-candidate evidence and limitations

## 5) UX artifacts

- `ux/README.md` — internal UX artifact index
- `ux/v2-safe-quickstart-output-storyboard.md` — expected terminal UX for
  the safe v2 Git quickstart before #165 implementation
- `ux/v2-ux-coverage-map.md` — coverage map and storyboard backlog for
  production-ready v2 UX after #165/#169
- `ux/v2-transcript-review-gate.md` — persona transcript review gate for v2
  CLI output changes and checked-in UX review evidence
- `ux/v2-aggregate-status-diff-storyboard.md` — aggregate selected
  `status`/`diff` terminal UX storyboard for multi-app output
- `ux/v2-aggregate-save-apply-storyboard.md` — aggregate selected
  `save`/`apply` confirmation UX storyboard for multi-app writes, including
  final outcome semantics
- `ux/v2-repeated-add-multiple-apps-storyboard.md` — repeated `add <target>`
  UX storyboard for selecting several supported apps/settings without implying
  unsupported multi-target add syntax

## 6) Process

- `../../AGENTS.md` — root agent/operator entrypoint for applying the external
  Project Execution Standard in this repo
- `process/documentation-governance.md` — documentation ownership/change policy
- `process/v2-execution-standards-tailoring.md` — repo-specific tailoring for
  applying the Project Execution Standards to v2 reset work

## 7) Project execution records

- `project/v2-project-execution-standards-transformation-plan.md` — written
  plan for transforming v2 work to follow the Project Execution Standards
- `project/v2-reset-execution-record.md` — active repo-editable project record
  for the v2 reset

## Canonicality

For active v2 reset work, precedence is:

1. the external Project Execution Standard referenced by root `AGENTS.md`;
2. `process/v2-execution-standards-tailoring.md`;
3. GitHub issue #209 plus
   `project/v2-reset-execution-record.md`;
4. the active GitHub issue contract for the specific work item;
5. v2 specs, contracts, UX artifacts, tests, and fixtures that are not marked
   stale or superseded.

The older v1 decisions, contracts, decision matrix, and acceptance checklist are
historical or legacy-behavior references for v2 unless the v2 execution record
or active issue contract explicitly reaffirms them. If v1-era docs conflict with
the v2 reset direction, stop and reconcile the conflict through the active issue
contract before implementation.

For legacy v1 maintenance outside the v2 reset, use this narrower precedence
unless the active issue says otherwise:

1. `specs/decisions.md`
2. `contracts/*`
3. `specs/decision-matrix.md`
4. `engineering/acceptance-checklist.md`
5. other summaries/overviews
