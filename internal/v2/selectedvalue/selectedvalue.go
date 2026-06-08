package selectedvalue

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shpoont/dotfiles-manager/internal/v2/filedriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/inidriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/jsondriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/plistdriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/shpoont/dotfiles-manager/internal/v2/tomldriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/yamldriver"
)

const (
	StatusOK      = "ok"
	StatusBlocked = "blocked"
	SeverityError = "error"
)

const (
	IntentSet    = "set"
	IntentDelete = "delete"
)

type Request struct {
	Recipe        *recipe.Recipe
	SettingRef    string
	LocationRoots map[string]string
}

type PreviewRequest struct {
	Request
	Desired            Desired
	WriteSafetyContext recipe.WriteSafetyContext
}

type Desired struct {
	intent string
	kind   string
	value  any
}

func SetString(value string) Desired {
	return Desired{intent: IntentSet, kind: "string", value: value}
}

func SetBool(value bool) Desired {
	return Desired{intent: IntentSet, kind: "bool", value: value}
}

func SetNumber(value json.Number) Desired {
	return Desired{intent: IntentSet, kind: "number", value: value}
}

func SetNull() Desired {
	return Desired{intent: IntentSet, kind: "null", value: nil}
}

func Delete() Desired {
	return Desired{intent: IntentDelete}
}

func (d Desired) Intent() string { return d.intent }
func (d Desired) Kind() string   { return d.kind }

func (d Desired) Value() (any, bool) {
	if d.intent != IntentSet {
		return nil, false
	}
	return d.value, true
}

func (d Desired) String() string {
	return fmt.Sprintf("Desired{intent:%s kind:%s value:<redacted>}", defaultString(d.intent, "<unset>"), defaultString(d.kind, "<none>"))
}

func (d Desired) GoString() string {
	return d.String()
}

func (d Desired) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]string{"intent": d.intent, "kind": d.kind})
}

type Plan struct {
	TargetRef    string       `json:"targetRef"`
	SettingRef   string       `json:"settingRef"`
	SettingID    string       `json:"settingId"`
	ScopeDefault string       `json:"scopeDefault,omitempty"`
	ResourceID   string       `json:"resourceId"`
	DriverID     string       `json:"driverId"`
	LocationID   string       `json:"locationId"`
	RelPath      string       `json:"relPath"`
	Path         string       `json:"path"`
	Format       string       `json:"format,omitempty"`
	Selector     SelectorInfo `json:"selector"`
	Current      Snapshot     `json:"current"`
	Desired      *Snapshot    `json:"desired,omitempty"`
	ChangeKind   string       `json:"changeKind,omitempty"`
	Intent       string       `json:"intent,omitempty"`
	Status       string       `json:"status"`
	Diagnostics  []Diagnostic `json:"diagnostics"`
}

type SelectorInfo struct {
	Kind            string   `json:"kind"`
	Summary         string   `json:"summary"`
	Section         string   `json:"section,omitempty"`
	Key             string   `json:"key,omitempty"`
	Path            []string `json:"path,omitempty"`
	MissingSection  string   `json:"missingSection,omitempty"`
	MissingKey      string   `json:"missingKey,omitempty"`
	CreateMissing   string   `json:"createMissing,omitempty"`
	DuplicatePolicy string   `json:"duplicatePolicy,omitempty"`
	DeleteKey       string   `json:"deleteKey,omitempty"`
}

type Snapshot struct {
	Exists     bool   `json:"exists"`
	SHA256     string `json:"sha256,omitempty"`
	Normalizer string `json:"normalizer"`
}

type Diagnostic struct {
	Code       string `json:"code"`
	Severity   string `json:"severity"`
	Message    string `json:"message"`
	Ref        string `json:"ref,omitempty"`
	ResourceID string `json:"resourceId,omitempty"`
	DriverID   string `json:"driverId,omitempty"`
	Path       string `json:"path,omitempty"`
}

type CurrentDesired struct {
	Desired Desired
	Plan    *Plan
}

type ApplyOptions struct {
	BackupHook BackupHook
	AfterApply func(*Plan) error
}

type ApplyResult struct {
	Plan     *Plan
	Mutated  bool
	Verified bool
	Backup   *BackupResult
}

type BackupRequest struct {
	SettingRef string
	ResourceID string
	DriverID   string
	Path       string
	Before     Snapshot
	BeforeFile []byte `json:"-"`
}

type BackupResult struct {
	ID     string
	Before Snapshot
}

type BackupHook func(BackupRequest) (BackupResult, error)

type PlanError struct {
	Diagnostics []Diagnostic
}

func (e *PlanError) Error() string {
	if e == nil || len(e.Diagnostics) == 0 {
		return "selected value plan blocked"
	}
	parts := make([]string, 0, len(e.Diagnostics))
	for _, diagnostic := range e.Diagnostics {
		parts = append(parts, fmt.Sprintf("%s[%s]: %s", diagnostic.Ref, diagnostic.Code, diagnostic.Message))
	}
	return strings.Join(parts, "; ")
}

func PlanRead(req Request) (*Plan, error) {
	ctx, plan, err := buildContext(req, false)
	if err != nil {
		return plan, err
	}
	if err := readCurrent(ctx, plan); err != nil {
		block(plan, driverDiagnostic("selectedvalue.driver.read", err, plan))
		return plan, &PlanError{Diagnostics: plan.Diagnostics}
	}
	return plan, nil
}

func ReadCurrentDesired(req Request) (*CurrentDesired, error) {
	ctx, plan, err := buildContext(req, false)
	if err != nil {
		return &CurrentDesired{Plan: plan}, err
	}
	desired, err := readCurrentDesired(ctx, plan)
	if err != nil {
		block(plan, driverDiagnostic("selectedvalue.driver.read", err, plan))
		return &CurrentDesired{Plan: plan}, &PlanError{Diagnostics: plan.Diagnostics}
	}
	return &CurrentDesired{Desired: desired, Plan: plan}, nil
}

