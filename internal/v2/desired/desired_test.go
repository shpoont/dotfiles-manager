package desired

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/shpoont/dotfiles-manager/internal/v2/resolution"
	"github.com/stretchr/testify/require"
)

func TestResolveURIMapsCanonicalScopesAndObjects(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cases := []struct {
		name       string
		uri        string
		wantScope  string
		wantSubj   string
		wantObject string
		wantRel    string
		wantSet    string
		wantArt    string
	}{
		{
			name:       "shared settings",
			uri:        "desired://shared/-/targets/git/settings#user.email",
			wantScope:  "shared",
			wantSubj:   "-",
			wantObject: ObjectSettings,
			wantRel:    filepath.Join("desired", "shared", "-", "targets", "git", "settings.yaml"),
			wantSet:    "user.email",
		},
		{
			name:       "user settings",
			uri:        "desired://user/leon/targets/git/settings#user.email",
			wantScope:  "user",
			wantSubj:   "leon",
			wantObject: ObjectSettings,
			wantRel:    filepath.Join("desired", "user", "leon", "targets", "git", "settings.yaml"),
			wantSet:    "user.email",
		},
		{
			name:       "machine manifest",
			uri:        "desired://machine/mbp-2026/targets/git/manifest",
			wantScope:  "machine",
			wantSubj:   "mbp-2026",
			wantObject: ObjectManifest,
			wantRel:    filepath.Join("desired", "machine", "mbp-2026", "targets", "git", "manifest.yaml"),
		},
		{
			name:       "machine user artifact",
			uri:        "desired://machine-user/mbp-2026/leon/targets/raycast/artifacts/settings/config.json",
			wantScope:  "machine-user",
			wantSubj:   "mbp-2026/leon",
			wantObject: ObjectArtifact,
			wantRel:    filepath.Join("desired", "machine-user", "mbp-2026", "leon", "targets", "raycast", "artifacts", "settings", "config.json"),
			wantArt:    "settings/config.json",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			resolved, err := ResolveURI(root, tc.uri)
			require.NoError(t, err)
			require.Equal(t, tc.uri, resolved.URI)
			require.Equal(t, tc.wantScope, resolved.Scope)
			require.Equal(t, tc.wantSubj, resolved.Subject)
			require.Equal(t, tc.wantObject, resolved.Object)
			require.Equal(t, tc.wantRel, resolved.RelPath)
			require.Equal(t, filepath.Join(root, tc.wantRel), resolved.Path)
			require.Equal(t, tc.wantSet, resolved.SettingID)
			require.Equal(t, tc.wantArt, resolved.ArtifactPath)
		})
	}
}

func TestResolveURIRejectsUnsafeAndUnderdefinedForms(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cases := []string{
		" desired://user/leon/targets/git/settings#user.email",
		"state://user/leon/targets/git/settings#user.email",
		"desired:///user/leon/targets/git/settings#user.email",
		"desired://user/leon/targets/git/settings#",
		"desired://user/leon/targets/git/settings#User.Email",
		"desired://user/leon/targets/git/settings/extra#user.email",
		"desired://user/leon/targets/git/manifest#meta",
		"desired://user/leon/targets/git/artifacts/config.json#meta",
		"desired://user/leon/targets/git/artifacts/../escape",
		"desired://user/leon/targets/git/artifacts/config%2ejson",
		"desired://user/leon/targets/git/artifacts/config\\evil",
		"desired://user/leon/targets/git/settings#user.email?x=1",
		"desired://user@host/leon/targets/git/settings#user.email",
		"desired://shared/leon/targets/git/settings#user.email",
		"desired://machine-user/mbp/targets/git/settings#user.email",
		"desired://user/Leon/targets/git/settings#user.email",
		"desired://user/leon/targets/Git/settings#user.email",
		"desired://user/leon/targets/git/unknown#user.email",
	}

	for _, uri := range cases {
		t.Run(uri, func(t *testing.T) {
			t.Parallel()
			_, err := ResolveURI(root, uri)
			require.Error(t, err)
		})
	}
}

func TestResolveForSettingRequiresResolvedSettingMatch(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	setting := resolution.ResolvedSetting{
		TargetID:       "git",
		SettingID:      "user.email",
		DesiredURI:     "desired://user/leon/targets/git/settings#user.email",
		DesiredRelPath: filepath.Join("desired", "user", "leon", "targets", "git", "settings.yaml"),
	}
	resolved, err := ResolveForSetting(root, setting)
	require.NoError(t, err)
	require.Equal(t, "git", resolved.TargetID)
	require.Equal(t, "user.email", resolved.SettingID)

	setting.DesiredURI = "desired://user/leon/targets/git/settings#user.name"
	_, err = ResolveForSetting(root, setting)
	require.Error(t, err)
	require.Contains(t, err.Error(), "mismatch")
}

