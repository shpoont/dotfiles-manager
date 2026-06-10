package recipe

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteSafetyRequiresMetadataForWriteCapableRecipes(t *testing.T) {
	t.Parallel()

	rec, err := Decode("recipe.yaml", strings.NewReader(validSelectedPathRecipe("test.json", JSONFileDriverID, "config.json")))
	require.NoError(t, err)

	err = rec.ValidateWriteSafety(trustedLocalContextForRecipe(t, rec, WriteSafetyContext{}))
	require.Error(t, err)

	diagnostics := ValidationDiagnostics(err)
	requireDiagnosticCodes(t, diagnostics,
		"writeSafety.resource.lifecycle.required",
		"writeSafety.resource.redaction.required",
		"writeSafety.resource.sensitivity.required",
		"writeSafety.setting.redaction.required",
		"writeSafety.setting.sensitivity.required",
	)
	require.Empty(t, warningDiagnostics(diagnostics))
}

func TestWriteSafetyAllowsMinimalReadOnlyRecipes(t *testing.T) {
	t.Parallel()

	body := strings.ReplaceAll(validSelectedPathRecipe("test.json", JSONFileDriverID, "config.json"), "capability: read-write", "capability: read-only")
	rec, err := Decode("recipe.yaml", strings.NewReader(body))
	require.NoError(t, err)

	require.NoError(t, rec.ValidateWriteSafety(WriteSafetyContext{Source: RecipeSourceLocal}))
	require.Empty(t, rec.WriteSafetyDiagnostics(WriteSafetyContext{Source: RecipeSourceLocal}))
}

func TestWriteSafetyUsesEffectiveCapabilityOverrides(t *testing.T) {
	t.Parallel()

	body := strings.ReplaceAll(validSelectedPathRecipe("test.json", JSONFileDriverID, "config.json"), "capability: read-write", "capability: read-only")
	body = strings.Replace(body, "    capability: read-only\n    artifactForm: scalar", "    capability: import-only\n    artifactForm: scalar", 1)
	rec, err := Decode("recipe.yaml", strings.NewReader(body))
	require.NoError(t, err)

	err = rec.ValidateWriteSafety(trustedLocalContextForRecipe(t, rec, WriteSafetyContext{}))
	require.Error(t, err)
	requireDiagnosticCodes(t, ValidationDiagnostics(err),
		"writeSafety.setting.redaction.required",
		"writeSafety.setting.sensitivity.required",
	)
}

func TestWriteSafetyRequiresTrustForLocalWriteCapableRecipes(t *testing.T) {
	t.Parallel()

	rec, err := Decode("recipe.yaml", strings.NewReader(writeSafeSelectedPathRecipe("test.json", JSONFileDriverID, "config.json")))
	require.NoError(t, err)

	err = rec.ValidateWriteSafety(WriteSafetyContext{Source: RecipeSourceLocal})
	require.Error(t, err)
	requireDiagnosticCodes(t, ValidationDiagnostics(err), "writeSafety.trust.untrusted")

	err = rec.ValidateWriteSafety(WriteSafetyContext{Trusted: true})
	require.Error(t, err)
	requireDiagnosticCodes(t, ValidationDiagnostics(err), "writeSafety.trust.sourceRequired")

	err = rec.ValidateWriteSafety(WriteSafetyContext{Source: "remote-catalog", Trusted: true})
	require.Error(t, err)
	requireDiagnosticCodes(t, ValidationDiagnostics(err), "writeSafety.trust.sourceUnsupported")

	err = rec.ValidateWriteSafety(WriteSafetyContext{Source: RecipeSourceLocal, Trusted: true})
	require.Error(t, err)
	requireDiagnosticCodes(t, ValidationDiagnostics(err), "writeSafety.trust.evidenceRequired")

	require.NoError(t, rec.ValidateWriteSafety(trustedLocalContextForRecipe(t, rec, WriteSafetyContext{})))
	require.NoError(t, rec.ValidateWriteSafety(WriteSafetyContext{Source: RecipeSourceBundled}))
}

