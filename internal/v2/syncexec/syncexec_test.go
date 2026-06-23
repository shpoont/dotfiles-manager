package syncexec

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shpoont/dotfiles-manager/internal/v2/desired"
	v2preview "github.com/shpoont/dotfiles-manager/internal/v2/preview"
	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/shpoont/dotfiles-manager/internal/v2/selectedlive"
	"github.com/shpoont/dotfiles-manager/internal/v2/selectedpreview"
	v2status "github.com/shpoont/dotfiles-manager/internal/v2/status"
	"github.com/stretchr/testify/require"
)

func TestRunExecutesSafeWritesInBothDirections(t *testing.T) {
	statusReport := previewReport(
		statusItem("git", "git:user.email", v2status.StateChangedCurrent, "live-a", "stored-a"),
		statusItem("starship", "starship:config", v2status.StateReadyToApply, "live-b", "stored-b"),
	)
	var executed []string
	report, err := Run(Options{
		RepoRoot:  t.TempDir(),
		StateRoot: t.TempDir(),
		Confirmed: true,
		JSONMode:  true,
		PreviewBuilder: func(opts selectedpreview.Options) (*selectedpreview.Report, error) {
			switch opts.Command {
			case selectedpreview.CommandStatus:
				return statusReport, nil
			case selectedpreview.CommandSave:
				return previewReport(preflightItem(opts.Ref, selectedpreview.PlannedActionWouldSave)), nil
			case selectedpreview.CommandApply:
				return previewReport(preflightItem(opts.Ref, selectedpreview.PlannedActionWouldApply)), nil
			default:
				return nil, errors.New("unexpected command " + opts.Command)
			}
		},
		LiveRunner: func(opts selectedlive.Options) (*selectedlive.Result, error) {
			executed = append(executed, opts.Command+":"+opts.Ref)
			return liveResult(opts.Ref, "run-"+opts.Command), nil
		},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"save:git:user.email", "apply:starship:config"}, executed)
	require.Equal(t, StatusComplete, report.Summary.Status)
	require.Equal(t, 2, report.Summary.Changed)
	require.Equal(t, 0, report.Summary.Failed)
	require.Equal(t, 1, report.Summary.WritesToStoredSettings)
	require.Equal(t, 1, report.Summary.WritesToLiveSettings)
	body := Text(report)
	require.Contains(t, body, "Sync complete.")
	require.Contains(t, body, "git:user.email")
	require.Contains(t, body, "live settings -> stored settings")
	require.Contains(t, body, "starship:config")
	require.Contains(t, body, "stored settings -> live settings")
	require.NotContains(t, body, "save")
	require.NotContains(t, body, "apply")
}

func TestRunRefusesConflictWithoutWrites(t *testing.T) {
	called := false
	report, err := Run(Options{
		RepoRoot:  t.TempDir(),
		StateRoot: t.TempDir(),
		Confirmed: true,
		JSONMode:  true,
		PreviewBuilder: func(opts selectedpreview.Options) (*selectedpreview.Report, error) {
			return previewReport(statusItem("git", "git:user.email", v2status.StateConflict, "live-a", "stored-a")), nil
		},
		LiveRunner: func(opts selectedlive.Options) (*selectedlive.Result, error) {
			called = true
			return nil, nil
		},
	})
	require.Error(t, err)
	require.False(t, called)
	require.Equal(t, StatusNeedsChoice, report.Summary.Status)
	require.Equal(t, 0, report.Summary.Changed)
	require.Equal(t, 1, report.Summary.Skipped)
	body := Text(report)
	require.Contains(t, body, "Sync not run: conflict needs a choice.")
	require.Contains(t, body, "Changed: 0")
	require.Contains(t, body, "Failed: 0")
}

