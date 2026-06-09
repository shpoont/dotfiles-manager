package recipe

import (
	"errors"
	"fmt"
	"strings"

	"github.com/shpoont/dotfiles-manager/internal/v2/inidriver"
)

var ErrBundledRuntimeUnavailable = errors.New("bundled runtime recipe unavailable")

type RuntimeRecipe struct {
	Recipe      *Recipe
	Source      string
	RecipeRef   string
	TrustStatus string
}

func LoadRuntime(repoRoot string, targetID string) (RuntimeRecipe, error) {
	ref := strings.TrimSpace(targetID)
	if ref == "" {
		return RuntimeRecipe{}, fmt.Errorf("runtime recipe target is required")
	}
	if target, bundled := LookupBundledTarget(ref); bundled {
		runtime := RuntimeRecipe{
			Source:      RecipeSourceBundled,
			RecipeRef:   target.RecipeRef,
			TrustStatus: target.TrustStatus,
		}
		switch target.ID {
		case GitTarget:
			rec := BundledGitRecipe()
			if err := rec.ValidateGit(); err != nil {
				return runtime, fmt.Errorf("validate bundled git recipe: %w", err)
			}
			runtime.Recipe = rec
			return runtime, nil
		case StarshipTarget:
			rec := BundledStarshipRecipe()
			if err := rec.ValidateStarship(); err != nil {
				return runtime, fmt.Errorf("validate bundled starship recipe: %w", err)
			}
			runtime.Recipe = rec
			return runtime, nil
		case TmuxTarget:
			rec := BundledTmuxRecipe()
			if err := rec.ValidateTmux(); err != nil {
				return runtime, fmt.Errorf("validate bundled tmux recipe: %w", err)
			}
			runtime.Recipe = rec
			return runtime, nil
		case NvimTarget:
			rec := BundledNvimRecipe()
			if err := rec.ValidateNvim(); err != nil {
				return runtime, fmt.Errorf("validate bundled nvim recipe: %w", err)
			}
			runtime.Recipe = rec
			return runtime, nil
		case ZshTarget:
			rec := BundledZshRecipe()
			if err := rec.ValidateZsh(); err != nil {
				return runtime, fmt.Errorf("validate bundled zsh recipe: %w", err)
			}
			runtime.Recipe = rec
			return runtime, nil
		default:
			return runtime, fmt.Errorf("%w: %s", ErrBundledRuntimeUnavailable, target.ID)
		}
	}

	rec, err := LoadLocal(repoRoot, ref)
	runtime := RuntimeRecipe{
		Recipe:    rec,
		Source:    RecipeSourceLocal,
		RecipeRef: "recipe://local/" + ref,
	}
	if err != nil {
		return runtime, err
	}
	return runtime, nil
}

func BundledGitRecipe() *Recipe {
	return &Recipe{
		Schema:        Schema,
		SchemaVersion: SupportedVersion,
		Target:        GitTarget,
		DisplayName:   "Git",
		SupportLevel:  "experimental",
		Capability:    "read-write",
		Locations: map[string]Location{
			"home": {Default: "~"},
		},
		SettingsGroups: map[string]SettingsGroup{
			"identity": {
				Label:        "Identity",
				Description:  "Non-credential Git user identity values.",
				SupportLevel: "experimental",
				Capability:   "read-write",
				Settings:     []string{"user.email", "user.name"},
			},
		},
		Settings: map[string]Setting{
			"user.email": {
				Label:        "User email",
				SupportLevel: "experimental",
				Capability:   "read-write",
				ArtifactForm: "scalar",
				Sensitivity:  SensitivityPersonal,
				Redaction:    RedactionRedactedForDisplay,
				Lifecycle:    LifecycleAllowed,
				ScopeDefault: "user",
				Resource:     "user-email",
			},
			"user.name": {
				Label:        "User name",
				SupportLevel: "experimental",
				Capability:   "read-write",
				ArtifactForm: "scalar",
				Sensitivity:  SensitivityPersonal,
				Redaction:    RedactionRedactedForDisplay,
				Lifecycle:    LifecycleAllowed,
				ScopeDefault: "user",
				Resource:     "user-name",
			},
		},
		Resources: map[string]Resource{
			"user-email": bundledGitResource("email"),
			"user-name":  bundledGitResource("name"),
		},
	}
}

