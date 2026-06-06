package migration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/shpoont/dotfiles-manager/internal/v2/customfiles"
	"github.com/shpoont/dotfiles-manager/internal/v2/filedriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/filetreedriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/shpoont/dotfiles-manager/internal/v2/resolution"
	"gopkg.in/yaml.v3"
)

const (
	ParityReportSchema = "dotfiles-manager.v2.migration-parity-report"
)

type ParityOptions struct {
	RunDir        string
	GeneratedRoot string
	Plan          *Plan
	HomeDir       string
	UserID        string
}

type ParityReport struct {
	Schema          string        `json:"schema" yaml:"schema"`
	SchemaVersion   int           `json:"schemaVersion" yaml:"schemaVersion"`
	RunID           string        `json:"runId" yaml:"runId"`
	MigrationRunDir string        `json:"migrationRunDir" yaml:"migrationRunDir"`
	GeneratedRoot   string        `json:"generatedRoot" yaml:"generatedRoot"`
	ConfigPath      string        `json:"configPath" yaml:"configPath"`
	Items           []ParityItem  `json:"items" yaml:"items"`
	Summary         ParitySummary `json:"summary" yaml:"summary"`
	Error           *ErrorObject  `json:"error,omitempty" yaml:"error,omitempty"`
}

type ParityItem struct {
	SyncIndex              int                    `json:"syncIndex" yaml:"syncIndex"`
	SyncRef                string                 `json:"syncRef" yaml:"syncRef"`
	LegacySource           string                 `json:"legacySource" yaml:"legacySource"`
	LegacyTarget           string                 `json:"legacyTarget" yaml:"legacyTarget"`
	ExpandedSourcePath     string                 `json:"expandedSourcePath" yaml:"expandedSourcePath"`
	ExpandedTargetPath     string                 `json:"expandedTargetPath" yaml:"expandedTargetPath"`
	SettingRef             string                 `json:"settingRef" yaml:"settingRef"`
	SettingID              string                 `json:"settingId" yaml:"settingId"`
	Driver                 string                 `json:"driver" yaml:"driver"`
	LocationID             string                 `json:"locationId" yaml:"locationId"`
	LocationDefault        string                 `json:"locationDefault" yaml:"locationDefault"`
	ResourceID             string                 `json:"resourceId" yaml:"resourceId"`
	ResourcePath           string                 `json:"resourcePath" yaml:"resourcePath"`
	DesiredArtifactBinding DesiredArtifactBinding `json:"desiredArtifactBinding" yaml:"desiredArtifactBinding"`
	LiveTargetPath         string                 `json:"liveTargetPath,omitempty" yaml:"liveTargetPath,omitempty"`
	DesiredArtifactPath    string                 `json:"desiredArtifactPath,omitempty" yaml:"desiredArtifactPath,omitempty"`
	DesiredRelPath         string                 `json:"desiredRelPath,omitempty" yaml:"desiredRelPath,omitempty"`
	SourceSnapshot         *ParitySnapshot        `json:"sourceSnapshot,omitempty" yaml:"sourceSnapshot,omitempty"`
	DesiredSnapshot        *ParitySnapshot        `json:"desiredSnapshot,omitempty" yaml:"desiredSnapshot,omitempty"`
	Result                 string                 `json:"result" yaml:"result"`
	Diagnostics            []Diagnostic           `json:"diagnostics,omitempty" yaml:"diagnostics,omitempty"`
}

type ParitySnapshot struct {
	Exists     bool   `json:"exists" yaml:"exists"`
	Size       int    `json:"size,omitempty" yaml:"size,omitempty"`
	EntryCount int    `json:"entryCount,omitempty" yaml:"entryCount,omitempty"`
	FileCount  int    `json:"fileCount,omitempty" yaml:"fileCount,omitempty"`
	DirCount   int    `json:"dirCount,omitempty" yaml:"dirCount,omitempty"`
	SHA256     string `json:"sha256,omitempty" yaml:"sha256,omitempty"`
}

type ParitySummary struct {
	Syncs     int    `json:"syncs" yaml:"syncs"`
	OK        int    `json:"ok" yaml:"ok"`
	Blocked   int    `json:"blocked" yaml:"blocked"`
	Files     int    `json:"files" yaml:"files"`
	FileTrees int    `json:"fileTrees" yaml:"fileTrees"`
	Status    string `json:"status" yaml:"status"`
}

