package appauthor

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
)

const (
	CreateSchema    = "dotfiles-manager.v2.app.create"
	ValidateSchema  = "dotfiles-manager.v2.app.validate"
	CreateCommand   = "app.create"
	ValidateCommand = "app.validate"
	CreateRunID     = "app-create"
	ValidateRunID   = "app-validate"
)

const (
	TemplateFile          = "file"
	TemplateSelectedValue = "selected-value"
	TemplateNativeExport  = "native-export"
)

const (
	CodeRepoInvalid       = "app.repo.invalid"
	CodeTargetInvalid     = "app.target.invalid"
	CodeTargetCollision   = "app.target.collision"
	CodeTargetMismatch    = "app.validate.target.mismatch"
	CodeTemplateRequired  = "app.template.required"
	CodeTemplateInvalid   = "app.template.invalid"
	CodeFlagInvalid       = "app.flag.invalid"
	CodePathInvalid       = "app.path.invalid"
	CodePathUnsafe        = "app.path.unsafe"
	CodeRecipeExists      = "app.recipe.exists"
	CodeWriteFailed       = "app.write-failed"
	CodeRecipeInvalid     = "app.recipe.invalid"
	CodeRecipeMissing     = "app.recipe.missing"
	CodeWriteSurfaceError = "app.write-surface.invalid"
)

const (
	SeverityInfo    = "info"
	SeverityWarning = "warning"
	SeverityError   = "error"
)

type CreateOptions struct {
	RepoRoot     string
	TargetID     string
	Template     string
	FromPath     string
	DisplayName  string
	SettingID    string
	SettingLabel string
	Driver       string
	Selector     string
	ScopeDefault string
	Lifecycle    string
	DryRun       bool
}

type ValidateOptions struct {
	RepoRoot string
	TargetID string
}

type CreateReport struct {
	Schema        string        `json:"schema"`
	SchemaVersion int           `json:"schemaVersion"`
	Command       string        `json:"command"`
	RunID         string        `json:"runId"`
	DryRun        bool          `json:"dryRun"`
	Summary       CreateSummary `json:"summary"`
	AppCreate     CreateResult  `json:"appCreate"`
	Diagnostics   []Diagnostic  `json:"diagnostics"`
	Error         *ErrorObject  `json:"error"`
}

type ValidateReport struct {
	Schema        string          `json:"schema"`
	SchemaVersion int             `json:"schemaVersion"`
	Command       string          `json:"command"`
	RunID         string          `json:"runId"`
	Summary       ValidateSummary `json:"summary"`
	AppValidate   ValidateResult  `json:"appValidate"`
	Diagnostics   []Diagnostic    `json:"diagnostics"`
	Error         *ErrorObject    `json:"error"`
}

type CreateSummary struct {
	Status    string `json:"status"`
	Planned   int    `json:"planned"`
	Written   int    `json:"written"`
	Unchanged int    `json:"unchanged"`
	Blocked   int    `json:"blocked"`
	Failed    int    `json:"failed"`
}

type ValidateSummary struct {
	Status   string `json:"status"`
	Checked  int    `json:"checked"`
	Warnings int    `json:"warnings"`
	Blocked  int    `json:"blocked"`
	Failed   int    `json:"failed"`
}

type CreateResult struct {
	Target      TargetInfo   `json:"target"`
	Template    string       `json:"template"`
	Files       []FileAction `json:"files"`
	NextActions []string     `json:"nextActions"`
}

type ValidateResult struct {
	Target   TargetInfo     `json:"target"`
	Trust    TrustInfo      `json:"trust"`
	Fixtures []FixtureCheck `json:"fixtures"`
}

