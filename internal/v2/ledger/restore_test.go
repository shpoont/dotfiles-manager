package ledger

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shpoont/dotfiles-manager/internal/v2/customfiles"
	"github.com/shpoont/dotfiles-manager/internal/v2/filedriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/filetreedriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/preview"
	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/shpoont/dotfiles-manager/internal/v2/resolution"
	"github.com/shpoont/dotfiles-manager/internal/v2/status"
	"github.com/stretchr/testify/require"
)

func TestRestoreCustomFilesDryRunPreviewsWithoutMutationOrNewLocalState(t *testing.T) {
	t.Parallel()

	root, liveRoot, profile, rec, stateRoot, store := setupRestoreFileSource(t, "live before\n", "desired after\n", "run-apply")
	livePath := filepath.Join(liveRoot, "config.txt")
	requireFile(t, livePath, "desired after\n")

	run, err := store.RestoreCustomFiles(CustomFilesRestoreOptions{
		SourceRunID:   "run-apply",
		RunID:         "run-restore-preview",
		Profile:       profile,
		Recipe:        rec,
		LocationRoots: map[string]string{"config": liveRoot},
		ProfileStack:  profile.Layers,
		DryRun:        true,
		StartedAt:     fixedTime().Add(2 * time.Second),
	})
	require.NoError(t, err)
	require.Equal(t, "run-apply", run.SourceBackup.RunID)
	require.Equal(t, preview.CommandRestore, run.Preview.Command)
	require.Equal(t, "run-restore-preview", run.Preview.RunID)
	require.Len(t, run.Preview.Items, 1)
	item := run.Preview.Items[0]
	require.Equal(t, RestoreCommand, item.Operation)
	require.Equal(t, "custom.files:file", item.SettingRef)
	require.Equal(t, preview.ResultWouldChange, item.Result)
	require.Equal(t, preview.BackupRequired, item.Backup.Policy)
	require.Contains(t, item.Backup.Message, "state://backups/run-apply")
	require.Equal(t, preview.ExitChanged, preview.ExitCode(run.Preview))
	require.Nil(t, run.RunRecord)
	require.Empty(t, run.LedgerEntries)
	requireFile(t, livePath, "desired after\n")
	assertMissing(t, filepath.Join(stateRoot, "ledger", "runs", "run-restore-preview.json"))
	assertMissing(t, filepath.Join(stateRoot, "backups", "run-restore-preview"))
	_ = root
}

