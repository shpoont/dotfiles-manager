package nativeops

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/stretchr/testify/require"
)

func TestBuildConstructsClosedCommandWithoutInheritedEnvironment(t *testing.T) {
	t.Parallel()

	rec := nativeTestRecipe(t)
	artifactRoot := t.TempDir()
	tempRoot := t.TempDir()
	plan, err := Build(Options{Recipe: rec, RecipeSource: recipe.RecipeSourceBundled, OperationID: "export-settings", GOOS: runtime.GOOS, RepoRoot: t.TempDir(), ArtifactRoot: artifactRoot, TempRoot: tempRoot})
	require.NoError(t, err)
	require.Equal(t, "export-settings", plan.OperationID)
	require.Equal(t, "export", plan.Kind)
	require.Equal(t, "/usr/bin/native-safe-tool", plan.ExecutablePath)
	require.Equal(t, tempRoot, plan.Dir)
	require.Equal(t, []string{"--out", filepath.Join(artifactRoot, "exports", "settings.bundle"), "--scratch", filepath.Join(tempRoot, "scratch", "run")}, plan.Args)
	require.Equal(t, []string{"DFM_OUTPUT=" + filepath.Join(artifactRoot, "exports", "settings.bundle")}, plan.Env)
	require.NotContains(t, plan.Env, "PATH=")
	require.Equal(t, "reviewed argv command: native-safe-tool", plan.CommandSummary)
}

func TestRunCapturesStructuredStatusWithoutRawOutput(t *testing.T) {
	t.Parallel()

	rec := nativeTestRecipe(t)
	fake := fakeExecutor{result: ExecResult{ExitCode: 0, Stdout: CaptureSummary{Mode: "capture", Bytes: 11, SHA256: "hash-only"}, Stderr: CaptureSummary{Mode: "discard"}}}
	result := Run(context.Background(), Options{Recipe: rec, RecipeSource: recipe.RecipeSourceBundled, OperationID: "export-settings", GOOS: runtime.GOOS, RepoRoot: t.TempDir(), ArtifactRoot: t.TempDir(), TempRoot: t.TempDir(), Executor: fake})
	require.Equal(t, StatusSucceeded, result.Status)
	require.NotNil(t, result.ExitCode)
	require.Equal(t, 0, *result.ExitCode)
	require.Equal(t, int64(11), result.Stdout.Bytes)
	require.Equal(t, "hash-only", result.Stdout.SHA256)
	require.NotContains(t, fmt.Sprintf("%+v", result), "secret-output")
	require.NotEmpty(t, result.Outputs)
}

func TestRunReportsFailureTimeoutAndOutputLimit(t *testing.T) {
	t.Parallel()

	rec := nativeTestRecipe(t)
	base := Options{Recipe: rec, RecipeSource: recipe.RecipeSourceBundled, OperationID: "export-settings", GOOS: runtime.GOOS, RepoRoot: t.TempDir(), ArtifactRoot: t.TempDir(), TempRoot: t.TempDir()}

	failed := Run(context.Background(), withExecutor(base, fakeExecutor{result: ExecResult{ExitCode: 3}}))
	require.Equal(t, StatusFailed, failed.Status)
	require.Equal(t, CodeExecutionFailed, failed.Diagnostics[0].Code)
	require.Equal(t, "native operation exited with code 3", failed.Diagnostics[0].Message)

	startErr := Run(context.Background(), withExecutor(base, fakeExecutor{result: ExecResult{ExitCode: 0, Err: fmt.Errorf("fork/exec /secret/local/tool: no such file")}}))
	require.Equal(t, StatusFailed, startErr.Status)
	require.Equal(t, "native operation failed to start or complete", startErr.Diagnostics[0].Message)
	require.NotContains(t, startErr.Diagnostics[0].Message, "/secret/local/tool")

	timedOut := Run(context.Background(), withExecutor(base, fakeExecutor{result: ExecResult{TimedOut: true}}))
	require.Equal(t, StatusTimedOut, timedOut.Status)
	require.Equal(t, CodeExecutionTimeout, timedOut.Diagnostics[0].Code)

	limited := Run(context.Background(), withExecutor(base, fakeExecutor{result: ExecResult{ExitCode: 0, Stdout: CaptureSummary{Mode: "capture", Bytes: 70000, LimitExceeded: true}}}))
	require.Equal(t, StatusBlocked, limited.Status)
	require.Equal(t, CodeOutputLimitExceeded, limited.Diagnostics[0].Code)
}

