package nativeops

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
)

const (
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
	StatusTimedOut  = "timed-out"
	StatusBlocked   = "blocked"

	SeverityError = "error"
)

const (
	CodeRecipeInvalid       = "nativeops.recipe.invalid"
	CodeOperationUnknown    = "nativeops.operation.unknown"
	CodeOperationUnreviewed = "nativeops.operation.unreviewed"
	CodeLocalTrustRequired  = "nativeops.local.trust-required"
	CodePlatformUnsupported = "nativeops.platform.unsupported"
	CodeExecutableBlocked   = "nativeops.executable.blocked"
	CodePathInvalid         = "nativeops.path.invalid"
	CodeEnvInvalid          = "nativeops.env.invalid"
	CodeExecutionFailed     = "nativeops.execution.failed"
	CodeExecutionTimeout    = "nativeops.execution.timeout"
	CodeOutputLimitExceeded = "nativeops.output.limit-exceeded"
)

type Options struct {
	Recipe             *recipe.Recipe
	RecipeSource       string
	TrustEvaluation    *recipe.TrustEvaluation
	OperationID        string
	GOOS               string
	RepoRoot           string
	ArtifactRoot       string
	TempRoot           string
	LocationRoots      map[string]string
	ExecutableResolver ExecutableResolver
	Executor           Executor
}

type Plan struct {
	OperationID    string
	Kind           string
	Timeout        time.Duration
	ExecutablePath string
	Args           []string
	Env            []string
	Dir            string
	Outputs        []PathSummary
	CommandSummary string
}

type PathSummary struct {
	ID   string `json:"id"`
	Root string `json:"root"`
	Path string `json:"path"`
}

type Result struct {
	OperationID    string         `json:"operationId"`
	Kind           string         `json:"kind"`
	Status         string         `json:"status"`
	ExitCode       *int           `json:"exitCode,omitempty"`
	DurationMillis int64          `json:"durationMillis"`
	TimeoutMillis  int64          `json:"timeoutMillis"`
	CommandSummary string         `json:"commandSummary"`
	Stdout         CaptureSummary `json:"stdout"`
	Stderr         CaptureSummary `json:"stderr"`
	Outputs        []PathSummary  `json:"outputs,omitempty"`
	Diagnostics    []Diagnostic   `json:"diagnostics,omitempty"`
}

type CaptureSummary struct {
	Mode          string `json:"mode"`
	Bytes         int64  `json:"bytes"`
	SHA256        string `json:"sha256,omitempty"`
	LimitExceeded bool   `json:"limitExceeded,omitempty"`
}

type Diagnostic struct {
	Code        string `json:"code"`
	Severity    string `json:"severity"`
	Message     string `json:"message"`
	OperationID string `json:"operationId,omitempty"`
}

type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

type ExecutableResolver interface {
	ResolveExecutable(executable string, opts Options) (string, error)
}

type Executor interface {
	Run(ctx context.Context, spec ExecSpec) ExecResult
}

type ExecSpec struct {
	Executable string
	Args       []string
	Env        []string
	Dir        string
	Stdout     recipe.NativeStreamPolicy
	Stderr     recipe.NativeStreamPolicy
}

type ExecResult struct {
	ExitCode int
	Stdout   CaptureSummary
	Stderr   CaptureSummary
	TimedOut bool
	Err      error
}

