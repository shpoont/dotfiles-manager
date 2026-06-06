package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/shpoont/dotfiles-manager/internal/config"
	"github.com/shpoont/dotfiles-manager/internal/dfmerr"
	"github.com/stretchr/testify/require"
)

func TestMigrateDryRunJSONUsesV2StyleAndWritesNothing(t *testing.T) {
	tempDir := t.TempDir()
	oldWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	require.NoError(t, os.Chdir(tempDir))

	configPath := filepath.Join(tempDir, config.DefaultConfigFile)
	configBody := []byte("syncs:\n  - source: dotfiles/git/.gitconfig\n    target: .gitconfig\n")
	require.NoError(t, os.WriteFile(configPath, configBody, 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "dotfiles", "git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "dotfiles", "git", ".gitconfig"), []byte("[user]\n\temail = leon@example.com\n"), 0o644))

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"migrate", "--dry-run", "--json"})

	err = cmd.Execute()
	require.NoError(t, err)
	require.Empty(t, stderr.String())
	require.Equal(t, configBody, readFile(t, configPath))
	require.NoDirExists(t, filepath.Join(tempDir, "migrations"))
	require.NoFileExists(t, filepath.Join(tempDir, "dotfiles-manager.v2.yaml"))
	require.NoDirExists(t, filepath.Join(tempDir, "profiles"))
	require.NoDirExists(t, filepath.Join(tempDir, "desired"))
	require.NoDirExists(t, filepath.Join(tempDir, "recipes"))

	var payload map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	require.Equal(t, "dotfiles-manager.v2.migration-preview", payload["schema"])
	require.Equal(t, float64(1), payload["schemaVersion"])
	require.Equal(t, "migrate", payload["command"])
	require.Equal(t, true, payload["dryRun"])
	require.NotContains(t, payload, "schema_version")
	require.NotContains(t, payload, "dry_run")

	items := payload["items"].([]any)
	require.Len(t, items, 1)
	item := items[0].(map[string]any)
	require.Equal(t, "dotfiles/git/.gitconfig", item["legacySource"])
	require.Equal(t, ".gitconfig", item["legacyTarget"])
	require.Equal(t, "custom.files", item["targetRef"])
	require.Equal(t, "custom.files:sync-0", item["settingRef"])
	require.Equal(t, "file", item["driver"])
	require.Equal(t, "leave-unchanged", item["v1ConfigAction"])
	require.Equal(t, true, item["behaviorUnchanged"])
	binding := item["desiredArtifactBinding"].(map[string]any)
	require.Equal(t, "desired://user/legacy/targets/custom.files/artifacts/sync-0", binding["uri"])
	require.NotEmpty(t, item["generatedFiles"])
	require.NotEmpty(t, payload["generatedFiles"])
}

func TestMigrateDryRunTextShowsMappingAndNoWritesBanner(t *testing.T) {
	tempDir := t.TempDir()
	oldWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	require.NoError(t, os.Chdir(tempDir))

	configPath := filepath.Join(tempDir, config.DefaultConfigFile)
	configBody := []byte("syncs:\n  - source: dotfiles/git/.gitconfig\n    target: .gitconfig\n")
	require.NoError(t, os.WriteFile(configPath, configBody, 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "dotfiles", "git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "dotfiles", "git", ".gitconfig"), []byte("[user]\n\temail = leon@example.com\n"), 0o644))

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"migrate", "--dry-run"})

	err = cmd.Execute()
	require.NoError(t, err)
	out := stdout.String()
	require.Contains(t, out, "MODE: DRY RUN (no writes)")
	require.Contains(t, out, "legacy source: dotfiles/git/.gitconfig")
	require.Contains(t, out, "legacy target: .gitconfig")
	require.Contains(t, out, "proposed: custom.files:sync-0 driver=file")
	require.Contains(t, out, "artifact binding: desired://user/legacy/targets/custom.files/artifacts/sync-0")
	require.Contains(t, out, "v1 config action: leave unchanged")
	require.Contains(t, out, "summary syncs=1 planned=1 blocked=0")
	require.Equal(t, configBody, readFile(t, configPath))
	require.NoDirExists(t, filepath.Join(tempDir, "migrations"))
}

