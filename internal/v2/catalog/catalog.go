// Package catalog manages v2 recipe catalog/source discovery state.
//
// Catalog source state is local manager state. It is intentionally stored outside
// the settings storage folder so a shared settings folder cannot grant discovery
// trust or live-write authority on another machine.
package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	v2ledger "github.com/shpoont/dotfiles-manager/internal/v2/ledger"
	v2recipe "github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"golang.org/x/sys/unix"
	"gopkg.in/yaml.v3"
)

const (
	Schema             = "dotfiles-manager.v2.catalogs"
	StateSchema        = "dotfiles-manager.v2.catalog-state"
	SchemaVersion      = 1
	BuiltInName        = "built-in"
	BuiltInSourceID    = "bundled"
	BuiltInCatalogID   = "org.dotfiles-manager.bundled"
	BuiltInDisplayName = "dotfiles-manager built-in recipes"
	SettingsFolderName = "local"
)

const (
	ListCommand    = "catalog.list"
	AddCommand     = "catalog.add"
	DisableCommand = "catalog.disable"
	EnableCommand  = "catalog.enable"
	RemoveCommand  = "catalog.remove"
)

const (
	SourceKindBundled = "bundled"
	SourceKindLocal   = "local"
	SourceKindRemote  = "remote"
)

const (
	StatusEnabled  = "enabled"
	StatusDisabled = "disabled"
	StatusBlocked  = "blocked"
	StatusRemoved  = "removed"
)

const (
	CodeStateInvalid       = "catalog.state.invalid"
	CodeCatalogInvalid     = "catalog.local.invalid"
	CodeCatalogNotFound    = "catalog.notFound"
	CodeCatalogDuplicate   = "catalog.duplicate"
	CodeNameInvalid        = "catalog.name.invalid"
	CodePathInvalid        = "catalog.path.invalid"
	CodeRemoteUnsupported  = "catalog.remote.unsupported"
	CodeBuiltInImmutable   = "catalog.builtin.immutable"
	CodeUnsupportedSource  = "catalog.source.unsupported"
	CodeStateRootInvalid   = "catalog.stateRoot.invalid"
	CodeStateWriteFailed   = "catalog.state.writeFailed"
	CodeStateReadFailed    = "catalog.state.readFailed"
	CodeCatalogEmpty       = "catalog.local.empty"
	CodeCatalogScanFailure = "catalog.local.scanFailed"
)

var githubOwnerRepoPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

type Options struct {
	RepoRoot       string
	StateRoot      string
	ManagerVersion string
}

type AddOptions struct {
	Options
	Name string
	Path string
}

type Summary struct {
	Status         string `json:"status"`
	Sources        int    `json:"sources"`
	Enabled        int    `json:"enabled"`
	Disabled       int    `json:"disabled"`
	ValidRecipes   int    `json:"validRecipes"`
	InvalidRecipes int    `json:"invalidRecipes"`
	HiddenRecipes  int    `json:"hiddenRecipes"`
	Failed         int    `json:"failed"`
}

type Report struct {
	Schema        string          `json:"schema"`
	SchemaVersion int             `json:"schemaVersion"`
	Command       string          `json:"command"`
	RunID         string          `json:"runId"`
	Summary       Summary         `json:"summary"`
	Sources       []Source        `json:"sources"`
	ChangedSource Source          `json:"changedSource,omitempty"`
	Validated     []ValidationRef `json:"validated,omitempty"`
	Invalid       []InvalidRecipe `json:"invalid,omitempty"`
	Diagnostics   []Diagnostic    `json:"diagnostics"`
	Error         *ErrorObject    `json:"error,omitempty"`
}

type Source struct {
	Name             string   `json:"name" yaml:"name"`
	SourceID         string   `json:"sourceId" yaml:"sourceId"`
	SourceKind       string   `json:"sourceKind" yaml:"sourceKind"`
	CatalogID        string   `json:"catalogId" yaml:"catalogId"`
	DisplayName      string   `json:"displayName" yaml:"displayName"`
	OriginURI        string   `json:"originUri" yaml:"originUri"`
	Status           string   `json:"status" yaml:"status"`
	SourceAcceptance string   `json:"sourceAcceptance" yaml:"sourceAcceptance"`
	IntegrityState   string   `json:"integrityState" yaml:"integrityState"`
	WriteDefault     string   `json:"writeDefault" yaml:"writeDefault"`
	UpdatePolicy     string   `json:"updatePolicy" yaml:"updatePolicy"`
	Path             string   `json:"path,omitempty" yaml:"path,omitempty"`
	PinnedIdentity   string   `json:"pinnedIdentity,omitempty" yaml:"pinnedIdentity,omitempty"`
	ValidRecipes     int      `json:"validRecipes" yaml:"-"`
	InvalidRecipes   int      `json:"invalidRecipes" yaml:"-"`
	HiddenRecipes    int      `json:"hiddenRecipes" yaml:"-"`
	BlockedReason    string   `json:"blockedReason,omitempty" yaml:"-"`
	SettingsFolder   bool     `json:"settingsFolder,omitempty" yaml:"-"`
	KnownTargets     []string `json:"knownTargets,omitempty" yaml:"-"`
}

type ValidationRef struct {
	TargetID     string `json:"targetId"`
	DisplayName  string `json:"displayName"`
	RecipeRef    string `json:"recipeRef"`
	SourceName   string `json:"sourceName"`
	SourceKind   string `json:"sourceKind"`
	SourceID     string `json:"sourceId"`
	CatalogID    string `json:"catalogId"`
	OriginURI    string `json:"originUri"`
	Role         string `json:"role"`
	RecipePath   string `json:"recipePath,omitempty"`
	RecipeDigest string `json:"recipeDigest,omitempty"`
}

type InvalidRecipe struct {
	TargetID string   `json:"targetId"`
	Path     string   `json:"path"`
	Errors   []string `json:"errors"`
}

type Diagnostic struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Path     string `json:"path,omitempty"`
	Ref      string `json:"ref,omitempty"`
}

type ErrorObject struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type Error struct {
	Code    string
	Message string
	Exit    int
	Details map[string]any
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *Error) ExitCode() int {
	if e == nil || e.Exit == 0 {
		return 1
	}
	return e.Exit
}

type Discovery struct {
	Sources         []Source       `json:"sources"`
	DisabledSources []Source       `json:"disabledSources"`
	Apps            []EffectiveApp `json:"apps"`
	Diagnostics     []Diagnostic   `json:"diagnostics"`
}

type EffectiveApp struct {
	ID         string            `json:"id"`
	Default    RecipeCandidate   `json:"default"`
	Candidates []RecipeCandidate `json:"candidates"`
	Ambiguous  bool              `json:"ambiguous,omitempty"`
}

type RecipeCandidate struct {
	TargetID          string           `json:"targetId"`
	DisplayName       string           `json:"displayName"`
	Aliases           []string         `json:"aliases,omitempty"`
	RecipeRef         string           `json:"recipeRef"`
	SourceName        string           `json:"sourceName"`
	SourceKind        string           `json:"sourceKind"`
	SourceID          string           `json:"sourceId"`
	CatalogID         string           `json:"catalogId"`
	SourceDisplayName string           `json:"sourceDisplayName"`
	OriginURI         string           `json:"originUri"`
	SelectedBy        string           `json:"selectedBy"`
	TrustStatus       string           `json:"trustStatus"`
	WriteAuthority    string           `json:"writeAuthority"`
	ReviewStatus      string           `json:"reviewStatus"`
	SupportLevel      string           `json:"supportLevel"`
	Capability        string           `json:"capability"`
	PlatformSupport   string           `json:"platformSupport"`
	Summary           string           `json:"summary"`
	RecipePath        string           `json:"recipePath,omitempty"`
	RecipeDigest      string           `json:"recipeDigest,omitempty"`
	Settings          []SettingSummary `json:"settings,omitempty"`
	DoNotManage       []string         `json:"doNotManage,omitempty"`
	Recipe            *v2recipe.Recipe `json:"-"`
}

