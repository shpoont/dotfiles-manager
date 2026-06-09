package addtarget

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/shpoont/dotfiles-manager/internal/v2/resolution"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestRunAddsGitDefaultsAndPreservesLayerContent(t *testing.T) {
	t.Parallel()

	root := setupRepo(t, []string{"global"}, map[string]string{
		"global": `schema: dotfiles-manager.v2.profile-layer
schemaVersion: 1
ownerNote: keep-me
selections:
  other.app:
    settings:
      config:
        scope: shared
`,
	})

	report, err := Run(Options{RepoRoot: root, Target: "git", Yes: true, DiscoverOptions: testDiscoverOptions(t)})
	require.NoError(t, err)
	require.Equal(t, "changed", report.Summary.Status)
	require.Equal(t, 2, report.Summary.Written)
	require.Len(t, report.Add.Settings, 2)
	require.Empty(t, report.Add.Settings[0].Artifact)
	require.Empty(t, report.Add.Settings[1].Artifact)

	body := readFile(t, filepath.Join(root, "profiles", "layers", "global.yaml"))
	require.Contains(t, body, "ownerNote: keep-me")
	require.Contains(t, body, "other.app:")
	require.Contains(t, body, "user.email:")
	require.Contains(t, body, "user.name:")
	require.NotContains(t, body, "artifact:")

	cleanRoot := setupRepo(t, []string{"global"}, map[string]string{"global": emptyLayer()})
	_, err = Run(Options{RepoRoot: cleanRoot, Target: "git", Yes: true, DiscoverOptions: testDiscoverOptions(t)})
	require.NoError(t, err)
	resolved, err := resolution.Resolve(cleanRoot, resolution.ResolveOptions{UserID: "leon"})
	require.NoError(t, err)
	resolvedByRef := map[string]resolution.ResolvedSetting{}
	for _, setting := range resolved.Settings {
		resolvedByRef[setting.Ref()] = setting
	}
	require.Equal(t, filepath.Join("desired", "user", "leon", "targets", "git", "settings.yaml"), resolvedByRef["git:user.email"].DesiredRelPath)
	require.Equal(t, "desired://user/leon/targets/git/settings#user.name", resolvedByRef["git:user.name"].DesiredURI)
}

func TestRunWritesExplicitArtifactsForFileSettingsAndDryRunDoesNotMutate(t *testing.T) {
	t.Parallel()

	root := setupRepo(t, []string{"global"}, map[string]string{"global": emptyLayer()})
	layerPath := filepath.Join(root, "profiles", "layers", "global.yaml")
	before := readFile(t, layerPath)

	dryRun, err := Run(Options{RepoRoot: root, Target: "zsh", Settings: []string{"zshrc"}, DryRun: true, DiscoverOptions: testDiscoverOptions(t)})
	require.NoError(t, err)
	require.Equal(t, "changed", dryRun.Summary.Status)
	require.Equal(t, 1, dryRun.Summary.Planned)
	require.Equal(t, 0, dryRun.Summary.Written)
	require.Equal(t, "artifacts/zshrc", dryRun.Add.Settings[0].Artifact)
	require.Equal(t, before, readFile(t, layerPath))

	report, err := Run(Options{RepoRoot: root, Target: "zsh", Settings: []string{"zshrc"}, DiscoverOptions: testDiscoverOptions(t)})
	require.NoError(t, err)
	require.Equal(t, 1, report.Summary.Written)
	body := readFile(t, layerPath)
	require.Contains(t, body, "zshrc:")
	require.Contains(t, body, "artifact: artifacts/zshrc")

	resolved, err := resolution.Resolve(root, resolution.ResolveOptions{UserID: "leon"})
	require.NoError(t, err)
	var found resolution.ResolvedSetting
	for _, setting := range resolved.Settings {
		if setting.Ref() == "zsh:zshrc" {
			found = setting
		}
	}
	require.Equal(t, filepath.Join("desired", "user", "leon", "targets", "zsh", "artifacts", "zshrc"), found.DesiredRelPath)
}

