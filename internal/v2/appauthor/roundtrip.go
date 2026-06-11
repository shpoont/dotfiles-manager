package appauthor

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/shpoont/dotfiles-manager/internal/v2/desired"
	"github.com/shpoont/dotfiles-manager/internal/v2/filedriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/inidriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/jsondriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/plistdriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/shpoont/dotfiles-manager/internal/v2/selectedvalue"
	"github.com/shpoont/dotfiles-manager/internal/v2/tomldriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/yamldriver"
	"gopkg.in/yaml.v3"
)

const (
	TestRoundtripSchema  = "dotfiles-manager.v2.app.test-roundtrip"
	TestRoundtripCommand = "app.test-roundtrip"
	TestRoundtripRunID   = "app-test-roundtrip"
)

const (
	CodeRoundtripModeRequired = "app.test.roundtrip.required"
	CodeFixtureNone           = "app.test.fixture.none"
	CodeFixtureMissing        = "app.test.fixture.missing"
	CodeFixtureInvalid        = "app.test.fixture.invalid"
	CodeFixtureUnsafe         = "app.test.fixture.unsafe"
	CodeManifestInvalid       = "app.test.manifest.invalid"
	CodeNoRunnableCases       = "app.test.no-runnable-cases"
	CodeDriverUnsupported     = "app.test.driver.unsupported"
	CodeNativeValidateOnly    = "app.test.native.validate-only"
	CodeLifecycleUnsupported  = "app.test.lifecycle.unsupported"
	CodeRoundtripMismatch     = "app.test.roundtrip.mismatch"
	CodeRoundtripFailed       = "app.test.roundtrip.failed"
)

const (
	roundtripFixtureSchema       = "dotfiles-manager.v2.app.roundtrip-fixture"
	roundtripSchemaVersion       = 1
	defaultFixtureUser           = "fixture-user"
	defaultFixtureMachine        = "fixture-machine"
	maxFixtureFiles              = 200
	maxFixtureBytes        int64 = 1 << 20
)

const (
	fixtureReasonOK                 = "ok"
	fixtureReasonFixtureInvalid     = "fixture-invalid"
	fixtureReasonRecipeInvalid      = "recipe-invalid"
	fixtureReasonUnsupportedDriver  = "unsupported-driver"
	fixtureReasonNativeValidateOnly = "native-validate-only"
	fixtureReasonRoundtripMismatch  = "roundtrip-mismatch"
	fixtureReasonSafetyBlocked      = "safety-blocked"
	fixtureReasonNoRunnableCases    = "no-runnable-cases"
)

type TestRoundtripOptions struct {
	RepoRoot  string
	TargetID  string
	Roundtrip bool
	Fixture   string
}

type TestRoundtripReport struct {
	Schema           string               `json:"schema"`
	SchemaVersion    int                  `json:"schemaVersion"`
	Command          string               `json:"command"`
	RunID            string               `json:"runId"`
	Summary          TestRoundtripSummary `json:"summary"`
	AppTestRoundtrip TestRoundtripResult  `json:"appTestRoundtrip"`
	Diagnostics      []Diagnostic         `json:"diagnostics"`
	Error            *ErrorObject         `json:"error"`
}

type TestRoundtripSummary struct {
	Status  string `json:"status"`
	Cases   int    `json:"cases"`
	Passed  int    `json:"passed"`
	Skipped int    `json:"skipped"`
	Blocked int    `json:"blocked"`
	Failed  int    `json:"failed"`
}

type TestRoundtripResult struct {
	Target   TargetInfo         `json:"target"`
	Fixtures []RoundtripFixture `json:"fixtures"`
}

type RoundtripFixture struct {
	Name   string          `json:"name"`
	Status string          `json:"status"`
	Reason string          `json:"reason"`
	Modes  []string        `json:"modes,omitempty"`
	Cases  []RoundtripCase `json:"cases,omitempty"`
}

type RoundtripCase struct {
	Setting  string `json:"setting"`
	Resource string `json:"resource"`
	Driver   string `json:"driver"`
	Save     string `json:"save,omitempty"`
	Apply    string `json:"apply,omitempty"`
}

type roundtripManifest struct {
	Schema        string                    `yaml:"schema"`
	SchemaVersion int                       `yaml:"schemaVersion"`
	Target        string                    `yaml:"target"`
	Name          string                    `yaml:"name,omitempty"`
	Synthetic     *bool                     `yaml:"synthetic"`
	Description   string                    `yaml:"description,omitempty"`
	Modes         []string                  `yaml:"modes,omitempty"`
	Settings      []string                  `yaml:"settings,omitempty"`
	Subjects      roundtripManifestSubjects `yaml:"subjects,omitempty"`
}

type roundtripManifestSubjects struct {
	User    string `yaml:"user,omitempty"`
	Machine string `yaml:"machine,omitempty"`
}

type roundtripFixturePlan struct {
	Name           string
	RelRoot        string
	AbsRoot        string
	Manifest       roundtripManifest
	Modes          []string
	Settings       []string
	SubjectUser    string
	SubjectMachine string
}

type roundtripScratch struct {
	Root     string
	LiveRoot string
}

