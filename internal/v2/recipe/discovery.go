package recipe

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

const (
	DiscoverCommand = "recipe.discover"
	DiscoverRunID   = "recipe-discover"
)

const (
	DiscoverStateUnsupportedPlatform = "unsupported-platform"
	DiscoverStateAmbiguous           = "ambiguous"
	DiscoverStateConfigPresent       = "config-present"
	DiscoverStateInstalled           = "installed"
	DiscoverStateConfigMissing       = "config-missing"
	DiscoverStateNotApplicable       = "not-applicable"

	DiscoverPlatformSupported   = "supported"
	DiscoverPlatformUnsupported = "unsupported"
	DiscoverPlatformUnknown     = "unknown"

	DiscoverBinaryInstalled     = "installed"
	DiscoverBinaryMissing       = "missing"
	DiscoverBinaryAmbiguous     = "ambiguous"
	DiscoverBinaryNotApplicable = "not-applicable"

	DiscoverConfigPresent       = "config-present"
	DiscoverConfigMissing       = "config-missing"
	DiscoverConfigAmbiguous     = "ambiguous"
	DiscoverConfigNotApplicable = "not-applicable"

	DiscoverProbeCommand = "command"
	DiscoverProbeConfig  = "config"

	DiscoverProbeInstalled = "installed"
	DiscoverProbePresent   = "present"
	DiscoverProbeMissing   = "missing"
	DiscoverProbeAmbiguous = "ambiguous"
)

const (
	DiscoverCodeUnknownTarget        = "recipe.discover.unknown-target"
	DiscoverCodeUnsupportedPlatform  = "recipe.discover.unsupported-platform"
	DiscoverCodeLocationInvalid      = "recipe.discover.location-invalid"
	DiscoverCodeResourcePathInvalid  = "recipe.discover.resource-path-invalid"
	DiscoverCodeConfigStatError      = "recipe.discover.config.stat-error"
	DiscoverCodeConfigTypeMismatch   = "recipe.discover.config.type-mismatch"
	DiscoverCodeConfigSymlinkBlocked = "recipe.discover.config.symlink-unsupported"
	DiscoverCodeCommandLookupError   = "recipe.discover.command.lookup-error"
)

type DiscoverOptions struct {
	RepoRoot       string
	Target         string
	GOOS           string
	LocationRoots  map[string]string
	PathEnv        string
	CommandLookup  func(string, string) (string, error)
	Lstat          func(string) (os.FileInfo, error)
	UserHomeExpand func(string) (string, error)
}

type DiscoverReport struct {
	Schema        string              `json:"schema"`
	SchemaVersion int                 `json:"schemaVersion"`
	Command       string              `json:"command"`
	RunID         string              `json:"runId"`
	Summary       ExplainSummary      `json:"summary"`
	Items         []any               `json:"items"`
	Discovery     DiscoveryResult     `json:"discovery"`
	Error         *ExplainErrorObject `json:"error,omitempty"`
}

type DiscoveryResult struct {
	Targets     []DiscoveredTarget  `json:"targets"`
	Diagnostics []ExplainDiagnostic `json:"diagnostics"`
}

type DiscoveredTarget struct {
	ID              string                  `json:"id"`
	DisplayName     string                  `json:"displayName"`
	Aliases         []string                `json:"aliases"`
	Source          string                  `json:"source"`
	RecipeRef       string                  `json:"recipeRef"`
	TrustStatus     string                  `json:"trustStatus"`
	Version         string                  `json:"version"`
	SupportLevel    string                  `json:"supportLevel"`
	Capability      string                  `json:"capability"`
	PlatformSupport string                  `json:"platformSupport"`
	Summary         string                  `json:"summary"`
	State           string                  `json:"state"`
	PlatformState   string                  `json:"platformState"`
	BinaryState     string                  `json:"binaryState"`
	ConfigState     string                  `json:"configState"`
	CommandProbes   []DiscoveryCommandProbe `json:"commandProbes"`
	ConfigProbes    []DiscoveryConfigProbe  `json:"configProbes"`
	Diagnostics     []ExplainDiagnostic     `json:"diagnostics"`
}

