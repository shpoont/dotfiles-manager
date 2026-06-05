package customfiles

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/shpoont/dotfiles-manager/internal/v2/filedriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/shpoont/dotfiles-manager/internal/v2/resolution"
	"github.com/stretchr/testify/require"
)

func TestSaveDryRunAndRealSaveCreateUpdateDeleteDesiredArtifact(t *testing.T) {
	t.Parallel()

	root, liveRoot, profile, rec := setupCustomFilesFixture(t)
	livePath := filepath.Join(liveRoot, "config.txt")
	desiredPath := desiredArtifactPath(root)
	req := Request{Profile: profile, Recipe: rec, SettingRef: "custom.files:file", LocationRoots: map[string]string{"config": liveRoot}}

	writeFile(t, livePath, "live v1\n")
	plan, err := PlanSave(req)
	require.NoError(t, err)
	require.Equal(t, filedriver.ChangeCreate, plan.Preview.Change.Kind)

	dry, err := Execute(plan, ExecuteOptions{DryRun: true})
	require.NoError(t, err)
	require.True(t, dry.DryRun)
	require.False(t, dry.Mutated)
	assertMissing(t, desiredPath)
	requireFile(t, livePath, "live v1\n")

	result, err := Execute(plan, ExecuteOptions{})
	require.NoError(t, err)
	require.True(t, result.Mutated)
	require.True(t, result.Verified)
	requireFile(t, desiredPath, "live v1\n")

	writeFile(t, livePath, "live v2\n")
	plan, err = PlanSave(req)
	require.NoError(t, err)
	require.Equal(t, filedriver.ChangeUpdate, plan.Preview.Change.Kind)
	result, err = Execute(plan, ExecuteOptions{})
	require.NoError(t, err)
	require.True(t, result.Mutated)
	require.True(t, result.Verified)
	requireFile(t, desiredPath, "live v2\n")

	require.NoError(t, os.Remove(livePath))
	plan, err = PlanSave(req)
	require.NoError(t, err)
	require.Equal(t, filedriver.ChangeDelete, plan.Preview.Change.Kind)
	dry, err = Execute(plan, ExecuteOptions{DryRun: true})
	require.NoError(t, err)
	require.False(t, dry.Mutated)
	requireFile(t, desiredPath, "live v2\n")

	result, err = Execute(plan, ExecuteOptions{})
	require.NoError(t, err)
	require.True(t, result.Mutated)
	require.True(t, result.Verified)
	assertMissing(t, desiredPath)
}

