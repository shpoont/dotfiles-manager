package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeployCreatesDirectoryEntries(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := setTempHome(t)
	setCWD(t, projectDir)

	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "source", "nvim", "lua", "plugins"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".config", "nvim"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "nvim", "lua", "plugins", "spec.lua"), []byte("plugin"), 0o644))

	writeConfig(t, projectDir, []byte(`syncs:
  - target: .config/nvim
    source: source/nvim
`))

	payload := runJSONCommand(t, []string{"deploy", "--json"})
	require.Equal(t, true, payload["ok"])
	require.Equal(t, false, payload["dry_run"])

	sync := payload["syncs"].([]any)[0].(map[string]any)
	copied := sync["copied"].([]any)

	foundDir := false
	for _, entry := range copied {
		item := entry.(map[string]any)
		if item["path"] == "lua/plugins" {
			require.Equal(t, "dir", item["type"])
			require.Equal(t, "create", item["change"])
			foundDir = true
		}
	}
	require.True(t, foundDir)

	info, err := os.Stat(filepath.Join(homeDir, ".config", "nvim", "lua", "plugins"))
	require.NoError(t, err)
	require.True(t, info.IsDir())
}
