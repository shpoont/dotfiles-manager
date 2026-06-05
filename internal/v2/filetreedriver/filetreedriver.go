// Package filetreedriver implements the MVP file-tree driver.
//
// Metadata policy:
//   - managed: slash-relative path identity, regular-file bytes, regular-file
//     presence, and directory presence;
//   - ignored by diff and verify: mtime, owner, group, xattrs, ACLs,
//     app-specific metadata, and mode bits except for deterministic write
//     behavior;
//   - unsupported: symlinks, hard-link identity, device files, sockets, FIFOs,
//     and other special entries;
//   - write modes: new directories are created 0755, new files are created
//     0644, and updates to an existing regular file preserve that file's
//     current permission bits while replacing bytes.
//
// Include/exclude globs are evaluated against slash-relative paths inside the
// effective resource root, where the effective resource root is the named
// location root plus Target.RelPath. The resource root itself is implicit and
// is not represented by an empty relative path. Excludes override includes; an
// excluded directory prunes its descendants. The default include is "**".
package filetreedriver

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/shpoont/dotfiles-manager/internal/v2/filedriver"
)

const NormalizerID = "file-tree.bytes.v1"

type Driver struct{}

type Target struct {
	LocationID        string
	Root              string
	RelPath           string
	Include           []string
	Exclude           []string
	AllowMissingRoot  bool
	RejectRootSymlink bool
}

type ResolvedRoot struct {
	LocationID string
	Root       string
	RelPath    string
	AbsPath    string
	Include    []string
	Exclude    []string
}

type Detection struct {
	Exists   bool
	Readable bool
	Path     string
}

type EntryKind string

const (
	EntryFile EntryKind = "file"
	EntryDir  EntryKind = "dir"
)

type Entry struct {
	Path   string
	Kind   EntryKind
	Bytes  []byte
	SHA256 string
}

type State struct {
	Exists     bool
	Entries    []Entry
	SHA256     string
	Normalizer string
}

type EntrySnapshot struct {
	Exists bool
	Kind   EntryKind
	Size   int
	SHA256 string
}

type Snapshot struct {
	Exists     bool
	EntryCount int
	FileCount  int
	DirCount   int
	SHA256     string
}

type EntryDiff struct {
	Path   string
	Kind   filedriver.ChangeKind
	Before EntrySnapshot
	After  EntrySnapshot
}

type Diff struct {
	Kind    filedriver.ChangeKind
	Before  Snapshot
	After   Snapshot
	Entries []EntryDiff
}

type Preview struct {
	Target     Target
	Path       string
	Change     Diff
	Normalizer string
}

type ApplyResult struct {
	Preview Preview
	Mutated bool
}

type VerifyResult struct {
	Verified bool
	Change   Diff
}

type BackupRequest struct {
	Target Target
	Path   string
	Before State
}

type BackupResult struct {
	ID     string
	Before Snapshot
}

type BackupHook func(BackupRequest) (BackupResult, error)

type RestoreRequest struct {
	Target Target
	Path   string
	Backup BackupResult
}

type RestoreHook func(RestoreRequest) error

func NormalizeGlobs(include []string, exclude []string) ([]string, []string, error) {
	normalizedInclude, err := normalizeGlobList("include", include)
	if err != nil {
		return nil, nil, err
	}
	if len(normalizedInclude) == 0 {
		normalizedInclude = []string{"**"}
	}
	normalizedExclude, err := normalizeGlobList("exclude", exclude)
	if err != nil {
		return nil, nil, err
	}
	return normalizedInclude, normalizedExclude, nil
}

