package initcmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestRunYesWritesScaffoldAndIdentityState(t *testing.T) {
	repoRoot := t.TempDir()
	stateRoot := filepath.Join(t.TempDir(), "state")

	report, err := Run(Options{RepoRoot: repoRoot, StateRoot: stateRoot, Yes: true, Hostname: "Leo MBP", LocalAccountName: "Leon K"})
	require.NoError(t, err)
	require.Equal(t, "changed", report.Summary.Status)
	require.Equal(t, 5, report.Summary.Written)
	require.Equal(t, "default", report.Init.ActiveProfileStack)
	require.Equal(t, []string{"global"}, report.Init.ProfileStack)
	require.Equal(t, "create", report.Init.IdentityFiles[0].Action)
	require.Equal(t, "leo-mbp", report.Init.IdentityFiles[0].ID)
	require.Equal(t, "leon-k", report.Init.IdentityFiles[1].LocalAccountKey)
	require.NotContains(t, report.Init.IdentityFiles[0].Path, repoRoot)

	require.FileExists(t, filepath.Join(repoRoot, "dotfiles-manager.v2.yaml"))
	require.FileExists(t, filepath.Join(repoRoot, "profiles", "stacks", "default.yaml"))
	require.FileExists(t, filepath.Join(repoRoot, "profiles", "layers", "global.yaml"))
	require.Contains(t, readFile(t, filepath.Join(stateRoot, "identity", "machine.yaml")), "machineId: leo-mbp")
	require.Contains(t, readFile(t, filepath.Join(stateRoot, "identity", "users", "leon-k.yaml")), "userId: leon-k")

	payload, err := JSON(report)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal([]byte(payload), &decoded))
	require.Equal(t, Schema, decoded["schema"])
}

func TestInitHappyPathTextAndJSONSnapshots(t *testing.T) {
	repoRoot := t.TempDir()
	stateRoot := filepath.Join(t.TempDir(), "state")

	report, err := Run(Options{RepoRoot: repoRoot, StateRoot: stateRoot, Yes: true, Hostname: "snapshot-machine", LocalAccountName: "snapshot-user"})
	require.NoError(t, err)
	require.Equal(t, `init
profile stack: default [global]
repo files:
  root-config action=create path=dotfiles-manager.v2.yaml
  profile-stack action=create path=profiles/stacks/default.yaml
  profile-layer action=create path=profiles/layers/global.yaml
identity files:
  machine action=create source=generated path=state://identity/machine.yaml id=snapshot-machine
  user action=create source=generated path=state://identity/users/snapshot-user.yaml id=snapshot-user localAccountKey=snapshot-user
summary status=changed planned=5 written=5 unchanged=0 blocked=0 failed=0`, Text(report))

	payload, err := JSON(report)
	require.NoError(t, err)
	require.JSONEq(t, `{
  "schema": "dotfiles-manager.v2.init",
  "schemaVersion": 1,
  "command": "init",
  "runId": "init",
  "dryRun": false,
  "summary": {
    "status": "changed",
    "planned": 5,
    "written": 5,
    "unchanged": 0,
    "blocked": 0,
    "failed": 0
  },
  "init": {
    "activeProfileStack": "default",
    "profileStack": ["global"],
    "repoFiles": [
      {"kind": "root-config", "path": "dotfiles-manager.v2.yaml", "action": "create"},
      {"kind": "profile-stack", "path": "profiles/stacks/default.yaml", "action": "create"},
      {"kind": "profile-layer", "path": "profiles/layers/global.yaml", "action": "create"}
    ],
    "identityFiles": [
      {"kind": "machine", "path": "state://identity/machine.yaml", "id": "snapshot-machine", "source": "generated", "action": "create"},
      {"kind": "user", "path": "state://identity/users/snapshot-user.yaml", "id": "snapshot-user", "localAccountKey": "snapshot-user", "source": "generated", "action": "create"}
    ],
    "missingChoices": []
  },
  "diagnostics": [],
  "error": null
}`, payload)
}

