package resolution

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFindRootUsesV2MarkerOnly(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	nested := filepath.Join(root, "a", "b")
	require.NoError(t, os.MkdirAll(nested, 0o755))
	writeFile(t, filepath.Join(root, "dotfiles-manager.v2.yaml"), "schema: dotfiles-manager.v2.root-config\nschemaVersion: 1\nactiveProfileStack: default\n")
	writeFile(t, filepath.Join(root, ".dotfiles-manager.yaml"), "syncs: []\n")

	resolved, err := FindRoot(nested)
	require.NoError(t, err)
	require.Equal(t, root, resolved)

	v1Only := t.TempDir()
	writeFile(t, filepath.Join(v1Only, ".dotfiles-manager.yaml"), "syncs: []\n")
	_, err = FindRoot(v1Only)
	require.Error(t, err)
	require.Contains(t, err.Error(), "dotfiles-manager.v2.yaml")
}

func TestFindRootHandlesFileStartsAndMissingStarts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeV2Root(t, root, "default")
	nestedFile := filepath.Join(root, "nested", "config.yaml")
	writeFile(t, nestedFile, "ignored")

	resolved, err := FindRoot(nestedFile)
	require.NoError(t, err)
	require.Equal(t, root, resolved)

	_, err = FindRoot(filepath.Join(root, "missing"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "stat v2 root start path")
}

func TestResolveAllScopesAndDesiredArtifacts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeV2Root(t, root, "default")
	writeStack(t, root, "default", []string{"global"})
	writeLayer(t, root, "global", `
selections:
  git:
    settings:
      user.email:
        scope: user
      user.name:
        scope: user
      shared.aliases:
        scope: shared
      host.name:
        scope: machine
      local.theme:
        scope: machine-user
`)

	resolved, err := Resolve(root, ResolveOptions{MachineID: "mbp-2026", UserID: "leon"})
	require.NoError(t, err)
	require.Equal(t, "default", resolved.ActiveProfileStack)
	require.Equal(t, []string{"global"}, resolved.Layers)

	settings := resolvedByRef(resolved)
	require.Equal(t, "desired://user/leon/targets/git/settings#user.email", settings["git:user.email"].DesiredURI)
	require.Equal(t, filepath.Join("desired", "user", "leon", "targets", "git", "settings.yaml"), settings["git:user.email"].DesiredRelPath)
	require.Equal(t, "desired://user/leon/targets/git/settings#user.name", settings["git:user.name"].DesiredURI)
	require.Equal(t, "desired://shared/-/targets/git/settings#shared.aliases", settings["git:shared.aliases"].DesiredURI)
	require.Equal(t, filepath.Join("desired", "shared", "-", "targets", "git", "settings.yaml"), settings["git:shared.aliases"].DesiredRelPath)
	require.Equal(t, "desired://machine/mbp-2026/targets/git/settings#host.name", settings["git:host.name"].DesiredURI)
	require.Equal(t, filepath.Join("desired", "machine", "mbp-2026", "targets", "git", "settings.yaml"), settings["git:host.name"].DesiredRelPath)
	require.Equal(t, "desired://machine-user/mbp-2026/leon/targets/git/settings#local.theme", settings["git:local.theme"].DesiredURI)
	require.Equal(t, filepath.Join("desired", "machine-user", "mbp-2026", "leon", "targets", "git", "settings.yaml"), settings["git:local.theme"].DesiredRelPath)
}

func TestResolveExplicitCanonicalArtifacts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeV2Root(t, root, "default")
	writeStack(t, root, "default", []string{"global"})
	writeLayer(t, root, "global", `
selections:
  cobona:
    settings:
      manifest.file:
        scope: shared
        artifact: manifest.yaml
      exported.preferences:
        scope: shared
        artifact: artifacts/preferences.json
`)

	resolved, err := Resolve(root, ResolveOptions{})
	require.NoError(t, err)

	settings := resolvedByRef(resolved)
	require.Equal(t, "desired://shared/-/targets/cobona/manifest", settings["cobona:manifest.file"].DesiredURI)
	require.Equal(t, filepath.Join("desired", "shared", "-", "targets", "cobona", "manifest.yaml"), settings["cobona:manifest.file"].DesiredRelPath)
	require.Equal(t, "desired://shared/-/targets/cobona/artifacts/preferences.json", settings["cobona:exported.preferences"].DesiredURI)
	require.Equal(t, filepath.Join("desired", "shared", "-", "targets", "cobona", "artifacts", "preferences.json"), settings["cobona:exported.preferences"].DesiredRelPath)
}

