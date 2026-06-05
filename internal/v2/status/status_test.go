package status

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shpoont/dotfiles-manager/internal/v2/filedriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/filetreedriver"
	"github.com/stretchr/testify/require"
)

func TestDeriveItemCanonicalStatesForFileResources(t *testing.T) {
	t.Parallel()

	desired := state("desired")
	current := state("current")
	baseline := state("baseline")

	tests := []struct {
		name     string
		input    Input
		want     StateCode
		actions  []Action
		message  string
		noBase   bool
		syncMode SyncMode
	}{
		{
			name:    "unchanged without ledger",
			input:   Input{Context: ContextStatus, Desired: desired, Current: desired},
			want:    StateUnchanged,
			actions: nil,
			message: "matches desired",
		},
		{
			name:    "missing desired before current comparison",
			input:   Input{Context: ContextStatus, Desired: missingState(), Current: current},
			want:    StateMissingDesired,
			actions: []Action{ActionSave, ActionCreate},
			message: "no desired artifact exists",
		},
		{
			name:    "missing current before baseline comparison",
			input:   Input{Context: ContextStatus, Desired: desired, Current: missingState(), LastApplied: &baseline},
			want:    StateMissingCurrent,
			actions: []Action{ActionApply, ActionSkip},
			message: "current live state is missing",
		},
		{
			name:    "command neutral no baseline is unknown with safe choices",
			input:   Input{Context: ContextStatus, Desired: desired, Current: current},
			want:    StateUnknown,
			actions: []Action{ActionDiff, ActionSave, ActionApply},
			message: "no previous sync baseline",
			noBase:  true,
		},
		{
			name:    "save no baseline is changed current",
			input:   Input{Context: ContextSave, Desired: desired, Current: current},
			want:    StateChangedCurrent,
			actions: []Action{ActionSave, ActionApply},
			message: "no previous sync baseline",
			noBase:  true,
		},
		{
			name:    "apply no baseline is ready to apply",
			input:   Input{Context: ContextApply, Desired: desired, Current: current},
			want:    StateReadyToApply,
			actions: []Action{ActionApply},
			message: "no previous sync baseline",
			noBase:  true,
		},
		{
			name:    "changed current when baseline matches desired",
			input:   Input{Context: ContextStatus, Desired: desired, Current: current, LastApplied: &desired},
			want:    StateChangedCurrent,
			actions: []Action{ActionSave, ActionApply},
			message: "baseline matches desired",
		},
		{
			name:    "ready to apply when baseline matches current",
			input:   Input{Context: ContextStatus, Desired: desired, Current: current, LastApplied: &current},
			want:    StateReadyToApply,
			actions: []Action{ActionApply},
			message: "baseline matches current",
		},
		{
			name:     "conflict when current and desired both differ from baseline",
			input:    Input{Context: ContextStatus, Desired: desired, Current: current, LastApplied: &baseline},
			want:     StateConflict,
			actions:  []Action{ActionGuidedSync, ActionDiff},
			message:  "both changed since the last successful baseline",
			syncMode: SyncModeGuidedChoice,
		},
		{
			name:    "blocked safety wins before state comparison",
			input:   Input{Context: ContextStatus, Desired: desired, Current: desired, Blocker: Blocker{Code: BlockerSafety, Message: "selector is unsafe"}},
			want:    StateBlockedSafety,
			actions: []Action{ActionInspect, ActionFix},
			message: "selector is unsafe",
		},
		{
			name:    "blocked lifecycle wins before state comparison",
			input:   Input{Context: ContextStatus, Desired: desired, Current: desired, Blocker: Blocker{Code: BlockerLifecycle, Message: "app is running"}},
			want:    StateBlockedLifecycle,
			actions: []Action{ActionQuit, ActionRetry, ActionSkip},
			message: "app is running",
		},
		{
			name:    "unsupported capability reports unsupported",
			input:   Input{Context: ContextStatus, Desired: desired, Current: desired, Blocker: Blocker{Code: BlockerUnsupported, Message: "driver cannot write"}},
			want:    StateUnsupported,
			actions: []Action{ActionSkip, ActionCreateRecipe},
			message: "driver cannot write",
		},
		{
			name:    "unknown blocker reports unknown",
			input:   Input{Context: ContextStatus, Desired: desired, Current: desired, Blocker: Blocker{Code: BlockerUnknown, Message: "read failed"}},
			want:    StateUnknown,
			actions: []Action{ActionInspect, ActionVerbose},
			message: "read failed",
		},
		{
			name:    "unknown incomplete hash",
			input:   Input{Context: ContextStatus, Desired: NormalizedState{Exists: true}, Current: current},
			want:    StateUnknown,
			actions: []Action{ActionInspect, ActionVerbose},
			message: "incomplete normalized hashes",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := DeriveItem(tc.input)
			require.Equal(t, tc.want, got.State)
			require.Equal(t, tc.actions, got.Actions)
			require.Equal(t, tc.noBase, got.NoBaseline)
			require.False(t, got.AutomaticMerge)
			if tc.syncMode == "" {
				require.Equal(t, SyncModeNone, got.SyncMode)
			} else {
				require.Equal(t, tc.syncMode, got.SyncMode)
			}
			if tc.message != "" {
				require.Contains(t, got.Message, tc.message)
			}
		})
	}
}

