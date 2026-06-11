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
	v2addtarget "github.com/shpoont/dotfiles-manager/internal/v2/addtarget"
	v2guidedsync "github.com/shpoont/dotfiles-manager/internal/v2/guidedsync"
	v2initcmd "github.com/shpoont/dotfiles-manager/internal/v2/initcmd"
	v2ledger "github.com/shpoont/dotfiles-manager/internal/v2/ledger"
	v2lifecycle "github.com/shpoont/dotfiles-manager/internal/v2/lifecycle"
	v2listcmd "github.com/shpoont/dotfiles-manager/internal/v2/listcmd"
	v2migration "github.com/shpoont/dotfiles-manager/internal/v2/migration"
	v2recipe "github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	v2resolution "github.com/shpoont/dotfiles-manager/internal/v2/resolution"
	v2selectedlive "github.com/shpoont/dotfiles-manager/internal/v2/selectedlive"
	v2selectedpreview "github.com/shpoont/dotfiles-manager/internal/v2/selectedpreview"
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
	rootCmd.AddCommand(newSaveCmd(opts))
	rootCmd.AddCommand(newApplyCmd(opts))
	rootCmd.AddCommand(newSyncCmd(opts))
	rootCmd.AddCommand(newInitCmd(opts))
	rootCmd.AddCommand(newAddCmd(opts))
	rootCmd.AddCommand(newListCmd(opts))
	rootCmd.AddCommand(newBackupCmd(opts))
	rootCmd.AddCommand(newRestoreCmd(opts))
	rootCmd.AddCommand(newMigrateCmd(opts))
	rootCmd.AddCommand(newRecipeCmd(opts))

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
	v2Flags := &selectedPreviewFlagOptions{}

	cmd := &cobra.Command{
		Use:   "status [path-or-ref]",
		Short: "Show drift and candidate operations",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCommand(cmd, opts, commandOptions{
				Name:       "status",
				PathArg:    firstArg(args),
				JSONOutput: jsonOutput,
				DryRun:     dryRun,
				V2:         selectedPreviewOptionsFromFlags(cmd, v2Flags),
			})
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit machine-readable JSON output")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Unsupported for status (validation error)")
	_ = cmd.Flags().MarkHidden("dry-run")
	addSelectedPreviewFlags(cmd, v2Flags)
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
	v2Flags := &selectedPreviewFlagOptions{}

	cmd := &cobra.Command{
		Use:   "diff [path-or-ref]",
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
				V2:           selectedPreviewOptionsFromFlags(cmd, v2Flags),
			})
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit machine-readable JSON output")
	cmd.Flags().StringVar(&direction, "direction", diffDirectionBoth, "Diff direction: both|deploy|import")
	cmd.Flags().IntVar(&contextLines, "context", diffDefaultContextLines, "Unified diff context lines (>= 0)")
	cmd.Flags().BoolVar(&includePatch, "patch", false, "Include patch body in JSON output")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Unsupported for diff (validation error)")
	_ = cmd.Flags().MarkHidden("dry-run")
	addSelectedPreviewFlags(cmd, v2Flags)

	return cmd
}

func newSaveCmd(opts *rootOptions) *cobra.Command {
	var jsonOutput bool
	var dryRun bool
	var yes bool
	var nonInteractive bool
	v2Flags := &selectedPreviewFlagOptions{}

	cmd := &cobra.Command{
		Use:   "save [ref]",
		Short: "Preview saving selected v2 settings to desired artifacts",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSelectedPreviewRootCommand(cmd, opts, commandOptions{
				Name:           "save",
				PathArg:        firstArg(args),
				JSONOutput:     jsonOutput,
				DryRun:         dryRun,
				Yes:            yes,
				NonInteractive: nonInteractive,
				V2:             selectedPreviewOptionsFromFlags(cmd, v2Flags),
			})
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview selected-value save without writing desired artifacts")
	cmd.Flags().BoolVar(&yes, "yes", false, "Confirm selected-value live save without interactive prompting")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "Never prompt; fail if selected-value save needs confirmation")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit machine-readable JSON output")
	addSelectedPreviewFlags(cmd, v2Flags)
	return cmd
}

func newApplyCmd(opts *rootOptions) *cobra.Command {
	var jsonOutput bool
	var dryRun bool
	var yes bool
	var nonInteractive bool
	v2Flags := &selectedPreviewFlagOptions{}

	cmd := &cobra.Command{
		Use:   "apply [ref]",
		Short: "Preview applying selected v2 settings to live state",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSelectedPreviewRootCommand(cmd, opts, commandOptions{
				Name:           "apply",
				PathArg:        firstArg(args),
				JSONOutput:     jsonOutput,
				DryRun:         dryRun,
				Yes:            yes,
				NonInteractive: nonInteractive,
				V2:             selectedPreviewOptionsFromFlags(cmd, v2Flags),
			})
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview selected-value apply without writing live state")
	cmd.Flags().BoolVar(&yes, "yes", false, "Confirm selected-value live apply without interactive prompting")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "Never prompt; fail if selected-value apply needs confirmation")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit machine-readable JSON output")
	addSelectedPreviewFlags(cmd, v2Flags)
	return cmd
}

