//go:build contract

package contract_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/shpoont/dotfiles-manager/internal/app"
	"github.com/shpoont/dotfiles-manager/internal/config"
	"github.com/stretchr/testify/require"
)

func TestContractStatusJSONGolden(t *testing.T) {
	projectDir, homeDir := setupSandbox(t)

	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "source", "nvim", "lua"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".config", "nvim", "lua"), 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "nvim", "lua", "init.lua"), []byte("source-init"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "nvim", "lua", "only-source.lua"), []byte("source-only"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "lua", "init.lua"), []byte("target-init"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "lua", "new.lua"), []byte("new"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "lua", "old.bak"), []byte("bak"), 0o644))

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

	payload := runJSONCommand(t, []string{"status", "--json"})
	canonicalizePayload(payload)
	assertGolden(t, "status.json", payload)
}

func TestContractDeployJSONGolden(t *testing.T) {
	projectDir, homeDir := setupSandbox(t)

	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "source", "nvim", "lua"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".config", "nvim", "lua"), 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "nvim", "lua", "init.lua"), []byte("source-init"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "nvim", "lua", "only-source.lua"), []byte("source-only"), 0o644))
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
	canonicalizePayload(payload)
	assertGolden(t, "deploy-dry-run.json", payload)
}

func TestContractImportJSONGolden(t *testing.T) {
	projectDir, homeDir := setupSandbox(t)

	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "source", "nvim", "lua"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".config", "nvim", "lua"), 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "nvim", "lua", "init.lua"), []byte("source-init"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "nvim", "lua", "missing.lua"), []byte("remove"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "lua", "init.lua"), []byte("target-init"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "lua", "new.lua"), []byte("new"), 0o644))

	writeConfig(t, projectDir, []byte(`syncs:
  - target: .config/nvim
    source: source/nvim
    on:
      import:
        add-unmanaged:
          include:
            - '**'
        remove-missing:
          include:
            - 'lua/**'
`))

	payload := runJSONCommand(t, []string{"import", "--dry-run", "--json"})
	canonicalizePayload(payload)
	assertGolden(t, "import-dry-run.json", payload)
}

func TestContractDiffJSONGolden(t *testing.T) {
	projectDir, homeDir := setupSandbox(t)

	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "source", "nvim", "lua"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".config", "nvim", "lua"), 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "nvim", "lua", "init.lua"), []byte("source-init\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "nvim", "lua", "only-source.lua"), []byte("source-only\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "lua", "init.lua"), []byte("target-init\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "lua", "new.lua"), []byte("new\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "lua", "old.bak"), []byte("bak\n"), 0o644))

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
	canonicalizePayload(payload)
	assertGolden(t, "diff.json", payload)
}

func setupSandbox(t *testing.T) (string, string) {
	t.Helper()
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	oldWD, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(projectDir))
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	oldHome, hadHome := os.LookupEnv("HOME")
	require.NoError(t, os.Setenv("HOME", homeDir))
	t.Cleanup(func() {
		if hadHome {
			_ = os.Setenv("HOME", oldHome)
		} else {
			_ = os.Unsetenv("HOME")
		}
	})

	return projectDir, homeDir
}

func runJSONCommand(t *testing.T, args []string) map[string]any {
	t.Helper()
	cmd := app.NewRootCmd()
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

func canonicalizePayload(payload map[string]any) {
	payload["config_path"] = "<CONFIG_PATH>"

	pathScope := payload["path_scope"].(map[string]any)
	if pathScope["normalized"] != nil {
		pathScope["normalized"] = "<PATH_SCOPE>"
	}

	syncs := payload["syncs"].([]any)
	for idx, entry := range syncs {
		sync := entry.(map[string]any)
		sync["source_root"] = fmt.Sprintf("<SOURCE_ROOT_%d>", idx)
		sync["target_root"] = fmt.Sprintf("<TARGET_ROOT_%d>", idx)
	}
}

func writeConfig(t *testing.T, projectDir string, body []byte) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, config.DefaultConfigFile), body, 0o644))
}

func assertGolden(t *testing.T, filename string, payload map[string]any) {
	t.Helper()
	goldenPath := filepath.Join(repoRoot(t), "testdata", "expected", "contract", filename)
	expected, err := os.ReadFile(goldenPath)
	require.NoError(t, err)

	actual, err := json.MarshalIndent(payload, "", "  ")
	require.NoError(t, err)

	require.JSONEq(t, string(expected), string(actual))
}
