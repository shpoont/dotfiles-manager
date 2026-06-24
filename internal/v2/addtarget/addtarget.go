package addtarget

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/shpoont/dotfiles-manager/internal/v2/resolution"
	"gopkg.in/yaml.v3"
)

const (
	Command = "add"
	RunID   = "add-target"
	Schema  = "dotfiles-manager.v2.add"
)

const (
	CodeChoiceRequired      = "add.choice-required"
	CodeRepoInvalid         = "add.repo.invalid"
	CodeTargetInvalid       = "add.target.invalid"
	CodeTargetUnsupported   = "add.target.unsupported"
	CodePlatformUnsupported = "add.platform.unsupported"
	CodeProfileInvalid      = "add.profile.invalid"
	CodeSettingInvalid      = "add.setting.invalid"
	CodeScopeInvalid        = "add.scope.invalid"
	CodeSelectionConflict   = "add.selection.conflict"
	CodeLayerReadFailed     = "add.layer.read-failed"
	CodeLayerWriteFailed    = "add.layer.write-failed"
)

const (
	SeverityInfo    = "info"
	SeverityWarning = "warning"
	SeverityError   = "error"
)

type Options struct {
	RepoRoot        string
	Target          string
	Settings        []string
	Scope           string
	ProfileLayer    string
	DryRun          bool
	Yes             bool
	NonInteractive  bool
	JSONMode        bool
	Input           io.Reader
	PromptOutput    io.Writer
	DiscoverOptions recipe.DiscoverOptions
}

type Report struct {
	Schema        string       `json:"schema"`
	SchemaVersion int          `json:"schemaVersion"`
	Command       string       `json:"command"`
	RunID         string       `json:"runId"`
	DryRun        bool         `json:"dryRun"`
	Summary       Summary      `json:"summary"`
	Add           AddResult    `json:"add"`
	Diagnostics   []Diagnostic `json:"diagnostics"`
	Error         *ErrorObject `json:"error"`
}

type Summary struct {
	Status    string `json:"status"`
	Planned   int    `json:"planned"`
	Written   int    `json:"written"`
	Unchanged int    `json:"unchanged"`
	Blocked   int    `json:"blocked"`
	Failed    int    `json:"failed"`
}

type AddResult struct {
	Target                  AddTarget       `json:"target"`
	ActiveProfileStack      string          `json:"activeProfileStack,omitempty"`
	ProfileStack            []string        `json:"profileStack"`
	DestinationProfileLayer string          `json:"destinationProfileLayer,omitempty"`
	Discovery               *AddDiscovery   `json:"discovery,omitempty"`
	Settings                []SettingChoice `json:"settings"`
	MissingChoices          []MissingChoice `json:"missingChoices"`
}

