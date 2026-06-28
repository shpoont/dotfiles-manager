package catalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestListSynthesizesBuiltInCatalogWithoutWritingState(t *testing.T) {
	repoRoot := t.TempDir()
	stateRoot := filepath.Join(t.TempDir(), "state")

	report, err := List(Options{RepoRoot: repoRoot, StateRoot: stateRoot})
	require.NoError(t, err)
	require.Equal(t, Schema, report.Schema)
	require.Equal(t, ListCommand, report.Command)
	require.Equal(t, "ok", report.Summary.Status)
	require.Equal(t, 1, report.Summary.Enabled)
	require.Len(t, report.Sources, 1)

	builtIn := report.Sources[0]
	require.Equal(t, BuiltInName, builtIn.Name)
	require.Equal(t, SourceKindBundled, builtIn.SourceKind)
	require.Equal(t, StatusEnabled, builtIn.Status)
	require.Equal(t, "org.dotfiles-manager.bundled", builtIn.CatalogID)
	require.Equal(t, "release-accepted", builtIn.SourceAcceptance)
	require.Equal(t, "not-required", builtIn.IntegrityState)
	require.Equal(t, "allowed", builtIn.WriteDefault)
	require.Equal(t, "release-only", builtIn.UpdatePolicy)
	require.NoFileExists(t, stateFilePath(stateRoot))

	text := Text(report)
	require.Contains(t, text, "Catalogs")
	require.Contains(t, text, "built-in")
	require.Contains(t, text, "ships with dotfiles-manager")
	require.Contains(t, text, "Local catalogs: none")
	require.Contains(t, text, "Remote catalogs: not supported yet")
	require.Contains(t, text, "No live settings were read or changed.")
}

func TestLocalCatalogLifecyclePersistsAndNeverDeletesSourceFolder(t *testing.T) {
	repoRoot := t.TempDir()
	stateRoot := filepath.Join(t.TempDir(), "state")
	catalogRoot := t.TempDir()
	catalogRootReal, err := filepath.EvalSymlinks(catalogRoot)
	require.NoError(t, err)
	writeCatalogRecipe(t, catalogRoot, "example-tool", "Example Tool", "config", "yaml-file", "config.yaml")
	writeCatalogRecipe(t, catalogRoot, "git", "Git Custom", "user.email", "yaml-file", "config.yaml")

	add, err := Add(AddOptions{Options: Options{RepoRoot: repoRoot, StateRoot: stateRoot}, Name: "personal", Path: catalogRoot})
	require.NoError(t, err)
	require.Equal(t, AddCommand, add.Command)
	require.Equal(t, "ok", add.Summary.Status)
	require.Equal(t, "personal", add.ChangedSource.Name)
	require.Equal(t, 2, add.Summary.ValidRecipes)
	require.Len(t, add.Validated, 2)
	require.Equal(t, "example-tool", add.Validated[0].TargetID)
	require.Equal(t, "local support", add.Validated[0].Role)
	require.Equal(t, "git", add.Validated[1].TargetID)
	require.Equal(t, "local candidate; built-in support remains the default", add.Validated[1].Role)
	require.Contains(t, Text(add), "Added local catalog: personal")
	require.Contains(t, Text(add), "Network: not used")

	list, err := List(Options{RepoRoot: repoRoot, StateRoot: stateRoot})
	require.NoError(t, err)
	require.Len(t, list.Sources, 2)
	personal := requireSource(t, list, "personal")
	require.Equal(t, StatusEnabled, personal.Status)
	require.Equal(t, SourceKindLocal, personal.SourceKind)
	require.Equal(t, catalogRootReal, personal.Path)
	require.Equal(t, 2, personal.ValidRecipes)

	discovery, err := Discover(Options{RepoRoot: repoRoot, StateRoot: stateRoot})
	require.NoError(t, err)
	example := requireDiscoveryApp(t, discovery, "example-tool")
	require.Equal(t, "personal", example.Default.SourceName)
	require.Equal(t, SourceKindLocal, example.Default.SourceKind)
	require.Equal(t, "requires-approval", example.Default.WriteAuthority)
	git := requireDiscoveryApp(t, discovery, "git")
	require.Equal(t, BuiltInName, git.Default.SourceName)
	require.Len(t, git.Candidates, 1)
	require.Equal(t, "personal", git.Candidates[0].SourceName)
	require.Equal(t, "candidate", git.Candidates[0].SelectedBy)

	disable, err := Disable(Options{RepoRoot: repoRoot, StateRoot: stateRoot}, "personal")
	require.NoError(t, err)
	require.Equal(t, StatusDisabled, disable.ChangedSource.Status)
	require.Equal(t, 2, disable.Summary.HiddenRecipes)
	require.Contains(t, Text(disable), "Disabled local catalog: personal")
	require.Contains(t, Text(disable), "Nothing was deleted.")
	require.DirExists(t, catalogRoot)

	disabledDiscovery, err := Discover(Options{RepoRoot: repoRoot, StateRoot: stateRoot})
	require.NoError(t, err)
	require.Nil(t, findDiscoveryApp(disabledDiscovery, "example-tool"))
	git = requireDiscoveryApp(t, disabledDiscovery, "git")
	require.Empty(t, git.Candidates)
	require.Len(t, disabledDiscovery.DisabledSources, 1)
	require.Equal(t, "personal", disabledDiscovery.DisabledSources[0].Name)

	enable, err := Enable(Options{RepoRoot: repoRoot, StateRoot: stateRoot}, "personal")
	require.NoError(t, err)
	require.Equal(t, StatusEnabled, enable.ChangedSource.Status)
	require.Contains(t, Text(enable), "Enabled local catalog: personal")

	remove, err := Remove(Options{RepoRoot: repoRoot, StateRoot: stateRoot}, "personal")
	require.NoError(t, err)
	require.Equal(t, RemoveCommand, remove.Command)
	require.Contains(t, Text(remove), "Removed local catalog: personal")
	require.Contains(t, Text(remove), "Nothing was deleted from that folder.")
	require.DirExists(t, catalogRoot)

	finalList, err := List(Options{RepoRoot: repoRoot, StateRoot: stateRoot})
	require.NoError(t, err)
	require.Nil(t, findSource(finalList, "personal"))
}

