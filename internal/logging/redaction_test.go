package logging

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRedactStringEmptyAndWhitespace(t *testing.T) {
	t.Parallel()
	require.Equal(t, "", RedactString(""))
	require.Equal(t, "", RedactString("   "))
}

func TestRedactStringSensitiveMarkers(t *testing.T) {
	t.Parallel()
	require.Equal(t, RedactedValue, RedactString("my-secret-token"))
	require.Equal(t, RedactedValue, RedactString("PASSWORD=abc"))
	require.Equal(t, RedactedValue, RedactString("Api_Key=123"))
}

func TestRedactStringKeepsNonSensitive(t *testing.T) {
	t.Parallel()
	require.Equal(t, "normal-value", RedactString(" normal-value "))
}
