package nativeapply

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shpoont/dotfiles-manager/internal/v2/nativeexport"
	"github.com/shpoont/dotfiles-manager/internal/v2/nativeops"
	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/shpoont/dotfiles-manager/internal/v2/resolution"
	"github.com/stretchr/testify/require"
)

func TestBuildPlanAndPrepareDesiredInputValidateManagerOwnedPayload(t *testing.T) {
	t.Parallel()

	repoRoot := evalSymlinksForNativeApplyTest(t, t.TempDir())
	stateRoot := evalSymlinksForNativeApplyTest(t, t.TempDir())
	rec := nativeApplyTestRecipe()
	setting := nativeApplyTestSetting(repoRoot)
	resource := rec.Resources["settings"]
	writeNativeApplyDesiredArtifact(t, setting.DesiredPath, setting, rec, "desired-secret")

	opts := Options{
		RepoRoot:      repoRoot,
		StateRoot:     stateRoot,
		Recipe:        rec,
		RecipeSource:  recipe.RecipeSourceBundled,
		Setting:       setting,
		ResourceID:    "settings",
		Resource:      resource,
		MachineID:     "mbp",
		UserID:        "leon",
		RunID:         "run-native-apply",
		LocationRoots: map[string]string{},
		Now: func() time.Time {
			return time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
		},
	}
	plan, err := BuildPlan(opts)
	require.NoError(t, err)
	require.Equal(t, StatusReady, plan.Status)
	require.True(t, plan.DesiredSummary.Exists)
	require.NotEmpty(t, plan.DesiredSummary.SHA256)

	input, err := PrepareDesiredInput(opts, plan)
	require.NoError(t, err)
	require.Contains(t, input.Root, filepath.Join(stateRoot, "temp", "native-apply"))
	require.NotContains(t, input.Payload, repoRoot)
	require.Equal(t, plan.DesiredSummary.SHA256, input.Summary.SHA256)
	require.Equal(t, "desired-secret", readNativeApplyFile(t, filepath.Join(input.Payload, "bundle.txt")))

	badPlan := plan
	badPlan.DesiredSummary.SHA256 = "wrong"
	_, err = PrepareDesiredInput(opts, badPlan)
	require.ErrorContains(t, err, "hash")

	missingSource := opts
	missingSource.Setting.DesiredPath = filepath.Join(repoRoot, "missing")
	_, err = PrepareDesiredInput(missingSource, plan)
	require.Error(t, err)
}

func TestBuildPlanBlocksUnsupportedNativeApplyPolicies(t *testing.T) {
	t.Parallel()

	repoRoot := evalSymlinksForNativeApplyTest(t, t.TempDir())
	rec := nativeApplyTestRecipe()
	setting := nativeApplyTestSetting(repoRoot)
	writeNativeApplyDesiredArtifact(t, setting.DesiredPath, setting, rec, "desired")

	tests := []struct {
		name string
		edit func(*recipe.Resource)
		code string
	}{
		{
			name: "missing import operation",
			edit: func(resource *recipe.Resource) { resource.NativeImportOperation = "" },
			code: "nativeapply.operation.invalid",
		},
		{
			name: "unsupported backup",
			edit: func(resource *recipe.Resource) { resource.NativeApply.Backup = "none" },
			code: "nativeapply.backup.policyUnsupported",
		},
		{
			name: "unsupported verify",
			edit: func(resource *recipe.Resource) { resource.NativeApply.Verify = "manual" },
			code: "nativeapply.verify.policyUnsupported",
		},
		{
			name: "lifecycle action blocked",
			edit: func(resource *recipe.Resource) { resource.Lifecycle = recipe.LifecycleQuitIfRunning },
			code: "nativeapply.lifecycle.blocked",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			resource := rec.Resources["settings"]
			tc.edit(&resource)
			plan, err := BuildPlan(Options{RepoRoot: repoRoot, StateRoot: evalSymlinksForNativeApplyTest(t, t.TempDir()), Recipe: rec, RecipeSource: recipe.RecipeSourceBundled, Setting: setting, ResourceID: "settings", Resource: resource})
			require.Error(t, err)
			require.Equal(t, StatusBlocked, plan.Status)
			require.Equal(t, tc.code, plan.Diagnostic.Code)
		})
	}
}

