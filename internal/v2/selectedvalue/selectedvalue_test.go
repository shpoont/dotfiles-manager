package selectedvalue

import (
	stdcontext "context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shpoont/dotfiles-manager/internal/v2/filedriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/inidriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/jsondriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/macosdefaultsdriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/plistdriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/shpoont/dotfiles-manager/internal/v2/tomldriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/yamldriver"
	"github.com/stretchr/testify/require"
	"howett.net/plist"
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
		{name: "toml", driverID: recipe.TOMLFileDriverID, relPath: "config.toml", content: "[user]\nemail = 'secret@example.com'\n", normalizer: tomldriver.NormalizerID},
		{name: "plist", driverID: recipe.PlistFileDriverID, relPath: "config.plist", content: plistXMLString("secret@example.com"), normalizer: plistdriver.NormalizerID},
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
		format     string
	}{
		{name: "ini string", driverID: recipe.IniFileDriverID, relPath: "config.ini", content: "[user]\nemail=old@example.com\n", desired: SetString("new@example.com"), normalizer: inidriver.NormalizerID, intent: "set"},
		{name: "json string", driverID: recipe.JSONFileDriverID, relPath: "config.json", content: `{"user":{"email":"old@example.com"}}`, desired: SetString("new@example.com"), normalizer: jsondriver.NormalizerID, intent: "set"},
		{name: "yaml string", driverID: recipe.YAMLFileDriverID, relPath: "config.yaml", content: "user:\n  email: old@example.com\n", desired: SetString("new@example.com"), normalizer: yamldriver.NormalizerID, intent: "set"},
		{name: "toml string", driverID: recipe.TOMLFileDriverID, relPath: "config.toml", content: "[user]\nemail = 'old@example.com'\n", desired: SetString("new@example.com"), normalizer: tomldriver.NormalizerID, intent: "set"},
		{name: "plist string", driverID: recipe.PlistFileDriverID, relPath: "config.plist", content: plistXMLString("old@example.com"), desired: SetString("new@example.com"), normalizer: plistdriver.NormalizerID, intent: "set", format: plistdriver.FormatXML},
		{name: "json bool", driverID: recipe.JSONFileDriverID, relPath: "config.json", content: `{"user":{"email":"old@example.com"}}`, desired: SetBool(true), normalizer: jsondriver.NormalizerID, intent: "set"},
		{name: "yaml number", driverID: recipe.YAMLFileDriverID, relPath: "config.yaml", content: "user:\n  email: old@example.com\n", desired: SetNumber(json.Number("42")), normalizer: yamldriver.NormalizerID, intent: "set"},
		{name: "toml number", driverID: recipe.TOMLFileDriverID, relPath: "config.toml", content: "[user]\nemail = 'old@example.com'\n", desired: SetNumber(json.Number("42")), normalizer: tomldriver.NormalizerID, intent: "set"},
		{name: "plist number", driverID: recipe.PlistFileDriverID, relPath: "config.plist", content: plistXMLString("old@example.com"), desired: SetNumber(json.Number("42")), normalizer: plistdriver.NormalizerID, intent: "set", format: plistdriver.FormatXML},
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
				WriteSafetyContext: trustedLocalWriteSafety(t, rec),
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
			if tc.format != "" {
				require.Equal(t, tc.format, plan.Format)
			}

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
		WriteSafetyContext: trustedLocalWriteSafety(t, rec),
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
			WriteSafetyContext: trustedLocalWriteSafety(t, rec),
		})
		require.Error(t, err)
		require.Equal(t, StatusBlocked, plan.Status)
		requireDiagnosticCode(t, plan, "selectedvalue.desired.iniTypeUnsupported")
	})

	t.Run("json yaml and toml require explicit desired intent", func(t *testing.T) {
		t.Parallel()

		for _, tc := range []struct {
			name     string
			driverID string
			relPath  string
			content  string
		}{
			{name: "json", driverID: recipe.JSONFileDriverID, relPath: "config.json", content: `{"user":{"email":"old@example.com"}}`},
			{name: "yaml", driverID: recipe.YAMLFileDriverID, relPath: "config.yaml", content: "user:\n  email: old@example.com\n"},
			{name: "toml", driverID: recipe.TOMLFileDriverID, relPath: "config.toml", content: "[user]\nemail = 'old@example.com'\n"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				root := t.TempDir()
				rec := decodeSelectedValueRecipe(t, tc.driverID, tc.relPath)
				require.NoError(t, os.WriteFile(filepath.Join(root, tc.relPath), []byte(tc.content), 0o644))

				plan, err := PlanPreview(PreviewRequest{
					Request:            Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": root}},
					Desired:            Desired{},
					WriteSafetyContext: trustedLocalWriteSafety(t, rec),
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
			WriteSafetyContext: trustedLocalWriteSafety(t, rec),
		})
		require.Error(t, err)
		require.Equal(t, StatusBlocked, plan.Status)
		requireDiagnosticCode(t, plan, "selectedvalue.desired.invalidNumber")
	})

	t.Run("toml rejects null desired values", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		rec := decodeSelectedValueRecipe(t, recipe.TOMLFileDriverID, "config.toml")
		require.NoError(t, os.WriteFile(filepath.Join(root, "config.toml"), []byte("[user]\nemail = 'old@example.com'\n"), 0o644))

		plan, err := PlanPreview(PreviewRequest{
			Request:            Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": root}},
			Desired:            SetNull(),
			WriteSafetyContext: trustedLocalWriteSafety(t, rec),
		})
		require.Error(t, err)
		require.Equal(t, StatusBlocked, plan.Status)
		requireDiagnosticCode(t, plan, "selectedvalue.desired.tomlNullUnsupported")
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

	t.Run("toml non scalar", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		rec := decodeSelectedValueRecipe(t, recipe.TOMLFileDriverID, "config.toml")
		require.NoError(t, os.WriteFile(filepath.Join(root, "config.toml"), []byte("[user.email]\nnested = true\n"), 0o644))

		plan, err := PlanRead(Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": root}})
		require.Error(t, err)
		require.Equal(t, StatusBlocked, plan.Status)
		requireDiagnosticCode(t, plan, "selectedvalue.driver.invalid-selector")
	})
}

func TestBundledGitCaseSafetyBlocksAmbiguousIdentityKeysBeforeMutation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
	}{
		{name: "mixed case user section", content: "[User]\n\temail = old@example.com\n"},
		{name: "mixed case email key", content: "[user]\n\tEmail = old@example.com\n"},
		{name: "case duplicate user section", content: "[user]\n\temail = old@example.com\n[USER]\n\temail = other@example.com\n"},
		{name: "case duplicate email key", content: "[user]\n\temail = old@example.com\n\tEMAIL = other@example.com\n"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			home := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(home, ".gitconfig"), []byte(tc.content), 0o644))
			rec := recipe.BundledGitRecipe()
			roots := map[string]string{"home": home}
			ctx := recipe.WriteSafetyContext{Source: recipe.RecipeSourceBundled, Trusted: true}

			plan, err := PlanRead(Request{Recipe: rec, SettingRef: "git:user.email", LocationRoots: roots})
			require.Error(t, err)
			require.Equal(t, StatusBlocked, plan.Status)
			requireDiagnosticCode(t, plan, "selectedvalue.driver.invalid-selector")

			backupCalled := false
			result, err := ApplyWithBackup(PreviewRequest{
				Request:            Request{Recipe: rec, SettingRef: "git:user.email", LocationRoots: roots},
				Desired:            SetString("new@example.com"),
				WriteSafetyContext: ctx,
			}, ApplyOptions{
				BackupHook: func(req BackupRequest) (BackupResult, error) {
					backupCalled = true
					return BackupResult{ID: "backup", Before: req.Before}, nil
				},
			})
			require.Error(t, err)
			require.False(t, backupCalled)
			require.NotNil(t, result.Plan)
			requireDiagnosticCode(t, result.Plan, "selectedvalue.driver.invalid-selector")
			require.NotContains(t, readSelectedValueFile(t, filepath.Join(home, ".gitconfig")), "new@example.com")
		})
	}
}

