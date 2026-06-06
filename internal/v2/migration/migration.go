// Package migration builds v1-to-v2 migration previews and generated output.
package migration

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/shpoont/dotfiles-manager/internal/config"
	"github.com/shpoont/dotfiles-manager/internal/v2/filedriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/filetreedriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"gopkg.in/yaml.v3"
)

const (
	PreviewSchema       = "dotfiles-manager.v2.migration-preview"
	MigrationPlanSchema = "dotfiles-manager.v2.migration-plan"
	Schema              = PreviewSchema
	SchemaVersion       = 1
	Command             = "migrate"

	DefaultRunID = "dry-run"
)

var runIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

type Options struct {
	ConfigPath string
	RunID      string
	HomeDir    string
	Stat       func(string) (os.FileInfo, error)
	Now        func() time.Time
}

type Plan struct {
	Schema         string          `json:"schema" yaml:"schema"`
	SchemaVersion  int             `json:"schemaVersion" yaml:"schemaVersion"`
	Command        string          `json:"command" yaml:"command"`
	RunID          string          `json:"runId" yaml:"runId"`
	DryRun         bool            `json:"dryRun" yaml:"dryRun"`
	ConfigPath     string          `json:"configPath" yaml:"configPath"`
	RepoRoot       string          `json:"repoRoot" yaml:"repoRoot"`
	OutputDir      string          `json:"outputDir,omitempty" yaml:"outputDir,omitempty"`
	Items          []Item          `json:"items" yaml:"items"`
	GeneratedFiles []GeneratedFile `json:"generatedFiles" yaml:"generatedFiles"`
	Summary        Summary         `json:"summary" yaml:"summary"`
	Error          any             `json:"error" yaml:"error"`
}

type Item struct {
	SyncIndex              int                    `json:"syncIndex" yaml:"syncIndex"`
	SyncRef                string                 `json:"syncRef" yaml:"syncRef"`
	LegacySource           string                 `json:"legacySource" yaml:"legacySource"`
	LegacyTarget           string                 `json:"legacyTarget" yaml:"legacyTarget"`
	ExpandedSourceRelPath  string                 `json:"expandedSourceRelPath" yaml:"expandedSourceRelPath"`
	ExpandedTargetRelPath  string                 `json:"expandedTargetRelPath" yaml:"expandedTargetRelPath"`
	ExpandedSourcePath     string                 `json:"expandedSourcePath" yaml:"expandedSourcePath"`
	ExpandedTargetPath     string                 `json:"expandedTargetPath" yaml:"expandedTargetPath"`
	TargetRef              string                 `json:"targetRef" yaml:"targetRef"`
	SettingRef             string                 `json:"settingRef" yaml:"settingRef"`
	SettingID              string                 `json:"settingId" yaml:"settingId"`
	Driver                 string                 `json:"driver" yaml:"driver"`
	LocationID             string                 `json:"locationId" yaml:"locationId"`
	LocationDefault        string                 `json:"locationDefault" yaml:"locationDefault"`
	ResourceID             string                 `json:"resourceId" yaml:"resourceId"`
	ResourcePath           string                 `json:"resourcePath" yaml:"resourcePath"`
	DesiredArtifactBinding DesiredArtifactBinding `json:"desiredArtifactBinding" yaml:"desiredArtifactBinding"`
	GeneratedFiles         []GeneratedFile        `json:"generatedFiles" yaml:"generatedFiles"`
	Result                 string                 `json:"result" yaml:"result"`
	Diagnostics            []Diagnostic           `json:"diagnostics,omitempty" yaml:"diagnostics,omitempty"`
	V1ConfigAction         string                 `json:"v1ConfigAction" yaml:"v1ConfigAction"`
	BehaviorUnchanged      bool                   `json:"behaviorUnchanged" yaml:"behaviorUnchanged"`
}

type DesiredArtifactBinding struct {
	Artifact string `json:"artifact" yaml:"artifact"`
	URI      string `json:"uri" yaml:"uri"`
	RelPath  string `json:"relPath" yaml:"relPath"`
}

type GeneratedFile struct {
	Path    string `json:"path" yaml:"path"`
	Kind    string `json:"kind" yaml:"kind"`
	Purpose string `json:"purpose" yaml:"purpose"`
	SyncRef string `json:"syncRef,omitempty" yaml:"syncRef,omitempty"`
}

