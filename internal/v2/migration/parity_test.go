package migration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestBuildParityReportProvesGeneratedFileAndTreeParityFromRunDir(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	homeRoot := t.TempDir()
	writeFile(t, filepath.Join(repoRoot, "legacy", ".gitconfig"), "[user]\n\temail = leon@example.com\n")
	writeFile(t, filepath.Join(repoRoot, "legacy", "nvim", "init.lua"), "vim.opt.number = true\n")
	configPath := writeV1Config(t, repoRoot, `syncs:
  - source: legacy/.gitconfig
    target: .gitconfig
  - source: legacy/nvim
    target: .config/nvim
`)

	plan, err := WriteMigrationOutput(Options{ConfigPath: configPath, HomeDir: homeRoot, RunID: "fixture"})
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(homeRoot, ".config"), 0o755))

	report, err := BuildParityReport(ParityOptions{RunDir: filepath.Join(repoRoot, "migrations", "v1-to-v2", "fixture"), HomeDir: homeRoot})
	require.NoError(t, err)
	require.Equal(t, ParityReportSchema, report.Schema)
	require.Equal(t, SchemaVersion, report.SchemaVersion)
	require.Equal(t, "fixture", report.RunID)
	require.Equal(t, plan.OutputDir, report.MigrationRunDir)
	require.Equal(t, filepath.Join(plan.OutputDir, "generated"), report.GeneratedRoot)
	require.Equal(t, ParitySummary{Syncs: 2, OK: 2, Blocked: 0, Files: 1, FileTrees: 1, Status: "ok"}, report.Summary)
	require.Nil(t, report.Error)
	require.Len(t, report.Items, 2)
	require.Equal(t, []string{"sync[0]", "sync[1]"}, []string{report.Items[0].SyncRef, report.Items[1].SyncRef})

	fileItem := report.Items[0]
	require.Equal(t, "ok", fileItem.Result)
	require.Equal(t, recipe.FileDriverID, fileItem.Driver)
	require.Equal(t, filepath.Join(homeRoot, ".gitconfig"), fileItem.LiveTargetPath)
	require.Equal(t, "custom.files:sync-0", fileItem.SettingRef)
	require.Equal(t, "sync-0-resource", fileItem.ResourceID)
	require.Equal(t, "sync-0-location", fileItem.LocationID)
	require.True(t, fileItem.SourceSnapshot.Exists)
	require.True(t, fileItem.DesiredSnapshot.Exists)
	require.Equal(t, fileItem.SourceSnapshot.SHA256, fileItem.DesiredSnapshot.SHA256)
	require.Empty(t, fileItem.Diagnostics)

	treeItem := report.Items[1]
	require.Equal(t, "ok", treeItem.Result)
	require.Equal(t, recipe.FileTreeDriverID, treeItem.Driver)
	require.Equal(t, filepath.Join(homeRoot, ".config", "nvim"), treeItem.LiveTargetPath)
	require.Equal(t, "custom.files:sync-1", treeItem.SettingRef)
	require.True(t, treeItem.SourceSnapshot.Exists)
	require.True(t, treeItem.DesiredSnapshot.Exists)
	require.Equal(t, treeItem.SourceSnapshot.SHA256, treeItem.DesiredSnapshot.SHA256)
	require.Equal(t, 1, treeItem.DesiredSnapshot.FileCount)
	require.Empty(t, treeItem.Diagnostics)

	jsonPayload, err := ParityJSON(report)
	require.NoError(t, err)
	require.Contains(t, jsonPayload, `"schemaVersion": 1`)
	require.Contains(t, jsonPayload, `"runId": "fixture"`)
	require.Contains(t, jsonPayload, `"syncRef": "sync[0]"`)
	require.NotContains(t, jsonPayload, "schema_version")

	var decodedJSON map[string]any
	require.NoError(t, json.Unmarshal([]byte(jsonPayload), &decodedJSON))
	require.Equal(t, ParityReportSchema, decodedJSON["schema"])

	yamlPayload, err := ParityYAML(report)
	require.NoError(t, err)
	var decodedYAML map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(yamlPayload), &decodedYAML))
	require.Equal(t, ParityReportSchema, decodedYAML["schema"])

	textPayload := ParityText(report)
	require.Contains(t, textPayload, "migration parity report")
	require.Contains(t, textPayload, "run: fixture")
	require.Contains(t, textPayload, "sync[0]")
	require.Contains(t, textPayload, "desired artifact:")
	require.Contains(t, textPayload, "summary syncs=2 ok=2 blocked=0 files=1 file-trees=1 status=ok")
}

