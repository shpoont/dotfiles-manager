// Package dogfood orchestrates internal, safe v2 readiness checks.
//
// The package is intentionally not wired to a public CLI command. It exists to
// make the first dogfood path deterministic, preview-first, and testable
// without giving users a shortcut that might write to real home-directory
// settings by accident.
package dogfood

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shpoont/dotfiles-manager/internal/v2/customfiles"
	"github.com/shpoont/dotfiles-manager/internal/v2/ledger"
	v2migration "github.com/shpoont/dotfiles-manager/internal/v2/migration"
	"github.com/shpoont/dotfiles-manager/internal/v2/preview"
	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/shpoont/dotfiles-manager/internal/v2/resolution"
)

const (
	Schema        = "dotfiles-manager.v2.dogfood-readiness-report"
	SchemaVersion = 1

	defaultUserID            = "legacy"
	defaultApplyRunPrefix    = "dogfood-apply"
	defaultRestoreRunPrefix  = "dogfood-restore"
	defaultRecoveryRunPrefix = "dogfood-recovery"
)

type LocationRootFunc func(v2migration.Item) (string, error)

type AfterConfirmedApplyFunc func(ItemReport) error

type Options struct {
	ConfigPath          string
	HomeDir             string
	StateRoot           string
	MigrationRunID      string
	UserID              string
	AllowedLiveRoots    []string
	LocationRoot        LocationRootFunc
	ApplyRunIDPrefix    string
	RestoreRunIDPrefix  string
	RecoveryRunIDPrefix string
	StartedAt           time.Time
	AfterConfirmedApply AfterConfirmedApplyFunc
}

type Report struct {
	Schema          string       `json:"schema"`
	SchemaVersion   int          `json:"schemaVersion"`
	ConfigPath      string       `json:"configPath"`
	MigrationRunDir string       `json:"migrationRunDir"`
	GeneratedRoot   string       `json:"generatedRoot"`
	ParityStatus    string       `json:"parityStatus"`
	Items           []ItemReport `json:"items"`
	Summary         Summary      `json:"summary"`
}

type ItemReport struct {
	SyncRef             string   `json:"syncRef"`
	SettingRef          string   `json:"settingRef"`
	Driver              string   `json:"driver"`
	LocationID          string   `json:"locationId"`
	LocationRoot        string   `json:"locationRoot"`
	LivePath            string   `json:"livePath"`
	DesiredPath         string   `json:"desiredPath"`
	ApplyPreviewRunID   string   `json:"applyPreviewRunId"`
	ApplyRunID          string   `json:"applyRunId"`
	RestorePreviewRunID string   `json:"restorePreviewRunId"`
	RestoreRunID        string   `json:"restoreRunId"`
	RecoveryRunID       string   `json:"recoveryRunId,omitempty"`
	ApplyPreviewClean   bool     `json:"applyPreviewClean"`
	RestorePreviewClean bool     `json:"restorePreviewClean"`
	ApplyVerified       bool     `json:"applyVerified"`
	RestoreVerified     bool     `json:"restoreVerified"`
	ApplyBackupRefs     []string `json:"applyBackupRefs,omitempty"`
	RestoreBackupRefs   []string `json:"restoreBackupRefs,omitempty"`
	RecoveryAttempted   bool     `json:"recoveryAttempted,omitempty"`
	RecoverySucceeded   bool     `json:"recoverySucceeded,omitempty"`
	RecoveryError       string   `json:"recoveryError,omitempty"`
}

type Summary struct {
	Status               string `json:"status"`
	Syncs                int    `json:"syncs"`
	ParityOK             int    `json:"parityOk"`
	ApplyPreviewsClean   int    `json:"applyPreviewsClean"`
	RestorePreviewsClean int    `json:"restorePreviewsClean"`
	AppliesVerified      int    `json:"appliesVerified"`
	RestoresVerified     int    `json:"restoresVerified"`
	ApplyBackups         int    `json:"applyBackups"`
	RestoreBackups       int    `json:"restoreBackups"`
	RecoveryAttempts     int    `json:"recoveryAttempts"`
	RecoverySucceeded    int    `json:"recoverySucceeded"`
}

