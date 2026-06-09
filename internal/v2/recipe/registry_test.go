package recipe

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBundledRegistryLookupAliasesAndDeterministicList(t *testing.T) {
	t.Parallel()

	registry := DefaultBundledRegistry()

	target, ok := registry.Lookup(GitTarget)
	require.True(t, ok)
	require.Equal(t, GitTarget, target.ID)
	require.Equal(t, RecipeSourceBundled, target.Source)
	require.Equal(t, "recipe://bundled/git", target.RecipeRef)
	require.Equal(t, "trusted", target.TrustStatus)
	require.Equal(t, []string{"gitconfig"}, target.Aliases)

	target, ok = registry.Lookup("gitconfig")
	require.True(t, ok)
	require.Equal(t, GitTarget, target.ID)

	target, ok = registry.Lookup("neovim")
	require.True(t, ok)
	require.Equal(t, NvimTarget, target.ID)
	require.Equal(t, "linux-darwin", target.PlatformSupport)

	target, ok = registry.Lookup(ZshTarget)
	require.True(t, ok)
	require.Equal(t, ZshTarget, target.ID)
	require.Empty(t, target.Aliases)

	target, ok = registry.Lookup(TmuxTarget)
	require.True(t, ok)
	require.Equal(t, TmuxTarget, target.ID)
	require.Equal(t, "linux-darwin", target.PlatformSupport)
	require.Empty(t, target.Aliases)

	target, ok = registry.Lookup("openssh")
	require.True(t, ok)
	require.Equal(t, SSHTarget, target.ID)
	require.Equal(t, []string{"openssh"}, target.Aliases)

	_, ok = registry.Lookup("zshrc")
	require.False(t, ok)

	targets := registry.List()
	require.Len(t, targets, 7)
	require.Equal(t, CustomFilesTarget, targets[0].ID)
	require.Equal(t, GitTarget, targets[1].ID)
	require.Equal(t, NvimTarget, targets[2].ID)
	require.Equal(t, SSHTarget, targets[3].ID)
	require.Equal(t, StarshipTarget, targets[4].ID)
	require.Equal(t, TmuxTarget, targets[5].ID)
	require.Equal(t, ZshTarget, targets[6].ID)
	require.Equal(t, "recipe://bundled/nvim", targets[2].RecipeRef)
	require.Equal(t, "linux-darwin", targets[2].PlatformSupport)
	require.Equal(t, "recipe://bundled/ssh", targets[3].RecipeRef)
	require.Equal(t, "linux-darwin", targets[3].PlatformSupport)
	require.Equal(t, "recipe://bundled/starship", targets[4].RecipeRef)
	require.Equal(t, "unknown", targets[4].PlatformSupport)
	require.Equal(t, "recipe://bundled/tmux", targets[5].RecipeRef)
	require.Equal(t, "linux-darwin", targets[5].PlatformSupport)
	require.Equal(t, "recipe://bundled/zsh", targets[6].RecipeRef)
	require.Equal(t, []string{CustomFilesTarget, GitTarget, NvimTarget, SSHTarget, StarshipTarget, TmuxTarget, ZshTarget}, KnownBundledTargetIDs())
}

