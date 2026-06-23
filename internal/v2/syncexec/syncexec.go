// Package syncexec implements reset-v2 smart-sync execution.
//
// It is the public `sync` execution layer for safe one-sided changes between
// live settings and stored settings. It may call lower-level directional
// primitives internally, but it owns the reset-v2 confirmation, refusal, and
// reporting vocabulary.
package syncexec

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	v2ledger "github.com/shpoont/dotfiles-manager/internal/v2/ledger"
	"github.com/shpoont/dotfiles-manager/internal/v2/lifecycle"
	"github.com/shpoont/dotfiles-manager/internal/v2/macosdefaultsdriver"
	v2preview "github.com/shpoont/dotfiles-manager/internal/v2/preview"
	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/shpoont/dotfiles-manager/internal/v2/selectedlive"
	"github.com/shpoont/dotfiles-manager/internal/v2/selectedpreview"
	v2status "github.com/shpoont/dotfiles-manager/internal/v2/status"
)

const (
	Schema        = "dotfiles-manager.sync-execution.v2"
	SchemaVersion = 1
	Command       = "sync"
	RunID         = "smart-sync-execution"
)

const (
	DecisionWrite       = "write"
	DecisionSkip        = "skip"
	DecisionNeedsChoice = "needs_choice"
	DecisionBlocked     = "blocked"
)

const (
	DirectionLiveToStored    = "live_to_stored"
	DirectionStoredToLive    = "stored_to_live"
	DirectionBothSidesChange = "both_sides_changed"
	DirectionNone            = "none"
	DirectionUnknown         = "unknown"
)

const (
	ResultPending      = "pending"
	ResultChanged      = "changed"
	ResultSkipped      = "skipped"
	ResultFailed       = "failed"
	ResultNotAttempted = "not_attempted"
)

const (
	StatusComplete              = "complete"
	StatusNoChanges             = "no-changes"
	StatusNeedsChoice           = "needs-choice"
	StatusBlocked               = "blocked"
	StatusConfirmationRequired  = "confirmation-required"
	StatusConfirmationRefused   = "confirmation-refused"
	StatusRefusedStalePlan      = "refused-stale-plan"
	StatusRefusedUnsafePlan     = "refused-unsafe-plan"
	StatusPartialExecutionError = "partial-execution-error"
	StatusError                 = "error"
)

const (
	CodePlanningFailed        = "syncexec.planningFailed"
	CodeChoiceRequired        = "syncexec.choiceRequired"
	CodeBlocked               = "syncexec.blocked"
	CodeConfirmationRequired  = "syncexec.confirmationRequired"
	CodeConfirmationRefused   = "syncexec.confirmationRefused"
	CodeStalePlan             = "syncexec.planStale"
	CodeUnsafePlan            = "syncexec.planUnsafe"
	CodeExecutionFailed       = "syncexec.executionFailed"
	CodeWriteSetInvalid       = "syncexec.writeSetInvalid"
	CodeInternalExecutionPlan = "syncexec.internalExecutionPlan"
)

const (
	ConfirmationSourceFlag   = "flag"
	ConfirmationSourcePrompt = "prompt"
	ConfirmationSourceNone   = "none"
)

// PreviewBuilder exists so tests can exercise execution and refusal branches
// without manufacturing a full live settings folder for every state.
type PreviewBuilder func(selectedpreview.Options) (*selectedpreview.Report, error)

// LiveRunner abstracts the lower-level internal directional write primitive.
type LiveRunner func(selectedlive.Options) (*selectedlive.Result, error)

type Options struct {
	ConfigPath     string
	RepoRoot       string
	StateRoot      string
	Ref            string
	MachineID      string
	UserID         string
	ExtraLayers    []string
	Confirmed      bool
	NonInteractive bool
	JSONMode       bool
	In             io.Reader
	PromptOut      io.Writer

	LocationRoots       map[string]map[string]string
	MacOSDefaultsRunner macosdefaultsdriver.Runner
	LifecycleDetector   lifecycle.Detector
	LifecycleController lifecycle.Controller
	Now                 func() time.Time

	PreviewBuilder PreviewBuilder
	LiveRunner     LiveRunner
}

type Report struct {
	Schema        string       `json:"schema"`
	SchemaVersion int          `json:"schemaVersion"`
	Command       string       `json:"command"`
	RunID         string       `json:"runId"`
	ProfileStack  []string     `json:"profileStack"`
	Selection     Selection    `json:"selection"`
	Confirmation  Confirmation `json:"confirmation"`
	Summary       Summary      `json:"summary"`
	Items         []Item       `json:"items"`
	Error         *ErrorObj    `json:"error,omitempty"`
}

type Selection struct {
	Kind string `json:"kind"`
	Ref  string `json:"ref,omitempty"`
}

type Confirmation struct {
	Required  bool   `json:"required"`
	Confirmed bool   `json:"confirmed"`
	Source    string `json:"source"`
}

type Summary struct {
	Status                 string `json:"status"`
	Changed                int    `json:"changed"`
	Skipped                int    `json:"skipped"`
	Failed                 int    `json:"failed"`
	NotAttempted           int    `json:"notAttempted"`
	WritesToStoredSettings int    `json:"writesToStoredSettings"`
	WritesToLiveSettings   int    `json:"writesToLiveSettings"`
	NeedsChoice            int    `json:"needsChoice"`
	Blocked                int    `json:"blocked"`
}