func PlanPreview(req PreviewRequest) (*Plan, error) {
	ctx, plan, err := buildContext(req.Request, true)
	if err != nil {
		return plan, err
	}
	if safetyErr := req.Recipe.ValidateWriteSafety(req.WriteSafetyContext); safetyErr != nil {
		for _, diagnostic := range recipe.ValidationDiagnostics(safetyErr) {
			block(plan, Diagnostic{Code: diagnostic.Code, Severity: diagnostic.Severity, Message: diagnostic.Message, Ref: plan.SettingRef, ResourceID: plan.ResourceID, DriverID: plan.DriverID, Path: diagnostic.Path})
		}
		if len(plan.Diagnostics) == 0 {
			block(plan, Diagnostic{Code: "selectedvalue.writeSafety.blocked", Severity: SeverityError, Message: safetyErr.Error(), Ref: plan.SettingRef, ResourceID: plan.ResourceID, DriverID: plan.DriverID})
		}
		return plan, &PlanError{Diagnostics: plan.Diagnostics}
	}
	if err := previewDesired(ctx, plan, req.Desired); err != nil {
		var desiredErr *DesiredError
		if errors.As(err, &desiredErr) {
			block(plan, Diagnostic{Code: desiredErr.Code, Severity: SeverityError, Message: desiredErr.Message, Ref: plan.SettingRef, ResourceID: plan.ResourceID, DriverID: plan.DriverID, Path: plan.Path})
		} else {
			block(plan, driverDiagnostic("selectedvalue.driver.preview", err, plan))
		}
		return plan, &PlanError{Diagnostics: plan.Diagnostics}
	}
	return plan, nil
}

func ApplyWithBackup(req PreviewRequest, opts ApplyOptions) (*ApplyResult, error) {
	ctx, plan, err := buildContext(req.Request, true)
	if err != nil {
		return &ApplyResult{Plan: plan}, err
	}
	if safetyErr := req.Recipe.ValidateWriteSafety(req.WriteSafetyContext); safetyErr != nil {
		for _, diagnostic := range recipe.ValidationDiagnostics(safetyErr) {
			block(plan, Diagnostic{Code: diagnostic.Code, Severity: diagnostic.Severity, Message: diagnostic.Message, Ref: plan.SettingRef, ResourceID: plan.ResourceID, DriverID: plan.DriverID, Path: diagnostic.Path})
		}
		if len(plan.Diagnostics) == 0 {
			block(plan, Diagnostic{Code: "selectedvalue.writeSafety.blocked", Severity: SeverityError, Message: safetyErr.Error(), Ref: plan.SettingRef, ResourceID: plan.ResourceID, DriverID: plan.DriverID})
		}
		return &ApplyResult{Plan: plan}, &PlanError{Diagnostics: plan.Diagnostics}
	}
	result, err := applyDesiredWithBackup(ctx, plan, req.Desired, opts)
	if err != nil {
		return result, err
	}
	return result, nil
}

type DesiredError struct {
	Code    string
	Message string
}

func (e *DesiredError) Error() string {
	if e == nil {
		return "selected-value desired state is invalid"
	}
	return e.Message
}

func desiredError(code string, message string) error {
	return &DesiredError{Code: code, Message: message}
}

type context struct {
	req      Request
	setting  recipe.Setting
	resource recipe.Resource
	target   filedriver.Target
}