func TestRunUsesDefaultStateRootWhenStateRootOmitted(t *testing.T) {
	repoRoot := t.TempDir()
	homeRoot := t.TempDir()
	t.Setenv("HOME", homeRoot)

	report, err := Run(Options{RepoRoot: repoRoot, Yes: true, Hostname: "Default State", LocalAccountName: "default-user"})
	require.NoError(t, err)
	require.Equal(t, "changed", report.Summary.Status)
	require.Equal(t, "default-state", report.Init.IdentityFiles[0].ID)
	switch runtime.GOOS {
	case "darwin":
		require.DirExists(t, filepath.Join(homeRoot, "Library", "Application Support", "dotfiles-manager", "v2"))
	case "linux":
		require.DirExists(t, filepath.Join(homeRoot, ".local", "state", "dotfiles-manager", "v2"))
	}
}

func TestRunJSONDoesNotPromptOrImplyIdentityApproval(t *testing.T) {
	repoRoot := t.TempDir()
	stateRoot := filepath.Join(t.TempDir(), "state")

	report, err := Run(Options{RepoRoot: repoRoot, StateRoot: stateRoot, JSONMode: true, Hostname: "Leo MBP", LocalAccountName: "Leon K", Input: strings.NewReader("leo\nleon\n")})
	require.Error(t, err)
	require.Equal(t, CodeIdentityRequired, report.Error.Code)
	require.Equal(t, "blocked", report.Summary.Status)
	require.Equal(t, "machine-id", report.Init.MissingChoices[0].Kind)
	require.NoFileExists(t, filepath.Join(repoRoot, "dotfiles-manager.v2.yaml"))
	require.NoFileExists(t, filepath.Join(stateRoot, "identity", "machine.yaml"))
}

func TestRunInteractivePromptsAndDryRunDoesNotWrite(t *testing.T) {
	repoRoot := t.TempDir()
	stateRoot := filepath.Join(t.TempDir(), "state")
	var prompts strings.Builder

	report, err := Run(Options{RepoRoot: repoRoot, StateRoot: stateRoot, Input: strings.NewReader("machine-a\nuser-a\n"), PromptOutput: &prompts, Hostname: "fallback", LocalAccountName: "fallback-user"})
	require.NoError(t, err)
	require.Contains(t, prompts.String(), "visible in repository paths")
	require.Equal(t, "machine-a", report.Init.IdentityFiles[0].ID)
	require.Equal(t, "user-a", report.Init.IdentityFiles[1].ID)

	dryRepo := t.TempDir()
	dryState := filepath.Join(t.TempDir(), "state")
	dry, err := Run(Options{RepoRoot: dryRepo, StateRoot: dryState, DryRun: true, Yes: true, Hostname: "dry", LocalAccountName: "dry"})
	require.NoError(t, err)
	require.Equal(t, "changed", dry.Summary.Status)
	require.Equal(t, 5, dry.Summary.Planned)
	require.Equal(t, 0, dry.Summary.Written)
	require.NoFileExists(t, filepath.Join(dryRepo, "dotfiles-manager.v2.yaml"))
}

func TestRunRejectsPartialScaffoldAndIdentityConflict(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files []string
	}{
		{name: "root only", files: []string{"root"}},
		{name: "stack only", files: []string{"stack"}},
		{name: "layer only", files: []string{"layer"}},
		{name: "root and stack", files: []string{"root", "stack"}},
		{name: "root and layer", files: []string{"root", "layer"}},
		{name: "stack and layer", files: []string{"stack", "layer"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repoRoot := t.TempDir()
			stateRoot := filepath.Join(t.TempDir(), "state")
			writeScaffoldSubset(t, repoRoot, tc.files...)
			report, err := Run(Options{RepoRoot: repoRoot, StateRoot: stateRoot, Yes: true})
			require.Error(t, err)
			require.Equal(t, CodeRepoPartial, report.Error.Code)
		})
	}

	completeRepo := t.TempDir()
	stateRoot := filepath.Join(t.TempDir(), "state")
	writeFile(t, filepath.Join(completeRepo, "dotfiles-manager.v2.yaml"), repoFileBody("root-config"))
	writeFile(t, filepath.Join(completeRepo, "profiles", "stacks", "default.yaml"), repoFileBody("profile-stack"))
	writeFile(t, filepath.Join(completeRepo, "profiles", "layers", "global.yaml"), repoFileBody("profile-layer"))
	writeFile(t, filepath.Join(stateRoot, "identity", "machine.yaml"), "schema: dotfiles-manager.v2.machine-identity\nschemaVersion: 1\nmachineId: old-machine\n")
	writeFile(t, filepath.Join(stateRoot, "identity", "users", "leon.yaml"), "schema: dotfiles-manager.v2.user-identity\nschemaVersion: 1\nlocalAccountKey: leon\nuserId: leon\n")

	report, err := Run(Options{RepoRoot: completeRepo, StateRoot: stateRoot, MachineID: "new-machine", UserID: "leon", LocalAccountName: "leon"})
	require.Error(t, err)
	require.Equal(t, CodeIdentityConflict, report.Error.Code)
}

