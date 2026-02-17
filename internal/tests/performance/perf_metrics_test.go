//go:build performance

package performance_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/shpoont/dotfiles-manager/internal/app"
	"github.com/shpoont/dotfiles-manager/internal/config"
	"github.com/stretchr/testify/require"
)

const (
	perfFixtureFileCount     = 1000
	perfManagedFileCount     = 80
	statusThresholdSeconds   = 2.0
	deployDryRunThresholdSec = 3.0
	importDryRunThresholdSec = 3.0
	deployThresholdSec       = 5.0
	importThresholdSec       = 5.0
)

type scenario string

const (
	scenarioStatus    scenario = "status"
	scenarioDeployDry scenario = "deploy-dry-run"
	scenarioImportDry scenario = "import-dry-run"
	scenarioDeploy    scenario = "deploy"
	scenarioImport    scenario = "import"
)

type perfMetrics struct {
	StatusSeconds       float64 `json:"status_seconds"`
	DeployDryRunSeconds float64 `json:"deploy_dry_run_seconds"`
	ImportDryRunSeconds float64 `json:"import_dry_run_seconds"`
	DeploySeconds       float64 `json:"deploy_seconds"`
	ImportSeconds       float64 `json:"import_seconds"`
}

func TestPerformanceMetricsArtifact(t *testing.T) {
	repo := repoRoot(t)

	metrics := perfMetrics{
		StatusSeconds:       runScenario(t, repo, scenarioStatus),
		DeployDryRunSeconds: runScenario(t, repo, scenarioDeployDry),
		ImportDryRunSeconds: runScenario(t, repo, scenarioImportDry),
		DeploySeconds:       runScenario(t, repo, scenarioDeploy),
		ImportSeconds:       runScenario(t, repo, scenarioImport),
	}
	require.LessOrEqual(t, metrics.StatusSeconds, statusThresholdSeconds, "status exceeded threshold")
	require.LessOrEqual(t, metrics.DeployDryRunSeconds, deployDryRunThresholdSec, "deploy --dry-run exceeded threshold")
	require.LessOrEqual(t, metrics.ImportDryRunSeconds, importDryRunThresholdSec, "import --dry-run exceeded threshold")
	require.LessOrEqual(t, metrics.DeploySeconds, deployThresholdSec, "deploy exceeded threshold")
	require.LessOrEqual(t, metrics.ImportSeconds, importThresholdSec, "import exceeded threshold")

	artifactPath := filepath.Join(repo, "artifacts", "perf-metrics.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(artifactPath), 0o755))
	writeJSON(t, artifactPath, metrics)
}

func runScenario(t *testing.T, repo string, mode scenario) float64 {
	t.Helper()

	sb := newPerfSandbox(t, repo, mode)
	args := scenarioArgs(mode)

	start := time.Now()
	sb.runCommand(t, args...)
	return time.Since(start).Seconds()
}

func scenarioArgs(mode scenario) []string {
	switch mode {
	case scenarioStatus:
		return []string{"status"}
	case scenarioDeployDry:
		return []string{"deploy", "--dry-run"}
	case scenarioImportDry:
		return []string{"import", "--dry-run"}
	case scenarioDeploy:
		return []string{"deploy"}
	case scenarioImport:
		return []string{"import"}
	default:
		panic(fmt.Sprintf("unsupported scenario: %s", mode))
	}
}

type perfSandbox struct {
	projectDir string
	homeDir    string
}

func newPerfSandbox(t *testing.T, repo string, mode scenario) perfSandbox {
	t.Helper()

	projectDir := t.TempDir()
	homeDir := filepath.Join(t.TempDir(), "home")
	require.NoError(t, os.MkdirAll(homeDir, 0o755))

	copyPerfFixtureConfig(t, repo, projectDir)

	sourceRoot := filepath.Join(projectDir, "source")
	targetRoot := filepath.Join(homeDir, ".config", "perf")
	sourceSyncRoot := filepath.Join(sourceRoot, "managed")
	targetSyncRoot := filepath.Join(targetRoot, "managed")
	populateBaselineTree(t, sourceRoot, targetRoot, sourceSyncRoot, targetSyncRoot)

	switch mode {
	case scenarioStatus, scenarioDeployDry, scenarioDeploy:
		applyDeployDrift(t, targetSyncRoot)
	case scenarioImportDry, scenarioImport:
		applyImportDrift(t, targetSyncRoot)
	}

	return perfSandbox{projectDir: projectDir, homeDir: homeDir}
}

func copyPerfFixtureConfig(t *testing.T, repo, projectDir string) {
	t.Helper()
	fixturePath := filepath.Join(repo, "testdata", "fixtures", "perf-1k", config.DefaultConfigFile)
	body, err := os.ReadFile(fixturePath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, config.DefaultConfigFile), body, 0o644))
}

