package app

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/shpoont/dotfiles-manager/internal/dfmerr"
	"github.com/stretchr/testify/require"
)

func TestApplyTierBMetadataIgnoresUnsupported(t *testing.T) {
	root := t.TempDir()
	targetPath := filepath.Join(root, "target.txt")
	require.NoError(t, os.WriteFile(targetPath, []byte("x"), 0o644))
	info, err := os.Stat(targetPath)
	require.NoError(t, err)

	origCapture := captureAtimePath
	origXattrs := copyXattrsPath
	origACL := copyACLPath
	origChtimes := chtimesPath
	t.Cleanup(func() {
		captureAtimePath = origCapture
		copyXattrsPath = origXattrs
		copyACLPath = origACL
		chtimesPath = origChtimes
	})

	captureAtimePath = func(string) (time.Time, error) { return time.Time{}, errMetadataUnsupported }
	copyXattrsPath = func(string, string) error { return errMetadataUnsupported }
	copyACLPath = func(string, string) error { return errMetadataUnsupported }
	chtimesPath = os.Chtimes

	err = applyTierBMetadata("/tmp/source", targetPath, info)
	require.NoError(t, err)
}

func TestApplyTierBMetadataWrapsSupportedFailures(t *testing.T) {
	root := t.TempDir()
	targetPath := filepath.Join(root, "target.txt")
	require.NoError(t, os.WriteFile(targetPath, []byte("x"), 0o644))
	info, err := os.Stat(targetPath)
	require.NoError(t, err)

	origCapture := captureAtimePath
	origXattrs := copyXattrsPath
	origACL := copyACLPath
	origChtimes := chtimesPath
	t.Cleanup(func() {
		captureAtimePath = origCapture
		copyXattrsPath = origXattrs
		copyACLPath = origACL
		chtimesPath = origChtimes
	})

	captureAtimePath = func(string) (time.Time, error) { return time.Now(), nil }
	copyXattrsPath = func(string, string) error { return nil }
	copyACLPath = func(string, string) error { return nil }
	chtimesPath = func(string, time.Time, time.Time) error { return syscall.EPERM }

	err = applyTierBMetadata("/tmp/source", targetPath, info)
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeMetadataApply, dfmerr.MustCode(err))

	dfm, ok := dfmerr.As(err)
	require.True(t, ok)
	require.Equal(t, "atime", dfm.Details["metadata"])
}

func TestApplyTierBMetadataXattrAndACLFailure(t *testing.T) {
	root := t.TempDir()
	targetPath := filepath.Join(root, "target.txt")
	require.NoError(t, os.WriteFile(targetPath, []byte("x"), 0o644))
	info, err := os.Stat(targetPath)
	require.NoError(t, err)

	origCapture := captureAtimePath
	origXattrs := copyXattrsPath
	origACL := copyACLPath
	origChtimes := chtimesPath
	t.Cleanup(func() {
		captureAtimePath = origCapture
		copyXattrsPath = origXattrs
		copyACLPath = origACL
		chtimesPath = origChtimes
	})

	captureAtimePath = func(string) (time.Time, error) { return time.Time{}, errMetadataUnsupported }
	chtimesPath = os.Chtimes
	copyXattrsPath = func(string, string) error { return errors.New("xattr failed") }
	copyACLPath = func(string, string) error { return nil }

	err = applyTierBMetadata("/tmp/source", targetPath, info)
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeMetadataApply, dfmerr.MustCode(err))
	dfm, ok := dfmerr.As(err)
	require.True(t, ok)
	require.Equal(t, "xattr", dfm.Details["metadata"])

	copyXattrsPath = func(string, string) error { return nil }
	copyACLPath = func(string, string) error { return errors.New("acl failed") }
	err = applyTierBMetadata("/tmp/source", targetPath, info)
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeMetadataApply, dfmerr.MustCode(err))
	dfm, ok = dfmerr.As(err)
	require.True(t, ok)
	require.Equal(t, "acl", dfm.Details["metadata"])
}

func TestIsMetadataUnsupported(t *testing.T) {
	t.Parallel()

	require.True(t, isMetadataUnsupported(errMetadataUnsupported))
	require.True(t, isMetadataUnsupported(syscall.ENOTSUP))
	require.True(t, isMetadataUnsupported(errors.New("operation not supported")))
	require.False(t, isMetadataUnsupported(errors.New("permission denied")))
}
