package preview

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shpoont/dotfiles-manager/internal/v2/customfiles"
	"github.com/shpoont/dotfiles-manager/internal/v2/filedriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/shpoont/dotfiles-manager/internal/v2/resolution"
	"github.com/shpoont/dotfiles-manager/internal/v2/status"
	"github.com/stretchr/testify/require"
)

func TestTextAndJSONSnapshotsAreDeterministic(t *testing.T) {
	t.Parallel()

	envelope := BuildEnvelope(EnvelopeOptions{
		Command:      CommandApply,
		RunID:        "run-preview-001",
		ProfileStack: []string{"shared", "user/leon"},
		Items: []Item{{
			TargetRef:      "custom.files",
			SettingRef:     "custom.files:file",
			Scope:          "user",
			Subject:        "leon",
			DesiredURI:     "desired://user/leon/targets/custom.files/artifacts/config.txt",
			DesiredRelPath: "desired/user/leon/targets/custom.files/artifacts/config.txt",
			Operation:      "apply",
			Driver:         "file",
			ResourceID:     "config-file",
			LivePath:       "/home/leon/.cobona/config.txt",
			DesiredPath:    "/repo/desired/user/leon/targets/custom.files/artifacts/config.txt",
			DryRun:         true,
			State:          status.StateReadyToApply,
			Message:        "Desired differs from current and there is no previous sync baseline; applying will replace live state.",
			Actions:        []status.Action{status.ActionApply},
			Change: Change{
				Kind:   filedriver.ChangeUpdate,
				Before: Snapshot{Exists: true, Size: 10, SHA256: "before"},
				After:  Snapshot{Exists: true, Size: 11, SHA256: "after"},
			},
			Backup: Backup{Policy: BackupRequired},
			Result: ResultWouldChange,
		}},
	})

	wantText := `dotfiles-manager v2 apply preview
Run: run-preview-001
Profiles: shared > user/leon
Summary: changed (changed=1 blocked=0 saved=0 applied=0 skipped=0 failed=0)

custom.files:file
  State: ready-to-apply
  Result: would-change
  Detail: Desired differs from current and there is no previous sync baseline; applying will replace live state.
  Change: update live target
  Live: /home/leon/.cobona/config.txt
  Desired artifact: desired/user/leon/targets/custom.files/artifacts/config.txt
  Desired path: /repo/desired/user/leon/targets/custom.files/artifacts/config.txt
  Desired URI: desired://user/leon/targets/custom.files/artifacts/config.txt
  Next: apply
  Backup: Backup would be required before live write; backup ledger is not available in this preview.

Use --json for technical details.
`
	require.Equal(t, wantText, RenderText(envelope))
	require.Equal(t, wantText, RenderText(envelope), "text rendering must be stable across calls")

	json1, err := JSON(envelope)
	require.NoError(t, err)
	json2, err := JSON(envelope)
	require.NoError(t, err)
	require.Equal(t, json1, json2, "JSON rendering must be stable across calls")

	wantJSON := `{
  "schema": "dotfiles-manager.v2.preview",
  "schemaVersion": 1,
  "command": "apply",
  "runId": "run-preview-001",
  "profileStack": [
    "shared",
    "user/leon"
  ],
  "summary": {
    "status": "changed",
    "changed": 1,
    "blocked": 0,
    "applied": 0,
    "saved": 0,
    "skipped": 0,
    "failed": 0
  },
  "items": [
    {
      "targetRef": "custom.files",
      "settingRef": "custom.files:file",
      "scope": "user",
      "subject": "leon",
      "desiredUri": "desired://user/leon/targets/custom.files/artifacts/config.txt",
      "desiredRelPath": "desired/user/leon/targets/custom.files/artifacts/config.txt",
      "operation": "apply",
      "driver": "file",
      "resourceId": "config-file",
      "livePath": "/home/leon/.cobona/config.txt",
      "desiredPath": "/repo/desired/user/leon/targets/custom.files/artifacts/config.txt",
      "dryRun": true,
      "state": "ready-to-apply",
      "message": "Desired differs from current and there is no previous sync baseline; applying will replace live state.",
      "actions": [
        "apply"
      ],
      "change": {
        "kind": "update",
        "before": {
          "exists": true,
          "size": 10,
          "sha256": "before"
        },
        "after": {
          "exists": true,
          "size": 11,
          "sha256": "after"
        }
      },
      "backup": {
        "policy": "required",
        "message": "Backup would be required before live write; backup ledger is not available in this preview."
      },
      "result": "would-change",
      "syncMode": "none",
      "automaticMerge": false
    }
  ]
}
`
	require.Equal(t, wantJSON, json1)
}

