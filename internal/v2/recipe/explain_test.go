package recipe

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExplainBundledGitAndCustomFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeNamedRecipe(t, root, GitTarget, stringsForExplainTest(`schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: git
displayName: Local Git Collision
supportLevel: experimental
capability: read-write
locations:
  home:
    default: "~"
settings:
  credential.helper:
    scopeDefault: user
    resource: helper
resources:
  helper:
    driver: file
    location: home
    path: .gitconfig
`))

	report, err := Explain(ExplainOptions{Target: GitTarget, RepoRoot: root})
	require.NoError(t, err)
	require.Equal(t, ExplainSchema, report.Schema)
	require.Equal(t, ExplainCommand, report.Command)
	require.Equal(t, "ok", report.Summary.Status)
	require.Equal(t, GitTarget, report.RecipeExplain.Target.Ref)
	require.Equal(t, "bundled", report.RecipeExplain.Recipe.Source)
	require.Len(t, report.RecipeExplain.Settings, 2)
	require.Equal(t, "git:user.email", report.RecipeExplain.Settings[0].Ref)
	require.Equal(t, "[user] email", report.RecipeExplain.Resources[0].Selector.Summary)
	requireDiagnosticCodeInRecipe(t, report, ExplainCodeLocalRecipeShadowed)
	requireDiagnosticCodeInRecipe(t, report, ExplainCodeSelectionUnresolved)

	text := ExplainText(report)
	require.Contains(t, text, "Target: git")
	require.Contains(t, text, "git:user.email")
	require.Contains(t, text, "Location: $HOME/.gitconfig ([user] email)")
	require.Contains(t, text, "- [credential] sections")
	require.Contains(t, text, "- credential.helper")
	require.NotContains(t, text, "recipe://")
	require.NotContains(t, text, "selector=")
	verboseText := ExplainVerboseText(report)
	require.Contains(t, verboseText, "target: git")
	require.Contains(t, verboseText, "selector=[user] email")
	require.Contains(t, verboseText, "do not manage: credential.helper")
	require.NotContains(t, text, "Local Git Collision")

	payload, err := ExplainJSON(report)
	require.NoError(t, err)
	require.Contains(t, payload, `"command": "recipe.explain"`)
	require.NotContains(t, payload, "Local Git Collision")

	custom, err := Explain(ExplainOptions{Target: CustomFilesTarget, RepoRoot: root})
	require.NoError(t, err)
	require.Equal(t, "recipe://bundled/custom.files", custom.RecipeExplain.Recipe.RecipeRef)
	require.Len(t, custom.RecipeExplain.Settings, 2)
	require.Len(t, custom.RecipeExplain.Resources, 2)
	require.Equal(t, FileTreeDriverID, custom.RecipeExplain.Resources[1].DriverID)
	require.Contains(t, ExplainText(custom), "custom.files:file-tree")

	starship, err := Explain(ExplainOptions{Target: StarshipTarget, RepoRoot: root})
	require.NoError(t, err)
	require.Equal(t, "recipe://bundled/starship", starship.RecipeExplain.Recipe.RecipeRef)
	require.Equal(t, StarshipTarget, starship.RecipeExplain.Target.Ref)
	require.Equal(t, "unknown", starship.RecipeExplain.Target.PlatformSupport)
	require.Len(t, starship.RecipeExplain.Settings, 4)
	require.Len(t, starship.RecipeExplain.Resources, 4)
	for idx, settingID := range starshipSettingIDs() {
		require.Equal(t, StarshipTarget+":"+settingID, starship.RecipeExplain.Settings[idx].Ref)
		require.Equal(t, TOMLFileDriverID, starship.RecipeExplain.Settings[idx].Driver)
		require.Equal(t, SensitivityLow, starship.RecipeExplain.Settings[idx].Sensitivity)
		require.Equal(t, settingID, starship.RecipeExplain.Resources[idx].ID)
		require.Equal(t, TOMLFileDriverID, starship.RecipeExplain.Resources[idx].DriverID)
		require.Equal(t, []string{settingID}, starship.RecipeExplain.Resources[idx].Selector.Path)
		require.Equal(t, "create", starship.RecipeExplain.Resources[idx].Selector.CreateMissing)
		require.Equal(t, "allow", starship.RecipeExplain.Resources[idx].Selector.DeleteKey)
	}
	starshipText := ExplainText(starship)
	require.Contains(t, starshipText, "Target: starship")
	require.Contains(t, starshipText, "starship:add_newline")
	require.Contains(t, starshipText, "Location: $XDG_CONFIG_HOME/starship.toml (add_newline)")
	require.Contains(t, starshipText, "- STARSHIP_CONFIG non-default locations")
	starshipVerbose := ExplainVerboseText(starship)
	require.Contains(t, starshipVerbose, "selector=add_newline")
	require.Contains(t, starshipVerbose, "do not manage: STARSHIP_CONFIG non-default locations")
	require.NotContains(t, starshipText, "secret@example.com")

	nvim, err := Explain(ExplainOptions{Target: NvimTarget, RepoRoot: root})
	require.NoError(t, err)
	require.Equal(t, "recipe://bundled/nvim", nvim.RecipeExplain.Recipe.RecipeRef)
	require.Equal(t, NvimTarget, nvim.RecipeExplain.Target.Ref)
	require.Equal(t, "linux-darwin", nvim.RecipeExplain.Target.PlatformSupport)
	require.Len(t, nvim.RecipeExplain.Settings, 1)
	require.Len(t, nvim.RecipeExplain.Resources, 1)
	require.Equal(t, NvimTarget+":config", nvim.RecipeExplain.Settings[0].Ref)
	require.Equal(t, FileTreeDriverID, nvim.RecipeExplain.Settings[0].Driver)
	require.Equal(t, SensitivityPersonal, nvim.RecipeExplain.Settings[0].Sensitivity)
	require.Equal(t, "file-tree", nvim.RecipeExplain.Settings[0].ArtifactForm)
	require.Equal(t, "user", nvim.RecipeExplain.Settings[0].DefaultScope)
	require.Equal(t, "config", nvim.RecipeExplain.Resources[0].ID)
	require.Equal(t, FileTreeDriverID, nvim.RecipeExplain.Resources[0].DriverID)
	require.Equal(t, "config", nvim.RecipeExplain.Resources[0].LocationID)
	require.Equal(t, "nvim", nvim.RecipeExplain.Resources[0].Path)
	require.Contains(t, nvim.RecipeExplain.Resources[0].Exclude, "shada/**")
	require.NotContains(t, nvim.RecipeExplain.Resources[0].Exclude, "**/*secret*")
	nvimText := ExplainVerboseText(nvim)
	require.Contains(t, nvimText, "target: nvim")
	require.Contains(t, nvimText, "nvim:config")
	require.Contains(t, nvimText, "resource=config driver=file-tree")
	require.Contains(t, nvimText, "do not manage: Neovim installation")
	require.NotContains(t, nvimText, "secret@example.com")

	tmux, err := Explain(ExplainOptions{Target: TmuxTarget, RepoRoot: root})
	require.NoError(t, err)
	require.Equal(t, "recipe://bundled/tmux", tmux.RecipeExplain.Recipe.RecipeRef)
	require.Equal(t, TmuxTarget, tmux.RecipeExplain.Target.Ref)
	require.Equal(t, "linux-darwin", tmux.RecipeExplain.Target.PlatformSupport)
	require.Len(t, tmux.RecipeExplain.Settings, 2)
	require.Len(t, tmux.RecipeExplain.Resources, 2)
	for idx, settingID := range tmuxSettingIDs() {
		require.Equal(t, TmuxTarget+":"+settingID, tmux.RecipeExplain.Settings[idx].Ref)
		require.Equal(t, FileDriverID, tmux.RecipeExplain.Settings[idx].Driver)
		require.Equal(t, SensitivityPersonal, tmux.RecipeExplain.Settings[idx].Sensitivity)
		require.Equal(t, "file", tmux.RecipeExplain.Settings[idx].ArtifactForm)
		require.Equal(t, "user", tmux.RecipeExplain.Settings[idx].DefaultScope)
		require.Equal(t, LifecycleWarn, tmux.RecipeExplain.Settings[idx].Lifecycle)
		require.Equal(t, settingID, tmux.RecipeExplain.Resources[idx].ID)
		require.Equal(t, FileDriverID, tmux.RecipeExplain.Resources[idx].DriverID)
		require.Equal(t, tmuxLocationID(settingID), tmux.RecipeExplain.Resources[idx].LocationID)
		require.Equal(t, tmuxResourcePath(settingID), tmux.RecipeExplain.Resources[idx].Path)
		require.Nil(t, tmux.RecipeExplain.Resources[idx].Selector)
		require.Empty(t, tmux.RecipeExplain.Resources[idx].Include)
		require.Empty(t, tmux.RecipeExplain.Resources[idx].Exclude)
	}
	tmuxText := ExplainVerboseText(tmux)
	require.Contains(t, tmuxText, "target: tmux")
	require.Contains(t, tmuxText, "tmux:home.conf")
	require.Contains(t, tmuxText, "tmux:xdg.conf")
	require.Contains(t, tmuxText, "resource=home.conf driver=file")
	require.Contains(t, tmuxText, "resource=xdg.conf driver=file")
	require.Contains(t, tmuxText, "do not manage: tmux server sockets")
	require.Contains(t, tmuxText, "do not manage: manual reload actions")
	require.Contains(t, tmuxText, "does not run tmux source-file")
	require.NotContains(t, tmuxText, "secret@example.com")

	ssh, err := Explain(ExplainOptions{Target: SSHTarget, RepoRoot: root})
	require.NoError(t, err)
	require.Equal(t, "recipe://bundled/ssh", ssh.RecipeExplain.Recipe.RecipeRef)
	require.Equal(t, SSHTarget, ssh.RecipeExplain.Target.Ref)
	require.Equal(t, "linux-darwin", ssh.RecipeExplain.Target.PlatformSupport)
	require.Len(t, ssh.RecipeExplain.Settings, 1)
	require.Len(t, ssh.RecipeExplain.Resources, 1)
	require.Equal(t, SSHTarget+":config", ssh.RecipeExplain.Settings[0].Ref)
	require.Equal(t, FileDriverID, ssh.RecipeExplain.Settings[0].Driver)
	require.Equal(t, SensitivityPersonal, ssh.RecipeExplain.Settings[0].Sensitivity)
	require.Equal(t, "file", ssh.RecipeExplain.Settings[0].ArtifactForm)
	require.Equal(t, "user", ssh.RecipeExplain.Settings[0].DefaultScope)
	require.Equal(t, LifecycleAllowed, ssh.RecipeExplain.Settings[0].Lifecycle)
	require.Equal(t, "config", ssh.RecipeExplain.Resources[0].ID)
	require.Equal(t, FileDriverID, ssh.RecipeExplain.Resources[0].DriverID)
	require.Equal(t, "home", ssh.RecipeExplain.Resources[0].LocationID)
	require.Equal(t, ".ssh/config", ssh.RecipeExplain.Resources[0].Path)
	require.Contains(t, ssh.RecipeExplain.Resources[0].BackupRestore, "content safety scanning passes")
	sshText := ExplainVerboseText(ssh)
	require.Contains(t, sshText, "target: ssh")
	require.Contains(t, sshText, "ssh:config")
	require.Contains(t, sshText, "resource=config driver=file")
	require.Contains(t, sshText, "do not manage: private keys")
	require.Contains(t, sshText, "do not manage: Include target files")
	require.Contains(t, sshText, "symlinked ~/.ssh/config")
	require.Contains(t, sshText, "does not walk ~/.ssh")
	require.Contains(t, sshText, "does not create ~/.ssh/config")
	require.NotContains(t, sshText, "secret@example.com")

	zsh, err := Explain(ExplainOptions{Target: ZshTarget, RepoRoot: root})
	require.NoError(t, err)
	require.Equal(t, "recipe://bundled/zsh", zsh.RecipeExplain.Recipe.RecipeRef)
	require.Equal(t, ZshTarget, zsh.RecipeExplain.Target.Ref)
	require.Equal(t, "unknown", zsh.RecipeExplain.Target.PlatformSupport)
	require.Len(t, zsh.RecipeExplain.Settings, 4)
	require.Len(t, zsh.RecipeExplain.Resources, 4)
	for idx, settingID := range zshSettingIDs() {
		require.Equal(t, ZshTarget+":"+settingID, zsh.RecipeExplain.Settings[idx].Ref)
		require.Equal(t, FileDriverID, zsh.RecipeExplain.Settings[idx].Driver)
		require.Equal(t, SensitivityPersonal, zsh.RecipeExplain.Settings[idx].Sensitivity)
		require.Equal(t, "file", zsh.RecipeExplain.Settings[idx].ArtifactForm)
		require.Equal(t, "user", zsh.RecipeExplain.Settings[idx].DefaultScope)
		require.Equal(t, settingID, zsh.RecipeExplain.Resources[idx].ID)
		require.Equal(t, FileDriverID, zsh.RecipeExplain.Resources[idx].DriverID)
		require.Equal(t, "home", zsh.RecipeExplain.Resources[idx].LocationID)
		require.Equal(t, zshResourcePath(settingID), zsh.RecipeExplain.Resources[idx].Path)
		require.Nil(t, zsh.RecipeExplain.Resources[idx].Selector)
		require.Contains(t, zsh.RecipeExplain.Resources[idx].BackupRestore, "pre-apply backup")
	}
	zshText := ExplainVerboseText(zsh)
	require.Contains(t, zshText, "target: zsh")
	require.Contains(t, zshText, "zsh:zshrc")
	require.Contains(t, zshText, "resource=zshrc driver=file")
	require.Contains(t, zshText, "do not manage: .zshenv and zsh:zshenv")
	require.Contains(t, zshText, "do not manage: .zsh_history and .zhistory")
	require.Contains(t, zshText, "do not manage: .zcompdump*")
	require.Contains(t, zshText, "do not manage: .oh-my-zsh")
	require.Contains(t, zshText, "do not manage: ZDOTDIR discovery")
	require.NotContains(t, zshText, "secret@example.com")
}

