package customfiles

import (
	"errors"
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

func TestCustomFilesFileTreeSaveAndApplyArtifactDirectory(t *testing.T) {
	t.Parallel()

	root, liveRoot, profile, rec := setupCustomFileTreeFixture(t)
	liveTree := filepath.Join(liveRoot, "profiles")
	desiredTree := desiredTreeArtifactPath(root)
	req := Request{Profile: profile, Recipe: rec, SettingRef: "custom.files:file", LocationRoots: map[string]string{"config": liveRoot}}

	writeFile(t, filepath.Join(liveTree, "config.yaml"), "live v1\n")
	writeFile(t, filepath.Join(liveTree, "cache", "ignored.yaml"), "ignored\n")
	require.NoError(t, os.MkdirAll(filepath.Join(liveTree, "empty"), 0o755))

	plan, err := PlanSave(req)
	require.NoError(t, err)
	require.Equal(t, filedriver.ChangeCreate, plan.TreePreview.Change.Kind)
	dry, err := Execute(plan, ExecuteOptions{DryRun: true})
	require.NoError(t, err)
	require.True(t, dry.DryRun)
	require.False(t, dry.Mutated)
	assertMissing(t, desiredTree)

	result, err := Execute(plan, ExecuteOptions{})
	require.NoError(t, err)
	require.True(t, result.Mutated)
	require.True(t, result.Verified)
	requireFile(t, filepath.Join(desiredTree, "config.yaml"), "live v1\n")
	assertMissing(t, filepath.Join(desiredTree, "cache", "ignored.yaml"))
	require.DirExists(t, filepath.Join(desiredTree, "empty"))

	require.NoError(t, os.RemoveAll(liveTree))
	writeFile(t, filepath.Join(desiredTree, "config.yaml"), "desired v2\n")
	plan, err = PlanApply(req)
	require.NoError(t, err)
	require.Equal(t, filedriver.ChangeCreate, plan.TreePreview.Change.Kind)

	backupCalls := 0
	result, err = Execute(plan, ExecuteOptions{BackupHook: func(req BackupRequest) (BackupResult, error) {
		backupCalls++
		require.Equal(t, OperationApply, req.Operation)
		require.Equal(t, "custom.files:file", req.SettingRef)
		require.Equal(t, "config-file", req.ResourceID)
		require.Equal(t, liveTree, req.Path)
		require.False(t, req.TreeBefore.Exists)
		assertMissing(t, liveTree)
		return BackupResult{ID: "memory://backup/tree-create", TreeBefore: req.TreeBefore.Snapshot()}, nil
	}})
	require.NoError(t, err)
	require.True(t, result.Mutated)
	require.True(t, result.Verified)
	require.Equal(t, 1, backupCalls)
	require.NotNil(t, result.Backup)
	require.Equal(t, "memory://backup/tree-create", result.Backup.ID)
	requireFile(t, filepath.Join(liveTree, "config.yaml"), "desired v2\n")
}

func TestCustomFilesFileTreeApplyRejectsStaleDesiredSymlinkBeforeMutation(t *testing.T) {
	t.Parallel()

	root, liveRoot, profile, rec := setupCustomFileTreeFixture(t)
	liveTree := filepath.Join(liveRoot, "profiles")
	desiredTree := desiredTreeArtifactPath(root)
	req := Request{Profile: profile, Recipe: rec, SettingRef: "custom.files:file", LocationRoots: map[string]string{"config": liveRoot}}

	writeFile(t, filepath.Join(liveTree, "config.yaml"), "live\n")
	writeFile(t, filepath.Join(desiredTree, "config.yaml"), "desired\n")
	plan, err := PlanApply(req)
	require.NoError(t, err)
	require.Equal(t, filedriver.ChangeUpdate, plan.TreePreview.Change.Kind)

	require.NoError(t, os.Remove(filepath.Join(desiredTree, "config.yaml")))
	require.NoError(t, os.Symlink(filepath.Join(liveTree, "config.yaml"), filepath.Join(desiredTree, "config.yaml")))
	backupCalled := false
	_, err = Execute(plan, ExecuteOptions{BackupHook: func(req BackupRequest) (BackupResult, error) {
		backupCalled = true
		return BackupResult{}, nil
	}})
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())
	require.False(t, backupCalled)
	requireFile(t, filepath.Join(liveTree, "config.yaml"), "live\n")
}

