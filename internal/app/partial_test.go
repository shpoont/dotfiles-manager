package app

import (
	"errors"
	"testing"

	"github.com/shpoont/dotfiles-manager/internal/dfmerr"
	"github.com/stretchr/testify/require"
)

func TestMarkPartial(t *testing.T) {
	t.Parallel()

	err := markPartial(dfmerr.New(dfmerr.CodeIOWrite, "Write failed", map[string]any{"path": "/tmp/a"}))
	require.True(t, errorIsPartial(err))
	details := err.(*dfmerr.Error).Details
	require.Equal(t, "/tmp/a", details["path"])
	require.Equal(t, true, details["partial"])

	plain := errors.New("plain")
	require.Equal(t, plain, markPartial(plain))
}
