package ledger

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shpoont/dotfiles-manager/internal/v2/customfiles"
	"github.com/shpoont/dotfiles-manager/internal/v2/filedriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/filetreedriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/shpoont/dotfiles-manager/internal/v2/resolution"
	"github.com/shpoont/dotfiles-manager/internal/v2/selectedvalue"
	"github.com/stretchr/testify/require"
)

func TestExecuteCustomFilesApplyPersistsBackupBeforeMutationAndVerifiedLedger(t *testing.T) {
	t.Parallel()

	root, liveRoot, profile, rec := setupLedgerCustomFilesFixture(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	store, err := NewStore(stateRoot, WithClock(func() time.Time { return fixedTime().Add(time.Second) }))
	require.NoError(t, err)

	livePath := filepath.Join(liveRoot, "config.txt")
	desiredPath := desiredArtifactPath(root)
	writeFile(t, livePath, "live before\n")
	writeFile(t, desiredPath, "desired after\n")

	plan, err := customfiles.PlanApply(customfiles.Request{Profile: profile, Recipe: rec, SettingRef: "custom.files:file", LocationRoots: map[string]string{"config": liveRoot}})
	require.NoError(t, err)

	run, err := store.ExecuteCustomFiles(plan, CustomFilesExecuteOptions{RunID: "run-001", ProfileStack: profile.Layers, StartedAt: fixedTime()})
	require.NoError(t, err)
	require.NotNil(t, run.Result)
	require.True(t, run.Result.Verified)
	require.True(t, run.Result.Mutated)
	requireFile(t, livePath, "desired after\n")

	backup := requireBackupMetadata(t, stateRoot, "run-001")
	require.Equal(t, BackupMetadataSchema, backup.Schema)
	require.Len(t, backup.Items, 1)
	item := backup.Items[0]
	require.Equal(t, "state://backups/run-001/custom.files_file-config-file", item.Ref)
	require.Equal(t, "custom.files:file", item.SettingRef)
	require.Equal(t, recipe.FileDriverID, item.Driver)
	require.Equal(t, FileDriverVersion, item.DriverVersion)
	require.Equal(t, filedriver.NormalizerID, item.Before.Normalizer)
	require.True(t, item.Before.Exists)
	require.Equal(t, "payloads/custom.files_file-config-file/before", item.PayloadRelPath)
	require.True(t, item.Restore.Compatible)
	require.Contains(t, item.Restore.Message, "full restore execution")
	requireFile(t, filepath.Join(stateRoot, "backups", "run-001", item.PayloadRelPath), "live before\n")

	runRecord := requireRunRecord(t, stateRoot, "run-001")
	require.Equal(t, RunStatusVerified, runRecord.Status)
	require.Equal(t, 1, runRecord.Summary.Verified)
	require.Len(t, runRecord.Items, 1)
	runItem := runRecord.Items[0]
	require.Equal(t, ItemResultVerified, runItem.Result)
	require.True(t, runItem.Verification.Verified)
	require.Equal(t, "state://ledger/runs/run-001", runItem.ArtifactRefs.RunRecord)
	require.Equal(t, item.Ref, runItem.ArtifactRefs.Backup)
	require.Equal(t, item.Ref+"/payload", runItem.ArtifactRefs.BackupPayload)
	require.Equal(t, plan.Setting.DesiredURI, runItem.ArtifactRefs.Desired)
	require.Equal(t, desiredPath, runItem.ArtifactRefs.DesiredPath)
	require.Equal(t, livePath, runItem.ArtifactRefs.LivePath)
	require.Equal(t, runItem.Desired.Hash, runItem.VerifiedState.Hash)

	entries := requireLedgerEntries(t, stateRoot)
	require.Len(t, entries, 1)
	require.Equal(t, LedgerEntrySchema, entries[0].Schema)
	require.Equal(t, "run-001", entries[0].RunID)
	require.Equal(t, "apply", entries[0].Command)
	require.True(t, entries[0].Item.Verification.Verified)
	require.Equal(t, []string{item.Ref}, entries[0].Item.BackupRefs)
}

func TestExecuteCustomFilesDryRunDoesNotCreateLocalStateOrMutateLive(t *testing.T) {
	t.Parallel()

	root, liveRoot, profile, rec := setupLedgerCustomFilesFixture(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	store, err := NewStore(stateRoot, WithClock(fixedTime))
	require.NoError(t, err)

	livePath := filepath.Join(liveRoot, "config.txt")
	desiredPath := desiredArtifactPath(root)
	writeFile(t, livePath, "live\n")
	writeFile(t, desiredPath, "desired\n")
	plan, err := customfiles.PlanApply(customfiles.Request{Profile: profile, Recipe: rec, SettingRef: "custom.files:file", LocationRoots: map[string]string{"config": liveRoot}})
	require.NoError(t, err)

	run, err := store.ExecuteCustomFiles(plan, CustomFilesExecuteOptions{RunID: "run-dry", ProfileStack: profile.Layers, DryRun: true, StartedAt: fixedTime()})
	require.NoError(t, err)
	require.NotNil(t, run.Result)
	require.True(t, run.Result.DryRun)
	require.False(t, run.Result.Mutated)
	require.Nil(t, run.RunRecord)
	requireFile(t, livePath, "live\n")
	_, err = os.Stat(stateRoot)
	require.True(t, os.IsNotExist(err), "dry-run must not create backup, run, or ledger artifacts")
}

func TestCommitRunRecordsPartialFailuresButAppendsOnlyVerifiedLedgerEntries(t *testing.T) {
	t.Parallel()

	stateRoot := filepath.Join(t.TempDir(), "state")
	store, err := NewStore(stateRoot, WithClock(fixedTime))
	require.NoError(t, err)

	record := RunRecord{
		RunID:        "run-partial",
		StartedAt:    fixedTime().Format(time.RFC3339Nano),
		FinishedAt:   fixedTime().Add(time.Second).Format(time.RFC3339Nano),
		Command:      "apply",
		ProfileStack: []string{"global"},
		Items: []ItemRecord{
			{
				TargetRef:     "custom.files",
				SettingRef:    "custom.files:file",
				Operation:     "apply",
				ResourceID:    "config-file",
				Driver:        recipe.FileDriverID,
				DriverVersion: FileDriverVersion,
				ArtifactRefs:  ArtifactRefs{RunRecord: stateURI("ledger", "runs", "run-partial")},
				Desired:       NormalizedState{Exists: true, Hash: "desired", Normalizer: filedriver.NormalizerID, DriverVersion: FileDriverVersion},
				VerifiedState: NormalizedState{Exists: true, Hash: "desired", Normalizer: filedriver.NormalizerID, DriverVersion: FileDriverVersion},
				Verification:  Verification{Verified: true, Result: "verified"},
				Result:        ItemResultVerified,
			},
			{
				TargetRef:     "custom.files",
				SettingRef:    "custom.files:other",
				Operation:     "apply",
				ResourceID:    "other-file",
				Driver:        recipe.FileDriverID,
				DriverVersion: FileDriverVersion,
				Verification:  Verification{Verified: false, Result: "failed", Message: "backup failed"},
				Result:        ItemResultFailed,
				Diagnostics:   []Diagnostic{{Code: "backup-failed", Message: "backup failed"}},
			},
		},
	}

	entries, err := store.CommitRun(record)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "custom.files:file", entries[0].Item.SettingRef)

	got := requireRunRecord(t, stateRoot, "run-partial")
	require.Equal(t, RunStatusPartial, got.Status)
	require.Equal(t, 1, got.Summary.Verified)
	require.Equal(t, 1, got.Summary.Failed)
	require.Len(t, got.Items, 2)

	ledgerEntries := requireLedgerEntries(t, stateRoot)
	require.Len(t, ledgerEntries, 1)
	require.Equal(t, "custom.files:file", ledgerEntries[0].Item.SettingRef)
}

func TestBackupListReturnsFileAndTreeBackupsDeterministically(t *testing.T) {
	t.Parallel()

	fileRoot, fileLiveRoot, fileProfile, fileRecipe := setupLedgerCustomFilesFixture(t)
	treeRoot, treeLiveRoot, treeProfile, treeRecipe := setupLedgerFileTreeFixture(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	store, err := NewStore(stateRoot, WithClock(fixedTime))
	require.NoError(t, err)

	fileLivePath := filepath.Join(fileLiveRoot, "config.txt")
	writeFile(t, fileLivePath, "file backup\n")
	writeFile(t, desiredArtifactPath(fileRoot), "desired file\n")
	filePlan, err := customfiles.PlanApply(customfiles.Request{Profile: fileProfile, Recipe: fileRecipe, SettingRef: "custom.files:file", LocationRoots: map[string]string{"config": fileLiveRoot}})
	require.NoError(t, err)
	_, err = store.WriteCustomFilesBackup("run-b", fixedTime().Add(2*time.Second), filePlan, customfiles.BackupRequest{Operation: customfiles.OperationApply, SettingRef: "custom.files:file", ResourceID: "config-file", Path: fileLivePath, Before: filePlan.DestinationState})
	require.NoError(t, err)

	treeLivePath := filepath.Join(treeLiveRoot, "profiles")
	writeFile(t, filepath.Join(treeLivePath, "config.yaml"), "tree backup\n")
	writeFile(t, filepath.Join(desiredTreeArtifactPath(treeRoot), "config.yaml"), "desired tree\n")
	treePlan, err := customfiles.PlanApply(customfiles.Request{Profile: treeProfile, Recipe: treeRecipe, SettingRef: "custom.files:file", LocationRoots: map[string]string{"config": treeLiveRoot}})
	require.NoError(t, err)
	_, err = store.WriteCustomFilesBackup("run-a", fixedTime().Add(time.Second), treePlan, customfiles.BackupRequest{Operation: customfiles.OperationApply, SettingRef: "custom.files:file", ResourceID: "config-file", Path: treeLivePath, TreeBefore: treePlan.TreeDestinationState})
	require.NoError(t, err)

	backups, err := store.ListBackups()
	require.NoError(t, err)
	require.Len(t, backups, 2)
	require.Equal(t, "run-a", backups[0].RunID)
	require.Equal(t, recipe.FileTreeDriverID, backups[0].Items[0].Driver)
	require.Equal(t, "payloads/custom.files_file-config-file/tree", backups[0].Items[0].PayloadRelPath)
	requireFile(t, filepath.Join(stateRoot, "backups", "run-a", backups[0].Items[0].PayloadRelPath, "config.yaml"), "tree backup\n")
	require.Equal(t, "run-b", backups[1].RunID)
	require.Equal(t, recipe.FileDriverID, backups[1].Items[0].Driver)
}

func TestStateRootHelpersUseRepoHashAndRejectRepoInternalRoots(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(repo, "desired"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(repo, "profiles"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(repo, "recipes"), 0o755))

	stateID, err := RepoStateID(repo)
	require.NoError(t, err)
	require.Len(t, stateID, 64)
	defaultRoot, err := DefaultStateRoot(repo)
	require.NoError(t, err)
	require.Contains(t, defaultRoot, stateID)
	require.NoError(t, ValidateStateRoot(repo, filepath.Join(t.TempDir(), "state")))
	require.Error(t, ValidateStateRoot(repo, repo))
	require.Error(t, ValidateStateRoot(repo, filepath.Join(repo, "state")))
	require.Error(t, ValidateStateRoot(repo, filepath.Join(repo, "desired", "state")))
	require.Error(t, ValidateStateRoot(repo, filepath.Join(repo, "profiles", "state")))
	require.Error(t, ValidateStateRoot(repo, filepath.Join(repo, "recipes", "state")))
}

func TestExecuteCustomFilesRejectsStateRootInsideRepoBeforeMutation(t *testing.T) {
	t.Parallel()

	root, liveRoot, profile, rec := setupLedgerCustomFilesFixture(t)
	unsafeStateRoot := filepath.Join(root, "state")
	store, err := NewStore(unsafeStateRoot, WithClock(fixedTime))
	require.NoError(t, err)

	livePath := filepath.Join(liveRoot, "config.txt")
	desiredPath := desiredArtifactPath(root)
	writeFile(t, livePath, "live\n")
	writeFile(t, desiredPath, "desired\n")
	plan, err := customfiles.PlanApply(customfiles.Request{Profile: profile, Recipe: rec, SettingRef: "custom.files:file", LocationRoots: map[string]string{"config": liveRoot}})
	require.NoError(t, err)

	_, err = store.ExecuteCustomFiles(plan, CustomFilesExecuteOptions{RunID: "run-unsafe", ProfileStack: profile.Layers, StartedAt: fixedTime()})
	require.Error(t, err)
	require.Contains(t, err.Error(), "must not resolve inside repository")
	requireFile(t, livePath, "live\n")
}

func fixedTime() time.Time {
	return time.Date(2026, 6, 5, 20, 30, 0, 0, time.UTC)
}

func setupLedgerCustomFilesFixture(t *testing.T) (string, string, *resolution.ResolvedProfile, *recipe.Recipe) {
	t.Helper()
	root := t.TempDir()
	liveRoot := filepath.Join(t.TempDir(), "cobona")
	require.NoError(t, os.MkdirAll(liveRoot, 0o755))
	writeLedgerV2Root(t, root)
	writeLedgerStack(t, root)
	writeLedgerLayer(t, root)
	writeLedgerRecipe(t, root)
	profile, err := resolution.Resolve(root, resolution.ResolveOptions{UserID: "leon", MachineID: "mbp"})
	require.NoError(t, err)
	rec, err := recipe.LoadCustomFiles(root)
	require.NoError(t, err)
	return root, liveRoot, profile, rec
}

func setupLedgerFileTreeFixture(t *testing.T) (string, string, *resolution.ResolvedProfile, *recipe.Recipe) {
	t.Helper()
	root := t.TempDir()
	liveRoot := filepath.Join(t.TempDir(), "cobona")
	require.NoError(t, os.MkdirAll(liveRoot, 0o755))
	writeLedgerV2Root(t, root)
	writeLedgerStack(t, root)
	writeLedgerTreeLayer(t, root)
	writeLedgerTreeRecipe(t, root)
	profile, err := resolution.Resolve(root, resolution.ResolveOptions{UserID: "leon", MachineID: "mbp"})
	require.NoError(t, err)
	rec, err := recipe.LoadCustomFiles(root)
	require.NoError(t, err)
	return root, liveRoot, profile, rec
}

func writeLedgerV2Root(t *testing.T, root string) {
	t.Helper()
	writeFile(t, filepath.Join(root, resolution.RootConfigFile), "schema: dotfiles-manager.v2.root-config\nschemaVersion: 1\nactiveProfileStack: default\n")
}

func writeLedgerStack(t *testing.T, root string) {
	t.Helper()
	writeFile(t, filepath.Join(root, "profiles", "stacks", "default.yaml"), "schema: dotfiles-manager.v2.profile-stack\nschemaVersion: 1\nprofileStack:\n  - global\n")
}

func writeLedgerLayer(t *testing.T, root string) {
	t.Helper()
	writeFile(t, filepath.Join(root, "profiles", "layers", "global.yaml"), `schema: dotfiles-manager.v2.profile-layer
schemaVersion: 1
selections:
  custom.files:
    settings:
      file:
        scope: user
        artifact: artifacts/config.txt
`)
}

func writeLedgerTreeLayer(t *testing.T, root string) {
	t.Helper()
	writeFile(t, filepath.Join(root, "profiles", "layers", "global.yaml"), `schema: dotfiles-manager.v2.profile-layer
schemaVersion: 1
selections:
  custom.files:
    settings:
      file:
        scope: user
        artifact: artifacts/profiles
`)
}

func writeLedgerRecipe(t *testing.T, root string) {
	t.Helper()
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
  file:
    scopeDefault: user
    resource: config-file
resources:
  config-file:
    driver: file
    location: config
    path: config.txt
`)
}

func writeLedgerTreeRecipe(t *testing.T, root string) {
	t.Helper()
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
  file:
    scopeDefault: user
    resource: config-file
resources:
  config-file:
    driver: file-tree
    location: config
    path: profiles
    include:
      - "**"
    exclude:
      - "cache/**"
`)
}

func desiredArtifactPath(root string) string {
	return filepath.Join(root, "desired", "user", "leon", "targets", "custom.files", "artifacts", "config.txt")
}

func desiredTreeArtifactPath(root string) string {
	return filepath.Join(root, "desired", "user", "leon", "targets", "custom.files", "artifacts", "profiles")
}

func writeFile(t *testing.T, path string, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func requireFile(t *testing.T, path string, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, want, string(got))
}

func requireBackupMetadata(t *testing.T, stateRoot string, runID string) BackupMetadata {
	t.Helper()
	metadata, err := readBackupMetadata(filepath.Join(stateRoot, "backups", runID, "backup.yaml"))
	require.NoError(t, err)
	return metadata
}

func requireRunRecord(t *testing.T, stateRoot string, runID string) RunRecord {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join(stateRoot, "ledger", "runs", runID+".json"))
	require.NoError(t, err)
	var record RunRecord
	require.NoError(t, json.Unmarshal(payload, &record))
	return NormalizeRunRecord(record)
}

