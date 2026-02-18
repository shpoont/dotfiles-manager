# FAQ

## Do I need `--config` every time?

No. Config is resolved in this order:
1. `--config <path>`
2. `DOTFILES_MANAGER_CONFIG`
3. `./.dotfiles-manager.yaml` in the current working directory

If none of these is available, the command fails.

## What is the difference between `deploy` and `import`?

- `deploy`: source → target
- `import`: target → source

Use `status` first to preview both sides.

## What is “source of truth” here?

`source` is the manifest/source of truth. Command direction (`deploy` vs `import`) determines which side updates the other.

## Can I use environment variables in `source`/`target` paths?

Yes.

Supported syntax:
- `$VAR`
- `${VAR}`

Rules:
- expansion happens before path validation
- missing or empty env values are errors
- expanded paths must still be relative and must not escape base directories via `..`

## How does `[path]` filtering work?

A command-scoped `[path]` only selects syncs where that path is the sync target or inside it. Parent-of-target paths do not match.

## Can unmanaged files be removed on deploy?

Yes. Configure `on.deploy.remove-unmanaged` patterns.

## Will `target: ./` scan my whole home directory by default?

No.

By default, unmanaged/missing candidate lists are disabled (`include: []`), so commands evaluate manifest paths only.
When pattern rules are enabled, discovery starts from literal pattern roots when possible (for example `.codex/skills/**` starts from `.codex/skills`).
Wildcard-first patterns (for example `**/*.tmp`) can still require broad scans.

## Can import add files that are not in source yet?

Yes. Configure `on.import.add-unmanaged.include` (and optional exclude).

## Can import delete files from source?

Yes, but only for paths matching `on.import.remove-missing.include` and not matching `on.import.remove-missing.exclude`.

## Is there a safe preview mode for mutating commands?

Yes: `--dry-run` on `deploy` and `import`.

## Does `status` support `--dry-run`?

No. `status` is already preview-only.

## Is JSON output available?

Yes, use `--json` with `status`, `deploy`, or `import`.

## Where are logs written?

Logs are always written to a file.

Default paths:
- macOS: `~/Library/Logs/dotfiles-manager/dotfiles-manager.log`
- Linux: `${XDG_STATE_HOME:-~/.local/state}/dotfiles-manager/dotfiles-manager.log`

Use `--log-file <path>` to override the log location.

Warnings/errors are still emitted as human-readable diagnostics on stderr.
Log format is always human-readable text.

## Can I control log verbosity?

Yes. Use `--log-level <debug|info|warn|error>`.

- default log level is `info`
- applies to all commands

## How do I check CLI version?

Use either:

```bash
dotfiles-manager --version
# or
dotfiles-manager version
```

Both print `dotfiles-manager version <value>` and exit.
They do not require config.
Release builds print semantic version; local non-release builds print `dev`.
