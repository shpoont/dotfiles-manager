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
	logFormat  string
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

	rootCmd.PersistentFlags().StringVar(&opts.configPath, "config", "", "Path to config file")
	rootCmd.PersistentFlags().StringVar(&opts.logFormat, "log-format", "text", "Log format: text|json")
	rootCmd.PersistentFlags().StringVar(&opts.logLevel, "log-level", "info", "Log level: debug|info|warn|error")

	rootCmd.AddCommand(newStatusCmd(opts))
	rootCmd.AddCommand(newDeployCmd(opts))
	rootCmd.AddCommand(newImportCmd(opts))

	return rootCmd
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

	logger, err := logging.New(opts.logFormat, opts.logLevel, cmd.ErrOrStderr())
	if err != nil {
		emitError(cmd.OutOrStdout(), cmd.ErrOrStderr(), commandOpts.JSONOutput, jsonContext{
			Command:        commandOpts.Name,
			DryRun:         commandOpts.DryRun,
			PathInput:      pathInput,
			PathNormalized: pathNormalized,
		}, err)
		return err
	}

	logger.Info("command.start",
		slog.String("command", commandOpts.Name),
		slog.Bool("dry_run", commandOpts.DryRun),
	)

	resolvedConfigPath, err := config.ResolvePath(config.ResolveOptions{ExplicitPath: opts.configPath})
	if err != nil {
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
		emitError(cmd.OutOrStdout(), cmd.ErrOrStderr(), commandOpts.JSONOutput, jsonContext{
			Command:        commandOpts.Name,
			DryRun:         commandOpts.DryRun,
			PathInput:      pathInput,
			PathNormalized: pathNormalized,
		}, cfgErr)
		return cfgErr
	}

	cfg, err := config.Load(resolvedConfigPath)
	if err != nil {
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
		emitError(cmd.OutOrStdout(), cmd.ErrOrStderr(), commandOpts.JSONOutput, jsonContext{
			Command:            commandOpts.Name,
			DryRun:             commandOpts.DryRun,
			ConfigPath:         absConfigPath,
			PathInput:          pathInput,
			PathNormalized:     pathNormalized,
			MatchedSyncIndexes: matchedIndexes,
		}, err)
		return err
	}

	if commandOpts.JSONOutput {
		if err := emitJSON(cmd.OutOrStdout(), result); err != nil {
			return err
		}
	} else {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s (stub): loaded %d sync entries\n", commandOpts.Name, len(cfg.Syncs))
	}

	logger.Info("command.complete",
		slog.String("command", commandOpts.Name),
		slog.Bool("dry_run", commandOpts.DryRun),
	)
	return nil
}

type jsonContext struct {
	Command            string
	DryRun             bool
	ConfigPath         any
	PathInput          any
	PathNormalized     any
	MatchedSyncIndexes []int
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

	payload := map[string]any{
		"schema_version": "1.0",
		"ok":             false,
		"dry_run":        ctx.DryRun,
		"command":        ctx.Command,
		"config_path":    ctx.ConfigPath,
		"path_scope": map[string]any{
			"input":                ctx.PathInput,
			"normalized":           ctx.PathNormalized,
			"matched_sync_indexes": sliceOrEmpty(ctx.MatchedSyncIndexes),
		},
		"syncs":   []any{},
		"summary": map[string]any{},
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
		"schema_version": "1.0",
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

		switch command {
		case "status":
			out = append(out, map[string]any{
				"sync_index":          syncIndex,
				"source_root":         sourceRoot,
				"target_root":         targetRoot,
				"scope_prefix":        scopePrefix,
				"deploy_changes":      []any{},
				"import_changes":      []any{},
				"incoming_unmanaged":  []any{},
				"removable_unmanaged": []any{},
				"removable_missing":   []any{},
			})
		case "deploy":
			out = append(out, map[string]any{
				"sync_index":        syncIndex,
				"source_root":       sourceRoot,
				"target_root":       targetRoot,
				"scope_prefix":      scopePrefix,
				"copied":            []any{},
				"removed_unmanaged": []any{},
			})
		case "import":
			out = append(out, map[string]any{
				"sync_index":       syncIndex,
				"source_root":      sourceRoot,
				"target_root":      targetRoot,
				"scope_prefix":     scopePrefix,
				"updated_manifest": []any{},
				"added_unmanaged":  []any{},
				"removed_missing":  []any{},
			})
		default:
			out = append(out, map[string]any{
				"sync_index":  syncIndex,
				"source_root": sourceRoot,
				"target_root": targetRoot,
			})
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
			"sync_count":              syncCount,
			"copied_count":            0,
			"removed_unmanaged_count": 0,
		}
	case "import":
		return map[string]any{
			"sync_count":             syncCount,
			"updated_manifest_count": 0,
			"added_unmanaged_count":  0,
			"removed_missing_count":  0,
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

func Execute() int {
	if err := NewRootCmd().Execute(); err != nil {
		return 1
	}
	return 0
}

func Main() {
	osExit(Execute())
}