func TestRunRejectsWrongSchemaInEveryScaffoldFile(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind string
		body string
	}{
		{name: "wrong root schema", kind: "root", body: "schema: wrong\nschemaVersion: 1\nactiveProfileStack: default\n"},
		{name: "wrong root version", kind: "root", body: "schema: dotfiles-manager.v2.root-config\nschemaVersion: 2\nactiveProfileStack: default\n"},
		{name: "wrong stack schema", kind: "stack", body: "schema: wrong\nschemaVersion: 1\nprofileStack:\n  - global\n"},
		{name: "wrong stack version", kind: "stack", body: "schema: dotfiles-manager.v2.profile-stack\nschemaVersion: 2\nprofileStack:\n  - global\n"},
		{name: "wrong layer schema", kind: "layer", body: "schema: wrong\nschemaVersion: 1\nselections: {}\n"},
		{name: "wrong layer version", kind: "layer", body: "schema: dotfiles-manager.v2.profile-layer\nschemaVersion: 2\nselections: {}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repoRoot := t.TempDir()
			writeScaffoldSubset(t, repoRoot, "root", "stack", "layer")
			switch tc.kind {
			case "root":
				writeFile(t, filepath.Join(repoRoot, "dotfiles-manager.v2.yaml"), tc.body)
			case "stack":
				writeFile(t, filepath.Join(repoRoot, "profiles", "stacks", "default.yaml"), tc.body)
			case "layer":
				writeFile(t, filepath.Join(repoRoot, "profiles", "layers", "global.yaml"), tc.body)
			}
			report, err := Run(Options{RepoRoot: repoRoot, StateRoot: filepath.Join(t.TempDir(), "state"), Yes: true})
			require.Error(t, err)
			require.Equal(t, CodeRepoInvalid, report.Error.Code)
		})
	}
}

func TestRunExistingScaffoldAndIdentitiesAreUnchangedAndTextIsUseful(t *testing.T) {
	repoRoot := t.TempDir()
	stateRoot := filepath.Join(t.TempDir(), "state")
	writeFile(t, filepath.Join(repoRoot, "dotfiles-manager.v2.yaml"), repoFileBody("root-config"))
	writeFile(t, filepath.Join(repoRoot, "profiles", "stacks", "default.yaml"), repoFileBody("profile-stack"))
	writeFile(t, filepath.Join(repoRoot, "profiles", "layers", "global.yaml"), repoFileBody("profile-layer"))
	writeFile(t, filepath.Join(stateRoot, "identity", "machine.yaml"), "schema: dotfiles-manager.v2.machine-identity\nschemaVersion: 1\nmachineId: mbp\n")
	writeFile(t, filepath.Join(stateRoot, "identity", "users", "leon.yaml"), "schema: dotfiles-manager.v2.user-identity\nschemaVersion: 1\nlocalAccountKey: leon\nuserId: leon\n")

	report, err := Run(Options{RepoRoot: repoRoot, StateRoot: stateRoot, MachineID: "mbp", UserID: "leon", LocalAccountName: "leon"})
	require.NoError(t, err)
	require.Equal(t, "ok", report.Summary.Status)
	require.Equal(t, 5, report.Summary.Unchanged)
	text := Text(report)
	require.Contains(t, text, "repo files:")
	require.Contains(t, text, "identity files:")
	require.Contains(t, text, "machine action=unchanged")
	require.Contains(t, text, "summary status=ok")
}