func TestWriteSafetyBlocksSensitiveOpaqueAndLifecyclePoliciesUntilContextAllowsThem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		defaultErr string
		allowedCtx WriteSafetyContext
		stillErr   string
	}{
		{
			name:       "secret sensitivity",
			body:       resourceOnlySafetyRecipe(SensitivitySecret, RedactionKnownSafe, LifecycleAllowed),
			defaultErr: "writeSafety.sensitivity.secretBlocked",
			allowedCtx: WriteSafetyContext{Source: RecipeSourceLocal, Trusted: true, AllowSensitive: true},
		},
		{
			name:       "unknown sensitivity",
			body:       resourceOnlySafetyRecipe(SensitivityUnknown, RedactionKnownSafe, LifecycleAllowed),
			defaultErr: "writeSafety.sensitivity.unknownBlocked",
			allowedCtx: WriteSafetyContext{Source: RecipeSourceLocal, Trusted: true, AllowUnknownSensitivity: true},
		},
		{
			name:       "opaque redaction",
			body:       resourceOnlySafetyRecipe(SensitivityPersonal, RedactionUnavailable, LifecycleAllowed),
			defaultErr: "writeSafety.redaction.unavailable",
			allowedCtx: WriteSafetyContext{Source: RecipeSourceLocal, Trusted: true, AllowOpaque: true},
		},
		{
			name:       "blocked save redaction",
			body:       resourceOnlySafetyRecipe(SensitivityPersonal, RedactionBlockedSave, LifecycleAllowed),
			defaultErr: "writeSafety.redaction.blockedSave",
			allowedCtx: WriteSafetyContext{Source: RecipeSourceLocal, Trusted: true, AllowOpaque: true, AllowSensitive: true, AllowUnknownSensitivity: true, HandlesLifecycleActions: true},
			stillErr:   "writeSafety.redaction.blockedSave",
		},
		{
			name:       "action lifecycle",
			body:       resourceOnlySafetyRecipe(SensitivityPersonal, RedactionKnownSafe, LifecycleQuitIfRunning),
			defaultErr: "writeSafety.lifecycle.actionRequired",
			allowedCtx: WriteSafetyContext{Source: RecipeSourceLocal, Trusted: true, HandlesLifecycleActions: true},
		},
		{
			name:       "blocked lifecycle",
			body:       resourceOnlySafetyRecipe(SensitivityPersonal, RedactionKnownSafe, LifecycleBlocked),
			defaultErr: "writeSafety.lifecycle.blocked",
			allowedCtx: WriteSafetyContext{Source: RecipeSourceLocal, Trusted: true, HandlesLifecycleActions: true},
			stillErr:   "writeSafety.lifecycle.blocked",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec, err := Decode("recipe.yaml", strings.NewReader(tc.body))
			require.NoError(t, err)

			err = rec.ValidateWriteSafety(trustedLocalContextForRecipe(t, rec, WriteSafetyContext{}))
			require.Error(t, err)
			requireDiagnosticCodes(t, ValidationDiagnostics(err), tc.defaultErr)

			err = rec.ValidateWriteSafety(trustedLocalContextForRecipe(t, rec, tc.allowedCtx))
			if tc.stillErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			requireDiagnosticCodes(t, ValidationDiagnostics(err), tc.stillErr)
		})
	}
}

func TestWriteSafetyLifecycleWarnIsNonBlockingDiagnostic(t *testing.T) {
	t.Parallel()

	rec, err := Decode("recipe.yaml", strings.NewReader(resourceOnlySafetyRecipe(SensitivityPersonal, RedactionKnownSafe, LifecycleWarn)))
	require.NoError(t, err)

	ctx := trustedLocalContextForRecipe(t, rec, WriteSafetyContext{})
	diagnostics := rec.WriteSafetyDiagnostics(ctx)
	requireDiagnosticCodes(t, diagnostics, "writeSafety.lifecycle.warn")
	require.Len(t, warningDiagnostics(diagnostics), 1)
	require.NoError(t, rec.ValidateWriteSafety(ctx))
}

