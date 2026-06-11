package appauthor

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/stretchr/testify/require"
)

func TestCreateFileScaffoldIsDeterministicAndValidateReady(t *testing.T) {
	repoRoot := t.TempDir()

	report, err := RunCreate(CreateOptions{
		RepoRoot:     repoRoot,
		TargetID:     "local-file-demo",
		Template:     TemplateFile,
		FromPath:     "~/.config/demo/config.yaml",
		SettingID:    "config",
		SettingLabel: "Config file",
		ScopeDefault: "machine-user",
		Lifecycle:    recipe.LifecycleAllowed,
	})
	require.NoError(t, err)
	require.Equal(t, "changed", report.Summary.Status)
	require.Equal(t, 3, report.Summary.Written)
	require.FileExists(t, filepath.Join(repoRoot, "recipes/local/local-file-demo/recipe.yaml"))
	require.FileExists(t, filepath.Join(repoRoot, "recipes/local/local-file-demo/README.md"))
	require.FileExists(t, filepath.Join(repoRoot, "recipes/local/local-file-demo/fixtures/README.md"))

	body := readFile(t, filepath.Join(repoRoot, "recipes/local/local-file-demo/recipe.yaml"))
	require.Contains(t, body, "target: local-file-demo")
	require.Contains(t, body, "path: \".config/demo/config.yaml\"")
	require.Contains(t, body, "scopeDefault: machine-user")
	require.Contains(t, body, "lifecycle: allowed")

	rec, loadErr := recipe.LoadLocal(repoRoot, "local-file-demo")
	require.NoError(t, loadErr)
	require.Equal(t, recipe.FileDriverID, rec.Resources["config-file"].Driver)

	validate, validateErr := RunValidate(ValidateOptions{RepoRoot: repoRoot, TargetID: "local-file-demo"})
	require.NoError(t, validateErr)
	require.Equal(t, "ok", validate.Summary.Status)
	require.True(t, validate.AppValidate.Trust.WriteTrustRequired)
	require.Equal(t, "not-checked", validate.AppValidate.Trust.LocalTrustState)
	require.NotEmpty(t, validate.AppValidate.Trust.WriteSurfaceFingerprint)
	require.Len(t, validate.Diagnostics, 1)
	require.Equal(t, "app.validate.trust.not-checked", validate.Diagnostics[0].Code)
}

func TestCreateDryRunWritesNothingAndRepeatFailsClosed(t *testing.T) {
	repoRoot := t.TempDir()
	opts := CreateOptions{
		RepoRoot:     repoRoot,
		TargetID:     "local-dry-run-demo",
		Template:     TemplateFile,
		FromPath:     ".config/demo/config.yaml",
		SettingID:    "config",
		SettingLabel: "Config file",
		ScopeDefault: "user",
		Lifecycle:    recipe.LifecycleAllowed,
		DryRun:       true,
	}

	report, err := RunCreate(opts)
	require.NoError(t, err)
	require.Equal(t, "ok", report.Summary.Status)
	require.Equal(t, "planned", report.AppCreate.Files[0].Action)
	require.NoDirExists(t, filepath.Join(repoRoot, "recipes/local/local-dry-run-demo"))

	opts.DryRun = false
	_, err = RunCreate(opts)
	require.NoError(t, err)
	_, err = RunCreate(opts)
	require.Error(t, err)
	var appErr *Error
	require.True(t, errors.As(err, &appErr))
	require.Equal(t, CodeRecipeExists, appErr.Code)
	require.Equal(t, 2, appErr.ExitCode())
}