type DiscoveryCommandProbe struct {
	Kind    string `json:"kind"`
	Command string `json:"command"`
	State   string `json:"state"`
}

type DiscoveryConfigProbe struct {
	Kind         string   `json:"kind"`
	ID           string   `json:"id"`
	LocationID   string   `json:"locationId"`
	Path         string   `json:"path"`
	ExpectedType string   `json:"expectedType"`
	ActualType   string   `json:"actualType,omitempty"`
	State        string   `json:"state"`
	Resources    []string `json:"resources"`
}

type configProbePlan struct {
	id            string
	locationID    string
	path          string
	expectedType  string
	absPath       string
	resources     []string
	rejectSymlink bool
}

func Discover(opts DiscoverOptions) (*DiscoverReport, error) {
	report := baseDiscoverReport()
	targets, err := discoverTargets(opts.Target)
	if err != nil {
		diagnostic := ExplainDiagnostic{Code: DiscoverCodeUnknownTarget, Severity: ExplainSeverityError, Message: err.Error(), Ref: strings.TrimSpace(opts.Target)}
		report.Summary.Status = "error"
		report.Summary.Failed = 1
		report.Discovery.Diagnostics = []ExplainDiagnostic{diagnostic}
		report.Error = &ExplainErrorObject{Code: diagnostic.Code, Message: diagnostic.Message, Details: map[string]any{"target": strings.TrimSpace(opts.Target)}}
		return report, &ExplainError{Code: diagnostic.Code, Message: diagnostic.Message, Exit: 2, Details: report.Error.Details}
	}
	for _, target := range targets {
		report.Discovery.Targets = append(report.Discovery.Targets, discoverTarget(target, opts))
	}
	sort.Slice(report.Discovery.Targets, func(i, j int) bool {
		return report.Discovery.Targets[i].ID < report.Discovery.Targets[j].ID
	})
	return finishDiscoverReport(report), nil
}

func DiscoverJSON(report *DiscoverReport) (string, error) {
	if report == nil {
		report = baseDiscoverReport()
		report.Summary.Status = "error"
	}
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	return string(payload) + "\n", nil
}

func DiscoverText(report *DiscoverReport) string {
	if report == nil {
		return "Discover supported app settings\n\nCommand result:\n  The command could not complete.\n"
	}
	if report.Error != nil {
		return friendlyDiscoverErrorText(report)
	}
	lines := []string{"Discover supported app settings", ""}
	if len(report.Discovery.Targets) == 0 {
		lines = append(lines, "No supported apps were found in the selected recipe set.")
	} else {
		for _, target := range report.Discovery.Targets {
			lines = append(lines, friendlyDiscoverTargetName(target))
			lines = append(lines, "  Detected: "+friendlyDiscoverState(target))
			if len(target.CommandProbes) > 0 {
				lines = append(lines, "  Commands:")
				for _, probe := range target.CommandProbes {
					lines = append(lines, fmt.Sprintf("    %s — %s", probe.Command, friendlyProbeState(probe.State)))
				}
			}
			if len(target.ConfigProbes) > 0 {
				lines = append(lines, "  Config files:")
				for _, probe := range target.ConfigProbes {
					lines = append(lines, fmt.Sprintf("    %s — %s", friendlyProbeLocation(probe.LocationID, probe.Path), friendlyProbeState(probe.State)))
				}
			}
			if explain, ok := bundledExplain(target.ID); ok {
				if len(explain.Settings) > 0 {
					lines = append(lines, "  Supported settings:")
					for _, setting := range explain.Settings {
						lines = append(lines, fmt.Sprintf("    %s — %s", setting.Ref, friendlySettingLabel(setting)))
					}
				}
				if len(explain.Safety.DoNotManage) > 0 {
					lines = append(lines, "  Not managed:")
					for _, item := range explain.Safety.DoNotManage {
						lines = append(lines, "    - "+item)
					}
				}
			}
			for _, diagnostic := range target.Diagnostics {
				if diagnostic.Severity == ExplainSeverityError {
					lines = append(lines, "  Problem: "+diagnostic.Message)
				}
			}
			if first := friendlyFirstDiscoverSetting(target.ID); first != nil {
				lines = append(lines, "  Next:", "    "+friendlyRecipeAddCommand(target.ID, first.ID, first.DefaultScope))
			}
			lines = append(lines, "")
		}
	}
	if len(report.Discovery.Diagnostics) > 0 {
		lines = append(lines, "Problems:")
		for _, diagnostic := range report.Discovery.Diagnostics {
			lines = append(lines, "  "+diagnostic.Message)
		}
		lines = append(lines, "")
	}
	lines = append(lines, fmt.Sprintf("Summary: %d app%s checked.", len(report.Discovery.Targets), pluralWord(len(report.Discovery.Targets))))
	lines = append(lines, "Use --verbose for detection probe details.")
	return strings.Join(trimBlank(lines), "\n")
}

