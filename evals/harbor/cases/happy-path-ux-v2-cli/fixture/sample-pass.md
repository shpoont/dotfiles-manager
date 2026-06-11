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
recipes can write. Backups and the ledger/last-applied record explain
what changed and support restore paths where available.

# What is not touched

Passwords, tokens, cookies, sessions, caches, opaque app databases,
and unsupported native export/import data are not synced by default.
Native operations require reviewed support and explicit confirmation.