func newSyncCmd(opts *rootOptions) *cobra.Command {
	var jsonOutput bool
	var yes bool
	var nonInteractive bool
	var choiceFlags []string
	v2Flags := &selectedPreviewFlagOptions{}

	cmd := &cobra.Command{
		Use:   "sync [ref]",
		Short: "Guided save/apply/skip flow for selected v2 settings",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGuidedSyncRootCommand(cmd, opts, firstArg(args), guidedSyncCommandOptions{
				JSONOutput:     jsonOutput,
				Yes:            yes,
				NonInteractive: nonInteractive,
				ChoiceFlags:    append([]string(nil), choiceFlags...),
				V2:             selectedPreviewOptionsFromFlags(cmd, v2Flags),
			})
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit machine-readable JSON output")
	cmd.Flags().BoolVar(&yes, "yes", false, "Execute explicit non-interactive choices without prompting")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "Never prompt; fail if execution needs missing choices")
	cmd.Flags().StringArrayVar(&choiceFlags, "choice", nil, "Guided choice in setting-ref=save|apply|skip form (repeatable)")
	addSelectedPreviewFlags(cmd, v2Flags)
	return cmd
}

func newInitCmd(opts *rootOptions) *cobra.Command {
	var jsonOutput bool
	var dryRun bool
	var yes bool
	var nonInteractive bool
	var machineID string
	var userID string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize v2 repository scaffold and local identity state",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInitCommand(cmd, opts, v2initcmd.Options{
				RepoRoot:       initRepoRootFromConfig(opts.configPath),
				MachineID:      machineID,
				UserID:         userID,
				DryRun:         dryRun,
				Yes:            yes,
				NonInteractive: nonInteractive,
				JSONMode:       jsonOutput,
				Input:          cmd.InOrStdin(),
				PromptOutput:   cmd.OutOrStdout(),
			}, jsonOutput)
		},
	}

	cmd.Flags().StringVar(&machineID, "machine-id", "", "v2 machine identity to store when bootstrapping")
	cmd.Flags().StringVar(&userID, "user-id", "", "v2 user identity to store when bootstrapping")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Plan initialization without writing files")
	cmd.Flags().BoolVar(&yes, "yes", false, "Accept safe generated identity candidates without prompting")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "Never prompt; fail if identity input is required")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit machine-readable JSON output")
	return cmd
}

func newAddCmd(opts *rootOptions) *cobra.Command {
	var jsonOutput bool
	var dryRun bool
	var yes bool
	var nonInteractive bool
	var profileLayer string
	var scope string
	var settings []string

	cmd := &cobra.Command{
		Use:   "add <target>",
		Short: "Add a supported v2 target to the active profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAddCommand(cmd, opts, v2addtarget.Options{
				Target:         args[0],
				Settings:       settings,
				Scope:          scope,
				ProfileLayer:   profileLayer,
				DryRun:         dryRun,
				Yes:            yes,
				NonInteractive: nonInteractive,
				JSONMode:       jsonOutput,
				Input:          cmd.InOrStdin(),
				PromptOutput:   cmd.OutOrStdout(),
			}, jsonOutput)
		},
	}

	cmd.Flags().StringArrayVar(&settings, "setting", nil, "Setting id or target:setting ref to add (repeatable or comma-separated)")
	cmd.Flags().StringVar(&scope, "scope", "", "Scope for selected settings: shared|user|machine|machine-user")
	cmd.Flags().StringVar(&profileLayer, "profile", "", "Active profile layer to update")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Plan profile selection changes without writing")
	cmd.Flags().BoolVar(&yes, "yes", false, "Accept safe recipe defaults without prompting")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "Never prompt; fail with missing-choice diagnostics when input is required")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit machine-readable JSON output")
	return cmd
}

func newListCmd(opts *rootOptions) *cobra.Command {
	var jsonOutput bool
	v2Flags := &selectedPreviewFlagOptions{}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List managed v2 settings in the active profile",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runListCommand(cmd, opts, v2listcmd.Options{
				MachineID:   v2Flags.machineID,
				UserID:      v2Flags.userID,
				ExtraLayers: append([]string(nil), v2Flags.profiles...),
			}, jsonOutput)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit machine-readable JSON output")
	addSelectedPreviewFlags(cmd, v2Flags)
	return cmd
}

func newRecipeCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "recipe",
		Short: "Inspect v2 recipe metadata",
	}
	cmd.AddCommand(newRecipeListCmd(opts))
	cmd.AddCommand(newRecipeExplainCmd(opts))
	cmd.AddCommand(newRecipeDiscoverCmd(opts))
	return cmd
}

func newRecipeListCmd(opts *rootOptions) *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List read-only v2 bundled recipe targets",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRecipeListCommand(cmd, opts, jsonOutput)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit machine-readable JSON output")
	return cmd
}

func newRecipeExplainCmd(opts *rootOptions) *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "explain <target>",
		Short: "Explain read-only v2 recipe metadata for a target",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRecipeExplainCommand(cmd, opts, args[0], jsonOutput)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit machine-readable JSON output")
	return cmd
}

func newRecipeDiscoverCmd(opts *rootOptions) *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "discover [target]",
		Short: "Discover read-only install/config state for bundled recipe targets",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRecipeDiscoverCommand(cmd, opts, firstArg(args), jsonOutput)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit machine-readable JSON output")
	return cmd
}

