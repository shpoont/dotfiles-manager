package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/shpoont/dotfiles-manager/internal/dfmerr"
	"github.com/stretchr/testify/require"
)

func TestResolvePathExplicitNotFound(t *testing.T) {
	t.Parallel()
	_, err := ResolvePath(ResolveOptions{ExplicitPath: filepath.Join(t.TempDir(), "missing.yaml")})
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeConfigNotFound, dfmerr.MustCode(err))
}

func TestResolvePathExplicitReadError(t *testing.T) {
	t.Parallel()
	_, err := ResolvePath(ResolveOptions{
		ExplicitPath: "config.yaml",
		Stat: func(string) (os.FileInfo, error) {
			return nil, os.ErrPermission
		},
	})
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeIORead, dfmerr.MustCode(err))
}

func TestEnsureConfigPathRejectsDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := ensureConfigPath(dir, os.Stat)
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeConfigNotFile, dfmerr.MustCode(err))
}

func TestLoadTypeErrorClassification(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	body := []byte("syncs:\n  - target: [1,2]\n    source: .config/nvim\n")
	require.NoError(t, os.WriteFile(cfgPath, body, 0o644))

	_, err := Load(cfgPath)
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeConfigSchemaType, dfmerr.MustCode(err))
}

func TestLoadParseErrorClassification(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(": bad yaml"), 0o644))

	_, err := Load(cfgPath)
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeConfigParse, dfmerr.MustCode(err))
}

func TestLoadReadFailure(t *testing.T) {
	t.Parallel()
	_, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeIORead, dfmerr.MustCode(err))
}

func TestValidateRequiredKeys(t *testing.T) {
	t.Parallel()

	err := Validate(nil)
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeConfigParse, dfmerr.MustCode(err))

	err = Validate(&Config{})
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeConfigSchemaRequired, dfmerr.MustCode(err))

	err = Validate(&Config{Syncs: []Sync{{Source: "a"}}})
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeConfigSchemaRequired, dfmerr.MustCode(err))

	err = Validate(&Config{Syncs: []Sync{{Target: "a"}}})
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeConfigSchemaRequired, dfmerr.MustCode(err))
}

func TestExtractUnknownKeyFallback(t *testing.T) {
	t.Parallel()
	require.Equal(t, "<unknown>", extractUnknownKey("no key marker"))
	require.Equal(t, "foo", extractUnknownKey("field foo"))
}

func TestClassifyParseErrorUnknownAndParseBranches(t *testing.T) {
	t.Parallel()
	err := classifyParseError("config.yaml", errors.New("line 1: field foo not found in type bar"))
	require.Equal(t, dfmerr.CodeConfigSchemaUnknownKey, dfmerr.MustCode(err))

	err = classifyParseError("config.yaml", errors.New("totally invalid"))
	require.Equal(t, dfmerr.CodeConfigParse, dfmerr.MustCode(err))
}

func TestValidateRelativeSuccess(t *testing.T) {
	t.Parallel()
	err := validateRelative(".config/nvim", "syncs[0].target")
	require.NoError(t, err)
}

func TestValidateRejectsTildeAndAbsoluteEnvExpansion(t *testing.T) {
	t.Parallel()

	err := validateWithLookup(
		&Config{Syncs: []Sync{{Target: "~/.config/nvim", Source: ".config/nvim"}}},
		func(string) (string, bool) { return "", false },
	)
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeConfigPathNotRelative, dfmerr.MustCode(err))

	err = validateWithLookup(
		&Config{Syncs: []Sync{{Target: ".config/nvim", Source: "$HOME/nvim"}}},
		func(key string) (string, bool) {
			if key == "HOME" {
				return "/Users/shpoont", true
			}
			return "", false
		},
	)
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeConfigPathNotRelative, dfmerr.MustCode(err))
}

func TestValidateAllowsEnvPlaceholders(t *testing.T) {
	t.Parallel()

	err := validateWithLookup(
		&Config{Syncs: []Sync{{Target: ".config/$HOSTNAME", Source: "./$HOSTNAME/$USER"}}},
		func(key string) (string, bool) {
			switch key {
			case "HOSTNAME":
				return "mbp", true
			case "USER":
				return "alice", true
			default:
				return "", false
			}
		},
	)
	require.NoError(t, err)
}

func TestValidateRejectsMissingOrEmptyEnvPlaceholders(t *testing.T) {
	t.Parallel()

	err := validateWithLookup(
		&Config{Syncs: []Sync{{Target: ".config/$HOSTNAME", Source: ".config/nvim"}}},
		func(string) (string, bool) { return "", false },
	)
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeConfigPathEnvUndefined, dfmerr.MustCode(err))

	err = validateWithLookup(
		&Config{Syncs: []Sync{{Target: ".config/nvim", Source: "./$USER"}}},
		func(key string) (string, bool) {
			if key == "USER" {
				return "", true
			}
			return "", false
		},
	)
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeConfigPathEnvUndefined, dfmerr.MustCode(err))
}

func TestExpandSyncPathRejectsInvalidPlaceholderAndEscape(t *testing.T) {
	t.Parallel()

	_, err := expandPathPlaceholders("./${HOSTNAME", "syncs[0].source", func(string) (string, bool) { return "", false })
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeConfigSchemaType, dfmerr.MustCode(err))

	_, err = expandAndValidateSyncPath("./$ENV", "syncs[0].source", func(key string) (string, bool) {
		if key == "ENV" {
			return "../escape", true
		}
		return "", false
	})
	require.Error(t, err)
	require.Equal(t, dfmerr.CodeConfigPathEscape, dfmerr.MustCode(err))
}
