# Native export/import candidate selection, 2026-06-10

Status: issue #112 decision artifact.
Reviewer: Codex agent.
Review date: 2026-06-10.
Scope: research and selection only; no runtime implementation is added by this
issue.

## Decision

No reviewed native export/import target is selected for issue #113.

Every reviewed candidate has useful configuration-management affordances, but none
meets the production gate for a bundled native export/import recipe: current
primary evidence for both export and import/apply, a non-ambiguous app-supported
mechanism, and a reviewed operation contract that can run without shell snippets,
user scripts, inherited `PATH`, inherited environment, or secret-bearing logs.

Issue #113 should not be started as currently written. The product should either
find another app with a true non-interactive supported export/import API, or
change the next implementation issue from "native export/import" to a simpler
file-backed app recipe. The latter is likely the better user-facing path for the
MVP because several common apps expose stable text configuration files or
app-watched import paths.

## Non-implementation boundary

Issue #112 may produce only this research artifact and tracker updates. It must
not add export/import code, command runners, app integrations, fixtures, schema
changes, or implementation stubs for issue #113.

## Acceptance gate used for this review

A candidate is acceptable only when all of the following are true:

- Current primary evidence verifies both export and import/apply.
- The mechanisms are app-supported native operations, not scraping private app
  storage or copying opaque internal databases.
- The later operation contract can be expressed as fixed argv/API or a fixed
  app-owned apply path with no shell, no arbitrary script, no inherited `PATH`,
  no broad inherited environment, bounded timeout, and explicit status handling.
- The artifact is stable enough for structured diff, or it has an honest
  normalized metadata representation.
- Accounts, credentials, license keys, tokens, sessions, history, caches,
  telemetry, generated indexes, and machine identifiers can be excluded or
  explicitly treated as unsupported.
- Lifecycle semantics are clear enough to avoid destructive or surprising
  restores.
- The app has enough user value to justify bundled support.

A candidate is rejected when any of these are missing. UI-only export/import can
be valuable for manual users, but it is not enough for a production native driver
unless the driver design explicitly supports attended UI flows. The current v2
native contract does not.

## Evidence hierarchy

- Primary source for acceptance: current vendor documentation, official app
  repository documentation, or official CLI/API help.
- Local observation: supporting evidence only. It can confirm installed versions
  or filesystem presence, but cannot justify acceptance by itself.
- Third-party posts, old issues, and reverse-engineering notes: rejection/risk
  support only.

## Local observation summary

The following metadata-only local checks were performed on 2026-06-10. No
configuration contents, credentials, tokens, history, or private payloads were
printed or inspected.

- Raycast was installed at `$HOME/Applications/Raycast.app`, bundle
  `com.raycast.macos`, version `1.104.19`.
- iTerm2 was installed at `$HOME/Applications/iTerm.app`, bundle
  `com.googlecode.iterm2`, version `3.6.11`.
- DataGrip was installed at `$HOME/Applications/DataGrip.app`, bundle
  `com.jetbrains.datagrip`, version `2025.2.5.1`.
- JetBrains Toolbox was installed at
  `$HOME/Applications/JetBrains Toolbox.app`, bundle
  `com.jetbrains.toolbox`, version `3.5.0.84344`.
- Rectangle was installed at `$HOME/Applications/Rectangle.app`,
  bundle `com.knollsoft.Rectangle`, version `0.96`.
- A stub directory named
  `$HOME/Applications/Visual Studio Code.app` existed, but it did not
  contain a readable `Contents/Info.plist`; no local VS Code version was used as
  evidence.
- Warp was not found in the checked standard per-user or system application
  paths.
- iTerm2's dynamic profile directory existed at
  `$HOME/Library/Application Support/iTerm2/DynamicProfiles`.
- Rectangle's defaults plist existed at
  `$HOME/Library/Preferences/com.knollsoft.Rectangle.plist`.
- No `code`, `warp`, or `rectangle` command-line executable was found on the
  checked `PATH`.

## Candidate summary

