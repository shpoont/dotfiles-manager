package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/shpoont/dotfiles-manager/internal/config"
	"github.com/shpoont/dotfiles-manager/internal/dfmerr"
	"github.com/stretchr/testify/require"
)

func TestBuildSummaryBranches(t *testing.T) {
	t.Parallel()

	deploy := buildSummary("deploy", 1)
	require.Contains(t, deploy, "copy_count")

	importSummary := buildSummary("import", 3)
	require.Contains(t, importSummary, "update_managed_count")

	diffSummary := buildSummary("diff", 2)
	require.Contains(t, diffSummary, "unified_patch_count")

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
	require.Contains(t, status[0].(map[string]any), "operations")

	deploy := buildSyncPayloads("deploy", selections)
	require.Len(t, deploy, 1)
	require.Contains(t, deploy[0].(map[string]any), "operations")

	importPayload := buildSyncPayloads("import", selections)
	require.Len(t, importPayload, 1)
	require.Contains(t, importPayload[0].(map[string]any), "operations")

	diffPayload := buildSyncPayloads("diff", selections)
	require.Len(t, diffPayload, 1)
	require.Contains(t, diffPayload[0].(map[string]any), "operations")

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

	all, err := selectSyncs(cfg, cfgPath, nil, nil)
	require.NoError(t, err)
	require.Len(t, all, 2)
	require.Equal(t, "", all[0].ScopePrefix)

	home, err := os.UserHomeDir()
	require.NoError(t, err)
	scopedPath := filepath.Join(home, ".config", "nvim", "lua")
	scoped, err := selectSyncs(cfg, cfgPath, scopedPath, scopedPath)
	require.NoError(t, err)
	require.Len(t, scoped, 1)
	require.Equal(t, 0, scoped[0].Index)
	require.Equal(t, "lua", scoped[0].ScopePrefix)

	none, err := selectSyncs(cfg, cfgPath, "~/.config/does-not-match", filepath.Join(home, ".config", "does-not-match"))
	require.Nil(t, none)
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeScopeNoMatch, dfmerr.MustCode(err))
}

func TestSelectSyncsMultipleMatches(t *testing.T) {
	t.Parallel()

	cfgPath := filepath.Join(t.TempDir(), ".dotfiles-manager.yaml")
	cfg := &config.Config{Syncs: []config.Sync{
		{Target: ".config/nvim", Source: "source/base"},
		{Target: ".config/nvim/lua", Source: "source/lua"},
	}}

	home, err := os.UserHomeDir()
	require.NoError(t, err)

	scopePath := filepath.Join(home, ".config", "nvim", "lua")
	selected, err := selectSyncs(cfg, cfgPath, scopePath, scopePath)
	require.NoError(t, err)
	require.Len(t, selected, 2)
	require.Equal(t, 0, selected[0].Index)
	require.Equal(t, "lua", selected[0].ScopePrefix)
	require.Equal(t, 1, selected[1].Index)
	require.Equal(t, "", selected[1].ScopePrefix)
}

func TestSelectSyncsExpandsEnvPaths(t *testing.T) {
	t.Setenv("DFM_TEST_HOST_ENV", "host-a")
	t.Setenv("DFM_TEST_USER_ENV", "alice")

	cfgPath := filepath.Join(t.TempDir(), ".dotfiles-manager.yaml")
	cfg := &config.Config{Syncs: []config.Sync{
		{Target: ".config/$DFM_TEST_HOST_ENV", Source: "./$DFM_TEST_USER_ENV/global"},
	}}

	selected, err := selectSyncs(cfg, cfgPath, nil, nil)
	require.NoError(t, err)
	require.Len(t, selected, 1)

	home, err := os.UserHomeDir()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, ".config", "host-a"), selected[0].TargetRoot)
	require.Equal(t, filepath.Join(filepath.Dir(cfgPath), "alice", "global"), selected[0].SourceRoot)
}

func TestSelectSyncsRejectsMissingEnvPath(t *testing.T) {
	t.Setenv("DFM_TEST_EMPTY_ENV", "")

	cfgPath := filepath.Join(t.TempDir(), ".dotfiles-manager.yaml")
	cfg := &config.Config{Syncs: []config.Sync{
		{Target: ".config/nvim", Source: "./$DFM_TEST_EMPTY_ENV/global"},
	}}

	selected, err := selectSyncs(cfg, cfgPath, nil, nil)
	require.Nil(t, selected)
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeConfigPathEnvUndefined, dfmerr.MustCode(err))
}

func TestIsWithinTarget(t *testing.T) {
	t.Parallel()
	target := filepath.Join("/tmp", "root")
	require.True(t, isWithinTarget(target, target))
	require.True(t, isWithinTarget(filepath.Join(target, "child"), target))
	require.False(t, isWithinTarget(filepath.Join("/tmp", "other"), target))
}

func TestBuildTextSummaryLineAndSummaryInt(t *testing.T) {
	t.Parallel()

	status := buildTextSummaryLine("status", false, map[string]any{
		"sync_count":               2,
		"deploy_count":             3,
		"import_count":             0,
		"incoming_unmanaged_count": 5,
		"remove_unmanaged_count":   0,
		"remove_missing_count":     7,
	})
	require.Equal(t, "summary deploy=3 incoming-unmanaged=5 remove-missing=7", status)

	deploy := buildTextSummaryLine("deploy", true, map[string]any{
		"sync_count":             float64(1),
		"copy_count":             int64(2),
		"remove_unmanaged_count": 0,
	})
	require.Equal(t, "summary dry-run=true copied=2", deploy)

	importLine := buildTextSummaryLine("import", false, map[string]any{
		"sync_count":           1,
		"update_managed_count": 2,
		"add_unmanaged_count":  0,
		"remove_missing_count": 4,
	})
	require.Equal(t, "summary dry-run=false updated-managed=2 removed-missing=4", importLine)

	require.Equal(t, "summary", buildTextSummaryLine("status", false, map[string]any{}))
	require.Equal(t, "summary dry-run=false", buildTextSummaryLine("deploy", false, map[string]any{}))
	require.Equal(t, "summary", buildTextSummaryLine("diff", false, map[string]any{}))

	unknown := buildTextSummaryLine("unknown", false, map[string]any{"sync_count": 9})
	require.Equal(t, "summary syncs=9", unknown)

	require.Equal(t, 0, summaryInt(nil, "missing"))
	require.Equal(t, 0, summaryInt(map[string]any{"x": "y"}, "x"))
}

func TestErrorSummary(t *testing.T) {
	t.Parallel()

	plain := errorSummary(assertAnError{})
	require.Equal(t, map[string]any{}, plain)

	partial := dfmerr.New(dfmerr.CodeIOWrite, "write failed", map[string]any{"partial": true})
	require.Equal(t, map[string]any{"partial": true}, errorSummary(partial))
}

func TestExtractPartialResult(t *testing.T) {
	t.Parallel()

	syncs, summary := extractPartialResult(errors.New("plain"))
	require.Nil(t, syncs)
	require.Nil(t, summary)

	partialErr := newPartialCommandError(
		dfmerr.New(dfmerr.CodeIOWrite, "write failed", nil),
		[]any{map[string]any{"sync_index": 0}},
		map[string]any{"sync_count": 1},
	)

	syncs, summary = extractPartialResult(partialErr)
	require.Len(t, syncs, 1)
	require.Equal(t, 1, summary["sync_count"])
}

type assertAnError struct{}

func (assertAnError) Error() string { return "x" }
