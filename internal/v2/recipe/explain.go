package recipe

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	ExplainSchema  = "dotfiles-manager.v2.preview"
	ExplainCommand = "recipe.explain"
	ExplainRunID   = "recipe-explain"
)

const (
	ExplainCodeInvalidRef            = "invalid-ref"
	ExplainCodeUnsupportedRefKind    = "unsupported-ref-kind"
	ExplainCodeUnknownTarget         = "unknown-target"
	ExplainCodeInvalidRecipe         = "invalid-recipe"
	ExplainCodeMetadataRenderBlocked = "metadata-render-blocked"
	ExplainCodeInternalError         = "internal-error"
	ExplainCodeSelectionUnresolved   = "selection-unresolved"
	ExplainCodeLocalRecipeShadowed   = "local-recipe-shadowed"
	ExplainSeverityInfo              = "info"
	ExplainSeverityWarning           = "warning"
	ExplainSeverityError             = "error"
)

type ExplainOptions struct {
	Target   string
	RepoRoot string
}

type ExplainReport struct {
	Schema        string              `json:"schema"`
	SchemaVersion int                 `json:"schemaVersion"`
	Command       string              `json:"command"`
	RunID         string              `json:"runId"`
	Summary       ExplainSummary      `json:"summary"`
	Items         []any               `json:"items"`
	RecipeExplain RecipeExplain       `json:"recipeExplain"`
	Error         *ExplainErrorObject `json:"error,omitempty"`
}

type ExplainSummary struct {
	Status  string `json:"status"`
	Changed int    `json:"changed"`
	Blocked int    `json:"blocked"`
	Applied int    `json:"applied"`
	Saved   int    `json:"saved"`
	Skipped int    `json:"skipped"`
	Failed  int    `json:"failed"`
}

type RecipeExplain struct {
	Target           ExplainTarget       `json:"target"`
	Recipe           ExplainRecipeSource `json:"recipe"`
	Selection        ExplainSelection    `json:"selection"`
	Settings         []ExplainSetting    `json:"settings"`
	SettingGroups    []any               `json:"settingGroups"`
	Resources        []ExplainResource   `json:"resources"`
	Drivers          []ExplainDriver     `json:"drivers"`
	NativeOperations []any               `json:"nativeOperations"`
	Safety           ExplainSafety       `json:"safety"`
	Diagnostics      []ExplainDiagnostic `json:"diagnostics"`
}

type ExplainTarget struct {
	Ref             string `json:"ref"`
	DisplayName     string `json:"displayName"`
	SupportLevel    string `json:"supportLevel"`
	Capability      string `json:"capability"`
	PlatformSupport string `json:"platformSupport"`
}

type ExplainRecipeSource struct {
	Source      string `json:"source"`
	RecipeRef   string `json:"recipeRef"`
	TrustStatus string `json:"trustStatus"`
	Version     string `json:"version"`
}

type ExplainSelection struct {
	Status       string   `json:"status"`
	Reason       string   `json:"reason"`
	ProfileStack []string `json:"profileStack"`
}

type ExplainSetting struct {
	Ref              string   `json:"ref"`
	ID               string   `json:"id"`
	Label            string   `json:"label"`
	SupportLevel     string   `json:"supportLevel"`
	Capability       string   `json:"capability"`
	DefaultScope     string   `json:"defaultScope"`
	ArtifactForm     string   `json:"artifactForm"`
	SelectionStatus  string   `json:"selectionStatus"`
	Sensitivity      string   `json:"sensitivity"`
	Lifecycle        string   `json:"lifecycle"`
	ResourceID       string   `json:"resourceId"`
	Driver           string   `json:"driver"`
	DiffLimitations  []string `json:"diffLimitations"`
	ApplyLimitations []string `json:"applyLimitations"`
}

type ExplainResource struct {
	ID            string           `json:"id"`
	LocationID    string           `json:"locationId"`
	Path          string           `json:"path"`
	Selector      *ExplainSelector `json:"selector,omitempty"`
	DriverID      string           `json:"driverId"`
	SupportedOps  []string         `json:"supportedOperations"`
	BackupRestore string           `json:"backupRestore"`
	Normalization string           `json:"normalization"`
	DiffMode      string           `json:"diffMode"`
	Include       []string         `json:"include,omitempty"`
	Exclude       []string         `json:"exclude,omitempty"`
}