func TestExplainBundledAliasAndLocalAliasCollision(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeNamedRecipe(t, root, "gitconfig", stringsForExplainTest(`schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: gitconfig
displayName: Local Git Alias Collision
supportLevel: experimental
capability: read-write
locations:
  home:
    default: "~"
settings:
  credential.helper:
    scopeDefault: user
    resource: helper
resources:
  helper:
    driver: file
    location: home
    path: .gitconfig
`))

	report, err := Explain(ExplainOptions{Target: "gitconfig", RepoRoot: root})
	require.NoError(t, err)
	require.Equal(t, GitTarget, report.RecipeExplain.Target.Ref)
	require.Equal(t, "recipe://bundled/git", report.RecipeExplain.Recipe.RecipeRef)
	requireDiagnosticCodeInRecipe(t, report, ExplainCodeLocalRecipeShadowed)
	requireDiagnosticCodeInRecipe(t, report, ExplainCodeSelectionUnresolved)
}

func TestExplainLocalRecipe(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeNamedRecipe(t, root, "cobona", stringsForExplainTest(`schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: cobona
displayName: Cobona
supportLevel: experimental
capability: read-write
locations:
  config:
    default: ~/.cobona
settings:
  user.email:
    scopeDefault: user
    resource: config-email
resources:
  config-email:
    driver: ini-file
    location: config
    path: config.ini
    selector:
      section: user
      key: email
      missingSection: create
      missingKey: create
`))

	report, err := Explain(ExplainOptions{Target: "cobona", RepoRoot: root})
	require.NoError(t, err)
	require.Equal(t, "local", report.RecipeExplain.Recipe.Source)
	require.Equal(t, "recipe://local/cobona", report.RecipeExplain.Recipe.RecipeRef)
	require.Equal(t, "untrusted", report.RecipeExplain.Recipe.TrustStatus)
	require.Equal(t, "cobona:user.email", report.RecipeExplain.Settings[0].Ref)
	require.Equal(t, IniFileDriverID, report.RecipeExplain.Drivers[0].ID)
	require.Equal(t, "[user] email", report.RecipeExplain.Resources[0].Selector.Summary)
	require.Contains(t, ExplainText(report), "local recipe is untrusted")
}

func TestExplainLocalSelectedPathRecipeMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		target   string
		driver   string
		path     string
		norm     string
		driverTx string
	}{
		{name: "json", target: "test.json", driver: JSONFileDriverID, path: "config.json", norm: "selected JSON scalar", driverTx: "JSON selected path"},
		{name: "yaml", target: "test.yaml", driver: YAMLFileDriverID, path: "config.yaml", norm: "selected YAML scalar", driverTx: "YAML selected path"},
		{name: "toml", target: "test.toml", driver: TOMLFileDriverID, path: "config.toml", norm: "selected TOML scalar", driverTx: "TOML selected path"},
		{name: "plist", target: "test.plist", driver: PlistFileDriverID, path: "config.plist", norm: "selected plist scalar", driverTx: "plist selected path"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeNamedRecipe(t, root, tc.target, validSelectedPathRecipe(tc.target, tc.driver, tc.path))

			report, err := Explain(ExplainOptions{Target: tc.target, RepoRoot: root})
			require.NoError(t, err)
			require.Equal(t, tc.driver, report.RecipeExplain.Resources[0].DriverID)
			require.Equal(t, "selected-path", report.RecipeExplain.Resources[0].DiffMode)
			require.Equal(t, tc.norm, report.RecipeExplain.Resources[0].Normalization)
			require.Equal(t, []string{"user", "email"}, report.RecipeExplain.Resources[0].Selector.Path)
			require.Equal(t, "user.email", report.RecipeExplain.Resources[0].Selector.Summary)
			require.Equal(t, tc.driver, report.RecipeExplain.Drivers[0].ID)
			require.Contains(t, report.RecipeExplain.Drivers[0].Summary, tc.driverTx)
			require.Equal(t, "scalar", report.RecipeExplain.Settings[0].ArtifactForm)
			require.Contains(t, ExplainText(report), "Location:")
			require.NotContains(t, ExplainText(report), "selector=")
			require.Contains(t, ExplainVerboseText(report), "selector=user.email")
		})
	}
}