func requireLedgerEntries(t *testing.T, stateRoot string) []LedgerEntry {
	t.Helper()
	path := filepath.Join(stateRoot, "ledger", "ledger.jsonl")
	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()
	var entries []LedgerEntry
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var entry LedgerEntry
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &entry))
		entries = append(entries, entry)
	}
	require.NoError(t, scanner.Err())
	return NormalizeLedgerEntries(entries)
}

var _ = filetreedriver.NormalizerID

func TestExecuteCustomFilesSavePersistsVerifiedLedgerWithoutBackup(t *testing.T) {
	t.Parallel()

	root, liveRoot, profile, rec := setupLedgerCustomFilesFixture(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	store, err := NewStore(stateRoot, WithClock(fixedTime))
	require.NoError(t, err)

	livePath := filepath.Join(liveRoot, "config.txt")
	writeFile(t, livePath, "live to save\n")
	plan, err := customfiles.PlanSave(customfiles.Request{Profile: profile, Recipe: rec, SettingRef: "custom.files:file", LocationRoots: map[string]string{"config": liveRoot}})
	require.NoError(t, err)

	run, err := store.ExecuteCustomFiles(plan, CustomFilesExecuteOptions{RunID: "run-save", ProfileStack: []string{"global"}, StartedAt: fixedTime()})
	require.NoError(t, err)
	require.True(t, run.Result.Verified)
	require.Nil(t, run.Backup)
	requireFile(t, desiredArtifactPath(root), "live to save\n")

	record := requireRunRecord(t, stateRoot, "run-save")
	require.Equal(t, RunStatusVerified, record.Status)
	require.Empty(t, record.Items[0].BackupRefs)
	require.Equal(t, ItemResultVerified, record.Items[0].Result)
	entries := requireLedgerEntries(t, stateRoot)
	require.Len(t, entries, 1)
	require.Empty(t, entries[0].Item.BackupRefs)
	_, err = store.ReadBackup("run-save")
	require.Error(t, err)
}

func TestExecuteCustomFilesFileTreeApplyPersistsBackupAndVerifiedLedger(t *testing.T) {
	t.Parallel()

	root, liveRoot, profile, rec := setupLedgerFileTreeFixture(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	store, err := NewStore(stateRoot, WithClock(fixedTime))
	require.NoError(t, err)

	liveTree := filepath.Join(liveRoot, "profiles")
	desiredTree := desiredTreeArtifactPath(root)
	writeFile(t, filepath.Join(liveTree, "config.yaml"), "tree live before\n")
	writeFile(t, filepath.Join(liveTree, "cache", "ignored.yaml"), "ignored\n")
	writeFile(t, filepath.Join(desiredTree, "config.yaml"), "tree desired after\n")
	plan, err := customfiles.PlanApply(customfiles.Request{Profile: profile, Recipe: rec, SettingRef: "custom.files:file", LocationRoots: map[string]string{"config": liveRoot}})
	require.NoError(t, err)

	run, err := store.ExecuteCustomFiles(plan, CustomFilesExecuteOptions{RunID: "run-tree", ProfileStack: profile.Layers, StartedAt: fixedTime()})
	require.NoError(t, err)
	require.True(t, run.Result.Verified)
	requireFile(t, filepath.Join(liveTree, "config.yaml"), "tree desired after\n")
	requireFile(t, filepath.Join(liveTree, "cache", "ignored.yaml"), "ignored\n")

	backup := requireBackupMetadata(t, stateRoot, "run-tree")
	require.Len(t, backup.Items, 1)
	item := backup.Items[0]
	require.Equal(t, recipe.FileTreeDriverID, item.Driver)
	require.Equal(t, FileTreeDriverVersion, item.DriverVersion)
	require.Equal(t, filetreedriver.NormalizerID, item.Before.Normalizer)
	require.Equal(t, "payloads/custom.files_file-config-file/tree", item.PayloadRelPath)
	requireFile(t, filepath.Join(stateRoot, "backups", "run-tree", item.PayloadRelPath, "config.yaml"), "tree live before\n")
	assertMissing(t, filepath.Join(stateRoot, "backups", "run-tree", item.PayloadRelPath, "cache", "ignored.yaml"))

	record := requireRunRecord(t, stateRoot, "run-tree")
	require.Equal(t, RunStatusVerified, record.Status)
	require.Equal(t, recipe.FileTreeDriverID, record.Items[0].Driver)
	require.Equal(t, item.Ref, record.Items[0].ArtifactRefs.Backup)
	require.Len(t, requireLedgerEntries(t, stateRoot), 1)
}

func TestFailedRunRecordAndNoSuccessLedgerForExecutionError(t *testing.T) {
	t.Parallel()

	root, liveRoot, profile, rec := setupLedgerCustomFilesFixture(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	store, err := NewStore(stateRoot, WithClock(fixedTime))
	require.NoError(t, err)

	livePath := filepath.Join(liveRoot, "config.txt")
	desiredPath := desiredArtifactPath(root)
	writeFile(t, livePath, "live\n")
	writeFile(t, desiredPath, "desired\n")
	plan, err := customfiles.PlanApply(customfiles.Request{Profile: profile, Recipe: rec, SettingRef: "custom.files:file", LocationRoots: map[string]string{"config": liveRoot}})
	require.NoError(t, err)

	failed := BuildFailedCustomFilesRunRecord(plan, RunRecordContext{RunID: "run-failed", StartedAt: fixedTime(), FinishedAt: fixedTime().Add(time.Second), Command: "apply", ProfileStack: []string{"global"}, Err: os.ErrPermission})
	require.Equal(t, RunStatusFailed, failed.Status)
	require.Equal(t, ItemResultFailed, failed.Items[0].Result)
	require.False(t, failed.Items[0].Verification.Verified)
	entries, err := store.CommitRun(failed)
	require.NoError(t, err)
	require.Empty(t, entries)
	_, err = os.Stat(filepath.Join(stateRoot, "ledger", "ledger.jsonl"))
	require.True(t, os.IsNotExist(err))
	got := requireRunRecord(t, stateRoot, "run-failed")
	require.Equal(t, RunStatusFailed, got.Status)
}

func TestStoreHelperErrorBranchesAndNormalization(t *testing.T) {
	t.Parallel()

	_, err := NewStore("")
	require.Error(t, err)
	var nilStore *Store
	require.Empty(t, nilStore.Root())
	_, err = nilStore.ListBackups()
	require.Error(t, err)
	require.Error(t, validateRunID("../bad"))
	require.Equal(t, "state://ledger/runs/run-1", stateURI("/ledger/", "runs", "run-1"))
	require.Empty(t, backupPayloadURI(""))
	require.Empty(t, formatTime(time.Time{}))

	stateRoot := filepath.Join(t.TempDir(), "state")
	store, err := NewStore(stateRoot)
	require.NoError(t, err)
	require.Equal(t, stateRoot, store.Root())
	backups, err := store.ListBackups()
	require.NoError(t, err)
	require.Empty(t, backups)
	require.Error(t, store.WriteRunRecord(RunRecord{RunID: "bad/run"}))
	require.NoError(t, store.AppendLedgerEntries(nil))

	metadata := NormalizeBackupMetadata(BackupMetadata{RunID: "run-z", Items: []BackupItem{{Ref: "b"}, {Ref: "a"}}})
	require.Equal(t, BackupMetadataSchema, metadata.Schema)
	require.Equal(t, SchemaVersion, metadata.SchemaVersion)
	require.Equal(t, "a", metadata.Items[0].Ref)

	repoFile := filepath.Join(t.TempDir(), "repo-file")
	writeFile(t, repoFile, "x")
	_, err = RepoStateID(repoFile)
	require.Error(t, err)
	_, err = RepoStateID(filepath.Join(t.TempDir(), "missing"))
	require.Error(t, err)
}

func TestDefaultStateRootForOSBranches(t *testing.T) {
	darwinRoot, err := defaultStateRootForOS("darwin", "abc123")
	require.NoError(t, err)
	require.Contains(t, darwinRoot, filepath.Join("Library", "Application Support", "dotfiles-manager", "v2", "abc123"))

	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "xdg-state"))
	linuxRoot, err := defaultStateRootForOS("linux", "abc123")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(os.Getenv("XDG_STATE_HOME"), "dotfiles-manager", "v2", "abc123"), linuxRoot)

	t.Setenv("XDG_STATE_HOME", "")
	linuxFallback, err := defaultStateRootForOS("linux", "abc123")
	require.NoError(t, err)
	require.Contains(t, linuxFallback, filepath.Join(".local", "state", "dotfiles-manager", "v2", "abc123"))

	_, err = defaultStateRootForOS("windows", "abc123")
	require.Error(t, err)
}