func TestMissingCurrentPrecedesBaselineComparison(t *testing.T) {
	t.Parallel()

	desired := state("desired")
	absentBaseline := missingState()
	got := DeriveItem(Input{Context: ContextStatus, Desired: desired, Current: missingState(), LastApplied: &absentBaseline})

	require.Equal(t, StateMissingCurrent, got.State)
	require.False(t, got.NoBaseline)
	require.Equal(t, []Action{ActionApply, ActionSkip}, got.Actions)
}

func TestNeedsRecheckWarningDoesNotReplaceDerivedState(t *testing.T) {
	t.Parallel()

	desired := stateWithVersions("desired", "norm.v2", "driver.v2", "recipe.v2")
	current := stateWithVersions("current", "norm.v2", "driver.v2", "recipe.v2")
	baseline := stateWithVersions("desired", "norm.v1", "driver.v1", "recipe.v1")

	got := DeriveItem(Input{Context: ContextStatus, Desired: desired, Current: current, LastApplied: &baseline})

	require.Equal(t, StateChangedCurrent, got.State)
	require.Len(t, got.Warnings, 1)
	require.Equal(t, WarningNeedsRecheck, got.Warnings[0].Code)
	require.Contains(t, got.Warnings[0].Message, "recompute normalized state")
}

func TestAggregateTargetSeverityOrderIncludesOpaqueChanged(t *testing.T) {
	t.Parallel()

	order := SeverityOrder()
	require.Equal(t, []StateCode{
		StateBlockedSafety,
		StateBlockedLifecycle,
		StateUnsupported,
		StateConflict,
		StateOpaqueChanged,
		StateChangedCurrent,
		StateReadyToApply,
		StateMissingDesired,
		StateMissingCurrent,
		StateUnknown,
		StateUnchanged,
	}, order)

	for i, want := range order {
		items := []Item{{State: StateUnchanged}}
		for _, lowerSeverity := range order[i:] {
			items = append(items, Item{State: lowerSeverity})
		}
		require.Equal(t, want, AggregateTarget(items), "highest severity from suffix %d", i)
	}
}

func TestOpaqueChangedIsEnumAndAggregationOnlyForFileTreeSlice(t *testing.T) {
	t.Parallel()

	got := AggregateTarget([]Item{{State: StateChangedCurrent}, {State: StateOpaqueChanged}})
	require.Equal(t, StateOpaqueChanged, got)

	// #48 covers custom.files file/file-tree resources. App-native opaque
	// artifact derivation is deliberately out of scope, but severity ordering
	// must already reserve the canonical opaque-changed slot for later drivers.
	require.Equal(t, []Action{ActionSave, ActionApply}, actionsFor(StateOpaqueChanged, ContextStatus, false))
}

