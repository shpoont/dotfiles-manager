package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	v2ledger "github.com/shpoont/dotfiles-manager/internal/v2/ledger"
	v2recipe "github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/stretchr/testify/require"
)

func TestV2StatusFallbackWhenOnlyV2RootExists(t *testing.T) {
	fixture := setupCLIV2SelectedPreviewFixture(t, true, true)
	setCWD(t, fixture.repoRoot)

	payload, stdout, err := runSelectedPreviewCLI(t, []string{"status", "--json", "--machine-id", "mbp", "--user-id", "leon", "test.app:identity.email"})
	require.NoError(t, err)
	require.Equal(t, "dotfiles-manager.v2.preview", payload["schema"])
	require.Equal(t, "status", payload["command"])
	require.Equal(t, float64(1), payload["schemaVersion"])
	require.Equal(t, false, payload["dryRun"])
	require.NotContains(t, stdout, "current@example.com")
	require.NotContains(t, stdout, "desired@example.com")

	items := payload["items"].([]any)
	require.Len(t, items, 1)
	item := items[0].(map[string]any)
	require.Equal(t, "test.app:identity.email", item["settingRef"])
	require.Equal(t, "test.app", item["targetRef"])
	require.Equal(t, "user", item["scope"])
	require.Equal(t, "leon", item["subject"])
	require.Equal(t, false, item["mutated"])
}

func TestExplicitV2ConfigSelectsV2Status(t *testing.T) {
	fixture := setupCLIV2SelectedPreviewFixture(t, true, true)
	otherDir := t.TempDir()
	setCWD(t, otherDir)

	payload, stdout, err := runSelectedPreviewCLI(t, []string{"status", "--json", "--config", filepath.Join(fixture.repoRoot, "dotfiles-manager.v2.yaml"), "--user-id", "leon", "test.app:identity.email"})
	require.NoError(t, err)
	require.Equal(t, "dotfiles-manager.v2.preview", payload["schema"])
	require.Equal(t, "status", payload["command"])
	require.NotContains(t, stdout, "current@example.com")
	require.NotContains(t, stdout, "desired@example.com")
}

func TestV1StatusWinsWhenBothMarkersExist(t *testing.T) {
	fixture := setupCLIV2SelectedPreviewFixture(t, true, true)
	writeCLIFile(t, filepath.Join(fixture.repoRoot, ".dotfiles-manager.yaml"), "syncs:\n  - target: .config/nvim\n    source: source/nvim\n")
	writeCLIFile(t, filepath.Join(fixture.repoRoot, "source", "nvim", "init.lua"), "source\n")
	writeCLIFile(t, filepath.Join(fixture.homeDir, ".config", "nvim", "init.lua"), "target\n")
	setCWD(t, fixture.repoRoot)

	payload, _, err := runSelectedPreviewCLI(t, []string{"status", "--json"})
	require.NoError(t, err)
	require.Equal(t, "4.0", payload["schema_version"])
	require.Equal(t, "status", payload["command"])
	require.NotEqual(t, "dotfiles-manager.v2.preview", payload["schema"])
}

func TestInvalidV1ConfigDoesNotFallbackToV2(t *testing.T) {
	fixture := setupCLIV2SelectedPreviewFixture(t, true, true)
	writeCLIFile(t, filepath.Join(fixture.repoRoot, ".dotfiles-manager.yaml"), "syncs: not-a-list\n")
	setCWD(t, fixture.repoRoot)

	payload, _, err := runSelectedPreviewCLI(t, []string{"status", "--json"})
	require.Error(t, err)
	require.Equal(t, false, payload["ok"])
	require.Equal(t, "4.0", payload["schema_version"])
	require.Equal(t, "status", payload["command"])
	errorObj := payload["error"].(map[string]any)
	require.Equal(t, "DFM_CONFIG_SCHEMA_TYPE", errorObj["code"])
	require.NotEqual(t, "dotfiles-manager.v2.preview", payload["schema"])
}

