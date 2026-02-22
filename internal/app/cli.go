package app

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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

var (
	executeStdout = io.Writer(os.Stdout)
	executeStderr = io.Writer(os.Stderr)
)

var (
	unknownCommandPattern       = regexp.MustCompile(`^unknown command "([^"]+)"`)
	unknownShorthandFlagPattern = regexp.MustCompile(`^unknown shorthand flag: '([^']+)' in (.+)$`)
)

func NewRootCmd() *cobra.Command {
	opts := &rootOptions{}

	rootCmd := &cobra.Command{
		Use:           "dotfiles-manager",
		Short:         "Config-driven dotfiles synchronization tool",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	rootCmd.Version = currentVersion()
	rootCmd.SetVersionTemplate(versionLine() + "\n")
	rootCmd.Flags().Bool("version", false, "Print version and exit")

	rootCmd.PersistentFlags().StringVar(&opts.configPath, "config", "", "Path to config file")
	rootCmd.PersistentFlags().StringVar(&opts.logFile, "log-file", "", "Path to log file")
	rootCmd.PersistentFlags().StringVar(&opts.logLevel, "log-level", "info", "Log level: debug|info|warn|error")

	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(newStatusCmd(opts))
	rootCmd.AddCommand(newDeployCmd(opts))
	rootCmd.AddCommand(newImportCmd(opts))
	rootCmd.AddCommand(newDiffCmd(opts))

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

func newDiffCmd(opts *rootOptions) *cobra.Command {
	var jsonOutput bool
	var dryRun bool
	var direction string
	var contextLines int
	var includePatch bool

	cmd := &cobra.Command{
		Use:   "diff [path]",
		Short: "Show unified patch previews for candidate changes",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCommand(cmd, opts, commandOptions{
				Name:         "diff",
				PathArg:      firstArg(args),
				JSONOutput:   jsonOutput,
				DryRun:       dryRun,
				Direction:    direction,
				ContextLines: contextLines,
				IncludePatch: includePatch,
			})
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit machine-readable JSON output")
	cmd.Flags().StringVar(&direction, "direction", diffDirectionBoth, "Diff direction: both|deploy|import")
	cmd.Flags().IntVar(&contextLines, "context", diffDefaultContextLines, "Unified diff context lines (>= 0)")
	cmd.Flags().BoolVar(&includePatch, "patch", false, "Include patch body in JSON output")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Unsupported for diff (validation error)")
	_ = cmd.Flags().MarkHidden("dry-run")

	return cmd
}

type commandOptions struct {
	Name         string
	PathArg      string
	JSONOutput   bool
	DryRun       bool
	Direction    string
	ContextLines int
	IncludePatch bool
}

func runCommand(cmd *cobra.Command, opts *rootOptions, commandOpts commandOptions) error {
	pathInput, pathNormalized, pathErr := normalizeScopePath(commandOpts.PathArg)
	configPathForErrors := explicitConfigPath(opts.configPath)
	if pathErr != nil {
		emitError(cmd.OutOrStdout(), cmd.ErrOrStderr(), commandOpts.JSONOutput, jsonContext{
			Command:        commandOpts.Name,
			DryRun:         commandOpts.DryRun,
			ConfigPath:     configPathForErrors,
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
			ConfigPath:     configPathForErrors,
			PathInput:      pathInput,
			PathNormalized: pathNormalized,
		}, err)
		return err
	}
	if commandOpts.Name == "diff" && commandOpts.DryRun {
		err := dfmerr.New(dfmerr.CodeFlagUnsupported, "Flag not supported for command: --dry-run", map[string]any{"flag": "--dry-run"})
		emitError(cmd.OutOrStdout(), cmd.ErrOrStderr(), commandOpts.JSONOutput, jsonContext{
			Command:        commandOpts.Name,
			DryRun:         commandOpts.DryRun,
			ConfigPath:     configPathForErrors,
			PathInput:      pathInput,
			PathNormalized: pathNormalized,
		}, err)
		return err
	}
	if commandOpts.Name == "diff" {
		if !isValidDiffDirection(commandOpts.Direction) {
			err := dfmerr.InvalidFlagValue("--direction", commandOpts.Direction, "both|deploy|import")
			emitError(cmd.OutOrStdout(), cmd.ErrOrStderr(), commandOpts.JSONOutput, jsonContext{
				Command:        commandOpts.Name,
				DryRun:         commandOpts.DryRun,
				ConfigPath:     configPathForErrors,
				PathInput:      pathInput,
				PathNormalized: pathNormalized,
			}, err)
			return err
		}
		if commandOpts.ContextLines < 0 {
			err := dfmerr.InvalidFlagValue("--context", fmt.Sprintf("%d", commandOpts.ContextLines), "integer >= 0")
			emitError(cmd.OutOrStdout(), cmd.ErrOrStderr(), commandOpts.JSONOutput, jsonContext{
				Command:        commandOpts.Name,
				DryRun:         commandOpts.DryRun,
				ConfigPath:     configPathForErrors,
				PathInput:      pathInput,
				PathNormalized: pathNormalized,
			}, err)
			return err
		}
		if commandOpts.IncludePatch && !commandOpts.JSONOutput {
			err := patchRequiresJSONError()
			emitError(cmd.OutOrStdout(), cmd.ErrOrStderr(), commandOpts.JSONOutput, jsonContext{
				Command:        commandOpts.Name,
				DryRun:         commandOpts.DryRun,
				ConfigPath:     configPathForErrors,
				PathInput:      pathInput,
				PathNormalized: pathNormalized,
			}, err)
			return err
		}
	}

	logPath, err := logging.ResolvePath(opts.logFile)
	if err != nil {
		emitError(cmd.OutOrStdout(), cmd.ErrOrStderr(), commandOpts.JSONOutput, jsonContext{
			Command:        commandOpts.Name,
			DryRun:         commandOpts.DryRun,
			ConfigPath:     configPathForErrors,
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
			ConfigPath:     configPathForErrors,
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
			ConfigPath:     configPathForErrors,
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
			ConfigPath:     configPathForErrors,
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
			ConfigPath:     resolvedConfigPath,
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

	excludedSyncCount := 0
	if len(cfg.Syncs) > len(selections) {
		excludedSyncCount = len(cfg.Syncs) - len(selections)
	}

	result, err := buildSuccessEnvelope(commandOpts, cfg, absConfigPath, pathInput, pathNormalized, matchedIndexes, selections, excludedSyncCount)
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
	if !jsonOutput {
		_, _ = fmt.Fprintln(stderr, err.Error())
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

func explicitConfigPath(configPath string) any {
	trimmed := strings.TrimSpace(configPath)
	if trimmed == "" {
		return nil
	}
	return trimmed
}

func patchRequiresJSONError() error {
	return dfmerr.New(dfmerr.CodeFlagUnsupported, "--patch requires --json", map[string]any{
		"flag":           "--patch",
		"required_flags": []string{"--json"},
		"example":        "dotfiles-manager diff --json --patch",
	})
}

func emitJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}

func buildSuccessEnvelope(commandOpts commandOptions, cfg *config.Config, configPath string, pathInput any, pathNormalized any, matchedIndexes []int, selections []syncSelection, excludedSyncCount int) (map[string]any, error) {
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
	} else if commandOpts.Name == "diff" {
		includePatch := !commandOpts.JSONOutput || commandOpts.IncludePatch
		syncPayloads, summary, err = buildDiffSyncPayloads(cfg, selections, commandOpts.Direction, commandOpts.ContextLines, includePatch)
		if err != nil {
			return nil, err
		}
	} else {
		syncPayloads = buildSyncPayloads(commandOpts.Name, selections)
		summary = buildSummary(commandOpts.Name, len(syncPayloads))
	}

	if summary == nil {
		summary = map[string]any{}
	}
	summary["excluded_sync_count"] = excludedSyncCount

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
		case "diff":
			basePayload["operations"] = []any{}
			basePayload["counts"] = map[string]any{
				"deploy":             0,
				"import":             0,
				"incoming_unmanaged": 0,
				"remove_unmanaged":   0,
				"remove_missing":     0,
				"unified_patch":      0,
				"binary":             0,
				"type_change":        0,
				"omitted":            0,
				"operation_count":    0,
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
		targetPath, err := config.ExpandSyncPath(syncCfg.Target, fmt.Sprintf("syncs[%d].target", idx))
		if err != nil {
			return nil, err
		}
		sourcePath, err := config.ExpandSyncPath(syncCfg.Source, fmt.Sprintf("syncs[%d].source", idx))
		if err != nil {
			return nil, err
		}

		targetRoot := filepath.Clean(filepath.Join(home, targetPath))
		sourceRoot := filepath.Clean(filepath.Join(configDir, sourcePath))

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
	case "diff":
		return map[string]any{
			"sync_count":               syncCount,
			"deploy_count":             0,
			"import_count":             0,
			"incoming_unmanaged_count": 0,
			"remove_unmanaged_count":   0,
			"remove_missing_count":     0,
			"unified_patch_count":      0,
			"binary_count":             0,
			"type_change_count":        0,
			"omitted_count":            0,
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
		parts = appendExcludedSyncSummary(parts, summary)
		return strings.Join(parts, " ")
	case "deploy":
		parts := []string{fmt.Sprintf("summary dry-run=%t", dryRun)}
		parts = appendSummaryCount(parts, "copied", summaryInt(summary, "copy_count"))
		parts = appendSummaryCount(parts, "remove-unmanaged", summaryInt(summary, "remove_unmanaged_count"))
		parts = appendExcludedSyncSummary(parts, summary)
		return strings.Join(parts, " ")
	case "import":
		parts := []string{fmt.Sprintf("summary dry-run=%t", dryRun)}
		parts = appendSummaryCount(parts, "updated-managed", summaryInt(summary, "update_managed_count"))
		parts = appendSummaryCount(parts, "added-unmanaged", summaryInt(summary, "add_unmanaged_count"))
		parts = appendSummaryCount(parts, "removed-missing", summaryInt(summary, "remove_missing_count"))
		parts = appendExcludedSyncSummary(parts, summary)
		return strings.Join(parts, " ")
	case "diff":
		parts := []string{"summary"}
		parts = appendSummaryCount(parts, "deploy-diff", summaryInt(summary, "deploy_count"))
		parts = appendSummaryCount(parts, "import-diff", summaryInt(summary, "import_count"))
		parts = appendSummaryCount(parts, "incoming-unmanaged", summaryInt(summary, "incoming_unmanaged_count"))
		parts = appendSummaryCount(parts, "remove-unmanaged", summaryInt(summary, "remove_unmanaged_count"))
		parts = appendSummaryCount(parts, "remove-missing", summaryInt(summary, "remove_missing_count"))
		parts = appendSummaryCount(parts, "unified", summaryInt(summary, "unified_patch_count"))
		parts = appendSummaryCount(parts, "binary", summaryInt(summary, "binary_count"))
		parts = appendSummaryCount(parts, "type-change", summaryInt(summary, "type_change_count"))
		parts = appendSummaryCount(parts, "omitted", summaryInt(summary, "omitted_count"))
		parts = appendExcludedSyncSummary(parts, summary)
		return strings.Join(parts, " ")
	default:
		parts := []string{fmt.Sprintf("summary syncs=%d", summaryInt(summary, "sync_count"))}
		parts = appendExcludedSyncSummary(parts, summary)
		return strings.Join(parts, " ")
	}
}

func appendExcludedSyncSummary(parts []string, summary map[string]any) []string {
	if summary == nil {
		return parts
	}
	if _, exists := summary["excluded_sync_count"]; !exists {
		return parts
	}
	return append(parts, fmt.Sprintf("excluded-syncs=%d", summaryInt(summary, "excluded_sync_count")))
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
	cmd := NewRootCmd()
	cmd.SetOut(executeStdout)
	cmd.SetErr(executeStderr)

	if err := cmd.Execute(); err != nil {
		if parserErr, ok := classifyParserError(err); ok {
			parserCtx := parserErrorContextFromArgs(os.Args[1:])
			emitParserError(cmd.OutOrStdout(), cmd.ErrOrStderr(), parserCtx, parserErr)
		}
		return 1
	}
	return 0
}

type parserErrorContext struct {
	JSONOutput bool
	DryRun     bool
	Command    any
	ConfigPath any
}

func emitParserError(stdout io.Writer, stderr io.Writer, ctx parserErrorContext, parserErr *dfmerr.Error) {
	if parserErr == nil {
		return
	}

	if !ctx.JSONOutput {
		_, _ = fmt.Fprintln(stderr, parserErr.Message)
		return
	}

	payload := map[string]any{
		"schema_version": jsonSchemaVersion,
		"ok":             false,
		"dry_run":        ctx.DryRun,
		"command":        ctx.Command,
		"config_path":    ctx.ConfigPath,
		"path_scope": map[string]any{
			"input":                nil,
			"normalized":           nil,
			"matched_sync_indexes": []int{},
		},
		"syncs":   []any{},
		"summary": map[string]any{},
		"error": map[string]any{
			"code":    parserErr.Code,
			"message": parserErr.Message,
		},
	}
	if len(parserErr.Details) > 0 {
		payload["error"].(map[string]any)["details"] = parserErr.Details
	}

	_ = emitJSON(stdout, payload)
}

func classifyParserError(err error) (*dfmerr.Error, bool) {
	if err == nil {
		return nil, false
	}

	if dfmError, ok := dfmerr.As(err); ok {
		switch dfmError.Code {
		case dfmerr.CodeParserUnknownFlag, dfmerr.CodeParserUnknownCommand, dfmerr.CodeParserArgFailure:
			return dfmError, true
		default:
			return nil, false
		}
	}

	message := strings.TrimSpace(err.Error())
	if message == "" {
		return nil, false
	}

	if flag, ok := parserUnknownFlag(message); ok {
		details := map[string]any{}
		if flag != "" {
			details["flag"] = flag
		}
		if len(details) == 0 {
			details = nil
		}
		return dfmerr.New(dfmerr.CodeParserUnknownFlag, message, details), true
	}

	if command, ok := parserUnknownCommand(message); ok {
		details := map[string]any{}
		if command != "" {
			details["command"] = command
		}
		if len(details) == 0 {
			details = nil
		}
		return dfmerr.New(dfmerr.CodeParserUnknownCommand, message, details), true
	}

	if isParserArgFailure(message) {
		return dfmerr.New(dfmerr.CodeParserArgFailure, message, nil), true
	}

	return nil, false
}

func parserUnknownFlag(message string) (string, bool) {
	const unknownFlagPrefix = "unknown flag: "
	if strings.HasPrefix(message, unknownFlagPrefix) {
		return strings.TrimSpace(strings.TrimPrefix(message, unknownFlagPrefix)), true
	}

	matches := unknownShorthandFlagPattern.FindStringSubmatch(message)
	if len(matches) == 3 {
		flag := strings.TrimSpace(matches[2])
		if flag == "" {
			flag = "-" + matches[1]
		}
		return flag, true
	}

	return "", false
}

func parserUnknownCommand(message string) (string, bool) {
	matches := unknownCommandPattern.FindStringSubmatch(message)
	if len(matches) != 2 {
		return "", false
	}
	return matches[1], true
}

func isParserArgFailure(message string) bool {
	if strings.HasPrefix(message, "flag needs an argument: ") {
		return true
	}

	if strings.HasPrefix(message, "invalid argument ") && strings.Contains(message, " flag") {
		return true
	}

	if strings.Contains(message, "arg(s)") && (strings.Contains(message, "accepts ") || strings.Contains(message, "requires ")) {
		return true
	}

	return false
}

func parserErrorContextFromArgs(args []string) parserErrorContext {
	ctx := parserErrorContext{
		JSONOutput: false,
		DryRun:     false,
		Command:    nil,
		ConfigPath: nil,
	}

	waitingFlagValue := ""
	for _, arg := range args {
		if arg == "--" {
			break
		}

		if waitingFlagValue != "" {
			if waitingFlagValue == "--config" {
				ctx.ConfigPath = arg
			}
			waitingFlagValue = ""
			continue
		}

		if value, ok := parseLongFlag(arg, "--config"); ok {
			if value == nil {
				waitingFlagValue = "--config"
			} else {
				ctx.ConfigPath = *value
			}
			continue
		}

		if value, ok := parseLongFlag(arg, "--json"); ok {
			ctx.JSONOutput = parseBoolFlagValue(value, true)
			continue
		}

		if value, ok := parseLongFlag(arg, "--dry-run"); ok {
			ctx.DryRun = parseBoolFlagValue(value, true)
			continue
		}

		if argRequiresValue(arg) {
			waitingFlagValue = arg
			continue
		}

		if strings.HasPrefix(arg, "-") {
			continue
		}

		if ctx.Command == nil {
			ctx.Command = arg
		}
	}

	return ctx
}

func parseLongFlag(arg string, name string) (*string, bool) {
	if arg == name {
		return nil, true
	}

	prefix := name + "="
	if strings.HasPrefix(arg, prefix) {
		value := strings.TrimPrefix(arg, prefix)
		return &value, true
	}

	return nil, false
}

func parseBoolFlagValue(value *string, defaultValue bool) bool {
	if value == nil {
		return defaultValue
	}

	parsedValue, err := strconv.ParseBool(strings.TrimSpace(*value))
	if err != nil {
		return defaultValue
	}
	return parsedValue
}

func argRequiresValue(arg string) bool {
	switch arg {
	case "--config", "--log-file", "--log-level", "--direction", "--context":
		return true
	default:
		return false
	}
}

func Main() {
	osExit(Execute())
}
