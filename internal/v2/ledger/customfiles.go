package ledger

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/shpoont/dotfiles-manager/internal/v2/customfiles"
	"github.com/shpoont/dotfiles-manager/internal/v2/filedriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/filetreedriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
)

const (
	FileDriverVersion     = "file.driver.v1"
	FileTreeDriverVersion = "file-tree.driver.v1"
)

var itemKeyRegexp = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

type CustomFilesExecuteOptions struct {
	RunID        string
	ProfileStack []string
	DryRun       bool
	StartedAt    time.Time
}

type CustomFilesRun struct {
	Result        *customfiles.Result
	RunRecord     *RunRecord
	LedgerEntries []LedgerEntry
	Backup        *BackupMetadata
}

func (s *Store) ExecuteCustomFiles(plan *customfiles.Plan, opts CustomFilesExecuteOptions) (*CustomFilesRun, error) {
	if s == nil {
		return nil, fmt.Errorf("ledger store is required")
	}
	if plan == nil {
		return nil, fmt.Errorf("custom.files plan is required")
	}
	if err := ValidateStateRoot(plan.RepoRoot, s.root); err != nil {
		return nil, err
	}
	if err := validateRunID(opts.RunID); err != nil {
		return nil, err
	}
	started := opts.StartedAt
	if started.IsZero() {
		started = s.now().UTC()
	}
	command := string(plan.Operation)
	profileStack := append([]string(nil), opts.ProfileStack...)

	if opts.DryRun {
		result, err := customfiles.Execute(plan, customfiles.ExecuteOptions{DryRun: true})
		if err != nil {
			return nil, err
		}
		return &CustomFilesRun{Result: result}, nil
	}

	execOpts := customfiles.ExecuteOptions{}
	if plan.Operation == customfiles.OperationApply {
		execOpts.BackupHook = s.customFilesBackupHook(opts.RunID, started, plan)
	}

	result, err := customfiles.Execute(plan, execOpts)
	finished := s.now().UTC()
	if err != nil {
		record := BuildFailedCustomFilesRunRecord(plan, RunRecordContext{
			RunID:        opts.RunID,
			StartedAt:    started,
			FinishedAt:   finished,
			Command:      command,
			ProfileStack: profileStack,
			Err:          err,
		})
		if recordErr := s.WriteRunRecord(record); recordErr != nil {
			return nil, fmt.Errorf("%w; additionally failed to write run record: %v", err, recordErr)
		}
		return &CustomFilesRun{RunRecord: &record}, err
	}

	record := BuildCustomFilesRunRecord(plan, result, RunRecordContext{
		RunID:        opts.RunID,
		StartedAt:    started,
		FinishedAt:   finished,
		Command:      command,
		ProfileStack: profileStack,
	})
	entries, err := s.CommitRun(record)
	if err != nil {
		return nil, fmt.Errorf("commit verified run %s: %w", opts.RunID, err)
	}
	var backup *BackupMetadata
	if result.Backup != nil {
		metadata, readErr := s.ReadBackup(opts.RunID)
		if readErr != nil {
			return nil, fmt.Errorf("read backup metadata for run %s after commit: %w", opts.RunID, readErr)
		}
		backup = &metadata
	}
	return &CustomFilesRun{Result: result, RunRecord: &record, LedgerEntries: entries, Backup: backup}, nil
}

type RunRecordContext struct {
	RunID        string
	StartedAt    time.Time
	FinishedAt   time.Time
	Command      string
	ProfileStack []string
	Err          error
}

func BuildCustomFilesRunRecord(plan *customfiles.Plan, result *customfiles.Result, ctx RunRecordContext) RunRecord {
	item := BuildCustomFilesItemRecord(plan, result, nil, ctx.RunID)
	return NormalizeRunRecord(RunRecord{
		Schema:        RunRecordSchema,
		SchemaVersion: SchemaVersion,
		RunID:         ctx.RunID,
		StartedAt:     formatTime(ctx.StartedAt),
		FinishedAt:    formatTime(ctx.FinishedAt),
		Command:       ctx.Command,
		ProfileStack:  append([]string(nil), ctx.ProfileStack...),
		Items:         []ItemRecord{item},
	})
}

