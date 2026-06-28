package appdiscovery

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	v2catalog "github.com/shpoont/dotfiles-manager/internal/v2/catalog"
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
	RepoRoot       string
	RepoRootSet    bool
	StateRoot      string
	ManagerVersion string
	MachineID      string
	UserID         string
	ExtraLayers    []string
	Query          string
}

type Summary struct {
	Status  string `json:"status"`
	Apps    int    `json:"apps"`
	Managed int    `json:"managed"`
	Matches int    `json:"matches"`
	Failed  int    `json:"failed"`
}

type Report struct {
	Schema           string       `json:"schema"`
	SchemaVersion    int          `json:"schemaVersion"`
	Command          string       `json:"command"`
	RunID            string       `json:"runId"`
	Summary          Summary      `json:"summary"`
	Query            string       `json:"query,omitempty"`
	Catalogs         []CatalogRef `json:"catalogs,omitempty"`
	DisabledCatalogs []CatalogRef `json:"disabledCatalogs,omitempty"`
	Apps             []App        `json:"apps"`
	Diagnostics      []Diagnostic `json:"diagnostics"`
	Error            *ErrorObject `json:"error,omitempty"`
}

type CatalogRef struct {
	Name        string `json:"name"`
	SourceKind  string `json:"sourceKind"`
	Status      string `json:"status"`
	DisplayName string `json:"displayName"`
	OriginURI   string `json:"originUri"`
	HiddenApps  int    `json:"hiddenApps,omitempty"`
}

type App struct {
	ID                   string      `json:"id"`
	DisplayName          string      `json:"displayName"`
	Aliases              []string    `json:"aliases"`
	Source               string      `json:"source"`
	SourceKind           string      `json:"sourceKind"`
	SourceID             string      `json:"sourceId"`
	CatalogID            string      `json:"catalogId"`
	SourceDisplayName    string      `json:"sourceDisplayName"`
	OriginURI            string      `json:"originUri"`
	State                string      `json:"state"`
	SelectedSettings     int         `json:"selectedSettings"`
	RecipeRef            string      `json:"recipeRef"`
	TrustStatus          string      `json:"trustStatus"`
	WriteAuthority       string      `json:"writeAuthority"`
	ReviewStatus         string      `json:"reviewStatus"`
	SelectedBy           string      `json:"selectedBy"`
	SupportLevel         string      `json:"supportLevel"`
	Capability           string      `json:"capability"`
	PlatformSupport      string      `json:"platformSupport"`
	Summary              string      `json:"summary"`
	Candidates           []Candidate `json:"candidates,omitempty"`
	SourceChoiceRequired bool        `json:"sourceChoiceRequired,omitempty"`
}

type Candidate struct {
	ID                string `json:"id"`
	DisplayName       string `json:"displayName"`
	Source            string `json:"source"`
	SourceKind        string `json:"sourceKind"`
	SourceID          string `json:"sourceId"`
	CatalogID         string `json:"catalogId"`
	SourceDisplayName string `json:"sourceDisplayName"`
	OriginURI         string `json:"originUri"`
	RecipeRef         string `json:"recipeRef"`
	SelectedBy        string `json:"selectedBy"`
	TrustStatus       string `json:"trustStatus"`
	WriteAuthority    string `json:"writeAuthority"`
	ReviewStatus      string `json:"reviewStatus"`
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
	ID                   string           `json:"id"`
	DisplayName          string           `json:"displayName"`
	Source               string           `json:"source"`
	SourceKind           string           `json:"sourceKind"`
	SourceID             string           `json:"sourceId"`
	CatalogID            string           `json:"catalogId"`
	SourceDisplayName    string           `json:"sourceDisplayName"`
	OriginURI            string           `json:"originUri"`
	SourceDescription    string           `json:"sourceDescription"`
	State                string           `json:"state"`
	SelectedSettings     int              `json:"selectedSettings"`
	RecipeRef            string           `json:"recipeRef"`
	TrustStatus          string           `json:"trustStatus"`
	WriteAuthority       string           `json:"writeAuthority"`
	ReviewStatus         string           `json:"reviewStatus"`
	SelectedBy           string           `json:"selectedBy"`
	SupportLevel         string           `json:"supportLevel"`
	Capability           string           `json:"capability"`
	PlatformSupport      string           `json:"platformSupport"`
	Settings             []ExplainSetting `json:"settings"`
	DoNotManage          []string         `json:"doNotManage"`
	Candidates           []Candidate      `json:"candidates,omitempty"`
	SourceChoiceRequired bool             `json:"sourceChoiceRequired,omitempty"`
}

