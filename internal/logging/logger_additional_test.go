package logging

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewDefaults(t *testing.T) {
	t.Parallel()
	logger, err := New("", "", &bytes.Buffer{})
	require.NoError(t, err)
	require.NotNil(t, logger)
}

func TestSupportedLevels(t *testing.T) {
	t.Parallel()
	levels := []string{"debug", "info", "warn", "error"}
	for _, level := range levels {
		logger, err := New("text", level, &bytes.Buffer{})
		require.NoError(t, err)
		require.NotNil(t, logger)
	}
}

func TestLoggerWritesToBuffer(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	logger, err := New("json", "info", buf)
	require.NoError(t, err)

	logger.Info("hello", slog.String("k", "v"))
	require.Contains(t, buf.String(), "\"msg\":\"hello\"")
}
