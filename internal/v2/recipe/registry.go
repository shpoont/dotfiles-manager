package recipe

import (
	"fmt"
	"sort"
	"strings"
)

type BundledTarget struct {
	ID              string   `json:"id"`
	DisplayName     string   `json:"displayName"`
	Aliases         []string `json:"aliases"`
	Source          string   `json:"source"`
	RecipeRef       string   `json:"recipeRef"`
	Version         string   `json:"version"`
	SupportLevel    string   `json:"supportLevel"`
	Capability      string   `json:"capability"`
	PlatformSupport string   `json:"platformSupport"`
	TrustStatus     string   `json:"trustStatus"`
	Summary         string   `json:"summary"`
}

type BundledRegistry struct {
	targets map[string]BundledTarget
	aliases map[string]string
}

func DefaultBundledRegistry() *BundledRegistry {
	return mustNewBundledRegistry(defaultBundledTargets())
}

func LookupBundledTarget(ref string) (BundledTarget, bool) {
	return DefaultBundledRegistry().Lookup(ref)
}

func ListBundledTargets() []BundledTarget {
	return DefaultBundledRegistry().List()
}

func KnownBundledTargetIDs() []string {
	targets := ListBundledTargets()
	ids := make([]string, 0, len(targets))
	for _, target := range targets {
		ids = append(ids, target.ID)
	}
	return ids
}

func (r *BundledRegistry) Lookup(ref string) (BundledTarget, bool) {
	if r == nil {
		return BundledTarget{}, false
	}
	normalized := strings.TrimSpace(ref)
	if target, ok := r.targets[normalized]; ok {
		return target, true
	}
	if canonicalID, ok := r.aliases[normalized]; ok {
		return r.targets[canonicalID], true
	}
	return BundledTarget{}, false
}

func (r *BundledRegistry) List() []BundledTarget {
	if r == nil {
		return nil
	}
	ids := sortedKeys(r.targets)
	targets := make([]BundledTarget, 0, len(ids))
	for _, id := range ids {
		target := r.targets[id]
		target.Aliases = append([]string(nil), target.Aliases...)
		sort.Strings(target.Aliases)
		targets = append(targets, target)
	}
	return targets
}

func (target BundledTarget) LocalCollisionIDs() []string {
	ids := append([]string{target.ID}, target.Aliases...)
	sort.Strings(ids)
	out := ids[:0]
	seen := map[string]bool{}
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return append([]string(nil), out...)
}

func newBundledRegistry(targets []BundledTarget) (*BundledRegistry, error) {
	registry := &BundledRegistry{targets: map[string]BundledTarget{}, aliases: map[string]string{}}
	for _, target := range targets {
		target.ID = strings.TrimSpace(target.ID)
		if target.ID == "" {
			return nil, fmt.Errorf("bundled target id is required")
		}
		if err := ValidatePublicID("target", target.ID); err != nil {
			return nil, err
		}
		if _, exists := registry.targets[target.ID]; exists {
			return nil, fmt.Errorf("duplicate bundled target id %s", target.ID)
		}
		target.Source = RecipeSourceBundled
		target.RecipeRef = fallback("recipe://bundled/"+target.ID, target.RecipeRef)
		target.Version = fallback("1", target.Version)
		target.TrustStatus = fallback("trusted", target.TrustStatus)
		target.PlatformSupport = fallback("unknown", target.PlatformSupport)
		target.Aliases = normalizeAliases(target.Aliases)
		registry.targets[target.ID] = target
	}
	for _, target := range registry.targets {
		for _, alias := range target.Aliases {
			if alias == target.ID {
				return nil, fmt.Errorf("bundled target %s alias must not repeat canonical id", target.ID)
			}
			if _, exists := registry.targets[alias]; exists {
				return nil, fmt.Errorf("bundled alias %s collides with a canonical target id", alias)
			}
			if existing, exists := registry.aliases[alias]; exists {
				return nil, fmt.Errorf("bundled alias %s collides between targets %s and %s", alias, existing, target.ID)
			}
			registry.aliases[alias] = target.ID
		}
	}
	return registry, nil
}

func mustNewBundledRegistry(targets []BundledTarget) *BundledRegistry {
	registry, err := newBundledRegistry(targets)
	if err != nil {
		panic(err)
	}
	return registry
}

func normalizeAliases(values []string) []string {
	seen := map[string]bool{}
	var aliases []string
	for _, value := range values {
		alias := strings.TrimSpace(value)
		if alias == "" || seen[alias] {
			continue
		}
		seen[alias] = true
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	return aliases
}

func defaultBundledTargets() []BundledTarget {
	return []BundledTarget{
		{
			ID:              CustomFilesTarget,
			DisplayName:     "Custom files",
			Aliases:         []string{"custom-files", "customfiles"},
			SupportLevel:    "experimental",
			Capability:      "read-write",
			PlatformSupport: "unknown",
			Summary:         "Manage explicitly declared files or file trees without app-specific semantics.",
		},
		{
			ID:              GitTarget,
			DisplayName:     "Git",
			Aliases:         []string{"gitconfig"},
			SupportLevel:    "experimental",
			Capability:      "read-write",
			PlatformSupport: "unknown",
			Summary:         "Manage selected non-credential Git identity settings.",
		},
		{
			ID:              StarshipTarget,
			DisplayName:     "Starship",
			SupportLevel:    "experimental",
			Capability:      "read-write",
			PlatformSupport: "unknown",
			Summary:         "Manage selected prompt-wide Starship TOML options.",
		},
	}
}
