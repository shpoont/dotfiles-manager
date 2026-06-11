package appauthor

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/shpoont/dotfiles-manager/internal/v2/selectedvalue"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"howett.net/plist"
)

func TestRoundtripFileFixtureRunsSaveAndApplyInTempRoots(t *testing.T) {
	repoRoot := t.TempDir()
	targetID := "local-file-roundtrip"

	_, err := RunCreate(CreateOptions{
		RepoRoot:     repoRoot,
		TargetID:     targetID,
		Template:     TemplateFile,
		FromPath:     ".config/demo/config.yaml",
		SettingID:    "config",
		SettingLabel: "Config file",
		ScopeDefault: "user",
		Lifecycle:    recipe.LifecycleAllowed,
	})
	require.NoError(t, err)

	fixtureRoot := roundtripFixtureRoot(repoRoot, targetID, "basic")
	writeRoundtripManifest(t, fixtureRoot, targetID, "basic", []string{"config"})
	writeFile(t, filepath.Join(fixtureRoot, "input/live/locations/home/.config/demo/config.yaml"), "fixture-source-file-value\n")
	writeFile(t, filepath.Join(fixtureRoot, "input/desired/user/fixture-user/targets/"+targetID+"/artifacts/config"), "fixture-desired-file-value\n")
	writeFile(t, filepath.Join(fixtureRoot, "expected/desired/user/fixture-user/targets/"+targetID+"/artifacts/config"), "fixture-source-file-value\n")
	writeFile(t, filepath.Join(fixtureRoot, "expected/live/locations/home/.config/demo/config.yaml"), "fixture-desired-file-value\n")

	report, err := RunTestRoundtrip(TestRoundtripOptions{RepoRoot: repoRoot, TargetID: targetID, Roundtrip: true})
	require.NoError(t, err)
	require.Equal(t, "ok", report.Summary.Status)
	require.Equal(t, 2, report.Summary.Cases)
	require.Equal(t, 2, report.Summary.Passed)
	require.Len(t, report.AppTestRoundtrip.Fixtures, 1)
	require.Equal(t, "passed", report.AppTestRoundtrip.Fixtures[0].Status)
	require.Equal(t, []RoundtripCase{{Setting: "config", Resource: "config-file", Driver: recipe.FileDriverID, Save: "passed", Apply: "passed"}}, report.AppTestRoundtrip.Fixtures[0].Cases)

	payload, err := JSONTestRoundtrip(report)
	require.NoError(t, err)
	text := TextTestRoundtrip(report)
	for _, output := range []string{payload, text} {
		require.NotContains(t, output, repoRoot)
		require.NotContains(t, output, "fixture-source-file-value")
		require.NotContains(t, output, "fixture-desired-file-value")
	}

	// The command must operate on scratch copies only; committed fixture inputs stay unchanged.
	require.Equal(t, "fixture-source-file-value\n", readFile(t, filepath.Join(fixtureRoot, "input/live/locations/home/.config/demo/config.yaml")))
	require.Equal(t, "fixture-desired-file-value\n", readFile(t, filepath.Join(fixtureRoot, "input/desired/user/fixture-user/targets/"+targetID+"/artifacts/config")))
}

func TestRoundtripDoesNotTouchRealDesiredTrustBackupsOrLedgers(t *testing.T) {
	repoRoot := t.TempDir()
	targetID := "local-isolation-roundtrip"
	_, err := RunCreate(CreateOptions{
		RepoRoot:     repoRoot,
		TargetID:     targetID,
		Template:     TemplateFile,
		FromPath:     ".config/demo/config.yaml",
		SettingID:    "config",
		SettingLabel: "Config file",
		ScopeDefault: "user",
		Lifecycle:    recipe.LifecycleAllowed,
	})
	require.NoError(t, err)

	fixtureRoot := roundtripFixtureRoot(repoRoot, targetID, "basic")
	writeRoundtripManifest(t, fixtureRoot, targetID, "basic", []string{"config"})
	writeFile(t, filepath.Join(fixtureRoot, "input/live/locations/home/.config/demo/config.yaml"), "fixture-live\n")
	writeFile(t, filepath.Join(fixtureRoot, "input/desired/user/fixture-user/targets/"+targetID+"/artifacts/config"), "fixture-desired\n")
	writeFile(t, filepath.Join(fixtureRoot, "expected/desired/user/fixture-user/targets/"+targetID+"/artifacts/config"), "fixture-live\n")
	writeFile(t, filepath.Join(fixtureRoot, "expected/live/locations/home/.config/demo/config.yaml"), "fixture-desired\n")

	writeFile(t, filepath.Join(repoRoot, "desired/user/fixture-user/targets/"+targetID+"/artifacts/config"), "real-desired-sentinel\n")
	writeFile(t, filepath.Join(repoRoot, ".dotfiles-manager/state/trust/trust-record.yaml"), "real-trust-sentinel\n")
	writeFile(t, filepath.Join(repoRoot, ".dotfiles-manager/state/backups/keep.txt"), "real-backup-sentinel\n")
	writeFile(t, filepath.Join(repoRoot, ".dotfiles-manager/state/ledgers/keep.txt"), "real-ledger-sentinel\n")
	stateBefore := treeSnapshot(t, filepath.Join(repoRoot, ".dotfiles-manager"))

	report, err := RunTestRoundtrip(TestRoundtripOptions{RepoRoot: repoRoot, TargetID: targetID, Roundtrip: true, Fixture: "basic"})
	require.NoError(t, err, TextTestRoundtrip(report))
	require.Equal(t, "real-desired-sentinel\n", readFile(t, filepath.Join(repoRoot, "desired/user/fixture-user/targets/"+targetID+"/artifacts/config")))
	require.Equal(t, stateBefore, treeSnapshot(t, filepath.Join(repoRoot, ".dotfiles-manager")))
}

func TestRoundtripSelectedValueFixturesSupportStructuredDrivers(t *testing.T) {
	cases := []struct {
		name         string
		driver       string
		fromPath     string
		inputLive    string
		expectedLive string
	}{
		{
			name:         "ini",
			driver:       recipe.IniFileDriverID,
			fromPath:     ".config/demo/config.ini",
			inputLive:    "[user]\n\temail = live@example.test\n",
			expectedLive: "[user]\n\temail = desired@example.test\n",
		},
		{
			name:      "json",
			driver:    recipe.JSONFileDriverID,
			fromPath:  ".config/demo/config.json",
			inputLive: `{"user":{"email":"live@example.test"}}`,
			expectedLive: `{
  "user": {
    "email": "desired@example.test"
  }
}
`,
		},
		{
			name:         "yaml",
			driver:       recipe.YAMLFileDriverID,
			fromPath:     ".config/demo/config.yaml",
			inputLive:    "user:\n  email: live@example.test\n",
			expectedLive: "user:\n  email: desired@example.test\n",
		},
		{
			name:         "toml",
			driver:       recipe.TOMLFileDriverID,
			fromPath:     ".config/demo/config.toml",
			inputLive:    "[user]\nemail = 'live@example.test'\n",
			expectedLive: "[user]\nemail = 'desired@example.test'\n",
		},
		{
			name:         "plist",
			driver:       recipe.PlistFileDriverID,
			fromPath:     ".config/demo/config.plist",
			inputLive:    xmlPlist(t, map[string]any{"user": map[string]any{"email": "live@example.test"}}),
			expectedLive: xmlPlist(t, map[string]any{"user": map[string]any{"email": "desired@example.test"}}),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repoRoot := t.TempDir()
			targetID := "local-" + tc.name + "-roundtrip"

			_, err := RunCreate(CreateOptions{
				RepoRoot:     repoRoot,
				TargetID:     targetID,
				Template:     TemplateSelectedValue,
				FromPath:     tc.fromPath,
				SettingID:    "user.email",
				SettingLabel: "User email",
				Driver:       tc.driver,
				Selector:     "user.email",
				ScopeDefault: "user",
				Lifecycle:    recipe.LifecycleAllowed,
			})
			require.NoError(t, err)

			fixtureRoot := roundtripFixtureRoot(repoRoot, targetID, "basic")
			writeRoundtripManifest(t, fixtureRoot, targetID, "basic", []string{"user.email"})
			writeFile(t, filepath.Join(fixtureRoot, "input/live/locations/home", tc.fromPath), tc.inputLive)
			writeFile(t, filepath.Join(fixtureRoot, "input/desired/user/fixture-user/targets/"+targetID+"/settings.yaml"), selectedValueSettings("desired@example.test"))
			writeFile(t, filepath.Join(fixtureRoot, "expected/desired/user/fixture-user/targets/"+targetID+"/settings.yaml"), selectedValueSettings("live@example.test"))
			writeFile(t, filepath.Join(fixtureRoot, "expected/live/locations/home", tc.fromPath), tc.expectedLive)

			report, err := RunTestRoundtrip(TestRoundtripOptions{RepoRoot: repoRoot, TargetID: targetID, Roundtrip: true, Fixture: "basic"})
			require.NoError(t, err, TextTestRoundtrip(report))
			require.Equal(t, "ok", report.Summary.Status)
			require.Equal(t, 2, report.Summary.Cases)
			require.Equal(t, 2, report.Summary.Passed)

			payload, err := JSONTestRoundtrip(report)
			require.NoError(t, err)
			require.NotContains(t, payload, repoRoot)
			require.NotContains(t, payload, "live@example.test")
			require.NotContains(t, payload, "desired@example.test")
		})
	}
}

