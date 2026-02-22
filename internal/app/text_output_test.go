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
	require.Contains(t, statusOutput, "reminder: deploy applies source -> target; import applies target -> source")
	require.Contains(t, statusOutput, "deploy[1] (source -> target)")
	require.Contains(t, statusOutput, "import[1] (target -> source)")
	require.Contains(t, statusOutput, "hint: same path in deploy/import: lua/init.lua")
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
					map[string]any{"phase": "copy", "action": "update", "state": "planned", "path": "lua/init.lua", "type": "file"},
					map[string]any{"phase": "remove_unmanaged", "action": "remove", "state": "planned", "path": "lua/old.bak", "type": "file"},
				},
			},
		},
		"summary": map[string]any{
			"copy_count":             1,
			"remove_unmanaged_count": 1,
		},
	})

	require.Contains(t, deployOutput, "MODE: DRY RUN (no writes)")
	require.Contains(t, deployOutput, "copy[1]")
	require.Contains(t, deployOutput, "remove-unmanaged[1]")
	require.Contains(t, deployOutput, "[planned] update")
	require.Contains(t, deployOutput, "[planned] remove")
	require.Contains(t, deployOutput, "summary dry-run=true copied=1 remove-unmanaged=1")

	importOutput := buildTextOutput("import", false, map[string]any{
		"syncs": []any{
			map[string]any{
				"sync": "sync[0] target=~/.config/nvim source=./source/nvim",
				"operations": []any{
					map[string]any{"phase": "update_managed", "action": "update", "state": "applied", "path": "lua/init.lua", "type": "file"},
				},
			},
		},
		"summary": map[string]any{
			"update_managed_count": 1,
		},
	})
	require.Contains(t, importOutput, "MODE: APPLY (writes enabled)")
	require.Contains(t, importOutput, "[applied] update")
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

func TestBuildTextOutputStatusHintAppearsOnlyForOverlaps(t *testing.T) {
	t.Parallel()

	noOverlapOutput := buildTextOutput("status", false, map[string]any{
		"syncs": []any{
			map[string]any{
				"sync": "sync[0] target=~/.config/nvim source=./source/nvim",
				"operations": []any{
					map[string]any{"phase": "deploy", "action": "can update", "path": "lua/init.lua", "source_type": "file", "target_type": "file"},
					map[string]any{"phase": "import", "action": "can update", "path": "lua/plugins.lua", "source_type": "file", "target_type": "file"},
				},
			},
		},
		"summary": map[string]any{
			"deploy_count":    1,
			"import_count":    1,
			"operation_count": 2,
		},
	})

	require.Contains(t, noOverlapOutput, "deploy[1] (source -> target)")
	require.Contains(t, noOverlapOutput, "import[1] (target -> source)")
	require.NotContains(t, noOverlapOutput, "hint: same path in deploy/import:")
}

func TestBuildTextOutputDiff(t *testing.T) {
	t.Parallel()

	diffOutput := buildTextOutput("diff", false, map[string]any{
		"syncs": []any{
			map[string]any{
				"sync": "sync[0] target=~/.config/nvim source=./source/nvim",
				"operations": []any{
					map[string]any{
						"phase":       "deploy",
						"action":      "can update",
						"path":        "lua/init.lua",
						"source_type": "file",
						"target_type": "file",
						"diff_kind":   "unified",
						"patch":       "--- target/lua/init.lua\n+++ source/lua/init.lua\n@@ -1 +1 @@\n-old\n+new\n",
					},
					map[string]any{
						"phase":  "remove_unmanaged",
						"action": "can remove",
						"path":   "lua/cache.bin",
						"type":   "file",
						"reason": "binary differs",
					},
				},
			},
		},
		"summary": map[string]any{
			"deploy_count":           1,
			"remove_unmanaged_count": 1,
			"unified_patch_count":    1,
			"binary_count":           1,
		},
	})

	require.Contains(t, diffOutput, "legend intent: deploy applies source -> target; import applies target -> source")
	require.Contains(t, diffOutput, "legend patch-orientation: deploy-diff compares target -> source; import-diff compares source -> target")
	require.Contains(t, diffOutput, "deploy-diff[1] (target -> source)")
	require.Contains(t, diffOutput, "--- target/lua/init.lua")
	require.Contains(t, diffOutput, "remove-unmanaged[1] (target -> /dev/null)")
	require.Contains(t, diffOutput, "note: binary differs")
	require.Contains(t, diffOutput, "summary deploy-diff=1 remove-unmanaged=1 unified=1 binary=1")
}

func TestDiffNoteFallbacks(t *testing.T) {
	t.Parallel()

	require.Equal(t, "explicit reason", diffNote(map[string]any{"reason": "explicit reason", "diff_kind": "binary"}))
	require.Equal(t, "binary differs", diffNote(map[string]any{"diff_kind": "binary"}))
	require.Equal(t, "type differs", diffNote(map[string]any{"diff_kind": "type_change"}))
	require.Equal(t, "patch omitted", diffNote(map[string]any{"diff_kind": "omitted"}))
	require.Equal(t, "", diffNote(map[string]any{"diff_kind": "unified"}))
}

func TestAppendStatusDirectionHintTruncates(t *testing.T) {
	t.Parallel()

	deployOps := []map[string]any{
		{"path": "lua/a.lua"},
		{"path": "lua/b.lua"},
		{"path": "lua/c.lua"},
		{"path": "lua/d.lua"},
	}
	importOps := []map[string]any{
		{"path": "lua/d.lua"},
		{"path": "lua/a.lua"},
		{"path": "lua/c.lua"},
		{"path": "lua/b.lua"},
	}

	lines := appendStatusDirectionHint(nil, deployOps, importOps)
	require.Len(t, lines, 1)
	require.Equal(t, "hint: same path in deploy/import: lua/a.lua, lua/b.lua, lua/c.lua (+1 more)", lines[0])
}

func TestOperationPayloadMapsByPhaseSkipsInvalidEntries(t *testing.T) {
	t.Parallel()

	sync := map[string]any{
		"operations": []any{
			map[string]any{"phase": "deploy", "path": "lua/init.lua"},
			"not a map",
			map[string]any{"phase": "import", "path": "lua/plugin.lua"},
			map[string]any{"path": "lua/missing-phase.lua"},
		},
	}

	deploy := operationPayloadMapsByPhase(sync, "deploy")
	require.Len(t, deploy, 1)
	require.Equal(t, "lua/init.lua", deploy[0]["path"])
}

func TestAppendDiffPhaseBlockDefaultNote(t *testing.T) {
	t.Parallel()

	lines := appendDiffPhaseBlock(nil, "deploy-diff", "(source -> target)", []map[string]any{
		{
			"path":        "lua/init.lua",
			"action":      "can update",
			"source_type": "file",
			"target_type": "file",
		},
	})

	require.Contains(t, lines, "  note: patch unavailable")
}
