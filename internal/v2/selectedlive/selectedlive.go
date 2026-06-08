package selectedlive

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/shpoont/dotfiles-manager/internal/v2/desired"
	"github.com/shpoont/dotfiles-manager/internal/v2/filedriver"
	v2ledger "github.com/shpoont/dotfiles-manager/internal/v2/ledger"
	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/shpoont/dotfiles-manager/internal/v2/resolution"
	"github.com/shpoont/dotfiles-manager/internal/v2/selectedpreview"
	"github.com/shpoont/dotfiles-manager/internal/v2/selectedvalue"
	v2status "github.com/shpoont/dotfiles-manager/internal/v2/status"
)

const (
	CodeConfirmationRequired = "selectedlive.confirmationRequired"
	CodePlanBlocked          = "selectedlive.planBlocked"
	CodeExecutionFailed      = "selectedlive.executionFailed"
)

type Options struct {
	Command       string
	RepoRoot      string
	StateRoot     string
	Ref           string
	MachineID     string
	UserID        string
	ExtraLayers   []string
	DryRun        bool
	Confirmed     bool
	RunID         string
	Now           func() time.Time
	LocationRoots map[string]map[string]string
	AfterApply    func(*selectedvalue.Plan) error
}

type Result struct {
	Report        *selectedpreview.Report
	RunRecord     *v2ledger.RunRecord
	LedgerEntries []v2ledger.LedgerEntry
	Backup        *v2ledger.BackupMetadata
}

