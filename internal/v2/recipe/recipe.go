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

	"github.com/shpoont/dotfiles-manager/internal/v2/contentsafety"
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
	NvimTarget                    = "nvim"
	SSHTarget                     = "ssh"
	StarshipTarget                = "starship"
	TmuxTarget                    = "tmux"
	ZshTarget                     = "zsh"
	FileDriverID                  = "file"
	FileTreeDriverID              = "file-tree"
	NativeExportDriverID          = "native-export"
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
	MaxNativeOperationTimeoutSeconds = 300
	MaxNativeCaptureBytes            = 64 * 1024
	MaxNativeExportBytes             = 50 * 1024 * 1024
	MaxNativeExportEntries           = 10000
	maxNativeOperationTimeoutSeconds = MaxNativeOperationTimeoutSeconds
	maxNativeCaptureBytes            = MaxNativeCaptureBytes
	maxNativeExportBytes             = MaxNativeExportBytes
	maxNativeExportEntries           = MaxNativeExportEntries
	maxNativeExpectedExitCodes       = 16
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

const (
	LifecycleDetectProcessName  = "process-name"
	LifecycleControlUnsupported = "unsupported"
	LifecycleControlManaged     = "managed"
	LifecycleReopenNone         = "none"
)

const (
	ZshRiskShellStartupFileCode     = "zsh.risk.shell-startup-file"
	ZshBlockedZshenvCode            = "zsh.blocked.zshenv"
	ZshBlockedHistoryCode           = "zsh.blocked.history"
	ZshBlockedCompletionCacheCode   = "zsh.blocked.completion-cache"
	ZshBlockedPluginStateCode       = "zsh.blocked.plugin-state"
	ZshBlockedSessionStateCode      = "zsh.blocked.session-state"
	TmuxManualReloadWarningCode     = "tmux.lifecycle.manual-reload"
	SSHConfigReviewWarningCode      = "ssh.config.review-required"
	SSHConfigSymlinkUnsupportedCode = "ssh.config.symlink-unsupported"
	SSHConfigExcludedContentCode    = "ssh.config.excluded-content"
	SSHRefExcludedCode              = "ssh.ref.excluded"
	SSHContentSafetyPolicy          = contentsafety.PolicySSHConfigObviousSecrets
)