func TestApplyDryRunAndRealApplyCreateUpdateDeleteWithBackupBeforeMutation(t *testing.T) {
	t.Parallel()

	root, liveRoot, profile, rec := setupCustomFilesFixture(t)
	livePath := filepath.Join(liveRoot, "config.txt")
	desiredPath := desiredArtifactPath(root)
	req := Request{Profile: profile, Recipe: rec, SettingRef: "custom.files:file", LocationRoots: map[string]string{"config": liveRoot}}

	backupCalls := 0
	backupHook := func(label string, wantBeforeExists bool, wantBeforeBody string) BackupHook {
		return func(req BackupRequest) (BackupResult, error) {
			backupCalls++
			require.Equal(t, OperationApply, req.Operation)
			require.Equal(t, "custom.files:file", req.SettingRef)
			require.Equal(t, "config-file", req.ResourceID)
			require.Equal(t, livePath, req.Path)
			require.Equal(t, wantBeforeExists, req.Before.Exists, label)
			if wantBeforeExists {
				require.Equal(t, wantBeforeBody, string(req.Before.Bytes), label)
				requireFile(t, livePath, wantBeforeBody)
			} else {
				assertMissing(t, livePath)
			}
			return BackupResult{ID: fmt.Sprintf("memory://backup/%s", label), Before: req.Before.Snapshot()}, nil
		}
	}

	writeFile(t, desiredPath, "desired v1\n")
	plan, err := PlanApply(req)
	require.NoError(t, err)
	require.Equal(t, filedriver.ChangeCreate, plan.Preview.Change.Kind)

	dry, err := Execute(plan, ExecuteOptions{DryRun: true, BackupHook: backupHook("dry", false, "")})
	require.NoError(t, err)
	require.True(t, dry.DryRun)
	require.False(t, dry.Mutated)
	require.Zero(t, backupCalls)
	assertMissing(t, livePath)

	result, err := Execute(plan, ExecuteOptions{BackupHook: backupHook("create", false, "")})
	require.NoError(t, err)
	require.True(t, result.Mutated)
	require.True(t, result.Verified)
	require.NotNil(t, result.Backup)
	require.Equal(t, "memory://backup/create", result.Backup.ID)
	requireFile(t, livePath, "desired v1\n")
	require.Equal(t, 1, backupCalls)

	writeFile(t, desiredPath, "desired v2\n")
	plan, err = PlanApply(req)
	require.NoError(t, err)
	require.Equal(t, filedriver.ChangeUpdate, plan.Preview.Change.Kind)
	result, err = Execute(plan, ExecuteOptions{BackupHook: backupHook("update", true, "desired v1\n")})
	require.NoError(t, err)
	require.True(t, result.Mutated)
	require.True(t, result.Verified)
	require.Equal(t, "memory://backup/update", result.Backup.ID)
	requireFile(t, livePath, "desired v2\n")
	require.Equal(t, 2, backupCalls)

	require.NoError(t, os.Remove(desiredPath))
	plan, err = PlanApply(req)
	require.NoError(t, err)
	require.Equal(t, filedriver.ChangeDelete, plan.Preview.Change.Kind)
	result, err = Execute(plan, ExecuteOptions{BackupHook: backupHook("delete", true, "desired v2\n")})
	require.NoError(t, err)
	require.True(t, result.Mutated)
	require.True(t, result.Verified)
	require.Equal(t, "memory://backup/delete", result.Backup.ID)
	assertMissing(t, livePath)
	require.Equal(t, 3, backupCalls)
}

func TestApplyRecomputesBackupWhenPlanBecomesStale(t *testing.T) {
	t.Parallel()

	root, liveRoot, profile, rec := setupCustomFilesFixture(t)
	livePath := filepath.Join(liveRoot, "config.txt")
	desiredPath := desiredArtifactPath(root)
	req := Request{Profile: profile, Recipe: rec, SettingRef: "custom.files:file", LocationRoots: map[string]string{"config": liveRoot}}

	writeFile(t, desiredPath, "desired\n")
	writeFile(t, livePath, "desired\n")
	plan, err := PlanApply(req)
	require.NoError(t, err)
	require.Equal(t, filedriver.ChangeUnchanged, plan.Preview.Change.Kind)

	writeFile(t, livePath, "drift\n")
	backupCalled := false
	result, err := Execute(plan, ExecuteOptions{BackupHook: func(req BackupRequest) (BackupResult, error) {
		backupCalled = true
		require.Equal(t, "drift\n", string(req.Before.Bytes))
		requireFile(t, livePath, "drift\n")
		return BackupResult{ID: "memory://backup/stale-plan", Before: req.Before.Snapshot()}, nil
	}})
	require.NoError(t, err)
	require.True(t, backupCalled)
	require.True(t, result.Mutated)
	require.True(t, result.Verified)
	require.NotNil(t, result.Backup)
	require.Equal(t, "memory://backup/stale-plan", result.Backup.ID)
	requireFile(t, livePath, "desired\n")
}

func TestCustomFilesRejectsUnsafeDesiredArtifactSymlinkTraversal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		symlinkDst func(t *testing.T, root string) string
	}{
		{
			name: "outside repo",
			symlinkDst: func(t *testing.T, root string) string {
				t.Helper()
				return t.TempDir()
			},
		},
		{
			name: "elsewhere inside repo",
			symlinkDst: func(t *testing.T, root string) string {
				t.Helper()
				path := filepath.Join(root, "elsewhere-inside-repo")
				require.NoError(t, os.MkdirAll(path, 0o755))
				return path
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root, liveRoot, profile, rec := setupCustomFilesFixture(t)
			writeFile(t, filepath.Join(liveRoot, "config.txt"), "live\n")
			targetDir := filepath.Join(root, "desired", "user", "leon", "targets", "custom.files")
			require.NoError(t, os.MkdirAll(targetDir, 0o755))
			require.NoError(t, os.Symlink(tc.symlinkDst(t, root), filepath.Join(targetDir, "artifacts")))

			_, err := PlanSave(Request{Profile: profile, Recipe: rec, SettingRef: "custom.files:file", LocationRoots: map[string]string{"config": liveRoot}})
			require.Error(t, err)
			require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())
		})
	}
}