type ExplainSelector struct {
	Section         string   `json:"section,omitempty"`
	Key             string   `json:"key,omitempty"`
	Path            []string `json:"path,omitempty"`
	MissingSection  string   `json:"missingSection,omitempty"`
	MissingKey      string   `json:"missingKey,omitempty"`
	DuplicatePolicy string   `json:"duplicatePolicy,omitempty"`
	DeleteKey       string   `json:"deleteKey,omitempty"`
	Summary         string   `json:"summary"`
}

type ExplainDriver struct {
	ID          string   `json:"id"`
	Summary     string   `json:"summary"`
	Operations  []string `json:"operations"`
	Limitations []string `json:"limitations"`
}

type ExplainSafety struct {
	RedactionSummary string   `json:"redactionSummary"`
	LifecycleSummary string   `json:"lifecycleSummary"`
	TrustSummary     string   `json:"trustSummary"`
	DoNotManage      []string `json:"doNotManage"`
}

type ExplainDiagnostic struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Ref      string `json:"ref,omitempty"`
	Source   string `json:"source,omitempty"`
	Path     string `json:"path,omitempty"`
}

type ExplainErrorObject struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type ExplainError struct {
	Code    string
	Message string
	Exit    int
	Details map[string]any
}

func (e *ExplainError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *ExplainError) ExitCode() int {
	if e == nil || e.Exit == 0 {
		return 1
	}
	return e.Exit
}

func Explain(opts ExplainOptions) (*ExplainReport, error) {
	target := strings.TrimSpace(opts.Target)
	report := baseExplainReport()
	if err := validateExplainTargetRef(target); err != nil {
		explainErr := err.(*ExplainError)
		return errorExplainReport(report, explainErr, target, ""), explainErr
	}
	report.RecipeExplain.Target.Ref = target

	repoRoot, err := normalizeExplainRepoRoot(opts.RepoRoot)
	if err != nil {
		explainErr := &ExplainError{Code: ExplainCodeInternalError, Message: err.Error(), Exit: 1}
		return errorExplainReport(report, explainErr, target, ""), explainErr
	}

	if bundledTarget, ok := LookupBundledTarget(target); ok {
		bundled, _ := bundledExplain(bundledTarget.ID)
		report.RecipeExplain = bundled
		report.RecipeExplain.Diagnostics = appendLocalRecipeCollisionDiagnostics(report.RecipeExplain.Diagnostics, repoRoot, bundledTarget)
		return finishExplainReport(report), nil
	}

	localPath := localRecipePath(repoRoot, target)
	if !fileExists(localPath) {
		knownTargets := KnownBundledTargetIDs()
		explainErr := &ExplainError{Code: ExplainCodeUnknownTarget, Message: fmt.Sprintf("unknown recipe target: %s (known bundled targets: %s)", target, strings.Join(knownTargets, ", ")), Exit: 2, Details: map[string]any{"target": target, "knownTargets": knownTargets}}
		return errorExplainReport(report, explainErr, target, ""), explainErr
	}

	rec, err := LoadLocal(repoRoot, target)
	if err != nil {
		safePath := safeRelOrBase(repoRoot, localPath)
		details := map[string]any{"target": target, "path": safePath, "reason": err.Error()}
		validationDiagnostics := ValidationDiagnostics(err)
		if len(validationDiagnostics) > 0 {
			details["diagnostics"] = validationDiagnostics
		}
		explainErr := &ExplainError{Code: ExplainCodeInvalidRecipe, Message: fmt.Sprintf("invalid local recipe for target %s", target), Exit: 2, Details: details}
		report := errorExplainReport(report, explainErr, target, safePath)
		appendValidationExplainDiagnostics(report, validationDiagnostics, target, safePath)
		return report, explainErr
	}
	if rec.Target != target {
		explainErr := &ExplainError{Code: ExplainCodeInvalidRecipe, Message: fmt.Sprintf("local recipe target mismatch: expected %s, got %s", target, rec.Target), Exit: 2, Details: map[string]any{"target": target, "recipeTarget": rec.Target, "path": safeRelOrBase(repoRoot, localPath)}}
		return errorExplainReport(report, explainErr, target, safeRelOrBase(repoRoot, localPath)), explainErr
	}
	report.RecipeExplain = explainFromRecipe(rec, "local", "recipe://local/"+target, "untrusted")
	return finishExplainReport(report), nil
}