func TestBuildBlocksUnsafeNativeOperations(t *testing.T) {
	t.Parallel()

	t.Run("unknown operation", func(t *testing.T) {
		_, err := Build(nativeOptions(t, nativeTestRecipe(t)))
		require.Error(t, err)
		require.Equal(t, CodeOperationUnknown, err.(*Error).Code)
	})

	t.Run("local recipe without matching trust", func(t *testing.T) {
		opts := nativeOptions(t, nativeTestRecipe(t))
		opts.OperationID = "export-settings"
		opts.RecipeSource = recipe.RecipeSourceLocal
		_, err := Build(opts)
		require.Error(t, err)
		require.Equal(t, CodeLocalTrustRequired, err.(*Error).Code)
	})

	t.Run("local recipe with matching trust", func(t *testing.T) {
		rec := nativeTestRecipe(t)
		opts := nativeOptions(t, rec)
		opts.OperationID = "export-settings"
		opts.RecipeSource = recipe.RecipeSourceLocal
		opts.TrustEvaluation = nativeTrustEvaluation(t, rec)
		_, err := Build(opts)
		require.NoError(t, err)
	})

	t.Run("local recipe with matching but unreviewed native trust", func(t *testing.T) {
		rec := nativeTestRecipe(t)
		opts := nativeOptions(t, rec)
		opts.OperationID = "export-settings"
		opts.RecipeSource = recipe.RecipeSourceLocal
		opts.TrustEvaluation = nativeTrustEvaluation(t, rec)
		opts.TrustEvaluation.ReviewedNativeOperations = false
		_, err := Build(opts)
		require.Error(t, err)
		require.Equal(t, CodeLocalTrustRequired, err.(*Error).Code)
	})

	t.Run("unsupported platform", func(t *testing.T) {
		opts := nativeOptions(t, nativeTestRecipe(t))
		opts.OperationID = "export-settings"
		opts.GOOS = "windows"
		_, err := Build(opts)
		require.Error(t, err)
		require.Equal(t, CodePlatformUnsupported, err.(*Error).Code)
	})

	t.Run("relative executable", func(t *testing.T) {
		rec := nativeTestRecipe(t)
		op := rec.NativeOperations["export-settings"]
		op.Command.Executable = "native-tool"
		rec.NativeOperations["export-settings"] = op
		opts := nativeOptions(t, rec)
		opts.OperationID = "export-settings"
		_, err := Build(opts)
		require.Error(t, err)
		require.Equal(t, CodeRecipeInvalid, err.(*Error).Code)
	})

	t.Run("executable inside repo", func(t *testing.T) {
		repoRoot := t.TempDir()
		rec := nativeTestRecipe(t)
		op := rec.NativeOperations["export-settings"]
		op.Command.Executable = filepath.Join(repoRoot, "bin", "native-tool")
		rec.NativeOperations["export-settings"] = op
		opts := nativeOptions(t, rec)
		opts.OperationID = "export-settings"
		opts.RepoRoot = repoRoot
		_, err := Build(opts)
		require.Error(t, err)
		require.Equal(t, CodeExecutableBlocked, err.(*Error).Code)
	})

	t.Run("undeclared output ref", func(t *testing.T) {
		rec := nativeTestRecipe(t)
		op := rec.NativeOperations["export-settings"]
		op.Command.Args = append(op.Command.Args, recipe.NativeArg{Output: "missing"})
		rec.NativeOperations["export-settings"] = op
		opts := nativeOptions(t, rec)
		opts.OperationID = "export-settings"
		_, err := Build(opts)
		require.Error(t, err)
		require.Equal(t, CodeRecipeInvalid, err.(*Error).Code)
	})

	t.Run("sensitive env key", func(t *testing.T) {
		rec := nativeTestRecipe(t)
		op := rec.NativeOperations["export-settings"]
		op.Env["API_TOKEN"] = recipe.NativeEnvValue{Literal: "nope"}
		rec.NativeOperations["export-settings"] = op
		opts := nativeOptions(t, rec)
		opts.OperationID = "export-settings"
		_, err := Build(opts)
		require.Error(t, err)
		require.Equal(t, CodeRecipeInvalid, err.(*Error).Code)
	})
}

