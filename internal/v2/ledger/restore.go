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

type RestoreOptions struct {
	SourceRunID    string
	RunID          string
	Profile        *resolution.ResolvedProfile
	LocationRoots  map[string]map[string]string
	ProfileStack   []string
	DryRun         bool
	Confirmed      bool
	NonInteractive bool
	StartedAt      time.Time
}

type RestoreRun struct {
	SourceBackup        BackupMetadata
	Preview             preview.Envelope
	RunRecord           *RunRecord
	LedgerEntries       []LedgerEntry
	BackupBeforeRestore *BackupMetadata
}

type restoreItemPlan struct {
	plan              *customfiles.Plan
	source            BackupItem
	fileState         filedriver.State
	treeState         filetreedriver.State
	current           NormalizedState
	restore           NormalizedState
	livePath          string
	desiredPath       string
	desiredURI        string
	desiredRelPath    string
	scope             string
	subject           string
	liveTarget        filedriver.Target
	treeLiveTarget    filetreedriver.Target
	selectedWholeFile bool
	sourcePayloadRef  string
	previewItem       preview.Item
}

type restoreExecutedItem struct {
	item        restoreItemPlan
	recordIndex int
	backupRef   string
	rollback    bool
}

type restoreRollbackItem struct {
	recordIndex int
	backupRef   string
	err         error
}