func TestCustomFilesRejectsUnknownSettingRef(t *testing.T) {
	t.Parallel()

	_, liveRoot, profile, rec := setupCustomFilesFixture(t)
	_, err := PlanSave(Request{Profile: profile, Recipe: rec, SettingRef: "custom.files:missing", LocationRoots: map[string]string{"config": liveRoot}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "no setting")
}

func setupCustomFilesFixture(t *testing.T) (string, string, *resolution.ResolvedProfile, *recipe.Recipe) {
	t.Helper()

	root := t.TempDir()
	liveRoot := filepath.Join(t.TempDir(), "cobona")
	require.NoError(t, os.MkdirAll(liveRoot, 0o755))
	writeV2Root(t, root)
	writeStack(t, root)
	writeLayer(t, root)
	writeRecipe(t, root)

	profile, err := resolution.Resolve(root, resolution.ResolveOptions{UserID: "leon", MachineID: "mbp"})
	require.NoError(t, err)
	rec, err := recipe.LoadCustomFiles(root)
	require.NoError(t, err)
	return root, liveRoot, profile, rec
}

func writeV2Root(t *testing.T, root string) {
	t.Helper()
	writeFile(t, filepath.Join(root, resolution.RootConfigFile), "schema: dotfiles-manager.v2.root-config\nschemaVersion: 1\nactiveProfileStack: default\n")
}

func writeStack(t *testing.T, root string) {
	t.Helper()
	writeFile(t, filepath.Join(root, "profiles", "stacks", "default.yaml"), "schema: dotfiles-manager.v2.profile-stack\nschemaVersion: 1\nprofileStack:\n  - global\n")
}

func writeLayer(t *testing.T, root string) {
	t.Helper()
	writeFile(t, filepath.Join(root, "profiles", "layers", "global.yaml"), `schema: dotfiles-manager.v2.profile-layer
schemaVersion: 1
selections:
  custom.files:
    settings:
      file:
        scope: user
        artifact: artifacts/config.txt
`)
}

func writeRecipe(t *testing.T, root string) {
	t.Helper()
	writeFile(t, filepath.Join(root, "recipes", "local", "custom.files", "recipe.yaml"), `schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: custom.files
displayName: Custom files
supportLevel: experimental
capability: read-write
locations:
  config:
    default: ~/.cobona
settings:
  file:
    scopeDefault: user
    resource: config-file
resources:
  config-file:
    driver: file
    location: config
    path: config.txt
`)
}

func desiredArtifactPath(root string) string {
	return filepath.Join(root, "desired", "user", "leon", "targets", "custom.files", "artifacts", "config.txt")
}

func writeFile(t *testing.T, path string, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func requireFile(t *testing.T, path string, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, want, string(got))
}

func assertMissing(t *testing.T, path string) {
	t.Helper()
	_, err := os.Stat(path)
	require.True(t, os.IsNotExist(err), "expected %s to be missing, got %v", path, err)
}

func TestCustomFilesConvenienceAndErrorBranches(t *testing.T) {
	t.Parallel()

	root, liveRoot, profile, rec := setupCustomFilesFixture(t)
	livePath := filepath.Join(liveRoot, "config.txt")
	desiredPath := desiredArtifactPath(root)
	req := Request{Profile: profile, Recipe: rec, SettingRef: "custom.files:file", LocationRoots: map[string]string{"config": liveRoot}}

	writeFile(t, livePath, "saved\n")
	saved, err := Save(req, ExecuteOptions{})
	require.NoError(t, err)
	require.True(t, saved.Verified)
	requireFile(t, desiredPath, "saved\n")

	writeFile(t, desiredPath, "applied\n")
	applied, err := Apply(req, ExecuteOptions{})
	require.NoError(t, err)
	require.True(t, applied.Verified)
	requireFile(t, livePath, "applied\n")

	_, err = Execute(nil, ExecuteOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "plan is required")

	restoreCalled := false
	err = RestoreWithHook(RestoreRequest{SettingRef: "custom.files:file"}, func(req RestoreRequest) error {
		restoreCalled = true
		require.Equal(t, "custom.files:file", req.SettingRef)
		return nil
	})
	require.NoError(t, err)
	require.True(t, restoreCalled)
	err = RestoreWithHook(RestoreRequest{}, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "restore hook is required")

	_, err = PlanSave(Request{Recipe: rec, SettingRef: "custom.files:file", LocationRoots: map[string]string{"config": liveRoot}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "resolved profile is required")
	_, err = PlanSave(Request{Profile: profile, SettingRef: "custom.files:file", LocationRoots: map[string]string{"config": liveRoot}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "recipe is required")
	_, err = PlanSave(Request{Profile: profile, Recipe: rec, SettingRef: "not-a-ref", LocationRoots: map[string]string{"config": liveRoot}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "target:setting")
}

func TestCustomFilesAmbiguousAndInvalidDesiredArtifacts(t *testing.T) {
	t.Parallel()

	_, liveRoot, profile, rec := setupCustomFilesFixture(t)
	plan, err := PlanSave(Request{Profile: profile, Recipe: rec, LocationRoots: map[string]string{"config": liveRoot}})
	require.NoError(t, err)
	require.Equal(t, "custom.files:file", plan.Setting.Ref())

	multi := *profile
	multi.Settings = append(append([]resolution.ResolvedSetting{}, profile.Settings...), resolution.ResolvedSetting{TargetID: "custom.files", SettingID: "other"})
	_, err = PlanSave(Request{Profile: &multi, Recipe: rec, LocationRoots: map[string]string{"config": liveRoot}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "setting ref is required")

	settingsArtifact := *profile
	settingsArtifact.Settings = append([]resolution.ResolvedSetting{}, profile.Settings...)
	settingsArtifact.Settings[0].DesiredRelPath = filepath.Join("desired", "user", "leon", "targets", "custom.files", "settings.yaml")
	_, err = PlanSave(Request{Profile: &settingsArtifact, Recipe: rec, SettingRef: "custom.files:file", LocationRoots: map[string]string{"config": liveRoot}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "artifacts")
}

func TestCustomFilesAdditionalErrorBranches(t *testing.T) {
	t.Parallel()

	root, liveRoot, profile, rec := setupCustomFilesFixture(t)
	writeFile(t, filepath.Join(liveRoot, "config.txt"), "live\n")
	req := Request{Profile: profile, Recipe: rec, SettingRef: "custom.files:file", LocationRoots: map[string]string{"config": liveRoot}}

	_, err := Save(Request{}, ExecuteOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "resolved profile")
	_, err = Apply(Request{}, ExecuteOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "resolved profile")

	badRecipe := *rec
	badRecipe.Capability = "read-only"
	_, err = PlanSave(Request{Profile: profile, Recipe: &badRecipe, SettingRef: "custom.files:file", LocationRoots: map[string]string{"config": liveRoot}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "read-write")

	missingSettingRecipe := *rec
	missingSettingRecipe.Settings = map[string]recipe.Setting{"other": {ScopeDefault: "user", Resource: "config-file"}}
	_, err = PlanSave(Request{Profile: profile, Recipe: &missingSettingRecipe, SettingRef: "custom.files:file", LocationRoots: map[string]string{"config": liveRoot}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "no setting")

	_, err = PlanSave(Request{Profile: profile, Recipe: rec, SettingRef: "custom.files:file", LocationRoots: map[string]string{"config": filepath.Join(root, "missing-live-root")}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "read live")

	otherProfile := *profile
	otherProfile.Settings = append([]resolution.ResolvedSetting{}, profile.Settings...)
	otherProfile.Settings = append(otherProfile.Settings, resolution.ResolvedSetting{TargetID: "other.files", SettingID: "file"})
	_, err = PlanSave(Request{Profile: &otherProfile, Recipe: rec, SettingRef: "other.files:file", LocationRoots: map[string]string{"config": liveRoot}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not managed by recipe")

	_, err = buildPlan(req, Operation("bad"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported")
}
