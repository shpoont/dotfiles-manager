package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDiffTextOutputShowsUnifiedPatchByDefault(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := setTempHome(t)
	setCWD(t, projectDir)

	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "source", "nvim", "lua"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".config", "nvim", "lua"), 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "nvim", "lua", "init.lua"), []byte("line1\nline2\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "lua", "init.lua"), []byte("line1\nlineX\n"), 0o644))

	writeConfig(t, projectDir, []byte(`syncs:
  - target: .config/nvim
    source: source/nvim
`))

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"diff"})

	require.NoError(t, cmd.Execute(), stderr.String())
	body := stdout.String()
	require.Contains(t, body, "deploy-diff[1] (source -> target)")
	require.Contains(t, body, "--- target/lua/init.lua")
	require.Contains(t, body, "+++ source/lua/init.lua")
	require.Contains(t, body, "@@")
}

func TestDiffJSONMetadataWithoutPatchBodyByDefault(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := setTempHome(t)
	setCWD(t, projectDir)

	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "source", "nvim", "lua"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".config", "nvim", "lua"), 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "nvim", "lua", "init.lua"), []byte("source\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "nvim", "lua", "missing.lua"), []byte("remove\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "lua", "init.lua"), []byte("target\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "lua", "new.lua"), []byte("new\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "lua", "old.bak"), []byte("old\n"), 0o644))

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
        remove-missing:
          include:
            - 'lua/**'
`))

	payload := runJSONCommand(t, []string{"diff", "--json"})
	require.Equal(t, true, payload["ok"])
	require.Equal(t, "diff", payload["command"])
	require.Equal(t, "4.0", payload["schema_version"])

	sync := payload["syncs"].([]any)[0].(map[string]any)
	require.Equal(t, []string{"lua/init.lua", "lua/missing.lua"}, operationPaths(sync, "deploy"))
	require.Equal(t, []string{"lua/init.lua"}, operationPaths(sync, "import"))
	require.Equal(t, []string{"lua/new.lua", "lua/old.bak"}, operationPaths(sync, "incoming_unmanaged"))
	require.Equal(t, []string{"lua/old.bak"}, operationPaths(sync, "remove_unmanaged"))
	require.Equal(t, []string{"lua/missing.lua"}, operationPaths(sync, "remove_missing"))

	op := findOperation(sync, "deploy", "lua/init.lua")
	require.NotNil(t, op)
	require.Equal(t, "unified", op["diff_kind"])
	require.Equal(t, true, op["patch_available"])
	require.Equal(t, false, op["patch_included"])
	_, hasPatch := op["patch"]
	require.False(t, hasPatch)
}

func TestDiffJSONPatchFlagIncludesPatchBody(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := setTempHome(t)
	setCWD(t, projectDir)

	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "source", "nvim"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".config", "nvim"), 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "nvim", "init.lua"), []byte("source\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "init.lua"), []byte("target\n"), 0o644))

	writeConfig(t, projectDir, []byte(`syncs:
  - target: .config/nvim
    source: source/nvim
