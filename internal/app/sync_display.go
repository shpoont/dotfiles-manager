package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/shpoont/dotfiles-manager/internal/config"
)

const jsonSchemaVersion = "2.0"

type syncDisplay struct {
	Label  string
	Target string
	Source string
}

func buildSyncDisplay(syncIndex int, syncCfg config.Sync) syncDisplay {
	target := renderTargetDisplay(syncCfg.Target)
	source := renderSourceDisplay(syncCfg.Source)
	return syncDisplay{
		Label:  fmt.Sprintf("sync[%d] target=%s source=%s", syncIndex, target, source),
		Target: target,
		Source: source,
	}
}

func renderTargetDisplay(target string) string {
	if target == "" {
		return "~"
	}
	clean := filepath.ToSlash(filepath.Clean(target))
	clean = strings.TrimPrefix(clean, "./")
	if clean == "." || clean == "" {
		return "~"
	}
	return "~/" + clean
}

func renderSourceDisplay(source string) string {
	if source == "" {
		return "."
	}
	clean := filepath.ToSlash(filepath.Clean(source))
	if clean == "." {
		return "."
	}
	if strings.HasPrefix(clean, "/") {
		return clean
	}
	if strings.HasPrefix(clean, "..") {
		return clean
	}
	if strings.HasPrefix(clean, "./") {
		return clean
	}
	return "./" + clean
}
