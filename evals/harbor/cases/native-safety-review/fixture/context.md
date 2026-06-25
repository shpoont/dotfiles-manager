# Context: proposed native support to review

A future bundled recipe wants to support a native export/import app.
The proposal says:

> Add a command string to the recipe, such as
> `/bin/sh -c "app export --all --out $HOME/export.zip"`, and let the
> app import that zip on apply. If the app is open, ask the user to
> close it. The export is opaque but users probably trust the app. If
> pre-write safety evidence cannot be recorded, continue and warn.

Important spec reminders:

- Native import/export is a constrained capability shape, not
  arbitrary scripting.
- Reviewed operations use fixed executables, argv-style typed
  tokens, empty/non-inherited environments, and manager-owned temp
  roots.
- `recipe explain` summarizes native support without raw argv/env or
  captured output.
- `status` does not run native export in this tranche.
- `apply --dry-run` for native apply validates the desired artifact
  and plan; it does not run export/import/verify commands by
  default or create public recovery workflows.
- `apply --yes` may run native apply only for trusted resources with
  explicit internal pre-apply recovery export and post-import export-hash
  verification policies.
- Secret, account, opaque, lifecycle, internal recovery-evidence, import, and
  verification failures fail closed as safety blockers.
