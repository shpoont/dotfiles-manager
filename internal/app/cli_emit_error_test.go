package app

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

type failWriter struct{}

func (failWriter) Write(_ []byte) (int, error) {
	return 0, io.ErrClosedPipe
}

func TestRunCommandJSONEmitFailure(t *testing.T) {
	projectDir := t.TempDir()
	setTempHome(t)
	setCWD(t, projectDir)

	writeConfig(t, projectDir, []byte(`syncs:
  - target: .config/nvim
    source: source/nvim
`))

	cmd := NewRootCmd()
	cmd.SetOut(failWriter{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"deploy", "--json", "--dry-run"})

	err := cmd.Execute()
	require.Error(t, err)
	require.True(t, errors.Is(err, io.ErrClosedPipe))
}
