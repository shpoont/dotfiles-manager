package app

import (
	"bytes"
	"fmt"
	"os"
	"unicode/utf8"

	"github.com/pmezard/go-difflib/difflib"
	"github.com/shpoont/dotfiles-manager/internal/config"
	"github.com/shpoont/dotfiles-manager/internal/dfmerr"
)

const (
	diffDirectionBoth   = "both"
	diffDirectionDeploy = "deploy"
	diffDirectionImport = "import"

	diffDefaultContextLines = 3
	diffPatchSizeLimitBytes = 1 << 20
)

type diffCounts struct {
	deploy            int
	importCount       int
	incomingUnmanaged int
	removeUnmanaged   int
	removeMissing     int
	unifiedPatch      int
	binary            int
	typeChange        int
	omitted           int
}

func isValidDiffDirection(direction string) bool {
	switch direction {
	case diffDirectionBoth, diffDirectionDeploy, diffDirectionImport:
		return true
	default:
		return false
	}
}

func buildDiffSyncPayloads(cfg *config.Config, selections []syncSelection, direction string, contextLines int, includePatch bool) ([]any, map[string]any, error) {
	payloads := make([]any, 0, len(selections))
	summary := diffCounts{}

	for _, selection := range selections {
		syncCfg := cfg.Syncs[selection.Index]
		payload, counts, err := evaluateDiffSync(selection.Index, syncCfg, selection, direction, contextLines, includePatch)
		if err != nil {
			return nil, nil, err
		}
		payloads = append(payloads, payload)
		summary.deploy += counts.deploy
		summary.importCount += counts.importCount
		summary.incomingUnmanaged += counts.incomingUnmanaged
		summary.removeUnmanaged += counts.removeUnmanaged
		summary.removeMissing += counts.removeMissing
		summary.unifiedPatch += counts.unifiedPatch
		summary.binary += counts.binary
		summary.typeChange += counts.typeChange
		summary.omitted += counts.omitted
	}

	summaryPayload := map[string]any{
		"sync_count":               len(payloads),
		"deploy_count":             summary.deploy,
		"import_count":             summary.importCount,
		"incoming_unmanaged_count": summary.incomingUnmanaged,
		"remove_unmanaged_count":   summary.removeUnmanaged,
		"remove_missing_count":     summary.removeMissing,
		"unified_patch_count":      summary.unifiedPatch,
		"binary_count":             summary.binary,
		"type_change_count":        summary.typeChange,
		"omitted_count":            summary.omitted,
		"operation_count":          summary.deploy + summary.importCount + summary.incomingUnmanaged + summary.removeUnmanaged + summary.removeMissing,
	}

	return payloads, summaryPayload, nil
}

