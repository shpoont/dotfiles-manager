package macosdefaultsdriver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/shpoont/dotfiles-manager/internal/v2/filedriver"
	"howett.net/plist"
)

const NormalizerID = "macos-defaults-readonly.selected-scalar.v1"

const (
	LogicalLocationID  = "macos-defaults"
	LogicalLocationURI = "macos-defaults://current-user"
)

const (
	DefaultTimeout        = 5 * time.Second
	DefaultMaxStdoutBytes = 1 << 20
	DefaultMaxStderrBytes = 16 << 10
)

type DesiredIntent string

const (
	IntentSet    DesiredIntent = "set"
	IntentDelete DesiredIntent = "delete"
)

type Driver struct{}

type Runner interface {
	Export(ctx context.Context, domain string, limits OutputLimits) (ExportResult, error)
}

type OutputLimits struct {
	StdoutBytes int `json:"stdoutBytes,omitempty"`
	StderrBytes int `json:"stderrBytes,omitempty"`
}

type ExportResult struct {
	Stdout   []byte `json:"-"`
	Stderr   []byte `json:"-"`
	ExitCode int    `json:"exitCode"`
}

type Request struct {
	Domain  string        `json:"domain"`
	Key     string        `json:"key"`
	Runner  Runner        `json:"-"`
	Timeout time.Duration `json:"-"`
	Limits  OutputLimits  `json:"-"`
}

type Detection struct {
	Exists   bool   `json:"exists"`
	Readable bool   `json:"readable"`
	Path     string `json:"path"`
}

type State struct {
	Exists     bool          `json:"exists"`
	Value      []byte        `json:"-"`
	SHA256     string        `json:"sha256,omitempty"`
	Normalizer string        `json:"normalizer"`
	Intent     DesiredIntent `json:"intent,omitempty"`
}

type Snapshot struct {
	Exists bool   `json:"exists"`
	SHA256 string `json:"sha256,omitempty"`
}

type Diff struct {
	Kind   filedriver.ChangeKind `json:"kind"`
	Before Snapshot              `json:"before"`
	After  Snapshot              `json:"after"`
}

type Preview struct {
	Request    Request       `json:"request"`
	Path       string        `json:"path"`
	Change     Diff          `json:"change"`
	Normalizer string        `json:"normalizer"`
	Intent     DesiredIntent `json:"intent"`
	ReadOnly   bool          `json:"readOnly"`
}

type ApplyResult struct {
	Preview Preview `json:"preview"`
	Mutated bool    `json:"mutated"`
}

type BackupRequest struct {
	Request Request `json:"request"`
	Path    string  `json:"path"`
	Before  State   `json:"before"`
}

type BackupResult struct {
	ID     string   `json:"id"`
	Before Snapshot `json:"before"`
}

type BackupHook func(BackupRequest) (BackupResult, error)

type RestoreRequest struct {
	Request Request      `json:"request"`
	Path    string       `json:"path"`
	Backup  BackupResult `json:"backup"`
}

type RestoreHook func(RestoreRequest) error

type systemRunner struct{}

type unsupportedRunner struct{}

type outputLimitError struct{ stream string }

type timeoutError struct{}

type unsupportedRuntimeError struct{}

func (e outputLimitError) Error() string { return e.stream + " exceeded configured output limit" }
func (e timeoutError) Error() string     { return "defaults export timed out" }
func (e unsupportedRuntimeError) Error() string {
	return "macOS defaults are available only on darwin"
}

func (d Driver) Detect(req Request) (Detection, error) {
	state, err := d.ReadCurrent(req)
	if err != nil {
		return Detection{}, err
	}
	return Detection{Exists: state.Exists, Readable: true, Path: LogicalPath(req.Domain, req.Key)}, nil
}