func Build(opts Options) (Plan, error) {
	if opts.Recipe == nil {
		return Plan{}, blocked(CodeRecipeInvalid, "recipe is required")
	}
	if err := opts.Recipe.Validate(); err != nil {
		return Plan{}, blocked(CodeRecipeInvalid, err.Error())
	}
	op, ok := opts.Recipe.NativeOperations[opts.OperationID]
	if !ok {
		return Plan{}, blocked(CodeOperationUnknown, fmt.Sprintf("native operation %q is not declared", opts.OperationID))
	}
	if !op.Reviewed {
		return Plan{}, blocked(CodeOperationUnreviewed, fmt.Sprintf("native operation %s is not reviewed", opts.OperationID))
	}
	if err := requireNativeTrust(opts); err != nil {
		return Plan{}, err
	}
	goos := opts.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	if !stringIn(goos, op.Platforms) {
		return Plan{}, blocked(CodePlatformUnsupported, fmt.Sprintf("native operation %s is not supported on %s", opts.OperationID, goos))
	}
	tempRoot, err := cleanAbsDir(opts.TempRoot, "temp root")
	if err != nil {
		return Plan{}, blocked(CodePathInvalid, err.Error())
	}
	resolver := opts.ExecutableResolver
	if resolver == nil {
		resolver = DefaultExecutableResolver{}
	}
	executable, err := resolver.ResolveExecutable(op.Command.Executable, opts)
	if err != nil {
		return Plan{}, blocked(CodeExecutableBlocked, err.Error())
	}
	args, err := resolveArgs(op, opts)
	if err != nil {
		return Plan{}, err
	}
	if strings.EqualFold(filepath.Base(executable), "osascript") {
		for _, arg := range args {
			if arg == "-e" {
				return Plan{}, blocked(CodeExecutableBlocked, "osascript -e inline script mode is blocked")
			}
		}
	}
	env, err := resolveEnv(op, opts)
	if err != nil {
		return Plan{}, err
	}
	outputs, err := outputSummaries(op, opts)
	if err != nil {
		return Plan{}, err
	}
	return Plan{
		OperationID:    opts.OperationID,
		Kind:           op.Kind,
		Timeout:        time.Duration(op.TimeoutSeconds) * time.Second,
		ExecutablePath: executable,
		Args:           args,
		Env:            env,
		Dir:            tempRoot,
		Outputs:        outputs,
		CommandSummary: "reviewed argv command: " + filepath.Base(executable),
	}, nil
}

func Run(ctx context.Context, opts Options) Result {
	started := time.Now()
	plan, err := Build(opts)
	if err != nil {
		return blockedResult(opts.OperationID, err, time.Since(started))
	}
	executor := opts.Executor
	if executor == nil {
		executor = DefaultExecutor{}
	}
	ctx, cancel := context.WithTimeout(ctx, plan.Timeout)
	defer cancel()
	op := opts.Recipe.NativeOperations[opts.OperationID]
	execResult := executor.Run(ctx, ExecSpec{Executable: plan.ExecutablePath, Args: plan.Args, Env: plan.Env, Dir: plan.Dir, Stdout: op.Stdout, Stderr: op.Stderr})
	result := Result{
		OperationID:    plan.OperationID,
		Kind:           plan.Kind,
		Status:         StatusSucceeded,
		DurationMillis: time.Since(started).Milliseconds(),
		TimeoutMillis:  plan.Timeout.Milliseconds(),
		CommandSummary: plan.CommandSummary,
		Stdout:         execResult.Stdout,
		Stderr:         execResult.Stderr,
		Outputs:        plan.Outputs,
	}
	if execResult.TimedOut || errors.Is(execResult.Err, context.DeadlineExceeded) {
		result.Status = StatusTimedOut
		result.Diagnostics = append(result.Diagnostics, diagnostic(CodeExecutionTimeout, opts.OperationID, "native operation timed out"))
		return result
	}
	if execResult.Stdout.LimitExceeded || execResult.Stderr.LimitExceeded {
		result.Status = StatusBlocked
		result.Diagnostics = append(result.Diagnostics, diagnostic(CodeOutputLimitExceeded, opts.OperationID, "native operation exceeded declared output limit"))
		return result
	}
	exitCode := execResult.ExitCode
	result.ExitCode = &exitCode
	if execResult.Err != nil || !intIn(exitCode, op.ExpectedExitCodes) {
		result.Status = StatusFailed
		message := fmt.Sprintf("native operation exited with code %d", exitCode)
		if execResult.Err != nil && intIn(exitCode, op.ExpectedExitCodes) {
			message = "native operation failed to start or complete"
		}
		result.Diagnostics = append(result.Diagnostics, diagnostic(CodeExecutionFailed, opts.OperationID, message))
	}
	return result
}

type DefaultExecutableResolver struct{}

