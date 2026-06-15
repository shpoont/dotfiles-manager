package app

import (
	"fmt"
	"strings"

	v2ledger "github.com/shpoont/dotfiles-manager/internal/v2/ledger"
	v2preview "github.com/shpoont/dotfiles-manager/internal/v2/preview"
	v2recipe "github.com/shpoont/dotfiles-manager/internal/v2/recipe"
)

func completedRestoreEnvelope(run *v2ledger.RestoreRun) (v2preview.Envelope, error) {
	if run == nil || run.RunRecord == nil {
		return v2preview.Envelope{}, fmt.Errorf("confirmed restore output requires a committed restore run record")
	}
	record := v2ledger.NormalizeRunRecord(*run.RunRecord)
	envelope := v2preview.NormalizeEnvelope(run.Preview)
	recordByKey := make(map[string]v2ledger.ItemRecord, len(record.Items))
	for _, item := range record.Items {
		recordByKey[restoreRecordKey(item.TargetRef, item.SettingRef, item.Operation, item.ResourceID)] = item
	}

	var missingBackup []string
	for i := range envelope.Items {
		item := envelope.Items[i]
		recordItem, ok := recordByKey[restoreRecordKey(item.TargetRef, item.SettingRef, item.Operation, item.ResourceID)]
		if !ok {
			item.Result = v2preview.ResultFailed
			item.State = "blocked-safety"
			item.Message = "Restore completed but no committed item record was found for this output item."
			item.Diagnostics = append(item.Diagnostics, v2preview.Diagnostic{
				Code:     "restore.committed-record-missing",
				Severity: v2preview.SeverityError,
				Message:  item.Message,
				Path:     item.LivePath,
				ExitCode: v2preview.ExitInternalError,
			})
			envelope.Items[i] = v2preview.NormalizeItem(item)
			missingBackup = append(missingBackup, restoreItemLabel(item))
			continue
		}

		item.DryRun = false
		item.Actions = nil
		item.Diagnostics = nil
		item.State = "unchanged"
		item.Change.Before = normalizedStateSnapshot(recordItem.Before)
		item.Change.After = normalizedStateSnapshot(recordItem.Desired)
		changed := !normalizedLedgerStatesEqual(recordItem.Before, recordItem.Desired)
		sourceRef := firstNonEmpty(firstString(recordItem.SourceBackupRefs), run.SourceBackup.RunID)
		if sourceRef == "" {
			sourceRef = "source backup"
		}

		switch recordItem.Result {
		case v2ledger.ItemResultVerified:
			if changed {
				item.Result = v2preview.ResultApplied
				item.Message = fmt.Sprintf("Restored live state from backup %s.", sourceRef)
				if isSelectedValueRestoreDriver(recordItem.Driver) {
					item.Message = fmt.Sprintf("Restored the whole backing file for selected value %s from backup %s; this was not a semantic single-value rollback.", recordItem.SettingRef, sourceRef)
				}
				if backupRef := firstString(recordItem.BackupRefs); backupRef != "" {
					item.Backup = v2preview.Backup{
						Policy:  v2preview.BackupRefSupplied,
						Ref:     backupRef,
						Message: fmt.Sprintf("Backup-before-restore was created in recovery run %s before the live write.", restoreBackupBeforeRunID(run)),
					}
				} else {
					item.Result = v2preview.ResultFailed
					item.State = "blocked-safety"
					item.Backup = v2preview.Backup{
						Policy:  v2preview.BackupSkippedForBlocker,
						Message: "Backup-before-restore metadata is missing for this changed restore item.",
					}
					item.Diagnostics = append(item.Diagnostics, v2preview.Diagnostic{
						Code:     "restore.backup-before-restore.missing",
						Severity: v2preview.SeverityError,
						Message:  "Confirmed restore changed live state but did not expose a backup-before-restore recovery handle.",
						Path:     item.LivePath,
						ExitCode: v2preview.ExitInternalError,
					})
					missingBackup = append(missingBackup, restoreItemLabel(item))
				}
			} else {
				item.Result = v2preview.ResultUnchanged
				item.Message = fmt.Sprintf("Live state already matched backup %s; no live write was needed.", sourceRef)
				if isSelectedValueRestoreDriver(recordItem.Driver) {
					item.Message = fmt.Sprintf("Whole backing file for selected value %s already matched backup %s; no live write was needed.", recordItem.SettingRef, sourceRef)
				}
				item.Backup = v2preview.Backup{
					Policy:  v2preview.BackupNotApplicable,
					Message: "No backup-before-restore was created because no live write was needed.",
				}
			}
		default:
			item.Result = v2preview.ResultFailed
			item.State = "blocked-safety"
			item.Message = defaultBackupString(recordItem.Verification.Message, "Restore item did not verify successfully.")
			item.Backup = v2preview.Backup{
				Policy:  v2preview.BackupSkippedForBlocker,
				Message: "Backup-before-restore recovery is not reported for this failed restore item.",
			}
			item.Diagnostics = append(item.Diagnostics, v2preview.Diagnostic{
				Code:     "restore.item-not-verified",
				Severity: v2preview.SeverityError,
				Message:  item.Message,
				Path:     item.LivePath,
				ExitCode: v2preview.ExitInternalError,
			})
		}
		envelope.Items[i] = v2preview.NormalizeItem(item)
	}
	envelope.RunID = record.RunID
	envelope.ProfileStack = append([]string(nil), record.ProfileStack...)
	envelope = v2preview.NormalizeEnvelope(envelope)
	if len(missingBackup) > 0 {
		return envelope, fmt.Errorf("backup-before-restore recovery handle is missing for confirmed restore item(s): %s", strings.Join(missingBackup, ", "))
	}
	return envelope, nil
}