| Candidate | Decision | Main reason |
| --- | --- | --- |
| Raycast | Rejected for issue #113 | Export/import exists, but full export is encrypted and includes history/private categories; scoped JSON commands are UI-driven, not a reviewed fixed operation. |
| Visual Studio Code | Rejected for issue #113 | Profile export/import is documented in the Profiles editor; CLI docs do not verify profile export/import. |
| iTerm2 | Rejected for issue #113 | Dynamic Profiles and external preferences are file-backed mechanisms, not a complete native export/import operation. |
| Warp | Rejected for issue #113 | Strong file-backed `settings.toml`, but no native export/import driver is needed or verified for settings. |
| DataGrip / JetBrains IDEs | Rejected for issue #113 | Settings ZIP export/import is UI-driven, may include account/server data, and import can overwrite/restart. |
| Rectangle | Rejected for issue #113 | Best near miss: JSON config import path is documented, but export is UI-only and import is launch-time one-shot. |

## Reading the candidate matrix

In the matrix below, “verified export mechanism” and “verified import/apply
mechanism” mean that the human-facing app capability was found in current
primary evidence. That wording does not mean the mechanism is approved for a v2
implementation. A candidate is accepted only when the documented capability also
meets the reviewed native-driver operation contract from the acceptance gate.

## Candidate matrix

### Raycast

- Candidate/app name: Raycast.
- Review date: 2026-06-10.
- Reviewer: Codex agent.
- Version/source context: local Raycast `1.104.19`; Raycast Manual pages
  retrieved 2026-06-10. The Import & Export page was last updated
  2026-05-13. The Settings page was last updated 2026-06-04. The Quicklinks
  and Snippets pages were last updated 2026-06-10. The Raycast API deeplinks
  page was also reviewed.
- Primary evidence:
  - Raycast Manual, Import & Export:
    <https://manual.raycast.com/import-export>.
  - Raycast Manual, Settings:
    <https://manual.raycast.com/settings>.
  - Raycast Manual, Quicklinks:
    <https://manual.raycast.com/quicklinks>.
  - Raycast Manual, Snippets:
    <https://manual.raycast.com/snippets>.
  - Raycast API, Deeplinks:
    <https://developers.raycast.com/information/lifecycle/deeplinks>.
- Local observation summary: installed locally; no Raycast data contents were
  inspected.
- Verified export mechanism: Raycast documents `Export Settings & Data`, which
  creates an encrypted `.rayconfig` bundle. It also documents `Export Snippets`
  and `Export Quicklinks` as JSON exports.
- Verified import/apply mechanism: Raycast documents `Import Settings & Data`
  for `.rayconfig`, with an import checklist, plus `Import Snippets` and
  `Import Quicklinks` commands for scoped imports.
- Artifact format: `.rayconfig` for full export; JSON for Quicklinks; CSV or
  supported text-expander formats for Snippet import/export flows.
- Diffability: full `.rayconfig` is encrypted and opaque. Quicklinks JSON is
  diffable. Snippet export can be diffable when JSON or CSV, but the exact
  export shape is command-driven and not enough for a fixed driver contract.
- Managed categories: full export covers many Raycast categories, including
  settings, aliases, hotkeys, extensions, Quicklinks, Snippets, Notes, MCP
  servers, AI chats/commands/agents, clipboard history, emoji/symbol history,
  Wrapped, and window layouts. Scoped commands cover Quicklinks and Snippets.
- Excluded categories: accounts, credentials, tokens, license keys, sessions,
  AI chats, Notes, clipboard history, emoji/symbol history, Wrapped, cache,
  telemetry, generated indexes, local machine IDs, and any extension-specific
  authenticated state.
- Account/secret/history/cache risk rating: high for full `.rayconfig` because
  it intentionally bundles private/history categories; medium for scoped
  Quicklinks/Snippets because those can contain private URLs or text but are
  user-authored and reviewable.
- Redaction/logging rule: only artifact path, size, category names, Raycast
  version, and operation metadata may be recorded. No `.rayconfig` payload,
  passphrase, Quicklink URL, Snippet text, AI chat, note, clipboard, token, or
  account payload may be logged.
