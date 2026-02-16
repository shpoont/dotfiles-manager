package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shpoont/dotfiles-manager/internal/config"
	"github.com/shpoont/dotfiles-manager/internal/dfmerr"
	"github.com/stretchr/testify/require"
)

func TestBuildSummaryBranches(t *testing.T) {
	t.Parallel()

	status := buildSummary("status", 2)
	require.Equal(t, 2, status["sync_count"])
	require.Contains(t, status, "deploy_change_count")

	deploy := buildSummary("deploy", 1)
	require.Contains(t, deploy, "copied_count")

	importSummary := buildSummary("import", 3)
	require.Contains(t, importSummary, "updated_manifest_count")

	fallback := buildSummary("unknown", 4)
	require.Equal(t, 4, fallback["sync_count"])
	require.Len(t, fallback, 1)
}

func TestBuildSyncPayloadBranches(t *testing.T) {
	t.Parallel()

	selections := []syncSelection{{
		Index:       0,
		SourceRoot:  "/tmp/source",
		TargetRoot:  "/tmp/target",
		ScopePrefix: "lua",
	}}

	status := buildSyncPayloads("status", selections)
	require.Len(t, status, 1)
	require.Contains(t, status[0].(map[string]any), "deploy_changes")

	deploy := buildSyncPayloads("deploy", selections)
	require.Len(t, deploy, 1)
	require.Contains(t, deploy[0].(map[string]any), "copied")

	importPayload := buildSyncPayloads("import", selections)
	require.Len(t, importPayload, 1)
	require.Contains(t, importPayload[0].(map[string]any), "updated_manifest")

	fallback := buildSyncPayloads("unknown", selections)
	require.Len(t, fallback, 1)
	require.Contains(t, fallback[0].(map[string]any), "sync_index")
}

func TestSliceOrEmptyBranches(t *testing.T) {
	t.Parallel()
	require.Equal(t, []int{}, sliceOrEmpty(nil))
	require.Equal(t, []int{1, 2}, sliceOrEmpty([]int{1, 2}))
}

func TestNormalizeScopePathBranches(t *testing.T) {
	t.Parallel()

	input, normalized, err := normalizeScopePath("")
	require.NoError(t, err)
	require.Nil(t, input)
	require.Nil(t, normalized)

	input, normalized, err = normalizeScopePath("~/.config/nvim")
	require.NoError(t, err)
	require.Equal(t, "~/.config/nvim", input)
	require.NotNil(t, normalized)

	input, normalized, err = normalizeScopePath(".config/nvim")
	require.NoError(t, err)
	require.Equal(t, ".config/nvim", input)
	require.NotNil(t, normalized)
}

func TestSelectSyncsBranches(t *testing.T) {
	t.Parallel()

	cfgPath := filepath.Join(t.TempDir(), ".dotfiles-manager.yaml")
	cfg := &config.Config{Syncs: []config.Sync{
		{Target: ".config/nvim", Source: ".config/nvim"},
		{Target: ".config/zsh", Source: "zsh"},
	}}

	all, err := selectSyncs(cfg, cfgPath, nil)
	require.NoError(t, err)
	require.Len(t, all, 2)
	require.Equal(t, "", all[0].ScopePrefix)

	home, err := os.UserHomeDir()
	require.NoError(t, err)
	scopedPath := filepath.Join(home, ".config", "nvim", "lua")
	scoped, err := selectSyncs(cfg, cfgPath, scopedPath)
	require.NoError(t, err)
	require.Len(t, scoped, 1)
	require.Equal(t, 0, scoped[0].Index)
	require.Equal(t, "lua", scoped[0].ScopePrefix)

	none, err := selectSyncs(cfg, cfgPath, filepath.Join(home, ".config", "does-not-match"))
	require.Nil(t, none)
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeScopeNoMatch, dfmerr.MustCode(err))
}

func TestIsWithinTarget(t *testing.T) {
	t.Parallel()
	target := filepath.Join("/tmp", "root")
	require.True(t, isWithinTarget(target, target))
	require.True(t, isWithinTarget(filepath.Join(target, "child"), target))
	require.False(t, isWithinTarget(filepath.Join("/tmp", "other"), target))
}
