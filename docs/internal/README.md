---
owner: Documentation Maintainer (TBD)
status: Reference
last-updated: 2026-06-04
canonical-source: docs/internal/README.md
---

# Internal documentation

This is the canonical internal documentation for `dotfiles-manager` implementation.

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

## 6) Process

- `process/documentation-governance.md` — documentation ownership/change policy

## Canonicality

If documents disagree, precedence is:

1. `specs/decisions.md`
2. `contracts/*`
3. `specs/decision-matrix.md`
4. `engineering/acceptance-checklist.md`
5. other summaries/overviews

The v2 package under `specs/v2/` is the implementation-prep and audit source for
the scoped v2 local-settings-manager release candidate. Current v1 file-sync
behavior remains governed by the v1 decisions/contracts above where the legacy
commands are still supported.