type TargetInfo struct {
	ID          string `json:"id,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	RecipeRef   string `json:"recipeRef,omitempty"`
}

type FileAction struct {
	Kind   string `json:"kind"`
	Path   string `json:"path"`
	Action string `json:"action"`
}

type TrustInfo struct {
	LocalTrustState         string                    `json:"localTrustState"`
	WriteTrustRequired      bool                      `json:"writeTrustRequired"`
	WriteSurfaceFingerprint string                    `json:"writeSurfaceFingerprint,omitempty"`
	WriteSurface            *recipe.TrustWriteSurface `json:"writeSurface,omitempty"`
}

type FixtureCheck struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

type Diagnostic struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Path     string `json:"path,omitempty"`
}

type ErrorObject struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
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

func RunCreate(opts CreateOptions) (*CreateReport, error) {
	report := baseCreateReport(opts.DryRun)
	repoRoot, err := normalizeRepoRoot(opts.RepoRoot)
	if err != nil {
		return failCreate(report, CodeRepoInvalid, err.Error(), 2, nil)
	}
	targetID, err := normalizeTargetID(opts.TargetID)
	if err != nil {
		return failCreate(report, CodeTargetInvalid, err.Error(), 2, nil)
	}
	displayName := strings.TrimSpace(opts.DisplayName)
	if displayName == "" {
		displayName = defaultDisplayName(targetID)
	}
	report.AppCreate.Target = TargetInfo{ID: targetID, DisplayName: displayName, RecipeRef: recipeRef(targetID)}

	if err := checkBundledCollision(targetID); err != nil {
		return failCreate(report, CodeTargetCollision, err.Error(), 2, map[string]any{"target": targetID})
	}
	template := strings.TrimSpace(opts.Template)
	if template == "" {
		return failCreate(report, CodeTemplateRequired, "--template is required", 2, nil)
	}
	report.AppCreate.Template = template

	plan, recBody, readmeBody, fixturesReadme, err := buildCreatePlan(targetID, displayName, template, opts)
	if err != nil {
		return failCreate(report, errorCode(err), err.Error(), errorExit(err, 2), errorDetails(err))
	}
	report.AppCreate.Files = plan
	report.AppCreate.NextActions = nextActionsForTemplate(template, targetID)
	report.Summary.Planned = len(plan)

	relRoot := localRecipeRelRoot(targetID)
	if err := validateRepoWriteTarget(repoRoot, relRoot); err != nil {
		return failCreate(report, errorCode(err), err.Error(), errorExit(err, 5), errorDetails(err))
	}

	if opts.DryRun {
		for idx := range report.AppCreate.Files {
			report.AppCreate.Files[idx].Action = "planned"
		}
		report.Summary.Status = "ok"
		return report, nil
	}

	files := map[string]string{
		path.Join(relRoot, "recipe.yaml"):        recBody,
		path.Join(relRoot, "README.md"):          readmeBody,
		path.Join(relRoot, "fixtures/README.md"): fixturesReadme,
	}
	if err := writeCreateFiles(repoRoot, files); err != nil {
		return failCreate(report, errorCode(err), err.Error(), errorExit(err, 2), errorDetails(err))
	}
	report.Summary.Written = len(plan)
	report.Summary.Status = "changed"
	return report, nil
}

func RunValidate(opts ValidateOptions) (*ValidateReport, error) {
	report := baseValidateReport()
	repoRoot, err := normalizeRepoRoot(opts.RepoRoot)
	if err != nil {
		return failValidate(report, CodeRepoInvalid, err.Error(), 2, nil)
	}
	targetID, err := normalizeTargetID(opts.TargetID)
	if err != nil {
		return failValidate(report, CodeTargetInvalid, err.Error(), 2, nil)
	}
	report.AppValidate.Target = TargetInfo{ID: targetID, RecipeRef: recipeRef(targetID)}
	report.AppValidate.Trust.LocalTrustState = "not-checked"
	report.AppValidate.Fixtures = []FixtureCheck{{Name: "fixtures-readme", State: "skipped"}}
	if err := checkBundledCollision(targetID); err != nil {
		return failValidate(report, CodeTargetCollision, err.Error(), 2, map[string]any{"target": targetID})
	}
	if err := validateLocalRecipeReadTarget(repoRoot, targetID); err != nil {
		diagnostics := []Diagnostic{{Code: errorCode(err), Severity: SeverityError, Message: err.Error(), Path: localRecipeRelPath(targetID)}}
		report.Diagnostics = append(report.Diagnostics, diagnostics...)
		return failValidateWithExistingDiagnostics(report, errorCode(err), err.Error(), errorExit(err, 2), errorDetails(err))
	}

	rec, err := loadLocalRecipe(repoRoot, targetID)
	if err != nil {
		code := CodeRecipeInvalid
		if errors.Is(err, os.ErrNotExist) {
			code = CodeRecipeMissing
		}
		diagnostics := diagnosticsFromRecipeError(err)
		if len(diagnostics) == 0 {
			diagnostics = []Diagnostic{{Code: code, Severity: SeverityError, Message: err.Error(), Path: localRecipeRelPath(targetID)}}
		}
		report.Diagnostics = append(report.Diagnostics, diagnostics...)
		return failValidateWithExistingDiagnostics(report, code, err.Error(), 2, nil)
	}
	report.Summary.Checked = 1
	report.AppValidate.Target.DisplayName = rec.DisplayName
	if rec.Target != targetID {
		message := fmt.Sprintf("local recipe target %s does not match requested target %s", rec.Target, targetID)
		report.Diagnostics = append(report.Diagnostics, Diagnostic{Code: CodeTargetMismatch, Severity: SeverityError, Message: message, Path: "$.target"})
		return failValidateWithExistingDiagnostics(report, CodeTargetMismatch, message, 2, map[string]any{"requestedTarget": targetID, "recipeTarget": rec.Target})
	}

	writeDiagnostics := authoringWriteSafetyDiagnostics(rec)
	report.Diagnostics = append(report.Diagnostics, writeDiagnostics...)
	if hasBlockingDiagnostics(writeDiagnostics) {
		return failValidateWithExistingDiagnostics(report, CodeRecipeInvalid, "recipe authoring metadata is not safe for write planning", 2, nil)
	}

	surface, fingerprint, err := recipe.RecipeWriteSurface(rec)
	if err != nil {
		diagnostics := diagnosticsFromRecipeError(err)
		if len(diagnostics) == 0 {
			diagnostics = []Diagnostic{{Code: CodeWriteSurfaceError, Severity: SeverityError, Message: err.Error()}}
		}
		report.Diagnostics = append(report.Diagnostics, diagnostics...)
		return failValidateWithExistingDiagnostics(report, CodeWriteSurfaceError, err.Error(), 2, nil)
	}
	report.AppValidate.Trust.WriteSurface = &surface
	report.AppValidate.Trust.WriteSurfaceFingerprint = fingerprint
	report.AppValidate.Trust.WriteTrustRequired = writeTrustRequired(surface)
	if report.AppValidate.Trust.WriteTrustRequired {
		report.Diagnostics = append(report.Diagnostics, Diagnostic{
			Code:     "app.validate.trust.not-checked",
			Severity: SeverityWarning,
			Message:  "local recipe writes require explicit trust before save/apply; app validate does not read or create trust records",
			Path:     "$.appValidate.trust.localTrustState",
		})
	}
	finishValidate(report)
	return report, nil
}

func JSONCreate(report *CreateReport) (string, error) {
	if report == nil {
		report = baseCreateReport(false)
		report.Summary.Status = "error"
	}
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	return string(payload) + "\n", nil
}

func JSONValidate(report *ValidateReport) (string, error) {
	if report == nil {
		report = baseValidateReport()
		report.Summary.Status = "error"
	}
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	return string(payload) + "\n", nil
}

func TextCreate(report *CreateReport) string {
	if report == nil {
		return "app create\nsummary status=error planned=0 written=0 blocked=0 failed=1"
	}
	lines := []string{"app create"}
	if report.AppCreate.Target.ID != "" {
		lines = append(lines, fmt.Sprintf("target: %s (%s)", report.AppCreate.Target.ID, report.AppCreate.Target.RecipeRef))
	}
	if report.AppCreate.Template != "" {
		lines = append(lines, "template: "+report.AppCreate.Template)
	}
	if len(report.AppCreate.Files) > 0 {
		lines = append(lines, "files:")
		for _, file := range report.AppCreate.Files {
			lines = append(lines, fmt.Sprintf("  %s %s %s", file.Action, file.Kind, file.Path))
		}
	}
	if len(report.AppCreate.NextActions) > 0 {
		lines = append(lines, "next:")
		for _, action := range report.AppCreate.NextActions {
			lines = append(lines, "  "+action)
		}
	}
	if report.Error == nil && report.Summary.Status != "blocked" && report.Summary.Status != "error" {
		lines = append(lines, "safety: no live app data was read, no desired values were written, and no trust record was created")
	}
	appendDiagnostics(&lines, report.Diagnostics)
	lines = append(lines, fmt.Sprintf("summary status=%s planned=%d written=%d blocked=%d failed=%d", report.Summary.Status, report.Summary.Planned, report.Summary.Written, report.Summary.Blocked, report.Summary.Failed))
	return strings.Join(lines, "\n")
}

func TextValidate(report *ValidateReport) string {
	if report == nil {
		return "app validate\nsummary status=error checked=0 warnings=0 blocked=0 failed=1"
	}
	lines := []string{"app validate"}
	if report.AppValidate.Target.ID != "" {
		lines = append(lines, fmt.Sprintf("target: %s (%s)", report.AppValidate.Target.ID, report.AppValidate.Target.RecipeRef))
	}
	trust := report.AppValidate.Trust
	if trust.LocalTrustState != "" {
		lines = append(lines, fmt.Sprintf("trust: local=%s writeRequired=%t fingerprint=%s", trust.LocalTrustState, trust.WriteTrustRequired, emptyDash(trust.WriteSurfaceFingerprint)))
	}
	appendDiagnostics(&lines, report.Diagnostics)
	lines = append(lines, fmt.Sprintf("summary status=%s checked=%d warnings=%d blocked=%d failed=%d", report.Summary.Status, report.Summary.Checked, report.Summary.Warnings, report.Summary.Blocked, report.Summary.Failed))
	return strings.Join(lines, "\n")
}

func buildCreatePlan(targetID string, displayName string, template string, opts CreateOptions) ([]FileAction, string, string, string, error) {
	settingID, err := normalizeSettingID(opts.SettingID)
	if err != nil {
		return nil, "", "", "", err
	}
	settingLabel := strings.TrimSpace(opts.SettingLabel)
	if settingLabel == "" {
		return nil, "", "", "", typedError(CodeFlagInvalid, "--setting-label is required", 2, nil)
	}
	scopeDefault := strings.TrimSpace(opts.ScopeDefault)
	if scopeDefault == "" {
		return nil, "", "", "", typedError(CodeFlagInvalid, "--scope-default is required", 2, nil)
	}
	if !knownScope(scopeDefault) {
		return nil, "", "", "", typedError(CodeFlagInvalid, "--scope-default must be one of shared, user, machine, machine-user", 2, map[string]any{"scopeDefault": scopeDefault})
	}
	lifecycle := strings.TrimSpace(opts.Lifecycle)
	if lifecycle == "" {
		return nil, "", "", "", typedError(CodeFlagInvalid, "--lifecycle is required", 2, nil)
	}
	if !knownLifecycle(lifecycle) {
		return nil, "", "", "", typedError(CodeFlagInvalid, "--lifecycle is unsupported", 2, map[string]any{"lifecycle": lifecycle})
	}

	var recBody string
	switch template {
	case TemplateFile:
		if driver := strings.TrimSpace(opts.Driver); driver != "" && driver != recipe.FileDriverID {
			return nil, "", "", "", typedError(CodeFlagInvalid, "--template file only supports --driver file or omitted --driver", 2, map[string]any{"driver": driver})
		}
		relPath, err := normalizeHomeRelativePath(opts.FromPath)
		if err != nil {
			return nil, "", "", "", err
		}
		if strings.TrimSpace(opts.Selector) != "" {
			return nil, "", "", "", typedError(CodeFlagInvalid, "--selector is not supported for --template file", 2, nil)
		}
		recBody = renderFileRecipe(targetID, displayName, settingID, settingLabel, resourceID(settingID, "file"), relPath, scopeDefault, lifecycle)
	case TemplateSelectedValue:
		driver := strings.TrimSpace(opts.Driver)
		selector, err := parseSelectedValueSelector(driver, opts.Selector)
		if err != nil {
			return nil, "", "", "", err
		}
		relPath, err := normalizeHomeRelativePath(opts.FromPath)
		if err != nil {
			return nil, "", "", "", err
		}
		recBody = renderSelectedValueRecipe(targetID, displayName, settingID, settingLabel, resourceID(settingID, "value"), relPath, driver, selector, scopeDefault, lifecycle)
	case TemplateNativeExport:
		if strings.TrimSpace(opts.FromPath) != "" || strings.TrimSpace(opts.Driver) != "" || strings.TrimSpace(opts.Selector) != "" {
			return nil, "", "", "", typedError(CodeFlagInvalid, "--template native-export does not accept --from-path, --driver, or --selector", 2, nil)
		}
		recBody = renderNativeExportDraftRecipe(targetID, displayName, settingID, settingLabel, resourceID(settingID, "native"), scopeDefault, lifecycle)
	default:
		return nil, "", "", "", typedError(CodeTemplateInvalid, "--template must be one of file, selected-value, native-export", 2, map[string]any{"template": template})
	}

	plan := []FileAction{
		{Kind: "recipe", Path: path.Join(localRecipeRelRoot(targetID), "recipe.yaml"), Action: "create"},
		{Kind: "readme", Path: path.Join(localRecipeRelRoot(targetID), "README.md"), Action: "create"},
		{Kind: "fixtures-readme", Path: path.Join(localRecipeRelRoot(targetID), "fixtures/README.md"), Action: "create"},
	}
	return plan, recBody, renderRecipeReadme(targetID, displayName, template), renderFixturesReadme(targetID), nil
}

type selectedSelector struct {
	Section string
	Key     string
	Path    []string
}

func parseSelectedValueSelector(driver string, selectorText string) (selectedSelector, error) {
	if driver == "" {
		return selectedSelector{}, typedError(CodeFlagInvalid, "--driver is required for --template selected-value", 2, nil)
	}
	switch driver {
	case recipe.IniFileDriverID, recipe.JSONFileDriverID, recipe.YAMLFileDriverID, recipe.TOMLFileDriverID, recipe.PlistFileDriverID:
	default:
		return selectedSelector{}, typedError(CodeFlagInvalid, "--driver for selected-value must be ini-file, json-file, yaml-file, toml-file, or plist-file", 2, map[string]any{"driver": driver})
	}
	selectorText = strings.TrimSpace(selectorText)
	if selectorText == "" {
		return selectedSelector{}, typedError(CodeFlagInvalid, "--selector is required for --template selected-value", 2, nil)
	}
	if strings.ContainsAny(selectorText, "\x00\r\n/\\[]{}*$'\"") {
		return selectedSelector{}, typedError(CodeFlagInvalid, "--selector must be a dot-separated literal selector without expressions, slashes, quotes, or control characters", 2, map[string]any{"selector": selectorText})
	}
	parts := strings.Split(selectorText, ".")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.TrimSpace(part) != part {
			return selectedSelector{}, typedError(CodeFlagInvalid, "--selector contains an empty or unsafe segment", 2, map[string]any{"selector": selectorText})
		}
		if !selectorSegmentPattern.MatchString(part) {
			return selectedSelector{}, typedError(CodeFlagInvalid, "--selector segments must use letters, digits, underscore, or hyphen", 2, map[string]any{"selector": selectorText})
		}
	}
	if driver == recipe.IniFileDriverID {
		if len(parts) != 2 {
			return selectedSelector{}, typedError(CodeFlagInvalid, "INI selectors must use section.key syntax", 2, map[string]any{"selector": selectorText})
		}
		return selectedSelector{Section: parts[0], Key: parts[1]}, nil
	}
	return selectedSelector{Path: parts}, nil
}

var selectorSegmentPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func renderFileRecipe(targetID, displayName, settingID, settingLabel, resourceID, relPath, scopeDefault, lifecycle string) string {
	return strings.Join([]string{
		"schema: dotfiles-manager.v2.recipe",
		"schemaVersion: 1",
		"target: " + targetID,
		"displayName: " + q(displayName),
		"supportLevel: experimental",
		"capability: read-write",
		"locations:",
		"  home:",
		"    default: \"~\"",
		"settings:",
		"  " + settingID + ":",
		"    label: " + q(settingLabel),
		"    supportLevel: experimental",
		"    capability: read-write",
		"    artifactForm: file",
		"    sensitivity: personal",
		"    redaction: redacted-for-display",
		"    lifecycle: " + lifecycle,
		"    scopeDefault: " + scopeDefault,
		"    resource: " + resourceID,
		"resources:",
		"  " + resourceID + ":",
		"    driver: file",
		"    location: home",
		"    path: " + q(relPath),
		"    capability: read-write",
		"    sensitivity: personal",
		"    redaction: redacted-for-display",
		"    lifecycle: " + lifecycle,
		"",
	}, "\n")
}

func renderSelectedValueRecipe(targetID, displayName, settingID, settingLabel, resourceID, relPath, driver string, selector selectedSelector, scopeDefault, lifecycle string) string {
	lines := []string{
		"schema: dotfiles-manager.v2.recipe",
		"schemaVersion: 1",
		"target: " + targetID,
		"displayName: " + q(displayName),
		"supportLevel: experimental",
		"capability: read-write",
		"locations:",
		"  home:",
		"    default: \"~\"",
		"settings:",
		"  " + settingID + ":",
		"    label: " + q(settingLabel),
		"    supportLevel: experimental",
		"    capability: read-write",
		"    artifactForm: scalar",
		"    sensitivity: personal",
		"    redaction: redacted-for-display",
		"    lifecycle: " + lifecycle,
		"    scopeDefault: " + scopeDefault,
		"    resource: " + resourceID,
		"resources:",
		"  " + resourceID + ":",
		"    driver: " + driver,
		"    location: home",
		"    path: " + q(relPath),
		"    capability: read-write",
		"    sensitivity: personal",
		"    redaction: redacted-for-display",
		"    lifecycle: " + lifecycle,
		"    selector:",
	}
	if driver == recipe.IniFileDriverID {
		lines = append(lines,
			"      section: "+q(selector.Section),
			"      key: "+q(selector.Key),
			"      missingSection: create",
			"      missingKey: create",
			"      duplicatePolicy: reject",
			"      deleteKey: reject",
		)
	} else {
		lines = append(lines, "      path:")
		for _, segment := range selector.Path {
			lines = append(lines, "        - "+q(segment))
		}
		lines = append(lines,
			"      createMissing: create",
			"      duplicatePolicy: reject",
			"      deleteKey: reject",
		)
	}
	lines = append(lines, "")
	return strings.Join(lines, "\n")
}

func renderNativeExportDraftRecipe(targetID, displayName, settingID, settingLabel, resourceID, scopeDefault, lifecycle string) string {
	artifactPath := path.Join("exports", targetID, "settings.native")
	return strings.Join([]string{
		"schema: dotfiles-manager.v2.recipe",
		"schemaVersion: 1",
		"target: " + targetID,
		"displayName: " + q(displayName),
		"supportLevel: experimental",
		"capability: export-only",
		"settings:",
		"  " + settingID + ":",
		"    label: " + q(settingLabel),
		"    supportLevel: experimental",
		"    capability: export-only",
		"    artifactForm: native-export",
		"    sensitivity: personal",
		"    redaction: metadata-only",
		"    lifecycle: " + lifecycle,
		"    scopeDefault: " + scopeDefault,
		"    resource: " + resourceID,
		"resources:",
		"  " + resourceID + ":",
		"    driver: native-export",
		"    nativeOperation: export-settings",
		"    capability: export-only",
		"    sensitivity: personal",
		"    redaction: metadata-only",
		"    lifecycle: " + lifecycle,
		"nativeOperations:",
		"  export-settings:",
		"    kind: export",
		"    reviewed: false",
		"    runner: command",
		"    platforms:",
		"      - darwin",
		"    artifactForm: native",
		"    diffMode: metadata-only",
		"    lifecycle: " + lifecycle,
		"    workingDirectory: temp",
		"    timeoutSeconds: 30",
		"    expectedExitCodes:",
		"      - 0",
		"    command:",
		"      executable: REPLACE_WITH_ABSOLUTE_REVIEWED_EXECUTABLE",
		"      args: []",
		"    stdin:",
		"      mode: none",
		"    stdout:",
		"      mode: capture",
		"      maxBytes: 65536",
		"    stderr:",
		"      mode: capture",
		"      maxBytes: 65536",
		"    outputs:",
		"      bundle:",
		"        root: artifact",
		"        path: " + q(artifactPath),
		"    redaction: metadata-only",
		"    review:",
		"      required: true",
		"      reasons:",
		"        - privacy-sensitive",
		"      message: \"Review the native export command before setting reviewed: true.\"",
		"",
	}, "\n")
}

func renderRecipeReadme(targetID, displayName, template string) string {
	lines := []string{
		"# " + displayName + " local recipe",
		"",
		"This recipe was generated for `" + targetID + "` by `dotfiles-manager app create`.",
		"",
		"What to edit next:",
		"",
		"1. Review `recipe.yaml` before using it for save/apply workflows.",
		"2. Confirm the named `home` location and resource paths match the app's documented config files.",
		"3. Keep `sensitivity`, `redaction`, `lifecycle`, and `scopeDefault` explicit for every managed setting/resource.",
		"4. Run `dotfiles-manager app validate " + targetID + "` after edits.",
		"",
		"Safety notes:",
		"",
		"- Generation did not read live app data and did not write desired values.",
		"- Generation did not create a trust record; write-capable local recipes still require explicit trust before future writes.",
		"- Fixtures must be synthetic or sanitized. Do not commit secrets, tokens, private account identifiers, or real personal values.",
		"",
		"Template: `" + template + "`.",
	}
	if template == TemplateNativeExport {
		lines = append(lines, "", "Native-export draft note:", "", "- This scaffold is intentionally non-runnable.", "- The native operation uses `reviewed: false` and an invalid placeholder executable.", "- Replace the native operation metadata and review it before expecting `app validate` to pass.")
	}
	lines = append(lines, "")
	return strings.Join(lines, "\n")
}

func renderFixturesReadme(targetID string) string {
	return strings.Join([]string{
		"# Fixtures for " + targetID,
		"",
		"Fixtures are optional but recommended before trusting a custom local recipe.",
		"",
		"Do not place real secrets, tokens, private account identifiers, or unsanitized personal data in fixtures.",
		"",
		"Roundtrip fixtures live under `roundtrip/<fixture-name>/` and are executed with:",
		"",
		"```bash",
		"dotfiles-manager app test " + targetID + " --roundtrip",
		"```",
		"",
		"Each roundtrip fixture uses:",
		"",
		"```text",
		"manifest.yaml",
		"input/live/locations/<location-id>/<resource.path>",
		"input/desired/",
		"expected/desired/",
		"expected/live/locations/<location-id>/<resource.path>",
		"```",
		"",
		"`app test --roundtrip` copies fixture data into a temporary directory. It must not read or write live app paths, the real desired root, trust records, backups, or ledgers.",
		"",
	}, "\n")
}