var (
	publicIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*(?:[.-][a-z0-9][a-z0-9_-]*)*$`)
	recipeIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

type Recipe struct {
	Schema           string                     `yaml:"schema"`
	SchemaVersion    int                        `yaml:"schemaVersion"`
	Target           string                     `yaml:"target"`
	DisplayName      string                     `yaml:"displayName"`
	SupportLevel     string                     `yaml:"supportLevel"`
	Capability       string                     `yaml:"capability"`
	Locations        map[string]Location        `yaml:"locations"`
	LifecycleTargets map[string]LifecycleTarget `yaml:"lifecycleTargets,omitempty"`
	SettingsGroups   map[string]SettingsGroup   `yaml:"settingsGroups,omitempty"`
	Settings         map[string]Setting         `yaml:"settings"`
	Resources        map[string]Resource        `yaml:"resources"`
	NativeOperations map[string]NativeOperation `yaml:"nativeOperations,omitempty"`
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
	Label           string          `yaml:"label,omitempty"`
	SupportLevel    string          `yaml:"supportLevel,omitempty"`
	Capability      string          `yaml:"capability,omitempty"`
	ArtifactForm    string          `yaml:"artifactForm,omitempty"`
	Sensitivity     string          `yaml:"sensitivity,omitempty"`
	Redaction       string          `yaml:"redaction,omitempty"`
	Lifecycle       string          `yaml:"lifecycle,omitempty"`
	LifecycleTarget string          `yaml:"lifecycleTarget,omitempty"`
	WriteWarnings   []ReviewWarning `yaml:"writeWarnings,omitempty"`
	ScopeDefault    string          `yaml:"scopeDefault"`
	Resource        string          `yaml:"resource"`
}

type Resource struct {
	Driver                string            `yaml:"driver"`
	Location              string            `yaml:"location"`
	Path                  string            `yaml:"path"`
	NativeOperation       string            `yaml:"nativeOperation,omitempty"`
	NativeImportOperation string            `yaml:"nativeImportOperation,omitempty"`
	NativeVerifyOperation string            `yaml:"nativeVerifyOperation,omitempty"`
	NativeApply           NativeApplyPolicy `yaml:"nativeApply,omitempty"`
	Capability            string            `yaml:"capability,omitempty"`
	Sensitivity           string            `yaml:"sensitivity,omitempty"`
	Redaction             string            `yaml:"redaction,omitempty"`
	Lifecycle             string            `yaml:"lifecycle,omitempty"`
	LifecycleTarget       string            `yaml:"lifecycleTarget,omitempty"`
	ContentSafetyPolicy   string            `yaml:"contentSafetyPolicy,omitempty"`
	WriteWarnings         []ReviewWarning   `yaml:"writeWarnings,omitempty"`
	Include               []string          `yaml:"include,omitempty"`
	Exclude               []string          `yaml:"exclude,omitempty"`
	Selector              *Selector         `yaml:"selector,omitempty"`
}

type NativeOperation struct {
	Kind              string                     `yaml:"kind"`
	Reviewed          bool                       `yaml:"reviewed"`
	Runner            string                     `yaml:"runner"`
	Platforms         []string                   `yaml:"platforms"`
	ArtifactForm      string                     `yaml:"artifactForm"`
	DiffMode          string                     `yaml:"diffMode"`
	Lifecycle         string                     `yaml:"lifecycle"`
	WorkingDirectory  string                     `yaml:"workingDirectory"`
	TimeoutSeconds    int                        `yaml:"timeoutSeconds"`
	ExpectedExitCodes []int                      `yaml:"expectedExitCodes"`
	Command           NativeCommand              `yaml:"command"`
	LifecycleTarget   string                     `yaml:"lifecycleTarget,omitempty"`
	Stdin             NativeStdinPolicy          `yaml:"stdin"`
	Stdout            NativeStreamPolicy         `yaml:"stdout"`
	Stderr            NativeStreamPolicy         `yaml:"stderr"`
	Env               map[string]NativeEnvValue  `yaml:"env,omitempty"`
	Inputs            map[string]NativePathSpec  `yaml:"inputs,omitempty"`
	Outputs           map[string]NativePathSpec  `yaml:"outputs,omitempty"`
	TempPaths         map[string]NativePathSpec  `yaml:"tempPaths,omitempty"`
	Redaction         string                     `yaml:"redaction"`
	Review            NativeReviewPolicy         `yaml:"review,omitempty"`
	Limits            NativeExportLimits         `yaml:"limits,omitempty"`
	ExportMetadata    NativeExportMetadataPolicy `yaml:"exportMetadata,omitempty"`
}

type NativeCommand struct {
	Executable string      `yaml:"executable"`
	Args       []NativeArg `yaml:"args,omitempty"`
}

type NativeArg struct {
	Literal string `yaml:"literal,omitempty"`
	Input   string `yaml:"input,omitempty"`
	Output  string `yaml:"output,omitempty"`
	Temp    string `yaml:"temp,omitempty"`
}

type NativeEnvValue struct {
	Literal string `yaml:"literal,omitempty"`
	Input   string `yaml:"input,omitempty"`
	Output  string `yaml:"output,omitempty"`
	Temp    string `yaml:"temp,omitempty"`
}

type NativePathSpec struct {
	Root     string `yaml:"root"`
	Location string `yaml:"location,omitempty"`
	Path     string `yaml:"path"`
}

type NativeStdinPolicy struct {
	Mode string `yaml:"mode"`
}

type NativeStreamPolicy struct {
	Mode     string `yaml:"mode"`
	MaxBytes int    `yaml:"maxBytes,omitempty"`
}

type NativeReviewPolicy struct {
	Required bool     `yaml:"required,omitempty"`
	Reasons  []string `yaml:"reasons,omitempty"`
	Message  string   `yaml:"message,omitempty"`
}

type NativeExportLimits struct {
	MaxBytes   int64 `yaml:"maxBytes,omitempty"`
	MaxEntries int   `yaml:"maxEntries,omitempty"`
}

type NativeExportMetadataPolicy struct {
	CapturedCategories []string `yaml:"capturedCategories,omitempty"`
	SecretExclusions   []string `yaml:"secretExclusions,omitempty"`
	AccountExclusions  []string `yaml:"accountExclusions,omitempty"`
	Limitations        []string `yaml:"limitations,omitempty"`
}

type NativeApplyPolicy struct {
	Backup string `yaml:"backup,omitempty"`
	Verify string `yaml:"verify,omitempty"`
}

type LifecycleTarget struct {
	DisplayName string                 `yaml:"displayName,omitempty"`
	Detect      LifecycleDetectPolicy  `yaml:"detect"`
	Quit        LifecycleControlPolicy `yaml:"quit,omitempty"`
	Reopen      LifecycleControlPolicy `yaml:"reopen,omitempty"`
}

type LifecycleDetectPolicy struct {
	Kind  string   `yaml:"kind"`
	Names []string `yaml:"names,omitempty"`
}

type LifecycleControlPolicy struct {
	Kind string `yaml:"kind"`
}

type ReviewWarning struct {
	Code     string   `yaml:"code"`
	Triggers []string `yaml:"triggers"`
	Message  string   `yaml:"message,omitempty"`
}

type Selector struct {
	Section         string   `yaml:"section,omitempty" json:"section,omitempty"`
	Key             string   `yaml:"key,omitempty" json:"key,omitempty"`
	Path            []string `yaml:"path,omitempty" json:"path,omitempty"`
	MissingSection  string   `yaml:"missingSection,omitempty" json:"missingSection,omitempty"`
	MissingKey      string   `yaml:"missingKey,omitempty" json:"missingKey,omitempty"`
	CreateMissing   string   `yaml:"createMissing,omitempty" json:"createMissing,omitempty"`
	DuplicatePolicy string   `yaml:"duplicatePolicy,omitempty" json:"duplicatePolicy,omitempty"`
	DeleteKey       string   `yaml:"deleteKey,omitempty" json:"deleteKey,omitempty"`
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
	defer func() { _ = file.Close() }()
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
	if len(r.Locations) == 0 && recipeNeedsLocations(r) {
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
		if resource.Driver == NativeExportDriverID {
			if resource.Location != "" {
				add("resource.location.unexpected", resourcePath+".location", fmt.Sprintf("resource %s driver %q must not declare location", resourceID, NativeExportDriverID))
			}
			if resource.Path != "" {
				add("resource.path.unexpected", resourcePath+".path", fmt.Sprintf("resource %s driver %q must not declare path", resourceID, NativeExportDriverID))
			}
		} else if resource.Location == "" {
			add("resource.location.required", resourcePath+".location", fmt.Sprintf("resource %s location is required", resourceID))
		} else if _, ok := r.Locations[resource.Location]; !ok {
			add("resource.location.unknown", resourcePath+".location", fmt.Sprintf("resource %s references unknown location %s", resourceID, resource.Location))
		}
		if resource.Driver != NativeExportDriverID {
			if _, err := ValidateResourcePath(resource.Path); err != nil {
				add("resource.path.invalid", resourcePath+".path", fmt.Sprintf("resource %s path: %s", resourceID, err.Error()))
			}
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
		if resource.LifecycleTarget != "" {
			if _, ok := r.LifecycleTargets[resource.LifecycleTarget]; !ok {
				add("resource.lifecycleTarget.unknown", resourcePath+".lifecycleTarget", fmt.Sprintf("resource %s references unknown lifecycleTarget %s", resourceID, resource.LifecycleTarget))
			}
		}
		if resource.ContentSafetyPolicy != "" && !knownContentSafetyPolicy(resource.ContentSafetyPolicy) {
			add("resource.contentSafetyPolicy.unsupported", resourcePath+".contentSafetyPolicy", fmt.Sprintf("resource %s unsupported content safety policy", resourceID))
		}
		if resource.ContentSafetyPolicy != "" && resource.Driver != FileDriverID {
			add("resource.contentSafetyPolicy.driverUnsupported", resourcePath+".contentSafetyPolicy", fmt.Sprintf("resource %s content safety policy is supported only for file resources", resourceID))
		}
		for idx, warning := range resource.WriteWarnings {
			addReviewWarningDiagnostics(add, resourcePath+fmt.Sprintf(".writeWarnings[%d]", idx), "resource "+resourceID, warning)
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
		if setting.LifecycleTarget != "" {
			if _, ok := r.LifecycleTargets[setting.LifecycleTarget]; !ok {
				add("setting.lifecycleTarget.unknown", settingPath+".lifecycleTarget", fmt.Sprintf("setting %s references unknown lifecycleTarget %s", settingID, setting.LifecycleTarget))
			}
		}
		for idx, warning := range setting.WriteWarnings {
			addReviewWarningDiagnostics(add, settingPath+fmt.Sprintf(".writeWarnings[%d]", idx), "setting "+settingID, warning)
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

	for _, targetID := range sortedKeys(r.LifecycleTargets) {
		target := r.LifecycleTargets[targetID]
		targetPath := "$.lifecycleTargets." + targetID
		addErr("lifecycleTarget.id.invalid", targetPath, ValidatePublicID("lifecycleTarget", targetID))
		addLifecycleTargetDiagnostics(add, targetPath, targetID, target)
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

	for _, operationID := range sortedKeys(r.NativeOperations) {
		operation := r.NativeOperations[operationID]
		addNativeOperationDiagnostics(add, operationID, operation)
		addNativeOperationLocationDiagnostics(add, "$.nativeOperations."+operationID, operationID, operation, r.Locations)
		if operation.LifecycleTarget != "" {
			if _, ok := r.LifecycleTargets[operation.LifecycleTarget]; !ok {
				add("nativeOperation.lifecycleTarget.unknown", "$.nativeOperations."+operationID+".lifecycleTarget", fmt.Sprintf("native operation %s references unknown lifecycleTarget %s", operationID, operation.LifecycleTarget))
			}
		}
	}

	for _, resourceID := range sortedKeys(r.Resources) {
		resource := r.Resources[resourceID]
		if resource.Driver != NativeExportDriverID {
			continue
		}
		resourcePath := "$.resources." + resourceID
		operationID := strings.TrimSpace(resource.NativeOperation)
		if operationID == "" {
			add("resource.nativeOperation.required", resourcePath+".nativeOperation", fmt.Sprintf("resource %s driver %q requires nativeOperation", resourceID, NativeExportDriverID))
		} else if operation, ok := r.NativeOperations[operationID]; !ok {
			add("resource.nativeOperation.unknown", resourcePath+".nativeOperation", fmt.Sprintf("resource %s references unknown native operation %s", resourceID, operationID))
		} else if operation.Kind != "export" {
			add("resource.nativeOperation.kindUnsupported", resourcePath+".nativeOperation", fmt.Sprintf("resource %s nativeOperation %s must be kind export", resourceID, operationID))
		}
		importOperationID := strings.TrimSpace(resource.NativeImportOperation)
		if importOperationID != "" {
			if operation, ok := r.NativeOperations[importOperationID]; !ok {
				add("resource.nativeImportOperation.unknown", resourcePath+".nativeImportOperation", fmt.Sprintf("resource %s references unknown native import operation %s", resourceID, importOperationID))
			} else if operation.Kind != "import" {
				add("resource.nativeImportOperation.kindUnsupported", resourcePath+".nativeImportOperation", fmt.Sprintf("resource %s nativeImportOperation %s must be kind import", resourceID, importOperationID))
			}
		}
		verifyOperationID := strings.TrimSpace(resource.NativeVerifyOperation)
		if verifyOperationID != "" {
			if operation, ok := r.NativeOperations[verifyOperationID]; !ok {
				add("resource.nativeVerifyOperation.unknown", resourcePath+".nativeVerifyOperation", fmt.Sprintf("resource %s references unknown native verify operation %s", resourceID, verifyOperationID))
			} else if operation.Kind != "verify" {
				add("resource.nativeVerifyOperation.kindUnsupported", resourcePath+".nativeVerifyOperation", fmt.Sprintf("resource %s nativeVerifyOperation %s must be kind verify", resourceID, verifyOperationID))
			}
		}
		importCapable := importOperationID != "" || resource.NativeApply.Backup != "" || resource.NativeApply.Verify != ""
		if importCapable {
			if importOperationID == "" {
				add("resource.nativeImportOperation.required", resourcePath+".nativeImportOperation", fmt.Sprintf("resource %s native apply requires nativeImportOperation", resourceID))
			}
			if resource.Capability != "read-write" {
				add("resource.capability.nativeImportUnsupported", resourcePath+".capability", fmt.Sprintf("resource %s import-capable native resource requires capability read-write", resourceID))
			}
			if resource.NativeApply.Backup != "pre-apply-export" {
				add("resource.nativeApply.backup.unsupported", resourcePath+".nativeApply.backup", fmt.Sprintf("resource %s native apply backup policy must be pre-apply-export", resourceID))
			}
			if resource.NativeApply.Verify != "post-import-export-hash" {
				add("resource.nativeApply.verify.unsupported", resourcePath+".nativeApply.verify", fmt.Sprintf("resource %s native apply verify policy must be post-import-export-hash", resourceID))
			}
		} else if resource.Capability != "export-only" {
			add("resource.capability.nativeExportUnsupported", resourcePath+".capability", fmt.Sprintf("resource %s export-only native resource requires capability export-only", resourceID))
		}
	}

	for _, settingID := range sortedKeys(r.Settings) {
		setting := r.Settings[settingID]
		resource, ok := r.Resources[setting.Resource]
		if !ok || resource.Driver != NativeExportDriverID {
			continue
		}
		settingPath := "$.settings." + settingID
		importCapable := strings.TrimSpace(resource.NativeImportOperation) != "" || resource.NativeApply.Backup != "" || resource.NativeApply.Verify != ""
		if importCapable {
			if setting.Capability != "read-write" {
				add("setting.capability.nativeImportUnsupported", settingPath+".capability", fmt.Sprintf("setting %s import-capable native resource requires capability read-write", settingID))
			}
		} else if setting.Capability != "export-only" {
			add("setting.capability.nativeExportUnsupported", settingPath+".capability", fmt.Sprintf("setting %s export-only native resource requires capability export-only", settingID))
		}
		if setting.ArtifactForm != "native-export" && setting.ArtifactForm != "opaque" {
			add("setting.artifactForm.nativeExportUnsupported", settingPath+".artifactForm", fmt.Sprintf("setting %s native-export resource requires artifactForm native-export or opaque", settingID))
		}
	}

	addLifecyclePolicyReferenceDiagnostics(add, r)

	return normalizeDiagnostics(diagnostics)
}

func addNativeOperationDiagnostics(add func(string, string, string), operationID string, operation NativeOperation) {
	operationPath := "$.nativeOperations." + operationID
	if err := ValidatePublicID("nativeOperation", operationID); err != nil {
		add("nativeOperation.id.invalid", operationPath, err.Error())
	}
	if !knownNativeOperationKind(operation.Kind) {
		add("nativeOperation.kind.unsupported", operationPath+".kind", fmt.Sprintf("native operation %s unsupported kind: %s", operationID, operation.Kind))
	}
	if !operation.Reviewed {
		add("nativeOperation.reviewed.required", operationPath+".reviewed", fmt.Sprintf("native operation %s must be reviewed before execution", operationID))
	}
	if operation.Runner != "command" {
		add("nativeOperation.runner.unsupported", operationPath+".runner", fmt.Sprintf("native operation %s unsupported runner: %s", operationID, operation.Runner))
	}
	if len(operation.Platforms) == 0 {
		add("nativeOperation.platforms.required", operationPath+".platforms", fmt.Sprintf("native operation %s must declare supported platforms", operationID))
	}
	for idx, platform := range operation.Platforms {
		if !knownNativePlatform(platform) {
			add("nativeOperation.platform.unsupported", operationPath+fmt.Sprintf(".platforms[%d]", idx), fmt.Sprintf("native operation %s unsupported platform: %s", operationID, platform))
		}
	}
	if !knownNativeArtifactForm(operation.ArtifactForm) {
		add("nativeOperation.artifactForm.unsupported", operationPath+".artifactForm", fmt.Sprintf("native operation %s unsupported artifactForm: %s", operationID, operation.ArtifactForm))
	}
	if !knownNativeDiffMode(operation.DiffMode) {
		add("nativeOperation.diffMode.unsupported", operationPath+".diffMode", fmt.Sprintf("native operation %s unsupported diffMode: %s", operationID, operation.DiffMode))
	}
	if !knownLifecycle(operation.Lifecycle) {
		add("nativeOperation.lifecycle.unsupported", operationPath+".lifecycle", fmt.Sprintf("native operation %s unsupported lifecycle: %s", operationID, operation.Lifecycle))
	}
	if operation.WorkingDirectory != "temp" {
		add("nativeOperation.workingDirectory.unsupported", operationPath+".workingDirectory", fmt.Sprintf("native operation %s must use explicit temp workingDirectory", operationID))
	}
	if operation.TimeoutSeconds <= 0 || operation.TimeoutSeconds > maxNativeOperationTimeoutSeconds {
		add("nativeOperation.timeout.invalid", operationPath+".timeoutSeconds", fmt.Sprintf("native operation %s timeoutSeconds must be 1..%d", operationID, maxNativeOperationTimeoutSeconds))
	}
	if len(operation.ExpectedExitCodes) == 0 || len(operation.ExpectedExitCodes) > maxNativeExpectedExitCodes {
		add("nativeOperation.expectedExitCodes.invalid", operationPath+".expectedExitCodes", fmt.Sprintf("native operation %s must declare 1..%d expected exit codes", operationID, maxNativeExpectedExitCodes))
	}
	seenExit := map[int]bool{}
	for idx, code := range operation.ExpectedExitCodes {
		if code < 0 || code > 255 || seenExit[code] {
			add("nativeOperation.expectedExitCode.invalid", operationPath+fmt.Sprintf(".expectedExitCodes[%d]", idx), fmt.Sprintf("native operation %s invalid expected exit code: %d", operationID, code))
		}
		seenExit[code] = true
	}
	addNativeCommandDiagnostics(add, operationPath, operationID, operation)
	addNativeStdinDiagnostics(add, operationPath+".stdin", operationID, operation.Stdin)
	addNativeStreamDiagnostics(add, operationPath+".stdout", operationID, "stdout", operation.Stdout)
	addNativeStreamDiagnostics(add, operationPath+".stderr", operationID, "stderr", operation.Stderr)
	addNativeEnvDiagnostics(add, operationPath, operationID, operation)
	addNativePathSpecsDiagnostics(add, operationPath+".inputs", operationID, "input", operation.Kind, operation.Inputs)
	addNativePathSpecsDiagnostics(add, operationPath+".outputs", operationID, "output", operation.Kind, operation.Outputs)
	addNativePathSpecsDiagnostics(add, operationPath+".tempPaths", operationID, "temp", operation.Kind, operation.TempPaths)
	if !knownNativeRedaction(operation.Redaction) {
		add("nativeOperation.redaction.unsupported", operationPath+".redaction", fmt.Sprintf("native operation %s unsupported redaction policy: %s", operationID, operation.Redaction))
	}
	addNativeReviewDiagnostics(add, operationPath+".review", operationID, operation.Review)
	addNativeLimitsDiagnostics(add, operationPath+".limits", operationID, operation.Limits)
	addNativeExportMetadataDiagnostics(add, operationPath+".exportMetadata", operationID, operation.ExportMetadata)
}

func addNativeCommandDiagnostics(add func(string, string, string), operationPath string, operationID string, operation NativeOperation) {
	executable := strings.TrimSpace(operation.Command.Executable)
	if executable == "" {
		add("nativeOperation.command.executable.required", operationPath+".command.executable", fmt.Sprintf("native operation %s command executable is required", operationID))
	} else {
		if executable != operation.Command.Executable {
			add("nativeOperation.command.executable.invalid", operationPath+".command.executable", fmt.Sprintf("native operation %s command executable must not have surrounding whitespace", operationID))
		}
		if !filepath.IsAbs(executable) {
			add("nativeOperation.command.executable.notAbsolute", operationPath+".command.executable", fmt.Sprintf("native operation %s command executable must be an absolute reviewed path", operationID))
		}
		cleaned := filepath.Clean(executable)
		if cleaned != executable || strings.Contains(filepath.ToSlash(executable), "../") {
			add("nativeOperation.command.executable.invalidPath", operationPath+".command.executable", fmt.Sprintf("native operation %s command executable path must be clean and non-traversing", operationID))
		}
		if blockedNativeExecutable(filepath.Base(executable)) {
			add("nativeOperation.command.executable.blocked", operationPath+".command.executable", fmt.Sprintf("native operation %s command executable is blocked: %s", operationID, filepath.Base(executable)))
		}
	}
	for idx, arg := range operation.Command.Args {
		if countNativeArgChoices(arg.Literal, arg.Input, arg.Output, arg.Temp) != 1 {
			add("nativeOperation.command.arg.invalid", operationPath+fmt.Sprintf(".command.args[%d]", idx), fmt.Sprintf("native operation %s args must be exactly one typed token", operationID))
			continue
		}
		if arg.Literal != "" && strings.Contains(arg.Literal, "\x00") {
			add("nativeOperation.command.arg.invalid", operationPath+fmt.Sprintf(".command.args[%d].literal", idx), fmt.Sprintf("native operation %s literal arg contains a NUL byte", operationID))
		}
		if strings.Contains(arg.Literal, "{{") || strings.Contains(arg.Literal, "}}") {
			add("nativeOperation.command.arg.interpolationUnsupported", operationPath+fmt.Sprintf(".command.args[%d].literal", idx), fmt.Sprintf("native operation %s literal arg must not contain interpolation syntax", operationID))
		}
		if arg.Input != "" && !declaresNativePath(operation.Inputs, arg.Input) {
			add("nativeOperation.command.arg.inputUnknown", operationPath+fmt.Sprintf(".command.args[%d].input", idx), fmt.Sprintf("native operation %s arg references undeclared input %s", operationID, arg.Input))
		}
		if arg.Output != "" && !declaresNativePath(operation.Outputs, arg.Output) {
			add("nativeOperation.command.arg.outputUnknown", operationPath+fmt.Sprintf(".command.args[%d].output", idx), fmt.Sprintf("native operation %s arg references undeclared output %s", operationID, arg.Output))
		}
		if arg.Temp != "" && !declaresNativePath(operation.TempPaths, arg.Temp) {
			add("nativeOperation.command.arg.tempUnknown", operationPath+fmt.Sprintf(".command.args[%d].temp", idx), fmt.Sprintf("native operation %s arg references undeclared temp path %s", operationID, arg.Temp))
		}
	}
	if strings.EqualFold(filepath.Base(executable), "osascript") {
		for idx, arg := range operation.Command.Args {
			if arg.Literal == "-e" {
				add("nativeOperation.command.osascriptInlineBlocked", operationPath+fmt.Sprintf(".command.args[%d].literal", idx), fmt.Sprintf("native operation %s must not use osascript -e inline script mode", operationID))
			}
		}
	}
}

func addNativeStdinDiagnostics(add func(string, string, string), path string, operationID string, policy NativeStdinPolicy) {
	if policy.Mode != "none" {
		add("nativeOperation.stdin.unsupported", path+".mode", fmt.Sprintf("native operation %s stdin mode must be none", operationID))
	}
}

func addNativeStreamDiagnostics(add func(string, string, string), path string, operationID string, stream string, policy NativeStreamPolicy) {
	switch policy.Mode {
	case "discard":
		if policy.MaxBytes != 0 {
			add("nativeOperation.stream.maxBytes.invalid", path+".maxBytes", fmt.Sprintf("native operation %s %s discard mode must not set maxBytes", operationID, stream))
		}
	case "capture":
		if policy.MaxBytes <= 0 || policy.MaxBytes > maxNativeCaptureBytes {
			add("nativeOperation.stream.maxBytes.invalid", path+".maxBytes", fmt.Sprintf("native operation %s %s capture maxBytes must be 1..%d", operationID, stream, maxNativeCaptureBytes))
		}
	default:
		add("nativeOperation.stream.mode.unsupported", path+".mode", fmt.Sprintf("native operation %s unsupported %s mode: %s", operationID, stream, policy.Mode))
	}
}

func addNativeEnvDiagnostics(add func(string, string, string), operationPath string, operationID string, operation NativeOperation) {
	for _, key := range sortedKeys(operation.Env) {
		value := operation.Env[key]
		path := operationPath + ".env." + key
		if !validNativeEnvKey(key) {
			add("nativeOperation.env.key.invalid", path, fmt.Sprintf("native operation %s invalid env key: %s", operationID, key))
		}
		if !safeNativeEnvKey(key) {
			add("nativeOperation.env.key.unsupported", path, fmt.Sprintf("native operation %s env key is not in the supported DFM_ manager namespace: %s", operationID, key))
		}
		if sensitiveNativeEnvKey(key) {
			add("nativeOperation.env.key.sensitive", path, fmt.Sprintf("native operation %s env key appears sensitive and is blocked: %s", operationID, key))
		}
		if countNativeArgChoices(value.Literal, value.Input, value.Output, value.Temp) != 1 {
			add("nativeOperation.env.value.invalid", path, fmt.Sprintf("native operation %s env value must be exactly one typed token", operationID))
			continue
		}
		if strings.Contains(value.Literal, "\x00") || strings.Contains(value.Literal, "\n") || strings.Contains(value.Literal, "\r") {
			add("nativeOperation.env.value.invalid", path+".literal", fmt.Sprintf("native operation %s literal env value contains unsupported control characters", operationID))
		}
		if strings.Contains(value.Literal, "{{") || strings.Contains(value.Literal, "}}") {
			add("nativeOperation.env.value.interpolationUnsupported", path+".literal", fmt.Sprintf("native operation %s literal env value must not contain interpolation syntax", operationID))
		}
		if value.Input != "" && !declaresNativePath(operation.Inputs, value.Input) {
			add("nativeOperation.env.inputUnknown", path+".input", fmt.Sprintf("native operation %s env references undeclared input %s", operationID, value.Input))
		}
		if value.Output != "" && !declaresNativePath(operation.Outputs, value.Output) {
			add("nativeOperation.env.outputUnknown", path+".output", fmt.Sprintf("native operation %s env references undeclared output %s", operationID, value.Output))
		}
		if value.Temp != "" && !declaresNativePath(operation.TempPaths, value.Temp) {
			add("nativeOperation.env.tempUnknown", path+".temp", fmt.Sprintf("native operation %s env references undeclared temp path %s", operationID, value.Temp))
		}
	}
}

func addNativePathSpecsDiagnostics(add func(string, string, string), path string, operationID string, role string, kind string, specs map[string]NativePathSpec) {
	for _, id := range sortedKeys(specs) {
		spec := specs[id]
		specPath := path + "." + id
		if err := ValidatePublicID("nativePath", id); err != nil {
			add("nativeOperation.path.id.invalid", specPath, err.Error())
		}
		if !knownNativePathRoot(spec.Root) {
			add("nativeOperation.path.root.unsupported", specPath+".root", fmt.Sprintf("native operation %s unsupported %s root: %s", operationID, role, spec.Root))
		}
		if spec.Root == "location" && strings.TrimSpace(spec.Location) == "" {
			add("nativeOperation.path.location.required", specPath+".location", fmt.Sprintf("native operation %s %s %s requires a named location", operationID, role, id))
		}
		if spec.Root != "location" && spec.Location != "" {
			add("nativeOperation.path.location.unexpected", specPath+".location", fmt.Sprintf("native operation %s %s %s must not set location for root %s", operationID, role, id, spec.Root))
		}
		if !safeNativeRelPath(spec.Path) {
			add("nativeOperation.path.path.invalid", specPath+".path", fmt.Sprintf("native operation %s %s %s path must be relative, clean, and non-traversing", operationID, role, id))
		}
		if role == "output" && kind == "export" && spec.Root == "location" {
			add("nativeOperation.path.output.exportRootUnsupported", specPath+".root", fmt.Sprintf("native operation %s export outputs must use artifact or temp root", operationID))
		}
		if role == "output" && kind == "verify" && spec.Root != "temp" {
			add("nativeOperation.path.output.verifyRootUnsupported", specPath+".root", fmt.Sprintf("native operation %s verify outputs must use temp root", operationID))
		}
		if role == "output" && kind == "import" && spec.Root != "temp" {
			add("nativeOperation.path.output.importRootUnsupported", specPath+".root", fmt.Sprintf("native operation %s import outputs must use temp root", operationID))
		}
		if role == "input" && kind == "export" && spec.Root == "artifact" {
			add("nativeOperation.path.input.exportArtifactUnsupported", specPath+".root", fmt.Sprintf("native operation %s export inputs must not read desired artifacts", operationID))
		}
		if role == "input" && kind == "import" && spec.Root == "location" {
			add("nativeOperation.path.input.importRootUnsupported", specPath+".root", fmt.Sprintf("native operation %s import inputs must use artifact or temp root", operationID))
		}
		if role == "temp" && kind == "import" && spec.Root == "location" {
			add("nativeOperation.path.temp.importRootUnsupported", specPath+".root", fmt.Sprintf("native operation %s import temp paths must use temp root", operationID))
		}
	}
}

func addLifecycleTargetDiagnostics(add func(string, string, string), targetPath string, targetID string, target LifecycleTarget) {
	if strings.TrimSpace(target.DisplayName) != target.DisplayName {
		add("lifecycleTarget.displayName.invalid", targetPath+".displayName", fmt.Sprintf("lifecycleTarget %s displayName must not have surrounding whitespace", targetID))
	}
	if target.Detect.Kind != LifecycleDetectProcessName {
		add("lifecycleTarget.detect.kind.unsupported", targetPath+".detect.kind", fmt.Sprintf("lifecycleTarget %s unsupported detect kind", targetID))
	}
	if len(target.Detect.Names) == 0 {
		add("lifecycleTarget.detect.names.required", targetPath+".detect.names", fmt.Sprintf("lifecycleTarget %s process-name detection requires at least one exact process name", targetID))
	}
	seenNames := map[string]bool{}
	for idx, name := range target.Detect.Names {
		namePath := targetPath + fmt.Sprintf(".detect.names[%d]", idx)
		if err := validateLifecycleProcessName(name); err != nil {
			add("lifecycleTarget.detect.name.invalid", namePath, fmt.Sprintf("lifecycleTarget %s process name is invalid: %s", targetID, err.Error()))
			continue
		}
		if seenNames[name] {
			add("lifecycleTarget.detect.name.duplicate", namePath, fmt.Sprintf("lifecycleTarget %s duplicates process name", targetID))
		}
		seenNames[name] = true
	}
	if target.Quit.Kind != "" && target.Quit.Kind != LifecycleControlUnsupported && target.Quit.Kind != LifecycleControlManaged {
		add("lifecycleTarget.quit.kind.unsupported", targetPath+".quit.kind", fmt.Sprintf("lifecycleTarget %s unsupported quit kind", targetID))
	}
	if target.Reopen.Kind != "" && target.Reopen.Kind != LifecycleReopenNone && target.Reopen.Kind != LifecycleControlManaged {
		add("lifecycleTarget.reopen.kind.unsupported", targetPath+".reopen.kind", fmt.Sprintf("lifecycleTarget %s unsupported reopen kind", targetID))
	}
}

func validateLifecycleProcessName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("must not be blank")
	}
	if strings.TrimSpace(name) != name {
		return fmt.Errorf("must not have surrounding whitespace")
	}
	if strings.ContainsAny(name, "\x00\r\n/\\*?[]{}$`|;&<>") {
		return fmt.Errorf("must be an exact process basename without path, glob, regex, shell, or control characters")
	}
	return nil
}