func TestRunCreatesExplicitUserAndRejectsUserConflict(t *testing.T) {
	repoRoot := t.TempDir()
	stateRoot := filepath.Join(t.TempDir(), "state")
	writeFile(t, filepath.Join(stateRoot, "identity", "machine.yaml"), "schema: dotfiles-manager.v2.machine-identity\nschemaVersion: 1\nmachineId: mbp\n")

	report, err := Run(Options{RepoRoot: repoRoot, StateRoot: stateRoot, MachineID: "mbp", UserID: "leon", LocalAccountName: "local"})
	require.NoError(t, err)
	require.Equal(t, "explicit", report.Init.IdentityFiles[1].Source)
	require.Contains(t, readFile(t, filepath.Join(stateRoot, "identity", "users", "local.yaml")), "userId: leon")

	report, err = Run(Options{RepoRoot: repoRoot, StateRoot: stateRoot, MachineID: "mbp", UserID: "other", LocalAccountName: "local"})
	require.Error(t, err)
	require.Equal(t, CodeIdentityConflict, report.Error.Code)
	require.Contains(t, Text(report), "error[init.identity.conflict]")
}

func TestRunValidationFailuresDoNotUseWorkarounds(t *testing.T) {
	t.Run("state root inside repo", func(t *testing.T) {
		repoRoot := t.TempDir()
		report, err := Run(Options{RepoRoot: repoRoot, StateRoot: filepath.Join(repoRoot, ".state"), Yes: true})
		require.Error(t, err)
		require.Equal(t, CodeStateRootInvalid, report.Error.Code)
	})

	t.Run("repo root missing and file root", func(t *testing.T) {
		report, err := Run(Options{RepoRoot: filepath.Join(t.TempDir(), "missing"), StateRoot: filepath.Join(t.TempDir(), "state"), Yes: true})
		require.Error(t, err)
		require.Equal(t, CodeRepoInvalid, report.Error.Code)
		fileRoot := filepath.Join(t.TempDir(), "file")
		writeFile(t, fileRoot, "x")
		report, err = Run(Options{RepoRoot: fileRoot, StateRoot: filepath.Join(t.TempDir(), "state"), Yes: true})
		require.Error(t, err)
		require.Equal(t, CodeRepoInvalid, report.Error.Code)
	})

	t.Run("bad existing schemas", func(t *testing.T) {
		repoRoot := t.TempDir()
		writeFile(t, filepath.Join(repoRoot, "dotfiles-manager.v2.yaml"), "schema: wrong\nschemaVersion: 1\n")
		writeFile(t, filepath.Join(repoRoot, "profiles", "stacks", "default.yaml"), repoFileBody("profile-stack"))
		writeFile(t, filepath.Join(repoRoot, "profiles", "layers", "global.yaml"), repoFileBody("profile-layer"))
		report, err := Run(Options{RepoRoot: repoRoot, StateRoot: filepath.Join(t.TempDir(), "state"), Yes: true})
		require.Error(t, err)
		require.Equal(t, CodeRepoInvalid, report.Error.Code)
	})

	t.Run("invalid identity records", func(t *testing.T) {
		repoRoot := t.TempDir()
		stateRoot := filepath.Join(t.TempDir(), "state")
		writeFile(t, filepath.Join(stateRoot, "identity", "machine.yaml"), "schema: dotfiles-manager.v2.machine-identity\nschemaVersion: 1\nmachineId: BAD\n")
		report, err := Run(Options{RepoRoot: repoRoot, StateRoot: stateRoot, Yes: true})
		require.Error(t, err)
		require.Equal(t, CodeIdentityInvalid, report.Error.Code)

		stateRoot = filepath.Join(t.TempDir(), "state")
		writeFile(t, filepath.Join(stateRoot, "identity", "users", "leon.yaml"), "schema: wrong\nschemaVersion: 1\nlocalAccountKey: leon\nuserId: leon\n")
		report, err = Run(Options{RepoRoot: repoRoot, StateRoot: stateRoot, MachineID: "mbp", UserID: "leon", LocalAccountName: "leon"})
		require.Error(t, err)
		require.Equal(t, CodeIdentityInvalid, report.Error.Code)
	})

	t.Run("user identity local key mismatch", func(t *testing.T) {
		repoRoot := t.TempDir()
		stateRoot := filepath.Join(t.TempDir(), "state")
		writeFile(t, filepath.Join(stateRoot, "identity", "machine.yaml"), "schema: dotfiles-manager.v2.machine-identity\nschemaVersion: 1\nmachineId: mbp\n")
		writeFile(t, filepath.Join(stateRoot, "identity", "users", "leon.yaml"), "schema: dotfiles-manager.v2.user-identity\nschemaVersion: 1\nlocalAccountKey: other\nuserId: leon\n")
		report, err := Run(Options{RepoRoot: repoRoot, StateRoot: stateRoot, LocalAccountName: "leon"})
		require.Error(t, err)
		require.Equal(t, CodeIdentityInvalid, report.Error.Code)
	})
}

