package listcmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunListsManagedSettingsWithIdentityAndLocationMetadata(t *testing.T) {
	repoRoot := setupListRepo(t, []string{"global"}, map[string]string{"global": `schema: dotfiles-manager.v2.profile-layer
schemaVersion: 1
selections:
  git:
    settings:
      user.email:
        scope: user
`})
	stateRoot := setupListIdentity(t, "mbp", "leon", "leon")

	report, err := Run(Options{RepoRoot: repoRoot, StateRoot: stateRoot, LocalAccountName: "leon"})
	require.NoError(t, err)
	require.Equal(t, "ok", report.Summary.Status)
	require.Equal(t, 1, report.Summary.Targets)
	require.Equal(t, 1, report.Summary.Settings)
	item := report.List.Settings[0]
	require.Equal(t, "git:user.email", item.Ref)
	require.Equal(t, "Git", item.Target.DisplayName)
	require.Equal(t, "User email", item.Setting.Label)
	require.True(t, item.Subject.Resolved)
	require.Equal(t, "leon", item.Subject.ID)
	require.Equal(t, "desired://user/leon/targets/git/settings#user.email", item.DesiredURI)
	require.Equal(t, filepath.Join("desired", "user", "leon", "targets", "git", "settings.yaml"), item.DesiredRelPath)
	require.Equal(t, "user-email", item.Resource.ID)
	require.Equal(t, "ini-file", item.Resource.DriverID)
	require.Equal(t, "home", item.Resource.LocationID)
	require.Equal(t, ".gitconfig", item.Resource.Path)
	require.Equal(t, "[user] email", item.SelectorSummary)
	require.Contains(t, strings.Join(item.NextActions, "\n"), "dotfiles-manager sync git:user.email")

	text := Text(report)
	require.Contains(t, text, "location=home:.gitconfig")
	require.Contains(t, text, "desired=desired://user/leon/targets/git/settings#user.email")
	require.NotContains(t, text, "resource group")

	payload, err := JSON(report)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal([]byte(payload), &decoded))
	require.Equal(t, Schema, decoded["schema"])
}

func TestListHappyPathTextAndJSONSnapshots(t *testing.T) {
	repoRoot := setupListRepo(t, []string{"global"}, map[string]string{"global": `schema: dotfiles-manager.v2.profile-layer
schemaVersion: 1
selections:
  git:
    settings:
      user.email:
        scope: user
`})

	report, err := Run(Options{RepoRoot: repoRoot, StateRoot: filepath.Join(t.TempDir(), "state"), UserID: "leon"})
	require.NoError(t, err)
	require.Equal(t, `list
profile stack: default [global]
managed settings:
  git:user.email User email
    scope=user (Me on all my machines) subject=leon resolved=true sourceLayer=global
    resource=user-email driver=ini-file location=home:.gitconfig selector=[user] email
    desired=desired://user/leon/targets/git/settings#user.email
    next: dotfiles-manager status git:user.email | dotfiles-manager save --dry-run git:user.email | dotfiles-manager sync git:user.email
summary status=ok targets=1 settings=1 unresolved=0 blocked=0 failed=0`, Text(report))

	payload, err := JSON(report)
	require.NoError(t, err)
	require.JSONEq(t, `{
  "schema": "dotfiles-manager.v2.list",
  "schemaVersion": 1,
  "command": "list",
  "runId": "list-managed",
  "summary": {"status": "ok", "targets": 1, "settings": 1, "unresolved": 0, "blocked": 0, "failed": 0},
  "list": {
    "activeProfileStack": "default",
    "profileStack": ["global"],
    "settings": [{
      "ref": "git:user.email",
      "target": {"id": "git", "displayName": "Git", "recipeRef": "recipe://bundled/git"},
      "setting": {"id": "user.email", "label": "User email"},
      "scope": "user",
      "scopeLabel": "Me on all my machines",
      "subject": {"resolved": true, "id": "leon", "missing": []},
      "sourceLayer": "global",
      "artifactForm": "scalar",
      "desiredUri": "desired://user/leon/targets/git/settings#user.email",
      "desiredRelPath": "desired/user/leon/targets/git/settings.yaml",
      "resource": {"id": "user-email", "driverId": "ini-file", "locationId": "home", "path": ".gitconfig"},
      "selectorSummary": "[user] email",
      "nextActions": [
        "dotfiles-manager status git:user.email",
        "dotfiles-manager save --dry-run git:user.email",
        "dotfiles-manager sync git:user.email"
      ]
    }]
  },
  "diagnostics": [],
  "error": null
}`, payload)
}

