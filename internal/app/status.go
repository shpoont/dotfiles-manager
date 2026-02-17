package app

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/shpoont/dotfiles-manager/internal/config"
	"github.com/shpoont/dotfiles-manager/internal/dfmerr"
)

type statusCounts struct {
	deployChanges      int
	importChanges      int
	incomingUnmanaged  int
	removableUnmanaged int
	removableMissing   int
}

type statusEntry struct {
	path    string
	absPath string
	typeID  string
}

func buildStatusSyncPayloads(cfg *config.Config, selections []syncSelection) ([]any, map[string]any, error) {
	payloads := make([]any, 0, len(selections))
	summary := statusCounts{}

	for _, selection := range selections {
		syncCfg := cfg.Syncs[selection.Index]
		payload, counts, err := evaluateStatusSync(selection.Index, syncCfg, selection)
		if err != nil {
			return nil, nil, err
		}
		payloads = append(payloads, payload)
		summary.deployChanges += counts.deployChanges
		summary.importChanges += counts.importChanges
		summary.incomingUnmanaged += counts.incomingUnmanaged
		summary.removableUnmanaged += counts.removableUnmanaged
		summary.removableMissing += counts.removableMissing
	}

	summaryPayload := map[string]any{
		"sync_count":               len(payloads),
		"deploy_count":             summary.deployChanges,
		"import_count":             summary.importChanges,
		"incoming_unmanaged_count": summary.incomingUnmanaged,
		"remove_unmanaged_count":   summary.removableUnmanaged,
		"remove_missing_count":     summary.removableMissing,
		"operation_count":          summary.deployChanges + summary.importChanges + summary.incomingUnmanaged + summary.removableUnmanaged + summary.removableMissing,
	}

	return payloads, summaryPayload, nil
}

func evaluateStatusSync(syncIndex int, syncCfg config.Sync, selection syncSelection) (map[string]any, statusCounts, error) {
	sourceEntries, err := scanSyncEntries(selection.SourceRoot, selection.ScopePrefix)
	if err != nil {
		return nil, statusCounts{}, err
	}
	targetEntries, err := scanTargetEntries(selection.TargetRoot, selection.ScopePrefix, sourceEntries, statusNeedsTargetWalk(syncCfg))
	if err != nil {
		return nil, statusCounts{}, err
	}

	allPaths := unionPaths(sourceEntries, targetEntries)

	deployChanges := make([]any, 0)
	importChanges := make([]any, 0)
	incomingUnmanaged := make([]any, 0)
	removableUnmanaged := make([]any, 0)
	removableMissing := make([]any, 0)
	operations := make([]any, 0)
	display := buildSyncDisplay(syncIndex, syncCfg)

	for _, relPath := range allPaths {
		sourceEntry, hasSource := sourceEntries[relPath]
		targetEntry, hasTarget := targetEntries[relPath]

		switch {
		case hasSource && hasTarget:
			if sourceEntry.typeID != targetEntry.typeID {
				change := buildChange(relPath, "replace_type", sourceEntry.typeID, targetEntry.typeID)
				deployChanges = append(deployChanges, change)
				importChanges = append(importChanges, change)
				operations = append(operations, buildStatusManagedOperation("deploy", relPath, "replace_type", sourceEntry.typeID, targetEntry.typeID))
				operations = append(operations, buildStatusManagedOperation("import", relPath, "replace_type", sourceEntry.typeID, targetEntry.typeID))
				continue
			}

			different, diffErr := entriesDifferent(sourceEntry, targetEntry)
			if diffErr != nil {
				return nil, statusCounts{}, diffErr
			}
			if different {
				change := buildChange(relPath, "update", sourceEntry.typeID, targetEntry.typeID)
				deployChanges = append(deployChanges, change)
				importChanges = append(importChanges, change)
				operations = append(operations, buildStatusManagedOperation("deploy", relPath, "update", sourceEntry.typeID, targetEntry.typeID))
				operations = append(operations, buildStatusManagedOperation("import", relPath, "update", sourceEntry.typeID, targetEntry.typeID))
			}

		case hasSource && !hasTarget:
			deployChanges = append(deployChanges, buildChange(relPath, "create", sourceEntry.typeID, "missing"))
			operations = append(operations, buildStatusManagedOperation("deploy", relPath, "create", sourceEntry.typeID, "missing"))

			matchMissing, matchErr := matchesIncludeExclude(
				relPath,
				syncCfg.On.Import.RemoveMissing.Include,
				fmt.Sprintf("syncs[%d].on.import.remove-missing.include", syncIndex),
				syncCfg.On.Import.RemoveMissing.Exclude,
				fmt.Sprintf("syncs[%d].on.import.remove-missing.exclude", syncIndex),
			)
			if matchErr != nil {
				return nil, statusCounts{}, matchErr
			}
			if matchMissing {
				removableMissing = append(removableMissing, buildTypedPath(relPath, sourceEntry.typeID))
				operations = append(operations, buildStatusTypedOperation("remove_missing", "remove", relPath, sourceEntry.typeID))
			}

		case !hasSource && hasTarget:
			matchIncoming, incomingErr := matchesIncludeExclude(
				relPath,
				syncCfg.On.Import.AddUnmanaged.Include,
				fmt.Sprintf("syncs[%d].on.import.add-unmanaged.include", syncIndex),
				syncCfg.On.Import.AddUnmanaged.Exclude,
				fmt.Sprintf("syncs[%d].on.import.add-unmanaged.exclude", syncIndex),
			)
			if incomingErr != nil {
				return nil, statusCounts{}, incomingErr
			}
			if matchIncoming {
				incomingUnmanaged = append(incomingUnmanaged, buildTypedPath(relPath, targetEntry.typeID))
				operations = append(operations, buildStatusTypedOperation("incoming_unmanaged", "add", relPath, targetEntry.typeID))
			}

			matchRemovable, removableErr := matchesAny(
				relPath,
				syncCfg.On.Deploy.RemoveUnmanaged,
				fmt.Sprintf("syncs[%d].on.deploy.remove-unmanaged", syncIndex),
			)
			if removableErr != nil {
				return nil, statusCounts{}, removableErr
			}
			if matchRemovable {
				removableUnmanaged = append(removableUnmanaged, buildTypedPath(relPath, targetEntry.typeID))
				operations = append(operations, buildStatusTypedOperation("remove_unmanaged", "remove", relPath, targetEntry.typeID))
			}
		}
	}

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
			"deploy":             len(deployChanges),
			"import":             len(importChanges),
			"incoming_unmanaged": len(incomingUnmanaged),
			"remove_unmanaged":   len(removableUnmanaged),
			"remove_missing":     len(removableMissing),
			"operation_count":    len(operations),
		},
	}

	counts := statusCounts{
		deployChanges:      len(deployChanges),
		importChanges:      len(importChanges),
		incomingUnmanaged:  len(incomingUnmanaged),
		removableUnmanaged: len(removableUnmanaged),
		removableMissing:   len(removableMissing),
	}

	return payload, counts, nil
}