func TestGeneratedIDsAvoidSilentDesiredSubjectAdoption(t *testing.T) {
	repoRoot := t.TempDir()
	stateRoot := filepath.Join(t.TempDir(), "state")
	writeFile(t, filepath.Join(repoRoot, "desired", "machine", "leo-mbp", "keep"), "x")
	writeFile(t, filepath.Join(repoRoot, "desired", "user", "leon", "keep"), "x")

	report, err := Run(Options{RepoRoot: repoRoot, StateRoot: stateRoot, Yes: true, Hostname: "Leo MBP", LocalAccountName: "leon"})
	require.NoError(t, err)
	require.Equal(t, "leo-mbp-2", report.Init.IdentityFiles[0].ID)
	require.Equal(t, "leon-2", report.Init.IdentityFiles[1].ID)
}

func TestHelpersAndRenderErrorBranches(t *testing.T) {
	oldWD, wdErr := os.Getwd()
	require.NoError(t, wdErr)
	tempWD := t.TempDir()
	require.NoError(t, os.Chdir(tempWD))
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	root, err := normalizeRepoRoot("")
	require.NoError(t, err)
	realTempWD, realErr := filepath.EvalSymlinks(tempWD)
	require.NoError(t, realErr)
	require.Equal(t, realTempWD, root)

	nilJSON, err := JSON(nil)
	require.NoError(t, err)
	require.Contains(t, nilJSON, `"status": "error"`)
	require.Equal(t, "local", safeIDCandidate("***", "local"))
	require.Equal(t, "hello-world", safeIDCandidate("Hello World", "fallback"))
	require.Equal(t, "local-user", localAccountKey("***"))
	require.NotEmpty(t, localAccountKey(""))
	require.NotEmpty(t, hostname())
	require.Equal(t, "machine", firstNonEmpty("", "machine"))
	require.Equal(t, "init\nsummary status=error planned=0 written=0 unchanged=0 blocked=0 failed=1", Text(nil))
	require.Contains(t, Text(&Report{Diagnostics: []Diagnostic{{Code: "d", Severity: SeverityWarning, Message: "warning"}}, Error: &ErrorObject{Code: "e", Message: "error"}}), "warning[d]")
	renderReport := baseReport(true)
	renderReport.Init.ActiveProfileStack = "default"
	renderReport.Init.ProfileStack = []string{"global"}
	renderReport.Init.RepoFiles = []InitFile{{Kind: "root-config", Path: "dotfiles-manager.v2.yaml", Action: "create"}}
	renderReport.Init.IdentityFiles = []IdentityFile{{Kind: "user", Path: "state://identity/users/leon.yaml", ID: "leon", LocalAccountKey: "leon", Source: "explicit", Action: "create"}}
	renderReport.Init.MissingChoices = []MissingChoice{{Kind: "machine-id", Recommended: []string{"mbp"}}}
	renderReport.Summary.Planned = 2
	finish(renderReport)
	rendered := Text(renderReport)
	require.Contains(t, rendered, "MODE: DRY RUN")
	require.Contains(t, rendered, "missing choice: machine-id recommended=mbp")
	require.Contains(t, rendered, "localAccountKey=leon")
	_, err = JSON(&Report{Error: &ErrorObject{Details: map[string]any{"bad": func() {}}}})
	require.Error(t, err)
	require.Equal(t, "", (*Error)(nil).Error())
	require.Equal(t, 1, (*Error)(nil).ExitCode())
	require.Equal(t, 9, (&Error{Exit: 9}).ExitCode())
	require.Equal(t, CodeRepoPartial, codeForRepoPlanError(&Error{Code: CodeRepoPartial}))
	require.Equal(t, CodeRepoInvalid, codeForRepoPlanError(os.ErrNotExist))
	require.Equal(t, "x", errorCode(&Error{Code: "x"}))
	require.Equal(t, CodeIdentityRequired, errorCode(os.ErrNotExist))
	require.Equal(t, 7, errorExit(&Error{Exit: 7}, 4))
	require.Equal(t, 4, errorExit(os.ErrNotExist, 4))
	require.Error(t, validateIdentityID("user", "BAD"))
	require.Equal(t, "", repoFileBody("unknown"))

	blocked := baseReport(false)
	blocked.Summary.Blocked = 1
	finish(blocked)
	require.Equal(t, "blocked", blocked.Summary.Status)
	failed := baseReport(false)
	failed.Summary.Failed = 1
	finish(failed)
	require.Equal(t, "error", failed.Summary.Status)

	_, err = promptIdentity(Options{Input: strings.NewReader(""), PromptOutput: &strings.Builder{}}, "User ID", "desired/user/local/...", "local")
	require.Error(t, err)
	line, err := readLine(strings.NewReader("value"))
	require.NoError(t, err)
	require.Equal(t, "value", line)
	_, err = readLine(strings.NewReader(""))
	require.Error(t, err)

	dir := t.TempDir()
	present, err := regularFileExists(dir)
	require.Error(t, err)
	require.False(t, present)
	target := filepath.Join(t.TempDir(), "target")
	writeFile(t, target, "x")
	link := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(target, link))
	present, err = regularFileExists(link)
	require.Error(t, err)
	require.False(t, present)

	badYAML := filepath.Join(t.TempDir(), "bad.yaml")
	writeFile(t, badYAML, "schema: [")
	require.Error(t, validateExistingSchema(badYAML, "bad", "schema"))
	scalarYAML := filepath.Join(t.TempDir(), "scalar.yaml")
	writeFile(t, scalarYAML, "scalar\n")
	require.Error(t, validateExistingSchema(scalarYAML, "bad", "schema"))
	badVersion := filepath.Join(t.TempDir(), "version.yaml")
	writeFile(t, badVersion, "schema: schema\nschemaVersion: 2\n")
	require.Error(t, validateExistingSchema(badVersion, "bad", "schema"))
	require.Equal(t, yaml.DocumentNode, documentMapping(&yaml.Node{Kind: yaml.DocumentNode}).Kind)
	require.Equal(t, "", scalarValue(&yaml.Node{Kind: yaml.ScalarNode}, "schema"))
	require.Equal(t, "", scalarValue(&yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{{Kind: yaml.ScalarNode, Value: "schema"}, {Kind: yaml.SequenceNode}}}, "schema"))

	identityPath := filepath.Join(t.TempDir(), "identity.yaml")
	writeFile(t, identityPath, "schema: wrong\nschemaVersion: 1\nmachineId: mbp\n")
	_, _, err = readMachineIdentity(identityPath)
	require.Error(t, err)
	userPath := filepath.Join(t.TempDir(), "user.yaml")
	writeFile(t, userPath, "schema: dotfiles-manager.v2.user-identity\nschemaVersion: 1\nlocalAccountKey: BAD\nuserId: leon\n")
	_, _, err = readUserIdentity(userPath)
	require.Error(t, err)

	fileRoot := filepath.Join(t.TempDir(), "file-root")
	writeFile(t, fileRoot, "x")
	require.Error(t, writePlannedRepoFiles(fileRoot, []InitFile{{Kind: "root-config", Path: "child.yaml", Action: "create"}}))
	require.NoError(t, writePlannedIdentityFiles(t.TempDir(), []IdentityFile{{Kind: "machine", Action: "unchanged", Path: "state://identity/machine.yaml", ID: "mbp"}}))
	require.Error(t, writePlannedIdentityFiles(fileRoot, []IdentityFile{{Kind: "machine", Action: "create", Path: "state://identity/machine.yaml", ID: "mbp"}}))
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

func writeScaffoldSubset(t *testing.T, repoRoot string, files ...string) {
	t.Helper()
	for _, file := range files {
		switch file {
		case "root":
			writeFile(t, filepath.Join(repoRoot, "dotfiles-manager.v2.yaml"), repoFileBody("root-config"))
		case "stack":
			writeFile(t, filepath.Join(repoRoot, "profiles", "stacks", "default.yaml"), repoFileBody("profile-stack"))
		case "layer":
			writeFile(t, filepath.Join(repoRoot, "profiles", "layers", "global.yaml"), repoFileBody("profile-layer"))
		}
	}
}