func newMigrateCmd(opts *rootOptions) *cobra.Command {
	var jsonOutput bool
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Preview or generate v1 syncs as v2 custom.files entries",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMigrateCommand(cmd, opts, dryRun, jsonOutput)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview migration without writing files")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit machine-readable JSON output")
	cmd.AddCommand(newMigrateParityCmd())
	cmd.AddCommand(newMigratePromotePreviewCmd())
	return cmd
}

func newMigrateParityCmd() *cobra.Command {
	var runDir string
	var jsonOutput bool
	var yamlOutput bool

	cmd := &cobra.Command{
		Use:   "parity",
		Short: "Check a generated v1-to-v2 migration run for parity",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMigrateParityCommand(cmd, runDir, jsonOutput, yamlOutput)
		},
	}

	cmd.Flags().StringVar(&runDir, "run-dir", "", "Path to a generated migration run directory")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit machine-readable JSON output")
	cmd.Flags().BoolVar(&yamlOutput, "yaml", false, "Emit machine-readable YAML output")
	return cmd
}

func newMigratePromotePreviewCmd() *cobra.Command {
	var runDir string
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "promote-preview",
		Short: "Preview optional promotion from generated custom.files entries to known targets",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMigratePromotePreviewCommand(cmd, runDir, jsonOutput)
		},
	}

	cmd.Flags().StringVar(&runDir, "run-dir", "", "Path to a generated migration run directory")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit machine-readable JSON output")
	return cmd
}

type commandOptions struct {
	Name           string
	PathArg        string
	JSONOutput     bool
	DryRun         bool
	Yes            bool
	NonInteractive bool
	Direction      string
	ContextLines   int
	IncludePatch   bool
	V2             selectedPreviewCommandOptions
}

type selectedPreviewFlagOptions struct {
	machineID string
	userID    string
	profiles  []string
}

type selectedPreviewCommandOptions struct {
	MachineID     string
	UserID        string
	Profiles      []string
	FlagsUsed     bool
	UsedFlagNames []string
}

type guidedSyncCommandOptions struct {
	JSONOutput     bool
	Yes            bool
	NonInteractive bool
	ChoiceFlags    []string
	V2             selectedPreviewCommandOptions
}

func addSelectedPreviewFlags(cmd *cobra.Command, flags *selectedPreviewFlagOptions) {
	cmd.Flags().StringVar(&flags.machineID, "machine-id", "", "v2 machine identity for selected-value preview")
	cmd.Flags().StringVar(&flags.userID, "user-id", "", "v2 user identity for selected-value preview")
	cmd.Flags().StringArrayVar(&flags.profiles, "profile", nil, "additional v2 profile layer for selected-value preview (repeatable)")
}

func selectedPreviewOptionsFromFlags(cmd *cobra.Command, flags *selectedPreviewFlagOptions) selectedPreviewCommandOptions {
	if flags == nil {
		return selectedPreviewCommandOptions{}
	}
	used := make([]string, 0, 3)
	for _, name := range []string{"machine-id", "user-id", "profile"} {
		if cmd.Flags().Changed(name) {
			used = append(used, "--"+name)
		}
	}
	return selectedPreviewCommandOptions{
		MachineID:     flags.machineID,
		UserID:        flags.userID,
		Profiles:      append([]string(nil), flags.profiles...),
		FlagsUsed:     len(used) > 0,
		UsedFlagNames: used,
	}
}