func TestRunKeepsReadOnlyListPartialWhenIdentityMissing(t *testing.T) {
	repoRoot := setupListRepo(t, []string{"global"}, map[string]string{"global": `schema: dotfiles-manager.v2.profile-layer
schemaVersion: 1
selections:
  git:
    settings:
      user.email:
        scope: user
`})
	stateRoot := filepath.Join(t.TempDir(), "state")

	report, err := Run(Options{RepoRoot: repoRoot, StateRoot: stateRoot, LocalAccountName: "leon"})
	require.NoError(t, err)
	require.Equal(t, "partial", report.Summary.Status)
	require.Equal(t, 1, report.Summary.Unresolved)
	require.False(t, report.List.Settings[0].Subject.Resolved)
	require.Equal(t, []string{"user-id"}, report.List.Settings[0].Subject.Missing)
	require.Empty(t, report.List.Settings[0].DesiredURI)
	require.NoDirExists(t, filepath.Join(stateRoot, "identity"))
}

func TestRunAppliesRepeatableProfileLayerOverlay(t *testing.T) {
	repoRoot := setupListRepo(t, []string{"global"}, map[string]string{
		"global": `schema: dotfiles-manager.v2.profile-layer
schemaVersion: 1
selections:
  git:
    settings:
      user.email:
        scope: user
`,
		"work": `schema: dotfiles-manager.v2.profile-layer
schemaVersion: 1
selections:
  git:
    settings:
      user.email:
        scope: machine-user
      user.name:
        scope: shared
`,
	})

	report, err := Run(Options{RepoRoot: repoRoot, StateRoot: filepath.Join(t.TempDir(), "state"), MachineID: "mbp", UserID: "leon", ExtraLayers: []string{"work"}})
	require.NoError(t, err)
	require.Equal(t, []string{"global", "work"}, report.List.ProfileStack)
	require.Len(t, report.List.Settings, 2)
	require.Equal(t, "git:user.email", report.List.Settings[0].Ref)
	require.Equal(t, "machine-user", report.List.Settings[0].Scope)
	require.Equal(t, "mbp/leon", report.List.Settings[0].Subject.ID)
	require.Equal(t, "work", report.List.Settings[0].SourceLayer)
	require.Equal(t, "git:user.name", report.List.Settings[1].Ref)
	require.Equal(t, "shared", report.List.Settings[1].Scope)
	require.Equal(t, "-", report.List.Settings[1].Subject.ID)
}

func TestRunDoesNotLetExplicitIdentityOverrideExistingLocalIdentity(t *testing.T) {
	repoRoot := setupListRepo(t, []string{"global"}, map[string]string{"global": `schema: dotfiles-manager.v2.profile-layer
schemaVersion: 1
selections:
  git:
    settings:
      user.email:
        scope: machine-user
`})
	stateRoot := setupListIdentity(t, "mbp", "leon", "local")

	report, err := Run(Options{RepoRoot: repoRoot, StateRoot: stateRoot, MachineID: "other-machine", UserID: "other-user", LocalAccountName: "local"})
	require.NoError(t, err)
	require.Equal(t, "partial", report.Summary.Status)
	require.Equal(t, "mbp/leon", report.List.Settings[0].Subject.ID)
	require.Len(t, report.Diagnostics, 2)
	messages := report.Diagnostics[0].Message + "\n" + report.Diagnostics[1].Message
	require.Contains(t, messages, "conflicting --machine-id ignored")
	require.Contains(t, messages, "conflicting --user-id ignored")
}

func TestRunReportsRecipeAndProfileProblems(t *testing.T) {
	repoRoot := setupListRepo(t, []string{"global"}, map[string]string{"global": `schema: dotfiles-manager.v2.profile-layer
schemaVersion: 1
selections:
  unknown.app:
    settings:
      config:
        scope: shared
`})
	report, err := Run(Options{RepoRoot: repoRoot, StateRoot: filepath.Join(t.TempDir(), "state")})
	require.NoError(t, err)
	require.Equal(t, "partial", report.Summary.Status)
	require.Equal(t, CodeRecipeInvalid, report.Diagnostics[0].Code)
	require.Equal(t, "unknown.app:config", report.List.Settings[0].Ref)

	badRoot := setupListRepo(t, []string{"global"}, map[string]string{"global": `schema: dotfiles-manager.v2.profile-layer
schemaVersion: 1
selections:
  git:
    settings:
      user.email:
        scope: planet
`})
	report, err = Run(Options{RepoRoot: badRoot, StateRoot: filepath.Join(t.TempDir(), "state")})
	require.Error(t, err)
	require.Equal(t, CodeSelectionInvalid, report.Error.Code)
}

