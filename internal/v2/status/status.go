// Package status implements the v2 canonical status derivation rules.
//
// The package is intentionally internal and derivation-only. It does not read
// live targets, persist ledgers, render the final CLI envelope, or implement
// guided sync. Callers provide desired, current, and optional last-applied
// normalized state summaries that were produced elsewhere.
package status

import (
	"strings"

	"github.com/shpoont/dotfiles-manager/internal/v2/filedriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/filetreedriver"
)

type Context string

const (
	ContextStatus Context = "status"
	ContextSave   Context = "save"
	ContextApply  Context = "apply"
)

type StateCode string

const (
	StateUnchanged        StateCode = "unchanged"
	StateChangedCurrent   StateCode = "changed-current"
	StateReadyToApply     StateCode = "ready-to-apply"
	StateMissingDesired   StateCode = "missing-desired"
	StateMissingCurrent   StateCode = "missing-current"
	StateConflict         StateCode = "conflict"
	StateOpaqueChanged    StateCode = "opaque-changed"
	StateBlockedLifecycle StateCode = "blocked-lifecycle"
	StateBlockedSafety    StateCode = "blocked-safety"
	StateUnsupported      StateCode = "unsupported"
	StateUnknown          StateCode = "unknown"
)

type Action string

const (
	ActionSave         Action = "save"
	ActionApply        Action = "apply"
	ActionDiff         Action = "diff"
	ActionCreate       Action = "create-artifact"
	ActionSkip         Action = "skip"
	ActionGuidedSync   Action = "guided-sync"
	ActionInspect      Action = "inspect"
	ActionFix          Action = "fix"
	ActionRetry        Action = "retry"
	ActionQuit         Action = "quit"
	ActionCreateRecipe Action = "create-recipe"
	ActionVerbose      Action = "verbose"
)

type WarningCode string

const WarningNeedsRecheck WarningCode = "needs-recheck"

type BlockerCode string

const (
	BlockerNone        BlockerCode = ""
	BlockerSafety      BlockerCode = "blocked-safety"
	BlockerLifecycle   BlockerCode = "blocked-lifecycle"
	BlockerUnsupported BlockerCode = "unsupported"
	BlockerUnknown     BlockerCode = "unknown"
)

type SyncMode string

const (
	SyncModeNone         SyncMode = "none"
	SyncModeGuidedChoice SyncMode = "guided-choice"
)

type NormalizedState struct {
	Exists        bool
	Hash          string
	Normalizer    string
	DriverVersion string
	RecipeVersion string
}

type Blocker struct {
	Code    BlockerCode
	Message string
}

type Input struct {
	Context     Context
	TargetRef   string
	SettingRef  string
	Desired     NormalizedState
	Current     NormalizedState
	LastApplied *NormalizedState
	Blocker     Blocker
}

type Warning struct {
	Code    WarningCode
	Message string
}

type Item struct {
	Context        Context
	TargetRef      string
	SettingRef     string
	State          StateCode
	Actions        []Action
	Message        string
	NoBaseline     bool
	Warnings       []Warning
	SyncMode       SyncMode
	AutomaticMerge bool
}

// FromFileState adapts the file driver state fields used for status derivation.
// It intentionally carries only existence, content hash, and normalizer;
// callers that need needs-recheck warnings must populate driver and recipe
// versions from ledger or recipe metadata.
func FromFileState(state filedriver.State) NormalizedState {
	return NormalizedState{
		Exists:     state.Exists,
		Hash:       state.SHA256,
		Normalizer: state.Normalizer,
	}
}

// FromFileTreeState adapts the file-tree driver state fields used for status
// derivation. It intentionally carries only existence, content hash, and
// normalizer; callers that need needs-recheck warnings must populate driver and
// recipe versions from ledger or recipe metadata.
func FromFileTreeState(state filetreedriver.State) NormalizedState {
	return NormalizedState{
		Exists:     state.Exists,
		Hash:       state.SHA256,
		Normalizer: state.Normalizer,
	}
}

func DeriveItem(input Input) Item {
	ctx := normalizedContext(input.Context)
	item := Item{
		Context:        ctx,
		TargetRef:      input.TargetRef,
		SettingRef:     input.SettingRef,
		SyncMode:       SyncModeNone,
		AutomaticMerge: false,
		Warnings:       versionWarnings(input),
	}

	switch input.Blocker.Code {
	case BlockerSafety:
		return finish(item, StateBlockedSafety, input.Blocker.Message)
	case BlockerLifecycle:
		return finish(item, StateBlockedLifecycle, input.Blocker.Message)
	case BlockerUnsupported:
		return finish(item, StateUnsupported, input.Blocker.Message)
	case BlockerUnknown:
		return finish(item, StateUnknown, input.Blocker.Message)
	}

	if !input.Desired.Exists {
		return finish(item, StateMissingDesired, "Setting is selected but no desired artifact exists.")
	}
	if !input.Current.Exists {
		return finish(item, StateMissingCurrent, "Desired state exists but current live state is missing.")
	}
	if !comparable(input.Desired) || !comparable(input.Current) {
		return finish(item, StateUnknown, "State cannot be determined safely from incomplete normalized hashes.")
	}
	if sameNormalizedContent(input.Current, input.Desired) {
		return finish(item, StateUnchanged, "Current state matches desired state.")
	}

	if input.LastApplied == nil {
		item.NoBaseline = true
		switch ctx {
		case ContextSave:
			return finish(item, StateChangedCurrent, "Current differs from desired and there is no previous sync baseline; saving will replace the desired artifact.")
		case ContextApply:
			return finish(item, StateReadyToApply, "Desired differs from current and there is no previous sync baseline; applying will replace live state.")
		default:
			// Command-neutral status deliberately uses unknown as a
			// direction-neutral no-baseline sentinel. The explicit
			// NoBaseline flag, message, and actions tell the user that
			// save and apply are both possible after reviewing the diff.
			return finish(item, StateUnknown, "Changed, no previous sync baseline: review diff, then choose save or apply.")
		}
	}

	if !comparable(*input.LastApplied) {
		return finish(item, StateUnknown, "State cannot be determined safely from incomplete last-applied baseline data.")
	}
	lastMatchesDesired := sameNormalizedContent(*input.LastApplied, input.Desired)
	lastMatchesCurrent := sameNormalizedContent(*input.LastApplied, input.Current)

	switch {
	case lastMatchesDesired:
		return finish(item, StateChangedCurrent, "Current differs from desired; last-applied baseline matches desired.")
	case lastMatchesCurrent:
		return finish(item, StateReadyToApply, "Desired differs from current; last-applied baseline matches current.")
	default:
		item.SyncMode = SyncModeGuidedChoice
		return finish(item, StateConflict, "Desired and current both changed since the last successful baseline; use guided sync or inspect the diff.")
	}
}

