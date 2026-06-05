package preview

import "github.com/shpoont/dotfiles-manager/internal/v2/status"

const (
	ExitSuccess       = 0
	ExitInternalError = 1
	ExitValidation    = 2
	ExitChanged       = 3
	ExitInputRequired = 4
	ExitSafetyBlocker = 5
	ExitPartial       = 6
)

func ExitCode(envelope Envelope) int {
	envelope = NormalizeEnvelope(envelope)
	if envelope.Summary.Status == SummaryPartial {
		return ExitPartial
	}

	code := diagnosticExitCode(envelope)
	if code != 0 {
		return code
	}

	if envelope.Summary.Status == SummaryError {
		return ExitInternalError
	}
	if envelope.Summary.Status == SummaryBlocked || hasBlockedItem(envelope.Items) {
		return ExitSafetyBlocker
	}
	if envelope.Summary.Status == SummaryChanged || envelope.Summary.Changed > 0 || hasCheckChange(envelope.Items) {
		return ExitChanged
	}
	return ExitSuccess
}

func diagnosticExitCode(envelope Envelope) int {
	for _, want := range []int{ExitInternalError, ExitValidation, ExitInputRequired, ExitSafetyBlocker} {
		for _, item := range envelope.Items {
			for _, diagnostic := range item.Diagnostics {
				if diagnostic.ExitCode == want {
					return want
				}
			}
		}
	}
	return 0
}

func hasBlockedItem(items []Item) bool {
	for _, item := range items {
		if item.Result == ResultBlocked {
			return true
		}
		switch item.State {
		case status.StateBlockedSafety, status.StateBlockedLifecycle, status.StateUnsupported:
			return true
		}
	}
	return false
}

func hasCheckChange(items []Item) bool {
	for _, item := range items {
		if item.Result == ResultWouldChange {
			return true
		}
		switch item.State {
		case status.StateChangedCurrent, status.StateReadyToApply, status.StateConflict, status.StateOpaqueChanged, status.StateMissingCurrent, status.StateMissingDesired:
			return true
		}
	}
	return false
}
