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
		"sync_count":                2,
		"deploy_change_count":       3,
		"import_change_count":       4,
		"incoming_unmanaged_count":  5,
		"removable_unmanaged_count": 6,
		"removable_missing_count":   7,
	})
	require.Equal(t, "status: syncs=2 deploy_changes=3 import_changes=4 incoming_unmanaged=5 removable_unmanaged=6 removable_missing=7", status)

	deploy := buildTextSummaryLine("deploy", true, map[string]any{
		"sync_count":              float64(1),
		"copied_count":            int64(2),
		"removed_unmanaged_count": 3,
	})
	require.Equal(t, "deploy (dry-run): syncs=1 copied=2 removed_unmanaged=3", deploy)

	importLine := buildTextSummaryLine("import", false, map[string]any{
		"sync_count":             1,
		"updated_manifest_count": 2,
		"added_unmanaged_count":  3,
		"removed_missing_count":  4,
	})
	require.Equal(t, "import: syncs=1 updated_manifest=2 added_unmanaged=3 removed_missing=4", importLine)

	unknown := buildTextSummaryLine("unknown", false, map[string]any{"sync_count": 9})
	require.Equal(t, "unknown: syncs=9", unknown)

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
