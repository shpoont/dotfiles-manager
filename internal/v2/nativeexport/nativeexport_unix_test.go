//go:build !windows

package nativeexport

import (
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnixSpecialFilesAreRejected(t *testing.T) {
	t.Parallel()

	payloadRoot := realTempDir(t)
	require.NoError(t, syscall.Mkfifo(filepath.Join(payloadRoot, "fifo"), 0o600))
	_, err := SummarizePayload(payloadRoot, Limits{MaxBytes: 1024, MaxEntries: 10})
	require.ErrorContains(t, err, "unsupported file type")

	copySrc := realTempDir(t)
	require.NoError(t, syscall.Mkfifo(filepath.Join(copySrc, "fifo"), 0o600))
	err = copyTree(copySrc, filepath.Join(realTempDir(t), "dst"))
	require.ErrorContains(t, err, "unsupported file")
}