type ExplainSetting struct {
	Ref          string `json:"ref"`
	ID           string `json:"id"`
	Label        string `json:"label"`
	DefaultScope string `json:"defaultScope"`
	ResourceID   string `json:"resourceId,omitempty"`
	Driver       string `json:"driver,omitempty"`
	LiveLocation string `json:"liveLocation,omitempty"`
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
	discovery, err := v2catalog.Discover(catalogOptions(opts))
	if err != nil {
		appErr := mapCatalogError(err)
		return failExplainReport(report, appErr), appErr
	}
	app, ok := findEffectiveApp(discovery, query)
	if !ok || app.Default.TargetID == "" {
		appErr := &Error{Code: CodeAppNotSupported, Message: fmt.Sprintf("app not supported: %s", query), Exit: 2, Details: map[string]any{"app": query}}
		return failExplainReport(report, appErr), appErr
	}
	count := counts[app.ID]
	report.App = explainAppFromEffective(app, count)
	if app.Default.SourceKind == v2catalog.SourceKindBundled {
		recipeReport, recipeErr := v2recipe.Explain(v2recipe.ExplainOptions{Target: app.Default.TargetID, RepoRoot: opts.RepoRoot})
		if recipeErr != nil {
			appErr := mapExplainError(query, recipeErr)
			return failExplainReport(report, appErr), appErr
		}
		if recipeReport != nil {
			applyRecipeExplain(report, recipeReport)
		}
	} else {
		for _, setting := range app.Default.Settings {
			report.App.Settings = append(report.App.Settings, ExplainSetting{
				Ref:          setting.Ref,
				ID:           setting.ID,
				Label:        displayName(setting.Label, setting.ID),
				DefaultScope: setting.DefaultScope,
				ResourceID:   setting.ResourceID,
				Driver:       setting.Driver,
				LiveLocation: setting.LiveLocation,
			})
		}
	}
	for _, diagnostic := range discovery.Diagnostics {
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
	discovery, err := v2catalog.Discover(catalogOptions(opts))
	if err != nil {
		appErr := mapCatalogError(err)
		return failReport(report, appErr), appErr
	}
	report.Catalogs = catalogRefs(discovery.Sources, false)
	report.DisabledCatalogs = catalogRefs(discovery.DisabledSources, true)
	for _, effective := range discovery.Apps {
		app := appFromEffective(effective, counts[effective.ID])
		if query != "" && !matchesApp(app, query) {
			continue
		}
		report.Apps = append(report.Apps, app)
	}
	for _, diagnostic := range discovery.Diagnostics {
		report.Diagnostics = append(report.Diagnostics, Diagnostic{Code: diagnostic.Code, Severity: diagnostic.Severity, Message: diagnostic.Message, Ref: diagnostic.Ref, Path: diagnostic.Path})
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

func appFromEffective(effective v2catalog.EffectiveApp, selectedSettings int) App {
	candidate := effective.Default
	candidates := effective.Candidates
	app := App{
		ID:                   candidate.TargetID,
		DisplayName:          displayName(candidate.DisplayName, candidate.TargetID),
		Aliases:              append([]string(nil), candidate.Aliases...),
		Source:               publicSource(candidate),
		SourceKind:           candidate.SourceKind,
		SourceID:             candidate.SourceID,
		CatalogID:            candidate.CatalogID,
		SourceDisplayName:    candidate.SourceDisplayName,
		OriginURI:            candidate.OriginURI,
		State:                stateForCount(selectedSettings),
		SelectedSettings:     selectedSettings,
		RecipeRef:            candidate.RecipeRef,
		TrustStatus:          candidate.TrustStatus,
		WriteAuthority:       candidate.WriteAuthority,
		ReviewStatus:         candidate.ReviewStatus,
		SelectedBy:           candidate.SelectedBy,
		SupportLevel:         candidate.SupportLevel,
		Capability:           candidate.Capability,
		PlatformSupport:      candidate.PlatformSupport,
		Summary:              candidate.Summary,
		Candidates:           []Candidate{},
		SourceChoiceRequired: effective.Ambiguous,
	}
	for _, other := range candidates {
		app.Candidates = append(app.Candidates, candidateRef(other))
	}
	return app
}

func explainAppFromEffective(effective v2catalog.EffectiveApp, selectedSettings int) ExplainApp {
	candidate := effective.Default
	candidates := effective.Candidates
	app := ExplainApp{
		ID:                   candidate.TargetID,
		DisplayName:          displayName(candidate.DisplayName, candidate.TargetID),
		Source:               publicSource(candidate),
		SourceKind:           candidate.SourceKind,
		SourceID:             candidate.SourceID,
		CatalogID:            candidate.CatalogID,
		SourceDisplayName:    candidate.SourceDisplayName,
		OriginURI:            candidate.OriginURI,
		SourceDescription:    sourceDescriptionFromCandidate(candidate),
		State:                stateForCount(selectedSettings),
		SelectedSettings:     selectedSettings,
		RecipeRef:            candidate.RecipeRef,
		TrustStatus:          candidate.TrustStatus,
		WriteAuthority:       candidate.WriteAuthority,
		ReviewStatus:         candidate.ReviewStatus,
		SelectedBy:           candidate.SelectedBy,
		SupportLevel:         candidate.SupportLevel,
		Capability:           candidate.Capability,
		PlatformSupport:      candidate.PlatformSupport,
		Settings:             []ExplainSetting{},
		DoNotManage:          []string{},
		Candidates:           []Candidate{},
		SourceChoiceRequired: effective.Ambiguous,
	}
	for _, other := range candidates {
		app.Candidates = append(app.Candidates, candidateRef(other))
	}
	return app
}

func applyRecipeExplain(report *ExplainReport, recipeReport *v2recipe.ExplainReport) {
	if report == nil || recipeReport == nil {
		return
	}
	recipeExplain := recipeReport.RecipeExplain
	report.App.Settings = report.App.Settings[:0]
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
	report.App.DoNotManage = append([]string(nil), recipeExplain.Safety.DoNotManage...)
	for _, diagnostic := range recipeExplain.Diagnostics {
		report.Diagnostics = append(report.Diagnostics, Diagnostic{Code: diagnostic.Code, Severity: diagnostic.Severity, Message: diagnostic.Message, Ref: diagnostic.Ref, Path: diagnostic.Path})
	}
}

func matchesApp(app App, rawQuery string) bool {
	query := strings.ToLower(strings.TrimSpace(rawQuery))
	if query == "" {
		return true
	}
	fields := []string{app.ID, app.DisplayName, app.Source, app.SourceKind, app.SourceDisplayName, app.Summary, app.SupportLevel, app.Capability, app.PlatformSupport}
	fields = append(fields, app.Aliases...)
	for _, candidate := range app.Candidates {
		fields = append(fields, candidate.Source, candidate.SourceKind, candidate.SourceDisplayName, candidate.RecipeRef)
	}
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
	title := app.DisplayName + " is supported."
	if app.SourceChoiceRequired {
		title = app.DisplayName + " is supported by multiple local catalogs."
	} else if len(app.Candidates) > 0 {
		title = app.DisplayName + " is supported by multiple sources."
	} else if app.SourceKind == v2catalog.SourceKindLocal {
		title = app.DisplayName + " is supported by a local catalog."
	}
	lines := []string{title, ""}
	lines = append(lines, "App ID: "+app.ID)
	if app.SourceChoiceRequired {
		lines = append(lines, "Source: choose one local catalog")
	} else {
		lines = append(lines, "Source: "+app.SourceDescription)
	}
	lines = append(lines, "State: "+strings.ReplaceAll(app.State, "-", " "))
	if len(app.Candidates) > 0 {
		heading := "Other available sources:"
		if app.SourceChoiceRequired {
			heading = "Available sources:"
		}
		lines = append(lines, "", heading)
		for _, candidate := range app.Candidates {
			lines = append(lines, "  "+candidateDescription(candidate))
			lines = append(lines, "    Status: candidate only")
		}
	}
	if len(app.Settings) > 0 && !app.SourceChoiceRequired {
		label := "Can manage:"
		if len(app.Candidates) > 0 {
			label = "Can manage from the default source:"
		}
		lines = append(lines, "", label)
		for _, setting := range app.Settings {
			lines = append(lines, fmt.Sprintf("  %-15s %s", setting.Ref, setting.Label))
			if setting.LiveLocation != "" && app.SourceKind == v2catalog.SourceKindLocal {
				lines = append(lines, "    Live location: "+setting.LiveLocation)
			}
		}
	}
	if len(app.DoNotManage) > 0 {
		lines = append(lines, "", "Does not manage:")
		for _, item := range app.DoNotManage {
			lines = append(lines, "  "+item)
		}
	}
	lines = append(lines, "", "Why this source is used:")
	lines = append(lines, "  "+sourceReasonForApp(app))
	if app.SourceKind == v2catalog.SourceKindLocal {
		lines = append(lines, "", "Before live settings can be changed:")
		lines = append(lines, "  dotfiles-manager will show this source and the paths it wants to manage.")
		lines = append(lines, "  Local support requires write approval before it can change live settings.")
	}
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
		lines = append(lines, fmt.Sprintf("source=%s sourceKind=%s sourceId=%s catalogId=%s recipe=%s trust=%s writeAuthority=%s state=%s selectedSettings=%d", report.App.Source, report.App.SourceKind, report.App.SourceID, report.App.CatalogID, report.App.RecipeRef, report.App.TrustStatus, report.App.WriteAuthority, report.App.State, report.App.SelectedSettings))
	}
	if len(report.App.Candidates) > 0 {
		lines = append(lines, "candidates:")
		for _, candidate := range report.App.Candidates {
			lines = append(lines, fmt.Sprintf("  source=%s sourceKind=%s sourceId=%s catalogId=%s recipe=%s writeAuthority=%s", candidate.Source, candidate.SourceKind, candidate.SourceID, candidate.CatalogID, candidate.RecipeRef, candidate.WriteAuthority))
		}
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
	lines := []string{"Supported apps", "", "  APP           SOURCE              STATE"}
	for _, app := range report.Apps {
		lines = append(lines, fmt.Sprintf("  %-13s %-19s %s", app.ID, app.Source, strings.ReplaceAll(app.State, "-", " ")))
		for _, candidate := range app.Candidates {
			if candidate.SourceKind == v2catalog.SourceKindLocal {
				if app.SourceChoiceRequired {
					lines = append(lines, fmt.Sprintf("  %-13s available in %s; choose a source before writes", "", candidate.Source))
				} else {
					lines = append(lines, fmt.Sprintf("  %-13s also in %s; built-in remains default", "", candidate.Source))
				}
			}
		}
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
		lines = append(lines, "", "Use `dotfiles-manager explain <app>` to see support details and candidates.")
	}
	if len(report.Catalogs) > 0 {
		lines = append(lines, "", "Catalogs:")
		for _, catalog := range report.Catalogs {
			switch catalog.SourceKind {
			case v2catalog.SourceKindBundled:
				lines = append(lines, fmt.Sprintf("  %-9s %s  ships with dotfiles-manager", catalog.Name, catalog.Status))
			case v2catalog.SourceKindLocal:
				if catalog.Name == v2catalog.SettingsFolderName {
					lines = append(lines, fmt.Sprintf("  %-9s %s  local recipes in this settings folder", catalog.Name, catalog.Status))
				} else {
					lines = append(lines, fmt.Sprintf("  %-9s %s  local catalog", catalog.Name, catalog.Status))
				}
			}
		}
	}
	if len(report.DisabledCatalogs) > 0 {
		lines = append(lines, "", "Disabled local catalogs:")
		for _, catalog := range report.DisabledCatalogs {
			lines = append(lines, fmt.Sprintf("  %s  %d hidden apps/candidates", catalog.Name, catalog.HiddenApps))
		}
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
	lines := []string{fmt.Sprintf("Search results for %q", query), "", "  APP           SOURCE              STATE"}
	for _, app := range report.Apps {
		lines = append(lines, fmt.Sprintf("  %-13s %-19s %s", app.ID, app.Source, strings.ReplaceAll(app.State, "-", " ")))
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
		Schema:           AppsSchema,
		SchemaVersion:    SchemaVersion,
		Command:          command,
		RunID:            runID,
		Summary:          Summary{Status: "ok"},
		Catalogs:         []CatalogRef{},
		DisabledCatalogs: []CatalogRef{},
		Apps:             []App{},
		Diagnostics:      []Diagnostic{},
	}
}

func baseExplainReport() *ExplainReport {
	return &ExplainReport{
		Schema:        AppSchema,
		SchemaVersion: SchemaVersion,
		Command:       ExplainCommand,
		RunID:         ExplainRunID,
		Summary:       Summary{Status: "ok"},
		App:           ExplainApp{Settings: []ExplainSetting{}, DoNotManage: []string{}, Candidates: []Candidate{}},
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

func mapCatalogError(err error) *Error {
	var catalogErr *v2catalog.Error
	if errors.As(err, &catalogErr) {
		return &Error{Code: catalogErr.Code, Message: catalogErr.Message, Exit: catalogErr.ExitCode(), Details: catalogErr.Details}
	}
	return &Error{Code: CodeRepoInvalid, Message: err.Error(), Exit: 2}
}

func catalogOptions(opts Options) v2catalog.Options {
	return v2catalog.Options{RepoRoot: opts.RepoRoot, StateRoot: opts.StateRoot, ManagerVersion: opts.ManagerVersion}
}

func catalogRefs(sources []v2catalog.Source, disabled bool) []CatalogRef {
	refs := make([]CatalogRef, 0, len(sources))
	for _, source := range sources {
		refs = append(refs, CatalogRef{Name: source.Name, SourceKind: source.SourceKind, Status: source.Status, DisplayName: source.DisplayName, OriginURI: source.OriginURI, HiddenApps: hiddenCount(source, disabled)})
	}
	return refs
}

func hiddenCount(source v2catalog.Source, disabled bool) int {
	if !disabled {
		return 0
	}
	if source.HiddenRecipes > 0 {
		return source.HiddenRecipes
	}
	return source.ValidRecipes
}

func findEffectiveApp(discovery *v2catalog.Discovery, query string) (v2catalog.EffectiveApp, bool) {
	query = strings.ToLower(strings.TrimSpace(query))
	if discovery == nil || query == "" {
		return v2catalog.EffectiveApp{}, false
	}
	for _, app := range discovery.Apps {
		if strings.ToLower(app.ID) == query || strings.ToLower(app.Default.TargetID) == query {
			return app, true
		}
		for _, alias := range app.Default.Aliases {
			if strings.ToLower(alias) == query {
				return app, true
			}
		}
	}
	return v2catalog.EffectiveApp{}, false
}

func publicSource(candidate v2catalog.RecipeCandidate) string {
	if candidate.SourceKind == v2catalog.SourceKindBundled {
		return "built-in"
	}
	if candidate.SourceKind == v2catalog.SourceKindLocal && strings.TrimSpace(candidate.SourceName) != "" {
		return candidate.SourceName
	}
	if candidate.SourceKind == "" {
		return "unknown"
	}
	return candidate.SourceKind
}

func candidateRef(candidate v2catalog.RecipeCandidate) Candidate {
	return Candidate{
		ID:                candidate.TargetID,
		DisplayName:       candidate.DisplayName,
		Source:            publicSource(candidate),
		SourceKind:        candidate.SourceKind,
		SourceID:          candidate.SourceID,
		CatalogID:         candidate.CatalogID,
		SourceDisplayName: candidate.SourceDisplayName,
		OriginURI:         candidate.OriginURI,
		RecipeRef:         candidate.RecipeRef,
		SelectedBy:        candidate.SelectedBy,
		TrustStatus:       candidate.TrustStatus,
		WriteAuthority:    candidate.WriteAuthority,
		ReviewStatus:      candidate.ReviewStatus,
	}
}

func sourceDescriptionFromCandidate(candidate v2catalog.RecipeCandidate) string {
	if candidate.SourceKind == v2catalog.SourceKindBundled {
		return "built-in support from dotfiles-manager"
	}
	if candidate.SourceKind == v2catalog.SourceKindLocal {
		if candidate.SourceID == "local" {
			return "local support from this settings folder"
		}
		return "local catalog " + candidate.SourceName
	}
	return displayName(candidate.SourceDisplayName, candidate.SourceKind)
}

func candidateDescription(candidate Candidate) string {
	if candidate.SourceKind == v2catalog.SourceKindLocal {
		return "local catalog: " + candidate.Source
	}
	return candidate.Source
}

func sourceReasonForApp(app ExplainApp) string {
	if app.SourceKind == v2catalog.SourceKindBundled && len(app.Candidates) > 0 {
		return "Built-in support remains the default unless you explicitly choose another source. Local support cannot silently replace built-in support."
	}
	if app.SourceKind == v2catalog.SourceKindBundled {
		return fmt.Sprintf("Built-in support is the default for %s and is trusted by the dotfiles-manager release.", app.DisplayName)
	}
	if app.SourceKind == v2catalog.SourceKindLocal && len(app.Candidates) > 0 {
		return "Multiple local catalogs provide support. A source must be chosen explicitly before live settings can be changed."
	}
	if app.SourceKind == v2catalog.SourceKindLocal && app.SourceID == "local" {
		return fmt.Sprintf("Local support for %s comes from this settings folder and may require review before writes.", app.DisplayName)
	}
	if app.SourceKind == v2catalog.SourceKindLocal {
		return fmt.Sprintf("Local support for %s comes from catalog %s and requires write approval before live writes.", app.DisplayName, app.Source)
	}
	return "This source was selected from available support metadata."
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
