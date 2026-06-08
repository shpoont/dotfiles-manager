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