func TestExplainLocalNativeOperationIsMetadataOnly(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeNamedRecipe(t, root, "native-test", stringsForExplainTest(`schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: native-test
displayName: Native Test
supportLevel: experimental
capability: read-write
locations:
  home:
    default: "~"
settings:
  settings:
    scopeDefault: user
    resource: marker
    artifactForm: native
resources:
  marker:
    driver: file
    location: home
    path: .native-test/marker
nativeOperations:
  export-settings:
    kind: export
    reviewed: true
    runner: command
    platforms: [darwin]
    artifactForm: native
    diffMode: metadata-only
    lifecycle: allowed
    workingDirectory: temp
    timeoutSeconds: 5
    expectedExitCodes: [0]
    command:
      executable: /definitely/not/a/real/native-tool
      args:
        - literal: "--out"
        - output: bundle
    stdin:
      mode: none
    stdout:
      mode: discard
    stderr:
      mode: discard
    outputs:
      bundle:
        root: artifact
        path: exports/settings.bundle
    redaction: metadata-only
`))

	report, err := Explain(ExplainOptions{Target: "native-test", RepoRoot: root})
	require.NoError(t, err)
	require.Len(t, report.RecipeExplain.NativeOperations, 1)
	require.Equal(t, "export-settings", report.RecipeExplain.NativeOperations[0].ID)
	require.Equal(t, "reviewed argv command; executable and args are not printed by recipe explain", report.RecipeExplain.NativeOperations[0].CommandSummary)
	require.NotContains(t, ExplainText(report), "/definitely/not/a/real/native-tool")
}