func TestRunRequiresConfirmationBeforeWrites(t *testing.T) {
	called := false
	report, err := Run(Options{
		RepoRoot:       t.TempDir(),
		StateRoot:      t.TempDir(),
		NonInteractive: true,
		PreviewBuilder: func(opts selectedpreview.Options) (*selectedpreview.Report, error) {
			return previewReport(statusItem("git", "git:user.email", v2status.StateChangedCurrent, "live-a", "stored-a")), nil
		},
		LiveRunner: func(opts selectedlive.Options) (*selectedlive.Result, error) {
			called = true
			return nil, nil
		},
	})
	require.Error(t, err)
	require.False(t, called)
	require.Equal(t, CodeConfirmationRequired, report.Error.Code)
	require.Equal(t, StatusConfirmationRequired, report.Summary.Status)
	body := Text(report)
	require.Contains(t, body, "Sync not run: confirmation required.")
	require.Contains(t, body, "Run again with --yes")
}

func TestRunRefusesStalePlanBeforeAnyWrite(t *testing.T) {
	statusCalls := 0
	called := false
	report, err := Run(Options{
		RepoRoot:  t.TempDir(),
		StateRoot: t.TempDir(),
		Confirmed: true,
		PreviewBuilder: func(opts selectedpreview.Options) (*selectedpreview.Report, error) {
			if opts.Command == selectedpreview.CommandStatus {
				statusCalls++
				if statusCalls == 1 {
					return previewReport(statusItem("git", "git:user.email", v2status.StateChangedCurrent, "live-a", "stored-a")), nil
				}
				return previewReport(statusItem("git", "git:user.email", v2status.StateChangedCurrent, "live-changed", "stored-a")), nil
			}
			return previewReport(preflightItem(opts.Ref, selectedpreview.PlannedActionWouldSave)), nil
		},
		LiveRunner: func(opts selectedlive.Options) (*selectedlive.Result, error) {
			called = true
			return nil, nil
		},
	})
	require.Error(t, err)
	require.False(t, called)
	require.Equal(t, CodeStalePlan, report.Error.Code)
	require.Equal(t, StatusRefusedStalePlan, report.Summary.Status)
	body := Text(report)
	require.Contains(t, body, "Sync not run: settings changed since the plan was checked.")
}

func TestRunStopsAfterPartialFailure(t *testing.T) {
	statusReport := previewReport(
		statusItem("git", "git:user.email", v2status.StateChangedCurrent, "live-a", "stored-a"),
		statusItem("starship", "starship:config", v2status.StateReadyToApply, "live-b", "stored-b"),
		statusItem("zsh", "zsh:aliases", v2status.StateChangedCurrent, "live-c", "stored-c"),
	)
	var executed []string
	report, err := Run(Options{
		RepoRoot:  t.TempDir(),
		StateRoot: t.TempDir(),
		Confirmed: true,
		PreviewBuilder: func(opts selectedpreview.Options) (*selectedpreview.Report, error) {
			switch opts.Command {
			case selectedpreview.CommandStatus:
				return statusReport, nil
			case selectedpreview.CommandSave:
				return previewReport(preflightItem(opts.Ref, selectedpreview.PlannedActionWouldSave)), nil
			case selectedpreview.CommandApply:
				return previewReport(preflightItem(opts.Ref, selectedpreview.PlannedActionWouldApply)), nil
			default:
				return nil, errors.New("unexpected command " + opts.Command)
			}
		},
		LiveRunner: func(opts selectedlive.Options) (*selectedlive.Result, error) {
			executed = append(executed, opts.Ref)
			if opts.Ref == "starship:config" {
				return liveResult(opts.Ref, "run-failed"), errors.New("write failed")
			}
			return liveResult(opts.Ref, "run-ok"), nil
		},
	})
	require.Error(t, err)
	require.Equal(t, []string{"git:user.email", "starship:config"}, executed)
	require.Equal(t, StatusPartialExecutionError, report.Summary.Status)
	require.Equal(t, 1, report.Summary.Changed)
	require.Equal(t, 1, report.Summary.Failed)
	require.Equal(t, 1, report.Summary.NotAttempted)
	body := Text(report)
	require.Contains(t, body, "Sync stopped after a failure.")
	require.Contains(t, body, "Not attempted")
	require.Contains(t, body, "zsh:aliases")
}