func TestRoundtripFixtureValidationAndSafetyFailClosed(t *testing.T) {
	t.Run("missing roundtrip fixtures", func(t *testing.T) {
		repoRoot, targetID := createRoundtripFileRecipe(t)

		report, err := RunTestRoundtrip(TestRoundtripOptions{RepoRoot: repoRoot, TargetID: targetID, Roundtrip: true})
		requireAppAuthorError(t, err, CodeFixtureNone, 2)
		require.Equal(t, CodeFixtureNone, report.Error.Code)
		require.Contains(t, diagnosticCodes(report.Diagnostics), CodeFixtureNone)
	})

	t.Run("strict manifest decoding", func(t *testing.T) {
		repoRoot, targetID := createRoundtripFileRecipe(t)
		fixtureRoot := roundtripFixtureRoot(repoRoot, targetID, "bad-manifest")
		writeFile(t, filepath.Join(fixtureRoot, "manifest.yaml"), `schema: dotfiles-manager.v2.app.roundtrip-fixture
schemaVersion: 1
target: `+targetID+`
synthetic: true
unknownField: nope
`)

		report, err := RunTestRoundtrip(TestRoundtripOptions{RepoRoot: repoRoot, TargetID: targetID, Roundtrip: true, Fixture: "bad-manifest"})
		requireAppAuthorError(t, err, CodeManifestInvalid, 2)
		require.Equal(t, CodeManifestInvalid, report.Error.Code)
		require.Contains(t, diagnosticCodes(report.Diagnostics), CodeManifestInvalid)
	})

	t.Run("symlinked fixture entries", func(t *testing.T) {
		repoRoot, targetID := createRoundtripFileRecipe(t)
		fixtureRoot := roundtripFixtureRoot(repoRoot, targetID, "unsafe")
		writeRoundtripManifest(t, fixtureRoot, targetID, "unsafe", []string{"config"})
		writeFile(t, filepath.Join(fixtureRoot, "input/live/locations/home/.config/demo/config.yaml"), "live\n")
		require.NoError(t, os.Symlink(t.TempDir(), filepath.Join(fixtureRoot, "input/live/symlinked-root")))

		report, err := RunTestRoundtrip(TestRoundtripOptions{RepoRoot: repoRoot, TargetID: targetID, Roundtrip: true, Fixture: "unsafe"})
		requireAppAuthorError(t, err, CodeFixtureUnsafe, 5)
		require.Equal(t, CodeFixtureUnsafe, report.Error.Code)
		require.Contains(t, diagnosticCodes(report.Diagnostics), CodeFixtureUnsafe)
	})

	t.Run("roundtrip mismatch", func(t *testing.T) {
		repoRoot, targetID := createRoundtripFileRecipe(t)
		fixtureRoot := roundtripFixtureRoot(repoRoot, targetID, "mismatch")
		writeRoundtripManifest(t, fixtureRoot, targetID, "mismatch", []string{"config"})
		writeFile(t, filepath.Join(fixtureRoot, "input/live/locations/home/.config/demo/config.yaml"), "SECRET_SHOULD_NOT_LEAK_LIVE\n")
		writeFile(t, filepath.Join(fixtureRoot, "input/desired/user/fixture-user/targets/"+targetID+"/artifacts/config"), "SECRET_SHOULD_NOT_LEAK_DESIRED\n")
		writeFile(t, filepath.Join(fixtureRoot, "expected/desired/user/fixture-user/targets/"+targetID+"/artifacts/config"), "SECRET_SHOULD_NOT_LEAK_EXPECTED\n")
		writeFile(t, filepath.Join(fixtureRoot, "expected/live/locations/home/.config/demo/config.yaml"), "SECRET_SHOULD_NOT_LEAK_DESIRED\n")

		report, err := RunTestRoundtrip(TestRoundtripOptions{RepoRoot: repoRoot, TargetID: targetID, Roundtrip: true, Fixture: "mismatch"})
		requireAppAuthorError(t, err, CodeRoundtripMismatch, 6)
		require.Equal(t, CodeRoundtripMismatch, report.Error.Code)
		require.Contains(t, diagnosticCodes(report.Diagnostics), CodeRoundtripMismatch)
		require.Contains(t, report.Diagnostics[len(report.Diagnostics)-1].Path, "expected/desired")
		payload, jsonErr := JSONTestRoundtrip(report)
		require.NoError(t, jsonErr)
		text := TextTestRoundtrip(report)
		for _, output := range []string{payload, text} {
			require.NotContains(t, output, "SECRET_SHOULD_NOT_LEAK_LIVE")
			require.NotContains(t, output, "SECRET_SHOULD_NOT_LEAK_DESIRED")
			require.NotContains(t, output, "SECRET_SHOULD_NOT_LEAK_EXPECTED")
		}
	})
}