func TestRunChecksConflictsAcrossFullActiveStack(t *testing.T) {
	t.Parallel()

	root := setupRepo(t, []string{"base", "local"}, map[string]string{
		"base": `schema: dotfiles-manager.v2.profile-layer
schemaVersion: 1
selections:
  git:
    settings:
      user.email:
        scope: user
`,
		"local": emptyLayer(),
	})

	unchanged, err := Run(Options{RepoRoot: root, Target: "git", Settings: []string{"user.email"}, Scope: "user", ProfileLayer: "local", DiscoverOptions: testDiscoverOptions(t)})
	require.NoError(t, err)
	require.Equal(t, "ok", unchanged.Summary.Status)
	require.Equal(t, 1, unchanged.Summary.Unchanged)
	require.Equal(t, "base", unchanged.Add.Settings[0].SourceLayer)
	require.NotContains(t, readFile(t, filepath.Join(root, "profiles", "layers", "local.yaml")), "git:")

	conflict, err := Run(Options{RepoRoot: root, Target: "git", Settings: []string{"user.email"}, Scope: "machine", ProfileLayer: "local", DiscoverOptions: testDiscoverOptions(t)})
	require.Error(t, err)
	require.Equal(t, CodeSelectionConflict, conflict.Error.Code)
	require.Equal(t, "blocked", conflict.Summary.Status)
	require.Contains(t, err.Error(), "different scope or artifact")
}

func TestRunRequiresProfileChoiceForMultiLayerYesAndJSONPaths(t *testing.T) {
	t.Parallel()

	root := setupRepo(t, []string{"global", "local"}, map[string]string{"global": emptyLayer(), "local": emptyLayer()})

	report, err := Run(Options{RepoRoot: root, Target: "ssh", Yes: true, DiscoverOptions: testDiscoverOptions(t)})
	require.Error(t, err)
	require.Equal(t, CodeChoiceRequired, report.Error.Code)
	require.Equal(t, "profile", report.Add.MissingChoices[0].Kind)

	report, err = Run(Options{RepoRoot: root, Target: "ssh", JSONMode: true, DiscoverOptions: testDiscoverOptions(t)})
	require.Error(t, err)
	require.Equal(t, CodeChoiceRequired, report.Error.Code)
	require.Len(t, report.Add.MissingChoices, 1)
}

func TestRunRejectsSymlinkedProfileLayer(t *testing.T) {
	t.Parallel()

	root := setupRepo(t, []string{"global"}, map[string]string{"global": emptyLayer()})
	layerPath := filepath.Join(root, "profiles", "layers", "global.yaml")
	targetPath := filepath.Join(root, "profiles", "layers", "real-global.yaml")
	require.NoError(t, os.Rename(layerPath, targetPath))
	require.NoError(t, os.Symlink(targetPath, layerPath))

	report, err := Run(Options{RepoRoot: root, Target: "git", Yes: true, DiscoverOptions: testDiscoverOptions(t)})
	require.Error(t, err)
	require.Equal(t, CodeRepoInvalid, report.Error.Code)
	require.Contains(t, report.Error.Message, "symlink")
}

func TestRunRejectsSymlinkedProfileLayerParents(t *testing.T) {
	t.Parallel()

	t.Run("layers directory symlink", func(t *testing.T) {
		root := setupRepo(t, []string{"global"}, map[string]string{"global": emptyLayer()})
		layersPath := filepath.Join(root, "profiles", "layers")
		outsideLayers := filepath.Join(t.TempDir(), "layers")
		require.NoError(t, os.Rename(layersPath, outsideLayers))
		require.NoError(t, os.Symlink(outsideLayers, layersPath))

		report, err := Run(Options{RepoRoot: root, Target: "git", Yes: true, DiscoverOptions: testDiscoverOptions(t)})
		require.Error(t, err)
		require.Equal(t, CodeRepoInvalid, report.Error.Code)
		require.Contains(t, report.Error.Message, "symlinked path component rejected")
	})

	t.Run("nested layer parent symlink", func(t *testing.T) {
		root := setupRepo(t, []string{"os/macos"}, map[string]string{"os/macos": emptyLayer()})
		nestedPath := filepath.Join(root, "profiles", "layers", "os")
		outsideNested := filepath.Join(t.TempDir(), "os")
		require.NoError(t, os.Rename(nestedPath, outsideNested))
		require.NoError(t, os.Symlink(outsideNested, nestedPath))

		report, err := Run(Options{RepoRoot: root, Target: "git", Yes: true, DiscoverOptions: testDiscoverOptions(t)})
		require.Error(t, err)
		require.Equal(t, CodeRepoInvalid, report.Error.Code)
		require.Contains(t, report.Error.Message, "symlinked path component rejected")
	})
}

