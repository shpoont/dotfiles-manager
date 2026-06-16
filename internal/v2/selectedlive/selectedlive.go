package selectedlive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/shpoont/dotfiles-manager/internal/v2/customfiles"
	"github.com/shpoont/dotfiles-manager/internal/v2/desired"
	"github.com/shpoont/dotfiles-manager/internal/v2/filedriver"
	v2ledger "github.com/shpoont/dotfiles-manager/internal/v2/ledger"
	"github.com/shpoont/dotfiles-manager/internal/v2/lifecycle"
	"github.com/shpoont/dotfiles-manager/internal/v2/macosdefaultsdriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/nativeapply"
	"github.com/shpoont/dotfiles-manager/internal/v2/nativeexport"
	"github.com/shpoont/dotfiles-manager/internal/v2/nativeops"
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
	Command             string
	ConfigPath          string
	RepoRoot            string
	StateRoot           string
	Ref                 string
	MachineID           string
	UserID              string
	ExtraLayers         []string
	DryRun              bool
	Confirmed           bool
	NonInteractive      bool
	RunID               string
	Now                 func() time.Time
	LocationRoots       map[string]map[string]string
	AfterApply          func(*selectedvalue.Plan) error
	MacOSDefaultsRunner macosdefaultsdriver.Runner
	NativeResolver      nativeops.ExecutableResolver
	NativeExecutor      nativeops.Executor
	LifecycleDetector   lifecycle.Detector
	LifecycleController lifecycle.Controller
	LifecyclePrompter   lifecycle.Prompter
	JSONMode            bool
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
		Command:             command,
		ConfigPath:          opts.ConfigPath,
		RepoRoot:            opts.RepoRoot,
		StateRoot:           opts.StateRoot,
		Ref:                 opts.Ref,
		MachineID:           opts.MachineID,
		UserID:              opts.UserID,
		ExtraLayers:         opts.ExtraLayers,
		DryRun:              opts.DryRun,
		LocationRoots:       opts.LocationRoots,
		MacOSDefaultsRunner: opts.MacOSDefaultsRunner,
		Confirmed:           opts.Confirmed,
		RunID:               opts.RunID,
		Now:                 opts.Now,
		NativeResolver:      opts.NativeResolver,
		NativeExecutor:      opts.NativeExecutor,
		LifecycleDetector:   opts.LifecycleDetector,
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
		return &Result{Report: report}, &selectedpreview.Error{Code: CodePlanBlocked, Message: "selected-value live write is blocked by the plan; no files were mutated", Exit: planBlockerExitCode(report)}
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
		rec, source, trustEval, trustContext, resourceID, resource, err := runtimeContext(repoRoot, stateRoot, setting, opts.Confirmed, command == selectedpreview.CommandApply)
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
		lifecycleBefore, lifecycleBlockedItem, lifecycleBlocked := evaluateLifecycleBeforeLive(command, runID, setting, rec, resourceID, resource, preItem, opts)
		if lifecycleBlocked {
			items = append(items, lifecycleBlockedItem)
			markReportItem(report, setting.Ref(), lifecycleBlockedItem)
			continue
		}
		_ = source
		if resource.Driver == recipe.FileDriverID || resource.Driver == recipe.FileTreeDriverID {
			item := executeFileResource(command, runID, started, store, profile, setting, rec, resourceID, resource, locationRoots, preItem)
			item = evaluateLifecycleAfterLive(command, setting, rec, resourceID, item, lifecycleBefore, opts)
			items = append(items, item)
			markReportItem(report, setting.Ref(), item)
			continue
		}
		if resource.Driver == recipe.NativeExportDriverID {
			if command == selectedpreview.CommandApply {
				item := executeNativeApply(repoRoot, stateRoot, runID, started, store, setting, rec, source, trustEval, resourceID, resource, locationRoots, preItem, opts)
				item = evaluateLifecycleAfterLive(command, setting, rec, resourceID, item, lifecycleBefore, opts)
				items = append(items, item)
				markReportItem(report, setting.Ref(), item)
				continue
			}
			item := executeNativeExport(command, runID, setting, resourceID, resource, preItem)
			items = append(items, item)
			markReportItem(report, setting.Ref(), item)
			continue
		}
		switch command {
		case selectedpreview.CommandSave:
			item := executeSave(repoRoot, runID, setting, rec, trustContext, resourceID, resource, locationRoots, preItem)
			items = append(items, item)
			markReportItem(report, setting.Ref(), item)
		case selectedpreview.CommandApply:
			item := executeApply(repoRoot, runID, started, store, setting, rec, trustContext, resourceID, resource, locationRoots, preItem, opts.AfterApply)
			item = evaluateLifecycleAfterLive(command, setting, rec, resourceID, item, lifecycleBefore, opts)
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
		return &Result{Report: report, RunRecord: &record, LedgerEntries: entries, Backup: backup}, &selectedpreview.Error{Code: CodeExecutionFailed, Message: "selected-value live write completed with failed items", Exit: executionFailedExitCode(record)}
	}
	return &Result{Report: report, RunRecord: &record, LedgerEntries: entries, Backup: backup}, nil
}

