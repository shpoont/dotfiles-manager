package ledger

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shpoont/dotfiles-manager/internal/v2/customfiles"
	"github.com/shpoont/dotfiles-manager/internal/v2/filedriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/filetreedriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/preview"
	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/shpoont/dotfiles-manager/internal/v2/resolution"
	"github.com/shpoont/dotfiles-manager/internal/v2/status"
)

const RestoreCommand = "restore"

type CustomFilesRestoreOptions struct {
	SourceRunID   string
	RunID         string
	Profile       *resolution.ResolvedProfile
	Recipe        *recipe.Recipe
	LocationRoots map[string]string
	ProfileStack  []string
	DryRun        bool
	Confirmed     bool
	StartedAt     time.Time
}

type CustomFilesRestoreRun struct {
	SourceBackup        BackupMetadata
	Preview             preview.Envelope
	RunRecord           *RunRecord
	LedgerEntries       []LedgerEntry
	BackupBeforeRestore *BackupMetadata
}

type restoreItemPlan struct {
	plan             *customfiles.Plan
	source           BackupItem
	fileState        filedriver.State
	treeState        filetreedriver.State
	current          NormalizedState
	restore          NormalizedState
	livePath         string
	desiredPath      string
	sourcePayloadRef string
	previewItem      preview.Item
}

func (s *Store) RestoreCustomFiles(opts CustomFilesRestoreOptions) (*CustomFilesRestoreRun, error) {
	if s == nil {
		return nil, fmt.Errorf("ledger store is required")
	}
	if opts.Profile == nil {
		return nil, fmt.Errorf("resolved profile is required")
	}
	if opts.Recipe == nil {
		return nil, fmt.Errorf("custom.files recipe is required")
	}
	if err := ValidateStateRoot(opts.Profile.RepoRoot, s.root); err != nil {
		return nil, err
	}
	if err := validateRunID(opts.SourceRunID); err != nil {
		return nil, fmt.Errorf("source run ID: %w", err)
	}
	if err := validateRunID(opts.RunID); err != nil {
		return nil, fmt.Errorf("restore run ID: %w", err)
	}
	if strings.TrimSpace(opts.SourceRunID) == strings.TrimSpace(opts.RunID) {
		return nil, fmt.Errorf("restore run ID must differ from source backup run ID: %s", opts.RunID)
	}

	started := opts.StartedAt
	if started.IsZero() {
		started = s.now().UTC()
	}
	profileStack := append([]string(nil), opts.ProfileStack...)
	if len(profileStack) == 0 {
		profileStack = append([]string(nil), opts.Profile.Layers...)
	}

	sourceBackup, itemPlans, envelope, err := s.planCustomFilesRestore(opts, profileStack)
	if err != nil {
		return nil, err
	}
	run := &CustomFilesRestoreRun{SourceBackup: sourceBackup, Preview: envelope}
	if opts.DryRun {
		return run, nil
	}
	if !opts.Confirmed {
		blocked := addRestoreConfirmationDiagnostic(envelope, opts.SourceRunID)
		run.Preview = blocked
		return run, fmt.Errorf("restore confirmation required for source run %s", opts.SourceRunID)
	}

	record := RunRecord{
		Schema:        RunRecordSchema,
		SchemaVersion: SchemaVersion,
		RunID:         opts.RunID,
		StartedAt:     formatTime(started),
		Command:       RestoreCommand,
		ProfileStack:  profileStack,
	}
	var restoreErrs []error
	for _, item := range itemPlans {
		recordItem, err := s.executeRestoreItem(opts.RunID, started, item)
		record.Items = append(record.Items, recordItem)
		if err != nil {
			restoreErrs = append(restoreErrs, err)
		}
	}
	record.FinishedAt = formatTime(s.now().UTC())
	record = NormalizeRunRecord(record)
	entries, commitErr := s.CommitRun(record)
	if commitErr != nil {
		return run, fmt.Errorf("commit restore run %s: %w", opts.RunID, commitErr)
	}
	run.RunRecord = &record
	run.LedgerEntries = entries
	if backup, readErr := s.ReadBackup(opts.RunID); readErr == nil {
		run.BackupBeforeRestore = &backup
	} else if !os.IsNotExist(readErr) {
		return run, fmt.Errorf("read backup-before-restore metadata for run %s: %w", opts.RunID, readErr)
	}
	if len(restoreErrs) > 0 {
		return run, errors.Join(restoreErrs...)
	}
	return run, nil
}