- Lifecycle behavior: full import is selective and additive/merge-oriented per
  the Raycast manual, but it is still an attended app command with checklist and
  passphrase handling. Scheduled exports can write encrypted `.rayconfig` files
  to a chosen folder. Quicklink and Snippet imports require Raycast commands and
  user review/confirmation.
- Verification method: primary docs review plus local installed-version check.
  No export/import was executed.
- User-value fit: high. Raycast is exactly the kind of app users expect a local
  settings manager to handle.
- Acceptance/rejection decision: rejected for issue #113.
- Accept/reject reason: the current evidence verifies Raycast export/import
  features, but not a production-ready, non-interactive reviewed operation for
  the v2 native driver. The full export is encrypted and intentionally includes
  excluded private/history categories. The scoped JSON flows are promising but
  are still exposed as Raycast UI commands/file choosers, not fixed argv/API
  operations. Raycast may become acceptable only after a dedicated attended UI
  driver is designed, or after Raycast provides a documented non-interactive
  export/import API for scoped categories. Manual backup/export capability alone
  is not dotfiles-safe scoped export/import approval.
- Follow-up implementation issue eligibility: no for current issue #113; yes
  only for a future attended/manual Raycast driver or a future official
  non-interactive Raycast API.

### Visual Studio Code

- Candidate/app name: Visual Studio Code.
- Review date: 2026-06-10.
- Reviewer: Codex agent.
- Version/source context: no usable local VS Code app version was found; VS Code
  documentation retrieved 2026-06-10. The Profiles page shows date
  2026-02-04. The CLI page and Settings page were reviewed.
- Primary evidence:
  - VS Code Profiles:
    <https://code.visualstudio.com/docs/configure/profiles>.
  - VS Code Command Line Interface:
    <https://code.visualstudio.com/docs/configure/command-line>.
  - VS Code User and workspace settings:
    <https://code.visualstudio.com/docs/configure/settings>.
- Local observation summary: `$HOME/Applications/Visual Studio Code.app`
  existed but was an empty-looking stub without `Contents/Info.plist`; `code`
  was not found on `PATH`.
- Verified export mechanism: VS Code documents exporting a profile from the
  Profiles editor to either a GitHub gist or a local `.code-profile` file.
- Verified import/apply mechanism: VS Code documents importing from the Profiles
  editor by selecting an import action, providing a gist URL or local profile
  file, then creating the imported profile.
- Artifact format: `.code-profile` for local profile exports; GitHub gist URL
  for online sharing.
- Diffability: not accepted. The local file may be inspectable, but the primary
  docs do not define a stable machine-writable schema or non-interactive export
  and import contract.
- Managed categories: profiles can include settings, keyboard shortcuts, MCP
  servers, snippets, tasks, extensions, UI layout, and profile associations.
- Excluded categories: accounts, tokens, extension secrets, sessions, histories,
  caches, telemetry, generated indexes, remote/SSH/container state, local paths,
  and machine-specific values. VS Code specifically notes that machine-specific
  settings are not exported in profiles.
- Account/secret/history/cache risk rating: medium. VS Code profiles are scoped,
  but extensions and tasks can reference private paths or commands, and gist
  sharing introduces account/network concerns.
- Redaction/logging rule: log profile name, profile file path, selected
  categories, size, VS Code version, and command metadata only. Do not log
  setting values, extension configuration payloads, task contents, gist URLs with
  private identifiers, tokens, or account information.
- Lifecycle behavior: export/import is an attended Profiles editor flow. The CLI
  supports selecting or creating a profile with `--profile` and managing
  extensions, but the reviewed docs do not verify CLI profile export/import.
- Verification method: primary docs review plus local app/CLI check. No profile
  export/import was executed.