`))

	payload := runJSONCommand(t, []string{"diff", "--json", "--patch"})
	sync := payload["syncs"].([]any)[0].(map[string]any)
	op := findOperation(sync, "deploy", "init.lua")
	require.NotNil(t, op)
	require.Equal(t, true, op["patch_included"])
	patch, ok := op["patch"].(string)
	require.True(t, ok)
	require.Contains(t, patch, "--- target/init.lua")
	require.Contains(t, patch, "+++ source/init.lua")
}

func TestDiffDirectionFiltersPhases(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := setTempHome(t)
	setCWD(t, projectDir)

	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "source", "nvim", "lua"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".config", "nvim", "lua"), 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "nvim", "lua", "init.lua"), []byte("source\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "lua", "init.lua"), []byte("target\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "lua", "orphan.bak"), []byte("x\n"), 0o644))

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
`))

	deployPayload := runJSONCommand(t, []string{"diff", "--json", "--direction", "deploy"})
	deploySync := deployPayload["syncs"].([]any)[0].(map[string]any)
	require.NotEmpty(t, operationPaths(deploySync, "deploy"))
	require.NotEmpty(t, operationPaths(deploySync, "remove_unmanaged"))
	require.Empty(t, operationPaths(deploySync, "import"))
	require.Empty(t, operationPaths(deploySync, "incoming_unmanaged"))
	require.Empty(t, operationPaths(deploySync, "remove_missing"))

	importPayload := runJSONCommand(t, []string{"diff", "--json", "--direction", "import"})
	importSync := importPayload["syncs"].([]any)[0].(map[string]any)
	require.NotEmpty(t, operationPaths(importSync, "import"))
	require.NotEmpty(t, operationPaths(importSync, "incoming_unmanaged"))
	require.Empty(t, operationPaths(importSync, "deploy"))
	require.Empty(t, operationPaths(importSync, "remove_unmanaged"))
}

func TestDiffJSONReportsBinaryEntries(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := setTempHome(t)
	setCWD(t, projectDir)

	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "source", "nvim"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".config", "nvim"), 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "nvim", "cache.bin"), []byte{0x00, 0x01, 0x02}, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "cache.bin"), []byte{0x00, 0x04, 0x05}, 0o644))

	writeConfig(t, projectDir, []byte(`syncs:
  - target: .config/nvim
    source: source/nvim
`))

	payload := runJSONCommand(t, []string{"diff", "--json"})
	sync := payload["syncs"].([]any)[0].(map[string]any)
	op := findOperation(sync, "deploy", "cache.bin")
	require.NotNil(t, op)
	require.Equal(t, "binary", op["diff_kind"])
	require.Equal(t, "binary differs", op["reason"])
}

func TestDiffPatchFlagRequiresJSON(t *testing.T) {
	projectDir := t.TempDir()
	setTempHome(t)
	setCWD(t, projectDir)

	writeConfig(t, projectDir, []byte(`syncs:
  - target: .config/nvim
    source: source/nvim
`))

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"diff", "--patch", "--json"})

	// control: patch flag is valid with --json
	err := cmd.Execute()
	require.NoError(t, err)

	cmd = NewRootCmd()
	stdout.Reset()
	stderr.Reset()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"diff", "--patch", "~/.config/nvim"})
	err = cmd.Execute()
	require.Error(t, err)
	require.Contains(t, stderr.String(), "Flag not supported for command: --patch")
}

func TestDiffFlagValidationErrors(t *testing.T) {
	projectDir := t.TempDir()
	setTempHome(t)
	setCWD(t, projectDir)

	writeConfig(t, projectDir, []byte(`syncs:
  - target: .config/nvim
    source: source/nvim
`))

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"diff", "--json", "--direction", "sideways"})
	err := cmd.Execute()
	require.Error(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	errorObj := payload["error"].(map[string]any)
	require.Equal(t, "DFM_FLAG_INVALID_VALUE", errorObj["code"])

	cmd = NewRootCmd()
	stdout.Reset()
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"diff", "--json", "--context", "-1"})
	err = cmd.Execute()
	require.Error(t, err)

	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	errorObj = payload["error"].(map[string]any)
	require.Equal(t, "DFM_FLAG_INVALID_VALUE", errorObj["code"])
}

func TestDiffDryRunUnsupported(t *testing.T) {
	projectDir := t.TempDir()
	setTempHome(t)
	setCWD(t, projectDir)

	writeConfig(t, projectDir, []byte(`syncs:
  - target: .config/nvim
    source: source/nvim
`))

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"diff", "--json", "--dry-run"})

	err := cmd.Execute()
	require.Error(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	errorObj := payload["error"].(map[string]any)
	require.Equal(t, "DFM_FLAG_UNSUPPORTED", errorObj["code"])
}
