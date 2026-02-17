package logging

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/shpoont/dotfiles-manager/internal/dfmerr"
	"github.com/stretchr/testify/require"
)

func TestNewAcceptsSupportedLevels(t *testing.T) {
	levels := []string{"", "debug", "info", "warn", "error"}
	for _, level := range levels {
		_, err := New(level, &bytes.Buffer{})
		require.NoError(t, err, level)
	}
}

func TestNewRejectsInvalidLevel(t *testing.T) {
	_, err := New("verbose", &bytes.Buffer{})
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeFlagInvalidValue, dfmerr.MustCode(err))
}

func TestResolvePathDefaultByPlatformRules(t *testing.T) {
	linuxDefault := defaultPathForOS("linux", "/home/alice", "")
	require.Equal(t, "/home/alice/.local/state/dotfiles-manager/dotfiles-manager.log", filepath.ToSlash(linuxDefault))

	linuxXDG := defaultPathForOS("linux", "/home/alice", "/tmp/state")
	require.Equal(t, "/tmp/state/dotfiles-manager/dotfiles-manager.log", filepath.ToSlash(linuxXDG))

	darwinDefault := defaultPathForOS("darwin", "/Users/alice", "")
	require.Equal(t, "/Users/alice/Library/Logs/dotfiles-manager/dotfiles-manager.log", filepath.ToSlash(darwinDefault))
}

func TestResolvePathOverrideAndTilde(t *testing.T) {
	homeDir := t.TempDir()
	originalHome, hadHome := os.LookupEnv("HOME")
	require.NoError(t, os.Setenv("HOME", homeDir))
	t.Cleanup(func() {
		if hadHome {
			_ = os.Setenv("HOME", originalHome)
		} else {
			_ = os.Unsetenv("HOME")
		}
	})

	path, err := ResolvePath("~/logs/custom.log")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(homeDir, "logs", "custom.log"), path)

	path, err = ResolvePath("relative.log")
	require.NoError(t, err)
	require.Equal(t, "relative.log", filepath.ToSlash(path))
}

func TestOpenFileCreatesDirectoriesAndAppends(t *testing.T) {
	root := t.TempDir()
	logPath := filepath.Join(root, "nested", "path", "app.log")

	fileHandle, err := OpenFile(logPath)
	require.NoError(t, err)
	_, err = fileHandle.WriteString("line-1\n")
	require.NoError(t, err)
	require.NoError(t, fileHandle.Close())

	fileHandle, err = OpenFile(logPath)
	require.NoError(t, err)
	_, err = fileHandle.WriteString("line-2\n")
	require.NoError(t, err)
	require.NoError(t, fileHandle.Close())

	content, err := os.ReadFile(logPath)
	require.NoError(t, err)
	require.Equal(t, "line-1\nline-2\n", string(content))
}