func previewReport(items ...selectedpreview.Item) *selectedpreview.Report {
	return &selectedpreview.Report{Schema: selectedpreview.Schema, SchemaVersion: selectedpreview.SchemaVersion, Command: selectedpreview.CommandStatus, RunID: selectedpreview.RunID, ProfileStack: []string{"global"}, Items: items}
}

func statusItem(target string, ref string, state v2status.StateCode, currentHash string, desiredHash string) selectedpreview.Item {
	return selectedpreview.Item{
		TargetRef:  target,
		SettingRef: ref,
		Scope:      "user",
		Subject:    "leon",
		State:      state,
		Desired: selectedpreview.DesiredInfo{
			Status:   desired.StatusPresent,
			Intent:   desired.IntentSet,
			Kind:     desired.KindString,
			Snapshot: selectedpreview.Snapshot{Exists: true, SHA256: desiredHash, Normalizer: "test"},
		},
		Current: selectedpreview.Snapshot{Exists: true, SHA256: currentHash, Normalizer: "test"},
	}
}

func preflightItem(ref string, action string) selectedpreview.Item {
	return selectedpreview.Item{TargetRef: targetFromRef(ref), SettingRef: ref, PlannedAction: action, State: v2status.StateChangedCurrent}
}

func liveResult(ref string, runID string) *selectedlive.Result {
	return &selectedlive.Result{Report: &selectedpreview.Report{Items: []selectedpreview.Item{{
		SettingRef: ref,
		Mutated:    true,
		Mutation: &selectedpreview.MutationInfo{
			RunID:        runID,
			Result:       "verified",
			Verification: selectedpreview.VerificationInfo{Verified: true, Result: "verified"},
		},
	}}}}
}

func targetFromRef(ref string) string {
	before, _, ok := strings.Cut(ref, ":")
	if !ok {
		return ref
	}
	return before
}

func TestRunRefusesMultipleWritesSharingBackingFile(t *testing.T) {
	first := statusItem("app", "app:one", v2status.StateChangedCurrent, "live-a", "stored-a")
	first.Resource = selectedpreview.ResourceInfo{ID: "config", DriverID: "yaml-file", Path: "/tmp/app.yaml"}
	first.DesiredURI = "desired://user/leon/targets/app/settings.yaml"
	first.DesiredRelPath = "desired/user/leon/targets/app/settings.yaml"
	second := statusItem("app", "app:two", v2status.StateReadyToApply, "live-b", "stored-b")
	second.Resource = first.Resource
	second.DesiredURI = first.DesiredURI
	second.DesiredRelPath = first.DesiredRelPath

	called := false
	report, err := Run(Options{
		RepoRoot:  t.TempDir(),
		StateRoot: t.TempDir(),
		Confirmed: true,
		PreviewBuilder: func(opts selectedpreview.Options) (*selectedpreview.Report, error) {
			return previewReport(first, second), nil
		},
		LiveRunner: func(opts selectedlive.Options) (*selectedlive.Result, error) {
			called = true
			return nil, nil
		},
	})
	require.Error(t, err)
	require.False(t, called)
	require.Equal(t, CodeWriteSetInvalid, report.Error.Code)
	require.Equal(t, StatusRefusedUnsafePlan, report.Summary.Status)
}

