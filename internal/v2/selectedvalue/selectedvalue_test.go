package selectedvalue

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shpoont/dotfiles-manager/internal/v2/filedriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/inidriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/jsondriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/shpoont/dotfiles-manager/internal/v2/yamldriver"
	"github.com/stretchr/testify/require"
)

func TestPlanReadSelectedValuesAcrossDrivers(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		driverID   string
		relPath    string
		content    string
		normalizer string
	}{
		{name: "ini", driverID: recipe.IniFileDriverID, relPath: "config.ini", content: "[user]\nemail=secret@example.com\n", normalizer: inidriver.NormalizerID},
		{name: "json", driverID: recipe.JSONFileDriverID, relPath: "config.json", content: `{"user":{"email":"secret@example.com"}}`, normalizer: jsondriver.NormalizerID},
		{name: "yaml", driverID: recipe.YAMLFileDriverID, relPath: "config.yaml", content: "user:\n  email: secret@example.com\n", normalizer: yamldriver.NormalizerID},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			rec := decodeSelectedValueRecipe(t, tc.driverID, tc.relPath)
			require.NoError(t, os.WriteFile(filepath.Join(root, tc.relPath), []byte(tc.content), 0o644))

			plan, err := PlanRead(Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": root}})
			require.NoError(t, err)
			require.Equal(t, StatusOK, plan.Status)
			require.Equal(t, "test.app:identity.email", plan.SettingRef)
			require.Equal(t, "identity.email", plan.SettingID)
			require.Equal(t, "user", plan.ScopeDefault)
			require.Equal(t, tc.driverID, plan.DriverID)
			require.Equal(t, tc.relPath, plan.RelPath)
			require.Equal(t, filepath.Join(root, tc.relPath), plan.Path)
			require.True(t, plan.Current.Exists)
			require.NotEmpty(t, plan.Current.SHA256)
			require.Equal(t, tc.normalizer, plan.Current.Normalizer)
			require.Empty(t, plan.Diagnostics)

			encoded, err := json.Marshal(plan)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), "secret@example.com")
		})
	}
}

func TestPlanPreviewSelectedValuesAcrossDrivers(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		driverID   string
		relPath    string
		content    string
		desired    Desired
		normalizer string
		intent     string
	}{
		{name: "ini string", driverID: recipe.IniFileDriverID, relPath: "config.ini", content: "[user]\nemail=old@example.com\n", desired: SetString("new@example.com"), normalizer: inidriver.NormalizerID, intent: "set"},
		{name: "json string", driverID: recipe.JSONFileDriverID, relPath: "config.json", content: `{"user":{"email":"old@example.com"}}`, desired: SetString("new@example.com"), normalizer: jsondriver.NormalizerID, intent: "set"},
		{name: "yaml string", driverID: recipe.YAMLFileDriverID, relPath: "config.yaml", content: "user:\n  email: old@example.com\n", desired: SetString("new@example.com"), normalizer: yamldriver.NormalizerID, intent: "set"},
		{name: "json bool", driverID: recipe.JSONFileDriverID, relPath: "config.json", content: `{"user":{"email":"old@example.com"}}`, desired: SetBool(true), normalizer: jsondriver.NormalizerID, intent: "set"},
		{name: "yaml number", driverID: recipe.YAMLFileDriverID, relPath: "config.yaml", content: "user:\n  email: old@example.com\n", desired: SetNumber(json.Number("42")), normalizer: yamldriver.NormalizerID, intent: "set"},
		{name: "json null", driverID: recipe.JSONFileDriverID, relPath: "config.json", content: `{"user":{"email":"old@example.com"}}`, desired: SetNull(), normalizer: jsondriver.NormalizerID, intent: "set"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			rec := decodeSelectedValueRecipe(t, tc.driverID, tc.relPath)
			require.NoError(t, os.WriteFile(filepath.Join(root, tc.relPath), []byte(tc.content), 0o644))

			plan, err := PlanPreview(PreviewRequest{
				Request:            Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": root}},
				Desired:            tc.desired,
				WriteSafetyContext: trustedLocalWriteSafety(),
			})
			require.NoError(t, err)
			require.Equal(t, StatusOK, plan.Status)
			require.True(t, plan.Current.Exists)
			require.NotNil(t, plan.Desired)
			require.True(t, plan.Desired.Exists)
			require.Equal(t, tc.normalizer, plan.Current.Normalizer)
			require.Equal(t, tc.normalizer, plan.Desired.Normalizer)
			require.Equal(t, string(filedriver.ChangeUpdate), plan.ChangeKind)
			require.Equal(t, tc.intent, plan.Intent)

			encoded, err := json.Marshal(plan)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), "old@example.com")
			require.NotContains(t, string(encoded), "new@example.com")
		})
	}
}

