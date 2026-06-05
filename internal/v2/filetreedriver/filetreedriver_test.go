package filetreedriver

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"testing"
	"time"

	"github.com/shpoont/dotfiles-manager/internal/v2/filedriver"
	"github.com/stretchr/testify/require"
)

func TestReadCurrentAppliesIncludeExcludeGlobsAndDirectoryPolicy(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	tree := filepath.Join(root, "profiles")
	writeTestFile(t, filepath.Join(tree, "init.lua"), "init")
	writeTestFile(t, filepath.Join(tree, "lua", "module.lua"), "module")
	writeTestFile(t, filepath.Join(tree, "lua", "cache", "ignored.lua"), "ignored")
	writeTestFile(t, filepath.Join(tree, "notes.txt"), "notes")
	require.NoError(t, os.MkdirAll(filepath.Join(tree, "empty-dir"), 0o755))

	target := Target{
		LocationID: "config",
		Root:       root,
		RelPath:    "profiles",
		Include:    []string{"**/*.lua", "empty-dir"},
		Exclude:    []string{"lua/cache/**"},
	}

	detection, err := Driver{}.Detect(target)
	require.NoError(t, err)
	require.True(t, detection.Exists)
	require.True(t, detection.Readable)

	state, err := Driver{}.ReadCurrent(target)
	require.NoError(t, err)
	require.True(t, state.Exists)
	require.Equal(t, []string{"empty-dir", "init.lua", "lua", "lua/module.lua"}, entryPaths(state))
	require.Equal(t, EntryDir, entryByPath(t, state, "empty-dir").Kind)
	require.Equal(t, EntryDir, entryByPath(t, state, "lua").Kind)
	require.Equal(t, EntryFile, entryByPath(t, state, "lua/module.lua").Kind)
	require.NotEmpty(t, state.SHA256)
}

func TestPreviewApplyCreateUpdateDeleteWithoutDryRunMutation(t *testing.T) {
	t.Parallel()

	driver := Driver{}
	liveRoot := t.TempDir()
	desiredRoot := t.TempDir()
	writeTestFile(t, filepath.Join(desiredRoot, "profiles", "config.yaml"), "one\n")
	require.NoError(t, os.MkdirAll(filepath.Join(desiredRoot, "profiles", "empty"), 0o755))

	liveTarget := Target{LocationID: "config", Root: liveRoot, RelPath: "profiles"}
	desiredTarget := Target{LocationID: "desired", Root: desiredRoot, RelPath: "profiles"}
	desired, err := driver.ReadCurrent(desiredTarget)
	require.NoError(t, err)

	preview, err := driver.PreviewApply(liveTarget, desired)
	require.NoError(t, err)
	require.Equal(t, filedriver.ChangeCreate, preview.Change.Kind)
	require.Len(t, preview.Change.Entries, 2)
	assertMissing(t, filepath.Join(liveRoot, "profiles"))

	applied, err := driver.Apply(liveTarget, desired)
	require.NoError(t, err)
	require.True(t, applied.Mutated)
	requireFile(t, filepath.Join(liveRoot, "profiles", "config.yaml"), "one\n")
	requireMode(t, filepath.Join(liveRoot, "profiles", "config.yaml"), 0o644)
	requireMode(t, filepath.Join(liveRoot, "profiles", "empty"), 0o755)

	require.NoError(t, os.Chmod(filepath.Join(liveRoot, "profiles", "config.yaml"), 0o755))
	writeTestFile(t, filepath.Join(desiredRoot, "profiles", "config.yaml"), "two\n")
	desired, err = driver.ReadCurrent(desiredTarget)
	require.NoError(t, err)
	preview, err = driver.PreviewApply(liveTarget, desired)
	require.NoError(t, err)
	require.Equal(t, filedriver.ChangeUpdate, preview.Change.Kind)
	requireFile(t, filepath.Join(liveRoot, "profiles", "config.yaml"), "one\n")

	applied, err = driver.Apply(liveTarget, desired)
	require.NoError(t, err)
	require.True(t, applied.Mutated)
	requireFile(t, filepath.Join(liveRoot, "profiles", "config.yaml"), "two\n")
	requireMode(t, filepath.Join(liveRoot, "profiles", "config.yaml"), 0o755)

	writeTestFile(t, filepath.Join(liveRoot, "profiles", "remove.txt"), "remove\n")
	writeTestFile(t, filepath.Join(liveRoot, "profiles", "old-dir", "old.txt"), "old\n")
	writeTestFile(t, filepath.Join(desiredRoot, "profiles", "new-dir", "child", "new.txt"), "new\n")
	desired, err = driver.ReadCurrent(desiredTarget)
	require.NoError(t, err)
	preview, err = driver.PreviewApply(liveTarget, desired)
	require.NoError(t, err)
	require.Equal(t, filedriver.ChangeUpdate, preview.Change.Kind)
	require.Contains(t, diffPaths(preview.Change.Entries), "remove.txt")
	require.Contains(t, diffPaths(preview.Change.Entries), "old-dir/old.txt")
	require.Contains(t, diffPaths(preview.Change.Entries), "new-dir/child/new.txt")

	applied, err = driver.Apply(liveTarget, desired)
	require.NoError(t, err)
	require.True(t, applied.Mutated)
	assertMissing(t, filepath.Join(liveRoot, "profiles", "remove.txt"))
	assertMissing(t, filepath.Join(liveRoot, "profiles", "old-dir"))
	requireFile(t, filepath.Join(liveRoot, "profiles", "new-dir", "child", "new.txt"), "new\n")

	preview, err = driver.PreviewApply(liveTarget, AbsentState())
	require.NoError(t, err)
	require.Equal(t, filedriver.ChangeDelete, preview.Change.Kind)
	requireFile(t, filepath.Join(liveRoot, "profiles", "config.yaml"), "two\n")

	applied, err = driver.Apply(liveTarget, AbsentState())
	require.NoError(t, err)
	require.True(t, applied.Mutated)
	verify, err := driver.Verify(liveTarget, AbsentState())
	require.NoError(t, err)
	require.True(t, verify.Verified)
	assertMissing(t, filepath.Join(liveRoot, "profiles"))
}

