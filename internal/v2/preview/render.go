package preview

import (
	"fmt"
	"strings"

	"github.com/shpoont/dotfiles-manager/internal/v2/filedriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/status"
)

func RenderText(envelope Envelope) string {
	envelope = NormalizeEnvelope(envelope)
	var b strings.Builder
	fmt.Fprintf(&b, "dotfiles-manager v2 %s preview\n", envelope.Command)
	if envelope.RunID != "" {
		fmt.Fprintf(&b, "Run: %s\n", envelope.RunID)
	}
	if len(envelope.ProfileStack) > 0 {
		fmt.Fprintf(&b, "Profiles: %s\n", strings.Join(envelope.ProfileStack, " > "))
	}
	fmt.Fprintf(&b, "Summary: %s (changed=%d blocked=%d saved=%d applied=%d skipped=%d failed=%d)\n",
		envelope.Summary.Status,
		envelope.Summary.Changed,
		envelope.Summary.Blocked,
		envelope.Summary.Saved,
		envelope.Summary.Applied,
		envelope.Summary.Skipped,
		envelope.Summary.Failed,
	)
	if envelope.LedgerRef != "" {
		fmt.Fprintf(&b, "Ledger: %s\n", envelope.LedgerRef)
	}
	b.WriteString("\n")

	if len(envelope.Items) == 0 {
		b.WriteString("No managed items matched this preview.\n\n")
	}
	for _, item := range envelope.Items {
		renderItem(&b, item)
	}
	b.WriteString("Use --json for technical details.\n")
	return b.String()
}

func renderItem(b *strings.Builder, item Item) {
	label := item.SettingRef
	if label == "" {
		label = item.TargetRef
	}
	if label == "" {
		label = "<unknown>"
	}
	fmt.Fprintf(b, "%s\n", label)
	fmt.Fprintf(b, "  State: %s\n", item.State)
	fmt.Fprintf(b, "  Result: %s\n", item.Result)
	if item.Message != "" {
		fmt.Fprintf(b, "  Detail: %s\n", item.Message)
	}
	fmt.Fprintf(b, "  Change: %s %s\n", changeVerb(item.Change.Kind), changeTarget(item))
	if item.LivePath != "" {
		fmt.Fprintf(b, "  Live: %s\n", item.LivePath)
	}
	if item.DesiredRelPath != "" {
		fmt.Fprintf(b, "  Desired artifact: %s\n", item.DesiredRelPath)
	}
	if item.DesiredPath != "" {
		fmt.Fprintf(b, "  Desired path: %s\n", item.DesiredPath)
	}
	if item.DesiredURI != "" {
		fmt.Fprintf(b, "  Desired URI: %s\n", item.DesiredURI)
	}
	if len(item.Actions) > 0 {
		fmt.Fprintf(b, "  Next: %s\n", joinActions(item.Actions))
	}
	fmt.Fprintf(b, "  Backup: %s\n", item.Backup.Message)
	for _, diagnostic := range item.Diagnostics {
		fmt.Fprintf(b, "  Diagnostic[%s]: %s", diagnostic.Severity, diagnostic.Code)
		if diagnostic.Message != "" {
			fmt.Fprintf(b, " - %s", diagnostic.Message)
		}
		if diagnostic.Ref != "" {
			fmt.Fprintf(b, " (%s)", diagnostic.Ref)
		}
		b.WriteString("\n")
	}
	for _, warning := range item.Warnings {
		fmt.Fprintf(b, "  Warning[%s]: %s\n", warning.Code, warning.Message)
	}
	b.WriteString("\n")
}

func changeVerb(kind filedriver.ChangeKind) string {
	switch kind {
	case filedriver.ChangeCreate:
		return "create"
	case filedriver.ChangeUpdate:
		return "update"
	case filedriver.ChangeDelete:
		return "delete"
	case filedriver.ChangeUnchanged:
		return "leave unchanged"
	default:
		if kind == "" {
			return "inspect"
		}
		return string(kind)
	}
}

func changeTarget(item Item) string {
	if item.Result == ResultBlocked || item.Result == ResultSkipped || item.Result == ResultFailed {
		return "skipped item"
	}
	switch item.Operation {
	case "save":
		return "desired artifact"
	case "apply":
		return "live target"
	default:
		return "managed item"
	}
}

func joinActions(actions []status.Action) string {
	parts := make([]string, 0, len(actions))
	for _, action := range actions {
		parts = append(parts, string(action))
	}
	return strings.Join(parts, ", ")
}