type Diagnostic struct {
	Code     string `json:"code" yaml:"code"`
	Severity string `json:"severity" yaml:"severity"`
	Message  string `json:"message" yaml:"message"`
	Path     string `json:"path,omitempty" yaml:"path,omitempty"`
}

type Summary struct {
	Syncs          int    `json:"syncs" yaml:"syncs"`
	Planned        int    `json:"planned" yaml:"planned"`
	Blocked        int    `json:"blocked" yaml:"blocked"`
	Files          int    `json:"files" yaml:"files"`
	FileTrees      int    `json:"fileTrees" yaml:"fileTrees"`
	GeneratedFiles int    `json:"generatedFiles" yaml:"generatedFiles"`
	Status         string `json:"status" yaml:"status"`
}

type ErrorPayload struct {
	Schema        string      `json:"schema"`
	SchemaVersion int         `json:"schemaVersion"`
	Command       string      `json:"command"`
	DryRun        bool        `json:"dryRun"`
	ConfigPath    any         `json:"configPath"`
	Summary       Summary     `json:"summary"`
	Error         ErrorObject `json:"error"`
}

type ErrorObject struct {
	Code    string         `json:"code" yaml:"code"`
	Message string         `json:"message" yaml:"message"`
	Details map[string]any `json:"details,omitempty" yaml:"details,omitempty"`
}

type Output struct {
	Plan     *Plan
	Payloads []Payload
}

type Payload struct {
	SyncRef   string
	Driver    string
	RelPath   string
	FileState filedriver.State
	TreeState filetreedriver.State
}

type BlockedError struct {
	Plan *Plan
}

func (e *BlockedError) Error() string {
	if e == nil || e.Plan == nil {
		return "migration blocked"
	}
	return fmt.Sprintf("migration blocked: %d blocked item(s)", e.Plan.Summary.Blocked)
}

func IsBlocked(err error) bool {
	var blocked *BlockedError
	return errors.As(err, &blocked)
}

func BuildDryRunPlan(opts Options) (*Plan, error) {
	cfg, absConfigPath, err := loadV1Config(opts.ConfigPath)
	if err != nil {
		return nil, err
	}
	return BuildDryRunPlanFromConfig(cfg, absConfigPath, opts)
}

func BuildDryRunPlanFromConfig(cfg *config.Config, configPath string, opts Options) (*Plan, error) {
	return buildPlanFromConfig(cfg, configPath, opts, true)
}

func BuildMigrationOutput(opts Options) (*Output, error) {
	cfg, absConfigPath, err := loadV1Config(opts.ConfigPath)
	if err != nil {
		return nil, err
	}
	plan, err := buildPlanFromConfig(cfg, absConfigPath, opts, false)
	if err != nil {
		return nil, err
	}
	output, err := capturePayloads(plan)
	if err != nil {
		return nil, err
	}
	if plan.Summary.Blocked > 0 {
		plan.Error = ErrorObject{Code: "migration-blocked", Message: fmt.Sprintf("migration has %d blocked item(s); no files were written", plan.Summary.Blocked)}
		return output, &BlockedError{Plan: plan}
	}
	return output, nil
}

func WriteMigrationOutput(opts Options) (*Plan, error) {
	output, err := BuildMigrationOutput(opts)
	if err != nil && !IsBlocked(err) {
		return nil, err
	}
	if output == nil || output.Plan == nil {
		return nil, fmt.Errorf("migration output is required")
	}
	if err != nil {
		return output.Plan, err
	}
	if err := output.Write(); err != nil {
		return output.Plan, err
	}
	return output.Plan, nil
}

