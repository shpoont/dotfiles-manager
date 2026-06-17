# Security policy

`dotfiles-manager` can read and write local configuration files. Treat bug
reports and diagnostics as potentially sensitive because config files can contain
usernames, hostnames, paths, internal service names, and accidentally included
secrets.

## Supported versions

Security fixes are handled on the current `main` branch and the newest published
release or release candidate when a fix is relevant to a released artifact.
Older releases are best-effort only unless a maintainer explicitly says
otherwise in the issue or advisory.

## Reporting a vulnerability

Do **not** paste secrets, private keys, tokens, credentials, private config
payloads, exploit details, or sensitive logs into a public issue.

Preferred private route:

1. Open the repository's GitHub **Security** page.
2. If GitHub shows **Report a vulnerability**, use that private vulnerability
   report form.
3. If private vulnerability reporting is not available, open a minimal public
   issue asking for a private security contact path. Include only:
   - the affected command or feature;
   - the affected version or commit;
   - a short redacted summary;
   - confirmation that you have private details to share.

A public issue must not include proof-of-concept payloads, secrets, private file
contents, or unreduced logs.

## Safe diagnostics

When reporting non-security bugs, prefer:

- command name and flags, with private paths replaced by placeholders;
- `dotfiles-manager version` output;
- redacted `--json` output when useful;
- the smallest synthetic config or fixture that reproduces the issue.

Do not attach your full settings repository, backup payloads, SSH config, Git
config, application exports, or logs until a maintainer confirms what is safe to
share and where to share it.