func normalizeHomeRelativePath(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", typedError(CodeFlagInvalid, "--from-path is required", 2, nil)
	}
	if strings.ContainsRune(trimmed, '\x00') {
		return "", typedError(CodePathInvalid, "--from-path must not contain NUL", 2, nil)
	}
	if strings.Contains(trimmed, "\\") {
		return "", typedError(CodePathInvalid, "--from-path must use slash separators, not backslashes", 2, map[string]any{"fromPath": trimmed})
	}
	var rel string
	switch {
	case trimmed == "~":
		return "", typedError(CodePathInvalid, "--from-path must point to a file or directory below home, not home itself", 2, map[string]any{"fromPath": trimmed})
	case strings.HasPrefix(trimmed, "~/"):
		rel = strings.TrimPrefix(trimmed, "~/")
	case filepath.IsAbs(trimmed) || strings.HasPrefix(trimmed, "/"):
		home, homeErr := os.UserHomeDir()
		if homeErr != nil || strings.TrimSpace(home) == "" {
			return "", typedError(CodePathInvalid, "--from-path was absolute and the current home directory could not be resolved", 2, map[string]any{"pathForm": "absolute"})
		}
		relPath, relErr := filepath.Rel(filepath.Clean(home), filepath.Clean(trimmed))
		if relErr != nil || relPath == "." || strings.HasPrefix(relPath, ".."+string(filepath.Separator)) || relPath == ".." {
			return "", typedError(CodePathInvalid, "--from-path must be below the current home directory or written as a home-relative path", 2, map[string]any{"pathForm": "absolute"})
		}
		rel = relPath
	default:
		rel = trimmed
	}
	rel = filepath.ToSlash(rel)
	cleaned, err := recipe.ValidateResourcePath(rel)
	if err != nil {
		return "", typedError(CodePathInvalid, err.Error(), 2, map[string]any{"pathForm": "home-relative"})
	}
	return cleaned, nil
}