func TestAdaptersFromFileAndFileTreeDrivers(t *testing.T) {
	t.Parallel()

	fileDriver := filedriver.Driver{}
	fileDesired := FromFileState(fileDriver.Normalize([]byte("desired\n")))
	fileCurrent := FromFileState(fileDriver.Normalize([]byte("current\n")))
	fileBaseline := fileDesired

	fileStatus := DeriveItem(Input{Context: ContextStatus, Desired: fileDesired, Current: fileCurrent, LastApplied: &fileBaseline})
	require.Equal(t, StateChangedCurrent, fileStatus.State)
	require.Equal(t, filedriver.NormalizerID, fileDesired.Normalizer)

	treeDriver := filetreedriver.Driver{}
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "desired", "config.yaml"), "desired\n")
	writeFile(t, filepath.Join(root, "current", "config.yaml"), "current\n")

	treeDesiredRaw, err := treeDriver.ReadCurrent(filetreedriver.Target{LocationID: "fixture", Root: root, RelPath: "desired"})
	require.NoError(t, err)
	treeCurrentRaw, err := treeDriver.ReadCurrent(filetreedriver.Target{LocationID: "fixture", Root: root, RelPath: "current"})
	require.NoError(t, err)
	treeDesired := FromFileTreeState(treeDesiredRaw)
	treeCurrent := FromFileTreeState(treeCurrentRaw)
	treeBaseline := treeCurrent

	treeStatus := DeriveItem(Input{Context: ContextStatus, Desired: treeDesired, Current: treeCurrent, LastApplied: &treeBaseline})
	require.Equal(t, StateReadyToApply, treeStatus.State)
	require.Equal(t, filetreedriver.NormalizerID, treeDesired.Normalizer)
}

func TestConflictStatusIsReportingOnlyNotAutomaticSync(t *testing.T) {
	t.Parallel()

	desired := state("desired")
	current := state("current")
	baseline := state("baseline")
	got := DeriveItem(Input{Context: ContextStatus, Desired: desired, Current: current, LastApplied: &baseline})

	require.Equal(t, StateConflict, got.State)
	require.Equal(t, SyncModeGuidedChoice, got.SyncMode)
	require.False(t, got.AutomaticMerge)
	require.Equal(t, []Action{ActionGuidedSync, ActionDiff}, got.Actions)
}

func state(hash string) NormalizedState {
	return stateWithVersions(hash, "norm.v1", "driver.v1", "recipe.v1")
}

func stateWithVersions(hash string, normalizer string, driverVersion string, recipeVersion string) NormalizedState {
	return NormalizedState{
		Exists:        true,
		Hash:          hash,
		Normalizer:    normalizer,
		DriverVersion: driverVersion,
		RecipeVersion: recipeVersion,
	}
}

func missingState() NormalizedState {
	return NormalizedState{Exists: false, Normalizer: "norm.v1", DriverVersion: "driver.v1", RecipeVersion: "recipe.v1"}
}