func TestNativeRunnerImportsStayConfinedToNativeExportFlow(t *testing.T) {
	t.Parallel()

	repoRoot, err := filepath.Abs("../../..")
	require.NoError(t, err)
	importPath := "github.com/shpoont/dotfiles-manager/internal/v2/nativeops"
	allowedProductionImporters := map[string]bool{
		filepath.Join("internal", "v2", "nativeexport", "nativeexport.go"):       true,
		filepath.Join("internal", "v2", "selectedlive", "selectedlive.go"):       true,
		filepath.Join("internal", "v2", "selectedpreview", "selectedpreview.go"): true,
	}
	var unexpected []string
	err = filepath.WalkDir(repoRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			rel, err := filepath.Rel(repoRoot, path)
			if err != nil {
				return err
			}
			if rel == ".git" || strings.HasPrefix(rel, filepath.Join("internal", "v2", "nativeops")) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(contents), importPath) {
			rel, err := filepath.Rel(repoRoot, path)
			if err != nil {
				return err
			}
			if !allowedProductionImporters[rel] {
				unexpected = append(unexpected, rel)
			}
		}
		return nil
	})
	require.NoError(t, err)
	require.Empty(t, unexpected, "native operation runner imports must remain confined to the reviewed native-export diff/save flow; status/apply/import must not grow ad-hoc native execution")
}

func TestDefaultExecutorCaptureLimitAndTimeout(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("test uses POSIX helper binaries")
	}
	executor := DefaultExecutor{}
	dir := t.TempDir()

	echo := executor.Run(context.Background(), ExecSpec{
		Executable: "/bin/echo",
		Args:       []string{"hello"},
		Dir:        dir,
		Stdout:     recipe.NativeStreamPolicy{Mode: "capture", MaxBytes: 64},
		Stderr:     recipe.NativeStreamPolicy{Mode: "discard"},
	})
	require.NoError(t, echo.Err)
	require.Equal(t, 0, echo.ExitCode)
	require.Equal(t, int64(6), echo.Stdout.Bytes)
	require.NotEmpty(t, echo.Stdout.SHA256)

	emptyEnv := executor.Run(context.Background(), ExecSpec{
		Executable: "/usr/bin/env",
		Dir:        dir,
		Stdout:     recipe.NativeStreamPolicy{Mode: "capture", MaxBytes: 64},
		Stderr:     recipe.NativeStreamPolicy{Mode: "discard"},
	})
	require.NoError(t, emptyEnv.Err)
	require.Equal(t, 0, emptyEnv.ExitCode)
	require.Equal(t, int64(0), emptyEnv.Stdout.Bytes)

	limited := executor.Run(context.Background(), ExecSpec{
		Executable: "/bin/echo",
		Args:       []string{"hello"},
		Dir:        dir,
		Stdout:     recipe.NativeStreamPolicy{Mode: "capture", MaxBytes: 1},
		Stderr:     recipe.NativeStreamPolicy{Mode: "discard"},
	})
	require.Error(t, limited.Err)
	require.True(t, limited.Stdout.LimitExceeded)

	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	timeout := executor.Run(ctx, ExecSpec{
		Executable: "/bin/sleep",
		Args:       []string{"1"},
		Dir:        dir,
		Stdout:     recipe.NativeStreamPolicy{Mode: "discard"},
		Stderr:     recipe.NativeStreamPolicy{Mode: "discard"},
	})
	require.True(t, timeout.TimedOut)
}