func RunTestRoundtrip(opts TestRoundtripOptions) (*TestRoundtripReport, error) {
	report := baseTestRoundtripReport()
	repoRoot, err := normalizeRepoRoot(opts.RepoRoot)
	if err != nil {
		return failTestRoundtrip(report, CodeRepoInvalid, err.Error(), 2, nil)
	}
	targetID, err := normalizeTargetID(opts.TargetID)
	if err != nil {
		return failTestRoundtrip(report, CodeTargetInvalid, err.Error(), 2, nil)
	}
	report.AppTestRoundtrip.Target = TargetInfo{ID: targetID, RecipeRef: recipeRef(targetID)}
	if !opts.Roundtrip {
		return failTestRoundtrip(report, CodeRoundtripModeRequired, "app test requires --roundtrip in this tranche", 2, nil)
	}
	if err := checkBundledCollision(targetID); err != nil {
		return failTestRoundtrip(report, CodeTargetCollision, err.Error(), 2, map[string]any{"target": targetID})
	}
	if err := validateLocalRecipeReadTarget(repoRoot, targetID); err != nil {
		diagnostic := Diagnostic{Code: errorCode(err), Severity: SeverityError, Message: err.Error(), Path: localRecipeRelPath(targetID)}
		report.Diagnostics = append(report.Diagnostics, diagnostic)
		return failTestRoundtripWithExistingDiagnostics(report, errorCode(err), err.Error(), errorExit(err, 2), errorDetails(err))
	}
	rec, err := loadLocalRecipe(repoRoot, targetID)
	if err != nil {
		code := CodeRecipeInvalid
		if errors.Is(err, os.ErrNotExist) {
			code = CodeRecipeMissing
		}
		diagnostics := diagnosticsFromRecipeError(err)
		if len(diagnostics) == 0 {
			diagnostics = []Diagnostic{{Code: code, Severity: SeverityError, Message: err.Error(), Path: localRecipeRelPath(targetID)}}
		}
		report.Diagnostics = append(report.Diagnostics, diagnostics...)
		return failTestRoundtripWithExistingDiagnostics(report, code, err.Error(), 2, nil)
	}
	report.AppTestRoundtrip.Target.DisplayName = rec.DisplayName
	if rec.Target != targetID {
		message := fmt.Sprintf("local recipe target %s does not match requested target %s", rec.Target, targetID)
		report.Diagnostics = append(report.Diagnostics, Diagnostic{Code: CodeTargetMismatch, Severity: SeverityError, Message: message, Path: "$.target"})
		return failTestRoundtripWithExistingDiagnostics(report, CodeTargetMismatch, message, 2, map[string]any{"requestedTarget": targetID, "recipeTarget": rec.Target})
	}
	writeDiagnostics := authoringWriteSafetyDiagnostics(rec)
	report.Diagnostics = append(report.Diagnostics, writeDiagnostics...)
	if hasBlockingDiagnostics(writeDiagnostics) {
		return failTestRoundtripWithExistingDiagnostics(report, CodeRecipeInvalid, "recipe authoring metadata is not safe for roundtrip testing", 2, nil)
	}

	plans, err := discoverRoundtripFixtures(repoRoot, targetID, strings.TrimSpace(opts.Fixture))
	if err != nil {
		code := errorCode(err)
		report.Diagnostics = append(report.Diagnostics, Diagnostic{Code: code, Severity: SeverityError, Message: err.Error()})
		return failTestRoundtripWithExistingDiagnostics(report, code, err.Error(), errorExit(err, 2), errorDetails(err))
	}

	for _, plan := range plans {
		fixture := runRoundtripFixture(repoRoot, rec, plan, &report.Diagnostics)
		report.AppTestRoundtrip.Fixtures = append(report.AppTestRoundtrip.Fixtures, fixture)
	}
	finishTestRoundtrip(report)
	if report.Summary.Status == "ok" {
		return report, nil
	}
	code := CodeRoundtripFailed
	exit := 2
	switch {
	case containsDiagnosticCode(report.Diagnostics, CodeFixtureInvalid):
		code = CodeFixtureInvalid
	case containsDiagnosticCode(report.Diagnostics, CodeManifestInvalid):
		code = CodeManifestInvalid
	case containsDiagnosticCode(report.Diagnostics, CodeRoundtripFailed):
		code = CodeRoundtripFailed
	case report.Summary.Failed > 0:
		code = CodeRoundtripMismatch
	}
	if report.Summary.Passed > 0 && (report.Summary.Failed > 0 || report.Summary.Blocked > 0 || report.Summary.Skipped > 0) {
		exit = 6
	}
	if containsDiagnosticCode(report.Diagnostics, CodeFixtureUnsafe) || containsDiagnosticCode(report.Diagnostics, CodePathUnsafe) {
		exit = 5
	}
	if report.Summary.Cases == 0 || (report.Summary.Passed == 0 && report.Summary.Failed == 0 && report.Summary.Skipped > 0) || containsDiagnosticCode(report.Diagnostics, CodeNoRunnableCases) {
		code = CodeNoRunnableCases
		exit = 2
	}
	return failTestRoundtripWithExistingDiagnostics(report, code, "roundtrip fixtures did not all pass", exit, nil)
}

func JSONTestRoundtrip(report *TestRoundtripReport) (string, error) {
	if report == nil {
		report = baseTestRoundtripReport()
		report.Summary.Status = "error"
	}
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	return string(payload) + "\n", nil
}

func TextTestRoundtrip(report *TestRoundtripReport) string {
	if report == nil {
		return "app test --roundtrip\nsummary status=error cases=0 passed=0 skipped=0 blocked=0 failed=1"
	}
	lines := []string{"app test --roundtrip"}
	if report.AppTestRoundtrip.Target.ID != "" {
		lines = append(lines, fmt.Sprintf("target: %s (%s)", report.AppTestRoundtrip.Target.ID, report.AppTestRoundtrip.Target.RecipeRef))
	}
	for _, fixture := range report.AppTestRoundtrip.Fixtures {
		lines = append(lines, fmt.Sprintf("%s: %s (%s)", fixture.Name, fixture.Status, fixture.Reason))
		for _, item := range fixture.Cases {
			parts := []string{}
			if item.Save != "" {
				parts = append(parts, "save="+item.Save)
			}
			if item.Apply != "" {
				parts = append(parts, "apply="+item.Apply)
			}
			lines = append(lines, fmt.Sprintf("  %s %s %s", item.Setting, item.Driver, strings.Join(parts, " ")))
		}
	}
	appendDiagnostics(&lines, report.Diagnostics)
	lines = append(lines, fmt.Sprintf("summary status=%s cases=%d passed=%d skipped=%d blocked=%d failed=%d", report.Summary.Status, report.Summary.Cases, report.Summary.Passed, report.Summary.Skipped, report.Summary.Blocked, report.Summary.Failed))
	return strings.Join(lines, "\n")
}

func discoverRoundtripFixtures(repoRoot string, targetID string, fixtureName string) ([]roundtripFixturePlan, error) {
	roundtripRel := path.Join(localRecipeRelRoot(targetID), "fixtures/roundtrip")
	roundtripAbs := filepath.Join(repoRoot, filepath.FromSlash(roundtripRel))
	if fixtureName != "" {
		if err := validateFixtureName(fixtureName); err != nil {
			return nil, err
		}
		plan, err := loadRoundtripFixture(repoRoot, targetID, fixtureName)
		if err != nil {
			return nil, err
		}
		return []roundtripFixturePlan{plan}, nil
	}
	if err := validateFixturePathComponents(repoRoot, roundtripRel, true); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(roundtripAbs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, typedError(CodeFixtureNone, fmt.Sprintf("no roundtrip fixtures found at %s", roundtripRel), 2, map[string]any{"path": roundtripRel})
		}
		return nil, typedError(CodeFixtureUnsafe, fmt.Sprintf("read roundtrip fixtures at %s: %v", roundtripRel, err), 5, map[string]any{"path": roundtripRel})
	}
	names := []string{}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			return nil, typedError(CodeFixtureUnsafe, fmt.Sprintf("fixture path %s is a symlink", path.Join(roundtripRel, entry.Name())), 5, map[string]any{"path": path.Join(roundtripRel, entry.Name())})
		}
		if !entry.IsDir() {
			return nil, typedError(CodeFixtureInvalid, fmt.Sprintf("fixture path %s is not a directory", path.Join(roundtripRel, entry.Name())), 2, map[string]any{"path": path.Join(roundtripRel, entry.Name())})
		}
		if err := validateFixtureName(entry.Name()); err != nil {
			return nil, err
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, typedError(CodeFixtureNone, fmt.Sprintf("no roundtrip fixtures found at %s", roundtripRel), 2, map[string]any{"path": roundtripRel})
	}
	plans := make([]roundtripFixturePlan, 0, len(names))
	for _, name := range names {
		plan, err := loadRoundtripFixture(repoRoot, targetID, name)
		if err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	return plans, nil
}

func loadRoundtripFixture(repoRoot string, targetID string, fixtureName string) (roundtripFixturePlan, error) {
	rel := path.Join(localRecipeRelRoot(targetID), "fixtures/roundtrip", fixtureName)
	if err := validateFixturePathComponents(repoRoot, rel, false); err != nil {
		if errorCode(err) == CodeRecipeMissing {
			return roundtripFixturePlan{}, typedError(CodeFixtureMissing, fmt.Sprintf("roundtrip fixture %s was not found", fixtureName), 2, map[string]any{"fixture": fixtureName})
		}
		return roundtripFixturePlan{}, err
	}
	if err := validateFixtureTree(filepath.Join(repoRoot, filepath.FromSlash(rel)), rel); err != nil {
		return roundtripFixturePlan{}, err
	}
	manifest, err := loadRoundtripManifest(filepath.Join(repoRoot, filepath.FromSlash(rel), "manifest.yaml"), targetID, fixtureName)
	if err != nil {
		return roundtripFixturePlan{}, err
	}
	modes := manifest.Modes
	if len(modes) == 0 {
		modes = []string{"save", "apply"}
	}
	user := manifest.Subjects.User
	if user == "" {
		user = defaultFixtureUser
	}
	machine := manifest.Subjects.Machine
	if machine == "" {
		machine = defaultFixtureMachine
	}
	return roundtripFixturePlan{Name: fixtureName, RelRoot: rel, AbsRoot: filepath.Join(repoRoot, filepath.FromSlash(rel)), Manifest: manifest, Modes: modes, Settings: append([]string(nil), manifest.Settings...), SubjectUser: user, SubjectMachine: machine}, nil
}