func ExplainJSON(report *ExplainReport) (string, error) {
	if report == nil {
		report = baseExplainReport()
		report.Summary.Status = "error"
	}
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	return string(payload) + "\n", nil
}

func ExplainText(report *ExplainReport) string {
	if report == nil {
		return "recipe explain\nsummary status=error"
	}
	var lines []string
	lines = append(lines, "recipe explain")
	if report.RecipeExplain.Target.Ref != "" {
		lines = append(lines, "target: "+report.RecipeExplain.Target.Ref)
	}
	if report.RecipeExplain.Target.DisplayName != "" {
		lines = append(lines, "display: "+report.RecipeExplain.Target.DisplayName)
	}
	if report.RecipeExplain.Recipe.RecipeRef != "" {
		lines = append(lines, fmt.Sprintf("recipe: %s source=%s trust=%s", report.RecipeExplain.Recipe.RecipeRef, report.RecipeExplain.Recipe.Source, report.RecipeExplain.Recipe.TrustStatus))
	}
	if report.RecipeExplain.Target.Capability != "" || report.RecipeExplain.Target.SupportLevel != "" {
		lines = append(lines, fmt.Sprintf("support: level=%s capability=%s platform=%s", report.RecipeExplain.Target.SupportLevel, report.RecipeExplain.Target.Capability, report.RecipeExplain.Target.PlatformSupport))
	}
	if report.RecipeExplain.Selection.Status != "" {
		lines = append(lines, fmt.Sprintf("selection: %s (%s)", report.RecipeExplain.Selection.Status, report.RecipeExplain.Selection.Reason))
	}
	if len(report.RecipeExplain.Settings) > 0 {
		lines = append(lines, "settings:")
		for _, setting := range report.RecipeExplain.Settings {
			lines = append(lines, fmt.Sprintf("  %s resource=%s driver=%s scope=%s", setting.Ref, setting.ResourceID, setting.Driver, setting.DefaultScope))
			if len(setting.DiffLimitations) > 0 {
				lines = append(lines, "    diff limitations: "+strings.Join(setting.DiffLimitations, "; "))
			}
			if len(setting.ApplyLimitations) > 0 {
				lines = append(lines, "    apply limitations: "+strings.Join(setting.ApplyLimitations, "; "))
			}
		}
	}
	if len(report.RecipeExplain.Resources) > 0 {
		lines = append(lines, "resources:")
		for _, resource := range report.RecipeExplain.Resources {
			line := fmt.Sprintf("  %s driver=%s location=%s path=%s", resource.ID, resource.DriverID, resource.LocationID, resource.Path)
			if resource.Selector != nil {
				line += " selector=" + resource.Selector.Summary
			}
			lines = append(lines, line)
		}
	}
	if len(report.RecipeExplain.Drivers) > 0 {
		lines = append(lines, "drivers:")
		for _, driver := range report.RecipeExplain.Drivers {
			lines = append(lines, fmt.Sprintf("  %s: %s", driver.ID, driver.Summary))
		}
	}
	if len(report.RecipeExplain.Safety.DoNotManage) > 0 || report.RecipeExplain.Safety.RedactionSummary != "" {
		lines = append(lines, "safety:")
		if report.RecipeExplain.Safety.RedactionSummary != "" {
			lines = append(lines, "  redaction: "+report.RecipeExplain.Safety.RedactionSummary)
		}
		if report.RecipeExplain.Safety.LifecycleSummary != "" {
			lines = append(lines, "  lifecycle: "+report.RecipeExplain.Safety.LifecycleSummary)
		}
		if report.RecipeExplain.Safety.TrustSummary != "" {
			lines = append(lines, "  trust: "+report.RecipeExplain.Safety.TrustSummary)
		}
		for _, item := range report.RecipeExplain.Safety.DoNotManage {
			lines = append(lines, "  do not manage: "+item)
		}
	}
	if report.Error != nil {
		lines = append(lines, fmt.Sprintf("error[%s]: %s", report.Error.Code, report.Error.Message))
	}
	if len(report.RecipeExplain.Diagnostics) > 0 {
		lines = append(lines, "diagnostics:")
		for _, diagnostic := range report.RecipeExplain.Diagnostics {
			lines = append(lines, fmt.Sprintf("  %s[%s]: %s", diagnostic.Severity, diagnostic.Code, diagnostic.Message))
		}
	}
	lines = append(lines, fmt.Sprintf("summary status=%s changed=%d blocked=%d", report.Summary.Status, report.Summary.Changed, report.Summary.Blocked))
	return strings.Join(lines, "\n")
}