func TestPlanPreviewDeleteIntentIsExplicitAndRedactionSafe(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	rec := decodeSelectedValueRecipe(t, recipe.JSONFileDriverID, "config.json")
	require.NoError(t, os.WriteFile(filepath.Join(root, "config.json"), []byte(`{"user":{"email":"secret@example.com"}}`), 0o644))

	plan, err := PlanPreview(PreviewRequest{
		Request:            Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": root}},
		Desired:            Delete(),
		WriteSafetyContext: trustedLocalWriteSafety(),
	})
	require.NoError(t, err)
	require.Equal(t, StatusOK, plan.Status)
	require.Equal(t, string(filedriver.ChangeDelete), plan.ChangeKind)
	require.Equal(t, IntentDelete, plan.Intent)
	require.NotNil(t, plan.Desired)
	require.False(t, plan.Desired.Exists)

	encoded, err := json.Marshal(plan)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "secret@example.com")
}

func TestPlanPreviewRequiresWriteSafetyBeforeDriverPreview(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	rec := decodeSelectedValueRecipe(t, recipe.JSONFileDriverID, "config.json")
	require.NoError(t, os.WriteFile(filepath.Join(root, "config.json"), []byte(`{"user":{"email":{"nested":true}}}`), 0o644))

	plan, err := PlanPreview(PreviewRequest{
		Request: Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": root}},
		Desired: SetString("new@example.com"),
		WriteSafetyContext: recipe.WriteSafetyContext{
			Source:  recipe.RecipeSourceLocal,
			Trusted: false,
		},
	})
	require.Error(t, err)
	require.Equal(t, StatusBlocked, plan.Status)
	requireDiagnosticCode(t, plan, "writeSafety.trust.untrusted")
	requireNoDiagnosticCode(t, plan, "selectedvalue.driver.invalid-selector")

	encoded, marshalErr := json.Marshal(plan)
	require.NoError(t, marshalErr)
	require.NotContains(t, string(encoded), "new@example.com")
}

func TestPlanPreviewRejectsUnsafeDesiredTypeCompatibility(t *testing.T) {
	t.Parallel()

	t.Run("ini accepts only string set or delete", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		rec := decodeSelectedValueRecipe(t, recipe.IniFileDriverID, "config.ini")
		require.NoError(t, os.WriteFile(filepath.Join(root, "config.ini"), []byte("[user]\nemail=old@example.com\n"), 0o644))

		plan, err := PlanPreview(PreviewRequest{
			Request:            Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": root}},
			Desired:            SetBool(true),
			WriteSafetyContext: trustedLocalWriteSafety(),
		})
		require.Error(t, err)
		require.Equal(t, StatusBlocked, plan.Status)
		requireDiagnosticCode(t, plan, "selectedvalue.desired.iniTypeUnsupported")
	})

	t.Run("json and yaml require explicit desired intent", func(t *testing.T) {
		t.Parallel()

		for _, tc := range []struct {
			name     string
			driverID string
			relPath  string
			content  string
		}{
			{name: "json", driverID: recipe.JSONFileDriverID, relPath: "config.json", content: `{"user":{"email":"old@example.com"}}`},
			{name: "yaml", driverID: recipe.YAMLFileDriverID, relPath: "config.yaml", content: "user:\n  email: old@example.com\n"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				root := t.TempDir()
				rec := decodeSelectedValueRecipe(t, tc.driverID, tc.relPath)
				require.NoError(t, os.WriteFile(filepath.Join(root, tc.relPath), []byte(tc.content), 0o644))

				plan, err := PlanPreview(PreviewRequest{
					Request:            Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": root}},
					Desired:            Desired{},
					WriteSafetyContext: trustedLocalWriteSafety(),
				})
				require.Error(t, err)
				require.Equal(t, StatusBlocked, plan.Status)
				requireDiagnosticCode(t, plan, "selectedvalue.desired.intentRequired")
			})
		}
	})

	t.Run("invalid json number is rejected before driver planning", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		rec := decodeSelectedValueRecipe(t, recipe.JSONFileDriverID, "config.json")
		require.NoError(t, os.WriteFile(filepath.Join(root, "config.json"), []byte(`{"user":{"email":"old@example.com"}}`), 0o644))

		plan, err := PlanPreview(PreviewRequest{
			Request:            Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": root}},
			Desired:            SetNumber(json.Number("01")),
			WriteSafetyContext: trustedLocalWriteSafety(),
		})
		require.Error(t, err)
		require.Equal(t, StatusBlocked, plan.Status)
		requireDiagnosticCode(t, plan, "selectedvalue.desired.invalidNumber")
	})
}

