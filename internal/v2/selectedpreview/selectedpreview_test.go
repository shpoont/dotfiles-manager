package selectedpreview

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shpoont/dotfiles-manager/internal/v2/desired"
	"github.com/shpoont/dotfiles-manager/internal/v2/macosdefaultsdriver"
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

	fileFixture := setupFixtureWithRecipe(t, "file.app", "identity.email", fileRecipeBody("file.app", fixtureLiveRootPlaceholder))
	report, err := Build(Options{Command: CommandStatus, RepoRoot: fileFixture.repoRoot, StateRoot: fileFixture.stateRoot, Ref: "file.app:identity.email", UserID: "leon"})
	require.NoError(t, err)
	require.Equal(t, SummaryBlocked, report.Summary.Status)
	require.Equal(t, v2status.StateUnsupported, report.Items[0].State)
	requireDiagnostic(t, report.Items[0], "selectedpreview.driver.unsupported")

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