func TestRunReportsProfileLoadAndIdentityDiagnostics(t *testing.T) {
	t.Run("missing repo root", func(t *testing.T) {
		report, err := Run(Options{RepoRoot: filepath.Join(t.TempDir(), "missing"), StateRoot: filepath.Join(t.TempDir(), "state")})
		require.Error(t, err)
		require.Equal(t, CodeProfileInvalid, report.Error.Code)
	})

	t.Run("invalid schema", func(t *testing.T) {
		root := t.TempDir()
		writeListFile(t, filepath.Join(root, "dotfiles-manager.v2.yaml"), "schema: wrong\nschemaVersion: 1\nactiveProfileStack: default\n")
		report, err := Run(Options{RepoRoot: root, StateRoot: filepath.Join(t.TempDir(), "state")})
		require.Error(t, err)
		require.Equal(t, CodeProfileInvalid, report.Error.Code)
	})

	t.Run("profile shape validation", func(t *testing.T) {
		root := t.TempDir()
		writeListFile(t, filepath.Join(root, "dotfiles-manager.v2.yaml"), "schema: dotfiles-manager.v2.root-config\nschemaVersion: 1\nactiveProfileStack: ../bad\n")
		report, err := Run(Options{RepoRoot: root, StateRoot: filepath.Join(t.TempDir(), "state")})
		require.Error(t, err)
		require.Equal(t, CodeProfileInvalid, report.Error.Code)

		root = t.TempDir()
		writeListFile(t, filepath.Join(root, "dotfiles-manager.v2.yaml"), "schema: dotfiles-manager.v2.root-config\nschemaVersion: 1\nactiveProfileStack: default\n")
		writeListFile(t, filepath.Join(root, "profiles", "stacks", "default.yaml"), "schema: dotfiles-manager.v2.profile-stack\nschemaVersion: 2\nprofileStack: []\n")
		report, err = Run(Options{RepoRoot: root, StateRoot: filepath.Join(t.TempDir(), "state")})
		require.Error(t, err)
		require.Equal(t, CodeProfileInvalid, report.Error.Code)

		root = t.TempDir()
		writeListFile(t, filepath.Join(root, "dotfiles-manager.v2.yaml"), "schema: dotfiles-manager.v2.root-config\nschemaVersion: 1\nactiveProfileStack: default\n")
		writeListFile(t, filepath.Join(root, "profiles", "stacks", "default.yaml"), "schema: dotfiles-manager.v2.profile-stack\nschemaVersion: 1\nprofileStack: []\n")
		report, err = Run(Options{RepoRoot: root, StateRoot: filepath.Join(t.TempDir(), "state")})
		require.Error(t, err)
		require.Equal(t, CodeProfileInvalid, report.Error.Code)
	})

	t.Run("invalid extra profile", func(t *testing.T) {
		repoRoot := setupListRepo(t, []string{"global"}, map[string]string{"global": "schema: dotfiles-manager.v2.profile-layer\nschemaVersion: 1\nselections: {}\n"})
		report, err := Run(Options{RepoRoot: repoRoot, StateRoot: filepath.Join(t.TempDir(), "state"), ExtraLayers: []string{"../bad"}})
		require.Error(t, err)
		require.Equal(t, CodeProfileInvalid, report.Error.Code)
	})

	t.Run("invalid identity flags remain partial", func(t *testing.T) {
		repoRoot := setupListRepo(t, []string{"global"}, map[string]string{"global": `schema: dotfiles-manager.v2.profile-layer
schemaVersion: 1
selections:
  git:
    settings:
      user.email:
        scope: user
`})
		report, err := Run(Options{RepoRoot: repoRoot, StateRoot: filepath.Join(t.TempDir(), "state"), UserID: "BAD"})
		require.NoError(t, err)
		require.Equal(t, "partial", report.Summary.Status)
		require.Equal(t, CodeIdentityInvalid, report.Diagnostics[0].Code)
		require.False(t, report.List.Settings[0].Subject.Resolved)
	})

	t.Run("invalid local identity records warn", func(t *testing.T) {
		repoRoot := setupListRepo(t, []string{"global"}, map[string]string{"global": `schema: dotfiles-manager.v2.profile-layer
schemaVersion: 1
selections:
  git:
    settings:
      user.email:
        scope: machine
`})
		stateRoot := filepath.Join(t.TempDir(), "state")
		writeListFile(t, filepath.Join(stateRoot, "identity", "machine.yaml"), "schema: dotfiles-manager.v2.machine-identity\nschemaVersion: 1\nmachineId: BAD\n")
		report, err := Run(Options{RepoRoot: repoRoot, StateRoot: stateRoot})
		require.NoError(t, err)
		require.Equal(t, "partial", report.Summary.Status)
		require.Equal(t, CodeIdentityInvalid, report.Diagnostics[0].Code)
		require.Equal(t, []string{"machine-id"}, report.List.Settings[0].Subject.Missing)
	})

	t.Run("invalid user identity record warns", func(t *testing.T) {
		repoRoot := setupListRepo(t, []string{"global"}, map[string]string{"global": `schema: dotfiles-manager.v2.profile-layer
schemaVersion: 1
selections:
  git:
    settings:
      user.email:
        scope: user
`})
		stateRoot := filepath.Join(t.TempDir(), "state")
		writeListFile(t, filepath.Join(stateRoot, "identity", "users", "leon.yaml"), "schema: dotfiles-manager.v2.user-identity\nschemaVersion: 1\nlocalAccountKey: other\nuserId: leon\n")
		report, err := Run(Options{RepoRoot: repoRoot, StateRoot: stateRoot, LocalAccountName: "leon"})
		require.NoError(t, err)
		require.Equal(t, "partial", report.Summary.Status)
		require.Equal(t, CodeIdentityInvalid, report.Diagnostics[0].Code)
		require.False(t, report.List.Settings[0].Subject.Resolved)
	})
}