func TestBackupAndRestoreHooksAreExplicit(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "profiles", "config.yaml"), "before\n")
	target := Target{LocationID: "config", Root: root, RelPath: "profiles"}
	driver := Driver{}

	called := false
	backup, err := driver.Backup(target, func(req BackupRequest) (BackupResult, error) {
		called = true
		require.True(t, req.Before.Exists)
		require.Equal(t, []string{"config.yaml"}, entryPaths(req.Before))
		return BackupResult{ID: "memory://backup/tree", Before: req.Before.Snapshot()}, nil
	})
	require.NoError(t, err)
	require.True(t, called)
	require.Equal(t, "memory://backup/tree", backup.ID)

	err = driver.Restore(target, backup, nil)
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsupported))

	restoreCalled := false
	err = driver.Restore(target, backup, func(req RestoreRequest) error {
		restoreCalled = true
		require.Equal(t, backup.ID, req.Backup.ID)
		return nil
	})
	require.NoError(t, err)
	require.True(t, restoreCalled)
}

func TestRejectsUnsafePathsSymlinksSpecialEntriesAndHardLinks(t *testing.T) {
	t.Parallel()

	driver := Driver{}
	root := t.TempDir()

	for _, rel := range []string{"../escape", "/tmp/escape", `nested\\escape`, "nested/./escape", "nested//escape"} {
		t.Run(rel, func(t *testing.T) {
			t.Parallel()
			_, err := driver.ReadCurrent(Target{LocationID: "config", Root: root, RelPath: rel})
			require.Error(t, err)
			require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())
		})
	}

	require.NoError(t, os.MkdirAll(filepath.Join(root, "profiles"), 0o755))
	require.NoError(t, os.Symlink("missing", filepath.Join(root, "profiles", "link")))
	_, err := driver.ReadCurrent(Target{LocationID: "config", Root: root, RelPath: "profiles"})
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())

	hardRoot := t.TempDir()
	writeTestFile(t, filepath.Join(hardRoot, "profiles", "config.yaml"), "hard\n")
	err = os.Link(filepath.Join(hardRoot, "profiles", "config.yaml"), filepath.Join(hardRoot, "profiles", "other.yaml"))
	if err != nil {
		t.Skipf("hard links are unsupported on this filesystem: %v", err)
	}
	_, err = driver.ReadCurrent(Target{LocationID: "config", Root: hardRoot, RelPath: "profiles"})
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsupported), err.Error())
}

func TestCaseConflictPolicyRejectsSelectedEntries(t *testing.T) {
	t.Parallel()

	_, err := normalizeEntries(map[string]Entry{
		"Config.yaml": normalizeFileEntry("Config.yaml", []byte("one")),
		"config.yaml": normalizeFileEntry("config.yaml", []byte("two")),
	})
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
	require.Contains(t, err.Error(), "case-conflicting")
}

