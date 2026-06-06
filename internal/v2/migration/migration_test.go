package migration

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shpoont/dotfiles-manager/internal/config"
	"github.com/shpoont/dotfiles-manager/internal/v2/customfiles"
	"github.com/shpoont/dotfiles-manager/internal/v2/filedriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/shpoont/dotfiles-manager/internal/v2/resolution"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestDryRunPlanPreservesLegacyPathsAndInfersFileDriver(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	homeRoot := t.TempDir()
	writeFile(t, filepath.Join(repoRoot, "dotfiles", "git", ".gitconfig"), "[user]\n\temail = leon@example.com\n")
	configPath := writeV1Config(t, repoRoot, `syncs:
  - source: dotfiles/git/.gitconfig
    target: .gitconfig
`)

	plan, err := BuildDryRunPlan(Options{ConfigPath: configPath, HomeDir: homeRoot, RunID: "test-run"})
	require.NoError(t, err)
	require.Equal(t, Schema, plan.Schema)
	require.Equal(t, 1, plan.SchemaVersion)
	require.True(t, plan.DryRun)
	require.Equal(t, "test-run", plan.RunID)
	require.Equal(t, Summary{Syncs: 1, Planned: 1, Blocked: 0, Files: 1, FileTrees: 0, GeneratedFiles: 6, Status: "ok"}, plan.Summary)
	require.Len(t, plan.Items, 1)

	item := plan.Items[0]
	require.Equal(t, "sync[0]", item.SyncRef)
	require.Equal(t, "dotfiles/git/.gitconfig", item.LegacySource)
	require.Equal(t, ".gitconfig", item.LegacyTarget)
	require.Equal(t, filepath.Join(repoRoot, "dotfiles", "git", ".gitconfig"), item.ExpandedSourcePath)
	require.Equal(t, filepath.Join(homeRoot, ".gitconfig"), item.ExpandedTargetPath)
	require.Equal(t, "custom.files", item.TargetRef)
	require.Equal(t, "custom.files:sync-0", item.SettingRef)
	require.Equal(t, recipe.FileDriverID, item.Driver)
	require.Equal(t, "~", item.LocationDefault)
	require.Equal(t, ".gitconfig", item.ResourcePath)
	require.Equal(t, "artifacts/sync-0", item.DesiredArtifactBinding.Artifact)
	require.Equal(t, "desired://user/legacy/targets/custom.files/artifacts/sync-0", item.DesiredArtifactBinding.URI)
	require.Equal(t, "desired/user/legacy/targets/custom.files/artifacts/sync-0", item.DesiredArtifactBinding.RelPath)
	require.Equal(t, "leave-unchanged", item.V1ConfigAction)
	require.True(t, item.BehaviorUnchanged)
	require.Empty(t, item.Diagnostics)
	require.NoDirExists(t, filepath.Join(repoRoot, "migrations"))
}

func TestDryRunPlanInfersFileTreeDriver(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	homeRoot := t.TempDir()
	writeFile(t, filepath.Join(repoRoot, "dotfiles", "nvim", "init.lua"), "vim.opt.number = true\n")
	configPath := writeV1Config(t, repoRoot, `syncs:
  - source: dotfiles/nvim
    target: .config/nvim
`)

	plan, err := BuildDryRunPlan(Options{ConfigPath: configPath, HomeDir: homeRoot})
	require.NoError(t, err)
	require.Len(t, plan.Items, 1)
	require.Equal(t, recipe.FileTreeDriverID, plan.Items[0].Driver)
	require.Equal(t, "~/.config", plan.Items[0].LocationDefault)
	require.Equal(t, "nvim", plan.Items[0].ResourcePath)
	require.Equal(t, Summary{Syncs: 1, Planned: 1, Blocked: 0, Files: 0, FileTrees: 1, GeneratedFiles: 6, Status: "ok"}, plan.Summary)
}

func TestDryRunPlanKeepsBlockedItemsWhenDriverCannotBeInferred(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	homeRoot := t.TempDir()
	configPath := writeV1Config(t, repoRoot, `syncs:
  - source: missing/source
    target: .missing-target
`)

	plan, err := BuildDryRunPlan(Options{ConfigPath: configPath, HomeDir: homeRoot})
	require.NoError(t, err)
	require.Len(t, plan.Items, 1)
	item := plan.Items[0]
	require.Equal(t, "blocked", item.Result)
	require.Equal(t, "unknown", item.Driver)
	require.Empty(t, item.GeneratedFiles)
	require.Len(t, item.Diagnostics, 1)
	require.Equal(t, "migration-driver-unknown", item.Diagnostics[0].Code)
	require.Equal(t, Summary{Syncs: 1, Planned: 0, Blocked: 1, Files: 0, FileTrees: 0, GeneratedFiles: 5, Status: "blocked"}, plan.Summary)

	text := Text(plan)
	require.Contains(t, text, "result: blocked")
	require.Contains(t, text, "diagnostic[migration-driver-unknown]")
}

