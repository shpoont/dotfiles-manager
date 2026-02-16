package logging

import (
	"io"
	"log/slog"
	"strings"

	"github.com/shpoont/dotfiles-manager/internal/dfmerr"
)

const (
	FormatText = "text"
	FormatJSON = "json"
)

func New(format, level string, out io.Writer) (*slog.Logger, error) {
	logLevel, err := parseLevel(level)
	if err != nil {
		return nil, err
	}

	handlerOptions := &slog.HandlerOptions{Level: logLevel}

	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", FormatText:
		return slog.New(slog.NewTextHandler(out, handlerOptions)), nil
	case FormatJSON:
		return slog.New(slog.NewJSONHandler(out, handlerOptions)), nil
	default:
		return nil, dfmerr.InvalidFlagValue("--log-format", format, "text|json")
	}
}

func parseLevel(level string) (slog.Leveler, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return nil, dfmerr.InvalidFlagValue("--log-level", level, "debug|info|warn|error")
	}
}
