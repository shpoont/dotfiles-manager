package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	v2ledger "github.com/shpoont/dotfiles-manager/internal/v2/ledger"
	v2recipe "github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/stretchr/testify/require"
)

func TestV2StatusFallbackWhenOnlyV2RootExists(t *testing.T) {
	fixture := setupCLIV2SelectedPreviewFixture(t, true, true)
	setCWD(t, fixture.repoRoot)

	payload, stdout, err := runSelectedPreviewCLI(t, []string{"status", "--json", "--machine-id", "mbp", "--user-id", "leon", "test.app:identity.email"})
	require.NoError(t, err)
	require.Equal(t, "dotfiles-manager.v2.preview", payload["schema"])
	require.Equal(t, "status", payload["command"])
	require.Equal(t, float64(1), payload["schemaVersion"])
	require.Equal(t, false, payload["dryRun"])
	require.NotContains(t, stdout, "current@example.com")
	require.NotContains(t, stdout, "desired@example.com")

	items := payload["items"].([]any)
	require.Len(t, items, 1)
	item := items[0].(map[string]any)
	require.Equal(t, "test.app:identity.email", item["settingRef"])
	require.Equal(t, "test.app", item["targetRef"])
	require.Equal(t, "user", item["scope"])
	require.Equal(t, "leon", item["subject"])
	require.Equal(t, false, item["mutated"])
}

func TestExplicitV2ConfigSelectsV2Status(t *testing.T) {
	fixture := setupCLIV2SelectedPreviewFixture(t, true, true)
	otherDir := t.TempDir()
	setCWD(t, otherDir)

	payload, stdout, err := runSelectedPreviewCLI(t, []string{"status", "--json", "--config", filepath.Join(fixture.repoRoot, "dotfiles-manager.v2.yaml"), "--user-id", "leon", "test.app:identity.email"})
	require.NoError(t, err)
	require.Equal(t, "dotfiles-manager.v2.preview", payload["schema"])
	require.Equal(t, "status", payload["command"])
	require.NotContains(t, stdout, "current@example.com")
	require.NotContains(t, stdout, "desired@example.com")
}

func TestV1StatusWinsWhenBothMarkersExist(t *testing.T) {
	fixture := setupCLIV2SelectedPreviewFixture(t, true, true)
	writeCLIFile(t, filepath.Join(fixture.repoRoot, ".dotfiles-manager.yaml"), "syncs:\n  - target: .config/nvim\n    source: source/nvim\n")
	writeCLIFile(t, filepath.Join(fixture.repoRoot, "source", "nvim", "init.lua"), "source\n")
	writeCLIFile(t, filepath.Join(fixture.homeDir, ".config", "nvim", "init.lua"), "target\n")
	setCWD(t, fixture.repoRoot)

	payload, _, err := runSelectedPreviewCLI(t, []string{"status", "--json"})
	require.NoError(t, err)
	require.Equal(t, "4.0", payload["schema_version"])
	require.Equal(t, "status", payload["command"])
	require.NotEqual(t, "dotfiles-manager.v2.preview", payload["schema"])
}

func TestInvalidV1ConfigDoesNotFallbackToV2(t *testing.T) {
	fixture := setupCLIV2SelectedPreviewFixture(t, true, true)
	writeCLIFile(t, filepath.Join(fixture.repoRoot, ".dotfiles-manager.yaml"), "syncs: not-a-list\n")
	setCWD(t, fixture.repoRoot)

	payload, _, err := runSelectedPreviewCLI(t, []string{"status", "--json"})
	require.Error(t, err)
	require.Equal(t, false, payload["ok"])
	require.Equal(t, "4.0", payload["schema_version"])
	require.Equal(t, "status", payload["command"])
	errorObj := payload["error"].(map[string]any)
	require.Equal(t, "DFM_CONFIG_SCHEMA_TYPE", errorObj["code"])
	require.NotEqual(t, "dotfiles-manager.v2.preview", payload["schema"])
}

func TestV2SaveApplyWithoutV2RootDoNotCreateState(t *testing.T) {
	homeDir := setTempHome(t)
	setCWD(t, t.TempDir())

	for _, command := range []string{"save", "apply"} {
		t.Run(command, func(t *testing.T) {
			payload, _, err := runSelectedPreviewCLI(t, []string{command, "--json", "test.app:identity.email"})
			require.Error(t, err)
			require.Equal(t, "dotfiles-manager.v2.preview", payload["schema"])
			require.Equal(t, command, payload["command"])
			require.Equal(t, false, payload["dryRun"])
			summary := payload["summary"].(map[string]any)
			require.Equal(t, "error", summary["status"])
			errorObj := payload["error"].(map[string]any)
			require.Equal(t, "selectedpreview.root.notFound", errorObj["code"])
		})
	}
	require.NoDirExists(t, filepath.Join(homeDir, "Library", "Application Support", "dotfiles-manager"))
}

func TestV2FlagsRejectedInV1Mode(t *testing.T) {
	projectDir := t.TempDir()
	setTempHome(t)
	setCWD(t, projectDir)
	writeCLIFile(t, filepath.Join(projectDir, ".dotfiles-manager.yaml"), "syncs:\n  - target: .config/nvim\n    source: source/nvim\n")

	payload, _, err := runSelectedPreviewCLI(t, []string{"status", "--json", "--machine-id", "mbp"})
	require.Error(t, err)
	require.Equal(t, false, payload["ok"])
	errorObj := payload["error"].(map[string]any)
	require.Equal(t, "DFM_FLAG_UNSUPPORTED", errorObj["code"])
}

func TestV2SaveDryRunPreviewDoesNotWriteDesiredArtifacts(t *testing.T) {
	fixture := setupCLIV2SelectedPreviewFixture(t, true, false)
	setCWD(t, fixture.repoRoot)
	desiredPath := filepath.Join(fixture.repoRoot, "desired", "user", "leon", "targets", "test.app", "settings.yaml")
	require.NoFileExists(t, desiredPath)

	payload, stdout, err := runSelectedPreviewCLI(t, []string{"save", "--dry-run", "--json", "--user-id", "leon", "test.app:identity.email"})
	require.NoError(t, err)
	require.Equal(t, "dotfiles-manager.v2.preview", payload["schema"])
	require.Equal(t, "save", payload["command"])
	require.Equal(t, true, payload["dryRun"])
	require.NoFileExists(t, desiredPath)
	require.NotContains(t, stdout, "current@example.com")
}