func populateBaselineTree(t *testing.T, sourceRoot, targetRoot, sourceSyncRoot, targetSyncRoot string) {
	t.Helper()
	for i := 0; i < perfManagedFileCount; i++ {
		rel := managedPath(i)
		content := []byte(fmt.Sprintf("manifest-file-%04d\n", i))
		writeFile(t, filepath.Join(sourceSyncRoot, filepath.FromSlash(rel)), content)
		writeFile(t, filepath.Join(targetSyncRoot, filepath.FromSlash(rel)), content)
	}

	for i := 0; i < perfFixtureFileCount-perfManagedFileCount; i++ {
		rel := fillerPath(i)
		content := []byte(fmt.Sprintf("filler-%04d\n", i))
		writeFile(t, filepath.Join(sourceRoot, filepath.FromSlash(rel)), content)
		writeFile(t, filepath.Join(targetRoot, filepath.FromSlash(rel)), content)
	}

	require.Equal(t, perfFixtureFileCount, countRegularFiles(t, sourceRoot), "source fixture must contain ~1k files")
}

func managedPath(i int) string {
	return fmt.Sprintf("files/group%02d/file%04d.conf", i%25, i)
}

func fillerPath(i int) string {
	return fmt.Sprintf("archive/chunk%02d/filler%04d.txt", i%25, i)
}

func applyDeployDrift(t *testing.T, targetRoot string) {
	t.Helper()
	for i := 0; i < 20; i++ {
		writeFile(t, filepath.Join(targetRoot, filepath.FromSlash(managedPath(i))), []byte(fmt.Sprintf("target-drift-%04d\n", i)))
	}
	for i := 20; i < 30; i++ {
		require.NoError(t, os.Remove(filepath.Join(targetRoot, filepath.FromSlash(managedPath(i)))))
	}
	for i := 0; i < 10; i++ {
		writeFile(t, filepath.Join(targetRoot, "trash", fmt.Sprintf("remove-%02d.tmp", i)), []byte("remove-me\n"))
	}
}

func applyImportDrift(t *testing.T, targetRoot string) {
	t.Helper()
	for i := 0; i < 20; i++ {
		writeFile(t, filepath.Join(targetRoot, filepath.FromSlash(managedPath(i))), []byte(fmt.Sprintf("target-update-%04d\n", i)))
	}
	for i := 20; i < 30; i++ {
		require.NoError(t, os.Remove(filepath.Join(targetRoot, filepath.FromSlash(managedPath(i)))))
	}
	for i := 0; i < 10; i++ {
		writeFile(t, filepath.Join(targetRoot, "incoming", "new", fmt.Sprintf("add-%02d.conf", i)), []byte("incoming\n"))
	}
}

func (s perfSandbox) runCommand(t *testing.T, args ...string) {
	t.Helper()
	cmd := app.NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)

	oldWD, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(s.projectDir))
	defer func() {
		_ = os.Chdir(oldWD)
	}()

	oldHome, hadHome := os.LookupEnv("HOME")
	require.NoError(t, os.Setenv("HOME", s.homeDir))
	defer func() {
		if hadHome {
			_ = os.Setenv("HOME", oldHome)
		} else {
			_ = os.Unsetenv("HOME")
		}
	}()

	require.NoError(t, cmd.Execute(), stderr.String())
}

func countRegularFiles(t *testing.T, root string) int {
	t.Helper()
	count := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			count++
		}
		return nil
	})
	require.NoError(t, err)
	return count
}

func writeFile(t *testing.T, path string, content []byte) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, content, 0o644))
}

func writeJSON(t *testing.T, path string, payload any) {
	t.Helper()
	file, err := os.Create(path)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = file.Close()
	})
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	require.NoError(t, encoder.Encode(payload))
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}
