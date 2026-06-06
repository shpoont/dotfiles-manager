package migration

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
)

const (
	PromotionReportSchema = "dotfiles-manager.v2.migration-promotion-preview"

	PromotionResultPromotable    = "promotable"
	PromotionResultBlocked       = "blocked"
	PromotionResultNotPromotable = "not-promotable"

	gitConfigCandidateID = "gitconfig"
	gitConfigRuleID      = "gitconfig-identity-v1"
)

type PromotionOptions struct {
	RunDir        string
	GeneratedRoot string
	Plan          *Plan
}

type PromotionReport struct {
	Schema          string           `json:"schema" yaml:"schema"`
	SchemaVersion   int              `json:"schemaVersion" yaml:"schemaVersion"`
	RunID           string           `json:"runId" yaml:"runId"`
	MigrationRunDir string           `json:"migrationRunDir" yaml:"migrationRunDir"`
	GeneratedRoot   string           `json:"generatedRoot" yaml:"generatedRoot"`
	ConfigPath      string           `json:"configPath" yaml:"configPath"`
	Items           []PromotionItem  `json:"items" yaml:"items"`
	Summary         PromotionSummary `json:"summary" yaml:"summary"`
	Error           *ErrorObject     `json:"error,omitempty" yaml:"error,omitempty"`
}

type PromotionItem struct {
	SyncIndex              int                    `json:"syncIndex" yaml:"syncIndex"`
	SyncRef                string                 `json:"syncRef" yaml:"syncRef"`
	CurrentTargetRef       string                 `json:"currentTargetRef" yaml:"currentTargetRef"`
	CurrentSettingRef      string                 `json:"currentSettingRef" yaml:"currentSettingRef"`
	LegacySource           string                 `json:"legacySource" yaml:"legacySource"`
	LegacyTarget           string                 `json:"legacyTarget" yaml:"legacyTarget"`
	Driver                 string                 `json:"driver" yaml:"driver"`
	LocationID             string                 `json:"locationId" yaml:"locationId"`
	LocationDefault        string                 `json:"locationDefault" yaml:"locationDefault"`
	ResourceID             string                 `json:"resourceId" yaml:"resourceId"`
	ResourcePath           string                 `json:"resourcePath" yaml:"resourcePath"`
	DesiredArtifactBinding DesiredArtifactBinding `json:"desiredArtifactBinding" yaml:"desiredArtifactBinding"`
	CandidateID            string                 `json:"candidateId,omitempty" yaml:"candidateId,omitempty"`
	PromotionRuleID        string                 `json:"promotionRuleId,omitempty" yaml:"promotionRuleId,omitempty"`
	ProposedTargetRef      string                 `json:"proposedTargetRef,omitempty" yaml:"proposedTargetRef,omitempty"`
	ProposedSettingRef     string                 `json:"proposedSettingRef,omitempty" yaml:"proposedSettingRef,omitempty"`
	Result                 string                 `json:"result" yaml:"result"`
	Diagnostics            []Diagnostic           `json:"diagnostics,omitempty" yaml:"diagnostics,omitempty"`
}

type PromotionSummary struct {
	Syncs               int    `json:"syncs" yaml:"syncs"`
	Promotable          int    `json:"promotable" yaml:"promotable"`
	Blocked             int    `json:"blocked" yaml:"blocked"`
	NotPromotable       int    `json:"notPromotable" yaml:"notPromotable"`
	GitConfigCandidates int    `json:"gitConfigCandidates" yaml:"gitConfigCandidates"`
	Status              string `json:"status" yaml:"status"`
}

func BuildPromotionReport(opts PromotionOptions) (*PromotionReport, error) {
	plan, runDir, err := promotionPlanAndRunDir(opts)
	if err != nil {
		return nil, err
	}
	generatedRoot, err := parityGeneratedRoot(ParityOptions{GeneratedRoot: opts.GeneratedRoot}, runDir)
	if err != nil {
		return nil, err
	}
	if err := validatePromotionGeneratedRoot(generatedRoot); err != nil {
		return nil, err
	}

	report := &PromotionReport{
		Schema:          PromotionReportSchema,
		SchemaVersion:   SchemaVersion,
		RunID:           plan.RunID,
		MigrationRunDir: runDir,
		GeneratedRoot:   generatedRoot,
		ConfigPath:      plan.ConfigPath,
	}

	items := append([]Item(nil), plan.Items...)
	sort.SliceStable(items, func(i, j int) bool { return items[i].SyncIndex < items[j].SyncIndex })
	for _, item := range items {
		report.Items = append(report.Items, buildPromotionItem(item))
	}
	report.Summary = summarizePromotion(report.Items)
	if report.Summary.Blocked > 0 {
		report.Error = &ErrorObject{Code: "promotion-preview-blocked", Message: fmt.Sprintf("promotion preview has %d blocked candidate(s)", report.Summary.Blocked)}
	}
	return report, nil
}

func PromotionJSON(report *PromotionReport) (string, error) {
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	return string(payload) + "\n", nil
}

func NewPromotionErrorReport(runDir string, code string, message string, details map[string]any) *PromotionReport {
	return &PromotionReport{
		Schema:          PromotionReportSchema,
		SchemaVersion:   SchemaVersion,
		MigrationRunDir: runDir,
		Summary:         PromotionSummary{Status: "error"},
		Error:           &ErrorObject{Code: code, Message: message, Details: details},
	}
}