type ErrorObj struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type Item struct {
	TargetRef        string                       `json:"targetRef"`
	SettingRef       string                       `json:"settingRef"`
	Scope            string                       `json:"scope,omitempty"`
	Subject          string                       `json:"subject,omitempty"`
	State            string                       `json:"state"`
	Decision         string                       `json:"decision"`
	Direction        string                       `json:"direction"`
	WouldWrite       bool                         `json:"wouldWrite"`
	ChoiceRequired   bool                         `json:"choiceRequired"`
	AllowedChoices   []string                     `json:"allowedChoices"`
	ExecutableBySync bool                         `json:"executableBySync"`
	Result           string                       `json:"result"`
	ReasonCode       string                       `json:"reasonCode"`
	Message          string                       `json:"message,omitempty"`
	EvidenceID       string                       `json:"evidenceId,omitempty"`
	ValuesRedacted   bool                         `json:"valuesRedacted"`
	UnderlyingRunID  string                       `json:"underlyingRunId,omitempty"`
	Diagnostics      []selectedpreview.Diagnostic `json:"-"`
	BackingKey       string                       `json:"-"`
}

type Error struct {
	Code    string
	Message string
	Exit    int
	Details map[string]any
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *Error) ExitCode() int {
	if e == nil || e.Exit == 0 {
		return v2preview.ExitInternalError
	}
	return e.Exit
}

func Run(opts Options) (*Report, error) {
	report, err := buildPlan(opts)
	if err != nil {
		finishReport(report)
		return report, err
	}

	writeIndexes := executableWriteIndexes(report)
	if len(writeIndexes) == 0 {
		if hasNeedsChoice(report) {
			err := &Error{Code: CodeChoiceRequired, Message: "sync needs a choice before any writes can happen", Exit: v2preview.ExitInputRequired}
			attachError(report, err)
			finishReport(report)
			return report, err
		}
		if hasBlocked(report) {
			err := &Error{Code: CodeBlocked, Message: "sync could not find a safe write to run", Exit: v2preview.ExitValidation}
			attachError(report, err)
			finishReport(report)
			return report, err
		}
		finishReport(report)
		return report, nil
	}

	if err := validateWriteSetNoSharedBacking(report, writeIndexes); err != nil {
		attachError(report, err)
		finishReport(report)
		return report, err
	}

	report.Confirmation.Required = true
	confirmed, confirmSource, confirmErr := confirmWriteSet(report, opts, writeIndexes)
	report.Confirmation.Confirmed = confirmed
	report.Confirmation.Source = confirmSource
	if confirmErr != nil {
		attachError(report, confirmErr)
		finishReport(report)
		return report, confirmErr
	}

	if err := revalidateCurrentPlan(report, opts, writeIndexes); err != nil {
		attachError(report, err)
		finishReport(report)
		return report, err
	}
	if err := preflightWriteSet(report, opts, writeIndexes); err != nil {
		attachError(report, err)
		finishReport(report)
		return report, err
	}
	if err := executeWriteSet(report, opts, writeIndexes); err != nil {
		attachError(report, err)
		finishReport(report)
		return report, err
	}

	finishReport(report)
	return report, nil
}

func JSON(report *Report) (string, error) {
	if report == nil {
		report = baseReport("")
		report.Summary.Status = StatusError
	}
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	return string(payload) + "\n", nil
}

func ErrorReport(code string, message string, details map[string]any) *Report {
	report := baseReport("")
	report.Error = &ErrorObj{Code: code, Message: message, Details: details}
	finishReport(report)
	return report
}

func Text(report *Report) string {
	if report == nil {
		return strings.Join([]string{"Sync not run: internal error.", "", "Summary:", "  Changed: 0", "  Skipped: 0", "  Failed: 0"}, "\n")
	}

	lines := []string{}
	if shouldRenderAcceptedPlan(report) {
		lines = append(lines, acceptedPlanLines(report)...)
		lines = append(lines, "")
	}

	lines = append(lines, headlineLines(report)...)
	if sections := resultSections(report); len(sections) > 0 {
		lines = append(lines, "")
		lines = append(lines, sections...)
	}
	lines = append(lines, "")
	lines = append(lines, summaryLines(report)...)
	return strings.Join(trimTrailingBlank(lines), "\n")
}

func buildPlan(opts Options) (*Report, error) {
	builder := opts.PreviewBuilder
	if builder == nil {
		builder = selectedpreview.Build
	}
	previewReport, err := builder(selectedpreview.Options{
		Command:             selectedpreview.CommandStatus,
		ConfigPath:          opts.ConfigPath,
		RepoRoot:            opts.RepoRoot,
		StateRoot:           opts.StateRoot,
		Ref:                 opts.Ref,
		MachineID:           opts.MachineID,
		UserID:              opts.UserID,
		ExtraLayers:         append([]string(nil), opts.ExtraLayers...),
		LocationRoots:       opts.LocationRoots,
		MacOSDefaultsRunner: opts.MacOSDefaultsRunner,
		LifecycleDetector:   opts.LifecycleDetector,
	})
	report := baseReport(opts.Ref)
	if previewReport != nil {
		report.ProfileStack = append([]string(nil), previewReport.ProfileStack...)
		for _, item := range previewReport.Items {
			report.Items = append(report.Items, fromPreviewItem(item))
		}
		if previewReport.Error != nil {
			report.Error = &ErrorObj{Code: previewReport.Error.Code, Message: previewReport.Error.Message, Details: previewReport.Error.Details}
		}
	}
	if err != nil {
		wrapped := toSyncError(err, CodePlanningFailed, "sync planning failed", v2preview.ExitValidation)
		if report.Error == nil {
			report.Error = &ErrorObj{Code: wrapped.Code, Message: wrapped.Message, Details: wrapped.Details}
		}
		return report, wrapped
	}
	return report, nil
}