func buildContext(req Request, allowMissingRoot bool) (context, *Plan, error) {
	if req.Recipe == nil {
		plan := blockedPlan("", "", "", "", "selectedvalue.recipe.required", "recipe is required")
		return context{}, plan, &PlanError{Diagnostics: plan.Diagnostics}
	}
	if err := req.Recipe.Validate(); err != nil {
		plan := blockedPlan(req.Recipe.Target, req.SettingRef, "", "", "selectedvalue.recipe.invalid", "recipe validation failed")
		for _, diagnostic := range recipe.ValidationDiagnostics(err) {
			block(plan, Diagnostic{Code: diagnostic.Code, Severity: diagnostic.Severity, Message: diagnostic.Message, Ref: req.SettingRef, Path: diagnostic.Path})
		}
		return context{}, plan, &PlanError{Diagnostics: plan.Diagnostics}
	}
	settingID, settingRef, err := resolveSettingRef(req.Recipe, req.SettingRef)
	if err != nil {
		plan := blockedPlan(req.Recipe.Target, req.SettingRef, "", "", "selectedvalue.setting.invalid", err.Error())
		return context{}, plan, &PlanError{Diagnostics: plan.Diagnostics}
	}
	setting, ok := req.Recipe.Settings[settingID]
	if !ok {
		plan := blockedPlan(req.Recipe.Target, settingRef, settingID, "", "selectedvalue.setting.unknown", fmt.Sprintf("unknown setting %s", settingID))
		return context{}, plan, &PlanError{Diagnostics: plan.Diagnostics}
	}
	resourceID, resource, err := req.Recipe.ResourceForSetting(settingID)
	if err != nil {
		plan := blockedPlan(req.Recipe.Target, settingRef, settingID, "", "selectedvalue.resource.unknown", err.Error())
		return context{}, plan, &PlanError{Diagnostics: plan.Diagnostics}
	}
	root, err := req.Recipe.LocationRoot(resource.Location, req.LocationRoots)
	if err != nil {
		plan := blockedPlan(req.Recipe.Target, settingRef, settingID, resourceID, "selectedvalue.location.resolve", err.Error())
		plan.DriverID = resource.Driver
		return context{}, plan, &PlanError{Diagnostics: plan.Diagnostics}
	}
	target := filedriver.Target{LocationID: resource.Location, Root: root, RelPath: resource.Path, AllowMissingRoot: allowMissingRoot, RejectRootSymlink: true}
	resolved, err := filedriver.ResolveTarget(target)
	path := ""
	if err == nil {
		path = resolved.AbsPath
	} else if root != "" && resource.Path != "" {
		path = filepath.Join(root, filepath.FromSlash(resource.Path))
	}
	plan := &Plan{
		TargetRef:    req.Recipe.Target,
		SettingRef:   settingRef,
		SettingID:    settingID,
		ScopeDefault: setting.ScopeDefault,
		ResourceID:   resourceID,
		DriverID:     resource.Driver,
		LocationID:   resource.Location,
		RelPath:      resource.Path,
		Path:         path,
		Selector:     selectorInfo(resource),
		Status:       StatusOK,
		Diagnostics:  []Diagnostic{},
	}
	if err != nil {
		block(plan, driverDiagnostic("selectedvalue.location.resolve", err, plan))
		return context{}, plan, &PlanError{Diagnostics: plan.Diagnostics}
	}
	if err := validateGitINIIdentityCaseSafety(req.Recipe, resource, path); err != nil {
		block(plan, driverDiagnostic("selectedvalue.driver.git-case-safety", err, plan))
		return context{}, plan, &PlanError{Diagnostics: plan.Diagnostics}
	}
	switch resource.Driver {
	case recipe.IniFileDriverID, recipe.JSONFileDriverID, recipe.YAMLFileDriverID, recipe.TOMLFileDriverID, recipe.PlistFileDriverID:
	default:
		block(plan, Diagnostic{Code: "selectedvalue.driver.unsupported", Severity: SeverityError, Message: fmt.Sprintf("driver %s is not supported by selected-value planning", resource.Driver), Ref: plan.SettingRef, ResourceID: plan.ResourceID, DriverID: plan.DriverID, Path: plan.Path})
		return context{}, plan, &PlanError{Diagnostics: plan.Diagnostics}
	}
	return context{req: req, setting: setting, resource: resource, target: target}, plan, nil
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

func readCurrent(ctx context, plan *Plan) error {
	switch ctx.resource.Driver {
	case recipe.IniFileDriverID:
		state, err := inidriver.Driver{}.ReadCurrent(iniRequest(ctx))
		if err != nil {
			return err
		}
		plan.Current = Snapshot{Exists: state.Exists, SHA256: state.SHA256, Normalizer: state.Normalizer}
	case recipe.JSONFileDriverID:
		state, err := jsondriver.Driver{}.ReadCurrent(jsonRequest(ctx))
		if err != nil {
			return err
		}
		plan.Current = Snapshot{Exists: state.Exists, SHA256: state.SHA256, Normalizer: state.Normalizer}
	case recipe.YAMLFileDriverID:
		state, err := yamldriver.Driver{}.ReadCurrent(yamlRequest(ctx))
		if err != nil {
			return err
		}
		plan.Current = Snapshot{Exists: state.Exists, SHA256: state.SHA256, Normalizer: state.Normalizer}
	case recipe.TOMLFileDriverID:
		state, err := tomldriver.Driver{}.ReadCurrent(tomlRequest(ctx))
		if err != nil {
			return err
		}
		plan.Current = Snapshot{Exists: state.Exists, SHA256: state.SHA256, Normalizer: state.Normalizer}
	case recipe.PlistFileDriverID:
		state, err := plistdriver.Driver{}.ReadCurrent(plistRequest(ctx))
		if err != nil {
			return err
		}
		plan.Current = Snapshot{Exists: state.Exists, SHA256: state.SHA256, Normalizer: state.Normalizer}
	}
	return nil
}

func readCurrentDesired(ctx context, plan *Plan) (Desired, error) {
	switch ctx.resource.Driver {
	case recipe.IniFileDriverID:
		state, err := inidriver.Driver{}.ReadCurrent(iniRequest(ctx))
		if err != nil {
			return Desired{}, err
		}
		plan.Current = Snapshot{Exists: state.Exists, SHA256: state.SHA256, Normalizer: state.Normalizer}
		if !state.Exists {
			return Delete(), nil
		}
		return SetString(state.Value), nil
	case recipe.JSONFileDriverID:
		state, err := jsondriver.Driver{}.ReadCurrent(jsonRequest(ctx))
		if err != nil {
			return Desired{}, err
		}
		plan.Current = Snapshot{Exists: state.Exists, SHA256: state.SHA256, Normalizer: state.Normalizer}
		return desiredFromJSONState(state)
	case recipe.YAMLFileDriverID:
		state, err := yamldriver.Driver{}.ReadCurrent(yamlRequest(ctx))
		if err != nil {
			return Desired{}, err
		}
		plan.Current = Snapshot{Exists: state.Exists, SHA256: state.SHA256, Normalizer: state.Normalizer}
		return desiredFromYAMLState(state)
	case recipe.TOMLFileDriverID:
		state, err := tomldriver.Driver{}.ReadCurrent(tomlRequest(ctx))
		if err != nil {
			return Desired{}, err
		}
		plan.Current = Snapshot{Exists: state.Exists, SHA256: state.SHA256, Normalizer: state.Normalizer}
		return desiredFromTOMLState(state)
	case recipe.PlistFileDriverID:
		state, err := plistdriver.Driver{}.ReadCurrent(plistRequest(ctx))
		if err != nil {
			return Desired{}, err
		}
		plan.Current = Snapshot{Exists: state.Exists, SHA256: state.SHA256, Normalizer: state.Normalizer}
		return desiredFromPlistState(state)
	default:
		return Desired{}, desiredError("selectedvalue.driver.unsupported", "resource driver is not supported by selected-value live reads")
	}
}

func previewDesired(ctx context, plan *Plan, desired Desired) error {
	switch ctx.resource.Driver {
	case recipe.IniFileDriverID:
		state, err := desiredINIState(desired)
		if err != nil {
			return err
		}
		preview, err := inidriver.Driver{}.PreviewApply(iniRequest(ctx), state)
		if err != nil {
			return err
		}
		plan.Current = Snapshot{Exists: preview.Change.Before.Exists, SHA256: preview.Change.Before.SHA256, Normalizer: preview.Normalizer}
		plan.Desired = &Snapshot{Exists: preview.Change.After.Exists, SHA256: preview.Change.After.SHA256, Normalizer: preview.Normalizer}
		plan.ChangeKind = string(preview.Change.Kind)
		plan.Intent = string(preview.Intent)
	case recipe.JSONFileDriverID:
		state, err := desiredJSONState(desired)
		if err != nil {
			return err
		}
		preview, err := jsondriver.Driver{}.PreviewApply(jsonRequest(ctx), state)
		if err != nil {
			return err
		}
		plan.Current = Snapshot{Exists: preview.Change.Before.Exists, SHA256: preview.Change.Before.SHA256, Normalizer: preview.Normalizer}
		plan.Desired = &Snapshot{Exists: preview.Change.After.Exists, SHA256: preview.Change.After.SHA256, Normalizer: preview.Normalizer}
		plan.ChangeKind = string(preview.Change.Kind)
		plan.Intent = string(preview.Intent)
	case recipe.YAMLFileDriverID:
		state, err := desiredYAMLState(desired)
		if err != nil {
			return err
		}
		preview, err := yamldriver.Driver{}.PreviewApply(yamlRequest(ctx), state)
		if err != nil {
			return err
		}
		plan.Current = Snapshot{Exists: preview.Change.Before.Exists, SHA256: preview.Change.Before.SHA256, Normalizer: preview.Normalizer}
		plan.Desired = &Snapshot{Exists: preview.Change.After.Exists, SHA256: preview.Change.After.SHA256, Normalizer: preview.Normalizer}
		plan.ChangeKind = string(preview.Change.Kind)
		plan.Intent = string(preview.Intent)
	case recipe.TOMLFileDriverID:
		state, err := desiredTOMLState(desired)
		if err != nil {
			return err
		}
		preview, err := tomldriver.Driver{}.PreviewApply(tomlRequest(ctx), state)
		if err != nil {
			return err
		}
		plan.Current = Snapshot{Exists: preview.Change.Before.Exists, SHA256: preview.Change.Before.SHA256, Normalizer: preview.Normalizer}
		plan.Desired = &Snapshot{Exists: preview.Change.After.Exists, SHA256: preview.Change.After.SHA256, Normalizer: preview.Normalizer}
		plan.ChangeKind = string(preview.Change.Kind)
		plan.Intent = string(preview.Intent)
	case recipe.PlistFileDriverID:
		state, err := desiredPlistState(desired)
		if err != nil {
			return err
		}
		preview, err := plistdriver.Driver{}.PreviewApply(plistRequest(ctx), state)
		if err != nil {
			return err
		}
		plan.Format = preview.Format
		plan.Current = Snapshot{Exists: preview.Change.Before.Exists, SHA256: preview.Change.Before.SHA256, Normalizer: preview.Normalizer}
		plan.Desired = &Snapshot{Exists: preview.Change.After.Exists, SHA256: preview.Change.After.SHA256, Normalizer: preview.Normalizer}
		plan.ChangeKind = string(preview.Change.Kind)
		plan.Intent = string(preview.Intent)
	}
	return nil
}

func applyDesiredWithBackup(ctx context, plan *Plan, desired Desired, opts ApplyOptions) (*ApplyResult, error) {
	if err := previewDesired(ctx, plan, desired); err != nil {
		var desiredErr *DesiredError
		if errors.As(err, &desiredErr) {
			block(plan, Diagnostic{Code: desiredErr.Code, Severity: SeverityError, Message: desiredErr.Message, Ref: plan.SettingRef, ResourceID: plan.ResourceID, DriverID: plan.DriverID, Path: plan.Path})
		} else {
			block(plan, driverDiagnostic("selectedvalue.driver.preview", err, plan))
		}
		return &ApplyResult{Plan: plan}, &PlanError{Diagnostics: plan.Diagnostics}
	}

	var backup *BackupResult
	switch ctx.resource.Driver {
	case recipe.IniFileDriverID:
		state, err := desiredINIState(desired)
		if err != nil {
			block(plan, Diagnostic{Code: "selectedvalue.desired.invalid", Severity: SeverityError, Message: "selected-value desired state is invalid", Ref: plan.SettingRef, ResourceID: plan.ResourceID, DriverID: plan.DriverID, Path: plan.Path})
			return &ApplyResult{Plan: plan}, &PlanError{Diagnostics: plan.Diagnostics}
		}
		apply, driverBackup, err := inidriver.Driver{}.ApplyWithBackup(iniRequest(ctx), state, iniBackupHook(plan, opts.BackupHook, &backup))
		result := &ApplyResult{Plan: plan, Mutated: apply.Mutated, Backup: backup}
		if driverBackup != nil && backup == nil {
			backup = &BackupResult{ID: driverBackup.ID, Before: Snapshot{Exists: driverBackup.Before.Exists, SHA256: driverBackup.Before.SHA256, Normalizer: inidriver.NormalizerID}}
			result.Backup = backup
		}
		if err != nil {
			return result, err
		}
		if err := runAfterApply(opts.AfterApply, plan); err != nil {
			return result, err
		}
		verify, err := inidriver.Driver{}.Verify(iniRequest(ctx), state)
		result.Verified = verify.Verified
		if err != nil {
			return result, err
		}
		return result, nil
	case recipe.JSONFileDriverID:
		state, err := desiredJSONState(desired)
		if err != nil {
			block(plan, Diagnostic{Code: "selectedvalue.desired.invalid", Severity: SeverityError, Message: "selected-value desired state is invalid", Ref: plan.SettingRef, ResourceID: plan.ResourceID, DriverID: plan.DriverID, Path: plan.Path})
			return &ApplyResult{Plan: plan}, &PlanError{Diagnostics: plan.Diagnostics}
		}
		apply, driverBackup, err := jsondriver.Driver{}.ApplyWithBackup(jsonRequest(ctx), state, jsonBackupHook(plan, opts.BackupHook, &backup))
		result := &ApplyResult{Plan: plan, Mutated: apply.Mutated, Backup: backup}
		if driverBackup != nil && backup == nil {
			backup = &BackupResult{ID: driverBackup.ID, Before: Snapshot{Exists: driverBackup.Before.Exists, SHA256: driverBackup.Before.SHA256, Normalizer: jsondriver.NormalizerID}}
			result.Backup = backup
		}
		if err != nil {
			return result, err
		}
		if err := runAfterApply(opts.AfterApply, plan); err != nil {
			return result, err
		}
		verify, err := jsondriver.Driver{}.Verify(jsonRequest(ctx), state)
		result.Verified = verify.Verified
		if err != nil {
			return result, err
		}
		return result, nil
	case recipe.YAMLFileDriverID:
		state, err := desiredYAMLState(desired)
		if err != nil {
			block(plan, Diagnostic{Code: "selectedvalue.desired.invalid", Severity: SeverityError, Message: "selected-value desired state is invalid", Ref: plan.SettingRef, ResourceID: plan.ResourceID, DriverID: plan.DriverID, Path: plan.Path})
			return &ApplyResult{Plan: plan}, &PlanError{Diagnostics: plan.Diagnostics}
		}
		apply, driverBackup, err := yamldriver.Driver{}.ApplyWithBackup(yamlRequest(ctx), state, yamlBackupHook(plan, opts.BackupHook, &backup))
		result := &ApplyResult{Plan: plan, Mutated: apply.Mutated, Backup: backup}
		if driverBackup != nil && backup == nil {
			backup = &BackupResult{ID: driverBackup.ID, Before: Snapshot{Exists: driverBackup.Before.Exists, SHA256: driverBackup.Before.SHA256, Normalizer: yamldriver.NormalizerID}}
			result.Backup = backup
		}
		if err != nil {
			return result, err
		}
		if err := runAfterApply(opts.AfterApply, plan); err != nil {
			return result, err
		}
		verify, err := yamldriver.Driver{}.Verify(yamlRequest(ctx), state)
		result.Verified = verify.Verified
		if err != nil {
			return result, err
		}
		return result, nil
	case recipe.TOMLFileDriverID:
		state, err := desiredTOMLState(desired)
		if err != nil {
			block(plan, Diagnostic{Code: "selectedvalue.desired.invalid", Severity: SeverityError, Message: "selected-value desired state is invalid", Ref: plan.SettingRef, ResourceID: plan.ResourceID, DriverID: plan.DriverID, Path: plan.Path})
			return &ApplyResult{Plan: plan}, &PlanError{Diagnostics: plan.Diagnostics}
		}
		apply, driverBackup, err := tomldriver.Driver{}.ApplyWithBackup(tomlRequest(ctx), state, tomlBackupHook(plan, opts.BackupHook, &backup))
		result := &ApplyResult{Plan: plan, Mutated: apply.Mutated, Backup: backup}
		if driverBackup != nil && backup == nil {
			backup = &BackupResult{ID: driverBackup.ID, Before: Snapshot{Exists: driverBackup.Before.Exists, SHA256: driverBackup.Before.SHA256, Normalizer: tomldriver.NormalizerID}}
			result.Backup = backup
		}
		if err != nil {
			return result, err
		}
		if err := runAfterApply(opts.AfterApply, plan); err != nil {
			return result, err
		}
		verify, err := tomldriver.Driver{}.Verify(tomlRequest(ctx), state)
		result.Verified = verify.Verified
		if err != nil {
			return result, err
		}
		return result, nil
	case recipe.PlistFileDriverID:
		state, err := desiredPlistState(desired)
		if err != nil {
			block(plan, Diagnostic{Code: "selectedvalue.desired.invalid", Severity: SeverityError, Message: "selected-value desired state is invalid", Ref: plan.SettingRef, ResourceID: plan.ResourceID, DriverID: plan.DriverID, Path: plan.Path})
			return &ApplyResult{Plan: plan}, &PlanError{Diagnostics: plan.Diagnostics}
		}
		apply, driverBackup, err := plistdriver.Driver{}.ApplyWithBackup(plistRequest(ctx), state, plistBackupHook(plan, opts.BackupHook, &backup))
		result := &ApplyResult{Plan: plan, Mutated: apply.Mutated, Backup: backup}
		plan.Format = apply.Preview.Format
		if driverBackup != nil && backup == nil {
			backup = &BackupResult{ID: driverBackup.ID, Before: Snapshot{Exists: driverBackup.Before.Exists, SHA256: driverBackup.Before.SHA256, Normalizer: plistdriver.NormalizerID}}
			result.Backup = backup
		}
		if err != nil {
			return result, err
		}
		if err := runAfterApply(opts.AfterApply, plan); err != nil {
			return result, err
		}
		verify, err := plistdriver.Driver{}.Verify(plistRequest(ctx), state)
		result.Verified = verify.Verified
		if err != nil {
			return result, err
		}
		return result, nil
	default:
		return &ApplyResult{Plan: plan}, desiredError("selectedvalue.driver.unsupported", "resource driver is not supported by selected-value live apply")
	}
}

func desiredINIState(desired Desired) (inidriver.State, error) {
	switch desired.intent {
	case IntentDelete:
		return inidriver.DeleteState(), nil
	case IntentSet:
		if desired.kind != "string" {
			return inidriver.State{}, desiredError("selectedvalue.desired.iniTypeUnsupported", "INI selected-value desired set requires a string scalar or delete intent")
		}
		value, ok := desired.value.(string)
		if !ok {
			return inidriver.State{}, desiredError("selectedvalue.desired.invalid", "desired string value has an invalid internal representation")
		}
		return inidriver.Driver{}.Normalize(value), nil
	default:
		return inidriver.State{}, desiredError("selectedvalue.desired.intentRequired", "desired intent is required")
	}
}

func desiredJSONState(desired Desired) (jsondriver.State, error) {
	switch desired.intent {
	case IntentDelete:
		return jsondriver.DeleteState(), nil
	case IntentSet:
		value, err := jsonScalarValue(desired)
		if err != nil {
			return jsondriver.State{}, err
		}
		return jsondriver.Driver{}.NormalizeValue(value)
	default:
		return jsondriver.State{}, desiredError("selectedvalue.desired.intentRequired", "desired intent is required")
	}
}

func desiredYAMLState(desired Desired) (yamldriver.State, error) {
	switch desired.intent {
	case IntentDelete:
		return yamldriver.DeleteState(), nil
	case IntentSet:
		value, err := jsonScalarValue(desired)
		if err != nil {
			return yamldriver.State{}, err
		}
		return yamldriver.Driver{}.NormalizeValue(value)
	default:
		return yamldriver.State{}, desiredError("selectedvalue.desired.intentRequired", "desired intent is required")
	}
}

func desiredTOMLState(desired Desired) (tomldriver.State, error) {
	switch desired.intent {
	case IntentDelete:
		return tomldriver.DeleteState(), nil
	case IntentSet:
		value, err := tomlScalarValue(desired)
		if err != nil {
			return tomldriver.State{}, err
		}
		return tomldriver.Driver{}.NormalizeValue(value)
	default:
		return tomldriver.State{}, desiredError("selectedvalue.desired.intentRequired", "desired intent is required")
	}
}

func desiredPlistState(desired Desired) (plistdriver.State, error) {
	switch desired.intent {
	case IntentDelete:
		return plistdriver.DeleteState(), nil
	case IntentSet:
		value, err := plistScalarValue(desired)
		if err != nil {
			return plistdriver.State{}, err
		}
		return plistdriver.Driver{}.NormalizeValue(value)
	default:
		return plistdriver.State{}, desiredError("selectedvalue.desired.intentRequired", "desired intent is required")
	}
}

func jsonScalarValue(desired Desired) (any, error) {
	switch desired.kind {
	case "string":
		value, ok := desired.value.(string)
		if !ok {
			return nil, desiredError("selectedvalue.desired.invalid", "desired string value has an invalid internal representation")
		}
		return value, nil
	case "bool":
		value, ok := desired.value.(bool)
		if !ok {
			return nil, desiredError("selectedvalue.desired.invalid", "desired bool value has an invalid internal representation")
		}
		return value, nil
	case "number":
		value, ok := desired.value.(json.Number)
		if !ok {
			return nil, desiredError("selectedvalue.desired.invalid", "desired number value has an invalid internal representation")
		}
		if _, err := json.Marshal(value); err != nil {
			return nil, desiredError("selectedvalue.desired.invalidNumber", "desired number value must be a valid JSON number")
		}
		return value, nil
	case "null":
		return nil, nil
	default:
		return nil, desiredError("selectedvalue.desired.jsonTypeUnsupported", "JSON/YAML selected-value desired set requires a JSON-compatible scalar or delete intent")
	}
}

func tomlScalarValue(desired Desired) (any, error) {
	if desired.kind == "null" {
		return nil, desiredError("selectedvalue.desired.tomlNullUnsupported", "TOML selected-value desired set does not support null; use delete intent to remove a key")
	}
	value, err := jsonScalarValue(desired)
	if err != nil {
		if desired.kind == "object" || desired.kind == "array" || desired.kind == "null" {
			return nil, desiredError("selectedvalue.desired.tomlTypeUnsupported", "TOML selected-value desired set requires a string, bool, finite number, or delete intent")
		}
		return nil, err
	}
	return value, nil
}

func plistScalarValue(desired Desired) (any, error) {
	if desired.kind == "null" {
		return nil, desiredError("selectedvalue.desired.plistNullUnsupported", "plist selected-value desired set does not support null; use delete intent to remove a key")
	}
	value, err := jsonScalarValue(desired)
	if err != nil {
		if desired.kind == "object" || desired.kind == "array" || desired.kind == "null" {
			return nil, desiredError("selectedvalue.desired.plistTypeUnsupported", "plist selected-value desired set requires a string, bool, finite number, or delete intent")
		}
		return nil, err
	}
	return value, nil
}

func desiredFromJSONState(state jsondriver.State) (Desired, error) {
	if !state.Exists {
		return Delete(), nil
	}
	return desiredFromCanonicalJSON(state.Value)
}

func desiredFromYAMLState(state yamldriver.State) (Desired, error) {
	if !state.Exists {
		return Delete(), nil
	}
	return desiredFromCanonicalJSON(state.Value)
}

func desiredFromTOMLState(state tomldriver.State) (Desired, error) {
	if !state.Exists {
		return Delete(), nil
	}
	return desiredFromCanonicalJSON(state.Value)
}

func desiredFromPlistState(state plistdriver.State) (Desired, error) {
	if !state.Exists {
		return Delete(), nil
	}
	return desiredFromCanonicalJSON(state.Value)
}

func desiredFromCanonicalJSON(raw []byte) (Desired, error) {
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return Desired{}, desiredError("selectedvalue.desired.currentInvalid", "current selected scalar could not be decoded")
	}
	switch typed := value.(type) {
	case string:
		return SetString(typed), nil
	case bool:
		return SetBool(typed), nil
	case json.Number:
		return SetNumber(typed), nil
	case nil:
		return SetNull(), nil
	default:
		return Desired{}, desiredError("selectedvalue.desired.currentUnsupported", "current selected value is not a supported scalar")
	}
}

