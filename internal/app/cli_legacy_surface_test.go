package app

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLegacyV1CommandsAreHiddenFromV2RootHelp(t *testing.T) {
	rootStdout, rootStderr, err := runSelectedPreviewTextCLI(t, []string{"--help"})
	require.NoError(t, err)
	require.Empty(t, rootStderr)

	for _, forbidden := range []string{"deploy", "import", "migrate"} {
		require.NotContains(t, rootStdout, "\n  "+forbidden, "legacy command %q must not be listed in root help", forbidden)
	}
	require.Contains(t, rootStdout, "Normal v2 workflow:\n  status -> diff -> sync")
	require.Contains(t, rootStdout, "sync        Sync safe v2 settings changes between live settings and stored settings")
}

func TestLegacyV1DirectHelpIdentifiesCompatibilitySurface(t *testing.T) {
	for _, command := range []string{"deploy", "import", "migrate"} {
		stdout, stderr, err := runSelectedPreviewTextCLI(t, []string{command, "--help"})
		require.NoError(t, err, command)
		require.Empty(t, stderr, command)
		require.Contains(t, stdout, "Legacy v1 compatibility", command)
		require.Contains(t, stdout, "hidden from the normal v2 help", command)
		require.Contains(t, stdout, "status -> diff -> sync", command)
	}
}

func TestLegacyV1MigrateSubcommandHelpIdentifiesCompatibilitySurface(t *testing.T) {
	for _, args := range [][]string{
		{"migrate", "parity", "--help"},
		{"migrate", "promote-preview", "--help"},
	} {
		stdout, stderr, err := runSelectedPreviewTextCLI(t, args)
		require.NoError(t, err, strings.Join(args, " "))
		require.Empty(t, stderr, strings.Join(args, " "))
		require.Contains(t, stdout, "Legacy v1 compatibility", strings.Join(args, " "))
	}
}

func TestPublicStatusDiffHelpUsesV2RefsNotLegacySourceTargetLanguage(t *testing.T) {
	for _, args := range [][]string{
		{"status", "--help"},
		{"diff", "--help"},
	} {
		stdout, stderr, err := runSelectedPreviewTextCLI(t, args)
		require.NoError(t, err, strings.Join(args, " "))
		require.Empty(t, stderr, strings.Join(args, " "))
		require.NotContains(t, stdout, "path-or-ref", strings.Join(args, " "))
		require.NotContains(t, stdout, "source -> target", strings.Join(args, " "))
		require.NotContains(t, stdout, "target -> source", strings.Join(args, " "))
		require.NotContains(t, stdout, "deploy|import", strings.Join(args, " "))
	}
}