func baseReport(ref string) *Report {
	kind := "all-apps"
	if strings.TrimSpace(ref) != "" {
		kind = "ref"
	}
	return &Report{
		Schema:        Schema,
		SchemaVersion: SchemaVersion,
		Command:       Command,
		RunID:         RunID,
		Selection:     Selection{Kind: kind, Ref: strings.TrimSpace(ref)},
		Confirmation:  Confirmation{Source: ConfirmationSourceNone},
		Items:         []Item{},
	}
}

func fromPreviewItem(item selectedpreview.Item) Item {
	out := Item{
		TargetRef:      item.TargetRef,
		SettingRef:     item.SettingRef,
		Scope:          item.Scope,
		Subject:        item.Subject,
		AllowedChoices: []string{},
		Result:         ResultSkipped,
		ValuesRedacted: true,
		EvidenceID:     evidenceID(item),
		Message:        publicMessage(item),
		Diagnostics:    append([]selectedpreview.Diagnostic(nil), item.Diagnostics...),
		BackingKey:     backingKey(item),
	}

	switch item.State {
	case v2status.StateUnchanged:
		out.State = "up-to-date"
		out.Decision = DecisionSkip
		out.Direction = DirectionNone
		out.ReasonCode = "up-to-date"
		out.Message = fallback(out.Message, "Live settings and stored settings already match.")
	case v2status.StateChangedCurrent:
		out.State = "changed-in-live-settings"
		out.Decision = DecisionWrite
		out.Direction = DirectionLiveToStored
		out.WouldWrite = true
		out.ExecutableBySync = true
		out.Result = ResultPending
		out.ReasonCode = "one-sided-live-change"
		out.Message = "Live settings changed and stored settings did not."
	case v2status.StateReadyToApply:
		out.State = "changed-in-stored-settings"
		out.Decision = DecisionWrite
		out.Direction = DirectionStoredToLive
		out.WouldWrite = true
		out.ExecutableBySync = true
		out.Result = ResultPending
		out.ReasonCode = "one-sided-stored-change"
		out.Message = "Stored settings changed and live settings did not."
	case v2status.StateMissingCurrent:
		out.State = "missing-in-live-settings"
		out.Decision = DecisionWrite
		out.Direction = DirectionStoredToLive
		out.WouldWrite = true
		out.ExecutableBySync = true
		out.Result = ResultPending
		out.ReasonCode = "missing-live-settings"
		out.Message = "Stored settings exist and the live setting is missing."
	case v2status.StateMissingDesired:
		out.State = "no-stored-settings-yet"
		out.Decision = DecisionNeedsChoice
		out.Direction = DirectionLiveToStored
		out.ChoiceRequired = true
		out.AllowedChoices = []string{"choose_live_settings", "skip"}
		out.ReasonCode = "no-stored-settings-yet"
		out.Message = "Live settings exist, but no stored settings exist yet."
	case v2status.StateConflict:
		out.State = "conflict"
		out.Decision = DecisionNeedsChoice
		out.Direction = DirectionBothSidesChange
		out.ChoiceRequired = true
		out.AllowedChoices = []string{"choose_live_settings", "choose_stored_settings", "skip"}
		out.ReasonCode = "both-sides-changed"
		out.Message = "Live settings and stored settings both changed."
	case v2status.StateOpaqueChanged:
		out.State = "conflict"
		out.Decision = DecisionNeedsChoice
		out.Direction = DirectionBothSidesChange
		out.ChoiceRequired = true
		out.AllowedChoices = []string{"choose_live_settings", "choose_stored_settings", "skip"}
		out.ReasonCode = "opaque-change"
		out.Message = "The setting changed, but sync cannot safely pick a direction."
	case v2status.StateBlockedLifecycle, v2status.StateBlockedSafety, v2status.StateUnsupported:
		out.State = "blocked"
		out.Decision = DecisionBlocked
		out.Direction = DirectionUnknown
		out.ReasonCode = publicBlockedReasonCode(item.State)
		out.Message = fallback(out.Message, "Cannot plan this setting safely.")
	default:
		out.State = "unknown"
		out.Decision = DecisionNeedsChoice
		out.Direction = DirectionUnknown
		out.ChoiceRequired = true
		out.AllowedChoices = []string{"choose_live_settings", "choose_stored_settings", "skip"}
		out.ReasonCode = "unknown-state"
		out.Message = fallback(out.Message, "State cannot be determined safely.")
	}
	if hasBlockingDiagnostics(out.Diagnostics) {
		blockItem(&out, "blocked-safety", "Cannot plan this setting safely.")
	}
	if item.Resource.DriverID == recipe.FileTreeDriverID && out.Decision == DecisionWrite {
		blockItem(&out, "folder-tree-review-required", "folder setting needs a detailed file-by-file review before sync can change it.")
	}
	return out
}

