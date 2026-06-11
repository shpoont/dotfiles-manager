# Install and release notes

These docs describe the v2 local-settings-manager workflow in this repository.
After installing any binary, verify that the binary exposes the v2 commands you
intend to use before following the quickstart.

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

## Release and contributor checks

Before maintainers publish a release, the implementation should pass normal CI
and the internal v2 release-readiness evidence. User docs intentionally keep the
internal gate mechanics out of the normal workflow. The user-facing guarantee is
simpler:

- the binary should print version/help successfully;
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
