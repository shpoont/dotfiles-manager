package listcmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	osuser "os/user"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/shpoont/dotfiles-manager/internal/v2/desired"
	"github.com/shpoont/dotfiles-manager/internal/v2/ledger"
	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/shpoont/dotfiles-manager/internal/v2/resolution"
	"gopkg.in/yaml.v3"
)

const (
	Schema  = "dotfiles-manager.v2.list"
	Command = "list"
	RunID   = "list-managed"
)

const (
	CodeRepoInvalid      = "list.repo.invalid"
	CodeProfileInvalid   = "list.profile.invalid"
	CodeSelectionInvalid = "list.selection.invalid"
	CodeIdentityInvalid  = "list.identity.invalid"
	CodeRecipeInvalid    = "list.recipe.invalid"
	CodeDesiredInvalid   = "list.desired.invalid"
)

const (
	SeverityInfo    = "info"
	SeverityWarning = "warning"
	SeverityError   = "error"
)

const (
	DesiredStateSaved    = "saved"
	DesiredStateNotSaved = "not-saved"
)

var identityIDRegexp = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

type Options struct {
	RepoRoot         string
	StateRoot        string
	MachineID        string
	UserID           string
	ExtraLayers      []string
	LocalAccountName string
}

type Report struct {
	Schema        string       `json:"schema"`
	SchemaVersion int          `json:"schemaVersion"`
	Command       string       `json:"command"`
	RunID         string       `json:"runId"`
	Summary       Summary      `json:"summary"`
	List          ListResult   `json:"list"`
	Diagnostics   []Diagnostic `json:"diagnostics"`
	Error         *ErrorObject `json:"error"`
}

type Summary struct {
	Status     string `json:"status"`
	Targets    int    `json:"targets"`
	Settings   int    `json:"settings"`
	Unresolved int    `json:"unresolved"`
	Blocked    int    `json:"blocked"`
	Failed     int    `json:"failed"`
}

type ListResult struct {
	ActiveProfileStack string           `json:"activeProfileStack"`
	ProfileStack       []string         `json:"profileStack"`
	Settings           []ManagedSetting `json:"settings"`
}

type ManagedSetting struct {
	Ref             string           `json:"ref"`
	Target          TargetInfo       `json:"target"`
	Setting         SettingInfo      `json:"setting"`
	Scope           string           `json:"scope"`
	ScopeLabel      string           `json:"scopeLabel"`
	Subject         SubjectInfo      `json:"subject"`
	SourceLayer     string           `json:"sourceLayer"`
	Artifact        string           `json:"artifact,omitempty"`
	ArtifactForm    string           `json:"artifactForm,omitempty"`
	DesiredURI      string           `json:"desiredUri,omitempty"`
	DesiredRelPath  string           `json:"desiredRelPath,omitempty"`
	DesiredState    DesiredStateInfo `json:"desiredState"`
	Resource        ResourceInfo     `json:"resource"`
	SelectorSummary string           `json:"selectorSummary,omitempty"`
	NextActions     []string         `json:"nextActions"`
	DesiredSaved    bool             `json:"-"`
}

type DesiredStateInfo struct {
	Status string `json:"status"`
	Saved  bool   `json:"saved"`
}

type TargetInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName,omitempty"`
	RecipeRef   string `json:"recipeRef,omitempty"`
}

type SettingInfo struct {
	ID    string `json:"id"`
	Label string `json:"label,omitempty"`
}

type SubjectInfo struct {
	Resolved bool     `json:"resolved"`
	ID       string   `json:"id,omitempty"`
	Missing  []string `json:"missing"`
}

