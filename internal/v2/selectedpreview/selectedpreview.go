package selectedpreview

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/shpoont/dotfiles-manager/internal/v2/desired"
	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/shpoont/dotfiles-manager/internal/v2/resolution"
	"github.com/shpoont/dotfiles-manager/internal/v2/selectedvalue"
	v2status "github.com/shpoont/dotfiles-manager/internal/v2/status"
)

const (
	Schema        = "dotfiles-manager.v2.preview"
	SchemaVersion = 1
	RunID         = "selected-value-preview"

	CommandStatus = "status"
	CommandDiff   = "diff"
	CommandSave   = "save"
	CommandApply  = "apply"

	SummaryOK      = "ok"
	SummaryChanged = "changed"
	SummaryBlocked = "blocked"
	SummaryError   = "error"

	SeverityError   = "error"
	SeverityWarning = "warning"
	SeverityInfo    = "info"
)

const (
	PlannedActionNone                  = "none"
	PlannedActionWouldSave             = "would-save"
	PlannedActionWouldPromote          = "would-promote"
	PlannedActionWouldApply            = "would-apply"
	PlannedActionBlockedMissingDesired = "blocked-missing-desired"
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
	LocationRoots map[string]map[string]string
}

type Report struct {
	Schema        string    `json:"schema"`
	SchemaVersion int       `json:"schemaVersion"`
	Command       string    `json:"command"`
	RunID         string    `json:"runId"`
	DryRun        bool      `json:"dryRun"`
	ProfileStack  []string  `json:"profileStack"`
	Summary       Summary   `json:"summary"`
	Items         []Item    `json:"items"`
	Error         *ErrorObj `json:"error,omitempty"`
}

type Summary struct {
	Status  string `json:"status"`
	Changed int    `json:"changed"`
	Blocked int    `json:"blocked"`
	Applied int    `json:"applied"`
	Saved   int    `json:"saved"`
	Skipped int    `json:"skipped"`
	Failed  int    `json:"failed"`
}

type ErrorObj struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type Item struct {
	TargetRef      string             `json:"targetRef"`
	SettingRef     string             `json:"settingRef"`
	Scope          string             `json:"scope"`
	Subject        string             `json:"subject"`
	SourceLayer    string             `json:"sourceLayer"`
	DesiredURI     string             `json:"desiredUri"`
	DesiredRelPath string             `json:"desiredRelPath"`
	Recipe         RecipeInfo         `json:"recipe"`
	Resource       ResourceInfo       `json:"resource"`
	Selector       SelectorInfo       `json:"selector"`
	Desired        DesiredInfo        `json:"desired"`
	Current        Snapshot           `json:"current"`
	Preview        *PreviewInfo       `json:"preview,omitempty"`
	Diff           *DiffInfo          `json:"diff,omitempty"`
	State          v2status.StateCode `json:"state"`
	NoBaseline     bool               `json:"noBaseline"`
	Message        string             `json:"message"`
	AllowedActions []v2status.Action  `json:"allowedActions"`
	PlannedAction  string             `json:"plannedAction,omitempty"`
	DryRun         bool               `json:"dryRun"`
	Mutated        bool               `json:"mutated"`
	Mutation       *MutationInfo      `json:"mutation,omitempty"`
	Diagnostics    []Diagnostic       `json:"diagnostics"`
}

type RecipeInfo struct {
	Source      string `json:"source"`
	RecipeRef   string `json:"recipeRef"`
	TrustStatus string `json:"trustStatus"`
}

type ResourceInfo struct {
	ID         string `json:"id"`
	DriverID   string `json:"driverId"`
	LocationID string `json:"locationId"`
	RelPath    string `json:"relPath"`
	Path       string `json:"path,omitempty"`
}

type SelectorInfo struct {
	Kind    string   `json:"kind"`
	Summary string   `json:"summary"`
	Section string   `json:"section,omitempty"`
	Key     string   `json:"key,omitempty"`
	Path    []string `json:"path,omitempty"`
}

type DesiredInfo struct {
	Status    string   `json:"status"`
	Intent    string   `json:"intent,omitempty"`
	Kind      string   `json:"kind,omitempty"`
	Snapshot  Snapshot `json:"snapshot"`
	Unmanaged bool     `json:"unmanaged"`
}

type Snapshot struct {
	Exists     bool   `json:"exists"`
	SHA256     string `json:"sha256,omitempty"`
	Normalizer string `json:"normalizer,omitempty"`
}

