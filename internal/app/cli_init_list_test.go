package app

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	v2initcmd "github.com/shpoont/dotfiles-manager/internal/v2/initcmd"
	v2ledger "github.com/shpoont/dotfiles-manager/internal/v2/ledger"
	v2listcmd "github.com/shpoont/dotfiles-manager/internal/v2/listcmd"
	"github.com/stretchr/testify/require"
)

func TestInitYesJSONBootstrapsRepoAndState(t *testing.T) {
	repoRoot := t.TempDir()
	setCWD(t, repoRoot)
	setTempHome(t)

	payload, stdout, stderr, err := runRootJSONCLI(t, []string{"init", "--yes", "--json"}, "")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.NotContains(t, stdout, "Accept [")
	require.Equal(t, "dotfiles-manager.v2.init", payload["schema"])
	require.Equal(t, "init", payload["command"])
	require.Equal(t, "changed", payload["summary"].(map[string]any)["status"])
	require.FileExists(t, filepath.Join(repoRoot, "dotfiles-manager.v2.yaml"))
	require.FileExists(t, filepath.Join(repoRoot, "profiles", "stacks", "default.yaml"))
	require.FileExists(t, filepath.Join(repoRoot, "profiles", "layers", "global.yaml"))
	stateRoot, stateErr := v2ledger.DefaultStateRoot(repoRoot)
	require.NoError(t, stateErr)
	require.FileExists(t, filepath.Join(stateRoot, "identity", "machine.yaml"))
}

func TestInitJSONWithoutYesFailsBeforeWritingOrPrompting(t *testing.T) {
	repoRoot := t.TempDir()
	setCWD(t, repoRoot)
	setTempHome(t)

	payload, stdout, stderr, err := runRootJSONCLI(t, []string{"init", "--json"}, "machine\nuser\n")
	require.Error(t, err)
	require.Equal(t, 4, err.(*v2initcmd.Error).ExitCode())
	require.Empty(t, stderr)
	require.NotContains(t, stdout, "Accept [")
	require.Equal(t, "blocked", payload["summary"].(map[string]any)["status"])
	require.Equal(t, "init.identity.required", payload["error"].(map[string]any)["code"])
	require.NoFileExists(t, filepath.Join(repoRoot, "dotfiles-manager.v2.yaml"))
}

func TestListJSONShowsOnlyManagedSelectionsAndProfileOverlay(t *testing.T) {
	repoRoot := setupAddCLIRepo(t, []string{"global"}, map[string]string{"global": addCLIEmptyLayer()})
	writeCLIFile(t, filepath.Join(repoRoot, "profiles", "layers", "work.yaml"), `schema: dotfiles-manager.v2.profile-layer
schemaVersion: 1
selections:
  git:
    settings:
      user.email:
        scope: user
`)
	setCWD(t, repoRoot)

	payload, stdout, stderr, err := runRootJSONCLI(t, []string{"list", "--json", "--user-id", "leon", "--profile", "work"}, "")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "dotfiles-manager.v2.list", payload["schema"])
	require.NotContains(t, stdout, "custom.files")
	list := payload["list"].(map[string]any)
	require.Equal(t, []any{"global", "work"}, list["profileStack"])
	settings := list["settings"].([]any)
	require.Len(t, settings, 1)
	setting := settings[0].(map[string]any)
	require.Equal(t, "git:user.email", setting["ref"])
	require.Equal(t, "work", setting["sourceLayer"])
	require.Equal(t, "desired://user/leon/targets/git/settings#user.email", setting["desiredUri"])
	resource := setting["resource"].(map[string]any)
	require.Equal(t, "home", resource["locationId"])
	require.Equal(t, ".gitconfig", resource["path"])
}

func TestListCLIReportsGitDesiredStateAtSettingGranularity(t *testing.T) {
	home := setTempHome(t)
	writeCLIFile(t, filepath.Join(home, ".gitconfig"), "[user]\n\temail = leon@example.com\n\tname = Leon\n")
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

	_ = runUX167JSON(t, []string{"save", "--yes", "--json", "--user-id", "leon", "git:user.email"})

	text := runUX167Text(t, []string{"list", "--user-id", "leon"})
	require.Contains(t, text, "git:user.email — User email\n    Scope: user — Me on all my machines\n    Subject: leon\n    Stored settings: stored")
	require.Contains(t, text, "git:user.name — User name\n    Scope: user — Me on all my machines\n    Subject: leon\n    Stored settings: not stored yet")
	require.Contains(t, text, "dotfiles-manager --config dotfiles-manager.v2.yaml save --dry-run --user-id leon git:user.name")

	payload := runUX167JSON(t, []string{"list", "--json", "--user-id", "leon"})
	require.Equal(t, "saved", listDesiredStateStatus(t, payload, "git:user.email"))
	require.Equal(t, true, listDesiredStateSaved(t, payload, "git:user.email"))
	require.Equal(t, "not-saved", listDesiredStateStatus(t, payload, "git:user.name"))
	require.Equal(t, false, listDesiredStateSaved(t, payload, "git:user.name"))
}