func RunMigrationReadiness(opts Options) (*Report, error) {
	if strings.TrimSpace(opts.ConfigPath) == "" {
		return nil, fmt.Errorf("dogfood config path is required")
	}
	if strings.TrimSpace(opts.HomeDir) == "" {
		return nil, fmt.Errorf("dogfood home dir is required")
	}
	if strings.TrimSpace(opts.StateRoot) == "" {
		return nil, fmt.Errorf("dogfood state root is required")
	}
	if opts.LocationRoot == nil {
		return nil, fmt.Errorf("dogfood location-root mapper is required")
	}
	allowedRoots, err := normalizeAllowedRoots(opts.AllowedLiveRoots)
	if err != nil {
		return nil, err
	}

	plan, err := v2migration.WriteMigrationOutput(v2migration.Options{
		ConfigPath: opts.ConfigPath,
		RunID:      opts.MigrationRunID,
		HomeDir:    opts.HomeDir,
	})
	if err != nil {
		return nil, err
	}
	report := &Report{
		Schema:          Schema,
		SchemaVersion:   SchemaVersion,
		ConfigPath:      plan.ConfigPath,
		MigrationRunDir: plan.OutputDir,
		GeneratedRoot:   filepath.Join(plan.OutputDir, "generated"),
	}

	parity, err := v2migration.BuildParityReport(v2migration.ParityOptions{RunDir: plan.OutputDir, HomeDir: opts.HomeDir})
	if err != nil {
		return report, err
	}
	report.ParityStatus = parity.Summary.Status
	if parity.Summary.Status != "ok" {
		return report, fmt.Errorf("dogfood parity gate blocked: %d blocked item(s)", parity.Summary.Blocked)
	}

	userID := strings.TrimSpace(opts.UserID)
	if userID == "" {
		userID = defaultUserID
	}
	profile, err := resolution.Resolve(report.GeneratedRoot, resolution.ResolveOptions{UserID: userID})
	if err != nil {
		return report, fmt.Errorf("resolve generated dogfood profile: %w", err)
	}
	rec, err := recipe.LoadCustomFiles(report.GeneratedRoot)
	if err != nil {
		return report, fmt.Errorf("load generated dogfood custom.files recipe: %w", err)
	}
	store, err := ledger.NewStore(opts.StateRoot)
	if err != nil {
		return report, err
	}

	clock := stepClock{start: opts.StartedAt}
	for _, migrationItem := range plan.Items {
		if migrationItem.Result != "planned" {
			return report, fmt.Errorf("dogfood migration item %s is not planned: %s", migrationItem.SyncRef, migrationItem.Result)
		}
		itemReport, err := runItem(store, profile, rec, migrationItem, opts, allowedRoots, &clock)
		report.Items = append(report.Items, itemReport)
		if err != nil {
			report.Summary = summarize(report.Items)
			return report, err
		}
	}
	report.Summary = summarize(report.Items)
	return report, nil
}