func TestPlanReadSupportsTargetQualifiedAndBareSettingRefsAndLocationOverride(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	rec := decodeSelectedValueRecipe(t, recipe.JSONFileDriverID, "config.json")
	require.NoError(t, os.WriteFile(filepath.Join(root, "config.json"), []byte(`{"user":{"email":"secret@example.com"}}`), 0o644))

	bare, err := PlanRead(Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": root}})
	require.NoError(t, err)
	require.Equal(t, "test.app:identity.email", bare.SettingRef)
	require.Equal(t, filepath.Join(root, "config.json"), bare.Path)

	qualified, err := PlanRead(Request{Recipe: rec, SettingRef: "test.app:identity.email", LocationRoots: map[string]string{"config": root}})
	require.NoError(t, err)
	require.Equal(t, bare.Path, qualified.Path)
	require.Equal(t, bare.Current.SHA256, qualified.Current.SHA256)
}

func TestPlanReadBlocksUnsupportedDriverAndDriverUnsafeCases(t *testing.T) {
	t.Parallel()

	t.Run("unsupported driver", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		rec := decodeUnsupportedDriverRecipe(t)
		require.NoError(t, os.WriteFile(filepath.Join(root, "config.txt"), []byte("secret@example.com\n"), 0o644))

		plan, err := PlanRead(Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": root}})
		require.Error(t, err)
		require.Equal(t, StatusBlocked, plan.Status)
		requireDiagnosticCode(t, plan, "selectedvalue.driver.unsupported")
	})

	t.Run("duplicate ini key", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		rec := decodeSelectedValueRecipe(t, recipe.IniFileDriverID, "config.ini")
		require.NoError(t, os.WriteFile(filepath.Join(root, "config.ini"), []byte("[user]\nemail=a@example.com\nemail=b@example.com\n"), 0o644))

		plan, err := PlanRead(Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": root}})
		require.Error(t, err)
		require.Equal(t, StatusBlocked, plan.Status)
		requireDiagnosticCode(t, plan, "selectedvalue.driver.invalid-selector")

		encoded, marshalErr := json.Marshal(plan)
		require.NoError(t, marshalErr)
		require.NotContains(t, string(encoded), "a@example.com")
		require.NotContains(t, string(encoded), "b@example.com")
	})

	t.Run("json non scalar", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		rec := decodeSelectedValueRecipe(t, recipe.JSONFileDriverID, "config.json")
		require.NoError(t, os.WriteFile(filepath.Join(root, "config.json"), []byte(`{"user":{"email":{"nested":true}}}`), 0o644))

		plan, err := PlanRead(Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": root}})
		require.Error(t, err)
		require.Equal(t, StatusBlocked, plan.Status)
		requireDiagnosticCode(t, plan, "selectedvalue.driver.invalid-selector")
	})

	t.Run("yaml non scalar", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		rec := decodeSelectedValueRecipe(t, recipe.YAMLFileDriverID, "config.yaml")
		require.NoError(t, os.WriteFile(filepath.Join(root, "config.yaml"), []byte("user:\n  email:\n    nested: true\n"), 0o644))

		plan, err := PlanRead(Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": root}})
		require.Error(t, err)
		require.Equal(t, StatusBlocked, plan.Status)
		requireDiagnosticCode(t, plan, "selectedvalue.driver.invalid-selector")
	})
}

func TestDesiredMarshalJSONDoesNotLeakRawValue(t *testing.T) {
	t.Parallel()

	encoded, err := json.Marshal(SetString("secret@example.com"))
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "secret@example.com")
	require.Contains(t, string(encoded), `"intent":"set"`)
	require.Contains(t, string(encoded), `"kind":"string"`)

	encoded, err = json.Marshal(Delete())
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"intent":"delete"`)
	require.NotContains(t, string(encoded), "secret@example.com")
}

func decodeSelectedValueRecipe(t *testing.T, driverID string, relPath string) *recipe.Recipe {
	t.Helper()

	body := selectedValueRecipe(driverID, relPath)
	rec, err := recipe.Decode("recipe.yaml", strings.NewReader(body))
	require.NoError(t, err)
	return rec
}

func decodeUnsupportedDriverRecipe(t *testing.T) *recipe.Recipe {
	t.Helper()

	body := `schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: test.app
displayName: Test App
supportLevel: experimental
capability: read-write
locations:
  config:
    default: /unused
settings:
  identity.email:
    label: User email
    supportLevel: experimental
    capability: read-write
    artifactForm: scalar
    sensitivity: personal
    redaction: redacted-for-display
    scopeDefault: user
    resource: config-email
resources:
  config-email:
    driver: file
    location: config
    path: config.txt
    capability: read-write
    sensitivity: personal
    redaction: redacted-for-display
    lifecycle: allowed
`
	rec, err := recipe.Decode("recipe.yaml", strings.NewReader(body))
	require.NoError(t, err)
	return rec
}

func selectedValueRecipe(driverID string, relPath string) string {
	selector := ""
	switch driverID {
	case recipe.IniFileDriverID:
		selector = `      section: user
      key: email
      missingSection: create
      missingKey: create
      duplicatePolicy: reject
      deleteKey: allow
`
	case recipe.JSONFileDriverID, recipe.YAMLFileDriverID:
		selector = `      path: [user, email]
      createMissing: create
      duplicatePolicy: reject
      deleteKey: allow
`
	default:
		panic(fmt.Sprintf("unsupported selected-value test driver %s", driverID))
	}

	return `schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: test.app
displayName: Test App
supportLevel: experimental
capability: read-write
locations:
  config:
    default: /unused
settingsGroups:
  identity:
    label: Identity
    supportLevel: experimental
    capability: read-write
    settings:
      - identity.email
settings:
  identity.email:
    label: User email
    supportLevel: experimental
    capability: read-write
    artifactForm: scalar
    sensitivity: personal
    redaction: redacted-for-display
    scopeDefault: user
    resource: config-email
resources:
  config-email:
    driver: ` + driverID + `
    location: config
    path: ` + relPath + `
    capability: read-write
    sensitivity: personal
    redaction: redacted-for-display
    lifecycle: allowed
    selector:
` + selector
}

func trustedLocalWriteSafety() recipe.WriteSafetyContext {
	return recipe.WriteSafetyContext{Source: recipe.RecipeSourceLocal, Trusted: true}
}

func requireDiagnosticCode(t *testing.T, plan *Plan, code string) {
	t.Helper()
	for _, diagnostic := range plan.Diagnostics {
		if diagnostic.Code == code {
			return
		}
	}
	require.Failf(t, "missing diagnostic code", "wanted %s in %+v", code, plan.Diagnostics)
}

func requireNoDiagnosticCode(t *testing.T, plan *Plan, code string) {
	t.Helper()
	for _, diagnostic := range plan.Diagnostics {
		require.NotEqual(t, code, diagnostic.Code, "unexpected diagnostic: %+v", diagnostic)
	}
}

func TestSelectedValueErrorStrings(t *testing.T) {
	t.Parallel()

	require.Equal(t, "selected value plan blocked", (*PlanError)(nil).Error())
	require.Equal(t, "selected value plan blocked", (&PlanError{}).Error())
	require.Contains(t, (&PlanError{Diagnostics: []Diagnostic{{Ref: "test.app:identity.email", Code: "test.code", Message: "test message"}}}).Error(), "test.app:identity.email[test.code]: test message")

	require.Equal(t, "selected-value desired state is invalid", (*DesiredError)(nil).Error())
	require.Equal(t, "bad desired", desiredError("selectedvalue.desired.test", "bad desired").Error())
}

func TestPlanReadBlocksInvalidRequests(t *testing.T) {
	t.Parallel()

	t.Run("nil recipe", func(t *testing.T) {
		t.Parallel()

		plan, err := PlanRead(Request{SettingRef: "identity.email"})
		require.Error(t, err)
		require.Equal(t, StatusBlocked, plan.Status)
		requireDiagnosticCode(t, plan, "selectedvalue.recipe.required")
	})

	t.Run("invalid recipe", func(t *testing.T) {
		t.Parallel()

		plan, err := PlanRead(Request{Recipe: &recipe.Recipe{}, SettingRef: "identity.email"})
		require.Error(t, err)
		require.Equal(t, StatusBlocked, plan.Status)
		requireDiagnosticCode(t, plan, "selectedvalue.recipe.invalid")
		requireDiagnosticCode(t, plan, "schema.invalid")
	})

	t.Run("setting ref errors", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		rec := decodeSelectedValueRecipe(t, recipe.JSONFileDriverID, "config.json")
		require.NoError(t, os.WriteFile(filepath.Join(root, "config.json"), []byte(`{"user":{"email":"secret@example.com"}}`), 0o644))

		for _, tc := range []struct {
			name string
			ref  string
			code string
		}{
			{name: "empty", ref: "", code: "selectedvalue.setting.invalid"},
			{name: "too many colons", ref: "test.app:identity:email", code: "selectedvalue.setting.invalid"},
			{name: "mismatched target", ref: "other.app:identity.email", code: "selectedvalue.setting.invalid"},
			{name: "unknown setting", ref: "missing.setting", code: "selectedvalue.setting.unknown"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				plan, err := PlanRead(Request{Recipe: rec, SettingRef: tc.ref, LocationRoots: map[string]string{"config": root}})
				require.Error(t, err)
				require.Equal(t, StatusBlocked, plan.Status)
				requireDiagnosticCode(t, plan, tc.code)
			})
		}
	})

	t.Run("location root missing", func(t *testing.T) {
		t.Parallel()

		rec := decodeSelectedValueRecipe(t, recipe.JSONFileDriverID, "config.json")
		missingRoot := filepath.Join(t.TempDir(), "missing")
		rec.Locations["config"] = recipe.Location{Default: missingRoot}

		plan, err := PlanRead(Request{Recipe: rec, SettingRef: "identity.email"})
		require.Error(t, err)
		require.Equal(t, StatusBlocked, plan.Status)
		requireDiagnosticCode(t, plan, "selectedvalue.driver.not-found")
		require.Equal(t, filepath.Join(missingRoot, "config.json"), plan.Path)
	})

	t.Run("symlink root rejected", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		realRoot := filepath.Join(root, "real")
		linkRoot := filepath.Join(root, "link")
		require.NoError(t, os.Mkdir(realRoot, 0o755))
		require.NoError(t, os.Symlink(realRoot, linkRoot))
		rec := decodeSelectedValueRecipe(t, recipe.JSONFileDriverID, "config.json")

		plan, err := PlanRead(Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": linkRoot}})
		require.Error(t, err)
		require.Equal(t, StatusBlocked, plan.Status)
		requireDiagnosticCode(t, plan, "selectedvalue.driver.unsafe-path")
	})
}

func TestPlanPreviewWriteSafetySourceBranches(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	rec := decodeSelectedValueRecipe(t, recipe.JSONFileDriverID, "config.json")
	require.NoError(t, os.WriteFile(filepath.Join(root, "config.json"), []byte(`{"user":{"email":"old@example.com"}}`), 0o644))

	for _, tc := range []struct {
		name string
		ctx  recipe.WriteSafetyContext
		code string
	}{
		{name: "source required", ctx: recipe.WriteSafetyContext{}, code: "writeSafety.trust.sourceRequired"},
		{name: "unsupported source", ctx: recipe.WriteSafetyContext{Source: "remote", Trusted: true}, code: "writeSafety.trust.sourceUnsupported"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := PlanPreview(PreviewRequest{
				Request:            Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": root}},
				Desired:            SetString("new@example.com"),
				WriteSafetyContext: tc.ctx,
			})
			require.Error(t, err)
			require.Equal(t, StatusBlocked, plan.Status)
			requireDiagnosticCode(t, plan, tc.code)
		})
	}
}

func TestDesiredCompatibilityAdditionalBranches(t *testing.T) {
	t.Parallel()

	t.Run("ini delete succeeds", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		rec := decodeSelectedValueRecipe(t, recipe.IniFileDriverID, "config.ini")
		require.NoError(t, os.WriteFile(filepath.Join(root, "config.ini"), []byte("[user]\nemail=old@example.com\n"), 0o644))

		plan, err := PlanPreview(PreviewRequest{
			Request:            Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": root}},
			Desired:            Delete(),
			WriteSafetyContext: trustedLocalWriteSafety(),
		})
		require.NoError(t, err)
		require.Equal(t, string(filedriver.ChangeDelete), plan.ChangeKind)
		require.Equal(t, IntentDelete, plan.Intent)
	})

	t.Run("yaml delete succeeds", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		rec := decodeSelectedValueRecipe(t, recipe.YAMLFileDriverID, "config.yaml")
		require.NoError(t, os.WriteFile(filepath.Join(root, "config.yaml"), []byte("user:\n  email: old@example.com\n"), 0o644))

		plan, err := PlanPreview(PreviewRequest{
			Request:            Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": root}},
			Desired:            Delete(),
			WriteSafetyContext: trustedLocalWriteSafety(),
		})
		require.NoError(t, err)
		require.Equal(t, string(filedriver.ChangeDelete), plan.ChangeKind)
		require.Equal(t, IntentDelete, plan.Intent)
	})

	t.Run("json yaml unsupported scalar kind", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		rec := decodeSelectedValueRecipe(t, recipe.YAMLFileDriverID, "config.yaml")
		require.NoError(t, os.WriteFile(filepath.Join(root, "config.yaml"), []byte("user:\n  email: old@example.com\n"), 0o644))

		plan, err := PlanPreview(PreviewRequest{
			Request:            Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": root}},
			Desired:            Desired{intent: IntentSet, kind: "object", value: map[string]any{"secret": "secret@example.com"}},
			WriteSafetyContext: trustedLocalWriteSafety(),
		})
		require.Error(t, err)
		require.Equal(t, StatusBlocked, plan.Status)
		requireDiagnosticCode(t, plan, "selectedvalue.desired.jsonTypeUnsupported")
		encoded, marshalErr := json.Marshal(plan)
		require.NoError(t, marshalErr)
		require.NotContains(t, string(encoded), "secret@example.com")
	})

	t.Run("internal invalid desired representations", func(t *testing.T) {
		t.Parallel()

		_, err := desiredINIState(Desired{intent: IntentSet, kind: "string", value: true})
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid internal representation")

		_, err = desiredJSONState(Desired{intent: IntentSet, kind: "string", value: true})
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid internal representation")

		_, err = desiredJSONState(Desired{intent: IntentSet, kind: "bool", value: "true"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid internal representation")

		_, err = desiredJSONState(Desired{intent: IntentSet, kind: "number", value: 42})
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid internal representation")
	})
}

func TestSelectorInfoAndDefaultStringBranches(t *testing.T) {
	t.Parallel()

	require.Equal(t, "fallback", defaultString("", "fallback"))
	require.Equal(t, "explicit", defaultString("explicit", "fallback"))
	require.Equal(t, SelectorInfo{Kind: "none"}, selectorInfo(recipe.Resource{Driver: recipe.JSONFileDriverID}))
	require.Equal(t, "unsupported", selectorInfo(recipe.Resource{Driver: "custom-driver", Selector: &recipe.Selector{}}).Kind)
}