func TestResolveLayerOverrideReplacesFullSelectionDeterministically(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeV2Root(t, root, "default")
	writeStack(t, root, "default", []string{"base", "user"})
	writeLayer(t, root, "base", `
selections:
  git:
    settings:
      user.email:
        scope: user
        artifact: settings.yaml#base.email
`)
	writeLayer(t, root, "user", `
selections:
  git:
    settings:
      user.email:
        scope: shared
`)
	writeLayer(t, root, "overrides/local", `
selections:
  git:
    settings:
      user.email:
        scope: machine-user
`)

	resolved, err := Resolve(root, ResolveOptions{MachineID: "mbp", UserID: "leon", ExtraLayers: []string{"overrides/local"}})
	require.NoError(t, err)
	require.Equal(t, []string{"base", "user", "overrides/local"}, resolved.Layers)

	setting := resolvedByRef(resolved)["git:user.email"]
	require.Equal(t, "machine-user", setting.Scope)
	require.Equal(t, "overrides/local", setting.SourceLayer)
	require.Equal(t, "desired://machine-user/mbp/leon/targets/git/settings#user.email", setting.DesiredURI)
}

func TestResolveAllowsSharedSettingsFileWithDifferentFragments(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeV2Root(t, root, "default")
	writeStack(t, root, "default", []string{"global"})
	writeLayer(t, root, "global", `
selections:
  git:
    settings:
      user.email:
        scope: user
      user.name:
        scope: user
`)

	_, err := Resolve(root, ResolveOptions{MachineID: "mbp", UserID: "leon"})
	require.NoError(t, err)
}

func TestResolveRejectsDuplicateArtifactBindingByNormalizedURI(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeV2Root(t, root, "default")
	writeStack(t, root, "default", []string{"global"})
	writeLayer(t, root, "global", `
selections:
  git:
    settings:
      user.email:
        scope: user
        artifact: settings.yaml#same
      user.name:
        scope: user
        artifact: settings.yaml#same
`)

	_, err := Resolve(root, ResolveOptions{MachineID: "mbp", UserID: "leon"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate artifact binding")
}

func TestResolveRejectsUnsafeAndNonCanonicalDesiredArtifactPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		artifact string
	}{
		{name: "escape", artifact: "../escape.yaml"},
		{name: "absolute", artifact: "/tmp/escape.yaml"},
		{name: "backslash", artifact: `artifacts\\escape.yaml`},
		{name: "dot segment", artifact: "artifacts/./escape.yaml"},
		{name: "non canonical root file", artifact: "custom.yaml"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeV2Root(t, root, "default")
			writeStack(t, root, "default", []string{"global"})
			writeLayer(t, root, "global", `
selections:
  git:
    settings:
      user.email:
        scope: user
        artifact: `+tc.artifact+`
`)

			_, err := Resolve(root, ResolveOptions{MachineID: "mbp", UserID: "leon"})
			require.Error(t, err)
		})
	}
}

