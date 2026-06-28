package appdiscovery

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	v2listcmd "github.com/shpoont/dotfiles-manager/internal/v2/listcmd"
	v2recipe "github.com/shpoont/dotfiles-manager/internal/v2/recipe"
)

const (
	AppsSchema      = "dotfiles-manager.v2.apps"
	AppSchema       = "dotfiles-manager.v2.app"
	SchemaVersion   = 1
	ListCommand     = "list"
	SearchCommand   = "search"
	ExplainCommand  = "explain"
	ListRunID       = "app-list"
	SearchRunID     = "app-search"
	ExplainRunID    = "app-explain"
	StateManaged    = "managed"
	StateNotManaged = "not-managed"
)

const (
	CodeRepoInvalid     = "apps.repo.invalid"
	CodeQueryInvalid    = "search.query.invalid"
	CodeAppNotSupported = "explain.app.notSupported"
)

type Options struct {
	RepoRoot    string
	RepoRootSet bool
	MachineID   string
	UserID      string
	ExtraLayers []string
	Query       string
}

type Summary struct {
	Status  string `json:"status"`
	Apps    int    `json:"apps,omitempty"`
	Managed int    `json:"managed,omitempty"`
	Matches int    `json:"matches,omitempty"`
	Failed  int    `json:"failed,omitempty"`
}

type Report struct {
	Schema        string       `json:"schema"`
	SchemaVersion int          `json:"schemaVersion"`
	Command       string       `json:"command"`
	RunID         string       `json:"runId"`
	Summary       Summary      `json:"summary"`
	Query         string       `json:"query,omitempty"`
	Apps          []App        `json:"apps"`
	Diagnostics   []Diagnostic `json:"diagnostics"`
	Error         *ErrorObject `json:"error,omitempty"`
}

type App struct {
	ID               string   `json:"id"`
	DisplayName      string   `json:"displayName"`
	Aliases          []string `json:"aliases"`
	Source           string   `json:"source"`
	State            string   `json:"state"`
	SelectedSettings int      `json:"selectedSettings"`
	RecipeRef        string   `json:"recipeRef"`
	TrustStatus      string   `json:"trustStatus"`
	SupportLevel     string   `json:"supportLevel"`
	Capability       string   `json:"capability"`
	PlatformSupport  string   `json:"platformSupport"`
	Summary          string   `json:"summary"`
}

type ExplainReport struct {
	Schema        string       `json:"schema"`
	SchemaVersion int          `json:"schemaVersion"`
	Command       string       `json:"command"`
	RunID         string       `json:"runId"`
	Summary       Summary      `json:"summary"`
	App           ExplainApp   `json:"app"`
	Diagnostics   []Diagnostic `json:"diagnostics"`
	Error         *ErrorObject `json:"error,omitempty"`
}

type ExplainApp struct {
	ID                string           `json:"id"`
	DisplayName       string           `json:"displayName"`
	Source            string           `json:"source"`
	SourceDescription string           `json:"sourceDescription"`
	State             string           `json:"state"`
	SelectedSettings  int              `json:"selectedSettings"`
	RecipeRef         string           `json:"recipeRef"`
	TrustStatus       string           `json:"trustStatus"`
	SupportLevel      string           `json:"supportLevel"`
	Capability        string           `json:"capability"`
	PlatformSupport   string           `json:"platformSupport"`
	Settings          []ExplainSetting `json:"settings"`
	DoNotManage       []string         `json:"doNotManage"`
}

type ExplainSetting struct {
	Ref          string `json:"ref"`
	ID           string `json:"id"`
	Label        string `json:"label"`
	DefaultScope string `json:"defaultScope"`
	ResourceID   string `json:"resourceId,omitempty"`
	Driver       string `json:"driver,omitempty"`
}

type Diagnostic struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Ref      string `json:"ref,omitempty"`
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

func List(opts Options) (*Report, error) {
	return listOrSearch(opts, "")
}

