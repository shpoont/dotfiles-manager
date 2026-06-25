package app

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBackupRestoreCommandsAreNotPublicV2Surface(t *testing.T) {
	root := NewRootCmd()
	for _, command := range []string{"backup", "restore"} {
		_, _, err := runSelectedPreviewTextCLI(t, []string{command, "--help"})
		require.Error(t, err, command)
		require.Contains(t, err.Error(), `unknown command "`+command+`"`)
	}

	usage := root.UsageString()
	for _, forbidden := range []string{"backup", "restore"} {
		require.NotContains(t, strings.ToLower(usage), forbidden)
	}
}
