package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shpoont/dotfiles-manager/internal/dfmerr"
	"github.com/stretchr/testify/require"
)

func TestDeployDryRunPlansWithoutMutating(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := setTempHome(t)
	setCWD(t, projectDir)

	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "source", "nvim", "lua"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".config", "nvim", "lua"), 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "nvim", "lua", "init.lua"), []byte("source-init"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "nvim", "lua", "new.lua"), []byte("new"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "lua", "init.lua"), []byte("target-init"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "lua", "old.bak"), []byte("bak"), 0o644))

	writeConfig(t, projectDir, []byte(`syncs:
  - target: .config/nvim
    source: source/nvim
    on:
      deploy:
        remove-unmanaged:
          - '**/*.bak'
`))

	payload := runJSONCommand(t, []string{"deploy", "--dry-run", "--json"})
	require.Equal(t, true, payload["ok"])
	require.Equal(t, true, payload["dry_run"])

	sync := payload["syncs"].([]any)[0].(map[string]any)
	require.Equal(t, []string{"lua/init.lua", "lua/new.lua"}, extractPaths(sync["copied"].([]any)))
	require.Equal(t, []string{"lua/old.bak"}, extractPaths(sync["removed_unmanaged"].([]any)))

	summary := payload["summary"].(map[string]any)
	require.Equal(t, float64(2), summary["copied_count"])
	require.Equal(t, float64(1), summary["removed_unmanaged_count"])

	content, err := os.ReadFile(filepath.Join(homeDir, ".config", "nvim", "lua", "init.lua"))
	require.NoError(t, err)
	require.Equal(t, "target-init", string(content))
	_, err = os.Stat(filepath.Join(homeDir, ".config", "nvim", "lua", "old.bak"))
	require.NoError(t, err)
}

func TestDeployAppliesCopyAndRemoval(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := setTempHome(t)
	setCWD(t, projectDir)

	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "source", "nvim", "lua"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".config", "nvim", "lua"), 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "nvim", "lua", "init.lua"), []byte("source-init"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "nvim", "lua", "new.lua"), []byte("new"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "lua", "init.lua"), []byte("target-init"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "lua", "old.bak"), []byte("bak"), 0o644))

	writeConfig(t, projectDir, []byte(`syncs:
  - target: .config/nvim
    source: source/nvim
    on:
      deploy:
        remove-unmanaged:
          - '**/*.bak'
`))

	payload := runJSONCommand(t, []string{"deploy", "--json"})
	sync := payload["syncs"].([]any)[0].(map[string]any)
	require.Equal(t, []string{"lua/init.lua", "lua/new.lua"}, extractPaths(sync["copied"].([]any)))
	require.Equal(t, []string{"lua/old.bak"}, extractPaths(sync["removed_unmanaged"].([]any)))

	content, err := os.ReadFile(filepath.Join(homeDir, ".config", "nvim", "lua", "init.lua"))
	require.NoError(t, err)
	require.Equal(t, "source-init", string(content))

	content, err = os.ReadFile(filepath.Join(homeDir, ".config", "nvim", "lua", "new.lua"))
	require.NoError(t, err)
	require.Equal(t, "new", string(content))

	_, err = os.Stat(filepath.Join(homeDir, ".config", "nvim", "lua", "old.bak"))
	require.Error(t, err)
	require.True(t, os.IsNotExist(err))
}

func TestDeployScopeFiltersToSubtree(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := setTempHome(t)
	setCWD(t, projectDir)

	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "source", "nvim", "lua"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "source", "nvim", "alpha"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".config", "nvim", "lua"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".config", "nvim", "alpha"), 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "nvim", "lua", "init.lua"), []byte("lua-source"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "nvim", "alpha", "init.lua"), []byte("alpha-source"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "lua", "init.lua"), []byte("lua-target"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "alpha", "init.lua"), []byte("alpha-target"), 0o644))

	writeConfig(t, projectDir, []byte(`syncs:
  - target: .config/nvim
    source: source/nvim
`))

	scopePath := filepath.Join(homeDir, ".config", "nvim", "lua")
	payload := runJSONCommand(t, []string{"deploy", "--json", scopePath})
	sync := payload["syncs"].([]any)[0].(map[string]any)
	require.Equal(t, "lua", sync["scope_prefix"])
	require.Equal(t, []string{"lua/init.lua"}, extractPaths(sync["copied"].([]any)))

	luaContent, err := os.ReadFile(filepath.Join(homeDir, ".config", "nvim", "lua", "init.lua"))
	require.NoError(t, err)
	require.Equal(t, "lua-source", string(luaContent))

	alphaContent, err := os.ReadFile(filepath.Join(homeDir, ".config", "nvim", "alpha", "init.lua"))
	require.NoError(t, err)
	require.Equal(t, "alpha-target", string(alphaContent))
}