func TestBundledStarshipTypeValidationAcceptsOnlyKnownPromptScalarTypes(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	rec := recipe.BundledStarshipRecipe()
	roots := map[string]string{"config": root}
	writeSafety := recipe.WriteSafetyContext{Source: recipe.RecipeSourceBundled, Trusted: true}
	require.NoError(t, os.WriteFile(filepath.Join(root, "starship.toml"), []byte("add_newline = true\nscan_timeout = 30\n"), 0o644))

	boolPlan, err := PlanPreview(PreviewRequest{
		Request:            Request{Recipe: rec, SettingRef: "starship:add_newline", LocationRoots: roots},
		Desired:            SetBool(false),
		WriteSafetyContext: writeSafety,
	})
	require.NoError(t, err)
	require.Equal(t, StatusOK, boolPlan.Status)
	require.Equal(t, "add_newline", boolPlan.Selector.Summary)

	intPlan, err := PlanPreview(PreviewRequest{
		Request:            Request{Recipe: rec, SettingRef: "starship:scan_timeout", LocationRoots: roots},
		Desired:            SetNumber(json.Number("10")),
		WriteSafetyContext: writeSafety,
	})
	require.NoError(t, err)
	require.Equal(t, StatusOK, intPlan.Status)
	require.Equal(t, "scan_timeout", intPlan.Selector.Summary)

	deletePlan, err := PlanPreview(PreviewRequest{
		Request:            Request{Recipe: rec, SettingRef: "starship:scan_timeout", LocationRoots: roots},
		Desired:            Delete(),
		WriteSafetyContext: writeSafety,
	})
	require.NoError(t, err)
	require.Equal(t, StatusOK, deletePlan.Status)
	require.Equal(t, IntentDelete, deletePlan.Intent)
}

func TestBundledStarshipTypeValidationBlocksWrongLiveTypesWithoutRawValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		setting  string
		content  string
		rawValue string
		code     string
	}{
		{name: "bool setting as string", setting: "starship:add_newline", content: "add_newline = 'WRONG-RAW-BOOL'\n", rawValue: "WRONG-RAW-BOOL", code: "selectedvalue.starship.boolTypeUnsupported"},
		{name: "integer setting as float", setting: "starship:scan_timeout", content: "scan_timeout = 1.5\n", rawValue: "1.5", code: "selectedvalue.starship.integerTypeUnsupported"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			rec := recipe.BundledStarshipRecipe()
			roots := map[string]string{"config": root}
			require.NoError(t, os.WriteFile(filepath.Join(root, "starship.toml"), []byte(tc.content), 0o644))

			plan, err := PlanRead(Request{Recipe: rec, SettingRef: tc.setting, LocationRoots: roots})
			require.Error(t, err)
			require.Equal(t, StatusBlocked, plan.Status)
			requireDiagnosticCode(t, plan, tc.code)
			payload, marshalErr := json.Marshal(plan)
			require.NoError(t, marshalErr)
			require.NotContains(t, string(payload), tc.rawValue)

			current, err := ReadCurrentDesired(Request{Recipe: rec, SettingRef: tc.setting, LocationRoots: roots})
			require.Error(t, err)
			require.NotNil(t, current.Plan)
			requireDiagnosticCode(t, current.Plan, tc.code)
			payload, marshalErr = json.Marshal(current.Plan)
			require.NoError(t, marshalErr)
			require.NotContains(t, string(payload), tc.rawValue)
		})
	}
}

func TestBundledStarshipTypeValidationBlocksWrongDesiredOrCurrentBeforeMutation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setting     string
		content     string
		desired     Desired
		code        string
		rawNotShown string
	}{
		{name: "bool desired as string", setting: "starship:add_newline", content: "add_newline = true\n", desired: SetString("WRONG-RAW-DESIRED"), code: "selectedvalue.starship.boolTypeUnsupported", rawNotShown: "WRONG-RAW-DESIRED"},
		{name: "integer desired as float", setting: "starship:scan_timeout", content: "scan_timeout = 30\n", desired: SetNumber(json.Number("1.5")), code: "selectedvalue.starship.integerTypeUnsupported", rawNotShown: "1.5"},
		{name: "current bool as string", setting: "starship:add_newline", content: "add_newline = 'WRONG-RAW-CURRENT'\n", desired: SetBool(false), code: "selectedvalue.starship.boolTypeUnsupported", rawNotShown: "WRONG-RAW-CURRENT"},
		{name: "current integer as string", setting: "starship:scan_timeout", content: "scan_timeout = 'WRONG-RAW-CURRENT'\n", desired: SetNumber(json.Number("30")), code: "selectedvalue.starship.integerTypeUnsupported", rawNotShown: "WRONG-RAW-CURRENT"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			path := filepath.Join(root, "starship.toml")
			rec := recipe.BundledStarshipRecipe()
			roots := map[string]string{"config": root}
			writeSafety := recipe.WriteSafetyContext{Source: recipe.RecipeSourceBundled, Trusted: true}
			require.NoError(t, os.WriteFile(path, []byte(tc.content), 0o644))

			backupCalled := false
			result, err := ApplyWithBackup(PreviewRequest{
				Request:            Request{Recipe: rec, SettingRef: tc.setting, LocationRoots: roots},
				Desired:            tc.desired,
				WriteSafetyContext: writeSafety,
			}, ApplyOptions{
				BackupHook: func(req BackupRequest) (BackupResult, error) {
					backupCalled = true
					return BackupResult{ID: "backup", Before: req.Before}, nil
				},
			})
			require.Error(t, err)
			require.False(t, backupCalled)
			require.NotNil(t, result.Plan)
			requireDiagnosticCode(t, result.Plan, tc.code)
			require.Equal(t, tc.content, readSelectedValueFile(t, path))
			payload, marshalErr := json.Marshal(result)
			require.NoError(t, marshalErr)
			require.NotContains(t, string(payload), tc.rawNotShown)
		})
	}
}

