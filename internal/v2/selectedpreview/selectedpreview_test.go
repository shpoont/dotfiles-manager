package selectedpreview

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shpoont/dotfiles-manager/internal/v2/desired"
	"github.com/shpoont/dotfiles-manager/internal/v2/macosdefaultsdriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/nativeexport"
	"github.com/shpoont/dotfiles-manager/internal/v2/nativeops"
	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/shpoont/dotfiles-manager/internal/v2/resolution"
	"github.com/shpoont/dotfiles-manager/internal/v2/selectedvalue"
	v2status "github.com/shpoont/dotfiles-manager/internal/v2/status"
	"github.com/stretchr/testify/require"
	"howett.net/plist"
)

func TestBuildStatusDiffAndDryRunReportsSelectedValueWithoutRawValues(t *testing.T) {
	t.Parallel()

	fixture := setupFixture(t)
	fixture.writeLiveYAML("current@example.com")
	fixture.writeDesiredSet("desired@example.com")
	fixture.trustRecipe()

	for _, command := range []string{CommandStatus, CommandDiff, CommandSave, CommandApply} {
		t.Run(command, func(t *testing.T) {
			report, err := Build(Options{Command: command, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "test.app:identity.email", UserID: "leon", MachineID: "mbp", DryRun: command == CommandSave || command == CommandApply})
			require.NoError(t, err)
			require.Len(t, report.Items, 1)
			item := report.Items[0]
			require.Equal(t, "test.app:identity.email", item.SettingRef)
			require.Equal(t, desired.StatusPresent, item.Desired.Status)
			require.True(t, item.Current.Exists)
			require.False(t, item.Mutated)
			require.NotContains(t, mustJSON(t, report), "current@example.com")
			require.NotContains(t, mustJSON(t, report), "desired@example.com")
			require.NotContains(t, Text(report), "current@example.com")
			require.NotContains(t, Text(report), "desired@example.com")
			if command == CommandDiff {
				require.NotNil(t, item.Diff)
				require.Equal(t, "metadata-only", item.Diff.Mode)
			}
			if command == CommandSave || command == CommandApply {
				require.True(t, report.DryRun)
				require.Contains(t, item.PlannedAction, "would-")
			}
		})
	}
}

func TestSelectedPreviewHelperBranches(t *testing.T) {
	t.Parallel()

	require.Equal(t, "recipe://bundled/ssh", recipeRef(recipe.RecipeSourceBundled, recipe.SSHTarget))
	require.Equal(t, "recipe://local/test.app", recipeRef(recipe.RecipeSourceLocal, "test.app"))
	require.Equal(t, "", recipeRef("remote", "test.app"))

	require.True(t, isSelectedValueDriver(recipe.JSONFileDriverID))
	require.False(t, isSelectedValueDriver(recipe.FileDriverID))
	require.Equal(t, "unknown", fileDiffInfo("").Kind)
	require.Equal(t, "update", fileDiffInfo("update").Kind)
	require.Equal(t, "unknown", treeDiffInfo("").Kind)
	require.Equal(t, "create", treeDiffInfo("create").Kind)
	require.Equal(t, "default", fallback(" ", "default"))
	require.Equal(t, "explicit", fallback("explicit", "default"))
	require.Equal(t, "existing", (Diagnostic{Ref: "existing"}).withRef("new").Ref)
	require.Equal(t, "new", (Diagnostic{}).withRef("new").Ref)
}

func TestDesiredValueFromSelectedBranches(t *testing.T) {
	t.Parallel()

	deleted, err := desiredValueFromSelected(selectedvalue.Delete())
	require.NoError(t, err)
	require.Equal(t, desired.IntentDelete, deleted.Intent())

	str, err := desiredValueFromSelected(selectedvalue.SetString("value"))
	require.NoError(t, err)
	require.Equal(t, desired.IntentSet, str.Intent())
	require.Equal(t, "string", str.Kind())

	boolean, err := desiredValueFromSelected(selectedvalue.SetBool(true))
	require.NoError(t, err)
	require.Equal(t, desired.IntentSet, boolean.Intent())
	require.Equal(t, "bool", boolean.Kind())

	number, err := desiredValueFromSelected(selectedvalue.SetNumber(json.Number("1.25")))
	require.NoError(t, err)
	require.Equal(t, desired.IntentSet, number.Intent())
	require.Equal(t, "number", number.Kind())

	nullValue, err := desiredValueFromSelected(selectedvalue.SetNull())
	require.NoError(t, err)
	require.Equal(t, desired.IntentSet, nullValue.Intent())
	require.Equal(t, "null", nullValue.Kind())

	_, err = desiredValueFromSelected(selectedvalue.Desired{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "intent is required")
}

func TestBuildBlocksLocalRecipeWithoutTrustBeforeLiveRead(t *testing.T) {
	t.Parallel()

	fixture := setupFixture(t)
	fixture.writeLiveYAML("secret-current@example.com")
	fixture.writeDesiredSet("desired@example.com")

	report, err := Build(Options{Command: CommandStatus, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, UserID: "leon"})
	require.NoError(t, err)
	require.Equal(t, SummaryBlocked, report.Summary.Status)
	require.Len(t, report.Items, 1)
	item := report.Items[0]
	require.Equal(t, v2status.StateBlockedSafety, item.State)
	require.Empty(t, item.Current.SHA256)
	requireDiagnostic(t, item, "trust.local.missingRecord")
	require.NotContains(t, mustJSON(t, report), "secret-current@example.com")
}

func TestBuildDistinguishesUnmanagedDesiredFromMissing(t *testing.T) {
	t.Parallel()

	fixture := setupFixture(t)
	fixture.writeDesiredUnmanaged()
	fixture.trustRecipe()

	report, err := Build(Options{Command: CommandStatus, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, UserID: "leon"})
	require.NoError(t, err)
	require.Equal(t, SummaryOK, report.Summary.Status)
	require.Len(t, report.Items, 1)
	item := report.Items[0]
	require.Equal(t, desired.StatusUnmanaged, item.Desired.Status)
	require.True(t, item.Desired.Unmanaged)
	require.Equal(t, v2status.StateUnchanged, item.State)
	require.Contains(t, item.Message, "intentionally unmanaged")
}

func TestBuildKeepsUnsupportedBundledRuntimeExplicitForNonExecutableTargets(t *testing.T) {
	t.Parallel()

	fixture := setupFixtureForTarget(t, recipe.CustomFilesTarget, "file")
	report, err := Build(Options{Command: CommandStatus, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, UserID: "leon"})
	require.NoError(t, err)
	require.Equal(t, SummaryBlocked, report.Summary.Status)
	require.Len(t, report.Items, 1)
	item := report.Items[0]
	require.Equal(t, recipe.RecipeSourceBundled, item.Recipe.Source)
	require.Equal(t, v2status.StateUnsupported, item.State)
	requireDiagnostic(t, item, "selectedpreview.recipe.bundledRuntimeUnavailable")
}

func TestBuildUsesBundledGitRuntimeForSelectedIdentitySettings(t *testing.T) {
	t.Parallel()

	fixture := setupFixtureForTarget(t, recipe.GitTarget, "user.email")
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".gitconfig"), "[credential]\n\thelper = store-secret-helper\n[user]\n\temail = current@example.com\n")
	fixture.writeDesiredSetFor(recipe.GitTarget, "user.email", "desired@example.com")
	roots := map[string]map[string]string{recipe.GitTarget: {"home": home}}

	for _, command := range []string{CommandStatus, CommandDiff, CommandSave, CommandApply} {
		t.Run(command, func(t *testing.T) {
			report, err := Build(Options{Command: command, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "git:user.email", UserID: "leon", DryRun: command == CommandSave || command == CommandApply, LocationRoots: roots})
			require.NoError(t, err)
			require.Len(t, report.Items, 1)
			item := report.Items[0]
			require.Equal(t, recipe.RecipeSourceBundled, item.Recipe.Source)
			require.Equal(t, "recipe://bundled/git", item.Recipe.RecipeRef)
			require.Equal(t, recipe.TrustStatusTrusted, item.Recipe.TrustStatus)
			switch command {
			case CommandSave:
				require.Equal(t, v2status.StateChangedCurrent, item.State)
			case CommandApply:
				require.Equal(t, v2status.StateReadyToApply, item.State)
			default:
				require.Equal(t, v2status.StateUnknown, item.State)
				require.True(t, item.NoBaseline)
			}
			require.True(t, item.Current.Exists)
			require.Equal(t, ".gitconfig", item.Resource.RelPath)
			require.Equal(t, "[user] email", item.Selector.Summary)
			if command == CommandDiff {
				require.NotNil(t, item.Diff)
				require.Equal(t, "metadata-only", item.Diff.Mode)
			}
			payload := mustJSON(t, report)
			require.NotContains(t, payload, "current@example.com")
			require.NotContains(t, payload, "desired@example.com")
			require.NotContains(t, payload, "store-secret-helper")
			require.NotContains(t, Text(report), "current@example.com")
		})
	}
}

func TestBuildUsesBundledZshFileResourceRuntime(t *testing.T) {
	t.Parallel()

	fixture := setupZshFixture(t, "zshrc")
	writeFile(t, filepath.Join(fixture.liveRoot, ".zshrc"), "raw-live-zshrc\n")
	writeFileResourceDesired(t, fixture, recipe.ZshTarget, "zshrc", "raw-desired-zshrc\n")
	roots := map[string]map[string]string{recipe.ZshTarget: {"home": fixture.liveRoot}}

	for _, command := range []string{CommandStatus, CommandDiff, CommandSave, CommandApply} {
		t.Run(command, func(t *testing.T) {
			report, err := Build(Options{Command: command, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "zsh:zshrc", UserID: "leon", DryRun: command == CommandSave || command == CommandApply, LocationRoots: roots})
			require.NoError(t, err)
			require.Len(t, report.Items, 1)
			item := report.Items[0]
			require.Equal(t, recipe.RecipeSourceBundled, item.Recipe.Source)
			require.Equal(t, "recipe://bundled/zsh", item.Recipe.RecipeRef)
			require.Equal(t, recipe.TrustStatusTrusted, item.Recipe.TrustStatus)
			require.Equal(t, "zsh:zshrc", item.SettingRef)
			require.Equal(t, recipe.FileDriverID, item.Resource.DriverID)
			require.Equal(t, "zshrc", item.Resource.ID)
			require.Equal(t, "home", item.Resource.LocationID)
			require.Equal(t, ".zshrc", item.Resource.RelPath)
			require.Equal(t, SelectorInfo{Kind: "file", Summary: ".zshrc"}, item.Selector)
			require.Equal(t, "desired://user/leon/targets/zsh/artifacts/zshrc", item.DesiredURI)
			require.Equal(t, filepath.ToSlash(filepath.Join("desired", "user", "leon", "targets", "zsh", "artifacts", "zshrc")), item.DesiredRelPath)
			require.True(t, item.Current.Exists)
			require.Equal(t, "present", item.Desired.Status)
			require.Equal(t, "file", item.Desired.Kind)
			require.True(t, item.Desired.Snapshot.Exists)
			require.NotNil(t, item.Preview)
			if command == CommandDiff {
				require.NotNil(t, item.Diff)
				require.Equal(t, "metadata-only", item.Diff.Mode)
				require.Equal(t, "raw file contents omitted", item.Diff.Redaction)
			}
			if command == CommandSave || command == CommandApply {
				requireDiagnostic(t, item, recipe.ZshRiskShellStartupFileCode)
				require.NotEqual(t, SummaryBlocked, report.Summary.Status)
			} else {
				requireNoDiagnostic(t, item, recipe.ZshRiskShellStartupFileCode)
			}
			payload := mustJSON(t, report)
			require.NotContains(t, payload, "raw-live-zshrc")
			require.NotContains(t, payload, "raw-desired-zshrc")
			require.NotContains(t, Text(report), "raw-live-zshrc")
			require.NotContains(t, Text(report), "raw-desired-zshrc")
		})
	}
}