func TestRunInteractivePromptsAndValidationBranches(t *testing.T) {
	t.Parallel()

	t.Run("interactive profile and settings prompts", func(t *testing.T) {
		root := setupRepo(t, []string{"global", "local"}, map[string]string{"global": emptyLayer(), "local": emptyLayer()})
		report, err := Run(Options{
			RepoRoot:        root,
			Target:          "git",
			Input:           strings.NewReader("2\nuser.email\n"),
			PromptOutput:    &strings.Builder{},
			DiscoverOptions: testDiscoverOptions(t),
		})
		require.NoError(t, err)
		require.Equal(t, "local", report.Add.DestinationProfileLayer)
		require.Len(t, report.Add.Settings, 1)
		require.Contains(t, readFile(t, filepath.Join(root, "profiles", "layers", "local.yaml")), "user.email:")
	})

	t.Run("json requires settings without yes", func(t *testing.T) {
		root := setupRepo(t, []string{"global"}, map[string]string{"global": emptyLayer()})
		report, err := Run(Options{RepoRoot: root, Target: "git", JSONMode: true, DiscoverOptions: testDiscoverOptions(t)})
		require.Error(t, err)
		require.Equal(t, CodeChoiceRequired, report.Error.Code)
		require.Equal(t, "settings", report.Add.MissingChoices[0].Kind)
	})

	t.Run("invalid profile", func(t *testing.T) {
		root := setupRepo(t, []string{"global"}, map[string]string{"global": emptyLayer()})
		report, err := Run(Options{RepoRoot: root, Target: "git", ProfileLayer: "../escape", Yes: true, DiscoverOptions: testDiscoverOptions(t)})
		require.Error(t, err)
		require.Equal(t, CodeProfileInvalid, report.Error.Code)
	})

	t.Run("profile not in active stack", func(t *testing.T) {
		root := setupRepo(t, []string{"global"}, map[string]string{"global": emptyLayer()})
		report, err := Run(Options{RepoRoot: root, Target: "git", ProfileLayer: "local", Yes: true, DiscoverOptions: testDiscoverOptions(t)})
		require.Error(t, err)
		require.Equal(t, CodeProfileInvalid, report.Error.Code)
	})

	t.Run("invalid setting", func(t *testing.T) {
		root := setupRepo(t, []string{"global"}, map[string]string{"global": emptyLayer()})
		report, err := Run(Options{RepoRoot: root, Target: "git", Settings: []string{"credential.helper"}, DiscoverOptions: testDiscoverOptions(t)})
		require.Error(t, err)
		require.Equal(t, CodeSettingInvalid, report.Error.Code)
	})

	t.Run("invalid scope", func(t *testing.T) {
		root := setupRepo(t, []string{"global"}, map[string]string{"global": emptyLayer()})
		report, err := Run(Options{RepoRoot: root, Target: "git", Settings: []string{"user.email"}, Scope: "planet", DiscoverOptions: testDiscoverOptions(t)})
		require.Error(t, err)
		require.Equal(t, CodeScopeInvalid, report.Error.Code)
	})

	t.Run("custom files unsupported", func(t *testing.T) {
		root := setupRepo(t, []string{"global"}, map[string]string{"global": emptyLayer()})
		report, err := Run(Options{RepoRoot: root, Target: "custom.files", Yes: true, DiscoverOptions: testDiscoverOptions(t)})
		require.Error(t, err)
		require.Equal(t, CodeTargetUnsupported, report.Error.Code)
	})

	t.Run("unsupported platform blocks", func(t *testing.T) {
		root := setupRepo(t, []string{"global"}, map[string]string{"global": emptyLayer()})
		discover := testDiscoverOptions(t)
		discover.GOOS = "windows"
		report, err := Run(Options{RepoRoot: root, Target: "nvim", Yes: true, DiscoverOptions: discover})
		require.Error(t, err)
		require.Equal(t, CodePlatformUnsupported, report.Error.Code)
		require.Equal(t, 5, err.(*Error).ExitCode())
	})
}