func TestFromCustomFilesPlanBuildsScriptableDryRunPreview(t *testing.T) {
	t.Parallel()

	root, liveRoot, profile, rec := setupCustomFilesFixture(t)
	livePath := filepath.Join(liveRoot, "config.txt")
	desiredPath := desiredArtifactPath(root)
	writeFile(t, livePath, "live v1\n")
	writeFile(t, desiredPath, "desired v0\n")

	plan, err := customfiles.PlanSave(customfiles.Request{Profile: profile, Recipe: rec, SettingRef: "custom.files:file", LocationRoots: map[string]string{"config": liveRoot}})
	require.NoError(t, err)
	item, err := FromCustomFilesPlan(plan, CustomFilesPlanOptions{DryRun: true})
	require.NoError(t, err)

	require.Equal(t, "custom.files", item.TargetRef)
	require.Equal(t, "custom.files:file", item.SettingRef)
	require.Equal(t, "user", item.Scope)
	require.Equal(t, "leon", item.Subject)
	require.Equal(t, "save", item.Operation)
	require.Equal(t, recipe.FileDriverID, item.Driver)
	require.Equal(t, filedriver.ChangeUpdate, item.Change.Kind)
	require.Equal(t, livePath, item.LivePath)
	require.Equal(t, desiredPath, item.DesiredPath)
	require.Equal(t, status.StateChangedCurrent, item.State)
	require.Equal(t, ResultWouldChange, item.Result)
	require.Equal(t, BackupNotApplicable, item.Backup.Policy)
	require.Contains(t, item.Backup.Message, "No live backup")

	envelope := BuildEnvelope(EnvelopeOptions{Command: CommandSave, RunID: "run-customfiles", ProfileStack: profile.Layers, Items: []Item{item}})
	require.Equal(t, SummaryChanged, envelope.Summary.Status)
	require.Equal(t, ExitChanged, ExitCode(envelope))
	require.Contains(t, RenderText(envelope), "Use --json for technical details.")
}

func TestApplyDryRunBackupMessageDoesNotInventLedger(t *testing.T) {
	t.Parallel()

	root, liveRoot, profile, rec := setupCustomFilesFixture(t)
	livePath := filepath.Join(liveRoot, "config.txt")
	desiredPath := desiredArtifactPath(root)
	writeFile(t, livePath, "live v1\n")
	writeFile(t, desiredPath, "desired v1\n")

	plan, err := customfiles.PlanApply(customfiles.Request{Profile: profile, Recipe: rec, SettingRef: "custom.files:file", LocationRoots: map[string]string{"config": liveRoot}})
	require.NoError(t, err)
	item, err := FromCustomFilesPlan(plan, CustomFilesPlanOptions{DryRun: true})
	require.NoError(t, err)

	require.Equal(t, BackupRequired, item.Backup.Policy)
	require.Empty(t, item.Backup.Ref)
	require.Equal(t, "Backup would be required before live write; backup ledger is not available in this preview.", item.Backup.Message)
}

