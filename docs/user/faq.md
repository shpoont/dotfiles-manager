# FAQ

## What should I read first?

Use [`getting-started.md`](./getting-started.md). It starts with a temporary
`HOME` workflow so you can run `init`, `add`, `status`, `diff`, and
`sync` without touching real dotfiles. When you need an explicit direction, it also shows the
compatibility aliases `save` and `apply`.

## Does v2 store actual values?

Yes. The settings folder stores actual managed values so settings can be
synced later.
For example, `git:user.email` with `--scope user --user-id docs-user` is stored
in:

```text
desired/user/docs-user/targets/git/settings.yaml
```

Whole-file and file-tree resources store managed bytes under `artifacts/...`.
Normal command output, previews, and ledger metadata redact raw values, but
stored settings payloads may contain the actual managed bytes. Local pre-write
evidence, where still implemented, may also contain managed bytes.

## Is this safe for secrets?

No. Do not manage secrets, credentials, private keys, tokens, account exports,
generated caches, or runtime state unless a specific reviewed recipe explicitly
supports that data. The current product is not a secret manager.

## Where are profiles stored?

In the settings folder:

```text
profiles/stacks/<stack>.yaml
profiles/layers/<layer>.yaml
```

The root `dotfiles-manager.v2.yaml` names the active stack. A stack lists layers,
and later layers override earlier selections. A command can add more layers with
repeated `--profile <layer>`, so one machine can use multiple profiles/layers.

## Can one machine have multiple profiles?

Yes. A machine has one local machine identity, but commands can resolve a stack
plus extra profile layers. For example, a laptop can use `global` from the
default stack and add `--profile work` for work-only selections.

## What scopes are available?

- `shared` — one stored value for everyone and every machine.
- `user` — one stored value for a logical user across machines.
- `machine` — one stored value for a machine.
- `machine-user` — one stored value for a logical user on a machine.

Scopes decide where stored settings are placed under `desired/`; recipes still
decide live file locations.

## Where is local state stored?

By default, outside the settings folder:

```text
macOS: ~/Library/Application Support/dotfiles-manager/v2/<settings-folder-state-id>/
Linux: ${XDG_STATE_HOME:-~/.local/state}/dotfiles-manager/v2/<settings-folder-state-id>/
```

It contains identity files, local pre-write evidence where implemented, and
ledger/run records.
Commands display state references as `state://...` URIs.

## How do I recover after a failed or wrong apply?

The public recovery workflow is review-first, not a built-in rollback command:

1. Stop and do not run another write until you understand the current state.
2. Run `status` and `diff` for the affected setting.
3. Inspect the live app setting directly when possible.
4. If you version the settings folder with Git or another versioning system, use
   that history to recover the stored settings you want.
5. Run `sync --dry-run`, `save --dry-run`, or `apply --dry-run` for the specific
   setting before confirming any repair.
6. Confirm only the intended direction with `--yes`.

Some write paths may keep internal pre-write recovery evidence in local state,
but that evidence is not a public backup/restore workflow and should not replace
reviewing the current `status` / `diff` result.

## How do I know what an app recipe manages?

Use:

```bash
dotfiles-manager recipe list
dotfiles-manager recipe discover <target>
dotfiles-manager recipe explain <target>
```

`recipe explain` shows settings, resources, drivers, lifecycle notes, and "do
not manage" exclusions. Read it before adding a target.

## What targets are supported now?

See the supported-surface matrix in [`README.md`](./README.md). The short
version is:

- Git and Starship selected values;
- Zsh, tmux, and SSH selected whole-file resources;
- Neovim selected file-tree resource;
- local app authoring for reviewed/validated local recipes and synthetic
  roundtrip fixtures;
- legacy v1 file sync compatibility.

## Does v2 support native export/import for apps like Raycast?

Not as a general public workflow in this tranche. Native export/import support
needs a reviewed recipe with explicit command metadata, lifecycle behavior,
redaction expectations, lifecycle behavior, and tests. Do not assume an
app's native export/import command is supported just because the app has one.

## Can I add my own app?

Advanced users can draft local recipes under `recipes/local/<target-id>/` and
validate them with `app validate` and synthetic fixtures with
`app test --roundtrip`. This is for local recipe authoring; it is not a public
recipe marketplace or arbitrary script execution system.

## What is `custom.files`?

`custom.files` is a low-level file/file-tree target used by migration and
internal dogfood flows. It has no app-specific semantics and does not classify
secrets for you. It is not the recommended first-user path for managing a new
application.

## How does v1 compatibility work?

Existing `.dotfiles-manager.yaml` file-sync configs still use:

```bash
dotfiles-manager status
dotfiles-manager diff
dotfiles-manager deploy --dry-run
dotfiles-manager import --dry-run
```

v1 `deploy` means source to target. v1 `import` means target to source. v2 uses
`dotfiles-manager.v2.yaml`, profiles, scopes, selected settings, stored
settings, and sync.

## How do I migrate v1 file syncs?

Preview first:

```bash
dotfiles-manager migrate --dry-run
```

Plain `migrate` writes a generated migration run under:

```text
migrations/v1-to-v2/<run-id>/generated/
```

It does not replace active root v2 config automatically. Review and promote the
generated files explicitly.

## Does `status` or `diff` write files?

No. `status` and `diff` are previews. Mutating v2 commands use explicit flags
such as `--yes`, and many support `--dry-run` for preview.

## Where are logs written?

Logs are always written to a log file.

Default paths:

- macOS: `~/Library/Logs/dotfiles-manager/dotfiles-manager.log`
- Linux: `${XDG_STATE_HOME:-~/.local/state}/dotfiles-manager/dotfiles-manager.log`

Use `--log-file <path>` to override the log location.

## How do I check CLI version?

Use either:

```bash
dotfiles-manager --version
# or
dotfiles-manager version
```

Release builds print release metadata; local non-release builds print `dev`
metadata.