func (DefaultExecutableResolver) ResolveExecutable(executable string, opts Options) (string, error) {
	trimmed := strings.TrimSpace(executable)
	if trimmed == "" || trimmed != executable {
		return "", fmt.Errorf("native executable must be a non-empty fixed path without surrounding whitespace")
	}
	if !filepath.IsAbs(trimmed) {
		return "", fmt.Errorf("native executable must be an absolute reviewed path")
	}
	cleaned := filepath.Clean(trimmed)
	if cleaned != trimmed || strings.Contains(filepath.ToSlash(trimmed), "../") {
		return "", fmt.Errorf("native executable path must be clean and non-traversing")
	}
	base := filepath.Base(trimmed)
	if blockedExecutable(base) {
		return "", fmt.Errorf("native executable is blocked: %s", base)
	}
	if opts.RepoRoot != "" {
		inside, err := pathInside(opts.RepoRoot, trimmed)
		if err != nil {
			return "", err
		}
		if inside {
			return "", fmt.Errorf("native executable must not be inside the writable repository")
		}
	}
	return trimmed, nil
}

type DefaultExecutor struct{}

func (DefaultExecutor) Run(ctx context.Context, spec ExecSpec) ExecResult {
	cmd := exec.CommandContext(ctx, spec.Executable, spec.Args...)
	cmd.Env = make([]string, len(spec.Env))
	copy(cmd.Env, spec.Env)
	cmd.Dir = spec.Dir
	stdout := newCaptureWriter(spec.Stdout)
	stderr := newCaptureWriter(spec.Stderr)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Stdin = nil
	err := cmd.Run()
	result := ExecResult{Stdout: stdout.Summary(), Stderr: stderr.Summary(), ExitCode: 0, Err: err}
	if ctx.Err() == context.DeadlineExceeded {
		result.TimedOut = true
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exitErr.ExitCode()
	}
	if stdout.LimitExceeded() || stderr.LimitExceeded() {
		result.Err = fmt.Errorf("output limit exceeded")
	}
	return result
}

func requireNativeTrust(opts Options) error {
	switch opts.RecipeSource {
	case recipe.RecipeSourceBundled:
		return nil
	case recipe.RecipeSourceLocal:
		if opts.TrustEvaluation == nil || opts.TrustEvaluation.Status != recipe.TrustStatusTrusted || opts.TrustEvaluation.Source != recipe.RecipeSourceLocal || opts.TrustEvaluation.Target != opts.Recipe.Target {
			return blocked(CodeLocalTrustRequired, "local native operations require matching trusted local review evidence")
		}
		contentHash, err := recipe.RecipeContentSHA256(opts.Recipe)
		if err != nil {
			return blocked(CodeLocalTrustRequired, "local native operation trust evidence cannot be verified")
		}
		surface, surfaceHash, err := recipe.RecipeWriteSurface(opts.Recipe)
		if err != nil {
			return blocked(CodeLocalTrustRequired, "local native operation trust surface cannot be verified")
		}
		if opts.TrustEvaluation.ContentSHA256 != contentHash || opts.TrustEvaluation.WriteSurfaceSHA256 != surfaceHash {
			return blocked(CodeLocalTrustRequired, "local native operation trust evidence does not match the current recipe")
		}
		if !opts.TrustEvaluation.ReviewedNativeOperations {
			return blocked(CodeLocalTrustRequired, "local native operations require reviewed native-operation trust evidence")
		}
		if !reflect.DeepEqual(opts.TrustEvaluation.WriteSurface.NativeOperations, surface.NativeOperations) {
			return blocked(CodeLocalTrustRequired, "local native operation trust evidence does not match the recipe native surface")
		}
		return nil
	case "":
		return blocked(CodeLocalTrustRequired, "native operation recipe source is required")
	default:
		return blocked(CodeLocalTrustRequired, "unknown recipe source for native operation")
	}
}

