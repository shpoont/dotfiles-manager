package app

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	v2appauthor "github.com/shpoont/dotfiles-manager/internal/v2/appauthor"
	"github.com/stretchr/testify/require"
)

func TestAppHelpOnlyRegistersCreateAndValidate(t *testing.T) {
	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"app", "--help"})

	err := cmd.Execute()
	require.NoError(t, err)
	require.Empty(t, stderr.String())
	text := stdout.String()
	require.Contains(t, text, "create")
	require.Contains(t, text, "validate")
	require.NotContains(t, text, "edit")
	require.NotContains(t, text, "test")
}

func TestAppCreateAndValidateJSONThroughRootCLI(t *testing.T) {
	repoRoot := setupAppCLIRepo(t)
	setCWD(t, repoRoot)

	payload, stdout, stderr, err := runRootJSONCLI(t, []string{
		"app", "create", "local-cli-demo",
		"--template", "selected-value",
		"--from-path", "~/.config/demo/config.yaml",
		"--setting", "user.email",
		"--setting-label", "User email",
		"--driver", "yaml-file",
		"--selector", "user.email",
		"--scope-default", "machine-user",
		"--lifecycle", "allowed",
		"--json",
	}, "")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, v2appauthor.CreateSchema, payload["schema"])
	require.Equal(t, "changed", payload["summary"].(map[string]any)["status"])
	require.NotContains(t, stdout, repoRoot)
	require.FileExists(t, filepath.Join(repoRoot, "recipes/local/local-cli-demo/recipe.yaml"))

	payload, stdout, stderr, err = runRootJSONCLI(t, []string{"app", "validate", "local-cli-demo", "--json"}, "")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, v2appauthor.ValidateSchema, payload["schema"])
	require.Equal(t, "ok", payload["summary"].(map[string]any)["status"])
	trust := payload["appValidate"].(map[string]any)["trust"].(map[string]any)
	require.Equal(t, "not-checked", trust["localTrustState"])
	require.Equal(t, true, trust["writeTrustRequired"])
	require.NotEmpty(t, trust["writeSurfaceFingerprint"])
	require.NotContains(t, stdout, repoRoot)
}

func TestAppCreateDryRunAndCollisionJSON(t *testing.T) {
	repoRoot := setupAppCLIRepo(t)
	setCWD(t, repoRoot)

	payload, _, stderr, err := runRootJSONCLI(t, []string{
		"app", "create", "local-dry-cli",
		"--template", "file",
		"--from-path", ".config/demo/config.yaml",
		"--setting", "config",
		"--setting-label", "Config file",
		"--scope-default", "user",
		"--lifecycle", "allowed",
		"--dry-run",
		"--json",
	}, "")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "ok", payload["summary"].(map[string]any)["status"])
	require.NoDirExists(t, filepath.Join(repoRoot, "recipes/local/local-dry-cli"))

	payload, _, stderr, err = runRootJSONCLI(t, []string{
		"app", "create", "gitconfig",
		"--template", "file",
		"--from-path", ".config/demo/config.yaml",
		"--setting", "config",
		"--setting-label", "Config file",
		"--scope-default", "user",
		"--lifecycle", "allowed",
		"--json",
	}, "")
	require.Error(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "blocked", payload["summary"].(map[string]any)["status"])
	require.Equal(t, v2appauthor.CodeTargetCollision, payload["error"].(map[string]any)["code"])
}

func TestAppValidateJSONReportsRecipeDiagnostics(t *testing.T) {
	repoRoot := setupAppCLIRepo(t)
	setCWD(t, repoRoot)
	writeCLIFile(t, filepath.Join(repoRoot, "recipes/local/local-bad/recipe.yaml"), `schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: local-bad
displayName: Local Bad
supportLevel: experimental
capability: read-write
unknownField: nope
settings: {}
resources: {}
`)

	payload, _, stderr, err := runRootJSONCLI(t, []string{"app", "validate", "local-bad", "--json"}, "")
	require.Error(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "blocked", payload["summary"].(map[string]any)["status"])
	require.Equal(t, v2appauthor.CodeRecipeInvalid, payload["error"].(map[string]any)["code"])
	diagnostics := payload["diagnostics"].([]any)
	require.NotEmpty(t, diagnostics)
	joined := strings.ToLower(stdoutLikeDiagnostics(diagnostics))
	require.Contains(t, joined, "unknownfield")
}

func setupAppCLIRepo(t *testing.T) string {
	t.Helper()
	repoRoot := t.TempDir()
	writeCLIFile(t, filepath.Join(repoRoot, "dotfiles-manager.v2.yaml"), "schema: dotfiles-manager.v2.root-config\nschemaVersion: 1\nactiveProfileStack: default\n")
	return repoRoot
}

func stdoutLikeDiagnostics(values []any) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		if object, ok := value.(map[string]any); ok {
			parts = append(parts, object["code"].(string)+" "+object["message"].(string))
		}
	}
	return strings.Join(parts, "\n")
}
