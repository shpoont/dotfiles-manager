package ledger

import (
	"testing"

	"github.com/shpoont/dotfiles-manager/internal/v2/macosdefaultsdriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/stretchr/testify/require"
)

func TestSelectedValueMacOSDefaultsReadOnlyLedgerMetadata(t *testing.T) {
	t.Parallel()

	require.Equal(t, MacOSDefaultsReadOnlySelectedDriverVersion, SelectedValueDriverVersion(recipe.MacOSDefaultsReadOnlyDriverID))
	require.Equal(t, macosdefaultsdriver.NormalizerID, SelectedValueNormalizer(recipe.MacOSDefaultsReadOnlyDriverID))
}