func TestRoundtripScratchSelectedValueWriterRejectsSymlinkEscape(t *testing.T) {
	scratchRoot := t.TempDir()
	outsideRoot := t.TempDir()
	targetID := "local-selected-symlink-roundtrip"
	settingID := "user.email"
	uri := desiredSettingsURI(targetID, settingID, "user", roundtripFixturePlan{SubjectUser: defaultFixtureUser, SubjectMachine: defaultFixtureMachine})
	targetDir := filepath.Join(scratchRoot, "desired/user/fixture-user/targets", targetID)
	require.NoError(t, os.MkdirAll(filepath.Dir(targetDir), 0o755))
	require.NoError(t, os.Symlink(outsideRoot, targetDir))

	err := writeFixtureSelectedValue(scratchRoot, uri, settingID, selectedvalue.SetString("SECRET_SHOULD_NOT_LEAK"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "symlink")
	_, statErr := os.Stat(filepath.Join(outsideRoot, "settings.yaml"))
	require.True(t, errors.Is(statErr, os.ErrNotExist), "outside settings.yaml must not be created: %v", statErr)
}

func TestRoundtripSelectedValueHelperBranches(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value selectedvalue.Desired
	}{
		{name: "string", value: selectedvalue.SetString("value")},
		{name: "bool", value: selectedvalue.SetBool(true)},
		{name: "number", value: selectedvalue.SetNumber(json.Number("42"))},
		{name: "null", value: selectedvalue.SetNull()},
		{name: "delete", value: selectedvalue.Delete()},
	} {
		t.Run("fixture value "+tc.name, func(t *testing.T) {
			_, err := fixtureSettingValueFromSelected(tc.value)
			require.NoError(t, err)
		})
		t.Run("scalar "+tc.name, func(t *testing.T) {
			got, err := selectedScalar(tc.value)
			if tc.name == "delete" {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tc.name == "null" {
				require.Nil(t, got)
			}
		})
	}
	_, err := fixtureSettingValueFromSelected(selectedvalue.Desired{})
	require.Error(t, err)
	_, err = selectedScalar(selectedvalue.Desired{})
	require.Error(t, err)

	_, err = iniStateFromSelected(selectedvalue.SetString("value"))
	require.NoError(t, err)
	_, err = iniStateFromSelected(selectedvalue.Delete())
	require.NoError(t, err)
	_, err = iniStateFromSelected(selectedvalue.SetBool(true))
	require.Error(t, err)
	for name, fn := range map[string]func(selectedvalue.Desired) error{
		"json":  func(v selectedvalue.Desired) error { _, err := jsonStateFromSelected(v); return err },
		"yaml":  func(v selectedvalue.Desired) error { _, err := yamlStateFromSelected(v); return err },
		"toml":  func(v selectedvalue.Desired) error { _, err := tomlStateFromSelected(v); return err },
		"plist": func(v selectedvalue.Desired) error { _, err := plistStateFromSelected(v); return err },
	} {
		t.Run("state "+name+" set", func(t *testing.T) {
			require.NoError(t, fn(selectedvalue.SetString("value")))
		})
		t.Run("state "+name+" delete", func(t *testing.T) {
			require.NoError(t, fn(selectedvalue.Delete()))
		})
		t.Run("state "+name+" invalid", func(t *testing.T) {
			require.Error(t, fn(selectedvalue.Desired{}))
		})
	}
}

func TestRoundtripMetadataAndReportingHelperBranches(t *testing.T) {
	require.Equal(t, recipe.LifecycleWarn, effectiveLifecycle(&recipe.Recipe{}, recipe.Setting{Lifecycle: recipe.LifecycleWarn}, recipe.Resource{Lifecycle: recipe.LifecycleAllowed}))
	require.Equal(t, recipe.LifecycleAllowed, effectiveLifecycle(&recipe.Recipe{}, recipe.Setting{}, recipe.Resource{Lifecycle: recipe.LifecycleAllowed}))
	require.Equal(t, "", effectiveLifecycle(&recipe.Recipe{}, recipe.Setting{}, recipe.Resource{}))
	require.NoError(t, lifecycleSupported(recipe.LifecycleWarn))
	require.Error(t, lifecycleSupported(recipe.LifecycleQuitIfRunning))
	require.NoError(t, roundtripRedactionSafety(recipe.SensitivityPersonal, recipe.RedactionRedactedForDisplay, "setting"))
	require.Error(t, roundtripRedactionSafety(recipe.SensitivitySecret, recipe.RedactionRedactedForDisplay, "setting"))
	require.Error(t, roundtripRedactionSafety(recipe.SensitivityPersonal, recipe.RedactionBlockedSave, "setting"))

	rec := &recipe.Recipe{Settings: map[string]recipe.Setting{
		"b": {ScopeDefault: "machine"},
		"a": {},
	}}
	got, err := fixtureSettings(rec, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b"}, got)
	got, err = fixtureSettings(rec, []string{"b"})
	require.NoError(t, err)
	require.Equal(t, []string{"b"}, got)
	_, err = fixtureSettings(rec, []string{"missing"})
	require.Error(t, err)

	require.Equal(t, "user", settingScope(&recipe.Recipe{Settings: map[string]recipe.Setting{"setting": {}}}, "setting"))
	require.Equal(t, "machine", settingScope(&recipe.Recipe{Settings: map[string]recipe.Setting{"setting": {ScopeDefault: "machine"}}}, "setting"))
	require.Equal(t, "desired://shared/-/targets/local-demo/settings#setting", desiredSettingsURI("local-demo", "setting", "shared", roundtripFixturePlan{}))
	require.Equal(t, "machine/fixture-machine/targets/local-demo", desiredTargetRelDir("machine", "local-demo", roundtripFixturePlan{SubjectMachine: "fixture-machine"}))
	require.Equal(t, "machine-user/fixture-machine/fixture-user/targets/local-demo", desiredTargetRelDir("machine-user", "local-demo", roundtripFixturePlan{SubjectUser: "fixture-user", SubjectMachine: "fixture-machine"}))

	require.NoError(t, validateModes([]string{"apply", "save"}))
	require.Error(t, validateModes([]string{"save", "save"}))
	require.Error(t, validateModes([]string{"bad"}))
	require.NoError(t, validateIDs("fixture setting", []string{"user.email"}))
	require.Error(t, validateIDs("fixture setting", []string{"../bad"}))

	payload, err := JSONTestRoundtrip(nil)
	require.NoError(t, err)
	require.Contains(t, payload, `"status": "error"`)
	require.Contains(t, TextTestRoundtrip(nil), "summary status=error")

	fixture := RoundtripFixture{}
	finishFixture(&fixture)
	require.Equal(t, "blocked", fixture.Status)
	require.Equal(t, fixtureReasonNoRunnableCases, fixture.Reason)
	for _, tc := range []struct {
		name   string
		cases  []RoundtripCase
		status string
		reason string
	}{
		{name: "failed", cases: []RoundtripCase{{Save: "failed"}}, status: "failed", reason: fixtureReasonRoundtripMismatch},
		{name: "blocked", cases: []RoundtripCase{{Save: "blocked"}}, status: "blocked", reason: fixtureReasonSafetyBlocked},
		{name: "skipped", cases: []RoundtripCase{{Save: "skipped"}}, status: "blocked", reason: fixtureReasonNoRunnableCases},
		{name: "partial skip", cases: []RoundtripCase{{Save: "passed", Apply: "skipped"}}, status: "skipped", reason: fixtureReasonUnsupportedDriver},
		{name: "passed", cases: []RoundtripCase{{Save: "passed", Apply: "passed"}}, status: "passed", reason: fixtureReasonOK},
	} {
		t.Run("finish fixture "+tc.name, func(t *testing.T) {
			fixture := RoundtripFixture{Cases: tc.cases}
			finishFixture(&fixture)
			require.Equal(t, tc.status, fixture.Status)
			require.Equal(t, tc.reason, fixture.Reason)
		})
	}

	report := baseTestRoundtripReport()
	report.AppTestRoundtrip.Fixtures = []RoundtripFixture{{Cases: []RoundtripCase{{Save: "passed", Apply: "failed"}}}}
	finishTestRoundtrip(report)
	require.Equal(t, "partial", report.Summary.Status)
	require.Equal(t, 1, report.Summary.Passed)
	require.Equal(t, 1, report.Summary.Failed)
	report, err = failTestRoundtrip(nil, CodeFixtureInvalid, "invalid", 2, nil)
	require.Error(t, err)
	require.Equal(t, "blocked", report.Summary.Status)
}

func TestRoundtripFilesystemHelperSafetyBranches(t *testing.T) {
	root := t.TempDir()
	require.Error(t, ensureNoSymlinkPath(root, filepath.Join(root, "..", "outside")))
	writeFile(t, filepath.Join(root, "file-parent"), "x")
	require.Error(t, ensureNoSymlinkPath(root, filepath.Join(root, "file-parent", "child")))

	_, err := liveFileTarget(recipe.Resource{Path: "../escape", Location: "home"}, roundtripScratch{LiveRoot: root})
	require.Error(t, err)
	_, err = desiredFileTarget("local-demo", "../bad", "user", roundtripFixturePlan{SubjectUser: defaultFixtureUser}, root)
	require.Error(t, err)

	dst := filepath.Join(t.TempDir(), "dst")
	require.Error(t, copyFixtureSubtree(filepath.Join(t.TempDir(), "missing"), dst))
	fileSrc := filepath.Join(t.TempDir(), "file")
	writeFile(t, fileSrc, "x")
	require.Error(t, copyFixtureSubtree(fileSrc, dst))
	linkSrc := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(t.TempDir(), linkSrc))
	require.Error(t, copyFixtureSubtree(linkSrc, dst))
	treeSrc := t.TempDir()
	require.NoError(t, os.Symlink(t.TempDir(), filepath.Join(treeSrc, "child-link")))
	require.Error(t, copyFixtureSubtree(treeSrc, dst))

	_, err = collectTree(filepath.Join(t.TempDir(), "missing"))
	require.Error(t, err)
	fileRoot := filepath.Join(t.TempDir(), "file-root")
	writeFile(t, fileRoot, "x")
	_, err = collectTree(fileRoot)
	require.Error(t, err)
	linkRoot := filepath.Join(t.TempDir(), "link-root")
	require.NoError(t, os.Symlink(t.TempDir(), linkRoot))
	_, err = collectTree(linkRoot)
	require.Error(t, err)

	fixtureRoot := t.TempDir()
	_, cleanup, err := prepareRoundtripScratch(roundtripFixturePlan{AbsRoot: fixtureRoot})
	require.Error(t, err)
	cleanup()
}

func TestRoundtripManifestAdditionalValidationBranches(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{
			name: "invalid mode",
			body: `schema: dotfiles-manager.v2.app.roundtrip-fixture
schemaVersion: 1
target: TARGET
synthetic: true
modes: [bad]
`,
		},
		{
			name: "duplicate mode",
			body: `schema: dotfiles-manager.v2.app.roundtrip-fixture
schemaVersion: 1
target: TARGET
synthetic: true
modes: [save, save]
`,
		},
		{
			name: "invalid subject",
			body: `schema: dotfiles-manager.v2.app.roundtrip-fixture
schemaVersion: 1
target: TARGET
synthetic: true
subjects:
  user: ../bad
`,
		},
		{
			name: "invalid setting id",
			body: `schema: dotfiles-manager.v2.app.roundtrip-fixture
schemaVersion: 1
target: TARGET
synthetic: true
settings: [bad/path]
`,
		},
		{
			name: "unsafe description",
			body: `schema: dotfiles-manager.v2.app.roundtrip-fixture
schemaVersion: 1
target: TARGET
synthetic: true
description: |
  line one
  line two
`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repoRoot, targetID := createRoundtripFileRecipe(t)
			fixtureRoot := roundtripFixtureRoot(repoRoot, targetID, "invalid")
			writeFile(t, filepath.Join(fixtureRoot, "manifest.yaml"), strings.ReplaceAll(tc.body, "TARGET", targetID))
			writeFile(t, filepath.Join(fixtureRoot, "input/live/.keep"), "")
			writeFile(t, filepath.Join(fixtureRoot, "input/desired/.keep"), "")
			writeFile(t, filepath.Join(fixtureRoot, "expected/live/.keep"), "")
			writeFile(t, filepath.Join(fixtureRoot, "expected/desired/.keep"), "")

			report, err := RunTestRoundtrip(TestRoundtripOptions{RepoRoot: repoRoot, TargetID: targetID, Roundtrip: true, Fixture: "invalid"})
			requireAppAuthorError(t, err, CodeManifestInvalid, 2)
			require.Contains(t, diagnosticCodes(report.Diagnostics), CodeManifestInvalid)
		})
	}
}