func TestBuildNativeExportDiffAndSaveDryRunUseMetadataOnlyArtifacts(t *testing.T) {
	t.Parallel()

	fixture := setupNativeExportFixture(t, false)
	fixture.trustNativeRecipe()
	executor := &recordingNativeExecutor{body: "native-export-secret"}

	diffReport, err := Build(Options{
		Command:        CommandDiff,
		RepoRoot:       fixture.repoRoot,
		StateRoot:      fixture.stateRoot,
		Ref:            "native.app:settings",
		UserID:         "leon",
		MachineID:      "mbp",
		NativeExecutor: executor,
		Now:            fixedPreviewTime,
	})
	require.NoError(t, err)
	require.Equal(t, 1, executor.calls)
	require.Len(t, diffReport.Items, 1)
	diffItem := diffReport.Items[0]
	require.Equal(t, recipe.NativeExportDriverID, diffItem.Resource.DriverID)
	require.NotNil(t, diffItem.Diff)
	require.Equal(t, "metadata-only", diffItem.Diff.Mode)
	require.Contains(t, diffItem.Diff.Message, "internal app settings are not semantically compared")
	require.True(t, diffItem.Current.Exists)
	require.Equal(t, "missing", diffItem.Desired.Status)
	require.NoDirExists(t, fixture.desiredArtifactPath())
	require.NotContains(t, mustJSON(t, diffReport), "native-export-secret")
	require.NotContains(t, Text(diffReport), "native-export-secret")

	saveReport, err := Build(Options{
		Command:        CommandSave,
		RepoRoot:       fixture.repoRoot,
		StateRoot:      fixture.stateRoot,
		Ref:            "native.app:settings",
		UserID:         "leon",
		MachineID:      "mbp",
		DryRun:         true,
		NativeExecutor: executor,
		Now:            fixedPreviewTime,
	})
	require.NoError(t, err)
	require.Equal(t, 2, executor.calls)
	require.Equal(t, PlannedActionWouldPromote, saveReport.Items[0].PlannedAction)
	require.NoDirExists(t, fixture.desiredArtifactPath())
	require.NotContains(t, mustJSON(t, saveReport), "native-export-secret")
}

func TestBuildNativeExportStatusAndApplyDoNotExecuteRunner(t *testing.T) {
	t.Parallel()

	fixture := setupNativeExportFixture(t, false)
	fixture.trustNativeRecipe()
	executor := &recordingNativeExecutor{body: "must-not-run"}

	statusReport, err := Build(Options{Command: CommandStatus, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "native.app:settings", UserID: "leon", NativeExecutor: executor})
	require.NoError(t, err)
	require.Equal(t, 0, executor.calls)
	require.Equal(t, v2status.StateUnknown, statusReport.Items[0].State)
	require.Contains(t, statusReport.Items[0].Message, "does not run the export operation")

	applyReport, err := Build(Options{Command: CommandApply, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "native.app:settings", UserID: "leon", DryRun: true, NativeExecutor: executor})
	require.NoError(t, err)
	require.Equal(t, 0, executor.calls)
	require.Equal(t, v2status.StateUnsupported, applyReport.Items[0].State)
	requireDiagnostic(t, applyReport.Items[0], "selectedpreview.nativeExport.applyUnsupported")
}

func TestBuildNativeApplyDryRunPlansWithoutExecutingRunner(t *testing.T) {
	t.Parallel()

	fixture := setupNativeApplyFixture(t)
	fixture.trustNativeRecipe()
	fixture.writeNativeDesiredArtifact("desired-native-secret")
	executor := &recordingNativeExecutor{body: "must-not-run"}

	report, err := Build(Options{
		Command:        CommandApply,
		RepoRoot:       fixture.repoRoot,
		StateRoot:      fixture.stateRoot,
		Ref:            "native.app:settings",
		UserID:         "leon",
		MachineID:      "mbp",
		DryRun:         true,
		NativeExecutor: executor,
	})
	require.NoError(t, err)
	require.Equal(t, 0, executor.calls)
	require.Len(t, report.Items, 1)
	item := report.Items[0]
	require.Equal(t, v2status.StateReadyToApply, item.State)
	require.Equal(t, PlannedActionWouldApply, item.PlannedAction)
	require.NotNil(t, item.NativeExport)
	require.True(t, item.NativeExport.ApplySupported)
	require.Equal(t, "import-settings", item.NativeExport.ImportOperationID)
	require.Equal(t, "pre-apply-export", item.NativeExport.BackupPolicy)
	require.Equal(t, "post-import-export-hash", item.NativeExport.VerifyPolicy)
	require.NotContains(t, mustJSON(t, report), "desired-native-secret")
}

func TestBuildNativeExportReviewGateBlocksBeforeRunner(t *testing.T) {
	t.Parallel()

	fixture := setupNativeExportFixture(t, true)
	fixture.trustNativeRecipe()
	executor := &recordingNativeExecutor{body: "reviewed"}

	blocked, err := Build(Options{Command: CommandSave, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "native.app:settings", UserID: "leon", DryRun: true, NativeExecutor: executor})
	require.NoError(t, err)
	require.Equal(t, SummaryBlocked, blocked.Summary.Status)
	require.Equal(t, 0, executor.calls)
	requireDiagnostic(t, blocked.Items[0], "nativeexport.review.required")

	confirmed, err := Build(Options{Command: CommandSave, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "native.app:settings", UserID: "leon", DryRun: true, Confirmed: true, NativeExecutor: executor, Now: fixedPreviewTime})
	require.NoError(t, err)
	require.NotEqual(t, SummaryBlocked, confirmed.Summary.Status)
	require.Equal(t, 1, executor.calls)
}

func TestBuildNativeExportUntrustedLocalRecipeDoesNotExecuteRunner(t *testing.T) {
	t.Parallel()

	fixture := setupNativeExportFixture(t, false)
	executor := &recordingNativeExecutor{body: "must-not-run"}

	report, err := Build(Options{Command: CommandDiff, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "native.app:settings", UserID: "leon", NativeExecutor: executor})
	require.NoError(t, err)
	require.Equal(t, SummaryBlocked, report.Summary.Status)
	require.Equal(t, 0, executor.calls)
	requireDiagnostic(t, report.Items[0], "trust.local.missingRecord")
}

func TestBuildZshOptInStartupFilesEmitWriteWarnings(t *testing.T) {
	t.Parallel()

	for _, settingID := range []string{"zprofile", "zlogin", "zlogout"} {
		t.Run(settingID, func(t *testing.T) {
			t.Parallel()

			fixture := setupZshFixture(t, settingID)
			path := "." + settingID
			writeFile(t, filepath.Join(fixture.liveRoot, path), "raw-live-"+settingID+"\n")
			writeFileResourceDesired(t, fixture, recipe.ZshTarget, settingID, "raw-desired-"+settingID+"\n")
			roots := map[string]map[string]string{recipe.ZshTarget: {"home": fixture.liveRoot}}

			for _, command := range []string{CommandSave, CommandApply} {
				report, err := Build(Options{Command: command, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "zsh:" + settingID, UserID: "leon", DryRun: true, LocationRoots: roots})
				require.NoError(t, err)
				require.Len(t, report.Items, 1)
				item := report.Items[0]
				require.Equal(t, "zsh:"+settingID, item.SettingRef)
				require.Equal(t, path, item.Resource.RelPath)
				requireDiagnostic(t, item, recipe.ZshRiskShellStartupFileCode)
				require.NotEqual(t, SummaryBlocked, report.Summary.Status)
				payload := mustJSON(t, report)
				require.NotContains(t, payload, "raw-live-"+settingID)
				require.NotContains(t, payload, "raw-desired-"+settingID)
			}
		})
	}
}

func TestBuildZshBlockedRefsDoNotReadRawFiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		settingID string
		code      string
		livePath  string
	}{
		{settingID: "zshenv", code: recipe.ZshBlockedZshenvCode, livePath: ".zshenv"},
		{settingID: "history", code: recipe.ZshBlockedHistoryCode, livePath: ".zsh_history"},
		{settingID: "zcompdump", code: recipe.ZshBlockedCompletionCacheCode, livePath: ".zcompdump"},
		{settingID: "cache", code: recipe.ZshBlockedCompletionCacheCode, livePath: filepath.Join(".cache", "zsh", "raw.cache")},
		{settingID: "oh-my-zsh", code: recipe.ZshBlockedPluginStateCode, livePath: filepath.Join(".oh-my-zsh", "custom", "raw.zsh")},
		{settingID: "custom", code: recipe.ZshBlockedPluginStateCode, livePath: filepath.Join(".oh-my-zsh", "custom", "raw.zsh")},
		{settingID: "zsh-sessions", code: recipe.ZshBlockedSessionStateCode, livePath: filepath.Join(".zsh_sessions", "raw.session")},
	}
	for _, tc := range tests {
		t.Run(tc.settingID, func(t *testing.T) {
			t.Parallel()

			fixture := setupZshFixture(t, tc.settingID)
			raw := "RAW-BLOCKED-ZSH-" + tc.settingID
			writeFile(t, filepath.Join(fixture.liveRoot, tc.livePath), raw+"\n")
			roots := map[string]map[string]string{recipe.ZshTarget: {"home": fixture.liveRoot}}

			report, err := Build(Options{Command: CommandStatus, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "zsh:" + tc.settingID, UserID: "leon", LocationRoots: roots})
			require.NoError(t, err)
			require.Equal(t, SummaryBlocked, report.Summary.Status)
			require.Len(t, report.Items, 1)
			item := report.Items[0]
			require.Equal(t, v2status.StateUnsupported, item.State)
			require.False(t, item.Current.Exists)
			require.Empty(t, item.Current.SHA256)
			requireDiagnostic(t, item, tc.code)
			require.NotContains(t, mustJSON(t, report), raw)
			require.NotContains(t, Text(report), raw)
		})
	}
}

func TestBuildUsesBundledTmuxFileResourceRuntime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		settingID  string
		locationID string
		relPath    string
	}{
		{settingID: "home.conf", locationID: "home", relPath: ".tmux.conf"},
		{settingID: "xdg.conf", locationID: "config", relPath: filepath.Join("tmux", "tmux.conf")},
	}
	for _, tc := range tests {
		t.Run(tc.settingID, func(t *testing.T) {
			t.Parallel()

			fixture := setupTmuxFixture(t, tc.settingID)
			writeFile(t, filepath.Join(fixture.liveRoot, tc.relPath), "raw-live-tmux-"+tc.settingID+"\n")
			writeFileResourceDesired(t, fixture, recipe.TmuxTarget, tc.settingID, "raw-desired-tmux-"+tc.settingID+"\n")
			roots := map[string]map[string]string{recipe.TmuxTarget: {tc.locationID: fixture.liveRoot}}

			for _, command := range []string{CommandStatus, CommandDiff, CommandSave, CommandApply} {
				t.Run(command, func(t *testing.T) {
					report, err := Build(Options{Command: command, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "tmux:" + tc.settingID, UserID: "leon", DryRun: command == CommandSave || command == CommandApply, LocationRoots: roots})
					require.NoError(t, err)
					require.Len(t, report.Items, 1)
					item := report.Items[0]
					require.Equal(t, recipe.RecipeSourceBundled, item.Recipe.Source)
					require.Equal(t, "recipe://bundled/tmux", item.Recipe.RecipeRef)
					require.Equal(t, recipe.TrustStatusTrusted, item.Recipe.TrustStatus)
					require.Equal(t, "tmux:"+tc.settingID, item.SettingRef)
					require.Equal(t, recipe.FileDriverID, item.Resource.DriverID)
					require.Equal(t, tc.settingID, item.Resource.ID)
					require.Equal(t, tc.locationID, item.Resource.LocationID)
					require.Equal(t, filepath.ToSlash(tc.relPath), filepath.ToSlash(item.Resource.RelPath))
					require.Equal(t, SelectorInfo{Kind: "file", Summary: filepath.ToSlash(tc.relPath)}, item.Selector)
					require.Equal(t, "desired://user/leon/targets/tmux/artifacts/"+tc.settingID, item.DesiredURI)
					require.Equal(t, filepath.ToSlash(filepath.Join("desired", "user", "leon", "targets", "tmux", "artifacts", tc.settingID)), item.DesiredRelPath)
					require.True(t, item.Current.Exists)
					require.Equal(t, "present", item.Desired.Status)
					require.Equal(t, "file", item.Desired.Kind)
					require.True(t, item.Desired.Snapshot.Exists)
					require.NotNil(t, item.Preview)
					if command == CommandDiff {
						require.NotNil(t, item.Diff)
						require.Equal(t, "metadata-only", item.Diff.Mode)
						require.Equal(t, "raw file contents omitted", item.Diff.Redaction)
					}
					if command == CommandSave || command == CommandApply {
						requireDiagnostic(t, item, recipe.TmuxManualReloadWarningCode)
						require.NotEqual(t, SummaryBlocked, report.Summary.Status)
					} else {
						requireNoDiagnostic(t, item, recipe.TmuxManualReloadWarningCode)
					}
					payload := mustJSON(t, report)
					require.NotContains(t, payload, "raw-live-tmux-"+tc.settingID)
					require.NotContains(t, payload, "raw-desired-tmux-"+tc.settingID)
					require.NotContains(t, Text(report), "raw-live-tmux-"+tc.settingID)
					require.NotContains(t, Text(report), "raw-desired-tmux-"+tc.settingID)
				})
			}
		})
	}
}

