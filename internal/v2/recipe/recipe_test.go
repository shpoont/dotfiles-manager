package recipe

import (
	"encoding/json"
	"errors"
	"fmt"
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

func TestLoadCustomFilesRecipeAcceptsSingleFileTreeResourceUnderNamedLocation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeRecipe(t, root, validCustomFileTreeRecipe("profiles", []string{"**/*.json", "empty-dir"}, []string{"cache/**"}))

	rec, err := LoadCustomFiles(root)
	require.NoError(t, err)
	resourceID, resource, err := rec.ResourceForSetting("file")
	require.NoError(t, err)
	require.Equal(t, "config-file", resourceID)
	require.Equal(t, FileTreeDriverID, resource.Driver)
	require.Equal(t, "config", resource.Location)
	require.Equal(t, "profiles", resource.Path)
	require.Equal(t, []string{"**/*.json", "empty-dir"}, resource.Include)
	require.Equal(t, []string{"cache/**"}, resource.Exclude)
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

func TestCustomFilesRecipeRestrictsDriversButAllowsMultipleResources(t *testing.T) {
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
			name:    "driver must be file or file-tree",
			body:    replace(validCustomFilesRecipe("config.yaml"), "driver: file", "driver: yaml-file"),
			wantErr: "driver must be \"file\" or \"file-tree\"",
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

	root := t.TempDir()
	writeRecipe(t, root, validCustomFilesRecipe("config.yaml")+"  other:\n    driver: file\n    location: config\n    path: other.txt\n")
	rec, err := LoadCustomFiles(root)
	require.NoError(t, err)
	require.Len(t, rec.Resources, 2)
}

func TestCustomFilesRecipeValidatesFileTreeGlobs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name:    "file resource rejects include",
			body:    validCustomFilesRecipe("config.yaml") + "    include:\n      - \"**\"\n",
			wantErr: "must not declare include/exclude",
		},
		{
			name:    "file-tree rejects traversal glob",
			body:    validCustomFileTreeRecipe("profiles", []string{"../escape"}, nil),
			wantErr: "unsafe segment",
		},
		{
			name:    "file-tree rejects absolute glob",
			body:    validCustomFileTreeRecipe("profiles", []string{"/escape"}, nil),
			wantErr: "must be relative",
		},
		{
			name:    "file-tree rejects backslash glob",
			body:    validCustomFileTreeRecipe("profiles", []string{`nested\\escape`}, nil),
			wantErr: "backslashes",
		},
		{
			name:    "file-tree rejects empty glob",
			body:    validCustomFileTreeRecipe("profiles", []string{""}, nil),
			wantErr: "glob is required",
		},
		{
			name:    "file-tree defaults include",
			body:    validCustomFileTreeRecipe("profiles", nil, []string{"cache/**"}),
			wantErr: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeRecipe(t, root, tc.body)
			_, err := LoadCustomFiles(root)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
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

func TestRecipeAcceptsAndValidatesINIFileResources(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeNamedRecipe(t, root, "test.ini", validINIRecipe())

	rec, err := LoadLocal(root, "test.ini")
	require.NoError(t, err)
	resourceID, resource, err := rec.ResourceForSetting("identity")
	require.NoError(t, err)
	require.Equal(t, "git-email", resourceID)
	require.Equal(t, IniFileDriverID, resource.Driver)
	require.Equal(t, ".gitconfig", resource.Path)
	require.NotNil(t, resource.Selector)
	require.Equal(t, "user", resource.Selector.Section)
	require.Equal(t, "email", resource.Selector.Key)
	require.Equal(t, "create", resource.Selector.MissingSection)
	require.Equal(t, "create", resource.Selector.MissingKey)
}

func TestRecipeAcceptsSettingsGroupsAndSelectedPathResources(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		recipeID   string
		body       string
		driver     string
		settingRef string
	}{
		{
			name:       "json selected scalar",
			recipeID:   "test.json",
			body:       validSelectedPathRecipe("test.json", JSONFileDriverID, "config.json"),
			driver:     JSONFileDriverID,
			settingRef: "identity.email",
		},
		{
			name:       "yaml selected scalar",
			recipeID:   "test.yaml",
			body:       validSelectedPathRecipe("test.yaml", YAMLFileDriverID, "config.yaml"),
			driver:     YAMLFileDriverID,
			settingRef: "identity.email",
		},
		{
			name:       "toml selected scalar",
			recipeID:   "test.toml",
			body:       validSelectedPathRecipe("test.toml", TOMLFileDriverID, "config.toml"),
			driver:     TOMLFileDriverID,
			settingRef: "identity.email",
		},
		{
			name:       "plist selected scalar",
			recipeID:   "test.plist",
			body:       validSelectedPathRecipe("test.plist", PlistFileDriverID, "config.plist"),
			driver:     PlistFileDriverID,
			settingRef: "identity.email",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeNamedRecipe(t, root, tc.recipeID, tc.body)

			rec, err := LoadLocal(root, tc.recipeID)
			require.NoError(t, err)
			require.Contains(t, rec.SettingsGroups, "identity")
			require.Equal(t, []string{tc.settingRef}, rec.SettingsGroups["identity"].Settings)
			_, resource, err := rec.ResourceForSetting(tc.settingRef)
			require.NoError(t, err)
			require.Equal(t, tc.driver, resource.Driver)
			require.Equal(t, []string{"user", "email"}, resource.Selector.Path)
			require.Equal(t, "create", resource.Selector.CreateMissing)
		})
	}
}

