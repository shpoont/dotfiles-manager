// Package nativeexport manages reviewed native export artifacts.
//
// Native exports are deliberately treated as opaque payloads unless a later
// driver adds a structured normalizer. This package never prints, stores in
// metadata, or returns payload contents. It records only structural metadata,
// hashes, counts, reviewed operation IDs, and bounded native runner summaries.
package nativeexport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/shpoont/dotfiles-manager/internal/v2/nativeops"
	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/shpoont/dotfiles-manager/internal/v2/resolution"
)

const (
	MetadataSchema  = "dotfiles-manager.v2.native-export"
	SchemaVersion   = 1
	DriverVersion   = "native-export.v1"
	Normalizer      = "native-export.payload-tree.v1"
	MetadataFile    = "metadata.json"
	PayloadDir      = "payload"
	ReviewCode      = "nativeexport.review.required"
	StatusSucceeded = "succeeded"
	StatusBlocked   = "blocked"
)

type Options struct {
	RepoRoot           string
	StateRoot          string
	Recipe             *recipe.Recipe
	RecipeSource       string
	TrustEvaluation    *recipe.TrustEvaluation
	Setting            resolution.ResolvedSetting
	ResourceID         string
	Resource           recipe.Resource
	MachineID          string
	UserID             string
	RunID              string
	LocationRoots      map[string]string
	Now                func() time.Time
	ExecutableResolver nativeops.ExecutableResolver
	Executor           nativeops.Executor
}

type Result struct {
	Status      string
	StagingRoot string
	PayloadRoot string
	Metadata    Metadata
	Native      nativeops.Result
	Diagnostic  Diagnostic
}

type DesiredRead struct {
	Status     string
	Metadata   *Metadata
	Diagnostic Diagnostic
}

type Diagnostic struct {
	Code    string
	Message string
	Path    string
}

type Metadata struct {
	Schema        string            `json:"schema"`
	SchemaVersion int               `json:"schemaVersion"`
	TargetRef     string            `json:"targetRef"`
	SettingRef    string            `json:"settingRef"`
	ResourceID    string            `json:"resourceId"`
	OperationID   string            `json:"operationId"`
	Recipe        RecipeMetadata    `json:"recipe"`
	Operation     OperationMetadata `json:"operation"`
	Source        SourceMetadata    `json:"source"`
	CapturedAt    string            `json:"capturedAt"`
	Payload       PayloadSummary    `json:"payload"`
	Native        NativeRunMetadata `json:"native"`
	Exclusions    ExclusionMetadata `json:"exclusions"`
	Limitations   []string          `json:"limitations,omitempty"`
}

type RecipeMetadata struct {
	Source      string `json:"source"`
	TrustStatus string `json:"trustStatus"`
}

type OperationMetadata struct {
	ArtifactForm string   `json:"artifactForm"`
	DiffMode     string   `json:"diffMode"`
	Redaction    string   `json:"redaction"`
	OutputIDs    []string `json:"outputIds"`
}

type SourceMetadata struct {
	Scope     string `json:"scope"`
	Subject   string `json:"subject"`
	MachineID string `json:"machineId,omitempty"`
	UserID    string `json:"userId,omitempty"`
}

type PayloadSummary struct {
	Exists     bool   `json:"exists"`
	SHA256     string `json:"sha256,omitempty"`
	Normalizer string `json:"normalizer,omitempty"`
	Size       int64  `json:"size,omitempty"`
	EntryCount int    `json:"entryCount,omitempty"`
	FileCount  int    `json:"fileCount,omitempty"`
	DirCount   int    `json:"dirCount,omitempty"`
}

type NativeRunMetadata struct {
	Status         string                   `json:"status"`
	DurationMillis int64                    `json:"durationMillis"`
	TimeoutMillis  int64                    `json:"timeoutMillis"`
	CommandSummary string                   `json:"commandSummary"`
	Stdout         nativeops.CaptureSummary `json:"stdout"`
	Stderr         nativeops.CaptureSummary `json:"stderr"`
}