func validateFixtureName(name string) error {
	if err := recipe.ValidatePublicID("fixture", name); err != nil {
		return typedError(CodeFixtureInvalid, err.Error(), 2, map[string]any{"fixture": name})
	}
	return nil
}

func validateFixturePathComponents(repoRoot string, rel string, allowMissingRoundtrip bool) error {
	parts := strings.Split(rel, "/")
	current := repoRoot
	for idx, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		currentRel := path.Join(parts[:idx+1]...)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				if allowMissingRoundtrip && idx == len(parts)-1 {
					return typedError(CodeFixtureNone, fmt.Sprintf("no roundtrip fixtures found at %s", rel), 2, map[string]any{"path": rel})
				}
				return typedError(CodeFixtureMissing, fmt.Sprintf("fixture path %s not found", currentRel), 2, map[string]any{"path": currentRel})
			}
			return typedError(CodeFixtureUnsafe, fmt.Sprintf("cannot inspect fixture path %s", currentRel), 5, map[string]any{"path": currentRel})
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return typedError(CodeFixtureUnsafe, fmt.Sprintf("fixture path %s is a symlink", currentRel), 5, map[string]any{"path": currentRel})
		}
		if !info.IsDir() {
			return typedError(CodeFixtureInvalid, fmt.Sprintf("fixture path %s is not a directory", currentRel), 2, map[string]any{"path": currentRel})
		}
	}
	return nil
}

func validateFixtureTree(root string, relRoot string) error {
	var files int
	var bytesSeen int64
	return filepath.WalkDir(root, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return typedError(CodeFixtureUnsafe, fmt.Sprintf("cannot inspect fixture path %s", relFromRoot(root, current, relRoot)), 5, map[string]any{"path": relFromRoot(root, current, relRoot)})
		}
		info, err := entry.Info()
		if err != nil {
			return typedError(CodeFixtureUnsafe, fmt.Sprintf("cannot inspect fixture path %s", relFromRoot(root, current, relRoot)), 5, map[string]any{"path": relFromRoot(root, current, relRoot)})
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return typedError(CodeFixtureUnsafe, fmt.Sprintf("fixture path %s is a symlink", relFromRoot(root, current, relRoot)), 5, map[string]any{"path": relFromRoot(root, current, relRoot)})
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return typedError(CodeFixtureUnsafe, fmt.Sprintf("fixture path %s is not a regular file", relFromRoot(root, current, relRoot)), 5, map[string]any{"path": relFromRoot(root, current, relRoot)})
		}
		files++
		bytesSeen += info.Size()
		if files > maxFixtureFiles {
			return typedError(CodeFixtureUnsafe, "roundtrip fixture contains too many files", 5, map[string]any{"limit": maxFixtureFiles})
		}
		if bytesSeen > maxFixtureBytes {
			return typedError(CodeFixtureUnsafe, "roundtrip fixture is too large", 5, map[string]any{"limitBytes": maxFixtureBytes})
		}
		return nil
	})
}

func loadRoundtripManifest(path string, targetID string, fixtureName string) (roundtripManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return roundtripManifest{}, typedError(CodeManifestInvalid, "roundtrip fixture manifest.yaml is required", 2, nil)
		}
		return roundtripManifest{}, typedError(CodeFixtureUnsafe, "cannot read roundtrip fixture manifest", 5, nil)
	}
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return roundtripManifest{}, typedError(CodeManifestInvalid, fmt.Sprintf("parse fixture manifest: %v", err), 2, nil)
	}
	if err := rejectDuplicateYAMLKeys(&node); err != nil {
		return roundtripManifest{}, typedError(CodeManifestInvalid, err.Error(), 2, nil)
	}
	var manifest roundtripManifest
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&manifest); err != nil {
		return roundtripManifest{}, typedError(CodeManifestInvalid, fmt.Sprintf("decode fixture manifest: %v", err), 2, nil)
	}
	if manifest.Schema != roundtripFixtureSchema {
		return roundtripManifest{}, typedError(CodeManifestInvalid, "fixture manifest schema is invalid", 2, map[string]any{"schema": manifest.Schema})
	}
	if manifest.SchemaVersion != roundtripSchemaVersion {
		return roundtripManifest{}, typedError(CodeManifestInvalid, "fixture manifest schemaVersion is invalid", 2, map[string]any{"schemaVersion": manifest.SchemaVersion})
	}
	if manifest.Target != targetID {
		return roundtripManifest{}, typedError(CodeManifestInvalid, "fixture manifest target does not match requested target", 2, map[string]any{"target": manifest.Target})
	}
	if manifest.Name != "" && manifest.Name != fixtureName {
		return roundtripManifest{}, typedError(CodeManifestInvalid, "fixture manifest name does not match fixture directory", 2, map[string]any{"name": manifest.Name, "fixture": fixtureName})
	}
	if manifest.Synthetic == nil || !*manifest.Synthetic {
		return roundtripManifest{}, typedError(CodeManifestInvalid, "fixture manifest requires synthetic: true", 2, nil)
	}
	if strings.ContainsAny(manifest.Description, "\x00\r\n") {
		return roundtripManifest{}, typedError(CodeManifestInvalid, "fixture manifest description must be a single safe line", 2, nil)
	}
	if err := validateModes(manifest.Modes); err != nil {
		return roundtripManifest{}, err
	}
	if err := validateIDs("setting", manifest.Settings); err != nil {
		return roundtripManifest{}, err
	}
	if manifest.Subjects.User != "" {
		if err := recipe.ValidatePublicID("fixture user", manifest.Subjects.User); err != nil {
			return roundtripManifest{}, typedError(CodeManifestInvalid, err.Error(), 2, nil)
		}
	}
	if manifest.Subjects.Machine != "" {
		if err := recipe.ValidatePublicID("fixture machine", manifest.Subjects.Machine); err != nil {
			return roundtripManifest{}, typedError(CodeManifestInvalid, err.Error(), 2, nil)
		}
	}
	return manifest, nil
}

func validateModes(modes []string) error {
	seen := map[string]bool{}
	for _, mode := range modes {
		if mode != "save" && mode != "apply" {
			return typedError(CodeManifestInvalid, fmt.Sprintf("unsupported fixture mode %s", mode), 2, map[string]any{"mode": mode})
		}
		if seen[mode] {
			return typedError(CodeManifestInvalid, fmt.Sprintf("duplicate fixture mode %s", mode), 2, map[string]any{"mode": mode})
		}
		seen[mode] = true
	}
	return nil
}

func validateIDs(kind string, values []string) error {
	seen := map[string]bool{}
	for _, value := range values {
		if err := recipe.ValidatePublicID(kind, value); err != nil {
			return typedError(CodeManifestInvalid, err.Error(), 2, map[string]any{kind: value})
		}
		if seen[value] {
			return typedError(CodeManifestInvalid, fmt.Sprintf("duplicate %s %s", kind, value), 2, map[string]any{kind: value})
		}
		seen[value] = true
	}
	return nil
}

