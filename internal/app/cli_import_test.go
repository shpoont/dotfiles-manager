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
	require.Contains(t, sync, "updated_manifest")
	require.Contains(t, sync, "added_unmanaged")
	require.Contains(t, sync, "removed_missing")

	summary := payload["summary"].(map[string]any)
	require.Equal(t, float64(1), summary["sync_count"])
	require.Equal(t, float64(0), summary["updated_manifest_count"])
	require.Equal(t, float64(0), summary["added_unmanaged_count"])
	require.Equal(t, float64(0), summary["removed_missing_count"])
}
