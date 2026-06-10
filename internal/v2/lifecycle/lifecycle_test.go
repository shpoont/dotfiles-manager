package lifecycle

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/stretchr/testify/require"
)

func TestEvaluateBeforePolicyMatrix(t *testing.T) {
	t.Parallel()

	rec := lifecycleRecipe(recipe.LifecycleAllowed)
	allowed := EvaluateBefore(context.Background(), Request{Recipe: rec, SettingID: "email", SettingRef: "app:email", ResourceID: "config", Detector: fakeDetector{state: RunningState(StateRunning)}})
	require.False(t, allowed.Blocked)
	require.Empty(t, allowed.Actions)

	rec = lifecycleRecipe(recipe.LifecycleWarn)
	warn := EvaluateBefore(context.Background(), Request{Recipe: rec, SettingID: "email", SettingRef: "app:email", ResourceID: "config", Detector: fakeDetector{state: RunningState(StateRunning)}})
	require.False(t, warn.Blocked)
	require.Len(t, warn.Actions, 1)
	require.Equal(t, ActionWarn, warn.Actions[0].Action)

	rec = lifecycleRecipe(recipe.LifecycleBlocked)
	blocked := EvaluateBefore(context.Background(), Request{Recipe: rec, SettingID: "email", SettingRef: "app:email", ResourceID: "config"})
	require.True(t, blocked.Blocked)
	require.Equal(t, CodePolicyBlocked, blocked.DiagnosticCode)
}

func TestBlockIfRunningDetectsAndFailsClosed(t *testing.T) {
	t.Parallel()

	rec := lifecycleRecipe(recipe.LifecycleBlockIfRunning)
	running := EvaluateBefore(context.Background(), Request{Recipe: rec, SettingID: "email", SettingRef: "app:email", ResourceID: "config", Detector: fakeDetector{state: RunningState(StateRunning), count: 2}})
	require.True(t, running.Blocked)
	require.Equal(t, CodeRunningBlocked, running.DiagnosticCode)
	require.Equal(t, 2, running.Actions[0].ProcessCount)

	notRunning := EvaluateBefore(context.Background(), Request{Recipe: rec, SettingID: "email", SettingRef: "app:email", ResourceID: "config", Detector: fakeDetector{state: RunningState(StateNotRunning)}})
	require.False(t, notRunning.Blocked)

	ambiguous := EvaluateBefore(context.Background(), Request{Recipe: rec, SettingID: "email", SettingRef: "app:email", ResourceID: "config", Detector: fakeDetector{state: RunningState(StateAmbiguous)}})
	require.True(t, ambiguous.Blocked)
	require.Equal(t, CodeDetectAmbiguous, ambiguous.DiagnosticCode)

	dryRun := EvaluateBefore(context.Background(), Request{Recipe: rec, SettingID: "email", SettingRef: "app:email", ResourceID: "config", DryRun: true, Detector: fakeDetector{state: RunningState(StateRunning), count: 1}})
	require.True(t, dryRun.Blocked)
	require.Equal(t, ActionDetect, dryRun.Actions[0].Action)
	require.Equal(t, ModeExecuted, dryRun.Actions[0].Mode)
	require.Equal(t, ActionBlock, dryRun.Actions[1].Action)
	require.Equal(t, ModePlanned, dryRun.Actions[1].Mode)
}

func TestAskToQuitManualPromptAndNonInteractiveBehavior(t *testing.T) {
	t.Parallel()

	rec := lifecycleRecipe(recipe.LifecycleAskToQuit)
	nonInteractive := EvaluateBefore(context.Background(), Request{Recipe: rec, SettingID: "email", SettingRef: "app:email", ResourceID: "config", NonInteractive: true, Detector: fakeDetector{state: RunningState(StateRunning)}})
	require.True(t, nonInteractive.Blocked)
	require.Equal(t, CodeConfirmationRequired, nonInteractive.DiagnosticCode)

	prompter := &countingPrompter{accepted: true}
	confirmed := EvaluateBefore(context.Background(), Request{Recipe: rec, SettingID: "email", SettingRef: "app:email", ResourceID: "config", Confirmed: true, Detector: fakeDetector{state: RunningState(StateRunning)}, Prompter: prompter})
	require.True(t, confirmed.Blocked)
	require.Equal(t, CodeConfirmationRequired, confirmed.DiagnosticCode)
	require.Equal(t, 0, prompter.calls)

	declined := EvaluateBefore(context.Background(), Request{Recipe: rec, SettingID: "email", SettingRef: "app:email", ResourceID: "config", Detector: &sequenceDetector{states: []RunningState{RunningState(StateRunning)}}, Prompter: fakePrompter{accepted: false}})
	require.True(t, declined.Blocked)
	require.Equal(t, CodeUserDeclined, declined.DiagnosticCode)

	accepted := EvaluateBefore(context.Background(), Request{Recipe: rec, SettingID: "email", SettingRef: "app:email", ResourceID: "config", Detector: &sequenceDetector{states: []RunningState{RunningState(StateRunning), RunningState(StateNotRunning)}}, Prompter: fakePrompter{accepted: true}})
	require.False(t, accepted.Blocked)
	require.False(t, accepted.ManagerStopped)
	require.Equal(t, ActionRecheck, accepted.Actions[len(accepted.Actions)-1].Action)
}

