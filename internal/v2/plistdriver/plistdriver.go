package plistdriver

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/shpoont/dotfiles-manager/internal/v2/filedriver"
	"howett.net/plist"
)

const NormalizerID = "plist-file.selected-scalar.v1"

const (
	FormatXML    = "xml"
	FormatBinary = "binary"
)

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
	Format     string        `json:"format,omitempty"`
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
	doc, err := parsePlistDocument(data)
	if err != nil {
		return State{}, driverError(filedriver.CodeInvalidSelector, "readCurrent", resolved.AbsPath, err)
	}
	selected, err := selectScalar(doc.root, req.Selector)
	if err != nil {
		return State{}, driverError(filedriver.CodeInvalidSelector, "readCurrent", resolved.AbsPath, err)
	}
	if !selected.exists {
		return AbsentState(), nil
	}
	return stateFromCanonical(selected.canonical), nil
}

func (d Driver) Normalize(raw []byte) (State, error) {
	canonical, err := normalizeScalarPlist(raw)
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
	format, err := readDocumentFormat(resolved.AbsPath)
	if err != nil {
		return Preview{}, err
	}
	if format == "" && desired.Exists {
		format = FormatXML
	}
	change := d.Diff(current, desired)
	if change.Kind != filedriver.ChangeUnchanged || desired.Intent == IntentDelete {
		if _, err := d.renderDesired(req, desired); err != nil {
			return Preview{}, err
		}
	}
	return Preview{Request: req, Path: resolved.AbsPath, Format: format, Change: change, Normalizer: NormalizerID, Intent: desired.Intent}, nil
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
	format := plist.XMLFormat
	var root any
	if data != nil {
		doc, err := parsePlistDocument(data)
		if err != nil {
			return nil, driverError(filedriver.CodeInvalidSelector, "render", resolved.AbsPath, err)
		}
		root = doc.root
		format = doc.format
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
		return marshalDocument(root, format)
	}
	root, err = deleteSelected(root, req.Selector)
	if err != nil {
		return nil, driverError(filedriver.CodeInvalidSelector, "render", resolved.AbsPath, err)
	}
	if root == nil {
		return nil, nil
	}
	return marshalDocument(root, format)
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
			return fmt.Errorf("selector path segment %d looks like an expression, not a dictionary key", idx)
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
			return selectedScalar{}, fmt.Errorf("selector path segment %q requires dictionary container", selector.Path[idx])
		}
		value, ok := object[segment]
		if !ok {
			return selectedScalar{exists: false}, nil
		}
		if idx == len(selector.Path)-1 {
			canonical, err := canonicalScalar(value)
			if err != nil {
				return selectedScalar{}, fmt.Errorf("selected value at path %s must be a supported plist scalar: %w", quotedPath(selector.Path), err)
			}
			return selectedScalar{exists: true, canonical: canonical}, nil
		}
		current = value
	}
	return selectedScalar{exists: false}, nil
}