func TestListTextKeepsHappyPathVocabulary(t *testing.T) {
	repoRoot := setupAddCLIRepo(t, []string{"global"}, map[string]string{"global": `schema: dotfiles-manager.v2.profile-layer
schemaVersion: 1
selections:
  git:
    settings:
      user.email:
        scope: user
`})
	setCWD(t, repoRoot)

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"list", "--user-id", "leon"})
	err := cmd.Execute()
	require.NoError(t, err)
	require.Empty(t, stderr.String())
	text := stdout.String()
	require.Contains(t, text, "Selected settings")
	require.Contains(t, text, "git:user.email — User email")
	require.Contains(t, text, "Stored settings: not stored yet")
	require.NotContains(t, text, "desired://")
	require.NotContains(t, text, "resource=")
	require.NotContains(t, strings.ToLower(text), "resource group")
}

func TestInitAndListRootErrorsHaveStableJSON(t *testing.T) {
	projectDir := t.TempDir()
	setCWD(t, projectDir)

	payload, _, stderr, err := runRootJSONCLI(t, []string{"init", "--config", filepath.Join(projectDir, "not-v2.yaml"), "--json"}, "")
	require.Error(t, err)
	require.Equal(t, 2, err.(*v2initcmd.Error).ExitCode())
	require.Empty(t, stderr)
	require.Equal(t, "dotfiles-manager.v2.init", payload["schema"])
	require.Equal(t, "init.config.invalid", payload["error"].(map[string]any)["code"])

	payload, _, stderr, err = runRootJSONCLI(t, []string{"list", "--json"}, "")
	require.Error(t, err)
	require.Equal(t, 2, err.(*v2listcmd.Error).ExitCode())
	require.Empty(t, stderr)
	require.Equal(t, "dotfiles-manager.v2.list", payload["schema"])
	require.Equal(t, "list.root.notFound", payload["error"].(map[string]any)["code"])
}

func TestInitHonorsExplicitV2ConfigDirectory(t *testing.T) {
	projectDir := t.TempDir()
	configPath := filepath.Join(projectDir, "dotfiles-manager.v2.yaml")
	setTempHome(t)

	payload, _, stderr, err := runRootJSONCLI(t, []string{"init", "--config", configPath, "--machine-id", "mbp", "--user-id", "leon", "--json"}, "")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "changed", payload["summary"].(map[string]any)["status"])
	require.FileExists(t, configPath)
}

func listDesiredStateStatus(t *testing.T, payload map[string]any, ref string) string {
	t.Helper()
	state := listDesiredState(t, payload, ref)
	status, ok := state["status"].(string)
	require.True(t, ok, "desiredState.status for %s must be a string", ref)
	return status
}

func listDesiredStateSaved(t *testing.T, payload map[string]any, ref string) bool {
	t.Helper()
	state := listDesiredState(t, payload, ref)
	saved, ok := state["saved"].(bool)
	require.True(t, ok, "desiredState.saved for %s must be a bool", ref)
	return saved
}

func listDesiredState(t *testing.T, payload map[string]any, ref string) map[string]any {
	t.Helper()
	list := payload["list"].(map[string]any)
	settings := list["settings"].([]any)
	for _, raw := range settings {
		setting := raw.(map[string]any)
		if setting["ref"] != ref {
			continue
		}
		state, ok := setting["desiredState"].(map[string]any)
		require.True(t, ok, "desiredState for %s must be an object", ref)
		return state
	}
	require.Failf(t, "setting not found", "ref %s was not in list JSON", ref)
	return nil
}

func runRootJSONCLI(t *testing.T, args []string, stdin string) (map[string]any, string, string, error) {
	t.Helper()
	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs(args)
	err := cmd.Execute()
	var payload map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload), "stdout=%s stderr=%s", stdout.String(), stderr.String())
	return payload, stdout.String(), stderr.String(), err
}