func TestQuitAndReopenLifecycle(t *testing.T) {
	t.Parallel()

	rec := lifecycleRecipe(recipe.LifecycleReopenIfStoppedByTool)
	withoutYes := EvaluateBefore(context.Background(), Request{Recipe: rec, SettingID: "email", SettingRef: "app:email", ResourceID: "config", Detector: fakeDetector{state: RunningState(StateRunning)}})
	require.True(t, withoutYes.Blocked)
	require.Equal(t, CodeConfirmationRequired, withoutYes.DiagnosticCode)

	quitFailed := EvaluateBefore(context.Background(), Request{Recipe: rec, SettingID: "email", SettingRef: "app:email", ResourceID: "config", Confirmed: true, Detector: fakeDetector{state: RunningState(StateRunning)}, Controller: fakeController{quitErr: errors.New("no")}})
	require.True(t, quitFailed.Blocked)
	require.Equal(t, CodeQuitFailed, quitFailed.DiagnosticCode)

	stillRunning := EvaluateBefore(context.Background(), Request{Recipe: rec, SettingID: "email", SettingRef: "app:email", ResourceID: "config", Confirmed: true, Detector: &sequenceDetector{states: []RunningState{RunningState(StateRunning), RunningState(StateRunning)}}, Controller: fakeController{}})
	require.True(t, stillRunning.Blocked)
	require.Equal(t, CodeStillRunning, stillRunning.DiagnosticCode)

	stopped := EvaluateBefore(context.Background(), Request{Recipe: rec, SettingID: "email", SettingRef: "app:email", ResourceID: "config", Confirmed: true, Detector: &sequenceDetector{states: []RunningState{RunningState(StateRunning), RunningState(StateNotRunning)}}, Controller: fakeController{}})
	require.False(t, stopped.Blocked)
	require.True(t, stopped.ManagerStopped)

	reopenFailed := EvaluateAfter(context.Background(), Request{Recipe: rec, SettingID: "email", SettingRef: "app:email", ResourceID: "config", Confirmed: true, Controller: fakeController{reopenErr: errors.New("no")}}, true)
	require.True(t, reopenFailed.Blocked)
	require.Equal(t, CodeReopenFailed, reopenFailed.DiagnosticCode)
	require.True(t, reopenFailed.Actions[0].ReopenAttempted)

	reopened := EvaluateAfter(context.Background(), Request{Recipe: rec, SettingID: "email", SettingRef: "app:email", ResourceID: "config", Confirmed: true, Controller: fakeController{}}, true)
	require.False(t, reopened.Blocked)
	require.Equal(t, ActionReopen, reopened.Actions[0].Action)
}

