package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shpoont/dotfiles-manager/internal/dfmerr"
	"github.com/stretchr/testify/require"
)

func TestResolvePathPrecedenceExplicitOverEnvAndDefault(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	explicit := filepath.Join(dir, "explicit.yaml")
	envPath := filepath.Join(dir, "env.yaml")
	defaultPath := filepath.Join(dir, DefaultConfigFile)

	require.NoError(t, os.WriteFile(explicit, []byte("syncs: []\n"), 0o644))
	require.NoError(t, os.WriteFile(envPath, []byte("syncs: []\n"), 0o644))
	require.NoError(t, os.WriteFile(defaultPath, []byte("syncs: []\n"), 0o644))

	resolved, err := ResolvePath(ResolveOptions{
		ExplicitPath: explicit,
		CWD:          dir,
		Getenv: func(key string) string {
			if key == ConfigEnvVar {
				return envPath
			}
			return ""
		},
	})

	require.NoError(t, err)
	require.Equal(t, explicit, resolved)
}

func TestResolvePathEnvOverDefault(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	envPath := filepath.Join(dir, "env.yaml")
	defaultPath := filepath.Join(dir, DefaultConfigFile)
	require.NoError(t, os.WriteFile(envPath, []byte("syncs: []\n"), 0o644))
	require.NoError(t, os.WriteFile(defaultPath, []byte("syncs: []\n"), 0o644))

	resolved, err := ResolvePath(ResolveOptions{
		CWD: dir,
		Getenv: func(key string) string {
			if key == ConfigEnvVar {
				return envPath
			}
			return ""
		},
	})

	require.NoError(t, err)
	require.Equal(t, envPath, resolved)
}

func TestResolvePathDefaultFallback(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	defaultPath := filepath.Join(dir, DefaultConfigFile)
	require.NoError(t, os.WriteFile(defaultPath, []byte("syncs: []\n"), 0o644))

	resolved, err := ResolvePath(ResolveOptions{CWD: dir, Getenv: func(string) string { return "" }})
	require.NoError(t, err)
	require.Equal(t, defaultPath, resolved)
}

func TestResolvePathRequiredWhenMissing(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	_, err := ResolvePath(ResolveOptions{CWD: dir, Getenv: func(string) string { return "" }})
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeConfigRequired, dfmerr.MustCode(err))
}

func TestLoadUnknownKeyReturnsStableCode(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("syncs:\n  - target: .config/nvim\n    source: .config/nvim\n    bad-key: 1\n"), 0o644))

	_, err := Load(cfgPath)
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeConfigSchemaUnknownKey, dfmerr.MustCode(err))
}

func TestValidateRejectsAbsoluteAndEscapePaths(t *testing.T) {
	t.Parallel()

	err := Validate(&Config{Syncs: []Sync{{Target: "/abs", Source: ".config/nvim"}}})
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeConfigPathNotRelative, dfmerr.MustCode(err))

	err = Validate(&Config{Syncs: []Sync{{Target: ".config/nvim", Source: "../escape"}}})
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeConfigPathEscape, dfmerr.MustCode(err))
}
