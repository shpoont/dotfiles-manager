package app

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/shpoont/dotfiles-manager/internal/config"
	"github.com/shpoont/dotfiles-manager/internal/dfmerr"
	"github.com/shpoont/dotfiles-manager/internal/logging"
	"github.com/spf13/cobra"
)

type rootOptions struct {
	configPath string
	logFile    string
	logLevel   string
}

var osExit = os.Exit

func NewRootCmd() *cobra.Command {
	opts := &rootOptions{}

	rootCmd := &cobra.Command{
		Use:           "dotfiles-manager",
		Short:         "Config-driven dotfiles synchronization tool",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	rootCmd.Version = currentVersion()
	rootCmd.SetVersionTemplate("dotfiles-manager version {{.Version}}\n")
	rootCmd.Flags().Bool("version", false, "Print version and exit")

	rootCmd.PersistentFlags().StringVar(&opts.configPath, "config", "", "Path to config file")
	rootCmd.PersistentFlags().StringVar(&opts.logFile, "log-file", "", "Path to log file")
	rootCmd.PersistentFlags().StringVar(&opts.logLevel, "log-level", "info", "Log level: debug|info|warn|error")

	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(newStatusCmd(opts))
	rootCmd.AddCommand(newDeployCmd(opts))
	rootCmd.AddCommand(newImportCmd(opts))

	return rootCmd
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), versionLine())
			return nil
		},
	}
}

func newStatusCmd(opts *rootOptions) *cobra.Command {
	var jsonOutput bool
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "status [path]",
		Short: "Show drift and candidate operations",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCommand(cmd, opts, commandOptions{
				Name:       "status",
				PathArg:    firstArg(args),
				JSONOutput: jsonOutput,
				DryRun:     dryRun,
			})
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit machine-readable JSON output")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Unsupported for status (validation error)")
	_ = cmd.Flags().MarkHidden("dry-run")
	return cmd
}

func newDeployCmd(opts *rootOptions) *cobra.Command {
	var jsonOutput bool
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "deploy [path]",
		Short: "Apply source -> target sync",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCommand(cmd, opts, commandOptions{
				Name:       "deploy",
				PathArg:    firstArg(args),
				JSONOutput: jsonOutput,
				DryRun:     dryRun,
			})
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Plan operations without writing files")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit machine-readable JSON output")

	return cmd
}

func newImportCmd(opts *rootOptions) *cobra.Command {
	var jsonOutput bool
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "import [path]",
		Short: "Apply target -> source sync",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCommand(cmd, opts, commandOptions{
				Name:       "import",
				PathArg:    firstArg(args),
				JSONOutput: jsonOutput,
				DryRun:     dryRun,
			})
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Plan operations without writing files")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit machine-readable JSON output")

	return cmd
}

type commandOptions struct {
	Name       string
	PathArg    string
	JSONOutput bool
	DryRun     bool
}