func nextActionsForTemplate(template string, targetID string) []string {
	if template == TemplateNativeExport {
		return []string{
			"edit " + path.Join(localRecipeRelRoot(targetID), "recipe.yaml") + " and replace native operation placeholder metadata",
			"dotfiles-manager app validate " + targetID,
		}
	}
	return []string{"dotfiles-manager app validate " + targetID}
}

func validateRepoWriteTarget(repoRoot string, relRoot string) error {
	if relRoot == "" || strings.HasPrefix(relRoot, "/") || strings.Contains(relRoot, "\\") {
		return typedError(CodePathUnsafe, "local recipe path must be repository-relative", 5, map[string]any{"path": relRoot})
	}
	cleaned := path.Clean(relRoot)
	if cleaned != relRoot || strings.HasPrefix(cleaned, "../") || cleaned == ".." {
		return typedError(CodePathUnsafe, "local recipe path escapes repository", 5, map[string]any{"path": relRoot})
	}
	parts := strings.Split(relRoot, "/")
	current := repoRoot
	for idx, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				break
			}
			return typedError(CodePathUnsafe, fmt.Sprintf("cannot inspect repository path %s", path.Join(parts[:idx+1]...)), 5, map[string]any{"path": path.Join(parts[:idx+1]...)})
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return typedError(CodePathUnsafe, fmt.Sprintf("repository path %s is a symlink", path.Join(parts[:idx+1]...)), 5, map[string]any{"path": path.Join(parts[:idx+1]...)})
		}
		if idx == len(parts)-1 {
			return typedError(CodeRecipeExists, fmt.Sprintf("local recipe already exists at %s", relRoot), 2, map[string]any{"path": relRoot})
		}
		if !info.IsDir() {
			return typedError(CodePathUnsafe, fmt.Sprintf("repository path %s is not a directory", path.Join(parts[:idx+1]...)), 5, map[string]any{"path": path.Join(parts[:idx+1]...)})
		}
	}
	return nil
}

