package recipe

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExplainBundledGitAndCustomFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeNamedRecipe(t, root, GitTarget, stringsForExplainTest(`schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: git
displayName: Local Git Collision
supportLevel: experimental
capability: read-write
locations:
  home:
    default: "~"
settings:
  credential.helper:
    scopeDefault: user
    resource: helper
resources:
  helper:
    driver: file
    location: home
    path: .gitconfig
`))

	report, err := Explain(ExplainOptions{Target: GitTarget, RepoRoot: root})
	require.NoError(t, err)
	require.Equal(t, ExplainSchema, report.Schema)
	require.Equal(t, ExplainCommand, report.Command)
	require.Equal(t, "ok", report.Summary.Status)
	require.Equal(t, GitTarget, report.RecipeExplain.Target.Ref)
	require.Equal(t, "bundled", report.RecipeExplain.Recipe.Source)
	require.Len(t, report.RecipeExplain.Settings, 2)
	require.Equal(t, "git:user.email", report.RecipeExplain.Settings[0].Ref)
	require.Equal(t, "[user] email", report.RecipeExplain.Resources[0].Selector.Summary)
	requireDiagnosticCodeInRecipe(t, report, ExplainCodeLocalRecipeShadowed)
	requireDiagnosticCodeInRecipe(t, report, ExplainCodeSelectionUnresolved)

	text := ExplainText(report)
	require.Contains(t, text, "target: git")
	require.Contains(t, text, "git:user.email")
	require.Contains(t, text, "selector=[user] email")
	require.Contains(t, text, "do not manage: [credential] sections")
	require.NotContains(t, text, "credential.helper")

	payload, err := ExplainJSON(report)
	require.NoError(t, err)
	require.Contains(t, payload, `"command": "recipe.explain"`)
	require.NotContains(t, payload, "credential.helper")

	custom, err := Explain(ExplainOptions{Target: CustomFilesTarget, RepoRoot: root})
	require.NoError(t, err)
	require.Equal(t, "recipe://bundled/custom.files", custom.RecipeExplain.Recipe.RecipeRef)
	require.Len(t, custom.RecipeExplain.Settings, 2)
	require.Len(t, custom.RecipeExplain.Resources, 2)
	require.Equal(t, FileTreeDriverID, custom.RecipeExplain.Resources[1].DriverID)
	require.Contains(t, ExplainText(custom), "custom.files:file-tree")
}

func TestExplainLocalRecipe(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeNamedRecipe(t, root, "cobona", stringsForExplainTest(`schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: cobona
displayName: Cobona
supportLevel: experimental
capability: read-write
locations:
  config:
    default: ~/.cobona
settings:
  user.email:
    scopeDefault: user
    resource: config-email
resources:
  config-email:
    driver: ini-file
    location: config
    path: config.ini
    selector:
      section: user
      key: email
      missingSection: create
      missingKey: create
`))

	report, err := Explain(ExplainOptions{Target: "cobona", RepoRoot: root})
	require.NoError(t, err)
	require.Equal(t, "local", report.RecipeExplain.Recipe.Source)
	require.Equal(t, "recipe://local/cobona", report.RecipeExplain.Recipe.RecipeRef)
	require.Equal(t, "untrusted", report.RecipeExplain.Recipe.TrustStatus)
	require.Equal(t, "cobona:user.email", report.RecipeExplain.Settings[0].Ref)
	require.Equal(t, IniFileDriverID, report.RecipeExplain.Drivers[0].ID)
	require.Equal(t, "[user] email", report.RecipeExplain.Resources[0].Selector.Summary)
	require.Contains(t, ExplainText(report), "local recipe is untrusted")
}