func TestExplainErrorsAndExitCodes(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	report, err := Explain(ExplainOptions{Target: "missing", RepoRoot: root})
	require.Error(t, err)
	require.Equal(t, ExplainCodeUnknownTarget, report.Error.Code)
	require.Equal(t, 2, err.(*ExplainError).ExitCode())
	require.Equal(t, []string{CustomFilesTarget, GitTarget, NvimTarget, SSHTarget, StarshipTarget, TmuxTarget, ZshTarget}, report.Error.Details["knownTargets"])
	require.Contains(t, ExplainText(report), "unknown recipe target")
	require.Contains(t, ExplainVerboseText(report), "error[unknown-target]")

	report, err = Explain(ExplainOptions{Target: "git:user.email", RepoRoot: root})
	require.Error(t, err)
	require.Equal(t, ExplainCodeUnsupportedRefKind, report.Error.Code)
	require.Equal(t, 2, err.(*ExplainError).ExitCode())

	report, err = Explain(ExplainOptions{Target: "BadTarget", RepoRoot: root})
	require.Error(t, err)
	require.Equal(t, ExplainCodeInvalidRef, report.Error.Code)
	require.Equal(t, 2, err.(*ExplainError).ExitCode())

	writeNamedRecipe(t, root, "broken", stringsForExplainTest(`schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: broken
displayName: Broken
supportLevel: experimental
capability: read-write
locations:
  config:
    default: ~/.broken
settings:
  bad:
    scopeDefault: user
    resource: missing
resources:
  other:
    driver: file
    location: config
    path: config.txt
`))
	report, err = Explain(ExplainOptions{Target: "broken", RepoRoot: root})
	require.Error(t, err)
	require.Equal(t, ExplainCodeInvalidRecipe, report.Error.Code)
	require.Equal(t, 2, err.(*ExplainError).ExitCode())
	requireDiagnosticCodeInRecipe(t, report, ExplainCodeInvalidRecipe)
	requireDiagnosticCodeInRecipe(t, report, "setting.resource.unknown")
	validationDiagnostics, ok := report.Error.Details["diagnostics"].([]ValidationDiagnostic)
	require.True(t, ok)
	require.NotEmpty(t, validationDiagnostics)
	require.Equal(t, "setting.resource.unknown", validationDiagnostics[0].Code)
	payload, err := ExplainJSON(report)
	require.NoError(t, err)
	require.Contains(t, payload, `"diagnostics"`)
	require.Contains(t, payload, `"setting.resource.unknown"`)

	require.Equal(t, "", (*ExplainError)(nil).Error())
	require.Equal(t, 1, (*ExplainError)(nil).ExitCode())
	require.Equal(t, 1, (&ExplainError{Message: "x"}).ExitCode())
}

