package appdiscovery

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	v2recipe "github.com/shpoont/dotfiles-manager/internal/v2/recipe"
)

func TestListAndSearchReportsManagedState(t *testing.T) {
	repoRoot := setupAppDiscoveryRepo(t, []string{"global"}, map[string]string{"global": `schema: dotfiles-manager.v2.profile-layer
schemaVersion: 1
selections:
  git:
    settings:
      user.email:
        scope: user
      user.name:
        scope: user
`})

	report, err := List(Options{RepoRoot: repoRoot, RepoRootSet: true, UserID: "leon"})
	require.NoError(t, err)
	require.Equal(t, AppsSchema, report.Schema)
	require.Equal(t, ListCommand, report.Command)
	require.Equal(t, ListRunID, report.RunID)
	require.Equal(t, "ok", report.Summary.Status)
	require.Equal(t, 6, report.Summary.Apps)
	require.Equal(t, 1, report.Summary.Managed)
	require.Empty(t, report.Diagnostics)
	require.Nil(t, report.Error)
	requireNoApp(t, report, "custom.files")

	git := requireApp(t, report, "git")
	require.Equal(t, "Git", git.DisplayName)
	require.Equal(t, []string{"gitconfig"}, git.Aliases)
	require.Equal(t, "official", git.Source)
	require.Equal(t, StateManaged, git.State)
	require.Equal(t, 2, git.SelectedSettings)
	require.Equal(t, "recipe://bundled/git", git.RecipeRef)
	require.Equal(t, "trusted", git.TrustStatus)

	zsh := requireApp(t, report, "zsh")
	require.Equal(t, StateNotManaged, zsh.State)
	require.Zero(t, zsh.SelectedSettings)

	text := Text(report)
	require.Contains(t, text, "Supported apps")
	require.Contains(t, text, "APP       CATALOG   STATE")
	require.Contains(t, text, "git       official  managed")
	require.Contains(t, text, "Use `dotfiles-manager explain <app>` to see what can be managed.")
	require.NotContains(t, text, "custom.files")
	require.NotContains(t, text, "built-in")
	require.NotContains(t, text, "No live settings were read or changed.")
	require.NotContains(t, text, "No stored settings were changed.")

	payload, err := JSON(report)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal([]byte(payload), &decoded))
	require.Equal(t, AppsSchema, decoded["schema"])

	search, err := Search(Options{RepoRoot: repoRoot, RepoRootSet: true, UserID: "leon", Query: "gitconfig"})
	require.NoError(t, err)
	require.Equal(t, SearchCommand, search.Command)
	require.Equal(t, SearchRunID, search.RunID)
	require.Equal(t, "gitconfig", search.Query)
	require.Equal(t, 1, search.Summary.Apps)
	require.Equal(t, 1, search.Summary.Matches)
	require.Equal(t, 1, search.Summary.Managed)
	require.Equal(t, "git", search.Apps[0].ID)
	require.Contains(t, searchText(search), `Search results for "gitconfig"`)
	require.Contains(t, searchText(search), "dotfiles-manager explain git")

	noMatch, err := Search(Options{Query: "not-a-supported-app"})
	require.NoError(t, err)
	require.Equal(t, 0, noMatch.Summary.Apps)
	require.Equal(t, 0, noMatch.Summary.Matches)
	require.Empty(t, noMatch.Apps)
	require.Contains(t, Text(noMatch), `No supported apps found for "not-a-supported-app".`)
	require.Contains(t, Text(noMatch), "The current official catalog supports:")
	require.Contains(t, Text(noMatch), "git, nvim, ssh, starship, tmux, zsh")
	require.NotContains(t, Text(noMatch), "catalog add")
	require.NotContains(t, Text(noMatch), "local catalog")
}

func TestSearchValidationAndNilRenderers(t *testing.T) {
	report, err := Search(Options{Query: "  "})
	require.Error(t, err)
	appErr := requireAppError(t, err)
	require.Equal(t, CodeQueryInvalid, appErr.Code)
	require.Equal(t, 2, appErr.ExitCode())
	require.Equal(t, "error", report.Summary.Status)
	require.Equal(t, 1, report.Summary.Failed)
	require.Equal(t, CodeQueryInvalid, report.Error.Code)
	require.Contains(t, Text(report), "search query is required")

	payload, err := JSON(nil)
	require.NoError(t, err)
	require.Contains(t, payload, `"status": "error"`)
	require.Contains(t, Text(nil), "The command could not complete.")

	payload, err = ExplainJSON(nil)
	require.NoError(t, err)
	require.Contains(t, payload, `"schema": "dotfiles-manager.v2.app"`)
	require.Contains(t, ExplainText(nil), "The command could not complete.")
	require.Contains(t, ExplainVerboseText(nil), "summary status=error")
}