func DiscoverVerboseText(report *DiscoverReport) string {
	return discoverTechnicalText(report)
}

func discoverTechnicalText(report *DiscoverReport) string {
	if report == nil {
		return "recipe discover\nsummary status=error targets=0"
	}
	var lines []string
	lines = append(lines, "recipe discover")
	if len(report.Discovery.Targets) > 0 {
		lines = append(lines, "targets:")
		for _, target := range report.Discovery.Targets {
			lines = append(lines, fmt.Sprintf("  %s state=%s platform=%s binary=%s config=%s", target.ID, target.State, target.PlatformState, target.BinaryState, target.ConfigState))
			if len(target.CommandProbes) > 0 {
				commands := make([]string, 0, len(target.CommandProbes))
				for _, probe := range target.CommandProbes {
					commands = append(commands, fmt.Sprintf("%s:%s", probe.Command, probe.State))
				}
				lines = append(lines, "    commands="+strings.Join(commands, ","))
			}
			if len(target.ConfigProbes) > 0 {
				configs := make([]string, 0, len(target.ConfigProbes))
				for _, probe := range target.ConfigProbes {
					configs = append(configs, fmt.Sprintf("%s:%s:%s", probe.LocationID, probe.Path, probe.State))
				}
				lines = append(lines, "    configs="+strings.Join(configs, ","))
			}
			for _, diagnostic := range target.Diagnostics {
				lines = append(lines, fmt.Sprintf("    diagnostic %s[%s]: %s", diagnostic.Severity, diagnostic.Code, diagnostic.Message))
			}
		}
	}
	if len(report.Discovery.Diagnostics) > 0 {
		lines = append(lines, "diagnostics:")
		for _, diagnostic := range report.Discovery.Diagnostics {
			lines = append(lines, fmt.Sprintf("  %s[%s]: %s", diagnostic.Severity, diagnostic.Code, diagnostic.Message))
		}
	}
	lines = append(lines, fmt.Sprintf("summary status=%s targets=%d", report.Summary.Status, len(report.Discovery.Targets)))
	return strings.Join(lines, "\n")
}

func friendlyDiscoverErrorText(report *DiscoverReport) string {
	lines := []string{"Discover supported app settings", "", "Command result:"}
	if report != nil && report.Error != nil {
		lines = append(lines, "  "+report.Error.Message)
	} else {
		lines = append(lines, "  The command could not complete.")
	}
	lines = append(lines, "", "No files changed.", "", "Run with --verbose for technical details.")
	return strings.Join(trimBlank(lines), "\n")
}