func (d Driver) Detect(target Target) (Detection, error) {
	resolved, err := ResolveTarget(target)
	if err != nil {
		return Detection{}, err
	}
	info, err := os.Lstat(resolved.AbsPath)
	if os.IsNotExist(err) {
		return Detection{Exists: false, Readable: false, Path: resolved.AbsPath}, nil
	}
	if err != nil {
		return Detection{}, classifyOSError("detect", resolved.AbsPath, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return Detection{}, driverError(filedriver.CodeUnsafePath, "detect", resolved.AbsPath, fmt.Errorf("file-tree root must not be a symlink"))
	}
	if !info.IsDir() {
		return Detection{}, driverError(filedriver.CodeInvalidSelector, "detect", resolved.AbsPath, fmt.Errorf("file-tree root is not a directory"))
	}
	dir, err := os.Open(resolved.AbsPath)
	if err != nil {
		return Detection{}, classifyOSError("detect", resolved.AbsPath, err)
	}
	_ = dir.Close()
	return Detection{Exists: true, Readable: true, Path: resolved.AbsPath}, nil
}

func (d Driver) ReadCurrent(target Target) (State, error) {
	resolved, err := ResolveTarget(target)
	if err != nil {
		return State{}, err
	}
	info, err := os.Lstat(resolved.AbsPath)
	if os.IsNotExist(err) {
		return AbsentState(), nil
	}
	if err != nil {
		return State{}, classifyOSError("readCurrent", resolved.AbsPath, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return State{}, driverError(filedriver.CodeUnsafePath, "readCurrent", resolved.AbsPath, fmt.Errorf("file-tree root must not be a symlink"))
	}
	if !info.IsDir() {
		return State{}, driverError(filedriver.CodeInvalidSelector, "readCurrent", resolved.AbsPath, fmt.Errorf("file-tree root is not a directory"))
	}

	matcher := globMatcher{include: resolved.Include, exclude: resolved.Exclude}
	entries := make(map[string]Entry)
	requiredDirs := make(map[string]bool)
	err = filepath.WalkDir(resolved.AbsPath, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return classifyOSError("readCurrent", current, walkErr)
		}
		if current == resolved.AbsPath {
			return nil
		}
		rel, err := slashRel(resolved.AbsPath, current)
		if err != nil {
			return driverError(filedriver.CodeUnsafePath, "readCurrent", current, err)
		}
		excluded, err := matcher.excluded(rel)
		if err != nil {
			return driverError(filedriver.CodeInvalidSelector, "readCurrent", rel, err)
		}
		if excluded {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return classifyOSError("readCurrent", current, err)
		}
		mode := info.Mode()
		if mode&os.ModeSymlink != 0 {
			return driverError(filedriver.CodeUnsafePath, "readCurrent", current, fmt.Errorf("file-tree entries must not be symlinks"))
		}
		if mode.IsDir() {
			included, err := matcher.included(rel)
			if err != nil {
				return driverError(filedriver.CodeInvalidSelector, "readCurrent", rel, err)
			}
			if included {
				entries[rel] = Entry{Path: rel, Kind: EntryDir}
			}
			return nil
		}
		if !mode.IsRegular() {
			return driverError(filedriver.CodeUnsupported, "readCurrent", current, fmt.Errorf("unsupported file-tree entry type: %s", mode.Type()))
		}
		included, err := matcher.included(rel)
		if err != nil {
			return driverError(filedriver.CodeInvalidSelector, "readCurrent", rel, err)
		}
		if !included {
			return nil
		}
		if hasMultipleLinks(info) {
			return driverError(filedriver.CodeUnsupported, "readCurrent", current, fmt.Errorf("hard-linked regular files are unsupported"))
		}
		data, err := os.ReadFile(current)
		if err != nil {
			return classifyOSError("readCurrent", current, err)
		}
		entries[rel] = normalizeFileEntry(rel, data)
		markParentDirs(rel, requiredDirs)
		return nil
	})
	if err != nil {
		return State{}, err
	}
	for rel := range requiredDirs {
		if _, ok := entries[rel]; !ok {
			entries[rel] = Entry{Path: rel, Kind: EntryDir}
		}
	}
	return normalizeEntries(entries)
}