func TestWriteSafetyUsesStableZshStartupWarningCode(t *testing.T) {
	t.Parallel()

	rec := BundledZshRecipe()
	diagnostics := rec.WriteSafetyDiagnostics(WriteSafetyContext{Source: RecipeSourceBundled, Trusted: true})
	requireDiagnosticCodes(t, diagnostics, ZshRiskShellStartupFileCode)
	require.NotEmpty(t, warningDiagnostics(diagnostics))
	require.Empty(t, blockingDiagnostics(diagnostics))
	require.NoError(t, rec.ValidateWriteSafety(WriteSafetyContext{Source: RecipeSourceBundled, Trusted: true}))
}

func TestWriteSafetyUsesStableTmuxManualReloadWarningCode(t *testing.T) {
	t.Parallel()

	rec := BundledTmuxRecipe()
	diagnostics := rec.WriteSafetyDiagnostics(WriteSafetyContext{Source: RecipeSourceBundled, Trusted: true})
	requireDiagnosticCodes(t, diagnostics, TmuxManualReloadWarningCode)
	require.NotEmpty(t, warningDiagnostics(diagnostics))
	require.Empty(t, blockingDiagnostics(diagnostics))
	require.NoError(t, rec.ValidateWriteSafety(WriteSafetyContext{Source: RecipeSourceBundled, Trusted: true}))
}

func TestSafetyValidationDiagnosticsDoNotEchoInvalidSafetyValues(t *testing.T) {
	t.Parallel()

	const secretLikeValue = "super-secret-token"
	body := strings.Replace(validSelectedPathRecipe("test.json", JSONFileDriverID, "config.json"), "    capability: read-write\n    artifactForm: scalar", "    capability: read-write\n    sensitivity: "+secretLikeValue+"\n    artifactForm: scalar", 1)
	body = strings.Replace(body, "    driver: json-file", "    sensitivity: "+secretLikeValue+"\n    driver: json-file", 1)

	_, err := Decode("recipe.yaml", strings.NewReader(body))
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported sensitivity classification")
	require.NotContains(t, err.Error(), secretLikeValue)

	payload, err := json.Marshal(ValidationDiagnostics(err))
	require.NoError(t, err)
	require.NotContains(t, string(payload), secretLikeValue)
}

func TestSafetyValidationRejectsInvalidRedactionAndLifecycleShapes(t *testing.T) {
	t.Parallel()

	body := strings.Replace(writeSafeSelectedPathRecipe("test.json", JSONFileDriverID, "config.json"), "redaction: redacted-for-display", "redaction: reveal-everything", 1)
	body = strings.Replace(body, "lifecycle: allowed", "lifecycle: run-app-script", 1)

	_, err := Decode("recipe.yaml", strings.NewReader(body))
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported redaction policy")
	require.Contains(t, err.Error(), "unsupported lifecycle policy")
	require.NotContains(t, err.Error(), "reveal-everything")
	require.NotContains(t, err.Error(), "run-app-script")
}

