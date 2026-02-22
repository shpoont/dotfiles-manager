package app

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVersionCommandAndFlagPrintVersionWithoutConfig(t *testing.T) {
	projectDir := t.TempDir()
	oldWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	require.NoError(t, os.Chdir(projectDir))

	oldVersion := buildVersion
	oldCommit := buildCommit
	oldDate := buildDate
	oldChannel := buildChannel
	oldProvenance := buildProvenance
	buildVersion = "1.2.3"
	buildCommit = "abc1234"
	buildDate = "2026-02-22T10:00:00Z"
	buildChannel = "stable"
	buildProvenance = "goreleaser"
	t.Cleanup(func() {
		buildVersion = oldVersion
		buildCommit = oldCommit
		buildDate = oldDate
		buildChannel = oldChannel
		buildProvenance = oldProvenance
	})

	testCases := [][]string{
		{"version"},
		{"--version"},
	}

	for _, args := range testCases {
		cmd := NewRootCmd()
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		cmd.SetOut(&stdout)
		cmd.SetErr(&stderr)
		cmd.SetArgs(args)

		require.NoError(t, cmd.Execute())
		require.Equal(t, "dotfiles-manager version=1.2.3 commit=abc1234 date=2026-02-22T10:00:00Z channel=stable provenance=goreleaser\n", stdout.String())
		require.Empty(t, stderr.String())
	}
}

func TestVersionCommandFallsBackToDevWhenUnset(t *testing.T) {
	projectDir := t.TempDir()
	oldWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	require.NoError(t, os.Chdir(projectDir))

	oldVersion := buildVersion
	oldCommit := buildCommit
	oldDate := buildDate
	oldChannel := buildChannel
	oldProvenance := buildProvenance
	buildVersion = ""
	buildCommit = ""
	buildDate = ""
	buildChannel = ""
	buildProvenance = ""
	t.Cleanup(func() {
		buildVersion = oldVersion
		buildCommit = oldCommit
		buildDate = oldDate
		buildChannel = oldChannel
		buildProvenance = oldProvenance
	})

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"version"})

	require.NoError(t, cmd.Execute())
	require.Equal(t, "dotfiles-manager version=dev commit=unknown date=unknown channel=dev provenance=unspecified\n", stdout.String())
}

func TestVersionCommandRejectsUnsupportedInputs(t *testing.T) {
	projectDir := t.TempDir()
	oldWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	require.NoError(t, os.Chdir(projectDir))

	testCases := []struct {
		args        []string
		errContains string
	}{
		{
			args:        []string{"version", "--json"},
			errContains: "unknown flag: --json",
		},
		{
			args:        []string{"version", "--dry-run"},
			errContains: "unknown flag: --dry-run",
		},
		{
			args:        []string{"version", "/tmp/path"},
			errContains: "unknown command \"/tmp/path\"",
		},
	}

	for _, tc := range testCases {
		cmd := NewRootCmd()
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs(tc.args)

		err := cmd.Execute()
		require.Error(t, err)
		require.Contains(t, err.Error(), tc.errContains)
	}
}
