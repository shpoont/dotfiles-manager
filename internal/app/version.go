package app

import (
	"fmt"
	"strings"
)

var (
	buildVersion    = "dev"
	buildCommit     = "unknown"
	buildDate       = "unknown"
	buildChannel    = "dev"
	buildProvenance = "unspecified"
)

func currentVersion() string {
	return buildValue(buildVersion, "dev")
}

func versionLine() string {
	return fmt.Sprintf(
		"dotfiles-manager version=%s commit=%s date=%s channel=%s provenance=%s",
		currentVersion(),
		buildValue(buildCommit, "unknown"),
		buildValue(buildDate, "unknown"),
		buildValue(buildChannel, "dev"),
		buildValue(buildProvenance, "unspecified"),
	)
}

func buildValue(raw string, fallback string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return fallback
	}
	return value
}
