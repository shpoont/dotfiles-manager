package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCatalogCLIAddsLocalCatalogAndFeedsAppDiscovery(t *testing.T) {
	repoRoot := setupAddCLIRepo(t, []string{"global"}, nil)
	setCWD(t, repoRoot)
	setTempHome(t)
	catalogRoot := t.TempDir()
	writeCLICatalogRecipe(t, catalogRoot, "example-tool", "Example Tool", "config", "config.yaml")
	writeCLICatalogRecipe(t, catalogRoot, "git", "Git Custom", "user.email", "config.yaml")

	stdout, stderr, err := runDiscoveryTextCLI(t, []string{"catalog", "list"})
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Catalogs")
	require.Contains(t, stdout, "built-in")
	require.Contains(t, stdout, "Local catalogs: none")
	require.Contains(t, stdout, "Remote catalogs: not supported yet")

	stdout, stderr, err = runDiscoveryTextCLI(t, []string{"catalog", "add", catalogRoot, "--name", "personal"})
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Added local catalog: personal")
	require.Contains(t, stdout, "example-tool  local support")
	require.Contains(t, stdout, "git           local candidate; built-in support remains the default")
	require.Contains(t, stdout, "Network: not used")

	payload, _, stderr, err := runDiscoveryJSONCLI(t, []string{"catalog", "list", "--json"})
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "dotfiles-manager.v2.catalogs", payload["schema"])
	require.Equal(t, "catalog.list", payload["command"])
	require.Len(t, payload["sources"].([]any), 2)

	stdout, stderr, err = runDiscoveryTextCLI(t, []string{"list"})
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "example-tool")
	require.Contains(t, stdout, "personal")
	require.Contains(t, stdout, "personal  enabled  local catalog")
	require.Contains(t, stdout, "git")
	require.Contains(t, stdout, "also in personal; built-in remains default")
	require.Contains(t, stdout, "No live settings were read or changed.")
	require.Contains(t, stdout, "No stored settings were changed.")

	stdout, stderr, err = runDiscoveryTextCLI(t, []string{"explain", "example-tool"})
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Example Tool is supported by a local catalog.")
	require.Contains(t, stdout, "Source: local catalog personal")
	require.Contains(t, stdout, "example-tool:config")
	require.Contains(t, stdout, "Live location: $HOME/.example-tool/config.yaml")
	require.Contains(t, stdout, "Local support requires write approval")

	stdout, stderr, err = runDiscoveryTextCLI(t, []string{"explain", "git"})
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Git is supported by multiple sources.")
	require.Contains(t, stdout, "local catalog: personal")
	require.Contains(t, stdout, "Built-in support remains the default")

	stdout, stderr, err = runDiscoveryTextCLI(t, []string{"catalog", "disable", "personal"})
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Disabled local catalog: personal")
	require.Contains(t, stdout, "Nothing was deleted.")
	require.DirExists(t, catalogRoot)

	stdout, stderr, err = runDiscoveryTextCLI(t, []string{"list"})
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.NotContains(t, stdout, "example-tool  personal")
	require.Contains(t, stdout, "Disabled local catalogs:")
	require.Contains(t, stdout, "personal  2 hidden apps/candidates")

	stdout, stderr, err = runDiscoveryTextCLI(t, []string{"catalog", "enable", "personal"})
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Enabled local catalog: personal")

	stdout, stderr, err = runDiscoveryTextCLI(t, []string{"catalog", "remove", "personal"})
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Removed local catalog: personal")
	require.Contains(t, stdout, "Nothing was deleted from that folder.")
	require.DirExists(t, catalogRoot)
}

func TestCatalogCLIAddLocalCatalogAppFailsExplicitly(t *testing.T) {
	repoRoot := setupAddCLIRepo(t, []string{"global"}, nil)
	setCWD(t, repoRoot)
	setTempHome(t)
	catalogRoot := t.TempDir()
	writeCLICatalogRecipe(t, catalogRoot, "example-tool", "Example Tool", "config", "config.yaml")

	stdout, stderr, err := runDiscoveryTextCLI(t, []string{"catalog", "add", catalogRoot, "--name", "personal"})
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Added local catalog: personal")

	stdout, stderr, err = runDiscoveryTextCLI(t, []string{"add", "example-tool", "--yes"})
	require.Error(t, err)
	require.Contains(t, stderr, "cannot add local-catalog apps")
	require.Contains(t, stdout, "example-tool is available from local catalog \"personal\"")
	require.Contains(t, stdout, "cannot add local-catalog apps to the managed set yet")
	require.Contains(t, stdout, "No profile files changed.")
	require.Contains(t, stdout, "No live app config changed.")
	require.NotContains(t, stdout, "unknown recipe target")
}

