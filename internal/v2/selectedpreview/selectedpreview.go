package selectedpreview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/shpoont/dotfiles-manager/internal/v2/customfiles"
	"github.com/shpoont/dotfiles-manager/internal/v2/desired"
	"github.com/shpoont/dotfiles-manager/internal/v2/filedriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/filetreedriver"
	v2ledger "github.com/shpoont/dotfiles-manager/internal/v2/ledger"
	"github.com/shpoont/dotfiles-manager/internal/v2/lifecycle"
	"github.com/shpoont/dotfiles-manager/internal/v2/macosdefaultsdriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/nativeapply"
	"github.com/shpoont/dotfiles-manager/internal/v2/nativeexport"
	"github.com/shpoont/dotfiles-manager/internal/v2/nativeops"
	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/shpoont/dotfiles-manager/internal/v2/resolution"
	"github.com/shpoont/dotfiles-manager/internal/v2/selectedvalue"
	v2status "github.com/shpoont/dotfiles-manager/internal/v2/status"
)

const (
	Schema        = "dotfiles-manager.v2.preview"
	SchemaVersion = 1
	RunID         = "selected-value-preview"

	CommandStatus = "status"
	CommandDiff   = "diff"
	CommandSync   = "sync"
	CommandSave   = "save"
	CommandApply  = "apply"

	SummaryOK      = "ok"
	SummaryChanged = "changed"
	SummaryBlocked = "blocked"
	SummaryError   = "error"

	SeverityError   = "error"
	SeverityWarning = "warning"
	SeverityInfo    = "info"
)

const (
	PlannedActionNone                  = "none"
	PlannedActionWouldSave             = "would-save"
	PlannedActionWouldPromote          = "would-promote"
	PlannedActionWouldApply            = "would-apply"
	PlannedActionBlockedMissingDesired = "blocked-missing-desired"
)

type Options struct {
	Command             string
	ConfigPath          string
	RepoRoot            string
	StateRoot           string
	Ref                 string
	MachineID           string
	UserID              string
	ExtraLayers         []string
	DryRun              bool
	LocationRoots       map[string]map[string]string
	MacOSDefaultsRunner macosdefaultsdriver.Runner
	Confirmed           bool
	RunID               string
	Now                 func() time.Time
	NativeResolver      nativeops.ExecutableResolver
	NativeExecutor      nativeops.Executor
	LifecycleDetector   lifecycle.Detector
}

type Report struct {
	Schema         string            `json:"schema"`
	SchemaVersion  int               `json:"schemaVersion"`
	Command        string            `json:"command"`
	Operation      string            `json:"operation,omitempty"`
	InvokedCommand string            `json:"invokedCommand,omitempty"`
	Direction      string            `json:"direction,omitempty"`
	RunID          string            `json:"runId"`
	DryRun         bool              `json:"dryRun"`
	ProfileStack   []string          `json:"profileStack"`
	Summary        Summary           `json:"summary"`
	Items          []Item            `json:"items"`
	Error          *ErrorObj         `json:"error,omitempty"`
	Invocation     InvocationContext `json:"-"`
}

type Summary struct {
	Status  string `json:"status"`
	Changed int    `json:"changed"`
	Blocked int    `json:"blocked"`
	Applied int    `json:"applied"`
	Saved   int    `json:"saved"`
	Skipped int    `json:"skipped"`
	Failed  int    `json:"failed"`
}

type InvocationContext struct {
	ConfigPath  string
	MachineID   string
	UserID      string
	ExtraLayers []string
}

type ErrorObj struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type Item struct {
	TargetRef      string                   `json:"targetRef"`
	SettingRef     string                   `json:"settingRef"`
	Scope          string                   `json:"scope"`
	Subject        string                   `json:"subject"`
	SourceLayer    string                   `json:"sourceLayer"`
	DesiredURI     string                   `json:"desiredUri"`
	DesiredRelPath string                   `json:"desiredRelPath"`
	Recipe         RecipeInfo               `json:"recipe"`
	Resource       ResourceInfo             `json:"resource"`
	Selector       SelectorInfo             `json:"selector"`
	Desired        DesiredInfo              `json:"desired"`
	Current        Snapshot                 `json:"current"`
	Preview        *PreviewInfo             `json:"preview,omitempty"`
	Diff           *DiffInfo                `json:"diff,omitempty"`
	State          v2status.StateCode       `json:"state"`
	NoBaseline     bool                     `json:"noBaseline"`
	Message        string                   `json:"message"`
	AllowedActions []v2status.Action        `json:"allowedActions"`
	PlannedAction  string                   `json:"plannedAction,omitempty"`
	DryRun         bool                     `json:"dryRun"`
	Mutated        bool                     `json:"mutated"`
	Mutation       *MutationInfo            `json:"mutation,omitempty"`
	FileTree       *FileTreeInfo            `json:"fileTree,omitempty"`
	NativeExport   *NativeExportInfo        `json:"nativeExport,omitempty"`
	Lifecycle      []lifecycle.ActionRecord `json:"lifecycle,omitempty"`
	Diagnostics    []Diagnostic             `json:"diagnostics"`
}

type RecipeInfo struct {
	Source      string `json:"source"`
	RecipeRef   string `json:"recipeRef"`
	TrustStatus string `json:"trustStatus"`
}

type ResourceInfo struct {
	ID          string `json:"id"`
	DriverID    string `json:"driverId"`
	LocationID  string `json:"locationId"`
	RelPath     string `json:"relPath"`
	Path        string `json:"path,omitempty"`
	DisplayPath string `json:"displayPath,omitempty"`
}

type SelectorInfo struct {
	Kind    string   `json:"kind"`
	Summary string   `json:"summary"`
	Section string   `json:"section,omitempty"`
	Key     string   `json:"key,omitempty"`
	Path    []string `json:"path,omitempty"`
}

type DesiredInfo struct {
	Status    string   `json:"status"`
	Intent    string   `json:"intent,omitempty"`
	Kind      string   `json:"kind,omitempty"`
	Snapshot  Snapshot `json:"snapshot"`
	Unmanaged bool     `json:"unmanaged"`
}

type Snapshot struct {
	Exists     bool   `json:"exists"`
	SHA256     string `json:"sha256,omitempty"`
	Normalizer string `json:"normalizer,omitempty"`
	Size       int    `json:"size,omitempty"`
	EntryCount int    `json:"entryCount,omitempty"`
	FileCount  int    `json:"fileCount,omitempty"`
	DirCount   int    `json:"dirCount,omitempty"`
}

type PreviewInfo struct {
	ChangeKind string `json:"changeKind"`
	Intent     string `json:"intent,omitempty"`
	ReadOnly   bool   `json:"readOnly,omitempty"`
}

type DiffInfo struct {
	Kind      string `json:"kind"`
	Mode      string `json:"mode"`
	Redaction string `json:"redaction"`
	Message   string `json:"message"`
}

type Diagnostic struct {
	Code       string `json:"code"`
	Severity   string `json:"severity"`
	Message    string `json:"message"`
	Ref        string `json:"ref,omitempty"`
	Source     string `json:"source,omitempty"`
	Path       string `json:"path,omitempty"`
	ResourceID string `json:"resourceId,omitempty"`
	DriverID   string `json:"driverId,omitempty"`
}

type MutationInfo struct {
	Result       string           `json:"result"`
	RunID        string           `json:"runId,omitempty"`
	LedgerRef    string           `json:"ledgerRef,omitempty"`
	BackupRefs   []string         `json:"backupRefs,omitempty"`
	Verification VerificationInfo `json:"verification"`
	ArtifactRefs MutationRefs     `json:"artifactRefs,omitempty"`
}

type VerificationInfo struct {
	Verified bool   `json:"verified"`
	Result   string `json:"result"`
	Message  string `json:"message,omitempty"`
}

type MutationRefs struct {
	RunRecord     string `json:"runRecord,omitempty"`
	Ledger        string `json:"ledger,omitempty"`
	Backup        string `json:"backup,omitempty"`
	BackupPayload string `json:"backupPayload,omitempty"`
}

const (
	FileTreeOperationActionCreate = "create"
	FileTreeOperationActionUpdate = "update"
	FileTreeOperationActionRemove = "remove"

	FileTreeOperationKindFile      = "file"
	FileTreeOperationKindDirectory = "directory"

	FileTreeOperationStatePlanned = "planned"
	FileTreeOperationStateApplied = "applied"
)

const fileTreeRemovalTextLimit = 20

type FileTreeInfo struct {
	Operations []FileTreeOperation `json:"operations,omitempty"`
}

type FileTreeOperation struct {
	Action string `json:"action"`
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	State  string `json:"state"`
}

type NativeExportInfo struct {
	OperationID       string   `json:"operationId"`
	ImportOperationID string   `json:"importOperationId,omitempty"`
	VerifyOperationID string   `json:"verifyOperationId,omitempty"`
	ArtifactForm      string   `json:"artifactForm"`
	DiffMode          string   `json:"diffMode"`
	Redaction         string   `json:"redaction"`
	ReviewRequired    bool     `json:"reviewRequired,omitempty"`
	ApplySupported    bool     `json:"applySupported,omitempty"`
	BackupPolicy      string   `json:"backupPolicy,omitempty"`
	VerifyPolicy      string   `json:"verifyPolicy,omitempty"`
	Limitations       []string `json:"limitations,omitempty"`
	StagingRoot       string   `json:"-"`
	PayloadRoot       string   `json:"-"`
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
		return 1
	}
	return e.Exit
}

func Build(opts Options) (*Report, error) {
	command, err := normalizeCommand(opts.Command)
	if err != nil {
		return errorReport(opts, "selectedpreview.command.invalid", err.Error(), nil), err
	}
	repoRoot, err := normalizeRepoRoot(opts.RepoRoot)
	if err != nil {
		return errorReport(opts, "selectedpreview.repo.invalid", err.Error(), nil), err
	}
	profile, err := resolution.Resolve(repoRoot, resolution.ResolveOptions{MachineID: opts.MachineID, UserID: opts.UserID, ExtraLayers: opts.ExtraLayers})
	if err != nil {
		wrapped := &Error{Code: "selectedpreview.profile.resolve", Message: err.Error(), Exit: 2}
		return errorReport(opts, wrapped.Code, wrapped.Message, nil), wrapped
	}

	ref, err := parseRef(opts.Ref)
	if err != nil {
		wrapped := &Error{Code: "selectedpreview.ref.invalid", Message: err.Error(), Exit: 2, Details: map[string]any{"ref": opts.Ref}}
		return errorReport(opts, wrapped.Code, wrapped.Message, wrapped.Details), wrapped
	}
	settings := filterSettings(profile.Settings, ref)
	if len(settings) == 0 {
		wrapped := &Error{Code: "selectedpreview.ref.notFound", Message: fmt.Sprintf("no selected settings match ref %q", opts.Ref), Exit: 2, Details: map[string]any{"ref": opts.Ref}}
		return errorReport(opts, wrapped.Code, wrapped.Message, wrapped.Details), wrapped
	}

	report := baseReport(command, commandDryRun(command, opts.DryRun), profile.Layers)
	report.Invocation = invocationContextFromOptions(opts)
	for _, setting := range settings {
		report.Items = append(report.Items, buildItem(profile.RepoRoot, opts.StateRoot, command, report.DryRun, setting, opts))
	}
	finishReport(report)
	return report, nil
}

func JSON(report *Report) (string, error) {
	if report == nil {
		report = baseReport(CommandStatus, false, nil)
		report.Summary.Status = SummaryError
	}
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	return string(payload) + "\n", nil
}

func ErrorReport(command string, dryRun bool, code string, message string, details map[string]any) *Report {
	report := baseReport(command, dryRun, nil)
	report.Summary.Status = SummaryError
	report.Error = &ErrorObj{Code: code, Message: message, Details: details}
	return report
}

func Text(report *Report) string {
	if report == nil {
		return "selected-value preview\nsummary status=error changed=0 blocked=0"
	}
	return defaultText(report)
}

func VerboseText(report *Report) string {
	if report == nil {
		return "selected-value preview\nsummary status=error changed=0 blocked=0"
	}
	defaultOutput := strings.TrimSpace(defaultText(report))
	technicalOutput := strings.TrimSpace(technicalText(report))
	if technicalOutput == "" {
		return defaultOutput
	}
	lines := []string{}
	if defaultOutput != "" {
		lines = append(lines, defaultOutput)
	}
	lines = append(lines, "", "Technical details:")
	for _, line := range strings.Split(technicalOutput, "\n") {
		lines = append(lines, "  "+line)
	}
	return strings.Join(lines, "\n")
}

func defaultText(report *Report) string {
	lines := []string{}
	if report.Error != nil && len(report.Items) == 0 {
		lines = append(lines, commandTitle(report.Command), "", "Blocked:", "  "+fallback(report.Error.Message, "The command could not complete."), "", "No files changed.")
		return strings.Join(lines, "\n")
	}
	if len(report.Items) == 0 {
		lines = append(lines, commandTitle(report.Command), "", "No selected settings matched this command.", "items: none", "", "No files changed.")
		return strings.Join(lines, "\n")
	}

	if len(report.Items) == 1 {
		lines = append(lines, singleItemDefaultText(report, report.Items[0])...)
	} else {
		lines = append(lines, multiItemDefaultText(report)...)
	}

	lines = append(lines, "")
	lines = append(lines, fileChangeLine(report))
	if report.Error != nil {
		lines = append(lines, "", "Command result:", "  "+fallback(report.Error.Message, "The command did not complete cleanly."))
	}
	if next := nextCommandLines(report); len(next) > 0 {
		lines = append(lines, "")
		lines = append(lines, next...)
	}
	return strings.Join(lines, "\n")
}

