package tomldriver

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/shpoont/dotfiles-manager/internal/v2/filedriver"
)

const NormalizerID = "toml-file.selected-scalar.v1"

type CreatePolicy string

const (
	CreatePolicyReject CreatePolicy = "reject"
	CreatePolicyCreate CreatePolicy = "create"
)

type DeletePolicy string

const (
	DeletePolicyReject DeletePolicy = "reject"
	DeletePolicyAllow  DeletePolicy = "allow"
)

type DuplicatePolicy string

const (
	DuplicatePolicyReject DuplicatePolicy = "reject"
)

type DesiredIntent string

const (
	IntentSet    DesiredIntent = "set"
	IntentDelete DesiredIntent = "delete"
)

type Driver struct{}

type Selector struct {
	Path            []string        `json:"path"`
	CreateMissing   CreatePolicy    `json:"createMissing,omitempty"`
	DeleteKey       DeletePolicy    `json:"deleteKey,omitempty"`
	DuplicatePolicy DuplicatePolicy `json:"duplicatePolicy,omitempty"`
}

type Request struct {
	Target   filedriver.Target `json:"target"`
	Selector Selector          `json:"selector"`
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
}

type ApplyResult struct {
	Preview Preview `json:"preview"`
	Mutated bool    `json:"mutated"`
}

type VerifyResult struct {
	Verified bool `json:"verified"`
	Change   Diff `json:"change"`
}