func TestV2SaveApplyLiveRequireYesForChangesAndYesMutates(t *testing.T) {
	fixture := setupCLIV2SelectedPreviewFixture(t, true, false)
	setCWD(t, fixture.repoRoot)
	desiredPath := filepath.Join(fixture.repoRoot, "desired", "user", "leon", "targets", "test.app", "settings.yaml")

	payload, stdout, err := runSelectedPreviewCLI(t, []string{"save", "--json", "--user-id", "leon", "test.app:identity.email"})
	require.Error(t, err)
	require.Equal(t, "dotfiles-manager.v2.preview", payload["schema"])
	errorObj := payload["error"].(map[string]any)
	require.Equal(t, "selectedlive.confirmationRequired", errorObj["code"])
	require.NoFileExists(t, desiredPath)
	require.NotContains(t, stdout, "current@example.com")

	payload, stdout, err = runSelectedPreviewCLI(t, []string{"save", "--yes", "--json", "--user-id", "leon", "test.app:identity.email"})
	require.NoError(t, err)
	require.Equal(t, "save", payload["command"])
	require.Equal(t, false, payload["dryRun"])
	require.FileExists(t, desiredPath)
	require.Contains(t, string(mustReadCLIFile(t, desiredPath)), "current@example.com")
	require.NotContains(t, stdout, "current@example.com")

	writeCLIFile(t, filepath.Join(fixture.liveRoot, "config.yaml"), "user:\n  email: changed-live@example.com\n")

	payload, stdout, err = runSelectedPreviewCLI(t, []string{"apply", "--json", "--user-id", "leon", "test.app:identity.email"})
	require.Error(t, err)
	require.Equal(t, "apply", payload["command"])
	errorObj = payload["error"].(map[string]any)
	require.Equal(t, "selectedlive.confirmationRequired", errorObj["code"])
	require.Contains(t, string(mustReadCLIFile(t, filepath.Join(fixture.liveRoot, "config.yaml"))), "changed-live@example.com")
	require.NotContains(t, stdout, "current@example.com")
	require.NotContains(t, stdout, "changed-live@example.com")

	payload, stdout, err = runSelectedPreviewCLI(t, []string{"apply", "--non-interactive", "--json", "--user-id", "leon", "test.app:identity.email"})
	require.Error(t, err)
	require.Equal(t, "apply", payload["command"])
	errorObj = payload["error"].(map[string]any)
	require.Equal(t, "selectedlive.confirmationRequired", errorObj["code"])
	require.Contains(t, string(mustReadCLIFile(t, filepath.Join(fixture.liveRoot, "config.yaml"))), "changed-live@example.com")
	require.NotContains(t, stdout, "current@example.com")
	require.NotContains(t, stdout, "changed-live@example.com")

	_, stdout, err = runSelectedPreviewCLI(t, []string{"apply", "--yes", "--json", "--user-id", "leon", "test.app:identity.email"})
	require.NoError(t, err)
	require.Contains(t, string(mustReadCLIFile(t, filepath.Join(fixture.liveRoot, "config.yaml"))), "current@example.com")
	require.NotContains(t, stdout, "current@example.com")
	require.NotContains(t, stdout, "changed-live@example.com")
}

func TestV2BundledGitSelectedSettingStatusDiffSaveApplyEndToEnd(t *testing.T) {
	fixture := setupCLIV2BundledGitFixture(t)
	setCWD(t, fixture.repoRoot)
	desiredPath := filepath.Join(fixture.repoRoot, "desired", "user", "leon", "targets", "git", "settings.yaml")
	helperSecret := "credential-helper-secret"
	writeCLIFile(t, filepath.Join(fixture.homeDir, ".gitconfig"), "[credential]\n\thelper = "+helperSecret+"\n[user]\n\temail = current@example.com\n")

	payload, stdout, err := runSelectedPreviewCLI(t, []string{"status", "--json", "--user-id", "leon", "git:user.email"})
	require.NoError(t, err)
	require.Equal(t, "status", payload["command"])
	items := payload["items"].([]any)
	require.Len(t, items, 1)
	item := items[0].(map[string]any)
	require.Equal(t, "git:user.email", item["settingRef"])
	recipeInfo := item["recipe"].(map[string]any)
	require.Equal(t, "bundled", recipeInfo["source"])
	require.Equal(t, "recipe://bundled/git", recipeInfo["recipeRef"])
	require.NotContains(t, stdout, "current@example.com")
	require.NotContains(t, stdout, helperSecret)

	payload, stdout, err = runSelectedPreviewCLI(t, []string{"save", "--dry-run", "--json", "--user-id", "leon", "git:user.email"})
	require.NoError(t, err)
	require.Equal(t, "save", payload["command"])
	require.Equal(t, true, payload["dryRun"])
	summary := payload["summary"].(map[string]any)
	require.Equal(t, "changed", summary["status"])
	require.Equal(t, float64(1), summary["saved"])
	items = payload["items"].([]any)
	item = items[0].(map[string]any)
	require.Equal(t, "would-promote", item["plannedAction"])
	require.Contains(t, item["message"], "promoted into desired state")
	require.NoFileExists(t, desiredPath)
	require.NotContains(t, stdout, "current@example.com")
	require.NotContains(t, stdout, helperSecret)

	payload, stdout, err = runSelectedPreviewCLI(t, []string{"save", "--yes", "--json", "--user-id", "leon", "git:user.email"})
	require.NoError(t, err)
	require.Equal(t, "save", payload["command"])
	require.FileExists(t, desiredPath)
	require.Contains(t, string(mustReadCLIFile(t, desiredPath)), "current@example.com")
	require.NotContains(t, stdout, "current@example.com")
	require.NotContains(t, stdout, helperSecret)

	writeCLIFile(t, filepath.Join(fixture.homeDir, ".gitconfig"), "[credential]\n\thelper = "+helperSecret+"\n[user]\n\temail = changed@example.com\n")
	payload, stdout, err = runSelectedPreviewCLI(t, []string{"diff", "--json", "--user-id", "leon", "git:user.email"})
	require.NoError(t, err)
	require.Equal(t, "diff", payload["command"])
	items = payload["items"].([]any)
	diffInfo := items[0].(map[string]any)["diff"].(map[string]any)
	require.Equal(t, "metadata-only", diffInfo["mode"])
	require.NotContains(t, stdout, "changed@example.com")
	require.NotContains(t, stdout, "current@example.com")
	require.NotContains(t, stdout, helperSecret)

	payload, stdout, err = runSelectedPreviewCLI(t, []string{"apply", "--dry-run", "--json", "--user-id", "leon", "git:user.email"})
	require.NoError(t, err)
	require.Equal(t, true, payload["dryRun"])
	require.Contains(t, string(mustReadCLIFile(t, filepath.Join(fixture.homeDir, ".gitconfig"))), "changed@example.com")
	require.NotContains(t, stdout, "changed@example.com")
	require.NotContains(t, stdout, "current@example.com")

	payload, stdout, err = runSelectedPreviewCLI(t, []string{"apply", "--yes", "--json", "--user-id", "leon", "git:user.email"})
	require.NoError(t, err)
	require.Equal(t, "apply", payload["command"])
	require.Contains(t, string(mustReadCLIFile(t, filepath.Join(fixture.homeDir, ".gitconfig"))), "current@example.com")
	require.Contains(t, string(mustReadCLIFile(t, filepath.Join(fixture.homeDir, ".gitconfig"))), helperSecret)
	require.NotContains(t, stdout, "changed@example.com")
	require.NotContains(t, stdout, "current@example.com")
	require.NotContains(t, stdout, helperSecret)
}

