package inidriver

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shpoont/dotfiles-manager/internal/v2/filedriver"
)

const NormalizerID = "ini-file.selected-key.v1"

type MissingPolicy string

const (
	MissingPolicyError  MissingPolicy = "error"
	MissingPolicyCreate MissingPolicy = "create"
)

type DuplicatePolicy string

const (
	DuplicatePolicyReject DuplicatePolicy = "reject"
)

type DeletePolicy string

const (
	DeletePolicyReject DeletePolicy = "reject"
	DeletePolicyAllow  DeletePolicy = "allow"
)

type DesiredIntent string

const (
	IntentSet    DesiredIntent = "set"
	IntentDelete DesiredIntent = "delete"
)

type Driver struct{}

type Selector struct {
	Section         string          `json:"section"`
	Key             string          `json:"key"`
	MissingSection  MissingPolicy   `json:"missingSection,omitempty"`
	MissingKey      MissingPolicy   `json:"missingKey,omitempty"`
	DuplicatePolicy DuplicatePolicy `json:"duplicatePolicy,omitempty"`
	DeleteKey       DeletePolicy    `json:"deleteKey,omitempty"`
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
	Value      string        `json:"-"`
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
	info, err := os.Stat(resolved.AbsPath)
	if os.IsNotExist(err) {
		return AbsentState(), nil
	}
	if err != nil {
		return State{}, classifyOSError("readCurrent", resolved.AbsPath, err)
	}
	if !info.Mode().IsRegular() {
		return State{}, driverError(filedriver.CodeInvalidSelector, "readCurrent", resolved.AbsPath, fmt.Errorf("path is not a regular file"))
	}
	data, err := os.ReadFile(resolved.AbsPath)
	if err != nil {
		return State{}, classifyOSError("readCurrent", resolved.AbsPath, err)
	}
	parsed, err := parseSelected(data, req.Selector)
	if err != nil {
		return State{}, driverError(filedriver.CodeInvalidSelector, "readCurrent", resolved.AbsPath, err)
	}
	if parsed.section == nil || parsed.key == nil {
		return AbsentState(), nil
	}
	return d.Normalize(parsed.key.value), nil
}