type SettingSummary struct {
	Ref          string `json:"ref"`
	ID           string `json:"id"`
	Label        string `json:"label"`
	DefaultScope string `json:"defaultScope"`
	ResourceID   string `json:"resourceId,omitempty"`
	Driver       string `json:"driver,omitempty"`
	LiveLocation string `json:"liveLocation,omitempty"`
}

type stateFile struct {
	Schema        string         `yaml:"schema"`
	SchemaVersion int            `yaml:"schemaVersion"`
	Sources       []storedSource `yaml:"sources,omitempty"`
}

type storedSource struct {
	Name             string   `yaml:"name"`
	SourceID         string   `yaml:"sourceId"`
	SourceKind       string   `yaml:"sourceKind"`
	CatalogID        string   `yaml:"catalogId"`
	DisplayName      string   `yaml:"displayName"`
	OriginURI        string   `yaml:"originUri"`
	Status           string   `yaml:"status"`
	SourceAcceptance string   `yaml:"sourceAcceptance"`
	IntegrityState   string   `yaml:"integrityState"`
	WriteDefault     string   `yaml:"writeDefault"`
	UpdatePolicy     string   `yaml:"updatePolicy"`
	Path             string   `yaml:"path,omitempty"`
	PinnedIdentity   string   `yaml:"pinnedIdentity,omitempty"`
	KnownTargets     []string `yaml:"knownTargets,omitempty"`
}

func List(opts Options) (*Report, error) {
	report := baseReport(ListCommand)
	state, err := loadState(opts)
	if err != nil {
		catErr := classifyStateError(err)
		return failReport(report, catErr), catErr
	}
	sources, err := sourcesFromState(opts, state, true)
	if err != nil {
		catErr := classifyStateError(err)
		return failReport(report, catErr), catErr
	}
	report.Sources = sources
	report.Summary = summarizeSources(sources)
	return report, nil
}

func Add(opts AddOptions) (*Report, error) {
	report := baseReport(AddCommand)
	if looksLikeRemote(opts.Path) {
		displayName := strings.TrimSpace(opts.Name)
		if displayName == "" {
			displayName = strings.TrimSpace(opts.Path)
		}
		catErr := &Error{Code: CodeRemoteUnsupported, Message: "remote GitHub catalogs are not supported in this version of dotfiles-manager", Exit: 2, Details: map[string]any{"name": displayName, "source": opts.Path}}
		return failReport(report, catErr), catErr
	}
	name, err := validateCatalogName(opts.Name)
	if err != nil {
		catErr := &Error{Code: CodeNameInvalid, Message: err.Error(), Exit: 2, Details: map[string]any{"name": opts.Name}}
		return failReport(report, catErr), catErr
	}
	absPath, err := resolveLocalPath(opts.Path)
	if err != nil {
		catErr := &Error{Code: CodePathInvalid, Message: err.Error(), Exit: 2, Details: map[string]any{"path": opts.Path}}
		return failReport(report, catErr), catErr
	}
	stateRoot, err := resolveStateRoot(opts.Options)
	if err != nil {
		catErr := &Error{Code: CodeStateRootInvalid, Message: err.Error(), Exit: 2}
		return failReport(report, catErr), catErr
	}
	unlock, err := lockStateRoot(stateRoot)
	if err != nil {
		catErr := &Error{Code: CodeStateWriteFailed, Message: err.Error(), Exit: 1}
		return failReport(report, catErr), catErr
	}
	defer unlock()
	state, err := readStateAt(stateRoot)
	if err != nil {
		catErr := classifyStateError(err)
		return failReport(report, catErr), catErr
	}
	if existing := findStoredSource(state.Sources, name); existing != nil && existing.Status != StatusRemoved {
		catErr := &Error{Code: CodeCatalogDuplicate, Message: fmt.Sprintf("local catalog already exists: %s", name), Exit: 2, Details: map[string]any{"name": name}}
		return failReport(report, catErr), catErr
	}
	source := localSource(name, absPath)
	scan, err := scanLocalCatalog(source)
	if err != nil {
		return catalogScanFailure(report, name, err)
	}
	if len(scan.invalid) > 0 {
		report.Invalid = scan.invalid
		report.Summary.Status = "error"
		report.Summary.InvalidRecipes = len(scan.invalid)
		report.Summary.Failed = 1
		catErr := &Error{Code: CodeCatalogInvalid, Message: fmt.Sprintf("catalog %s has invalid support definitions", name), Exit: 2, Details: map[string]any{"name": name, "invalid": len(scan.invalid)}}
		report.Error = &ErrorObject{Code: catErr.Code, Message: catErr.Message, Details: catErr.Details}
		return report, catErr
	}
	if len(scan.candidates) == 0 {
		catErr := &Error{Code: CodeCatalogEmpty, Message: fmt.Sprintf("local catalog contains no recipe.yaml files: %s", absPath), Exit: 2, Details: map[string]any{"path": absPath}}
		return failReport(report, catErr), catErr
	}
	source.KnownTargets = targetsFromCandidates(scan.candidates)
	if idx := storedSourceIndex(state.Sources, name); idx >= 0 {
		state.Sources[idx] = storedFromSource(source)
	} else {
		state.Sources = append(state.Sources, storedFromSource(source))
	}
	sortStoredSources(state.Sources)
	if err := writeStateAt(stateRoot, state); err != nil {
		catErr := &Error{Code: CodeStateWriteFailed, Message: err.Error(), Exit: 1}
		return failReport(report, catErr), catErr
	}
	source.ValidRecipes = len(scan.candidates)
	report.ChangedSource = source
	report.Validated = validationRefs(scan.candidates)
	report.Summary.Status = "ok"
	report.Summary.ValidRecipes = len(scan.candidates)
	return report, nil
}

func Disable(opts Options, name string) (*Report, error) {
	return setEnabled(opts, name, StatusDisabled, DisableCommand)
}

func Enable(opts Options, name string) (*Report, error) {
	return setEnabled(opts, name, StatusEnabled, EnableCommand)
}

