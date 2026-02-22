package app

import (
	"fmt"
	"sort"
	"strings"
)

func buildTextOutput(command string, dryRun bool, result map[string]any) string {
	if result == nil {
		return buildTextSummaryLine(command, dryRun, nil)
	}

	lines := make([]string, 0)
	syncs := syncPayloadMaps(result["syncs"])

	if command == "status" && len(syncs) > 0 {
		lines = append(lines, "reminder: deploy applies source -> target; import applies target -> source")
	}
	if command == "diff" && len(syncs) > 0 {
		lines = append(lines, diffLegendLines()...)
	}

	for idx, sync := range syncs {
		if idx > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, buildSyncHeader(sync))

		switch command {
		case "status":
			deployOps := operationPayloadMapsByPhase(sync, "deploy")
			importOps := operationPayloadMapsByPhase(sync, "import")
			lines = appendPhaseBlockWithContext(lines, "deploy", "(source -> target)", deployOps)
			lines = appendPhaseBlockWithContext(lines, "import", "(target -> source)", importOps)
			lines = appendStatusDirectionHint(lines, deployOps, importOps)
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
		case "diff":
			lines = appendDiffPhaseBlock(lines, "deploy-diff", "(target -> source)", operationPayloadMapsByPhase(sync, "deploy"))
			lines = appendDiffPhaseBlock(lines, "import-diff", "(source -> target)", operationPayloadMapsByPhase(sync, "import"))
			lines = appendDiffPhaseBlock(lines, "incoming-unmanaged", "(target -> source)", operationPayloadMapsByPhase(sync, "incoming_unmanaged"))
			lines = appendDiffPhaseBlock(lines, "remove-unmanaged", "(target -> /dev/null)", operationPayloadMapsByPhase(sync, "remove_unmanaged"))
			lines = appendDiffPhaseBlock(lines, "remove-missing", "(source -> /dev/null)", operationPayloadMapsByPhase(sync, "remove_missing"))
		}
	}

	lines = append(lines, buildTextSummaryLine(command, dryRun, result["summary"]))

	return strings.Join(lines, "\n")
}

func diffLegendLines() []string {
	return []string{
		"legend intent: deploy applies source -> target; import applies target -> source",
		"legend patch-orientation: deploy-diff compares target -> source; import-diff compares source -> target",
	}
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
	return appendPhaseBlockWithContext(lines, label, "", operations)
}

func appendPhaseBlockWithContext(lines []string, label string, context string, operations []map[string]any) []string {
	if len(operations) == 0 {
		return lines
	}

	header := fmt.Sprintf("%s[%d]", label, len(operations))
	if context != "" {
		header = fmt.Sprintf("%s %s", header, context)
	}
	lines = append(lines, header)
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

func appendDiffPhaseBlock(lines []string, label string, context string, operations []map[string]any) []string {
	if len(operations) == 0 {
		return lines
	}

	header := fmt.Sprintf("%s[%d]", label, len(operations))
	if context != "" {
		header = fmt.Sprintf("%s %s", header, context)
	}
	lines = append(lines, header)

	for idx, op := range operations {
		path := stringValue(op["path"])
		if path == "" {
			path = "<unknown>"
		}

		details := operationDetails(op)
		if details != "" {
			lines = append(lines, fmt.Sprintf("  path: %s (%s)", path, details))
		} else {
			lines = append(lines, fmt.Sprintf("  path: %s", path))
		}

		patch := stringValue(op["patch"])
		if patch != "" {
			patchLines := strings.Split(strings.TrimSuffix(patch, "\n"), "\n")
			lines = append(lines, patchLines...)
		} else {
			note := diffNote(op)
			if note == "" {
				note = "patch unavailable"
			}
			lines = append(lines, "  note: "+note)
		}

		if idx < len(operations)-1 {
			lines = append(lines, "")
		}
	}

	return lines
}

func appendStatusDirectionHint(lines []string, deployOps []map[string]any, importOps []map[string]any) []string {
	overlapPaths := overlappingOperationPaths(deployOps, importOps)
	if len(overlapPaths) == 0 {
		return lines
	}

	const maxHintPaths = 3
	preview := overlapPaths
	extra := 0
	if len(preview) > maxHintPaths {
		extra = len(preview) - maxHintPaths
		preview = preview[:maxHintPaths]
	}

	hint := fmt.Sprintf("hint: same path in deploy/import: %s", strings.Join(preview, ", "))
	if extra > 0 {
		hint = fmt.Sprintf("%s (+%d more)", hint, extra)
	}
	return append(lines, hint)
}

func overlappingOperationPaths(leftOps []map[string]any, rightOps []map[string]any) []string {
	if len(leftOps) == 0 || len(rightOps) == 0 {
		return nil
	}

	leftPaths := make(map[string]struct{}, len(leftOps))
	for _, op := range leftOps {
		path := stringValue(op["path"])
		if path != "" {
			leftPaths[path] = struct{}{}
		}
	}
	if len(leftPaths) == 0 {
		return nil
	}

	overlapSet := make(map[string]struct{})
	for _, op := range rightOps {
		path := stringValue(op["path"])
		if path == "" {
			continue
		}
		if _, ok := leftPaths[path]; ok {
			overlapSet[path] = struct{}{}
		}
	}
	if len(overlapSet) == 0 {
		return nil
	}

	overlapPaths := make([]string, 0, len(overlapSet))
	for path := range overlapSet {
		overlapPaths = append(overlapPaths, path)
	}
	sort.Strings(overlapPaths)
	return overlapPaths
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

func diffNote(op map[string]any) string {
	reason := stringValue(op["reason"])
	kind := stringValue(op["diff_kind"])
	if reason != "" {
		return reason
	}
	switch kind {
	case "binary":
		return "binary differs"
	case "type_change":
		return "type differs"
	case "omitted":
		return "patch omitted"
	default:
		return ""
	}
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