func Run(opts Options) (*Result, error) {
	command, err := normalizeCommand(opts.Command)
	if err != nil {
		report := selectedpreview.ErrorReport(opts.Command, opts.DryRun, "selectedlive.command.invalid", err.Error(), nil)
		return &Result{Report: report}, &selectedpreview.Error{Code: "selectedlive.command.invalid", Message: err.Error(), Exit: 2}
	}
	report, err := selectedpreview.Build(selectedpreview.Options{
		Command:       command,
		RepoRoot:      opts.RepoRoot,
		StateRoot:     opts.StateRoot,
		Ref:           opts.Ref,
		MachineID:     opts.MachineID,
		UserID:        opts.UserID,
		ExtraLayers:   opts.ExtraLayers,
		DryRun:        opts.DryRun,
		LocationRoots: opts.LocationRoots,
	})
	if err != nil || opts.DryRun {
		return &Result{Report: report}, err
	}
	if report == nil {
		report = selectedpreview.ErrorReport(command, false, CodeExecutionFailed, "selected-value live planning did not produce a report", nil)
		return &Result{Report: report}, &selectedpreview.Error{Code: CodeExecutionFailed, Message: "selected-value live planning did not produce a report", Exit: 2}
	}
	if hasPlanBlocker(report) {
		attachReportError(report, CodePlanBlocked, "selected-value live write is blocked by the plan; no files were mutated", nil)
		return &Result{Report: report}, &selectedpreview.Error{Code: CodePlanBlocked, Message: "selected-value live write is blocked by the plan; no files were mutated", Exit: 2}
	}
	if !opts.Confirmed {
		if requiresConfirmation(report) {
			attachReportError(report, CodeConfirmationRequired, "selected-value live write requires --yes before mutation", map[string]any{"requiredFlag": "--yes"})
			return &Result{Report: report}, &selectedpreview.Error{Code: CodeConfirmationRequired, Message: "selected-value live write requires --yes before mutation", Exit: 4, Details: map[string]any{"requiredFlag": "--yes"}}
		}
		return &Result{Report: report}, nil
	}

	repoRoot, err := normalizeRepoRoot(opts.RepoRoot)
	if err != nil {
		attachReportError(report, "selectedlive.repo.invalid", err.Error(), nil)
		return &Result{Report: report}, &selectedpreview.Error{Code: "selectedlive.repo.invalid", Message: err.Error(), Exit: 2}
	}
	stateRoot, err := normalizeStateRoot(repoRoot, opts.StateRoot)
	if err != nil {
		attachReportError(report, "selectedlive.stateRoot.invalid", "selected-value local state root is invalid", nil)
		return &Result{Report: report}, &selectedpreview.Error{Code: "selectedlive.stateRoot.invalid", Message: "selected-value local state root is invalid", Exit: 2}
	}
	store, err := v2ledger.NewStore(stateRoot, v2ledger.WithClock(clock(opts)))
	if err != nil {
		attachReportError(report, "selectedlive.ledger.open", "selected-value ledger store could not be opened", nil)
		return &Result{Report: report}, &selectedpreview.Error{Code: "selectedlive.ledger.open", Message: "selected-value ledger store could not be opened", Exit: 2}
	}
	if err := v2ledger.ValidateStateRoot(repoRoot, stateRoot); err != nil {
		attachReportError(report, "selectedlive.stateRoot.invalid", "selected-value local state root is invalid", nil)
		return &Result{Report: report}, &selectedpreview.Error{Code: "selectedlive.stateRoot.invalid", Message: "selected-value local state root is invalid", Exit: 2}
	}

	profile, err := resolution.Resolve(repoRoot, resolution.ResolveOptions{MachineID: opts.MachineID, UserID: opts.UserID, ExtraLayers: opts.ExtraLayers})
	if err != nil {
		attachReportError(report, "selectedlive.profile.resolve", "selected-value profile could not be resolved", nil)
		return &Result{Report: report}, &selectedpreview.Error{Code: "selectedlive.profile.resolve", Message: "selected-value profile could not be resolved", Exit: 2}
	}
	ref, err := parseRef(opts.Ref)
	if err != nil {
		attachReportError(report, "selectedlive.ref.invalid", err.Error(), map[string]any{"ref": opts.Ref})
		return &Result{Report: report}, &selectedpreview.Error{Code: "selectedlive.ref.invalid", Message: err.Error(), Exit: 2}
	}
	settings := filterSettings(profile.Settings, ref)
	if len(settings) == 0 {
		msg := fmt.Sprintf("no selected settings match ref %q", opts.Ref)
		attachReportError(report, "selectedlive.ref.notFound", msg, map[string]any{"ref": opts.Ref})
		return &Result{Report: report}, &selectedpreview.Error{Code: "selectedlive.ref.notFound", Message: msg, Exit: 2}
	}

	started := clock(opts)().UTC()
	runID := runID(opts, started)
	report.RunID = runID
	itemReports := itemReportMap(report)
	items := make([]v2ledger.ItemRecord, 0, len(settings))
	for _, setting := range settings {
		preItem := itemReports[setting.Ref()]
		if preItem.PlannedAction == "none" {
			items = append(items, unchangedItemRecord(command, runID, setting, preItem))
			markReportItem(report, setting.Ref(), items[len(items)-1])
			continue
		}
		if !isActionable(command, preItem) {
			items = append(items, skippedItemRecord(command, runID, setting, preItem, "selected-value item is not actionable for live mutation"))
			markReportItem(report, setting.Ref(), items[len(items)-1])
			continue
		}
		rec, source, trustContext, resourceID, resource, err := runtimeContext(repoRoot, stateRoot, setting)
		if err != nil {
			item := failedItemRecord(command, runID, setting, resourceID, resource, preItem, safeDiagnostic("selectedlive.plan", err, preItem.Resource.Path), nil)
			items = append(items, item)
			markReportItem(report, setting.Ref(), item)
			continue
		}
		locationRoots := opts.LocationRoots[setting.TargetID]
		if locationRoots == nil {
			locationRoots = map[string]string{}
		}
		_ = source
		switch command {
		case selectedpreview.CommandSave:
			item := executeSave(repoRoot, runID, setting, rec, trustContext, resourceID, resource, locationRoots, preItem)
			items = append(items, item)
			markReportItem(report, setting.Ref(), item)
		case selectedpreview.CommandApply:
			item := executeApply(repoRoot, runID, started, store, setting, rec, trustContext, resourceID, resource, locationRoots, preItem, opts.AfterApply)
			items = append(items, item)
			markReportItem(report, setting.Ref(), item)
		}
	}
	finished := clock(opts)().UTC()
	record := v2ledger.NormalizeRunRecord(v2ledger.RunRecord{
		RunID:        runID,
		StartedAt:    formatTime(started),
		FinishedAt:   formatTime(finished),
		Command:      command,
		ProfileStack: append([]string(nil), profile.Layers...),
		Items:        items,
	})
	entries, err := store.CommitRun(record)
	if err != nil {
		attachReportError(report, "selectedlive.ledger.commit", "selected-value ledger commit failed", nil)
		return &Result{Report: report, RunRecord: &record}, &selectedpreview.Error{Code: "selectedlive.ledger.commit", Message: "selected-value ledger commit failed", Exit: 2}
	}
	var backup *v2ledger.BackupMetadata
	if command == selectedpreview.CommandApply {
		if metadata, readErr := store.ReadBackup(runID); readErr == nil {
			backup = &metadata
		} else if !errors.Is(readErr, os.ErrNotExist) {
			attachReportError(report, "selectedlive.backup.read", "selected-value backup metadata could not be read after apply", nil)
			return &Result{Report: report, RunRecord: &record, LedgerEntries: entries}, &selectedpreview.Error{Code: "selectedlive.backup.read", Message: "selected-value backup metadata could not be read after apply", Exit: 2}
		}
	}
	finishLiveReport(report, record)
	if record.Summary.Failed > 0 {
		attachReportError(report, CodeExecutionFailed, "selected-value live write completed with failed items", nil)
		return &Result{Report: report, RunRecord: &record, LedgerEntries: entries, Backup: backup}, &selectedpreview.Error{Code: CodeExecutionFailed, Message: "selected-value live write completed with failed items", Exit: 2}
	}
	return &Result{Report: report, RunRecord: &record, LedgerEntries: entries, Backup: backup}, nil
}