func (o *Output) Write() error {
	if o == nil || o.Plan == nil {
		return fmt.Errorf("migration output is required")
	}
	plan := o.Plan
	if plan.DryRun {
		return fmt.Errorf("dry-run migration output must not be written")
	}
	if plan.Summary.Blocked > 0 {
		return &BlockedError{Plan: plan}
	}
	if err := validateRunID(plan.RunID); err != nil {
		return err
	}
	repoRoot := plan.RepoRoot
	if repoRoot == "" {
		repoRoot = filepath.Dir(plan.ConfigPath)
	}
	finalRel := filepath.FromSlash(path.Join("migrations", "v1-to-v2", plan.RunID))
	if err := ensureNoSymlinkInExistingPath(repoRoot, finalRel); err != nil {
		return err
	}
	finalDir := filepath.Join(repoRoot, finalRel)
	if _, err := os.Lstat(finalDir); err == nil {
		return fmt.Errorf("migration output path already exists: %s", finalDir)
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inspect migration output path %s: %w", finalDir, err)
	}
	parent := filepath.Dir(finalDir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create migration output parent %s: %w", parent, err)
	}
	staging, err := os.MkdirTemp(parent, "."+plan.RunID+"-tmp-")
	if err != nil {
		return fmt.Errorf("create migration staging directory: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(staging)
		}
	}()

	if err := o.writeStaged(staging); err != nil {
		return err
	}
	if err := os.Rename(staging, finalDir); err != nil {
		return fmt.Errorf("commit migration output %s: %w", finalDir, err)
	}
	committed = true
	return nil
}

func (o *Output) writeStaged(staging string) error {
	generatedRoot := filepath.Join(staging, "generated")
	if err := writeTextFile(filepath.Join(generatedRoot, "dotfiles-manager.v2.yaml"), generatedRootConfigYAML()); err != nil {
		return err
	}
	if err := writeTextFile(filepath.Join(generatedRoot, "profiles", "stacks", "legacy.yaml"), generatedStackYAML()); err != nil {
		return err
	}
	if err := writeTextFile(filepath.Join(generatedRoot, "profiles", "layers", "legacy.yaml"), generatedLayerYAML(o.Plan.Items)); err != nil {
		return err
	}
	if err := writeTextFile(filepath.Join(generatedRoot, "recipes", "local", recipe.CustomFilesTarget, "recipe.yaml"), generatedRecipeYAML(o.Plan.Items)); err != nil {
		return err
	}
	for _, payload := range o.Payloads {
		artifactRoot := filepath.Join(generatedRoot, "desired", "user", "legacy", "targets", recipe.CustomFilesTarget, "artifacts")
		switch payload.Driver {
		case recipe.FileDriverID:
			_, err := filedriver.Driver{}.Apply(filedriver.Target{LocationID: "desired", Root: artifactRoot, RelPath: payload.RelPath, AllowMissingRoot: true, RejectRootSymlink: true}, payload.FileState)
			if err != nil {
				return fmt.Errorf("write desired artifact %s: %w", payload.SyncRef, err)
			}
		case recipe.FileTreeDriverID:
			_, err := filetreedriver.Driver{}.Apply(filetreedriver.Target{LocationID: "desired", Root: artifactRoot, RelPath: payload.RelPath, AllowMissingRoot: true, RejectRootSymlink: true}, payload.TreeState)
			if err != nil {
				return fmt.Errorf("write desired artifact %s: %w", payload.SyncRef, err)
			}
		default:
			return fmt.Errorf("unsupported payload driver for %s: %s", payload.SyncRef, payload.Driver)
		}
	}
	planYAML, err := YAML(o.Plan)
	if err != nil {
		return err
	}
	return writeTextFile(filepath.Join(staging, "migration-plan.yaml"), planYAML)
}

func loadV1Config(configPath string) (*config.Config, string, error) {
	trimmed := strings.TrimSpace(configPath)
	if trimmed == "" {
		return nil, "", fmt.Errorf("config path is required")
	}
	absConfigPath, err := filepath.Abs(trimmed)
	if err != nil {
		return nil, "", fmt.Errorf("resolve config path %q: %w", configPath, err)
	}
	cfg, err := config.Load(absConfigPath)
	if err != nil {
		return nil, "", err
	}
	return cfg, absConfigPath, nil
}