func TestRunRevalidatesBeforeEachMutation(t *testing.T) {
	first := statusItem("git", "git:user.email", v2status.StateChangedCurrent, "live-a", "stored-a")
	second := statusItem("starship", "starship:config", v2status.StateReadyToApply, "live-b", "stored-b")
	staleSecond := statusItem("starship", "starship:config", v2status.StateReadyToApply, "live-b-stale", "stored-b")
	executedFirst := false
	var executed []string

	report, err := Run(Options{
		RepoRoot:  t.TempDir(),
		StateRoot: t.TempDir(),
		Confirmed: true,
		PreviewBuilder: func(opts selectedpreview.Options) (*selectedpreview.Report, error) {
			switch opts.Command {
			case selectedpreview.CommandStatus:
				if executedFirst {
					return previewReport(first, staleSecond), nil
				}
				return previewReport(first, second), nil
			case selectedpreview.CommandSave:
				return previewReport(preflightItem(opts.Ref, selectedpreview.PlannedActionWouldSave)), nil
			case selectedpreview.CommandApply:
				return previewReport(preflightItem(opts.Ref, selectedpreview.PlannedActionWouldApply)), nil
			default:
				return nil, errors.New("unexpected command " + opts.Command)
			}
		},
		LiveRunner: func(opts selectedlive.Options) (*selectedlive.Result, error) {
			executed = append(executed, opts.Ref)
			if opts.Ref == "git:user.email" {
				executedFirst = true
			}
			return liveResult(opts.Ref, "run-"+opts.Command), nil
		},
	})
	require.Error(t, err)
	require.Equal(t, []string{"git:user.email"}, executed)
	require.Equal(t, CodeExecutionFailed, report.Error.Code)
	require.Equal(t, StatusPartialExecutionError, report.Summary.Status)
	require.Equal(t, 1, report.Summary.Changed)
	require.Equal(t, 1, report.Summary.NotAttempted)
}

func TestRunRefusesStaleWriteIdentityBeforeMutation(t *testing.T) {
	planned := statusItem("git", "git:user.email", v2status.StateChangedCurrent, "live-a", "stored-a")
	planned.Resource = selectedpreview.ResourceInfo{ID: "config-a", DriverID: recipe.YAMLFileDriverID, Path: "/tmp/a.yaml"}
	planned.DesiredURI = "desired://user/leon/targets/git/settings-a.yaml"
	planned.DesiredRelPath = "desired/user/leon/targets/git/settings-a.yaml"
	latest := planned
	latest.Resource = selectedpreview.ResourceInfo{ID: "config-b", DriverID: recipe.YAMLFileDriverID, Path: "/tmp/b.yaml"}
	latest.DesiredURI = "desired://user/leon/targets/git/settings-b.yaml"
	latest.DesiredRelPath = "desired/user/leon/targets/git/settings-b.yaml"

	statusCalls := 0
	called := false
	report, err := Run(Options{
		RepoRoot:  t.TempDir(),
		StateRoot: t.TempDir(),
		Confirmed: true,
		PreviewBuilder: func(opts selectedpreview.Options) (*selectedpreview.Report, error) {
			if opts.Command == selectedpreview.CommandStatus {
				statusCalls++
				if statusCalls == 1 {
					return previewReport(planned), nil
				}
				return previewReport(latest), nil
			}
			return previewReport(preflightItem(opts.Ref, selectedpreview.PlannedActionWouldSave)), nil
		},
		LiveRunner: func(opts selectedlive.Options) (*selectedlive.Result, error) {
			called = true
			return nil, nil
		},
	})
	require.Error(t, err)
	require.False(t, called)
	require.Equal(t, CodeStalePlan, report.Error.Code)
	require.Equal(t, StatusRefusedStalePlan, report.Summary.Status)
}

