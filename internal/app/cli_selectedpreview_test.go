package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
	require.Equal(t, "save", payload["command"])
	require.Equal(t, "sync", payload["operation"])
	require.Equal(t, "save", payload["invokedCommand"])
	require.Equal(t, "live_to_stored", payload["direction"])
	errorObj := payload["error"].(map[string]any)
	require.Equal(t, "selectedlive.confirmationRequired", errorObj["code"])
	require.NoFileExists(t, desiredPath)
	require.NotContains(t, stdout, "current@example.com")

	payload, stdout, err = runSelectedPreviewCLI(t, []string{"save", "--yes", "--json", "--user-id", "leon", "test.app:identity.email"})
	require.NoError(t, err)
	require.Equal(t, "save", payload["command"])
	require.Equal(t, "sync", payload["operation"])
	require.Equal(t, "save", payload["invokedCommand"])
	require.Equal(t, "live_to_stored", payload["direction"])
	require.Equal(t, false, payload["dryRun"])
	require.FileExists(t, desiredPath)
	require.Contains(t, string(mustReadCLIFile(t, desiredPath)), "current@example.com")
	require.NotContains(t, stdout, "current@example.com")

	writeCLIFile(t, filepath.Join(fixture.liveRoot, "config.yaml"), "user:\n  email: changed-live@example.com\n")

	payload, stdout, err = runSelectedPreviewCLI(t, []string{"apply", "--json", "--user-id", "leon", "test.app:identity.email"})
	require.Error(t, err)
	require.Equal(t, "apply", payload["command"])
	require.Equal(t, "sync", payload["operation"])
	require.Equal(t, "apply", payload["invokedCommand"])
	require.Equal(t, "stored_to_live", payload["direction"])
	errorObj = payload["error"].(map[string]any)
	require.Equal(t, "selectedlive.confirmationRequired", errorObj["code"])
	require.Contains(t, string(mustReadCLIFile(t, filepath.Join(fixture.liveRoot, "config.yaml"))), "changed-live@example.com")
	require.NotContains(t, stdout, "current@example.com")
	require.NotContains(t, stdout, "changed-live@example.com")

	payload, stdout, err = runSelectedPreviewCLI(t, []string{"apply", "--non-interactive", "--json", "--user-id", "leon", "test.app:identity.email"})
	require.Error(t, err)
	require.Equal(t, "apply", payload["command"])
	require.Equal(t, "sync", payload["operation"])
	require.Equal(t, "apply", payload["invokedCommand"])
	require.Equal(t, "stored_to_live", payload["direction"])
	errorObj = payload["error"].(map[string]any)
	require.Equal(t, "selectedlive.confirmationRequired", errorObj["code"])
	require.Contains(t, string(mustReadCLIFile(t, filepath.Join(fixture.liveRoot, "config.yaml"))), "changed-live@example.com")
	require.NotContains(t, stdout, "current@example.com")
	require.NotContains(t, stdout, "changed-live@example.com")

	payload, stdout, err = runSelectedPreviewCLI(t, []string{"apply", "--yes", "--json", "--user-id", "leon", "test.app:identity.email"})
	require.NoError(t, err)
	require.Equal(t, "apply", payload["command"])
	require.Equal(t, "sync", payload["operation"])
	require.Equal(t, "apply", payload["invokedCommand"])
	require.Equal(t, "stored_to_live", payload["direction"])
	require.Contains(t, string(mustReadCLIFile(t, filepath.Join(fixture.liveRoot, "config.yaml"))), "current@example.com")
	require.NotContains(t, stdout, "current@example.com")
	require.NotContains(t, stdout, "changed-live@example.com")
}

func TestV2BundledGitSelectedSettingBlockedAndMissingDefaultText(t *testing.T) {
	forbidden := []string{"state=", "action=", "missing-desired", "missing-current", "would-promote", "resource=", "driver=", "selector=", "desired://", "state://"}

	t.Run("apply missing desired is plain-language blocked", func(t *testing.T) {
		fixture := setupCLIV2BundledGitFixture(t)
		setCWD(t, fixture.repoRoot)
		writeCLIFile(t, filepath.Join(fixture.homeDir, ".gitconfig"), "[user]\n\temail = current@example.com\n")

		stdout, _, err := runSelectedPreviewTextCLI(t, []string{"apply", "--dry-run", "--user-id", "leon", "git:user.email"})
		require.NoError(t, err)
		require.Contains(t, stdout, "Cannot apply Git user email yet")
		require.Contains(t, stdout, "Blocked because no stored settings exist yet")
		require.Contains(t, stdout, "No files changed")
		require.Contains(t, stdout, "dotfiles-manager --config dotfiles-manager.v2.yaml save --dry-run --user-id leon git:user.email")
		require.NotContains(t, stdout, "Would update live file")
		require.NotContains(t, stdout, "current@example.com")
		for _, token := range forbidden {
			require.NotContains(t, stdout, token)
		}
	})

	t.Run("save missing live value stays readable and non-technical", func(t *testing.T) {
		fixture := setupCLIV2BundledGitFixture(t)
		setCWD(t, fixture.repoRoot)

		stdout, _, err := runSelectedPreviewTextCLI(t, []string{"save", "--dry-run", "--user-id", "leon", "git:user.email"})
		require.NoError(t, err)
		require.Contains(t, stdout, "Dry run: would sync Git user email to stored settings")
		require.Contains(t, stdout, "No live value found")
		require.Contains(t, stdout, "desired/user/leon/targets/git/settings.yaml")
		require.Contains(t, stdout, "No files changed")
		for _, token := range forbidden {
			require.NotContains(t, stdout, token)
		}
	})
}

