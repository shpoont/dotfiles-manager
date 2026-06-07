package yamldriver

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/shpoont/dotfiles-manager/internal/v2/filedriver"
	"gopkg.in/yaml.v3"
)

const NormalizerID = "yaml-file.selected-scalar.v1"

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
	root, err := parseYAMLDocument(data)
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
	canonical, err := normalizeScalarYAML(raw)
	if err != nil {
		return State{}, err
	}
	return stateFromCanonical(canonical), nil
}

func (d Driver) NormalizeValue(value any) (State, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return State{}, err
	}
	return d.Normalize(raw)
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
	var root *yaml.Node
	if data != nil {
		root, err = parseYAMLDocument(data)
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
			return fmt.Errorf("selector path segment %d looks like an expression, not a mapping key", idx)
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

func selectScalar(root *yaml.Node, selector Selector) (selectedScalar, error) {
	current := root
	for idx, segment := range selector.Path {
		if current == nil || current.Kind != yaml.MappingNode {
			return selectedScalar{}, fmt.Errorf("selector path segment %q requires mapping container", selector.Path[idx])
		}
		value, ok := mappingValue(current, segment)
		if !ok {
			return selectedScalar{exists: false}, nil
		}
		if idx == len(selector.Path)-1 {
			if value.Kind != yaml.ScalarNode {
				return selectedScalar{}, fmt.Errorf("selected value at path %s must be a supported YAML scalar", strings.Join(selector.Path, "."))
			}
			canonical, err := canonicalScalar(value)
			if err != nil {
				return selectedScalar{}, err
			}
			return selectedScalar{exists: true, canonical: canonical}, nil
		}
		current = value
	}
	return selectedScalar{exists: false}, nil
}

func setSelected(root *yaml.Node, selector Selector, value *yaml.Node) (*yaml.Node, error) {
	if value == nil || value.Kind != yaml.ScalarNode {
		return nil, fmt.Errorf("desired selected value must be a supported YAML scalar")
	}
	if _, err := canonicalScalar(value); err != nil {
		return nil, fmt.Errorf("desired selected value must be a supported YAML scalar: %w", err)
	}
	if root == nil {
		if selectorCreatePolicy(selector) != CreatePolicyCreate {
			return nil, fmt.Errorf("missing YAML document requires createMissing policy %q", CreatePolicyCreate)
		}
		root = newMappingNode()
	}
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("selector root requires mapping document")
	}
	current := root
	for idx, segment := range selector.Path {
		if idx == len(selector.Path)-1 {
			valueIdx, exists := mappingValueIndex(current, segment)
			if !exists && selectorCreatePolicy(selector) != CreatePolicyCreate {
				return nil, fmt.Errorf("missing selected path requires createMissing policy %q", CreatePolicyCreate)
			}
			if exists && current.Content[valueIdx].Kind != yaml.ScalarNode {
				return nil, fmt.Errorf("existing selected value at path %s must be a supported YAML scalar", strings.Join(selector.Path, "."))
			}
			if exists {
				current.Content[valueIdx] = cloneScalarNode(value)
			} else {
				current.Content = append(current.Content, newKeyNode(segment), cloneScalarNode(value))
			}
			return root, nil
		}
		valueIdx, exists := mappingValueIndex(current, segment)
		if !exists {
			if selectorCreatePolicy(selector) != CreatePolicyCreate {
				return nil, fmt.Errorf("missing selector container requires createMissing policy %q", CreatePolicyCreate)
			}
			child := newMappingNode()
			current.Content = append(current.Content, newKeyNode(segment), child)
			current = child
			continue
		}
		child := current.Content[valueIdx]
		if child.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("selector path segment %q requires mapping container", segment)
		}
		current = child
	}
	return root, nil
}

func deleteSelected(root *yaml.Node, selector Selector) (*yaml.Node, error) {
	if selectorDeletePolicy(selector) != DeletePolicyAllow {
		return nil, fmt.Errorf("delete intent requires deleteKey policy %q", DeletePolicyAllow)
	}
	if root == nil {
		return nil, nil
	}
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("selector root requires mapping document")
	}
	current := root
	for idx, segment := range selector.Path {
		if idx == len(selector.Path)-1 {
			valueIdx, exists := mappingValueIndex(current, segment)
			if exists && current.Content[valueIdx].Kind != yaml.ScalarNode {
				return nil, fmt.Errorf("selected value at path %s must be a supported YAML scalar", strings.Join(selector.Path, "."))
			}
			if exists {
				keyIdx := valueIdx - 1
				current.Content = append(current.Content[:keyIdx], current.Content[valueIdx+1:]...)
			}
			return root, nil
		}
		valueIdx, exists := mappingValueIndex(current, segment)
		if !exists {
			return root, nil
		}
		child := current.Content[valueIdx]
		if child.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("selector path segment %q requires mapping container", segment)
		}
		current = child
	}
	return root, nil
}