func resolveArgs(op recipe.NativeOperation, opts Options) ([]string, error) {
	args := make([]string, 0, len(op.Command.Args))
	for _, arg := range op.Command.Args {
		switch {
		case arg.Literal != "":
			if strings.Contains(arg.Literal, "{{") || strings.Contains(arg.Literal, "}}") {
				return nil, blocked(CodePathInvalid, "literal args must not contain interpolation syntax")
			}
			args = append(args, arg.Literal)
		case arg.Input != "":
			path, err := resolveNamedPath(op.Inputs, arg.Input, opts)
			if err != nil {
				return nil, err
			}
			args = append(args, path)
		case arg.Output != "":
			path, err := resolveNamedPath(op.Outputs, arg.Output, opts)
			if err != nil {
				return nil, err
			}
			args = append(args, path)
		case arg.Temp != "":
			path, err := resolveNamedPath(op.TempPaths, arg.Temp, opts)
			if err != nil {
				return nil, err
			}
			args = append(args, path)
		default:
			return nil, blocked(CodePathInvalid, "native args must be typed whole tokens")
		}
	}
	return args, nil
}

func resolveEnv(op recipe.NativeOperation, opts Options) ([]string, error) {
	keys := make([]string, 0, len(op.Env))
	for key := range op.Env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	env := make([]string, 0, len(keys))
	for _, key := range keys {
		if !safeEnvKey(key) {
			return nil, blocked(CodeEnvInvalid, "unsupported native env key: "+key)
		}
		if sensitiveEnvKey(key) {
			return nil, blocked(CodeEnvInvalid, "sensitive native env keys are blocked: "+key)
		}
		value := op.Env[key]
		resolved := ""
		var err error
		switch {
		case value.Literal != "":
			resolved = value.Literal
		case value.Input != "":
			resolved, err = resolveNamedPath(op.Inputs, value.Input, opts)
		case value.Output != "":
			resolved, err = resolveNamedPath(op.Outputs, value.Output, opts)
		case value.Temp != "":
			resolved, err = resolveNamedPath(op.TempPaths, value.Temp, opts)
		default:
			err = blocked(CodeEnvInvalid, "native env values must be typed")
		}
		if err != nil {
			return nil, err
		}
		env = append(env, key+"="+resolved)
	}
	return env, nil
}

func outputSummaries(op recipe.NativeOperation, opts Options) ([]PathSummary, error) {
	ids := make([]string, 0, len(op.Outputs))
	for id := range op.Outputs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]PathSummary, 0, len(ids))
	for _, id := range ids {
		resolved, err := resolveNamedPath(op.Outputs, id, opts)
		if err != nil {
			return nil, err
		}
		out = append(out, PathSummary{ID: id, Root: op.Outputs[id].Root, Path: redactedPath(resolved)})
	}
	return out, nil
}

func resolveNamedPath(specs map[string]recipe.NativePathSpec, id string, opts Options) (string, error) {
	spec, ok := specs[id]
	if !ok {
		return "", blocked(CodePathInvalid, "undeclared native path reference: "+id)
	}
	base := ""
	switch spec.Root {
	case "artifact":
		base = opts.ArtifactRoot
	case "temp":
		base = opts.TempRoot
	case "location":
		base = opts.LocationRoots[spec.Location]
		if base == "" {
			return "", blocked(CodePathInvalid, "missing named location root: "+spec.Location)
		}
	default:
		return "", blocked(CodePathInvalid, "unsupported native path root: "+spec.Root)
	}
	baseAbs, err := cleanAbsDir(base, spec.Root+" root")
	if err != nil {
		return "", blocked(CodePathInvalid, err.Error())
	}
	if !safeRelPath(spec.Path) {
		return "", blocked(CodePathInvalid, "native path is not a safe relative path: "+spec.Path)
	}
	candidate := filepath.Join(baseAbs, filepath.FromSlash(spec.Path))
	inside, err := pathInside(baseAbs, candidate)
	if err != nil {
		return "", blocked(CodePathInvalid, err.Error())
	}
	if !inside {
		return "", blocked(CodePathInvalid, "native path escapes declared root")
	}
	return candidate, nil
}

func cleanAbsDir(path string, label string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", fmt.Errorf("%s is required", label)
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", err
	}
	if filepath.Clean(abs) != abs {
		return "", fmt.Errorf("%s must be clean", label)
	}
	return abs, nil
}