func TestMigratedFileSyncBehavesThroughV2CustomFilesSlice(t *testing.T) {
	t.Parallel()

	v1RepoRoot := t.TempDir()
	liveRoot := t.TempDir()
	writeFile(t, filepath.Join(v1RepoRoot, "legacy", ".gitconfig"), "[user]\n\temail = leon@example.com\n")
	configPath := writeV1Config(t, v1RepoRoot, `syncs:
  - source: legacy/.gitconfig
    target: .gitconfig
`)

	plan, err := BuildDryRunPlan(Options{ConfigPath: configPath, HomeDir: liveRoot, RunID: "fixture"})
	require.NoError(t, err)
	require.Len(t, plan.Items, 1)
	item := plan.Items[0]
	require.Equal(t, recipe.FileDriverID, item.Driver)

	v2Root := t.TempDir()
	writeSingleItemV2Fixture(t, v2Root, item, "[user]\n\temail = leon@example.com\n")

	profile, err := resolution.Resolve(v2Root, resolution.ResolveOptions{UserID: "legacy"})
	require.NoError(t, err)
	rec, err := recipe.LoadCustomFiles(v2Root)
	require.NoError(t, err)

	req := customfiles.Request{
		Profile:       profile,
		Recipe:        rec,
		SettingRef:    item.SettingRef,
		LocationRoots: map[string]string{item.LocationID: liveRoot},
	}
	applyPlan, err := customfiles.PlanApply(req)
	require.NoError(t, err)
	require.Equal(t, filedriver.ChangeCreate, applyPlan.Preview.Change.Kind)

	dry, err := customfiles.Execute(applyPlan, customfiles.ExecuteOptions{DryRun: true})
	require.NoError(t, err)
	require.False(t, dry.Mutated)
	require.NoFileExists(t, filepath.Join(liveRoot, ".gitconfig"))

	result, err := customfiles.Execute(applyPlan, customfiles.ExecuteOptions{})
	require.NoError(t, err)
	require.True(t, result.Mutated)
	require.True(t, result.Verified)
	requireFile(t, filepath.Join(liveRoot, ".gitconfig"), "[user]\n\temail = leon@example.com\n")
}

func TestDryRunPlanInfersDriverFromTargetWhenSourceIsMissing(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	homeRoot := t.TempDir()
	writeFile(t, filepath.Join(homeRoot, ".fallback-target"), "target content\n")
	configPath := writeV1Config(t, repoRoot, `syncs:
  - source: missing/source
    target: .fallback-target
`)

	plan, err := BuildDryRunPlan(Options{ConfigPath: configPath, HomeDir: homeRoot})
	require.NoError(t, err)
	require.Len(t, plan.Items, 1)
	require.Equal(t, recipe.FileDriverID, plan.Items[0].Driver)
	require.Equal(t, "planned", plan.Items[0].Result)
}