func iniBackupHook(plan *Plan, hook BackupHook, captured **BackupResult) inidriver.BackupHook {
	if hook == nil {
		return nil
	}
	return func(req inidriver.BackupRequest) (inidriver.BackupResult, error) {
		result, err := hook(BackupRequest{
			SettingRef: plan.SettingRef,
			ResourceID: plan.ResourceID,
			DriverID:   plan.DriverID,
			Path:       req.Path,
			Before:     Snapshot{Exists: req.Before.Exists, SHA256: req.Before.SHA256, Normalizer: req.Before.Normalizer},
			BeforeFile: append([]byte(nil), req.BeforeFile...),
		})
		if err != nil {
			return inidriver.BackupResult{}, err
		}
		*captured = &result
		return inidriver.BackupResult{ID: result.ID, Before: inidriver.Snapshot{Exists: result.Before.Exists, SHA256: result.Before.SHA256}}, nil
	}
}

func jsonBackupHook(plan *Plan, hook BackupHook, captured **BackupResult) jsondriver.BackupHook {
	if hook == nil {
		return nil
	}
	return func(req jsondriver.BackupRequest) (jsondriver.BackupResult, error) {
		result, err := hook(BackupRequest{
			SettingRef: plan.SettingRef,
			ResourceID: plan.ResourceID,
			DriverID:   plan.DriverID,
			Path:       req.Path,
			Before:     Snapshot{Exists: req.Before.Exists, SHA256: req.Before.SHA256, Normalizer: req.Before.Normalizer},
			BeforeFile: append([]byte(nil), req.BeforeFile...),
		})
		if err != nil {
			return jsondriver.BackupResult{}, err
		}
		*captured = &result
		return jsondriver.BackupResult{ID: result.ID, Before: jsondriver.Snapshot{Exists: result.Before.Exists, SHA256: result.Before.SHA256}}, nil
	}
}