func setSelected(root any, selector Selector, value any) (any, error) {
	value, err := plistScalarValue(value)
	if err != nil {
		return nil, fmt.Errorf("desired selected value must be a supported plist scalar: %w", err)
	}
	if root == nil {
		if selectorCreatePolicy(selector) != CreatePolicyCreate {
			return nil, fmt.Errorf("missing plist document requires createMissing policy %q", CreatePolicyCreate)
		}
		root = map[string]any{}
	}
	current, ok := root.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("selector root requires dictionary document")
	}
	for idx, segment := range selector.Path {
		if idx == len(selector.Path)-1 {
			existing, exists := current[segment]
			if !exists && selectorCreatePolicy(selector) != CreatePolicyCreate {
				return nil, fmt.Errorf("missing selected path requires createMissing policy %q", CreatePolicyCreate)
			}
			if exists {
				if _, err := plistScalarValue(existing); err != nil {
					return nil, fmt.Errorf("existing selected value at path %s must be a supported plist scalar: %w", quotedPath(selector.Path), err)
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
			return nil, fmt.Errorf("selector path segment %q requires dictionary container", segment)
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
		return nil, fmt.Errorf("selector root requires dictionary document")
	}
	for idx, segment := range selector.Path {
		if idx == len(selector.Path)-1 {
			if existing, exists := current[segment]; exists {
				if _, err := plistScalarValue(existing); err != nil {
					return nil, fmt.Errorf("selected value at path %s must be a supported plist scalar: %w", quotedPath(selector.Path), err)
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
			return nil, fmt.Errorf("selector path segment %q requires dictionary container", segment)
		}
		current = child
	}
	return root, nil
}

func normalizeScalarPlist(raw []byte) ([]byte, error) {
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
	return plistScalarValue(value)
}

func canonicalScalar(value any) ([]byte, error) {
	canonicalValue, err := plistScalarValue(value)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(canonicalValue)
	if err != nil {
		return nil, err
	}
	return data, nil
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

type plistDocument struct {
	root       any
	format     int
	formatName string
}

func parsePlistDocument(data []byte) (plistDocument, error) {
	var root any
	format, err := plist.Unmarshal(data, &root)
	if err != nil {
		return plistDocument{}, err
	}
	switch format {
	case plist.XMLFormat:
		if err := validateXMLDuplicateKeys(data); err != nil {
			return plistDocument{}, err
		}
	case plist.BinaryFormat:
		if err := validateBinaryPlistSafety(data); err != nil {
			return plistDocument{}, err
		}
	default:
		return plistDocument{}, fmt.Errorf("unsupported plist format %s; only XML and Binary are supported", plistFormatDisplay(format))
	}
	root = normalizeContainers(root)
	if err := rejectUIDAnywhere(root); err != nil {
		return plistDocument{}, err
	}
	return plistDocument{root: root, format: format, formatName: plistFormatName(format)}, nil
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

func readDocumentFormat(path string) (string, error) {
	data, err := readRawFile(path)
	if err != nil || data == nil {
		return "", err
	}
	doc, err := parsePlistDocument(data)
	if err != nil {
		return "", driverError(filedriver.CodeInvalidSelector, "format", path, err)
	}
	return doc.formatName, nil
}

func plistFormatName(format int) string {
	switch format {
	case plist.XMLFormat:
		return FormatXML
	case plist.BinaryFormat:
		return FormatBinary
	default:
		return ""
	}
}

func marshalDocument(value any, format int) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	switch format {
	case plist.XMLFormat:
		return plist.MarshalIndent(value, plist.XMLFormat, "\t")
	case plist.BinaryFormat:
		return plist.Marshal(value, plist.BinaryFormat)
	default:
		return nil, fmt.Errorf("unsupported plist render format %s", plistFormatDisplay(format))
	}
}

func plistFormatDisplay(format int) string {
	if name := plistFormatName(format); name != "" {
		return name
	}
	if format >= 0 && format < len(plist.FormatNames) {
		return plist.FormatNames[format]
	}
	return fmt.Sprintf("unknown(%d)", format)
}

func plistScalarValue(value any) (any, error) {
	switch typed := value.(type) {
	case string:
		return typed, nil
	case bool:
		return typed, nil
	case json.Number:
		if _, err := json.Marshal(typed); err != nil {
			return nil, fmt.Errorf("plist number must be a valid JSON number")
		}
		if integer, err := typed.Int64(); err == nil {
			return integer, nil
		}
		floating, err := typed.Float64()
		if err != nil {
			return nil, fmt.Errorf("plist number must be a valid integer or finite float")
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
		return nil, fmt.Errorf("plist selected values do not support null")
	case time.Time:
		return nil, fmt.Errorf("plist date values are unsupported selected scalars")
	case plist.UID:
		return nil, fmt.Errorf("plist UID values are unsupported selected scalars")
	case []byte:
		return nil, fmt.Errorf("plist data values are unsupported selected scalars")
	case []any:
		return nil, fmt.Errorf("plist arrays are unsupported selected scalars")
	case map[string]any:
		return nil, fmt.Errorf("plist dictionaries are unsupported selected scalars")
	default:
		return nil, fmt.Errorf("unsupported plist selected scalar type %T", value)
	}
}

func unsignedToInt64(value uint64) (int64, error) {
	if value > math.MaxInt64 {
		return 0, fmt.Errorf("plist integer exceeds int64 range")
	}
	return int64(value), nil
}

func finiteFloat64(value float64) (float64, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("plist float must be finite")
	}
	return value, nil
}

func rejectUIDAnywhere(value any) error {
	switch typed := value.(type) {
	case plist.UID:
		return fmt.Errorf("plist UID values are unsupported in write-capable documents")
	case map[string]any:
		for key, child := range typed {
			if err := rejectUIDAnywhere(child); err != nil {
				return fmt.Errorf("dictionary key %q: %w", key, err)
			}
		}
	case []any:
		for idx, child := range typed {
			if err := rejectUIDAnywhere(child); err != nil {
				return fmt.Errorf("array index %d: %w", idx, err)
			}
		}
	}
	return nil
}

func validateXMLDuplicateKeys(data []byte) error {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		token, err := decoder.Token()
		if errorsIsEOF(err) {
			return nil
		}
		if err != nil {
			return err
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "dict" {
			continue
		}
		if err := validateXMLDict(decoder); err != nil {
			return err
		}
	}
}

func validateXMLDict(decoder *xml.Decoder) error {
	seen := map[string]struct{}{}
	for {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		switch typed := token.(type) {
		case xml.StartElement:
			switch typed.Name.Local {
			case "key":
				key, err := readXMLTextElement(decoder, typed.Name)
				if err != nil {
					return err
				}
				if _, exists := seen[key]; exists {
					return fmt.Errorf("duplicate plist dictionary key %q", key)
				}
				seen[key] = struct{}{}
			case "dict":
				if err := validateXMLDict(decoder); err != nil {
					return err
				}
			case "array":
				if err := validateXMLContainer(decoder, "array"); err != nil {
					return err
				}
			default:
				if err := skipXMLElement(decoder, typed.Name.Local); err != nil {
					return err
				}
			}
		case xml.EndElement:
			if typed.Name.Local == "dict" {
				return nil
			}
		}
	}
}

func validateXMLContainer(decoder *xml.Decoder, endName string) error {
	for {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		switch typed := token.(type) {
		case xml.StartElement:
			switch typed.Name.Local {
			case "dict":
				if err := validateXMLDict(decoder); err != nil {
					return err
				}
			case "array":
				if err := validateXMLContainer(decoder, "array"); err != nil {
					return err
				}
			default:
				if err := skipXMLElement(decoder, typed.Name.Local); err != nil {
					return err
				}
			}
		case xml.EndElement:
			if typed.Name.Local == endName {
				return nil
			}
		}
	}
}

func readXMLTextElement(decoder *xml.Decoder, name xml.Name) (string, error) {
	var builder strings.Builder
	for {
		token, err := decoder.Token()
		if err != nil {
			return "", err
		}
		switch typed := token.(type) {
		case xml.CharData:
			builder.Write([]byte(typed))
		case xml.StartElement:
			return "", fmt.Errorf("unexpected nested element <%s> inside <%s>", typed.Name.Local, name.Local)
		case xml.EndElement:
			if typed.Name.Local == name.Local {
				return builder.String(), nil
			}
		}
	}
}

func skipXMLElement(decoder *xml.Decoder, name string) error {
	depth := 1
	for depth > 0 {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		switch typed := token.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
			_ = typed
		}
	}
	return nil
}

type binaryPlistSafety struct {
	data       []byte
	offsetSize int
	refSize    int
	numObjects uint64
	offsets    []uint64
	visited    map[uint64]bool
}

func validateBinaryPlistSafety(data []byte) error {
	if len(data) < 40 || !bytes.HasPrefix(data, []byte("bplist")) {
		return fmt.Errorf("invalid binary plist header")
	}
	trailer := data[len(data)-32:]
	offsetSize := int(trailer[6])
	refSize := int(trailer[7])
	numObjects := binary.BigEndian.Uint64(trailer[8:16])
	offsetTableOffset := binary.BigEndian.Uint64(trailer[24:32])
	if offsetSize <= 0 || offsetSize > 8 || refSize <= 0 || refSize > 8 {
		return fmt.Errorf("invalid binary plist offset/ref size")
	}
	if numObjects > uint64(len(data)) {
		return fmt.Errorf("binary plist object count is unreasonable")
	}
	tableBytes := numObjects * uint64(offsetSize)
	if offsetTableOffset > uint64(len(data)) || tableBytes > uint64(len(data)) || offsetTableOffset+tableBytes > uint64(len(data)) {
		return fmt.Errorf("binary plist offset table escapes document")
	}
	validator := &binaryPlistSafety{data: data, offsetSize: offsetSize, refSize: refSize, numObjects: numObjects, offsets: make([]uint64, numObjects), visited: map[uint64]bool{}}
	for idx := uint64(0); idx < numObjects; idx++ {
		off, err := validator.readUint(offsetTableOffset+idx*uint64(offsetSize), offsetSize)
		if err != nil {
			return err
		}
		if off >= uint64(len(data)) {
			return fmt.Errorf("binary plist object %d offset escapes document", idx)
		}
		validator.offsets[idx] = off
	}
	for idx := uint64(0); idx < numObjects; idx++ {
		if err := validator.validateObject(idx); err != nil {
			return err
		}
	}
	return nil
}

func (p *binaryPlistSafety) validateObject(id uint64) error {
	if id >= p.numObjects {
		return fmt.Errorf("binary plist object reference %d is out of range", id)
	}
	if p.visited[id] {
		return nil
	}
	p.visited[id] = true
	off := p.offsets[id]
	tag, err := p.byteAt(off)
	if err != nil {
		return err
	}
	kind := tag >> 4
	info := tag & 0x0f
	switch kind {
	case 0x1:
		nbytes := int(1) << info
		if nbytes > 8 {
			return fmt.Errorf("binary plist integer object %d uses %d bytes; only up to 8-byte integers are supported", id, nbytes)
		}
	case 0x8:
		return fmt.Errorf("binary plist UID object %d is unsupported in write-capable documents", id)
	case 0xa:
		count, pos, err := p.objectCount(off+1, info)
		if err != nil {
			return err
		}
		for idx := uint64(0); idx < count; idx++ {
			ref, err := p.readRef(pos + idx*uint64(p.refSize))
			if err != nil {
				return err
			}
			if err := p.validateObject(ref); err != nil {
				return err
			}
		}
	case 0xd:
		count, pos, err := p.objectCount(off+1, info)
		if err != nil {
			return err
		}
		keysStart := pos
		valuesStart := pos + count*uint64(p.refSize)
		seen := map[string]struct{}{}
		for idx := uint64(0); idx < count; idx++ {
			keyRef, err := p.readRef(keysStart + idx*uint64(p.refSize))
			if err != nil {
				return err
			}
			key, err := p.stringObject(keyRef)
			if err != nil {
				return err
			}
			if _, exists := seen[key]; exists {
				return fmt.Errorf("duplicate plist dictionary key %q", key)
			}
			seen[key] = struct{}{}
			if err := p.validateObject(keyRef); err != nil {
				return err
			}
		}
		for idx := uint64(0); idx < count; idx++ {
			valueRef, err := p.readRef(valuesStart + idx*uint64(p.refSize))
			if err != nil {
				return err
			}
			if err := p.validateObject(valueRef); err != nil {
				return err
			}
		}
	case 0x4, 0x5, 0x6:
		if _, _, err := p.objectCount(off+1, info); err != nil {
			return err
		}
	}
	return nil
}

func (p *binaryPlistSafety) objectCount(pos uint64, info byte) (uint64, uint64, error) {
	if info < 0x0f {
		return uint64(info), pos, nil
	}
	tag, err := p.byteAt(pos)
	if err != nil {
		return 0, 0, err
	}
	if tag>>4 != 0x1 {
		return 0, 0, fmt.Errorf("binary plist extended count does not use an integer object")
	}
	nbytes := int(1) << (tag & 0x0f)
	if nbytes > 8 {
		return 0, 0, fmt.Errorf("binary plist extended count uses %d bytes; only up to 8-byte integers are supported", nbytes)
	}
	count, err := p.readUint(pos+1, nbytes)
	if err != nil {
		return 0, 0, err
	}
	return count, pos + 1 + uint64(nbytes), nil
}

func (p *binaryPlistSafety) stringObject(id uint64) (string, error) {
	if id >= p.numObjects {
		return "", fmt.Errorf("binary plist string reference %d is out of range", id)
	}
	off := p.offsets[id]
	tag, err := p.byteAt(off)
	if err != nil {
		return "", err
	}
	kind := tag >> 4
	info := tag & 0x0f
	count, pos, err := p.objectCount(off+1, info)
	if err != nil {
		return "", err
	}
	switch kind {
	case 0x5:
		if count > uint64(len(p.data)) || pos+count > uint64(len(p.data)) {
			return "", fmt.Errorf("binary plist ASCII string escapes document")
		}
		return string(p.data[pos : pos+count]), nil
	case 0x6:
		bytesLen := count * 2
		if bytesLen > uint64(len(p.data)) || pos+bytesLen > uint64(len(p.data)) {
			return "", fmt.Errorf("binary plist UTF-16 string escapes document")
		}
		units := make([]uint16, count)
		for idx := uint64(0); idx < count; idx++ {
			units[idx] = binary.BigEndian.Uint16(p.data[pos+idx*2 : pos+idx*2+2])
		}
		return string(utf16.Decode(units)), nil
	default:
		return "", fmt.Errorf("binary plist dictionary key object %d is not a string", id)
	}
}

func (p *binaryPlistSafety) byteAt(pos uint64) (byte, error) {
	if pos >= uint64(len(p.data)) {
		return 0, fmt.Errorf("binary plist offset escapes document")
	}
	return p.data[pos], nil
}

func (p *binaryPlistSafety) readRef(pos uint64) (uint64, error) {
	return p.readUint(pos, p.refSize)
}

func (p *binaryPlistSafety) readUint(pos uint64, nbytes int) (uint64, error) {
	if nbytes <= 0 || nbytes > 8 {
		return 0, fmt.Errorf("invalid binary plist integer byte width %d", nbytes)
	}
	if pos > uint64(len(p.data)) || pos+uint64(nbytes) > uint64(len(p.data)) {
		return 0, fmt.Errorf("binary plist integer read escapes document")
	}
	var value uint64
	for idx := 0; idx < nbytes; idx++ {
		value = (value << 8) | uint64(p.data[pos+uint64(idx)])
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
	tmp, err := os.CreateTemp(parent, ".dfm-plist-*")
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

func quotedPath(path []string) string {
	data, err := json.Marshal(path)
	if err != nil {
		return fmt.Sprintf("%q", path)
	}
	return string(data)
}