func TestBuildParityReportBlocksWhenGeneratedArtifactMissing(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	homeRoot := t.TempDir()
	writeFile(t, filepath.Join(repoRoot, "legacy", ".gitconfig"), "[user]\n\temail = leon@example.com\n")
	configPath := writeV1Config(t, repoRoot, `syncs:
  - source: legacy/.gitconfig
    target: .gitconfig
`)
	plan, err := WriteMigrationOutput(Options{ConfigPath: configPath, HomeDir: homeRoot, RunID: "fixture"})
	require.NoError(t, err)
	require.NoError(t, os.Remove(filepath.Join(plan.OutputDir, "generated", filepath.FromSlash(plan.Items[0].DesiredArtifactBinding.RelPath))))

	report, err := BuildParityReport(ParityOptions{RunDir: plan.OutputDir, HomeDir: homeRoot})
	require.NoError(t, err)
	require.Equal(t, ParitySummary{Syncs: 1, OK: 0, Blocked: 1, Files: 1, FileTrees: 0, Status: "blocked"}, report.Summary)
	require.NotNil(t, report.Error)
	require.Equal(t, "blocked", report.Items[0].Result)
	requireDiagnosticCode(t, report.Items[0].Diagnostics, "parity-generated-artifact-missing")
}

func TestBuildParityReportCarriesBlockedMigrationItems(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	homeRoot := t.TempDir()
	configPath := writeV1Config(t, repoRoot, `syncs:
  - source: missing/source
    target: .missing-target
`)

	plan, err := BuildDryRunPlan(Options{ConfigPath: configPath, HomeDir: homeRoot, RunID: "blocked"})
	require.NoError(t, err)
	plan.Schema = MigrationPlanSchema
	plan.DryRun = false
	report, err := BuildParityReport(ParityOptions{Plan: plan, GeneratedRoot: t.TempDir(), HomeDir: homeRoot})
	require.NoError(t, err)
	require.Equal(t, ParitySummary{Syncs: 1, OK: 0, Blocked: 1, Files: 0, FileTrees: 0, Status: "blocked"}, report.Summary)
	require.Equal(t, "blocked", report.Items[0].Result)
	requireDiagnosticCode(t, report.Items[0].Diagnostics, "migration-driver-unknown")
	requireDiagnosticCode(t, report.Items[0].Diagnostics, "parity-migration-item-blocked")
}

func TestBuildParityReportBlocksWhenGeneratedRootCannotResolve(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	homeRoot := t.TempDir()
	writeFile(t, filepath.Join(repoRoot, "legacy", ".gitconfig"), "[user]\n\temail = leon@example.com\n")
	configPath := writeV1Config(t, repoRoot, `syncs:
  - source: legacy/.gitconfig
    target: .gitconfig
`)
	plan, err := BuildMigrationOutput(Options{ConfigPath: configPath, HomeDir: homeRoot, RunID: "fixture"})
	require.NoError(t, err)

	report, err := BuildParityReport(ParityOptions{Plan: plan.Plan, GeneratedRoot: t.TempDir(), HomeDir: homeRoot})
	require.NoError(t, err)
	require.Equal(t, "blocked", report.Summary.Status)
	requireDiagnosticCode(t, report.Items[0].Diagnostics, "parity-generated-root-unavailable")
}