func TestDryRunWritesNoDesiredArtifactsAndNoLiveState(t *testing.T) {
	t.Parallel()

	root, liveRoot, profile, rec := setupCustomFilesFixture(t)
	livePath := filepath.Join(liveRoot, "config.txt")
	desiredPath := desiredArtifactPath(root)
	req := customfiles.Request{Profile: profile, Recipe: rec, SettingRef: "custom.files:file", LocationRoots: map[string]string{"config": liveRoot}}

	writeFile(t, livePath, "live save\n")
	savePlan, err := customfiles.PlanSave(req)
	require.NoError(t, err)
	_, err = FromCustomFilesPlan(savePlan, CustomFilesPlanOptions{DryRun: true})
	require.NoError(t, err)
	saveResult, err := customfiles.Execute(savePlan, customfiles.ExecuteOptions{DryRun: true})
	require.NoError(t, err)
	require.True(t, saveResult.DryRun)
	require.False(t, saveResult.Mutated)
	assertMissing(t, desiredPath)
	requireFile(t, livePath, "live save\n")

	writeFile(t, desiredPath, "desired apply\n")
	writeFile(t, livePath, "live before apply\n")
	applyPlan, err := customfiles.PlanApply(req)
	require.NoError(t, err)
	_, err = FromCustomFilesPlan(applyPlan, CustomFilesPlanOptions{DryRun: true})
	require.NoError(t, err)
	applyResult, err := customfiles.Execute(applyPlan, customfiles.ExecuteOptions{DryRun: true})
	require.NoError(t, err)
	require.True(t, applyResult.DryRun)
	require.False(t, applyResult.Mutated)
	requireFile(t, desiredPath, "desired apply\n")
	requireFile(t, livePath, "live before apply\n")
}

func TestTextPreviewShowsSkippedAndBlockedItems(t *testing.T) {
	t.Parallel()

	envelope := BuildEnvelope(EnvelopeOptions{Command: CommandApply, RunID: "run-blocked", ProfileStack: []string{"global"}, Items: []Item{
		{
			TargetRef:  "custom.files",
			SettingRef: "custom.files:blocked",
			Operation:  "apply",
			LivePath:   "/home/leon/.cobona/config.txt",
			State:      status.StateBlockedSafety,
			Result:     ResultBlocked,
			Backup:     Backup{Policy: BackupSkippedForBlocker},
			Diagnostics: []Diagnostic{{
				Code:     "unsafe-selector",
				Severity: SeverityError,
				Message:  "selector escapes the managed location",
				Ref:      "custom.files:blocked",
				ExitCode: ExitSafetyBlocker,
			}},
		},
		{
			TargetRef:  "custom.files",
			SettingRef: "custom.files:skipped",
			Operation:  "apply",
			LivePath:   "/home/leon/.cobona/skipped.txt",
			State:      status.StateUnchanged,
			Result:     ResultSkipped,
			Backup:     Backup{Policy: BackupNotApplicable, Message: "Backup is not planned because the item is skipped."},
		},
	}})

	text := RenderText(envelope)
	require.Contains(t, text, "Summary: blocked (changed=0 blocked=1 saved=0 applied=0 skipped=1 failed=0)")
	require.Contains(t, text, "custom.files:blocked")
	require.Contains(t, text, "Result: blocked")
	require.Contains(t, text, "Change: inspect skipped item")
	require.Contains(t, text, "Diagnostic[error]: unsafe-selector - selector escapes the managed location (custom.files:blocked)")
	require.Contains(t, text, "custom.files:skipped")
	require.Contains(t, text, "Result: skipped")
	require.Contains(t, text, "Backup is not planned because the item is skipped.")
}

