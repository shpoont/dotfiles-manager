package recipe

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	TrustRecordSchema  = "dotfiles-manager.v2.trust-record"
	TrustRecordVersion = 1

	TrustStatusTrusted        = "trusted"
	TrustStatusReviewRequired = "review-required"
	TrustStatusBlocked        = "blocked"
)

type TrustRecord struct {
	Schema        string                            `yaml:"schema" json:"schema"`
	SchemaVersion int                               `yaml:"schemaVersion" json:"schemaVersion"`
	LocalRecipes  map[string]LocalRecipeTrustRecord `yaml:"localRecipes" json:"localRecipes"`
}

type LocalRecipeTrustRecord struct {
	Source                   string            `yaml:"source" json:"source"`
	Target                   string            `yaml:"target" json:"target"`
	SchemaVersion            int               `yaml:"schemaVersion" json:"schemaVersion"`
	ContentSHA256            string            `yaml:"contentSHA256" json:"contentSHA256"`
	WriteSurfaceSHA256       string            `yaml:"writeSurfaceSHA256" json:"writeSurfaceSHA256"`
	WriteSurface             TrustWriteSurface `yaml:"writeSurface" json:"writeSurface"`
	ReviewedNativeOperations bool              `yaml:"reviewedNativeOperations" json:"reviewedNativeOperations"`
}

type TrustWriteSurface struct {
	Target           string                       `yaml:"target" json:"target"`
	SchemaVersion    int                          `yaml:"schemaVersion" json:"schemaVersion"`
	Capability       string                       `yaml:"capability" json:"capability"`
	Locations        []TrustLocationSurface       `yaml:"locations" json:"locations"`
	Settings         []TrustSettingSurface        `yaml:"settings" json:"settings"`
	Resources        []TrustResourceSurface       `yaml:"resources" json:"resources"`
	NativeOperations TrustNativeOperationsSurface `yaml:"nativeOperations" json:"nativeOperations"`
}

type TrustLocationSurface struct {
	ID      string `yaml:"id" json:"id"`
	Default string `yaml:"default" json:"default"`
}

type TrustSettingSurface struct {
	ID           string `yaml:"id" json:"id"`
	Capability   string `yaml:"capability" json:"capability"`
	Resource     string `yaml:"resource" json:"resource"`
	Sensitivity  string `yaml:"sensitivity,omitempty" json:"sensitivity,omitempty"`
	Redaction    string `yaml:"redaction,omitempty" json:"redaction,omitempty"`
	Lifecycle    string `yaml:"lifecycle,omitempty" json:"lifecycle,omitempty"`
	ArtifactForm string `yaml:"artifactForm,omitempty" json:"artifactForm,omitempty"`
	ScopeDefault string `yaml:"scopeDefault,omitempty" json:"scopeDefault,omitempty"`
}

type TrustResourceSurface struct {
	ID          string    `yaml:"id" json:"id"`
	Driver      string    `yaml:"driver" json:"driver"`
	Location    string    `yaml:"location" json:"location"`
	Path        string    `yaml:"path" json:"path"`
	Capability  string    `yaml:"capability" json:"capability"`
	Sensitivity string    `yaml:"sensitivity,omitempty" json:"sensitivity,omitempty"`
	Redaction   string    `yaml:"redaction,omitempty" json:"redaction,omitempty"`
	Lifecycle   string    `yaml:"lifecycle,omitempty" json:"lifecycle,omitempty"`
	Include     []string  `yaml:"include,omitempty" json:"include,omitempty"`
	Exclude     []string  `yaml:"exclude,omitempty" json:"exclude,omitempty"`
	Selector    *Selector `yaml:"selector,omitempty" json:"selector,omitempty"`
}

type TrustNativeOperationsSurface struct {
	Supported  bool                          `yaml:"supported" json:"supported"`
	Count      int                           `yaml:"count" json:"count"`
	Summary    string                        `yaml:"summary" json:"summary"`
	Operations []TrustNativeOperationSurface `yaml:"operations,omitempty" json:"operations,omitempty"`
}