func (d Driver) Diff(current State, desired State) Diff {
	before := current.Snapshot()
	after := desired.Snapshot()
	entryDiffs := diffEntries(current, desired)
	var kind filedriver.ChangeKind
	switch {
	case !current.Exists && !desired.Exists:
		kind = filedriver.ChangeUnchanged
	case !current.Exists && desired.Exists:
		kind = filedriver.ChangeCreate
	case current.Exists && !desired.Exists:
		kind = filedriver.ChangeDelete
	case current.SHA256 == desired.SHA256 && len(entryDiffs) == 0:
		kind = filedriver.ChangeUnchanged
	default:
		kind = filedriver.ChangeUpdate
	}
	return Diff{Kind: kind, Before: before, After: after, Entries: entryDiffs}
}

func (d Driver) PreviewApply(target Target, desired State) (Preview, error) {
	resolved, err := ResolveTarget(target)
	if err != nil {
		return Preview{}, err
	}
	current, err := d.ReadCurrent(target)
	if err != nil {
		return Preview{}, err
	}
	return Preview{
		Target:     target,
		Path:       resolved.AbsPath,
		Change:     d.Diff(current, desired),
		Normalizer: NormalizerID,
	}, nil
}

func (d Driver) Backup(target Target, hook BackupHook) (BackupResult, error) {
	resolved, err := ResolveTarget(target)
	if err != nil {
		return BackupResult{}, err
	}
	before, err := d.ReadCurrent(target)
	if err != nil {
		return BackupResult{}, err
	}
	if hook == nil {
		return BackupResult{ID: "noop", Before: before.Snapshot()}, nil
	}
	result, err := hook(BackupRequest{Target: target, Path: resolved.AbsPath, Before: before})
	if err != nil {
		return BackupResult{}, fmt.Errorf("backup hook for %s: %w", resolved.AbsPath, err)
	}
	if result.Before == (Snapshot{}) {
		result.Before = before.Snapshot()
	}
	return result, nil
}

func (d Driver) Apply(target Target, desired State) (ApplyResult, error) {
	result, _, err := d.ApplyWithBackup(target, desired, nil)
	return result, err
}

func (d Driver) ApplyWithBackup(target Target, desired State, hook BackupHook) (ApplyResult, *BackupResult, error) {
	preview, err := d.PreviewApply(target, desired)
	if err != nil {
		return ApplyResult{}, nil, err
	}
	if preview.Change.Kind == filedriver.ChangeUnchanged {
		return ApplyResult{Preview: preview, Mutated: false}, nil, nil
	}
	backup, err := d.Backup(target, hook)
	if err != nil {
		return ApplyResult{}, nil, err
	}
	if err := d.applyState(target, desired); err != nil {
		return ApplyResult{}, nil, err
	}
	return ApplyResult{Preview: preview, Mutated: true}, &backup, nil
}

func (d Driver) Verify(target Target, desired State) (VerifyResult, error) {
	current, err := d.ReadCurrent(target)
	if err != nil {
		return VerifyResult{}, err
	}
	change := d.Diff(current, desired)
	if change.Kind != filedriver.ChangeUnchanged {
		resolved, resolveErr := ResolveTarget(target)
		path := ""
		if resolveErr == nil {
			path = resolved.AbsPath
		}
		return VerifyResult{Verified: false, Change: change}, driverError(filedriver.CodeVerificationFailed, "verify", path, fmt.Errorf("post-apply state differs: %s", change.Kind))
	}
	return VerifyResult{Verified: true, Change: change}, nil
}

func (d Driver) Restore(target Target, backup BackupResult, hook RestoreHook) error {
	resolved, err := ResolveTarget(target)
	if err != nil {
		return err
	}
	if hook == nil {
		return driverError(filedriver.CodeUnsupported, "restore", resolved.AbsPath, fmt.Errorf("restore requires a caller-provided restore hook in this slice"))
	}
	if err := hook(RestoreRequest{Target: target, Path: resolved.AbsPath, Backup: backup}); err != nil {
		return fmt.Errorf("restore hook for %s: %w", resolved.AbsPath, err)
	}
	return nil
}