func baseExplainReport() *ExplainReport {
	return &ExplainReport{
		Schema:        ExplainSchema,
		SchemaVersion: SupportedVersion,
		Command:       ExplainCommand,
		RunID:         ExplainRunID,
		Summary:       ExplainSummary{Status: "ok"},
		Items:         []any{},
		RecipeExplain: RecipeExplain{
			SettingGroups:    []any{},
			NativeOperations: []any{},
		},
	}
}

func finishExplainReport(report *ExplainReport) *ExplainReport {
	if report == nil {
		return baseExplainReport()
	}
	if report.RecipeExplain.Selection.Status == "" {
		report.RecipeExplain.Selection = ExplainSelection{Status: "unknown", Reason: "active profile selection was not resolved because recipe explain is metadata-only", ProfileStack: []string{}}
	}
	report.RecipeExplain.Diagnostics = append(report.RecipeExplain.Diagnostics, ExplainDiagnostic{
		Code:     ExplainCodeSelectionUnresolved,
		Severity: ExplainSeverityInfo,
		Message:  "active profile selection was not resolved because recipe explain must not bootstrap identity or write local state",
		Ref:      report.RecipeExplain.Target.Ref,
	})
	sortDiagnostics(report.RecipeExplain.Diagnostics)
	return report
}

func errorExplainReport(report *ExplainReport, err *ExplainError, target string, path string) *ExplainReport {
	if report == nil {
		report = baseExplainReport()
	}
	report.Summary.Status = "error"
	message := "recipe explain failed"
	code := ExplainCodeInternalError
	var details map[string]any
	if err != nil {
		message = err.Message
		code = err.Code
		details = err.Details
	}
	report.Error = &ExplainErrorObject{Code: code, Message: message, Details: details}
	report.RecipeExplain.Target.Ref = target
	report.RecipeExplain.Diagnostics = append(report.RecipeExplain.Diagnostics, ExplainDiagnostic{Code: code, Severity: ExplainSeverityError, Message: message, Ref: target, Path: path})
	return report
}

func appendValidationExplainDiagnostics(report *ExplainReport, diagnostics []ValidationDiagnostic, target string, path string) {
	if report == nil || len(diagnostics) == 0 {
		return
	}
	for _, diagnostic := range diagnostics {
		report.RecipeExplain.Diagnostics = append(report.RecipeExplain.Diagnostics, ExplainDiagnostic{
			Code:     diagnostic.Code,
			Severity: diagnostic.Severity,
			Message:  diagnostic.Message,
			Ref:      target,
			Source:   "validation",
			Path:     path + "#" + diagnostic.Path,
		})
	}
	sortDiagnostics(report.RecipeExplain.Diagnostics)
}

func appendLocalRecipeCollisionDiagnostics(diagnostics []ExplainDiagnostic, repoRoot string, target BundledTarget) []ExplainDiagnostic {
	if strings.TrimSpace(repoRoot) == "" {
		return diagnostics
	}
	for _, collisionID := range target.LocalCollisionIDs() {
		if localPath := localRecipePath(repoRoot, collisionID); fileExists(localPath) {
			diagnostics = append(diagnostics, ExplainDiagnostic{
				Code:     ExplainCodeLocalRecipeShadowed,
				Severity: ExplainSeverityWarning,
				Message:  "local recipe with bundled target or alias id is ignored because bundled recipe precedence is required for this MVP",
				Ref:      target.ID,
				Source:   "local",
				Path:     safeRelOrBase(repoRoot, localPath),
			})
		}
	}
	return diagnostics
}