func TestSelectedValueSettingsReadWriteMissingPresentDeleteAndUnmanaged(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	uri := "desired://user/leon/targets/git/settings#user.email"
	safety := safeDecision(t, recipe.IniFileDriverID)

	read, err := ReadSelectedValue(root, uri)
	require.NoError(t, err)
	require.Equal(t, StatusMissing, read.Status)

	require.NoError(t, WriteSelectedValue(WriteRequest{RepoRoot: root, URI: uri, Value: SetString("leon@example.com"), Safety: safety}))
	settingsPath := filepath.Join(root, "desired", "user", "leon", "targets", "git", "settings.yaml")
	requireFile(t, settingsPath, `schema: dotfiles-manager.v2.desired-settings
schemaVersion: 1
values:
  user.email:
    intent: set
    kind: string
    value: leon@example.com
`)

	read, err = ReadSelectedValue(root, uri)
	require.NoError(t, err)
	require.Equal(t, StatusPresent, read.Status)
	require.Equal(t, IntentSet, read.Intent)
	require.Equal(t, KindString, read.Kind)
	require.NotNil(t, read.Desired)

	encoded, err := json.Marshal(read)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "leon@example.com")

	require.NoError(t, WriteSelectedValue(WriteRequest{RepoRoot: root, URI: "desired://user/leon/targets/git/settings#user.name", Value: SetBool(true), Safety: safeDecisionForSetting(t, recipe.JSONFileDriverID, "user.name")}))
	requireFile(t, settingsPath, `schema: dotfiles-manager.v2.desired-settings
schemaVersion: 1
values:
  user.email:
    intent: set
    kind: string
    value: leon@example.com
  user.name:
    intent: set
    kind: bool
    value: true
`)

	require.NoError(t, WriteSelectedValue(WriteRequest{RepoRoot: root, URI: uri, Value: Delete(), Safety: safety}))
	read, err = ReadSelectedValue(root, uri)
	require.NoError(t, err)
	require.Equal(t, StatusPresent, read.Status)
	require.Equal(t, IntentDelete, read.Intent)
	require.Empty(t, read.Kind)
	require.NotNil(t, read.Desired)

	require.NoError(t, MarkSelectedValueUnmanaged(WriteRequest{RepoRoot: root, URI: uri, Safety: safety}))
	read, err = ReadSelectedValue(root, uri)
	require.NoError(t, err)
	require.Equal(t, StatusUnmanaged, read.Status)
	require.Equal(t, IntentUnmanaged, read.Intent)
	require.Nil(t, read.Desired)
}

func TestSelectedValueSettingsSupportsScalarKinds(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		uri   string
		value SelectedValue
		want  string
	}{
		{name: "number", uri: "desired://user/leon/targets/git/settings#number.value", value: SetNumber(json.Number("42.5")), want: "value: \"42.5\""},
		{name: "null", uri: "desired://user/leon/targets/git/settings#null.value", value: SetNull(), want: "kind: \"null\""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			settingID := strings.TrimPrefix(tc.uri, "desired://user/leon/targets/git/settings#")
			safety := safeDecisionForSetting(t, recipe.YAMLFileDriverID, settingID)
			require.NoError(t, WriteSelectedValue(WriteRequest{RepoRoot: root, URI: tc.uri, Value: tc.value, Safety: safety}))
			read, err := ReadSelectedValue(root, tc.uri)
			require.NoError(t, err)
			require.Equal(t, StatusPresent, read.Status)
			require.NotNil(t, read.Desired)
			body := readFile(t, filepath.Join(root, "desired", "user", "leon", "targets", "git", "settings.yaml"))
			require.Contains(t, body, tc.want)
		})
	}
}

func TestReadSelectedValueDistinguishesMissingEntryAndRejectsInvalidFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	uri := "desired://user/leon/targets/git/settings#user.email"
	path := filepath.Join(root, "desired", "user", "leon", "targets", "git", "settings.yaml")
	writeFile(t, path, "schema: dotfiles-manager.v2.desired-settings\nschemaVersion: 1\nvalues:\n  user.name:\n    intent: set\n    kind: string\n    value: Leon\n")
	read, err := ReadSelectedValue(root, uri)
	require.NoError(t, err)
	require.Equal(t, StatusMissing, read.Status)

	invalids := []struct {
		name string
		body string
	}{
		{name: "schema", body: "schema: dotfiles-manager.v2.other\nschemaVersion: 1\nvalues: {}\n"},
		{name: "version", body: "schema: dotfiles-manager.v2.desired-settings\nschemaVersion: 2\nvalues: {}\n"},
		{name: "unknown field", body: "schema: dotfiles-manager.v2.desired-settings\nschemaVersion: 1\nvalues:\n  user.email:\n    intent: set\n    kind: string\n    value: x\n    extra: true\n"},
		{name: "bad number", body: "schema: dotfiles-manager.v2.desired-settings\nschemaVersion: 1\nvalues:\n  user.email:\n    intent: set\n    kind: number\n    value: \"01\"\n"},
		{name: "bad bool", body: "schema: dotfiles-manager.v2.desired-settings\nschemaVersion: 1\nvalues:\n  user.email:\n    intent: set\n    kind: bool\n    value: \"true\"\n"},
		{name: "delete with value", body: "schema: dotfiles-manager.v2.desired-settings\nschemaVersion: 1\nvalues:\n  user.email:\n    intent: delete\n    value: x\n"},
		{name: "unsupported intent", body: "schema: dotfiles-manager.v2.desired-settings\nschemaVersion: 1\nvalues:\n  user.email:\n    intent: merge\n"},
	}
	for _, tc := range invalids {
		t.Run(tc.name, func(t *testing.T) {
			writeFile(t, path, tc.body)
			_, err := ReadSelectedValue(root, uri)
			require.Error(t, err)
			require.NotContains(t, err.Error(), "secret@example.com")
		})
	}
}

