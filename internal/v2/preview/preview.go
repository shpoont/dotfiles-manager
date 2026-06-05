// Package preview renders deterministic v2 preview envelopes for planned
// settings operations. It is intentionally CLI-adjacent rather than CLI-owned:
// callers supply already-resolved plans, run IDs, profile stacks, diagnostics,
// and optional ledger references, then this package normalizes, renders, and
// classifies the result without reading or writing live state.
package preview

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/shpoont/dotfiles-manager/internal/v2/customfiles"
	"github.com/shpoont/dotfiles-manager/internal/v2/filedriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/filetreedriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/shpoont/dotfiles-manager/internal/v2/status"
)

const (
	Schema        = "dotfiles-manager.v2.preview"
	SchemaVersion = 1
)

type Command string

const (
	CommandInit          Command = "init"
	CommandAdd           Command = "add"
	CommandList          Command = "list"
	CommandStatus        Command = "status"
	CommandDiff          Command = "diff"
	CommandSave          Command = "save"
	CommandApply         Command = "apply"
	CommandSync          Command = "sync"
	CommandBackupList    Command = "backup.list"
	CommandRestore       Command = "restore"
	CommandMigrate       Command = "migrate"
	CommandRecipeExplain Command = "recipe.explain"
)

type SummaryStatus string

const (
	SummaryOK      SummaryStatus = "ok"
	SummaryChanged SummaryStatus = "changed"
	SummaryBlocked SummaryStatus = "blocked"
	SummaryPartial SummaryStatus = "partial"
	SummaryError   SummaryStatus = "error"
)

type Result string

const (
	ResultPending     Result = "pending"
	ResultUnchanged   Result = "unchanged"
	ResultWouldChange Result = "would-change"
	ResultSkipped     Result = "skipped"
	ResultBlocked     Result = "blocked"
	ResultSaved       Result = "saved"
	ResultApplied     Result = "applied"
	ResultFailed      Result = "failed"
)

type DiagnosticSeverity string

const (
	SeverityInfo    DiagnosticSeverity = "info"
	SeverityWarning DiagnosticSeverity = "warning"
	SeverityError   DiagnosticSeverity = "error"
)

type BackupPolicy string

const (
	BackupNotApplicable     BackupPolicy = "not-applicable"
	BackupRequired          BackupPolicy = "required"
	BackupUnsupported       BackupPolicy = "unsupported"
	BackupRefSupplied       BackupPolicy = "ref-supplied"
	BackupSkippedForBlocker BackupPolicy = "skipped-blocked"
)

type Envelope struct {
	Schema        string   `json:"schema"`
	SchemaVersion int      `json:"schemaVersion"`
	Command       Command  `json:"command"`
	RunID         string   `json:"runId"`
	ProfileStack  []string `json:"profileStack"`
	Summary       Summary  `json:"summary"`
	Items         []Item   `json:"items"`
	LedgerRef     string   `json:"ledgerRef,omitempty"`
}

type Summary struct {
	Status  SummaryStatus `json:"status"`
	Changed int           `json:"changed"`
	Blocked int           `json:"blocked"`
	Applied int           `json:"applied"`
	Saved   int           `json:"saved"`
	Skipped int           `json:"skipped"`
	Failed  int           `json:"failed"`
}

type Item struct {
	TargetRef      string           `json:"targetRef"`
	SettingRef     string           `json:"settingRef"`
	Scope          string           `json:"scope,omitempty"`
	Subject        string           `json:"subject,omitempty"`
	DesiredURI     string           `json:"desiredUri,omitempty"`
	DesiredRelPath string           `json:"desiredRelPath,omitempty"`
	Operation      string           `json:"operation,omitempty"`
	Driver         string           `json:"driver,omitempty"`
	ResourceID     string           `json:"resourceId,omitempty"`
	LivePath       string           `json:"livePath,omitempty"`
	DesiredPath    string           `json:"desiredPath,omitempty"`
	DryRun         bool             `json:"dryRun"`
	State          status.StateCode `json:"state"`
	Message        string           `json:"message,omitempty"`
	Actions        []status.Action  `json:"actions,omitempty"`
	Change         Change           `json:"change"`
	Backup         Backup           `json:"backup"`
	Result         Result           `json:"result"`
	Diagnostics    []Diagnostic     `json:"diagnostics,omitempty"`
	Warnings       []status.Warning `json:"warnings,omitempty"`
	NoBaseline     bool             `json:"noBaseline,omitempty"`
	SyncMode       status.SyncMode  `json:"syncMode,omitempty"`
	AutomaticMerge bool             `json:"automaticMerge"`
}