func executeSave(repoRoot string, runID string, setting resolution.ResolvedSetting, rec *recipe.Recipe, trustContext recipe.WriteSafetyContext, resourceID string, resource recipe.Resource, roots map[string]string, preItem selectedpreview.Item) v2ledger.ItemRecord {
	beforeRead, _ := desired.ReadSelectedValueForSetting(repoRoot, setting)
	beforeSnapshot := selectedValueDesiredSnapshot(rec, setting, roots, beforeRead, trustContext)
	current, err := selectedvalue.ReadCurrentDesired(selectedvalue.Request{Recipe: rec, SettingRef: setting.Ref(), LocationRoots: roots})
	if err != nil {
		return failedItemRecord(selectedpreview.CommandSave, runID, setting, resourceID, resource, preItem, selectedValueCurrentDiagnostic(current, "selectedlive.save.read", err), nil)
	}
	saveValue, err := desiredValueFromSelected(current.Desired)
	if err != nil {
		return failedItemRecord(selectedpreview.CommandSave, runID, setting, resourceID, resource, preItem, v2ledger.Diagnostic{Code: "selectedlive.save.currentInvalid", Message: "current selected value cannot be represented as desired state", Path: preItem.Resource.Path}, nil)
	}
	if err := desired.WriteSelectedValue(desired.WriteRequest{RepoRoot: repoRoot, URI: setting.DesiredURI, Value: saveValue, Safety: &desired.WriteSafetyDecision{Recipe: rec, SettingRef: setting.Ref(), Context: trustContext}}); err != nil {
		return failedItemRecord(selectedpreview.CommandSave, runID, setting, resourceID, resource, preItem, desiredSafetyDiagnostic("selectedlive.save.write", err, setting.DesiredRelPath), nil)
	}
	verifyRead, err := desired.ReadSelectedValueForSetting(repoRoot, setting)
	if err != nil || verifyRead.Desired == nil {
		return failedItemRecord(selectedpreview.CommandSave, runID, setting, resourceID, resource, preItem, safeDiagnostic("selectedlive.save.verifyDesired", err, setting.DesiredRelPath), nil)
	}
	verifyPlan, err := selectedvalue.PlanPreview(selectedvalue.PreviewRequest{Request: selectedvalue.Request{Recipe: rec, SettingRef: setting.Ref(), LocationRoots: roots}, Desired: *verifyRead.Desired, WriteSafetyContext: trustContext})
	if err != nil {
		return failedItemRecord(selectedpreview.CommandSave, runID, setting, resourceID, resource, preItem, selectedValuePlanDiagnosticFromPlan(verifyPlan, "selectedlive.save.verify", err), nil)
	}
	before := beforeSnapshot
	desiredState := v2ledger.SelectedValueState(current.Plan.Current, resource.Driver)
	verified := v2ledger.SelectedValueState(*verifyPlan.Desired, resource.Driver)
	mutated := !sameState(before, desiredState)
	result := v2ledger.ItemResultVerified
	if !mutated {
		result = v2ledger.ItemResultUnchanged
	}
	return normalizeSelectedValueItem(v2ledger.ItemRecord{
		TargetRef:      setting.TargetID,
		SettingRef:     setting.Ref(),
		Operation:      selectedpreview.CommandSave,
		ResourceID:     resourceID,
		Driver:         resource.Driver,
		DriverVersion:  v2ledger.SelectedValueDriverVersion(resource.Driver),
		DesiredURI:     setting.DesiredURI,
		DesiredRelPath: setting.DesiredRelPath,
		LivePath:       preItem.Resource.Path,
		DesiredPath:    setting.DesiredPath,
		ArtifactRefs:   selectedValueArtifactRefs(runID, setting, preItem.Resource.Path),
		Before:         before,
		Desired:        desiredState,
		VerifiedState:  verified,
		Verification:   v2ledger.Verification{Verified: sameState(desiredState, verified), Result: verificationResult(sameState(desiredState, verified))},
		Result:         resultForVerification(result, sameState(desiredState, verified)),
	})
}

func executeApply(repoRoot string, runID string, started time.Time, store *v2ledger.Store, setting resolution.ResolvedSetting, rec *recipe.Recipe, trustContext recipe.WriteSafetyContext, resourceID string, resource recipe.Resource, roots map[string]string, preItem selectedpreview.Item, afterApply func(*selectedvalue.Plan) error) v2ledger.ItemRecord {
	read, err := desired.ReadSelectedValueForSetting(repoRoot, setting)
	if err != nil {
		return failedItemRecord(selectedpreview.CommandApply, runID, setting, resourceID, resource, preItem, safeDiagnostic("selectedlive.apply.desiredRead", err, setting.DesiredRelPath), nil)
	}
	if read.Status == desired.StatusUnmanaged {
		return skippedItemRecord(selectedpreview.CommandApply, runID, setting, preItem, "selected value is intentionally unmanaged")
	}
	if read.Status == desired.StatusMissing || read.Desired == nil || read.Value == nil {
		return failedItemRecord(selectedpreview.CommandApply, runID, setting, resourceID, resource, preItem, v2ledger.Diagnostic{Code: "selectedlive.apply.missingDesired", Message: "desired selected-value artifact is missing", Path: setting.DesiredRelPath}, nil)
	}
	if err := desired.ValidateSelectedValueWriteSafety(desired.WriteRequest{RepoRoot: repoRoot, URI: setting.DesiredURI, Value: *read.Value, Safety: &desired.WriteSafetyDecision{Recipe: rec, SettingRef: setting.Ref(), Context: trustContext}}); err != nil {
		return failedItemRecord(selectedpreview.CommandApply, runID, setting, resourceID, resource, preItem, desiredSafetyDiagnostic("selectedlive.apply.writeSafety", err, setting.DesiredRelPath), nil)
	}
	result, err := selectedvalue.ApplyWithBackup(selectedvalue.PreviewRequest{Request: selectedvalue.Request{Recipe: rec, SettingRef: setting.Ref(), LocationRoots: roots}, Desired: *read.Desired, WriteSafetyContext: trustContext}, selectedvalue.ApplyOptions{
		BackupHook: selectedValueBackupHook(store, runID, started, setting, resourceID, resource),
		AfterApply: afterApply,
	})
	if err != nil {
		return failedItemRecord(selectedpreview.CommandApply, runID, setting, resourceID, resource, preItem, selectedValuePlanDiagnostic(result, "selectedlive.apply.execute", err), selectedValueBackupRefs(result))
	}
	item := selectedValueItemFromApply(runID, setting, resourceID, resource, preItem, result)
	return item
}