func runCommand(cmd *cobra.Command, opts *rootOptions, commandOpts commandOptions) error {
	pathInput, pathNormalized, pathErr := normalizeScopePath(commandOpts.PathArg)
	if pathErr != nil {
		emitError(cmd.OutOrStdout(), cmd.ErrOrStderr(), commandOpts.JSONOutput, jsonContext{
			Command:        commandOpts.Name,
			DryRun:         commandOpts.DryRun,
			PathInput:      pathInput,
			PathNormalized: pathNormalized,
		}, pathErr)
		return pathErr
	}

	if commandOpts.Name == "status" && commandOpts.DryRun {
		err := dfmerr.New(dfmerr.CodeFlagUnsupported, "Flag not supported for command: --dry-run", map[string]any{"flag": "--dry-run"})
		emitError(cmd.OutOrStdout(), cmd.ErrOrStderr(), commandOpts.JSONOutput, jsonContext{
			Command:        commandOpts.Name,
			DryRun:         commandOpts.DryRun,
			PathInput:      pathInput,
			PathNormalized: pathNormalized,
		}, err)
		return err
	}

	logPath, err := logging.ResolvePath(opts.logFile)
	if err != nil {
		emitError(cmd.OutOrStdout(), cmd.ErrOrStderr(), commandOpts.JSONOutput, jsonContext{
			Command:        commandOpts.Name,
			DryRun:         commandOpts.DryRun,
			PathInput:      pathInput,
			PathNormalized: pathNormalized,
		}, err)
		return err
	}

	logWriter, err := logging.OpenFile(logPath)
	if err != nil {
		emitError(cmd.OutOrStdout(), cmd.ErrOrStderr(), commandOpts.JSONOutput, jsonContext{
			Command:        commandOpts.Name,
			DryRun:         commandOpts.DryRun,
			PathInput:      pathInput,
			PathNormalized: pathNormalized,
		}, err)
		return err
	}
	defer func() {
		_ = logWriter.Close()
	}()

	logger, err := logging.New(opts.logLevel, logWriter)
	if err != nil {
		emitError(cmd.OutOrStdout(), cmd.ErrOrStderr(), commandOpts.JSONOutput, jsonContext{
			Command:        commandOpts.Name,
			DryRun:         commandOpts.DryRun,
			PathInput:      pathInput,
			PathNormalized: pathNormalized,
		}, err)
		return err
	}

	commandLogger := logger.With(
		slog.String("component", "cli"),
		slog.String("command", commandOpts.Name),
		slog.Bool("dry_run", commandOpts.DryRun),
	)
	commandLogger.Debug("log.path", slog.String("log_path", logging.RedactString(logPath)))
	commandLogger.Info("command.start")

	resolvedConfigPath, err := config.ResolvePath(config.ResolveOptions{ExplicitPath: opts.configPath})
	if err != nil {
		logCommandError(commandLogger, err)
		emitError(cmd.OutOrStdout(), cmd.ErrOrStderr(), commandOpts.JSONOutput, jsonContext{
			Command:        commandOpts.Name,
			DryRun:         commandOpts.DryRun,
			PathInput:      pathInput,
			PathNormalized: pathNormalized,
		}, err)
		return err
	}

	absConfigPath, err := filepath.Abs(resolvedConfigPath)
	if err != nil {
		cfgErr := dfmerr.Wrap(dfmerr.CodeIORead, fmt.Sprintf("Read failed: %s", resolvedConfigPath), map[string]any{"path": resolvedConfigPath}, err)
		logCommandError(commandLogger, cfgErr)
		emitError(cmd.OutOrStdout(), cmd.ErrOrStderr(), commandOpts.JSONOutput, jsonContext{
			Command:        commandOpts.Name,
			DryRun:         commandOpts.DryRun,
			PathInput:      pathInput,
			PathNormalized: pathNormalized,
		}, cfgErr)
		return cfgErr
	}
	commandLogger.Debug("config.resolved", slog.String("config_path", logging.RedactString(absConfigPath)))

	cfg, err := config.Load(resolvedConfigPath)
	if err != nil {
		logCommandError(commandLogger, err)
		emitError(cmd.OutOrStdout(), cmd.ErrOrStderr(), commandOpts.JSONOutput, jsonContext{
			Command:        commandOpts.Name,
			DryRun:         commandOpts.DryRun,
			ConfigPath:     absConfigPath,
			PathInput:      pathInput,
			PathNormalized: pathNormalized,
		}, err)
		return err
	}

	selections, err := selectSyncs(cfg, absConfigPath, pathInput, pathNormalized)
	if err != nil {
		logCommandError(commandLogger, err)
		emitError(cmd.OutOrStdout(), cmd.ErrOrStderr(), commandOpts.JSONOutput, jsonContext{
			Command:        commandOpts.Name,
			DryRun:         commandOpts.DryRun,
			ConfigPath:     absConfigPath,
			PathInput:      pathInput,
			PathNormalized: pathNormalized,
		}, err)
		return err
	}
	matchedIndexes := make([]int, 0, len(selections))
	for _, selection := range selections {
		matchedIndexes = append(matchedIndexes, selection.Index)
	}

	result, err := buildSuccessEnvelope(commandOpts, cfg, absConfigPath, pathInput, pathNormalized, matchedIndexes, selections)
	if err != nil {
		logCommandError(commandLogger, err)
		partialSyncs, partialSummary := extractPartialResult(err)
		emitError(cmd.OutOrStdout(), cmd.ErrOrStderr(), commandOpts.JSONOutput, jsonContext{
			Command:            commandOpts.Name,
			DryRun:             commandOpts.DryRun,
			ConfigPath:         absConfigPath,
			PathInput:          pathInput,
			PathNormalized:     pathNormalized,
			MatchedSyncIndexes: matchedIndexes,
			Syncs:              partialSyncs,
			Summary:            partialSummary,
		}, err)
		return err
	}

	if commandOpts.JSONOutput {
		if err := emitJSON(cmd.OutOrStdout(), result); err != nil {
			logCommandError(commandLogger, err)
			return err
		}
	} else {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), buildTextOutput(commandOpts.Name, commandOpts.DryRun, result))
	}

	commandLogger.Info("command.complete")
	return nil
}

