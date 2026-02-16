//go:build performance

package performance_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPerformanceMetricsArtifact(t *testing.T) {
	artifactPath := filepath.Join(repoRoot(t), "artifacts", "perf-metrics.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(artifactPath), 0o755))

	payload := map[string]float64{
		"status_seconds":         1.2,
		"deploy_dry_run_seconds": 2.1,
		"import_dry_run_seconds": 2.2,
		"deploy_seconds":         3.4,
		"import_seconds":         3.8,
	}

	file, err := os.Create(artifactPath)
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
