package ledger

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shpoont/dotfiles-manager/internal/v2/customfiles"
	"github.com/shpoont/dotfiles-manager/internal/v2/preview"
	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/shpoont/dotfiles-manager/internal/v2/resolution"
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
	unchanged := restorePreviewItem(itemPlans[0].plan, itemPlans[0].source, itemPlans[0].livePath, itemPlans[0].desiredPath, preview.Change{Kind: "unchanged"}, true)
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

func writeBackupYAML(t *testing.T, path string, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}
