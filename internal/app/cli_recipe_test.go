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
	require.Contains(t, doNotManage, "credential.helper")
	require.Contains(t, doNotManage, "include and includeIf expansion")
}

func TestRecipeExplainGitTextAndCustomFilesText(t *testing.T) {
	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"recipe", "explain", "git", "--verbose"})

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
	cmd.SetArgs([]string{"recipe", "explain", "custom.files", "--verbose"})
	err = cmd.Execute()
	require.NoError(t, err)
	require.Empty(t, stderr.String())
	out = stdout.String()
	require.Contains(t, out, "target: custom.files")
	require.Contains(t, out, "custom.files:file")
	require.Contains(t, out, "custom.files:file-tree")
	require.Contains(t, out, "driver=file-tree")

	cmd = NewRootCmd()
	stdout.Reset()
	stderr.Reset()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"recipe", "explain", "starship", "--verbose"})
	err = cmd.Execute()
	require.NoError(t, err)
	require.Empty(t, stderr.String())
	out = stdout.String()
	require.Contains(t, out, "target: starship")
	require.Contains(t, out, "starship:add_newline")
	require.Contains(t, out, "selector=add_newline")
	require.Contains(t, out, "do not manage: STARSHIP_CONFIG non-default locations")
	require.NotContains(t, out, "secret@example.com")

	cmd = NewRootCmd()
	stdout.Reset()
	stderr.Reset()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"recipe", "explain", "tmux", "--verbose"})
	err = cmd.Execute()
	require.NoError(t, err)
	require.Empty(t, stderr.String())
	out = stdout.String()
	require.Contains(t, out, "target: tmux")
	require.Contains(t, out, "tmux:home.conf")
	require.Contains(t, out, "tmux:xdg.conf")
	require.Contains(t, out, "resource=home.conf driver=file")
	require.Contains(t, out, "do not manage: tmux server sockets")
	require.Contains(t, out, "does not run tmux source-file")
	require.NotContains(t, out, "secret@example.com")

	cmd = NewRootCmd()
	stdout.Reset()
	stderr.Reset()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"recipe", "explain", "zsh", "--verbose"})
	err = cmd.Execute()
	require.NoError(t, err)
	require.Empty(t, stderr.String())
	out = stdout.String()
	require.Contains(t, out, "target: zsh")
	require.Contains(t, out, "zsh:zshrc")
	require.Contains(t, out, "resource=zshrc driver=file")
	require.Contains(t, out, "do not manage: .zshenv and zsh:zshenv")
	require.Contains(t, out, "do not manage: ZDOTDIR discovery")
	require.NotContains(t, out, "secret@example.com")
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
	require.Contains(t, out, "nvim source=bundled")
	require.Contains(t, out, "ssh source=bundled")
	require.Contains(t, out, "starship source=bundled")
	require.Contains(t, out, "tmux source=bundled")
	require.Contains(t, out, "zsh source=bundled")
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
	require.Len(t, targets, 7)
	require.Equal(t, "custom.files", targets[0].(map[string]any)["id"])
	require.Equal(t, "git", targets[1].(map[string]any)["id"])
	require.Equal(t, "nvim", targets[2].(map[string]any)["id"])
	require.Equal(t, "ssh", targets[3].(map[string]any)["id"])
	require.Equal(t, "starship", targets[4].(map[string]any)["id"])
	require.Equal(t, "tmux", targets[5].(map[string]any)["id"])
	require.Equal(t, "zsh", targets[6].(map[string]any)["id"])
	require.Equal(t, "bundled", targets[1].(map[string]any)["source"])
	require.Equal(t, "trusted", targets[1].(map[string]any)["trustStatus"])
	require.NotContains(t, stdout.String(), "config-present")
	require.NotContains(t, stdout.String(), "recipe.discover")
}

func TestRecipeDiscoverTextAndJSON(t *testing.T) {
	tempDir := t.TempDir()
	home := filepath.Join(tempDir, "home")
	binDir := filepath.Join(tempDir, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	require.NoError(t, os.MkdirAll(home, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".gitconfig"), []byte("[user]\n\temail = secret@example.com\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "git"), []byte("#!/bin/sh\nexit 0\n"), 0o755))
	t.Setenv("HOME", home)
	t.Setenv("PATH", binDir)

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"recipe", "discover", "git", "--verbose"})

	err := cmd.Execute()
	require.NoError(t, err)
	require.Empty(t, stderr.String())
	out := stdout.String()
	require.Contains(t, out, "recipe discover")
	require.Contains(t, out, "git state=config-present")
	require.Contains(t, out, "commands=git:installed")
	require.Contains(t, out, "configs=home:.gitconfig:present")
	require.NotContains(t, out, "secret@example.com")
	require.NotContains(t, out, home)

	cmd = NewRootCmd()
	stdout.Reset()
	stderr.Reset()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"recipe", "discover", "git", "--json"})

	err = cmd.Execute()
	require.NoError(t, err)
	require.Empty(t, stderr.String())
	require.NotContains(t, stdout.String(), "secret@example.com")
	require.NotContains(t, stdout.String(), home)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	require.Equal(t, "recipe.discover", payload["command"])
	require.Equal(t, float64(1), payload["schemaVersion"])
	discovery := payload["discovery"].(map[string]any)
	targets := discovery["targets"].([]any)
	require.Len(t, targets, 1)
	target := targets[0].(map[string]any)
	require.Equal(t, "git", target["id"])
	require.Equal(t, "config-present", target["state"])
	require.Equal(t, "installed", target["binaryState"])
	require.Equal(t, "config-present", target["configState"])
}

