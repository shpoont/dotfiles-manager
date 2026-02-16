package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/shpoont/dotfiles-manager/internal/config"
	"github.com/shpoont/dotfiles-manager/internal/dfmerr"
)

var (
	chmodPath   = os.Chmod
	chtimesPath = os.Chtimes
)

type deployCounts struct {
	copied           int
	removedUnmanaged int
}

type deployCopyOperation struct {
	path      string
	change    string
	typeID    string
	sourceAbs string
	targetAbs string
}

type deployRemoveOperation struct {
	path      string
	typeID    string
	targetAbs string
}

func buildDeploySyncPayloads(cfg *config.Config, selections []syncSelection, dryRun bool) ([]any, map[string]any, error) {
	payloads := make([]any, 0, len(selections))
	summary := deployCounts{}

	for _, selection := range selections {
		syncCfg := cfg.Syncs[selection.Index]
		payload, counts, err := evaluateDeploySync(selection.Index, syncCfg, selection, dryRun)
		if err != nil {
			if len(payloads) > 0 {
				err = markPartial(err)
			}
			return nil, nil, err
		}
		payloads = append(payloads, payload)
		summary.copied += counts.copied
		summary.removedUnmanaged += counts.removedUnmanaged
	}

	summaryPayload := map[string]any{
		"sync_count":              len(payloads),
		"copied_count":            summary.copied,
		"removed_unmanaged_count": summary.removedUnmanaged,
	}

	return payloads, summaryPayload, nil
}

func evaluateDeploySync(syncIndex int, syncCfg config.Sync, selection syncSelection, dryRun bool) (map[string]any, deployCounts, error) {
	sourceEntries, err := scanSyncEntries(selection.SourceRoot, selection.ScopePrefix)
	if err != nil {
		return nil, deployCounts{}, err
	}
	targetEntries, err := scanSyncEntries(selection.TargetRoot, selection.ScopePrefix)
	if err != nil {
		return nil, deployCounts{}, err
	}

	allPaths := unionPaths(sourceEntries, targetEntries)

	copyOps := make([]deployCopyOperation, 0)
	removeOps := make([]deployRemoveOperation, 0)
	copiedPayload := make([]any, 0)
	removedPayload := make([]any, 0)

	for _, relPath := range allPaths {
		sourceEntry, hasSource := sourceEntries[relPath]
		targetEntry, hasTarget := targetEntries[relPath]

		switch {
		case hasSource && !hasTarget:
			copyOps = append(copyOps, deployCopyOperation{
				path:      relPath,
				change:    "create",
				typeID:    sourceEntry.typeID,
				sourceAbs: sourceEntry.absPath,
				targetAbs: filepath.Join(selection.TargetRoot, filepath.FromSlash(relPath)),
			})
			copiedPayload = append(copiedPayload, buildDeployCopied(relPath, "create", sourceEntry.typeID))

		case hasSource && hasTarget:
			change := ""
			if sourceEntry.typeID != targetEntry.typeID {
				change = "replace_type"
			} else {
				different, diffErr := entriesDifferent(sourceEntry, targetEntry)
				if diffErr != nil {
					return nil, deployCounts{}, diffErr
				}
				if different {
					change = "update"
				}
			}
			if change != "" {
				copyOps = append(copyOps, deployCopyOperation{
					path:      relPath,
					change:    change,
					typeID:    sourceEntry.typeID,
					sourceAbs: sourceEntry.absPath,
					targetAbs: targetEntry.absPath,
				})
				copiedPayload = append(copiedPayload, buildDeployCopied(relPath, change, sourceEntry.typeID))
			}

		case !hasSource && hasTarget:
			matchRemovable, removableErr := matchesAny(
				relPath,
				syncCfg.On.Deploy.RemoveUnmanaged,
				fmt.Sprintf("syncs[%d].on.deploy.remove-unmanaged", syncIndex),
			)
			if removableErr != nil {
				return nil, deployCounts{}, removableErr
			}
			if matchRemovable {
				removeOps = append(removeOps, deployRemoveOperation{
					path:      relPath,
					typeID:    targetEntry.typeID,
					targetAbs: targetEntry.absPath,
				})
				removedPayload = append(removedPayload, buildTypedPath(relPath, targetEntry.typeID))
			}
		}
	}

	if !dryRun {
		appliedAny := false
		for _, op := range copyOps {
			if err := applyDeployCopy(op); err != nil {
				if appliedAny {
					err = markPartial(err)
				}
				return nil, deployCounts{}, err
			}
			appliedAny = true
		}
		for _, op := range removeOps {
			if err := applyDeployRemove(op); err != nil {
				if appliedAny {
					err = markPartial(err)
				}
				return nil, deployCounts{}, err
			}
			appliedAny = true
		}
	}

	payload := map[string]any{
		"sync_index":        syncIndex,
		"source_root":       selection.SourceRoot,
		"target_root":       selection.TargetRoot,
		"scope_prefix":      selection.ScopePrefix,
		"copied":            copiedPayload,
		"removed_unmanaged": removedPayload,
	}

	counts := deployCounts{
		copied:           len(copiedPayload),
		removedUnmanaged: len(removedPayload),
	}
	return payload, counts, nil
}