func evaluateDiffSync(syncIndex int, syncCfg config.Sync, selection syncSelection, direction string, contextLines int, includePatch bool) (map[string]any, diffCounts, error) {
	sourceEntries, err := scanSyncEntries(selection.SourceRoot, selection.ScopePrefix)
	if err != nil {
		return nil, diffCounts{}, err
	}
	targetEntries, err := scanTargetEntries(selection.TargetRoot, selection.ScopePrefix, sourceEntries, statusTargetScanPatterns(syncCfg))
	if err != nil {
		return nil, diffCounts{}, err
	}

	allPaths := unionPaths(sourceEntries, targetEntries)
	operations := make([]any, 0)
	counts := diffCounts{}

	for _, relPath := range allPaths {
		sourceEntry, hasSource := sourceEntries[relPath]
		targetEntry, hasTarget := targetEntries[relPath]

		switch {
		case hasSource && hasTarget:
			change := ""
			if sourceEntry.typeID != targetEntry.typeID {
				change = "replace_type"
			} else {
				different, diffErr := entriesDifferent(sourceEntry, targetEntry)
				if diffErr != nil {
					return nil, diffCounts{}, diffErr
				}
				if different {
					change = "update"
				}
			}
			if change == "" {
				continue
			}

			sourceCopy := sourceEntry
			targetCopy := targetEntry

			if phaseAllowedByDirection("deploy", direction) {
				op, opErr := buildDiffOperation("deploy", change, relPath, &sourceCopy, &targetCopy, contextLines, includePatch)
				if opErr != nil {
					return nil, diffCounts{}, opErr
				}
				operations = append(operations, op)
				incrementDiffPhaseCount(&counts, "deploy")
				incrementDiffKindCount(&counts, stringValue(op["diff_kind"]))
			}
			if phaseAllowedByDirection("import", direction) {
				op, opErr := buildDiffOperation("import", change, relPath, &sourceCopy, &targetCopy, contextLines, includePatch)
				if opErr != nil {
					return nil, diffCounts{}, opErr
				}
				operations = append(operations, op)
				incrementDiffPhaseCount(&counts, "import")
				incrementDiffKindCount(&counts, stringValue(op["diff_kind"]))
			}

		case hasSource && !hasTarget:
			sourceCopy := sourceEntry
			if phaseAllowedByDirection("deploy", direction) {
				op, opErr := buildDiffOperation("deploy", "create", relPath, &sourceCopy, nil, contextLines, includePatch)
				if opErr != nil {
					return nil, diffCounts{}, opErr
				}
				operations = append(operations, op)
				incrementDiffPhaseCount(&counts, "deploy")
				incrementDiffKindCount(&counts, stringValue(op["diff_kind"]))
			}

			matchMissing, matchErr := matchesIncludeExclude(
				relPath,
				syncCfg.On.Import.RemoveMissing.Include,
				fmt.Sprintf("syncs[%d].on.import.remove-missing.include", syncIndex),
				syncCfg.On.Import.RemoveMissing.Exclude,
				fmt.Sprintf("syncs[%d].on.import.remove-missing.exclude", syncIndex),
			)
			if matchErr != nil {
				return nil, diffCounts{}, matchErr
			}
			if matchMissing && phaseAllowedByDirection("remove_missing", direction) {
				op, opErr := buildDiffOperation("remove_missing", "remove", relPath, &sourceCopy, nil, contextLines, includePatch)
				if opErr != nil {
					return nil, diffCounts{}, opErr
				}
				operations = append(operations, op)
				incrementDiffPhaseCount(&counts, "remove_missing")
				incrementDiffKindCount(&counts, stringValue(op["diff_kind"]))
			}

		case !hasSource && hasTarget:
			targetCopy := targetEntry
			matchIncoming, incomingErr := matchesIncludeExclude(
				relPath,
				syncCfg.On.Import.AddUnmanaged.Include,
				fmt.Sprintf("syncs[%d].on.import.add-unmanaged.include", syncIndex),
				syncCfg.On.Import.AddUnmanaged.Exclude,
				fmt.Sprintf("syncs[%d].on.import.add-unmanaged.exclude", syncIndex),
			)
			if incomingErr != nil {
				return nil, diffCounts{}, incomingErr
			}
			if matchIncoming && phaseAllowedByDirection("incoming_unmanaged", direction) {
				op, opErr := buildDiffOperation("incoming_unmanaged", "add", relPath, nil, &targetCopy, contextLines, includePatch)
				if opErr != nil {
					return nil, diffCounts{}, opErr
				}
				operations = append(operations, op)
				incrementDiffPhaseCount(&counts, "incoming_unmanaged")
				incrementDiffKindCount(&counts, stringValue(op["diff_kind"]))
			}

			matchRemovable, removableErr := matchesAny(
				relPath,
				syncCfg.On.Deploy.RemoveUnmanaged,
				fmt.Sprintf("syncs[%d].on.deploy.remove-unmanaged", syncIndex),
			)
			if removableErr != nil {
				return nil, diffCounts{}, removableErr
			}
			if matchRemovable && phaseAllowedByDirection("remove_unmanaged", direction) {
				op, opErr := buildDiffOperation("remove_unmanaged", "remove", relPath, nil, &targetCopy, contextLines, includePatch)
				if opErr != nil {
					return nil, diffCounts{}, opErr
				}
				operations = append(operations, op)
				incrementDiffPhaseCount(&counts, "remove_unmanaged")
				incrementDiffKindCount(&counts, stringValue(op["diff_kind"]))
			}
		}
	}

	display := buildSyncDisplay(syncIndex, syncCfg)
	payload := map[string]any{
		"sync_index":   syncIndex,
		"sync":         display.Label,
		"target":       display.Target,
		"source":       display.Source,
		"source_root":  selection.SourceRoot,
		"target_root":  selection.TargetRoot,
		"scope_prefix": selection.ScopePrefix,
		"operations":   operations,
		"counts": map[string]any{
			"deploy":             counts.deploy,
			"import":             counts.importCount,
			"incoming_unmanaged": counts.incomingUnmanaged,
			"remove_unmanaged":   counts.removeUnmanaged,
			"remove_missing":     counts.removeMissing,
			"unified_patch":      counts.unifiedPatch,
			"binary":             counts.binary,
			"type_change":        counts.typeChange,
			"omitted":            counts.omitted,
			"operation_count":    len(operations),
		},
	}

	return payload, counts, nil
}