func Search(opts Options) (*Report, error) {
	query := strings.TrimSpace(opts.Query)
	if query == "" {
		report := baseReport(SearchCommand, SearchRunID)
		err := &Error{Code: CodeQueryInvalid, Message: "search query is required", Exit: 2}
		return failReport(report, err), err
	}
	return listOrSearch(opts, query)
}

func Explain(opts Options) (*ExplainReport, error) {
	query := strings.TrimSpace(opts.Query)
	report := baseExplainReport()
	counts, err := managedCounts(opts)
	if err != nil {
		appErr := &Error{Code: CodeRepoInvalid, Message: err.Error(), Exit: 2}
		return failExplainReport(report, appErr), appErr
	}
	recipeRoot := opts.RepoRoot
	recipeReport, recipeErr := v2recipe.Explain(v2recipe.ExplainOptions{Target: query, RepoRoot: recipeRoot})
	if recipeErr != nil {
		appErr := mapExplainError(query, recipeErr)
		return failExplainReport(report, appErr), appErr
	}
	if recipeReport == nil {
		appErr := &Error{Code: CodeAppNotSupported, Message: fmt.Sprintf("app not supported: %s", query), Exit: 2, Details: map[string]any{"app": query}}
		return failExplainReport(report, appErr), appErr
	}
	recipeExplain := recipeReport.RecipeExplain
	id := strings.TrimSpace(recipeExplain.Target.Ref)
	count := counts[id]
	report.App = ExplainApp{
		ID:                id,
		DisplayName:       displayName(recipeExplain.Target.DisplayName, id),
		Source:            appSource(recipeExplain.Recipe.Source),
		SourceDescription: sourceDescription(recipeExplain.Recipe.Source),
		State:             stateForCount(count),
		SelectedSettings:  count,
		RecipeRef:         recipeExplain.Recipe.RecipeRef,
		TrustStatus:       recipeExplain.Recipe.TrustStatus,
		SupportLevel:      recipeExplain.Target.SupportLevel,
		Capability:        recipeExplain.Target.Capability,
		PlatformSupport:   recipeExplain.Target.PlatformSupport,
		DoNotManage:       append([]string(nil), recipeExplain.Safety.DoNotManage...),
	}
	for _, setting := range recipeExplain.Settings {
		report.App.Settings = append(report.App.Settings, ExplainSetting{
			Ref:          setting.Ref,
			ID:           setting.ID,
			Label:        displayName(setting.Label, setting.ID),
			DefaultScope: setting.DefaultScope,
			ResourceID:   setting.ResourceID,
			Driver:       setting.Driver,
		})
	}
	for _, diagnostic := range recipeExplain.Diagnostics {
		report.Diagnostics = append(report.Diagnostics, Diagnostic{Code: diagnostic.Code, Severity: diagnostic.Severity, Message: diagnostic.Message, Ref: diagnostic.Ref, Path: diagnostic.Path})
	}
	report.Summary = Summary{Status: "ok", Apps: 1, Managed: boolToInt(count > 0)}
	return report, nil
}

func listOrSearch(opts Options, query string) (*Report, error) {
	command := ListCommand
	runID := ListRunID
	if query != "" {
		command = SearchCommand
		runID = SearchRunID
	}
	report := baseReport(command, runID)
	report.Query = query
	counts, err := managedCounts(opts)
	if err != nil {
		appErr := &Error{Code: CodeRepoInvalid, Message: err.Error(), Exit: 2}
		return failReport(report, appErr), appErr
	}
	for _, target := range v2recipe.ListBundledTargets() {
		app := appFromBundledTarget(target, counts[target.ID])
		if query != "" && !matchesApp(app, query) {
			continue
		}
		report.Apps = append(report.Apps, app)
	}
	sort.Slice(report.Apps, func(i, j int) bool { return report.Apps[i].ID < report.Apps[j].ID })
	report.Summary = Summary{Status: "ok", Apps: len(report.Apps)}
	for _, app := range report.Apps {
		if app.State == StateManaged {
			report.Summary.Managed++
		}
	}
	if query != "" {
		report.Summary.Matches = len(report.Apps)
	}
	return report, nil
}

