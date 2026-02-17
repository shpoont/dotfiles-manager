package app

import (
	"testing"

	"github.com/shpoont/dotfiles-manager/internal/config"
	"github.com/stretchr/testify/require"
)

func TestBuildTextOutputStatusAndDeploy(t *testing.T) {
	t.Parallel()

	statusOutput := buildTextOutput("status", false, map[string]any{
		"syncs": []any{
			map[string]any{
				"sync":         "sync[0] target=~/.config/nvim source=./source/nvim",
				"scope_prefix": "lua",
				"operations": []any{
					map[string]any{"phase": "deploy", "action": "can create", "path": "lua/init.lua", "source_type": "file", "target_type": "missing"},
					map[string]any{"phase": "import", "action": "can update", "path": "lua/init.lua", "source_type": "file", "target_type": "file"},
					map[string]any{"phase": "incoming_unmanaged", "action": "can add", "path": "lua/new.lua", "type": "file"},
				},
			},
		},
		"summary": map[string]any{
			"deploy_count":             1,
			"import_count":             1,
			"incoming_unmanaged_count": 1,
			"remove_unmanaged_count":   0,
			"remove_missing_count":     0,
			"operation_count":          3,
		},
	})

	require.Contains(t, statusOutput, "sync[0] target=~/.config/nvim source=./source/nvim scope=lua")
	require.Contains(t, statusOutput, "deploy[1]")
	require.Contains(t, statusOutput, "can create   lua/init.lua (file->missing)")
	require.Contains(t, statusOutput, "incoming-unmanaged[1]")
	require.Contains(t, statusOutput, "summary deploy=1 import=1 incoming-unmanaged=1")
	require.NotContains(t, statusOutput, "remove-unmanaged[0]")
	require.NotContains(t, statusOutput, "remove-missing[0]")

	deployOutput := buildTextOutput("deploy", true, map[string]any{
		"syncs": []any{
			map[string]any{
				"sync": "sync[0] target=~/.config/nvim source=./source/nvim",
				"operations": []any{
					map[string]any{"phase": "copy", "action": "update", "path": "lua/init.lua", "type": "file"},
					map[string]any{"phase": "remove_unmanaged", "action": "remove", "path": "lua/old.bak", "type": "file"},
				},
			},
		},
		"summary": map[string]any{
			"copy_count":             1,
			"remove_unmanaged_count": 1,
		},
	})

	require.Contains(t, deployOutput, "copy[1]")
	require.Contains(t, deployOutput, "remove-unmanaged[1]")
	require.Contains(t, deployOutput, "summary dry-run=true copied=1 remove-unmanaged=1")
}

func TestBuildTextOutputFallbackAndHelpers(t *testing.T) {
	t.Parallel()

	fallback := buildTextOutput("unknown", false, map[string]any{
		"summary": map[string]any{"sync_count": 2},
	})
	require.Equal(t, "summary syncs=2", fallback)

	require.Equal(t, "~/.config/nvim", renderTargetDisplay(".config/nvim"))
	require.Equal(t, "~", renderTargetDisplay("."))
	require.Equal(t, "~", renderTargetDisplay(""))

	require.Equal(t, "./source/nvim", renderSourceDisplay("source/nvim"))
	require.Equal(t, "./.config/nvim", renderSourceDisplay(".config/nvim"))
	require.Equal(t, "..", renderSourceDisplay(".."))

	display := buildSyncDisplay(2, config.Sync{Target: ".config/nvim", Source: "source/nvim"})
	require.Equal(t, "sync[2] target=~/.config/nvim source=./source/nvim", display.Label)
	require.Equal(t, "~/.config/nvim", display.Target)
	require.Equal(t, "./source/nvim", display.Source)
}