func (s *Store) planCustomFilesRestore(opts CustomFilesRestoreOptions, profileStack []string) (BackupMetadata, []restoreItemPlan, preview.Envelope, error) {
	sourceBackup, err := s.ReadBackup(opts.SourceRunID)
	if err != nil {
		return BackupMetadata{}, nil, preview.Envelope{}, fmt.Errorf("locate restore backup %s: %w", opts.SourceRunID, err)
	}
	sourceBackup = NormalizeBackupMetadata(sourceBackup)
	if len(sourceBackup.Items) == 0 {
		return BackupMetadata{}, nil, preview.Envelope{}, fmt.Errorf("restore backup %s has no restorable items", opts.SourceRunID)
	}

	items := make([]restoreItemPlan, 0, len(sourceBackup.Items))
	previewItems := make([]preview.Item, 0, len(sourceBackup.Items))
	for _, item := range sourceBackup.Items {
		planned, err := s.planRestoreItem(opts, item)
		if err != nil {
			return BackupMetadata{}, nil, preview.Envelope{}, err
		}
		items = append(items, planned)
		previewItems = append(previewItems, planned.previewItem)
	}
	envelope := preview.BuildEnvelope(preview.EnvelopeOptions{
		Command:      preview.CommandRestore,
		RunID:        opts.RunID,
		ProfileStack: profileStack,
		LedgerRef:    stateURI("ledger", "ledger.jsonl"),
		Items:        previewItems,
	})
	return sourceBackup, items, envelope, nil
}

func (s *Store) planRestoreItem(opts CustomFilesRestoreOptions, item BackupItem) (restoreItemPlan, error) {
	item = NormalizeBackupItem(item)
	if !item.Restore.Compatible {
		return restoreItemPlan{}, fmt.Errorf("restore backup item %s is incompatible: %s", item.Ref, item.Restore.Message)
	}
	if item.Driver != recipe.FileDriverID && item.Driver != recipe.FileTreeDriverID {
		return restoreItemPlan{}, fmt.Errorf("restore backup item %s uses unsupported driver %s", item.Ref, item.Driver)
	}
	plan, err := customfiles.PlanApply(customfiles.Request{
		Profile:       opts.Profile,
		Recipe:        opts.Recipe,
		SettingRef:    item.SettingRef,
		LocationRoots: opts.LocationRoots,
	})
	if err != nil {
		return restoreItemPlan{}, fmt.Errorf("resolve restore target %s: %w", item.SettingRef, err)
	}
	if plan.ResourceID != item.ResourceID {
		return restoreItemPlan{}, fmt.Errorf("restore backup item %s resource mismatch: backup=%s current=%s", item.Ref, item.ResourceID, plan.ResourceID)
	}
	if plan.Resource.Driver != item.Driver {
		return restoreItemPlan{}, fmt.Errorf("restore backup item %s driver mismatch: backup=%s current=%s", item.Ref, item.Driver, plan.Resource.Driver)
	}

	planned := restoreItemPlan{plan: plan, source: item}
	switch item.Driver {
	case recipe.FileTreeDriverID:
		state, err := s.loadTreeRestoreState(opts.SourceRunID, item)
		if err != nil {
			return restoreItemPlan{}, err
		}
		planned.treeState = state
		planned.current = fromTreeSnapshot(plan.TreeDestinationState.Snapshot(), driverVersion(item.Driver), filetreedriver.NormalizerID)
		planned.restore = fromTreeSnapshot(state.Snapshot(), driverVersion(item.Driver), filetreedriver.NormalizerID)
		live, desired := resolveTreePaths(plan)
		planned.livePath, planned.desiredPath = live, desired
		driver := filetreedriver.Driver{}
		restorePreview, err := driver.PreviewApply(plan.TreeLiveTarget, state)
		if err != nil {
			return restoreItemPlan{}, fmt.Errorf("preview restore %s: %w", item.SettingRef, err)
		}
		planned.previewItem = restorePreviewItem(plan, item, live, desired, treePreviewChange(restorePreview.Change), opts.DryRun)
	default:
		state, err := s.loadFileRestoreState(opts.SourceRunID, item)
		if err != nil {
			return restoreItemPlan{}, err
		}
		planned.fileState = state
		planned.current = fromFileSnapshot(plan.DestinationState.Snapshot(), driverVersion(item.Driver), filedriver.NormalizerID)
		planned.restore = fromFileSnapshot(state.Snapshot(), driverVersion(item.Driver), filedriver.NormalizerID)
		live, desired := resolveFilePaths(plan)
		planned.livePath, planned.desiredPath = live, desired
		driver := filedriver.Driver{}
		restorePreview, err := driver.PreviewApply(plan.LiveTarget, state)
		if err != nil {
			return restoreItemPlan{}, fmt.Errorf("preview restore %s: %w", item.SettingRef, err)
		}
		planned.previewItem = restorePreviewItem(plan, item, live, desired, filePreviewChange(restorePreview.Change), opts.DryRun)
	}
	if item.PayloadRelPath != "" {
		planned.sourcePayloadRef = item.Ref + "/payload"
	}
	return planned, nil
}