func runRoundtripFixture(repoRoot string, rec *recipe.Recipe, plan roundtripFixturePlan, diagnostics *[]Diagnostic) RoundtripFixture {
	fixture := RoundtripFixture{Name: plan.Name, Status: "passed", Reason: fixtureReasonOK, Modes: append([]string(nil), plan.Modes...), Cases: []RoundtripCase{}}
	settings, err := fixtureSettings(rec, plan.Settings)
	if err != nil {
		*diagnostics = append(*diagnostics, Diagnostic{Code: CodeManifestInvalid, Severity: SeverityError, Message: err.Error(), Path: path.Join(plan.RelRoot, "manifest.yaml")})
		fixture.Status = "blocked"
		fixture.Reason = fixtureReasonFixtureInvalid
		return fixture
	}
	for _, settingID := range settings {
		setting := rec.Settings[settingID]
		resourceID, resource, err := rec.ResourceForSetting(settingID)
		if err != nil {
			*diagnostics = append(*diagnostics, Diagnostic{Code: CodeRecipeInvalid, Severity: SeverityError, Message: err.Error(), Path: "$.settings." + settingID})
			fixture.Cases = append(fixture.Cases, RoundtripCase{Setting: settingID, Resource: resourceID, Driver: resource.Driver, Save: "blocked", Apply: "blocked"})
			continue
		}
		caseResult := RoundtripCase{Setting: settingID, Resource: resourceID, Driver: resource.Driver}
		if err := roundtripSettingSupported(rec, setting, resource); err != nil {
			status := "blocked"
			reason := errorCode(err)
			if reason == CodeNativeValidateOnly || reason == CodeDriverUnsupported {
				status = "skipped"
			}
			if hasMode(plan.Modes, "save") {
				caseResult.Save = status
			}
			if hasMode(plan.Modes, "apply") {
				caseResult.Apply = status
			}
			fixture.Cases = append(fixture.Cases, caseResult)
			*diagnostics = append(*diagnostics, Diagnostic{Code: reason, Severity: SeverityError, Message: err.Error(), Path: "$.resources." + resourceID + ".driver"})
			continue
		}
		if hasMode(plan.Modes, "save") {
			if err := runSaveRoundtrip(repoRoot, rec, settingID, resourceID, resource, plan); err != nil {
				caseResult.Save = "failed"
				*diagnostics = append(*diagnostics, Diagnostic{Code: errorCode(err), Severity: SeverityError, Message: err.Error(), Path: errorPath(err)})
			} else {
				caseResult.Save = "passed"
			}
		}
		if hasMode(plan.Modes, "apply") {
			if err := runApplyRoundtrip(repoRoot, rec, settingID, resourceID, resource, plan); err != nil {
				caseResult.Apply = "failed"
				*diagnostics = append(*diagnostics, Diagnostic{Code: errorCode(err), Severity: SeverityError, Message: err.Error(), Path: errorPath(err)})
			} else {
				caseResult.Apply = "passed"
			}
		}
		fixture.Cases = append(fixture.Cases, caseResult)
	}
	finishFixture(&fixture)
	return fixture
}

func fixtureSettings(rec *recipe.Recipe, selected []string) ([]string, error) {
	if len(selected) > 0 {
		out := make([]string, 0, len(selected))
		for _, settingID := range selected {
			if _, ok := rec.Settings[settingID]; !ok {
				return nil, fmt.Errorf("fixture references unknown setting %s", settingID)
			}
			out = append(out, settingID)
		}
		return out, nil
	}
	out := make([]string, 0, len(rec.Settings))
	for settingID := range rec.Settings {
		out = append(out, settingID)
	}
	sort.Strings(out)
	return out, nil
}

func roundtripSettingSupported(rec *recipe.Recipe, setting recipe.Setting, resource recipe.Resource) error {
	if err := lifecycleSupported(effectiveLifecycle(rec, setting, resource)); err != nil {
		return err
	}
	if err := roundtripRedactionSafety(setting.Sensitivity, setting.Redaction, "setting"); err != nil {
		return err
	}
	if err := roundtripRedactionSafety(resource.Sensitivity, resource.Redaction, "resource"); err != nil {
		return err
	}
	switch resource.Driver {
	case recipe.FileDriverID, recipe.IniFileDriverID, recipe.JSONFileDriverID, recipe.YAMLFileDriverID, recipe.TOMLFileDriverID, recipe.PlistFileDriverID:
		return nil
	case recipe.NativeExportDriverID:
		return typedError(CodeNativeValidateOnly, "native-export fixtures are validate-only in app test --roundtrip", 2, nil)
	default:
		return typedError(CodeDriverUnsupported, fmt.Sprintf("driver %s is not supported by app test --roundtrip", resource.Driver), 2, nil)
	}
}

func effectiveLifecycle(rec *recipe.Recipe, setting recipe.Setting, resource recipe.Resource) string {
	if setting.Lifecycle != "" {
		return setting.Lifecycle
	}
	if resource.Lifecycle != "" {
		return resource.Lifecycle
	}
	return ""
}

func lifecycleSupported(lifecycle string) error {
	switch lifecycle {
	case "", recipe.LifecycleAllowed, recipe.LifecycleWarn:
		return nil
	default:
		return typedError(CodeLifecycleUnsupported, fmt.Sprintf("lifecycle %s is not supported by fixture roundtrip", lifecycle), 2, nil)
	}
}

func roundtripRedactionSafety(sensitivity string, redaction string, subject string) error {
	switch sensitivity {
	case recipe.SensitivitySecret, recipe.SensitivityUnknown:
		return typedError(CodeFixtureInvalid, fmt.Sprintf("%s sensitivity %s is not allowed in roundtrip fixtures", subject, sensitivity), 2, nil)
	}
	switch redaction {
	case recipe.RedactionBlockedSave, recipe.RedactionUnavailable:
		return typedError(CodeFixtureInvalid, fmt.Sprintf("%s redaction %s is not allowed in roundtrip fixtures", subject, redaction), 2, nil)
	}
	return nil
}

func runSaveRoundtrip(repoRoot string, rec *recipe.Recipe, settingID string, resourceID string, resource recipe.Resource, plan roundtripFixturePlan) error {
	scratch, cleanup, err := prepareRoundtripScratch(plan)
	if err != nil {
		return err
	}
	defer cleanup()
	if resource.Driver == recipe.FileDriverID {
		if err := saveFileRoundtrip(rec, settingID, resource, plan, scratch); err != nil {
			return err
		}
	} else {
		if err := saveSelectedRoundtrip(rec, settingID, plan, scratch); err != nil {
			return err
		}
	}
	return compareExpectedTree(filepath.Join(scratch.Root, "desired"), filepath.Join(plan.AbsRoot, "expected/desired"), "expected/desired")
}

func runApplyRoundtrip(repoRoot string, rec *recipe.Recipe, settingID string, resourceID string, resource recipe.Resource, plan roundtripFixturePlan) error {
	scratch, cleanup, err := prepareRoundtripScratch(plan)
	if err != nil {
		return err
	}
	defer cleanup()
	if resource.Driver == recipe.FileDriverID {
		if err := applyFileRoundtrip(rec, settingID, resource, plan, scratch); err != nil {
			return err
		}
	} else {
		if err := applySelectedRoundtrip(rec, settingID, plan, scratch); err != nil {
			return err
		}
	}
	return compareExpectedTree(filepath.Join(scratch.Root, "live"), filepath.Join(plan.AbsRoot, "expected/live"), "expected/live")
}

