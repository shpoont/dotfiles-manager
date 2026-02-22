package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shpoont/dotfiles-manager/internal/dfmerr"
	"github.com/stretchr/testify/require"
)

func TestBuildDiffMetadataKinds(t *testing.T) {
	t.Parallel()

	metadata, err := buildDiffMetadata("deploy", "lua", &statusEntry{typeID: "dir"}, nil, 3, false, 4)
	require.NoError(t, err)
	require.Equal(t, "omitted", metadata["diff_kind"])
	require.Equal(t, "directory diff omitted", metadata["reason"])
	require.Equal(t, 4, metadata["omitted_entry_count"])
	require.Equal(t, "scope diff to this directory path for file-level changes", metadata["inspect_hint"])
	require.Equal(t, false, metadata["patch_available"])
	require.Equal(t, false, metadata["patch_included"])

	metadata, err = buildDiffMetadata("deploy", "lua/init.lua", &statusEntry{typeID: "file"}, &statusEntry{typeID: "symlink"}, 3, false, 0)
	require.NoError(t, err)
	require.Equal(t, "type_change", metadata["diff_kind"])
	require.Equal(t, "type differs", metadata["reason"])
	require.Equal(t, false, metadata["patch_available"])
	require.Equal(t, false, metadata["patch_included"])

	tempDir := t.TempDir()
	sourceBinary := filepath.Join(tempDir, "source.bin")
	targetBinary := filepath.Join(tempDir, "target.bin")
	require.NoError(t, os.WriteFile(sourceBinary, []byte{0x00, 0x01, 0x02}, 0o644))
	require.NoError(t, os.WriteFile(targetBinary, []byte{0x00, 0x05, 0x06}, 0o644))

	metadata, err = buildDiffMetadata(
		"deploy",
		"cache.bin",
		&statusEntry{typeID: "file", absPath: sourceBinary},
		&statusEntry{typeID: "file", absPath: targetBinary},
		3,
		false,
		0,
	)
	require.NoError(t, err)
	require.Equal(t, "binary", metadata["diff_kind"])
	require.Equal(t, "binary differs", metadata["reason"])
	require.Equal(t, false, metadata["patch_available"])
	require.Equal(t, false, metadata["patch_included"])
}

func TestBuildDiffMetadataPatchModes(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "source.txt")
	targetPath := filepath.Join(tempDir, "target.txt")
	require.NoError(t, os.WriteFile(sourcePath, []byte("alpha\nbeta\n"), 0o644))
	require.NoError(t, os.WriteFile(targetPath, []byte("alpha\ngamma\n"), 0o644))

	sourceEntry := &statusEntry{typeID: "file", absPath: sourcePath}
	targetEntry := &statusEntry{typeID: "file", absPath: targetPath}

	metadataNoPatch, err := buildDiffMetadata("deploy", "notes.txt", sourceEntry, targetEntry, 3, false, 0)
	require.NoError(t, err)
	require.Equal(t, "unified", metadataNoPatch["diff_kind"])
	require.Equal(t, true, metadataNoPatch["patch_available"])
	require.Equal(t, false, metadataNoPatch["patch_included"])
	_, hasPatch := metadataNoPatch["patch"]
	require.False(t, hasPatch)

	metadataWithPatch, err := buildDiffMetadata("deploy", "notes.txt", sourceEntry, targetEntry, 3, true, 0)
	require.NoError(t, err)
	require.Equal(t, "unified", metadataWithPatch["diff_kind"])
	require.Equal(t, true, metadataWithPatch["patch_available"])
	require.Equal(t, true, metadataWithPatch["patch_included"])
	patch, ok := metadataWithPatch["patch"].(string)
	require.True(t, ok)
	require.Contains(t, patch, "--- target/notes.txt")
	require.Contains(t, patch, "+++ source/notes.txt")
}

func TestBuildDiffMetadataOmittedWhenPatchTooLarge(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "source-large.txt")
	targetPath := filepath.Join(tempDir, "target-large.txt")
	require.NoError(t, os.WriteFile(sourcePath, []byte(strings.Repeat("a", diffPatchSizeLimitBytes)+"\n"), 0o644))
	require.NoError(t, os.WriteFile(targetPath, []byte(strings.Repeat("b", diffPatchSizeLimitBytes)+"\n"), 0o644))

	metadata, err := buildDiffMetadata(
		"deploy",
		"large.txt",
		&statusEntry{typeID: "file", absPath: sourcePath},
		&statusEntry{typeID: "file", absPath: targetPath},
		0,
		true,
		0,
	)
	require.NoError(t, err)
	require.Equal(t, "omitted", metadata["diff_kind"])
	require.Contains(t, metadata["reason"], "exceeds")
	require.Equal(t, false, metadata["patch_available"])
	require.Equal(t, false, metadata["patch_included"])
	_, hasPatch := metadata["patch"]
	require.False(t, hasPatch)
}

