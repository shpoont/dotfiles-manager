package app

import "testing"

import "github.com/stretchr/testify/require"

func TestOperationPhaseAliasMappings(t *testing.T) {
	t.Parallel()

	require.Equal(t, "deploy", operationPhaseAlias("copy"))
	require.Equal(t, "import", operationPhaseAlias("update_managed"))
	require.Equal(t, "incoming_unmanaged", operationPhaseAlias("add_unmanaged"))
	require.Equal(t, "remove_unmanaged", operationPhaseAlias("remove_unmanaged"))
}

func TestPhaseHeaderAliasMappings(t *testing.T) {
	t.Parallel()

	require.Equal(t, "deploy", phaseHeaderAlias("copy"))
	require.Equal(t, "import", phaseHeaderAlias("update-managed"))
	require.Equal(t, "incoming-unmanaged", phaseHeaderAlias("add-unmanaged"))
	require.Equal(t, "", phaseHeaderAlias("remove-unmanaged"))
}