func ResolveTarget(target Target) (ResolvedRoot, error) {
	root := strings.TrimSpace(target.Root)
	if root == "" {
		return ResolvedRoot{}, driverError(filedriver.CodeInvalidSelector, "resolve", "", fmt.Errorf("target root is required"))
	}
	rel, err := filedriver.ValidateRelativePath(target.RelPath)
	if err != nil {
		return ResolvedRoot{}, driverError(filedriver.CodeUnsafePath, "resolve", target.RelPath, err)
	}
	include, exclude, err := NormalizeGlobs(target.Include, target.Exclude)
	if err != nil {
		return ResolvedRoot{}, driverError(filedriver.CodeInvalidSelector, "resolve", rel, err)
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return ResolvedRoot{}, driverError(filedriver.CodeInvalidSelector, "resolve", root, err)
	}
	rootLstat, err := os.Lstat(rootAbs)
	if os.IsNotExist(err) {
		if !target.AllowMissingRoot {
			return ResolvedRoot{}, driverError(filedriver.CodeNotFound, "resolve", rootAbs, fmt.Errorf("location root does not exist"))
		}
		return ResolvedRoot{
			LocationID: target.LocationID,
			Root:       rootAbs,
			RelPath:    rel,
			AbsPath:    filepath.Join(rootAbs, filepath.FromSlash(rel)),
			Include:    include,
			Exclude:    exclude,
		}, nil
	}
	if err != nil {
		return ResolvedRoot{}, classifyOSError("resolve", rootAbs, err)
	}
	if target.RejectRootSymlink && rootLstat.Mode()&os.ModeSymlink != 0 {
		return ResolvedRoot{}, driverError(filedriver.CodeUnsafePath, "resolve", rootAbs, fmt.Errorf("location root must not be a symlink"))
	}
	rootInfo, err := os.Stat(rootAbs)
	if err != nil {
		return ResolvedRoot{}, classifyOSError("resolve", rootAbs, err)
	}
	if !rootInfo.IsDir() {
		return ResolvedRoot{}, driverError(filedriver.CodeInvalidSelector, "resolve", rootAbs, fmt.Errorf("location root is not a directory"))
	}
	resourceRoot := filepath.Join(rootAbs, filepath.FromSlash(rel))
	if err := ensureLexicallyInside(rootAbs, resourceRoot); err != nil {
		return ResolvedRoot{}, driverError(filedriver.CodeUnsafePath, "resolve", resourceRoot, err)
	}
	if err := ensureNoSymlinkInExistingPath(rootAbs, rel); err != nil {
		return ResolvedRoot{}, err
	}
	return ResolvedRoot{
		LocationID: target.LocationID,
		Root:       rootAbs,
		RelPath:    rel,
		AbsPath:    resourceRoot,
		Include:    include,
		Exclude:    exclude,
	}, nil
}

func AbsentState() State {
	return State{Exists: false, Normalizer: NormalizerID}
}

func (s State) Snapshot() Snapshot {
	if !s.Exists {
		return Snapshot{Exists: false}
	}
	snapshot := Snapshot{Exists: true, EntryCount: len(s.Entries), SHA256: s.SHA256}
	for _, entry := range s.Entries {
		switch entry.Kind {
		case EntryFile:
			snapshot.FileCount++
		case EntryDir:
			snapshot.DirCount++
		}
	}
	return snapshot
}

func (e Entry) Snapshot() EntrySnapshot {
	if e.Kind == "" {
		return EntrySnapshot{Exists: false}
	}
	snapshot := EntrySnapshot{Exists: true, Kind: e.Kind}
	if e.Kind == EntryFile {
		snapshot.Size = len(e.Bytes)
		snapshot.SHA256 = e.SHA256
	}
	return snapshot
}