func TestResolveRejectsUnsupportedArtifactFragments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		artifact string
		wantErr  string
	}{
		{name: "manifest fragment", artifact: `"manifest.yaml#metadata"`, wantErr: "manifest artifact"},
		{name: "payload fragment", artifact: `"artifacts/preferences.json#user.email"`, wantErr: "artifact payload path"},
		{name: "empty settings fragment", artifact: `"settings.yaml#"`, wantErr: "artifact fragment must not be empty"},
		{name: "invalid settings fragment", artifact: `"settings.yaml#User.Email"`, wantErr: "invalid artifact fragment id"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeV2Root(t, root, "default")
			writeStack(t, root, "default", []string{"global"})
			writeLayer(t, root, "global", `
selections:
  git:
    settings:
      user.email:
        scope: user
        artifact: `+tc.artifact+`
`)

			_, err := Resolve(root, ResolveOptions{MachineID: "mbp", UserID: "leon"})
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestResolveRejectsInvalidIDsAndMissingSubjects(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeV2Root(t, root, "default")
	writeStack(t, root, "default", []string{"global"})
	writeLayer(t, root, "global", `
selections:
  Git:
    settings:
      user.email:
        scope: user
`)

	_, err := Resolve(root, ResolveOptions{MachineID: "mbp", UserID: "leon"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid target id")

	root = t.TempDir()
	writeV2Root(t, root, "default")
	writeStack(t, root, "default", []string{"global"})
	writeLayer(t, root, "global", `
selections:
  git:
    settings:
      user.email:
        scope: user
`)
	_, err = Resolve(root, ResolveOptions{MachineID: "mbp"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "user id required")
}

func TestResolveRejectsRootAndProfileShapeErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T, root string)
		wantErr string
	}{
		{
			name: "root unknown field",
			setup: func(t *testing.T, root string) {
				writeFile(t, filepath.Join(root, RootConfigFile), "schema: dotfiles-manager.v2.root-config\nschemaVersion: 1\nactiveProfileStack: default\nunexpected: true\n")
			},
			wantErr: "field unexpected",
		},
		{
			name: "root wrong schema",
			setup: func(t *testing.T, root string) {
				writeFile(t, filepath.Join(root, RootConfigFile), "schema: dotfiles-manager.v2.other\nschemaVersion: 1\nactiveProfileStack: default\n")
			},
			wantErr: "invalid root config schema",
		},
		{
			name: "root wrong schema version",
			setup: func(t *testing.T, root string) {
				writeFile(t, filepath.Join(root, RootConfigFile), "schema: dotfiles-manager.v2.root-config\nschemaVersion: 2\nactiveProfileStack: default\n")
			},
			wantErr: "invalid root config schemaVersion",
		},
		{
			name: "stack wrong schema",
			setup: func(t *testing.T, root string) {
				writeV2Root(t, root, "default")
				writeFile(t, filepath.Join(root, "profiles", "stacks", "default.yaml"), "schema: dotfiles-manager.v2.other\nschemaVersion: 1\nprofileStack:\n  - global\n")
			},
			wantErr: "invalid profile stack schema",
		},
		{
			name: "layer wrong schema version",
			setup: func(t *testing.T, root string) {
				writeV2Root(t, root, "default")
				writeStack(t, root, "default", []string{"global"})
				writeFile(t, filepath.Join(root, "profiles", "layers", "global.yaml"), "schema: dotfiles-manager.v2.profile-layer\nschemaVersion: 2\nselections: {}\n")
			},
			wantErr: "invalid profile layer schemaVersion",
		},
		{
			name: "layer unknown setting field",
			setup: func(t *testing.T, root string) {
				writeV2Root(t, root, "default")
				writeStack(t, root, "default", []string{"global"})
				writeLayer(t, root, "global", `
selections:
  git:
    settings:
      user.email:
        scope: user
        unexpected: true
`)
			},
			wantErr: "field unexpected",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			tc.setup(t, root)

			_, err := Resolve(root, ResolveOptions{MachineID: "mbp", UserID: "leon"})
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestResolveRejectsInvalidRootsAndProfileLayerPaths(t *testing.T) {
	t.Parallel()

	_, err := Resolve("", ResolveOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "v2 repo root is required")

	fileRoot := filepath.Join(t.TempDir(), "not-a-directory")
	writeFile(t, fileRoot, "content")
	_, err = Resolve(fileRoot, ResolveOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not a directory")

	missingMarkerRoot := t.TempDir()
	_, err = Resolve(missingMarkerRoot, ResolveOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing dotfiles-manager.v2.yaml")

	emptyStackRoot := t.TempDir()
	writeV2Root(t, emptyStackRoot, "default")
	writeStack(t, emptyStackRoot, "default", nil)
	_, err = Resolve(emptyStackRoot, ResolveOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "has no layers")

	absoluteStackRoot := t.TempDir()
	writeV2Root(t, absoluteStackRoot, "/absolute")
	_, err = Resolve(absoluteStackRoot, ResolveOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "active profile stack")

	badLayerRoot := t.TempDir()
	writeV2Root(t, badLayerRoot, "default")
	writeStack(t, badLayerRoot, "default", []string{"../escape"})
	_, err = Resolve(badLayerRoot, ResolveOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "profile layer")

	extraLayerRoot := t.TempDir()
	writeV2Root(t, extraLayerRoot, "default")
	writeStack(t, extraLayerRoot, "default", []string{"global"})
	writeLayer(t, extraLayerRoot, "global", "selections: {}\n")
	_, err = Resolve(extraLayerRoot, ResolveOptions{ExtraLayers: []string{"../escape"}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "profile layer")
}

func TestResolveRejectsPublicIDsWithSurroundingWhitespace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		selectionYAML string
		wantErr       string
	}{
		{
			name: "target id",
			selectionYAML: `
selections:
  " git ":
    settings:
      user.email:
        scope: user
`,
			wantErr: "invalid target id",
		},
		{
			name: "setting id",
			selectionYAML: `
selections:
  git:
    settings:
      " user.email ":
        scope: user
`,
			wantErr: "invalid setting id",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeV2Root(t, root, "default")
			writeStack(t, root, "default", []string{"global"})
			writeLayer(t, root, "global", tc.selectionYAML)

			_, err := Resolve(root, ResolveOptions{MachineID: "mbp", UserID: "leon"})
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func resolvedByRef(profile *ResolvedProfile) map[string]ResolvedSetting {
	out := make(map[string]ResolvedSetting, len(profile.Settings))
	for _, setting := range profile.Settings {
		out[setting.Ref()] = setting
	}
	return out
}

func writeV2Root(t *testing.T, root string, activeStack string) {
	t.Helper()
	writeFile(t, filepath.Join(root, RootConfigFile), "schema: dotfiles-manager.v2.root-config\nschemaVersion: 1\nactiveProfileStack: "+activeStack+"\n")
}

func writeStack(t *testing.T, root string, id string, layers []string) {
	t.Helper()
	body := "schema: dotfiles-manager.v2.profile-stack\nschemaVersion: 1\nprofileStack:\n"
	for _, layer := range layers {
		body += "  - " + layer + "\n"
	}
	writeFile(t, filepath.Join(root, "profiles", "stacks", filepath.FromSlash(id)+".yaml"), body)
}

func writeLayer(t *testing.T, root string, id string, body string) {
	t.Helper()
	prefix := "schema: dotfiles-manager.v2.profile-layer\nschemaVersion: 1\n"
	writeFile(t, filepath.Join(root, "profiles", "layers", filepath.FromSlash(id)+".yaml"), prefix+body)
}

func writeFile(t *testing.T, path string, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}