func (s *Store) executeRestoreItem(runID string, started time.Time, item restoreItemPlan) (ItemRecord, error) {
	switch item.source.Driver {
	case recipe.FileTreeDriverID:
		return s.executeTreeRestoreItem(runID, started, item)
	default:
		return s.executeFileRestoreItem(runID, started, item)
	}
}

func (s *Store) executeFileRestoreItem(runID string, started time.Time, item restoreItemPlan) (ItemRecord, error) {
	driver := filedriver.Driver{}
	current, err := driver.ReadCurrent(item.plan.LiveTarget)
	if err != nil {
		restoreErr := fmt.Errorf("read current before restore %s: %w", item.source.SettingRef, err)
		return failedRestoreItemRecord(item, nil, restoreErr, runID), restoreErr
	}
	currentRecord := fromFileSnapshot(current.Snapshot(), driverVersion(item.source.Driver), filedriver.NormalizerID)
	var backupRef string
	if driver.Diff(current, item.fileState).Kind != filedriver.ChangeUnchanged {
		backup, err := s.WriteCustomFilesBackup(runID, started, item.plan, customfiles.BackupRequest{
			Operation:  customfiles.Operation(RestoreCommand),
			SettingRef: item.source.SettingRef,
			ResourceID: item.source.ResourceID,
			Path:       item.livePath,
			Before:     current,
		})
		if err != nil {
			restoreErr := fmt.Errorf("backup before restore %s: %w", item.source.SettingRef, err)
			return failedRestoreItemRecord(item, &currentRecord, restoreErr, runID), restoreErr
		}
		backupRef = backup.Ref
		if _, err := driver.Apply(item.plan.LiveTarget, item.fileState); err != nil {
			restoreErr := fmt.Errorf("apply restore %s: %w", item.source.SettingRef, err)
			record := failedRestoreItemRecord(item, &currentRecord, restoreErr, runID)
			attachRestoreBackup(&record, backupRef)
			return record, restoreErr
		}
	}
	if _, err := driver.Verify(item.plan.LiveTarget, item.fileState); err != nil {
		restoreErr := fmt.Errorf("verify restore %s: %w", item.source.SettingRef, err)
		record := failedRestoreItemRecord(item, &currentRecord, restoreErr, runID)
		attachRestoreBackup(&record, backupRef)
		return record, restoreErr
	}
	record := verifiedRestoreItemRecord(item, currentRecord, runID)
	attachRestoreBackup(&record, backupRef)
	return record, nil
}