func (d Driver) applyState(target Target, desired State) error {
	resolved, err := ResolveTarget(target)
	if err != nil {
		return err
	}
	if desired.Exists {
		if err := safeMkdirAllUnder(resolved.Root, resolved.AbsPath); err != nil {
			return err
		}
	}
	current, err := d.ReadCurrent(target)
	if err != nil {
		return err
	}
	if err := deleteRemovedEntries(resolved.AbsPath, current, desired); err != nil {
		return err
	}
	if !desired.Exists {
		return removeRootIfAbsent(resolved.AbsPath)
	}
	if err := createDesiredDirs(resolved.AbsPath, desired); err != nil {
		return err
	}
	return writeDesiredFiles(resolved.AbsPath, desired)
}

func normalizeGlobList(kind string, values []string) ([]string, error) {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return nil, fmt.Errorf("%s glob is required", kind)
		}
		if trimmed != value {
			return nil, fmt.Errorf("%s glob must not have surrounding whitespace: %s", kind, value)
		}
		if strings.ContainsRune(trimmed, '\x00') {
			return nil, fmt.Errorf("%s glob contains NUL", kind)
		}
		if strings.Contains(trimmed, "\\") {
			return nil, fmt.Errorf("%s glob must not contain backslashes: %s", kind, value)
		}
		if filepath.IsAbs(trimmed) || strings.HasPrefix(trimmed, "/") {
			return nil, fmt.Errorf("%s glob must be relative: %s", kind, value)
		}
		parts := strings.Split(trimmed, "/")
		for _, part := range parts {
			if part == "" || part == "." || part == ".." {
				return nil, fmt.Errorf("%s glob contains unsafe segment: %s", kind, value)
			}
		}
		if _, err := doublestar.PathMatch(trimmed, "sentinel"); err != nil {
			return nil, fmt.Errorf("%s glob is invalid %s: %w", kind, value, err)
		}
		normalized = append(normalized, trimmed)
	}
	return normalized, nil
}

type globMatcher struct {
	include []string
	exclude []string
}

func (m globMatcher) included(rel string) (bool, error) {
	return matchAny(m.include, rel)
}

func (m globMatcher) excluded(rel string) (bool, error) {
	return matchAny(m.exclude, rel)
}

func matchAny(patterns []string, rel string) (bool, error) {
	for _, pattern := range patterns {
		matched, err := doublestar.PathMatch(pattern, rel)
		if err != nil {
			return false, err
		}
		if matched {
			return true, nil
		}
	}
	return false, nil
}

func normalizeFileEntry(rel string, data []byte) Entry {
	copyBytes := append([]byte(nil), data...)
	sum := sha256.Sum256(copyBytes)
	return Entry{
		Path:   rel,
		Kind:   EntryFile,
		Bytes:  copyBytes,
		SHA256: hex.EncodeToString(sum[:]),
	}
}

func normalizeEntries(entries map[string]Entry) (State, error) {
	if err := rejectCaseConflicts(entries); err != nil {
		return State{}, err
	}
	paths := make([]string, 0, len(entries))
	for rel := range entries {
		paths = append(paths, rel)
	}
	sort.Strings(paths)
	normalized := make([]Entry, 0, len(paths))
	hash := sha256.New()
	for _, rel := range paths {
		entry := entries[rel]
		entry.Path = rel
		if entry.Kind == EntryFile {
			entry.Bytes = append([]byte(nil), entry.Bytes...)
		}
		normalized = append(normalized, entry)
		hash.Write([]byte(entry.Path))
		hash.Write([]byte{0})
		hash.Write([]byte(entry.Kind))
		hash.Write([]byte{0})
		hash.Write([]byte(entry.SHA256))
		hash.Write([]byte{0})
	}
	return State{
		Exists:     true,
		Entries:    normalized,
		SHA256:     hex.EncodeToString(hash.Sum(nil)),
		Normalizer: NormalizerID,
	}, nil
}