func (d Driver) ReadCurrent(req Request) (State, error) {
	if err := ValidateRequest(req); err != nil {
		return State{}, driverError(filedriver.CodeInvalidSelector, "selector", LogicalPath(req.Domain, req.Key), err)
	}
	root, err := readDomain(req)
	if err != nil {
		return State{}, err
	}
	value, exists := root[req.Key]
	if !exists {
		return AbsentState(), nil
	}
	canonical, err := canonicalScalar(value)
	if err != nil {
		return State{}, driverError(filedriver.CodeInvalidSelector, "read", LogicalPath(req.Domain, req.Key), fmt.Errorf("selected defaults key %s must be a supported scalar: %w", SelectorSummary(req.Domain, req.Key), err))
	}
	return stateFromCanonical(canonical), nil
}

func (d Driver) Normalize(raw []byte) (State, error) {
	value, err := parseJSONScalar(raw)
	if err != nil {
		return State{}, err
	}
	canonical, err := canonicalScalar(value)
	if err != nil {
		return State{}, err
	}
	return stateFromCanonical(canonical), nil
}

func (d Driver) NormalizeValue(value any) (State, error) {
	canonical, err := canonicalScalar(value)
	if err != nil {
		return State{}, err
	}
	return stateFromCanonical(canonical), nil
}

func AbsentState() State {
	return State{Exists: false, Normalizer: NormalizerID}
}

func DeleteState() State {
	return State{Exists: false, Normalizer: NormalizerID, Intent: IntentDelete}
}

func (s State) Snapshot() Snapshot {
	if !s.Exists {
		return Snapshot{Exists: false}
	}
	return Snapshot{Exists: true, SHA256: s.SHA256}
}

func (d Driver) Diff(current State, desired State) Diff {
	before := current.Snapshot()
	after := desired.Snapshot()
	switch {
	case !current.Exists && !desired.Exists:
		return Diff{Kind: filedriver.ChangeUnchanged, Before: before, After: after}
	case !current.Exists && desired.Exists:
		return Diff{Kind: filedriver.ChangeCreate, Before: before, After: after}
	case current.Exists && !desired.Exists:
		return Diff{Kind: filedriver.ChangeDelete, Before: before, After: after}
	case current.SHA256 == desired.SHA256 && bytes.Equal(current.Value, desired.Value):
		return Diff{Kind: filedriver.ChangeUnchanged, Before: before, After: after}
	default:
		return Diff{Kind: filedriver.ChangeUpdate, Before: before, After: after}
	}
}

func (d Driver) PreviewDiff(req Request, desired State) (Preview, error) {
	if err := ValidateRequest(req); err != nil {
		return Preview{}, driverError(filedriver.CodeInvalidSelector, "selector", LogicalPath(req.Domain, req.Key), err)
	}
	desired, err := normalizeDesired(desired)
	if err != nil {
		return Preview{}, driverError(filedriver.CodeInvalidSelector, "preview", LogicalPath(req.Domain, req.Key), err)
	}
	current, err := d.ReadCurrent(req)
	if err != nil {
		return Preview{}, err
	}
	return Preview{Request: sanitizedRequest(req), Path: LogicalPath(req.Domain, req.Key), Change: d.Diff(current, desired), Normalizer: NormalizerID, Intent: desired.Intent, ReadOnly: true}, nil
}

func (d Driver) Backup(req Request, hook BackupHook) (BackupResult, error) {
	if err := ValidateRequest(req); err != nil {
		return BackupResult{}, driverError(filedriver.CodeInvalidSelector, "selector", LogicalPath(req.Domain, req.Key), err)
	}
	return BackupResult{}, readOnlyError("backup", LogicalPath(req.Domain, req.Key))
}

func (d Driver) Apply(req Request, desired State) (ApplyResult, error) {
	result, _, err := d.ApplyWithBackup(req, desired, nil)
	return result, err
}

