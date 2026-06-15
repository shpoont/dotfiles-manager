package app

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	v2ledger "github.com/shpoont/dotfiles-manager/internal/v2/ledger"
	v2recipe "github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestBackupListAndSelectedValueWholeFileRestoreCLI(t *testing.T) {
	fixture := setupCLIV2SelectedPreviewFixture(t, true, true)
	setCWD(t, fixture.repoRoot)

	payload, stdout, err := runSelectedPreviewCLI(t, []string{"apply", "--yes", "--json", "--user-id", "leon", "test.app:identity.email"})
	require.NoError(t, err)
	runID := payload["runId"].(string)
	require.NotEmpty(t, runID)
	require.NotContains(t, stdout, "current@example.com")
	require.NotContains(t, stdout, "desired@example.com")
	require.Contains(t, string(mustReadCLIFile(t, filepath.Join(fixture.liveRoot, "config.yaml"))), "desired@example.com")

	writeCLIFile(t, filepath.Join(fixture.liveRoot, "config.yaml"), "user:\n  email: broken@example.com\n")

	listPayload, listStdout, err := runSelectedPreviewCLI(t, []string{"backup", "list", "--json"})
	require.NoError(t, err)
	require.Equal(t, "dotfiles-manager.v2.backup-report", listPayload["schema"])
	require.Equal(t, "backup.list", listPayload["command"])
	require.Contains(t, listStdout, runID)
	require.NotContains(t, listStdout, "current@example.com")
	require.NotContains(t, listStdout, "desired@example.com")
	require.NotContains(t, listStdout, "broken@example.com")

	dryRunPayload, dryRunStdout, err := runSelectedPreviewCLI(t, []string{"restore", runID, "--dry-run", "--json", "--user-id", "leon"})
	require.NoError(t, err)
	require.Equal(t, "restore", dryRunPayload["command"])
	require.Equal(t, "changed", dryRunPayload["summary"].(map[string]any)["status"])
	items := dryRunPayload["items"].([]any)
	require.Len(t, items, 1)
	item := items[0].(map[string]any)
	require.Equal(t, "would-change", item["result"])
	require.Contains(t, item["message"], "whole backing file")
	require.Contains(t, item["backup"].(map[string]any)["message"], "will be created")
	require.NotContains(t, dryRunStdout, "current@example.com")
	require.NotContains(t, dryRunStdout, "desired@example.com")
	require.NotContains(t, dryRunStdout, "broken@example.com")
	require.Contains(t, string(mustReadCLIFile(t, filepath.Join(fixture.liveRoot, "config.yaml"))), "broken@example.com")

	confirmPayload, _, err := runSelectedPreviewCLI(t, []string{"restore", runID, "--non-interactive", "--json", "--user-id", "leon"})
	require.Error(t, err)
	require.Equal(t, "blocked", confirmPayload["summary"].(map[string]any)["status"])
	require.Contains(t, string(mustReadCLIFile(t, filepath.Join(fixture.liveRoot, "config.yaml"))), "broken@example.com")

	restorePayload, restoreStdout, err := runSelectedPreviewCLI(t, []string{"restore", runID, "--yes", "--json", "--user-id", "leon"})
	require.NoError(t, err)
	require.Equal(t, "restore", restorePayload["command"])
	require.Equal(t, "ok", restorePayload["summary"].(map[string]any)["status"])
	require.Equal(t, float64(1), restorePayload["summary"].(map[string]any)["applied"])
	items = restorePayload["items"].([]any)
	require.Len(t, items, 1)
	item = items[0].(map[string]any)
	require.Equal(t, "applied", item["result"])
	require.Contains(t, item["message"], "whole backing file")
	backup := item["backup"].(map[string]any)
	require.Equal(t, "ref-supplied", backup["policy"])
	require.Contains(t, backup["ref"], "state://backups/restore-")
	require.Contains(t, backup["message"], "Backup-before-restore was created")
	require.NotContains(t, restoreStdout, "would-change")
	require.NotContains(t, restoreStdout, "will be created")
	require.NotContains(t, restoreStdout, "current@example.com")
	require.NotContains(t, restoreStdout, "desired@example.com")
	require.NotContains(t, restoreStdout, "broken@example.com")
	require.Contains(t, string(mustReadCLIFile(t, filepath.Join(fixture.liveRoot, "config.yaml"))), "current@example.com")
}