func AggregateTarget(items []Item) StateCode {
	if len(items) == 0 {
		return StateUnchanged
	}
	winner := items[0].State
	for _, item := range items[1:] {
		if moreSevere(item.State, winner) {
			winner = item.State
		}
	}
	return winner
}

func SeverityOrder() []StateCode {
	return []StateCode{
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
	}
}

func normalizedContext(ctx Context) Context {
	switch ctx {
	case ContextSave, ContextApply, ContextStatus:
		return ctx
	case "":
		return ContextStatus
	default:
		return ContextStatus
	}
}

func finish(item Item, state StateCode, message string) Item {
	item.State = state
	if strings.TrimSpace(message) == "" {
		message = defaultMessage(state)
	}
	item.Message = message
	item.Actions = actionsFor(state, item.Context, item.NoBaseline)
	return item
}

func actionsFor(state StateCode, ctx Context, noBaseline bool) []Action {
	if noBaseline && ctx == ContextStatus {
		return []Action{ActionDiff, ActionSave, ActionApply}
	}
	switch state {
	case StateUnchanged:
		return nil
	case StateChangedCurrent:
		return []Action{ActionSave, ActionApply}
	case StateReadyToApply:
		return []Action{ActionApply}
	case StateMissingDesired:
		return []Action{ActionSave, ActionCreate}
	case StateMissingCurrent:
		return []Action{ActionApply, ActionSkip}
	case StateConflict:
		return []Action{ActionGuidedSync, ActionDiff}
	case StateOpaqueChanged:
		return []Action{ActionSave, ActionApply}
	case StateBlockedLifecycle:
		return []Action{ActionQuit, ActionRetry, ActionSkip}
	case StateBlockedSafety:
		return []Action{ActionInspect, ActionFix}
	case StateUnsupported:
		return []Action{ActionSkip, ActionCreateRecipe}
	case StateUnknown:
		return []Action{ActionInspect, ActionVerbose}
	default:
		return []Action{ActionInspect, ActionVerbose}
	}
}

func defaultMessage(state StateCode) string {
	switch state {
	case StateBlockedSafety:
		return "Safety, trust, selector, secret, or recipe policy blocks status derivation."
	case StateBlockedLifecycle:
		return "Target lifecycle policy blocks status derivation."
	case StateUnsupported:
		return "No trusted recipe or capability supports this operation."
	case StateUnknown:
		return "State cannot be determined safely."
	default:
		return string(state)
	}
}

func comparable(state NormalizedState) bool {
	return !state.Exists || strings.TrimSpace(state.Hash) != ""
}

func sameNormalizedContent(a NormalizedState, b NormalizedState) bool {
	if a.Exists != b.Exists {
		return false
	}
	if !a.Exists {
		return true
	}
	return a.Hash == b.Hash
}

func moreSevere(candidate StateCode, current StateCode) bool {
	return severityRank(candidate) < severityRank(current)
}

func severityRank(state StateCode) int {
	for rank, candidate := range SeverityOrder() {
		if candidate == state {
			return rank
		}
	}
	return len(SeverityOrder())
}

func versionWarnings(input Input) []Warning {
	if input.LastApplied == nil {
		return nil
	}
	baseline := *input.LastApplied
	if changedSinceBaseline(baseline.Normalizer, input.Desired.Normalizer, input.Current.Normalizer) ||
		changedSinceBaseline(baseline.DriverVersion, input.Desired.DriverVersion, input.Current.DriverVersion) ||
		changedSinceBaseline(baseline.RecipeVersion, input.Desired.RecipeVersion, input.Current.RecipeVersion) {
		return []Warning{{Code: WarningNeedsRecheck, Message: "Recipe, driver, or normalizer version changed since the last-applied baseline; recompute normalized state before writing."}}
	}
	return nil
}

func changedSinceBaseline(baseline string, candidates ...string) bool {
	baseline = strings.TrimSpace(baseline)
	if baseline == "" {
		return false
	}
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate != "" && candidate != baseline {
			return true
		}
	}
	return false
}
