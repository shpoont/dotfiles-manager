package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	v2ledger "github.com/shpoont/dotfiles-manager/internal/v2/ledger"
	v2preview "github.com/shpoont/dotfiles-manager/internal/v2/preview"
	v2recipe "github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	v2resolution "github.com/shpoont/dotfiles-manager/internal/v2/resolution"
	"github.com/spf13/cobra"
)

const (
	backupReportSchema = "dotfiles-manager.v2.backup-report"
)

type backupReport struct {
	Schema        string              `json:"schema"`
	SchemaVersion int                 `json:"schemaVersion"`
	Command       string              `json:"command"`
	Summary       backupReportSummary `json:"summary"`
	Backups       []backupView        `json:"backups"`
	Error         *backupErrorObject  `json:"error,omitempty"`
}

type backupReportSummary struct {
	Status      string `json:"status"`
	Backups     int    `json:"backups"`
	Items       int    `json:"items"`
	Compatible  int    `json:"compatible"`
	Unsupported int    `json:"unsupported"`
}

type backupView struct {
	RunID         string           `json:"runId"`
	CreatedAt     string           `json:"createdAt"`
	ItemCount     int              `json:"itemCount"`
	RestoreStatus string           `json:"restoreStatus"`
	Items         []backupItemView `json:"items"`
}

type backupItemView struct {
	Ref            string                        `json:"ref"`
	TargetRef      string                        `json:"targetRef"`
	SettingRef     string                        `json:"settingRef"`
	ResourceID     string                        `json:"resourceId"`
	Driver         string                        `json:"driver"`
	DriverVersion  string                        `json:"driverVersion"`
	LivePath       string                        `json:"livePath"`
	PayloadRelPath string                        `json:"payloadRelPath,omitempty"`
	CreatedAt      string                        `json:"createdAt"`
	Before         v2ledger.NormalizedState      `json:"before"`
	Restore        v2ledger.RestoreCompatibility `json:"restore"`
}

type backupErrorObject struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type backupCLIError struct {
	code    string
	message string
	exit    int
}

func (e *backupCLIError) Error() string {
	if e == nil {
		return ""
	}
	return e.message
}

func (e *backupCLIError) ExitCode() int {
	if e == nil || e.exit == 0 {
		return 1
	}
	return e.exit
}

func newBackupCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Inspect v2 local backups",
	}
	cmd.AddCommand(newBackupListCmd(opts))
	cmd.AddCommand(newBackupShowCmd(opts))
	return cmd
}

func newBackupListCmd(opts *rootOptions) *cobra.Command {
	var jsonOutput bool
	var verbose bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List v2 local backups",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBackupListCommand(cmd, opts, jsonOutput, verbose)
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit machine-readable JSON output")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Emit human-readable technical details in text output")
	return cmd
}

func newBackupShowCmd(opts *rootOptions) *cobra.Command {
	var jsonOutput bool
	var verbose bool
	cmd := &cobra.Command{
		Use:   "show <run-id>",
		Short: "Show v2 local backup metadata",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBackupShowCommand(cmd, opts, args[0], jsonOutput, verbose)
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit machine-readable JSON output")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Emit human-readable technical details in text output")
	return cmd
}

func newRestoreCmd(opts *rootOptions) *cobra.Command {
	var jsonOutput bool
	var verbose bool
	var dryRun bool
	var yes bool
	var nonInteractive bool
	v2Flags := &selectedPreviewFlagOptions{}

	cmd := &cobra.Command{
		Use:   "restore <run-id>",
		Short: "Preview or restore from a v2 local backup",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRestoreCommand(cmd, opts, commandOptions{
				Name:           "restore",
				PathArg:        args[0],
				JSONOutput:     jsonOutput,
				Verbose:        verbose,
				DryRun:         dryRun,
				Yes:            yes,
				NonInteractive: nonInteractive,
				V2:             selectedPreviewOptionsFromFlags(cmd, v2Flags),
			})
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview restore without writing live state")
	cmd.Flags().BoolVar(&yes, "yes", false, "Confirm restore without interactive prompting")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "Never prompt; fail if restore needs confirmation")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit machine-readable JSON output")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Emit human-readable technical details in text output")
	addSelectedPreviewFlags(cmd, v2Flags)
	return cmd
}

