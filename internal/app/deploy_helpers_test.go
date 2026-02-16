package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shpoont/dotfiles-manager/internal/dfmerr"
	"github.com/stretchr/testify/require"
)

func TestApplyDeployCopyCoversTypeBranches(t *testing.T) {
	root := t.TempDir()
	sourceRoot := filepath.Join(root, "source")
	targetRoot := filepath.Join(root, "target")
	require.NoError(t, os.MkdirAll(sourceRoot, 0o755))
	require.NoError(t, os.MkdirAll(targetRoot, 0o755))

	sourceFile := filepath.Join(sourceRoot, "init.lua")
	require.NoError(t, os.WriteFile(sourceFile, []byte("content"), 0o644))

	sourceLink := filepath.Join(sourceRoot, "init.link")
	require.NoError(t, os.Symlink("init.lua", sourceLink))

	sourceDir := filepath.Join(sourceRoot, "lua", "plugins")
	require.NoError(t, os.MkdirAll(sourceDir, 0o755))
	dirTarget := filepath.Join(targetRoot, "lua", "plugins")
	require.NoError(t, applyDeployCopy(deployCopyOperation{change: "create", typeID: "dir", sourceAbs: sourceDir, targetAbs: dirTarget}))
	info, err := os.Stat(dirTarget)
	require.NoError(t, err)
	require.True(t, info.IsDir())

	fileTarget := filepath.Join(targetRoot, "lua", "init.lua")
	require.NoError(t, applyDeployCopy(deployCopyOperation{change: "create", typeID: "file", sourceAbs: sourceFile, targetAbs: fileTarget}))
	copied, err := os.ReadFile(fileTarget)
	require.NoError(t, err)
	require.Equal(t, "content", string(copied))

	linkTarget := filepath.Join(targetRoot, "lua", "init.link")
	require.NoError(t, applyDeployCopy(deployCopyOperation{change: "create", typeID: "symlink", sourceAbs: sourceLink, targetAbs: linkTarget}))
	linkDest, err := os.Readlink(linkTarget)
	require.NoError(t, err)
	require.Equal(t, "init.lua", linkDest)

	replaceTarget := filepath.Join(targetRoot, "lua", "replace-me")
	require.NoError(t, os.WriteFile(replaceTarget, []byte("old"), 0o644))
	replaceSource := filepath.Join(sourceRoot, "lua", "replace-me")
	require.NoError(t, os.MkdirAll(replaceSource, 0o755))
	require.NoError(t, applyDeployCopy(deployCopyOperation{change: "replace_type", typeID: "dir", sourceAbs: replaceSource, targetAbs: replaceTarget}))
	replaceInfo, err := os.Stat(replaceTarget)
	require.NoError(t, err)
	require.True(t, replaceInfo.IsDir())

	require.NoError(t, applyDeployCopy(deployCopyOperation{change: "create", typeID: "unknown", targetAbs: filepath.Join(targetRoot, "noop")}))
}

func TestCopyFileErrorBranches(t *testing.T) {
	root := t.TempDir()
	sourceFile := filepath.Join(root, "source.txt")
	require.NoError(t, os.WriteFile(sourceFile, []byte("x"), 0o644))

	err := copyFile(filepath.Join(root, "missing.txt"), filepath.Join(root, "out.txt"))
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeIORead, dfmerr.MustCode(err))

	parentFile := filepath.Join(root, "parent-file")
	require.NoError(t, os.WriteFile(parentFile, []byte("x"), 0o644))
	err = copyFile(sourceFile, filepath.Join(parentFile, "out.txt"))
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeIOWrite, dfmerr.MustCode(err))

	targetDir := filepath.Join(root, "target-dir")
	require.NoError(t, os.MkdirAll(targetDir, 0o755))
	err = copyFile(sourceFile, targetDir)
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeIOWrite, dfmerr.MustCode(err))
}