func TestV2BundledGitCredentialHelperSelectionIsUnsupported(t *testing.T) {
	fixture := setupCLIV2BundledGitFixture(t)
	setCWD(t, fixture.repoRoot)
	writeCLIFile(t, filepath.Join(fixture.repoRoot, "profiles", "layers", "global.yaml"), "schema: dotfiles-manager.v2.profile-layer\nschemaVersion: 1\nselections:\n  git:\n    settings:\n      credential.helper:\n        scope: user\n")
	writeCLIFile(t, filepath.Join(fixture.homeDir, ".gitconfig"), "[credential]\n\thelper = credential-helper-secret\n")

	payload, stdout, err := runSelectedPreviewCLI(t, []string{"status", "--json", "--user-id", "leon", "git:credential.helper"})
	require.NoError(t, err)
	summary := payload["summary"].(map[string]any)
	require.Equal(t, "blocked", summary["status"])
	items := payload["items"].([]any)
	require.Len(t, items, 1)
	item := items[0].(map[string]any)
	require.Equal(t, "unsupported", item["state"])
	diagnostics := item["diagnostics"].([]any)
	require.Equal(t, "selectedpreview.resource.unknown", diagnostics[0].(map[string]any)["code"])
	require.NotContains(t, stdout, "credential-helper-secret")
}

func TestV2BundledGitPromotionBlocksOnCaseAmbiguousConfig(t *testing.T) {
	fixture := setupCLIV2BundledGitFixture(t)
	setCWD(t, fixture.repoRoot)
	desiredPath := filepath.Join(fixture.repoRoot, "desired", "user", "leon", "targets", "git", "settings.yaml")
	writeCLIFile(t, filepath.Join(fixture.homeDir, ".gitconfig"), "[User]\n\tEmail = raw-case@example.com\n")

	payload, stdout, err := runSelectedPreviewCLI(t, []string{"save", "--yes", "--json", "--user-id", "leon", "git:user.email"})
	require.Error(t, err)
	require.Equal(t, "save", payload["command"])
	summary := payload["summary"].(map[string]any)
	require.Equal(t, "error", summary["status"])
	errorObj := payload["error"].(map[string]any)
	require.Equal(t, "selectedlive.planBlocked", errorObj["code"])
	items := payload["items"].([]any)
	require.Len(t, items, 1)
	item := items[0].(map[string]any)
	require.Equal(t, "blocked-safety", item["state"])
	require.Empty(t, item["plannedAction"])
	diagnostics := item["diagnostics"].([]any)
	require.Equal(t, "selectedvalue.driver.invalid-selector", diagnostics[0].(map[string]any)["code"])
	require.NoFileExists(t, desiredPath)
	require.NotContains(t, stdout, "raw-case@example.com")
}

func TestV2BundledStarshipSelectedSettingStatusDiffSaveApplyEndToEnd(t *testing.T) {
	fixture := setupCLIV2BundledStarshipFixture(t, "add_newline")
	setCWD(t, fixture.repoRoot)
	desiredPath := filepath.Join(fixture.repoRoot, "desired", "user", "leon", "targets", "starship", "settings.yaml")
	starshipPath := filepath.Join(fixture.homeDir, ".config", "starship.toml")
	secretFormat := "SECRET-LIKE-STARSHIP-FORMAT"
	writeCLIFile(t, starshipPath, "format = '"+secretFormat+"'\nadd_newline = true\n")

	payload, stdout, err := runSelectedPreviewCLI(t, []string{"status", "--json", "--user-id", "leon", "starship:add_newline"})
	require.NoError(t, err)
	require.Equal(t, "status", payload["command"])
	items := payload["items"].([]any)
	require.Len(t, items, 1)
	item := items[0].(map[string]any)
	require.Equal(t, "starship:add_newline", item["settingRef"])
	recipeInfo := item["recipe"].(map[string]any)
	require.Equal(t, "bundled", recipeInfo["source"])
	require.Equal(t, "recipe://bundled/starship", recipeInfo["recipeRef"])
	require.NotContains(t, stdout, secretFormat)

	payload, stdout, err = runSelectedPreviewCLI(t, []string{"save", "--dry-run", "--json", "--user-id", "leon", "starship:add_newline"})
	require.NoError(t, err)
	require.Equal(t, "save", payload["command"])
	require.Equal(t, true, payload["dryRun"])
	summary := payload["summary"].(map[string]any)
	require.Equal(t, "changed", summary["status"])
	require.Equal(t, float64(1), summary["saved"])
	items = payload["items"].([]any)
	item = items[0].(map[string]any)
	require.Equal(t, "would-promote", item["plannedAction"])
	require.NoFileExists(t, desiredPath)
	require.NotContains(t, stdout, secretFormat)

	payload, stdout, err = runSelectedPreviewCLI(t, []string{"save", "--yes", "--json", "--user-id", "leon", "starship:add_newline"})
	require.NoError(t, err)
	require.Equal(t, "save", payload["command"])
	require.FileExists(t, desiredPath)
	desiredBody := string(mustReadCLIFile(t, desiredPath))
	require.Contains(t, desiredBody, "add_newline:")
	require.Contains(t, desiredBody, "kind: bool")
	require.Contains(t, desiredBody, "value: true")
	require.NotContains(t, stdout, secretFormat)

	writeCLIFile(t, starshipPath, "format = '"+secretFormat+"'\nadd_newline = false\n")
	payload, stdout, err = runSelectedPreviewCLI(t, []string{"diff", "--json", "--user-id", "leon", "starship:add_newline"})
	require.NoError(t, err)
	require.Equal(t, "diff", payload["command"])
	items = payload["items"].([]any)
	diffInfo := items[0].(map[string]any)["diff"].(map[string]any)
	require.Equal(t, "metadata-only", diffInfo["mode"])
	require.NotContains(t, stdout, secretFormat)

	payload, stdout, err = runSelectedPreviewCLI(t, []string{"apply", "--dry-run", "--json", "--user-id", "leon", "starship:add_newline"})
	require.NoError(t, err)
	require.Equal(t, true, payload["dryRun"])
	require.Contains(t, string(mustReadCLIFile(t, starshipPath)), "add_newline = false")
	require.NotContains(t, stdout, secretFormat)

	payload, stdout, err = runSelectedPreviewCLI(t, []string{"apply", "--yes", "--json", "--user-id", "leon", "starship:add_newline"})
	require.NoError(t, err)
	require.Equal(t, "apply", payload["command"])
	liveBody := string(mustReadCLIFile(t, starshipPath))
	require.Contains(t, liveBody, "add_newline = true")
	require.Contains(t, liveBody, secretFormat)
	require.NotContains(t, stdout, secretFormat)
}

func TestV2BundledStarshipUnsupportedSelectionIsBlocked(t *testing.T) {
	fixture := setupCLIV2BundledStarshipFixture(t, "custom.command")
	setCWD(t, fixture.repoRoot)
	secretFormat := "SECRET-LIKE-UNSUPPORTED"
	writeCLIFile(t, filepath.Join(fixture.homeDir, ".config", "starship.toml"), "format = '"+secretFormat+"'\n")

	payload, stdout, err := runSelectedPreviewCLI(t, []string{"status", "--json", "--user-id", "leon", "starship:custom.command"})
	require.NoError(t, err)
	summary := payload["summary"].(map[string]any)
	require.Equal(t, "blocked", summary["status"])
	items := payload["items"].([]any)
	require.Len(t, items, 1)
	item := items[0].(map[string]any)
	require.Equal(t, "unsupported", item["state"])
	diagnostics := item["diagnostics"].([]any)
	require.Equal(t, "selectedpreview.resource.unknown", diagnostics[0].(map[string]any)["code"])
	require.NotContains(t, stdout, secretFormat)
}

