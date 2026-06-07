package migration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildPromotionReportClassifiesAllowlistedCandidatesWithoutReadingContents(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	homeRoot := t.TempDir()
	writeFile(t, filepath.Join(repoRoot, "legacy", ".gitconfig"), "[user]\n\temail = secret@example.com\n")
	writeFile(t, filepath.Join(repoRoot, "legacy", "misc.txt"), "secret-token-value\n")
	writeFile(t, filepath.Join(repoRoot, "legacy", "nvim", "init.lua"), "vim.opt.number = true\n")
	writeFile(t, filepath.Join(homeRoot, ".gitconfig"), "live git before\n")
	configPath := writeV1Config(t, repoRoot, `syncs:
  - source: legacy/.gitconfig
    target: .gitconfig
  - source: legacy/misc.txt
    target: .misc
  - source: legacy/nvim
    target: .config/nvim
`)

	plan, err := WriteMigrationOutput(Options{ConfigPath: configPath, HomeDir: homeRoot, RunID: "promotion"})
	require.NoError(t, err)
	runRoot := filepath.Join(repoRoot, "migrations", "v1-to-v2", "promotion")

	report, err := BuildPromotionReport(PromotionOptions{RunDir: runRoot})
	require.NoError(t, err)
	require.Equal(t, PromotionReportSchema, report.Schema)
	require.Equal(t, SchemaVersion, report.SchemaVersion)
	require.Equal(t, plan.RunID, report.RunID)
	require.Equal(t, runRoot, report.MigrationRunDir)
	require.Equal(t, filepath.Join(runRoot, "generated"), report.GeneratedRoot)
	require.Equal(t, PromotionSummary{Syncs: 3, Blocked: 1, NotPromotable: 2, GitConfigCandidates: 1, Status: "blocked"}, report.Summary)
	require.NotNil(t, report.Error)
	require.Equal(t, "promotion-preview-blocked", report.Error.Code)

	gitItem := report.Items[0]
	require.Equal(t, "sync[0]", gitItem.SyncRef)
	require.Equal(t, "custom.files", gitItem.CurrentTargetRef)
	require.Equal(t, "custom.files:sync-0", gitItem.CurrentSettingRef)
	require.Equal(t, ".gitconfig", gitItem.ResourcePath)
	require.Equal(t, gitConfigCandidateID, gitItem.CandidateID)
	require.Equal(t, gitConfigRuleID, gitItem.PromotionRuleID)
	require.Equal(t, "git", gitItem.ProposedTargetRef)
	require.Equal(t, "git:identity", gitItem.ProposedSettingRef)
	require.Equal(t, PromotionResultBlocked, gitItem.Result)
	requireDiagnostic(t, gitItem.Diagnostics, "promotion-confirmed-write-unimplemented")

	miscItem := report.Items[1]
	require.Equal(t, PromotionResultNotPromotable, miscItem.Result)
	require.Empty(t, miscItem.CandidateID)
	requireDiagnostic(t, miscItem.Diagnostics, "promotion-rule-not-allowlisted")

	treeItem := report.Items[2]
	require.Equal(t, "file-tree", treeItem.Driver)
	require.Equal(t, PromotionResultNotPromotable, treeItem.Result)
	requireDiagnostic(t, treeItem.Diagnostics, "promotion-rule-not-allowlisted")

	jsonPayload, err := PromotionJSON(report)
	require.NoError(t, err)
	require.Contains(t, jsonPayload, `"schema": "dotfiles-manager.v2.migration-promotion-preview"`)
	require.NotContains(t, jsonPayload, "secret@example.com")
	require.NotContains(t, jsonPayload, "secret-token-value")
	require.NotContains(t, jsonPayload, "live git before")

	text := PromotionText(report)
	require.Contains(t, text, "migration promotion preview")
	require.Contains(t, text, "PREVIEW ONLY (no active v2 writes, no live file writes)")
	require.Contains(t, text, "proposed promotion: git:identity")
	require.Contains(t, text, "result: blocked")
	require.Contains(t, text, "result: not-promotable")

	require.Equal(t, "live git before\n", string(readFile(t, filepath.Join(homeRoot, ".gitconfig"))))
	require.NoFileExists(t, filepath.Join(repoRoot, "dotfiles-manager.v2.yaml"))
	require.NoDirExists(t, filepath.Join(repoRoot, "profiles"))
	require.NoDirExists(t, filepath.Join(repoRoot, "desired"))
	require.NoDirExists(t, filepath.Join(repoRoot, "recipes"))
}