func TestRenderersPromptsAndHelpers(t *testing.T) {
	t.Parallel()

	nilJSON, err := JSON(nil)
	require.NoError(t, err)
	require.Contains(t, nilJSON, `"status": "error"`)
	_, err = JSON(&Report{Error: &ErrorObject{Details: map[string]any{"bad": func() {}}}})
	require.Error(t, err)
	require.Contains(t, Text(nil), "summary status=error")
	require.Equal(t, "", (*Error)(nil).Error())
	require.Equal(t, 1, (*Error)(nil).ExitCode())
	require.Equal(t, 1, (&Error{}).ExitCode())
	require.Equal(t, 4, (&Error{Exit: 4}).ExitCode())

	report := baseReport(true)
	report.Add.Target = AddTarget{ID: "git"}
	report.Add.ActiveProfileStack = "default"
	report.Add.ProfileStack = []string{"global"}
	report.Add.DestinationProfileLayer = "global"
	report.Add.Discovery = &AddDiscovery{State: "installed", BinaryState: "installed", ConfigState: "config-missing", PlatformState: "unknown"}
	report.Add.MissingChoices = []MissingChoice{{Kind: "settings", Allowed: []string{"user.email"}, Recommended: []string{"user.email"}}}
	report.Add.Settings = []SettingChoice{{Ref: "git:user.email", ID: "user.email", Scope: "machine-user", ScopeLabel: ScopeLabel("machine-user"), Action: "add", SourceLayer: "global"}}
	report.Diagnostics = []Diagnostic{{Code: "z", Severity: SeverityWarning, Message: "last"}, {Code: "a", Severity: SeverityInfo, Message: "first"}}
	report.Error = &ErrorObject{Code: "sample", Message: "sample error"}
	finish(report)
	text := Text(report)
	require.Contains(t, text, "MODE: DRY RUN")
	require.Contains(t, text, "discovery: installed")
	require.Contains(t, text, "missing choice: settings")
	require.Contains(t, text, "git:user.email")
	require.Contains(t, text, "error[sample]")
	payload, err := JSON(report)
	require.NoError(t, err)
	require.Contains(t, payload, `"command": "add"`)

	require.Equal(t, "Everyone using this repo", ScopeLabel("shared"))
	require.Equal(t, "Me on all my machines", ScopeLabel("user"))
	require.Equal(t, "This machine", ScopeLabel("machine"))
	require.Equal(t, "custom", ScopeLabel("custom"))
	require.Equal(t, "artifacts/native-preferences", canonicalArtifactForSetting(recipe.ExplainSetting{ID: "native-preferences", ArtifactForm: "native"}))

	settings := map[string]recipe.ExplainSetting{"manual": {ID: "manual", Ref: "tool:manual", Label: "Manual", Capability: "read-write", SupportLevel: "experimental", ArtifactForm: "scalar"}}
	choices, missing, err := chooseScopesAndArtifacts("tool", Options{Input: strings.NewReader("machine-user\n"), PromptOutput: io.Discard}, []string{"manual"}, settings, true)
	require.NoError(t, err)
	require.Empty(t, missing)
	require.Equal(t, "machine-user", choices[0].Scope)
	_, missing, err = chooseScopesAndArtifacts("tool", Options{}, []string{"manual"}, settings, false)
	require.Error(t, err)
	require.Equal(t, "scope", missing[0].Kind)

	line, err := readLine(strings.NewReader("value"))
	require.NoError(t, err)
	require.Equal(t, "value", line)
}

func TestMalformedRepoAndSelectionBranches(t *testing.T) {
	t.Parallel()

	t.Run("missing repo root", func(t *testing.T) {
		report, err := Run(Options{RepoRoot: filepath.Join(t.TempDir(), "missing"), Target: "git", Yes: true, DiscoverOptions: testDiscoverOptions(t)})
		require.Error(t, err)
		require.Equal(t, CodeRepoInvalid, report.Error.Code)
	})

	t.Run("malformed selections", func(t *testing.T) {
		root := setupRepo(t, []string{"global"}, map[string]string{"global": "schema: dotfiles-manager.v2.profile-layer\nschemaVersion: 1\nselections: nope\n"})
		report, err := Run(Options{RepoRoot: root, Target: "git", Settings: []string{"user.email"}, DiscoverOptions: testDiscoverOptions(t)})
		require.Error(t, err)
		require.Equal(t, CodeSelectionConflict, report.Error.Code)
	})

	t.Run("non scalar existing artifact", func(t *testing.T) {
		root := setupRepo(t, []string{"global"}, map[string]string{"global": `schema: dotfiles-manager.v2.profile-layer
schemaVersion: 1
selections:
  git:
    settings:
      user.email:
        scope: user
        artifact: [bad]
`})
		report, err := Run(Options{RepoRoot: root, Target: "git", Settings: []string{"user.email"}, Scope: "user", DiscoverOptions: testDiscoverOptions(t)})
		require.Error(t, err)
		require.Equal(t, CodeSelectionConflict, report.Error.Code)
	})
}