func TestV2BundledStarshipTypeSafetyBlocksCLIPlanningWithoutMutation(t *testing.T) {
	t.Run("wrong live bool blocks status and save", func(t *testing.T) {
		fixture := setupCLIV2BundledStarshipFixture(t, "add_newline")
		setCWD(t, fixture.repoRoot)
		desiredPath := filepath.Join(fixture.repoRoot, "desired", "user", "leon", "targets", "starship", "settings.yaml")
		raw := "WRONG-LIVE-BOOL"
		writeCLIFile(t, filepath.Join(fixture.homeDir, ".config", "starship.toml"), "add_newline = '"+raw+"'\n")

		payload, stdout, err := runSelectedPreviewCLI(t, []string{"status", "--json", "--user-id", "leon", "starship:add_newline"})
		require.NoError(t, err)
		summary := payload["summary"].(map[string]any)
		require.Equal(t, "blocked", summary["status"])
		items := payload["items"].([]any)
		diagnostics := items[0].(map[string]any)["diagnostics"].([]any)
		require.Equal(t, "selectedvalue.starship.boolTypeUnsupported", diagnostics[0].(map[string]any)["code"])
		require.NotContains(t, stdout, raw)

		payload, stdout, err = runSelectedPreviewCLI(t, []string{"save", "--yes", "--json", "--user-id", "leon", "starship:add_newline"})
		require.Error(t, err)
		errorObj := payload["error"].(map[string]any)
		require.Equal(t, "selectedlive.planBlocked", errorObj["code"])
		require.NoFileExists(t, desiredPath)
		require.NotContains(t, stdout, raw)
	})

	t.Run("wrong desired bool blocks apply before mutation", func(t *testing.T) {
		fixture := setupCLIV2BundledStarshipFixture(t, "add_newline")
		setCWD(t, fixture.repoRoot)
		starshipPath := filepath.Join(fixture.homeDir, ".config", "starship.toml")
		raw := "WRONG-DESIRED-BOOL"
		writeCLIFile(t, starshipPath, "add_newline = true\n")
		writeCLIFile(t, filepath.Join(fixture.repoRoot, "desired", "user", "leon", "targets", "starship", "settings.yaml"), "schema: dotfiles-manager.v2.desired-settings\nschemaVersion: 1\nvalues:\n  add_newline:\n    intent: set\n    kind: string\n    value: "+raw+"\n")

		payload, stdout, err := runSelectedPreviewCLI(t, []string{"apply", "--yes", "--json", "--user-id", "leon", "starship:add_newline"})
		require.Error(t, err)
		errorObj := payload["error"].(map[string]any)
		require.Equal(t, "selectedlive.planBlocked", errorObj["code"])
		items := payload["items"].([]any)
		diagnostics := items[0].(map[string]any)["diagnostics"].([]any)
		require.Equal(t, "selectedvalue.starship.boolTypeUnsupported", diagnostics[0].(map[string]any)["code"])
		require.Equal(t, "add_newline = true\n", string(mustReadCLIFile(t, starshipPath)))
		require.NotContains(t, stdout, raw)
	})

	t.Run("wrong desired integer blocks apply before mutation", func(t *testing.T) {
		fixture := setupCLIV2BundledStarshipFixture(t, "scan_timeout")
		setCWD(t, fixture.repoRoot)
		starshipPath := filepath.Join(fixture.homeDir, ".config", "starship.toml")
		writeCLIFile(t, starshipPath, "scan_timeout = 30\n")
		writeCLIFile(t, filepath.Join(fixture.repoRoot, "desired", "user", "leon", "targets", "starship", "settings.yaml"), "schema: dotfiles-manager.v2.desired-settings\nschemaVersion: 1\nvalues:\n  scan_timeout:\n    intent: set\n    kind: number\n    value: \"1.5\"\n")

		payload, stdout, err := runSelectedPreviewCLI(t, []string{"apply", "--yes", "--json", "--user-id", "leon", "starship:scan_timeout"})
		require.Error(t, err)
		errorObj := payload["error"].(map[string]any)
		require.Equal(t, "selectedlive.planBlocked", errorObj["code"])
		items := payload["items"].([]any)
		diagnostics := items[0].(map[string]any)["diagnostics"].([]any)
		require.Equal(t, "selectedvalue.starship.integerTypeUnsupported", diagnostics[0].(map[string]any)["code"])
		require.Equal(t, "scan_timeout = 30\n", string(mustReadCLIFile(t, starshipPath)))
		require.NotContains(t, stdout, "1.5")
	})
}

func TestV2BundledStarshipDeleteIntentAppliesSelectedKeyOnly(t *testing.T) {
	fixture := setupCLIV2BundledStarshipFixture(t, "add_newline")
	setCWD(t, fixture.repoRoot)
	starshipPath := filepath.Join(fixture.homeDir, ".config", "starship.toml")
	secretFormat := "SECRET-LIKE-DELETE-FORMAT"
	writeCLIFile(t, starshipPath, "format = '"+secretFormat+"'\nadd_newline = true\n")
	writeCLIFile(t, filepath.Join(fixture.repoRoot, "desired", "user", "leon", "targets", "starship", "settings.yaml"), "schema: dotfiles-manager.v2.desired-settings\nschemaVersion: 1\nvalues:\n  add_newline:\n    intent: delete\n")

	payload, stdout, err := runSelectedPreviewCLI(t, []string{"apply", "--yes", "--json", "--user-id", "leon", "starship:add_newline"})
	require.NoError(t, err)
	require.Equal(t, "apply", payload["command"])
	liveBody := string(mustReadCLIFile(t, starshipPath))
	require.NotContains(t, liveBody, "add_newline")
	require.Contains(t, liveBody, secretFormat)
	require.NotContains(t, stdout, secretFormat)
}

