package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/shpoont/dotfiles-manager/internal/dfmerr"
	"gopkg.in/yaml.v3"
)

const (
	DefaultConfigFile = ".dotfiles-manager.yaml"
	ConfigEnvVar      = "DOTFILES_MANAGER_CONFIG"
)

type Config struct {
	Syncs []Sync `yaml:"syncs"`
}

type Sync struct {
	Target string   `yaml:"target"`
	Source string   `yaml:"source"`
	On     OnConfig `yaml:"on"`
}

type OnConfig struct {
	Deploy DeployConfig `yaml:"deploy"`
	Import ImportConfig `yaml:"import"`
}

type DeployConfig struct {
	RemoveUnmanaged []string `yaml:"remove-unmanaged"`
}

type ImportConfig struct {
	AddUnmanaged  PatternToggle `yaml:"add-unmanaged"`
	RemoveMissing PatternToggle `yaml:"remove-missing"`
}

type PatternToggle struct {
	Include []string `yaml:"include"`
	Exclude []string `yaml:"exclude"`
}

type ResolveOptions struct {
	ExplicitPath string
	CWD          string
	Getenv       func(string) string
	Stat         func(string) (os.FileInfo, error)
}

type lookupEnvFunc func(string) (string, bool)

func ResolvePath(opts ResolveOptions) (string, error) {
	getenv := opts.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}

	stat := opts.Stat
	if stat == nil {
		stat = os.Stat
	}

	cwd := opts.CWD
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return "", dfmerr.Wrap(dfmerr.CodeIORead, "Failed to resolve current working directory", nil, err)
		}
	}

	if opts.ExplicitPath != "" {
		return ensureConfigPath(opts.ExplicitPath, stat)
	}

	if envPath := strings.TrimSpace(getenv(ConfigEnvVar)); envPath != "" {
		return ensureConfigPath(envPath, stat)
	}

	defaultPath := filepath.Join(cwd, DefaultConfigFile)
	if _, err := stat(defaultPath); err == nil {
		return ensureConfigPath(defaultPath, stat)
	} else if !os.IsNotExist(err) {
		return "", dfmerr.Wrap(dfmerr.CodeIORead, fmt.Sprintf("Read failed: %s", defaultPath), map[string]any{"path": defaultPath}, err)
	}

	return "", dfmerr.New(dfmerr.CodeConfigRequired, "Config not found: pass --config, set DOTFILES_MANAGER_CONFIG, or create ./.dotfiles-manager.yaml", nil)
}

func ensureConfigPath(configPath string, stat func(string) (os.FileInfo, error)) (string, error) {
	info, err := stat(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", dfmerr.Wrap(dfmerr.CodeConfigNotFound, fmt.Sprintf("Config file not found: %s", configPath), map[string]any{"config_path": configPath}, err)
		}
		return "", dfmerr.Wrap(dfmerr.CodeIORead, fmt.Sprintf("Read failed: %s", configPath), map[string]any{"path": configPath}, err)
	}
	if !info.Mode().IsRegular() {
		return "", dfmerr.New(dfmerr.CodeConfigNotFile, fmt.Sprintf("Config path is not a file: %s", configPath), map[string]any{"config_path": configPath})
	}
	return configPath, nil
}