func TestBackupTextOutputAndMissingShowError(t *testing.T) {
	fixture := setupCLIV2SelectedPreviewFixture(t, true, true)
	setCWD(t, fixture.repoRoot)
	payload, _, err := runSelectedPreviewCLI(t, []string{"apply", "--yes", "--json", "--user-id", "leon", "test.app:identity.email"})
	require.NoError(t, err)
	runID := payload["runId"].(string)

	stdout, err := runBackupRestoreRawCLI(t, []string{"backup", "list", "--verbose"})
	require.NoError(t, err)
	require.Contains(t, stdout, "dotfiles-manager v2 backup.list")
	require.Contains(t, stdout, runID)
	require.Contains(t, stdout, "Backup payload contents are never printed.")
	require.NotContains(t, stdout, "current@example.com")
	require.NotContains(t, stdout, "desired@example.com")

	stdout, err = runBackupRestoreRawCLI(t, []string{"backup", "show", "missing-run", "--json"})
	require.Error(t, err)
	exitErr, ok := err.(interface{ ExitCode() int })
	require.True(t, ok)
	require.Equal(t, 2, exitErr.ExitCode())
	require.Contains(t, err.Error(), "missing-run")
	var errorPayload map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &errorPayload))
	require.Equal(t, "dotfiles-manager.v2.backup-report", errorPayload["schema"])
	require.Equal(t, "backup.show", errorPayload["command"])
	require.Equal(t, "error", errorPayload["summary"].(map[string]any)["status"])
	require.Equal(t, "backup.show", errorPayload["error"].(map[string]any)["code"])
}