func phaseAllowedByDirection(phase, direction string) bool {
	switch direction {
	case diffDirectionDeploy:
		return phase == "deploy" || phase == "remove_unmanaged"
	case diffDirectionImport:
		return phase == "import" || phase == "incoming_unmanaged" || phase == "remove_missing"
	default:
		return true
	}
}

func incrementDiffPhaseCount(counts *diffCounts, phase string) {
	switch phase {
	case "deploy":
		counts.deploy++
	case "import":
		counts.importCount++
	case "incoming_unmanaged":
		counts.incomingUnmanaged++
	case "remove_unmanaged":
		counts.removeUnmanaged++
	case "remove_missing":
		counts.removeMissing++
	}
}

func incrementDiffKindCount(counts *diffCounts, kind string) {
	switch kind {
	case "unified":
		counts.unifiedPatch++
	case "binary":
		counts.binary++
	case "type_change":
		counts.typeChange++
	default:
		counts.omitted++
	}
}

func buildDiffOperation(phase, action, relPath string, sourceEntry, targetEntry *statusEntry, contextLines int, includePatch bool) (map[string]any, error) {
	op := map[string]any{
		"phase":  phase,
		"action": statusActionLabel(action),
		"state":  "candidate",
		"path":   relPath,
	}

	sourceType := "missing"
	targetType := "missing"
	if sourceEntry != nil {
		sourceType = sourceEntry.typeID
	}
	if targetEntry != nil {
		targetType = targetEntry.typeID
	}

	switch phase {
	case "deploy", "import":
		op["source_type"] = sourceType
		op["target_type"] = targetType
	default:
		entryType := targetType
		if phase == "remove_missing" {
			entryType = sourceType
		}
		op["type"] = entryType
	}

	diffMeta, err := buildDiffMetadata(phase, relPath, sourceEntry, targetEntry, contextLines, includePatch)
	if err != nil {
		return nil, err
	}
	for key, value := range diffMeta {
		op[key] = value
	}

	return op, nil
}

