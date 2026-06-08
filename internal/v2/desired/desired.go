package desired

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/shpoont/dotfiles-manager/internal/v2/resolution"
	"github.com/shpoont/dotfiles-manager/internal/v2/selectedvalue"
	"gopkg.in/yaml.v3"
)

const (
	SettingsSchema  = "dotfiles-manager.v2.desired-settings"
	SchemaVersion   = 1
	Scheme          = "desired"
	SeverityError   = "error"
	IntentSet       = "set"
	IntentDelete    = "delete"
	IntentUnmanaged = "unmanaged"
	KindString      = "string"
	KindBool        = "bool"
	KindNumber      = "number"
	KindNull        = "null"
	ObjectSettings  = "settings"
	ObjectManifest  = "manifest"
	ObjectArtifact  = "artifact"
	StatusMissing   = "missing"
	StatusPresent   = "present"
	StatusUnmanaged = "unmanaged"
)

var (
	publicIDPattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*(?:[.-][a-z0-9][a-z0-9_-]*)*$`)
	identityIDRegexp = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
)

type ResolvedURI struct {
	URI          string `json:"uri"`
	Scope        string `json:"scope"`
	Subject      string `json:"subject"`
	TargetID     string `json:"targetId"`
	Object       string `json:"object"`
	SettingID    string `json:"settingId,omitempty"`
	ArtifactPath string `json:"artifactPath,omitempty"`
	RelPath      string `json:"relPath"`
	Path         string `json:"path"`
	TargetRelDir string `json:"targetRelDir"`
	Root         string `json:"-"`
}

type SelectedValue struct {
	intent string
	kind   string
	value  any
}

func SetString(value string) SelectedValue {
	return SelectedValue{intent: IntentSet, kind: KindString, value: value}
}

func SetBool(value bool) SelectedValue {
	return SelectedValue{intent: IntentSet, kind: KindBool, value: value}
}

func SetNumber(value json.Number) SelectedValue {
	return SelectedValue{intent: IntentSet, kind: KindNumber, value: value}
}

func SetNull() SelectedValue {
	return SelectedValue{intent: IntentSet, kind: KindNull, value: nil}
}

func Delete() SelectedValue {
	return SelectedValue{intent: IntentDelete}
}

func Unmanaged() SelectedValue {
	return SelectedValue{intent: IntentUnmanaged}
}

func (v SelectedValue) Intent() string { return v.intent }
func (v SelectedValue) Kind() string   { return v.kind }

func (v SelectedValue) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]string{"intent": v.intent, "kind": v.kind})
}

func (v SelectedValue) ToSelectedValueDesired() (selectedvalue.Desired, bool, error) {
	switch v.intent {
	case IntentSet:
		switch v.kind {
		case KindString:
			value, ok := v.value.(string)
			if !ok {
				return selectedvalue.Desired{}, false, valueError("desired string value has invalid internal representation")
			}
			return selectedvalue.SetString(value), true, nil
		case KindBool:
			value, ok := v.value.(bool)
			if !ok {
				return selectedvalue.Desired{}, false, valueError("desired bool value has invalid internal representation")
			}
			return selectedvalue.SetBool(value), true, nil
		case KindNumber:
			value, ok := v.value.(json.Number)
			if !ok {
				return selectedvalue.Desired{}, false, valueError("desired number value has invalid internal representation")
			}
			if _, err := json.Marshal(value); err != nil {
				return selectedvalue.Desired{}, false, valueError("desired number value must be a valid JSON number")
			}
			return selectedvalue.SetNumber(value), true, nil
		case KindNull:
			return selectedvalue.SetNull(), true, nil
		default:
			return selectedvalue.Desired{}, false, valueError("desired set value kind is unsupported")
		}
	case IntentDelete:
		return selectedvalue.Delete(), true, nil
	case IntentUnmanaged:
		return selectedvalue.Desired{}, false, nil
	default:
		return selectedvalue.Desired{}, false, valueError("desired intent is required")
	}
}

type ReadResult struct {
	Status  string                 `json:"status"`
	URI     ResolvedURI            `json:"uri"`
	Intent  string                 `json:"intent,omitempty"`
	Kind    string                 `json:"kind,omitempty"`
	Desired *selectedvalue.Desired `json:"-"`
}

type WriteSafetyDecision struct {
	Recipe     *recipe.Recipe
	SettingRef string
	Context    recipe.WriteSafetyContext
}

type WriteRequest struct {
	RepoRoot string
	URI      string
	Value    SelectedValue
	Safety   *WriteSafetyDecision
}

type Diagnostic struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Path     string `json:"path,omitempty"`
}

type SafetyError struct {
	Diagnostics []Diagnostic `json:"diagnostics"`
}

func (e *SafetyError) Error() string {
	if e == nil || len(e.Diagnostics) == 0 {
		return "desired write blocked"
	}
	parts := make([]string, 0, len(e.Diagnostics))
	for _, diagnostic := range e.Diagnostics {
		parts = append(parts, fmt.Sprintf("%s[%s]: %s", defaultString(diagnostic.Path, "$"), diagnostic.Code, diagnostic.Message))
	}
	return strings.Join(parts, "; ")
}

func ResolveURI(repoRoot string, rawURI string) (ResolvedURI, error) {
	root, err := normalizeRepoRoot(repoRoot)
	if err != nil {
		return ResolvedURI{}, err
	}
	parsed, err := parseDesiredURI(rawURI)
	if err != nil {
		return ResolvedURI{}, err
	}
	parsed.Root = root
	parsed.Path = filepath.Join(root, parsed.RelPath)
	if err := ensurePathInside(filepath.Join(root, parsed.TargetRelDir), parsed.Path); err != nil {
		return ResolvedURI{}, err
	}
	return parsed, nil
}

func ResolveForSetting(repoRoot string, setting resolution.ResolvedSetting) (ResolvedURI, error) {
	resolved, err := ResolveURI(repoRoot, setting.DesiredURI)
	if err != nil {
		return ResolvedURI{}, err
	}
	if resolved.Object != ObjectSettings {
		return ResolvedURI{}, fmt.Errorf("resolved setting %s desired URI must point to settings", setting.Ref())
	}
	if resolved.TargetID != setting.TargetID || resolved.SettingID != setting.SettingID {
		return ResolvedURI{}, fmt.Errorf("resolved setting %s desired URI target/setting mismatch", setting.Ref())
	}
	if filepath.Clean(resolved.RelPath) != filepath.Clean(setting.DesiredRelPath) {
		return ResolvedURI{}, fmt.Errorf("resolved setting %s desired path mismatch", setting.Ref())
	}
	return resolved, nil
}

func ReadSelectedValue(repoRoot string, desiredURI string) (ReadResult, error) {
	resolved, err := ResolveURI(repoRoot, desiredURI)
	if err != nil {
		return ReadResult{}, err
	}
	return readSelectedValue(resolved)
}

func ReadSelectedValueForSetting(repoRoot string, setting resolution.ResolvedSetting) (ReadResult, error) {
	resolved, err := ResolveForSetting(repoRoot, setting)
	if err != nil {
		return ReadResult{}, err
	}
	return readSelectedValue(resolved)
}

func WriteSelectedValue(req WriteRequest) error {
	resolved, err := ResolveURI(req.RepoRoot, req.URI)
	if err != nil {
		return err
	}
	if resolved.Object != ObjectSettings || resolved.SettingID == "" {
		return fmt.Errorf("selected-value desired writes require a settings URI with a setting fragment")
	}
	if err := validateWriteSafety(resolved, req.Value, req.Safety); err != nil {
		return err
	}
	return writeSelectedValue(resolved, req.Value)
}

func MarkSelectedValueUnmanaged(req WriteRequest) error {
	req.Value = Unmanaged()
	return WriteSelectedValue(req)
}

func readSelectedValue(resolved ResolvedURI) (ReadResult, error) {
	if resolved.Object != ObjectSettings || resolved.SettingID == "" {
		return ReadResult{}, fmt.Errorf("selected-value desired reads require a settings URI with a setting fragment")
	}
	if exists, err := ensureSafeReadPath(resolved.Root, resolved.Path); err != nil {
		return ReadResult{}, err
	} else if !exists {
		return ReadResult{Status: StatusMissing, URI: resolved}, nil
	}

	file, err := loadSettingsFile(resolved.Path)
	if err != nil {
		return ReadResult{}, err
	}
	entry, ok := file.Values[resolved.SettingID]
	if !ok {
		return ReadResult{Status: StatusMissing, URI: resolved}, nil
	}
	value, err := selectedValueFromEntry(entry)
	if err != nil {
		return ReadResult{}, err
	}
	desired, hasDesired, err := value.ToSelectedValueDesired()
	if err != nil {
		return ReadResult{}, err
	}
	result := ReadResult{URI: resolved, Intent: value.intent, Kind: value.kind}
	if !hasDesired {
		result.Status = StatusUnmanaged
		return result, nil
	}
	result.Status = StatusPresent
	result.Desired = &desired
	return result, nil
}

func writeSelectedValue(resolved ResolvedURI, value SelectedValue) error {
	if _, _, err := value.ToSelectedValueDesired(); err != nil {
		return err
	}
	if err := ensureSafeWritePath(resolved.Root, resolved.Path); err != nil {
		return err
	}
	file, err := loadOrCreateSettingsFile(resolved.Path)
	if err != nil {
		return err
	}
	if file.Values == nil {
		file.Values = map[string]selectedValueEntry{}
	}
	entry, err := entryFromSelectedValue(value)
	if err != nil {
		return err
	}
	file.Values[resolved.SettingID] = entry
	return writeSettingsFileAtomic(resolved.Root, resolved.Path, file)
}

type desiredURIParts struct {
	Scope        string
	Subject      string
	SubjectParts []string
	TargetID     string
	Object       string
	SettingID    string
	ArtifactPath string
}

func parseDesiredURI(rawURI string) (ResolvedURI, error) {
	trimmed := strings.TrimSpace(rawURI)
	if trimmed == "" {
		return ResolvedURI{}, fmt.Errorf("desired URI is required")
	}
	if trimmed != rawURI {
		return ResolvedURI{}, fmt.Errorf("desired URI must not have surrounding whitespace")
	}
	if strings.ContainsAny(rawURI, "\\\x00") {
		return ResolvedURI{}, fmt.Errorf("desired URI contains forbidden characters")
	}
	if strings.Contains(rawURI, "%") {
		return ResolvedURI{}, fmt.Errorf("desired URI percent-encoding is not supported in MVP")
	}
	if strings.Contains(rawURI, "?") {
		return ResolvedURI{}, fmt.Errorf("desired URI query is not supported")
	}
	if strings.Contains(rawURI, "@") {
		return ResolvedURI{}, fmt.Errorf("desired URI userinfo/host ambiguity is not supported")
	}
	if !strings.HasPrefix(rawURI, Scheme+"://") {
		return ResolvedURI{}, fmt.Errorf("desired URI must start with desired://")
	}
	withoutScheme := strings.TrimPrefix(rawURI, Scheme+"://")
	if withoutScheme == "" || strings.HasPrefix(withoutScheme, "/") {
		return ResolvedURI{}, fmt.Errorf("desired URI path is required")
	}
	if strings.Count(withoutScheme, "#") > 1 {
		return ResolvedURI{}, fmt.Errorf("desired URI must contain at most one fragment")
	}
	pathPart, fragment, _ := strings.Cut(withoutScheme, "#")
	if strings.TrimSpace(fragment) != fragment {
		return ResolvedURI{}, fmt.Errorf("desired URI fragment must not have surrounding whitespace")
	}
	segments := strings.Split(pathPart, "/")
	if len(segments) < 5 {
		return ResolvedURI{}, fmt.Errorf("desired URI is incomplete")
	}
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." {
			return ResolvedURI{}, fmt.Errorf("desired URI contains unsafe segment")
		}
		if strings.Contains(segment, ":") {
			return ResolvedURI{}, fmt.Errorf("desired URI segment contains unsupported authority separator")
		}
	}

	parts, err := parseDesiredSegments(segments, fragment)
	if err != nil {
		return ResolvedURI{}, err
	}
	targetRelDirSlash := path.Join(append(append([]string{"desired", parts.Scope}, parts.SubjectParts...), "targets", parts.TargetID)...)
	var relPathSlash string
	switch parts.Object {
	case ObjectSettings:
		relPathSlash = path.Join(targetRelDirSlash, "settings.yaml")
	case ObjectManifest:
		relPathSlash = path.Join(targetRelDirSlash, "manifest.yaml")
	case ObjectArtifact:
		relPathSlash = path.Join(targetRelDirSlash, "artifacts", parts.ArtifactPath)
	default:
		return ResolvedURI{}, fmt.Errorf("unsupported desired URI object: %s", parts.Object)
	}
	return ResolvedURI{
		URI:          rawURI,
		Scope:        parts.Scope,
		Subject:      parts.Subject,
		TargetID:     parts.TargetID,
		Object:       parts.Object,
		SettingID:    parts.SettingID,
		ArtifactPath: parts.ArtifactPath,
		RelPath:      filepath.FromSlash(relPathSlash),
		TargetRelDir: filepath.FromSlash(targetRelDirSlash),
	}, nil
}

func parseDesiredSegments(segments []string, fragment string) (desiredURIParts, error) {
	scope := segments[0]
	subject, subjectParts, cursor, err := parseSubject(scope, segments)
	if err != nil {
		return desiredURIParts{}, err
	}
	if len(segments) <= cursor+2 || segments[cursor] != "targets" {
		return desiredURIParts{}, fmt.Errorf("desired URI must include targets/<target-id>/<object>")
	}
	targetID := segments[cursor+1]
	if err := validatePublicID("target", targetID); err != nil {
		return desiredURIParts{}, err
	}
	objectSegment := segments[cursor+2]
	objectRest := segments[cursor+3:]
	switch objectSegment {
	case ObjectSettings:
		if len(objectRest) != 0 {
			return desiredURIParts{}, fmt.Errorf("settings desired URI must not include extra path segments")
		}
		if fragment == "" {
			return desiredURIParts{}, fmt.Errorf("settings desired URI requires a setting fragment")
		}
		if err := validatePublicID("setting", fragment); err != nil {
			return desiredURIParts{}, err
		}
		return desiredURIParts{Scope: scope, Subject: subject, SubjectParts: subjectParts, TargetID: targetID, Object: ObjectSettings, SettingID: fragment}, nil
	case ObjectManifest:
		if len(objectRest) != 0 {
			return desiredURIParts{}, fmt.Errorf("manifest desired URI must not include extra path segments")
		}
		if fragment != "" {
			return desiredURIParts{}, fmt.Errorf("manifest desired URI must not include a fragment")
		}
		return desiredURIParts{Scope: scope, Subject: subject, SubjectParts: subjectParts, TargetID: targetID, Object: ObjectManifest}, nil
	case "artifacts":
		if len(objectRest) == 0 {
			return desiredURIParts{}, fmt.Errorf("artifact desired URI requires an artifact path")
		}
		if fragment != "" {
			return desiredURIParts{}, fmt.Errorf("artifact desired URI must not include a fragment")
		}
		artifactPath, err := validateArtifactPath(strings.Join(objectRest, "/"))
		if err != nil {
			return desiredURIParts{}, err
		}
		return desiredURIParts{Scope: scope, Subject: subject, SubjectParts: subjectParts, TargetID: targetID, Object: ObjectArtifact, ArtifactPath: artifactPath}, nil
	default:
		return desiredURIParts{}, fmt.Errorf("unsupported desired URI object: %s", objectSegment)
	}
}

func parseSubject(scope string, segments []string) (string, []string, int, error) {
	switch scope {
	case "shared":
		if len(segments) < 2 || segments[1] != "-" {
			return "", nil, 0, fmt.Errorf("shared desired URI subject must be -")
		}
		return "-", []string{"-"}, 2, nil
	case "user":
		if len(segments) < 2 {
			return "", nil, 0, fmt.Errorf("user desired URI subject is required")
		}
		if err := validateIdentityID("user", segments[1]); err != nil {
			return "", nil, 0, err
		}
		return segments[1], []string{segments[1]}, 2, nil
	case "machine":
		if len(segments) < 2 {
			return "", nil, 0, fmt.Errorf("machine desired URI subject is required")
		}
		if err := validateIdentityID("machine", segments[1]); err != nil {
			return "", nil, 0, err
		}
		return segments[1], []string{segments[1]}, 2, nil
	case "machine-user":
		if len(segments) < 3 {
			return "", nil, 0, fmt.Errorf("machine-user desired URI requires machine and user subjects")
		}
		if err := validateIdentityID("machine", segments[1]); err != nil {
			return "", nil, 0, err
		}
		if err := validateIdentityID("user", segments[2]); err != nil {
			return "", nil, 0, err
		}
		return segments[1] + "/" + segments[2], []string{segments[1], segments[2]}, 3, nil
	default:
		return "", nil, 0, fmt.Errorf("unknown desired scope: %s", scope)
	}
}

type settingsFile struct {
	Schema        string                        `yaml:"schema"`
	SchemaVersion int                           `yaml:"schemaVersion"`
	Values        map[string]selectedValueEntry `yaml:"values,omitempty"`
}

type selectedValueEntry struct {
	Intent string    `yaml:"intent"`
	Kind   string    `yaml:"kind,omitempty"`
	Value  yaml.Node `yaml:"value,omitempty"`
}

func loadOrCreateSettingsFile(path string) (*settingsFile, error) {
	file, err := loadSettingsFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &settingsFile{Schema: SettingsSchema, SchemaVersion: SchemaVersion, Values: map[string]selectedValueEntry{}}, nil
	}
	return file, err
}

func loadSettingsFile(path string) (*settingsFile, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	dec := yaml.NewDecoder(file)
	dec.KnownFields(true)
	var parsed settingsFile
	if err := dec.Decode(&parsed); err != nil {
		return nil, fmt.Errorf("parse desired settings: %w", err)
	}
	if err := validateSettingsFile(parsed); err != nil {
		return nil, err
	}
	if parsed.Values == nil {
		parsed.Values = map[string]selectedValueEntry{}
	}
	return &parsed, nil
}

func validateSettingsFile(file settingsFile) error {
	if file.Schema != SettingsSchema {
		return fmt.Errorf("invalid desired settings schema")
	}
	if file.SchemaVersion != SchemaVersion {
		return fmt.Errorf("invalid desired settings schemaVersion")
	}
	for settingID := range file.Values {
		if err := validatePublicID("setting", settingID); err != nil {
			return err
		}
	}
	return nil
}

func selectedValueFromEntry(entry selectedValueEntry) (SelectedValue, error) {
	switch entry.Intent {
	case IntentSet:
		switch entry.Kind {
		case KindString:
			if entry.Value.Kind == 0 || entry.Value.Kind != yaml.ScalarNode || entry.Value.Tag != "!!str" {
				return SelectedValue{}, valueError("desired string entry must contain a string value")
			}
			return SetString(entry.Value.Value), nil
		case KindBool:
			if entry.Value.Kind == 0 || entry.Value.Kind != yaml.ScalarNode || entry.Value.Tag != "!!bool" {
				return SelectedValue{}, valueError("desired bool entry must contain a bool value")
			}
			var value bool
			if err := entry.Value.Decode(&value); err != nil {
				return SelectedValue{}, valueError("desired bool entry is invalid")
			}
			return SetBool(value), nil
		case KindNumber:
			if entry.Value.Kind == 0 || entry.Value.Kind != yaml.ScalarNode {
				return SelectedValue{}, valueError("desired number entry must contain a JSON number value")
			}
			number := json.Number(entry.Value.Value)
			if _, err := json.Marshal(number); err != nil {
				return SelectedValue{}, valueError("desired number entry must contain a valid JSON number")
			}
			return SetNumber(number), nil
		case KindNull:
			if entry.Value.Kind != 0 && entry.Value.Tag != "!!null" {
				return SelectedValue{}, valueError("desired null entry must not contain a non-null value")
			}
			return SetNull(), nil
		default:
			return SelectedValue{}, valueError("desired set entry has unsupported kind")
		}
	case IntentDelete:
		if entry.Kind != "" || entry.Value.Kind != 0 {
			return SelectedValue{}, valueError("desired delete entry must not contain kind or value")
		}
		return Delete(), nil
	case IntentUnmanaged:
		if entry.Kind != "" || entry.Value.Kind != 0 {
			return SelectedValue{}, valueError("desired unmanaged entry must not contain kind or value")
		}
		return Unmanaged(), nil
	default:
		return SelectedValue{}, valueError("desired entry intent is unsupported")
	}
}

func entryFromSelectedValue(value SelectedValue) (selectedValueEntry, error) {
	switch value.intent {
	case IntentSet:
		switch value.kind {
		case KindString:
			v, ok := value.value.(string)
			if !ok {
				return selectedValueEntry{}, valueError("desired string value has invalid internal representation")
			}
			return selectedValueEntry{Intent: IntentSet, Kind: KindString, Value: *scalarNode("!!str", v)}, nil
		case KindBool:
			v, ok := value.value.(bool)
			if !ok {
				return selectedValueEntry{}, valueError("desired bool value has invalid internal representation")
			}
			return selectedValueEntry{Intent: IntentSet, Kind: KindBool, Value: *scalarNode("!!bool", fmt.Sprintf("%t", v))}, nil
		case KindNumber:
			v, ok := value.value.(json.Number)
			if !ok {
				return selectedValueEntry{}, valueError("desired number value has invalid internal representation")
			}
			if _, err := json.Marshal(v); err != nil {
				return selectedValueEntry{}, valueError("desired number value must be a valid JSON number")
			}
			return selectedValueEntry{Intent: IntentSet, Kind: KindNumber, Value: *scalarNode("!!str", v.String())}, nil
		case KindNull:
			return selectedValueEntry{Intent: IntentSet, Kind: KindNull}, nil
		default:
			return selectedValueEntry{}, valueError("desired set value kind is unsupported")
		}
	case IntentDelete:
		return selectedValueEntry{Intent: IntentDelete}, nil
	case IntentUnmanaged:
		return selectedValueEntry{Intent: IntentUnmanaged}, nil
	default:
		return selectedValueEntry{}, valueError("desired intent is required")
	}
}

func scalarNode(tag string, value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value}
}

func writeSettingsFileAtomic(root string, path string, file *settingsFile) error {
	data, err := marshalSettingsFile(file)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := ensureSafeWritePath(root, path); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".settings.yaml.tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func marshalSettingsFile(file *settingsFile) ([]byte, error) {
	root := &yaml.Node{Kind: yaml.MappingNode}
	appendScalarPair(root, "schema", SettingsSchema)
	appendIntPair(root, "schemaVersion", SchemaVersion)
	values := &yaml.Node{Kind: yaml.MappingNode}
	for _, settingID := range sortedKeys(file.Values) {
		entry := file.Values[settingID]
		entryNode := &yaml.Node{Kind: yaml.MappingNode}
		appendScalarPair(entryNode, "intent", entry.Intent)
		if entry.Kind != "" {
			appendScalarPair(entryNode, "kind", entry.Kind)
		}
		if entry.Value.Kind != 0 {
			value := entry.Value
			entryNode.Content = append(entryNode.Content, scalarNode("!!str", "value"), &value)
		}
		values.Content = append(values.Content, scalarNode("!!str", settingID), entryNode)
	}
	root.Content = append(root.Content, scalarNode("!!str", "values"), values)
	var b strings.Builder
	encoder := yaml.NewEncoder(&b)
	encoder.SetIndent(2)
	if err := encoder.Encode(root); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return []byte(b.String()), nil
}

func appendScalarPair(root *yaml.Node, key string, value string) {
	root.Content = append(root.Content, scalarNode("!!str", key), scalarNode("!!str", value))
}

func appendIntPair(root *yaml.Node, key string, value int) {
	root.Content = append(root.Content, scalarNode("!!str", key), scalarNode("!!int", fmt.Sprintf("%d", value)))
}

func validateWriteSafety(resolved ResolvedURI, value SelectedValue, decision *WriteSafetyDecision) error {
	if decision == nil {
		return safetyError(diagnostic("desired.writeSafety.decisionRequired", "$", "desired-value write safety decision is required"))
	}
	if decision.Recipe == nil {
		return safetyError(diagnostic("desired.writeSafety.recipeRequired", "$", "recipe is required for desired-value writes"))
	}
	if err := decision.Recipe.Validate(); err != nil {
		return safetyError(diagnostic("desired.writeSafety.recipeInvalid", "$", "recipe validation failed"))
	}
	settingID, _, err := resolveSettingRef(decision.Recipe, decision.SettingRef)
	if err != nil {
		return safetyError(diagnostic("desired.writeSafety.settingRefInvalid", "$", err.Error()))
	}
	if decision.Recipe.Target != resolved.TargetID || settingID != resolved.SettingID {
		return safetyError(diagnostic("desired.writeSafety.settingMismatch", "$", "write safety setting context does not match desired URI"))
	}
	setting, ok := decision.Recipe.Settings[settingID]
	if !ok {
		return safetyError(diagnostic("desired.writeSafety.settingUnknown", "$.settings", fmt.Sprintf("unknown setting %s", settingID)))
	}
	resourceID, resource, err := decision.Recipe.ResourceForSetting(settingID)
	if err != nil {
		return safetyError(diagnostic("desired.writeSafety.resourceUnknown", "$.resources", err.Error()))
	}

	diagnostics := []Diagnostic{}
	add := func(code string, path string, message string) {
		diagnostics = append(diagnostics, diagnostic(code, path, message))
	}
	if safetyErr := decision.Recipe.ValidateWriteSafety(decision.Context); safetyErr != nil {
		for _, recipeDiagnostic := range recipe.ValidationDiagnostics(safetyErr) {
			add(recipeDiagnostic.Code, recipeDiagnostic.Path, recipeDiagnostic.Message)
		}
	}
	if !isSelectedValueDriver(resource.Driver) {
		add("desired.writeSafety.driverUnsupported", "$.resources."+resourceID+".driver", fmt.Sprintf("resource %s driver is not a selected-value driver", resourceID))
	} else if !desiredValueCompatibleWithDriver(resource.Driver, value) {
		add("desired.writeSafety.desiredTypeUnsupported", "$.resources."+resourceID+".driver", fmt.Sprintf("resource %s driver does not support the desired value kind", resourceID))
	}
	if !isWriteCapable(effectiveSettingCapability(decision.Recipe, setting)) {
		add("desired.writeSafety.setting.capabilityBlocked", "$.settings."+settingID+".capability", fmt.Sprintf("setting %s is not write-capable", settingID))
	}
	if !isWriteCapable(effectiveResourceCapability(decision.Recipe, resource)) {
		add("desired.writeSafety.resource.capabilityBlocked", "$.resources."+resourceID+".capability", fmt.Sprintf("resource %s is not write-capable", resourceID))
	}
	settingPath := "$.settings." + settingID
	resourcePath := "$.resources." + resourceID
	if setting.Sensitivity == "" {
		add("desired.writeSafety.setting.sensitivity.required", settingPath+".sensitivity", fmt.Sprintf("setting %s requires sensitivity metadata before desired-value writes", settingID))
	} else {
		addSensitivityDiagnostics(add, setting.Sensitivity, settingPath+".sensitivity", "setting "+settingID, decision.Context)
	}
	if setting.Redaction == "" {
		add("desired.writeSafety.setting.redaction.required", settingPath+".redaction", fmt.Sprintf("setting %s requires redaction metadata before desired-value writes", settingID))
	} else {
		addRedactionDiagnostics(add, setting.Redaction, settingPath+".redaction", "setting "+settingID, decision.Context)
	}
	if resource.Sensitivity == "" {
		add("desired.writeSafety.resource.sensitivity.required", resourcePath+".sensitivity", fmt.Sprintf("resource %s requires sensitivity metadata before desired-value writes", resourceID))
	} else {
		addSensitivityDiagnostics(add, resource.Sensitivity, resourcePath+".sensitivity", "resource "+resourceID, decision.Context)
	}
	if resource.Redaction == "" {
		add("desired.writeSafety.resource.redaction.required", resourcePath+".redaction", fmt.Sprintf("resource %s requires redaction metadata before desired-value writes", resourceID))
	} else {
		addRedactionDiagnostics(add, resource.Redaction, resourcePath+".redaction", "resource "+resourceID, decision.Context)
	}
	if decision.Context.Source == "" {
		add("desired.writeSafety.trust.sourceRequired", "$", "write safety context source is required before desired-value writes")
	} else if decision.Context.Source == recipe.RecipeSourceLocal && !decision.Context.Trusted {
		add("desired.writeSafety.trust.untrusted", "$", "local recipes require explicit trust before desired-value writes")
	} else if decision.Context.Source != recipe.RecipeSourceLocal && decision.Context.Source != recipe.RecipeSourceBundled {
		add("desired.writeSafety.trust.sourceUnsupported", "$", "write safety context source must be bundled or local")
	}
	if len(diagnostics) > 0 {
		return &SafetyError{Diagnostics: diagnostics}
	}
	return nil
}

func addSensitivityDiagnostics(add func(string, string, string), value string, path string, subject string, ctx recipe.WriteSafetyContext) {
	switch value {
	case recipe.SensitivitySecret:
		if !ctx.AllowSensitive {
			add("desired.writeSafety.sensitivity.secretBlocked", path, subject+" sensitivity policy secret requires explicit sensitive-value approval")
		}
	case recipe.SensitivityUnknown:
		if !ctx.AllowUnknownSensitivity {
			add("desired.writeSafety.sensitivity.unknownBlocked", path, subject+" sensitivity policy unknown requires explicit unknown-sensitivity approval")
		}
	}
}

func addRedactionDiagnostics(add func(string, string, string), value string, path string, subject string, ctx recipe.WriteSafetyContext) {
	switch value {
	case recipe.RedactionBlockedSave:
		add("desired.writeSafety.redaction.blockedSave", path, subject+" redaction policy blocks desired-value persistence")
	case recipe.RedactionUnavailable:
		if !ctx.AllowOpaque {
			add("desired.writeSafety.redaction.unavailable", path, subject+" redaction policy requires explicit opaque-artifact approval")
		}
	}
}

func resolveSettingRef(rec *recipe.Recipe, ref string) (string, string, error) {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" {
		return "", "", fmt.Errorf("setting ref is required")
	}
	if strings.Contains(trimmed, ":") {
		parts := strings.Split(trimmed, ":")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return "", trimmed, fmt.Errorf("invalid setting ref %s", trimmed)
		}
		if parts[0] != rec.Target {
			return "", trimmed, fmt.Errorf("setting ref target %s does not match recipe target %s", parts[0], rec.Target)
		}
		return parts[1], trimmed, nil
	}
	return trimmed, rec.Target + ":" + trimmed, nil
}

func isSelectedValueDriver(driver string) bool {
	switch driver {
	case recipe.IniFileDriverID, recipe.JSONFileDriverID, recipe.YAMLFileDriverID:
		return true
	default:
		return false
	}
}

func desiredValueCompatibleWithDriver(driver string, value SelectedValue) bool {
	switch value.intent {
	case IntentDelete, IntentUnmanaged:
		return true
	case IntentSet:
		switch driver {
		case recipe.IniFileDriverID:
			return value.kind == KindString
		case recipe.JSONFileDriverID, recipe.YAMLFileDriverID:
			switch value.kind {
			case KindString, KindBool, KindNumber, KindNull:
				return true
			default:
				return false
			}
		default:
			return false
		}
	default:
		return false
	}
}

func isWriteCapable(capability string) bool {
	switch capability {
	case "read-write", "import-only", "export-only":
		return true
	default:
		return false
	}
}

func effectiveSettingCapability(rec *recipe.Recipe, setting recipe.Setting) string {
	if setting.Capability != "" {
		return setting.Capability
	}
	return rec.Capability
}

func effectiveResourceCapability(rec *recipe.Recipe, resource recipe.Resource) string {
	if resource.Capability != "" {
		return resource.Capability
	}
	return rec.Capability
}

func safetyError(diagnostics ...Diagnostic) error { return &SafetyError{Diagnostics: diagnostics} }

func diagnostic(code string, path string, message string) Diagnostic {
	return Diagnostic{Code: code, Severity: SeverityError, Path: path, Message: message}
}

func valueError(message string) error {
	return fmt.Errorf("desired selected-value entry is invalid: %s", message)
}

func normalizeRepoRoot(repoRoot string) (string, error) {
	trimmed := strings.TrimSpace(repoRoot)
	if trimmed == "" {
		return "", fmt.Errorf("repo root is required")
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("repo root is not a directory: %s", abs)
	}
	return abs, nil
}

func validateIdentityID(kind string, value string) error {
	if !identityIDRegexp.MatchString(value) {
		return fmt.Errorf("invalid %s id: %s", kind, value)
	}
	return nil
}

func validatePublicID(kind string, value string) error {
	if strings.TrimSpace(value) != value || !publicIDPattern.MatchString(value) {
		return fmt.Errorf("invalid %s id: %s", kind, value)
	}
	return nil
}

func validateArtifactPath(value string) (string, error) {
	if strings.TrimSpace(value) != value || value == "" {
		return "", fmt.Errorf("artifact path is required")
	}
	if strings.Contains(value, "\\") || filepath.IsAbs(value) || strings.HasPrefix(value, "/") {
		return "", fmt.Errorf("artifact path must be relative and use forward slashes")
	}
	segments := strings.Split(value, "/")
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." {
			return "", fmt.Errorf("artifact path contains unsafe segment")
		}
	}
	cleaned := path.Clean(value)
	if cleaned != value || strings.HasPrefix(cleaned, "../") || cleaned == ".." {
		return "", fmt.Errorf("artifact path escapes desired target directory")
	}
	return value, nil
}

func ensurePathInside(base string, candidate string) error {
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return err
	}
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(baseAbs, candidateAbs)
	if err != nil {
		return err
	}
	relSlash := filepath.ToSlash(rel)
	if relSlash == ".." || strings.HasPrefix(relSlash, "../") || filepath.IsAbs(rel) {
		return fmt.Errorf("resolved desired path escapes target directory: %s", candidate)
	}
	return nil
}

func ensureSafeReadPath(root string, path string) (bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		if err := ensureNoSymlinkParents(root, filepath.Dir(path)); err != nil {
			return false, err
		}
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false, fmt.Errorf("desired settings path must not be a symlink: %s", path)
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("desired settings path must be a regular file: %s", path)
	}
	if err := ensureNoSymlinkParents(root, filepath.Dir(path)); err != nil {
		return false, err
	}
	return true, nil
}

func ensureSafeWritePath(root string, path string) error {
	if err := ensureNoSymlinkParents(root, filepath.Dir(path)); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("desired settings path must not be a symlink: %s", path)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("desired settings path must be a regular file: %s", path)
	}
	return nil
}

func ensureNoSymlinkParents(root string, dir string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	dirAbs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if err := ensurePathInside(rootAbs, dirAbs); err != nil {
		return err
	}
	rootInfo, err := os.Lstat(rootAbs)
	if err != nil {
		return err
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("repo root must not be a symlink for desired writes: %s", rootAbs)
	}
	rel, err := filepath.Rel(rootAbs, dirAbs)
	if err != nil {
		return err
	}
	if rel == "." {
		return nil
	}
	current := rootAbs
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("desired settings parent must not be a symlink: %s", current)
		}
	}
	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func defaultString(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