func TestSelectedValueWritesEnforceDriverDesiredKindCompatibility(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	uri := "desired://user/leon/targets/git/settings#user.email"
	path := filepath.Join(root, "desired", "user", "leon", "targets", "git", "settings.yaml")

	err := WriteSelectedValue(WriteRequest{RepoRoot: root, URI: uri, Value: SetBool(true), Safety: safeDecision(t, recipe.IniFileDriverID)})
	requireSafetyCode(t, err, "desired.writeSafety.desiredTypeUnsupported")
	assertMissing(t, path)

	require.NoError(t, WriteSelectedValue(WriteRequest{RepoRoot: root, URI: uri, Value: SetBool(true), Safety: safeDecision(t, recipe.JSONFileDriverID)}))
	read, err := ReadSelectedValue(root, uri)
	require.NoError(t, err)
	require.Equal(t, StatusPresent, read.Status)
	require.Equal(t, KindBool, read.Kind)

	require.NoError(t, WriteSelectedValue(WriteRequest{RepoRoot: root, URI: uri, Value: SetNumber(json.Number("42")), Safety: safeDecision(t, recipe.TOMLFileDriverID)}))
	read, err = ReadSelectedValue(root, uri)
	require.NoError(t, err)
	require.Equal(t, StatusPresent, read.Status)
	require.Equal(t, KindNumber, read.Kind)

	err = WriteSelectedValue(WriteRequest{RepoRoot: root, URI: uri, Value: SetNull(), Safety: safeDecision(t, recipe.TOMLFileDriverID)})
	requireSafetyCode(t, err, "desired.writeSafety.desiredTypeUnsupported")

	require.NoError(t, WriteSelectedValue(WriteRequest{RepoRoot: root, URI: uri, Value: SetBool(false), Safety: safeDecision(t, recipe.PlistFileDriverID)}))
	read, err = ReadSelectedValue(root, uri)
	require.NoError(t, err)
	require.Equal(t, StatusPresent, read.Status)
	require.Equal(t, KindBool, read.Kind)

	require.NoError(t, MarkSelectedValueUnmanaged(WriteRequest{RepoRoot: root, URI: uri, Safety: safeDecision(t, recipe.IniFileDriverID)}))
	read, err = ReadSelectedValue(root, uri)
	require.NoError(t, err)
	require.Equal(t, StatusUnmanaged, read.Status)
}

func TestSelectedValueWritesRequireSafetyDecision(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	uri := "desired://user/leon/targets/git/settings#user.email"
	path := filepath.Join(root, "desired", "user", "leon", "targets", "git", "settings.yaml")

	err := WriteSelectedValue(WriteRequest{RepoRoot: root, URI: uri, Value: SetString("secret@example.com")})
	requireSafetyCode(t, err, "desired.writeSafety.decisionRequired")
	assertMissing(t, path)

	err = MarkSelectedValueUnmanaged(WriteRequest{RepoRoot: root, URI: uri, Safety: &WriteSafetyDecision{Recipe: safeRecipe(t, recipe.IniFileDriverID, "user.email"), SettingRef: "user.email"}})
	requireSafetyCode(t, err, "desired.writeSafety.trust.sourceRequired")
	assertMissing(t, path)

	untrusted := safeDecision(t, recipe.IniFileDriverID)
	untrusted.Context.Trusted = false
	err = WriteSelectedValue(WriteRequest{RepoRoot: root, URI: uri, Value: SetString("secret@example.com"), Safety: untrusted})
	requireSafetyCode(t, err, "desired.writeSafety.trust.untrusted")
	assertMissing(t, path)
}