func runBackupListCommand(cmd *cobra.Command, opts *rootOptions, jsonOutput bool, verbose bool) error {
	store, err := backupStore(opts)
	if err != nil {
		return emitBackupError(cmd.OutOrStdout(), jsonOutput, verbose, "backup.root", "backup.list", err, v2preview.ExitValidation)
	}
	backups, err := store.ListBackups()
	if err != nil {
		return emitBackupError(cmd.OutOrStdout(), jsonOutput, verbose, "backup.list", "backup.list", err, v2preview.ExitInternalError)
	}
	report := backupReportFromMetadata("backup.list", backups)
	return emitBackupReport(cmd.OutOrStdout(), report, jsonOutput, verbose)
}

func runBackupShowCommand(cmd *cobra.Command, opts *rootOptions, runID string, jsonOutput bool, verbose bool) error {
	store, err := backupStore(opts)
	if err != nil {
		return emitBackupError(cmd.OutOrStdout(), jsonOutput, verbose, "backup.root", "backup.show", err, v2preview.ExitValidation)
	}
	backup, err := store.ReadBackup(runID)
	if err != nil {
		return emitBackupError(cmd.OutOrStdout(), jsonOutput, verbose, "backup.show", "backup.show", err, v2preview.ExitValidation)
	}
	report := backupReportFromMetadata("backup.show", []v2ledger.BackupMetadata{backup})
	return emitBackupReport(cmd.OutOrStdout(), report, jsonOutput, verbose)
}

func runRestoreCommand(cmd *cobra.Command, opts *rootOptions, commandOpts commandOptions) error {
	repoRoot, err := backupRestoreRepoRoot(opts, "restore")
	if err != nil {
		report := restoreErrorEnvelope(commandOpts, "restore.root.notFound", err.Error(), v2preview.ExitValidation)
		_ = emitRestoreReport(cmd.OutOrStdout(), report, nil, commandOpts.PathArg, commandOpts.JSONOutput, commandOpts.Verbose, false)
		return &backupCLIError{code: "restore.root.notFound", message: err.Error(), exit: v2preview.ExitValidation}
	}
	stateRoot, err := v2ledger.DefaultStateRoot(repoRoot)
	if err != nil {
		report := restoreErrorEnvelope(commandOpts, "restore.stateRoot.default", err.Error(), v2preview.ExitValidation)
		_ = emitRestoreReport(cmd.OutOrStdout(), report, nil, commandOpts.PathArg, commandOpts.JSONOutput, commandOpts.Verbose, false)
		return &backupCLIError{code: "restore.stateRoot.default", message: err.Error(), exit: v2preview.ExitValidation}
	}
	profile, err := v2resolution.Resolve(repoRoot, v2resolution.ResolveOptions{
		MachineID:   commandOpts.V2.MachineID,
		UserID:      commandOpts.V2.UserID,
		ExtraLayers: commandOpts.V2.Profiles,
	})
	if err != nil {
		report := restoreErrorEnvelope(commandOpts, "restore.profile.resolve", err.Error(), v2preview.ExitValidation)
		_ = emitRestoreReport(cmd.OutOrStdout(), report, nil, commandOpts.PathArg, commandOpts.JSONOutput, commandOpts.Verbose, false)
		return &backupCLIError{code: "restore.profile.resolve", message: err.Error(), exit: v2preview.ExitValidation}
	}
	store, err := v2ledger.NewStore(stateRoot)
	if err != nil {
		report := restoreErrorEnvelope(commandOpts, "restore.ledger.open", err.Error(), v2preview.ExitValidation)
		_ = emitRestoreReport(cmd.OutOrStdout(), report, nil, commandOpts.PathArg, commandOpts.JSONOutput, commandOpts.Verbose, false)
		return &backupCLIError{code: "restore.ledger.open", message: err.Error(), exit: v2preview.ExitValidation}
	}
	started := time.Now().UTC()
	restoreRun, restoreErr := store.Restore(v2ledger.RestoreOptions{
		SourceRunID:    commandOpts.PathArg,
		RunID:          restoreRunID(started),
		Profile:        profile,
		DryRun:         commandOpts.DryRun,
		Confirmed:      commandOpts.Yes,
		NonInteractive: commandOpts.NonInteractive || commandOpts.JSONOutput,
		StartedAt:      started,
	})
	if restoreRun == nil || restoreRun.Preview.Command == "" {
		report := restoreErrorEnvelope(commandOpts, "restore.failed", defaultBackupString(restoreErrString(restoreErr), "restore failed"), v2preview.ExitValidation)
		_ = emitRestoreReport(cmd.OutOrStdout(), report, restoreRun, commandOpts.PathArg, commandOpts.JSONOutput, commandOpts.Verbose, false)
		return &backupCLIError{code: "restore.failed", message: restoreErrString(restoreErr), exit: v2preview.ExitValidation}
	}
	outputEnvelope := restoreRun.Preview
	completed := false
	var completedErr error
	if restoreErr == nil && commandOpts.Yes && !commandOpts.DryRun && restoreRun.RunRecord != nil {
		outputEnvelope, completedErr = completedRestoreEnvelope(restoreRun)
		completed = true
	}
	if emitErr := emitRestoreReport(cmd.OutOrStdout(), outputEnvelope, restoreRun, commandOpts.PathArg, commandOpts.JSONOutput, commandOpts.Verbose, completed); emitErr != nil {
		return emitErr
	}
	if completedErr != nil {
		return &backupCLIError{code: "restore.backupBeforeRestore.missing", message: completedErr.Error(), exit: v2preview.ExitInternalError}
	}
	if restoreErr != nil {
		return &backupCLIError{code: "restore.failed", message: restoreErr.Error(), exit: restoreExitCode(restoreRun.Preview)}
	}
	return nil
}