func selectedValueItemFromApply(runID string, setting resolution.ResolvedSetting, resourceID string, resource recipe.Resource, preItem selectedpreview.Item, result *selectedvalue.ApplyResult) v2ledger.ItemRecord {
	before := v2ledger.NormalizedState{DriverVersion: v2ledger.SelectedValueDriverVersion(resource.Driver), Normalizer: v2ledger.SelectedValueNormalizer(resource.Driver)}
	desiredState := before
	if result != nil && result.Plan != nil {
		before = v2ledger.SelectedValueState(result.Plan.Current, resource.Driver)
		if result.Plan.Desired != nil {
			desiredState = v2ledger.SelectedValueState(*result.Plan.Desired, resource.Driver)
		}
	}
	verified := desiredState
	refs := selectedValueArtifactRefs(runID, setting, preItem.Resource.Path)
	backupRefs := selectedValueBackupRefs(result)
	if len(backupRefs) > 0 {
		refs.Backup = backupRefs[0]
		refs.BackupPayload = strings.TrimRight(backupRefs[0], "/") + "/payload"
	}
	itemResult := v2ledger.ItemResultVerified
	if result == nil || !result.Mutated {
		itemResult = v2ledger.ItemResultUnchanged
	}
	return normalizeSelectedValueItem(v2ledger.ItemRecord{
		TargetRef:      setting.TargetID,
		SettingRef:     setting.Ref(),
		Operation:      selectedpreview.CommandApply,
		ResourceID:     resourceID,
		Driver:         resource.Driver,
		DriverVersion:  v2ledger.SelectedValueDriverVersion(resource.Driver),
		DesiredURI:     setting.DesiredURI,
		DesiredRelPath: setting.DesiredRelPath,
		LivePath:       preItem.Resource.Path,
		DesiredPath:    setting.DesiredPath,
		ArtifactRefs:   refs,
		Before:         before,
		Desired:        desiredState,
		VerifiedState:  verified,
		BackupRefs:     backupRefs,
		Verification:   v2ledger.Verification{Verified: result != nil && result.Verified, Result: verificationResult(result != nil && result.Verified)},
		Result:         resultForVerification(itemResult, result != nil && result.Verified),
	})
}

func selectedValueBackupHook(store *v2ledger.Store, runID string, started time.Time, setting resolution.ResolvedSetting, resourceID string, resource recipe.Resource) selectedvalue.BackupHook {
	return func(req selectedvalue.BackupRequest) (selectedvalue.BackupResult, error) {
		item, err := store.WriteSelectedValueBackup(runID, started, v2ledger.SelectedValueBackupRequest{
			TargetRef:  setting.TargetID,
			SettingRef: setting.Ref(),
			ResourceID: resourceID,
			Driver:     resource.Driver,
			LivePath:   req.Path,
			Before:     v2ledger.SelectedValueState(req.Before, resource.Driver),
			BeforeFile: append([]byte(nil), req.BeforeFile...),
		})
		if err != nil {
			return selectedvalue.BackupResult{}, err
		}
		return selectedvalue.BackupResult{ID: item.Ref, Before: req.Before}, nil
	}
}

func runtimeContext(repoRoot string, stateRoot string, setting resolution.ResolvedSetting) (*recipe.Recipe, string, recipe.WriteSafetyContext, string, recipe.Resource, error) {
	runtime, err := recipe.LoadRuntime(repoRoot, setting.TargetID)
	if err != nil {
		return nil, runtime.Source, recipe.WriteSafetyContext{}, "", recipe.Resource{}, err
	}
	rec := runtime.Recipe
	resourceID, resource, err := rec.ResourceForSetting(setting.SettingID)
	if err != nil {
		return rec, runtime.Source, recipe.WriteSafetyContext{}, "", recipe.Resource{}, err
	}
	eval, err := recipe.EvaluateRecipeTrust(repoRoot, stateRoot, runtime.Source, rec)
	if err != nil {
		return rec, runtime.Source, recipe.WriteSafetyContext{}, resourceID, resource, err
	}
	if eval.Status != recipe.TrustStatusTrusted {
		return rec, runtime.Source, recipe.WriteSafetyContext{}, resourceID, resource, fmt.Errorf("recipe trust is not trusted")
	}
	ctx := eval.WriteSafetyContext(recipe.WriteSafetyContext{})
	if err := rec.ValidateWriteSafety(ctx); err != nil {
		return rec, runtime.Source, ctx, resourceID, resource, err
	}
	return rec, runtime.Source, ctx, resourceID, resource, nil
}