func TestCaptureWriterBranches(t *testing.T) {
	t.Parallel()

	discard := newCaptureWriter(recipe.NativeStreamPolicy{Mode: "discard"})
	n, err := discard.Write([]byte("secret-output"))
	require.NoError(t, err)
	require.Equal(t, len("secret-output"), n)
	require.Equal(t, CaptureSummary{Mode: "discard", Bytes: int64(len("secret-output"))}, discard.Summary())
	require.False(t, discard.LimitExceeded())

	unknown := newCaptureWriter(recipe.NativeStreamPolicy{Mode: "unknown"})
	n, err = unknown.Write([]byte("ignored"))
	require.NoError(t, err)
	require.Equal(t, len("ignored"), n)

	capture := newCaptureWriter(recipe.NativeStreamPolicy{Mode: "capture", MaxBytes: 4})
	n, err = capture.Write([]byte("abcd"))
	require.NoError(t, err)
	require.Equal(t, 4, n)
	n, err = capture.Write([]byte("e"))
	require.Error(t, err)
	require.Equal(t, 0, n)
	require.True(t, capture.Summary().LimitExceeded)
}

func TestBuildCoversPathAndTrustEdges(t *testing.T) {
	t.Parallel()

	t.Run("nil recipe", func(t *testing.T) {
		_, err := Build(Options{OperationID: "x"})
		require.Error(t, err)
		require.Equal(t, CodeRecipeInvalid, err.(*Error).Code)
		require.Contains(t, err.Error(), "recipe is required")
		require.Empty(t, (*Error)(nil).Error())
	})

	t.Run("unknown source", func(t *testing.T) {
		opts := nativeOptions(t, nativeTestRecipe(t))
		opts.OperationID = "export-settings"
		opts.RecipeSource = "downloaded"
		_, err := Build(opts)
		require.Error(t, err)
		require.Equal(t, CodeLocalTrustRequired, err.(*Error).Code)
	})

	t.Run("missing source", func(t *testing.T) {
		opts := nativeOptions(t, nativeTestRecipe(t))
		opts.OperationID = "export-settings"
		opts.RecipeSource = ""
		_, err := Build(opts)
		require.Error(t, err)
		require.Equal(t, CodeLocalTrustRequired, err.(*Error).Code)
	})

	t.Run("local trust mismatched surface", func(t *testing.T) {
		rec := nativeTestRecipe(t)
		opts := nativeOptions(t, rec)
		opts.OperationID = "export-settings"
		opts.RecipeSource = recipe.RecipeSourceLocal
		opts.TrustEvaluation = nativeTrustEvaluation(t, rec)
		opts.TrustEvaluation.WriteSurfaceSHA256 = "wrong"
		_, err := Build(opts)
		require.Error(t, err)
		require.Equal(t, CodeLocalTrustRequired, err.(*Error).Code)
	})

	t.Run("local trust blocks same-count executable change", func(t *testing.T) {
		rec := nativeTestRecipe(t)
		trusted := nativeTrustEvaluation(t, rec)
		op := rec.NativeOperations["export-settings"]
		op.Command.Executable = "/usr/bin/native-other-tool"
		rec.NativeOperations["export-settings"] = op
		opts := nativeOptions(t, rec)
		opts.OperationID = "export-settings"
		opts.RecipeSource = recipe.RecipeSourceLocal
		opts.TrustEvaluation = trusted
		_, err := Build(opts)
		require.Error(t, err)
		require.Equal(t, CodeLocalTrustRequired, err.(*Error).Code)
	})

	t.Run("local trust blocks same-count env and path changes", func(t *testing.T) {
		rec := nativeTestRecipe(t)
		trusted := nativeTrustEvaluation(t, rec)
		op := rec.NativeOperations["export-settings"]
		op.Env = map[string]recipe.NativeEnvValue{"DFM_OTHER": {Literal: "value"}}
		op.Outputs = map[string]recipe.NativePathSpec{"bundle": {Root: "artifact", Path: "exports/other.bundle"}}
		rec.NativeOperations["export-settings"] = op
		opts := nativeOptions(t, rec)
		opts.OperationID = "export-settings"
		opts.RecipeSource = recipe.RecipeSourceLocal
		opts.TrustEvaluation = trusted
		_, err := Build(opts)
		require.Error(t, err)
		require.Equal(t, CodeLocalTrustRequired, err.(*Error).Code)
	})

	t.Run("input location and temp env resolve", func(t *testing.T) {
		rec := nativeTestRecipe(t)
		op := rec.NativeOperations["export-settings"]
		op.Inputs = map[string]recipe.NativePathSpec{"source": {Root: "location", Location: "home", Path: ".native/input"}}
		op.Command.Args = append([]recipe.NativeArg{{Literal: "--in"}, {Input: "source"}}, op.Command.Args...)
		op.Env["DFM_TEMP"] = recipe.NativeEnvValue{Temp: "scratch"}
		rec.NativeOperations["export-settings"] = op
		opts := nativeOptions(t, rec)
		opts.OperationID = "export-settings"
		opts.LocationRoots = map[string]string{"home": t.TempDir()}
		plan, err := Build(opts)
		require.NoError(t, err)
		require.Contains(t, strings.Join(plan.Args, "\x00"), ".native/input")
		require.Contains(t, strings.Join(plan.Env, "\x00"), "DFM_TEMP=")
	})

	t.Run("missing location root", func(t *testing.T) {
		rec := nativeTestRecipe(t)
		op := rec.NativeOperations["export-settings"]
		op.Inputs = map[string]recipe.NativePathSpec{"source": {Root: "location", Location: "home", Path: ".native/input"}}
		op.Command.Args = append([]recipe.NativeArg{{Input: "source"}}, op.Command.Args...)
		rec.NativeOperations["export-settings"] = op
		opts := nativeOptions(t, rec)
		opts.OperationID = "export-settings"
		opts.LocationRoots = nil
		_, err := Build(opts)
		require.Error(t, err)
		require.Equal(t, CodePathInvalid, err.(*Error).Code)
	})

	t.Run("empty temp root", func(t *testing.T) {
		opts := nativeOptions(t, nativeTestRecipe(t))
		opts.OperationID = "export-settings"
		opts.TempRoot = ""
		_, err := Build(opts)
		require.Error(t, err)
		require.Equal(t, CodePathInvalid, err.(*Error).Code)
	})

	t.Run("default resolver rejects whitespace and blocked basename", func(t *testing.T) {
		_, err := DefaultExecutableResolver{}.ResolveExecutable(" /bin/echo", Options{})
		require.Error(t, err)
		_, err = DefaultExecutableResolver{}.ResolveExecutable("/bin/bash", Options{})
		require.Error(t, err)
		require.True(t, blockedExecutable("bash"))
		require.True(t, blockedExecutable("powershell.exe"))
		require.False(t, blockedExecutable("native-tool"))
		require.True(t, safeEnvKey("DFM_OUTPUT"))
		require.False(t, safeEnvKey("PATH"))
		require.False(t, safeEnvKey("DFM_PATH"))
	})

	t.Run("safe path helpers", func(t *testing.T) {
		require.True(t, safeRelPath("a/b"))
		require.False(t, safeRelPath("../a"))
		require.False(t, safeRelPath("/a"))
		inside, err := pathInside(t.TempDir(), filepath.Join(t.TempDir(), "x"))
		require.NoError(t, err)
		require.False(t, inside)
	})

	t.Run("blocked result carries code", func(t *testing.T) {
		result := blockedResult("op", &Error{Code: CodeEnvInvalid, Message: "bad env"}, time.Millisecond)
		require.Equal(t, StatusBlocked, result.Status)
		require.Equal(t, CodeEnvInvalid, result.Diagnostics[0].Code)
	})
}