func statusNeedsTargetWalk(syncCfg config.Sync) bool {
	return len(syncCfg.On.Deploy.RemoveUnmanaged) > 0 || len(syncCfg.On.Import.AddUnmanaged.Include) > 0
}

func buildStatusManagedOperation(phase, path, action, sourceType, targetType string) map[string]any {
	return map[string]any{
		"phase":       phase,
		"action":      statusActionLabel(action),
		"state":       "candidate",
		"path":        path,
		"source_type": sourceType,
		"target_type": targetType,
	}
}

func buildStatusTypedOperation(phase, action, path, entryType string) map[string]any {
	return map[string]any{
		"phase":  phase,
		"action": statusActionLabel(action),
		"state":  "candidate",
		"path":   path,
		"type":   entryType,
	}
}

func statusActionLabel(action string) string {
	switch action {
	case "create":
		return "can create"
	case "update":
		return "can update"
	case "replace_type":
		return "can replace type"
	case "add":
		return "can add"
	case "remove":
		return "can remove"
	default:
		return action
	}
}

func scanTargetEntries(root, scopePrefix string, sourceEntries map[string]statusEntry, includeUnmanaged bool) (map[string]statusEntry, error) {
	entries := make(map[string]statusEntry, len(sourceEntries))

	for relPath := range sourceEntries {
		entry, exists, err := probeSyncEntry(root, relPath)
		if err != nil {
			return nil, err
		}
		if exists {
			entries[relPath] = entry
		}
	}

	if !includeUnmanaged {
		return entries, nil
	}

	scannedEntries, err := scanSyncEntries(root, scopePrefix)
	if err != nil {
		return nil, err
	}
	for relPath, entry := range scannedEntries {
		entries[relPath] = entry
	}
	return entries, nil
}

func scanSyncEntries(root, scopePrefix string) (map[string]statusEntry, error) {
	entries := map[string]statusEntry{}

	info, err := os.Lstat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return entries, nil
		}
		return nil, dfmerr.Wrap(dfmerr.CodeIORead, fmt.Sprintf("Read failed: %s", root), map[string]any{"path": root}, err)
	}

	if !info.IsDir() {
		return nil, dfmerr.New(dfmerr.CodeIORead, fmt.Sprintf("Read failed: %s", root), map[string]any{"path": root})
	}

	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}

		relPath, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		relPath = filepath.ToSlash(relPath)

		if !isPathInScope(relPath, scopePrefix) {
			if d.IsDir() && !pathCanContainScope(relPath, scopePrefix) {
				return filepath.SkipDir
			}
			return nil
		}

		entryType, typeErr := entryTypeFromDirEntry(path, d)
		if typeErr != nil {
			return typeErr
		}

		entries[relPath] = statusEntry{
			path:    relPath,
			absPath: path,
			typeID:  entryType,
		}
		return nil
	})
	if walkErr != nil {
		return nil, dfmerr.Wrap(dfmerr.CodeIORead, fmt.Sprintf("Read failed: %s", root), map[string]any{"path": root}, walkErr)
	}

	return entries, nil
}