func blockItem(item *Item, reasonCode string, message string) {
	if item == nil {
		return
	}
	item.State = "blocked"
	item.Decision = DecisionBlocked
	item.Direction = DirectionUnknown
	item.WouldWrite = false
	item.ChoiceRequired = false
	item.AllowedChoices = []string{}
	item.ExecutableBySync = false
	item.Result = ResultSkipped
	item.ReasonCode = reasonCode
	item.Message = message
}

func publicMessage(item selectedpreview.Item) string {
	message := strings.TrimSpace(item.Message)
	replacements := map[string]string{
		"desired":        "stored",
		"Desired":        "Stored",
		"current":        "live",
		"Current":        "Live",
		"selected-value": "settings",
		"Selected-value": "Settings",
	}
	for old, newValue := range replacements {
		message = strings.ReplaceAll(message, old, newValue)
	}
	return message
}

func publicBlockedReasonCode(state v2status.StateCode) string {
	switch state {
	case v2status.StateBlockedLifecycle:
		return "blocked-lifecycle"
	case v2status.StateUnsupported:
		return "unsupported"
	default:
		return "blocked-safety"
	}
}

func executableWriteIndexes(report *Report) []int {
	if report == nil {
		return nil
	}
	indexes := []int{}
	for idx, item := range report.Items {
		if isExecutableWrite(item) {
			indexes = append(indexes, idx)
		}
	}
	return indexes
}

func isExecutableWrite(item Item) bool {
	return executableBySync(item, true)
}

func executableBySync(item Item, evidenceFresh bool) bool {
	return item.Decision == DecisionWrite &&
		strings.TrimSpace(item.SettingRef) != "" &&
		item.WouldWrite &&
		item.ExecutableBySync &&
		!item.ChoiceRequired &&
		evidenceFresh &&
		(item.Direction == DirectionLiveToStored || item.Direction == DirectionStoredToLive) &&
		!hasBlockingDiagnostics(item.Diagnostics)
}

func hasBlockingDiagnostics(diagnostics []selectedpreview.Diagnostic) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == selectedpreview.SeverityError {
			return true
		}
	}
	return false
}

func validateWriteSetNoSharedBacking(report *Report, writeIndexes []int) error {
	seen := map[string]string{}
	for _, idx := range writeIndexes {
		item := report.Items[idx]
		key := strings.TrimSpace(item.BackingKey)
		if key == "" {
			continue
		}
		if previous, exists := seen[key]; exists {
			return &Error{Code: CodeWriteSetInvalid, Message: "sync cannot safely write multiple settings that share the same backing file yet", Exit: v2preview.ExitValidation, Details: map[string]any{"settingRefs": []string{previous, item.SettingRef}}}
		}
		seen[key] = item.SettingRef
	}
	return nil
}

func hasNeedsChoice(report *Report) bool {
	if report == nil {
		return false
	}
	for _, item := range report.Items {
		if item.ChoiceRequired || item.Decision == DecisionNeedsChoice {
			return true
		}
	}
	return false
}

func hasBlocked(report *Report) bool {
	if report == nil {
		return false
	}
	for _, item := range report.Items {
		if item.Decision == DecisionBlocked {
			return true
		}
	}
	return false
}

func confirmWriteSet(report *Report, opts Options, writeIndexes []int) (bool, string, error) {
	if opts.Confirmed {
		return true, ConfirmationSourceFlag, nil
	}
	if opts.NonInteractive || opts.JSONMode {
		return false, ConfirmationSourceNone, &Error{Code: CodeConfirmationRequired, Message: "sync needs confirmation before writing settings", Exit: v2preview.ExitInputRequired, Details: map[string]any{"requiredFlag": "--yes"}}
	}
	if err := promptForConfirmation(report, opts, writeIndexes); err != nil {
		return false, ConfirmationSourcePrompt, err
	}
	return true, ConfirmationSourcePrompt, nil
}