func TestDeployReplaceTypeToSymlink(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := setTempHome(t)
	setCWD(t, projectDir)

	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "source", "nvim", "lua"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".config", "nvim", "lua"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "nvim", "lua", "target.lua"), []byte("target"), 0o644))
	require.NoError(t, os.Symlink("target.lua", filepath.Join(projectDir, "source", "nvim", "lua", "link")))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "lua", "link"), []byte("not-link"), 0o644))

	writeConfig(t, projectDir, []byte(`syncs:
  - target: .config/nvim
    source: source/nvim
`))

	payload := runJSONCommand(t, []string{"deploy", "--json"})
	sync := payload["syncs"].([]any)[0].(map[string]any)
	copied := sync["copied"].([]any)
	foundReplace := false
	for _, entry := range copied {
		item := entry.(map[string]any)
		if item["path"] == "lua/link" {
			require.Equal(t, "replace_type", item["change"])
			foundReplace = true
		}
	}
	require.True(t, foundReplace)

	linkTarget, err := os.Readlink(filepath.Join(homeDir, ".config", "nvim", "lua", "link"))
	require.NoError(t, err)
	require.Equal(t, "target.lua", linkTarget)
}

func TestDeployInvalidRemovePatternReturnsError(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := setTempHome(t)
	setCWD(t, projectDir)

	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "source", "nvim"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".config", "nvim"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "unmanaged.txt"), []byte("x"), 0o644))

	writeConfig(t, projectDir, []byte(`syncs:
  - target: .config/nvim
    source: source/nvim
    on:
      deploy:
        remove-unmanaged:
          - '['
`))

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"deploy", "--json"})

	err := cmd.Execute()
	require.Error(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	require.Equal(t, false, payload["ok"])
	errorObj := payload["error"].(map[string]any)
	require.Equal(t, "DFM_CONFIG_SCHEMA_TYPE", errorObj["code"])
}

func TestDeployEmptyRemovePatternsDoNotDeleteUnmanaged(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := setTempHome(t)
	setCWD(t, projectDir)

	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "source", "nvim"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".config", "nvim"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "keep.txt"), []byte("keep"), 0o644))

	writeConfig(t, projectDir, []byte(`syncs:
  - target: .config/nvim
    source: source/nvim
`))

	payload := runJSONCommand(t, []string{"deploy", "--json"})
	sync := payload["syncs"].([]any)[0].(map[string]any)
	require.Empty(t, sync["removed_unmanaged"].([]any))

	_, err := os.Stat(filepath.Join(homeDir, ".config", "nvim", "keep.txt"))
	require.NoError(t, err)
}

func TestDeployPreservesFileModeAndMtime(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := setTempHome(t)
	setCWD(t, projectDir)

	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "source", "nvim", "lua"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".config", "nvim", "lua"), 0o755))

	sourcePath := filepath.Join(projectDir, "source", "nvim", "lua", "init.lua")
	require.NoError(t, os.WriteFile(sourcePath, []byte("source-init"), 0o750))
	sourceTime := time.Unix(1_703_000_000, 0).UTC()
	require.NoError(t, os.Chtimes(sourcePath, sourceTime, sourceTime))

	writeConfig(t, projectDir, []byte(`syncs:
  - target: .config/nvim
    source: source/nvim
`))

	_ = runJSONCommand(t, []string{"deploy", "--json"})

	targetPath := filepath.Join(homeDir, ".config", "nvim", "lua", "init.lua")
	sourceInfo, err := os.Stat(sourcePath)
	require.NoError(t, err)
	targetInfo, err := os.Stat(targetPath)
	require.NoError(t, err)
	require.Equal(t, sourceInfo.Mode().Perm(), targetInfo.Mode().Perm())
	require.Equal(t, sourceInfo.ModTime().Unix(), targetInfo.ModTime().Unix())
}

func TestDeployJSONErrorIncludesAppliedPartialOperations(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := setTempHome(t)
	setCWD(t, projectDir)

	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "source", "nvim"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".config", "nvim"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "nvim", "a.lua"), []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "nvim", "b.lua"), []byte("b"), 0o644))

	writeConfig(t, projectDir, []byte(`syncs:
  - target: .config/nvim
    source: source/nvim
`))

	originalRunCopy := runDeployCopy
	originalRunRemove := runDeployRemove
	t.Cleanup(func() {
		runDeployCopy = originalRunCopy
		runDeployRemove = originalRunRemove
	})

	copyCalls := 0
	runDeployCopy = func(op deployCopyOperation) error {
		copyCalls++
		if copyCalls == 2 {
			return dfmerr.Wrap(dfmerr.CodeIOWrite, "Write failed: "+op.targetAbs, map[string]any{"path": op.targetAbs}, errors.New("boom"))
		}
		return nil
	}
	runDeployRemove = applyDeployRemove

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"deploy", "--json"})

	err := cmd.Execute()
	require.Error(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	require.Equal(t, false, payload["ok"])
	require.Equal(t, "DFM_IO_WRITE", payload["error"].(map[string]any)["code"])

	summary := payload["summary"].(map[string]any)
	require.Equal(t, true, summary["partial"])
	require.Equal(t, float64(1), summary["sync_count"])
	require.Equal(t, float64(1), summary["copied_count"])

	syncs := payload["syncs"].([]any)
	require.Len(t, syncs, 1)
	sync := syncs[0].(map[string]any)
	require.Equal(t, []string{"a.lua"}, extractPaths(sync["copied"].([]any)))
}