func addLifecyclePolicyReferenceDiagnostics(add func(string, string, string), r *Recipe) {
	if r == nil {
		return
	}
	for _, resourceID := range sortedKeys(r.Resources) {
		resource := r.Resources[resourceID]
		path := "$.resources." + resourceID
		addLifecycleSubjectReferenceDiagnostics(add, r, path, "resource "+resourceID, resource.Lifecycle, resource.LifecycleTarget)
	}
	for _, settingID := range sortedKeys(r.Settings) {
		setting := r.Settings[settingID]
		resource := Resource{}
		if setting.Resource != "" {
			resource = r.Resources[setting.Resource]
		}
		targetID := setting.LifecycleTarget
		if targetID == "" {
			targetID = resource.LifecycleTarget
		}
		path := "$.settings." + settingID
		addLifecycleSubjectReferenceDiagnostics(add, r, path, "setting "+settingID, setting.Lifecycle, targetID)
	}
	for _, operationID := range sortedKeys(r.NativeOperations) {
		operation := r.NativeOperations[operationID]
		path := "$.nativeOperations." + operationID
		addLifecycleSubjectReferenceDiagnostics(add, r, path, "native operation "+operationID, operation.Lifecycle, operation.LifecycleTarget)
	}
}

func addLifecycleSubjectReferenceDiagnostics(add func(string, string, string), r *Recipe, path string, subject string, lifecycle string, targetID string) {
	if lifecycle == "" || !knownLifecycle(lifecycle) || !lifecycleNeedsTarget(lifecycle) {
		return
	}
	if strings.TrimSpace(targetID) == "" {
		add("lifecycleTarget.required", path+".lifecycleTarget", fmt.Sprintf("%s lifecycle policy requires an explicit lifecycleTarget", subject))
		return
	}
	target, ok := r.LifecycleTargets[targetID]
	if !ok {
		return
	}
	if lifecycleNeedsManagedQuit(lifecycle) && target.Quit.Kind != LifecycleControlManaged {
		add("lifecycleTarget.quit.unsupported", path+".lifecycleTarget", fmt.Sprintf("%s lifecycle policy requires lifecycleTarget %s to declare managed quit", subject, targetID))
	}
	if lifecycleNeedsManagedReopen(lifecycle) && target.Reopen.Kind != LifecycleControlManaged {
		add("lifecycleTarget.reopen.unsupported", path+".lifecycleTarget", fmt.Sprintf("%s lifecycle policy requires lifecycleTarget %s to declare managed reopen", subject, targetID))
	}
}