func (s *Store) executeTreeRestoreItem(runID string, started time.Time, item restoreItemPlan) (ItemRecord, error) {
	driver := filetreedriver.Driver{}
	current, err := driver.ReadCurrent(item.plan.TreeLiveTarget)
	if err != nil {
		restoreErr := fmt.Errorf("read current before restore %s: %w", item.source.SettingRef, err)
		return failedRestoreItemRecord(item, nil, restoreErr, runID), restoreErr
	}
	currentRecord := fromTreeSnapshot(current.Snapshot(), driverVersion(item.source.Driver), filetreedriver.NormalizerID)
	var backupRef string
	if driver.Diff(current, item.treeState).Kind != filedriver.ChangeUnchanged {
		backup, err := s.WriteCustomFilesBackup(runID, started, item.plan, customfiles.BackupRequest{
			Operation:  customfiles.Operation(RestoreCommand),
			SettingRef: item.source.SettingRef,
			ResourceID: item.source.ResourceID,
			Path:       item.livePath,
			TreeBefore: current,
		})
		if err != nil {
			restoreErr := fmt.Errorf("backup before restore %s: %w", item.source.SettingRef, err)
			return failedRestoreItemRecord(item, &currentRecord, restoreErr, runID), restoreErr
		}
		backupRef = backup.Ref
		if _, err := driver.Apply(item.plan.TreeLiveTarget, item.treeState); err != nil {
			restoreErr := fmt.Errorf("apply restore %s: %w", item.source.SettingRef, err)
			record := failedRestoreItemRecord(item, &currentRecord, restoreErr, runID)
			attachRestoreBackup(&record, backupRef)
			return record, restoreErr
		}
	}
	if _, err := driver.Verify(item.plan.TreeLiveTarget, item.treeState); err != nil {
		restoreErr := fmt.Errorf("verify restore %s: %w", item.source.SettingRef, err)
		record := failedRestoreItemRecord(item, &currentRecord, restoreErr, runID)
		attachRestoreBackup(&record, backupRef)
		return record, restoreErr
	}
	record := verifiedRestoreItemRecord(item, currentRecord, runID)
	attachRestoreBackup(&record, backupRef)
	return record, nil
}

func verifiedRestoreItemRecord(item restoreItemPlan, current NormalizedState, runID string) ItemRecord {
	record := baseRestoreItemRecord(item, current, runID)
	record.VerifiedState = item.restore
	record.Verification = Verification{Verified: true, Result: "verified"}
	record.Result = ItemResultVerified
	return NormalizeItemRecord(record)
}

func failedRestoreItemRecord(item restoreItemPlan, current *NormalizedState, restoreErr error, runID string) ItemRecord {
	before := item.current
	if current != nil {
		before = *current
	}
	record := baseRestoreItemRecord(item, before, runID)
	message := ""
	if restoreErr != nil {
		message = restoreErr.Error()
	}
	record.Verification = Verification{Verified: false, Result: "failed", Message: message}
	record.Result = ItemResultFailed
	record.Diagnostics = []Diagnostic{{Code: "restore-failed", Message: message, Path: item.livePath}}
	return NormalizeItemRecord(record)
}

func baseRestoreItemRecord(item restoreItemPlan, before NormalizedState, runID string) ItemRecord {
	record := ItemRecord{
		TargetRef:        item.source.TargetRef,
		SettingRef:       item.source.SettingRef,
		Operation:        RestoreCommand,
		ResourceID:       item.source.ResourceID,
		Driver:           item.source.Driver,
		DriverVersion:    driverVersion(item.source.Driver),
		DesiredURI:       item.plan.Setting.DesiredURI,
		DesiredRelPath:   item.plan.DesiredRelPath,
		LivePath:         item.livePath,
		DesiredPath:      item.desiredPath,
		Before:           before,
		Desired:          item.restore,
		SourceBackupRefs: []string{item.source.Ref},
		ArtifactRefs: ArtifactRefs{
			Desired:             item.plan.Setting.DesiredURI,
			DesiredURI:          item.plan.Setting.DesiredURI,
			DesiredPath:         item.desiredPath,
			LivePath:            item.livePath,
			SourceBackup:        item.source.Ref,
			SourceBackupPayload: item.sourcePayloadRef,
			RunRecord:           stateURI("ledger", "runs", runID),
			Ledger:              stateURI("ledger", "ledger.jsonl"),
		},
	}
	return NormalizeItemRecord(record)
}

func attachRestoreBackup(record *ItemRecord, backupRef string) {
	if record == nil || strings.TrimSpace(backupRef) == "" {
		return
	}
	record.BackupRefs = []string{backupRef}
	record.ArtifactRefs.Backup = backupRef
	record.ArtifactRefs.BackupPayload = backupPayloadURI(backupRef)
	*record = NormalizeItemRecord(*record)
}