func mappingValue(mapping *yaml.Node, key string) (*yaml.Node, bool) {
	idx, ok := mappingValueIndex(mapping, key)
	if !ok {
		return nil, false
	}
	return mapping.Content[idx], true
}

func mappingValueIndex(mapping *yaml.Node, key string) (int, bool) {
	for idx := 0; idx < len(mapping.Content); idx += 2 {
		if mapping.Content[idx].Value == key {
			return idx + 1, true
		}
	}
	return 0, false
}

func newMappingNode() *yaml.Node {
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
}

func newKeyNode(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

func cloneScalarNode(value *yaml.Node) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: value.Tag, Value: value.Value}
}

func normalizeScalarYAML(raw []byte) ([]byte, error) {
	value, err := parseYAMLDocument(raw)
	if err != nil {
		return nil, err
	}
	if value.Kind != yaml.ScalarNode {
		return nil, fmt.Errorf("selected value must be a YAML scalar")
	}
	return canonicalScalar(value)
}

func parseDesiredScalar(raw []byte) (*yaml.Node, error) {
	value, err := parseYAMLDocument(raw)
	if err != nil {
		return nil, err
	}
	if value.Kind != yaml.ScalarNode {
		return nil, fmt.Errorf("desired selected value must be a YAML scalar")
	}
	if _, err := canonicalScalar(value); err != nil {
		return nil, err
	}
	return cloneScalarNode(value), nil
}

func canonicalScalar(value *yaml.Node) ([]byte, error) {
	if value == nil || value.Kind != yaml.ScalarNode {
		return nil, fmt.Errorf("value must be a YAML scalar")
	}
	switch value.Tag {
	case "!!str":
		return json.Marshal(value.Value)
	case "!!bool":
		var decoded bool
		if err := value.Decode(&decoded); err != nil {
			return nil, err
		}
		return json.Marshal(decoded)
	case "!!null":
		return []byte("null"), nil
	case "!!int":
		var decoded int64
		if err := value.Decode(&decoded); err != nil {
			return nil, err
		}
		return json.Marshal(decoded)
	case "!!float":
		var decoded float64
		if err := value.Decode(&decoded); err != nil {
			return nil, err
		}
		if math.IsNaN(decoded) || math.IsInf(decoded, 0) {
			return nil, fmt.Errorf("YAML float must be finite")
		}
		return json.Marshal(decoded)
	default:
		return nil, fmt.Errorf("unsupported YAML scalar tag %q", value.Tag)
	}
}

func marshalDocument(value *yaml.Node) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(value); err != nil {
		_ = encoder.Close()
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func parseYAMLDocument(data []byte) (*yaml.Node, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	if err := decoder.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("empty YAML document")
		}
		return nil, err
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err == nil {
		return nil, fmt.Errorf("multiple YAML documents are unsupported")
	} else if !errors.Is(err, io.EOF) {
		return nil, err
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0] == nil {
		return nil, fmt.Errorf("YAML document must contain exactly one root node")
	}
	root := doc.Content[0]
	if err := validateYAMLNode(root, false); err != nil {
		return nil, err
	}
	return root, nil
}

func validateYAMLNode(node *yaml.Node, key bool) error {
	if node == nil {
		return fmt.Errorf("YAML node is nil")
	}
	if node.Anchor != "" {
		return fmt.Errorf("YAML anchors are unsupported")
	}
	if node.Kind == yaml.AliasNode {
		return fmt.Errorf("YAML aliases are unsupported")
	}
	if node.Tag == "!!merge" {
		return fmt.Errorf("YAML merge keys are unsupported")
	}
	if key {
		if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
			return fmt.Errorf("YAML mapping keys must be string scalars")
		}
		return nil
	}
	switch node.Kind {
	case yaml.ScalarNode:
		_, err := canonicalScalar(node)
		return err
	case yaml.MappingNode:
		if node.Tag != "!!map" {
			return fmt.Errorf("unsupported YAML mapping tag %q", node.Tag)
		}
		if len(node.Content)%2 != 0 {
			return fmt.Errorf("YAML mapping has an uneven key/value node count")
		}
		seen := map[string]struct{}{}
		for idx := 0; idx < len(node.Content); idx += 2 {
			keyNode := node.Content[idx]
			if err := validateYAMLNode(keyNode, true); err != nil {
				return err
			}
			keyValue := keyNode.Value
			if _, exists := seen[keyValue]; exists {
				return fmt.Errorf("duplicate mapping key %q", keyValue)
			}
			seen[keyValue] = struct{}{}
			if err := validateYAMLNode(node.Content[idx+1], false); err != nil {
				return err
			}
		}
		return nil
	case yaml.SequenceNode:
		if node.Tag != "!!seq" {
			return fmt.Errorf("unsupported YAML sequence tag %q", node.Tag)
		}
		for _, child := range node.Content {
			if err := validateYAMLNode(child, false); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported YAML node kind %d", node.Kind)
	}
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
	tmp, err := os.CreateTemp(parent, ".dfm-yaml-*")
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
