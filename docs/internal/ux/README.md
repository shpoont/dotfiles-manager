# Internal UX artifacts

This directory contains pre-implementation UX artifacts for v2 command output. Public backup/restore storyboards created before #212 are historical only, not active product guidance.
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
- `v2-repeated-add-multiple-apps-storyboard.md` — high-fidelity terminal
  storyboard for selecting several supported apps/settings through the current
  repeated `add <target>` flow, without implying unsupported multi-target add
  syntax.
- `v2-catalog-discovery-storyboard.md` — recontracted pre-implementation
  terminal storyboard for #228 official-catalog app discovery from the bundled
  snapshot using the flattened normal-user `list`, `search`, and `explain`
  surface. It removes internal pseudo-app targets and catalog lifecycle commands
  from the normal #228 path and reserves catalog updates/additional remote
  catalogs for #229.
- `v2-cli-discovery-normalization-storyboard.md` — pre-implementation
  storyboard for the #252 focused discovery slice that changes normal `list`,
  adds `search` and top-level `explain`, and preserves selected-settings list
  behavior behind an explicit compatibility path.
- `v2-restore-preview-confirm-storyboard.md` — historical pre-#212 terminal
  storyboard for prototype restore UX. It is retained only as background and
  must not be used to implement active public v2 backup/restore behavior.
- `reviews/` — checked-in completed transcript reviews, starting with the safe
  Git email quickstart review required by #168 and the aggregate status/diff
  review required by #177 plus the aggregate save/apply review required by
  #179, final outcome semantics review required by #181, repeated add
  multi-app review required by #183, and historical restore preview/confirm review
  required by #187 before #212 superseded that public workflow.