- User-value fit: high. Editor settings are a central dotfiles-manager use case.
- Acceptance/rejection decision: rejected for issue #113.
- Accept/reject reason: current primary docs verify a UI-based profile sharing
  flow, but not a fixed non-interactive export/import operation suitable for the
  v2 native operation contract. VS Code remains a good candidate for ordinary
  file-backed management of `settings.json`, `keybindings.json`, snippets, and
  an extensions list, not for this native export/import issue.
- Follow-up implementation issue eligibility: no for current issue #113; yes for
  a separate file-backed VS Code settings recipe or if VS Code adds an official
  CLI/API for profile import/export.

### iTerm2

- Candidate/app name: iTerm2.
- Review date: 2026-06-10.
- Reviewer: Codex agent.
- Version/source context: local iTerm2 `3.6.11`; iTerm2 documentation retrieved
  2026-06-10.
- Primary evidence:
  - iTerm2 Dynamic Profiles:
    <https://iterm2.com/documentation-dynamic-profiles.html>.
  - iTerm2 Preferences documentation:
    <https://iterm2.com/documentation/2.1/documentation-preferences.html>.
- Local observation summary: installed locally. The dynamic profile directory
  existed. No profile content was printed or inspected.
- Verified export mechanism: iTerm2 documents that the easiest way to get JSON
  for an existing profile is to open Settings > Profiles, choose the profile,
  open Other Actions, and choose Save Profile as JSON. The Preferences page also
  documents loading preferences from a custom folder or URL and saving on quit.
- Verified import/apply mechanism: iTerm2 Dynamic Profiles are loaded from
  `~/Library/Application Support/iTerm2/DynamicProfiles`; iTerm2 monitors this
  folder and reloads valid property-list files at runtime.
- Artifact format: JSON, XML, or binary Apple property list for dynamic profiles;
  preference folder/plist for broader preferences.
- Diffability: JSON and XML dynamic profile files are diffable; binary plists
  and full preference plists require normalization.
- Managed categories: profile-level settings through Dynamic Profiles; broader
  app preferences through the custom preference folder mechanism.
- Excluded categories: terminal scrollback, paste/copy history, session
  restoration state, tmux live sessions, shell history, command output logs,
  secrets in commands, local host paths, generated state, caches, and machine
  identifiers.
- Account/secret/history/cache risk rating: medium. Dynamic Profiles can be
  safe, but command fields may include private hosts or commands. Broader plist
  preferences can include history-related or machine-specific values.
- Redaction/logging rule: log only file path, file hash, size, profile count,
  iTerm2 version, and validation status. Do not log command strings, hostnames,
  paths, terminal output, paste history, scrollback, or profile payload values.
- Lifecycle behavior: Dynamic Profiles hot-reload while iTerm2 is running. UI
  edits to dynamic profiles do not update the source file unless the profile is
  marked rewritable. Full external preferences may prompt on quit and are more
  global/destructive.
- Verification method: primary docs review plus local installed-version and path
  presence check. No dynamic profile file was changed.
- User-value fit: high for developer machines.
- Acceptance/rejection decision: rejected for issue #113.
- Accept/reject reason: iTerm2 has excellent file-backed apply semantics for a
  specific category, but it is not a complete app-owned native export/import
  operation. Export is an attended UI action for a selected profile, while apply
  is a file-watched location. This belongs in a file-backed recipe or named
  location design, not in the first native export/import recipe.
- Follow-up implementation issue eligibility: no for current issue #113; yes for
  a future file-backed iTerm2 Dynamic Profiles recipe.

### Warp

- Candidate/app name: Warp.
- Review date: 2026-06-10.
- Reviewer: Codex agent.
- Version/source context: Warp was not found locally in checked app paths; Warp
  docs retrieved 2026-06-10. The Settings file page was updated within the prior
  week; the Settings Sync page was last updated 2026-06-09.
- Primary evidence:
  - Warp Settings file:
    <https://docs.warp.dev/terminal/settings/>.
  - Warp All settings reference:
    <https://docs.warp.dev/terminal/settings/all-settings/>.
  - Warp Drive overview:
    <https://docs.warp.dev/knowledge-and-collaboration/warp-drive/>.
  - Warp Settings Sync:
    <https://docs.warp.dev/terminal/more-features/settings-sync/>.
