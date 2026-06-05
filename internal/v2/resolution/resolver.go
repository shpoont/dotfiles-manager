package resolution

import (
	"fmt"
	"io"
	"os"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	RootConfigFile = "dotfiles-manager.v2.yaml"

	rootConfigSchema     = "dotfiles-manager.v2.root-config"
	profileStackSchema   = "dotfiles-manager.v2.profile-stack"
	profileLayerSchema   = "dotfiles-manager.v2.profile-layer"
	supportedSchemaMajor = 1
)

var (
	publicIDPattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*(?:[.-][a-z0-9][a-z0-9_-]*)*$`)
	identityIDRegexp = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
)

type ResolveOptions struct {
	MachineID   string
	UserID      string
	ExtraLayers []string
}

type ResolvedProfile struct {
	RepoRoot           string
	ActiveProfileStack string
	Layers             []string
	Settings           []ResolvedSetting
}

type ResolvedSetting struct {
	TargetID       string
	SettingID      string
	Scope          string
	Subject        string
	SourceLayer    string
	DesiredURI     string
	DesiredRelPath string
	DesiredPath    string
}

func (s ResolvedSetting) Ref() string {
	return s.TargetID + ":" + s.SettingID
}

type rootConfigFile struct {
	Schema             string `yaml:"schema"`
	SchemaVersion      int    `yaml:"schemaVersion"`
	ActiveProfileStack string `yaml:"activeProfileStack"`
}

type profileStackFile struct {
	Schema        string   `yaml:"schema"`
	SchemaVersion int      `yaml:"schemaVersion"`
	ProfileStack  []string `yaml:"profileStack"`
}

type profileLayerFile struct {
	Schema        string                     `yaml:"schema"`
	SchemaVersion int                        `yaml:"schemaVersion"`
	Selections    map[string]targetSelection `yaml:"selections"`
}

type targetSelection struct {
	Settings map[string]settingSelection `yaml:"settings"`
}

type settingSelection struct {
	Scope    string `yaml:"scope"`
	Artifact string `yaml:"artifact,omitempty"`
}

type mergedSelection struct {
	TargetID    string
	SettingID   string
	SourceLayer string
	Selection   settingSelection
}

func FindRoot(startDir string) (string, error) {
	start := strings.TrimSpace(startDir)
	if start == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve cwd for v2 root: %w", err)
		}
		start = cwd
	}

	abs, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("resolve v2 root start path %q: %w", startDir, err)
	}

	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("stat v2 root start path %q: %w", abs, err)
	}
	if !info.IsDir() {
		abs = filepath.Dir(abs)
	}

	for {
		candidate := filepath.Join(abs, RootConfigFile)
		if isRegularFile(candidate) {
			return abs, nil
		}

		parent := filepath.Dir(abs)
		if parent == abs {
			return "", fmt.Errorf("v2 repository root not found from %q: missing %s", startDir, RootConfigFile)
		}
		abs = parent
	}
}

func Resolve(repoRoot string, opts ResolveOptions) (*ResolvedProfile, error) {
	root, err := normalizeRepoRoot(repoRoot)
	if err != nil {
		return nil, err
	}

	rootConfig, err := loadRootConfig(root)
	if err != nil {
		return nil, err
	}
	activeStack, err := validateProfilePathID("active profile stack", rootConfig.ActiveProfileStack)
	if err != nil {
		return nil, err
	}

	stack, err := loadProfileStack(root, activeStack)
	if err != nil {
		return nil, err
	}

	layerIDs := append([]string{}, stack.ProfileStack...)
	layerIDs = append(layerIDs, opts.ExtraLayers...)
	if len(layerIDs) == 0 {
		return nil, fmt.Errorf("profile stack %q has no layers", activeStack)
	}

	merged := map[string]mergedSelection{}
	resolvedLayers := make([]string, 0, len(layerIDs))
	for _, rawLayerID := range layerIDs {
		layerID, err := validateProfilePathID("profile layer", rawLayerID)
		if err != nil {
			return nil, err
		}
		layer, err := loadProfileLayer(root, layerID)
		if err != nil {
			return nil, err
		}
		resolvedLayers = append(resolvedLayers, layerID)
		applyLayerSelections(merged, layerID, layer)
	}

	settings, err := resolveSelections(root, merged, opts)
	if err != nil {
		return nil, err
	}

	return &ResolvedProfile{
		RepoRoot:           root,
		ActiveProfileStack: activeStack,
		Layers:             resolvedLayers,
		Settings:           settings,
	}, nil
}

func normalizeRepoRoot(repoRoot string) (string, error) {
	trimmed := strings.TrimSpace(repoRoot)
	if trimmed == "" {
		return "", fmt.Errorf("v2 repo root is required")
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", fmt.Errorf("resolve v2 repo root %q: %w", repoRoot, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("stat v2 repo root %q: %w", abs, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("v2 repo root is not a directory: %s", abs)
	}
	if !isRegularFile(filepath.Join(abs, RootConfigFile)) {
		return "", fmt.Errorf("v2 repo root missing %s: %s", RootConfigFile, abs)
	}
	return abs, nil
}

func loadRootConfig(root string) (*rootConfigFile, error) {
	var cfg rootConfigFile
	if err := decodeKnownYAML(filepath.Join(root, RootConfigFile), &cfg); err != nil {
		return nil, err
	}
	if err := validateSchema("root config", cfg.Schema, cfg.SchemaVersion, rootConfigSchema); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.ActiveProfileStack) == "" {
		return nil, fmt.Errorf("root config activeProfileStack is required")
	}
	return &cfg, nil
}

func loadProfileStack(root string, stackID string) (*profileStackFile, error) {
	path := filepath.Join(root, "profiles", "stacks", filepath.FromSlash(stackID)+".yaml")
	var stack profileStackFile
	if err := decodeKnownYAML(path, &stack); err != nil {
		return nil, err
	}
	if err := validateSchema("profile stack", stack.Schema, stack.SchemaVersion, profileStackSchema); err != nil {
		return nil, err
	}
	for _, layerID := range stack.ProfileStack {
		if _, err := validateProfilePathID("profile layer", layerID); err != nil {
			return nil, err
		}
	}
	return &stack, nil
}

func loadProfileLayer(root string, layerID string) (*profileLayerFile, error) {
	path := filepath.Join(root, "profiles", "layers", filepath.FromSlash(layerID)+".yaml")
	var layer profileLayerFile
	if err := decodeKnownYAML(path, &layer); err != nil {
		return nil, err
	}
	if err := validateSchema("profile layer", layer.Schema, layer.SchemaVersion, profileLayerSchema); err != nil {
		return nil, err
	}
	return &layer, nil
}

func decodeKnownYAML(path string, out any) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	defer file.Close()
	return decodeKnownYAMLReader(path, file, out)
}

func decodeKnownYAMLReader(path string, r io.Reader, out any) error {
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

func validateSchema(kind string, actual string, version int, expected string) error {
	if actual != expected {
		return fmt.Errorf("invalid %s schema: %q (expected %q)", kind, actual, expected)
	}
	if version != supportedSchemaMajor {
		return fmt.Errorf("invalid %s schemaVersion: %d (expected %d)", kind, version, supportedSchemaMajor)
	}
	return nil
}

func applyLayerSelections(merged map[string]mergedSelection, layerID string, layer *profileLayerFile) {
	targetIDs := sortedKeys(layer.Selections)
	for _, targetID := range targetIDs {
		target := layer.Selections[targetID]
		settingIDs := sortedKeys(target.Settings)
		for _, settingID := range settingIDs {
			key := selectionKey(targetID, settingID)
			merged[key] = mergedSelection{
				TargetID:    targetID,
				SettingID:   settingID,
				SourceLayer: layerID,
				Selection:   target.Settings[settingID],
			}
		}
	}
}

func resolveSelections(root string, merged map[string]mergedSelection, opts ResolveOptions) ([]ResolvedSetting, error) {
	keys := sortedKeys(merged)
	settings := make([]ResolvedSetting, 0, len(keys))
	seenURI := map[string]string{}

	for _, key := range keys {
		selection := merged[key]
		if err := validatePublicID("target", selection.TargetID); err != nil {
			return nil, err
		}
		if err := validatePublicID("setting", selection.SettingID); err != nil {
			return nil, err
		}

		scope := strings.TrimSpace(selection.Selection.Scope)
		subjectURI, subjectPath, err := resolveSubject(scope, opts)
		if err != nil {
			return nil, err
		}

		artifact, err := resolveArtifact(scope, subjectURI, subjectPath, selection.TargetID, selection.SettingID, selection.Selection.Artifact)
		if err != nil {
			return nil, err
		}
		if previousKey, exists := seenURI[artifact.URI]; exists && previousKey != key {
			return nil, fmt.Errorf("duplicate artifact binding: %s and %s both resolve to %s", previousKey, key, artifact.URI)
		}
		seenURI[artifact.URI] = key

		desiredPath := filepath.Join(root, artifact.RelPath)
		if err := ensurePathInside(filepath.Join(root, artifact.TargetRelDir), desiredPath); err != nil {
			return nil, err
		}

		settings = append(settings, ResolvedSetting{
			TargetID:       selection.TargetID,
			SettingID:      selection.SettingID,
			Scope:          scope,
			Subject:        subjectURI,
			SourceLayer:    selection.SourceLayer,
			DesiredURI:     artifact.URI,
			DesiredRelPath: artifact.RelPath,
			DesiredPath:    desiredPath,
		})
	}

	return settings, nil
}

type resolvedArtifact struct {
	URI          string
	RelPath      string
	TargetRelDir string
}

func resolveArtifact(scope string, subjectURI string, subjectPath []string, targetID string, settingID string, rawArtifact string) (resolvedArtifact, error) {
	targetRelParts := append([]string{"desired", scope}, subjectPath...)
	targetRelParts = append(targetRelParts, "targets", targetID)
	targetRelDirSlash := pathpkg.Join(targetRelParts...)

	artifact := strings.TrimSpace(rawArtifact)
	if artifact == "" {
		relPathSlash := pathpkg.Join(targetRelDirSlash, "settings.yaml")
		return resolvedArtifact{
			URI:          desiredSettingsURI(scope, subjectURI, targetID, settingID),
			RelPath:      filepath.FromSlash(relPathSlash),
			TargetRelDir: filepath.FromSlash(targetRelDirSlash),
		}, nil
	}

	pathPart, fragment, err := splitArtifactFragment(artifact)
	if err != nil {
		return resolvedArtifact{}, err
	}
	pathPart, err = validateArtifactPath(pathPart)
	if err != nil {
		return resolvedArtifact{}, err
	}

	uri, err := desiredURIForArtifactPath(scope, subjectURI, targetID, pathPart, fragment)
	if err != nil {
		return resolvedArtifact{}, err
	}

	relPathSlash := pathpkg.Join(targetRelDirSlash, pathPart)
	return resolvedArtifact{
		URI:          uri,
		RelPath:      filepath.FromSlash(relPathSlash),
		TargetRelDir: filepath.FromSlash(targetRelDirSlash),
	}, nil
}

func splitArtifactFragment(value string) (string, string, error) {
	parts := strings.SplitN(value, "#", 2)
	pathPart := strings.TrimSpace(parts[0])
	fragment := ""
	if len(parts) == 2 {
		fragment = strings.TrimSpace(parts[1])
		if fragment == "" {
			return "", "", fmt.Errorf("artifact fragment must not be empty: %s", value)
		}
		if err := validatePublicID("artifact fragment", fragment); err != nil {
			return "", "", err
		}
	}
	return pathPart, fragment, nil
}

func desiredURIForArtifactPath(scope string, subjectURI string, targetID string, artifactPath string, fragment string) (string, error) {
	switch {
	case artifactPath == "manifest.yaml":
		if fragment != "" {
			return "", fmt.Errorf("manifest artifact must not include a fragment")
		}
		return fmt.Sprintf("desired://%s/%s/targets/%s/manifest", scope, subjectURI, targetID), nil
	case artifactPath == "settings.yaml":
		return desiredSettingsURI(scope, subjectURI, targetID, fragment), nil
	case strings.HasPrefix(artifactPath, "artifacts/"):
		if fragment != "" {
			return "", fmt.Errorf("artifact payload path must not include a fragment: %s", artifactPath)
		}
		return fmt.Sprintf("desired://%s/%s/targets/%s/artifacts/%s", scope, subjectURI, targetID, strings.TrimPrefix(artifactPath, "artifacts/")), nil
	default:
		return "", fmt.Errorf("artifact path must be manifest.yaml, settings.yaml, or artifacts/...: %s", artifactPath)
	}
}

func desiredSettingsURI(scope string, subjectURI string, targetID string, fragment string) string {
	uri := fmt.Sprintf("desired://%s/%s/targets/%s/settings", scope, subjectURI, targetID)
	if fragment != "" {
		uri += "#" + fragment
	}
	return uri
}

func resolveSubject(scope string, opts ResolveOptions) (string, []string, error) {
	switch scope {
	case "shared":
		return "-", []string{"-"}, nil
	case "user":
		userID, err := requireIdentity("user", opts.UserID)
		if err != nil {
			return "", nil, err
		}
		return userID, []string{userID}, nil
	case "machine":
		machineID, err := requireIdentity("machine", opts.MachineID)
		if err != nil {
			return "", nil, err
		}
		return machineID, []string{machineID}, nil
	case "machine-user":
		machineID, err := requireIdentity("machine", opts.MachineID)
		if err != nil {
			return "", nil, err
		}
		userID, err := requireIdentity("user", opts.UserID)
		if err != nil {
			return "", nil, err
		}
		return machineID + "/" + userID, []string{machineID, userID}, nil
	default:
		return "", nil, fmt.Errorf("unknown scope: %s", scope)
	}
}

func requireIdentity(kind string, value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("%s id required for selected scope", kind)
	}
	if !identityIDRegexp.MatchString(trimmed) {
		return "", fmt.Errorf("invalid %s id: %s", kind, value)
	}
	return trimmed, nil
}

func validatePublicID(kind string, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed != value {
		return fmt.Errorf("invalid %s id: %s", kind, value)
	}
	if !publicIDPattern.MatchString(value) {
		return fmt.Errorf("invalid %s id: %s", kind, value)
	}
	return nil
}

func validateProfilePathID(kind string, value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("%s id is required", kind)
	}
	if strings.Contains(trimmed, "\\") {
		return "", fmt.Errorf("%s id must not contain backslashes: %s", kind, value)
	}
	if filepath.IsAbs(trimmed) || strings.HasPrefix(trimmed, "/") {
		return "", fmt.Errorf("%s id must be relative: %s", kind, value)
	}

	slashed := filepath.ToSlash(trimmed)
	parts := strings.Split(slashed, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("%s id contains unsafe segment: %s", kind, value)
		}
	}
	cleaned := pathpkg.Clean(slashed)
	if cleaned != slashed || strings.HasPrefix(cleaned, "../") || cleaned == ".." {
		return "", fmt.Errorf("%s id escapes profile directory: %s", kind, value)
	}
	return slashed, nil
}

func validateArtifactPath(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("artifact path is required")
	}
	if strings.Contains(trimmed, "\\") {
		return "", fmt.Errorf("artifact path must not contain backslashes: %s", value)
	}
	if filepath.IsAbs(trimmed) || strings.HasPrefix(trimmed, "/") {
		return "", fmt.Errorf("artifact path must be relative: %s", value)
	}
	slashed := filepath.ToSlash(trimmed)
	parts := strings.Split(slashed, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("artifact path contains unsafe segment: %s", value)
		}
	}
	cleaned := pathpkg.Clean(slashed)
	if cleaned != slashed || strings.HasPrefix(cleaned, "../") || cleaned == ".." {
		return "", fmt.Errorf("artifact path escapes desired target directory: %s", value)
	}
	return slashed, nil
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

func selectionKey(targetID string, settingID string) string {
	return targetID + ":" + settingID
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
