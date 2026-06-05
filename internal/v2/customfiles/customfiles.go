package customfiles

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shpoont/dotfiles-manager/internal/v2/filedriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/shpoont/dotfiles-manager/internal/v2/resolution"
)

type Operation string

const (
	OperationSave  Operation = "save"
	OperationApply Operation = "apply"
)

type Request struct {
	Profile       *resolution.ResolvedProfile
	Recipe        *recipe.Recipe
	SettingRef    string
	LocationRoots map[string]string
}

type Plan struct {
	Operation         Operation
	Setting           resolution.ResolvedSetting
	ResourceID        string
	Resource          recipe.Resource
	RepoRoot          string
	DesiredRelPath    string
	LiveTarget        filedriver.Target
	DesiredTarget     filedriver.Target
	SourceState       filedriver.State
	DestinationState  filedriver.State
	DesiredFinalState filedriver.State
	Preview           filedriver.Preview
}

type ExecuteOptions struct {
	DryRun     bool
	BackupHook BackupHook
}

type BackupRequest struct {
	Operation  Operation
	SettingRef string
	ResourceID string
	Path       string
	Before     filedriver.State
}

type BackupResult struct {
	ID     string
	Before filedriver.Snapshot
}

type BackupHook func(BackupRequest) (BackupResult, error)

type RestoreRequest struct {
	SettingRef string
	ResourceID string
	Path       string
	Backup     BackupResult
}

type RestoreHook func(RestoreRequest) error

type Result struct {
	Operation  Operation
	SettingRef string
	DryRun     bool
	Mutated    bool
	Verified   bool
	Preview    filedriver.Preview
	Backup     *BackupResult
}

func PlanSave(req Request) (*Plan, error) {
	return buildPlan(req, OperationSave)
}

func PlanApply(req Request) (*Plan, error) {
	return buildPlan(req, OperationApply)
}

func Save(req Request, opts ExecuteOptions) (*Result, error) {
	plan, err := PlanSave(req)
	if err != nil {
		return nil, err
	}
	return Execute(plan, opts)
}

func Apply(req Request, opts ExecuteOptions) (*Result, error) {
	plan, err := PlanApply(req)
	if err != nil {
		return nil, err
	}
	return Execute(plan, opts)
}

func Execute(plan *Plan, opts ExecuteOptions) (*Result, error) {
	if plan == nil {
		return nil, fmt.Errorf("operation plan is required")
	}
	result := &Result{
		Operation:  plan.Operation,
		SettingRef: plan.Setting.Ref(),
		DryRun:     opts.DryRun,
		Preview:    plan.Preview,
	}
	if opts.DryRun {
		return result, nil
	}

	driver := filedriver.Driver{}
	if err := ensureDesiredPathSafe(plan); err != nil {
		return nil, err
	}

	if plan.Operation == OperationApply {
		applyResult, backup, err := applyLiveWithBackup(driver, plan, opts.BackupHook)
		if err != nil {
			return nil, err
		}
		result.Preview = applyResult.Preview
		if backup != nil {
			result.Backup = backup
		}
		result.Mutated = applyResult.Mutated
	} else {
		applyResult, err := driver.Apply(plan.DesiredTarget, plan.DesiredFinalState)
		if err != nil {
			return nil, err
		}
		result.Preview = applyResult.Preview
		result.Mutated = applyResult.Mutated
	}

	verify, err := driver.Verify(destinationTarget(plan), plan.DesiredFinalState)
	if err != nil {
		return nil, err
	}
	result.Verified = verify.Verified
	return result, nil
}

func RestoreWithHook(req RestoreRequest, hook RestoreHook) error {
	if hook == nil {
		return fmt.Errorf("restore hook is required")
	}
	return hook(req)
}

func buildPlan(req Request, op Operation) (*Plan, error) {
	if req.Profile == nil {
		return nil, fmt.Errorf("resolved profile is required")
	}
	if req.Recipe == nil {
		return nil, fmt.Errorf("custom.files recipe is required")
	}
	if err := req.Recipe.ValidateCustomFiles(); err != nil {
		return nil, err
	}
	setting, err := resolveSetting(req.Profile, req.Recipe.Target, req.SettingRef)
	if err != nil {
		return nil, err
	}
	resourceID, resource, err := req.Recipe.ResourceForSetting(setting.SettingID)
	if err != nil {
		return nil, err
	}
	resourceRelPath, err := recipe.ValidateResourcePath(resource.Path)
	if err != nil {
		return nil, fmt.Errorf("resource %s path: %w", resourceID, err)
	}
	locationRoot, err := req.Recipe.LocationRoot(resource.Location, req.LocationRoots)
	if err != nil {
		return nil, err
	}
	liveTarget := filedriver.Target{LocationID: resource.Location, Root: locationRoot, RelPath: resourceRelPath}
	desiredTarget, desiredRel, err := desiredTargetForSetting(req.Profile.RepoRoot, setting)
	if err != nil {
		return nil, err
	}

	driver := filedriver.Driver{}
	liveState, err := driver.ReadCurrent(liveTarget)
	if err != nil {
		return nil, fmt.Errorf("read live %s: %w", setting.Ref(), err)
	}
	desiredState, err := driver.ReadCurrent(desiredTarget)
	if err != nil {
		return nil, fmt.Errorf("read desired %s: %w", setting.Ref(), err)
	}

	plan := &Plan{
		Operation:        op,
		Setting:          setting,
		ResourceID:       resourceID,
		Resource:         resource,
		RepoRoot:         req.Profile.RepoRoot,
		DesiredRelPath:   desiredRel,
		LiveTarget:       liveTarget,
		DesiredTarget:    desiredTarget,
		SourceState:      liveState,
		DestinationState: desiredState,
	}
	switch op {
	case OperationSave:
		plan.DesiredFinalState = liveState
		preview, err := driver.PreviewApply(desiredTarget, liveState)
		if err != nil {
			return nil, fmt.Errorf("preview save %s: %w", setting.Ref(), err)
		}
		plan.Preview = preview
	case OperationApply:
		plan.SourceState = desiredState
		plan.DestinationState = liveState
		plan.DesiredFinalState = desiredState
		preview, err := driver.PreviewApply(liveTarget, desiredState)
		if err != nil {
			return nil, fmt.Errorf("preview apply %s: %w", setting.Ref(), err)
		}
		plan.Preview = preview
	default:
		return nil, fmt.Errorf("unsupported custom.files operation: %s", op)
	}
	return plan, nil
}