func TestRecipeDiscoverUnknownTargetEmitsStableJSONError(t *testing.T) {
	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"recipe", "discover", "missing", "--json"})

	err := cmd.Execute()
	require.Error(t, err)
	require.Empty(t, stderr.String())
	var payload map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	require.Equal(t, "recipe.discover", payload["command"])
	require.Equal(t, "error", payload["summary"].(map[string]any)["status"])
	require.Equal(t, "recipe.discover.unknown-target", payload["error"].(map[string]any)["code"])
}

func TestRecipeExplainStarshipJSONIsMetadataOnly(t *testing.T) {
	tempDir := t.TempDir()
	oldWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	require.NoError(t, os.Chdir(tempDir))
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, ".config"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, ".config", "starship.toml"), []byte("format = 'secret-prompt-format'\nadd_newline = true\n"), 0o644))

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"recipe", "explain", "starship", "--json"})

	err = cmd.Execute()
	require.NoError(t, err)
	require.Empty(t, stderr.String())
	require.NotContains(t, stdout.String(), "secret-prompt-format")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	recipeExplain := payload["recipeExplain"].(map[string]any)
	target := recipeExplain["target"].(map[string]any)
	require.Equal(t, "starship", target["ref"])
	require.Equal(t, "unknown", target["platformSupport"])
	recipeObj := recipeExplain["recipe"].(map[string]any)
	require.Equal(t, "bundled", recipeObj["source"])
	require.Equal(t, "recipe://bundled/starship", recipeObj["recipeRef"])
	settings := recipeExplain["settings"].([]any)
	require.Len(t, settings, 4)
	require.Equal(t, "starship:add_newline", settings[0].(map[string]any)["ref"])
	require.Equal(t, "starship:command_timeout", settings[1].(map[string]any)["ref"])
	require.Equal(t, "starship:follow_symlinks", settings[2].(map[string]any)["ref"])
	require.Equal(t, "starship:scan_timeout", settings[3].(map[string]any)["ref"])
	resources := recipeExplain["resources"].([]any)
	require.Len(t, resources, 4)
	require.Equal(t, "toml-file", resources[0].(map[string]any)["driverId"])
	selector := resources[0].(map[string]any)["selector"].(map[string]any)
	require.Equal(t, []any{"add_newline"}, selector["path"].([]any))
	require.Equal(t, "create", selector["createMissing"])
	require.Equal(t, "allow", selector["deleteKey"])
}

func TestRecipeExplainZshJSONIsMetadataOnly(t *testing.T) {
	tempDir := t.TempDir()
	oldWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	require.NoError(t, os.Chdir(tempDir))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, ".zshrc"), []byte("export SECRET_LIKE_ZSHRC=value\n"), 0o644))

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"recipe", "explain", "zsh", "--json"})

	err = cmd.Execute()
	require.NoError(t, err)
	require.Empty(t, stderr.String())
	require.NotContains(t, stdout.String(), "SECRET_LIKE_ZSHRC")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	recipeExplain := payload["recipeExplain"].(map[string]any)
	target := recipeExplain["target"].(map[string]any)
	require.Equal(t, "zsh", target["ref"])
	require.Equal(t, "unknown", target["platformSupport"])
	recipeObj := recipeExplain["recipe"].(map[string]any)
	require.Equal(t, "bundled", recipeObj["source"])
	require.Equal(t, "recipe://bundled/zsh", recipeObj["recipeRef"])
	settings := recipeExplain["settings"].([]any)
	require.Len(t, settings, 4)
	require.Equal(t, "zsh:zshrc", settings[0].(map[string]any)["ref"])
	require.Equal(t, "file", settings[0].(map[string]any)["artifactForm"])
	require.Equal(t, "user", settings[0].(map[string]any)["defaultScope"])
	resources := recipeExplain["resources"].([]any)
	require.Len(t, resources, 4)
	require.Equal(t, ".zshrc", resources[0].(map[string]any)["path"])
	require.Equal(t, "file", resources[0].(map[string]any)["driverId"])
	require.Nil(t, resources[0].(map[string]any)["selector"])
	safety := recipeExplain["safety"].(map[string]any)
	doNotManage := safety["doNotManage"].([]any)
	require.Contains(t, doNotManage, ".zshenv and zsh:zshenv are blocked because .zshenv affects almost every zsh invocation")
	require.Contains(t, doNotManage, ".zsh_history and .zhistory")
	require.Contains(t, doNotManage, ".zcompdump* completion dump files")
	require.Contains(t, doNotManage, "ZDOTDIR discovery or non-default Zsh locations")
}