func TestCreateSelectedValueDriversAndSelectors(t *testing.T) {
	cases := []struct {
		name       string
		driver     string
		selector   string
		wantInYAML []string
	}{
		{name: "ini", driver: recipe.IniFileDriverID, selector: "user.email", wantInYAML: []string{"section: \"user\"", "key: \"email\"", "missingSection: create", "missingKey: create"}},
		{name: "json", driver: recipe.JSONFileDriverID, selector: "user.email", wantInYAML: []string{"driver: json-file", "path:", "- \"user\"", "- \"email\"", "createMissing: create"}},
		{name: "yaml", driver: recipe.YAMLFileDriverID, selector: "user.email", wantInYAML: []string{"path:", "- \"user\"", "- \"email\"", "createMissing: create"}},
		{name: "toml", driver: recipe.TOMLFileDriverID, selector: "user.email", wantInYAML: []string{"driver: toml-file", "path:", "- \"user\"", "- \"email\"", "createMissing: create"}},
		{name: "plist", driver: recipe.PlistFileDriverID, selector: "RootSetting", wantInYAML: []string{"driver: plist-file", "- \"RootSetting\""}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repoRoot := t.TempDir()
			targetID := "local-" + tc.name + "-demo"
			_, err := RunCreate(CreateOptions{
				RepoRoot:     repoRoot,
				TargetID:     targetID,
				Template:     TemplateSelectedValue,
				FromPath:     ".config/demo/config.yaml",
				SettingID:    "user.email",
				SettingLabel: "User email",
				Driver:       tc.driver,
				Selector:     tc.selector,
				ScopeDefault: "user",
				Lifecycle:    recipe.LifecycleAllowed,
			})
			require.NoError(t, err)
			body := readFile(t, filepath.Join(repoRoot, "recipes/local", targetID, "recipe.yaml"))
			for _, want := range tc.wantInYAML {
				require.Contains(t, body, want)
			}
			_, validateErr := RunValidate(ValidateOptions{RepoRoot: repoRoot, TargetID: targetID})
			require.NoError(t, validateErr)
		})
	}
}

func TestCreateRejectsUnsafeInputsAndBundledCollisions(t *testing.T) {
	repoRoot := t.TempDir()
	base := CreateOptions{
		RepoRoot:     repoRoot,
		TargetID:     "local-invalid-demo",
		Template:     TemplateSelectedValue,
		FromPath:     ".config/demo/config.yaml",
		SettingID:    "user.email",
		SettingLabel: "User email",
		Driver:       recipe.YAMLFileDriverID,
		Selector:     "user.email",
		ScopeDefault: "user",
		Lifecycle:    recipe.LifecycleAllowed,
	}

	cases := []struct {
		name string
		mut  func(*CreateOptions)
		code string
	}{
		{name: "bundled canonical collision", mut: func(o *CreateOptions) { o.TargetID = recipe.GitTarget }, code: CodeTargetCollision},
		{name: "bundled alias collision", mut: func(o *CreateOptions) { o.TargetID = "gitconfig" }, code: CodeTargetCollision},
		{name: "invalid target id", mut: func(o *CreateOptions) { o.TargetID = "BadTarget" }, code: CodeTargetInvalid},
		{name: "global scope rejected", mut: func(o *CreateOptions) { o.ScopeDefault = "global" }, code: CodeFlagInvalid},
		{name: "unsafe path traversal", mut: func(o *CreateOptions) { o.FromPath = "../escape.yaml" }, code: CodePathInvalid},
		{name: "absolute outside home", mut: func(o *CreateOptions) { o.FromPath = "/etc/hosts" }, code: CodePathInvalid},
		{name: "expression selector", mut: func(o *CreateOptions) { o.Selector = "user.*" }, code: CodeFlagInvalid},
		{name: "file driver rejected", mut: func(o *CreateOptions) { o.Driver = recipe.FileDriverID }, code: CodeFlagInvalid},
		{name: "ini multi-dot selector rejected", mut: func(o *CreateOptions) { o.Driver = recipe.IniFileDriverID; o.Selector = "a.b.c" }, code: CodeFlagInvalid},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := base
			tc.mut(&opts)
			_, err := RunCreate(opts)
			require.Error(t, err)
			var appErr *Error
			require.True(t, errors.As(err, &appErr))
			require.Equal(t, tc.code, appErr.Code)
		})
	}
}

