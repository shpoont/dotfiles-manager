# Task

Read `/app/context.md`. Produce `/app/solution.md` as end-user-facing
guidance for the v2 happy path.

The user wants to manage a supported application setting with minimal
work. Explain the normal flow before advanced concepts. Cover:

- `init`, `add`, `status`, `diff`, `save`, and `apply`;
- what preview/dry-run means;
- when desired artifacts are updated;
- when live state is touched;
- trust, ledger/last-applied state, native export/import,
  unsupported/risky settings, and secrets.

Do not implement code or run live app commands.