func defaultBackupString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func backupStore(opts *rootOptions) (*v2ledger.Store, error) {
	repoRoot, err := backupRestoreRepoRoot(opts, "backup")
	if err != nil {
		return nil, err
	}
	stateRoot, err := v2ledger.DefaultStateRoot(repoRoot)
	if err != nil {
		return nil, err
	}
	return v2ledger.NewStore(stateRoot)
}

func backupRestoreRepoRoot(opts *rootOptions, command string) (string, error) {
	if opts != nil && strings.TrimSpace(opts.configPath) != "" {
		if !isExplicitV2Config(opts.configPath) {
			return "", fmt.Errorf("--config for v2 %s must point to %s", command, v2resolution.RootConfigFile)
		}
		return repoRootFromExplicitV2Config(opts.configPath)
	}
	return v2resolution.FindRoot("")
}

func backupReportFromMetadata(command string, backups []v2ledger.BackupMetadata) backupReport {
	report := backupReport{Schema: backupReportSchema, SchemaVersion: 1, Command: command}
	for _, backup := range backups {
		view := backupViewFromMetadata(backup)
		report.Backups = append(report.Backups, view)
		report.Summary.Backups++
		report.Summary.Items += view.ItemCount
		for _, item := range view.Items {
			if item.Restore.Compatible && restoreDriverSupported(item.Driver) {
				report.Summary.Compatible++
			} else {
				report.Summary.Unsupported++
			}
		}
	}
	report.Summary.Status = "ok"
	return report
}

func backupViewFromMetadata(metadata v2ledger.BackupMetadata) backupView {
	metadata = v2ledger.NormalizeBackupMetadata(metadata)
	view := backupView{RunID: metadata.RunID, CreatedAt: metadata.CreatedAt, ItemCount: len(metadata.Items), RestoreStatus: "restorable"}
	if len(metadata.Items) == 0 {
		view.RestoreStatus = "empty"
	}
	for _, item := range metadata.Items {
		itemView := backupItemViewFromMetadata(item)
		if !itemView.Restore.Compatible || !restoreDriverSupported(itemView.Driver) {
			view.RestoreStatus = "blocked"
		}
		view.Items = append(view.Items, itemView)
	}
	return view
}

func backupItemViewFromMetadata(item v2ledger.BackupItem) backupItemView {
	item = v2ledger.NormalizeBackupItem(item)
	return backupItemView{
		Ref:            item.Ref,
		TargetRef:      item.TargetRef,
		SettingRef:     item.SettingRef,
		ResourceID:     item.ResourceID,
		Driver:         item.Driver,
		DriverVersion:  item.DriverVersion,
		LivePath:       redactDisplayPath(item.LivePath),
		PayloadRelPath: item.PayloadRelPath,
		CreatedAt:      item.CreatedAt,
		Before:         item.Before,
		Restore:        item.Restore,
	}
}

func restoreDriverSupported(driver string) bool {
	switch driver {
	case v2recipe.FileDriverID, v2recipe.FileTreeDriverID, v2recipe.IniFileDriverID, v2recipe.JSONFileDriverID, v2recipe.YAMLFileDriverID, v2recipe.TOMLFileDriverID, v2recipe.PlistFileDriverID:
		return true
	default:
		return false
	}
}

func emitBackupReport(stdout io.Writer, report backupReport, jsonOutput bool, verbose bool) error {
	if jsonOutput {
		return emitJSON(stdout, report)
	}
	if verbose {
		_, err := fmt.Fprint(stdout, backupReportVerboseText(report))
		return err
	}
	_, err := fmt.Fprint(stdout, backupReportText(report))
	return err
}