func TestBuildTmuxMissingFileSemanticsAreFailClosed(t *testing.T) {
	t.Parallel()

	t.Run("missing live blocks save without deleting desired", func(t *testing.T) {
		t.Parallel()
		fixture := setupTmuxFixture(t, "home.conf")
		desiredPath := writeFileResourceDesired(t, fixture, recipe.TmuxTarget, "home.conf", "raw-desired-tmux-home\n")
		roots := map[string]map[string]string{recipe.TmuxTarget: {"home": fixture.liveRoot}}

		report, err := Build(Options{Command: CommandSave, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "tmux:home.conf", UserID: "leon", DryRun: true, LocationRoots: roots})
		require.NoError(t, err)
		require.Equal(t, SummaryBlocked, report.Summary.Status)
		require.Len(t, report.Items, 1)
		item := report.Items[0]
		require.Equal(t, v2status.StateBlockedSafety, item.State)
		require.False(t, item.Current.Exists)
		require.Equal(t, "present", item.Desired.Status)
		requireDiagnosticMessageContains(t, item, "live file is missing")
		require.Equal(t, "raw-desired-tmux-home\n", readFile(t, desiredPath))
		require.NotContains(t, mustJSON(t, report), "raw-desired-tmux-home")
	})

	t.Run("missing live blocks apply instead of creating config file", func(t *testing.T) {
		t.Parallel()
		fixture := setupTmuxFixture(t, "xdg.conf")
		desiredPath := writeFileResourceDesired(t, fixture, recipe.TmuxTarget, "xdg.conf", "raw-desired-tmux-xdg\n")
		roots := map[string]map[string]string{recipe.TmuxTarget: {"config": fixture.liveRoot}}

		report, err := Build(Options{Command: CommandApply, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "tmux:xdg.conf", UserID: "leon", DryRun: true, LocationRoots: roots})
		require.NoError(t, err)
		require.Equal(t, SummaryBlocked, report.Summary.Status)
		require.Len(t, report.Items, 1)
		item := report.Items[0]
		require.Equal(t, v2status.StateBlockedSafety, item.State)
		require.False(t, item.Current.Exists)
		require.Equal(t, "present", item.Desired.Status)
		requireDiagnosticMessageContains(t, item, "live file is missing")
		require.Equal(t, "raw-desired-tmux-xdg\n", readFile(t, desiredPath))
		require.NoFileExists(t, filepath.Join(fixture.liveRoot, "tmux", "tmux.conf"))
		require.NoDirExists(t, filepath.Join(fixture.liveRoot, "tmux"))
		require.NotContains(t, mustJSON(t, report), "raw-desired-tmux-xdg")
	})

	t.Run("missing desired blocks apply without deleting live", func(t *testing.T) {
		t.Parallel()
		fixture := setupTmuxFixture(t, "home.conf")
		livePath := filepath.Join(fixture.liveRoot, ".tmux.conf")
		writeFile(t, livePath, "raw-live-tmux-home\n")
		roots := map[string]map[string]string{recipe.TmuxTarget: {"home": fixture.liveRoot}}

		report, err := Build(Options{Command: CommandApply, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "tmux:home.conf", UserID: "leon", DryRun: true, LocationRoots: roots})
		require.NoError(t, err)
		require.Equal(t, SummaryBlocked, report.Summary.Status)
		require.Len(t, report.Items, 1)
		item := report.Items[0]
		require.Equal(t, v2status.StateBlockedSafety, item.State)
		require.True(t, item.Current.Exists)
		require.Equal(t, "missing", item.Desired.Status)
		requireDiagnosticMessageContains(t, item, "desired artifact is missing")
		require.Equal(t, "raw-live-tmux-home\n", readFile(t, livePath))
		require.NotContains(t, mustJSON(t, report), "raw-live-tmux-home")
	})
}

func TestBuildUsesBundledSSHConfigOnlyRuntime(t *testing.T) {
	t.Parallel()

	fixture := setupSSHFixture(t, "config")
	configPath := filepath.Join(fixture.liveRoot, ".ssh", "config")
	keyPath := filepath.Join(fixture.liveRoot, ".ssh", "id_ed25519")
	includePath := filepath.Join(fixture.liveRoot, ".ssh", "config.d", "work.conf")
	writeFile(t, configPath, "Host github.com\n  HostName github.com\n  IdentityFile ~/.ssh/id_ed25519\n  Include ~/.ssh/config.d/*.conf\n")
	writeFile(t, keyPath, "-----BEGIN OPENSSH PRIVATE KEY-----\nnot-read\n-----END OPENSSH PRIVATE KEY-----\n")
	writeFile(t, includePath, "Host included\n  User should-not-be-read\n")
	writeFileResourceDesired(t, fixture, recipe.SSHTarget, "config", "Host gitlab.com\n  HostName gitlab.com\n")
	roots := map[string]map[string]string{recipe.SSHTarget: {"home": fixture.liveRoot}}

	for _, command := range []string{CommandStatus, CommandDiff, CommandSave, CommandApply} {
		t.Run(command, func(t *testing.T) {
			report, err := Build(Options{Command: command, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "ssh:config", UserID: "leon", DryRun: command == CommandSave || command == CommandApply, LocationRoots: roots})
			require.NoError(t, err)
			require.Len(t, report.Items, 1)
			item := report.Items[0]
			require.Equal(t, recipe.RecipeSourceBundled, item.Recipe.Source)
			require.Equal(t, "recipe://bundled/ssh", item.Recipe.RecipeRef)
			require.Equal(t, recipe.TrustStatusTrusted, item.Recipe.TrustStatus)
			require.Equal(t, "ssh:config", item.SettingRef)
			require.Equal(t, recipe.FileDriverID, item.Resource.DriverID)
			require.Equal(t, "config", item.Resource.ID)
			require.Equal(t, "home", item.Resource.LocationID)
			require.Equal(t, ".ssh/config", item.Resource.RelPath)
			require.Equal(t, SelectorInfo{Kind: "file", Summary: ".ssh/config"}, item.Selector)
			require.Equal(t, "desired://user/leon/targets/ssh/artifacts/config", item.DesiredURI)
			require.Equal(t, filepath.ToSlash(filepath.Join("desired", "user", "leon", "targets", "ssh", "artifacts", "config")), item.DesiredRelPath)
			require.True(t, item.Current.Exists)
			require.Equal(t, "present", item.Desired.Status)
			require.Equal(t, "file", item.Desired.Kind)
			require.True(t, item.Desired.Snapshot.Exists)
			if command == CommandDiff {
				require.NotNil(t, item.Diff)
				require.Equal(t, "metadata-only", item.Diff.Mode)
			}
			if command == CommandSave || command == CommandApply {
				requireDiagnostic(t, item, recipe.SSHConfigReviewWarningCode)
				require.NotEqual(t, SummaryBlocked, report.Summary.Status)
			} else {
				requireNoDiagnostic(t, item, recipe.SSHConfigReviewWarningCode)
			}
			payload := mustJSON(t, report)
			require.NotContains(t, payload, "github.com")
			require.NotContains(t, payload, "gitlab.com")
			require.NotContains(t, payload, "not-read")
			require.NotContains(t, payload, "should-not-be-read")
			require.NotContains(t, Text(report), "github.com")
			require.NotContains(t, Text(report), "gitlab.com")
			require.NotContains(t, Text(report), "not-read")
			require.NotContains(t, Text(report), "should-not-be-read")
		})
	}
}

func TestBuildSSHBlocksExcludedRefsWithoutFileReads(t *testing.T) {
	t.Parallel()

	fixture := setupSSHFixture(t, "keys")
	writeFile(t, filepath.Join(fixture.liveRoot, ".ssh", "id_ed25519"), "raw-private-key-material\n")
	roots := map[string]map[string]string{recipe.SSHTarget: {"home": fixture.liveRoot}}

	report, err := Build(Options{Command: CommandStatus, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "ssh:keys", UserID: "leon", LocationRoots: roots})
	require.NoError(t, err)
	require.Equal(t, SummaryBlocked, report.Summary.Status)
	require.Len(t, report.Items, 1)
	item := report.Items[0]
	require.Equal(t, v2status.StateUnsupported, item.State)
	requireDiagnostic(t, item, recipe.SSHRefExcludedCode)
	require.Contains(t, item.Message, "excluded")
	require.Empty(t, item.Current.SHA256)
	require.NotContains(t, mustJSON(t, report), "raw-private-key-material")
}

func TestBuildSSHContentSafetyBlocksSaveApplyAndSymlink(t *testing.T) {
	t.Parallel()

	t.Run("save blocks live private key header", func(t *testing.T) {
		t.Parallel()

		fixture := setupSSHFixture(t, "config")
		writeFile(t, filepath.Join(fixture.liveRoot, ".ssh", "config"), "Host bad\n-----BEGIN OPENSSH PRIVATE KEY-----\nabc\n-----END OPENSSH PRIVATE KEY-----\n")
		roots := map[string]map[string]string{recipe.SSHTarget: {"home": fixture.liveRoot}}

		report, err := Build(Options{Command: CommandSave, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "ssh:config", UserID: "leon", DryRun: true, LocationRoots: roots})
		require.NoError(t, err)
		require.Equal(t, SummaryBlocked, report.Summary.Status)
		item := report.Items[0]
		require.Equal(t, v2status.StateBlockedSafety, item.State)
		requireDiagnostic(t, item, recipe.SSHConfigExcludedContentCode)
		requireDiagnosticMessageContains(t, item, "private-key")
		payload := mustJSON(t, report)
		require.NotContains(t, payload, "BEGIN OPENSSH PRIVATE KEY")
		require.NotContains(t, Text(report), "BEGIN OPENSSH PRIVATE KEY")
	})

	t.Run("apply blocks desired public key and preserves live", func(t *testing.T) {
		t.Parallel()

		fixture := setupSSHFixture(t, "config")
		livePath := filepath.Join(fixture.liveRoot, ".ssh", "config")
		writeFile(t, livePath, "Host safe\n  HostName example.com\n")
		desiredPath := writeFileResourceDesired(t, fixture, recipe.SSHTarget, "config", "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMM user@example\n")
		roots := map[string]map[string]string{recipe.SSHTarget: {"home": fixture.liveRoot}}

		report, err := Build(Options{Command: CommandApply, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "ssh:config", UserID: "leon", DryRun: true, LocationRoots: roots})
		require.NoError(t, err)
		require.Equal(t, SummaryBlocked, report.Summary.Status)
		item := report.Items[0]
		requireDiagnostic(t, item, recipe.SSHConfigExcludedContentCode)
		requireDiagnosticMessageContains(t, item, "ssh-public-key")
		require.Equal(t, "Host safe\n  HostName example.com\n", readFile(t, livePath))
		require.Contains(t, readFile(t, desiredPath), "ssh-ed25519")
		require.NotContains(t, mustJSON(t, report), "AAAAC3NzaC1lZDI1NTE5")
	})

	t.Run("apply blocks live backup with known hosts content", func(t *testing.T) {
		t.Parallel()

		fixture := setupSSHFixture(t, "config")
		writeFile(t, filepath.Join(fixture.liveRoot, ".ssh", "config"), "|1|abcdefghijklmnop=|qrstuvwxyzabcdef= ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMM\n")
		writeFileResourceDesired(t, fixture, recipe.SSHTarget, "config", "Host safe\n  HostName example.com\n")
		roots := map[string]map[string]string{recipe.SSHTarget: {"home": fixture.liveRoot}}

		report, err := Build(Options{Command: CommandApply, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "ssh:config", UserID: "leon", DryRun: true, LocationRoots: roots})
		require.NoError(t, err)
		require.Equal(t, SummaryBlocked, report.Summary.Status)
		item := report.Items[0]
		requireDiagnostic(t, item, recipe.SSHConfigExcludedContentCode)
		requireDiagnosticMessageContains(t, item, "ssh-known-hosts")
		require.NotContains(t, mustJSON(t, report), "abcdefghijklmnop")
	})

	t.Run("status blocks symlinked config before reading target", func(t *testing.T) {
		t.Parallel()

		fixture := setupSSHFixture(t, "config")
		keyPath := filepath.Join(fixture.liveRoot, ".ssh", "id_ed25519")
		configPath := filepath.Join(fixture.liveRoot, ".ssh", "config")
		writeFile(t, keyPath, "raw-symlink-private-key\n")
		require.NoError(t, os.Symlink(keyPath, configPath))
		writeFileResourceDesired(t, fixture, recipe.SSHTarget, "config", "Host safe\n")
		roots := map[string]map[string]string{recipe.SSHTarget: {"home": fixture.liveRoot}}

		report, err := Build(Options{Command: CommandStatus, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "ssh:config", UserID: "leon", LocationRoots: roots})
		require.NoError(t, err)
		require.Equal(t, SummaryBlocked, report.Summary.Status)
		item := report.Items[0]
		requireDiagnostic(t, item, recipe.SSHConfigSymlinkUnsupportedCode)
		require.Empty(t, item.Current.SHA256)
		require.NotContains(t, mustJSON(t, report), "raw-symlink-private-key")
	})
}