func runCommand(cmd *cobra.Command, opts *rootOptions, commandOpts commandOptions) error {
	if maybeHandled, err := maybeRunSelectedPreviewCommand(cmd, opts, commandOpts); maybeHandled || err != nil {
		return err
	}

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
	if commandOpts.V2.FlagsUsed {
		err := dfmerr.New(dfmerr.CodeFlagUnsupported, "v2 selected-value flags require v2 mode", map[string]any{"flags": commandOpts.V2.UsedFlagNames})
		emitError(cmd.OutOrStdout(), cmd.ErrOrStderr(), commandOpts.JSONOutput, jsonContext{
			Command:        commandOpts.Name,
			DryRun:         commandOpts.DryRun,
			ConfigPath:     configPathForErrors,
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

func maybeRunSelectedPreviewCommand(cmd *cobra.Command, opts *rootOptions, commandOpts commandOptions) (bool, error) {
	if commandOpts.Name != "status" && commandOpts.Name != "diff" {
		return false, nil
	}
	if opts != nil && strings.TrimSpace(opts.configPath) != "" {
		if isExplicitV2Config(opts.configPath) {
			repoRoot, err := repoRootFromExplicitV2Config(opts.configPath)
			if err != nil {
				return true, emitSelectedPreviewError(cmd, commandOpts, "selectedpreview.config.invalid", err.Error(), nil)
			}
			return true, runSelectedPreviewCommand(cmd, commandOpts, repoRoot)
		}
		return false, nil
	}

	_, v1Err := config.ResolvePath(config.ResolveOptions{})
	if v1Err == nil {
		return false, nil
	}
	if dfmerr.MustCode(v1Err) != dfmerr.CodeConfigRequired {
		return false, nil
	}
	repoRoot, err := v2resolution.FindRoot("")
	if err != nil {
		return false, nil
	}
	return true, runSelectedPreviewCommand(cmd, commandOpts, repoRoot)
}

func runSelectedPreviewRootCommand(cmd *cobra.Command, opts *rootOptions, commandOpts commandOptions) error {
	if commandOpts.Name != "save" && commandOpts.Name != "apply" {
		return fmt.Errorf("unsupported selected-value root command: %s", commandOpts.Name)
	}

	repoRoot, err := selectedPreviewRepoRoot(opts)
	if err != nil {
		return emitSelectedPreviewError(cmd, commandOpts, "selectedpreview.root.notFound", err.Error(), nil)
	}
	if commandOpts.DryRun {
		return runSelectedPreviewCommand(cmd, commandOpts, repoRoot)
	}
	stateRoot, err := v2ledger.DefaultStateRoot(repoRoot)
	if err != nil {
		return emitSelectedPreviewError(cmd, commandOpts, "selectedpreview.stateRoot.default", err.Error(), nil)
	}
	result, err := v2selectedlive.Run(v2selectedlive.Options{
		Command:           commandOpts.Name,
		RepoRoot:          repoRoot,
		StateRoot:         stateRoot,
		Ref:               commandOpts.PathArg,
		MachineID:         commandOpts.V2.MachineID,
		UserID:            commandOpts.V2.UserID,
		ExtraLayers:       commandOpts.V2.Profiles,
		Confirmed:         commandOpts.Yes,
		NonInteractive:    commandOpts.NonInteractive,
		JSONMode:          commandOpts.JSONOutput,
		LifecyclePrompter: selectedLivePrompter(cmd, commandOpts),
	})
	if emitErr := emitSelectedPreviewReport(cmd.OutOrStdout(), result.Report, commandOpts.JSONOutput); emitErr != nil {
		return emitErr
	}
	return err
}

func runGuidedSyncRootCommand(cmd *cobra.Command, opts *rootOptions, ref string, syncOpts guidedSyncCommandOptions) error {
	repoRoot, err := selectedPreviewRepoRootFor(opts, "sync")
	if err != nil {
		report := v2guidedsync.ErrorReport("guidedsync.root.notFound", err.Error(), nil)
		_ = emitGuidedSyncReport(cmd.OutOrStdout(), report, syncOpts.JSONOutput)
		return &v2guidedsync.Error{Code: "guidedsync.root.notFound", Message: err.Error(), Exit: 2}
	}
	stateRoot, err := v2ledger.DefaultStateRoot(repoRoot)
	if err != nil {
		report := v2guidedsync.ErrorReport("guidedsync.stateRoot.default", err.Error(), nil)
		_ = emitGuidedSyncReport(cmd.OutOrStdout(), report, syncOpts.JSONOutput)
		return &v2guidedsync.Error{Code: "guidedsync.stateRoot.default", Message: err.Error(), Exit: 2}
	}
	choices := make([]v2guidedsync.Choice, 0, len(syncOpts.ChoiceFlags))
	for _, raw := range syncOpts.ChoiceFlags {
		choice, parseErr := v2guidedsync.ParseChoice(raw)
		if parseErr != nil {
			report := v2guidedsync.ErrorReport(v2guidedsync.CodeChoiceInvalid, parseErr.Error(), nil)
			_ = emitGuidedSyncReport(cmd.OutOrStdout(), report, syncOpts.JSONOutput)
			return parseErr
		}
		choices = append(choices, choice)
	}
	report, err := v2guidedsync.Run(v2guidedsync.Options{
		RepoRoot:       repoRoot,
		StateRoot:      stateRoot,
		Ref:            ref,
		MachineID:      syncOpts.V2.MachineID,
		UserID:         syncOpts.V2.UserID,
		ExtraLayers:    syncOpts.V2.Profiles,
		Choices:        choices,
		Confirmed:      syncOpts.Yes,
		NonInteractive: syncOpts.NonInteractive,
		JSONMode:       syncOpts.JSONOutput,
		In:             cmd.InOrStdin(),
		PromptOut:      cmd.OutOrStdout(),
	})
	if emitErr := emitGuidedSyncReport(cmd.OutOrStdout(), report, syncOpts.JSONOutput); emitErr != nil {
		return emitErr
	}
	return err
}

func runInitCommand(cmd *cobra.Command, opts *rootOptions, initOpts v2initcmd.Options, jsonOutput bool) error {
	repoRoot := strings.TrimSpace(initOpts.RepoRoot)
	if repoRoot == "" {
		cwd, err := os.Getwd()
		if err != nil {
			report := v2initcmdErrorReport("init.repo.invalid", err.Error())
			_ = emitInitReport(cmd.OutOrStdout(), report, jsonOutput)
			return err
		}
		repoRoot = cwd
	}
	if opts != nil && strings.TrimSpace(opts.configPath) != "" && !isExplicitV2Config(opts.configPath) {
		report := v2initcmdErrorReport("init.config.invalid", fmt.Sprintf("--config for v2 init must point to %s", v2resolution.RootConfigFile))
		_ = emitInitReport(cmd.OutOrStdout(), report, jsonOutput)
		return &v2initcmd.Error{Code: "init.config.invalid", Message: report.Error.Message, Exit: 2}
	}
	initOpts.RepoRoot = repoRoot
	report, err := v2initcmd.Run(initOpts)
	if emitErr := emitInitReport(cmd.OutOrStdout(), report, jsonOutput); emitErr != nil {
		return emitErr
	}
	return err
}

func runListCommand(cmd *cobra.Command, opts *rootOptions, listOpts v2listcmd.Options, jsonOutput bool) error {
	repoRoot, err := selectedPreviewRepoRootFor(opts, "list")
	if err != nil {
		report := v2listcmdErrorReport("list.root.notFound", err.Error())
		_ = emitListReport(cmd.OutOrStdout(), report, jsonOutput)
		return &v2listcmd.Error{Code: "list.root.notFound", Message: err.Error(), Exit: 2}
	}
	listOpts.RepoRoot = repoRoot
	report, err := v2listcmd.Run(listOpts)
	if emitErr := emitListReport(cmd.OutOrStdout(), report, jsonOutput); emitErr != nil {
		return emitErr
	}
	return err
}

func runSelectedPreviewCommand(cmd *cobra.Command, commandOpts commandOptions, repoRoot string) error {
	stateRoot, err := v2ledger.DefaultStateRoot(repoRoot)
	if err != nil {
		return emitSelectedPreviewError(cmd, commandOpts, "selectedpreview.stateRoot.default", err.Error(), nil)
	}
	report, err := v2selectedpreview.Build(v2selectedpreview.Options{
		Command:     commandOpts.Name,
		RepoRoot:    repoRoot,
		StateRoot:   stateRoot,
		Ref:         commandOpts.PathArg,
		MachineID:   commandOpts.V2.MachineID,
		UserID:      commandOpts.V2.UserID,
		ExtraLayers: commandOpts.V2.Profiles,
		DryRun:      commandOpts.DryRun,
		Confirmed:   commandOpts.Yes,
	})
	if emitErr := emitSelectedPreviewReport(cmd.OutOrStdout(), report, commandOpts.JSONOutput); emitErr != nil {
		return emitErr
	}
	return err
}

func selectedLivePrompter(cmd *cobra.Command, commandOpts commandOptions) v2lifecycle.Prompter {
	if commandOpts.JSONOutput || commandOpts.NonInteractive {
		return nil
	}
	return v2lifecycle.TextPrompter{In: cmd.InOrStdin(), Out: cmd.OutOrStdout()}
}

func selectedPreviewRepoRoot(opts *rootOptions) (string, error) {
	return selectedPreviewRepoRootFor(opts, "save/apply")
}

func selectedPreviewRepoRootFor(opts *rootOptions, operation string) (string, error) {
	if opts != nil && strings.TrimSpace(opts.configPath) != "" {
		if !isExplicitV2Config(opts.configPath) {
			return "", fmt.Errorf("--config for v2 selected-value %s must point to %s", operation, v2resolution.RootConfigFile)
		}
		return repoRootFromExplicitV2Config(opts.configPath)
	}
	return v2resolution.FindRoot("")
}

func isExplicitV2Config(configPath string) bool {
	return filepath.Base(strings.TrimSpace(configPath)) == v2resolution.RootConfigFile
}

func repoRootFromExplicitV2Config(configPath string) (string, error) {
	trimmed := strings.TrimSpace(configPath)
	if trimmed == "" {
		return "", fmt.Errorf("config path is required")
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("v2 config path is not a file: %s", abs)
	}
	if filepath.Base(abs) != v2resolution.RootConfigFile {
		return "", fmt.Errorf("v2 config path must be named %s", v2resolution.RootConfigFile)
	}
	return filepath.Dir(abs), nil
}

func initRepoRootFromConfig(configPath string) string {
	trimmed := strings.TrimSpace(configPath)
	if trimmed == "" || filepath.Base(trimmed) != v2resolution.RootConfigFile {
		return ""
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return filepath.Dir(trimmed)
	}
	return filepath.Dir(abs)
}

func emitInitReport(stdout io.Writer, report *v2initcmd.Report, jsonOutput bool) error {
	if jsonOutput {
		payload, err := v2initcmd.JSON(report)
		if err != nil {
			return err
		}
		_, err = fmt.Fprint(stdout, payload)
		return err
	}
	_, err := fmt.Fprintln(stdout, v2initcmd.Text(report))
	return err
}

func emitListReport(stdout io.Writer, report *v2listcmd.Report, jsonOutput bool) error {
	if jsonOutput {
		payload, err := v2listcmd.JSON(report)
		if err != nil {
			return err
		}
		_, err = fmt.Fprint(stdout, payload)
		return err
	}
	_, err := fmt.Fprintln(stdout, v2listcmd.Text(report))
	return err
}

func v2initcmdErrorReport(code string, message string) *v2initcmd.Report {
	report := &v2initcmd.Report{
		Schema:        v2initcmd.Schema,
		SchemaVersion: 1,
		Command:       v2initcmd.Command,
		RunID:         v2initcmd.RunID,
		Summary:       v2initcmd.Summary{Status: "error", Failed: 1},
		Init:          v2initcmd.InitResult{ProfileStack: []string{}, RepoFiles: []v2initcmd.InitFile{}, IdentityFiles: []v2initcmd.IdentityFile{}, MissingChoices: []v2initcmd.MissingChoice{}},
		Error:         &v2initcmd.ErrorObject{Code: code, Message: message},
		Diagnostics:   []v2initcmd.Diagnostic{{Code: code, Severity: "error", Message: message}},
	}
	return report
}

func v2listcmdErrorReport(code string, message string) *v2listcmd.Report {
	report := &v2listcmd.Report{
		Schema:        v2listcmd.Schema,
		SchemaVersion: 1,
		Command:       v2listcmd.Command,
		RunID:         v2listcmd.RunID,
		Summary:       v2listcmd.Summary{Status: "error", Failed: 1},
		List:          v2listcmd.ListResult{ProfileStack: []string{}, Settings: []v2listcmd.ManagedSetting{}},
		Error:         &v2listcmd.ErrorObject{Code: code, Message: message},
		Diagnostics:   []v2listcmd.Diagnostic{{Code: code, Severity: "error", Message: message}},
	}
	return report
}

func emitSelectedPreviewReport(stdout io.Writer, report *v2selectedpreview.Report, jsonOutput bool) error {
	if jsonOutput {
		payload, err := v2selectedpreview.JSON(report)
		if err != nil {
			return err
		}
		_, err = fmt.Fprint(stdout, payload)
		return err
	}
	_, err := fmt.Fprintln(stdout, v2selectedpreview.Text(report))
	return err
}

func emitGuidedSyncReport(stdout io.Writer, report *v2guidedsync.Report, jsonOutput bool) error {
	if jsonOutput {
		payload, err := v2guidedsync.JSON(report)
		if err != nil {
			return err
		}
		_, err = fmt.Fprint(stdout, payload)
		return err
	}
	_, err := fmt.Fprintln(stdout, v2guidedsync.Text(report))
	return err
}

func emitSelectedPreviewError(cmd *cobra.Command, commandOpts commandOptions, code string, message string, details map[string]any) error {
	report := v2selectedpreview.ErrorReport(commandOpts.Name, commandOpts.DryRun, code, message, details)
	_ = emitSelectedPreviewReport(cmd.OutOrStdout(), report, commandOpts.JSONOutput)
	return &v2selectedpreview.Error{Code: code, Message: message, Exit: 2, Details: details}
}

func runRecipeExplainCommand(cmd *cobra.Command, opts *rootOptions, target string, jsonOutput bool) error {
	repoRoot, err := recipeExplainRepoRoot(opts)
	if err != nil {
		explainErr := &v2recipe.ExplainError{Code: v2recipe.ExplainCodeInternalError, Message: err.Error(), Exit: 1}
		report := &v2recipe.ExplainReport{
			Schema:        v2recipe.ExplainSchema,
			SchemaVersion: v2recipe.SupportedVersion,
			Command:       v2recipe.ExplainCommand,
			RunID:         v2recipe.ExplainRunID,
			Summary:       v2recipe.ExplainSummary{Status: "error"},
			Items:         []any{},
			Error:         &v2recipe.ExplainErrorObject{Code: explainErr.Code, Message: explainErr.Message},
		}
		_ = emitRecipeExplainReport(cmd.OutOrStdout(), report, jsonOutput)
		return explainErr
	}
	report, err := v2recipe.Explain(v2recipe.ExplainOptions{Target: target, RepoRoot: repoRoot})
	if emitErr := emitRecipeExplainReport(cmd.OutOrStdout(), report, jsonOutput); emitErr != nil {
		return emitErr
	}
	return err
}

func runRecipeListCommand(cmd *cobra.Command, opts *rootOptions, jsonOutput bool) error {
	repoRoot, err := recipeExplainRepoRoot(opts)
	if err != nil {
		return err
	}
	report := v2recipe.List(v2recipe.ListOptions{RepoRoot: repoRoot})
	return emitRecipeListReport(cmd.OutOrStdout(), report, jsonOutput)
}

func runRecipeDiscoverCommand(cmd *cobra.Command, opts *rootOptions, target string, jsonOutput bool) error {
	report, err := v2recipe.Discover(v2recipe.DiscoverOptions{Target: target})
	if emitErr := emitRecipeDiscoverReport(cmd.OutOrStdout(), report, jsonOutput); emitErr != nil {
		return emitErr
	}
	return err
}

func runAddCommand(cmd *cobra.Command, opts *rootOptions, addOpts v2addtarget.Options, jsonOutput bool) error {
	repoStart, err := addRepoStart(opts)
	if err != nil {
		report, addErr := v2addtarget.Run(addOpts)
		if report == nil {
			report = &v2addtarget.Report{
				Schema:        v2recipe.ExplainSchema,
				SchemaVersion: 1,
				Command:       v2addtarget.Command,
				RunID:         v2addtarget.RunID,
				DryRun:        addOpts.DryRun,
				Summary:       v2addtarget.Summary{Status: "error", Failed: 1},
				Error:         &v2addtarget.ErrorObject{Code: v2addtarget.CodeRepoInvalid, Message: err.Error()},
			}
		}
		_ = addErr
		if emitErr := emitAddReport(cmd.OutOrStdout(), report, jsonOutput); emitErr != nil {
			return emitErr
		}
		if !jsonOutput {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), err.Error())
		}
		return err
	}
	repoRoot, err := v2resolution.FindRoot(repoStart)
	if err != nil {
		addOpts.RepoRoot = repoStart
	} else {
		addOpts.RepoRoot = repoRoot
	}

	report, runErr := v2addtarget.Run(addOpts)
	if emitErr := emitAddReport(cmd.OutOrStdout(), report, jsonOutput); emitErr != nil {
		return emitErr
	}
	if runErr != nil && !jsonOutput {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), runErr.Error())
	}
	return runErr
}