func resolveSetting(profile *resolution.ResolvedProfile, targetID string, ref string) (resolution.ResolvedSetting, error) {
	if strings.TrimSpace(ref) != "" {
		parts := strings.SplitN(ref, ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return resolution.ResolvedSetting{}, fmt.Errorf("setting ref must be target:setting, got %q", ref)
		}
		for _, setting := range profile.Settings {
			if setting.TargetID == parts[0] && setting.SettingID == parts[1] {
				if setting.TargetID != targetID {
					return resolution.ResolvedSetting{}, fmt.Errorf("setting %s is not managed by recipe target %s", ref, targetID)
				}
				return setting, nil
			}
		}
		return resolution.ResolvedSetting{}, fmt.Errorf("resolved profile has no setting %s", ref)
	}

	matches := make([]resolution.ResolvedSetting, 0, 1)
	for _, setting := range profile.Settings {
		if setting.TargetID == targetID {
			matches = append(matches, setting)
		}
	}
	if len(matches) != 1 {
		return resolution.ResolvedSetting{}, fmt.Errorf("setting ref is required when target %s has %d resolved settings", targetID, len(matches))
	}
	return matches[0], nil
}

func desiredTargetForSetting(repoRoot string, setting resolution.ResolvedSetting) (filedriver.Target, string, error) {
	desiredRel := filepath.ToSlash(setting.DesiredRelPath)
	targetRel, artifactRel, ok := strings.Cut(desiredRel, "/artifacts/")
	if !ok || artifactRel == "" {
		return filedriver.Target{}, "", fmt.Errorf("custom.files setting %s must use a desired artifact under artifacts/..., got %s", setting.Ref(), desiredRel)
	}
	if _, err := filedriver.ValidateRelativePath(artifactRel); err != nil {
		return filedriver.Target{}, "", fmt.Errorf("desired artifact path for %s: %w", setting.Ref(), err)
	}
	if err := rejectDesiredSymlinkPath(repoRoot, desiredRel); err != nil {
		return filedriver.Target{}, "", err
	}
	return filedriver.Target{
		LocationID:        "desired",
		Root:              filepath.Join(repoRoot, filepath.FromSlash(targetRel), "artifacts"),
		RelPath:           artifactRel,
		AllowMissingRoot:  true,
		RejectRootSymlink: true,
	}, desiredRel, nil
}

func ensureDesiredPathSafe(plan *Plan) error {
	if plan == nil {
		return fmt.Errorf("operation plan is required")
	}
	return rejectDesiredSymlinkPath(plan.RepoRoot, plan.DesiredRelPath)
}

func rejectDesiredSymlinkPath(repoRoot string, desiredRel string) error {
	root := strings.TrimSpace(repoRoot)
	if root == "" {
		return fmt.Errorf("repo root is required for desired artifact safety")
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve repo root %q: %w", repoRoot, err)
	}
	rel, err := filedriver.ValidateRelativePath(filepath.ToSlash(desiredRel))
	if err != nil {
		return &filedriver.Error{Code: filedriver.CodeUnsafePath, Op: "resolve desired", Path: desiredRel, Err: err}
	}
	current := rootAbs
	parts := strings.Split(rel, "/")
	for _, part := range parts {
		current = filepath.Join(current, filepath.FromSlash(part))
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect desired path %s: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return &filedriver.Error{Code: filedriver.CodeUnsafePath, Op: "resolve desired", Path: current, Err: fmt.Errorf("desired artifact path must not contain symlinks")}
		}
	}
	return nil
}

func applyLiveWithBackup(driver filedriver.Driver, plan *Plan, hook BackupHook) (filedriver.ApplyResult, *BackupResult, error) {
	var wrapped filedriver.BackupHook
	if hook != nil {
		wrapped = func(req filedriver.BackupRequest) (filedriver.BackupResult, error) {
			result, err := hook(BackupRequest{
				Operation:  plan.Operation,
				SettingRef: plan.Setting.Ref(),
				ResourceID: plan.ResourceID,
				Path:       req.Path,
				Before:     req.Before,
			})
			if err != nil {
				return filedriver.BackupResult{}, err
			}
			return filedriver.BackupResult{ID: result.ID, Before: result.Before}, nil
		}
	}
	applied, backup, err := driver.ApplyWithBackup(plan.LiveTarget, plan.DesiredFinalState, wrapped)
	if err != nil {
		return filedriver.ApplyResult{}, nil, err
	}
	if backup == nil {
		return applied, nil, nil
	}
	return applied, &BackupResult{ID: backup.ID, Before: backup.Before}, nil
}

func destinationTarget(plan *Plan) filedriver.Target {
	if plan.Operation == OperationSave {
		return plan.DesiredTarget
	}
	return plan.LiveTarget
}