type ExclusionMetadata struct {
	Declared           bool     `json:"declared"`
	CapturedCategories []string `json:"capturedCategories,omitempty"`
	SecretExclusions   []string `json:"secretExclusions,omitempty"`
	AccountExclusions  []string `json:"accountExclusions,omitempty"`
}

type Limits struct {
	MaxBytes   int64
	MaxEntries int
}

func ReviewRequired(op recipe.NativeOperation) bool {
	return op.Review.Required
}

func ReviewDiagnostic(settingRef string, op recipe.NativeOperation) Diagnostic {
	message := strings.TrimSpace(op.Review.Message)
	if message == "" {
		message = "native export requires explicit confirmation before executing the reviewed export operation"
	}
	if len(op.Review.Reasons) > 0 {
		message += " (" + strings.Join(op.Review.Reasons, ", ") + ")"
	}
	return Diagnostic{Code: ReviewCode, Message: message, Path: settingRef}
}

func Export(ctx context.Context, opts Options) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	op, err := exportOperation(opts)
	if err != nil {
		return blockedResult(opts, Diagnostic{Code: "nativeexport.operation.invalid", Message: err.Error(), Path: opts.Setting.Ref()}), err
	}
	stagingRoot, payloadRoot, tempRoot, err := createStaging(opts)
	if err != nil {
		diag := Diagnostic{Code: "nativeexport.staging.create", Message: "native export staging directory could not be created", Path: opts.Setting.Ref()}
		return blockedResult(opts, diag), err
	}
	nativeResult := nativeops.Run(ctx, nativeops.Options{
		Recipe:             opts.Recipe,
		RecipeSource:       opts.RecipeSource,
		TrustEvaluation:    opts.TrustEvaluation,
		OperationID:        opts.Resource.NativeOperation,
		RepoRoot:           opts.RepoRoot,
		ArtifactRoot:       payloadRoot,
		TempRoot:           tempRoot,
		LocationRoots:      opts.LocationRoots,
		ExecutableResolver: opts.ExecutableResolver,
		Executor:           opts.Executor,
	})
	if nativeResult.Status != nativeops.StatusSucceeded {
		diag := firstNativeDiagnostic(nativeResult, "native export operation did not complete successfully")
		return Result{Status: StatusBlocked, StagingRoot: stagingRoot, PayloadRoot: payloadRoot, Native: nativeResult, Diagnostic: diag}, errors.New(diag.Message)
	}
	if err := requireArtifactOutputs(payloadRoot, op); err != nil {
		diag := Diagnostic{Code: "nativeexport.outputs.missing", Message: err.Error(), Path: opts.Setting.Ref()}
		return Result{Status: StatusBlocked, StagingRoot: stagingRoot, PayloadRoot: payloadRoot, Native: nativeResult, Diagnostic: diag}, err
	}
	payload, err := SummarizePayload(payloadRoot, EffectiveLimits(op))
	if err != nil {
		diag := payloadDiagnostic(err, opts.Setting.Ref())
		return Result{Status: StatusBlocked, StagingRoot: stagingRoot, PayloadRoot: payloadRoot, Native: nativeResult, Diagnostic: diag}, err
	}
	metadata := buildMetadata(opts, op, payload, nativeResult)
	if err := WriteMetadata(stagingRoot, metadata); err != nil {
		diag := Diagnostic{Code: "nativeexport.metadata.write", Message: "native export metadata could not be written", Path: opts.Setting.Ref()}
		return Result{Status: StatusBlocked, StagingRoot: stagingRoot, PayloadRoot: payloadRoot, Metadata: metadata, Native: nativeResult, Diagnostic: diag}, err
	}
	return Result{Status: StatusSucceeded, StagingRoot: stagingRoot, PayloadRoot: payloadRoot, Metadata: metadata, Native: nativeResult}, nil
}

func EffectiveLimits(op recipe.NativeOperation) Limits {
	limits := Limits{MaxBytes: int64(recipe.MaxNativeExportBytes), MaxEntries: recipe.MaxNativeExportEntries}
	if op.Limits.MaxBytes > 0 && op.Limits.MaxBytes < limits.MaxBytes {
		limits.MaxBytes = op.Limits.MaxBytes
	}
	if op.Limits.MaxEntries > 0 && op.Limits.MaxEntries < limits.MaxEntries {
		limits.MaxEntries = op.Limits.MaxEntries
	}
	return limits
}