func addRepoStart(opts *rootOptions) (string, error) {
	if opts != nil && strings.TrimSpace(opts.configPath) != "" {
		abs, err := filepath.Abs(strings.TrimSpace(opts.configPath))
		if err != nil {
			return "", err
		}
		return filepath.Dir(abs), nil
	}
	return os.Getwd()
}

func recipeExplainRepoRoot(opts *rootOptions) (string, error) {
	if opts != nil && strings.TrimSpace(opts.configPath) != "" {
		abs, err := filepath.Abs(strings.TrimSpace(opts.configPath))
		if err != nil {
			return "", err
		}
		return filepath.Dir(abs), nil
	}
	return os.Getwd()
}

func emitRecipeExplainReport(stdout io.Writer, report *v2recipe.ExplainReport, jsonOutput bool) error {
	if jsonOutput {
		payload, err := v2recipe.ExplainJSON(report)
		if err != nil {
			return err
		}
		_, err = fmt.Fprint(stdout, payload)
		return err
	}
	_, err := fmt.Fprintln(stdout, v2recipe.ExplainText(report))
	return err
}

func emitRecipeListReport(stdout io.Writer, report *v2recipe.ListReport, jsonOutput bool) error {
	if jsonOutput {
		payload, err := v2recipe.ListJSON(report)
		if err != nil {
			return err
		}
		_, err = fmt.Fprint(stdout, payload)
		return err
	}
	_, err := fmt.Fprintln(stdout, v2recipe.ListText(report))
	return err
}