func TestV2BundledZshSelectedStartupFileStatusDiffSaveApplyEndToEnd(t *testing.T) {
	fixture := setupCLIV2BundledZshFixture(t, "zshrc")
	setCWD(t, fixture.repoRoot)
	livePath := filepath.Join(fixture.homeDir, ".zshrc")
	desiredPath := filepath.Join(fixture.repoRoot, "desired", "user", "leon", "targets", "zsh", "artifacts", "zshrc")
	currentBody := "CURRENT-ZSHRC-CONTENT\n"
	changedBody := "CHANGED-ZSHRC-CONTENT\n"
	writeCLIFile(t, livePath, currentBody)

	payload, stdout, err := runSelectedPreviewCLI(t, []string{"status", "--json", "--user-id", "leon", "zsh:zshrc"})
	require.NoError(t, err)
	require.Equal(t, "status", payload["command"])
	items := payload["items"].([]any)
	require.Len(t, items, 1)
	item := items[0].(map[string]any)
	require.Equal(t, "zsh:zshrc", item["settingRef"])
	require.Equal(t, "desired://user/leon/targets/zsh/artifacts/zshrc", item["desiredUri"])
	resource := item["resource"].(map[string]any)
	require.Equal(t, "file", resource["driverId"])
	require.Equal(t, ".zshrc", resource["relPath"])
	recipeInfo := item["recipe"].(map[string]any)
	require.Equal(t, "bundled", recipeInfo["source"])
	require.Equal(t, "recipe://bundled/zsh", recipeInfo["recipeRef"])
	require.NotContains(t, stdout, currentBody)

	payload, stdout, err = runSelectedPreviewCLI(t, []string{"save", "--dry-run", "--json", "--user-id", "leon", "zsh:zshrc"})
	require.NoError(t, err)
	require.Equal(t, true, payload["dryRun"])
	items = payload["items"].([]any)
	item = items[0].(map[string]any)
	require.Equal(t, "would-promote", item["plannedAction"])
	requireCLIDiagnosticCode(t, item, v2recipe.ZshRiskShellStartupFileCode)
	require.NoFileExists(t, desiredPath)
	require.NotContains(t, stdout, currentBody)

	payload, stdout, err = runSelectedPreviewCLI(t, []string{"save", "--yes", "--json", "--user-id", "leon", "zsh:zshrc"})
	require.NoError(t, err)
	require.FileExists(t, desiredPath)
	require.Equal(t, currentBody, string(mustReadCLIFile(t, desiredPath)))
	items = payload["items"].([]any)
	requireCLIDiagnosticCode(t, items[0].(map[string]any), v2recipe.ZshRiskShellStartupFileCode)
	require.NotContains(t, stdout, currentBody)

	writeCLIFile(t, livePath, changedBody)
	payload, stdout, err = runSelectedPreviewCLI(t, []string{"diff", "--json", "--user-id", "leon", "zsh:zshrc"})
	require.NoError(t, err)
	items = payload["items"].([]any)
	item = items[0].(map[string]any)
	diffInfo := item["diff"].(map[string]any)
	require.Equal(t, "metadata-only", diffInfo["mode"])
	require.Equal(t, "raw file contents omitted", diffInfo["redaction"])
	requireNoCLIDiagnosticCode(t, item, v2recipe.ZshRiskShellStartupFileCode)
	require.NotContains(t, stdout, changedBody)
	require.NotContains(t, stdout, currentBody)

	payload, stdout, err = runSelectedPreviewCLI(t, []string{"apply", "--dry-run", "--json", "--user-id", "leon", "zsh:zshrc"})
	require.NoError(t, err)
	require.Equal(t, true, payload["dryRun"])
	require.Equal(t, changedBody, string(mustReadCLIFile(t, livePath)))
	items = payload["items"].([]any)
	requireCLIDiagnosticCode(t, items[0].(map[string]any), v2recipe.ZshRiskShellStartupFileCode)
	require.NotContains(t, stdout, changedBody)
	require.NotContains(t, stdout, currentBody)

	payload, stdout, err = runSelectedPreviewCLI(t, []string{"apply", "--yes", "--json", "--user-id", "leon", "zsh:zshrc"})
	require.NoError(t, err)
	require.Equal(t, currentBody, string(mustReadCLIFile(t, livePath)))
	items = payload["items"].([]any)
	item = items[0].(map[string]any)
	requireCLIDiagnosticCode(t, item, v2recipe.ZshRiskShellStartupFileCode)
	mutation := item["mutation"].(map[string]any)
	require.NotEmpty(t, mutation["backupRefs"])
	runID := mutation["runId"].(string)
	stateRoot, stateErr := v2ledger.DefaultStateRoot(fixture.repoRoot)
	require.NoError(t, stateErr)
	ledgerBody := string(mustReadCLIFile(t, filepath.Join(stateRoot, "ledger", "ledger.jsonl")))
	require.NotContains(t, ledgerBody, changedBody)
	require.NotContains(t, ledgerBody, currentBody)
	backupMetadata := string(mustReadCLIFile(t, filepath.Join(stateRoot, "backups", runID, "backup.yaml")))
	require.NotContains(t, backupMetadata, changedBody)
	require.NotContains(t, backupMetadata, currentBody)
	require.NotContains(t, stdout, changedBody)
	require.NotContains(t, stdout, currentBody)
}

func TestV2BundledTmuxSelectedConfigFileStatusDiffSaveApplyEndToEnd(t *testing.T) {
	fixture := setupCLIV2BundledTmuxFixture(t, "home.conf")
	setCWD(t, fixture.repoRoot)
	livePath := filepath.Join(fixture.homeDir, ".tmux.conf")
	desiredPath := filepath.Join(fixture.repoRoot, "desired", "user", "leon", "targets", "tmux", "artifacts", "home.conf")
	currentBody := "CURRENT-TMUX-CONFIG\n"
	changedBody := "CHANGED-TMUX-CONFIG\n"
	writeCLIFile(t, livePath, currentBody)

	payload, stdout, err := runSelectedPreviewCLI(t, []string{"status", "--json", "--user-id", "leon", "tmux:home.conf"})
	require.NoError(t, err)
	require.Equal(t, "status", payload["command"])
	items := payload["items"].([]any)
	require.Len(t, items, 1)
	item := items[0].(map[string]any)
	require.Equal(t, "tmux:home.conf", item["settingRef"])
	require.Equal(t, "desired://user/leon/targets/tmux/artifacts/home.conf", item["desiredUri"])
	resource := item["resource"].(map[string]any)
	require.Equal(t, "file", resource["driverId"])
	require.Equal(t, ".tmux.conf", resource["relPath"])
	recipeInfo := item["recipe"].(map[string]any)
	require.Equal(t, "bundled", recipeInfo["source"])
	require.Equal(t, "recipe://bundled/tmux", recipeInfo["recipeRef"])
	requireNoCLIDiagnosticCode(t, item, v2recipe.TmuxManualReloadWarningCode)
	require.NotContains(t, stdout, currentBody)

	payload, stdout, err = runSelectedPreviewCLI(t, []string{"save", "--dry-run", "--json", "--user-id", "leon", "tmux:home.conf"})
	require.NoError(t, err)
	require.Equal(t, true, payload["dryRun"])
	items = payload["items"].([]any)
	item = items[0].(map[string]any)
	require.Equal(t, "would-promote", item["plannedAction"])
	requireCLIDiagnosticCode(t, item, v2recipe.TmuxManualReloadWarningCode)
	require.NoFileExists(t, desiredPath)
	require.NotContains(t, stdout, currentBody)

	payload, stdout, err = runSelectedPreviewCLI(t, []string{"save", "--yes", "--json", "--user-id", "leon", "tmux:home.conf"})
	require.NoError(t, err)
	require.FileExists(t, desiredPath)
	require.Equal(t, currentBody, string(mustReadCLIFile(t, desiredPath)))
	items = payload["items"].([]any)
	requireCLIDiagnosticCode(t, items[0].(map[string]any), v2recipe.TmuxManualReloadWarningCode)
	require.NotContains(t, stdout, currentBody)

	writeCLIFile(t, livePath, changedBody)
	payload, stdout, err = runSelectedPreviewCLI(t, []string{"diff", "--json", "--user-id", "leon", "tmux:home.conf"})
	require.NoError(t, err)
	items = payload["items"].([]any)
	item = items[0].(map[string]any)
	diffInfo := item["diff"].(map[string]any)
	require.Equal(t, "metadata-only", diffInfo["mode"])
	require.Equal(t, "raw file contents omitted", diffInfo["redaction"])
	requireNoCLIDiagnosticCode(t, item, v2recipe.TmuxManualReloadWarningCode)
	require.NotContains(t, stdout, changedBody)
	require.NotContains(t, stdout, currentBody)

	payload, stdout, err = runSelectedPreviewCLI(t, []string{"apply", "--dry-run", "--json", "--user-id", "leon", "tmux:home.conf"})
	require.NoError(t, err)
	require.Equal(t, true, payload["dryRun"])
	require.Equal(t, changedBody, string(mustReadCLIFile(t, livePath)))
	items = payload["items"].([]any)
	requireCLIDiagnosticCode(t, items[0].(map[string]any), v2recipe.TmuxManualReloadWarningCode)
	require.NotContains(t, stdout, changedBody)
	require.NotContains(t, stdout, currentBody)

	payload, stdout, err = runSelectedPreviewCLI(t, []string{"apply", "--yes", "--json", "--user-id", "leon", "tmux:home.conf"})
	require.NoError(t, err)
	require.Equal(t, currentBody, string(mustReadCLIFile(t, livePath)))
	items = payload["items"].([]any)
	item = items[0].(map[string]any)
	requireCLIDiagnosticCode(t, item, v2recipe.TmuxManualReloadWarningCode)
	mutation := item["mutation"].(map[string]any)
	require.NotEmpty(t, mutation["backupRefs"])
	runID := mutation["runId"].(string)
	stateRoot, stateErr := v2ledger.DefaultStateRoot(fixture.repoRoot)
	require.NoError(t, stateErr)
	ledgerBody := string(mustReadCLIFile(t, filepath.Join(stateRoot, "ledger", "ledger.jsonl")))
	require.NotContains(t, ledgerBody, changedBody)
	require.NotContains(t, ledgerBody, currentBody)
	backupMetadata := string(mustReadCLIFile(t, filepath.Join(stateRoot, "backups", runID, "backup.yaml")))
	require.NotContains(t, backupMetadata, changedBody)
	require.NotContains(t, backupMetadata, currentBody)
	require.NotContains(t, stdout, changedBody)
	require.NotContains(t, stdout, currentBody)
}