func TestBuildPlanBlocksMissingDesiredAndPayloadMismatches(t *testing.T) {
	t.Parallel()

	repoRoot := evalSymlinksForNativeApplyTest(t, t.TempDir())
	rec := nativeApplyTestRecipe()
	setting := nativeApplyTestSetting(repoRoot)
	resource := rec.Resources["settings"]

	missing, err := BuildPlan(Options{RepoRoot: repoRoot, StateRoot: evalSymlinksForNativeApplyTest(t, t.TempDir()), Recipe: rec, RecipeSource: recipe.RecipeSourceBundled, Setting: setting, ResourceID: "settings", Resource: resource})
	require.Error(t, err)
	require.Equal(t, "nativeapply.desired.missing", missing.Diagnostic.Code)

	writeNativeApplyDesiredArtifact(t, setting.DesiredPath, setting, rec, "desired")
	require.NoError(t, os.WriteFile(filepath.Join(setting.DesiredPath, nativeexport.PayloadDir, "bundle.txt"), []byte("tampered"), 0o644))
	mismatch, err := BuildPlan(Options{RepoRoot: repoRoot, StateRoot: evalSymlinksForNativeApplyTest(t, t.TempDir()), Recipe: rec, RecipeSource: recipe.RecipeSourceBundled, Setting: setting, ResourceID: "settings", Resource: resource})
	require.Error(t, err)
	require.Equal(t, "nativeapply.payload.hashMismatch", mismatch.Diagnostic.Code)
}

func TestRunImportUsesPreparedInputAndReportsNativeFailure(t *testing.T) {
	t.Parallel()

	repoRoot := evalSymlinksForNativeApplyTest(t, t.TempDir())
	stateRoot := evalSymlinksForNativeApplyTest(t, t.TempDir())
	rec := nativeApplyTestRecipe()
	setting := nativeApplyTestSetting(repoRoot)
	resource := rec.Resources["settings"]
	writeNativeApplyDesiredArtifact(t, setting.DesiredPath, setting, rec, "desired")
	liveLocationRoot := filepath.Join(stateRoot, "live-location")
	base := Options{RepoRoot: repoRoot, StateRoot: stateRoot, Recipe: rec, RecipeSource: recipe.RecipeSourceBundled, Setting: setting, ResourceID: "settings", Resource: resource, RunID: "run-import", LocationRoots: map[string]string{"home": liveLocationRoot}}
	plan, err := BuildPlan(base)
	require.NoError(t, err)
	input, err := PrepareDesiredInput(base, plan)
	require.NoError(t, err)

	executor := &nativeApplyImportExecutor{}
	result, err := RunImport(context.Background(), withNativeApplyExecutor(base, executor), plan, input)
	require.NoError(t, err)
	require.Equal(t, nativeops.StatusSucceeded, result.Status)
	require.Equal(t, "desired", executor.imported)
	require.Len(t, executor.args, 2)
	require.Contains(t, executor.args[1], filepath.Join(stateRoot, "temp", "native-apply"))
	require.NotContains(t, executor.args[1], repoRoot)
	require.NotContains(t, strings.Join(executor.args, "\x00"), liveLocationRoot)
	require.NotContains(t, strings.Join(executor.env, "\x00"), liveLocationRoot)

	failing := &nativeApplyImportExecutor{result: nativeops.ExecResult{ExitCode: 3, Err: errors.New("boom")}}
	result, err = RunImport(context.Background(), withNativeApplyExecutor(base, failing), plan, input)
	require.Error(t, err)
	require.Equal(t, nativeops.StatusFailed, result.Status)
	require.Contains(t, err.Error(), "native operation exited with code 3")
}