func bundledGitResource(key string) Resource {
	return Resource{
		Driver:      IniFileDriverID,
		Location:    "home",
		Path:        ".gitconfig",
		Capability:  "read-write",
		Sensitivity: SensitivityPersonal,
		Redaction:   RedactionRedactedForDisplay,
		Lifecycle:   LifecycleAllowed,
		Selector: &Selector{
			Section:         "user",
			Key:             key,
			MissingSection:  string(inidriver.MissingPolicyCreate),
			MissingKey:      string(inidriver.MissingPolicyCreate),
			DuplicatePolicy: string(inidriver.DuplicatePolicyReject),
			DeleteKey:       string(inidriver.DeletePolicyReject),
		},
	}
}

func BundledStarshipRecipe() *Recipe {
	return &Recipe{
		Schema:        Schema,
		SchemaVersion: SupportedVersion,
		Target:        StarshipTarget,
		DisplayName:   "Starship",
		SupportLevel:  "experimental",
		Capability:    "read-write",
		Locations: map[string]Location{
			"config": {Default: "~/.config"},
		},
		SettingsGroups: map[string]SettingsGroup{
			"prompt": {
				Label:        "Prompt-wide options",
				Description:  "Selected root-level Starship prompt configuration values.",
				SupportLevel: "experimental",
				Capability:   "read-write",
				Settings:     starshipSettingIDs(),
			},
		},
		Settings: map[string]Setting{
			"add_newline":     bundledStarshipSetting("add_newline", "Add newline"),
			"command_timeout": bundledStarshipSetting("command_timeout", "Command timeout"),
			"follow_symlinks": bundledStarshipSetting("follow_symlinks", "Follow symlinks"),
			"scan_timeout":    bundledStarshipSetting("scan_timeout", "Scan timeout"),
		},
		Resources: map[string]Resource{
			"add_newline":     bundledStarshipResource("add_newline"),
			"command_timeout": bundledStarshipResource("command_timeout"),
			"follow_symlinks": bundledStarshipResource("follow_symlinks"),
			"scan_timeout":    bundledStarshipResource("scan_timeout"),
		},
	}
}

func bundledStarshipSetting(id string, label string) Setting {
	return Setting{
		Label:        label,
		SupportLevel: "experimental",
		Capability:   "read-write",
		ArtifactForm: "scalar",
		Sensitivity:  SensitivityLow,
		Redaction:    RedactionKnownSafe,
		Lifecycle:    LifecycleAllowed,
		ScopeDefault: "user",
		Resource:     id,
	}
}

func bundledStarshipResource(key string) Resource {
	return Resource{
		Driver:      TOMLFileDriverID,
		Location:    "config",
		Path:        "starship.toml",
		Capability:  "read-write",
		Sensitivity: SensitivityLow,
		Redaction:   RedactionKnownSafe,
		Lifecycle:   LifecycleAllowed,
		Selector: &Selector{
			Path:            []string{key},
			CreateMissing:   "create",
			DuplicatePolicy: "reject",
			DeleteKey:       "allow",
		},
	}
}

func BundledNvimRecipe() *Recipe {
	return &Recipe{
		Schema:        Schema,
		SchemaVersion: SupportedVersion,
		Target:        NvimTarget,
		DisplayName:   "Neovim",
		SupportLevel:  "experimental",
		Capability:    "read-write",
		Locations: map[string]Location{
			"config": {Default: "~/.config"},
		},
		SettingsGroups: map[string]SettingsGroup{
			"config": {
				Label:        "Config tree",
				Description:  "The Neovim configuration tree under the config location.",
				SupportLevel: "experimental",
				Capability:   "read-write",
				Settings:     nvimSettingIDs(),
			},
		},
		Settings: map[string]Setting{
			"config": {
				Label:        "Config tree",
				SupportLevel: "experimental",
				Capability:   "read-write",
				ArtifactForm: "file-tree",
				Sensitivity:  SensitivityPersonal,
				Redaction:    RedactionRedactedForDisplay,
				Lifecycle:    LifecycleAllowed,
				ScopeDefault: "user",
				Resource:     "config",
			},
		},
		Resources: map[string]Resource{
			"config": {
				Driver:      FileTreeDriverID,
				Location:    "config",
				Path:        "nvim",
				Capability:  "read-write",
				Sensitivity: SensitivityPersonal,
				Redaction:   RedactionRedactedForDisplay,
				Lifecycle:   LifecycleAllowed,
				Include:     []string{"**"},
				Exclude:     nvimExcludeGlobs(),
			},
		},
	}
}