type fakeExecutor struct{ result ExecResult }

func (f fakeExecutor) Run(ctx context.Context, spec ExecSpec) ExecResult {
	return f.result
}

func withExecutor(opts Options, executor Executor) Options {
	opts.Executor = executor
	return opts
}

func nativeOptions(t *testing.T, rec *recipe.Recipe) Options {
	t.Helper()
	return Options{Recipe: rec, RecipeSource: recipe.RecipeSourceBundled, OperationID: "missing", GOOS: runtime.GOOS, RepoRoot: t.TempDir(), ArtifactRoot: t.TempDir(), TempRoot: t.TempDir()}
}

func nativeTrustEvaluation(t *testing.T, rec *recipe.Recipe) *recipe.TrustEvaluation {
	t.Helper()
	contentHash, err := recipe.RecipeContentSHA256(rec)
	require.NoError(t, err)
	surface, surfaceHash, err := recipe.RecipeWriteSurface(rec)
	require.NoError(t, err)
	return &recipe.TrustEvaluation{
		Source:                   recipe.RecipeSourceLocal,
		Target:                   rec.Target,
		Status:                   recipe.TrustStatusTrusted,
		ContentSHA256:            contentHash,
		WriteSurfaceSHA256:       surfaceHash,
		WriteSurface:             surface,
		ReviewedNativeOperations: true,
	}
}

