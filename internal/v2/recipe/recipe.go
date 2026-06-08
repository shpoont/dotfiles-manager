package recipe

import (
	"bytes"
	"errors"
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
	"github.com/shpoont/dotfiles-manager/internal/v2/macosdefaultsdriver"
	"gopkg.in/yaml.v3"
)

const (
	Schema                        = "dotfiles-manager.v2.recipe"
	SupportedVersion              = 1
	CustomFilesTarget             = "custom.files"
	GitTarget                     = "git"
	StarshipTarget                = "starship"
	FileDriverID                  = "file"
	FileTreeDriverID              = "file-tree"
	IniFileDriverID               = "ini-file"
	JSONFileDriverID              = "json-file"
	YAMLFileDriverID              = "yaml-file"
	TOMLFileDriverID              = "toml-file"
	PlistFileDriverID             = "plist-file"
	MacOSDefaultsReadOnlyDriverID = "macos-defaults-readonly"
	localRecipeRelRoot            = "recipes/local"
)

const (
	ValidationSeverityError   = "error"
	ValidationSeverityWarning = "warning"
)

const (
	RecipeSourceBundled = "bundled"
	RecipeSourceLocal   = "local"
)

const (
	SensitivityLow          = "low"
	SensitivityPersonal     = "personal"
	SensitivityMachineLocal = "machine-local"
	SensitivitySecret       = "secret"
	SensitivityUnknown      = "unknown"
)

const (
	RedactionKnownSafe          = "known-safe"
	RedactionRedactedForDisplay = "redacted-for-display"
	RedactionBlockedSave        = "blocked-save"
	RedactionUnavailable        = "redaction-unavailable"
)

const (
	LifecycleAllowed               = "allowed"
	LifecycleWarn                  = "warn"
	LifecycleBlocked               = "blocked"
	LifecycleAskToQuit             = "ask-to-quit"
	LifecycleQuitIfRunning         = "quit-if-running"
	LifecycleBlockIfRunning        = "block-if-running"
	LifecycleReopenIfStoppedByTool = "reopen-if-stopped-by-tool"
)

var (
	publicIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*(?:[.-][a-z0-9][a-z0-9_-]*)*$`)
	recipeIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

type Recipe struct {
	Schema         string                   `yaml:"schema"`
	SchemaVersion  int                      `yaml:"schemaVersion"`
	Target         string                   `yaml:"target"`
	DisplayName    string                   `yaml:"displayName"`
	SupportLevel   string                   `yaml:"supportLevel"`
	Capability     string                   `yaml:"capability"`
	Locations      map[string]Location      `yaml:"locations"`
	SettingsGroups map[string]SettingsGroup `yaml:"settingsGroups,omitempty"`
	Settings       map[string]Setting       `yaml:"settings"`
	Resources      map[string]Resource      `yaml:"resources"`
}

type Location struct {
	Default string `yaml:"default"`
}

type SettingsGroup struct {
	Label        string   `yaml:"label,omitempty"`
	Description  string   `yaml:"description,omitempty"`
	SupportLevel string   `yaml:"supportLevel,omitempty"`
	Capability   string   `yaml:"capability,omitempty"`
	Settings     []string `yaml:"settings"`
}

type Setting struct {
	Label        string `yaml:"label,omitempty"`
	SupportLevel string `yaml:"supportLevel,omitempty"`
	Capability   string `yaml:"capability,omitempty"`
	ArtifactForm string `yaml:"artifactForm,omitempty"`
	Sensitivity  string `yaml:"sensitivity,omitempty"`
	Redaction    string `yaml:"redaction,omitempty"`
	Lifecycle    string `yaml:"lifecycle,omitempty"`
	ScopeDefault string `yaml:"scopeDefault"`
	Resource     string `yaml:"resource"`
}

type Resource struct {
	Driver      string    `yaml:"driver"`
	Location    string    `yaml:"location"`
	Path        string    `yaml:"path"`
	Capability  string    `yaml:"capability,omitempty"`
	Sensitivity string    `yaml:"sensitivity,omitempty"`
	Redaction   string    `yaml:"redaction,omitempty"`
	Lifecycle   string    `yaml:"lifecycle,omitempty"`
	Include     []string  `yaml:"include,omitempty"`
	Exclude     []string  `yaml:"exclude,omitempty"`
	Selector    *Selector `yaml:"selector,omitempty"`
}

type Selector struct {
	Section         string   `yaml:"section,omitempty"`
	Key             string   `yaml:"key,omitempty"`
	Path            []string `yaml:"path,omitempty"`
	MissingSection  string   `yaml:"missingSection,omitempty"`
	MissingKey      string   `yaml:"missingKey,omitempty"`
	CreateMissing   string   `yaml:"createMissing,omitempty"`
	DuplicatePolicy string   `yaml:"duplicatePolicy,omitempty"`
	DeleteKey       string   `yaml:"deleteKey,omitempty"`
}

type INISelector = Selector

type ValidationDiagnostic struct {
	Code     string `json:"code"`
	Path     string `json:"path"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type ValidationError struct {
	Diagnostics []ValidationDiagnostic `json:"diagnostics"`
}

type WriteSafetyContext struct {
	Source                  string
	Trusted                 bool
	AllowOpaque             bool
	AllowSensitive          bool
	AllowUnknownSensitivity bool
	HandlesLifecycleActions bool
	localTrustEvidence      *localTrustEvidence
}

func (e *ValidationError) Error() string {
	if e == nil || len(e.Diagnostics) == 0 {
		return "recipe validation failed"
	}
	parts := make([]string, 0, len(e.Diagnostics))
	for _, diagnostic := range e.Diagnostics {
		parts = append(parts, fmt.Sprintf("%s[%s]: %s", diagnostic.Path, diagnostic.Code, diagnostic.Message))
	}
	return strings.Join(parts, "; ")
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
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read recipe %s: %w", name, err)
	}
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return nil, fmt.Errorf("parse recipe %s: %w", name, err)
	}
	if err := validationError(duplicateYAMLKeyDiagnostics(&node)); err != nil {
		return nil, fmt.Errorf("validate recipe %s: %w", name, err)
	}

	var rec Recipe
	dec := yaml.NewDecoder(bytes.NewReader(data))
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
	return validationError(r.ValidationDiagnostics())
}