func rejectCaseConflicts(entries map[string]Entry) error {
	seen := map[string]string{}
	for rel := range entries {
		key := strings.ToLower(rel)
		if prior, ok := seen[key]; ok && prior != rel {
			return driverError(filedriver.CodeInvalidSelector, "normalize", rel, fmt.Errorf("case-conflicting paths %q and %q", prior, rel))
		}
		seen[key] = rel
	}
	return nil
}

func diffEntries(current State, desired State) []EntryDiff {
	currentMap := entryMap(current)
	desiredMap := entryMap(desired)
	all := map[string]bool{}
	for rel := range currentMap {
		all[rel] = true
	}
	for rel := range desiredMap {
		all[rel] = true
	}
	paths := make([]string, 0, len(all))
	for rel := range all {
		paths = append(paths, rel)
	}
	sort.Strings(paths)
	diffs := make([]EntryDiff, 0)
	for _, rel := range paths {
		before, beforeOK := currentMap[rel]
		after, afterOK := desiredMap[rel]
		switch {
		case !beforeOK && afterOK:
			diffs = append(diffs, EntryDiff{Path: rel, Kind: filedriver.ChangeCreate, Before: EntrySnapshot{}, After: after.Snapshot()})
		case beforeOK && !afterOK:
			diffs = append(diffs, EntryDiff{Path: rel, Kind: filedriver.ChangeDelete, Before: before.Snapshot(), After: EntrySnapshot{}})
		case beforeOK && afterOK && !sameEntry(before, after):
			diffs = append(diffs, EntryDiff{Path: rel, Kind: filedriver.ChangeUpdate, Before: before.Snapshot(), After: after.Snapshot()})
		}
	}
	return diffs
}

func sameEntry(left Entry, right Entry) bool {
	if left.Kind != right.Kind {
		return false
	}
	if left.Kind == EntryDir {
		return true
	}
	return left.SHA256 == right.SHA256 && bytes.Equal(left.Bytes, right.Bytes)
}

func entryMap(state State) map[string]Entry {
	entries := make(map[string]Entry, len(state.Entries))
	if !state.Exists {
		return entries
	}
	for _, entry := range state.Entries {
		entries[entry.Path] = entry
	}
	return entries
}

func deleteRemovedEntries(root string, current State, desired State) error {
	currentMap := entryMap(current)
	desiredMap := entryMap(desired)
	files := make([]string, 0)
	dirs := make([]string, 0)
	for rel, before := range currentMap {
		after, ok := desiredMap[rel]
		if ok && before.Kind == after.Kind {
			continue
		}
		switch before.Kind {
		case EntryFile:
			files = append(files, rel)
		case EntryDir:
			dirs = append(dirs, rel)
		}
	}
	sort.Strings(files)
	sort.Slice(dirs, func(i, j int) bool {
		return pathDepth(dirs[i]) > pathDepth(dirs[j])
	})
	for _, rel := range files {
		if err := removeFile(root, rel); err != nil {
			return err
		}
	}
	for _, rel := range dirs {
		if err := removeDir(root, rel); err != nil {
			return err
		}
	}
	return nil
}

func createDesiredDirs(root string, desired State) error {
	dirs := make([]string, 0)
	for _, entry := range desired.Entries {
		if entry.Kind == EntryDir {
			dirs = append(dirs, entry.Path)
		}
	}
	sort.Slice(dirs, func(i, j int) bool {
		return pathDepth(dirs[i]) < pathDepth(dirs[j])
	})
	for _, rel := range dirs {
		if err := safeMkdirAllUnder(root, filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			return err
		}
	}
	return nil
}