func yamlBackupHook(plan *Plan, hook BackupHook, captured **BackupResult) yamldriver.BackupHook {
	if hook == nil {
		return nil
	}
	return func(req yamldriver.BackupRequest) (yamldriver.BackupResult, error) {
		result, err := hook(BackupRequest{
			SettingRef: plan.SettingRef,
			ResourceID: plan.ResourceID,
			DriverID:   plan.DriverID,
			Path:       req.Path,
			Before:     Snapshot{Exists: req.Before.Exists, SHA256: req.Before.SHA256, Normalizer: req.Before.Normalizer},
			BeforeFile: append([]byte(nil), req.BeforeFile...),
		})
		if err != nil {
			return yamldriver.BackupResult{}, err
		}
		*captured = &result
		return yamldriver.BackupResult{ID: result.ID, Before: yamldriver.Snapshot{Exists: result.Before.Exists, SHA256: result.Before.SHA256}}, nil
	}
}

func tomlBackupHook(plan *Plan, hook BackupHook, captured **BackupResult) tomldriver.BackupHook {
	if hook == nil {
		return nil
	}
	return func(req tomldriver.BackupRequest) (tomldriver.BackupResult, error) {
		result, err := hook(BackupRequest{
			SettingRef: plan.SettingRef,
			ResourceID: plan.ResourceID,
			DriverID:   plan.DriverID,
			Path:       req.Path,
			Before:     Snapshot{Exists: req.Before.Exists, SHA256: req.Before.SHA256, Normalizer: req.Before.Normalizer},
			BeforeFile: append([]byte(nil), req.BeforeFile...),
		})
		if err != nil {
			return tomldriver.BackupResult{}, err
		}
		*captured = &result
		return tomldriver.BackupResult{ID: result.ID, Before: tomldriver.Snapshot{Exists: result.Before.Exists, SHA256: result.Before.SHA256}}, nil
	}
}

