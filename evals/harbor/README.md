# Harbor agent tests

This directory is reserved for a local Harbor agent-test harness for v2
spec-hardening work.

Purpose:

- turn selected v2 acceptance criteria into agent-facing cases;
- evaluate issue plans, process handoffs, documentation quality, UX simplicity,
  support-boundary reasoning, and safety/trust planning;
- complement deterministic tests.

Not purpose:

- no runtime `dotfiles-manager` behavior;
- no user-facing feature;
- no replacement for Go unit, integration, contract, fixture, snapshot, safety,
  or performance tests;
- no CI/cloud auth design;
- no broad app reverse-engineering test suite.

Planned structure:

```text
evals/harbor/
  README.md
  cases/        # future reviewed/sanitized shareable cases
  rubrics/      # future reviewed/sanitized scoring rubrics
  fixtures/     # future sanitized inputs, if needed
  jobs/         # generated per-case Harbor jobs; ignored
  all-jobs/     # generated aggregate jobs; ignored
  results/      # local run outputs; ignored
  tmp/          # local scratch; ignored
  local-auth/   # local-private auth quarantine if ever needed; ignored
  local-build/  # local Docker/build contexts/images; ignored
  local-output/ # generated local artifacts; ignored
```

Auth mode:

Harbor is local-private by default. This repository does not define shareable,
CI-safe, or cloud-safe auth. Any shareable/CI/cloud Harbor auth design requires
a separate design and review.

Copied Codex auth is not a shareable artifact and must not be committed. The
ignored `local-auth/` path is a defensive quarantine, not a recommended
workflow.

Commit policy:

Do not commit generated jobs, aggregate jobs, run results, copied auth, local
Docker build contexts, local images, or generated local outputs.