func TestRunNoChangeBlockedAndPlanningErrorBranches(t *testing.T) {
	t.Run("no changes complete without confirmation", func(t *testing.T) {
		report, err := Run(Options{
			RepoRoot:  t.TempDir(),
			StateRoot: t.TempDir(),
			PreviewBuilder: func(opts selectedpreview.Options) (*selectedpreview.Report, error) {
				return previewReport(statusItem("git", "git:user.email", v2status.StateUnchanged, "same", "same")), nil
			},
		})
		require.NoError(t, err)
		require.Equal(t, StatusNoChanges, report.Summary.Status)
		require.False(t, report.Confirmation.Required)
		require.Contains(t, Text(report), "Live settings and stored settings already match.")
	})

	t.Run("blocked item refuses without writes", func(t *testing.T) {
		called := false
		report, err := Run(Options{
			RepoRoot:  t.TempDir(),
			StateRoot: t.TempDir(),
			Confirmed: true,
			PreviewBuilder: func(opts selectedpreview.Options) (*selectedpreview.Report, error) {
				return previewReport(statusItem("raycast", "raycast:preferences", v2status.StateBlockedLifecycle, "live", "stored")), nil
			},
			LiveRunner: func(opts selectedlive.Options) (*selectedlive.Result, error) {
				called = true
				return nil, nil
			},
		})
		require.Error(t, err)
		require.False(t, called)
		require.Equal(t, CodeBlocked, report.Error.Code)
		require.Equal(t, StatusBlocked, report.Summary.Status)
		require.Contains(t, Text(report), "Sync not run: no safe writes are available.")
	})

	t.Run("error diagnostic on write-like state blocks rather than silently skipping", func(t *testing.T) {
		called := false
		report, err := Run(Options{
			RepoRoot:  t.TempDir(),
			StateRoot: t.TempDir(),
			Confirmed: true,
			PreviewBuilder: func(opts selectedpreview.Options) (*selectedpreview.Report, error) {
				item := statusItem("git", "git:user.email", v2status.StateChangedCurrent, "live", "stored")
				item.Diagnostics = []selectedpreview.Diagnostic{{Code: "selectedpreview.test.error", Severity: selectedpreview.SeverityError, Message: "unsafe"}}
				return previewReport(item), nil
			},
			LiveRunner: func(opts selectedlive.Options) (*selectedlive.Result, error) {
				called = true
				return nil, nil
			},
		})
		require.Error(t, err)
		require.False(t, called)
		require.Equal(t, CodeBlocked, report.Error.Code)
		require.Equal(t, StatusBlocked, report.Summary.Status)
		require.Equal(t, DecisionBlocked, report.Items[0].Decision)
		require.False(t, report.Items[0].ExecutableBySync)
	})

	t.Run("file tree writes require detailed review outside sync", func(t *testing.T) {
		called := false
		report, err := Run(Options{
			RepoRoot:  t.TempDir(),
			StateRoot: t.TempDir(),
			Confirmed: true,
			PreviewBuilder: func(opts selectedpreview.Options) (*selectedpreview.Report, error) {
				item := statusItem("nvim", "nvim:config", v2status.StateReadyToApply, "live", "stored")
				item.Resource = selectedpreview.ResourceInfo{ID: "config", DriverID: recipe.FileTreeDriverID, Path: "/tmp/nvim"}
				return previewReport(item), nil
			},
			LiveRunner: func(opts selectedlive.Options) (*selectedlive.Result, error) {
				called = true
				return nil, nil
			},
		})
		require.Error(t, err)
		require.False(t, called)
		require.Equal(t, CodeBlocked, report.Error.Code)
		require.Equal(t, StatusBlocked, report.Summary.Status)
		require.Equal(t, "folder-tree-review-required", report.Items[0].ReasonCode)
		require.False(t, report.Items[0].ExecutableBySync)
		require.Contains(t, Text(report), "folder setting needs a detailed file-by-file review")
	})

	t.Run("planning error becomes sync planning failure", func(t *testing.T) {
		report, err := Run(Options{
			RepoRoot:  t.TempDir(),
			StateRoot: t.TempDir(),
			PreviewBuilder: func(opts selectedpreview.Options) (*selectedpreview.Report, error) {
				return nil, errors.New("planner unavailable")
			},
		})
		require.Error(t, err)
		require.Equal(t, StatusError, report.Summary.Status)
		require.Equal(t, CodePlanningFailed, report.Error.Code)
		require.Contains(t, Text(report), "planner unavailable")
	})
}