func TestExplainReportTextVerboseJSONAndUnknown(t *testing.T) {
	repoRoot := setupAppDiscoveryRepo(t, []string{"global"}, map[string]string{"global": `schema: dotfiles-manager.v2.profile-layer
schemaVersion: 1
selections:
  git:
    settings:
      user.email:
        scope: user
`})

	report, err := Explain(Options{RepoRoot: repoRoot, RepoRootSet: true, UserID: "leon", Query: "git"})
	require.NoError(t, err)
	require.Equal(t, AppSchema, report.Schema)
	require.Equal(t, ExplainCommand, report.Command)
	require.Equal(t, ExplainRunID, report.RunID)
	require.Equal(t, "ok", report.Summary.Status)
	require.Equal(t, 1, report.Summary.Apps)
	require.Equal(t, 1, report.Summary.Managed)
	require.Equal(t, "git", report.App.ID)
	require.Equal(t, "Git", report.App.DisplayName)
	require.Equal(t, "official", report.App.Source)
	require.Equal(t, "official catalog", report.App.SourceDescription)
	require.Equal(t, StateManaged, report.App.State)
	require.Equal(t, 1, report.App.SelectedSettings)
	require.NotEmpty(t, report.App.Settings)
	require.Contains(t, report.App.DoNotManage, "credential.helper")

	text := ExplainText(report)
	require.Contains(t, text, "Git is supported.")
	require.Contains(t, text, "State: managed")
	require.Contains(t, text, "Catalog: official")
	require.Contains(t, text, "Can manage:")
	require.NotContains(t, text, "Why this source is used:")
	require.NotContains(t, text, "No live values were printed.")
	require.NotContains(t, text, "No live settings were changed.")
	require.NotContains(t, text, "No stored settings were changed.")

	verbose := ExplainVerboseText(report)
	require.Contains(t, verbose, "app: git")
	require.Contains(t, verbose, "selectedSettings=1")
	require.Contains(t, verbose, "summary status=ok apps=1 managed=1 failed=0")

	payload, err := ExplainJSON(report)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal([]byte(payload), &decoded))
	require.Equal(t, AppSchema, decoded["schema"])

	unknown, err := Explain(Options{Query: "missing"})
	require.Error(t, err)
	appErr := requireAppError(t, err)
	require.Equal(t, CodeAppNotSupported, appErr.Code)
	require.Equal(t, 2, appErr.ExitCode())
	require.Equal(t, "missing", appErr.Details["app"])
	require.Equal(t, "error", unknown.Summary.Status)
	require.Equal(t, 1, unknown.Summary.Failed)
	require.Equal(t, CodeAppNotSupported, unknown.Error.Code)
	require.Contains(t, ExplainText(unknown), "App not supported: missing")
	require.Contains(t, ExplainVerboseText(unknown), "error[explain.app.notSupported]")
	require.NotContains(t, ExplainText(unknown), "No live settings were read or changed.")
	require.NotContains(t, ExplainText(unknown), "No stored settings were changed.")

	pseudoApp, err := Explain(Options{Query: "custom.files"})
	require.Error(t, err)
	appErr = requireAppError(t, err)
	require.Equal(t, CodeAppNotSupported, appErr.Code)
	require.Equal(t, "custom.files", appErr.Details["app"])
	require.Equal(t, "error", pseudoApp.Summary.Status)
	require.NotContains(t, ExplainText(pseudoApp), "Custom files is supported.")

	_, err = Explain(Options{Query: "custom-files"})
	require.Error(t, err)
	appErr = requireAppError(t, err)
	require.Equal(t, CodeAppNotSupported, appErr.Code)
	require.Equal(t, "custom-files", appErr.Details["app"])
}

func TestRepoRootErrorsReturnStableReports(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")

	list, err := List(Options{RepoRoot: missing, RepoRootSet: true})
	require.Error(t, err)
	require.Equal(t, "error", list.Summary.Status)
	require.Equal(t, 1, list.Summary.Failed)
	require.Equal(t, CodeRepoInvalid, list.Error.Code)
	require.Contains(t, Text(list), "stat repo root")

	search, err := Search(Options{RepoRoot: missing, RepoRootSet: true, Query: "git"})
	require.Error(t, err)
	require.Equal(t, CodeRepoInvalid, requireAppError(t, err).Code)
	require.Equal(t, CodeRepoInvalid, search.Error.Code)

	explain, err := Explain(Options{RepoRoot: missing, RepoRootSet: true, Query: "git"})
	require.Error(t, err)
	require.Equal(t, CodeRepoInvalid, requireAppError(t, err).Code)
	require.Equal(t, CodeRepoInvalid, explain.Error.Code)
}

