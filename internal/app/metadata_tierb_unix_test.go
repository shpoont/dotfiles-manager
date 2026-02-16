//go:build linux || darwin

package app

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestDefaultCaptureAtimeBranches(t *testing.T) {
	originalStat := statPath
	t.Cleanup(func() {
		statPath = originalStat
	})

	statPath = func(string, *unix.Stat_t) error { return syscall.ENOTSUP }
	_, err := defaultCaptureAtime("/tmp/missing")
	require.ErrorIs(t, err, errMetadataUnsupported)

	statPath = func(string, *unix.Stat_t) error { return errors.New("boom") }
	_, err = defaultCaptureAtime("/tmp/missing")
	require.EqualError(t, err, "boom")

	atimeTs := unix.NsecToTimespec(123456789)
	statPath = func(_ string, st *unix.Stat_t) error {
		st.Atim = atimeTs
		return nil
	}
	atime, err := defaultCaptureAtime("/tmp/any")
	require.NoError(t, err)
	require.Equal(t, time.Unix(atimeTs.Sec, atimeTs.Nsec), atime)
}

func TestDefaultCopyXattrsWithStubbedDeps(t *testing.T) {
	origList := listXattrNamesPath
	origRead := readXattrPath
	origSet := setXattrPath
	t.Cleanup(func() {
		listXattrNamesPath = origList
		readXattrPath = origRead
		setXattrPath = origSet
	})

	listXattrNamesPath = func(string) ([]string, error) { return nil, errMetadataUnsupported }
	err := defaultCopyXattrs("src", "dst")
	require.ErrorIs(t, err, errMetadataUnsupported)

	listXattrNamesPath = func(string) ([]string, error) { return nil, errors.New("list fail") }
	err = defaultCopyXattrs("src", "dst")
	require.EqualError(t, err, "list fail")

	listXattrNamesPath = func(string) ([]string, error) { return []string{"a"}, nil }
	readXattrPath = func(string, string) ([]byte, error) { return nil, errMetadataUnsupported }
	err = defaultCopyXattrs("src", "dst")
	require.ErrorIs(t, err, errMetadataUnsupported)

	readXattrPath = func(string, string) ([]byte, error) { return nil, errors.New("read fail") }
	err = defaultCopyXattrs("src", "dst")
	require.EqualError(t, err, "read fail")

	readXattrPath = func(string, string) ([]byte, error) { return []byte("v"), nil }
	setXattrPath = func(string, string, []byte, int) error { return syscall.ENOTSUP }
	err = defaultCopyXattrs("src", "dst")
	require.ErrorIs(t, err, errMetadataUnsupported)

	setXattrPath = func(string, string, []byte, int) error { return errors.New("set fail") }
	err = defaultCopyXattrs("src", "dst")
	require.EqualError(t, err, "set fail")

	setCalled := false
	setXattrPath = func(path, name string, value []byte, flags int) error {
		setCalled = true
		require.Equal(t, "dst", path)
		require.Equal(t, "a", name)
		require.Equal(t, []byte("v"), value)
		require.Equal(t, 0, flags)
		return nil
	}
	err = defaultCopyXattrs("src", "dst")
	require.NoError(t, err)
	require.True(t, setCalled)
}

func TestDefaultCopyACLWithStubbedDeps(t *testing.T) {
	origLookPath := lookPathPath
	origReadACL := readACLPath
	origWriteACL := writeACLPath
	t.Cleanup(func() {
		lookPathPath = origLookPath
		readACLPath = origReadACL
		writeACLPath = origWriteACL
	})

	lookPathPath = func(name string) (string, error) {
		if name == "getfacl" {
			return "", errors.New("missing")
		}
		return "/bin/echo", nil
	}
	err := defaultCopyACL("src", "dst")
	require.ErrorIs(t, err, errMetadataUnsupported)

	lookPathPath = func(name string) (string, error) {
		if name == "setfacl" {
			return "", errors.New("missing")
		}
		return "/bin/echo", nil
	}
	err = defaultCopyACL("src", "dst")
	require.ErrorIs(t, err, errMetadataUnsupported)

	lookPathPath = func(string) (string, error) { return "/bin/echo", nil }
	readACLPath = func(string, string) ([]byte, error) { return nil, errMetadataUnsupported }
	err = defaultCopyACL("src", "dst")
	require.ErrorIs(t, err, errMetadataUnsupported)

	readACLPath = func(string, string) ([]byte, error) { return nil, errors.New("read fail") }
	err = defaultCopyACL("src", "dst")
	require.EqualError(t, err, "getfacl failed: read fail")

	readACLPath = func(string, string) ([]byte, error) { return []byte("acl"), nil }
	writeACLPath = func(string, string, []byte) error { return errMetadataUnsupported }
	err = defaultCopyACL("src", "dst")
	require.ErrorIs(t, err, errMetadataUnsupported)

	writeACLPath = func(string, string, []byte) error { return errors.New("write fail") }
	err = defaultCopyACL("src", "dst")
	require.EqualError(t, err, "setfacl failed: write fail")

	writeCalled := false
	writeACLPath = func(path, target string, payload []byte) error {
		writeCalled = true
		require.Equal(t, "/bin/echo", path)
		require.Equal(t, "dst", target)
		require.Equal(t, []byte("acl"), payload)
		return nil
	}
	err = defaultCopyACL("src", "dst")
	require.NoError(t, err)
	require.True(t, writeCalled)
}

func TestACLHelpersAndRealXattrCopy(t *testing.T) {
	root := t.TempDir()
	unsupportedScript := filepath.Join(root, "unsupported.sh")
	require.NoError(t, os.WriteFile(unsupportedScript, []byte("#!/bin/sh\necho 'operation not supported' 1>&2\nexit 1\n"), 0o755))

	_, err := readACLFromSource("echo", "source")
	require.NoError(t, err)
	_, err = readACLFromSource(unsupportedScript, "source")
	require.ErrorIs(t, err, errMetadataUnsupported)

	err = writeACLToTarget("true", "target", []byte("acl"))
	require.NoError(t, err)
	err = writeACLToTarget(unsupportedScript, "target", []byte("acl"))
	require.ErrorIs(t, err, errMetadataUnsupported)

	sourcePath := filepath.Join(root, "source.txt")
	targetPath := filepath.Join(root, "target.txt")
	require.NoError(t, os.WriteFile(sourcePath, []byte("s"), 0o644))
	require.NoError(t, os.WriteFile(targetPath, []byte("t"), 0o644))

	key := "user.dotfiles_manager_test"
	if runtime.GOOS == "darwin" {
		key = "com.dotfiles-manager.test"
	}
	setErr := unix.Setxattr(sourcePath, key, []byte("value"), 0)
	if isMetadataUnsupported(setErr) {
		t.Skip("xattrs unsupported on this filesystem")
	}
	require.NoError(t, setErr)

	err = defaultCopyXattrs(sourcePath, targetPath)
	if isMetadataUnsupported(err) {
		t.Skip("xattrs unsupported during copy")
	}
	require.NoError(t, err)

	value, err := readXattr(targetPath, key)
	require.NoError(t, err)
	require.Equal(t, []byte("value"), value)
}