func plistBackupHook(plan *Plan, hook BackupHook, captured **BackupResult) plistdriver.BackupHook {
	if hook == nil {
		return nil
	}
	return func(req plistdriver.BackupRequest) (plistdriver.BackupResult, error) {
		result, err := hook(BackupRequest{
			SettingRef: plan.SettingRef,
			ResourceID: plan.ResourceID,
			DriverID:   plan.DriverID,
			Path:       req.Path,
			Before:     Snapshot{Exists: req.Before.Exists, SHA256: req.Before.SHA256, Normalizer: req.Before.Normalizer},
			BeforeFile: append([]byte(nil), req.BeforeFile...),
		})
		if err != nil {
			return plistdriver.BackupResult{}, err
		}
		*captured = &result
		return plistdriver.BackupResult{ID: result.ID, Before: plistdriver.Snapshot{Exists: result.Before.Exists, SHA256: result.Before.SHA256}}, nil
	}
}

func runAfterApply(hook func(*Plan) error, plan *Plan) error {
	if hook == nil {
		return nil
	}
	return hook(plan)
}

func iniRequest(ctx context) inidriver.Request {
	selector := ctx.resource.Selector
	return inidriver.Request{Target: ctx.target, Selector: inidriver.Selector{
		Section:         selector.Section,
		Key:             selector.Key,
		MissingSection:  inidriver.MissingPolicy(defaultString(selector.MissingSection, string(inidriver.MissingPolicyError))),
		MissingKey:      inidriver.MissingPolicy(defaultString(selector.MissingKey, string(inidriver.MissingPolicyError))),
		DuplicatePolicy: inidriver.DuplicatePolicy(defaultString(selector.DuplicatePolicy, string(inidriver.DuplicatePolicyReject))),
		DeleteKey:       inidriver.DeletePolicy(defaultString(selector.DeleteKey, string(inidriver.DeletePolicyReject))),
	}}
}