func emitRecipeDiscoverReport(stdout io.Writer, report *v2recipe.DiscoverReport, jsonOutput bool) error {
	if jsonOutput {
		payload, err := v2recipe.DiscoverJSON(report)
		if err != nil {
			return err
		}
		_, err = fmt.Fprint(stdout, payload)
		return err
	}
	_, err := fmt.Fprintln(stdout, v2recipe.DiscoverText(report))
	return err
}

func emitAddReport(stdout io.Writer, report *v2addtarget.Report, jsonOutput bool) error {
	if jsonOutput {
		payload, err := v2addtarget.JSON(report)
		if err != nil {
			return err
		}
		_, err = fmt.Fprint(stdout, payload)
		return err
	}
	_, err := fmt.Fprintln(stdout, v2addtarget.Text(report))
	return err
}

func runMigrateCommand(cmd *cobra.Command, opts *rootOptions, dryRun bool, jsonOutput bool) error {
	resolvedConfigPath, err := config.ResolvePath(config.ResolveOptions{ExplicitPath: opts.configPath})
	if err != nil {
		emitMigrateError(cmd.OutOrStdout(), cmd.ErrOrStderr(), jsonOutput, dryRun, explicitConfigPath(opts.configPath), err)
		return err
	}

	absConfigPath, err := filepath.Abs(resolvedConfigPath)
	if err != nil {
		cfgErr := dfmerr.Wrap(dfmerr.CodeIORead, fmt.Sprintf("Read failed: %s", resolvedConfigPath), map[string]any{"path": resolvedConfigPath}, err)
		emitMigrateError(cmd.OutOrStdout(), cmd.ErrOrStderr(), jsonOutput, dryRun, resolvedConfigPath, cfgErr)
		return cfgErr
	}

	var plan *v2migration.Plan
	if dryRun {
		plan, err = v2migration.BuildDryRunPlan(v2migration.Options{
			ConfigPath: absConfigPath,
			RunID:      v2migration.DefaultRunID,
		})
	} else {
		plan, err = v2migration.WriteMigrationOutput(v2migration.Options{
			ConfigPath: absConfigPath,
		})
	}
	if err != nil {
		if v2migration.IsBlocked(err) && plan != nil {
			if jsonOutput {
				payload, renderErr := v2migration.JSON(plan)
				if renderErr != nil {
					return renderErr
				}
				_, _ = fmt.Fprint(cmd.OutOrStdout(), payload)
			} else {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), v2migration.Text(plan))
			}
			return err
		}
		emitMigrateError(cmd.OutOrStdout(), cmd.ErrOrStderr(), jsonOutput, dryRun, absConfigPath, err)
		return err
	}

	if jsonOutput {
		payload, err := v2migration.JSON(plan)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprint(cmd.OutOrStdout(), payload)
		return nil
	}

	_, _ = fmt.Fprintln(cmd.OutOrStdout(), v2migration.Text(plan))
	return nil
}