func TestRoundtripTopLevelFailureBranches(t *testing.T) {
	fileRoot := filepath.Join(t.TempDir(), "not-a-dir")
	writeFile(t, fileRoot, "x")
	report, err := RunTestRoundtrip(TestRoundtripOptions{RepoRoot: fileRoot, TargetID: "local-demo", Roundtrip: true})
	requireAppAuthorError(t, err, CodeRepoInvalid, 2)
	require.Equal(t, CodeRepoInvalid, report.Error.Code)

	report, err = RunTestRoundtrip(TestRoundtripOptions{RepoRoot: t.TempDir(), TargetID: "../bad", Roundtrip: true})
	requireAppAuthorError(t, err, CodeTargetInvalid, 2)
	require.Equal(t, CodeTargetInvalid, report.Error.Code)

	repoRoot, targetID := createRoundtripFileRecipe(t)
	report, err = RunTestRoundtrip(TestRoundtripOptions{RepoRoot: repoRoot, TargetID: targetID})
	requireAppAuthorError(t, err, CodeRoundtripModeRequired, 2)
	require.Equal(t, CodeRoundtripModeRequired, report.Error.Code)

	report, err = RunTestRoundtrip(TestRoundtripOptions{RepoRoot: t.TempDir(), TargetID: recipe.GitTarget, Roundtrip: true})
	requireAppAuthorError(t, err, CodeTargetCollision, 2)
	require.Equal(t, CodeTargetCollision, report.Error.Code)

	report, err = RunTestRoundtrip(TestRoundtripOptions{RepoRoot: t.TempDir(), TargetID: "local-missing", Roundtrip: true})
	requireAppAuthorError(t, err, CodeRecipeMissing, 2)
	require.Equal(t, CodeRecipeMissing, report.Error.Code)

	mismatchRoot, mismatchTarget := createRoundtripFileRecipe(t)
	recipePath := filepath.Join(mismatchRoot, "recipes/local", mismatchTarget, "recipe.yaml")
	body := readFile(t, recipePath)
	writeFile(t, recipePath, strings.Replace(body, "target: "+mismatchTarget, "target: local-other-target", 1))
	report, err = RunTestRoundtrip(TestRoundtripOptions{RepoRoot: mismatchRoot, TargetID: mismatchTarget, Roundtrip: true})
	requireAppAuthorError(t, err, CodeTargetMismatch, 2)
	require.Equal(t, CodeTargetMismatch, report.Error.Code)

	invalidRoot := t.TempDir()
	invalidTarget := "local-invalid-recipe"
	writeFile(t, filepath.Join(invalidRoot, "recipes/local", invalidTarget, "recipe.yaml"), "schema: wrong\n")
	report, err = RunTestRoundtrip(TestRoundtripOptions{RepoRoot: invalidRoot, TargetID: invalidTarget, Roundtrip: true})
	requireAppAuthorError(t, err, CodeRecipeInvalid, 2)
	require.Equal(t, CodeRecipeInvalid, report.Error.Code)
}

func TestRoundtripScratchSelectedValueWriterExistingSettingsBranches(t *testing.T) {
	scratchRoot := t.TempDir()
	targetID := "local-existing-settings-roundtrip"
	settingID := "user.email"
	uri := desiredSettingsURI(targetID, settingID, "user", roundtripFixturePlan{SubjectUser: defaultFixtureUser, SubjectMachine: defaultFixtureMachine})
	settingsPath := filepath.Join(scratchRoot, "desired/user/fixture-user/targets", targetID, "settings.yaml")
	writeFile(t, settingsPath, `schema: dotfiles-manager.v2.desired-settings
schemaVersion: 1
`)
	require.NoError(t, writeFixtureSelectedValue(scratchRoot, uri, settingID, selectedvalue.SetBool(true)))
	require.Contains(t, readFile(t, settingsPath), "kind: bool")

	require.Error(t, writeFixtureSelectedValue(scratchRoot, uri+"-wrong", settingID, selectedvalue.SetString("value")))
	require.Error(t, writeFixtureSelectedValue(scratchRoot, "desired://user/fixture-user/targets/"+targetID+"/artifacts/config", settingID, selectedvalue.SetString("value")))

	invalidSchemaRoot := t.TempDir()
	invalidSchemaPath := filepath.Join(invalidSchemaRoot, "desired/user/fixture-user/targets", targetID, "settings.yaml")
	writeFile(t, invalidSchemaPath, `schema: wrong
schemaVersion: 1
values: {}
`)
	require.Error(t, writeFixtureSelectedValue(invalidSchemaRoot, uri, settingID, selectedvalue.SetString("value")))

	duplicateRoot := t.TempDir()
	duplicatePath := filepath.Join(duplicateRoot, "desired/user/fixture-user/targets", targetID, "settings.yaml")
	writeFile(t, duplicatePath, `schema: dotfiles-manager.v2.desired-settings
schema: dotfiles-manager.v2.desired-settings
schemaVersion: 1
values: {}
`)
	require.Error(t, writeFixtureSelectedValue(duplicateRoot, uri, settingID, selectedvalue.SetString("value")))

	malformedRoot := t.TempDir()
	malformedPath := filepath.Join(malformedRoot, "desired/user/fixture-user/targets", targetID, "settings.yaml")
	writeFile(t, malformedPath, "schema: [\n")
	require.Error(t, writeFixtureSelectedValue(malformedRoot, uri, settingID, selectedvalue.SetString("value")))
}

func TestRoundtripSelectorAndApplyHelperBranches(t *testing.T) {
	require.Equal(t, inidriverZeroSelectorSection(), iniSelector(nil).Section)
	require.Empty(t, jsonSelector(nil).Path)
	require.Empty(t, yamlSelector(nil).Path)
	require.Empty(t, tomlSelector(nil).Path)
	require.Empty(t, plistSelector(nil).Path)

	err := applySelectedDesiredToLive(recipe.Resource{Driver: "unsupported", Location: "home", Path: "config.txt"}, roundtripScratch{LiveRoot: t.TempDir()}, selectedvalue.SetString("value"))
	require.Error(t, err)
}

func inidriverZeroSelectorSection() string {
	return ""
}

func TestRoundtripOperationErrorBranches(t *testing.T) {
	t.Run("file save/apply missing inputs", func(t *testing.T) {
		repoRoot, targetID := createRoundtripFileRecipe(t)
		rec, err := loadLocalRecipe(repoRoot, targetID)
		require.NoError(t, err)
		resourceID, resource, err := rec.ResourceForSetting("config")
		require.NoError(t, err)
		fixtureRoot := roundtripFixtureRoot(repoRoot, targetID, "missing")
		writeRoundtripManifest(t, fixtureRoot, targetID, "missing", []string{"config"})
		writeFile(t, filepath.Join(fixtureRoot, "input/live/.keep"), "")
		writeFile(t, filepath.Join(fixtureRoot, "input/desired/.keep"), "")
		writeFile(t, filepath.Join(fixtureRoot, "expected/live/.keep"), "")
		writeFile(t, filepath.Join(fixtureRoot, "expected/desired/.keep"), "")
		plan, err := loadRoundtripFixture(repoRoot, targetID, "missing")
		require.NoError(t, err)

		err = runSaveRoundtrip(repoRoot, rec, "config", resourceID, resource, plan)
		require.Error(t, err)
		require.Equal(t, CodeRoundtripFailed, errorCode(err))
		err = runApplyRoundtrip(repoRoot, rec, "config", resourceID, resource, plan)
		require.Error(t, err)
		require.Equal(t, CodeRoundtripFailed, errorCode(err))
	})

	t.Run("selected save/apply missing inputs", func(t *testing.T) {
		repoRoot := t.TempDir()
		targetID := "local-selected-missing-roundtrip"
		_, err := RunCreate(CreateOptions{
			RepoRoot:     repoRoot,
			TargetID:     targetID,
			Template:     TemplateSelectedValue,
			FromPath:     ".config/demo/config.json",
			SettingID:    "user.email",
			SettingLabel: "User email",
			Driver:       recipe.JSONFileDriverID,
			Selector:     "user.email",
			ScopeDefault: "user",
			Lifecycle:    recipe.LifecycleAllowed,
		})
		require.NoError(t, err)
		rec, err := loadLocalRecipe(repoRoot, targetID)
		require.NoError(t, err)
		resourceID, resource, err := rec.ResourceForSetting("user.email")
		require.NoError(t, err)
		fixtureRoot := roundtripFixtureRoot(repoRoot, targetID, "missing")
		writeRoundtripManifest(t, fixtureRoot, targetID, "missing", []string{"user.email"})
		writeFile(t, filepath.Join(fixtureRoot, "input/live/.keep"), "")
		writeFile(t, filepath.Join(fixtureRoot, "input/desired/.keep"), "")
		writeFile(t, filepath.Join(fixtureRoot, "expected/live/.keep"), "")
		writeFile(t, filepath.Join(fixtureRoot, "expected/desired/.keep"), "")
		plan, err := loadRoundtripFixture(repoRoot, targetID, "missing")
		require.NoError(t, err)

		err = runSaveRoundtrip(repoRoot, rec, "user.email", resourceID, resource, plan)
		require.Error(t, err)
		require.Equal(t, CodeRoundtripFailed, errorCode(err))
		err = runApplyRoundtrip(repoRoot, rec, "user.email", resourceID, resource, plan)
		require.Error(t, err)
		require.Equal(t, CodeRoundtripFailed, errorCode(err))
	})

	t.Run("metadata support decisions", func(t *testing.T) {
		require.NoError(t, roundtripSettingSupported(&recipe.Recipe{}, recipe.Setting{Sensitivity: recipe.SensitivityPersonal, Redaction: recipe.RedactionRedactedForDisplay}, recipe.Resource{Driver: recipe.FileDriverID, Sensitivity: recipe.SensitivityPersonal, Redaction: recipe.RedactionRedactedForDisplay}))
		require.Equal(t, CodeLifecycleUnsupported, errorCode(roundtripSettingSupported(&recipe.Recipe{}, recipe.Setting{Lifecycle: recipe.LifecycleQuitIfRunning}, recipe.Resource{Driver: recipe.FileDriverID})))
		require.Equal(t, CodeFixtureInvalid, errorCode(roundtripSettingSupported(&recipe.Recipe{}, recipe.Setting{Sensitivity: recipe.SensitivitySecret}, recipe.Resource{Driver: recipe.FileDriverID})))
		require.Equal(t, CodeFixtureInvalid, errorCode(roundtripSettingSupported(&recipe.Recipe{}, recipe.Setting{}, recipe.Resource{Driver: recipe.FileDriverID, Redaction: recipe.RedactionUnavailable})))
		require.Equal(t, CodeNativeValidateOnly, errorCode(roundtripSettingSupported(&recipe.Recipe{}, recipe.Setting{}, recipe.Resource{Driver: recipe.NativeExportDriverID})))
		require.Equal(t, CodeDriverUnsupported, errorCode(roundtripSettingSupported(&recipe.Recipe{}, recipe.Setting{}, recipe.Resource{Driver: recipe.FileTreeDriverID})))
	})

	t.Run("fixture planning failures", func(t *testing.T) {
		repoRoot, targetID := createRoundtripFileRecipe(t)
		rec, err := loadLocalRecipe(repoRoot, targetID)
		require.NoError(t, err)
		diagnostics := []Diagnostic{}
		fixture := runRoundtripFixture(repoRoot, rec, roundtripFixturePlan{Name: "bad", Settings: []string{"missing"}}, &diagnostics)
		require.Equal(t, "blocked", fixture.Status)
		require.Contains(t, diagnosticCodes(diagnostics), CodeManifestInvalid)

		badRec := &recipe.Recipe{
			Settings:  map[string]recipe.Setting{"setting": {Resource: "missing"}},
			Resources: map[string]recipe.Resource{},
		}
		diagnostics = []Diagnostic{}
		fixture = runRoundtripFixture(repoRoot, badRec, roundtripFixturePlan{Name: "bad", Settings: []string{"setting"}, Modes: []string{"save", "apply"}}, &diagnostics)
		require.Len(t, fixture.Cases, 1)
		require.Equal(t, "blocked", fixture.Cases[0].Save)
		require.Contains(t, diagnosticCodes(diagnostics), CodeRecipeInvalid)
	})
}