type BackupRequest struct {
	Request    Request `json:"request"`
	Path       string  `json:"path"`
	Before     State   `json:"before"`
	BeforeFile []byte  `json:"-"`
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

func (d Driver) Detect(req Request) (Detection, error) {
	resolved, err := resolveRequest(req)
	if err != nil {
		return Detection{}, err
	}
	info, err := os.Stat(resolved.AbsPath)
	if os.IsNotExist(err) {
		return Detection{Exists: false, Readable: false, Path: resolved.AbsPath}, nil
	}
	if err != nil {
		return Detection{}, classifyOSError("detect", resolved.AbsPath, err)
	}
	if !info.Mode().IsRegular() {
		return Detection{}, driverError(filedriver.CodeInvalidSelector, "detect", resolved.AbsPath, fmt.Errorf("path is not a regular file"))
	}
	file, err := os.Open(resolved.AbsPath)
	if err != nil {
		return Detection{}, classifyOSError("detect", resolved.AbsPath, err)
	}
	_ = file.Close()
	return Detection{Exists: true, Readable: true, Path: resolved.AbsPath}, nil
}

func (d Driver) ReadCurrent(req Request) (State, error) {
	resolved, err := resolveRequest(req)
	if err != nil {
		return State{}, err
	}
	data, err := readRawFile(resolved.AbsPath)
	if err != nil {
		return State{}, err
	}
	if data == nil {
		return AbsentState(), nil
	}
	root, err := parseTOMLDocument(data)
	if err != nil {
		return State{}, driverError(filedriver.CodeInvalidSelector, "readCurrent", resolved.AbsPath, err)
	}
	selected, err := selectScalar(root, req.Selector)
	if err != nil {
		return State{}, driverError(filedriver.CodeInvalidSelector, "readCurrent", resolved.AbsPath, err)
	}
	if !selected.exists {
		return AbsentState(), nil
	}
	return stateFromCanonical(selected.canonical), nil
}

func (d Driver) Normalize(raw []byte) (State, error) {
	canonical, err := normalizeScalarTOML(raw)
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

func (d Driver) PreviewApply(req Request, desired State) (Preview, error) {
	resolved, err := resolveRequest(req)
	if err != nil {
		return Preview{}, err
	}
	desired, err = normalizeDesired(desired)
	if err != nil {
		return Preview{}, driverError(filedriver.CodeInvalidSelector, "previewApply", resolved.AbsPath, err)
	}
	current, err := d.ReadCurrent(req)
	if err != nil {
		return Preview{}, err
	}
	change := d.Diff(current, desired)
	if change.Kind != filedriver.ChangeUnchanged || desired.Intent == IntentDelete {
		if _, err := d.renderDesired(req, desired); err != nil {
			return Preview{}, err
		}
	}
	return Preview{Request: req, Path: resolved.AbsPath, Change: change, Normalizer: NormalizerID, Intent: desired.Intent}, nil
}

func (d Driver) Backup(req Request, hook BackupHook) (BackupResult, error) {
	resolved, err := resolveRequest(req)
	if err != nil {
		return BackupResult{}, err
	}
	before, err := d.ReadCurrent(req)
	if err != nil {
		return BackupResult{}, err
	}
	raw, err := readRawFile(resolved.AbsPath)
	if err != nil {
		return BackupResult{}, err
	}
	if hook == nil {
		return BackupResult{ID: "noop", Before: before.Snapshot()}, nil
	}
	result, err := hook(BackupRequest{Request: req, Path: resolved.AbsPath, Before: before, BeforeFile: raw})
	if err != nil {
		return BackupResult{}, fmt.Errorf("backup hook for %s: %w", resolved.AbsPath, err)
	}
	if result.Before == (Snapshot{}) {
		result.Before = before.Snapshot()
	}
	return result, nil
}

func (d Driver) Apply(req Request, desired State) (ApplyResult, error) {
	result, _, err := d.ApplyWithBackup(req, desired, nil)
	return result, err
}

func (d Driver) ApplyWithBackup(req Request, desired State, hook BackupHook) (ApplyResult, *BackupResult, error) {
	preview, err := d.PreviewApply(req, desired)
	if err != nil {
		return ApplyResult{}, nil, err
	}
	if preview.Change.Kind == filedriver.ChangeUnchanged {
		return ApplyResult{Preview: preview, Mutated: false}, nil, nil
	}
	backup, err := d.Backup(req, hook)
	if err != nil {
		return ApplyResult{}, nil, err
	}
	data, err := d.renderDesired(req, desired)
	if err != nil {
		return ApplyResult{}, nil, err
	}
	if err := writeTarget(req.Target, data); err != nil {
		return ApplyResult{}, nil, err
	}
	if _, err := d.Verify(req, desired); err != nil {
		return ApplyResult{}, nil, err
	}
	return ApplyResult{Preview: preview, Mutated: true}, &backup, nil
}

func (d Driver) Verify(req Request, desired State) (VerifyResult, error) {
	resolved, err := resolveRequest(req)
	if err != nil {
		return VerifyResult{}, err
	}
	desired, err = normalizeDesired(desired)
	if err != nil {
		return VerifyResult{}, driverError(filedriver.CodeInvalidSelector, "verify", resolved.AbsPath, err)
	}
	current, err := d.ReadCurrent(req)
	if err != nil {
		return VerifyResult{}, err
	}
	change := d.Diff(current, desired)
	if change.Kind != filedriver.ChangeUnchanged {
		return VerifyResult{Verified: false, Change: change}, driverError(filedriver.CodeVerificationFailed, "verify", resolved.AbsPath, fmt.Errorf("post-apply selected value differs: %s", change.Kind))
	}
	return VerifyResult{Verified: true, Change: change}, nil
}

func (d Driver) Restore(req Request, backup BackupResult, hook RestoreHook) error {
	resolved, err := resolveRequest(req)
	if err != nil {
		return err
	}
	if hook == nil {
		return driverError(filedriver.CodeUnsupported, "restore", resolved.AbsPath, fmt.Errorf("restore requires a caller-provided whole-file restore hook in this slice"))
	}
	if err := hook(RestoreRequest{Request: req, Path: resolved.AbsPath, Backup: backup}); err != nil {
		return fmt.Errorf("restore hook for %s: %w", resolved.AbsPath, err)
	}
	return nil
}

func (d Driver) renderDesired(req Request, desired State) ([]byte, error) {
	resolved, err := resolveRequest(req)
	if err != nil {
		return nil, err
	}
	desired, err = normalizeDesired(desired)
	if err != nil {
		return nil, driverError(filedriver.CodeInvalidSelector, "render", resolved.AbsPath, err)
	}
	data, err := readRawFile(resolved.AbsPath)
	if err != nil {
		return nil, err
	}
	var root any
	if data != nil {
		root, err = parseTOMLDocument(data)
		if err != nil {
			return nil, driverError(filedriver.CodeInvalidSelector, "render", resolved.AbsPath, err)
		}
	}
	if desired.Exists {
		value, err := parseDesiredScalar(desired.Value)
		if err != nil {
			return nil, driverError(filedriver.CodeInvalidSelector, "render", resolved.AbsPath, err)
		}
		root, err = setSelected(root, req.Selector, value)
		if err != nil {
			return nil, driverError(filedriver.CodeInvalidSelector, "render", resolved.AbsPath, err)
		}
		return marshalDocument(root)
	}
	root, err = deleteSelected(root, req.Selector)
	if err != nil {
		return nil, driverError(filedriver.CodeInvalidSelector, "render", resolved.AbsPath, err)
	}
	if root == nil {
		return nil, nil
	}
	return marshalDocument(root)
}

func resolveRequest(req Request) (filedriver.ResolvedPath, error) {
	if err := validateSelector(req.Selector); err != nil {
		return filedriver.ResolvedPath{}, driverError(filedriver.CodeInvalidSelector, "selector", "", err)
	}
	return filedriver.ResolveTarget(req.Target)
}

func validateSelector(selector Selector) error {
	if len(selector.Path) == 0 {
		return fmt.Errorf("selector path is required")
	}
	for idx, segment := range selector.Path {
		if segment == "" {
			return fmt.Errorf("selector path segment %d is required", idx)
		}
		if strings.ContainsAny(segment, "\r\n\x00") {
			return fmt.Errorf("selector path segment %d must not contain CR, LF, or NUL", idx)
		}
		if isExpressionSegment(segment) {
			return fmt.Errorf("selector path segment %d looks like an expression, not an table key", idx)
		}
	}
	switch selectorCreatePolicy(selector) {
	case CreatePolicyReject, CreatePolicyCreate:
	default:
		return fmt.Errorf("unsupported createMissing policy %q", selector.CreateMissing)
	}
	switch selectorDeletePolicy(selector) {
	case DeletePolicyReject, DeletePolicyAllow:
	default:
		return fmt.Errorf("unsupported deleteKey policy %q", selector.DeleteKey)
	}
	switch selectorDuplicatePolicy(selector) {
	case DuplicatePolicyReject:
	default:
		return fmt.Errorf("unsupported duplicatePolicy %q", selector.DuplicatePolicy)
	}
	return nil
}

func isExpressionSegment(segment string) bool {
	return segment == "*" || segment == "$" || segment == "." || segment == ".." || strings.ContainsAny(segment, "[]") || strings.HasPrefix(segment, "$.")
}

func selectorCreatePolicy(selector Selector) CreatePolicy {
	if selector.CreateMissing == "" {
		return CreatePolicyReject
	}
	return selector.CreateMissing
}

func selectorDeletePolicy(selector Selector) DeletePolicy {
	if selector.DeleteKey == "" {
		return DeletePolicyReject
	}
	return selector.DeleteKey
}

func selectorDuplicatePolicy(selector Selector) DuplicatePolicy {
	if selector.DuplicatePolicy == "" {
		return DuplicatePolicyReject
	}
	return selector.DuplicatePolicy
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

type selectedScalar struct {
	exists    bool
	canonical []byte
}

func selectScalar(root any, selector Selector) (selectedScalar, error) {
	current := root
	for idx, segment := range selector.Path {
		object, ok := current.(map[string]any)
		if !ok {
			return selectedScalar{}, fmt.Errorf("selector path segment %q requires table container", selector.Path[idx])
		}
		value, ok := object[segment]
		if !ok {
			return selectedScalar{exists: false}, nil
		}
		if idx == len(selector.Path)-1 {
			canonical, err := canonicalScalar(value)
			if err != nil {
				return selectedScalar{}, fmt.Errorf("selected value at path %s must be a supported TOML scalar: %w", strings.Join(selector.Path, "."), err)
			}
			return selectedScalar{exists: true, canonical: canonical}, nil
		}
		current = value
	}
	return selectedScalar{exists: false}, nil
}

func setSelected(root any, selector Selector, value any) (any, error) {
	value, err := tomlScalarValue(value)
	if err != nil {
		return nil, fmt.Errorf("desired selected value must be a supported TOML scalar: %w", err)
	}
	if root == nil {
		if selectorCreatePolicy(selector) != CreatePolicyCreate {
			return nil, fmt.Errorf("missing TOML document requires createMissing policy %q", CreatePolicyCreate)
		}
		root = map[string]any{}
	}
	current, ok := root.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("selector root requires table document")
	}
	for idx, segment := range selector.Path {
		if idx == len(selector.Path)-1 {
			existing, exists := current[segment]
			if !exists && selectorCreatePolicy(selector) != CreatePolicyCreate {
				return nil, fmt.Errorf("missing selected path requires createMissing policy %q", CreatePolicyCreate)
			}
			if exists {
				if _, err := tomlScalarValue(existing); err != nil {
					return nil, fmt.Errorf("existing selected value at path %s must be a supported TOML scalar: %w", strings.Join(selector.Path, "."), err)
				}
			}
			current[segment] = value
			return root, nil
		}
		next, exists := current[segment]
		if !exists {
			if selectorCreatePolicy(selector) != CreatePolicyCreate {
				return nil, fmt.Errorf("missing selector container requires createMissing policy %q", CreatePolicyCreate)
			}
			child := map[string]any{}
			current[segment] = child
			current = child
			continue
		}
		child, ok := next.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("selector path segment %q requires table container", segment)
		}
		current = child
	}
	return root, nil
}

func deleteSelected(root any, selector Selector) (any, error) {
	if selectorDeletePolicy(selector) != DeletePolicyAllow {
		return nil, fmt.Errorf("delete intent requires deleteKey policy %q", DeletePolicyAllow)
	}
	if root == nil {
		return nil, nil
	}
	current, ok := root.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("selector root requires table document")
	}
	for idx, segment := range selector.Path {
		if idx == len(selector.Path)-1 {
			if existing, exists := current[segment]; exists {
				if _, err := tomlScalarValue(existing); err != nil {
					return nil, fmt.Errorf("selected value at path %s must be a supported TOML scalar: %w", strings.Join(selector.Path, "."), err)
				}
			}
			delete(current, segment)
			return root, nil
		}
		next, exists := current[segment]
		if !exists {
			return root, nil
		}
		child, ok := next.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("selector path segment %q requires table container", segment)
		}
		current = child
	}
	return root, nil
}