func runMigrateParityCommand(cmd *cobra.Command, runDir string, jsonOutput bool, yamlOutput bool) error {
	if jsonOutput && yamlOutput {
		err := dfmerr.New(dfmerr.CodeFlagInvalidValue, "--json and --yaml cannot be used together", map[string]any{
			"flags": []string{"--json", "--yaml"},
		})
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), err.Error())
		return err
	}

	runDir = strings.TrimSpace(runDir)
	if runDir == "" {
		err := dfmerr.New(dfmerr.CodeFlagInvalidValue, "Flag required: --run-dir", map[string]any{"flag": "--run-dir"})
		emitMigrateParityError(cmd.OutOrStdout(), cmd.ErrOrStderr(), jsonOutput, yamlOutput, runDir, err)
		return err
	}

	report, err := v2migration.BuildParityReport(v2migration.ParityOptions{RunDir: runDir})
	if err != nil {
		emitMigrateParityError(cmd.OutOrStdout(), cmd.ErrOrStderr(), jsonOutput, yamlOutput, runDir, err)
		return err
	}
	if err := emitMigrateParityReport(cmd.OutOrStdout(), report, jsonOutput, yamlOutput); err != nil {
		return err
	}
	if report.Summary.Status != "ok" {
		return migrateParityBlockedError{blocked: report.Summary.Blocked}
	}
	return nil
}