func TestRoundtripTreeAndMiscHelperBranches(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "nested/config.txt"), "value")
	dst := filepath.Join(t.TempDir(), "dst")
	require.NoError(t, copyFixtureSubtree(src, dst))
	require.Equal(t, "value", readFile(t, filepath.Join(dst, "nested/config.txt")))
	entries, err := collectTree(dst)
	require.NoError(t, err)
	require.Equal(t, []treeEntry{{Path: "nested", Kind: "dir"}, {Path: "nested/config.txt", Kind: "file", Bytes: []byte("value")}}, entries)
	require.NoError(t, compareExpectedTree(dst, dst, "expected/live"))

	empty := t.TempDir()
	require.Error(t, compareExpectedTree(dst, empty, "expected/live"))
	require.Error(t, compareExpectedTree(filepath.Join(t.TempDir(), "missing"), empty, "expected/live"))
	require.Error(t, compareExpectedTree(empty, filepath.Join(t.TempDir(), "missing"), "expected/live"))

	report := baseTestRoundtripReport()
	report.AppTestRoundtrip.Fixtures = []RoundtripFixture{{Status: "blocked"}}
	finishTestRoundtrip(report)
	require.Equal(t, "blocked", report.Summary.Status)
	require.Equal(t, 1, report.Summary.Blocked)
	report = baseTestRoundtripReport()
	finishTestRoundtrip(report)
	require.Equal(t, "blocked", report.Summary.Status)

	report, err = failTestRoundtripWithExistingDiagnostics(nil, CodeFixtureInvalid, "invalid", 2, map[string]any{"path": "fixture"})
	require.Error(t, err)
	require.Equal(t, CodeFixtureInvalid, report.Error.Code)
	report = baseTestRoundtripReport()
	report.Summary.Status = "error"
	report.Summary.Blocked = 2
	report, err = failTestRoundtripWithExistingDiagnostics(report, CodeFixtureInvalid, "invalid", 2, nil)
	require.Error(t, err)
	require.Equal(t, "error", report.Summary.Status)
	require.Equal(t, 2, report.Summary.Blocked)

	require.False(t, hasMode([]string{"save"}, "apply"))
	root := t.TempDir()
	require.Equal(t, "root", relFromRoot(root, root, "root"))
	require.Equal(t, "root/nested/file", relFromRoot(root, filepath.Join(root, "nested/file"), "root"))
	require.Equal(t, ".", firstNonEmpty("", ""))
	require.Equal(t, "first", firstNonEmpty("", "first", "second"))
	require.Equal(t, "fixture", errorPath(&Error{Details: map[string]any{"path": "fixture"}}))
	require.Equal(t, "$.settings.setting", errorPath(&Error{Details: map[string]any{"setting": "setting"}}))
	require.Equal(t, "", errorPath(errors.New("plain")))

	require.NoError(t, rejectDuplicateYAMLKeys(nil))
	var scalar yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte("value\n"), &scalar))
	require.NoError(t, rejectDuplicateYAMLKeys(&scalar))
}

func TestRoundtripApplySelectedInvalidStateBranches(t *testing.T) {
	scratch := roundtripScratch{LiveRoot: t.TempDir()}
	invalidPathResource := recipe.Resource{Driver: recipe.JSONFileDriverID, Location: "home", Path: "../bad", Selector: &recipe.Selector{Path: []string{"user", "email"}}}
	require.Error(t, applySelectedDesiredToLive(invalidPathResource, scratch, selectedvalue.SetString("value")))

	for _, tc := range []struct {
		name     string
		driverID string
		selector *recipe.Selector
	}{
		{name: "ini", driverID: recipe.IniFileDriverID, selector: &recipe.Selector{Section: "user", Key: "email"}},
		{name: "json", driverID: recipe.JSONFileDriverID, selector: &recipe.Selector{Path: []string{"user", "email"}}},
		{name: "yaml", driverID: recipe.YAMLFileDriverID, selector: &recipe.Selector{Path: []string{"user", "email"}}},
		{name: "toml", driverID: recipe.TOMLFileDriverID, selector: &recipe.Selector{Path: []string{"user", "email"}}},
		{name: "plist", driverID: recipe.PlistFileDriverID, selector: &recipe.Selector{Path: []string{"user", "email"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resource := recipe.Resource{Driver: tc.driverID, Location: "home", Path: ".config/demo/config", Selector: tc.selector}
			require.Error(t, applySelectedDesiredToLive(resource, scratch, selectedvalue.Desired{}))
		})
	}
}

func TestRoundtripDiscoveryAndManifestHelperBranches(t *testing.T) {
	repoRoot := t.TempDir()
	targetID := "local-discovery-roundtrip"
	_, err := discoverRoundtripFixtures(repoRoot, targetID, "missing")
	require.Error(t, err)
	require.Equal(t, CodeFixtureMissing, errorCode(err))

	roundtripRoot := filepath.Join(repoRoot, "recipes/local", targetID, "fixtures/roundtrip")
	writeFile(t, filepath.Join(roundtripRoot, "not-a-dir"), "x")
	_, err = discoverRoundtripFixtures(repoRoot, targetID, "")
	require.Error(t, err)
	require.Equal(t, CodeFixtureInvalid, errorCode(err))
	require.NoError(t, os.Remove(filepath.Join(roundtripRoot, "not-a-dir")))

	require.NoError(t, os.Symlink(t.TempDir(), filepath.Join(roundtripRoot, "linked")))
	_, err = discoverRoundtripFixtures(repoRoot, targetID, "")
	require.Error(t, err)
	require.Equal(t, CodeFixtureUnsafe, errorCode(err))
	require.NoError(t, os.Remove(filepath.Join(roundtripRoot, "linked")))

	require.NoError(t, os.MkdirAll(filepath.Join(roundtripRoot, "BadName"), 0o755))
	_, err = discoverRoundtripFixtures(repoRoot, targetID, "")
	require.Error(t, err)
	require.Equal(t, CodeFixtureInvalid, errorCode(err))
	require.NoError(t, os.Remove(filepath.Join(roundtripRoot, "BadName")))

	_, err = discoverRoundtripFixtures(repoRoot, targetID, "")
	require.Error(t, err)
	require.Equal(t, CodeFixtureNone, errorCode(err))

	missingManifest := filepath.Join(roundtripRoot, "missing-manifest")
	require.NoError(t, os.MkdirAll(missingManifest, 0o755))
	writeFile(t, filepath.Join(missingManifest, "input/live/.keep"), "")
	writeFile(t, filepath.Join(missingManifest, "input/desired/.keep"), "")
	writeFile(t, filepath.Join(missingManifest, "expected/live/.keep"), "")
	writeFile(t, filepath.Join(missingManifest, "expected/desired/.keep"), "")
	_, err = loadRoundtripFixture(repoRoot, targetID, "missing-manifest")
	require.Error(t, err)
	require.Equal(t, CodeManifestInvalid, errorCode(err))
	_, err = discoverRoundtripFixtures(repoRoot, targetID, "")
	require.Error(t, err)
	require.Equal(t, CodeManifestInvalid, errorCode(err))

	tooMany := filepath.Join(t.TempDir(), "too-many")
	require.NoError(t, os.MkdirAll(tooMany, 0o755))
	for i := 0; i < 201; i++ {
		writeFile(t, filepath.Join(tooMany, "file-"+strconv.Itoa(i)), "x")
	}
	require.Error(t, validateFixtureTree(tooMany, "fixture"))

	tooLarge := filepath.Join(t.TempDir(), "too-large")
	require.NoError(t, os.MkdirAll(tooLarge, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tooLarge, "big"), bytes.Repeat([]byte("x"), int(maxFixtureBytes+1)), 0o644))
	require.Error(t, validateFixtureTree(tooLarge, "fixture"))

	pathRoot := t.TempDir()
	writeFile(t, filepath.Join(pathRoot, "recipes"), "file-parent")
	err = validateFixturePathComponents(pathRoot, "recipes/local/"+targetID, false)
	require.Error(t, err)
	require.Equal(t, CodeFixtureInvalid, errorCode(err))
}