func TestSelectedValueWritesBlockLikelySecretsBeforeSideEffects(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		value  string
		driver string
	}{
		{name: "private key", value: "-----BEGIN OPENSSH PRIVATE KEY-----\nabc\n-----END OPENSSH PRIVATE KEY-----", driver: recipe.IniFileDriverID},
		{name: "escaped private key", value: "-----BEGIN RSA PRIVATE KEY-----\\nabc\\n-----END RSA PRIVATE KEY-----", driver: recipe.IniFileDriverID},
		{name: "github token", value: "ghp_abcdefghijklmnopqrstuvwxyzABCDEFGH123456", driver: recipe.IniFileDriverID},
		{name: "github fine grained token", value: "github_pat_" + strings.Repeat("A", 90), driver: recipe.IniFileDriverID},
		{name: "gitlab token", value: "glpat-abcdefghijklmnopqrstuvwxyz12", driver: recipe.IniFileDriverID},
		{name: "slack token", value: "xoxb-not-a-real-token-value-abcdef", driver: recipe.IniFileDriverID},
		{name: "openai key", value: "sk-proj-abcdefghijklmnopqrstuvwxyz1234567890", driver: recipe.IniFileDriverID},
		{name: "aws access key id", value: "AKIAIOSFODNN7EXAMPLE", driver: recipe.IniFileDriverID},
		{name: "google api key", value: "AIza" + strings.Repeat("A", 35), driver: recipe.IniFileDriverID},
		{name: "stripe live key", value: "sk_live_" + strings.Repeat("a", 24), driver: recipe.IniFileDriverID},
		{name: "jwt", value: "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.abcdefghijklmnopqrstuvwxyz", driver: recipe.JSONFileDriverID},
		{name: "context entropy", value: "A9bC7dE8fG1hJ2kL3mN4pQ5r", driver: recipe.JSONFileDriverID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			uri := "desired://user/leon/targets/git/settings#api_token"
			path := filepath.Join(root, "desired", "user", "leon", "targets", "git", "settings.yaml")
			err := WriteSelectedValue(WriteRequest{RepoRoot: root, URI: uri, Value: SetString(tc.value), Safety: safeDecisionForSetting(t, tc.driver, "api_token")})
			requireSafetyCode(t, err, "desired.writeSafety.secretDetected")
			assertMissing(t, filepath.Join(root, "desired"))
			assertMissing(t, path)
			require.NotContains(t, err.Error(), tc.value)
			payload, marshalErr := json.Marshal(err)
			require.NoError(t, marshalErr)
			require.NotContains(t, string(payload), tc.value)
		})
	}
}

func TestSelectedValueWritesAllowBenignStringsAndNonStrings(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		setting string
		value   SelectedValue
	}{
		{name: "email", setting: "user.email", value: SetString("leonid@example.com")},
		{name: "name", setting: "user.name", value: SetString("Leonid Komarovsky")},
		{name: "url", setting: "homepage", value: SetString("https://example.com/docs/theme")},
		{name: "theme", setting: "theme", value: SetString("nord-dark")},
		{name: "human label in sensitive context", setting: "token_label", value: SetString("this-is-a-long-but-human-readable-token-label")},
		{name: "bool", setting: "api_token", value: SetBool(true)},
		{name: "number", setting: "api_token", value: SetNumber(json.Number("42"))},
		{name: "null", setting: "api_token", value: SetNull()},
		{name: "delete", setting: "api_token", value: Delete()},
		{name: "unmanaged", setting: "api_token", value: Unmanaged()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			uri := "desired://user/leon/targets/git/settings#" + tc.setting
			err := WriteSelectedValue(WriteRequest{RepoRoot: root, URI: uri, Value: tc.value, Safety: safeDecisionForSetting(t, recipe.JSONFileDriverID, tc.setting)})
			require.NoError(t, err)
		})
	}
}

func TestSelectedValueFormattingDoesNotLeakRawValue(t *testing.T) {
	t.Parallel()

	secret := "ghp_abcdefghijklmnopqrstuvwxyzABCDEFGH123456"
	value := SetString(secret)
	for _, rendered := range []string{
		fmt.Sprintf("%v", value),
		fmt.Sprintf("%+v", value),
		fmt.Sprintf("%#v", value),
		fmt.Sprintf("%q", value),
	} {
		require.NotContains(t, rendered, secret)
		require.Contains(t, rendered, "<redacted>")
	}
}