func validateExplainTargetRef(target string) error {
	if target == "" {
		return &ExplainError{Code: ExplainCodeInvalidRef, Message: "target ref is required", Exit: 2}
	}
	if strings.ContainsAny(target, ":#/") || strings.Contains(target, "://") {
		return &ExplainError{Code: ExplainCodeUnsupportedRefKind, Message: fmt.Sprintf("unsupported recipe explain ref kind: %s", target), Exit: 2, Details: map[string]any{"ref": target}}
	}
	if err := ValidatePublicID("target", target); err != nil {
		return &ExplainError{Code: ExplainCodeInvalidRef, Message: err.Error(), Exit: 2, Details: map[string]any{"ref": target}}
	}
	return nil
}

func normalizeExplainRepoRoot(repoRoot string) (string, error) {
	trimmed := strings.TrimSpace(repoRoot)
	if trimmed == "" {
		return os.Getwd()
	}
	return filepath.Abs(trimmed)
}

func bundledExplain(target string) (RecipeExplain, bool) {
	bundledTarget, ok := LookupBundledTarget(target)
	if !ok {
		return RecipeExplain{}, false
	}
	switch bundledTarget.ID {
	case CustomFilesTarget:
		explain := bundledCustomFilesExplain()
		applyBundledTargetMetadata(&explain, bundledTarget)
		return explain, true
	case GitTarget:
		explain := bundledGitExplain()
		applyBundledTargetMetadata(&explain, bundledTarget)
		return explain, true
	default:
		return RecipeExplain{}, false
	}
}

func applyBundledTargetMetadata(explain *RecipeExplain, target BundledTarget) {
	if explain == nil {
		return
	}
	explain.Target.Ref = target.ID
	explain.Target.DisplayName = target.DisplayName
	explain.Target.SupportLevel = target.SupportLevel
	explain.Target.Capability = target.Capability
	explain.Target.PlatformSupport = target.PlatformSupport
	explain.Recipe.Source = target.Source
	explain.Recipe.RecipeRef = target.RecipeRef
	explain.Recipe.TrustStatus = target.TrustStatus
	explain.Recipe.Version = target.Version
}

func bundledCustomFilesExplain() RecipeExplain {
	explain := RecipeExplain{
		Target:           ExplainTarget{Ref: CustomFilesTarget, DisplayName: "Custom files", SupportLevel: "experimental", Capability: "read-write", PlatformSupport: "unknown"},
		Recipe:           ExplainRecipeSource{Source: "bundled", RecipeRef: "recipe://bundled/custom.files", TrustStatus: "trusted", Version: "1"},
		Selection:        ExplainSelection{Status: "unknown", Reason: "active profile selection was not resolved because recipe explain is metadata-only", ProfileStack: []string{}},
		SettingGroups:    []any{},
		NativeOperations: []any{},
		Safety: ExplainSafety{
			RedactionSummary: "metadata-only explanation; live and desired values are not read",
			LifecycleSummary: "file resources do not require app lifecycle actions; target-specific lifecycle is user-owned",
			TrustSummary:     "bundled recipe metadata is trusted by the manager release",
			DoNotManage:      []string{"secrets are not specially classified by custom.files", "unsafe paths and symlink escapes", "structured app semantics beyond raw file/file-tree resources"},
		},
	}
	explain.Settings = []ExplainSetting{
		{Ref: CustomFilesTarget + ":file", ID: "file", Label: "Single file", SupportLevel: "experimental", Capability: "read-write", DefaultScope: "user", ArtifactForm: "file", SelectionStatus: "unknown", Sensitivity: "unknown", Lifecycle: "not-declared", ResourceID: "file", Driver: FileDriverID, DiffLimitations: []string{"raw file diff only"}, ApplyLimitations: []string{"applies only declared file resources"}},
		{Ref: CustomFilesTarget + ":file-tree", ID: "file-tree", Label: "File tree", SupportLevel: "experimental", Capability: "read-write", DefaultScope: "user", ArtifactForm: "file-tree", SelectionStatus: "unknown", Sensitivity: "unknown", Lifecycle: "not-declared", ResourceID: "file-tree", Driver: FileTreeDriverID, DiffLimitations: []string{"file-tree metadata and entries only"}, ApplyLimitations: []string{"include/exclude globs constrain managed paths"}},
	}
	explain.Resources = []ExplainResource{
		{ID: "file", LocationID: "recipe-defined", Path: "recipe-defined", DriverID: FileDriverID, SupportedOps: []string{"detect", "read", "diff", "preview", "backup", "apply", "verify", "restore"}, BackupRestore: "supported", Normalization: "byte-preserving metadata", DiffMode: "file"},
		{ID: "file-tree", LocationID: "recipe-defined", Path: "recipe-defined", DriverID: FileTreeDriverID, SupportedOps: []string{"detect", "read", "diff", "preview", "backup", "apply", "verify", "restore"}, BackupRestore: "supported", Normalization: "tree entries and metadata", DiffMode: "file-tree"},
	}
	explain.Drivers = driverExplains(FileDriverID, FileTreeDriverID)
	return explain
}