func TestBundledRegistryRejectsUnsafeOrAmbiguousAliases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		targets []BundledTarget
		wantErr string
	}{
		{
			name: "duplicate target id",
			targets: []BundledTarget{
				{ID: "git", DisplayName: "Git"},
				{ID: "git", DisplayName: "Git duplicate"},
			},
			wantErr: "duplicate bundled target id",
		},
		{
			name: "alias repeats canonical",
			targets: []BundledTarget{
				{ID: "git", Aliases: []string{"git"}},
			},
			wantErr: "alias must not repeat canonical id",
		},
		{
			name: "alias collides with canonical",
			targets: []BundledTarget{
				{ID: "git", Aliases: []string{"shell"}},
				{ID: "shell"},
			},
			wantErr: "collides with a canonical target id",
		},
		{
			name: "alias collides with alias",
			targets: []BundledTarget{
				{ID: "git", Aliases: []string{"config"}},
				{ID: "nvim", Aliases: []string{"config"}},
			},
			wantErr: "collides between targets",
		},
		{
			name: "invalid id",
			targets: []BundledTarget{
				{ID: "BadTarget"},
			},
			wantErr: "invalid target id",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := newBundledRegistry(tc.targets)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestRecipeListReportIsStableMetadataOnly(t *testing.T) {
	t.Parallel()

	report := List(ListOptions{})
	require.Equal(t, ListCommand, report.Command)
	require.Equal(t, "ok", report.Summary.Status)
	require.Len(t, report.RecipeList.Targets, 7)
	require.Equal(t, CustomFilesTarget, report.RecipeList.Targets[0].ID)
	require.Equal(t, GitTarget, report.RecipeList.Targets[1].ID)
	require.Equal(t, NvimTarget, report.RecipeList.Targets[2].ID)
	require.Equal(t, SSHTarget, report.RecipeList.Targets[3].ID)
	require.Equal(t, StarshipTarget, report.RecipeList.Targets[4].ID)
	require.Equal(t, TmuxTarget, report.RecipeList.Targets[5].ID)
	require.Equal(t, ZshTarget, report.RecipeList.Targets[6].ID)
	require.Equal(t, RecipeSourceBundled, report.RecipeList.Targets[1].Source)
	require.Equal(t, "trusted", report.RecipeList.Targets[1].TrustStatus)
	require.Equal(t, []string{"gitconfig"}, report.RecipeList.Targets[1].Aliases)

	payload, err := ListJSON(report)
	require.NoError(t, err)
	require.Contains(t, payload, `"command": "recipe.list"`)
	require.Contains(t, payload, `"recipeList"`)
	require.NotContains(t, payload, "secret@example.com")

	text := ListText(report)
	require.Contains(t, text, "recipe list")
	require.Contains(t, text, "custom.files source=bundled")
	require.Contains(t, text, "git source=bundled")
	require.Contains(t, text, "nvim source=bundled")
	require.Contains(t, text, "ssh source=bundled")
	require.Contains(t, text, "starship source=bundled")
	require.Contains(t, text, "tmux source=bundled")
	require.Contains(t, text, "zsh source=bundled")
	require.Contains(t, text, "aliases=gitconfig")
	require.True(t, strings.Index(text, "custom.files") < strings.Index(text, "git source=bundled"))
	require.True(t, strings.Index(text, "git source=bundled") < strings.Index(text, "ssh source=bundled"))
	require.True(t, strings.Index(text, "ssh source=bundled") < strings.Index(text, "starship source=bundled"))
	require.True(t, strings.Index(text, "starship source=bundled") < strings.Index(text, "tmux source=bundled"))
	require.True(t, strings.Index(text, "tmux source=bundled") < strings.Index(text, "zsh source=bundled"))

	nilJSON, err := ListJSON(nil)
	require.NoError(t, err)
	require.Contains(t, nilJSON, `"status": "error"`)
	require.Contains(t, ListText(nil), "summary status=error targets=0")
	require.Equal(t, "-", aliasesText(nil))
}

func TestRecipeListWarnsOnLocalAliasCollision(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeNamedRecipe(t, root, "gitconfig", `schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: gitconfig
displayName: Local Git Alias Collision
supportLevel: experimental
capability: read-write
locations:
  home:
    default: "~"
settings:
  credential.helper:
    scopeDefault: user
    resource: helper
resources:
  helper:
    driver: file
    location: home
    path: .gitconfig
`)

	report := List(ListOptions{RepoRoot: root})
	requireDiagnosticCodeInList(t, report, ExplainCodeLocalRecipeShadowed)
	require.Contains(t, ListText(report), "diagnostics:")
}

func requireDiagnosticCodeInList(t *testing.T, report *ListReport, code string) {
	t.Helper()
	for _, diagnostic := range report.RecipeList.Diagnostics {
		if diagnostic.Code == code {
			return
		}
	}
	require.Failf(t, "diagnostic code missing", "expected diagnostic code %q in %#v", code, report.RecipeList.Diagnostics)
}