func TestV2BundledGitSelectedSettingReadableTranscript(t *testing.T) {
	fixture := setupCLIV2BundledGitFixture(t)
	setCWD(t, fixture.repoRoot)
	helperSecret := "credential-helper-secret"
	writeCLIFile(t, filepath.Join(fixture.homeDir, ".gitconfig"), "[credential]\n\thelper = "+helperSecret+"\n[user]\n\temail = current@example.com\n")

	forbidden := []string{"state=", "action=", "resource=", "driver=", "selector=", "desired://", "state://", "no-baseline", "current@example.com", "changed@example.com", helperSecret}
	requireReadableDefaultOutput := func(t *testing.T, stdout string) {
		t.Helper()
		for _, token := range forbidden {
			require.NotContains(t, stdout, token)
		}
	}

	statusOut, _, err := runSelectedPreviewTextCLI(t, []string{"status", "--user-id", "leon", "git:user.email"})
	require.NoError(t, err)
	require.Contains(t, statusOut, "Selected, but not stored in the settings folder yet.")
	require.Contains(t, statusOut, "No files changed.")
	require.Contains(t, statusOut, "dotfiles-manager --config dotfiles-manager.v2.yaml save --dry-run --user-id leon git:user.email")
	requireReadableDefaultOutput(t, statusOut)

	saveDryRunOut, _, err := runSelectedPreviewTextCLI(t, []string{"save", "--dry-run", "--user-id", "leon", "git:user.email"})
	require.NoError(t, err)
	require.Contains(t, saveDryRunOut, "Dry run: would sync Git user email to stored settings.")
	require.Contains(t, saveDryRunOut, "No files changed.")
	require.Contains(t, saveDryRunOut, "dotfiles-manager --config dotfiles-manager.v2.yaml save --yes --user-id leon git:user.email")
	requireReadableDefaultOutput(t, saveDryRunOut)

	saveYesOut, _, err := runSelectedPreviewTextCLI(t, []string{"save", "--yes", "--user-id", "leon", "git:user.email"})
	require.NoError(t, err)
	require.Contains(t, saveYesOut, "Synced Git user email to stored settings.")
	require.Contains(t, saveYesOut, "No live Git config was changed.")
	require.Contains(t, saveYesOut, "Stored settings changed.")
	require.Contains(t, saveYesOut, "dotfiles-manager --config dotfiles-manager.v2.yaml diff --user-id leon git:user.email")
	requireReadableDefaultOutput(t, saveYesOut)

	writeCLIFile(t, filepath.Join(fixture.homeDir, ".gitconfig"), "[credential]\n\thelper = "+helperSecret+"\n[user]\n\temail = changed@example.com\n")

	diffOut, _, err := runSelectedPreviewTextCLI(t, []string{"diff", "--user-id", "leon", "git:user.email"})
	require.NoError(t, err)
	require.Contains(t, diffOut, "Git user email differs between live settings and stored settings.")
	require.Contains(t, diffOut, "Reason:\n  Live settings differ from stored settings. The previous apply recorded by this tool matches stored settings.")
	require.NotContains(t, diffOut, "This setting has not previously been applied by this tool; review before confirming.")
	require.NotContains(t, diffOut, "Review note:")
	require.Contains(t, diffOut, "No files changed.")
	require.Contains(t, diffOut, "dotfiles-manager --config dotfiles-manager.v2.yaml sync --user-id leon git:user.email")
	requireReadableDefaultOutput(t, diffOut)

	applyDryRunOut, _, err := runSelectedPreviewTextCLI(t, []string{"apply", "--dry-run", "--user-id", "leon", "git:user.email"})
	require.NoError(t, err)
	require.Contains(t, applyDryRunOut, "Dry run: would sync Git user email to live settings.")
	require.NotContains(t, applyDryRunOut, "backup")
	require.NotContains(t, applyDryRunOut, "restore")
	require.Contains(t, applyDryRunOut, "No files changed.")
	require.Contains(t, applyDryRunOut, "dotfiles-manager --config dotfiles-manager.v2.yaml apply --yes --user-id leon git:user.email")
	requireReadableDefaultOutput(t, applyDryRunOut)

	applyYesOut, _, err := runSelectedPreviewTextCLI(t, []string{"apply", "--yes", "--user-id", "leon", "git:user.email"})
	require.NoError(t, err)
	require.Contains(t, applyYesOut, "Synced Git user email to live settings.")
	require.Contains(t, applyYesOut, "Live files changed.")
	require.NotContains(t, applyYesOut, "backup")
	require.NotContains(t, applyYesOut, "restore")
	require.NotContains(t, applyYesOut, "Review the paths before confirming")
	requireReadableDefaultOutput(t, applyYesOut)
}

