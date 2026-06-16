package guidedsync

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/shpoont/dotfiles-manager/internal/v2/selectedlive"
	"github.com/shpoont/dotfiles-manager/internal/v2/selectedpreview"
	v2status "github.com/shpoont/dotfiles-manager/internal/v2/status"
	"github.com/stretchr/testify/require"
)

func TestPlanExposesConflictNoBaselineAndBlockedItems(t *testing.T) {
	report, err := Run(Options{NonInteractive: true, PreviewBuilder: fakePreviewBuilder(
		previewItem("app:conflict", v2status.StateConflict),
		previewItem("app:unknown", v2status.StateUnknown, func(item *selectedpreview.Item) { item.NoBaseline = true }),
		previewItem("app:blocked", v2status.StateUnsupported),
	)})
	require.NoError(t, err)
	require.Equal(t, SummaryNeedsChoice, report.Summary.Status)
	require.Equal(t, 2, report.Summary.NeedsChoice)
	require.Equal(t, 1, report.Summary.Blocked)
	require.Equal(t, []string{ChoiceSave, ChoiceApply, ChoiceSkip}, report.Items[0].AllowedChoices)
	require.Empty(t, report.Items[0].Recommended)
	require.Equal(t, []string{ChoiceSave, ChoiceApply, ChoiceSkip}, report.Items[1].AllowedChoices)
	require.Empty(t, report.Items[2].AllowedChoices)
	require.Equal(t, OutcomeBlocked, report.Items[2].Outcome)
}

func TestFlagChoiceValidationFailsBeforeExecution(t *testing.T) {
	items := []selectedpreview.Item{
		previewItem("app:email", v2status.StateChangedCurrent),
		previewItem("app:blocked", v2status.StateUnsupported),
	}

	cases := []struct {
		name    string
		choices []Choice
		code    string
	}{
		{name: "unknown ref", choices: []Choice{{Ref: "app:missing", Action: ChoiceSave}}, code: CodeChoiceUnknownRef},
		{name: "duplicate ref", choices: []Choice{{Ref: "app:email", Action: ChoiceSave}, {Ref: "app:email", Action: ChoiceSkip}}, code: CodeChoiceDuplicate},
		{name: "blocked item", choices: []Choice{{Ref: "app:blocked", Action: ChoiceSkip}}, code: CodeChoiceNotAllowed},
		{name: "not allowed for state", choices: []Choice{{Ref: "app:email", Action: "delete"}}, code: CodeChoiceInvalid},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			report, err := Run(Options{Confirmed: true, Choices: tc.choices, PreviewBuilder: fakePreviewBuilder(items...), LiveRunner: func(selectedlive.Options) (*selectedlive.Result, error) {
				calls++
				return nil, nil
			}})
			require.Error(t, err)
			require.Equal(t, 0, calls)
			require.Equal(t, SummaryError, report.Summary.Status)
			require.Equal(t, tc.code, report.Error.Code)
		})
	}
}

func TestInteractivePromptCollectsAllChoicesBeforeAnyWrite(t *testing.T) {
	calls := 0
	report, err := Run(Options{
		In:             strings.NewReader("save\n"),
		PromptOut:      &strings.Builder{},
		PreviewBuilder: fakePreviewBuilder(previewItem("app:email", v2status.StateChangedCurrent), previewItem("app:name", v2status.StateReadyToApply)),
		LiveRunner: func(selectedlive.Options) (*selectedlive.Result, error) {
			calls++
			return nil, nil
		},
	})
	require.Error(t, err)
	require.Equal(t, 0, calls)
	require.Equal(t, SummaryError, report.Summary.Status)
	require.Equal(t, CodePromptRequired, report.Error.Code)
}

func TestInteractivePromptMutatesChosenItemsWithoutYes(t *testing.T) {
	calls := []selectedlive.Options{}
	promptOut := &strings.Builder{}
	report, err := Run(Options{
		In:             strings.NewReader("save\nskip\n"),
		PromptOut:      promptOut,
		PreviewBuilder: fakePreviewBuilder(previewItem("app:email", v2status.StateChangedCurrent), previewItem("app:name", v2status.StateReadyToApply)),
		LiveRunner: func(opts selectedlive.Options) (*selectedlive.Result, error) {
			calls = append(calls, opts)
			return fakeLiveResult(opts.Ref, opts.Command), nil
		},
	})
	require.NoError(t, err)
	require.Len(t, calls, 1)
	require.Equal(t, ChoiceSave, calls[0].Command)
	require.True(t, calls[0].Confirmed)
	require.True(t, calls[0].NonInteractive)
	require.Contains(t, promptOut.String(), "Choosing save/apply will mutate this item")
	require.Equal(t, OutcomeExecuted, report.Items[0].Outcome)
	require.Equal(t, OutcomeSkipped, report.Items[1].Outcome)
}