func normalizeSelectedValueItem(item v2ledger.ItemRecord) v2ledger.ItemRecord {
	item.DriverVersion = v2ledger.SelectedValueDriverVersion(item.Driver)
	if item.Before.DriverVersion == "" {
		item.Before.DriverVersion = item.DriverVersion
	}
	if item.Desired.DriverVersion == "" {
		item.Desired.DriverVersion = item.DriverVersion
	}
	if item.VerifiedState.DriverVersion == "" {
		item.VerifiedState.DriverVersion = item.DriverVersion
	}
	return v2ledger.NormalizeItemRecord(item)
}

func failedItemRecord(command string, runID string, setting resolution.ResolvedSetting, resourceID string, resource recipe.Resource, preItem selectedpreview.Item, diagnostic v2ledger.Diagnostic, backupRefs []string) v2ledger.ItemRecord {
	if diagnostic.Code == "" {
		diagnostic.Code = "selectedlive.failed"
	}
	if diagnostic.Message == "" {
		diagnostic.Message = "selected-value live operation failed"
	}
	refs := selectedValueArtifactRefs(runID, setting, preItem.Resource.Path)
	if len(backupRefs) > 0 {
		refs.Backup = backupRefs[0]
		refs.BackupPayload = strings.TrimRight(backupRefs[0], "/") + "/payload"
	}
	return normalizeSelectedValueItem(v2ledger.ItemRecord{
		TargetRef:      setting.TargetID,
		SettingRef:     setting.Ref(),
		Operation:      command,
		ResourceID:     resourceID,
		Driver:         defaultString(resource.Driver, preItem.Resource.DriverID),
		DriverVersion:  v2ledger.SelectedValueDriverVersion(defaultString(resource.Driver, preItem.Resource.DriverID)),
		DesiredURI:     setting.DesiredURI,
		DesiredRelPath: setting.DesiredRelPath,
		LivePath:       preItem.Resource.Path,
		DesiredPath:    setting.DesiredPath,
		ArtifactRefs:   refs,
		BackupRefs:     backupRefs,
		Before:         v2ledger.NormalizedState{DriverVersion: v2ledger.SelectedValueDriverVersion(defaultString(resource.Driver, preItem.Resource.DriverID)), Normalizer: v2ledger.SelectedValueNormalizer(defaultString(resource.Driver, preItem.Resource.DriverID))},
		Desired:        v2ledger.NormalizedState{DriverVersion: v2ledger.SelectedValueDriverVersion(defaultString(resource.Driver, preItem.Resource.DriverID)), Normalizer: v2ledger.SelectedValueNormalizer(defaultString(resource.Driver, preItem.Resource.DriverID))},
		VerifiedState:  v2ledger.NormalizedState{DriverVersion: v2ledger.SelectedValueDriverVersion(defaultString(resource.Driver, preItem.Resource.DriverID)), Normalizer: v2ledger.SelectedValueNormalizer(defaultString(resource.Driver, preItem.Resource.DriverID))},
		Verification:   v2ledger.Verification{Verified: false, Result: "failed", Message: diagnostic.Message},
		Result:         v2ledger.ItemResultFailed,
		Diagnostics:    []v2ledger.Diagnostic{diagnostic},
	})
}

func skippedItemRecord(command string, runID string, setting resolution.ResolvedSetting, preItem selectedpreview.Item, message string) v2ledger.ItemRecord {
	return normalizeSelectedValueItem(v2ledger.ItemRecord{
		TargetRef:      setting.TargetID,
		SettingRef:     setting.Ref(),
		Operation:      command,
		ResourceID:     preItem.Resource.ID,
		Driver:         preItem.Resource.DriverID,
		DriverVersion:  v2ledger.SelectedValueDriverVersion(preItem.Resource.DriverID),
		DesiredURI:     setting.DesiredURI,
		DesiredRelPath: setting.DesiredRelPath,
		LivePath:       preItem.Resource.Path,
		DesiredPath:    setting.DesiredPath,
		ArtifactRefs:   selectedValueArtifactRefs(runID, setting, preItem.Resource.Path),
		Before:         v2ledger.NormalizedState{DriverVersion: v2ledger.SelectedValueDriverVersion(preItem.Resource.DriverID), Normalizer: v2ledger.SelectedValueNormalizer(preItem.Resource.DriverID)},
		Desired:        v2ledger.NormalizedState{DriverVersion: v2ledger.SelectedValueDriverVersion(preItem.Resource.DriverID), Normalizer: v2ledger.SelectedValueNormalizer(preItem.Resource.DriverID)},
		VerifiedState:  v2ledger.NormalizedState{DriverVersion: v2ledger.SelectedValueDriverVersion(preItem.Resource.DriverID), Normalizer: v2ledger.SelectedValueNormalizer(preItem.Resource.DriverID)},
		Verification:   v2ledger.Verification{Verified: true, Result: "skipped", Message: message},
		Result:         v2ledger.ItemResultSkipped,
	})
}