func Remove(opts Options, name string) (*Report, error) {
	report := baseReport(RemoveCommand)
	rawName := strings.TrimSpace(name)
	if isBuiltInName(rawName) {
		catErr := &Error{Code: CodeBuiltInImmutable, Message: "built-in catalog cannot be removed", Exit: 2, Details: map[string]any{"name": rawName}}
		return failReport(report, catErr), catErr
	}
	name, err := validateCatalogName(rawName)
	if err != nil {
		catErr := &Error{Code: CodeNameInvalid, Message: err.Error(), Exit: 2, Details: map[string]any{"name": rawName}}
		return failReport(report, catErr), catErr
	}
	if isBuiltInName(name) {
		catErr := &Error{Code: CodeBuiltInImmutable, Message: "built-in catalog cannot be removed", Exit: 2, Details: map[string]any{"name": name}}
		return failReport(report, catErr), catErr
	}
	stateRoot, err := resolveStateRoot(opts)
	if err != nil {
		catErr := &Error{Code: CodeStateRootInvalid, Message: err.Error(), Exit: 2}
		return failReport(report, catErr), catErr
	}
	unlock, err := lockStateRoot(stateRoot)
	if err != nil {
		catErr := &Error{Code: CodeStateWriteFailed, Message: err.Error(), Exit: 1}
		return failReport(report, catErr), catErr
	}
	defer unlock()
	state, err := readStateAt(stateRoot)
	if err != nil {
		catErr := classifyStateError(err)
		return failReport(report, catErr), catErr
	}
	idx := storedSourceIndex(state.Sources, name)
	if idx < 0 {
		catErr := &Error{Code: CodeCatalogNotFound, Message: fmt.Sprintf("local catalog not found: %s", name), Exit: 2, Details: map[string]any{"name": name}}
		return failReport(report, catErr), catErr
	}
	if state.Sources[idx].Status == StatusRemoved {
		catErr := &Error{Code: CodeCatalogNotFound, Message: fmt.Sprintf("local catalog not found: %s", name), Exit: 2, Details: map[string]any{"name": name}}
		return failReport(report, catErr), catErr
	}
	removed := sourceFromStored(state.Sources[idx])
	scan, _ := scanLocalCatalog(removed)
	removed.ValidRecipes = len(scan.candidates)
	removed.KnownTargets = targetsFromCandidates(scan.candidates)
	if len(removed.KnownTargets) == 0 {
		removed.KnownTargets = append([]string(nil), state.Sources[idx].KnownTargets...)
	}
	removed.Status = StatusRemoved
	state.Sources[idx] = storedFromSource(removed)
	if err := writeStateAt(stateRoot, state); err != nil {
		catErr := &Error{Code: CodeStateWriteFailed, Message: err.Error(), Exit: 1}
		return failReport(report, catErr), catErr
	}
	report.ChangedSource = removed
	report.Validated = validationRefs(scan.candidates)
	report.Summary.Status = "ok"
	report.Summary.HiddenRecipes = len(scan.candidates)
	return report, nil
}

type TargetLookup struct {
	Found      bool
	Available  bool
	Ambiguous  bool
	Candidate  RecipeCandidate
	Candidates []RecipeCandidate
	Source     Source
	Sources    []Source
}

func LookupTarget(opts Options, targetID string, includeDisabled bool) (TargetLookup, error) {
	targetID = strings.TrimSpace(targetID)
	if targetID == "" {
		return TargetLookup{}, fmt.Errorf("target id is required")
	}
	state, err := loadState(opts)
	if err != nil {
		return TargetLookup{}, classifyStateError(err)
	}
	sources, err := sourcesFromStateWithRemoved(opts, state, false, includeDisabled)
	if err != nil {
		return TargetLookup{}, classifyStateError(err)
	}
	var matches []RecipeCandidate
	var sourceMatches []Source
	for _, source := range sources {
		if source.SourceKind != SourceKindLocal {
			continue
		}
		available := source.Status == StatusEnabled
		if !available && !includeDisabled {
			continue
		}
		scan, err := scanLocalCatalog(source)
		if err != nil {
			if targetInKnownTargets(source.KnownTargets, targetID) {
				blocked := source
				if available {
					blocked.Status = StatusBlocked
					blocked.BlockedReason = fmt.Sprintf("catalog %s could not be scanned: %v", source.Name, err)
				}
				return TargetLookup{Found: true, Available: false, Candidate: RecipeCandidate{TargetID: targetID, RecipeRef: localRecipeRef(source.Name, targetID), SourceName: source.Name, SourceKind: source.SourceKind, SourceID: source.SourceID, CatalogID: source.CatalogID, SourceDisplayName: source.DisplayName, OriginURI: source.OriginURI}, Source: blocked}, nil
			}
			continue
		}
		var matchedCandidate *RecipeCandidate
		for i := range scan.candidates {
			if scan.candidates[i].TargetID == targetID {
				matchedCandidate = &scan.candidates[i]
				break
			}
		}
		if len(scan.invalid) > 0 {
			if matchedCandidate != nil || targetInKnownTargets(source.KnownTargets, targetID) {
				blocked := source
				blocked.Status = StatusBlocked
				blocked.InvalidRecipes = len(scan.invalid)
				blocked.BlockedReason = fmt.Sprintf("catalog %s has %d invalid support definition%s", source.Name, len(scan.invalid), plural(len(scan.invalid)))
				candidate := RecipeCandidate{TargetID: targetID, RecipeRef: localRecipeRef(source.Name, targetID), SourceName: source.Name, SourceKind: source.SourceKind, SourceID: source.SourceID, CatalogID: source.CatalogID, SourceDisplayName: source.DisplayName, OriginURI: source.OriginURI}
				if matchedCandidate != nil {
					candidate = *matchedCandidate
				}
				return TargetLookup{Found: true, Available: false, Candidate: candidate, Source: blocked}, nil
			}
			continue
		}
		if matchedCandidate == nil && targetInKnownTargets(source.KnownTargets, targetID) {
			return TargetLookup{Found: true, Available: false, Candidate: RecipeCandidate{TargetID: targetID, RecipeRef: localRecipeRef(source.Name, targetID), SourceName: source.Name, SourceKind: source.SourceKind, SourceID: source.SourceID, CatalogID: source.CatalogID, SourceDisplayName: source.DisplayName, OriginURI: source.OriginURI}, Source: source}, nil
		}
		if matchedCandidate != nil {
			if !available {
				return TargetLookup{Found: true, Available: false, Candidate: *matchedCandidate, Source: source}, nil
			}
			matches = append(matches, *matchedCandidate)
			sourceMatches = append(sourceMatches, source)
		}
	}
	if len(matches) == 0 {
		return TargetLookup{}, nil
	}
	if len(matches) > 1 {
		return TargetLookup{Found: true, Available: false, Ambiguous: true, Candidate: matches[0], Candidates: matches, Source: sourceMatches[0], Sources: sourceMatches}, nil
	}
	return TargetLookup{Found: true, Available: true, Candidate: matches[0], Candidates: matches, Source: sourceMatches[0], Sources: sourceMatches}, nil
}

