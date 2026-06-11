# Context: proposed v2 issue draft to review

Product goal: v2 is a local settings manager that helps users manage
supported application settings safely. Users should be able to ask the
manager to handle a target, inspect/diff/save/apply the selected
settings, and optionally add custom local recipes later.

Canonical draft specs to cite when relevant:

- docs/internal/specs/v2/README.md
- docs/internal/specs/v2/mvp-implementation-roadmap.md
- docs/internal/specs/v2/02-cli-contract.md
- docs/internal/specs/v2/06-recipe-schema.md
- docs/internal/specs/v2/09-security-redaction-trust.md
- docs/internal/specs/v2/11-mvp-acceptance-tests.md

Proposed issue draft:

> Add app config support. Let the agent find app config files, export
> them, import them, and update the GitHub project. Harbor can judge
> if it works. It is okay to change v1 because v2 replaces it. The
> CLI should be nice. Acceptance: it works for common apps.

Review question: Is this ready for an implementation agent? If not,
rewrite the issue so a future agent can implement the right tranche
without unsafe assumptions.