func Load(configPath string) (*Config, error) {
	file, err := os.Open(configPath)
	if err != nil {
		return nil, dfmerr.Wrap(dfmerr.CodeIORead, fmt.Sprintf("Read failed: %s", configPath), map[string]any{"path": configPath}, err)
	}
	defer file.Close()

	cfg, err := parseYAML(file)
	if err != nil {
		return nil, classifyParseError(configPath, err)
	}

	if err := Validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func parseYAML(r io.Reader) (*Config, error) {
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)

	var cfg Config
	if err := dec.Decode(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func classifyParseError(configPath string, err error) error {
	var typeErr *yaml.TypeError
	if strings.Contains(err.Error(), "field") && strings.Contains(err.Error(), "not found in type") {
		return dfmerr.Wrap(dfmerr.CodeConfigSchemaUnknownKey, fmt.Sprintf("Unknown config key: %s", extractUnknownKey(err.Error())), map[string]any{"config_path": configPath}, err)
	}
	if errors.As(err, &typeErr) {
		return dfmerr.Wrap(dfmerr.CodeConfigSchemaType, fmt.Sprintf("Invalid type in config: %s", configPath), map[string]any{"config_path": configPath}, err)
	}
	return dfmerr.Wrap(dfmerr.CodeConfigParse, fmt.Sprintf("Failed to parse YAML config: %s", configPath), map[string]any{"config_path": configPath}, err)
}

func extractUnknownKey(message string) string {
	// Typical yaml.v3 message: "yaml: unmarshal errors:\n  line 4: field foo not found in type ..."
	marker := "field "
	idx := strings.Index(message, marker)
	if idx == -1 {
		return "<unknown>"
	}
	rest := message[idx+len(marker):]
	end := strings.Index(rest, " ")
	if end == -1 {
		return rest
	}
	return rest[:end]
}

func Validate(cfg *Config) error {
	return validateWithLookup(cfg, os.LookupEnv)
}

func validateWithLookup(cfg *Config, lookup lookupEnvFunc) error {
	if cfg == nil {
		return dfmerr.New(dfmerr.CodeConfigParse, "Failed to parse YAML config: <nil>", nil)
	}

	if len(cfg.Syncs) == 0 {
		return dfmerr.New(dfmerr.CodeConfigSchemaRequired, "Missing required key: syncs", map[string]any{"key_path": "syncs"})
	}

	for idx, sync := range cfg.Syncs {
		targetKey := fmt.Sprintf("syncs[%d].target", idx)
		sourceKey := fmt.Sprintf("syncs[%d].source", idx)
		if strings.TrimSpace(sync.Target) == "" {
			return dfmerr.New(dfmerr.CodeConfigSchemaRequired, fmt.Sprintf("Missing required key: %s", targetKey), map[string]any{"key_path": targetKey})
		}
		if strings.TrimSpace(sync.Source) == "" {
			return dfmerr.New(dfmerr.CodeConfigSchemaRequired, fmt.Sprintf("Missing required key: %s", sourceKey), map[string]any{"key_path": sourceKey})
		}

		if _, err := expandAndValidateSyncPath(sync.Target, targetKey, lookup); err != nil {
			return err
		}
		if _, err := expandAndValidateSyncPath(sync.Source, sourceKey, lookup); err != nil {
			return err
		}
	}

	return nil
}

func ExpandSyncPath(value string, keyPath string) (string, error) {
	return expandAndValidateSyncPath(value, keyPath, os.LookupEnv)
}

func expandAndValidateSyncPath(value string, keyPath string, lookup lookupEnvFunc) (string, error) {
	expanded, err := expandPathPlaceholders(value, keyPath, lookup)
	if err != nil {
		return "", err
	}
	if err := validateRelative(expanded, keyPath); err != nil {
		return "", err
	}
	return expanded, nil
}

func expandPathPlaceholders(value string, keyPath string, lookup lookupEnvFunc) (string, error) {
	if lookup == nil {
		lookup = os.LookupEnv
	}

	var out strings.Builder
	out.Grow(len(value))

	for idx := 0; idx < len(value); {
		if value[idx] != '$' {
			out.WriteByte(value[idx])
			idx++
			continue
		}

		if idx+1 >= len(value) {
			out.WriteByte('$')
			idx++
			continue
		}

		if value[idx+1] == '{' {
			closeOffset := strings.IndexByte(value[idx+2:], '}')
			if closeOffset < 0 {
				return "", dfmerr.New(
					dfmerr.CodeConfigSchemaType,
					fmt.Sprintf("Invalid env placeholder in path: %s", keyPath),
					map[string]any{"key_path": keyPath},
				)
			}

			name := value[idx+2 : idx+2+closeOffset]
			if !isValidEnvName(name) {
				return "", dfmerr.New(
					dfmerr.CodeConfigSchemaType,
					fmt.Sprintf("Invalid env placeholder in path: %s", keyPath),
					map[string]any{"key_path": keyPath},
				)
			}

			val, ok := lookup(name)
			if !ok || val == "" {
				return "", dfmerr.New(
					dfmerr.CodeConfigPathEnvUndefined,
					fmt.Sprintf("Environment variable %s required for path: %s", name, keyPath),
					map[string]any{"key_path": keyPath, "var": name},
				)
			}

			out.WriteString(val)
			idx += closeOffset + 3
			continue
		}

		next := value[idx+1]
		if !isEnvVarStart(next) {
			out.WriteByte('$')
			idx++
			continue
		}

		end := idx + 2
		for end < len(value) && isEnvVarPart(value[end]) {
			end++
		}

		name := value[idx+1 : end]
		val, ok := lookup(name)
		if !ok || val == "" {
			return "", dfmerr.New(
				dfmerr.CodeConfigPathEnvUndefined,
				fmt.Sprintf("Environment variable %s required for path: %s", name, keyPath),
				map[string]any{"key_path": keyPath, "var": name},
			)
		}

		out.WriteString(val)
		idx = end
	}

	return out.String(), nil
}

func isValidEnvName(name string) bool {
	if name == "" {
		return false
	}
	if !isEnvVarStart(name[0]) {
		return false
	}
	for idx := 1; idx < len(name); idx++ {
		if !isEnvVarPart(name[idx]) {
			return false
		}
	}
	return true
}

func isEnvVarStart(ch byte) bool {
	return (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || ch == '_'
}

func isEnvVarPart(ch byte) bool {
	return isEnvVarStart(ch) || (ch >= '0' && ch <= '9')
}

func validateRelative(value, keyPath string) error {
	trimmed := strings.TrimSpace(value)
	if filepath.IsAbs(trimmed) || strings.HasPrefix(trimmed, "~") {
		return dfmerr.New(dfmerr.CodeConfigPathNotRelative, fmt.Sprintf("Path must be relative: %s", keyPath), map[string]any{"key_path": keyPath})
	}

	cleaned := filepath.Clean(trimmed)
	slashed := filepath.ToSlash(cleaned)
	if slashed == ".." || strings.HasPrefix(slashed, "../") {
		return dfmerr.New(dfmerr.CodeConfigPathEscape, fmt.Sprintf("Path escapes base directory: %s", keyPath), map[string]any{"key_path": keyPath})
	}
	return nil
}