type ResourceInfo struct {
	ID          string `json:"id,omitempty"`
	DriverID    string `json:"driverId,omitempty"`
	LocationID  string `json:"locationId,omitempty"`
	Path        string `json:"path,omitempty"`
	DisplayPath string `json:"displayPath,omitempty"`
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
	report := baseReport()
	repoRoot, err := resolveRepoRoot(opts.RepoRoot)
	if err != nil {
		return fail(report, CodeRepoInvalid, err.Error(), 2, nil)
	}
	stateRoot := strings.TrimSpace(opts.StateRoot)
	if stateRoot == "" {
		stateRoot, err = ledger.DefaultStateRoot(repoRoot)
		if err != nil {
			return fail(report, CodeRepoInvalid, err.Error(), 2, nil)
		}
	}

	repo, err := loadRepo(repoRoot, opts.ExtraLayers)
	if err != nil {
		return fail(report, CodeProfileInvalid, err.Error(), 2, nil)
	}
	report.List.ActiveProfileStack = repo.activeStack
	report.List.ProfileStack = append([]string(nil), repo.layerIDs...)

	ids, identityDiagnostics := resolveIdentities(stateRoot, opts)
	report.Diagnostics = append(report.Diagnostics, identityDiagnostics...)

	settings, diagnostics, err := buildSettings(repo, ids)
	if err != nil {
		return fail(report, CodeSelectionInvalid, err.Error(), 2, nil)
	}
	report.Diagnostics = append(report.Diagnostics, diagnostics...)
	report.List.Settings = settings
	finish(report)
	return report, nil
}

func JSON(report *Report) (string, error) {
	if report == nil {
		report = baseReport()
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
		return "Selected settings\n\nCommand result:\n  The command could not complete.\n"
	}
	if report.Error != nil {
		return friendlyListErrorText(report)
	}
	lines := []string{"Selected settings", ""}
	if len(report.List.Settings) == 0 {
		lines = append(lines, "No settings are selected yet.", "", "Next:", "  Discover supported settings:", "  dotfiles-manager --config dotfiles-manager.v2.yaml recipe discover")
	} else {
		groups := groupManagedSettings(report.List.Settings)
		for _, target := range sortedManagedTargets(groups) {
			lines = append(lines, target)
			for _, setting := range groups[target] {
				lines = append(lines, fmt.Sprintf("  %s — %s", setting.Ref, settingLabel(setting)))
				lines = append(lines, fmt.Sprintf("    Scope: %s — %s", setting.Scope, setting.ScopeLabel))
				if setting.Subject.Resolved {
					lines = append(lines, "    Subject: "+setting.Subject.ID)
				} else if len(setting.Subject.Missing) > 0 {
					lines = append(lines, "    Subject: unresolved; missing "+strings.Join(setting.Subject.Missing, ", "))
				}
				lines = append(lines, "    Stored settings: "+friendlyDesiredSaved(setting))
			}
			lines = append(lines, "")
		}
		if first := firstActionableListSetting(report.List.Settings); first != nil {
			lines = append(lines, "Next:")
			if first.DesiredSaved {
				lines = append(lines, "  Inspect drift:", "  "+listCommandLine("diff", first.Ref, first.Subject.ID))
			} else {
				lines = append(lines, "  Preview explicit sync from live settings to stored settings:", "  "+listCommandLine("save", first.Ref, first.Subject.ID))
			}
			lines = append(lines, "")
		}
		lines = append(lines, fmt.Sprintf("Summary: %d selected setting%s, %d unresolved.", report.Summary.Settings, plural(report.Summary.Settings), report.Summary.Unresolved))
	}
	for _, diagnostic := range report.Diagnostics {
		switch diagnostic.Severity {
		case SeverityError:
			lines = append(lines, "", "Problem:", "  "+diagnostic.Message)
		case SeverityWarning:
			lines = append(lines, "", "Warning:", "  "+diagnostic.Message)
		}
	}
	return strings.Join(trimBlank(lines), "\n")
}

func VerboseText(report *Report) string {
	return technicalText(report)
}

