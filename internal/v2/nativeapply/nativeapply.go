// Package nativeapply plans and executes reviewed native import/apply flows.
//
// The package deliberately keeps native apply metadata-only. It validates
// manager-owned native export desired artifacts, copies desired payloads into
// manager-owned temp input roots before import, and never exposes payload
// content, argv, executable paths, account IDs, or temp paths.
package nativeapply

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shpoont/dotfiles-manager/internal/v2/nativeexport"
	"github.com/shpoont/dotfiles-manager/internal/v2/nativeops"
	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/shpoont/dotfiles-manager/internal/v2/resolution"
)

const (
	BackupPreApplyExport       = "pre-apply-export"
	VerifyPostImportExportHash = "post-import-export-hash"

	StatusReady     = "ready"
	StatusBlocked   = "blocked"
	StatusSucceeded = "succeeded"

	ReviewCode = "nativeapply.review.required"
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

type Plan struct {
	Status            string
	Expected          nativeexport.ExpectedIdentity
	DesiredMetadata   nativeexport.Metadata
	DesiredSummary    nativeexport.PayloadSummary
	ExportOperationID string
	ImportOperationID string
	VerifyOperationID string
	BackupPolicy      string
	VerifyPolicy      string
	ArtifactForm      string
	DiffMode          string
	Redaction         string
	ReviewRequired    bool
	Limitations       []string
	Diagnostic        Diagnostic
}

type PreparedInput struct {
	Root    string
	Payload string
	Summary nativeexport.PayloadSummary
}

type Diagnostic struct {
	Code    string
	Message string
	Path    string
}

func ImportCapable(resource recipe.Resource) bool {
	return strings.TrimSpace(resource.NativeImportOperation) != "" || resource.NativeApply.Backup != "" || resource.NativeApply.Verify != ""
}

func BuildPlan(opts Options) (Plan, error) {
	exportOp, importOp, verifyOp, err := operations(opts)
	if err != nil {
		diag := Diagnostic{Code: "nativeapply.operation.invalid", Message: err.Error(), Path: opts.Setting.Ref()}
		return Plan{Status: StatusBlocked, Diagnostic: diag}, err
	}
	plan := Plan{
		Status:            StatusReady,
		Expected:          nativeexport.Expected(opts.nativeExportOptions()),
		ExportOperationID: opts.Resource.NativeOperation,
		ImportOperationID: opts.Resource.NativeImportOperation,
		VerifyOperationID: opts.Resource.NativeVerifyOperation,
		BackupPolicy:      opts.Resource.NativeApply.Backup,
		VerifyPolicy:      opts.Resource.NativeApply.Verify,
		ArtifactForm:      exportOp.ArtifactForm,
		DiffMode:          exportOp.DiffMode,
		Redaction:         exportOp.Redaction,
		ReviewRequired:    nativeexport.ReviewRequired(exportOp) || nativeexport.ReviewRequired(importOp),
		Limitations:       append([]string(nil), exportOp.ExportMetadata.Limitations...),
	}
	if verifyOp.Kind != "" {
		plan.ReviewRequired = plan.ReviewRequired || nativeexport.ReviewRequired(verifyOp)
	}
	if err := validatePolicy(opts, exportOp, importOp, verifyOp); err != nil {
		diag := policyDiagnostic(err, opts.Setting.Ref())
		plan.Status = StatusBlocked
		plan.Diagnostic = diag
		return plan, err
	}
	desiredRead := nativeexport.ReadDesired(opts.Setting.DesiredPath, plan.Expected)
	if desiredRead.Status != "present" || desiredRead.Metadata == nil {
		diag := desiredDiagnostic(desiredRead, opts.Setting)
		plan.Status = StatusBlocked
		plan.Diagnostic = diag
		return plan, errors.New(diag.Message)
	}
	payloadRoot := filepath.Join(opts.Setting.DesiredPath, nativeexport.PayloadDir)
	summary, err := nativeexport.ValidatePayload(payloadRoot, desiredRead.Metadata.Payload, nativeexport.EffectiveLimits(exportOp))
	if err != nil {
		diag := payloadDiagnostic(err, opts.Setting.Ref())
		plan.Status = StatusBlocked
		plan.Diagnostic = diag
		return plan, err
	}
	plan.DesiredMetadata = *desiredRead.Metadata
	plan.DesiredSummary = summary
	return plan, nil
}

func ReviewDiagnostic(settingRef string, plan Plan) Diagnostic {
	message := "native apply requires explicit confirmation before backup, import, and verification operations run"
	if plan.BackupPolicy != "" || plan.VerifyPolicy != "" {
		message += " (backup: " + defaultString(plan.BackupPolicy, "missing") + ", verify: " + defaultString(plan.VerifyPolicy, "missing") + ")"
	}
	return Diagnostic{Code: ReviewCode, Message: message, Path: settingRef}
}

func PrepareDesiredInput(opts Options, plan Plan) (PreparedInput, error) {
	stateRoot, err := cleanAbs(opts.StateRoot)
	if err != nil {
		return PreparedInput{}, err
	}
	root := filepath.Join(stateRoot, "temp", "native-apply")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return PreparedInput{}, err
	}
	prefix := safePrefix(defaultString(opts.RunID, "run") + "-" + opts.Setting.TargetID + "-" + opts.Setting.SettingID)
	inputRoot, err := os.MkdirTemp(root, prefix+"-input-*")
	if err != nil {
		return PreparedInput{}, err
	}
	payloadRoot := filepath.Join(inputRoot, nativeexport.PayloadDir)
	sourcePayload := filepath.Join(opts.Setting.DesiredPath, nativeexport.PayloadDir)
	if err := nativeexport.CopyPayload(sourcePayload, payloadRoot); err != nil {
		_ = os.RemoveAll(inputRoot)
		return PreparedInput{}, err
	}
	summary, err := nativeexport.ValidatePayload(payloadRoot, plan.DesiredSummary, nativeexport.Limits{MaxBytes: int64(recipe.MaxNativeExportBytes), MaxEntries: recipe.MaxNativeExportEntries})
	if err != nil {
		_ = os.RemoveAll(inputRoot)
		return PreparedInput{}, err
	}
	return PreparedInput{Root: inputRoot, Payload: payloadRoot, Summary: summary}, nil
}

