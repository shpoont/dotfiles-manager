// Package guidedsync implements the v2 guided sync planner/executor.
//
// Guided sync is intentionally not a merge engine. It turns the existing
// selected-preview state into explicit per-setting save/apply/skip decisions
// and then composes the existing selected-live execution path for the choices
// the user confirmed.
package guidedsync

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/shpoont/dotfiles-manager/internal/v2/lifecycle"
	"github.com/shpoont/dotfiles-manager/internal/v2/macosdefaultsdriver"
	v2preview "github.com/shpoont/dotfiles-manager/internal/v2/preview"
	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/shpoont/dotfiles-manager/internal/v2/selectedlive"
	"github.com/shpoont/dotfiles-manager/internal/v2/selectedpreview"
	v2status "github.com/shpoont/dotfiles-manager/internal/v2/status"
)

const (
	Schema        = "dotfiles-manager.v2.guided-sync"
	SchemaVersion = 1
	Command       = "sync"
	RunID         = "guided-sync"
)

const (
	ChoiceSave  = "save"
	ChoiceApply = "apply"
	ChoiceSkip  = "skip"
)

const (
	ChoiceSourceFlag   = "flag"
	ChoiceSourcePrompt = "prompt"
)

const (
	OutcomePlanned      = "planned"
	OutcomeChosen       = "chosen"
	OutcomeExecuted     = "executed"
	OutcomeSkipped      = "skipped"
	OutcomeBlocked      = "blocked"
	OutcomeFailed       = "failed"
	OutcomeNotAttempted = "not_attempted"
)

const (
	SummaryOK          = "ok"
	SummaryNeedsChoice = "needs-choice"
	SummaryChanged     = "changed"
	SummaryBlocked     = "blocked"
	SummaryPartial     = "partial"
	SummaryError       = "error"
)

const (
	CodeChoiceInvalid         = "guidedsync.choice.invalid"
	CodeChoiceDuplicate       = "guidedsync.choice.duplicate"
	CodeChoiceUnknownRef      = "guidedsync.choice.unknownRef"
	CodeChoiceNotAllowed      = "guidedsync.choice.notAllowed"
	CodeChoiceRequired        = "guidedsync.choice.required"
	CodePromptRequired        = "guidedsync.prompt.required"
	CodeConfirmationRequired  = "guidedsync.confirmationRequired"
	CodeExecutionFailed       = "guidedsync.executionFailed"
	CodeFileTreeApplyDeferred = "guidedsync.fileTreeApply.deferred"
)

// PreviewBuilder exists so tests can exercise conflict and failure branches
// without manufacturing a full live repository state for every status.
type PreviewBuilder func(selectedpreview.Options) (*selectedpreview.Report, error)

type LiveRunner func(selectedlive.Options) (*selectedlive.Result, error)

