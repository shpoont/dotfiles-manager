package app

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDiscoveryListBeforeInitIsSupportedAppsAndReadOnly(t *testing.T) {
	projectDir := t.TempDir()
	setCWD(t, projectDir)
	setTempHome(t)

	stdout, stderr, err := runDiscoveryTextCLI(t, []string{"list"})
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Supported apps")
	require.Contains(t, stdout, "git")
	require.Contains(t, stdout, "official")
	require.Contains(t, stdout, "not managed")
	require.Contains(t, stdout, "dotfiles-manager explain <app>")
	require.NotContains(t, stdout, "built-in")
	require.NotContains(t, stdout, "custom.files")
	require.NotContains(t, stdout, "No live settings were read or changed.")
	require.NotContains(t, stdout, "No stored settings were changed.")
	requireNoDiscoveryState(t, projectDir)
}

func TestDiscoveryListJSONShowsManagedStateWithoutOldListSchema(t *testing.T) {
	repoRoot := setupAddCLIRepo(t, []string{"global"}, map[string]string{"global": `schema: dotfiles-manager.v2.profile-layer
schemaVersion: 1
selections:
  git:
    settings:
      user.email:
        scope: user
      user.name:
        scope: user
`})
	setCWD(t, repoRoot)
	setTempHome(t)

	payload, stdout, stderr, err := runDiscoveryJSONCLI(t, []string{"list", "--json", "--user-id", "leon"})
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.NotContains(t, stdout, "Selected settings")
	require.Equal(t, "dotfiles-manager.v2.apps", payload["schema"])
	require.Equal(t, "list", payload["command"])
	summary := payload["summary"].(map[string]any)
	require.Equal(t, "ok", summary["status"])
	require.Equal(t, float64(6), summary["apps"])
	require.Equal(t, float64(1), summary["managed"])

	git := requireDiscoveryApp(t, payload, "git")
	require.Equal(t, "official", git["source"])
	require.Equal(t, "managed", git["state"])
	require.Equal(t, float64(2), git["selectedSettings"])
	require.Equal(t, "recipe://bundled/git", git["recipeRef"])

	zsh := requireDiscoveryApp(t, payload, "zsh")
	require.Equal(t, "not-managed", zsh["state"])
	require.Equal(t, float64(0), zsh["selectedSettings"])
	requireNoDiscoveryApp(t, payload, "custom.files")
}

func TestListSettingsCompatibilityKeepsPreviousSelectedSettingsOutput(t *testing.T) {
	repoRoot := setupAddCLIRepo(t, []string{"global"}, map[string]string{"global": `schema: dotfiles-manager.v2.profile-layer
schemaVersion: 1
selections:
  git:
    settings:
      user.email:
        scope: user
`})
	setCWD(t, repoRoot)

	stdout, stderr, err := runDiscoveryTextCLI(t, []string{"list", "--settings", "--user-id", "leon"})
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Selected settings")
	require.Contains(t, stdout, "git:user.email — User email")
	require.NotContains(t, stdout, "Supported apps")

	payload, _, stderr, err := runDiscoveryJSONCLI(t, []string{"list", "--settings", "--json", "--user-id", "leon"})
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "dotfiles-manager.v2.list", payload["schema"])
	require.Equal(t, "list", payload["command"])
}

func TestDiscoverySearchMatchesAndNoMatches(t *testing.T) {
	projectDir := t.TempDir()
	setCWD(t, projectDir)
	setTempHome(t)

	stdout, stderr, err := runDiscoveryTextCLI(t, []string{"search", "git"})
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, `Search results for "git"`)
	require.Contains(t, stdout, "  APP  CATALOG   STATE\n  git  official  not managed")
	require.Contains(t, stdout, "git")
	require.Contains(t, stdout, "official")
	require.Contains(t, stdout, "not managed")
	require.Contains(t, stdout, "dotfiles-manager explain git")
	require.NotContains(t, stdout, "No live settings were read or changed.")
	require.NotContains(t, stdout, "No stored settings were changed.")

	stdout, stderr, err = runDiscoveryTextCLI(t, []string{"search", "wezterm"})
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, `No supported apps found for "wezterm".`)
	require.Contains(t, stdout, "The current official catalog supports:")
	require.Contains(t, stdout, "git, nvim, ssh, starship, tmux, zsh")
	require.Contains(t, stdout, "Future versions may refresh official support data or add remote catalogs.")
	require.NotContains(t, stdout, "catalog add")
	require.NotContains(t, stdout, "local catalog")
	require.NotContains(t, stdout, "No live settings were read or changed.")
	require.NotContains(t, stdout, "No stored settings were changed.")

	payload, _, stderr, err := runDiscoveryJSONCLI(t, []string{"search", "wezterm", "--json"})
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "dotfiles-manager.v2.apps", payload["schema"])
	require.Equal(t, "search", payload["command"])
	require.Equal(t, "app-search", payload["runId"])
	summary := payload["summary"].(map[string]any)
	require.Equal(t, "ok", summary["status"])
	require.Equal(t, float64(0), summary["apps"])
	require.Equal(t, float64(0), summary["managed"])
	require.Equal(t, float64(0), summary["matches"])
	require.Equal(t, float64(0), summary["failed"])
	require.Empty(t, payload["apps"].([]any))
	requireNoDiscoveryState(t, projectDir)
}