func BuildParityReport(opts ParityOptions) (*ParityReport, error) {
	plan, runDir, err := parityPlanAndRunDir(opts)
	if err != nil {
		return nil, err
	}
	generatedRoot, err := parityGeneratedRoot(opts, runDir)
	if err != nil {
		return nil, err
	}
	homeDir, err := parityHomeDir(opts.HomeDir)
	if err != nil {
		return nil, err
	}
	userID := strings.TrimSpace(opts.UserID)
	if userID == "" {
		userID = "legacy"
	}

	report := &ParityReport{
		Schema:          ParityReportSchema,
		SchemaVersion:   SchemaVersion,
		RunID:           plan.RunID,
		MigrationRunDir: runDir,
		GeneratedRoot:   generatedRoot,
		ConfigPath:      plan.ConfigPath,
	}

	profile, rec, globalDiagnostic := loadGeneratedParityInputs(generatedRoot, userID)
	items := append([]Item(nil), plan.Items...)
	sort.SliceStable(items, func(i, j int) bool { return items[i].SyncIndex < items[j].SyncIndex })
	for _, item := range items {
		report.Items = append(report.Items, buildParityItem(plan, item, generatedRoot, homeDir, profile, rec, globalDiagnostic))
	}
	report.Summary = summarizeParity(report.Items)
	if report.Summary.Blocked > 0 {
		report.Error = &ErrorObject{Code: "parity-blocked", Message: fmt.Sprintf("parity report has %d blocked item(s)", report.Summary.Blocked)}
	}
	return report, nil
}

func ParityJSON(report *ParityReport) (string, error) {
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	return string(payload) + "\n", nil
}

