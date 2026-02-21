package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/shpoont/dotfiles-manager/internal/config"
	"github.com/stretchr/testify/require"
)

func TestStatusJSONErrorEnvelopeWhenConfigMissing(t *testing.T) {
	tempDir := t.TempDir()
	oldWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})
	require.NoError(t, os.Chdir(tempDir))

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"status", "--json"})

	err = cmd.Execute()
	require.Error(t, err)
	require.NotEmpty(t, stdout.String())

	var payload map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	require.Equal(t, false, payload["ok"])
	require.Equal(t, "4.0", payload["schema_version"])
	require.Equal(t, "status", payload["command"])
	require.Contains(t, payload, "path_scope")
	require.Contains(t, payload, "syncs")
	require.Contains(t, payload, "summary")
	errorObj, ok := payload["error"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "DFM_CONFIG_REQUIRED", errorObj["code"])
}

func TestDeployDryRunWithDefaultConfig(t *testing.T) {
	tempDir := t.TempDir()
	oldWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})
	require.NoError(t, os.Chdir(tempDir))

	configPath := filepath.Join(tempDir, config.DefaultConfigFile)
	configBody := []byte("syncs:\n  - target: .config/nvim\n    source: .config/nvim\n")
	require.NoError(t, os.WriteFile(configPath, configBody, 0o644))

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"deploy", "--dry-run", "--json"})

	err = cmd.Execute()
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	require.Equal(t, true, payload["ok"])
	require.Equal(t, "deploy", payload["command"])
	require.Equal(t, true, payload["dry_run"])
	require.Equal(t, "4.0", payload["schema_version"])
	require.Contains(t, payload, "syncs")
	require.Contains(t, payload, "summary")
}

func TestStatusDryRunUnsupported(t *testing.T) {
	tempDir := t.TempDir()
	oldWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})
	require.NoError(t, os.Chdir(tempDir))

	configPath := filepath.Join(tempDir, config.DefaultConfigFile)
	configBody := []byte("syncs:\n  - target: .config/nvim\n    source: .config/nvim\n")
	require.NoError(t, os.WriteFile(configPath, configBody, 0o644))

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"status", "--json", "--dry-run"})

	err = cmd.Execute()
	require.Error(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	require.Equal(t, false, payload["ok"])
	errorObj := payload["error"].(map[string]any)
	require.Equal(t, "DFM_FLAG_UNSUPPORTED", errorObj["code"])
}

func TestPathScopeNoMatchReturnsError(t *testing.T) {
	tempDir := t.TempDir()
	oldWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})
	require.NoError(t, os.Chdir(tempDir))

	configPath := filepath.Join(tempDir, config.DefaultConfigFile)
	configBody := []byte("syncs:\n  - target: .config/nvim\n    source: .config/nvim\n")
	require.NoError(t, os.WriteFile(configPath, configBody, 0o644))

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"status", "--json", "~/.config/does-not-exist"})

	err = cmd.Execute()
	require.Error(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	require.Equal(t, false, payload["ok"])
	errorObj := payload["error"].(map[string]any)
	require.Equal(t, "DFM_SCOPE_NO_MATCH", errorObj["code"])
}

func TestPathScopeParentOfTargetReturnsError(t *testing.T) {
	tempDir := t.TempDir()
	oldWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})
	require.NoError(t, os.Chdir(tempDir))

	configPath := filepath.Join(tempDir, config.DefaultConfigFile)
	configBody := []byte("syncs:\n  - target: .config/nvim\n    source: .config/nvim\n")
	require.NoError(t, os.WriteFile(configPath, configBody, 0o644))

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"deploy", "--json", "~/.config"})

	err = cmd.Execute()
	require.Error(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	require.Equal(t, false, payload["ok"])
	errorObj := payload["error"].(map[string]any)
	require.Equal(t, "DFM_SCOPE_NO_MATCH", errorObj["code"])
}