func managedCounts(opts Options) (map[string]int, error) {
	counts := map[string]int{}
	if !opts.RepoRootSet || strings.TrimSpace(opts.RepoRoot) == "" {
		return counts, nil
	}
	report, err := v2listcmd.Run(v2listcmd.Options{
		RepoRoot:    opts.RepoRoot,
		MachineID:   opts.MachineID,
		UserID:      opts.UserID,
		ExtraLayers: append([]string(nil), opts.ExtraLayers...),
	})
	if err != nil {
		return counts, err
	}
	if report == nil {
		return counts, nil
	}
	for _, setting := range report.List.Settings {
		targetID := strings.TrimSpace(setting.Target.ID)
		if targetID == "" {
			targetID = strings.TrimSpace(strings.Split(setting.Ref, ":")[0])
		}
		if targetID != "" {
			counts[targetID]++
		}
	}
	return counts, nil
}

func appFromBundledTarget(target v2recipe.BundledTarget, selectedSettings int) App {
	return App{
		ID:               target.ID,
		DisplayName:      displayName(target.DisplayName, target.ID),
		Aliases:          append([]string(nil), target.Aliases...),
		Source:           appSource(target.Source),
		State:            stateForCount(selectedSettings),
		SelectedSettings: selectedSettings,
		RecipeRef:        target.RecipeRef,
		TrustStatus:      target.TrustStatus,
		SupportLevel:     target.SupportLevel,
		Capability:       target.Capability,
		PlatformSupport:  target.PlatformSupport,
		Summary:          target.Summary,
	}
}

func matchesApp(app App, rawQuery string) bool {
	query := strings.ToLower(strings.TrimSpace(rawQuery))
	if query == "" {
		return true
	}
	fields := []string{app.ID, app.DisplayName, app.Source, app.Summary, app.SupportLevel, app.Capability, app.PlatformSupport}
	fields = append(fields, app.Aliases...)
	for _, field := range fields {
		if strings.Contains(strings.ToLower(field), query) {
			return true
		}
	}
	return false
}