func nativeTestRecipe(t *testing.T) *recipe.Recipe {
	t.Helper()
	return &recipe.Recipe{
		Schema:        recipe.Schema,
		SchemaVersion: recipe.SupportedVersion,
		Target:        "native-test",
		DisplayName:   "Native Test",
		SupportLevel:  "experimental",
		Capability:    "read-write",
		Locations: map[string]recipe.Location{
			"home": {Default: "~"},
		},
		Resources: map[string]recipe.Resource{
			"marker": {Driver: recipe.FileDriverID, Location: "home", Path: ".native-test/marker", Capability: "read-write"},
		},
		Settings: map[string]recipe.Setting{
			"settings": {Label: "Settings", ScopeDefault: "user", Resource: "marker", Capability: "read-write", ArtifactForm: "native", Lifecycle: recipe.LifecycleAllowed},
		},
		NativeOperations: map[string]recipe.NativeOperation{
			"export-settings": {
				Kind:              "export",
				Reviewed:          true,
				Runner:            "command",
				Platforms:         []string{runtime.GOOS},
				ArtifactForm:      "native",
				DiffMode:          "metadata-only",
				Lifecycle:         recipe.LifecycleAllowed,
				WorkingDirectory:  "temp",
				TimeoutSeconds:    5,
				ExpectedExitCodes: []int{0},
				Command: recipe.NativeCommand{Executable: "/usr/bin/native-safe-tool", Args: []recipe.NativeArg{
					{Literal: "--out"},
					{Output: "bundle"},
					{Literal: "--scratch"},
					{Temp: "scratch"},
				}},
				Stdin:     recipe.NativeStdinPolicy{Mode: "none"},
				Stdout:    recipe.NativeStreamPolicy{Mode: "capture", MaxBytes: 4096},
				Stderr:    recipe.NativeStreamPolicy{Mode: "discard"},
				Env:       map[string]recipe.NativeEnvValue{"DFM_OUTPUT": {Output: "bundle"}},
				Outputs:   map[string]recipe.NativePathSpec{"bundle": {Root: "artifact", Path: "exports/settings.bundle"}},
				TempPaths: map[string]recipe.NativePathSpec{"scratch": {Root: "temp", Path: "scratch/run"}},
				Redaction: "metadata-only",
			},
		},
	}
}