func TestRestoreTextOutputAndConfigErrorBranches(t *testing.T) {
	fixture := setupCLIV2SelectedPreviewFixture(t, true, true)
	setCWD(t, fixture.repoRoot)
	payload, _, err := runSelectedPreviewCLI(t, []string{"apply", "--yes", "--json", "--user-id", "leon", "test.app:identity.email"})
	require.NoError(t, err)
	runID := payload["runId"].(string)

	stdout, err := runBackupRestoreRawCLI(t, []string{"restore", runID, "--dry-run", "--user-id", "leon"})
	require.NoError(t, err)
	require.Contains(t, stdout, "Restore preview for backup "+runID)
	require.Contains(t, stdout, "Dry run: no files changed.")
	require.Contains(t, stdout, "Whole file/artifact restore.")
	require.NotContains(t, stdout, "current@example.com")
	require.NotContains(t, stdout, "desired@example.com")

	stdout, err = runBackupRestoreRawCLI(t, []string{"restore", runID, "--dry-run", "--verbose", "--user-id", "leon"})
	require.NoError(t, err)
	require.Contains(t, stdout, "Restore preview for backup "+runID)
	require.Contains(t, stdout, "Technical details:")
	require.Contains(t, stdout, "result=would-change")
	require.Contains(t, stdout, "backupPolicy=required")

	writeCLIFile(t, filepath.Join(fixture.liveRoot, "config.yaml"), "user:\n  email: broken@example.com\n")
	stdout, err = runBackupRestoreRawCLI(t, []string{"restore", runID, "--yes", "--verbose", "--user-id", "leon"})
	require.NoError(t, err)
	require.Contains(t, stdout, "Restore completed.")
	require.Contains(t, stdout, "Live file restored:")
	require.Contains(t, stdout, "Created recovery handle restore-")
	require.Contains(t, stdout, "Technical details:")
	require.Contains(t, stdout, "result=applied")
	require.Contains(t, stdout, "backupBeforeRestore=restore-")
	require.NotContains(t, stdout, "restore preview")
	require.NotContains(t, stdout, "would-change")
	require.NotContains(t, stdout, "will be created")
	require.NotContains(t, stdout, "broken@example.com")

	stdout, err = runBackupRestoreRawCLI(t, []string{"restore", runID, "--yes", "--verbose", "--user-id", "leon"})
	require.NoError(t, err)
	require.Contains(t, stdout, "Restore completed.")
	require.Contains(t, stdout, "No live files needed changes")
	require.Contains(t, stdout, "No recovery handle was created because no live write was needed.")
	require.Contains(t, stdout, "Technical details:")
	require.Contains(t, stdout, "result=unchanged")
	require.NotContains(t, stdout, "restore preview")
	require.NotContains(t, stdout, "would-change")
	require.NotContains(t, stdout, "will be created")

	stdout, err = runBackupRestoreRawCLI(t, []string{"restore", runID, "--yes", "--user-id", "leon"})
	require.NoError(t, err)
	require.Contains(t, stdout, "Restore completed.")
	require.Contains(t, stdout, "No live files needed changes")
	require.Contains(t, stdout, "No recovery handle was created because no live write was needed.")
	require.NotContains(t, stdout, "Technical details:")
	require.NotContains(t, stdout, "restore preview")
	require.NotContains(t, stdout, "would-change")
	require.NotContains(t, stdout, "will be created")

	noChangePayload, noChangeStdout, err := runSelectedPreviewCLI(t, []string{"restore", runID, "--yes", "--json", "--user-id", "leon"})
	require.NoError(t, err)
	require.Equal(t, "ok", noChangePayload["summary"].(map[string]any)["status"])
	require.Equal(t, float64(0), noChangePayload["summary"].(map[string]any)["applied"])
	noChangeItems := noChangePayload["items"].([]any)
	require.Len(t, noChangeItems, 1)
	noChangeItem := noChangeItems[0].(map[string]any)
	require.Equal(t, "unchanged", noChangeItem["result"])
	require.Equal(t, "not-applicable", noChangeItem["backup"].(map[string]any)["policy"])
	require.Contains(t, noChangeItem["backup"].(map[string]any)["message"], "No backup-before-restore was created")
	require.NotContains(t, noChangeStdout, "would-change")
	require.NotContains(t, noChangeStdout, "will be created")

	v1Config := filepath.Join(fixture.repoRoot, ".dotfiles-manager.yaml")
	writeCLIFile(t, v1Config, "syncs: []\n")
	stdout, err = runBackupRestoreRawCLI(t, []string{"backup", "list", "--config", v1Config, "--json"})
	require.Error(t, err)
	require.Contains(t, stdout, "backup.root")
}

func TestRestoreRootAndProfileErrorsUseStableEnvelope(t *testing.T) {
	setTempHome(t)
	setCWD(t, t.TempDir())
	payload, _, err := runSelectedPreviewCLI(t, []string{"restore", "missing-run", "--json"})
	require.Error(t, err)
	require.Equal(t, "restore.root.notFound", payload["items"].([]any)[0].(map[string]any)["diagnostics"].([]any)[0].(map[string]any)["code"])

	fixture := setupCLIV2SelectedPreviewFixture(t, true, true)
	setCWD(t, fixture.repoRoot)
	writeCLIFile(t, filepath.Join(fixture.repoRoot, "profiles", "stacks", "default.yaml"), "schema: dotfiles-manager.v2.profile-stack\nschemaVersion: 1\nprofileStack: []\n")
	payload, _, err = runSelectedPreviewCLI(t, []string{"restore", "missing-run", "--json", "--user-id", "leon"})
	require.Error(t, err)
	require.Equal(t, "restore.profile.resolve", payload["items"].([]any)[0].(map[string]any)["diagnostics"].([]any)[0].(map[string]any)["code"])
}

