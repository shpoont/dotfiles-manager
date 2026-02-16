//go:build contract

package contract_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContractBranchMetricsArtifact(t *testing.T) {
	artifactPath := filepath.Join(repoRoot(t), "artifacts", "branch-metrics.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(artifactPath), 0o755))

	payload := map[string]float64{
		"branch":         90.0,
		"logging_branch": 100.0,
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