func TestV2SaveApplyWithoutV2RootDoNotCreateState(t *testing.T) {
	homeDir := setTempHome(t)
	setCWD(t, t.TempDir())

	for _, command := range []string{"save", "apply"} {
		t.Run(command, func(t *testing.T) {
			payload, _, err := runSelectedPreviewCLI(t, []string{command, "--json", "test.app:identity.email"})
			require.Error(t, err)
			require.Equal(t, "dotfiles-manager.v2.preview", payload["schema"])
			require.Equal(t, command, payload["command"])
			require.Equal(t, false, payload["dryRun"])
			summary := payload["summary"].(map[string]any)
			require.Equal(t, "error", summary["status"])
			errorObj := payload["error"].(map[string]any)
			require.Equal(t, "selectedpreview.root.notFound", errorObj["code"])
		})
	}
	require.NoDirExists(t, filepath.Join(homeDir, "Library", "Application Support", "dotfiles-manager"))
}

func TestV2FlagsRejectedInV1Mode(t *testing.T) {
	projectDir := t.TempDir()
	setTempHome(t)
	setCWD(t, projectDir)
	writeCLIFile(t, filepath.Join(projectDir, ".dotfiles-manager.yaml"), "syncs:\n  - target: .config/nvim\n    source: source/nvim\n")

	payload, _, err := runSelectedPreviewCLI(t, []string{"status", "--json", "--machine-id", "mbp"})
	require.Error(t, err)
	require.Equal(t, false, payload["ok"])
	errorObj := payload["error"].(map[string]any)
	require.Equal(t, "DFM_FLAG_UNSUPPORTED", errorObj["code"])
}

func TestV2SaveDryRunPreviewDoesNotWriteDesiredArtifacts(t *testing.T) {
	fixture := setupCLIV2SelectedPreviewFixture(t, true, false)
	setCWD(t, fixture.repoRoot)
	desiredPath := filepath.Join(fixture.repoRoot, "desired", "user", "leon", "targets", "test.app", "settings.yaml")
	require.NoFileExists(t, desiredPath)

	payload, stdout, err := runSelectedPreviewCLI(t, []string{"save", "--dry-run", "--json", "--user-id", "leon", "test.app:identity.email"})
	require.NoError(t, err)
	require.Equal(t, "dotfiles-manager.v2.preview", payload["schema"])
	require.Equal(t, "save", payload["command"])
	require.Equal(t, true, payload["dryRun"])
	require.NoFileExists(t, desiredPath)
	require.NotContains(t, stdout, "current@example.com")
}

func TestV2SaveApplyLiveRequireYesForChangesAndYesMutates(t *testing.T) {
	fixture := setupCLIV2SelectedPreviewFixture(t, true, false)
	setCWD(t, fixture.repoRoot)
	desiredPath := filepath.Join(fixture.repoRoot, "desired", "user", "leon", "targets", "test.app", "settings.yaml")

	payload, stdout, err := runSelectedPreviewCLI(t, []string{"save", "--json", "--user-id", "leon", "test.app:identity.email"})
	require.Error(t, err)
	require.Equal(t, "dotfiles-manager.v2.preview", payload["schema"])
	errorObj := payload["error"].(map[string]any)
	require.Equal(t, "selectedlive.confirmationRequired", errorObj["code"])
	require.NoFileExists(t, desiredPath)
	require.NotContains(t, stdout, "current@example.com")

	payload, stdout, err = runSelectedPreviewCLI(t, []string{"save", "--yes", "--json", "--user-id", "leon", "test.app:identity.email"})
	require.NoError(t, err)
	require.Equal(t, "save", payload["command"])
	require.Equal(t, false, payload["dryRun"])
	require.FileExists(t, desiredPath)
	require.Contains(t, string(mustReadCLIFile(t, desiredPath)), "current@example.com")
	require.NotContains(t, stdout, "current@example.com")

	writeCLIFile(t, filepath.Join(fixture.liveRoot, "config.yaml"), "user:\n  email: changed-live@example.com\n")
	_, stdout, err = runSelectedPreviewCLI(t, []string{"apply", "--yes", "--json", "--user-id", "leon", "test.app:identity.email"})
	require.NoError(t, err)
	require.Contains(t, string(mustReadCLIFile(t, filepath.Join(fixture.liveRoot, "config.yaml"))), "current@example.com")
	require.NotContains(t, stdout, "current@example.com")
	require.NotContains(t, stdout, "changed-live@example.com")
}