func TestDiscoverySearchJSONEmptyQueryHasStableError(t *testing.T) {
	projectDir := t.TempDir()
	setCWD(t, projectDir)
	setTempHome(t)

	payload, _, stderr, err := runDiscoveryJSONCLI(t, []string{"search", "--json"})
	require.Error(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "dotfiles-manager.v2.apps", payload["schema"])
	require.Equal(t, "search", payload["command"])
	require.Equal(t, "error", payload["summary"].(map[string]any)["status"])
	require.Equal(t, float64(1), payload["summary"].(map[string]any)["failed"])
	require.Equal(t, "search.query.invalid", payload["error"].(map[string]any)["code"])
	requireNoDiscoveryState(t, projectDir)
}

func TestTopLevelExplainIsAppOrientedAndUnknownAppIsStable(t *testing.T) {
	projectDir := t.TempDir()
	setCWD(t, projectDir)
	setTempHome(t)

	stdout, stderr, err := runDiscoveryTextCLI(t, []string{"explain", "git"})
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Git is supported.")
	require.Contains(t, stdout, "App ID: git")
	require.Contains(t, stdout, "Catalog: official")
	require.Contains(t, stdout, "State: not managed")
	require.Contains(t, stdout, "Can manage:")
	require.Contains(t, stdout, "git:user.email")
	require.Contains(t, stdout, "Does not manage:")
	require.Contains(t, stdout, "credential.helper")
	require.NotContains(t, stdout, "Source: built-in")
	require.NotContains(t, stdout, "Why this source is used:")
	require.NotContains(t, stdout, "No live values were printed.")
	require.NotContains(t, stdout, "No live settings were changed.")
	require.NotContains(t, stdout, "No stored settings were changed.")
	require.NotContains(t, stdout, "Git recipe")
	require.NotContains(t, stdout, "recipe explain")

	payload, _, stderr, err := runDiscoveryJSONCLI(t, []string{"explain", "git", "--json"})
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "dotfiles-manager.v2.app", payload["schema"])
	require.Equal(t, "explain", payload["command"])
	app := payload["app"].(map[string]any)
	require.Equal(t, "git", app["id"])
	require.Equal(t, "official", app["source"])

	stdout, stderr, err = runDiscoveryTextCLI(t, []string{"explain", "missing"})
	require.Error(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "App not supported: missing")
	require.Contains(t, stdout, "dotfiles-manager search missing")
	require.NotContains(t, stdout, "No live settings were read or changed.")
	require.NotContains(t, stdout, "No stored settings were changed.")

	payload, _, stderr, err = runDiscoveryJSONCLI(t, []string{"explain", "missing", "--json"})
	require.Error(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "dotfiles-manager.v2.app", payload["schema"])
	require.Equal(t, "explain", payload["command"])
	require.Equal(t, "error", payload["summary"].(map[string]any)["status"])
	require.Equal(t, "explain.app.notSupported", payload["error"].(map[string]any)["code"])

	stdout, stderr, err = runDiscoveryTextCLI(t, []string{"explain", "custom.files"})
	require.Error(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "App not supported: custom.files")
	require.NotContains(t, stdout, "Custom files is supported.")

	stdout, stderr, err = runDiscoveryTextCLI(t, []string{"explain", "custom-files"})
	require.Error(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "App not supported: custom-files")
	require.NotContains(t, stdout, "Custom files is supported.")

	requireNoDiscoveryState(t, projectDir)
}