func (d Driver) ApplyWithBackup(req Request, desired State, hook BackupHook) (ApplyResult, *BackupResult, error) {
	if err := ValidateRequest(req); err != nil {
		return ApplyResult{}, nil, driverError(filedriver.CodeInvalidSelector, "selector", LogicalPath(req.Domain, req.Key), err)
	}
	return ApplyResult{}, nil, readOnlyError("apply", LogicalPath(req.Domain, req.Key))
}

func (d Driver) Restore(req Request, backup BackupResult, hook RestoreHook) error {
	if err := ValidateRequest(req); err != nil {
		return driverError(filedriver.CodeInvalidSelector, "selector", LogicalPath(req.Domain, req.Key), err)
	}
	return readOnlyError("restore", LogicalPath(req.Domain, req.Key))
}

func ValidateRequest(req Request) error {
	if err := ValidateDomain(req.Domain); err != nil {
		return err
	}
	if err := ValidateKey(req.Key); err != nil {
		return err
	}
	return nil
}

func ValidateDomain(domain string) error {
	if domain == "" {
		return fmt.Errorf("defaults domain is required")
	}
	if strings.TrimSpace(domain) != domain {
		return fmt.Errorf("defaults domain must not have surrounding whitespace")
	}
	if strings.ContainsAny(domain, "\r\n\x00") {
		return fmt.Errorf("defaults domain must be a single-line string without NUL")
	}
	if strings.ContainsAny(domain, "/\\") {
		return fmt.Errorf("defaults domain must not contain slash or backslash")
	}
	if strings.HasPrefix(domain, "-") {
		return fmt.Errorf("defaults domain must not start with '-' due to command option ambiguity")
	}
	return nil
}

func ValidateKey(key string) error {
	if key == "" {
		return fmt.Errorf("defaults key is required")
	}
	if strings.TrimSpace(key) != key {
		return fmt.Errorf("defaults key must not have surrounding whitespace")
	}
	if strings.ContainsAny(key, "\r\n\x00") {
		return fmt.Errorf("defaults key must be a single-line string without NUL")
	}
	if strings.HasPrefix(key, "-") {
		return fmt.Errorf("defaults key must not start with '-' due to command option ambiguity")
	}
	return nil
}

func LogicalPath(domain string, key string) string {
	return "defaults://current-user/" + url.PathEscape(domain) + "/" + url.PathEscape(key)
}

func SelectorSummary(domain string, key string) string {
	domainJSON, _ := json.Marshal(domain)
	keyJSON, _ := json.Marshal(key)
	return "domain=" + string(domainJSON) + " key=" + string(keyJSON)
}