func BuildFailedCustomFilesRunRecord(plan *customfiles.Plan, ctx RunRecordContext) RunRecord {
	item := BuildCustomFilesItemRecord(plan, nil, ctx.Err, ctx.RunID)
	return NormalizeRunRecord(RunRecord{
		Schema:        RunRecordSchema,
		SchemaVersion: SchemaVersion,
		RunID:         ctx.RunID,
		StartedAt:     formatTime(ctx.StartedAt),
		FinishedAt:    formatTime(ctx.FinishedAt),
		Command:       ctx.Command,
		ProfileStack:  append([]string(nil), ctx.ProfileStack...),
		Items:         []ItemRecord{item},
	})
}

func BuildCustomFilesItemRecord(plan *customfiles.Plan, result *customfiles.Result, runErr error, runID string) ItemRecord {
	item := ItemRecord{
		TargetRef:      plan.Setting.TargetID,
		SettingRef:     plan.Setting.Ref(),
		Operation:      string(plan.Operation),
		ResourceID:     plan.ResourceID,
		Driver:         plan.Resource.Driver,
		DriverVersion:  driverVersion(plan.Resource.Driver),
		DesiredURI:     plan.Setting.DesiredURI,
		DesiredRelPath: plan.DesiredRelPath,
		ArtifactRefs: ArtifactRefs{
			Desired:    plan.Setting.DesiredURI,
			DesiredURI: plan.Setting.DesiredURI,
			RunRecord:  stateURI("ledger", "runs", runID),
			Ledger:     stateURI("ledger", "ledger.jsonl"),
		},
	}

	if plan.Resource.Driver == recipe.FileTreeDriverID {
		item.LivePath, item.DesiredPath = resolveTreePaths(plan)
		item.ArtifactRefs.LivePath = item.LivePath
		item.ArtifactRefs.DesiredPath = item.DesiredPath
		item.Before, item.Desired = treeStatesForRecord(plan, result)
	} else {
		item.LivePath, item.DesiredPath = resolveFilePaths(plan)
		item.ArtifactRefs.LivePath = item.LivePath
		item.ArtifactRefs.DesiredPath = item.DesiredPath
		item.Before, item.Desired = fileStatesForRecord(plan, result)
	}

	if result != nil && result.Backup != nil {
		item.BackupRefs = []string{result.Backup.ID}
		item.ArtifactRefs.Backup = result.Backup.ID
		item.ArtifactRefs.BackupPayload = backupPayloadURI(result.Backup.ID)
	}

	if runErr != nil {
		item.Result = ItemResultFailed
		item.Verification = Verification{Verified: false, Result: "failed", Message: runErr.Error()}
		item.Diagnostics = []Diagnostic{{Code: "custom-files-execute-failed", Message: runErr.Error(), Path: item.LivePath}}
		return NormalizeItemRecord(item)
	}

	if result != nil && result.Verified {
		item.VerifiedState = item.Desired
		item.Result = ItemResultVerified
		item.Verification = Verification{Verified: true, Result: "verified"}
		return NormalizeItemRecord(item)
	}

	item.Result = ItemResultFailed
	item.Verification = Verification{Verified: false, Result: "unverified", Message: "mutation result was not verified"}
	item.Diagnostics = []Diagnostic{{Code: "verification-missing", Message: "mutation result was not verified", Path: item.LivePath}}
	return NormalizeItemRecord(item)
}

func (s *Store) customFilesBackupHook(runID string, createdAt time.Time, plan *customfiles.Plan) customfiles.BackupHook {
	return func(req customfiles.BackupRequest) (customfiles.BackupResult, error) {
		item, err := s.WriteCustomFilesBackup(runID, createdAt, plan, req)
		if err != nil {
			return customfiles.BackupResult{}, err
		}
		return customfiles.BackupResult{ID: item.Ref, Before: req.Before.Snapshot(), TreeBefore: req.TreeBefore.Snapshot()}, nil
	}
}