func TestValidationAndErrorBranches(t *testing.T) {
	t.Parallel()

	include, exclude, err := NormalizeGlobs(nil, []string{"cache/**"})
	require.NoError(t, err)
	require.Equal(t, []string{"**"}, include)
	require.Equal(t, []string{"cache/**"}, exclude)

	for _, glob := range []string{"", " ../escape", "../escape", "/escape", `nested\\escape`, "nested//escape", "["} {
		t.Run(glob, func(t *testing.T) {
			t.Parallel()
			_, _, err := NormalizeGlobs([]string{glob}, nil)
			require.Error(t, err)
		})
	}
	_, _, err = NormalizeGlobs(nil, []string{"["})
	require.Error(t, err)

	root := t.TempDir()
	file := filepath.Join(root, "not-dir")
	writeTestFile(t, file, "x")
	_, err = Driver{}.Detect(Target{LocationID: "config", Root: file, RelPath: "profiles"})
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())

	resourceFileRoot := t.TempDir()
	writeTestFile(t, filepath.Join(resourceFileRoot, "profiles"), "not a dir")
	_, err = Driver{}.Detect(Target{LocationID: "config", Root: resourceFileRoot, RelPath: "profiles"})
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())

	_, err = Driver{}.Detect(Target{LocationID: "config", Root: filepath.Join(root, "missing"), RelPath: "profiles"})
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeNotFound), err.Error())

	_, err = ResolveTarget(Target{LocationID: "config", Root: root, RelPath: "profiles", Include: []string{"["}})
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())

	missingRoot := filepath.Join(root, "missing-root")
	resolved, err := ResolveTarget(Target{LocationID: "desired", Root: missingRoot, RelPath: "profiles", AllowMissingRoot: true})
	require.NoError(t, err)
	require.Equal(t, filepath.Join(missingRoot, "profiles"), resolved.AbsPath)

	err = Driver{}.Restore(Target{LocationID: "config", Root: root, RelPath: "profiles"}, BackupResult{ID: "backup"}, func(req RestoreRequest) error {
		return fmt.Errorf("restore failed")
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "restore failed")
}

func TestAdditionalSafetyAndErrorBranches(t *testing.T) {
	t.Parallel()

	driver := Driver{}
	root := t.TempDir()
	target := Target{LocationID: "config", Root: root, RelPath: "profiles"}

	detection, err := driver.Detect(target)
	require.NoError(t, err)
	require.False(t, detection.Exists)

	writeTestFile(t, filepath.Join(root, "profiles", "nested", "config.yaml"), "same\n")
	state, err := driver.ReadCurrent(target)
	require.NoError(t, err)

	backup, err := driver.Backup(target, nil)
	require.NoError(t, err)
	require.Equal(t, "noop", backup.ID)
	require.True(t, backup.Before.Exists)

	backup, err = driver.Backup(target, func(req BackupRequest) (BackupResult, error) {
		return BackupResult{ID: "memory://default-before"}, nil
	})
	require.NoError(t, err)
	require.Equal(t, "memory://default-before", backup.ID)
	require.True(t, backup.Before.Exists)

	applied, backupResult, err := driver.ApplyWithBackup(target, state, func(req BackupRequest) (BackupResult, error) {
		t.Fatalf("unchanged apply must not call backup")
		return BackupResult{}, nil
	})
	require.NoError(t, err)
	require.False(t, applied.Mutated)
	require.Nil(t, backupResult)

	verify, err := driver.Verify(target, AbsentState())
	require.Error(t, err)
	require.False(t, verify.Verified)
	require.True(t, filedriver.IsCode(err, filedriver.CodeVerificationFailed), err.Error())

	_, err = driver.PreviewApply(Target{LocationID: "config", Root: root, RelPath: "../bad"}, AbsentState())
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())

	badDesired := State{Exists: true, Entries: []Entry{{Path: "../bad", Kind: EntryFile, Bytes: []byte("bad")}}, Normalizer: NormalizerID}
	_, _, err = driver.ApplyWithBackup(target, badDesired, nil)
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())

	rootLink := filepath.Join(t.TempDir(), "root-link")
	require.NoError(t, os.Symlink(root, rootLink))
	_, err = ResolveTarget(Target{LocationID: "config", Root: rootLink, RelPath: "profiles", RejectRootSymlink: true})
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())

	require.NoError(t, os.Symlink("profiles", filepath.Join(root, "profiles-link")))
	_, err = ResolveTarget(Target{LocationID: "config", Root: root, RelPath: "profiles-link/config"})
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())

	require.Equal(t, 3, pathDepth("a/b/c"))
	require.Equal(t, 0, pathDepth(""))
	require.False(t, sameEntry(Entry{Kind: EntryDir}, Entry{Kind: EntryFile}))
	require.False(t, Entry{}.Snapshot().Exists)
	_, err = slashRel(root, filepath.Dir(root))
	require.Error(t, err)
	require.True(t, filedriver.IsCode(classifyOSError("op", "path", os.ErrPermission), filedriver.CodePermissionDenied))
	require.True(t, filedriver.IsCode(classifyOSError("op", "path", os.ErrNotExist), filedriver.CodeNotFound))
	require.True(t, filedriver.IsCode(classifyOSError("op", "path", fmt.Errorf("boom")), filedriver.CodeInternal))
}