func lifecycleNeedsTarget(value string) bool {
	switch value {
	case LifecycleAskToQuit, LifecycleQuitIfRunning, LifecycleBlockIfRunning, LifecycleReopenIfStoppedByTool:
		return true
	default:
		return false
	}
}

func lifecycleNeedsManagedQuit(value string) bool {
	switch value {
	case LifecycleQuitIfRunning, LifecycleReopenIfStoppedByTool:
		return true
	default:
		return false
	}
}

func lifecycleNeedsManagedReopen(value string) bool {
	return value == LifecycleReopenIfStoppedByTool
}

func addNativeOperationLocationDiagnostics(add func(string, string, string), operationPath string, operationID string, operation NativeOperation, locations map[string]Location) {
	check := func(role string, specs map[string]NativePathSpec) {
		for _, id := range sortedKeys(specs) {
			spec := specs[id]
			if spec.Root != "location" || strings.TrimSpace(spec.Location) == "" {
				continue
			}
			if _, ok := locations[spec.Location]; !ok {
				add("nativeOperation.path.location.unknown", operationPath+"."+role+"."+id+".location", fmt.Sprintf("native operation %s %s %s references unknown location %s", operationID, role, id, spec.Location))
			}
		}
	}
	check("inputs", operation.Inputs)
	check("outputs", operation.Outputs)
	check("tempPaths", operation.TempPaths)
}