func TestV2BundledZshBlockedRefDoesNotReadRawFiles(t *testing.T) {
	fixture := setupCLIV2BundledZshFixture(t, "zshenv")
	setCWD(t, fixture.repoRoot)
	raw := "RAW-ZSHENV-BLOCKED-CONTENT"
	writeCLIFile(t, filepath.Join(fixture.homeDir, ".zshenv"), raw+"\n")

	payload, stdout, err := runSelectedPreviewCLI(t, []string{"status", "--json", "--user-id", "leon", "zsh:zshenv"})
	require.NoError(t, err)
	require.Equal(t, "blocked", payload["summary"].(map[string]any)["status"])
	items := payload["items"].([]any)
	require.Len(t, items, 1)
	item := items[0].(map[string]any)
	require.Equal(t, "unsupported", item["state"])
	requireCLIDiagnosticCode(t, item, v2recipe.ZshBlockedZshenvCode)
	require.NotContains(t, stdout, raw)
}

func TestV2FileResourceStatusDiffSaveApplyEndToEnd(t *testing.T) {
	fixture := setupCLIV2FileResourceFixture(t, false)
	setCWD(t, fixture.repoRoot)
	livePath := filepath.Join(fixture.liveRoot, "config.txt")
	desiredPath := filepath.Join(fixture.repoRoot, "desired", "user", "leon", "targets", "test.files", "artifacts", "config")
	currentBody := "CURRENT-FILE-CONTENT\n"
	changedBody := "CHANGED-FILE-CONTENT\n"
	writeCLIFile(t, livePath, currentBody)

	payload, stdout, err := runSelectedPreviewCLI(t, []string{"status", "--json", "--user-id", "leon", "test.files:config"})
	require.NoError(t, err)
	require.Equal(t, "status", payload["command"])
	items := payload["items"].([]any)
	item := items[0].(map[string]any)
	require.Equal(t, "test.files:config", item["settingRef"])
	require.Equal(t, "desired://user/leon/targets/test.files/artifacts/config", item["desiredUri"])
	require.Equal(t, filepath.ToSlash(filepath.Join("desired", "user", "leon", "targets", "test.files", "artifacts", "config")), item["desiredRelPath"])
	resource := item["resource"].(map[string]any)
	require.Equal(t, "file", resource["driverId"])
	require.NotContains(t, stdout, currentBody)

	payload, stdout, err = runSelectedPreviewCLI(t, []string{"save", "--dry-run", "--json", "--user-id", "leon", "test.files:config"})
	require.NoError(t, err)
	require.Equal(t, true, payload["dryRun"])
	items = payload["items"].([]any)
	item = items[0].(map[string]any)
	require.Equal(t, "would-promote", item["plannedAction"])
	require.NoFileExists(t, desiredPath)
	require.NotContains(t, stdout, currentBody)

	_, stdout, err = runSelectedPreviewCLI(t, []string{"save", "--yes", "--json", "--user-id", "leon", "test.files:config"})
	require.NoError(t, err)
	require.FileExists(t, desiredPath)
	require.Equal(t, currentBody, string(mustReadCLIFile(t, desiredPath)))
	require.NotContains(t, stdout, currentBody)

	writeCLIFile(t, livePath, changedBody)
	payload, stdout, err = runSelectedPreviewCLI(t, []string{"diff", "--json", "--user-id", "leon", "test.files:config"})
	require.NoError(t, err)
	items = payload["items"].([]any)
	diffInfo := items[0].(map[string]any)["diff"].(map[string]any)
	require.Equal(t, "metadata-only", diffInfo["mode"])
	require.Equal(t, "raw file contents omitted", diffInfo["redaction"])
	require.NotContains(t, stdout, changedBody)
	require.NotContains(t, stdout, currentBody)

	payload, stdout, err = runSelectedPreviewCLI(t, []string{"apply", "--dry-run", "--json", "--user-id", "leon", "test.files:config"})
	require.NoError(t, err)
	require.Equal(t, true, payload["dryRun"])
	require.Equal(t, changedBody, string(mustReadCLIFile(t, livePath)))
	require.NotContains(t, stdout, changedBody)
	require.NotContains(t, stdout, currentBody)

	payload, stdout, err = runSelectedPreviewCLI(t, []string{"apply", "--yes", "--json", "--user-id", "leon", "test.files:config"})
	require.NoError(t, err)
	require.Equal(t, currentBody, string(mustReadCLIFile(t, livePath)))
	items = payload["items"].([]any)
	mutation := items[0].(map[string]any)["mutation"].(map[string]any)
	require.NotEmpty(t, mutation["backupRefs"])
	runID := mutation["runId"].(string)
	stateRoot, stateErr := v2ledger.DefaultStateRoot(fixture.repoRoot)
	require.NoError(t, stateErr)
	ledgerBody := string(mustReadCLIFile(t, filepath.Join(stateRoot, "ledger", "ledger.jsonl")))
	require.NotContains(t, ledgerBody, changedBody)
	require.NotContains(t, ledgerBody, currentBody)
	backupMetadata := string(mustReadCLIFile(t, filepath.Join(stateRoot, "backups", runID, "backup.yaml")))
	require.NotContains(t, backupMetadata, changedBody)
	require.NotContains(t, backupMetadata, currentBody)
	require.NotContains(t, stdout, changedBody)
	require.NotContains(t, stdout, currentBody)
}