func RunImport(ctx context.Context, opts Options, plan Plan, input PreparedInput) (nativeops.Result, error) {
	tempRoot, err := createTempRoot(opts, "import")
	if err != nil {
		return nativeops.Result{}, err
	}
	result := nativeops.Run(ctx, nativeops.Options{
		Recipe:             opts.Recipe,
		RecipeSource:       opts.RecipeSource,
		TrustEvaluation:    opts.TrustEvaluation,
		OperationID:        plan.ImportOperationID,
		RepoRoot:           opts.RepoRoot,
		ArtifactRoot:       input.Payload,
		TempRoot:           tempRoot,
		LocationRoots:      nil,
		ExecutableResolver: opts.ExecutableResolver,
		Executor:           opts.Executor,
	})
	if result.Status != nativeops.StatusSucceeded {
		return result, errors.New(firstNativeMessage(result, "native import operation did not complete successfully"))
	}
	return result, nil
}

func VerifyPostImport(desired nativeexport.PayloadSummary, current nativeexport.PayloadSummary) error {
	if !desired.Exists || desired.SHA256 == "" {
		return fmt.Errorf("desired native export payload summary is missing")
	}
	if !current.Exists || current.SHA256 == "" {
		return fmt.Errorf("post-import native export payload summary is missing")
	}
	if current.SHA256 != desired.SHA256 || current.Normalizer != desired.Normalizer {
		return fmt.Errorf("post-import native export hash does not match desired artifact")
	}
	return nil
}

func (opts Options) nativeExportOptions() nativeexport.Options {
	return nativeexport.Options{
		RepoRoot:           opts.RepoRoot,
		StateRoot:          opts.StateRoot,
		Recipe:             opts.Recipe,
		RecipeSource:       opts.RecipeSource,
		TrustEvaluation:    opts.TrustEvaluation,
		Setting:            opts.Setting,
		ResourceID:         opts.ResourceID,
		Resource:           opts.Resource,
		MachineID:          opts.MachineID,
		UserID:             opts.UserID,
		RunID:              opts.RunID,
		LocationRoots:      opts.LocationRoots,
		Now:                opts.Now,
		ExecutableResolver: opts.ExecutableResolver,
		Executor:           opts.Executor,
	}
}

func operations(opts Options) (recipe.NativeOperation, recipe.NativeOperation, recipe.NativeOperation, error) {
	if opts.Recipe == nil {
		return recipe.NativeOperation{}, recipe.NativeOperation{}, recipe.NativeOperation{}, fmt.Errorf("recipe is required")
	}
	if opts.Resource.Driver != recipe.NativeExportDriverID {
		return recipe.NativeOperation{}, recipe.NativeOperation{}, recipe.NativeOperation{}, fmt.Errorf("resource driver is not native-export")
	}
	exportOp, ok := opts.Recipe.NativeOperations[opts.Resource.NativeOperation]
	if !ok || exportOp.Kind != "export" {
		return recipe.NativeOperation{}, recipe.NativeOperation{}, recipe.NativeOperation{}, fmt.Errorf("native apply requires a reviewed export operation")
	}
	importID := strings.TrimSpace(opts.Resource.NativeImportOperation)
	if importID == "" {
		return recipe.NativeOperation{}, recipe.NativeOperation{}, recipe.NativeOperation{}, fmt.Errorf("native apply requires nativeImportOperation")
	}
	importOp, ok := opts.Recipe.NativeOperations[importID]
	if !ok || importOp.Kind != "import" {
		return recipe.NativeOperation{}, recipe.NativeOperation{}, recipe.NativeOperation{}, fmt.Errorf("nativeImportOperation must reference a declared import operation")
	}
	var verifyOp recipe.NativeOperation
	verifyID := strings.TrimSpace(opts.Resource.NativeVerifyOperation)
	if verifyID != "" {
		var ok bool
		verifyOp, ok = opts.Recipe.NativeOperations[verifyID]
		if !ok || verifyOp.Kind != "verify" {
			return recipe.NativeOperation{}, recipe.NativeOperation{}, recipe.NativeOperation{}, fmt.Errorf("nativeVerifyOperation must reference a declared verify operation")
		}
	}
	return exportOp, importOp, verifyOp, nil
}

