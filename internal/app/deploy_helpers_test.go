package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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

	dirTarget := filepath.Join(targetRoot, "lua", "plugins")
	require.NoError(t, applyDeployCopy(deployCopyOperation{change: "create", typeID: "dir", targetAbs: dirTarget}))
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
	require.NoError(t, applyDeployCopy(deployCopyOperation{change: "replace_type", typeID: "dir", targetAbs: replaceTarget}))
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