type Change struct {
	Kind    filedriver.ChangeKind `json:"kind"`
	Before  Snapshot              `json:"before"`
	After   Snapshot              `json:"after"`
	Entries []EntryChange         `json:"entries,omitempty"`
}

type Snapshot struct {
	Exists     bool   `json:"exists"`
	Size       int    `json:"size,omitempty"`
	SHA256     string `json:"sha256,omitempty"`
	EntryCount int    `json:"entryCount,omitempty"`
	FileCount  int    `json:"fileCount,omitempty"`
	DirCount   int    `json:"dirCount,omitempty"`
}

type EntryChange struct {
	Path   string                `json:"path"`
	Kind   filedriver.ChangeKind `json:"kind"`
	Before Snapshot              `json:"before"`
	After  Snapshot              `json:"after"`
}

type Backup struct {
	Policy  BackupPolicy `json:"policy"`
	Ref     string       `json:"ref,omitempty"`
	Message string       `json:"message"`
}

type Diagnostic struct {
	Code     string             `json:"code"`
	Severity DiagnosticSeverity `json:"severity"`
	Message  string             `json:"message"`
	Ref      string             `json:"ref,omitempty"`
	Path     string             `json:"path,omitempty"`
	ExitCode int                `json:"exitCode,omitempty"`
}

type EnvelopeOptions struct {
	Command      Command
	RunID        string
	ProfileStack []string
	LedgerRef    string
	Items        []Item
}

func BuildEnvelope(opts EnvelopeOptions) Envelope {
	envelope := Envelope{
		Schema:        Schema,
		SchemaVersion: SchemaVersion,
		Command:       opts.Command,
		RunID:         strings.TrimSpace(opts.RunID),
		ProfileStack:  append([]string(nil), opts.ProfileStack...),
		Items:         append([]Item(nil), opts.Items...),
		LedgerRef:     strings.TrimSpace(opts.LedgerRef),
	}
	return NormalizeEnvelope(envelope)
}

func NormalizeEnvelope(envelope Envelope) Envelope {
	if envelope.Schema == "" {
		envelope.Schema = Schema
	}
	if envelope.SchemaVersion == 0 {
		envelope.SchemaVersion = SchemaVersion
	}
	envelope.ProfileStack = append([]string(nil), envelope.ProfileStack...)
	envelope.Items = append([]Item(nil), envelope.Items...)
	for i := range envelope.Items {
		envelope.Items[i] = NormalizeItem(envelope.Items[i])
	}
	sort.SliceStable(envelope.Items, func(i, j int) bool {
		return itemSortKey(envelope.Items[i]) < itemSortKey(envelope.Items[j])
	})
	envelope.Summary = summarize(envelope.Items)
	return envelope
}