func Discover(opts Options) (*Discovery, error) {
	state, err := loadState(opts)
	if err != nil {
		return nil, classifyStateError(err)
	}
	sources, err := sourcesFromState(opts, state, true)
	if err != nil {
		return nil, classifyStateError(err)
	}
	discovery := &Discovery{Sources: []Source{}, DisabledSources: []Source{}, Apps: []EffectiveApp{}, Diagnostics: []Diagnostic{}}
	appsByID := map[string]*EffectiveApp{}
	for _, source := range sources {
		if source.Status == StatusDisabled {
			discovery.DisabledSources = append(discovery.DisabledSources, source)
			continue
		}
		if source.Status == StatusBlocked {
			discovery.Diagnostics = append(discovery.Diagnostics, Diagnostic{Code: CodeCatalogInvalid, Severity: "error", Message: source.BlockedReason, Ref: source.Name, Path: source.Path})
			continue
		}
		discovery.Sources = append(discovery.Sources, source)
		switch source.SourceKind {
		case SourceKindBundled:
			for _, target := range v2recipe.ListBundledTargets() {
				candidate := candidateFromBundled(source, target)
				ensureApp(appsByID, candidate.TargetID).Default = candidate
			}
		case SourceKindLocal:
			scan, err := scanLocalCatalog(source)
			if err != nil {
				discovery.Diagnostics = append(discovery.Diagnostics, Diagnostic{Code: CodeCatalogScanFailure, Severity: "error", Message: err.Error(), Ref: source.Name, Path: source.Path})
				continue
			}
			if len(scan.invalid) > 0 {
				discovery.Diagnostics = append(discovery.Diagnostics, Diagnostic{Code: CodeCatalogInvalid, Severity: "error", Message: fmt.Sprintf("catalog %s has %d invalid support definition%s", source.Name, len(scan.invalid), plural(len(scan.invalid))), Ref: source.Name, Path: source.Path})
				continue
			}
			for _, candidate := range scan.candidates {
				app := ensureApp(appsByID, candidate.TargetID)
				if app.Default.TargetID == "" {
					candidate.SelectedBy = "local-only"
					app.Default = candidate
				} else if app.Default.SourceKind == SourceKindBundled {
					candidate.SelectedBy = "candidate"
					app.Candidates = append(app.Candidates, candidate)
				} else {
					if !app.Ambiguous {
						previous := app.Default
						previous.SelectedBy = "source-choice-required"
						app.Candidates = append([]RecipeCandidate{previous}, app.Candidates...)
					}
					app.Ambiguous = true
					candidate.SelectedBy = "source-choice-required"
					app.Candidates = append(app.Candidates, candidate)
					app.Default = ambiguousLocalCandidate(app.Default, app.Candidates)
				}
			}
		default:
			return nil, &Error{Code: CodeUnsupportedSource, Message: fmt.Sprintf("unknown sourceKind for catalog %s: %s", source.Name, source.SourceKind), Exit: 2}
		}
	}
	for _, app := range appsByID {
		sort.Slice(app.Candidates, func(i, j int) bool {
			if app.Candidates[i].SourceName != app.Candidates[j].SourceName {
				return app.Candidates[i].SourceName < app.Candidates[j].SourceName
			}
			return app.Candidates[i].RecipeRef < app.Candidates[j].RecipeRef
		})
		discovery.Apps = append(discovery.Apps, *app)
	}
	sort.Slice(discovery.Apps, func(i, j int) bool { return discovery.Apps[i].ID < discovery.Apps[j].ID })
	sort.Slice(discovery.Sources, func(i, j int) bool { return discovery.Sources[i].Name < discovery.Sources[j].Name })
	sort.Slice(discovery.DisabledSources, func(i, j int) bool { return discovery.DisabledSources[i].Name < discovery.DisabledSources[j].Name })
	return discovery, nil
}

func JSON(report *Report) (string, error) {
	if report == nil {
		report = baseReport(ListCommand)
		report.Summary.Status = "error"
	}
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	return string(payload) + "\n", nil
}

func Text(report *Report) string {
	if report == nil {
		return "Catalogs\n\nCommand result:\n  The command could not complete.\n"
	}
	if report.Error != nil {
		return errorText(report)
	}
	switch report.Command {
	case ListCommand:
		return listText(report)
	case AddCommand:
		return addText(report)
	case DisableCommand:
		return disableText(report)
	case EnableCommand:
		return enableText(report)
	case RemoveCommand:
		return removeText(report)
	default:
		return listText(report)
	}
}

func stateFilePath(stateRoot string) string {
	return filepath.Join(stateRoot, "catalogs", "sources.yaml")
}

func baseReport(command string) *Report {
	return &Report{
		Schema:        Schema,
		SchemaVersion: SchemaVersion,
		Command:       command,
		RunID:         strings.ReplaceAll(command, ".", "-"),
		Summary:       Summary{Status: "ok"},
		Sources:       []Source{},
		Validated:     []ValidationRef{},
		Invalid:       []InvalidRecipe{},
		Diagnostics:   []Diagnostic{},
	}
}

func failReport(report *Report, err *Error) *Report {
	if report == nil {
		report = baseReport(ListCommand)
	}
	report.Summary.Status = "error"
	report.Summary.Failed = 1
	if err != nil {
		report.Error = &ErrorObject{Code: err.Code, Message: err.Message, Details: err.Details}
	}
	return report
}

func catalogScanFailure(report *Report, name string, err error) (*Report, error) {
	catErr := &Error{Code: CodeCatalogScanFailure, Message: err.Error(), Exit: 2, Details: map[string]any{"name": name}}
	return failReport(report, catErr), catErr
}

func loadState(opts Options) (stateFile, error) {
	stateRoot, err := resolveStateRootIfPossible(opts)
	if err != nil {
		return stateFile{}, err
	}
	if stateRoot == "" {
		return emptyState(), nil
	}
	return readStateAt(stateRoot)
}

func resolveStateRootIfPossible(opts Options) (string, error) {
	if strings.TrimSpace(opts.StateRoot) != "" || strings.TrimSpace(opts.RepoRoot) != "" {
		return resolveStateRoot(opts)
	}
	return "", nil
}

func resolveStateRoot(opts Options) (string, error) {
	if strings.TrimSpace(opts.StateRoot) != "" {
		return filepath.Abs(strings.TrimSpace(opts.StateRoot))
	}
	if strings.TrimSpace(opts.RepoRoot) == "" {
		return "", fmt.Errorf("repo root or state root is required")
	}
	return v2ledger.DefaultStateRoot(opts.RepoRoot)
}

func emptyState() stateFile {
	return stateFile{Schema: StateSchema, SchemaVersion: SchemaVersion, Sources: []storedSource{}}
}

func readStateAt(stateRoot string) (stateFile, error) {
	path := stateFilePath(stateRoot)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return emptyState(), nil
	}
	if err != nil {
		return stateFile{}, fmt.Errorf("read catalog state %s: %w", path, err)
	}
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return stateFile{}, fmt.Errorf("parse catalog state %s: %w", path, err)
	}
	var state stateFile
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&state); err != nil {
		return stateFile{}, fmt.Errorf("parse catalog state %s: %w", path, err)
	}
	if err := validateState(state); err != nil {
		return stateFile{}, err
	}
	return state, nil
}

func writeStateAt(stateRoot string, state stateFile) error {
	state.Schema = StateSchema
	state.SchemaVersion = SchemaVersion
	sortStoredSources(state.Sources)
	payload, err := yaml.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode catalog state: %w", err)
	}
	path := stateFilePath(stateRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create catalog state directory: %w", err)
	}
	tmp := fmt.Sprintf("%s.tmp-%d", path, time.Now().UnixNano())
	if err := os.WriteFile(tmp, payload, 0o644); err != nil {
		return fmt.Errorf("write catalog state temp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace catalog state: %w", err)
	}
	return nil
}

func lockStateRoot(stateRoot string) (func(), error) {
	lockPath := filepath.Join(stateRoot, "catalogs", ".sources.lock")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, fmt.Errorf("create catalog lock directory: %w", err)
	}
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open catalog lock: %w", err)
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("lock catalog state: %w", err)
	}
	return func() {
		_ = unix.Flock(int(file.Fd()), unix.LOCK_UN)
		_ = file.Close()
	}, nil
}

