package catalogdiscovery

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestListOfficialCatalogState(t *testing.T) {
	report := List()

	require.Equal(t, Schema, report.Schema)
	require.Equal(t, SchemaVersion, report.SchemaVersion)
	require.Equal(t, Command, report.Command)
	require.Equal(t, RunID, report.RunID)
	require.Equal(t, "ok", report.Summary.Status)
	require.Equal(t, 1, report.Summary.Catalogs)
	require.Empty(t, report.Diagnostics)
	require.Len(t, report.Catalogs, 1)

	official := report.Catalogs[0]
	require.Equal(t, "dotfiles-manager/official", official.ID)
	require.Equal(t, "active for discovery", official.State)
	require.Equal(t, "9f2c7a1", official.Version)
	require.Equal(t, "2026-06-30 18:00 UTC", official.Updated)
	require.Equal(t, "app/tool support", official.Purpose)

	text := Text(report)
	require.Contains(t, text, "Catalogs")
	require.Contains(t, text, "Catalogs define app/tool support; they do not store your settings.")
	require.Contains(t, text, "dotfiles-manager/official  active for discovery")
	require.Contains(t, text, "Catalog version: 9f2c7a1")
	require.Contains(t, text, "Catalog updated: 2026-06-30 18:00 UTC")
	require.NotContains(t, text, "Source:")
	require.NotContains(t, text, "Local copy:")
	require.NotContains(t, text, "Offline use:")
	require.NotContains(t, text, "Updates:")
	require.NotContains(t, text, "Removable:")
	require.NotContains(t, text, "catalog update")
	require.NotContains(t, text, "catalog add")

	payload, err := JSON(report)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal([]byte(payload), &decoded))
	require.Equal(t, Schema, decoded["schema"])
	require.Equal(t, Command, decoded["command"])
}

func TestNilReportRenderersAreStable(t *testing.T) {
	text := Text(nil)
	require.Contains(t, text, "Catalogs")
	require.Contains(t, text, "The command could not complete.")

	payload, err := JSON(nil)
	require.NoError(t, err)
	require.Contains(t, payload, `"status": "error"`)
}