func TestMigratePlainJSONWritesGeneratedRunAndLeavesV1Config(t *testing.T) {
	tempDir := t.TempDir()
	oldWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	require.NoError(t, os.Chdir(tempDir))

	configPath := filepath.Join(tempDir, config.DefaultConfigFile)
	configBody := []byte("syncs:\n  - source: dotfiles/git/.gitconfig\n    target: .gitconfig\n")
	require.NoError(t, os.WriteFile(configPath, configBody, 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "dotfiles", "git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "dotfiles", "git", ".gitconfig"), []byte("[user]\n\temail = leon@example.com\n"), 0o644))

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"migrate", "--json"})

	err = cmd.Execute()
	require.NoError(t, err)
	require.Empty(t, stderr.String())
	require.Equal(t, configBody, readFile(t, configPath))
	require.NoFileExists(t, filepath.Join(tempDir, "dotfiles-manager.v2.yaml"))
	require.NoDirExists(t, filepath.Join(tempDir, "profiles"))
	require.NoDirExists(t, filepath.Join(tempDir, "desired"))
	require.NoDirExists(t, filepath.Join(tempDir, "recipes"))

	var payload map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	require.Equal(t, "dotfiles-manager.v2.migration-plan", payload["schema"])
	require.Equal(t, float64(1), payload["schemaVersion"])
	require.Equal(t, "migrate", payload["command"])
	require.Equal(t, false, payload["dryRun"])
	require.NotContains(t, payload, "schema_version")
	require.NotContains(t, payload, "dry_run")
	require.Equal(t, "ok", payload["summary"].(map[string]any)["status"])
	runID := payload["runId"].(string)
	require.NotEmpty(t, runID)
	runRoot := filepath.Join(tempDir, "migrations", "v1-to-v2", runID)
	require.FileExists(t, filepath.Join(runRoot, "migration-plan.yaml"))
	require.FileExists(t, filepath.Join(runRoot, "generated", "dotfiles-manager.v2.yaml"))
	requireFile(t, filepath.Join(runRoot, "generated", "desired", "user", "legacy", "targets", "custom.files", "artifacts", "sync-0"), "[user]\n\temail = leon@example.com\n")
}

func TestMigratePlainJSONBlockedPlanWritesNothing(t *testing.T) {
	tempDir := t.TempDir()
	oldWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	require.NoError(t, os.Chdir(tempDir))

	configPath := filepath.Join(tempDir, config.DefaultConfigFile)
	configBody := []byte("syncs:\n  - source: missing/source\n    target: .missing-target\n")
	require.NoError(t, os.WriteFile(configPath, configBody, 0o644))

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"migrate", "--json"})

	err = cmd.Execute()
	require.Error(t, err)
	require.Empty(t, stderr.String())
	require.Equal(t, configBody, readFile(t, configPath))
	require.NoDirExists(t, filepath.Join(tempDir, "migrations"))
	require.NoFileExists(t, filepath.Join(tempDir, "dotfiles-manager.v2.yaml"))
	require.NoDirExists(t, filepath.Join(tempDir, "profiles"))
	require.NoDirExists(t, filepath.Join(tempDir, "desired"))
	require.NoDirExists(t, filepath.Join(tempDir, "recipes"))

	var payload map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	require.Equal(t, "dotfiles-manager.v2.migration-plan", payload["schema"])
	require.Equal(t, false, payload["dryRun"])
	require.Equal(t, "blocked", payload["summary"].(map[string]any)["status"])
	items := payload["items"].([]any)
	require.Len(t, items, 1)
	item := items[0].(map[string]any)
	require.Equal(t, "blocked", item["result"])
	diagnostics := item["diagnostics"].([]any)
	require.Equal(t, "migration-driver-unknown", diagnostics[0].(map[string]any)["code"])
}