type TrustNativeOperationSurface struct {
	ID        string          `yaml:"id" json:"id"`
	Operation NativeOperation `yaml:"operation" json:"operation"`
}

type TrustEvaluation struct {
	Source                   string                 `json:"source"`
	Target                   string                 `json:"target"`
	Status                   string                 `json:"status"`
	RecordPath               string                 `json:"recordPath,omitempty"`
	ContentSHA256            string                 `json:"contentSHA256,omitempty"`
	WriteSurfaceSHA256       string                 `json:"writeSurfaceSHA256,omitempty"`
	WriteSurface             TrustWriteSurface      `json:"writeSurface"`
	ReviewedNativeOperations bool                   `json:"reviewedNativeOperations"`
	Diagnostics              []ValidationDiagnostic `json:"diagnostics,omitempty"`
	localEvidence            *localTrustEvidence
}

type localTrustEvidence struct {
	status             string
	source             string
	target             string
	schemaVersion      int
	contentSHA256      string
	writeSurfaceSHA256 string
}

type trustPaths struct {
	repoRoot   string
	stateRoot  string
	recordPath string
}

func (e TrustEvaluation) WriteSafetyContext(base WriteSafetyContext) WriteSafetyContext {
	base.Source = e.Source
	if e.Source == RecipeSourceLocal && e.Status == TrustStatusTrusted && e.localEvidence != nil {
		base.Trusted = true
		base.localTrustEvidence = e.localEvidence
	}
	if e.Source == RecipeSourceBundled && e.Status == TrustStatusTrusted {
		base.Trusted = true
	}
	return base
}

func RecordLocalRecipeTrust(repoRoot string, stateRoot string, rec *Recipe) (TrustEvaluation, error) {
	eval, paths, err := newTrustEvaluation(repoRoot, stateRoot, RecipeSourceLocal, rec)
	if err != nil {
		return eval, err
	}
	if eval.Status == TrustStatusBlocked {
		return eval, validationError(eval.Diagnostics)
	}

	record, err := readTrustRecord(paths.recordPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			diag := validationDiagnostic("trust.record.corrupt", "trust/trust-record.yaml", "trust record is unreadable or invalid")
			eval.Status = TrustStatusBlocked
			eval.Diagnostics = []ValidationDiagnostic{diag}
			return eval, err
		}
		record = TrustRecord{Schema: TrustRecordSchema, SchemaVersion: TrustRecordVersion, LocalRecipes: map[string]LocalRecipeTrustRecord{}}
	}
	if record.LocalRecipes == nil {
		record.LocalRecipes = map[string]LocalRecipeTrustRecord{}
	}
	record.LocalRecipes[rec.Target] = LocalRecipeTrustRecord{
		Source:                   RecipeSourceLocal,
		Target:                   rec.Target,
		SchemaVersion:            rec.SchemaVersion,
		ContentSHA256:            eval.ContentSHA256,
		WriteSurfaceSHA256:       eval.WriteSurfaceSHA256,
		WriteSurface:             eval.WriteSurface,
		ReviewedNativeOperations: false,
	}
	if err := writeTrustRecordAtomic(paths, record); err != nil {
		return eval, err
	}
	return EvaluateRecipeTrust(repoRoot, stateRoot, RecipeSourceLocal, rec)
}