func TestWriteMigrationOutputGeneratesV2RunForSingleAndMultiSyncs(t *testing.T) {
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
	v1Before := readFile(t, configPath)

	plan, err := WriteMigrationOutput(Options{ConfigPath: configPath, HomeDir: homeRoot, RunID: "fixture"})
	require.NoError(t, err)
	require.Equal(t, MigrationPlanSchema, plan.Schema)
	require.False(t, plan.DryRun)
	require.Equal(t, Summary{Syncs: 2, Planned: 2, Blocked: 0, Files: 1, FileTrees: 1, GeneratedFiles: 7, Status: "ok"}, plan.Summary)

	runRoot := filepath.Join(repoRoot, "migrations", "v1-to-v2", "fixture")
	generatedRoot := filepath.Join(runRoot, "generated")
	require.FileExists(t, filepath.Join(runRoot, "migration-plan.yaml"))
	require.FileExists(t, filepath.Join(generatedRoot, "dotfiles-manager.v2.yaml"))
	require.FileExists(t, filepath.Join(generatedRoot, "profiles", "stacks", "legacy.yaml"))
	require.FileExists(t, filepath.Join(generatedRoot, "profiles", "layers", "legacy.yaml"))
	require.FileExists(t, filepath.Join(generatedRoot, "recipes", "local", "custom.files", "recipe.yaml"))
	requireFile(t, filepath.Join(generatedRoot, "desired", "user", "legacy", "targets", "custom.files", "artifacts", "sync-0"), "[user]\n\temail = leon@example.com\n")
	requireFile(t, filepath.Join(generatedRoot, "desired", "user", "legacy", "targets", "custom.files", "artifacts", "sync-1", "init.lua"), "vim.opt.number = true\n")

	require.Equal(t, v1Before, readFile(t, configPath))
	_, err = config.Load(configPath)
	require.NoError(t, err)
	require.NoFileExists(t, filepath.Join(repoRoot, "dotfiles-manager.v2.yaml"))
	require.NoDirExists(t, filepath.Join(repoRoot, "profiles"))
	require.NoDirExists(t, filepath.Join(repoRoot, "desired"))
	require.NoDirExists(t, filepath.Join(repoRoot, "recipes"))

	var planYAML map[string]any
	require.NoError(t, yaml.Unmarshal(readFile(t, filepath.Join(runRoot, "migration-plan.yaml")), &planYAML))
	require.Equal(t, MigrationPlanSchema, planYAML["schema"])
	require.Equal(t, 1, planYAML["schemaVersion"])

	profile, err := resolution.Resolve(generatedRoot, resolution.ResolveOptions{UserID: "legacy"})
	require.NoError(t, err)
	require.Len(t, profile.Settings, 2)
	rec, err := recipe.LoadCustomFiles(generatedRoot)
	require.NoError(t, err)
	require.Len(t, rec.Resources, 2)
	require.Len(t, rec.Locations, 2)

	fileItem := plan.Items[0]
	fileReq := customfiles.Request{
		Profile:       profile,
		Recipe:        rec,
		SettingRef:    fileItem.SettingRef,
		LocationRoots: map[string]string{fileItem.LocationID: homeRoot},
	}
	filePlan, err := customfiles.PlanApply(fileReq)
	require.NoError(t, err)
	require.Equal(t, filedriver.ChangeCreate, filePlan.Preview.Change.Kind)
	fileResult, err := customfiles.Execute(filePlan, customfiles.ExecuteOptions{})
	require.NoError(t, err)
	require.True(t, fileResult.Verified)
	requireFile(t, filepath.Join(homeRoot, ".gitconfig"), "[user]\n\temail = leon@example.com\n")

	treeItem := plan.Items[1]
	require.NoError(t, os.MkdirAll(filepath.Join(homeRoot, ".config"), 0o755))
	treeReq := customfiles.Request{
		Profile:       profile,
		Recipe:        rec,
		SettingRef:    treeItem.SettingRef,
		LocationRoots: map[string]string{treeItem.LocationID: filepath.Join(homeRoot, ".config")},
	}
	treePlan, err := customfiles.PlanApply(treeReq)
	require.NoError(t, err)
	require.Equal(t, filedriver.ChangeCreate, treePlan.TreePreview.Change.Kind)
	treeDry, err := customfiles.Execute(treePlan, customfiles.ExecuteOptions{DryRun: true})
	require.NoError(t, err)
	require.False(t, treeDry.Mutated)
	require.NoFileExists(t, filepath.Join(homeRoot, ".config", "nvim", "init.lua"))
	treeResult, err := customfiles.Execute(treePlan, customfiles.ExecuteOptions{})
	require.NoError(t, err)
	require.True(t, treeResult.Verified)
	requireFile(t, filepath.Join(homeRoot, ".config", "nvim", "init.lua"), "vim.opt.number = true\n")
}

func TestWriteMigrationOutputBlocksAndWritesNothingWhenSourceMissing(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	homeRoot := t.TempDir()
	writeFile(t, filepath.Join(homeRoot, ".fallback-target"), "live fallback\n")
	configPath := writeV1Config(t, repoRoot, `syncs:
  - source: missing/source
    target: .fallback-target
`)

	plan, err := WriteMigrationOutput(Options{ConfigPath: configPath, HomeDir: homeRoot, RunID: "blocked"})
	require.Error(t, err)
	require.True(t, IsBlocked(err))
	require.NotNil(t, plan)
	require.Equal(t, "blocked", plan.Summary.Status)
	require.Equal(t, "blocked", plan.Items[0].Result)
	require.Equal(t, "migration-source-unavailable", plan.Items[0].Diagnostics[len(plan.Items[0].Diagnostics)-1].Code)
	require.NoDirExists(t, filepath.Join(repoRoot, "migrations"))
}