func TestBundledStarshipTypeValidationAdditionalBranches(t *testing.T) {
	t.Parallel()

	rec := recipe.BundledStarshipRecipe()
	starshipCtx := func(settingID string) context {
		return context{
			req:       Request{Recipe: rec},
			settingID: settingID,
			resource:  recipe.Resource{Driver: recipe.TOMLFileDriverID},
		}
	}

	require.NoError(t, validateStarshipDesired(context{req: Request{Recipe: rec}, settingID: "add_newline", resource: recipe.Resource{Driver: recipe.JSONFileDriverID}}, SetString("not-starship-toml")))

	for _, tc := range []struct {
		name    string
		setting string
		desired Desired
		code    string
	}{
		{name: "missing intent", setting: "add_newline", desired: Desired{}, code: "selectedvalue.desired.intentRequired"},
		{name: "unsupported intent", setting: "add_newline", desired: Desired{intent: "merge"}, code: "selectedvalue.desired.intentRequired"},
		{name: "bool invalid internal", setting: "add_newline", desired: Desired{intent: IntentSet, kind: "bool", value: "true"}, code: "selectedvalue.desired.invalid"},
		{name: "number invalid internal", setting: "scan_timeout", desired: Desired{intent: IntentSet, kind: "number", value: 42}, code: "selectedvalue.desired.invalid"},
		{name: "number invalid json", setting: "scan_timeout", desired: SetNumber(json.Number("01")), code: "selectedvalue.starship.integerTypeUnsupported"},
		{name: "negative integer", setting: "scan_timeout", desired: SetNumber(json.Number("-1")), code: "selectedvalue.starship.integerTypeUnsupported"},
		{name: "unsupported setting", setting: "format", desired: SetString("$all"), code: "selectedvalue.starship.settingUnsupported"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateStarshipDesired(starshipCtx(tc.setting), tc.desired)
			require.Error(t, err)
			var desiredErr *DesiredError
			require.True(t, errors.As(err, &desiredErr))
			require.Equal(t, tc.code, desiredErr.Code)
			require.NotContains(t, err.Error(), "$all")
			require.NotContains(t, err.Error(), "true")
		})
	}

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "starship.toml"), []byte("add_newline = true\n"), 0o644))
	plan, err := PlanPreview(PreviewRequest{
		Request:            Request{Recipe: rec, SettingRef: "starship:add_newline", LocationRoots: map[string]string{"config": root}},
		Desired:            SetString("WRONG-RAW-PLANPREVIEW"),
		WriteSafetyContext: recipe.WriteSafetyContext{Source: recipe.RecipeSourceBundled, Trusted: true},
	})
	require.Error(t, err)
	require.Equal(t, StatusBlocked, plan.Status)
	requireDiagnosticCode(t, plan, "selectedvalue.starship.boolTypeUnsupported")
	payload, marshalErr := json.Marshal(plan)
	require.NoError(t, marshalErr)
	require.NotContains(t, string(payload), "WRONG-RAW-PLANPREVIEW")
}

func TestBundledStarshipValidatesBothBoolAndIntegerSettingNames(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	rec := recipe.BundledStarshipRecipe()
	roots := map[string]string{"config": root}
	writeSafety := recipe.WriteSafetyContext{Source: recipe.RecipeSourceBundled, Trusted: true}
	require.NoError(t, os.WriteFile(filepath.Join(root, "starship.toml"), []byte("add_newline = true\nfollow_symlinks = false\nscan_timeout = 30\ncommand_timeout = 500\n"), 0o644))

	for _, tc := range []struct {
		setting string
		desired Desired
	}{
		{setting: "starship:add_newline", desired: SetBool(false)},
		{setting: "starship:follow_symlinks", desired: SetBool(true)},
		{setting: "starship:scan_timeout", desired: SetNumber(json.Number("31"))},
		{setting: "starship:command_timeout", desired: SetNumber(json.Number("501"))},
	} {
		t.Run(tc.setting, func(t *testing.T) {
			t.Parallel()
			plan, err := PlanPreview(PreviewRequest{
				Request:            Request{Recipe: rec, SettingRef: tc.setting, LocationRoots: roots},
				Desired:            tc.desired,
				WriteSafetyContext: writeSafety,
			})
			require.NoError(t, err)
			require.Equal(t, StatusOK, plan.Status)
		})
	}
}

func TestBundledStarshipDeleteIntentRemovesSelectedKeyOnly(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "starship.toml")
	rec := recipe.BundledStarshipRecipe()
	roots := map[string]string{"config": root}
	writeSafety := recipe.WriteSafetyContext{Source: recipe.RecipeSourceBundled, Trusted: true}
	const unrelated = "SECRET-LIKE-FORMAT"
	require.NoError(t, os.WriteFile(path, []byte("format = '"+unrelated+"'\nadd_newline = true\n"), 0o644))

	backupCalled := false
	result, err := ApplyWithBackup(PreviewRequest{
		Request:            Request{Recipe: rec, SettingRef: "starship:add_newline", LocationRoots: roots},
		Desired:            Delete(),
		WriteSafetyContext: writeSafety,
	}, ApplyOptions{
		BackupHook: func(req BackupRequest) (BackupResult, error) {
			backupCalled = true
			require.Contains(t, string(req.BeforeFile), unrelated)
			return BackupResult{ID: "backup", Before: req.Before}, nil
		},
	})
	require.NoError(t, err)
	require.True(t, backupCalled)
	require.True(t, result.Mutated)
	require.True(t, result.Verified)
	after := readSelectedValueFile(t, path)
	require.NotContains(t, after, "add_newline")
	require.Contains(t, after, "format")
	payload, marshalErr := json.Marshal(result)
	require.NoError(t, marshalErr)
	require.NotContains(t, string(payload), unrelated)
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

func TestDesiredFormattingDoesNotLeakRawValue(t *testing.T) {
	t.Parallel()

	secret := "sk-proj-abcdefghijklmnopqrstuvwxyz1234567890"
	desired := SetString(secret)
	for _, rendered := range []string{
		fmt.Sprintf("%v", desired),
		fmt.Sprintf("%+v", desired),
		fmt.Sprintf("%#v", desired),
		fmt.Sprintf("%q", desired),
	} {
		require.NotContains(t, rendered, secret)
		require.Contains(t, rendered, "<redacted>")
	}
}

