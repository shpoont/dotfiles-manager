package app

import (
	"fmt"
	"strings"
)

var buildVersion = "dev"

func currentVersion() string {
	version := strings.TrimSpace(buildVersion)
	if version == "" {
		return "dev"
	}
	return version
}

func versionLine() string {
	return fmt.Sprintf("dotfiles-manager version %s", currentVersion())
}