func TestBackupRestoreHelperBranches(t *testing.T) {
	require.Equal(t, "", (*backupCLIError)(nil).Error())
	require.Equal(t, 1, (&backupCLIError{}).ExitCode())
	require.Equal(t, "restore", RestoreCommandName(""))
	require.Equal(t, "custom", RestoreCommandName(" custom "))
	require.False(t, restoreDriverSupported("native-export"))
	require.Equal(t, "fallback", defaultBackupString("", "fallback"))
	require.Equal(t, "", restoreErrString(nil))
	require.Equal(t, "~", redactDisplayPath("~"))
	replaced, ok := replacePathPrefix("/tmp/example/file", "", "$X")
	require.False(t, ok)
	require.Equal(t, "/tmp/example/file", replaced)
	replaced, ok = replacePathPrefix("/tmp/example/file", "/other", "$X")
	require.False(t, ok)
	require.Equal(t, "/tmp/example/file", replaced)
}

func TestBackupFriendlyRendererHelpersCoverFallbackBranches(t *testing.T) {
	t.Parallel()

	noBackups := backupReport{Command: "backup.list"}
	require.Contains(t, backupReportText(noBackups), "No backups found yet.")

	errText := backupReportText(backupReport{Command: "backup.show", Error: &backupErrorObject{Message: "backup missing"}})
	require.Contains(t, errText, "Command result:")
	require.Contains(t, errText, "backup missing")
	require.Contains(t, errText, "No files changed.")

	report := backupReport{
		Command: "backup.show",
		Backups: []backupView{{
			RunID:     "backup-1",
			CreatedAt: "2026-06-13T12:00:00Z",
			ItemCount: 3,
			Items: []backupItemView{
				{
					SettingRef: "git:user.email",
					Driver:     v2recipe.IniFileDriverID,
					Restore:    v2ledger.RestoreCompatibility{Compatible: true},
				},
				{
					Ref:     "raycast/native-export",
					Driver:  "native",
					Restore: v2ledger.RestoreCompatibility{Compatible: false, Message: "native import is manual"},
				},
				{
					TargetRef: "fallback.target",
					Driver:    "native",
					Restore:   v2ledger.RestoreCompatibility{},
				},
			},
		}},
	}
	text := backupReportText(report)
	require.Contains(t, text, "Backup backup-1")
	require.Contains(t, text, "git:user.email — User email")
	require.Contains(t, text, "To the value from before the apply run.")
	require.Contains(t, text, "selected setting — Raycast native export")
	require.Contains(t, text, "Cannot restore automatically: native import is manual")
	require.Contains(t, text, "fallback.target — Selected setting")
	require.Contains(t, text, "Cannot restore automatically: Restore is not supported for this backup item.")
	require.Contains(t, text, "Use --verbose for technical details or --json for machine-readable output.")

	require.Equal(t, "git:user.email", backupItemFriendlyRef(backupItemView{SettingRef: "git:user.email", TargetRef: "git"}))
	require.Equal(t, "git", backupItemFriendlyRef(backupItemView{TargetRef: "git"}))
	require.Equal(t, "selected setting", backupItemFriendlyRef(backupItemView{}))
	require.Equal(t, "User email", backupItemFriendlyLabel(backupItemView{SettingRef: "git:user.email"}))
	require.Equal(t, "Credential helper secret", backupItemFriendlyLabel(backupItemView{Ref: "credential-helper-secret"}))
	require.Equal(t, "Selected setting", backupItemFriendlyLabel(backupItemView{}))
	require.Equal(t, "Selected setting", selectedSettingLabelForBackup("  "))
}