func prepareRoundtripScratch(plan roundtripFixturePlan) (roundtripScratch, func(), error) {
	root, err := os.MkdirTemp("", "dotfiles-manager-app-test-*")
	if err != nil {
		return roundtripScratch{}, func() {}, typedError(CodeFixtureUnsafe, "create roundtrip temp directory", 5, nil)
	}
	cleanup := func() { _ = os.RemoveAll(root) }
	if err := copyFixtureSubtree(filepath.Join(plan.AbsRoot, "input/live"), filepath.Join(root, "live")); err != nil {
		cleanup()
		return roundtripScratch{}, func() {}, err
	}
	if err := copyFixtureSubtree(filepath.Join(plan.AbsRoot, "input/desired"), filepath.Join(root, "desired")); err != nil {
		cleanup()
		return roundtripScratch{}, func() {}, err
	}
	return roundtripScratch{Root: root, LiveRoot: filepath.Join(root, "live")}, cleanup, nil
}

func saveFileRoundtrip(rec *recipe.Recipe, settingID string, resource recipe.Resource, plan roundtripFixturePlan, scratch roundtripScratch) error {
	target, err := liveFileTarget(resource, scratch)
	if err != nil {
		return err
	}
	state, err := filedriver.Driver{}.ReadCurrent(target)
	if err != nil {
		return typedError(CodeRoundtripFailed, fmt.Sprintf("read fixture live file for %s: %v", settingID, err), 2, map[string]any{"path": resource.Path})
	}
	if !state.Exists {
		return typedError(CodeRoundtripFailed, fmt.Sprintf("fixture live file for %s is missing", settingID), 2, map[string]any{"path": resource.Path})
	}
	desiredTarget, err := desiredFileTarget(rec.Target, settingID, settingScope(rec, settingID), plan, scratch.Root)
	if err != nil {
		return err
	}
	_, err = filedriver.Driver{}.Apply(desiredTarget, state)
	if err != nil {
		return typedError(CodeRoundtripFailed, fmt.Sprintf("write fixture desired artifact for %s: %v", settingID, err), 2, map[string]any{"path": desiredTarget.RelPath})
	}
	return nil
}

func applyFileRoundtrip(rec *recipe.Recipe, settingID string, resource recipe.Resource, plan roundtripFixturePlan, scratch roundtripScratch) error {
	desiredTarget, err := desiredFileTarget(rec.Target, settingID, settingScope(rec, settingID), plan, scratch.Root)
	if err != nil {
		return err
	}
	state, err := filedriver.Driver{}.ReadCurrent(desiredTarget)
	if err != nil {
		return typedError(CodeRoundtripFailed, fmt.Sprintf("read fixture desired artifact for %s: %v", settingID, err), 2, map[string]any{"path": desiredTarget.RelPath})
	}
	if !state.Exists {
		return typedError(CodeRoundtripFailed, fmt.Sprintf("fixture desired artifact for %s is missing", settingID), 2, map[string]any{"path": desiredTarget.RelPath})
	}
	liveTarget, err := liveFileTarget(resource, scratch)
	if err != nil {
		return err
	}
	_, err = filedriver.Driver{}.Apply(liveTarget, state)
	if err != nil {
		return typedError(CodeRoundtripFailed, fmt.Sprintf("apply fixture desired artifact for %s: %v", settingID, err), 2, map[string]any{"path": resource.Path})
	}
	return nil
}

func saveSelectedRoundtrip(rec *recipe.Recipe, settingID string, plan roundtripFixturePlan, scratch roundtripScratch) error {
	current, err := selectedvalue.ReadCurrentDesired(selectedvalue.Request{Recipe: rec, SettingRef: settingID, LocationRoots: fixtureLocationRoots(rec, scratch)})
	if err != nil {
		return typedError(CodeRoundtripFailed, fmt.Sprintf("read fixture selected value for %s: %v", settingID, err), 2, map[string]any{"setting": settingID})
	}
	if current.Desired.Intent() == "" {
		return typedError(CodeRoundtripFailed, fmt.Sprintf("fixture selected value for %s is missing", settingID), 2, map[string]any{"setting": settingID})
	}
	uri := desiredSettingsURI(rec.Target, settingID, settingScope(rec, settingID), plan)
	if err := writeFixtureSelectedValue(scratch.Root, uri, settingID, current.Desired); err != nil {
		return typedError(CodeRoundtripFailed, fmt.Sprintf("write fixture desired selected value for %s: %v", settingID, err), 2, map[string]any{"setting": settingID})
	}
	return nil
}

func applySelectedRoundtrip(rec *recipe.Recipe, settingID string, plan roundtripFixturePlan, scratch roundtripScratch) error {
	read, err := desired.ReadSelectedValue(scratch.Root, desiredSettingsURI(rec.Target, settingID, settingScope(rec, settingID), plan))
	if err != nil {
		return typedError(CodeRoundtripFailed, fmt.Sprintf("read fixture desired selected value for %s: %v", settingID, err), 2, map[string]any{"setting": settingID})
	}
	if read.Desired == nil {
		return typedError(CodeRoundtripFailed, fmt.Sprintf("fixture desired selected value for %s is missing", settingID), 2, map[string]any{"setting": settingID})
	}
	_, resource, err := rec.ResourceForSetting(settingID)
	if err != nil {
		return typedError(CodeRecipeInvalid, err.Error(), 2, map[string]any{"setting": settingID})
	}
	err = applySelectedDesiredToLive(resource, scratch, *read.Desired)
	if err != nil {
		return typedError(CodeRoundtripFailed, fmt.Sprintf("apply fixture selected value for %s: %v", settingID, err), 2, map[string]any{"setting": settingID})
	}
	return nil
}

type fixtureSettingsFile struct {
	Schema        string                         `yaml:"schema"`
	SchemaVersion int                            `yaml:"schemaVersion"`
	Values        map[string]fixtureSettingValue `yaml:"values"`
}

type fixtureSettingValue struct {
	Intent string `yaml:"intent"`
	Kind   string `yaml:"kind,omitempty"`
	Value  any    `yaml:"value,omitempty"`
}