func TestSelectedValueWritesBlockUnsafeRecipeMetadata(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	uri := "desired://user/leon/targets/git/settings#user.email"
	path := filepath.Join(root, "desired", "user", "leon", "targets", "git", "settings.yaml")

	cases := []struct {
		name string
		mut  func(*recipe.Recipe)
		code string
	}{
		{name: "non selected driver", mut: func(r *recipe.Recipe) {
			res := r.Resources["user-email"]
			res.Driver = recipe.FileDriverID
			res.Selector = nil
			r.Resources["user-email"] = res
		}, code: "desired.writeSafety.driverUnsupported"},
		{name: "setting read only", mut: func(r *recipe.Recipe) {
			s := r.Settings["user.email"]
			s.Capability = "read-only"
			r.Settings["user.email"] = s
		}, code: "desired.writeSafety.setting.capabilityBlocked"},
		{name: "resource read only", mut: func(r *recipe.Recipe) {
			res := r.Resources["user-email"]
			res.Capability = "read-only"
			r.Resources["user-email"] = res
		}, code: "desired.writeSafety.resource.capabilityBlocked"},
		{name: "missing setting sensitivity", mut: func(r *recipe.Recipe) {
			s := r.Settings["user.email"]
			s.Sensitivity = ""
			r.Settings["user.email"] = s
		}, code: "desired.writeSafety.setting.sensitivity.required"},
		{name: "missing setting redaction", mut: func(r *recipe.Recipe) { s := r.Settings["user.email"]; s.Redaction = ""; r.Settings["user.email"] = s }, code: "desired.writeSafety.setting.redaction.required"},
		{name: "missing resource sensitivity", mut: func(r *recipe.Recipe) {
			res := r.Resources["user-email"]
			res.Sensitivity = ""
			r.Resources["user-email"] = res
		}, code: "desired.writeSafety.resource.sensitivity.required"},
		{name: "missing resource redaction", mut: func(r *recipe.Recipe) {
			res := r.Resources["user-email"]
			res.Redaction = ""
			r.Resources["user-email"] = res
		}, code: "desired.writeSafety.resource.redaction.required"},
		{name: "secret", mut: func(r *recipe.Recipe) {
			s := r.Settings["user.email"]
			s.Sensitivity = recipe.SensitivitySecret
			r.Settings["user.email"] = s
		}, code: "desired.writeSafety.sensitivity.secretBlocked"},
		{name: "unknown", mut: func(r *recipe.Recipe) {
			s := r.Settings["user.email"]
			s.Sensitivity = recipe.SensitivityUnknown
			r.Settings["user.email"] = s
		}, code: "desired.writeSafety.sensitivity.unknownBlocked"},
		{name: "blocked save", mut: func(r *recipe.Recipe) {
			s := r.Settings["user.email"]
			s.Redaction = recipe.RedactionBlockedSave
			r.Settings["user.email"] = s
		}, code: "desired.writeSafety.redaction.blockedSave"},
		{name: "redaction unavailable", mut: func(r *recipe.Recipe) {
			s := r.Settings["user.email"]
			s.Redaction = recipe.RedactionUnavailable
			r.Settings["user.email"] = s
		}, code: "desired.writeSafety.redaction.unavailable"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := safeRecipe(t, recipe.IniFileDriverID, "user.email")
			tc.mut(rec)
			err := WriteSelectedValue(WriteRequest{RepoRoot: root, URI: uri, Value: SetString("secret@example.com"), Safety: &WriteSafetyDecision{Recipe: rec, SettingRef: "user.email", Context: trustedLocalContext(t, rec)}})
			requireSafetyCode(t, err, tc.code)
			assertMissing(t, path)
		})
	}
}

func TestSelectedValueWritesAllowExplicitSensitiveAndOpaqueApprovals(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	uri := "desired://user/leon/targets/git/settings#user.email"
	rec := safeRecipe(t, recipe.IniFileDriverID, "user.email")
	setting := rec.Settings["user.email"]
	setting.Sensitivity = recipe.SensitivitySecret
	setting.Redaction = recipe.RedactionUnavailable
	rec.Settings["user.email"] = setting
	res := rec.Resources["user-email"]
	res.Sensitivity = recipe.SensitivityUnknown
	rec.Resources["user-email"] = res
	ctx := trustedLocalContext(t, rec)
	ctx.AllowSensitive = true
	ctx.AllowUnknownSensitivity = true
	ctx.AllowOpaque = true

	err := WriteSelectedValue(WriteRequest{RepoRoot: root, URI: uri, Value: SetString("allowed@example.com"), Safety: &WriteSafetyDecision{Recipe: rec, SettingRef: "git:user.email", Context: ctx}})
	require.NoError(t, err)
}

func TestSelectedValueWritesRejectMismatchAndUnsafePaths(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	err := WriteSelectedValue(WriteRequest{RepoRoot: root, URI: "desired://user/leon/targets/git/settings#user.email", Value: SetString("x"), Safety: safeDecisionForSetting(t, recipe.IniFileDriverID, "user.name")})
	requireSafetyCode(t, err, "desired.writeSafety.settingMismatch")

	targetDir := filepath.Join(root, "desired", "user", "leon", "targets", "git")
	require.NoError(t, os.MkdirAll(filepath.Dir(targetDir), 0o755))
	require.NoError(t, os.Symlink(t.TempDir(), targetDir))
	err = WriteSelectedValue(WriteRequest{RepoRoot: root, URI: "desired://user/leon/targets/git/settings#user.email", Value: SetString("x"), Safety: safeDecision(t, recipe.IniFileDriverID)})
	require.Error(t, err)
	require.Contains(t, err.Error(), "symlink")
}