func TestExecutionStopsOnFirstFailureAndReportsNotAttempted(t *testing.T) {
	calls := []string{}
	report, err := Run(Options{
		Confirmed: true,
		Choices: []Choice{
			{Ref: "app:email", Action: ChoiceSave},
			{Ref: "app:name", Action: ChoiceApply},
			{Ref: "app:theme", Action: ChoiceSave},
		},
		PreviewBuilder: fakePreviewBuilder(
			previewItem("app:email", v2status.StateChangedCurrent),
			previewItem("app:name", v2status.StateReadyToApply),
			previewItem("app:theme", v2status.StateChangedCurrent),
		),
		LiveRunner: func(opts selectedlive.Options) (*selectedlive.Result, error) {
			calls = append(calls, opts.Ref+"="+opts.Command)
			if opts.Ref == "app:name" {
				return fakeLiveResult(opts.Ref, opts.Command), errors.New("apply failed")
			}
			return fakeLiveResult(opts.Ref, opts.Command), nil
		},
	})
	require.Error(t, err)
	require.Equal(t, []string{"app:email=save", "app:name=apply"}, calls)
	require.Equal(t, SummaryPartial, report.Summary.Status)
	require.Equal(t, OutcomeExecuted, report.Items[0].Outcome)
	require.Equal(t, OutcomeFailed, report.Items[1].Outcome)
	require.Equal(t, OutcomeNotAttempted, report.Items[2].Outcome)
	require.Equal(t, 6, err.(interface{ ExitCode() int }).ExitCode())
}

func TestCLIChoiceRequiresYesBeforeMutation(t *testing.T) {
	report, err := Run(Options{
		Choices:        []Choice{{Ref: "app:email", Action: ChoiceSave}},
		PreviewBuilder: fakePreviewBuilder(previewItem("app:email", v2status.StateChangedCurrent)),
		LiveRunner: func(selectedlive.Options) (*selectedlive.Result, error) {
			t.Fatal("live runner must not be called without --yes")
			return nil, nil
		},
	})
	require.Error(t, err)
	require.Equal(t, SummaryError, report.Summary.Status)
	require.Equal(t, CodeConfirmationRequired, report.Error.Code)
}

func fakePreviewBuilder(items ...selectedpreview.Item) PreviewBuilder {
	return func(selectedpreview.Options) (*selectedpreview.Report, error) {
		return &selectedpreview.Report{Schema: selectedpreview.Schema, SchemaVersion: selectedpreview.SchemaVersion, Command: selectedpreview.CommandStatus, RunID: selectedpreview.RunID, ProfileStack: []string{"global"}, Summary: selectedpreview.Summary{Status: selectedpreview.SummaryChanged, Changed: len(items)}, Items: append([]selectedpreview.Item(nil), items...)}, nil
	}
}

func previewItem(ref string, state v2status.StateCode, mutate ...func(*selectedpreview.Item)) selectedpreview.Item {
	parts := strings.SplitN(ref, ":", 2)
	item := selectedpreview.Item{TargetRef: parts[0], SettingRef: ref, Scope: "user", Subject: "leon", State: state, Message: "state message", Resource: selectedpreview.ResourceInfo{ID: "config", DriverID: "yaml", RelPath: "config.yaml"}, Selector: selectedpreview.SelectorInfo{Kind: "selected-path", Summary: "user.email"}}
	for _, fn := range mutate {
		fn(&item)
	}
	return item
}

func fakeLiveResult(ref string, command string) *selectedlive.Result {
	return &selectedlive.Result{Report: &selectedpreview.Report{Schema: selectedpreview.Schema, SchemaVersion: selectedpreview.SchemaVersion, Command: command, RunID: command + "-run", Summary: selectedpreview.Summary{Status: selectedpreview.SummaryChanged}, Items: []selectedpreview.Item{{SettingRef: ref, State: v2status.StateChangedCurrent, Mutation: &selectedpreview.MutationInfo{Result: "succeeded", RunID: command + "-run", BackupRefs: []string{"backup://" + ref}}}}}}
}