func TestV2BundledGitSelectedSettingVerboseTextAndJSONContract(t *testing.T) {
	fixture := setupCLIV2BundledGitFixture(t)
	setCWD(t, fixture.repoRoot)
	helperSecret := "credential-helper-secret"
	writeCLIFile(t, filepath.Join(fixture.homeDir, ".gitconfig"), "[credential]\n\thelper = "+helperSecret+"\n[user]\n\temail = current@example.com\n")

	statusDefault, _, err := runSelectedPreviewTextCLI(t, []string{"status", "--user-id", "leon", "git:user.email"})
	require.NoError(t, err)
	require.Contains(t, statusDefault, "Git user email")
	require.Contains(t, statusDefault, "Selected, but not stored in the settings folder yet")
	require.Contains(t, statusDefault, "$HOME/.gitconfig [user] email")
	require.Contains(t, statusDefault, "Value hidden for safety")
	require.Contains(t, statusDefault, "No files changed")
	require.NotContains(t, statusDefault, "resource=")
	require.NotContains(t, statusDefault, "desired://")
	require.NotContains(t, statusDefault, "state=")
	require.NotContains(t, statusDefault, "action=")
	require.NotContains(t, statusDefault, "no-baseline")
	require.NotContains(t, statusDefault, "current@example.com")
	require.NotContains(t, statusDefault, helperSecret)

	statusVerbose, _, err := runSelectedPreviewTextCLI(t, []string{"status", "--verbose", "--user-id", "leon", "git:user.email"})
	require.NoError(t, err)
	require.Contains(t, statusVerbose, "Selected, but not stored in the settings folder yet")
	require.Contains(t, statusVerbose, "Technical details:")
	require.Contains(t, statusVerbose, "resource=user-email")
	require.Contains(t, statusVerbose, "driver=ini-file")
	require.Contains(t, statusVerbose, "selector=[user] email")
	require.Contains(t, statusVerbose, "desired://user/leon/targets/git/settings#user.email")
	require.Contains(t, statusVerbose, "state=missing-desired")
	require.NotContains(t, statusVerbose, "current@example.com")
	require.NotContains(t, statusVerbose, helperSecret)

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "save dry-run", args: []string{"save", "--dry-run", "--verbose", "--user-id", "leon", "git:user.email"}, want: "Dry run: would sync Git user email to stored settings"},
		{name: "save yes", args: []string{"save", "--yes", "--verbose", "--user-id", "leon", "git:user.email"}, want: "Synced Git user email to stored settings"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, _, err := runSelectedPreviewTextCLI(t, tc.args)
			require.NoError(t, err)
			require.Contains(t, stdout, tc.want)
			require.Contains(t, stdout, "Technical details:")
			require.NotContains(t, stdout, "current@example.com")
			require.NotContains(t, stdout, helperSecret)
		})
	}

	writeCLIFile(t, filepath.Join(fixture.homeDir, ".gitconfig"), "[credential]\n\thelper = "+helperSecret+"\n[user]\n\temail = changed@example.com\n")

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "diff", args: []string{"diff", "--verbose", "--user-id", "leon", "git:user.email"}, want: "Git user email differs between live settings and stored settings"},
		{name: "apply dry-run", args: []string{"apply", "--dry-run", "--verbose", "--user-id", "leon", "git:user.email"}, want: "Dry run: would sync Git user email to live settings"},
		{name: "apply yes", args: []string{"apply", "--yes", "--verbose", "--user-id", "leon", "git:user.email"}, want: "Synced Git user email to live settings"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, _, err := runSelectedPreviewTextCLI(t, tc.args)
			require.NoError(t, err)
			require.Contains(t, stdout, tc.want)
			require.Contains(t, stdout, "Technical details:")
			require.NotContains(t, stdout, "changed@example.com")
			require.NotContains(t, stdout, "current@example.com")
			require.NotContains(t, stdout, helperSecret)
		})
		if tc.name == "apply yes" {
			writeCLIFile(t, filepath.Join(fixture.homeDir, ".gitconfig"), "[credential]\n\thelper = "+helperSecret+"\n[user]\n\temail = changed-again@example.com\n")
		}
	}
}