func TestRoundtripDirectInvalidBranchHelpers(t *testing.T) {
	scratchRoot := t.TempDir()
	err := writeFixtureSelectedValue(scratchRoot, "not-desired", "setting", selectedvalue.SetString("value"))
	require.Error(t, err)
	err = writeFixtureSelectedValue(scratchRoot, desiredSettingsURI("local-demo", "setting", "user", roundtripFixturePlan{SubjectUser: defaultFixtureUser}), "setting", selectedvalue.Desired{})
	require.Error(t, err)

	repoRoot, targetID := createRoundtripFileRecipe(t)
	rec, err := loadLocalRecipe(repoRoot, targetID)
	require.NoError(t, err)
	plan := roundtripFixturePlan{SubjectUser: defaultFixtureUser, SubjectMachine: defaultFixtureMachine}
	scratch := roundtripScratch{Root: t.TempDir(), LiveRoot: t.TempDir()}
	err = saveFileRoundtrip(rec, "config", recipe.Resource{Driver: recipe.FileDriverID, Location: "home", Path: "../bad"}, plan, scratch)
	require.Error(t, err)
	err = applyFileRoundtrip(rec, "config", recipe.Resource{Driver: recipe.FileDriverID, Location: "home", Path: "../bad"}, plan, scratch)
	require.Error(t, err)
}

func TestRoundtripCoverageBufferMeaningfulFailureBranches(t *testing.T) {
	t.Run("unknown fixture setting through top level", func(t *testing.T) {
		repoRoot, targetID := createRoundtripFileRecipe(t)
		fixtureRoot := roundtripFixtureRoot(repoRoot, targetID, "unknown-setting")
		writeRoundtripManifest(t, fixtureRoot, targetID, "unknown-setting", []string{"missing"})
		writeFile(t, filepath.Join(fixtureRoot, "input/live/.keep"), "")
		writeFile(t, filepath.Join(fixtureRoot, "input/desired/.keep"), "")
		writeFile(t, filepath.Join(fixtureRoot, "expected/live/.keep"), "")
		writeFile(t, filepath.Join(fixtureRoot, "expected/desired/.keep"), "")
		report, err := RunTestRoundtrip(TestRoundtripOptions{RepoRoot: repoRoot, TargetID: targetID, Roundtrip: true, Fixture: "unknown-setting"})
		requireAppAuthorError(t, err, CodeNoRunnableCases, 2)
		require.Contains(t, diagnosticCodes(report.Diagnostics), CodeManifestInvalid)
	})

	t.Run("scratch preparation missing desired tree", func(t *testing.T) {
		fixtureRoot := t.TempDir()
		writeFile(t, filepath.Join(fixtureRoot, "input/live/.keep"), "")
		_, cleanup, err := prepareRoundtripScratch(roundtripFixturePlan{AbsRoot: fixtureRoot})
		require.Error(t, err)
		cleanup()
	})

	t.Run("file desired target validation failures", func(t *testing.T) {
		repoRoot, targetID := createRoundtripFileRecipe(t)
		rec, err := loadLocalRecipe(repoRoot, targetID)
		require.NoError(t, err)
		plan := roundtripFixturePlan{SubjectUser: defaultFixtureUser, SubjectMachine: defaultFixtureMachine}
		scratch := roundtripScratch{Root: t.TempDir(), LiveRoot: t.TempDir()}
		writeFile(t, filepath.Join(scratch.LiveRoot, "locations/home/.config/demo/config.yaml"), "live\n")
		err = saveFileRoundtrip(rec, "../bad", recipe.Resource{Driver: recipe.FileDriverID, Location: "home", Path: ".config/demo/config.yaml"}, plan, scratch)
		require.Error(t, err)
		err = applyFileRoundtrip(rec, "../bad", recipe.Resource{Driver: recipe.FileDriverID, Location: "home", Path: ".config/demo/config.yaml"}, plan, scratch)
		require.Error(t, err)

		writeFile(t, filepath.Join(scratch.Root, "desired/user/fixture-user/targets", targetID, "artifacts/config"), "desired\n")
		err = applyFileRoundtrip(rec, "config", recipe.Resource{Driver: recipe.FileDriverID, Location: "home", Path: "../bad"}, plan, scratch)
		require.Error(t, err)
	})

	t.Run("file read and write operation failures", func(t *testing.T) {
		repoRoot, targetID := createRoundtripFileRecipe(t)
		rec, err := loadLocalRecipe(repoRoot, targetID)
		require.NoError(t, err)
		_, resource, err := rec.ResourceForSetting("config")
		require.NoError(t, err)
		plan := roundtripFixturePlan{SubjectUser: defaultFixtureUser, SubjectMachine: defaultFixtureMachine}

		err = runSaveRoundtrip(repoRoot, rec, "config", "config-file", resource, roundtripFixturePlan{AbsRoot: filepath.Join(t.TempDir(), "missing")})
		require.Error(t, err)
		err = runApplyRoundtrip(repoRoot, rec, "config", "config-file", resource, roundtripFixturePlan{AbsRoot: filepath.Join(t.TempDir(), "missing")})
		require.Error(t, err)

		scratch := roundtripScratch{Root: t.TempDir(), LiveRoot: t.TempDir()}
		livePath := filepath.Join(scratch.LiveRoot, "locations/home/.config/demo/config.yaml")
		require.NoError(t, os.MkdirAll(filepath.Dir(livePath), 0o755))
		require.NoError(t, os.Symlink(filepath.Join(t.TempDir(), "target"), livePath))
		err = saveFileRoundtrip(rec, "config", resource, plan, scratch)
		require.Error(t, err)

		scratch = roundtripScratch{Root: t.TempDir(), LiveRoot: t.TempDir()}
		desiredArtifact := filepath.Join(scratch.Root, "desired/user/fixture-user/targets", targetID, "artifacts/config")
		require.NoError(t, os.MkdirAll(filepath.Dir(desiredArtifact), 0o755))
		require.NoError(t, os.Symlink(filepath.Join(t.TempDir(), "target"), desiredArtifact))
		err = applyFileRoundtrip(rec, "config", resource, plan, scratch)
		require.Error(t, err)

		scratch = roundtripScratch{Root: t.TempDir(), LiveRoot: t.TempDir()}
		writeFile(t, filepath.Join(scratch.LiveRoot, "locations/home/.config/demo/config.yaml"), "live\n")
		writeFile(t, filepath.Join(scratch.Root, "desired/user/fixture-user/targets", targetID, "artifacts"), "not-a-dir")
		err = saveFileRoundtrip(rec, "config", resource, plan, scratch)
		require.Error(t, err)

		scratch = roundtripScratch{Root: t.TempDir(), LiveRoot: t.TempDir()}
		writeFile(t, filepath.Join(scratch.Root, "desired/user/fixture-user/targets", targetID, "artifacts/config"), "desired\n")
		writeFile(t, filepath.Join(scratch.LiveRoot, "locations/home/.config/demo"), "not-a-dir")
		err = applyFileRoundtrip(rec, "config", resource, plan, scratch)
		require.Error(t, err)
	})

	t.Run("selected apply missing resource and save write failure", func(t *testing.T) {
		targetID := "local-selected-buffer-roundtrip"
		plan := roundtripFixturePlan{SubjectUser: defaultFixtureUser, SubjectMachine: defaultFixtureMachine}
		rec := &recipe.Recipe{
			Target:    targetID,
			Locations: map[string]recipe.Location{"home": {Default: "~"}},
			Settings:  map[string]recipe.Setting{"user.email": {Resource: "missing", ScopeDefault: "user"}},
			Resources: map[string]recipe.Resource{},
		}
		scratch := roundtripScratch{Root: t.TempDir(), LiveRoot: t.TempDir()}
		writeFile(t, filepath.Join(scratch.Root, "desired/user/fixture-user/targets", targetID, "settings.yaml"), selectedValueSettings("desired@example.test"))
		err := applySelectedRoundtrip(rec, "user.email", plan, scratch)
		require.Error(t, err)
		require.Equal(t, CodeRecipeInvalid, errorCode(err))

		repoRoot := t.TempDir()
		_, err = RunCreate(CreateOptions{
			RepoRoot:     repoRoot,
			TargetID:     targetID,
			Template:     TemplateSelectedValue,
			FromPath:     ".config/demo/config.json",
			SettingID:    "user.email",
			SettingLabel: "User email",
			Driver:       recipe.JSONFileDriverID,
			Selector:     "user.email",
			ScopeDefault: "user",
			Lifecycle:    recipe.LifecycleAllowed,
		})
		require.NoError(t, err)
		realRec, err := loadLocalRecipe(repoRoot, targetID)
		require.NoError(t, err)
		scratch = roundtripScratch{Root: t.TempDir(), LiveRoot: t.TempDir()}
		writeFile(t, filepath.Join(scratch.LiveRoot, "locations/home/.config/demo/config.json"), `{"user":{"email":"live@example.test"}}`)
		targetDir := filepath.Join(scratch.Root, "desired/user/fixture-user/targets", targetID)
		require.NoError(t, os.MkdirAll(filepath.Dir(targetDir), 0o755))
		require.NoError(t, os.Symlink(t.TempDir(), targetDir))
		err = saveSelectedRoundtrip(realRec, "user.email", plan, scratch)
		require.Error(t, err)
		require.Equal(t, CodeRoundtripFailed, errorCode(err))

		scratch = roundtripScratch{Root: t.TempDir(), LiveRoot: t.TempDir()}
		writeFile(t, filepath.Join(scratch.Root, "desired/user/fixture-user/targets", targetID, "settings.yaml"), "schema: [\n")
		err = applySelectedRoundtrip(realRec, "user.email", plan, scratch)
		require.Error(t, err)
		require.Equal(t, CodeRoundtripFailed, errorCode(err))
	})
}