func EvaluateRecipeTrust(repoRoot string, stateRoot string, source string, rec *Recipe) (TrustEvaluation, error) {
	eval, paths, err := newTrustEvaluation(repoRoot, stateRoot, source, rec)
	if err != nil || eval.Status == TrustStatusBlocked || source == RecipeSourceBundled {
		return eval, err
	}

	if err := rejectSymlinksUnder(paths.stateRoot, filepath.Dir(paths.recordPath)); err != nil {
		eval.Status = TrustStatusBlocked
		eval.Diagnostics = []ValidationDiagnostic{validationDiagnostic("trust.stateRoot.invalid", "$", err.Error())}
		return eval, nil
	}
	if err := rejectSymlinksUnder(paths.stateRoot, paths.recordPath); err != nil {
		eval.Status = TrustStatusBlocked
		eval.Diagnostics = []ValidationDiagnostic{validationDiagnostic("trust.stateRoot.invalid", "$", err.Error())}
		return eval, nil
	}
	record, err := readTrustRecord(paths.recordPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			eval.Status = TrustStatusReviewRequired
			eval.Diagnostics = []ValidationDiagnostic{validationDiagnostic("trust.local.missingRecord", "trust/trust-record.yaml", fmt.Sprintf("local recipe %s requires an external local trust record before writes", rec.Target))}
			return eval, nil
		}
		eval.Status = TrustStatusBlocked
		eval.Diagnostics = []ValidationDiagnostic{validationDiagnostic("trust.record.corrupt", "trust/trust-record.yaml", "trust record is unreadable or invalid")}
		return eval, nil
	}

	entry, ok := record.LocalRecipes[rec.Target]
	if !ok {
		eval.Status = TrustStatusReviewRequired
		eval.Diagnostics = []ValidationDiagnostic{validationDiagnostic("trust.local.missingRecord", "trust/trust-record.yaml", fmt.Sprintf("local recipe %s requires an external local trust record before writes", rec.Target))}
		return eval, nil
	}
	if entry.Source != RecipeSourceLocal || entry.Target != rec.Target || entry.SchemaVersion != rec.SchemaVersion {
		eval.Status = TrustStatusReviewRequired
		eval.Diagnostics = []ValidationDiagnostic{validationDiagnostic("trust.local.recipeChanged", "trust/trust-record.yaml", fmt.Sprintf("local recipe %s trust record identity no longer matches", rec.Target))}
		return eval, nil
	}

	var diagnostics []ValidationDiagnostic
	if entry.ContentSHA256 != eval.ContentSHA256 {
		diagnostics = append(diagnostics, validationDiagnostic("trust.local.recipeChanged", "trust/trust-record.yaml", fmt.Sprintf("local recipe %s changed since trust was recorded", rec.Target)))
	}
	if entry.WriteSurfaceSHA256 != eval.WriteSurfaceSHA256 {
		diagnostics = append(diagnostics, validationDiagnostic("trust.local.writeSurfaceChanged", "trust/trust-record.yaml", fmt.Sprintf("local recipe %s write surface changed since trust was recorded", rec.Target)))
		if writeSurfaceBroadened(entry.WriteSurface, eval.WriteSurface) {
			diagnostics = append(diagnostics, validationDiagnostic("trust.local.writeSurfaceBroadened", "trust/trust-record.yaml", fmt.Sprintf("local recipe %s declares new or broadened write-capable metadata", rec.Target)))
		}
	}
	if eval.WriteSurface.NativeOperations.Count > 0 && !entry.ReviewedNativeOperations {
		diagnostics = append(diagnostics, validationDiagnostic("trust.local.nativeOperationsUnreviewed", "trust/trust-record.yaml", fmt.Sprintf("local recipe %s declares unreviewed native operations", rec.Target)))
	}
	if len(diagnostics) > 0 {
		eval.Status = TrustStatusReviewRequired
		eval.Diagnostics = normalizeDiagnostics(diagnostics)
		return eval, nil
	}

	eval.Status = TrustStatusTrusted
	eval.ReviewedNativeOperations = entry.ReviewedNativeOperations
	eval.localEvidence = &localTrustEvidence{
		status:             TrustStatusTrusted,
		source:             RecipeSourceLocal,
		target:             rec.Target,
		schemaVersion:      rec.SchemaVersion,
		contentSHA256:      eval.ContentSHA256,
		writeSurfaceSHA256: eval.WriteSurfaceSHA256,
	}
	return eval, nil
}

