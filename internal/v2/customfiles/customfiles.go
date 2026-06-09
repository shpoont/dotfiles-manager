package customfiles

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shpoont/dotfiles-manager/internal/v2/filedriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/filetreedriver"
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
	Operation             Operation
	Setting               resolution.ResolvedSetting
	ResourceID            string
	Resource              recipe.Resource
	RepoRoot              string
	DesiredRelPath        string
	LiveTarget            filedriver.Target
	DesiredTarget         filedriver.Target
	TreeLiveTarget        filetreedriver.Target
	TreeDesiredTarget     filetreedriver.Target
	SourceState           filedriver.State
	DestinationState      filedriver.State
	DesiredFinalState     filedriver.State
	TreeSourceState       filetreedriver.State
	TreeDestinationState  filetreedriver.State
	TreeDesiredFinalState filetreedriver.State
	Preview               filedriver.Preview
	TreePreview           filetreedriver.Preview
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
	TreeBefore filetreedriver.State
}

type BackupResult struct {
	ID         string
	Before     filedriver.Snapshot
	TreeBefore filetreedriver.Snapshot
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
	Operation   Operation
	SettingRef  string
	DryRun      bool
	Mutated     bool
	Verified    bool
	Preview     filedriver.Preview
	TreePreview filetreedriver.Preview
	Backup      *BackupResult
}

func PlanSave(req Request) (*Plan, error) {
	return buildPlan(req, OperationSave)
}

func PlanApply(req Request) (*Plan, error) {
	return buildPlan(req, OperationApply)
}

func PlanFileSave(req Request) (*Plan, error) {
	return buildFileResourcePlan(req, OperationSave)
}

func PlanFileApply(req Request) (*Plan, error) {
	return buildFileResourcePlan(req, OperationApply)
}

func PlanFileRead(req Request) (*Plan, error) {
	return buildFileResourceReadPlan(req)
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
		Operation:   plan.Operation,
		SettingRef:  plan.Setting.Ref(),
		DryRun:      opts.DryRun,
		Preview:     plan.Preview,
		TreePreview: plan.TreePreview,
	}
	if opts.DryRun {
		return result, nil
	}

	if err := ensureDesiredPathSafe(plan); err != nil {
		return nil, err
	}

	if plan.Resource.Driver == recipe.FileTreeDriverID {
		return executeTree(plan, opts, result)
	}
	return executeFile(plan, opts, result)
}

