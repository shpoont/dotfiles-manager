package selectedpreview

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shpoont/dotfiles-manager/internal/v2/desired"
	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	v2status "github.com/shpoont/dotfiles-manager/internal/v2/status"
	"github.com/stretchr/testify/require"
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

func TestBuildKeepsBundledRuntimeUnsupportedUntilBundledRecipeIssue(t *testing.T) {
	t.Parallel()

	fixture := setupFixtureForTarget(t, "git", "user.email")
	report, err := Build(Options{Command: CommandStatus, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, UserID: "leon"})
	require.NoError(t, err)
	require.Equal(t, SummaryBlocked, report.Summary.Status)
	require.Len(t, report.Items, 1)
	item := report.Items[0]
	require.Equal(t, recipe.RecipeSourceBundled, item.Recipe.Source)
	require.Equal(t, v2status.StateUnsupported, item.State)
	requireDiagnostic(t, item, "selectedpreview.recipe.bundledRuntimeUnavailable")
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
	if target != recipe.GitTarget {
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
	writeFile(f.t, f.repoRoot+"/desired/user/leon/targets/test.app/settings.yaml", "schema: dotfiles-manager.v2.desired-settings\nschemaVersion: 1\nvalues:\n  identity.email:\n    intent: set\n    kind: string\n    value: "+email+"\n")
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
	require.Equal(t, "would-save", saveItem.PlannedAction)
	require.NotNil(t, saveItem.Preview)
	require.Equal(t, "create", saveItem.Preview.ChangeKind)
	require.Equal(t, desired.IntentSet, saveItem.Preview.Intent)
	require.False(t, saveItem.Mutated)

	report, err = Build(Options{Command: CommandApply, RepoRoot: fixture.repoRoot, StateRoot: fixture.stateRoot, Ref: "test.app:identity.email", UserID: "leon", DryRun: true})
	require.NoError(t, err)
	applyItem := report.Items[0]
	require.Equal(t, v2status.StateMissingDesired, applyItem.State)
	require.Equal(t, "blocked-missing-desired", applyItem.PlannedAction)
	require.Contains(t, applyItem.Message, "no desired artifact")
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
	require.Equal(t, "would-apply", item.PlannedAction)
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
	require.Equal(t, "none", selectorFromRecipe(recipe.Resource{}).Kind)
	require.Equal(t, "unsupported", selectorFromRecipe(recipe.Resource{Driver: "other", Selector: &recipe.Selector{}}).Kind)

	require.Equal(t, "", plannedAction(CommandStatus, Item{State: v2status.StateChangedCurrent}))
	require.Equal(t, "", plannedAction(CommandSave, Item{State: v2status.StateBlockedSafety}))
	require.Equal(t, "none", plannedAction(CommandSave, Item{State: v2status.StateUnchanged}))
	require.Equal(t, "none", plannedAction(CommandApply, Item{State: v2status.StateUnchanged}))
	require.Equal(t, "would-apply", plannedAction(CommandApply, Item{State: v2status.StateReadyToApply}))

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