func newTrustEvaluation(repoRoot string, stateRoot string, source string, rec *Recipe) (TrustEvaluation, trustPaths, error) {
	eval := TrustEvaluation{Source: source, Status: TrustStatusBlocked}
	if rec == nil {
		eval.Diagnostics = []ValidationDiagnostic{validationDiagnostic("recipe.required", "$", "recipe is required")}
		return eval, trustPaths{}, nil
	}
	eval.Target = rec.Target
	if err := rec.Validate(); err != nil {
		eval.Diagnostics = ValidationDiagnostics(err)
		return eval, trustPaths{}, nil
	}
	contentHash, err := RecipeContentSHA256(rec)
	if err != nil {
		return eval, trustPaths{}, err
	}
	surface, surfaceHash, err := RecipeWriteSurface(rec)
	if err != nil {
		return eval, trustPaths{}, err
	}
	eval.ContentSHA256 = contentHash
	eval.WriteSurfaceSHA256 = surfaceHash
	eval.WriteSurface = surface

	switch source {
	case RecipeSourceBundled:
		eval.Status = TrustStatusTrusted
		eval.ReviewedNativeOperations = true
		return eval, trustPaths{}, nil
	case RecipeSourceLocal:
		paths, err := validateTrustRoots(repoRoot, stateRoot)
		if err != nil {
			eval.Diagnostics = []ValidationDiagnostic{validationDiagnostic("trust.stateRoot.invalid", "$", err.Error())}
			return eval, trustPaths{}, nil
		}
		eval.RecordPath = paths.recordPath
		eval.Status = TrustStatusReviewRequired
		return eval, paths, nil
	case "":
		eval.Diagnostics = []ValidationDiagnostic{validationDiagnostic("trust.source.required", "$", "recipe trust source is required")}
		return eval, trustPaths{}, nil
	default:
		eval.Diagnostics = []ValidationDiagnostic{validationDiagnostic("trust.source.unsupported", "$", "recipe trust source must be bundled or local")}
		return eval, trustPaths{}, nil
	}
}

func RecipeContentSHA256(rec *Recipe) (string, error) {
	if rec == nil {
		return "", fmt.Errorf("recipe is required")
	}
	if err := rec.Validate(); err != nil {
		return "", err
	}
	return canonicalSHA256(rec)
}

func RecipeWriteSurface(rec *Recipe) (TrustWriteSurface, string, error) {
	if rec == nil {
		return TrustWriteSurface{}, "", fmt.Errorf("recipe is required")
	}
	if err := rec.Validate(); err != nil {
		return TrustWriteSurface{}, "", err
	}
	surface := TrustWriteSurface{
		Target:        rec.Target,
		SchemaVersion: rec.SchemaVersion,
		Capability:    rec.Capability,
		Locations:     []TrustLocationSurface{},
		Settings:      []TrustSettingSurface{},
		Resources:     []TrustResourceSurface{},
		NativeOperations: TrustNativeOperationsSurface{
			Supported: false,
			Count:     0,
			Summary:   "none-declared-current-schema",
		},
	}
	if len(rec.NativeOperations) > 0 {
		surface.NativeOperations = TrustNativeOperationsSurface{
			Supported:  true,
			Count:      len(rec.NativeOperations),
			Summary:    nativeOperationsSurfaceSummary(rec.NativeOperations),
			Operations: nativeOperationsSurface(rec.NativeOperations),
		}
	}

	locationIDs := map[string]bool{}
	for _, settingID := range sortedKeys(rec.Settings) {
		setting := rec.Settings[settingID]
		capability := effectiveSettingCapability(rec, setting)
		if !isWriteCapableCapability(capability) {
			continue
		}
		surface.Settings = append(surface.Settings, TrustSettingSurface{
			ID:           settingID,
			Capability:   capability,
			Resource:     setting.Resource,
			Sensitivity:  setting.Sensitivity,
			Redaction:    setting.Redaction,
			Lifecycle:    setting.Lifecycle,
			ArtifactForm: setting.ArtifactForm,
			ScopeDefault: setting.ScopeDefault,
		})
		if resource, ok := rec.Resources[setting.Resource]; ok && resource.Location != "" {
			locationIDs[resource.Location] = true
		}
	}
	for _, resourceID := range sortedKeys(rec.Resources) {
		resource := rec.Resources[resourceID]
		capability := effectiveResourceCapability(rec, resource)
		if !isWriteCapableCapability(capability) {
			continue
		}
		include := append([]string(nil), resource.Include...)
		exclude := append([]string(nil), resource.Exclude...)
		sort.Strings(include)
		sort.Strings(exclude)
		locationIDs[resource.Location] = true
		surface.Resources = append(surface.Resources, TrustResourceSurface{
			ID:          resourceID,
			Driver:      resource.Driver,
			Location:    resource.Location,
			Path:        resource.Path,
			Capability:  capability,
			Sensitivity: resource.Sensitivity,
			Redaction:   resource.Redaction,
			Lifecycle:   resource.Lifecycle,
			Include:     include,
			Exclude:     exclude,
			Selector:    copySelector(resource.Selector),
		})
	}
	for _, locationID := range sortedBoolKeys(locationIDs) {
		if location, ok := rec.Locations[locationID]; ok {
			surface.Locations = append(surface.Locations, TrustLocationSurface{ID: locationID, Default: location.Default})
		}
	}
	hash, err := canonicalSHA256(surface)
	return surface, hash, err
}