func TestAddRejectsRemoteSyntaxAndInvalidRecipesWithoutPersisting(t *testing.T) {
	repoRoot := t.TempDir()
	stateRoot := filepath.Join(t.TempDir(), "state")

	remote, err := Add(AddOptions{Options: Options{RepoRoot: repoRoot, StateRoot: stateRoot}, Name: "community", Path: "shpoont/custom-recipes"})
	require.Error(t, err)
	require.Equal(t, CodeRemoteUnsupported, reportErrorCode(remote))
	require.Contains(t, Text(remote), "Remote GitHub catalogs are not supported")
	require.NoFileExists(t, stateFilePath(stateRoot))

	brokenRoot := t.TempDir()
	brokenRecipeDir := filepath.Join(brokenRoot, "broken-tool")
	require.NoError(t, os.MkdirAll(brokenRecipeDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(brokenRecipeDir, "recipe.yaml"), []byte(`schema: dotfiles-manager.v2.recipe
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

	invalid, err := Add(AddOptions{Options: Options{RepoRoot: repoRoot, StateRoot: stateRoot}, Name: "broken", Path: brokenRoot})
	require.Error(t, err)
	require.Equal(t, CodeCatalogInvalid, reportErrorCode(invalid))
	require.Len(t, invalid.Invalid, 1)
	require.Equal(t, "broken-tool", invalid.Invalid[0].TargetID)
	require.Contains(t, strings.Join(invalid.Invalid[0].Errors, "\n"), "dangerousCommand")
	require.Contains(t, Text(invalid), "Catalog not added: broken")
	require.NoFileExists(t, stateFilePath(stateRoot))
}

func TestBuiltinLifecycleIsImmutableAndStateValidationFailsClosed(t *testing.T) {
	repoRoot := t.TempDir()
	stateRoot := filepath.Join(t.TempDir(), "state")

	for _, op := range []struct {
		name string
		fn   func() (*Report, error)
	}{
		{"disable", func() (*Report, error) {
			return Disable(Options{RepoRoot: repoRoot, StateRoot: stateRoot}, BuiltInName)
		}},
		{"enable", func() (*Report, error) { return Enable(Options{RepoRoot: repoRoot, StateRoot: stateRoot}, BuiltInName) }},
		{"remove", func() (*Report, error) { return Remove(Options{RepoRoot: repoRoot, StateRoot: stateRoot}, BuiltInName) }},
	} {
		t.Run(op.name, func(t *testing.T) {
			report, err := op.fn()
			require.Error(t, err)
			require.Equal(t, CodeBuiltInImmutable, reportErrorCode(report))
		})
	}

	require.NoError(t, os.MkdirAll(filepath.Dir(stateFilePath(stateRoot)), 0o755))
	require.NoError(t, os.WriteFile(stateFilePath(stateRoot), []byte(`schema: dotfiles-manager.v2.catalog-state
schemaVersion: 1
sources:
  - name: bad
    displayName: Bad
    sourceKind: mystery
    status: enabled
    path: /tmp/bad
`), 0o644))

	list, err := List(Options{RepoRoot: repoRoot, StateRoot: stateRoot})
	require.Error(t, err)
	require.Equal(t, CodeStateInvalid, reportErrorCode(list))
	require.Contains(t, list.Error.Message, "unknown sourceKind")

	require.NoError(t, os.WriteFile(stateFilePath(stateRoot), []byte(`schema: dotfiles-manager.v2.catalog-state
schemaVersion: 1
sources:
  - name: personal
    sourceId: local:other
    sourceKind: local
    catalogId: local.other
    displayName: Personal
    originUri: file:///tmp/personal
    status: enabled
    sourceAcceptance: user-accepted
    integrityState: not-required
    writeDefault: denied
    updatePolicy: manual
    path: /tmp/personal
    pinnedIdentity: /tmp/personal
`), 0o644))

	list, err = List(Options{RepoRoot: repoRoot, StateRoot: stateRoot})
	require.Error(t, err)
	require.Equal(t, CodeStateInvalid, reportErrorCode(list))
	require.Contains(t, list.Error.Message, "sourceId must match")

	require.NoError(t, os.WriteFile(stateFilePath(stateRoot), []byte(`schema: dotfiles-manager.v2.catalog-state
schemaVersion: 1
sources:
  - name: personal
    sourceId: local:personal
    sourceKind: local
    catalogId: local.personal
    displayName: Personal
    originUri: file://recipes
    status: enabled
    sourceAcceptance: user-accepted
    integrityState: not-required
    writeDefault: denied
    updatePolicy: manual
    path: recipes
    pinnedIdentity: recipes
`), 0o644))

	list, err = List(Options{RepoRoot: repoRoot, StateRoot: stateRoot})
	require.Error(t, err)
	require.Equal(t, CodeStateInvalid, reportErrorCode(list))
	require.Contains(t, list.Error.Message, "path must be absolute")
}

func TestSettingsFolderLocalRecipesAreSynthesizedAsLocalSource(t *testing.T) {
	repoRoot := t.TempDir()
	stateRoot := filepath.Join(t.TempDir(), "state")
	localRoot := filepath.Join(repoRoot, "recipes", "local")
	writeCatalogRecipe(t, localRoot, "cobona", "Cobona", "config", "yaml-file", "config.yaml")

	discovery, err := Discover(Options{RepoRoot: repoRoot, StateRoot: stateRoot})
	require.NoError(t, err)
	app := requireDiscoveryApp(t, discovery, "cobona")
	require.Equal(t, SettingsFolderName, app.Default.SourceName)
	require.Equal(t, "local", app.Default.SourceID)
	require.Equal(t, "local.settings-folder", app.Default.CatalogID)
	require.Equal(t, "recipe://local/cobona", app.Default.RecipeRef)

	list, err := List(Options{RepoRoot: repoRoot, StateRoot: stateRoot})
	require.NoError(t, err)
	source := requireSource(t, list, SettingsFolderName)
	require.True(t, source.SettingsFolder)
	require.Equal(t, 1, source.ValidRecipes)
	require.NoFileExists(t, stateFilePath(stateRoot))
}

func TestEnabledCatalogWithChangedInvalidRecipeFailsClosed(t *testing.T) {
	repoRoot := t.TempDir()
	stateRoot := filepath.Join(t.TempDir(), "state")
	catalogRoot := t.TempDir()
	writeCatalogRecipe(t, catalogRoot, "example-tool", "Example Tool", "config", "yaml-file", "config.yaml")
	_, err := Add(AddOptions{Options: Options{RepoRoot: repoRoot, StateRoot: stateRoot}, Name: "personal", Path: catalogRoot})
	require.NoError(t, err)

	writeCatalogRecipe(t, catalogRoot, "example-tool", "Example Tool", "config", "yaml-file", "config.yaml")
	path := filepath.Join(catalogRoot, "example-tool", "recipe.yaml")
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, append(body, []byte("dangerousCommand: rm -rf ~\n")...), 0o644))

	list, err := List(Options{RepoRoot: repoRoot, StateRoot: stateRoot})
	require.NoError(t, err)
	source := requireSource(t, list, "personal")
	require.Equal(t, StatusBlocked, source.Status)
	require.Equal(t, 1, source.InvalidRecipes)
	require.Contains(t, source.BlockedReason, "invalid support")

	discovery, err := Discover(Options{RepoRoot: repoRoot, StateRoot: stateRoot})
	require.NoError(t, err)
	require.Nil(t, findDiscoveryApp(discovery, "example-tool"))
	require.NotEmpty(t, discovery.Diagnostics)

	lookup, err := LookupTarget(Options{RepoRoot: repoRoot, StateRoot: stateRoot}, "example-tool", true)
	require.NoError(t, err)
	require.True(t, lookup.Found)
	require.False(t, lookup.Available)
	require.Equal(t, StatusBlocked, lookup.Source.Status)
	require.Contains(t, lookup.Source.BlockedReason, "invalid support")

	_, err = Disable(Options{RepoRoot: repoRoot, StateRoot: stateRoot}, "personal")
	require.NoError(t, err)
	enable, err := Enable(Options{RepoRoot: repoRoot, StateRoot: stateRoot}, "personal")
	require.Error(t, err)
	require.Equal(t, CodeCatalogInvalid, reportErrorCode(enable))
}

func TestRemovedCatalogTombstoneKeepsUnavailableLookupWithoutDiscovery(t *testing.T) {
	repoRoot := t.TempDir()
	stateRoot := filepath.Join(t.TempDir(), "state")
	catalogRoot := t.TempDir()
	writeCatalogRecipe(t, catalogRoot, "example-tool", "Example Tool", "config", "yaml-file", "config.yaml")
	_, err := Add(AddOptions{Options: Options{RepoRoot: repoRoot, StateRoot: stateRoot}, Name: "personal", Path: catalogRoot})
	require.NoError(t, err)
	_, err = Remove(Options{RepoRoot: repoRoot, StateRoot: stateRoot}, "personal")
	require.NoError(t, err)

	discovery, err := Discover(Options{RepoRoot: repoRoot, StateRoot: stateRoot})
	require.NoError(t, err)
	require.Nil(t, findDiscoveryApp(discovery, "example-tool"))

	lookup, err := LookupTarget(Options{RepoRoot: repoRoot, StateRoot: stateRoot}, "example-tool", true)
	require.NoError(t, err)
	require.True(t, lookup.Found)
	require.False(t, lookup.Available)
	require.Equal(t, StatusRemoved, lookup.Source.Status)
	require.Equal(t, "personal", lookup.Source.Name)
}

func TestLookupTargetBlocksAmbiguousLocalCatalogs(t *testing.T) {
	repoRoot := t.TempDir()
	stateRoot := filepath.Join(t.TempDir(), "state")
	first := t.TempDir()
	second := t.TempDir()
	writeCatalogRecipe(t, first, "example-tool", "Example Tool", "config", "yaml-file", "config.yaml")
	writeCatalogRecipe(t, second, "example-tool", "Example Tool", "config", "yaml-file", "config.yaml")
	_, err := Add(AddOptions{Options: Options{RepoRoot: repoRoot, StateRoot: stateRoot}, Name: "personal", Path: first})
	require.NoError(t, err)
	_, err = Add(AddOptions{Options: Options{RepoRoot: repoRoot, StateRoot: stateRoot}, Name: "team", Path: second})
	require.NoError(t, err)

	lookup, err := LookupTarget(Options{RepoRoot: repoRoot, StateRoot: stateRoot}, "example-tool", true)
	require.NoError(t, err)
	require.True(t, lookup.Found)
	require.True(t, lookup.Ambiguous)
	require.False(t, lookup.Available)
	require.Len(t, lookup.Candidates, 2)
}

func TestLookupTargetPreservesUnavailableSourceWhenKnownRecipeIsDeleted(t *testing.T) {
	repoRoot := t.TempDir()
	stateRoot := filepath.Join(t.TempDir(), "state")
	catalogRoot := t.TempDir()
	writeCatalogRecipe(t, catalogRoot, "example-tool", "Example Tool", "config", "yaml-file", "config.yaml")
	_, err := Add(AddOptions{Options: Options{RepoRoot: repoRoot, StateRoot: stateRoot}, Name: "personal", Path: catalogRoot})
	require.NoError(t, err)
	require.NoError(t, os.RemoveAll(filepath.Join(catalogRoot, "example-tool")))

	_, err = Disable(Options{RepoRoot: repoRoot, StateRoot: stateRoot}, "personal")
	require.NoError(t, err)
	lookup, err := LookupTarget(Options{RepoRoot: repoRoot, StateRoot: stateRoot}, "example-tool", true)
	require.NoError(t, err)
	require.True(t, lookup.Found)
	require.False(t, lookup.Available)
	require.Equal(t, "personal", lookup.Source.Name)
	require.Equal(t, StatusDisabled, lookup.Source.Status)
	require.Equal(t, "recipe://local/personal/example-tool", lookup.Candidate.RecipeRef)

	_, err = Enable(Options{RepoRoot: repoRoot, StateRoot: stateRoot}, "personal")
	require.NoError(t, err)
	_, err = Remove(Options{RepoRoot: repoRoot, StateRoot: stateRoot}, "personal")
	require.NoError(t, err)
	lookup, err = LookupTarget(Options{RepoRoot: repoRoot, StateRoot: stateRoot}, "example-tool", true)
	require.NoError(t, err)
	require.True(t, lookup.Found)
	require.False(t, lookup.Available)
	require.Equal(t, "personal", lookup.Source.Name)
	require.Equal(t, StatusRemoved, lookup.Source.Status)
}

func TestLookupTargetBlocksSettingsFolderAndExternalLocalAmbiguity(t *testing.T) {
	repoRoot := t.TempDir()
	stateRoot := filepath.Join(t.TempDir(), "state")
	settingsLocal := filepath.Join(repoRoot, "recipes", "local")
	writeCatalogRecipe(t, settingsLocal, "example-tool", "Example Tool", "config", "yaml-file", "config.yaml")
	external := t.TempDir()
	writeCatalogRecipe(t, external, "example-tool", "Example Tool", "config", "yaml-file", "config.yaml")
	_, err := Add(AddOptions{Options: Options{RepoRoot: repoRoot, StateRoot: stateRoot}, Name: "personal", Path: external})
	require.NoError(t, err)

	lookup, err := LookupTarget(Options{RepoRoot: repoRoot, StateRoot: stateRoot}, "example-tool", true)
	require.NoError(t, err)
	require.True(t, lookup.Found)
	require.True(t, lookup.Ambiguous)
	require.False(t, lookup.Available)
	require.Len(t, lookup.Candidates, 2)
}

func requireSource(t *testing.T, report *Report, name string) Source {
	t.Helper()
	source := findSource(report, name)
	require.NotNil(t, source, "source %s not found", name)
	return *source
}

func findSource(report *Report, name string) *Source {
	if report == nil {
		return nil
	}
	for i := range report.Sources {
		if report.Sources[i].Name == name {
			return &report.Sources[i]
		}
	}
	return nil
}

func requireDiscoveryApp(t *testing.T, discovery *Discovery, id string) EffectiveApp {
	t.Helper()
	app := findDiscoveryApp(discovery, id)
	require.NotNil(t, app, "app %s not found", id)
	return *app
}

func findDiscoveryApp(discovery *Discovery, id string) *EffectiveApp {
	if discovery == nil {
		return nil
	}
	for i := range discovery.Apps {
		if discovery.Apps[i].ID == id {
			return &discovery.Apps[i]
		}
	}
	return nil
}

func reportErrorCode(report *Report) string {
	if report == nil || report.Error == nil {
		return ""
	}
	return report.Error.Code
}

func writeCatalogRecipe(t *testing.T, root string, target string, display string, setting string, driver string, resourcePath string) {
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
    driver: ` + driver + `
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