type PreviewInfo struct {
	ChangeKind string `json:"changeKind"`
	Intent     string `json:"intent,omitempty"`
}

type DiffInfo struct {
	Kind      string `json:"kind"`
	Mode      string `json:"mode"`
	Redaction string `json:"redaction"`
	Message   string `json:"message"`
}

type Diagnostic struct {
	Code       string `json:"code"`
	Severity   string `json:"severity"`
	Message    string `json:"message"`
	Ref        string `json:"ref,omitempty"`
	Source     string `json:"source,omitempty"`
	Path       string `json:"path,omitempty"`
	ResourceID string `json:"resourceId,omitempty"`
	DriverID   string `json:"driverId,omitempty"`
}

type MutationInfo struct {
	Result       string           `json:"result"`
	RunID        string           `json:"runId,omitempty"`
	LedgerRef    string           `json:"ledgerRef,omitempty"`
	BackupRefs   []string         `json:"backupRefs,omitempty"`
	Verification VerificationInfo `json:"verification"`
	ArtifactRefs MutationRefs     `json:"artifactRefs,omitempty"`
}

type VerificationInfo struct {
	Verified bool   `json:"verified"`
	Result   string `json:"result"`
	Message  string `json:"message,omitempty"`
}

type MutationRefs struct {
	RunRecord     string `json:"runRecord,omitempty"`
	Ledger        string `json:"ledger,omitempty"`
	Backup        string `json:"backup,omitempty"`
	BackupPayload string `json:"backupPayload,omitempty"`
}

type Error struct {
	Code    string
	Message string
	Exit    int
	Details map[string]any
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *Error) ExitCode() int {
	if e == nil || e.Exit == 0 {
		return 1
	}
	return e.Exit
}

func Build(opts Options) (*Report, error) {
	command, err := normalizeCommand(opts.Command)
	if err != nil {
		return errorReport(opts, "selectedpreview.command.invalid", err.Error(), nil), err
	}
	repoRoot, err := normalizeRepoRoot(opts.RepoRoot)
	if err != nil {
		return errorReport(opts, "selectedpreview.repo.invalid", err.Error(), nil), err
	}
	profile, err := resolution.Resolve(repoRoot, resolution.ResolveOptions{MachineID: opts.MachineID, UserID: opts.UserID, ExtraLayers: opts.ExtraLayers})
	if err != nil {
		wrapped := &Error{Code: "selectedpreview.profile.resolve", Message: err.Error(), Exit: 2}
		return errorReport(opts, wrapped.Code, wrapped.Message, nil), wrapped
	}

	ref, err := parseRef(opts.Ref)
	if err != nil {
		wrapped := &Error{Code: "selectedpreview.ref.invalid", Message: err.Error(), Exit: 2, Details: map[string]any{"ref": opts.Ref}}
		return errorReport(opts, wrapped.Code, wrapped.Message, wrapped.Details), wrapped
	}
	settings := filterSettings(profile.Settings, ref)
	if len(settings) == 0 {
		wrapped := &Error{Code: "selectedpreview.ref.notFound", Message: fmt.Sprintf("no selected settings match ref %q", opts.Ref), Exit: 2, Details: map[string]any{"ref": opts.Ref}}
		return errorReport(opts, wrapped.Code, wrapped.Message, wrapped.Details), wrapped
	}

	report := baseReport(command, commandDryRun(command, opts.DryRun), profile.Layers)
	for _, setting := range settings {
		report.Items = append(report.Items, buildItem(profile.RepoRoot, opts.StateRoot, command, report.DryRun, setting, opts.LocationRoots))
	}
	finishReport(report)
	return report, nil
}

func JSON(report *Report) (string, error) {
	if report == nil {
		report = baseReport(CommandStatus, false, nil)
		report.Summary.Status = SummaryError
	}
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	return string(payload) + "\n", nil
}

func ErrorReport(command string, dryRun bool, code string, message string, details map[string]any) *Report {
	report := baseReport(command, dryRun, nil)
	report.Summary.Status = SummaryError
	report.Error = &ErrorObj{Code: code, Message: message, Details: details}
	return report
}