func TestExitCodeFixtures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		item Item
		want int
	}{
		{name: "success", item: Item{TargetRef: "ok", SettingRef: "ok:file", State: status.StateUnchanged, Result: ResultUnchanged}, want: ExitSuccess},
		{name: "validation error", item: Item{TargetRef: "bad", SettingRef: "bad:file", Result: ResultFailed, Diagnostics: []Diagnostic{{Code: "invalid-recipe", Severity: SeverityError, Message: "schema failed", ExitCode: ExitValidation}}}, want: ExitValidation},
		{name: "changed check result", item: Item{TargetRef: "changed", SettingRef: "changed:file", State: status.StateChangedCurrent, Result: ResultWouldChange, Change: Change{Kind: filedriver.ChangeUpdate}}, want: ExitChanged},
		{name: "input required", item: Item{TargetRef: "identity", SettingRef: "identity:file", Result: ResultFailed, Diagnostics: []Diagnostic{{Code: "identity-required", Severity: SeverityError, Message: "missing user id", ExitCode: ExitInputRequired}}}, want: ExitInputRequired},
		{name: "safety blocker", item: Item{TargetRef: "blocked", SettingRef: "blocked:file", State: status.StateBlockedSafety, Result: ResultBlocked}, want: ExitSafetyBlocker},
		{name: "internal error", item: Item{TargetRef: "panic", SettingRef: "panic:file", Result: ResultFailed, Diagnostics: []Diagnostic{{Code: "internal-error", Severity: SeverityError, Message: "boom", ExitCode: ExitInternalError}}}, want: ExitInternalError},
		{name: "partial success", item: Item{TargetRef: "partial", SettingRef: "partial:file", State: status.StateChangedCurrent, Result: ResultSaved, Diagnostics: []Diagnostic{{Code: "other-failed", Severity: SeverityError, Message: "second item failed", ExitCode: ExitSafetyBlocker}}}, want: ExitPartial},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			envelope := BuildEnvelope(EnvelopeOptions{Command: CommandStatus, RunID: "run-exit", ProfileStack: []string{"global"}, Items: []Item{tc.item}})
			require.Equal(t, tc.want, ExitCode(envelope))
		})
	}
}

func TestNormalizationSortsItemsDiagnosticsAndTreeEntries(t *testing.T) {
	t.Parallel()

	envelope := BuildEnvelope(EnvelopeOptions{Command: CommandDiff, RunID: "run-sort", Items: []Item{
		{TargetRef: "z", SettingRef: "z:file", Result: ResultUnchanged, Diagnostics: []Diagnostic{{Code: "z", Severity: SeverityWarning, Message: "z"}}},
		{TargetRef: "a", SettingRef: "a:file", Result: ResultWouldChange, Change: Change{Kind: filedriver.ChangeUpdate, Entries: []EntryChange{{Path: "z.txt", Kind: filedriver.ChangeDelete}, {Path: "a.txt", Kind: filedriver.ChangeCreate}}}, Diagnostics: []Diagnostic{{Code: "b", Severity: SeverityWarning, Message: "b"}, {Code: "a", Severity: SeverityWarning, Message: "a"}}},
	}})

	require.Equal(t, "a:file", envelope.Items[0].SettingRef)
	require.Equal(t, []string{"a.txt", "z.txt"}, []string{envelope.Items[0].Change.Entries[0].Path, envelope.Items[0].Change.Entries[1].Path})
	require.Equal(t, []string{"a", "b"}, []string{envelope.Items[0].Diagnostics[0].Code, envelope.Items[0].Diagnostics[1].Code})
}

func setupCustomFilesFixture(t *testing.T) (string, string, *resolution.ResolvedProfile, *recipe.Recipe) {
	t.Helper()

	root := t.TempDir()
	liveRoot := filepath.Join(t.TempDir(), "cobona")
	require.NoError(t, os.MkdirAll(liveRoot, 0o755))
	writeV2Root(t, root)
	writeStack(t, root)
	writeLayer(t, root)
	writeRecipe(t, root)

	profile, err := resolution.Resolve(root, resolution.ResolveOptions{UserID: "leon", MachineID: "mbp"})
	require.NoError(t, err)
	rec, err := recipe.LoadCustomFiles(root)
	require.NoError(t, err)
	return root, liveRoot, profile, rec
}

func writeV2Root(t *testing.T, root string) {
	t.Helper()
	writeFile(t, filepath.Join(root, resolution.RootConfigFile), "schema: dotfiles-manager.v2.root-config\nschemaVersion: 1\nactiveProfileStack: default\n")
}