func emitBackupError(stdout io.Writer, jsonOutput bool, verbose bool, code string, command string, err error, exit int) error {
	message := restoreErrString(err)
	report := backupReport{
		Schema:        backupReportSchema,
		SchemaVersion: 1,
		Command:       command,
		Summary:       backupReportSummary{Status: "error"},
		Error:         &backupErrorObject{Code: code, Message: message},
	}
	_ = emitBackupReport(stdout, report, jsonOutput, verbose)
	return &backupCLIError{code: code, message: message, exit: exit}
}

func backupReportText(report backupReport) string {
	var b strings.Builder
	switch report.Command {
	case "backup.show":
		if len(report.Backups) == 1 {
			fmt.Fprintf(&b, "Backup %s\n", report.Backups[0].RunID)
		} else {
			b.WriteString("Backup details\n")
		}
	default:
		b.WriteString("Backups\n")
	}
	if report.Error != nil {
		fmt.Fprintf(&b, "\nCommand result:\n  %s\n", report.Error.Message)
		b.WriteString("\nNo files changed.\nRun with --verbose for technical details.\n")
		return b.String()
	}
	if len(report.Backups) == 0 {
		b.WriteString("\nNo backups found yet.\n\nBackups are created before confirmed apply/restore writes.\n")
		b.WriteString("Use --verbose or --json for technical details.\n")
		return b.String()
	}
	for _, backup := range report.Backups {
		if report.Command != "backup.show" {
			fmt.Fprintf(&b, "\n%s\n", backup.RunID)
		}
		if backup.CreatedAt != "" {
			fmt.Fprintf(&b, "  Created: %s\n", backup.CreatedAt)
		}
		fmt.Fprintf(&b, "  Restorable items: %d\n", backup.ItemCount)
		if len(backup.Items) > 0 {
			b.WriteString("  Can restore:\n")
			for _, item := range backup.Items {
				fmt.Fprintf(&b, "    %s — %s\n", backupItemFriendlyRef(item), backupItemFriendlyLabel(item))
				if item.Restore.Compatible && restoreDriverSupported(item.Driver) {
					b.WriteString("      To the value from before the apply run.\n")
				} else {
					message := strings.TrimSpace(item.Restore.Message)
					if message == "" {
						message = "Restore is not supported for this backup item."
					}
					fmt.Fprintf(&b, "      Cannot restore automatically: %s\n", message)
				}
			}
		}
		fmt.Fprintf(&b, "  Preview restore:\n    dotfiles-manager --config %s restore %s --dry-run\n", v2resolution.RootConfigFile, backup.RunID)
	}
	b.WriteString("\nBackup payload contents are stored for restore but are not printed.\n")
	b.WriteString("Use --verbose for technical details or --json for machine-readable output.\n")
	return b.String()
}

func backupReportVerboseText(report backupReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "dotfiles-manager v2 %s\n", report.Command)
	if report.Error != nil {
		fmt.Fprintf(&b, "Error[%s]: %s\n", report.Error.Code, report.Error.Message)
		return b.String()
	}
	fmt.Fprintf(&b, "Backups: %d (items=%d compatible=%d unsupported=%d)\n", report.Summary.Backups, report.Summary.Items, report.Summary.Compatible, report.Summary.Unsupported)
	for _, backup := range report.Backups {
		fmt.Fprintf(&b, "%s created=%s items=%d restore=%s\n", backup.RunID, backup.CreatedAt, backup.ItemCount, backup.RestoreStatus)
		for _, item := range backup.Items {
			state := "compatible"
			if !item.Restore.Compatible || !restoreDriverSupported(item.Driver) {
				state = "unsupported"
			}
			fmt.Fprintf(&b, "  %s driver=%s resource=%s live=%s restore=%s\n", item.SettingRef, item.Driver, item.ResourceID, item.LivePath, state)
			if item.Restore.Message != "" {
				fmt.Fprintf(&b, "    detail: %s\n", item.Restore.Message)
			}
		}
	}
	b.WriteString("Use --json for technical details. Backup payload contents are never printed.\n")
	return b.String()
}

func backupItemFriendlyRef(item backupItemView) string {
	if strings.TrimSpace(item.SettingRef) != "" {
		return item.SettingRef
	}
	if strings.TrimSpace(item.TargetRef) != "" {
		return item.TargetRef
	}
	return "selected setting"
}