func BundledTmuxRecipe() *Recipe {
	settings := map[string]Setting{}
	resources := map[string]Resource{}
	for _, id := range tmuxSettingIDs() {
		settings[id] = bundledTmuxSetting(id)
		resources[id] = bundledTmuxResource(id)
	}
	return &Recipe{
		Schema:        Schema,
		SchemaVersion: SupportedVersion,
		Target:        TmuxTarget,
		DisplayName:   "tmux",
		SupportLevel:  "experimental",
		Capability:    "read-write",
		Locations: map[string]Location{
			"home":   {Default: "~"},
			"config": {Default: "~/.config"},
		},
		SettingsGroups: map[string]SettingsGroup{
			"user-config-files": {
				Label:        "User config files",
				Description:  "Alternative tmux user configuration file locations; tmux decides which user config file is loaded.",
				SupportLevel: "experimental",
				Capability:   "read-write",
				Settings:     tmuxSettingIDs(),
			},
		},
		Settings:  settings,
		Resources: resources,
	}
}

func bundledTmuxSetting(id string) Setting {
	return Setting{
		Label:        tmuxSettingLabel(id),
		SupportLevel: "experimental",
		Capability:   "read-write",
		ArtifactForm: "file",
		Sensitivity:  SensitivityPersonal,
		Redaction:    RedactionRedactedForDisplay,
		Lifecycle:    LifecycleWarn,
		ScopeDefault: "user",
		Resource:     id,
	}
}

func bundledTmuxResource(id string) Resource {
	return Resource{
		Driver:      FileDriverID,
		Location:    tmuxLocationID(id),
		Path:        tmuxResourcePath(id),
		Capability:  "read-write",
		Sensitivity: SensitivityPersonal,
		Redaction:   RedactionRedactedForDisplay,
		Lifecycle:   LifecycleWarn,
	}
}

func tmuxSettingLabel(id string) string {
	switch id {
	case "home.conf":
		return "~/.tmux.conf"
	case "xdg.conf":
		return "~/.config/tmux/tmux.conf"
	default:
		return fallbackLabel(id)
	}
}

func BundledZshRecipe() *Recipe {
	settings := map[string]Setting{}
	resources := map[string]Resource{}
	for _, id := range zshSettingIDs() {
		settings[id] = bundledZshSetting(id, zshSettingLabel(id))
		resources[id] = bundledZshResource(id)
	}
	return &Recipe{
		Schema:        Schema,
		SchemaVersion: SupportedVersion,
		Target:        ZshTarget,
		DisplayName:   "Zsh",
		SupportLevel:  "experimental",
		Capability:    "read-write",
		Locations: map[string]Location{
			"home": {Default: "~"},
		},
		SettingsGroups: map[string]SettingsGroup{
			"startup-files": {
				Label:        "Startup files",
				Description:  "Selected Zsh startup files under the home location.",
				SupportLevel: "experimental",
				Capability:   "read-write",
				Settings:     zshSettingIDs(),
			},
		},
		Settings:  settings,
		Resources: resources,
	}
}

func bundledZshSetting(id string, label string) Setting {
	return Setting{
		Label:        label,
		SupportLevel: "experimental",
		Capability:   "read-write",
		ArtifactForm: "file",
		Sensitivity:  SensitivityPersonal,
		Redaction:    RedactionRedactedForDisplay,
		Lifecycle:    LifecycleWarn,
		ScopeDefault: "user",
		Resource:     id,
	}
}

func bundledZshResource(id string) Resource {
	return Resource{
		Driver:      FileDriverID,
		Location:    "home",
		Path:        zshResourcePath(id),
		Capability:  "read-write",
		Sensitivity: SensitivityPersonal,
		Redaction:   RedactionRedactedForDisplay,
		Lifecycle:   LifecycleWarn,
	}
}

func zshSettingLabel(id string) string {
	switch id {
	case "zshrc":
		return ".zshrc"
	case "zprofile":
		return ".zprofile"
	case "zlogin":
		return ".zlogin"
	case "zlogout":
		return ".zlogout"
	default:
		return fallbackLabel(id)
	}
}