- Local observation summary: no installed Warp app or `warp` CLI was found.
- Verified export mechanism: for terminal settings, Warp stores preferences in a
  plain-text `settings.toml` that can be edited directly or checked into version
  control. Warp Drive objects have UI import/export flows, including an
  "Export all Warp Drive objects" command.
- Verified import/apply mechanism: `settings.toml` changes hot-reload and are
  reflected in the graphical settings panel. Warp Drive imports are UI-driven.
- Artifact format: TOML for terminal settings; YAML, Markdown, or dotenv for
  certain Warp Drive object categories.
- Diffability: high for `settings.toml`; high for supported Warp Drive text
  exports, with the usual caution that dotenv may contain secrets.
- Managed categories: terminal settings represented by `settings.toml`; Warp
  Drive workflows, notebooks, prompts, and environment variables through Warp
  Drive import/export; cloud Settings Sync for many settings.
- Excluded categories: accounts, cloud sync state, environment variables with
  secrets, AI/session sharing data, history, caches, local paths, startup shell,
  platform-specific settings, and any device-specific setting.
- Account/secret/history/cache risk rating: low to medium for `settings.toml`;
  high for environment variable exports; medium for cloud Settings Sync because
  it is account-backed and remote.
- Redaction/logging rule: for `settings.toml`, log path, hash, size, setting key
  names, Warp version, and parse status only. Do not log values that may contain
  paths, prompts, environment values, account identifiers, or AI/session data.
- Lifecycle behavior: `settings.toml` hot-reloads immediately and invalid TOML
  falls back to defaults for affected settings while showing a warning. Warp
  Drive import/export is attended UI. Settings Sync writes to Warp cloud and has
  non-synced categories.
- Verification method: primary docs review plus local absence check. No file was
  changed.
- User-value fit: high if the user uses Warp; lower for this specific machine
  because Warp is not installed.
- Acceptance/rejection decision: rejected for issue #113.
- Accept/reject reason: Warp does not need a native export/import driver for
  terminal settings because the primary supported surface is already a plain text
  file. This is a strong argument for a file-backed first recipe, not a native
  export/import target.
- Follow-up implementation issue eligibility: no for current issue #113; yes for
  a separate file-backed Warp `settings.toml` recipe if Warp support is desired.

### DataGrip / JetBrains IDEs

- Candidate/app name: DataGrip as the locally installed JetBrains IDE, with
  JetBrains IDE documentation used for shared IDE settings behavior.
- Review date: 2026-06-10.
- Reviewer: Codex agent.
- Version/source context: local DataGrip `2025.2.5.1`; JetBrains IntelliJ IDEA
  2026.1 Help and DataGrip Help pages retrieved 2026-06-10. The IDE settings
  backup and sync page was dated 2026-03-17; the DataGrip configuration page was
  dated 2026-03-18.
- Primary evidence:
  - JetBrains IDE settings backup and sync:
    <https://www.jetbrains.com/help/idea/sharing-your-ide-settings.html>.
  - JetBrains Configuring the IDE:
    <https://www.jetbrains.com/help/idea/configuring-project-and-ide-settings.html>.
  - JetBrains DataGrip configuration:
    <https://www.jetbrains.com/help/datagrip/configuring-project-and-ide-settings.html>.
- Local observation summary: DataGrip and JetBrains Toolbox were installed.
  No JetBrains config contents were inspected.
- Verified export mechanism: JetBrains documents exporting selected IDE settings
  to a ZIP archive from File > Manage IDE Settings > Export Settings.
- Verified import/apply mechanism: JetBrains documents importing from File >
  Manage IDE Settings > Import Settings, selecting a ZIP archive or backup
  directory, selecting components, and applying them. Applying backup settings
  can overwrite current IDE configuration and require restart.
- Artifact format: ZIP archive for manual settings export; backup configuration
  directory for restore/import; cloud-backed Backup and Sync as an alternative.