func TestRestoreBlocksAllWritesWhenRunContainsNativeUnsupportedItem(t *testing.T) {
	fixture := setupCLIV2SelectedPreviewFixture(t, true, true)
	setCWD(t, fixture.repoRoot)

	payload, _, err := runSelectedPreviewCLI(t, []string{"apply", "--yes", "--json", "--user-id", "leon", "test.app:identity.email"})
	require.NoError(t, err)
	runID := payload["runId"].(string)
	appendNativeUnsupportedBackupItem(t, fixture.repoRoot, runID)

	writeCLIFile(t, filepath.Join(fixture.liveRoot, "config.yaml"), "user:\n  email: broken@example.com\n")
	blockedDefaultText, err := runBackupRestoreRawCLI(t, []string{"restore", runID, "--yes", "--user-id", "leon"})
	require.Error(t, err)
	require.Contains(t, blockedDefaultText, "Restore blocked before writing live files.")
	require.Contains(t, blockedDefaultText, "No files changed.")
	require.Contains(t, blockedDefaultText, "Native export")
	require.Contains(t, blockedDefaultText, "Resolve the blocker above before retrying restore.")
	require.NotContains(t, blockedDefaultText, "Technical details:")
	require.NotContains(t, blockedDefaultText, "If this is the backup you want, confirm restore")
	require.Contains(t, string(mustReadCLIFile(t, filepath.Join(fixture.liveRoot, "config.yaml"))), "broken@example.com")

	blockedText, err := runBackupRestoreRawCLI(t, []string{"restore", runID, "--yes", "--verbose", "--user-id", "leon"})
	require.Error(t, err)
	require.Contains(t, blockedText, "Restore blocked before writing live files.")
	require.Contains(t, blockedText, "No files changed.")
	require.Contains(t, blockedText, "native-export")
	require.Contains(t, blockedText, "Technical details:")
	require.Contains(t, blockedText, "result=blocked")
	require.Contains(t, blockedText, "Resolve the blocker above before retrying restore.")
	require.NotContains(t, blockedText, "If this is the backup you want, confirm restore")
	require.Contains(t, string(mustReadCLIFile(t, filepath.Join(fixture.liveRoot, "config.yaml"))), "broken@example.com")

	restorePayload, restoreStdout, err := runSelectedPreviewCLI(t, []string{"restore", runID, "--yes", "--json", "--user-id", "leon"})
	require.Error(t, err)
	require.Equal(t, "blocked", restorePayload["summary"].(map[string]any)["status"])
	require.Contains(t, restoreStdout, "native-export")
	require.Contains(t, string(mustReadCLIFile(t, filepath.Join(fixture.liveRoot, "config.yaml"))), "broken@example.com")

	showPayload, showStdout, err := runSelectedPreviewCLI(t, []string{"backup", "show", runID, "--json"})
	require.NoError(t, err)
	require.Equal(t, "backup.show", showPayload["command"])
	require.Contains(t, showStdout, "native-export")
}

func TestFileResourceRestoreCLI(t *testing.T) {
	fixture := setupCLIV2FileResourceFixture(t, true)
	setCWD(t, fixture.repoRoot)
	writeCLIFile(t, filepath.Join(fixture.liveRoot, "config.txt"), "old file\n")
	writeCLIFile(t, filepath.Join(fixture.repoRoot, "desired", "user", "leon", "targets", "test.files", "artifacts", "config"), "new file\n")

	payload, _, err := runSelectedPreviewCLI(t, []string{"apply", "--yes", "--json", "--user-id", "leon", "test.files:config"})
	require.NoError(t, err)
	runID := payload["runId"].(string)
	require.Contains(t, string(mustReadCLIFile(t, filepath.Join(fixture.liveRoot, "config.txt"))), "new file")

	writeCLIFile(t, filepath.Join(fixture.liveRoot, "config.txt"), "broken file\n")
	restorePayload, restoreStdout, err := runSelectedPreviewCLI(t, []string{"restore", runID, "--yes", "--json", "--user-id", "leon"})
	require.NoError(t, err)
	require.Equal(t, "restore", restorePayload["command"])
	require.Equal(t, "applied", restorePayload["items"].([]any)[0].(map[string]any)["result"])
	require.Equal(t, "ref-supplied", restorePayload["items"].([]any)[0].(map[string]any)["backup"].(map[string]any)["policy"])
	require.NotContains(t, restoreStdout, "would-change")
	require.NotContains(t, restoreStdout, "will be created")
	require.NotContains(t, restoreStdout, "old file")
	require.NotContains(t, restoreStdout, "new file")
	require.NotContains(t, restoreStdout, "broken file")
	require.Equal(t, "old file\n", string(mustReadCLIFile(t, filepath.Join(fixture.liveRoot, "config.txt"))))
}