func TestExplainErrorsAndExitCodes(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	report, err := Explain(ExplainOptions{Target: "missing", RepoRoot: root})
	require.Error(t, err)
	require.Equal(t, ExplainCodeUnknownTarget, report.Error.Code)
	require.Equal(t, 2, err.(*ExplainError).ExitCode())
	require.Contains(t, ExplainText(report), "error[unknown-target]")

	report, err = Explain(ExplainOptions{Target: "git:user.email", RepoRoot: root})
	require.Error(t, err)
	require.Equal(t, ExplainCodeUnsupportedRefKind, report.Error.Code)
	require.Equal(t, 2, err.(*ExplainError).ExitCode())

	report, err = Explain(ExplainOptions{Target: "BadTarget", RepoRoot: root})
	require.Error(t, err)
	require.Equal(t, ExplainCodeInvalidRef, report.Error.Code)
	require.Equal(t, 2, err.(*ExplainError).ExitCode())

	writeNamedRecipe(t, root, "broken", stringsForExplainTest(`schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: broken
displayName: Broken
supportLevel: experimental
capability: read-write
locations:
  config:
    default: ~/.broken
settings:
  bad:
    scopeDefault: user
    resource: missing
resources:
  other:
    driver: file
    location: config
    path: config.txt
`))
	report, err = Explain(ExplainOptions{Target: "broken", RepoRoot: root})
	require.Error(t, err)
	require.Equal(t, ExplainCodeInvalidRecipe, report.Error.Code)
	require.Equal(t, 2, err.(*ExplainError).ExitCode())
	requireDiagnosticCodeInRecipe(t, report, ExplainCodeInvalidRecipe)

	require.Equal(t, "", (*ExplainError)(nil).Error())
	require.Equal(t, 1, (*ExplainError)(nil).ExitCode())
	require.Equal(t, 1, (&ExplainError{Message: "x"}).ExitCode())
}

func TestExplainRenderingAndHelperBranches(t *testing.T) {
	t.Parallel()

	jsonPayload, err := ExplainJSON(nil)
	require.NoError(t, err)
	require.Contains(t, jsonPayload, `"status": "error"`)
	require.Contains(t, ExplainText(nil), "summary status=error")

	resource := explainResource("tree", Resource{Driver: FileTreeDriverID, Location: "config", Path: "profiles", Include: []string{"**/*.json"}, Exclude: []string{"cache/**"}})
	require.Equal(t, "file-tree", resource.DiffMode)
	require.Equal(t, []string{"**/*.json"}, resource.Include)
	require.Equal(t, []string{"cache/**"}, resource.Exclude)

	resource = explainResource("raw", Resource{Driver: FileDriverID, Location: "config", Path: "config.txt"})
	require.Equal(t, "file", resource.DiffMode)

	resource = explainResource("unknown", Resource{Driver: "custom-driver", Location: "config", Path: "config.txt"})
	require.Equal(t, "unknown", resource.DiffMode)

	drivers := driverExplains(FileDriverID, FileDriverID, "custom-driver", "")
	require.Len(t, drivers, 2)
	require.Equal(t, "custom-driver", drivers[0].ID)
	require.Equal(t, FileDriverID, drivers[1].ID)
	require.Equal(t, "unknown", artifactFormForDriver("custom-driver"))
	require.Equal(t, "unknown", fallbackUnknown(""))
	require.Equal(t, "user email", fallbackLabel("user.email"))

	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "other.yaml")
	require.Equal(t, "other.yaml", safeRelOrBase(root, outside))
	require.False(t, fileExists(filepath.Join(root, "missing.yaml")))
	file := filepath.Join(root, "exists.yaml")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o644))
	require.True(t, fileExists(file))
}

func requireDiagnosticCodeInRecipe(t *testing.T, report *ExplainReport, code string) {
	t.Helper()
	for _, diagnostic := range report.RecipeExplain.Diagnostics {
		if diagnostic.Code == code {
			return
		}
	}
	require.Failf(t, "diagnostic code missing", "expected diagnostic code %q in %#v", code, report.RecipeExplain.Diagnostics)
}

func stringsForExplainTest(value string) string {
	return value
}
