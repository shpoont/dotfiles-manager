# Internal UX artifacts

This directory contains pre-implementation UX artifacts for v2 command output.
These files are not runtime contracts by themselves; they define expected user
experience and review criteria that implementation issues convert into code,
tests, and command documentation.

- `v2-safe-quickstart-output-storyboard.md` — visual terminal storyboard for the
  safe temp-HOME Git email quickstart before #165 implementation.
- `v2-ux-coverage-map.md` — user-flow coverage map and storyboard
  backlog for production-ready v2 UX after #165/#169.
- `v2-transcript-review-gate.md` — reusable persona transcript review gate for
  v2 CLI output changes.
- `v2-aggregate-status-diff-storyboard.md` — high-fidelity terminal storyboard
  for aggregate selected `status` and `diff` output across multiple apps and
  mixed states.
- `v2-aggregate-save-apply-storyboard.md` — high-fidelity terminal storyboard
  for aggregate selected `save`/`apply` dry-run and confirmed-write
  confirmations across multiple apps and mixed states, plus the #181 final
  outcome semantics addendum.
- `reviews/` — checked-in completed transcript reviews, starting with the safe
  Git email quickstart review required by #168 and the aggregate status/diff
  review required by #177 plus the aggregate save/apply review required by
  #179 and final outcome semantics review required by #181.