func validateState(state stateFile) error {
	if state.Schema != StateSchema {
		return fmt.Errorf("invalid catalog state schema: %q", state.Schema)
	}
	if state.SchemaVersion != SchemaVersion {
		return fmt.Errorf("invalid catalog state schemaVersion: %d", state.SchemaVersion)
	}
	seen := map[string]bool{}
	for _, source := range state.Sources {
		if err := v2recipe.ValidatePublicID("catalog", source.Name); err != nil {
			return err
		}
		if isBuiltInName(source.Name) {
			return fmt.Errorf("catalog state must not store built-in source")
		}
		if seen[source.Name] {
			return fmt.Errorf("duplicate catalog source: %s", source.Name)
		}
		seen[source.Name] = true
		if source.SourceKind != SourceKindLocal {
			return fmt.Errorf("unknown sourceKind for catalog %s: %s", source.Name, source.SourceKind)
		}
		if source.Status != StatusEnabled && source.Status != StatusDisabled && source.Status != StatusBlocked && source.Status != StatusRemoved {
			return fmt.Errorf("unknown status for catalog %s: %s", source.Name, source.Status)
		}
		if source.SourceID != "local:"+source.Name {
			return fmt.Errorf("local catalog %s sourceId must match local:%s", source.Name, source.Name)
		}
		if source.CatalogID != "local."+source.Name {
			return fmt.Errorf("local catalog %s catalogId must match local.%s", source.Name, source.Name)
		}
		if source.DisplayName == "" {
			return fmt.Errorf("local catalog %s displayName is required", source.Name)
		}
		if source.SourceAcceptance != "user-accepted" {
			return fmt.Errorf("unknown sourceAcceptance for catalog %s: %s", source.Name, source.SourceAcceptance)
		}
		if source.IntegrityState != "not-required" {
			return fmt.Errorf("unknown integrityState for catalog %s: %s", source.Name, source.IntegrityState)
		}
		if source.WriteDefault != "denied" {
			return fmt.Errorf("unknown writeDefault for catalog %s: %s", source.Name, source.WriteDefault)
		}
		if source.UpdatePolicy != "manual" {
			return fmt.Errorf("unknown updatePolicy for catalog %s: %s", source.Name, source.UpdatePolicy)
		}
		if strings.TrimSpace(source.Path) == "" {
			return fmt.Errorf("local catalog %s path is required", source.Name)
		}
		if !filepath.IsAbs(source.Path) {
			return fmt.Errorf("local catalog %s path must be absolute", source.Name)
		}
		if filepath.Clean(source.Path) != source.Path {
			return fmt.Errorf("local catalog %s path must be normalized", source.Name)
		}
		if source.PinnedIdentity != source.Path {
			return fmt.Errorf("local catalog %s pinnedIdentity must match path", source.Name)
		}
		if source.OriginURI != fileURI(source.Path) {
			return fmt.Errorf("local catalog %s originUri must match path", source.Name)
		}
	}
	return nil
}

func sourcesFromState(opts Options, state stateFile, includeRecipeCounts bool) ([]Source, error) {
	return sourcesFromStateWithRemoved(opts, state, includeRecipeCounts, false)
}

func sourcesFromStateWithRemoved(opts Options, state stateFile, includeRecipeCounts bool, includeRemoved bool) ([]Source, error) {
	sources := []Source{builtInSource(opts.ManagerVersion)}
	if settingsFolder, ok := settingsFolderSource(opts.RepoRoot); ok {
		if includeRecipeCounts {
			settingsFolder = sourceWithRecipeCounts(settingsFolder)
		}
		sources = append(sources, settingsFolder)
	}
	for _, stored := range state.Sources {
		if stored.Status == StatusRemoved && !includeRemoved {
			continue
		}
		source := sourceFromStored(stored)
		if includeRecipeCounts {
			source = sourceWithRecipeCounts(source)
		}
		sources = append(sources, source)
	}
	sort.Slice(sources, func(i, j int) bool {
		if sources[i].Name == BuiltInName {
			return true
		}
		if sources[j].Name == BuiltInName {
			return false
		}
		return sources[i].Name < sources[j].Name
	})
	return sources, nil
}

func sourceWithRecipeCounts(source Source) Source {
	scan, err := scanLocalCatalog(source)
	if err != nil {
		if source.Status == StatusEnabled {
			source.Status = StatusBlocked
			source.BlockedReason = err.Error()
		}
		return source
	}
	source.ValidRecipes = len(scan.candidates)
	source.InvalidRecipes = len(scan.invalid)
	source.HiddenRecipes = len(scan.candidates)
	source.KnownTargets = targetsFromCandidates(scan.candidates)
	if len(scan.invalid) > 0 && source.Status == StatusEnabled {
		source.Status = StatusBlocked
		source.BlockedReason = fmt.Sprintf("catalog %s has %d invalid support definition%s", source.Name, len(scan.invalid), plural(len(scan.invalid)))
	}
	return source
}

func builtInSource(version string) Source {
	version = strings.TrimSpace(version)
	if version == "" {
		version = "current"
	}
	return Source{
		Name:             BuiltInName,
		SourceID:         BuiltInSourceID,
		SourceKind:       SourceKindBundled,
		CatalogID:        BuiltInCatalogID,
		DisplayName:      BuiltInDisplayName,
		OriginURI:        "app-release://dotfiles-manager/" + version + "/recipes",
		Status:           StatusEnabled,
		SourceAcceptance: "release-accepted",
		IntegrityState:   "not-required",
		WriteDefault:     "allowed",
		UpdatePolicy:     "release-only",
		ValidRecipes:     len(v2recipe.ListBundledTargets()),
	}
}

func localSource(name string, absPath string) Source {
	return Source{
		Name:             name,
		SourceID:         "local:" + name,
		SourceKind:       SourceKindLocal,
		CatalogID:        "local." + name,
		DisplayName:      name,
		OriginURI:        fileURI(absPath),
		Status:           StatusEnabled,
		SourceAcceptance: "user-accepted",
		IntegrityState:   "not-required",
		WriteDefault:     "denied",
		UpdatePolicy:     "manual",
		Path:             absPath,
		PinnedIdentity:   absPath,
	}
}

func settingsFolderSource(repoRoot string) (Source, bool) {
	root := strings.TrimSpace(repoRoot)
	if root == "" {
		return Source{}, false
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return Source{}, false
	}
	path := filepath.Join(abs, "recipes", "local")
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return Source{}, false
	}
	return Source{
		Name:             SettingsFolderName,
		SourceID:         "local",
		SourceKind:       SourceKindLocal,
		CatalogID:        "local.settings-folder",
		DisplayName:      "settings-folder local recipes",
		OriginURI:        fileURI(path),
		Status:           StatusEnabled,
		SourceAcceptance: "review-required",
		IntegrityState:   "not-required",
		WriteDefault:     "denied",
		UpdatePolicy:     "manual",
		Path:             path,
		PinnedIdentity:   path,
		SettingsFolder:   true,
	}, true
}

func storedFromSource(source Source) storedSource {
	return storedSource{
		Name:             source.Name,
		SourceID:         source.SourceID,
		SourceKind:       source.SourceKind,
		CatalogID:        source.CatalogID,
		DisplayName:      source.DisplayName,
		OriginURI:        source.OriginURI,
		Status:           source.Status,
		SourceAcceptance: source.SourceAcceptance,
		IntegrityState:   source.IntegrityState,
		WriteDefault:     source.WriteDefault,
		UpdatePolicy:     source.UpdatePolicy,
		Path:             source.Path,
		PinnedIdentity:   source.PinnedIdentity,
		KnownTargets:     append([]string(nil), source.KnownTargets...),
	}
}

func sourceFromStored(stored storedSource) Source {
	return Source{
		Name:             stored.Name,
		SourceID:         stored.SourceID,
		SourceKind:       stored.SourceKind,
		CatalogID:        stored.CatalogID,
		DisplayName:      stored.DisplayName,
		OriginURI:        stored.OriginURI,
		Status:           stored.Status,
		SourceAcceptance: stored.SourceAcceptance,
		IntegrityState:   stored.IntegrityState,
		WriteDefault:     stored.WriteDefault,
		UpdatePolicy:     stored.UpdatePolicy,
		Path:             stored.Path,
		PinnedIdentity:   stored.PinnedIdentity,
		KnownTargets:     append([]string(nil), stored.KnownTargets...),
	}
}

