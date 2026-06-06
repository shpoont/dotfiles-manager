package dogfood

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	v2migration "github.com/shpoont/dotfiles-manager/internal/v2/migration"
	"github.com/stretchr/testify/require"
)

func TestRunMigrationReadinessProvesGeneratedFileAndTreeApplyRestore(t *testing.T) {
	fixture := setupDogfoodFixture(t, true)

	report, err := RunMigrationReadiness(Options{
		ConfigPath:         fixture.configPath,
		HomeDir:            fixture.homeRoot,
		StateRoot:          fixture.stateRoot,
		MigrationRunID:     "dogfood-fixture",
		AllowedLiveRoots:   []string{fixture.homeRoot},
		LocationRoot:       fixture.locationRoot,
		ApplyRunIDPrefix:   "safe-apply",
		RestoreRunIDPrefix: "safe-restore",
		StartedAt:          fixedDogfoodTime(),
	})
	require.NoError(t, err)
	require.Equal(t, Schema, report.Schema)
	require.Equal(t, SchemaVersion, report.SchemaVersion)
	require.Equal(t, "ok", report.ParityStatus)
	require.Equal(t, "ok", report.Summary.Status)
	require.Equal(t, Summary{
		Status:               "ok",
		Syncs:                2,
		ParityOK:             2,
		ApplyPreviewsClean:   2,
		RestorePreviewsClean: 2,
		AppliesVerified:      2,
		RestoresVerified:     2,
		ApplyBackups:         2,
		RestoreBackups:       2,
	}, report.Summary)
	require.Len(t, report.Items, 2)

	for _, item := range report.Items {
		require.True(t, item.ApplyPreviewClean)
		require.True(t, item.RestorePreviewClean)
		require.True(t, item.ApplyVerified)
		require.True(t, item.RestoreVerified)
		require.NotEmpty(t, item.LivePath)
		require.NotEmpty(t, item.DesiredPath)
		require.Len(t, item.ApplyBackupRefs, 1)
		require.Len(t, item.RestoreBackupRefs, 1)
		require.Contains(t, item.ApplyRunID, "safe-apply")
		require.Contains(t, item.RestoreRunID, "safe-restore")
		require.False(t, item.RecoveryAttempted)
	}

	requireFile(t, filepath.Join(fixture.homeRoot, ".gitconfig"), "live git before\n")
	requireFile(t, filepath.Join(fixture.homeRoot, ".config", "nvim", "init.lua"), "live nvim before\n")
	require.NoFileExists(t, filepath.Join(fixture.homeRoot, ".config", "nvim", "lua", "plugin.lua"))

	require.FileExists(t, filepath.Join(fixture.stateRoot, "ledger", "runs", "safe-apply-sync-0.json"))
	require.FileExists(t, filepath.Join(fixture.stateRoot, "ledger", "runs", "safe-apply-sync-1.json"))
	require.FileExists(t, filepath.Join(fixture.stateRoot, "ledger", "runs", "safe-restore-sync-0.json"))
	require.FileExists(t, filepath.Join(fixture.stateRoot, "ledger", "runs", "safe-restore-sync-1.json"))
	require.FileExists(t, filepath.Join(fixture.stateRoot, "backups", "safe-apply-sync-0", "backup.yaml"))
	require.FileExists(t, filepath.Join(fixture.stateRoot, "backups", "safe-apply-sync-1", "backup.yaml"))
	require.FileExists(t, filepath.Join(fixture.stateRoot, "backups", "safe-restore-sync-0", "backup.yaml"))
	require.FileExists(t, filepath.Join(fixture.stateRoot, "backups", "safe-restore-sync-1", "backup.yaml"))

	ledgerBody := string(readFile(t, filepath.Join(fixture.stateRoot, "ledger", "ledger.jsonl")))
	require.Len(t, strings.Split(strings.TrimSpace(ledgerBody), "\n"), 4)
	require.Contains(t, ledgerBody, `"command":"apply"`)
	require.Contains(t, ledgerBody, `"command":"restore"`)

	reportJSON, err := json.Marshal(report)
	require.NoError(t, err)
	require.NotContains(t, string(reportJSON), "live git before")
	require.NotContains(t, string(reportJSON), "desired git after")
	require.NotContains(t, string(reportJSON), "live nvim before")
	require.NotContains(t, string(reportJSON), "desired nvim after")
}