func JSON(report *Report) (string, error) {
	if report == nil {
		report = baseReport(ListCommand, ListRunID)
		report.Summary.Status = "error"
	}
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	return string(payload) + "\n", nil
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

func Text(report *Report) string {
	if report == nil {
		return "Supported apps\n\nCommand result:\n  The command could not complete.\n"
	}
	if report.Error != nil {
		return appsErrorText(report)
	}
	if report.Command == SearchCommand {
		return searchText(report)
	}
	return listText(report)
}

func ExplainText(report *ExplainReport) string {
	if report == nil {
		return "App support\n\nCommand result:\n  The command could not complete.\n"
	}
	if report.Error != nil {
		app := ""
		if report.Error.Details != nil {
			if value, ok := report.Error.Details["app"].(string); ok {
				app = value
			}
		}
		if app == "" {
			app = "unknown"
		}
		return strings.Join([]string{
			"App not supported: " + app,
			"",
			"Try:",
			"  dotfiles-manager search " + app,
			"  dotfiles-manager list",
			"",
			"No live settings were read or changed.",
			"No stored settings were changed.",
		}, "\n")
	}
	app := report.App
	lines := []string{app.DisplayName + " is supported.", ""}
	lines = append(lines, "App ID: "+app.ID)
	lines = append(lines, "Source: "+app.SourceDescription)
	lines = append(lines, "State: "+strings.ReplaceAll(app.State, "-", " "))
	if len(app.Settings) > 0 {
		lines = append(lines, "", "Can manage:")
		for _, setting := range app.Settings {
			lines = append(lines, fmt.Sprintf("  %-15s %s", setting.Ref, setting.Label))
		}
	}
	if len(app.DoNotManage) > 0 {
		lines = append(lines, "", "Does not manage:")
		for _, item := range app.DoNotManage {
			lines = append(lines, "  "+item)
		}
	}
	lines = append(lines, "", "Why this source is used:")
	lines = append(lines, "  "+sourceReason(app.Source, app.DisplayName))
	lines = append(lines, "", "No live values were printed.", "No live settings were changed.", "No stored settings were changed.")
	return strings.Join(trimBlank(lines), "\n")
}

func ExplainVerboseText(report *ExplainReport) string {
	if report == nil {
		return "explain\nsummary status=error apps=0 failed=1"
	}
	lines := []string{"explain"}
	if report.App.ID != "" {
		lines = append(lines, "app: "+report.App.ID)
		lines = append(lines, fmt.Sprintf("source=%s recipe=%s trust=%s state=%s selectedSettings=%d", report.App.Source, report.App.RecipeRef, report.App.TrustStatus, report.App.State, report.App.SelectedSettings))
	}
	if len(report.App.Settings) > 0 {
		lines = append(lines, "settings:")
		for _, setting := range report.App.Settings {
			lines = append(lines, fmt.Sprintf("  %s scope=%s resource=%s driver=%s", setting.Ref, setting.DefaultScope, setting.ResourceID, setting.Driver))
		}
	}
	if len(report.App.DoNotManage) > 0 {
		lines = append(lines, "do not manage:")
		for _, item := range report.App.DoNotManage {
			lines = append(lines, "  "+item)
		}
	}
	for _, diagnostic := range report.Diagnostics {
		lines = append(lines, fmt.Sprintf("%s[%s]: %s", diagnostic.Severity, diagnostic.Code, diagnostic.Message))
	}
	if report.Error != nil {
		lines = append(lines, fmt.Sprintf("error[%s]: %s", report.Error.Code, report.Error.Message))
	}
	lines = append(lines, fmt.Sprintf("summary status=%s apps=%d managed=%d failed=%d", report.Summary.Status, report.Summary.Apps, report.Summary.Managed, report.Summary.Failed))
	return strings.Join(lines, "\n")
}

func listText(report *Report) string {
	lines := []string{"Supported apps", "", "  APP           SOURCE     STATE"}
	for _, app := range report.Apps {
		lines = append(lines, fmt.Sprintf("  %-13s %-9s %s", app.ID, app.Source, strings.ReplaceAll(app.State, "-", " ")))
	}
	managed := managedApps(report.Apps)
	if len(managed) > 0 {
		lines = append(lines, "", "Managed apps:")
		for _, app := range managed {
			lines = append(lines, fmt.Sprintf("  %s  %d selected setting%s", app.ID, app.SelectedSettings, plural(app.SelectedSettings)))
		}
		if first := managed[0]; first.ID != "" {
			lines = append(lines, "", fmt.Sprintf("Use `dotfiles-manager status %s` to inspect drift for %s.", first.ID, first.DisplayName))
		}
	} else {
		lines = append(lines, "", "Use `dotfiles-manager explain <app>` to see what can be managed.")
	}
	lines = append(lines, "Use `dotfiles-manager list --settings` to list selected managed settings.")
	lines = append(lines, "", "No live settings were read or changed.", "No stored settings were changed.")
	return strings.Join(trimBlank(lines), "\n")
}

func searchText(report *Report) string {
	query := report.Query
	if len(report.Apps) == 0 {
		return strings.Join([]string{
			fmt.Sprintf("No supported apps found for %q.", query),
			"",
			"Try:",
			"  dotfiles-manager list",
			"",
			"No live settings were read or changed.",
			"No stored settings were changed.",
		}, "\n")
	}
	lines := []string{fmt.Sprintf("Search results for %q", query), "", "  APP           SOURCE     STATE"}
	for _, app := range report.Apps {
		lines = append(lines, fmt.Sprintf("  %-13s %-9s %s", app.ID, app.Source, strings.ReplaceAll(app.State, "-", " ")))
	}
	lines = append(lines, "", fmt.Sprintf("Use `dotfiles-manager explain %s` to see what can be managed.", report.Apps[0].ID))
	lines = append(lines, "No live settings were read or changed.")
	lines = append(lines, "No stored settings were changed.")
	return strings.Join(trimBlank(lines), "\n")
}

func appsErrorText(report *Report) string {
	message := "The command could not complete."
	if report != nil && report.Error != nil {
		message = report.Error.Message
	}
	return strings.Join([]string{"Supported apps", "", "Command result:", "  " + message, "", "No files changed.", "", "Run with --json for machine-readable details."}, "\n")
}

func baseReport(command string, runID string) *Report {
	return &Report{
		Schema:        AppsSchema,
		SchemaVersion: SchemaVersion,
		Command:       command,
		RunID:         runID,
		Summary:       Summary{Status: "ok"},
		Apps:          []App{},
		Diagnostics:   []Diagnostic{},
	}
}

func baseExplainReport() *ExplainReport {
	return &ExplainReport{
		Schema:        AppSchema,
		SchemaVersion: SchemaVersion,
		Command:       ExplainCommand,
		RunID:         ExplainRunID,
		Summary:       Summary{Status: "ok"},
		App:           ExplainApp{Settings: []ExplainSetting{}, DoNotManage: []string{}},
		Diagnostics:   []Diagnostic{},
	}
}

func failReport(report *Report, err *Error) *Report {
	if report == nil {
		report = baseReport(ListCommand, ListRunID)
	}
	report.Summary.Status = "error"
	report.Summary.Failed = 1
	if err != nil {
		report.Error = &ErrorObject{Code: err.Code, Message: err.Message, Details: err.Details}
	}
	return report
}

func failExplainReport(report *ExplainReport, err *Error) *ExplainReport {
	if report == nil {
		report = baseExplainReport()
	}
	report.Summary.Status = "error"
	report.Summary.Failed = 1
	if err != nil {
		report.Error = &ErrorObject{Code: err.Code, Message: err.Message, Details: err.Details}
	}
	return report
}

func managedApps(apps []App) []App {
	var out []App
	for _, app := range apps {
		if app.State == StateManaged {
			out = append(out, app)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func mapExplainError(query string, err error) *Error {
	if explainErr, ok := err.(*v2recipe.ExplainError); ok {
		if explainErr.Code == v2recipe.ExplainCodeUnknownTarget {
			return &Error{Code: CodeAppNotSupported, Message: fmt.Sprintf("app not supported: %s", query), Exit: 2, Details: map[string]any{"app": query}}
		}
		return &Error{Code: "explain." + explainErr.Code, Message: explainErr.Message, Exit: explainErr.ExitCode(), Details: explainErr.Details}
	}
	return &Error{Code: "explain.failed", Message: err.Error(), Exit: 1, Details: map[string]any{"app": query}}
}

func appSource(source string) string {
	switch source {
	case v2recipe.RecipeSourceBundled:
		return "built-in"
	case v2recipe.RecipeSourceLocal:
		return "local"
	case "":
		return "unknown"
	default:
		return source
	}
}

func sourceDescription(source string) string {
	switch source {
	case v2recipe.RecipeSourceBundled:
		return "built-in support from dotfiles-manager"
	case v2recipe.RecipeSourceLocal:
		return "local support from this settings folder"
	default:
		return displayName(source, "unknown")
	}
}

func sourceReason(source string, display string) string {
	switch source {
	case "built-in":
		return fmt.Sprintf("Built-in support is the default for %s and is trusted by the dotfiles-manager release.", display)
	case "local":
		return fmt.Sprintf("Local support for %s comes from this settings folder and may require review before writes.", display)
	default:
		return "This source was selected from available support metadata."
	}
}

func stateForCount(count int) string {
	if count > 0 {
		return StateManaged
	}
	return StateNotManaged
}

func displayName(value string, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed != "" {
		return trimmed
	}
	fallback = strings.TrimSpace(fallback)
	if fallback == "" {
		return "App"
	}
	parts := strings.Fields(strings.NewReplacer(".", " ", "-", " ", "_", " ").Replace(fallback))
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
	}
	return strings.Join(parts, " ")
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func plural(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

func trimBlank(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
