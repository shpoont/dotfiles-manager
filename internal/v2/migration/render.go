package migration

import (
	"fmt"
	"strings"
)

func Text(plan *Plan) string {
	if plan == nil {
		return "summary syncs=0 planned=0 blocked=0 generated-files=0 status=error"
	}
	var lines []string
	lines = append(lines, "MODE: DRY RUN (no writes)")
	lines = append(lines, fmt.Sprintf("migration run=%s config=%s", plan.RunID, plan.ConfigPath))
	lines = append(lines, "v1 config action: leave unchanged")
	lines = append(lines, "v1 command behavior: unchanged")
	for _, item := range plan.Items {
		lines = append(lines, "")
		lines = append(lines, item.SyncRef)
		lines = append(lines, "  legacy source: "+item.LegacySource)
		lines = append(lines, "  legacy target: "+item.LegacyTarget)
		lines = append(lines, "  expanded source: "+item.ExpandedSourcePath)
		lines = append(lines, "  expanded target: "+item.ExpandedTargetPath)
		lines = append(lines, fmt.Sprintf("  proposed: %s driver=%s", item.SettingRef, item.Driver))
		lines = append(lines, "  artifact binding: "+item.DesiredArtifactBinding.URI)
		lines = append(lines, "  v1 config: "+item.V1ConfigAction)
		if item.Result == "blocked" {
			lines = append(lines, "  result: blocked")
			for _, diagnostic := range item.Diagnostics {
				lines = append(lines, fmt.Sprintf("  diagnostic[%s]: %s", diagnostic.Code, diagnostic.Message))
			}
			continue
		}
		lines = append(lines, "  result: planned")
		lines = append(lines, "  generated files:")
		for _, file := range item.GeneratedFiles {
			lines = append(lines, "    "+file.Path)
		}
	}
	if len(plan.GeneratedFiles) > 0 {
		lines = append(lines, "")
		lines = append(lines, "shared generated files:")
		for _, file := range plan.GeneratedFiles {
			lines = append(lines, "  "+file.Path)
		}
	}
	lines = append(lines, fmt.Sprintf(
		"summary syncs=%d planned=%d blocked=%d files=%d file-trees=%d generated-files=%d status=%s",
		plan.Summary.Syncs,
		plan.Summary.Planned,
		plan.Summary.Blocked,
		plan.Summary.Files,
		plan.Summary.FileTrees,
		plan.Summary.GeneratedFiles,
		plan.Summary.Status,
	))
	return strings.Join(lines, "\n")
}