func nativeOperationsSurfaceSummary(operations map[string]NativeOperation) string {
	if len(operations) == 0 {
		return "none-declared-current-schema"
	}
	parts := make([]string, 0, len(operations))
	for _, id := range sortedKeys(operations) {
		op := operations[id]
		parts = append(parts, id+":"+op.Kind+":"+op.Runner+":"+op.ArtifactForm+":"+op.DiffMode)
	}
	return strings.Join(parts, ",")
}

func nativeOperationsSurface(operations map[string]NativeOperation) []TrustNativeOperationSurface {
	if len(operations) == 0 {
		return nil
	}
	surface := make([]TrustNativeOperationSurface, 0, len(operations))
	for _, id := range sortedKeys(operations) {
		surface = append(surface, TrustNativeOperationSurface{ID: id, Operation: copyNativeOperation(operations[id])})
	}
	return surface
}

func copyNativeOperation(operation NativeOperation) NativeOperation {
	copy := operation
	copy.Platforms = append([]string(nil), operation.Platforms...)
	copy.ExpectedExitCodes = append([]int(nil), operation.ExpectedExitCodes...)
	copy.Command.Args = append([]NativeArg(nil), operation.Command.Args...)
	copy.Env = copyNativeEnvValues(operation.Env)
	copy.Inputs = copyNativePathSpecs(operation.Inputs)
	copy.Outputs = copyNativePathSpecs(operation.Outputs)
	copy.TempPaths = copyNativePathSpecs(operation.TempPaths)
	return copy
}

func copyNativeEnvValues(values map[string]NativeEnvValue) map[string]NativeEnvValue {
	if values == nil {
		return nil
	}
	copy := make(map[string]NativeEnvValue, len(values))
	for key, value := range values {
		copy[key] = value
	}
	return copy
}

func copyNativePathSpecs(values map[string]NativePathSpec) map[string]NativePathSpec {
	if values == nil {
		return nil
	}
	copy := make(map[string]NativePathSpec, len(values))
	for key, value := range values {
		copy[key] = value
	}
	return copy
}