func SummarizePayload(root string, limits Limits) (PayloadSummary, error) {
	cleanRoot, err := cleanAbs(root)
	if err != nil {
		return PayloadSummary{}, err
	}
	info, err := os.Lstat(cleanRoot)
	if err != nil {
		return PayloadSummary{}, err
	}
	if !info.IsDir() {
		return PayloadSummary{}, fmt.Errorf("native export payload root must be a directory")
	}
	var entries []treeEntry
	summary := PayloadSummary{Exists: true, Normalizer: Normalizer}
	err = filepath.WalkDir(cleanRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == cleanRoot {
			return nil
		}
		rel, err := filepath.Rel(cleanRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("native export payload contains unsupported symlink: %s", rel)
		}
		if info.Mode().IsRegular() {
			summary.FileCount++
			summary.EntryCount++
			summary.Size += info.Size()
			if summary.Size > limits.MaxBytes {
				return fmt.Errorf("native export payload exceeds maxBytes")
			}
			if summary.EntryCount > limits.MaxEntries {
				return fmt.Errorf("native export payload exceeds maxEntries")
			}
			hash, err := fileSHA256(path)
			if err != nil {
				return err
			}
			entries = append(entries, treeEntry{Kind: "file", Path: rel, Size: info.Size(), Hash: hash})
			return nil
		}
		if info.IsDir() {
			summary.DirCount++
			summary.EntryCount++
			if summary.EntryCount > limits.MaxEntries {
				return fmt.Errorf("native export payload exceeds maxEntries")
			}
			entries = append(entries, treeEntry{Kind: "dir", Path: rel})
			return nil
		}
		return fmt.Errorf("native export payload contains unsupported file type: %s", rel)
	})
	if err != nil {
		return PayloadSummary{}, err
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Path+"\x00"+entries[i].Kind < entries[j].Path+"\x00"+entries[j].Kind
	})
	sum := sha256.New()
	for _, entry := range entries {
		_, _ = fmt.Fprintf(sum, "%s\x00%s\x00%d\x00%s\n", entry.Kind, entry.Path, entry.Size, entry.Hash)
	}
	summary.SHA256 = hex.EncodeToString(sum.Sum(nil))
	return summary, nil
}

func WriteMetadata(stagingRoot string, metadata Metadata) error {
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeFileAtomic(filepath.Join(stagingRoot, MetadataFile), data, 0o644)
}

func ReadDesired(path string, expected ExpectedIdentity) DesiredRead {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return DesiredRead{Status: "invalid", Diagnostic: Diagnostic{Code: "nativeexport.desired.pathInvalid", Message: "desired native export path is required"}}
	}
	info, err := os.Lstat(trimmed)
	if errors.Is(err, os.ErrNotExist) {
		return DesiredRead{Status: "missing"}
	}
	if err != nil {
		return DesiredRead{Status: "blocked", Diagnostic: Diagnostic{Code: "nativeexport.desired.read", Message: "desired native export artifact could not be read", Path: filepath.ToSlash(trimmed)}}
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return DesiredRead{Status: "blocked", Diagnostic: Diagnostic{Code: "nativeexport.desired.untrusted", Message: "existing desired native export path is not a manager-owned artifact directory", Path: filepath.ToSlash(trimmed)}}
	}
	metadata, err := readMetadata(filepath.Join(trimmed, MetadataFile))
	if err != nil {
		return DesiredRead{Status: "blocked", Diagnostic: Diagnostic{Code: "nativeexport.desired.metadataInvalid", Message: "existing desired native export metadata is missing or invalid", Path: filepath.ToSlash(filepath.Join(trimmed, MetadataFile))}}
	}
	if err := expected.Matches(metadata); err != nil {
		return DesiredRead{Status: "blocked", Diagnostic: Diagnostic{Code: "nativeexport.desired.metadataMismatch", Message: err.Error(), Path: filepath.ToSlash(filepath.Join(trimmed, MetadataFile))}}
	}
	return DesiredRead{Status: "present", Metadata: &metadata}
}