type scanResult struct {
	candidates []RecipeCandidate
	invalid    []InvalidRecipe
}

func scanLocalCatalog(source Source) (scanResult, error) {
	if source.SourceKind != SourceKindLocal {
		return scanResult{}, fmt.Errorf("scan requires local source: %s", source.Name)
	}
	info, err := os.Stat(source.Path)
	if err != nil {
		return scanResult{}, fmt.Errorf("stat local catalog %s: %w", source.Path, err)
	}
	if !info.IsDir() {
		return scanResult{}, fmt.Errorf("local catalog path is not a directory: %s", source.Path)
	}
	entries, err := os.ReadDir(source.Path)
	if err != nil {
		return scanResult{}, fmt.Errorf("read local catalog %s: %w", source.Path, err)
	}
	var result scanResult
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		targetID := entry.Name()
		recipePath := filepath.Join(source.Path, targetID, "recipe.yaml")
		if _, err := os.Stat(recipePath); errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return result, fmt.Errorf("stat local recipe %s: %w", recipePath, err)
		}
		candidate, invalid := loadLocalCandidate(source, targetID, recipePath)
		if invalid != nil {
			result.invalid = append(result.invalid, *invalid)
			continue
		}
		result.candidates = append(result.candidates, candidate)
	}
	sort.Slice(result.candidates, func(i, j int) bool { return result.candidates[i].TargetID < result.candidates[j].TargetID })
	sort.Slice(result.invalid, func(i, j int) bool { return result.invalid[i].TargetID < result.invalid[j].TargetID })
	return result, nil
}

func loadLocalCandidate(source Source, dirTargetID string, recipePath string) (RecipeCandidate, *InvalidRecipe) {
	if err := v2recipe.ValidatePublicID("target", dirTargetID); err != nil {
		return RecipeCandidate{}, &InvalidRecipe{TargetID: dirTargetID, Path: recipePath, Errors: []string{err.Error()}}
	}
	file, err := os.Open(recipePath)
	if err != nil {
		return RecipeCandidate{}, &InvalidRecipe{TargetID: dirTargetID, Path: recipePath, Errors: []string{err.Error()}}
	}
	defer func() { _ = file.Close() }()
	rec, err := v2recipe.Decode(recipePath, file)
	if err != nil {
		return RecipeCandidate{}, &InvalidRecipe{TargetID: dirTargetID, Path: recipePath, Errors: []string{err.Error()}}
	}
	if rec.Target != dirTargetID {
		return RecipeCandidate{}, &InvalidRecipe{TargetID: dirTargetID, Path: recipePath, Errors: []string{fmt.Sprintf("recipe target %q must match catalog directory %q", rec.Target, dirTargetID)}}
	}
	digest, _ := fileDigest(recipePath)
	candidate := RecipeCandidate{
		TargetID:          rec.Target,
		DisplayName:       displayName(rec.DisplayName, rec.Target),
		RecipeRef:         localRecipeRef(source.Name, rec.Target),
		SourceName:        source.Name,
		SourceKind:        source.SourceKind,
		SourceID:          source.SourceID,
		CatalogID:         source.CatalogID,
		SourceDisplayName: source.DisplayName,
		OriginURI:         source.OriginURI,
		SelectedBy:        "local-only",
		TrustStatus:       "untrusted",
		WriteAuthority:    "requires-approval",
		ReviewStatus:      "machine-validated",
		SupportLevel:      rec.SupportLevel,
		Capability:        rec.Capability,
		PlatformSupport:   platformSupport(rec),
		Summary:           displayName(rec.DisplayName, rec.Target) + " local support",
		RecipePath:        recipePath,
		RecipeDigest:      digest,
		Settings:          settingsFromRecipe(rec),
		Recipe:            rec,
	}
	return candidate, nil
}

func candidateFromBundled(source Source, target v2recipe.BundledTarget) RecipeCandidate {
	return RecipeCandidate{
		TargetID:          target.ID,
		DisplayName:       displayName(target.DisplayName, target.ID),
		Aliases:           append([]string(nil), target.Aliases...),
		RecipeRef:         target.RecipeRef,
		SourceName:        BuiltInName,
		SourceKind:        SourceKindBundled,
		SourceID:          source.SourceID,
		CatalogID:         source.CatalogID,
		SourceDisplayName: source.DisplayName,
		OriginURI:         source.OriginURI,
		SelectedBy:        "bundled-default",
		TrustStatus:       target.TrustStatus,
		WriteAuthority:    "allowed",
		ReviewStatus:      "release-accepted",
		SupportLevel:      target.SupportLevel,
		Capability:        target.Capability,
		PlatformSupport:   target.PlatformSupport,
		Summary:           target.Summary,
	}
}