func bundledGitExplain() RecipeExplain {
	explain := RecipeExplain{
		Target:           ExplainTarget{Ref: GitTarget, DisplayName: "Git", SupportLevel: "experimental", Capability: "read-write", PlatformSupport: "unknown"},
		Recipe:           ExplainRecipeSource{Source: "bundled", RecipeRef: "recipe://bundled/git", TrustStatus: "trusted", Version: "1"},
		Selection:        ExplainSelection{Status: "unknown", Reason: "active profile selection was not resolved because recipe explain is metadata-only", ProfileStack: []string{}},
		SettingGroups:    []any{},
		NativeOperations: []any{},
		Safety: ExplainSafety{
			RedactionSummary: "metadata-only explanation; git config values are not read or emitted",
			LifecycleSummary: "git identity selected keys do not require shutting down Git",
			TrustSummary:     "bundled recipe metadata is trusted by the manager release",
			DoNotManage:      []string{"[credential] sections", "credential-related helper configuration", "secret-bearing authentication material and URL credential rewrites", "include and includeIf expansion", "arbitrary .gitconfig sections or keys", "global rewrites outside [user] name/email"},
		},
	}
	explain.Settings = []ExplainSetting{
		{Ref: GitTarget + ":user.email", ID: "user.email", Label: "User email", SupportLevel: "experimental", Capability: "read-write", DefaultScope: "user", ArtifactForm: "scalar", SelectionStatus: "unknown", Sensitivity: "personal", Lifecycle: "not-required", ResourceID: "user-email", Driver: IniFileDriverID, DiffLimitations: []string{"selected-key scalar diff only"}, ApplyLimitations: []string{"writes only .gitconfig [user] email when later live operations are implemented"}},
		{Ref: GitTarget + ":user.name", ID: "user.name", Label: "User name", SupportLevel: "experimental", Capability: "read-write", DefaultScope: "user", ArtifactForm: "scalar", SelectionStatus: "unknown", Sensitivity: "personal", Lifecycle: "not-required", ResourceID: "user-name", Driver: IniFileDriverID, DiffLimitations: []string{"selected-key scalar diff only"}, ApplyLimitations: []string{"writes only .gitconfig [user] name when later live operations are implemented"}},
	}
	explain.Resources = []ExplainResource{
		{ID: "user-email", LocationID: "home", Path: ".gitconfig", DriverID: IniFileDriverID, Selector: &ExplainSelector{Section: "user", Key: "email", MissingSection: "create", MissingKey: "create", DuplicatePolicy: "reject", DeleteKey: "reject", Summary: "[user] email"}, SupportedOps: []string{"selected-key metadata", "future read", "future preview", "future apply"}, BackupRestore: "not-implemented", Normalization: "selected INI scalar", DiffMode: "selected-key"},
		{ID: "user-name", LocationID: "home", Path: ".gitconfig", DriverID: IniFileDriverID, Selector: &ExplainSelector{Section: "user", Key: "name", MissingSection: "create", MissingKey: "create", DuplicatePolicy: "reject", DeleteKey: "reject", Summary: "[user] name"}, SupportedOps: []string{"selected-key metadata", "future read", "future preview", "future apply"}, BackupRestore: "not-implemented", Normalization: "selected INI scalar", DiffMode: "selected-key"},
	}
	explain.Drivers = driverExplains(IniFileDriverID)
	return explain
}

