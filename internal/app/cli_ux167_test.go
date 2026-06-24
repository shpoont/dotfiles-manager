package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestV2SetupRecipeListAndBackupDefaultUXIsReadable(t *testing.T) {
	repoRoot := t.TempDir()
	home := setTempHome(t)
	binDir := filepath.Join(t.TempDir(), "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	writeCLIFile(t, filepath.Join(binDir, "git"), "#!/bin/sh\nexit 0\n")
	require.NoError(t, os.Chmod(filepath.Join(binDir, "git"), 0o755))
	t.Setenv("PATH", binDir)
	writeCLIFile(t, filepath.Join(home, ".gitconfig"), "[user]\n\temail = private@example.com\n[credential]\n\thelper = credential-helper-secret\n")
	setCWD(t, repoRoot)

	initOut := runUX167Text(t, []string{"init", "--yes", "--machine-id", "mbp", "--user-id", "leon"})
	require.Contains(t, initOut, "Initialized dotfiles-manager v2 workspace.")
	require.Contains(t, initOut, "Machine: mbp")
	require.Contains(t, initOut, "User: leon")

	discoverOut := runUX167Text(t, []string{"recipe", "discover", "git"})
	require.Contains(t, discoverOut, "Discover supported app settings")
	require.Contains(t, discoverOut, "$HOME/.gitconfig — present")
	require.Contains(t, discoverOut, "git:user.email — User email")

	explainOut := runUX167Text(t, []string{"recipe", "explain", "git"})
	require.Contains(t, explainOut, "Git recipe")
	require.Contains(t, explainOut, "What it can manage:")
	require.Contains(t, explainOut, "Not managed:")

	addDryRunOut := runUX167Text(t, []string{"add", "git", "--setting", "user.email", "--scope", "user", "--dry-run", "--yes"})
	require.Contains(t, addDryRunOut, "Preview: select Git settings.")
	require.Contains(t, addDryRunOut, "No profile files will be changed")
	require.Contains(t, addDryRunOut, "  git:user.email — User email")
	require.NotContains(t, addDryRunOut, "Would select: git:user.email")
	require.Contains(t, addDryRunOut, "No live app config was changed")
	require.Contains(t, addDryRunOut, "dotfiles-manager --config dotfiles-manager.v2.yaml add git --setting user.email --scope user --profile global --yes")

	addOut := runUX167Text(t, []string{"add", "git", "--setting", "user.email", "--scope", "user", "--yes"})
	require.Contains(t, addOut, "Selected Git settings.")
	require.Contains(t, addOut, "No live app config was changed")
	require.Contains(t, addOut, "Preview explicit sync from live settings to stored settings")

	listBeforeSave := runUX167Text(t, []string{"list", "--user-id", "leon"})
	require.Contains(t, listBeforeSave, "Selected settings")
	require.Contains(t, listBeforeSave, "git:user.email — User email")
	require.Contains(t, listBeforeSave, "Stored settings: not stored yet")

	_ = runUX167Text(t, []string{"save", "--yes", "--user-id", "leon", "git:user.email"})
	desiredPath := filepath.Join(repoRoot, "desired", "user", "leon", "targets", "git", "settings.yaml")
	writeCLIFile(t, desiredPath, "schema: dotfiles-manager.v2.desired-settings\nschemaVersion: 1\nvalues:\n  user.email:\n    intent: set\n    kind: string\n    value: desired@example.com\n")

	listAfterSave := runUX167Text(t, []string{"list", "--user-id", "leon"})
	require.Contains(t, listAfterSave, "Stored settings: stored")
	require.Contains(t, listAfterSave, "Inspect drift:")

	applyOut := runUX167Text(t, []string{"apply", "--yes", "--user-id", "leon", "git:user.email"})
	require.Contains(t, applyOut, "Git user email")

	backupList := runUX167Text(t, []string{"backup", "list"})
	require.Contains(t, backupList, "Backups")
	require.Contains(t, backupList, "Can restore:")
	require.Contains(t, backupList, "git:user.email — User email")
	require.Contains(t, backupList, "Preview restore:")
	require.Contains(t, backupList, "Backup payload contents are stored for restore but are not printed.")
	require.Contains(t, backupList, "Use --verbose for technical details or --json for machine-readable output.")

	backupJSON := runUX167JSON(t, []string{"backup", "list", "--json"})
	backups := backupJSON["backups"].([]any)
	require.NotEmpty(t, backups)
	runID := backups[0].(map[string]any)["runId"].(string)
	backupShow := runUX167Text(t, []string{"backup", "show", runID})
	require.Contains(t, backupShow, "Backup "+runID)
	require.Contains(t, backupShow, "To the value from before the apply run.")

	defaultTranscript := strings.Join([]string{initOut, discoverOut, explainOut, addDryRunOut, addOut, listBeforeSave, listAfterSave, backupList, backupShow}, "\n---\n")
	for _, forbidden := range []string{"state://identity", "desired://", "recipe://", "resource=", "driver=", "sourceLayer=", "selector=", "credential-helper-secret", "private@example.com", "desired@example.com"} {
		require.NotContains(t, defaultTranscript, forbidden)
	}

	verboseTranscript := strings.Join([]string{
		runUX167Text(t, []string{"init", "--verbose", "--yes", "--machine-id", "mbp", "--user-id", "leon"}),
		runUX167Text(t, []string{"recipe", "discover", "git", "--verbose"}),
		runUX167Text(t, []string{"recipe", "explain", "git", "--verbose"}),
		runUX167Text(t, []string{"add", "git", "--setting", "user.email", "--scope", "user", "--dry-run", "--yes", "--verbose"}),
		runUX167Text(t, []string{"list", "--user-id", "leon", "--verbose"}),
		runUX167Text(t, []string{"backup", "list", "--verbose"}),
		runUX167Text(t, []string{"backup", "show", runID, "--verbose"}),
	}, "\n---\n")
	for _, expected := range []string{"state://identity", "recipe://bundled/git", "resource=", "driver=", "sourceLayer=", "selector="} {
		require.Contains(t, verboseTranscript, expected)
	}
	require.NotContains(t, verboseTranscript, "credential-helper-secret")
	require.NotContains(t, verboseTranscript, "private@example.com")

	for _, args := range [][]string{
		{"init", "--json", "--yes", "--machine-id", "mbp", "--user-id", "leon"},
		{"recipe", "discover", "git", "--json"},
		{"recipe", "explain", "git", "--json"},
		{"add", "git", "--setting", "user.email", "--scope", "user", "--dry-run", "--yes", "--json"},
		{"list", "--user-id", "leon", "--json"},
		{"backup", "list", "--json"},
		{"backup", "show", runID, "--json"},
	} {
		payload := runUX167JSON(t, args)
		require.NotEmpty(t, payload["schema"], args)
	}
	for _, args := range [][]string{
		{"init", "--json", "--verbose", "--yes", "--machine-id", "mbp", "--user-id", "leon"},
		{"recipe", "discover", "git", "--json", "--verbose"},
		{"recipe", "explain", "git", "--json", "--verbose"},
		{"add", "git", "--setting", "user.email", "--scope", "user", "--dry-run", "--yes", "--json", "--verbose"},
		{"list", "--user-id", "leon", "--json", "--verbose"},
		{"backup", "list", "--json", "--verbose"},
		{"backup", "show", runID, "--json", "--verbose"},
	} {
		payload, stdout := runUX167JSONRaw(t, args)
		require.NotEmpty(t, payload["schema"], args)
		require.NotContains(t, stdout, "Selected settings", args)
		require.NotContains(t, stdout, "Discover supported app settings", args)
		require.NotContains(t, stdout, "resource=", args)
	}
}

func runUX167Text(t *testing.T, args []string) string {
	t.Helper()
	stdout, stderr, err := runSelectedPreviewTextCLI(t, args)
	require.NoError(t, err, "args=%v stdout=%s stderr=%s", args, stdout, stderr)
	require.Empty(t, stderr, "args=%v", args)
	return stdout
}

func runUX167JSON(t *testing.T, args []string) map[string]any {
	t.Helper()
	payload, _ := runUX167JSONRaw(t, args)
	return payload
}

func runUX167JSONRaw(t *testing.T, args []string) (map[string]any, string) {
	t.Helper()
	stdout, stderr, err := runSelectedPreviewTextCLI(t, args)
	require.NoError(t, err, "args=%v stdout=%s stderr=%s", args, stdout, stderr)
	require.Empty(t, stderr, "args=%v", args)
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload), "args=%v stdout=%s", args, stdout)
	return payload, stdout
}