func TestCopyDirAndFilePreserveTierAMetadata(t *testing.T) {
	root := t.TempDir()

	sourceFile := filepath.Join(root, "source-file")
	require.NoError(t, os.WriteFile(sourceFile, []byte("x"), 0o700))
	sourceTime := time.Unix(1_701_000_000, 0).UTC()
	require.NoError(t, os.Chtimes(sourceFile, sourceTime, sourceTime))

	targetFile := filepath.Join(root, "target-file")
	require.NoError(t, copyFile(sourceFile, targetFile))

	sourceFileInfo, err := os.Stat(sourceFile)
	require.NoError(t, err)
	targetFileInfo, err := os.Stat(targetFile)
	require.NoError(t, err)
	require.Equal(t, sourceFileInfo.Mode().Perm(), targetFileInfo.Mode().Perm())
	require.Equal(t, sourceFileInfo.ModTime().Unix(), targetFileInfo.ModTime().Unix())

	sourceDir := filepath.Join(root, "source-dir")
	require.NoError(t, os.MkdirAll(sourceDir, 0o751))
	dirTime := time.Unix(1_702_000_000, 0).UTC()
	require.NoError(t, os.Chtimes(sourceDir, dirTime, dirTime))

	targetDir := filepath.Join(root, "target-dir", "nested")
	require.NoError(t, copyDir(sourceDir, targetDir))

	sourceDirInfo, err := os.Stat(sourceDir)
	require.NoError(t, err)
	targetDirInfo, err := os.Stat(targetDir)
	require.NoError(t, err)
	require.Equal(t, sourceDirInfo.Mode().Perm(), targetDirInfo.Mode().Perm())
	require.Equal(t, sourceDirInfo.ModTime().Unix(), targetDirInfo.ModTime().Unix())
}

func TestMetadataApplyFailuresReturnCode(t *testing.T) {
	root := t.TempDir()
	sourceFile := filepath.Join(root, "source-file")
	require.NoError(t, os.WriteFile(sourceFile, []byte("x"), 0o644))
	targetFile := filepath.Join(root, "target-file")

	originalChmod := chmodPath
	originalChtimes := chtimesPath
	t.Cleanup(func() {
		chmodPath = originalChmod
		chtimesPath = originalChtimes
	})

	chmodPath = func(string, os.FileMode) error { return errors.New("chmod failed") }
	chtimesPath = os.Chtimes
	err := copyFile(sourceFile, targetFile)
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeMetadataApply, dfmerr.MustCode(err))

	chmodPath = os.Chmod
	chtimesPath = func(string, time.Time, time.Time) error { return errors.New("chtimes failed") }
	sourceDir := filepath.Join(root, "source-dir")
	require.NoError(t, os.MkdirAll(sourceDir, 0o755))
	err = copyDir(sourceDir, filepath.Join(root, "target-dir"))
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeMetadataApply, dfmerr.MustCode(err))
}

func TestCopySymlinkBranches(t *testing.T) {
	root := t.TempDir()
	sourceFile := filepath.Join(root, "source.txt")
	require.NoError(t, os.WriteFile(sourceFile, []byte("x"), 0o644))
	sourceLink := filepath.Join(root, "source.link")
	require.NoError(t, os.Symlink("source.txt", sourceLink))

	err := copySymlink(sourceFile, filepath.Join(root, "bad.link"))
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeIORead, dfmerr.MustCode(err))

	parentFile := filepath.Join(root, "parent-file")
	require.NoError(t, os.WriteFile(parentFile, []byte("x"), 0o644))
	err = copySymlink(sourceLink, filepath.Join(parentFile, "child.link"))
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeIOWrite, dfmerr.MustCode(err))

	longTarget := filepath.Join(root, strings.Repeat("a", 300))
	err = copySymlink(sourceLink, longTarget)
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeIORead, dfmerr.MustCode(err))

	replaceTarget := filepath.Join(root, "replace.link")
	require.NoError(t, os.WriteFile(replaceTarget, []byte("x"), 0o644))
	require.NoError(t, copySymlink(sourceLink, replaceTarget))
	linkDest, err := os.Readlink(replaceTarget)
	require.NoError(t, err)
	require.Equal(t, "source.txt", linkDest)
}

func TestDeployRemoveBranches(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "remove-me")
	require.NoError(t, os.WriteFile(target, []byte("x"), 0o644))

	err := applyDeployRemove(deployRemoveOperation{targetAbs: target})
	require.NoError(t, err)
	_, err = os.Stat(target)
	require.Error(t, err)
	require.True(t, os.IsNotExist(err))

	err = removePath(filepath.Join(root, "missing"))
	require.NoError(t, err)

	invalidPath := string([]byte{'b', 'a', 'd', 0, 'p', 'a', 't', 'h'})
	err = applyDeployRemove(deployRemoveOperation{targetAbs: invalidPath})
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeIORemove, dfmerr.MustCode(err))

	err = removePath(invalidPath)
	require.Error(t, err)
}