func TestRecipeExplainTmuxJSONIsMetadataOnly(t *testing.T) {
	tempDir := t.TempDir()
	oldWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	require.NoError(t, os.Chdir(tempDir))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, ".tmux.conf"), []byte("set -g @secret_plugin_token value\n"), 0o644))

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"recipe", "explain", "tmux", "--json"})

	err = cmd.Execute()
	require.NoError(t, err)
	require.Empty(t, stderr.String())
	require.NotContains(t, stdout.String(), "@secret_plugin_token")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	recipeExplain := payload["recipeExplain"].(map[string]any)
	target := recipeExplain["target"].(map[string]any)
	require.Equal(t, "tmux", target["ref"])
	require.Equal(t, "linux-darwin", target["platformSupport"])
	recipeObj := recipeExplain["recipe"].(map[string]any)
	require.Equal(t, "bundled", recipeObj["source"])
	require.Equal(t, "recipe://bundled/tmux", recipeObj["recipeRef"])
	settings := recipeExplain["settings"].([]any)
	require.Len(t, settings, 2)
	require.Equal(t, "tmux:home.conf", settings[0].(map[string]any)["ref"])
	require.Equal(t, "file", settings[0].(map[string]any)["artifactForm"])
	require.Equal(t, "warn", settings[0].(map[string]any)["lifecycle"])
	require.Equal(t, "tmux:xdg.conf", settings[1].(map[string]any)["ref"])
	resources := recipeExplain["resources"].([]any)
	require.Len(t, resources, 2)
	require.Equal(t, ".tmux.conf", resources[0].(map[string]any)["path"])
	require.Equal(t, "file", resources[0].(map[string]any)["driverId"])
	require.Nil(t, resources[0].(map[string]any)["selector"])
	require.Equal(t, "tmux/tmux.conf", resources[1].(map[string]any)["path"])
	require.Nil(t, resources[1].(map[string]any)["selector"])
	safety := recipeExplain["safety"].(map[string]any)
	doNotManage := safety["doNotManage"].([]any)
	require.Contains(t, doNotManage, "tmux server sockets, clients, sessions, windows, panes, and runtime state")
	require.Contains(t, doNotManage, "manual reload actions such as tmux source-file, server restart, or session mutation")
}

func TestRecipeExplainNvimJSONIsMetadataOnly(t *testing.T) {
	tempDir := t.TempDir()
	oldWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	require.NoError(t, os.Chdir(tempDir))
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, ".config", "nvim"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, ".config", "nvim", "init.lua"), []byte("vim.g.SECRET_LIKE_NVIM = 'value'\n"), 0o644))

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"recipe", "explain", "nvim", "--json"})

	err = cmd.Execute()
	require.NoError(t, err)
	require.Empty(t, stderr.String())
	require.NotContains(t, stdout.String(), "SECRET_LIKE_NVIM")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	recipeExplain := payload["recipeExplain"].(map[string]any)
	target := recipeExplain["target"].(map[string]any)
	require.Equal(t, "nvim", target["ref"])
	require.Equal(t, "linux-darwin", target["platformSupport"])
	recipeObj := recipeExplain["recipe"].(map[string]any)
	require.Equal(t, "bundled", recipeObj["source"])
	require.Equal(t, "recipe://bundled/nvim", recipeObj["recipeRef"])
	settings := recipeExplain["settings"].([]any)
	require.Len(t, settings, 1)
	require.Equal(t, "nvim:config", settings[0].(map[string]any)["ref"])
	require.Equal(t, "file-tree", settings[0].(map[string]any)["artifactForm"])
	require.Equal(t, "user", settings[0].(map[string]any)["defaultScope"])
	resources := recipeExplain["resources"].([]any)
	require.Len(t, resources, 1)
	require.Equal(t, "nvim", resources[0].(map[string]any)["path"])
	require.Equal(t, "file-tree", resources[0].(map[string]any)["driverId"])
	require.Nil(t, resources[0].(map[string]any)["selector"])
	excludes := resources[0].(map[string]any)["exclude"].([]any)
	require.Contains(t, excludes, "shada/**")
	require.NotContains(t, excludes, "**/*secret*")
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