func friendlyDiscoverTargetName(target DiscoveredTarget) string {
	if strings.TrimSpace(target.DisplayName) != "" {
		return target.DisplayName
	}
	if strings.TrimSpace(target.ID) != "" {
		return titleWords(strings.ReplaceAll(target.ID, ".", " "))
	}
	return "App"
}

func friendlyDiscoverState(target DiscoveredTarget) string {
	switch target.State {
	case DiscoverStateConfigPresent:
		return "installed or configured; a known config file is present"
	case DiscoverStateInstalled:
		return "installed; no known config file found yet"
	case DiscoverStateConfigMissing:
		return "supported, but no known config file found"
	case DiscoverStateUnsupportedPlatform:
		return "not supported on this platform"
	case DiscoverStateAmbiguous:
		return "needs review because detection was ambiguous"
	case DiscoverStateNotApplicable:
		return "detection is not applicable"
	case "":
		return "not checked"
	default:
		return strings.ReplaceAll(target.State, "-", " ")
	}
}

func friendlyProbeState(state string) string {
	switch state {
	case "installed":
		return "installed"
	case "present", "config-present":
		return "present"
	case "missing", "config-missing":
		return "missing"
	case "ambiguous", "config-ambiguous":
		return "needs review"
	case "not-applicable":
		return "not applicable"
	case "":
		return "not checked"
	default:
		return strings.ReplaceAll(state, "-", " ")
	}
}

func friendlyProbeLocation(locationID string, path string) string {
	return friendlyResourceLocation(ExplainResource{LocationID: locationID, Path: path})
}

func friendlyFirstDiscoverSetting(targetID string) *ExplainSetting {
	explain, ok := bundledExplain(targetID)
	if !ok {
		return nil
	}
	return firstManageableSetting(explain.Settings)
}

func pluralWord(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

func baseDiscoverReport() *DiscoverReport {
	return &DiscoverReport{
		Schema:        ExplainSchema,
		SchemaVersion: SupportedVersion,
		Command:       DiscoverCommand,
		RunID:         DiscoverRunID,
		Summary:       ExplainSummary{Status: "ok"},
		Items:         []any{},
		Discovery:     DiscoveryResult{Diagnostics: []ExplainDiagnostic{}},
	}
}

func finishDiscoverReport(report *DiscoverReport) *DiscoverReport {
	if report == nil {
		return baseDiscoverReport()
	}
	blocked := 0
	for idx := range report.Discovery.Targets {
		target := &report.Discovery.Targets[idx]
		sort.Strings(target.Aliases)
		sort.Slice(target.CommandProbes, func(i, j int) bool { return target.CommandProbes[i].Command < target.CommandProbes[j].Command })
		sort.Slice(target.ConfigProbes, func(i, j int) bool {
			if target.ConfigProbes[i].LocationID != target.ConfigProbes[j].LocationID {
				return target.ConfigProbes[i].LocationID < target.ConfigProbes[j].LocationID
			}
			return target.ConfigProbes[i].Path < target.ConfigProbes[j].Path
		})
		sortDiagnostics(target.Diagnostics)
		if target.State == DiscoverStateAmbiguous || target.State == DiscoverStateUnsupportedPlatform {
			blocked++
		}
	}
	sortDiagnostics(report.Discovery.Diagnostics)
	report.Summary.Blocked = blocked
	if report.Summary.Status == "ok" && blocked > 0 {
		report.Summary.Status = "partial"
	}
	return report
}

func discoverTargets(ref string) ([]BundledTarget, error) {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" {
		return ListBundledTargets(), nil
	}
	target, ok := LookupBundledTarget(trimmed)
	if !ok {
		return nil, fmt.Errorf("unknown recipe discovery target: %s", trimmed)
	}
	return []BundledTarget{target}, nil
}