func logCommandError(logger *slog.Logger, err error) {
	if logger == nil || err == nil {
		return
	}

	dfmCode := ""
	if dfmError, ok := dfmerr.As(err); ok {
		dfmCode = string(dfmError.Code)
	}

	logger.Error(
		"command.error",
		slog.String("error_code", dfmCode),
		slog.String("error_message", logging.RedactString(err.Error())),
	)
}

type jsonContext struct {
	Command            string
	DryRun             bool
	ConfigPath         any
	PathInput          any
	PathNormalized     any
	MatchedSyncIndexes []int
	Syncs              []any
	Summary            map[string]any
}

func emitError(stdout io.Writer, stderr io.Writer, jsonOutput bool, ctx jsonContext, err error) {
	_, _ = fmt.Fprintln(stderr, err.Error())

	if !jsonOutput {
		return
	}

	dfmError, ok := dfmerr.As(err)
	if !ok {
		dfmError = &dfmerr.Error{Code: "", Message: err.Error()}
	}

	summaryPayload := errorSummary(err)
	if ctx.Summary != nil {
		summaryPayload = ctx.Summary
		if errorIsPartial(err) {
			summaryPayload["partial"] = true
		}
	}
	syncPayload := []any{}
	if ctx.Syncs != nil {
		syncPayload = ctx.Syncs
	}

	payload := map[string]any{
		"schema_version": jsonSchemaVersion,
		"ok":             false,
		"dry_run":        ctx.DryRun,
		"command":        ctx.Command,
		"config_path":    ctx.ConfigPath,
		"path_scope": map[string]any{
			"input":                ctx.PathInput,
			"normalized":           ctx.PathNormalized,
			"matched_sync_indexes": sliceOrEmpty(ctx.MatchedSyncIndexes),
		},
		"syncs":   syncPayload,
		"summary": summaryPayload,
		"error": map[string]any{
			"code":    dfmError.Code,
			"message": dfmError.Message,
		},
	}
	if len(dfmError.Details) > 0 {
		payload["error"].(map[string]any)["details"] = dfmError.Details
	}

	_ = emitJSON(stdout, payload)
}

func emitJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}

func buildSuccessEnvelope(commandOpts commandOptions, cfg *config.Config, configPath string, pathInput any, pathNormalized any, matchedIndexes []int, selections []syncSelection) (map[string]any, error) {
	var (
		syncPayloads []any
		summary      map[string]any
		err          error
	)

	if commandOpts.Name == "status" {
		syncPayloads, summary, err = buildStatusSyncPayloads(cfg, selections)
		if err != nil {
			return nil, err
		}
	} else if commandOpts.Name == "deploy" {
		syncPayloads, summary, err = buildDeploySyncPayloads(cfg, selections, commandOpts.DryRun)
		if err != nil {
			return nil, err
		}
	} else if commandOpts.Name == "import" {
		syncPayloads, summary, err = buildImportSyncPayloads(cfg, selections, commandOpts.DryRun)
		if err != nil {
			return nil, err
		}
	} else {
		syncPayloads = buildSyncPayloads(commandOpts.Name, selections)
		summary = buildSummary(commandOpts.Name, len(syncPayloads))
	}

	return map[string]any{
		"schema_version": jsonSchemaVersion,
		"ok":             true,
		"dry_run":        commandOpts.DryRun,
		"command":        commandOpts.Name,
		"config_path":    configPath,
		"path_scope": map[string]any{
			"input":                pathInput,
			"normalized":           pathNormalized,
			"matched_sync_indexes": matchedIndexes,
		},
		"syncs":   syncPayloads,
		"summary": summary,
		"error":   nil,
	}, nil
}