func TestRoundtripFailureDiagnosticsDoNotLeakMalformedFixtureValues(t *testing.T) {
	repoRoot := t.TempDir()
	targetID := "local-json-malformed-roundtrip"
	_, err := RunCreate(CreateOptions{
		RepoRoot:     repoRoot,
		TargetID:     targetID,
		Template:     TemplateSelectedValue,
		FromPath:     ".config/demo/config.json",
		SettingID:    "user.email",
		SettingLabel: "User email",
		Driver:       recipe.JSONFileDriverID,
		Selector:     "user.email",
		ScopeDefault: "user",
		Lifecycle:    recipe.LifecycleAllowed,
	})
	require.NoError(t, err)
	fixtureRoot := roundtripFixtureRoot(repoRoot, targetID, "malformed")
	writeRoundtripManifest(t, fixtureRoot, targetID, "malformed", []string{"user.email"})
	writeFile(t, filepath.Join(fixtureRoot, "input/live/locations/home/.config/demo/config.json"), `{"user":{"email":"SECRET_SHOULD_NOT_LEAK"`)
	writeFile(t, filepath.Join(fixtureRoot, "input/desired/user/fixture-user/targets/"+targetID+"/settings.yaml"), selectedValueSettings("desired@example.test"))
	writeFile(t, filepath.Join(fixtureRoot, "expected/desired/user/fixture-user/targets/"+targetID+"/settings.yaml"), selectedValueSettings("unused@example.test"))
	writeFile(t, filepath.Join(fixtureRoot, "expected/live/locations/home/.config/demo/config.json"), `{
  "user": {
    "email": "desired@example.test"
  }
}
`)

	report, err := RunTestRoundtrip(TestRoundtripOptions{RepoRoot: repoRoot, TargetID: targetID, Roundtrip: true, Fixture: "malformed"})
	requireAppAuthorError(t, err, CodeRoundtripFailed, 2)
	require.Contains(t, diagnosticCodes(report.Diagnostics), CodeRoundtripFailed)
	payload, jsonErr := JSONTestRoundtrip(report)
	require.NoError(t, jsonErr)
	text := TextTestRoundtrip(report)
	for _, output := range []string{payload, text} {
		require.NotContains(t, output, "SECRET_SHOULD_NOT_LEAK")
		require.NotContains(t, output, "desired@example.test")
	}
}

func TestRoundtripManifestSemanticValidation(t *testing.T) {
	cases := []struct {
		name string
		body func(targetID string) string
	}{
		{
			name: "schema mismatch",
			body: func(targetID string) string {
				return `schema: dotfiles-manager.v2.other
schemaVersion: 1
target: ` + targetID + `
synthetic: true
`
			},
		},
		{
			name: "version mismatch",
			body: func(targetID string) string {
				return `schema: dotfiles-manager.v2.app.roundtrip-fixture
schemaVersion: 2
target: ` + targetID + `
synthetic: true
`
			},
		},
		{
			name: "target mismatch",
			body: func(string) string {
				return `schema: dotfiles-manager.v2.app.roundtrip-fixture
schemaVersion: 1
target: local-other-target
synthetic: true
`
			},
		},
		{
			name: "name mismatch",
			body: func(targetID string) string {
				return `schema: dotfiles-manager.v2.app.roundtrip-fixture
schemaVersion: 1
target: ` + targetID + `
name: other-fixture
synthetic: true
`
			},
		},
		{
			name: "synthetic false",
			body: func(targetID string) string {
				return `schema: dotfiles-manager.v2.app.roundtrip-fixture
schemaVersion: 1
target: ` + targetID + `
synthetic: false
`
			},
		},
		{
			name: "synthetic missing",
			body: func(targetID string) string {
				return `schema: dotfiles-manager.v2.app.roundtrip-fixture
schemaVersion: 1
target: ` + targetID + `
`
			},
		},
		{
			name: "duplicate key",
			body: func(targetID string) string {
				return `schema: dotfiles-manager.v2.app.roundtrip-fixture
schema: dotfiles-manager.v2.app.roundtrip-fixture
schemaVersion: 1
target: ` + targetID + `
synthetic: true
`
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repoRoot, targetID := createRoundtripFileRecipe(t)
			fixtureRoot := roundtripFixtureRoot(repoRoot, targetID, "invalid")
			writeFile(t, filepath.Join(fixtureRoot, "manifest.yaml"), tc.body(targetID))

			report, err := RunTestRoundtrip(TestRoundtripOptions{RepoRoot: repoRoot, TargetID: targetID, Roundtrip: true, Fixture: "invalid"})
			requireAppAuthorError(t, err, CodeManifestInvalid, 2)
			require.Equal(t, CodeManifestInvalid, report.Error.Code)
		})
	}
}

func TestRoundtripRejectsInvalidTargetAndFixtureNamesBeforeTraversal(t *testing.T) {
	repoRoot := t.TempDir()
	for _, targetID := range []string{"../escape", "LocalUpper", "local/bad"} {
		t.Run("target "+targetID, func(t *testing.T) {
			report, err := RunTestRoundtrip(TestRoundtripOptions{RepoRoot: repoRoot, TargetID: targetID, Roundtrip: true})
			requireAppAuthorError(t, err, CodeTargetInvalid, 2)
			require.Equal(t, CodeTargetInvalid, report.Error.Code)
		})
	}

	_, err := RunCreate(CreateOptions{
		RepoRoot:     repoRoot,
		TargetID:     "local-fixture-name",
		Template:     TemplateFile,
		FromPath:     ".config/demo/config.yaml",
		SettingID:    "config",
		SettingLabel: "Config file",
		ScopeDefault: "user",
		Lifecycle:    recipe.LifecycleAllowed,
	})
	require.NoError(t, err)
	for _, fixtureName := range []string{"../escape", "Upper", "bad/name"} {
		t.Run("fixture "+fixtureName, func(t *testing.T) {
			report, err := RunTestRoundtrip(TestRoundtripOptions{RepoRoot: repoRoot, TargetID: "local-fixture-name", Roundtrip: true, Fixture: fixtureName})
			requireAppAuthorError(t, err, CodeFixtureInvalid, 2)
			require.Equal(t, CodeFixtureInvalid, report.Error.Code)
		})
	}
}

func TestRoundtripNativeRecipesAreValidateOnlyAndNeverExecute(t *testing.T) {
	repoRoot := t.TempDir()
	targetID := "local-native-roundtrip"
	markerPath := filepath.Join(t.TempDir(), "native-marker")
	writeFile(t, filepath.Join(repoRoot, "recipes/local/"+targetID+"/recipe.yaml"), nativeExportRecipe(targetID, markerPath))
	fixtureRoot := roundtripFixtureRoot(repoRoot, targetID, "native")
	writeRoundtripManifest(t, fixtureRoot, targetID, "native", []string{"settings"})

	report, err := RunTestRoundtrip(TestRoundtripOptions{RepoRoot: repoRoot, TargetID: targetID, Roundtrip: true, Fixture: "native"})
	requireAppAuthorError(t, err, CodeNoRunnableCases, 2)
	require.NoFileExists(t, markerPath)
	require.Contains(t, diagnosticCodes(report.Diagnostics), CodeNativeValidateOnly)
	require.Equal(t, "blocked", report.Summary.Status)
	require.Equal(t, 2, report.Summary.Skipped)
}

func TestRoundtripNativeImportIsSkippedAndNeverExecuted(t *testing.T) {
	repoRoot := t.TempDir()
	targetID := "local-native-import-roundtrip"
	exportMarker := filepath.Join(t.TempDir(), "native-export-marker")
	importMarker := filepath.Join(t.TempDir(), "native-import-marker")
	writeFile(t, filepath.Join(repoRoot, "recipes/local/"+targetID+"/recipe.yaml"), nativeExportImportRecipe(targetID, exportMarker, importMarker))
	fixtureRoot := roundtripFixtureRoot(repoRoot, targetID, "native")
	writeRoundtripManifest(t, fixtureRoot, targetID, "native", []string{"settings"})

	report, err := RunTestRoundtrip(TestRoundtripOptions{RepoRoot: repoRoot, TargetID: targetID, Roundtrip: true, Fixture: "native"})
	requireAppAuthorError(t, err, CodeNoRunnableCases, 2)
	require.NoFileExists(t, exportMarker)
	require.NoFileExists(t, importMarker)
	require.Contains(t, diagnosticCodes(report.Diagnostics), CodeNativeValidateOnly)
}