func TestNativeOperationPolicyOverridesSettingAndUsesBoundedControllerContext(t *testing.T) {
	t.Parallel()

	rec := lifecycleRecipe(recipe.LifecycleAllowed)
	rec.NativeOperations = map[string]recipe.NativeOperation{
		"import": {Lifecycle: recipe.LifecycleBlockIfRunning, LifecycleTarget: "primary"},
	}
	blocked := EvaluateBefore(context.Background(), Request{Recipe: rec, SettingID: "email", SettingRef: "app:email", ResourceID: "config", NativeOperationID: "import", Detector: fakeDetector{state: RunningState(StateRunning)}})
	require.True(t, blocked.Blocked)
	require.Equal(t, CodeRunningBlocked, blocked.DiagnosticCode)
	require.Equal(t, "import", blocked.Policy.NativeOperationID)
	require.Equal(t, "import", blocked.Actions[0].NativeOperationID)

	rec.NativeOperations["import"] = recipe.NativeOperation{Lifecycle: recipe.LifecycleQuitIfRunning, LifecycleTarget: "primary"}
	controller := &deadlineController{}
	stopped := EvaluateBefore(context.Background(), Request{
		Recipe:            rec,
		SettingID:         "email",
		SettingRef:        "app:email",
		ResourceID:        "config",
		NativeOperationID: "import",
		Confirmed:         true,
		ActionTimeout:     time.Second,
		Detector:          &sequenceDetector{states: []RunningState{RunningState(StateRunning), RunningState(StateNotRunning)}},
		Controller:        controller,
	})
	require.False(t, stopped.Blocked)
	require.True(t, controller.quitDeadline)
}

func TestAfterWriteAndRecheckFailureBranches(t *testing.T) {
	t.Parallel()

	rec := lifecycleRecipe(recipe.LifecycleReopenIfStoppedByTool)
	skipped := EvaluateAfter(context.Background(), Request{Recipe: rec, SettingID: "email", SettingRef: "app:email", ResourceID: "config"}, false)
	require.False(t, skipped.Blocked)
	require.Empty(t, skipped.Actions)

	missingTarget := EvaluateAfter(context.Background(), Request{Recipe: &recipe.Recipe{
		Settings:  map[string]recipe.Setting{"email": {Resource: "config"}},
		Resources: map[string]recipe.Resource{"config": {Lifecycle: recipe.LifecycleReopenIfStoppedByTool, LifecycleTarget: "missing"}},
	}, SettingID: "email", SettingRef: "app:email", ResourceID: "config"}, true)
	require.True(t, missingTarget.Blocked)
	require.Equal(t, CodeTargetMissing, missingTarget.DiagnosticCode)

	unsupportedReopenRecipe := lifecycleRecipe(recipe.LifecycleReopenIfStoppedByTool)
	target := unsupportedReopenRecipe.LifecycleTargets["primary"]
	target.Reopen.Kind = recipe.LifecycleReopenNone
	unsupportedReopenRecipe.LifecycleTargets["primary"] = target
	unsupported := EvaluateAfter(context.Background(), Request{Recipe: unsupportedReopenRecipe, SettingID: "email", SettingRef: "app:email", ResourceID: "config", Controller: fakeController{}}, true)
	require.True(t, unsupported.Blocked)
	require.Equal(t, CodeReopenUnsupported, unsupported.DiagnosticCode)

	recheckFailed := EvaluateBefore(context.Background(), Request{
		Recipe:     lifecycleRecipe(recipe.LifecycleQuitIfRunning),
		SettingID:  "email",
		SettingRef: "app:email",
		ResourceID: "config",
		Confirmed:  true,
		Detector:   &sequenceDetector{states: []RunningState{RunningState(StateRunning)}, errs: map[int]error{1: errors.New("recheck failed")}},
		Controller: fakeController{},
	})
	require.True(t, recheckFailed.Blocked)
	require.Equal(t, CodeDetectFailed, recheckFailed.DiagnosticCode)
}