func explainFromRecipe(rec *Recipe, source string, recipeRef string, trustStatus string) RecipeExplain {
	explain := RecipeExplain{
		Target:           ExplainTarget{Ref: rec.Target, DisplayName: rec.DisplayName, SupportLevel: rec.SupportLevel, Capability: rec.Capability, PlatformSupport: "unknown"},
		Recipe:           ExplainRecipeSource{Source: source, RecipeRef: recipeRef, TrustStatus: trustStatus, Version: fmt.Sprintf("%d", rec.SchemaVersion)},
		Selection:        ExplainSelection{Status: "unknown", Reason: "active profile selection was not resolved because recipe explain is metadata-only", ProfileStack: []string{}},
		SettingGroups:    []any{},
		NativeOperations: []any{},
		Safety:           ExplainSafety{RedactionSummary: "metadata-only explanation; live and desired values are not read", LifecycleSummary: "not declared in current recipe model", TrustSummary: "local recipe is untrusted for writes until a later trust workflow", DoNotManage: []string{"value-bearing defaults", "undeclared resources", "driver operations during explanation"}},
	}
	for _, settingID := range sortedKeys(rec.Settings) {
		setting := rec.Settings[settingID]
		resource := rec.Resources[setting.Resource]
		explain.Settings = append(explain.Settings, ExplainSetting{Ref: rec.Target + ":" + settingID, ID: settingID, Label: fallbackLabel(settingID), SupportLevel: fallback(rec.SupportLevel, setting.SupportLevel), Capability: effectiveSettingCapability(rec, setting), DefaultScope: fallbackUnknown(setting.ScopeDefault), ArtifactForm: fallback(artifactFormForDriver(resource.Driver), setting.ArtifactForm), SelectionStatus: "unknown", Sensitivity: fallbackDeclared(setting.Sensitivity), Lifecycle: fallbackDeclared(setting.Lifecycle), ResourceID: setting.Resource, Driver: resource.Driver, DiffLimitations: []string{"metadata explanation only"}, ApplyLimitations: []string{"recipe explain does not apply"}})
	}
	for _, resourceID := range sortedKeys(rec.Resources) {
		resource := rec.Resources[resourceID]
		explain.Resources = append(explain.Resources, explainResource(resourceID, resource))
	}
	explain.Drivers = driverExplains(uniqueDrivers(explain.Resources)...)
	return explain
}

func explainResource(resourceID string, resource Resource) ExplainResource {
	explained := ExplainResource{ID: resourceID, LocationID: resource.Location, Path: resource.Path, DriverID: resource.Driver, SupportedOps: []string{"metadata explanation"}, BackupRestore: "unknown", Normalization: "unknown", DiffMode: "unknown", Include: append([]string(nil), resource.Include...), Exclude: append([]string(nil), resource.Exclude...)}
	switch resource.Driver {
	case FileDriverID:
		explained.BackupRestore = "supported when used by custom.files"
		explained.Normalization = "byte-preserving metadata"
		explained.DiffMode = "file"
	case FileTreeDriverID:
		explained.BackupRestore = "supported when used by custom.files"
		explained.Normalization = "tree entries and metadata"
		explained.DiffMode = "file-tree"
	case IniFileDriverID:
		explained.BackupRestore = "not-implemented"
		explained.Normalization = "selected INI scalar"
		explained.DiffMode = "selected-key"
		if resource.Selector != nil {
			explained.Selector = &ExplainSelector{Section: resource.Selector.Section, Key: resource.Selector.Key, MissingSection: selectorMissingSection(resource.Selector), MissingKey: selectorMissingKey(resource.Selector), DuplicatePolicy: selectorDuplicatePolicy(resource.Selector), DeleteKey: selectorDeleteKey(resource.Selector), Summary: fmt.Sprintf("[%s] %s", resource.Selector.Section, resource.Selector.Key)}
		}
	case JSONFileDriverID:
		explained.BackupRestore = "not-implemented"
		explained.Normalization = "selected JSON scalar"
		explained.DiffMode = "selected-path"
		if resource.Selector != nil {
			explained.Selector = &ExplainSelector{Path: append([]string(nil), resource.Selector.Path...), DuplicatePolicy: selectorDuplicatePolicy(resource.Selector), DeleteKey: selectorDeleteKey(resource.Selector), Summary: strings.Join(resource.Selector.Path, ".")}
		}
	case YAMLFileDriverID:
		explained.BackupRestore = "not-implemented"
		explained.Normalization = "selected YAML scalar"
		explained.DiffMode = "selected-path"
		if resource.Selector != nil {
			explained.Selector = &ExplainSelector{Path: append([]string(nil), resource.Selector.Path...), DuplicatePolicy: selectorDuplicatePolicy(resource.Selector), DeleteKey: selectorDeleteKey(resource.Selector), Summary: strings.Join(resource.Selector.Path, ".")}
		}
	}
	return explained
}