func technicalText(report *Report) string {
	if report == nil {
		return "list\nsummary status=error targets=0 settings=0 unresolved=0 blocked=0 failed=1"
	}
	lines := []string{"list"}
	if report.List.ActiveProfileStack != "" {
		lines = append(lines, "profile stack: "+report.List.ActiveProfileStack+" ["+strings.Join(report.List.ProfileStack, " -> ")+"]")
	}
	if len(report.List.Settings) == 0 {
		lines = append(lines, "managed settings: none")
	} else {
		lines = append(lines, "managed settings:")
		for _, setting := range report.List.Settings {
			label := setting.Setting.Label
			if label == "" {
				label = setting.Setting.ID
			}
			lines = append(lines, fmt.Sprintf("  %s %s", setting.Ref, label))
			subject := "unresolved"
			if setting.Subject.Resolved {
				subject = setting.Subject.ID
			}
			lines = append(lines, fmt.Sprintf("    scope=%s (%s) subject=%s resolved=%t sourceLayer=%s", setting.Scope, setting.ScopeLabel, subject, setting.Subject.Resolved, setting.SourceLayer))
			if len(setting.Subject.Missing) > 0 {
				lines = append(lines, "    missing identity: "+strings.Join(setting.Subject.Missing, ","))
			}
			resourceLine := fmt.Sprintf("    resource=%s driver=%s", dash(setting.Resource.ID), dash(setting.Resource.DriverID))
			if setting.Resource.LocationID != "" || setting.Resource.Path != "" {
				resourceLine += fmt.Sprintf(" location=%s", dash(listResourceLocation(setting.Resource)))
			}
			if setting.SelectorSummary != "" {
				resourceLine += " selector=" + setting.SelectorSummary
			}
			lines = append(lines, resourceLine)
			if setting.Artifact != "" {
				lines = append(lines, "    artifact="+setting.Artifact)
			}
			if setting.DesiredURI != "" {
				lines = append(lines, "    desired="+setting.DesiredURI+" status="+setting.DesiredState.Status)
			}
			if len(setting.NextActions) > 0 {
				lines = append(lines, "    next: "+strings.Join(setting.NextActions, " | "))
			}
		}
	}
	for _, diagnostic := range report.Diagnostics {
		lines = append(lines, fmt.Sprintf("%s[%s]: %s", diagnostic.Severity, diagnostic.Code, diagnostic.Message))
	}
	if report.Error != nil {
		lines = append(lines, fmt.Sprintf("error[%s]: %s", report.Error.Code, report.Error.Message))
	}
	lines = append(lines, fmt.Sprintf("summary status=%s targets=%d settings=%d unresolved=%d blocked=%d failed=%d", report.Summary.Status, report.Summary.Targets, report.Summary.Settings, report.Summary.Unresolved, report.Summary.Blocked, report.Summary.Failed))
	return strings.Join(lines, "\n")
}

func listResourceLocation(resource ResourceInfo) string {
	if strings.TrimSpace(resource.DisplayPath) != "" {
		return resource.DisplayPath
	}
	if resource.LocationID != "" || resource.Path != "" {
		return dash(resource.LocationID) + ":" + dash(resource.Path)
	}
	return "-"
}

func friendlyListErrorText(report *Report) string {
	lines := []string{"Selected settings", "", "Command result:", "  " + report.Error.Message, "", "No files changed.", "", "Run with --verbose for technical details."}
	return strings.Join(lines, "\n")
}

func groupManagedSettings(settings []ManagedSetting) map[string][]ManagedSetting {
	groups := map[string][]ManagedSetting{}
	for _, setting := range settings {
		target := setting.Target.DisplayName
		if strings.TrimSpace(target) == "" {
			target = targetDisplayName(setting.Target.ID)
		}
		groups[target] = append(groups[target], setting)
	}
	return groups
}

func sortedManagedTargets(groups map[string][]ManagedSetting) []string {
	targets := make([]string, 0, len(groups))
	for target := range groups {
		targets = append(targets, target)
	}
	sort.Strings(targets)
	return targets
}

func settingLabel(setting ManagedSetting) string {
	if strings.TrimSpace(setting.Setting.Label) != "" {
		return setting.Setting.Label
	}
	if strings.TrimSpace(setting.Setting.ID) != "" {
		return wordsFromID(setting.Setting.ID)
	}
	return setting.Ref
}

func friendlyDesiredSaved(setting ManagedSetting) string {
	switch setting.DesiredState.Status {
	case DesiredStateSaved:
		return "stored"
	case DesiredStateNotSaved:
		return "not stored yet"
	case "":
		if setting.DesiredSaved {
			return "stored"
		}
		return "not stored yet"
	default:
		return setting.DesiredState.Status
	}
}

func firstActionableListSetting(settings []ManagedSetting) *ManagedSetting {
	for i := range settings {
		if settings[i].Subject.Resolved && !settings[i].DesiredSaved {
			return &settings[i]
		}
	}
	for i := range settings {
		if settings[i].Subject.Resolved {
			return &settings[i]
		}
	}
	if len(settings) == 0 {
		return nil
	}
	return &settings[0]
}