func TestMigrateJSONConfigErrorUsesV2ErrorEnvelope(t *testing.T) {
	tempDir := t.TempDir()
	oldWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	require.NoError(t, os.Chdir(tempDir))

	missingConfigPath := filepath.Join(tempDir, "missing.yaml")
	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"--config", missingConfigPath, "migrate", "--json"})

	err = cmd.Execute()
	require.Error(t, err)
	require.Empty(t, stderr.String())

	var payload map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	require.Equal(t, "dotfiles-manager.v2.migration-plan", payload["schema"])
	require.Equal(t, float64(1), payload["schemaVersion"])
	require.Equal(t, "migrate", payload["command"])
	require.Equal(t, false, payload["dryRun"])
	require.Equal(t, missingConfigPath, payload["configPath"])
	require.Equal(t, "error", payload["summary"].(map[string]any)["status"])
	errorObj := payload["error"].(map[string]any)
	require.Equal(t, "DFM_CONFIG_NOT_FOUND", errorObj["code"])
	require.NotEmpty(t, errorObj["message"])
}

func TestEmitMigrateErrorTextAndGenericJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := errors.New("plain failure")

	emitMigrateError(&stdout, &stderr, false, false, "ignored", err)
	require.Empty(t, stdout.String())
	require.Equal(t, "plain failure\n", stderr.String())

	stdout.Reset()
	stderr.Reset()
	emitMigrateError(&stdout, &stderr, true, true, "config.yaml", err)
	require.Empty(t, stderr.String())

	var payload map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	require.Equal(t, "dotfiles-manager.v2.migration-preview", payload["schema"])
	require.Equal(t, true, payload["dryRun"])
	require.Equal(t, "config.yaml", payload["configPath"])
	errorObj := payload["error"].(map[string]any)
	require.Equal(t, "", errorObj["code"])
	require.Equal(t, "plain failure", errorObj["message"])
}

func TestParserHelpersPreserveJSONErrorContextForMigrate(t *testing.T) {
	parserErr, ok := classifyParserError(errors.New(`unknown command "bogus" for "dotfiles-manager"`))
	require.True(t, ok)
	require.Equal(t, dfmerr.CodeParserUnknownCommand, parserErr.Code)
	require.Equal(t, "bogus", parserErr.Details["command"])

	parserErr, ok = classifyParserError(errors.New("unknown shorthand flag: 'j' in -j"))
	require.True(t, ok)
	require.Equal(t, dfmerr.CodeParserUnknownFlag, parserErr.Code)
	require.Equal(t, "-j", parserErr.Details["flag"])

	parserErr, ok = classifyParserError(errors.New("flag needs an argument: --config"))
	require.True(t, ok)
	require.Equal(t, dfmerr.CodeParserArgFailure, parserErr.Code)

	parserErr, ok = classifyParserError(dfmerr.New(dfmerr.CodeConfigRequired, "not a parser error", nil))
	require.False(t, ok)
	require.Nil(t, parserErr)

	ctx := parserErrorContextFromArgs([]string{"--config=config.yaml", "migrate", "--json=false", "--dry-run=true", "--log-level", "debug", "--", "--ignored"})
	require.Equal(t, "config.yaml", ctx.ConfigPath)
	require.Equal(t, "migrate", ctx.Command)
	require.False(t, ctx.JSONOutput)
	require.True(t, ctx.DryRun)

	value, ok := parseLongFlag("--json=false", "--json")
	require.True(t, ok)
	require.NotNil(t, value)
	require.Equal(t, "false", *value)
	require.False(t, parseBoolFlagValue(value, true))
	invalidBool := "not-bool"
	require.True(t, parseBoolFlagValue(&invalidBool, true))
	require.True(t, parseBoolFlagValue(nil, true))

	require.True(t, argRequiresValue("--context"))
	require.False(t, argRequiresValue("--not-a-known-value-flag"))
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	return body
}

func requireFile(t *testing.T, path string, want string) {
	t.Helper()
	require.Equal(t, want, string(readFile(t, path)))
}
