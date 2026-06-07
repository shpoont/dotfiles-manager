package recipe

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const (
	ListCommand = "recipe.list"
	ListRunID   = "recipe-list"
)

type ListReport struct {
	Schema        string         `json:"schema"`
	SchemaVersion int            `json:"schemaVersion"`
	Command       string         `json:"command"`
	RunID         string         `json:"runId"`
	Summary       ExplainSummary `json:"summary"`
	Items         []any          `json:"items"`
	RecipeList    RecipeList     `json:"recipeList"`
}

type ListOptions struct {
	RepoRoot string
}

type RecipeList struct {
	Targets     []RecipeListTarget  `json:"targets"`
	Diagnostics []ExplainDiagnostic `json:"diagnostics"`
}

type RecipeListTarget struct {
	ID              string   `json:"id"`
	DisplayName     string   `json:"displayName"`
	Aliases         []string `json:"aliases"`
	Source          string   `json:"source"`
	RecipeRef       string   `json:"recipeRef"`
	TrustStatus     string   `json:"trustStatus"`
	Version         string   `json:"version"`
	SupportLevel    string   `json:"supportLevel"`
	Capability      string   `json:"capability"`
	PlatformSupport string   `json:"platformSupport"`
	Summary         string   `json:"summary"`
}

func List(opts ListOptions) *ListReport {
	report := &ListReport{
		Schema:        ExplainSchema,
		SchemaVersion: SupportedVersion,
		Command:       ListCommand,
		RunID:         ListRunID,
		Summary:       ExplainSummary{Status: "ok"},
		Items:         []any{},
		RecipeList:    RecipeList{Diagnostics: []ExplainDiagnostic{}},
	}
	for _, target := range ListBundledTargets() {
		report.RecipeList.Targets = append(report.RecipeList.Targets, RecipeListTarget{
			ID:              target.ID,
			DisplayName:     target.DisplayName,
			Aliases:         append([]string(nil), target.Aliases...),
			Source:          target.Source,
			RecipeRef:       target.RecipeRef,
			TrustStatus:     target.TrustStatus,
			Version:         target.Version,
			SupportLevel:    target.SupportLevel,
			Capability:      target.Capability,
			PlatformSupport: target.PlatformSupport,
			Summary:         target.Summary,
		})
		report.RecipeList.Diagnostics = appendLocalRecipeCollisionDiagnostics(report.RecipeList.Diagnostics, opts.RepoRoot, target)
	}
	sort.Slice(report.RecipeList.Targets, func(i, j int) bool {
		return report.RecipeList.Targets[i].ID < report.RecipeList.Targets[j].ID
	})
	sortDiagnostics(report.RecipeList.Diagnostics)
	return report
}

func ListJSON(report *ListReport) (string, error) {
	if report == nil {
		report = &ListReport{
			Schema:        ExplainSchema,
			SchemaVersion: SupportedVersion,
			Command:       ListCommand,
			RunID:         ListRunID,
			Summary:       ExplainSummary{Status: "error"},
			Items:         []any{},
		}
	}
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	return string(payload) + "\n", nil
}

func ListText(report *ListReport) string {
	if report == nil {
		return "recipe list\nsummary status=error targets=0"
	}
	var lines []string
	lines = append(lines, "recipe list")
	if len(report.RecipeList.Targets) > 0 {
		lines = append(lines, "targets:")
		for _, target := range report.RecipeList.Targets {
			lines = append(lines, fmt.Sprintf("  %s source=%s trust=%s support=%s capability=%s platform=%s aliases=%s", target.ID, target.Source, target.TrustStatus, target.SupportLevel, target.Capability, target.PlatformSupport, aliasesText(target.Aliases)))
			if strings.TrimSpace(target.Summary) != "" {
				lines = append(lines, "    "+target.Summary)
			}
		}
	}
	if len(report.RecipeList.Diagnostics) > 0 {
		lines = append(lines, "diagnostics:")
		for _, diagnostic := range report.RecipeList.Diagnostics {
			lines = append(lines, fmt.Sprintf("  %s[%s]: %s", diagnostic.Severity, diagnostic.Code, diagnostic.Message))
		}
	}
	lines = append(lines, fmt.Sprintf("summary status=%s targets=%d", report.Summary.Status, len(report.RecipeList.Targets)))
	return strings.Join(lines, "\n")
}

func aliasesText(aliases []string) string {
	if len(aliases) == 0 {
		return "-"
	}
	copied := append([]string(nil), aliases...)
	sort.Strings(copied)
	return strings.Join(copied, ",")
}
