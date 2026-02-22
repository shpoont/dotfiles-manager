package app

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func runExecuteWithArgs(t *testing.T, args []string) (int, string, string) {
	t.Helper()

	oldArgs := os.Args
	oldStdout := executeStdout
	oldStderr := executeStderr
	defer func() {
		os.Args = oldArgs
		executeStdout = oldStdout
		executeStderr = oldStderr
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	executeStdout = &stdout
	executeStderr = &stderr
	os.Args = append([]string{"dotfiles-manager"}, args...)

	exitCode := Execute()
	return exitCode, stdout.String(), stderr.String()
}

func TestExecuteUnknownFlagTextGoesToStderr(t *testing.T) {
	exitCode, stdout, stderr := runExecuteWithArgs(t, []string{"status", "--bogus"})

	require.Equal(t, 1, exitCode)
	require.Empty(t, stdout)
	require.Contains(t, stderr, "unknown flag: --bogus")
}

func TestExecuteUnknownFlagWithJSONWritesParserEnvelopeToStdout(t *testing.T) {
	exitCode, stdout, stderr := runExecuteWithArgs(t, []string{"status", "--json", "--bogus"})

	require.Equal(t, 1, exitCode)
	require.Empty(t, stderr)

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, false, payload["ok"])
	require.Equal(t, "4.0", payload["schema_version"])
	require.Equal(t, "status", payload["command"])

	errorObj := payload["error"].(map[string]any)
	require.Equal(t, "DFM_PARSER_UNKNOWN_FLAG", errorObj["code"])
	require.Contains(t, errorObj["message"], "unknown flag: --bogus")

	details := errorObj["details"].(map[string]any)
	require.Equal(t, "--bogus", details["flag"])
}

func TestExecuteUnknownCommandTextGoesToStderr(t *testing.T) {
	exitCode, stdout, stderr := runExecuteWithArgs(t, []string{"sttaus"})

	require.Equal(t, 1, exitCode)
	require.Empty(t, stdout)
	require.Contains(t, stderr, `unknown command "sttaus"`)
}

func TestExecuteParserArgFailureTextGoesToStderr(t *testing.T) {
	exitCode, stdout, stderr := runExecuteWithArgs(t, []string{"status", "one", "two"})

	require.Equal(t, 1, exitCode)
	require.Empty(t, stdout)
	require.Contains(t, stderr, "accepts at most 1 arg(s), received 2")
}

func TestExecuteParserJSONEnvelopePropagatesConfigFlag(t *testing.T) {
	exitCode, stdout, stderr := runExecuteWithArgs(t, []string{"status", "--config", "/tmp/custom.yaml", "--json", "--bogus"})

	require.Equal(t, 1, exitCode)
	require.Empty(t, stderr)

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "/tmp/custom.yaml", payload["config_path"])

	errorObj := payload["error"].(map[string]any)
	require.Equal(t, "DFM_PARSER_UNKNOWN_FLAG", errorObj["code"])
}

func TestExecuteRuntimeValidationBehaviorIsUnchanged(t *testing.T) {
	exitCode, stdout, stderr := runExecuteWithArgs(t, []string{"status", "--json", "--log-level", "verbose"})

	require.Equal(t, 1, exitCode)
	require.Contains(t, stderr, "Invalid value for --log-level")

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, false, payload["ok"])
	errorObj := payload["error"].(map[string]any)
	require.Equal(t, "DFM_FLAG_INVALID_VALUE", errorObj["code"])
}
