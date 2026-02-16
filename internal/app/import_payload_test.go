package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/shpoont/dotfiles-manager/internal/dfmerr"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestImportDryRunPlansWithoutMutating(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := setTempHome(t)
	setCWD(t, projectDir)

	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "source", "nvim", "lua"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".config", "nvim", "lua"), 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "nvim", "lua", "init.lua"), []byte("source-init"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "nvim", "lua", "missing.lua"), []byte("remove-me"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "nvim", "lua", "keep.lua"), []byte("keep-me"), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "lua", "init.lua"), []byte("target-init"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "lua", "new.lua"), []byte("new"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "lua", "ignored.tmp"), []byte("tmp"), 0o644))

	writeConfig(t, projectDir, []byte(`syncs:
  - target: .config/nvim
    source: source/nvim
    on:
      import:
        add-unmanaged:
          include:
            - '**'
          exclude:
            - '**/*.tmp'
        remove-missing:
          include:
            - 'lua/**'
          exclude:
            - 'lua/keep.lua'
`))

	payload := runJSONCommand(t, []string{"import", "--dry-run", "--json"})
	require.Equal(t, true, payload["ok"])
	require.Equal(t, true, payload["dry_run"])

	sync := payload["syncs"].([]any)[0].(map[string]any)
	require.Equal(t, []string{"lua/init.lua"}, extractPaths(sync["updated_manifest"].([]any)))
	require.Equal(t, []string{"lua/new.lua"}, extractPaths(sync["added_unmanaged"].([]any)))
	require.Equal(t, []string{"lua/missing.lua"}, extractPaths(sync["removed_missing"].([]any)))

	summary := payload["summary"].(map[string]any)
	require.Equal(t, float64(1), summary["updated_manifest_count"])
	require.Equal(t, float64(1), summary["added_unmanaged_count"])
	require.Equal(t, float64(1), summary["removed_missing_count"])

	content, err := os.ReadFile(filepath.Join(projectDir, "source", "nvim", "lua", "init.lua"))
	require.NoError(t, err)
	require.Equal(t, "source-init", string(content))

	_, err = os.Stat(filepath.Join(projectDir, "source", "nvim", "lua", "new.lua"))
	require.Error(t, err)
	require.True(t, os.IsNotExist(err))

	_, err = os.Stat(filepath.Join(projectDir, "source", "nvim", "lua", "missing.lua"))
	require.NoError(t, err)
}

func TestImportAppliesUpdatesAddsAndMissingRemovals(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := setTempHome(t)
	setCWD(t, projectDir)

	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "source", "nvim", "lua"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".config", "nvim", "lua"), 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "nvim", "lua", "init.lua"), []byte("source-init"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "nvim", "lua", "missing.lua"), []byte("remove-me"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "nvim", "lua", "keep.lua"), []byte("keep-me"), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "lua", "init.lua"), []byte("target-init"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "lua", "new.lua"), []byte("new"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "lua", "ignored.tmp"), []byte("tmp"), 0o644))

	writeConfig(t, projectDir, []byte(`syncs:
  - target: .config/nvim
    source: source/nvim
    on:
      import:
        add-unmanaged:
          include:
            - '**'
          exclude:
            - '**/*.tmp'
        remove-missing:
          include:
            - 'lua/**'
          exclude:
            - 'lua/keep.lua'
`))

	payload := runJSONCommand(t, []string{"import", "--json"})
	sync := payload["syncs"].([]any)[0].(map[string]any)
	require.Equal(t, []string{"lua/init.lua"}, extractPaths(sync["updated_manifest"].([]any)))
	require.Equal(t, []string{"lua/new.lua"}, extractPaths(sync["added_unmanaged"].([]any)))
	require.Equal(t, []string{"lua/missing.lua"}, extractPaths(sync["removed_missing"].([]any)))

	content, err := os.ReadFile(filepath.Join(projectDir, "source", "nvim", "lua", "init.lua"))
	require.NoError(t, err)
	require.Equal(t, "target-init", string(content))

	content, err = os.ReadFile(filepath.Join(projectDir, "source", "nvim", "lua", "new.lua"))
	require.NoError(t, err)
	require.Equal(t, "new", string(content))

	_, err = os.Stat(filepath.Join(projectDir, "source", "nvim", "lua", "missing.lua"))
	require.Error(t, err)
	require.True(t, os.IsNotExist(err))

	_, err = os.Stat(filepath.Join(projectDir, "source", "nvim", "lua", "keep.lua"))
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(projectDir, "source", "nvim", "lua", "ignored.tmp"))
	require.Error(t, err)
	require.True(t, os.IsNotExist(err))
}