func buildDiffMetadata(phase, relPath string, sourceEntry, targetEntry *statusEntry, contextLines int, includePatch bool) (map[string]any, error) {
	oldEntry, newEntry, oldLabel, newLabel := diffPerspective(phase, relPath, sourceEntry, targetEntry)

	result := map[string]any{
		"old_label": oldLabel,
		"new_label": newLabel,
	}

	if (oldEntry != nil && oldEntry.typeID == "dir") || (newEntry != nil && newEntry.typeID == "dir") {
		result["diff_kind"] = "omitted"
		result["reason"] = "directory diff omitted"
		result["patch_available"] = false
		result["patch_included"] = false
		return result, nil
	}

	if oldEntry != nil && newEntry != nil && oldEntry.typeID != newEntry.typeID {
		result["diff_kind"] = "type_change"
		result["reason"] = "type differs"
		result["patch_available"] = false
		result["patch_included"] = false
		return result, nil
	}

	oldBytes, oldBinary, err := diffEntryContent(oldEntry)
	if err != nil {
		return nil, err
	}
	newBytes, newBinary, err := diffEntryContent(newEntry)
	if err != nil {
		return nil, err
	}

	if oldBinary || newBinary {
		result["diff_kind"] = "binary"
		result["reason"] = "binary differs"
		result["patch_available"] = false
		result["patch_included"] = false
		return result, nil
	}

	patch, err := unifiedPatch(oldLabel, newLabel, oldBytes, newBytes, contextLines)
	if err != nil {
		return nil, err
	}
	if len(patch) > diffPatchSizeLimitBytes {
		result["diff_kind"] = "omitted"
		result["reason"] = fmt.Sprintf("patch omitted: exceeds %d bytes", diffPatchSizeLimitBytes)
		result["patch_available"] = false
		result["patch_included"] = false
		return result, nil
	}

	result["diff_kind"] = "unified"
	result["patch_available"] = true
	result["patch_included"] = includePatch
	if includePatch {
		result["patch"] = patch
	}
	return result, nil
}

func diffPerspective(phase, relPath string, sourceEntry, targetEntry *statusEntry) (oldEntry *statusEntry, newEntry *statusEntry, oldLabel string, newLabel string) {
	switch phase {
	case "deploy":
		return targetEntry, sourceEntry, diffLabel("target", relPath, targetEntry != nil), diffLabel("source", relPath, sourceEntry != nil)
	case "import":
		return sourceEntry, targetEntry, diffLabel("source", relPath, sourceEntry != nil), diffLabel("target", relPath, targetEntry != nil)
	case "incoming_unmanaged":
		return nil, targetEntry, "/dev/null", diffLabel("target", relPath, targetEntry != nil)
	case "remove_unmanaged":
		return targetEntry, nil, diffLabel("target", relPath, targetEntry != nil), "/dev/null"
	case "remove_missing":
		return sourceEntry, nil, diffLabel("source", relPath, sourceEntry != nil), "/dev/null"
	default:
		return nil, nil, "/dev/null", "/dev/null"
	}
}

func diffLabel(prefix, relPath string, exists bool) string {
	if !exists {
		return "/dev/null"
	}
	return prefix + "/" + relPath
}

func diffEntryContent(entry *statusEntry) ([]byte, bool, error) {
	if entry == nil {
		return []byte{}, false, nil
	}

	switch entry.typeID {
	case "file":
		content, err := os.ReadFile(entry.absPath)
		if err != nil {
			return nil, false, dfmerr.Wrap(dfmerr.CodeIORead, fmt.Sprintf("Read failed: %s", entry.absPath), map[string]any{"path": entry.absPath}, err)
		}
		return content, isBinaryContent(content), nil
	case "symlink":
		linkTarget, err := os.Readlink(entry.absPath)
		if err != nil {
			return nil, false, dfmerr.Wrap(dfmerr.CodeIORead, fmt.Sprintf("Read failed: %s", entry.absPath), map[string]any{"path": entry.absPath}, err)
		}
		return []byte("symlink -> " + linkTarget + "\n"), false, nil
	default:
		return []byte{}, false, nil
	}
}

func isBinaryContent(content []byte) bool {
	if len(content) == 0 {
		return false
	}
	if bytes.IndexByte(content, 0) >= 0 {
		return true
	}
	return !utf8.Valid(content)
}

func unifiedPatch(oldLabel, newLabel string, oldBytes, newBytes []byte, contextLines int) (string, error) {
	patch, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(string(oldBytes)),
		B:        difflib.SplitLines(string(newBytes)),
		FromFile: oldLabel,
		ToFile:   newLabel,
		Context:  contextLines,
	})
	if err != nil {
		return "", dfmerr.Wrap(dfmerr.CodeIORead, "Read failed: diff generation", nil, err)
	}
	return patch, nil
}