func TestLowLevelValidationAndYAMLHelperBranches(t *testing.T) {
	t.Parallel()

	t.Run("schema and yaml validation", func(t *testing.T) {
		dir := t.TempDir()
		_, _, err := loadYAMLDocument(filepath.Join(dir, "missing.yaml"))
		require.Error(t, err)

		emptyPath := filepath.Join(dir, "empty.yaml")
		writeFile(t, emptyPath, "")
		_, _, err = loadYAMLDocument(emptyPath)
		require.Error(t, err)

		badYAML := filepath.Join(dir, "bad.yaml")
		writeFile(t, badYAML, "schema: [")
		_, _, err = loadYAMLDocument(badYAML)
		require.Error(t, err)

		doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "schema"}, {Kind: yaml.ScalarNode, Value: "wrong"},
			{Kind: yaml.ScalarNode, Value: "schemaVersion"}, {Kind: yaml.ScalarNode, Value: "1"},
		}}}}
		require.Error(t, validateSchemaNode(doc, "thing", "expected"))
		mappingValue(documentMapping(doc), "schema").Value = "expected"
		mappingValue(documentMapping(doc), "schemaVersion").Value = "nope"
		require.Error(t, validateSchemaNode(doc, "thing", "expected"))

		_, err = requiredScalar(&yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}, "missing")
		require.Error(t, err)
		mappingValue(documentMapping(doc), "schemaVersion").Kind = yaml.SequenceNode
		_, err = requiredScalar(doc, "schemaVersion")
		require.Error(t, err)
		_, err = lstatRegularNoSymlink(filepath.Join(dir, "missing-lstat.yaml"))
		require.Error(t, err)
	})

	t.Run("profile path validation", func(t *testing.T) {
		for _, value := range []string{" global", "/abs", `bad\\path`, ".", "../x", "bad//path", "Bad"} {
			_, err := validateProfilePathID("profile layer", value)
			require.Error(t, err, value)
		}
		got, err := validateProfilePathID("profile layer", "os/macos.local")
		require.NoError(t, err)
		require.Equal(t, "os/macos.local", got)
	})

	t.Run("repo-owned path containment and symlink helpers", func(t *testing.T) {
		base := t.TempDir()
		child := filepath.Join(base, "profiles", "layers", "global.yaml")
		writeFile(t, child, emptyLayer())

		require.NoError(t, ensureInside(base, child))
		require.Error(t, ensureInside(filepath.Join(base, "profiles", "layers"), filepath.Join(base, "profiles", "stacks", "default.yaml")))
		require.NoError(t, ensureNoSymlinkComponentsBetween(base, base))
		require.NoError(t, ensureNoSymlinkComponentsBetween(base, child))
		require.Error(t, ensureNoSymlinkComponentsBetween(filepath.Join(base, "profiles", "layers"), filepath.Join(base, "profiles", "stacks", "default.yaml")))
		require.Error(t, ensureNoSymlinkComponentsBetween(base, filepath.Join(base, "missing", "file.yaml")))

		link := filepath.Join(base, "profiles", "linked-layers")
		require.NoError(t, os.Symlink(filepath.Join(base, "profiles", "layers"), link))
		require.Error(t, ensureNoSymlinkComponentsBetween(base, filepath.Join(link, "global.yaml")))
	})

	t.Run("sequence and mapping helper validation", func(t *testing.T) {
		doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "profileStack"}, {Kind: yaml.ScalarNode, Value: "global"},
		}}}}
		_, err := requiredStringSequence(doc, "profileStack")
		require.Error(t, err)
		mappingValue(documentMapping(doc), "profileStack").Kind = yaml.SequenceNode
		mappingValue(documentMapping(doc), "profileStack").Content = []*yaml.Node{{Kind: yaml.MappingNode}}
		_, err = requiredStringSequence(doc, "profileStack")
		require.Error(t, err)

		mapping := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "existing"}, {Kind: yaml.ScalarNode, Value: "scalar"},
		}}
		existing := ensureMapping(mapping, "existing")
		require.Equal(t, yaml.MappingNode, existing.Kind)
		setScalar(existing, "scope", "user")
		setScalar(existing, "scope", "machine")
		require.Equal(t, "machine", mappingValue(existing, "scope").Value)
		require.Same(t, mapping, documentMapping(mapping))
	})

	t.Run("selection node malformed shapes", func(t *testing.T) {
		doc := docWithSelectionsValue(&yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "git"}, {Kind: yaml.ScalarNode, Value: "bad"},
		}})
		_, err := selectedSettingNode(doc, "git", "user.email")
		require.Error(t, err)

		doc = docWithSelectionsValue(&yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "git"}, {Kind: yaml.MappingNode, Content: []*yaml.Node{
				{Kind: yaml.ScalarNode, Value: "settings"}, {Kind: yaml.ScalarNode, Value: "bad"},
			}},
		}})
		_, err = selectedSettingNode(doc, "git", "user.email")
		require.Error(t, err)

		doc = docWithSelectionsValue(&yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "git"}, {Kind: yaml.MappingNode, Content: []*yaml.Node{
				{Kind: yaml.ScalarNode, Value: "settings"}, {Kind: yaml.MappingNode, Content: []*yaml.Node{
					{Kind: yaml.ScalarNode, Value: "user.email"}, {Kind: yaml.ScalarNode, Value: "bad"},
				}},
			}},
		}})
		_, err = selectedSettingNode(doc, "git", "user.email")
		require.Error(t, err)
	})

	t.Run("choice helpers and error detail branches", func(t *testing.T) {
		_, _, err := chooseSettings("x", Options{Yes: true}, nil, nil, nil, false)
		require.Error(t, err)

		ids, err := normalizeSettingInputs("git", []string{"git:user.email,user.name"}, map[string]recipe.ExplainSetting{
			"user.email": {ID: "user.email", Capability: "read-write", SupportLevel: "experimental"},
			"user.name":  {ID: "user.name", Capability: "read-write", SupportLevel: "experimental"},
		})
		require.NoError(t, err)
		require.Equal(t, []string{"user.email", "user.name"}, ids)
		_, err = normalizeSettingInputs("git", []string{","}, map[string]recipe.ExplainSetting{})
		require.Error(t, err)

		addErr := &Error{Code: "x", Details: map[string]any{"x": "y"}, Exit: 7}
		require.Equal(t, map[string]any{"x": "y"}, errorDetails(addErr))
		require.Equal(t, 7, exitCode(addErr, 1))
		explainErr := &recipe.ExplainError{Code: "e", Details: map[string]any{"e": "d"}, Exit: 2}
		require.Equal(t, map[string]any{"e": "d"}, errorDetails(explainErr))
		require.Equal(t, 2, exitCode(explainErr, 1))
		require.Nil(t, errorDetails(os.ErrNotExist))
		require.Equal(t, 9, exitCode(os.ErrNotExist, 9))
	})
}