func TestInternalApplyHelpersRejectUnsafeRuntimePaths(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	require.NoError(t, safeMkdirAllUnder(root, filepath.Join(root, "a", "b", "c")))
	require.DirExists(t, filepath.Join(root, "a", "b", "c"))

	require.NoError(t, os.Symlink(filepath.Join(root, "a"), filepath.Join(root, "link-dir")))
	err := safeMkdirAllUnder(root, filepath.Join(root, "link-dir", "child"))
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())

	writeTestFile(t, filepath.Join(root, "file-parent"), "x")
	err = safeMkdirAllUnder(root, filepath.Join(root, "file-parent", "child"))
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())

	err = safeMkdirAllUnder(root, filepath.Dir(root))
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())

	err = writeFile(root, "link-dir/file.txt", []byte("x"))
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())

	require.NoError(t, os.MkdirAll(filepath.Join(root, "dir-as-file"), 0o755))
	err = writeFile(root, "dir-as-file", []byte("x"))
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())

	err = removeFile(root, "missing.txt")
	require.NoError(t, err)
	err = removeFile(root, "dir-as-file")
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
	require.NoError(t, os.Symlink("missing", filepath.Join(root, "file-link")))
	err = removeFile(root, "file-link")
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())
	writeTestFile(t, filepath.Join(root, "hard-a"), "hard")
	err = os.Link(filepath.Join(root, "hard-a"), filepath.Join(root, "hard-b"))
	if err == nil {
		err = removeFile(root, "hard-a")
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeUnsupported), err.Error())
	}

	err = removeDir(root, "missing-dir")
	require.NoError(t, err)
	writeTestFile(t, filepath.Join(root, "file-as-dir"), "x")
	err = removeDir(root, "file-as-dir")
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
	require.NoError(t, os.Symlink("missing", filepath.Join(root, "dir-link")))
	err = removeDir(root, "dir-link")
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())

	err = removeRootIfAbsent(filepath.Join(root, "missing-root"))
	require.NoError(t, err)
	writeTestFile(t, filepath.Join(root, "root-file"), "x")
	err = removeRootIfAbsent(filepath.Join(root, "root-file"))
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
	require.NoError(t, os.Symlink("missing", filepath.Join(root, "root-link")))
	err = removeRootIfAbsent(filepath.Join(root, "root-link"))
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())
}

func TestMoreDriverErrorBranches(t *testing.T) {
	t.Parallel()

	driver := Driver{}
	root := t.TempDir()
	target := Target{LocationID: "config", Root: root, RelPath: "profiles"}
	writeTestFile(t, filepath.Join(root, "profiles", "config.yaml"), "before\n")
	desired := State{Exists: true, Entries: []Entry{normalizeFileEntry("config.yaml", []byte("after\n"))}, Normalizer: NormalizerID}
	desired, err := normalizeEntries(entryMap(desired))
	require.NoError(t, err)

	_, err = driver.Backup(Target{LocationID: "config", Root: root, RelPath: "../bad"}, nil)
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())

	_, err = driver.Backup(target, func(req BackupRequest) (BackupResult, error) {
		return BackupResult{}, fmt.Errorf("backup failed")
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "backup failed")

	_, _, err = driver.ApplyWithBackup(target, desired, func(req BackupRequest) (BackupResult, error) {
		return BackupResult{}, fmt.Errorf("backup failed")
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "backup failed")
	requireFile(t, filepath.Join(root, "profiles", "config.yaml"), "before\n")

	fileRoot := t.TempDir()
	writeTestFile(t, filepath.Join(fileRoot, "profiles"), "not a dir")
	_, err = driver.ReadCurrent(Target{LocationID: "config", Root: fileRoot, RelPath: "profiles"})
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())

	fifoRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(fifoRoot, "profiles"), 0o755))
	fifo := filepath.Join(fifoRoot, "profiles", "fifo")
	if err := syscall.Mkfifo(fifo, 0o644); err != nil {
		t.Skipf("fifo fixture unsupported on this filesystem: %v", err)
	}
	_, err = driver.ReadCurrent(Target{LocationID: "config", Root: fifoRoot, RelPath: "profiles"})
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsupported), err.Error())
}