func buildPlanFromConfig(cfg *config.Config, configPath string, opts Options, dryRun bool) (*Plan, error) {
	if cfg == nil {
		return nil, fmt.Errorf("v1 config is required")
	}
	absConfigPath, err := filepath.Abs(strings.TrimSpace(configPath))
	if err != nil {
		return nil, fmt.Errorf("resolve config path %q: %w", configPath, err)
	}
	homeDir := strings.TrimSpace(opts.HomeDir)
	if homeDir == "" {
		homeDir, err = os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve home directory: %w", err)
		}
	}
	homeDir, err = filepath.Abs(homeDir)
	if err != nil {
		return nil, fmt.Errorf("resolve home directory %q: %w", opts.HomeDir, err)
	}
	stat := opts.Stat
	if stat == nil {
		stat = os.Stat
	}
	runID := strings.TrimSpace(opts.RunID)
	if runID == "" {
		if dryRun {
			runID = DefaultRunID
		} else {
			runID = NewRunID(now(opts))
		}
	}
	if !dryRun {
		if err := validateRunID(runID); err != nil {
			return nil, err
		}
	}

	repoRoot := filepath.Dir(absConfigPath)
	plan := &Plan{
		Schema:        schemaForMode(dryRun),
		SchemaVersion: SchemaVersion,
		Command:       Command,
		RunID:         runID,
		DryRun:        dryRun,
		ConfigPath:    absConfigPath,
		RepoRoot:      repoRoot,
		OutputDir:     filepath.Join(repoRoot, "migrations", "v1-to-v2", runID),
		Error:         nil,
	}
	plan.GeneratedFiles = baseGeneratedFiles(runID)

	for idx, syncCfg := range cfg.Syncs {
		item, err := buildItem(runID, idx, syncCfg, repoRoot, homeDir, stat)
		if err != nil {
			return nil, err
		}
		plan.Items = append(plan.Items, item)
	}
	plan.Summary = summarize(plan.Items, plan.GeneratedFiles)
	return plan, nil
}

func capturePayloads(plan *Plan) (*Output, error) {
	output := &Output{Plan: plan}
	if plan == nil {
		return output, fmt.Errorf("migration plan is required")
	}
	for idx := range plan.Items {
		item := &plan.Items[idx]
		if item.Result == "blocked" {
			continue
		}
		payload, diagnostics := captureItemPayload(plan.RepoRoot, *item)
		if len(diagnostics) > 0 {
			item.Result = "blocked"
			item.GeneratedFiles = nil
			item.Diagnostics = append(item.Diagnostics, diagnostics...)
			continue
		}
		output.Payloads = append(output.Payloads, payload)
	}
	plan.Summary = summarize(plan.Items, plan.GeneratedFiles)
	return output, nil
}

func captureItemPayload(repoRoot string, item Item) (Payload, []Diagnostic) {
	sourceRel := item.ExpandedSourceRelPath
	if sourceRel == "" {
		return Payload{}, sourceDiagnostic(item, "migration-source-unavailable", "legacy source path is not available for desired artifact capture", item.ExpandedSourcePath)
	}
	switch item.Driver {
	case recipe.FileDriverID:
		state, err := filedriver.Driver{}.ReadCurrent(filedriver.Target{LocationID: "legacy-source", Root: repoRoot, RelPath: sourceRel, RejectRootSymlink: true})
		if err != nil {
			return Payload{}, sourceDiagnostic(item, "migration-source-unavailable", fmt.Sprintf("cannot read legacy source file: %v", err), item.ExpandedSourcePath)
		}
		if !state.Exists {
			return Payload{}, sourceDiagnostic(item, "migration-source-unavailable", "legacy source file is missing", item.ExpandedSourcePath)
		}
		return Payload{SyncRef: item.SyncRef, Driver: item.Driver, RelPath: item.SettingID, FileState: state}, nil
	case recipe.FileTreeDriverID:
		state, err := filetreedriver.Driver{}.ReadCurrent(filetreedriver.Target{LocationID: "legacy-source", Root: repoRoot, RelPath: sourceRel, RejectRootSymlink: true})
		if err != nil {
			return Payload{}, sourceDiagnostic(item, "migration-source-unavailable", fmt.Sprintf("cannot read legacy source tree: %v", err), item.ExpandedSourcePath)
		}
		if !state.Exists {
			return Payload{}, sourceDiagnostic(item, "migration-source-unavailable", "legacy source tree is missing", item.ExpandedSourcePath)
		}
		return Payload{SyncRef: item.SyncRef, Driver: item.Driver, RelPath: item.SettingID, TreeState: state}, nil
	default:
		return Payload{}, sourceDiagnostic(item, "migration-driver-unknown", "cannot capture desired artifact for unknown driver", item.ExpandedSourcePath)
	}
}