func Text(report *Report) string {
	if report == nil {
		return "selected-value preview\nsummary status=error changed=0 blocked=0"
	}
	lines := []string{fmt.Sprintf("selected-value %s", report.Command)}
	if len(report.ProfileStack) > 0 {
		lines = append(lines, "profile: "+strings.Join(report.ProfileStack, " -> "))
	}
	if report.DryRun {
		lines = append(lines, "MODE: DRY RUN (no writes)")
	}
	if len(report.Items) == 0 {
		lines = append(lines, "items: none")
	}
	for _, item := range report.Items {
		line := fmt.Sprintf("  %s scope=%s subject=%s state=%s desired=%s current=%s", item.SettingRef, item.Scope, item.Subject, item.State, item.Desired.Status, existsLabel(item.Current.Exists))
		if item.PlannedAction != "" {
			line += " action=" + item.PlannedAction
		}
		if item.NoBaseline {
			line += " no-baseline"
		}
		lines = append(lines, line)
		if item.Resource.DriverID != "" {
			lines = append(lines, fmt.Sprintf("    resource=%s driver=%s selector=%s", item.Resource.ID, item.Resource.DriverID, item.Selector.Summary))
		}
		if item.Diff != nil {
			lines = append(lines, fmt.Sprintf("    diff=%s mode=%s redaction=%s", item.Diff.Kind, item.Diff.Mode, item.Diff.Redaction))
		}
		if item.Message != "" {
			lines = append(lines, "    message: "+item.Message)
		}
		if item.Mutation != nil {
			lines = append(lines, fmt.Sprintf("    mutation=%s verified=%t run=%s", item.Mutation.Result, item.Mutation.Verification.Verified, item.Mutation.RunID))
			if len(item.Mutation.BackupRefs) > 0 {
				lines = append(lines, "    backups="+strings.Join(item.Mutation.BackupRefs, ","))
			}
		}
		for _, diagnostic := range item.Diagnostics {
			lines = append(lines, fmt.Sprintf("    %s[%s]: %s", diagnostic.Severity, diagnostic.Code, diagnostic.Message))
		}
	}
	lines = append(lines, fmt.Sprintf("summary status=%s changed=%d blocked=%d saved=%d applied=%d", report.Summary.Status, report.Summary.Changed, report.Summary.Blocked, report.Summary.Saved, report.Summary.Applied))
	return strings.Join(lines, "\n")
}

