package app

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestImportCommandJSONEnvelope(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := setTempHome(t)
	setCWD(t, projectDir)

	writeConfig(t, projectDir, []byte(`syncs:
  - target: .config/nvim
    source: source/nvim
`))

	payload := runJSONCommand(t, []string{"import", "--json"})
	require.Equal(t, true, payload["ok"])
	require.Equal(t, "import", payload["command"])
	require.Equal(t, false, payload["dry_run"])

	pathScope := payload["path_scope"].(map[string]any)
	require.Contains(t, pathScope, "input")
	require.Contains(t, pathScope, "normalized")
	require.Equal(t, []any{float64(0)}, pathScope["matched_sync_indexes"])

	syncs := payload["syncs"].([]any)
	require.Len(t, syncs, 1)
	sync := syncs[0].(map[string]any)
	require.Equal(t, homeDir+"/.config/nvim", sync["target_root"])
	require.Equal(t, "sync[0] target=~/.config/nvim source=./source/nvim", sync["sync"])
	require.Contains(t, sync, "operations")
	require.Contains(t, sync, "counts")

	summary := payload["summary"].(map[string]any)
	require.Equal(t, float64(1), summary["sync_count"])
	require.Equal(t, float64(0), summary["update_managed_count"])
	require.Equal(t, float64(0), summary["add_unmanaged_count"])
	require.Equal(t, float64(0), summary["remove_missing_count"])
}