func runItem(store *ledger.Store, profile *resolution.ResolvedProfile, rec *recipe.Recipe, migrationItem v2migration.Item, opts Options, allowedRoots []string, clock *stepClock) (ItemReport, error) {
	settingKey := runIDKey(migrationItem.SettingID, migrationItem.SyncIndex)
	item := ItemReport{
		SyncRef:             migrationItem.SyncRef,
		SettingRef:          migrationItem.SettingRef,
		Driver:              migrationItem.Driver,
		LocationID:          migrationItem.LocationID,
		ApplyPreviewRunID:   runID(prefix(opts.ApplyRunIDPrefix, defaultApplyRunPrefix), "preview", settingKey),
		ApplyRunID:          runID(prefix(opts.ApplyRunIDPrefix, defaultApplyRunPrefix), settingKey),
		RestorePreviewRunID: runID(prefix(opts.RestoreRunIDPrefix, defaultRestoreRunPrefix), "preview", settingKey),
		RestoreRunID:        runID(prefix(opts.RestoreRunIDPrefix, defaultRestoreRunPrefix), settingKey),
		RecoveryRunID:       runID(prefix(opts.RecoveryRunIDPrefix, defaultRecoveryRunPrefix), settingKey),
	}
	locationRoot, err := opts.LocationRoot(migrationItem)
	if err != nil {
		return item, fmt.Errorf("dogfood location root for %s: %w", migrationItem.SyncRef, err)
	}
	item.LocationRoot = strings.TrimSpace(locationRoot)
	if item.LocationRoot == "" {
		return item, fmt.Errorf("dogfood location root for %s is required", migrationItem.SyncRef)
	}
	locationRoots := map[string]string{migrationItem.LocationID: item.LocationRoot}

	plan, err := customfiles.PlanApply(customfiles.Request{
		Profile:       profile,
		Recipe:        rec,
		SettingRef:    migrationItem.SettingRef,
		LocationRoots: locationRoots,
	})
	if err != nil {
		return item, fmt.Errorf("plan dogfood apply %s: %w", migrationItem.SettingRef, err)
	}
	item.LivePath, item.DesiredPath = liveAndDesiredPaths(plan)
	if err := requireAllowedPath(item.LivePath, allowedRoots); err != nil {
		return item, err
	}

	previewRun, err := store.ExecuteCustomFiles(plan, ledger.CustomFilesExecuteOptions{
		RunID:        item.ApplyPreviewRunID,
		ProfileStack: profile.Layers,
		DryRun:       true,
		StartedAt:    clock.next(),
	})
	if err != nil {
		return item, fmt.Errorf("dogfood apply preview %s: %w", migrationItem.SettingRef, err)
	}
	item.ApplyPreviewClean = previewRun != nil && previewRun.Result != nil && previewRun.Result.DryRun && !previewRun.Result.Mutated && previewRun.RunRecord == nil && len(previewRun.LedgerEntries) == 0 && previewRun.Backup == nil
	if !item.ApplyPreviewClean {
		return item, fmt.Errorf("dogfood apply preview for %s was not clean", migrationItem.SettingRef)
	}

	applied := false
	restored := false
	applyRun, err := store.ExecuteCustomFiles(plan, ledger.CustomFilesExecuteOptions{
		RunID:        item.ApplyRunID,
		ProfileStack: profile.Layers,
		StartedAt:    clock.next(),
	})
	if err != nil {
		return item, fmt.Errorf("dogfood apply %s: %w", migrationItem.SettingRef, err)
	}
	applied = true
	item.ApplyVerified = applyRun != nil && applyRun.RunRecord != nil && applyRun.RunRecord.Status == ledger.RunStatusVerified && applyRun.Result != nil && applyRun.Result.Verified && len(applyRun.LedgerEntries) > 0
	if applyRun != nil && applyRun.RunRecord != nil && len(applyRun.RunRecord.Items) > 0 {
		item.ApplyBackupRefs = append([]string(nil), applyRun.RunRecord.Items[0].BackupRefs...)
	}
	if !item.ApplyVerified || len(item.ApplyBackupRefs) == 0 || applyRun == nil || applyRun.Backup == nil {
		err := fmt.Errorf("dogfood apply evidence for %s is incomplete", migrationItem.SettingRef)
		return recoverAfterApply(store, profile, rec, locationRoots, item, err, opts, clock, applied, restored)
	}

	if opts.AfterConfirmedApply != nil {
		if err := opts.AfterConfirmedApply(item); err != nil {
			return recoverAfterApply(store, profile, rec, locationRoots, item, fmt.Errorf("dogfood after-apply hook for %s: %w", migrationItem.SettingRef, err), opts, clock, applied, restored)
		}
	}

	restorePreview, err := store.RestoreCustomFiles(ledger.CustomFilesRestoreOptions{
		SourceRunID:   item.ApplyRunID,
		RunID:         item.RestorePreviewRunID,
		Profile:       profile,
		Recipe:        rec,
		LocationRoots: locationRoots,
		ProfileStack:  profile.Layers,
		DryRun:        true,
		StartedAt:     clock.next(),
	})
	if err != nil {
		return recoverAfterApply(store, profile, rec, locationRoots, item, fmt.Errorf("dogfood restore preview %s: %w", migrationItem.SettingRef, err), opts, clock, applied, restored)
	}
	item.RestorePreviewClean = restorePreview != nil && restorePreview.RunRecord == nil && len(restorePreview.LedgerEntries) == 0 && restorePreview.BackupBeforeRestore == nil && restorePreview.Preview.Summary.Status != preview.SummaryError
	if !item.RestorePreviewClean {
		err := fmt.Errorf("dogfood restore preview for %s was not clean", migrationItem.SettingRef)
		return recoverAfterApply(store, profile, rec, locationRoots, item, err, opts, clock, applied, restored)
	}

	restoreRun, err := store.RestoreCustomFiles(ledger.CustomFilesRestoreOptions{
		SourceRunID:   item.ApplyRunID,
		RunID:         item.RestoreRunID,
		Profile:       profile,
		Recipe:        rec,
		LocationRoots: locationRoots,
		ProfileStack:  profile.Layers,
		Confirmed:     true,
		StartedAt:     clock.next(),
	})
	if err != nil {
		return recoverAfterApply(store, profile, rec, locationRoots, item, fmt.Errorf("dogfood restore %s: %w", migrationItem.SettingRef, err), opts, clock, applied, restored)
	}
	restored = true
	item.RestoreVerified = restoreRun != nil && restoreRun.RunRecord != nil && restoreRun.RunRecord.Status == ledger.RunStatusVerified && len(restoreRun.LedgerEntries) > 0
	if restoreRun != nil && restoreRun.RunRecord != nil && len(restoreRun.RunRecord.Items) > 0 {
		item.RestoreBackupRefs = append([]string(nil), restoreRun.RunRecord.Items[0].BackupRefs...)
	}
	if !item.RestoreVerified || len(item.RestoreBackupRefs) == 0 || restoreRun == nil || restoreRun.BackupBeforeRestore == nil {
		err := fmt.Errorf("dogfood restore evidence for %s is incomplete", migrationItem.SettingRef)
		return recoverAfterApply(store, profile, rec, locationRoots, item, err, opts, clock, applied, restored)
	}
	return item, nil
}