func NormalizeItem(item Item) Item {
	item.TargetRef = strings.TrimSpace(item.TargetRef)
	item.SettingRef = strings.TrimSpace(item.SettingRef)
	item.Scope = strings.TrimSpace(item.Scope)
	item.Subject = strings.TrimSpace(item.Subject)
	item.DesiredURI = strings.TrimSpace(item.DesiredURI)
	item.DesiredRelPath = strings.TrimSpace(item.DesiredRelPath)
	item.Operation = strings.TrimSpace(item.Operation)
	item.Driver = strings.TrimSpace(item.Driver)
	item.ResourceID = strings.TrimSpace(item.ResourceID)
	item.LivePath = strings.TrimSpace(item.LivePath)
	item.DesiredPath = strings.TrimSpace(item.DesiredPath)
	item.Message = strings.TrimSpace(item.Message)
	if item.Result == "" {
		item.Result = resultFromStateAndChange(item.State, item.Change.Kind)
	}
	item.Actions = normalizeActions(item.Actions)
	item.Diagnostics = append([]Diagnostic(nil), item.Diagnostics...)
	sort.SliceStable(item.Diagnostics, func(i, j int) bool {
		return diagnosticSortKey(item.Diagnostics[i]) < diagnosticSortKey(item.Diagnostics[j])
	})
	item.Warnings = append([]status.Warning(nil), item.Warnings...)
	sort.SliceStable(item.Warnings, func(i, j int) bool {
		left := string(item.Warnings[i].Code) + "\x00" + item.Warnings[i].Message
		right := string(item.Warnings[j].Code) + "\x00" + item.Warnings[j].Message
		return left < right
	})
	item.Change.Entries = append([]EntryChange(nil), item.Change.Entries...)
	sort.SliceStable(item.Change.Entries, func(i, j int) bool {
		left := item.Change.Entries[i].Path + "\x00" + string(item.Change.Entries[i].Kind)
		right := item.Change.Entries[j].Path + "\x00" + string(item.Change.Entries[j].Kind)
		return left < right
	})
	if item.Backup.Policy == "" {
		item.Backup.Policy = BackupNotApplicable
	}
	if strings.TrimSpace(item.Backup.Message) == "" {
		item.Backup.Message = defaultBackupMessage(item)
	}
	if item.SyncMode == "" {
		item.SyncMode = status.SyncModeNone
	}
	return item
}

type CustomFilesPlanOptions struct {
	DryRun      bool
	LastApplied *status.NormalizedState
	BackupRef   string
	Diagnostics []Diagnostic
}

func FromCustomFilesPlan(plan *customfiles.Plan, opts CustomFilesPlanOptions) (Item, error) {
	if plan == nil {
		return Item{}, fmt.Errorf("custom.files plan is required")
	}

	item := Item{
		TargetRef:      plan.Setting.TargetID,
		SettingRef:     plan.Setting.Ref(),
		Scope:          plan.Setting.Scope,
		Subject:        plan.Setting.Subject,
		DesiredURI:     plan.Setting.DesiredURI,
		DesiredRelPath: plan.DesiredRelPath,
		Operation:      string(plan.Operation),
		Driver:         plan.Resource.Driver,
		ResourceID:     plan.ResourceID,
		DryRun:         opts.DryRun,
		Diagnostics:    append([]Diagnostic(nil), opts.Diagnostics...),
		AutomaticMerge: false,
	}

	var statusInput status.Input
	statusInput.Context = status.Context(plan.Operation)
	statusInput.TargetRef = item.TargetRef
	statusInput.SettingRef = item.SettingRef
	statusInput.LastApplied = opts.LastApplied

	switch plan.Resource.Driver {
	case recipe.FileTreeDriverID:
		live, err := filetreedriver.ResolveTarget(plan.TreeLiveTarget)
		if err != nil {
			return Item{}, fmt.Errorf("resolve live tree target: %w", err)
		}
		desired, err := filetreedriver.ResolveTarget(plan.TreeDesiredTarget)
		if err != nil {
			return Item{}, fmt.Errorf("resolve desired tree target: %w", err)
		}
		item.LivePath = live.AbsPath
		item.DesiredPath = desired.AbsPath
		item.Change = fromTreeDiff(plan.TreePreview.Change)
		if plan.Operation == customfiles.OperationSave {
			statusInput.Desired = status.FromFileTreeState(plan.TreeDestinationState)
			statusInput.Current = status.FromFileTreeState(plan.TreeSourceState)
		} else {
			statusInput.Desired = status.FromFileTreeState(plan.TreeSourceState)
			statusInput.Current = status.FromFileTreeState(plan.TreeDestinationState)
		}
	case recipe.FileDriverID:
		live, err := filedriver.ResolveTarget(plan.LiveTarget)
		if err != nil {
			return Item{}, fmt.Errorf("resolve live target: %w", err)
		}
		desired, err := filedriver.ResolveTarget(plan.DesiredTarget)
		if err != nil {
			return Item{}, fmt.Errorf("resolve desired target: %w", err)
		}
		item.LivePath = live.AbsPath
		item.DesiredPath = desired.AbsPath
		item.Change = fromFileDiff(plan.Preview.Change)
		if plan.Operation == customfiles.OperationSave {
			statusInput.Desired = status.FromFileState(plan.DestinationState)
			statusInput.Current = status.FromFileState(plan.SourceState)
		} else {
			statusInput.Desired = status.FromFileState(plan.SourceState)
			statusInput.Current = status.FromFileState(plan.DestinationState)
		}
	default:
		return Item{}, fmt.Errorf("unsupported custom.files driver: %s", plan.Resource.Driver)
	}

	statusItem := status.DeriveItem(statusInput)
	item.State = statusItem.State
	item.Message = statusItem.Message
	item.Actions = statusItem.Actions
	item.Warnings = statusItem.Warnings
	item.NoBaseline = statusItem.NoBaseline
	item.SyncMode = statusItem.SyncMode
	item.AutomaticMerge = statusItem.AutomaticMerge
	item.Result = resultFromStateAndChange(item.State, item.Change.Kind)
	item.Backup = backupForPlan(plan.Operation, opts.BackupRef, item.Result)
	return NormalizeItem(item), nil
}