func unchangedItemRecord(command string, runID string, setting resolution.ResolvedSetting, preItem selectedpreview.Item) v2ledger.ItemRecord {
	driverVersion := v2ledger.SelectedValueDriverVersion(preItem.Resource.DriverID)
	before := v2ledger.NormalizedState{Exists: preItem.Current.Exists, Hash: preItem.Current.SHA256, Normalizer: preItem.Current.Normalizer, DriverVersion: driverVersion}
	desiredState := v2ledger.NormalizedState{Exists: preItem.Desired.Snapshot.Exists, Hash: preItem.Desired.Snapshot.SHA256, Normalizer: preItem.Desired.Snapshot.Normalizer, DriverVersion: driverVersion}
	if !desiredState.Exists && before.Exists {
		desiredState = before
	}
	if !before.Exists && desiredState.Exists {
		before = desiredState
	}
	return normalizeSelectedValueItem(v2ledger.ItemRecord{
		TargetRef:      setting.TargetID,
		SettingRef:     setting.Ref(),
		Operation:      command,
		ResourceID:     preItem.Resource.ID,
		Driver:         preItem.Resource.DriverID,
		DriverVersion:  driverVersion,
		DesiredURI:     setting.DesiredURI,
		DesiredRelPath: setting.DesiredRelPath,
		LivePath:       preItem.Resource.Path,
		DesiredPath:    setting.DesiredPath,
		ArtifactRefs:   selectedValueArtifactRefs(runID, setting, preItem.Resource.Path),
		Before:         before,
		Desired:        desiredState,
		VerifiedState:  desiredState,
		Verification:   v2ledger.Verification{Verified: true, Result: "unchanged"},
		Result:         v2ledger.ItemResultUnchanged,
	})
}

func selectedValueArtifactRefs(runID string, setting resolution.ResolvedSetting, livePath string) v2ledger.ArtifactRefs {
	return v2ledger.ArtifactRefs{
		Desired:     setting.DesiredURI,
		DesiredURI:  setting.DesiredURI,
		DesiredPath: setting.DesiredPath,
		LivePath:    livePath,
		RunRecord:   "state://ledger/runs/" + runID,
		Ledger:      "state://ledger/ledger.jsonl",
	}
}

func selectedValueDesiredSnapshot(rec *recipe.Recipe, setting resolution.ResolvedSetting, roots map[string]string, read desired.ReadResult, trustContext recipe.WriteSafetyContext) v2ledger.NormalizedState {
	if read.Status != desired.StatusPresent || read.Desired == nil {
		return v2ledger.NormalizedState{Exists: false, DriverVersion: "", Normalizer: ""}
	}
	plan, err := selectedvalue.PlanPreview(selectedvalue.PreviewRequest{Request: selectedvalue.Request{Recipe: rec, SettingRef: setting.Ref(), LocationRoots: roots}, Desired: *read.Desired, WriteSafetyContext: trustContext})
	if err != nil || plan == nil || plan.Desired == nil {
		return v2ledger.NormalizedState{Exists: false}
	}
	resourceID, resource, _ := rec.ResourceForSetting(setting.SettingID)
	_ = resourceID
	return v2ledger.SelectedValueState(*plan.Desired, resource.Driver)
}

func desiredValueFromSelected(value selectedvalue.Desired) (desired.SelectedValue, error) {
	switch value.Intent() {
	case selectedvalue.IntentDelete:
		return desired.Delete(), nil
	case selectedvalue.IntentSet:
		raw, ok := value.Value()
		if !ok {
			return desired.SelectedValue{}, fmt.Errorf("selected desired value is missing")
		}
		switch value.Kind() {
		case "string":
			typed, ok := raw.(string)
			if !ok {
				return desired.SelectedValue{}, fmt.Errorf("selected desired string has invalid representation")
			}
			return desired.SetString(typed), nil
		case "bool":
			typed, ok := raw.(bool)
			if !ok {
				return desired.SelectedValue{}, fmt.Errorf("selected desired bool has invalid representation")
			}
			return desired.SetBool(typed), nil
		case "number":
			typed, ok := raw.(json.Number)
			if !ok {
				return desired.SelectedValue{}, fmt.Errorf("selected desired number has invalid representation")
			}
			return desired.SetNumber(typed), nil
		case "null":
			return desired.SetNull(), nil
		default:
			return desired.SelectedValue{}, fmt.Errorf("unsupported selected desired kind")
		}
	default:
		return desired.SelectedValue{}, fmt.Errorf("selected desired intent is required")
	}
}