func TestV2FileResourceMissingFilesBlockDeleteSemantics(t *testing.T) {
	t.Run("missing live blocks save and preserves desired", func(t *testing.T) {
		fixture := setupCLIV2FileResourceFixture(t, false)
		setCWD(t, fixture.repoRoot)
		desiredPath := filepath.Join(fixture.repoRoot, "desired", "user", "leon", "targets", "test.files", "artifacts", "config")
		writeCLIFile(t, desiredPath, "DESIRED-STAYS\n")

		payload, stdout, err := runSelectedPreviewCLI(t, []string{"save", "--dry-run", "--json", "--user-id", "leon", "test.files:config"})
		require.NoError(t, err)
		require.Equal(t, "blocked", payload["summary"].(map[string]any)["status"])
		require.Equal(t, "DESIRED-STAYS\n", string(mustReadCLIFile(t, desiredPath)))
		require.NotContains(t, stdout, "DESIRED-STAYS")

		payload, stdout, err = runSelectedPreviewCLI(t, []string{"save", "--yes", "--json", "--user-id", "leon", "test.files:config"})
		require.Error(t, err)
		require.Equal(t, "selectedlive.planBlocked", payload["error"].(map[string]any)["code"])
		require.Equal(t, "DESIRED-STAYS\n", string(mustReadCLIFile(t, desiredPath)))
		require.NotContains(t, stdout, "DESIRED-STAYS")
	})

	t.Run("missing desired blocks apply and preserves live without backup", func(t *testing.T) {
		fixture := setupCLIV2FileResourceFixture(t, false)
		setCWD(t, fixture.repoRoot)
		livePath := filepath.Join(fixture.liveRoot, "config.txt")
		writeCLIFile(t, livePath, "LIVE-STAYS\n")
		stateRoot, stateErr := v2ledger.DefaultStateRoot(fixture.repoRoot)
		require.NoError(t, stateErr)

		payload, stdout, err := runSelectedPreviewCLI(t, []string{"apply", "--dry-run", "--json", "--user-id", "leon", "test.files:config"})
		require.NoError(t, err)
		require.Equal(t, "blocked", payload["summary"].(map[string]any)["status"])
		require.Equal(t, "LIVE-STAYS\n", string(mustReadCLIFile(t, livePath)))
		require.NotContains(t, stdout, "LIVE-STAYS")

		payload, stdout, err = runSelectedPreviewCLI(t, []string{"apply", "--yes", "--json", "--user-id", "leon", "test.files:config"})
		require.Error(t, err)
		require.Equal(t, "selectedlive.planBlocked", payload["error"].(map[string]any)["code"])
		require.Equal(t, "LIVE-STAYS\n", string(mustReadCLIFile(t, livePath)))
		require.NoDirExists(t, filepath.Join(stateRoot, "backups"))
		require.NotContains(t, stdout, "LIVE-STAYS")
	})
}

func TestV2FileResourceExplicitArtifactBinding(t *testing.T) {
	fixture := setupCLIV2FileResourceFixture(t, true)
	setCWD(t, fixture.repoRoot)
	writeCLIFile(t, filepath.Join(fixture.liveRoot, "config.txt"), "EXPLICIT-BINDING-LIVE\n")

	payload, stdout, err := runSelectedPreviewCLI(t, []string{"save", "--dry-run", "--json", "--user-id", "leon", "test.files:config"})
	require.NoError(t, err)
	items := payload["items"].([]any)
	item := items[0].(map[string]any)
	require.Equal(t, "desired://user/leon/targets/test.files/artifacts/config", item["desiredUri"])
	require.Equal(t, "would-promote", item["plannedAction"])
	require.NotContains(t, stdout, "EXPLICIT-BINDING-LIVE")
}

type cliV2SelectedPreviewFixture struct {
	repoRoot string
	liveRoot string
	homeDir  string
}

func setupCLIV2SelectedPreviewFixture(t *testing.T, trusted bool, withDesired bool) cliV2SelectedPreviewFixture {
	t.Helper()
	homeDir := setTempHome(t)
	repoRoot := t.TempDir()
	liveRoot := t.TempDir()

	writeCLIFile(t, filepath.Join(repoRoot, "dotfiles-manager.v2.yaml"), "schema: dotfiles-manager.v2.root-config\nschemaVersion: 1\nactiveProfileStack: default\n")
	writeCLIFile(t, filepath.Join(repoRoot, "profiles", "stacks", "default.yaml"), "schema: dotfiles-manager.v2.profile-stack\nschemaVersion: 1\nprofileStack: [global]\n")
	writeCLIFile(t, filepath.Join(repoRoot, "profiles", "layers", "global.yaml"), "schema: dotfiles-manager.v2.profile-layer\nschemaVersion: 1\nselections:\n  test.app:\n    settings:\n      identity.email:\n        scope: user\n")
	writeCLIFile(t, filepath.Join(repoRoot, "recipes", "local", "test.app", "recipe.yaml"), cliSelectedPreviewRecipeBody(liveRoot))
	writeCLIFile(t, filepath.Join(liveRoot, "config.yaml"), "user:\n  email: current@example.com\n")
	if withDesired {
		writeCLIFile(t, filepath.Join(repoRoot, "desired", "user", "leon", "targets", "test.app", "settings.yaml"), "schema: dotfiles-manager.v2.desired-settings\nschemaVersion: 1\nvalues:\n  identity.email:\n    intent: set\n    kind: string\n    value: desired@example.com\n")
	}
	if trusted {
		rec, err := v2recipe.LoadLocal(repoRoot, "test.app")
		require.NoError(t, err)
		stateRoot, err := v2ledger.DefaultStateRoot(repoRoot)
		require.NoError(t, err)
		_, err = v2recipe.RecordLocalRecipeTrust(repoRoot, stateRoot, rec)
		require.NoError(t, err)
	}
	return cliV2SelectedPreviewFixture{repoRoot: repoRoot, liveRoot: liveRoot, homeDir: homeDir}
}

func setupCLIV2BundledGitFixture(t *testing.T) cliV2SelectedPreviewFixture {
	t.Helper()
	homeDir := setTempHome(t)
	repoRoot := t.TempDir()
	writeCLIFile(t, filepath.Join(repoRoot, "dotfiles-manager.v2.yaml"), "schema: dotfiles-manager.v2.root-config\nschemaVersion: 1\nactiveProfileStack: default\n")
	writeCLIFile(t, filepath.Join(repoRoot, "profiles", "stacks", "default.yaml"), "schema: dotfiles-manager.v2.profile-stack\nschemaVersion: 1\nprofileStack: [global]\n")
	writeCLIFile(t, filepath.Join(repoRoot, "profiles", "layers", "global.yaml"), "schema: dotfiles-manager.v2.profile-layer\nschemaVersion: 1\nselections:\n  git:\n    settings:\n      user.email:\n        scope: user\n")
	return cliV2SelectedPreviewFixture{repoRoot: repoRoot, homeDir: homeDir}
}

