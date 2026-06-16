package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReleaseVersionMetadataScriptAcceptsEnrichedReleaseOutput(t *testing.T) {
	t.Parallel()

	binary := fakeVersionBinary(t, "dotfiles-manager version=0.2.0-rc.1 commit=cd127ba0969c07eba05916004547e0094303f9cb date=2026-06-15T18:06:25Z channel=prerelease provenance=goreleaser")
	cmd := exec.Command(releaseMetadataScriptPath(t),
		"--binary", binary,
		"--expected-version", "0.2.0-rc.1",
		"--expected-channel", "prerelease",
		"--expected-provenance", "goreleaser",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	require.Contains(t, string(out), "release version metadata ok")
}

func TestReleaseVersionMetadataScriptRejectsDevFallbacks(t *testing.T) {
	t.Parallel()

	binary := fakeVersionBinary(t, "dotfiles-manager version=dev commit=unknown date=unknown channel=dev provenance=unspecified")
	cmd := exec.Command(releaseMetadataScriptPath(t), "--binary", binary)
	out, err := cmd.CombinedOutput()
	require.Error(t, err)
	text := string(out)
	require.Contains(t, text, "version must not be dev/empty")
	require.Contains(t, text, "commit must be a non-unknown git SHA")
	require.Contains(t, text, "date must be a non-unknown UTC RFC3339 timestamp")
	require.Contains(t, text, "channel must not be dev/empty")
	require.Contains(t, text, "provenance must not be unspecified/empty")
}

func TestGoReleaserArchiveMetadataScriptChecksArchiveBinary(t *testing.T) {
	t.Parallel()

	goExe := goExecutable(t)
	binary := filepath.Join(t.TempDir(), "dotfiles-manager")
	buildCmd := exec.Command(goExe,
		"build",
		"-o", binary,
		"-ldflags", "-X github.com/shpoont/dotfiles-manager/internal/app.buildVersion=0.2.0 -X github.com/shpoont/dotfiles-manager/internal/app.buildCommit=cd127ba0969c07eba05916004547e0094303f9cb -X github.com/shpoont/dotfiles-manager/internal/app.buildDate=2026-06-15T18:06:25Z -X github.com/shpoont/dotfiles-manager/internal/app.buildChannel=stable -X github.com/shpoont/dotfiles-manager/internal/app.buildProvenance=goreleaser",
		filepath.Join(repoRoot(t), "cmd", "dotfiles-manager"),
	)
	buildOut, err := buildCmd.CombinedOutput()
	require.NoError(t, err, string(buildOut))

	payload := filepath.Join(t.TempDir(), "payload")
	require.NoError(t, os.Mkdir(payload, 0o755))
	require.NoError(t, os.Rename(binary, filepath.Join(payload, "dotfiles-manager")))

	dist := t.TempDir()
	archive := filepath.Join(dist, "dotfiles-manager_0.2.0_darwin_arm64.tar.gz")
	tarCmd := exec.Command("tar", "-czf", archive, "-C", payload, "dotfiles-manager")
	tarOut, err := tarCmd.CombinedOutput()
	require.NoError(t, err, string(tarOut))

	cmd := exec.Command(releaseArchiveMetadataScriptPath(t),
		dist,
		"--expected-version", "0.2.0",
		"--expected-channel", "stable",
		"--expected-provenance", "goreleaser",
	)
	cmd.Env = append(os.Environ(), "PATH="+filepath.Dir(goExe)+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	require.Contains(t, string(out), "checked 1 GoReleaser archive(s)")
}

func fakeVersionBinary(t *testing.T, output string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "dotfiles-manager")
	writeFakeVersionBinary(t, path, output)
	return path
}

func writeFakeVersionBinary(t *testing.T, path string, output string) {
	t.Helper()

	body := "#!/usr/bin/env sh\nprintf '%s\\n' '" + output + "'\n"
	require.NoError(t, os.WriteFile(path, []byte(body), 0o755))
}

func releaseMetadataScriptPath(t *testing.T) string {
	t.Helper()

	path := filepath.Join("..", "..", "scripts", "ci", "check-release-version-metadata.sh")
	abs, err := filepath.Abs(path)
	require.NoError(t, err)
	return abs
}

func releaseArchiveMetadataScriptPath(t *testing.T) string {
	t.Helper()

	path := filepath.Join("..", "..", "scripts", "ci", "check-goreleaser-archive-version-metadata.sh")
	abs, err := filepath.Abs(path)
	require.NoError(t, err)
	return abs
}

func repoRoot(t *testing.T) string {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	return root
}

func goExecutable(t *testing.T) string {
	t.Helper()

	if path, err := exec.LookPath("go"); err == nil {
		return path
	}
	if home := os.Getenv("HOME"); home != "" {
		path := filepath.Join(home, ".asdf", "shims", "go")
		if info, err := os.Stat(path); err == nil && info.Mode().Perm()&0o111 != 0 {
			return path
		}
	}
	t.Fatal("go executable not found")
	return ""
}