func selectedValuePlanDiagnostic(result *selectedvalue.ApplyResult, fallbackCode string, err error) v2ledger.Diagnostic {
	if result != nil && result.Plan != nil {
		return selectedValuePlanDiagnosticFromPlan(result.Plan, fallbackCode, err)
	}
	return safeDiagnostic(fallbackCode, err, "")
}

func selectedValueCurrentDiagnostic(result *selectedvalue.CurrentDesired, fallbackCode string, err error) v2ledger.Diagnostic {
	if result != nil && result.Plan != nil {
		return selectedValuePlanDiagnosticFromPlan(result.Plan, fallbackCode, err)
	}
	return safeDiagnostic(fallbackCode, err, "")
}

func selectedValuePlanDiagnosticFromPlan(plan *selectedvalue.Plan, fallbackCode string, err error) v2ledger.Diagnostic {
	if plan != nil && len(plan.Diagnostics) > 0 {
		diagnostic := plan.Diagnostics[0]
		return v2ledger.Diagnostic{Code: diagnostic.Code, Message: diagnostic.Message, Path: diagnostic.Path}
	}
	return safeDiagnostic(fallbackCode, err, "")
}

func desiredSafetyDiagnostic(fallbackCode string, err error, path string) v2ledger.Diagnostic {
	var safetyErr *desired.SafetyError
	if errors.As(err, &safetyErr) && len(safetyErr.Diagnostics) > 0 {
		diagnostic := safetyErr.Diagnostics[0]
		return v2ledger.Diagnostic{Code: diagnostic.Code, Message: diagnostic.Message, Path: diagnostic.Path}
	}
	return safeDiagnostic(fallbackCode, err, path)
}

func safeDiagnostic(code string, err error, path string) v2ledger.Diagnostic {
	message := "selected-value live operation failed"
	var driverErr *filedriver.Error
	if errors.As(err, &driverErr) {
		code = "selectedlive.driver." + string(driverErr.Code)
		if driverErr.Op != "" {
			message = "selected-value driver " + driverErr.Op + " failed"
		} else {
			message = "selected-value driver failed"
		}
	}
	if strings.TrimSpace(code) == "" {
		code = "selectedlive.failed"
	}
	return v2ledger.Diagnostic{Code: code, Message: message, Path: path}
}

func selectedValueBackupRefs(result *selectedvalue.ApplyResult) []string {
	if result == nil || result.Backup == nil || strings.TrimSpace(result.Backup.ID) == "" {
		return nil
	}
	return []string{strings.TrimSpace(result.Backup.ID)}
}

func markReportItem(report *selectedpreview.Report, settingRef string, item v2ledger.ItemRecord) {
	if report == nil {
		return
	}
	for idx := range report.Items {
		if report.Items[idx].SettingRef != settingRef {
			continue
		}
		report.Items[idx].Mutated = item.Result == v2ledger.ItemResultVerified && item.Operation != "" && !sameState(item.Before, item.VerifiedState)
		report.Items[idx].PlannedAction = string(item.Result)
		report.Items[idx].Mutation = &selectedpreview.MutationInfo{
			Result:     string(item.Result),
			RunID:      runIDFromRunRecordRef(item.ArtifactRefs.RunRecord),
			LedgerRef:  item.ArtifactRefs.Ledger,
			BackupRefs: append([]string(nil), item.BackupRefs...),
			Verification: selectedpreview.VerificationInfo{
				Verified: item.Verification.Verified,
				Result:   item.Verification.Result,
				Message:  item.Verification.Message,
			},
			ArtifactRefs: selectedpreview.MutationRefs{
				RunRecord:     item.ArtifactRefs.RunRecord,
				Ledger:        item.ArtifactRefs.Ledger,
				Backup:        item.ArtifactRefs.Backup,
				BackupPayload: item.ArtifactRefs.BackupPayload,
			},
		}
		for _, diagnostic := range item.Diagnostics {
			report.Items[idx].Diagnostics = append(report.Items[idx].Diagnostics, selectedpreview.Diagnostic{Code: diagnostic.Code, Severity: selectedpreview.SeverityError, Message: diagnostic.Message, Ref: settingRef, Path: diagnostic.Path})
		}
		return
	}
}

func runIDFromRunRecordRef(ref string) string {
	return strings.TrimPrefix(strings.TrimSpace(ref), "state://ledger/runs/")
}

func finishLiveReport(report *selectedpreview.Report, record v2ledger.RunRecord) {
	report.Summary.Changed = 0
	report.Summary.Blocked = 0
	report.Summary.Applied = 0
	report.Summary.Saved = 0
	report.Summary.Skipped = record.Summary.Skipped
	report.Summary.Failed = record.Summary.Failed
	for _, item := range record.Items {
		switch item.Result {
		case v2ledger.ItemResultVerified:
			report.Summary.Changed++
			if item.Operation == selectedpreview.CommandApply {
				report.Summary.Applied++
			}
			if item.Operation == selectedpreview.CommandSave {
				report.Summary.Saved++
			}
		case v2ledger.ItemResultFailed:
			report.Summary.Blocked++
		}
	}
	switch {
	case record.Summary.Failed > 0:
		report.Summary.Status = selectedpreview.SummaryError
	case report.Summary.Changed > 0:
		report.Summary.Status = selectedpreview.SummaryChanged
	default:
		report.Summary.Status = selectedpreview.SummaryOK
	}
}