func TestPromptConfirmationAndJSONHelpers(t *testing.T) {
	report := baseReport("git:user.email")
	report.Items = []Item{{
		TargetRef:        "git",
		SettingRef:       "git:user.email",
		Decision:         DecisionWrite,
		Direction:        DirectionLiveToStored,
		WouldWrite:       true,
		ExecutableBySync: true,
		Result:           ResultPending,
	}}

	var acceptedPrompt bytes.Buffer
	require.NoError(t, promptForConfirmation(report, Options{In: strings.NewReader("yes\n"), PromptOut: &acceptedPrompt}, []int{0}))
	require.Contains(t, acceptedPrompt.String(), "Proceed with sync? [y/N]")

	var refusedPrompt bytes.Buffer
	err := promptForConfirmation(report, Options{In: strings.NewReader("no\n"), PromptOut: &refusedPrompt}, []int{0})
	require.Error(t, err)
	require.Equal(t, CodeConfirmationRefused, err.(*Error).Code)

	err = promptForConfirmation(report, Options{In: strings.NewReader(""), PromptOut: ioDiscardBuffer{}}, []int{0})
	require.Error(t, err)
	require.Equal(t, CodeConfirmationRequired, err.(*Error).Code)

	confirmed, source, err := confirmWriteSet(report, Options{Confirmed: true}, []int{0})
	require.NoError(t, err)
	require.True(t, confirmed)
	require.Equal(t, ConfirmationSourceFlag, source)

	jsonBody, err := JSON(report)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal([]byte(jsonBody), &decoded))
	require.Equal(t, Schema, decoded["schema"])

	nilJSON, err := JSON(nil)
	require.NoError(t, err)
	require.Contains(t, nilJSON, StatusError)

	errorReport := ErrorReport("test.code", "test message", map[string]any{"k": "v"})
	require.Equal(t, StatusError, errorReport.Summary.Status)
	require.Contains(t, Text(nil), "Sync not run: internal error.")
	require.Equal(t, "test message", (&Error{Message: "test message"}).Error())
	require.Equal(t, v2preview.ExitInternalError, (&Error{}).ExitCode())
}

type ioDiscardBuffer struct{}

func (ioDiscardBuffer) Write(p []byte) (int, error) { return len(p), nil }

func TestPreviewStateMappingAndRenderingBranches(t *testing.T) {
	cases := []struct {
		state     v2status.StateCode
		decision  string
		direction string
		reason    string
	}{
		{v2status.StateMissingCurrent, DecisionWrite, DirectionStoredToLive, "missing-live-settings"},
		{v2status.StateMissingDesired, DecisionNeedsChoice, DirectionLiveToStored, "no-stored-settings-yet"},
		{v2status.StateOpaqueChanged, DecisionNeedsChoice, DirectionBothSidesChange, "opaque-change"},
		{v2status.StateBlockedSafety, DecisionBlocked, DirectionUnknown, "blocked-safety"},
		{v2status.StateUnsupported, DecisionBlocked, DirectionUnknown, "unsupported"},
		{v2status.StateUnknown, DecisionNeedsChoice, DirectionUnknown, "unknown-state"},
	}
	for _, tc := range cases {
		t.Run(string(tc.state), func(t *testing.T) {
			item := fromPreviewItem(statusItem("app", "app:setting", tc.state, "live", "stored"))
			require.Equal(t, tc.decision, item.Decision)
			require.Equal(t, tc.direction, item.Direction)
			require.Equal(t, tc.reason, item.ReasonCode)
		})
	}

	require.Equal(t, "no safe direction", publicDirection("bad"))
	require.Equal(t, "not planned", publicDirection(DirectionNone))
	require.Equal(t, "both sides changed", publicDirection(DirectionBothSidesChange))
	require.Equal(t, "choice required before sync can continue.", skipReason(Item{Decision: DecisionNeedsChoice}))
	require.Equal(t, "cannot plan safely.", skipReason(Item{Decision: DecisionBlocked}))
	require.Equal(t, "custom message.", skipReason(Item{Message: "custom message."}))
	require.Equal(t, "Stored settings could not be written.", strings.ToUpper(failureMessageForDirection(DirectionLiveToStored)[:1])+failureMessageForDirection(DirectionLiveToStored)[1:])
	require.Equal(t, "Settings", targetLabel(""))
	require.Equal(t, "Git User Email", targetLabel("git-user_email"))
	require.Equal(t, "git", targetFromSettingRef("git:user.email"))
	require.Equal(t, "plain", targetFromSettingRef("plain"))
	require.Equal(t, "fallback", fallback("", "fallback"))
	require.Equal(t, "value", fallback(" value ", "fallback"))
}