func TestParseChoiceAndRenderers(t *testing.T) {
	choice, err := ParseChoice(" app:email = SAVE ")
	require.NoError(t, err)
	require.Equal(t, Choice{Ref: "app:email", Action: ChoiceSave}, choice)

	for _, raw := range []string{"", "app:email", "=save", "app:email=delete"} {
		_, err = ParseChoice(raw)
		require.Error(t, err)
		require.Equal(t, CodeChoiceInvalid, err.(*Error).Code)
	}

	report := ErrorReport("code", "message", map[string]any{"x": "y"})
	require.Equal(t, SummaryError, report.Summary.Status)
	jsonBody, err := JSON(report)
	require.NoError(t, err)
	require.Contains(t, jsonBody, "dotfiles-manager.v2.guided-sync")
	require.Contains(t, JSONString(t, nil), "summary")

	text := Text(&Report{Command: Command, Summary: Summary{Status: SummaryOK}, Items: []Item{{SettingRef: "app:email", Scope: "user", Subject: "leon", State: v2status.StateChangedCurrent, Outcome: OutcomeChosen, Recommended: ChoiceSave, SelectedChoice: ChoiceSave, ChoiceSource: ChoiceSourceFlag, NoBaseline: true, AllowedChoices: []string{ChoiceSave, ChoiceSkip}, Message: "hello", Resource: selectedpreview.ResourceInfo{ID: "config", DriverID: "yaml"}, Selector: selectedpreview.SelectorInfo{Summary: "user.email"}, Diff: &selectedpreview.DiffInfo{Kind: "changed", Mode: "metadata-only", Redaction: "redacted"}, UnderlyingRunID: "run", BackupRefs: []string{"backup"}, Diagnostics: []selectedpreview.Diagnostic{{Code: "diag", Severity: selectedpreview.SeverityWarning, Message: "warn"}}}}})
	require.Contains(t, text, "choice=save(flag)")
	require.Contains(t, text, "diff=changed mode=metadata-only")
	require.Contains(t, Text(nil), "status=error")

	require.Empty(t, (*Error)(nil).Error())
	require.Equal(t, 1, (*Error)(nil).ExitCode())
	require.Equal(t, 1, (&Error{}).ExitCode())
	require.Equal(t, 4, (&Error{Exit: 4}).ExitCode())
}

func TestAllowedChoicesAndRecommendationsByState(t *testing.T) {
	cases := []struct {
		state       v2status.StateCode
		noBaseline  bool
		recommended string
		choices     []string
		outcome     string
	}{
		{state: v2status.StateUnchanged, recommended: ChoiceSkip, choices: []string{ChoiceSkip}, outcome: OutcomePlanned},
		{state: v2status.StateChangedCurrent, recommended: ChoiceSave, choices: []string{ChoiceSave, ChoiceApply, ChoiceSkip}, outcome: OutcomePlanned},
		{state: v2status.StateReadyToApply, recommended: ChoiceApply, choices: []string{ChoiceApply, ChoiceSkip}, outcome: OutcomePlanned},
		{state: v2status.StateMissingDesired, recommended: ChoiceSave, choices: []string{ChoiceSave, ChoiceSkip}, outcome: OutcomePlanned},
		{state: v2status.StateMissingCurrent, recommended: ChoiceApply, choices: []string{ChoiceApply, ChoiceSkip}, outcome: OutcomePlanned},
		{state: v2status.StateConflict, choices: []string{ChoiceSave, ChoiceApply, ChoiceSkip}, outcome: OutcomePlanned},
		{state: v2status.StateOpaqueChanged, choices: []string{ChoiceSave, ChoiceApply, ChoiceSkip}, outcome: OutcomePlanned},
		{state: v2status.StateUnknown, noBaseline: true, choices: []string{ChoiceSave, ChoiceApply, ChoiceSkip}, outcome: OutcomePlanned},
		{state: v2status.StateUnknown, choices: []string{}, outcome: OutcomePlanned},
		{state: v2status.StateBlockedSafety, choices: []string{}, outcome: OutcomeBlocked},
		{state: v2status.StateBlockedLifecycle, choices: []string{}, outcome: OutcomeBlocked},
	}
	for _, tc := range cases {
		item := fromPreviewItem(previewItem("app:setting", tc.state, func(item *selectedpreview.Item) { item.NoBaseline = tc.noBaseline }))
		require.Equal(t, tc.recommended, item.Recommended, tc.state)
		require.Equal(t, tc.choices, item.AllowedChoices, tc.state)
		require.Equal(t, tc.outcome, item.Outcome, tc.state)
	}
}