func normalizeScalarTOML(raw []byte) ([]byte, error) {
	value, err := parseJSONScalar(raw)
	if err != nil {
		return nil, err
	}
	return canonicalScalar(value)
}

func parseDesiredScalar(raw []byte) (any, error) {
	value, err := parseJSONScalar(raw)
	if err != nil {
		return nil, err
	}
	return tomlScalarValue(value)
}

func isTOMLScalar(value any) bool {
	_, err := tomlScalarValue(value)
	return err == nil
}

func canonicalScalar(value any) ([]byte, error) {
	canonicalValue, err := tomlScalarValue(value)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(canonicalValue)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func marshalDocument(value any) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	return toml.Marshal(value)
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
	} else if !errorsIsEOF(err) {
		return nil, err
	}
	return value, nil
}

func parseTOMLDocument(data []byte) (any, error) {
	root := map[string]any{}
	if err := toml.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	if root == nil {
		root = map[string]any{}
	}
	return root, nil
}

func tomlScalarValue(value any) (any, error) {
	switch typed := value.(type) {
	case string:
		return typed, nil
	case bool:
		return typed, nil
	case json.Number:
		if _, err := json.Marshal(typed); err != nil {
			return nil, fmt.Errorf("TOML number must be a valid JSON number")
		}
		if integer, err := typed.Int64(); err == nil {
			return integer, nil
		}
		floating, err := typed.Float64()
		if err != nil {
			return nil, fmt.Errorf("TOML number must be a valid integer or finite float")
		}
		if math.IsNaN(floating) || math.IsInf(floating, 0) {
			return nil, fmt.Errorf("TOML float must be finite")
		}
		return floating, nil
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
		return nil, fmt.Errorf("TOML selected values do not support null")
	case toml.LocalDate, toml.LocalDateTime, toml.LocalTime, time.Time:
		return nil, fmt.Errorf("TOML date/time values are unsupported selected scalars")
	case []any:
		return nil, fmt.Errorf("TOML arrays are unsupported selected scalars")
	case map[string]any:
		return nil, fmt.Errorf("TOML tables are unsupported selected scalars")
	default:
		return nil, fmt.Errorf("unsupported TOML selected scalar type %T", value)
	}
}