func TestReadCurrentDesiredAndApplyWithBackupAcrossDrivers(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		driverID string
		relPath  string
		before   string
		desired  Desired
		want     string
	}{
		{name: "ini", driverID: recipe.IniFileDriverID, relPath: "config.ini", before: "[user]\nemail=old@example.com\n", desired: SetString("new@example.com"), want: "new@example.com"},
		{name: "json", driverID: recipe.JSONFileDriverID, relPath: "config.json", before: `{"user":{"email":"old@example.com"}}`, desired: SetBool(true), want: `"email": true`},
		{name: "yaml", driverID: recipe.YAMLFileDriverID, relPath: "config.yaml", before: "user:\n  email: old@example.com\n", desired: SetNumber(json.Number("42")), want: "email: 42"},
		{name: "toml", driverID: recipe.TOMLFileDriverID, relPath: "config.toml", before: "[user]\nemail = 'old@example.com'\n", desired: SetNumber(json.Number("42")), want: "email = 42"},
		{name: "plist", driverID: recipe.PlistFileDriverID, relPath: "config.plist", before: plistXMLString("old@example.com"), desired: SetNumber(json.Number("42")), want: "<integer>42</integer>"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			rec := decodeSelectedValueRecipe(t, tc.driverID, tc.relPath)
			require.NoError(t, os.WriteFile(filepath.Join(root, tc.relPath), []byte(tc.before), 0o644))

			current, err := ReadCurrentDesired(Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": root}})
			require.NoError(t, err)
			require.NotNil(t, current.Plan)
			require.True(t, current.Plan.Current.Exists)
			require.Equal(t, IntentSet, current.Desired.Intent())

			var backupReqs []BackupRequest
			result, err := ApplyWithBackup(PreviewRequest{
				Request:            Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": root}},
				Desired:            tc.desired,
				WriteSafetyContext: trustedLocalWriteSafety(t, rec),
			}, ApplyOptions{
				BackupHook: func(req BackupRequest) (BackupResult, error) {
					backupReqs = append(backupReqs, req)
					return BackupResult{ID: "backup-" + tc.name, Before: req.Before}, nil
				},
			})
			require.NoError(t, err)
			require.NotNil(t, result)
			require.True(t, result.Mutated)
			require.True(t, result.Verified)
			require.NotNil(t, result.Backup)
			require.Equal(t, "backup-"+tc.name, result.Backup.ID)
			require.Len(t, backupReqs, 1)
			require.NotEmpty(t, backupReqs[0].BeforeFile)
			require.Contains(t, string(mustReadSelectedValueFile(t, filepath.Join(root, tc.relPath))), tc.want)

			encoded, err := json.Marshal(result)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), "old@example.com")
			require.NotContains(t, string(encoded), "new@example.com")
		})
	}
}

