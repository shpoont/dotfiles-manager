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

## How does `[path]` filtering work?

A command-scoped `[path]` only selects syncs where that path is the sync target or inside it. Parent-of-target paths do not match.

## Can unmanaged files be removed on deploy?

Yes. Configure `on.deploy.remove-unmanaged` patterns.

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

## Can I get logs in JSON format?

Yes. Use `--log-format json`.

- Logs are emitted on stderr.
- Command output (including `--json`) remains on stdout.

## Can I control log verbosity?

Yes. Use `--log-level <debug|info|warn|error>`.

- default log level is `info`
- applies to all commands