func writeStack(t *testing.T, root string) {
	t.Helper()
	writeFile(t, filepath.Join(root, "profiles", "stacks", "default.yaml"), "schema: dotfiles-manager.v2.profile-stack\nschemaVersion: 1\nprofileStack:\n  - global\n")
}

func writeLayer(t *testing.T, root string) {
	t.Helper()
	writeFile(t, filepath.Join(root, "profiles", "layers", "global.yaml"), `schema: dotfiles-manager.v2.profile-layer
schemaVersion: 1
selections:
  custom.files:
    settings:
      file:
        scope: user
        artifact: artifacts/config.txt
`)
}

func writeRecipe(t *testing.T, root string) {
	t.Helper()
	writeFile(t, filepath.Join(root, "recipes", "local", "custom.files", "recipe.yaml"), `schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: custom.files
displayName: Custom files
supportLevel: experimental
capability: read-write
locations:
  config:
    default: ~/.cobona
settings:
  file:
    scopeDefault: user
    resource: config-file
resources:
  config-file:
    driver: file
    location: config
    path: config.txt
`)
}

func desiredArtifactPath(root string) string {
	return filepath.Join(root, "desired", "user", "leon", "targets", "custom.files", "artifacts", "config.txt")
}

func writeFile(t *testing.T, path string, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func requireFile(t *testing.T, path string, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, want, string(got))
}

func assertMissing(t *testing.T, path string) {
	t.Helper()
	_, err := os.Stat(path)
	require.True(t, os.IsNotExist(err), "expected %s to be missing, got %v", path, err)
}

func TestFileTreePlanPreviewCoversTreeEntries(t *testing.T) {
	t.Parallel()

	root, liveRoot, profile, rec := setupCustomFileTreeFixture(t)
	liveTree := filepath.Join(liveRoot, "profiles")
	desiredTree := desiredTreeArtifactPath(root)
	writeFile(t, filepath.Join(liveTree, "z.yaml"), "live z\n")
	writeFile(t, filepath.Join(liveTree, "a.yaml"), "live a\n")
	writeFile(t, filepath.Join(desiredTree, "a.yaml"), "desired a\n")

	plan, err := customfiles.PlanSave(customfiles.Request{Profile: profile, Recipe: rec, SettingRef: "custom.files:file", LocationRoots: map[string]string{"config": liveRoot}})
	require.NoError(t, err)
	item, err := FromCustomFilesPlan(plan, CustomFilesPlanOptions{DryRun: true, BackupRef: "state://backups/run/tree"})
	require.NoError(t, err)

	require.Equal(t, recipe.FileTreeDriverID, item.Driver)
	require.Equal(t, liveTree, item.LivePath)
	require.Equal(t, desiredTree, item.DesiredPath)
	require.Equal(t, filedriver.ChangeUpdate, item.Change.Kind)
	require.NotEmpty(t, item.Change.Entries)
	for i := 1; i < len(item.Change.Entries); i++ {
		require.LessOrEqual(t, item.Change.Entries[i-1].Path, item.Change.Entries[i].Path)
	}
}