func readDomain(req Request) (map[string]any, error) {
	runner := req.Runner
	if runner == nil {
		runner = defaultRunner()
	}
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	limits := req.Limits
	if limits.StdoutBytes <= 0 {
		limits.StdoutBytes = DefaultMaxStdoutBytes
	}
	if limits.StderrBytes <= 0 {
		limits.StderrBytes = DefaultMaxStderrBytes
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	result, err := runner.Export(ctx, req.Domain, limits)
	if err != nil {
		return nil, classifyRunnerError(req, err)
	}
	if limits.StdoutBytes > 0 && len(result.Stdout) > limits.StdoutBytes {
		return nil, driverError(filedriver.CodeInternal, "read", LogicalPath(req.Domain, req.Key), fmt.Errorf("defaults export stdout exceeded output limit"))
	}
	if limits.StderrBytes > 0 && len(result.Stderr) > limits.StderrBytes {
		return nil, driverError(filedriver.CodeInternal, "read", LogicalPath(req.Domain, req.Key), fmt.Errorf("defaults export stderr exceeded output limit"))
	}
	if result.ExitCode != 0 {
		if isMissingDomain(result.Stderr) {
			return map[string]any{}, nil
		}
		return nil, driverError(filedriver.CodeInternal, "read", LogicalPath(req.Domain, req.Key), fmt.Errorf("defaults export failed: %s", stableStderrCategory(result.Stderr)))
	}
	var root any
	format, err := plist.Unmarshal(result.Stdout, &root)
	if err != nil {
		return nil, driverError(filedriver.CodeInvalidSelector, "read", LogicalPath(req.Domain, req.Key), fmt.Errorf("defaults export returned malformed plist output"))
	}
	if format != plist.XMLFormat {
		return nil, driverError(filedriver.CodeInvalidSelector, "read", LogicalPath(req.Domain, req.Key), fmt.Errorf("defaults export returned unsupported plist format"))
	}
	object, ok := normalizeContainers(root).(map[string]any)
	if !ok {
		return nil, driverError(filedriver.CodeInvalidSelector, "read", LogicalPath(req.Domain, req.Key), fmt.Errorf("defaults export root must be a dictionary"))
	}
	return object, nil
}

func defaultRunner() Runner {
	if runtime.GOOS != "darwin" {
		return unsupportedRunner{}
	}
	return systemRunner{}
}

func (r unsupportedRunner) Export(ctx context.Context, domain string, limits OutputLimits) (ExportResult, error) {
	return ExportResult{}, unsupportedRuntimeError{}
}

func (r systemRunner) Export(ctx context.Context, domain string, limits OutputLimits) (ExportResult, error) {
	stdout := &boundedBuffer{limit: limits.StdoutBytes, stream: "stdout"}
	stderr := &boundedBuffer{limit: limits.StderrBytes, stream: "stderr"}
	cmd := exec.CommandContext(ctx, "/usr/bin/defaults", "export", domain, "-")
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	result := ExportResult{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	if ctx.Err() != nil {
		return result, timeoutError{}
	}
	if stdout.truncated {
		return result, outputLimitError{stream: "stdout"}
	}
	if stderr.truncated {
		return result, outputLimitError{stream: "stderr"}
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
			return result, nil
		}
		return result, err
	}
	return result, nil
}

type boundedBuffer struct {
	limit     int
	stream    string
	buf       bytes.Buffer
	truncated bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.limit <= 0 {
		return len(p), nil
	}
	remaining := b.limit - b.buf.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = b.buf.Write(p[:remaining])
		b.truncated = true
		return len(p), nil
	}
	_, _ = b.buf.Write(p)
	return len(p), nil
}

func (b *boundedBuffer) Bytes() []byte { return b.buf.Bytes() }

func classifyRunnerError(req Request, err error) error {
	var driverErr *filedriver.Error
	if errors.As(err, &driverErr) {
		return err
	}
	var timeout timeoutError
	if errors.As(err, &timeout) {
		return driverError(filedriver.CodeInternal, "read", LogicalPath(req.Domain, req.Key), fmt.Errorf("defaults export timed out"))
	}
	var unsupported unsupportedRuntimeError
	if errors.As(err, &unsupported) {
		return driverError(filedriver.CodeUnsupported, "read", LogicalPath(req.Domain, req.Key), unsupported)
	}
	var limitErr outputLimitError
	if errors.As(err, &limitErr) {
		return driverError(filedriver.CodeInternal, "read", LogicalPath(req.Domain, req.Key), fmt.Errorf("defaults export %s exceeded output limit", limitErr.stream))
	}
	return driverError(filedriver.CodeInternal, "read", LogicalPath(req.Domain, req.Key), fmt.Errorf("defaults export command failed"))
}

func isMissingDomain(stderr []byte) bool {
	message := strings.ToLower(strings.TrimSpace(string(stderr)))
	return strings.Contains(message, "domain") && strings.Contains(message, "does not exist")
}

func stableStderrCategory(stderr []byte) string {
	if isMissingDomain(stderr) {
		return "missing-domain"
	}
	if len(bytes.TrimSpace(stderr)) == 0 {
		return "command-failed"
	}
	return "command-failed"
}

