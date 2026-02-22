package app

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegressionSelectedDefaults(t *testing.T) {
	t.Run("json error policy stdout only", func(t *testing.T) {
		projectDir := t.TempDir()
		setTempHome(t)
		setCWD(t, projectDir)
		customConfig := filepath.Join(projectDir, "custom-config.yaml")

		cmd := NewRootCmd()
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		cmd.SetOut(&stdout)
		cmd.SetErr(&stderr)
		cmd.SetArgs([]string{"status", "--json", "--dry-run", "--config", customConfig})

		err := cmd.Execute()
		require.Error(t, err)
		require.Empty(t, stderr.String())

		var payload map[string]any
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
		require.Equal(t, false, payload["ok"])
		require.Equal(t, customConfig, payload["config_path"])
		require.Equal(t, "DFM_FLAG_UNSUPPORTED", payload["error"].(map[string]any)["code"])
	})

	t.Run("diff legend remains explicit two-line block", func(t *testing.T) {
		diffOutput := buildTextOutput("diff", false, map[string]any{
			"syncs": []any{
				map[string]any{
					"sync":       "sync[0] target=~/.config/nvim source=./source/nvim",
					"operations": []any{},
				},
			},
			"summary": map[string]any{},
		})

		intentLine := "legend intent: deploy applies source -> target; import applies target -> source"
		orientationLine := "legend patch-orientation: deploy-diff compares target -> source; import-diff compares source -> target"
		require.Equal(t, 1, strings.Count(diffOutput, intentLine))
		require.Equal(t, 1, strings.Count(diffOutput, orientationLine))
	})

	t.Run("scoped summaries expose excluded sync count", func(t *testing.T) {
		_, _, scopePath := setupScopedSummaryFixture(t)

		payload := runJSONCommand(t, []string{"status", "--json", scopePath})
		summary := payload["summary"].(map[string]any)
		require.Equal(t, float64(1), summary["excluded_sync_count"])
	})

	t.Run("omitted directory metric uses scanned entry maps", func(t *testing.T) {
		sourceEntries := map[string]statusEntry{
			"lua":          {path: "lua", typeID: "dir"},
			"lua/init.lua": {path: "lua/init.lua", typeID: "file"},
		}
		targetEntries := map[string]statusEntry{
			"lua":             {path: "lua", typeID: "dir"},
			"lua/plugins.lua": {path: "lua/plugins.lua", typeID: "file"},
		}

		require.Equal(t, 2, omittedEntryCountForPath("lua", sourceEntries, targetEntries))
	})

	t.Run("version output is a single enriched provenance line", func(t *testing.T) {
		oldVersion := buildVersion
		oldCommit := buildCommit
		oldDate := buildDate
		oldChannel := buildChannel
		oldProvenance := buildProvenance
		t.Cleanup(func() {
			buildVersion = oldVersion
			buildCommit = oldCommit
			buildDate = oldDate
			buildChannel = oldChannel
			buildProvenance = oldProvenance
		})

		buildVersion = "2.0.0"
		buildCommit = "deadbee"
		buildDate = "2026-02-22T12:34:56Z"
		buildChannel = "stable"
		buildProvenance = "ci"

		cmd := NewRootCmd()
		var stdout bytes.Buffer
		cmd.SetOut(&stdout)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"version"})
		require.NoError(t, cmd.Execute())

		out := stdout.String()
		require.Equal(t, 1, strings.Count(out, "\n"))
		require.Contains(t, out, "version=2.0.0")
		require.Contains(t, out, "commit=deadbee")
		require.Contains(t, out, "date=2026-02-22T12:34:56Z")
		require.Contains(t, out, "channel=stable")
		require.Contains(t, out, "provenance=ci")
	})
}
