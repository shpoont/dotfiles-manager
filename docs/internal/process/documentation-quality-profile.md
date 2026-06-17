---
owner: Documentation Maintainer (TBD)
status: Living Profile
last-updated: 2026-06-18
canonical-source: docs/internal/process/documentation-quality-profile.md
---

# dotfiles-manager documentation quality profile

This document applies the reusable documentation quality acceptance criteria to
`dotfiles-manager`. The reusable standard is maintained outside this repository as a common
asset; this file is the project-specific profile, inventory, and compliance map
for the public docs in this repository.

## Project adoption profile

- **Project name:** `dotfiles-manager`
- **Project type:** CLI, local settings manager, local file/config tool
- **Primary audiences:** technical end users managing their own configuration,
  CLI users, operators of their own machines, custom recipe authors,
  contributors, and maintainers
- **Public interfaces:**
  - CLI: `dotfiles-manager` commands and flags documented in
    [`../../user/commands.md`](../../user/commands.md)
  - Configuration/schema: v2 repository layout and legacy v1 config documented
    in [`../../user/configuration.md`](../../user/configuration.md)
  - Extension system: local v2 app recipes documented in
    [`../../user/configuration.md`](../../user/configuration.md) and
    [`../../user/commands.md`](../../user/commands.md)
  - UI/API/HTTP protocol: not applicable; there is no UI, API server, or HTTP
    protocol surface
- **First-success journey:** safe temporary-`HOME` Git email workflow in
  [`../../user/getting-started.md`](../../user/getting-started.md)
- **Realistic non-trivial journey:** add several supported targets, inspect
  status/diff, save desired state, apply desired state, and restore from backup;
  see [`../../user/manual.md`](../../user/manual.md)
- **Risky or destructive operations:** `apply --yes`, `restore --yes`, legacy
  `deploy`, legacy `import`, and any future native import/export/app lifecycle
  operation
- **Data handled:** selected config values, whole-file config payloads, file-tree
  config payloads, desired artifacts, local backups, logs, and metadata
- **Network or remote-service behavior:** no routine remote service behavior;
  the CLI reads/writes local files. GitHub/Homebrew/Go network access appears in
  install workflows, not normal v2 save/apply commands.
- **Supported platforms/runtimes/package managers:** macOS and Linux are the
  documented platforms; Go 1.22+ is required for source builds; Homebrew and Go
  install are documented release paths.
- **Version and compatibility policy:** v2 is experimental and narrow; v1 file
  sync remains available for existing `.dotfiles-manager.yaml` configs.
- **Support route:** public GitHub issues with redacted diagnostics.
- **Security-reporting route:** [`../../../SECURITY.md`](../../../SECURITY.md)
- **Required validation evidence:** Markdown link checks, CLI help/reference
  check, isolated temporary-home walkthrough, and `go test ./...`.

## Public interface inventory

| Interface | Source of truth | Docs location | Validation method | Owner |
| --- | --- | --- | --- | --- |
| Root CLI and v2 commands | Cobra command tree and `dotfiles-manager <command> --help` | [`../../user/commands.md`](../../user/commands.md) | Build current binary and compare public commands/help to docs | CLI/docs maintainer |
| Legacy v1 commands | Cobra command tree plus v1 tests/contracts | [`../../user/commands.md`](../../user/commands.md) | Build current binary and compare `status`, `diff`, `deploy`, `import`, `migrate` help to docs | CLI/docs maintainer |
| v2 repository layout | Config loader, v2 init output, desired-state implementation | [`../../user/configuration.md`](../../user/configuration.md) | Run `init`, inspect generated layout, and run config tests | CLI/docs maintainer |
| Supported bundled targets | Recipe registry and `recipe list/discover/explain` | [`../../user/manual.md`](../../user/manual.md), [`../../user/README.md`](../../user/README.md), [`../../user/commands.md`](../../user/commands.md) | Run `recipe list` and `recipe explain <target>` for each target | Recipe/docs maintainer |
| Local app recipes | App authoring commands and recipe validation/tests | [`../../user/configuration.md`](../../user/configuration.md), [`../../user/commands.md`](../../user/commands.md) | Run `app create`, `app validate`, and `app test` with fixture recipes | Recipe/docs maintainer |
| Security/reporting policy | Repository security policy | [`../../../SECURITY.md`](../../../SECURITY.md) | Check file exists and is linked from README/docs map | Maintainer |