func singleItemDefaultText(report *Report, item Item) []string {
	label := itemDisplayName(item)
	lines := []string{singleItemHeadline(report, item, label), ""}
	if alias := commandAliasLines(report.Command); len(alias) > 0 {
		lines = append(lines, alias...)
		lines = append(lines, "")
	}

	if status := itemStatusText(report, item); status != "" {
		lines = append(lines, "Status:", "  "+status, "")
	}
	if reason := itemReasonText(item); reason != "" && item.Mutation == nil {
		lines = append(lines, "Reason:", "  "+reason, "")
	}

	switch report.Command {
	case CommandSave:
		if report.DryRun {
			lines = append(lines, "From live settings:")
		} else if item.Mutation != nil {
			lines = append(lines, "Read from live settings:")
		} else {
			lines = append(lines, "Live settings:")
		}
		lines = append(lines, liveValueLines(item)...)
		lines = append(lines, "")
		if report.DryRun {
			lines = append(lines, "Would update stored settings file:")
		} else if item.Mutation != nil {
			lines = append(lines, "Updated stored settings file:")
		} else {
			lines = append(lines, "Stored settings file:")
		}
		lines = append(lines, "  "+desiredPathLabel(item), "")
		if item.Mutation != nil {
			lines = append(lines, liveUnchangedLine(item), "")
		}
	case CommandApply:
		if itemBlocked(item) {
			lines = append(lines, "Stored settings:")
		} else if report.DryRun {
			lines = append(lines, "Would read stored settings from:")
		} else if item.Mutation != nil {
			lines = append(lines, "Read stored settings from:")
		} else {
			lines = append(lines, "Stored settings:")
		}
		lines = append(lines, desiredValueLines(item)...)
		lines = append(lines, "")
		if itemBlocked(item) {
			lines = append(lines, "Live value:")
		} else if report.DryRun {
			lines = append(lines, "Would update live file:")
		} else if item.Mutation != nil {
			lines = append(lines, "Updated live file:")
		} else {
			lines = append(lines, "Live value:")
		}
		lines = append(lines, livePathLines(item)...)
		lines = append(lines, "")
		if removalLines := fileTreeRemovalDefaultLines(report, item); len(removalLines) > 0 {
			lines = append(lines, removalLines...)
		}
		if backup := backupSummaryLine(report, item); backup != "" {
			lines = append(lines, "Backup:", "  "+backup, "")
		}
	default:
		lines = append(lines, "Live value:")
		lines = append(lines, liveValueLines(item)...)
		lines = append(lines, "")
		lines = append(lines, "Stored settings:")
		lines = append(lines, desiredValueLines(item)...)
		lines = append(lines, "")
		if report.Command == CommandDiff && item.Diff != nil {
			lines = append(lines, "Diff:", "  "+diffText(item), "")
		}
	}

	if review := noBaselineReviewLines(report, item); len(review) > 0 {
		lines = append(lines, review...)
	}
	if diagnosticsHidden(item) {
		lines = append(lines, "Diagnostics:", "  Run again with --verbose to see technical diagnostics.", "")
	}
	return trimTrailingBlank(lines)
}

func multiItemDefaultText(report *Report) []string {
	counts := defaultCounts(report)
	lines := []string{fmt.Sprintf("Checked %d selected settings.", len(report.Items)), ""}
	if alias := commandAliasLines(report.Command); len(alias) > 0 {
		lines = append(lines, alias...)
		lines = append(lines, "")
	}
	lines = append(lines, fmt.Sprintf("Summary: %d changed, %d unchanged, %d blocked.", counts.changed, counts.unchanged, counts.blocked), "")
	if report.DryRun {
		lines = append(lines, "Dry run: no files were changed.", "")
	}
	groups := groupItemsByTarget(report.Items)
	targets := make([]string, 0, len(groups))
	for target := range groups {
		targets = append(targets, target)
	}
	sort.Strings(targets)
	for _, target := range targets {
		items := groups[target]
		lines = append(lines, targetDisplayName(target))
		for _, item := range items {
			line := "  - " + itemDisplayName(item) + ": " + itemStatusText(report, item)
			lines = append(lines, line)
			if reason := itemReasonText(item); reason != "" && itemBlocked(item) {
				lines = append(lines, "    Reason: "+reason)
			}
			if item.NoBaseline {
				lines = append(lines, "    Review: not previously applied by this tool; review before confirming.")
			}
			if removalLine := fileTreeRemovalSummaryLine(report, item); removalLine != "" {
				lines = append(lines, "    "+removalLine)
			}
		}
		lines = append(lines, "")
	}
	return trimTrailingBlank(lines)
}

func technicalText(report *Report) string {
	if report == nil {
		return "selected-value preview\nsummary status=error changed=0 blocked=0"
	}
	lines := []string{fmt.Sprintf("selected-value %s", report.Command)}
	if len(report.ProfileStack) > 0 {
		lines = append(lines, "profile: "+strings.Join(report.ProfileStack, " -> "))
	}
	if report.DryRun {
		lines = append(lines, "MODE: DRY RUN (no writes)")
	}
	if len(report.Items) == 0 {
		lines = append(lines, "items: none")
	}
	for _, item := range report.Items {
		line := fmt.Sprintf("  %s scope=%s subject=%s state=%s desired=%s current=%s", item.SettingRef, item.Scope, item.Subject, item.State, item.Desired.Status, existsLabel(item.Current.Exists))
		if item.PlannedAction != "" {
			line += " action=" + item.PlannedAction
		}
		if item.NoBaseline {
			line += " no-baseline"
		}
		lines = append(lines, line)
		if item.Resource.DriverID != "" {
			lines = append(lines, fmt.Sprintf("    resource=%s driver=%s selector=%s", item.Resource.ID, item.Resource.DriverID, item.Selector.Summary))
		}
		if item.DesiredURI != "" || item.DesiredRelPath != "" {
			lines = append(lines, fmt.Sprintf("    desiredArtifact=%s desiredPath=%s", item.DesiredURI, item.DesiredRelPath))
		}
		if item.Preview != nil && item.Preview.ReadOnly {
			lines = append(lines, "    mode=read-only (save/apply unsupported)")
		}
		if item.Diff != nil {
			lines = append(lines, fmt.Sprintf("    diff=%s mode=%s redaction=%s", item.Diff.Kind, item.Diff.Mode, item.Diff.Redaction))
		}
		if item.Message != "" {
			lines = append(lines, "    message: "+item.Message)
		}
		if item.Mutation != nil {
			lines = append(lines, fmt.Sprintf("    mutation=%s verified=%t run=%s", item.Mutation.Result, item.Mutation.Verification.Verified, item.Mutation.RunID))
			if len(item.Mutation.BackupRefs) > 0 {
				lines = append(lines, "    backups="+strings.Join(item.Mutation.BackupRefs, ","))
			}
		}
		for _, action := range item.Lifecycle {
			lifecycleLine := fmt.Sprintf("    lifecycle phase=%s action=%s mode=%s result=%s", action.Phase, action.Action, action.Mode, action.Result)
			if action.LifecycleTargetID != "" {
				lifecycleLine += " target=" + action.LifecycleTargetID
			}
			if action.StateAfter != "" {
				lifecycleLine += " state=" + action.StateAfter
			}
			if action.Code != "" {
				lifecycleLine += " code=" + action.Code
			}
			lines = append(lines, lifecycleLine)
		}
		for _, diagnostic := range item.Diagnostics {
			lines = append(lines, fmt.Sprintf("    %s[%s]: %s", diagnostic.Severity, diagnostic.Code, diagnostic.Message))
		}
	}
	lines = append(lines, fmt.Sprintf("summary status=%s changed=%d blocked=%d saved=%d applied=%d", report.Summary.Status, report.Summary.Changed, report.Summary.Blocked, report.Summary.Saved, report.Summary.Applied))
	return strings.Join(lines, "\n")
}

type textCounts struct {
	changed   int
	unchanged int
	blocked   int
}

func defaultCounts(report *Report) textCounts {
	counts := textCounts{}
	if report == nil {
		return counts
	}
	for _, item := range report.Items {
		switch {
		case itemBlocked(item):
			counts.blocked++
		case itemUnchanged(item):
			counts.unchanged++
		default:
			counts.changed++
		}
	}
	return counts
}

func commandTitle(command string) string {
	switch command {
	case CommandStatus:
		return "Status"
	case CommandDiff:
		return "Diff"
	case CommandSave:
		return "Save (sync live settings -> stored settings)"
	case CommandApply:
		return "Apply (sync stored settings -> live settings)"
	default:
		return "Selected settings"
	}
}

func commandAliasLines(command string) []string {
	switch command {
	case CommandSave:
		return []string{
			"Command alias:",
			"  save is a compatibility alias for sync.",
			"Sync direction:",
			"  live settings -> stored settings",
			"Primary command:",
			"  sync",
		}
	case CommandApply:
		return []string{
			"Command alias:",
			"  apply is a compatibility alias for sync.",
			"Sync direction:",
			"  stored settings -> live settings",
			"Primary command:",
			"  sync",
		}
	default:
		return nil
	}
}

func singleItemHeadline(report *Report, item Item, label string) string {
	if report == nil {
		return label
	}
	if itemBlocked(item) {
		if report != nil {
			switch report.Command {
			case CommandApply:
				return "Cannot apply " + label + " yet."
			case CommandSave:
				return "Cannot save " + label + " yet."
			}
		}
		return label + " is blocked."
	}
	if item.Mutation != nil {
		switch report.Command {
		case CommandSave:
			if item.Mutated || report.Summary.Saved > 0 {
				return "Synced " + label + " to stored settings."
			}
			return label + " was already in stored settings."
		case CommandApply:
			if item.Mutated || report.Summary.Applied > 0 {
				return "Synced " + label + " to live settings."
			}
			return label + " was already up to date."
		}
	}
	if report.DryRun {
		switch report.Command {
		case CommandSave:
			if IsSavePlannedAction(item.PlannedAction) {
				return "Dry run: would sync " + label + " to stored settings."
			}
		case CommandApply:
			if item.PlannedAction == PlannedActionWouldApply {
				return "Dry run: would sync " + label + " to live settings."
			}
		}
	}
	if report.Command == CommandDiff && !itemUnchanged(item) {
		return label + " differs between live settings and stored settings."
	}
	return label
}

func itemStatusText(report *Report, item Item) string {
	if report != nil && item.Mutation != nil {
		switch report.Command {
		case CommandSave:
			return "Stored settings now contain this live value."
		case CommandApply:
			return "Live settings now match the stored settings."
		}
	}
	if itemBlocked(item) {
		if item.PlannedAction == PlannedActionBlockedMissingDesired || item.State == v2status.StateMissingDesired || item.Desired.Status == desired.StatusMissing {
			return "Blocked because no stored settings exist yet."
		}
		return "Blocked; no files will be changed."
	}
	if item.Preview != nil && item.Preview.ReadOnly {
		return "read-only; directional sync aliases are not supported for this setting."
	}
	if item.Desired.Status == desired.StatusUnmanaged || item.Desired.Unmanaged {
		return "Intentionally unmanaged."
	}
	if item.Desired.Status == desired.StatusMissing {
		if item.Current.Exists {
			return "Selected, but not stored in the settings folder yet."
		}
		return "Selected, but neither live settings nor stored settings exist yet."
	}
	if itemUnchanged(item) {
		return "Live settings match stored settings."
	}
	if report != nil {
		switch report.Command {
		case CommandSave:
			if IsSavePlannedAction(item.PlannedAction) {
				if item.Current.Exists {
					return "Live settings can be synced to stored settings."
				}
				return "Live value is missing; sync would record that in stored settings."
			}
		case CommandApply:
			if item.PlannedAction == PlannedActionWouldApply {
				return "Stored settings can be synced to live settings."
			}
		case CommandDiff:
			if item.Diff != nil {
				return "Live settings differ from stored settings."
			}
		}
	}
	if item.Current.Exists && item.Desired.Snapshot.Exists {
		return "Live settings differ from stored settings."
	}
	if item.Current.Exists {
		return "Live value exists."
	}
	return "Live value is missing."
}

func itemReasonText(item Item) string {
	if item.Message != "" {
		return humanizeInternalText(item.Message)
	}
	if itemBlocked(item) {
		return "Safety policy blocked this item. Run with --verbose for technical diagnostics."
	}
	return ""
}

func liveValueLines(item Item) []string {
	lines := livePathLines(item)
	if item.Current.Exists {
		lines = append(lines, "  Value hidden for safety.")
	} else {
		lines = append(lines, "  No live value found.")
	}
	return lines
}

