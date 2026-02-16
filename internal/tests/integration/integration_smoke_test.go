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
	require.Contains(t, out.String(), "status: syncs=1")
}
