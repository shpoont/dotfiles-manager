# Context: desired user explanation

The product should feel like: tell the manager which supported app or
config target to manage; review what it sees; save selected current
settings to the external desired folder; apply desired settings later
to this or another machine/user when safe.

Important spec reminders:

- `init` bootstraps/connects local manager state and identities.
- `add <target>` selects supported settings with safe defaults.
- `status` and `diff` are read/preview commands.
- `save` imports current selected values into desired artifacts.
- `apply` writes desired artifacts back to live state only after
  preview/confirmation and safety checks.
- Dry-run must not change desired artifacts or live state.
- Live writes use preview, explicit confirmation, verification, and ledger/last-applied evidence; old recovery commands are not part of the happy path.
- Native export/import is constrained, reviewed, and not implied by
  every recipe.
- Secrets, account sessions, cookies, opaque app databases, caches,
  runtime state, and unsupported settings are not managed by default.