## First-success journey

- **User:** first-time technical CLI user.
- **Goal:** prove v2 can manage one Git config value without touching the real
  home directory.
- **Starting state:** binary available on `PATH` or via `DFM=<path>`; Git
  installed; empty temporary directories.
- **Safe environment:** temporary `HOME` and temporary settings repository.
- **Steps:** verify binary, create temp `HOME`, create temp repo, seed Git email,
  run `init`, inspect `recipe explain git`, run `add`, `save --dry-run`,
  `save --yes`, `apply --dry-run`, `apply --yes`, `backup list`, `backup show`,
  and `restore --dry-run`.
- **Expected output/behavior:** output names the selected setting, says whether
  commands are read-only or write project/live state, redacts managed values,
  and gives a safe next command.
- **Verification:** read generated desired artifact and run Git config command in
  the temporary `HOME`.
- **Cleanup/rollback/removal:** delete the temporary root; no real home files are
  touched.
- **Next steps:** real Git config workflow and supported targets in the manual.
- **Troubleshooting link:** [`../../user/faq.md`](../../user/faq.md)
- **Validation evidence:** run the walkthrough from
  [`../../user/getting-started.md`](../../user/getting-started.md) with the
  current binary before release.

## Product vocabulary

| Term | User-facing definition | Example | Related terms | Not to confuse with |
| --- | --- | --- | --- | --- |
| Supported target | An app/tool/config surface the manager knows how to inspect and manage. | `git`, `starship`, `tmux` | recipe, selection | arbitrary installed app |
| Recipe | The metadata that says what a target can manage, where it lives, and what is excluded. | bundled Git recipe | driver/backend, named location | desired state |
| Selection | A profile entry saying a setting/artifact should be managed. | `git:user.email` in `profiles/layers/global.yaml` | profile layer, scope | the stored value itself |
| Desired state | The saved value or artifact in the settings repository. | `desired/user/docs-user/targets/git/settings.yaml` | live state, backup | command output |
| Live state | The current machine's real config files/settings. | `~/.gitconfig` | desired state, backup | repository copy |
| Profile layer | A named YAML file with selected managed targets/settings. | `profiles/layers/global.yaml` | profile stack | OS user account |
| Scope | The sharing level for desired state. | `shared`, `user`, `machine`, `machine-user` | subject | filesystem path |
| Named location | A recipe-defined config location. | default Starship config location | live path, recipe | arbitrary unreviewed path |
| Backup | Local copy/metadata created before a supported live write. | backup run shown by `backup list` | restore, run id | desired state repository |
| Restore | Preview or perform recovery from a backup run. | `restore <run-id> --dry-run` | backup | applying desired state |
| Do-not-manage data | Data intentionally excluded for safety/privacy/runtime reasons. | SSH keys, caches, tokens | recipe exclusions | unsupported by accident |

## Safety and data-flow profile

- **Can change:** v2 desired-state files in the settings repository, selected
  live config files/settings on confirmed apply/restore, and local backup/log
  metadata.
- **Can delete/overwrite:** file-tree apply and restore can overwrite or remove
  files only after preview/confirmation. Docs must show `--dry-run` before
  `--yes`.
- **Can read:** selected local config files/settings, recipe metadata, profile
  files, desired artifacts, backup metadata, and logs.
- **Can send over the network:** normal v2 save/apply/status/diff do not send
  data over the network. Install/update commands using GitHub, Go, Homebrew, or
  Git can use the network.