func applyDeployCopy(op deployCopyOperation) error {
	if op.change == "replace_type" {
		if err := removePath(op.targetAbs); err != nil {
			return dfmerr.Wrap(dfmerr.CodeTypeReplace, fmt.Sprintf("Failed to replace path type: %s", op.targetAbs), map[string]any{"path": op.targetAbs}, err)
		}
	}

	switch op.typeID {
	case "dir":
		return copyDir(op.sourceAbs, op.targetAbs)
	case "file":
		if err := copyFile(op.sourceAbs, op.targetAbs); err != nil {
			return err
		}
		return nil
	case "symlink":
		if err := copySymlink(op.sourceAbs, op.targetAbs); err != nil {
			return err
		}
		return nil
	default:
		return nil
	}
}

func copyDir(sourcePath, targetPath string) error {
	info, err := os.Lstat(sourcePath)
	if err != nil {
		return dfmerr.Wrap(dfmerr.CodeIORead, fmt.Sprintf("Read failed: %s", sourcePath), map[string]any{"path": sourcePath}, err)
	}
	if err := os.MkdirAll(targetPath, info.Mode().Perm()); err != nil {
		return dfmerr.Wrap(dfmerr.CodeIOWrite, fmt.Sprintf("Write failed: %s", targetPath), map[string]any{"path": targetPath}, err)
	}
	if err := applyTierAMetadata(info, targetPath); err != nil {
		return err
	}
	return nil
}

func copyFile(sourcePath, targetPath string) error {
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		return dfmerr.Wrap(dfmerr.CodeIORead, fmt.Sprintf("Read failed: %s", sourcePath), map[string]any{"path": sourcePath}, err)
	}
	info, err := os.Lstat(sourcePath)
	if err != nil {
		return dfmerr.Wrap(dfmerr.CodeIORead, fmt.Sprintf("Read failed: %s", sourcePath), map[string]any{"path": sourcePath}, err)
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return dfmerr.Wrap(dfmerr.CodeIOWrite, fmt.Sprintf("Write failed: %s", filepath.Dir(targetPath)), map[string]any{"path": filepath.Dir(targetPath)}, err)
	}
	if err := os.WriteFile(targetPath, content, info.Mode().Perm()); err != nil {
		return dfmerr.Wrap(dfmerr.CodeIOWrite, fmt.Sprintf("Write failed: %s", targetPath), map[string]any{"path": targetPath}, err)
	}
	if err := applyTierAMetadata(info, targetPath); err != nil {
		return err
	}
	return nil
}

func copySymlink(sourcePath, targetPath string) error {
	linkTarget, err := os.Readlink(sourcePath)
	if err != nil {
		return dfmerr.Wrap(dfmerr.CodeIORead, fmt.Sprintf("Read failed: %s", sourcePath), map[string]any{"path": sourcePath}, err)
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return dfmerr.Wrap(dfmerr.CodeIOWrite, fmt.Sprintf("Write failed: %s", filepath.Dir(targetPath)), map[string]any{"path": filepath.Dir(targetPath)}, err)
	}
	if _, err := os.Lstat(targetPath); err == nil {
		if err := removePath(targetPath); err != nil {
			return dfmerr.Wrap(dfmerr.CodeIOWrite, fmt.Sprintf("Write failed: %s", targetPath), map[string]any{"path": targetPath}, err)
		}
	} else if !os.IsNotExist(err) {
		return dfmerr.Wrap(dfmerr.CodeIORead, fmt.Sprintf("Read failed: %s", targetPath), map[string]any{"path": targetPath}, err)
	}
	if err := os.Symlink(linkTarget, targetPath); err != nil {
		return dfmerr.Wrap(dfmerr.CodeIOWrite, fmt.Sprintf("Write failed: %s", targetPath), map[string]any{"path": targetPath}, err)
	}
	return nil
}

func applyDeployRemove(op deployRemoveOperation) error {
	if err := removePath(op.targetAbs); err != nil {
		return dfmerr.Wrap(dfmerr.CodeIORemove, fmt.Sprintf("Remove failed: %s", op.targetAbs), map[string]any{"path": op.targetAbs}, err)
	}
	return nil
}

func removePath(path string) error {
	if err := os.RemoveAll(path); err != nil {
		return err
	}
	return nil
}

func applyTierAMetadata(sourceInfo os.FileInfo, targetPath string) error {
	mode := sourceInfo.Mode().Perm()
	if err := chmodPath(targetPath, mode); err != nil {
		return dfmerr.Wrap(dfmerr.CodeMetadataApply, fmt.Sprintf("Failed to apply metadata: %s", targetPath), map[string]any{"path": targetPath, "metadata": "mode"}, err)
	}

	modTime := sourceInfo.ModTime()
	if err := chtimesPath(targetPath, modTime, modTime); err != nil {
		return dfmerr.Wrap(dfmerr.CodeMetadataApply, fmt.Sprintf("Failed to apply metadata: %s", targetPath), map[string]any{"path": targetPath, "metadata": "mtime"}, err)
	}

	return nil
}

func buildDeployCopied(path, change, entryType string) map[string]any {
	return map[string]any{
		"path":   path,
		"change": change,
		"type":   entryType,
	}
}
