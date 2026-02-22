package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestScopedCommandsReportExcludedSyncCountInJSON(t *testing.T) {
	projectDir, homeDir, scopePath := setupScopedSummaryFixture(t)

	commands := [][]string{
		{"status", "--json", scopePath},
		{"diff", "--json", scopePath},
		{"deploy", "--json", "--dry-run", scopePath},
		{"import", "--json", "--dry-run", scopePath},
	}

	for _, args := range commands {
		cmd := NewRootCmd()
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		cmd.SetOut(&stdout)
		cmd.SetErr(&stderr)
		cmd.SetArgs(args)

		require.NoError(t, cmd.Execute(), stderr.String())

		var payload map[string]any
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
		summary := payload["summary"].(map[string]any)
		require.Equal(t, float64(1), summary["sync_count"])
		require.Equal(t, float64(1), summary["excluded_sync_count"])
	}

	unscoped := runJSONCommand(t, []string{"status", "--json"})
	unscopedSummary := unscoped["summary"].(map[string]any)
	require.Equal(t, float64(2), unscopedSummary["sync_count"])
	require.Equal(t, float64(0), unscopedSummary["excluded_sync_count"])

	_ = projectDir
	_ = homeDir
}

func TestScopedTextSummaryReportsExcludedSyncCount(t *testing.T) {
	_, _, scopePath := setupScopedSummaryFixture(t)

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"status", scopePath})

	require.NoError(t, cmd.Execute(), stderr.String())
	require.Contains(t, stdout.String(), "excluded-syncs=1")
}

func setupScopedSummaryFixture(t *testing.T) (projectDir string, homeDir string, scopePath string) {
	t.Helper()

	projectDir = t.TempDir()
	homeDir = setTempHome(t)
	setCWD(t, projectDir)

	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "source", "nvim"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "source", "zsh"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".config", "nvim"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".config", "zsh"), 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "nvim", "init.lua"), []byte("source-nvim\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "init.lua"), []byte("target-nvim\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "zsh", ".zshrc"), []byte("source-zsh\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "zsh", ".zshrc"), []byte("target-zsh\n"), 0o644))

	writeConfig(t, projectDir, []byte(`syncs:
  - target: .config/nvim
    source: source/nvim
  - target: .config/zsh
    source: source/zsh
`))

	scopePath = filepath.Join(homeDir, ".config", "nvim")
	return projectDir, homeDir, scopePath
}