func assertMissing(t *testing.T, path string) {
	t.Helper()
	_, err := os.Stat(path)
	require.True(t, os.IsNotExist(err), "expected %s to be missing, got %v", path, err)
}

func TestExecuteCustomFilesWritesFailureRunRecordWhenMutationValidationFails(t *testing.T) {
	t.Parallel()

	root, liveRoot, profile, rec := setupLedgerCustomFilesFixture(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	store, err := NewStore(stateRoot, WithClock(fixedTime))
	require.NoError(t, err)

	livePath := filepath.Join(liveRoot, "config.txt")
	desiredPath := desiredArtifactPath(root)
	writeFile(t, livePath, "live\n")
	writeFile(t, desiredPath, "desired\n")
	plan, err := customfiles.PlanApply(customfiles.Request{Profile: profile, Recipe: rec, SettingRef: "custom.files:file", LocationRoots: map[string]string{"config": liveRoot}})
	require.NoError(t, err)

	require.NoError(t, os.Remove(desiredPath))
	require.NoError(t, os.Symlink(livePath, desiredPath))
	run, err := store.ExecuteCustomFiles(plan, CustomFilesExecuteOptions{RunID: "run-invalid", ProfileStack: profile.Layers, StartedAt: fixedTime()})
	require.Error(t, err)
	require.NotNil(t, run)
	require.Nil(t, run.Result)
	record := requireRunRecord(t, stateRoot, "run-invalid")
	require.Equal(t, RunStatusFailed, record.Status)
	require.Equal(t, ItemResultFailed, record.Items[0].Result)
	require.False(t, record.Items[0].Verification.Verified)
	require.Contains(t, record.Items[0].Diagnostics[0].Message, "unsafe-path")
	_, statErr := os.Stat(filepath.Join(stateRoot, "ledger", "ledger.jsonl"))
	require.True(t, os.IsNotExist(statErr))
	requireFile(t, livePath, "live\n")
}

func TestBackupUpsertAndPayloadErrorBranches(t *testing.T) {
	t.Parallel()

	root, liveRoot, profile, rec := setupLedgerCustomFilesFixture(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	store, err := NewStore(stateRoot, WithClock(fixedTime))
	require.NoError(t, err)
	livePath := filepath.Join(liveRoot, "config.txt")
	desiredPath := desiredArtifactPath(root)
	writeFile(t, livePath, "first\n")
	writeFile(t, desiredPath, "desired\n")
	plan, err := customfiles.PlanApply(customfiles.Request{Profile: profile, Recipe: rec, SettingRef: "custom.files:file", LocationRoots: map[string]string{"config": liveRoot}})
	require.NoError(t, err)

	_, err = store.WriteCustomFilesBackup("run-upsert", fixedTime(), plan, customfiles.BackupRequest{Path: livePath, Before: plan.DestinationState})
	require.NoError(t, err)
	writeFile(t, livePath, "second\n")
	state, err := filedriver.Driver{}.ReadCurrent(filedriver.Target{LocationID: "config", Root: liveRoot, RelPath: "config.txt"})
	require.NoError(t, err)
	_, err = store.WriteCustomFilesBackup("run-upsert", fixedTime().Add(time.Second), plan, customfiles.BackupRequest{Path: livePath, Before: state})
	require.NoError(t, err)
	metadata := requireBackupMetadata(t, stateRoot, "run-upsert")
	require.Len(t, metadata.Items, 1)
	require.Equal(t, state.SHA256, metadata.Items[0].Before.Hash)
	requireFile(t, filepath.Join(stateRoot, "backups", "run-upsert", metadata.Items[0].PayloadRelPath), "second\n")

	err = writeTreePayload(filepath.Join(t.TempDir(), "payload"), filetreedriver.State{Exists: true, Entries: []filetreedriver.Entry{{Path: "../bad", Kind: filetreedriver.EntryFile}}})
	require.Error(t, err)
	err = writeTreePayload(filepath.Join(t.TempDir(), "payload"), filetreedriver.State{Exists: true, Entries: []filetreedriver.Entry{{Path: "ok", Kind: filetreedriver.EntryKind("socket")}}})
	require.Error(t, err)

	blocker := filepath.Join(t.TempDir(), "blocker")
	writeFile(t, blocker, "not a dir")
	err = writeFileAtomic(filepath.Join(blocker, "child"), []byte("x"), 0o644)
	require.Error(t, err)

	var nilStore *Store
	require.Error(t, nilStore.WriteRunRecord(RunRecord{RunID: "run"}))
	require.Error(t, nilStore.AppendLedgerEntries([]LedgerEntry{{RunID: "run", Item: ItemRecord{SettingRef: "x", Verification: Verification{Verified: true}, Result: ItemResultVerified}}}))
	_, err = nilStore.CommitRun(RunRecord{RunID: "run"})
	require.Error(t, err)
}

func TestWriteSelectedValueBackupPersistsMetadataAndPayload(t *testing.T) {
	t.Parallel()

	stateRoot := t.TempDir()
	store, err := NewStore(stateRoot, WithClock(fixedTime))
	require.NoError(t, err)

	item, err := store.WriteSelectedValueBackup("run-selected", fixedTime(), SelectedValueBackupRequest{
		TargetRef:  "test.app",
		SettingRef: "test.app:identity.email",
		ResourceID: "config-email",
		Driver:     recipe.YAMLFileDriverID,
		LivePath:   filepath.Join(t.TempDir(), "config.yaml"),
		Before:     NormalizedState{Exists: true, Hash: "abc", Normalizer: "yaml-file.selected-scalar.v1"},
		BeforeFile: []byte("user:\n  email: old@example.com\n"),
	})
	require.NoError(t, err)
	require.Equal(t, "state://backups/run-selected/test.app_identity.email-config-email", item.Ref)
	require.Equal(t, YAMLFileSelectedDriverVersion, item.DriverVersion)
	require.Equal(t, "payloads/test.app_identity.email-config-email/before", item.PayloadRelPath)
	require.Equal(t, "yaml-file.selected-scalar.v1", item.Restore.Normalizer)
	requireFile(t, filepath.Join(stateRoot, "backups", "run-selected", item.PayloadRelPath), "user:\n  email: old@example.com\n")

	metadata := requireBackupMetadata(t, stateRoot, "run-selected")
	require.Len(t, metadata.Items, 1)
	require.NotContains(t, mustMarshalJSON(t, metadata), "old@example.com")

	absent, err := store.WriteSelectedValueBackup("run-selected-absent", fixedTime(), SelectedValueBackupRequest{
		TargetRef:  "test.app",
		SettingRef: "test.app:identity.email",
		ResourceID: "config-email",
		Driver:     recipe.IniFileDriverID,
		Before:     NormalizedState{Exists: false},
	})
	require.NoError(t, err)
	require.Empty(t, absent.PayloadRelPath)
	require.Equal(t, IniFileSelectedDriverVersion, SelectedValueDriverVersion(recipe.IniFileDriverID))
	require.Equal(t, JSONFileSelectedDriverVersion, SelectedValueDriverVersion(recipe.JSONFileDriverID))
	require.Equal(t, YAMLFileSelectedDriverVersion, SelectedValueDriverVersion(recipe.YAMLFileDriverID))
	require.Equal(t, "other", SelectedValueDriverVersion("other"))
	require.NotEmpty(t, SelectedValueNormalizer(recipe.IniFileDriverID))
	require.NotEmpty(t, SelectedValueNormalizer(recipe.JSONFileDriverID))
	require.NotEmpty(t, SelectedValueNormalizer(recipe.YAMLFileDriverID))
	require.Empty(t, SelectedValueNormalizer("other"))
	require.Equal(t, NormalizedState{Exists: true, Hash: "h", Normalizer: "n", DriverVersion: YAMLFileSelectedDriverVersion}, SelectedValueState(selectedvalue.Snapshot{Exists: true, SHA256: "h", Normalizer: "n"}, recipe.YAMLFileDriverID))
}

func TestExecuteAndBackupValidationBranches(t *testing.T) {
	t.Parallel()

	root, liveRoot, profile, rec := setupLedgerCustomFilesFixture(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	store, err := NewStore(stateRoot, WithClock(fixedTime))
	require.NoError(t, err)
	writeFile(t, desiredArtifactPath(root), "desired\n")
	plan, err := customfiles.PlanApply(customfiles.Request{Profile: profile, Recipe: rec, SettingRef: "custom.files:file", LocationRoots: map[string]string{"config": liveRoot}})
	require.NoError(t, err)

	var nilStore *Store
	_, err = nilStore.ExecuteCustomFiles(plan, CustomFilesExecuteOptions{RunID: "run"})
	require.Error(t, err)
	_, err = store.ExecuteCustomFiles(nil, CustomFilesExecuteOptions{RunID: "run"})
	require.Error(t, err)
	_, err = store.ExecuteCustomFiles(plan, CustomFilesExecuteOptions{RunID: "../bad"})
	require.Error(t, err)

	_, err = nilStore.WriteCustomFilesBackup("run", fixedTime(), plan, customfiles.BackupRequest{})
	require.Error(t, err)
	_, err = store.WriteCustomFilesBackup("run", fixedTime(), nil, customfiles.BackupRequest{})
	require.Error(t, err)
	_, err = store.WriteCustomFilesBackup("../bad", fixedTime(), plan, customfiles.BackupRequest{})
	require.Error(t, err)

	item, err := store.WriteCustomFilesBackup("run-absent", time.Time{}, plan, customfiles.BackupRequest{Path: filepath.Join(liveRoot, "config.txt"), Before: plan.DestinationState})
	require.NoError(t, err)
	require.False(t, item.Before.Exists)
	require.Empty(t, item.PayloadRelPath)
	backup := requireBackupMetadata(t, stateRoot, "run-absent")
	require.Equal(t, fixedTime().Format(time.RFC3339Nano), backup.Items[0].CreatedAt)
}

func TestBackupReadListAndMetadataErrorBranches(t *testing.T) {
	t.Parallel()

	stateRoot := filepath.Join(t.TempDir(), "state")
	store, err := NewStore(stateRoot, WithClock(fixedTime))
	require.NoError(t, err)
	_, err = store.ReadBackup("../bad")
	require.Error(t, err)
	_, err = store.ReadBackup("missing")
	require.Error(t, err)

	corruptPath := filepath.Join(stateRoot, "backups", "corrupt", "backup.yaml")
	writeFile(t, corruptPath, "schema: [not yaml\n")
	_, err = store.ListBackups()
	require.Error(t, err)
	_, err = readBackupMetadata(corruptPath)
	require.Error(t, err)

	blocker := filepath.Join(t.TempDir(), "blocker")
	writeFile(t, blocker, "not a dir")
	err = writeBackupMetadata(filepath.Join(blocker, "backup.yaml"), BackupMetadata{RunID: "run"})
	require.Error(t, err)
}

func TestNormalizationDefaultBranches(t *testing.T) {
	t.Parallel()

	verified := NormalizeItemRecord(ItemRecord{SettingRef: "s", Verification: Verification{Verified: true}})
	require.Equal(t, ItemResultVerified, verified.Result)
	failed := NormalizeItemRecord(ItemRecord{SettingRef: "s"})
	require.Equal(t, ItemResultFailed, failed.Result)

	record := NormalizeRunRecord(RunRecord{RunID: "run-norm", ProfileStack: []string{" global ", "global", "machine"}, Items: []ItemRecord{
		{TargetRef: "b", SettingRef: "b:two", Verification: Verification{Verified: true}, Result: ItemResultVerified},
		{TargetRef: "a", SettingRef: "a:one", Result: ItemResultSkipped},
		{TargetRef: "c", SettingRef: "c:dry", Result: ItemResultDryRun},
	}})
	require.Equal(t, RunRecordSchema, record.Schema)
	require.Equal(t, []string{"global", "machine"}, record.ProfileStack)
	require.Equal(t, "a:one", record.Items[0].SettingRef)
	require.Equal(t, 1, record.Summary.Verified)
	require.Equal(t, 1, record.Summary.Skipped)
	require.Equal(t, 1, record.Summary.DryRun)
	require.Equal(t, RunStatusVerified, record.Status)

	dry := summarizeItems([]ItemRecord{{Result: ItemResultDryRun}})
	require.Equal(t, RunStatusDryRun, dry.Status)
	failedSummary := summarizeItems([]ItemRecord{{Result: ItemResultFailed}})
	require.Equal(t, RunStatusFailed, failedSummary.Status)

	entries := NormalizeLedgerEntries([]LedgerEntry{{RunID: "run", Item: ItemRecord{TargetRef: "b", SettingRef: "b:two"}}, {RunID: "run", Item: ItemRecord{TargetRef: "a", SettingRef: "a:one"}}})
	require.Equal(t, LedgerEntrySchema, entries[0].Schema)
	require.Equal(t, "a:one", entries[0].Item.SettingRef)
}

func mustMarshalJSON(t *testing.T, value any) string {
	t.Helper()
	payload, err := json.Marshal(value)
	require.NoError(t, err)
	return string(payload)
}

func TestWriteSelectedValueBackupValidationAndKeyBranches(t *testing.T) {
	t.Parallel()

	var nilStore *Store
	_, err := nilStore.WriteSelectedValueBackup("run", fixedTime(), SelectedValueBackupRequest{})
	require.Error(t, err)

	stateRoot := t.TempDir()
	store, err := NewStore(stateRoot, WithClock(fixedTime))
	require.NoError(t, err)
	_, err = store.WriteSelectedValueBackup("../bad", fixedTime(), SelectedValueBackupRequest{})
	require.Error(t, err)

	zeroTime, err := store.WriteSelectedValueBackup("run-zero-time", time.Time{}, SelectedValueBackupRequest{
		TargetRef:  "test.app",
		SettingRef: "test.app:identity.email",
		ResourceID: "config-email",
		Driver:     recipe.JSONFileDriverID,
		Before:     NormalizedState{Exists: false},
	})
	require.NoError(t, err)
	require.Equal(t, fixedTime().Format(time.RFC3339Nano), zeroTime.CreatedAt)

	key := selectedValueItemKey("test.app:identity.email", "config-email")
	blockingParent := filepath.Join(stateRoot, "backups", "run-payload-error", "payloads", key)
	require.NoError(t, os.MkdirAll(filepath.Dir(blockingParent), 0o755))
	require.NoError(t, os.WriteFile(blockingParent, []byte("not a directory"), 0o644))
	_, err = store.WriteSelectedValueBackup("run-payload-error", fixedTime(), SelectedValueBackupRequest{
		TargetRef:  "test.app",
		SettingRef: "test.app:identity.email",
		ResourceID: "config-email",
		Driver:     recipe.YAMLFileDriverID,
		Before:     NormalizedState{Exists: true, Hash: "abc"},
		BeforeFile: []byte("payload"),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "write selected-value backup payload")

	require.Equal(t, "item--", selectedValueItemKey("", ""))
	require.Equal(t, "item-.-", selectedValueItemKey(".", ""))
}