func TestPreviewHelperBranches(t *testing.T) {
	t.Parallel()

	require.Contains(t, RenderText(BuildEnvelope(EnvelopeOptions{Command: CommandList, RunID: "run-empty", LedgerRef: "state://ledger/current/run-empty"})), "No managed items matched this preview.")
	require.Equal(t, "create", changeVerb(filedriver.ChangeCreate))
	require.Equal(t, "delete", changeVerb(filedriver.ChangeDelete))
	require.Equal(t, "leave unchanged", changeVerb(filedriver.ChangeUnchanged))
	require.Equal(t, "inspect", changeVerb(""))
	require.Equal(t, "replace", changeVerb("replace"))
	require.Equal(t, "desired artifact", changeTarget(Item{Operation: "save"}))
	require.Equal(t, "managed item", changeTarget(Item{Operation: "status"}))

	require.Equal(t, ResultBlocked, resultFromStateAndChange(status.StateBlockedLifecycle, filedriver.ChangeUpdate))
	require.Equal(t, ResultUnchanged, resultFromStateAndChange(status.StateUnchanged, filedriver.ChangeUnchanged))
	require.Equal(t, ResultWouldChange, resultFromStateAndChange(status.StateConflict, filedriver.ChangeUpdate))
	require.Equal(t, ExitSafetyBlocker, ExitCode(BuildEnvelope(EnvelopeOptions{Command: CommandStatus, Items: []Item{{TargetRef: "life", SettingRef: "life:file", State: status.StateBlockedLifecycle}}})))
	require.Equal(t, ExitChanged, ExitCode(BuildEnvelope(EnvelopeOptions{Command: CommandStatus, Items: []Item{{TargetRef: "conflict", SettingRef: "conflict:file", State: status.StateConflict}}})))

	require.Equal(t,
		[]status.Action{status.ActionGuidedSync, status.ActionDiff, status.ActionSave, status.ActionApply, status.ActionCreate, status.ActionQuit, status.ActionRetry, status.ActionSkip, status.ActionInspect, status.ActionFix, status.ActionCreateRecipe, status.ActionVerbose, status.Action("zzz")},
		normalizeActions([]status.Action{status.Action("zzz"), status.ActionVerbose, status.ActionCreateRecipe, status.ActionFix, status.ActionInspect, status.ActionSkip, status.ActionRetry, status.ActionQuit, status.ActionCreate, status.ActionApply, status.ActionSave, status.ActionDiff, status.ActionGuidedSync, status.ActionGuidedSync}),
	)

	require.Equal(t, "Backup is unsupported for this item; policy must allow proceeding before a live write.", defaultBackupMessage(Item{Backup: Backup{Policy: BackupUnsupported}}))
	require.Equal(t, "Backup would be required before live write; backup ledger is not available in this preview.", defaultBackupMessage(Item{Operation: "apply"}))
	require.Equal(t, "No live backup is required for this preview.", defaultBackupMessage(Item{}))
}

func setupCustomFileTreeFixture(t *testing.T) (string, string, *resolution.ResolvedProfile, *recipe.Recipe) {
	t.Helper()

	root := t.TempDir()
	liveRoot := filepath.Join(t.TempDir(), "cobona")
	require.NoError(t, os.MkdirAll(liveRoot, 0o755))
	writeV2Root(t, root)
	writeStack(t, root)
	writeTreeLayer(t, root)
	writeTreeRecipe(t, root)

	profile, err := resolution.Resolve(root, resolution.ResolveOptions{UserID: "leon", MachineID: "mbp"})
	require.NoError(t, err)
	rec, err := recipe.LoadCustomFiles(root)
	require.NoError(t, err)
	return root, liveRoot, profile, rec
}

func writeTreeLayer(t *testing.T, root string) {
	t.Helper()
	writeFile(t, filepath.Join(root, "profiles", "layers", "global.yaml"), `schema: dotfiles-manager.v2.profile-layer
schemaVersion: 1
selections:
  custom.files:
    settings:
      file:
        scope: user
        artifact: artifacts/profiles
`)
}

func writeTreeRecipe(t *testing.T, root string) {
	t.Helper()
	writeFile(t, filepath.Join(root, "recipes", "local", "custom.files", "recipe.yaml"), `schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: custom.files
displayName: Custom files
supportLevel: experimental
capability: read-write
locations:
  config:
    default: ~/.cobona
settings:
  file:
    scopeDefault: user
    resource: config-file
resources:
  config-file:
    driver: file-tree
    location: config
    path: profiles
    include:
      - "**"
    exclude:
      - "cache/**"
`)
}

func desiredTreeArtifactPath(root string) string {
	return filepath.Join(root, "desired", "user", "leon", "targets", "custom.files", "artifacts", "profiles")
}
