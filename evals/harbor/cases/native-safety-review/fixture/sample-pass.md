# Verdict

Block this native proposal. Native support cannot be arbitrary command
execution.

# Required safety behavior

Use reviewed native metadata with argv-style typed tokens, not a
shell string. Do not inherit PATH, cwd, HOME, or caller environment;
use manager-owned temp copies and explicit input/output roots.
Secrets, tokens, credentials, account sessions, and opaque captured
data require exclusions, redaction, metadata-only diffability, and
explicit opt-in where allowed.

Lifecycle quit/reopen must be declared and checked. Import needs
internal pre-write recovery evidence before write and post-import export verification.
Trust must be bundled/reviewed or external trust evidence. Missing
trust, lifecycle, internal recovery-evidence, import, or verification support must fail
closed with a safety blocker such as exit 5. Harbor complements
deterministic tests for schema, command runner, internal recovery evidence, ledger, and
secret leakage behavior.