func writeCreateFiles(repoRoot string, files map[string]string) error {
	rels := make([]string, 0, len(files))
	for rel := range files {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	for _, rel := range rels {
		abs := filepath.Join(repoRoot, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return typedError(CodeWriteFailed, fmt.Sprintf("create parent directory for %s: %v", rel, err), 2, map[string]any{"path": rel})
		}
		if _, err := os.Lstat(abs); err == nil {
			return typedError(CodeRecipeExists, fmt.Sprintf("file already exists at %s", rel), 2, map[string]any{"path": rel})
		} else if !errors.Is(err, os.ErrNotExist) {
			return typedError(CodeWriteFailed, fmt.Sprintf("inspect %s: %v", rel, err), 2, map[string]any{"path": rel})
		}
		if err := os.WriteFile(abs, []byte(files[rel]), 0o644); err != nil {
			return typedError(CodeWriteFailed, fmt.Sprintf("write %s: %v", rel, err), 2, map[string]any{"path": rel})
		}
	}
	return nil
}

func validateLocalRecipeReadTarget(repoRoot string, targetID string) error {
	rel := localRecipeRelPath(targetID)
	parts := strings.Split(rel, "/")
	current := repoRoot
	for idx, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		currentRel := path.Join(parts[:idx+1]...)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return typedError(CodeRecipeMissing, fmt.Sprintf("local recipe %s not found at %s", targetID, rel), 2, map[string]any{"path": rel})
			}
			return typedError(CodePathUnsafe, fmt.Sprintf("cannot inspect repository path %s", currentRel), 5, map[string]any{"path": currentRel})
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return typedError(CodePathUnsafe, fmt.Sprintf("repository path %s is a symlink", currentRel), 5, map[string]any{"path": currentRel})
		}
		if idx < len(parts)-1 {
			if !info.IsDir() {
				return typedError(CodePathUnsafe, fmt.Sprintf("repository path %s is not a directory", currentRel), 5, map[string]any{"path": currentRel})
			}
			continue
		}
		if !info.Mode().IsRegular() {
			return typedError(CodePathUnsafe, fmt.Sprintf("repository path %s is not a regular recipe file", currentRel), 5, map[string]any{"path": currentRel})
		}
	}
	return nil
}