func buildSyncPayloads(command string, selections []syncSelection) []any {
	out := make([]any, 0, len(selections))

	for _, selection := range selections {
		syncIndex := selection.Index
		sourceRoot := selection.SourceRoot
		targetRoot := selection.TargetRoot
		scopePrefix := selection.ScopePrefix

		basePayload := map[string]any{
			"sync_index":   syncIndex,
			"sync":         fmt.Sprintf("sync[%d]", syncIndex),
			"target":       targetRoot,
			"source":       sourceRoot,
			"source_root":  sourceRoot,
			"target_root":  targetRoot,
			"scope_prefix": scopePrefix,
		}

		switch command {
		case "status":
			basePayload["operations"] = []any{}
			basePayload["counts"] = map[string]any{
				"deploy":             0,
				"import":             0,
				"incoming_unmanaged": 0,
				"remove_unmanaged":   0,
				"remove_missing":     0,
				"operation_count":    0,
			}
			out = append(out, basePayload)
		case "deploy":
			basePayload["operations"] = []any{}
			basePayload["counts"] = map[string]any{
				"copy":             0,
				"remove_unmanaged": 0,
				"operation_count":  0,
			}
			out = append(out, basePayload)
		case "import":
			basePayload["operations"] = []any{}
			basePayload["counts"] = map[string]any{
				"update_managed":  0,
				"add_unmanaged":   0,
				"remove_missing":  0,
				"operation_count": 0,
			}
			out = append(out, basePayload)
		default:
			out = append(out, basePayload)
		}
	}
	return out
}

type syncSelection struct {
	Index       int
	SourceRoot  string
	TargetRoot  string
	ScopePrefix string
}

func selectSyncs(cfg *config.Config, configPath string, pathInput any, normalizedPath any) ([]syncSelection, error) {
	home, _ := os.UserHomeDir()
	configDir := filepath.Dir(configPath)
	selections := make([]syncSelection, 0, len(cfg.Syncs))
	pathValue, hasScope := normalizedPath.(string)

	for idx, syncCfg := range cfg.Syncs {
		targetRoot := filepath.Clean(filepath.Join(home, syncCfg.Target))
		sourceRoot := filepath.Clean(filepath.Join(configDir, syncCfg.Source))

		if !hasScope {
			selections = append(selections, syncSelection{
				Index:       idx,
				SourceRoot:  sourceRoot,
				TargetRoot:  targetRoot,
				ScopePrefix: "",
			})
			continue
		}

		if !isWithinTarget(pathValue, targetRoot) {
			continue
		}

		scopePrefix := ""
		if rel, err := filepath.Rel(targetRoot, pathValue); err == nil && rel != "." {
			scopePrefix = filepath.ToSlash(rel)
		}

		selections = append(selections, syncSelection{
			Index:       idx,
			SourceRoot:  sourceRoot,
			TargetRoot:  targetRoot,
			ScopePrefix: scopePrefix,
		})
	}

	if hasScope && len(selections) == 0 {
		inputDetail := pathValue
		if v, ok := pathInput.(string); ok && v != "" {
			inputDetail = v
		}
		return nil, dfmerr.New(dfmerr.CodeScopeNoMatch, "No sync matched provided path", map[string]any{"input_path": inputDetail})
	}

	return selections, nil
}

func isWithinTarget(scopePath, targetRoot string) bool {
	scope := filepath.Clean(scopePath)
	target := filepath.Clean(targetRoot)
	if scope == target {
		return true
	}
	return strings.HasPrefix(scope, target+string(os.PathSeparator))
}

func buildSummary(command string, syncCount int) map[string]any {
	switch command {
	case "deploy":
		return map[string]any{
			"sync_count":             syncCount,
			"copy_count":             0,
			"remove_unmanaged_count": 0,
			"operation_count":        0,
		}
	case "import":
		return map[string]any{
			"sync_count":           syncCount,
			"update_managed_count": 0,
			"add_unmanaged_count":  0,
			"remove_missing_count": 0,
			"operation_count":      0,
		}
	case "status":
		return map[string]any{
			"sync_count":               syncCount,
			"deploy_count":             0,
			"import_count":             0,
			"incoming_unmanaged_count": 0,
			"remove_unmanaged_count":   0,
			"remove_missing_count":     0,
			"operation_count":          0,
		}
	default:
		return map[string]any{
			"sync_count": syncCount,
		}
	}
}