func (s *Store) WriteCustomFilesBackup(runID string, createdAt time.Time, plan *customfiles.Plan, req customfiles.BackupRequest) (BackupItem, error) {
	if s == nil {
		return BackupItem{}, fmt.Errorf("ledger store is required")
	}
	if plan == nil {
		return BackupItem{}, fmt.Errorf("custom.files plan is required")
	}
	if err := ValidateStateRoot(plan.RepoRoot, s.root); err != nil {
		return BackupItem{}, err
	}
	if err := validateRunID(runID); err != nil {
		return BackupItem{}, err
	}
	stamp := formatTime(createdAt)
	if stamp == "" {
		stamp = formatTime(s.now().UTC())
	}
	key := itemKey(plan)
	ref := stateURI("backups", runID, key)
	payloadRel := ""
	var before NormalizedState
	if plan.Resource.Driver == recipe.FileTreeDriverID {
		before = fromTreeSnapshot(req.TreeBefore.Snapshot(), driverVersion(plan.Resource.Driver), filetreedriver.NormalizerID)
		if req.TreeBefore.Exists {
			payloadRel = filepath.ToSlash(filepath.Join("payloads", key, "tree"))
			if err := writeTreePayload(filepath.Join(s.root, "backups", runID, payloadRel), req.TreeBefore); err != nil {
				return BackupItem{}, err
			}
		}
	} else {
		before = fromFileSnapshot(req.Before.Snapshot(), driverVersion(plan.Resource.Driver), filedriver.NormalizerID)
		if req.Before.Exists {
			payloadRel = filepath.ToSlash(filepath.Join("payloads", key, "before"))
			if err := writeFileAtomic(filepath.Join(s.root, "backups", runID, payloadRel), req.Before.Bytes, 0o600); err != nil {
				return BackupItem{}, fmt.Errorf("write file backup payload: %w", err)
			}
		}
	}
	item := NormalizeBackupItem(BackupItem{
		Ref:            ref,
		TargetRef:      plan.Setting.TargetID,
		SettingRef:     plan.Setting.Ref(),
		ResourceID:     plan.ResourceID,
		Driver:         plan.Resource.Driver,
		DriverVersion:  driverVersion(plan.Resource.Driver),
		LivePath:       req.Path,
		PayloadRelPath: payloadRel,
		CreatedAt:      stamp,
		Before:         before,
		Restore: RestoreCompatibility{
			Compatible:    true,
			Driver:        plan.Resource.Driver,
			DriverVersion: driverVersion(plan.Resource.Driver),
			Normalizer:    normalizerForDriver(plan.Resource.Driver),
			Message:       "Restore payload compatibility is recorded; full restore execution is handled by the restore flow.",
		},
	})
	if err := s.upsertBackupItem(runID, stamp, item); err != nil {
		return BackupItem{}, err
	}
	return item, nil
}

func (s *Store) ReadBackup(runID string) (BackupMetadata, error) {
	if err := validateRunID(runID); err != nil {
		return BackupMetadata{}, err
	}
	return readBackupMetadata(filepath.Join(s.root, "backups", runID, "backup.yaml"))
}

func (s *Store) upsertBackupItem(runID string, createdAt string, item BackupItem) error {
	path := filepath.Join(s.root, "backups", runID, "backup.yaml")
	metadata, err := readBackupMetadata(path)
	if os.IsNotExist(err) {
		metadata = BackupMetadata{Schema: BackupMetadataSchema, SchemaVersion: SchemaVersion, RunID: runID, CreatedAt: createdAt}
	} else if err != nil {
		return err
	}
	metadata.RunID = runID
	if strings.TrimSpace(metadata.CreatedAt) == "" {
		metadata.CreatedAt = createdAt
	}
	updated := false
	for i := range metadata.Items {
		if metadata.Items[i].Ref == item.Ref {
			metadata.Items[i] = item
			updated = true
			break
		}
	}
	if !updated {
		metadata.Items = append(metadata.Items, item)
	}
	return writeBackupMetadata(path, metadata)
}