func TestWriteMigrationOutputRefusesExistingRunDirectory(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	homeRoot := t.TempDir()
	writeFile(t, filepath.Join(repoRoot, "legacy", ".gitconfig"), "[user]\n\temail = leon@example.com\n")
	configPath := writeV1Config(t, repoRoot, `syncs:
  - source: legacy/.gitconfig
    target: .gitconfig
`)
	existing := filepath.Join(repoRoot, "migrations", "v1-to-v2", "fixture")
	writeFile(t, filepath.Join(existing, "keep.txt"), "do not overwrite\n")

	plan, err := WriteMigrationOutput(Options{ConfigPath: configPath, HomeDir: homeRoot, RunID: "fixture"})
	require.Error(t, err)
	require.NotNil(t, plan)
	require.Contains(t, err.Error(), "already exists")
	requireFile(t, filepath.Join(existing, "keep.txt"), "do not overwrite\n")
}

func TestWriteMigrationOutputBlocksSourceSymlinkEscapeAndWritesNothing(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	homeRoot := t.TempDir()
	outsideRoot := t.TempDir()
	outsideFile := filepath.Join(outsideRoot, "outside.txt")
	writeFile(t, outsideFile, "outside\n")
	require.NoError(t, os.Symlink(outsideFile, filepath.Join(repoRoot, "source-link")))
	configPath := writeV1Config(t, repoRoot, `syncs:
  - source: source-link
    target: .outside
`)

	plan, err := WriteMigrationOutput(Options{ConfigPath: configPath, HomeDir: homeRoot, RunID: "blocked"})
	require.Error(t, err)
	require.True(t, IsBlocked(err))
	require.NotNil(t, plan)
	require.Equal(t, "blocked", plan.Summary.Status)
	require.Equal(t, "migration-source-unavailable", plan.Items[0].Diagnostics[len(plan.Items[0].Diagnostics)-1].Code)
	require.NoDirExists(t, filepath.Join(repoRoot, "migrations"))
}

func TestWriteMigrationOutputRejectsMigrationPathSymlinks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		setupSymlink  func(t *testing.T, repoRoot string, outsideRoot string)
		outsideRunDir func(outsideRoot string) string
	}{
		{
			name: "migrations symlink",
			setupSymlink: func(t *testing.T, repoRoot string, outsideRoot string) {
				t.Helper()
				require.NoError(t, os.Symlink(outsideRoot, filepath.Join(repoRoot, "migrations")))
			},
			outsideRunDir: func(outsideRoot string) string {
				return filepath.Join(outsideRoot, "v1-to-v2", "fixture")
			},
		},
		{
			name: "v1-to-v2 symlink",
			setupSymlink: func(t *testing.T, repoRoot string, outsideRoot string) {
				t.Helper()
				require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "migrations"), 0o755))
				require.NoError(t, os.Symlink(outsideRoot, filepath.Join(repoRoot, "migrations", "v1-to-v2")))
			},
			outsideRunDir: func(outsideRoot string) string {
				return filepath.Join(outsideRoot, "fixture")
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repoRoot := t.TempDir()
			homeRoot := t.TempDir()
			outsideRoot := t.TempDir()
			writeFile(t, filepath.Join(repoRoot, "legacy", ".gitconfig"), "[user]\n\temail = leon@example.com\n")
			configPath := writeV1Config(t, repoRoot, `syncs:
  - source: legacy/.gitconfig
    target: .gitconfig
`)
			tc.setupSymlink(t, repoRoot, outsideRoot)

			plan, err := WriteMigrationOutput(Options{ConfigPath: configPath, HomeDir: homeRoot, RunID: "fixture"})
			require.Error(t, err)
			require.NotNil(t, plan)
			require.Contains(t, err.Error(), "must not traverse symlinks")
			require.NoDirExists(t, tc.outsideRunDir(outsideRoot))
		})
	}
}