func TestFileTreeApplyChoiceDeferredUntilRemovalPreviewIsExposed(t *testing.T) {
	fileTreeItem := previewItem("app:config", v2status.StateReadyToApply, func(item *selectedpreview.Item) {
		item.Resource = selectedpreview.ResourceInfo{ID: "config-tree", DriverID: recipe.FileTreeDriverID, RelPath: "artifacts/config"}
	})

	report, err := Run(Options{NonInteractive: true, PreviewBuilder: fakePreviewBuilder(fileTreeItem)})
	require.NoError(t, err)
	require.Equal(t, SummaryNeedsChoice, report.Summary.Status)
	require.Equal(t, []string{ChoiceSkip}, report.Items[0].AllowedChoices)
	require.Empty(t, report.Items[0].Recommended)
	require.Contains(t, report.Items[0].Message, "File-tree apply is deferred in sync")
	requireCLIPackageDiagnosticCode(t, report.Items[0].Diagnostics, CodeFileTreeApplyDeferred)

	calls := 0
	report, err = Run(Options{
		Confirmed:      true,
		Choices:        []Choice{{Ref: "app:config", Action: ChoiceApply}},
		PreviewBuilder: fakePreviewBuilder(fileTreeItem),
		LiveRunner: func(selectedlive.Options) (*selectedlive.Result, error) {
			calls++
			return nil, nil
		},
	})
	require.Error(t, err)
	require.Equal(t, 0, calls)
	require.Equal(t, SummaryError, report.Summary.Status)
	require.Equal(t, CodeChoiceNotAllowed, report.Error.Code)
	require.Equal(t, []string{ChoiceSkip}, report.Error.Details["allowedChoices"])
}

func TestConfirmedExecutionRequiresChoicesBeforeWrites(t *testing.T) {
	calls := 0
	report, err := Run(Options{
		Confirmed:      true,
		PreviewBuilder: fakePreviewBuilder(previewItem("app:email", v2status.StateChangedCurrent)),
		LiveRunner: func(selectedlive.Options) (*selectedlive.Result, error) {
			calls++
			return nil, nil
		},
	})
	require.Error(t, err)
	require.Equal(t, 0, calls)
	require.Equal(t, CodeChoiceRequired, report.Error.Code)
}

func TestSkipChoiceIsRecordedWithoutYes(t *testing.T) {
	report, err := Run(Options{
		Choices:        []Choice{{Ref: "app:email", Action: ChoiceSkip}},
		PreviewBuilder: fakePreviewBuilder(previewItem("app:email", v2status.StateChangedCurrent)),
	})
	require.NoError(t, err)
	require.Equal(t, OutcomeSkipped, report.Items[0].Outcome)
	require.Equal(t, SummaryChanged, report.Summary.Status)
}

func TestBuildPlanErrorPreservesPreviewError(t *testing.T) {
	previewErr := &selectedpreview.Error{Code: "selectedpreview.ref.notFound", Message: "missing", Exit: 2, Details: map[string]any{"ref": "x"}}
	report, err := Run(Options{PreviewBuilder: func(selectedpreview.Options) (*selectedpreview.Report, error) {
		return &selectedpreview.Report{Error: &selectedpreview.ErrorObj{Code: previewErr.Code, Message: previewErr.Message, Details: previewErr.Details}}, previewErr
	}})
	require.Error(t, err)
	require.Equal(t, SummaryError, report.Summary.Status)
	require.Equal(t, "selectedpreview.ref.notFound", report.Error.Code)
	require.Equal(t, "x", report.Error.Details["ref"])
}

func TestExecutionMergesLiveReportErrorDiagnosticAndRunID(t *testing.T) {
	now := func() time.Time { return time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC) }
	report, err := Run(Options{
		Confirmed:      true,
		Now:            now,
		Choices:        []Choice{{Ref: "app:email", Action: ChoiceSave}},
		PreviewBuilder: fakePreviewBuilder(previewItem("app:email", v2status.StateChangedCurrent)),
		LiveRunner: func(opts selectedlive.Options) (*selectedlive.Result, error) {
			require.Equal(t, "guided-sync-20260610T120000Z-app-email-save", opts.RunID)
			return &selectedlive.Result{Report: &selectedpreview.Report{Error: &selectedpreview.ErrorObj{Code: "live.failed", Message: "live failed"}}}, errors.New("live failed")
		},
	})
	require.Error(t, err)
	require.Equal(t, SummaryError, report.Summary.Status)
	require.Equal(t, OutcomeFailed, report.Items[0].Outcome)
	requireCLIPackageDiagnosticCode(t, report.Items[0].Diagnostics, "live.failed")
}

func JSONString(t *testing.T, report *Report) string {
	t.Helper()
	body, err := JSON(report)
	require.NoError(t, err)
	return body
}

func requireCLIPackageDiagnosticCode(t *testing.T, diagnostics []selectedpreview.Diagnostic, code string) {
	t.Helper()
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return
		}
	}
	require.Failf(t, "missing diagnostic", "wanted %s in %#v", code, diagnostics)
}