func unsignedToInt64(value uint64) (int64, error) {
	if value > math.MaxInt64 {
		return 0, fmt.Errorf("TOML integer exceeds int64 range")
	}
	return int64(value), nil
}

func finiteFloat64(value float64) (float64, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("TOML float must be finite")
	}
	return value, nil
}

func errorsIsEOF(err error) bool {
	return err == io.EOF
}

func readRawFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, classifyOSError("readRaw", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, driverError(filedriver.CodeInvalidSelector, "readRaw", path, fmt.Errorf("path is not a regular file"))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, classifyOSError("readRaw", path, err)
	}
	return data, nil
}

func writeTarget(target filedriver.Target, data []byte) error {
	resolved, err := filedriver.ResolveTarget(target)
	if err != nil {
		return err
	}
	writePath := resolved.AbsPath
	if linkInfo, err := os.Lstat(resolved.AbsPath); err == nil && linkInfo.Mode()&os.ModeSymlink != 0 {
		real, err := filepath.EvalSymlinks(resolved.AbsPath)
		if err != nil {
			return classifyOSError("apply", resolved.AbsPath, err)
		}
		if err := ensureInside(resolved.RootReal, real); err != nil {
			return driverError(filedriver.CodeUnsafePath, "apply", resolved.AbsPath, err)
		}
		writePath = real
	}
	if info, err := os.Stat(writePath); err == nil && !info.Mode().IsRegular() {
		return driverError(filedriver.CodeInvalidSelector, "apply", writePath, fmt.Errorf("path is not a regular file"))
	} else if err != nil && !os.IsNotExist(err) {
		return classifyOSError("apply", writePath, err)
	}
	if err := os.MkdirAll(filepath.Dir(writePath), 0o755); err != nil {
		return classifyOSError("apply", filepath.Dir(writePath), err)
	}
	if err := writeFileAtomic(writePath, data); err != nil {
		return classifyOSError("apply", writePath, err)
	}
	return nil
}

func writeFileAtomic(path string, data []byte) error {
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	parent := filepath.Dir(path)
	tmp, err := os.CreateTemp(parent, ".dfm-toml-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func ensureInside(root string, candidate string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(rootAbs, candidateAbs)
	if err != nil {
		return err
	}
	relSlash := filepath.ToSlash(rel)
	if relSlash == ".." || strings.HasPrefix(relSlash, "../") || filepath.IsAbs(rel) {
		return fmt.Errorf("path escapes root: %s", candidate)
	}
	return nil
}

func classifyOSError(op string, path string, err error) error {
	if os.IsPermission(err) {
		return driverError(filedriver.CodePermissionDenied, op, path, err)
	}
	if os.IsNotExist(err) {
		return driverError(filedriver.CodeNotFound, op, path, err)
	}
	return driverError(filedriver.CodeInternal, op, path, err)
}

func driverError(code filedriver.ErrorCode, op string, path string, err error) error {
	return &filedriver.Error{Code: code, Op: op, Path: path, Err: err}
}