func restoreReportText(envelope v2preview.Envelope, run *v2ledger.RestoreRun, sourceRunID string, completed bool, verbose bool) string {
	envelope = v2preview.NormalizeEnvelope(envelope)
	source := restoreSourceRunID(run, sourceRunID)
	backupRunID := restoreBackupBeforeRunID(run)
	applied := restoreUniquePathCount(envelope.Items, v2preview.ResultApplied)
	wouldChange := restoreUniquePathCount(envelope.Items, v2preview.ResultWouldChange)
	unchanged := restoreItemResultCount(envelope.Items, v2preview.ResultUnchanged)
	blocked := envelope.Summary.Blocked + envelope.Summary.Failed

	var b strings.Builder
	switch {
	case completed:
		b.WriteString("Restore completed.\n\n")
		if applied > 0 {
			fmt.Fprintf(&b, "Restored %d live %s from backup %s.\n\n", applied, plural(applied, "file", "files"), source)
		} else {
			fmt.Fprintf(&b, "No live files needed changes from backup %s.\n\n", source)
		}
	case restoreHasPostWriteFailure(envelope):
		b.WriteString("Restore failed before completion.\n\n")
	case blocked > 0:
		b.WriteString("Restore blocked before writing live files.\n\n")
	default:
		fmt.Fprintf(&b, "Restore preview for backup %s.\n\n", source)
		if wouldChange > 0 {
			fmt.Fprintf(&b, "Would restore %d live %s.\n\n", wouldChange, plural(wouldChange, "file", "files"))
		} else if unchanged > 0 {
			b.WriteString("No live files would change because live state already matches this backup.\n\n")
		}
	}

	if source != "" {
		b.WriteString("Source backup:\n")
		fmt.Fprintf(&b, "  %s\n\n", source)
	}
	restoreWriteListText(&b, envelope.Items, completed)
	restoreTypeText(&b, envelope.Items, completed)
	restoreReasonText(&b, envelope.Items, completed)
	restoreBackupText(&b, envelope.Items, backupRunID, completed)
	restoreSafetyText(&b, envelope, completed, backupRunID)
	restoreNextText(&b, envelope.Items, source, backupRunID, completed)
	if verbose {
		restoreVerboseText(&b, envelope, source, backupRunID)
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

func restoreWriteListText(b *strings.Builder, items []v2preview.Item, completed bool) {
	if len(items) == 0 {
		return
	}
	applied := restoreItemResultCount(items, v2preview.ResultApplied)
	wouldChange := restoreItemResultCount(items, v2preview.ResultWouldChange)
	blocked := restoreItemResultCount(items, v2preview.ResultBlocked) + restoreItemResultCount(items, v2preview.ResultFailed)
	switch {
	case completed && applied > 0:
		b.WriteString("Live file restored:\n")
	case completed:
		b.WriteString("Live file checked:\n")
	case blocked > 0:
		b.WriteString("Live file that would have been restored:\n")
	case wouldChange > 0:
		b.WriteString("Live file that would be written:\n")
	default:
		b.WriteString("Live file checked:\n")
	}
	for _, item := range items {
		path := defaultBackupString(item.LivePath, "<unknown live path>")
		fmt.Fprintf(b, "  - %s\n", path)
		fmt.Fprintf(b, "    Backup item: %s\n", selectedSettingLabelForBackup(restoreItemLabel(item)))
		if isSelectedValueRestoreDriver(item.Driver) {
			b.WriteString("    Restore type: whole file/artifact restore\n")
		}
	}
	b.WriteString("\n")
}

func restoreTypeText(b *strings.Builder, items []v2preview.Item, completed bool) {
	var selected *v2preview.Item
	for i := range items {
		if isSelectedValueRestoreDriver(items[i].Driver) {
			selected = &items[i]
			break
		}
	}
	if selected == nil {
		return
	}
	live := defaultBackupString(selected.LivePath, "the live backing file")
	setting := defaultBackupString(selected.SettingRef, "the selected value")
	b.WriteString("Restore type:\n")
	b.WriteString("  Whole file/artifact restore.\n")
	if completed {
		fmt.Fprintf(b, "  Restore wrote the backed-up %s artifact.\n", live)
		fmt.Fprintf(b, "  It did not edit only %s inside the file.\n\n", setting)
		return
	}
	fmt.Fprintf(b, "  Confirming restore would write the backed-up %s artifact.\n", live)
	fmt.Fprintf(b, "  It would not edit only %s inside the file.\n\n", setting)
}

func restoreReasonText(b *strings.Builder, items []v2preview.Item, completed bool) {
	if completed {
		return
	}
	reasons := make([]string, 0)
	for _, item := range items {
		if item.Result != v2preview.ResultBlocked && item.Result != v2preview.ResultFailed {
			continue
		}
		for _, diagnostic := range item.Diagnostics {
			if diagnostic.Message != "" {
				reasons = append(reasons, diagnostic.Message)
			}
		}
		if len(item.Diagnostics) == 0 && item.Message != "" {
			reasons = append(reasons, item.Message)
		}
	}
	if len(reasons) == 0 {
		return
	}
	b.WriteString("Reason:\n")
	for _, reason := range reasons {
		fmt.Fprintf(b, "  %s\n", reason)
	}
	b.WriteString("\n")
}

func restoreBackupText(b *strings.Builder, items []v2preview.Item, backupRunID string, completed bool) {
	if !completed {
		return
	}
	b.WriteString("Backup before restore:\n")
	if restoreItemResultCount(items, v2preview.ResultApplied) == 0 {
		b.WriteString("  No recovery handle was created because no live write was needed.\n\n")
		return
	}
	if backupRunID != "" {
		fmt.Fprintf(b, "  Created recovery handle %s for the live state as it existed immediately before this restore.\n\n", backupRunID)
		return
	}
	b.WriteString("  Recovery handle is missing for at least one changed restore item; treat this as an error and inspect technical details.\n\n")
}

func restoreSafetyText(b *strings.Builder, envelope v2preview.Envelope, completed bool, backupRunID string) {
	b.WriteString("Safety:\n")
	switch {
	case completed && restoreItemResultCount(envelope.Items, v2preview.ResultApplied) > 0:
		b.WriteString("  Confirmed restore changed only the live files listed above.\n")
	case completed:
		b.WriteString("  No files changed because live state already matched the backup.\n")
	case restoreHasPostWriteFailure(envelope):
		b.WriteString("  Restore did not complete. Any written items were rolled back when possible; inspect diagnostics before retrying.\n")
	case envelope.Summary.Blocked > 0 || envelope.Summary.Failed > 0:
		b.WriteString("  No files changed.\n")
	default:
		b.WriteString("  Dry run: no files changed.\n")
	}
	if !completed {
		if envelope.Summary.Blocked > 0 || envelope.Summary.Failed > 0 {
			b.WriteString("  No backup-before-restore recovery handle was created.\n")
		} else {
			b.WriteString("  No backup-before-restore recovery handle was created because this dry run did not write live files.\n")
		}
	} else if backupRunID == "" && restoreItemResultCount(envelope.Items, v2preview.ResultApplied) > 0 {
		b.WriteString("  Backup-before-restore recovery handle is missing for a changed restore item.\n")
	}
	b.WriteString("  Values and backup payload contents are hidden.\n")
	b.WriteString("  Internal artifact refs are hidden from default output.\n\n")
}

func restoreNextText(b *strings.Builder, items []v2preview.Item, source string, backupRunID string, completed bool) {
	firstRef := ""
	for _, item := range items {
		firstRef = restoreItemLabel(item)
		if firstRef != "" {
			break
		}
	}
	postWriteFailure := restoreHasPostWriteFailure(v2preview.Envelope{Items: items})
	blocked := restoreItemResultCount(items, v2preview.ResultBlocked)+restoreItemResultCount(items, v2preview.ResultFailed) > 0
	b.WriteString("Next:\n")
	switch {
	case completed && backupRunID != "":
		if firstRef != "" {
			fmt.Fprintf(b, "  Check current drift for the restored setting:\n  dotfiles-manager status %s\n\n", firstRef)
		}
		fmt.Fprintf(b, "  Preview recovery of the pre-restore live state:\n  dotfiles-manager restore %s --dry-run\n\n", backupRunID)
	case completed:
		b.WriteString("  No recovery command is shown because no backup-before-restore recovery handle exists.\n\n")
	case postWriteFailure:
		b.WriteString("  Inspect the diagnostics above before retrying restore.\n\n")
	case blocked:
		if source != "" {
			fmt.Fprintf(b, "  Review the source backup metadata:\n  dotfiles-manager backup show %s\n\n", source)
		}
		b.WriteString("  Resolve the blocker above before retrying restore.\n\n")
	default:
		if source != "" {
			fmt.Fprintf(b, "  Review the backup metadata:\n  dotfiles-manager backup show %s\n\n", source)
			fmt.Fprintf(b, "  If this is the backup you want, confirm restore:\n  dotfiles-manager restore %s --yes\n\n", source)
		} else {
			b.WriteString("  Review available backups:\n  dotfiles-manager backup list\n\n")
		}
	}
}

func restoreHasPostWriteFailure(envelope v2preview.Envelope) bool {
	for _, item := range envelope.Items {
		for _, diagnostic := range item.Diagnostics {
			code := strings.ToLower(diagnostic.Code)
			message := strings.ToLower(diagnostic.Message)
			if strings.Contains(code, "rollback") || strings.Contains(message, "rollback") {
				return true
			}
			if strings.Contains(code, "execution-failure") {
				return true
			}
		}
	}
	return false
}

func restoreVerboseText(b *strings.Builder, envelope v2preview.Envelope, source string, backupRunID string) {
	b.WriteString("Technical details:\n")
	fmt.Fprintf(b, "  command=%s\n", envelope.Command)
	if envelope.RunID != "" {
		fmt.Fprintf(b, "  restoreRun=%s\n", envelope.RunID)
	}
	if source != "" {
		fmt.Fprintf(b, "  sourceBackup=%s\n", source)
	}
	if backupRunID != "" {
		fmt.Fprintf(b, "  backupBeforeRestore=%s\n", backupRunID)
	}
	if len(envelope.ProfileStack) > 0 {
		fmt.Fprintf(b, "  profiles=%s\n", strings.Join(envelope.ProfileStack, " > "))
	}
	fmt.Fprintf(b, "  summary=status:%s changed:%d blocked:%d applied:%d saved:%d skipped:%d failed:%d\n",
		envelope.Summary.Status,
		envelope.Summary.Changed,
		envelope.Summary.Blocked,
		envelope.Summary.Applied,
		envelope.Summary.Saved,
		envelope.Summary.Skipped,
		envelope.Summary.Failed,
	)
	if envelope.LedgerRef != "" {
		fmt.Fprintf(b, "  ledger=%s\n", envelope.LedgerRef)
	}
	for _, item := range envelope.Items {
		fmt.Fprintf(b, "  item=%s\n", defaultBackupString(restoreItemLabel(item), "<unknown>"))
		fmt.Fprintf(b, "    target=%s\n", item.TargetRef)
		fmt.Fprintf(b, "    resource=%s\n", item.ResourceID)
		fmt.Fprintf(b, "    driver=%s\n", item.Driver)
		fmt.Fprintf(b, "    operation=%s\n", item.Operation)
		fmt.Fprintf(b, "    state=%s\n", item.State)
		fmt.Fprintf(b, "    result=%s\n", item.Result)
		fmt.Fprintf(b, "    dryRun=%t\n", item.DryRun)
		fmt.Fprintf(b, "    change=%s\n", item.Change.Kind)
		if item.LivePath != "" {
			fmt.Fprintf(b, "    live=%s\n", item.LivePath)
		}
		if item.DesiredRelPath != "" {
			fmt.Fprintf(b, "    desiredArtifact=%s\n", item.DesiredRelPath)
		}
		if item.DesiredURI != "" {
			fmt.Fprintf(b, "    desiredURI=%s\n", item.DesiredURI)
		}
		fmt.Fprintf(b, "    backupPolicy=%s\n", item.Backup.Policy)
		if item.Backup.Ref != "" {
			fmt.Fprintf(b, "    backupRef=%s\n", item.Backup.Ref)
		}
		for _, diagnostic := range item.Diagnostics {
			fmt.Fprintf(b, "    diagnostic=%s/%s", diagnostic.Severity, diagnostic.Code)
			if diagnostic.Message != "" {
				fmt.Fprintf(b, ": %s", diagnostic.Message)
			}
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
}

func restoreSourceRunID(run *v2ledger.RestoreRun, fallback string) string {
	if run != nil && strings.TrimSpace(run.SourceBackup.RunID) != "" {
		return strings.TrimSpace(run.SourceBackup.RunID)
	}
	return strings.TrimSpace(fallback)
}

func restoreBackupBeforeRunID(run *v2ledger.RestoreRun) string {
	if run == nil || run.BackupBeforeRestore == nil {
		return ""
	}
	return strings.TrimSpace(run.BackupBeforeRestore.RunID)
}

func restoreRecordKey(targetRef string, settingRef string, operation string, resourceID string) string {
	return strings.TrimSpace(targetRef) + "\x00" + strings.TrimSpace(settingRef) + "\x00" + strings.TrimSpace(operation) + "\x00" + strings.TrimSpace(resourceID)
}

func restoreItemLabel(item v2preview.Item) string {
	if strings.TrimSpace(item.SettingRef) != "" {
		return strings.TrimSpace(item.SettingRef)
	}
	return strings.TrimSpace(item.TargetRef)
}

func firstString(values []string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func normalizedLedgerStatesEqual(left v2ledger.NormalizedState, right v2ledger.NormalizedState) bool {
	return left.Exists == right.Exists &&
		left.Hash == right.Hash &&
		left.Normalizer == right.Normalizer &&
		left.DriverVersion == right.DriverVersion &&
		left.Size == right.Size &&
		left.EntryCount == right.EntryCount &&
		left.FileCount == right.FileCount &&
		left.DirCount == right.DirCount
}

func normalizedStateSnapshot(state v2ledger.NormalizedState) v2preview.Snapshot {
	return v2preview.Snapshot{
		Exists:     state.Exists,
		Size:       state.Size,
		SHA256:     state.Hash,
		EntryCount: state.EntryCount,
		FileCount:  state.FileCount,
		DirCount:   state.DirCount,
	}
}

func restoreItemResultCount(items []v2preview.Item, result v2preview.Result) int {
	count := 0
	for _, item := range items {
		if item.Result == result {
			count++
		}
	}
	return count
}

func restoreUniquePathCount(items []v2preview.Item, result v2preview.Result) int {
	seen := map[string]bool{}
	count := 0
	for _, item := range items {
		if item.Result != result {
			continue
		}
		key := defaultBackupString(item.LivePath, restoreItemLabel(item))
		if seen[key] {
			continue
		}
		seen[key] = true
		count++
	}
	return count
}

func isSelectedValueRestoreDriver(driver string) bool {
	switch strings.TrimSpace(driver) {
	case v2recipe.IniFileDriverID, v2recipe.JSONFileDriverID, v2recipe.YAMLFileDriverID, v2recipe.TOMLFileDriverID, v2recipe.PlistFileDriverID:
		return true
	default:
		return false
	}
}

func plural(count int, singular string, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
}
