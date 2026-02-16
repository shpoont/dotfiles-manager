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