func TestAdditionalLoadRepoPromptAndWriteErrorBranches(t *testing.T) {
	t.Parallel()

	t.Run("active stack and stack layer validation", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, filepath.Join(root, resolution.RootConfigFile), "schema: dotfiles-manager.v2.root-config\nschemaVersion: 1\nactiveProfileStack: ../bad\n")
		_, err := loadRepo(root)
		require.Error(t, err)

		root = t.TempDir()
		writeFile(t, filepath.Join(root, resolution.RootConfigFile), "schema: dotfiles-manager.v2.root-config\nschemaVersion: 1\nactiveProfileStack: default\n")
		writeFile(t, filepath.Join(root, "profiles", "stacks", "default.yaml"), "schema: dotfiles-manager.v2.profile-stack\nschemaVersion: 1\nprofileStack: []\n")
		_, err = loadRepo(root)
		require.Error(t, err)

		root = t.TempDir()
		writeFile(t, filepath.Join(root, resolution.RootConfigFile), "schema: dotfiles-manager.v2.root-config\nschemaVersion: 1\nactiveProfileStack: default\n")
		writeFile(t, filepath.Join(root, "profiles", "stacks", "default.yaml"), "schema: dotfiles-manager.v2.profile-stack\nschemaVersion: 1\nprofileStack:\n  - ../bad\n")
		_, err = loadRepo(root)
		require.Error(t, err)
	})

	t.Run("layer directory rejected", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, filepath.Join(root, resolution.RootConfigFile), "schema: dotfiles-manager.v2.root-config\nschemaVersion: 1\nactiveProfileStack: default\n")
		writeFile(t, filepath.Join(root, "profiles", "stacks", "default.yaml"), "schema: dotfiles-manager.v2.profile-stack\nschemaVersion: 1\nprofileStack:\n  - global\n")
		require.NoError(t, os.MkdirAll(filepath.Join(root, "profiles", "layers", "global.yaml"), 0o755))
		_, err := loadRepo(root)
		require.Error(t, err)
	})

	t.Run("prompt invalid inputs", func(t *testing.T) {
		_, err := promptProfile(Options{Input: strings.NewReader("bogus\n"), PromptOutput: io.Discard}, []string{"global"})
		require.Error(t, err)
		_, err = promptProfile(Options{Input: strings.NewReader(""), PromptOutput: io.Discard}, []string{"global"})
		require.Error(t, err)

		_, err = promptSettings(Options{Input: strings.NewReader("bad\n"), PromptOutput: io.Discard}, "git", []recipe.ExplainSetting{{ID: "user.email", Label: "Email", DefaultScope: "user"}}, []string{"user.email"}, map[string]recipe.ExplainSetting{"user.email": {ID: "user.email", Capability: "read-write", SupportLevel: "experimental"}})
		require.Error(t, err)
		_, err = promptSettings(Options{Input: strings.NewReader(""), PromptOutput: io.Discard}, "git", nil, nil, nil)
		require.Error(t, err)

		_, err = promptScope(Options{Input: strings.NewReader("bad\n"), PromptOutput: io.Discard}, "git:user.email")
		require.Error(t, err)
		_, err = promptScope(Options{Input: strings.NewReader(""), PromptOutput: io.Discard}, "git:user.email")
		require.Error(t, err)

		_, err = readLine(strings.NewReader(""))
		require.Error(t, err)
	})

	t.Run("atomic write temp creation failure", func(t *testing.T) {
		infoPath := filepath.Join(t.TempDir(), "layer.yaml")
		writeFile(t, infoPath, emptyLayer())
		info, err := os.Lstat(infoPath)
		require.NoError(t, err)
		err = atomicWriteYAML(t.TempDir(), filepath.Join(t.TempDir(), "missing", "layer.yaml"), info, &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}})
		require.Error(t, err)
	})

	t.Run("atomic write rejects temp outside repo root", func(t *testing.T) {
		dir := t.TempDir()
		targetPath := filepath.Join(dir, "layer.yaml")
		writeFile(t, targetPath, emptyLayer())
		info, err := os.Lstat(targetPath)
		require.NoError(t, err)
		err = atomicWriteYAML(filepath.Join(t.TempDir(), "different-root"), targetPath, info, &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}})
		require.Error(t, err)
	})

	t.Run("patch layer revalidates parent symlink before write", func(t *testing.T) {
		root := setupRepo(t, []string{"global"}, map[string]string{"global": emptyLayer()})
		repo, err := loadRepo(root)
		require.NoError(t, err)
		layersPath := filepath.Join(root, "profiles", "layers")
		outsideLayers := filepath.Join(t.TempDir(), "layers")
		require.NoError(t, os.Rename(layersPath, outsideLayers))
		require.NoError(t, os.Symlink(outsideLayers, layersPath))

		err = patchLayer(repo.layers[0], "git", []SettingChoice{{ID: "user.email", Scope: "user", Action: "add"}})
		require.Error(t, err)
		require.Contains(t, err.Error(), "symlinked path component rejected")
	})
}