func TestCustomFilesFileTreeErrorBranches(t *testing.T) {
	t.Parallel()

	root, liveRoot, profile, rec := setupCustomFileTreeFixture(t)
	liveTree := filepath.Join(liveRoot, "profiles")
	desiredTree := desiredTreeArtifactPath(root)
	req := Request{Profile: profile, Recipe: rec, SettingRef: "custom.files:file", LocationRoots: map[string]string{"config": liveRoot}}

	writeFile(t, filepath.Join(liveTree, "config.yaml"), "live\n")
	writeFile(t, filepath.Join(desiredTree, "config.yaml"), "desired\n")
	plan, err := PlanApply(req)
	require.NoError(t, err)
	_, err = Execute(plan, ExecuteOptions{BackupHook: func(req BackupRequest) (BackupResult, error) {
		return BackupResult{}, fmt.Errorf("tree backup failed")
	}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "tree backup failed")
	requireFile(t, filepath.Join(liveTree, "config.yaml"), "live\n")

	settingsArtifact := *profile
	settingsArtifact.Settings = append([]resolution.ResolvedSetting{}, profile.Settings...)
	settingsArtifact.Settings[0].DesiredRelPath = filepath.Join("desired", "user", "leon", "targets", "custom.files", "settings.yaml")
	_, err = PlanSave(Request{Profile: &settingsArtifact, Recipe: rec, SettingRef: "custom.files:file", LocationRoots: map[string]string{"config": liveRoot}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "artifacts")

	invalidArtifact := *profile
	invalidArtifact.Settings = append([]resolution.ResolvedSetting{}, profile.Settings...)
	invalidArtifact.Settings[0].DesiredRelPath = "desired/user/leon/targets/custom.files/artifacts/../bad"
	_, err = PlanSave(Request{Profile: &invalidArtifact, Recipe: rec, SettingRef: "custom.files:file", LocationRoots: map[string]string{"config": liveRoot}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "desired artifact path")
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

func setupCustomFileTreeFixture(t *testing.T) (string, string, *resolution.ResolvedProfile, *recipe.Recipe) {
	t.Helper()

	root := t.TempDir()
	liveRoot := filepath.Join(t.TempDir(), "cobona")
	require.NoError(t, os.MkdirAll(liveRoot, 0o755))
	writeV2Root(t, root)
	writeStack(t, root)
	writeTreeLayer(t, root)
	writeTreeRecipe(t, root)

	profile, err := resolution.Resolve(root, resolution.ResolveOptions{UserID: "leon", MachineID: "mbp"})
	require.NoError(t, err)
	rec, err := recipe.LoadCustomFiles(root)
	require.NoError(t, err)
	return root, liveRoot, profile, rec
}

func setupGenericFileResourceFixture(t *testing.T) (string, string, *resolution.ResolvedProfile, *recipe.Recipe) {
	t.Helper()

	root := t.TempDir()
	liveRoot := filepath.Join(t.TempDir(), "test-files")
	require.NoError(t, os.MkdirAll(liveRoot, 0o755))
	writeV2Root(t, root)
	writeStack(t, root)
	writeFile(t, filepath.Join(root, "profiles", "layers", "global.yaml"), `schema: dotfiles-manager.v2.profile-layer
schemaVersion: 1
selections:
  test.files:
    settings:
      config:
        scope: user
`)
	writeFile(t, filepath.Join(root, "recipes", "local", "test.files", "recipe.yaml"), `schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: test.files
displayName: Test files
supportLevel: experimental
capability: read-write
locations:
  config:
    default: `+liveRoot+`
settings:
  config:
    label: Config file
    supportLevel: experimental
    capability: read-write
    artifactForm: file
    sensitivity: personal
    redaction: redacted-for-display
    lifecycle: allowed
    scopeDefault: user
    resource: config-file
resources:
  config-file:
    driver: file
    location: config
    path: config.txt
    capability: read-write
    sensitivity: personal
    redaction: redacted-for-display
    lifecycle: allowed
`)
	profile, err := resolution.Resolve(root, resolution.ResolveOptions{UserID: "leon", MachineID: "mbp"})
	require.NoError(t, err)
	rec, err := recipe.LoadLocal(root, "test.files")
	require.NoError(t, err)
	return root, liveRoot, profile, rec
}

func setupGenericFileTreeResourceFixture(t *testing.T) (string, string, *resolution.ResolvedProfile, *recipe.Recipe) {
	t.Helper()

	root := t.TempDir()
	liveRoot := filepath.Join(t.TempDir(), "test-files")
	require.NoError(t, os.MkdirAll(liveRoot, 0o755))
	writeV2Root(t, root)
	writeStack(t, root)
	writeFile(t, filepath.Join(root, "profiles", "layers", "global.yaml"), `schema: dotfiles-manager.v2.profile-layer
schemaVersion: 1
selections:
  test.files:
    settings:
      config:
        scope: user
`)
	writeFile(t, filepath.Join(root, "recipes", "local", "test.files", "recipe.yaml"), `schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: test.files
displayName: Test files
supportLevel: experimental
capability: read-write
locations:
  config:
    default: `+liveRoot+`
settings:
  config:
    label: Config tree
    supportLevel: experimental
    capability: read-write
    artifactForm: file-tree
    sensitivity: personal
    redaction: redacted-for-display
    lifecycle: allowed
    scopeDefault: user
    resource: config-tree
resources:
  config-tree:
    driver: file-tree
    location: config
    path: nvim
    capability: read-write
    sensitivity: personal
    redaction: redacted-for-display
    lifecycle: allowed
    include:
      - "**"
    exclude:
      - "cache/**"
`)
	profile, err := resolution.Resolve(root, resolution.ResolveOptions{UserID: "leon", MachineID: "mbp"})
	require.NoError(t, err)
	rec, err := recipe.LoadLocal(root, "test.files")
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

func writeTreeLayer(t *testing.T, root string) {
	t.Helper()
	writeFile(t, filepath.Join(root, "profiles", "layers", "global.yaml"), `schema: dotfiles-manager.v2.profile-layer
schemaVersion: 1
selections:
  custom.files:
    settings:
      file:
        scope: user
        artifact: artifacts/profiles
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

func writeTreeRecipe(t *testing.T, root string) {
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
    driver: file-tree
    location: config
    path: profiles
    include:
      - "**"
    exclude:
      - "cache/**"
`)
}

func desiredArtifactPath(root string) string {
	return filepath.Join(root, "desired", "user", "leon", "targets", "custom.files", "artifacts", "config.txt")
}

func desiredTreeArtifactPath(root string) string {
	return filepath.Join(root, "desired", "user", "leon", "targets", "custom.files", "artifacts", "profiles")
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

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	return got
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

func TestFileResourcePlanUsesConventionalArtifactAndBlocksDeletes(t *testing.T) {
	t.Parallel()

	root, liveRoot, profile, rec := setupGenericFileResourceFixture(t)
	livePath := filepath.Join(liveRoot, "config.txt")
	desiredPath := filepath.Join(root, "desired", "user", "leon", "targets", "test.files", "artifacts", "config")
	req := Request{Profile: profile, Recipe: rec, SettingRef: "test.files:config", LocationRoots: map[string]string{"config": liveRoot}}

	writeFile(t, livePath, "live\n")
	readPlan, err := PlanFileRead(req)
	require.NoError(t, err)
	require.Equal(t, Operation(""), readPlan.Operation)
	require.Equal(t, filedriver.ChangeDelete, readPlan.Preview.Change.Kind)
	require.Equal(t, "desired://user/leon/targets/test.files/artifacts/config", readPlan.Setting.DesiredURI)

	savePlan, err := PlanFileSave(req)
	require.NoError(t, err)
	require.Equal(t, filepath.ToSlash(filepath.Join("desired", "user", "leon", "targets", "test.files", "artifacts", "config")), filepath.ToSlash(savePlan.DesiredRelPath))
	require.Equal(t, "desired://user/leon/targets/test.files/artifacts/config", savePlan.Setting.DesiredURI)
	require.Equal(t, filedriver.ChangeCreate, savePlan.Preview.Change.Kind)

	writeFile(t, desiredPath, "desired\n")
	require.NoError(t, os.Remove(livePath))
	_, err = PlanFileSave(req)
	require.Error(t, err)
	require.Contains(t, err.Error(), "live file is missing")
	require.Equal(t, "desired\n", string(mustReadFile(t, desiredPath)))

	writeFile(t, livePath, "live\n")
	require.NoError(t, os.Remove(desiredPath))
	_, err = PlanFileApply(req)
	require.Error(t, err)
	require.Contains(t, err.Error(), "desired artifact is missing")
	require.Equal(t, "live\n", string(mustReadFile(t, livePath)))
}

func TestFileTreeResourcePlanUsesConventionalArtifactAndMissingPolicy(t *testing.T) {
	t.Parallel()

	root, liveRoot, profile, rec := setupGenericFileTreeResourceFixture(t)
	liveTree := filepath.Join(liveRoot, "nvim")
	desiredTree := filepath.Join(root, "desired", "user", "leon", "targets", "test.files", "artifacts", "config")
	req := Request{Profile: profile, Recipe: rec, SettingRef: "test.files:config", LocationRoots: map[string]string{"config": liveRoot}}

	writeFile(t, filepath.Join(liveTree, "init.lua"), "live\n")
	writeFile(t, filepath.Join(liveTree, "cache", "ignored.lua"), "ignored\n")
	readPlan, err := PlanFileRead(req)
	require.NoError(t, err)
	require.Equal(t, Operation(""), readPlan.Operation)
	require.Equal(t, filedriver.ChangeDelete, readPlan.TreePreview.Change.Kind)
	require.Equal(t, "desired://user/leon/targets/test.files/artifacts/config", readPlan.Setting.DesiredURI)

	savePlan, err := PlanFileSave(req)
	require.NoError(t, err)
	require.Equal(t, filepath.ToSlash(filepath.Join("desired", "user", "leon", "targets", "test.files", "artifacts", "config")), filepath.ToSlash(savePlan.DesiredRelPath))
	require.Equal(t, "desired://user/leon/targets/test.files/artifacts/config", savePlan.Setting.DesiredURI)
	require.Equal(t, filedriver.ChangeCreate, savePlan.TreePreview.Change.Kind)
	dry, err := Execute(savePlan, ExecuteOptions{DryRun: true})
	require.NoError(t, err)
	require.True(t, dry.DryRun)
	assertMissing(t, desiredTree)
	result, err := Execute(savePlan, ExecuteOptions{})
	require.NoError(t, err)
	require.True(t, result.Verified)
	requireFile(t, filepath.Join(desiredTree, "init.lua"), "live\n")
	assertMissing(t, filepath.Join(desiredTree, "cache", "ignored.lua"))

	require.NoError(t, os.RemoveAll(liveTree))
	writeFile(t, filepath.Join(desiredTree, "init.lua"), "desired\n")
	_, err = PlanFileSave(req)
	require.Error(t, err)
	require.Contains(t, err.Error(), "live tree is missing")
	requireFile(t, filepath.Join(desiredTree, "init.lua"), "desired\n")

	applyPlan, err := PlanFileApply(req)
	require.NoError(t, err)
	require.Equal(t, filedriver.ChangeCreate, applyPlan.TreePreview.Change.Kind)
	backupCalls := 0
	result, err = Execute(applyPlan, ExecuteOptions{BackupHook: func(req BackupRequest) (BackupResult, error) {
		backupCalls++
		require.False(t, req.TreeBefore.Exists)
		return BackupResult{ID: "memory://tree-backup", TreeBefore: req.TreeBefore.Snapshot()}, nil
	}})
	require.NoError(t, err)
	require.True(t, result.Verified)
	require.Equal(t, 1, backupCalls)
	requireFile(t, filepath.Join(liveTree, "init.lua"), "desired\n")

	require.NoError(t, os.RemoveAll(desiredTree))
	_, err = PlanFileApply(req)
	require.Error(t, err)
	require.Contains(t, err.Error(), "desired artifact is missing")
	requireFile(t, filepath.Join(liveTree, "init.lua"), "desired\n")

	_, err = PlanFileRead(Request{Profile: profile, Recipe: rec, SettingRef: "test.files:config", LocationRoots: map[string]string{"config": filepath.Join(root, "missing-root")}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "location root does not exist")
}

func TestFileResourcePlanReadErrorBranches(t *testing.T) {
	t.Parallel()

	_, liveRoot, profile, rec := setupGenericFileResourceFixture(t)
	req := Request{Profile: profile, Recipe: rec, SettingRef: "test.files:config", LocationRoots: map[string]string{"config": liveRoot}}

	_, err := PlanFileRead(Request{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "resolved profile")

	_, err = PlanFileRead(Request{Profile: profile})
	require.Error(t, err)
	require.Contains(t, err.Error(), "file-resource recipe")

	fileTreeRecipe := *rec
	fileTreeRecipe.Resources = map[string]recipe.Resource{}
	for id, resource := range rec.Resources {
		resource.Driver = recipe.FileTreeDriverID
		fileTreeRecipe.Resources[id] = resource
	}
	treePlan, err := PlanFileRead(Request{Profile: profile, Recipe: &fileTreeRecipe, SettingRef: "test.files:config", LocationRoots: map[string]string{"config": liveRoot}})
	require.NoError(t, err)
	require.Equal(t, recipe.FileTreeDriverID, treePlan.Resource.Driver)
	require.Equal(t, filedriver.ChangeUnchanged, treePlan.TreePreview.Change.Kind)

	unsupportedRecipe := *rec
	unsupportedRecipe.Resources = map[string]recipe.Resource{}
	for id, resource := range rec.Resources {
		resource.Driver = recipe.YAMLFileDriverID
		resource.Selector = &recipe.Selector{Path: []string{"user", "email"}}
		unsupportedRecipe.Resources[id] = resource
	}
	_, err = PlanFileRead(Request{Profile: profile, Recipe: &unsupportedRecipe, SettingRef: "test.files:config", LocationRoots: map[string]string{"config": liveRoot}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported file-resource driver")

	missingLocationRecipe := *rec
	missingLocationRecipe.Resources = map[string]recipe.Resource{}
	for id, resource := range rec.Resources {
		resource.Location = "missing-location"
		missingLocationRecipe.Resources[id] = resource
	}
	_, err = PlanFileRead(Request{Profile: profile, Recipe: &missingLocationRecipe, SettingRef: "test.files:config", LocationRoots: map[string]string{"config": liveRoot}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "location")

	settingsArtifactProfile := *profile
	settingsArtifactProfile.Settings = append([]resolution.ResolvedSetting{}, profile.Settings...)
	settingsArtifactProfile.Settings[0].DesiredRelPath = filepath.Join("desired", "user", "leon", "targets", "test.files", "settings.json")
	_, err = PlanFileRead(Request{Profile: &settingsArtifactProfile, Recipe: rec, SettingRef: "test.files:config", LocationRoots: map[string]string{"config": liveRoot}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "artifacts")

	_, err = buildFileResourcePlan(req, Operation("bad"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported file-resource operation")
}

func TestFileResourceContentSafetyAndPlanErrorBranches(t *testing.T) {
	t.Parallel()

	require.Equal(t, "file-resource planning failed", (*PlanError)(nil).Error())
	require.Equal(t, "file-resource planning failed", (&PlanError{}).Error())
	require.Equal(t, "ssh:config[ssh.config.excluded-content]: file-resource planning failed; ssh:config[custom]: custom message", (&PlanError{Diagnostics: []Diagnostic{
		{Ref: "ssh:config", Code: recipe.SSHConfigExcludedContentCode},
		{Ref: "ssh:config", Code: "custom", Message: "custom message"},
	}}).Error())

	_, _, profile, _ := setupCustomFilesFixture(t)
	setting := profile.Settings[0]
	setting.TargetID = recipe.SSHTarget
	setting.SettingID = "config"
	resource := recipe.BundledSSHRecipe().Resources["config"]
	plan := &Plan{
		Setting:    setting,
		ResourceID: "config",
		Resource:   resource,
	}
	targetRoot := t.TempDir()
	target := filedriver.Target{LocationID: "home", Root: targetRoot, RelPath: ".ssh/config"}

	require.NoError(t, enforceFileContentSafety(nil, OperationSave, "live", filedriver.State{Exists: true, Bytes: []byte("-----BEGIN OPENSSH PRIVATE KEY-----")}, target))
	noPolicy := *plan
	noPolicy.Resource.ContentSafetyPolicy = ""
	require.NoError(t, enforceFileContentSafety(&noPolicy, OperationSave, "live", filedriver.State{Exists: true, Bytes: []byte("-----BEGIN OPENSSH PRIVATE KEY-----")}, target))
	require.NoError(t, enforceFileContentSafety(plan, OperationSave, "live", filedriver.State{Exists: false, Bytes: []byte("-----BEGIN OPENSSH PRIVATE KEY-----")}, target))
	require.NoError(t, enforceFileContentSafety(plan, OperationSave, "live", filedriver.State{Exists: true, Bytes: []byte("Host github.com\n  IdentityFile ~/.ssh/id_ed25519\n")}, target))

	err := enforceFileContentSafety(plan, OperationApply, "desired", filedriver.State{Exists: true, Bytes: []byte("Host bad\n-----BEGIN OPENSSH PRIVATE KEY-----\nraw-secret\n-----END OPENSSH PRIVATE KEY-----\n")}, target)
	require.Error(t, err)
	var planErr *PlanError
	require.True(t, errors.As(err, &planErr))
	require.Len(t, planErr.Diagnostics, 1)
	diagnostic := planErr.Diagnostics[0]
	require.Equal(t, recipe.SSHConfigExcludedContentCode, diagnostic.Code)
	require.Equal(t, "error", diagnostic.Severity)
	require.Equal(t, "ssh:config", diagnostic.Ref)
	require.Equal(t, "config", diagnostic.ResourceID)
	require.Equal(t, recipe.FileDriverID, diagnostic.DriverID)
	require.Equal(t, "private-key", diagnostic.Category)
	require.Equal(t, "private_key_header", diagnostic.PatternID)
	require.Equal(t, "apply", diagnostic.Operation)
	require.Contains(t, diagnostic.Message, "raw content omitted")
	require.NotContains(t, diagnostic.Message, "raw-secret")
	require.NotContains(t, err.Error(), "OPENSSH PRIVATE KEY")
	require.NotContains(t, err.Error(), "raw-secret")

	unresolvableTarget := filedriver.Target{LocationID: "home", Root: filepath.Join(targetRoot, "missing"), RelPath: ".ssh/config"}
	err = enforceFileContentSafety(plan, OperationSave, "live", filedriver.State{Exists: true, Bytes: []byte("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFakeFakeFakeFakeFakeFakeFakeFakeFakeFakeFake comment\n")}, unresolvableTarget)
	require.Error(t, err)
	require.True(t, errors.As(err, &planErr))
	require.Equal(t, ".ssh/config", planErr.Diagnostics[0].Path)
}

func TestFileResourceReadErrorMapsSSHLeafSymlinksMetadataOnly(t *testing.T) {
	t.Parallel()

	setting := resolution.ResolvedSetting{TargetID: recipe.SSHTarget, SettingID: "config"}
	resource := recipe.BundledSSHRecipe().Resources["config"]
	driverErr := &filedriver.Error{Code: filedriver.CodeSymlinkUnsupported, Op: "resolve", Path: "/home/leon/.ssh/config", Err: fmt.Errorf("leaf is a symlink")}

	err := fileResourceReadError(setting, "config", resource, "read live", driverErr)
	require.Error(t, err)
	var planErr *PlanError
	require.True(t, errors.As(err, &planErr))
	require.Len(t, planErr.Diagnostics, 1)
	diagnostic := planErr.Diagnostics[0]
	require.Equal(t, recipe.SSHConfigSymlinkUnsupportedCode, diagnostic.Code)
	require.Equal(t, "error", diagnostic.Severity)
	require.Equal(t, "ssh:config", diagnostic.Ref)
	require.Equal(t, "/home/leon/.ssh/config", diagnostic.Path)
	require.Equal(t, "config", diagnostic.ResourceID)
	require.Equal(t, recipe.FileDriverID, diagnostic.DriverID)
	require.NotContains(t, diagnostic.Message, "/home/leon")

	genericErr := fileResourceReadError(setting, "config", recipe.Resource{Driver: recipe.FileDriverID}, "read live", driverErr)
	require.Error(t, genericErr)
	require.Contains(t, genericErr.Error(), "read live ssh:config")
	require.True(t, filedriver.IsCode(genericErr, filedriver.CodeSymlinkUnsupported))
	require.Equal(t, "/home/leon/.ssh/config", filedriverErrorPath(driverErr))
	require.Equal(t, "", filedriverErrorPath(fmt.Errorf("plain error")))
}

func TestApplyFileContentTargetPolicyBranches(t *testing.T) {
	t.Parallel()

	applyFileContentTargetPolicy(recipe.Resource{ContentSafetyPolicy: recipe.SSHContentSafetyPolicy}, nil)

	target := filedriver.Target{}
	applyFileContentTargetPolicy(recipe.Resource{}, &target)
	require.False(t, target.RejectLeafSymlink)

	applyFileContentTargetPolicy(recipe.Resource{ContentSafetyPolicy: recipe.SSHContentSafetyPolicy}, &target)
	require.True(t, target.RejectLeafSymlink)
}

func TestCustomFilesAdditionalErrorBranches(t *testing.T) {
	t.Parallel()

	root, liveRoot, profile, rec := setupCustomFilesFixture(t)
	livePath := filepath.Join(liveRoot, "config.txt")
	desiredPath := desiredArtifactPath(root)
	writeFile(t, livePath, "live\n")
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

	writeFile(t, desiredPath, "desired hook error\n")
	plan, err := PlanApply(req)
	require.NoError(t, err)
	_, err = Execute(plan, ExecuteOptions{BackupHook: func(req BackupRequest) (BackupResult, error) {
		return BackupResult{}, fmt.Errorf("file backup failed")
	}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "file backup failed")
	requireFile(t, livePath, "live\n")

	planSave, err := PlanSave(req)
	require.NoError(t, err)
	require.NoError(t, os.Remove(desiredPath))
	require.NoError(t, os.Symlink(livePath, desiredPath))
	_, err = Execute(planSave, ExecuteOptions{})
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())
	requireFile(t, livePath, "live\n")

	err = ensureDesiredPathSafe(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "plan is required")
	err = rejectDesiredSymlinkPath("", "desired/user/leon/targets/custom.files/artifacts/config.txt")
	require.Error(t, err)
	require.Contains(t, err.Error(), "repo root")
	err = rejectDesiredSymlinkPath(root, "../bad")
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())
}