func TestBuildRejectsRefsAndMissingMatches(t *testing.T) {
	t.Parallel()

	fixture := setupFixture(t)
	_, err := Build(Options{Command: CommandStatus, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "desired://user/leon/targets/test.app/settings#identity.email", UserID: "leon"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported selected-value ref kind")

	_, err = Build(Options{Command: CommandStatus, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "test.app:missing", UserID: "leon"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "no selected settings match")
}

func requireDiagnostic(t *testing.T, item Item, code string) {
	t.Helper()
	for _, diagnostic := range item.Diagnostics {
		if diagnostic.Code == code {
			return
		}
	}
	require.Failf(t, "missing diagnostic", "wanted %s in %+v", code, item.Diagnostics)
}

func requireNoDiagnostic(t *testing.T, item Item, code string) {
	t.Helper()
	for _, diagnostic := range item.Diagnostics {
		require.NotEqual(t, code, diagnostic.Code, "unexpected diagnostic in %+v", item.Diagnostics)
	}
}

func requireDiagnosticMessageContains(t *testing.T, item Item, text string) {
	t.Helper()
	for _, diagnostic := range item.Diagnostics {
		if strings.Contains(diagnostic.Message, text) {
			return
		}
	}
	require.Failf(t, "missing diagnostic message", "wanted message containing %q in %+v", text, item.Diagnostics)
}

func mustJSON(t *testing.T, report *Report) string {
	t.Helper()
	payload, err := JSON(report)
	require.NoError(t, err)
	return payload
}

type fixture struct {
	repoRoot  string
	liveRoot  string
	stateRoot string
	recipe    *recipe.Recipe
	t         *testing.T
}

func setupFixture(t *testing.T) fixture {
	return setupFixtureForTarget(t, "test.app", "identity.email")
}

func setupFixtureForTarget(t *testing.T, target string, settingID string) fixture {
	t.Helper()
	repoRoot := t.TempDir()
	liveRoot := t.TempDir()
	stateRoot := t.TempDir()
	writeV2Root(t, repoRoot, target, settingID)
	var rec *recipe.Recipe
	if target != recipe.GitTarget && target != recipe.CustomFilesTarget {
		body := selectedRecipeBody(target, liveRoot)
		writeFile(t, repoRoot+"/recipes/local/"+target+"/recipe.yaml", body)
		rec = decodeRecipe(t, body)
	}
	return fixture{repoRoot: repoRoot, liveRoot: liveRoot, stateRoot: stateRoot, recipe: rec, t: t}
}

func setupZshFixture(t *testing.T, settingID string) fixture {
	t.Helper()
	repoRoot := t.TempDir()
	homeRoot := t.TempDir()
	stateRoot := t.TempDir()
	writeV2Root(t, repoRoot, recipe.ZshTarget, settingID)
	return fixture{repoRoot: repoRoot, liveRoot: homeRoot, stateRoot: stateRoot, t: t}
}

func setupTmuxFixture(t *testing.T, settingID string) fixture {
	t.Helper()
	repoRoot := t.TempDir()
	locationRoot := t.TempDir()
	stateRoot := t.TempDir()
	writeV2Root(t, repoRoot, recipe.TmuxTarget, settingID)
	return fixture{repoRoot: repoRoot, liveRoot: locationRoot, stateRoot: stateRoot, t: t}
}

func setupSSHFixture(t *testing.T, settingID string) fixture {
	t.Helper()
	repoRoot := t.TempDir()
	homeRoot := t.TempDir()
	stateRoot := t.TempDir()
	writeV2Root(t, repoRoot, recipe.SSHTarget, settingID)
	return fixture{repoRoot: repoRoot, liveRoot: homeRoot, stateRoot: stateRoot, t: t}
}

func (f fixture) writeLiveYAML(email string) {
	writeFile(f.t, f.liveRoot+"/config.yaml", "user:\n  email: "+email+"\n")
}

func (f fixture) writeDesiredSet(email string) {
	f.writeDesiredSetFor("test.app", "identity.email", email)
}

func (f fixture) writeDesiredSetFor(target string, setting string, value string) {
	writeFile(f.t, f.repoRoot+"/desired/user/leon/targets/"+target+"/settings.yaml", "schema: dotfiles-manager.v2.desired-settings\nschemaVersion: 1\nvalues:\n  "+setting+":\n    intent: set\n    kind: string\n    value: "+value+"\n")
}

func (f fixture) writeDesiredUnmanaged() {
	writeFile(f.t, f.repoRoot+"/desired/user/leon/targets/test.app/settings.yaml", "schema: dotfiles-manager.v2.desired-settings\nschemaVersion: 1\nvalues:\n  identity.email:\n    intent: unmanaged\n")
}

func (f fixture) trustRecipe() {
	_, err := recipe.RecordLocalRecipeTrust(f.repoRoot, f.stateRoot, f.recipe)
	require.NoError(f.t, err)
}

func setupNativeExportFixture(t *testing.T, reviewRequired bool) fixture {
	t.Helper()
	repoRoot := t.TempDir()
	stateRoot := t.TempDir()
	writeNativeExportRoot(t, repoRoot)
	body := nativeExportRecipeBody(reviewRequired)
	writeFile(t, filepath.Join(repoRoot, "recipes", "local", "native.app", "recipe.yaml"), body)
	rec := decodeRecipe(t, body)
	return fixture{repoRoot: repoRoot, stateRoot: stateRoot, recipe: rec, t: t}
}

func setupNativeApplyFixture(t *testing.T) fixture {
	t.Helper()
	repoRoot := t.TempDir()
	stateRoot := t.TempDir()
	writeNativeExportRoot(t, repoRoot)
	body := nativeApplyRecipeBody()
	writeFile(t, filepath.Join(repoRoot, "recipes", "local", "native.app", "recipe.yaml"), body)
	rec := decodeRecipe(t, body)
	return fixture{repoRoot: repoRoot, stateRoot: stateRoot, recipe: rec, t: t}
}

func (f fixture) trustNativeRecipe() {
	_, err := recipe.RecordLocalRecipeTrust(f.repoRoot, f.stateRoot, f.recipe)
	require.NoError(f.t, err)
	path := filepath.Join(f.stateRoot, "trust", "trust-record.yaml")
	payload := readFile(f.t, path)
	payload = strings.Replace(payload, "reviewedNativeOperations: false", "reviewedNativeOperations: true", 1)
	writeFile(f.t, path, payload)
}

func (f fixture) desiredArtifactPath() string {
	return filepath.Join(f.repoRoot, "desired", "user", "leon", "targets", "native.app", "artifacts", "settings")
}

func (f fixture) writeNativeDesiredArtifact(body string) {
	f.t.Helper()
	payloadRoot := filepath.Join(f.desiredArtifactPath(), nativeexport.PayloadDir)
	require.NoError(f.t, os.MkdirAll(payloadRoot, 0o755))
	require.NoError(f.t, os.WriteFile(filepath.Join(payloadRoot, "bundle.txt"), []byte(body), 0o644))
	summary, err := nativeexport.SummarizePayload(payloadRoot, nativeexport.EffectiveLimits(f.recipe.NativeOperations["export-settings"]))
	require.NoError(f.t, err)
	require.NoError(f.t, nativeexport.WriteMetadata(f.desiredArtifactPath(), nativeexport.Metadata{
		Schema:        nativeexport.MetadataSchema,
		SchemaVersion: nativeexport.SchemaVersion,
		TargetRef:     "native.app",
		SettingRef:    "native.app:settings",
		ResourceID:    "settings",
		OperationID:   "export-settings",
		Recipe:        nativeexport.RecipeMetadata{Source: recipe.RecipeSourceLocal, TrustStatus: string(recipe.TrustStatusTrusted)},
		Operation:     nativeexport.OperationMetadata{ArtifactForm: "native-export", DiffMode: "metadata-only", Redaction: "metadata-only", OutputIDs: []string{"bundle"}},
		Source:        nativeexport.SourceMetadata{Scope: "user", Subject: "leon", MachineID: "mbp", UserID: "leon"},
		CapturedAt:    "2026-06-09T12:00:00Z",
		Payload:       summary,
		Native:        nativeexport.NativeRunMetadata{Status: nativeexport.StatusSucceeded},
	}))
}

func writeNativeExportRoot(t *testing.T, root string) {
	t.Helper()
	writeFile(t, filepath.Join(root, "dotfiles-manager.v2.yaml"), "schema: dotfiles-manager.v2.root-config\nschemaVersion: 1\nactiveProfileStack: default\n")
	writeFile(t, filepath.Join(root, "profiles", "stacks", "default.yaml"), "schema: dotfiles-manager.v2.profile-stack\nschemaVersion: 1\nprofileStack: [global]\n")
	writeFile(t, filepath.Join(root, "profiles", "layers", "global.yaml"), "schema: dotfiles-manager.v2.profile-layer\nschemaVersion: 1\nselections:\n  native.app:\n    settings:\n      settings:\n        scope: user\n        artifact: artifacts/settings\n")
}

func nativeExportRecipeBody(reviewRequired bool) string {
	review := ""
	if reviewRequired {
		review = `
    review:
      required: true
      reasons: [opaque, account-bound]
      message: Review native export before running`
	}
	return `schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: native.app
displayName: Native App
supportLevel: experimental
capability: export-only
settings:
  settings:
    label: Settings bundle
    supportLevel: experimental
    capability: export-only
    artifactForm: native-export
    sensitivity: personal
    redaction: redacted-for-display
    lifecycle: allowed
    scopeDefault: user
    resource: settings
resources:
  settings:
    driver: native-export
    nativeOperation: export-settings
    capability: export-only
    sensitivity: personal
    redaction: redacted-for-display
    lifecycle: allowed
nativeOperations:
  export-settings:
    kind: export
    reviewed: true
    runner: command
    platforms: [darwin, linux]
    artifactForm: native-export
    diffMode: metadata-only
    lifecycle: allowed
    workingDirectory: temp
    timeoutSeconds: 5
    expectedExitCodes: [0]
    command:
      executable: /usr/bin/native-safe-tool
      args:
        - literal: export
        - output: bundle
    stdin:
      mode: none
    stdout:
      mode: discard
    stderr:
      mode: discard
    outputs:
      bundle:
        root: artifact
        path: bundle.txt
    redaction: metadata-only
    limits:
      maxBytes: 1024
      maxEntries: 10
    exportMetadata:
      capturedCategories: [settings]
      secretExclusions: [tokens]
      accountExclusions: [sessions]
      limitations:
        - Internal app settings are not semantically compared` + review + `
`
}

func nativeApplyRecipeBody() string {
	return `schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: native.app
displayName: Native App
supportLevel: experimental
capability: read-write
settings:
  settings:
    label: Settings bundle
    supportLevel: experimental
    capability: read-write
    artifactForm: native-export
    sensitivity: personal
    redaction: redacted-for-display
    lifecycle: allowed
    scopeDefault: user
    resource: settings
resources:
  settings:
    driver: native-export
    nativeOperation: export-settings
    nativeImportOperation: import-settings
    nativeApply:
      backup: pre-apply-export
      verify: post-import-export-hash
    capability: read-write
    sensitivity: personal
    redaction: redacted-for-display
    lifecycle: allowed
nativeOperations:
  export-settings:
    kind: export
    reviewed: true
    runner: command
    platforms: [darwin, linux]
    artifactForm: native-export
    diffMode: metadata-only
    lifecycle: allowed
    workingDirectory: temp
    timeoutSeconds: 5
    expectedExitCodes: [0]
    command:
      executable: /usr/bin/native-safe-tool
      args:
        - literal: export
        - output: bundle
    stdin:
      mode: none
    stdout:
      mode: discard
    stderr:
      mode: discard
    outputs:
      bundle:
        root: artifact
        path: bundle.txt
    redaction: metadata-only
    limits:
      maxBytes: 1024
      maxEntries: 10
    exportMetadata:
      capturedCategories: [settings]
      limitations:
        - Internal app settings are not semantically compared
  import-settings:
    kind: import
    reviewed: true
    runner: command
    platforms: [darwin, linux]
    artifactForm: native-export
    diffMode: metadata-only
    lifecycle: allowed
    workingDirectory: temp
    timeoutSeconds: 5
    expectedExitCodes: [0]
    command:
      executable: /usr/bin/native-safe-tool
      args:
        - literal: import
        - input: bundle
    stdin:
      mode: none
    stdout:
      mode: discard
    stderr:
      mode: discard
    inputs:
      bundle:
        root: artifact
        path: bundle.txt
    redaction: metadata-only
    limits:
      maxBytes: 1024
      maxEntries: 10
`
}

var fixedPreviewTime = func() time.Time {
	return time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
}

type recordingNativeExecutor struct {
	body  string
	calls int
}

func (e *recordingNativeExecutor) Run(ctx context.Context, spec nativeops.ExecSpec) nativeops.ExecResult {
	e.calls++
	if len(spec.Args) < 2 {
		return nativeops.ExecResult{ExitCode: 2, Err: errors.New("missing output arg")}
	}
	if err := os.WriteFile(spec.Args[1], []byte(e.body), 0o644); err != nil {
		return nativeops.ExecResult{ExitCode: 1, Err: err}
	}
	return nativeops.ExecResult{ExitCode: 0, Stdout: nativeops.CaptureSummary{Mode: spec.Stdout.Mode}, Stderr: nativeops.CaptureSummary{Mode: spec.Stderr.Mode}}
}

func writeV2Root(t *testing.T, root string, target string, settingID string) {
	t.Helper()
	writeFile(t, root+"/dotfiles-manager.v2.yaml", "schema: dotfiles-manager.v2.root-config\nschemaVersion: 1\nactiveProfileStack: default\n")
	writeFile(t, root+"/profiles/stacks/default.yaml", "schema: dotfiles-manager.v2.profile-stack\nschemaVersion: 1\nprofileStack: [global]\n")
	writeFile(t, root+"/profiles/layers/global.yaml", "schema: dotfiles-manager.v2.profile-layer\nschemaVersion: 1\nselections:\n  "+target+":\n    settings:\n      "+settingID+":\n        scope: user\n")
}

func selectedRecipeBody(target string, liveRoot string) string {
	return `schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: ` + target + `
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

func decodeRecipe(t *testing.T, body string) *recipe.Recipe {
	t.Helper()
	rec, err := recipe.Decode("recipe.yaml", strings.NewReader(body))
	require.NoError(t, err)
	return rec
}

func writeFile(t *testing.T, path string, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

func TestBuildMissingDesiredCoversStatusDiffSaveApply(t *testing.T) {
	t.Parallel()

	fixture := setupFixture(t)
	fixture.writeLiveYAML("current@example.com")
	fixture.trustRecipe()

	report, err := Build(Options{Command: CommandStatus, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "test.app:identity.email", UserID: "leon"})
	require.NoError(t, err)
	require.Equal(t, SummaryChanged, report.Summary.Status)
	require.Len(t, report.Items, 1)
	statusItem := report.Items[0]
	require.Equal(t, desired.StatusMissing, statusItem.Desired.Status)
	require.True(t, statusItem.Current.Exists)
	require.Equal(t, v2status.StateMissingDesired, statusItem.State)
	require.Contains(t, statusItem.AllowedActions, v2status.ActionSave)

	report, err = Build(Options{Command: CommandDiff, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "test.app:identity.email", UserID: "leon"})
	require.NoError(t, err)
	require.NotNil(t, report.Items[0].Diff)
	require.Equal(t, "metadata-only", report.Items[0].Diff.Mode)

	report, err = Build(Options{Command: CommandSave, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "test.app:identity.email", UserID: "leon", DryRun: true})
	require.NoError(t, err)
	saveItem := report.Items[0]
	require.True(t, report.DryRun)
	require.Equal(t, PlannedActionWouldPromote, saveItem.PlannedAction)
	require.Equal(t, 1, report.Summary.Saved)
	require.Contains(t, saveItem.Message, "promoted into desired state")
	require.NotNil(t, saveItem.Preview)
	require.Equal(t, "create", saveItem.Preview.ChangeKind)
	require.Equal(t, desired.IntentSet, saveItem.Preview.Intent)
	require.False(t, saveItem.Mutated)

	report, err = Build(Options{Command: CommandApply, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "test.app:identity.email", UserID: "leon", DryRun: true})
	require.NoError(t, err)
	applyItem := report.Items[0]
	require.Equal(t, v2status.StateMissingDesired, applyItem.State)
	require.Equal(t, PlannedActionBlockedMissingDesired, applyItem.PlannedAction)
	require.Contains(t, applyItem.Message, "no desired artifact")
}

func TestBuildMissingDesiredWithoutLiveValueDoesNotUsePromotionAction(t *testing.T) {
	t.Parallel()

	fixture := setupFixture(t)
	fixture.trustRecipe()

	report, err := Build(Options{Command: CommandSave, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "test.app:identity.email", UserID: "leon", DryRun: true})
	require.NoError(t, err)
	require.Equal(t, SummaryChanged, report.Summary.Status)
	require.Equal(t, 1, report.Summary.Saved)
	require.Len(t, report.Items, 1)
	item := report.Items[0]
	require.False(t, item.Current.Exists)
	require.Equal(t, PlannedActionWouldSave, item.PlannedAction)
	require.NotContains(t, item.Message, "promoted into desired state")
	require.NotNil(t, item.Preview)
	require.Equal(t, "create", item.Preview.ChangeKind)
	require.Equal(t, desired.IntentDelete, item.Preview.Intent)
}

func TestBuildExistingDesiredDoesNotUsePromotionAction(t *testing.T) {
	t.Parallel()

	fixture := setupFixture(t)
	fixture.writeLiveYAML("current@example.com")
	fixture.writeDesiredSet("desired@example.com")
	fixture.trustRecipe()

	report, err := Build(Options{Command: CommandSave, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "test.app:identity.email", UserID: "leon", DryRun: true})
	require.NoError(t, err)
	require.Equal(t, SummaryChanged, report.Summary.Status)
	require.Equal(t, 1, report.Summary.Saved)
	require.Len(t, report.Items, 1)
	item := report.Items[0]
	require.True(t, item.Current.Exists)
	require.Equal(t, desired.StatusPresent, item.Desired.Status)
	require.Equal(t, PlannedActionWouldSave, item.PlannedAction)
	require.NotContains(t, item.Message, "promoted into desired state")
	require.NotContains(t, mustJSON(t, report), "current@example.com")
	require.NotContains(t, mustJSON(t, report), "desired@example.com")
}

func TestBuildDeleteIntentUsesDeleteSentinelAndOmitsRawValues(t *testing.T) {
	t.Parallel()

	fixture := setupFixture(t)
	fixture.writeLiveYAML("delete-me@example.com")
	fixture.writeDesiredDelete()
	fixture.trustRecipe()

	report, err := Build(Options{Command: CommandApply, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "test.app:identity.email", UserID: "leon", DryRun: true})
	require.NoError(t, err)
	require.Equal(t, SummaryChanged, report.Summary.Status)
	require.Len(t, report.Items, 1)
	item := report.Items[0]
	require.Equal(t, desired.IntentDelete, item.Desired.Intent)
	require.False(t, item.Desired.Snapshot.Exists)
	require.True(t, item.Current.Exists)
	require.NotNil(t, item.Preview)
	require.Equal(t, "delete", item.Preview.ChangeKind)
	require.Equal(t, PlannedActionWouldApply, item.PlannedAction)
	require.NotContains(t, mustJSON(t, report), "delete-me@example.com")
}

func TestBuildBlocksWriteSafetyBeforeLiveRead(t *testing.T) {
	t.Parallel()

	fixture := setupFixtureWithRecipe(t, "test.app", "identity.email", selectedRecipeBodyWithRedaction("test.app", fixtureLiveRootPlaceholder, recipe.RedactionBlockedSave))
	fixture.writeLiveYAML("secret-live@example.com")
	fixture.writeDesiredSet("desired@example.com")
	fixture.trustRecipe()

	report, err := Build(Options{Command: CommandStatus, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "test.app:identity.email", UserID: "leon"})
	require.NoError(t, err)
	require.Equal(t, SummaryBlocked, report.Summary.Status)
	item := report.Items[0]
	require.Equal(t, v2status.StateBlockedSafety, item.State)
	require.Empty(t, item.Current.SHA256)
	requireDiagnostic(t, item, "writeSafety.redaction.blockedSave")
	require.NotContains(t, mustJSON(t, report), "secret-live@example.com")
}

func TestBuildSaveApplyBlockSecretRuntimeValuesBeforeLiveWrites(t *testing.T) {
	t.Parallel()

	t.Run("save current secret", func(t *testing.T) {
		t.Parallel()

		fixture := setupFixture(t)
		secret := "sk-proj-abcdefghijklmnopqrstuvwxyz1234567890"
		fixture.writeLiveYAML(secret)
		fixture.trustRecipe()

		report, err := Build(Options{Command: CommandSave, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "test.app:identity.email", UserID: "leon", DryRun: true})
		require.NoError(t, err)
		require.Equal(t, SummaryBlocked, report.Summary.Status)
		require.Equal(t, v2status.StateBlockedSafety, report.Items[0].State)
		requireDiagnostic(t, report.Items[0], "desired.writeSafety.secretDetected")
		require.NotContains(t, mustJSON(t, report), secret)
	})

	t.Run("apply desired secret", func(t *testing.T) {
		t.Parallel()

		fixture := setupFixture(t)
		secret := "sk-proj-abcdefghijklmnopqrstuvwxyz1234567890"
		fixture.writeLiveYAML("old@example.com")
		fixture.writeDesiredSet(secret)
		fixture.trustRecipe()

		report, err := Build(Options{Command: CommandApply, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "test.app:identity.email", UserID: "leon", DryRun: true})
		require.NoError(t, err)
		require.Equal(t, SummaryBlocked, report.Summary.Status)
		require.Equal(t, v2status.StateBlockedSafety, report.Items[0].State)
		requireDiagnostic(t, report.Items[0], "desired.writeSafety.secretDetected")
		require.NotContains(t, mustJSON(t, report), secret)
	})
}

func TestBuildReportsUnsupportedAndInvalidRecipeShapes(t *testing.T) {
	t.Parallel()

	fileTreeBody := strings.Replace(fileRecipeBody("file.app", fixtureLiveRootPlaceholder), "driver: file", "driver: file-tree", 1)
	fileFixture := setupFixtureWithRecipe(t, "file.app", "identity.email", fileTreeBody)
	report, err := Build(Options{Command: CommandStatus, RepoRoot: fileFixture.repoRoot, StateRoot: fileFixture.stateRoot, Ref: "file.app:identity.email", UserID: "leon"})
	require.NoError(t, err)
	require.Equal(t, SummaryBlocked, report.Summary.Status)
	require.Equal(t, v2status.StateBlockedSafety, report.Items[0].State)
	requireDiagnostic(t, report.Items[0], "trust.local.missingRecord")

	missingSettingFixture := setupFixtureForTarget(t, "test.app", "missing.setting")
	report, err = Build(Options{Command: CommandStatus, RepoRoot: missingSettingFixture.repoRoot, StateRoot: missingSettingFixture.stateRoot, Ref: "test.app:missing.setting", UserID: "leon"})
	require.NoError(t, err)
	require.Equal(t, v2status.StateUnsupported, report.Items[0].State)
	requireDiagnostic(t, report.Items[0], "selectedpreview.resource.unknown")

	invalidFixture := setupFixture(t)
	writeFile(t, invalidFixture.repoRoot+"/recipes/local/test.app/recipe.yaml", "schema: dotfiles-manager.v2.recipe\nschemaVersion: 1\n")
	report, err = Build(Options{Command: CommandStatus, RepoRoot: invalidFixture.repoRoot, StateRoot: invalidFixture.stateRoot, Ref: "test.app:identity.email", UserID: "leon"})
	require.NoError(t, err)
	require.Equal(t, v2status.StateUnsupported, report.Items[0].State)
	requireDiagnostic(t, report.Items[0], "selectedpreview.recipe.invalid")
}

func TestBuildFileResourceCommandsUseMetadataOnlyDesiredArtifacts(t *testing.T) {
	t.Parallel()

	fixture := setupFixtureWithRecipe(t, "file.app", "identity.email", fileRecipeBody("file.app", fixtureLiveRootPlaceholder))
	writeFile(t, filepath.Join(fixture.liveRoot, "config.txt"), "raw-live-file\n")
	writeFileResourceDesired(t, fixture, "file.app", "identity.email", "raw-desired-file\n")
	fixture.trustRecipe()

	for _, command := range []string{CommandStatus, CommandDiff, CommandSave, CommandApply} {
		t.Run(command, func(t *testing.T) {
			report, err := Build(Options{
				Command:   command,
				RepoRoot:  fixture.repoRoot,
				StateRoot: fixture.stateRoot,
				Ref:       "file.app:identity.email",
				UserID:    "leon",
				DryRun:    command == CommandSave || command == CommandApply,
			})
			require.NoError(t, err)
			require.Len(t, report.Items, 1)
			item := report.Items[0]
			require.Equal(t, recipe.FileDriverID, item.Resource.DriverID)
			require.Equal(t, "config.txt", item.Resource.RelPath)
			require.True(t, filepath.IsAbs(item.Resource.Path))
			require.Equal(t, SelectorInfo{Kind: "file", Summary: "config.txt"}, item.Selector)
			require.Equal(t, "desired://user/leon/targets/file.app/artifacts/identity.email", item.DesiredURI)
			require.Equal(t, filepath.ToSlash(filepath.Join("desired", "user", "leon", "targets", "file.app", "artifacts", "identity.email")), item.DesiredRelPath)
			require.True(t, item.Current.Exists)
			require.Equal(t, len("raw-live-file\n"), item.Current.Size)
			require.Equal(t, "present", item.Desired.Status)
			require.Equal(t, "file", item.Desired.Kind)
			require.True(t, item.Desired.Snapshot.Exists)
			require.Equal(t, len("raw-desired-file\n"), item.Desired.Snapshot.Size)
			require.NotNil(t, item.Preview)
			require.Equal(t, "update", item.Preview.ChangeKind)
			if command == CommandSave || command == CommandApply {
				require.Equal(t, desired.IntentSet, item.Preview.Intent)
				require.Contains(t, item.PlannedAction, "would-")
			}
			if command == CommandDiff {
				require.NotNil(t, item.Diff)
				require.Equal(t, "metadata-only", item.Diff.Mode)
				require.Equal(t, "raw file contents omitted", item.Diff.Redaction)
				require.Equal(t, "update", item.Diff.Kind)
			}
			payload := mustJSON(t, report)
			require.NotContains(t, payload, "raw-live-file")
			require.NotContains(t, payload, "raw-desired-file")
			require.NotContains(t, Text(report), "raw-live-file")
			require.NotContains(t, Text(report), "raw-desired-file")
		})
	}
}

func TestBuildFileResourceMissingStatesDoNotDelete(t *testing.T) {
	t.Parallel()

	t.Run("save blocks missing live and preserves desired", func(t *testing.T) {
		t.Parallel()

		fixture := setupFixtureWithRecipe(t, "file.app", "identity.email", fileRecipeBody("file.app", fixtureLiveRootPlaceholder))
		desiredPath := writeFileResourceDesired(t, fixture, "file.app", "identity.email", "keep-desired\n")
		fixture.trustRecipe()

		report, err := Build(Options{Command: CommandSave, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "file.app:identity.email", UserID: "leon", DryRun: true})
		require.NoError(t, err)
		require.Equal(t, SummaryBlocked, report.Summary.Status)
		require.Len(t, report.Items, 1)
		item := report.Items[0]
		require.Equal(t, v2status.StateBlockedSafety, item.State)
		require.Equal(t, "desired://user/leon/targets/file.app/artifacts/identity.email", item.DesiredURI)
		require.Equal(t, filepath.ToSlash(filepath.Join("desired", "user", "leon", "targets", "file.app", "artifacts", "identity.email")), item.DesiredRelPath)
		requireDiagnostic(t, item, "selectedpreview.fileResource.plan")
		require.Contains(t, item.Diagnostics[0].Message, "live file is missing")
		require.Equal(t, "keep-desired\n", readFile(t, desiredPath))
	})

	t.Run("apply blocks missing desired and preserves live", func(t *testing.T) {
		t.Parallel()

		fixture := setupFixtureWithRecipe(t, "file.app", "identity.email", fileRecipeBody("file.app", fixtureLiveRootPlaceholder))
		livePath := filepath.Join(fixture.liveRoot, "config.txt")
		writeFile(t, livePath, "keep-live\n")
		fixture.trustRecipe()

		report, err := Build(Options{Command: CommandApply, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "file.app:identity.email", UserID: "leon", DryRun: true})
		require.NoError(t, err)
		require.Equal(t, SummaryBlocked, report.Summary.Status)
		require.Len(t, report.Items, 1)
		item := report.Items[0]
		require.Equal(t, v2status.StateBlockedSafety, item.State)
		require.Equal(t, "desired://user/leon/targets/file.app/artifacts/identity.email", item.DesiredURI)
		require.Equal(t, filepath.ToSlash(filepath.Join("desired", "user", "leon", "targets", "file.app", "artifacts", "identity.email")), item.DesiredRelPath)
		requireDiagnostic(t, item, "selectedpreview.fileResource.plan")
		require.Contains(t, item.Diagnostics[0].Message, "desired artifact is missing")
		require.Equal(t, "keep-live\n", readFile(t, livePath))
	})

	t.Run("diff reports missing desired without delete intent", func(t *testing.T) {
		t.Parallel()

		fixture := setupFixtureWithRecipe(t, "file.app", "identity.email", fileRecipeBody("file.app", fixtureLiveRootPlaceholder))
		writeFile(t, filepath.Join(fixture.liveRoot, "config.txt"), "live-only\n")
		fixture.trustRecipe()

		report, err := Build(Options{Command: CommandDiff, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "file.app:identity.email", UserID: "leon"})
		require.NoError(t, err)
		require.Len(t, report.Items, 1)
		item := report.Items[0]
		require.NotNil(t, item.Diff)
		require.Equal(t, "missing-desired", item.Diff.Kind)
		require.Empty(t, item.Preview.Intent)
	})

	t.Run("save promotes existing live file into missing desired artifact", func(t *testing.T) {
		t.Parallel()

		fixture := setupFixtureWithRecipe(t, "file.app", "identity.email", fileRecipeBody("file.app", fixtureLiveRootPlaceholder))
		writeFile(t, filepath.Join(fixture.liveRoot, "config.txt"), "promote-live\n")
		fixture.trustRecipe()

		report, err := Build(Options{Command: CommandSave, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "file.app:identity.email", UserID: "leon", DryRun: true})
		require.NoError(t, err)
		require.Len(t, report.Items, 1)
		item := report.Items[0]
		require.Equal(t, PlannedActionWouldPromote, item.PlannedAction)
		require.Contains(t, item.Message, "promoted into a desired artifact")
		require.Equal(t, "missing", item.Desired.Status)
		require.Equal(t, desired.IntentSet, item.Preview.Intent)
		require.Equal(t, "create", item.Preview.ChangeKind)
	})
}

func TestBuildFileTreeResourceCommandsUseMetadataOnlyDesiredArtifacts(t *testing.T) {
	t.Parallel()

	fixture := setupFixtureWithRecipe(t, "tree.app", "config", fileTreeRecipeBody("tree.app", fixtureLiveRootPlaceholder))
	writeFile(t, filepath.Join(fixture.liveRoot, "nvim", "init.lua"), "raw-live-tree\n")
	writeFile(t, filepath.Join(fixture.liveRoot, "nvim", "cache", "ignored.lua"), "ignored-live-cache\n")
	writeFileTreeResourceDesired(t, fixture, "tree.app", "config", "init.lua", "raw-desired-tree\n")
	fixture.trustRecipe()

	for _, command := range []string{CommandStatus, CommandDiff, CommandSave, CommandApply} {
		t.Run(command, func(t *testing.T) {
			report, err := Build(Options{
				Command:   command,
				RepoRoot:  fixture.repoRoot,
				StateRoot: fixture.stateRoot,
				Ref:       "tree.app:config",
				UserID:    "leon",
				DryRun:    command == CommandSave || command == CommandApply,
			})
			require.NoError(t, err)
			require.Len(t, report.Items, 1)
			item := report.Items[0]
			require.Equal(t, recipe.FileTreeDriverID, item.Resource.DriverID)
			require.Equal(t, "nvim", item.Resource.RelPath)
			require.True(t, filepath.IsAbs(item.Resource.Path))
			require.Equal(t, SelectorInfo{Kind: "file-tree", Summary: "nvim"}, item.Selector)
			require.Equal(t, "desired://user/leon/targets/tree.app/artifacts/config", item.DesiredURI)
			require.Equal(t, filepath.ToSlash(filepath.Join("desired", "user", "leon", "targets", "tree.app", "artifacts", "config")), item.DesiredRelPath)
			require.True(t, item.Current.Exists)
			require.Equal(t, 1, item.Current.FileCount)
			require.Equal(t, "present", item.Desired.Status)
			require.Equal(t, "file-tree", item.Desired.Kind)
			require.True(t, item.Desired.Snapshot.Exists)
			require.Equal(t, 1, item.Desired.Snapshot.FileCount)
			require.NotNil(t, item.Preview)
			require.Equal(t, "update", item.Preview.ChangeKind)
			if command == CommandSave || command == CommandApply {
				require.Equal(t, desired.IntentSet, item.Preview.Intent)
				require.Contains(t, item.PlannedAction, "would-")
			}
			if command == CommandDiff {
				require.NotNil(t, item.Diff)
				require.Equal(t, "metadata-only", item.Diff.Mode)
				require.Equal(t, "raw file-tree contents omitted", item.Diff.Redaction)
				require.Equal(t, "update", item.Diff.Kind)
			}
			payload := mustJSON(t, report)
			require.NotContains(t, payload, "raw-live-tree")
			require.NotContains(t, payload, "raw-desired-tree")
			require.NotContains(t, payload, "ignored-live-cache")
			require.NotContains(t, Text(report), "raw-live-tree")
			require.NotContains(t, Text(report), "raw-desired-tree")
		})
	}
}

func TestBuildBundledNvimUsesConfigArtifactDirectory(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	liveConfigRoot := t.TempDir()
	stateRoot := t.TempDir()
	writeV2Root(t, repoRoot, recipe.NvimTarget, "config")
	writeFile(t, filepath.Join(liveConfigRoot, "nvim", "init.lua"), "raw-nvim-config\n")
	writeFile(t, filepath.Join(liveConfigRoot, "nvim", "cache", "ignored.lua"), "ignored-cache\n")

	report, err := Build(Options{
		Command:   CommandSave,
		RepoRoot:  repoRoot,
		StateRoot: stateRoot,
		Ref:       "nvim:config",
		UserID:    "leon",
		DryRun:    true,
		LocationRoots: map[string]map[string]string{
			recipe.NvimTarget: {"config": liveConfigRoot},
		},
	})
	require.NoError(t, err)
	require.Len(t, report.Items, 1)
	item := report.Items[0]
	require.Equal(t, recipe.NvimTarget, item.TargetRef)
	require.Equal(t, "nvim:config", item.SettingRef)
	require.Equal(t, recipe.FileTreeDriverID, item.Resource.DriverID)
	require.Equal(t, "nvim", item.Resource.RelPath)
	require.Equal(t, "desired://user/leon/targets/nvim/artifacts/config", item.DesiredURI)
	require.Equal(t, filepath.ToSlash(filepath.Join("desired", "user", "leon", "targets", "nvim", "artifacts", "config")), item.DesiredRelPath)
	require.NotContains(t, item.DesiredRelPath, "settings.yaml")
	require.Equal(t, "file-tree", item.Desired.Kind)
	require.Equal(t, PlannedActionWouldPromote, item.PlannedAction)
	require.NotContains(t, mustJSON(t, report), "raw-nvim-config")
	require.NotContains(t, mustJSON(t, report), "ignored-cache")
}

func TestBuildFileTreeResourceMissingStatesFollowNoDeletePolicy(t *testing.T) {
	t.Parallel()

	t.Run("save blocks missing live tree and preserves desired", func(t *testing.T) {
		t.Parallel()

		fixture := setupFixtureWithRecipe(t, "tree.app", "config", fileTreeRecipeBody("tree.app", fixtureLiveRootPlaceholder))
		desiredRoot := writeFileTreeResourceDesired(t, fixture, "tree.app", "config", "init.lua", "keep-desired-tree\n")
		fixture.trustRecipe()

		report, err := Build(Options{Command: CommandSave, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "tree.app:config", UserID: "leon", DryRun: true})
		require.NoError(t, err)
		require.Equal(t, SummaryBlocked, report.Summary.Status)
		require.Len(t, report.Items, 1)
		item := report.Items[0]
		require.Equal(t, v2status.StateBlockedSafety, item.State)
		require.Equal(t, "desired://user/leon/targets/tree.app/artifacts/config", item.DesiredURI)
		require.Equal(t, filepath.ToSlash(filepath.Join("desired", "user", "leon", "targets", "tree.app", "artifacts", "config")), item.DesiredRelPath)
		requireDiagnostic(t, item, "selectedpreview.fileResource.plan")
		require.Contains(t, item.Diagnostics[0].Message, "live tree is missing")
		require.Equal(t, "keep-desired-tree\n", readFile(t, filepath.Join(desiredRoot, "init.lua")))
	})

	t.Run("apply previews creation when live tree is missing", func(t *testing.T) {
		t.Parallel()

		fixture := setupFixtureWithRecipe(t, "tree.app", "config", fileTreeRecipeBody("tree.app", fixtureLiveRootPlaceholder))
		writeFileTreeResourceDesired(t, fixture, "tree.app", "config", "init.lua", "desired-tree\n")
		fixture.trustRecipe()

		report, err := Build(Options{Command: CommandApply, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "tree.app:config", UserID: "leon", DryRun: true})
		require.NoError(t, err)
		require.Equal(t, SummaryChanged, report.Summary.Status)
		require.Len(t, report.Items, 1)
		item := report.Items[0]
		require.Equal(t, v2status.StateMissingCurrent, item.State)
		require.Equal(t, PlannedActionWouldApply, item.PlannedAction)
		require.NotNil(t, item.Preview)
		require.Equal(t, "create", item.Preview.ChangeKind)
		require.Equal(t, desired.IntentSet, item.Preview.Intent)
	})

	t.Run("apply blocks missing desired tree and preserves live", func(t *testing.T) {
		t.Parallel()

		fixture := setupFixtureWithRecipe(t, "tree.app", "config", fileTreeRecipeBody("tree.app", fixtureLiveRootPlaceholder))
		writeFile(t, filepath.Join(fixture.liveRoot, "nvim", "init.lua"), "keep-live-tree\n")
		fixture.trustRecipe()

		report, err := Build(Options{Command: CommandApply, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "tree.app:config", UserID: "leon", DryRun: true})
		require.NoError(t, err)
		require.Equal(t, SummaryBlocked, report.Summary.Status)
		require.Len(t, report.Items, 1)
		item := report.Items[0]
		require.Equal(t, v2status.StateBlockedSafety, item.State)
		require.Equal(t, "desired://user/leon/targets/tree.app/artifacts/config", item.DesiredURI)
		require.Equal(t, filepath.ToSlash(filepath.Join("desired", "user", "leon", "targets", "tree.app", "artifacts", "config")), item.DesiredRelPath)
		requireDiagnostic(t, item, "selectedpreview.fileResource.plan")
		require.Contains(t, item.Diagnostics[0].Message, "desired artifact is missing")
		require.Equal(t, "keep-live-tree\n", readFile(t, filepath.Join(fixture.liveRoot, "nvim", "init.lua")))
	})
}

func TestBuildFileResourceRequiresTrustBeforeLiveRead(t *testing.T) {
	t.Parallel()

	fixture := setupFixtureWithRecipe(t, "file.app", "identity.email", fileRecipeBody("file.app", fixtureLiveRootPlaceholder))
	writeFile(t, filepath.Join(fixture.liveRoot, "config.txt"), "raw-live-file\n")
	writeFileResourceDesired(t, fixture, "file.app", "identity.email", "raw-desired-file\n")

	report, err := Build(Options{Command: CommandStatus, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "file.app:identity.email", UserID: "leon"})
	require.NoError(t, err)
	require.Equal(t, SummaryBlocked, report.Summary.Status)
	require.Len(t, report.Items, 1)
	item := report.Items[0]
	require.Equal(t, v2status.StateBlockedSafety, item.State)
	require.Empty(t, item.Current.SHA256)
	requireDiagnostic(t, item, "trust.local.missingRecord")
	require.NotContains(t, mustJSON(t, report), "raw-live-file")
	require.NotContains(t, mustJSON(t, report), "raw-desired-file")
}

func TestErrorReportAndHelperBranches(t *testing.T) {
	t.Parallel()

	report := ErrorReport(CommandApply, true, "code", "message", map[string]any{"x": "y"})
	require.Equal(t, SummaryError, report.Summary.Status)
	require.Equal(t, "code", report.Error.Code)
	payload, err := JSON(report)
	require.NoError(t, err)
	require.Contains(t, payload, "message")

	nilPayload, err := JSON(nil)
	require.NoError(t, err)
	require.Contains(t, nilPayload, SummaryError)
	require.Contains(t, Text(nil), "summary status=error")

	require.Equal(t, "boom", (&Error{Message: "boom"}).Error())
	require.Equal(t, 1, (&Error{}).ExitCode())
	require.Equal(t, 7, (&Error{Exit: 7}).ExitCode())
	require.Empty(t, (*Error)(nil).Error())
	require.Equal(t, 1, (*Error)(nil).ExitCode())

	_, err = Build(Options{Command: "unknown", RepoRoot: "."})
	require.Error(t, err)
	_, err = Build(Options{Command: CommandStatus, RepoRoot: ""})
	require.Error(t, err)

	ref, err := parseRef("test.app")
	require.NoError(t, err)
	require.Equal(t, "test.app", ref.Target)
	_, err = parseRef("bad:ref:extra")
	require.Error(t, err)
	_, err = parseRef("Bad")
	require.Error(t, err)

	require.Equal(t, "", recipeRef("unknown", "target"))
	require.Equal(t, "missing", existsLabel(false))
	require.Equal(t, "present", existsLabel(true))
	require.Equal(t, "unchanged", saveChangeKind(Snapshot{}, Snapshot{}))
	require.Equal(t, "delete", saveChangeKind(Snapshot{}, Snapshot{Exists: true, SHA256: "x", Normalizer: "n"}))
	require.Equal(t, "update", saveChangeKind(Snapshot{Exists: true, SHA256: "a", Normalizer: "n"}, Snapshot{Exists: true, SHA256: "b", Normalizer: "n"}))
	require.Equal(t, "unknown", diffInfo("").Kind)
}

const fixtureLiveRootPlaceholder = "__LIVE_ROOT__"

func setupFixtureWithRecipe(t *testing.T, target string, settingID string, bodyTemplate string) fixture {
	t.Helper()
	repoRoot := t.TempDir()
	liveRoot := t.TempDir()
	stateRoot := t.TempDir()
	writeV2Root(t, repoRoot, target, settingID)
	body := strings.ReplaceAll(bodyTemplate, fixtureLiveRootPlaceholder, liveRoot)
	writeFile(t, repoRoot+"/recipes/local/"+target+"/recipe.yaml", body)
	rec := decodeRecipe(t, body)
	return fixture{repoRoot: repoRoot, liveRoot: liveRoot, stateRoot: stateRoot, recipe: rec, t: t}
}

func (f fixture) writeDesiredDelete() {
	writeFile(f.t, f.repoRoot+"/desired/user/leon/targets/test.app/settings.yaml", "schema: dotfiles-manager.v2.desired-settings\nschemaVersion: 1\nvalues:\n  identity.email:\n    intent: delete\n")
}

func selectedRecipeBodyWithRedaction(target string, liveRoot string, redaction string) string {
	body := selectedRecipeBody(target, liveRoot)
	return strings.ReplaceAll(body, "redaction: redacted-for-display", "redaction: "+redaction)
}

func fileRecipeBody(target string, liveRoot string) string {
	return `schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: ` + target + `
displayName: File App
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
    artifactForm: file
    sensitivity: personal
    redaction: redacted-for-display
    lifecycle: allowed
    scopeDefault: user
    resource: config-file
resources:
  config-file:
    driver: file
    location: config
    path: config.txt
    capability: read-write
    sensitivity: personal
    redaction: redacted-for-display
    lifecycle: allowed
`
}

func fileTreeRecipeBody(target string, liveRoot string) string {
	return `schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: ` + target + `
displayName: Tree App
supportLevel: experimental
capability: read-write
locations:
  config:
    default: ` + liveRoot + `
settings:
  config:
    label: Config tree
    supportLevel: experimental
    capability: read-write
    artifactForm: file-tree
    sensitivity: personal
    redaction: redacted-for-display
    lifecycle: allowed
    scopeDefault: user
    resource: config-tree
resources:
  config-tree:
    driver: file-tree
    location: config
    path: nvim
    capability: read-write
    sensitivity: personal
    redaction: redacted-for-display
    lifecycle: allowed
    include:
      - "**"
    exclude:
      - "cache/**"
`
}

func writeFileResourceDesired(t *testing.T, fixture fixture, target string, artifact string, body string) string {
	t.Helper()
	path := filepath.Join(fixture.repoRoot, "desired", "user", "leon", "targets", target, "artifacts", artifact)
	writeFile(t, path, body)
	return path
}

func writeFileTreeResourceDesired(t *testing.T, fixture fixture, target string, artifact string, relPath string, body string) string {
	t.Helper()
	root := filepath.Join(fixture.repoRoot, "desired", "user", "leon", "targets", target, "artifacts", artifact)
	writeFile(t, filepath.Join(root, filepath.FromSlash(relPath)), body)
	return root
}

func TestRemainingHelperBranches(t *testing.T) {
	t.Parallel()

	iniSelector := selectorFromRecipe(recipe.Resource{Driver: recipe.IniFileDriverID, Selector: &recipe.Selector{Section: "user", Key: "email"}})
	require.Equal(t, "ini-key", iniSelector.Kind)
	require.Equal(t, "[user] email", iniSelector.Summary)
	tomlSelector := selectorFromRecipe(recipe.Resource{Driver: recipe.TOMLFileDriverID, Selector: &recipe.Selector{Path: []string{"user", "email"}}})
	require.Equal(t, SelectorInfo{Kind: "selected-path", Summary: "user.email", Path: []string{"user", "email"}}, tomlSelector)
	require.True(t, isSelectedValueDriver(recipe.TOMLFileDriverID))
	plistSelector := selectorFromRecipe(recipe.Resource{Driver: recipe.PlistFileDriverID, Selector: &recipe.Selector{Path: []string{"com.example", "enabled"}}})
	require.Equal(t, SelectorInfo{Kind: "selected-path", Summary: `["com.example","enabled"]`, Path: []string{"com.example", "enabled"}}, plistSelector)
	require.True(t, isSelectedValueDriver(recipe.PlistFileDriverID))
	require.Equal(t, "none", selectorFromRecipe(recipe.Resource{}).Kind)
	require.Equal(t, "unsupported", selectorFromRecipe(recipe.Resource{Driver: "other", Selector: &recipe.Selector{}}).Kind)

	require.Equal(t, "", plannedAction(CommandStatus, Item{State: v2status.StateChangedCurrent}))
	require.Equal(t, "", plannedAction(CommandSave, Item{State: v2status.StateBlockedSafety}))
	require.Equal(t, PlannedActionNone, plannedAction(CommandSave, Item{State: v2status.StateUnchanged}))
	require.Equal(t, PlannedActionNone, plannedAction(CommandApply, Item{State: v2status.StateUnchanged}))
	require.Equal(t, PlannedActionWouldApply, plannedAction(CommandApply, Item{State: v2status.StateReadyToApply}))
	require.Equal(t, PlannedActionWouldPromote, plannedSaveActionForMissingDesired(Item{Current: Snapshot{Exists: true}}))
	require.Equal(t, PlannedActionWouldSave, plannedSaveActionForMissingDesired(Item{}))
	require.True(t, IsSavePlannedAction(PlannedActionWouldPromote))
	require.True(t, IsSavePlannedAction(PlannedActionWouldSave))
	require.False(t, IsSavePlannedAction(PlannedActionWouldApply))

	require.Equal(t, desired.IntentDelete, saveIntent(Snapshot{}))
	require.Equal(t, desired.IntentSet, saveIntent(Snapshot{Exists: true}))
	require.Equal(t, "create", saveChangeKind(Snapshot{Exists: true, SHA256: "x", Normalizer: "n"}, Snapshot{}))
	require.Equal(t, "unchanged", saveChangeKind(Snapshot{Exists: true, SHA256: "x", Normalizer: "n"}, Snapshot{Exists: true, SHA256: "x", Normalizer: "n"}))

	require.Equal(t, v2status.BlockerLifecycle, blockerForState(v2status.StateBlockedLifecycle, "app running").Code)
	require.Equal(t, v2status.BlockerUnknown, blockerForState(v2status.StateUnknown, "unknown").Code)
	require.Equal(t, "selected-value.delete.v1", deleteSentinel("").Normalizer)

	d := diagnostic("code", "", "message", "ref")
	require.Equal(t, SeverityError, d.Severity)
	require.Equal(t, "existing", (Diagnostic{Ref: "existing"}).withRef("new").Ref)
	rd := fromRecipeDiagnostic(recipe.ValidationDiagnostic{Code: "recipe.code", Message: "recipe message"}, "ref", recipe.RecipeSourceLocal, "resource", recipe.YAMLFileDriverID)
	require.Equal(t, SeverityError, rd.Severity)

	applyPlanToItem(nil, nil)
	applyReadPlanToItem(nil, nil)
	item := Item{}
	applyPlanToItem(&item, nil)
	applyReadPlanToItem(&item, nil)
	appendPlanDiagnostics(nil, nil)
	appendPlanDiagnostics(&item, nil)

	badJSON := &Report{Schema: Schema, SchemaVersion: SchemaVersion, Command: CommandStatus, Error: &ErrorObj{Details: map[string]any{"bad": func() {}}}}
	_, err := JSON(badJSON)
	require.Error(t, err)

	fileRoot := filepath.Join(t.TempDir(), "not-dir")
	writeFile(t, fileRoot, "x")
	_, err = normalizeRepoRoot(fileRoot)
	require.Error(t, err)

	fixture := setupFixture(t)
	_, err = Build(Options{Command: CommandStatus, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "test.app:identity.email"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "user id required")
}

func TestAdditionalPreviewHelperBranchesForLiveWriteUX(t *testing.T) {
	t.Parallel()

	emptyReport := baseReport(CommandApply, false, nil)
	require.Contains(t, Text(emptyReport), "items: none")

	richReport := baseReport(CommandApply, true, []string{"global", "user/leon"})
	richReport.Items = []Item{{
		SettingRef:    "test.app:identity.email",
		Scope:         "user",
		Subject:       "leon",
		State:         v2status.StateReadyToApply,
		Desired:       DesiredInfo{Status: desired.StatusPresent},
		Current:       Snapshot{Exists: true},
		Resource:      ResourceInfo{ID: "config-email", DriverID: recipe.YAMLFileDriverID},
		Selector:      SelectorInfo{Summary: "user.email"},
		Diff:          diffInfo("update"),
		Message:       "safe message",
		PlannedAction: PlannedActionWouldApply,
		Mutation: &MutationInfo{
			Result:     "verified",
			RunID:      "run-rich",
			BackupRefs: []string{"state://backups/run-rich/items/config-email"},
			Verification: VerificationInfo{
				Verified: true,
				Result:   "verified",
			},
		},
		Diagnostics: []Diagnostic{{Severity: SeverityWarning, Code: "safe.warning", Message: "safe diagnostic"}},
	}}
	richText := Text(richReport)
	require.Contains(t, richText, "MODE: DRY RUN")
	require.Contains(t, richText, "profile: global -> user/leon")
	require.Contains(t, richText, "backups=state://backups/run-rich/items/config-email")
	require.Contains(t, richText, "warning[safe.warning]")

	_, err := normalizeRepoRoot(filepath.Join(t.TempDir(), "missing"))
	require.Error(t, err)
	report := errorReport(Options{}, "safe.code", "safe message", nil)
	require.Equal(t, CommandStatus, report.Command)

	_, err = parseRef("test.app:Bad")
	require.Error(t, err)
	_, err = parseRef("test.app:")
	require.Error(t, err)
	_, err = parseRef("bad/target")
	require.Error(t, err)

	settings := []resolution.ResolvedSetting{
		{TargetID: "a", SettingID: "one"},
		{TargetID: "b", SettingID: "two"},
	}
	require.Len(t, filterSettings(settings, parsedRef{Empty: true}), 2)
	require.Len(t, filterSettings(settings, parsedRef{Target: "a"}), 1)
	require.Empty(t, filterSettings(settings, parsedRef{Target: "a", Setting: "missing"}))

	appendDesiredDiagnostics(nil, errors.New("safe"))
	item := Item{SettingRef: "test.app:identity.email", Resource: ResourceInfo{ID: "config-email", DriverID: recipe.YAMLFileDriverID}}
	appendDesiredDiagnostics(&item, nil)
	require.Empty(t, item.Diagnostics)
	appendDesiredDiagnostics(&item, errors.New("safe fallback"))
	requireDiagnostic(t, item, "selectedpreview.desired.writeSafety")

	err = validateExistingDesiredForPlanning("", desired.ReadResult{}, nil, resolution.ResolvedSetting{}, recipe.WriteSafetyContext{})
	require.Error(t, err)
	err = validateCurrentForSavePlanning("", nil, resolution.ResolvedSetting{}, nil, recipe.WriteSafetyContext{})
	require.Error(t, err)

	_, err = desiredValueFromSelected(selectedvalue.Desired{})
	require.Error(t, err)

	planItem := Item{}
	plan := &selectedvalue.Plan{
		Path:        "/tmp/config.yaml",
		Selector:    selectedvalue.SelectorInfo{Kind: "selected-path", Summary: "user.email", Path: []string{"user", "email"}},
		Current:     selectedvalue.Snapshot{Exists: true, SHA256: "current", Normalizer: "yaml-file.selected-scalar.v1"},
		Desired:     &selectedvalue.Snapshot{Exists: true, SHA256: "desired", Normalizer: "yaml-file.selected-scalar.v1"},
		ChangeKind:  "update",
		Intent:      desired.IntentSet,
		Diagnostics: []selectedvalue.Diagnostic{{Code: "plan.safe", Severity: SeverityInfo, Message: "safe plan", Ref: "test.app:identity.email", Path: "config.yaml", ResourceID: "config-email", DriverID: recipe.YAMLFileDriverID}},
	}
	applyPlanToItem(&planItem, plan)
	require.Equal(t, "/tmp/config.yaml", planItem.Resource.Path)
	require.Equal(t, "selected-path", planItem.Selector.Kind)
	require.Equal(t, "desired", planItem.Desired.Snapshot.SHA256)
	require.NotNil(t, planItem.Preview)
	requireDiagnostic(t, planItem, "plan.safe")

	readItem := Item{}
	applyReadPlanToItem(&readItem, plan)
	require.Equal(t, "current", readItem.Current.SHA256)
	requireDiagnostic(t, readItem, "plan.safe")

	deleteItem := Item{TargetRef: "test.app", SettingRef: "test.app:identity.email", Current: Snapshot{}, Desired: DesiredInfo{Snapshot: Snapshot{Normalizer: "yaml-file.selected-scalar.v1"}}}
	deriveItemState(&deleteItem, CommandApply, desired.IntentDelete)
	require.Equal(t, v2status.StateUnchanged, deleteItem.State)
	require.Equal(t, v2status.ContextSave, statusContext(CommandSave))
	require.Equal(t, v2status.ContextApply, statusContext(CommandApply))
	require.Equal(t, v2status.ContextStatus, statusContext(CommandDiff))
	require.Equal(t, "hash", normalizedSnapshot(Snapshot{Exists: true, SHA256: "hash", Normalizer: "norm"}).Hash)
	require.Equal(t, Snapshot{Exists: true, SHA256: "hash", Normalizer: "norm"}, fromSnapshot(selectedvalue.Snapshot{Exists: true, SHA256: "hash", Normalizer: "norm"}))
	require.Equal(t, SelectorInfo{Kind: "selected-path", Summary: "user.email", Path: []string{"user", "email"}}, selectorInfo(selectedvalue.SelectorInfo{Kind: "selected-path", Summary: "user.email", Path: []string{"user", "email"}}))

	blockedLifecycle := finishBlocked(Item{}, v2status.StateBlockedLifecycle, "app is running")
	require.Equal(t, v2status.StateBlockedLifecycle, blockedLifecycle.State)
	require.NotEmpty(t, blockedLifecycle.AllowedActions)

	finishReport(nil)
	blocked := &Report{Command: CommandSave, Items: []Item{
		{State: v2status.StateBlockedLifecycle},
		{State: v2status.StateUnchanged},
		{State: v2status.StateChangedCurrent, PlannedAction: PlannedActionWouldSave},
		{State: v2status.StateMissingDesired, PlannedAction: PlannedActionWouldPromote},
	}}
	finishReport(blocked)
	require.Equal(t, SummaryBlocked, blocked.Summary.Status)
	require.Equal(t, 1, blocked.Summary.Blocked)
	require.Equal(t, 2, blocked.Summary.Saved)

	changedApply := &Report{Command: CommandApply, Items: []Item{{State: v2status.StateReadyToApply, PlannedAction: PlannedActionWouldApply}}}
	finishReport(changedApply)
	require.Equal(t, SummaryChanged, changedApply.Summary.Status)
	require.Equal(t, 1, changedApply.Summary.Applied)

	okReport := &Report{Command: CommandStatus, Items: []Item{{State: v2status.StateUnchanged}}}
	finishReport(okReport)
	require.Equal(t, SummaryOK, okReport.Summary.Status)
}

func TestBuildMacOSDefaultsReadOnlyStatusAndDiffUseFakeRunner(t *testing.T) {
	t.Parallel()

	fixture := setupFixtureWithRecipe(t, "test.defaults", "show-hidden-files", macOSDefaultsPreviewRecipeBody("test.defaults"))
	fixture.writeDesiredSetFor("test.defaults", "show-hidden-files", "new@example.com")
	fixture.trustRecipe()

	for _, command := range []string{CommandStatus, CommandDiff} {
		t.Run(command, func(t *testing.T) {
			runner := &previewDefaultsRunner{result: macosdefaultsdriver.ExportResult{Stdout: defaultsExportForPreview(t, map[string]any{"AppleShowAllFiles": "old@example.com"})}}
			report, err := Build(Options{Command: command, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "test.defaults:show-hidden-files", UserID: "leon", MachineID: "mbp", MacOSDefaultsRunner: runner})
			require.NoError(t, err)
			require.Equal(t, SummaryChanged, report.Summary.Status)
			require.Len(t, report.Items, 1)
			item := report.Items[0]
			require.Equal(t, recipe.MacOSDefaultsReadOnlyDriverID, item.Resource.DriverID)
			require.Equal(t, "defaults://current-user/com.apple.finder/AppleShowAllFiles", item.Resource.Path)
			require.Equal(t, "macos-defaults-key", item.Selector.Kind)
			require.True(t, item.Current.Exists)
			require.NotNil(t, item.Preview)
			require.True(t, item.Preview.ReadOnly)
			require.Len(t, runner.calls, 1)
			if command == CommandDiff {
				require.NotNil(t, item.Diff)
				require.Equal(t, "metadata-only", item.Diff.Mode)
			}
			require.NotContains(t, mustJSON(t, report), "old@example.com")
			require.NotContains(t, mustJSON(t, report), "new@example.com")
			require.Contains(t, Text(report), "read-only")
		})
	}
}

func TestBuildMacOSDefaultsReadOnlyBlocksSaveApplyBeforeLiveRead(t *testing.T) {
	t.Parallel()

	fixture := setupFixtureWithRecipe(t, "test.defaults", "show-hidden-files", macOSDefaultsPreviewRecipeBody("test.defaults"))
	fixture.writeDesiredSetFor("test.defaults", "show-hidden-files", "new@example.com")
	fixture.trustRecipe()

	for _, command := range []string{CommandSave, CommandApply} {
		t.Run(command, func(t *testing.T) {
			runner := &previewDefaultsRunner{result: macosdefaultsdriver.ExportResult{Stdout: defaultsExportForPreview(t, map[string]any{"AppleShowAllFiles": "old@example.com"})}}
			report, err := Build(Options{Command: command, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "test.defaults:show-hidden-files", UserID: "leon", MachineID: "mbp", DryRun: true, MacOSDefaultsRunner: runner})
			require.NoError(t, err)
			require.Equal(t, SummaryBlocked, report.Summary.Status)
			require.Len(t, report.Items, 1)
			require.Equal(t, v2status.StateUnsupported, report.Items[0].State)
			requireDiagnostic(t, report.Items[0], "selectedpreview.driver.readOnly")
			require.Empty(t, runner.calls, "save/apply must be blocked before a defaults export can synthesize or mutate state")
			require.NotContains(t, mustJSON(t, report), "old@example.com")
			require.NotContains(t, mustJSON(t, report), "new@example.com")
		})
	}
}

func TestBuildMacOSDefaultsReadOnlyBlocksSaveWithMissingDesiredBeforeLiveRead(t *testing.T) {
	t.Parallel()

	fixture := setupFixtureWithRecipe(t, "test.defaults", "show-hidden-files", macOSDefaultsPreviewRecipeBody("test.defaults"))
	fixture.trustRecipe()
	runner := &previewDefaultsRunner{result: macosdefaultsdriver.ExportResult{Stdout: defaultsExportForPreview(t, map[string]any{"AppleShowAllFiles": true})}}

	report, err := Build(Options{Command: CommandSave, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "test.defaults:show-hidden-files", UserID: "leon", MachineID: "mbp", DryRun: true, MacOSDefaultsRunner: runner})
	require.NoError(t, err)
	require.Equal(t, SummaryBlocked, report.Summary.Status)
	requireDiagnostic(t, report.Items[0], "selectedpreview.driver.readOnly")
	require.Empty(t, runner.calls)
}

type previewDefaultsRunner struct {
	result macosdefaultsdriver.ExportResult
	err    error
	calls  []string
}

func (r *previewDefaultsRunner) Export(ctx context.Context, domain string, limits macosdefaultsdriver.OutputLimits) (macosdefaultsdriver.ExportResult, error) {
	r.calls = append(r.calls, domain)
	return r.result, r.err
}

func macOSDefaultsPreviewRecipeBody(target string) string {
	return `schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: ` + target + `
displayName: Test Defaults
supportLevel: experimental
capability: read-only
locations:
  macos-defaults:
    default: macos-defaults://current-user
settings:
  show-hidden-files:
    label: Show hidden files
    supportLevel: experimental
    capability: read-only
    artifactForm: scalar
    sensitivity: low
    redaction: known-safe
    lifecycle: allowed
    scopeDefault: user
    resource: finder-show-hidden
resources:
  finder-show-hidden:
    driver: macos-defaults-readonly
    location: macos-defaults
    path: com.apple.finder
    capability: read-only
    sensitivity: low
    redaction: known-safe
    lifecycle: allowed
    selector:
      key: AppleShowAllFiles
`
}

func defaultsExportForPreview(t *testing.T, value map[string]any) []byte {
	t.Helper()
	data, err := plist.Marshal(value, plist.XMLFormat)
	require.NoError(t, err)
	return data
}