func recoverAfterApply(store *ledger.Store, profile *resolution.ResolvedProfile, rec *recipe.Recipe, locationRoots map[string]string, item ItemReport, runErr error, opts Options, clock *stepClock, applied bool, restored bool) (ItemReport, error) {
	if !applied || restored {
		return item, runErr
	}
	item.RecoveryAttempted = true
	recovery, recoveryErr := store.RestoreCustomFiles(ledger.CustomFilesRestoreOptions{
		SourceRunID:   item.ApplyRunID,
		RunID:         item.RecoveryRunID,
		Profile:       profile,
		Recipe:        rec,
		LocationRoots: locationRoots,
		ProfileStack:  profile.Layers,
		Confirmed:     true,
		StartedAt:     clock.next(),
	})
	if recoveryErr != nil {
		item.RecoveryError = recoveryErr.Error()
		return item, fmt.Errorf("%w; best-effort dogfood recovery failed: %v", runErr, recoveryErr)
	}
	item.RecoverySucceeded = recovery != nil && recovery.RunRecord != nil && recovery.RunRecord.Status == ledger.RunStatusVerified
	if !item.RecoverySucceeded {
		return item, fmt.Errorf("%w; best-effort dogfood recovery did not verify", runErr)
	}
	return item, runErr
}

func liveAndDesiredPaths(plan *customfiles.Plan) (string, string) {
	if plan.Resource.Driver == recipe.FileTreeDriverID {
		livePath := filepath.Join(plan.TreeLiveTarget.Root, filepath.FromSlash(plan.TreeLiveTarget.RelPath))
		desiredPath := filepath.Join(plan.TreeDesiredTarget.Root, filepath.FromSlash(plan.TreeDesiredTarget.RelPath))
		return livePath, desiredPath
	}
	livePath := filepath.Join(plan.LiveTarget.Root, filepath.FromSlash(plan.LiveTarget.RelPath))
	desiredPath := filepath.Join(plan.DesiredTarget.Root, filepath.FromSlash(plan.DesiredTarget.RelPath))
	return livePath, desiredPath
}

func normalizeAllowedRoots(roots []string) ([]string, error) {
	if len(roots) == 0 {
		return nil, fmt.Errorf("dogfood allowed live roots are required")
	}
	normalized := make([]string, 0, len(roots))
	for _, root := range roots {
		trimmed := strings.TrimSpace(root)
		if trimmed == "" {
			return nil, fmt.Errorf("dogfood allowed live root is required")
		}
		abs, err := filepath.Abs(trimmed)
		if err != nil {
			return nil, fmt.Errorf("resolve dogfood allowed live root %q: %w", root, err)
		}
		abs = filepath.Clean(abs)
		real, err := filepath.EvalSymlinks(abs)
		if err != nil {
			return nil, fmt.Errorf("resolve dogfood allowed live root %q: %w", abs, err)
		}
		normalized = append(normalized, abs)
		real = filepath.Clean(real)
		if real != abs {
			normalized = append(normalized, real)
		}
	}
	return normalized, nil
}