func TestSubjectsAndDesiredBindingBranches(t *testing.T) {
	ids := identityIDs{MachineID: "mbp", UserID: "leon"}
	require.Equal(t, SubjectInfo{Resolved: true, ID: "-", Missing: []string{}}, subjectFor("shared", ids))
	require.Equal(t, SubjectInfo{Resolved: true, ID: "mbp", Missing: []string{}}, subjectFor("machine", ids))
	require.Equal(t, SubjectInfo{Resolved: false, Missing: []string{"machine-id"}}, subjectFor("machine", identityIDs{}))
	require.Equal(t, SubjectInfo{Resolved: false, Missing: []string{"machine-id", "user-id"}}, subjectFor("machine-user", identityIDs{}))

	uri, rel, err := desiredBinding("user", "leon", "git", "user.email", "settings.yaml#user.email")
	require.NoError(t, err)
	require.Equal(t, "desired://user/leon/targets/git/settings#user.email", uri)
	require.Equal(t, filepath.Join("desired", "user", "leon", "targets", "git", "settings.yaml"), rel)
	uri, rel, err = desiredBinding("user", "leon", "zsh", "zshrc", "artifacts/zshrc")
	require.NoError(t, err)
	require.Equal(t, "desired://user/leon/targets/zsh/artifacts/zshrc", uri)
	require.Equal(t, filepath.Join("desired", "user", "leon", "targets", "zsh", "artifacts", "zshrc"), rel)
	uri, _, err = desiredBinding("shared", "-", "git", "user.email", "manifest.yaml")
	require.NoError(t, err)
	require.Equal(t, "desired://shared/-/targets/git/manifest", uri)
}

