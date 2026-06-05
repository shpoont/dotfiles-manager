package recipe

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadCustomFilesRecipeAcceptsSingleFileResourceUnderNamedLocation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeRecipe(t, root, validCustomFilesRecipe("config.yaml"))

	rec, err := LoadCustomFiles(root)
	require.NoError(t, err)
	require.Equal(t, CustomFilesTarget, rec.Target)
	require.Equal(t, "read-write", rec.Capability)
	resourceID, resource, err := rec.ResourceForSetting("file")
	require.NoError(t, err)
	require.Equal(t, "config-file", resourceID)
	require.Equal(t, FileDriverID, resource.Driver)
	require.Equal(t, "config", resource.Location)
	require.Equal(t, "config.yaml", resource.Path)
}

func TestRecipeRejectsUnknownFieldsAndInvalidReferences(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name:    "unknown top-level field",
			body:    validCustomFilesRecipe("config.yaml") + "unexpected: true\n",
			wantErr: "field unexpected",
		},
		{
			name:    "unknown setting resource",
			body:    replace(validCustomFilesRecipe("config.yaml"), "resource: config-file", "resource: missing"),
			wantErr: "references unknown resource missing",
		},
		{
			name:    "unknown resource location",
			body:    replace(validCustomFilesRecipe("config.yaml"), "location: config", "location: missing"),
			wantErr: "references unknown location missing",
		},
		{
			name:    "unsupported capability",
			body:    replace(validCustomFilesRecipe("config.yaml"), "capability: read-write", "capability: command-io"),
			wantErr: "unsupported capability",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeRecipe(t, root, tc.body)
			_, err := LoadCustomFiles(root)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestCustomFilesRecipeIsIntentionallyNarrow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name:    "wrong target",
			body:    replace(validCustomFilesRecipe("config.yaml"), "target: custom.files", "target: other.files"),
			wantErr: "target must be",
		},
		{
			name:    "driver must be file",
			body:    replace(validCustomFilesRecipe("config.yaml"), "driver: file", "driver: yaml-file"),
			wantErr: "driver must be \"file\"",
		},
		{
			name:    "exactly one resource",
			body:    validCustomFilesRecipe("config.yaml") + "  other:\n    driver: file\n    location: config\n    path: other.txt\n",
			wantErr: "exactly one resource",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeRecipe(t, root, tc.body)
			_, err := LoadCustomFiles(root)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestValidateResourcePathRejectsTraversalAndNonCanonicalPaths(t *testing.T) {
	t.Parallel()

	tests := []string{
		"../escape.yaml",
		"/tmp/escape.yaml",
		`nested\\escape.yaml`,
		"nested/./escape.yaml",
		"nested//escape.yaml",
		`" escape.yaml"`,
	}

	for _, value := range tests {
		t.Run(value, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeRecipe(t, root, validCustomFilesRecipe(value))
			_, err := LoadCustomFiles(root)
			require.Error(t, err)
			require.Contains(t, err.Error(), "path")
		})
	}
}