func runMigratePromotePreviewCommand(cmd *cobra.Command, runDir string, jsonOutput bool) error {
	runDir = strings.TrimSpace(runDir)
	if runDir == "" {
		err := dfmerr.New(dfmerr.CodeFlagInvalidValue, "Flag required: --run-dir", map[string]any{"flag": "--run-dir"})
		emitMigratePromotionError(cmd.OutOrStdout(), cmd.ErrOrStderr(), jsonOutput, runDir, err)
		return err
	}

	report, err := v2migration.BuildPromotionReport(v2migration.PromotionOptions{RunDir: runDir})
	if err != nil {
		emitMigratePromotionError(cmd.OutOrStdout(), cmd.ErrOrStderr(), jsonOutput, runDir, err)
		return err
	}
	return emitMigratePromotionReport(cmd.OutOrStdout(), report, jsonOutput)
}

type migrateParityBlockedError struct {
	blocked int
}

func (e migrateParityBlockedError) Error() string {
	return fmt.Sprintf("migration parity blocked: %d blocked item(s)", e.blocked)
}

func emitMigrateParityReport(stdout io.Writer, report *v2migration.ParityReport, jsonOutput bool, yamlOutput bool) error {
	var (
		payload string
		err     error
	)
	switch {
	case jsonOutput:
		payload, err = v2migration.ParityJSON(report)
	case yamlOutput:
		payload, err = v2migration.ParityYAML(report)
	default:
		payload = v2migration.ParityText(report) + "\n"
	}
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(stdout, payload)
	return err
}

func emitMigratePromotionReport(stdout io.Writer, report *v2migration.PromotionReport, jsonOutput bool) error {
	var (
		payload string
		err     error
	)
	if jsonOutput {
		payload, err = v2migration.PromotionJSON(report)
	} else {
		payload = v2migration.PromotionText(report) + "\n"
	}
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(stdout, payload)
	return err
}

func emitMigrateParityError(stdout io.Writer, stderr io.Writer, jsonOutput bool, yamlOutput bool, runDir string, err error) {
	if !jsonOutput && !yamlOutput {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return
	}

	code := ""
	message := err.Error()
	var details map[string]any
	if dfmError, ok := dfmerr.As(err); ok {
		code = string(dfmError.Code)
		message = dfmError.Message
		details = dfmError.Details
	}
	report := v2migration.NewParityErrorReport(runDir, code, message, details)
	if renderErr := emitMigrateParityReport(stdout, report, jsonOutput, yamlOutput); renderErr != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
	}
}

func emitMigratePromotionError(stdout io.Writer, stderr io.Writer, jsonOutput bool, runDir string, err error) {
	if !jsonOutput {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return
	}

	code := ""
	message := err.Error()
	var details map[string]any
	if dfmError, ok := dfmerr.As(err); ok {
		code = string(dfmError.Code)
		message = dfmError.Message
		details = dfmError.Details
	}
	report := v2migration.NewPromotionErrorReport(runDir, code, message, details)
	if renderErr := emitMigratePromotionReport(stdout, report, jsonOutput); renderErr != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
	}
}

func emitMigrateError(stdout io.Writer, stderr io.Writer, jsonOutput bool, dryRun bool, configPath any, err error) {
	if !jsonOutput {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return
	}

	code := ""
	message := err.Error()
	var details map[string]any
	if dfmError, ok := dfmerr.As(err); ok {
		code = string(dfmError.Code)
		message = dfmError.Message
		details = dfmError.Details
	}
	payload := v2migration.NewErrorPayload(dryRun, configPath, code, message, details)
	rendered, renderErr := v2migration.ErrorJSON(payload)
	if renderErr != nil {
		return
	}
	_, _ = fmt.Fprint(stdout, rendered)
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

	switch commandOpts.Name {
	case "status":
		syncPayloads, summary, err = buildStatusSyncPayloads(cfg, selections)
		if err != nil {
			return nil, err
		}
	case "deploy":
		syncPayloads, summary, err = buildDeploySyncPayloads(cfg, selections, commandOpts.DryRun)
		if err != nil {
			return nil, err
		}
	case "import":
		syncPayloads, summary, err = buildImportSyncPayloads(cfg, selections, commandOpts.DryRun)
		if err != nil {
			return nil, err
		}
	case "diff":
		includePatch := !commandOpts.JSONOutput || commandOpts.IncludePatch
		syncPayloads, summary, err = buildDiffSyncPayloads(cfg, selections, commandOpts.Direction, commandOpts.ContextLines, includePatch)
		if err != nil {
			return nil, err
		}
	default:
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
		if exitErr, ok := err.(interface{ ExitCode() int }); ok {
			return exitErr.ExitCode()
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