func TestImportScopeFiltersToSubtree(t *testing.T) {
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
	payload := runJSONCommand(t, []string{"import", "--json", scopePath})
	sync := payload["syncs"].([]any)[0].(map[string]any)
	require.Equal(t, "lua", sync["scope_prefix"])
	require.Equal(t, []string{"lua/init.lua"}, extractPaths(sync["updated_manifest"].([]any)))

	luaContent, err := os.ReadFile(filepath.Join(projectDir, "source", "nvim", "lua", "init.lua"))
	require.NoError(t, err)
	require.Equal(t, "lua-target", string(luaContent))

	alphaContent, err := os.ReadFile(filepath.Join(projectDir, "source", "nvim", "alpha", "init.lua"))
	require.NoError(t, err)
	require.Equal(t, "alpha-source", string(alphaContent))
}

func TestImportReplaceTypeToSymlink(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := setTempHome(t)
	setCWD(t, projectDir)

	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "source", "nvim", "lua"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".config", "nvim", "lua"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "lua", "target.lua"), []byte("target"), 0o644))
	require.NoError(t, os.Symlink("target.lua", filepath.Join(homeDir, ".config", "nvim", "lua", "link")))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "nvim", "lua", "link"), []byte("not-link"), 0o644))

	writeConfig(t, projectDir, []byte(`syncs:
  - target: .config/nvim
    source: source/nvim
`))

	payload := runJSONCommand(t, []string{"import", "--json"})
	sync := payload["syncs"].([]any)[0].(map[string]any)
	updated := sync["updated_manifest"].([]any)
	foundReplace := false
	for _, entry := range updated {
		item := entry.(map[string]any)
		if item["path"] == "lua/link" {
			require.Equal(t, "replace_type", item["change"])
			foundReplace = true
		}
	}
	require.True(t, foundReplace)

	linkTarget, err := os.Readlink(filepath.Join(projectDir, "source", "nvim", "lua", "link"))
	require.NoError(t, err)
	require.Equal(t, "target.lua", linkTarget)
}

func TestImportInvalidPatternReturnsError(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := setTempHome(t)
	setCWD(t, projectDir)

	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".config", "nvim"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "unmanaged.txt"), []byte("x"), 0o644))

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
	cmd.SetArgs([]string{"import", "--json"})

	err := cmd.Execute()
	require.Error(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	require.Equal(t, false, payload["ok"])
	errorObj := payload["error"].(map[string]any)
	require.Equal(t, "DFM_CONFIG_SCHEMA_TYPE", errorObj["code"])
}

func TestImportWithoutPatternsSkipsUnmanagedAddAndMissingRemoval(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := setTempHome(t)
	setCWD(t, projectDir)

	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "source", "nvim", "lua"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".config", "nvim", "lua"), 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "nvim", "lua", "missing.lua"), []byte("keep"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "lua", "new.lua"), []byte("new"), 0o644))

	writeConfig(t, projectDir, []byte(`syncs:
  - target: .config/nvim
    source: source/nvim
`))

	payload := runJSONCommand(t, []string{"import", "--json"})
	sync := payload["syncs"].([]any)[0].(map[string]any)
	require.Empty(t, sync["added_unmanaged"].([]any))
	require.Empty(t, sync["removed_missing"].([]any))
	require.Empty(t, sync["updated_manifest"].([]any))

	_, err := os.Stat(filepath.Join(projectDir, "source", "nvim", "lua", "missing.lua"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(projectDir, "source", "nvim", "lua", "new.lua"))
	require.Error(t, err)
	require.True(t, os.IsNotExist(err))
}

func TestImportPreservesFileModeAndMtime(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := setTempHome(t)
	setCWD(t, projectDir)

	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "source", "nvim", "lua"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".config", "nvim", "lua"), 0o755))

	targetPath := filepath.Join(homeDir, ".config", "nvim", "lua", "init.lua")
	require.NoError(t, os.WriteFile(targetPath, []byte("target-init"), 0o740))
	targetTime := time.Unix(1_704_000_000, 0).UTC()
	require.NoError(t, os.Chtimes(targetPath, targetTime, targetTime))

	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "nvim", "lua", "init.lua"), []byte("source-init"), 0o600))

	writeConfig(t, projectDir, []byte(`syncs:
  - target: .config/nvim
    source: source/nvim
`))

	_ = runJSONCommand(t, []string{"import", "--json"})

	sourcePath := filepath.Join(projectDir, "source", "nvim", "lua", "init.lua")
	sourceInfo, err := os.Stat(sourcePath)
	require.NoError(t, err)
	targetInfo, err := os.Stat(targetPath)
	require.NoError(t, err)
	require.Equal(t, targetInfo.Mode().Perm(), sourceInfo.Mode().Perm())
	require.Equal(t, targetInfo.ModTime().Unix(), sourceInfo.ModTime().Unix())
}