func TestMoreMkdirAndPathHelperBranches(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	missingBase := filepath.Join(root, "missing-base")
	err := safeMkdirAllUnder(missingBase, filepath.Join(missingBase, "child"))
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeNotFound), err.Error())

	baseLink := filepath.Join(root, "base-link")
	require.NoError(t, os.Symlink(root, baseLink))
	err = safeMkdirAllUnder(baseLink, filepath.Join(baseLink, "child"))
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())

	baseFile := filepath.Join(root, "base-file")
	writeTestFile(t, baseFile, "x")
	err = safeMkdirAllUnder(baseFile, filepath.Join(baseFile, "child"))
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())

	_, err = pathInside(root, "../bad")
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())

	_, err = pathInside(root, "ok/file.txt")
	require.NoError(t, err)

	require.False(t, hasMultipleLinks(fakeFileInfo{mode: os.ModeDir}))
}

func TestApplyStateErrorBranches(t *testing.T) {
	t.Parallel()

	driver := Driver{}
	root := t.TempDir()
	desiredEmpty, err := normalizeEntries(map[string]Entry{"dir": {Path: "dir", Kind: EntryDir}})
	require.NoError(t, err)

	err = driver.applyState(Target{LocationID: "config", Root: "", RelPath: "profiles"}, desiredEmpty)
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())

	missingRoot := filepath.Join(root, "missing-root")
	err = driver.applyState(Target{LocationID: "desired", Root: missingRoot, RelPath: "profiles", AllowMissingRoot: true}, desiredEmpty)
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeNotFound), err.Error())

	writeTestFile(t, filepath.Join(root, "profiles", "existing.txt"), "existing\n")
	badDirDesired := State{Exists: true, Entries: []Entry{{Path: "../bad-dir", Kind: EntryDir}}, Normalizer: NormalizerID}
	err = driver.applyState(Target{LocationID: "config", Root: root, RelPath: "profiles"}, badDirDesired)
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())

	writeTestFile(t, filepath.Join(root, "profiles", "hard-a"), "hard")
	err = os.Link(filepath.Join(root, "profiles", "hard-a"), filepath.Join(root, "profiles", "hard-b"))
	if err == nil {
		err = writeFile(filepath.Join(root, "profiles"), "hard-a", []byte("new"))
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeUnsupported), err.Error())
	}
}

type fakeFileInfo struct {
	mode os.FileMode
}

func (f fakeFileInfo) Name() string       { return "fake" }
func (f fakeFileInfo) Size() int64        { return 0 }
func (f fakeFileInfo) Mode() os.FileMode  { return f.mode }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return f.mode.IsDir() }
func (f fakeFileInfo) Sys() any           { return nil }

func entryPaths(state State) []string {
	paths := make([]string, 0, len(state.Entries))
	for _, entry := range state.Entries {
		paths = append(paths, entry.Path)
	}
	sort.Strings(paths)
	return paths
}

func entryByPath(t *testing.T, state State, rel string) Entry {
	t.Helper()
	for _, entry := range state.Entries {
		if entry.Path == rel {
			return entry
		}
	}
	t.Fatalf("missing entry %s in %#v", rel, state.Entries)
	return Entry{}
}

func diffPaths(diffs []EntryDiff) []string {
	paths := make([]string, 0, len(diffs))
	for _, diff := range diffs {
		paths = append(paths, diff.Path)
	}
	sort.Strings(paths)
	return paths
}

func writeTestFile(t *testing.T, path string, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func requireFile(t *testing.T, path string, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, want, string(got))
}

func requireMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, want, info.Mode().Perm())
}

func assertMissing(t *testing.T, path string) {
	t.Helper()
	_, err := os.Stat(path)
	require.True(t, os.IsNotExist(err), "expected %s to be missing, got %v", path, err)
}
