package app

import (
	"fmt"
	"strings"
)

func buildTextOutput(command string, dryRun bool, result map[string]any) string {
	if result == nil {
		return buildTextSummaryLine(command, dryRun, nil)
	}

	lines := make([]string, 0)
	syncs := syncPayloadMaps(result["syncs"])

	for idx, sync := range syncs {
		if idx > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, buildSyncHeader(sync))

		switch command {
		case "status":
			lines = appendPhaseBlock(lines, "deploy", operationPayloadMapsByPhase(sync, "deploy"))
			lines = appendPhaseBlock(lines, "import", operationPayloadMapsByPhase(sync, "import"))
			lines = appendPhaseBlock(lines, "incoming-unmanaged", operationPayloadMapsByPhase(sync, "incoming_unmanaged"))
			lines = appendPhaseBlock(lines, "remove-unmanaged", operationPayloadMapsByPhase(sync, "remove_unmanaged"))
			lines = appendPhaseBlock(lines, "remove-missing", operationPayloadMapsByPhase(sync, "remove_missing"))
		case "deploy":
			lines = appendPhaseBlock(lines, "copy", operationPayloadMapsByPhase(sync, "copy"))
			lines = appendPhaseBlock(lines, "remove-unmanaged", operationPayloadMapsByPhase(sync, "remove_unmanaged"))
		case "import":
			lines = appendPhaseBlock(lines, "update-managed", operationPayloadMapsByPhase(sync, "update_managed"))
			lines = appendPhaseBlock(lines, "add-unmanaged", operationPayloadMapsByPhase(sync, "add_unmanaged"))
			lines = appendPhaseBlock(lines, "remove-missing", operationPayloadMapsByPhase(sync, "remove_missing"))
		}
	}

	lines = append(lines, buildTextSummaryLine(command, dryRun, result["summary"]))

	return strings.Join(lines, "\n")
}

func buildSyncHeader(sync map[string]any) string {
	label := stringValue(sync["sync"])
	if label == "" {
		label = fmt.Sprintf("sync[%d]", summaryInt(sync, "sync_index"))
	}
	scopePrefix := stringValue(sync["scope_prefix"])
	if scopePrefix == "" {
		return label
	}
	return fmt.Sprintf("%s scope=%s", label, scopePrefix)
}

func appendPhaseBlock(lines []string, label string, operations []map[string]any) []string {
	lines = append(lines, fmt.Sprintf("%s[%d]", label, len(operations)))
	for _, op := range operations {
		action := stringValue(op["action"])
		path := stringValue(op["path"])
		if action == "" {
			action = "unknown"
		}
		if path == "" {
			path = "<unknown>"
		}
		line := fmt.Sprintf("  %-12s %s", action, path)
		if details := operationDetails(op); details != "" {
			line += " (" + details + ")"
		}
		lines = append(lines, line)
	}
	return lines
}

func operationDetails(op map[string]any) string {
	sourceType := stringValue(op["source_type"])
	targetType := stringValue(op["target_type"])
	if sourceType != "" || targetType != "" {
		return fmt.Sprintf("%s->%s", sourceType, targetType)
	}
	entryType := stringValue(op["type"])
	return entryType
}

func syncPayloadMaps(raw any) []map[string]any {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if ok {
			out = append(out, entry)
		}
	}
	return out
}

func operationPayloadMapsByPhase(sync map[string]any, phase string) []map[string]any {
	items, ok := sync["operations"].([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if stringValue(entry["phase"]) != phase {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func stringValue(v any) string {
	s, _ := v.(string)
	return s
}