func writeFixtureSelectedValue(repoRoot string, uri string, settingID string, value selectedvalue.Desired) error {
	resolved, err := desired.ResolveURI(repoRoot, uri)
	if err != nil {
		return err
	}
	if resolved.Object != desired.ObjectSettings || resolved.SettingID != settingID {
		return fmt.Errorf("fixture selected-value writes require a matching settings URI")
	}
	if err := ensureNoSymlinkPath(repoRoot, resolved.Path); err != nil {
		return err
	}
	settings := fixtureSettingsFile{Schema: desired.SettingsSchema, SchemaVersion: desired.SchemaVersion, Values: map[string]fixtureSettingValue{}}
	if data, err := os.ReadFile(resolved.Path); err == nil {
		var node yaml.Node
		if err := yaml.Unmarshal(data, &node); err != nil {
			return err
		}
		if err := rejectDuplicateYAMLKeys(&node); err != nil {
			return err
		}
		decoder := yaml.NewDecoder(bytes.NewReader(data))
		decoder.KnownFields(true)
		if err := decoder.Decode(&settings); err != nil {
			return err
		}
		if settings.Schema != desired.SettingsSchema || settings.SchemaVersion != desired.SchemaVersion {
			return fmt.Errorf("fixture desired settings schema is invalid")
		}
		if settings.Values == nil {
			settings.Values = map[string]fixtureSettingValue{}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	entry, err := fixtureSettingValueFromSelected(value)
	if err != nil {
		return err
	}
	settings.Values[settingID] = entry
	var marshaled strings.Builder
	encoder := yaml.NewEncoder(&marshaled)
	encoder.SetIndent(2)
	if err := encoder.Encode(settings); err != nil {
		return err
	}
	if err := encoder.Close(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(resolved.Path), 0o755); err != nil {
		return err
	}
	if err := ensureNoSymlinkPath(repoRoot, resolved.Path); err != nil {
		return err
	}
	return os.WriteFile(resolved.Path, []byte(marshaled.String()), 0o644)
}

func fixtureSettingValueFromSelected(value selectedvalue.Desired) (fixtureSettingValue, error) {
	switch value.Intent() {
	case selectedvalue.IntentSet:
		raw, ok := value.Value()
		if !ok {
			return fixtureSettingValue{}, fmt.Errorf("selected fixture value is missing")
		}
		switch value.Kind() {
		case desired.KindString:
			typed, ok := raw.(string)
			if !ok {
				return fixtureSettingValue{}, fmt.Errorf("selected fixture string value has invalid internal representation")
			}
			return fixtureSettingValue{Intent: desired.IntentSet, Kind: desired.KindString, Value: typed}, nil
		case desired.KindBool:
			typed, ok := raw.(bool)
			if !ok {
				return fixtureSettingValue{}, fmt.Errorf("selected fixture bool value has invalid internal representation")
			}
			return fixtureSettingValue{Intent: desired.IntentSet, Kind: desired.KindBool, Value: typed}, nil
		case desired.KindNumber:
			typed, ok := raw.(json.Number)
			if !ok {
				return fixtureSettingValue{}, fmt.Errorf("selected fixture number value has invalid internal representation")
			}
			return fixtureSettingValue{Intent: desired.IntentSet, Kind: desired.KindNumber, Value: typed}, nil
		case desired.KindNull:
			return fixtureSettingValue{Intent: desired.IntentSet, Kind: desired.KindNull}, nil
		default:
			return fixtureSettingValue{}, fmt.Errorf("selected fixture value kind is unsupported")
		}
	case selectedvalue.IntentDelete:
		return fixtureSettingValue{Intent: desired.IntentDelete}, nil
	default:
		return fixtureSettingValue{}, fmt.Errorf("selected fixture value intent is unsupported")
	}
}

func applySelectedDesiredToLive(resource recipe.Resource, scratch roundtripScratch, value selectedvalue.Desired) error {
	target, err := liveFileTarget(resource, scratch)
	if err != nil {
		return err
	}
	switch resource.Driver {
	case recipe.IniFileDriverID:
		state, err := iniStateFromSelected(value)
		if err != nil {
			return err
		}
		_, err = inidriver.Driver{}.Apply(inidriver.Request{Target: target, Selector: iniSelector(resource.Selector)}, state)
		return err
	case recipe.JSONFileDriverID:
		state, err := jsonStateFromSelected(value)
		if err != nil {
			return err
		}
		_, err = jsondriver.Driver{}.Apply(jsondriver.Request{Target: target, Selector: jsonSelector(resource.Selector)}, state)
		return err
	case recipe.YAMLFileDriverID:
		state, err := yamlStateFromSelected(value)
		if err != nil {
			return err
		}
		_, err = yamldriver.Driver{}.Apply(yamldriver.Request{Target: target, Selector: yamlSelector(resource.Selector)}, state)
		return err
	case recipe.TOMLFileDriverID:
		state, err := tomlStateFromSelected(value)
		if err != nil {
			return err
		}
		_, err = tomldriver.Driver{}.Apply(tomldriver.Request{Target: target, Selector: tomlSelector(resource.Selector)}, state)
		return err
	case recipe.PlistFileDriverID:
		state, err := plistStateFromSelected(value)
		if err != nil {
			return err
		}
		_, err = plistdriver.Driver{}.Apply(plistdriver.Request{Target: target, Selector: plistSelector(resource.Selector)}, state)
		return err
	default:
		return fmt.Errorf("driver %s is not supported by fixture selected-value apply", resource.Driver)
	}
}

func iniStateFromSelected(value selectedvalue.Desired) (inidriver.State, error) {
	switch value.Intent() {
	case selectedvalue.IntentSet:
		raw, ok := value.Value()
		if !ok || value.Kind() != desired.KindString {
			return inidriver.State{}, fmt.Errorf("ini selected-value fixtures support string set or delete only")
		}
		typed, ok := raw.(string)
		if !ok {
			return inidriver.State{}, fmt.Errorf("ini selected fixture string value has invalid internal representation")
		}
		return inidriver.Driver{}.Normalize(typed), nil
	case selectedvalue.IntentDelete:
		return inidriver.DeleteState(), nil
	default:
		return inidriver.State{}, fmt.Errorf("selected fixture value intent is unsupported")
	}
}

func jsonStateFromSelected(value selectedvalue.Desired) (jsondriver.State, error) {
	if value.Intent() == selectedvalue.IntentDelete {
		return jsondriver.DeleteState(), nil
	}
	scalar, err := selectedScalar(value)
	if err != nil {
		return jsondriver.State{}, err
	}
	return jsondriver.Driver{}.NormalizeValue(scalar)
}

func yamlStateFromSelected(value selectedvalue.Desired) (yamldriver.State, error) {
	if value.Intent() == selectedvalue.IntentDelete {
		return yamldriver.DeleteState(), nil
	}
	scalar, err := selectedScalar(value)
	if err != nil {
		return yamldriver.State{}, err
	}
	return yamldriver.Driver{}.NormalizeValue(scalar)
}

func tomlStateFromSelected(value selectedvalue.Desired) (tomldriver.State, error) {
	if value.Intent() == selectedvalue.IntentDelete {
		return tomldriver.DeleteState(), nil
	}
	scalar, err := selectedScalar(value)
	if err != nil {
		return tomldriver.State{}, err
	}
	return tomldriver.Driver{}.NormalizeValue(scalar)
}

func plistStateFromSelected(value selectedvalue.Desired) (plistdriver.State, error) {
	if value.Intent() == selectedvalue.IntentDelete {
		return plistdriver.DeleteState(), nil
	}
	scalar, err := selectedScalar(value)
	if err != nil {
		return plistdriver.State{}, err
	}
	return plistdriver.Driver{}.NormalizeValue(scalar)
}

func selectedScalar(value selectedvalue.Desired) (any, error) {
	if value.Intent() != selectedvalue.IntentSet {
		return nil, fmt.Errorf("selected fixture value intent is unsupported")
	}
	raw, ok := value.Value()
	if !ok {
		return nil, fmt.Errorf("selected fixture value is missing")
	}
	switch value.Kind() {
	case desired.KindString:
		typed, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("selected fixture string value has invalid internal representation")
		}
		return typed, nil
	case desired.KindBool:
		typed, ok := raw.(bool)
		if !ok {
			return nil, fmt.Errorf("selected fixture bool value has invalid internal representation")
		}
		return typed, nil
	case desired.KindNumber:
		typed, ok := raw.(json.Number)
		if !ok {
			return nil, fmt.Errorf("selected fixture number value has invalid internal representation")
		}
		return typed, nil
	case desired.KindNull:
		return nil, nil
	default:
		return nil, fmt.Errorf("selected fixture value kind is unsupported")
	}
}

func iniSelector(selector *recipe.Selector) inidriver.Selector {
	if selector == nil {
		return inidriver.Selector{}
	}
	return inidriver.Selector{
		Section:         selector.Section,
		Key:             selector.Key,
		MissingSection:  inidriver.MissingPolicy(selector.MissingSection),
		MissingKey:      inidriver.MissingPolicy(selector.MissingKey),
		DuplicatePolicy: inidriver.DuplicatePolicy(selector.DuplicatePolicy),
		DeleteKey:       inidriver.DeletePolicy(selector.DeleteKey),
	}
}

func jsonSelector(selector *recipe.Selector) jsondriver.Selector {
	if selector == nil {
		return jsondriver.Selector{}
	}
	return jsondriver.Selector{Path: append([]string(nil), selector.Path...), CreateMissing: jsondriver.CreatePolicy(selector.CreateMissing), DeleteKey: jsondriver.DeletePolicy(selector.DeleteKey), DuplicatePolicy: jsondriver.DuplicatePolicy(selector.DuplicatePolicy)}
}

func yamlSelector(selector *recipe.Selector) yamldriver.Selector {
	if selector == nil {
		return yamldriver.Selector{}
	}
	return yamldriver.Selector{Path: append([]string(nil), selector.Path...), CreateMissing: yamldriver.CreatePolicy(selector.CreateMissing), DeleteKey: yamldriver.DeletePolicy(selector.DeleteKey), DuplicatePolicy: yamldriver.DuplicatePolicy(selector.DuplicatePolicy)}
}

func tomlSelector(selector *recipe.Selector) tomldriver.Selector {
	if selector == nil {
		return tomldriver.Selector{}
	}
	return tomldriver.Selector{Path: append([]string(nil), selector.Path...), CreateMissing: tomldriver.CreatePolicy(selector.CreateMissing), DeleteKey: tomldriver.DeletePolicy(selector.DeleteKey), DuplicatePolicy: tomldriver.DuplicatePolicy(selector.DuplicatePolicy)}
}

func plistSelector(selector *recipe.Selector) plistdriver.Selector {
	if selector == nil {
		return plistdriver.Selector{}
	}
	return plistdriver.Selector{Path: append([]string(nil), selector.Path...), CreateMissing: plistdriver.CreatePolicy(selector.CreateMissing), DeleteKey: plistdriver.DeletePolicy(selector.DeleteKey), DuplicatePolicy: plistdriver.DuplicatePolicy(selector.DuplicatePolicy)}
}

func ensureNoSymlinkPath(root string, target string) error {
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		return fmt.Errorf("fixture path escapes scratch root")
	}
	current := root
	parts := strings.Split(rel, string(filepath.Separator))
	for idx, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("fixture path contains a symlink")
			}
			if idx < len(parts)-1 && !info.IsDir() {
				return fmt.Errorf("fixture path parent is not a directory")
			}
			continue
		}
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return nil
}