func settingsFromRecipe(rec *v2recipe.Recipe) []SettingSummary {
	if rec == nil {
		return nil
	}
	ids := make([]string, 0, len(rec.Settings))
	for id := range rec.Settings {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	settings := make([]SettingSummary, 0, len(ids))
	for _, id := range ids {
		setting := rec.Settings[id]
		resource := rec.Resources[setting.Resource]
		settings = append(settings, SettingSummary{
			Ref:          rec.Target + ":" + id,
			ID:           id,
			Label:        displayName(setting.Label, id),
			DefaultScope: setting.ScopeDefault,
			ResourceID:   setting.Resource,
			Driver:       resource.Driver,
			LiveLocation: liveLocation(rec, resource),
		})
	}
	return settings
}

func liveLocation(rec *v2recipe.Recipe, resource v2recipe.Resource) string {
	if rec == nil || strings.TrimSpace(resource.Location) == "" {
		return ""
	}
	location := rec.Locations[resource.Location]
	base := strings.TrimSpace(location.Default)
	path := strings.TrimSpace(resource.Path)
	if base == "" && path == "" {
		return ""
	}
	if strings.HasPrefix(base, "~/") {
		base = "$HOME/" + strings.TrimPrefix(base, "~/")
	} else if base == "~" {
		base = "$HOME"
	}
	if path == "" {
		return base
	}
	if base == "" {
		return path
	}
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(path, "/")
}

func platformSupport(rec *v2recipe.Recipe) string {
	if rec == nil {
		return "unknown"
	}
	for _, op := range rec.NativeOperations {
		if len(op.Platforms) > 0 {
			return strings.Join(op.Platforms, ",")
		}
	}
	return "cross-platform"
}

func validationRefs(candidates []RecipeCandidate) []ValidationRef {
	refs := make([]ValidationRef, 0, len(candidates))
	bundled := bundledTargetIDs()
	for _, candidate := range candidates {
		role := "local support"
		if bundled[candidate.TargetID] {
			role = "local candidate; built-in support remains the default"
		}
		refs = append(refs, ValidationRef{
			TargetID:     candidate.TargetID,
			DisplayName:  candidate.DisplayName,
			RecipeRef:    candidate.RecipeRef,
			SourceName:   candidate.SourceName,
			SourceKind:   candidate.SourceKind,
			SourceID:     candidate.SourceID,
			CatalogID:    candidate.CatalogID,
			OriginURI:    candidate.OriginURI,
			Role:         role,
			RecipePath:   candidate.RecipePath,
			RecipeDigest: candidate.RecipeDigest,
		})
	}
	return refs
}

func bundledTargetIDs() map[string]bool {
	out := map[string]bool{}
	for _, target := range v2recipe.ListBundledTargets() {
		out[target.ID] = true
		for _, alias := range target.Aliases {
			out[alias] = true
		}
	}
	return out
}

func setEnabled(opts Options, rawName string, status string, command string) (*Report, error) {
	report := baseReport(command)
	trimmedName := strings.TrimSpace(rawName)
	if isBuiltInName(trimmedName) {
		verb := "modified"
		if status == StatusEnabled {
			verb = "enabled"
		} else if status == StatusDisabled {
			verb = "disabled"
		}
		catErr := &Error{Code: CodeBuiltInImmutable, Message: fmt.Sprintf("built-in catalog cannot be %s", verb), Exit: 2, Details: map[string]any{"name": trimmedName}}
		return failReport(report, catErr), catErr
	}
	name, err := validateCatalogName(trimmedName)
	if err != nil {
		catErr := &Error{Code: CodeNameInvalid, Message: err.Error(), Exit: 2, Details: map[string]any{"name": rawName}}
		return failReport(report, catErr), catErr
	}
	if isBuiltInName(name) {
		verb := "modified"
		if status == StatusEnabled {
			verb = "enabled"
		} else if status == StatusDisabled {
			verb = "disabled"
		}
		catErr := &Error{Code: CodeBuiltInImmutable, Message: fmt.Sprintf("built-in catalog cannot be %s", verb), Exit: 2, Details: map[string]any{"name": name}}
		return failReport(report, catErr), catErr
	}
	stateRoot, err := resolveStateRoot(opts)
	if err != nil {
		catErr := &Error{Code: CodeStateRootInvalid, Message: err.Error(), Exit: 2}
		return failReport(report, catErr), catErr
	}
	unlock, err := lockStateRoot(stateRoot)
	if err != nil {
		catErr := &Error{Code: CodeStateWriteFailed, Message: err.Error(), Exit: 1}
		return failReport(report, catErr), catErr
	}
	defer unlock()
	state, err := readStateAt(stateRoot)
	if err != nil {
		catErr := classifyStateError(err)
		return failReport(report, catErr), catErr
	}
	idx := storedSourceIndex(state.Sources, name)
	if idx < 0 {
		catErr := &Error{Code: CodeCatalogNotFound, Message: fmt.Sprintf("local catalog not found: %s", name), Exit: 2, Details: map[string]any{"name": name}}
		return failReport(report, catErr), catErr
	}
	if state.Sources[idx].Status == StatusRemoved {
		catErr := &Error{Code: CodeCatalogNotFound, Message: fmt.Sprintf("local catalog not found: %s", name), Exit: 2, Details: map[string]any{"name": name}}
		return failReport(report, catErr), catErr
	}
	changed := sourceFromStored(state.Sources[idx])
	scan, scanErr := scanLocalCatalog(changed)
	if status == StatusEnabled {
		if scanErr != nil {
			catErr := &Error{Code: CodeCatalogScanFailure, Message: scanErr.Error(), Exit: 2, Details: map[string]any{"name": name}}
			return failReport(report, catErr), catErr
		}
		if len(scan.invalid) > 0 {
			report.Invalid = scan.invalid
			report.Summary.Status = "error"
			report.Summary.InvalidRecipes = len(scan.invalid)
			report.Summary.Failed = 1
			catErr := &Error{Code: CodeCatalogInvalid, Message: fmt.Sprintf("catalog %s has invalid support definitions", name), Exit: 2, Details: map[string]any{"name": name, "invalid": len(scan.invalid)}}
			report.Error = &ErrorObject{Code: catErr.Code, Message: catErr.Message, Details: catErr.Details}
			return report, catErr
		}
	}
	state.Sources[idx].Status = status
	if scanErr == nil {
		if targets := targetsFromCandidates(scan.candidates); len(targets) > 0 {
			state.Sources[idx].KnownTargets = targets
		}
	}
	changed = sourceFromStored(state.Sources[idx])
	changed.ValidRecipes = len(scan.candidates)
	changed.HiddenRecipes = len(scan.candidates)
	if err := writeStateAt(stateRoot, state); err != nil {
		catErr := &Error{Code: CodeStateWriteFailed, Message: err.Error(), Exit: 1}
		return failReport(report, catErr), catErr
	}
	report.ChangedSource = changed
	report.Validated = validationRefs(scan.candidates)
	report.Summary.Status = "ok"
	report.Summary.HiddenRecipes = len(scan.candidates)
	report.Summary.ValidRecipes = len(scan.candidates)
	return report, nil
}

func classifyStateError(err error) *Error {
	var catErr *Error
	if errors.As(err, &catErr) {
		return catErr
	}
	if err == nil {
		return nil
	}
	message := err.Error()
	code := CodeStateInvalid
	if strings.Contains(message, "read catalog state") {
		code = CodeStateReadFailed
	}
	return &Error{Code: code, Message: message, Exit: 2}
}

func summarizeSources(sources []Source) Summary {
	summary := Summary{Status: "ok", Sources: len(sources)}
	for _, source := range sources {
		switch source.Status {
		case StatusEnabled:
			summary.Enabled++
		case StatusDisabled:
			summary.Disabled++
		}
		summary.ValidRecipes += source.ValidRecipes
		summary.InvalidRecipes += source.InvalidRecipes
		if source.Status == StatusDisabled {
			summary.HiddenRecipes += source.HiddenRecipes
		}
	}
	return summary
}

func ambiguousLocalCandidate(previous RecipeCandidate, candidates []RecipeCandidate) RecipeCandidate {
	candidate := previous
	candidate.SourceName = "multiple"
	candidate.SourceDisplayName = "multiple local catalogs"
	candidate.SourceID = "local:multiple"
	candidate.CatalogID = "local.multiple"
	candidate.OriginURI = ""
	candidate.RecipeRef = ""
	candidate.SelectedBy = "source-choice-required"
	candidate.TrustStatus = "untrusted"
	candidate.WriteAuthority = "blocked-source-choice-required"
	candidate.ReviewStatus = "source-choice-required"
	candidate.Settings = nil
	candidate.Recipe = nil
	if len(candidates) > 0 {
		candidate.DisplayName = candidates[0].DisplayName
		candidate.TargetID = candidates[0].TargetID
	}
	return candidate
}

func ensureApp(apps map[string]*EffectiveApp, id string) *EffectiveApp {
	app := apps[id]
	if app == nil {
		app = &EffectiveApp{ID: id, Candidates: []RecipeCandidate{}}
		apps[id] = app
	}
	return app
}

func validateCatalogName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", fmt.Errorf("catalog name is required")
	}
	if err := v2recipe.ValidatePublicID("catalog", name); err != nil {
		return "", err
	}
	if isBuiltInName(name) || name == SettingsFolderName || name == "bundled" || name == "remote" {
		return "", fmt.Errorf("reserved catalog name: %s", name)
	}
	return name, nil
}

func isBuiltInName(name string) bool {
	return name == BuiltInName || name == BuiltInSourceID
}

func looksLikeRemote(raw string) bool {
	value := strings.TrimSpace(raw)
	if value == "" {
		return false
	}
	lower := strings.ToLower(value)
	if strings.Contains(lower, "://") || strings.HasPrefix(lower, "git@") || strings.HasPrefix(lower, "ssh:") {
		return true
	}
	if githubOwnerRepoPattern.MatchString(value) && !strings.HasPrefix(value, "./") && !strings.HasPrefix(value, "../") && !strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "~/") {
		return true
	}
	return false
}

