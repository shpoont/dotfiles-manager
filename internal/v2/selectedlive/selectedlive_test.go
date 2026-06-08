package selectedlive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shpoont/dotfiles-manager/internal/v2/desired"
	"github.com/shpoont/dotfiles-manager/internal/v2/filedriver"
	v2ledger "github.com/shpoont/dotfiles-manager/internal/v2/ledger"
	"github.com/shpoont/dotfiles-manager/internal/v2/macosdefaultsdriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/shpoont/dotfiles-manager/internal/v2/resolution"
	"github.com/shpoont/dotfiles-manager/internal/v2/selectedpreview"
	"github.com/shpoont/dotfiles-manager/internal/v2/selectedvalue"
	v2status "github.com/shpoont/dotfiles-manager/internal/v2/status"
	"github.com/stretchr/testify/require"
)

func TestApplyChangedBacksUpMutatesVerifiesAndAppendsLedgerWithoutRawMetadata(t *testing.T) {
	t.Parallel()

	fixture := setupLiveFixture(t, "create", "allow")
	fixture.writeLive("old@example.com")
	fixture.writeDesired("new@example.com")
	fixture.trustRecipe()

	result, err := Run(fixture.options(selectedpreview.CommandApply, "run-apply", true))
	require.NoError(t, err)
	require.NotNil(t, result.RunRecord)
	require.Len(t, result.LedgerEntries, 1)
	require.NotNil(t, result.Backup)
	require.Contains(t, readFile(t, filepath.Join(fixture.liveRoot, "config.yaml")), "new@example.com")

	item := result.RunRecord.Items[0]
	require.Equal(t, v2ledger.ItemResultVerified, item.Result)
	require.True(t, item.Verification.Verified)
	require.Len(t, item.BackupRefs, 1)
	backupMetadata := readFile(t, filepath.Join(fixture.stateRoot, "backups", "run-apply", "backup.yaml"))
	require.NotContains(t, backupMetadata, "old@example.com")
	require.NotContains(t, backupMetadata, "new@example.com")
	runRecord := readFile(t, filepath.Join(fixture.stateRoot, "ledger", "runs", "run-apply.json"))
	require.NotContains(t, runRecord, "old@example.com")
	require.NotContains(t, runRecord, "new@example.com")
	ledgerPayload := readFile(t, filepath.Join(fixture.stateRoot, "ledger", "ledger.jsonl"))
	require.NotContains(t, ledgerPayload, "old@example.com")
	require.NotContains(t, ledgerPayload, "new@example.com")

	payloadRel := result.Backup.Items[0].PayloadRelPath
	payloadPath := filepath.Join(fixture.stateRoot, "backups", "run-apply", filepath.FromSlash(payloadRel))
	require.Contains(t, readFile(t, payloadPath), "old@example.com")
	info, err := os.Stat(payloadPath)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	reportJSON := mustJSON(t, result.Report)
	require.NotContains(t, reportJSON, "old@example.com")
	require.NotContains(t, reportJSON, "new@example.com")
	require.True(t, result.Report.Items[0].Mutated)
	require.Equal(t, "verified", result.Report.Items[0].Mutation.Result)
}

