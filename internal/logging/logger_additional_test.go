package logging

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/shpoont/dotfiles-manager/internal/dfmerr"
	"github.com/stretchr/testify/require"
)

func TestResolvePathDefaultUsesHome(t *testing.T) {
	homeDir := t.TempDir()
	originalHome, hadHome := os.LookupEnv("HOME")
	originalXDG, hadXDG := os.LookupEnv("XDG_STATE_HOME")
	require.NoError(t, os.Setenv("HOME", homeDir))
	require.NoError(t, os.Unsetenv("XDG_STATE_HOME"))
	t.Cleanup(func() {
		if hadHome {
			_ = os.Setenv("HOME", originalHome)
		} else {
			_ = os.Unsetenv("HOME")
		}
		if hadXDG {
			_ = os.Setenv("XDG_STATE_HOME", originalXDG)
		} else {
			_ = os.Unsetenv("XDG_STATE_HOME")
		}
	})

	resolved, err := ResolvePath("")
	require.NoError(t, err)
	require.Equal(t, appLogFile, filepath.Base(resolved))
	if runtime.GOOS == "darwin" {
		require.Contains(t, filepath.ToSlash(resolved), "/Library/Logs/dotfiles-manager/")
	} else {
		require.Contains(t, filepath.ToSlash(resolved), "/.local/state/dotfiles-manager/")
	}
}

func TestOpenFileInvalidPathReturnsIOWrite(t *testing.T) {
	_, err := OpenFile(string([]byte{'b', 'a', 'd', 0, 'p', 'a', 't', 'h'}))
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeIOWrite, dfmerr.MustCode(err))
}

func TestExpandTildeHomeOnlyAndHomeError(t *testing.T) {
	homeDir := t.TempDir()
	originalHomeFn := userHomeDir
	userHomeDir = func() (string, error) { return homeDir, nil }
	t.Cleanup(func() { userHomeDir = originalHomeFn })

	path, err := expandTilde("~")
	require.NoError(t, err)
	require.Equal(t, homeDir, path)

	userHomeDir = func() (string, error) { return "", errors.New("home unavailable") }
	_, err = expandTilde("~/logs/app.log")
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeIOWrite, dfmerr.MustCode(err))
}

func TestDefaultLogPathHomeError(t *testing.T) {
	originalHomeFn := userHomeDir
	userHomeDir = func() (string, error) { return "", errors.New("home unavailable") }
	t.Cleanup(func() { userHomeDir = originalHomeFn })

	_, err := defaultLogPath()
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeIOWrite, dfmerr.MustCode(err))
}

func TestOpenFileErrorBranches(t *testing.T) {
	originalMkdirAll := mkdirAll
	originalOpenFile := openFile
	t.Cleanup(func() {
		mkdirAll = originalMkdirAll
		openFile = originalOpenFile
	})

	mkdirAll = func(string, fs.FileMode) error { return errors.New("mkdir failed") }
	_, err := OpenFile(filepath.Join(t.TempDir(), "logs", "app.log"))
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeIOWrite, dfmerr.MustCode(err))

	mkdirAll = originalMkdirAll
	openFile = func(string, int, fs.FileMode) (*os.File, error) { return nil, errors.New("open failed") }
	_, err = OpenFile(filepath.Join(t.TempDir(), "logs", "app.log"))
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeIOWrite, dfmerr.MustCode(err))
}
