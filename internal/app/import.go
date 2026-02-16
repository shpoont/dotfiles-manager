package app

import (
	"fmt"
	"path/filepath"

	"github.com/shpoont/dotfiles-manager/internal/config"
)

type importCounts struct {
	updatedManifest int
	addedUnmanaged  int
	removedMissing  int
}

type importCopyOperation struct {
	path      string
	change    string
	typeID    string
	sourceAbs string
	targetAbs string
}

type importRemoveOperation struct {
	path      string
	typeID    string
	sourceAbs string
}

func buildImportSyncPayloads(cfg *config.Config, selections []syncSelection, dryRun bool) ([]any, map[string]any, error) {
	payloads := make([]any, 0, len(selections))
	summary := importCounts{}

	for _, selection := range selections {
		syncCfg := cfg.Syncs[selection.Index]
		payload, counts, err := evaluateImportSync(selection.Index, syncCfg, selection, dryRun)
		if err != nil {
			return nil, nil, err
		}
		payloads = append(payloads, payload)
		summary.updatedManifest += counts.updatedManifest
		summary.addedUnmanaged += counts.addedUnmanaged
		summary.removedMissing += counts.removedMissing
	}

	summaryPayload := map[string]any{
		"sync_count":             len(payloads),
		"updated_manifest_count": summary.updatedManifest,
		"added_unmanaged_count":  summary.addedUnmanaged,
		"removed_missing_count":  summary.removedMissing,
	}

	return payloads, summaryPayload, nil
}

func evaluateImportSync(syncIndex int, syncCfg config.Sync, selection syncSelection, dryRun bool) (map[string]any, importCounts, error) {
	sourceEntries, err := scanSyncEntries(selection.SourceRoot, selection.ScopePrefix)
	if err != nil {
		return nil, importCounts{}, err
	}
	targetEntries, err := scanSyncEntries(selection.TargetRoot, selection.ScopePrefix)
	if err != nil {
		return nil, importCounts{}, err
	}

	allPaths := unionPaths(sourceEntries, targetEntries)

	updatedManifestPayload := make([]any, 0)
	addedUnmanagedPayload := make([]any, 0)
	removedMissingPayload := make([]any, 0)
	copyOps := make([]importCopyOperation, 0)
	removeOps := make([]importRemoveOperation, 0)

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
					return nil, importCounts{}, diffErr
				}
				if different {
					change = "update"
				}
			}
			if change != "" {
				copyOps = append(copyOps, importCopyOperation{
					path:      relPath,
					change:    change,
					typeID:    targetEntry.typeID,
					sourceAbs: targetEntry.absPath,
					targetAbs: sourceEntry.absPath,
				})
				updatedManifestPayload = append(updatedManifestPayload, buildImportManifest(relPath, change, targetEntry.typeID))
			}

		case !hasSource && hasTarget:
			matchAdd, addErr := matchesIncludeExclude(
				relPath,
				syncCfg.On.Import.AddUnmanaged.Include,
				fmt.Sprintf("syncs[%d].on.import.add-unmanaged.include", syncIndex),
				syncCfg.On.Import.AddUnmanaged.Exclude,
				fmt.Sprintf("syncs[%d].on.import.add-unmanaged.exclude", syncIndex),
			)
			if addErr != nil {
				return nil, importCounts{}, addErr
			}
			if matchAdd {
				copyOps = append(copyOps, importCopyOperation{
					path:      relPath,
					change:    "create",
					typeID:    targetEntry.typeID,
					sourceAbs: targetEntry.absPath,
					targetAbs: filepath.Join(selection.SourceRoot, filepath.FromSlash(relPath)),
				})
				addedUnmanagedPayload = append(addedUnmanagedPayload, buildTypedPath(relPath, targetEntry.typeID))
			}

		case hasSource && !hasTarget:
			matchRemove, removeErr := matchesIncludeExclude(
				relPath,
				syncCfg.On.Import.RemoveMissing.Include,
				fmt.Sprintf("syncs[%d].on.import.remove-missing.include", syncIndex),
				syncCfg.On.Import.RemoveMissing.Exclude,
				fmt.Sprintf("syncs[%d].on.import.remove-missing.exclude", syncIndex),
			)
			if removeErr != nil {
				return nil, importCounts{}, removeErr
			}
			if matchRemove {
				removeOps = append(removeOps, importRemoveOperation{
					path:      relPath,
					typeID:    sourceEntry.typeID,
					sourceAbs: sourceEntry.absPath,
				})
				removedMissingPayload = append(removedMissingPayload, buildTypedPath(relPath, sourceEntry.typeID))
			}
		}
	}

	if !dryRun {
		for _, op := range copyOps {
			if err := applyImportCopy(op); err != nil {
				return nil, importCounts{}, err
			}
		}
		for _, op := range removeOps {
			if err := applyImportRemove(op); err != nil {
				return nil, importCounts{}, err
			}
		}
	}

	payload := map[string]any{
		"sync_index":       syncIndex,
		"source_root":      selection.SourceRoot,
		"target_root":      selection.TargetRoot,
		"scope_prefix":     selection.ScopePrefix,
		"updated_manifest": updatedManifestPayload,
		"added_unmanaged":  addedUnmanagedPayload,
		"removed_missing":  removedMissingPayload,
	}

	counts := importCounts{
		updatedManifest: len(updatedManifestPayload),
		addedUnmanaged:  len(addedUnmanagedPayload),
		removedMissing:  len(removedMissingPayload),
	}
	return payload, counts, nil
}

func applyImportCopy(op importCopyOperation) error {
	return applyDeployCopy(deployCopyOperation(op))
}

func applyImportRemove(op importRemoveOperation) error {
	return applyDeployRemove(deployRemoveOperation{
		path:      op.path,
		typeID:    op.typeID,
		targetAbs: op.sourceAbs,
	})
}

func buildImportManifest(path, change, entryType string) map[string]any {
	return map[string]any{
		"path":   path,
		"change": change,
		"type":   entryType,
	}
}
