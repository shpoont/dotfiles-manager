package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shpoont/dotfiles-manager/internal/dfmerr"
	"github.com/shpoont/dotfiles-manager/internal/logging"
	"github.com/stretchr/testify/require"
)

func TestStatusLogsWrittenToFileAndRedactConfigPath(t *testing.T) {
	projectDir := t.TempDir()
	setTempHome(t)
	setCWD(t, projectDir)

	configPath := filepath.Join(projectDir, "secret-token-config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`syncs:
  - target: .config/zsh
    source: zsh
`), 0o644))

	logPath := filepath.Join(projectDir, "logs", "command.log")

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"status", "--config", configPath, "--log-file", logPath, "--log-level", "debug"})

	require.NoError(t, cmd.Execute(), stderr.String())
	require.Contains(t, stdout.String(), "sync[0] target=~/.config/zsh source=./zsh")
	require.Contains(t, stdout.String(), "summary")
	require.NotContains(t, stdout.String(), "deploy[0]")
	require.Empty(t, stderr.String())

	logBody := readLogFile(t, logPath)
	require.Contains(t, logBody, "msg=command.start")
	require.Contains(t, logBody, "component=cli")
	require.Contains(t, logBody, "command=status")
	require.Contains(t, logBody, "msg=config.resolved")
	require.Contains(t, logBody, "config_path="+logging.RedactedValue)
	require.Contains(t, logBody, "msg=command.complete")
	require.NotContains(t, logBody, "secret-token")
}

func TestDiffLogsWrittenToFile(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := setTempHome(t)
	setCWD(t, projectDir)

	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "source", "nvim"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".config", "nvim"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "source", "nvim", "init.lua"), []byte("source\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "init.lua"), []byte("target\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, ".dotfiles-manager.yaml"), []byte(`syncs:
  - target: .config/nvim
    source: source/nvim
`), 0o644))

	logPath := filepath.Join(projectDir, "logs", "diff.log")

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"diff", "--log-file", logPath, "--log-level", "debug"})

	require.NoError(t, cmd.Execute(), stderr.String())
	require.Contains(t, stdout.String(), "deploy-diff")
	require.Empty(t, stderr.String())

	logBody := readLogFile(t, logPath)
	require.Contains(t, logBody, "msg=command.start")
	require.Contains(t, logBody, "command=diff")
	require.Contains(t, logBody, "msg=command.complete")
}

func TestStatusJSONErrorLogsIncludeCode(t *testing.T) {
	projectDir := t.TempDir()
	setTempHome(t)
	setCWD(t, projectDir)

	logPath := filepath.Join(projectDir, "logs", "errors.log")

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"status", "--json", "--log-file", logPath})

	err := cmd.Execute()
	require.Error(t, err)
	require.Contains(t, stderr.String(), "Config not found")

	logBody := readLogFile(t, logPath)
	require.Contains(t, logBody, "msg=command.error")
	require.Contains(t, logBody, "error_code="+string(dfmerr.CodeConfigRequired))

	var payload map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	require.Equal(t, false, payload["ok"])
}

func TestLogCommandErrorBranches(t *testing.T) {
	var buffer bytes.Buffer
	logger, err := logging.New("error", &buffer)
	require.NoError(t, err)

	logCommandError(nil, errors.New("ignored"))
	logCommandError(logger, nil)
	require.Equal(t, "", strings.TrimSpace(buffer.String()))

	logCommandError(logger, errors.New("plain failure"))
	require.Contains(t, buffer.String(), "msg=command.error")
	require.Contains(t, buffer.String(), "error_code=")
	require.Contains(t, buffer.String(), "error_message=\"plain failure\"")

	buffer.Reset()
	logCommandError(logger, dfmerr.New(dfmerr.CodeIOWrite, "Write failed: token=abc", nil))
	require.Contains(t, buffer.String(), "error_code="+string(dfmerr.CodeIOWrite))
	require.Contains(t, buffer.String(), "error_message="+logging.RedactedValue)
}

func readLogFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(content)
}