func normalizeCommand(command string) (string, error) {
	switch strings.TrimSpace(command) {
	case CommandStatus:
		return CommandStatus, nil
	case CommandDiff:
		return CommandDiff, nil
	case CommandSave:
		return CommandSave, nil
	case CommandApply:
		return CommandApply, nil
	default:
		return "", fmt.Errorf("unsupported selected-value preview command: %s", command)
	}
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

func commandDryRun(command string, dryRun bool) bool {
	return dryRun
}

func baseReport(command string, dryRun bool, profileStack []string) *Report {
	return &Report{Schema: Schema, SchemaVersion: SchemaVersion, Command: command, RunID: RunID, DryRun: dryRun, ProfileStack: append([]string(nil), profileStack...), Summary: Summary{Status: SummaryOK}, Items: []Item{}}
}

func errorReport(opts Options, code string, message string, details map[string]any) *Report {
	command := strings.TrimSpace(opts.Command)
	if command == "" {
		command = CommandStatus
	}
	report := baseReport(command, opts.DryRun, nil)
	report.Summary.Status = SummaryError
	report.Error = &ErrorObj{Code: code, Message: message, Details: details}
	return report
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

func buildItem(repoRoot string, stateRoot string, command string, dryRun bool, setting resolution.ResolvedSetting, roots map[string]map[string]string) Item {
	item := Item{TargetRef: setting.TargetID, SettingRef: setting.Ref(), Scope: setting.Scope, Subject: setting.Subject, SourceLayer: setting.SourceLayer, DesiredURI: setting.DesiredURI, DesiredRelPath: filepath.ToSlash(setting.DesiredRelPath), State: v2status.StateUnknown, DryRun: dryRun, Mutated: false, Diagnostics: []Diagnostic{}}

	runtime, blocked := loadRuntimeRecipe(repoRoot, setting.TargetID)
	rec := runtime.Recipe
	item.Recipe.Source = runtime.Source
	item.Recipe.RecipeRef = runtime.RecipeRef
	item.Recipe.TrustStatus = runtime.TrustStatus
	if len(blocked) > 0 {
		for _, diagnostic := range blocked {
			item.Diagnostics = append(item.Diagnostics, diagnostic.withRef(item.SettingRef))
		}
		return finishBlocked(item, v2status.StateUnsupported, "Recipe runtime is not available for selected-value preview.")
	}

	resourceID, resource, err := rec.ResourceForSetting(setting.SettingID)
	if err != nil {
		item.Diagnostics = append(item.Diagnostics, diagnostic("selectedpreview.resource.unknown", SeverityError, err.Error(), item.SettingRef))
		return finishBlocked(item, v2status.StateUnsupported, "Selected setting is not supported by the recipe runtime.")
	}
	item.Resource = ResourceInfo{ID: resourceID, DriverID: resource.Driver, LocationID: resource.Location, RelPath: resource.Path}
	item.Selector = selectorInfo(selectedvalue.SelectorInfo{})
	if resource.Selector != nil {
		item.Selector = selectorFromRecipe(resource)
	}
	if !isSelectedValueDriver(resource.Driver) {
		item.Diagnostics = append(item.Diagnostics, Diagnostic{Code: "selectedpreview.driver.unsupported", Severity: SeverityError, Message: fmt.Sprintf("driver %s is not a selected-value driver", resource.Driver), Ref: item.SettingRef, ResourceID: resourceID, DriverID: resource.Driver})
		return finishBlocked(item, v2status.StateUnsupported, "Resource driver is not supported by selected-value preview.")
	}

	trustEval, trustContext := evaluateTrust(repoRoot, stateRoot, runtime.Source, rec)
	item.Recipe.TrustStatus = trustEval.Status
	if trustEval.Status != recipe.TrustStatusTrusted {
		for _, diagnostic := range trustEval.Diagnostics {
			item.Diagnostics = append(item.Diagnostics, fromRecipeDiagnostic(diagnostic, item.SettingRef, runtime.Source, resourceID, resource.Driver))
		}
		if len(item.Diagnostics) == 0 {
			item.Diagnostics = append(item.Diagnostics, diagnostic("selectedpreview.trust.required", SeverityError, "selected-value preview requires trusted recipe evidence before live reads", item.SettingRef))
		}
		return finishBlocked(item, v2status.StateBlockedSafety, "Recipe trust must be reviewed before selected-value preview can read live state.")
	}

	if err := rec.ValidateWriteSafety(trustContext); err != nil {
		for _, validation := range recipe.ValidationDiagnostics(err) {
			item.Diagnostics = append(item.Diagnostics, fromRecipeDiagnostic(validation, item.SettingRef, runtime.Source, resourceID, resource.Driver))
		}
		return finishBlocked(item, v2status.StateBlockedSafety, "Recipe write-safety metadata blocks selected-value preview.")
	}

	read, err := desired.ReadSelectedValueForSetting(repoRoot, setting)
	if err != nil {
		item.Diagnostics = append(item.Diagnostics, diagnostic("selectedpreview.desired.read", SeverityError, err.Error(), item.SettingRef))
		return finishBlocked(item, v2status.StateBlockedSafety, "Desired selected-value artifact could not be read safely.")
	}
	item.Desired.Status = read.Status
	item.Desired.Intent = read.Intent
	item.Desired.Kind = read.Kind
	item.Desired.Unmanaged = read.Status == desired.StatusUnmanaged

	locationRoots := roots[setting.TargetID]
	if locationRoots == nil {
		locationRoots = map[string]string{}
	}
	if read.Status == desired.StatusUnmanaged {
		item.State = v2status.StateUnchanged
		item.Message = "Setting is intentionally unmanaged in desired settings."
		return item
	}

	if read.Status == desired.StatusMissing {
		return buildMissingDesiredItem(repoRoot, item, rec, setting, locationRoots, command, trustContext)
	}

	if read.Desired == nil {
		item.Diagnostics = append(item.Diagnostics, diagnostic("selectedpreview.desired.invalid", SeverityError, "desired selected-value entry is present but has no normalized desired state", item.SettingRef))
		return finishBlocked(item, v2status.StateBlockedSafety, "Desired selected-value entry is invalid.")
	}
	if command == CommandApply {
		if err := validateExistingDesiredForPlanning(repoRoot, read, rec, setting, trustContext); err != nil {
			appendDesiredDiagnostics(&item, err)
			return finishBlocked(item, v2status.StateBlockedSafety, "Desired selected-value entry is blocked by write-safety policy.")
		}
	}
	if command == CommandSave {
		if err := validateCurrentForSavePlanning(repoRoot, rec, setting, locationRoots, trustContext); err != nil {
			if planErr, ok := err.(*selectedvalue.PlanError); ok {
				appendPlanDiagnostics(&item, &selectedvalue.Plan{Diagnostics: planErr.Diagnostics})
			} else {
				appendDesiredDiagnostics(&item, err)
			}
			return finishBlocked(item, v2status.StateBlockedSafety, "Current selected value is blocked by write-safety policy.")
		}
	}

	plan, err := selectedvalue.PlanPreview(selectedvalue.PreviewRequest{Request: selectedvalue.Request{Recipe: rec, SettingRef: setting.Ref(), LocationRoots: locationRoots}, Desired: *read.Desired, WriteSafetyContext: trustContext})
	if err != nil {
		appendPlanDiagnostics(&item, plan)
		return finishBlocked(item, v2status.StateBlockedSafety, "Selected-value driver preview is blocked.")
	}
	applyPlanToItem(&item, plan)
	deriveItemState(&item, command, read.Intent)
	item.PlannedAction = plannedAction(command, item)
	if command == CommandDiff {
		item.Diff = diffInfo(item.Preview.ChangeKind)
	}
	if command == CommandSave {
		item.Preview.ChangeKind = saveChangeKind(item.Current, item.Desired.Snapshot)
		item.Preview.Intent = saveIntent(item.Current)
	}
	return item
}

func loadRuntimeRecipe(repoRoot string, targetID string) (recipe.RuntimeRecipe, []Diagnostic) {
	runtime, err := recipe.LoadRuntime(repoRoot, targetID)
	if err != nil {
		code := "selectedpreview.recipe.notFound"
		switch {
		case errors.Is(err, recipe.ErrBundledRuntimeUnavailable):
			code = "selectedpreview.recipe.bundledRuntimeUnavailable"
		case !errors.Is(err, os.ErrNotExist):
			code = "selectedpreview.recipe.invalid"
		}
		if runtime.Source == "" {
			runtime.Source = recipe.RecipeSourceLocal
		}
		if runtime.RecipeRef == "" {
			runtime.RecipeRef = recipeRef(runtime.Source, targetID)
		}
		return runtime, []Diagnostic{diagnostic(code, SeverityError, err.Error(), targetID)}
	}
	return runtime, nil
}

func recipeRef(source string, targetID string) string {
	switch source {
	case recipe.RecipeSourceBundled:
		return "recipe://bundled/" + targetID
	case recipe.RecipeSourceLocal:
		return "recipe://local/" + targetID
	default:
		return ""
	}
}

func evaluateTrust(repoRoot string, stateRoot string, source string, rec *recipe.Recipe) (recipe.TrustEvaluation, recipe.WriteSafetyContext) {
	eval, err := recipe.EvaluateRecipeTrust(repoRoot, stateRoot, source, rec)
	if err != nil {
		return recipe.TrustEvaluation{Status: recipe.TrustStatusBlocked, Diagnostics: []recipe.ValidationDiagnostic{{Code: "selectedpreview.trust.evaluate", Severity: recipe.ValidationSeverityError, Message: err.Error(), Path: "$"}}}, recipe.WriteSafetyContext{}
	}
	return eval, eval.WriteSafetyContext(recipe.WriteSafetyContext{})
}

func buildMissingDesiredItem(repoRoot string, item Item, rec *recipe.Recipe, setting resolution.ResolvedSetting, roots map[string]string, command string, trustContext recipe.WriteSafetyContext) Item {
	if command == CommandApply {
		item.State = v2status.StateMissingDesired
		item.Message = "Selected setting has no desired artifact; apply dry-run cannot change live state."
		item.AllowedActions = []v2status.Action{v2status.ActionSave, v2status.ActionCreate}
		item.PlannedAction = PlannedActionBlockedMissingDesired
		return item
	}
	plan, err := selectedvalue.PlanRead(selectedvalue.Request{Recipe: rec, SettingRef: setting.Ref(), LocationRoots: roots})
	if err != nil {
		appendPlanDiagnostics(&item, plan)
		return finishBlocked(item, v2status.StateBlockedSafety, "Selected-value driver read is blocked.")
	}
	applyReadPlanToItem(&item, plan)
	if command == CommandSave {
		current, err := selectedvalue.ReadCurrentDesired(selectedvalue.Request{Recipe: rec, SettingRef: setting.Ref(), LocationRoots: roots})
		if err != nil {
			appendPlanDiagnostics(&item, current.Plan)
			return finishBlocked(item, v2status.StateBlockedSafety, "Selected-value driver read is blocked.")
		}
		saveValue, err := desiredValueFromSelected(current.Desired)
		if err != nil {
			item.Diagnostics = append(item.Diagnostics, diagnostic("selectedpreview.current.invalid", SeverityError, "current selected value cannot be represented as desired state", item.SettingRef))
			return finishBlocked(item, v2status.StateBlockedSafety, "Current selected value is invalid.")
		}
		if err := desired.ValidateSelectedValueWriteSafety(desired.WriteRequest{RepoRoot: repoRoot, URI: setting.DesiredURI, Value: saveValue, Safety: &desired.WriteSafetyDecision{Recipe: rec, SettingRef: setting.Ref(), Context: trustContext}}); err != nil {
			appendDesiredDiagnostics(&item, err)
			return finishBlocked(item, v2status.StateBlockedSafety, "Current selected value is blocked by write-safety policy.")
		}
	}
	stateItem := v2status.DeriveItem(v2status.Input{Context: statusContext(command), TargetRef: item.TargetRef, SettingRef: item.SettingRef, Desired: v2status.NormalizedState{Exists: false}, Current: normalizedSnapshot(item.Current)})
	item.State = stateItem.State
	item.NoBaseline = stateItem.NoBaseline
	item.Message = stateItem.Message
	item.AllowedActions = stateItem.Actions
	if command == CommandSave {
		item.PlannedAction = plannedSaveActionForMissingDesired(item)
		item.Preview = &PreviewInfo{ChangeKind: "create", Intent: saveIntent(item.Current)}
		if item.PlannedAction == PlannedActionWouldPromote {
			item.Message = "Existing live selected value can be promoted into desired state with save --yes; raw value remains redacted in output."
		}
	}
	if command == CommandDiff {
		item.Diff = diffInfo("missing-desired")
	}
	return item
}

func applyPlanToItem(item *Item, plan *selectedvalue.Plan) {
	if plan == nil {
		return
	}
	item.Resource.Path = plan.Path
	item.Selector = selectorInfo(plan.Selector)
	item.Current = fromSnapshot(plan.Current)
	if plan.Desired != nil {
		item.Desired.Snapshot = fromSnapshot(*plan.Desired)
	}
	item.Preview = &PreviewInfo{ChangeKind: plan.ChangeKind, Intent: plan.Intent}
	appendPlanDiagnostics(item, plan)
}

func applyReadPlanToItem(item *Item, plan *selectedvalue.Plan) {
	if plan == nil {
		return
	}
	item.Resource.Path = plan.Path
	item.Selector = selectorInfo(plan.Selector)
	item.Current = fromSnapshot(plan.Current)
	appendPlanDiagnostics(item, plan)
}

func appendPlanDiagnostics(item *Item, plan *selectedvalue.Plan) {
	if item == nil || plan == nil {
		return
	}
	for _, diagnostic := range plan.Diagnostics {
		item.Diagnostics = append(item.Diagnostics, Diagnostic{Code: diagnostic.Code, Severity: diagnostic.Severity, Message: diagnostic.Message, Ref: diagnostic.Ref, Path: diagnostic.Path, ResourceID: diagnostic.ResourceID, DriverID: diagnostic.DriverID})
	}
}

func appendDesiredDiagnostics(item *Item, err error) {
	if item == nil || err == nil {
		return
	}
	var safetyErr *desired.SafetyError
	if errors.As(err, &safetyErr) {
		for _, diagnostic := range safetyErr.Diagnostics {
			item.Diagnostics = append(item.Diagnostics, Diagnostic{Code: diagnostic.Code, Severity: diagnostic.Severity, Message: diagnostic.Message, Ref: item.SettingRef, Path: diagnostic.Path, ResourceID: item.Resource.ID, DriverID: item.Resource.DriverID})
		}
		return
	}
	item.Diagnostics = append(item.Diagnostics, Diagnostic{Code: "selectedpreview.desired.writeSafety", Severity: SeverityError, Message: "selected-value write-safety policy blocked planning", Ref: item.SettingRef, ResourceID: item.Resource.ID, DriverID: item.Resource.DriverID})
}

func validateExistingDesiredForPlanning(repoRoot string, read desired.ReadResult, rec *recipe.Recipe, setting resolution.ResolvedSetting, trustContext recipe.WriteSafetyContext) error {
	if read.Value == nil {
		return fmt.Errorf("desired selected-value entry is missing raw validation state")
	}
	return desired.ValidateSelectedValueWriteSafety(desired.WriteRequest{RepoRoot: repoRoot, URI: setting.DesiredURI, Value: *read.Value, Safety: &desired.WriteSafetyDecision{Recipe: rec, SettingRef: setting.Ref(), Context: trustContext}})
}

func validateCurrentForSavePlanning(repoRoot string, rec *recipe.Recipe, setting resolution.ResolvedSetting, roots map[string]string, trustContext recipe.WriteSafetyContext) error {
	current, err := selectedvalue.ReadCurrentDesired(selectedvalue.Request{Recipe: rec, SettingRef: setting.Ref(), LocationRoots: roots})
	if err != nil {
		if current != nil && current.Plan != nil {
			return &selectedvalue.PlanError{Diagnostics: current.Plan.Diagnostics}
		}
		return err
	}
	saveValue, err := desiredValueFromSelected(current.Desired)
	if err != nil {
		return err
	}
	return desired.ValidateSelectedValueWriteSafety(desired.WriteRequest{RepoRoot: repoRoot, URI: setting.DesiredURI, Value: saveValue, Safety: &desired.WriteSafetyDecision{Recipe: rec, SettingRef: setting.Ref(), Context: trustContext}})
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

func deriveItemState(item *Item, command string, desiredIntent string) {
	desiredState := normalizedSnapshot(item.Desired.Snapshot)
	currentState := normalizedSnapshot(item.Current)
	if desiredIntent == desired.IntentDelete {
		desiredState = deleteSentinel(item.Desired.Snapshot.Normalizer)
		if !item.Current.Exists {
			currentState = deleteSentinel(item.Desired.Snapshot.Normalizer)
		}
	}
	stateItem := v2status.DeriveItem(v2status.Input{Context: statusContext(command), TargetRef: item.TargetRef, SettingRef: item.SettingRef, Desired: desiredState, Current: currentState})
	item.State = stateItem.State
	item.NoBaseline = stateItem.NoBaseline
	item.Message = stateItem.Message
	item.AllowedActions = stateItem.Actions
}

func statusContext(command string) v2status.Context {
	switch command {
	case CommandSave:
		return v2status.ContextSave
	case CommandApply:
		return v2status.ContextApply
	default:
		return v2status.ContextStatus
	}
}

func normalizedSnapshot(snapshot Snapshot) v2status.NormalizedState {
	return v2status.NormalizedState{Exists: snapshot.Exists, Hash: snapshot.SHA256, Normalizer: snapshot.Normalizer}
}

func deleteSentinel(normalizer string) v2status.NormalizedState {
	if normalizer == "" {
		normalizer = "selected-value.delete.v1"
	}
	return v2status.NormalizedState{Exists: true, Hash: "selected-value-delete", Normalizer: normalizer}
}

func fromSnapshot(snapshot selectedvalue.Snapshot) Snapshot {
	return Snapshot{Exists: snapshot.Exists, SHA256: snapshot.SHA256, Normalizer: snapshot.Normalizer}
}

func selectorInfo(selector selectedvalue.SelectorInfo) SelectorInfo {
	return SelectorInfo{Kind: selector.Kind, Summary: selector.Summary, Section: selector.Section, Key: selector.Key, Path: append([]string(nil), selector.Path...)}
}

func selectorFromRecipe(resource recipe.Resource) SelectorInfo {
	if resource.Selector == nil {
		return SelectorInfo{Kind: "none"}
	}
	switch resource.Driver {
	case recipe.IniFileDriverID:
		return SelectorInfo{Kind: "ini-key", Summary: fmt.Sprintf("[%s] %s", resource.Selector.Section, resource.Selector.Key), Section: resource.Selector.Section, Key: resource.Selector.Key}
	case recipe.JSONFileDriverID, recipe.YAMLFileDriverID, recipe.TOMLFileDriverID:
		return SelectorInfo{Kind: "selected-path", Summary: strings.Join(resource.Selector.Path, "."), Path: append([]string(nil), resource.Selector.Path...)}
	case recipe.PlistFileDriverID:
		return SelectorInfo{Kind: "selected-path", Summary: quotedPathSummary(resource.Selector.Path), Path: append([]string(nil), resource.Selector.Path...)}
	default:
		return SelectorInfo{Kind: "unsupported"}
	}
}

func isSelectedValueDriver(driver string) bool {
	switch driver {
	case recipe.IniFileDriverID, recipe.JSONFileDriverID, recipe.YAMLFileDriverID, recipe.TOMLFileDriverID, recipe.PlistFileDriverID:
		return true
	default:
		return false
	}
}

func quotedPathSummary(path []string) string {
	data, err := json.Marshal(path)
	if err != nil {
		return fmt.Sprintf("%q", path)
	}
	return string(data)
}

func plannedAction(command string, item Item) string {
	if item.State == v2status.StateBlockedSafety || item.State == v2status.StateUnsupported || item.State == v2status.StateBlockedLifecycle {
		return ""
	}
	switch command {
	case CommandSave:
		if item.State == v2status.StateUnchanged {
			return PlannedActionNone
		}
		return PlannedActionWouldSave
	case CommandApply:
		if item.State == v2status.StateUnchanged {
			return PlannedActionNone
		}
		return PlannedActionWouldApply
	}
	return ""
}

func plannedSaveActionForMissingDesired(item Item) string {
	if item.Current.Exists {
		return PlannedActionWouldPromote
	}
	return PlannedActionWouldSave
}

func IsSavePlannedAction(action string) bool {
	return action == PlannedActionWouldSave || action == PlannedActionWouldPromote
}

func saveIntent(current Snapshot) string {
	if current.Exists {
		return desired.IntentSet
	}
	return desired.IntentDelete
}

func saveChangeKind(current Snapshot, desired Snapshot) string {
	switch {
	case current.Exists && !desired.Exists:
		return "create"
	case !current.Exists && desired.Exists:
		return "delete"
	case !current.Exists && !desired.Exists:
		return "unchanged"
	case current.SHA256 == desired.SHA256 && current.Normalizer == desired.Normalizer:
		return "unchanged"
	default:
		return "update"
	}
}

func diffInfo(kind string) *DiffInfo {
	if kind == "" {
		kind = "unknown"
	}
	return &DiffInfo{Kind: kind, Mode: "metadata-only", Redaction: "raw selected values omitted", Message: "Selected scalar diff is redacted; compare normalized metadata only."}
}

func finishBlocked(item Item, state v2status.StateCode, message string) Item {
	item.State = state
	item.Message = message
	item.AllowedActions = v2status.DeriveItem(v2status.Input{Context: v2status.ContextStatus, Desired: v2status.NormalizedState{Exists: true, Hash: "blocked", Normalizer: "blocked"}, Current: v2status.NormalizedState{Exists: true, Hash: "blocked", Normalizer: "blocked"}, Blocker: blockerForState(state, message)}).Actions
	return item
}

func blockerForState(state v2status.StateCode, message string) v2status.Blocker {
	switch state {
	case v2status.StateUnsupported:
		return v2status.Blocker{Code: v2status.BlockerUnsupported, Message: message}
	case v2status.StateBlockedLifecycle:
		return v2status.Blocker{Code: v2status.BlockerLifecycle, Message: message}
	case v2status.StateBlockedSafety:
		return v2status.Blocker{Code: v2status.BlockerSafety, Message: message}
	default:
		return v2status.Blocker{Code: v2status.BlockerUnknown, Message: message}
	}
}

func finishReport(report *Report) {
	if report == nil {
		return
	}
	for _, item := range report.Items {
		switch item.State {
		case v2status.StateBlockedSafety, v2status.StateBlockedLifecycle, v2status.StateUnsupported:
			report.Summary.Blocked++
		case v2status.StateUnchanged:
		default:
			report.Summary.Changed++
		}
		if report.Command == CommandSave && IsSavePlannedAction(item.PlannedAction) {
			report.Summary.Saved++
		}
		if report.Command == CommandApply && item.PlannedAction == PlannedActionWouldApply {
			report.Summary.Applied++
		}
	}
	if report.Summary.Blocked > 0 {
		report.Summary.Status = SummaryBlocked
	} else if report.Summary.Changed > 0 || report.Summary.Saved > 0 || report.Summary.Applied > 0 {
		report.Summary.Status = SummaryChanged
	} else {
		report.Summary.Status = SummaryOK
	}
}

func diagnostic(code string, severity string, message string, ref string) Diagnostic {
	if severity == "" {
		severity = SeverityError
	}
	return Diagnostic{Code: code, Severity: severity, Message: message, Ref: ref}
}

func (d Diagnostic) withRef(ref string) Diagnostic {
	if d.Ref == "" {
		d.Ref = ref
	}
	return d
}

func fromRecipeDiagnostic(d recipe.ValidationDiagnostic, ref string, source string, resourceID string, driverID string) Diagnostic {
	severity := d.Severity
	if severity == "" {
		severity = SeverityError
	}
	return Diagnostic{Code: d.Code, Severity: severity, Message: d.Message, Ref: ref, Source: source, Path: d.Path, ResourceID: resourceID, DriverID: driverID}
}

func existsLabel(exists bool) string {
	if exists {
		return "present"
	}
	return "missing"
}