func probeSyncEntry(root, relPath string) (statusEntry, bool, error) {
	absPath := filepath.Join(root, filepath.FromSlash(relPath))
	info, err := os.Lstat(absPath)
	if err != nil {
		if pathMissing(err) {
			return statusEntry{}, false, nil
		}
		return statusEntry{}, false, dfmerr.Wrap(dfmerr.CodeIORead, fmt.Sprintf("Read failed: %s", absPath), map[string]any{"path": absPath}, err)
	}

	return statusEntry{
		path:    relPath,
		absPath: absPath,
		typeID:  entryTypeFromInfo(info),
	}, true, nil
}

func pathMissing(err error) bool {
	return os.IsNotExist(err) || errors.Is(err, syscall.ENOTDIR)
}

func isPathInScope(path, scopePrefix string) bool {
	if scopePrefix == "" {
		return true
	}
	if path == scopePrefix {
		return true
	}
	return strings.HasPrefix(path, scopePrefix+"/")
}

func pathCanContainScope(path, scopePrefix string) bool {
	if scopePrefix == "" {
		return true
	}
	if path == "" {
		return true
	}
	return strings.HasPrefix(scopePrefix, path+"/")
}

func entryTypeFromInfo(info os.FileInfo) string {
	mode := info.Mode()
	if mode&os.ModeSymlink != 0 {
		return "symlink"
	}
	if info.IsDir() {
		return "dir"
	}
	return "file"
}

func entryTypeFromDirEntry(path string, d fs.DirEntry) (string, error) {
	mode := d.Type()
	if mode&os.ModeSymlink != 0 {
		return "symlink", nil
	}
	if d.IsDir() {
		return "dir", nil
	}
	if mode.IsRegular() {
		return "file", nil
	}

	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "symlink", nil
	}
	if info.IsDir() {
		return "dir", nil
	}
	return "file", nil
}

func unionPaths(source map[string]statusEntry, target map[string]statusEntry) []string {
	set := make(map[string]struct{}, len(source)+len(target))
	for key := range source {
		set[key] = struct{}{}
	}
	for key := range target {
		set[key] = struct{}{}
	}
	paths := make([]string, 0, len(set))
	for key := range set {
		paths = append(paths, key)
	}
	sort.Strings(paths)
	return paths
}

func entriesDifferent(source, target statusEntry) (bool, error) {
	switch source.typeID {
	case "file":
		return fileContentsDifferent(source.absPath, target.absPath)
	case "symlink":
		return symlinkTargetsDifferent(source.absPath, target.absPath)
	default:
		return false, nil
	}
}

func fileContentsDifferent(sourcePath, targetPath string) (bool, error) {
	sourceContent, err := os.ReadFile(sourcePath)
	if err != nil {
		return false, dfmerr.Wrap(dfmerr.CodeIORead, fmt.Sprintf("Read failed: %s", sourcePath), map[string]any{"path": sourcePath}, err)
	}
	targetContent, err := os.ReadFile(targetPath)
	if err != nil {
		return false, dfmerr.Wrap(dfmerr.CodeIORead, fmt.Sprintf("Read failed: %s", targetPath), map[string]any{"path": targetPath}, err)
	}
	return !bytes.Equal(sourceContent, targetContent), nil
}

func symlinkTargetsDifferent(sourcePath, targetPath string) (bool, error) {
	sourceTarget, err := os.Readlink(sourcePath)
	if err != nil {
		return false, dfmerr.Wrap(dfmerr.CodeIORead, fmt.Sprintf("Read failed: %s", sourcePath), map[string]any{"path": sourcePath}, err)
	}
	targetTarget, err := os.Readlink(targetPath)
	if err != nil {
		return false, dfmerr.Wrap(dfmerr.CodeIORead, fmt.Sprintf("Read failed: %s", targetPath), map[string]any{"path": targetPath}, err)
	}
	return sourceTarget != targetTarget, nil
}

func matchesIncludeExclude(path string, include []string, includeKey string, exclude []string, excludeKey string) (bool, error) {
	if len(include) == 0 {
		return false, nil
	}
	included, err := matchesAny(path, include, includeKey)
	if err != nil {
		return false, err
	}
	if !included {
		return false, nil
	}
	excluded, err := matchesAny(path, exclude, excludeKey)
	if err != nil {
		return false, err
	}
	return !excluded, nil
}

func matchesAny(path string, patterns []string, keyPath string) (bool, error) {
	for _, pattern := range patterns {
		matched, err := doublestar.PathMatch(pattern, path)
		if err != nil {
			return false, dfmerr.New(
				dfmerr.CodeConfigSchemaType,
				fmt.Sprintf("Invalid pattern at %s: expected string glob", keyPath),
				map[string]any{"key_path": keyPath, "pattern": pattern},
			)
		}
		if matched {
			return true, nil
		}
	}
	return false, nil
}

func buildChange(path, change, sourceType, targetType string) map[string]any {
	return map[string]any{
		"path":        path,
		"change":      change,
		"source_type": sourceType,
		"target_type": targetType,
	}
}

func buildTypedPath(path, entryType string) map[string]any {
	return map[string]any{
		"path": path,
		"type": entryType,
	}
}