func resolveLocalPath(raw string) (string, error) {
	path := strings.TrimSpace(raw)
	if path == "" {
		return "", fmt.Errorf("local catalog path is required")
	}
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		if path == "~" {
			path = home
		} else if strings.HasPrefix(path, "~/") {
			path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve local catalog path %q: %w", raw, err)
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolve local catalog path %q: %w", raw, err)
	}
	info, err := os.Stat(real)
	if err != nil {
		return "", fmt.Errorf("stat local catalog path %q: %w", raw, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("local catalog path is not a directory: %s", real)
	}
	return real, nil
}

func findStoredSource(sources []storedSource, name string) *storedSource {
	idx := storedSourceIndex(sources, name)
	if idx < 0 {
		return nil
	}
	return &sources[idx]
}

func storedSourceIndex(sources []storedSource, name string) int {
	for i := range sources {
		if sources[i].Name == name {
			return i
		}
	}
	return -1
}

func sortStoredSources(sources []storedSource) {
	sort.Slice(sources, func(i, j int) bool { return sources[i].Name < sources[j].Name })
}

func fileDigest(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func fileURI(path string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}

func localRecipeRef(sourceName string, targetID string) string {
	if sourceName == SettingsFolderName {
		return "recipe://local/" + targetID
	}
	return "recipe://local/" + sourceName + "/" + targetID
}

func targetsFromCandidates(candidates []RecipeCandidate) []string {
	targets := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.TargetID != "" {
			targets = append(targets, candidate.TargetID)
		}
	}
	sort.Strings(targets)
	return targets
}

func targetInKnownTargets(targets []string, targetID string) bool {
	for _, target := range targets {
		if target == targetID {
			return true
		}
	}
	return false
}

func displayName(value string, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed != "" {
		return trimmed
	}
	fallback = strings.TrimSpace(fallback)
	if fallback == "" {
		return "App"
	}
	parts := strings.Fields(strings.NewReplacer(".", " ", "-", " ", "_", " ").Replace(fallback))
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
	}
	return strings.Join(parts, " ")
}

func listText(report *Report) string {
	lines := []string{"Catalogs", ""}
	for _, source := range report.Sources {
		if source.SourceKind == SourceKindBundled {
			lines = append(lines, fmt.Sprintf("  %-9s Built in  %s", source.Name, source.Status))
			lines = append(lines, "    Source: ships with dotfiles-manager")
			lines = append(lines, "    Updates: with dotfiles-manager releases")
			lines = append(lines, "    Network: not used")
			lines = append(lines, "    Removable: no")
			continue
		}
		if source.SourceKind == SourceKindLocal {
			lines = append(lines, fmt.Sprintf("  %-9s Local     %s", source.Name, source.Status))
			lines = append(lines, "    Source: "+source.Path)
			lines = append(lines, fmt.Sprintf("    Support: %d valid, %d invalid", source.ValidRecipes, source.InvalidRecipes))
			if source.BlockedReason != "" {
				lines = append(lines, "    Reason: "+source.BlockedReason)
			}
			lines = append(lines, "    Writes: local support requires write approval before live settings can change")
			lines = append(lines, "    Network: not used")
		}
	}
	if localSourceCount(report.Sources) == 0 {
		lines = append(lines, "", "Local catalogs: none")
	}
	lines = append(lines, "Remote catalogs: not supported yet")
	lines = append(lines, "", "No live settings were read or changed.", "No stored settings were changed.")
	return strings.Join(trimBlank(lines), "\n")
}

func addText(report *Report) string {
	name := report.ChangedSource.Name
	lines := []string{"Added local catalog: " + name, "", "Source:", "  " + report.ChangedSource.Path, "", "Validated support:"}
	for _, item := range report.Validated {
		lines = append(lines, fmt.Sprintf("  %-13s %s", item.TargetID, item.Role))
	}
	lines = append(lines, "", "Network: not used", "No live settings were read or changed.", "No stored settings were changed.")
	return strings.Join(trimBlank(lines), "\n")
}

func disableText(report *Report) string {
	lines := []string{"Disabled local catalog: " + report.ChangedSource.Name, ""}
	if len(report.Validated) > 0 {
		lines = append(lines, "No longer available from this catalog:")
		for _, item := range report.Validated {
			label := item.TargetID
			if strings.Contains(item.Role, "candidate") {
				label += " local candidate"
			}
			lines = append(lines, "  "+label)
		}
		lines = append(lines, "")
	}
	lines = append(lines, "Nothing was deleted.", "Live app settings were not changed.", "Stored settings were not changed.", "", "If a managed app depends on this catalog, it will show as source unavailable until you enable the catalog, add another source, or stop managing that app.")
	return strings.Join(trimBlank(lines), "\n")
}

func enableText(report *Report) string {
	lines := []string{"Enabled local catalog: " + report.ChangedSource.Name, "", "Validated support:"}
	for _, item := range report.Validated {
		lines = append(lines, fmt.Sprintf("  %-13s %s", item.TargetID, item.Role))
	}
	lines = append(lines, "", "No live settings were read or changed.", "No stored settings were changed.")
	return strings.Join(trimBlank(lines), "\n")
}

func removeText(report *Report) string {
	lines := []string{"Removed local catalog: " + report.ChangedSource.Name, "", "Forgotten by dotfiles-manager:", "  " + report.ChangedSource.Path, "", "Nothing was deleted from that folder.", "Live app settings were not changed.", "Stored settings were not changed.", "", "Apps that depended on this catalog are now source unavailable until you re-add this catalog, choose another source, or stop managing those apps."}
	return strings.Join(trimBlank(lines), "\n")
}

func errorText(report *Report) string {
	name := report.ChangedSource.Name
	if name == "" && report.Error != nil && report.Error.Details != nil {
		if raw, ok := report.Error.Details["name"].(string); ok {
			name = raw
		}
		if raw, ok := report.Error.Details["source"].(string); ok && name == "" {
			name = raw
		}
	}
	if report.Command == AddCommand && report.Error.Code == CodeRemoteUnsupported {
		return strings.Join([]string{
			"Catalog not added: " + name,
			"",
			"Reason:",
			"  Remote GitHub catalogs are not supported in this version of dotfiles-manager.",
			"",
			"For now, use a local catalog folder:",
			"  dotfiles-manager catalog add ./custom-recipes --name personal",
			"",
			"Remote catalog trust, updates, and write gates are planned separately.",
			"No live settings were read or changed.",
			"No stored settings were changed.",
		}, "\n")
	}
	if report.Command == AddCommand && report.Error.Code == CodeCatalogInvalid {
		lines := []string{"Catalog not added: " + name, "", "Reason:", fmt.Sprintf("  %d support definition%s failed validation.", len(report.Invalid), plural(len(report.Invalid))), "", "Invalid support:"}
		for _, invalid := range report.Invalid {
			lines = append(lines, "  "+invalid.TargetID)
			for _, msg := range invalid.Errors {
				lines = append(lines, "    - "+msg)
			}
		}
		lines = append(lines, "", "No live settings were read or changed.", "No stored settings were changed.")
		return strings.Join(trimBlank(lines), "\n")
	}
	message := "The command could not complete."
	if report.Error != nil {
		message = report.Error.Message
	}
	return strings.Join([]string{"Catalogs", "", "Command result:", "  " + message, "", "No live settings were read or changed.", "No stored settings were changed.", "", "Run with --json for machine-readable details."}, "\n")
}

func localSourceCount(sources []Source) int {
	count := 0
	for _, source := range sources {
		if source.SourceKind == SourceKindLocal {
			count++
		}
	}
	return count
}

func plural(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

func trimBlank(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