func recipeNeedsLocations(r *Recipe) bool {
	if r == nil {
		return true
	}
	for _, resource := range r.Resources {
		if resource.Driver != NativeExportDriverID {
			return true
		}
	}
	for _, operation := range r.NativeOperations {
		for _, specs := range []map[string]NativePathSpec{operation.Inputs, operation.Outputs, operation.TempPaths} {
			for _, spec := range specs {
				if spec.Root == "location" {
					return true
				}
			}
		}
	}
	return false
}

func addNativeReviewDiagnostics(add func(string, string, string), path string, operationID string, review NativeReviewPolicy) {
	for idx, reason := range review.Reasons {
		if !knownNativeReviewReason(reason) {
			add("nativeOperation.review.reason.unsupported", path+fmt.Sprintf(".reasons[%d]", idx), fmt.Sprintf("native operation %s unsupported review reason: %s", operationID, reason))
		}
	}
	if strings.ContainsAny(review.Message, "\r\n\x00") {
		add("nativeOperation.review.message.invalid", path+".message", fmt.Sprintf("native operation %s review message must be a single safe line", operationID))
	}
	if !review.Required && (len(review.Reasons) > 0 || strings.TrimSpace(review.Message) != "") {
		add("nativeOperation.review.required.invalid", path+".required", fmt.Sprintf("native operation %s review reasons/message require review.required true", operationID))
	}
}

func addNativeLimitsDiagnostics(add func(string, string, string), path string, operationID string, limits NativeExportLimits) {
	if limits.MaxBytes < 0 || limits.MaxBytes > int64(maxNativeExportBytes) {
		add("nativeOperation.limits.maxBytes.invalid", path+".maxBytes", fmt.Sprintf("native operation %s maxBytes must be 1..%d when set", operationID, maxNativeExportBytes))
	} else if limits.MaxBytes == 0 {
		// Zero means use the conservative manager default.
	} else if limits.MaxBytes < 1 {
		add("nativeOperation.limits.maxBytes.invalid", path+".maxBytes", fmt.Sprintf("native operation %s maxBytes must be 1..%d when set", operationID, maxNativeExportBytes))
	}
	if limits.MaxEntries < 0 || limits.MaxEntries > maxNativeExportEntries {
		add("nativeOperation.limits.maxEntries.invalid", path+".maxEntries", fmt.Sprintf("native operation %s maxEntries must be 1..%d when set", operationID, maxNativeExportEntries))
	} else if limits.MaxEntries == 0 {
		// Zero means use the conservative manager default.
	} else if limits.MaxEntries < 1 {
		add("nativeOperation.limits.maxEntries.invalid", path+".maxEntries", fmt.Sprintf("native operation %s maxEntries must be 1..%d when set", operationID, maxNativeExportEntries))
	}
}