func TestRestoreBundledGitEmailTempHomeSmoke(t *testing.T) {
	fixture := setupCLIV2BundledGitFixture(t)
	setCWD(t, fixture.repoRoot)
	helperSecret := "credential-helper-secret"
	gitConfig := filepath.Join(fixture.homeDir, ".gitconfig")
	writeCLIFile(t, gitConfig, "[credential]\n\thelper = "+helperSecret+"\n[user]\n\temail = current@example.com\n")

	_, stdout, err := runSelectedPreviewCLI(t, []string{"save", "--yes", "--json", "--user-id", "leon", "git:user.email"})
	require.NoError(t, err)
	require.NotContains(t, stdout, "current@example.com")
	require.NotContains(t, stdout, helperSecret)

	writeCLIFile(t, gitConfig, "[credential]\n\thelper = "+helperSecret+"\n[user]\n\temail = changed@example.com\n")
	applyPayload, stdout, err := runSelectedPreviewCLI(t, []string{"apply", "--yes", "--json", "--user-id", "leon", "git:user.email"})
	require.NoError(t, err)
	runID := applyPayload["runId"].(string)
	require.NotEmpty(t, runID)
	require.Contains(t, string(mustReadCLIFile(t, gitConfig)), "current@example.com")
	require.NotContains(t, stdout, "changed@example.com")
	require.NotContains(t, stdout, "current@example.com")
	require.NotContains(t, stdout, helperSecret)

	writeCLIFile(t, gitConfig, "[credential]\n\thelper = "+helperSecret+"\n[user]\n\temail = broken@example.com\n")
	stdout, err = runBackupRestoreRawCLI(t, []string{"restore", runID, "--dry-run", "--user-id", "leon"})
	require.NoError(t, err)
	require.Contains(t, stdout, "Restore preview for backup "+runID)
	require.Contains(t, stdout, "$HOME/.gitconfig")
	require.Contains(t, stdout, "Dry run: no files changed.")
	require.Contains(t, string(mustReadCLIFile(t, gitConfig)), "broken@example.com")
	require.NotContains(t, stdout, "broken@example.com")
	require.NotContains(t, stdout, "changed@example.com")
	require.NotContains(t, stdout, "current@example.com")
	require.NotContains(t, stdout, helperSecret)

	stdout, err = runBackupRestoreRawCLI(t, []string{"restore", runID, "--yes", "--user-id", "leon"})
	require.NoError(t, err)
	require.Contains(t, stdout, "Restore completed.")
	require.Contains(t, stdout, "Restored 1 live file from backup "+runID)
	require.Contains(t, stdout, "$HOME/.gitconfig")
	require.Contains(t, stdout, "Created recovery handle restore-")
	require.NotContains(t, stdout, "restore preview")
	require.NotContains(t, stdout, "would-change")
	require.NotContains(t, stdout, "will be created")
	require.NotContains(t, stdout, "broken@example.com")
	require.NotContains(t, stdout, "changed@example.com")
	require.NotContains(t, stdout, "current@example.com")
	require.NotContains(t, stdout, helperSecret)
	require.Contains(t, string(mustReadCLIFile(t, gitConfig)), "changed@example.com")
	require.Contains(t, string(mustReadCLIFile(t, gitConfig)), helperSecret)

	listPayload, listStdout, err := runSelectedPreviewCLI(t, []string{"backup", "list", "--json"})
	require.NoError(t, err)
	require.Equal(t, "backup.list", listPayload["command"])
	require.Contains(t, listStdout, "restore-")
	require.NotContains(t, listStdout, "broken@example.com")
	require.NotContains(t, listStdout, "changed@example.com")
	require.NotContains(t, listStdout, "current@example.com")
	require.NotContains(t, listStdout, helperSecret)
}