func TestCreateRejectsTemplateSpecificInvalidInputs(t *testing.T) {
	repoRoot := t.TempDir()
	base := CreateOptions{
		RepoRoot:     repoRoot,
		TargetID:     "local-invalid-template",
		Template:     TemplateFile,
		FromPath:     ".config/demo/config.yaml",
		SettingID:    "config",
		SettingLabel: "Config file",
		ScopeDefault: "user",
		Lifecycle:    recipe.LifecycleAllowed,
	}

	cases := []struct {
		name string
		mut  func(*CreateOptions)
		code string
	}{
		{name: "missing template", mut: func(o *CreateOptions) { o.Template = "" }, code: CodeTemplateRequired},
		{name: "invalid template", mut: func(o *CreateOptions) { o.Template = "wizard" }, code: CodeTemplateInvalid},
		{name: "file template rejects selected driver", mut: func(o *CreateOptions) { o.Driver = recipe.JSONFileDriverID }, code: CodeFlagInvalid},
		{name: "file template rejects selector", mut: func(o *CreateOptions) { o.Selector = "user.email" }, code: CodeFlagInvalid},
		{name: "native template rejects from path", mut: func(o *CreateOptions) { o.Template = TemplateNativeExport; o.FromPath = ".config/demo/config.yaml" }, code: CodeFlagInvalid},
		{name: "missing setting", mut: func(o *CreateOptions) { o.SettingID = "" }, code: CodeFlagInvalid},
		{name: "invalid setting", mut: func(o *CreateOptions) { o.SettingID = "BadSetting" }, code: CodeFlagInvalid},
		{name: "missing label", mut: func(o *CreateOptions) { o.SettingLabel = "" }, code: CodeFlagInvalid},
		{name: "missing scope", mut: func(o *CreateOptions) { o.ScopeDefault = "" }, code: CodeFlagInvalid},
		{name: "missing lifecycle", mut: func(o *CreateOptions) { o.Lifecycle = "" }, code: CodeFlagInvalid},
		{name: "unsupported lifecycle", mut: func(o *CreateOptions) { o.Lifecycle = "forever" }, code: CodeFlagInvalid},
		{name: "home path rejected", mut: func(o *CreateOptions) { o.FromPath = "~" }, code: CodePathInvalid},
		{name: "backslash path rejected", mut: func(o *CreateOptions) { o.FromPath = ".config\\demo.yaml" }, code: CodePathInvalid},
		{name: "nul path rejected", mut: func(o *CreateOptions) { o.FromPath = ".config/demo\x00.yaml" }, code: CodePathInvalid},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := base
			tc.mut(&opts)
			report, err := RunCreate(opts)
			require.Error(t, err)
			var appErr *Error
			require.True(t, errors.As(err, &appErr))
			require.Equal(t, tc.code, appErr.Code)
			require.Equal(t, tc.code, report.Error.Code)
			require.Contains(t, diagnosticCodes(report.Diagnostics), tc.code)
		})
	}
}

func TestCreateNormalizesAbsoluteHomePathsAndResourceIDs(t *testing.T) {
	repoRoot := t.TempDir()
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	report, err := RunCreate(CreateOptions{
		RepoRoot:     repoRoot,
		TargetID:     "local-resource-fallback",
		Template:     TemplateFile,
		FromPath:     filepath.Join(home, ".config/demo/config.yaml"),
		SettingID:    "config",
		SettingLabel: "Config file",
		ScopeDefault: "machine",
		Lifecycle:    recipe.LifecycleWarn,
	})
	require.NoError(t, err)
	require.Equal(t, "changed", report.Summary.Status)
	body := readFile(t, filepath.Join(repoRoot, "recipes/local/local-resource-fallback/recipe.yaml"))
	require.Contains(t, body, "path: \".config/demo/config.yaml\"")
	require.Contains(t, body, "config-file:")
	require.NotContains(t, body, home)
	require.Equal(t, "resource-file", resourceID("___", "file"))
}