func addNativeExportMetadataDiagnostics(add func(string, string, string), path string, operationID string, metadata NativeExportMetadataPolicy) {
	validateList := func(field string, values []string, idOnly bool) {
		for idx, value := range values {
			itemPath := path + fmt.Sprintf(".%s[%d]", field, idx)
			if strings.TrimSpace(value) != value || value == "" || strings.ContainsAny(value, "\r\n\x00") {
				add("nativeOperation.exportMetadata.value.invalid", itemPath, fmt.Sprintf("native operation %s export metadata %s must contain safe single-line values", operationID, field))
				continue
			}
			if idOnly {
				if err := ValidatePublicID("native export metadata "+field, value); err != nil {
					add("nativeOperation.exportMetadata.id.invalid", itemPath, err.Error())
				}
			}
		}
	}
	validateList("capturedCategories", metadata.CapturedCategories, true)
	validateList("secretExclusions", metadata.SecretExclusions, true)
	validateList("accountExclusions", metadata.AccountExclusions, true)
	validateList("limitations", metadata.Limitations, false)
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

func (r *Recipe) ValidateZsh() error {
	if err := r.Validate(); err != nil {
		return err
	}
	if r.Target != ZshTarget {
		return fmt.Errorf("zsh recipe target must be %q, got %q", ZshTarget, r.Target)
	}
	if r.Capability != "read-write" {
		return fmt.Errorf("zsh recipe capability must be read-write, got %s", r.Capability)
	}
	if len(r.Locations) != 1 {
		return fmt.Errorf("zsh recipe must declare only the home location")
	}
	location, ok := r.Locations["home"]
	if !ok {
		return fmt.Errorf("zsh recipe must declare home location")
	}
	if strings.TrimSpace(location.Default) != "~" {
		return fmt.Errorf("zsh home location default must be ~")
	}
	if len(r.Settings) != len(zshSettingIDs()) {
		return fmt.Errorf("zsh recipe must declare only supported startup file settings")
	}
	if len(r.Resources) != len(zshSettingIDs()) {
		return fmt.Errorf("zsh recipe must declare exactly one file resource per supported startup file")
	}
	for _, settingID := range zshSettingIDs() {
		if err := r.validateZshSetting(settingID); err != nil {
			return err
		}
	}
	if _, exists := r.Settings["zshenv"]; exists {
		return fmt.Errorf("zsh recipe must not declare .zshenv as a managed setting")
	}
	if _, exists := r.Resources["zshenv"]; exists {
		return fmt.Errorf("zsh recipe must not declare .zshenv as a managed resource")
	}
	return nil
}

func (r *Recipe) ValidateNvim() error {
	if err := r.Validate(); err != nil {
		return err
	}
	if r.Target != NvimTarget {
		return fmt.Errorf("nvim recipe target must be %q, got %q", NvimTarget, r.Target)
	}
	if r.Capability != "read-write" {
		return fmt.Errorf("nvim recipe capability must be read-write, got %s", r.Capability)
	}
	if len(r.Locations) != 1 {
		return fmt.Errorf("nvim recipe must declare only the config location")
	}
	location, ok := r.Locations["config"]
	if !ok {
		return fmt.Errorf("nvim recipe must declare config location")
	}
	if strings.TrimSpace(location.Default) != "~/.config" {
		return fmt.Errorf("nvim config location default must be ~/.config")
	}
	if len(r.Settings) != len(nvimSettingIDs()) {
		return fmt.Errorf("nvim recipe must declare only supported settings")
	}
	if len(r.Resources) != len(nvimSettingIDs()) {
		return fmt.Errorf("nvim recipe must declare exactly one file-tree resource per supported setting")
	}
	for _, settingID := range nvimSettingIDs() {
		if err := r.validateNvimSetting(settingID); err != nil {
			return err
		}
	}
	return nil
}

func (r *Recipe) ValidateTmux() error {
	if err := r.Validate(); err != nil {
		return err
	}
	if r.Target != TmuxTarget {
		return fmt.Errorf("tmux recipe target must be %q, got %q", TmuxTarget, r.Target)
	}
	if r.Capability != "read-write" {
		return fmt.Errorf("tmux recipe capability must be read-write, got %s", r.Capability)
	}
	home, ok := r.Locations["home"]
	if !ok {
		return fmt.Errorf("tmux recipe must declare home location")
	}
	if strings.TrimSpace(home.Default) != "~" {
		return fmt.Errorf("tmux home location default must be ~")
	}
	config, ok := r.Locations["config"]
	if !ok {
		return fmt.Errorf("tmux recipe must declare config location")
	}
	if strings.TrimSpace(config.Default) != "~/.config" {
		return fmt.Errorf("tmux config location default must be ~/.config")
	}
	if len(r.Locations) != 2 {
		return fmt.Errorf("tmux recipe must declare only home and config locations")
	}
	if len(r.Settings) != len(tmuxSettingIDs()) {
		return fmt.Errorf("tmux recipe must declare only supported user config file settings")
	}
	if len(r.Resources) != len(tmuxSettingIDs()) {
		return fmt.Errorf("tmux recipe must declare exactly one file resource per supported user config file")
	}
	for _, settingID := range tmuxSettingIDs() {
		if err := r.validateTmuxSetting(settingID); err != nil {
			return err
		}
	}
	return nil
}

func (r *Recipe) ValidateSSH() error {
	if err := r.Validate(); err != nil {
		return err
	}
	if r.Target != SSHTarget {
		return fmt.Errorf("ssh recipe target must be %q, got %q", SSHTarget, r.Target)
	}
	if r.Capability != "read-write" {
		return fmt.Errorf("ssh recipe capability must be read-write, got %s", r.Capability)
	}
	if len(r.Locations) != 1 {
		return fmt.Errorf("ssh recipe must declare only the home location")
	}
	location, ok := r.Locations["home"]
	if !ok {
		return fmt.Errorf("ssh recipe must declare home location")
	}
	if strings.TrimSpace(location.Default) != "~" {
		return fmt.Errorf("ssh home location default must be ~")
	}
	if len(r.Settings) != len(sshSettingIDs()) {
		return fmt.Errorf("ssh recipe must declare only supported config file settings")
	}
	if len(r.Resources) != len(sshSettingIDs()) {
		return fmt.Errorf("ssh recipe must declare exactly one file resource for the config file")
	}
	for _, settingID := range sshSettingIDs() {
		if err := r.validateSSHSetting(settingID); err != nil {
			return err
		}
	}
	return nil
}

func (r *Recipe) validateSSHSetting(settingID string) error {
	setting, ok := r.Settings[settingID]
	if !ok {
		return fmt.Errorf("ssh recipe missing setting %s", settingID)
	}
	if setting.ScopeDefault != "user" {
		return fmt.Errorf("ssh setting %s scopeDefault must be user", settingID)
	}
	if setting.SupportLevel != "experimental" {
		return fmt.Errorf("ssh setting %s supportLevel must be experimental", settingID)
	}
	if setting.Capability != "read-write" {
		return fmt.Errorf("ssh setting %s capability must be read-write", settingID)
	}
	if setting.ArtifactForm != "file" {
		return fmt.Errorf("ssh setting %s artifactForm must be file", settingID)
	}
	if setting.Sensitivity != SensitivityPersonal {
		return fmt.Errorf("ssh setting %s sensitivity must be %s", settingID, SensitivityPersonal)
	}
	if setting.Redaction != RedactionRedactedForDisplay {
		return fmt.Errorf("ssh setting %s redaction must be %s", settingID, RedactionRedactedForDisplay)
	}
	if setting.Lifecycle != LifecycleAllowed {
		return fmt.Errorf("ssh setting %s lifecycle must be %s", settingID, LifecycleAllowed)
	}
	if len(setting.WriteWarnings) > 0 {
		return fmt.Errorf("ssh setting %s must not declare setting-level write warnings", settingID)
	}
	if setting.Resource != settingID {
		return fmt.Errorf("ssh setting %s resource must be %s", settingID, settingID)
	}
	resource, ok := r.Resources[setting.Resource]
	if !ok {
		return fmt.Errorf("ssh setting %s references unknown resource %s", settingID, setting.Resource)
	}
	if resource.Driver != FileDriverID {
		return fmt.Errorf("ssh setting %s driver must be %q, got %q", settingID, FileDriverID, resource.Driver)
	}
	if resource.Location != "home" {
		return fmt.Errorf("ssh setting %s location must be home", settingID)
	}
	if resource.Path != ".ssh/config" {
		return fmt.Errorf("ssh setting %s path must be .ssh/config", settingID)
	}
	if resource.Capability != "read-write" {
		return fmt.Errorf("ssh resource %s capability must be read-write", settingID)
	}
	if resource.Sensitivity != SensitivityPersonal {
		return fmt.Errorf("ssh resource %s sensitivity must be %s", settingID, SensitivityPersonal)
	}
	if resource.Redaction != RedactionRedactedForDisplay {
		return fmt.Errorf("ssh resource %s redaction must be %s", settingID, RedactionRedactedForDisplay)
	}
	if resource.Lifecycle != LifecycleAllowed {
		return fmt.Errorf("ssh resource %s lifecycle must be %s", settingID, LifecycleAllowed)
	}
	if resource.ContentSafetyPolicy != SSHContentSafetyPolicy {
		return fmt.Errorf("ssh resource %s contentSafetyPolicy must be %s", settingID, SSHContentSafetyPolicy)
	}
	if len(resource.WriteWarnings) != 1 || resource.WriteWarnings[0].Code != SSHConfigReviewWarningCode || !stringSlicesEqual(resource.WriteWarnings[0].Triggers, []string{"save", "apply"}) {
		return fmt.Errorf("ssh resource %s must declare the SSH config save/apply review warning", settingID)
	}
	if resource.Selector != nil {
		return fmt.Errorf("ssh resource %s must not declare selector", settingID)
	}
	if len(resource.Include) > 0 || len(resource.Exclude) > 0 {
		return fmt.Errorf("ssh resource %s must not declare include/exclude globs", settingID)
	}
	return nil
}

func (r *Recipe) validateTmuxSetting(settingID string) error {
	setting, ok := r.Settings[settingID]
	if !ok {
		return fmt.Errorf("tmux recipe missing setting %s", settingID)
	}
	if setting.ScopeDefault != "user" {
		return fmt.Errorf("tmux setting %s scopeDefault must be user", settingID)
	}
	if setting.SupportLevel != "experimental" {
		return fmt.Errorf("tmux setting %s supportLevel must be experimental", settingID)
	}
	if setting.Capability != "read-write" {
		return fmt.Errorf("tmux setting %s capability must be read-write", settingID)
	}
	if setting.ArtifactForm != "file" {
		return fmt.Errorf("tmux setting %s artifactForm must be file", settingID)
	}
	if setting.Sensitivity != SensitivityPersonal {
		return fmt.Errorf("tmux setting %s sensitivity must be %s", settingID, SensitivityPersonal)
	}
	if setting.Redaction != RedactionRedactedForDisplay {
		return fmt.Errorf("tmux setting %s redaction must be %s", settingID, RedactionRedactedForDisplay)
	}
	if setting.Lifecycle != LifecycleWarn {
		return fmt.Errorf("tmux setting %s lifecycle must be %s", settingID, LifecycleWarn)
	}
	if setting.Resource != settingID {
		return fmt.Errorf("tmux setting %s resource must be %s", settingID, settingID)
	}
	resource, ok := r.Resources[setting.Resource]
	if !ok {
		return fmt.Errorf("tmux setting %s references unknown resource %s", settingID, setting.Resource)
	}
	if resource.Driver != FileDriverID {
		return fmt.Errorf("tmux setting %s driver must be %q, got %q", settingID, FileDriverID, resource.Driver)
	}
	if resource.Location != tmuxLocationID(settingID) {
		return fmt.Errorf("tmux setting %s location must be %s", settingID, tmuxLocationID(settingID))
	}
	if resource.Path != tmuxResourcePath(settingID) {
		return fmt.Errorf("tmux setting %s path must be %s", settingID, tmuxResourcePath(settingID))
	}
	if resource.Capability != "read-write" {
		return fmt.Errorf("tmux resource %s capability must be read-write", settingID)
	}
	if resource.Sensitivity != SensitivityPersonal {
		return fmt.Errorf("tmux resource %s sensitivity must be %s", settingID, SensitivityPersonal)
	}
	if resource.Redaction != RedactionRedactedForDisplay {
		return fmt.Errorf("tmux resource %s redaction must be %s", settingID, RedactionRedactedForDisplay)
	}
	if resource.Lifecycle != LifecycleWarn {
		return fmt.Errorf("tmux resource %s lifecycle must be %s", settingID, LifecycleWarn)
	}
	if resource.Selector != nil {
		return fmt.Errorf("tmux resource %s must not declare selector", settingID)
	}
	if len(resource.Include) > 0 || len(resource.Exclude) > 0 {
		return fmt.Errorf("tmux resource %s must not declare include/exclude globs", settingID)
	}
	return nil
}

func (r *Recipe) validateNvimSetting(settingID string) error {
	setting, ok := r.Settings[settingID]
	if !ok {
		return fmt.Errorf("nvim recipe missing setting %s", settingID)
	}
	if setting.ScopeDefault != "user" {
		return fmt.Errorf("nvim setting %s scopeDefault must be user", settingID)
	}
	if setting.SupportLevel != "experimental" {
		return fmt.Errorf("nvim setting %s supportLevel must be experimental", settingID)
	}
	if setting.Capability != "read-write" {
		return fmt.Errorf("nvim setting %s capability must be read-write", settingID)
	}
	if setting.ArtifactForm != "file-tree" {
		return fmt.Errorf("nvim setting %s artifactForm must be file-tree", settingID)
	}
	if setting.Sensitivity != SensitivityPersonal {
		return fmt.Errorf("nvim setting %s sensitivity must be %s", settingID, SensitivityPersonal)
	}
	if setting.Redaction != RedactionRedactedForDisplay {
		return fmt.Errorf("nvim setting %s redaction must be %s", settingID, RedactionRedactedForDisplay)
	}
	if setting.Lifecycle != LifecycleAllowed {
		return fmt.Errorf("nvim setting %s lifecycle must be %s", settingID, LifecycleAllowed)
	}
	if setting.Resource != settingID {
		return fmt.Errorf("nvim setting %s resource must be %s", settingID, settingID)
	}
	resource, ok := r.Resources[setting.Resource]
	if !ok {
		return fmt.Errorf("nvim setting %s references unknown resource %s", settingID, setting.Resource)
	}
	if resource.Driver != FileTreeDriverID {
		return fmt.Errorf("nvim setting %s driver must be %q, got %q", settingID, FileTreeDriverID, resource.Driver)
	}
	if resource.Location != "config" {
		return fmt.Errorf("nvim setting %s location must be config", settingID)
	}
	if resource.Path != "nvim" {
		return fmt.Errorf("nvim setting %s path must be nvim", settingID)
	}
	if resource.Capability != "read-write" {
		return fmt.Errorf("nvim resource %s capability must be read-write", settingID)
	}
	if resource.Sensitivity != SensitivityPersonal {
		return fmt.Errorf("nvim resource %s sensitivity must be %s", settingID, SensitivityPersonal)
	}
	if resource.Redaction != RedactionRedactedForDisplay {
		return fmt.Errorf("nvim resource %s redaction must be %s", settingID, RedactionRedactedForDisplay)
	}
	if resource.Lifecycle != LifecycleAllowed {
		return fmt.Errorf("nvim resource %s lifecycle must be %s", settingID, LifecycleAllowed)
	}
	if resource.Selector != nil {
		return fmt.Errorf("nvim resource %s must not declare selector", settingID)
	}
	include, exclude, err := filetreedriver.NormalizeGlobs(resource.Include, resource.Exclude)
	if err != nil {
		return fmt.Errorf("nvim resource %s globs: %w", settingID, err)
	}
	if !stringSlicesEqual(include, []string{"**"}) {
		return fmt.Errorf("nvim resource %s include globs must be the default **", settingID)
	}
	if !stringSlicesEqual(exclude, nvimExcludeGlobs()) {
		return fmt.Errorf("nvim resource %s exclude globs must match bundled generated-state policy", settingID)
	}
	return nil
}

func (r *Recipe) validateZshSetting(settingID string) error {
	setting, ok := r.Settings[settingID]
	if !ok {
		return fmt.Errorf("zsh recipe missing setting %s", settingID)
	}
	if setting.ScopeDefault != "user" {
		return fmt.Errorf("zsh setting %s scopeDefault must be user", settingID)
	}
	if setting.SupportLevel != "experimental" {
		return fmt.Errorf("zsh setting %s supportLevel must be experimental", settingID)
	}
	if setting.Capability != "read-write" {
		return fmt.Errorf("zsh setting %s capability must be read-write", settingID)
	}
	if setting.ArtifactForm != "file" {
		return fmt.Errorf("zsh setting %s artifactForm must be file", settingID)
	}
	if setting.Sensitivity != SensitivityPersonal {
		return fmt.Errorf("zsh setting %s sensitivity must be %s", settingID, SensitivityPersonal)
	}
	if setting.Redaction != RedactionRedactedForDisplay {
		return fmt.Errorf("zsh setting %s redaction must be %s", settingID, RedactionRedactedForDisplay)
	}
	if setting.Lifecycle != LifecycleWarn {
		return fmt.Errorf("zsh setting %s lifecycle must be %s", settingID, LifecycleWarn)
	}
	if setting.Resource != settingID {
		return fmt.Errorf("zsh setting %s resource must be %s", settingID, settingID)
	}
	resource, ok := r.Resources[setting.Resource]
	if !ok {
		return fmt.Errorf("zsh setting %s references unknown resource %s", settingID, setting.Resource)
	}
	if resource.Driver != FileDriverID {
		return fmt.Errorf("zsh setting %s driver must be %q, got %q", settingID, FileDriverID, resource.Driver)
	}
	if resource.Location != "home" {
		return fmt.Errorf("zsh setting %s location must be home", settingID)
	}
	if resource.Path != zshResourcePath(settingID) {
		return fmt.Errorf("zsh setting %s path must be %s", settingID, zshResourcePath(settingID))
	}
	if resource.Capability != "read-write" {
		return fmt.Errorf("zsh resource %s capability must be read-write", settingID)
	}
	if resource.Sensitivity != SensitivityPersonal {
		return fmt.Errorf("zsh resource %s sensitivity must be %s", settingID, SensitivityPersonal)
	}
	if resource.Redaction != RedactionRedactedForDisplay {
		return fmt.Errorf("zsh resource %s redaction must be %s", settingID, RedactionRedactedForDisplay)
	}
	if resource.Lifecycle != LifecycleWarn {
		return fmt.Errorf("zsh resource %s lifecycle must be %s", settingID, LifecycleWarn)
	}
	if resource.Selector != nil {
		return fmt.Errorf("zsh resource %s must not declare selector", settingID)
	}
	if len(resource.Include) > 0 || len(resource.Exclude) > 0 {
		return fmt.Errorf("zsh resource %s must not declare include/exclude globs", settingID)
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

func zshSettingIDs() []string {
	return []string{"zshrc", "zprofile", "zlogin", "zlogout"}
}

func nvimSettingIDs() []string {
	return []string{"config"}
}

func sshSettingIDs() []string {
	return []string{"config"}
}

func tmuxSettingIDs() []string {
	return []string{"home.conf", "xdg.conf"}
}

func tmuxLocationID(settingID string) string {
	switch settingID {
	case "home.conf":
		return "home"
	case "xdg.conf":
		return "config"
	default:
		return ""
	}
}

func tmuxResourcePath(settingID string) string {
	switch settingID {
	case "home.conf":
		return ".tmux.conf"
	case "xdg.conf":
		return "tmux/tmux.conf"
	default:
		return ""
	}
}

func nvimExcludeGlobs() []string {
	return []string{
		".git/**",
		"**/.git/**",
		"**/*.swp",
		"**/*.swo",
		"**/*.swn",
		"**/*~",
		"**/*.bak",
		"**/*.backup",
		"**/*.tmp",
		"Session.vim",
		"**/Session.vim",
		"session/**",
		"sessions/**",
		"shada/**",
		"**/*.shada",
		"**/*.shada.tmp",
		"main.shada",
		"swap/**",
		"undo/**",
		"view/**",
		"cache/**",
		".cache/**",
		".netrwhist",
		"pack/**/start/**",
		"pack/**/opt/**",
		"site/pack/**",
		"bundle/**",
		"plugged/**",
		"plugin/packer_compiled.lua",
		"**/node_modules/**",
		"**/.deps/**",
		"**/.rocks/**",
		"**/.env",
		"**/*.pem",
		"**/*.key",
		"**/*.p12",
		"**/*.pfx",
		"**/id_rsa",
		"**/id_ed25519",
	}
}

func zshResourcePath(settingID string) string {
	switch settingID {
	case "zshrc":
		return ".zshrc"
	case "zprofile":
		return ".zprofile"
	case "zlogin":
		return ".zlogin"
	case "zlogout":
		return ".zlogout"
	default:
		return ""
	}
}

func ZshBlockedSettingDiagnostic(settingID string) (ValidationDiagnostic, bool) {
	switch strings.TrimSpace(settingID) {
	case "zshenv":
		return zshBlockedDiagnostic(ZshBlockedZshenvCode, settingID, ".zshenv is blocked because it affects almost every zsh invocation and can break login or session startup"), true
	case "history", "zsh-history", "zhistory":
		return zshBlockedDiagnostic(ZshBlockedHistoryCode, settingID, "Zsh history files are blocked because shell history can contain secrets and transient commands"), true
	case "zcompdump", "completion-cache", "cache", "zsh-cache":
		return zshBlockedDiagnostic(ZshBlockedCompletionCacheCode, settingID, "Zsh completion dump/cache files and cache directories are generated state and are blocked"), true
	case "zsh-sessions", "sessions":
		return zshBlockedDiagnostic(ZshBlockedSessionStateCode, settingID, "Zsh session directories are generated local state and are blocked"), true
	case "oh-my-zsh", "custom", "zprezto", "zinit", "zim", "zplug", "plugin-state":
		return zshBlockedDiagnostic(ZshBlockedPluginStateCode, settingID, "Zsh plugin-manager or generated custom state is blocked in this recipe"), true
	default:
		return ValidationDiagnostic{}, false
	}
}

func zshBlockedDiagnostic(code string, settingID string, message string) ValidationDiagnostic {
	return ValidationDiagnostic{Code: code, Severity: ValidationSeverityError, Message: message, Path: "$.settings." + settingID}
}

func SSHExcludedSettingDiagnostic(settingID string) (ValidationDiagnostic, bool) {
	switch strings.TrimSpace(settingID) {
	case "keys", "private-keys", "public-keys", "identity", "known_hosts", "known-hosts", "authorized_keys", "authorized-keys", "agent", "sockets", "control-sockets", "config.d", "includes", "certificates", "host-keys":
		return ValidationDiagnostic{
			Code:     SSHRefExcludedCode,
			Severity: ValidationSeverityError,
			Message:  "bundled SSH recipe supports only ssh:config and does not read keys, known_hosts, authorized_keys, sockets, include targets, certificates, host keys, or agent state",
			Path:     "$.settings." + settingID,
		}, true
	default:
		return ValidationDiagnostic{}, false
	}
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
	case NativeExportDriverID:
		return validateNativeExportResource(resourceID, resource)
	case FileDriverID, FileTreeDriverID:
		if resource.Selector != nil {
			return fmt.Errorf("resource %s driver %q must not declare selector", resourceID, resource.Driver)
		}
	default:
		return fmt.Errorf("resource %s unsupported driver %q", resourceID, resource.Driver)
	}
	return nil
}

func validateNativeExportResource(resourceID string, resource Resource) error {
	if resource.Location != "" || resource.Path != "" {
		return fmt.Errorf("resource %s driver %q must not declare location or path", resourceID, NativeExportDriverID)
	}
	if strings.TrimSpace(resource.NativeOperation) == "" {
		return fmt.Errorf("resource %s driver %q requires nativeOperation", resourceID, NativeExportDriverID)
	}
	if len(resource.Include) > 0 || len(resource.Exclude) > 0 {
		return fmt.Errorf("resource %s driver %q must not declare include/exclude globs", resourceID, NativeExportDriverID)
	}
	if resource.Selector != nil {
		return fmt.Errorf("resource %s driver %q must not declare selector", resourceID, NativeExportDriverID)
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
	case "file", "file-tree", "scalar", "structured", "native", "native-export", "opaque", "metadata-only":
		return true
	default:
		return false
	}
}

func knownNativeOperationKind(value string) bool {
	switch value {
	case "export", "import", "verify":
		return true
	default:
		return false
	}
}

func knownNativePlatform(value string) bool {
	switch value {
	case "darwin", "linux", "windows":
		return true
	default:
		return false
	}
}

func knownNativeArtifactForm(value string) bool {
	switch value {
	case "native", "native-export", "opaque":
		return true
	default:
		return false
	}
}

func knownNativeDiffMode(value string) bool {
	switch value {
	case "metadata-only", "structured", "opaque":
		return true
	default:
		return false
	}
}

func knownNativeRedaction(value string) bool {
	switch value {
	case "metadata-only", "redacted-counts", "blocked-save":
		return true
	default:
		return false
	}
}

func knownNativeReviewReason(value string) bool {
	switch value {
	case "large", "account-bound", "opaque", "privacy-sensitive":
		return true
	default:
		return false
	}
}

func knownNativePathRoot(value string) bool {
	switch value {
	case "artifact", "temp", "location":
		return true
	default:
		return false
	}
}

func countNativeArgChoices(values ...string) int {
	count := 0
	for _, value := range values {
		if value != "" {
			count++
		}
	}
	return count
}

func declaresNativePath(specs map[string]NativePathSpec, id string) bool {
	if strings.TrimSpace(id) != id || id == "" {
		return false
	}
	_, ok := specs[id]
	return ok
}

func blockedNativeExecutable(base string) bool {
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

func validNativeEnvKey(key string) bool {
	if key == "" || strings.TrimSpace(key) != key {
		return false
	}
	for i, r := range key {
		if (r >= 'A' && r <= 'Z') || r == '_' || (i > 0 && r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

func safeNativeEnvKey(key string) bool {
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

func sensitiveNativeEnvKey(key string) bool {
	upper := strings.ToUpper(key)
	for _, marker := range []string{"TOKEN", "KEY", "PASSWORD", "PASS", "SECRET", "CREDENTIAL", "AUTH"} {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	return false
}

func safeNativeRelPath(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed != value {
		return false
	}
	if filepath.IsAbs(trimmed) || strings.Contains(trimmed, "\\") {
		return false
	}
	cleaned := pathpkg.Clean(trimmed)
	if cleaned != trimmed || cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return false
	}
	for _, segment := range strings.Split(cleaned, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
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

func knownContentSafetyPolicy(value string) bool {
	switch value {
	case SSHContentSafetyPolicy:
		return true
	default:
		return false
	}
}

func addReviewWarningDiagnostics(add func(string, string, string), path string, subject string, warning ReviewWarning) {
	code := strings.TrimSpace(warning.Code)
	if code == "" {
		add("reviewWarning.code.required", path+".code", subject+" review warning code is required")
	} else if err := ValidatePublicID("reviewWarning", code); err != nil {
		add("reviewWarning.code.invalid", path+".code", subject+" review warning code is invalid")
	}
	if len(warning.Triggers) == 0 {
		add("reviewWarning.triggers.required", path+".triggers", subject+" review warning requires at least one trigger")
	}
	seen := map[string]bool{}
	for idx, trigger := range warning.Triggers {
		trimmed := strings.TrimSpace(trigger)
		triggerPath := fmt.Sprintf("%s.triggers[%d]", path, idx)
		if !knownReviewWarningTrigger(trimmed) {
			add("reviewWarning.trigger.unsupported", triggerPath, subject+" review warning trigger is unsupported")
			continue
		}
		if seen[trimmed] {
			add("reviewWarning.trigger.duplicate", triggerPath, subject+" review warning trigger is duplicated")
		}
		seen[trimmed] = true
	}
	if strings.TrimSpace(warning.Message) != warning.Message {
		add("reviewWarning.message.invalid", path+".message", subject+" review warning message must not have surrounding whitespace")
	}
}

func knownReviewWarningTrigger(value string) bool {
	switch value {
	case "save", "apply":
		return true
	default:
		return false
	}
}

func stringSlicesEqual(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