func desiredValueLines(item Item) []string {
	path := desiredPathLabel(item)
	if item.Desired.Status == desired.StatusMissing || path == "not created yet" {
		return []string{"  Not created yet."}
	}
	lines := []string{"  " + path}
	if desiredExists(item) {
		lines = append(lines, "  Value hidden for safety.")
	}
	return lines
}

func livePathLines(item Item) []string {
	label := livePathLabel(item)
	if selector := strings.TrimSpace(item.Selector.Summary); selector != "" && selector != label && !strings.Contains(label, selector) {
		label += " " + selector
	}
	return []string{"  " + label}
}

func livePathLabel(item Item) string {
	if item.Resource.LocationID == "home" && strings.TrimSpace(item.Resource.RelPath) != "" {
		return "$HOME/" + strings.TrimPrefix(filepath.ToSlash(item.Resource.RelPath), "/")
	}
	if strings.TrimSpace(item.Resource.DisplayPath) != "" {
		return item.Resource.DisplayPath
	}
	if strings.TrimSpace(item.Resource.Path) != "" {
		return item.Resource.Path
	}
	if strings.TrimSpace(item.Resource.RelPath) != "" {
		if item.Resource.LocationID != "" {
			return item.Resource.LocationID + ":" + filepath.ToSlash(item.Resource.RelPath)
		}
		return filepath.ToSlash(item.Resource.RelPath)
	}
	if strings.TrimSpace(item.Resource.ID) != "" {
		return item.Resource.ID
	}
	return item.SettingRef
}

func desiredPathLabel(item Item) string {
	if strings.TrimSpace(item.DesiredRelPath) != "" {
		return filepath.ToSlash(item.DesiredRelPath)
	}
	if strings.TrimSpace(item.DesiredURI) == "" {
		return "not created yet"
	}
	return desiredURIToPath(item.DesiredURI)
}

func desiredURIToPath(uri string) string {
	trimmed := strings.TrimPrefix(strings.TrimSpace(uri), "desired://")
	if trimmed == "" {
		return "not created yet"
	}
	if before, _, ok := strings.Cut(trimmed, "#"); ok {
		if strings.HasSuffix(before, "/settings") {
			return "desired/" + before + ".yaml"
		}
		return "desired/" + before
	}
	return "desired/" + trimmed
}

func desiredExists(item Item) bool {
	return item.Desired.Status == desired.StatusPresent || item.Desired.Snapshot.Exists
}

func diffText(item Item) string {
	if item.Diff == nil {
		return "No visible diff."
	}
	if item.Diff.Mode == "metadata-only" {
		return "Values are hidden; only metadata is compared."
	}
	return humanizeInternalText(fallback(item.Diff.Message, item.Diff.Kind))
}

func backupSummaryLine(report *Report, item Item) string {
	if item.Mutation == nil {
		if report != nil && report.DryRun && report.Command == CommandApply && item.PlannedAction == PlannedActionWouldApply {
			if label := strings.TrimSpace(livePathLabel(item)); label != "" {
				return "A local backup of " + label + " would be created before writing."
			}
			return "A local backup would be created before writing."
		}
		return ""
	}
	if len(item.Mutation.BackupRefs) == 0 {
		return "No backup was needed for this item."
	}
	if strings.TrimSpace(item.Mutation.RunID) != "" {
		return "Local backup recorded for restore as backup run " + item.Mutation.RunID + "."
	}
	return "Local backup recorded for restore."
}

func fileTreeRemovalDefaultLines(report *Report, item Item) []string {
	if report == nil || report.Command != CommandApply {
		return nil
	}
	removals := fileTreeRemoveOperations(item)
	if len(removals) == 0 {
		return nil
	}
	heading := "Will remove live paths not present in stored settings:"
	if fileTreeRemovalsApplied(report, item) {
		heading = "Removed live paths not present in stored settings:"
	}
	lines := []string{"File-tree removals:", "  " + heading}
	limit := fileTreeRemovalTextLimit
	if limit <= 0 || limit > len(removals) {
		limit = len(removals)
	}
	for _, operation := range removals[:limit] {
		lines = append(lines, fmt.Sprintf("  - %s (%s)", operation.Path, operation.Kind))
	}
	if omitted := len(removals) - limit; omitted > 0 {
		lines = append(lines, fmt.Sprintf("  ... and %d more; use --json to see the full fileTree.operations list.", omitted))
	}
	return append(lines, "")
}

func fileTreeRemovalSummaryLine(report *Report, item Item) string {
	if report == nil || report.Command != CommandApply {
		return ""
	}
	removals := fileTreeRemoveOperations(item)
	if len(removals) == 0 {
		return ""
	}
	if fileTreeRemovalsApplied(report, item) {
		return fmt.Sprintf("Removed %d live %s not present in stored settings; use --json for full fileTree.operations.", len(removals), pluralizePath(len(removals)))
	}
	return fmt.Sprintf("Will remove %d live %s not present in stored settings; run a focused dry-run or --json for full paths.", len(removals), pluralizePath(len(removals)))
}

func fileTreeRemoveOperations(item Item) []FileTreeOperation {
	if item.FileTree == nil || len(item.FileTree.Operations) == 0 {
		return nil
	}
	removals := make([]FileTreeOperation, 0)
	for _, operation := range item.FileTree.Operations {
		if operation.Action == FileTreeOperationActionRemove {
			removals = append(removals, operation)
		}
	}
	return removals
}

func fileTreeRemovalsApplied(report *Report, item Item) bool {
	if report == nil || report.DryRun || item.Mutation == nil {
		return false
	}
	if item.Mutation.Result != "verified" || !item.Mutated {
		return false
	}
	for _, operation := range fileTreeRemoveOperations(item) {
		if operation.State != FileTreeOperationStateApplied {
			return false
		}
	}
	return true
}

func pluralizePath(count int) string {
	if count == 1 {
		return "path"
	}
	return "paths"
}

func noBaselineReviewLines(report *Report, item Item) []string {
	if !item.NoBaseline {
		return nil
	}
	lines := []string{"Review note:", "  This setting has not previously been applied by this tool."}
	if report != nil && report.Command == CommandApply && item.Mutation != nil {
		lines[1] = "  This was the first apply recorded by this tool for this setting."
		if len(item.Mutation.BackupRefs) > 0 {
			lines = append(lines, "  A backup was created before writing.")
		}
		return append(lines, "")
	}
	if report != nil && (report.Command == CommandDiff || report.Command == CommandApply) {
		lines = append(lines, "  Review the paths before confirming an apply.")
	} else {
		lines = append(lines, "  Review the paths before confirming.")
	}
	return append(lines, "")
}

func liveUnchangedLine(item Item) string {
	return fmt.Sprintf("No live %s config was changed.", targetDisplayName(item.TargetRef))
}

func fileChangeLine(report *Report) string {
	if report == nil {
		return "No files changed."
	}
	if report.DryRun || report.Command == CommandStatus || report.Command == CommandDiff || report.Summary.Status == SummaryBlocked || report.Summary.Status == SummaryError {
		return "No files changed."
	}
	switch report.Command {
	case CommandSave:
		if report.Summary.Saved > 0 || report.Summary.Changed > 0 {
			return "Stored settings changed."
		}
	case CommandApply:
		if report.Summary.Applied > 0 || report.Summary.Changed > 0 {
			if reportHasBackupRefs(report) {
				return "Live files changed after backup."
			}
			return "Live files changed."
		}
	}
	return "No files changed."
}

func reportHasBackupRefs(report *Report) bool {
	if report == nil {
		return false
	}
	for _, item := range report.Items {
		if item.Mutation != nil && len(item.Mutation.BackupRefs) > 0 {
			return true
		}
	}
	return false
}

func nextCommandLines(report *Report) []string {
	if report == nil || len(report.Items) == 0 {
		return nil
	}
	item := report.Items[0]
	if len(report.Items) > 1 {
		if defaultCounts(report).blocked > 0 {
			return []string{"Next:", "  Run with --verbose to see technical diagnostics for blocked items."}
		}
		return []string{"Next:", "  Review the grouped settings above, then run the matching dry-run command before confirming."}
	}
	ref := fallback(item.SettingRef, "<target:setting>")
	if itemBlocked(item) {
		if item.PlannedAction == PlannedActionBlockedMissingDesired || item.Desired.Status == desired.StatusMissing {
			return []string{"Next:", "  Preview explicit sync from live settings to stored settings:", "  " + selectedCommandLine(report, CommandSave, []string{"--dry-run"}, ref)}
		}
		return []string{"Next:", "  Run with --verbose for technical diagnostics:", "  " + selectedCommandLine(report, fallback(report.Command, CommandStatus), []string{"--verbose"}, ref)}
	}
	switch report.Command {
	case CommandStatus:
		if item.NativeExport != nil {
			return []string{"Next:", "  Inspect native export metadata before choosing a sync direction:", "  " + selectedCommandLine(report, CommandDiff, nil, ref)}
		}
		if item.Desired.Status == desired.StatusMissing {
			return []string{"Next:", "  Preview explicit sync from live settings to stored settings:", "  " + selectedCommandLine(report, CommandSave, []string{"--dry-run"}, ref)}
		}
		if !itemUnchanged(item) {
			return []string{"Next:", "  Inspect the hidden-value diff:", "  " + selectedCommandLine(report, CommandDiff, nil, ref)}
		}
	case CommandSave:
		if report.DryRun && IsSavePlannedAction(item.PlannedAction) {
			return []string{"To confirm:", "  " + selectedCommandLine(report, CommandSave, []string{"--yes"}, ref)}
		}
		if item.Mutation != nil {
			return []string{"Next:", "  Inspect drift later with:", "  " + selectedCommandLine(report, CommandDiff, nil, ref)}
		}
	case CommandDiff:
		if !itemUnchanged(item) {
			return []string{"Next:", "  Run sync to use the safe direction after reviewing this diff:", "  " + selectedCommandLine(report, CommandSync, nil, ref)}
		}
	case CommandApply:
		if report.DryRun && item.PlannedAction == PlannedActionWouldApply {
			return []string{"To confirm:", "  " + selectedCommandLine(report, CommandApply, []string{"--yes"}, ref)}
		}
		if item.Mutation != nil && item.Mutation.RunID != "" {
			return []string{"Next:", "  Preview restore if needed:", "  " + restoreCommandLine(report, item.Mutation.RunID, []string{"--dry-run"})}
		}
	}
	return nil
}

func selectedCommandLine(report *Report, command string, commandFlags []string, ref string) string {
	args := commandPrefixArgs(report)
	args = append(args, strings.TrimSpace(command))
	args = append(args, nonEmptyArgs(commandFlags)...)
	args = append(args, selectedContextArgs(report)...)
	if strings.TrimSpace(ref) != "" {
		args = append(args, ref)
	}
	return shellCommandLine(args)
}

func restoreCommandLine(report *Report, runID string, commandFlags []string) string {
	args := commandPrefixArgs(report)
	args = append(args, "restore", strings.TrimSpace(runID))
	args = append(args, nonEmptyArgs(commandFlags)...)
	args = append(args, selectedContextArgs(report)...)
	return shellCommandLine(args)
}

func commandPrefixArgs(report *Report) []string {
	args := []string{"dotfiles-manager"}
	configPath := ""
	if report != nil {
		configPath = strings.TrimSpace(report.Invocation.ConfigPath)
	}
	if configPath == "" {
		configPath = resolution.RootConfigFile
	}
	return append(args, "--config", configPath)
}

func selectedContextArgs(report *Report) []string {
	if report == nil {
		return nil
	}
	args := []string{}
	if report.Invocation.MachineID != "" {
		args = append(args, "--machine-id", report.Invocation.MachineID)
	}
	if report.Invocation.UserID != "" {
		args = append(args, "--user-id", report.Invocation.UserID)
	}
	for _, layer := range report.Invocation.ExtraLayers {
		layer = strings.TrimSpace(layer)
		if layer == "" {
			continue
		}
		args = append(args, "--profile", layer)
	}
	return args
}

func nonEmptyArgs(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func shellCommandLine(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		if arg == "" {
			continue
		}
		quoted = append(quoted, shellQuoteArg(arg))
	}
	return strings.Join(quoted, " ")
}

func shellQuoteArg(arg string) string {
	if arg == "" {
		return "''"
	}
	if strings.IndexFunc(arg, func(r rune) bool {
		return !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || strings.ContainsRune("._:/=-", r))
	}) == -1 {
		return arg
	}
	return "'" + strings.ReplaceAll(arg, "'", "'\\''") + "'"
}

func itemDisplayName(item Item) string {
	settingID := item.SettingRef
	if _, rest, ok := strings.Cut(settingID, ":"); ok {
		settingID = rest
	}
	return strings.TrimSpace(targetDisplayName(item.TargetRef) + " " + wordsFromID(settingID))
}

func targetDisplayName(target string) string {
	switch strings.TrimSpace(target) {
	case "git":
		return "Git"
	case "zsh":
		return "Zsh"
	case "tmux":
		return "tmux"
	case "nvim":
		return "Neovim"
	case "ssh":
		return "SSH"
	case "starship":
		return "Starship"
	case "":
		return "Selected setting"
	default:
		return titleWords(strings.ReplaceAll(target, ".", " "))
	}
}

func titleWords(value string) string {
	parts := strings.Fields(value)
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
	}
	return strings.Join(parts, " ")
}