func TestSelectedValueJSONDoesNotLeakRawValue(t *testing.T) {
	t.Parallel()

	encoded, err := json.Marshal(SetString("secret@example.com"))
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"intent":"set"`)
	require.Contains(t, string(encoded), `"kind":"string"`)
	require.NotContains(t, string(encoded), "secret@example.com")

	_, ok, err := Unmanaged().ToSelectedValueDesired()
	require.NoError(t, err)
	require.False(t, ok)
}

func safeDecision(t *testing.T, driver string) *WriteSafetyDecision {
	t.Helper()
	return safeDecisionForSetting(t, driver, "user.email")
}

func safeDecisionForSetting(t *testing.T, driver string, settingID string) *WriteSafetyDecision {
	t.Helper()
	rec := safeRecipe(t, driver, settingID)
	return &WriteSafetyDecision{Recipe: rec, SettingRef: settingID, Context: trustedLocalContext(t, rec)}
}

func trustedLocalContext(t *testing.T, rec *recipe.Recipe) recipe.WriteSafetyContext {
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

func safeRecipe(t *testing.T, driver string, settingID string) *recipe.Recipe {
	t.Helper()
	resourceID := strings.ReplaceAll(settingID, ".", "-")
	selector := ""
	switch driver {
	case recipe.IniFileDriverID:
		selector = "      section: user\n      key: email\n      missingSection: create\n      missingKey: create\n      duplicatePolicy: reject\n      deleteKey: allow\n"
	case recipe.JSONFileDriverID, recipe.YAMLFileDriverID, recipe.TOMLFileDriverID, recipe.PlistFileDriverID:
		selector = "      path: [user, email]\n      createMissing: create\n      duplicatePolicy: reject\n      deleteKey: allow\n"
	case recipe.FileDriverID:
		selector = ""
	default:
		t.Fatalf("unsupported test driver %s", driver)
	}
	body := `schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: git
displayName: Git
supportLevel: experimental
capability: read-write
locations:
  home:
    default: /unused
settings:
  ` + settingID + `:
    label: User email
    supportLevel: experimental
    capability: read-write
    artifactForm: scalar
    sensitivity: personal
    redaction: redacted-for-display
    scopeDefault: user
    resource: ` + resourceID + `
resources:
  ` + resourceID + `:
    driver: ` + driver + `
    location: home
    path: config
    capability: read-write
    sensitivity: personal
    redaction: redacted-for-display
    lifecycle: allowed
`
	if selector != "" {
		body += "    selector:\n" + selector
	}
	rec, err := recipe.Decode("recipe.yaml", strings.NewReader(body))
	require.NoError(t, err)
	return rec
}

func requireSafetyCode(t *testing.T, err error, code string) {
	t.Helper()
	require.Error(t, err)
	var safety *SafetyError
	require.True(t, errors.As(err, &safety), "expected SafetyError, got %T: %v", err, err)
	for _, diagnostic := range safety.Diagnostics {
		if diagnostic.Code == code {
			return
		}
	}
	require.Failf(t, "missing safety code", "wanted %s in %+v", code, safety.Diagnostics)
}

func requireFile(t *testing.T, path string, want string) {
	t.Helper()
	require.Equal(t, want, readFile(t, path))
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

func writeFile(t *testing.T, path string, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func assertMissing(t *testing.T, path string) {
	t.Helper()
	_, err := os.Stat(path)
	require.True(t, os.IsNotExist(err), "expected %s missing, got %v", path, err)
}

func TestDesiredBranchCoverageForErrorsAndHelpers(t *testing.T) {
	t.Parallel()

	require.Equal(t, "set", SetString("x").Intent())
	require.Equal(t, "string", SetString("x").Kind())
	require.Equal(t, "desired write blocked", (*SafetyError)(nil).Error())
	require.Equal(t, "desired write blocked", (&SafetyError{}).Error())
	require.Contains(t, (&SafetyError{Diagnostics: []Diagnostic{{Code: "c", Message: "m"}}}).Error(), "$[c]: m")
	require.Equal(t, "fallback", defaultString("", "fallback"))
	require.Equal(t, "value", defaultString("value", "fallback"))

	_, ok, err := SetBool(true).ToSelectedValueDesired()
	require.NoError(t, err)
	require.True(t, ok)
	_, ok, err = SetNumber(json.Number("42")).ToSelectedValueDesired()
	require.NoError(t, err)
	require.True(t, ok)
	_, ok, err = SetNull().ToSelectedValueDesired()
	require.NoError(t, err)
	require.True(t, ok)
	_, _, err = SelectedValue{intent: IntentSet, kind: KindString, value: true}.ToSelectedValueDesired()
	require.Error(t, err)
	_, _, err = SelectedValue{intent: IntentSet, kind: KindBool, value: "true"}.ToSelectedValueDesired()
	require.Error(t, err)
	_, _, err = SelectedValue{intent: IntentSet, kind: KindNumber, value: 42}.ToSelectedValueDesired()
	require.Error(t, err)
	_, _, err = SelectedValue{intent: IntentSet, kind: KindNumber, value: json.Number("01")}.ToSelectedValueDesired()
	require.Error(t, err)
	_, _, err = SelectedValue{intent: IntentSet, kind: "object", value: map[string]any{}}.ToSelectedValueDesired()
	require.Error(t, err)
	_, _, err = SelectedValue{}.ToSelectedValueDesired()
	require.Error(t, err)

	_, err = entryFromSelectedValue(SelectedValue{intent: IntentSet, kind: KindString, value: true})
	require.Error(t, err)
	_, err = entryFromSelectedValue(SelectedValue{intent: IntentSet, kind: KindBool, value: "true"})
	require.Error(t, err)
	_, err = entryFromSelectedValue(SelectedValue{intent: IntentSet, kind: KindNumber, value: 42})
	require.Error(t, err)
	_, err = entryFromSelectedValue(SelectedValue{intent: IntentSet, kind: KindNumber, value: json.Number("01")})
	require.Error(t, err)
	_, err = entryFromSelectedValue(SelectedValue{intent: IntentSet, kind: "object"})
	require.Error(t, err)
	_, err = entryFromSelectedValue(SelectedValue{})
	require.Error(t, err)
}

func TestSelectedValueEntryValidationBranches(t *testing.T) {
	t.Parallel()

	_, err := selectedValueFromEntry(selectedValueEntry{Intent: IntentSet, Kind: KindString})
	require.Error(t, err)
	_, err = selectedValueFromEntry(selectedValueEntry{Intent: IntentSet, Kind: KindBool, Value: *scalarNode("!!str", "true")})
	require.Error(t, err)
	_, err = selectedValueFromEntry(selectedValueEntry{Intent: IntentSet, Kind: KindNumber})
	require.Error(t, err)
	_, err = selectedValueFromEntry(selectedValueEntry{Intent: IntentSet, Kind: KindNull, Value: *scalarNode("!!str", "not-null")})
	require.Error(t, err)
	_, err = selectedValueFromEntry(selectedValueEntry{Intent: IntentSet, Kind: "object"})
	require.Error(t, err)
	_, err = selectedValueFromEntry(selectedValueEntry{Intent: IntentDelete, Kind: KindString})
	require.Error(t, err)
	_, err = selectedValueFromEntry(selectedValueEntry{Intent: IntentUnmanaged, Value: *scalarNode("!!str", "x")})
	require.Error(t, err)
	_, err = selectedValueFromEntry(selectedValueEntry{})
	require.Error(t, err)
}

func TestResolveAndPathHelperBranches(t *testing.T) {
	t.Parallel()

	_, err := ResolveURI("", "desired://user/leon/targets/git/settings#user.email")
	require.Error(t, err)
	fileRoot := filepath.Join(t.TempDir(), "file")
	writeFile(t, fileRoot, "x")
	_, err = ResolveURI(fileRoot, "desired://user/leon/targets/git/settings#user.email")
	require.Error(t, err)

	root := t.TempDir()
	_, err = ResolveURI(root, "")
	require.Error(t, err)
	_, err = ResolveURI(root, "desired://user/leon/targets/git/settings#user.email#again")
	require.Error(t, err)
	_, err = ResolveURI(root, "desired://user/leon/targets/git/artifacts")
	require.Error(t, err)
	_, err = ResolveURI(root, "desired://machine-user/mbp/targets/git/settings#user.email")
	require.Error(t, err)
	_, err = ResolveURI(root, "desired://machine/mbp/targets/git/manifest/extra")
	require.Error(t, err)

	setting := resolution.ResolvedSetting{TargetID: "git", SettingID: "user.email", DesiredURI: "desired://user/leon/targets/git/manifest", DesiredRelPath: filepath.Join("desired", "user", "leon", "targets", "git", "manifest.yaml")}
	_, err = ResolveForSetting(root, setting)
	require.Error(t, err)
	setting = resolution.ResolvedSetting{TargetID: "git", SettingID: "user.email", DesiredURI: "desired://user/leon/targets/git/settings#user.email", DesiredRelPath: "wrong"}
	_, err = ResolveForSetting(root, setting)
	require.Error(t, err)

	_, err = ReadSelectedValueForSetting(root, resolution.ResolvedSetting{TargetID: "git", SettingID: "user.email", DesiredURI: "desired://user/leon/targets/git/settings#user.email", DesiredRelPath: filepath.Join("desired", "user", "leon", "targets", "git", "settings.yaml")})
	require.NoError(t, err)

	_, err = ReadSelectedValue(root, "desired://user/leon/targets/git/manifest")
	require.Error(t, err)
	err = WriteSelectedValue(WriteRequest{RepoRoot: root, URI: "desired://user/leon/targets/git/manifest", Value: SetString("x"), Safety: safeDecision(t, recipe.IniFileDriverID)})
	require.Error(t, err)

	outside := filepath.Join(t.TempDir(), "outside")
	require.Error(t, ensurePathInside(root, outside))
	_, err = ensureSafeReadPath(root, filepath.Join(root, "missing", "settings.yaml"))
	require.NoError(t, err)
}

func TestReadWritePathSafetyBranches(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	settings := filepath.Join(root, "desired", "user", "leon", "targets", "git", "settings.yaml")
	writeFile(t, settings, "schema: dotfiles-manager.v2.desired-settings\nschemaVersion: 1\nvalues: {}\n")
	exists, err := ensureSafeReadPath(root, settings)
	require.NoError(t, err)
	require.True(t, exists)
	require.NoError(t, ensureSafeWritePath(root, settings))

	dirAsFile := filepath.Join(root, "desired", "user", "leon", "targets", "git", "dir-settings.yaml")
	require.NoError(t, os.MkdirAll(dirAsFile, 0o755))
	_, err = ensureSafeReadPath(root, dirAsFile)
	require.Error(t, err)
	err = ensureSafeWritePath(root, dirAsFile)
	require.Error(t, err)

	linkFile := filepath.Join(root, "desired", "user", "leon", "targets", "git", "link-settings.yaml")
	require.NoError(t, os.Symlink(settings, linkFile))
	_, err = ensureSafeReadPath(root, linkFile)
	require.Error(t, err)
	err = ensureSafeWritePath(root, linkFile)
	require.Error(t, err)

	linkDirRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(linkDirRoot, "desired", "user", "leon", "targets"), 0o755))
	require.NoError(t, os.Symlink(t.TempDir(), filepath.Join(linkDirRoot, "desired", "user", "leon", "targets", "git")))
	err = ensureNoSymlinkParents(linkDirRoot, filepath.Join(linkDirRoot, "desired", "user", "leon", "targets", "git"))
	require.Error(t, err)

	err = ensureNoSymlinkParents(linkDirRoot, filepath.Join(t.TempDir(), "outside"))
	require.Error(t, err)
}

func TestWriteSafetyAdditionalBranches(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	uri := "desired://user/leon/targets/git/settings#user.email"
	bundledContext := recipe.WriteSafetyContext{Source: recipe.RecipeSourceBundled}
	err := WriteSelectedValue(WriteRequest{RepoRoot: root, URI: uri, Value: SetString("x"), Safety: &WriteSafetyDecision{SettingRef: "user.email", Context: bundledContext}})
	requireSafetyCode(t, err, "desired.writeSafety.recipeRequired")

	badRecipe := &recipe.Recipe{}
	err = WriteSelectedValue(WriteRequest{RepoRoot: root, URI: uri, Value: SetString("x"), Safety: &WriteSafetyDecision{Recipe: badRecipe, SettingRef: "user.email", Context: bundledContext}})
	requireSafetyCode(t, err, "desired.writeSafety.recipeInvalid")

	safeRec := safeRecipe(t, recipe.IniFileDriverID, "user.email")
	safeCtx := trustedLocalContext(t, safeRec)
	err = WriteSelectedValue(WriteRequest{RepoRoot: root, URI: uri, Value: SetString("x"), Safety: &WriteSafetyDecision{Recipe: safeRec, SettingRef: "", Context: safeCtx}})
	requireSafetyCode(t, err, "desired.writeSafety.settingRefInvalid")
	err = WriteSelectedValue(WriteRequest{RepoRoot: root, URI: uri, Value: SetString("x"), Safety: &WriteSafetyDecision{Recipe: safeRec, SettingRef: "other:user.email", Context: safeCtx}})
	requireSafetyCode(t, err, "desired.writeSafety.settingRefInvalid")
	err = WriteSelectedValue(WriteRequest{RepoRoot: root, URI: uri, Value: SetString("x"), Safety: &WriteSafetyDecision{Recipe: safeRec, SettingRef: "git:user:email", Context: safeCtx}})
	requireSafetyCode(t, err, "desired.writeSafety.settingRefInvalid")

	unsupported := safeDecision(t, recipe.IniFileDriverID)
	unsupported.Context.Source = "remote"
	err = WriteSelectedValue(WriteRequest{RepoRoot: root, URI: uri, Value: SetString("x"), Safety: unsupported})
	requireSafetyCode(t, err, "desired.writeSafety.trust.sourceUnsupported")

	rec := safeRecipe(t, recipe.IniFileDriverID, "user.email")
	setting := rec.Settings["user.email"]
	setting.Capability = ""
	rec.Settings["user.email"] = setting
	resource := rec.Resources["user-email"]
	resource.Capability = ""
	rec.Resources["user-email"] = resource
	err = WriteSelectedValue(WriteRequest{RepoRoot: root, URI: uri, Value: SetString("x"), Safety: &WriteSafetyDecision{Recipe: rec, SettingRef: "user.email", Context: trustedLocalContext(t, rec)}})
	require.NoError(t, err)
}