type restoreRollbackResult struct {
	Attempted bool
	Items     []restoreRollbackItem
	ReadError error
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
	var executed []restoreExecutedItem
	for _, item := range itemPlans {
		recordItem, executedItem, err := s.executeRestoreItemWithRollback(opts.RunID, started, item)
		record.Items = append(record.Items, recordItem)
		executedItem.recordIndex = len(record.Items) - 1
		if executedItem.rollback {
			executed = append(executed, executedItem)
		}
		if err != nil {
			restoreErrs = append(restoreErrs, err)
			rollback := s.rollbackRestoreExecution(opts.RunID, executed)
			record = annotateRestoreRunRollback(record, len(record.Items)-1, rollback, err)
			run.Preview = annotateRestorePreviewRollback(envelope, len(record.Items)-1, rollback, err)
			if rollbackErr := rollback.Err(); rollbackErr != nil {
				restoreErrs = append(restoreErrs, rollbackErr)
			}
			break
		}
	}
	record.FinishedAt = formatTime(s.now().UTC())
	record = NormalizeRunRecord(record)
	entries, commitErr := s.CommitRun(record)
	if commitErr != nil {
		rollback := s.rollbackRestoreExecution(opts.RunID, executed)
		run.Preview = annotateRestorePreviewRollback(envelope, len(record.Items)-1, rollback, commitErr)
		if rollbackErr := rollback.Err(); rollbackErr != nil {
			return run, fmt.Errorf("commit restore run %s after rollback status %s: %w; rollback: %w", opts.RunID, rollback.Status(), commitErr, rollbackErr)
		}
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

func (s *Store) Restore(opts RestoreOptions) (*RestoreRun, error) {
	if s == nil {
		return nil, fmt.Errorf("ledger store is required")
	}
	if opts.Profile == nil {
		return nil, fmt.Errorf("resolved profile is required")
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

	sourceBackup, itemPlans, envelope, planErr := s.planRestore(opts, profileStack)
	run := &RestoreRun{SourceBackup: sourceBackup, Preview: envelope}
	if planErr != nil {
		return run, planErr
	}
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
	var executed []restoreExecutedItem
	for _, item := range itemPlans {
		recordItem, executedItem, err := s.executeRestoreItemWithRollback(opts.RunID, started, item)
		record.Items = append(record.Items, recordItem)
		executedItem.recordIndex = len(record.Items) - 1
		if executedItem.rollback {
			executed = append(executed, executedItem)
		}
		if err != nil {
			restoreErrs = append(restoreErrs, err)
			rollback := s.rollbackRestoreExecution(opts.RunID, executed)
			record = annotateRestoreRunRollback(record, len(record.Items)-1, rollback, err)
			run.Preview = annotateRestorePreviewRollback(envelope, len(record.Items)-1, rollback, err)
			if rollbackErr := rollback.Err(); rollbackErr != nil {
				restoreErrs = append(restoreErrs, rollbackErr)
			}
			break
		}
	}
	record.FinishedAt = formatTime(s.now().UTC())
	record = NormalizeRunRecord(record)
	entries, commitErr := s.CommitRun(record)
	if commitErr != nil {
		rollback := s.rollbackRestoreExecution(opts.RunID, executed)
		run.Preview = annotateRestorePreviewRollback(envelope, len(record.Items)-1, rollback, commitErr)
		if rollbackErr := rollback.Err(); rollbackErr != nil {
			return run, fmt.Errorf("commit restore run %s after rollback status %s: %w; rollback: %w", opts.RunID, rollback.Status(), commitErr, rollbackErr)
		}
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

func (s *Store) planRestore(opts RestoreOptions, profileStack []string) (BackupMetadata, []restoreItemPlan, preview.Envelope, error) {
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
	var blockers []error
	for _, item := range sourceBackup.Items {
		planned, err := s.planRestoreItemAny(opts, item)
		if err != nil {
			blockers = append(blockers, err)
			previewItems = append(previewItems, blockedRestorePreviewItem(item, err, opts.DryRun))
			continue
		}
		items = append(items, planned)
		previewItems = append(previewItems, planned.previewItem)
	}
	if len(blockers) > 0 {
		previewItems = blockAllRestorePreviewItems(previewItems, opts.SourceRunID)
	}
	envelope := preview.BuildEnvelope(preview.EnvelopeOptions{
		Command:      preview.CommandRestore,
		RunID:        opts.RunID,
		ProfileStack: profileStack,
		LedgerRef:    stateURI("ledger", "ledger.jsonl"),
		Items:        previewItems,
	})
	if len(blockers) > 0 {
		return sourceBackup, nil, envelope, fmt.Errorf("restore run %s is blocked before writes: %w", opts.SourceRunID, errors.Join(blockers...))
	}
	return sourceBackup, items, envelope, nil
}

func (s *Store) planRestoreItemAny(opts RestoreOptions, item BackupItem) (restoreItemPlan, error) {
	item = NormalizeBackupItem(item)
	if !item.Restore.Compatible {
		return restoreItemPlan{}, fmt.Errorf("restore backup item %s is incompatible: %s", item.Ref, item.Restore.Message)
	}
	switch item.Driver {
	case recipe.FileDriverID, recipe.FileTreeDriverID:
		return s.planFileResourceRestoreItem(opts, item)
	case recipe.IniFileDriverID, recipe.JSONFileDriverID, recipe.YAMLFileDriverID, recipe.TOMLFileDriverID, recipe.PlistFileDriverID:
		return s.planSelectedValueRestoreItem(opts, item)
	case recipe.NativeExportDriverID:
		return restoreItemPlan{}, fmt.Errorf("restore backup item %s uses unsupported native opaque driver %s: %s", item.Ref, item.Driver, item.Restore.Message)
	default:
		return restoreItemPlan{}, fmt.Errorf("restore backup item %s uses unsupported driver %s", item.Ref, item.Driver)
	}
}

func (s *Store) planFileResourceRestoreItem(opts RestoreOptions, item BackupItem) (restoreItemPlan, error) {
	rec, setting, err := runtimeForRestoreItem(opts.Profile, item)
	if err != nil {
		return restoreItemPlan{}, err
	}
	roots := targetLocationRoots(opts.LocationRoots, item.TargetRef)
	plan, err := customfiles.PlanFileRead(customfiles.Request{
		Profile:       opts.Profile,
		Recipe:        rec,
		SettingRef:    item.SettingRef,
		LocationRoots: roots,
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

	planned := restoreItemPlan{plan: plan, source: item, scope: setting.Scope, subject: setting.Subject, desiredURI: setting.DesiredURI, desiredRelPath: setting.DesiredRelPath}
	switch item.Driver {
	case recipe.FileTreeDriverID:
		state, err := s.loadTreeRestoreState(opts.SourceRunID, item)
		if err != nil {
			return restoreItemPlan{}, err
		}
		planned.treeState = state
		planned.current = fromTreeSnapshot(plan.TreeSourceState.Snapshot(), driverVersion(item.Driver), filetreedriver.NormalizerID)
		planned.restore = fromTreeSnapshot(state.Snapshot(), driverVersion(item.Driver), filetreedriver.NormalizerID)
		live, desired := resolveTreePaths(plan)
		planned.livePath, planned.desiredPath = live, desired
		planned.treeLiveTarget = plan.TreeLiveTarget
		driver := filetreedriver.Driver{}
		restorePreview, err := driver.PreviewApply(plan.TreeLiveTarget, state)
		if err != nil {
			return restoreItemPlan{}, fmt.Errorf("preview restore %s: %w", item.SettingRef, err)
		}
		planned.previewItem = restorePreviewItem(plan, item, setting, live, desired, setting.DesiredURI, setting.DesiredRelPath, treePreviewChange(restorePreview.Change), opts.DryRun, false)
	default:
		state, err := s.loadFileRestoreState(opts.SourceRunID, item)
		if err != nil {
			return restoreItemPlan{}, err
		}
		planned.fileState = state
		planned.current = fromFileSnapshot(plan.SourceState.Snapshot(), driverVersion(item.Driver), filedriver.NormalizerID)
		planned.restore = fromFileSnapshot(state.Snapshot(), driverVersion(item.Driver), filedriver.NormalizerID)
		live, desired := resolveFilePaths(plan)
		planned.livePath, planned.desiredPath = live, desired
		planned.liveTarget = plan.LiveTarget
		driver := filedriver.Driver{}
		restorePreview, err := driver.PreviewApply(plan.LiveTarget, state)
		if err != nil {
			return restoreItemPlan{}, fmt.Errorf("preview restore %s: %w", item.SettingRef, err)
		}
		planned.previewItem = restorePreviewItem(plan, item, setting, live, desired, setting.DesiredURI, setting.DesiredRelPath, filePreviewChange(restorePreview.Change), opts.DryRun, false)
	}
	if item.PayloadRelPath != "" {
		planned.sourcePayloadRef = item.Ref + "/payload"
	}
	return planned, nil
}

func (s *Store) planSelectedValueRestoreItem(opts RestoreOptions, item BackupItem) (restoreItemPlan, error) {
	rec, setting, err := runtimeForRestoreItem(opts.Profile, item)
	if err != nil {
		return restoreItemPlan{}, err
	}
	resourceID, resource, err := rec.ResourceForSetting(setting.SettingID)
	if err != nil {
		return restoreItemPlan{}, fmt.Errorf("resolve restore resource %s: %w", item.SettingRef, err)
	}
	if resourceID != item.ResourceID {
		return restoreItemPlan{}, fmt.Errorf("restore backup item %s resource mismatch: backup=%s current=%s", item.Ref, item.ResourceID, resourceID)
	}
	if resource.Driver != item.Driver {
		return restoreItemPlan{}, fmt.Errorf("restore backup item %s driver mismatch: backup=%s current=%s", item.Ref, item.Driver, resource.Driver)
	}
	roots := targetLocationRoots(opts.LocationRoots, item.TargetRef)
	root, err := rec.LocationRoot(resource.Location, roots)
	if err != nil {
		return restoreItemPlan{}, fmt.Errorf("resolve restore target %s: %w", item.SettingRef, err)
	}
	target := filedriver.Target{LocationID: resource.Location, Root: root, RelPath: resource.Path, AllowMissingRoot: true, RejectRootSymlink: true}
	resolved, err := filedriver.ResolveTarget(target)
	if err != nil {
		return restoreItemPlan{}, fmt.Errorf("resolve restore target %s: %w", item.SettingRef, err)
	}
	if !sameFilePath(item.LivePath, resolved.AbsPath) {
		return restoreItemPlan{}, fmt.Errorf("restore backup item %s live path mismatch: backup=%s current=%s", item.Ref, item.LivePath, resolved.AbsPath)
	}
	state, err := s.loadFileRestoreState(opts.SourceRunID, item)
	if err != nil {
		return restoreItemPlan{}, err
	}
	driver := filedriver.Driver{}
	current, err := driver.ReadCurrent(target)
	if err != nil {
		return restoreItemPlan{}, fmt.Errorf("read current before restore preview %s: %w", item.SettingRef, err)
	}
	restorePreview, err := driver.PreviewApply(target, state)
	if err != nil {
		return restoreItemPlan{}, fmt.Errorf("preview restore %s: %w", item.SettingRef, err)
	}
	planned := restoreItemPlan{
		source:            item,
		fileState:         state,
		current:           fromFileSnapshot(current.Snapshot(), driverVersion(item.Driver), filedriver.NormalizerID),
		restore:           fromFileSnapshot(state.Snapshot(), driverVersion(item.Driver), filedriver.NormalizerID),
		livePath:          resolved.AbsPath,
		desiredPath:       setting.DesiredPath,
		desiredURI:        setting.DesiredURI,
		desiredRelPath:    setting.DesiredRelPath,
		scope:             setting.Scope,
		subject:           setting.Subject,
		liveTarget:        target,
		selectedWholeFile: true,
		previewItem:       restorePreviewItem(nil, item, setting, resolved.AbsPath, setting.DesiredPath, setting.DesiredURI, setting.DesiredRelPath, filePreviewChange(restorePreview.Change), opts.DryRun, true),
	}
	if item.PayloadRelPath != "" {
		planned.sourcePayloadRef = item.Ref + "/payload"
	}
	return planned, nil
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
		planned.previewItem = restorePreviewItem(plan, item, plan.Setting, live, desired, plan.Setting.DesiredURI, plan.DesiredRelPath, treePreviewChange(restorePreview.Change), opts.DryRun, false)
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
		planned.previewItem = restorePreviewItem(plan, item, plan.Setting, live, desired, plan.Setting.DesiredURI, plan.DesiredRelPath, filePreviewChange(restorePreview.Change), opts.DryRun, false)
	}
	if item.PayloadRelPath != "" {
		planned.sourcePayloadRef = item.Ref + "/payload"
	}
	return planned, nil
}

func (s *Store) executeRestoreItemWithRollback(runID string, started time.Time, item restoreItemPlan) (ItemRecord, restoreExecutedItem, error) {
	if item.selectedWholeFile {
		return s.executeSelectedValueRestoreItemWithRollback(runID, started, item)
	}
	switch item.source.Driver {
	case recipe.FileTreeDriverID:
		return s.executeTreeRestoreItemWithRollback(runID, started, item)
	default:
		return s.executeFileRestoreItemWithRollback(runID, started, item)
	}
}

func (s *Store) executeSelectedValueRestoreItemWithRollback(runID string, started time.Time, item restoreItemPlan) (ItemRecord, restoreExecutedItem, error) {
	executed := restoreExecutedItem{item: item}
	driver := filedriver.Driver{}
	current, err := driver.ReadCurrent(item.liveTarget)
	if err != nil {
		restoreErr := fmt.Errorf("read current before restore %s: %w", item.source.SettingRef, err)
		return failedRestoreItemRecord(item, nil, restoreErr, runID), executed, restoreErr
	}
	currentRecord := fromFileSnapshot(current.Snapshot(), driverVersion(item.source.Driver), filedriver.NormalizerID)
	var backupRef string
	if driver.Diff(current, item.fileState).Kind != filedriver.ChangeUnchanged {
		beforeFile := []byte(nil)
		if current.Exists {
			beforeFile = append([]byte(nil), current.Bytes...)
		}
		backup, err := s.WriteSelectedValueBackup(runID, started, SelectedValueBackupRequest{
			TargetRef:  item.source.TargetRef,
			SettingRef: item.source.SettingRef,
			ResourceID: item.source.ResourceID,
			Driver:     item.source.Driver,
			LivePath:   item.livePath,
			Before:     currentRecord,
			BeforeFile: beforeFile,
		})
		if err != nil {
			restoreErr := fmt.Errorf("backup before restore %s: %w", item.source.SettingRef, err)
			return failedRestoreItemRecord(item, &currentRecord, restoreErr, runID), executed, restoreErr
		}
		backupRef = backup.Ref
		executed.backupRef = backup.Ref
		executed.rollback = true
		if _, err := driver.Apply(item.liveTarget, item.fileState); err != nil {
			restoreErr := fmt.Errorf("apply restore %s: %w", item.source.SettingRef, err)
			record := failedRestoreItemRecord(item, &currentRecord, restoreErr, runID)
			attachRestoreBackup(&record, backupRef)
			return record, executed, restoreErr
		}
	}
	if _, err := driver.Verify(item.liveTarget, item.fileState); err != nil {
		restoreErr := fmt.Errorf("verify restore %s: %w", item.source.SettingRef, err)
		record := failedRestoreItemRecord(item, &currentRecord, restoreErr, runID)
		attachRestoreBackup(&record, backupRef)
		return record, executed, restoreErr
	}
	record := verifiedRestoreItemRecord(item, currentRecord, runID)
	attachRestoreBackup(&record, backupRef)
	return record, executed, nil
}

func (s *Store) executeFileRestoreItem(runID string, started time.Time, item restoreItemPlan) (ItemRecord, error) {
	record, _, err := s.executeFileRestoreItemWithRollback(runID, started, item)
	return record, err
}

func (s *Store) executeFileRestoreItemWithRollback(runID string, started time.Time, item restoreItemPlan) (ItemRecord, restoreExecutedItem, error) {
	executed := restoreExecutedItem{item: item}
	driver := filedriver.Driver{}
	current, err := driver.ReadCurrent(item.plan.LiveTarget)
	if err != nil {
		restoreErr := fmt.Errorf("read current before restore %s: %w", item.source.SettingRef, err)
		return failedRestoreItemRecord(item, nil, restoreErr, runID), executed, restoreErr
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
			return failedRestoreItemRecord(item, &currentRecord, restoreErr, runID), executed, restoreErr
		}
		backupRef = backup.Ref
		executed.backupRef = backup.Ref
		executed.rollback = true
		if _, err := driver.Apply(item.plan.LiveTarget, item.fileState); err != nil {
			restoreErr := fmt.Errorf("apply restore %s: %w", item.source.SettingRef, err)
			record := failedRestoreItemRecord(item, &currentRecord, restoreErr, runID)
			attachRestoreBackup(&record, backupRef)
			return record, executed, restoreErr
		}
	}
	if _, err := driver.Verify(item.plan.LiveTarget, item.fileState); err != nil {
		restoreErr := fmt.Errorf("verify restore %s: %w", item.source.SettingRef, err)
		record := failedRestoreItemRecord(item, &currentRecord, restoreErr, runID)
		attachRestoreBackup(&record, backupRef)
		return record, executed, restoreErr
	}
	record := verifiedRestoreItemRecord(item, currentRecord, runID)
	attachRestoreBackup(&record, backupRef)
	return record, executed, nil
}

func (s *Store) executeTreeRestoreItem(runID string, started time.Time, item restoreItemPlan) (ItemRecord, error) {
	record, _, err := s.executeTreeRestoreItemWithRollback(runID, started, item)
	return record, err
}

func (s *Store) executeTreeRestoreItemWithRollback(runID string, started time.Time, item restoreItemPlan) (ItemRecord, restoreExecutedItem, error) {
	executed := restoreExecutedItem{item: item}
	driver := filetreedriver.Driver{}
	current, err := driver.ReadCurrent(item.plan.TreeLiveTarget)
	if err != nil {
		restoreErr := fmt.Errorf("read current before restore %s: %w", item.source.SettingRef, err)
		return failedRestoreItemRecord(item, nil, restoreErr, runID), executed, restoreErr
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
			return failedRestoreItemRecord(item, &currentRecord, restoreErr, runID), executed, restoreErr
		}
		backupRef = backup.Ref
		executed.backupRef = backup.Ref
		executed.rollback = true
		if _, err := driver.Apply(item.plan.TreeLiveTarget, item.treeState); err != nil {
			restoreErr := fmt.Errorf("apply restore %s: %w", item.source.SettingRef, err)
			record := failedRestoreItemRecord(item, &currentRecord, restoreErr, runID)
			attachRestoreBackup(&record, backupRef)
			return record, executed, restoreErr
		}
	}
	if _, err := driver.Verify(item.plan.TreeLiveTarget, item.treeState); err != nil {
		restoreErr := fmt.Errorf("verify restore %s: %w", item.source.SettingRef, err)
		record := failedRestoreItemRecord(item, &currentRecord, restoreErr, runID)
		attachRestoreBackup(&record, backupRef)
		return record, executed, restoreErr
	}
	record := verifiedRestoreItemRecord(item, currentRecord, runID)
	attachRestoreBackup(&record, backupRef)
	return record, executed, nil
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
	desiredURI := item.desiredURI
	desiredRelPath := item.desiredRelPath
	if item.plan != nil {
		if desiredURI == "" {
			desiredURI = item.plan.Setting.DesiredURI
		}
		if desiredRelPath == "" {
			desiredRelPath = item.plan.DesiredRelPath
		}
	}
	record := ItemRecord{
		TargetRef:        item.source.TargetRef,
		SettingRef:       item.source.SettingRef,
		Operation:        RestoreCommand,
		ResourceID:       item.source.ResourceID,
		Driver:           item.source.Driver,
		DriverVersion:    driverVersion(item.source.Driver),
		DesiredURI:       desiredURI,
		DesiredRelPath:   desiredRelPath,
		LivePath:         item.livePath,
		DesiredPath:      item.desiredPath,
		Before:           before,
		Desired:          item.restore,
		SourceBackupRefs: []string{item.source.Ref},
		ArtifactRefs: ArtifactRefs{
			Desired:             desiredURI,
			DesiredURI:          desiredURI,
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

func (s *Store) rollbackRestoreExecution(runID string, executed []restoreExecutedItem) restoreRollbackResult {
	rollbackItems := make([]restoreExecutedItem, 0, len(executed))
	for _, item := range executed {
		if item.rollback {
			rollbackItems = append(rollbackItems, item)
		}
	}
	if len(rollbackItems) == 0 {
		return restoreRollbackResult{}
	}

	result := restoreRollbackResult{Attempted: true}
	backup, err := s.ReadBackup(runID)
	if err != nil {
		result.ReadError = fmt.Errorf("read backup-before-restore metadata for rollback run %s: %w", runID, err)
		return result
	}
	backup = NormalizeBackupMetadata(backup)
	for i := len(rollbackItems) - 1; i >= 0; i-- {
		item := rollbackItems[i]
		result.Items = append(result.Items, restoreRollbackItem{
			recordIndex: item.recordIndex,
			backupRef:   item.backupRef,
			err:         s.rollbackRestoreItem(runID, backup, item),
		})
	}
	return result
}

func (s *Store) rollbackRestoreItem(runID string, backup BackupMetadata, executed restoreExecutedItem) error {
	if strings.TrimSpace(executed.backupRef) == "" {
		return fmt.Errorf("rollback backup reference is missing for %s", executed.item.source.SettingRef)
	}
	backupItem, ok := backupItemByRef(backup, executed.backupRef)
	if !ok {
		return fmt.Errorf("rollback backup %s was not found in backup-before-restore metadata", executed.backupRef)
	}
	if executed.item.source.Driver == recipe.FileTreeDriverID && !executed.item.selectedWholeFile {
		state, err := s.loadTreeRestoreState(runID, backupItem)
		if err != nil {
			return fmt.Errorf("load rollback tree payload %s: %w", executed.backupRef, err)
		}
		driver := filetreedriver.Driver{}
		if _, err := driver.Apply(executed.item.plan.TreeLiveTarget, state); err != nil {
			return fmt.Errorf("apply rollback %s: %w", executed.item.source.SettingRef, err)
		}
		if _, err := driver.Verify(executed.item.plan.TreeLiveTarget, state); err != nil {
			return fmt.Errorf("verify rollback %s: %w", executed.item.source.SettingRef, err)
		}
		return nil
	}

	state, err := s.loadFileRestoreState(runID, backupItem)
	if err != nil {
		return fmt.Errorf("load rollback file payload %s: %w", executed.backupRef, err)
	}
	target := executed.item.liveTarget
	if !executed.item.selectedWholeFile {
		target = executed.item.plan.LiveTarget
	}
	driver := filedriver.Driver{}
	if _, err := driver.Apply(target, state); err != nil {
		return fmt.Errorf("apply rollback %s: %w", executed.item.source.SettingRef, err)
	}
	if _, err := driver.Verify(target, state); err != nil {
		return fmt.Errorf("verify rollback %s: %w", executed.item.source.SettingRef, err)
	}
	return nil
}

func backupItemByRef(metadata BackupMetadata, ref string) (BackupItem, bool) {
	ref = strings.TrimSpace(ref)
	for _, item := range metadata.Items {
		item = NormalizeBackupItem(item)
		if item.Ref == ref {
			return item, true
		}
	}
	return BackupItem{}, false
}

func (r restoreRollbackResult) Status() string {
	if !r.Attempted {
		return "not-attempted"
	}
	failed := 0
	succeeded := 0
	if r.ReadError != nil {
		failed++
	}
	for _, item := range r.Items {
		if item.err != nil {
			failed++
		} else {
			succeeded++
		}
	}
	switch {
	case failed > 0 && succeeded > 0:
		return "partially-failed"
	case failed > 0:
		return "failed"
	default:
		return "succeeded"
	}
}

func (r restoreRollbackResult) Err() error {
	var errs []error
	if r.ReadError != nil {
		errs = append(errs, r.ReadError)
	}
	for _, item := range r.Items {
		if item.err != nil {
			errs = append(errs, fmt.Errorf("rollback %s: %w", item.backupRef, item.err))
		}
	}
	return errors.Join(errs...)
}

func annotateRestoreRunRollback(record RunRecord, failedIndex int, rollback restoreRollbackResult, trigger error) RunRecord {
	if failedIndex >= 0 && failedIndex < len(record.Items) {
		record.Items[failedIndex].Diagnostics = append(record.Items[failedIndex].Diagnostics, Diagnostic{
			Code:    "restore.execution-failed",
			Message: restoreExecutionMessage(trigger, rollback),
			Path:    record.Items[failedIndex].LivePath,
		})
		if !rollback.Attempted {
			record.Items[failedIndex].Diagnostics = append(record.Items[failedIndex].Diagnostics, Diagnostic{
				Code:    "restore.rollback-not-attempted",
				Message: "Rollback was not attempted because no backup-before-restore artifact had reached the apply stage for this failed restore run.",
				Path:    record.Items[failedIndex].LivePath,
			})
		}
		if rollback.ReadError != nil {
			record.Items[failedIndex].Diagnostics = append(record.Items[failedIndex].Diagnostics, Diagnostic{
				Code:    "restore.rollback-failed",
				Message: rollback.ReadError.Error(),
				Path:    record.Items[failedIndex].LivePath,
			})
		}
		record.Items[failedIndex] = NormalizeItemRecord(record.Items[failedIndex])
	}

	for _, result := range rollback.Items {
		if result.recordIndex < 0 || result.recordIndex >= len(record.Items) {
			continue
		}
		item := &record.Items[result.recordIndex]
		if result.err != nil {
			item.Verification = Verification{Verified: false, Result: "rollback-failed", Message: result.err.Error()}
			item.Result = ItemResultFailed
			item.Diagnostics = append(item.Diagnostics, Diagnostic{
				Code:    "restore.rollback-failed",
				Message: result.err.Error(),
				Path:    item.LivePath,
			})
		} else {
			item.Verification = Verification{Verified: false, Result: "rolled-back", Message: "Restore was rolled back and rollback verification succeeded."}
			item.Result = ItemResultFailed
			item.Diagnostics = append(item.Diagnostics, Diagnostic{
				Code:    "restore.rollback-succeeded",
				Message: "Restore wrote this item, then rolled it back from the backup-before-restore artifact because the restore run failed.",
				Path:    item.LivePath,
			})
		}
		*item = NormalizeItemRecord(*item)
	}
	return record
}

func annotateRestorePreviewRollback(envelope preview.Envelope, failedIndex int, rollback restoreRollbackResult, trigger error) preview.Envelope {
	envelope = preview.NormalizeEnvelope(envelope)
	rollbackByIndex := map[int]restoreRollbackItem{}
	for _, item := range rollback.Items {
		rollbackByIndex[item.recordIndex] = item
	}
	for i := range envelope.Items {
		switch {
		case i == failedIndex:
			markRestorePreviewExecutionFailed(&envelope.Items[i], trigger, rollback)
		case i > failedIndex && failedIndex >= 0:
			markRestorePreviewBlockedAfterExecutionFailure(&envelope.Items[i])
		case i < failedIndex:
			if result, ok := rollbackByIndex[i]; ok {
				markRestorePreviewRollbackResult(&envelope.Items[i], result)
			} else {
				markRestorePreviewNoRollbackNeeded(&envelope.Items[i])
			}
		}
		if result, ok := rollbackByIndex[i]; ok && i == failedIndex {
			markRestorePreviewRollbackResult(&envelope.Items[i], result)
		}
		envelope.Items[i] = preview.NormalizeItem(envelope.Items[i])
	}
	return preview.NormalizeEnvelope(envelope)
}

func markRestorePreviewExecutionFailed(item *preview.Item, trigger error, rollback restoreRollbackResult) {
	if item == nil {
		return
	}
	item.Result = preview.ResultFailed
	item.State = status.StateBlockedSafety
	item.Message = restoreExecutionMessage(trigger, rollback)
	item.Diagnostics = append(item.Diagnostics, preview.Diagnostic{
		Code:     "restore.execution-failed",
		Severity: preview.SeverityError,
		Message:  restoreErrorMessage(trigger),
		Path:     item.LivePath,
		ExitCode: preview.ExitInternalError,
	})
	if !rollback.Attempted {
		item.Diagnostics = append(item.Diagnostics, preview.Diagnostic{
			Code:     "restore.rollback-not-attempted",
			Severity: preview.SeverityInfo,
			Message:  "Rollback was not attempted because no backup-before-restore artifact had reached the apply stage for this failed restore run.",
			Path:     item.LivePath,
		})
	}
	if rollback.ReadError != nil {
		item.Diagnostics = append(item.Diagnostics, preview.Diagnostic{
			Code:     "restore.rollback-failed",
			Severity: preview.SeverityError,
			Message:  rollback.ReadError.Error(),
			Path:     item.LivePath,
			ExitCode: preview.ExitInternalError,
		})
	}
}

func markRestorePreviewRollbackResult(item *preview.Item, result restoreRollbackItem) {
	if item == nil {
		return
	}
	item.Result = preview.ResultFailed
	item.State = status.StateBlockedSafety
	if result.err != nil {
		item.Message = "Restore rollback failed; live state may require manual recovery from the backup-before-restore artifact."
		item.Diagnostics = append(item.Diagnostics, preview.Diagnostic{
			Code:     "restore.rollback-failed",
			Severity: preview.SeverityError,
			Message:  result.err.Error(),
			Ref:      result.backupRef,
			Path:     item.LivePath,
			ExitCode: preview.ExitInternalError,
		})
		return
	}
	item.Message = "Restore wrote this item, then rolled it back from the backup-before-restore artifact because the restore run failed."
	item.Diagnostics = append(item.Diagnostics, preview.Diagnostic{
		Code:     "restore.rollback-succeeded",
		Severity: preview.SeverityInfo,
		Message:  "Rollback verification succeeded.",
		Ref:      result.backupRef,
		Path:     item.LivePath,
	})
}

func markRestorePreviewNoRollbackNeeded(item *preview.Item) {
	if item == nil {
		return
	}
	item.Result = preview.ResultBlocked
	item.State = status.StateBlockedSafety
	item.Message = "Restore run failed after this item was checked; no rollback was needed because this item had no live write."
	item.Diagnostics = append(item.Diagnostics, preview.Diagnostic{
		Code:     "restore.rollback-not-attempted",
		Severity: preview.SeverityInfo,
		Message:  "No backup-before-restore artifact had reached the apply stage for this item.",
		Path:     item.LivePath,
	})
}

func markRestorePreviewBlockedAfterExecutionFailure(item *preview.Item) {
	if item == nil {
		return
	}
	item.Result = preview.ResultBlocked
	item.State = status.StateBlockedSafety
	item.Message = "Restore run is all-or-nothing and failed before this item executed."
	item.Backup = preview.Backup{
		Policy:  preview.BackupSkippedForBlocker,
		Message: "No backup-before-restore was created because an earlier live restore failed.",
	}
	item.Diagnostics = append(item.Diagnostics, preview.Diagnostic{
		Code:     "restore.blocked-by-execution-failure",
		Severity: preview.SeverityError,
		Message:  "An earlier item failed during live restore; remaining writes were not attempted.",
		Path:     item.LivePath,
		ExitCode: preview.ExitSafetyBlocker,
	})
}

func restoreExecutionMessage(trigger error, rollback restoreRollbackResult) string {
	message := "Restore failed during live execution."
	if trigger != nil {
		message = "Restore failed during live execution: " + trigger.Error()
	}
	switch rollback.Status() {
	case "succeeded":
		return message + " Rollback succeeded and was verified for every written item."
	case "partially-failed":
		return message + " Rollback partially failed; manual recovery may be required."
	case "failed":
		return message + " Rollback failed; manual recovery may be required."
	default:
		return message + " Rollback was not attempted because no live write reached the rollbackable stage."
	}
}

func restoreErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func restorePreviewItem(plan *customfiles.Plan, source BackupItem, setting resolution.ResolvedSetting, livePath string, desiredPath string, desiredURI string, desiredRelPath string, change preview.Change, dryRun bool, selectedWholeFile bool) preview.Item {
	result := preview.ResultWouldChange
	stateCode := status.StateReadyToApply
	message := fmt.Sprintf("Restore would replace live state with backup %s from run metadata.", source.Ref)
	if selectedWholeFile {
		message = fmt.Sprintf("Restore would roll back the whole backing file for selected value %s using backup %s; this is not a semantic single-value rollback.", source.SettingRef, source.Ref)
	}
	if change.Kind == filedriver.ChangeUnchanged {
		result = preview.ResultUnchanged
		stateCode = status.StateUnchanged
		message = fmt.Sprintf("Live state already matches backup %s.", source.Ref)
		if selectedWholeFile {
			message = fmt.Sprintf("Whole backing file for selected value %s already matches backup %s.", source.SettingRef, source.Ref)
		}
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
		Scope:          setting.Scope,
		Subject:        setting.Subject,
		DesiredURI:     desiredURI,
		DesiredRelPath: desiredRelPath,
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

func blockedRestorePreviewItem(item BackupItem, err error, dryRun bool) preview.Item {
	code := "restore.blocked"
	if !item.Restore.Compatible || item.Driver == recipe.NativeExportDriverID {
		code = "restore.unsupported"
	}
	message := "restore is blocked before writes"
	if err != nil {
		message = err.Error()
	}
	return preview.NormalizeItem(preview.Item{
		TargetRef:  item.TargetRef,
		SettingRef: item.SettingRef,
		Operation:  RestoreCommand,
		Driver:     item.Driver,
		ResourceID: item.ResourceID,
		LivePath:   item.LivePath,
		DryRun:     dryRun,
		State:      status.StateBlockedSafety,
		Message:    "Restore run is all-or-nothing; this item blocks every write in the run.",
		Backup: preview.Backup{
			Policy:  preview.BackupSkippedForBlocker,
			Message: fmt.Sprintf("Restore source: %s. No backup-before-restore will be created because planning is blocked.", item.Ref),
		},
		Result: preview.ResultBlocked,
		Diagnostics: []preview.Diagnostic{{
			Code:     code,
			Severity: preview.SeverityError,
			Message:  message,
			Ref:      item.Ref,
			Path:     item.LivePath,
			ExitCode: preview.ExitSafetyBlocker,
		}},
		Actions:        []status.Action{status.ActionInspect},
		AutomaticMerge: false,
	})
}

func blockAllRestorePreviewItems(items []preview.Item, sourceRunID string) []preview.Item {
	blocked := append([]preview.Item(nil), items...)
	for i := range blocked {
		if blocked[i].Result == preview.ResultBlocked {
			blocked[i] = preview.NormalizeItem(blocked[i])
			continue
		}
		blocked[i].Result = preview.ResultBlocked
		blocked[i].State = status.StateBlockedSafety
		blocked[i].Message = fmt.Sprintf("Restore run %s is all-or-nothing and has at least one blocker; this planned write was not executed.", sourceRunID)
		blocked[i].Backup = preview.Backup{
			Policy:  preview.BackupSkippedForBlocker,
			Message: "No backup-before-restore will be created because the restore run is blocked before writes.",
		}
		blocked[i].Diagnostics = append(blocked[i].Diagnostics, preview.Diagnostic{
			Code:     "restore.blocked-by-run",
			Severity: preview.SeverityError,
			Message:  "Another item in this restore run is blocked; all writes are disabled for the run.",
			ExitCode: preview.ExitSafetyBlocker,
		})
		blocked[i] = preview.NormalizeItem(blocked[i])
	}
	return blocked
}

func runtimeForRestoreItem(profile *resolution.ResolvedProfile, item BackupItem) (*recipe.Recipe, resolution.ResolvedSetting, error) {
	if profile == nil {
		return nil, resolution.ResolvedSetting{}, fmt.Errorf("resolved profile is required")
	}
	setting, err := resolvedSettingForBackup(profile, item)
	if err != nil {
		return nil, resolution.ResolvedSetting{}, err
	}
	runtime, err := recipe.LoadRuntime(profile.RepoRoot, setting.TargetID)
	if err != nil {
		if setting.TargetID == recipe.CustomFilesTarget && errors.Is(err, recipe.ErrBundledRuntimeUnavailable) {
			rec, loadErr := recipe.LoadCustomFiles(profile.RepoRoot)
			if loadErr != nil {
				return nil, setting, fmt.Errorf("load restore recipe %s: %w", setting.TargetID, loadErr)
			}
			return rec, setting, nil
		}
		return nil, setting, fmt.Errorf("load restore recipe %s: %w", setting.TargetID, err)
	}
	if runtime.Recipe == nil {
		return nil, setting, fmt.Errorf("load restore recipe %s: recipe is unavailable", setting.TargetID)
	}
	return runtime.Recipe, setting, nil
}

func resolvedSettingForBackup(profile *resolution.ResolvedProfile, item BackupItem) (resolution.ResolvedSetting, error) {
	target, settingID, err := splitSettingRef(item.SettingRef)
	if err != nil {
		return resolution.ResolvedSetting{}, err
	}
	if item.TargetRef != "" && item.TargetRef != target {
		return resolution.ResolvedSetting{}, fmt.Errorf("restore backup item %s target mismatch: targetRef=%s settingRef=%s", item.Ref, item.TargetRef, item.SettingRef)
	}
	for _, setting := range profile.Settings {
		if setting.TargetID == target && setting.SettingID == settingID {
			return setting, nil
		}
	}
	return resolution.ResolvedSetting{}, fmt.Errorf("restore backup item %s setting %s is not selected in the resolved profile", item.Ref, item.SettingRef)
}

func splitSettingRef(ref string) (string, string, error) {
	target, setting, ok := strings.Cut(strings.TrimSpace(ref), ":")
	if !ok || strings.TrimSpace(target) == "" || strings.TrimSpace(setting) == "" || strings.Contains(setting, ":") {
		return "", "", fmt.Errorf("invalid setting ref %s", ref)
	}
	return target, setting, nil
}

func targetLocationRoots(roots map[string]map[string]string, targetRef string) map[string]string {
	if roots == nil {
		return nil
	}
	return roots[targetRef]
}

func sameFilePath(left string, right string) bool {
	leftAbs, leftErr := filepath.Abs(strings.TrimSpace(left))
	rightAbs, rightErr := filepath.Abs(strings.TrimSpace(right))
	if leftErr != nil || rightErr != nil {
		return strings.TrimSpace(left) == strings.TrimSpace(right)
	}
	return filepath.Clean(leftAbs) == filepath.Clean(rightAbs)
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