func TestExplainRenderingAndHelperBranches(t *testing.T) {
	t.Parallel()

	jsonPayload, err := ExplainJSON(nil)
	require.NoError(t, err)
	require.Contains(t, jsonPayload, `"status": "error"`)
	require.Contains(t, ExplainText(nil), "The command could not complete")
	require.Contains(t, ExplainVerboseText(nil), "summary status=error")

	resource := explainResource("tree", Resource{Driver: FileTreeDriverID, Location: "config", Path: "profiles", Include: []string{"**/*.json"}, Exclude: []string{"cache/**"}})
	require.Equal(t, "file-tree", resource.DiffMode)
	require.Equal(t, []string{"**/*.json"}, resource.Include)
	require.Equal(t, []string{"cache/**"}, resource.Exclude)

	resource = explainResource("raw", Resource{Driver: FileDriverID, Location: "config", Path: "config.txt"})
	require.Equal(t, "file", resource.DiffMode)

	resource = explainResource("unknown", Resource{Driver: "custom-driver", Location: "config", Path: "config.txt"})
	require.Equal(t, "unknown", resource.DiffMode)

	drivers := driverExplains(FileDriverID, FileDriverID, "custom-driver", "")
	require.Len(t, drivers, 2)
	require.Equal(t, "custom-driver", drivers[0].ID)
	require.Equal(t, FileDriverID, drivers[1].ID)
	require.Equal(t, "unknown", artifactFormForDriver("custom-driver"))
	require.Equal(t, "unknown", fallbackUnknown(""))
	require.Equal(t, "user email", fallbackLabel("user.email"))

	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "other.yaml")
	require.Equal(t, "other.yaml", safeRelOrBase(root, outside))
	require.False(t, fileExists(filepath.Join(root, "missing.yaml")))
	file := filepath.Join(root, "exists.yaml")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o644))
	require.True(t, fileExists(file))
}