func validatePolicy(opts Options, exportOp recipe.NativeOperation, importOp recipe.NativeOperation, verifyOp recipe.NativeOperation) error {
	if opts.Resource.NativeApply.Backup != BackupPreApplyExport {
		return fmt.Errorf("native apply backup policy must be %s", BackupPreApplyExport)
	}
	if opts.Resource.NativeApply.Verify != VerifyPostImportExportHash {
		return fmt.Errorf("native apply verify policy must be %s", VerifyPostImportExportHash)
	}
	for _, lifecycle := range []string{opts.Resource.Lifecycle, exportOp.Lifecycle, importOp.Lifecycle, verifyOp.Lifecycle} {
		if lifecycle != "" && lifecycle != recipe.LifecycleAllowed {
			return fmt.Errorf("native apply lifecycle policy %s is blocked until lifecycle handling is implemented", lifecycle)
		}
	}
	return nil
}

func createTempRoot(opts Options, purpose string) (string, error) {
	stateRoot, err := cleanAbs(opts.StateRoot)
	if err != nil {
		return "", err
	}
	root := filepath.Join(stateRoot, "temp", "native-apply")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", err
	}
	prefix := safePrefix(defaultString(opts.RunID, "run") + "-" + opts.Setting.TargetID + "-" + opts.Setting.SettingID + "-" + purpose)
	return os.MkdirTemp(root, prefix+"-*")
}

func desiredDiagnostic(read nativeexport.DesiredRead, setting resolution.ResolvedSetting) Diagnostic {
	switch read.Status {
	case "missing":
		return Diagnostic{Code: "nativeapply.desired.missing", Message: "desired native export artifact is missing; native apply cannot run", Path: setting.DesiredRelPath}
	case "blocked", "invalid":
		return Diagnostic{Code: read.Diagnostic.Code, Message: read.Diagnostic.Message, Path: read.Diagnostic.Path}
	default:
		return Diagnostic{Code: "nativeapply.desired.invalid", Message: "desired native export artifact is invalid", Path: setting.DesiredRelPath}
	}
}

func policyDiagnostic(err error, ref string) Diagnostic {
	code := "nativeapply.policy.invalid"
	message := "native apply policy is invalid"
	if err != nil {
		message = err.Error()
		switch {
		case strings.Contains(message, "backup"):
			code = "nativeapply.backup.policyUnsupported"
		case strings.Contains(message, "verify"):
			code = "nativeapply.verify.policyUnsupported"
		case strings.Contains(message, "lifecycle"):
			code = "nativeapply.lifecycle.blocked"
		case strings.Contains(message, "nativeImportOperation"):
			code = "nativeapply.importOperation.required"
		}
	}
	return Diagnostic{Code: code, Message: message, Path: ref}
}

func payloadDiagnostic(err error, ref string) Diagnostic {
	code := "nativeapply.payload.invalid"
	message := "desired native export payload is invalid"
	if err != nil {
		message = err.Error()
		switch {
		case strings.Contains(message, "hash"):
			code = "nativeapply.payload.hashMismatch"
		case strings.Contains(message, "maxBytes"):
			code = "nativeapply.payload.maxBytes"
		case strings.Contains(message, "maxEntries"):
			code = "nativeapply.payload.maxEntries"
		case strings.Contains(message, "symlink"), strings.Contains(message, "unsupported"):
			code = "nativeapply.payload.unsupportedFileType"
		}
	}
	return Diagnostic{Code: code, Message: message, Path: ref}
}

func firstNativeMessage(result nativeops.Result, fallback string) string {
	if len(result.Diagnostics) > 0 && strings.TrimSpace(result.Diagnostics[0].Message) != "" {
		return result.Diagnostics[0].Message
	}
	return fallback
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
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-_.")
	if out == "" {
		return "native-apply"
	}
	return out
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