func TestRunMigrationReadinessRequiresAllowedExplicitLocationRootsBeforeApply(t *testing.T) {
	fixture := setupDogfoodFixture(t, false)
	otherRoot := filepath.Join(t.TempDir(), "other-live")
	require.NoError(t, os.MkdirAll(otherRoot, 0o755))

	report, err := RunMigrationReadiness(Options{
		ConfigPath:       fixture.configPath,
		HomeDir:          fixture.homeRoot,
		StateRoot:        fixture.stateRoot,
		MigrationRunID:   "dogfood-disallowed",
		AllowedLiveRoots: []string{fixture.homeRoot},
		LocationRoot: func(v2migration.Item) (string, error) {
			return otherRoot, nil
		},
		StartedAt: fixedDogfoodTime(),
	})
	require.Error(t, err)
	require.NotNil(t, report)
	require.Contains(t, err.Error(), "outside allowed roots")
	requireFile(t, filepath.Join(fixture.homeRoot, ".gitconfig"), "live git before\n")
	require.NoDirExists(t, fixture.stateRoot)
}

func TestRunMigrationReadinessStopsOnLocationRootErrorBeforeApply(t *testing.T) {
	fixture := setupDogfoodFixture(t, false)

	report, err := RunMigrationReadiness(Options{
		ConfigPath:       fixture.configPath,
		HomeDir:          fixture.homeRoot,
		StateRoot:        fixture.stateRoot,
		MigrationRunID:   "dogfood-location-error",
		AllowedLiveRoots: []string{fixture.homeRoot},
		LocationRoot: func(v2migration.Item) (string, error) {
			return "", errors.New("mapper failed")
		},
		StartedAt: fixedDogfoodTime(),
	})
	require.Error(t, err)
	require.NotNil(t, report)
	require.Contains(t, err.Error(), "mapper failed")
	requireFile(t, filepath.Join(fixture.homeRoot, ".gitconfig"), "live git before\n")
	require.NoDirExists(t, fixture.stateRoot)
}