func requireAllowedPath(path string, allowedRoots []string) error {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return fmt.Errorf("dogfood live path is required")
	}
	if strings.ContainsRune(trimmed, '\x00') {
		return fmt.Errorf("dogfood live path contains NUL")
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return fmt.Errorf("resolve dogfood live path %q: %w", path, err)
	}
	cleanPath := filepath.Clean(abs)
	for _, root := range allowedRoots {
		if !sameOrInside(root, cleanPath) {
			continue
		}
		ancestor, err := nearestExistingAncestor(cleanPath)
		if err != nil {
			return err
		}
		realAncestor, err := filepath.EvalSymlinks(ancestor)
		if err != nil {
			return fmt.Errorf("resolve dogfood live path ancestor %q: %w", ancestor, err)
		}
		if !insideAny(allowedRoots, realAncestor) {
			return fmt.Errorf("dogfood live path ancestor resolves outside allowed root: %s", realAncestor)
		}
		return nil
	}
	return fmt.Errorf("dogfood live path is outside allowed roots: %s", cleanPath)
}

func nearestExistingAncestor(path string) (string, error) {
	current := filepath.Clean(path)
	for {
		if _, err := os.Lstat(current); err == nil {
			return current, nil
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("inspect dogfood live path ancestor %q: %w", current, err)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("dogfood live path has no existing ancestor: %s", path)
		}
		current = parent
	}
}

func sameOrInside(root string, path string) bool {
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	if root == path {
		return true
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func insideAny(roots []string, path string) bool {
	for _, root := range roots {
		if sameOrInside(root, path) {
			return true
		}
	}
	return false
}

func prefix(configured string, fallback string) string {
	trimmed := strings.TrimSpace(configured)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

func runID(parts ...string) string {
	var cleaned []string
	for _, part := range parts {
		part = strings.Trim(strings.TrimSpace(part), "-")
		if part != "" {
			cleaned = append(cleaned, part)
		}
	}
	return strings.Join(cleaned, "-")
}

func runIDKey(settingID string, syncIndex int) string {
	key := strings.TrimSpace(settingID)
	if key == "" {
		key = fmt.Sprintf("sync-%d", syncIndex)
	}
	return key
}

func summarize(items []ItemReport) Summary {
	summary := Summary{Syncs: len(items), ParityOK: len(items)}
	for _, item := range items {
		if item.ApplyPreviewClean {
			summary.ApplyPreviewsClean++
		}
		if item.RestorePreviewClean {
			summary.RestorePreviewsClean++
		}
		if item.ApplyVerified {
			summary.AppliesVerified++
		}
		if item.RestoreVerified {
			summary.RestoresVerified++
		}
		if len(item.ApplyBackupRefs) > 0 {
			summary.ApplyBackups++
		}
		if len(item.RestoreBackupRefs) > 0 {
			summary.RestoreBackups++
		}
		if item.RecoveryAttempted {
			summary.RecoveryAttempts++
		}
		if item.RecoverySucceeded {
			summary.RecoverySucceeded++
		}
	}
	if summary.Syncs > 0 &&
		summary.ApplyPreviewsClean == summary.Syncs &&
		summary.RestorePreviewsClean == summary.Syncs &&
		summary.AppliesVerified == summary.Syncs &&
		summary.RestoresVerified == summary.Syncs &&
		summary.ApplyBackups == summary.Syncs &&
		summary.RestoreBackups == summary.Syncs {
		summary.Status = "ok"
	} else {
		summary.Status = "blocked"
	}
	return summary
}

type stepClock struct {
	start time.Time
	step  int
}

func (c *stepClock) next() time.Time {
	if c == nil || c.start.IsZero() {
		return time.Time{}
	}
	next := c.start.Add(time.Duration(c.step) * time.Second)
	c.step++
	return next
}