func restorePreviewItem(plan *customfiles.Plan, source BackupItem, livePath string, desiredPath string, change preview.Change, dryRun bool) preview.Item {
	result := preview.ResultWouldChange
	stateCode := status.StateReadyToApply
	message := fmt.Sprintf("Restore would replace live state with backup %s from run metadata.", source.Ref)
	if change.Kind == filedriver.ChangeUnchanged {
		result = preview.ResultUnchanged
		stateCode = status.StateUnchanged
		message = fmt.Sprintf("Live state already matches backup %s.", source.Ref)
	}
	backup := preview.Backup{
		Policy:  preview.BackupRequired,
		Message: fmt.Sprintf("Restore source: %s. A backup-before-restore will be created before any live write.", source.Ref),
	}
	if change.Kind == filedriver.ChangeUnchanged {
		backup = preview.Backup{
			Policy:  preview.BackupNotApplicable,
			Message: fmt.Sprintf("Restore source: %s. No backup-before-restore is needed because no live write is planned.", source.Ref),
		}
	}
	return preview.NormalizeItem(preview.Item{
		TargetRef:      source.TargetRef,
		SettingRef:     source.SettingRef,
		Scope:          plan.Setting.Scope,
		Subject:        plan.Setting.Subject,
		DesiredURI:     plan.Setting.DesiredURI,
		DesiredRelPath: plan.DesiredRelPath,
		Operation:      RestoreCommand,
		Driver:         source.Driver,
		ResourceID:     source.ResourceID,
		LivePath:       livePath,
		DesiredPath:    desiredPath,
		DryRun:         dryRun,
		State:          stateCode,
		Message:        message,
		Change:         change,
		Backup:         backup,
		Result:         result,
		Actions:        []status.Action{status.ActionDiff, status.ActionApply},
		AutomaticMerge: false,
	})
}

func addRestoreConfirmationDiagnostic(envelope preview.Envelope, sourceRunID string) preview.Envelope {
	envelope = preview.NormalizeEnvelope(envelope)
	for i := range envelope.Items {
		envelope.Items[i].Result = preview.ResultBlocked
		envelope.Items[i].Diagnostics = append(envelope.Items[i].Diagnostics, preview.Diagnostic{
			Code:     "restore-confirmation-required",
			Severity: preview.SeverityError,
			Message:  fmt.Sprintf("Restore from run %s requires explicit confirmation before live writes.", sourceRunID),
			ExitCode: preview.ExitInputRequired,
		})
		envelope.Items[i] = preview.NormalizeItem(envelope.Items[i])
	}
	return preview.NormalizeEnvelope(envelope)
}

func (s *Store) loadFileRestoreState(runID string, item BackupItem) (filedriver.State, error) {
	if !item.Before.Exists {
		if strings.TrimSpace(item.PayloadRelPath) != "" {
			return filedriver.State{}, fmt.Errorf("restore backup item %s is incompatible: absent file backup must not declare payload %s", item.Ref, item.PayloadRelPath)
		}
		return filedriver.AbsentState(), nil
	}
	payloadPath, err := s.backupPayloadPath(runID, item.PayloadRelPath)
	if err != nil {
		return filedriver.State{}, fmt.Errorf("restore backup item %s payload: %w", item.Ref, err)
	}
	data, err := os.ReadFile(payloadPath)
	if err != nil {
		return filedriver.State{}, fmt.Errorf("read restore payload %s: %w", item.PayloadRelPath, err)
	}
	state := filedriver.Driver{}.Normalize(data)
	if err := requireMatchingFileBackupState(item, state); err != nil {
		return filedriver.State{}, err
	}
	return state, nil
}