func liveFileTarget(resource recipe.Resource, scratch roundtripScratch) (filedriver.Target, error) {
	resourceRelPath, err := recipe.ValidateResourcePath(resource.Path)
	if err != nil {
		return filedriver.Target{}, typedError(CodeRecipeInvalid, fmt.Sprintf("resource path is invalid: %v", err), 2, nil)
	}
	return filedriver.Target{LocationID: resource.Location, Root: filepath.Join(scratch.LiveRoot, "locations", resource.Location), RelPath: resourceRelPath, AllowMissingRoot: true, RejectRootSymlink: true, RejectLeafSymlink: true}, nil
}

func fixtureLocationRoots(rec *recipe.Recipe, scratch roundtripScratch) map[string]string {
	roots := map[string]string{}
	for locationID := range rec.Locations {
		roots[locationID] = filepath.Join(scratch.LiveRoot, "locations", locationID)
	}
	return roots
}

func desiredFileTarget(targetID string, settingID string, scope string, plan roundtripFixturePlan, scratchRoot string) (filedriver.Target, error) {
	targetRelDir := desiredTargetRelDir(scope, targetID, plan)
	artifactRel, err := filedriver.ValidateRelativePath(settingID)
	if err != nil {
		return filedriver.Target{}, typedError(CodeRecipeInvalid, fmt.Sprintf("setting artifact path is invalid: %v", err), 2, nil)
	}
	return filedriver.Target{LocationID: "desired", Root: filepath.Join(scratchRoot, "desired", filepath.FromSlash(targetRelDir), "artifacts"), RelPath: artifactRel, AllowMissingRoot: true, RejectRootSymlink: true, RejectLeafSymlink: true}, nil
}

func desiredSettingsURI(targetID string, settingID string, scope string, plan roundtripFixturePlan) string {
	return "desired://" + desiredTargetRelDir(scope, targetID, plan) + "/settings#" + settingID
}

func desiredTargetRelDir(scope string, targetID string, plan roundtripFixturePlan) string {
	switch scope {
	case "shared":
		return path.Join("shared", "-", "targets", targetID)
	case "machine":
		return path.Join("machine", plan.SubjectMachine, "targets", targetID)
	case "machine-user":
		return path.Join("machine-user", plan.SubjectMachine, plan.SubjectUser, "targets", targetID)
	default:
		return path.Join("user", plan.SubjectUser, "targets", targetID)
	}
}

func settingScope(rec *recipe.Recipe, settingID string) string {
	setting := rec.Settings[settingID]
	if setting.ScopeDefault != "" {
		return setting.ScopeDefault
	}
	return "user"
}

func copyFixtureSubtree(src string, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return typedError(CodeFixtureInvalid, fmt.Sprintf("fixture directory %s is required", filepath.ToSlash(src)), 2, nil)
		}
		return typedError(CodeFixtureUnsafe, "cannot inspect fixture directory", 5, nil)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return typedError(CodeFixtureUnsafe, "fixture directory must not be a symlink", 5, nil)
	}
	if !info.IsDir() {
		return typedError(CodeFixtureInvalid, "fixture path must be a directory", 2, nil)
	}
	return filepath.WalkDir(src, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return typedError(CodeFixtureUnsafe, "cannot copy fixture tree", 5, nil)
		}
		info, err := entry.Info()
		if err != nil {
			return typedError(CodeFixtureUnsafe, "cannot inspect fixture tree", 5, nil)
		}
		rel, err := filepath.Rel(src, current)
		if err != nil {
			return typedError(CodeFixtureUnsafe, "cannot resolve fixture tree path", 5, nil)
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		out := filepath.Join(dst, rel)
		if info.Mode()&os.ModeSymlink != 0 {
			return typedError(CodeFixtureUnsafe, "fixture tree contains a symlink", 5, nil)
		}
		if info.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		if !info.Mode().IsRegular() {
			return typedError(CodeFixtureUnsafe, "fixture tree contains a non-regular file", 5, nil)
		}
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		in, err := os.Open(current)
		if err != nil {
			return err
		}
		defer func() { _ = in.Close() }()
		created, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(created, in)
		closeErr := created.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}

func compareExpectedTree(actual string, expected string, expectedLabel string) error {
	actualEntries, err := collectTree(actual)
	if err != nil {
		return err
	}
	expectedEntries, err := collectTree(expected)
	if err != nil {
		return err
	}
	if len(actualEntries) != len(expectedEntries) {
		return typedError(CodeRoundtripMismatch, fmt.Sprintf("%s entry count mismatch", expectedLabel), 2, map[string]any{"path": expectedLabel})
	}
	for idx := range actualEntries {
		actualEntry := actualEntries[idx]
		expectedEntry := expectedEntries[idx]
		if actualEntry.Path != expectedEntry.Path || actualEntry.Kind != expectedEntry.Kind || !bytes.Equal(actualEntry.Bytes, expectedEntry.Bytes) {
			return typedError(CodeRoundtripMismatch, fmt.Sprintf("%s differs at %s", expectedLabel, firstNonEmpty(actualEntry.Path, expectedEntry.Path)), 2, map[string]any{"path": path.Join(expectedLabel, firstNonEmpty(actualEntry.Path, expectedEntry.Path))})
		}
	}
	return nil
}