func TestLifecycleHelperAndFailureBranches(t *testing.T) {
	t.Parallel()

	missing := EvaluateBefore(context.Background(), Request{Recipe: &recipe.Recipe{Resources: map[string]recipe.Resource{"config": {Lifecycle: recipe.LifecycleBlockIfRunning, LifecycleTarget: "missing"}}}, SettingRef: "app:email", ResourceID: "config"})
	require.True(t, missing.Blocked)
	require.Equal(t, CodeTargetMissing, missing.DiagnosticCode)

	unsupported := EvaluateBefore(context.Background(), Request{Recipe: &recipe.Recipe{
		LifecycleTargets: map[string]recipe.LifecycleTarget{"primary": {Detect: recipe.LifecycleDetectPolicy{Kind: recipe.LifecycleDetectProcessName, Names: []string{"Example App"}}}},
		Resources:        map[string]recipe.Resource{"config": {Lifecycle: "future-lifecycle", LifecycleTarget: "primary"}},
	}, SettingRef: "app:email", ResourceID: "config", Detector: fakeDetector{state: RunningState(StateRunning)}})
	require.True(t, unsupported.Blocked)
	require.Equal(t, CodeTargetUnsupported, unsupported.DiagnosticCode)

	detectFailed := EvaluateBefore(context.Background(), Request{Recipe: lifecycleRecipe(recipe.LifecycleBlockIfRunning), SettingID: "email", SettingRef: "app:email", ResourceID: "config", Detector: fakeDetector{err: errors.New("ps failed")}})
	require.True(t, detectFailed.Blocked)
	require.Equal(t, CodeDetectFailed, detectFailed.DiagnosticCode)

	diagnostics := RecordsToDiagnostics([]ActionRecord{{Code: "safe", Message: "message"}, {Message: "ignored"}})
	require.Equal(t, []Diagnostic{{Code: "safe", Message: "message"}}, diagnostics)

	records := SortRecords([]ActionRecord{
		{SettingRef: "b", Phase: PhaseAfterWrite, Action: ActionReopen},
		{SettingRef: "a", Phase: PhaseBeforeWrite, Action: ActionDetect},
	})
	require.Equal(t, "a", records[0].SettingRef)
	require.Equal(t, "app", targetRef("app:email"))
	require.Equal(t, "", targetRef("email"))
	require.Equal(t, ModePlanned, mode(Request{DryRun: true}))

	_, ok := targetFor(nil, "missing")
	require.False(t, ok)
	_, ok = targetFor(&recipe.Recipe{}, "")
	require.False(t, ok)
	require.Equal(t, "primary", displayName("primary", recipe.LifecycleTarget{}))
}

func TestDefaultDetectorControllerAndPrompterBranches(t *testing.T) {
	t.Parallel()

	detector := ProcessNameDetector{}
	_, err := detector.Detect(context.Background(), recipe.LifecycleTarget{Detect: recipe.LifecycleDetectPolicy{Kind: "shell", Names: []string{"App"}}})
	require.Error(t, err)
	_, err = detector.Detect(context.Background(), recipe.LifecycleTarget{Detect: recipe.LifecycleDetectPolicy{Kind: recipe.LifecycleDetectProcessName}})
	require.Error(t, err)

	controller := UnsupportedController{}
	require.Error(t, controller.Quit(context.Background(), recipe.LifecycleTarget{}))
	require.Error(t, controller.Reopen(context.Background(), recipe.LifecycleTarget{}))

	var out bytes.Buffer
	prompter := TextPrompter{In: strings.NewReader("yes\n"), Out: &out}
	accepted, err := prompter.Prompt(context.Background(), Prompt{TargetID: "primary", DisplayName: "Example App", Message: "Quit it."})
	require.NoError(t, err)
	require.True(t, accepted)
	require.Contains(t, out.String(), "Example App")

	var declinedOut bytes.Buffer
	declined, err := (TextPrompter{In: strings.NewReader("no\n"), Out: &declinedOut}).Prompt(context.Background(), Prompt{TargetID: "primary", Message: "Quit it."})
	require.NoError(t, err)
	require.False(t, declined)
	require.Contains(t, declinedOut.String(), "primary")

	_, err = (TextPrompter{}).Prompt(context.Background(), Prompt{})
	require.Error(t, err)

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = (TextPrompter{In: strings.NewReader("yes\n"), Out: &bytes.Buffer{}}).Prompt(cancelled, Prompt{})
	require.Error(t, err)
}

func TestProcessNameDetectorDetectsCurrentProcessAndMissingProcess(t *testing.T) {
	t.Parallel()

	executable, err := os.Executable()
	require.NoError(t, err)
	name := filepath.Base(executable)
	detector := ProcessNameDetector{}

	running, err := detector.Detect(context.Background(), recipe.LifecycleTarget{Detect: recipe.LifecycleDetectPolicy{Kind: recipe.LifecycleDetectProcessName, Names: []string{name}}})
	require.NoError(t, err)
	require.Equal(t, RunningState(StateRunning), running.State)
	require.GreaterOrEqual(t, running.Count, 1)

	missing, err := detector.Detect(context.Background(), recipe.LifecycleTarget{Detect: recipe.LifecycleDetectPolicy{Kind: recipe.LifecycleDetectProcessName, Names: []string{"dotfiles-manager-definitely-not-running-process"}}})
	require.NoError(t, err)
	require.Equal(t, RunningState(StateNotRunning), missing.State)
}