type ExpectedIdentity struct {
	TargetRef    string
	SettingRef   string
	ResourceID   string
	OperationID  string
	ArtifactForm string
}

func Expected(opts Options) ExpectedIdentity {
	op := recipe.NativeOperation{}
	if opts.Recipe != nil {
		op = opts.Recipe.NativeOperations[opts.Resource.NativeOperation]
	}
	return ExpectedIdentity{TargetRef: opts.Setting.TargetID, SettingRef: opts.Setting.Ref(), ResourceID: opts.ResourceID, OperationID: opts.Resource.NativeOperation, ArtifactForm: op.ArtifactForm}
}

func (e ExpectedIdentity) Matches(metadata Metadata) error {
	switch {
	case metadata.Schema != MetadataSchema || metadata.SchemaVersion != SchemaVersion:
		return fmt.Errorf("existing desired native export metadata schema does not match")
	case metadata.TargetRef != e.TargetRef:
		return fmt.Errorf("existing desired native export target does not match")
	case metadata.SettingRef != e.SettingRef:
		return fmt.Errorf("existing desired native export setting does not match")
	case metadata.ResourceID != e.ResourceID:
		return fmt.Errorf("existing desired native export resource does not match")
	case metadata.OperationID != e.OperationID:
		return fmt.Errorf("existing desired native export operation does not match")
	case metadata.Operation.ArtifactForm != e.ArtifactForm:
		return fmt.Errorf("existing desired native export artifact form does not match")
	default:
		return nil
	}
}

func WriteDesired(desiredPath string, stagingRoot string, expected ExpectedIdentity) error {
	cleanDesiredPath, err := cleanAbs(desiredPath)
	if err != nil {
		return err
	}
	desiredPath = cleanDesiredPath
	if read := ReadDesired(desiredPath, expected); read.Status != "missing" && read.Status != "present" {
		return fmt.Errorf(read.Diagnostic.Message)
	}
	metadata, err := readMetadata(filepath.Join(stagingRoot, MetadataFile))
	if err != nil {
		return fmt.Errorf("staged native export metadata is invalid")
	}
	if err := expected.Matches(metadata); err != nil {
		return err
	}
	if _, err := SummarizePayload(filepath.Join(stagingRoot, PayloadDir), Limits{MaxBytes: int64(recipe.MaxNativeExportBytes), MaxEntries: recipe.MaxNativeExportEntries}); err != nil {
		return err
	}
	parent := filepath.Dir(desiredPath)
	base := filepath.Base(desiredPath)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	if err := validateNoSymlinkParents(filepath.Dir(parent), desiredPath); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(parent, "."+base+".tmp-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if err := copyTree(stagingRoot, tmp); err != nil {
		return err
	}
	if err := validateNoSymlinkParents(parent, desiredPath); err != nil {
		return err
	}
	if _, err := os.Lstat(desiredPath); errors.Is(err, os.ErrNotExist) {
		return os.Rename(tmp, desiredPath)
	} else if err != nil {
		return err
	}
	backup := filepath.Join(parent, "."+base+".old-"+fmt.Sprintf("%d", time.Now().UnixNano()))
	if err := os.Rename(desiredPath, backup); err != nil {
		return err
	}
	if err := os.Rename(tmp, desiredPath); err != nil {
		_ = os.Rename(backup, desiredPath)
		return err
	}
	return os.RemoveAll(backup)
}

func Snapshot(metadata *Metadata) PayloadSummary {
	if metadata == nil {
		return PayloadSummary{}
	}
	return metadata.Payload
}

func ChangeKind(current PayloadSummary, desired PayloadSummary) string {
	switch {
	case current.Exists && !desired.Exists:
		return "create"
	case !current.Exists && desired.Exists:
		return "delete"
	case !current.Exists && !desired.Exists:
		return "unchanged"
	case current.SHA256 == desired.SHA256 && current.Normalizer == desired.Normalizer:
		return "unchanged"
	default:
		return "update"
	}
}