type treeEntry struct {
	Path  string
	Kind  string
	Bytes []byte
}

func collectTree(root string) ([]treeEntry, error) {
	if info, err := os.Lstat(root); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, typedError(CodeRoundtripMismatch, "expected fixture tree is missing", 2, nil)
		}
		return nil, typedError(CodeFixtureUnsafe, "cannot inspect fixture tree for comparison", 5, nil)
	} else if info.Mode()&os.ModeSymlink != 0 {
		return nil, typedError(CodeFixtureUnsafe, "fixture comparison tree is a symlink", 5, nil)
	} else if !info.IsDir() {
		return nil, typedError(CodeFixtureInvalid, "fixture comparison tree must be a directory", 2, nil)
	}
	entries := []treeEntry{}
	err := filepath.WalkDir(root, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return typedError(CodeFixtureUnsafe, "cannot compare fixture tree", 5, nil)
		}
		info, err := entry.Info()
		if err != nil {
			return typedError(CodeFixtureUnsafe, "cannot inspect fixture comparison entry", 5, nil)
		}
		rel, err := filepath.Rel(root, current)
		if err != nil {
			return typedError(CodeFixtureUnsafe, "cannot resolve fixture comparison entry", 5, nil)
		}
		if rel == "." {
			return nil
		}
		slashed := filepath.ToSlash(rel)
		if info.Mode()&os.ModeSymlink != 0 {
			return typedError(CodeFixtureUnsafe, "fixture comparison tree contains a symlink", 5, map[string]any{"path": slashed})
		}
		if info.IsDir() {
			entries = append(entries, treeEntry{Path: slashed, Kind: "dir"})
			return nil
		}
		if !info.Mode().IsRegular() {
			return typedError(CodeFixtureUnsafe, "fixture comparison tree contains a non-regular file", 5, map[string]any{"path": slashed})
		}
		data, err := os.ReadFile(current)
		if err != nil {
			return typedError(CodeFixtureUnsafe, "cannot read fixture comparison entry", 5, map[string]any{"path": slashed})
		}
		entries = append(entries, treeEntry{Path: slashed, Kind: "file", Bytes: data})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

func rejectDuplicateYAMLKeys(node *yaml.Node) error {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		return rejectDuplicateYAMLKeys(node.Content[0])
	}
	if node.Kind == yaml.MappingNode {
		seen := map[string]bool{}
		for idx := 0; idx+1 < len(node.Content); idx += 2 {
			key := node.Content[idx]
			if seen[key.Value] {
				return fmt.Errorf("duplicate YAML key %s", key.Value)
			}
			seen[key.Value] = true
			if err := rejectDuplicateYAMLKeys(node.Content[idx+1]); err != nil {
				return err
			}
		}
	}
	for _, child := range node.Content {
		if err := rejectDuplicateYAMLKeys(child); err != nil {
			return err
		}
	}
	return nil
}

func finishFixture(fixture *RoundtripFixture) {
	var passed, skipped, blocked, failed int
	for _, item := range fixture.Cases {
		for _, status := range []string{item.Save, item.Apply} {
			switch status {
			case "passed":
				passed++
			case "skipped":
				skipped++
			case "blocked":
				blocked++
			case "failed":
				failed++
			}
		}
	}
	switch {
	case passed == 0 && skipped == 0 && blocked == 0 && failed == 0:
		fixture.Status = "blocked"
		fixture.Reason = fixtureReasonNoRunnableCases
	case failed > 0:
		fixture.Status = "failed"
		fixture.Reason = fixtureReasonRoundtripMismatch
	case blocked > 0 && passed == 0:
		fixture.Status = "blocked"
		fixture.Reason = fixtureReasonSafetyBlocked
	case skipped > 0 && passed == 0:
		fixture.Status = "blocked"
		fixture.Reason = fixtureReasonNoRunnableCases
	case skipped > 0 || blocked > 0:
		fixture.Status = "skipped"
		fixture.Reason = fixtureReasonUnsupportedDriver
	default:
		fixture.Status = "passed"
		fixture.Reason = fixtureReasonOK
	}
}

func finishTestRoundtrip(report *TestRoundtripReport) {
	for _, fixture := range report.AppTestRoundtrip.Fixtures {
		for _, item := range fixture.Cases {
			for _, status := range []string{item.Save, item.Apply} {
				switch status {
				case "passed":
					report.Summary.Cases++
					report.Summary.Passed++
				case "skipped":
					report.Summary.Cases++
					report.Summary.Skipped++
				case "blocked":
					report.Summary.Cases++
					report.Summary.Blocked++
				case "failed":
					report.Summary.Cases++
					report.Summary.Failed++
				}
			}
		}
		if len(fixture.Cases) == 0 && fixture.Status == "blocked" {
			report.Summary.Blocked++
		}
	}
	switch {
	case report.Summary.Failed == 0 && report.Summary.Blocked == 0 && report.Summary.Skipped == 0 && report.Summary.Passed > 0:
		report.Summary.Status = "ok"
	case report.Summary.Passed > 0:
		report.Summary.Status = "partial"
	case report.Summary.Blocked > 0 || report.Summary.Failed > 0 || report.Summary.Skipped > 0:
		report.Summary.Status = "blocked"
	default:
		report.Summary.Status = "blocked"
	}
}

func baseTestRoundtripReport() *TestRoundtripReport {
	return &TestRoundtripReport{Schema: TestRoundtripSchema, SchemaVersion: 1, Command: TestRoundtripCommand, RunID: TestRoundtripRunID, Summary: TestRoundtripSummary{Status: "ok"}, AppTestRoundtrip: TestRoundtripResult{Fixtures: []RoundtripFixture{}}, Diagnostics: []Diagnostic{}}
}

func failTestRoundtrip(report *TestRoundtripReport, code string, message string, exit int, details map[string]any) (*TestRoundtripReport, error) {
	if report == nil {
		report = baseTestRoundtripReport()
	}
	report.Diagnostics = append(report.Diagnostics, Diagnostic{Code: code, Severity: SeverityError, Message: message})
	return failTestRoundtripWithExistingDiagnostics(report, code, message, exit, details)
}

func failTestRoundtripWithExistingDiagnostics(report *TestRoundtripReport, code string, message string, exit int, details map[string]any) (*TestRoundtripReport, error) {
	if report == nil {
		report = baseTestRoundtripReport()
	}
	if report.Summary.Status == "ok" {
		report.Summary.Status = "blocked"
	}
	if report.Summary.Blocked == 0 && report.Summary.Failed == 0 && report.Summary.Skipped == 0 {
		report.Summary.Blocked = countBlockingDiagnostics(report.Diagnostics)
		if report.Summary.Blocked == 0 {
			report.Summary.Blocked = 1
		}
	}
	report.Error = &ErrorObject{Code: code, Message: message, Details: details}
	return report, &Error{Code: code, Message: message, Exit: exit, Details: details}
}

func containsDiagnosticCode(diagnostics []Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}

func hasMode(modes []string, mode string) bool {
	for _, item := range modes {
		if item == mode {
			return true
		}
	}
	return false
}

func relFromRoot(root string, current string, relRoot string) string {
	rel, err := filepath.Rel(root, current)
	if err != nil || rel == "." {
		return relRoot
	}
	return path.Join(relRoot, filepath.ToSlash(rel))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return "."
}

func errorPath(err error) string {
	var appErr *Error
	if errors.As(err, &appErr) && appErr.Details != nil {
		if value, ok := appErr.Details["path"].(string); ok {
			return value
		}
		if value, ok := appErr.Details["setting"].(string); ok {
			return "$.settings." + value
		}
	}
	return ""
}