func TestLifecycleTargetValidationRequiresExplicitSafeDeclarations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		code string
	}{
		{
			name: "action lifecycle without target",
			body: lifecycleTargetValidationRecipe("quit-if-running", "", `lifecycleTargets:
  app:
    detect:
      kind: process-name
      names: ["Example App"]
    quit:
      kind: managed
`, ""),
			code: "lifecycleTarget.required",
		},
		{
			name: "unknown lifecycle target",
			body: lifecycleTargetValidationRecipe("block-if-running", "missing", `lifecycleTargets:
  app:
    detect:
      kind: process-name
      names: ["Example App"]
`, ""),
			code: "resource.lifecycleTarget.unknown",
		},
		{
			name: "glob process name rejected",
			body: lifecycleTargetValidationRecipe("block-if-running", "app", `lifecycleTargets:
  app:
    detect:
      kind: process-name
      names: ["Example*"]
`, ""),
			code: "lifecycleTarget.detect.name.invalid",
		},
		{
			name: "unsupported detector rejected",
			body: lifecycleTargetValidationRecipe("block-if-running", "app", `lifecycleTargets:
  app:
    detect:
      kind: shell
      names: ["Example App"]
`, ""),
			code: "lifecycleTarget.detect.kind.unsupported",
		},
		{
			name: "managed quit required",
			body: lifecycleTargetValidationRecipe("quit-if-running", "app", `lifecycleTargets:
  app:
    detect:
      kind: process-name
      names: ["Example App"]
    quit:
      kind: unsupported
`, ""),
			code: "lifecycleTarget.quit.unsupported",
		},
		{
			name: "managed reopen required",
			body: lifecycleTargetValidationRecipe("reopen-if-stopped-by-tool", "app", `lifecycleTargets:
  app:
    detect:
      kind: process-name
      names: ["Example App"]
    quit:
      kind: managed
    reopen:
      kind: none
`, ""),
			code: "lifecycleTarget.reopen.unsupported",
		},
		{
			name: "allowed does not require target",
			body: lifecycleTargetValidationRecipe("allowed", "", "", ""),
			code: "",
		},
		{
			name: "warn does not require target",
			body: lifecycleTargetValidationRecipe("warn", "", "", ""),
			code: "",
		},
		{
			name: "blocked does not require target",
			body: lifecycleTargetValidationRecipe("blocked", "", "", ""),
			code: "",
		},
		{
			name: "native operation action lifecycle without target",
			body: nativeOperationLifecycleValidationRecipe(),
			code: "lifecycleTarget.required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := Decode("recipe.yaml", strings.NewReader(tc.body))
			if tc.code == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			requireDiagnosticCodes(t, ValidationDiagnostics(err), tc.code)
		})
	}
}

func TestWriteSafetyHelperBranches(t *testing.T) {
	t.Parallel()

	require.Equal(t, "import-only", effectiveSettingCapability(nil, Setting{Capability: "import-only"}))
	require.Equal(t, "", effectiveSettingCapability(nil, Setting{}))
	require.Equal(t, "export-only", effectiveResourceCapability(&Recipe{Capability: "read-write"}, Resource{Capability: "export-only"}))
	require.Equal(t, "", effectiveResourceCapability(nil, Resource{}))
	require.False(t, hasErrorDiagnostics([]ValidationDiagnostic{{Code: "warn", Severity: ValidationSeverityWarning}}))
	require.True(t, hasErrorDiagnostics([]ValidationDiagnostic{{Code: "blank-severity"}}))
	require.False(t, knownRedaction("reveal-everything"))
	require.False(t, knownLifecycle("run-app-script"))
}

func lifecycleTargetValidationRecipe(lifecycle string, lifecycleTarget string, lifecycleTargets string, extra string) string {
	targetLine := ""
	if lifecycleTarget != "" {
		targetLine = "    lifecycleTarget: " + lifecycleTarget + "\n"
	}
	return `schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: lifecycle-test
displayName: Lifecycle test
supportLevel: experimental
capability: read-write
locations:
  config:
    default: ~/.example-tool
` + lifecycleTargets + `settings:
  email:
    capability: read-write
    artifactForm: native-export
    scopeDefault: user
    sensitivity: personal
    redaction: redacted-for-display
    resource: config
resources:
  config:
    driver: yaml-file
    location: config
    path: config.yaml
    sensitivity: personal
    redaction: redacted-for-display
    lifecycle: ` + lifecycle + `
` + targetLine + extra + `    selector:
      path: [user, email]
`
}

func nativeOperationLifecycleValidationRecipe() string {
	return `schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: lifecycle-test
displayName: Lifecycle test
supportLevel: experimental
capability: read-write
locations:
  config:
    default: ~/.example-tool
settings:
  email:
    scopeDefault: user
    sensitivity: personal
    redaction: redacted-for-display
    resource: config
resources:
  config:
    driver: native-export
    nativeOperation: export-settings
    nativeImportOperation: import-settings
    nativeApply:
      backup: pre-apply-export
      verify: post-import-export-hash
    capability: read-write
    sensitivity: personal
    redaction: metadata-only
nativeOperations:
  export-settings:
    kind: export
    reviewed: true
    runner: command
    platforms: [darwin]
    artifactForm: native-export
    diffMode: metadata-only
    lifecycle: allowed
    workingDirectory: temp
    timeoutSeconds: 30
    expectedExitCodes: [0]
    command:
      executable: /usr/bin/true
    stdin:
      mode: none
    stdout:
      mode: discard
    stderr:
      mode: discard
    redaction: metadata-only
  import-settings:
    kind: import
    reviewed: true
    runner: command
    platforms: [darwin]
    artifactForm: native-export
    diffMode: metadata-only
    lifecycle: block-if-running
    workingDirectory: temp
    timeoutSeconds: 30
    expectedExitCodes: [0]
    command:
      executable: /usr/bin/true
    stdin:
      mode: none
    stdout:
      mode: discard
    stderr:
      mode: discard
    redaction: metadata-only
`
}

