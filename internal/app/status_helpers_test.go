package app

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/shpoont/dotfiles-manager/internal/dfmerr"
	"github.com/stretchr/testify/require"
)

func TestScanSyncEntriesMissingAndNonDir(t *testing.T) {
	t.Parallel()

	missingEntries, err := scanSyncEntries(filepath.Join(t.TempDir(), "missing"), "")
	require.NoError(t, err)
	require.Empty(t, missingEntries)

	rootFile := filepath.Join(t.TempDir(), "root-file")
	require.NoError(t, os.WriteFile(rootFile, []byte("x"), 0o644))

	_, err = scanSyncEntries(rootFile, "")
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeIORead, dfmerr.MustCode(err))
}

func TestScanSyncEntriesScopeFiltering(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "lua", "sub"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "other"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "lua", "sub", "a.lua"), []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "other", "b.lua"), []byte("b"), 0o644))

	entries, err := scanSyncEntries(root, "lua")
	require.NoError(t, err)
	require.Contains(t, entries, "lua")
	require.Contains(t, entries, "lua/sub")
	require.Contains(t, entries, "lua/sub/a.lua")
	require.NotContains(t, entries, "other")
	require.NotContains(t, entries, "other/b.lua")
}

func TestScanSyncEntriesScopeFilteringKeepsScopeAncestors(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "lua", "sub"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "other"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "lua", "sub", "a.lua"), []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "other", "b.lua"), []byte("b"), 0o644))

	entries, err := scanSyncEntries(root, "lua/sub")
	require.NoError(t, err)
	require.NotContains(t, entries, "lua")
	require.Contains(t, entries, "lua/sub")
	require.Contains(t, entries, "lua/sub/a.lua")
	require.NotContains(t, entries, "other")
	require.NotContains(t, entries, "other/b.lua")
}

func TestScanTargetEntriesManifestOnlySkipsUnmanaged(t *testing.T) {
	t.Parallel()

	targetRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(targetRoot, "managed.txt"), []byte("managed"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(targetRoot, "unmanaged.txt"), []byte("unmanaged"), 0o644))

	sourceEntries := map[string]statusEntry{
		"managed.txt": {path: "managed.txt"},
	}

	targetEntries, err := scanTargetEntries(targetRoot, "", sourceEntries, nil)
	require.NoError(t, err)
	require.Contains(t, targetEntries, "managed.txt")
	require.NotContains(t, targetEntries, "unmanaged.txt")

	targetEntries, err = scanTargetEntries(targetRoot, "", sourceEntries, []string{"**"})
	require.NoError(t, err)
	require.Contains(t, targetEntries, "managed.txt")
	require.Contains(t, targetEntries, "unmanaged.txt")
}

func TestScanTargetEntriesTreatsNotDirectoryAsMissing(t *testing.T) {
	t.Parallel()

	targetRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(targetRoot, "lua"), []byte("not-a-dir"), 0o644))

	sourceEntries := map[string]statusEntry{
		"lua":          {path: "lua"},
		"lua/init.lua": {path: "lua/init.lua"},
	}

	targetEntries, err := scanTargetEntries(targetRoot, "", sourceEntries, nil)
	require.NoError(t, err)
	require.Contains(t, targetEntries, "lua")
	require.Equal(t, "file", targetEntries["lua"].typeID)
	require.NotContains(t, targetEntries, "lua/init.lua")
}

func TestScanTargetEntriesPatternPrefixSkipsUnreadableSiblings(t *testing.T) {
	t.Parallel()

	targetRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(targetRoot, "allowed"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(targetRoot, "allowed", "keep.txt"), []byte("x"), 0o644))

	blocked := filepath.Join(targetRoot, "blocked")
	require.NoError(t, os.MkdirAll(filepath.Join(blocked, "nested"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(blocked, "nested", "deny.txt"), []byte("x"), 0o644))

	require.NoError(t, os.Chmod(blocked, 0o000))
	t.Cleanup(func() { _ = os.Chmod(blocked, 0o755) })

	targetEntries, err := scanTargetEntries(targetRoot, "", map[string]statusEntry{}, []string{"allowed/**"})
	require.NoError(t, err)
	require.Contains(t, targetEntries, "allowed")
	require.Contains(t, targetEntries, "allowed/keep.txt")
	require.NotContains(t, targetEntries, "blocked")
}

func TestScanPrefixesForPatterns(t *testing.T) {
	t.Parallel()

	require.Equal(t, []string{""}, scanPrefixesForPatterns([]string{"**/*.tmp", ".codex/skills/**"}))

	prefixes := scanPrefixesForPatterns([]string{
		".codex/skills/**",
		".codex/skills/custom/**",
		".codex/prompts/**",
		"foo/*/bar/**",
		"./local/file.txt",
	})

	require.Equal(t, []string{".codex/prompts", ".codex/skills", "foo", "local/file.txt"}, prefixes)
}

