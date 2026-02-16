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

func TestStatusJSONReportsDriftAndCandidates(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := setTempHome(t)
	setCWD(t, projectDir)

	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "source", "nvim", "lua"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "source", "nvim", "alpha"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".config", "nvim", "lua"), 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "nvim", "lua", "init.lua"), []byte("source-init"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "nvim", "lua", "only-source.lua"), []byte("source-only"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "nvim", "alpha", "a.lua"), []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "nvim", "alpha", "z.lua"), []byte("z"), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "lua", "init.lua"), []byte("target-init"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "lua", "new.lua"), []byte("new"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "lua", "old.bak"), []byte("bak"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "lua", "tmp.tmp"), []byte("tmp"), 0o644))

	writeConfig(t, projectDir, []byte(`syncs:
  - target: .config/nvim
    source: source/nvim
    on:
      deploy:
        remove-unmanaged:
          - '**/*.bak'
      import:
        add-unmanaged:
          include:
            - '**'
          exclude:
            - '**/*.tmp'
        remove-missing:
          include:
            - 'lua/**'
`))

	payload := runJSONCommand(t, []string{"status", "--json"})
	require.Equal(t, true, payload["ok"])
	require.Equal(t, "status", payload["command"])
	require.Equal(t, "1.0", payload["schema_version"])

	pathScope := payload["path_scope"].(map[string]any)
	require.Nil(t, pathScope["input"])
	require.Nil(t, pathScope["normalized"])
	require.Equal(t, []any{float64(0)}, pathScope["matched_sync_indexes"])

	syncs := payload["syncs"].([]any)
	require.Len(t, syncs, 1)
	sync := syncs[0].(map[string]any)
	require.Equal(t, float64(0), sync["sync_index"])
	require.Equal(t, "", sync["scope_prefix"])

	deployChanges := sync["deploy_changes"].([]any)
	require.Equal(t, []string{"alpha", "alpha/a.lua", "alpha/z.lua", "lua/init.lua", "lua/only-source.lua"}, extractPaths(deployChanges))
	require.Equal(t, "create", deployChanges[0].(map[string]any)["change"])
	require.Equal(t, "update", deployChanges[3].(map[string]any)["change"])

	importChanges := sync["import_changes"].([]any)
	require.Equal(t, []string{"lua/init.lua"}, extractPaths(importChanges))
	require.Equal(t, "update", importChanges[0].(map[string]any)["change"])

	incoming := sync["incoming_unmanaged"].([]any)
	require.Equal(t, []string{"lua/new.lua", "lua/old.bak"}, extractPaths(incoming))

	removableUnmanaged := sync["removable_unmanaged"].([]any)
	require.Equal(t, []string{"lua/old.bak"}, extractPaths(removableUnmanaged))

	removableMissing := sync["removable_missing"].([]any)
	require.Equal(t, []string{"lua/only-source.lua"}, extractPaths(removableMissing))

	summary := payload["summary"].(map[string]any)
	require.Equal(t, float64(1), summary["sync_count"])
	require.Equal(t, float64(5), summary["deploy_change_count"])
	require.Equal(t, float64(1), summary["import_change_count"])
	require.Equal(t, float64(2), summary["incoming_unmanaged_count"])
	require.Equal(t, float64(1), summary["removable_unmanaged_count"])
	require.Equal(t, float64(1), summary["removable_missing_count"])
}

func TestStatusJSONScopeFiltersSubtree(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := setTempHome(t)
	setCWD(t, projectDir)

	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "source", "nvim", "lua"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "source", "nvim", "alpha"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".config", "nvim", "lua"), 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "nvim", "lua", "init.lua"), []byte("A"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "nvim", "alpha", "other.lua"), []byte("B"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "lua", "init.lua"), []byte("C"), 0o644))

	writeConfig(t, projectDir, []byte(`syncs:
  - target: .config/nvim
    source: source/nvim
`))

	payload := runJSONCommand(t, []string{"status", "--json", filepath.Join(homeDir, ".config", "nvim", "lua")})
	sync := payload["syncs"].([]any)[0].(map[string]any)
	require.Equal(t, "lua", sync["scope_prefix"])
	require.Equal(t, []string{"lua/init.lua"}, extractPaths(sync["deploy_changes"].([]any)))
	require.Equal(t, []string{"lua/init.lua"}, extractPaths(sync["import_changes"].([]any)))

	pathScope := payload["path_scope"].(map[string]any)
	require.Equal(t, []any{float64(0)}, pathScope["matched_sync_indexes"])
}

func TestStatusJSONReportsReplaceTypeChange(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := setTempHome(t)
	setCWD(t, projectDir)

	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "source", "nvim", "lua"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".config", "nvim", "lua"), 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "nvim", "lua", "link"), []byte("not-symlink"), 0o644))
	require.NoError(t, os.Symlink("target.lua", filepath.Join(homeDir, ".config", "nvim", "lua", "link")))

	writeConfig(t, projectDir, []byte(`syncs:
  - target: .config/nvim
    source: source/nvim
`))

	payload := runJSONCommand(t, []string{"status", "--json"})
	sync := payload["syncs"].([]any)[0].(map[string]any)
	deploy := sync["deploy_changes"].([]any)
	require.Equal(t, 1, len(deploy))
	require.Equal(t, "replace_type", deploy[0].(map[string]any)["change"])
	require.Equal(t, "file", deploy[0].(map[string]any)["source_type"])
	require.Equal(t, "symlink", deploy[0].(map[string]any)["target_type"])
}

func TestStatusJSONInvalidPatternReturnsError(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := setTempHome(t)
	setCWD(t, projectDir)

	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".config", "nvim"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "new.lua"), []byte("new"), 0o644))

	writeConfig(t, projectDir, []byte(`syncs:
  - target: .config/nvim
    source: source/nvim
    on:
      import:
        add-unmanaged:
          include:
            - '['
`))

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"status", "--json"})

	err := cmd.Execute()
	require.Error(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	require.Equal(t, false, payload["ok"])
	errorObj := payload["error"].(map[string]any)
	require.Equal(t, "DFM_CONFIG_SCHEMA_TYPE", errorObj["code"])
}

func runJSONCommand(t *testing.T, args []string) map[string]any {
	t.Helper()
	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)

	err := cmd.Execute()
	require.NoError(t, err, stderr.String())

	var payload map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	return payload
}

func writeConfig(t *testing.T, projectDir string, content []byte) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, config.DefaultConfigFile), content, 0o644))
}

func setCWD(t *testing.T, dir string) {
	t.Helper()
	oldWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	require.NoError(t, os.Chdir(dir))
}

func setTempHome(t *testing.T) string {
	t.Helper()
	homeDir := t.TempDir()
	oldHome, hadHome := os.LookupEnv("HOME")
	require.NoError(t, os.Setenv("HOME", homeDir))
	t.Cleanup(func() {
		if hadHome {
			_ = os.Setenv("HOME", oldHome)
		} else {
			_ = os.Unsetenv("HOME")
		}
	})
	return homeDir
}

func extractPaths(items []any) []string {
	paths := make([]string, 0, len(items))
	for _, item := range items {
		paths = append(paths, item.(map[string]any)["path"].(string))
	}
	return paths
}