func setupCLIV2BundledStarshipFixture(t *testing.T, settingID string) cliV2SelectedPreviewFixture {
	t.Helper()
	homeDir := setTempHome(t)
	repoRoot := t.TempDir()
	writeCLIFile(t, filepath.Join(repoRoot, "dotfiles-manager.v2.yaml"), "schema: dotfiles-manager.v2.root-config\nschemaVersion: 1\nactiveProfileStack: default\n")
	writeCLIFile(t, filepath.Join(repoRoot, "profiles", "stacks", "default.yaml"), "schema: dotfiles-manager.v2.profile-stack\nschemaVersion: 1\nprofileStack: [global]\n")
	writeCLIFile(t, filepath.Join(repoRoot, "profiles", "layers", "global.yaml"), "schema: dotfiles-manager.v2.profile-layer\nschemaVersion: 1\nselections:\n  starship:\n    settings:\n      "+settingID+":\n        scope: user\n")
	return cliV2SelectedPreviewFixture{repoRoot: repoRoot, homeDir: homeDir}
}

func setupCLIV2BundledZshFixture(t *testing.T, settingID string) cliV2SelectedPreviewFixture {
	t.Helper()
	homeDir := setTempHome(t)
	repoRoot := t.TempDir()
	writeCLIFile(t, filepath.Join(repoRoot, "dotfiles-manager.v2.yaml"), "schema: dotfiles-manager.v2.root-config\nschemaVersion: 1\nactiveProfileStack: default\n")
	writeCLIFile(t, filepath.Join(repoRoot, "profiles", "stacks", "default.yaml"), "schema: dotfiles-manager.v2.profile-stack\nschemaVersion: 1\nprofileStack: [global]\n")
	writeCLIFile(t, filepath.Join(repoRoot, "profiles", "layers", "global.yaml"), "schema: dotfiles-manager.v2.profile-layer\nschemaVersion: 1\nselections:\n  zsh:\n    settings:\n      "+settingID+":\n        scope: user\n")
	return cliV2SelectedPreviewFixture{repoRoot: repoRoot, homeDir: homeDir}
}

func setupCLIV2BundledTmuxFixture(t *testing.T, settingID string) cliV2SelectedPreviewFixture {
	t.Helper()
	homeDir := setTempHome(t)
	repoRoot := t.TempDir()
	writeCLIFile(t, filepath.Join(repoRoot, "dotfiles-manager.v2.yaml"), "schema: dotfiles-manager.v2.root-config\nschemaVersion: 1\nactiveProfileStack: default\n")
	writeCLIFile(t, filepath.Join(repoRoot, "profiles", "stacks", "default.yaml"), "schema: dotfiles-manager.v2.profile-stack\nschemaVersion: 1\nprofileStack: [global]\n")
	writeCLIFile(t, filepath.Join(repoRoot, "profiles", "layers", "global.yaml"), "schema: dotfiles-manager.v2.profile-layer\nschemaVersion: 1\nselections:\n  tmux:\n    settings:\n      "+settingID+":\n        scope: user\n")
	return cliV2SelectedPreviewFixture{repoRoot: repoRoot, homeDir: homeDir}
}

func setupCLIV2FileResourceFixture(t *testing.T, explicitArtifact bool) cliV2SelectedPreviewFixture {
	t.Helper()
	homeDir := setTempHome(t)
	repoRoot := t.TempDir()
	liveRoot := t.TempDir()
	artifactLine := ""
	if explicitArtifact {
		artifactLine = "        artifact: artifacts/config\n"
	}
	writeCLIFile(t, filepath.Join(repoRoot, "dotfiles-manager.v2.yaml"), "schema: dotfiles-manager.v2.root-config\nschemaVersion: 1\nactiveProfileStack: default\n")
	writeCLIFile(t, filepath.Join(repoRoot, "profiles", "stacks", "default.yaml"), "schema: dotfiles-manager.v2.profile-stack\nschemaVersion: 1\nprofileStack: [global]\n")
	writeCLIFile(t, filepath.Join(repoRoot, "profiles", "layers", "global.yaml"), "schema: dotfiles-manager.v2.profile-layer\nschemaVersion: 1\nselections:\n  test.files:\n    settings:\n      config:\n        scope: user\n"+artifactLine)
	writeCLIFile(t, filepath.Join(repoRoot, "recipes", "local", "test.files", "recipe.yaml"), `schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: test.files
displayName: Test files
supportLevel: experimental
capability: read-write
locations:
  config:
    default: `+liveRoot+`
settings:
  config:
    label: Config file
    supportLevel: experimental
    capability: read-write
    artifactForm: file
    sensitivity: personal
    redaction: redacted-for-display
    lifecycle: allowed
    scopeDefault: user
    resource: config-file
resources:
  config-file:
    driver: file
    location: config
    path: config.txt
    capability: read-write
    sensitivity: personal
    redaction: redacted-for-display
    lifecycle: allowed
`)
	rec, err := v2recipe.LoadLocal(repoRoot, "test.files")
	require.NoError(t, err)
	stateRoot, err := v2ledger.DefaultStateRoot(repoRoot)
	require.NoError(t, err)
	_, err = v2recipe.RecordLocalRecipeTrust(repoRoot, stateRoot, rec)
	require.NoError(t, err)
	return cliV2SelectedPreviewFixture{repoRoot: repoRoot, liveRoot: liveRoot, homeDir: homeDir}
}

func cliSelectedPreviewRecipeBody(liveRoot string) string {
	return `schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: test.app
displayName: Test App
supportLevel: experimental
capability: read-write
locations:
  config:
    default: ` + liveRoot + `
settings:
  identity.email:
    label: User email
    supportLevel: experimental
    capability: read-write
    artifactForm: scalar
    sensitivity: personal
    redaction: redacted-for-display
    lifecycle: allowed
    scopeDefault: user
    resource: config-email
resources:
  config-email:
    driver: yaml-file
    location: config
    path: config.yaml
    capability: read-write
    sensitivity: personal
    redaction: redacted-for-display
    lifecycle: allowed
    selector:
      path: [user, email]
      createMissing: create
      duplicatePolicy: reject
      deleteKey: allow
`
}

func runSelectedPreviewCLI(t *testing.T, args []string) (map[string]any, string, error) {
	t.Helper()
	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	if stdout.Len() == 0 {
		return nil, stdout.String(), err
	}
	var payload map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload), "stdout=%s stderr=%s", stdout.String(), stderr.String())
	return payload, stdout.String(), err
}

func requireCLIDiagnosticCode(t *testing.T, item map[string]any, code string) {
	t.Helper()
	diagnostics, ok := item["diagnostics"].([]any)
	require.True(t, ok, "diagnostics missing from %#v", item)
	for _, raw := range diagnostics {
		diagnostic := raw.(map[string]any)
		if diagnostic["code"] == code {
			return
		}
	}
	require.Failf(t, "missing diagnostic", "wanted %s in %#v", code, diagnostics)
}

func requireNoCLIDiagnosticCode(t *testing.T, item map[string]any, code string) {
	t.Helper()
	diagnostics, ok := item["diagnostics"].([]any)
	if !ok {
		return
	}
	for _, raw := range diagnostics {
		diagnostic := raw.(map[string]any)
		require.NotEqual(t, code, diagnostic["code"], "unexpected diagnostic in %#v", diagnostics)
	}
}

func writeCLIFile(t *testing.T, path string, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func mustReadCLIFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}