func promptForConfirmation(report *Report, opts Options, writeIndexes []int) error {
	in := opts.In
	if in == nil {
		in = strings.NewReader("")
	}
	out := opts.PromptOut
	if out == nil {
		out = io.Discard
	}
	_, _ = fmt.Fprintln(out, "Sync plan accepted.")
	_, _ = fmt.Fprintf(out, "Will sync %d %s:\n", len(writeIndexes), plural("setting", len(writeIndexes)))
	for _, idx := range writeIndexes {
		item := report.Items[idx]
		_, _ = fmt.Fprintf(out, "- %s: %s\n", item.SettingRef, publicDirection(item.Direction))
	}
	_, _ = fmt.Fprint(out, "Proceed with sync? [y/N] ")
	reader := bufio.NewReader(in)
	line, err := reader.ReadString('\n')
	if err != nil && len(line) == 0 {
		return &Error{Code: CodeConfirmationRequired, Message: "sync needs confirmation before writing settings", Exit: v2preview.ExitInputRequired, Details: map[string]any{"requiredAnswer": "yes"}}
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	if answer == "y" || answer == "yes" {
		return nil
	}
	return &Error{Code: CodeConfirmationRefused, Message: "sync was not confirmed; no settings were changed", Exit: v2preview.ExitInputRequired}
}

func revalidateCurrentPlan(report *Report, opts Options, writeIndexes []int) error {
	if len(writeIndexes) == 0 {
		return nil
	}
	current, err := buildPlan(opts)
	if err != nil {
		return err
	}
	currentByRef := map[string]Item{}
	for _, item := range current.Items {
		currentByRef[item.SettingRef] = item
	}
	freshWriteSet := baseReport(report.Selection.Ref)
	freshWriteIndexes := make([]int, 0, len(writeIndexes))
	for _, idx := range writeIndexes {
		planned := report.Items[idx]
		latest, ok := currentByRef[planned.SettingRef]
		if !ok || !sameExecutableWriteIdentity(planned, latest) || !isExecutableWrite(latest) {
			return &Error{Code: CodeStalePlan, Message: "settings changed since the plan was checked", Exit: v2preview.ExitValidation, Details: map[string]any{"settingRef": planned.SettingRef}}
		}
		freshWriteIndexes = append(freshWriteIndexes, len(freshWriteSet.Items))
		freshWriteSet.Items = append(freshWriteSet.Items, latest)
	}
	if err := validateWriteSetNoSharedBacking(freshWriteSet, freshWriteIndexes); err != nil {
		return err
	}
	return nil
}

func sameExecutableWriteIdentity(planned Item, latest Item) bool {
	return latest.EvidenceID == planned.EvidenceID &&
		latest.TargetRef == planned.TargetRef &&
		latest.SettingRef == planned.SettingRef &&
		latest.Scope == planned.Scope &&
		latest.Subject == planned.Subject &&
		latest.Decision == DecisionWrite &&
		latest.Direction == planned.Direction &&
		latest.BackingKey == planned.BackingKey
}

func preflightWriteSet(report *Report, opts Options, writeIndexes []int) error {
	builder := opts.PreviewBuilder
	if builder == nil {
		builder = selectedpreview.Build
	}
	for _, idx := range writeIndexes {
		item := report.Items[idx]
		command, err := internalCommandForDirection(item.Direction)
		if err != nil {
			return err
		}
		preflight, err := builder(selectedpreview.Options{
			Command:             command,
			ConfigPath:          opts.ConfigPath,
			RepoRoot:            opts.RepoRoot,
			StateRoot:           opts.StateRoot,
			Ref:                 item.SettingRef,
			MachineID:           opts.MachineID,
			UserID:              opts.UserID,
			ExtraLayers:         append([]string(nil), opts.ExtraLayers...),
			DryRun:              true,
			Confirmed:           true,
			LocationRoots:       opts.LocationRoots,
			MacOSDefaultsRunner: opts.MacOSDefaultsRunner,
			LifecycleDetector:   opts.LifecycleDetector,
		})
		if err != nil {
			return toSyncError(err, CodeUnsafePlan, "sync write preflight failed", v2preview.ExitValidation)
		}
		preflightItem, ok := findPreviewItem(preflight, item.SettingRef)
		if !ok || !preflightAllowsDirection(preflightItem, item.Direction) {
			return &Error{Code: CodeUnsafePlan, Message: "sync plan is no longer safe; no settings were changed", Exit: v2preview.ExitValidation, Details: map[string]any{"settingRef": item.SettingRef}}
		}
	}
	return nil
}

func executeWriteSet(report *Report, opts Options, writeIndexes []int) error {
	runner := opts.LiveRunner
	if runner == nil {
		runner = selectedlive.Run
	}
	var firstErr error
	stop := false
	for pos, idx := range writeIndexes {
		item := &report.Items[idx]
		if stop {
			item.Result = ResultNotAttempted
			continue
		}
		if err := revalidateCurrentPlan(report, opts, writeIndexes[pos:]); err != nil {
			item.Result = ResultNotAttempted
			if firstErr == nil {
				firstErr = &Error{Code: CodeExecutionFailed, Message: "sync stopped before the next write because settings changed", Exit: v2preview.ExitPartial, Details: map[string]any{"settingRef": item.SettingRef}}
			}
			stop = true
			continue
		}
		if err := preflightWriteSet(report, opts, []int{idx}); err != nil {
			item.Result = ResultNotAttempted
			if firstErr == nil {
				firstErr = &Error{Code: CodeExecutionFailed, Message: "sync stopped before the next write because the write preflight failed", Exit: v2preview.ExitPartial, Details: map[string]any{"settingRef": item.SettingRef}}
			}
			stop = true
			continue
		}
		command, err := internalCommandForDirection(item.Direction)
		if err != nil {
			item.Result = ResultFailed
			firstErr = err
			stop = true
			continue
		}
		result, err := runner(selectedlive.Options{
			Command:             command,
			ConfigPath:          opts.ConfigPath,
			RepoRoot:            opts.RepoRoot,
			StateRoot:           opts.StateRoot,
			Ref:                 item.SettingRef,
			MachineID:           opts.MachineID,
			UserID:              opts.UserID,
			ExtraLayers:         append([]string(nil), opts.ExtraLayers...),
			Confirmed:           true,
			NonInteractive:      true,
			JSONMode:            true,
			RunID:               runIDForItem(opts, item),
			Now:                 opts.Now,
			LocationRoots:       opts.LocationRoots,
			MacOSDefaultsRunner: opts.MacOSDefaultsRunner,
			LifecycleDetector:   opts.LifecycleDetector,
			LifecycleController: opts.LifecycleController,
			LifecyclePrompter:   nil,
		})
		verified := mergeLiveResult(item, result)
		if err != nil {
			item.Result = ResultFailed
			if item.Message == "" || strings.Contains(item.Message, "selected-value") {
				item.Message = failureMessageForDirection(item.Direction)
			}
			if firstErr == nil {
				firstErr = toSyncError(err, CodeExecutionFailed, "sync stopped after a failure", v2preview.ExitInternalError)
			}
			stop = true
			continue
		}
		if !verified {
			item.Result = ResultFailed
			item.Message = failureMessageForDirection(item.Direction)
			if firstErr == nil {
				firstErr = &Error{Code: CodeExecutionFailed, Message: "sync stopped because write verification did not succeed", Exit: v2preview.ExitInternalError, Details: map[string]any{"settingRef": item.SettingRef}}
			}
			stop = true
			continue
		}
		item.Result = ResultChanged
		item.Message = "Synced " + publicDirection(item.Direction) + "."
	}
	if firstErr != nil {
		if syncErr, ok := firstErr.(*Error); ok && (hasChangedResult(report) || hasNotAttemptedResult(report)) {
			syncErr.Exit = v2preview.ExitPartial
		}
		return firstErr
	}
	return nil
}

func internalCommandForDirection(direction string) (string, error) {
	switch direction {
	case DirectionLiveToStored:
		return selectedpreview.CommandSave, nil
	case DirectionStoredToLive:
		return selectedpreview.CommandApply, nil
	default:
		return "", &Error{Code: CodeInternalExecutionPlan, Message: "sync write has no executable direction", Exit: v2preview.ExitValidation, Details: map[string]any{"direction": direction}}
	}
}

func preflightAllowsDirection(item selectedpreview.Item, direction string) bool {
	switch direction {
	case DirectionLiveToStored:
		return selectedpreview.IsSavePlannedAction(item.PlannedAction)
	case DirectionStoredToLive:
		return item.PlannedAction == selectedpreview.PlannedActionWouldApply
	default:
		return false
	}
}

func findPreviewItem(report *selectedpreview.Report, ref string) (selectedpreview.Item, bool) {
	if report == nil {
		return selectedpreview.Item{}, false
	}
	for _, item := range report.Items {
		if item.SettingRef == ref {
			return item, true
		}
	}
	return selectedpreview.Item{}, false
}

func mergeLiveResult(item *Item, result *selectedlive.Result) bool {
	if item == nil || result == nil || result.Report == nil {
		return false
	}
	for _, liveItem := range result.Report.Items {
		if liveItem.SettingRef != item.SettingRef {
			continue
		}
		if liveItem.Mutation != nil {
			item.UnderlyingRunID = liveItem.Mutation.RunID
		}
		item.Diagnostics = append(item.Diagnostics, liveItem.Diagnostics...)
		if strings.TrimSpace(liveItem.Message) != "" {
			item.Message = publicMessage(liveItem)
		}
		return liveItem.Mutation != nil &&
			liveItem.Mutation.Result == string(v2ledger.ItemResultVerified) &&
			liveItem.Mutation.Verification.Verified
	}
	if result.Report.Error != nil {
		item.Diagnostics = append(item.Diagnostics, selectedpreview.Diagnostic{Code: result.Report.Error.Code, Severity: selectedpreview.SeverityError, Message: result.Report.Error.Message, Ref: item.SettingRef})
	}
	return false
}

func finishReport(report *Report) {
	if report == nil {
		return
	}
	summary := Summary{}
	for _, item := range report.Items {
		switch item.Result {
		case ResultChanged:
			summary.Changed++
		case ResultFailed:
			summary.Failed++
		case ResultNotAttempted:
			summary.NotAttempted++
		case ResultSkipped:
			summary.Skipped++
		}
		if item.Decision == DecisionNeedsChoice || item.ChoiceRequired {
			summary.NeedsChoice++
		}
		if item.Decision == DecisionBlocked {
			summary.Blocked++
		}
		if item.Result == ResultChanged || item.Result == ResultPending {
			switch item.Direction {
			case DirectionLiveToStored:
				summary.WritesToStoredSettings++
			case DirectionStoredToLive:
				summary.WritesToLiveSettings++
			}
		}
	}
	switch {
	case report.Error != nil:
		summary.Status = statusForError(report.Error.Code, summary)
	case summary.Failed > 0 || summary.NotAttempted > 0:
		summary.Status = StatusPartialExecutionError
	case summary.Changed > 0:
		summary.Status = StatusComplete
	case summary.NeedsChoice > 0:
		summary.Status = StatusNeedsChoice
	case summary.Blocked > 0:
		summary.Status = StatusBlocked
	default:
		summary.Status = StatusNoChanges
	}
	report.Summary = summary
}

func statusForError(code string, summary Summary) string {
	switch code {
	case CodeConfirmationRequired:
		return StatusConfirmationRequired
	case CodeConfirmationRefused:
		return StatusConfirmationRefused
	case CodeChoiceRequired:
		return StatusNeedsChoice
	case CodeBlocked:
		return StatusBlocked
	case CodeStalePlan:
		return StatusRefusedStalePlan
	case CodeUnsafePlan, CodeWriteSetInvalid:
		return StatusRefusedUnsafePlan
	case CodeExecutionFailed:
		if summary.Changed > 0 || summary.NotAttempted > 0 || summary.Failed > 0 {
			return StatusPartialExecutionError
		}
		return StatusError
	default:
		return StatusError
	}
}

func attachError(report *Report, err error) {
	if report == nil || err == nil {
		return
	}
	syncErr := toSyncError(err, CodeExecutionFailed, err.Error(), v2preview.ExitInternalError)
	report.Error = &ErrorObj{Code: syncErr.Code, Message: syncErr.Message, Details: syncErr.Details}
}

func toSyncError(err error, code string, fallbackMessage string, fallbackExit int) *Error {
	if err == nil {
		return &Error{Code: code, Message: fallbackMessage, Exit: fallbackExit}
	}
	if syncErr, ok := err.(*Error); ok {
		return syncErr
	}
	message := strings.TrimSpace(err.Error())
	if message == "" {
		message = fallbackMessage
	}
	exit := fallbackExit
	if exitErr, ok := err.(interface{ ExitCode() int }); ok {
		exit = exitErr.ExitCode()
	}
	if previewErr, ok := err.(*selectedpreview.Error); ok {
		return &Error{Code: previewErr.Code, Message: message, Exit: exit, Details: previewErr.Details}
	}
	return &Error{Code: code, Message: message, Exit: exit}
}

func evidenceID(item selectedpreview.Item) string {
	payload := struct {
		TargetRef      string
		SettingRef     string
		State          v2status.StateCode
		NoBaseline     bool
		DesiredStatus  string
		DesiredIntent  string
		DesiredKind    string
		DesiredExists  bool
		DesiredSHA256  string
		CurrentExists  bool
		CurrentSHA256  string
		CurrentNorm    string
		ResourceDriver string
		Selector       string
	}{
		TargetRef:      item.TargetRef,
		SettingRef:     item.SettingRef,
		State:          item.State,
		NoBaseline:     item.NoBaseline,
		DesiredStatus:  item.Desired.Status,
		DesiredIntent:  item.Desired.Intent,
		DesiredKind:    item.Desired.Kind,
		DesiredExists:  item.Desired.Snapshot.Exists,
		DesiredSHA256:  item.Desired.Snapshot.SHA256,
		CurrentExists:  item.Current.Exists,
		CurrentSHA256:  item.Current.SHA256,
		CurrentNorm:    item.Current.Normalizer,
		ResourceDriver: item.Resource.DriverID,
		Selector:       item.Selector.Summary,
	}
	body, _ := json.Marshal(payload)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])[:16]
}