func TestDiffEntryContentAndBinaryHelpers(t *testing.T) {
	t.Parallel()

	content, binary, err := diffEntryContent(nil)
	require.NoError(t, err)
	require.Empty(t, content)
	require.False(t, binary)

	content, binary, err = diffEntryContent(&statusEntry{typeID: "dir"})
	require.NoError(t, err)
	require.Empty(t, content)
	require.False(t, binary)

	tempDir := t.TempDir()
	_, _, err = diffEntryContent(&statusEntry{typeID: "file", absPath: filepath.Join(tempDir, "missing.txt")})
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeIORead, dfmerr.MustCode(err))

	targetPath := filepath.Join(tempDir, "target.txt")
	require.NoError(t, os.WriteFile(targetPath, []byte("payload\n"), 0o644))
	linkPath := filepath.Join(tempDir, "link.txt")
	require.NoError(t, os.Symlink("target.txt", linkPath))

	content, binary, err = diffEntryContent(&statusEntry{typeID: "symlink", absPath: linkPath})
	require.NoError(t, err)
	require.Equal(t, []byte("symlink -> target.txt\n"), content)
	require.False(t, binary)

	_, _, err = diffEntryContent(&statusEntry{typeID: "symlink", absPath: filepath.Join(tempDir, "missing-link.txt")})
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeIORead, dfmerr.MustCode(err))

	require.False(t, isBinaryContent(nil))
	require.False(t, isBinaryContent([]byte("hello\n")))
	require.True(t, isBinaryContent([]byte{0x00, 0x01}))
	require.True(t, isBinaryContent([]byte{0xff, 0xfe}))
}

func TestDiffPerspectivePatchAndCounters(t *testing.T) {
	t.Parallel()

	sourceEntry := &statusEntry{typeID: "file"}
	targetEntry := &statusEntry{typeID: "file"}

	oldEntry, newEntry, oldLabel, newLabel := diffPerspective("deploy", "lua/init.lua", sourceEntry, targetEntry)
	require.Equal(t, targetEntry, oldEntry)
	require.Equal(t, sourceEntry, newEntry)
	require.Equal(t, "target/lua/init.lua", oldLabel)
	require.Equal(t, "source/lua/init.lua", newLabel)

	oldEntry, newEntry, oldLabel, newLabel = diffPerspective("import", "lua/init.lua", sourceEntry, targetEntry)
	require.Equal(t, sourceEntry, oldEntry)
	require.Equal(t, targetEntry, newEntry)
	require.Equal(t, "source/lua/init.lua", oldLabel)
	require.Equal(t, "target/lua/init.lua", newLabel)

	oldEntry, newEntry, oldLabel, newLabel = diffPerspective("incoming_unmanaged", "lua/new.lua", sourceEntry, targetEntry)
	require.Nil(t, oldEntry)
	require.Equal(t, targetEntry, newEntry)
	require.Equal(t, "/dev/null", oldLabel)
	require.Equal(t, "target/lua/new.lua", newLabel)

	oldEntry, newEntry, oldLabel, newLabel = diffPerspective("remove_unmanaged", "lua/new.lua", sourceEntry, targetEntry)
	require.Equal(t, targetEntry, oldEntry)
	require.Nil(t, newEntry)
	require.Equal(t, "target/lua/new.lua", oldLabel)
	require.Equal(t, "/dev/null", newLabel)

	oldEntry, newEntry, oldLabel, newLabel = diffPerspective("remove_missing", "lua/new.lua", sourceEntry, targetEntry)
	require.Equal(t, sourceEntry, oldEntry)
	require.Nil(t, newEntry)
	require.Equal(t, "source/lua/new.lua", oldLabel)
	require.Equal(t, "/dev/null", newLabel)

	oldEntry, newEntry, oldLabel, newLabel = diffPerspective("unknown", "lua/new.lua", sourceEntry, targetEntry)
	require.Nil(t, oldEntry)
	require.Nil(t, newEntry)
	require.Equal(t, "/dev/null", oldLabel)
	require.Equal(t, "/dev/null", newLabel)

	require.Equal(t, "/dev/null", diffLabel("target", "lua/init.lua", false))

	patch, err := unifiedPatch("old/file", "new/file", []byte("alpha\n"), []byte("beta\n"), 1)
	require.NoError(t, err)
	require.Contains(t, patch, "--- old/file")
	require.Contains(t, patch, "+++ new/file")

	var counts diffCounts
	incrementDiffKindCount(&counts, "unified")
	incrementDiffKindCount(&counts, "binary")
	incrementDiffKindCount(&counts, "type_change")
	incrementDiffKindCount(&counts, "unknown")
	require.Equal(t, 1, counts.unifiedPatch)
	require.Equal(t, 1, counts.binary)
	require.Equal(t, 1, counts.typeChange)
	require.Equal(t, 1, counts.omitted)
}

func TestOmittedEntryCountForPathUsesExistingScannedMaps(t *testing.T) {
	t.Parallel()

	sourceEntries := map[string]statusEntry{
		"lua":               {path: "lua", typeID: "dir"},
		"lua/init.lua":      {path: "lua/init.lua", typeID: "file"},
		"lua/plugins":       {path: "lua/plugins", typeID: "dir"},
		"lua/plugins/a.lua": {path: "lua/plugins/a.lua", typeID: "file"},
	}
	targetEntries := map[string]statusEntry{
		"lua":               {path: "lua", typeID: "dir"},
		"lua/plugins":       {path: "lua/plugins", typeID: "dir"},
		"lua/plugins/b.lua": {path: "lua/plugins/b.lua", typeID: "file"},
	}

	require.Equal(t, 4, omittedEntryCountForPath("lua", sourceEntries, targetEntries))
	require.Equal(t, 2, omittedEntryCountForPath("lua/plugins", sourceEntries, targetEntries))
	require.Equal(t, 0, omittedEntryCountForPath("missing", sourceEntries, targetEntries))
}
