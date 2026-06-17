# Install and release notes

These docs describe the v2 local-settings-manager workflow in this repository.
After installing any binary, verify that the binary exposes the v2 commands you
intend to use before following the quickstart.

A release-built binary should also identify where it came from:

```text
dotfiles-manager version=0.2.0 commit=<40-char-sha> date=<utc-rfc3339-z> channel=stable provenance=goreleaser
```

Homebrew formula builds use the same fields but report
`provenance=homebrew-source`. Local source builds that were not stamped by a
release tool report the explicit development fallback
`version=dev commit=unknown date=unknown channel=dev provenance=unspecified`.

## Prerequisites

- Git, for the Git-based quickstart and normal repository work.
- Go 1.22 or newer when building from source.
- A repository directory where desired settings will be stored.

## Install from the current source checkout

This is the safest path when you are working from these docs before a new public
release has been cut.

```bash
git clone https://github.com/shpoont/dotfiles-manager.git
cd dotfiles-manager
go test ./...
go build -o ./bin/dotfiles-manager ./cmd/dotfiles-manager
./bin/dotfiles-manager version
./bin/dotfiles-manager init --help
./bin/dotfiles-manager add --help
./bin/dotfiles-manager save --help
./bin/dotfiles-manager apply --help
```

Use `./bin/dotfiles-manager` as the command in the quickstart, or add `./bin` to
your shell `PATH` after you verify the build.

## Install a published release

Published releases exist for this repository, and the Homebrew tap exists. A
published binary can lag the current documentation, so always verify the v2
commands after installation.

Homebrew:

```bash
brew install shpoont/tap/dotfiles-manager
dotfiles-manager version
dotfiles-manager init --help
dotfiles-manager add --help
dotfiles-manager save --help
dotfiles-manager apply --help
```

Go module install:

```bash
go install github.com/shpoont/dotfiles-manager/cmd/dotfiles-manager@latest
dotfiles-manager version
dotfiles-manager init --help
dotfiles-manager add --help
dotfiles-manager save --help
dotfiles-manager apply --help
```

If `init`, `add`, `save`, and `apply` are not present, the installed binary is
not the v2-capable binary described by these docs. Build from the current source
checkout or install a newer release.

## Uninstall or stop using dotfiles-manager

`dotfiles-manager` does not run a background service. To stop using it:

1. Stop running the CLI.
2. Keep or archive your settings repository if you want the saved desired state
   for later. Delete it only after reviewing that it contains no values you still
   need.
3. Remove the binary installed by your chosen install method:
   - source checkout: delete `./bin/dotfiles-manager` or the checkout directory;
   - Homebrew: run `brew uninstall dotfiles-manager`;
   - Go install: remove the installed `dotfiles-manager` binary from your
     `GOBIN`/`GOPATH/bin`.
4. Optionally remove local v2 state and logs after you no longer need backups:
   - macOS state/logs: `~/Library/Application Support/dotfiles-manager/` and
     `~/Library/Logs/dotfiles-manager/`;
   - Linux state/logs: `${XDG_STATE_HOME:-~/.local/state}/dotfiles-manager/`.

Removing the binary does not undo live config changes that were already applied.
Use `backup list`, `backup show <run-id>`, and `restore <run-id> --dry-run`
before deleting local state if you may need to recover a previous config.

## Release and contributor checks

Before maintainers publish a release, the implementation should pass normal CI
and the internal v2 release-readiness evidence. User docs intentionally keep the
internal gate mechanics out of the normal workflow. The user-facing guarantee is
simpler:

- the binary should print version/help successfully;
- release artifacts and Homebrew formula builds should not report
  `commit=unknown`, `date=unknown`, `channel=dev`, or
  `provenance=unspecified`;
- the safe temporary-home quickstart in [`getting-started.md`](./getting-started.md)
  should complete without touching the real home directory;
- live writes should require explicit confirmation and create local backup
  evidence when a supported apply writes live state;
- recovery should be previewable with `restore <run-id> --dry-run` before
  `restore <run-id> --yes`.

Internal release and dogfood details live in:

- [`../internal/engineering/ci-cd.md`](../internal/engineering/ci-cd.md)
- [`../internal/engineering/v2-dogfood-readiness.md`](../internal/engineering/v2-dogfood-readiness.md)
- [`../internal/engineering/v2-issue-122-dogfood-release-gate.md`](../internal/engineering/v2-issue-122-dogfood-release-gate.md)