func runIDForItem(opts Options, item *Item) string {
	if opts.Now == nil || item == nil {
		return ""
	}
	stamp := opts.Now().UTC().Format("20060102T150405Z")
	ref := strings.NewReplacer(":", "-", "/", "-", " ", "-").Replace(item.SettingRef)
	return "smart-sync-" + stamp + "-" + ref + "-" + item.Direction
}

func shouldRenderAcceptedPlan(report *Report) bool {
	if report == nil || !report.Confirmation.Required {
		return false
	}
	// Interactive prompts already printed the accepted plan before execution.
	if report.Confirmation.Source == ConfirmationSourcePrompt && report.Confirmation.Confirmed && report.Error == nil {
		return false
	}
	return len(executableWriteIndexes(report)) > 0
}

func acceptedPlanLines(report *Report) []string {
	indexes := executableWriteIndexes(report)
	lines := []string{"Sync plan accepted.", fmt.Sprintf("Will sync %d %s:", len(indexes), plural("setting", len(indexes)))}
	for _, idx := range indexes {
		item := report.Items[idx]
		lines = append(lines, fmt.Sprintf("- %s: %s", item.SettingRef, publicDirection(item.Direction)))
	}
	return lines
}

func headlineLines(report *Report) []string {
	if report.Error != nil {
		switch report.Error.Code {
		case CodeConfirmationRequired:
			return []string{"Sync not run: confirmation required.", "Run again with --yes after reviewing the plan, or run without --non-interactive to confirm at the prompt."}
		case CodeConfirmationRefused:
			return []string{"Sync not run: confirmation refused."}
		case CodeChoiceRequired:
			if hasConflict(report) {
				return []string{"Sync not run: conflict needs a choice."}
			}
			return []string{"Sync not run: a choice is required before settings can be changed."}
		case CodeStalePlan:
			return []string{"Sync not run: settings changed since the plan was checked.", "Run status again to review the current state before syncing."}
		case CodeUnsafePlan, CodeWriteSetInvalid:
			return []string{"Sync not run: the plan is no longer safe to execute."}
		case CodeBlocked:
			return []string{"Sync not run: no safe writes are available."}
		case CodeExecutionFailed:
			return []string{"Sync stopped after a failure."}
		default:
			return []string{"Sync not run: " + fallback(report.Error.Message, "the command failed") + "."}
		}
	}
	if report.Summary.Changed == 0 && report.Summary.Skipped == 0 && report.Summary.Failed == 0 {
		return []string{"Sync complete.", "No settings needed to change."}
	}
	return []string{"Sync complete."}
}