func TestRenderAndHelperBranches(t *testing.T) {
	oldWD, wdErr := os.Getwd()
	require.NoError(t, wdErr)
	tempWD := t.TempDir()
	writeListFile(t, filepath.Join(tempWD, "dotfiles-manager.v2.yaml"), "schema: dotfiles-manager.v2.root-config\nschemaVersion: 1\nactiveProfileStack: default\n")
	require.NoError(t, os.Chdir(tempWD))
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	root, err := resolveRepoRoot("")
	require.NoError(t, err)
	realTempWD, realErr := filepath.EvalSymlinks(tempWD)
	require.NoError(t, realErr)
	require.Equal(t, realTempWD, root)

	require.Contains(t, Text(nil), "summary status=error")
	nilJSON, err := JSON(nil)
	require.NoError(t, err)
	require.Contains(t, nilJSON, `"status": "error"`)
	report := baseReport()
	report.List.Settings = []ManagedSetting{{
		Ref:          "ssh:config",
		Scope:        "machine-user",
		ScopeLabel:   ScopeLabel("machine-user"),
		Subject:      SubjectInfo{Resolved: false, Missing: []string{"machine-id", "user-id"}},
		SourceLayer:  "global",
		Artifact:     "artifacts/config",
		Resource:     ResourceInfo{ID: "config", DriverID: "file", LocationID: "home", Path: ".ssh/config"},
		NextActions:  nextActions("ssh:config"),
		Target:       TargetInfo{ID: "ssh"},
		Setting:      SettingInfo{ID: "config", Label: "Config"},
		DesiredURI:   "",
		ArtifactForm: "file",
	}}
	report.Diagnostics = []Diagnostic{{Code: "d", Severity: SeverityWarning, Message: "warning"}}
	report.Error = &ErrorObject{Code: "e", Message: "error"}
	finish(report)
	text := Text(report)
	require.Contains(t, text, "missing identity: machine-id,user-id")
	require.Contains(t, text, "artifact=artifacts/config")
	require.Contains(t, text, "warning[d]")
	require.Contains(t, text, "error[e]")
	_, err = JSON(&Report{Error: &ErrorObject{Details: map[string]any{"bad": func() {}}}})
	require.Error(t, err)
	require.Equal(t, "", (*Error)(nil).Error())
	require.Equal(t, "boom", (&Error{Message: "boom"}).Error())
	require.Equal(t, 1, (*Error)(nil).ExitCode())
	require.Equal(t, 4, (&Error{Exit: 4}).ExitCode())
	require.Equal(t, "Everyone using this repo", ScopeLabel("shared"))
	require.Equal(t, "Me on all my machines", ScopeLabel("user"))
	require.Equal(t, "This machine", ScopeLabel("machine"))
	require.Equal(t, "Me on this machine", ScopeLabel("machine-user"))
	require.Equal(t, "custom", ScopeLabel("custom"))
	require.Equal(t, "x", dash("x"))
	require.Equal(t, "-", dash(""))
	require.Equal(t, "local", safeIDCandidate("***", "local"))
	require.Equal(t, "hello-world", safeIDCandidate("Hello World", "fallback"))
	require.NotEmpty(t, localAccountKey(""))
	for _, value := range []string{"", "/abs", "../bad", "bad//path", `bad\\path`, "Bad"} {
		_, err = validateProfilePathID("profile layer", value)
		require.Error(t, err, value)
	}
	failed := baseReport()
	failed.Summary.Failed = 1
	finish(failed)
	require.Equal(t, "error", failed.Summary.Status)
	blocked := baseReport()
	blocked.Summary.Blocked = 1
	finish(blocked)
	require.Equal(t, "blocked", blocked.Summary.Status)
	_, _, err = desiredBinding("user", "leon", "git", "user.email", "../bad")
	require.Error(t, err)
	_, _, err = desiredBinding("user", "leon", "git", "user.email", "artifacts/user.email#bad")
	require.Error(t, err)
	_, _, err = desiredBinding("user", "leon", "git", "user.email", "manifest.yaml#bad")
	require.Error(t, err)
}

func setupListRepo(t *testing.T, stack []string, layers map[string]string) string {
	t.Helper()
	root := t.TempDir()
	writeListFile(t, filepath.Join(root, "dotfiles-manager.v2.yaml"), "schema: dotfiles-manager.v2.root-config\nschemaVersion: 1\nactiveProfileStack: default\n")
	stackBody := "schema: dotfiles-manager.v2.profile-stack\nschemaVersion: 1\nprofileStack:\n"
	for _, layer := range stack {
		stackBody += "  - " + layer + "\n"
	}
	writeListFile(t, filepath.Join(root, "profiles", "stacks", "default.yaml"), stackBody)
	for layer, body := range layers {
		writeListFile(t, filepath.Join(root, "profiles", "layers", filepath.FromSlash(layer)+".yaml"), body)
	}
	return root
}

func setupListIdentity(t *testing.T, machineID string, userID string, localKey string) string {
	t.Helper()
	stateRoot := filepath.Join(t.TempDir(), "state")
	writeListFile(t, filepath.Join(stateRoot, "identity", "machine.yaml"), "schema: dotfiles-manager.v2.machine-identity\nschemaVersion: 1\nmachineId: "+machineID+"\n")
	writeListFile(t, filepath.Join(stateRoot, "identity", "users", localKey+".yaml"), "schema: dotfiles-manager.v2.user-identity\nschemaVersion: 1\nlocalAccountKey: "+localKey+"\nuserId: "+userID+"\n")
	return stateRoot
}

func writeListFile(t *testing.T, path string, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}