func TestReadCurrentDesiredMissingAndJSONScalarKinds(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	rec := decodeSelectedValueRecipe(t, recipe.JSONFileDriverID, "config.json")

	missing, err := ReadCurrentDesired(Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": root}})
	require.NoError(t, err)
	require.Equal(t, IntentDelete, missing.Desired.Intent())

	require.NoError(t, os.WriteFile(filepath.Join(root, "config.json"), []byte(`{"user":{"email":null}}`), 0o644))
	nullValue, err := ReadCurrentDesired(Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": root}})
	require.NoError(t, err)
	require.Equal(t, "null", nullValue.Desired.Kind())

	require.NoError(t, os.WriteFile(filepath.Join(root, "config.json"), []byte(`{"user":{"email":"person@example.com"}}`), 0o644))
	stringValue, err := ReadCurrentDesired(Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": root}})
	require.NoError(t, err)
	require.Equal(t, "string", stringValue.Desired.Kind())
	raw, ok := stringValue.Desired.Value()
	require.True(t, ok)
	require.Equal(t, "person@example.com", raw)
}

func TestReadCurrentDesiredMissingAndTOMLScalarKinds(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	rec := decodeSelectedValueRecipe(t, recipe.TOMLFileDriverID, "config.toml")

	missing, err := ReadCurrentDesired(Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": root}})
	require.NoError(t, err)
	require.Equal(t, IntentDelete, missing.Desired.Intent())

	require.NoError(t, os.WriteFile(filepath.Join(root, "config.toml"), []byte("[user]\nemail = true\n"), 0o644))
	boolValue, err := ReadCurrentDesired(Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": root}})
	require.NoError(t, err)
	require.Equal(t, "bool", boolValue.Desired.Kind())

	require.NoError(t, os.WriteFile(filepath.Join(root, "config.toml"), []byte("[user]\nemail = 42\n"), 0o644))
	numberValue, err := ReadCurrentDesired(Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": root}})
	require.NoError(t, err)
	require.Equal(t, "number", numberValue.Desired.Kind())
}

func TestApplyWithBackupNoopAndVerificationFailure(t *testing.T) {
	t.Parallel()

	t.Run("noop", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		rec := decodeSelectedValueRecipe(t, recipe.YAMLFileDriverID, "config.yaml")
		require.NoError(t, os.WriteFile(filepath.Join(root, "config.yaml"), []byte("user:\n  email: same@example.com\n"), 0o644))

		result, err := ApplyWithBackup(PreviewRequest{
			Request:            Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": root}},
			Desired:            SetString("same@example.com"),
			WriteSafetyContext: trustedLocalWriteSafety(t, rec),
		}, ApplyOptions{BackupHook: func(req BackupRequest) (BackupResult, error) {
			require.Fail(t, "backup hook should not run for unchanged apply")
			return BackupResult{}, nil
		}})
		require.NoError(t, err)
		require.False(t, result.Mutated)
		require.True(t, result.Verified)
		require.Nil(t, result.Backup)
	})

	t.Run("verification failure after apply hook", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		rec := decodeSelectedValueRecipe(t, recipe.YAMLFileDriverID, "config.yaml")
		require.NoError(t, os.WriteFile(filepath.Join(root, "config.yaml"), []byte("user:\n  email: old@example.com\n"), 0o644))

		result, err := ApplyWithBackup(PreviewRequest{
			Request:            Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": root}},
			Desired:            SetString("new@example.com"),
			WriteSafetyContext: trustedLocalWriteSafety(t, rec),
		}, ApplyOptions{
			BackupHook: func(req BackupRequest) (BackupResult, error) {
				return BackupResult{ID: "backup", Before: req.Before}, nil
			},
			AfterApply: func(plan *Plan) error {
				return os.WriteFile(plan.Path, []byte("user:\n  email: drift@example.com\n"), 0o644)
			},
		})
		require.Error(t, err)
		require.NotNil(t, result)
		require.True(t, result.Mutated)
		require.False(t, result.Verified)
		require.NotNil(t, result.Backup)
		require.True(t, filedriver.IsCode(err, filedriver.CodeVerificationFailed), err.Error())
	})
}

func TestApplyWithBackupBlocksBeforeMutationBranches(t *testing.T) {
	t.Parallel()

	t.Run("nil recipe", func(t *testing.T) {
		t.Parallel()
		result, err := ApplyWithBackup(PreviewRequest{Desired: SetString("x")}, ApplyOptions{})
		require.Error(t, err)
		require.NotNil(t, result.Plan)
		requireDiagnosticCode(t, result.Plan, "selectedvalue.recipe.required")
	})

	t.Run("write safety", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		rec := decodeSelectedValueRecipe(t, recipe.YAMLFileDriverID, "config.yaml")
		require.NoError(t, os.WriteFile(filepath.Join(root, "config.yaml"), []byte("user:\n  email: old@example.com\n"), 0o644))
		result, err := ApplyWithBackup(PreviewRequest{
			Request:            Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": root}},
			Desired:            SetString("new@example.com"),
			WriteSafetyContext: recipe.WriteSafetyContext{Source: recipe.RecipeSourceLocal, Trusted: false},
		}, ApplyOptions{})
		require.Error(t, err)
		requireDiagnosticCode(t, result.Plan, "writeSafety.trust.untrusted")
	})

	t.Run("invalid desired for ini", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		rec := decodeSelectedValueRecipe(t, recipe.IniFileDriverID, "config.ini")
		require.NoError(t, os.WriteFile(filepath.Join(root, "config.ini"), []byte("[user]\nemail=old@example.com\n"), 0o644))
		result, err := ApplyWithBackup(PreviewRequest{
			Request:            Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": root}},
			Desired:            SetBool(true),
			WriteSafetyContext: trustedLocalWriteSafety(t, rec),
		}, ApplyOptions{})
		require.Error(t, err)
		requireDiagnosticCode(t, result.Plan, "selectedvalue.desired.iniTypeUnsupported")
	})

	t.Run("backup hook error", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		rec := decodeSelectedValueRecipe(t, recipe.JSONFileDriverID, "config.json")
		require.NoError(t, os.WriteFile(filepath.Join(root, "config.json"), []byte(`{"user":{"email":"old@example.com"}}`), 0o644))
		result, err := ApplyWithBackup(PreviewRequest{
			Request:            Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": root}},
			Desired:            SetString("new@example.com"),
			WriteSafetyContext: trustedLocalWriteSafety(t, rec),
		}, ApplyOptions{BackupHook: func(req BackupRequest) (BackupResult, error) {
			return BackupResult{}, fmt.Errorf("safe backup hook failure")
		}})
		require.Error(t, err)
		require.NotNil(t, result)
		require.False(t, result.Mutated)
	})

	t.Run("after apply hook error", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		rec := decodeSelectedValueRecipe(t, recipe.IniFileDriverID, "config.ini")
		require.NoError(t, os.WriteFile(filepath.Join(root, "config.ini"), []byte("[user]\nemail=old@example.com\n"), 0o644))
		result, err := ApplyWithBackup(PreviewRequest{
			Request:            Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": root}},
			Desired:            SetString("new@example.com"),
			WriteSafetyContext: trustedLocalWriteSafety(t, rec),
		}, ApplyOptions{AfterApply: func(plan *Plan) error {
			return fmt.Errorf("safe after apply failure")
		}})
		require.Error(t, err)
		require.NotNil(t, result)
		require.True(t, result.Mutated)
	})
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
	case recipe.JSONFileDriverID, recipe.YAMLFileDriverID, recipe.TOMLFileDriverID, recipe.PlistFileDriverID:
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

func trustedLocalWriteSafety(t *testing.T, rec *recipe.Recipe) recipe.WriteSafetyContext {
	t.Helper()
	repoRoot := t.TempDir()
	stateRoot := t.TempDir()
	_, err := recipe.RecordLocalRecipeTrust(repoRoot, stateRoot, rec)
	require.NoError(t, err)
	eval, err := recipe.EvaluateRecipeTrust(repoRoot, stateRoot, recipe.RecipeSourceLocal, rec)
	require.NoError(t, err)
	require.Equalf(t, recipe.TrustStatusTrusted, eval.Status, "diagnostics: %#v", eval.Diagnostics)
	return eval.WriteSafetyContext(recipe.WriteSafetyContext{})
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

func mustReadSelectedValueFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
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
			WriteSafetyContext: trustedLocalWriteSafety(t, rec),
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
			WriteSafetyContext: trustedLocalWriteSafety(t, rec),
		})
		require.NoError(t, err)
		require.Equal(t, string(filedriver.ChangeDelete), plan.ChangeKind)
		require.Equal(t, IntentDelete, plan.Intent)
	})

	t.Run("plist delete succeeds", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		rec := decodeSelectedValueRecipe(t, recipe.PlistFileDriverID, "config.plist")
		require.NoError(t, os.WriteFile(filepath.Join(root, "config.plist"), []byte(plistXMLString("old@example.com")), 0o644))

		plan, err := PlanPreview(PreviewRequest{
			Request:            Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": root}},
			Desired:            Delete(),
			WriteSafetyContext: trustedLocalWriteSafety(t, rec),
		})
		require.NoError(t, err)
		require.Equal(t, string(filedriver.ChangeDelete), plan.ChangeKind)
		require.Equal(t, IntentDelete, plan.Intent)
		require.Equal(t, plistdriver.FormatXML, plan.Format)
	})

	t.Run("json yaml unsupported scalar kind", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		rec := decodeSelectedValueRecipe(t, recipe.YAMLFileDriverID, "config.yaml")
		require.NoError(t, os.WriteFile(filepath.Join(root, "config.yaml"), []byte("user:\n  email: old@example.com\n"), 0o644))

		plan, err := PlanPreview(PreviewRequest{
			Request:            Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": root}},
			Desired:            Desired{intent: IntentSet, kind: "object", value: map[string]any{"secret": "secret@example.com"}},
			WriteSafetyContext: trustedLocalWriteSafety(t, rec),
		})
		require.Error(t, err)
		require.Equal(t, StatusBlocked, plan.Status)
		requireDiagnosticCode(t, plan, "selectedvalue.desired.jsonTypeUnsupported")
		encoded, marshalErr := json.Marshal(plan)
		require.NoError(t, marshalErr)
		require.NotContains(t, string(encoded), "secret@example.com")
	})

	t.Run("plist null and container desired kinds are blocked", func(t *testing.T) {
		t.Parallel()

		for _, tc := range []struct {
			name    string
			desired Desired
			code    string
		}{
			{name: "null", desired: SetNull(), code: "selectedvalue.desired.plistNullUnsupported"},
			{name: "object", desired: Desired{intent: IntentSet, kind: "object", value: map[string]any{"secret": "secret@example.com"}}, code: "selectedvalue.desired.plistTypeUnsupported"},
			{name: "array", desired: Desired{intent: IntentSet, kind: "array", value: []any{"secret@example.com"}}, code: "selectedvalue.desired.plistTypeUnsupported"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				root := t.TempDir()
				rec := decodeSelectedValueRecipe(t, recipe.PlistFileDriverID, "config.plist")
				require.NoError(t, os.WriteFile(filepath.Join(root, "config.plist"), []byte(plistXMLString("old@example.com")), 0o644))

				plan, err := PlanPreview(PreviewRequest{
					Request:            Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": root}},
					Desired:            tc.desired,
					WriteSafetyContext: trustedLocalWriteSafety(t, rec),
				})
				require.Error(t, err)
				require.Equal(t, StatusBlocked, plan.Status)
				requireDiagnosticCode(t, plan, tc.code)
				encoded, marshalErr := json.Marshal(plan)
				require.NoError(t, marshalErr)
				require.NotContains(t, string(encoded), "secret@example.com")
			})
		}
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

		_, err = desiredPlistState(Desired{intent: IntentSet, kind: "number", value: 42})
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

func TestReadCurrentDesiredAdditionalBranches(t *testing.T) {
	t.Parallel()

	t.Run("invalid request returns blocked plan", func(t *testing.T) {
		t.Parallel()

		current, err := ReadCurrentDesired(Request{SettingRef: "identity.email"})
		require.Error(t, err)
		require.NotNil(t, current)
		require.NotNil(t, current.Plan)
		requireDiagnosticCode(t, current.Plan, "selectedvalue.recipe.required")
	})

	t.Run("missing ini and yaml values become delete intents", func(t *testing.T) {
		t.Parallel()

		for _, tc := range []struct {
			name     string
			driverID string
			relPath  string
		}{
			{name: "ini", driverID: recipe.IniFileDriverID, relPath: "config.ini"},
			{name: "yaml", driverID: recipe.YAMLFileDriverID, relPath: "config.yaml"},
			{name: "plist", driverID: recipe.PlistFileDriverID, relPath: "config.plist"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				root := t.TempDir()
				rec := decodeSelectedValueRecipe(t, tc.driverID, tc.relPath)
				current, err := ReadCurrentDesired(Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": root}})
				require.NoError(t, err)
				require.Equal(t, IntentDelete, current.Desired.Intent())
				raw, ok := current.Desired.Value()
				require.False(t, ok)
				require.Nil(t, raw)
			})
		}
	})

	t.Run("json bool and number scalar values round trip as desired values", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		rec := decodeSelectedValueRecipe(t, recipe.JSONFileDriverID, "config.json")

		require.NoError(t, os.WriteFile(filepath.Join(root, "config.json"), []byte(`{"user":{"email":true}}`), 0o644))
		boolValue, err := ReadCurrentDesired(Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": root}})
		require.NoError(t, err)
		require.Equal(t, "bool", boolValue.Desired.Kind())
		raw, ok := boolValue.Desired.Value()
		require.True(t, ok)
		require.Equal(t, true, raw)

		require.NoError(t, os.WriteFile(filepath.Join(root, "config.json"), []byte(`{"user":{"email":42}}`), 0o644))
		numberValue, err := ReadCurrentDesired(Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": root}})
		require.NoError(t, err)
		require.Equal(t, "number", numberValue.Desired.Kind())
		raw, ok = numberValue.Desired.Value()
		require.True(t, ok)
		require.Equal(t, json.Number("42"), raw)
	})

	t.Run("plist bool and number scalar values round trip as desired values", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		rec := decodeSelectedValueRecipe(t, recipe.PlistFileDriverID, "config.plist")

		require.NoError(t, os.WriteFile(filepath.Join(root, "config.plist"), []byte(plistXMLRawValue("<true/>")), 0o644))
		boolValue, err := ReadCurrentDesired(Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": root}})
		require.NoError(t, err)
		require.Equal(t, "bool", boolValue.Desired.Kind())
		raw, ok := boolValue.Desired.Value()
		require.True(t, ok)
		require.Equal(t, true, raw)

		require.NoError(t, os.WriteFile(filepath.Join(root, "config.plist"), []byte(plistXMLRawValue("<integer>42</integer>")), 0o644))
		numberValue, err := ReadCurrentDesired(Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": root}})
		require.NoError(t, err)
		require.Equal(t, "number", numberValue.Desired.Kind())
		raw, ok = numberValue.Desired.Value()
		require.True(t, ok)
		require.Equal(t, json.Number("42"), raw)
	})

	t.Run("driver read failures are surfaced without raw values", func(t *testing.T) {
		t.Parallel()

		for _, tc := range []struct {
			name     string
			driverID string
			relPath  string
			content  string
		}{
			{name: "ini duplicate key", driverID: recipe.IniFileDriverID, relPath: "config.ini", content: "[user]\nemail=a@example.com\nemail=b@example.com\n"},
			{name: "json non scalar", driverID: recipe.JSONFileDriverID, relPath: "config.json", content: `{"user":{"email":{"nested":true}}}`},
			{name: "yaml non scalar", driverID: recipe.YAMLFileDriverID, relPath: "config.yaml", content: "user:\n  email:\n    nested: true\n"},
			{name: "toml non scalar", driverID: recipe.TOMLFileDriverID, relPath: "config.toml", content: "[user.email]\nnested = true\n"},
			{name: "plist non scalar", driverID: recipe.PlistFileDriverID, relPath: "config.plist", content: plistXMLRawValue("<dict><key>nested</key><true/></dict>")},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				root := t.TempDir()
				rec := decodeSelectedValueRecipe(t, tc.driverID, tc.relPath)
				require.NoError(t, os.WriteFile(filepath.Join(root, tc.relPath), []byte(tc.content), 0o644))

				current, err := ReadCurrentDesired(Request{Recipe: rec, SettingRef: "identity.email", LocationRoots: map[string]string{"config": root}})
				require.Error(t, err)
				require.NotNil(t, current.Plan)
				requireDiagnosticCode(t, current.Plan, "selectedvalue.driver.invalid-selector")
				encoded, marshalErr := json.Marshal(current.Plan)
				require.NoError(t, marshalErr)
				require.NotContains(t, string(encoded), "a@example.com")
				require.NotContains(t, string(encoded), "b@example.com")
			})
		}
	})
}

func TestSelectedValueInternalHelperAdditionalBranches(t *testing.T) {
	t.Parallel()

	_, err := desiredINIState(Desired{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "desired intent is required")

	_, err = desiredYAMLState(Desired{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "desired intent is required")

	_, err = desiredTOMLState(SetNull())
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not support null")

	_, err = desiredPlistState(Desired{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "desired intent is required")

	desired, err := desiredFromYAMLState(yamldriver.AbsentState())
	require.NoError(t, err)
	require.Equal(t, IntentDelete, desired.Intent())

	desired, err = desiredFromPlistState(plistdriver.AbsentState())
	require.NoError(t, err)
	require.Equal(t, IntentDelete, desired.Intent())

	_, err = desiredFromCanonicalJSON([]byte("{"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "could not be decoded")

	_, err = desiredFromCanonicalJSON([]byte("{}"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "not a supported scalar")

	plan := blockedPlan("test.app", "", "identity.email", "config-email", "test.code", "test message")
	require.Equal(t, "test.app:identity.email", plan.SettingRef)
	requireDiagnosticCode(t, plan, "test.code")

	defaultSeverityPlan := &Plan{}
	block(defaultSeverityPlan, Diagnostic{Code: "test.defaultSeverity"})
	require.Equal(t, SeverityError, defaultSeverityPlan.Diagnostics[0].Severity)

	require.Nil(t, iniBackupHook(&Plan{}, nil, nil))
	require.Nil(t, jsonBackupHook(&Plan{}, nil, nil))
	require.Nil(t, yamlBackupHook(&Plan{}, nil, nil))
	require.Nil(t, plistBackupHook(&Plan{}, nil, nil))

	require.NoError(t, checkGitINIIdentityCase([]byte("# comment\n[core]\n\tEmail = ignored\n[user]\n\temail = ok@example.com\n"), "email"))
	section, ok := parseGitCaseGuardSection("  [user]  ")
	require.True(t, ok)
	require.Equal(t, "user", section)
	_, ok = parseGitCaseGuardSection("; comment")
	require.False(t, ok)
	_, ok = parseGitCaseGuardSection("[")
	require.False(t, ok)
	key, ok := parseGitCaseGuardKey("  email = ok@example.com")
	require.True(t, ok)
	require.Equal(t, "email", key)
	_, ok = parseGitCaseGuardKey("  # comment")
	require.False(t, ok)
	_, ok = parseGitCaseGuardKey("no-equals")
	require.False(t, ok)
	require.NoError(t, validateGitINIIdentityCaseSafety(recipe.BundledGitRecipe(), recipe.Resource{Driver: recipe.JSONFileDriverID}, filepath.Join(t.TempDir(), "missing")))

	backupPlan := &Plan{SettingRef: "test.app:identity.email", ResourceID: "config-email", DriverID: recipe.IniFileDriverID}
	var captured *BackupResult
	iniHook := iniBackupHook(backupPlan, func(req BackupRequest) (BackupResult, error) {
		require.Equal(t, "test.app:identity.email", req.SettingRef)
		require.Equal(t, "config-email", req.ResourceID)
		require.Equal(t, "/tmp/config.ini", req.Path)
		require.Equal(t, "safe-before", string(req.BeforeFile))
		return BackupResult{ID: "backup-ini", Before: req.Before}, nil
	}, &captured)
	iniBackup, err := iniHook(inidriver.BackupRequest{
		Path:       "/tmp/config.ini",
		Before:     inidriver.State{Exists: true, SHA256: "abc123", Normalizer: inidriver.NormalizerID},
		BeforeFile: []byte("safe-before"),
	})
	require.NoError(t, err)
	require.Equal(t, "backup-ini", iniBackup.ID)
	require.NotNil(t, captured)
	require.Equal(t, "backup-ini", captured.ID)

	jsonHook := jsonBackupHook(backupPlan, func(req BackupRequest) (BackupResult, error) {
		return BackupResult{}, fmt.Errorf("safe json backup failure")
	}, &captured)
	_, err = jsonHook(jsondriver.BackupRequest{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "safe json backup failure")

	yamlHook := yamlBackupHook(backupPlan, func(req BackupRequest) (BackupResult, error) {
		return BackupResult{}, fmt.Errorf("safe yaml backup failure")
	}, &captured)
	_, err = yamlHook(yamldriver.BackupRequest{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "safe yaml backup failure")

	tomlHook := tomlBackupHook(backupPlan, func(req BackupRequest) (BackupResult, error) {
		require.Equal(t, "/tmp/config.toml", req.Path)
		require.Equal(t, "safe-toml-before", string(req.BeforeFile))
		return BackupResult{ID: "backup-toml", Before: req.Before}, nil
	}, &captured)
	tomlBackup, err := tomlHook(tomldriver.BackupRequest{
		Path:       "/tmp/config.toml",
		Before:     tomldriver.State{Exists: true, SHA256: "toml123", Normalizer: tomldriver.NormalizerID},
		BeforeFile: []byte("safe-toml-before"),
	})
	require.NoError(t, err)
	require.Equal(t, "backup-toml", tomlBackup.ID)

	tomlHook = tomlBackupHook(backupPlan, func(req BackupRequest) (BackupResult, error) {
		return BackupResult{}, fmt.Errorf("safe toml backup failure")
	}, &captured)
	_, err = tomlHook(tomldriver.BackupRequest{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "safe toml backup failure")

	plistHook := plistBackupHook(backupPlan, func(req BackupRequest) (BackupResult, error) {
		require.Equal(t, "test.app:identity.email", req.SettingRef)
		require.Equal(t, "config-email", req.ResourceID)
		require.Equal(t, "/tmp/config.plist", req.Path)
		require.Equal(t, "safe-plist-before", string(req.BeforeFile))
		return BackupResult{ID: "backup-plist", Before: req.Before}, nil
	}, &captured)
	plistBackup, err := plistHook(plistdriver.BackupRequest{
		Path:       "/tmp/config.plist",
		Before:     plistdriver.State{Exists: true, SHA256: "plist123", Normalizer: plistdriver.NormalizerID},
		BeforeFile: []byte("safe-plist-before"),
	})
	require.NoError(t, err)
	require.Equal(t, "backup-plist", plistBackup.ID)
	require.NotNil(t, captured)
	require.Equal(t, "backup-plist", captured.ID)

	plistHook = plistBackupHook(backupPlan, func(req BackupRequest) (BackupResult, error) {
		return BackupResult{}, fmt.Errorf("safe plist backup failure")
	}, &captured)
	_, err = plistHook(plistdriver.BackupRequest{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "safe plist backup failure")
}

func readSelectedValueFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

func plistXMLString(email string) string {
	return plistXMLRawValue(fmt.Sprintf("<string>%s</string>", email))
}

func plistXMLRawValue(valueXML string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
	<key>user</key>
	<dict>
		<key>email</key>
		%s
	</dict>
</dict>
</plist>
`, valueXML)
}

func TestMacOSDefaultsReadOnlyPlanReadBypassesFilesystemResolution(t *testing.T) {
	t.Parallel()

	runner := &selectedValueDefaultsRunner{result: macosdefaultsdriver.ExportResult{Stdout: defaultsExportForSelectedValue(t, map[string]any{"AppleShowAllFiles": true})}}
	rec := decodeDefaultsSelectedValueRecipe(t)
	missingOverride := filepath.Join(t.TempDir(), "missing-root")

	plan, err := PlanRead(Request{Recipe: rec, SettingRef: "show-hidden-files", LocationRoots: map[string]string{"macos-defaults": missingOverride}, MacOSDefaultsRunner: runner})
	require.NoError(t, err)
	require.Equal(t, StatusOK, plan.Status)
	require.Equal(t, recipe.MacOSDefaultsReadOnlyDriverID, plan.DriverID)
	require.True(t, plan.ReadOnly)
	require.Equal(t, "com.apple.finder", plan.RelPath)
	require.Equal(t, "defaults://current-user/com.apple.finder/AppleShowAllFiles", plan.Path)
	require.NotContains(t, plan.Path, missingOverride)
	require.Equal(t, "macos-defaults-key", plan.Selector.Kind)
	require.Equal(t, `domain="com.apple.finder" key="AppleShowAllFiles"`, plan.Selector.Summary)
	require.True(t, plan.Current.Exists)
	require.Equal(t, macosdefaultsdriver.NormalizerID, plan.Current.Normalizer)
	require.Len(t, runner.calls, 1)
}

func TestMacOSDefaultsReadOnlyPreviewAllowsMetadataDiffButBlocksLiveWrites(t *testing.T) {
	t.Parallel()

	rec := decodeDefaultsSelectedValueRecipe(t)
	runner := &selectedValueDefaultsRunner{result: macosdefaultsdriver.ExportResult{Stdout: defaultsExportForSelectedValue(t, map[string]any{"AppleShowAllFiles": "old@example.com"})}}
	plan, err := PlanPreview(PreviewRequest{Request: Request{Recipe: rec, SettingRef: "show-hidden-files", MacOSDefaultsRunner: runner}, Desired: SetString("new@example.com")})
	require.NoError(t, err)
	require.Equal(t, StatusOK, plan.Status)
	require.True(t, plan.ReadOnly)
	require.Equal(t, string(filedriver.ChangeUpdate), plan.ChangeKind)
	require.Equal(t, IntentSet, plan.Intent)
	require.NotNil(t, plan.Desired)
	require.Equal(t, macosdefaultsdriver.NormalizerID, plan.Desired.Normalizer)

	payload, err := json.Marshal(plan)
	require.NoError(t, err)
	require.Contains(t, string(payload), `"readOnly":true`)
	require.NotContains(t, string(payload), "old@example.com")
	require.NotContains(t, string(payload), "new@example.com")

	readRunner := &selectedValueDefaultsRunner{result: runner.result}
	current, err := ReadCurrentDesired(Request{Recipe: rec, SettingRef: "show-hidden-files", MacOSDefaultsRunner: readRunner})
	require.Error(t, err)
	require.NotNil(t, current.Plan)
	requireDiagnosticCode(t, current.Plan, "selectedvalue.driver.readOnly")
	require.Empty(t, readRunner.calls, "export-as-desired must be blocked before live defaults read")

	backupCalled := false
	applyRunner := &selectedValueDefaultsRunner{result: runner.result}
	result, err := ApplyWithBackup(PreviewRequest{Request: Request{Recipe: rec, SettingRef: "show-hidden-files", MacOSDefaultsRunner: applyRunner}, Desired: SetBool(false)}, ApplyOptions{BackupHook: func(req BackupRequest) (BackupResult, error) {
		backupCalled = true
		return BackupResult{ID: "backup", Before: req.Before}, nil
	}})
	require.Error(t, err)
	require.NotNil(t, result.Plan)
	requireDiagnosticCode(t, result.Plan, "selectedvalue.driver.readOnly")
	require.False(t, backupCalled)
	require.Empty(t, applyRunner.calls, "apply must be blocked before live defaults read or backup")
}

func TestMacOSDefaultsReadOnlyRejectsNullDesiredValues(t *testing.T) {
	t.Parallel()

	rec := decodeDefaultsSelectedValueRecipe(t)
	runner := &selectedValueDefaultsRunner{result: macosdefaultsdriver.ExportResult{Stdout: defaultsExportForSelectedValue(t, map[string]any{"AppleShowAllFiles": true})}}
	plan, err := PlanPreview(PreviewRequest{Request: Request{Recipe: rec, SettingRef: "show-hidden-files", MacOSDefaultsRunner: runner}, Desired: SetNull()})
	require.Error(t, err)
	require.Equal(t, StatusBlocked, plan.Status)
	requireDiagnosticCode(t, plan, "selectedvalue.desired.defaultsNullUnsupported")
}

type selectedValueDefaultsRunner struct {
	result macosdefaultsdriver.ExportResult
	err    error
	calls  []string
}

func (r *selectedValueDefaultsRunner) Export(ctx stdcontext.Context, domain string, limits macosdefaultsdriver.OutputLimits) (macosdefaultsdriver.ExportResult, error) {
	r.calls = append(r.calls, domain)
	return r.result, r.err
}

func decodeDefaultsSelectedValueRecipe(t *testing.T) *recipe.Recipe {
	t.Helper()
	rec, err := recipe.Decode("defaults-recipe.yaml", strings.NewReader(`schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: test.defaults
displayName: Test Defaults
supportLevel: experimental
capability: read-only
locations:
  macos-defaults:
    default: macos-defaults://current-user
settings:
  show-hidden-files:
    label: Show hidden files
    supportLevel: experimental
    capability: read-only
    artifactForm: scalar
    sensitivity: low
    redaction: known-safe
    scopeDefault: user
    resource: finder-show-hidden
resources:
  finder-show-hidden:
    driver: macos-defaults-readonly
    location: macos-defaults
    path: com.apple.finder
    capability: read-only
    sensitivity: low
    redaction: known-safe
    selector:
      key: AppleShowAllFiles
`))
	require.NoError(t, err)
	return rec
}

func defaultsExportForSelectedValue(t *testing.T, value map[string]any) []byte {
	t.Helper()
	data, err := plist.Marshal(value, plist.XMLFormat)
	require.NoError(t, err)
	return data
}