func TestRootHelpSurfacesFlattenedDiscoveryBeforeRecipeNamespace(t *testing.T) {
	stdout, stderr, err := runDiscoveryTextCLI(t, []string{"--help"})
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "list")
	require.Contains(t, stdout, "search")
	require.Contains(t, stdout, "explain")
	require.Contains(t, stdout, "catalog")
	require.Contains(t, stdout, "recipe")
	require.Less(t, strings.Index(stdout, "list"), strings.Index(stdout, "recipe"))
	require.NotContains(t, stdout, "recipe list")
}

func TestCatalogListShowsOfficialCatalogState(t *testing.T) {
	projectDir := t.TempDir()
	setCWD(t, projectDir)
	setTempHome(t)

	stdout, stderr, err := runDiscoveryTextCLI(t, []string{"catalog", "list"})
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Catalogs")
	require.Contains(t, stdout, "Catalogs define app/tool support; they do not store your settings.")
	require.Contains(t, stdout, "dotfiles-manager/official  active for discovery")
	require.Contains(t, stdout, "Catalog version: 9f2c7a1")
	require.Contains(t, stdout, "Catalog updated: 2026-06-30 18:00 UTC")
	require.NotContains(t, stdout, "Source:")
	require.NotContains(t, stdout, "Local copy:")
	require.NotContains(t, stdout, "Offline use:")
	require.NotContains(t, stdout, "Updates:")
	require.NotContains(t, stdout, "Removable:")
	require.NotContains(t, stdout, "catalog update")
	require.NotContains(t, stdout, "catalog add")
	requireNoDiscoveryState(t, projectDir)

	payload, _, stderr, err := runDiscoveryJSONCLI(t, []string{"catalog", "list", "--json"})
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "dotfiles-manager.v2.catalogs", payload["schema"])
	require.Equal(t, "catalog.list", payload["command"])
	catalogs := payload["catalogs"].([]any)
	require.Len(t, catalogs, 1)
	official := catalogs[0].(map[string]any)
	require.Equal(t, "dotfiles-manager/official", official["id"])
	require.Equal(t, "active for discovery", official["state"])
	require.Equal(t, "9f2c7a1", official["version"])
	require.Equal(t, "2026-06-30 18:00 UTC", official["updated"])

	stdout, stderr, err = runDiscoveryTextCLI(t, []string{"catalog", "list", "--help"})
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.NotContains(t, stdout, "--verbose")

	stdout, stderr, err = runDiscoveryTextCLI(t, []string{"catalog", "list", "--verbose"})
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.Contains(t, err.Error(), "unknown flag: --verbose")
}

func TestCatalogLifecycleCommandsAreUnsupportedIn228(t *testing.T) {
	projectDir := t.TempDir()
	setCWD(t, projectDir)
	setTempHome(t)

	for _, args := range [][]string{
		{"catalog", "add", "shpoont/custom-recipes"},
		{"catalog", "update"},
	} {
		stdout, stderr, err := runDiscoveryTextCLI(t, args)
		require.Error(t, err, args)
		require.Empty(t, stdout, args)
		require.Empty(t, stderr, args)
		require.Contains(t, err.Error(), "unknown command", args)
	}
	requireNoDiscoveryState(t, projectDir)
}

func runDiscoveryTextCLI(t *testing.T, args []string) (string, string, error) {
	t.Helper()
	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func runDiscoveryJSONCLI(t *testing.T, args []string) (map[string]any, string, string, error) {
	t.Helper()
	stdout, stderr, err := runDiscoveryTextCLI(t, args)
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload), "stdout=%s stderr=%s", stdout, stderr)
	return payload, stdout, stderr, err
}

func requireDiscoveryApp(t *testing.T, payload map[string]any, id string) map[string]any {
	t.Helper()
	apps := payload["apps"].([]any)
	for _, raw := range apps {
		app := raw.(map[string]any)
		if app["id"] == id {
			return app
		}
	}
	require.Failf(t, "missing app", "app %s not found in %#v", id, apps)
	return nil
}

func requireNoDiscoveryApp(t *testing.T, payload map[string]any, id string) {
	t.Helper()
	apps := payload["apps"].([]any)
	for _, raw := range apps {
		app := raw.(map[string]any)
		require.NotEqual(t, id, app["id"])
	}
}

func requireNoDiscoveryState(t *testing.T, root string) {
	t.Helper()
	require.NoFileExists(t, filepath.Join(root, "dotfiles-manager.v2.yaml"))
	require.NoDirExists(t, filepath.Join(root, "profiles"))
	require.NoDirExists(t, filepath.Join(root, "desired"))
	require.NoDirExists(t, filepath.Join(root, "state"))
	require.NoDirExists(t, filepath.Join(root, "migrations"))
}