func TestRemainingBranchEdges(t *testing.T) {
	t.Parallel()

	t.Run("run target and discover errors", func(t *testing.T) {
		root := setupRepo(t, []string{"global"}, map[string]string{"global": emptyLayer()})
		report, err := Run(Options{RepoRoot: root, Target: "nope", Yes: true, DiscoverOptions: testDiscoverOptions(t)})
		require.Error(t, err)
		require.Equal(t, CodeTargetInvalid, report.Error.Code)

		_, err = discoverTarget("nope", recipe.DiscoverOptions{})
		require.Error(t, err)
	})

	t.Run("render empty settings and artifact", func(t *testing.T) {
		report := baseReport(false)
		report.Add.Target = AddTarget{ID: "ssh"}
		report.Add.Settings = []SettingChoice{{Ref: "ssh:config", ID: "config", Scope: "user", ScopeLabel: ScopeLabel("user"), Artifact: "artifacts/config", Action: "add"}}
		finish(report)
		require.Contains(t, Text(report), "artifact=artifacts/config")

		empty := baseReport(false)
		empty.Add.Target = AddTarget{ID: "empty"}
		require.Contains(t, Text(empty), "settings: none")
		blocked := baseReport(false)
		blocked.Summary.Blocked = 1
		finish(blocked)
		require.Equal(t, "blocked", blocked.Summary.Status)
		failed := baseReport(false)
		failed.Summary.Failed = 1
		finish(failed)
		require.Equal(t, "error", failed.Summary.Status)
		_, err := fail(baseReport(false), "x", "x", 0, nil)
		require.Equal(t, 1, err.(*Error).ExitCode())
		_, err = failWithMissing(baseReport(false), os.ErrNotExist, nil)
		require.Equal(t, CodeChoiceRequired, err.(*Error).Code)
	})

	t.Run("selection helper nil branches", func(t *testing.T) {
		emptyDoc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
		node, err := selectedSettingNode(emptyDoc, "git", "user.email")
		require.NoError(t, err)
		require.Nil(t, node)

		doc := docWithSelectionsValue(&yaml.Node{Kind: yaml.MappingNode})
		node, err = selectedSettingNode(doc, "git", "user.email")
		require.NoError(t, err)
		require.Nil(t, node)

		doc = docWithSelectionsValue(&yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "git"}, {Kind: yaml.MappingNode},
		}})
		node, err = selectedSettingNode(doc, "git", "user.email")
		require.NoError(t, err)
		require.Nil(t, node)

		doc = docWithSelectionsValue(&yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "git"}, {Kind: yaml.MappingNode, Content: []*yaml.Node{
				{Kind: yaml.ScalarNode, Value: "settings"}, {Kind: yaml.MappingNode},
			}},
		}})
		node, err = selectedSettingNode(doc, "git", "user.email")
		require.NoError(t, err)
		require.Nil(t, node)

		root := setupRepo(t, []string{"global"}, map[string]string{"global": `schema: dotfiles-manager.v2.profile-layer
schemaVersion: 1
selections:
  git:
    settings:
      user.email:
        artifact: settings.yaml#user.email
`})
		repo, err := loadRepo(root)
		require.NoError(t, err)
		_, err = findExistingSelections(repo.layers, "git", "user.email")
		require.Error(t, err)
		require.Equal(t, "settings.yaml#user.email", artifactKey("user.email", "settings.yaml#user.email"))
	})

	t.Run("choice direct helpers", func(t *testing.T) {
		settings := []recipe.ExplainSetting{
			{},
			{ID: "blocked", Capability: "read-write", SupportLevel: "blocked", DefaultScope: "user"},
			{ID: "read", Capability: "read-only", SupportLevel: "experimental", DefaultScope: "user"},
		}
		_, selectable, recommended := selectableSettings(settings)
		require.Empty(t, selectable)
		require.Empty(t, recommended)

		got, err := promptProfile(Options{Input: strings.NewReader("global\n"), PromptOutput: io.Discard}, []string{"global"})
		require.NoError(t, err)
		require.Equal(t, "global", got)
		ids, err := promptSettings(Options{Input: strings.NewReader("\n"), PromptOutput: io.Discard}, "git", []recipe.ExplainSetting{{ID: "user.email", Label: "Email", DefaultScope: "user"}}, []string{"user.email"}, map[string]recipe.ExplainSetting{"user.email": {ID: "user.email", Capability: "read-write", SupportLevel: "experimental"}})
		require.NoError(t, err)
		require.Equal(t, []string{"user.email"}, ids)
	})
}