type AddTarget struct {
	ID          string `json:"id,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	RecipeRef   string `json:"recipeRef,omitempty"`
}

type AddDiscovery struct {
	State         string `json:"state"`
	BinaryState   string `json:"binaryState"`
	ConfigState   string `json:"configState"`
	PlatformState string `json:"platformState"`
}

type SettingChoice struct {
	Ref             string       `json:"ref"`
	ID              string       `json:"id"`
	Label           string       `json:"label"`
	Scope           string       `json:"scope"`
	ScopeLabel      string       `json:"scopeLabel"`
	Artifact        string       `json:"artifact,omitempty"`
	ArtifactForm    string       `json:"artifactForm"`
	Action          string       `json:"action"`
	SourceLayer     string       `json:"sourceLayer,omitempty"`
	Resource        ResourceInfo `json:"resource"`
	SelectorSummary string       `json:"selectorSummary,omitempty"`
	NextActions     []string     `json:"nextActions"`
}

type ResourceInfo struct {
	ID         string `json:"id,omitempty"`
	DriverID   string `json:"driverId,omitempty"`
	LocationID string `json:"locationId,omitempty"`
	Path       string `json:"path,omitempty"`
}

type MissingChoice struct {
	Kind        string   `json:"kind"`
	Message     string   `json:"message"`
	Allowed     []string `json:"allowed"`
	Recommended []string `json:"recommended,omitempty"`
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

func Run(opts Options) (*Report, error) {
	report := baseReport(opts.DryRun)
	if opts.Input == nil {
		opts.Input = os.Stdin
	}
	if _, ok := opts.Input.(*bufio.Reader); !ok {
		opts.Input = bufio.NewReader(opts.Input)
	}

	repoRoot, err := resolveRepoRoot(opts.RepoRoot)
	if err != nil {
		return fail(report, CodeRepoInvalid, err.Error(), 2, nil)
	}

	repo, err := loadRepo(repoRoot)
	if err != nil {
		return fail(report, CodeRepoInvalid, err.Error(), 2, nil)
	}
	report.Add.ActiveProfileStack = repo.activeStack
	report.Add.ProfileStack = append([]string(nil), repo.layerIDs...)

	explain, err := recipe.Explain(recipe.ExplainOptions{Target: strings.TrimSpace(opts.Target), RepoRoot: repoRoot})
	if err != nil {
		return fail(report, CodeTargetInvalid, err.Error(), exitCode(err, 2), errorDetails(err))
	}
	target := explain.RecipeExplain.Target
	report.Add.Target = AddTarget{ID: target.Ref, DisplayName: target.DisplayName, RecipeRef: explain.RecipeExplain.Recipe.RecipeRef}
	if target.Ref == recipe.CustomFilesTarget {
		return fail(report, CodeTargetUnsupported, "custom.files requires explicit custom resource authoring and is not supported by add in this tranche", 2, map[string]any{"target": target.Ref})
	}

	discovery, err := discoverTarget(target.Ref, opts.DiscoverOptions)
	if err != nil {
		return fail(report, CodeTargetInvalid, err.Error(), exitCode(err, 2), errorDetails(err))
	}
	report.Add.Discovery = discovery
	if discovery != nil && discovery.State == recipe.DiscoverStateUnsupportedPlatform {
		return fail(report, CodePlatformUnsupported, fmt.Sprintf("target %s is not supported on this platform", target.Ref), 5, map[string]any{"target": target.Ref, "state": discovery.State})
	}

	settingsByID, selectable, recommended := selectableSettings(explain.RecipeExplain.Settings)
	if len(selectable) == 0 {
		return fail(report, CodeTargetUnsupported, fmt.Sprintf("target %s has no add-selectable settings", target.Ref), 2, map[string]any{"target": target.Ref})
	}

	interactive := isInteractive(opts)
	destinationLayer, missing, err := chooseProfile(repo, opts, interactive)
	if err != nil {
		return failWithMissing(report, err, missing)
	}
	report.Add.DestinationProfileLayer = destinationLayer.id

	selectedIDs, missing, err := chooseSettings(target.Ref, opts, selectable, recommended, settingsByID, interactive)
	if err != nil {
		return failWithMissing(report, err, missing)
	}

	choices, missing, err := chooseScopesAndArtifacts(target.Ref, opts, selectedIDs, settingsByID, resourcesByID(explain.RecipeExplain.Resources), interactive)
	if err != nil {
		return failWithMissing(report, err, missing)
	}

	if err := classifyStackConflicts(repo, target.Ref, choices); err != nil {
		report.Add.Settings = choices
		return fail(report, CodeSelectionConflict, err.Error(), 5, errorDetails(err))
	}

	planned := 0
	unchanged := 0
	for i := range choices {
		switch choices[i].Action {
		case "add":
			planned++
		case "unchanged":
			unchanged++
		}
	}
	report.Add.Settings = choices
	report.Summary.Planned = planned
	report.Summary.Unchanged = unchanged

	if !opts.DryRun && planned > 0 {
		if err := patchLayer(destinationLayer, target.Ref, choices); err != nil {
			report.Add.Settings = choices
			return fail(report, CodeLayerWriteFailed, err.Error(), 2, map[string]any{"profileLayer": destinationLayer.id})
		}
		report.Summary.Written = planned
	}

	finish(report)
	return report, nil
}

func JSON(report *Report) (string, error) {
	if report == nil {
		report = baseReport(false)
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
		return "Select app settings\n\nCommand result:\n  The command could not complete.\n"
	}
	if report.Error != nil {
		return friendlyAddErrorText(report)
	}
	targetName := addTargetDisplayName(report.Add.Target)
	targetID := strings.TrimSpace(report.Add.Target.ID)
	lines := []string{}
	if report.DryRun {
		lines = append(lines, fmt.Sprintf("Preview: select %s settings.", targetName))
	} else if report.Summary.Written > 0 {
		lines = append(lines, fmt.Sprintf("Selected %s settings.", targetName))
	} else {
		lines = append(lines, fmt.Sprintf("%s settings are already selected.", targetName))
	}
	lines = append(lines, "")
	if report.DryRun {
		lines = append(lines, "No profile files will be changed in this preview.", "")
	}
	if len(report.Add.Settings) == 0 {
		lines = append(lines, "No settings were selected.")
	} else {
		if report.DryRun {
			lines = append(lines, "Would select:")
		} else {
			lines = append(lines, "Selection:")
		}
		for _, setting := range report.Add.Settings {
			action := friendlyAddSettingAction(report, setting.Action)
			if report.DryRun && action == "Would select" {
				lines = append(lines, fmt.Sprintf("  %s — %s", setting.Ref, addSettingLabel(setting)))
			} else {
				lines = append(lines, fmt.Sprintf("  %s: %s — %s", action, setting.Ref, addSettingLabel(setting)))
			}
			lines = append(lines, fmt.Sprintf("    Scope: %s — %s", setting.Scope, setting.ScopeLabel))
			if report.Add.DestinationProfileLayer != "" {
				lines = append(lines, "    Profile layer: "+report.Add.DestinationProfileLayer)
			}
		}
	}
	if len(report.Add.MissingChoices) > 0 {
		lines = append(lines, "", "Needs input:")
		for _, choice := range report.Add.MissingChoices {
			lines = append(lines, "  "+choice.Message)
			if len(choice.Recommended) > 0 {
				lines = append(lines, "    Recommended: "+strings.Join(choice.Recommended, ", "))
			}
			if len(choice.Allowed) > 0 {
				lines = append(lines, "    Allowed: "+strings.Join(choice.Allowed, ", "))
			}
		}
	}
	lines = append(lines, "", "No live app config was changed.")
	if report.DryRun {
		if cmd := addConfirmCommandLine(report); cmd != "" {
			lines = append(lines, "", "To confirm:", "  "+cmd)
		}
	} else if first := firstAddSetting(report.Add.Settings); first != nil {
		lines = append(lines, "", "Next:", "  Preview explicit sync from live settings to stored settings:", "  "+addNextSaveCommandLine(first.Ref))
	}
	for _, diagnostic := range report.Diagnostics {
		if diagnostic.Severity == SeverityError {
			lines = append(lines, "", "Problem:", "  "+diagnostic.Message)
		}
	}
	if targetID != "" && targetID != targetName {
		lines = append(lines, "", fmt.Sprintf("Summary: %d selected setting%s for %s.", len(report.Add.Settings), addPlural(len(report.Add.Settings)), targetID))
	} else {
		lines = append(lines, "", fmt.Sprintf("Summary: %d selected setting%s.", len(report.Add.Settings), addPlural(len(report.Add.Settings))))
	}
	return strings.Join(trimBlank(lines), "\n")
}

func VerboseText(report *Report) string {
	return technicalText(report)
}

func technicalText(report *Report) string {
	if report == nil {
		return "add\nsummary status=error planned=0 written=0 unchanged=0"
	}
	lines := []string{"add " + report.Add.Target.ID}
	if report.DryRun {
		lines = append(lines, "MODE: DRY RUN (no profile files will be changed)")
	}
	if report.Add.ActiveProfileStack != "" {
		lines = append(lines, "profile stack: "+report.Add.ActiveProfileStack+" ["+strings.Join(report.Add.ProfileStack, " -> ")+"]")
	}
	if report.Add.DestinationProfileLayer != "" {
		lines = append(lines, "profile layer: "+report.Add.DestinationProfileLayer)
	}
	if report.Add.Discovery != nil {
		lines = append(lines, fmt.Sprintf("discovery: %s binary=%s config=%s platform=%s", report.Add.Discovery.State, report.Add.Discovery.BinaryState, report.Add.Discovery.ConfigState, report.Add.Discovery.PlatformState))
	}
	if len(report.Add.MissingChoices) > 0 {
		for _, choice := range report.Add.MissingChoices {
			line := fmt.Sprintf("missing choice: %s allowed=%s", choice.Kind, strings.Join(choice.Allowed, ","))
			if len(choice.Recommended) > 0 {
				line += " recommended=" + strings.Join(choice.Recommended, ",")
			}
			lines = append(lines, line)
		}
	}
	if len(report.Add.Settings) == 0 {
		lines = append(lines, "settings: none")
	}
	for _, setting := range report.Add.Settings {
		line := fmt.Sprintf("  %s action=%s scope=%s (%s)", setting.Ref, setting.Action, setting.Scope, setting.ScopeLabel)
		if setting.Artifact != "" {
			line += " artifact=" + setting.Artifact
		}
		if setting.SourceLayer != "" {
			line += " sourceLayer=" + setting.SourceLayer
		}
		lines = append(lines, line)
		resourceLine := fmt.Sprintf("    resource=%s driver=%s", dash(setting.Resource.ID), dash(setting.Resource.DriverID))
		if setting.Resource.LocationID != "" || setting.Resource.Path != "" {
			resourceLine += fmt.Sprintf(" location=%s:%s", dash(setting.Resource.LocationID), dash(setting.Resource.Path))
		}
		if setting.SelectorSummary != "" {
			resourceLine += " selector=" + setting.SelectorSummary
		}
		lines = append(lines, resourceLine)
		if len(setting.NextActions) > 0 {
			lines = append(lines, "    next: "+strings.Join(setting.NextActions, " | "))
		}
	}
	for _, diagnostic := range report.Diagnostics {
		lines = append(lines, fmt.Sprintf("%s[%s]: %s", diagnostic.Severity, diagnostic.Code, diagnostic.Message))
	}
	if report.Error != nil {
		lines = append(lines, fmt.Sprintf("error[%s]: %s", report.Error.Code, report.Error.Message))
	}
	lines = append(lines, fmt.Sprintf("summary status=%s planned=%d written=%d unchanged=%d blocked=%d failed=%d", report.Summary.Status, report.Summary.Planned, report.Summary.Written, report.Summary.Unchanged, report.Summary.Blocked, report.Summary.Failed))
	return strings.Join(lines, "\n")
}

func friendlyAddErrorText(report *Report) string {
	lines := []string{"Select app settings", "", "Command result:"}
	if report != nil && report.Error != nil {
		lines = append(lines, "  "+report.Error.Message)
	} else {
		lines = append(lines, "  The command could not complete.")
	}
	if report != nil && len(report.Add.MissingChoices) > 0 {
		lines = append(lines, "", "Needs input:")
		for _, choice := range report.Add.MissingChoices {
			lines = append(lines, "  "+choice.Message)
			if len(choice.Recommended) > 0 {
				lines = append(lines, "    Recommended: "+strings.Join(choice.Recommended, ", "))
			}
			if len(choice.Allowed) > 0 {
				lines = append(lines, "    Allowed: "+strings.Join(choice.Allowed, ", "))
			}
		}
	}
	lines = append(lines, "", "No profile files changed.", "No live app config changed.", "", "Run with --verbose for technical details.")
	return strings.Join(trimBlank(lines), "\n")
}

func addTargetDisplayName(target AddTarget) string {
	if strings.TrimSpace(target.DisplayName) != "" {
		return target.DisplayName
	}
	if strings.TrimSpace(target.ID) != "" {
		return titleWords(strings.ReplaceAll(target.ID, ".", " "))
	}
	return "app"
}

func addSettingLabel(setting SettingChoice) string {
	if strings.TrimSpace(setting.Label) != "" {
		return setting.Label
	}
	if strings.TrimSpace(setting.ID) != "" {
		return wordsFromID(setting.ID)
	}
	return setting.Ref
}

func friendlyAddSettingAction(report *Report, action string) string {
	switch action {
	case "add":
		if report != nil && report.DryRun {
			return "Would select"
		}
		return "Selected"
	case "unchanged":
		return "Already selected"
	default:
		if action == "" {
			return "Selected"
		}
		return titleWords(action)
	}
}

func addConfirmCommandLine(report *Report) string {
	if report == nil || strings.TrimSpace(report.Add.Target.ID) == "" {
		return ""
	}
	args := []string{"dotfiles-manager", "--config", resolution.RootConfigFile, "add", report.Add.Target.ID}
	for _, setting := range report.Add.Settings {
		if strings.TrimSpace(setting.ID) != "" {
			args = append(args, "--setting", setting.ID)
		}
	}
	if first := firstAddSetting(report.Add.Settings); first != nil && strings.TrimSpace(first.Scope) != "" {
		args = append(args, "--scope", first.Scope)
	}
	if strings.TrimSpace(report.Add.DestinationProfileLayer) != "" {
		args = append(args, "--profile", report.Add.DestinationProfileLayer)
	}
	args = append(args, "--yes")
	return strings.Join(args, " ")
}

func addNextSaveCommandLine(ref string) string {
	return strings.Join([]string{"dotfiles-manager", "--config", resolution.RootConfigFile, "save", "--dry-run", ref}, " ")
}

func firstAddSetting(settings []SettingChoice) *SettingChoice {
	if len(settings) == 0 {
		return nil
	}
	return &settings[0]
}

func wordsFromID(id string) string {
	parts := strings.Fields(strings.NewReplacer(".", " ", "-", " ", "_", " ").Replace(id))
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToLower(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

func titleWords(value string) string {
	parts := strings.Fields(value)
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
	}
	return strings.Join(parts, " ")
}

func addPlural(count int) string {
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

func baseReport(dryRun bool) *Report {
	return &Report{
		Schema:        Schema,
		SchemaVersion: 1,
		Command:       Command,
		RunID:         RunID,
		DryRun:        dryRun,
		Summary:       Summary{Status: "ok"},
		Add:           AddResult{ProfileStack: []string{}, Settings: []SettingChoice{}, MissingChoices: []MissingChoice{}},
		Diagnostics:   []Diagnostic{},
	}
}

func finish(report *Report) {
	sort.Slice(report.Add.Settings, func(i, j int) bool { return report.Add.Settings[i].Ref < report.Add.Settings[j].Ref })
	sortDiagnostics(report.Diagnostics)
	switch {
	case report.Summary.Blocked > 0:
		report.Summary.Status = "blocked"
	case report.Summary.Failed > 0:
		report.Summary.Status = "error"
	case report.Summary.Planned > 0 || report.Summary.Written > 0:
		report.Summary.Status = "changed"
	default:
		report.Summary.Status = "ok"
	}
}

func fail(report *Report, code string, message string, exit int, details map[string]any) (*Report, error) {
	if exit == 0 {
		exit = 1
	}
	report.Error = &ErrorObject{Code: code, Message: message, Details: details}
	diagnostic := Diagnostic{Code: code, Severity: SeverityError, Message: message}
	report.Diagnostics = append(report.Diagnostics, diagnostic)
	if exit == 4 || exit == 5 {
		report.Summary.Status = "blocked"
		report.Summary.Blocked = 1
	} else {
		report.Summary.Status = "error"
		report.Summary.Failed = 1
	}
	return report, &Error{Code: code, Message: message, Exit: exit, Details: details}
}

func failWithMissing(report *Report, err error, missing []MissingChoice) (*Report, error) {
	if len(missing) > 0 {
		report.Add.MissingChoices = missing
	}
	var addErr *Error
	if err != nil {
		if parsed, ok := err.(*Error); ok {
			addErr = parsed
		}
	}
	if addErr == nil {
		addErr = &Error{Code: CodeChoiceRequired, Message: err.Error(), Exit: 4}
	}
	details := addErr.Details
	if len(missing) > 0 {
		details = map[string]any{"missingChoices": missing}
	}
	return fail(report, addErr.Code, addErr.Message, addErr.Exit, details)
}

func resolveRepoRoot(repoRoot string) (string, error) {
	if strings.TrimSpace(repoRoot) != "" {
		return filepath.Abs(strings.TrimSpace(repoRoot))
	}
	return resolution.FindRoot("")
}

type repoState struct {
	root        string
	activeStack string
	layerIDs    []string
	layers      []layerState
}

type layerState struct {
	id   string
	path string
	root string
	info os.FileInfo
	doc  *yaml.Node
}

func loadRepo(root string) (*repoState, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := validateRegularFileNoSymlink(filepath.Join(abs, resolution.RootConfigFile)); err != nil {
		return nil, fmt.Errorf("root config invalid: %w", err)
	}
	rootDoc, _, err := loadYAMLDocument(filepath.Join(abs, resolution.RootConfigFile))
	if err != nil {
		return nil, err
	}
	if err := validateSchemaNode(rootDoc, "root config", "dotfiles-manager.v2.root-config"); err != nil {
		return nil, err
	}
	activeStack, err := requiredScalar(rootDoc, "activeProfileStack")
	if err != nil {
		return nil, err
	}
	activeStack, err = validateProfilePathID("active profile stack", activeStack)
	if err != nil {
		return nil, err
	}

	stackPath := filepath.Join(abs, "profiles", "stacks", filepath.FromSlash(activeStack)+".yaml")
	if err := ensureNoSymlinkComponentsBetween(abs, stackPath); err != nil {
		return nil, fmt.Errorf("profile stack invalid: %w", err)
	}
	if err := validateRegularFileNoSymlink(stackPath); err != nil {
		return nil, fmt.Errorf("profile stack invalid: %w", err)
	}
	stackDoc, _, err := loadYAMLDocument(stackPath)
	if err != nil {
		return nil, err
	}
	if err := validateSchemaNode(stackDoc, "profile stack", "dotfiles-manager.v2.profile-stack"); err != nil {
		return nil, err
	}
	layerIDs, err := requiredStringSequence(stackDoc, "profileStack")
	if err != nil {
		return nil, err
	}
	if len(layerIDs) == 0 {
		return nil, fmt.Errorf("profile stack %q has no layers", activeStack)
	}

	repo := &repoState{root: abs, activeStack: activeStack, layerIDs: make([]string, 0, len(layerIDs))}
	for _, rawLayerID := range layerIDs {
		layerID, err := validateProfilePathID("profile layer", rawLayerID)
		if err != nil {
			return nil, err
		}
		layerPath := filepath.Join(abs, "profiles", "layers", filepath.FromSlash(layerID)+".yaml")
		if err := ensureInside(filepath.Join(abs, "profiles", "layers"), layerPath); err != nil {
			return nil, err
		}
		if err := ensureNoSymlinkComponentsBetween(abs, layerPath); err != nil {
			return nil, fmt.Errorf("profile layer %q invalid: %w", layerID, err)
		}
		info, err := lstatRegularNoSymlink(layerPath)
		if err != nil {
			return nil, fmt.Errorf("profile layer %q invalid: %w", layerID, err)
		}
		doc, _, err := loadYAMLDocument(layerPath)
		if err != nil {
			return nil, err
		}
		if err := validateSchemaNode(doc, "profile layer", "dotfiles-manager.v2.profile-layer"); err != nil {
			return nil, err
		}
		repo.layerIDs = append(repo.layerIDs, layerID)
		repo.layers = append(repo.layers, layerState{id: layerID, path: layerPath, root: abs, info: info, doc: doc})
	}
	return repo, nil
}

func validateRegularFileNoSymlink(path string) error {
	_, err := lstatRegularNoSymlink(path)
	return err
}

func lstatRegularNoSymlink(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("symlinked file rejected: %s", path)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("not a regular file: %s", path)
	}
	return info, nil
}

func loadYAMLDocument(path string) (*yaml.Node, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, nil, fmt.Errorf("YAML document must be a mapping: %s", path)
	}
	return &doc, data, nil
}

func validateSchemaNode(doc *yaml.Node, kind string, expected string) error {
	schema, err := requiredScalar(doc, "schema")
	if err != nil {
		return err
	}
	if schema != expected {
		return fmt.Errorf("invalid %s schema: %q (expected %q)", kind, schema, expected)
	}
	versionRaw, err := requiredScalar(doc, "schemaVersion")
	if err != nil {
		return err
	}
	version, err := strconv.Atoi(versionRaw)
	if err != nil || version != 1 {
		return fmt.Errorf("invalid %s schemaVersion: %s (expected 1)", kind, versionRaw)
	}
	return nil
}

func requiredScalar(doc *yaml.Node, key string) (string, error) {
	value := mappingValue(documentMapping(doc), key)
	if value == nil || value.Kind != yaml.ScalarNode {
		return "", fmt.Errorf("%s is required", key)
	}
	return strings.TrimSpace(value.Value), nil
}

func requiredStringSequence(doc *yaml.Node, key string) ([]string, error) {
	value := mappingValue(documentMapping(doc), key)
	if value == nil || value.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("%s must be a sequence", key)
	}
	out := make([]string, 0, len(value.Content))
	for _, item := range value.Content {
		if item.Kind != yaml.ScalarNode {
			return nil, fmt.Errorf("%s entries must be scalars", key)
		}
		out = append(out, strings.TrimSpace(item.Value))
	}
	return out, nil
}

func validateProfilePathID(kind string, value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed != value {
		return "", fmt.Errorf("invalid %s: %s", kind, value)
	}
	if filepath.IsAbs(trimmed) || strings.HasPrefix(trimmed, "/") || strings.Contains(trimmed, "\\") {
		return "", fmt.Errorf("invalid %s path: %s", kind, value)
	}
	slashed := filepath.ToSlash(trimmed)
	if path.Clean(slashed) != slashed || slashed == "." || strings.HasPrefix(slashed, "../") || slashed == ".." {
		return "", fmt.Errorf("invalid %s path: %s", kind, value)
	}
	for _, segment := range strings.Split(slashed, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", fmt.Errorf("invalid %s path segment: %s", kind, value)
		}
		if err := recipe.ValidatePublicID(kind, segment); err != nil {
			return "", err
		}
	}
	return slashed, nil
}

func ensureInside(base string, candidate string) error {
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return err
	}
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(baseAbs, candidateAbs)
	if err != nil {
		return err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("path escapes base: %s", candidate)
	}
	return nil
}

func ensureNoSymlinkComponentsBetween(base string, candidate string) error {
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return err
	}
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(baseAbs, candidateAbs)
	if err != nil {
		return err
	}
	if rel == "." {
		return nil
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("path escapes base: %s", candidate)
	}
	current := baseAbs
	for _, segment := range strings.Split(rel, string(filepath.Separator)) {
		if segment == "" || segment == "." {
			continue
		}
		current = filepath.Join(current, segment)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlinked path component rejected: %s", current)
		}
	}
	return nil
}

func discoverTarget(target string, opts recipe.DiscoverOptions) (*AddDiscovery, error) {
	opts.Target = target
	report, err := recipe.Discover(opts)
	if report == nil || len(report.Discovery.Targets) == 0 {
		return nil, err
	}
	row := report.Discovery.Targets[0]
	return &AddDiscovery{State: row.State, BinaryState: row.BinaryState, ConfigState: row.ConfigState, PlatformState: row.PlatformState}, err
}

func selectableSettings(settings []recipe.ExplainSetting) (map[string]recipe.ExplainSetting, []recipe.ExplainSetting, []recipe.ExplainSetting) {
	settingsByID := map[string]recipe.ExplainSetting{}
	var selectable []recipe.ExplainSetting
	var recommended []recipe.ExplainSetting
	for _, setting := range settings {
		if setting.ID == "" {
			continue
		}
		settingsByID[setting.ID] = setting
		if !addSelectableCapability(setting.Capability) || setting.SupportLevel == "blocked" || setting.SupportLevel == "deprecated" {
			continue
		}
		selectable = append(selectable, setting)
		if strings.TrimSpace(setting.DefaultScope) != "" && strings.TrimSpace(setting.DefaultScope) != "unknown" {
			recommended = append(recommended, setting)
		}
	}
	sortSettings(selectable)
	sortSettings(recommended)
	return settingsByID, selectable, recommended
}

func sortSettings(settings []recipe.ExplainSetting) {
	sort.Slice(settings, func(i, j int) bool { return settings[i].ID < settings[j].ID })
}

func isInteractive(opts Options) bool {
	return !opts.NonInteractive && !opts.Yes && !opts.JSONMode
}

func chooseProfile(repo *repoState, opts Options, interactive bool) (layerState, []MissingChoice, error) {
	if raw := strings.TrimSpace(opts.ProfileLayer); raw != "" {
		layerID, err := validateProfilePathID("profile layer", raw)
		if err != nil {
			return layerState{}, nil, &Error{Code: CodeProfileInvalid, Message: err.Error(), Exit: 2}
		}
		for _, layer := range repo.layers {
			if layer.id == layerID {
				return layer, nil, nil
			}
		}
		return layerState{}, nil, &Error{Code: CodeProfileInvalid, Message: fmt.Sprintf("profile layer %q is not in active stack", layerID), Exit: 2, Details: map[string]any{"profileLayer": layerID, "activeStack": repo.layerIDs}}
	}
	if len(repo.layers) == 1 {
		return repo.layers[0], nil, nil
	}
	missing := []MissingChoice{{Kind: "profile", Message: "choose which active profile layer to update", Allowed: append([]string(nil), repo.layerIDs...)}}
	if !interactive {
		return layerState{}, missing, &Error{Code: CodeChoiceRequired, Message: "profile layer choice required", Exit: 4}
	}
	choice, err := promptProfile(opts, repo.layerIDs)
	if err != nil {
		return layerState{}, missing, err
	}
	for _, layer := range repo.layers {
		if layer.id == choice {
			return layer, nil, nil
		}
	}
	return layerState{}, missing, &Error{Code: CodeChoiceRequired, Message: "profile layer choice required", Exit: 4}
}

func chooseSettings(target string, opts Options, selectable []recipe.ExplainSetting, recommended []recipe.ExplainSetting, settingsByID map[string]recipe.ExplainSetting, interactive bool) ([]string, []MissingChoice, error) {
	allowedIDs := settingIDs(selectable)
	recommendedIDs := settingIDs(recommended)
	if len(opts.Settings) > 0 {
		ids, err := normalizeSettingInputs(target, opts.Settings, settingsByID)
		if err != nil {
			return nil, nil, &Error{Code: CodeSettingInvalid, Message: err.Error(), Exit: 2}
		}
		return ids, nil, nil
	}
	if len(recommendedIDs) == 0 {
		return nil, nil, &Error{Code: CodeTargetUnsupported, Message: "target has no recommended settings", Exit: 2}
	}
	if opts.Yes {
		return recommendedIDs, nil, nil
	}
	missing := []MissingChoice{{Kind: "settings", Message: "choose settings to manage", Allowed: allowedIDs, Recommended: recommendedIDs}}
	if !interactive {
		return nil, missing, &Error{Code: CodeChoiceRequired, Message: "settings choice required", Exit: 4}
	}
	ids, err := promptSettings(opts, target, selectable, recommendedIDs, settingsByID)
	if err != nil {
		return nil, missing, err
	}
	return ids, nil, nil
}

func settingIDs(settings []recipe.ExplainSetting) []string {
	ids := make([]string, 0, len(settings))
	for _, setting := range settings {
		ids = append(ids, setting.ID)
	}
	sort.Strings(ids)
	return ids
}

func normalizeSettingInputs(target string, inputs []string, settingsByID map[string]recipe.ExplainSetting) ([]string, error) {
	seen := map[string]bool{}
	var ids []string
	for _, input := range inputs {
		for _, token := range splitComma(input) {
			id := strings.TrimSpace(token)
			id = strings.TrimPrefix(id, target+":")
			if id == "" {
				continue
			}
			setting, ok := settingsByID[id]
			if !ok || !addSelectableCapability(setting.Capability) || setting.SupportLevel == "blocked" || setting.SupportLevel == "deprecated" {
				return nil, fmt.Errorf("unknown or unsupported setting %q for target %s", token, target)
			}
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("at least one setting is required")
	}
	sort.Strings(ids)
	return ids, nil
}

func addSelectableCapability(capability string) bool {
	return capability == "read-write" || capability == "export-only"
}

func chooseScopesAndArtifacts(target string, opts Options, selectedIDs []string, settingsByID map[string]recipe.ExplainSetting, resources map[string]recipe.ExplainResource, interactive bool) ([]SettingChoice, []MissingChoice, error) {
	scopeOverride := strings.TrimSpace(opts.Scope)
	if scopeOverride != "" && !knownScope(scopeOverride) {
		return nil, nil, &Error{Code: CodeScopeInvalid, Message: fmt.Sprintf("invalid scope %q", scopeOverride), Exit: 2, Details: map[string]any{"scope": scopeOverride}}
	}
	choices := make([]SettingChoice, 0, len(selectedIDs))
	for _, id := range selectedIDs {
		setting := settingsByID[id]
		scope := scopeOverride
		if scope == "" {
			scope = strings.TrimSpace(setting.DefaultScope)
		}
		if scope == "" || scope == "unknown" {
			missing := []MissingChoice{{Kind: "scope", Message: "choose a scope for " + target + ":" + id, Allowed: scopeCodes()}}
			if !interactive {
				return nil, missing, &Error{Code: CodeChoiceRequired, Message: "scope choice required", Exit: 4}
			}
			chosen, err := promptScope(opts, target+":"+id)
			if err != nil {
				return nil, missing, err
			}
			scope = chosen
		}
		if !knownScope(scope) {
			return nil, nil, &Error{Code: CodeScopeInvalid, Message: fmt.Sprintf("invalid scope %q", scope), Exit: 2}
		}
		resource := resources[setting.ResourceID]
		choices = append(choices, SettingChoice{
			Ref:             target + ":" + id,
			ID:              id,
			Label:           setting.Label,
			Scope:           scope,
			ScopeLabel:      ScopeLabel(scope),
			Artifact:        canonicalArtifactForSetting(setting),
			ArtifactForm:    setting.ArtifactForm,
			Action:          "add",
			Resource:        resourceInfo(setting, resource),
			SelectorSummary: selectorSummary(resource),
			NextActions:     nextActions(target + ":" + id),
		})
	}
	sort.Slice(choices, func(i, j int) bool { return choices[i].Ref < choices[j].Ref })
	return choices, nil, nil
}

func resourcesByID(resources []recipe.ExplainResource) map[string]recipe.ExplainResource {
	out := map[string]recipe.ExplainResource{}
	for _, resource := range resources {
		out[resource.ID] = resource
	}
	return out
}

func resourceInfo(setting recipe.ExplainSetting, resource recipe.ExplainResource) ResourceInfo {
	info := ResourceInfo{
		ID:         setting.ResourceID,
		DriverID:   setting.Driver,
		LocationID: resource.LocationID,
		Path:       resource.Path,
	}
	if info.DriverID == "" {
		info.DriverID = resource.DriverID
	}
	return info
}

func selectorSummary(resource recipe.ExplainResource) string {
	if resource.Selector == nil {
		return ""
	}
	return resource.Selector.Summary
}

func nextActions(ref string) []string {
	return []string{
		"dotfiles-manager status " + ref,
		"dotfiles-manager save --dry-run " + ref,
	}
}

func dash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func canonicalArtifactForSetting(setting recipe.ExplainSetting) string {
	switch setting.ArtifactForm {
	case "file", "file-tree", "native", "native-export", "opaque":
		return "artifacts/" + setting.ID
	default:
		return ""
	}
}

func knownScope(scope string) bool {
	switch scope {
	case "shared", "user", "machine", "machine-user":
		return true
	default:
		return false
	}
}

func scopeCodes() []string {
	return []string{"shared", "user", "machine", "machine-user"}
}

func ScopeLabel(scope string) string {
	switch scope {
	case "shared":
		return "Everyone using this repo"
	case "user":
		return "Me on all my machines"
	case "machine":
		return "This machine"
	case "machine-user":
		return "Me on this machine"
	default:
		return scope
	}
}

type existingSelection struct {
	LayerID  string
	Scope    string
	Artifact string
}

func classifyStackConflicts(repo *repoState, target string, choices []SettingChoice) error {
	for i := range choices {
		choice := &choices[i]
		existing, err := findExistingSelections(repo.layers, target, choice.ID)
		if err != nil {
			return err
		}
		if len(existing) == 0 {
			choice.Action = "add"
			continue
		}
		requestedArtifact := artifactKey(choice.ID, choice.Artifact)
		for _, current := range existing {
			if current.Scope != choice.Scope || artifactKey(choice.ID, current.Artifact) != requestedArtifact {
				return &Error{Code: CodeSelectionConflict, Message: fmt.Sprintf("%s already selected in profile layer %s with different scope or artifact", choice.Ref, current.LayerID), Exit: 5, Details: map[string]any{"setting": choice.Ref, "sourceLayer": current.LayerID}}
			}
		}
		choice.Action = "unchanged"
		choice.SourceLayer = existing[len(existing)-1].LayerID
	}
	return nil
}

func findExistingSelections(layers []layerState, target string, settingID string) ([]existingSelection, error) {
	var out []existingSelection
	for _, layer := range layers {
		settingNode, err := selectedSettingNode(layer.doc, target, settingID)
		if err != nil {
			return nil, err
		}
		if settingNode == nil {
			continue
		}
		scopeNode := mappingValue(settingNode, "scope")
		if scopeNode == nil || scopeNode.Kind != yaml.ScalarNode {
			return nil, &Error{Code: CodeSelectionConflict, Message: fmt.Sprintf("%s:%s selection in profile layer %s has no scalar scope", target, settingID, layer.id), Exit: 5}
		}
		artifact := ""
		if artifactNode := mappingValue(settingNode, "artifact"); artifactNode != nil {
			if artifactNode.Kind != yaml.ScalarNode {
				return nil, &Error{Code: CodeSelectionConflict, Message: fmt.Sprintf("%s:%s selection in profile layer %s has non-scalar artifact", target, settingID, layer.id), Exit: 5}
			}
			artifact = strings.TrimSpace(artifactNode.Value)
		}
		out = append(out, existingSelection{LayerID: layer.id, Scope: strings.TrimSpace(scopeNode.Value), Artifact: artifact})
	}
	return out, nil
}

func artifactKey(settingID string, artifact string) string {
	trimmed := strings.TrimSpace(artifact)
	if trimmed == "" {
		return "settings.yaml#" + settingID
	}
	return trimmed
}

func selectedSettingNode(doc *yaml.Node, target string, settingID string) (*yaml.Node, error) {
	root := documentMapping(doc)
	selections := mappingValue(root, "selections")
	if selections == nil {
		return nil, nil
	}
	if selections.Kind != yaml.MappingNode {
		return nil, &Error{Code: CodeSelectionConflict, Message: "profile layer selections must be a mapping", Exit: 5}
	}
	targetNode := mappingValue(selections, target)
	if targetNode == nil {
		return nil, nil
	}
	if targetNode.Kind != yaml.MappingNode {
		return nil, &Error{Code: CodeSelectionConflict, Message: "target selection must be a mapping", Exit: 5}
	}
	settings := mappingValue(targetNode, "settings")
	if settings == nil {
		return nil, nil
	}
	if settings.Kind != yaml.MappingNode {
		return nil, &Error{Code: CodeSelectionConflict, Message: "target settings selection must be a mapping", Exit: 5}
	}
	setting := mappingValue(settings, settingID)
	if setting == nil {
		return nil, nil
	}
	if setting.Kind != yaml.MappingNode {
		return nil, &Error{Code: CodeSelectionConflict, Message: "setting selection must be a mapping", Exit: 5}
	}
	return setting, nil
}

func patchLayer(layer layerState, target string, choices []SettingChoice) error {
	if err := ensureNoSymlinkComponentsBetween(layer.root, layer.path); err != nil {
		return err
	}
	info, err := lstatRegularNoSymlink(layer.path)
	if err != nil {
		return err
	}
	root := documentMapping(layer.doc)
	selections := ensureMapping(root, "selections")
	targetNode := ensureMapping(selections, target)
	settings := ensureMapping(targetNode, "settings")
	for _, choice := range choices {
		if choice.Action != "add" {
			continue
		}
		settingNode := ensureMapping(settings, choice.ID)
		setScalar(settingNode, "scope", choice.Scope)
		if choice.Artifact != "" {
			setScalar(settingNode, "artifact", choice.Artifact)
		}
	}
	return atomicWriteYAML(layer.root, layer.path, info, layer.doc)
}

func atomicWriteYAML(repoRoot string, targetPath string, info os.FileInfo, doc *yaml.Node) error {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		_ = enc.Close()
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}

	dir := filepath.Dir(targetPath)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(targetPath)+".tmp-")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := ensureNoSymlinkComponentsBetween(repoRoot, tmpPath); err != nil {
		_ = tmp.Close()
		return err
	}
	mode := info.Mode().Perm()
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := ensureNoSymlinkComponentsBetween(repoRoot, targetPath); err != nil {
		return err
	}
	return os.Rename(tmpPath, targetPath)
}

func documentMapping(doc *yaml.Node) *yaml.Node {
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		return doc.Content[0]
	}
	return doc
}

func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Kind == yaml.ScalarNode && mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

func ensureMapping(mapping *yaml.Node, key string) *yaml.Node {
	if existing := mappingValue(mapping, key); existing != nil {
		if existing.Kind != yaml.MappingNode {
			existing.Kind = yaml.MappingNode
			existing.Tag = "!!map"
			existing.Value = ""
			existing.Content = nil
		}
		return existing
	}
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	valueNode := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	mapping.Content = append(mapping.Content, keyNode, valueNode)
	return valueNode
}

func setScalar(mapping *yaml.Node, key string, value string) {
	if existing := mappingValue(mapping, key); existing != nil {
		existing.Kind = yaml.ScalarNode
		existing.Tag = "!!str"
		existing.Value = value
		existing.Content = nil
		return
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value},
	)
}

func promptProfile(opts Options, layerIDs []string) (string, error) {
	out := opts.PromptOutput
	if out == nil {
		out = io.Discard
	}
	_, _ = fmt.Fprintln(out, "Choose profile layer to update:")
	for i, id := range layerIDs {
		_, _ = fmt.Fprintf(out, "  %d) %s\n", i+1, id)
	}
	_, _ = fmt.Fprint(out, "Profile layer: ")
	line, err := readLine(opts.Input)
	if err != nil {
		return "", &Error{Code: CodeChoiceRequired, Message: "profile layer choice required", Exit: 4}
	}
	choice := strings.TrimSpace(line)
	if n, err := strconv.Atoi(choice); err == nil && n >= 1 && n <= len(layerIDs) {
		return layerIDs[n-1], nil
	}
	for _, id := range layerIDs {
		if choice == id {
			return id, nil
		}
	}
	return "", &Error{Code: CodeChoiceRequired, Message: "profile layer choice required", Exit: 4}
}

func promptSettings(opts Options, target string, selectable []recipe.ExplainSetting, recommendedIDs []string, settingsByID map[string]recipe.ExplainSetting) ([]string, error) {
	out := opts.PromptOutput
	if out == nil {
		out = io.Discard
	}
	_, _ = fmt.Fprintf(out, "Choose settings to manage for %s. Press Enter for recommended: %s\n", target, strings.Join(recommendedIDs, ","))
	for _, setting := range selectable {
		_, _ = fmt.Fprintf(out, "  - %s (%s, default scope: %s)\n", setting.ID, setting.Label, setting.DefaultScope)
	}
	_, _ = fmt.Fprint(out, "Settings: ")
	line, err := readLine(opts.Input)
	if err != nil {
		return nil, &Error{Code: CodeChoiceRequired, Message: "settings choice required", Exit: 4}
	}
	if strings.TrimSpace(line) == "" {
		return append([]string(nil), recommendedIDs...), nil
	}
	return normalizeSettingInputs(target, []string{line}, settingsByID)
}

func promptScope(opts Options, ref string) (string, error) {
	out := opts.PromptOutput
	if out == nil {
		out = io.Discard
	}
	_, _ = fmt.Fprintf(out, "Choose scope for %s:\n", ref)
	for _, scope := range scopeCodes() {
		_, _ = fmt.Fprintf(out, "  %s - %s\n", scope, ScopeLabel(scope))
	}
	_, _ = fmt.Fprint(out, "Scope: ")
	line, err := readLine(opts.Input)
	if err != nil {
		return "", &Error{Code: CodeChoiceRequired, Message: "scope choice required", Exit: 4}
	}
	scope := strings.TrimSpace(line)
	if !knownScope(scope) {
		return "", &Error{Code: CodeChoiceRequired, Message: "scope choice required", Exit: 4}
	}
	return scope, nil
}

func readLine(input io.Reader) (string, error) {
	if input == nil {
		input = os.Stdin
	}
	reader, ok := input.(*bufio.Reader)
	if !ok {
		reader = bufio.NewReader(input)
	}
	line, err := reader.ReadString('\n')
	if err != nil && len(line) == 0 {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func splitComma(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func sortDiagnostics(diagnostics []Diagnostic) {
	sort.Slice(diagnostics, func(i, j int) bool {
		return diagnostics[i].Code+diagnostics[i].Ref+diagnostics[i].Path+diagnostics[i].Message < diagnostics[j].Code+diagnostics[j].Ref+diagnostics[j].Path+diagnostics[j].Message
	})
}

func errorDetails(err error) map[string]any {
	if addErr, ok := err.(*Error); ok {
		return addErr.Details
	}
	if explainErr, ok := err.(*recipe.ExplainError); ok {
		return explainErr.Details
	}
	return nil
}

func exitCode(err error, fallback int) int {
	if exitErr, ok := err.(interface{ ExitCode() int }); ok {
		return exitErr.ExitCode()
	}
	return fallback
}