func writeFile(t *testing.T, path string, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func TestDefaultMessagesContextsAndDefensiveBranches(t *testing.T) {
	t.Parallel()

	desired := state("same")
	current := state("same")

	defaultMessageCases := []struct {
		name    string
		blocker BlockerCode
		want    StateCode
		message string
	}{
		{name: "safety", blocker: BlockerSafety, want: StateBlockedSafety, message: "Safety, trust"},
		{name: "lifecycle", blocker: BlockerLifecycle, want: StateBlockedLifecycle, message: "lifecycle policy"},
		{name: "unsupported", blocker: BlockerUnsupported, want: StateUnsupported, message: "No trusted recipe"},
		{name: "unknown", blocker: BlockerUnknown, want: StateUnknown, message: "cannot be determined safely"},
	}
	for _, tc := range defaultMessageCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := DeriveItem(Input{Desired: desired, Current: current, Blocker: Blocker{Code: tc.blocker}})
			require.Equal(t, tc.want, got.State)
			require.Contains(t, got.Message, tc.message)
		})
	}

	emptyContext := DeriveItem(Input{Desired: desired, Current: current})
	require.Equal(t, ContextStatus, emptyContext.Context)

	invalidContext := DeriveItem(Input{Context: Context("bogus"), Desired: desired, Current: current})
	require.Equal(t, ContextStatus, invalidContext.Context)

	require.Equal(t, StateUnchanged, AggregateTarget(nil))
	require.Equal(t, StateUnchanged, AggregateTarget([]Item{{State: StateUnchanged}, {State: StateCode("future-state")}}))
	require.Equal(t, []Action{ActionInspect, ActionVerbose}, actionsFor(StateCode("future-state"), ContextStatus, false))
	require.True(t, sameNormalizedContent(missingState(), missingState()))
	require.False(t, sameNormalizedContent(state("a"), missingState()))
}

func TestSafetyAndLifecycleBlockersPrecedeMissingOrIncomparableState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input Input
		want  StateCode
	}{
		{
			name:  "safety before missing desired and current",
			input: Input{Desired: missingState(), Current: missingState(), Blocker: Blocker{Code: BlockerSafety}},
			want:  StateBlockedSafety,
		},
		{
			name:  "safety before incomplete hashes",
			input: Input{Desired: NormalizedState{Exists: true}, Current: NormalizedState{Exists: true}, Blocker: Blocker{Code: BlockerSafety}},
			want:  StateBlockedSafety,
		},
		{
			name:  "lifecycle before missing desired and current",
			input: Input{Desired: missingState(), Current: missingState(), Blocker: Blocker{Code: BlockerLifecycle}},
			want:  StateBlockedLifecycle,
		},
		{
			name:  "lifecycle before incomplete hashes",
			input: Input{Desired: NormalizedState{Exists: true}, Current: NormalizedState{Exists: true}, Blocker: Blocker{Code: BlockerLifecycle}},
			want:  StateBlockedLifecycle,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, DeriveItem(tc.input).State)
		})
	}
}

func TestNeedsRecheckWarningCoversEachVersionDimension(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		desired  NormalizedState
		current  NormalizedState
		baseline NormalizedState
	}{
		{
			name:     "normalizer only",
			desired:  stateWithVersions("desired", "norm.v2", "driver.v1", "recipe.v1"),
			current:  stateWithVersions("current", "norm.v2", "driver.v1", "recipe.v1"),
			baseline: stateWithVersions("desired", "norm.v1", "driver.v1", "recipe.v1"),
		},
		{
			name:     "driver only",
			desired:  stateWithVersions("desired", "norm.v1", "driver.v2", "recipe.v1"),
			current:  stateWithVersions("current", "norm.v1", "driver.v2", "recipe.v1"),
			baseline: stateWithVersions("desired", "norm.v1", "driver.v1", "recipe.v1"),
		},
		{
			name:     "recipe only",
			desired:  stateWithVersions("desired", "norm.v1", "driver.v1", "recipe.v2"),
			current:  stateWithVersions("current", "norm.v1", "driver.v1", "recipe.v2"),
			baseline: stateWithVersions("desired", "norm.v1", "driver.v1", "recipe.v1"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := DeriveItem(Input{Context: ContextStatus, Desired: tc.desired, Current: tc.current, LastApplied: &tc.baseline})
			require.Equal(t, StateChangedCurrent, got.State)
			require.Len(t, got.Warnings, 1)
			require.Equal(t, WarningNeedsRecheck, got.Warnings[0].Code)
		})
	}
}