func validateLocalTrustEvidence(rec *Recipe, ctx WriteSafetyContext) []ValidationDiagnostic {
	if ctx.localTrustEvidence == nil {
		return []ValidationDiagnostic{writeSafetyDiagnostic("writeSafety.trust.evidenceRequired", "$", ValidationSeverityError, "local write-capable recipes require evaluated external local trust evidence")}
	}
	evidence := ctx.localTrustEvidence
	contentHash, err := RecipeContentSHA256(rec)
	if err != nil {
		return []ValidationDiagnostic{writeSafetyDiagnostic("writeSafety.trust.fingerprintFailed", "$", ValidationSeverityError, "local recipe fingerprinting failed during write safety validation")}
	}
	_, surfaceHash, err := RecipeWriteSurface(rec)
	if err != nil {
		return []ValidationDiagnostic{writeSafetyDiagnostic("writeSafety.trust.fingerprintFailed", "$", ValidationSeverityError, "local recipe write-surface fingerprinting failed during write safety validation")}
	}
	var diagnostics []ValidationDiagnostic
	if evidence.status != TrustStatusTrusted || evidence.source != RecipeSourceLocal {
		diagnostics = append(diagnostics, writeSafetyDiagnostic("writeSafety.trust.evidenceInvalid", "$", ValidationSeverityError, "local recipe trust evidence is not trusted"))
	}
	if evidence.target != rec.Target || evidence.schemaVersion != rec.SchemaVersion {
		diagnostics = append(diagnostics, writeSafetyDiagnostic("writeSafety.trust.evidenceMismatch", "$", ValidationSeverityError, "local recipe trust evidence does not match this recipe"))
	}
	if evidence.contentSHA256 != contentHash {
		diagnostics = append(diagnostics, writeSafetyDiagnostic("writeSafety.trust.recipeChanged", "$", ValidationSeverityError, "local recipe changed since trust evidence was evaluated"))
	}
	if evidence.writeSurfaceSHA256 != surfaceHash {
		diagnostics = append(diagnostics, writeSafetyDiagnostic("writeSafety.trust.writeSurfaceChanged", "$", ValidationSeverityError, "local recipe write surface changed since trust evidence was evaluated"))
	}
	return normalizeDiagnostics(diagnostics)
}

func readTrustRecord(path string) (TrustRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return TrustRecord{}, err
	}
	var record TrustRecord
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&record); err != nil {
		return TrustRecord{}, err
	}
	if err := validateTrustRecord(record); err != nil {
		return TrustRecord{}, err
	}
	return record, nil
}

func validateTrustRecord(record TrustRecord) error {
	var diagnostics []ValidationDiagnostic
	add := func(code, path, message string) {
		diagnostics = append(diagnostics, validationDiagnostic(code, path, message))
	}
	if record.Schema != TrustRecordSchema {
		add("trust.record.schema.invalid", "$.schema", fmt.Sprintf("invalid trust record schema %q", record.Schema))
	}
	if record.SchemaVersion != TrustRecordVersion {
		add("trust.record.schemaVersion.invalid", "$.schemaVersion", fmt.Sprintf("invalid trust record schemaVersion %d", record.SchemaVersion))
	}
	for targetID, entry := range record.LocalRecipes {
		entryPath := "$.localRecipes." + targetID
		if err := ValidatePublicID("target", targetID); err != nil {
			add("trust.record.target.invalid", entryPath, err.Error())
		}
		if entry.Source != RecipeSourceLocal {
			add("trust.record.source.invalid", entryPath+".source", "local recipe trust record source must be local")
		}
		if entry.Target != targetID {
			add("trust.record.target.mismatch", entryPath+".target", "local recipe trust record target must match map key")
		}
		if entry.SchemaVersion != SupportedVersion {
			add("trust.record.recipeSchemaVersion.invalid", entryPath+".schemaVersion", "local recipe trust record schemaVersion is unsupported")
		}
		if entry.ContentSHA256 == "" {
			add("trust.record.contentSHA256.required", entryPath+".contentSHA256", "local recipe trust record contentSHA256 is required")
		}
		computedSurfaceHash, err := canonicalSHA256(entry.WriteSurface)
		if err != nil {
			add("trust.record.writeSurface.invalid", entryPath+".writeSurface", "local recipe trust record writeSurface is invalid")
		} else if entry.WriteSurfaceSHA256 != computedSurfaceHash {
			add("trust.record.writeSurfaceSHA256.mismatch", entryPath+".writeSurfaceSHA256", "local recipe trust record writeSurfaceSHA256 does not match writeSurface")
		}
	}
	return validationError(diagnostics)
}