func (d Driver) Normalize(raw string) State {
	value := normalizeScalar(raw)
	sum := sha256.Sum256([]byte(value))
	return State{
		Exists:     true,
		Value:      value,
		SHA256:     hex.EncodeToString(sum[:]),
		Normalizer: NormalizerID,
		Intent:     IntentSet,
	}
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
	case current.SHA256 == desired.SHA256 && current.Value == desired.Value:
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
	if change.Kind != filedriver.ChangeUnchanged {
		if _, err := d.renderDesired(req, desired); err != nil {
			return Preview{}, err
		}
	} else if desired.Intent == IntentDelete && selectorDeletePolicy(req.Selector) != DeletePolicyAllow {
		return Preview{}, driverError(filedriver.CodeInvalidSelector, "previewApply", resolved.AbsPath, fmt.Errorf("delete intent requires deleteKey policy %q", DeletePolicyAllow))
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
	parsed, err := parseSelected(data, req.Selector)
	if err != nil {
		return nil, driverError(filedriver.CodeInvalidSelector, "render", resolved.AbsPath, err)
	}
	if desired.Exists {
		return renderSet(data, parsed, req.Selector, desired.Value, resolved.AbsPath)
	}
	return renderDelete(data, parsed, req.Selector, resolved.AbsPath)
}

func renderSet(data []byte, parsed *parsedINI, selector Selector, value string, path string) ([]byte, error) {
	if parsed.section == nil {
		if selectorMissingSection(selector) != MissingPolicyCreate || selectorMissingKey(selector) != MissingPolicyCreate {
			return nil, driverError(filedriver.CodeInvalidSelector, "render", path, fmt.Errorf("missing section/key requires missingSection and missingKey policy %q", MissingPolicyCreate))
		}
		return appendSection(data, parsed.newline, selector.Section, selector.Key, value), nil
	}
	if parsed.key == nil {
		if selectorMissingKey(selector) != MissingPolicyCreate {
			return nil, driverError(filedriver.CodeInvalidSelector, "render", path, fmt.Errorf("missing key requires missingKey policy %q", MissingPolicyCreate))
		}
		return insertLine(parsed.lines, parsed.section.end, canonicalAssignment(parsed.newline, selector.Key, value)), nil
	}
	return replaceLine(parsed.lines, parsed.key.index, canonicalAssignment(parsed.key.newlineOr(parsed.newline), selector.Key, value)), nil
}

func renderDelete(data []byte, parsed *parsedINI, selector Selector, path string) ([]byte, error) {
	if selectorDeletePolicy(selector) != DeletePolicyAllow {
		return nil, driverError(filedriver.CodeInvalidSelector, "render", path, fmt.Errorf("delete intent requires deleteKey policy %q", DeletePolicyAllow))
	}
	if parsed.section == nil || parsed.key == nil {
		return append([]byte(nil), data...), nil
	}
	return removeLine(parsed.lines, parsed.key.index), nil
}

func resolveRequest(req Request) (filedriver.ResolvedPath, error) {
	if err := validateSelector(req.Selector); err != nil {
		return filedriver.ResolvedPath{}, driverError(filedriver.CodeInvalidSelector, "selector", "", err)
	}
	return filedriver.ResolveTarget(req.Target)
}

func validateSelector(selector Selector) error {
	if selector.Section == "" || strings.TrimSpace(selector.Section) != selector.Section {
		return fmt.Errorf("selector section is required and must not have surrounding whitespace")
	}
	if strings.ContainsAny(selector.Section, "\r\n[]") {
		return fmt.Errorf("selector section must be an unbracketed single-line section name")
	}
	if selector.Key == "" || strings.TrimSpace(selector.Key) != selector.Key {
		return fmt.Errorf("selector key is required and must not have surrounding whitespace")
	}
	if strings.ContainsAny(selector.Key, "\r\n=") {
		return fmt.Errorf("selector key must be a single-line key name without equals")
	}
	switch selectorMissingSection(selector) {
	case MissingPolicyError, MissingPolicyCreate:
	default:
		return fmt.Errorf("unsupported missingSection policy %q", selector.MissingSection)
	}
	switch selectorMissingKey(selector) {
	case MissingPolicyError, MissingPolicyCreate:
	default:
		return fmt.Errorf("unsupported missingKey policy %q", selector.MissingKey)
	}
	switch selectorDuplicatePolicy(selector) {
	case DuplicatePolicyReject:
	default:
		return fmt.Errorf("unsupported duplicatePolicy %q", selector.DuplicatePolicy)
	}
	switch selectorDeletePolicy(selector) {
	case DeletePolicyReject, DeletePolicyAllow:
	default:
		return fmt.Errorf("unsupported deleteKey policy %q", selector.DeleteKey)
	}
	return nil
}

func selectorMissingSection(selector Selector) MissingPolicy {
	if selector.MissingSection == "" {
		return MissingPolicyError
	}
	return selector.MissingSection
}

func selectorMissingKey(selector Selector) MissingPolicy {
	if selector.MissingKey == "" {
		return MissingPolicyError
	}
	return selector.MissingKey
}

func selectorDuplicatePolicy(selector Selector) DuplicatePolicy {
	if selector.DuplicatePolicy == "" {
		return DuplicatePolicyReject
	}
	return selector.DuplicatePolicy
}

func selectorDeletePolicy(selector Selector) DeletePolicy {
	if selector.DeleteKey == "" {
		return DeletePolicyReject
	}
	return selector.DeleteKey
}

func normalizeDesired(desired State) (State, error) {
	if desired.Exists {
		if desired.Intent != "" && desired.Intent != IntentSet {
			return State{}, fmt.Errorf("desired set state must use intent %q", IntentSet)
		}
		value := normalizeScalar(desired.Value)
		if containsInvalidScalarByte(value) {
			return State{}, fmt.Errorf("selected value must be a single-line scalar without CR, LF, or NUL")
		}
		sum := sha256.Sum256([]byte(value))
		desired.Value = value
		desired.SHA256 = hex.EncodeToString(sum[:])
		desired.Normalizer = NormalizerID
		desired.Intent = IntentSet
		return desired, nil
	}
	if desired.Intent != IntentDelete {
		return State{}, fmt.Errorf("delete desired state must use explicit intent %q", IntentDelete)
	}
	desired.Value = ""
	desired.SHA256 = ""
	desired.Normalizer = NormalizerID
	return desired, nil
}

func normalizeScalar(raw string) string {
	return strings.Trim(raw, " \t")
}

func containsInvalidScalarByte(value string) bool {
	return strings.ContainsAny(value, "\r\n\x00")
}

type iniLine struct {
	raw     string
	content string
	newline string
}

type selectedKey struct {
	index   int
	value   string
	newline string
}

func (k selectedKey) newlineOr(fallback string) string {
	if k.newline != "" {
		return k.newline
	}
	return fallback
}

type selectedSection struct {
	index int
	end   int
}

type parsedINI struct {
	lines   []iniLine
	newline string
	section *selectedSection
	key     *selectedKey
}

func parseSelected(data []byte, selector Selector) (*parsedINI, error) {
	if err := validateSelector(selector); err != nil {
		return nil, err
	}
	lines := splitLines(data)
	parsed := &parsedINI{lines: lines, newline: detectNewline(lines)}
	sections := make([]selectedSection, 0, 1)
	keys := make([]selectedKey, 0, 1)
	currentSelectedSection := -1
	for idx, line := range lines {
		sectionName, boundary, malformedSelected := parseSectionBoundary(line.content, selector.Section)
		if malformedSelected {
			return nil, fmt.Errorf("selected section line must be a bracket-only section header")
		}
		if boundary {
			if currentSelectedSection >= 0 {
				sections[currentSelectedSection].end = idx
			}
			currentSelectedSection = -1
			if sectionName == selector.Section {
				sections = append(sections, selectedSection{index: idx, end: len(lines)})
				currentSelectedSection = len(sections) - 1
			}
			continue
		}
		if currentSelectedSection < 0 {
			continue
		}
		keyName, value, ok := parseAssignment(line.content)
		if !ok || keyName != selector.Key {
			continue
		}
		value = normalizeScalar(value)
		if containsInvalidScalarByte(value) {
			return nil, fmt.Errorf("selected value must be a single-line scalar without CR, LF, or NUL")
		}
		keys = append(keys, selectedKey{index: idx, value: value, newline: line.newline})
	}
	for sectionIdx := range sections {
		if sections[sectionIdx].end == 0 {
			sections[sectionIdx].end = len(lines)
		}
	}
	if len(sections) > 1 {
		return nil, fmt.Errorf("duplicate selected section %q is ambiguous", selector.Section)
	}
	if len(keys) > 1 {
		return nil, fmt.Errorf("duplicate selected key %q in section %q is ambiguous", selector.Key, selector.Section)
	}
	if len(sections) == 1 {
		section := sections[0]
		parsed.section = &section
	}
	if len(keys) == 1 {
		key := keys[0]
		parsed.key = &key
	}
	return parsed, nil
}

func splitLines(data []byte) []iniLine {
	if len(data) == 0 {
		return nil
	}
	lines := make([]iniLine, 0)
	start := 0
	for idx, b := range data {
		if b != '\n' {
			continue
		}
		contentEnd := idx
		newline := "\n"
		if idx > start && data[idx-1] == '\r' {
			contentEnd = idx - 1
			newline = "\r\n"
		}
		raw := string(data[start : idx+1])
		content := string(data[start:contentEnd])
		lines = append(lines, iniLine{raw: raw, content: content, newline: newline})
		start = idx + 1
	}
	if start < len(data) {
		raw := string(data[start:])
		lines = append(lines, iniLine{raw: raw, content: raw})
	}
	return lines
}

func detectNewline(lines []iniLine) string {
	for _, line := range lines {
		if line.newline != "" {
			return line.newline
		}
	}
	return "\n"
}

func parseSectionBoundary(content string, selectedSection string) (name string, boundary bool, malformedSelected bool) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" || isFullLineComment(trimmed) || !strings.HasPrefix(trimmed, "[") {
		return "", false, false
	}
	closeIdx := strings.Index(trimmed, "]")
	if closeIdx < 0 {
		return "", false, false
	}
	name = strings.TrimSpace(trimmed[1:closeIdx])
	if name == "" {
		return "", false, false
	}
	if closeIdx != len(trimmed)-1 && name == selectedSection {
		return name, true, true
	}
	return name, true, false
}