func TestPromotionReportValidatesGeneratedOutputAndRunDir(t *testing.T) {
	t.Parallel()

	missingRunDir := filepath.Join(t.TempDir(), "missing-run")
	_, err := BuildPromotionReport(PromotionOptions{RunDir: missingRunDir})
	require.Error(t, err)
	require.Contains(t, err.Error(), "read migration plan")

	repoRoot := t.TempDir()
	homeRoot := t.TempDir()
	writeFile(t, filepath.Join(repoRoot, "legacy", ".gitconfig"), "[user]\n\temail = leon@example.com\n")
	configPath := writeV1Config(t, repoRoot, `syncs:
  - source: legacy/.gitconfig
    target: .gitconfig
`)
	plan, err := WriteMigrationOutput(Options{ConfigPath: configPath, HomeDir: homeRoot, RunID: "promotion"})
	require.NoError(t, err)
	runRoot := filepath.Join(repoRoot, "migrations", "v1-to-v2", "promotion")
	require.NoError(t, os.RemoveAll(filepath.Join(runRoot, "generated", "recipes")))

	_, err = BuildPromotionReport(PromotionOptions{Plan: plan, RunDir: runRoot})
	require.Error(t, err)
	require.Contains(t, err.Error(), "inspect generated migration output")
}

func TestPromotionReportHelpersAndErrorReport(t *testing.T) {
	t.Parallel()

	report := NewPromotionErrorReport("missing-run", "DFM_TEST", "test failure", map[string]any{"runDir": "missing-run"})
	require.Equal(t, PromotionReportSchema, report.Schema)
	require.Equal(t, "error", report.Summary.Status)
	require.Equal(t, "DFM_TEST", report.Error.Code)

	jsonPayload, err := PromotionJSON(report)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal([]byte(jsonPayload), &decoded))
	require.Equal(t, "dotfiles-manager.v2.migration-promotion-preview", decoded["schema"])

	_, err = PromotionJSON(NewPromotionErrorReport("bad", "DFM_TEST", "bad details", map[string]any{"notJSON": func() {}}))
	require.Error(t, err)

	require.Equal(t, "migration promotion preview\nsummary syncs=0 promotable=0 blocked=0 not-promotable=0 git-config-candidates=0 status=error", PromotionText(nil))
}

func TestPromotionItemAndSummaryHelperBranches(t *testing.T) {
	t.Parallel()

	blocked := buildPromotionItem(Item{
		SyncIndex:    7,
		SyncRef:      "sync[7]",
		TargetRef:    "custom.files",
		SettingRef:   "custom.files:sync-7",
		Result:       "blocked",
		Diagnostics:  []Diagnostic{{Code: "migration-driver-unknown", Severity: "error", Message: "driver unknown"}},
		LegacySource: "missing",
		LegacyTarget: ".missing",
	})
	require.Equal(t, PromotionResultBlocked, blocked.Result)
	requireDiagnostic(t, blocked.Diagnostics, "migration-driver-unknown")
	requireDiagnostic(t, blocked.Diagnostics, "promotion-migration-item-blocked")

	require.False(t, isGitConfigPromotionCandidate(Item{TargetRef: "git", Driver: "file", LocationDefault: "~", ResourcePath: ".gitconfig"}))
	require.False(t, isGitConfigPromotionCandidate(Item{TargetRef: "custom.files", Driver: "file-tree", LocationDefault: "~", ResourcePath: ".gitconfig"}))
	require.False(t, isGitConfigPromotionCandidate(Item{TargetRef: "custom.files", Driver: "file", LocationDefault: "~/.config", ResourcePath: ".gitconfig"}))
	require.False(t, isGitConfigPromotionCandidate(Item{TargetRef: "custom.files", Driver: "file", LocationDefault: "~", ResourcePath: ".not-gitconfig"}))

	summary := summarizePromotion([]PromotionItem{
		{Result: PromotionResultPromotable, CandidateID: gitConfigCandidateID},
		{Result: PromotionResultNotPromotable},
	})
	require.Equal(t, PromotionSummary{Syncs: 2, Promotable: 1, NotPromotable: 1, GitConfigCandidates: 1, Status: "ok"}, summary)
}

func requireDiagnostic(t *testing.T, diagnostics []Diagnostic, code string) {
	t.Helper()
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return
		}
	}
	require.Failf(t, "diagnostic code missing", "expected %q in %#v", code, diagnostics)
}