func jsonRequest(ctx context) jsondriver.Request {
	selector := ctx.resource.Selector
	return jsondriver.Request{Target: ctx.target, Selector: jsondriver.Selector{
		Path:            append([]string(nil), selector.Path...),
		CreateMissing:   jsondriver.CreatePolicy(defaultString(selector.CreateMissing, string(jsondriver.CreatePolicyReject))),
		DuplicatePolicy: jsondriver.DuplicatePolicy(defaultString(selector.DuplicatePolicy, string(jsondriver.DuplicatePolicyReject))),
		DeleteKey:       jsondriver.DeletePolicy(defaultString(selector.DeleteKey, string(jsondriver.DeletePolicyReject))),
	}}
}

func yamlRequest(ctx context) yamldriver.Request {
	selector := ctx.resource.Selector
	return yamldriver.Request{Target: ctx.target, Selector: yamldriver.Selector{
		Path:            append([]string(nil), selector.Path...),
		CreateMissing:   yamldriver.CreatePolicy(defaultString(selector.CreateMissing, string(yamldriver.CreatePolicyReject))),
		DuplicatePolicy: yamldriver.DuplicatePolicy(defaultString(selector.DuplicatePolicy, string(yamldriver.DuplicatePolicyReject))),
		DeleteKey:       yamldriver.DeletePolicy(defaultString(selector.DeleteKey, string(yamldriver.DeletePolicyReject))),
	}}
}

func tomlRequest(ctx context) tomldriver.Request {
	selector := ctx.resource.Selector
	return tomldriver.Request{Target: ctx.target, Selector: tomldriver.Selector{
		Path:            append([]string(nil), selector.Path...),
		CreateMissing:   tomldriver.CreatePolicy(defaultString(selector.CreateMissing, string(tomldriver.CreatePolicyReject))),
		DuplicatePolicy: tomldriver.DuplicatePolicy(defaultString(selector.DuplicatePolicy, string(tomldriver.DuplicatePolicyReject))),
		DeleteKey:       tomldriver.DeletePolicy(defaultString(selector.DeleteKey, string(tomldriver.DeletePolicyReject))),
	}}
}

func plistRequest(ctx context) plistdriver.Request {
	selector := ctx.resource.Selector
	return plistdriver.Request{Target: ctx.target, Selector: plistdriver.Selector{
		Path:            append([]string(nil), selector.Path...),
		CreateMissing:   plistdriver.CreatePolicy(defaultString(selector.CreateMissing, string(plistdriver.CreatePolicyReject))),
		DuplicatePolicy: plistdriver.DuplicatePolicy(defaultString(selector.DuplicatePolicy, string(plistdriver.DuplicatePolicyReject))),
		DeleteKey:       plistdriver.DeletePolicy(defaultString(selector.DeleteKey, string(plistdriver.DeletePolicyReject))),
	}}
}