func TestRecipeRejectsInvalidSettingsGroups(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name:    "unknown setting ref",
			body:    strings.Replace(validSelectedPathRecipe("test.json", JSONFileDriverID, "config.json"), "      - identity.email", "      - missing.setting", 1),
			wantErr: "references unknown setting missing.setting",
		},
		{
			name:    "duplicate setting ref",
			body:    strings.Replace(validSelectedPathRecipe("test.json", JSONFileDriverID, "config.json"), "      - identity.email", "      - identity.email\n      - identity.email", 1),
			wantErr: "duplicates setting ref identity.email",
		},
		{
			name:    "unsupported group capability",
			body:    strings.Replace(validSelectedPathRecipe("test.json", JSONFileDriverID, "config.json"), "capability: read-write\n    settings:", "capability: command-io\n    settings:", 1),
			wantErr: "unsupported capability",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeNamedRecipe(t, root, "test.json", tc.body)
			_, err := LoadLocal(root, "test.json")
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestDecodeRejectsDuplicateYAMLIDsWithStableDiagnostics(t *testing.T) {
	t.Parallel()

	body := strings.Replace(validCustomFilesRecipe("config.yaml"), "settings:\n  file:\n", "settings:\n  file:\n    scopeDefault: user\n    resource: config-file\n  file:\n", 1)

	_, err := Decode("duplicate.yaml", strings.NewReader(body))
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate mapping key")

	diagnostics := ValidationDiagnostics(err)
	require.Len(t, diagnostics, 1)
	require.Equal(t, "yaml.duplicate-key", diagnostics[0].Code)
	require.Equal(t, "$.settings.file", diagnostics[0].Path)
	require.Equal(t, ValidationSeverityError, diagnostics[0].Severity)

	payload, err := json.Marshal(diagnostics)
	require.NoError(t, err)
	require.Contains(t, string(payload), `"code":"yaml.duplicate-key"`)
}

func TestRecipeRejectsInvalidSelectedPathResourceShapes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name:    "missing selector",
			body:    strings.Replace(validSelectedPathRecipe("test.json", JSONFileDriverID, "config.json"), "    selector:\n      path: [user, email]\n      createMissing: create\n      duplicatePolicy: reject\n", "", 1),
			wantErr: "requires selector",
		},
		{
			name:    "missing path",
			body:    strings.Replace(validSelectedPathRecipe("test.json", JSONFileDriverID, "config.json"), "path: [user, email]", "path: []", 1),
			wantErr: "selector path is required",
		},
		{
			name:    "expression segment",
			body:    strings.Replace(validSelectedPathRecipe("test.yaml", YAMLFileDriverID, "config.yaml"), "path: [user, email]", "path: [$, email]", 1),
			wantErr: "looks like an expression",
		},
		{
			name:    "unsupported create policy",
			body:    strings.Replace(validSelectedPathRecipe("test.json", JSONFileDriverID, "config.json"), "createMissing: create", "createMissing: append", 1),
			wantErr: "unsupported selector createMissing",
		},
		{
			name:    "ini fields rejected",
			body:    strings.Replace(validSelectedPathRecipe("test.yaml", YAMLFileDriverID, "config.yaml"), "path: [user, email]", "section: user\n      key: email", 1),
			wantErr: "must not declare INI selector fields",
		},
		{
			name:    "include rejected",
			body:    validSelectedPathRecipe("test.json", JSONFileDriverID, "config.json") + "    include:\n      - \"**\"\n",
			wantErr: "must not declare include/exclude",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeNamedRecipe(t, root, "test.json", tc.body)
			_, err := LoadLocal(root, "test.json")
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestRecipeDefaultsINISelectorPoliciesForGenericResources(t *testing.T) {
	t.Parallel()

	body := strings.Replace(validINIRecipe(), "      missingSection: create\n      missingKey: create\n", "      deleteKey: allow\n", 1)
	root := t.TempDir()
	writeNamedRecipe(t, root, "test.ini", body)

	rec, err := LoadLocal(root, "test.ini")
	require.NoError(t, err)
	_, resource, err := rec.ResourceForSetting("identity")
	require.NoError(t, err)
	require.Equal(t, "error", selectorMissingSection(resource.Selector))
	require.Equal(t, "error", selectorMissingKey(resource.Selector))
	require.Equal(t, "reject", selectorDuplicatePolicy(resource.Selector))
	require.Equal(t, "allow", selectorDeleteKey(resource.Selector))
}

func TestRecipeRejectsInvalidINIFileResourceShapes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name:    "missing selector",
			body:    strings.Replace(validINIRecipe(), "    selector:\n      section: user\n      key: email\n      missingSection: create\n      missingKey: create\n", "", 1),
			wantErr: "requires selector",
		},
		{
			name:    "include rejected",
			body:    validINIRecipe() + "    include:\n      - \"**\"\n",
			wantErr: "must not declare include/exclude",
		},
		{
			name:    "selector section blank",
			body:    strings.Replace(validINIRecipe(), "section: user", "section: ' '", 1),
			wantErr: "selector section is required",
		},
		{
			name:    "selector section bracketed",
			body:    strings.Replace(validINIRecipe(), "section: user", "section: '[user]'", 1),
			wantErr: "unbracketed single-line section",
		},
		{
			name:    "selector key equals",
			body:    strings.Replace(validINIRecipe(), "key: email", "key: user=email", 1),
			wantErr: "single-line key name without equals",
		},
		{
			name:    "selected path fields rejected",
			body:    strings.Replace(validINIRecipe(), "section: user\n      key: email", "path: [user, email]", 1),
			wantErr: "must not declare selected-path selector fields",
		},
		{
			name:    "unsupported missing policy",
			body:    strings.Replace(validINIRecipe(), "missingSection: create", "missingSection: append", 1),
			wantErr: "unsupported selector missingSection",
		},
		{
			name:    "unsupported duplicate policy",
			body:    strings.Replace(validINIRecipe(), "missingKey: create", "missingKey: create\n      duplicatePolicy: last", 1),
			wantErr: "unsupported selector duplicatePolicy",
		},
		{
			name:    "unsupported delete policy",
			body:    strings.Replace(validINIRecipe(), "missingKey: create", "missingKey: create\n      deleteKey: force", 1),
			wantErr: "unsupported selector deleteKey",
		},
		{
			name:    "file driver selector rejected",
			body:    strings.Replace(validINIRecipe(), "driver: ini-file", "driver: file", 1),
			wantErr: "must not declare selector",
		},
		{
			name:    "unknown driver rejected",
			body:    strings.Replace(validINIRecipe(), "driver: ini-file", "driver: unknown-driver", 1),
			wantErr: "unsupported driver",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeNamedRecipe(t, root, "test.ini", tc.body)
			_, err := LoadLocal(root, "test.ini")
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestCustomFilesRecipeStillRejectsINIFileResources(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeRecipe(t, root, strings.Replace(validINIRecipe(), "target: test.ini", "target: custom.files", 1))

	_, err := LoadCustomFiles(root)
	require.Error(t, err)
	require.Contains(t, err.Error(), `driver must be "file" or "file-tree"`)
}

func TestLoadGitRecipeAcceptsSelectedIdentitySettingsOnly(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeNamedRecipe(t, root, GitTarget, validGitRecipe())

	rec, err := LoadGit(root)
	require.NoError(t, err)
	require.Equal(t, GitTarget, rec.Target)
	require.Equal(t, "Git", rec.DisplayName)
	require.Equal(t, "read-write", rec.Capability)
	require.Equal(t, "~", rec.Locations["home"].Default)

	nameResourceID, nameResource, err := rec.ResourceForSetting("user.name")
	require.NoError(t, err)
	require.Equal(t, "user-name", nameResourceID)
	requireGitINIResource(t, nameResource, "name")

	emailResourceID, emailResource, err := rec.ResourceForSetting("user.email")
	require.NoError(t, err)
	require.Equal(t, "user-email", emailResourceID)
	requireGitINIResource(t, emailResource, "email")
}

func TestLoadRuntimeUsesBundledGitAndIgnoresLocalGitShadow(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeNamedRecipe(t, root, GitTarget, strings.Replace(validGitRecipe(), "  user.email:\n", "  credential.helper:\n", 1))

	runtime, err := LoadRuntime(root, GitTarget)
	require.NoError(t, err)
	require.Equal(t, RecipeSourceBundled, runtime.Source)
	require.Equal(t, "recipe://bundled/git", runtime.RecipeRef)
	require.Equal(t, TrustStatusTrusted, runtime.TrustStatus)
	require.NotNil(t, runtime.Recipe)
	require.NoError(t, runtime.Recipe.ValidateGit())
	require.Contains(t, runtime.Recipe.Settings, "user.email")
	require.NotContains(t, runtime.Recipe.Settings, "credential.helper")

	eval, err := EvaluateRecipeTrust(root, t.TempDir(), runtime.Source, runtime.Recipe)
	require.NoError(t, err)
	require.Equal(t, TrustStatusTrusted, eval.Status)
	require.NoError(t, runtime.Recipe.ValidateWriteSafety(eval.WriteSafetyContext(WriteSafetyContext{})))
}

func TestLoadRuntimeKeepsBundledRuntimeUnavailableExplicitForNonExecutableTargets(t *testing.T) {
	t.Parallel()

	runtime, err := LoadRuntime(t.TempDir(), CustomFilesTarget)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrBundledRuntimeUnavailable))
	require.Equal(t, RecipeSourceBundled, runtime.Source)
	require.Equal(t, "recipe://bundled/custom.files", runtime.RecipeRef)
	require.Nil(t, runtime.Recipe)
}