func resultSections(report *Report) []string {
	sections := []string{}
	sections = append(sections, sectionLines("Changed", report, ResultChanged)...)
	sections = append(sections, sectionLines("Failed", report, ResultFailed)...)
	sections = append(sections, sectionLines("Not attempted", report, ResultNotAttempted)...)
	sections = append(sections, sectionLines("Skipped", report, ResultSkipped)...)
	return trimTrailingBlank(sections)
}

func sectionLines(title string, report *Report, result string) []string {
	items := itemsWithResult(report, result)
	if len(items) == 0 {
		return nil
	}
	lines := []string{title + ":"}
	groups := groupItemsByTarget(items)
	targets := make([]string, 0, len(groups))
	for target := range groups {
		targets = append(targets, target)
	}
	sort.Strings(targets)
	for _, target := range targets {
		lines = append(lines, targetLabel(target))
		for _, item := range groups[target] {
			lines = append(lines, "  - "+item.SettingRef)
			switch result {
			case ResultChanged:
				lines = append(lines, "    Synced: "+publicDirection(item.Direction))
			case ResultFailed:
				lines = append(lines, "    Failed: "+failureMessageForDirection(item.Direction))
				lines = append(lines, "    Direction: "+publicDirection(item.Direction))
			case ResultNotAttempted:
				lines = append(lines, "    Not attempted because an earlier write failed.")
				lines = append(lines, "    Direction: "+publicDirection(item.Direction))
			case ResultSkipped:
				lines = append(lines, "    Skipped: "+skipReason(item))
				lines = append(lines, "    Direction: "+publicDirection(item.Direction))
			}
			if result == ResultChanged || item.ValuesRedacted {
				lines = append(lines, "    Values hidden for safety.")
			}
		}
		lines = append(lines, "")
	}
	return lines
}