func TestProcessNameDetectorUsesAbsolutePSAndTimesOut(t *testing.T) {
	fakeDir := t.TempDir()
	fakePS := filepath.Join(fakeDir, "ps")
	require.NoError(t, os.WriteFile(fakePS, []byte("#!/bin/sh\nexit 42\n"), 0o755))
	t.Setenv("PATH", fakeDir)

	detector := ProcessNameDetector{}
	missing, err := detector.Detect(context.Background(), recipe.LifecycleTarget{Detect: recipe.LifecycleDetectPolicy{Kind: recipe.LifecycleDetectProcessName, Names: []string{"dotfiles-manager-definitely-not-running-process"}}})
	require.NoError(t, err)
	require.Equal(t, RunningState(StateNotRunning), missing.State)

	slowPS := filepath.Join(fakeDir, "slow-ps")
	require.NoError(t, os.WriteFile(slowPS, []byte("#!/bin/sh\nsleep 1\nprintf '%s\\n' slow-ps\n"), 0o755))
	timedOut, err := (ProcessNameDetector{PSPath: slowPS, Timeout: time.Millisecond}).Detect(context.Background(), recipe.LifecycleTarget{Detect: recipe.LifecycleDetectPolicy{Kind: recipe.LifecycleDetectProcessName, Names: []string{"slow-ps"}}})
	require.Error(t, err)
	require.Equal(t, RunningState(StateUnknown), timedOut.State)

	_, err = (ProcessNameDetector{PSPath: "ps"}).Detect(context.Background(), recipe.LifecycleTarget{Detect: recipe.LifecycleDetectPolicy{Kind: recipe.LifecycleDetectProcessName, Names: []string{"ps"}}})
	require.Error(t, err)
}

func lifecycleRecipe(lifecycleValue string) *recipe.Recipe {
	return &recipe.Recipe{
		Target: "app",
		LifecycleTargets: map[string]recipe.LifecycleTarget{
			"primary": {
				DisplayName: "Example App",
				Detect:      recipe.LifecycleDetectPolicy{Kind: recipe.LifecycleDetectProcessName, Names: []string{"Example App"}},
				Quit:        recipe.LifecycleControlPolicy{Kind: recipe.LifecycleControlManaged},
				Reopen:      recipe.LifecycleControlPolicy{Kind: recipe.LifecycleControlManaged},
			},
		},
		Settings:  map[string]recipe.Setting{"email": {Resource: "config"}},
		Resources: map[string]recipe.Resource{"config": {Lifecycle: lifecycleValue, LifecycleTarget: "primary"}},
	}
}

type fakeDetector struct {
	state RunningState
	count int
	err   error
}

func (d fakeDetector) Detect(context.Context, recipe.LifecycleTarget) (DetectionResult, error) {
	if d.err != nil {
		return DetectionResult{State: RunningState(StateUnknown)}, d.err
	}
	return DetectionResult{State: d.state, Count: d.count}, nil
}

type sequenceDetector struct {
	states []RunningState
	errs   map[int]error
	idx    int
}

func (d *sequenceDetector) Detect(context.Context, recipe.LifecycleTarget) (DetectionResult, error) {
	if len(d.states) == 0 {
		return DetectionResult{State: RunningState(StateUnknown)}, nil
	}
	call := d.idx
	idx := call
	if idx >= len(d.states) {
		idx = len(d.states) - 1
	}
	d.idx++
	if d.errs != nil && d.errs[call] != nil {
		return DetectionResult{State: RunningState(StateUnknown)}, d.errs[call]
	}
	return DetectionResult{State: d.states[idx]}, nil
}

type fakePrompter struct{ accepted bool }

func (p fakePrompter) Prompt(context.Context, Prompt) (bool, error) { return p.accepted, nil }

type countingPrompter struct {
	accepted bool
	calls    int
}

func (p *countingPrompter) Prompt(context.Context, Prompt) (bool, error) {
	p.calls++
	return p.accepted, nil
}

type fakeController struct {
	quitErr   error
	reopenErr error
}

func (c fakeController) Quit(context.Context, recipe.LifecycleTarget) error   { return c.quitErr }
func (c fakeController) Reopen(context.Context, recipe.LifecycleTarget) error { return c.reopenErr }

type deadlineController struct{ quitDeadline bool }

func (c *deadlineController) Quit(ctx context.Context, _ recipe.LifecycleTarget) error {
	_, c.quitDeadline = ctx.Deadline()
	return nil
}

func (c *deadlineController) Reopen(context.Context, recipe.LifecycleTarget) error { return nil }