func writeTrustRecordAtomic(paths trustPaths, record TrustRecord) error {
	if record.Schema == "" {
		record.Schema = TrustRecordSchema
	}
	if record.SchemaVersion == 0 {
		record.SchemaVersion = TrustRecordVersion
	}
	if record.LocalRecipes == nil {
		record.LocalRecipes = map[string]LocalRecipeTrustRecord{}
	}
	if err := os.MkdirAll(paths.stateRoot, 0o700); err != nil {
		return err
	}
	if err := rejectSymlinksUnder(paths.stateRoot, paths.stateRoot); err != nil {
		return err
	}
	dir := filepath.Dir(paths.recordPath)
	if err := rejectSymlinksUnder(paths.stateRoot, dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := rejectSymlinksUnder(paths.stateRoot, dir); err != nil {
		return err
	}
	if err := rejectSymlinksUnder(paths.stateRoot, paths.recordPath); err != nil {
		return err
	}
	data, err := yaml.Marshal(record)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".trust-record-*.tmp")
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
	if err := os.Rename(tmpPath, paths.recordPath); err != nil {
		return err
	}
	return nil
}

func validateTrustRoots(repoRoot string, stateRoot string) (trustPaths, error) {
	repo, err := normalizeRepoRoot(repoRoot)
	if err != nil {
		return trustPaths{}, err
	}
	trimmed := strings.TrimSpace(stateRoot)
	if trimmed == "" {
		return trustPaths{}, fmt.Errorf("state root is required")
	}
	state, err := filepath.Abs(trimmed)
	if err != nil {
		return trustPaths{}, fmt.Errorf("resolve state root %q: %w", stateRoot, err)
	}
	state = filepath.Clean(state)
	if isPathWithin(repo, state) {
		return trustPaths{}, fmt.Errorf("state root must be outside repository root")
	}
	if info, err := os.Lstat(state); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return trustPaths{}, fmt.Errorf("state root must not be a symlink")
		}
		if !info.IsDir() {
			return trustPaths{}, fmt.Errorf("state root is not a directory")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return trustPaths{}, err
	}
	return trustPaths{repoRoot: repo, stateRoot: state, recordPath: filepath.Join(state, "trust", "trust-record.yaml")}, nil
}

func rejectSymlinksUnder(root string, target string) error {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	if !isPathWithin(root, target) {
		return fmt.Errorf("path escapes state root: %s", target)
	}
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return err
	}
	if rel == "." {
		info, err := os.Lstat(root)
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("state root must not be a symlink")
		}
		return nil
	}
	current := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("state path must not contain symlink: %s", current)
		}
	}
	return nil
}

func isPathWithin(root string, target string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(target))
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func canonicalSHA256(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func writeSurfaceBroadened(previous TrustWriteSurface, current TrustWriteSurface) bool {
	previousKeys := writeSurfaceKeys(previous)
	for key := range writeSurfaceKeys(current) {
		if !previousKeys[key] {
			return true
		}
	}
	return false
}

func writeSurfaceKeys(surface TrustWriteSurface) map[string]bool {
	keys := map[string]bool{}
	for _, location := range surface.Locations {
		keys["location:"+location.ID+":"+mustCanonicalSHA256(location)] = true
	}
	for _, setting := range surface.Settings {
		keys["setting:"+setting.ID+":"+mustCanonicalSHA256(setting)] = true
	}
	for _, resource := range surface.Resources {
		keys["resource:"+resource.ID+":"+mustCanonicalSHA256(resource)] = true
	}
	if surface.NativeOperations.Count > 0 {
		keys["native:"+mustCanonicalSHA256(surface.NativeOperations)] = true
	}
	return keys
}

func mustCanonicalSHA256(value any) string {
	hash, err := canonicalSHA256(value)
	if err != nil {
		panic(err)
	}
	return hash
}

func copySelector(selector *Selector) *Selector {
	if selector == nil {
		return nil
	}
	copy := *selector
	copy.Path = append([]string(nil), selector.Path...)
	return &copy
}

func sortedBoolKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