func executeFile(plan *Plan, opts ExecuteOptions, result *Result) (*Result, error) {
	driver := filedriver.Driver{}
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

func executeTree(plan *Plan, opts ExecuteOptions, result *Result) (*Result, error) {
	driver := filetreedriver.Driver{}
	if plan.Operation == OperationApply {
		desiredState, err := driver.ReadCurrent(plan.TreeDesiredTarget)
		if err != nil {
			return nil, fmt.Errorf("read desired %s: %w", plan.Setting.Ref(), err)
		}
		plan.TreeDesiredFinalState = desiredState
		applyResult, backup, err := applyLiveTreeWithBackup(driver, plan, opts.BackupHook)
		if err != nil {
			return nil, err
		}
		result.TreePreview = applyResult.Preview
		if backup != nil {
			result.Backup = backup
		}
		result.Mutated = applyResult.Mutated
	} else {
		applyResult, err := driver.Apply(plan.TreeDesiredTarget, plan.TreeDesiredFinalState)
		if err != nil {
			return nil, err
		}
		result.TreePreview = applyResult.Preview
		result.Mutated = applyResult.Mutated
	}

	verify, err := driver.Verify(destinationTreeTarget(plan), plan.TreeDesiredFinalState)
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
	if resource.Driver == recipe.FileTreeDriverID {
		return buildTreePlan(req.Profile, setting, resourceID, resource, resourceRelPath, locationRoot, op)
	}
	if resource.Driver != recipe.FileDriverID {
		return nil, fmt.Errorf("unsupported custom.files resource driver: %s", resource.Driver)
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

func buildFileResourcePlan(req Request, op Operation) (*Plan, error) {
	plan, err := buildFileResourceReadPlan(req)
	if err != nil {
		return nil, err
	}
	plan.Operation = op
	driver := filedriver.Driver{}
	switch op {
	case OperationSave:
		if !plan.SourceState.Exists {
			return nil, fmt.Errorf("file-resource save %s blocked: live file is missing; delete/tombstone semantics are out of scope", plan.Setting.Ref())
		}
		plan.DesiredFinalState = plan.SourceState
		preview, err := driver.PreviewApply(plan.DesiredTarget, plan.SourceState)
		if err != nil {
			return nil, fmt.Errorf("preview save %s: %w", plan.Setting.Ref(), err)
		}
		plan.Preview = preview
	case OperationApply:
		liveState := plan.SourceState
		desiredState := plan.DestinationState
		if !liveState.Exists {
			return nil, fmt.Errorf("file-resource apply %s blocked: live file is missing; creating live files without a backup is out of scope", plan.Setting.Ref())
		}
		if !desiredState.Exists {
			return nil, fmt.Errorf("file-resource apply %s blocked: desired artifact is missing; delete/tombstone semantics are out of scope", plan.Setting.Ref())
		}
		plan.SourceState = desiredState
		plan.DestinationState = liveState
		plan.DesiredFinalState = desiredState
		preview, err := driver.PreviewApply(plan.LiveTarget, desiredState)
		if err != nil {
			return nil, fmt.Errorf("preview apply %s: %w", plan.Setting.Ref(), err)
		}
		plan.Preview = preview
	default:
		return nil, fmt.Errorf("unsupported file-resource operation: %s", op)
	}
	return plan, nil
}

func buildFileResourceReadPlan(req Request) (*Plan, error) {
	if req.Profile == nil {
		return nil, fmt.Errorf("resolved profile is required")
	}
	if req.Recipe == nil {
		return nil, fmt.Errorf("file-resource recipe is required")
	}
	if err := req.Recipe.Validate(); err != nil {
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
	if resource.Driver == recipe.FileTreeDriverID {
		return nil, fmt.Errorf("file-tree resource %s is not supported by the selected file-resource command path", resourceID)
	}
	if resource.Driver != recipe.FileDriverID {
		return nil, fmt.Errorf("unsupported file-resource driver: %s", resource.Driver)
	}
	resourceRelPath, err := recipe.ValidateResourcePath(resource.Path)
	if err != nil {
		return nil, fmt.Errorf("resource %s path: %w", resourceID, err)
	}
	locationRoot, err := req.Recipe.LocationRoot(resource.Location, req.LocationRoots)
	if err != nil {
		return nil, err
	}

	setting, err = withConventionalFileArtifact(setting)
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
	resolvedLive, err := filedriver.ResolveTarget(liveTarget)
	if err != nil {
		return nil, fmt.Errorf("resolve live %s: %w", setting.Ref(), err)
	}
	preview := filedriver.Preview{
		Target:     liveTarget,
		Path:       resolvedLive.AbsPath,
		Change:     driver.Diff(liveState, desiredState),
		Normalizer: filedriver.NormalizerID,
	}

	return &Plan{
		Setting:           setting,
		ResourceID:        resourceID,
		Resource:          resource,
		RepoRoot:          req.Profile.RepoRoot,
		DesiredRelPath:    desiredRel,
		LiveTarget:        liveTarget,
		DesiredTarget:     desiredTarget,
		SourceState:       liveState,
		DestinationState:  desiredState,
		DesiredFinalState: desiredState,
		Preview:           preview,
	}, nil
}

func buildTreePlan(profile *resolution.ResolvedProfile, setting resolution.ResolvedSetting, resourceID string, resource recipe.Resource, resourceRelPath string, locationRoot string, op Operation) (*Plan, error) {
	include, exclude, err := filetreedriver.NormalizeGlobs(resource.Include, resource.Exclude)
	if err != nil {
		return nil, fmt.Errorf("resource %s globs: %w", resourceID, err)
	}
	liveTarget := filetreedriver.Target{
		LocationID:        resource.Location,
		Root:              locationRoot,
		RelPath:           resourceRelPath,
		Include:           include,
		Exclude:           exclude,
		RejectRootSymlink: true,
	}
	desiredTarget, desiredRel, err := desiredTreeTargetForSetting(profile.RepoRoot, setting, include, exclude)
	if err != nil {
		return nil, err
	}

	driver := filetreedriver.Driver{}
	liveState, err := driver.ReadCurrent(liveTarget)
	if err != nil {
		return nil, fmt.Errorf("read live %s: %w", setting.Ref(), err)
	}
	desiredState, err := driver.ReadCurrent(desiredTarget)
	if err != nil {
		return nil, fmt.Errorf("read desired %s: %w", setting.Ref(), err)
	}

	plan := &Plan{
		Operation:            op,
		Setting:              setting,
		ResourceID:           resourceID,
		Resource:             resource,
		RepoRoot:             profile.RepoRoot,
		DesiredRelPath:       desiredRel,
		TreeLiveTarget:       liveTarget,
		TreeDesiredTarget:    desiredTarget,
		TreeSourceState:      liveState,
		TreeDestinationState: desiredState,
	}
	switch op {
	case OperationSave:
		plan.TreeDesiredFinalState = liveState
		preview, err := driver.PreviewApply(desiredTarget, liveState)
		if err != nil {
			return nil, fmt.Errorf("preview save %s: %w", setting.Ref(), err)
		}
		plan.TreePreview = preview
	case OperationApply:
		plan.TreeSourceState = desiredState
		plan.TreeDestinationState = liveState
		plan.TreeDesiredFinalState = desiredState
		preview, err := driver.PreviewApply(liveTarget, desiredState)
		if err != nil {
			return nil, fmt.Errorf("preview apply %s: %w", setting.Ref(), err)
		}
		plan.TreePreview = preview
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

func withConventionalFileArtifact(setting resolution.ResolvedSetting) (resolution.ResolvedSetting, error) {
	desiredRel := filepath.ToSlash(setting.DesiredRelPath)
	if strings.Contains(desiredRel, "/artifacts/") {
		return setting, nil
	}
	const settingsSuffix = "/settings.yaml"
	if !strings.HasSuffix(desiredRel, settingsSuffix) {
		return resolution.ResolvedSetting{}, fmt.Errorf("file-resource setting %s must use a desired artifact under artifacts/..., got %s", setting.Ref(), desiredRel)
	}
	artifactRel, err := filedriver.ValidateRelativePath(setting.SettingID)
	if err != nil {
		return resolution.ResolvedSetting{}, fmt.Errorf("file-resource conventional artifact for %s: %w", setting.Ref(), err)
	}
	targetRel := strings.TrimSuffix(desiredRel, settingsSuffix)
	artifactPath := filepath.ToSlash(filepath.Join(targetRel, "artifacts", artifactRel))
	setting.DesiredRelPath = filepath.FromSlash(artifactPath)
	setting.DesiredPath = filepath.Join(setting.DesiredPath, "..", "artifacts", filepath.FromSlash(artifactRel))
	setting.DesiredPath = filepath.Clean(setting.DesiredPath)
	setting.DesiredURI = fmt.Sprintf("desired://%s/%s/targets/%s/artifacts/%s", setting.Scope, setting.Subject, setting.TargetID, artifactRel)
	return setting, nil
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

func desiredTreeTargetForSetting(repoRoot string, setting resolution.ResolvedSetting, include []string, exclude []string) (filetreedriver.Target, string, error) {
	desiredRel := filepath.ToSlash(setting.DesiredRelPath)
	targetRel, artifactRel, ok := strings.Cut(desiredRel, "/artifacts/")
	if !ok || artifactRel == "" {
		return filetreedriver.Target{}, "", fmt.Errorf("custom.files setting %s must use a desired artifact under artifacts/..., got %s", setting.Ref(), desiredRel)
	}
	if _, err := filedriver.ValidateRelativePath(artifactRel); err != nil {
		return filetreedriver.Target{}, "", fmt.Errorf("desired artifact path for %s: %w", setting.Ref(), err)
	}
	if err := rejectDesiredSymlinkPath(repoRoot, desiredRel); err != nil {
		return filetreedriver.Target{}, "", err
	}
	return filetreedriver.Target{
		LocationID:        "desired",
		Root:              repoRoot,
		RelPath:           filepath.ToSlash(filepath.Join(targetRel, "artifacts", artifactRel)),
		Include:           include,
		Exclude:           exclude,
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

func applyLiveTreeWithBackup(driver filetreedriver.Driver, plan *Plan, hook BackupHook) (filetreedriver.ApplyResult, *BackupResult, error) {
	var wrapped filetreedriver.BackupHook
	if hook != nil {
		wrapped = func(req filetreedriver.BackupRequest) (filetreedriver.BackupResult, error) {
			result, err := hook(BackupRequest{
				Operation:  plan.Operation,
				SettingRef: plan.Setting.Ref(),
				ResourceID: plan.ResourceID,
				Path:       req.Path,
				TreeBefore: req.Before,
			})
			if err != nil {
				return filetreedriver.BackupResult{}, err
			}
			return filetreedriver.BackupResult{ID: result.ID, Before: result.TreeBefore}, nil
		}
	}
	applied, backup, err := driver.ApplyWithBackup(plan.TreeLiveTarget, plan.TreeDesiredFinalState, wrapped)
	if err != nil {
		return filetreedriver.ApplyResult{}, nil, err
	}
	if backup == nil {
		return applied, nil, nil
	}
	return applied, &BackupResult{ID: backup.ID, TreeBefore: backup.Before}, nil
}

func destinationTarget(plan *Plan) filedriver.Target {
	if plan.Operation == OperationSave {
		return plan.DesiredTarget
	}
	return plan.LiveTarget
}

func destinationTreeTarget(plan *Plan) filetreedriver.Target {
	if plan.Operation == OperationSave {
		return plan.TreeDesiredTarget
	}
	return plan.TreeLiveTarget
}
