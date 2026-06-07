package recipe

import (
	"fmt"
	"io"
	"os"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/shpoont/dotfiles-manager/internal/v2/filetreedriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/inidriver"
	"gopkg.in/yaml.v3"
)

const (
	Schema             = "dotfiles-manager.v2.recipe"
	SupportedVersion   = 1
	CustomFilesTarget  = "custom.files"
	GitTarget          = "git"
	FileDriverID       = "file"
	FileTreeDriverID   = "file-tree"
	IniFileDriverID    = "ini-file"
	localRecipeRelRoot = "recipes/local"
)

var (
	publicIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*(?:[.-][a-z0-9][a-z0-9_-]*)*$`)
	recipeIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

type Recipe struct {
	Schema        string              `yaml:"schema"`
	SchemaVersion int                 `yaml:"schemaVersion"`
	Target        string              `yaml:"target"`
	DisplayName   string              `yaml:"displayName"`
	SupportLevel  string              `yaml:"supportLevel"`
	Capability    string              `yaml:"capability"`
	Locations     map[string]Location `yaml:"locations"`
	Settings      map[string]Setting  `yaml:"settings"`
	Resources     map[string]Resource `yaml:"resources"`
}

type Location struct {
	Default string `yaml:"default"`
}

type Setting struct {
	ScopeDefault string `yaml:"scopeDefault"`
	Resource     string `yaml:"resource"`
}

type Resource struct {
	Driver   string       `yaml:"driver"`
	Location string       `yaml:"location"`
	Path     string       `yaml:"path"`
	Include  []string     `yaml:"include,omitempty"`
	Exclude  []string     `yaml:"exclude,omitempty"`
	Selector *INISelector `yaml:"selector,omitempty"`
}

type INISelector struct {
	Section         string `yaml:"section"`
	Key             string `yaml:"key"`
	MissingSection  string `yaml:"missingSection,omitempty"`
	MissingKey      string `yaml:"missingKey,omitempty"`
	DuplicatePolicy string `yaml:"duplicatePolicy,omitempty"`
	DeleteKey       string `yaml:"deleteKey,omitempty"`
}

func LoadCustomFiles(repoRoot string) (*Recipe, error) {
	rec, err := LoadLocal(repoRoot, CustomFilesTarget)
	if err != nil {
		return nil, err
	}
	if err := rec.ValidateCustomFiles(); err != nil {
		return nil, err
	}
	return rec, nil
}

func LoadGit(repoRoot string) (*Recipe, error) {
	rec, err := LoadLocal(repoRoot, GitTarget)
	if err != nil {
		return nil, err
	}
	if err := rec.ValidateGit(); err != nil {
		return nil, err
	}
	return rec, nil
}

func LoadLocal(repoRoot string, recipeID string) (*Recipe, error) {
	root, err := normalizeRepoRoot(repoRoot)
	if err != nil {
		return nil, err
	}
	id, err := validateRecipeID(recipeID)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(root, localRecipeRelRoot, id, "recipe.yaml")
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read local recipe %s: %w", path, err)
	}
	defer file.Close()
	return Decode(path, file)
}

func Decode(name string, r io.Reader) (*Recipe, error) {
	var rec Recipe
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)
	if err := dec.Decode(&rec); err != nil {
		return nil, fmt.Errorf("parse recipe %s: %w", name, err)
	}
	if err := rec.Validate(); err != nil {
		return nil, fmt.Errorf("validate recipe %s: %w", name, err)
	}
	return &rec, nil
}

func (r *Recipe) Validate() error {
	if r == nil {
		return fmt.Errorf("recipe is required")
	}
	if r.Schema != Schema {
		return fmt.Errorf("invalid recipe schema: %q (expected %q)", r.Schema, Schema)
	}
	if r.SchemaVersion != SupportedVersion {
		return fmt.Errorf("invalid recipe schemaVersion: %d (expected %d)", r.SchemaVersion, SupportedVersion)
	}
	if err := ValidatePublicID("target", r.Target); err != nil {
		return err
	}
	if strings.TrimSpace(r.DisplayName) == "" {
		return fmt.Errorf("recipe displayName is required")
	}
	if !knownSupportLevel(r.SupportLevel) {
		return fmt.Errorf("unsupported supportLevel: %s", r.SupportLevel)
	}
	if !knownCapability(r.Capability) {
		return fmt.Errorf("unsupported capability: %s", r.Capability)
	}
	if len(r.Locations) == 0 {
		return fmt.Errorf("recipe must declare at least one location")
	}
	if len(r.Resources) == 0 {
		return fmt.Errorf("recipe must declare at least one resource")
	}
	if len(r.Settings) == 0 {
		return fmt.Errorf("recipe must declare at least one setting")
	}

	for _, locationID := range sortedKeys(r.Locations) {
		location := r.Locations[locationID]
		if err := ValidatePublicID("location", locationID); err != nil {
			return err
		}
		if strings.TrimSpace(location.Default) == "" {
			return fmt.Errorf("location %s default is required", locationID)
		}
		if strings.ContainsRune(location.Default, '\x00') {
			return fmt.Errorf("location %s default contains NUL", locationID)
		}
	}

	for _, resourceID := range sortedKeys(r.Resources) {
		resource := r.Resources[resourceID]
		if err := ValidatePublicID("resource", resourceID); err != nil {
			return err
		}
		if resource.Driver == "" {
			return fmt.Errorf("resource %s driver is required", resourceID)
		}
		if resource.Location == "" {
			return fmt.Errorf("resource %s location is required", resourceID)
		}
		if _, ok := r.Locations[resource.Location]; !ok {
			return fmt.Errorf("resource %s references unknown location %s", resourceID, resource.Location)
		}
		if _, err := ValidateResourcePath(resource.Path); err != nil {
			return fmt.Errorf("resource %s path: %w", resourceID, err)
		}
		if err := r.validateResourceDriverShape(resourceID, resource); err != nil {
			return err
		}
	}

	for _, settingID := range sortedKeys(r.Settings) {
		setting := r.Settings[settingID]
		if err := ValidatePublicID("setting", settingID); err != nil {
			return err
		}
		if setting.ScopeDefault != "" && !knownScope(setting.ScopeDefault) {
			return fmt.Errorf("setting %s unsupported scopeDefault: %s", settingID, setting.ScopeDefault)
		}
		if setting.Resource == "" {
			return fmt.Errorf("setting %s resource is required", settingID)
		}
		if _, ok := r.Resources[setting.Resource]; !ok {
			return fmt.Errorf("setting %s references unknown resource %s", settingID, setting.Resource)
		}
	}
	return nil
}

func (r *Recipe) ValidateGit() error {
	if err := r.Validate(); err != nil {
		return err
	}
	if r.Target != GitTarget {
		return fmt.Errorf("git recipe target must be %q, got %q", GitTarget, r.Target)
	}
	if r.Capability != "read-write" {
		return fmt.Errorf("git recipe capability must be read-write, got %s", r.Capability)
	}
	if len(r.Locations) != 1 {
		return fmt.Errorf("git recipe must declare only the home location")
	}
	location, ok := r.Locations["home"]
	if !ok {
		return fmt.Errorf("git recipe must declare home location")
	}
	if strings.TrimSpace(location.Default) != "~" {
		return fmt.Errorf("git home location default must be ~")
	}
	if len(r.Settings) != 2 {
		return fmt.Errorf("git recipe must declare only user.name and user.email settings")
	}
	if len(r.Resources) != 2 {
		return fmt.Errorf("git recipe must declare exactly two selected-key resources")
	}
	if err := r.validateGitSetting("user.name", "name"); err != nil {
		return err
	}
	if err := r.validateGitSetting("user.email", "email"); err != nil {
		return err
	}
	return nil
}

func (r *Recipe) validateGitSetting(settingID string, key string) error {
	setting, ok := r.Settings[settingID]
	if !ok {
		return fmt.Errorf("git recipe missing setting %s", settingID)
	}
	if setting.ScopeDefault != "user" {
		return fmt.Errorf("git setting %s scopeDefault must be user", settingID)
	}
	resource, ok := r.Resources[setting.Resource]
	if !ok {
		return fmt.Errorf("git setting %s references unknown resource %s", settingID, setting.Resource)
	}
	if resource.Driver != IniFileDriverID {
		return fmt.Errorf("git setting %s driver must be %q, got %q", settingID, IniFileDriverID, resource.Driver)
	}
	if resource.Location != "home" {
		return fmt.Errorf("git setting %s location must be home", settingID)
	}
	if resource.Path != ".gitconfig" {
		return fmt.Errorf("git setting %s path must be .gitconfig", settingID)
	}
	if resource.Selector == nil {
		return fmt.Errorf("git setting %s requires an INI selector", settingID)
	}
	if resource.Selector.Section != "user" || resource.Selector.Key != key {
		return fmt.Errorf("git setting %s must select [user] %s", settingID, key)
	}
	if selectorMissingSection(resource.Selector) != string(inidriver.MissingPolicyCreate) {
		return fmt.Errorf("git setting %s missingSection must be %q", settingID, inidriver.MissingPolicyCreate)
	}
	if selectorMissingKey(resource.Selector) != string(inidriver.MissingPolicyCreate) {
		return fmt.Errorf("git setting %s missingKey must be %q", settingID, inidriver.MissingPolicyCreate)
	}
	if selectorDuplicatePolicy(resource.Selector) != string(inidriver.DuplicatePolicyReject) {
		return fmt.Errorf("git setting %s duplicatePolicy must be %q", settingID, inidriver.DuplicatePolicyReject)
	}
	if selectorDeleteKey(resource.Selector) != string(inidriver.DeletePolicyReject) {
		return fmt.Errorf("git setting %s deleteKey must be %q", settingID, inidriver.DeletePolicyReject)
	}
	return nil
}

func (r *Recipe) ValidateCustomFiles() error {
	if err := r.Validate(); err != nil {
		return err
	}
	if r.Target != CustomFilesTarget {
		return fmt.Errorf("custom.files recipe target must be %q, got %q", CustomFilesTarget, r.Target)
	}
	if r.Capability != "read-write" {
		return fmt.Errorf("custom.files recipe capability must be read-write, got %s", r.Capability)
	}
	if len(r.Resources) == 0 {
		return fmt.Errorf("custom.files recipe must declare at least one resource")
	}
	for resourceID, resource := range r.Resources {
		switch resource.Driver {
		case FileDriverID:
			if len(resource.Include) > 0 || len(resource.Exclude) > 0 {
				return fmt.Errorf("custom.files resource %s driver %q must not declare include/exclude globs", resourceID, FileDriverID)
			}
			if resource.Selector != nil {
				return fmt.Errorf("custom.files resource %s driver %q must not declare selector", resourceID, FileDriverID)
			}
		case FileTreeDriverID:
			if _, _, err := filetreedriver.NormalizeGlobs(resource.Include, resource.Exclude); err != nil {
				return fmt.Errorf("custom.files resource %s globs: %w", resourceID, err)
			}
			if resource.Selector != nil {
				return fmt.Errorf("custom.files resource %s driver %q must not declare selector", resourceID, FileTreeDriverID)
			}
		default:
			return fmt.Errorf("custom.files resource %s driver must be %q or %q, got %q", resourceID, FileDriverID, FileTreeDriverID, resource.Driver)
		}
	}
	return nil
}

func (r *Recipe) ResourceForSetting(settingID string) (string, Resource, error) {
	if r == nil {
		return "", Resource{}, fmt.Errorf("recipe is required")
	}
	setting, ok := r.Settings[settingID]
	if !ok {
		return "", Resource{}, fmt.Errorf("recipe target %s has no setting %s", r.Target, settingID)
	}
	resource, ok := r.Resources[setting.Resource]
	if !ok {
		return "", Resource{}, fmt.Errorf("setting %s references unknown resource %s", settingID, setting.Resource)
	}
	return setting.Resource, resource, nil
}

func (r *Recipe) LocationRoot(locationID string, overrides map[string]string) (string, error) {
	if override := strings.TrimSpace(overrides[locationID]); override != "" {
		return override, nil
	}
	location, ok := r.Locations[locationID]
	if !ok {
		return "", fmt.Errorf("unknown location %s", locationID)
	}
	return ExpandLocationDefault(location.Default)
}

func (r *Recipe) validateResourceDriverShape(resourceID string, resource Resource) error {
	if r.Target == CustomFilesTarget {
		switch resource.Driver {
		case FileDriverID, FileTreeDriverID:
		default:
			return fmt.Errorf("custom.files resource %s driver must be %q or %q, got %q", resourceID, FileDriverID, FileTreeDriverID, resource.Driver)
		}
	}
	switch resource.Driver {
	case IniFileDriverID:
		return validateINIResource(resourceID, resource)
	case FileDriverID, FileTreeDriverID:
		if resource.Selector != nil {
			return fmt.Errorf("resource %s driver %q must not declare selector", resourceID, resource.Driver)
		}
	default:
		return fmt.Errorf("resource %s unsupported driver %q", resourceID, resource.Driver)
	}
	return nil
}

func validateINIResource(resourceID string, resource Resource) error {
	if len(resource.Include) > 0 || len(resource.Exclude) > 0 {
		return fmt.Errorf("resource %s driver %q must not declare include/exclude globs", resourceID, IniFileDriverID)
	}
	if resource.Selector == nil {
		return fmt.Errorf("resource %s driver %q requires selector", resourceID, IniFileDriverID)
	}
	selector := resource.Selector
	if selector.Section == "" || strings.TrimSpace(selector.Section) != selector.Section {
		return fmt.Errorf("resource %s selector section is required and must not have surrounding whitespace", resourceID)
	}
	if strings.ContainsAny(selector.Section, "\r\n[]\x00") {
		return fmt.Errorf("resource %s selector section must be an unbracketed single-line section name", resourceID)
	}
	if selector.Key == "" || strings.TrimSpace(selector.Key) != selector.Key {
		return fmt.Errorf("resource %s selector key is required and must not have surrounding whitespace", resourceID)
	}
	if strings.ContainsAny(selector.Key, "\r\n=\x00") {
		return fmt.Errorf("resource %s selector key must be a single-line key name without equals", resourceID)
	}
	switch selectorMissingSection(selector) {
	case string(inidriver.MissingPolicyError), string(inidriver.MissingPolicyCreate):
	default:
		return fmt.Errorf("resource %s unsupported selector missingSection policy %q", resourceID, selector.MissingSection)
	}
	switch selectorMissingKey(selector) {
	case string(inidriver.MissingPolicyError), string(inidriver.MissingPolicyCreate):
	default:
		return fmt.Errorf("resource %s unsupported selector missingKey policy %q", resourceID, selector.MissingKey)
	}
	switch selectorDuplicatePolicy(selector) {
	case string(inidriver.DuplicatePolicyReject):
	default:
		return fmt.Errorf("resource %s unsupported selector duplicatePolicy %q", resourceID, selector.DuplicatePolicy)
	}
	switch selectorDeleteKey(selector) {
	case string(inidriver.DeletePolicyReject), string(inidriver.DeletePolicyAllow):
	default:
		return fmt.Errorf("resource %s unsupported selector deleteKey policy %q", resourceID, selector.DeleteKey)
	}
	return nil
}

func selectorMissingSection(selector *INISelector) string {
	if selector == nil || selector.MissingSection == "" {
		return string(inidriver.MissingPolicyError)
	}
	return selector.MissingSection
}

func selectorMissingKey(selector *INISelector) string {
	if selector == nil || selector.MissingKey == "" {
		return string(inidriver.MissingPolicyError)
	}
	return selector.MissingKey
}

func selectorDuplicatePolicy(selector *INISelector) string {
	if selector == nil || selector.DuplicatePolicy == "" {
		return string(inidriver.DuplicatePolicyReject)
	}
	return selector.DuplicatePolicy
}

func selectorDeleteKey(selector *INISelector) string {
	if selector == nil || selector.DeleteKey == "" {
		return string(inidriver.DeletePolicyReject)
	}
	return selector.DeleteKey
}

func ExpandLocationDefault(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("location default is required")
	}
	if strings.ContainsRune(trimmed, '\x00') {
		return "", fmt.Errorf("location default contains NUL")
	}
	if trimmed == "~" || strings.HasPrefix(trimmed, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		if trimmed == "~" {
			return home, nil
		}
		return filepath.Join(home, filepath.FromSlash(strings.TrimPrefix(trimmed, "~/"))), nil
	}
	return trimmed, nil
}

func ValidatePublicID(kind string, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed != value {
		return fmt.Errorf("invalid %s id: %s", kind, value)
	}
	if !publicIDPattern.MatchString(value) {
		return fmt.Errorf("invalid %s id: %s", kind, value)
	}
	return nil
}

func ValidateResourcePath(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("resource path is required")
	}
	if trimmed != value {
		return "", fmt.Errorf("resource path must not have surrounding whitespace: %s", value)
	}
	if strings.Contains(trimmed, "\\") {
		return "", fmt.Errorf("resource path must not contain backslashes: %s", value)
	}
	if filepath.IsAbs(trimmed) || strings.HasPrefix(trimmed, "/") {
		return "", fmt.Errorf("resource path must be relative: %s", value)
	}
	slashed := filepath.ToSlash(trimmed)
	parts := strings.Split(slashed, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("resource path contains unsafe segment: %s", value)
		}
	}
	cleaned := pathpkg.Clean(slashed)
	if cleaned != slashed || strings.HasPrefix(cleaned, "../") || cleaned == ".." {
		return "", fmt.Errorf("resource path escapes named location: %s", value)
	}
	return slashed, nil
}

func normalizeRepoRoot(repoRoot string) (string, error) {
	trimmed := strings.TrimSpace(repoRoot)
	if trimmed == "" {
		return "", fmt.Errorf("repo root is required")
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", fmt.Errorf("resolve repo root %q: %w", repoRoot, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("stat repo root %q: %w", abs, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("repo root is not a directory: %s", abs)
	}
	return abs, nil
}

func validateRecipeID(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("recipe id is required")
	}
	if trimmed != value || strings.ContainsAny(trimmed, `/\\`) || filepath.IsAbs(trimmed) || !recipeIDPattern.MatchString(trimmed) {
		return "", fmt.Errorf("invalid recipe id: %s", value)
	}
	return trimmed, nil
}

func knownSupportLevel(value string) bool {
	switch value {
	case "stable", "read-only", "experimental", "deprecated", "blocked":
		return true
	default:
		return false
	}
}

func knownCapability(value string) bool {
	switch value {
	case "inspect-only", "read-only", "read-write", "import-only", "export-only", "never":
		return true
	default:
		return false
	}
}

func knownScope(value string) bool {
	switch value {
	case "shared", "user", "machine", "machine-user":
		return true
	default:
		return false
	}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