func normalizeDesired(desired State) (State, error) {
	if desired.Exists {
		if desired.Intent != "" && desired.Intent != IntentSet {
			return State{}, fmt.Errorf("desired set state must use intent %q", IntentSet)
		}
		state, err := Driver{}.Normalize(desired.Value)
		if err != nil {
			return State{}, err
		}
		state.Intent = IntentSet
		return state, nil
	}
	if desired.Intent != IntentDelete {
		return State{}, fmt.Errorf("delete desired state must use explicit intent %q", IntentDelete)
	}
	return DeleteState(), nil
}

func stateFromCanonical(canonical []byte) State {
	copyValue := append([]byte(nil), canonical...)
	sum := sha256.Sum256(copyValue)
	return State{Exists: true, Value: copyValue, SHA256: hex.EncodeToString(sum[:]), Normalizer: NormalizerID, Intent: IntentSet}
}

func parseJSONScalar(data []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return nil, fmt.Errorf("trailing JSON data after selected scalar")
	} else if !errors.Is(err, io.EOF) {
		return nil, err
	}
	return value, nil
}

func canonicalScalar(value any) ([]byte, error) {
	canonicalValue, err := defaultsScalarValue(value)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(canonicalValue)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func defaultsScalarValue(value any) (any, error) {
	switch typed := value.(type) {
	case string:
		return typed, nil
	case bool:
		return typed, nil
	case json.Number:
		if _, err := json.Marshal(typed); err != nil {
			return nil, fmt.Errorf("defaults number must be a valid JSON number")
		}
		if integer, err := typed.Int64(); err == nil {
			return integer, nil
		}
		floating, err := typed.Float64()
		if err != nil {
			return nil, fmt.Errorf("defaults number must be a valid integer or finite float")
		}
		return finiteFloat64(floating)
	case int:
		return int64(typed), nil
	case int8:
		return int64(typed), nil
	case int16:
		return int64(typed), nil
	case int32:
		return int64(typed), nil
	case int64:
		return typed, nil
	case uint:
		return unsignedToInt64(uint64(typed))
	case uint8:
		return int64(typed), nil
	case uint16:
		return int64(typed), nil
	case uint32:
		return int64(typed), nil
	case uint64:
		return unsignedToInt64(typed)
	case float32:
		return finiteFloat64(float64(typed))
	case float64:
		return finiteFloat64(typed)
	case nil:
		return nil, fmt.Errorf("defaults selected values do not support null")
	case []byte:
		return nil, fmt.Errorf("defaults data values are unsupported selected scalars")
	case time.Time:
		return nil, fmt.Errorf("defaults date values are unsupported selected scalars")
	case plist.UID:
		return nil, fmt.Errorf("defaults UID values are unsupported selected scalars")
	case []any:
		return nil, fmt.Errorf("defaults arrays are unsupported selected scalars")
	case map[string]any:
		return nil, fmt.Errorf("defaults dictionaries are unsupported selected scalars")
	default:
		return nil, fmt.Errorf("unsupported defaults selected scalar type %T", value)
	}
}

func unsignedToInt64(value uint64) (int64, error) {
	if value > math.MaxInt64 {
		return 0, fmt.Errorf("defaults integer exceeds int64 range")
	}
	return int64(value), nil
}

func finiteFloat64(value float64) (float64, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("defaults float must be finite")
	}
	return value, nil
}

func normalizeContainers(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := map[string]any{}
		for key, child := range typed {
			out[key] = normalizeContainers(child)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for idx, child := range typed {
			out[idx] = normalizeContainers(child)
		}
		return out
	default:
		return value
	}
}

func sanitizedRequest(req Request) Request {
	return Request{Domain: req.Domain, Key: req.Key, Timeout: req.Timeout, Limits: req.Limits}
}

func driverError(code filedriver.ErrorCode, op string, path string, err error) error {
	return &filedriver.Error{Code: code, Op: op, Path: path, Err: err}
}

func readOnlyError(op string, path string) error {
	return driverError(filedriver.CodeUnsupported, op, path, fmt.Errorf("macOS defaults driver is read-only; mutation, backup, restore, and export-as-desired are unsupported"))
}
