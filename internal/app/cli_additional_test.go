package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/shpoont/dotfiles-manager/internal/config"
	"github.com/shpoont/dotfiles-manager/internal/dfmerr"
	"github.com/stretchr/testify/require"
)

func TestStatusTextOutputAndStderrLogging(t *testing.T) {
	tempDir := t.TempDir()
	setTempHome(t)
	oldWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	require.NoError(t, os.Chdir(tempDir))

	require.NoError(t, os.WriteFile(filepath.Join(tempDir, config.DefaultConfigFile), []byte("syncs:\n  - target: .config/zsh\n    source: zsh\n"), 0o644))

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"status"})

	require.NoError(t, cmd.Execute())
	require.Contains(t, stdout.String(), "sync[0] target=~/.config/zsh source=./zsh")
	require.Contains(t, stdout.String(), "summary")
	require.NotContains(t, stdout.String(), "deploy[0]")
	require.Empty(t, stderr.String())
}

func TestInvalidLogLevelWithJSONEnvelope(t *testing.T) {
	tempDir := t.TempDir()
	setTempHome(t)
	oldWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	require.NoError(t, os.Chdir(tempDir))

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"status", "--json", "--log-level", "verbose"})

	err = cmd.Execute()
	require.Error(t, err)
	require.Contains(t, stderr.String(), "Invalid value for --log-level")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	require.Equal(t, false, payload["ok"])
	errorObj := payload["error"].(map[string]any)
	require.Equal(t, "DFM_FLAG_INVALID_VALUE", errorObj["code"])
}

func TestDeployWithPathScopeValue(t *testing.T) {
	tempDir := t.TempDir()
	setTempHome(t)
	oldWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	require.NoError(t, os.Chdir(tempDir))

	require.NoError(t, os.WriteFile(filepath.Join(tempDir, config.DefaultConfigFile), []byte("syncs:\n  - target: .config/nvim\n    source: .config/nvim\n"), 0o644))

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"deploy", "--json", "~/.config/nvim"})

	require.NoError(t, cmd.Execute())
	var payload map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	scope := payload["path_scope"].(map[string]any)
	require.Equal(t, "~/.config/nvim", scope["input"])
	require.NotNil(t, scope["normalized"])
}

func TestDeployWithRelativePathScopeFromHome(t *testing.T) {
	tempDir := t.TempDir()
	homeDir := setTempHome(t)
	oldWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	require.NoError(t, os.Chdir(tempDir))

	require.NoError(t, os.WriteFile(filepath.Join(tempDir, config.DefaultConfigFile), []byte("syncs:\n  - target: .config/nvim\n    source: .config/nvim\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, ".config", "nvim"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".config", "nvim"), 0o755))

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"deploy", "--json", ".config/nvim"})

	require.NoError(t, cmd.Execute())

	var payload map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	require.Equal(t, true, payload["ok"])

	pathScope := payload["path_scope"].(map[string]any)
	require.Equal(t, ".config/nvim", pathScope["input"])
	require.Equal(t, filepath.Join(homeDir, ".config", "nvim"), pathScope["normalized"])
	require.Equal(t, []any{float64(0)}, pathScope["matched_sync_indexes"])
}

func TestMainWrapper(t *testing.T) {
	tempDir := t.TempDir()
	oldWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	require.NoError(t, os.Chdir(tempDir))

	require.NoError(t, os.WriteFile(filepath.Join(tempDir, config.DefaultConfigFile), []byte("syncs:\n  - target: .config/a\n    source: .config/a\n"), 0o644))

	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"dotfiles-manager", "status"}

	require.Equal(t, 0, Execute())
}

func TestExecuteReturnsOneOnError(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })

	os.Args = []string{"dotfiles-manager", "status", "--log-level", "verbose", "--json"}
	require.Equal(t, 1, Execute())
}

func TestMainUsesExitHook(t *testing.T) {
	tempDir := t.TempDir()
	oldWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	require.NoError(t, os.Chdir(tempDir))

	require.NoError(t, os.WriteFile(filepath.Join(tempDir, config.DefaultConfigFile), []byte("syncs:\n  - target: .config/a\n    source: .config/a\n"), 0o644))

	oldArgs := os.Args
	oldExit := osExit
	t.Cleanup(func() {
		os.Args = oldArgs
		osExit = oldExit
	})

	os.Args = []string{"dotfiles-manager", "status"}
	exitCode := -1
	osExit = func(code int) { exitCode = code }

	Main()
	require.Equal(t, 0, exitCode)
}

func TestEmitErrorTextAndJSONBranches(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	emitError(&stdout, &stderr, false, jsonContext{Command: "status"}, errors.New("plain error"))
	require.Contains(t, stderr.String(), "plain error")
	require.Empty(t, stdout.String())

	stdout.Reset()
	stderr.Reset()
	emitError(&stdout, &stderr, true, jsonContext{Command: "status"}, errors.New("plain error"))
	require.Contains(t, stdout.String(), "\"ok\":false")
	require.Contains(t, stdout.String(), "\"schema_version\":\"3.0\"")
	require.Contains(t, stderr.String(), "plain error")

	stdout.Reset()
	stderr.Reset()
	emitError(&stdout, &stderr, true, jsonContext{Command: "status"}, dfmerr.New(dfmerr.CodeScopeNoMatch, "No sync matched provided path", map[string]any{"path": "~/.config"}))
	require.Contains(t, stdout.String(), "DFM_SCOPE_NO_MATCH")
	require.Contains(t, stderr.String(), "No sync matched provided path")
}

func TestEmitErrorJSONIncludesPartialSummary(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := dfmerr.New(dfmerr.CodeIOWrite, "Write failed", map[string]any{"partial": true})
	emitError(&stdout, &stderr, true, jsonContext{Command: "deploy"}, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	require.Equal(t, map[string]any{"partial": true}, payload["summary"])
}
