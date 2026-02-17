//go:build integration

package integration_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/shpoont/dotfiles-manager/internal/app"
	"github.com/shpoont/dotfiles-manager/internal/config"
	"github.com/stretchr/testify/require"
)

func TestIntegrationStatusSmoke(t *testing.T) {
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

	cmd := app.NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"status"})

	require.NoError(t, cmd.Execute())
	require.Contains(t, out.String(), "sync[0] target=~/.config/nvim source=./.config/nvim")
	require.Contains(t, out.String(), "summary")
	require.NotContains(t, out.String(), "deploy[0]")
}

func TestIntegrationDeployOverlapLaterSyncWins(t *testing.T) {
	tempDir := t.TempDir()
	homeDir := t.TempDir()

	oldWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})
	require.NoError(t, os.Chdir(tempDir))

	oldHome, hadHome := os.LookupEnv("HOME")
	require.NoError(t, os.Setenv("HOME", homeDir))
	t.Cleanup(func() {
		if hadHome {
			_ = os.Setenv("HOME", oldHome)
		} else {
			_ = os.Unsetenv("HOME")
		}
	})

	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "source", "base"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "source", "override"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "source", "base", "init.lua"), []byte("base"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "source", "override", "init.lua"), []byte("override"), 0o644))

	configPath := filepath.Join(tempDir, config.DefaultConfigFile)
	configBody := []byte(`syncs:
  - target: .config/nvim
    source: source/base
  - target: .config/nvim
    source: source/override
`)
	require.NoError(t, os.WriteFile(configPath, configBody, 0o644))

	cmd := app.NewRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"deploy"})
	require.NoError(t, cmd.Execute())

	content, err := os.ReadFile(filepath.Join(homeDir, ".config", "nvim", "init.lua"))
	require.NoError(t, err)
	require.Equal(t, "override", string(content))
}