func executeSave(repoRoot string, runID string, setting resolution.ResolvedSetting, rec *recipe.Recipe, trustContext recipe.WriteSafetyContext, resourceID string, resource recipe.Resource, roots map[string]string, preItem selectedpreview.Item) v2ledger.ItemRecord {
	if resource.Driver == recipe.MacOSDefaultsReadOnlyDriverID {
		return failedItemRecord(selectedpreview.CommandSave, runID, setting, resourceID, resource, preItem, readOnlyDiagnostic(preItem), nil)
	}
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
	if resource.Driver == recipe.MacOSDefaultsReadOnlyDriverID {
		return failedItemRecord(selectedpreview.CommandApply, runID, setting, resourceID, resource, preItem, readOnlyDiagnostic(preItem), nil)
	}
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

func executeFileResource(command string, runID string, started time.Time, store *v2ledger.Store, profile *resolution.ResolvedProfile, setting resolution.ResolvedSetting, rec *recipe.Recipe, resourceID string, resource recipe.Resource, roots map[string]string, preItem selectedpreview.Item) v2ledger.ItemRecord {
	req := customfiles.Request{Profile: profile, Recipe: rec, SettingRef: setting.Ref(), LocationRoots: roots}
	var plan *customfiles.Plan
	var err error
	switch command {
	case selectedpreview.CommandSave:
		plan, err = customfiles.PlanFileSave(req)
	case selectedpreview.CommandApply:
		plan, err = customfiles.PlanFileApply(req)
	default:
		err = fmt.Errorf("unsupported file-resource live command: %s", command)
	}
	if err != nil {
		return failedItemRecord(command, runID, setting, resourceID, resource, preItem, customfilesPlanDiagnostic(err, preItem), nil)
	}

	execOpts := customfiles.ExecuteOptions{}
	if command == selectedpreview.CommandApply {
		execOpts.BackupHook = fileResourceBackupHook(store, runID, started, plan)
	}
	result, err := customfiles.Execute(plan, execOpts)
	if err != nil {
		item := v2ledger.BuildCustomFilesItemRecord(plan, nil, err, runID)
		return item
	}
	item := v2ledger.BuildCustomFilesItemRecord(plan, result, nil, runID)
	if item.Result == v2ledger.ItemResultVerified && !result.Mutated {
		item.Result = v2ledger.ItemResultUnchanged
	}
	return v2ledger.NormalizeItemRecord(item)
}

func executeNativeExport(command string, runID string, setting resolution.ResolvedSetting, resourceID string, resource recipe.Resource, preItem selectedpreview.Item) v2ledger.ItemRecord {
	if command != selectedpreview.CommandSave {
		return failedNativeExportItemRecord(command, runID, setting, resourceID, resource, preItem, v2ledger.Diagnostic{Code: "selectedlive.nativeExport.applyUnsupported", Message: "native import/apply is not implemented in this tranche"})
	}
	if preItem.NativeExport == nil || strings.TrimSpace(preItem.NativeExport.StagingRoot) == "" {
		return failedNativeExportItemRecord(command, runID, setting, resourceID, resource, preItem, v2ledger.Diagnostic{Code: "selectedlive.nativeExport.missingStaging", Message: "native export staging artifact is missing"})
	}
	expected := nativeexport.Expected(nativeexport.Options{Setting: setting, ResourceID: resourceID, Resource: resource, Recipe: &recipe.Recipe{NativeOperations: map[string]recipe.NativeOperation{resource.NativeOperation: {ArtifactForm: preItem.NativeExport.ArtifactForm}}}})
	beforeRead := nativeexport.ReadDesired(setting.DesiredPath, expected)
	if beforeRead.Status != "missing" && beforeRead.Status != "present" {
		return failedNativeExportItemRecord(command, runID, setting, resourceID, resource, preItem, v2ledger.Diagnostic{Code: beforeRead.Diagnostic.Code, Message: beforeRead.Diagnostic.Message, Path: beforeRead.Diagnostic.Path})
	}
	before := nativeState(nativeexport.Snapshot(beforeRead.Metadata))
	if err := nativeexport.WriteDesired(setting.DesiredPath, preItem.NativeExport.StagingRoot, expected); err != nil {
		return failedNativeExportItemRecord(command, runID, setting, resourceID, resource, preItem, v2ledger.Diagnostic{Code: "selectedlive.nativeExport.writeDesired", Message: "native export desired artifact write failed", Path: setting.DesiredRelPath})
	}
	verifyRead := nativeexport.ReadDesired(setting.DesiredPath, expected)
	if verifyRead.Status != "present" || verifyRead.Metadata == nil {
		return failedNativeExportItemRecord(command, runID, setting, resourceID, resource, preItem, v2ledger.Diagnostic{Code: "selectedlive.nativeExport.verifyDesired", Message: "native export desired artifact verification failed", Path: setting.DesiredRelPath})
	}
	desiredState := nativeState(verifyRead.Metadata.Payload)
	verified := sameState(desiredState, nativeStateFromPreview(preItem.Current))
	result := v2ledger.ItemResultVerified
	if sameState(before, desiredState) {
		result = v2ledger.ItemResultUnchanged
	}
	return v2ledger.NormalizeItemRecord(v2ledger.ItemRecord{
		TargetRef:      setting.TargetID,
		SettingRef:     setting.Ref(),
		Operation:      selectedpreview.CommandSave,
		ResourceID:     resourceID,
		Driver:         resource.Driver,
		DriverVersion:  nativeexport.DriverVersion,
		DesiredURI:     setting.DesiredURI,
		DesiredRelPath: setting.DesiredRelPath,
		DesiredPath:    setting.DesiredPath,
		ArtifactRefs:   selectedValueArtifactRefs(runID, setting, ""),
		Before:         before,
		Desired:        desiredState,
		VerifiedState:  desiredState,
		Verification:   v2ledger.Verification{Verified: verified, Result: verificationResult(verified)},
		Result:         resultForVerification(result, verified),
	})
}

func executeNativeApply(repoRoot string, stateRoot string, runID string, started time.Time, store *v2ledger.Store, setting resolution.ResolvedSetting, rec *recipe.Recipe, source string, trustEval recipe.TrustEvaluation, resourceID string, resource recipe.Resource, roots map[string]string, preItem selectedpreview.Item, opts Options) v2ledger.ItemRecord {
	applyOpts := nativeapply.Options{
		RepoRoot:           repoRoot,
		StateRoot:          stateRoot,
		Recipe:             rec,
		RecipeSource:       source,
		TrustEvaluation:    &trustEval,
		Setting:            setting,
		ResourceID:         resourceID,
		Resource:           resource,
		MachineID:          opts.MachineID,
		UserID:             opts.UserID,
		RunID:              runID,
		LocationRoots:      roots,
		Now:                opts.Now,
		ExecutableResolver: opts.NativeResolver,
		Executor:           opts.NativeExecutor,
	}
	plan, err := nativeapply.BuildPlan(applyOpts)
	if err != nil || plan.Status != nativeapply.StatusReady {
		return failedNativeApplyItemRecord(runID, setting, resourceID, resource, preItem, plan, v2ledger.NormalizedState{}, nil, nativeApplyPlanDiagnostic(plan, err))
	}
	desiredState := nativeState(plan.DesiredSummary)
	input, err := nativeapply.PrepareDesiredInput(applyOpts, plan)
	if err != nil {
		return failedNativeApplyItemRecord(runID, setting, resourceID, resource, preItem, plan, v2ledger.NormalizedState{}, nil, v2ledger.Diagnostic{Code: "selectedlive.nativeApply.prepareInput", Message: "native apply could not prepare a manager-owned import input copy", Path: setting.DesiredRelPath})
	}
	defer func() { _ = os.RemoveAll(input.Root) }()

	exportLimits := nativeexport.EffectiveLimits(rec.NativeOperations[resource.NativeOperation])
	backupExport, err := nativeexport.Export(context.Background(), nativeExportOptionsFromApply(applyOpts, runID+"-backup"))
	if err != nil || backupExport.Status != nativeexport.StatusSucceeded {
		return failedNativeApplyItemRecord(runID, setting, resourceID, resource, preItem, plan, v2ledger.NormalizedState{}, nil, nativeExportLiveDiagnostic("selectedlive.nativeApply.backupExport", "native apply pre-apply backup export failed; import was not run", backupExport, err))
	}
	if _, err := nativeexport.ValidatePayload(backupExport.PayloadRoot, backupExport.Metadata.Payload, exportLimits); err != nil {
		return failedNativeApplyItemRecord(runID, setting, resourceID, resource, preItem, plan, v2ledger.NormalizedState{}, nil, v2ledger.Diagnostic{Code: "selectedlive.nativeApply.backupPayloadInvalid", Message: "native apply pre-apply backup payload validation failed; import was not run", Path: setting.Ref()})
	}
	beforeState := nativeState(backupExport.Metadata.Payload)
	backupItem, err := store.WriteNativeExportBackup(runID, started, v2ledger.NativeExportBackupRequest{
		RepoRoot:     repoRoot,
		TargetRef:    setting.TargetID,
		SettingRef:   setting.Ref(),
		ResourceID:   resourceID,
		StagingRoot:  backupExport.StagingRoot,
		Expected:     plan.Expected,
		Before:       beforeState,
		OperationID:  resource.NativeOperation,
		ArtifactForm: plan.ArtifactForm,
	})
	if err != nil {
		return failedNativeApplyItemRecord(runID, setting, resourceID, resource, preItem, plan, beforeState, nil, v2ledger.Diagnostic{Code: "selectedlive.nativeApply.backupRecord", Message: "native apply pre-apply backup could not be recorded; import was not run", Path: setting.Ref()})
	}
	backupRefs := []string{backupItem.Ref}

	importResult, err := nativeapply.RunImport(context.Background(), applyOpts, plan, input)
	if err != nil || importResult.Status != nativeops.StatusSucceeded {
		return failedNativeApplyItemRecord(runID, setting, resourceID, resource, preItem, plan, beforeState, backupRefs, nativeImportLiveDiagnostic(importResult, err))
	}

	verifyExport, err := nativeexport.Export(context.Background(), nativeExportOptionsFromApply(applyOpts, runID+"-verify"))
	if err != nil || verifyExport.Status != nativeexport.StatusSucceeded {
		return failedNativeApplyItemRecord(runID, setting, resourceID, resource, preItem, plan, beforeState, backupRefs, nativeExportLiveDiagnostic("selectedlive.nativeApply.verifyExport", "native apply post-import verification export failed after import", verifyExport, err))
	}
	if _, err := nativeexport.ValidatePayload(verifyExport.PayloadRoot, verifyExport.Metadata.Payload, exportLimits); err != nil {
		return failedNativeApplyItemRecord(runID, setting, resourceID, resource, preItem, plan, beforeState, backupRefs, v2ledger.Diagnostic{Code: "selectedlive.nativeApply.verifyPayloadInvalid", Message: "native apply post-import verification payload validation failed after import", Path: setting.Ref()})
	}
	if err := nativeapply.VerifyPostImport(plan.DesiredSummary, verifyExport.Metadata.Payload); err != nil {
		return failedNativeApplyItemRecord(runID, setting, resourceID, resource, preItem, plan, beforeState, backupRefs, v2ledger.Diagnostic{Code: "selectedlive.nativeApply.verifyHashMismatch", Message: "native apply post-import export hash does not match desired artifact", Path: setting.Ref()})
	}

	verifiedState := nativeState(verifyExport.Metadata.Payload)
	itemResult := v2ledger.ItemResultVerified
	if sameState(beforeState, desiredState) {
		itemResult = v2ledger.ItemResultUnchanged
	}
	refs := withBackupRefs(selectedValueArtifactRefs(runID, setting, ""), backupRefs)
	return v2ledger.NormalizeItemRecord(v2ledger.ItemRecord{
		TargetRef:      setting.TargetID,
		SettingRef:     setting.Ref(),
		Operation:      selectedpreview.CommandApply,
		ResourceID:     resourceID,
		Driver:         recipe.NativeExportDriverID,
		DriverVersion:  nativeexport.DriverVersion,
		DesiredURI:     setting.DesiredURI,
		DesiredRelPath: setting.DesiredRelPath,
		DesiredPath:    setting.DesiredPath,
		ArtifactRefs:   refs,
		Before:         beforeState,
		Desired:        desiredState,
		VerifiedState:  verifiedState,
		BackupRefs:     backupRefs,
		Verification:   v2ledger.Verification{Verified: true, Result: verificationResult(true)},
		Result:         itemResult,
	})
}

func evaluateLifecycleBeforeLive(command string, runID string, setting resolution.ResolvedSetting, rec *recipe.Recipe, resourceID string, resource recipe.Resource, preItem selectedpreview.Item, opts Options) (lifecycle.Decision, v2ledger.ItemRecord, bool) {
	if command != selectedpreview.CommandApply {
		return lifecycle.Decision{}, v2ledger.ItemRecord{}, false
	}
	decision := lifecycle.EvaluateBefore(context.Background(), lifecycle.Request{
		Recipe:            rec,
		SettingID:         setting.SettingID,
		SettingRef:        setting.Ref(),
		ResourceID:        resourceID,
		NativeOperationID: lifecycleNativeOperationID(resource, command),
		Command:           command,
		DryRun:            false,
		Confirmed:         opts.Confirmed,
		NonInteractive:    opts.NonInteractive || opts.JSONMode,
		Detector:          opts.LifecycleDetector,
		Controller:        opts.LifecycleController,
		Prompter:          opts.LifecyclePrompter,
	})
	if !decision.Blocked {
		return decision, v2ledger.ItemRecord{}, false
	}
	diagnostic := lifecycleDecisionDiagnostic(decision, "selectedlive.lifecycle.blocked")
	item := failedItemRecord(command, runID, setting, resourceID, resource, preItem, diagnostic, nil)
	item.Lifecycle = append(item.Lifecycle, decision.Actions...)
	return decision, item, true
}

func evaluateLifecycleAfterLive(command string, setting resolution.ResolvedSetting, rec *recipe.Recipe, resourceID string, item v2ledger.ItemRecord, before lifecycle.Decision, opts Options) v2ledger.ItemRecord {
	if command != selectedpreview.CommandApply || !before.ManagerStopped {
		item.Lifecycle = append(item.Lifecycle, before.Actions...)
		return v2ledger.NormalizeItemRecord(item)
	}
	after := lifecycle.EvaluateAfter(context.Background(), lifecycle.Request{
		Recipe:            rec,
		SettingID:         setting.SettingID,
		SettingRef:        setting.Ref(),
		ResourceID:        resourceID,
		NativeOperationID: lifecycleNativeOperationID(rec.Resources[resourceID], command),
		Command:           command,
		DryRun:            false,
		Confirmed:         opts.Confirmed,
		NonInteractive:    opts.NonInteractive || opts.JSONMode,
		Detector:          opts.LifecycleDetector,
		Controller:        opts.LifecycleController,
		Prompter:          opts.LifecyclePrompter,
	}, before.ManagerStopped)
	item.Lifecycle = append(item.Lifecycle, before.Actions...)
	item.Lifecycle = append(item.Lifecycle, after.Actions...)
	if after.Blocked {
		diagnostic := lifecycleDecisionDiagnostic(after, "selectedlive.lifecycle.afterWrite")
		item.Diagnostics = append(item.Diagnostics, diagnostic)
		item.Result = v2ledger.ItemResultFailed
		if item.Verification.Result == "" {
			item.Verification.Result = "failed"
		}
		if item.Verification.Message == "" {
			item.Verification.Message = diagnostic.Message
		}
	}
	return v2ledger.NormalizeItemRecord(item)
}

func lifecycleNativeOperationID(resource recipe.Resource, command string) string {
	if command != selectedpreview.CommandApply || resource.Driver != recipe.NativeExportDriverID {
		return ""
	}
	return strings.TrimSpace(resource.NativeImportOperation)
}

func lifecycleDecisionDiagnostic(decision lifecycle.Decision, fallbackCode string) v2ledger.Diagnostic {
	code := strings.TrimSpace(decision.DiagnosticCode)
	message := strings.TrimSpace(decision.Message)
	for _, action := range decision.Actions {
		if code == "" {
			code = strings.TrimSpace(action.Code)
		}
		if message == "" {
			message = strings.TrimSpace(action.Message)
		}
	}
	if code == "" {
		code = fallbackCode
	}
	if message == "" {
		message = "lifecycle policy blocked live apply"
	}
	return v2ledger.Diagnostic{Code: code, Message: message}
}

func fileResourceBackupHook(store *v2ledger.Store, runID string, started time.Time, plan *customfiles.Plan) customfiles.BackupHook {
	return func(req customfiles.BackupRequest) (customfiles.BackupResult, error) {
		item, err := store.WriteCustomFilesBackup(runID, started, plan, req)
		if err != nil {
			return customfiles.BackupResult{}, err
		}
		return customfiles.BackupResult{ID: item.Ref, Before: req.Before.Snapshot(), TreeBefore: req.TreeBefore.Snapshot()}, nil
	}
}

func customfilesPlanDiagnostic(err error, preItem selectedpreview.Item) v2ledger.Diagnostic {
	var planErr *customfiles.PlanError
	if errors.As(err, &planErr) && len(planErr.Diagnostics) > 0 {
		diagnostic := planErr.Diagnostics[0]
		return v2ledger.Diagnostic{Code: diagnostic.Code, Message: diagnostic.Message, Path: defaultString(diagnostic.Path, preItem.Resource.Path)}
	}
	return v2ledger.Diagnostic{Code: "selectedlive.fileResource.plan", Message: err.Error(), Path: preItem.Resource.Path}
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

func runtimeContext(repoRoot string, stateRoot string, setting resolution.ResolvedSetting, allowNativeOpaque bool, handlesLifecycleActions ...bool) (*recipe.Recipe, string, recipe.TrustEvaluation, recipe.WriteSafetyContext, string, recipe.Resource, error) {
	runtime, err := recipe.LoadRuntime(repoRoot, setting.TargetID)
	if err != nil {
		return nil, runtime.Source, recipe.TrustEvaluation{}, recipe.WriteSafetyContext{}, "", recipe.Resource{}, err
	}
	rec := runtime.Recipe
	resourceID, resource, err := rec.ResourceForSetting(setting.SettingID)
	if err != nil {
		return rec, runtime.Source, recipe.TrustEvaluation{}, recipe.WriteSafetyContext{}, "", recipe.Resource{}, err
	}
	eval, err := recipe.EvaluateRecipeTrust(repoRoot, stateRoot, runtime.Source, rec)
	if err != nil {
		return rec, runtime.Source, eval, recipe.WriteSafetyContext{}, resourceID, resource, err
	}
	if eval.Status != recipe.TrustStatusTrusted {
		return rec, runtime.Source, eval, recipe.WriteSafetyContext{}, resourceID, resource, fmt.Errorf("recipe trust is not trusted")
	}
	ctx := eval.WriteSafetyContext(recipe.WriteSafetyContext{})
	if len(handlesLifecycleActions) > 0 && handlesLifecycleActions[0] {
		ctx.HandlesLifecycleActions = true
	}
	if resource.Driver == recipe.NativeExportDriverID && allowNativeOpaque {
		ctx.AllowOpaque = true
	}
	if err := rec.ValidateWriteSafety(ctx); err != nil {
		return rec, runtime.Source, eval, ctx, resourceID, resource, err
	}
	return rec, runtime.Source, eval, ctx, resourceID, resource, nil
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

func failedNativeExportItemRecord(command string, runID string, setting resolution.ResolvedSetting, resourceID string, resource recipe.Resource, preItem selectedpreview.Item, diagnostic v2ledger.Diagnostic) v2ledger.ItemRecord {
	if diagnostic.Code == "" {
		diagnostic.Code = "selectedlive.nativeExport.failed"
	}
	if diagnostic.Message == "" {
		diagnostic.Message = "native export live save failed"
	}
	return v2ledger.NormalizeItemRecord(v2ledger.ItemRecord{
		TargetRef:      setting.TargetID,
		SettingRef:     setting.Ref(),
		Operation:      command,
		ResourceID:     resourceID,
		Driver:         defaultString(resource.Driver, recipe.NativeExportDriverID),
		DriverVersion:  nativeexport.DriverVersion,
		DesiredURI:     setting.DesiredURI,
		DesiredRelPath: setting.DesiredRelPath,
		DesiredPath:    setting.DesiredPath,
		ArtifactRefs:   selectedValueArtifactRefs(runID, setting, ""),
		Before:         nativeStateFromPreview(preItem.Desired.Snapshot),
		Desired:        nativeStateFromPreview(preItem.Current),
		VerifiedState:  v2ledger.NormalizedState{DriverVersion: nativeexport.DriverVersion, Normalizer: nativeexport.Normalizer},
		Verification:   v2ledger.Verification{Verified: false, Result: "failed", Message: diagnostic.Message},
		Result:         v2ledger.ItemResultFailed,
		Diagnostics:    []v2ledger.Diagnostic{diagnostic},
	})
}

func failedNativeApplyItemRecord(runID string, setting resolution.ResolvedSetting, resourceID string, resource recipe.Resource, preItem selectedpreview.Item, plan nativeapply.Plan, before v2ledger.NormalizedState, backupRefs []string, diagnostic v2ledger.Diagnostic) v2ledger.ItemRecord {
	if diagnostic.Code == "" {
		diagnostic.Code = "selectedlive.nativeApply.failed"
	}
	if diagnostic.Message == "" {
		diagnostic.Message = "native apply failed"
	}
	if before.DriverVersion == "" {
		before = nativeStateFromPreview(preItem.Current)
	}
	if before.DriverVersion == "" {
		before.DriverVersion = nativeexport.DriverVersion
	}
	if before.Normalizer == "" {
		before.Normalizer = nativeexport.Normalizer
	}
	desiredState := nativeState(plan.DesiredSummary)
	if desiredState.DriverVersion == "" || !desiredState.Exists {
		desiredState = nativeStateFromPreview(preItem.Desired.Snapshot)
	}
	if desiredState.DriverVersion == "" {
		desiredState.DriverVersion = nativeexport.DriverVersion
	}
	if desiredState.Normalizer == "" {
		desiredState.Normalizer = nativeexport.Normalizer
	}
	return v2ledger.NormalizeItemRecord(v2ledger.ItemRecord{
		TargetRef:      setting.TargetID,
		SettingRef:     setting.Ref(),
		Operation:      selectedpreview.CommandApply,
		ResourceID:     resourceID,
		Driver:         defaultString(resource.Driver, recipe.NativeExportDriverID),
		DriverVersion:  nativeexport.DriverVersion,
		DesiredURI:     setting.DesiredURI,
		DesiredRelPath: setting.DesiredRelPath,
		DesiredPath:    setting.DesiredPath,
		ArtifactRefs:   withBackupRefs(selectedValueArtifactRefs(runID, setting, ""), backupRefs),
		BackupRefs:     append([]string(nil), backupRefs...),
		Before:         before,
		Desired:        desiredState,
		VerifiedState:  v2ledger.NormalizedState{DriverVersion: nativeexport.DriverVersion, Normalizer: nativeexport.Normalizer},
		Verification:   v2ledger.Verification{Verified: false, Result: "failed", Message: diagnostic.Message},
		Result:         v2ledger.ItemResultFailed,
		Diagnostics:    []v2ledger.Diagnostic{diagnostic},
	})
}

func nativeExportOptionsFromApply(opts nativeapply.Options, runID string) nativeexport.Options {
	return nativeexport.Options{
		RepoRoot:           opts.RepoRoot,
		StateRoot:          opts.StateRoot,
		Recipe:             opts.Recipe,
		RecipeSource:       opts.RecipeSource,
		TrustEvaluation:    opts.TrustEvaluation,
		Setting:            opts.Setting,
		ResourceID:         opts.ResourceID,
		Resource:           opts.Resource,
		MachineID:          opts.MachineID,
		UserID:             opts.UserID,
		RunID:              runID,
		LocationRoots:      opts.LocationRoots,
		Now:                opts.Now,
		ExecutableResolver: opts.ExecutableResolver,
		Executor:           opts.Executor,
	}
}

func nativeApplyPlanDiagnostic(plan nativeapply.Plan, err error) v2ledger.Diagnostic {
	if plan.Diagnostic.Code != "" {
		return v2ledger.Diagnostic{Code: plan.Diagnostic.Code, Message: plan.Diagnostic.Message, Path: plan.Diagnostic.Path}
	}
	message := "native apply plan is blocked"
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		message = err.Error()
	}
	return v2ledger.Diagnostic{Code: "selectedlive.nativeApply.planBlocked", Message: message}
}

func nativeExportLiveDiagnostic(code string, fallback string, result nativeexport.Result, err error) v2ledger.Diagnostic {
	if result.Diagnostic.Code != "" {
		return v2ledger.Diagnostic{Code: result.Diagnostic.Code, Message: result.Diagnostic.Message, Path: result.Diagnostic.Path}
	}
	if strings.TrimSpace(fallback) == "" {
		fallback = "native export operation failed"
	}
	_ = err
	return v2ledger.Diagnostic{Code: code, Message: fallback}
}

func nativeImportLiveDiagnostic(result nativeops.Result, err error) v2ledger.Diagnostic {
	if len(result.Diagnostics) > 0 {
		diag := result.Diagnostics[0]
		if diag.Code != "" || diag.Message != "" {
			return v2ledger.Diagnostic{Code: defaultString(diag.Code, "selectedlive.nativeApply.import"), Message: defaultString(diag.Message, "native import operation failed")}
		}
	}
	message := "native import operation failed after backup"
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		message = err.Error()
	}
	return v2ledger.Diagnostic{Code: "selectedlive.nativeApply.import", Message: message}
}

func withBackupRefs(refs v2ledger.ArtifactRefs, backupRefs []string) v2ledger.ArtifactRefs {
	if len(backupRefs) == 0 {
		return refs
	}
	refs.Backup = backupRefs[0]
	refs.BackupPayload = strings.TrimRight(backupRefs[0], "/") + "/payload"
	return refs
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

func nativeState(summary nativeexport.PayloadSummary) v2ledger.NormalizedState {
	return v2ledger.NormalizedState{Exists: summary.Exists, Hash: summary.SHA256, Normalizer: summary.Normalizer, DriverVersion: nativeexport.DriverVersion, Size: int(summary.Size), EntryCount: summary.EntryCount, FileCount: summary.FileCount, DirCount: summary.DirCount}
}

func nativeStateFromPreview(snapshot selectedpreview.Snapshot) v2ledger.NormalizedState {
	return v2ledger.NormalizedState{Exists: snapshot.Exists, Hash: snapshot.SHA256, Normalizer: snapshot.Normalizer, DriverVersion: nativeexport.DriverVersion, Size: snapshot.Size, EntryCount: snapshot.EntryCount, FileCount: snapshot.FileCount, DirCount: snapshot.DirCount}
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

func readOnlyDiagnostic(item selectedpreview.Item) v2ledger.Diagnostic {
	return v2ledger.Diagnostic{Code: "selectedlive.driver.readOnly", Message: "macOS defaults selected values are read-only; live save/apply are unsupported", Path: item.Resource.Path}
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
		report.Items[idx].Mutated = item.Operation != "" && !sameState(item.Before, item.VerifiedState)
		report.Items[idx].PlannedAction = string(item.Result)
		report.Items[idx].Lifecycle = append(report.Items[idx].Lifecycle, item.Lifecycle...)
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
		if item.Driver == recipe.FileTreeDriverID && item.Operation == selectedpreview.CommandApply && item.Result == v2ledger.ItemResultVerified {
			selectedpreview.SetFileTreeOperationState(&report.Items[idx], selectedpreview.FileTreeOperationStateApplied)
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

func planBlockerExitCode(report *selectedpreview.Report) int {
	if report == nil {
		return 5
	}
	for _, item := range report.Items {
		switch item.State {
		case v2status.StateBlockedSafety, v2status.StateBlockedLifecycle:
			return 5
		}
		for _, diagnostic := range item.Diagnostics {
			code := diagnostic.Code
			if strings.Contains(code, "trust") ||
				strings.Contains(code, "safety") ||
				strings.Contains(code, "lifecycle") ||
				strings.Contains(code, "backup") ||
				strings.Contains(code, "verify") ||
				strings.Contains(code, "nativeApply") {
				return 5
			}
		}
	}
	return 2
}

func executionFailedExitCode(record v2ledger.RunRecord) int {
	for _, item := range record.Items {
		if item.Driver == recipe.NativeExportDriverID && item.Result == v2ledger.ItemResultFailed {
			return 5
		}
		for _, diagnostic := range item.Diagnostics {
			code := diagnostic.Code
			if strings.Contains(code, "trust") ||
				strings.Contains(code, "safety") ||
				strings.Contains(code, "lifecycle") ||
				strings.Contains(code, "backup") ||
				strings.Contains(code, "verify") ||
				strings.Contains(code, "import") ||
				strings.Contains(code, "nativeApply") {
				return 5
			}
		}
	}
	return 2
}

func requiresConfirmation(report *selectedpreview.Report) bool {
	if report == nil {
		return false
	}
	for _, item := range report.Items {
		if selectedpreview.IsSavePlannedAction(item.PlannedAction) || item.PlannedAction == selectedpreview.PlannedActionWouldApply {
			return true
		}
	}
	return false
}

func isActionable(command string, item selectedpreview.Item) bool {
	switch command {
	case selectedpreview.CommandSave:
		return selectedpreview.IsSavePlannedAction(item.PlannedAction)
	case selectedpreview.CommandApply:
		return item.PlannedAction == selectedpreview.PlannedActionWouldApply
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
