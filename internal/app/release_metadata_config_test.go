package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGoReleaserConfigStampsReleaseVersionMetadata(t *testing.T) {
	t.Parallel()

	config := readRepoFile(t, ".goreleaser.yml")
	for _, variable := range []string{
		"buildVersion={{ .Version }}",
		"buildCommit={{ .FullCommit }}",
		"buildDate={{ .CommitDate }}",
		"buildChannel={{ if .IsSnapshot }}snapshot{{ else if .Prerelease }}prerelease{{ else }}stable{{ end }}",
		"buildProvenance=goreleaser",
	} {
		require.GreaterOrEqualf(t, strings.Count(config, variable), 2, "expected both linux and darwin builds to set %s", variable)
	}
}

func TestReleaseWorkflowsValidateAndDispatchVersionMetadata(t *testing.T) {
	t.Parallel()

	releaseWorkflow := readRepoFile(t, ".github/workflows/release.yml")
	for _, expected := range []string{
		"check-goreleaser-archive-version-metadata.sh dist",
		"--expected-provenance goreleaser",
		"release --clean --skip=publish",
		"Verify release archive version metadata before publish",
		"${GITHUB_REF_TYPE}",
		"source_commit",
		"source_date",
		"channel",
		"provenance",
		"git rev-list -n 1",
		"date -u -d",
		"homebrew-source",
	} {
		require.Contains(t, releaseWorkflow, expected)
	}

	dispatchWorkflow := readRepoFile(t, ".github/workflows/dispatch-homebrew-tap.yml")
	for _, expected := range []string{
		"source_commit",
		"source_date",
		"channel",
		"provenance",
		"git rev-list -n 1",
		"date -u -d",
		"homebrew-source",
	} {
		require.Contains(t, dispatchWorkflow, expected)
	}
}

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()

	path := filepath.Join("..", "..", rel)
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(contents)
}