func listCommandLine(command string, ref string, userID string) string {
	args := []string{"dotfiles-manager", "--config", resolution.RootConfigFile, command}
	if command == "save" {
		args = append(args, "--dry-run")
	}
	if strings.TrimSpace(userID) != "" && userID != "-" {
		args = append(args, "--user-id", userID)
	}
	args = append(args, ref)
	return strings.Join(args, " ")
}

func targetDisplayName(target string) string {
	switch strings.TrimSpace(target) {
	case "git":
		return "Git"
	case "starship":
		return "Starship"
	case "zsh":
		return "Zsh"
	case "tmux":
		return "tmux"
	case "nvim":
		return "Neovim"
	case "ssh":
		return "SSH"
	case "":
		return "Selected target"
	default:
		return titleWords(strings.ReplaceAll(target, ".", " "))
	}
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

func baseReport() *Report {
	return &Report{
		Schema:        Schema,
		SchemaVersion: 1,
		Command:       Command,
		RunID:         RunID,
		Summary:       Summary{Status: "ok"},
		List:          ListResult{ProfileStack: []string{}, Settings: []ManagedSetting{}},
		Diagnostics:   []Diagnostic{},
	}
}

type repoState struct {
	root        string
	activeStack string
	layerIDs    []string
	merged      map[string]mergedSelection
}

type rootConfigFile struct {
	Schema             string `yaml:"schema"`
	SchemaVersion      int    `yaml:"schemaVersion"`
	ActiveProfileStack string `yaml:"activeProfileStack"`
}

type profileStackFile struct {
	Schema        string   `yaml:"schema"`
	SchemaVersion int      `yaml:"schemaVersion"`
	ProfileStack  []string `yaml:"profileStack"`
}

type profileLayerFile struct {
	Schema        string                     `yaml:"schema"`
	SchemaVersion int                        `yaml:"schemaVersion"`
	Selections    map[string]targetSelection `yaml:"selections"`
}

type targetSelection struct {
	Settings map[string]settingSelection `yaml:"settings"`
}

type settingSelection struct {
	Scope    string `yaml:"scope"`
	Artifact string `yaml:"artifact,omitempty"`
}

type mergedSelection struct {
	TargetID    string
	SettingID   string
	SourceLayer string
	Selection   settingSelection
}

func resolveRepoRoot(repoRoot string) (string, error) {
	if strings.TrimSpace(repoRoot) != "" {
		abs, err := filepath.Abs(strings.TrimSpace(repoRoot))
		if err != nil {
			return "", err
		}
		return abs, nil
	}
	return resolution.FindRoot("")
}

func loadRepo(repoRoot string, extraLayers []string) (*repoState, error) {
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return nil, err
	}
	var cfg rootConfigFile
	if err := decodeKnownYAML(filepath.Join(root, resolution.RootConfigFile), &cfg); err != nil {
		return nil, err
	}
	if err := validateSchema("root config", cfg.Schema, cfg.SchemaVersion, "dotfiles-manager.v2.root-config"); err != nil {
		return nil, err
	}
	active, err := validateProfilePathID("active profile stack", cfg.ActiveProfileStack)
	if err != nil {
		return nil, err
	}
	var stack profileStackFile
	if err := decodeKnownYAML(filepath.Join(root, "profiles", "stacks", filepath.FromSlash(active)+".yaml"), &stack); err != nil {
		return nil, err
	}
	if err := validateSchema("profile stack", stack.Schema, stack.SchemaVersion, "dotfiles-manager.v2.profile-stack"); err != nil {
		return nil, err
	}
	layers := append([]string(nil), stack.ProfileStack...)
	layers = append(layers, extraLayers...)
	if len(layers) == 0 {
		return nil, fmt.Errorf("profile stack %q has no layers", active)
	}
	repo := &repoState{root: root, activeStack: active, merged: map[string]mergedSelection{}}
	for _, raw := range layers {
		layerID, err := validateProfilePathID("profile layer", raw)
		if err != nil {
			return nil, err
		}
		var layer profileLayerFile
		if err := decodeKnownYAML(filepath.Join(root, "profiles", "layers", filepath.FromSlash(layerID)+".yaml"), &layer); err != nil {
			return nil, err
		}
		if err := validateSchema("profile layer", layer.Schema, layer.SchemaVersion, "dotfiles-manager.v2.profile-layer"); err != nil {
			return nil, err
		}
		repo.layerIDs = append(repo.layerIDs, layerID)
		applyLayer(repo.merged, layerID, layer)
	}
	return repo, nil
}

