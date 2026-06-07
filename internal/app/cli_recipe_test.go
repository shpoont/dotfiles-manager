package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRecipeExplainGitJSONIsMetadataOnly(t *testing.T) {
	tempDir := t.TempDir()
	oldWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	require.NoError(t, os.Chdir(tempDir))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, ".gitconfig"), []byte("[user]\n\temail = secret@example.com\n"), 0o644))

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"recipe", "explain", "git", "--json"})

	err = cmd.Execute()
	require.NoError(t, err)
	require.Empty(t, stderr.String())
	require.NoDirExists(t, filepath.Join(tempDir, "profiles"))
	require.NoDirExists(t, filepath.Join(tempDir, "desired"))
	require.NoDirExists(t, filepath.Join(tempDir, "state"))
	require.NoDirExists(t, filepath.Join(tempDir, "migrations"))

	out := stdout.String()
	require.NotContains(t, out, "secret@example.com")
	require.NotContains(t, out, "credential.helper")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	require.Equal(t, "dotfiles-manager.v2.preview", payload["schema"])
	require.Equal(t, "recipe.explain", payload["command"])
	require.Empty(t, payload["items"].([]any))
	recipeExplain := payload["recipeExplain"].(map[string]any)
	target := recipeExplain["target"].(map[string]any)
	require.Equal(t, "git", target["ref"])
	recipeObj := recipeExplain["recipe"].(map[string]any)
	require.Equal(t, "bundled", recipeObj["source"])
	require.Equal(t, "recipe://bundled/git", recipeObj["recipeRef"])
	settings := recipeExplain["settings"].([]any)
	require.Len(t, settings, 2)
	require.Equal(t, "git:user.email", settings[0].(map[string]any)["ref"])
	require.Equal(t, "git:user.name", settings[1].(map[string]any)["ref"])
	resources := recipeExplain["resources"].([]any)
	require.Len(t, resources, 2)
	require.Equal(t, ".gitconfig", resources[0].(map[string]any)["path"])
	require.Equal(t, "ini-file", resources[0].(map[string]any)["driverId"])
	safety := recipeExplain["safety"].(map[string]any)
	doNotManage := safety["doNotManage"].([]any)
	require.Contains(t, doNotManage, "[credential] sections")
	require.Contains(t, doNotManage, "include and includeIf expansion")
}

func TestRecipeExplainGitTextAndCustomFilesText(t *testing.T) {
	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"recipe", "explain", "git"})

	err := cmd.Execute()
	require.NoError(t, err)
	require.Empty(t, stderr.String())
	out := stdout.String()
	require.Contains(t, out, "recipe explain")
	require.Contains(t, out, "target: git")
	require.Contains(t, out, "git:user.email")
	require.Contains(t, out, "selector=[user] email")
	require.Contains(t, out, "do not manage: [credential] sections")
	require.NotContains(t, out, "secret@example.com")

	cmd = NewRootCmd()
	stdout.Reset()
	stderr.Reset()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"recipe", "explain", "custom.files"})
	err = cmd.Execute()
	require.NoError(t, err)
	require.Empty(t, stderr.String())
	out = stdout.String()
	require.Contains(t, out, "target: custom.files")
	require.Contains(t, out, "custom.files:file")
	require.Contains(t, out, "custom.files:file-tree")
	require.Contains(t, out, "driver=file-tree")
}

func TestRecipeListTextAndJSON(t *testing.T) {
	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"recipe", "list"})

	err := cmd.Execute()
	require.NoError(t, err)
	require.Empty(t, stderr.String())
	out := stdout.String()
	require.Contains(t, out, "recipe list")
	require.Contains(t, out, "custom.files source=bundled")
	require.Contains(t, out, "git source=bundled")
	require.Contains(t, out, "aliases=gitconfig")

	cmd = NewRootCmd()
	stdout.Reset()
	stderr.Reset()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"recipe", "list", "--json"})

	err = cmd.Execute()
	require.NoError(t, err)
	require.Empty(t, stderr.String())

	var payload map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	require.Equal(t, "recipe.list", payload["command"])
	recipeList := payload["recipeList"].(map[string]any)
	targets := recipeList["targets"].([]any)
	require.Len(t, targets, 2)
	require.Equal(t, "custom.files", targets[0].(map[string]any)["id"])
	require.Equal(t, "git", targets[1].(map[string]any)["id"])
	require.Equal(t, "bundled", targets[1].(map[string]any)["source"])
	require.Equal(t, "trusted", targets[1].(map[string]any)["trustStatus"])
}