func backupItemFriendlyLabel(item backupItemView) string {
	if strings.TrimSpace(item.SettingRef) != "" {
		return selectedSettingLabelForBackup(item.SettingRef)
	}
	if strings.TrimSpace(item.Ref) != "" {
		return selectedSettingLabelForBackup(item.Ref)
	}
	return "Selected setting"
}

func selectedSettingLabelForBackup(ref string) string {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" {
		return "Selected setting"
	}
	if _, setting, ok := strings.Cut(trimmed, ":"); ok {
		trimmed = setting
	}
	parts := strings.Fields(strings.NewReplacer(".", " ", "-", " ", "_", " ", "/", " ").Replace(trimmed))
	if len(parts) == 0 {
		return "Selected setting"
	}
	for i, part := range parts {
		parts[i] = strings.ToLower(part)
	}
	parts[0] = strings.ToUpper(parts[0][:1]) + parts[0][1:]
	return strings.Join(parts, " ")
}

func emitRestoreReport(stdout io.Writer, envelope v2preview.Envelope, run *v2ledger.RestoreRun, sourceRunID string, jsonOutput bool, verbose bool, completed bool) error {
	envelope = redactPreviewEnvelope(envelope)
	if jsonOutput {
		payload, err := v2preview.JSON(envelope)
		if err != nil {
			return err
		}
		_, err = fmt.Fprint(stdout, payload)
		return err
	}
	_, err := fmt.Fprintln(stdout, restoreReportText(envelope, run, sourceRunID, completed, verbose))
	return err
}

func restoreErrorEnvelope(commandOpts commandOptions, code string, message string, exit int) v2preview.Envelope {
	return v2preview.BuildEnvelope(v2preview.EnvelopeOptions{
		Command: v2preview.CommandRestore,
		Items: []v2preview.Item{{
			Operation: RestoreCommandName(commandOpts.Name),
			DryRun:    commandOpts.DryRun,
			State:     "blocked-safety",
			Result:    v2preview.ResultBlocked,
			Message:   message,
			Backup: v2preview.Backup{
				Policy:  v2preview.BackupSkippedForBlocker,
				Message: "No backup-before-restore was created because restore did not pass validation.",
			},
			Diagnostics: []v2preview.Diagnostic{{
				Code:     code,
				Severity: v2preview.SeverityError,
				Message:  message,
				ExitCode: exit,
			}},
		}},
	})
}

func restoreExitCode(envelope v2preview.Envelope) int {
	code := v2preview.ExitCode(envelope)
	if code == v2preview.ExitChanged {
		return v2preview.ExitSafetyBlocker
	}
	return code
}

func restoreRunID(started time.Time) string {
	return "restore-" + started.UTC().Format("20060102T150405.000000000Z")
}

func restoreErrString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func redactPreviewEnvelope(envelope v2preview.Envelope) v2preview.Envelope {
	envelope = v2preview.NormalizeEnvelope(envelope)
	for i := range envelope.Items {
		envelope.Items[i].LivePath = redactDisplayPath(envelope.Items[i].LivePath)
		envelope.Items[i].DesiredPath = redactDisplayPath(envelope.Items[i].DesiredPath)
		for j := range envelope.Items[i].Diagnostics {
			envelope.Items[i].Diagnostics[j].Path = redactDisplayPath(envelope.Items[i].Diagnostics[j].Path)
		}
	}
	return v2preview.NormalizeEnvelope(envelope)
}

func redactDisplayPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err == nil {
		if redacted, ok := replacePathPrefix(trimmed, home, "$HOME"); ok {
			return redacted
		}
	}
	if strings.HasPrefix(trimmed, "~") {
		return trimmed
	}
	return filepath.Clean(trimmed)
}

func replacePathPrefix(path string, prefix string, replacement string) (string, bool) {
	if strings.TrimSpace(prefix) == "" {
		return path, false
	}
	cleanPath := filepath.Clean(path)
	cleanPrefix := filepath.Clean(prefix)
	if cleanPath == cleanPrefix {
		return replacement, true
	}
	rel, err := filepath.Rel(cleanPrefix, cleanPath)
	if err != nil || rel == "." || strings.HasPrefix(filepath.ToSlash(rel), "../") || filepath.IsAbs(rel) {
		return path, false
	}
	return filepath.ToSlash(filepath.Join(replacement, rel)), true
}

func RestoreCommandName(name string) string {
	if strings.TrimSpace(name) == "" {
		return "restore"
	}
	return strings.TrimSpace(name)
}