func itemReportMap(report *selectedpreview.Report) map[string]selectedpreview.Item {
	out := map[string]selectedpreview.Item{}
	if report == nil {
		return out
	}
	for _, item := range report.Items {
		out[item.SettingRef] = item
	}
	return out
}

func hasPlanBlocker(report *selectedpreview.Report) bool {
	if report == nil {
		return true
	}
	for _, item := range report.Items {
		switch item.State {
		case v2status.StateBlockedSafety, v2status.StateBlockedLifecycle, v2status.StateUnsupported:
			return true
		}
		if strings.HasPrefix(item.PlannedAction, "blocked-") {
			return true
		}
	}
	return false
}

func requiresConfirmation(report *selectedpreview.Report) bool {
	if report == nil {
		return false
	}
	for _, item := range report.Items {
		if item.PlannedAction == "would-save" || item.PlannedAction == "would-apply" {
			return true
		}
	}
	return false
}

func isActionable(command string, item selectedpreview.Item) bool {
	switch command {
	case selectedpreview.CommandSave:
		return item.PlannedAction == "would-save"
	case selectedpreview.CommandApply:
		return item.PlannedAction == "would-apply"
	default:
		return false
	}
}

func attachReportError(report *selectedpreview.Report, code string, message string, details map[string]any) {
	if report == nil {
		return
	}
	report.Summary.Status = selectedpreview.SummaryError
	report.Error = &selectedpreview.ErrorObj{Code: code, Message: message, Details: details}
}

func resultForVerification(result v2ledger.ItemResult, verified bool) v2ledger.ItemResult {
	if !verified {
		return v2ledger.ItemResultFailed
	}
	return result
}

func verificationResult(verified bool) string {
	if verified {
		return "verified"
	}
	return "verification-failed"
}

func sameState(left v2ledger.NormalizedState, right v2ledger.NormalizedState) bool {
	return left.Exists == right.Exists && left.Hash == right.Hash && left.Normalizer == right.Normalizer
}

func normalizeCommand(command string) (string, error) {
	switch strings.TrimSpace(command) {
	case selectedpreview.CommandSave:
		return selectedpreview.CommandSave, nil
	case selectedpreview.CommandApply:
		return selectedpreview.CommandApply, nil
	default:
		return "", fmt.Errorf("unsupported selected-value live command: %s", command)
	}
}

type parsedRef struct {
	Target  string
	Setting string
	Empty   bool
}

func parseRef(raw string) (parsedRef, error) {
	ref := strings.TrimSpace(raw)
	if ref == "" {
		return parsedRef{Empty: true}, nil
	}
	if strings.Contains(ref, "://") || strings.ContainsAny(ref, "#/") {
		return parsedRef{}, fmt.Errorf("unsupported selected-value ref kind: %s", raw)
	}
	parts := strings.Split(ref, ":")
	if len(parts) == 1 {
		if err := recipe.ValidatePublicID("target", parts[0]); err != nil {
			return parsedRef{}, err
		}
		return parsedRef{Target: parts[0]}, nil
	}
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return parsedRef{}, fmt.Errorf("setting ref must be target:setting, got %q", raw)
	}
	if err := recipe.ValidatePublicID("target", parts[0]); err != nil {
		return parsedRef{}, err
	}
	if err := recipe.ValidatePublicID("setting", parts[1]); err != nil {
		return parsedRef{}, err
	}
	return parsedRef{Target: parts[0], Setting: parts[1]}, nil
}

func filterSettings(settings []resolution.ResolvedSetting, ref parsedRef) []resolution.ResolvedSetting {
	out := make([]resolution.ResolvedSetting, 0, len(settings))
	for _, setting := range settings {
		if !ref.Empty && setting.TargetID != ref.Target {
			continue
		}
		if ref.Setting != "" && setting.SettingID != ref.Setting {
			continue
		}
		out = append(out, setting)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ref() < out[j].Ref() })
	return out
}

func normalizeRepoRoot(repoRoot string) (string, error) {
	trimmed := strings.TrimSpace(repoRoot)
	if trimmed == "" {
		return "", fmt.Errorf("v2 repo root is required")
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("v2 repo root is not a directory: %s", abs)
	}
	return abs, nil
}

func normalizeStateRoot(repoRoot string, stateRoot string) (string, error) {
	if strings.TrimSpace(stateRoot) != "" {
		return filepath.Abs(stateRoot)
	}
	return v2ledger.DefaultStateRoot(repoRoot)
}

func runID(opts Options, started time.Time) string {
	if strings.TrimSpace(opts.RunID) != "" {
		return strings.TrimSpace(opts.RunID)
	}
	return "selected-value-" + started.UTC().Format("20060102T150405.000000000Z")
}

func clock(opts Options) func() time.Time {
	if opts.Now != nil {
		return opts.Now
	}
	return time.Now
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
