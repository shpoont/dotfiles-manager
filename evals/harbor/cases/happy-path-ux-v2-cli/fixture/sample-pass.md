# Happy path

Start with `init`, then `add` the supported target. Run `status` and
`diff` to preview what the manager sees. Use `save --dry-run` before
`save` so you know which desired artifact will change. Use
`apply --dry-run` before `apply` so you know which live state is
touched.

# Safety cues

Desired data is the manager-owned copy of selected settings. Dry-run
is preview only. `save` updates desired data; `apply` writes live
files only after confirmation and safety checks. Trust controls which
recipes can write. Ledger/last-applied records explain what changed. Old recovery commands are not part of the normal v2 user workflow.

# What is not touched

Passwords, tokens, cookies, sessions, caches, opaque app databases,
and unsupported native export/import data are not synced by default.
Native operations require reviewed support and explicit confirmation.
