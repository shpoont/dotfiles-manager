package catalogdiscovery

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	Schema        = "dotfiles-manager.v2.catalogs"
	SchemaVersion = 1
	Command       = "catalog.list"
	RunID         = "catalog-list"

	OfficialCatalogID      = "dotfiles-manager/official"
	OfficialCatalogVersion = "9f2c7a1"
	OfficialCatalogUpdated = "2026-06-30 18:00 UTC"
)

type Summary struct {
	Status   string `json:"status"`
	Catalogs int    `json:"catalogs"`
	Failed   int    `json:"failed"`
}

type Report struct {
	Schema        string       `json:"schema"`
	SchemaVersion int          `json:"schemaVersion"`
	Command       string       `json:"command"`
	RunID         string       `json:"runId"`
	Summary       Summary      `json:"summary"`
	Catalogs      []Catalog    `json:"catalogs"`
	Diagnostics   []Diagnostic `json:"diagnostics"`
	Error         *ErrorObject `json:"error,omitempty"`
}

type Catalog struct {
	ID      string `json:"id"`
	State   string `json:"state"`
	Purpose string `json:"purpose"`
	Version string `json:"version"`
	Updated string `json:"updated"`
}

type Diagnostic struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type ErrorObject struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

func List() *Report {
	return &Report{
		Schema:        Schema,
		SchemaVersion: SchemaVersion,
		Command:       Command,
		RunID:         RunID,
		Summary:       Summary{Status: "ok", Catalogs: 1},
		Catalogs: []Catalog{
			{
				ID:      OfficialCatalogID,
				State:   "active for discovery",
				Purpose: "app/tool support",
				Version: OfficialCatalogVersion,
				Updated: OfficialCatalogUpdated,
			},
		},
		Diagnostics: []Diagnostic{},
	}
}

func JSON(report *Report) (string, error) {
	if report == nil {
		report = &Report{
			Schema:        Schema,
			SchemaVersion: SchemaVersion,
			Command:       Command,
			RunID:         RunID,
			Summary:       Summary{Status: "error", Failed: 1},
			Catalogs:      []Catalog{},
			Diagnostics:   []Diagnostic{},
		}
	}
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	return string(payload) + "\n", nil
}

func Text(report *Report) string {
	if report == nil || report.Error != nil {
		message := "The command could not complete."
		if report != nil && report.Error != nil && strings.TrimSpace(report.Error.Message) != "" {
			message = report.Error.Message
		}
		return strings.Join([]string{"Catalogs", "", message}, "\n")
	}
	lines := []string{
		"Catalogs",
		"",
		"Catalogs define app/tool support; they do not store your settings.",
		"",
	}
	for _, catalog := range report.Catalogs {
		lines = append(lines, fmt.Sprintf("  %s  %s", catalog.ID, catalog.State))
		if strings.TrimSpace(catalog.Version) != "" {
			lines = append(lines, "    Catalog version: "+catalog.Version)
		}
		if strings.TrimSpace(catalog.Updated) != "" {
			lines = append(lines, "    Catalog updated: "+catalog.Updated)
		}
	}
	return strings.Join(lines, "\n")
}