func TestRenderHelpersCoverFallbackSourcesAndErrors(t *testing.T) {
	report := &Report{
		Schema:        AppsSchema,
		SchemaVersion: SchemaVersion,
		Command:       ListCommand,
		RunID:         ListRunID,
		Summary:       Summary{Status: "ok", Apps: 2, Managed: 1},
		Apps: []App{
			{ID: "local.tool", DisplayName: "Local Tool", Source: "local", State: StateManaged, SelectedSettings: 1},
			{ID: "unknown_tool", Source: "unknown", State: StateNotManaged},
		},
	}
	require.Contains(t, listText(report), "local.tool")
	require.Contains(t, listText(report), "local")
	require.Contains(t, listText(report), "managed")

	explain := &ExplainReport{
		Schema:        AppSchema,
		SchemaVersion: SchemaVersion,
		Command:       ExplainCommand,
		RunID:         ExplainRunID,
		Summary:       Summary{Status: "ok", Apps: 1, Managed: 1},
		App: ExplainApp{
			ID:                "local.tool",
			DisplayName:       "Local Tool",
			Source:            "local",
			SourceDescription: sourceDescription(v2recipe.RecipeSourceLocal),
			State:             StateManaged,
			SelectedSettings:  1,
		},
		Diagnostics: []Diagnostic{{Severity: "warning", Code: "demo.warning", Message: "review local recipe"}},
	}
	require.Contains(t, ExplainText(explain), "Catalog: local")
	require.Contains(t, ExplainVerboseText(explain), "warning[demo.warning]: review local recipe")

	unsupported := mapExplainError("tool", errors.New("boom"))
	require.Equal(t, "explain.failed", unsupported.Code)
	require.Equal(t, 1, unsupported.ExitCode())
	require.Equal(t, "tool", unsupported.Details["app"])

	wrapped := mapExplainError("tool", &v2recipe.ExplainError{Code: "source.invalid", Message: "bad source", Exit: 7, Details: map[string]any{"target": "tool"}})
	require.Equal(t, "explain.source.invalid", wrapped.Code)
	require.Equal(t, 7, wrapped.ExitCode())
	require.Equal(t, "tool", wrapped.Details["target"])

	require.Equal(t, "official", appSource(v2recipe.RecipeSourceBundled))
	require.Equal(t, "local", appSource(v2recipe.RecipeSourceLocal))
	require.Equal(t, "unknown", appSource(""))
	require.Equal(t, "third-party", appSource("third-party"))
	require.Equal(t, "third.party", sourceDescription("third.party"))
	require.Equal(t, "Custom Tool", displayName("", "custom_tool"))
	require.Equal(t, "App", displayName("", ""))
	require.Equal(t, 1, boolToInt(true))
	require.Equal(t, 0, boolToInt(false))
	require.Equal(t, "", plural(1))
	require.Equal(t, "s", plural(2))
	require.Equal(t, []string{"a"}, trimBlank([]string{"a", "", "  "}))
}

func setupAppDiscoveryRepo(t *testing.T, stack []string, layers map[string]string) string {
	t.Helper()
	root := t.TempDir()
	writeAppDiscoveryFile(t, filepath.Join(root, "dotfiles-manager.v2.yaml"), "schema: dotfiles-manager.v2.root-config\nschemaVersion: 1\nactiveProfileStack: default\n")
	stackBody := "schema: dotfiles-manager.v2.profile-stack\nschemaVersion: 1\nprofileStack:\n"
	for _, layer := range stack {
		stackBody += "  - " + layer + "\n"
	}
	writeAppDiscoveryFile(t, filepath.Join(root, "profiles", "stacks", "default.yaml"), stackBody)
	for layer, body := range layers {
		writeAppDiscoveryFile(t, filepath.Join(root, "profiles", "layers", filepath.FromSlash(layer)+".yaml"), body)
	}
	return root
}

func writeAppDiscoveryFile(t *testing.T, path string, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func requireApp(t *testing.T, report *Report, id string) App {
	t.Helper()
	for _, app := range report.Apps {
		if app.ID == id {
			return app
		}
	}
	require.Failf(t, "missing app", "app %s not found in %v", id, report.Apps)
	return App{}
}

func requireAppError(t *testing.T, err error) *Error {
	t.Helper()
	var appErr *Error
	require.ErrorAs(t, err, &appErr)
	require.NotNil(t, appErr)
	return appErr
}

func TestMatchesAppUsesAllPublicSearchFields(t *testing.T) {
	app := App{
		ID:              "demo",
		DisplayName:     "Demo App",
		Aliases:         []string{"sample"},
		Source:          "local",
		SupportLevel:    "experimental",
		Capability:      "read-write",
		PlatformSupport: "macos",
		Summary:         "Manages prompt settings",
	}
	for _, query := range []string{"demo", "app", "sample", "local", "experimental", "read-write", "macos", "prompt", "  PROMPT  ", ""} {
		require.True(t, matchesApp(app, query), query)
	}
	require.False(t, matchesApp(app, "browser"))
	require.True(t, strings.Contains(Text(&Report{Command: SearchCommand, Query: "browser", Apps: []App{}}), "No supported apps"))
}

func requireNoApp(t *testing.T, report *Report, id string) {
	t.Helper()
	for _, app := range report.Apps {
		require.NotEqual(t, id, app.ID)
	}
}