func TestCreateRejectsInvalidReposAndUnsafeWriteTargets(t *testing.T) {
	repoRoot := t.TempDir()
	base := CreateOptions{
		RepoRoot:     repoRoot,
		TargetID:     "local-write-safety",
		Template:     TemplateFile,
		FromPath:     ".config/demo/config.yaml",
		SettingID:    "config",
		SettingLabel: "Config file",
		ScopeDefault: "user",
		Lifecycle:    recipe.LifecycleAllowed,
	}

	fileRepo := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(fileRepo, []byte("file"), 0o644))
	opts := base
	opts.RepoRoot = fileRepo
	report, err := RunCreate(opts)
	require.Error(t, err)
	require.Equal(t, CodeRepoInvalid, report.Error.Code)

	opts = base
	opts.RepoRoot = filepath.Join(t.TempDir(), "missing")
	report, err = RunCreate(opts)
	require.Error(t, err)
	require.Equal(t, CodeRepoInvalid, report.Error.Code)

	fileParentRepo := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(fileParentRepo, "recipes"), []byte("file"), 0o644))
	opts = base
	opts.RepoRoot = fileParentRepo
	report, err = RunCreate(opts)
	require.Error(t, err)
	require.Equal(t, CodePathUnsafe, report.Error.Code)

	linkParentRepo := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(linkParentRepo, "recipes"), 0o755))
	require.NoError(t, os.Symlink(t.TempDir(), filepath.Join(linkParentRepo, "recipes/local")))
	opts = base
	opts.RepoRoot = linkParentRepo
	report, err = RunCreate(opts)
	require.Error(t, err)
	require.Equal(t, CodePathUnsafe, report.Error.Code)
}

func TestValidateCatchesAuthoringMetadataWithoutReadingTrustRecords(t *testing.T) {
	repoRoot := t.TempDir()
	writeFile(t, filepath.Join(repoRoot, "recipes/local/local-bad-demo/recipe.yaml"), `schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: local-bad-demo
displayName: Local Bad Demo
supportLevel: experimental
capability: read-write
locations:
  home:
    default: "~"
settings:
  config:
    scopeDefault: user
    resource: config-file
resources:
  config-file:
    driver: file
    location: home
    path: .config/demo/config.yaml
    capability: read-write
`)
	writeFile(t, filepath.Join(repoRoot, ".dotfiles-manager/state/trust/corrupt.yaml"), "not: valid: yaml:\n")

	report, err := RunValidate(ValidateOptions{RepoRoot: repoRoot, TargetID: "local-bad-demo"})
	require.Error(t, err)
	codes := diagnosticCodes(report.Diagnostics)
	require.Contains(t, codes, "writeSafety.setting.sensitivity.required")
	require.Contains(t, codes, "writeSafety.setting.redaction.required")
	require.Contains(t, codes, "writeSafety.resource.sensitivity.required")
	require.Contains(t, codes, "writeSafety.resource.redaction.required")
	require.Contains(t, codes, "writeSafety.resource.lifecycle.required")
	require.NotContains(t, strings.Join(codes, ","), "trust")
}

func TestValidateRejectsInvalidTargetsReposAndMissingRecipes(t *testing.T) {
	repoRoot := t.TempDir()

	report, err := RunValidate(ValidateOptions{RepoRoot: filepath.Join(repoRoot, "missing"), TargetID: "local-missing"})
	require.Error(t, err)
	require.Equal(t, CodeRepoInvalid, report.Error.Code)
	assertBlockedValidateJSON(t, report, CodeRepoInvalid, CodeRepoInvalid)

	report, err = RunValidate(ValidateOptions{RepoRoot: repoRoot, TargetID: "BadTarget"})
	require.Error(t, err)
	require.Equal(t, CodeTargetInvalid, report.Error.Code)
	assertBlockedValidateJSON(t, report, CodeTargetInvalid, CodeTargetInvalid)

	report, err = RunValidate(ValidateOptions{RepoRoot: repoRoot, TargetID: "local-missing"})
	require.Error(t, err)
	require.Equal(t, CodeRecipeMissing, report.Error.Code)
	assertBlockedValidateJSON(t, report, CodeRecipeMissing, CodeRecipeMissing)
}