type Options struct {
	RepoRoot       string
	StateRoot      string
	Ref            string
	MachineID      string
	UserID         string
	ExtraLayers    []string
	Choices        []Choice
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

type Choice struct {
	Ref    string `json:"ref"`
	Action string `json:"action"`
}

type Report struct {
	Schema        string    `json:"schema"`
	SchemaVersion int       `json:"schemaVersion"`
	Command       string    `json:"command"`
	RunID         string    `json:"runId"`
	ProfileStack  []string  `json:"profileStack"`
	Summary       Summary   `json:"summary"`
	Items         []Item    `json:"items"`
	Error         *ErrorObj `json:"error,omitempty"`
}

type Summary struct {
	Status       string `json:"status"`
	Planned      int    `json:"planned"`
	NeedsChoice  int    `json:"needsChoice"`
	Chosen       int    `json:"chosen"`
	Executed     int    `json:"executed"`
	Skipped      int    `json:"skipped"`
	Blocked      int    `json:"blocked"`
	Failed       int    `json:"failed"`
	NotAttempted int    `json:"notAttempted"`
	Changed      int    `json:"changed"`
}

type ErrorObj struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type Item struct {
	TargetRef       string                        `json:"targetRef"`
	SettingRef      string                        `json:"settingRef"`
	Scope           string                        `json:"scope"`
	Subject         string                        `json:"subject"`
	SourceLayer     string                        `json:"sourceLayer,omitempty"`
	State           v2status.StateCode            `json:"state"`
	NoBaseline      bool                          `json:"noBaseline,omitempty"`
	Message         string                        `json:"message,omitempty"`
	Recommended     string                        `json:"recommendedAction,omitempty"`
	AllowedChoices  []string                      `json:"allowedChoices"`
	SelectedChoice  string                        `json:"selectedChoice,omitempty"`
	ChoiceSource    string                        `json:"choiceSource,omitempty"`
	Outcome         string                        `json:"outcome"`
	DesiredURI      string                        `json:"desiredUri,omitempty"`
	DesiredRelPath  string                        `json:"desiredRelPath,omitempty"`
	Resource        selectedpreview.ResourceInfo  `json:"resource"`
	Selector        selectedpreview.SelectorInfo  `json:"selector"`
	Diff            *selectedpreview.DiffInfo     `json:"diff,omitempty"`
	Mutation        *selectedpreview.MutationInfo `json:"mutation,omitempty"`
	UnderlyingRunID string                        `json:"underlyingRunId,omitempty"`
	BackupRefs      []string                      `json:"backupRefs,omitempty"`
	Diagnostics     []selectedpreview.Diagnostic  `json:"diagnostics,omitempty"`
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

func ParseChoice(raw string) (Choice, error) {
	trimmed := strings.TrimSpace(raw)
	parts := strings.Split(trimmed, "=")
	if len(parts) != 2 {
		return Choice{}, &Error{Code: CodeChoiceInvalid, Message: "choice must use setting-ref=save|apply|skip", Exit: v2preview.ExitValidation, Details: map[string]any{"choice": raw}}
	}
	ref := strings.TrimSpace(parts[0])
	action := normalizeChoice(parts[1])
	if ref == "" || !isKnownChoice(action) {
		return Choice{}, &Error{Code: CodeChoiceInvalid, Message: "choice must use setting-ref=save|apply|skip", Exit: v2preview.ExitValidation, Details: map[string]any{"choice": raw}}
	}
	return Choice{Ref: ref, Action: action}, nil
}

func Run(opts Options) (*Report, error) {
	report, err := buildPlan(opts)
	if err != nil {
		return report, err
	}
	if err := applyFlagChoices(report, opts.Choices); err != nil {
		attachError(report, err)
		finishReport(report)
		return report, err
	}

	prompted := false
	if shouldPrompt(opts, report) {
		if err := promptForChoices(report, opts); err != nil {
			attachError(report, err)
			finishReport(report)
			return report, err
		}
		prompted = true
	}

	execute := prompted || opts.Confirmed
	if execute {
		if err := requireAllChoices(report); err != nil {
			attachError(report, err)
			finishReport(report)
			return report, err
		}
	}

	if !execute && hasMutatingChoice(report) {
		err := &Error{Code: CodeConfirmationRequired, Message: "guided sync choices require --yes before non-interactive mutation", Exit: v2preview.ExitInputRequired, Details: map[string]any{"requiredFlag": "--yes"}}
		attachError(report, err)
		finishReport(report)
		return report, err
	}

	if execute {
		if err := executeChoices(report, opts); err != nil {
			attachError(report, err)
			finishReport(report)
			return report, err
		}
	}

	finishReport(report)
	return report, nil
}

func JSON(report *Report) (string, error) {
	if report == nil {
		report = baseReport()
		report.Summary.Status = SummaryError
	}
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	return string(payload) + "\n", nil
}

func ErrorReport(code string, message string, details map[string]any) *Report {
	report := baseReport()
	report.Error = &ErrorObj{Code: code, Message: message, Details: details}
	finishReport(report)
	return report
}

func Text(report *Report) string {
	if report == nil {
		return "guided sync\nsummary status=error planned=0 needs-choice=0"
	}
	lines := []string{"guided sync"}
	if len(report.ProfileStack) > 0 {
		lines = append(lines, "profile: "+strings.Join(report.ProfileStack, " -> "))
	}
	for _, item := range report.Items {
		line := fmt.Sprintf("  %s scope=%s subject=%s state=%s outcome=%s", item.SettingRef, item.Scope, item.Subject, item.State, item.Outcome)
		if item.Recommended != "" {
			line += " recommended=" + item.Recommended
		}
		if item.SelectedChoice != "" {
			line += " choice=" + item.SelectedChoice
			if item.ChoiceSource != "" {
				line += "(" + item.ChoiceSource + ")"
			}
		}
		if item.NoBaseline {
			line += " no-baseline"
		}
		lines = append(lines, line)
		if len(item.AllowedChoices) > 0 {
			lines = append(lines, "    choices="+strings.Join(item.AllowedChoices, ","))
		}
		if item.Message != "" {
			lines = append(lines, "    message: "+item.Message)
		}
		if item.Resource.DriverID != "" {
			lines = append(lines, fmt.Sprintf("    resource=%s driver=%s selector=%s", item.Resource.ID, item.Resource.DriverID, item.Selector.Summary))
		}
		if item.Diff != nil {
			lines = append(lines, fmt.Sprintf("    diff=%s mode=%s redaction=%s", item.Diff.Kind, item.Diff.Mode, item.Diff.Redaction))
		}
		if item.UnderlyingRunID != "" {
			lines = append(lines, "    underlying-run="+item.UnderlyingRunID)
		}
		if len(item.BackupRefs) > 0 {
			lines = append(lines, "    backups="+strings.Join(item.BackupRefs, ","))
		}
		for _, diagnostic := range item.Diagnostics {
			lines = append(lines, fmt.Sprintf("    %s[%s]: %s", diagnostic.Severity, diagnostic.Code, diagnostic.Message))
		}
	}
	if report.Error != nil {
		lines = append(lines, fmt.Sprintf("error[%s]: %s", report.Error.Code, report.Error.Message))
	}
	lines = append(lines, fmt.Sprintf("summary status=%s planned=%d needs-choice=%d chosen=%d executed=%d skipped=%d blocked=%d failed=%d not-attempted=%d", report.Summary.Status, report.Summary.Planned, report.Summary.NeedsChoice, report.Summary.Chosen, report.Summary.Executed, report.Summary.Skipped, report.Summary.Blocked, report.Summary.Failed, report.Summary.NotAttempted))
	return strings.Join(lines, "\n")
}

func buildPlan(opts Options) (*Report, error) {
	builder := opts.PreviewBuilder
	if builder == nil {
		builder = selectedpreview.Build
	}
	previewReport, err := builder(selectedpreview.Options{
		Command:             selectedpreview.CommandStatus,
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
	report := baseReport()
	if previewReport != nil {
		report.ProfileStack = append([]string(nil), previewReport.ProfileStack...)
		for _, item := range previewReport.Items {
			report.Items = append(report.Items, fromPreviewItem(item))
		}
		if previewReport.Error != nil {
			report.Error = &ErrorObj{Code: previewReport.Error.Code, Message: previewReport.Error.Message, Details: previewReport.Error.Details}
		}
	}
	finishReport(report)
	if err != nil {
		wrapped := toGuidedError(err, "guided sync planning failed", v2preview.ExitValidation)
		if report.Error == nil {
			report.Error = &ErrorObj{Code: wrapped.Code, Message: wrapped.Message, Details: wrapped.Details}
		}
		report.Summary.Status = SummaryError
		return report, wrapped
	}
	return report, nil
}

func baseReport() *Report {
	return &Report{Schema: Schema, SchemaVersion: SchemaVersion, Command: Command, RunID: RunID, Items: []Item{}}
}

func fromPreviewItem(item selectedpreview.Item) Item {
	outcome := OutcomePlanned
	if isBlockedState(item.State) {
		outcome = OutcomeBlocked
	}
	out := Item{
		TargetRef:      item.TargetRef,
		SettingRef:     item.SettingRef,
		Scope:          item.Scope,
		Subject:        item.Subject,
		SourceLayer:    item.SourceLayer,
		State:          item.State,
		NoBaseline:     item.NoBaseline,
		Message:        item.Message,
		Recommended:    recommendedAction(item),
		AllowedChoices: allowedChoices(item),
		Outcome:        outcome,
		DesiredURI:     item.DesiredURI,
		DesiredRelPath: item.DesiredRelPath,
		Resource:       item.Resource,
		Selector:       item.Selector,
		Diff:           item.Diff,
		Diagnostics:    append([]selectedpreview.Diagnostic(nil), item.Diagnostics...),
	}
	applyFileTreeSyncRestrictions(&out)
	return out
}

func applyFileTreeSyncRestrictions(item *Item) {
	if item == nil || item.Resource.DriverID != recipe.FileTreeDriverID || !choiceAllowed(ChoiceApply, item.AllowedChoices) {
		return
	}

	item.AllowedChoices = removeChoice(item.AllowedChoices, ChoiceApply)
	if item.Recommended == ChoiceApply {
		item.Recommended = ""
	}
	if item.Message != "" {
		item.Message += " "
	}
	item.Message += "File-tree apply is deferred in sync; use explicit apply --dry-run/--yes so removals are shown before confirmation."
	item.Diagnostics = append(item.Diagnostics, selectedpreview.Diagnostic{
		Code:       CodeFileTreeApplyDeferred,
		Severity:   selectedpreview.SeverityWarning,
		Message:    "guided sync file-tree apply choices are deferred; use explicit apply --dry-run and apply --yes so pending removals are shown before confirmation",
		Ref:        item.SettingRef,
		ResourceID: item.Resource.ID,
		DriverID:   item.Resource.DriverID,
	})
}

func removeChoice(choices []string, remove string) []string {
	out := make([]string, 0, len(choices))
	for _, choice := range choices {
		if choice != remove {
			out = append(out, choice)
		}
	}
	return out
}

func applyFlagChoices(report *Report, choices []Choice) error {
	if len(choices) == 0 {
		return nil
	}
	itemsByRef := map[string]*Item{}
	for idx := range report.Items {
		itemsByRef[report.Items[idx].SettingRef] = &report.Items[idx]
	}
	seen := map[string]struct{}{}
	for _, choice := range choices {
		ref := strings.TrimSpace(choice.Ref)
		action := normalizeChoice(choice.Action)
		if ref == "" || !isKnownChoice(action) {
			return &Error{Code: CodeChoiceInvalid, Message: "choice must use setting-ref=save|apply|skip", Exit: v2preview.ExitValidation, Details: map[string]any{"ref": choice.Ref, "action": choice.Action}}
		}
		if _, exists := seen[ref]; exists {
			return &Error{Code: CodeChoiceDuplicate, Message: "duplicate guided sync choice", Exit: v2preview.ExitValidation, Details: map[string]any{"ref": ref}}
		}
		seen[ref] = struct{}{}
		item, ok := itemsByRef[ref]
		if !ok {
			return &Error{Code: CodeChoiceUnknownRef, Message: "guided sync choice references an unknown setting", Exit: v2preview.ExitValidation, Details: map[string]any{"ref": ref}}
		}
		if !choiceAllowed(action, item.AllowedChoices) {
			return &Error{Code: CodeChoiceNotAllowed, Message: "guided sync choice is not allowed for this item state", Exit: v2preview.ExitValidation, Details: map[string]any{"ref": ref, "action": action, "state": item.State, "allowedChoices": item.AllowedChoices}}
		}
		item.SelectedChoice = action
		item.ChoiceSource = ChoiceSourceFlag
		if action == ChoiceSkip {
			item.Outcome = OutcomeSkipped
		} else {
			item.Outcome = OutcomeChosen
		}
	}
	return nil
}

func shouldPrompt(opts Options, report *Report) bool {
	if opts.JSONMode || opts.NonInteractive || opts.Confirmed || len(opts.Choices) > 0 {
		return false
	}
	for _, item := range report.Items {
		if requiresChoice(item) && item.SelectedChoice == "" {
			return true
		}
	}
	return false
}

func promptForChoices(report *Report, opts Options) error {
	in := opts.In
	if in == nil {
		in = strings.NewReader("")
	}
	out := opts.PromptOut
	if out == nil {
		out = io.Discard
	}
	reader := bufio.NewReader(in)
	for idx := range report.Items {
		item := &report.Items[idx]
		if !requiresChoice(*item) || item.SelectedChoice != "" {
			continue
		}
		_, _ = fmt.Fprintf(out, "%s state=%s choices=%s. Choosing save/apply will mutate this item. Choose: ", item.SettingRef, item.State, strings.Join(item.AllowedChoices, "/"))
		line, err := reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			return &Error{Code: CodePromptRequired, Message: "guided sync prompt requires an explicit choice before any writes", Exit: v2preview.ExitInputRequired, Details: map[string]any{"ref": item.SettingRef}}
		}
		choice := normalizeChoice(line)
		if !choiceAllowed(choice, item.AllowedChoices) {
			return &Error{Code: CodeChoiceInvalid, Message: "guided sync prompt choice is invalid; no writes were attempted", Exit: v2preview.ExitValidation, Details: map[string]any{"ref": item.SettingRef, "choice": strings.TrimSpace(line), "allowedChoices": item.AllowedChoices}}
		}
		item.SelectedChoice = choice
		item.ChoiceSource = ChoiceSourcePrompt
		if choice == ChoiceSkip {
			item.Outcome = OutcomeSkipped
		} else {
			item.Outcome = OutcomeChosen
		}
	}
	return nil
}

func requireAllChoices(report *Report) error {
	missing := []string{}
	for _, item := range report.Items {
		if requiresChoice(item) && item.SelectedChoice == "" {
			missing = append(missing, item.SettingRef)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return &Error{Code: CodeChoiceRequired, Message: "guided sync execution requires explicit save/apply/skip choices for all actionable items", Exit: v2preview.ExitInputRequired, Details: map[string]any{"missingChoices": missing}}
}

func executeChoices(report *Report, opts Options) error {
	runner := opts.LiveRunner
	if runner == nil {
		runner = selectedlive.Run
	}
	var firstErr error
	stopMutations := false
	for idx := range report.Items {
		item := &report.Items[idx]
		switch item.SelectedChoice {
		case "":
			continue
		case ChoiceSkip:
			item.Outcome = OutcomeSkipped
			continue
		}
		if stopMutations {
			item.Outcome = OutcomeNotAttempted
			continue
		}
		result, err := runner(selectedlive.Options{
			Command:             item.SelectedChoice,
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
		mergeLiveResult(item, result)
		if err != nil {
			item.Outcome = OutcomeFailed
			if firstErr == nil {
				firstErr = toGuidedError(err, "guided sync execution failed; later choices were not attempted", v2preview.ExitInternalError)
			}
			stopMutations = true
			continue
		}
		item.Outcome = OutcomeExecuted
	}
	if firstErr != nil {
		if hasExecutedItem(report) || hasNotAttemptedItem(report) {
			if guidedErr, ok := firstErr.(*Error); ok {
				guidedErr.Exit = v2preview.ExitPartial
			}
		}
		return firstErr
	}
	return nil
}

func mergeLiveResult(item *Item, result *selectedlive.Result) {
	if result == nil || result.Report == nil {
		return
	}
	for _, liveItem := range result.Report.Items {
		if liveItem.SettingRef != item.SettingRef {
			continue
		}
		item.Mutation = liveItem.Mutation
		if liveItem.Mutation != nil {
			item.UnderlyingRunID = liveItem.Mutation.RunID
			item.BackupRefs = append([]string(nil), liveItem.Mutation.BackupRefs...)
		}
		item.Diagnostics = append(item.Diagnostics, liveItem.Diagnostics...)
		item.Message = liveItem.Message
		item.State = liveItem.State
		return
	}
	if result.Report.Error != nil {
		item.Diagnostics = append(item.Diagnostics, selectedpreview.Diagnostic{Code: result.Report.Error.Code, Severity: selectedpreview.SeverityError, Message: result.Report.Error.Message, Ref: item.SettingRef})
	}
}

func runIDForItem(opts Options, item *Item) string {
	if opts.Now == nil {
		return ""
	}
	stamp := opts.Now().UTC().Format("20060102T150405Z")
	ref := strings.NewReplacer(":", "-", "/", "-", " ", "-").Replace(item.SettingRef)
	return "guided-sync-" + stamp + "-" + ref + "-" + item.SelectedChoice
}

func attachError(report *Report, err error) {
	if report == nil || err == nil {
		return
	}
	guided := toGuidedError(err, err.Error(), v2preview.ExitInternalError)
	report.Error = &ErrorObj{Code: guided.Code, Message: guided.Message, Details: guided.Details}
	report.Summary.Status = SummaryError
}

func finishReport(report *Report) {
	if report == nil {
		return
	}
	summary := Summary{}
	for _, item := range report.Items {
		switch item.Outcome {
		case OutcomePlanned:
			summary.Planned++
		case OutcomeChosen:
			summary.Chosen++
		case OutcomeExecuted:
			summary.Executed++
		case OutcomeSkipped:
			summary.Skipped++
		case OutcomeBlocked:
			summary.Blocked++
		case OutcomeFailed:
			summary.Failed++
		case OutcomeNotAttempted:
			summary.NotAttempted++
		}
		if requiresChoice(item) && item.SelectedChoice == "" {
			summary.NeedsChoice++
		}
		if item.State != v2status.StateUnchanged {
			summary.Changed++
		}
	}
	switch {
	case report.Error != nil && (summary.Executed > 0 || summary.NotAttempted > 0):
		summary.Status = SummaryPartial
	case report.Error != nil:
		summary.Status = SummaryError
	case summary.Failed > 0 || summary.NotAttempted > 0:
		summary.Status = SummaryPartial
	case summary.NeedsChoice > 0:
		summary.Status = SummaryNeedsChoice
	case summary.Blocked > 0 && summary.Executed == 0 && summary.Chosen == 0:
		summary.Status = SummaryBlocked
	case summary.Changed > 0 || summary.Executed > 0 || summary.Skipped > 0 || summary.Chosen > 0:
		summary.Status = SummaryChanged
	default:
		summary.Status = SummaryOK
	}
	report.Summary = summary
}

func recommendedAction(item selectedpreview.Item) string {
	switch item.State {
	case v2status.StateChangedCurrent, v2status.StateMissingDesired:
		return ChoiceSave
	case v2status.StateReadyToApply, v2status.StateMissingCurrent:
		return ChoiceApply
	case v2status.StateUnchanged:
		return ChoiceSkip
	default:
		return ""
	}
}

func allowedChoices(item selectedpreview.Item) []string {
	if isBlockedState(item.State) {
		return []string{}
	}
	switch item.State {
	case v2status.StateUnchanged:
		return []string{ChoiceSkip}
	case v2status.StateChangedCurrent:
		return []string{ChoiceSave, ChoiceApply, ChoiceSkip}
	case v2status.StateReadyToApply:
		return []string{ChoiceApply, ChoiceSkip}
	case v2status.StateMissingDesired:
		return []string{ChoiceSave, ChoiceSkip}
	case v2status.StateMissingCurrent:
		return []string{ChoiceApply, ChoiceSkip}
	case v2status.StateConflict, v2status.StateOpaqueChanged:
		return []string{ChoiceSave, ChoiceApply, ChoiceSkip}
	case v2status.StateUnknown:
		if item.NoBaseline {
			return []string{ChoiceSave, ChoiceApply, ChoiceSkip}
		}
	}
	return []string{}
}

func requiresChoice(item Item) bool {
	if item.State == v2status.StateUnchanged || isBlockedState(item.State) {
		return false
	}
	return len(item.AllowedChoices) > 0
}

func isBlockedState(state v2status.StateCode) bool {
	switch state {
	case v2status.StateBlockedSafety, v2status.StateBlockedLifecycle, v2status.StateUnsupported:
		return true
	default:
		return false
	}
}

func normalizeChoice(choice string) string {
	return strings.ToLower(strings.TrimSpace(choice))
}

func isKnownChoice(choice string) bool {
	switch choice {
	case ChoiceSave, ChoiceApply, ChoiceSkip:
		return true
	default:
		return false
	}
}

func choiceAllowed(choice string, allowed []string) bool {
	for _, candidate := range allowed {
		if choice == candidate {
			return true
		}
	}
	return false
}

func hasMutatingChoice(report *Report) bool {
	if report == nil {
		return false
	}
	for _, item := range report.Items {
		if item.SelectedChoice == ChoiceSave || item.SelectedChoice == ChoiceApply {
			return true
		}
	}
	return false
}

func hasExecutedItem(report *Report) bool {
	if report == nil {
		return false
	}
	for _, item := range report.Items {
		if item.Outcome == OutcomeExecuted {
			return true
		}
	}
	return false
}

func hasNotAttemptedItem(report *Report) bool {
	if report == nil {
		return false
	}
	for _, item := range report.Items {
		if item.Outcome == OutcomeNotAttempted {
			return true
		}
	}
	return false
}

func toGuidedError(err error, fallbackMessage string, fallbackExit int) *Error {
	if err == nil {
		return &Error{Code: CodeExecutionFailed, Message: fallbackMessage, Exit: fallbackExit}
	}
	if guided, ok := err.(*Error); ok {
		return guided
	}
	message := err.Error()
	if strings.TrimSpace(message) == "" {
		message = fallbackMessage
	}
	exit := fallbackExit
	if exitErr, ok := err.(interface{ ExitCode() int }); ok {
		exit = exitErr.ExitCode()
	}
	code := CodeExecutionFailed
	if previewErr, ok := err.(*selectedpreview.Error); ok {
		code = previewErr.Code
		if len(previewErr.Details) > 0 {
			return &Error{Code: code, Message: message, Exit: exit, Details: previewErr.Details}
		}
	}
	return &Error{Code: code, Message: message, Exit: exit}
}