func TestBundledGitApplyUsesSelectedIdentityOnlyAndKeepsCredentialDataLocalToBackupPayload(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	home := t.TempDir()
	stateRoot := t.TempDir()
	helperSecret := "credential-helper-secret"
	writeLiveFile(t, filepath.Join(repoRoot, "dotfiles-manager.v2.yaml"), "schema: dotfiles-manager.v2.root-config\nschemaVersion: 1\nactiveProfileStack: default\n")
	writeLiveFile(t, filepath.Join(repoRoot, "profiles", "stacks", "default.yaml"), "schema: dotfiles-manager.v2.profile-stack\nschemaVersion: 1\nprofileStack: [global]\n")
	writeLiveFile(t, filepath.Join(repoRoot, "profiles", "layers", "global.yaml"), "schema: dotfiles-manager.v2.profile-layer\nschemaVersion: 1\nselections:\n  git:\n    settings:\n      user.email:\n        scope: user\n")
	writeLiveFile(t, filepath.Join(repoRoot, "desired", "user", "leon", "targets", "git", "settings.yaml"), "schema: dotfiles-manager.v2.desired-settings\nschemaVersion: 1\nvalues:\n  user.email:\n    intent: set\n    kind: string\n    value: new@example.com\n")
	writeLiveFile(t, filepath.Join(home, ".gitconfig"), "[credential]\n\thelper = "+helperSecret+"\n[user]\n\temail = old@example.com\n")

	result, err := Run(Options{
		Command:   selectedpreview.CommandApply,
		RepoRoot:  repoRoot,
		StateRoot: stateRoot,
		Ref:       "git:user.email",
		UserID:    "leon",
		Confirmed: true,
		RunID:     "run-git-apply",
		LocationRoots: map[string]map[string]string{
			recipe.GitTarget: {"home": home},
		},
		Now: func() time.Time {
			return time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result.RunRecord)
	require.NotNil(t, result.Backup)
	require.Contains(t, readFile(t, filepath.Join(home, ".gitconfig")), "new@example.com")
	require.Contains(t, readFile(t, filepath.Join(home, ".gitconfig")), helperSecret)
	require.Equal(t, v2ledger.ItemResultVerified, result.RunRecord.Items[0].Result)
	require.Equal(t, recipe.IniFileDriverID, result.RunRecord.Items[0].Driver)

	reportJSON := mustJSON(t, result.Report)
	runRecord := readFile(t, filepath.Join(stateRoot, "ledger", "runs", "run-git-apply.json"))
	ledgerPayload := readFile(t, filepath.Join(stateRoot, "ledger", "ledger.jsonl"))
	backupMetadata := readFile(t, filepath.Join(stateRoot, "backups", "run-git-apply", "backup.yaml"))
	for _, payload := range []string{reportJSON, runRecord, ledgerPayload, backupMetadata} {
		require.NotContains(t, payload, "old@example.com")
		require.NotContains(t, payload, "new@example.com")
		require.NotContains(t, payload, helperSecret)
	}

	payloadRel := result.Backup.Items[0].PayloadRelPath
	backupPayload := readFile(t, filepath.Join(stateRoot, "backups", "run-git-apply", filepath.FromSlash(payloadRel)))
	require.Contains(t, backupPayload, "old@example.com")
	require.Contains(t, backupPayload, helperSecret)
}

func TestApplyNoopWritesRunRecordButNoLedgerEntry(t *testing.T) {
	t.Parallel()

	fixture := setupLiveFixture(t, "create", "allow")
	fixture.writeLive("same@example.com")
	fixture.writeDesired("same@example.com")
	fixture.trustRecipe()

	result, err := Run(fixture.options(selectedpreview.CommandApply, "run-noop", true))
	require.NoError(t, err)
	require.NotNil(t, result.RunRecord)
	require.Empty(t, result.LedgerEntries)
	require.Equal(t, v2ledger.ItemResultUnchanged, result.RunRecord.Items[0].Result)
	require.NoFileExists(t, filepath.Join(fixture.stateRoot, "ledger", "ledger.jsonl"))
}

func TestSaveChangedWritesDesiredAndLedgerWithoutRawMetadata(t *testing.T) {
	t.Parallel()

	fixture := setupLiveFixture(t, "create", "allow")
	fixture.writeLive("current@example.com")
	fixture.trustRecipe()

	result, err := Run(fixture.options(selectedpreview.CommandSave, "run-save", true))
	require.NoError(t, err)
	require.NotNil(t, result.RunRecord)
	require.Len(t, result.LedgerEntries, 1)
	require.Contains(t, readFile(t, fixture.desiredPath()), "current@example.com")
	runRecord := readFile(t, filepath.Join(fixture.stateRoot, "ledger", "runs", "run-save.json"))
	ledgerPayload := readFile(t, filepath.Join(fixture.stateRoot, "ledger", "ledger.jsonl"))
	require.NotContains(t, runRecord, "current@example.com")
	require.NotContains(t, ledgerPayload, "current@example.com")
	require.Equal(t, v2ledger.ItemResultVerified, result.RunRecord.Items[0].Result)
}

func TestSaveExistingDesiredRecordsBeforeSnapshot(t *testing.T) {
	t.Parallel()

	fixture := setupLiveFixture(t, "create", "allow")
	fixture.writeLive("current@example.com")
	fixture.writeDesired("old@example.com")
	fixture.trustRecipe()

	result, err := Run(fixture.options(selectedpreview.CommandSave, "run-save-existing", true))
	require.NoError(t, err)
	require.Equal(t, v2ledger.ItemResultVerified, result.RunRecord.Items[0].Result)
	require.True(t, result.RunRecord.Items[0].Before.Exists)
	require.True(t, result.RunRecord.Items[0].Desired.Exists)
	require.NotEqual(t, result.RunRecord.Items[0].Before.Hash, result.RunRecord.Items[0].Desired.Hash)
}

func TestSaveSecretLiveValueBlocksBeforeDesiredWrite(t *testing.T) {
	t.Parallel()

	fixture := setupLiveFixture(t, "create", "allow")
	secret := "sk-proj-abcdefghijklmnopqrstuvwxyz1234567890"
	fixture.writeLive(secret)
	fixture.trustRecipe()

	result, err := Run(fixture.options(selectedpreview.CommandSave, "run-secret", true))
	require.Error(t, err)
	var previewErr *selectedpreview.Error
	require.True(t, errors.As(err, &previewErr))
	require.Equal(t, CodePlanBlocked, previewErr.Code)
	require.NoFileExists(t, fixture.desiredPath())
	require.NotContains(t, mustJSON(t, result.Report), secret)
	require.NoDirExists(t, filepath.Join(fixture.stateRoot, "ledger"))
}

func TestApplySecretDesiredBlocksBeforeBackupOrMutation(t *testing.T) {
	t.Parallel()

	fixture := setupLiveFixture(t, "create", "allow")
	secret := "sk-proj-abcdefghijklmnopqrstuvwxyz1234567890"
	fixture.writeLive("old@example.com")
	fixture.writeDesired(secret)
	fixture.trustRecipe()

	result, err := Run(fixture.options(selectedpreview.CommandApply, "run-secret-apply", true))
	require.Error(t, err)
	var previewErr *selectedpreview.Error
	require.True(t, errors.As(err, &previewErr))
	require.Equal(t, CodePlanBlocked, previewErr.Code)
	require.Contains(t, readFile(t, filepath.Join(fixture.liveRoot, "config.yaml")), "old@example.com")
	require.NoDirExists(t, filepath.Join(fixture.stateRoot, "backups"))
	require.NotContains(t, mustJSON(t, result.Report), secret)
}

func TestNoYesChangedRequiresConfirmationButNoopDoesNot(t *testing.T) {
	t.Parallel()

	changed := setupLiveFixture(t, "create", "allow")
	changed.writeLive("old@example.com")
	changed.writeDesired("new@example.com")
	changed.trustRecipe()
	_, err := Run(changed.options(selectedpreview.CommandApply, "run-confirm", false))
	require.Error(t, err)
	var previewErr *selectedpreview.Error
	require.True(t, errors.As(err, &previewErr))
	require.Equal(t, 4, previewErr.ExitCode())
	require.Contains(t, readFile(t, filepath.Join(changed.liveRoot, "config.yaml")), "old@example.com")

	noop := setupLiveFixture(t, "create", "allow")
	noop.writeLive("same@example.com")
	noop.writeDesired("same@example.com")
	noop.trustRecipe()
	result, err := Run(noop.options(selectedpreview.CommandApply, "run-no-confirm", false))
	require.NoError(t, err)
	require.Equal(t, selectedpreview.SummaryOK, result.Report.Summary.Status)
	require.NoDirExists(t, filepath.Join(noop.stateRoot, "ledger"))
}

func TestApplyMissingDesiredAndPolicyDeniedPathsBlockBeforeMutation(t *testing.T) {
	t.Parallel()

	t.Run("missing desired", func(t *testing.T) {
		t.Parallel()
		fixture := setupLiveFixture(t, "create", "allow")
		fixture.writeLive("old@example.com")
		fixture.trustRecipe()

		_, err := Run(fixture.options(selectedpreview.CommandApply, "run-missing", true))
		require.Error(t, err)
		require.Contains(t, readFile(t, filepath.Join(fixture.liveRoot, "config.yaml")), "old@example.com")
		require.NoDirExists(t, filepath.Join(fixture.stateRoot, "backups"))
	})

	t.Run("create denied", func(t *testing.T) {
		t.Parallel()
		fixture := setupLiveFixture(t, "reject", "allow")
		fixture.writeDesired("new@example.com")
		fixture.trustRecipe()

		_, err := Run(fixture.options(selectedpreview.CommandApply, "run-create-denied", true))
		require.Error(t, err)
		require.NoFileExists(t, filepath.Join(fixture.liveRoot, "config.yaml"))
		require.NoDirExists(t, filepath.Join(fixture.stateRoot, "backups"))
	})

	t.Run("delete denied", func(t *testing.T) {
		t.Parallel()
		fixture := setupLiveFixture(t, "create", "reject")
		fixture.writeLive("old@example.com")
		fixture.writeDesiredDelete()
		fixture.trustRecipe()

		_, err := Run(fixture.options(selectedpreview.CommandApply, "run-delete-denied", true))
		require.Error(t, err)
		require.Contains(t, readFile(t, filepath.Join(fixture.liveRoot, "config.yaml")), "old@example.com")
		require.NoDirExists(t, filepath.Join(fixture.stateRoot, "backups"))
	})
}

func TestVerificationFailedRecordsFailedRunWithoutLedgerSuccess(t *testing.T) {
	t.Parallel()

	fixture := setupLiveFixture(t, "create", "allow")
	fixture.writeLive("old@example.com")
	fixture.writeDesired("new@example.com")
	fixture.trustRecipe()
	opts := fixture.options(selectedpreview.CommandApply, "run-verify-failed", true)
	opts.AfterApply = func(plan *selectedvalue.Plan) error {
		require.NotNil(t, plan)
		return os.WriteFile(plan.Path, []byte("user:\n  email: drift@example.com\n"), 0o644)
	}

	result, err := Run(opts)
	require.Error(t, err)
	require.NotNil(t, result.RunRecord)
	require.Equal(t, v2ledger.ItemResultFailed, result.RunRecord.Items[0].Result)
	require.False(t, result.RunRecord.Items[0].Verification.Verified)
	require.Empty(t, result.LedgerEntries)
	require.NoFileExists(t, filepath.Join(fixture.stateRoot, "ledger", "ledger.jsonl"))
	require.NotNil(t, result.Backup)
	runRecord := readFile(t, filepath.Join(fixture.stateRoot, "ledger", "runs", "run-verify-failed.json"))
	require.NotContains(t, runRecord, "old@example.com")
	require.NotContains(t, runRecord, "new@example.com")
	require.NotContains(t, runRecord, "drift@example.com")
}

func TestApplyCorruptBackupMetadataReportsBackupReadError(t *testing.T) {
	t.Parallel()

	fixture := setupLiveFixture(t, "create", "allow")
	fixture.writeLive("old@example.com")
	fixture.writeDesired("new@example.com")
	fixture.trustRecipe()
	opts := fixture.options(selectedpreview.CommandApply, "run-corrupt-backup", true)
	opts.AfterApply = func(plan *selectedvalue.Plan) error {
		return os.WriteFile(filepath.Join(fixture.stateRoot, "backups", "run-corrupt-backup", "backup.yaml"), []byte("schema: [broken\n"), 0o644)
	}

	result, err := Run(opts)
	require.Error(t, err)
	var previewErr *selectedpreview.Error
	require.True(t, errors.As(err, &previewErr))
	require.Equal(t, "selectedlive.backup.read", previewErr.Code)
	require.NotNil(t, result.RunRecord)
}

func TestDirectExecuteBranchesBlockedBeforeMutation(t *testing.T) {
	t.Parallel()

	t.Run("apply missing desired", func(t *testing.T) {
		t.Parallel()
		fixture := setupLiveFixture(t, "create", "allow")
		fixture.writeLive("old@example.com")
		fixture.trustRecipe()
		setting, rec, trustContext, resourceID, resource, preItem := executionContextForFixture(t, fixture)
		item := executeApply(fixture.repoRoot, "run-direct-missing", fixedSelectedLiveTime(), mustStore(t, fixture.stateRoot), setting, rec, trustContext, resourceID, resource, nil, preItem, nil)
		require.Equal(t, v2ledger.ItemResultFailed, item.Result)
		require.Equal(t, "selectedlive.apply.missingDesired", item.Diagnostics[0].Code)
	})

	t.Run("apply unmanaged desired", func(t *testing.T) {
		t.Parallel()
		fixture := setupLiveFixture(t, "create", "allow")
		fixture.writeLive("old@example.com")
		fixture.writeDesiredUnmanaged()
		fixture.trustRecipe()
		setting, rec, trustContext, resourceID, resource, preItem := executionContextForFixture(t, fixture)
		item := executeApply(fixture.repoRoot, "run-direct-unmanaged", fixedSelectedLiveTime(), mustStore(t, fixture.stateRoot), setting, rec, trustContext, resourceID, resource, nil, preItem, nil)
		require.Equal(t, v2ledger.ItemResultSkipped, item.Result)
	})

	t.Run("apply secret desired", func(t *testing.T) {
		t.Parallel()
		fixture := setupLiveFixture(t, "create", "allow")
		fixture.writeLive("old@example.com")
		fixture.writeDesired("sk-proj-abcdefghijklmnopqrstuvwxyz1234567890")
		fixture.trustRecipe()
		setting, rec, trustContext, resourceID, resource, preItem := executionContextForFixture(t, fixture)
		item := executeApply(fixture.repoRoot, "run-direct-secret", fixedSelectedLiveTime(), mustStore(t, fixture.stateRoot), setting, rec, trustContext, resourceID, resource, nil, preItem, nil)
		require.Equal(t, v2ledger.ItemResultFailed, item.Result)
		require.Equal(t, "desired.writeSafety.secretDetected", item.Diagnostics[0].Code)
	})

	t.Run("save read failure", func(t *testing.T) {
		t.Parallel()
		fixture := setupLiveFixture(t, "create", "allow")
		writeLiveFile(t, filepath.Join(fixture.liveRoot, "config.yaml"), "user:\n  email:\n    nested: true\n")
		fixture.trustRecipe()
		setting, rec, trustContext, resourceID, resource, preItem := executionContextForFixture(t, fixture)
		item := executeSave(fixture.repoRoot, "run-direct-save-read", setting, rec, trustContext, resourceID, resource, nil, preItem)
		require.Equal(t, v2ledger.ItemResultFailed, item.Result)
	})
}

func TestUnmanagedApplySkipsAndSaveNoopRecordsUnchanged(t *testing.T) {
	t.Parallel()

	t.Run("unmanaged apply skips", func(t *testing.T) {
		t.Parallel()
		fixture := setupLiveFixture(t, "create", "allow")
		fixture.writeLive("old@example.com")
		fixture.writeDesiredUnmanaged()
		fixture.trustRecipe()

		result, err := Run(fixture.options(selectedpreview.CommandApply, "run-unmanaged", true))
		require.NoError(t, err)
		require.Equal(t, v2ledger.ItemResultSkipped, result.RunRecord.Items[0].Result)
		require.Empty(t, result.LedgerEntries)
	})

	t.Run("save noop unchanged", func(t *testing.T) {
		t.Parallel()
		fixture := setupLiveFixture(t, "create", "allow")
		fixture.writeLive("same@example.com")
		fixture.writeDesired("same@example.com")
		fixture.trustRecipe()

		result, err := Run(fixture.options(selectedpreview.CommandSave, "run-save-noop", true))
		require.NoError(t, err)
		require.Equal(t, v2ledger.ItemResultUnchanged, result.RunRecord.Items[0].Result)
		require.Empty(t, result.LedgerEntries)
	})
}

func TestSelectedLiveHelperBranches(t *testing.T) {
	t.Parallel()

	_, err := normalizeCommand("bad")
	require.Error(t, err)
	require.Equal(t, "save", mustNormalizeCommand(t, selectedpreview.CommandSave))
	require.Equal(t, "apply", mustNormalizeCommand(t, selectedpreview.CommandApply))

	ref, err := parseRef("")
	require.NoError(t, err)
	require.True(t, ref.Empty)
	ref, err = parseRef("test.app")
	require.NoError(t, err)
	require.Equal(t, "test.app", ref.Target)
	_, err = parseRef("bad:ref:extra")
	require.Error(t, err)
	_, err = parseRef("desired://user/leon/targets/test.app/settings#identity.email")
	require.Error(t, err)

	filePath := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(filePath, []byte("x"), 0o644))
	_, err = normalizeRepoRoot(filePath)
	require.Error(t, err)
	absState, err := normalizeStateRoot(t.TempDir(), filepath.Join(t.TempDir(), "state"))
	require.NoError(t, err)
	require.True(t, filepath.IsAbs(absState))
	require.NotEmpty(t, runID(Options{}, time.Date(2026, 6, 8, 12, 0, 0, 1, time.UTC)))
	require.NotNil(t, clock(Options{}))
	require.Empty(t, formatTime(time.Time{}))
	require.Equal(t, "fallback", defaultString("", "fallback"))

	for _, selected := range []selectedvalue.Desired{
		selectedvalue.Delete(),
		selectedvalue.SetString("x"),
		selectedvalue.SetBool(true),
		selectedvalue.SetNumber(json.Number("7")),
		selectedvalue.SetNull(),
	} {
		_, err := desiredValueFromSelected(selected)
		require.NoError(t, err)
	}
	_, err = desiredValueFromSelected(selectedvalue.Desired{})
	require.Error(t, err)

	diag := desiredSafetyDiagnostic("fallback", &desired.SafetyError{Diagnostics: []desired.Diagnostic{{Code: "desired.secret", Message: "safe", Path: "$.value"}}}, "path")
	require.Equal(t, "desired.secret", diag.Code)
	require.Equal(t, "safe", diag.Message)
	require.Equal(t, "selectedlive.failed", safeDiagnostic("", nil, "").Code)
	require.Equal(t, "fallback", selectedValueCurrentDiagnostic(nil, "fallback", nil).Code)
	require.Equal(t, "fallback", selectedValuePlanDiagnostic(nil, "fallback", nil).Code)
	require.Equal(t, "fallback", selectedValuePlanDiagnosticFromPlan(nil, "fallback", nil).Code)
	require.Nil(t, selectedValueBackupRefs(nil))
	require.Nil(t, selectedValueBackupRefs(&selectedvalue.ApplyResult{}))
	markReportItem(nil, "missing", v2ledger.ItemRecord{})
	require.Empty(t, itemReportMap(nil))
	require.True(t, hasPlanBlocker(nil))
	require.False(t, requiresConfirmation(nil))
	require.False(t, isActionable("unknown", selectedpreview.Item{}))
	attachReportError(nil, "code", "message", nil)
	require.Equal(t, "state://ledger/runs/run", selectedValueArtifactRefs("run", resolutionSetting("test.app", "identity.email"), "").RunRecord)
	require.Equal(t, "run", runIDFromRunRecordRef("state://ledger/runs/run"))
	require.Equal(t, v2ledger.ItemResultFailed, resultForVerification(v2ledger.ItemResultVerified, false))
	require.Equal(t, "verification-failed", verificationResult(false))
}

func TestRunAndRuntimeContextErrorBranches(t *testing.T) {
	t.Parallel()

	result, err := Run(Options{Command: "bad"})
	require.Error(t, err)
	require.Equal(t, "selectedlive.command.invalid", result.Report.Error.Code)

	fixture := setupLiveFixture(t, "create", "allow")
	fixture.writeLive("old@example.com")
	fixture.writeDesired("new@example.com")
	fixture.trustRecipe()
	dry := fixture.options(selectedpreview.CommandApply, "run-dry", false)
	dry.DryRun = true
	result, err = Run(dry)
	require.NoError(t, err)
	require.True(t, result.Report.DryRun)

	_, _, _, _, _, err = runtimeContext(fixture.repoRoot, fixture.stateRoot, resolutionSetting("missing.app", "identity.email"))
	require.Error(t, err)

	untrusted := setupLiveFixture(t, "create", "allow")
	untrusted.writeLive("old@example.com")
	untrusted.writeDesired("new@example.com")
	_, _, _, _, _, err = runtimeContext(untrusted.repoRoot, untrusted.stateRoot, resolutionSetting("test.app", "identity.email"))
	require.Error(t, err)

	badRunID := setupLiveFixture(t, "create", "allow")
	badRunID.writeLive("same@example.com")
	badRunID.writeDesired("same@example.com")
	badRunID.trustRecipe()
	_, err = Run(badRunID.options(selectedpreview.CommandApply, "../bad", true))
	require.Error(t, err)
}

func TestSelectedLiveRemainingHelperBranches(t *testing.T) {
	t.Parallel()

	fixture := setupLiveFixture(t, "create", "allow")
	fixture.writeLive("old@example.com")
	fixture.writeDesired("new@example.com")
	fixture.trustRecipe()
	read, err := desired.ReadSelectedValueForSetting(fixture.repoRoot, resolutionSetting("test.app", "identity.email"))
	require.NoError(t, err)
	require.False(t, selectedValueDesiredSnapshot(fixture.recipe, resolutionSetting("test.app", "identity.email"), map[string]string{"config": fixture.liveRoot}, desired.ReadResult{Status: desired.StatusPresent}, recipe.WriteSafetyContext{}).Exists)
	require.False(t, selectedValueDesiredSnapshot(fixture.recipe, resolutionSetting("test.app", "identity.email"), map[string]string{"config": fixture.liveRoot}, desired.ReadResult{Status: desired.StatusPresent, Desired: &selectedvalue.Desired{}}, recipe.WriteSafetyContext{}).Exists)
	require.True(t, selectedValueDesiredSnapshot(fixture.recipe, resolutionSetting("test.app", "identity.email"), map[string]string{"config": fixture.liveRoot}, read, trustedContextForFixture(t, fixture)).Exists)

	diagPlan := &selectedvalue.Plan{Diagnostics: []selectedvalue.Diagnostic{{Code: "plan.code", Message: "safe", Path: "$"}}}
	require.Equal(t, "plan.code", selectedValuePlanDiagnosticFromPlan(diagPlan, "fallback", nil).Code)
	require.Equal(t, "plan.code", selectedValueCurrentDiagnostic(&selectedvalue.CurrentDesired{Plan: diagPlan}, "fallback", nil).Code)
	require.Equal(t, "selectedlive.driver.internal-error", safeDiagnostic("", &filedriver.Error{Code: filedriver.CodeInternal}, "").Code)

	settings := []resolution.ResolvedSetting{
		resolutionSetting("a", "one"),
		resolutionSetting("b", "two"),
	}
	require.Len(t, filterSettings(settings, parsedRef{Empty: true}), 2)
	require.Len(t, filterSettings(settings, parsedRef{Target: "a"}), 1)
	require.Len(t, filterSettings(settings, parsedRef{Target: "a", Setting: "missing"}), 0)
}

type liveFixture struct {
	repoRoot  string
	liveRoot  string
	stateRoot string
	recipe    *recipe.Recipe
	t         *testing.T
}

func setupLiveFixture(t *testing.T, createPolicy string, deletePolicy string) liveFixture {
	t.Helper()
	repoRoot := t.TempDir()
	liveRoot := t.TempDir()
	stateRoot := t.TempDir()
	writeLiveFile(t, filepath.Join(repoRoot, "dotfiles-manager.v2.yaml"), "schema: dotfiles-manager.v2.root-config\nschemaVersion: 1\nactiveProfileStack: default\n")
	writeLiveFile(t, filepath.Join(repoRoot, "profiles", "stacks", "default.yaml"), "schema: dotfiles-manager.v2.profile-stack\nschemaVersion: 1\nprofileStack: [global]\n")
	writeLiveFile(t, filepath.Join(repoRoot, "profiles", "layers", "global.yaml"), "schema: dotfiles-manager.v2.profile-layer\nschemaVersion: 1\nselections:\n  test.app:\n    settings:\n      identity.email:\n        scope: user\n")
	body := liveRecipeBody(liveRoot, createPolicy, deletePolicy)
	writeLiveFile(t, filepath.Join(repoRoot, "recipes", "local", "test.app", "recipe.yaml"), body)
	rec, err := recipe.Decode("recipe.yaml", strings.NewReader(body))
	require.NoError(t, err)
	return liveFixture{repoRoot: repoRoot, liveRoot: liveRoot, stateRoot: stateRoot, recipe: rec, t: t}
}

func (f liveFixture) options(command string, runID string, confirmed bool) Options {
	return Options{
		Command:   command,
		RepoRoot:  f.repoRoot,
		StateRoot: f.stateRoot,
		Ref:       "test.app:identity.email",
		UserID:    "leon",
		Confirmed: confirmed,
		RunID:     runID,
		Now: func() time.Time {
			return time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
		},
	}
}

func (f liveFixture) writeLive(email string) {
	writeLiveFile(f.t, filepath.Join(f.liveRoot, "config.yaml"), "user:\n  email: "+email+"\n")
}

func (f liveFixture) writeDesired(email string) {
	writeLiveFile(f.t, f.desiredPath(), "schema: dotfiles-manager.v2.desired-settings\nschemaVersion: 1\nvalues:\n  identity.email:\n    intent: set\n    kind: string\n    value: "+email+"\n")
}

func (f liveFixture) writeDesiredDelete() {
	writeLiveFile(f.t, f.desiredPath(), "schema: dotfiles-manager.v2.desired-settings\nschemaVersion: 1\nvalues:\n  identity.email:\n    intent: delete\n")
}

func (f liveFixture) writeDesiredUnmanaged() {
	writeLiveFile(f.t, f.desiredPath(), "schema: dotfiles-manager.v2.desired-settings\nschemaVersion: 1\nvalues:\n  identity.email:\n    intent: unmanaged\n")
}

func (f liveFixture) desiredPath() string {
	return filepath.Join(f.repoRoot, "desired", "user", "leon", "targets", "test.app", "settings.yaml")
}

func (f liveFixture) trustRecipe() {
	_, err := recipe.RecordLocalRecipeTrust(f.repoRoot, f.stateRoot, f.recipe)
	require.NoError(f.t, err)
}

func liveRecipeBody(liveRoot string, createPolicy string, deletePolicy string) string {
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
      createMissing: ` + createPolicy + `
      duplicatePolicy: reject
      deleteKey: ` + deletePolicy + `
`
}

func writeLiveFile(t *testing.T, path string, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

func mustJSON(t *testing.T, report *selectedpreview.Report) string {
	t.Helper()
	payload, err := json.Marshal(report)
	require.NoError(t, err)
	return string(payload)
}

func mustNormalizeCommand(t *testing.T, command string) string {
	t.Helper()
	normalized, err := normalizeCommand(command)
	require.NoError(t, err)
	return normalized
}

func resolutionSetting(target string, setting string) resolution.ResolvedSetting {
	rel := filepath.Join("desired", "user", "leon", "targets", target, "settings.yaml")
	return resolution.ResolvedSetting{TargetID: target, SettingID: setting, Scope: "user", Subject: "leon", DesiredURI: "desired://user/leon/targets/" + target + "/settings#" + setting, DesiredRelPath: rel}
}

func trustedContextForFixture(t *testing.T, fixture liveFixture) recipe.WriteSafetyContext {
	t.Helper()
	eval, err := recipe.EvaluateRecipeTrust(fixture.repoRoot, fixture.stateRoot, recipe.RecipeSourceLocal, fixture.recipe)
	require.NoError(t, err)
	require.Equal(t, recipe.TrustStatusTrusted, eval.Status)
	return eval.WriteSafetyContext(recipe.WriteSafetyContext{})
}

func executionContextForFixture(t *testing.T, fixture liveFixture) (resolution.ResolvedSetting, *recipe.Recipe, recipe.WriteSafetyContext, string, recipe.Resource, selectedpreview.Item) {
	t.Helper()
	profile, err := resolution.Resolve(fixture.repoRoot, resolution.ResolveOptions{UserID: "leon"})
	require.NoError(t, err)
	require.Len(t, profile.Settings, 1)
	setting := profile.Settings[0]
	rec, _, trustContext, resourceID, resource, err := runtimeContext(fixture.repoRoot, fixture.stateRoot, setting)
	require.NoError(t, err)
	preItem := selectedpreview.Item{
		TargetRef:      setting.TargetID,
		SettingRef:     setting.Ref(),
		DesiredURI:     setting.DesiredURI,
		DesiredRelPath: setting.DesiredRelPath,
		Resource: selectedpreview.ResourceInfo{
			ID:       resourceID,
			DriverID: resource.Driver,
			Path:     filepath.Join(fixture.liveRoot, "config.yaml"),
		},
	}
	return setting, rec, trustContext, resourceID, resource, preItem
}

func mustStore(t *testing.T, stateRoot string) *v2ledger.Store {
	t.Helper()
	store, err := v2ledger.NewStore(stateRoot)
	require.NoError(t, err)
	return store
}

func fixedSelectedLiveTime() time.Time {
	return time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
}

func TestSelectedLiveAdditionalHelperBranches(t *testing.T) {
	t.Parallel()

	setting := resolutionSetting("test.app", "identity.email")
	baseItem := selectedpreview.Item{
		TargetRef:      setting.TargetID,
		SettingRef:     setting.Ref(),
		DesiredURI:     setting.DesiredURI,
		DesiredRelPath: setting.DesiredRelPath,
		Resource: selectedpreview.ResourceInfo{
			ID:       "config-email",
			DriverID: recipe.YAMLFileDriverID,
			Path:     "/tmp/config.yaml",
		},
		Current: selectedpreview.Snapshot{Exists: true, SHA256: "before", Normalizer: "yaml-file.selected-scalar.v1"},
		Desired: selectedpreview.DesiredInfo{Snapshot: selectedpreview.Snapshot{Exists: false, Normalizer: "yaml-file.selected-scalar.v1"}},
	}
	unchanged := unchangedItemRecord(selectedpreview.CommandApply, "run-helper", setting, baseItem)
	require.Equal(t, v2ledger.ItemResultUnchanged, unchanged.Result)
	require.True(t, unchanged.Desired.Exists, "absent desired snapshot should reuse existing before state for unchanged live items")

	absentBeforeItem := baseItem
	absentBeforeItem.Current = selectedpreview.Snapshot{Exists: false, Normalizer: "yaml-file.selected-scalar.v1"}
	absentBeforeItem.Desired.Snapshot = selectedpreview.Snapshot{Exists: true, SHA256: "desired", Normalizer: "yaml-file.selected-scalar.v1"}
	unchanged = unchangedItemRecord(selectedpreview.CommandSave, "run-helper", setting, absentBeforeItem)
	require.True(t, unchanged.Before.Exists, "absent before snapshot should reuse desired state for unchanged desired items")

	failed := failedItemRecord(selectedpreview.CommandApply, "run-helper", setting, "", recipe.Resource{}, baseItem, v2ledger.Diagnostic{}, []string{"state://backups/run-helper/items/one"})
	require.Equal(t, v2ledger.ItemResultFailed, failed.Result)
	require.Equal(t, "selectedlive.failed", failed.Diagnostics[0].Code)
	require.Equal(t, "selected-value live operation failed", failed.Diagnostics[0].Message)
	require.Equal(t, "state://backups/run-helper/items/one", failed.ArtifactRefs.Backup)
	require.Equal(t, "state://backups/run-helper/items/one/payload", failed.ArtifactRefs.BackupPayload)

	normalized := normalizeSelectedValueItem(v2ledger.ItemRecord{Driver: recipe.JSONFileDriverID})
	require.Equal(t, v2ledger.SelectedValueDriverVersion(recipe.JSONFileDriverID), normalized.DriverVersion)
	require.Equal(t, normalized.DriverVersion, normalized.Before.DriverVersion)
	require.Equal(t, normalized.DriverVersion, normalized.Desired.DriverVersion)
	require.Equal(t, normalized.DriverVersion, normalized.VerifiedState.DriverVersion)

	applyNil := selectedValueItemFromApply("run-helper", setting, "config-email", recipe.Resource{Driver: recipe.YAMLFileDriverID}, baseItem, nil)
	require.Equal(t, v2ledger.ItemResultFailed, applyNil.Result)
	require.False(t, applyNil.Verification.Verified)
	applyNoDesired := selectedValueItemFromApply("run-helper", setting, "config-email", recipe.Resource{Driver: recipe.YAMLFileDriverID}, baseItem, &selectedvalue.ApplyResult{
		Plan:     &selectedvalue.Plan{Current: selectedvalue.Snapshot{Exists: true, SHA256: "before", Normalizer: "yaml-file.selected-scalar.v1"}},
		Mutated:  true,
		Verified: false,
	})
	require.Equal(t, v2ledger.ItemResultFailed, applyNoDesired.Result)
	require.False(t, applyNoDesired.Verification.Verified)

	store := mustStore(t, t.TempDir())
	backupHook := selectedValueBackupHook(store, "../bad", fixedSelectedLiveTime(), setting, "config-email", recipe.Resource{Driver: recipe.YAMLFileDriverID})
	_, err := backupHook(selectedvalue.BackupRequest{Path: "/tmp/config.yaml", Before: selectedvalue.Snapshot{Exists: true, SHA256: "hash", Normalizer: "yaml-file.selected-scalar.v1"}, BeforeFile: []byte("payload")})
	require.Error(t, err)

	driverDiagnostic := safeDiagnostic("fallback", &filedriver.Error{Code: filedriver.CodeNotFound, Op: "read"}, "config.yaml")
	require.Equal(t, "selectedlive.driver.not-found", driverDiagnostic.Code)
	require.Equal(t, "selected-value driver read failed", driverDiagnostic.Message)

	_, err = parseRef("Bad")
	require.Error(t, err)
	_, err = parseRef("test.app:Bad")
	require.Error(t, err)
	_, err = parseRef("test.app:")
	require.Error(t, err)

	_, err = normalizeRepoRoot("")
	require.Error(t, err)
	_, err = normalizeRepoRoot(filepath.Join(t.TempDir(), "missing"))
	require.Error(t, err)
	defaultState, err := normalizeStateRoot(t.TempDir(), "")
	require.NoError(t, err)
	require.True(t, filepath.IsAbs(defaultState))
	require.Equal(t, "explicit-run", runID(Options{RunID: " explicit-run "}, fixedSelectedLiveTime()))
	customNow := func() time.Time { return fixedSelectedLiveTime() }
	require.Equal(t, fixedSelectedLiveTime(), clock(Options{Now: customNow})())
	require.Equal(t, "2026-06-08T12:00:00Z", formatTime(fixedSelectedLiveTime()))
	require.Equal(t, "explicit", defaultString("explicit", "fallback"))

	report := &selectedpreview.Report{Items: []selectedpreview.Item{{SettingRef: "test.app:identity.email", State: v2status.StateBlockedSafety}}}
	require.True(t, hasPlanBlocker(report))
	report = &selectedpreview.Report{Items: []selectedpreview.Item{{SettingRef: "test.app:identity.email", PlannedAction: "blocked-driver"}}}
	require.True(t, hasPlanBlocker(report))
	report = &selectedpreview.Report{Items: []selectedpreview.Item{{SettingRef: "test.app:identity.email", PlannedAction: selectedpreview.PlannedActionWouldSave}}}
	require.True(t, requiresConfirmation(report))
	report = &selectedpreview.Report{Items: []selectedpreview.Item{{SettingRef: "test.app:identity.email", PlannedAction: selectedpreview.PlannedActionWouldPromote}}}
	require.True(t, requiresConfirmation(report))
	require.True(t, isActionable(selectedpreview.CommandSave, selectedpreview.Item{PlannedAction: selectedpreview.PlannedActionWouldSave}))
	require.True(t, isActionable(selectedpreview.CommandSave, selectedpreview.Item{PlannedAction: selectedpreview.PlannedActionWouldPromote}))
	require.True(t, isActionable(selectedpreview.CommandApply, selectedpreview.Item{PlannedAction: selectedpreview.PlannedActionWouldApply}))

	attachReportError(report, "safe.code", "safe message", map[string]any{"flag": "--yes"})
	require.Equal(t, selectedpreview.SummaryError, report.Summary.Status)
	require.Equal(t, "safe.code", report.Error.Code)

	finish := &selectedpreview.Report{}
	finishLiveReport(finish, v2ledger.RunRecord{Summary: v2ledger.RunSummary{Failed: 1}, Items: []v2ledger.ItemRecord{
		{Result: v2ledger.ItemResultVerified, Operation: selectedpreview.CommandApply},
		{Result: v2ledger.ItemResultVerified, Operation: selectedpreview.CommandSave},
		{Result: v2ledger.ItemResultFailed, Operation: selectedpreview.CommandApply},
	}})
	require.Equal(t, selectedpreview.SummaryError, finish.Summary.Status)
	require.Equal(t, 2, finish.Summary.Changed)
	require.Equal(t, 1, finish.Summary.Applied)
	require.Equal(t, 1, finish.Summary.Saved)
	require.Equal(t, 1, finish.Summary.Blocked)
}

func TestMacOSDefaultsReadOnlyRunBlocksSaveApplyBeforeLiveExecution(t *testing.T) {
	t.Parallel()

	for _, command := range []string{selectedpreview.CommandSave, selectedpreview.CommandApply} {
		t.Run(command, func(t *testing.T) {
			repoRoot := t.TempDir()
			stateRoot := t.TempDir()
			writeLiveFile(t, filepath.Join(repoRoot, "dotfiles-manager.v2.yaml"), "schema: dotfiles-manager.v2.root-config\nschemaVersion: 1\nactiveProfileStack: default\n")
			writeLiveFile(t, filepath.Join(repoRoot, "profiles", "stacks", "default.yaml"), "schema: dotfiles-manager.v2.profile-stack\nschemaVersion: 1\nprofileStack: [global]\n")
			writeLiveFile(t, filepath.Join(repoRoot, "profiles", "layers", "global.yaml"), "schema: dotfiles-manager.v2.profile-layer\nschemaVersion: 1\nselections:\n  test.defaults:\n    settings:\n      show-hidden-files:\n        scope: user\n")
			body := liveDefaultsRecipeBody()
			writeLiveFile(t, filepath.Join(repoRoot, "recipes", "local", "test.defaults", "recipe.yaml"), body)
			rec, err := recipe.Decode("defaults.yaml", strings.NewReader(body))
			require.NoError(t, err)
			_, err = recipe.RecordLocalRecipeTrust(repoRoot, stateRoot, rec)
			require.NoError(t, err)
			writeLiveFile(t, filepath.Join(repoRoot, "desired", "user", "leon", "targets", "test.defaults", "settings.yaml"), "schema: dotfiles-manager.v2.desired-settings\nschemaVersion: 1\nvalues:\n  show-hidden-files:\n    intent: set\n    kind: bool\n    value: true\n")
			runner := &liveDefaultsRunner{}

			result, err := Run(Options{Command: command, RepoRoot: repoRoot, StateRoot: stateRoot, Ref: "test.defaults:show-hidden-files", UserID: "leon", Confirmed: true, RunID: "run-defaults-" + command, MacOSDefaultsRunner: runner})
			require.Error(t, err)
			var previewErr *selectedpreview.Error
			require.True(t, errors.As(err, &previewErr))
			require.Equal(t, CodePlanBlocked, previewErr.Code)
			require.NotNil(t, result.Report)
			require.Equal(t, selectedpreview.SummaryError, result.Report.Summary.Status)
			require.Empty(t, runner.calls)
			require.NoDirExists(t, filepath.Join(stateRoot, "ledger"))
			require.NoDirExists(t, filepath.Join(stateRoot, "backups"))
			require.Equal(t, "selectedpreview.driver.readOnly", result.Report.Items[0].Diagnostics[0].Code)
		})
	}
}

type liveDefaultsRunner struct{ calls []string }

func (r *liveDefaultsRunner) Export(ctx context.Context, domain string, limits macosdefaultsdriver.OutputLimits) (macosdefaultsdriver.ExportResult, error) {
	r.calls = append(r.calls, domain)
	return macosdefaultsdriver.ExportResult{}, fmt.Errorf("defaults export should not be called during live save/apply planning")
}

func liveDefaultsRecipeBody() string {
	return `schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: test.defaults
displayName: Test Defaults
supportLevel: experimental
capability: read-only
locations:
  macos-defaults:
    default: macos-defaults://current-user
settings:
  show-hidden-files:
    label: Show hidden files
    supportLevel: experimental
    capability: read-only
    artifactForm: scalar
    sensitivity: low
    redaction: known-safe
    lifecycle: allowed
    scopeDefault: user
    resource: finder-show-hidden
resources:
  finder-show-hidden:
    driver: macos-defaults-readonly
    location: macos-defaults
    path: com.apple.finder
    capability: read-only
    sensitivity: low
    redaction: known-safe
    lifecycle: allowed
    selector:
      key: AppleShowAllFiles
`
}