func TestMigrationOutputValidationAndHelperBranches(t *testing.T) {
	t.Parallel()

	var nilOutput *Output
	require.Error(t, nilOutput.Write())
	require.Error(t, (&Output{}).Write())
	require.Error(t, (&Output{Plan: &Plan{DryRun: true}}).Write())

	blockedPlan := &Plan{Summary: Summary{Blocked: 2}}
	err := (&Output{Plan: blockedPlan}).Write()
	require.Error(t, err)
	require.True(t, IsBlocked(err))
	require.Equal(t, "migration blocked", (&BlockedError{}).Error())
	require.Contains(t, (&BlockedError{Plan: blockedPlan}).Error(), "2 blocked")

	require.Equal(t, "20260102-030405-000000006", NewRunID(fixedTimeForRunID()))
	generated, err := BuildMigrationOutput(Options{ConfigPath: writeOneFileMigrationRepo(t), Now: fixedTimeForRunID})
	require.NoError(t, err)
	require.Equal(t, "20260102-030405-000000006", generated.Plan.RunID)
	require.NoDirExists(t, filepath.Join(generated.Plan.RepoRoot, "migrations"))

	_, err = BuildMigrationOutput(Options{ConfigPath: generated.Plan.ConfigPath, RunID: "../bad"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "path-safe")

	_, err = JSON(&Plan{Error: func() {}})
	require.Error(t, err)

	parentFile := filepath.Join(t.TempDir(), "parent-file")
	writeFile(t, parentFile, "not a dir\n")
	err = writeTextFile(filepath.Join(parentFile, "child.txt"), "body")
	require.Error(t, err)

	require.Equal(t, `"quote\" slash\\ newline\n tab\t carriage\r"`, yamlQuote("quote\" slash\\ newline\n tab\t carriage\r"))
}

func TestMigrationOutputCaptureAndWriteStagedErrorBranches(t *testing.T) {
	t.Parallel()

	plan := &Plan{
		RunID:    "fixture",
		RepoRoot: t.TempDir(),
		Items: []Item{{
			SyncRef:               "sync[0]",
			ExpandedSourceRelPath: "legacy/file",
			ExpandedSourcePath:    "/repo/legacy/file",
			Driver:                "native",
			Result:                "planned",
			GeneratedFiles:        []GeneratedFile{{Path: "generated"}},
		}},
		GeneratedFiles: baseGeneratedFiles("fixture"),
	}
	output, err := capturePayloads(plan)
	require.NoError(t, err)
	require.Equal(t, "blocked", output.Plan.Items[0].Result)
	require.Equal(t, "migration-driver-unknown", output.Plan.Items[0].Diagnostics[0].Code)
	require.Equal(t, Summary{Syncs: 1, Planned: 0, Blocked: 1, GeneratedFiles: 5, Status: "blocked"}, output.Plan.Summary)

	err = (&Output{
		Plan: &Plan{
			RunID:    "fixture",
			RepoRoot: t.TempDir(),
			Items: []Item{{
				SyncIndex: 0,
				SyncRef:   "sync[0]",
				SettingID: "sync-0",
				Driver:    "native",
				Result:    "planned",
			}},
			GeneratedFiles: baseGeneratedFiles("fixture"),
		},
		Payloads: []Payload{{SyncRef: "sync[0]", Driver: "native"}},
	}).writeStaged(t.TempDir())
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported payload driver")
}

func TestDryRunPlanErrorAndHelperBranches(t *testing.T) {
	t.Parallel()

	_, err := BuildDryRunPlan(Options{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "config path is required")

	_, err = BuildDryRunPlanFromConfig(nil, filepath.Join(t.TempDir(), config.DefaultConfigFile), Options{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "v1 config is required")

	payload := NewErrorPayload(false, nil, "DFM_FLAG_UNSUPPORTED", "migrate currently supports --dry-run only", map[string]any{"flag": "--dry-run"})
	rendered, err := ErrorJSON(payload)
	require.NoError(t, err)
	require.Contains(t, rendered, `"schemaVersion": 1`)
	require.Contains(t, rendered, `"dryRun": false`)
	require.Contains(t, rendered, `"code": "DFM_FLAG_UNSUPPORTED"`)

	require.Equal(t, "summary syncs=0 planned=0 blocked=0 generated-files=0 status=error", Text(nil))
	require.Nil(t, SortedGeneratedFiles(nil))

	plan := &Plan{
		GeneratedFiles: []GeneratedFile{{Path: "b"}, {Path: "a"}},
		Items:          []Item{{GeneratedFiles: []GeneratedFile{{Path: "d"}, {Path: "c"}}}},
	}
	sorted := SortedGeneratedFiles(plan)
	require.Equal(t, []string{"a", "b", "c", "d"}, []string{sorted[0].Path, sorted[1].Path, sorted[2].Path, sorted[3].Path})
}

func TestTextAndJSONUseV2StyleMigrationPreview(t *testing.T) {
	t.Parallel()

	plan := &Plan{
		Schema:        Schema,
		SchemaVersion: SchemaVersion,
		Command:       Command,
		RunID:         "dry-run",
		DryRun:        true,
		ConfigPath:    "/repo/.dotfiles-manager.yaml",
		Items: []Item{{
			SyncIndex:          0,
			SyncRef:            "sync[0]",
			LegacySource:       "source/file",
			LegacyTarget:       ".target-file",
			ExpandedSourcePath: "/repo/source/file",
			ExpandedTargetPath: "/home/leon/.target-file",
			SettingRef:         "custom.files:sync-0",
			Driver:             recipe.FileDriverID,
			DesiredArtifactBinding: DesiredArtifactBinding{
				URI: "desired://user/legacy/targets/custom.files/artifacts/sync-0",
			},
			GeneratedFiles:    []GeneratedFile{{Path: "migrations/v1-to-v2/dry-run/generated/desired/user/legacy/targets/custom.files/artifacts/sync-0"}},
			Result:            "planned",
			V1ConfigAction:    "leave-unchanged",
			BehaviorUnchanged: true,
		}},
		GeneratedFiles: baseGeneratedFiles("dry-run"),
		Summary:        Summary{Syncs: 1, Planned: 1, GeneratedFiles: 6, Status: "ok"},
	}

	text := Text(plan)
	require.Contains(t, text, "MODE: DRY RUN (no writes)")
	require.Contains(t, text, "legacy source: source/file")
	require.Contains(t, text, "proposed: custom.files:sync-0 driver=file")
	require.Contains(t, text, "artifact binding: desired://user/legacy/targets/custom.files/artifacts/sync-0")

	jsonPayload, err := JSON(plan)
	require.NoError(t, err)
	require.Contains(t, jsonPayload, `"schemaVersion": 1`)
	require.Contains(t, jsonPayload, `"dryRun": true`)
	require.NotContains(t, jsonPayload, "schema_version")
	require.NotContains(t, jsonPayload, "dry_run")
}

func writeV1Config(t *testing.T, root string, body string) string {
	t.Helper()
	path := filepath.Join(root, config.DefaultConfigFile)
	writeFile(t, path, body)
	return path
}

func writeSingleItemV2Fixture(t *testing.T, root string, item Item, desiredBody string) {
	t.Helper()
	writeFile(t, filepath.Join(root, "dotfiles-manager.v2.yaml"), `schema: dotfiles-manager.v2.root-config
schemaVersion: 1
activeProfileStack: legacy
`)
	writeFile(t, filepath.Join(root, "profiles", "stacks", "legacy.yaml"), `schema: dotfiles-manager.v2.profile-stack
schemaVersion: 1
profileStack:
  - legacy
`)
	writeFile(t, filepath.Join(root, "profiles", "layers", "legacy.yaml"), `schema: dotfiles-manager.v2.profile-layer
schemaVersion: 1
selections:
  custom.files:
    settings:
      `+item.SettingID+`:
        scope: user
        artifact: `+item.DesiredArtifactBinding.Artifact+`
`)
	writeFile(t, filepath.Join(root, "recipes", "local", "custom.files", "recipe.yaml"), `schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: custom.files
displayName: Migrated custom files
supportLevel: experimental
capability: read-write
locations:
  `+item.LocationID+`:
    default: "`+item.LocationDefault+`"
settings:
  `+item.SettingID+`:
    scopeDefault: user
    resource: `+item.ResourceID+`
resources:
  `+item.ResourceID+`:
    driver: `+item.Driver+`
    location: `+item.LocationID+`
    path: `+item.ResourcePath+`
`)
	writeFile(t, filepath.Join(root, filepath.FromSlash(item.DesiredArtifactBinding.RelPath)), desiredBody)
}

func writeFile(t *testing.T, path string, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func requireFile(t *testing.T, path string, want string) {
	t.Helper()
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, want, string(body))
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	return body
}

func fixedTimeForRunID() time.Time {
	return time.Date(2026, 1, 2, 3, 4, 5, 6, time.UTC)
}

func writeOneFileMigrationRepo(t *testing.T) string {
	t.Helper()
	repoRoot := t.TempDir()
	writeFile(t, filepath.Join(repoRoot, "legacy", ".gitconfig"), "[user]\n\temail = leon@example.com\n")
	return writeV1Config(t, repoRoot, `syncs:
  - source: legacy/.gitconfig
    target: .gitconfig
`)
}