func TestValidateRejectsTargetMismatchAndBundledCollisions(t *testing.T) {
	repoRoot := t.TempDir()
	writeFile(t, filepath.Join(repoRoot, "recipes/local/local-mismatch/recipe.yaml"), strings.Replace(validAppAuthorFileRecipe("other-target"), "target: other-target", "target: other-target", 1))

	report, err := RunValidate(ValidateOptions{RepoRoot: repoRoot, TargetID: "local-mismatch"})
	require.Error(t, err)
	require.Equal(t, CodeTargetMismatch, report.Error.Code)
	require.Contains(t, diagnosticCodes(report.Diagnostics), CodeTargetMismatch)
	require.Empty(t, report.AppValidate.Trust.WriteSurfaceFingerprint)
	require.Nil(t, report.AppValidate.Trust.WriteSurface)
	assertBlockedValidateJSON(t, report, CodeTargetMismatch, CodeTargetMismatch)

	writeFile(t, filepath.Join(repoRoot, "recipes/local/git/recipe.yaml"), validAppAuthorFileRecipe("git"))
	report, err = RunValidate(ValidateOptions{RepoRoot: repoRoot, TargetID: "git"})
	require.Error(t, err)
	require.Equal(t, CodeTargetCollision, report.Error.Code)
	assertBlockedValidateJSON(t, report, CodeTargetCollision, CodeTargetCollision)

	writeFile(t, filepath.Join(repoRoot, "recipes/local/gitconfig/recipe.yaml"), validAppAuthorFileRecipe("gitconfig"))
	report, err = RunValidate(ValidateOptions{RepoRoot: repoRoot, TargetID: "gitconfig"})
	require.Error(t, err)
	require.Equal(t, CodeTargetCollision, report.Error.Code)
	assertBlockedValidateJSON(t, report, CodeTargetCollision, CodeTargetCollision)
}

func TestValidateRejectsSymlinkedRecipeMetadataPaths(t *testing.T) {
	repoRoot := t.TempDir()

	// Symlinked top-level recipes parent; even a valid outside recipe must not be read.
	topRepo := t.TempDir()
	outsideRecipes := t.TempDir()
	writeFile(t, filepath.Join(outsideRecipes, "local/local-link-top/recipe.yaml"), validAppAuthorFileRecipe("local-link-top"))
	require.NoError(t, os.Symlink(outsideRecipes, filepath.Join(topRepo, "recipes")))
	report, err := RunValidate(ValidateOptions{RepoRoot: topRepo, TargetID: "local-link-top"})
	require.Error(t, err)
	require.Equal(t, CodePathUnsafe, report.Error.Code)
	require.Empty(t, report.AppValidate.Target.DisplayName)
	assertBlockedValidateJSON(t, report, CodePathUnsafe, CodePathUnsafe)

	// Symlinked recipes/local parent.
	parentRepo := t.TempDir()
	outsideParent := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(parentRepo, "recipes"), 0o755))
	require.NoError(t, os.Symlink(outsideParent, filepath.Join(parentRepo, "recipes/local")))
	report, err = RunValidate(ValidateOptions{RepoRoot: parentRepo, TargetID: "local-link-parent"})
	require.Error(t, err)
	require.Equal(t, CodePathUnsafe, report.Error.Code)
	assertBlockedValidateJSON(t, report, CodePathUnsafe, CodePathUnsafe)

	// Symlinked target directory.
	outsideDir := t.TempDir()
	writeFile(t, filepath.Join(outsideDir, "recipe.yaml"), validAppAuthorFileRecipe("local-link-dir"))
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "recipes/local"), 0o755))
	require.NoError(t, os.Symlink(outsideDir, filepath.Join(repoRoot, "recipes/local/local-link-dir")))
	report, err = RunValidate(ValidateOptions{RepoRoot: repoRoot, TargetID: "local-link-dir"})
	require.Error(t, err)
	require.Equal(t, CodePathUnsafe, report.Error.Code)
	assertBlockedValidateJSON(t, report, CodePathUnsafe, CodePathUnsafe)

	// Symlinked recipe file.
	writeFile(t, filepath.Join(repoRoot, "recipes/local/local-link-file-real/recipe.yaml"), validAppAuthorFileRecipe("local-link-file"))
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "recipes/local/local-link-file"), 0o755))
	require.NoError(t, os.Symlink(filepath.Join(repoRoot, "recipes/local/local-link-file-real/recipe.yaml"), filepath.Join(repoRoot, "recipes/local/local-link-file/recipe.yaml")))
	report, err = RunValidate(ValidateOptions{RepoRoot: repoRoot, TargetID: "local-link-file"})
	require.Error(t, err)
	require.Equal(t, CodePathUnsafe, report.Error.Code)
	assertBlockedValidateJSON(t, report, CodePathUnsafe, CodePathUnsafe)
}