func pathInside(base string, candidate string) (bool, error) {
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return false, err
	}
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return false, err
	}
	rel, err := filepath.Rel(baseAbs, candidateAbs)
	if err != nil {
		return false, err
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)), nil
}

func safeRelPath(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed != value || filepath.IsAbs(trimmed) || strings.Contains(trimmed, "\\") {
		return false
	}
	cleaned := filepath.Clean(filepath.FromSlash(trimmed))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return false
	}
	return filepath.ToSlash(cleaned) == trimmed
}

func blocked(code string, message string) *Error {
	return &Error{Code: code, Message: message}
}

func blockedResult(operationID string, err error, elapsed time.Duration) Result {
	code := CodeRecipeInvalid
	if nativeErr, ok := err.(*Error); ok {
		code = nativeErr.Code
	}
	return Result{OperationID: operationID, Status: StatusBlocked, DurationMillis: elapsed.Milliseconds(), Diagnostics: []Diagnostic{diagnostic(code, operationID, err.Error())}}
}

func diagnostic(code string, operationID string, message string) Diagnostic {
	return Diagnostic{Code: code, Severity: SeverityError, Message: message, OperationID: operationID}
}

func stringIn(value string, list []string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

func intIn(value int, list []int) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

func blockedExecutable(base string) bool {
	switch strings.ToLower(strings.TrimSpace(base)) {
	case "sh", "sh.exe", "bash", "bash.exe", "zsh", "zsh.exe", "fish", "fish.exe",
		"osascript", "cmd", "cmd.exe", "powershell", "powershell.exe", "pwsh", "pwsh.exe",
		"wscript", "wscript.exe", "cscript", "cscript.exe", "mshta", "mshta.exe",
		"rundll32", "rundll32.exe", "regsvr32", "regsvr32.exe":
		return true
	default:
		return false
	}
}

func safeEnvKey(key string) bool {
	upper := strings.ToUpper(strings.TrimSpace(key))
	if !strings.HasPrefix(upper, "DFM_") {
		return false
	}
	for _, marker := range []string{"PATH", "LD_", "DYLD_", "PYTHONPATH", "NODE_OPTIONS", "RUBYOPT", "GIT_", "SHELL", "HOME", "COMSPEC", "PATHEXT", "SYSTEMROOT"} {
		if strings.Contains(upper, marker) {
			return false
		}
	}
	return true
}

func sensitiveEnvKey(key string) bool {
	upper := strings.ToUpper(key)
	for _, marker := range []string{"TOKEN", "KEY", "PASSWORD", "PASS", "SECRET", "CREDENTIAL", "AUTH"} {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	return false
}

func redactedPath(path string) string {
	return filepath.Base(path)
}

type captureWriter struct {
	policy        recipe.NativeStreamPolicy
	buf           bytes.Buffer
	written       int64
	limitExceeded bool
}

func newCaptureWriter(policy recipe.NativeStreamPolicy) *captureWriter {
	return &captureWriter{policy: policy}
}

func (w *captureWriter) Write(p []byte) (int, error) {
	w.written += int64(len(p))
	if w.policy.Mode == "discard" {
		_, _ = io.Discard.Write(p)
		return len(p), nil
	}
	if w.policy.Mode != "capture" {
		return len(p), nil
	}
	remaining := int64(w.policy.MaxBytes) - int64(w.buf.Len())
	if remaining <= 0 {
		w.limitExceeded = true
		return 0, fmt.Errorf("capture limit exceeded")
	}
	if int64(len(p)) > remaining {
		w.buf.Write(p[:remaining])
		w.limitExceeded = true
		return int(remaining), fmt.Errorf("capture limit exceeded")
	}
	return w.buf.Write(p)
}

func (w *captureWriter) Summary() CaptureSummary {
	summary := CaptureSummary{Mode: w.policy.Mode, Bytes: w.written, LimitExceeded: w.limitExceeded}
	if w.policy.Mode == "capture" && w.buf.Len() > 0 {
		sum := sha256.Sum256(w.buf.Bytes())
		summary.SHA256 = hex.EncodeToString(sum[:])
	}
	return summary
}

func (w *captureWriter) LimitExceeded() bool {
	return w.limitExceeded
}