func JSON(envelope Envelope) (string, error) {
	payload, err := json.MarshalIndent(NormalizeEnvelope(envelope), "", "  ")
	if err != nil {
		return "", err
	}
	return string(payload) + "\n", nil
}

func summarize(items []Item) Summary {
	var summary Summary
	var successfulOrReviewable int
	for _, item := range items {
		switch item.Result {
		case ResultBlocked:
			summary.Blocked++
		case ResultSkipped:
			summary.Skipped++
		case ResultSaved:
			summary.Saved++
			successfulOrReviewable++
		case ResultApplied:
			summary.Applied++
			successfulOrReviewable++
		case ResultFailed:
			summary.Failed++
		case ResultWouldChange:
			summary.Changed++
			successfulOrReviewable++
		case ResultUnchanged:
			successfulOrReviewable++
		}
		if item.Result != ResultBlocked && item.Result != ResultFailed {
			for _, diagnostic := range item.Diagnostics {
				if diagnostic.Severity == SeverityError && diagnostic.ExitCode != 0 {
					summary.Failed++
					break
				}
			}
		}
	}

	switch {
	case summary.Failed > 0 && successfulOrReviewable > 0:
		summary.Status = SummaryPartial
	case summary.Failed > 0:
		summary.Status = SummaryError
	case summary.Blocked > 0 && successfulOrReviewable > 0:
		summary.Status = SummaryPartial
	case summary.Blocked > 0:
		summary.Status = SummaryBlocked
	case summary.Changed > 0:
		summary.Status = SummaryChanged
	default:
		summary.Status = SummaryOK
	}
	return summary
}

func fromFileDiff(diff filedriver.Diff) Change {
	return Change{Kind: diff.Kind, Before: fromFileSnapshot(diff.Before), After: fromFileSnapshot(diff.After)}
}

func fromTreeDiff(diff filetreedriver.Diff) Change {
	entries := make([]EntryChange, 0, len(diff.Entries))
	for _, entry := range diff.Entries {
		entries = append(entries, EntryChange{
			Path:   entry.Path,
			Kind:   entry.Kind,
			Before: fromTreeEntrySnapshot(entry.Before),
			After:  fromTreeEntrySnapshot(entry.After),
		})
	}
	return Change{Kind: diff.Kind, Before: fromTreeSnapshot(diff.Before), After: fromTreeSnapshot(diff.After), Entries: entries}
}

func fromFileSnapshot(snapshot filedriver.Snapshot) Snapshot {
	return Snapshot{Exists: snapshot.Exists, Size: snapshot.Size, SHA256: snapshot.SHA256}
}