func setupRepo(t *testing.T, stack []string, layers map[string]string) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, resolution.RootConfigFile), "schema: dotfiles-manager.v2.root-config\nschemaVersion: 1\nactiveProfileStack: default\n")
	stackBody := "schema: dotfiles-manager.v2.profile-stack\nschemaVersion: 1\nprofileStack:\n"
	for _, layer := range stack {
		stackBody += "  - " + layer + "\n"
	}
	writeFile(t, filepath.Join(root, "profiles", "stacks", "default.yaml"), stackBody)
	for _, layer := range stack {
		body, ok := layers[layer]
		if !ok {
			body = emptyLayer()
		}
		writeFile(t, filepath.Join(root, "profiles", "layers", filepath.FromSlash(layer)+".yaml"), body)
	}
	return root
}

func docWithSelectionsValue(selections *yaml.Node) *yaml.Node {
	return &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Value: "selections"}, selections,
	}}}}
}

func emptyLayer() string {
	return "schema: dotfiles-manager.v2.profile-layer\nschemaVersion: 1\nselections: {}\n"
}

func testDiscoverOptions(t *testing.T) recipe.DiscoverOptions {
	t.Helper()
	liveRoot := t.TempDir()
	return recipe.DiscoverOptions{
		LocationRoots: map[string]string{"home": liveRoot, "config": liveRoot},
		PathEnv:       "",
	}
}

func writeFile(t *testing.T, path string, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return strings.ReplaceAll(string(data), "\r\n", "\n")
}