func loadLocalRecipe(repoRoot string, targetID string) (*recipe.Recipe, error) {
	rel := localRecipeRelPath(targetID)
	file, err := os.Open(filepath.Join(repoRoot, filepath.FromSlash(rel)))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("local recipe %s not found at %s: %w", targetID, rel, os.ErrNotExist)
		}
		return nil, fmt.Errorf("read local recipe %s: %w", rel, err)
	}
	defer func() { _ = file.Close() }()
	return recipe.Decode(rel, file)
}

func diagnosticsFromRecipeError(err error) []Diagnostic {
	var out []Diagnostic
	for _, diagnostic := range recipe.ValidationDiagnostics(err) {
		out = append(out, Diagnostic{Code: diagnostic.Code, Severity: diagnostic.Severity, Message: diagnostic.Message, Path: diagnostic.Path})
	}
	return out
}

func authoringWriteSafetyDiagnostics(rec *recipe.Recipe) []Diagnostic {
	var out []Diagnostic
	for _, diagnostic := range rec.WriteSafetyDiagnostics(recipe.WriteSafetyContext{Source: recipe.RecipeSourceLocal, Trusted: false}) {
		if diagnostic.Code == "writeSafety.trust.untrusted" {
			continue
		}
		out = append(out, Diagnostic{Code: diagnostic.Code, Severity: diagnostic.Severity, Message: diagnostic.Message, Path: diagnostic.Path})
	}
	return out
}

