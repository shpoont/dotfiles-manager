package logging

import (
	"bytes"
	"testing"

	"github.com/shpoont/dotfiles-manager/internal/dfmerr"
	"github.com/stretchr/testify/require"
)

func TestNewAcceptsTextAndJSON(t *testing.T) {
	t.Parallel()

	_, err := New("text", "info", &bytes.Buffer{})
	require.NoError(t, err)

	_, err = New("json", "debug", &bytes.Buffer{})
	require.NoError(t, err)
}

func TestNewRejectsInvalidFormat(t *testing.T) {
	t.Parallel()

	_, err := New("yaml", "info", &bytes.Buffer{})
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeFlagInvalidValue, dfmerr.MustCode(err))
}

func TestNewRejectsInvalidLevel(t *testing.T) {
	t.Parallel()

	_, err := New("text", "verbose", &bytes.Buffer{})
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeFlagInvalidValue, dfmerr.MustCode(err))
}