func normalizeScopePath(pathArg string) (any, any, error) {
	if strings.TrimSpace(pathArg) == "" {
		return nil, nil, nil
	}

	normalizedInput := strings.TrimSpace(pathArg)
	if normalizedInput == "~" || strings.HasPrefix(normalizedInput, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return normalizedInput, nil, dfmerr.Wrap(dfmerr.CodeScopeInvalidPath, fmt.Sprintf("Invalid path argument: %s", pathArg), map[string]any{"input_path": pathArg}, err)
		}
		normalizedInput = filepath.Join(home, strings.TrimPrefix(normalizedInput, "~/"))
	} else if !filepath.IsAbs(normalizedInput) {
		home, err := os.UserHomeDir()
		if err != nil {
			return normalizedInput, nil, dfmerr.Wrap(dfmerr.CodeScopeInvalidPath, fmt.Sprintf("Invalid path argument: %s", pathArg), map[string]any{"input_path": pathArg}, err)
		}
		normalizedInput = filepath.Join(home, normalizedInput)
	}

	absPath, err := filepath.Abs(normalizedInput)
	if err != nil {
		return pathArg, nil, dfmerr.Wrap(dfmerr.CodeScopeInvalidPath, fmt.Sprintf("Invalid path argument: %s", pathArg), map[string]any{"input_path": pathArg}, err)
	}

	return pathArg, absPath, nil
}

func sliceOrEmpty(values []int) []int {
	if len(values) == 0 {
		return []int{}
	}
	return values
}

func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

func extractPartialResult(err error) ([]any, map[string]any) {
	partial, ok := asPartialCommandError(err)
	if !ok || partial == nil {
		return nil, nil
	}
	return partial.syncs, partial.summary
}

func errorSummary(err error) map[string]any {
	if !errorIsPartial(err) {
		return map[string]any{}
	}
	return map[string]any{
		"partial": true,
	}
}

func errorIsPartial(err error) bool {
	dfmError, ok := dfmerr.As(err)
	if !ok || dfmError.Details == nil {
		return false
	}
	partial, ok := dfmError.Details["partial"].(bool)
	return ok && partial
}

func buildTextSummaryLine(command string, dryRun bool, summaryValue any) string {
	summary, _ := summaryValue.(map[string]any)

	switch command {
	case "status":
		parts := []string{"summary"}
		parts = appendSummaryCount(parts, "deploy", summaryInt(summary, "deploy_count"))
		parts = appendSummaryCount(parts, "import", summaryInt(summary, "import_count"))
		parts = appendSummaryCount(parts, "incoming-unmanaged", summaryInt(summary, "incoming_unmanaged_count"))
		parts = appendSummaryCount(parts, "remove-unmanaged", summaryInt(summary, "remove_unmanaged_count"))
		parts = appendSummaryCount(parts, "remove-missing", summaryInt(summary, "remove_missing_count"))
		return strings.Join(parts, " ")
	case "deploy":
		parts := []string{fmt.Sprintf("summary dry-run=%t", dryRun)}
		parts = appendSummaryCount(parts, "copied", summaryInt(summary, "copy_count"))
		parts = appendSummaryCount(parts, "remove-unmanaged", summaryInt(summary, "remove_unmanaged_count"))
		return strings.Join(parts, " ")
	case "import":
		parts := []string{fmt.Sprintf("summary dry-run=%t", dryRun)}
		parts = appendSummaryCount(parts, "updated-managed", summaryInt(summary, "update_managed_count"))
		parts = appendSummaryCount(parts, "added-unmanaged", summaryInt(summary, "add_unmanaged_count"))
		parts = appendSummaryCount(parts, "removed-missing", summaryInt(summary, "remove_missing_count"))
		return strings.Join(parts, " ")
	default:
		return fmt.Sprintf("summary syncs=%d", summaryInt(summary, "sync_count"))
	}
}

func appendSummaryCount(parts []string, label string, count int) []string {
	if count <= 0 {
		return parts
	}
	return append(parts, fmt.Sprintf("%s=%d", label, count))
}

func summaryInt(summary map[string]any, key string) int {
	if summary == nil {
		return 0
	}
	switch v := summary[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

func Execute() int {
	if err := NewRootCmd().Execute(); err != nil {
		return 1
	}
	return 0
}

func Main() {
	osExit(Execute())
}