func TestRoundtripUnsupportedFileTreeAndLifecycleActionsFailClosed(t *testing.T) {
	t.Run("file tree skipped as unsupported", func(t *testing.T) {
		repoRoot := t.TempDir()
		targetID := "local-file-tree-roundtrip"
		writeFile(t, filepath.Join(repoRoot, "recipes/local/"+targetID+"/recipe.yaml"), fileTreeRecipe(targetID))
		fixtureRoot := roundtripFixtureRoot(repoRoot, targetID, "tree")
		writeRoundtripManifest(t, fixtureRoot, targetID, "tree", []string{"config"})

		report, err := RunTestRoundtrip(TestRoundtripOptions{RepoRoot: repoRoot, TargetID: targetID, Roundtrip: true, Fixture: "tree"})
		requireAppAuthorError(t, err, CodeNoRunnableCases, 2)
		require.Contains(t, diagnosticCodes(report.Diagnostics), CodeDriverUnsupported)
	})

	t.Run("lifecycle actions block before fixture writes", func(t *testing.T) {
		repoRoot := t.TempDir()
		targetID := "local-lifecycle-roundtrip"
		writeFile(t, filepath.Join(repoRoot, "recipes/local/"+targetID+"/recipe.yaml"), lifecycleActionRecipe(targetID))
		fixtureRoot := roundtripFixtureRoot(repoRoot, targetID, "lifecycle")
		writeRoundtripManifest(t, fixtureRoot, targetID, "lifecycle", []string{"config"})
		writeFile(t, filepath.Join(fixtureRoot, "input/live/locations/home/.config/demo/config.yaml"), "live\n")
		writeFile(t, filepath.Join(fixtureRoot, "input/desired/user/fixture-user/targets/"+targetID+"/artifacts/config"), "desired\n")
		stateBefore := treeSnapshot(t, repoRoot)

		report, err := RunTestRoundtrip(TestRoundtripOptions{RepoRoot: repoRoot, TargetID: targetID, Roundtrip: true, Fixture: "lifecycle"})
		requireAppAuthorError(t, err, CodeRecipeInvalid, 2)
		require.Contains(t, diagnosticCodes(report.Diagnostics), "writeSafety.lifecycle.actionRequired")
		require.Equal(t, stateBefore, treeSnapshot(t, repoRoot))
	})
}

func TestRoundtripRequiresExplicitModeAndStableJSONEnvelope(t *testing.T) {
	repoRoot, targetID := createRoundtripFileRecipe(t)

	report, err := RunTestRoundtrip(TestRoundtripOptions{RepoRoot: repoRoot, TargetID: targetID})
	requireAppAuthorError(t, err, CodeRoundtripModeRequired, 2)

	payload, jsonErr := JSONTestRoundtrip(report)
	require.NoError(t, jsonErr)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal([]byte(payload), &decoded))
	require.Equal(t, TestRoundtripSchema, decoded["schema"])
	require.Equal(t, TestRoundtripCommand, decoded["command"])
	require.Equal(t, TestRoundtripRunID, decoded["runId"])
	require.Equal(t, "blocked", decoded["summary"].(map[string]any)["status"])
	require.Equal(t, CodeRoundtripModeRequired, decoded["error"].(map[string]any)["code"])
	require.NotContains(t, payload, repoRoot)
}

func createRoundtripFileRecipe(t *testing.T) (string, string) {
	t.Helper()
	repoRoot := t.TempDir()
	targetID := "local-file-" + strings.ToLower(strings.ReplaceAll(t.Name(), "/", "-"))
	targetID = strings.ReplaceAll(targetID, "_", "-")
	if len(targetID) > 60 {
		targetID = "local-file-test"
	}
	_, err := RunCreate(CreateOptions{
		RepoRoot:     repoRoot,
		TargetID:     targetID,
		Template:     TemplateFile,
		FromPath:     ".config/demo/config.yaml",
		SettingID:    "config",
		SettingLabel: "Config file",
		ScopeDefault: "user",
		Lifecycle:    recipe.LifecycleAllowed,
	})
	require.NoError(t, err)
	return repoRoot, targetID
}

func roundtripFixtureRoot(repoRoot string, targetID string, fixtureName string) string {
	return filepath.Join(repoRoot, "recipes/local", targetID, "fixtures/roundtrip", fixtureName)
}

func writeRoundtripManifest(t *testing.T, fixtureRoot string, targetID string, fixtureName string, settings []string) {
	t.Helper()
	body := `schema: dotfiles-manager.v2.app.roundtrip-fixture
schemaVersion: 1
target: ` + targetID + `
name: ` + fixtureName + `
synthetic: true
settings:
`
	for _, setting := range settings {
		body += `  - ` + setting + "\n"
	}
	writeFile(t, filepath.Join(fixtureRoot, "manifest.yaml"), body)
}

func selectedValueSettings(value string) string {
	return `schema: dotfiles-manager.v2.desired-settings
schemaVersion: 1
values:
  user.email:
    intent: set
    kind: string
    value: ` + value + `
`
}

func xmlPlist(t *testing.T, value any) string {
	t.Helper()
	data, err := plist.MarshalIndent(value, plist.XMLFormat, "\t")
	require.NoError(t, err)
	return string(data)
}

func treeSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		slashed := filepath.ToSlash(rel)
		if entry.IsDir() {
			out[slashed] = "dir"
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			out[slashed] = info.Mode().String()
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[slashed] = "file:" + string(data)
		return nil
	})
	require.NoError(t, err)
	return out
}

func requireAppAuthorError(t *testing.T, err error, code string, exit int) {
	t.Helper()
	require.Error(t, err)
	var appErr *Error
	require.True(t, errors.As(err, &appErr), "err=%T %v", err, err)
	require.Equal(t, code, appErr.Code)
	require.Equal(t, exit, appErr.ExitCode())
}

func nativeExportRecipe(targetID string, markerPath string) string {
	return `schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: ` + targetID + `
displayName: Local Native Roundtrip
supportLevel: experimental
capability: export-only
settings:
  settings:
    label: Settings
    supportLevel: experimental
    capability: export-only
    artifactForm: native-export
    sensitivity: personal
    redaction: redacted-for-display
    lifecycle: allowed
    scopeDefault: user
    resource: settings-bundle
resources:
  settings-bundle:
    driver: native-export
    nativeOperation: export-settings
    capability: export-only
    sensitivity: personal
    redaction: redacted-for-display
    lifecycle: allowed
nativeOperations:
  export-settings:
    kind: export
    reviewed: true
    runner: command
    platforms:
      - darwin
    artifactForm: native
    diffMode: metadata-only
    lifecycle: allowed
    workingDirectory: temp
    timeoutSeconds: 5
    expectedExitCodes:
      - 0
    command:
      executable: /usr/bin/touch
      args:
        - literal: ` + markerPath + `
    stdin:
      mode: none
    stdout:
      mode: discard
    stderr:
      mode: discard
    outputs:
      bundle:
        root: artifact
        path: exports/settings.bundle
    redaction: metadata-only
`
}

func nativeExportImportRecipe(targetID string, exportMarker string, importMarker string) string {
	return `schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: ` + targetID + `
displayName: Local Native Import Roundtrip
supportLevel: experimental
capability: read-write
settings:
  settings:
    label: Settings
    supportLevel: experimental
    capability: read-write
    artifactForm: native-export
    sensitivity: personal
    redaction: redacted-for-display
    lifecycle: allowed
    scopeDefault: user
    resource: settings-bundle
resources:
  settings-bundle:
    driver: native-export
    nativeOperation: export-settings
    nativeImportOperation: import-settings
    nativeApply:
      backup: pre-apply-export
      verify: post-import-export-hash
    capability: read-write
    sensitivity: personal
    redaction: redacted-for-display
    lifecycle: allowed
nativeOperations:
  export-settings:
    kind: export
    reviewed: true
    runner: command
    platforms: [darwin]
    artifactForm: native
    diffMode: metadata-only
    lifecycle: allowed
    workingDirectory: temp
    timeoutSeconds: 5
    expectedExitCodes: [0]
    command:
      executable: /usr/bin/touch
      args:
        - literal: ` + exportMarker + `
    stdin:
      mode: none
    stdout:
      mode: discard
    stderr:
      mode: discard
    outputs:
      bundle:
        root: artifact
        path: exports/settings.bundle
    redaction: metadata-only
  import-settings:
    kind: import
    reviewed: true
    runner: command
    platforms: [darwin]
    artifactForm: native
    diffMode: metadata-only
    lifecycle: allowed
    workingDirectory: temp
    timeoutSeconds: 5
    expectedExitCodes: [0]
    command:
      executable: /usr/bin/touch
      args:
        - literal: ` + importMarker + `
    stdin:
      mode: none
    stdout:
      mode: discard
    stderr:
      mode: discard
    inputs:
      bundle:
        root: artifact
        path: exports/settings.bundle
    redaction: metadata-only
`
}

func fileTreeRecipe(targetID string) string {
	return `schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: ` + targetID + `
displayName: Local File Tree Roundtrip
supportLevel: experimental
capability: read-write
locations:
  home:
    default: "~"
settings:
  config:
    label: Config Tree
    supportLevel: experimental
    capability: read-write
    artifactForm: file-tree
    sensitivity: personal
    redaction: redacted-for-display
    lifecycle: allowed
    scopeDefault: user
    resource: config-tree
resources:
  config-tree:
    driver: file-tree
    location: home
    path: .config/demo
    capability: read-write
    sensitivity: personal
    redaction: redacted-for-display
    lifecycle: allowed
`
}

func lifecycleActionRecipe(targetID string) string {
	return `schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: ` + targetID + `
displayName: Local Lifecycle Roundtrip
supportLevel: experimental
capability: read-write
locations:
  home:
    default: "~"
lifecycleTargets:
  app:
    displayName: Demo App
    detect:
      kind: process-name
      names: [DemoApp]
    quit:
      kind: managed
settings:
  config:
    label: Config
    supportLevel: experimental
    capability: read-write
    artifactForm: file
    sensitivity: personal
    redaction: redacted-for-display
    lifecycle: quit-if-running
    lifecycleTarget: app
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
    lifecycle: quit-if-running
    lifecycleTarget: app
`
}
