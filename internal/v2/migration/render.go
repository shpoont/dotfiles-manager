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
	if plan.DryRun {
		lines = append(lines, "MODE: DRY RUN (no writes)")
	} else {
		lines = append(lines, "MODE: MIGRATE (writes generated output only)")
	}
	lines = append(lines, fmt.Sprintf("migration run=%s config=%s", plan.RunID, plan.ConfigPath))
	if !plan.DryRun && plan.OutputDir != "" {
		lines = append(lines, "output: "+plan.OutputDir)
	}
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

func ParityText(report *ParityReport) string {
	if report == nil {
		return "migration parity report\nsummary syncs=0 ok=0 blocked=0 files=0 file-trees=0 status=error"
	}
	var lines []string
	lines = append(lines, "migration parity report")
	lines = append(lines, "run: "+report.RunID)
	lines = append(lines, "run dir: "+report.MigrationRunDir)
	lines = append(lines, "generated root: "+report.GeneratedRoot)
	if report.ConfigPath != "" {
		lines = append(lines, "config: "+report.ConfigPath)
	}
	if report.Error != nil && report.Summary.Status == "error" {
		lines = append(lines, fmt.Sprintf("error[%s]: %s", report.Error.Code, report.Error.Message))
	}
	for _, item := range report.Items {
		lines = append(lines, "")
		lines = append(lines, item.SyncRef)
		lines = append(lines, fmt.Sprintf("  proposed: %s driver=%s", item.SettingRef, item.Driver))
		lines = append(lines, "  legacy source: "+item.ExpandedSourcePath)
		lines = append(lines, "  legacy target: "+item.ExpandedTargetPath)
		if item.LiveTargetPath != "" {
			lines = append(lines, "  live target: "+item.LiveTargetPath)
		}
		if item.DesiredArtifactPath != "" {
			lines = append(lines, "  desired artifact: "+item.DesiredArtifactPath)
		}
		lines = append(lines, "  result: "+item.Result)
		for _, diagnostic := range item.Diagnostics {
			lines = append(lines, fmt.Sprintf("  diagnostic[%s]: %s", diagnostic.Code, diagnostic.Message))
		}
	}
	if report.Error != nil && report.Summary.Status != "error" {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("error[%s]: %s", report.Error.Code, report.Error.Message))
	}
	lines = append(lines, fmt.Sprintf(
		"summary syncs=%d ok=%d blocked=%d files=%d file-trees=%d status=%s",
		report.Summary.Syncs,
		report.Summary.OK,
		report.Summary.Blocked,
		report.Summary.Files,
		report.Summary.FileTrees,
		report.Summary.Status,
	))
	return strings.Join(lines, "\n")
}