func TestRunMigrationReadinessBestEffortRestoresAfterConfirmedApplyFailure(t *testing.T) {
	fixture := setupDogfoodFixture(t, false)
	hookCalls := 0

	report, err := RunMigrationReadiness(Options{
		ConfigPath:       fixture.configPath,
		HomeDir:          fixture.homeRoot,
		StateRoot:        fixture.stateRoot,
		MigrationRunID:   "dogfood-recovery",
		AllowedLiveRoots: []string{fixture.homeRoot},
		LocationRoot:     fixture.locationRoot,
		StartedAt:        fixedDogfoodTime(),
		AfterConfirmedApply: func(item ItemReport) error {
			hookCalls++
			requireFile(t, item.LivePath, "desired git after\n")
			return errors.New("simulated post-apply failure")
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "simulated post-apply failure")
	require.NotNil(t, report)
	require.Equal(t, 1, hookCalls)
	require.Len(t, report.Items, 1)
	item := report.Items[0]
	require.True(t, item.ApplyVerified)
	require.True(t, item.RecoveryAttempted)
	require.True(t, item.RecoverySucceeded)
	require.Empty(t, item.RecoveryError)
	require.Equal(t, "dogfood-recovery-sync-0", item.RecoveryRunID)
	require.Equal(t, "blocked", report.Summary.Status)
	require.Equal(t, 1, report.Summary.RecoveryAttempts)
	require.Equal(t, 1, report.Summary.RecoverySucceeded)
	requireFile(t, filepath.Join(fixture.homeRoot, ".gitconfig"), "live git before\n")
	require.FileExists(t, filepath.Join(fixture.stateRoot, "ledger", "runs", "dogfood-recovery-sync-0.json"))
}

func TestRunMigrationReadinessReportsBestEffortRecoveryFailure(t *testing.T) {
	fixture := setupDogfoodFixture(t, false)

	report, err := RunMigrationReadiness(Options{
		ConfigPath:       fixture.configPath,
		HomeDir:          fixture.homeRoot,
		StateRoot:        fixture.stateRoot,
		MigrationRunID:   "dogfood-recovery-fails",
		AllowedLiveRoots: []string{fixture.homeRoot},
		LocationRoot:     fixture.locationRoot,
		StartedAt:        fixedDogfoodTime(),
		AfterConfirmedApply: func(item ItemReport) error {
			require.NoError(t, os.RemoveAll(filepath.Join(fixture.stateRoot, "backups", item.ApplyRunID)))
			return errors.New("simulated unrecoverable post-apply failure")
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "simulated unrecoverable post-apply failure")
	require.Contains(t, err.Error(), "best-effort dogfood recovery failed")
	require.NotNil(t, report)
	require.Len(t, report.Items, 1)
	item := report.Items[0]
	require.True(t, item.ApplyVerified)
	require.True(t, item.RecoveryAttempted)
	require.False(t, item.RecoverySucceeded)
	require.Contains(t, item.RecoveryError, "locate restore backup")
	requireFile(t, filepath.Join(fixture.homeRoot, ".gitconfig"), "desired git after\n")
}

func TestRunMigrationReadinessRequiresLocationRootMapper(t *testing.T) {
	fixture := setupDogfoodFixture(t, false)
	report, err := RunMigrationReadiness(Options{
		ConfigPath:       fixture.configPath,
		HomeDir:          fixture.homeRoot,
		StateRoot:        fixture.stateRoot,
		MigrationRunID:   "dogfood-no-mapper",
		AllowedLiveRoots: []string{fixture.homeRoot},
	})
	require.Error(t, err)
	require.Nil(t, report)
	require.Contains(t, err.Error(), "location-root mapper is required")
	require.NoDirExists(t, fixture.stateRoot)
}

func TestRunMigrationReadinessValidatesRequiredInputs(t *testing.T) {
	_, err := RunMigrationReadiness(Options{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "config path is required")

	_, err = RunMigrationReadiness(Options{ConfigPath: "config.yaml"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "home dir is required")

	_, err = RunMigrationReadiness(Options{ConfigPath: "config.yaml", HomeDir: "home"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "state root is required")

	_, err = RunMigrationReadiness(Options{ConfigPath: "config.yaml", HomeDir: "home", StateRoot: "state"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "location-root mapper is required")

	_, err = normalizeAllowedRoots(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "allowed live roots are required")

	_, err = normalizeAllowedRoots([]string{""})
	require.Error(t, err)
	require.Contains(t, err.Error(), "allowed live root is required")

	_, err = normalizeAllowedRoots([]string{filepath.Join(t.TempDir(), "missing")})
	require.Error(t, err)
	require.Contains(t, err.Error(), "resolve dogfood allowed live root")
}

func TestDogfoodPathAndRunIDHelpers(t *testing.T) {
	root := t.TempDir()
	allowed, err := normalizeAllowedRoots([]string{root})
	require.NoError(t, err)

	require.NoError(t, requireAllowedPath(filepath.Join(root, "child", "file.txt"), allowed))

	regularFile := filepath.Join(root, "regular-file")
	require.NoError(t, os.WriteFile(regularFile, []byte("regular\n"), 0o644))
	require.NoError(t, requireAllowedPath(regularFile, allowed))

	err = requireAllowedPath("", allowed)
	require.Error(t, err)
	require.Contains(t, err.Error(), "live path is required")

	err = requireAllowedPath("bad\x00path", allowed)
	require.Error(t, err)
	require.Contains(t, err.Error(), "contains NUL")

	err = requireAllowedPath(filepath.Join(t.TempDir(), "outside"), allowed)
	require.Error(t, err)
	require.Contains(t, err.Error(), "outside allowed roots")

	outsideRoot := t.TempDir()
	require.NoError(t, os.Symlink(outsideRoot, filepath.Join(root, "escape-link")))
	err = requireAllowedPath(filepath.Join(root, "escape-link", "missing-parent", "file.txt"), allowed)
	require.Error(t, err)
	require.Contains(t, err.Error(), "ancestor resolves outside allowed root")

	ancestor, err := nearestExistingAncestor(filepath.Join(root, "missing", "child", "file.txt"))
	require.NoError(t, err)
	require.Equal(t, root, ancestor)

	require.True(t, sameOrInside(root, root))
	require.True(t, sameOrInside(root, filepath.Join(root, "nested")))
	require.False(t, sameOrInside(root, filepath.Dir(root)))
	require.True(t, insideAny(allowed, filepath.Join(root, "nested")))
	require.False(t, insideAny(allowed, filepath.Join(t.TempDir(), "outside")))
	require.Equal(t, "custom", prefix("custom", "fallback"))
	require.Equal(t, "fallback", prefix("", "fallback"))
	require.Equal(t, "sync-7", runIDKey("", 7))

	var zero stepClock
	require.True(t, zero.next().IsZero())
	clock := stepClock{start: fixedDogfoodTime()}
	require.Equal(t, fixedDogfoodTime(), clock.next())
	require.Equal(t, fixedDogfoodTime().Add(time.Second), clock.next())
}

type dogfoodFixture struct {
	repoRoot     string
	homeRoot     string
	stateRoot    string
	configPath   string
	locationRoot LocationRootFunc
}

func setupDogfoodFixture(t *testing.T, includeTree bool) dogfoodFixture {
	t.Helper()

	repoRoot := filepath.Join(t.TempDir(), "repo")
	homeRoot := filepath.Join(t.TempDir(), "home")
	stateRoot := filepath.Join(t.TempDir(), "state")
	require.NoError(t, os.MkdirAll(repoRoot, 0o755))
	require.NoError(t, os.MkdirAll(homeRoot, 0o755))

	writeFile(t, filepath.Join(repoRoot, "legacy", "git", ".gitconfig"), "desired git after\n")
	writeFile(t, filepath.Join(homeRoot, ".gitconfig"), "live git before\n")

	config := "syncs:\n  - source: legacy/git/.gitconfig\n    target: .gitconfig\n"
	if includeTree {
		writeFile(t, filepath.Join(repoRoot, "legacy", "nvim", "init.lua"), "desired nvim after\n")
		writeFile(t, filepath.Join(repoRoot, "legacy", "nvim", "lua", "plugin.lua"), "desired plugin after\n")
		writeFile(t, filepath.Join(homeRoot, ".config", "nvim", "init.lua"), "live nvim before\n")
		config += "  - source: legacy/nvim\n    target: .config/nvim\n"
	}
	configPath := filepath.Join(repoRoot, "dotfiles-manager.yaml")
	writeFile(t, configPath, config)

	locationRoot := func(item v2migration.Item) (string, error) {
		switch item.LegacyTarget {
		case ".gitconfig":
			return homeRoot, nil
		case ".config/nvim":
			return filepath.Join(homeRoot, ".config"), nil
		default:
			return "", fmt.Errorf("unexpected dogfood target: %s", item.LegacyTarget)
		}
	}

	return dogfoodFixture{
		repoRoot:     repoRoot,
		homeRoot:     homeRoot,
		stateRoot:    stateRoot,
		configPath:   configPath,
		locationRoot: locationRoot,
	}
}

func fixedDogfoodTime() time.Time {
	return time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
}

func writeFile(t *testing.T, path string, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
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