func writeTrustRequired(surface recipe.TrustWriteSurface) bool {
	return len(surface.Settings) > 0 || len(surface.Resources) > 0 || surface.NativeOperations.Count > 0
}

func checkBundledCollision(targetID string) error {
	if target, ok := recipe.LookupBundledTarget(targetID); ok {
		return fmt.Errorf("local target %s collides with bundled target %s", targetID, target.ID)
	}
	return nil
}

func normalizeTargetID(value string) (string, error) {
	id := strings.TrimSpace(value)
	if err := recipe.ValidatePublicID("target", id); err != nil {
		return "", err
	}
	return id, nil
}

func normalizeSettingID(value string) (string, error) {
	id := strings.TrimSpace(value)
	if id == "" {
		return "", typedError(CodeFlagInvalid, "--setting is required", 2, nil)
	}
	if err := recipe.ValidatePublicID("setting", id); err != nil {
		return "", typedError(CodeFlagInvalid, err.Error(), 2, map[string]any{"setting": id})
	}
	return id, nil
}

func normalizeRepoRoot(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("repo root is required")
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
		return "", fmt.Errorf("repo root is not a directory: %s", abs)
	}
	return abs, nil
}

func localRecipeRelRoot(targetID string) string {
	return path.Join("recipes/local", targetID)
}

func localRecipeRelPath(targetID string) string {
	return path.Join(localRecipeRelRoot(targetID), "recipe.yaml")
}

func recipeRef(targetID string) string {
	return "recipe://local/" + targetID
}

func defaultDisplayName(targetID string) string {
	fields := strings.FieldsFunc(targetID, func(r rune) bool { return r == '.' || r == '-' || r == '_' })
	for idx, field := range fields {
		if field == "" {
			continue
		}
		fields[idx] = strings.ToUpper(field[:1]) + field[1:]
	}
	if len(fields) == 0 {
		return targetID
	}
	return strings.Join(fields, " ")
}

