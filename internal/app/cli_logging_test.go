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

func TestStatusJSONLogsIncludeComponentAndRedactConfigPath(t *testing.T) {
	projectDir := t.TempDir()
	setTempHome(t)
	setCWD(t, projectDir)

	configPath := filepath.Join(projectDir, "secret-token-config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`syncs:
  - target: .config/zsh
    source: zsh
`), 0o644))

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"status", "--config", configPath, "--log-format", "json", "--log-level", "debug"})

	require.NoError(t, cmd.Execute(), stderr.String())
	require.Contains(t, stdout.String(), "status: syncs=1")
	require.NotContains(t, stderr.String(), "secret-token")

	entries := parseJSONLogLines(t, stderr.String())
	require.NotEmpty(t, entries)

	start := findLogByMessage(entries, "command.start")
	require.NotNil(t, start)
	require.Equal(t, "cli", start["component"])
	require.Equal(t, "status", start["command"])
	require.Equal(t, false, start["dry_run"])

	resolved := findLogByMessage(entries, "config.resolved")
	require.NotNil(t, resolved)
	require.Equal(t, logging.RedactedValue, resolved["config_path"])

	complete := findLogByMessage(entries, "command.complete")
	require.NotNil(t, complete)
	require.Equal(t, "cli", complete["component"])
}

func TestStatusJSONErrorLogsIncludeCode(t *testing.T) {
	projectDir := t.TempDir()
	setCWD(t, projectDir)

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"status", "--json", "--log-format", "json"})

	err := cmd.Execute()
	require.Error(t, err)

	entries := parseJSONLogLines(t, stderr.String())
	require.NotEmpty(t, entries)

	errorLog := findLogByMessage(entries, "command.error")
	require.NotNil(t, errorLog)
	require.Equal(t, "cli", errorLog["component"])
	require.Equal(t, "status", errorLog["command"])
	require.Equal(t, string(dfmerr.CodeConfigRequired), errorLog["error_code"])

	var payload map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	require.Equal(t, false, payload["ok"])
}

func TestLogCommandErrorBranches(t *testing.T) {
	var buffer bytes.Buffer
	logger, err := logging.New("json", "error", &buffer)
	require.NoError(t, err)

	logCommandError(nil, errors.New("ignored"))
	logCommandError(logger, nil)
	require.Equal(t, "", strings.TrimSpace(buffer.String()))

	logCommandError(logger, errors.New("plain failure"))
	entries := parseJSONLogLines(t, buffer.String())
	require.Len(t, entries, 1)
	require.Equal(t, "command.error", entries[0]["msg"])
	require.Equal(t, "", entries[0]["error_code"])
	require.Equal(t, "plain failure", entries[0]["error_message"])

	buffer.Reset()
	logCommandError(logger, dfmerr.New(dfmerr.CodeIOWrite, "Write failed: token=abc", nil))
	entries = parseJSONLogLines(t, buffer.String())
	require.Len(t, entries, 1)
	require.Equal(t, string(dfmerr.CodeIOWrite), entries[0]["error_code"])
	require.Equal(t, logging.RedactedValue, entries[0]["error_message"])
}

func parseJSONLogLines(t *testing.T, raw string) []map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	entries := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry), line)
		entries = append(entries, entry)
	}
	return entries
}

func findLogByMessage(entries []map[string]any, message string) map[string]any {
	for _, entry := range entries {
		if entry["msg"] == message {
			return entry
		}
	}
	return nil
}