func TestNativeExportTemplateIsDraftAndValidateReportsNativeMetadataErrors(t *testing.T) {
	repoRoot := t.TempDir()
	createReport, err := RunCreate(CreateOptions{
		RepoRoot:     repoRoot,
		TargetID:     "local-native-demo",
		Template:     TemplateNativeExport,
		SettingID:    "settings",
		SettingLabel: "Settings export",
		ScopeDefault: "machine-user",
		Lifecycle:    recipe.LifecycleBlocked,
	})
	require.NoError(t, err)
	require.Len(t, createReport.AppCreate.NextActions, 2)
	require.Contains(t, createReport.AppCreate.NextActions[0], "replace native operation placeholder metadata")
	require.Equal(t, "dotfiles-manager app validate local-native-demo", createReport.AppCreate.NextActions[1])
	require.NotContains(t, strings.ToLower(strings.Join(createReport.AppCreate.NextActions, "\n")), "should pass")
	require.NotContains(t, strings.ToLower(strings.Join(createReport.AppCreate.NextActions, "\n")), "ready to run")
	body := readFile(t, filepath.Join(repoRoot, "recipes/local/local-native-demo/recipe.yaml"))
	require.Contains(t, body, "reviewed: false")
	require.Contains(t, body, "REPLACE_WITH_ABSOLUTE_REVIEWED_EXECUTABLE")

	report, validateErr := RunValidate(ValidateOptions{RepoRoot: repoRoot, TargetID: "local-native-demo"})
	require.Error(t, validateErr)
	require.Contains(t, diagnosticCodes(report.Diagnostics), "nativeOperation.reviewed.required")
	require.Empty(t, report.AppValidate.Trust.WriteSurfaceFingerprint)
	require.Nil(t, report.AppValidate.Trust.WriteSurface)
	assertBlockedValidateJSON(t, report, CodeRecipeInvalid, "nativeOperation.reviewed.required")
}

func TestJSONOutputUsesStableSchema(t *testing.T) {
	repoRoot := t.TempDir()
	report, err := RunCreate(CreateOptions{
		RepoRoot:     repoRoot,
		TargetID:     "local-json-demo",
		Template:     TemplateFile,
		FromPath:     ".config/demo/config.yaml",
		SettingID:    "config",
		SettingLabel: "Config file",
		ScopeDefault: "user",
		Lifecycle:    recipe.LifecycleAllowed,
	})
	require.NoError(t, err)
	payload, err := JSONCreate(report)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal([]byte(payload), &decoded))
	require.Equal(t, CreateSchema, decoded["schema"])
	require.Equal(t, CreateCommand, decoded["command"])
	require.NotContains(t, payload, repoRoot)
}