func wordsFromID(id string) string {
	parts := strings.Fields(strings.NewReplacer(".", " ", "-", " ", "_", " ").Replace(id))
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToLower(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

func itemBlocked(item Item) bool {
	if strings.HasPrefix(item.PlannedAction, "blocked-") {
		return true
	}
	switch item.State {
	case v2status.StateBlockedSafety, v2status.StateBlockedLifecycle, v2status.StateUnsupported:
		return true
	default:
		return false
	}
}

func itemUnchanged(item Item) bool {
	return item.State == v2status.StateUnchanged || item.PlannedAction == PlannedActionNone
}

func diagnosticsHidden(item Item) bool {
	return len(item.Diagnostics) > 0 && !itemBlocked(item)
}

func humanizeInternalText(text string) string {
	out := strings.TrimSpace(text)
	switch out {
	case "Setting is selected but no desired artifact exists.":
		return "This setting is selected, but the settings folder does not have stored settings for it yet."
	case "Setting is selected but no stored settings exists.":
		return "This setting is selected, but the settings folder does not have stored settings for it yet."
	case "Existing live selected value can be promoted into desired state with save --yes; raw value remains redacted in output.":
		return "Existing live value can be synced to stored settings with save --yes; raw value remains hidden."
	case "Current differs from desired and there is no previous sync baseline; saving will replace the desired artifact.":
		return "Live settings differ from stored settings. Syncing to stored settings would replace the stored value."
	case "Desired differs from current and there is no previous sync baseline; applying will replace live state.":
		return "Stored settings differ from live settings. Syncing to live settings would replace the live value."
	case "Changed, no previous sync baseline: review diff, then choose save or apply.":
		return "Live settings differ from stored settings."
	case "State cannot be determined safely from incomplete last-applied baseline data.":
		return "State cannot be determined safely from incomplete previous apply data."
	case "Current differs from desired; last-applied baseline matches desired.":
		return "Live settings differ from stored settings. The previous apply recorded by this tool matches stored settings."
	case "Desired differs from current; last-applied baseline matches current.":
		return "Stored settings differ from live settings. The previous apply recorded by this tool matches current live settings."
	}
	out = strings.ReplaceAll(out, "selected-value", "selected value")
	out = strings.ReplaceAll(out, "selected value", "value")
	out = strings.ReplaceAll(out, "desired artifact", "stored settings")
	out = strings.ReplaceAll(out, "Desired value artifact", "Stored settings")
	out = strings.ReplaceAll(out, "no desired artifact", "no stored settings")
	return out
}

func groupItemsByTarget(items []Item) map[string][]Item {
	groups := map[string][]Item{}
	for _, item := range items {
		groups[item.TargetRef] = append(groups[item.TargetRef], item)
	}
	return groups
}

func trimTrailingBlank(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func normalizeCommand(command string) (string, error) {
	switch strings.TrimSpace(command) {
	case CommandStatus:
		return CommandStatus, nil
	case CommandDiff:
		return CommandDiff, nil
	case CommandSave:
		return CommandSave, nil
	case CommandApply:
		return CommandApply, nil
	default:
		return "", fmt.Errorf("unsupported selected-value preview command: %s", command)
	}
}

func normalizeRepoRoot(repoRoot string) (string, error) {
	trimmed := strings.TrimSpace(repoRoot)
	if trimmed == "" {
		return "", fmt.Errorf("v2 repo root is required")
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("v2 repo root is not a directory: %s", abs)
	}
	return abs, nil
}

func commandDryRun(command string, dryRun bool) bool {
	return dryRun
}

func baseReport(command string, dryRun bool, profileStack []string) *Report {
	report := &Report{Schema: Schema, SchemaVersion: SchemaVersion, Command: command, RunID: RunID, DryRun: dryRun, ProfileStack: append([]string(nil), profileStack...), Summary: Summary{Status: SummaryOK}, Items: []Item{}}
	if operation, direction, ok := commandAliasOperation(command); ok {
		report.Operation = operation
		report.InvokedCommand = command
		report.Direction = direction
	}
	return report
}

func commandAliasOperation(command string) (operation string, direction string, ok bool) {
	switch command {
	case CommandSave:
		return "sync", "live_to_stored", true
	case CommandApply:
		return "sync", "stored_to_live", true
	default:
		return "", "", false
	}
}

func errorReport(opts Options, code string, message string, details map[string]any) *Report {
	command := strings.TrimSpace(opts.Command)
	if command == "" {
		command = CommandStatus
	}
	report := baseReport(command, opts.DryRun, nil)
	report.Invocation = invocationContextFromOptions(opts)
	report.Summary.Status = SummaryError
	report.Error = &ErrorObj{Code: code, Message: message, Details: details}
	return report
}

func invocationContextFromOptions(opts Options) InvocationContext {
	return InvocationContext{
		ConfigPath:  strings.TrimSpace(opts.ConfigPath),
		MachineID:   strings.TrimSpace(opts.MachineID),
		UserID:      strings.TrimSpace(opts.UserID),
		ExtraLayers: append([]string(nil), opts.ExtraLayers...),
	}
}

type parsedRef struct {
	Target  string
	Setting string
	Empty   bool
}

func parseRef(raw string) (parsedRef, error) {
	ref := strings.TrimSpace(raw)
	if ref == "" {
		return parsedRef{Empty: true}, nil
	}
	if strings.Contains(ref, "://") || strings.ContainsAny(ref, "#/") {
		return parsedRef{}, fmt.Errorf("unsupported selected-value ref kind: %s", raw)
	}
	parts := strings.Split(ref, ":")
	if len(parts) == 1 {
		if err := recipe.ValidatePublicID("target", parts[0]); err != nil {
			return parsedRef{}, err
		}
		return parsedRef{Target: parts[0]}, nil
	}
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return parsedRef{}, fmt.Errorf("setting ref must be target:setting, got %q", raw)
	}
	if err := recipe.ValidatePublicID("target", parts[0]); err != nil {
		return parsedRef{}, err
	}
	if err := recipe.ValidatePublicID("setting", parts[1]); err != nil {
		return parsedRef{}, err
	}
	return parsedRef{Target: parts[0], Setting: parts[1]}, nil
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func filterSettings(settings []resolution.ResolvedSetting, ref parsedRef) []resolution.ResolvedSetting {
	out := make([]resolution.ResolvedSetting, 0, len(settings))
	for _, setting := range settings {
		if !ref.Empty && setting.TargetID != ref.Target {
			continue
		}
		if ref.Setting != "" && setting.SettingID != ref.Setting {
			continue
		}
		out = append(out, setting)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ref() < out[j].Ref() })
	return out
}

func buildItem(repoRoot string, stateRoot string, command string, dryRun bool, setting resolution.ResolvedSetting, opts Options) Item {
	item := Item{TargetRef: setting.TargetID, SettingRef: setting.Ref(), Scope: setting.Scope, Subject: setting.Subject, SourceLayer: setting.SourceLayer, DesiredURI: setting.DesiredURI, DesiredRelPath: filepath.ToSlash(setting.DesiredRelPath), State: v2status.StateUnknown, DryRun: dryRun, Mutated: false, Diagnostics: []Diagnostic{}}

	runtime, blocked := loadRuntimeRecipe(repoRoot, setting.TargetID)
	rec := runtime.Recipe
	item.Recipe.Source = runtime.Source
	item.Recipe.RecipeRef = runtime.RecipeRef
	item.Recipe.TrustStatus = runtime.TrustStatus
	if len(blocked) > 0 {
		for _, diagnostic := range blocked {
			item.Diagnostics = append(item.Diagnostics, diagnostic.withRef(item.SettingRef))
		}
		return finishBlocked(item, v2status.StateUnsupported, "Recipe runtime is not available for selected-value preview.")
	}

	resourceID, resource, err := rec.ResourceForSetting(setting.SettingID)
	if err != nil {
		if rec.Target == recipe.ZshTarget {
			if blockedDiagnostic, ok := recipe.ZshBlockedSettingDiagnostic(setting.SettingID); ok {
				item.Diagnostics = append(item.Diagnostics, fromRecipeDiagnostic(blockedDiagnostic, item.SettingRef, runtime.Source, "", ""))
				return finishBlocked(item, v2status.StateUnsupported, "Zsh setting is blocked by bundled recipe policy.")
			}
		}
		if rec.Target == recipe.SSHTarget {
			if blockedDiagnostic, ok := recipe.SSHExcludedSettingDiagnostic(setting.SettingID); ok {
				item.Diagnostics = append(item.Diagnostics, fromRecipeDiagnostic(blockedDiagnostic, item.SettingRef, runtime.Source, "", ""))
				return finishBlocked(item, v2status.StateUnsupported, "SSH setting is excluded by bundled recipe policy.")
			}
		}
		item.Diagnostics = append(item.Diagnostics, diagnostic("selectedpreview.resource.unknown", SeverityError, err.Error(), item.SettingRef))
		return finishBlocked(item, v2status.StateUnsupported, "Selected setting is not supported by the recipe runtime.")
	}
	locationRoots := opts.LocationRoots[setting.TargetID]
	if locationRoots == nil {
		locationRoots = map[string]string{}
	}
	item.Resource = ResourceInfo{
		ID:          resourceID,
		DriverID:    resource.Driver,
		LocationID:  resource.Location,
		RelPath:     resource.Path,
		DisplayPath: recipe.FriendlyLocationPath(rec, resource.Location, resource.Path, locationRoots),
	}
	item.Selector = selectorInfo(selectedvalue.SelectorInfo{})
	if resource.Selector != nil {
		item.Selector = selectorFromRecipe(resource)
	}
	if resource.Driver == recipe.NativeExportDriverID {
		trustEval, trustContext := evaluateTrust(repoRoot, stateRoot, runtime.Source, rec)
		trustContext = writeSafetyContextForCommand(trustContext, command)
		if opts.Confirmed {
			trustContext.AllowOpaque = true
		}
		item.Recipe.TrustStatus = trustEval.Status
		if trustEval.Status != recipe.TrustStatusTrusted {
			for _, diagnostic := range trustEval.Diagnostics {
				item.Diagnostics = append(item.Diagnostics, fromRecipeDiagnostic(diagnostic, item.SettingRef, runtime.Source, resourceID, resource.Driver))
			}
			if len(item.Diagnostics) == 0 {
				item.Diagnostics = append(item.Diagnostics, diagnostic("selectedpreview.trust.required", SeverityError, "native export requires trusted recipe evidence before running reviewed export operations", item.SettingRef))
			}
			return finishBlocked(item, v2status.StateBlockedSafety, "Recipe trust must be reviewed before native export can run.")
		}
		if err := rec.ValidateWriteSafety(trustContext); err != nil {
			validations := recipe.ValidationDiagnostics(err)
			for _, validation := range validations {
				item.Diagnostics = append(item.Diagnostics, fromRecipeDiagnostic(validation, item.SettingRef, runtime.Source, resourceID, resource.Driver))
			}
			return finishBlocked(item, blockedStateForRecipeDiagnostics(validations), "Recipe write-safety metadata blocks native export preview.")
		}
		appendWriteSafetyWarnings(&item, command, rec, setting, resourceID, resource.Driver, runtime.Source, trustContext)
		return applyLifecyclePreview(buildNativeExportItem(repoRoot, stateRoot, command, item, rec, runtime.Source, trustEval, setting, resourceID, resource, locationRoots, opts), rec, setting, resourceID, command, opts)
	}
	if resource.Driver == recipe.FileDriverID || resource.Driver == recipe.FileTreeDriverID {
		trustEval, trustContext := evaluateTrust(repoRoot, stateRoot, runtime.Source, rec)
		trustContext = writeSafetyContextForCommand(trustContext, command)
		item.Recipe.TrustStatus = trustEval.Status
		if trustEval.Status != recipe.TrustStatusTrusted {
			for _, diagnostic := range trustEval.Diagnostics {
				item.Diagnostics = append(item.Diagnostics, fromRecipeDiagnostic(diagnostic, item.SettingRef, runtime.Source, resourceID, resource.Driver))
			}
			if len(item.Diagnostics) == 0 {
				item.Diagnostics = append(item.Diagnostics, diagnostic("selectedpreview.trust.required", SeverityError, "filesystem-resource preview requires trusted recipe evidence before live reads", item.SettingRef))
			}
			return finishBlocked(item, v2status.StateBlockedSafety, "Recipe trust must be reviewed before filesystem-resource preview can read live state.")
		}

		if err := rec.ValidateWriteSafety(trustContext); err != nil {
			validations := recipe.ValidationDiagnostics(err)
			for _, validation := range validations {
				item.Diagnostics = append(item.Diagnostics, fromRecipeDiagnostic(validation, item.SettingRef, runtime.Source, resourceID, resource.Driver))
			}
			return finishBlocked(item, blockedStateForRecipeDiagnostics(validations), "Recipe write-safety metadata blocks filesystem-resource preview.")
		}
		appendWriteSafetyWarnings(&item, command, rec, setting, resourceID, resource.Driver, runtime.Source, trustContext)

		return applyLifecyclePreview(buildFileResourceItem(repoRoot, stateRoot, command, item, rec, setting, locationRoots), rec, setting, resourceID, command, opts)
	}
	if !isSelectedValueDriver(resource.Driver) {
		item.Diagnostics = append(item.Diagnostics, Diagnostic{Code: "selectedpreview.driver.unsupported", Severity: SeverityError, Message: fmt.Sprintf("driver %s is not a selected-value driver", resource.Driver), Ref: item.SettingRef, ResourceID: resourceID, DriverID: resource.Driver})
		return finishBlocked(item, v2status.StateUnsupported, "Resource driver is not supported by selected-value preview.")
	}

	trustEval, trustContext := evaluateTrust(repoRoot, stateRoot, runtime.Source, rec)
	trustContext = writeSafetyContextForCommand(trustContext, command)
	item.Recipe.TrustStatus = trustEval.Status
	if trustEval.Status != recipe.TrustStatusTrusted {
		for _, diagnostic := range trustEval.Diagnostics {
			item.Diagnostics = append(item.Diagnostics, fromRecipeDiagnostic(diagnostic, item.SettingRef, runtime.Source, resourceID, resource.Driver))
		}
		if len(item.Diagnostics) == 0 {
			item.Diagnostics = append(item.Diagnostics, diagnostic("selectedpreview.trust.required", SeverityError, "selected-value preview requires trusted recipe evidence before live reads", item.SettingRef))
		}
		return finishBlocked(item, v2status.StateBlockedSafety, "Recipe trust must be reviewed before selected-value preview can read live state.")
	}

	if err := rec.ValidateWriteSafety(trustContext); err != nil {
		validations := recipe.ValidationDiagnostics(err)
		for _, validation := range validations {
			item.Diagnostics = append(item.Diagnostics, fromRecipeDiagnostic(validation, item.SettingRef, runtime.Source, resourceID, resource.Driver))
		}
		return finishBlocked(item, blockedStateForRecipeDiagnostics(validations), "Recipe write-safety metadata blocks selected-value preview.")
	}
	appendWriteSafetyWarnings(&item, command, rec, setting, resourceID, resource.Driver, runtime.Source, trustContext)

	read, err := desired.ReadSelectedValueForSetting(repoRoot, setting)
	if err != nil {
		item.Diagnostics = append(item.Diagnostics, diagnostic("selectedpreview.desired.read", SeverityError, err.Error(), item.SettingRef))
		return finishBlocked(item, v2status.StateBlockedSafety, "Desired selected-value artifact could not be read safely.")
	}
	item.Desired.Status = read.Status
	item.Desired.Intent = read.Intent
	item.Desired.Kind = read.Kind
	item.Desired.Unmanaged = read.Status == desired.StatusUnmanaged

	if read.Status == desired.StatusUnmanaged {
		item.State = v2status.StateUnchanged
		item.Message = "Setting is intentionally unmanaged in stored settings."
		return item
	}

	if read.Status == desired.StatusMissing {
		return buildMissingDesiredItem(repoRoot, item, rec, setting, locationRoots, command, trustContext, opts.MacOSDefaultsRunner)
	}

	if read.Desired == nil {
		item.Diagnostics = append(item.Diagnostics, diagnostic("selectedpreview.desired.invalid", SeverityError, "desired selected-value entry is present but has no normalized desired state", item.SettingRef))
		return finishBlocked(item, v2status.StateBlockedSafety, "Desired selected-value entry is invalid.")
	}
	if command == CommandApply {
		if isMacOSDefaultsReadOnlyDriver(resource.Driver) {
			item.Diagnostics = append(item.Diagnostics, readOnlyDiagnostic(item))
			return finishBlocked(item, v2status.StateUnsupported, "macOS defaults selected values are read-only; apply is not supported.")
		}
		if err := validateExistingDesiredForPlanning(repoRoot, read, rec, setting, trustContext); err != nil {
			appendDesiredDiagnostics(&item, err)
			return finishBlocked(item, v2status.StateBlockedSafety, "Desired selected-value entry is blocked by write-safety policy.")
		}
	}
	if command == CommandSave {
		if isMacOSDefaultsReadOnlyDriver(resource.Driver) {
			item.Diagnostics = append(item.Diagnostics, readOnlyDiagnostic(item))
			return finishBlocked(item, v2status.StateUnsupported, "macOS defaults selected values are read-only; save is not supported.")
		}
		if err := validateCurrentForSavePlanning(repoRoot, rec, setting, locationRoots, trustContext); err != nil {
			if planErr, ok := err.(*selectedvalue.PlanError); ok {
				appendPlanDiagnostics(&item, &selectedvalue.Plan{Diagnostics: planErr.Diagnostics})
			} else {
				appendDesiredDiagnostics(&item, err)
			}
			return finishBlocked(item, v2status.StateBlockedSafety, "Current selected value is blocked by write-safety policy.")
		}
	}

	plan, err := selectedvalue.PlanPreview(selectedvalue.PreviewRequest{Request: selectedvalue.Request{Recipe: rec, SettingRef: setting.Ref(), LocationRoots: locationRoots, MacOSDefaultsRunner: opts.MacOSDefaultsRunner}, Desired: *read.Desired, WriteSafetyContext: trustContext})
	if err != nil {
		appendPlanDiagnostics(&item, plan)
		return finishBlocked(item, v2status.StateBlockedSafety, "Selected-value driver preview is blocked.")
	}
	applyPlanToItem(&item, plan)
	deriveItemState(&item, command, read.Intent, opts.StateRoot)
	item.PlannedAction = plannedAction(command, item)
	if command == CommandDiff {
		item.Diff = diffInfo(item.Preview.ChangeKind)
	}
	if command == CommandSave {
		item.Preview.ChangeKind = saveChangeKind(item.Current, item.Desired.Snapshot)
		item.Preview.Intent = saveIntent(item.Current)
	}
	return applyLifecyclePreview(item, rec, setting, resourceID, command, opts)
}

func loadRuntimeRecipe(repoRoot string, targetID string) (recipe.RuntimeRecipe, []Diagnostic) {
	runtime, err := recipe.LoadRuntime(repoRoot, targetID)
	if err != nil {
		code := "selectedpreview.recipe.notFound"
		switch {
		case errors.Is(err, recipe.ErrBundledRuntimeUnavailable):
			code = "selectedpreview.recipe.bundledRuntimeUnavailable"
		case !errors.Is(err, os.ErrNotExist):
			code = "selectedpreview.recipe.invalid"
		}
		if runtime.Source == "" {
			runtime.Source = recipe.RecipeSourceLocal
		}
		if runtime.RecipeRef == "" {
			runtime.RecipeRef = recipeRef(runtime.Source, targetID)
		}
		return runtime, []Diagnostic{diagnostic(code, SeverityError, err.Error(), targetID)}
	}
	return runtime, nil
}

func recipeRef(source string, targetID string) string {
	switch source {
	case recipe.RecipeSourceBundled:
		return "recipe://bundled/" + targetID
	case recipe.RecipeSourceLocal:
		return "recipe://local/" + targetID
	default:
		return ""
	}
}

func evaluateTrust(repoRoot string, stateRoot string, source string, rec *recipe.Recipe) (recipe.TrustEvaluation, recipe.WriteSafetyContext) {
	eval, err := recipe.EvaluateRecipeTrust(repoRoot, stateRoot, source, rec)
	if err != nil {
		return recipe.TrustEvaluation{Status: recipe.TrustStatusBlocked, Diagnostics: []recipe.ValidationDiagnostic{{Code: "selectedpreview.trust.evaluate", Severity: recipe.ValidationSeverityError, Message: err.Error(), Path: "$"}}}, recipe.WriteSafetyContext{}
	}
	return eval, eval.WriteSafetyContext(recipe.WriteSafetyContext{})
}

func writeSafetyContextForCommand(ctx recipe.WriteSafetyContext, command string) recipe.WriteSafetyContext {
	if command == CommandApply {
		ctx.HandlesLifecycleActions = true
	}
	return ctx
}

func blockedStateForRecipeDiagnostics(diagnostics []recipe.ValidationDiagnostic) v2status.StateCode {
	for _, diagnostic := range diagnostics {
		if strings.Contains(diagnostic.Code, "lifecycle") {
			return v2status.StateBlockedLifecycle
		}
	}
	return v2status.StateBlockedSafety
}

func applyLifecyclePreview(item Item, rec *recipe.Recipe, setting resolution.ResolvedSetting, resourceID string, command string, opts Options) Item {
	if command != CommandApply || item.State == v2status.StateBlockedSafety || item.State == v2status.StateBlockedLifecycle || item.State == v2status.StateUnsupported {
		return item
	}
	decision := lifecycle.EvaluateBefore(context.Background(), lifecycle.Request{
		Recipe:            rec,
		SettingID:         setting.SettingID,
		SettingRef:        setting.Ref(),
		ResourceID:        resourceID,
		NativeOperationID: lifecycleNativeOperationID(rec, resourceID, command),
		Command:           command,
		DryRun:            true,
		Detector:          opts.LifecycleDetector,
	})
	item.Lifecycle = append(item.Lifecycle, decision.Actions...)
	for _, diagnostic := range lifecycle.RecordsToDiagnostics(decision.Actions) {
		item.Diagnostics = append(item.Diagnostics, Diagnostic{Code: diagnostic.Code, Severity: SeverityError, Message: diagnostic.Message, Ref: item.SettingRef, ResourceID: resourceID, DriverID: item.Resource.DriverID})
	}
	if decision.Blocked {
		message := decision.Message
		if message == "" {
			message = "Lifecycle policy blocks live apply."
		}
		return finishBlocked(item, v2status.StateBlockedLifecycle, message)
	}
	return item
}

func lifecycleNativeOperationID(rec *recipe.Recipe, resourceID string, command string) string {
	if command != CommandApply || rec == nil {
		return ""
	}
	resource, ok := rec.Resources[resourceID]
	if !ok || resource.Driver != recipe.NativeExportDriverID {
		return ""
	}
	return strings.TrimSpace(resource.NativeImportOperation)
}

func buildFileResourceItem(repoRoot string, stateRoot string, command string, item Item, rec *recipe.Recipe, setting resolution.ResolvedSetting, roots map[string]string) Item {
	profile := &resolution.ResolvedProfile{RepoRoot: repoRoot, Settings: []resolution.ResolvedSetting{setting}}
	req := customfiles.Request{Profile: profile, Recipe: rec, SettingRef: setting.Ref(), LocationRoots: roots}

	var plan *customfiles.Plan
	var err error
	switch command {
	case CommandSave:
		plan, err = customfiles.PlanFileSave(req)
	case CommandApply:
		plan, err = customfiles.PlanFileApply(req)
	default:
		plan, err = customfiles.PlanFileRead(req)
	}
	if err != nil {
		item = hydrateFileResourceReadState(item, req)
		appendFileResourceDiagnostics(&item, err)
		return finishBlocked(item, v2status.StateBlockedSafety, "File-resource planning is blocked; no files will be mutated.")
	}
	if plan.Resource.Driver == recipe.FileTreeDriverID {
		return buildFileTreeResourceItem(stateRoot, command, item, plan)
	}

	item.DesiredURI = plan.Setting.DesiredURI
	item.DesiredRelPath = filepath.ToSlash(plan.Setting.DesiredRelPath)
	item.Resource.Path = plan.Preview.Path
	item.Selector = SelectorInfo{Kind: "file", Summary: plan.Resource.Path}

	current := plan.SourceState
	desiredState := plan.DestinationState
	if plan.Operation == customfiles.OperationApply {
		current = plan.DestinationState
		desiredState = plan.SourceState
	}
	item.Current = fromFileState(current)
	item.Desired.Status = desiredStatus(desiredState.Exists)
	item.Desired.Kind = "file"
	item.Desired.Snapshot = fromFileState(desiredState)
	item.Preview = &PreviewInfo{ChangeKind: string(plan.Preview.Change.Kind), Intent: fileResourceIntent(command, current, desiredState)}
	if command == CommandDiff {
		diffKind := string(plan.Preview.Change.Kind)
		if !desiredState.Exists {
			diffKind = "missing-desired"
		}
		item.Diff = fileDiffInfo(diffKind)
	}

	stateItem := v2status.DeriveItem(v2status.Input{Context: statusContext(command), TargetRef: item.TargetRef, SettingRef: item.SettingRef, Desired: normalizedFileState(desiredState), Current: normalizedFileState(current), LastApplied: lastAppliedState(stateRoot, item)})
	item.State = stateItem.State
	item.NoBaseline = stateItem.NoBaseline
	item.Message = stateItem.Message
	item.AllowedActions = stateItem.Actions
	item.PlannedAction = plannedAction(command, item)
	if command == CommandSave && !plan.DestinationState.Exists && plan.SourceState.Exists {
		item.PlannedAction = PlannedActionWouldPromote
		item.Message = "Existing live file can be synced to stored settings with save --yes; raw file contents remain omitted from output."
	}
	return item
}

func buildNativeExportItem(repoRoot string, stateRoot string, command string, item Item, rec *recipe.Recipe, source string, trustEval recipe.TrustEvaluation, setting resolution.ResolvedSetting, resourceID string, resource recipe.Resource, roots map[string]string, opts Options) Item {
	op := rec.NativeOperations[resource.NativeOperation]
	item.Resource = ResourceInfo{ID: resourceID, DriverID: resource.Driver, LocationID: resource.Location, RelPath: resource.Path, DisplayPath: recipe.FriendlyLocationPath(rec, resource.Location, resource.Path, roots)}
	item.Selector = SelectorInfo{Kind: "native-export", Summary: resource.NativeOperation}
	item.DesiredURI = setting.DesiredURI
	item.DesiredRelPath = filepath.ToSlash(setting.DesiredRelPath)
	item.Desired.Kind = "native-export"
	item.NativeExport = &NativeExportInfo{
		OperationID:       resource.NativeOperation,
		ImportOperationID: resource.NativeImportOperation,
		VerifyOperationID: resource.NativeVerifyOperation,
		ArtifactForm:      op.ArtifactForm,
		DiffMode:          op.DiffMode,
		Redaction:         op.Redaction,
		ReviewRequired:    nativeexport.ReviewRequired(op),
		ApplySupported:    nativeapply.ImportCapable(resource),
		BackupPolicy:      resource.NativeApply.Backup,
		VerifyPolicy:      resource.NativeApply.Verify,
		Limitations:       append([]string(nil), op.ExportMetadata.Limitations...),
	}
	expected := nativeexport.Expected(nativeexport.Options{Recipe: rec, Setting: setting, ResourceID: resourceID, Resource: resource})
	desiredRead := nativeexport.ReadDesired(setting.DesiredPath, expected)
	switch desiredRead.Status {
	case "present":
		item.Desired.Status = "present"
		item.Desired.Snapshot = nativeSnapshot(nativeexport.Snapshot(desiredRead.Metadata))
	case "missing":
		item.Desired.Status = "missing"
	default:
		item.Diagnostics = append(item.Diagnostics, Diagnostic{Code: desiredRead.Diagnostic.Code, Severity: SeverityError, Message: desiredRead.Diagnostic.Message, Ref: item.SettingRef, Path: desiredRead.Diagnostic.Path, ResourceID: resourceID, DriverID: resource.Driver})
		return finishBlocked(item, v2status.StateBlockedSafety, "Desired native export artifact is not manager-owned or has invalid metadata.")
	}

	if command == CommandApply {
		if !nativeapply.ImportCapable(resource) {
			item.Diagnostics = append(item.Diagnostics, diagnostic("selectedpreview.nativeExport.applyUnsupported", SeverityError, "native import/apply is not declared for this resource", item.SettingRef))
			return finishBlocked(item, v2status.StateUnsupported, "Native export apply is unsupported because the recipe declares export-only behavior.")
		}
		plan, err := nativeapply.BuildPlan(nativeapply.Options{
			RepoRoot:           repoRoot,
			StateRoot:          stateRoot,
			Recipe:             rec,
			RecipeSource:       source,
			TrustEvaluation:    &trustEval,
			Setting:            setting,
			ResourceID:         resourceID,
			Resource:           resource,
			MachineID:          opts.MachineID,
			UserID:             opts.UserID,
			RunID:              defaultString(opts.RunID, RunID),
			LocationRoots:      roots,
			Now:                opts.Now,
			ExecutableResolver: opts.NativeResolver,
			Executor:           opts.NativeExecutor,
		})
		if err != nil || plan.Status != nativeapply.StatusReady {
			diag := plan.Diagnostic
			if diag.Code == "" {
				diag = nativeapply.Diagnostic{Code: "selectedpreview.nativeApply.planBlocked", Message: "native apply plan is blocked", Path: item.SettingRef}
			}
			item.Diagnostics = append(item.Diagnostics, Diagnostic{Code: diag.Code, Severity: SeverityError, Message: diag.Message, Ref: item.SettingRef, Path: diag.Path, ResourceID: resourceID, DriverID: resource.Driver})
			state := v2status.StateBlockedSafety
			if strings.Contains(diag.Code, "lifecycle") {
				state = v2status.StateBlockedLifecycle
			}
			return finishBlocked(item, state, "Native apply is blocked before any native operation can run.")
		}
		item.NativeExport.ImportOperationID = plan.ImportOperationID
		item.NativeExport.VerifyOperationID = plan.VerifyOperationID
		item.NativeExport.BackupPolicy = plan.BackupPolicy
		item.NativeExport.VerifyPolicy = plan.VerifyPolicy
		item.NativeExport.ApplySupported = true
		item.NativeExport.ReviewRequired = plan.ReviewRequired
		item.NativeExport.Limitations = append([]string(nil), plan.Limitations...)
		item.Desired.Status = "present"
		item.Desired.Snapshot = nativeSnapshot(plan.DesiredSummary)
		item.Current = Snapshot{}
		item.Preview = &PreviewInfo{ChangeKind: "native-apply-plan", Intent: desired.IntentSet}
		item.State = v2status.StateReadyToApply
		item.Message = "Native apply plan is ready; apply --yes will back up with a pre-apply export, import from a manager-owned temp copy, and verify by post-import export hash. Internal app settings are not semantically diffed."
		item.AllowedActions = []v2status.Action{v2status.ActionApply, v2status.ActionDiff, v2status.ActionSkip}
		item.PlannedAction = PlannedActionWouldApply
		return item
	}
	if command == CommandStatus {
		item.State = v2status.StateUnknown
		item.Message = "Native export status does not run the export operation in this tranche; use diff to compare metadata before choosing a sync direction."
		item.AllowedActions = []v2status.Action{v2status.ActionDiff, v2status.ActionSave}
		return item
	}
	if nativeexport.ReviewRequired(op) && !opts.Confirmed {
		review := nativeexport.ReviewDiagnostic(item.SettingRef, op)
		item.Diagnostics = append(item.Diagnostics, Diagnostic{Code: review.Code, Severity: SeverityError, Message: review.Message, Ref: item.SettingRef, Path: review.Path, ResourceID: resourceID, DriverID: resource.Driver})
		return finishBlocked(item, v2status.StateBlockedSafety, "Native export requires explicit confirmation before the export operation runs.")
	}

	runID := opts.RunID
	if strings.TrimSpace(runID) == "" {
		runID = RunID
	}
	export, err := nativeexport.Export(context.Background(), nativeexport.Options{
		RepoRoot:           repoRoot,
		StateRoot:          stateRoot,
		Recipe:             rec,
		RecipeSource:       source,
		TrustEvaluation:    &trustEval,
		Setting:            setting,
		ResourceID:         resourceID,
		Resource:           resource,
		MachineID:          opts.MachineID,
		UserID:             opts.UserID,
		RunID:              runID,
		LocationRoots:      roots,
		Now:                opts.Now,
		ExecutableResolver: opts.NativeResolver,
		Executor:           opts.NativeExecutor,
	})
	if err != nil || export.Status != nativeexport.StatusSucceeded {
		diag := export.Diagnostic
		if diag.Code == "" {
			diag = nativeexport.Diagnostic{Code: "selectedpreview.nativeExport.failed", Message: "native export failed", Path: item.SettingRef}
		}
		item.Diagnostics = append(item.Diagnostics, Diagnostic{Code: diag.Code, Severity: SeverityError, Message: diag.Message, Ref: item.SettingRef, Path: diag.Path, ResourceID: resourceID, DriverID: resource.Driver})
		return finishBlocked(item, v2status.StateBlockedSafety, "Native export operation did not produce a valid managed artifact.")
	}
	item.NativeExport.StagingRoot = export.StagingRoot
	item.NativeExport.PayloadRoot = export.PayloadRoot
	item.Current = nativeSnapshot(export.Metadata.Payload)
	item.Preview = &PreviewInfo{ChangeKind: nativeexport.ChangeKind(export.Metadata.Payload, nativeexport.Snapshot(desiredRead.Metadata)), Intent: desired.IntentSet}
	if command == CommandDiff {
		diffKind := item.Preview.ChangeKind
		if desiredRead.Status == "missing" {
			diffKind = "missing-desired"
		}
		item.Diff = nativeExportDiffInfo(diffKind)
	}
	stateItem := v2status.DeriveItem(v2status.Input{Context: statusContext(command), TargetRef: item.TargetRef, SettingRef: item.SettingRef, Desired: normalizedNativeState(nativeexport.Snapshot(desiredRead.Metadata)), Current: normalizedNativeState(export.Metadata.Payload), LastApplied: lastAppliedState(stateRoot, item)})
	item.State = stateItem.State
	item.NoBaseline = stateItem.NoBaseline
	item.Message = stateItem.Message
	item.AllowedActions = stateItem.Actions
	item.PlannedAction = plannedAction(command, item)
	if command == CommandSave && desiredRead.Status == "missing" && export.Metadata.Payload.Exists {
		item.PlannedAction = PlannedActionWouldPromote
		item.Message = "Existing native export can be synced to stored settings with save --yes; internal app settings are not semantically diffed."
	}
	return item
}

func hydrateFileResourceReadState(item Item, req customfiles.Request) Item {
	plan, err := customfiles.PlanFileRead(req)
	if err != nil || plan == nil {
		return item
	}
	item.DesiredURI = plan.Setting.DesiredURI
	item.DesiredRelPath = filepath.ToSlash(plan.Setting.DesiredRelPath)
	switch plan.Resource.Driver {
	case recipe.FileTreeDriverID:
		item.Resource.Path = plan.TreePreview.Path
		item.Selector = SelectorInfo{Kind: "file-tree", Summary: plan.Resource.Path}
		item.Current = fromTreeState(plan.TreeSourceState)
		item.Desired.Status = desiredStatus(plan.TreeDestinationState.Exists)
		item.Desired.Kind = "file-tree"
		item.Desired.Snapshot = fromTreeState(plan.TreeDestinationState)
		item.Preview = &PreviewInfo{ChangeKind: string(plan.TreePreview.Change.Kind)}
	case recipe.FileDriverID:
		item.Resource.Path = plan.Preview.Path
		item.Selector = SelectorInfo{Kind: "file", Summary: plan.Resource.Path}
		item.Current = fromFileState(plan.SourceState)
		item.Desired.Status = desiredStatus(plan.DestinationState.Exists)
		item.Desired.Kind = "file"
		item.Desired.Snapshot = fromFileState(plan.DestinationState)
		item.Preview = &PreviewInfo{ChangeKind: string(plan.Preview.Change.Kind)}
	}
	return item
}

func buildFileTreeResourceItem(stateRoot string, command string, item Item, plan *customfiles.Plan) Item {
	item.DesiredURI = plan.Setting.DesiredURI
	item.DesiredRelPath = filepath.ToSlash(plan.Setting.DesiredRelPath)
	item.Resource.Path = plan.TreePreview.Path
	item.Selector = SelectorInfo{Kind: "file-tree", Summary: plan.Resource.Path}

	current := plan.TreeSourceState
	desiredState := plan.TreeDestinationState
	if plan.Operation == customfiles.OperationApply {
		current = plan.TreeDestinationState
		desiredState = plan.TreeSourceState
	}
	item.Current = fromTreeState(current)
	item.Desired.Status = desiredStatus(desiredState.Exists)
	item.Desired.Kind = "file-tree"
	item.Desired.Snapshot = fromTreeState(desiredState)
	item.Preview = &PreviewInfo{ChangeKind: string(plan.TreePreview.Change.Kind), Intent: fileTreeResourceIntent(command, current, desiredState)}
	if command == CommandApply {
		if operations := fileTreeOperationsFromPreview(plan.TreePreview, FileTreeOperationStatePlanned); len(operations) > 0 {
			item.FileTree = &FileTreeInfo{Operations: operations}
		}
	}
	if command == CommandDiff {
		diffKind := string(plan.TreePreview.Change.Kind)
		if !desiredState.Exists {
			diffKind = "missing-desired"
		}
		item.Diff = treeDiffInfo(diffKind)
	}

	stateItem := v2status.DeriveItem(v2status.Input{Context: statusContext(command), TargetRef: item.TargetRef, SettingRef: item.SettingRef, Desired: normalizedTreeState(desiredState), Current: normalizedTreeState(current), LastApplied: lastAppliedState(stateRoot, item)})
	item.State = stateItem.State
	item.NoBaseline = stateItem.NoBaseline
	item.Message = stateItem.Message
	item.AllowedActions = stateItem.Actions
	item.PlannedAction = plannedAction(command, item)
	if command == CommandSave && !plan.TreeDestinationState.Exists && plan.TreeSourceState.Exists {
		item.PlannedAction = PlannedActionWouldPromote
		item.Message = "Existing live file tree can be synced to stored settings with save --yes; raw file contents remain omitted from output."
	}
	return item
}

func fileTreeOperationsFromPreview(preview filetreedriver.Preview, state string) []FileTreeOperation {
	if len(preview.Change.Entries) == 0 {
		return nil
	}
	operations := make([]FileTreeOperation, 0, len(preview.Change.Entries))
	for _, entry := range preview.Change.Entries {
		operation, ok := fileTreeOperationFromEntry(entry, state)
		if ok {
			operations = append(operations, operation)
		}
	}
	return operations
}

func fileTreeOperationFromEntry(entry filetreedriver.EntryDiff, state string) (FileTreeOperation, bool) {
	path := filepath.ToSlash(strings.TrimSpace(entry.Path))
	if !safeFileTreeOperationPath(path) {
		return FileTreeOperation{}, false
	}
	operation := FileTreeOperation{Path: path, State: defaultString(state, FileTreeOperationStatePlanned)}
	switch entry.Kind {
	case filedriver.ChangeCreate:
		operation.Action = FileTreeOperationActionCreate
		operation.Kind = fileTreeOperationKind(entry.After.Kind)
	case filedriver.ChangeUpdate:
		operation.Action = FileTreeOperationActionUpdate
		operation.Kind = fileTreeOperationKind(entry.After.Kind)
		if operation.Kind == "" {
			operation.Kind = fileTreeOperationKind(entry.Before.Kind)
		}
	case filedriver.ChangeDelete:
		operation.Action = FileTreeOperationActionRemove
		operation.Kind = fileTreeOperationKind(entry.Before.Kind)
	default:
		return FileTreeOperation{}, false
	}
	if operation.Kind == "" {
		return FileTreeOperation{}, false
	}
	return operation, true
}

func safeFileTreeOperationPath(path string) bool {
	if path == "" || strings.HasPrefix(path, "/") {
		return false
	}
	for _, part := range strings.Split(path, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func fileTreeOperationKind(kind filetreedriver.EntryKind) string {
	switch kind {
	case filetreedriver.EntryFile:
		return FileTreeOperationKindFile
	case filetreedriver.EntryDir:
		return FileTreeOperationKindDirectory
	default:
		return ""
	}
}

func SetFileTreeOperationState(item *Item, state string) {
	if item == nil || item.FileTree == nil {
		return
	}
	normalized := strings.TrimSpace(state)
	if normalized == "" {
		normalized = FileTreeOperationStatePlanned
	}
	for idx := range item.FileTree.Operations {
		item.FileTree.Operations[idx].State = normalized
	}
}

func buildMissingDesiredItem(repoRoot string, item Item, rec *recipe.Recipe, setting resolution.ResolvedSetting, roots map[string]string, command string, trustContext recipe.WriteSafetyContext, defaultsRunner macosdefaultsdriver.Runner) Item {
	if isMacOSDefaultsReadOnlyDriver(item.Resource.DriverID) && (command == CommandSave || command == CommandApply) {
		item.Diagnostics = append(item.Diagnostics, readOnlyDiagnostic(item))
		return finishBlocked(item, v2status.StateUnsupported, "macOS defaults selected values are read-only; save/apply are not supported.")
	}
	if command == CommandApply {
		item.State = v2status.StateMissingDesired
		item.Message = "Selected setting has no stored settings; apply dry-run cannot change live state."
		item.AllowedActions = []v2status.Action{v2status.ActionSave, v2status.ActionCreate}
		item.PlannedAction = PlannedActionBlockedMissingDesired
		return item
	}
	plan, err := selectedvalue.PlanRead(selectedvalue.Request{Recipe: rec, SettingRef: setting.Ref(), LocationRoots: roots, MacOSDefaultsRunner: defaultsRunner})
	if err != nil {
		appendPlanDiagnostics(&item, plan)
		return finishBlocked(item, v2status.StateBlockedSafety, "Selected-value driver read is blocked.")
	}
	applyReadPlanToItem(&item, plan)
	if command == CommandSave {
		current, err := selectedvalue.ReadCurrentDesired(selectedvalue.Request{Recipe: rec, SettingRef: setting.Ref(), LocationRoots: roots, MacOSDefaultsRunner: defaultsRunner})
		if err != nil {
			appendPlanDiagnostics(&item, current.Plan)
			return finishBlocked(item, v2status.StateBlockedSafety, "Selected-value driver read is blocked.")
		}
		saveValue, err := desiredValueFromSelected(current.Desired)
		if err != nil {
			item.Diagnostics = append(item.Diagnostics, diagnostic("selectedpreview.current.invalid", SeverityError, "current selected value cannot be represented as desired state", item.SettingRef))
			return finishBlocked(item, v2status.StateBlockedSafety, "Current selected value is invalid.")
		}
		if err := desired.ValidateSelectedValueWriteSafety(desired.WriteRequest{RepoRoot: repoRoot, URI: setting.DesiredURI, Value: saveValue, Safety: &desired.WriteSafetyDecision{Recipe: rec, SettingRef: setting.Ref(), Context: trustContext}}); err != nil {
			appendDesiredDiagnostics(&item, err)
			return finishBlocked(item, v2status.StateBlockedSafety, "Current selected value is blocked by write-safety policy.")
		}
	}
	stateItem := v2status.DeriveItem(v2status.Input{Context: statusContext(command), TargetRef: item.TargetRef, SettingRef: item.SettingRef, Desired: v2status.NormalizedState{Exists: false}, Current: normalizedSnapshot(item.Current)})
	item.State = stateItem.State
	item.NoBaseline = stateItem.NoBaseline
	item.Message = stateItem.Message
	item.AllowedActions = stateItem.Actions
	if command == CommandSave {
		item.PlannedAction = plannedSaveActionForMissingDesired(item)
		item.Preview = &PreviewInfo{ChangeKind: "create", Intent: saveIntent(item.Current)}
		if item.PlannedAction == PlannedActionWouldPromote {
			item.Message = "Existing live selected value can be synced to stored settings with save --yes; raw value remains redacted in output."
		}
	}
	if command == CommandDiff {
		item.Diff = diffInfo("missing-desired")
	}
	return item
}

func applyPlanToItem(item *Item, plan *selectedvalue.Plan) {
	if plan == nil {
		return
	}
	item.Resource.Path = plan.Path
	item.Selector = selectorInfo(plan.Selector)
	item.Current = fromSnapshot(plan.Current)
	if plan.Desired != nil {
		item.Desired.Snapshot = fromSnapshot(*plan.Desired)
	}
	item.Preview = &PreviewInfo{ChangeKind: plan.ChangeKind, Intent: plan.Intent, ReadOnly: plan.ReadOnly}
	appendPlanDiagnostics(item, plan)
}

func applyReadPlanToItem(item *Item, plan *selectedvalue.Plan) {
	if plan == nil {
		return
	}
	item.Resource.Path = plan.Path
	item.Selector = selectorInfo(plan.Selector)
	item.Current = fromSnapshot(plan.Current)
	appendPlanDiagnostics(item, plan)
}

func appendPlanDiagnostics(item *Item, plan *selectedvalue.Plan) {
	if item == nil || plan == nil {
		return
	}
	for _, diagnostic := range plan.Diagnostics {
		item.Diagnostics = append(item.Diagnostics, Diagnostic{Code: diagnostic.Code, Severity: diagnostic.Severity, Message: diagnostic.Message, Ref: diagnostic.Ref, Path: diagnostic.Path, ResourceID: diagnostic.ResourceID, DriverID: diagnostic.DriverID})
	}
}

func appendWriteSafetyWarnings(item *Item, command string, rec *recipe.Recipe, setting resolution.ResolvedSetting, resourceID string, driverID string, source string, ctx recipe.WriteSafetyContext) {
	if item == nil || rec == nil || (command != CommandSave && command != CommandApply) {
		return
	}
	settingPath := "$.settings." + setting.SettingID
	resourcePath := "$.resources." + resourceID
	seen := map[string]bool{}
	for _, diagnostic := range item.Diagnostics {
		if diagnostic.Severity == SeverityWarning {
			seen[diagnostic.Code] = true
		}
	}
	for _, warning := range rec.WriteSafetyDiagnostics(ctx) {
		if warning.Severity != recipe.ValidationSeverityWarning {
			continue
		}
		if !strings.HasPrefix(warning.Path, settingPath) && !strings.HasPrefix(warning.Path, resourcePath) {
			continue
		}
		if seen[warning.Code] {
			continue
		}
		item.Diagnostics = append(item.Diagnostics, fromRecipeDiagnostic(warning, item.SettingRef, source, resourceID, driverID))
		seen[warning.Code] = true
	}
	for _, warning := range rec.WriteReviewDiagnostics(command, setting.SettingID, resourceID) {
		if warning.Severity != recipe.ValidationSeverityWarning || seen[warning.Code] {
			continue
		}
		item.Diagnostics = append(item.Diagnostics, fromRecipeDiagnostic(warning, item.SettingRef, source, resourceID, driverID))
		seen[warning.Code] = true
	}
}

func appendDesiredDiagnostics(item *Item, err error) {
	if item == nil || err == nil {
		return
	}
	var safetyErr *desired.SafetyError
	if errors.As(err, &safetyErr) {
		for _, diagnostic := range safetyErr.Diagnostics {
			item.Diagnostics = append(item.Diagnostics, Diagnostic{Code: diagnostic.Code, Severity: diagnostic.Severity, Message: diagnostic.Message, Ref: item.SettingRef, Path: diagnostic.Path, ResourceID: item.Resource.ID, DriverID: item.Resource.DriverID})
		}
		return
	}
	item.Diagnostics = append(item.Diagnostics, Diagnostic{Code: "selectedpreview.desired.writeSafety", Severity: SeverityError, Message: "selected-value write-safety policy blocked planning", Ref: item.SettingRef, ResourceID: item.Resource.ID, DriverID: item.Resource.DriverID})
}

func validateExistingDesiredForPlanning(repoRoot string, read desired.ReadResult, rec *recipe.Recipe, setting resolution.ResolvedSetting, trustContext recipe.WriteSafetyContext) error {
	if read.Value == nil {
		return fmt.Errorf("desired selected-value entry is missing raw validation state")
	}
	return desired.ValidateSelectedValueWriteSafety(desired.WriteRequest{RepoRoot: repoRoot, URI: setting.DesiredURI, Value: *read.Value, Safety: &desired.WriteSafetyDecision{Recipe: rec, SettingRef: setting.Ref(), Context: trustContext}})
}

func validateCurrentForSavePlanning(repoRoot string, rec *recipe.Recipe, setting resolution.ResolvedSetting, roots map[string]string, trustContext recipe.WriteSafetyContext) error {
	current, err := selectedvalue.ReadCurrentDesired(selectedvalue.Request{Recipe: rec, SettingRef: setting.Ref(), LocationRoots: roots})
	if err != nil {
		if current != nil && current.Plan != nil {
			return &selectedvalue.PlanError{Diagnostics: current.Plan.Diagnostics}
		}
		return err
	}
	saveValue, err := desiredValueFromSelected(current.Desired)
	if err != nil {
		return err
	}
	return desired.ValidateSelectedValueWriteSafety(desired.WriteRequest{RepoRoot: repoRoot, URI: setting.DesiredURI, Value: saveValue, Safety: &desired.WriteSafetyDecision{Recipe: rec, SettingRef: setting.Ref(), Context: trustContext}})
}

func desiredValueFromSelected(value selectedvalue.Desired) (desired.SelectedValue, error) {
	switch value.Intent() {
	case selectedvalue.IntentDelete:
		return desired.Delete(), nil
	case selectedvalue.IntentSet:
		raw, ok := value.Value()
		if !ok {
			return desired.SelectedValue{}, fmt.Errorf("selected desired value is missing")
		}
		switch value.Kind() {
		case "string":
			typed, ok := raw.(string)
			if !ok {
				return desired.SelectedValue{}, fmt.Errorf("selected desired string has invalid representation")
			}
			return desired.SetString(typed), nil
		case "bool":
			typed, ok := raw.(bool)
			if !ok {
				return desired.SelectedValue{}, fmt.Errorf("selected desired bool has invalid representation")
			}
			return desired.SetBool(typed), nil
		case "number":
			typed, ok := raw.(json.Number)
			if !ok {
				return desired.SelectedValue{}, fmt.Errorf("selected desired number has invalid representation")
			}
			return desired.SetNumber(typed), nil
		case "null":
			return desired.SetNull(), nil
		default:
			return desired.SelectedValue{}, fmt.Errorf("unsupported selected desired kind")
		}
	default:
		return desired.SelectedValue{}, fmt.Errorf("selected desired intent is required")
	}
}

func deriveItemState(item *Item, command string, desiredIntent string, stateRoot string) {
	desiredState := normalizedSnapshot(item.Desired.Snapshot)
	currentState := normalizedSnapshot(item.Current)
	if desiredIntent == desired.IntentDelete {
		desiredState = deleteSentinel(item.Desired.Snapshot.Normalizer)
		if !item.Current.Exists {
			currentState = deleteSentinel(item.Desired.Snapshot.Normalizer)
		}
	}
	stateItem := v2status.DeriveItem(v2status.Input{Context: statusContext(command), TargetRef: item.TargetRef, SettingRef: item.SettingRef, Desired: desiredState, Current: currentState, LastApplied: lastAppliedState(stateRoot, *item)})
	item.State = stateItem.State
	item.NoBaseline = stateItem.NoBaseline
	item.Message = stateItem.Message
	item.AllowedActions = stateItem.Actions
}

func statusContext(command string) v2status.Context {
	switch command {
	case CommandSave:
		return v2status.ContextSave
	case CommandApply:
		return v2status.ContextApply
	default:
		return v2status.ContextStatus
	}
}

func lastAppliedState(stateRoot string, item Item) *v2status.NormalizedState {
	if strings.TrimSpace(stateRoot) == "" || strings.TrimSpace(item.SettingRef) == "" {
		return nil
	}
	store, err := v2ledger.NewStore(stateRoot)
	if err != nil {
		return nil
	}
	ledgerItem, ok, err := store.LatestVerifiedItem(v2ledger.VerifiedItemLookup{
		TargetRef:      item.TargetRef,
		SettingRef:     item.SettingRef,
		ResourceID:     item.Resource.ID,
		Driver:         item.Resource.DriverID,
		DesiredURI:     item.DesiredURI,
		DesiredRelPath: item.DesiredRelPath,
		LivePath:       item.Resource.Path,
	})
	if err != nil || !ok {
		return nil
	}
	state := v2status.NormalizedState{
		Exists:        ledgerItem.VerifiedState.Exists,
		Hash:          ledgerItem.VerifiedState.Hash,
		Normalizer:    ledgerItem.VerifiedState.Normalizer,
		DriverVersion: ledgerItem.VerifiedState.DriverVersion,
	}
	return &state
}

func normalizedSnapshot(snapshot Snapshot) v2status.NormalizedState {
	return v2status.NormalizedState{Exists: snapshot.Exists, Hash: snapshot.SHA256, Normalizer: snapshot.Normalizer}
}

func deleteSentinel(normalizer string) v2status.NormalizedState {
	if normalizer == "" {
		normalizer = "selected-value.delete.v1"
	}
	return v2status.NormalizedState{Exists: true, Hash: "selected-value-delete", Normalizer: normalizer}
}

func fromSnapshot(snapshot selectedvalue.Snapshot) Snapshot {
	return Snapshot{Exists: snapshot.Exists, SHA256: snapshot.SHA256, Normalizer: snapshot.Normalizer}
}

func selectorInfo(selector selectedvalue.SelectorInfo) SelectorInfo {
	return SelectorInfo{Kind: selector.Kind, Summary: selector.Summary, Section: selector.Section, Key: selector.Key, Path: append([]string(nil), selector.Path...)}
}

func selectorFromRecipe(resource recipe.Resource) SelectorInfo {
	if resource.Selector == nil {
		return SelectorInfo{Kind: "none"}
	}
	switch resource.Driver {
	case recipe.IniFileDriverID:
		return SelectorInfo{Kind: "ini-key", Summary: fmt.Sprintf("[%s] %s", resource.Selector.Section, resource.Selector.Key), Section: resource.Selector.Section, Key: resource.Selector.Key}
	case recipe.JSONFileDriverID, recipe.YAMLFileDriverID, recipe.TOMLFileDriverID:
		return SelectorInfo{Kind: "selected-path", Summary: strings.Join(resource.Selector.Path, "."), Path: append([]string(nil), resource.Selector.Path...)}
	case recipe.PlistFileDriverID:
		return SelectorInfo{Kind: "selected-path", Summary: quotedPathSummary(resource.Selector.Path), Path: append([]string(nil), resource.Selector.Path...)}
	case recipe.MacOSDefaultsReadOnlyDriverID:
		return SelectorInfo{Kind: "macos-defaults-key", Summary: macosdefaultsdriver.SelectorSummary(resource.Path, resource.Selector.Key), Key: resource.Selector.Key}
	default:
		return SelectorInfo{Kind: "unsupported"}
	}
}

func isSelectedValueDriver(driver string) bool {
	switch driver {
	case recipe.IniFileDriverID, recipe.JSONFileDriverID, recipe.YAMLFileDriverID, recipe.TOMLFileDriverID, recipe.PlistFileDriverID, recipe.MacOSDefaultsReadOnlyDriverID:
		return true
	default:
		return false
	}
}

func isMacOSDefaultsReadOnlyDriver(driver string) bool {
	return driver == recipe.MacOSDefaultsReadOnlyDriverID
}

func quotedPathSummary(path []string) string {
	data, err := json.Marshal(path)
	if err != nil {
		return fmt.Sprintf("%q", path)
	}
	return string(data)
}

func plannedAction(command string, item Item) string {
	if item.State == v2status.StateBlockedSafety || item.State == v2status.StateUnsupported || item.State == v2status.StateBlockedLifecycle {
		return ""
	}
	switch command {
	case CommandSave:
		if item.State == v2status.StateUnchanged {
			return PlannedActionNone
		}
		return PlannedActionWouldSave
	case CommandApply:
		if item.State == v2status.StateUnchanged {
			return PlannedActionNone
		}
		return PlannedActionWouldApply
	}
	return ""
}

func plannedSaveActionForMissingDesired(item Item) string {
	if item.Current.Exists {
		return PlannedActionWouldPromote
	}
	return PlannedActionWouldSave
}

func IsSavePlannedAction(action string) bool {
	return action == PlannedActionWouldSave || action == PlannedActionWouldPromote
}

func saveIntent(current Snapshot) string {
	if current.Exists {
		return desired.IntentSet
	}
	return desired.IntentDelete
}

func saveChangeKind(current Snapshot, desired Snapshot) string {
	switch {
	case current.Exists && !desired.Exists:
		return "create"
	case !current.Exists && desired.Exists:
		return "delete"
	case !current.Exists && !desired.Exists:
		return "unchanged"
	case current.SHA256 == desired.SHA256 && current.Normalizer == desired.Normalizer:
		return "unchanged"
	default:
		return "update"
	}
}

func diffInfo(kind string) *DiffInfo {
	if kind == "" {
		kind = "unknown"
	}
	return &DiffInfo{Kind: kind, Mode: "metadata-only", Redaction: "raw selected values omitted", Message: "Selected scalar diff is redacted; compare normalized metadata only."}
}

func fileDiffInfo(kind string) *DiffInfo {
	if kind == "" {
		kind = "unknown"
	}
	return &DiffInfo{Kind: kind, Mode: "metadata-only", Redaction: "raw file contents omitted", Message: "File-resource diff is metadata-only in this slice; compare existence, size, hash, and normalizer."}
}

func treeDiffInfo(kind string) *DiffInfo {
	if kind == "" {
		kind = "unknown"
	}
	return &DiffInfo{Kind: kind, Mode: "metadata-only", Redaction: "raw file-tree contents omitted", Message: "File-tree diff is metadata-only; compare existence, entry counts, hash, and normalizer."}
}

func nativeExportDiffInfo(kind string) *DiffInfo {
	if kind == "" {
		kind = "unknown"
	}
	return &DiffInfo{Kind: kind, Mode: "metadata-only", Redaction: "raw native export contents omitted", Message: "Opaque native export diff is metadata-only; internal app settings are not semantically compared."}
}

func fromFileState(state filedriver.State) Snapshot {
	snapshot := state.Snapshot()
	return Snapshot{Exists: snapshot.Exists, SHA256: snapshot.SHA256, Normalizer: state.Normalizer, Size: snapshot.Size}
}

func fromTreeState(state filetreedriver.State) Snapshot {
	snapshot := state.Snapshot()
	return Snapshot{
		Exists:     snapshot.Exists,
		SHA256:     snapshot.SHA256,
		Normalizer: state.Normalizer,
		EntryCount: snapshot.EntryCount,
		FileCount:  snapshot.FileCount,
		DirCount:   snapshot.DirCount,
	}
}

func normalizedFileState(state filedriver.State) v2status.NormalizedState {
	return v2status.NormalizedState{Exists: state.Exists, Hash: state.SHA256, Normalizer: state.Normalizer}
}

func normalizedTreeState(state filetreedriver.State) v2status.NormalizedState {
	return v2status.FromFileTreeState(state)
}

func nativeSnapshot(summary nativeexport.PayloadSummary) Snapshot {
	return Snapshot{
		Exists:     summary.Exists,
		SHA256:     summary.SHA256,
		Normalizer: summary.Normalizer,
		Size:       int(summary.Size),
		EntryCount: summary.EntryCount,
		FileCount:  summary.FileCount,
		DirCount:   summary.DirCount,
	}
}

func normalizedNativeState(summary nativeexport.PayloadSummary) v2status.NormalizedState {
	return v2status.NormalizedState{Exists: summary.Exists, Hash: summary.SHA256, Normalizer: summary.Normalizer, DriverVersion: nativeexport.DriverVersion}
}

func desiredStatus(exists bool) string {
	if exists {
		return "present"
	}
	return "missing"
}

func fileResourceIntent(command string, current filedriver.State, desiredState filedriver.State) string {
	switch command {
	case CommandSave:
		if current.Exists {
			return desired.IntentSet
		}
	case CommandApply:
		if desiredState.Exists {
			return desired.IntentSet
		}
	}
	return ""
}

func fileTreeResourceIntent(command string, current filetreedriver.State, desiredState filetreedriver.State) string {
	switch command {
	case CommandSave:
		if current.Exists {
			return desired.IntentSet
		}
	case CommandApply:
		if desiredState.Exists {
			return desired.IntentSet
		}
	}
	return ""
}

func fileResourceDiagnostic(code string, err error, item Item) Diagnostic {
	message := "file-resource planning failed"
	if err != nil {
		message = err.Error()
	}
	return Diagnostic{Code: code, Severity: SeverityError, Message: message, Ref: item.SettingRef, Path: item.Resource.Path, ResourceID: item.Resource.ID, DriverID: item.Resource.DriverID}
}

func appendFileResourceDiagnostics(item *Item, err error) {
	if item == nil {
		return
	}
	var planErr *customfiles.PlanError
	if errors.As(err, &planErr) && len(planErr.Diagnostics) > 0 {
		for _, diagnostic := range planErr.Diagnostics {
			item.Diagnostics = append(item.Diagnostics, Diagnostic{
				Code:       diagnostic.Code,
				Severity:   diagnostic.Severity,
				Message:    diagnostic.Message,
				Ref:        fallback(diagnostic.Ref, item.SettingRef),
				Path:       fallback(diagnostic.Path, item.Resource.Path),
				ResourceID: fallback(diagnostic.ResourceID, item.Resource.ID),
				DriverID:   fallback(diagnostic.DriverID, item.Resource.DriverID),
			})
		}
		return
	}
	item.Diagnostics = append(item.Diagnostics, fileResourceDiagnostic("selectedpreview.fileResource.plan", err, *item))
}

func fallback(value string, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
}

func finishBlocked(item Item, state v2status.StateCode, message string) Item {
	item.State = state
	item.Message = message
	item.AllowedActions = v2status.DeriveItem(v2status.Input{Context: v2status.ContextStatus, Desired: v2status.NormalizedState{Exists: true, Hash: "blocked", Normalizer: "blocked"}, Current: v2status.NormalizedState{Exists: true, Hash: "blocked", Normalizer: "blocked"}, Blocker: blockerForState(state, message)}).Actions
	return item
}

func blockerForState(state v2status.StateCode, message string) v2status.Blocker {
	switch state {
	case v2status.StateUnsupported:
		return v2status.Blocker{Code: v2status.BlockerUnsupported, Message: message}
	case v2status.StateBlockedLifecycle:
		return v2status.Blocker{Code: v2status.BlockerLifecycle, Message: message}
	case v2status.StateBlockedSafety:
		return v2status.Blocker{Code: v2status.BlockerSafety, Message: message}
	default:
		return v2status.Blocker{Code: v2status.BlockerUnknown, Message: message}
	}
}

func finishReport(report *Report) {
	if report == nil {
		return
	}
	for _, item := range report.Items {
		switch item.State {
		case v2status.StateBlockedSafety, v2status.StateBlockedLifecycle, v2status.StateUnsupported:
			report.Summary.Blocked++
		case v2status.StateUnchanged:
		default:
			report.Summary.Changed++
		}
		if report.Command == CommandSave && IsSavePlannedAction(item.PlannedAction) {
			report.Summary.Saved++
		}
		if report.Command == CommandApply && item.PlannedAction == PlannedActionWouldApply {
			report.Summary.Applied++
		}
	}
	if report.Summary.Blocked > 0 {
		report.Summary.Status = SummaryBlocked
	} else if report.Summary.Changed > 0 || report.Summary.Saved > 0 || report.Summary.Applied > 0 {
		report.Summary.Status = SummaryChanged
	} else {
		report.Summary.Status = SummaryOK
	}
}

func diagnostic(code string, severity string, message string, ref string) Diagnostic {
	if severity == "" {
		severity = SeverityError
	}
	return Diagnostic{Code: code, Severity: severity, Message: message, Ref: ref}
}

func readOnlyDiagnostic(item Item) Diagnostic {
	return Diagnostic{Code: "selectedpreview.driver.readOnly", Severity: SeverityError, Message: "macOS defaults selected values are read-only; status and diff are available, but save/apply are unsupported.", Ref: item.SettingRef, ResourceID: item.Resource.ID, DriverID: item.Resource.DriverID, Path: item.Resource.Path}
}

func (d Diagnostic) withRef(ref string) Diagnostic {
	if d.Ref == "" {
		d.Ref = ref
	}
	return d
}

func fromRecipeDiagnostic(d recipe.ValidationDiagnostic, ref string, source string, resourceID string, driverID string) Diagnostic {
	severity := d.Severity
	if severity == "" {
		severity = SeverityError
	}
	return Diagnostic{Code: d.Code, Severity: severity, Message: d.Message, Ref: ref, Source: source, Path: d.Path, ResourceID: resourceID, DriverID: driverID}
}

func existsLabel(exists bool) string {
	if exists {
		return "present"
	}
	return "missing"
}