func TestBuildParityReportBlocksGeneratedFileTreeRecipeDrift(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	homeRoot := t.TempDir()
	writeFile(t, filepath.Join(repoRoot, "legacy", ".gitconfig"), "[user]\n\temail = leon@example.com\n")
	writeFile(t, filepath.Join(repoRoot, "legacy", "nvim", "init.lua"), "vim.opt.number = true\n")
	configPath := writeV1Config(t, repoRoot, `syncs:
  - source: legacy/.gitconfig
    target: .gitconfig
  - source: legacy/nvim
    target: .config/nvim
`)
	plan, err := WriteMigrationOutput(Options{ConfigPath: configPath, HomeDir: homeRoot, RunID: "fixture"})
	require.NoError(t, err)

	recipePath := filepath.Join(plan.OutputDir, "generated", "recipes", "local", recipe.CustomFilesTarget, "recipe.yaml")
	body, err := os.ReadFile(recipePath)
	require.NoError(t, err)
	rewritten := string(body)
	rewritten = replaceOnce(t, rewritten, `    default: "~/.config"`, `    default: "~/.generated-config"`)
	rewritten = replaceOnce(t, rewritten, `    path: "nvim"`, `    path: "nvim"
    include:
      - "**/*.lua"`)
	require.NoError(t, os.WriteFile(recipePath, []byte(rewritten), 0o644))

	report, err := BuildParityReport(ParityOptions{RunDir: plan.OutputDir, HomeDir: homeRoot})
	require.NoError(t, err)
	require.Equal(t, "blocked", report.Summary.Status)
	requireDiagnosticCode(t, report.Items[1].Diagnostics, "parity-generated-globs-drift")
	requireDiagnosticCode(t, report.Items[1].Diagnostics, "parity-generated-location-drift")
}

func TestBuildParityReportUsesDefaultHomeWhenHomeEnvMatchesMigration(t *testing.T) {
	repoRoot := t.TempDir()
	homeRoot := t.TempDir()
	oldHome, hadHome := os.LookupEnv("HOME")
	require.NoError(t, os.Setenv("HOME", homeRoot))
	t.Cleanup(func() {
		if hadHome {
			_ = os.Setenv("HOME", oldHome)
		} else {
			_ = os.Unsetenv("HOME")
		}
	})

	writeFile(t, filepath.Join(repoRoot, "legacy", ".gitconfig"), "[user]\n\temail = leon@example.com\n")
	configPath := writeV1Config(t, repoRoot, `syncs:
  - source: legacy/.gitconfig
    target: .gitconfig
`)
	plan, err := WriteMigrationOutput(Options{ConfigPath: configPath, HomeDir: homeRoot, RunID: "fixture"})
	require.NoError(t, err)

	report, err := BuildParityReport(ParityOptions{RunDir: plan.OutputDir})
	require.NoError(t, err)
	require.Equal(t, "ok", report.Summary.Status)
	require.Equal(t, filepath.Join(homeRoot, ".gitconfig"), report.Items[0].LiveTargetPath)
}

func TestBuildParityReportBlocksOnSourceMissingAndTargetMismatch(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	homeRoot := t.TempDir()
	writeFile(t, filepath.Join(repoRoot, "legacy", ".gitconfig"), "[user]\n\temail = leon@example.com\n")
	configPath := writeV1Config(t, repoRoot, `syncs:
  - source: legacy/.gitconfig
    target: .gitconfig
`)
	plan, err := WriteMigrationOutput(Options{ConfigPath: configPath, HomeDir: homeRoot, RunID: "fixture"})
	require.NoError(t, err)
	require.NoError(t, os.Remove(filepath.Join(repoRoot, "legacy", ".gitconfig")))
	differentHome := t.TempDir()

	report, err := BuildParityReport(ParityOptions{RunDir: plan.OutputDir, HomeDir: differentHome})
	require.NoError(t, err)
	require.Equal(t, "blocked", report.Summary.Status)
	requireDiagnosticCode(t, report.Items[0].Diagnostics, "parity-live-target-mismatch")
	requireDiagnosticCode(t, report.Items[0].Diagnostics, "parity-legacy-source-unavailable")
}