func discoverTarget(target BundledTarget, opts DiscoverOptions) DiscoveredTarget {
	out := DiscoveredTarget{
		ID:              target.ID,
		DisplayName:     target.DisplayName,
		Aliases:         append([]string(nil), target.Aliases...),
		Source:          target.Source,
		RecipeRef:       target.RecipeRef,
		TrustStatus:     target.TrustStatus,
		Version:         target.Version,
		SupportLevel:    target.SupportLevel,
		Capability:      target.Capability,
		PlatformSupport: target.PlatformSupport,
		Summary:         target.Summary,
		Diagnostics:     []ExplainDiagnostic{},
	}
	goos := strings.TrimSpace(opts.GOOS)
	if goos == "" {
		goos = currentGOOS()
	}
	out.PlatformState = discoverPlatformState(target.PlatformSupport, goos)
	if out.PlatformState == DiscoverPlatformUnsupported {
		out.State = DiscoverStateUnsupportedPlatform
		out.BinaryState = DiscoverBinaryNotApplicable
		out.ConfigState = DiscoverConfigNotApplicable
		out.Diagnostics = append(out.Diagnostics, ExplainDiagnostic{Code: DiscoverCodeUnsupportedPlatform, Severity: ExplainSeverityWarning, Message: fmt.Sprintf("target %s is not supported on platform %s", target.ID, goos), Ref: target.ID})
		return out
	}
	out.CommandProbes, out.BinaryState, out.Diagnostics = discoverCommandProbes(target, opts, out.Diagnostics)
	out.ConfigProbes, out.ConfigState, out.Diagnostics = discoverConfigProbes(target, opts, out.Diagnostics)
	out.State = summarizeDiscoveryState(out.PlatformState, out.BinaryState, out.ConfigState)
	return out
}

func discoverPlatformState(platformSupport string, goos string) string {
	trimmed := strings.TrimSpace(platformSupport)
	if trimmed == "" || trimmed == "unknown" {
		return DiscoverPlatformUnknown
	}
	for _, part := range strings.FieldsFunc(trimmed, func(r rune) bool { return r == '-' || r == ',' || r == ' ' }) {
		if part == goos {
			return DiscoverPlatformSupported
		}
	}
	return DiscoverPlatformUnsupported
}

func discoverCommandProbes(target BundledTarget, opts DiscoverOptions, diagnostics []ExplainDiagnostic) ([]DiscoveryCommandProbe, string, []ExplainDiagnostic) {
	commands := append([]string(nil), target.DiscoveryCommands...)
	sort.Strings(commands)
	if len(commands) == 0 {
		return []DiscoveryCommandProbe{}, DiscoverBinaryNotApplicable, diagnostics
	}
	lookup := opts.CommandLookup
	if lookup == nil {
		lookup = lookupCommandInPath
	}
	var probes []DiscoveryCommandProbe
	state := DiscoverBinaryMissing
	for _, command := range commands {
		_, err := lookup(command, opts.PathEnv)
		probe := DiscoveryCommandProbe{Kind: DiscoverProbeCommand, Command: command, State: DiscoverProbeMissing}
		switch {
		case err == nil:
			probe.State = DiscoverProbeInstalled
			state = DiscoverBinaryInstalled
		case errors.Is(err, exec.ErrNotFound):
			// Missing remains non-blocking.
		default:
			probe.State = DiscoverProbeAmbiguous
			state = DiscoverBinaryAmbiguous
			diagnostics = append(diagnostics, ExplainDiagnostic{Code: DiscoverCodeCommandLookupError, Severity: ExplainSeverityError, Message: fmt.Sprintf("command lookup for %s failed without a stable missing result", command), Ref: target.ID})
		}
		probes = append(probes, probe)
	}
	return probes, state, diagnostics
}