- Diffability: poor to medium. ZIP archives and configuration directories can be
  inspected after unpacking, but primary docs do not define a stable normalized
  schema for a safe automated driver.
- Managed categories: IDE themes, keymaps, color schemes, UI settings, menus,
  toolbar settings, project view settings, editor settings, code completion,
  hints, live templates, code styles, enabled/disabled plugins, deployment
  servers, Git settings, Debugger settings, Registry keys, database tools, CSV
  formats, server certificates, and more.
- Excluded categories: JetBrains Account state, license state, registered GitHub
  or other VCS accounts, deployment server credentials, database credentials,
  certificates/private keys, plugins with secrets, workspace/project-local
  state, caches, generated indexes, logs, histories, telemetry, and machine
  identifiers.
- Account/secret/history/cache risk rating: high. Official docs explicitly list
  categories such as configured deployment servers, Git settings including
  registered GitHub accounts, database/tool settings, and server certificates.
- Redaction/logging rule: log only IDE product/version, archive path, size,
  selected top-level component names, and import/export status. Do not log ZIP
  contents, XML payload values, accounts, server names, certificates, database
  connection details, paths, tokens, or license data.
- Lifecycle behavior: export/import is an attended menu/dialog flow. Cloud
  Backup and Sync is account-backed and updates when settings change. Applying a
  backup can overwrite current configuration and restart the IDE.
- Verification method: primary docs review plus local installed-version check.
  No export/import was executed.
- User-value fit: high for developer machines, but risk is high.
- Acceptance/rejection decision: rejected for issue #113.
- Accept/reject reason: JetBrains has a real settings export/import feature, but
  it is UI-driven, broad, and sensitive. It is not currently expressible as a
  safe fixed native operation without attended UI, careful component selection,
  and secret/account exclusion design. The overwrite/restart semantics are too
  risky for the first native recipe. Manual ZIP backup/export capability alone
  is not dotfiles-safe scoped export/import approval.
- Follow-up implementation issue eligibility: no for current issue #113; yes
  only after a dedicated attended UI or vendor API contract exists with explicit
  component allowlists and secret/account exclusions.

### Rectangle

- Candidate/app name: Rectangle.
- Review date: 2026-06-10.
- Reviewer: Codex agent.
- Version/source context: local Rectangle `0.96`; official Rectangle repository
  documentation retrieved 2026-06-10. The latest repository release shown during
  review was `v0.96` from 2026-05-31.
- Primary evidence:
  - Rectangle repository README:
    <https://github.com/rxhanson/Rectangle>.
  - Rectangle app site:
    <https://rectangleapp.com/>.
- Local observation summary: installed locally. Rectangle defaults plist existed.
  No defaults plist contents were inspected.
- Verified export mechanism: Rectangle documents Preferences-pane buttons for
  importing and exporting config as a JSON file.
- Verified import/apply mechanism: Rectangle documents that, on launch, it loads
  `~/Library/Application Support/Rectangle/RectangleConfig.json` if present and
  renames the file with a timestamp so it is not read on subsequent launches.
- Artifact format: JSON config for import/export; NSUserDefaults plist for
  internal storage.
- Diffability: medium to high for JSON config; lower for defaults plist. The
  JSON may still include serialized values that require normalization before
  friendly diffs.
- Managed categories: Rectangle preferences and keyboard shortcuts.
- Excluded categories: macOS Accessibility/TCC grants, app launch agent state,
  logs, crash reports, update state, telemetry, defaults unrelated to Rectangle,
  machine-specific display/window state, and any private app/window names that
  might appear in future config.
- Account/secret/history/cache risk rating: low. Rectangle is a local window
  manager and the documented config is preferences/shortcuts rather than account
  state. Machine/display-specific layout values remain possible.
- Redaction/logging rule: log only Rectangle version, JSON path, size, hash,
  top-level key names, import-file consumption status, and parse status. Do not
  log shortcut payloads, app names, display identifiers, or plist contents by
  default.