type cliV2SelectedPreviewFixture struct {
	repoRoot string
	liveRoot string
	homeDir  string
}

func setupCLIV2SelectedPreviewFixture(t *testing.T, trusted bool, withDesired bool) cliV2SelectedPreviewFixture {
	t.Helper()
	homeDir := setTempHome(t)
	repoRoot := t.TempDir()
	liveRoot := t.TempDir()

	writeCLIFile(t, filepath.Join(repoRoot, "dotfiles-manager.v2.yaml"), "schema: dotfiles-manager.v2.root-config\nschemaVersion: 1\nactiveProfileStack: default\n")
	writeCLIFile(t, filepath.Join(repoRoot, "profiles", "stacks", "default.yaml"), "schema: dotfiles-manager.v2.profile-stack\nschemaVersion: 1\nprofileStack: [global]\n")
	writeCLIFile(t, filepath.Join(repoRoot, "profiles", "layers", "global.yaml"), "schema: dotfiles-manager.v2.profile-layer\nschemaVersion: 1\nselections:\n  test.app:\n    settings:\n      identity.email:\n        scope: user\n")
	writeCLIFile(t, filepath.Join(repoRoot, "recipes", "local", "test.app", "recipe.yaml"), cliSelectedPreviewRecipeBody(liveRoot))
	writeCLIFile(t, filepath.Join(liveRoot, "config.yaml"), "user:\n  email: current@example.com\n")
	if withDesired {
		writeCLIFile(t, filepath.Join(repoRoot, "desired", "user", "leon", "targets", "test.app", "settings.yaml"), "schema: dotfiles-manager.v2.desired-settings\nschemaVersion: 1\nvalues:\n  identity.email:\n    intent: set\n    kind: string\n    value: desired@example.com\n")
	}
	if trusted {
		rec, err := v2recipe.LoadLocal(repoRoot, "test.app")
		require.NoError(t, err)
		stateRoot, err := v2ledger.DefaultStateRoot(repoRoot)
		require.NoError(t, err)
		_, err = v2recipe.RecordLocalRecipeTrust(repoRoot, stateRoot, rec)
		require.NoError(t, err)
	}
	return cliV2SelectedPreviewFixture{repoRoot: repoRoot, liveRoot: liveRoot, homeDir: homeDir}
}

func cliSelectedPreviewRecipeBody(liveRoot string) string {
	return `schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: test.app
displayName: Test App
supportLevel: experimental
capability: read-write
locations:
  config:
    default: ` + liveRoot + `
settings:
  identity.email:
    label: User email
    supportLevel: experimental
    capability: read-write
    artifactForm: scalar
    sensitivity: personal
    redaction: redacted-for-display
    lifecycle: allowed
    scopeDefault: user
    resource: config-email
resources:
  config-email:
    driver: yaml-file
    location: config
    path: config.yaml
    capability: read-write
    sensitivity: personal
    redaction: redacted-for-display
    lifecycle: allowed
    selector:
      path: [user, email]
      createMissing: create
      duplicatePolicy: reject
      deleteKey: allow
`
}

func runSelectedPreviewCLI(t *testing.T, args []string) (map[string]any, string, error) {
	t.Helper()
	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	if stdout.Len() == 0 {
		return nil, stdout.String(), err
	}
	var payload map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload), "stdout=%s stderr=%s", stdout.String(), stderr.String())
	return payload, stdout.String(), err
}

func writeCLIFile(t *testing.T, path string, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func mustReadCLIFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}