func TestNativeApplyHelperBranches(t *testing.T) {
	t.Parallel()

	resource := recipe.Resource{NativeImportOperation: "import-settings"}
	require.True(t, ImportCapable(resource))
	require.True(t, ImportCapable(recipe.Resource{NativeApply: recipe.NativeApplyPolicy{Backup: BackupPreApplyExport}}))
	require.False(t, ImportCapable(recipe.Resource{}))

	review := ReviewDiagnostic("native.app:settings", Plan{BackupPolicy: BackupPreApplyExport, VerifyPolicy: VerifyPostImportExportHash})
	require.Equal(t, ReviewCode, review.Code)
	require.Contains(t, review.Message, BackupPreApplyExport)

	require.Equal(t, "nativeapply.payload.maxBytes", payloadDiagnostic(errors.New("maxBytes"), "ref").Code)
	require.Equal(t, "nativeapply.payload.maxEntries", payloadDiagnostic(errors.New("maxEntries"), "ref").Code)
	require.Equal(t, "nativeapply.payload.unsupportedFileType", payloadDiagnostic(errors.New("unsupported symlink"), "ref").Code)
	require.Equal(t, "nativeapply.payload.invalid", payloadDiagnostic(nil, "ref").Code)
	require.Equal(t, "nativeexport.desired.metadataInvalid", desiredDiagnostic(nativeexport.DesiredRead{Status: "blocked", Diagnostic: nativeexport.Diagnostic{Code: "nativeexport.desired.metadataInvalid", Message: "bad", Path: "metadata.json"}}, resolution.ResolvedSetting{}).Code)
	require.Equal(t, "nativeapply.desired.invalid", desiredDiagnostic(nativeexport.DesiredRead{Status: "weird"}, resolution.ResolvedSetting{}).Code)
	require.Equal(t, "fallback", firstNativeMessage(nativeops.Result{}, "fallback"))
	require.Equal(t, "native said no", firstNativeMessage(nativeops.Result{Diagnostics: []nativeops.Diagnostic{{Message: "native said no"}}}, "fallback"))
	require.Equal(t, "fallback", defaultString("", "fallback"))
	require.Equal(t, "value", defaultString("value", "fallback"))
	require.Equal(t, "native-apply", safePrefix("...///"))

	_, err := cleanAbs("")
	require.Error(t, err)
	_, err = PrepareDesiredInput(Options{StateRoot: ""}, Plan{})
	require.Error(t, err)
	_, err = createTempRoot(Options{StateRoot: ""}, "import")
	require.Error(t, err)
}

func TestOperationsRejectInvalidNativeApplyShapes(t *testing.T) {
	t.Parallel()

	rec := nativeApplyTestRecipe()
	resource := rec.Resources["settings"]

	_, _, _, err := operations(Options{})
	require.ErrorContains(t, err, "recipe is required")
	_, _, _, err = operations(Options{Recipe: rec, Resource: recipe.Resource{Driver: recipe.JSONFileDriverID}})
	require.ErrorContains(t, err, "resource driver")

	missingExport := nativeApplyTestRecipe()
	delete(missingExport.NativeOperations, "export-settings")
	_, _, _, err = operations(Options{Recipe: missingExport, Resource: resource})
	require.ErrorContains(t, err, "export operation")

	wrongExport := nativeApplyTestRecipe()
	exportOp := wrongExport.NativeOperations["export-settings"]
	exportOp.Kind = "verify"
	wrongExport.NativeOperations["export-settings"] = exportOp
	_, _, _, err = operations(Options{Recipe: wrongExport, Resource: resource})
	require.ErrorContains(t, err, "export operation")

	missingImport := nativeApplyTestRecipe()
	delete(missingImport.NativeOperations, "import-settings")
	_, _, _, err = operations(Options{Recipe: missingImport, Resource: resource})
	require.ErrorContains(t, err, "nativeImportOperation")

	wrongImport := nativeApplyTestRecipe()
	importOp := wrongImport.NativeOperations["import-settings"]
	importOp.Kind = "verify"
	wrongImport.NativeOperations["import-settings"] = importOp
	_, _, _, err = operations(Options{Recipe: wrongImport, Resource: resource})
	require.ErrorContains(t, err, "nativeImportOperation")

	withVerify := nativeApplyTestRecipe()
	verifyResource := withVerify.Resources["settings"]
	verifyResource.NativeVerifyOperation = "verify-settings"
	withVerify.Resources["settings"] = verifyResource
	_, _, _, err = operations(Options{Recipe: withVerify, Resource: verifyResource})
	require.ErrorContains(t, err, "nativeVerifyOperation")
	verifyOp := nativeApplyTestImportOperation()
	verifyOp.Kind = "verify"
	withVerify.NativeOperations["verify-settings"] = verifyOp
	_, _, verify, err := operations(Options{Recipe: withVerify, Resource: verifyResource})
	require.NoError(t, err)
	require.Equal(t, "verify", verify.Kind)
}