func TestLoadRuntimeLocalAndInvalidBranches(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeNamedRecipe(t, root, "test.ini", validINIRecipe())

	runtime, err := LoadRuntime(root, "test.ini")
	require.NoError(t, err)
	require.Equal(t, RecipeSourceLocal, runtime.Source)
	require.Equal(t, "recipe://local/test.ini", runtime.RecipeRef)
	require.NotNil(t, runtime.Recipe)
	require.Equal(t, "test.ini", runtime.Recipe.Target)

	_, err = LoadRuntime(root, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "target is required")

	runtime, err = LoadRuntime(root, "missing")
	require.Error(t, err)
	require.Equal(t, RecipeSourceLocal, runtime.Source)
	require.Equal(t, "recipe://local/missing", runtime.RecipeRef)
	require.Nil(t, runtime.Recipe)
	require.True(t, errors.Is(err, os.ErrNotExist))
}

func TestGitRecipeRejectsCredentialAndBroadConfigDeclarations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name:    "wrong target rejected",
			body:    strings.Replace(validGitRecipe(), "target: git", "target: git.extra", 1),
			wantErr: "target must be",
		},
		{
			name:    "read only capability rejected",
			body:    strings.Replace(validGitRecipe(), "capability: read-write", "capability: read-only", 1),
			wantErr: "capability must be read-write",
		},
		{
			name:    "extra location rejected",
			body:    strings.Replace(validGitRecipe(), "settings:\n", "  work:\n    default: /tmp\nsettings:\n", 1),
			wantErr: "only the home location",
		},
		{
			name:    "missing home location rejected",
			body:    strings.Replace(strings.Replace(validGitRecipe(), "  home:\n    default: \"~\"\n", "  work:\n    default: /tmp\n", 1), "location: home", "location: work", -1),
			wantErr: "must declare home location",
		},
		{
			name:    "extra credential setting rejected",
			body:    strings.Replace(validGitRecipe(), "resources:\n", "  credential.helper:\n    scopeDefault: user\n    resource: user-email\nresources:\n", 1),
			wantErr: "only user.name and user.email",
		},
		{
			name:    "missing user name setting rejected",
			body:    strings.Replace(validGitRecipe(), "  user.name:\n    scopeDefault: user\n    resource: user-name\n", "  user.alias:\n    scopeDefault: user\n    resource: user-name\n", 1),
			wantErr: "missing setting user.name",
		},
		{
			name:    "wrong default scope rejected",
			body:    strings.Replace(validGitRecipe(), "scopeDefault: user", "scopeDefault: machine", 1),
			wantErr: "scopeDefault must be user",
		},
		{
			name:    "extra selected key resource rejected",
			body:    validGitRecipe() + "  extra:\n    driver: file\n    location: home\n    path: extra\n",
			wantErr: "exactly two selected-key resources",
		},
		{
			name: "wrong selected key driver rejected",
			body: strings.Replace(
				strings.Replace(validGitRecipe(), "driver: ini-file", "driver: file", 1),
				"    selector:\n      section: user\n      key: email\n      missingSection: create\n      missingKey: create\n      duplicatePolicy: reject\n",
				"",
				1,
			),
			wantErr: "driver must be",
		},
		{
			name:    "credential section rejected",
			body:    strings.Replace(validGitRecipe(), "section: user", "section: credential", 1),
			wantErr: "must select [user]",
		},
		{
			name:    "credential key rejected",
			body:    strings.Replace(validGitRecipe(), "key: email", "key: helper", 1),
			wantErr: "must select [user] email",
		},
		{
			name:    "url section rejected",
			body:    strings.Replace(validGitRecipe(), "section: user", "section: url", 1),
			wantErr: "must select [user]",
		},
		{
			name:    "include key rejected",
			body:    strings.Replace(validGitRecipe(), "key: email", "key: path", 1),
			wantErr: "must select [user] email",
		},
		{
			name:    "wrong path rejected",
			body:    strings.Replace(validGitRecipe(), "path: .gitconfig", "path: .config/git/config", 1),
			wantErr: "path must be .gitconfig",
		},
		{
			name:    "wrong location rejected",
			body:    strings.Replace(validGitRecipe(), "location: home", "location: config", 1),
			wantErr: "references unknown location",
		},
		{
			name:    "wrong home default rejected",
			body:    strings.Replace(validGitRecipe(), "default: \"~\"", "default: ~/.config", 1),
			wantErr: "home location default must be ~",
		},
		{
			name:    "missing create policy rejected",
			body:    strings.Replace(validGitRecipe(), "missingSection: create", "missingSection: error", 1),
			wantErr: "missingSection must be",
		},
		{
			name:    "delete allow rejected",
			body:    strings.Replace(validGitRecipe(), "duplicatePolicy: reject", "duplicatePolicy: reject\n      deleteKey: allow", 1),
			wantErr: "deleteKey must be",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeNamedRecipe(t, root, GitTarget, tc.body)
			_, err := LoadGit(root)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func writeRecipe(t *testing.T, root string, body string) {
	t.Helper()
	writeNamedRecipe(t, root, CustomFilesTarget, body)
}

func writeNamedRecipe(t *testing.T, root string, recipeID string, body string) {
	t.Helper()
	path := filepath.Join(root, "recipes", "local", recipeID, "recipe.yaml")
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

func validINIRecipe() string {
	return `schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: test.ini
displayName: Test INI
supportLevel: experimental
capability: read-write
locations:
  home:
    default: "~"
settings:
  identity:
    scopeDefault: user
    resource: git-email
resources:
  git-email:
    driver: ini-file
    location: home
    path: .gitconfig
    selector:
      section: user
      key: email
      missingSection: create
      missingKey: create
`
}

func validGitRecipe() string {
	return `schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: git
displayName: Git
supportLevel: experimental
capability: read-write
locations:
  home:
    default: "~"
settings:
  user.email:
    scopeDefault: user
    resource: user-email
  user.name:
    scopeDefault: user
    resource: user-name
resources:
  user-email:
    driver: ini-file
    location: home
    path: .gitconfig
    selector:
      section: user
      key: email
      missingSection: create
      missingKey: create
      duplicatePolicy: reject
  user-name:
    driver: ini-file
    location: home
    path: .gitconfig
    selector:
      section: user
      key: name
      missingSection: create
      missingKey: create
      duplicatePolicy: reject
`
}

func validSelectedPathRecipe(target string, driver string, resourcePath string) string {
	return `schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: ` + target + `
displayName: Selected path test
supportLevel: experimental
capability: read-write
locations:
  config:
    default: ~/.example-tool
settingsGroups:
  identity:
    label: Identity
    supportLevel: experimental
    capability: read-write
    settings:
      - identity.email
settings:
  identity.email:
    label: User email
    supportLevel: experimental
    capability: read-write
    artifactForm: scalar
    scopeDefault: user
    resource: config-email
resources:
  config-email:
    driver: ` + driver + `
    location: config
    path: ` + resourcePath + `
    selector:
      path: [user, email]
      createMissing: create
      duplicatePolicy: reject
`
}

func requireGitINIResource(t *testing.T, resource Resource, key string) {
	t.Helper()
	require.Equal(t, IniFileDriverID, resource.Driver)
	require.Equal(t, "home", resource.Location)
	require.Equal(t, ".gitconfig", resource.Path)
	require.Empty(t, resource.Include)
	require.Empty(t, resource.Exclude)
	require.NotNil(t, resource.Selector)
	require.Equal(t, "user", resource.Selector.Section)
	require.Equal(t, key, resource.Selector.Key)
	require.Equal(t, "create", resource.Selector.MissingSection)
	require.Equal(t, "create", resource.Selector.MissingKey)
	require.Equal(t, "reject", selectorDuplicatePolicy(resource.Selector))
	require.Equal(t, "reject", selectorDeleteKey(resource.Selector))
}

func validCustomFileTreeRecipe(resourcePath string, include []string, exclude []string) string {
	body := replace(validCustomFilesRecipe(resourcePath), "driver: file", "driver: file-tree")
	if include != nil {
		body += "    include:\n"
		for _, value := range include {
			body += "      - " + yamlQuote(value) + "\n"
		}
	}
	if exclude != nil {
		body += "    exclude:\n"
		for _, value := range exclude {
			body += "      - " + yamlQuote(value) + "\n"
		}
	}
	return body
}

func yamlQuote(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}

func replace(value string, old string, new string) string {
	return strings.ReplaceAll(value, old, new)
}

func TestRecipeValidationHelperBranches(t *testing.T) {
	t.Parallel()

	require.Equal(t, "recipe validation failed", (*ValidationError)(nil).Error())
	require.Equal(t, "recipe validation failed", (&ValidationError{}).Error())
	require.Empty(t, ValidationDiagnostics(fmt.Errorf("plain error")))

	diagnostics := normalizeDiagnostics([]ValidationDiagnostic{{Code: "test.code", Message: "test message"}})
	require.Equal(t, "$", diagnostics[0].Path)
	require.Equal(t, ValidationSeverityError, diagnostics[0].Severity)
	err := validationError(diagnostics)
	require.Error(t, err)
	require.Contains(t, err.Error(), "$[test.code]: test message")
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
		{name: "setting label whitespace", recipe: rec(func(r *Recipe) {
			r.Settings["file"] = Setting{Label: " File", ScopeDefault: "user", Resource: "config-file"}
		}), wantErr: "label must not have surrounding whitespace"},
		{name: "setting support", recipe: rec(func(r *Recipe) {
			r.Settings["file"] = Setting{SupportLevel: "unsafe", ScopeDefault: "user", Resource: "config-file"}
		}), wantErr: "unsupported supportLevel"},
		{name: "setting capability", recipe: rec(func(r *Recipe) {
			r.Settings["file"] = Setting{Capability: "command-io", ScopeDefault: "user", Resource: "config-file"}
		}), wantErr: "unsupported capability"},
		{name: "setting artifact", recipe: rec(func(r *Recipe) {
			r.Settings["file"] = Setting{ArtifactForm: "script", ScopeDefault: "user", Resource: "config-file"}
		}), wantErr: "unsupported artifactForm"},
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
