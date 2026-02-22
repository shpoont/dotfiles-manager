package dfmerr

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewAndMustCode(t *testing.T) {
	err := New(CodeConfigRequired, "config required", map[string]any{"x": 1})
	require.Equal(t, "config required", err.Error())
	require.Equal(t, CodeConfigRequired, MustCode(err))
}

func TestWrapAndAs(t *testing.T) {
	base := errors.New("base")
	err := Wrap(CodeIORead, "Read failed", map[string]any{"path": "/tmp/a"}, base)

	dfm, ok := As(err)
	require.True(t, ok)
	require.Equal(t, CodeIORead, dfm.Code)
	require.EqualError(t, errors.Unwrap(err), "base")
}

func TestAsUnknownAndMustCodeUnknown(t *testing.T) {
	_, ok := As(errors.New("x"))
	require.False(t, ok)
	require.Equal(t, Code(""), MustCode(errors.New("x")))
	require.Equal(t, Code(""), MustCode(nil))
}

func TestInvalidFlagValueAndWithDetails(t *testing.T) {
	err := InvalidFlagValue("--log-level", "verbose", "debug|info|warn|error")
	require.Equal(t, CodeFlagInvalidValue, MustCode(err))

	updated := WithDetails(err, map[string]any{"flag": "--log-level", "expected": "debug|info|warn|error"})
	dfm, ok := As(updated)
	require.True(t, ok)
	require.Equal(t, "--log-level", dfm.Details["flag"])
}

func TestWithDetailsNonDFMError(t *testing.T) {
	base := errors.New("plain")
	require.Equal(t, base, WithDetails(base, map[string]any{"x": 1}))
}

func TestParserErrorCodes(t *testing.T) {
	require.Equal(t, Code("DFM_PARSER_UNKNOWN_FLAG"), CodeParserUnknownFlag)
	require.Equal(t, Code("DFM_PARSER_UNKNOWN_COMMAND"), CodeParserUnknownCommand)
	require.Equal(t, Code("DFM_PARSER_ARG_FAILURE"), CodeParserArgFailure)
}