func TestVerifyPostImportRequiresMatchingNativeExportHash(t *testing.T) {
	t.Parallel()

	desired := nativeexport.PayloadSummary{Exists: true, SHA256: "desired", Normalizer: nativeexport.Normalizer}
	require.NoError(t, VerifyPostImport(desired, desired))
	require.Error(t, VerifyPostImport(desired, nativeexport.PayloadSummary{Exists: true, SHA256: "other", Normalizer: nativeexport.Normalizer}))
	require.Error(t, VerifyPostImport(nativeexport.PayloadSummary{}, desired))
	require.Error(t, VerifyPostImport(desired, nativeexport.PayloadSummary{}))
}

func evalSymlinksForNativeApplyTest(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	require.NoError(t, err)
	return resolved
}

type nativeApplyImportExecutor struct {
	args     []string
	env      []string
	imported string
	result   nativeops.ExecResult
}

func (e *nativeApplyImportExecutor) Run(ctx context.Context, spec nativeops.ExecSpec) nativeops.ExecResult {
	e.args = append([]string(nil), spec.Args...)
	e.env = append([]string(nil), spec.Env...)
	if e.result.Err != nil || e.result.ExitCode != 0 {
		return e.result
	}
	if len(spec.Args) >= 2 {
		data, err := os.ReadFile(spec.Args[1])
		if err != nil {
			return nativeops.ExecResult{ExitCode: 1, Err: err}
		}
		e.imported = string(data)
	}
	return nativeops.ExecResult{ExitCode: 0, Stdout: nativeops.CaptureSummary{Mode: spec.Stdout.Mode}, Stderr: nativeops.CaptureSummary{Mode: spec.Stderr.Mode}}
}

func withNativeApplyExecutor(opts Options, executor nativeops.Executor) Options {
	opts.Executor = executor
	return opts
}

func nativeApplyTestRecipe() *recipe.Recipe {
	return &recipe.Recipe{
		Schema:        recipe.Schema,
		SchemaVersion: recipe.SupportedVersion,
		Target:        "native.app",
		DisplayName:   "Native App",
		SupportLevel:  "experimental",
		Capability:    "read-write",
		Settings: map[string]recipe.Setting{
			"settings": {
				Label:        "Settings",
				SupportLevel: "experimental",
				Capability:   "read-write",
				ArtifactForm: "native-export",
				Sensitivity:  recipe.SensitivityPersonal,
				Redaction:    recipe.RedactionRedactedForDisplay,
				Lifecycle:    recipe.LifecycleAllowed,
				ScopeDefault: "user",
				Resource:     "settings",
			},
		},
		Resources: map[string]recipe.Resource{
			"settings": {
				Driver:                recipe.NativeExportDriverID,
				NativeOperation:       "export-settings",
				NativeImportOperation: "import-settings",
				Capability:            "read-write",
				Sensitivity:           recipe.SensitivityPersonal,
				Redaction:             recipe.RedactionRedactedForDisplay,
				Lifecycle:             recipe.LifecycleAllowed,
				NativeApply:           recipe.NativeApplyPolicy{Backup: BackupPreApplyExport, Verify: VerifyPostImportExportHash},
			},
		},
		NativeOperations: map[string]recipe.NativeOperation{
			"export-settings": nativeApplyTestExportOperation(),
			"import-settings": nativeApplyTestImportOperation(),
		},
	}
}