func ParityYAML(report *ParityReport) (string, error) {
	payload, err := yaml.Marshal(report)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func parityPlanAndRunDir(opts ParityOptions) (*Plan, string, error) {
	if opts.Plan != nil {
		if err := validateParityPlan(opts.Plan); err != nil {
			return nil, "", err
		}
		runDir := strings.TrimSpace(opts.RunDir)
		if runDir == "" {
			runDir = opts.Plan.OutputDir
		}
		if runDir != "" {
			absRunDir, err := filepath.Abs(runDir)
			if err != nil {
				return nil, "", fmt.Errorf("resolve migration run dir %q: %w", runDir, err)
			}
			runDir = absRunDir
		}
		return opts.Plan, runDir, nil
	}

	runDir := strings.TrimSpace(opts.RunDir)
	if runDir == "" {
		return nil, "", fmt.Errorf("migration run dir is required")
	}
	absRunDir, err := filepath.Abs(runDir)
	if err != nil {
		return nil, "", fmt.Errorf("resolve migration run dir %q: %w", runDir, err)
	}
	body, err := os.ReadFile(filepath.Join(absRunDir, "migration-plan.yaml"))
	if err != nil {
		return nil, "", fmt.Errorf("read migration plan: %w", err)
	}
	var plan Plan
	if err := yaml.Unmarshal(body, &plan); err != nil {
		return nil, "", fmt.Errorf("decode migration plan: %w", err)
	}
	if plan.Schema != MigrationPlanSchema {
		return nil, "", fmt.Errorf("migration plan schema must be %s, got %s", MigrationPlanSchema, plan.Schema)
	}
	if err := validateParityPlan(&plan); err != nil {
		return nil, "", err
	}
	return &plan, absRunDir, nil
}

func validateParityPlan(plan *Plan) error {
	if plan == nil {
		return fmt.Errorf("migration plan is required")
	}
	if plan.Schema != MigrationPlanSchema {
		return fmt.Errorf("migration plan schema must be %s, got %s", MigrationPlanSchema, plan.Schema)
	}
	if plan.SchemaVersion != SchemaVersion {
		return fmt.Errorf("migration plan schemaVersion must be %d, got %d", SchemaVersion, plan.SchemaVersion)
	}
	if plan.Command != Command {
		return fmt.Errorf("migration plan command must be %s, got %s", Command, plan.Command)
	}
	if plan.DryRun {
		return fmt.Errorf("migration parity requires a non-dry-run migration plan")
	}
	return nil
}

func parityGeneratedRoot(opts ParityOptions, runDir string) (string, error) {
	generatedRoot := strings.TrimSpace(opts.GeneratedRoot)
	if generatedRoot == "" {
		if runDir == "" {
			return "", fmt.Errorf("generated root is required when migration run dir is unavailable")
		}
		generatedRoot = filepath.Join(runDir, "generated")
	}
	absGeneratedRoot, err := filepath.Abs(generatedRoot)
	if err != nil {
		return "", fmt.Errorf("resolve generated root %q: %w", generatedRoot, err)
	}
	return absGeneratedRoot, nil
}

func parityHomeDir(homeDir string) (string, error) {
	trimmed := strings.TrimSpace(homeDir)
	if trimmed == "" {
		resolved, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		trimmed = resolved
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", fmt.Errorf("resolve home directory %q: %w", homeDir, err)
	}
	return abs, nil
}

func loadGeneratedParityInputs(generatedRoot string, userID string) (*resolution.ResolvedProfile, *recipe.Recipe, []Diagnostic) {
	profile, err := resolution.Resolve(generatedRoot, resolution.ResolveOptions{UserID: userID})
	if err != nil {
		return nil, nil, []Diagnostic{parityDiagnostic("parity-generated-root-unavailable", fmt.Sprintf("cannot resolve generated v2 root: %v", err), generatedRoot)}
	}
	rec, err := recipe.LoadCustomFiles(generatedRoot)
	if err != nil {
		return nil, nil, []Diagnostic{parityDiagnostic("parity-generated-root-unavailable", fmt.Sprintf("cannot load generated custom.files recipe: %v", err), generatedRoot)}
	}
	return profile, rec, nil
}

func buildParityItem(plan *Plan, item Item, generatedRoot string, homeDir string, profile *resolution.ResolvedProfile, rec *recipe.Recipe, globalDiagnostics []Diagnostic) ParityItem {
	parityItem := baseParityItem(item)
	if item.Result == "blocked" {
		parityItem.Result = "blocked"
		parityItem.Diagnostics = append(parityItem.Diagnostics, item.Diagnostics...)
		parityItem.Diagnostics = append(parityItem.Diagnostics, parityDiagnostic("parity-migration-item-blocked", "migration item is blocked; parity cannot be proven", item.ExpandedSourcePath))
		return parityItem
	}
	if len(globalDiagnostics) > 0 {
		parityItem.Result = "blocked"
		parityItem.Diagnostics = append(parityItem.Diagnostics, globalDiagnostics...)
		return parityItem
	}
	if diagnostics := generatedParityMappingDiagnostics(item, rec); len(diagnostics) > 0 {
		parityItem.Result = "blocked"
		parityItem.Diagnostics = append(parityItem.Diagnostics, diagnostics...)
		return parityItem
	}

	locationRoot, err := expandParityLocationDefault(item.LocationDefault, homeDir)
	if err != nil {
		parityItem.Result = "blocked"
		parityItem.Diagnostics = append(parityItem.Diagnostics, parityDiagnostic("parity-location-unavailable", err.Error(), item.LocationDefault))
		return parityItem
	}
	req := customfiles.Request{
		Profile:       profile,
		Recipe:        rec,
		SettingRef:    item.SettingRef,
		LocationRoots: map[string]string{item.LocationID: locationRoot},
	}
	applyPlan, err := customfiles.PlanApply(req)
	if err != nil {
		parityItem.Result = "blocked"
		parityItem.Diagnostics = append(parityItem.Diagnostics, parityDiagnostic("parity-apply-plan-unavailable", fmt.Sprintf("cannot build generated custom.files apply plan: %v", err), item.SettingRef))
		return parityItem
	}

	switch item.Driver {
	case recipe.FileDriverID:
		populateFileParity(plan, item, applyPlan, &parityItem)
	case recipe.FileTreeDriverID:
		populateFileTreeParity(plan, item, applyPlan, &parityItem)
	default:
		parityItem.Diagnostics = append(parityItem.Diagnostics, parityDiagnostic("parity-driver-unknown", "cannot prove parity for unknown driver", item.Driver))
	}
	if len(parityItem.Diagnostics) > 0 {
		parityItem.Result = "blocked"
	} else {
		parityItem.Result = "ok"
	}
	return parityItem
}

func generatedParityMappingDiagnostics(item Item, rec *recipe.Recipe) []Diagnostic {
	if rec == nil {
		return []Diagnostic{parityDiagnostic("parity-generated-root-unavailable", "generated custom.files recipe is unavailable", item.SettingRef)}
	}
	var diagnostics []Diagnostic
	setting, ok := rec.Settings[item.SettingID]
	if !ok {
		return []Diagnostic{parityDiagnostic("parity-generated-setting-missing", "generated custom.files setting is missing", item.SettingID)}
	}
	if setting.Resource != item.ResourceID {
		diagnostics = append(diagnostics, parityDiagnostic("parity-generated-setting-drift", "generated custom.files setting resource does not match migration item", item.SettingID))
	}

	resource, ok := rec.Resources[item.ResourceID]
	if !ok {
		return append(diagnostics, parityDiagnostic("parity-generated-resource-missing", "generated custom.files resource is missing", item.ResourceID))
	}
	if resource.Driver != item.Driver {
		diagnostics = append(diagnostics, parityDiagnostic("parity-generated-resource-drift", "generated custom.files resource driver does not match migration item", item.ResourceID))
	}
	if resource.Location != item.LocationID {
		diagnostics = append(diagnostics, parityDiagnostic("parity-generated-resource-drift", "generated custom.files resource location does not match migration item", item.ResourceID))
	}
	if resource.Path != item.ResourcePath {
		diagnostics = append(diagnostics, parityDiagnostic("parity-generated-resource-drift", "generated custom.files resource path does not match migration item", item.ResourceID))
	}
	if item.Driver == recipe.FileTreeDriverID && (len(resource.Include) > 0 || len(resource.Exclude) > 0) {
		diagnostics = append(diagnostics, parityDiagnostic("parity-generated-globs-drift", "generated file-tree include/exclude globs would change legacy v1 tree semantics", item.ResourceID))
	}

	location, ok := rec.Locations[item.LocationID]
	if !ok {
		return append(diagnostics, parityDiagnostic("parity-generated-location-missing", "generated custom.files location is missing", item.LocationID))
	}
	if location.Default != item.LocationDefault {
		diagnostics = append(diagnostics, parityDiagnostic("parity-generated-location-drift", "generated custom.files location default does not match migration item", item.LocationID))
	}
	return diagnostics
}

func baseParityItem(item Item) ParityItem {
	return ParityItem{
		SyncIndex:              item.SyncIndex,
		SyncRef:                item.SyncRef,
		LegacySource:           item.LegacySource,
		LegacyTarget:           item.LegacyTarget,
		ExpandedSourcePath:     item.ExpandedSourcePath,
		ExpandedTargetPath:     item.ExpandedTargetPath,
		SettingRef:             item.SettingRef,
		SettingID:              item.SettingID,
		Driver:                 item.Driver,
		LocationID:             item.LocationID,
		LocationDefault:        item.LocationDefault,
		ResourceID:             item.ResourceID,
		ResourcePath:           item.ResourcePath,
		DesiredArtifactBinding: item.DesiredArtifactBinding,
		Result:                 "blocked",
	}
}

func populateFileParity(plan *Plan, item Item, applyPlan *customfiles.Plan, parityItem *ParityItem) {
	parityItem.LiveTargetPath = applyPlan.Preview.Path
	parityItem.DesiredArtifactPath = applyPlan.DesiredTarget.Root
	if applyPlan.DesiredTarget.RelPath != "" {
		parityItem.DesiredArtifactPath = filepath.Join(applyPlan.DesiredTarget.Root, filepath.FromSlash(applyPlan.DesiredTarget.RelPath))
	}
	parityItem.DesiredRelPath = applyPlan.DesiredRelPath
	if !sameCleanPath(applyPlan.Preview.Path, item.ExpandedTargetPath) {
		parityItem.Diagnostics = append(parityItem.Diagnostics, parityDiagnostic("parity-live-target-mismatch", "generated v2 live target does not match legacy target", applyPlan.Preview.Path))
	}

	sourceState, err := filedriver.Driver{}.ReadCurrent(filedriver.Target{LocationID: "legacy-source", Root: plan.RepoRoot, RelPath: item.ExpandedSourceRelPath, RejectRootSymlink: true})
	if err != nil {
		parityItem.Diagnostics = append(parityItem.Diagnostics, parityDiagnostic("parity-legacy-source-unavailable", fmt.Sprintf("cannot read legacy source file: %v", err), item.ExpandedSourcePath))
		return
	}
	desiredState := applyPlan.DesiredFinalState
	parityItem.SourceSnapshot = fileParitySnapshot(sourceState)
	parityItem.DesiredSnapshot = fileParitySnapshot(desiredState)
	if !sourceState.Exists {
		parityItem.Diagnostics = append(parityItem.Diagnostics, parityDiagnostic("parity-legacy-source-unavailable", "legacy source file is missing", item.ExpandedSourcePath))
	}
	if !desiredState.Exists {
		parityItem.Diagnostics = append(parityItem.Diagnostics, parityDiagnostic("parity-generated-artifact-missing", "generated desired file artifact is missing", parityItem.DesiredArtifactPath))
	}
	if sourceState.Exists && desiredState.Exists && (sourceState.SHA256 != desiredState.SHA256 || !bytes.Equal(sourceState.Bytes, desiredState.Bytes)) {
		parityItem.Diagnostics = append(parityItem.Diagnostics, parityDiagnostic("parity-content-mismatch", "generated desired file content does not match legacy source content", parityItem.DesiredArtifactPath))
	}
}

func populateFileTreeParity(plan *Plan, item Item, applyPlan *customfiles.Plan, parityItem *ParityItem) {
	parityItem.LiveTargetPath = applyPlan.TreePreview.Path
	parityItem.DesiredArtifactPath = applyPlan.TreeDesiredTarget.Root
	if applyPlan.TreeDesiredTarget.RelPath != "" {
		parityItem.DesiredArtifactPath = filepath.Join(applyPlan.TreeDesiredTarget.Root, filepath.FromSlash(applyPlan.TreeDesiredTarget.RelPath))
	}
	parityItem.DesiredRelPath = applyPlan.DesiredRelPath
	if !sameCleanPath(applyPlan.TreePreview.Path, item.ExpandedTargetPath) {
		parityItem.Diagnostics = append(parityItem.Diagnostics, parityDiagnostic("parity-live-target-mismatch", "generated v2 live target does not match legacy target", applyPlan.TreePreview.Path))
	}

	sourceState, err := filetreedriver.Driver{}.ReadCurrent(filetreedriver.Target{LocationID: "legacy-source", Root: plan.RepoRoot, RelPath: item.ExpandedSourceRelPath, RejectRootSymlink: true})
	if err != nil {
		parityItem.Diagnostics = append(parityItem.Diagnostics, parityDiagnostic("parity-legacy-source-unavailable", fmt.Sprintf("cannot read legacy source tree: %v", err), item.ExpandedSourcePath))
		return
	}
	desiredState := applyPlan.TreeDesiredFinalState
	parityItem.SourceSnapshot = treeParitySnapshot(sourceState)
	parityItem.DesiredSnapshot = treeParitySnapshot(desiredState)
	if !sourceState.Exists {
		parityItem.Diagnostics = append(parityItem.Diagnostics, parityDiagnostic("parity-legacy-source-unavailable", "legacy source tree is missing", item.ExpandedSourcePath))
	}
	if !desiredState.Exists {
		parityItem.Diagnostics = append(parityItem.Diagnostics, parityDiagnostic("parity-generated-artifact-missing", "generated desired file-tree artifact is missing", parityItem.DesiredArtifactPath))
	}
	if sourceState.Exists && desiredState.Exists && sourceState.SHA256 != desiredState.SHA256 {
		parityItem.Diagnostics = append(parityItem.Diagnostics, parityDiagnostic("parity-content-mismatch", "generated desired file-tree content does not match legacy source content", parityItem.DesiredArtifactPath))
	}
}

func fileParitySnapshot(state filedriver.State) *ParitySnapshot {
	snapshot := state.Snapshot()
	return &ParitySnapshot{Exists: snapshot.Exists, Size: snapshot.Size, SHA256: snapshot.SHA256}
}

func treeParitySnapshot(state filetreedriver.State) *ParitySnapshot {
	snapshot := state.Snapshot()
	return &ParitySnapshot{
		Exists:     snapshot.Exists,
		EntryCount: snapshot.EntryCount,
		FileCount:  snapshot.FileCount,
		DirCount:   snapshot.DirCount,
		SHA256:     snapshot.SHA256,
	}
}

func summarizeParity(items []ParityItem) ParitySummary {
	summary := ParitySummary{Syncs: len(items)}
	for _, item := range items {
		switch item.Result {
		case "ok":
			summary.OK++
		default:
			summary.Blocked++
		}
		switch item.Driver {
		case recipe.FileDriverID:
			summary.Files++
		case recipe.FileTreeDriverID:
			summary.FileTrees++
		}
	}
	if summary.Blocked == 0 {
		summary.Status = "ok"
	} else {
		summary.Status = "blocked"
	}
	return summary
}

func expandParityLocationDefault(defaultValue string, homeDir string) (string, error) {
	trimmed := strings.TrimSpace(defaultValue)
	if trimmed == "" {
		return "", fmt.Errorf("location default is required")
	}
	if strings.ContainsRune(trimmed, '\x00') {
		return "", fmt.Errorf("location default contains NUL")
	}
	if trimmed == "~" {
		return homeDir, nil
	}
	if strings.HasPrefix(trimmed, "~/") {
		return filepath.Join(homeDir, filepath.FromSlash(strings.TrimPrefix(trimmed, "~/"))), nil
	}
	return trimmed, nil
}

func sameCleanPath(left string, right string) bool {
	return filepath.Clean(left) == filepath.Clean(right)
}

func parityDiagnostic(code string, message string, diagnosticPath string) Diagnostic {
	return Diagnostic{Code: code, Severity: "error", Message: message, Path: diagnosticPath}
}