type treeEntry struct {
	Kind string
	Path string
	Size int64
	Hash string
}

func exportOperation(opts Options) (recipe.NativeOperation, error) {
	if opts.Recipe == nil {
		return recipe.NativeOperation{}, fmt.Errorf("recipe is required")
	}
	if opts.Resource.Driver != recipe.NativeExportDriverID {
		return recipe.NativeOperation{}, fmt.Errorf("resource driver is not native-export")
	}
	op, ok := opts.Recipe.NativeOperations[opts.Resource.NativeOperation]
	if !ok {
		return recipe.NativeOperation{}, fmt.Errorf("native export operation is not declared")
	}
	if op.Kind != "export" {
		return recipe.NativeOperation{}, fmt.Errorf("native export operation must be kind export")
	}
	return op, nil
}

func createStaging(opts Options) (string, string, string, error) {
	stateRoot, err := cleanAbs(opts.StateRoot)
	if err != nil {
		return "", "", "", err
	}
	root := filepath.Join(stateRoot, "temp", "native-export")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", "", "", err
	}
	prefix := safePrefix(defaultString(opts.RunID, "run") + "-" + opts.Setting.TargetID + "-" + opts.Setting.SettingID)
	stagingRoot, err := os.MkdirTemp(root, prefix+"-*")
	if err != nil {
		return "", "", "", err
	}
	payloadRoot := filepath.Join(stagingRoot, PayloadDir)
	tempRoot := filepath.Join(stagingRoot, "work")
	if err := os.MkdirAll(payloadRoot, 0o700); err != nil {
		return "", "", "", err
	}
	if err := os.MkdirAll(tempRoot, 0o700); err != nil {
		return "", "", "", err
	}
	return stagingRoot, payloadRoot, tempRoot, nil
}

func requireArtifactOutputs(payloadRoot string, op recipe.NativeOperation) error {
	artifactOutputs := 0
	for _, id := range sortedKeys(op.Outputs) {
		spec := op.Outputs[id]
		if spec.Root != "artifact" {
			continue
		}
		artifactOutputs++
		if _, err := os.Lstat(filepath.Join(payloadRoot, filepath.FromSlash(spec.Path))); err != nil {
			return fmt.Errorf("native export declared artifact output %s was not produced", id)
		}
	}
	if artifactOutputs == 0 {
		return fmt.Errorf("native export operation must declare at least one artifact output")
	}
	return nil
}

func buildMetadata(opts Options, op recipe.NativeOperation, payload PayloadSummary, native nativeops.Result) Metadata {
	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}
	exclusions := ExclusionMetadata{
		CapturedCategories: append([]string(nil), op.ExportMetadata.CapturedCategories...),
		SecretExclusions:   append([]string(nil), op.ExportMetadata.SecretExclusions...),
		AccountExclusions:  append([]string(nil), op.ExportMetadata.AccountExclusions...),
	}
	exclusions.Declared = len(exclusions.CapturedCategories)+len(exclusions.SecretExclusions)+len(exclusions.AccountExclusions) > 0
	trustStatus := ""
	if opts.TrustEvaluation != nil {
		trustStatus = opts.TrustEvaluation.Status
	}
	return Metadata{
		Schema:        MetadataSchema,
		SchemaVersion: SchemaVersion,
		TargetRef:     opts.Setting.TargetID,
		SettingRef:    opts.Setting.Ref(),
		ResourceID:    opts.ResourceID,
		OperationID:   opts.Resource.NativeOperation,
		Recipe:        RecipeMetadata{Source: opts.RecipeSource, TrustStatus: trustStatus},
		Operation: OperationMetadata{
			ArtifactForm: op.ArtifactForm,
			DiffMode:     op.DiffMode,
			Redaction:    op.Redaction,
			OutputIDs:    sortedKeys(op.Outputs),
		},
		Source:      SourceMetadata{Scope: opts.Setting.Scope, Subject: opts.Setting.Subject, MachineID: opts.MachineID, UserID: opts.UserID},
		CapturedAt:  now().UTC().Format(time.RFC3339),
		Payload:     payload,
		Native:      NativeRunMetadata{Status: native.Status, DurationMillis: native.DurationMillis, TimeoutMillis: native.TimeoutMillis, CommandSummary: native.CommandSummary, Stdout: native.Stdout, Stderr: native.Stderr},
		Exclusions:  exclusions,
		Limitations: append([]string(nil), op.ExportMetadata.Limitations...),
	}
}