func nativeApplyTestExportOperation() recipe.NativeOperation {
	return recipe.NativeOperation{
		Kind:              "export",
		Reviewed:          true,
		Runner:            "command",
		Platforms:         []string{"darwin", "linux"},
		ArtifactForm:      "native-export",
		DiffMode:          "metadata-only",
		Lifecycle:         recipe.LifecycleAllowed,
		WorkingDirectory:  "temp",
		TimeoutSeconds:    5,
		ExpectedExitCodes: []int{0},
		Command:           recipe.NativeCommand{Executable: "/usr/bin/native-safe-tool", Args: []recipe.NativeArg{{Literal: "export"}, {Output: "bundle"}}},
		Stdin:             recipe.NativeStdinPolicy{Mode: "none"},
		Stdout:            recipe.NativeStreamPolicy{Mode: "discard"},
		Stderr:            recipe.NativeStreamPolicy{Mode: "discard"},
		Outputs:           map[string]recipe.NativePathSpec{"bundle": {Root: "artifact", Path: "bundle.txt"}},
		Redaction:         "metadata-only",
		Limits:            recipe.NativeExportLimits{MaxBytes: 1024, MaxEntries: 10},
		ExportMetadata:    recipe.NativeExportMetadataPolicy{CapturedCategories: []string{"settings"}},
	}
}

func nativeApplyTestImportOperation() recipe.NativeOperation {
	return recipe.NativeOperation{
		Kind:              "import",
		Reviewed:          true,
		Runner:            "command",
		Platforms:         []string{"darwin", "linux"},
		ArtifactForm:      "native-export",
		DiffMode:          "metadata-only",
		Lifecycle:         recipe.LifecycleAllowed,
		WorkingDirectory:  "temp",
		TimeoutSeconds:    5,
		ExpectedExitCodes: []int{0},
		Command:           recipe.NativeCommand{Executable: "/usr/bin/native-safe-tool", Args: []recipe.NativeArg{{Literal: "import"}, {Input: "bundle"}}},
		Stdin:             recipe.NativeStdinPolicy{Mode: "none"},
		Stdout:            recipe.NativeStreamPolicy{Mode: "discard"},
		Stderr:            recipe.NativeStreamPolicy{Mode: "discard"},
		Inputs:            map[string]recipe.NativePathSpec{"bundle": {Root: "artifact", Path: "bundle.txt"}},
		Redaction:         "metadata-only",
		Limits:            recipe.NativeExportLimits{MaxBytes: 1024, MaxEntries: 10},
	}
}

func nativeApplyTestSetting(repoRoot string) resolution.ResolvedSetting {
	return resolution.ResolvedSetting{
		TargetID:       "native.app",
		SettingID:      "settings",
		Scope:          "user",
		Subject:        "leon",
		DesiredURI:     "desired://user/leon/targets/native.app/artifacts/settings",
		DesiredRelPath: filepath.Join("desired", "user", "leon", "targets", "native.app", "artifacts", "settings"),
		DesiredPath:    filepath.Join(repoRoot, "desired", "user", "leon", "targets", "native.app", "artifacts", "settings"),
	}
}

func writeNativeApplyDesiredArtifact(t *testing.T, artifactRoot string, setting resolution.ResolvedSetting, rec *recipe.Recipe, body string) {
	t.Helper()
	payloadRoot := filepath.Join(artifactRoot, nativeexport.PayloadDir)
	require.NoError(t, os.MkdirAll(payloadRoot, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "bundle.txt"), []byte(body), 0o644))
	summary, err := nativeexport.SummarizePayload(payloadRoot, nativeexport.EffectiveLimits(rec.NativeOperations["export-settings"]))
	require.NoError(t, err)
	require.NoError(t, nativeexport.WriteMetadata(artifactRoot, nativeexport.Metadata{
		Schema:        nativeexport.MetadataSchema,
		SchemaVersion: nativeexport.SchemaVersion,
		TargetRef:     setting.TargetID,
		SettingRef:    setting.Ref(),
		ResourceID:    "settings",
		OperationID:   "export-settings",
		Recipe:        nativeexport.RecipeMetadata{Source: recipe.RecipeSourceBundled, TrustStatus: string(recipe.TrustStatusTrusted)},
		Operation:     nativeexport.OperationMetadata{ArtifactForm: "native-export", DiffMode: "metadata-only", Redaction: "metadata-only", OutputIDs: []string{"bundle"}},
		Source:        nativeexport.SourceMetadata{Scope: setting.Scope, Subject: setting.Subject, MachineID: "mbp", UserID: "leon"},
		CapturedAt:    "2026-06-10T12:00:00Z",
		Payload:       summary,
		Native:        nativeexport.NativeRunMetadata{Status: nativeexport.StatusSucceeded},
	}))
}

func readNativeApplyFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}