func discoverConfigProbes(target BundledTarget, opts DiscoverOptions, diagnostics []ExplainDiagnostic) ([]DiscoveryConfigProbe, string, []ExplainDiagnostic) {
	plans, planDiagnostics := configProbePlans(target, opts)
	diagnostics = append(diagnostics, planDiagnostics...)
	if len(plans) == 0 {
		if len(planDiagnostics) > 0 {
			return []DiscoveryConfigProbe{}, DiscoverConfigAmbiguous, diagnostics
		}
		return []DiscoveryConfigProbe{}, DiscoverConfigNotApplicable, diagnostics
	}
	lstat := opts.Lstat
	if lstat == nil {
		lstat = os.Lstat
	}
	state := DiscoverConfigMissing
	var probes []DiscoveryConfigProbe
	for _, plan := range plans {
		probe := DiscoveryConfigProbe{Kind: DiscoverProbeConfig, ID: plan.id, LocationID: plan.locationID, Path: plan.path, ExpectedType: plan.expectedType, State: DiscoverProbeMissing, Resources: append([]string(nil), plan.resources...)}
		info, err := lstat(plan.absPath)
		switch {
		case err == nil:
			actualType := fileInfoKind(info)
			probe.ActualType = actualType
			if actualType == "symlink" && plan.rejectSymlink {
				probe.State = DiscoverProbeAmbiguous
				state = DiscoverConfigAmbiguous
				diagnostics = append(diagnostics, ExplainDiagnostic{Code: DiscoverCodeConfigSymlinkBlocked, Severity: ExplainSeverityError, Message: fmt.Sprintf("config probe %s rejects symlink targets", plan.id), Ref: target.ID, Path: plan.path})
			} else if !actualTypeMatches(plan.expectedType, actualType) {
				probe.State = DiscoverProbeAmbiguous
				state = DiscoverConfigAmbiguous
				diagnostics = append(diagnostics, ExplainDiagnostic{Code: DiscoverCodeConfigTypeMismatch, Severity: ExplainSeverityError, Message: fmt.Sprintf("config probe %s expected %s but found %s", plan.id, plan.expectedType, actualType), Ref: target.ID, Path: plan.path})
			} else if state != DiscoverConfigAmbiguous {
				probe.State = DiscoverProbePresent
				state = DiscoverConfigPresent
			} else {
				probe.State = DiscoverProbePresent
			}
		case os.IsNotExist(err):
			// Missing remains non-blocking.
		default:
			probe.State = DiscoverProbeAmbiguous
			state = DiscoverConfigAmbiguous
			diagnostics = append(diagnostics, ExplainDiagnostic{Code: DiscoverCodeConfigStatError, Severity: ExplainSeverityError, Message: fmt.Sprintf("config probe %s could not be classified", plan.id), Ref: target.ID, Path: plan.path})
		}
		probes = append(probes, probe)
	}
	return probes, state, diagnostics
}

func configProbePlans(target BundledTarget, opts DiscoverOptions) ([]configProbePlan, []ExplainDiagnostic) {
	runtime, err := LoadRuntime("", target.ID)
	if err != nil || runtime.Recipe == nil {
		return nil, nil
	}
	rec := runtime.Recipe
	var diagnostics []ExplainDiagnostic
	plansByID := map[string]*configProbePlan{}
	resourceIDs := sortedKeys(rec.Resources)
	for _, resourceID := range resourceIDs {
		resource := rec.Resources[resourceID]
		expectedType := discoveryExpectedType(resource.Driver)
		if expectedType == "" {
			continue
		}
		resourceRelPath, err := ValidateResourcePath(resource.Path)
		if err != nil {
			diagnostics = append(diagnostics, ExplainDiagnostic{Code: DiscoverCodeResourcePathInvalid, Severity: ExplainSeverityError, Message: fmt.Sprintf("resource %s path is invalid for discovery", resourceID), Ref: target.ID, Path: resource.Path})
			continue
		}
		locationRoot, err := discoveryLocationRoot(rec, resource.Location, opts)
		if err != nil {
			diagnostics = append(diagnostics, ExplainDiagnostic{Code: DiscoverCodeLocationInvalid, Severity: ExplainSeverityError, Message: fmt.Sprintf("resource %s location %s could not be resolved for discovery", resourceID, resource.Location), Ref: target.ID, Path: resource.Path})
			continue
		}
		id := resource.Location + ":" + resourceRelPath + ":" + expectedType
		plan, ok := plansByID[id]
		if !ok {
			plan = &configProbePlan{id: id, locationID: resource.Location, path: resourceRelPath, expectedType: expectedType, absPath: filepath.Join(locationRoot, filepath.FromSlash(resourceRelPath)), rejectSymlink: discoveryRejectsLeafSymlink(resource)}
			plansByID[id] = plan
		}
		plan.resources = append(plan.resources, resourceID)
		if discoveryRejectsLeafSymlink(resource) {
			plan.rejectSymlink = true
		}
	}
	ids := make([]string, 0, len(plansByID))
	for id := range plansByID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	plans := make([]configProbePlan, 0, len(ids))
	for _, id := range ids {
		plan := *plansByID[id]
		sort.Strings(plan.resources)
		plans = append(plans, plan)
	}
	return plans, diagnostics
}