func TestCatalogCLIRejectsRemoteSyntaxWithoutNetworkBehavior(t *testing.T) {
	projectDir := t.TempDir()
	setCWD(t, projectDir)
	setTempHome(t)

	stdout, stderr, err := runDiscoveryTextCLI(t, []string{"catalog", "add", "shpoont/custom-recipes"})
	require.Error(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Catalog not added: shpoont/custom-recipes")
	require.Contains(t, stdout, "Remote GitHub catalogs are not supported")

	stdout, stderr, err = runDiscoveryTextCLI(t, []string{"catalog", "add", "shpoont/custom-recipes", "--name", "community"})
	require.Error(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Catalog not added: community")
	require.Contains(t, stdout, "Remote GitHub catalogs are not supported")
	require.Contains(t, stdout, "dotfiles-manager catalog add ./custom-recipes --name personal")

	payload, stdout, stderr, err := runDiscoveryJSONCLI(t, []string{"catalog", "add", "shpoont/custom-recipes", "--name", "community", "--json"})
	require.Error(t, err)
	require.Empty(t, stderr)
	require.NotContains(t, stdout, "Remote GitHub catalogs are not supported in this version of dotfiles-manager.\n\nFor now")
	require.Equal(t, "dotfiles-manager.v2.catalogs", payload["schema"])
	require.Equal(t, "catalog.add", payload["command"])
	require.Equal(t, "catalog.remote.unsupported", payload["error"].(map[string]any)["code"])
}

func TestCatalogCLIRejectsInvalidLocalCatalogBeforePersisting(t *testing.T) {
	repoRoot := setupAddCLIRepo(t, []string{"global"}, nil)
	setCWD(t, repoRoot)
	setTempHome(t)
	catalogRoot := t.TempDir()
	brokenDir := filepath.Join(catalogRoot, "broken-tool")
	require.NoError(t, os.MkdirAll(brokenDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(brokenDir, "recipe.yaml"), []byte(`schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: broken-tool
displayName: Broken Tool
supportLevel: experimental
capability: read-write
dangerousCommand: rm -rf ~
locations:
  config:
    default: ~/.broken
settings:
  config:
    scopeDefault: user
    resource: config-file
resources:
  config-file:
    driver: yaml-file
    location: config
    path: config.yaml
    selector:
      path: [user, email]
`), 0o644))

	stdout, stderr, err := runDiscoveryTextCLI(t, []string{"catalog", "add", catalogRoot, "--name", "broken"})
	require.Error(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Catalog not added: broken")
	require.Contains(t, stdout, "dangerousCommand")

	payload, stdout, stderr, err := runDiscoveryJSONCLI(t, []string{"catalog", "list", "--json"})
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "dotfiles-manager.v2.catalogs", payload["schema"])
	require.NotContains(t, stdout, "broken")
	require.Len(t, payload["sources"].([]any), 1)
}

func TestCatalogCLISelectedLocalSupportShowsOriginBeforeWriteAndDisabledBlocker(t *testing.T) {
	repoRoot := setupAddCLIRepo(t, []string{"global"}, map[string]string{"global": `schema: dotfiles-manager.v2.profile-layer
schemaVersion: 1
selections:
  example-tool:
    settings:
      config:
        scope: user
`})
	setCWD(t, repoRoot)
	setTempHome(t)
	catalogRoot := t.TempDir()
	writeCLICatalogRecipe(t, catalogRoot, "example-tool", "Example Tool", "config", "config.yaml")

	stdout, stderr, err := runDiscoveryTextCLI(t, []string{"catalog", "add", catalogRoot, "--name", "personal"})
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Added local catalog: personal")

	stdout, stderr, err = runDiscoveryTextCLI(t, []string{"status", "example-tool", "--user-id", "leon"})
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Example-tool config is blocked")
	require.Contains(t, stdout, "Source:")
	require.Contains(t, stdout, "local catalog personal")
	require.Contains(t, stdout, "recipe://local/personal/example-tool")
	require.Contains(t, stdout, "catalog-specific write approval")
	require.Contains(t, stdout, "No files changed.")

	stdout, stderr, err = runDiscoveryTextCLI(t, []string{"sync", "example-tool", "--user-id", "leon", "--non-interactive"})
	require.Error(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Source: local catalog personal")
	require.Contains(t, stdout, "Recipe: recipe://local/personal/example-tool")
	require.Contains(t, stdout, "catalog-specific write approval")

	stdout, stderr, err = runDiscoveryTextCLI(t, []string{"catalog", "disable", "personal"})
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Disabled local catalog: personal")

	stdout, stderr, err = runDiscoveryTextCLI(t, []string{"status", "example-tool", "--user-id", "leon"})
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Example-tool config is blocked")
	require.Contains(t, stdout, "local catalog personal")
	require.Contains(t, stdout, "catalog \"personal\", but that catalog is disabled or unavailable")
	require.Contains(t, stdout, "No files changed.")

	stdout, stderr, err = runDiscoveryTextCLI(t, []string{"catalog", "enable", "personal"})
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Enabled local catalog: personal")
	stdout, stderr, err = runDiscoveryTextCLI(t, []string{"catalog", "remove", "personal"})
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Removed local catalog: personal")
	stdout, stderr, err = runDiscoveryTextCLI(t, []string{"status", "example-tool", "--user-id", "leon"})
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "catalog \"personal\", but that catalog is disabled or unavailable")
}

func TestCatalogCLIRequiresSettingsFolderForLocalMutation(t *testing.T) {
	projectDir := t.TempDir()
	setCWD(t, projectDir)
	setTempHome(t)
	catalogRoot := t.TempDir()
	writeCLICatalogRecipe(t, catalogRoot, "example-tool", "Example Tool", "config", "config.yaml")

	stdout, stderr, err := runDiscoveryTextCLI(t, []string{"catalog", "add", catalogRoot, "--name", "personal"})
	require.Error(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "catalog add requires a v2 settings storage folder")
}

func TestCatalogCLIBlocksAmbiguousLocalCatalogSelection(t *testing.T) {
	repoRoot := setupAddCLIRepo(t, []string{"global"}, map[string]string{"global": `schema: dotfiles-manager.v2.profile-layer
schemaVersion: 1
selections:
  example-tool:
    settings:
      config:
        scope: user
`})
	setCWD(t, repoRoot)
	setTempHome(t)
	first := t.TempDir()
	second := t.TempDir()
	writeCLICatalogRecipe(t, first, "example-tool", "Example Tool", "config", "config.yaml")
	writeCLICatalogRecipe(t, second, "example-tool", "Example Tool", "config", "config.yaml")
	stdout, stderr, err := runDiscoveryTextCLI(t, []string{"catalog", "add", first, "--name", "personal"})
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Added local catalog: personal")
	stdout, stderr, err = runDiscoveryTextCLI(t, []string{"catalog", "add", second, "--name", "team"})
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Added local catalog: team")

	stdout, stderr, err = runDiscoveryTextCLI(t, []string{"list"})
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "example-tool  multiple")
	require.Contains(t, stdout, "available in personal; choose a source before writes")
	require.Contains(t, stdout, "available in team; choose a source before writes")
	require.NotContains(t, stdout, "example-tool  personal")
	require.NotContains(t, stdout, "built-in remains default")

	stdout, stderr, err = runDiscoveryTextCLI(t, []string{"explain", "example-tool"})
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Example Tool is supported by multiple local catalogs.")
	require.Contains(t, stdout, "Source: choose one local catalog")
	require.Contains(t, stdout, "Multiple local catalogs provide support")
	require.NotContains(t, stdout, "Can manage from the default source")

	stdout, stderr, err = runDiscoveryTextCLI(t, []string{"status", "example-tool", "--user-id", "leon"})
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Multiple local catalogs provide support for example-tool")
	require.Contains(t, stdout, "choose one source")
}

func writeCLICatalogRecipe(t *testing.T, root string, target string, display string, setting string, resourcePath string) {
	t.Helper()
	body := `schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: ` + target + `
displayName: ` + display + `
supportLevel: experimental
capability: read-write
locations:
  config:
    default: ~/.` + target + `
settings:
  ` + setting + `:
    label: ` + display + ` setting
    supportLevel: experimental
    capability: read-write
    artifactForm: scalar
    scopeDefault: user
    resource: config-resource
resources:
  config-resource:
    driver: yaml-file
    location: config
    path: ` + resourcePath + `
    selector:
      path: [user, email]
      createMissing: create
      duplicatePolicy: reject
`
	path := filepath.Join(root, target, "recipe.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}