func writeDesiredFiles(root string, desired State) error {
	files := make([]Entry, 0)
	for _, entry := range desired.Entries {
		if entry.Kind == EntryFile {
			files = append(files, entry)
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	for _, entry := range files {
		if err := writeFile(root, entry.Path, entry.Bytes); err != nil {
			return err
		}
	}
	return nil
}

func removeRootIfAbsent(root string) error {
	info, err := os.Lstat(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return classifyOSError("apply", root, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return driverError(filedriver.CodeUnsafePath, "apply", root, fmt.Errorf("file-tree root must not be a symlink"))
	}
	if !info.IsDir() {
		return driverError(filedriver.CodeInvalidSelector, "apply", root, fmt.Errorf("file-tree root is not a directory"))
	}
	if err := os.Remove(root); err != nil {
		return classifyOSError("apply", root, err)
	}
	return nil
}

func removeFile(root string, rel string) error {
	path, err := pathInside(root, rel)
	if err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return classifyOSError("apply", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return driverError(filedriver.CodeUnsafePath, "apply", path, fmt.Errorf("file-tree entries must not be symlinks"))
	}
	if !info.Mode().IsRegular() {
		return driverError(filedriver.CodeInvalidSelector, "apply", path, fmt.Errorf("path is not a regular file"))
	}
	if hasMultipleLinks(info) {
		return driverError(filedriver.CodeUnsupported, "apply", path, fmt.Errorf("hard-linked regular files are unsupported"))
	}
	if err := os.Remove(path); err != nil {
		return classifyOSError("apply", path, err)
	}
	return nil
}

func removeDir(root string, rel string) error {
	path, err := pathInside(root, rel)
	if err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return classifyOSError("apply", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return driverError(filedriver.CodeUnsafePath, "apply", path, fmt.Errorf("file-tree entries must not be symlinks"))
	}
	if !info.IsDir() {
		return driverError(filedriver.CodeInvalidSelector, "apply", path, fmt.Errorf("path is not a directory"))
	}
	if err := os.Remove(path); err != nil {
		return classifyOSError("apply", path, err)
	}
	return nil
}

func writeFile(root string, rel string, data []byte) error {
	path, err := pathInside(root, rel)
	if err != nil {
		return err
	}
	if err := safeMkdirAllUnder(root, filepath.Dir(path)); err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	info, err := os.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return driverError(filedriver.CodeUnsafePath, "apply", path, fmt.Errorf("file-tree entries must not be symlinks"))
		}
		if !info.Mode().IsRegular() {
			return driverError(filedriver.CodeInvalidSelector, "apply", path, fmt.Errorf("path is not a regular file"))
		}
		if hasMultipleLinks(info) {
			return driverError(filedriver.CodeUnsupported, "apply", path, fmt.Errorf("hard-linked regular files are unsupported"))
		}
		mode = info.Mode().Perm()
	} else if err != nil && !os.IsNotExist(err) {
		return classifyOSError("apply", path, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".dfm-file-tree-*")
	if err != nil {
		return classifyOSError("apply", filepath.Dir(path), err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return classifyOSError("apply", tmpName, err)
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return classifyOSError("apply", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return classifyOSError("apply", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return classifyOSError("apply", path, err)
	}
	return nil
}

func safeMkdirAllUnder(base string, path string) error {
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return driverError(filedriver.CodeInvalidSelector, "apply", base, err)
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return driverError(filedriver.CodeInvalidSelector, "apply", path, err)
	}
	if err := ensureLexicallyInside(baseAbs, pathAbs); err != nil {
		return driverError(filedriver.CodeUnsafePath, "apply", pathAbs, err)
	}
	baseInfo, err := os.Lstat(baseAbs)
	if err != nil {
		return classifyOSError("apply", baseAbs, err)
	}
	if baseInfo.Mode()&os.ModeSymlink != 0 {
		return driverError(filedriver.CodeUnsafePath, "apply", baseAbs, fmt.Errorf("managed root must not be a symlink"))
	}
	if !baseInfo.IsDir() {
		return driverError(filedriver.CodeInvalidSelector, "apply", baseAbs, fmt.Errorf("managed root is not a directory"))
	}
	if baseAbs == pathAbs {
		return nil
	}
	rel, err := filepath.Rel(baseAbs, pathAbs)
	if err != nil {
		return driverError(filedriver.CodeUnsafePath, "apply", pathAbs, err)
	}
	current := baseAbs
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		if part == "" || part == "." || part == ".." {
			return driverError(filedriver.CodeUnsafePath, "apply", pathAbs, fmt.Errorf("path escapes managed root"))
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			if err := os.Mkdir(current, 0o755); err != nil {
				return classifyOSError("apply", current, err)
			}
			continue
		}
		if err != nil {
			return classifyOSError("apply", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return driverError(filedriver.CodeUnsafePath, "apply", current, fmt.Errorf("directory path must not contain symlinks"))
		}
		if !info.IsDir() {
			return driverError(filedriver.CodeInvalidSelector, "apply", current, fmt.Errorf("ancestor is not a directory"))
		}
	}
	return nil
}

func pathInside(root string, rel string) (string, error) {
	cleanRel, err := filedriver.ValidateRelativePath(rel)
	if err != nil {
		return "", driverError(filedriver.CodeUnsafePath, "apply", rel, err)
	}
	path := filepath.Join(root, filepath.FromSlash(cleanRel))
	if err := ensureLexicallyInside(root, path); err != nil {
		return "", driverError(filedriver.CodeUnsafePath, "apply", path, err)
	}
	return path, nil
}

func markParentDirs(rel string, dirs map[string]bool) {
	parent := pathpkg.Dir(rel)
	for parent != "." && parent != "/" {
		dirs[parent] = true
		parent = pathpkg.Dir(parent)
	}
}

func slashRel(root string, path string) (string, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", err
	}
	slashed := filepath.ToSlash(rel)
	if slashed == "." || strings.HasPrefix(slashed, "../") || slashed == ".." || filepath.IsAbs(rel) {
		return "", fmt.Errorf("path escapes root: %s", path)
	}
	return slashed, nil
}

func pathDepth(rel string) int {
	if rel == "" {
		return 0
	}
	return strings.Count(rel, "/") + 1
}

func ensureNoSymlinkInExistingPath(rootAbs string, rel string) error {
	current := rootAbs
	for _, part := range strings.Split(rel, "/") {
		current = filepath.Join(current, filepath.FromSlash(part))
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return classifyOSError("resolve", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return driverError(filedriver.CodeUnsafePath, "resolve", current, fmt.Errorf("file-tree path must not contain symlinks"))
		}
	}
	return nil
}

func hasMultipleLinks(info fs.FileInfo) bool {
	if !info.Mode().IsRegular() {
		return false
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return stat.Nlink > 1
	}
	return false
}

func ensureLexicallyInside(base string, candidate string) error {
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return err
	}
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(baseAbs, candidateAbs)
	if err != nil {
		return err
	}
	relSlash := filepath.ToSlash(rel)
	if relSlash == ".." || strings.HasPrefix(relSlash, "../") || filepath.IsAbs(rel) {
		return fmt.Errorf("path escapes root: %s", candidate)
	}
	return nil
}

func classifyOSError(op string, path string, err error) error {
	if os.IsPermission(err) {
		return driverError(filedriver.CodePermissionDenied, op, path, err)
	}
	if os.IsNotExist(err) {
		return driverError(filedriver.CodeNotFound, op, path, err)
	}
	return driverError(filedriver.CodeInternal, op, path, err)
}

func driverError(code filedriver.ErrorCode, op string, path string, err error) error {
	return &filedriver.Error{Code: code, Op: op, Path: path, Err: err}
}