func TestBuildParityReportValidationHelperBranches(t *testing.T) {
	t.Parallel()

	_, err := BuildParityReport(ParityOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "migration run dir is required")

	runDir := t.TempDir()
	writeFile(t, filepath.Join(runDir, "migration-plan.yaml"), "schema: wrong\nschemaVersion: 1\n")
	_, err = BuildParityReport(ParityOptions{RunDir: runDir})
	require.Error(t, err)
	require.Contains(t, err.Error(), "migration plan schema")

	_, err = BuildParityReport(ParityOptions{Plan: &Plan{Schema: MigrationPlanSchema, SchemaVersion: SchemaVersion - 1, Command: Command, RunID: "fixture"}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "schemaVersion")

	_, err = BuildParityReport(ParityOptions{Plan: &Plan{Schema: MigrationPlanSchema, SchemaVersion: SchemaVersion, Command: "wrong", RunID: "fixture"}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "command")

	_, err = BuildParityReport(ParityOptions{Plan: &Plan{Schema: MigrationPlanSchema, SchemaVersion: SchemaVersion, Command: Command, RunID: "fixture", DryRun: true}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "non-dry-run")

	_, err = BuildParityReport(ParityOptions{Plan: &Plan{Schema: MigrationPlanSchema, SchemaVersion: SchemaVersion, Command: Command, RunID: "fixture"}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "generated root is required")

	expanded, err := expandParityLocationDefault("~/Library/Application Support", "/tmp/home")
	require.NoError(t, err)
	require.Equal(t, filepath.Join("/tmp/home", "Library", "Application Support"), expanded)

	expanded, err = expandParityLocationDefault("/Library/Application Support", "/tmp/home")
	require.NoError(t, err)
	require.Equal(t, "/Library/Application Support", expanded)

	_, err = expandParityLocationDefault("", "/tmp/home")
	require.Error(t, err)
	_, err = expandParityLocationDefault("bad\x00path", "/tmp/home")
	require.Error(t, err)
}

func TestGeneratedParityMappingDiagnosticsReportRecipeDrift(t *testing.T) {
	t.Parallel()

	item := Item{
		SettingRef:         recipe.CustomFilesTarget + ":sync-0",
		SettingID:          "sync-0",
		Driver:             recipe.FileTreeDriverID,
		LocationID:         "sync-0-location",
		LocationDefault:    "~/.config",
		ResourceID:         "sync-0-resource",
		ResourcePath:       "nvim",
		ExpandedTargetPath: "/home/user/.config/nvim",
	}

	requireDiagnosticCode(t, generatedParityMappingDiagnostics(item, nil), "parity-generated-root-unavailable")
	requireDiagnosticCode(t, generatedParityMappingDiagnostics(item, &recipe.Recipe{}), "parity-generated-setting-missing")

	diagnostics := generatedParityMappingDiagnostics(item, &recipe.Recipe{
		Settings: map[string]recipe.Setting{item.SettingID: {Resource: "other-resource"}},
	})
	requireDiagnosticCode(t, diagnostics, "parity-generated-setting-drift")
	requireDiagnosticCode(t, diagnostics, "parity-generated-resource-missing")

	diagnostics = generatedParityMappingDiagnostics(item, &recipe.Recipe{
		Settings: map[string]recipe.Setting{item.SettingID: {Resource: item.ResourceID}},
		Resources: map[string]recipe.Resource{item.ResourceID: {
			Driver:   recipe.FileDriverID,
			Location: "other-location",
			Path:     "other-path",
			Include:  []string{"**/*.lua"},
		}},
	})
	requireDiagnosticCode(t, diagnostics, "parity-generated-resource-drift")
	requireDiagnosticCode(t, diagnostics, "parity-generated-globs-drift")
	requireDiagnosticCode(t, diagnostics, "parity-generated-location-missing")
}

func TestParityTextAndErrorReportHelperBranches(t *testing.T) {
	t.Parallel()

	require.Contains(t, ParityText(nil), "summary syncs=0 ok=0 blocked=0 files=0 file-trees=0 status=error")

	report := NewParityErrorReport("missing-run", "DFM_TEST", "cannot read migration plan", map[string]any{"runDir": "missing-run"})
	require.Equal(t, ParityReportSchema, report.Schema)
	require.Equal(t, SchemaVersion, report.SchemaVersion)
	require.Equal(t, "missing-run", report.MigrationRunDir)
	require.Equal(t, "error", report.Summary.Status)
	require.Equal(t, "DFM_TEST", report.Error.Code)
	require.Equal(t, "missing-run", report.Error.Details["runDir"])

	textPayload := ParityText(report)
	require.Contains(t, textPayload, "migration parity report")
	require.Contains(t, textPayload, "run dir: missing-run")
	require.Contains(t, textPayload, "error[DFM_TEST]: cannot read migration plan")
	require.Contains(t, textPayload, "summary syncs=0 ok=0 blocked=0 files=0 file-trees=0 status=error")
}

func requireDiagnosticCode(t *testing.T, diagnostics []Diagnostic, code string) {
	t.Helper()
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return
		}
	}
	require.Failf(t, "missing diagnostic", "diagnostic code %s not found in %#v", code, diagnostics)
}

func replaceOnce(t *testing.T, input string, old string, replacement string) string {
	t.Helper()
	next := strings.Replace(input, old, replacement, 1)
	require.NotEqual(t, input, next)
	return next
}