func TestRestoreCorruptedPayloadFailsClosed(t *testing.T) {
	fixture := setupCLIV2SelectedPreviewFixture(t, true, true)
	setCWD(t, fixture.repoRoot)

	payload, _, err := runSelectedPreviewCLI(t, []string{"apply", "--yes", "--json", "--user-id", "leon", "test.app:identity.email"})
	require.NoError(t, err)
	runID := payload["runId"].(string)
	corruptFirstBackupPayload(t, fixture.repoRoot, runID, "user:\n  email: tampered@example.com\n")

	writeCLIFile(t, filepath.Join(fixture.liveRoot, "config.yaml"), "user:\n  email: broken@example.com\n")
	restorePayload, restoreStdout, err := runSelectedPreviewCLI(t, []string{"restore", runID, "--yes", "--json", "--user-id", "leon"})
	require.Error(t, err)
	require.Equal(t, "blocked", restorePayload["summary"].(map[string]any)["status"])
	require.NotContains(t, restoreStdout, "tampered@example.com")
	require.NotContains(t, restoreStdout, "broken@example.com")
	require.Contains(t, restoreStdout, "payload hash mismatch")
	require.Contains(t, string(mustReadCLIFile(t, filepath.Join(fixture.liveRoot, "config.yaml"))), "broken@example.com")
}

func TestRestoreMissingBackupReturnsStableJSONError(t *testing.T) {
	fixture := setupCLIV2SelectedPreviewFixture(t, true, true)
	setCWD(t, fixture.repoRoot)

	payload, _, err := runSelectedPreviewCLI(t, []string{"restore", "missing-run", "--json", "--user-id", "leon"})
	require.Error(t, err)
	require.Equal(t, "dotfiles-manager.v2.preview", payload["schema"])
	require.Equal(t, "restore", payload["command"])
	require.Equal(t, "blocked", payload["summary"].(map[string]any)["status"])
	items := payload["items"].([]any)
	require.Len(t, items, 1)
	requireCLIDiagnosticCode(t, items[0].(map[string]any), "restore.failed")
}

func appendNativeUnsupportedBackupItem(t *testing.T, repoRoot string, runID string) {
	t.Helper()
	stateRoot, err := v2ledger.DefaultStateRoot(repoRoot)
	require.NoError(t, err)
	path := filepath.Join(stateRoot, "backups", runID, "backup.yaml")
	var metadata v2ledger.BackupMetadata
	require.NoError(t, yaml.Unmarshal(mustReadCLIFile(t, path), &metadata))
	metadata.Items = append(metadata.Items, v2ledger.BackupItem{
		Ref:           "state://backups/" + runID + "/native-settings",
		TargetRef:     "native.app",
		SettingRef:    "native.app:settings",
		ResourceID:    "settings",
		Driver:        v2recipe.NativeExportDriverID,
		DriverVersion: "native-export.driver.v1",
		LivePath:      "export-settings",
		CreatedAt:     metadata.CreatedAt,
		Before:        v2ledger.NormalizedState{Exists: true, Hash: strings.Repeat("a", 64), Normalizer: "native-export.metadata.v1", DriverVersion: "native-export.driver.v1"},
		Restore: v2ledger.RestoreCompatibility{
			Compatible:    false,
			Driver:        v2recipe.NativeExportDriverID,
			DriverVersion: "native-export.driver.v1",
			Normalizer:    "native-export.metadata.v1",
			Message:       "Native export backup was recorded before native apply; automatic native restore is not implemented in this tranche.",
		},
	})
	payload, err := yaml.Marshal(metadata)
	require.NoError(t, err)
	writeCLIFile(t, path, string(payload))
}

func corruptFirstBackupPayload(t *testing.T, repoRoot string, runID string, payload string) {
	t.Helper()
	stateRoot, err := v2ledger.DefaultStateRoot(repoRoot)
	require.NoError(t, err)
	metadataPath := filepath.Join(stateRoot, "backups", runID, "backup.yaml")
	var metadata v2ledger.BackupMetadata
	require.NoError(t, yaml.Unmarshal(mustReadCLIFile(t, metadataPath), &metadata))
	require.NotEmpty(t, metadata.Items)
	require.NotEmpty(t, metadata.Items[0].PayloadRelPath)
	writeCLIFile(t, filepath.Join(stateRoot, "backups", runID, metadata.Items[0].PayloadRelPath), payload)
}

func runBackupRestoreRawCLI(t *testing.T, args []string) (string, error) {
	t.Helper()
	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), err
}