func discoveryLocationRoot(rec *Recipe, locationID string, opts DiscoverOptions) (string, error) {
	if override := strings.TrimSpace(opts.LocationRoots[locationID]); override != "" {
		return override, nil
	}
	location, ok := rec.Locations[locationID]
	if !ok {
		return "", fmt.Errorf("unknown location %s", locationID)
	}
	expand := opts.UserHomeExpand
	if expand == nil {
		expand = ExpandLocationDefault
	}
	return expand(location.Default)
}

func discoveryExpectedType(driver string) string {
	switch driver {
	case FileTreeDriverID:
		return "directory"
	case FileDriverID, IniFileDriverID, JSONFileDriverID, YAMLFileDriverID, TOMLFileDriverID, PlistFileDriverID:
		return "file"
	default:
		return ""
	}
}

func discoveryRejectsLeafSymlink(resource Resource) bool {
	return resource.ContentSafetyPolicy == SSHContentSafetyPolicy
}

func fileInfoKind(info os.FileInfo) string {
	if info.Mode()&os.ModeSymlink != 0 {
		return "symlink"
	}
	if info.IsDir() {
		return "directory"
	}
	if info.Mode().IsRegular() {
		return "file"
	}
	return "other"
}

func actualTypeMatches(expected string, actual string) bool {
	if actual == "symlink" {
		return true
	}
	return expected == actual
}

func summarizeDiscoveryState(platformState string, binaryState string, configState string) string {
	if platformState == DiscoverPlatformUnsupported {
		return DiscoverStateUnsupportedPlatform
	}
	if binaryState == DiscoverBinaryAmbiguous || configState == DiscoverConfigAmbiguous {
		return DiscoverStateAmbiguous
	}
	if configState == DiscoverConfigPresent {
		return DiscoverStateConfigPresent
	}
	if binaryState == DiscoverBinaryInstalled {
		return DiscoverStateInstalled
	}
	if configState == DiscoverConfigMissing {
		return DiscoverStateConfigMissing
	}
	return DiscoverStateNotApplicable
}

func lookupCommandInPath(command string, pathEnv string) (string, error) {
	if strings.TrimSpace(command) == "" || strings.ContainsRune(command, filepath.Separator) {
		return "", exec.ErrNotFound
	}
	if pathEnv == "" {
		pathEnv = os.Getenv("PATH")
	}
	for _, dir := range filepath.SplitList(pathEnv) {
		if strings.TrimSpace(dir) == "" {
			continue
		}
		candidate := filepath.Join(dir, command)
		info, err := os.Stat(candidate)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		if info.IsDir() || info.Mode().Perm()&0o111 == 0 {
			continue
		}
		return candidate, nil
	}
	return "", exec.ErrNotFound
}

func currentGOOS() string {
	return runtimeGOOS
}

var runtimeGOOS = runtime.GOOS