func parseAssignment(content string) (key string, value string, ok bool) {
	trimmedLeft := strings.TrimLeft(content, " \t")
	if trimmedLeft == "" || isFullLineComment(trimmedLeft) || strings.HasPrefix(trimmedLeft, "[") {
		return "", "", false
	}
	eq := strings.Index(content, "=")
	if eq < 0 {
		return "", "", false
	}
	key = strings.TrimSpace(content[:eq])
	if key == "" {
		return "", "", false
	}
	return key, content[eq+1:], true
}

func isFullLineComment(trimmedLeft string) bool {
	return strings.HasPrefix(trimmedLeft, "#") || strings.HasPrefix(trimmedLeft, ";")
}

func canonicalAssignment(newline string, key string, value string) string {
	if newline == "" {
		newline = "\n"
	}
	return "\t" + key + " = " + value + newline
}

func replaceLine(lines []iniLine, index int, replacement string) []byte {
	var out bytes.Buffer
	for idx, line := range lines {
		if idx == index {
			out.WriteString(replacement)
			continue
		}
		out.WriteString(line.raw)
	}
	return out.Bytes()
}

func insertLine(lines []iniLine, index int, inserted string) []byte {
	var out bytes.Buffer
	for idx, line := range lines {
		if idx == index {
			if idx > 0 && lines[idx-1].newline == "" {
				out.WriteString(detectNewline(lines))
			}
			out.WriteString(inserted)
		}
		out.WriteString(line.raw)
	}
	if index >= len(lines) {
		if len(lines) > 0 && lines[len(lines)-1].newline == "" {
			out.WriteString(detectNewline(lines))
		}
		out.WriteString(inserted)
	}
	return out.Bytes()
}

func removeLine(lines []iniLine, index int) []byte {
	var out bytes.Buffer
	for idx, line := range lines {
		if idx == index {
			continue
		}
		out.WriteString(line.raw)
	}
	return out.Bytes()
}

func appendSection(data []byte, newline string, section string, key string, value string) []byte {
	if newline == "" {
		newline = "\n"
	}
	var out bytes.Buffer
	out.Write(data)
	if len(data) > 0 && !bytes.HasSuffix(data, []byte("\n")) {
		out.WriteString(newline)
	}
	if len(data) > 0 && !bytes.HasSuffix(out.Bytes(), []byte(newline+newline)) {
		out.WriteString(newline)
	}
	out.WriteString("[")
	out.WriteString(section)
	out.WriteString("]")
	out.WriteString(newline)
	out.WriteString(canonicalAssignment(newline, key, value))
	return out.Bytes()
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
	tmp, err := os.CreateTemp(parent, ".dfm-ini-*")
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