func TestTextAndNilJSONReportsAreStable(t *testing.T) {
	repoRoot := t.TempDir()
	createReport, err := RunCreate(CreateOptions{
		RepoRoot:     repoRoot,
		TargetID:     "local-text-demo",
		Template:     TemplateFile,
		FromPath:     ".config/demo/config.yaml",
		SettingID:    "config",
		SettingLabel: "Config file",
		ScopeDefault: "user",
		Lifecycle:    recipe.LifecycleAllowed,
	})
	require.NoError(t, err)

	createText := TextCreate(createReport)
	require.Contains(t, createText, "app create")
	require.Contains(t, createText, "target: local-text-demo (recipe://local/local-text-demo)")
	require.Contains(t, createText, "dotfiles-manager app validate local-text-demo")
	require.Contains(t, createText, "safety: no live app data was read")
	require.Contains(t, createText, "summary status=changed")
	require.NotContains(t, createText, repoRoot)

	validateReport, err := RunValidate(ValidateOptions{RepoRoot: repoRoot, TargetID: "local-text-demo"})
	require.NoError(t, err)
	validateText := TextValidate(validateReport)
	require.Contains(t, validateText, "app validate")
	require.Contains(t, validateText, "trust: local=not-checked writeRequired=true fingerprint=")
	require.Contains(t, validateText, "warning[app.validate.trust.not-checked]")
	require.Contains(t, validateText, "summary status=ok checked=1 warnings=1 blocked=0 failed=0")
	require.NotContains(t, validateText, "fingerprint=-")
	require.NotContains(t, validateText, repoRoot)

	missingReport, missingErr := RunValidate(ValidateOptions{RepoRoot: repoRoot, TargetID: "local-text-missing"})
	require.Error(t, missingErr)
	missingText := TextValidate(missingReport)
	require.Contains(t, missingText, "error[app.recipe.missing]")
	require.Contains(t, missingText, "fingerprint=-")
	require.Contains(t, missingText, "summary status=blocked")

	createJSON, err := JSONCreate(nil)
	require.NoError(t, err)
	require.Contains(t, createJSON, `"schema": "`+CreateSchema+`"`)
	require.Contains(t, createJSON, `"status": "error"`)

	validateJSON, err := JSONValidate(nil)
	require.NoError(t, err)
	require.Contains(t, validateJSON, `"schema": "`+ValidateSchema+`"`)
	require.Contains(t, validateJSON, `"status": "error"`)

	require.Equal(t, "app create\nsummary status=error planned=0 written=0 blocked=0 failed=1", TextCreate(nil))
	require.Equal(t, "app validate\nsummary status=error checked=0 warnings=0 blocked=0 failed=1", TextValidate(nil))
}