- Lifecycle behavior: import is launch-time and one-shot; Rectangle renames the
  import file after consuming it. Applying a config may require quitting and
  relaunching Rectangle or ensuring it is not running before the file is placed.
  Export is currently an attended Preferences-pane button.
- Verification method: primary docs review plus local installed-version and path
  presence check. No config was changed and Rectangle was not relaunched.
- User-value fit: medium to high. Window-manager shortcuts are useful and
  low-risk, especially for new Mac setup.
- Acceptance/rejection decision: rejected for issue #113, but closest near miss.
- Accept/reject reason: Rectangle has the cleanest local, low-secret JSON story
  among reviewed candidates. However, the reviewed evidence still lacks a fixed
  non-interactive export operation. Import/apply is a fixed launch-time file
  path, but export is a Preferences UI action. This is not enough for the first
  native export/import recipe under the current gate. It is a strong candidate
  if issue #113 is reframed as a file-backed/import-path recipe rather than a
  native export/import recipe.
- Follow-up implementation issue eligibility: no for current issue #113; yes for
  a separate Rectangle file-backed/import-path recipe with explicit lifecycle
  handling.

## Near-miss ranking for follow-up triage

1. Rectangle: best local low-secret near miss. It should be treated as a
   file-backed/import-path candidate, not as native export/import. The decision
   would change only if Rectangle gains a documented non-interactive export API
   or if the follow-up issue is explicitly reframed around its launch-time JSON
   import path.
2. iTerm2 Dynamic Profiles: strong developer-machine value and hot-reloaded
   JSON/XML property-list files. The decision would change only for a
   file-backed Dynamic Profiles recipe or if iTerm2 exposes a documented
   non-interactive full export/import contract.
3. Warp `settings.toml`: cleanest text settings model, but Warp was not found
   locally. The decision would change for a file-backed Warp recipe, not for a
   native export/import issue.
4. Raycast: strategically important but blocked by attended commands, encrypted
   full backups, and private/history categories. The decision would change only
   after an attended-driver safety design or an official scoped non-interactive
   API exists.

## Product implications

The research suggests that the v2 user-facing happy path should not depend on
native export/import being common. Common apps tend to expose one of three
patterns:

1. Plain-text or app-watched configuration files, such as Warp `settings.toml` or
   iTerm2 Dynamic Profiles.
2. Attended UI export/import flows, such as Raycast, VS Code Profiles,
   JetBrains settings, and Rectangle's Preferences buttons.
3. Broad cloud sync or encrypted backup features that are convenient for humans
   but unsuitable for transparent dotfiles-style diffs.

For a production MVP, the next implementation issue should probably support a
file-backed app recipe first. Native export/import should remain in the model,
but only for apps with either a documented non-interactive API/CLI or an
explicitly designed attended-driver mode with user confirmation.

## Validation checklist

- Only this markdown research artifact is required for issue #112.
- No implementation files, schemas, fixtures, stubs, command runners, bundled
  recipes, or app integrations were added.
- No export/import/apply operations were executed against live app data.
- No private configuration payloads, credentials, tokens, account data, history,
  caches, or secret-bearing files were inspected.
- Local observation was limited to app/path presence and version metadata, with
  home paths redacted as `$HOME/...`.
- Issue #113 implementation work was not started.

## Follow-up recommendation

- Keep issue #112 as the evidence-backed decision that no native target was
  selected.
- Do not start issue #113 as currently written.
- Either block issue #113 pending discovery of a true native target, or replace
  its scope with a first file-backed app recipe. Good candidates from this
  review are:
  - Rectangle, because it is installed locally, low-secret, has a documented JSON
    config and launch-time import path, but needs lifecycle handling.
  - iTerm2 Dynamic Profiles, because it is installed locally and has documented
    hot-reloaded JSON/XML property-list profiles.
  - Warp `settings.toml`, because it is the cleanest text settings model, but
    Warp is not installed on this machine.
- If Raycast remains strategically important, create a separate design issue for
  attended UI/native-command drivers instead of forcing it into the fixed native
  operation contract.