func writeTreePayload(root string, state filetreedriver.State) error {
	if err := os.RemoveAll(root); err != nil {
		return fmt.Errorf("clear tree backup payload: %w", err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return fmt.Errorf("create tree backup payload: %w", err)
	}
	entries := append([]filetreedriver.Entry(nil), state.Entries...)
	for _, entry := range entries {
		rel, err := filedriver.ValidateRelativePath(entry.Path)
		if err != nil {
			return fmt.Errorf("backup tree entry %q: %w", entry.Path, err)
		}
		path := filepath.Join(root, filepath.FromSlash(rel))
		switch entry.Kind {
		case filetreedriver.EntryDir:
			if err := os.MkdirAll(path, 0o700); err != nil {
				return fmt.Errorf("write tree backup dir %s: %w", rel, err)
			}
		case filetreedriver.EntryFile:
			if err := writeFileAtomic(path, entry.Bytes, 0o600); err != nil {
				return fmt.Errorf("write tree backup file %s: %w", rel, err)
			}
		default:
			return fmt.Errorf("unsupported backup tree entry kind %s for %s", entry.Kind, rel)
		}
	}
	return nil
}

func fileStatesForRecord(plan *customfiles.Plan, result *customfiles.Result) (NormalizedState, NormalizedState) {
	driverVersion := driverVersion(plan.Resource.Driver)
	if result != nil {
		return fromFileSnapshot(result.Preview.Change.Before, driverVersion, result.Preview.Normalizer), fromFileSnapshot(result.Preview.Change.After, driverVersion, result.Preview.Normalizer)
	}
	if plan.Operation == customfiles.OperationApply {
		return fromFileSnapshot(plan.Preview.Change.Before, driverVersion, plan.Preview.Normalizer), fromFileSnapshot(plan.Preview.Change.After, driverVersion, plan.Preview.Normalizer)
	}
	return fromFileSnapshot(plan.Preview.Change.Before, driverVersion, plan.Preview.Normalizer), fromFileSnapshot(plan.Preview.Change.After, driverVersion, plan.Preview.Normalizer)
}

func treeStatesForRecord(plan *customfiles.Plan, result *customfiles.Result) (NormalizedState, NormalizedState) {
	driverVersion := driverVersion(plan.Resource.Driver)
	if result != nil {
		return fromTreeSnapshot(result.TreePreview.Change.Before, driverVersion, result.TreePreview.Normalizer), fromTreeSnapshot(result.TreePreview.Change.After, driverVersion, result.TreePreview.Normalizer)
	}
	return fromTreeSnapshot(plan.TreePreview.Change.Before, driverVersion, plan.TreePreview.Normalizer), fromTreeSnapshot(plan.TreePreview.Change.After, driverVersion, plan.TreePreview.Normalizer)
}

func fromFileSnapshot(snapshot filedriver.Snapshot, driverVersion string, normalizer string) NormalizedState {
	return NormalizedState{Exists: snapshot.Exists, Hash: snapshot.SHA256, Normalizer: normalizer, DriverVersion: driverVersion, Size: snapshot.Size}
}

func fromTreeSnapshot(snapshot filetreedriver.Snapshot, driverVersion string, normalizer string) NormalizedState {
	return NormalizedState{Exists: snapshot.Exists, Hash: snapshot.SHA256, Normalizer: normalizer, DriverVersion: driverVersion, EntryCount: snapshot.EntryCount, FileCount: snapshot.FileCount, DirCount: snapshot.DirCount}
}

func resolveFilePaths(plan *customfiles.Plan) (string, string) {
	live, liveErr := filedriver.ResolveTarget(plan.LiveTarget)
	desired, desiredErr := filedriver.ResolveTarget(plan.DesiredTarget)
	livePath, desiredPath := "", ""
	if liveErr == nil {
		livePath = live.AbsPath
	}
	if desiredErr == nil {
		desiredPath = desired.AbsPath
	}
	return livePath, desiredPath
}

func resolveTreePaths(plan *customfiles.Plan) (string, string) {
	live, liveErr := filetreedriver.ResolveTarget(plan.TreeLiveTarget)
	desired, desiredErr := filetreedriver.ResolveTarget(plan.TreeDesiredTarget)
	livePath, desiredPath := "", ""
	if liveErr == nil {
		livePath = live.AbsPath
	}
	if desiredErr == nil {
		desiredPath = desired.AbsPath
	}
	return livePath, desiredPath
}

func driverVersion(driver string) string {
	switch driver {
	case recipe.FileTreeDriverID:
		return FileTreeDriverVersion
	default:
		return FileDriverVersion
	}
}

func normalizerForDriver(driver string) string {
	switch driver {
	case recipe.FileTreeDriverID:
		return filetreedriver.NormalizerID
	default:
		return filedriver.NormalizerID
	}
}

func itemKey(plan *customfiles.Plan) string {
	base := plan.Setting.Ref() + "-" + plan.ResourceID
	base = strings.Trim(itemKeyRegexp.ReplaceAllString(base, "_"), "_")
	if base == "" {
		base = "item"
	}
	if !safePathIDPattern.MatchString(base) {
		base = "item-" + base
	}
	return base
}

func backupPayloadURI(backupRef string) string {
	if strings.TrimSpace(backupRef) == "" {
		return ""
	}
	return strings.TrimRight(backupRef, "/") + "/payload"
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