func promotionPlanAndRunDir(opts PromotionOptions) (*Plan, string, error) {
	if opts.Plan != nil {
		if err := validateParityPlan(opts.Plan); err != nil {
			return nil, "", err
		}
		runDir := strings.TrimSpace(opts.RunDir)
		if runDir == "" {
			runDir = opts.Plan.OutputDir
		}
		if runDir != "" {
			absRunDir, err := filepath.Abs(runDir)
			if err != nil {
				return nil, "", fmt.Errorf("resolve migration run dir %q: %w", runDir, err)
			}
			runDir = absRunDir
		}
		return opts.Plan, runDir, nil
	}

	plan, runDir, err := parityPlanAndRunDir(ParityOptions{RunDir: opts.RunDir})
	if err != nil {
		return nil, "", err
	}
	return plan, runDir, nil
}

func validatePromotionGeneratedRoot(generatedRoot string) error {
	info, err := os.Stat(generatedRoot)
	if err != nil {
		return fmt.Errorf("inspect generated root: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("generated root is not a directory: %s", generatedRoot)
	}
	required := []string{
		"dotfiles-manager.v2.yaml",
		filepath.Join("profiles", "stacks", "legacy.yaml"),
		filepath.Join("profiles", "layers", "legacy.yaml"),
		filepath.Join("recipes", "local", recipe.CustomFilesTarget, "recipe.yaml"),
	}
	for _, rel := range required {
		path := filepath.Join(generatedRoot, rel)
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("inspect generated migration output %s: %w", path, err)
		}
	}
	return nil
}

func buildPromotionItem(item Item) PromotionItem {
	promotionItem := basePromotionItem(item)
	if item.Result == "blocked" {
		promotionItem.Result = PromotionResultBlocked
		promotionItem.Diagnostics = append(promotionItem.Diagnostics, item.Diagnostics...)
		promotionItem.Diagnostics = append(promotionItem.Diagnostics, promotionDiagnostic(
			"promotion-migration-item-blocked",
			"migration item is blocked; promotion preview cannot evaluate it",
			item.ExpandedSourcePath,
			"error",
		))
		return promotionItem
	}
	if isGitConfigPromotionCandidate(item) {
		promotionItem.CandidateID = gitConfigCandidateID
		promotionItem.PromotionRuleID = gitConfigRuleID
		promotionItem.ProposedTargetRef = "git"
		promotionItem.ProposedSettingRef = "git:identity"
		promotionItem.Result = PromotionResultBlocked
		promotionItem.Diagnostics = append(promotionItem.Diagnostics, promotionDiagnostic(
			"promotion-target-unimplemented",
			"git config promotion is recognized but blocked until a bundled git recipe and structured git config driver are implemented",
			item.LegacyTarget,
			"error",
		))
		return promotionItem
	}
	promotionItem.Result = PromotionResultNotPromotable
	promotionItem.Diagnostics = append(promotionItem.Diagnostics, promotionDiagnostic(
		"promotion-rule-not-allowlisted",
		"no allowlisted promotion rule exists for this generated custom.files entry",
		item.LegacyTarget,
		"info",
	))
	return promotionItem
}

func basePromotionItem(item Item) PromotionItem {
	return PromotionItem{
		SyncIndex:              item.SyncIndex,
		SyncRef:                item.SyncRef,
		CurrentTargetRef:       item.TargetRef,
		CurrentSettingRef:      item.SettingRef,
		LegacySource:           item.LegacySource,
		LegacyTarget:           item.LegacyTarget,
		Driver:                 item.Driver,
		LocationID:             item.LocationID,
		LocationDefault:        item.LocationDefault,
		ResourceID:             item.ResourceID,
		ResourcePath:           item.ResourcePath,
		DesiredArtifactBinding: item.DesiredArtifactBinding,
		Result:                 PromotionResultBlocked,
	}
}

func isGitConfigPromotionCandidate(item Item) bool {
	if item.TargetRef != recipe.CustomFilesTarget {
		return false
	}
	if item.Driver != recipe.FileDriverID {
		return false
	}
	return strings.TrimSpace(item.LocationDefault) == "~" && filepath.ToSlash(filepath.Clean(item.ResourcePath)) == ".gitconfig"
}

func summarizePromotion(items []PromotionItem) PromotionSummary {
	summary := PromotionSummary{Syncs: len(items)}
	for _, item := range items {
		switch item.Result {
		case PromotionResultPromotable:
			summary.Promotable++
		case PromotionResultBlocked:
			summary.Blocked++
		case PromotionResultNotPromotable:
			summary.NotPromotable++
		}
		if item.CandidateID == gitConfigCandidateID {
			summary.GitConfigCandidates++
		}
	}
	if summary.Blocked > 0 {
		summary.Status = "blocked"
	} else {
		summary.Status = "ok"
	}
	return summary
}

func promotionDiagnostic(code string, message string, diagnosticPath string, severity string) Diagnostic {
	return Diagnostic{Code: code, Severity: severity, Message: message, Path: diagnosticPath}
}