func TestV2SelectedPreviewJSONVerboseDoesNotChangeJSONContract(t *testing.T) {
	fixture := setupCLIV2BundledGitFixture(t)
	setCWD(t, fixture.repoRoot)
	writeCLIFile(t, filepath.Join(fixture.homeDir, ".gitconfig"), "[user]\n\temail = current@example.com\n")

	statusPayload, _, err := runSelectedPreviewCLI(t, []string{"status", "--json", "--user-id", "leon", "git:user.email"})
	require.NoError(t, err)
	statusVerbosePayload, statusVerboseStdout, err := runSelectedPreviewCLI(t, []string{"status", "--json", "--verbose", "--user-id", "leon", "git:user.email"})
	require.NoError(t, err)
	require.Equal(t, statusPayload, statusVerbosePayload)
	require.NotContains(t, statusVerboseStdout, "Technical details")
	require.NotContains(t, statusVerboseStdout, "Selected, but not saved")

	savePayload, _, err := runSelectedPreviewCLI(t, []string{"save", "--dry-run", "--json", "--user-id", "leon", "git:user.email"})
	require.NoError(t, err)
	saveVerbosePayload, saveVerboseStdout, err := runSelectedPreviewCLI(t, []string{"save", "--dry-run", "--json", "--verbose", "--user-id", "leon", "git:user.email"})
	require.NoError(t, err)
	require.Equal(t, savePayload, saveVerbosePayload)
	require.NotContains(t, saveVerboseStdout, "Technical details")
	require.NotContains(t, saveVerboseStdout, "Dry run: would save")
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
	require.Equal(t, "sync", payload["operation"])
	require.Equal(t, "save", payload["invokedCommand"])
	require.Equal(t, "live_to_stored", payload["direction"])
	require.Equal(t, true, payload["dryRun"])
	summary := payload["summary"].(map[string]any)
	require.Equal(t, "changed", summary["status"])
	require.Equal(t, float64(1), summary["saved"])
	items = payload["items"].([]any)
	item = items[0].(map[string]any)
	require.Equal(t, "would-promote", item["plannedAction"])
	require.Contains(t, item["message"], "synced to stored settings")
	require.NoFileExists(t, desiredPath)
	require.NotContains(t, stdout, "current@example.com")
	require.NotContains(t, stdout, helperSecret)

	payload, stdout, err = runSelectedPreviewCLI(t, []string{"save", "--yes", "--json", "--user-id", "leon", "git:user.email"})
	require.NoError(t, err)
	require.Equal(t, "save", payload["command"])
	require.Equal(t, "sync", payload["operation"])
	require.Equal(t, "save", payload["invokedCommand"])
	require.Equal(t, "live_to_stored", payload["direction"])
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
	require.Equal(t, "sync", payload["operation"])
	require.Equal(t, "apply", payload["invokedCommand"])
	require.Equal(t, "stored_to_live", payload["direction"])
	require.Equal(t, true, payload["dryRun"])
	require.Contains(t, string(mustReadCLIFile(t, filepath.Join(fixture.homeDir, ".gitconfig"))), "changed@example.com")
	require.NotContains(t, stdout, "changed@example.com")
	require.NotContains(t, stdout, "current@example.com")

	payload, stdout, err = runSelectedPreviewCLI(t, []string{"apply", "--yes", "--json", "--user-id", "leon", "git:user.email"})
	require.NoError(t, err)
	require.Equal(t, "sync", payload["operation"])
	require.Equal(t, "apply", payload["invokedCommand"])
	require.Equal(t, "stored_to_live", payload["direction"])
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
	require.NotContains(t, mutation, "backupRefs")
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
	require.NotContains(t, mutation, "backupRefs")
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
	require.NotContains(t, mutation, "backupRefs")
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

func TestV2FileTreeApplyReportsRemovalPathsBeforeAndAfterConfirmation(t *testing.T) {
	fixture := setupCLIV2FileTreeResourceFixture(t)
	setCWD(t, fixture.repoRoot)
	liveTree := filepath.Join(fixture.liveRoot, "profiles")
	desiredTree := filepath.Join(fixture.repoRoot, "desired", "user", "leon", "targets", "test.tree", "artifacts", "config")
	currentBody := "CURRENT-TREE-CONTENT\n"
	desiredBody := "DESIRED-TREE-CONTENT\n"
	extraBody := "EXTRA-LIVE-TREE-CONTENT\n"
	writeCLIFile(t, filepath.Join(liveTree, "init.lua"), currentBody)
	writeCLIFile(t, filepath.Join(liveTree, "lua", "extra.lua"), extraBody)
	writeCLIFile(t, filepath.Join(desiredTree, "init.lua"), desiredBody)

	stdout, _, err := runSelectedPreviewTextCLI(t, []string{"apply", "--dry-run", "--user-id", "leon", "test.tree:config"})
	require.NoError(t, err)
	require.Contains(t, stdout, "Will remove live paths not present in stored settings")
	require.Contains(t, stdout, "lua/extra.lua")
	require.Equal(t, extraBody, string(mustReadCLIFile(t, filepath.Join(liveTree, "lua", "extra.lua"))))
	require.NotContains(t, stdout, currentBody)
	require.NotContains(t, stdout, desiredBody)
	require.NotContains(t, stdout, extraBody)

	payload, jsonStdout, err := runSelectedPreviewCLI(t, []string{"apply", "--dry-run", "--json", "--user-id", "leon", "test.tree:config"})
	require.NoError(t, err)
	items := payload["items"].([]any)
	item := items[0].(map[string]any)
	fileTree := item["fileTree"].(map[string]any)
	operations := fileTree["operations"].([]any)
	requireFileTreeOperation(t, operations, "remove", "lua/extra.lua", "file", "planned")
	requireFileTreeOperation(t, operations, "update", "init.lua", "file", "planned")
	require.NotContains(t, jsonStdout, currentBody)
	require.NotContains(t, jsonStdout, desiredBody)
	require.NotContains(t, jsonStdout, extraBody)

	stdout, _, err = runSelectedPreviewTextCLI(t, []string{"apply", "--yes", "--user-id", "leon", "test.tree:config"})
	require.NoError(t, err)
	require.Contains(t, stdout, "Removed live paths not present in stored settings")
	require.Contains(t, stdout, "lua/extra.lua")
	require.NoFileExists(t, filepath.Join(liveTree, "lua", "extra.lua"))
	require.Equal(t, desiredBody, string(mustReadCLIFile(t, filepath.Join(liveTree, "init.lua"))))
	require.NotContains(t, stdout, currentBody)
	require.NotContains(t, stdout, desiredBody)
	require.NotContains(t, stdout, extraBody)
}

func TestV2FileTreeApplyJSONReportsAppliedRemovalOperations(t *testing.T) {
	fixture := setupCLIV2FileTreeResourceFixture(t)
	setCWD(t, fixture.repoRoot)
	liveTree := filepath.Join(fixture.liveRoot, "profiles")
	desiredTree := filepath.Join(fixture.repoRoot, "desired", "user", "leon", "targets", "test.tree", "artifacts", "config")
	writeCLIFile(t, filepath.Join(liveTree, "init.lua"), "CURRENT-TREE-CONTENT\n")
	writeCLIFile(t, filepath.Join(liveTree, "lua", "extra.lua"), "EXTRA-LIVE-TREE-CONTENT\n")
	writeCLIFile(t, filepath.Join(desiredTree, "init.lua"), "DESIRED-TREE-CONTENT\n")

	payload, stdout, err := runSelectedPreviewCLI(t, []string{"apply", "--yes", "--json", "--user-id", "leon", "test.tree:config"})
	require.NoError(t, err)
	items := payload["items"].([]any)
	item := items[0].(map[string]any)
	fileTree := item["fileTree"].(map[string]any)
	operations := fileTree["operations"].([]any)
	requireFileTreeOperation(t, operations, "remove", "lua/extra.lua", "file", "applied")
	requireFileTreeOperation(t, operations, "update", "init.lua", "file", "applied")
	require.NoFileExists(t, filepath.Join(liveTree, "lua", "extra.lua"))
	require.NotContains(t, stdout, "CURRENT-TREE-CONTENT")
	require.NotContains(t, stdout, "DESIRED-TREE-CONTENT")
	require.NotContains(t, stdout, "EXTRA-LIVE-TREE-CONTENT")
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

func TestV2SyncJSONLiveToStoredRequiresYesAndMutates(t *testing.T) {
	fixture := setupCLIV2SelectedPreviewFixture(t, true, true)
	setCWD(t, fixture.repoRoot)
	livePath := filepath.Join(fixture.liveRoot, "config.yaml")
	desiredPath := filepath.Join(fixture.repoRoot, "desired", "user", "leon", "targets", "test.app", "settings.yaml")

	_, _, err := runSelectedPreviewCLI(t, []string{"apply", "--yes", "--json", "--user-id", "leon", "test.app:identity.email"})
	require.NoError(t, err)
	writeCLIFile(t, livePath, "user:\n  email: changed-live@example.com\n")

	payload, stdout, err := runSelectedPreviewCLI(t, []string{"sync", "--json", "--user-id", "leon", "test.app:identity.email"})
	require.Error(t, err)
	require.Equal(t, "dotfiles-manager.sync-execution.v2", payload["schema"])
	require.Equal(t, "confirmation-required", payload["summary"].(map[string]any)["status"])
	require.Equal(t, "syncexec.confirmationRequired", payload["error"].(map[string]any)["code"])
	require.NotContains(t, string(mustReadCLIFile(t, desiredPath)), "changed-live@example.com")
	require.NotContains(t, stdout, "Proceed with sync?")
	require.NotContains(t, stdout, "changed-live@example.com")

	payload, stdout, err = runSelectedPreviewCLI(t, []string{"sync", "--non-interactive", "--json", "--user-id", "leon", "test.app:identity.email"})
	require.Error(t, err)
	require.Equal(t, "dotfiles-manager.sync-execution.v2", payload["schema"])
	require.Equal(t, "confirmation-required", payload["summary"].(map[string]any)["status"])
	require.Equal(t, "syncexec.confirmationRequired", payload["error"].(map[string]any)["code"])
	require.NotContains(t, string(mustReadCLIFile(t, desiredPath)), "changed-live@example.com")
	require.NotContains(t, stdout, "changed-live@example.com")

	payload, stdout, err = runSelectedPreviewCLI(t, []string{"sync", "--yes", "--json", "--user-id", "leon", "test.app:identity.email"})
	require.NoError(t, err)
	require.Equal(t, "dotfiles-manager.sync-execution.v2", payload["schema"])
	require.Equal(t, "sync", payload["command"])
	summary := payload["summary"].(map[string]any)
	require.Equal(t, "complete", summary["status"])
	require.Equal(t, float64(1), summary["changed"])
	require.Equal(t, float64(1), summary["writesToStoredSettings"])
	items := payload["items"].([]any)
	item := items[0].(map[string]any)
	require.Equal(t, "write", item["decision"])
	require.Equal(t, "live_to_stored", item["direction"])
	require.Equal(t, "changed", item["result"])
	require.NotContains(t, item, "backupRefs")
	require.Contains(t, string(mustReadCLIFile(t, desiredPath)), "changed-live@example.com")
	require.NotContains(t, stdout, "changed-live@example.com")
}

func TestV2SyncJSONNoWritesDoesNotRequireYes(t *testing.T) {
	fixture := setupCLIV2SelectedPreviewFixture(t, true, true)
	setCWD(t, fixture.repoRoot)

	_, _, err := runSelectedPreviewCLI(t, []string{"apply", "--yes", "--json", "--user-id", "leon", "test.app:identity.email"})
	require.NoError(t, err)

	payload, stdout, err := runSelectedPreviewCLI(t, []string{"sync", "--json", "--user-id", "leon", "test.app:identity.email"})
	require.NoError(t, err)
	require.Equal(t, "dotfiles-manager.sync-execution.v2", payload["schema"])
	require.Equal(t, "no-changes", payload["summary"].(map[string]any)["status"])
	require.Equal(t, float64(0), payload["summary"].(map[string]any)["changed"])
	require.Equal(t, false, payload["confirmation"].(map[string]any)["required"])
	require.NotContains(t, stdout, "Proceed with sync?")
}

func TestV2SyncNonInteractiveTextRequiresYesWithoutMutation(t *testing.T) {
	fixture := setupCLIV2SelectedPreviewFixture(t, true, true)
	setCWD(t, fixture.repoRoot)
	livePath := filepath.Join(fixture.liveRoot, "config.yaml")
	desiredPath := filepath.Join(fixture.repoRoot, "desired", "user", "leon", "targets", "test.app", "settings.yaml")

	_, _, err := runSelectedPreviewCLI(t, []string{"apply", "--yes", "--json", "--user-id", "leon", "test.app:identity.email"})
	require.NoError(t, err)
	writeCLIFile(t, livePath, "user:\n  email: changed-live@example.com\n")

	stdout, stderr, err := runSyncTextCLI(t, []string{"sync", "--non-interactive", "--user-id", "leon", "test.app:identity.email"}, "")
	require.Error(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Sync not run: confirmation required.")
	require.Contains(t, stdout, "Run again with --yes")
	require.NotContains(t, stdout, "Proceed with sync?")
	require.NotContains(t, string(mustReadCLIFile(t, desiredPath)), "changed-live@example.com")
	require.NotContains(t, stdout, "changed-live@example.com")
}

func TestV2SyncInteractiveEOFRequiresConfirmationWithoutMutation(t *testing.T) {
	fixture := setupCLIV2SelectedPreviewFixture(t, true, true)
	setCWD(t, fixture.repoRoot)
	livePath := filepath.Join(fixture.liveRoot, "config.yaml")
	desiredPath := filepath.Join(fixture.repoRoot, "desired", "user", "leon", "targets", "test.app", "settings.yaml")

	_, _, err := runSelectedPreviewCLI(t, []string{"apply", "--yes", "--json", "--user-id", "leon", "test.app:identity.email"})
	require.NoError(t, err)
	writeCLIFile(t, livePath, "user:\n  email: changed-live@example.com\n")

	stdout, stderr, err := runSyncTextCLI(t, []string{"sync", "--user-id", "leon", "test.app:identity.email"}, "")
	require.Error(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Proceed with sync? [y/N]")
	require.Contains(t, stdout, "Sync not run: confirmation required.")
	require.NotContains(t, string(mustReadCLIFile(t, desiredPath)), "changed-live@example.com")
	require.NotContains(t, stdout, "changed-live@example.com")
}

func TestV2SyncJSONDoesNotExposeInternalDiagnostics(t *testing.T) {
	fixture := setupCLIV2SelectedPreviewFixture(t, false, true)
	setCWD(t, fixture.repoRoot)

	payload, stdout, err := runSelectedPreviewCLI(t, []string{"sync", "--non-interactive", "--json", "--user-id", "leon", "test.app:identity.email"})
	require.Error(t, err)
	require.Equal(t, "dotfiles-manager.sync-execution.v2", payload["schema"])
	items := payload["items"].([]any)
	require.Len(t, items, 1)
	item := items[0].(map[string]any)
	require.NotContains(t, item, "diagnostics")
	for _, forbidden := range []string{"driverId", "resourceId", "selectedpreview.", "desired://"} {
		require.NotContains(t, stdout, forbidden)
	}
}

func TestV2SyncJSONStoredToLiveMutatesWithoutExposingBackupRefs(t *testing.T) {
	fixture := setupCLIV2SelectedPreviewFixture(t, true, false)
	setCWD(t, fixture.repoRoot)
	livePath := filepath.Join(fixture.liveRoot, "config.yaml")
	desiredPath := filepath.Join(fixture.repoRoot, "desired", "user", "leon", "targets", "test.app", "settings.yaml")

	_, _, err := runSelectedPreviewCLI(t, []string{"save", "--yes", "--json", "--user-id", "leon", "test.app:identity.email"})
	require.NoError(t, err)
	writeCLIFile(t, desiredPath, "schema: dotfiles-manager.v2.desired-settings\nschemaVersion: 1\nvalues:\n  identity.email:\n    intent: set\n    kind: string\n    value: changed-stored@example.com\n")

	payload, stdout, err := runSelectedPreviewCLI(t, []string{"sync", "--yes", "--json", "--user-id", "leon", "test.app:identity.email"})
	require.NoError(t, err)
	require.Equal(t, "dotfiles-manager.sync-execution.v2", payload["schema"])
	summary := payload["summary"].(map[string]any)
	require.Equal(t, "complete", summary["status"])
	require.Equal(t, float64(1), summary["changed"])
	require.Equal(t, float64(1), summary["writesToLiveSettings"])
	item := payload["items"].([]any)[0].(map[string]any)
	require.Equal(t, "stored_to_live", item["direction"])
	require.Equal(t, "changed", item["result"])
	require.NotContains(t, item, "backupRefs")
	require.Contains(t, string(mustReadCLIFile(t, livePath)), "changed-stored@example.com")
	require.NotContains(t, stdout, "changed-stored@example.com")
}

func TestV2SyncRefusesFileTreeStoredToLiveUntilDetailedReview(t *testing.T) {
	fixture := setupCLIV2FileTreeResourceFixture(t)
	setCWD(t, fixture.repoRoot)
	liveTree := filepath.Join(fixture.liveRoot, "profiles")
	desiredTree := filepath.Join(fixture.repoRoot, "desired", "user", "leon", "targets", "test.tree", "artifacts", "config")
	currentBody := "CURRENT-TREE-CONTENT\n"
	changedDesiredBody := "CHANGED-DESIRED-TREE-CONTENT\n"
	extraLiveBody := "EXTRA-LIVE-TREE-CONTENT\n"
	writeCLIFile(t, filepath.Join(liveTree, "init.lua"), currentBody)
	writeCLIFile(t, filepath.Join(liveTree, "lua", "extra.lua"), extraLiveBody)

	_, _, err := runSelectedPreviewCLI(t, []string{"save", "--yes", "--json", "--user-id", "leon", "test.tree:config"})
	require.NoError(t, err)
	writeCLIFile(t, filepath.Join(desiredTree, "init.lua"), changedDesiredBody)
	os.Remove(filepath.Join(desiredTree, "lua", "extra.lua"))

	payload, stdout, err := runSelectedPreviewCLI(t, []string{"sync", "--yes", "--json", "--user-id", "leon", "test.tree:config"})
	require.Error(t, err)
	require.Equal(t, "dotfiles-manager.sync-execution.v2", payload["schema"])
	require.Equal(t, "blocked", payload["summary"].(map[string]any)["status"])
	require.Equal(t, "syncexec.blocked", payload["error"].(map[string]any)["code"])
	items := payload["items"].([]any)
	require.Len(t, items, 1)
	item := items[0].(map[string]any)
	require.Equal(t, "folder-tree-review-required", item["reasonCode"])
	require.Equal(t, false, item["executableBySync"])
	require.Equal(t, "skipped", item["result"])
	require.Contains(t, item["message"], "folder setting needs a detailed file-by-file review")
	require.Equal(t, currentBody, string(mustReadCLIFile(t, filepath.Join(liveTree, "init.lua"))))
	require.Equal(t, extraLiveBody, string(mustReadCLIFile(t, filepath.Join(liveTree, "lua", "extra.lua"))))
	require.NotContains(t, stdout, currentBody)
	require.NotContains(t, stdout, changedDesiredBody)
	require.NotContains(t, stdout, extraLiveBody)
}

func TestV2SyncRefusesConflictWithoutMutation(t *testing.T) {
	fixture := setupCLIV2SelectedPreviewFixture(t, true, false)
	setCWD(t, fixture.repoRoot)
	livePath := filepath.Join(fixture.liveRoot, "config.yaml")
	desiredPath := filepath.Join(fixture.repoRoot, "desired", "user", "leon", "targets", "test.app", "settings.yaml")

	_, _, err := runSelectedPreviewCLI(t, []string{"save", "--yes", "--json", "--user-id", "leon", "test.app:identity.email"})
	require.NoError(t, err)
	writeCLIFile(t, livePath, "user:\n  email: changed-live@example.com\n")
	writeCLIFile(t, desiredPath, "schema: dotfiles-manager.v2.desired-settings\nschemaVersion: 1\nvalues:\n  identity.email:\n    intent: set\n    kind: string\n    value: changed-stored@example.com\n")

	payload, stdout, err := runSelectedPreviewCLI(t, []string{"sync", "--yes", "--json", "--user-id", "leon", "test.app:identity.email"})
	require.Error(t, err)
	require.Equal(t, "dotfiles-manager.sync-execution.v2", payload["schema"])
	require.Equal(t, "needs-choice", payload["summary"].(map[string]any)["status"])
	require.Equal(t, "syncexec.choiceRequired", payload["error"].(map[string]any)["code"])
	item := payload["items"].([]any)[0].(map[string]any)
	require.Equal(t, "needs_choice", item["decision"])
	require.Equal(t, "both_sides_changed", item["direction"])
	require.Equal(t, "skipped", item["result"])
	require.Contains(t, string(mustReadCLIFile(t, livePath)), "changed-live@example.com")
	require.Contains(t, string(mustReadCLIFile(t, desiredPath)), "changed-stored@example.com")
	require.NotContains(t, stdout, "changed-live@example.com")
	require.NotContains(t, stdout, "changed-stored@example.com")
}

func TestV2SyncInteractivePromptExecutesAcceptedWriteSet(t *testing.T) {
	fixture := setupCLIV2SelectedPreviewFixture(t, true, true)
	setCWD(t, fixture.repoRoot)
	livePath := filepath.Join(fixture.liveRoot, "config.yaml")
	desiredPath := filepath.Join(fixture.repoRoot, "desired", "user", "leon", "targets", "test.app", "settings.yaml")

	_, _, err := runSelectedPreviewCLI(t, []string{"apply", "--yes", "--json", "--user-id", "leon", "test.app:identity.email"})
	require.NoError(t, err)
	writeCLIFile(t, livePath, "user:\n  email: prompt-live@example.com\n")

	stdout, stderr, err := runSyncTextCLI(t, []string{"sync", "--user-id", "leon", "test.app:identity.email"}, "yes\n")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Sync plan accepted.")
	require.Contains(t, stdout, "Will sync 1 setting:")
	require.Contains(t, stdout, "test.app:identity.email: live settings -> stored settings")
	require.Contains(t, stdout, "Proceed with sync? [y/N]")
	require.Contains(t, stdout, "Sync complete.")
	require.Contains(t, stdout, "Changed: 1")
	require.Contains(t, string(mustReadCLIFile(t, desiredPath)), "prompt-live@example.com")
	require.NotContains(t, stdout, "prompt-live@example.com")
}

func TestV2SyncHelpUsesResetVocabulary(t *testing.T) {
	rootStdout, rootStderr, err := runSelectedPreviewTextCLI(t, []string{"--help"})
	require.NoError(t, err)
	require.Empty(t, rootStderr)
	require.Contains(t, rootStdout, "Normal v2 workflow:\n  status -> diff -> sync")
	require.Contains(t, rootStdout, "Sync safe v2 settings changes between live settings and stored settings")
	require.Contains(t, rootStdout, "Compatibility aliases:")
	require.Contains(t, rootStdout, "save  sync live settings -> stored settings")
	require.Contains(t, rootStdout, "apply sync stored settings -> live settings")
	require.Less(t, strings.Index(rootStdout, "sync        Sync safe"), strings.Index(rootStdout, "save        Compatibility alias"))
	require.Less(t, strings.Index(rootStdout, "save        Compatibility alias"), strings.Index(rootStdout, "apply       Compatibility alias"))

	saveStdout, saveStderr, err := runSelectedPreviewTextCLI(t, []string{"save", "--help"})
	require.NoError(t, err)
	require.Empty(t, saveStderr)
	require.Contains(t, saveStdout, "Compatibility alias for directional sync")
	require.Contains(t, saveStdout, "save copies selected live settings to stored settings")
	require.Contains(t, saveStdout, "For normal use, run status, then diff, then sync")
	require.Contains(t, saveStdout, "explicit live-settings-to-stored-settings direction")
	require.NotContains(t, saveStdout, "should be taught first")
	require.NotContains(t, strings.ToLower(saveStdout), "desired")
	require.NotContains(t, strings.ToLower(saveStdout), "repository")

	applyStdout, applyStderr, err := runSelectedPreviewTextCLI(t, []string{"apply", "--help"})
	require.NoError(t, err)
	require.Empty(t, applyStderr)
	require.Contains(t, applyStdout, "Compatibility alias for directional sync")
	require.Contains(t, applyStdout, "apply copies selected stored settings from the settings folder to live settings")
	require.Contains(t, applyStdout, "For normal use, run status, then diff, then sync")
	require.Contains(t, applyStdout, "explicit stored-settings-to-live-settings direction")
	require.NotContains(t, applyStdout, "should be taught first")
	require.NotContains(t, strings.ToLower(applyStdout), "desired")
	require.NotContains(t, strings.ToLower(applyStdout), "repository")

	syncStdout, syncStderr, err := runSelectedPreviewTextCLI(t, []string{"sync", "--help"})
	require.NoError(t, err)
	require.Empty(t, syncStderr)
	require.Contains(t, syncStdout, "live settings -> stored settings")
	require.Contains(t, syncStdout, "stored settings -> live settings")
	require.Contains(t, syncStdout, "settings folder")
	for _, forbidden := range []string{"guided", "save/apply", "--choice", "repository", "backup", "restore", "migration", "desired://", "driver", "resource"} {
		require.NotContains(t, strings.ToLower(syncStdout), forbidden)
	}
}

func TestV2SyncDocsDescribeAcceptedExecutionPath(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))

	commandsDoc := string(mustReadCLIFile(t, filepath.Join(repoRoot, "docs", "user", "commands.md")))
	require.Contains(t, commandsDoc, "## v2 sync: safe settings execution")
	syncSection := commandsDoc[strings.Index(commandsDoc, "## v2 sync: safe settings execution"):]
	if nextHeading := strings.Index(syncSection[len("## v2 sync: safe settings execution"):], "\n## "); nextHeading >= 0 {
		syncSection = syncSection[:len("## v2 sync: safe settings execution")+nextHeading]
	}
	require.Contains(t, commandsDoc, "`sync [ref]` checks the current state and runs only safe one-sided settings changes")
	require.Contains(t, syncSection, "live settings -> stored settings")
	require.Contains(t, syncSection, "stored settings -> live settings")
	for _, forbidden := range []string{"guided sync", "--choice", "backup", "restore", "migration", "desired://", "driver", "resource"} {
		require.NotContains(t, strings.ToLower(syncSection), forbidden)
	}
}

func runSyncTextCLI(t *testing.T, args []string, stdin string) (string, string, error) {
	t.Helper()
	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
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

func setupCLIV2FileTreeResourceFixture(t *testing.T) cliV2SelectedPreviewFixture {
	t.Helper()
	homeDir := setTempHome(t)
	repoRoot := t.TempDir()
	liveRoot := t.TempDir()
	writeCLIFile(t, filepath.Join(repoRoot, "dotfiles-manager.v2.yaml"), "schema: dotfiles-manager.v2.root-config\nschemaVersion: 1\nactiveProfileStack: default\n")
	writeCLIFile(t, filepath.Join(repoRoot, "profiles", "stacks", "default.yaml"), "schema: dotfiles-manager.v2.profile-stack\nschemaVersion: 1\nprofileStack: [global]\n")
	writeCLIFile(t, filepath.Join(repoRoot, "profiles", "layers", "global.yaml"), "schema: dotfiles-manager.v2.profile-layer\nschemaVersion: 1\nselections:\n  test.tree:\n    settings:\n      config:\n        scope: user\n        artifact: artifacts/config\n")
	writeCLIFile(t, filepath.Join(repoRoot, "recipes", "local", "test.tree", "recipe.yaml"), `schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: test.tree
displayName: Test tree
supportLevel: experimental
capability: read-write
locations:
  config:
    default: `+liveRoot+`
settings:
  config:
    label: Config tree
    supportLevel: experimental
    capability: read-write
    artifactForm: file-tree
    sensitivity: personal
    redaction: redacted-for-display
    lifecycle: allowed
    scopeDefault: user
    resource: config-tree
resources:
  config-tree:
    driver: file-tree
    location: config
    path: profiles
    capability: read-write
    sensitivity: personal
    redaction: redacted-for-display
    lifecycle: allowed
    include:
      - "**"
`)
	rec, err := v2recipe.LoadLocal(repoRoot, "test.tree")
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

func runSelectedPreviewTextCLI(t *testing.T, args []string) (string, string, error) {
	t.Helper()
	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
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

func requireFileTreeOperation(t *testing.T, operations []any, action string, path string, kind string, state string) {
	t.Helper()
	for _, raw := range operations {
		operation := raw.(map[string]any)
		if operation["action"] == action && operation["path"] == path && operation["kind"] == kind && operation["state"] == state {
			return
		}
	}
	require.Failf(t, "missing file-tree operation", "wanted action=%s path=%s kind=%s state=%s in %#v", action, path, kind, state, operations)
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