func TestScopedPrefixAndOverlap(t *testing.T) {
	t.Parallel()

	require.True(t, pathScopesOverlap(".codex", ".codex/skills"))
	require.True(t, pathScopesOverlap(".codex/skills", ".codex"))
	require.False(t, pathScopesOverlap(".codex", ".config"))

	require.Equal(t, "skills", scopedPrefix(".codex/skills", ".codex"))
	require.Equal(t, "", scopedPrefix(".codex", ".codex/skills"))
	require.Equal(t, "", scopedPrefix("", ".codex"))
}

func TestScanSyncEntriesForPrefixSkipsOutsideRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	entries, err := scanSyncEntriesForPrefix(root, "", "../outside")
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestEntryTypeFromDirEntryFallbackAndError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "a.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("a"), 0o644))
	symlinkPath := filepath.Join(dir, "link")
	require.NoError(t, os.Symlink("a.txt", symlinkPath))

	fallback := fakeDirEntry{}
	typeID, err := entryTypeFromDirEntry(filePath, fallback)
	require.NoError(t, err)
	require.Equal(t, "file", typeID)

	dirEntries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var symlinkEntry fs.DirEntry
	for _, entry := range dirEntries {
		if entry.Name() == "link" {
			symlinkEntry = entry
			break
		}
	}
	require.NotNil(t, symlinkEntry)

	typeID, err = entryTypeFromDirEntry(symlinkPath, symlinkEntry)
	require.NoError(t, err)
	require.Equal(t, "symlink", typeID)

	_, err = entryTypeFromDirEntry(filepath.Join(dir, "missing"), fallback)
	require.Error(t, err)
}

func TestEntriesDifferentForFileAndSymlink(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fileA := filepath.Join(dir, "a.txt")
	fileB := filepath.Join(dir, "b.txt")
	require.NoError(t, os.WriteFile(fileA, []byte("same"), 0o644))
	require.NoError(t, os.WriteFile(fileB, []byte("same"), 0o644))

	different, err := entriesDifferent(statusEntry{absPath: fileA, typeID: "file"}, statusEntry{absPath: fileB, typeID: "file"})
	require.NoError(t, err)
	require.False(t, different)

	require.NoError(t, os.WriteFile(fileB, []byte("diff"), 0o644))
	different, err = entriesDifferent(statusEntry{absPath: fileA, typeID: "file"}, statusEntry{absPath: fileB, typeID: "file"})
	require.NoError(t, err)
	require.True(t, different)

	linkA := filepath.Join(dir, "linkA")
	linkB := filepath.Join(dir, "linkB")
	require.NoError(t, os.Symlink("a.txt", linkA))
	require.NoError(t, os.Symlink("a.txt", linkB))
	different, err = entriesDifferent(statusEntry{absPath: linkA, typeID: "symlink"}, statusEntry{absPath: linkB, typeID: "symlink"})
	require.NoError(t, err)
	require.False(t, different)

	require.NoError(t, os.Remove(linkB))
	require.NoError(t, os.Symlink("b.txt", linkB))
	different, err = entriesDifferent(statusEntry{absPath: linkA, typeID: "symlink"}, statusEntry{absPath: linkB, typeID: "symlink"})
	require.NoError(t, err)
	require.True(t, different)

	different, err = entriesDifferent(statusEntry{typeID: "dir"}, statusEntry{typeID: "dir"})
	require.NoError(t, err)
	require.False(t, different)

	_, err = entriesDifferent(statusEntry{absPath: filepath.Join(dir, "missing"), typeID: "file"}, statusEntry{absPath: fileB, typeID: "file"})
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeIORead, dfmerr.MustCode(err))
}

func TestMatchesIncludeExcludeAndInvalidPattern(t *testing.T) {
	t.Parallel()

	ok, err := matchesIncludeExclude("lua/a.lua", nil, "include", nil, "exclude")
	require.NoError(t, err)
	require.False(t, ok)

	ok, err = matchesIncludeExclude("lua/a.lua", []string{"lua/**"}, "include", []string{"**/*.tmp"}, "exclude")
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = matchesIncludeExclude("lua/a.tmp", []string{"lua/**"}, "include", []string{"**/*.tmp"}, "exclude")
	require.NoError(t, err)
	require.False(t, ok)

	_, err = matchesAny("lua/a.lua", []string{"["}, "syncs[0].on.import.add-unmanaged.include")
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeConfigSchemaType, dfmerr.MustCode(err))
}

type fakeDirEntry struct{}

func (fakeDirEntry) Name() string               { return "fake" }
func (fakeDirEntry) IsDir() bool                { return false }
func (fakeDirEntry) Type() fs.FileMode          { return fs.ModeDevice }
func (fakeDirEntry) Info() (fs.FileInfo, error) { return nil, nil }