func readMetadata(path string) (Metadata, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return Metadata{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return Metadata{}, fmt.Errorf("native export metadata must be a regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Metadata{}, err
	}
	var metadata Metadata
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&metadata); err != nil {
		return Metadata{}, err
	}
	return metadata, nil
}

func blockedResult(opts Options, diagnostic Diagnostic) Result {
	return Result{Status: StatusBlocked, Diagnostic: diagnostic, Metadata: Metadata{TargetRef: opts.Setting.TargetID, SettingRef: opts.Setting.Ref(), ResourceID: opts.ResourceID, OperationID: opts.Resource.NativeOperation}}
}

func firstNativeDiagnostic(result nativeops.Result, fallback string) Diagnostic {
	if len(result.Diagnostics) > 0 {
		diag := result.Diagnostics[0]
		return Diagnostic{Code: diag.Code, Message: diag.Message, Path: diag.OperationID}
	}
	return Diagnostic{Code: "nativeexport.operation.failed", Message: fallback}
}

func payloadDiagnostic(err error, ref string) Diagnostic {
	code := "nativeexport.payload.invalid"
	message := "native export payload is invalid"
	if err != nil {
		text := err.Error()
		switch {
		case strings.Contains(text, "maxBytes"):
			code = "nativeexport.payload.maxBytes"
			message = "native export payload exceeds declared maxBytes"
		case strings.Contains(text, "maxEntries"):
			code = "nativeexport.payload.maxEntries"
			message = "native export payload exceeds declared maxEntries"
		case strings.Contains(text, "symlink"), strings.Contains(text, "unsupported file type"):
			code = "nativeexport.payload.unsupportedFileType"
			message = "native export payload contains unsupported file types"
		}
	}
	return Diagnostic{Code: code, Message: message, Path: ref}
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	sum := sha256.New()
	if _, err := io.Copy(sum, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".metadata.tmp-*")
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
	if err := os.Chmod(tmpPath, perm); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func copyTree(src string, dst string) error {
	src, err := cleanAbs(src)
	if err != nil {
		return err
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == src {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("native export copy refuses symlink")
		}
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("native export copy refuses unsupported file")
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			_ = in.Close()
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			_ = in.Close()
			_ = out.Close()
			return err
		}
		if err := in.Close(); err != nil {
			_ = out.Close()
			return err
		}
		return out.Close()
	})
}

func validateNoSymlinkParents(root string, target string) error {
	rootAbs, err := cleanAbs(root)
	if err != nil {
		return err
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil {
		return err
	}
	if rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return fmt.Errorf("target escapes replacement root")
	}
	return validateNoSymlinkPath(targetAbs)
}

func validateNoSymlinkPath(targetAbs string) error {
	cleanTarget, err := cleanAbs(targetAbs)
	if err != nil {
		return err
	}
	volume := filepath.VolumeName(cleanTarget)
	rest := strings.TrimPrefix(cleanTarget, volume)
	current := volume
	separator := string(filepath.Separator)
	if strings.HasPrefix(rest, separator) {
		current += separator
		rest = strings.TrimPrefix(rest, separator)
	}
	if current == "" {
		current = separator
	}
	for _, part := range strings.Split(rest, separator) {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("native export desired path uses a symlink")
		}
	}
	return nil
}

func cleanAbs(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", fmt.Errorf("path is required")
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", err
	}
	if filepath.Clean(abs) != abs {
		return "", fmt.Errorf("path must be clean")
	}
	return abs, nil
}

func safePrefix(value string) string {
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('-')
	}
	out := strings.Trim(b.String(), ".-")
	if out == "" {
		return "native-export"
	}
	if len(out) > 80 {
		return out[:80]
	}
	return out
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