func TestFriendlyExplainHelpersCoverFallbackBranches(t *testing.T) {
	t.Parallel()

	require.Contains(t, friendlyExplainErrorText(&ExplainReport{Error: &ExplainErrorObject{Message: "boom"}}), "boom")
	require.Contains(t, friendlyExplainErrorText(nil), "The command could not complete.")

	require.Equal(t, "Git", friendlyTargetName(ExplainTarget{DisplayName: "Git"}))
	require.Equal(t, "Custom App", friendlyTargetName(ExplainTarget{Ref: "custom.app"}))
	require.Equal(t, "App", friendlyTargetName(ExplainTarget{}))

	require.Equal(t, "User email", friendlySettingLabel(ExplainSetting{ID: "user.email", Label: "User email"}))
	require.Equal(t, "user email", friendlySettingLabel(ExplainSetting{ID: "user.email"}))

	require.Equal(t, "$HOME/.gitconfig", friendlyResourceLocation(ExplainResource{LocationID: "home", Path: ".gitconfig"}))
	require.Equal(t, "$XDG_CONFIG_HOME/starship.toml", friendlyResourceLocation(ExplainResource{LocationID: "config", Path: "/starship.toml"}))
	require.Equal(t, "chosen by the custom recipe", friendlyResourceLocation(ExplainResource{LocationID: "recipe-defined", Path: "ignored"}))
	require.Equal(t, "bundle/settings.json", friendlyResourceLocation(ExplainResource{LocationID: "bundle", Path: "/settings.json"}))
	require.Equal(t, "bundle", friendlyResourceLocation(ExplainResource{LocationID: "bundle"}))
	require.Equal(t, "path-only.yaml", friendlyResourceLocation(ExplainResource{Path: "path-only.yaml"}))
	require.Equal(t, "JSON path $.user.email", friendlyResourceLocation(ExplainResource{Selector: &ExplainSelector{Summary: "JSON path $.user.email"}}))
	require.Equal(t, "$HOME/.gitconfig ([user] email)", friendlyResourceLocation(ExplainResource{LocationID: "home", Path: ".gitconfig", Selector: &ExplainSelector{Summary: "[user] email"}}))

	require.Equal(t, "single value", friendlyArtifactForm("scalar"))
	require.Equal(t, "file", friendlyArtifactForm("file"))
	require.Equal(t, "folder tree", friendlyArtifactForm("file-tree"))
	require.Equal(t, "native export", friendlyArtifactForm("native"))
	require.Equal(t, "native export", friendlyArtifactForm("native-export"))
	require.Equal(t, "custom form", friendlyArtifactForm("custom-form"))

	settings := []ExplainSetting{
		{ID: "read", Capability: "read-only"},
		{ID: "export", Capability: "export-only"},
		{ID: "write", Capability: "read-write"},
	}
	require.Equal(t, "export", firstManageableSetting(settings).ID)
	require.Equal(t, "read", firstManageableSetting([]ExplainSetting{{ID: "read", Capability: "read-only"}}).ID)
	require.Nil(t, firstManageableSetting(nil))

	require.Equal(t, "dotfiles-manager --config dotfiles-manager.v2.yaml add git --setting user.email --scope user --dry-run", friendlyRecipeAddCommand("git", "user.email", "user"))
	require.Equal(t, "dotfiles-manager --config dotfiles-manager.v2.yaml add git --dry-run", friendlyRecipeAddCommand("git", "", "unknown"))
	require.Equal(t, "Custom App", titleWords("custom app"))
	require.Equal(t, []string{"line"}, trimBlank([]string{"line", "", "  "}))
}

func requireDiagnosticCodeInRecipe(t *testing.T, report *ExplainReport, code string) {
	t.Helper()
	for _, diagnostic := range report.RecipeExplain.Diagnostics {
		if diagnostic.Code == code {
			return
		}
	}
	require.Failf(t, "diagnostic code missing", "expected diagnostic code %q in %#v", code, report.RecipeExplain.Diagnostics)
}

func stringsForExplainTest(value string) string {
	return value
}