func driverExplains(ids ...string) []ExplainDriver {
	seen := map[string]bool{}
	var out []ExplainDriver
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		switch id {
		case FileDriverID:
			out = append(out, ExplainDriver{ID: id, Summary: "manages one declared file as raw file content", Operations: []string{"detect", "read", "diff", "preview", "backup", "apply", "verify", "restore"}, Limitations: []string{"no structured key semantics"}})
		case FileTreeDriverID:
			out = append(out, ExplainDriver{ID: id, Summary: "manages a declared file tree with include/exclude globs", Operations: []string{"detect", "read", "diff", "preview", "backup", "apply", "verify", "restore"}, Limitations: []string{"only paths allowed by recipe globs are managed"}})
		case IniFileDriverID:
			out = append(out, ExplainDriver{ID: id, Summary: "explains deterministic INI selected section/key resources", Operations: []string{"metadata", "future selected-key read/preview/apply"}, Limitations: []string{"no include/includeIf expansion", "no arbitrary section/key writes", "duplicate keys rejected"}})
		case JSONFileDriverID:
			out = append(out, ExplainDriver{ID: id, Summary: "explains deterministic JSON selected path scalar resources", Operations: []string{"metadata", "future selected-path read/preview/apply"}, Limitations: []string{"no JSONPath expressions", "selected leaf must be scalar"}})
		case YAMLFileDriverID:
			out = append(out, ExplainDriver{ID: id, Summary: "explains deterministic YAML selected path scalar resources", Operations: []string{"metadata", "future selected-path read/preview/apply"}, Limitations: []string{"no path expressions", "selected leaf must be supported scalar"}})
		default:
			out = append(out, ExplainDriver{ID: id, Summary: "unknown driver metadata", Operations: []string{"metadata explanation"}, Limitations: []string{"driver is not bundled"}})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func uniqueDrivers(resources []ExplainResource) []string {
	ids := make([]string, 0, len(resources))
	for _, resource := range resources {
		ids = append(ids, resource.DriverID)
	}
	return ids
}

func artifactFormForDriver(driver string) string {
	switch driver {
	case FileDriverID:
		return "file"
	case FileTreeDriverID:
		return "file-tree"
	case IniFileDriverID, JSONFileDriverID, YAMLFileDriverID:
		return "scalar"
	default:
		return "unknown"
	}
}

func fallbackLabel(value string) string {
	label := strings.ReplaceAll(value, ".", " ")
	label = strings.ReplaceAll(label, "-", " ")
	return label
}

func fallbackUnknown(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unknown"
	}
	return value
}

func fallbackDeclared(value string) string {
	if strings.TrimSpace(value) == "" {
		return "not-declared"
	}
	return value
}

func fallback(defaultValue string, value string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
}

func localRecipePath(repoRoot string, recipeID string) string {
	return filepath.Join(repoRoot, localRecipeRelRoot, recipeID, "recipe.yaml")
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func safeRelOrBase(root string, path string) string {
	if root != "" {
		if rel, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(rel, "..") && rel != "." {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.Base(path)
}

func sortDiagnostics(diagnostics []ExplainDiagnostic) {
	sort.SliceStable(diagnostics, func(i, j int) bool {
		if diagnostics[i].Severity != diagnostics[j].Severity {
			return diagnostics[i].Severity < diagnostics[j].Severity
		}
		return diagnostics[i].Code < diagnostics[j].Code
	})
}