func TestRestoreCustomFilesRequiresConfirmationBeforeLiveMutation(t *testing.T) {
	t.Parallel()

	_, liveRoot, profile, rec, stateRoot, store := setupRestoreFileSource(t, "live before\n", "desired after\n", "run-apply")
	livePath := filepath.Join(liveRoot, "config.txt")

	run, err := store.RestoreCustomFiles(CustomFilesRestoreOptions{
		SourceRunID:   "run-apply",
		RunID:         "run-restore-confirm",
		Profile:       profile,
		Recipe:        rec,
		LocationRoots: map[string]string{"config": liveRoot},
		ProfileStack:  profile.Layers,
		StartedAt:     fixedTime().Add(2 * time.Second),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "confirmation required")
	require.NotNil(t, run)
	require.Equal(t, preview.ExitInputRequired, preview.ExitCode(run.Preview))
	require.Len(t, run.Preview.Items, 1)
	require.Equal(t, preview.ResultBlocked, run.Preview.Items[0].Result)
	require.Equal(t, "restore-confirmation-required", run.Preview.Items[0].Diagnostics[0].Code)
	requireFile(t, livePath, "desired after\n")
	assertMissing(t, filepath.Join(stateRoot, "ledger", "runs", "run-restore-confirm.json"))
	assertMissing(t, filepath.Join(stateRoot, "backups", "run-restore-confirm"))
}

func TestRestoreCustomFilesAppliesFileBackupCreatesRollbackBackupAndLedger(t *testing.T) {
	t.Parallel()

	_, liveRoot, profile, rec, stateRoot, store := setupRestoreFileSource(t, "live before\n", "desired after\n", "run-apply")
	livePath := filepath.Join(liveRoot, "config.txt")

	run, err := store.RestoreCustomFiles(CustomFilesRestoreOptions{
		SourceRunID:   "run-apply",
		RunID:         "run-restore",
		Profile:       profile,
		Recipe:        rec,
		LocationRoots: map[string]string{"config": liveRoot},
		ProfileStack:  profile.Layers,
		Confirmed:     true,
		StartedAt:     fixedTime().Add(2 * time.Second),
	})
	require.NoError(t, err)
	requireFile(t, livePath, "live before\n")
	require.NotNil(t, run.RunRecord)
	require.Equal(t, RunStatusVerified, run.RunRecord.Status)
	require.Equal(t, RestoreCommand, run.RunRecord.Command)
	require.Len(t, run.RunRecord.Items, 1)
	item := run.RunRecord.Items[0]
	require.Equal(t, RestoreCommand, item.Operation)
	require.Equal(t, ItemResultVerified, item.Result)
	require.True(t, item.Verification.Verified)
	require.Equal(t, []string{"state://backups/run-apply/custom.files_file-config-file"}, item.SourceBackupRefs)
	require.Equal(t, "state://backups/run-apply/custom.files_file-config-file", item.ArtifactRefs.SourceBackup)
	require.Equal(t, "state://backups/run-apply/custom.files_file-config-file/payload", item.ArtifactRefs.SourceBackupPayload)
	require.Equal(t, []string{"state://backups/run-restore/custom.files_file-config-file"}, item.BackupRefs)
	require.Equal(t, "state://backups/run-restore/custom.files_file-config-file", item.ArtifactRefs.Backup)
	require.Equal(t, item.Desired.Hash, item.VerifiedState.Hash)

	restoreBackup := requireBackupMetadata(t, stateRoot, "run-restore")
	require.Len(t, restoreBackup.Items, 1)
	require.Equal(t, "state://backups/run-restore/custom.files_file-config-file", restoreBackup.Items[0].Ref)
	requireFile(t, filepath.Join(stateRoot, "backups", "run-restore", restoreBackup.Items[0].PayloadRelPath), "desired after\n")
	require.NotNil(t, run.BackupBeforeRestore)

	entries := requireLedgerEntries(t, stateRoot)
	require.Len(t, entries, 2)
	require.Equal(t, RestoreCommand, entries[1].Command)
	require.Equal(t, "run-restore", entries[1].RunID)
	require.Equal(t, []string{"state://backups/run-apply/custom.files_file-config-file"}, entries[1].Item.SourceBackupRefs)
	require.Equal(t, []string{"state://backups/run-restore/custom.files_file-config-file"}, entries[1].Item.BackupRefs)

	noChange, err := store.RestoreCustomFiles(CustomFilesRestoreOptions{
		SourceRunID:   "run-apply",
		RunID:         "run-restore-unchanged",
		Profile:       profile,
		Recipe:        rec,
		LocationRoots: map[string]string{"config": liveRoot},
		ProfileStack:  profile.Layers,
		Confirmed:     true,
		StartedAt:     fixedTime().Add(3 * time.Second),
	})
	require.NoError(t, err)
	require.Equal(t, RunStatusVerified, noChange.RunRecord.Status)
	require.Empty(t, noChange.RunRecord.Items[0].BackupRefs)
	require.Nil(t, noChange.BackupBeforeRestore)
}

func TestRestoreCustomFilesAbsentFileBackupDeletesLiveFile(t *testing.T) {
	t.Parallel()

	root, liveRoot, profile, rec := setupLedgerCustomFilesFixture(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	store, err := NewStore(stateRoot, WithClock(fixedTime))
	require.NoError(t, err)
	desiredPath := desiredArtifactPath(root)
	writeFile(t, desiredPath, "desired create\n")

	plan, err := customfiles.PlanApply(customfiles.Request{Profile: profile, Recipe: rec, SettingRef: "custom.files:file", LocationRoots: map[string]string{"config": liveRoot}})
	require.NoError(t, err)
	_, err = store.ExecuteCustomFiles(plan, CustomFilesExecuteOptions{RunID: "run-create", ProfileStack: profile.Layers, StartedAt: fixedTime()})
	require.NoError(t, err)
	livePath := filepath.Join(liveRoot, "config.txt")
	requireFile(t, livePath, "desired create\n")
	backup := requireBackupMetadata(t, stateRoot, "run-create")
	require.False(t, backup.Items[0].Before.Exists)
	require.Empty(t, backup.Items[0].PayloadRelPath)

	run, err := store.RestoreCustomFiles(CustomFilesRestoreOptions{
		SourceRunID:   "run-create",
		RunID:         "run-delete",
		Profile:       profile,
		Recipe:        rec,
		LocationRoots: map[string]string{"config": liveRoot},
		ProfileStack:  profile.Layers,
		Confirmed:     true,
		StartedAt:     fixedTime().Add(2 * time.Second),
	})
	require.NoError(t, err)
	assertMissing(t, livePath)
	require.Equal(t, RunStatusVerified, run.RunRecord.Status)
	require.False(t, run.RunRecord.Items[0].Desired.Exists)
	require.True(t, run.RunRecord.Items[0].VerifiedState.Exists == false)
	restoreBackup := requireBackupMetadata(t, stateRoot, "run-delete")
	requireFile(t, filepath.Join(stateRoot, "backups", "run-delete", restoreBackup.Items[0].PayloadRelPath), "desired create\n")
}

func TestRestoreCustomFilesFileTreeRestoresManagedPayloadAndPreservesExcludedLivePaths(t *testing.T) {
	t.Parallel()

	root, liveRoot, profile, rec := setupLedgerFileTreeFixture(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	store, err := NewStore(stateRoot, WithClock(fixedTime))
	require.NoError(t, err)
	liveTree := filepath.Join(liveRoot, "profiles")
	desiredTree := desiredTreeArtifactPath(root)
	writeFile(t, filepath.Join(liveTree, "config.yaml"), "tree live before\n")
	writeFile(t, filepath.Join(liveTree, "cache", "ignored.yaml"), "ignored before\n")
	writeFile(t, filepath.Join(desiredTree, "config.yaml"), "tree desired after\n")
	plan, err := customfiles.PlanApply(customfiles.Request{Profile: profile, Recipe: rec, SettingRef: "custom.files:file", LocationRoots: map[string]string{"config": liveRoot}})
	require.NoError(t, err)
	_, err = store.ExecuteCustomFiles(plan, CustomFilesExecuteOptions{RunID: "run-tree-apply", ProfileStack: profile.Layers, StartedAt: fixedTime()})
	require.NoError(t, err)
	requireFile(t, filepath.Join(liveTree, "config.yaml"), "tree desired after\n")
	writeFile(t, filepath.Join(liveTree, "cache", "ignored.yaml"), "ignored changed\n")

	run, err := store.RestoreCustomFiles(CustomFilesRestoreOptions{
		SourceRunID:   "run-tree-apply",
		RunID:         "run-tree-restore",
		Profile:       profile,
		Recipe:        rec,
		LocationRoots: map[string]string{"config": liveRoot},
		ProfileStack:  profile.Layers,
		Confirmed:     true,
		StartedAt:     fixedTime().Add(2 * time.Second),
	})
	require.NoError(t, err)
	requireFile(t, filepath.Join(liveTree, "config.yaml"), "tree live before\n")
	requireFile(t, filepath.Join(liveTree, "cache", "ignored.yaml"), "ignored changed\n")
	require.Equal(t, RunStatusVerified, run.RunRecord.Status)
	require.Equal(t, recipe.FileTreeDriverID, run.RunRecord.Items[0].Driver)
	require.Equal(t, []string{"state://backups/run-tree-apply/custom.files_file-config-file"}, run.RunRecord.Items[0].SourceBackupRefs)
	restoreBackup := requireBackupMetadata(t, stateRoot, "run-tree-restore")
	requireFile(t, filepath.Join(stateRoot, "backups", "run-tree-restore", restoreBackup.Items[0].PayloadRelPath, "config.yaml"), "tree desired after\n")
	assertMissing(t, filepath.Join(stateRoot, "backups", "run-tree-restore", restoreBackup.Items[0].PayloadRelPath, "cache", "ignored.yaml"))

	noChange, err := store.RestoreCustomFiles(CustomFilesRestoreOptions{
		SourceRunID:   "run-tree-apply",
		RunID:         "run-tree-restore-unchanged",
		Profile:       profile,
		Recipe:        rec,
		LocationRoots: map[string]string{"config": liveRoot},
		ProfileStack:  profile.Layers,
		Confirmed:     true,
		StartedAt:     fixedTime().Add(3 * time.Second),
	})
	require.NoError(t, err)
	require.Equal(t, RunStatusVerified, noChange.RunRecord.Status)
	require.Empty(t, noChange.RunRecord.Items[0].BackupRefs)
	require.Nil(t, noChange.BackupBeforeRestore)
}

func TestRestoreGenericFileResourceUsesProfileRecipeAndLocationRoots(t *testing.T) {
	t.Parallel()

	_, liveRoot, profile, _, stateRoot, store := setupRestoreFileSource(t, "live before\n", "desired after\n", "run-apply")
	livePath := filepath.Join(liveRoot, "config.txt")
	requireFile(t, livePath, "desired after\n")

	run, err := store.Restore(RestoreOptions{
		SourceRunID: "run-apply",
		RunID:       "run-generic-file-restore",
		Profile:     profile,
		LocationRoots: map[string]map[string]string{
			"custom.files": {"config": liveRoot},
		},
		Confirmed: true,
		StartedAt: fixedTime().Add(2 * time.Second),
	})
	require.NoError(t, err)
	requireFile(t, livePath, "live before\n")
	require.Equal(t, RunStatusVerified, run.RunRecord.Status)
	require.Equal(t, RestoreCommand, run.RunRecord.Command)
	require.Equal(t, recipe.FileDriverID, run.RunRecord.Items[0].Driver)
	require.Equal(t, []string{"state://backups/run-apply/custom.files_file-config-file"}, run.RunRecord.Items[0].SourceBackupRefs)
	restoreBackup := requireBackupMetadata(t, stateRoot, "run-generic-file-restore")
	requireFile(t, filepath.Join(stateRoot, "backups", "run-generic-file-restore", restoreBackup.Items[0].PayloadRelPath), "desired after\n")
}

func TestRestoreGenericFileTreeResourceRestoresManagedPayload(t *testing.T) {
	t.Parallel()

	root, liveRoot, profile, rec := setupLedgerFileTreeFixture(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	store, err := NewStore(stateRoot, WithClock(fixedTime))
	require.NoError(t, err)
	liveTree := filepath.Join(liveRoot, "profiles")
	desiredTree := desiredTreeArtifactPath(root)
	writeFile(t, filepath.Join(liveTree, "config.yaml"), "tree live before\n")
	writeFile(t, filepath.Join(liveTree, "cache", "ignored.yaml"), "ignored before\n")
	writeFile(t, filepath.Join(desiredTree, "config.yaml"), "tree desired after\n")
	plan, err := customfiles.PlanApply(customfiles.Request{Profile: profile, Recipe: rec, SettingRef: "custom.files:file", LocationRoots: map[string]string{"config": liveRoot}})
	require.NoError(t, err)
	_, err = store.ExecuteCustomFiles(plan, CustomFilesExecuteOptions{RunID: "run-tree-apply", ProfileStack: profile.Layers, StartedAt: fixedTime()})
	require.NoError(t, err)
	writeFile(t, filepath.Join(liveTree, "cache", "ignored.yaml"), "ignored changed\n")

	run, err := store.Restore(RestoreOptions{
		SourceRunID: "run-tree-apply",
		RunID:       "run-generic-tree-restore",
		Profile:     profile,
		LocationRoots: map[string]map[string]string{
			"custom.files": {"config": liveRoot},
		},
		Confirmed: true,
		StartedAt: fixedTime().Add(2 * time.Second),
	})
	require.NoError(t, err)
	requireFile(t, filepath.Join(liveTree, "config.yaml"), "tree live before\n")
	requireFile(t, filepath.Join(liveTree, "cache", "ignored.yaml"), "ignored changed\n")
	require.Equal(t, RunStatusVerified, run.RunRecord.Status)
	require.Equal(t, recipe.FileTreeDriverID, run.RunRecord.Items[0].Driver)
	restoreBackup := requireBackupMetadata(t, stateRoot, "run-generic-tree-restore")
	requireFile(t, filepath.Join(stateRoot, "backups", "run-generic-tree-restore", restoreBackup.Items[0].PayloadRelPath, "config.yaml"), "tree desired after\n")
}

func TestRestoreGenericSelectedValueRollsBackWholeBackingFile(t *testing.T) {
	t.Parallel()

	_, liveRoot, profile := setupLedgerSelectedValueFixture(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	store, err := NewStore(stateRoot, WithClock(fixedTime))
	require.NoError(t, err)
	livePath := filepath.Join(liveRoot, "config.yaml")
	before := "user:\n  email: before@example.com\n  name: Before\n"
	after := "user:\n  email: after@example.com\n  name: After\n"
	writeFile(t, livePath, before)
	item, err := store.WriteSelectedValueBackup("run-selected", fixedTime(), SelectedValueBackupRequest{
		TargetRef:  "test.app",
		SettingRef: "test.app:identity.email",
		ResourceID: "config-email",
		Driver:     recipe.YAMLFileDriverID,
		LivePath:   livePath,
		Before:     NormalizedState{Exists: true},
		BeforeFile: []byte(before),
	})
	require.NoError(t, err)
	require.Equal(t, filedriver.NormalizerID, item.Restore.Normalizer)
	writeFile(t, livePath, after)

	run, err := store.Restore(RestoreOptions{
		SourceRunID: "run-selected",
		RunID:       "run-selected-restore",
		Profile:     profile,
		Confirmed:   true,
		StartedAt:   fixedTime().Add(2 * time.Second),
	})
	require.NoError(t, err)
	requireFile(t, livePath, before)
	require.Equal(t, RunStatusVerified, run.RunRecord.Status)
	require.Equal(t, recipe.YAMLFileDriverID, run.RunRecord.Items[0].Driver)
	require.Contains(t, run.Preview.Items[0].Message, "whole backing file")
	require.Equal(t, []string{"state://backups/run-selected/test.app_identity.email-config-email"}, run.RunRecord.Items[0].SourceBackupRefs)
	restoreBackup := requireBackupMetadata(t, stateRoot, "run-selected-restore")
	requireFile(t, filepath.Join(stateRoot, "backups", "run-selected-restore", restoreBackup.Items[0].PayloadRelPath), after)

	noChange, err := store.Restore(RestoreOptions{
		SourceRunID: "run-selected",
		RunID:       "run-selected-restore-unchanged",
		Profile:     profile,
		Confirmed:   true,
		StartedAt:   fixedTime().Add(3 * time.Second),
	})
	require.NoError(t, err)
	require.Empty(t, noChange.RunRecord.Items[0].BackupRefs)
	require.Nil(t, noChange.BackupBeforeRestore)
}

func TestRestoreGenericRequiresConfirmationBeforeWrites(t *testing.T) {
	t.Parallel()

	_, liveRoot, profile, _, stateRoot, store := setupRestoreFileSource(t, "live before\n", "desired after\n", "run-apply")
	run, err := store.Restore(RestoreOptions{
		SourceRunID: "run-apply",
		RunID:       "run-generic-confirm",
		Profile:     profile,
		LocationRoots: map[string]map[string]string{
			"custom.files": {"config": liveRoot},
		},
		StartedAt: fixedTime().Add(2 * time.Second),
	})
	require.Error(t, err)
	require.NotNil(t, run)
	require.Equal(t, preview.ExitInputRequired, preview.ExitCode(run.Preview))
	require.Equal(t, preview.ResultBlocked, run.Preview.Items[0].Result)
	requireFile(t, filepath.Join(liveRoot, "config.txt"), "desired after\n")
	assertMissing(t, filepath.Join(stateRoot, "ledger", "runs", "run-generic-confirm.json"))
}

func TestRestoreGenericValidationDryRunAndUnsupportedDriverBranches(t *testing.T) {
	t.Parallel()

	_, liveRoot, profile, _, stateRoot, store := setupRestoreFileSource(t, "live before\n", "desired after\n", "run-apply")
	var nilStore *Store
	_, err := nilStore.Restore(RestoreOptions{})
	require.Error(t, err)
	_, err = store.Restore(RestoreOptions{SourceRunID: "run-apply", RunID: "run-restore"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "resolved profile")
	_, err = store.Restore(RestoreOptions{SourceRunID: "../bad", RunID: "run-restore", Profile: profile})
	require.Error(t, err)
	_, err = store.Restore(RestoreOptions{SourceRunID: "run-apply", RunID: "../bad", Profile: profile})
	require.Error(t, err)
	_, err = store.Restore(RestoreOptions{SourceRunID: "run-apply", RunID: "run-apply", Profile: profile})
	require.Error(t, err)

	dryRun, err := store.Restore(RestoreOptions{
		SourceRunID: "run-apply",
		RunID:       "run-generic-dry-run",
		Profile:     profile,
		LocationRoots: map[string]map[string]string{
			"custom.files": {"config": liveRoot},
		},
		DryRun:    true,
		StartedAt: fixedTime().Add(2 * time.Second),
	})
	require.NoError(t, err)
	require.Nil(t, dryRun.RunRecord)
	require.Equal(t, preview.ResultWouldChange, dryRun.Preview.Items[0].Result)
	assertMissing(t, filepath.Join(stateRoot, "ledger", "runs", "run-generic-dry-run.json"))

	metadata := requireBackupMetadata(t, stateRoot, "run-apply")
	badDriver := metadata
	badDriver.RunID = "run-bad-driver-generic"
	badDriver.Items[0].Driver = "mystery-driver"
	badDriver.Items[0].Restore.Compatible = true
	badDriver.Items[0].Restore.Driver = "mystery-driver"
	require.NoError(t, writeBackupMetadata(filepath.Join(stateRoot, "backups", "run-bad-driver-generic", "backup.yaml"), badDriver))
	blocked, err := store.Restore(RestoreOptions{
		SourceRunID: "run-bad-driver-generic",
		RunID:       "run-bad-driver-generic-restore",
		Profile:     profile,
		Confirmed:   true,
	})
	require.Error(t, err)
	require.Equal(t, preview.SummaryBlocked, blocked.Preview.Summary.Status)
	require.Contains(t, blocked.Preview.Items[0].Diagnostics[0].Message, "unsupported driver")

	targetMismatch := requireBackupMetadata(t, stateRoot, "run-apply")
	targetMismatch.RunID = "run-target-mismatch"
	targetMismatch.Items[0].TargetRef = "other.target"
	require.NoError(t, writeBackupMetadata(filepath.Join(stateRoot, "backups", "run-target-mismatch", "backup.yaml"), targetMismatch))
	blocked, err = store.Restore(RestoreOptions{
		SourceRunID: "run-target-mismatch",
		RunID:       "run-target-mismatch-restore",
		Profile:     profile,
		Confirmed:   true,
	})
	require.Error(t, err)
	require.Equal(t, preview.SummaryBlocked, blocked.Preview.Summary.Status)
	require.Contains(t, blocked.Preview.Items[0].Diagnostics[0].Message, "target mismatch")
}

func TestRestoreGenericBlocksWholeRunWhenAnyItemUnsupported(t *testing.T) {
	t.Parallel()

	_, liveRoot, profile := setupLedgerSelectedValueFixture(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	store, err := NewStore(stateRoot, WithClock(fixedTime))
	require.NoError(t, err)
	livePath := filepath.Join(liveRoot, "config.yaml")
	before := "user:\n  email: before@example.com\n"
	writeFile(t, livePath, before)
	_, err = store.WriteSelectedValueBackup("run-mixed", fixedTime(), SelectedValueBackupRequest{
		TargetRef:  "test.app",
		SettingRef: "test.app:identity.email",
		ResourceID: "config-email",
		Driver:     recipe.YAMLFileDriverID,
		LivePath:   livePath,
		Before:     NormalizedState{Exists: true},
		BeforeFile: []byte(before),
	})
	require.NoError(t, err)
	metadata := requireBackupMetadata(t, stateRoot, "run-mixed")
	metadata.Items = append(metadata.Items, BackupItem{
		Ref:           "state://backups/run-mixed/native-settings",
		TargetRef:     "native.app",
		SettingRef:    "native.app:settings",
		ResourceID:    "settings",
		Driver:        recipe.NativeExportDriverID,
		DriverVersion: "native-export.driver.v1",
		LivePath:      "export-settings",
		CreatedAt:     metadata.CreatedAt,
		Before:        NormalizedState{Exists: true, Hash: "native", Normalizer: "native-export.metadata.v1", DriverVersion: "native-export.driver.v1"},
		Restore: RestoreCompatibility{
			Compatible:    false,
			Driver:        recipe.NativeExportDriverID,
			DriverVersion: "native-export.driver.v1",
			Normalizer:    "native-export.metadata.v1",
			Message:       "automatic native restore is not implemented",
		},
	})
	require.NoError(t, writeBackupMetadata(filepath.Join(stateRoot, "backups", "run-mixed", "backup.yaml"), metadata))
	writeFile(t, livePath, "user:\n  email: broken@example.com\n")

	run, err := store.Restore(RestoreOptions{
		SourceRunID: "run-mixed",
		RunID:       "run-mixed-restore",
		Profile:     profile,
		Confirmed:   true,
		StartedAt:   fixedTime().Add(2 * time.Second),
	})
	require.Error(t, err)
	require.NotNil(t, run)
	require.Equal(t, preview.SummaryBlocked, run.Preview.Summary.Status)
	require.Len(t, run.Preview.Items, 2)
	for _, item := range run.Preview.Items {
		require.Equal(t, preview.ResultBlocked, item.Result)
	}
	requireFile(t, livePath, "user:\n  email: broken@example.com\n")
	assertMissing(t, filepath.Join(stateRoot, "ledger", "runs", "run-mixed-restore.json"))
}

func TestGenericRestoreRollsBackPriorWritesWhenLaterExecutionFails(t *testing.T) {
	t.Parallel()

	root, liveRoot, profile, rec := setupLedgerMultiFileFixture(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	store, err := NewStore(stateRoot, WithClock(fixedTime))
	require.NoError(t, err)

	firstLive := filepath.Join(liveRoot, "first.txt")
	secondLive := filepath.Join(liveRoot, "second.txt")
	writeFile(t, firstLive, "current first\n")
	writeFile(t, secondLive, "current second\n")
	writeFile(t, filepath.Join(root, "desired", "user", "leon", "targets", "custom.files", "artifacts", "first.txt"), "desired first\n")
	writeFile(t, filepath.Join(root, "desired", "user", "leon", "targets", "custom.files", "artifacts", "second.txt"), "desired second\n")

	firstPlan, err := customfiles.PlanFileRead(customfiles.Request{Profile: profile, Recipe: rec, SettingRef: "custom.files:first", LocationRoots: map[string]string{"config": liveRoot}})
	require.NoError(t, err)
	secondPlan, err := customfiles.PlanFileRead(customfiles.Request{Profile: profile, Recipe: rec, SettingRef: "custom.files:second", LocationRoots: map[string]string{"config": liveRoot}})
	require.NoError(t, err)
	_, err = store.WriteCustomFilesBackup("run-source-multi", fixedTime(), firstPlan, customfiles.BackupRequest{
		Operation:  customfiles.OperationApply,
		SettingRef: "custom.files:first",
		ResourceID: "first-file",
		Path:       firstLive,
		Before:     filedriver.Driver{}.Normalize([]byte("restore first\n")),
	})
	require.NoError(t, err)
	_, err = store.WriteCustomFilesBackup("run-source-multi", fixedTime(), secondPlan, customfiles.BackupRequest{
		Operation:  customfiles.OperationApply,
		SettingRef: "custom.files:second",
		ResourceID: "second-file",
		Path:       secondLive,
		Before:     filedriver.Driver{}.Normalize([]byte("restore second\n")),
	})
	require.NoError(t, err)

	// Force only the second item's backup-before-restore write to fail after the
	// first item has written live state and recorded a rollback artifact.
	writeFile(t, filepath.Join(stateRoot, "backups", "run-restore-multi", "payloads", "custom.files_second-second-file"), "not a directory\n")

	run, err := store.Restore(RestoreOptions{
		SourceRunID: "run-source-multi",
		RunID:       "run-restore-multi",
		Profile:     profile,
		LocationRoots: map[string]map[string]string{
			"custom.files": {"config": liveRoot},
		},
		Confirmed: true,
		StartedAt: fixedTime(),
	})
	require.Error(t, err)
	require.NotNil(t, run)
	require.Contains(t, err.Error(), "backup before restore custom.files:second")
	require.Contains(t, err.Error(), "not a directory")
	requireFile(t, firstLive, "current first\n")
	requireFile(t, secondLive, "current second\n")

	record := requireRunRecord(t, stateRoot, "run-restore-multi")
	require.Equal(t, RunStatusFailed, record.Status)
	require.Len(t, record.Items, 2)
	for _, item := range record.Items {
		require.Equal(t, ItemResultFailed, item.Result)
		require.False(t, item.Verification.Verified)
	}
	firstRecord := requireRunItem(t, record, "custom.files:first")
	requireDiagnosticCode(t, firstRecord.Diagnostics, "restore.rollback-succeeded")
	secondRecord := requireRunItem(t, record, "custom.files:second")
	requireDiagnosticCode(t, secondRecord.Diagnostics, "restore.execution-failed")
	require.Contains(t, secondRecord.Diagnostics[0].Message+secondRecord.Diagnostics[1].Message, "Rollback succeeded")
	require.Empty(t, run.LedgerEntries)

	firstPreview := requirePreviewItem(t, run.Preview, "custom.files:first")
	require.Equal(t, preview.ResultFailed, firstPreview.Result)
	requirePreviewDiagnosticCode(t, firstPreview.Diagnostics, "restore.rollback-succeeded")
	secondPreview := requirePreviewItem(t, run.Preview, "custom.files:second")
	require.Equal(t, preview.ResultFailed, secondPreview.Result)
	requirePreviewDiagnosticCode(t, secondPreview.Diagnostics, "restore.execution-failed")
	require.Contains(t, secondPreview.Message, "Rollback succeeded")
}

func TestGenericRestoreRollsBackWhenCommitRunFailsAfterWrites(t *testing.T) {
	t.Parallel()

	_, liveRoot, profile, _, stateRoot, store := setupRestoreFileSource(t, "live before\n", "desired after\n", "run-commit-source")
	livePath := filepath.Join(liveRoot, "config.txt")
	writeFile(t, livePath, "broken before restore\n")
	require.NoError(t, os.RemoveAll(filepath.Join(stateRoot, "ledger", "runs")))
	writeFile(t, filepath.Join(stateRoot, "ledger", "runs"), "not a directory\n")

	run, err := store.Restore(RestoreOptions{
		SourceRunID: "run-commit-source",
		RunID:       "run-commit-restore",
		Profile:     profile,
		LocationRoots: map[string]map[string]string{
			"custom.files": {"config": liveRoot},
		},
		Confirmed: true,
		StartedAt: fixedTime(),
	})
	require.Error(t, err)
	require.NotNil(t, run)
	require.Contains(t, err.Error(), "commit restore run run-commit-restore")
	requireFile(t, livePath, "broken before restore\n")
	item := requirePreviewItem(t, run.Preview, "custom.files:file")
	require.Equal(t, preview.ResultFailed, item.Result)
	requirePreviewDiagnosticCode(t, item.Diagnostics, "restore.rollback-succeeded")
}

func TestRestoreRollbackStatusAndPreviewAnnotationBranches(t *testing.T) {
	t.Parallel()

	require.Equal(t, "not-attempted", restoreRollbackResult{}.Status())
	require.Equal(t, "succeeded", restoreRollbackResult{Attempted: true, Items: []restoreRollbackItem{{recordIndex: 0, backupRef: "state://backups/run/a"}}}.Status())
	require.Equal(t, "failed", restoreRollbackResult{Attempted: true, ReadError: errors.New("read backup failed")}.Status())
	partialRollback := restoreRollbackResult{Attempted: true, Items: []restoreRollbackItem{
		{recordIndex: 0, backupRef: "state://backups/run/a"},
		{recordIndex: 1, backupRef: "state://backups/run/b", err: errors.New("rollback failed")},
	}}
	require.Equal(t, "partially-failed", partialRollback.Status())
	require.Error(t, partialRollback.Err())
	require.Equal(t, "", restoreErrorMessage(nil))

	envelope := preview.BuildEnvelope(preview.EnvelopeOptions{
		Command: preview.CommandRestore,
		RunID:   "run-rollback-preview",
		Items: []preview.Item{
			{SettingRef: "custom.files:first", Operation: RestoreCommand, State: status.StateReadyToApply, Result: preview.ResultWouldChange, LivePath: "/live/first"},
			{SettingRef: "custom.files:second", Operation: RestoreCommand, State: status.StateReadyToApply, Result: preview.ResultWouldChange, LivePath: "/live/second"},
			{SettingRef: "custom.files:third", Operation: RestoreCommand, State: status.StateReadyToApply, Result: preview.ResultWouldChange, LivePath: "/live/third"},
		},
	})
	annotated := annotateRestorePreviewRollback(envelope, 1, partialRollback, errors.New("verify failed"))
	require.Equal(t, preview.SummaryError, annotated.Summary.Status)
	requirePreviewDiagnosticCode(t, annotated.Items[0].Diagnostics, "restore.rollback-succeeded")
	requirePreviewDiagnosticCode(t, annotated.Items[1].Diagnostics, "restore.execution-failed")
	requirePreviewDiagnosticCode(t, annotated.Items[1].Diagnostics, "restore.rollback-failed")
	requirePreviewDiagnosticCode(t, annotated.Items[2].Diagnostics, "restore.blocked-by-execution-failure")

	notAttempted := annotateRestorePreviewRollback(envelope, 1, restoreRollbackResult{}, errors.New("read failed"))
	requirePreviewDiagnosticCode(t, notAttempted.Items[0].Diagnostics, "restore.rollback-not-attempted")
	requirePreviewDiagnosticCode(t, notAttempted.Items[1].Diagnostics, "restore.rollback-not-attempted")
	require.Contains(t, notAttempted.Items[1].Message, "Rollback was not attempted")

	record := RunRecord{Items: []ItemRecord{
		{SettingRef: "custom.files:first", LivePath: "/live/first", Verification: Verification{Verified: true, Result: "verified"}, Result: ItemResultVerified},
		{SettingRef: "custom.files:second", LivePath: "/live/second", Verification: Verification{Verified: false, Result: "failed"}, Result: ItemResultFailed},
	}}
	record = annotateRestoreRunRollback(record, 1, partialRollback, errors.New("verify failed"))
	requireDiagnosticCode(t, record.Items[0].Diagnostics, "restore.rollback-succeeded")
	requireDiagnosticCode(t, record.Items[1].Diagnostics, "restore.execution-failed")
	requireDiagnosticCode(t, record.Items[1].Diagnostics, "restore.rollback-failed")
}

func TestRestoreRollbackItemRestoresFileTreeFromBackupArtifact(t *testing.T) {
	t.Parallel()

	root, liveRoot, profile, rec := setupLedgerFileTreeFixture(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	store, err := NewStore(stateRoot, WithClock(fixedTime))
	require.NoError(t, err)
	liveTree := filepath.Join(liveRoot, "profiles")
	writeFile(t, filepath.Join(liveTree, "config.yaml"), "tree current\n")
	writeFile(t, filepath.Join(root, "desired", "user", "leon", "targets", "custom.files", "artifacts", "profiles", "config.yaml"), "tree desired\n")

	plan, err := customfiles.PlanFileRead(customfiles.Request{Profile: profile, Recipe: rec, SettingRef: "custom.files:file", LocationRoots: map[string]string{"config": liveRoot}})
	require.NoError(t, err)
	before, err := filetreedriver.Driver{}.ReadCurrent(plan.TreeLiveTarget)
	require.NoError(t, err)
	backupItem, err := store.WriteCustomFilesBackup("run-tree-rollback", fixedTime(), plan, customfiles.BackupRequest{
		Operation:  customfiles.OperationApply,
		SettingRef: "custom.files:file",
		ResourceID: "config-file",
		Path:       liveTree,
		TreeBefore: before,
	})
	require.NoError(t, err)

	writeFile(t, filepath.Join(liveTree, "config.yaml"), "tree mutated\n")
	metadata := requireBackupMetadata(t, stateRoot, "run-tree-rollback")
	err = store.rollbackRestoreItem("run-tree-rollback", metadata, restoreExecutedItem{
		item:      restoreItemPlan{plan: plan, source: backupItem},
		backupRef: backupItem.Ref,
	})
	require.NoError(t, err)
	requireFile(t, filepath.Join(liveTree, "config.yaml"), "tree current\n")

	err = store.rollbackRestoreItem("run-tree-rollback", metadata, restoreExecutedItem{
		item:      restoreItemPlan{plan: plan, source: backupItem},
		backupRef: "state://backups/run-tree-rollback/missing",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "was not found")

	missing := store.rollbackRestoreExecution("missing-rollback-run", []restoreExecutedItem{{
		item:      restoreItemPlan{plan: plan, source: backupItem},
		backupRef: backupItem.Ref,
		rollback:  true,
	}})
	require.Equal(t, "failed", missing.Status())
	require.Error(t, missing.Err())
}

func TestSelectedValueRestoreExecuteFailureBranches(t *testing.T) {
	t.Parallel()

	_, liveRoot, profile := setupLedgerSelectedValueFixture(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	store, err := NewStore(stateRoot, WithClock(fixedTime))
	require.NoError(t, err)
	livePath := filepath.Join(liveRoot, "config.yaml")
	writeFile(t, livePath, "user:\n  email: current@example.com\n")
	_, err = store.WriteSelectedValueBackup("run-selected-execute-source", fixedTime(), SelectedValueBackupRequest{
		TargetRef:  "test.app",
		SettingRef: "test.app:identity.email",
		ResourceID: "config-email",
		Driver:     recipe.YAMLFileDriverID,
		LivePath:   livePath,
		Before:     NormalizedState{Exists: true},
		BeforeFile: []byte("user:\n  email: restore@example.com\n"),
	})
	require.NoError(t, err)
	_, plans, _, err := store.planRestore(RestoreOptions{
		SourceRunID: "run-selected-execute-source",
		RunID:       "run-selected-execute-restore",
		Profile:     profile,
	}, profile.Layers)
	require.NoError(t, err)
	require.Len(t, plans, 1)

	readFail := plans[0]
	badRoot := filepath.Join(t.TempDir(), "not-a-directory")
	writeFile(t, badRoot, "not a directory\n")
	readFail.liveTarget.Root = badRoot
	record, executed, err := store.executeSelectedValueRestoreItemWithRollback("run-selected-read-fail", fixedTime(), readFail)
	require.Error(t, err)
	require.Contains(t, err.Error(), "read current before restore")
	require.Equal(t, ItemResultFailed, record.Result)
	require.False(t, executed.rollback)

	backupFail := plans[0]
	writeFile(t, filepath.Join(stateRoot, "backups", "run-selected-backup-fail", "payloads", "test.app_identity.email-config-email"), "not a directory\n")
	record, executed, err = store.executeSelectedValueRestoreItemWithRollback("run-selected-backup-fail", fixedTime(), backupFail)
	require.Error(t, err)
	require.Contains(t, err.Error(), "backup before restore")
	require.Equal(t, ItemResultFailed, record.Result)
	require.False(t, executed.rollback)
}

func TestRestoreGenericSelectedValueMismatchBlockers(t *testing.T) {
	t.Parallel()

	_, liveRoot, profile := setupLedgerSelectedValueFixture(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	store, err := NewStore(stateRoot, WithClock(fixedTime))
	require.NoError(t, err)
	livePath := filepath.Join(liveRoot, "config.yaml")
	before := "user:\n  email: before@example.com\n"
	writeFile(t, livePath, before)
	_, err = store.WriteSelectedValueBackup("run-selected-mismatch", fixedTime(), SelectedValueBackupRequest{
		TargetRef:  "test.app",
		SettingRef: "test.app:identity.email",
		ResourceID: "config-email",
		Driver:     recipe.YAMLFileDriverID,
		LivePath:   livePath,
		Before:     NormalizedState{Exists: true},
		BeforeFile: []byte(before),
	})
	require.NoError(t, err)

	resourceMismatch := requireBackupMetadata(t, stateRoot, "run-selected-mismatch")
	resourceMismatch.RunID = "run-resource-mismatch"
	resourceMismatch.Items[0].ResourceID = "other-resource"
	require.NoError(t, writeBackupMetadata(filepath.Join(stateRoot, "backups", "run-resource-mismatch", "backup.yaml"), resourceMismatch))
	blocked, err := store.Restore(RestoreOptions{SourceRunID: "run-resource-mismatch", RunID: "run-resource-mismatch-restore", Profile: profile, Confirmed: true})
	require.Error(t, err)
	require.Equal(t, preview.SummaryBlocked, blocked.Preview.Summary.Status)
	require.Contains(t, blocked.Preview.Items[0].Diagnostics[0].Message, "resource mismatch")

	pathMismatch := requireBackupMetadata(t, stateRoot, "run-selected-mismatch")
	pathMismatch.RunID = "run-path-mismatch"
	pathMismatch.Items[0].LivePath = filepath.Join(liveRoot, "other.yaml")
	require.NoError(t, writeBackupMetadata(filepath.Join(stateRoot, "backups", "run-path-mismatch", "backup.yaml"), pathMismatch))
	blocked, err = store.Restore(RestoreOptions{SourceRunID: "run-path-mismatch", RunID: "run-path-mismatch-restore", Profile: profile, Confirmed: true})
	require.Error(t, err)
	require.Equal(t, preview.SummaryBlocked, blocked.Preview.Summary.Status)
	require.Contains(t, blocked.Preview.Items[0].Diagnostics[0].Message, "live path mismatch")
}

func TestRestoreCustomFilesFailsClearlyForMissingIncompatibleUnsupportedAndBadPayloads(t *testing.T) {
	t.Parallel()

	_, liveRoot, profile, rec, stateRoot, store := setupRestoreFileSource(t, "live before\n", "desired after\n", "run-apply")
	livePath := filepath.Join(liveRoot, "config.txt")

	_, err := store.RestoreCustomFiles(CustomFilesRestoreOptions{SourceRunID: "missing", RunID: "run-missing", Profile: profile, Recipe: rec, LocationRoots: map[string]string{"config": liveRoot}, Confirmed: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "locate restore backup missing")

	writeBackupYAML(t, filepath.Join(stateRoot, "backups", "bad-incompatible", "backup.yaml"), `schema: dotfiles-manager.v2.backup-metadata
schemaVersion: 1
runId: bad-incompatible
createdAt: "2026-06-05T20:30:00Z"
items:
  - ref: state://backups/bad-incompatible/custom.files_file-config-file
    targetRef: custom.files
    settingRef: custom.files:file
    resourceId: config-file
    driver: file
    driverVersion: file.driver.v1
    livePath: /tmp/config.txt
    createdAt: "2026-06-05T20:30:00Z"
    before:
      exists: true
      hash: abc
    restore:
      compatible: false
      driver: file
      driverVersion: file.driver.v1
      normalizer: file.bytes.v1
      message: incompatible fixture
`)
	_, err = store.RestoreCustomFiles(CustomFilesRestoreOptions{SourceRunID: "bad-incompatible", RunID: "run-incompatible", Profile: profile, Recipe: rec, LocationRoots: map[string]string{"config": liveRoot}, Confirmed: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "incompatible")

	writeBackupYAML(t, filepath.Join(stateRoot, "backups", "bad-driver", "backup.yaml"), `schema: dotfiles-manager.v2.backup-metadata
schemaVersion: 1
runId: bad-driver
createdAt: "2026-06-05T20:30:00Z"
items:
  - ref: state://backups/bad-driver/custom.files_file-config-file
    targetRef: custom.files
    settingRef: custom.files:file
    resourceId: config-file
    driver: native-export
    driverVersion: native.v1
    livePath: /tmp/config.txt
    createdAt: "2026-06-05T20:30:00Z"
    before:
      exists: true
      hash: abc
    restore:
      compatible: true
      driver: native-export
      driverVersion: native.v1
      normalizer: opaque.v1
      message: ok
`)
	_, err = store.RestoreCustomFiles(CustomFilesRestoreOptions{SourceRunID: "bad-driver", RunID: "run-driver", Profile: profile, Recipe: rec, LocationRoots: map[string]string{"config": liveRoot}, Confirmed: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported driver")

	metadata := requireBackupMetadata(t, stateRoot, "run-apply")
	require.NoError(t, os.Remove(filepath.Join(stateRoot, "backups", "run-apply", metadata.Items[0].PayloadRelPath)))
	_, err = store.RestoreCustomFiles(CustomFilesRestoreOptions{SourceRunID: "run-apply", RunID: "run-missing-payload", Profile: profile, Recipe: rec, LocationRoots: map[string]string{"config": liveRoot}, Confirmed: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "read restore payload")
	requireFile(t, livePath, "desired after\n")
	assertMissing(t, filepath.Join(stateRoot, "ledger", "runs", "run-missing-payload.json"))
}

func TestRestoreValidationAndHelperBranches(t *testing.T) {
	t.Parallel()

	root, liveRoot, profile, rec, stateRoot, store := setupRestoreFileSource(t, "live before\n", "desired after\n", "run-apply")
	var nilStore *Store
	_, err := nilStore.RestoreCustomFiles(CustomFilesRestoreOptions{})
	require.Error(t, err)
	_, err = store.RestoreCustomFiles(CustomFilesRestoreOptions{SourceRunID: "run-apply", RunID: "run-restore", Recipe: rec})
	require.Error(t, err)
	require.Contains(t, err.Error(), "resolved profile")
	_, err = store.RestoreCustomFiles(CustomFilesRestoreOptions{SourceRunID: "run-apply", RunID: "run-restore", Profile: profile})
	require.Error(t, err)
	require.Contains(t, err.Error(), "custom.files recipe")

	unsafeStore, err := NewStore(filepath.Join(root, "state"))
	require.NoError(t, err)
	_, err = unsafeStore.RestoreCustomFiles(CustomFilesRestoreOptions{SourceRunID: "run-apply", RunID: "run-restore", Profile: profile, Recipe: rec})
	require.Error(t, err)
	require.Contains(t, err.Error(), "must not resolve inside repository")
	_, err = store.RestoreCustomFiles(CustomFilesRestoreOptions{SourceRunID: "../bad", RunID: "run-restore", Profile: profile, Recipe: rec})
	require.Error(t, err)
	_, err = store.RestoreCustomFiles(CustomFilesRestoreOptions{SourceRunID: "run-apply", RunID: "../bad", Profile: profile, Recipe: rec})
	require.Error(t, err)
	_, err = store.RestoreCustomFiles(CustomFilesRestoreOptions{SourceRunID: "run-apply", RunID: "run-apply", Profile: profile, Recipe: rec})
	require.Error(t, err)
	require.Contains(t, err.Error(), "must differ")

	_, itemPlans, _, err := store.planCustomFilesRestore(CustomFilesRestoreOptions{
		SourceRunID:   "run-apply",
		RunID:         "run-restore-plan",
		Profile:       profile,
		Recipe:        rec,
		LocationRoots: map[string]string{"config": liveRoot},
		DryRun:        true,
	}, profile.Layers)
	require.NoError(t, err)
	require.Len(t, itemPlans, 1)
	failed := failedRestoreItemRecord(itemPlans[0], nil, errors.New("restore helper failed"), "run-fail")
	require.Equal(t, ItemResultFailed, failed.Result)
	require.Equal(t, "restore-failed", failed.Diagnostics[0].Code)
	attachRestoreBackup(nil, "state://backups/run/unused")
	attachRestoreBackup(&failed, "")
	require.Empty(t, failed.BackupRefs)
	unchanged := restorePreviewItem(itemPlans[0].plan, itemPlans[0].source, itemPlans[0].plan.Setting, itemPlans[0].livePath, itemPlans[0].desiredPath, itemPlans[0].plan.Setting.DesiredURI, itemPlans[0].plan.DesiredRelPath, preview.Change{Kind: "unchanged"}, true, false)
	require.Equal(t, preview.BackupNotApplicable, unchanged.Backup.Policy)

	sourceItem := requireBackupMetadata(t, stateRoot, "run-apply").Items[0]
	absentWithPayload := sourceItem
	absentWithPayload.Before.Exists = false
	absentWithPayload.PayloadRelPath = "payloads/custom.files_file-config-file/before"
	_, err = store.loadFileRestoreState("run-apply", absentWithPayload)
	require.Error(t, err)
	hashMismatch := sourceItem
	hashMismatch.Before.Hash = "wrong"
	_, err = store.loadFileRestoreState("run-apply", hashMismatch)
	require.Error(t, err)
	_, err = store.backupPayloadPath("bad/run", "payloads/x")
	require.Error(t, err)
	_, err = store.backupPayloadPath("run", "../bad")
	require.Error(t, err)

	mismatch := sourceItem
	mismatch.ResourceID = "other-resource"
	_, err = store.planRestoreItem(CustomFilesRestoreOptions{SourceRunID: "run-apply", RunID: "run-mismatch", Profile: profile, Recipe: rec, LocationRoots: map[string]string{"config": liveRoot}}, mismatch)
	require.Error(t, err)
	require.Contains(t, err.Error(), "resource mismatch")
	driverMismatch := sourceItem
	driverMismatch.Driver = recipe.FileTreeDriverID
	_, err = store.planRestoreItem(CustomFilesRestoreOptions{SourceRunID: "run-apply", RunID: "run-driver-mismatch", Profile: profile, Recipe: rec, LocationRoots: map[string]string{"config": liveRoot}}, driverMismatch)
	require.Error(t, err)
	require.Contains(t, err.Error(), "driver mismatch")

	treeAbsentPayload := BackupItem{Ref: "state://backups/run-tree/item", Driver: recipe.FileTreeDriverID, Before: NormalizedState{Exists: false}, PayloadRelPath: "payloads/item/tree"}
	_, err = store.loadTreeRestoreState("run-tree", treeAbsentPayload)
	require.Error(t, err)
	notDir := BackupItem{Ref: "state://backups/run-tree-not-dir/item", Driver: recipe.FileTreeDriverID, Before: NormalizedState{Exists: true}, PayloadRelPath: "payloads/item/tree"}
	writeFile(t, filepath.Join(stateRoot, "backups", "run-tree-not-dir", notDir.PayloadRelPath), "not a dir\n")
	_, err = store.loadTreeRestoreState("run-tree-not-dir", notDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not a directory")
}

func TestRestoreTreePayloadHashMismatchFailsClearly(t *testing.T) {
	t.Parallel()

	root, liveRoot, profile, rec := setupLedgerFileTreeFixture(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	store, err := NewStore(stateRoot, WithClock(fixedTime))
	require.NoError(t, err)
	liveTree := filepath.Join(liveRoot, "profiles")
	desiredTree := desiredTreeArtifactPath(root)
	writeFile(t, filepath.Join(liveTree, "config.yaml"), "tree live before\n")
	writeFile(t, filepath.Join(desiredTree, "config.yaml"), "tree desired after\n")
	plan, err := customfiles.PlanApply(customfiles.Request{Profile: profile, Recipe: rec, SettingRef: "custom.files:file", LocationRoots: map[string]string{"config": liveRoot}})
	require.NoError(t, err)
	_, err = store.ExecuteCustomFiles(plan, CustomFilesExecuteOptions{RunID: "run-tree-apply", ProfileStack: profile.Layers, StartedAt: fixedTime()})
	require.NoError(t, err)

	item := requireBackupMetadata(t, stateRoot, "run-tree-apply").Items[0]
	item.Before.Hash = "wrong"
	_, err = store.loadTreeRestoreState("run-tree-apply", item)
	require.Error(t, err)
	require.Contains(t, err.Error(), "hash mismatch")
}

func TestRestoreExecuteFailureBranchesRecordFailedItems(t *testing.T) {
	t.Parallel()

	_, liveRoot, profile, rec, stateRoot, store := setupRestoreFileSource(t, "live before\n", "desired after\n", "run-apply")
	_, fileItems, _, err := store.planCustomFilesRestore(CustomFilesRestoreOptions{
		SourceRunID:   "run-apply",
		RunID:         "run-restore-failures",
		Profile:       profile,
		Recipe:        rec,
		LocationRoots: map[string]string{"config": liveRoot},
	}, profile.Layers)
	require.NoError(t, err)

	readFail := fileItems[0]
	readFailPlan := *readFail.plan
	readFail.plan = &readFailPlan
	readFail.plan.LiveTarget.Root = filepath.Join(t.TempDir(), "missing-root")
	record, err := store.executeFileRestoreItem("run-file-read-fail", fixedTime(), readFail)
	require.Error(t, err)
	require.Contains(t, err.Error(), "read current before restore")
	require.Equal(t, ItemResultFailed, record.Result)
	require.Empty(t, record.BackupRefs)

	backupFail := fileItems[0]
	backupFailPlan := *backupFail.plan
	backupFail.plan = &backupFailPlan
	require.NoError(t, os.MkdirAll(filepath.Join(stateRoot, "backups"), 0o755))
	writeFile(t, filepath.Join(stateRoot, "backups", "run-file-backup-fail"), "not a dir")
	record, err = store.executeFileRestoreItem("run-file-backup-fail", fixedTime(), backupFail)
	require.Error(t, err)
	require.Contains(t, err.Error(), "backup before restore")
	require.Equal(t, ItemResultFailed, record.Result)
	require.Empty(t, record.BackupRefs)

	applyFail := fileItems[0]
	applyFailPlan := *applyFail.plan
	applyFail.plan = &applyFailPlan
	require.NoError(t, os.Chmod(liveRoot, 0o500))
	record, err = store.executeFileRestoreItem("run-file-apply-fail", fixedTime(), applyFail)
	require.NoError(t, os.Chmod(liveRoot, 0o755))
	require.Error(t, err)
	require.Contains(t, err.Error(), "apply restore")
	require.Equal(t, ItemResultFailed, record.Result)
	require.Equal(t, []string{"state://backups/run-file-apply-fail/custom.files_file-config-file"}, record.BackupRefs)

	treeRoot, treeLiveRoot, treeProfile, treeRecipe := setupLedgerFileTreeFixture(t)
	treeStateRoot := filepath.Join(t.TempDir(), "state")
	treeStore, err := NewStore(treeStateRoot, WithClock(fixedTime))
	require.NoError(t, err)
	liveTree := filepath.Join(treeLiveRoot, "profiles")
	desiredTree := desiredTreeArtifactPath(treeRoot)
	writeFile(t, filepath.Join(liveTree, "config.yaml"), "tree live before\n")
	writeFile(t, filepath.Join(desiredTree, "config.yaml"), "tree desired after\n")
	treePlan, err := customfiles.PlanApply(customfiles.Request{Profile: treeProfile, Recipe: treeRecipe, SettingRef: "custom.files:file", LocationRoots: map[string]string{"config": treeLiveRoot}})
	require.NoError(t, err)
	_, err = treeStore.ExecuteCustomFiles(treePlan, CustomFilesExecuteOptions{RunID: "run-tree-apply", ProfileStack: treeProfile.Layers, StartedAt: fixedTime()})
	require.NoError(t, err)
	_, treeItems, _, err := treeStore.planCustomFilesRestore(CustomFilesRestoreOptions{
		SourceRunID:   "run-tree-apply",
		RunID:         "run-tree-restore-failures",
		Profile:       treeProfile,
		Recipe:        treeRecipe,
		LocationRoots: map[string]string{"config": treeLiveRoot},
	}, treeProfile.Layers)
	require.NoError(t, err)

	treeReadFail := treeItems[0]
	treeReadFailPlan := *treeReadFail.plan
	treeReadFail.plan = &treeReadFailPlan
	treeReadFail.plan.TreeLiveTarget.Root = filepath.Join(t.TempDir(), "missing-root")
	record, err = treeStore.executeTreeRestoreItem("run-tree-read-fail", fixedTime(), treeReadFail)
	require.Error(t, err)
	require.Contains(t, err.Error(), "read current before restore")
	require.Equal(t, ItemResultFailed, record.Result)

	treeBackupFail := treeItems[0]
	treeBackupFailPlan := *treeBackupFail.plan
	treeBackupFail.plan = &treeBackupFailPlan
	require.NoError(t, os.MkdirAll(filepath.Join(treeStateRoot, "backups"), 0o755))
	writeFile(t, filepath.Join(treeStateRoot, "backups", "run-tree-backup-fail"), "not a dir")
	record, err = treeStore.executeTreeRestoreItem("run-tree-backup-fail", fixedTime(), treeBackupFail)
	require.Error(t, err)
	require.Contains(t, err.Error(), "backup before restore")
	require.Equal(t, ItemResultFailed, record.Result)

	treeApplyFail := treeItems[0]
	treeApplyFailPlan := *treeApplyFail.plan
	treeApplyFail.plan = &treeApplyFailPlan
	require.NoError(t, os.Chmod(liveTree, 0o500))
	record, err = treeStore.executeTreeRestoreItem("run-tree-apply-fail", fixedTime(), treeApplyFail)
	require.NoError(t, os.Chmod(liveTree, 0o755))
	require.Error(t, err)
	require.Contains(t, err.Error(), "apply restore")
	require.Equal(t, ItemResultFailed, record.Result)
	require.Equal(t, []string{"state://backups/run-tree-apply-fail/custom.files_file-config-file"}, record.BackupRefs)
}

func setupRestoreFileSource(t *testing.T, liveBefore string, desiredAfter string, sourceRunID string) (string, string, *resolution.ResolvedProfile, *recipe.Recipe, string, *Store) {
	t.Helper()
	root, liveRoot, profile, rec := setupLedgerCustomFilesFixture(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	store, err := NewStore(stateRoot, WithClock(fixedTime))
	require.NoError(t, err)
	writeFile(t, filepath.Join(liveRoot, "config.txt"), liveBefore)
	writeFile(t, desiredArtifactPath(root), desiredAfter)
	plan, err := customfiles.PlanApply(customfiles.Request{Profile: profile, Recipe: rec, SettingRef: "custom.files:file", LocationRoots: map[string]string{"config": liveRoot}})
	require.NoError(t, err)
	_, err = store.ExecuteCustomFiles(plan, CustomFilesExecuteOptions{RunID: sourceRunID, ProfileStack: profile.Layers, StartedAt: fixedTime()})
	require.NoError(t, err)
	return root, liveRoot, profile, rec, stateRoot, store
}

func setupLedgerMultiFileFixture(t *testing.T) (string, string, *resolution.ResolvedProfile, *recipe.Recipe) {
	t.Helper()
	root := t.TempDir()
	liveRoot := filepath.Join(t.TempDir(), "cobona")
	require.NoError(t, os.MkdirAll(liveRoot, 0o755))
	writeLedgerV2Root(t, root)
	writeLedgerStack(t, root)
	writeFile(t, filepath.Join(root, "profiles", "layers", "global.yaml"), `schema: dotfiles-manager.v2.profile-layer
schemaVersion: 1
selections:
  custom.files:
    settings:
      first:
        scope: user
        artifact: artifacts/first.txt
      second:
        scope: user
        artifact: artifacts/second.txt
`)
	writeFile(t, filepath.Join(root, "recipes", "local", "custom.files", "recipe.yaml"), `schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: custom.files
displayName: Custom files
supportLevel: experimental
capability: read-write
locations:
  config:
    default: ~/.cobona
settings:
  first:
    scopeDefault: user
    resource: first-file
  second:
    scopeDefault: user
    resource: second-file
resources:
  first-file:
    driver: file
    location: config
    path: first.txt
  second-file:
    driver: file
    location: config
    path: second.txt
`)
	profile, err := resolution.Resolve(root, resolution.ResolveOptions{UserID: "leon", MachineID: "mbp"})
	require.NoError(t, err)
	rec, err := recipe.LoadCustomFiles(root)
	require.NoError(t, err)
	return root, liveRoot, profile, rec
}

func requireRunItem(t *testing.T, record RunRecord, settingRef string) ItemRecord {
	t.Helper()
	for _, item := range record.Items {
		if item.SettingRef == settingRef {
			return item
		}
	}
	t.Fatalf("run item %s not found", settingRef)
	return ItemRecord{}
}

func requirePreviewItem(t *testing.T, envelope preview.Envelope, settingRef string) preview.Item {
	t.Helper()
	for _, item := range envelope.Items {
		if item.SettingRef == settingRef {
			return item
		}
	}
	t.Fatalf("preview item %s not found", settingRef)
	return preview.Item{}
}

func requireDiagnosticCode(t *testing.T, diagnostics []Diagnostic, code string) {
	t.Helper()
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return
		}
	}
	t.Fatalf("diagnostic code %s not found in %#v", code, diagnostics)
}

func requirePreviewDiagnosticCode(t *testing.T, diagnostics []preview.Diagnostic, code string) {
	t.Helper()
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return
		}
	}
	t.Fatalf("preview diagnostic code %s not found in %#v", code, diagnostics)
}

func setupLedgerSelectedValueFixture(t *testing.T) (string, string, *resolution.ResolvedProfile) {
	t.Helper()
	root := t.TempDir()
	liveRoot := filepath.Join(t.TempDir(), "selected")
	require.NoError(t, os.MkdirAll(liveRoot, 0o755))
	writeLedgerV2Root(t, root)
	writeLedgerStack(t, root)
	writeFile(t, filepath.Join(root, "profiles", "layers", "global.yaml"), `schema: dotfiles-manager.v2.profile-layer
schemaVersion: 1
selections:
  test.app:
    settings:
      identity.email:
        scope: user
`)
	writeFile(t, filepath.Join(root, "recipes", "local", "test.app", "recipe.yaml"), `schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: test.app
displayName: Test App
supportLevel: experimental
capability: read-write
locations:
  config:
    default: `+liveRoot+`
settings:
  identity.email:
    scopeDefault: user
    resource: config-email
resources:
  config-email:
    driver: yaml-file
    location: config
    path: config.yaml
    selector:
      path: [user, email]
      createMissing: create
      duplicatePolicy: reject
      deleteKey: allow
`)
	profile, err := resolution.Resolve(root, resolution.ResolveOptions{UserID: "leon", MachineID: "mbp"})
	require.NoError(t, err)
	return root, liveRoot, profile
}

func writeBackupYAML(t *testing.T, path string, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}