func TestImportJSONErrorIncludesAppliedPartialOperations(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := setTempHome(t)
	setCWD(t, projectDir)

	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "source", "nvim"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".config", "nvim"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "a.lua"), []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "b.lua"), []byte("b"), 0o644))

	writeConfig(t, projectDir, []byte(`syncs:
  - target: .config/nvim
    source: source/nvim
    on:
      import:
        add-unmanaged:
          include:
            - '**'
`))

	originalRunCopy := runImportCopy
	originalRunRemove := runImportRemove
	t.Cleanup(func() {
		runImportCopy = originalRunCopy
		runImportRemove = originalRunRemove
	})

	copyCalls := 0
	runImportCopy = func(op importCopyOperation) error {
		copyCalls++
		if copyCalls == 2 {
			return dfmerr.Wrap(dfmerr.CodeIOWrite, "Write failed: "+op.targetAbs, map[string]any{"path": op.targetAbs}, errors.New("boom"))
		}
		return nil
	}
	runImportRemove = applyImportRemove

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"import", "--json"})

	err := cmd.Execute()
	require.Error(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	require.Equal(t, false, payload["ok"])
	require.Equal(t, "DFM_IO_WRITE", payload["error"].(map[string]any)["code"])

	summary := payload["summary"].(map[string]any)
	require.Equal(t, true, summary["partial"])
	require.Equal(t, float64(1), summary["sync_count"])
	require.Equal(t, float64(0), summary["updated_manifest_count"])
	require.Equal(t, float64(1), summary["added_unmanaged_count"])

	syncs := payload["syncs"].([]any)
	require.Len(t, syncs, 1)
	sync := syncs[0].(map[string]any)
	require.Empty(t, sync["updated_manifest"].([]any))
	require.Equal(t, []string{"a.lua"}, extractPaths(sync["added_unmanaged"].([]any)))
}

func TestImportJSONErrorIncludesCompletedAndFailedSyncEntries(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := setTempHome(t)
	setCWD(t, projectDir)

	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "source", "s1"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "source", "s2"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".config", "s1"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".config", "s2"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "s1", "a.lua"), []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "s2", "b.lua"), []byte("b"), 0o644))

	writeConfig(t, projectDir, []byte(`syncs:
  - target: .config/s1
    source: source/s1
    on:
      import:
        add-unmanaged:
          include:
            - '**'
  - target: .config/s2
    source: source/s2
    on:
      import:
        add-unmanaged:
          include:
            - '**'
`))

	originalRunCopy := runImportCopy
	originalRunRemove := runImportRemove
	t.Cleanup(func() {
		runImportCopy = originalRunCopy
		runImportRemove = originalRunRemove
	})

	copyCalls := 0
	runImportCopy = func(op importCopyOperation) error {
		copyCalls++
		if copyCalls == 2 {
			return dfmerr.Wrap(dfmerr.CodeIOWrite, "Write failed: "+op.targetAbs, map[string]any{"path": op.targetAbs}, errors.New("boom"))
		}
		return nil
	}
	runImportRemove = applyImportRemove

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"import", "--json"})

	err := cmd.Execute()
	require.Error(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	require.Equal(t, false, payload["ok"])
	summary := payload["summary"].(map[string]any)
	require.Equal(t, true, summary["partial"])
	require.Equal(t, float64(2), summary["sync_count"])
	require.Equal(t, float64(1), summary["added_unmanaged_count"])

	syncs := payload["syncs"].([]any)
	require.Len(t, syncs, 2)
	require.Equal(t, []string{"a.lua"}, extractPaths(syncs[0].(map[string]any)["added_unmanaged"].([]any)))
	require.Empty(t, syncs[1].(map[string]any)["added_unmanaged"].([]any))
}

func TestImportPreservesXattrsWhenSupported(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := setTempHome(t)
	setCWD(t, projectDir)

	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "source", "nvim"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".config", "nvim"), 0o755))

	sourcePath := filepath.Join(projectDir, "source", "nvim", "init.lua")
	targetPath := filepath.Join(homeDir, ".config", "nvim", "init.lua")
	require.NoError(t, os.WriteFile(sourcePath, []byte("source"), 0o644))
	require.NoError(t, os.WriteFile(targetPath, []byte("target"), 0o644))

	xattrKey := "user.dotfiles_manager_test"
	if runtime.GOOS == "darwin" {
		xattrKey = "com.dotfiles-manager.test"
	}
	setErr := unix.Setxattr(targetPath, xattrKey, []byte("xattr-value"), 0)
	if isMetadataUnsupported(setErr) {
		t.Skip("xattrs unsupported on this filesystem")
	}
	require.NoError(t, setErr)

	writeConfig(t, projectDir, []byte(`syncs:
  - target: .config/nvim
    source: source/nvim
`))

	_ = runJSONCommand(t, []string{"import", "--json"})

	value, err := readXattr(sourcePath, xattrKey)
	if isMetadataUnsupported(err) {
		t.Skip("xattrs unsupported during source read")
	}
	require.NoError(t, err)
	require.Equal(t, []byte("xattr-value"), value)
}