func sourceDiagnostic(item Item, code string, message string, path string) []Diagnostic {
	return []Diagnostic{{Code: code, Severity: "error", Message: message, Path: path}}
}

func JSON(plan *Plan) (string, error) {
	payload, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return "", err
	}
	return string(payload) + "\n", nil
}

func YAML(plan *Plan) (string, error) {
	payload, err := yaml.Marshal(plan)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func ErrorJSON(payload ErrorPayload) (string, error) {
	payload.Schema = schemaForMode(payload.DryRun)
	payload.SchemaVersion = SchemaVersion
	payload.Command = Command
	bytes, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	return string(bytes) + "\n", nil
}

func NewErrorPayload(dryRun bool, configPath any, code string, message string, details map[string]any) ErrorPayload {
	return ErrorPayload{
		Schema:        schemaForMode(dryRun),
		SchemaVersion: SchemaVersion,
		Command:       Command,
		DryRun:        dryRun,
		ConfigPath:    configPath,
		Summary:       Summary{Status: "error"},
		Error: ErrorObject{
			Code:    code,
			Message: message,
			Details: details,
		},
	}
}

func buildItem(runID string, idx int, syncCfg config.Sync, configDir string, homeDir string, stat func(string) (os.FileInfo, error)) (Item, error) {
	syncRef := fmt.Sprintf("sync[%d]", idx)
	settingID := fmt.Sprintf("sync-%d", idx)
	resourceID := settingID + "-resource"
	locationID := settingID + "-location"

	expandedSourceRel, err := config.ExpandSyncPath(syncCfg.Source, fmt.Sprintf("syncs[%d].source", idx))
	if err != nil {
		return Item{}, err
	}
	expandedTargetRel, err := config.ExpandSyncPath(syncCfg.Target, fmt.Sprintf("syncs[%d].target", idx))
	if err != nil {
		return Item{}, err
	}
	expandedSourceRel = filepath.ToSlash(filepath.Clean(expandedSourceRel))
	expandedTargetRel = filepath.ToSlash(filepath.Clean(expandedTargetRel))
	expandedSourcePath := filepath.Clean(filepath.Join(configDir, filepath.FromSlash(expandedSourceRel)))
	expandedTargetPath := filepath.Clean(filepath.Join(homeDir, filepath.FromSlash(expandedTargetRel)))
	locationDefault, resourcePath := targetLocationAndResourcePath(expandedTargetRel)
	artifact := "artifacts/" + settingID
	desiredRelPath := path.Join("desired", "user", "legacy", "targets", recipe.CustomFilesTarget, artifact)

	item := Item{
		SyncIndex:             idx,
		SyncRef:               syncRef,
		LegacySource:          syncCfg.Source,
		LegacyTarget:          syncCfg.Target,
		ExpandedSourceRelPath: expandedSourceRel,
		ExpandedTargetRelPath: expandedTargetRel,
		ExpandedSourcePath:    expandedSourcePath,
		ExpandedTargetPath:    expandedTargetPath,
		TargetRef:             recipe.CustomFilesTarget,
		SettingRef:            recipe.CustomFilesTarget + ":" + settingID,
		SettingID:             settingID,
		Driver:                "unknown",
		LocationID:            locationID,
		LocationDefault:       locationDefault,
		ResourceID:            resourceID,
		ResourcePath:          resourcePath,
		DesiredArtifactBinding: DesiredArtifactBinding{
			Artifact: artifact,
			URI:      "desired://user/legacy/targets/" + recipe.CustomFilesTarget + "/artifacts/" + settingID,
			RelPath:  desiredRelPath,
		},
		Result:            "planned",
		V1ConfigAction:    "leave-unchanged",
		BehaviorUnchanged: true,
	}

	driver, diagnostics := inferDriver(expandedSourcePath, expandedTargetPath, stat)
	item.Driver = driver
	item.Diagnostics = diagnostics
	if driver == "unknown" {
		item.Result = "blocked"
		item.GeneratedFiles = nil
		return item, nil
	}
	item.GeneratedFiles = itemGeneratedFiles(runID, item)
	return item, nil
}

func inferDriver(sourcePath string, targetPath string, stat func(string) (os.FileInfo, error)) (string, []Diagnostic) {
	if driver, ok := driverFromPath(sourcePath, stat); ok {
		return driver, nil
	}
	if driver, ok := driverFromPath(targetPath, stat); ok {
		return driver, nil
	}
	return "unknown", []Diagnostic{{
		Code:     "migration-driver-unknown",
		Severity: "error",
		Message:  "cannot infer file or file-tree driver because neither legacy source nor target exists as a regular file or directory",
		Path:     sourcePath,
	}}
}

func driverFromPath(path string, stat func(string) (os.FileInfo, error)) (string, bool) {
	info, err := stat(path)
	if err != nil {
		return "", false
	}
	if info.IsDir() {
		return recipe.FileTreeDriverID, true
	}
	if info.Mode().IsRegular() {
		return recipe.FileDriverID, true
	}
	return "", false
}

func targetLocationAndResourcePath(targetRel string) (string, string) {
	slashed := filepath.ToSlash(filepath.Clean(targetRel))
	parent := path.Dir(slashed)
	base := path.Base(slashed)
	if parent == "." || parent == "/" {
		return "~", base
	}
	return "~/" + parent, base
}

func baseGeneratedFiles(runID string) []GeneratedFile {
	root := path.Join("migrations", "v1-to-v2", runID)
	return []GeneratedFile{
		{Path: path.Join(root, "migration-plan.yaml"), Kind: "migration-plan", Purpose: "records proposed v1 sync to v2 custom.files mapping"},
		{Path: path.Join(root, "generated", "dotfiles-manager.v2.yaml"), Kind: "root-config", Purpose: "generated v2 root config preview"},
		{Path: path.Join(root, "generated", "profiles", "stacks", "legacy.yaml"), Kind: "profile-stack", Purpose: "generated legacy profile stack preview"},
		{Path: path.Join(root, "generated", "profiles", "layers", "legacy.yaml"), Kind: "profile-layer", Purpose: "generated legacy profile layer preview"},
		{Path: path.Join(root, "generated", "recipes", "local", recipe.CustomFilesTarget, "recipe.yaml"), Kind: "recipe", Purpose: "generated custom.files recipe preview"},
	}
}

func itemGeneratedFiles(runID string, item Item) []GeneratedFile {
	root := path.Join("migrations", "v1-to-v2", runID, "generated")
	return []GeneratedFile{{
		Path:    path.Join(root, item.DesiredArtifactBinding.RelPath),
		Kind:    "desired-artifact",
		Purpose: "desired artifact payload copied from legacy source preview",
		SyncRef: item.SyncRef,
	}}
}

func summarize(items []Item, baseFiles []GeneratedFile) Summary {
	summary := Summary{Syncs: len(items), GeneratedFiles: len(baseFiles)}
	for _, item := range items {
		if item.Result == "blocked" {
			summary.Blocked++
		} else {
			summary.Planned++
			summary.GeneratedFiles += len(item.GeneratedFiles)
		}
		switch item.Driver {
		case recipe.FileDriverID:
			summary.Files++
		case recipe.FileTreeDriverID:
			summary.FileTrees++
		}
	}
	switch {
	case summary.Blocked == 0:
		summary.Status = "ok"
	case summary.Planned == 0:
		summary.Status = "blocked"
	default:
		summary.Status = "partial"
	}
	return summary
}

func SortedGeneratedFiles(plan *Plan) []GeneratedFile {
	if plan == nil {
		return nil
	}
	files := append([]GeneratedFile(nil), plan.GeneratedFiles...)
	for _, item := range plan.Items {
		files = append(files, item.GeneratedFiles...)
	}
	sort.SliceStable(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files
}

func NewRunID(t time.Time) string {
	if t.IsZero() {
		t = time.Now()
	}
	utc := t.UTC()
	return fmt.Sprintf("%s-%09d", utc.Format("20060102-150405"), utc.Nanosecond())
}

func validateRunID(runID string) error {
	trimmed := strings.TrimSpace(runID)
	if trimmed == "" {
		return fmt.Errorf("migration run id is required")
	}
	if trimmed != runID || !runIDPattern.MatchString(runID) || strings.Contains(runID, "..") {
		return fmt.Errorf("migration run id must be path-safe: %q", runID)
	}
	return nil
}

func schemaForMode(dryRun bool) string {
	if dryRun {
		return PreviewSchema
	}
	return MigrationPlanSchema
}

func now(opts Options) time.Time {
	if opts.Now != nil {
		return opts.Now()
	}
	return time.Now()
}

func generatedRootConfigYAML() string {
	return "schema: dotfiles-manager.v2.root-config\nschemaVersion: 1\nactiveProfileStack: legacy\n"
}

func generatedStackYAML() string {
	return "schema: dotfiles-manager.v2.profile-stack\nschemaVersion: 1\nprofileStack:\n  - legacy\n"
}

func generatedLayerYAML(items []Item) string {
	var b strings.Builder
	b.WriteString("schema: dotfiles-manager.v2.profile-layer\n")
	b.WriteString("schemaVersion: 1\n")
	b.WriteString("selections:\n")
	b.WriteString("  custom.files:\n")
	b.WriteString("    settings:\n")
	for _, item := range plannedItems(items) {
		b.WriteString("      ")
		b.WriteString(item.SettingID)
		b.WriteString(":\n")
		b.WriteString("        scope: user\n")
		b.WriteString("        artifact: ")
		b.WriteString(item.DesiredArtifactBinding.Artifact)
		b.WriteString("\n")
	}
	return b.String()
}

func generatedRecipeYAML(items []Item) string {
	planned := plannedItems(items)
	var b strings.Builder
	b.WriteString("schema: dotfiles-manager.v2.recipe\n")
	b.WriteString("schemaVersion: 1\n")
	b.WriteString("target: custom.files\n")
	b.WriteString("displayName: Migrated custom files\n")
	b.WriteString("supportLevel: experimental\n")
	b.WriteString("capability: read-write\n")
	b.WriteString("locations:\n")
	for _, item := range planned {
		b.WriteString("  ")
		b.WriteString(item.LocationID)
		b.WriteString(":\n")
		b.WriteString("    default: ")
		b.WriteString(yamlQuote(item.LocationDefault))
		b.WriteString("\n")
	}
	b.WriteString("settings:\n")
	for _, item := range planned {
		b.WriteString("  ")
		b.WriteString(item.SettingID)
		b.WriteString(":\n")
		b.WriteString("    scopeDefault: user\n")
		b.WriteString("    resource: ")
		b.WriteString(item.ResourceID)
		b.WriteString("\n")
	}
	b.WriteString("resources:\n")
	for _, item := range planned {
		b.WriteString("  ")
		b.WriteString(item.ResourceID)
		b.WriteString(":\n")
		b.WriteString("    driver: ")
		b.WriteString(item.Driver)
		b.WriteString("\n")
		b.WriteString("    location: ")
		b.WriteString(item.LocationID)
		b.WriteString("\n")
		b.WriteString("    path: ")
		b.WriteString(yamlQuote(item.ResourcePath))
		b.WriteString("\n")
	}
	return b.String()
}

func plannedItems(items []Item) []Item {
	planned := make([]Item, 0, len(items))
	for _, item := range items {
		if item.Result == "planned" {
			planned = append(planned, item)
		}
	}
	sort.SliceStable(planned, func(i, j int) bool { return planned[i].SyncIndex < planned[j].SyncIndex })
	return planned
}

func yamlQuote(value string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range value {
		switch r {
		case '\\':
			b.WriteString("\\\\")
		case '"':
			b.WriteString("\\\"")
		case '\n':
			b.WriteString("\\n")
		case '\r':
			b.WriteString("\\r")
		case '\t':
			b.WriteString("\\t")
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func writeTextFile(dest string, body string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("create parent for %s: %w", dest, err)
	}
	if err := os.WriteFile(dest, []byte(body), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", dest, err)
	}
	return nil
}

func ensureNoSymlinkInExistingPath(root string, rel string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve root %q: %w", root, err)
	}
	cleanRel, err := filedriver.ValidateRelativePath(filepath.ToSlash(rel))
	if err != nil {
		return err
	}
	current := rootAbs
	for _, part := range strings.Split(cleanRel, "/") {
		current = filepath.Join(current, filepath.FromSlash(part))
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect migration output path %s: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("migration output path must not traverse symlinks: %s", current)
		}
	}
	return nil
}