func TestRecipeExplainBundledAliasJSON(t *testing.T) {
	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"recipe", "explain", "gitconfig", "--json"})

	err := cmd.Execute()
	require.NoError(t, err)
	require.Empty(t, stderr.String())

	var payload map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	recipeExplain := payload["recipeExplain"].(map[string]any)
	target := recipeExplain["target"].(map[string]any)
	require.Equal(t, "git", target["ref"])
	recipeObj := recipeExplain["recipe"].(map[string]any)
	require.Equal(t, "recipe://bundled/git", recipeObj["recipeRef"])
}

func TestRecipeExplainLocalRecipeJSON(t *testing.T) {
	tempDir := t.TempDir()
	oldWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	require.NoError(t, os.Chdir(tempDir))
	writeLocalRecipeFile(t, tempDir, "cobona", `schema: dotfiles-manager.v2.recipe
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
`)
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, ".cobona"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, ".cobona", "config.ini"), []byte("[user]\nemail=secret@example.com\n"), 0o644))

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"recipe", "explain", "cobona", "--json"})

	err = cmd.Execute()
	require.NoError(t, err)
	require.Empty(t, stderr.String())
	require.NotContains(t, stdout.String(), "secret@example.com")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	recipeExplain := payload["recipeExplain"].(map[string]any)
	recipeObj := recipeExplain["recipe"].(map[string]any)
	require.Equal(t, "local", recipeObj["source"])
	require.Equal(t, "recipe://local/cobona", recipeObj["recipeRef"])
	require.Equal(t, "untrusted", recipeObj["trustStatus"])
	settings := recipeExplain["settings"].([]any)
	require.Len(t, settings, 1)
	require.Equal(t, "cobona:user.email", settings[0].(map[string]any)["ref"])
	resources := recipeExplain["resources"].([]any)
	require.Equal(t, "[user] email", resources[0].(map[string]any)["selector"].(map[string]any)["summary"])
}

func TestRecipeExplainBundledPrecedenceWarnsOnLocalCollision(t *testing.T) {
	tempDir := t.TempDir()
	oldWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	require.NoError(t, os.Chdir(tempDir))
	writeLocalRecipeFile(t, tempDir, "git", `schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: git
displayName: Unsafe local git
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
`)

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"recipe", "explain", "git", "--json"})

	err = cmd.Execute()
	require.NoError(t, err)
	require.Empty(t, stderr.String())
	var payload map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	recipeExplain := payload["recipeExplain"].(map[string]any)
	require.Equal(t, "bundled", recipeExplain["recipe"].(map[string]any)["source"])
	diagnostics := recipeExplain["diagnostics"].([]any)
	requireDiagnosticObjectCode(t, diagnostics, "local-recipe-shadowed")
}

func TestRecipeExplainErrorsAndExitCodes(t *testing.T) {
	tempDir := t.TempDir()
	oldWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	require.NoError(t, os.Chdir(tempDir))

	exitCode, stdout, stderr := runExecuteWithArgs(t, []string{"recipe", "explain", "missing", "--json"})
	require.Equal(t, 2, exitCode)
	require.Empty(t, stderr)
	requireRecipeExplainErrorCode(t, stdout, "unknown-target")

	exitCode, stdout, stderr = runExecuteWithArgs(t, []string{"recipe", "explain", "git:user.email", "--json"})
	require.Equal(t, 2, exitCode)
	require.Empty(t, stderr)
	requireRecipeExplainErrorCode(t, stdout, "unsupported-ref-kind")

	exitCode, stdout, stderr = runExecuteWithArgs(t, []string{"recipe", "explain", "BadTarget", "--json"})
	require.Equal(t, 2, exitCode)
	require.Empty(t, stderr)
	requireRecipeExplainErrorCode(t, stdout, "invalid-ref")

	writeLocalRecipeFile(t, tempDir, "broken", `schema: dotfiles-manager.v2.recipe
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
`)
	exitCode, stdout, stderr = runExecuteWithArgs(t, []string{"recipe", "explain", "broken", "--json"})
	require.Equal(t, 2, exitCode)
	require.Empty(t, stderr)
	requireRecipeExplainErrorCode(t, stdout, "invalid-recipe")
}

func writeLocalRecipeFile(t *testing.T, root string, recipeID string, body string) {
	t.Helper()
	path := filepath.Join(root, "recipes", "local", recipeID, "recipe.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func requireRecipeExplainErrorCode(t *testing.T, stdout string, code string) {
	t.Helper()
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, code, payload["error"].(map[string]any)["code"])
	diagnostics := payload["recipeExplain"].(map[string]any)["diagnostics"].([]any)
	requireDiagnosticObjectCode(t, diagnostics, code)
}

func requireDiagnosticObjectCode(t *testing.T, diagnostics []any, code string) {
	t.Helper()
	for _, raw := range diagnostics {
		diagnostic := raw.(map[string]any)
		if diagnostic["code"] == code {
			return
		}
	}
	require.Failf(t, "diagnostic code missing", "expected diagnostic code %q in %#v", code, diagnostics)
}