func resourceID(settingID, suffix string) string {
	base := strings.NewReplacer(".", "-", "_", "-").Replace(settingID)
	if base == "" {
		base = "setting"
	}
	id := base + "-" + suffix
	if err := recipe.ValidatePublicID("resource", id); err == nil {
		return id
	}
	return "resource-" + suffix
}

func q(value string) string {
	return strconv.Quote(value)
}

func knownScope(value string) bool {
	switch value {
	case "shared", "user", "machine", "machine-user":
		return true
	default:
		return false
	}
}

func knownLifecycle(value string) bool {
	switch value {
	case recipe.LifecycleAllowed, recipe.LifecycleWarn, recipe.LifecycleBlocked, recipe.LifecycleAskToQuit, recipe.LifecycleQuitIfRunning, recipe.LifecycleBlockIfRunning, recipe.LifecycleReopenIfStoppedByTool:
		return true
	default:
		return false
	}
}

func baseCreateReport(dryRun bool) *CreateReport {
	return &CreateReport{
		Schema:        CreateSchema,
		SchemaVersion: 1,
		Command:       CreateCommand,
		RunID:         CreateRunID,
		DryRun:        dryRun,
		Summary:       CreateSummary{Status: "ok"},
		AppCreate:     CreateResult{Files: []FileAction{}, NextActions: []string{}},
		Diagnostics:   []Diagnostic{},
	}
}

func baseValidateReport() *ValidateReport {
	return &ValidateReport{
		Schema:        ValidateSchema,
		SchemaVersion: 1,
		Command:       ValidateCommand,
		RunID:         ValidateRunID,
		Summary:       ValidateSummary{Status: "ok"},
		AppValidate:   ValidateResult{Fixtures: []FixtureCheck{}, Trust: TrustInfo{LocalTrustState: "not-checked"}},
		Diagnostics:   []Diagnostic{},
	}
}

func failCreate(report *CreateReport, code string, message string, exit int, details map[string]any) (*CreateReport, error) {
	if report == nil {
		report = baseCreateReport(false)
	}
	report.Summary.Status = "blocked"
	report.Summary.Blocked = 1
	if code == CodeWriteFailed || code == CodeRepoInvalid {
		report.Summary.Failed = 1
	}
	report.Diagnostics = append(report.Diagnostics, Diagnostic{Code: code, Severity: SeverityError, Message: message})
	report.Error = &ErrorObject{Code: code, Message: message, Details: details}
	return report, &Error{Code: code, Message: message, Exit: exit, Details: details}
}

func failValidate(report *ValidateReport, code string, message string, exit int, details map[string]any) (*ValidateReport, error) {
	if report == nil {
		report = baseValidateReport()
	}
	report.Diagnostics = append(report.Diagnostics, Diagnostic{Code: code, Severity: SeverityError, Message: message})
	return failValidateWithExistingDiagnostics(report, code, message, exit, details)
}

func failValidateWithExistingDiagnostics(report *ValidateReport, code string, message string, exit int, details map[string]any) (*ValidateReport, error) {
	if report == nil {
		report = baseValidateReport()
	}
	report.Summary.Status = "blocked"
	report.Summary.Blocked = countBlockingDiagnostics(report.Diagnostics)
	if report.Summary.Blocked == 0 {
		report.Summary.Blocked = 1
	}
	report.Summary.Warnings = countWarningDiagnostics(report.Diagnostics)
	report.Error = &ErrorObject{Code: code, Message: message, Details: details}
	return report, &Error{Code: code, Message: message, Exit: exit, Details: details}
}

func finishValidate(report *ValidateReport) {
	report.Summary.Warnings = countWarningDiagnostics(report.Diagnostics)
	if report.Summary.Status == "" {
		report.Summary.Status = "ok"
	}
}

func appendDiagnostics(lines *[]string, diagnostics []Diagnostic) {
	if len(diagnostics) == 0 {
		return
	}
	*lines = append(*lines, "diagnostics:")
	for _, diagnostic := range diagnostics {
		pathText := ""
		if diagnostic.Path != "" {
			pathText = " " + diagnostic.Path
		}
		*lines = append(*lines, fmt.Sprintf("  %s[%s]%s: %s", diagnostic.Severity, diagnostic.Code, pathText, diagnostic.Message))
	}
}

func hasBlockingDiagnostics(diagnostics []Diagnostic) bool {
	return countBlockingDiagnostics(diagnostics) > 0
}

func countBlockingDiagnostics(diagnostics []Diagnostic) int {
	count := 0
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == "" || diagnostic.Severity == SeverityError {
			count++
		}
	}
	return count
}

func countWarningDiagnostics(diagnostics []Diagnostic) int {
	count := 0
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == SeverityWarning {
			count++
		}
	}
	return count
}

func emptyDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func typedError(code string, message string, exit int, details map[string]any) *Error {
	return &Error{Code: code, Message: message, Exit: exit, Details: details}
}

func errorCode(err error) string {
	var appErr *Error
	if errors.As(err, &appErr) && appErr.Code != "" {
		return appErr.Code
	}
	return CodeWriteFailed
}

func errorExit(err error, fallback int) int {
	var appErr *Error
	if errors.As(err, &appErr) && appErr.Exit != 0 {
		return appErr.Exit
	}
	return fallback
}

func errorDetails(err error) map[string]any {
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr.Details
	}
	return nil
}