func TestInternalErrorAndPathHelpersCoverFailClosedEdges(t *testing.T) {
	var nilErr *Error
	require.Empty(t, nilErr.Error())
	require.Equal(t, 1, nilErr.ExitCode())
	require.Equal(t, "boom", (&Error{Message: "boom"}).Error())
	require.Equal(t, 1, (&Error{Message: "boom"}).ExitCode())

	report := &ValidateReport{}
	finishValidate(report)
	require.Equal(t, "ok", report.Summary.Status)

	_, err := loadLocalRecipe(t.TempDir(), "local-missing")
	require.Error(t, err)
	require.True(t, errors.Is(err, os.ErrNotExist))

	repoRoot := t.TempDir()
	writeFile(t, filepath.Join(repoRoot, "existing.txt"), "old")
	err = writeCreateFiles(repoRoot, map[string]string{"existing.txt": "new"})
	require.Error(t, err)
	require.Equal(t, CodeRecipeExists, errorCode(err))

	err = validateRepoWriteTarget(repoRoot, "../escape")
	require.Error(t, err)
	require.Equal(t, CodePathUnsafe, errorCode(err))

	err = validateRepoWriteTarget(repoRoot, "recipes\\local")
	require.Error(t, err)
	require.Equal(t, CodePathUnsafe, errorCode(err))

	_, err = parseSelectedValueSelector(recipe.JSONFileDriverID, "")
	require.Error(t, err)
	require.Equal(t, CodeFlagInvalid, errorCode(err))

	_, err = parseSelectedValueSelector("unsupported-driver", "user.email")
	require.Error(t, err)
	require.Equal(t, CodeFlagInvalid, errorCode(err))

	_, err = parseSelectedValueSelector(recipe.YAMLFileDriverID, "user..email")
	require.Error(t, err)
	require.Equal(t, CodeFlagInvalid, errorCode(err))

	_, err = parseSelectedValueSelector(recipe.YAMLFileDriverID, "user email")
	require.Error(t, err)
	require.Equal(t, CodeFlagInvalid, errorCode(err))

	typed := typedError("custom.code", "custom", 7, map[string]any{"ok": true})
	require.Equal(t, "custom.code", errorCode(typed))
	require.Equal(t, 7, errorExit(typed, 2))
	require.Equal(t, map[string]any{"ok": true}, errorDetails(typed))
	require.Equal(t, CodeWriteFailed, errorCode(errors.New("plain")))
	require.Equal(t, 3, errorExit(errors.New("plain"), 3))
	require.Nil(t, errorDetails(errors.New("plain")))
}

func validAppAuthorFileRecipe(targetID string) string {
	return `schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: ` + targetID + `
displayName: Local Test
supportLevel: experimental
capability: read-write
locations:
  home:
    default: "~"
settings:
  config:
    label: Config
    supportLevel: experimental
    capability: read-write
    artifactForm: file
    sensitivity: personal
    redaction: redacted-for-display
    lifecycle: allowed
    scopeDefault: user
    resource: config-file
resources:
  config-file:
    driver: file
    location: home
    path: .config/demo/config.yaml
    capability: read-write
    sensitivity: personal
    redaction: redacted-for-display
    lifecycle: allowed
`
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return strings.ReplaceAll(string(data), "\r\n", "\n")
}

func writeFile(t *testing.T, path string, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func diagnosticCodes(diagnostics []Diagnostic) []string {
	codes := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		codes = append(codes, diagnostic.Code)
	}
	return codes
}

func assertBlockedValidateJSON(t *testing.T, report *ValidateReport, wantErrorCode string, wantDiagnosticCode string) {
	t.Helper()
	payload, err := JSONValidate(report)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal([]byte(payload), &decoded))
	require.Equal(t, ValidateSchema, decoded["schema"])
	require.Equal(t, ValidateCommand, decoded["command"])
	require.Equal(t, "blocked", decoded["summary"].(map[string]any)["status"])
	require.Equal(t, wantErrorCode, decoded["error"].(map[string]any)["code"])

	diagnostics := decoded["diagnostics"].([]any)
	require.NotEmpty(t, diagnostics)
	var found bool
	for _, raw := range diagnostics {
		diagnostic := raw.(map[string]any)
		require.NotEmpty(t, diagnostic["code"])
		require.NotEmpty(t, diagnostic["severity"])
		if diagnostic["code"] == wantDiagnosticCode {
			found = true
			require.Equal(t, SeverityError, diagnostic["severity"])
		}
	}
	require.True(t, found, "diagnostic %q not found in %s", wantDiagnosticCode, payload)

	trust := decoded["appValidate"].(map[string]any)["trust"].(map[string]any)
	require.Equal(t, "not-checked", trust["localTrustState"])
	require.NotContains(t, trust, "writeSurfaceFingerprint")
	require.NotContains(t, trust, "writeSurface")
}