func TestPreflightAndExecutionErrorBranches(t *testing.T) {
	t.Run("preflight missing item refuses before mutation", func(t *testing.T) {
		called := false
		report, err := Run(Options{
			RepoRoot:  t.TempDir(),
			StateRoot: t.TempDir(),
			Confirmed: true,
			PreviewBuilder: func(opts selectedpreview.Options) (*selectedpreview.Report, error) {
				switch opts.Command {
				case selectedpreview.CommandStatus:
					return previewReport(statusItem("git", "git:user.email", v2status.StateChangedCurrent, "live", "stored")), nil
				case selectedpreview.CommandSave:
					return previewReport(), nil
				default:
					return nil, errors.New("unexpected command " + opts.Command)
				}
			},
			LiveRunner: func(opts selectedlive.Options) (*selectedlive.Result, error) {
				called = true
				return nil, nil
			},
		})
		require.Error(t, err)
		require.False(t, called)
		require.Equal(t, CodeUnsafePlan, report.Error.Code)
		require.Equal(t, StatusRefusedUnsafePlan, report.Summary.Status)
	})

	t.Run("live runner report error is redacted into sync failure", func(t *testing.T) {
		report, err := Run(Options{
			RepoRoot:  t.TempDir(),
			StateRoot: t.TempDir(),
			Confirmed: true,
			Now:       func() time.Time { return time.Date(2026, 6, 23, 20, 0, 0, 0, time.UTC) },
			PreviewBuilder: func(opts selectedpreview.Options) (*selectedpreview.Report, error) {
				switch opts.Command {
				case selectedpreview.CommandStatus:
					return previewReport(statusItem("git", "git:user.email", v2status.StateChangedCurrent, "live", "stored")), nil
				case selectedpreview.CommandSave:
					return previewReport(preflightItem(opts.Ref, selectedpreview.PlannedActionWouldSave)), nil
				default:
					return nil, errors.New("unexpected command " + opts.Command)
				}
			},
			LiveRunner: func(opts selectedlive.Options) (*selectedlive.Result, error) {
				require.Contains(t, opts.RunID, "smart-sync-20260623T200000Z")
				return &selectedlive.Result{Report: &selectedpreview.Report{Error: &selectedpreview.ErrorObj{Code: "selectedlive.failed", Message: "lower-level failure"}}}, errors.New("lower-level failure")
			},
		})
		require.Error(t, err)
		require.Equal(t, StatusPartialExecutionError, report.Summary.Status)
		require.Equal(t, 1, report.Summary.Failed)
		require.Contains(t, Text(report), "stored settings could not be written")
	})

	t.Run("nil lower level error still requires verified mutation", func(t *testing.T) {
		report, err := Run(Options{
			RepoRoot:  t.TempDir(),
			StateRoot: t.TempDir(),
			Confirmed: true,
			PreviewBuilder: func(opts selectedpreview.Options) (*selectedpreview.Report, error) {
				switch opts.Command {
				case selectedpreview.CommandStatus:
					return previewReport(statusItem("git", "git:user.email", v2status.StateChangedCurrent, "live", "stored")), nil
				case selectedpreview.CommandSave:
					return previewReport(preflightItem(opts.Ref, selectedpreview.PlannedActionWouldSave)), nil
				default:
					return nil, errors.New("unexpected command " + opts.Command)
				}
			},
			LiveRunner: func(opts selectedlive.Options) (*selectedlive.Result, error) {
				return &selectedlive.Result{Report: &selectedpreview.Report{Items: []selectedpreview.Item{{
					SettingRef: opts.Ref,
					Mutation: &selectedpreview.MutationInfo{
						Result:       "failed",
						Verification: selectedpreview.VerificationInfo{Verified: false, Result: "failed"},
					},
				}}}}, nil
			},
		})
		require.Error(t, err)
		require.Equal(t, CodeExecutionFailed, report.Error.Code)
		require.Equal(t, StatusPartialExecutionError, report.Summary.Status)
		require.Equal(t, 1, report.Summary.Failed)
		require.Equal(t, 0, report.Summary.Changed)
	})
}