func validateGitINIIdentityCaseSafety(rec *recipe.Recipe, resource recipe.Resource, path string) error {
	if rec == nil || rec.Target != recipe.GitTarget || resource.Driver != recipe.IniFileDriverID || resource.Path != ".gitconfig" || resource.Selector == nil {
		return nil
	}
	selector := resource.Selector
	if selector.Section != "user" || (selector.Key != "email" && selector.Key != "name") {
		return nil
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return nil
	}
	if err := checkGitINIIdentityCase(data, selector.Key); err != nil {
		return &filedriver.Error{Code: filedriver.CodeInvalidSelector, Op: "gitCaseSafety", Path: path, Err: err}
	}
	return nil
}

func checkGitINIIdentityCase(data []byte, key string) error {
	lines := strings.Split(string(data), "\n")
	inUserSection := false
	userSections := 0
	keyMatches := 0
	for _, rawLine := range lines {
		line := strings.TrimSuffix(rawLine, "\r")
		if sectionName, ok := parseGitCaseGuardSection(line); ok {
			inUserSection = false
			if strings.EqualFold(sectionName, "user") {
				userSections++
				if sectionName != "user" {
					return fmt.Errorf("git [user] identity section must use canonical lowercase spelling")
				}
				if userSections > 1 {
					return fmt.Errorf("git [user] identity section is duplicated case-insensitively")
				}
				inUserSection = true
			}
			continue
		}
		if !inUserSection {
			continue
		}
		keyName, ok := parseGitCaseGuardKey(line)
		if !ok || !strings.EqualFold(keyName, key) {
			continue
		}
		keyMatches++
		if keyName != key {
			return fmt.Errorf("git [user] %s key must use canonical lowercase spelling", key)
		}
		if keyMatches > 1 {
			return fmt.Errorf("git [user] %s key is duplicated case-insensitively", key)
		}
	}
	return nil
}

func parseGitCaseGuardSection(content string) (string, bool) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") || !strings.HasPrefix(trimmed, "[") {
		return "", false
	}
	closeIdx := strings.Index(trimmed, "]")
	if closeIdx < 0 {
		return "", false
	}
	name := strings.TrimSpace(trimmed[1:closeIdx])
	if name == "" {
		return "", false
	}
	return name, true
}

func parseGitCaseGuardKey(content string) (string, bool) {
	trimmedLeft := strings.TrimLeft(content, " \t")
	if trimmedLeft == "" || strings.HasPrefix(trimmedLeft, "#") || strings.HasPrefix(trimmedLeft, ";") || strings.HasPrefix(trimmedLeft, "[") {
		return "", false
	}
	eq := strings.Index(content, "=")
	if eq < 0 {
		return "", false
	}
	key := strings.TrimSpace(content[:eq])
	if key == "" {
		return "", false
	}
	return key, true
}

func selectorInfo(resource recipe.Resource) SelectorInfo {
	if resource.Selector == nil {
		return SelectorInfo{Kind: "none"}
	}
	switch resource.Driver {
	case recipe.IniFileDriverID:
		return SelectorInfo{Kind: "ini-key", Summary: fmt.Sprintf("[%s] %s", resource.Selector.Section, resource.Selector.Key), Section: resource.Selector.Section, Key: resource.Selector.Key, MissingSection: defaultString(resource.Selector.MissingSection, string(inidriver.MissingPolicyError)), MissingKey: defaultString(resource.Selector.MissingKey, string(inidriver.MissingPolicyError)), DuplicatePolicy: defaultString(resource.Selector.DuplicatePolicy, string(inidriver.DuplicatePolicyReject)), DeleteKey: defaultString(resource.Selector.DeleteKey, string(inidriver.DeletePolicyReject))}
	case recipe.JSONFileDriverID, recipe.YAMLFileDriverID, recipe.TOMLFileDriverID:
		return SelectorInfo{Kind: "selected-path", Summary: strings.Join(resource.Selector.Path, "."), Path: append([]string(nil), resource.Selector.Path...), CreateMissing: defaultString(resource.Selector.CreateMissing, "reject"), DuplicatePolicy: defaultString(resource.Selector.DuplicatePolicy, "reject"), DeleteKey: defaultString(resource.Selector.DeleteKey, "reject")}
	case recipe.PlistFileDriverID:
		return SelectorInfo{Kind: "selected-path", Summary: quotedPathSummary(resource.Selector.Path), Path: append([]string(nil), resource.Selector.Path...), CreateMissing: defaultString(resource.Selector.CreateMissing, "reject"), DuplicatePolicy: defaultString(resource.Selector.DuplicatePolicy, "reject"), DeleteKey: defaultString(resource.Selector.DeleteKey, "reject")}
	default:
		return SelectorInfo{Kind: "unsupported"}
	}
}

func quotedPathSummary(path []string) string {
	data, err := json.Marshal(path)
	if err != nil {
		return fmt.Sprintf("%q", path)
	}
	return string(data)
}

func blockedPlan(targetRef string, settingRef string, settingID string, resourceID string, code string, message string) *Plan {
	ref := settingRef
	if ref == "" && targetRef != "" && settingID != "" {
		ref = targetRef + ":" + settingID
	}
	plan := &Plan{TargetRef: targetRef, SettingRef: ref, SettingID: settingID, ResourceID: resourceID, Status: StatusBlocked, Diagnostics: []Diagnostic{}}
	block(plan, Diagnostic{Code: code, Severity: SeverityError, Message: message, Ref: ref, ResourceID: resourceID})
	return plan
}

func block(plan *Plan, diagnostic Diagnostic) {
	if diagnostic.Severity == "" {
		diagnostic.Severity = SeverityError
	}
	plan.Status = StatusBlocked
	plan.Diagnostics = append(plan.Diagnostics, diagnostic)
}

func driverDiagnostic(code string, err error, plan *Plan) Diagnostic {
	diagnostic := Diagnostic{Code: code, Severity: SeverityError, Message: "selected-value driver blocked the resource plan", Ref: plan.SettingRef, ResourceID: plan.ResourceID, DriverID: plan.DriverID, Path: plan.Path}
	var driverErr *filedriver.Error
	if errors.As(err, &driverErr) {
		diagnostic.Code = "selectedvalue.driver." + string(driverErr.Code)
		if driverErr.Op != "" {
			diagnostic.Message = fmt.Sprintf("selected-value driver %s blocked the resource plan", driverErr.Op)
		}
	}
	return diagnostic
}

func defaultString(value string, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
}