func applyLayer(merged map[string]mergedSelection, layerID string, layer profileLayerFile) {
	for _, targetID := range sortedKeys(layer.Selections) {
		target := layer.Selections[targetID]
		for _, settingID := range sortedKeys(target.Settings) {
			merged[targetID+":"+settingID] = mergedSelection{TargetID: targetID, SettingID: settingID, SourceLayer: layerID, Selection: target.Settings[settingID]}
		}
	}
}

func decodeKnownYAML(path string, out any) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()
	dec := yaml.NewDecoder(file)
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

func validateSchema(kind string, actual string, version int, expected string) error {
	if actual != expected {
		return fmt.Errorf("invalid %s schema: %q (expected %q)", kind, actual, expected)
	}
	if version != 1 {
		return fmt.Errorf("invalid %s schemaVersion: %d (expected 1)", kind, version)
	}
	return nil
}

func validateProfilePathID(kind string, value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("%s id is required", kind)
	}
	if strings.Contains(trimmed, "\\") || filepath.IsAbs(trimmed) || strings.HasPrefix(trimmed, "/") {
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

type identityIDs struct {
	MachineID string
	UserID    string
}

func resolveIdentities(stateRoot string, opts Options) (identityIDs, []Diagnostic) {
	explicitMachineID := strings.TrimSpace(opts.MachineID)
	explicitUserID := strings.TrimSpace(opts.UserID)
	ids := identityIDs{}
	var diagnostics []Diagnostic
	if explicitMachineID != "" && !identityIDRegexp.MatchString(explicitMachineID) {
		diagnostics = append(diagnostics, Diagnostic{Code: CodeIdentityInvalid, Severity: SeverityWarning, Message: "--machine-id is invalid; machine-scoped selections remain unresolved"})
		explicitMachineID = ""
	}
	if explicitUserID != "" && !identityIDRegexp.MatchString(explicitUserID) {
		diagnostics = append(diagnostics, Diagnostic{Code: CodeIdentityInvalid, Severity: SeverityWarning, Message: "--user-id is invalid; user-scoped selections remain unresolved"})
		explicitUserID = ""
	}

	if id, err := readMachineID(filepath.Join(stateRoot, "identity", "machine.yaml")); err == nil {
		ids.MachineID = id
		if explicitMachineID != "" && explicitMachineID != id {
			diagnostics = append(diagnostics, Diagnostic{Code: CodeIdentityInvalid, Severity: SeverityWarning, Message: "conflicting --machine-id ignored because a local machine identity already exists", Path: "state://identity/machine.yaml"})
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		diagnostics = append(diagnostics, Diagnostic{Code: CodeIdentityInvalid, Severity: SeverityWarning, Message: err.Error(), Path: "state://identity/machine.yaml"})
	} else {
		ids.MachineID = explicitMachineID
	}

	localKey := localAccountKey(opts.LocalAccountName)
	if id, err := readUserID(filepath.Join(stateRoot, "identity", "users", localKey+".yaml"), localKey); err == nil {
		ids.UserID = id
		if explicitUserID != "" && explicitUserID != id {
			diagnostics = append(diagnostics, Diagnostic{Code: CodeIdentityInvalid, Severity: SeverityWarning, Message: "conflicting --user-id ignored because a local user identity already exists", Path: "state://identity/users/" + localKey + ".yaml"})
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		diagnostics = append(diagnostics, Diagnostic{Code: CodeIdentityInvalid, Severity: SeverityWarning, Message: err.Error(), Path: "state://identity/users/" + localKey + ".yaml"})
	} else {
		ids.UserID = explicitUserID
	}
	return ids, diagnostics
}

type machineIdentityFile struct {
	Schema        string `yaml:"schema"`
	SchemaVersion int    `yaml:"schemaVersion"`
	MachineID     string `yaml:"machineId"`
}

type userIdentityFile struct {
	Schema          string `yaml:"schema"`
	SchemaVersion   int    `yaml:"schemaVersion"`
	LocalAccountKey string `yaml:"localAccountKey"`
	UserID          string `yaml:"userId"`
}

func readMachineID(path string) (string, error) {
	var identity machineIdentityFile
	if err := decodeKnownYAML(path, &identity); err != nil {
		return "", err
	}
	if identity.Schema != "dotfiles-manager.v2.machine-identity" || identity.SchemaVersion != 1 || !identityIDRegexp.MatchString(identity.MachineID) {
		return "", fmt.Errorf("invalid machine identity record")
	}
	return identity.MachineID, nil
}

func readUserID(path string, localKey string) (string, error) {
	var identity userIdentityFile
	if err := decodeKnownYAML(path, &identity); err != nil {
		return "", err
	}
	if identity.Schema != "dotfiles-manager.v2.user-identity" || identity.SchemaVersion != 1 || identity.LocalAccountKey != localKey || !identityIDRegexp.MatchString(identity.UserID) {
		return "", fmt.Errorf("invalid user identity record")
	}
	return identity.UserID, nil
}

func buildSettings(repo *repoState, ids identityIDs) ([]ManagedSetting, []Diagnostic, error) {
	var settings []ManagedSetting
	var diagnostics []Diagnostic
	for _, key := range sortedKeys(repo.merged) {
		selection := repo.merged[key]
		if err := recipe.ValidatePublicID("target", selection.TargetID); err != nil {
			return nil, nil, err
		}
		if err := recipe.ValidatePublicID("setting", selection.SettingID); err != nil {
			return nil, nil, err
		}
		scope := strings.TrimSpace(selection.Selection.Scope)
		if !knownScope(scope) {
			return nil, nil, fmt.Errorf("unknown scope for %s: %s", key, scope)
		}
		meta, diag := metadataFor(repo.root, selection.TargetID, selection.SettingID)
		diagnostics = append(diagnostics, diag...)
		subject := subjectFor(scope, ids)
		item := ManagedSetting{
			Ref:             key,
			Target:          meta.target,
			Setting:         meta.setting,
			Scope:           scope,
			ScopeLabel:      ScopeLabel(scope),
			Subject:         subject,
			SourceLayer:     selection.SourceLayer,
			Artifact:        strings.TrimSpace(selection.Selection.Artifact),
			ArtifactForm:    meta.artifactForm,
			Resource:        meta.resource,
			SelectorSummary: meta.selectorSummary,
			NextActions:     nextActions(key),
			DesiredState:    desiredStateInfo(false),
		}
		if item.Setting.ID == "" {
			item.Setting.ID = selection.SettingID
		}
		if item.Target.ID == "" {
			item.Target.ID = selection.TargetID
		}
		if subject.Resolved {
			uri, relPath, err := desiredBinding(scope, subject.ID, selection.TargetID, selection.SettingID, selection.Selection.Artifact)
			if err != nil {
				return nil, nil, err
			}
			item.DesiredURI = uri
			item.DesiredRelPath = relPath
			saved, diagnostic := desiredStateSaved(repo.root, item)
			item.DesiredState = desiredStateInfo(saved)
			item.DesiredSaved = saved
			if diagnostic != nil {
				diagnostics = append(diagnostics, *diagnostic)
			}
		}
		settings = append(settings, item)
	}
	return settings, diagnostics, nil
}

func desiredStateInfo(saved bool) DesiredStateInfo {
	if saved {
		return DesiredStateInfo{Status: DesiredStateSaved, Saved: true}
	}
	return DesiredStateInfo{Status: DesiredStateNotSaved, Saved: false}
}

func desiredStateSaved(repoRoot string, item ManagedSetting) (bool, *Diagnostic) {
	if desiredURIHasFragment(item.DesiredURI) {
		read, err := desired.ReadSelectedValue(repoRoot, item.DesiredURI)
		if err != nil {
			return false, &Diagnostic{
				Code:     CodeDesiredInvalid,
				Severity: SeverityWarning,
				Message:  fmt.Sprintf("stored settings for %s are invalid; treating the setting as not stored: %s", item.Ref, err),
				Ref:      item.Ref,
				Path:     item.DesiredRelPath,
			}
		}
		return read.Status == desired.StatusPresent && read.Desired != nil, nil
	}
	return desiredArtifactExists(repoRoot, item.DesiredRelPath), nil
}

func desiredURIHasFragment(uri string) bool {
	_, fragment, ok := strings.Cut(strings.TrimSpace(uri), "#")
	return ok && strings.TrimSpace(fragment) != ""
}

func desiredArtifactExists(repoRoot string, relPath string) bool {
	trimmed := strings.TrimSpace(relPath)
	if trimmed == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(trimmed)))
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}

type metadata struct {
	target          TargetInfo
	setting         SettingInfo
	artifactForm    string
	resource        ResourceInfo
	selectorSummary string
}

func metadataFor(repoRoot string, targetID string, settingID string) (metadata, []Diagnostic) {
	var out metadata
	report, err := recipe.Explain(recipe.ExplainOptions{Target: targetID, RepoRoot: repoRoot})
	if err != nil || report == nil {
		message := "recipe metadata unavailable"
		if err != nil {
			message = err.Error()
		}
		return out, []Diagnostic{{Code: CodeRecipeInvalid, Severity: SeverityWarning, Message: message, Ref: targetID}}
	}
	explain := report.RecipeExplain
	out.target = TargetInfo{ID: explain.Target.Ref, DisplayName: explain.Target.DisplayName, RecipeRef: explain.Recipe.RecipeRef}
	resources := map[string]recipe.ExplainResource{}
	for _, resource := range explain.Resources {
		resources[resource.ID] = resource
	}
	for _, setting := range explain.Settings {
		if setting.ID != settingID {
			continue
		}
		out.setting = SettingInfo{ID: setting.ID, Label: setting.Label}
		out.artifactForm = setting.ArtifactForm
		resource := resources[setting.ResourceID]
		out.resource = ResourceInfo{ID: setting.ResourceID, DriverID: setting.Driver, LocationID: resource.LocationID, Path: resource.Path, DisplayPath: resource.DisplayPath}
		if out.resource.DriverID == "" {
			out.resource.DriverID = resource.DriverID
		}
		if resource.Selector != nil {
			out.selectorSummary = resource.Selector.Summary
		}
		return out, nil
	}
	return out, []Diagnostic{{Code: CodeRecipeInvalid, Severity: SeverityWarning, Message: "setting is not declared by recipe metadata", Ref: targetID + ":" + settingID}}
}

func subjectFor(scope string, ids identityIDs) SubjectInfo {
	subject := SubjectInfo{Resolved: true, Missing: []string{}}
	switch scope {
	case "shared":
		subject.ID = "-"
	case "user":
		if ids.UserID == "" {
			subject.Resolved = false
			subject.Missing = []string{"user-id"}
		} else {
			subject.ID = ids.UserID
		}
	case "machine":
		if ids.MachineID == "" {
			subject.Resolved = false
			subject.Missing = []string{"machine-id"}
		} else {
			subject.ID = ids.MachineID
		}
	case "machine-user":
		if ids.MachineID == "" {
			subject.Missing = append(subject.Missing, "machine-id")
		}
		if ids.UserID == "" {
			subject.Missing = append(subject.Missing, "user-id")
		}
		if len(subject.Missing) > 0 {
			subject.Resolved = false
		} else {
			subject.ID = ids.MachineID + "/" + ids.UserID
		}
	}
	return subject
}

func desiredBinding(scope string, subject string, targetID string, settingID string, rawArtifact string) (string, string, error) {
	subjectParts := []string{subject}
	if scope == "machine-user" {
		subjectParts = strings.Split(subject, "/")
	}
	targetRelParts := append([]string{"desired", scope}, subjectParts...)
	targetRelParts = append(targetRelParts, "targets", targetID)
	targetRelDir := path.Join(targetRelParts...)
	artifact := strings.TrimSpace(rawArtifact)
	if artifact == "" {
		return fmt.Sprintf("desired://%s/%s/targets/%s/settings#%s", scope, subject, targetID, settingID), filepath.FromSlash(path.Join(targetRelDir, "settings.yaml")), nil
	}
	pathPart, fragment, _ := strings.Cut(artifact, "#")
	pathPart = strings.TrimSpace(pathPart)
	fragment = strings.TrimSpace(fragment)
	if strings.Contains(pathPart, "\\") || filepath.IsAbs(pathPart) || strings.HasPrefix(pathPart, "/") || path.Clean(pathPart) != pathPart || strings.HasPrefix(pathPart, "../") || pathPart == ".." || pathPart == "." {
		return "", "", fmt.Errorf("artifact path is invalid for %s: %s", targetID+":"+settingID, artifact)
	}
	uri := ""
	switch {
	case pathPart == "settings.yaml":
		uri = fmt.Sprintf("desired://%s/%s/targets/%s/settings", scope, subject, targetID)
		if fragment != "" {
			uri += "#" + fragment
		}
	case strings.HasPrefix(pathPart, "artifacts/"):
		if fragment != "" {
			return "", "", fmt.Errorf("artifact payload path must not include a fragment: %s", artifact)
		}
		uri = fmt.Sprintf("desired://%s/%s/targets/%s/artifacts/%s", scope, subject, targetID, strings.TrimPrefix(pathPart, "artifacts/"))
	case pathPart == "manifest.yaml":
		if fragment != "" {
			return "", "", fmt.Errorf("manifest artifact must not include a fragment")
		}
		uri = fmt.Sprintf("desired://%s/%s/targets/%s/manifest", scope, subject, targetID)
	default:
		return "", "", fmt.Errorf("artifact path must be manifest.yaml, settings.yaml, or artifacts/...: %s", artifact)
	}
	return uri, filepath.FromSlash(path.Join(targetRelDir, pathPart)), nil
}

func knownScope(scope string) bool {
	switch scope {
	case "shared", "user", "machine", "machine-user":
		return true
	default:
		return false
	}
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

func nextActions(ref string) []string {
	return []string{
		"dotfiles-manager status " + ref,
		"dotfiles-manager save --dry-run " + ref,
	}
}

func finish(report *Report) {
	targets := map[string]bool{}
	for _, item := range report.List.Settings {
		targets[item.Target.ID] = true
		if !item.Subject.Resolved {
			report.Summary.Unresolved++
		}
	}
	report.Summary.Targets = len(targets)
	report.Summary.Settings = len(report.List.Settings)
	sort.Slice(report.Diagnostics, func(i, j int) bool {
		if report.Diagnostics[i].Code == report.Diagnostics[j].Code {
			if report.Diagnostics[i].Ref == report.Diagnostics[j].Ref {
				return report.Diagnostics[i].Message < report.Diagnostics[j].Message
			}
			return report.Diagnostics[i].Ref < report.Diagnostics[j].Ref
		}
		return report.Diagnostics[i].Code < report.Diagnostics[j].Code
	})
	switch {
	case report.Summary.Failed > 0:
		report.Summary.Status = "error"
	case report.Summary.Blocked > 0:
		report.Summary.Status = "blocked"
	case report.Summary.Unresolved > 0 || len(report.Diagnostics) > 0:
		report.Summary.Status = "partial"
	default:
		report.Summary.Status = "ok"
	}
}

func fail(report *Report, code string, message string, exit int, details map[string]any) (*Report, error) {
	if exit == 0 {
		exit = 1
	}
	report.Error = &ErrorObject{Code: code, Message: message, Details: details}
	report.Diagnostics = append(report.Diagnostics, Diagnostic{Code: code, Severity: SeverityError, Message: message})
	report.Summary.Failed = 1
	report.Summary.Status = "error"
	return report, &Error{Code: code, Message: message, Exit: exit, Details: details}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func dash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func localAccountKey(raw string) string {
	if strings.TrimSpace(raw) == "" {
		if current, err := osuser.Current(); err == nil && strings.TrimSpace(current.Username) != "" {
			raw = filepath.Base(current.Username)
		} else if env := strings.TrimSpace(os.Getenv("USER")); env != "" {
			raw = env
		} else {
			raw = "local-user"
		}
	}
	return safeIDCandidate(raw, "local-user")
}

func safeIDCandidate(raw string, fallback string) string {
	lower := strings.ToLower(strings.TrimSpace(raw))
	var b strings.Builder
	lastDash := false
	for _, r := range lower {
		safe := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-'
		if safe {
			b.WriteRune(r)
			lastDash = r == '-'
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	candidate := strings.Trim(b.String(), ".-_")
	if candidate == "" || !identityIDRegexp.MatchString(candidate) {
		candidate = fallback
	}
	if !identityIDRegexp.MatchString(candidate) {
		candidate = "local"
	}
	return candidate
}
