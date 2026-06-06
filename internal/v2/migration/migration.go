// Package migration builds preview-only v1-to-v2 migration plans.
package migration

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/shpoont/dotfiles-manager/internal/config"
	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
)

const (
	Schema        = "dotfiles-manager.v2.migration-preview"
	SchemaVersion = 1
	Command       = "migrate"

	DefaultRunID = "dry-run"
)

type Options struct {
	ConfigPath string
	RunID      string
	HomeDir    string
	Stat       func(string) (os.FileInfo, error)
}

type Plan struct {
	Schema         string          `json:"schema"`
	SchemaVersion  int             `json:"schemaVersion"`
	Command        string          `json:"command"`
	RunID          string          `json:"runId"`
	DryRun         bool            `json:"dryRun"`
	ConfigPath     string          `json:"configPath"`
	Items          []Item          `json:"items"`
	GeneratedFiles []GeneratedFile `json:"generatedFiles"`
	Summary        Summary         `json:"summary"`
	Error          any             `json:"error"`
}

type Item struct {
	SyncIndex              int                    `json:"syncIndex"`
	SyncRef                string                 `json:"syncRef"`
	LegacySource           string                 `json:"legacySource"`
	LegacyTarget           string                 `json:"legacyTarget"`
	ExpandedSourcePath     string                 `json:"expandedSourcePath"`
	ExpandedTargetPath     string                 `json:"expandedTargetPath"`
	TargetRef              string                 `json:"targetRef"`
	SettingRef             string                 `json:"settingRef"`
	SettingID              string                 `json:"settingId"`
	Driver                 string                 `json:"driver"`
	LocationID             string                 `json:"locationId"`
	LocationDefault        string                 `json:"locationDefault"`
	ResourceID             string                 `json:"resourceId"`
	ResourcePath           string                 `json:"resourcePath"`
	DesiredArtifactBinding DesiredArtifactBinding `json:"desiredArtifactBinding"`
	GeneratedFiles         []GeneratedFile        `json:"generatedFiles"`
	Result                 string                 `json:"result"`
	Diagnostics            []Diagnostic           `json:"diagnostics,omitempty"`
	V1ConfigAction         string                 `json:"v1ConfigAction"`
	BehaviorUnchanged      bool                   `json:"behaviorUnchanged"`
}

type DesiredArtifactBinding struct {
	Artifact string `json:"artifact"`
	URI      string `json:"uri"`
	RelPath  string `json:"relPath"`
}

type GeneratedFile struct {
	Path    string `json:"path"`
	Kind    string `json:"kind"`
	Purpose string `json:"purpose"`
	SyncRef string `json:"syncRef,omitempty"`
}

type Diagnostic struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Path     string `json:"path,omitempty"`
}

type Summary struct {
	Syncs          int    `json:"syncs"`
	Planned        int    `json:"planned"`
	Blocked        int    `json:"blocked"`
	Files          int    `json:"files"`
	FileTrees      int    `json:"fileTrees"`
	GeneratedFiles int    `json:"generatedFiles"`
	Status         string `json:"status"`
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
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

func BuildDryRunPlan(opts Options) (*Plan, error) {
	configPath := strings.TrimSpace(opts.ConfigPath)
	if configPath == "" {
		return nil, fmt.Errorf("config path is required")
	}
	absConfigPath, err := filepath.Abs(configPath)
	if err != nil {
		return nil, fmt.Errorf("resolve config path %q: %w", configPath, err)
	}
	cfg, err := config.Load(absConfigPath)
	if err != nil {
		return nil, err
	}
	return BuildDryRunPlanFromConfig(cfg, absConfigPath, opts)
}

func BuildDryRunPlanFromConfig(cfg *config.Config, configPath string, opts Options) (*Plan, error) {
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
		runID = DefaultRunID
	}

	plan := &Plan{
		Schema:        Schema,
		SchemaVersion: SchemaVersion,
		Command:       Command,
		RunID:         runID,
		DryRun:        true,
		ConfigPath:    absConfigPath,
		Error:         nil,
	}
	plan.GeneratedFiles = baseGeneratedFiles(runID)

	configDir := filepath.Dir(absConfigPath)
	for idx, syncCfg := range cfg.Syncs {
		item, err := buildItem(runID, idx, syncCfg, configDir, homeDir, stat)
		if err != nil {
			return nil, err
		}
		plan.Items = append(plan.Items, item)
	}
	plan.Summary = summarize(plan.Items, plan.GeneratedFiles)
	return plan, nil
}

func JSON(plan *Plan) (string, error) {
	payload, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return "", err
	}
	return string(payload) + "\n", nil
}

func ErrorJSON(payload ErrorPayload) (string, error) {
	payload.Schema = Schema
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
		Schema:        Schema,
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
	locationID := "legacy-target"

	expandedSourceRel, err := config.ExpandSyncPath(syncCfg.Source, fmt.Sprintf("syncs[%d].source", idx))
	if err != nil {
		return Item{}, err
	}
	expandedTargetRel, err := config.ExpandSyncPath(syncCfg.Target, fmt.Sprintf("syncs[%d].target", idx))
	if err != nil {
		return Item{}, err
	}
	expandedSourcePath := filepath.Clean(filepath.Join(configDir, expandedSourceRel))
	expandedTargetPath := filepath.Clean(filepath.Join(homeDir, expandedTargetRel))
	locationDefault, resourcePath := targetLocationAndResourcePath(expandedTargetRel)
	artifact := "artifacts/" + settingID
	desiredRelPath := path.Join("desired", "user", "legacy", "targets", recipe.CustomFilesTarget, artifact)

	item := Item{
		SyncIndex:          idx,
		SyncRef:            syncRef,
		LegacySource:       syncCfg.Source,
		LegacyTarget:       syncCfg.Target,
		ExpandedSourcePath: expandedSourcePath,
		ExpandedTargetPath: expandedTargetPath,
		TargetRef:          recipe.CustomFilesTarget,
		SettingRef:         recipe.CustomFilesTarget + ":" + settingID,
		SettingID:          settingID,
		Driver:             "unknown",
		LocationID:         locationID,
		LocationDefault:    locationDefault,
		ResourceID:         resourceID,
		ResourcePath:       resourcePath,
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