func writeSafeSelectedPathRecipe(target string, driver string, resourcePath string) string {
	body := validSelectedPathRecipe(target, driver, resourcePath)
	body = strings.Replace(body, "    artifactForm: scalar\n    scopeDefault: user", "    artifactForm: scalar\n    sensitivity: personal\n    redaction: redacted-for-display\n    scopeDefault: user", 1)
	body = strings.Replace(body, "    driver: "+driver, "    sensitivity: personal\n    redaction: redacted-for-display\n    lifecycle: allowed\n    driver: "+driver, 1)
	return body
}

func resourceOnlySafetyRecipe(sensitivity string, redaction string, lifecycle string) string {
	lifecycleTargets := ""
	lifecycleTargetRef := ""
	if lifecycleNeedsTarget(lifecycle) {
		lifecycleTargets = `lifecycleTargets:
  app:
    displayName: Example Tool
    detect:
      kind: process-name
      names: ["example-tool"]
    quit:
      kind: managed
    reopen:
      kind: managed
`
		lifecycleTargetRef = "    lifecycleTarget: app\n"
	}
	return `schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: safety-test
displayName: Safety test
supportLevel: experimental
capability: read-write
locations:
  config:
    default: ~/.example-tool
` + lifecycleTargets + `
settings:
  read-only-setting:
    capability: read-only
    scopeDefault: user
    resource: config-value
resources:
  config-value:
    sensitivity: ` + sensitivity + `
    redaction: ` + redaction + `
    lifecycle: ` + lifecycle + `
` + lifecycleTargetRef + `
    driver: yaml-file
    location: config
    path: config.yaml
    selector:
      path: [user, email]
      createMissing: create
      duplicatePolicy: reject
`
}

func requireDiagnosticCodes(t *testing.T, diagnostics []ValidationDiagnostic, want ...string) {
	t.Helper()
	seen := map[string]bool{}
	for _, diagnostic := range diagnostics {
		seen[diagnostic.Code] = true
	}
	for _, code := range want {
		require.Truef(t, seen[code], "expected diagnostic code %q in %#v", code, diagnostics)
	}
}

func warningDiagnostics(diagnostics []ValidationDiagnostic) []ValidationDiagnostic {
	var warnings []ValidationDiagnostic
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == ValidationSeverityWarning {
			warnings = append(warnings, diagnostic)
		}
	}
	return warnings
}

func blockingDiagnostics(diagnostics []ValidationDiagnostic) []ValidationDiagnostic {
	var blocking []ValidationDiagnostic
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == "" || diagnostic.Severity == ValidationSeverityError {
			blocking = append(blocking, diagnostic)
		}
	}
	return blocking
}

func trustedLocalContextForRecipe(t *testing.T, rec *Recipe, base WriteSafetyContext) WriteSafetyContext {
	t.Helper()
	repoRoot := t.TempDir()
	stateRoot := t.TempDir()
	_, err := RecordLocalRecipeTrust(repoRoot, stateRoot, rec)
	require.NoError(t, err)
	eval, err := EvaluateRecipeTrust(repoRoot, stateRoot, RecipeSourceLocal, rec)
	require.NoError(t, err)
	require.Equalf(t, TrustStatusTrusted, eval.Status, "diagnostics: %#v", eval.Diagnostics)
	return eval.WriteSafetyContext(base)
}