- **Can store locally/remotely:** desired artifacts and backups may contain the
  actual managed bytes. If the settings repository is pushed to a remote Git
  host, those bytes leave the local machine.
- **Can appear in logs/diagnostics:** paths, target IDs, refs, run IDs, and
  metadata can appear. Managed values and secret-bearing payload bytes should be
  redacted in normal command output.
- **Must redact:** secrets, tokens, SSH keys, private config payloads, internal
  hostnames, customer data, and full logs/backups unless a maintainer confirms a
  safe private path.
- **Safest permissions:** run as the normal user whose configs are being
  managed; do not run with `sudo` unless a future recipe explicitly documents
  why it is required.
- **Rollback/recovery path:** inspect backups with `backup list`/`backup show`,
  preview with `restore <run-id> --dry-run`, then explicitly confirm with
  `restore <run-id> --yes` if the preview is correct.
- **External trust:** custom recipes and future native import/export drivers must
  be reviewed and fixture-tested before users trust them with real config.

## Compliance map

| Standard area | dotfiles-manager evidence | Status |
| --- | --- | --- |
| README/front door | [`../../../README.md`](../../../README.md) explains purpose, maturity, first safe path, install, safe example, docs map, support/security links. | Covered |
| Installation/access/upgrade/removal | [`../../user/install-and-release.md`](../../user/install-and-release.md) covers source, release, Homebrew, Go install, verification, release checks, uninstall/stop-using. | Covered |
| Getting started tutorial | [`../../user/getting-started.md`](../../user/getting-started.md) is the temporary-`HOME` first-success path. | Covered |
| Concepts/explanation | [`../../user/manual.md`](../../user/manual.md) explains concept graph, relationships, profiles/scopes, desired/live/backup model, and deferred areas. | Covered |
| How-to/task guidance | Manual and command docs cover Git, Starship, Zsh, tmux, SSH, Neovim, backup/restore, local edits, stopping management, and custom recipes. | Covered |
| CLI reference | [`../../user/commands.md`](../../user/commands.md) documents root command format, v2, v1, migration, completion, output, and exit codes. | Covered |
| Configuration/schema reference | [`../../user/configuration.md`](../../user/configuration.md) documents config loading, validation, repository control plane, scopes, desired data, locations, URI style, local state, local recipes, unsupported shapes, and v1 config. | Covered |
| Troubleshooting/FAQ | [`../../user/faq.md`](../../user/faq.md) covers read-first guidance, actual values, secrets, profiles, scopes, state, recovery, recipes, native import/export, custom apps, logs, and version checks. | Covered |
| Safety/security/privacy | README, user manual, FAQ, command docs, configuration docs, and [`../../../SECURITY.md`](../../../SECURITY.md) document redaction, do-not-manage data, actual stored values, logs, backups, and reporting. | Covered |
| Contributor/extension docs | Local recipe authoring is documented in user configuration/command docs; engineering docs and README document build/test/release checks. | Covered |
| Migration/compatibility | User docs distinguish v2 from legacy v1, document migration commands, and keep `sync` experimental. | Covered |
| Glossary/vocabulary | Manual concept sections and the vocabulary table above define project nouns and relationships. | Covered |
| Validation evidence | Release/doc PRs should include link checks, CLI help/reference checks, isolated temp-home walkthrough, and `go test ./...`. | Required per PR |

## Current validation checklist

A documentation PR that claims compliance with this profile must include:

- [ ] local Markdown link check for changed docs;
- [ ] `go test ./...`;
- [ ] built binary help/reference spot check for every public command listed in
      the public interface inventory;
- [ ] safe temporary-home first-success walkthrough from `getting-started.md`;
- [ ] review that examples do not include secrets, real private data, or
      fictional unsupported apps unless explicitly marked as schematic;
- [ ] reviewer pass from the perspective of a new technical user.