func writeRecipe(t *testing.T, root string, body string) {
	t.Helper()
	path := filepath.Join(root, "recipes", "local", CustomFilesTarget, "recipe.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func validCustomFilesRecipe(resourcePath string) string {
	return `schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: custom.files
displayName: Custom files
supportLevel: experimental
capability: read-write
locations:
  config:
    default: ~/.cobona
settings:
  file:
    scopeDefault: user
    resource: config-file
resources:
  config-file:
    driver: file
    location: config
    path: ` + resourcePath + `
`
}

func replace(value string, old string, new string) string {
	return strings.ReplaceAll(value, old, new)
}

func TestRecipeValidationErrorBranches(t *testing.T) {
	t.Parallel()

	rec := func(mut func(*Recipe)) *Recipe {
		r := &Recipe{
			Schema:        Schema,
			SchemaVersion: SupportedVersion,
			Target:        CustomFilesTarget,
			DisplayName:   "Custom files",
			SupportLevel:  "experimental",
			Capability:    "read-write",
			Locations:     map[string]Location{"config": {Default: "~/.cobona"}},
			Settings:      map[string]Setting{"file": {ScopeDefault: "user", Resource: "config-file"}},
			Resources:     map[string]Resource{"config-file": {Driver: "file", Location: "config", Path: "config.yaml"}},
		}
		mut(r)
		return r
	}

	tests := []struct {
		name    string
		recipe  *Recipe
		wantErr string
	}{
		{name: "nil", recipe: nil, wantErr: "recipe is required"},
		{name: "schema", recipe: rec(func(r *Recipe) { r.Schema = "wrong" }), wantErr: "invalid recipe schema"},
		{name: "version", recipe: rec(func(r *Recipe) { r.SchemaVersion = 2 }), wantErr: "invalid recipe schemaVersion"},
		{name: "target", recipe: rec(func(r *Recipe) { r.Target = "Bad" }), wantErr: "invalid target id"},
		{name: "display", recipe: rec(func(r *Recipe) { r.DisplayName = " " }), wantErr: "displayName"},
		{name: "support", recipe: rec(func(r *Recipe) { r.SupportLevel = "unsafe" }), wantErr: "supportLevel"},
		{name: "locations", recipe: rec(func(r *Recipe) { r.Locations = nil }), wantErr: "at least one location"},
		{name: "resources", recipe: rec(func(r *Recipe) { r.Resources = nil }), wantErr: "at least one resource"},
		{name: "settings", recipe: rec(func(r *Recipe) { r.Settings = nil }), wantErr: "at least one setting"},
		{name: "location id", recipe: rec(func(r *Recipe) { r.Locations = map[string]Location{"Bad": {Default: "/tmp"}} }), wantErr: "invalid location id"},
		{name: "location default blank", recipe: rec(func(r *Recipe) { r.Locations["config"] = Location{Default: " "} }), wantErr: "default is required"},
		{name: "location default nul", recipe: rec(func(r *Recipe) { r.Locations["config"] = Location{Default: "bad\x00"} }), wantErr: "contains NUL"},
		{name: "resource id", recipe: rec(func(r *Recipe) {
			r.Resources = map[string]Resource{"Bad": {Driver: "file", Location: "config", Path: "config.yaml"}}
		}), wantErr: "invalid resource id"},
		{name: "resource driver", recipe: rec(func(r *Recipe) { r.Resources["config-file"] = Resource{Location: "config", Path: "config.yaml"} }), wantErr: "driver is required"},
		{name: "resource location", recipe: rec(func(r *Recipe) { r.Resources["config-file"] = Resource{Driver: "file", Path: "config.yaml"} }), wantErr: "location is required"},
		{name: "setting id", recipe: rec(func(r *Recipe) {
			r.Settings = map[string]Setting{"Bad": {ScopeDefault: "user", Resource: "config-file"}}
		}), wantErr: "invalid setting id"},
		{name: "setting scope", recipe: rec(func(r *Recipe) { r.Settings["file"] = Setting{ScopeDefault: "planet", Resource: "config-file"} }), wantErr: "unsupported scopeDefault"},
		{name: "setting resource required", recipe: rec(func(r *Recipe) { r.Settings["file"] = Setting{ScopeDefault: "user"} }), wantErr: "resource is required"},
		{name: "custom capability", recipe: rec(func(r *Recipe) { r.Capability = "read-only" }), wantErr: "capability must be read-write"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.recipe.Validate()
			if tc.name == "custom capability" {
				err = tc.recipe.ValidateCustomFiles()
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestRecipePathAndLocationHelpers(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeRecipe(t, root, validCustomFilesRecipe("config.yaml"))
	rec, err := LoadCustomFiles(root)
	require.NoError(t, err)

	override, err := rec.LocationRoot("config", map[string]string{"config": filepath.Join(root, "override")})
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "override"), override)

	defaultRoot, err := rec.LocationRoot("config", nil)
	require.NoError(t, err)
	require.NotEmpty(t, defaultRoot)

	_, err = rec.LocationRoot("missing", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown location")

	_, _, err = (*Recipe)(nil).ResourceForSetting("file")
	require.Error(t, err)
	require.Contains(t, err.Error(), "recipe is required")

	_, _, err = rec.ResourceForSetting("missing")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no setting")

	home, err := os.UserHomeDir()
	require.NoError(t, err)
	expanded, err := ExpandLocationDefault("~/Library/Test")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, "Library", "Test"), expanded)
	expanded, err = ExpandLocationDefault("~")
	require.NoError(t, err)
	require.Equal(t, home, expanded)
	_, err = ExpandLocationDefault(" ")
	require.Error(t, err)
	_, err = ExpandLocationDefault("bad\x00")
	require.Error(t, err)

	_, err = LoadLocal("", CustomFilesTarget)
	require.Error(t, err)
	_, err = LoadLocal(root, "../escape")
	require.Error(t, err)
	_, err = LoadLocal(root, "missing")
	require.Error(t, err)
}
