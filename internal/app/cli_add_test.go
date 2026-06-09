package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAddGitYesJSONWritesProfileSelections(t *testing.T) {
	repoRoot := setupAddCLIRepo(t, []string{"global"}, map[string]string{"global": addCLIEmptyLayer()})
	setCWD(t, repoRoot)

	payload, stdout, stderr, err := runAddCLI(t, []string{"add", "git", "--yes", "--json"}, "")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.NotContains(t, stdout, "@example.com")
	require.Equal(t, "dotfiles-manager.v2.preview", payload["schema"])
	require.Equal(t, "add", payload["command"])
	add := payload["add"].(map[string]any)
	require.Equal(t, "global", add["destinationProfileLayer"])
	settings := add["settings"].([]any)
	require.Len(t, settings, 2)
	require.Equal(t, "git:user.email", settings[0].(map[string]any)["ref"])
	require.Equal(t, "user", settings[0].(map[string]any)["scope"])
	require.NotContains(t, settings[0].(map[string]any), "artifact")
	summary := payload["summary"].(map[string]any)
	require.Equal(t, "changed", summary["status"])
	require.Equal(t, float64(2), summary["written"])

	body := readCLIFile(t, filepath.Join(repoRoot, "profiles", "layers", "global.yaml"))
	require.Contains(t, body, "user.email:")
	require.Contains(t, body, "user.name:")
	require.NotContains(t, body, "artifact:")
}

func TestAddJSONDoesNotPromptAndReportsMissingProfileChoice(t *testing.T) {
	repoRoot := setupAddCLIRepo(t, []string{"global", "local"}, map[string]string{"global": addCLIEmptyLayer(), "local": addCLIEmptyLayer()})
	setCWD(t, repoRoot)

	payload, stdout, stderr, err := runAddCLI(t, []string{"add", "git", "--json"}, "")
	require.Error(t, err)
	require.Empty(t, stderr)
	require.NotContains(t, stdout, "Choose profile")
	require.Equal(t, "add", payload["command"])
	require.Equal(t, "blocked", payload["summary"].(map[string]any)["status"])
	require.Equal(t, "add.choice-required", payload["error"].(map[string]any)["code"])
	missing := payload["add"].(map[string]any)["missingChoices"].([]any)
	require.Equal(t, "profile", missing[0].(map[string]any)["kind"])
}

func TestAddInteractivePromptsForProfileAndSettings(t *testing.T) {
	repoRoot := setupAddCLIRepo(t, []string{"global", "local"}, map[string]string{"global": addCLIEmptyLayer(), "local": addCLIEmptyLayer()})
	setCWD(t, repoRoot)

	_, stdout, stderr, err := runAddCLI(t, []string{"add", "git"}, "2\nuser.email\n")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Choose profile layer")
	require.Contains(t, stdout, "Choose settings to manage")
	require.Contains(t, stdout, "profile layer: local")

	global := readCLIFile(t, filepath.Join(repoRoot, "profiles", "layers", "global.yaml"))
	local := readCLIFile(t, filepath.Join(repoRoot, "profiles", "layers", "local.yaml"))
	require.NotContains(t, global, "git:")
	require.Contains(t, local, "user.email:")
	require.NotContains(t, local, "user.name:")
}

func TestAddExplicitScopeAndFileArtifactJSON(t *testing.T) {
	repoRoot := setupAddCLIRepo(t, []string{"global"}, map[string]string{"global": addCLIEmptyLayer()})
	setCWD(t, repoRoot)

	payload, _, stderr, err := runAddCLI(t, []string{"add", "ssh", "--setting", "config", "--scope", "machine-user", "--json"}, "")
	require.NoError(t, err)
	require.Empty(t, stderr)
	settings := payload["add"].(map[string]any)["settings"].([]any)
	require.Len(t, settings, 1)
	setting := settings[0].(map[string]any)
	require.Equal(t, "ssh:config", setting["ref"])
	require.Equal(t, "machine-user", setting["scope"])
	require.Equal(t, "artifacts/config", setting["artifact"])
	require.Equal(t, "Me on this machine", setting["scopeLabel"])

	body := readCLIFile(t, filepath.Join(repoRoot, "profiles", "layers", "global.yaml"))
	require.Contains(t, body, "scope: machine-user")
	require.Contains(t, body, "artifact: artifacts/config")
}

func runAddCLI(t *testing.T, args []string, stdin string) (map[string]any, string, string, error) {
	t.Helper()
	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs(args)
	err := cmd.Execute()
	var payload map[string]any
	if strings.Contains(strings.Join(args, " "), "--json") {
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload), "stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
	return payload, stdout.String(), stderr.String(), err
}

func setupAddCLIRepo(t *testing.T, stack []string, layers map[string]string) string {
	t.Helper()
	repoRoot := t.TempDir()
	writeCLIFile(t, filepath.Join(repoRoot, "dotfiles-manager.v2.yaml"), "schema: dotfiles-manager.v2.root-config\nschemaVersion: 1\nactiveProfileStack: default\n")
	stackBody := "schema: dotfiles-manager.v2.profile-stack\nschemaVersion: 1\nprofileStack:\n"
	for _, layer := range stack {
		stackBody += "  - " + layer + "\n"
	}
	writeCLIFile(t, filepath.Join(repoRoot, "profiles", "stacks", "default.yaml"), stackBody)
	for _, layer := range stack {
		body := layers[layer]
		if body == "" {
			body = addCLIEmptyLayer()
		}
		writeCLIFile(t, filepath.Join(repoRoot, "profiles", "layers", filepath.FromSlash(layer)+".yaml"), body)
	}
	return repoRoot
}

func addCLIEmptyLayer() string {
	return "schema: dotfiles-manager.v2.profile-layer\nschemaVersion: 1\nselections: {}\n"
}

func readCLIFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return strings.ReplaceAll(string(data), "\r\n", "\n")
}