func (s *Store) loadTreeRestoreState(runID string, item BackupItem) (filetreedriver.State, error) {
	if !item.Before.Exists {
		if strings.TrimSpace(item.PayloadRelPath) != "" {
			return filetreedriver.State{}, fmt.Errorf("restore backup item %s is incompatible: absent tree backup must not declare payload %s", item.Ref, item.PayloadRelPath)
		}
		return filetreedriver.AbsentState(), nil
	}
	payloadPath, err := s.backupPayloadPath(runID, item.PayloadRelPath)
	if err != nil {
		return filetreedriver.State{}, fmt.Errorf("restore backup item %s payload: %w", item.Ref, err)
	}
	if info, err := os.Stat(payloadPath); err != nil {
		return filetreedriver.State{}, fmt.Errorf("read restore tree payload %s: %w", item.PayloadRelPath, err)
	} else if !info.IsDir() {
		return filetreedriver.State{}, fmt.Errorf("restore tree payload is not a directory: %s", item.PayloadRelPath)
	}
	state, err := filetreedriver.Driver{}.ReadCurrent(filetreedriver.Target{
		LocationID:        "backup",
		Root:              filepath.Join(s.root, "backups", runID),
		RelPath:           item.PayloadRelPath,
		Include:           []string{"**"},
		RejectRootSymlink: true,
	})
	if err != nil {
		return filetreedriver.State{}, fmt.Errorf("read restore tree payload %s: %w", item.PayloadRelPath, err)
	}
	if err := requireMatchingTreeBackupState(item, state); err != nil {
		return filetreedriver.State{}, err
	}
	return state, nil
}

func (s *Store) backupPayloadPath(runID string, relPath string) (string, error) {
	if err := validateRunID(runID); err != nil {
		return "", err
	}
	rel, err := filedriver.ValidateRelativePath(filepath.ToSlash(strings.TrimSpace(relPath)))
	if err != nil {
		return "", err
	}
	root := filepath.Join(s.root, "backups", runID)
	path := filepath.Join(root, filepath.FromSlash(rel))
	if !sameOrInside(root, path) {
		return "", fmt.Errorf("backup payload path escapes backup root: %s", relPath)
	}
	return path, nil
}

func requireMatchingFileBackupState(item BackupItem, state filedriver.State) error {
	if item.Before.Exists != state.Exists {
		return fmt.Errorf("restore backup item %s payload existence mismatch", item.Ref)
	}
	if item.Before.Hash != "" && item.Before.Hash != state.SHA256 {
		return fmt.Errorf("restore backup item %s payload hash mismatch", item.Ref)
	}
	return nil
}

func requireMatchingTreeBackupState(item BackupItem, state filetreedriver.State) error {
	if item.Before.Exists != state.Exists {
		return fmt.Errorf("restore backup item %s payload existence mismatch", item.Ref)
	}
	if item.Before.Hash != "" && item.Before.Hash != state.SHA256 {
		return fmt.Errorf("restore backup item %s payload hash mismatch", item.Ref)
	}
	return nil
}

func filePreviewChange(diff filedriver.Diff) preview.Change {
	return preview.Change{Kind: diff.Kind, Before: fileSnapshot(diff.Before), After: fileSnapshot(diff.After)}
}

func treePreviewChange(diff filetreedriver.Diff) preview.Change {
	entries := make([]preview.EntryChange, 0, len(diff.Entries))
	for _, entry := range diff.Entries {
		entries = append(entries, preview.EntryChange{Path: entry.Path, Kind: entry.Kind, Before: entrySnapshot(entry.Before), After: entrySnapshot(entry.After)})
	}
	return preview.Change{Kind: diff.Kind, Before: treeSnapshot(diff.Before), After: treeSnapshot(diff.After), Entries: entries}
}

func fileSnapshot(snapshot filedriver.Snapshot) preview.Snapshot {
	return preview.Snapshot{Exists: snapshot.Exists, Size: snapshot.Size, SHA256: snapshot.SHA256}
}

func treeSnapshot(snapshot filetreedriver.Snapshot) preview.Snapshot {
	return preview.Snapshot{Exists: snapshot.Exists, SHA256: snapshot.SHA256, EntryCount: snapshot.EntryCount, FileCount: snapshot.FileCount, DirCount: snapshot.DirCount}
}

func entrySnapshot(snapshot filetreedriver.EntrySnapshot) preview.Snapshot {
	return preview.Snapshot{Exists: snapshot.Exists, Size: snapshot.Size, SHA256: snapshot.SHA256}
}
