package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/shpoont/dotfiles-manager/internal/dfmerr"
)

const (
	appLogDir  = "dotfiles-manager"
	appLogFile = "dotfiles-manager.log"
)

var (
	userHomeDir = os.UserHomeDir
	mkdirAll    = os.MkdirAll
	openFile    = os.OpenFile
)

func New(level string, out io.Writer) (*slog.Logger, error) {
	logLevel, err := parseLevel(level)
	if err != nil {
		return nil, err
	}

	handlerOptions := &slog.HandlerOptions{Level: logLevel}
	return slog.New(slog.NewTextHandler(out, handlerOptions)), nil
}

func ResolvePath(override string) (string, error) {
	path := strings.TrimSpace(override)
	if path == "" {
		return defaultLogPath()
	}
	return expandTilde(path)
}

func OpenFile(path string) (*os.File, error) {
	dir := filepath.Dir(path)
	if err := mkdirAll(dir, 0o755); err != nil {
		return nil, dfmerr.Wrap(
			dfmerr.CodeIOWrite,
			fmt.Sprintf("Write failed: %s", dir),
			map[string]any{"path": dir},
			err,
		)
	}

	fileHandle, err := openFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, dfmerr.Wrap(
			dfmerr.CodeIOWrite,
			fmt.Sprintf("Write failed: %s", path),
			map[string]any{"path": path},
			err,
		)
	}
	return fileHandle, nil
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

func defaultLogPath() (string, error) {
	homeDir, err := userHomeDir()
	if err != nil {
		return "", dfmerr.Wrap(
			dfmerr.CodeIOWrite,
			"Write failed: cannot resolve user home directory",
			nil,
			err,
		)
	}

	path := defaultPathForOS(runtime.GOOS, homeDir, os.Getenv("XDG_STATE_HOME"))
	return filepath.Clean(path), nil
}

func defaultPathForOS(goos, homeDir, xdgStateHome string) string {
	if goos == "darwin" {
		return filepath.Join(homeDir, "Library", "Logs", appLogDir, appLogFile)
	}

	stateHome := strings.TrimSpace(xdgStateHome)
	if stateHome == "" {
		stateHome = filepath.Join(homeDir, ".local", "state")
	}
	return filepath.Join(stateHome, appLogDir, appLogFile)
}

func expandTilde(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return filepath.Clean(path), nil
	}

	homeDir, err := userHomeDir()
	if err != nil {
		return "", dfmerr.Wrap(
			dfmerr.CodeIOWrite,
			"Write failed: cannot resolve user home directory",
			nil,
			err,
		)
	}

	if path == "~" {
		return filepath.Clean(homeDir), nil
	}
	return filepath.Clean(filepath.Join(homeDir, strings.TrimPrefix(path, "~/"))), nil
}