func (r *Recipe) ValidationDiagnostics() []ValidationDiagnostic {
	if r == nil {
		return []ValidationDiagnostic{validationDiagnostic("recipe.required", "$", "recipe is required")}
	}
	var diagnostics []ValidationDiagnostic
	add := func(code string, path string, message string) {
		diagnostics = append(diagnostics, validationDiagnostic(code, path, message))
	}
	addErr := func(code string, path string, err error) {
		if err != nil {
			add(code, path, err.Error())
		}
	}

	if r.Schema != Schema {
		add("schema.invalid", "$.schema", fmt.Sprintf("invalid recipe schema: %q (expected %q)", r.Schema, Schema))
	}
	if r.SchemaVersion != SupportedVersion {
		add("schemaVersion.invalid", "$.schemaVersion", fmt.Sprintf("invalid recipe schemaVersion: %d (expected %d)", r.SchemaVersion, SupportedVersion))
	}
	addErr("target.invalid", "$.target", ValidatePublicID("target", r.Target))
	if strings.TrimSpace(r.DisplayName) == "" {
		add("displayName.required", "$.displayName", "recipe displayName is required")
	}
	if !knownSupportLevel(r.SupportLevel) {
		add("supportLevel.unsupported", "$.supportLevel", fmt.Sprintf("unsupported supportLevel: %s", r.SupportLevel))
	}
	if !knownCapability(r.Capability) {
		add("capability.unsupported", "$.capability", fmt.Sprintf("unsupported capability: %s", r.Capability))
	}
	if len(r.Locations) == 0 {
		add("locations.required", "$.locations", "recipe must declare at least one location")
	}
	if len(r.Resources) == 0 {
		add("resources.required", "$.resources", "recipe must declare at least one resource")
	}
	if len(r.Settings) == 0 {
		add("settings.required", "$.settings", "recipe must declare at least one setting")
	}

	for _, locationID := range sortedKeys(r.Locations) {
		location := r.Locations[locationID]
		locationPath := "$.locations." + locationID
		addErr("location.id.invalid", locationPath, ValidatePublicID("location", locationID))
		if strings.TrimSpace(location.Default) == "" {
			add("location.default.required", locationPath+".default", fmt.Sprintf("location %s default is required", locationID))
		}
		if strings.ContainsRune(location.Default, '\x00') {
			add("location.default.nul", locationPath+".default", fmt.Sprintf("location %s default contains NUL", locationID))
		}
	}

	for _, resourceID := range sortedKeys(r.Resources) {
		resource := r.Resources[resourceID]
		resourcePath := "$.resources." + resourceID
		addErr("resource.id.invalid", resourcePath, ValidatePublicID("resource", resourceID))
		driverDeclared := true
		if resource.Driver == "" {
			add("resource.driver.required", resourcePath+".driver", fmt.Sprintf("resource %s driver is required", resourceID))
			driverDeclared = false
		}
		if resource.Location == "" {
			add("resource.location.required", resourcePath+".location", fmt.Sprintf("resource %s location is required", resourceID))
		} else if _, ok := r.Locations[resource.Location]; !ok {
			add("resource.location.unknown", resourcePath+".location", fmt.Sprintf("resource %s references unknown location %s", resourceID, resource.Location))
		}
		if _, err := ValidateResourcePath(resource.Path); err != nil {
			add("resource.path.invalid", resourcePath+".path", fmt.Sprintf("resource %s path: %s", resourceID, err.Error()))
		}
		if resource.Capability != "" && !knownCapability(resource.Capability) {
			add("resource.capability.unsupported", resourcePath+".capability", fmt.Sprintf("resource %s unsupported capability: %s", resourceID, resource.Capability))
		}
		if resource.Sensitivity != "" && !knownSensitivity(resource.Sensitivity) {
			add("resource.sensitivity.unsupported", resourcePath+".sensitivity", fmt.Sprintf("resource %s unsupported sensitivity classification", resourceID))
		}
		if resource.Redaction != "" && !knownRedaction(resource.Redaction) {
			add("resource.redaction.unsupported", resourcePath+".redaction", fmt.Sprintf("resource %s unsupported redaction policy", resourceID))
		}
		if resource.Lifecycle != "" && !knownLifecycle(resource.Lifecycle) {
			add("resource.lifecycle.unsupported", resourcePath+".lifecycle", fmt.Sprintf("resource %s unsupported lifecycle policy", resourceID))
		}
		if driverDeclared {
			addErr("resource.driver.invalid", resourcePath+".driver", r.validateResourceDriverShape(resourceID, resource))
		}
	}

	for _, settingID := range sortedKeys(r.Settings) {
		setting := r.Settings[settingID]
		settingPath := "$.settings." + settingID
		addErr("setting.id.invalid", settingPath, ValidatePublicID("setting", settingID))
		if strings.TrimSpace(setting.Label) != setting.Label {
			add("setting.label.invalid", settingPath+".label", fmt.Sprintf("setting %s label must not have surrounding whitespace", settingID))
		}
		if setting.SupportLevel != "" && !knownSupportLevel(setting.SupportLevel) {
			add("setting.supportLevel.unsupported", settingPath+".supportLevel", fmt.Sprintf("setting %s unsupported supportLevel: %s", settingID, setting.SupportLevel))
		}
		if setting.Capability != "" && !knownCapability(setting.Capability) {
			add("setting.capability.unsupported", settingPath+".capability", fmt.Sprintf("setting %s unsupported capability: %s", settingID, setting.Capability))
		}
		if setting.ArtifactForm != "" && !knownArtifactForm(setting.ArtifactForm) {
			add("setting.artifactForm.unsupported", settingPath+".artifactForm", fmt.Sprintf("setting %s unsupported artifactForm: %s", settingID, setting.ArtifactForm))
		}
		if setting.Sensitivity != "" && !knownSensitivity(setting.Sensitivity) {
			add("setting.sensitivity.unsupported", settingPath+".sensitivity", fmt.Sprintf("setting %s unsupported sensitivity classification", settingID))
		}
		if setting.Redaction != "" && !knownRedaction(setting.Redaction) {
			add("setting.redaction.unsupported", settingPath+".redaction", fmt.Sprintf("setting %s unsupported redaction policy", settingID))
		}
		if setting.Lifecycle != "" && !knownLifecycle(setting.Lifecycle) {
			add("setting.lifecycle.unsupported", settingPath+".lifecycle", fmt.Sprintf("setting %s unsupported lifecycle policy", settingID))
		}
		if setting.ScopeDefault != "" && !knownScope(setting.ScopeDefault) {
			add("setting.scopeDefault.unsupported", settingPath+".scopeDefault", fmt.Sprintf("setting %s unsupported scopeDefault: %s", settingID, setting.ScopeDefault))
		}
		if setting.Resource == "" {
			add("setting.resource.required", settingPath+".resource", fmt.Sprintf("setting %s resource is required", settingID))
		} else if _, ok := r.Resources[setting.Resource]; !ok {
			add("setting.resource.unknown", settingPath+".resource", fmt.Sprintf("setting %s references unknown resource %s", settingID, setting.Resource))
		}
	}

	for _, groupID := range sortedKeys(r.SettingsGroups) {
		group := r.SettingsGroups[groupID]
		groupPath := "$.settingsGroups." + groupID
		addErr("settingsGroup.id.invalid", groupPath, ValidatePublicID("settingsGroup", groupID))
		if strings.TrimSpace(group.Label) != group.Label {
			add("settingsGroup.label.invalid", groupPath+".label", fmt.Sprintf("settingsGroup %s label must not have surrounding whitespace", groupID))
		}
		if group.SupportLevel != "" && !knownSupportLevel(group.SupportLevel) {
			add("settingsGroup.supportLevel.unsupported", groupPath+".supportLevel", fmt.Sprintf("settingsGroup %s unsupported supportLevel: %s", groupID, group.SupportLevel))
		}
		if group.Capability != "" && !knownCapability(group.Capability) {
			add("settingsGroup.capability.unsupported", groupPath+".capability", fmt.Sprintf("settingsGroup %s unsupported capability: %s", groupID, group.Capability))
		}
		if len(group.Settings) == 0 {
			add("settingsGroup.settings.required", groupPath+".settings", fmt.Sprintf("settingsGroup %s must reference at least one setting", groupID))
		}
		seen := map[string]bool{}
		for idx, settingRef := range group.Settings {
			settingPath := fmt.Sprintf("%s.settings[%d]", groupPath, idx)
			if err := ValidatePublicID("setting", settingRef); err != nil {
				add("settingsGroup.setting.invalid", settingPath, fmt.Sprintf("settingsGroup %s invalid setting ref %s", groupID, settingRef))
				continue
			}
			if seen[settingRef] {
				add("settingsGroup.setting.duplicate", settingPath, fmt.Sprintf("settingsGroup %s duplicates setting ref %s", groupID, settingRef))
			}
			seen[settingRef] = true
			if _, ok := r.Settings[settingRef]; !ok {
				add("settingsGroup.setting.unknown", settingPath, fmt.Sprintf("settingsGroup %s references unknown setting %s", groupID, settingRef))
			}
		}
	}

	return normalizeDiagnostics(diagnostics)
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

func (r *Recipe) ValidateStarship() error {
	if err := r.Validate(); err != nil {
		return err
	}
	if r.Target != StarshipTarget {
		return fmt.Errorf("starship recipe target must be %q, got %q", StarshipTarget, r.Target)
	}
	if r.Capability != "read-write" {
		return fmt.Errorf("starship recipe capability must be read-write, got %s", r.Capability)
	}
	if len(r.Locations) != 1 {
		return fmt.Errorf("starship recipe must declare only the config location")
	}
	location, ok := r.Locations["config"]
	if !ok {
		return fmt.Errorf("starship recipe must declare config location")
	}
	if strings.TrimSpace(location.Default) != "~/.config" {
		return fmt.Errorf("starship config location default must be ~/.config")
	}
	if len(r.Settings) != len(starshipSettingIDs()) {
		return fmt.Errorf("starship recipe must declare only supported prompt-wide settings")
	}
	if len(r.Resources) != len(starshipSettingIDs()) {
		return fmt.Errorf("starship recipe must declare exactly one selected TOML resource per supported setting")
	}
	for _, settingID := range starshipSettingIDs() {
		if err := r.validateStarshipSetting(settingID); err != nil {
			return err
		}
	}
	return nil
}

func (r *Recipe) validateStarshipSetting(settingID string) error {
	setting, ok := r.Settings[settingID]
	if !ok {
		return fmt.Errorf("starship recipe missing setting %s", settingID)
	}
	if setting.ScopeDefault != "user" {
		return fmt.Errorf("starship setting %s scopeDefault must be user", settingID)
	}
	if setting.SupportLevel != "experimental" {
		return fmt.Errorf("starship setting %s supportLevel must be experimental", settingID)
	}
	if setting.Capability != "read-write" {
		return fmt.Errorf("starship setting %s capability must be read-write", settingID)
	}
	if setting.ArtifactForm != "scalar" {
		return fmt.Errorf("starship setting %s artifactForm must be scalar", settingID)
	}
	if setting.Sensitivity != SensitivityLow {
		return fmt.Errorf("starship setting %s sensitivity must be %s", settingID, SensitivityLow)
	}
	if setting.Redaction != RedactionKnownSafe {
		return fmt.Errorf("starship setting %s redaction must be %s", settingID, RedactionKnownSafe)
	}
	if setting.Lifecycle != LifecycleAllowed {
		return fmt.Errorf("starship setting %s lifecycle must be %s", settingID, LifecycleAllowed)
	}
	if setting.Resource != settingID {
		return fmt.Errorf("starship setting %s resource must be %s", settingID, settingID)
	}
	resource, ok := r.Resources[setting.Resource]
	if !ok {
		return fmt.Errorf("starship setting %s references unknown resource %s", settingID, setting.Resource)
	}
	if resource.Driver != TOMLFileDriverID {
		return fmt.Errorf("starship setting %s driver must be %q, got %q", settingID, TOMLFileDriverID, resource.Driver)
	}
	if resource.Location != "config" {
		return fmt.Errorf("starship setting %s location must be config", settingID)
	}
	if resource.Path != "starship.toml" {
		return fmt.Errorf("starship setting %s path must be starship.toml", settingID)
	}
	if resource.Capability != "read-write" {
		return fmt.Errorf("starship resource %s capability must be read-write", settingID)
	}
	if resource.Sensitivity != SensitivityLow {
		return fmt.Errorf("starship resource %s sensitivity must be %s", settingID, SensitivityLow)
	}
	if resource.Redaction != RedactionKnownSafe {
		return fmt.Errorf("starship resource %s redaction must be %s", settingID, RedactionKnownSafe)
	}
	if resource.Lifecycle != LifecycleAllowed {
		return fmt.Errorf("starship resource %s lifecycle must be %s", settingID, LifecycleAllowed)
	}
	if resource.Selector == nil {
		return fmt.Errorf("starship setting %s requires a TOML selector", settingID)
	}
	if len(resource.Selector.Path) != 1 || resource.Selector.Path[0] != settingID {
		return fmt.Errorf("starship setting %s must select root TOML key %s", settingID, settingID)
	}
	if selectorCreatePolicy(resource.Selector) != "create" {
		return fmt.Errorf("starship setting %s createMissing must be create", settingID)
	}
	if selectorDuplicatePolicy(resource.Selector) != "reject" {
		return fmt.Errorf("starship setting %s duplicatePolicy must be reject", settingID)
	}
	if selectorDeleteKey(resource.Selector) != "allow" {
		return fmt.Errorf("starship setting %s deleteKey must be allow", settingID)
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

func starshipSettingIDs() []string {
	return []string{"add_newline", "command_timeout", "follow_symlinks", "scan_timeout"}
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
	case JSONFileDriverID, YAMLFileDriverID, TOMLFileDriverID, PlistFileDriverID:
		return validateSelectedPathResource(resourceID, resource)
	case MacOSDefaultsReadOnlyDriverID:
		return r.validateMacOSDefaultsReadOnlyResource(resourceID, resource)
	case FileDriverID, FileTreeDriverID:
		if resource.Selector != nil {
			return fmt.Errorf("resource %s driver %q must not declare selector", resourceID, resource.Driver)
		}
	default:
		return fmt.Errorf("resource %s unsupported driver %q", resourceID, resource.Driver)
	}
	return nil
}

func (r *Recipe) validateMacOSDefaultsReadOnlyResource(resourceID string, resource Resource) error {
	if len(resource.Include) > 0 || len(resource.Exclude) > 0 {
		return fmt.Errorf("resource %s driver %q must not declare include/exclude globs", resourceID, MacOSDefaultsReadOnlyDriverID)
	}
	if resource.Location != macosdefaultsdriver.LogicalLocationID {
		return fmt.Errorf("resource %s driver %q must use location %q", resourceID, MacOSDefaultsReadOnlyDriverID, macosdefaultsdriver.LogicalLocationID)
	}
	location, ok := r.Locations[resource.Location]
	if !ok {
		return fmt.Errorf("resource %s driver %q references missing synthetic location %q", resourceID, MacOSDefaultsReadOnlyDriverID, macosdefaultsdriver.LogicalLocationID)
	}
	if location.Default != macosdefaultsdriver.LogicalLocationURI {
		return fmt.Errorf("resource %s driver %q location %q default must be %q", resourceID, MacOSDefaultsReadOnlyDriverID, macosdefaultsdriver.LogicalLocationID, macosdefaultsdriver.LogicalLocationURI)
	}
	if resource.Capability != "read-only" {
		return fmt.Errorf("resource %s driver %q capability must be read-only", resourceID, MacOSDefaultsReadOnlyDriverID)
	}
	if err := macosdefaultsdriver.ValidateDomain(resource.Path); err != nil {
		return fmt.Errorf("resource %s defaults domain: %w", resourceID, err)
	}
	if resource.Selector == nil {
		return fmt.Errorf("resource %s driver %q requires selector", resourceID, MacOSDefaultsReadOnlyDriverID)
	}
	selector := resource.Selector
	if selector.Section != "" || len(selector.Path) > 0 || selector.MissingSection != "" || selector.MissingKey != "" || selector.CreateMissing != "" || selector.DuplicatePolicy != "" || selector.DeleteKey != "" {
		return fmt.Errorf("resource %s driver %q supports only selector.key and must not declare file selector policy fields", resourceID, MacOSDefaultsReadOnlyDriverID)
	}
	if err := macosdefaultsdriver.ValidateKey(selector.Key); err != nil {
		return fmt.Errorf("resource %s selector key: %w", resourceID, err)
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
	if len(selector.Path) > 0 || selector.CreateMissing != "" {
		return fmt.Errorf("resource %s driver %q must not declare selected-path selector fields", resourceID, IniFileDriverID)
	}
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

func validateSelectedPathResource(resourceID string, resource Resource) error {
	if len(resource.Include) > 0 || len(resource.Exclude) > 0 {
		return fmt.Errorf("resource %s driver %q must not declare include/exclude globs", resourceID, resource.Driver)
	}
	if resource.Selector == nil {
		return fmt.Errorf("resource %s driver %q requires selector", resourceID, resource.Driver)
	}
	selector := resource.Selector
	if selector.Section != "" || selector.Key != "" || selector.MissingSection != "" || selector.MissingKey != "" {
		return fmt.Errorf("resource %s driver %q must not declare INI selector fields", resourceID, resource.Driver)
	}
	if len(selector.Path) == 0 {
		return fmt.Errorf("resource %s selector path is required", resourceID)
	}
	for idx, segment := range selector.Path {
		if segment == "" {
			return fmt.Errorf("resource %s selector path segment %d is required", resourceID, idx)
		}
		if strings.ContainsAny(segment, "\r\n\x00") {
			return fmt.Errorf("resource %s selector path segment %d must not contain CR, LF, or NUL", resourceID, idx)
		}
		if isExpressionSegment(segment) {
			return fmt.Errorf("resource %s selector path segment %d looks like an expression", resourceID, idx)
		}
	}
	switch selectorCreatePolicy(selector) {
	case "reject", "create":
	default:
		return fmt.Errorf("resource %s unsupported selector createMissing policy %q", resourceID, selector.CreateMissing)
	}
	switch selectorDuplicatePolicy(selector) {
	case "reject":
	default:
		return fmt.Errorf("resource %s unsupported selector duplicatePolicy %q", resourceID, selector.DuplicatePolicy)
	}
	switch selectorDeleteKey(selector) {
	case "reject", "allow":
	default:
		return fmt.Errorf("resource %s unsupported selector deleteKey policy %q", resourceID, selector.DeleteKey)
	}
	return nil
}

func selectorMissingSection(selector *Selector) string {
	if selector == nil || selector.MissingSection == "" {
		return string(inidriver.MissingPolicyError)
	}
	return selector.MissingSection
}

func selectorMissingKey(selector *Selector) string {
	if selector == nil || selector.MissingKey == "" {
		return string(inidriver.MissingPolicyError)
	}
	return selector.MissingKey
}

func selectorCreatePolicy(selector *Selector) string {
	if selector == nil || selector.CreateMissing == "" {
		return "reject"
	}
	return selector.CreateMissing
}

func selectorDuplicatePolicy(selector *Selector) string {
	if selector == nil || selector.DuplicatePolicy == "" {
		return string(inidriver.DuplicatePolicyReject)
	}
	return selector.DuplicatePolicy
}

func selectorDeleteKey(selector *Selector) string {
	if selector == nil || selector.DeleteKey == "" {
		return string(inidriver.DeletePolicyReject)
	}
	return selector.DeleteKey
}

func isExpressionSegment(segment string) bool {
	return segment == "*" || segment == "$" || segment == "." || segment == ".." || strings.ContainsAny(segment, "[]") || strings.HasPrefix(segment, "$.")
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

func ValidationDiagnostics(err error) []ValidationDiagnostic {
	var validationErr *ValidationError
	if errors.As(err, &validationErr) {
		return append([]ValidationDiagnostic(nil), validationErr.Diagnostics...)
	}
	return nil
}

func validationError(diagnostics []ValidationDiagnostic) error {
	diagnostics = normalizeDiagnostics(diagnostics)
	if len(diagnostics) == 0 {
		return nil
	}
	return &ValidationError{Diagnostics: diagnostics}
}

func validationDiagnostic(code string, path string, message string) ValidationDiagnostic {
	return ValidationDiagnostic{Code: code, Path: path, Severity: ValidationSeverityError, Message: message}
}

func duplicateYAMLKeyDiagnostics(node *yaml.Node) []ValidationDiagnostic {
	var diagnostics []ValidationDiagnostic
	var walk func(*yaml.Node, string)
	walk = func(current *yaml.Node, path string) {
		if current == nil {
			return
		}
		switch current.Kind {
		case yaml.DocumentNode:
			if len(current.Content) > 0 {
				walk(current.Content[0], path)
			}
		case yaml.MappingNode:
			seen := map[string]string{}
			for idx := 0; idx+1 < len(current.Content); idx += 2 {
				keyNode := current.Content[idx]
				valueNode := current.Content[idx+1]
				key := keyNode.Value
				if key == "" {
					key = fmt.Sprintf("<key-%d>", idx/2)
				}
				keyPath := path + "." + key
				if firstPath, ok := seen[key]; ok {
					diagnostics = append(diagnostics, validationDiagnostic("yaml.duplicate-key", keyPath, fmt.Sprintf("duplicate mapping key %q (first declared at %s)", key, firstPath)))
				} else {
					seen[key] = keyPath
				}
				walk(valueNode, keyPath)
			}
		case yaml.SequenceNode:
			for idx, child := range current.Content {
				walk(child, fmt.Sprintf("%s[%d]", path, idx))
			}
		}
	}
	walk(node, "$")
	return normalizeDiagnostics(diagnostics)
}

func normalizeDiagnostics(diagnostics []ValidationDiagnostic) []ValidationDiagnostic {
	out := append([]ValidationDiagnostic(nil), diagnostics...)
	for idx := range out {
		if out[idx].Severity == "" {
			out[idx].Severity = ValidationSeverityError
		}
		if out[idx].Path == "" {
			out[idx].Path = "$"
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		if out[i].Code != out[j].Code {
			return out[i].Code < out[j].Code
		}
		return out[i].Message < out[j].Message
	})
	return out
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

func knownArtifactForm(value string) bool {
	switch value {
	case "file", "file-tree", "scalar", "structured", "native-export", "opaque", "metadata-only":
		return true
	default:
		return false
	}
}

func knownSensitivity(value string) bool {
	switch value {
	case SensitivityLow, SensitivityPersonal, SensitivityMachineLocal, SensitivitySecret, SensitivityUnknown:
		return true
	default:
		return false
	}
}

func knownRedaction(value string) bool {
	switch value {
	case RedactionKnownSafe, RedactionRedactedForDisplay, RedactionBlockedSave, RedactionUnavailable:
		return true
	default:
		return false
	}
}

func knownLifecycle(value string) bool {
	switch value {
	case LifecycleAllowed, LifecycleWarn, LifecycleBlocked, LifecycleAskToQuit, LifecycleQuitIfRunning, LifecycleBlockIfRunning, LifecycleReopenIfStoppedByTool:
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