func fromTreeSnapshot(snapshot filetreedriver.Snapshot) Snapshot {
	return Snapshot{Exists: snapshot.Exists, EntryCount: snapshot.EntryCount, FileCount: snapshot.FileCount, DirCount: snapshot.DirCount, SHA256: snapshot.SHA256}
}

func fromTreeEntrySnapshot(snapshot filetreedriver.EntrySnapshot) Snapshot {
	return Snapshot{Exists: snapshot.Exists, Size: snapshot.Size, SHA256: snapshot.SHA256}
}

func backupForPlan(op customfiles.Operation, backupRef string, result Result) Backup {
	if result == ResultBlocked || result == ResultFailed {
		return Backup{Policy: BackupSkippedForBlocker, Message: "Backup is not planned because this item is blocked or failed before writing."}
	}
	if op == customfiles.OperationApply {
		if strings.TrimSpace(backupRef) != "" {
			return Backup{Policy: BackupRefSupplied, Ref: strings.TrimSpace(backupRef), Message: "Backup reference supplied by caller for the planned live write."}
		}
		return Backup{Policy: BackupRequired, Message: "Backup would be required before live write; backup ledger is not available in this preview."}
	}
	return Backup{Policy: BackupNotApplicable, Message: "No live backup is required for save because only desired artifacts would be written."}
}

func defaultBackupMessage(item Item) string {
	if item.Result == ResultBlocked || item.Result == ResultFailed {
		return "Backup is not planned because this item is blocked or failed before writing."
	}
	if item.Backup.Policy == BackupRequired {
		return "Backup would be required before live write; backup ledger is not available in this preview."
	}
	if item.Backup.Policy == BackupUnsupported {
		return "Backup is unsupported for this item; policy must allow proceeding before a live write."
	}
	if item.Operation == string(customfiles.OperationApply) {
		return "Backup would be required before live write; backup ledger is not available in this preview."
	}
	return "No live backup is required for this preview."
}

func resultFromStateAndChange(state status.StateCode, kind filedriver.ChangeKind) Result {
	switch state {
	case status.StateBlockedSafety, status.StateBlockedLifecycle, status.StateUnsupported:
		return ResultBlocked
	}
	if kind == filedriver.ChangeUnchanged {
		return ResultUnchanged
	}
	return ResultWouldChange
}

func itemSortKey(item Item) string {
	return item.TargetRef + "\x00" + item.SettingRef + "\x00" + item.Operation + "\x00" + item.LivePath + "\x00" + item.DesiredPath
}

func diagnosticSortKey(diagnostic Diagnostic) string {
	return fmt.Sprintf("%03d\x00%s\x00%s\x00%s\x00%s", diagnostic.ExitCode, diagnostic.Code, diagnostic.Ref, diagnostic.Path, diagnostic.Message)
}

func normalizeActions(actions []status.Action) []status.Action {
	if len(actions) == 0 {
		return nil
	}
	seen := map[status.Action]bool{}
	normalized := make([]status.Action, 0, len(actions))
	for _, action := range actions {
		if action == "" || seen[action] {
			continue
		}
		seen[action] = true
		normalized = append(normalized, action)
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		leftRank, rightRank := actionRank(normalized[i]), actionRank(normalized[j])
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		return normalized[i] < normalized[j]
	})
	return normalized
}

func actionRank(action status.Action) int {
	switch action {
	case status.ActionGuidedSync:
		return 10
	case status.ActionDiff:
		return 20
	case status.ActionSave:
		return 30
	case status.ActionApply:
		return 40
	case status.ActionCreate:
		return 50
	case status.ActionQuit:
		return 60
	case status.ActionRetry:
		return 70
	case status.ActionSkip:
		return 80
	case status.ActionInspect:
		return 90
	case status.ActionFix:
		return 100
	case status.ActionCreateRecipe:
		return 110
	case status.ActionVerbose:
		return 120
	default:
		return 1000
	}
}
