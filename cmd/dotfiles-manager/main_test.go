package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunReturnsZeroForHelp(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"dotfiles-manager", "--help"}

	require.Equal(t, 0, run())
}

func TestRunReturnsZeroForVersion(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"dotfiles-manager", "--version"}

	require.Equal(t, 0, run())
}

func TestMainUsesExitHook(t *testing.T) {
	oldArgs := os.Args
	oldExit := osExit
	t.Cleanup(func() {
		os.Args = oldArgs
		osExit = oldExit
	})

	os.Args = []string{"dotfiles-manager", "--help"}

	exitCode := -1
	osExit = func(code int) {
		exitCode = code
	}

	main()
	require.Equal(t, 0, exitCode)
}