func summaryLines(report *Report) []string {
	lines := []string{"Summary:", fmt.Sprintf("  Changed: %d", report.Summary.Changed), fmt.Sprintf("  Skipped: %d", report.Summary.Skipped), fmt.Sprintf("  Failed: %d", report.Summary.Failed)}
	if report.Summary.NotAttempted > 0 {
		lines = append(lines, fmt.Sprintf("  Not attempted: %d", report.Summary.NotAttempted))
	}
	return lines
}

func itemsWithResult(report *Report, result string) []Item {
	if report == nil {
		return nil
	}
	items := []Item{}
	for _, item := range report.Items {
		if item.Result == result {
			items = append(items, item)
		}
	}
	return items
}

func groupItemsByTarget(items []Item) map[string][]Item {
	groups := map[string][]Item{}
	for _, item := range items {
		key := item.TargetRef
		if key == "" {
			key = targetFromSettingRef(item.SettingRef)
		}
		groups[key] = append(groups[key], item)
	}
	for key := range groups {
		sort.SliceStable(groups[key], func(i, j int) bool {
			return groups[key][i].SettingRef < groups[key][j].SettingRef
		})
	}
	return groups
}

func hasConflict(report *Report) bool {
	if report == nil {
		return false
	}
	for _, item := range report.Items {
		if item.ReasonCode == "both-sides-changed" || item.State == "conflict" {
			return true
		}
	}
	return false
}

func hasChangedResult(report *Report) bool {
	if report == nil {
		return false
	}
	for _, item := range report.Items {
		if item.Result == ResultChanged {
			return true
		}
	}
	return false
}

func hasNotAttemptedResult(report *Report) bool {
	if report == nil {
		return false
	}
	for _, item := range report.Items {
		if item.Result == ResultNotAttempted {
			return true
		}
	}
	return false
}

func backingKey(item selectedpreview.Item) string {
	parts := []string{
		strings.TrimSpace(item.TargetRef),
		strings.TrimSpace(item.Resource.ID),
		strings.TrimSpace(item.Resource.DriverID),
		strings.TrimSpace(item.Resource.Path),
		strings.TrimSpace(item.DesiredURI),
		filepathSlash(item.DesiredRelPath),
	}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, "\x00")
}

func filepathSlash(path string) string {
	return strings.ReplaceAll(strings.TrimSpace(path), "\\", "/")
}

func publicDirection(direction string) string {
	switch direction {
	case DirectionLiveToStored:
		return "live settings -> stored settings"
	case DirectionStoredToLive:
		return "stored settings -> live settings"
	case DirectionBothSidesChange:
		return "both sides changed"
	case DirectionNone:
		return "not planned"
	default:
		return "no safe direction"
	}
}

func skipReason(item Item) string {
	switch item.Decision {
	case DecisionNeedsChoice:
		if item.ReasonCode == "both-sides-changed" {
			return "conflict needs a choice."
		}
		return "choice required before sync can continue."
	case DecisionBlocked:
		if strings.TrimSpace(item.Message) != "" {
			return strings.TrimSuffix(item.Message, ".") + "."
		}
		return "cannot plan safely."
	default:
		if strings.TrimSpace(item.Message) != "" {
			return strings.TrimSuffix(item.Message, ".") + "."
		}
		return "no write was planned."
	}
}

func failureMessageForDirection(direction string) string {
	if direction == DirectionStoredToLive {
		return "live settings could not be written."
	}
	return "stored settings could not be written."
}

func targetLabel(target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return "Settings"
	}
	parts := strings.FieldsFunc(target, func(r rune) bool { return r == '.' || r == '-' || r == '_' || r == ':' || r == '/' })
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	label := strings.Join(parts, " ")
	if strings.TrimSpace(label) == "" {
		return target
	}
	return label
}

func targetFromSettingRef(ref string) string {
	if before, _, ok := strings.Cut(ref, ":"); ok {
		return before
	}
	return ref
}

func fallback(value string, fallbackValue string) string {
	if strings.TrimSpace(value) == "" {
		return fallbackValue
	}
	return strings.TrimSpace(value)
}

func plural(word string, count int) string {
	if count == 1 {
		return word
	}
	return word + "s"
}

func trimTrailingBlank(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
