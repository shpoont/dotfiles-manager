package initcmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	osuser "os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/shpoont/dotfiles-manager/internal/v2/ledger"
	"github.com/shpoont/dotfiles-manager/internal/v2/resolution"
	"gopkg.in/yaml.v3"
)

const (
	Schema  = "dotfiles-manager.v2.init"
	Command = "init"
	RunID   = "init"
)

const (
	CodeRepoInvalid      = "init.repo.invalid"
	CodeRepoPartial      = "init.repo.partial"
	CodeStateRootInvalid = "init.state-root.invalid"
	CodeIdentityRequired = "init.identity.required"
	CodeIdentityInvalid  = "init.identity.invalid"
	CodeIdentityConflict = "init.identity.conflict"
	CodeWriteFailed      = "init.write-failed"
)

const (
	SeverityInfo    = "info"
	SeverityWarning = "warning"
	SeverityError   = "error"
)

var identityIDRegexp = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

type Options struct {
	RepoRoot         string
	StateRoot        string
	MachineID        string
	UserID           string
	DryRun           bool
	Yes              bool
	NonInteractive   bool
	JSONMode         bool
	Input            io.Reader
	PromptOutput     io.Writer
	Hostname         string
	LocalAccountName string
}

type Report struct {
	Schema        string       `json:"schema"`
	SchemaVersion int          `json:"schemaVersion"`
	Command       string       `json:"command"`
	RunID         string       `json:"runId"`
	DryRun        bool         `json:"dryRun"`
	Summary       Summary      `json:"summary"`
	Init          InitResult   `json:"init"`
	Diagnostics   []Diagnostic `json:"diagnostics"`
	Error         *ErrorObject `json:"error"`
}

type Summary struct {
	Status    string `json:"status"`
	Planned   int    `json:"planned"`
	Written   int    `json:"written"`
	Unchanged int    `json:"unchanged"`
	Blocked   int    `json:"blocked"`
	Failed    int    `json:"failed"`
}

type InitResult struct {
	ActiveProfileStack string          `json:"activeProfileStack"`
	ProfileStack       []string        `json:"profileStack"`
	RepoFiles          []InitFile      `json:"repoFiles"`
	IdentityFiles      []IdentityFile  `json:"identityFiles"`
	MissingChoices     []MissingChoice `json:"missingChoices"`
}

type InitFile struct {
	Kind   string `json:"kind"`
	Path   string `json:"path"`
	Action string `json:"action"`
}

type IdentityFile struct {
	Kind            string `json:"kind"`
	Path            string `json:"path"`
	ID              string `json:"id,omitempty"`
	LocalAccountKey string `json:"localAccountKey,omitempty"`
	Source          string `json:"source"`
	Action          string `json:"action"`
}

type MissingChoice struct {
	Kind        string   `json:"kind"`
	Message     string   `json:"message"`
	Recommended []string `json:"recommended,omitempty"`
}

type Diagnostic struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Path     string `json:"path,omitempty"`
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

func Run(opts Options) (*Report, error) {
	report := baseReport(opts.DryRun)
	if opts.Input == nil {
		opts.Input = os.Stdin
	}
	if _, ok := opts.Input.(*bufio.Reader); !ok {
		opts.Input = bufio.NewReader(opts.Input)
	}
	if opts.PromptOutput == nil {
		opts.PromptOutput = io.Discard
	}

	repoRoot, err := normalizeRepoRoot(opts.RepoRoot)
	if err != nil {
		return fail(report, CodeRepoInvalid, err.Error(), 2, nil)
	}
	stateRoot := strings.TrimSpace(opts.StateRoot)
	if stateRoot == "" {
		stateRoot, err = ledger.DefaultStateRoot(repoRoot)
		if err != nil {
			return fail(report, CodeStateRootInvalid, err.Error(), 2, nil)
		}
	}
	if err := ledger.ValidateStateRoot(repoRoot, stateRoot); err != nil {
		return fail(report, CodeStateRootInvalid, err.Error(), 2, nil)
	}

	repoFiles, err := planRepoScaffold(repoRoot)
	if err != nil {
		return fail(report, codeForRepoPlanError(err), err.Error(), 2, nil)
	}
	report.Init.ActiveProfileStack = "default"
	report.Init.ProfileStack = []string{"global"}
	report.Init.RepoFiles = repoFiles

	identityFiles, missing, err := planIdentities(repoRoot, stateRoot, opts)
	if len(missing) > 0 {
		report.Init.MissingChoices = missing
	}
	if err != nil {
		return fail(report, errorCode(err), err.Error(), errorExit(err, 4), map[string]any{"missingChoices": missing})
	}
	report.Init.IdentityFiles = identityFiles

	for _, file := range report.Init.RepoFiles {
		countAction(report, file.Action)
	}
	for _, file := range report.Init.IdentityFiles {
		countAction(report, file.Action)
	}

	if !opts.DryRun {
		if err := writePlannedRepoFiles(repoRoot, report.Init.RepoFiles); err != nil {
			return fail(report, CodeWriteFailed, err.Error(), 2, nil)
		}
		if err := writePlannedIdentityFiles(stateRoot, report.Init.IdentityFiles); err != nil {
			return fail(report, CodeWriteFailed, err.Error(), 2, nil)
		}
		report.Summary.Written = report.Summary.Planned
	}
	finish(report)
	return report, nil
}

func JSON(report *Report) (string, error) {
	if report == nil {
		report = baseReport(false)
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
		return "Initialize dotfiles-manager v2 workspace.\n\nCommand result:\n  The command could not complete.\n"
	}
	if report.Error != nil {
		return friendlyInitErrorText(report)
	}
	lines := []string{}
	if report.DryRun {
		lines = append(lines, "Preview: initialize dotfiles-manager v2 workspace.")
	} else if report.Summary.Written > 0 || report.Summary.Planned > 0 {
		lines = append(lines, "Initialized dotfiles-manager v2 workspace.")
	} else {
		lines = append(lines, "dotfiles-manager v2 workspace is already initialized.")
	}
	lines = append(lines, "")
	if len(report.Init.RepoFiles) > 0 {
		lines = append(lines, "Repo files:")
		for _, file := range report.Init.RepoFiles {
			lines = append(lines, "  "+friendlyInitAction(report, file.Action)+" "+file.Path)
		}
		lines = append(lines, "")
	}
	if len(report.Init.IdentityFiles) > 0 {
		lines = append(lines, "Local identity:")
		for _, file := range report.Init.IdentityFiles {
			label := titleWord(file.Kind)
			if file.ID != "" {
				lines = append(lines, fmt.Sprintf("  %s: %s", label, file.ID))
			} else {
				lines = append(lines, fmt.Sprintf("  %s: %s", label, friendlyInitAction(report, file.Action)))
			}
		}
		lines = append(lines, "", "These local identity files are used to keep this machine/user separate from shared repo state.", "")
	}
	if len(report.Init.MissingChoices) > 0 {
		lines = append(lines, "Needs input:")
		for _, choice := range report.Init.MissingChoices {
			lines = append(lines, "  "+choice.Message)
		}
		lines = append(lines, "")
	}
	for _, diagnostic := range report.Diagnostics {
		if diagnostic.Severity == SeverityError {
			lines = append(lines, "Problem:", "  "+diagnostic.Message, "")
		}
	}
	lines = append(lines, fmt.Sprintf("Summary: %d repo files %s, %d local identity files %s.",
		len(report.Init.RepoFiles),
		friendlyInitRepoPluralAction(report, report.Init.RepoFiles),
		len(report.Init.IdentityFiles),
		friendlyInitIdentityPluralAction(report, report.Init.IdentityFiles),
	))
	if report.DryRun {
		lines = append(lines, "", "No files changed.")
	}
	lines = append(lines, "", "Next:", "  Discover supported settings:", "  dotfiles-manager --config dotfiles-manager.v2.yaml recipe discover")
	return strings.Join(trimBlank(lines), "\n")
}

func VerboseText(report *Report) string {
	return technicalText(report)
}

func technicalText(report *Report) string {
	if report == nil {
		return "init\nsummary status=error planned=0 written=0 unchanged=0 blocked=0 failed=1"
	}
	lines := []string{"init"}
	if report.DryRun {
		lines = append(lines, "MODE: DRY RUN (no files will be changed)")
	}
	if report.Init.ActiveProfileStack != "" {
		lines = append(lines, "profile stack: "+report.Init.ActiveProfileStack+" ["+strings.Join(report.Init.ProfileStack, " -> ")+"]")
	}
	if len(report.Init.RepoFiles) > 0 {
		lines = append(lines, "repo files:")
		for _, file := range report.Init.RepoFiles {
			lines = append(lines, fmt.Sprintf("  %s action=%s path=%s", file.Kind, file.Action, file.Path))
		}
	}
	if len(report.Init.IdentityFiles) > 0 {
		lines = append(lines, "identity files:")
		for _, file := range report.Init.IdentityFiles {
			line := fmt.Sprintf("  %s action=%s source=%s path=%s", file.Kind, file.Action, file.Source, file.Path)
			if file.ID != "" {
				line += " id=" + file.ID
			}
			if file.LocalAccountKey != "" {
				line += " localAccountKey=" + file.LocalAccountKey
			}
			lines = append(lines, line)
		}
	}
	for _, choice := range report.Init.MissingChoices {
		line := fmt.Sprintf("missing choice: %s", choice.Kind)
		if len(choice.Recommended) > 0 {
			line += " recommended=" + strings.Join(choice.Recommended, ",")
		}
		lines = append(lines, line)
	}
	for _, diagnostic := range report.Diagnostics {
		lines = append(lines, fmt.Sprintf("%s[%s]: %s", diagnostic.Severity, diagnostic.Code, diagnostic.Message))
	}
	if report.Error != nil {
		lines = append(lines, fmt.Sprintf("error[%s]: %s", report.Error.Code, report.Error.Message))
	}
	lines = append(lines, fmt.Sprintf("summary status=%s planned=%d written=%d unchanged=%d blocked=%d failed=%d", report.Summary.Status, report.Summary.Planned, report.Summary.Written, report.Summary.Unchanged, report.Summary.Blocked, report.Summary.Failed))
	return strings.Join(lines, "\n")
}

func friendlyInitErrorText(report *Report) string {
	lines := []string{"Initialize dotfiles-manager v2 workspace.", "", "Command result:", "  " + report.Error.Message}
	if len(report.Init.MissingChoices) > 0 {
		lines = append(lines, "", "Needs input:")
		for _, choice := range report.Init.MissingChoices {
			lines = append(lines, "  "+choice.Message)
		}
	}
	lines = append(lines, "", "No files changed.", "", "Run with --verbose for technical details.")
	return strings.Join(lines, "\n")
}

func friendlyInitAction(report *Report, action string) string {
	switch action {
	case "create":
		if report != nil && report.DryRun {
			return "Would create"
		}
		return "Created"
	case "unchanged":
		return "Already exists"
	default:
		if action == "" {
			return "Checked"
		}
		return titleWord(action)
	}
}

func friendlyInitRepoPluralAction(report *Report, files []InitFile) string {
	if len(files) == 0 {
		return "checked"
	}
	created := 0
	unchanged := 0
	for _, file := range files {
		if file.Action == "create" {
			created++
		} else if file.Action == "unchanged" {
			unchanged++
		}
	}
	return friendlyPluralAction(report, len(files), created, unchanged)
}

func friendlyInitIdentityPluralAction(report *Report, files []IdentityFile) string {
	if len(files) == 0 {
		return "checked"
	}
	created := 0
	unchanged := 0
	for _, file := range files {
		if file.Action == "create" {
			created++
		} else if file.Action == "unchanged" {
			unchanged++
		}
	}
	return friendlyPluralAction(report, len(files), created, unchanged)
}

func friendlyPluralAction(report *Report, total int, created int, unchanged int) string {
	if created > 0 {
		if report != nil && report.DryRun {
			return "would be created"
		}
		return "created"
	}
	if unchanged == total {
		return "already existed"
	}
	return "checked"
}

func titleWord(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func trimBlank(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func baseReport(dryRun bool) *Report {
	return &Report{
		Schema:        Schema,
		SchemaVersion: 1,
		Command:       Command,
		RunID:         RunID,
		DryRun:        dryRun,
		Summary:       Summary{Status: "ok"},
		Init:          InitResult{ProfileStack: []string{}, RepoFiles: []InitFile{}, IdentityFiles: []IdentityFile{}, MissingChoices: []MissingChoice{}},
		Diagnostics:   []Diagnostic{},
	}
}

func normalizeRepoRoot(root string) (string, error) {
	trimmed := strings.TrimSpace(root)
	if trimmed == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve cwd: %w", err)
		}
		trimmed = cwd
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", fmt.Errorf("resolve repo root %q: %w", root, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("stat repo root %q: %w", abs, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("repo root is not a directory: %s", abs)
	}
	return abs, nil
}

func planRepoScaffold(repoRoot string) ([]InitFile, error) {
	files := []InitFile{
		{Kind: "root-config", Path: resolution.RootConfigFile},
		{Kind: "profile-stack", Path: "profiles/stacks/default.yaml"},
		{Kind: "profile-layer", Path: "profiles/layers/global.yaml"},
	}
	exists := 0
	for i := range files {
		path := filepath.Join(repoRoot, filepath.FromSlash(files[i].Path))
		present, err := regularFileExists(path)
		if err != nil {
			return nil, err
		}
		if present {
			exists++
			files[i].Action = "unchanged"
		} else {
			files[i].Action = "create"
		}
	}
	if exists != 0 && exists != len(files) {
		return nil, &Error{Code: CodeRepoPartial, Message: "partial v2 repository scaffold exists; init does not repair missing scaffold files", Exit: 2}
	}
	if exists == len(files) {
		if err := validateExistingSchema(filepath.Join(repoRoot, resolution.RootConfigFile), "root config", "dotfiles-manager.v2.root-config"); err != nil {
			return nil, err
		}
		if err := validateExistingSchema(filepath.Join(repoRoot, "profiles", "stacks", "default.yaml"), "profile stack", "dotfiles-manager.v2.profile-stack"); err != nil {
			return nil, err
		}
		if err := validateExistingSchema(filepath.Join(repoRoot, "profiles", "layers", "global.yaml"), "profile layer", "dotfiles-manager.v2.profile-layer"); err != nil {
			return nil, err
		}
	}
	return files, nil
}

func regularFileExists(path string) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false, fmt.Errorf("symlinked path rejected: %s", path)
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("not a regular file: %s", path)
	}
	return true, nil
}

func validateExistingSchema(path string, kind string, expected string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	root := documentMapping(&doc)
	if root == nil || root.Kind != yaml.MappingNode {
		return fmt.Errorf("%s must be a YAML mapping", kind)
	}
	schema := scalarValue(root, "schema")
	version := scalarValue(root, "schemaVersion")
	if schema != expected {
		return fmt.Errorf("invalid %s schema: %q (expected %q)", kind, schema, expected)
	}
	if version != "1" {
		return fmt.Errorf("invalid %s schemaVersion: %s (expected 1)", kind, version)
	}
	return nil
}

func documentMapping(doc *yaml.Node) *yaml.Node {
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		return doc.Content[0]
	}
	return doc
}

func scalarValue(mapping *yaml.Node, key string) string {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return ""
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Kind == yaml.ScalarNode && mapping.Content[i].Value == key && mapping.Content[i+1].Kind == yaml.ScalarNode {
			return strings.TrimSpace(mapping.Content[i+1].Value)
		}
	}
	return ""
}

func planIdentities(repoRoot string, stateRoot string, opts Options) ([]IdentityFile, []MissingChoice, error) {
	localAccountKey := localAccountKey(opts.LocalAccountName)
	machinePath := filepath.Join(stateRoot, "identity", "machine.yaml")
	userPath := filepath.Join(stateRoot, "identity", "users", localAccountKey+".yaml")
	machineCandidate := disambiguateSubjectID(repoRoot, "machine", safeIDCandidate(firstNonEmpty(opts.Hostname, hostname()), "machine-local"))
	userCandidate := disambiguateSubjectID(repoRoot, "user", safeIDCandidate(localAccountKey, "user-local"))

	interactive := !opts.NonInteractive && !opts.Yes && !opts.JSONMode
	machineID, machineSource, machineAction, missing, err := planMachineIdentity(opts, machinePath, machineCandidate, interactive)
	if err != nil || len(missing) > 0 {
		return nil, missing, err
	}
	userID, userSource, userAction, missing, err := planUserIdentity(opts, userPath, localAccountKey, userCandidate, interactive)
	if err != nil || len(missing) > 0 {
		return nil, missing, err
	}
	files := []IdentityFile{
		{Kind: "machine", Path: "state://identity/machine.yaml", ID: machineID, Source: machineSource, Action: machineAction},
		{Kind: "user", Path: "state://identity/users/" + localAccountKey + ".yaml", ID: userID, LocalAccountKey: localAccountKey, Source: userSource, Action: userAction},
	}
	return files, nil, nil
}

func planMachineIdentity(opts Options, path string, candidate string, interactive bool) (string, string, string, []MissingChoice, error) {
	existing, present, err := readMachineIdentity(path)
	if err != nil {
		return "", "", "", nil, err
	}
	explicit := strings.TrimSpace(opts.MachineID)
	if explicit != "" {
		if err := validateIdentityID("machine", explicit); err != nil {
			return "", "", "", nil, err
		}
	}
	if present {
		if explicit != "" && explicit != existing.MachineID {
			return "", "", "", nil, &Error{Code: CodeIdentityConflict, Message: "existing machine identity conflicts with --machine-id", Exit: 5, Details: map[string]any{"kind": "machine"}}
		}
		return existing.MachineID, "existing", "unchanged", nil, nil
	}
	if explicit != "" {
		return explicit, "explicit", "create", nil, nil
	}
	if opts.Yes {
		return candidate, "generated", "create", nil, nil
	}
	missing := []MissingChoice{{Kind: "machine-id", Message: "choose a machine id visible in desired/machine/<machine-id>/...", Recommended: []string{candidate}}}
	if !interactive {
		return "", "", "", missing, &Error{Code: CodeIdentityRequired, Message: "machine identity required", Exit: 4}
	}
	chosen, err := promptIdentity(opts, "Machine ID", "desired/machine/"+candidate+"/...", candidate)
	if err != nil {
		return "", "", "", missing, err
	}
	if err := validateIdentityID("machine", chosen); err != nil {
		return "", "", "", nil, err
	}
	return chosen, "prompted", "create", nil, nil
}

func planUserIdentity(opts Options, path string, localAccountKey string, candidate string, interactive bool) (string, string, string, []MissingChoice, error) {
	existing, present, err := readUserIdentity(path)
	if err != nil {
		return "", "", "", nil, err
	}
	explicit := strings.TrimSpace(opts.UserID)
	if explicit != "" {
		if err := validateIdentityID("user", explicit); err != nil {
			return "", "", "", nil, err
		}
	}
	if present {
		if existing.LocalAccountKey != localAccountKey {
			return "", "", "", nil, &Error{Code: CodeIdentityInvalid, Message: "existing user identity localAccountKey does not match current local account key", Exit: 2}
		}
		if explicit != "" && explicit != existing.UserID {
			return "", "", "", nil, &Error{Code: CodeIdentityConflict, Message: "existing user identity conflicts with --user-id", Exit: 5, Details: map[string]any{"kind": "user"}}
		}
		return existing.UserID, "existing", "unchanged", nil, nil
	}
	if explicit != "" {
		return explicit, "explicit", "create", nil, nil
	}
	if opts.Yes {
		return candidate, "generated", "create", nil, nil
	}
	missing := []MissingChoice{{Kind: "user-id", Message: "choose a user id visible in desired/user/<user-id>/...", Recommended: []string{candidate}}}
	if !interactive {
		return "", "", "", missing, &Error{Code: CodeIdentityRequired, Message: "user identity required", Exit: 4}
	}
	chosen, err := promptIdentity(opts, "User ID", "desired/user/"+candidate+"/...", candidate)
	if err != nil {
		return "", "", "", missing, err
	}
	if err := validateIdentityID("user", chosen); err != nil {
		return "", "", "", nil, err
	}
	return chosen, "prompted", "create", nil, nil
}

func promptIdentity(opts Options, label string, visiblePath string, candidate string) (string, error) {
	_, _ = fmt.Fprintf(opts.PromptOutput, "%s is visible in repository paths such as %s. Accept [%s]: ", label, visiblePath, candidate)
	line, err := readLine(opts.Input)
	if err != nil {
		return "", &Error{Code: CodeIdentityRequired, Message: err.Error(), Exit: 4}
	}
	line = strings.TrimSpace(line)
	if line == "" {
		line = candidate
	}
	return line, nil
}

func readLine(r io.Reader) (string, error) {
	reader, ok := r.(*bufio.Reader)
	if !ok {
		reader = bufio.NewReader(r)
	}
	line, err := reader.ReadString('\n')
	if err != nil && len(line) == 0 {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

type machineIdentityFile struct {
	Schema        string `yaml:"schema"`
	SchemaVersion int    `yaml:"schemaVersion"`
	MachineID     string `yaml:"machineId"`
}

type userIdentityFile struct {
	Schema          string `yaml:"schema"`
	SchemaVersion   int    `yaml:"schemaVersion"`
	LocalAccountKey string `yaml:"localAccountKey"`
	UserID          string `yaml:"userId"`
}

func readMachineIdentity(path string) (machineIdentityFile, bool, error) {
	var identity machineIdentityFile
	present, err := regularFileExists(path)
	if err != nil {
		return identity, false, err
	}
	if !present {
		return identity, false, nil
	}
	if err := decodeKnownYAML(path, &identity); err != nil {
		return identity, false, &Error{Code: CodeIdentityInvalid, Message: err.Error(), Exit: 2}
	}
	if identity.Schema != "dotfiles-manager.v2.machine-identity" || identity.SchemaVersion != 1 {
		return identity, false, &Error{Code: CodeIdentityInvalid, Message: "invalid machine identity schema", Exit: 2}
	}
	if err := validateIdentityID("machine", identity.MachineID); err != nil {
		return identity, false, err
	}
	return identity, true, nil
}

func readUserIdentity(path string) (userIdentityFile, bool, error) {
	var identity userIdentityFile
	present, err := regularFileExists(path)
	if err != nil {
		return identity, false, err
	}
	if !present {
		return identity, false, nil
	}
	if err := decodeKnownYAML(path, &identity); err != nil {
		return identity, false, &Error{Code: CodeIdentityInvalid, Message: err.Error(), Exit: 2}
	}
	if identity.Schema != "dotfiles-manager.v2.user-identity" || identity.SchemaVersion != 1 {
		return identity, false, &Error{Code: CodeIdentityInvalid, Message: "invalid user identity schema", Exit: 2}
	}
	if err := validateIdentityID("local account key", identity.LocalAccountKey); err != nil {
		return identity, false, err
	}
	if err := validateIdentityID("user", identity.UserID); err != nil {
		return identity, false, err
	}
	return identity, true, nil
}

func decodeKnownYAML(path string, out any) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()
	dec := yaml.NewDecoder(file)
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

func writePlannedRepoFiles(repoRoot string, files []InitFile) error {
	for _, file := range files {
		if file.Action != "create" {
			continue
		}
		path := filepath.Join(repoRoot, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(repoFileBody(file.Kind)), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func repoFileBody(kind string) string {
	switch kind {
	case "root-config":
		return "schema: dotfiles-manager.v2.root-config\nschemaVersion: 1\nactiveProfileStack: default\n"
	case "profile-stack":
		return "schema: dotfiles-manager.v2.profile-stack\nschemaVersion: 1\nprofileStack:\n  - global\n"
	case "profile-layer":
		return "schema: dotfiles-manager.v2.profile-layer\nschemaVersion: 1\nselections: {}\n"
	default:
		return ""
	}
}

func writePlannedIdentityFiles(stateRoot string, files []IdentityFile) error {
	for _, file := range files {
		if file.Action != "create" {
			continue
		}
		path := strings.TrimPrefix(file.Path, "state://")
		abs := filepath.Join(stateRoot, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
			return err
		}
		body := ""
		switch file.Kind {
		case "machine":
			body = fmt.Sprintf("schema: dotfiles-manager.v2.machine-identity\nschemaVersion: 1\nmachineId: %s\n", file.ID)
		case "user":
			body = fmt.Sprintf("schema: dotfiles-manager.v2.user-identity\nschemaVersion: 1\nlocalAccountKey: %s\nuserId: %s\n", file.LocalAccountKey, file.ID)
		}
		if err := os.WriteFile(abs, []byte(body), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func countAction(report *Report, action string) {
	switch action {
	case "create":
		report.Summary.Planned++
	case "unchanged":
		report.Summary.Unchanged++
	}
}

func finish(report *Report) {
	sort.Slice(report.Diagnostics, func(i, j int) bool {
		if report.Diagnostics[i].Code == report.Diagnostics[j].Code {
			return report.Diagnostics[i].Message < report.Diagnostics[j].Message
		}
		return report.Diagnostics[i].Code < report.Diagnostics[j].Code
	})
	switch {
	case report.Summary.Blocked > 0:
		report.Summary.Status = "blocked"
	case report.Summary.Failed > 0:
		report.Summary.Status = "error"
	case report.Summary.Planned > 0 || report.Summary.Written > 0:
		report.Summary.Status = "changed"
	default:
		report.Summary.Status = "ok"
	}
}

func fail(report *Report, code string, message string, exit int, details map[string]any) (*Report, error) {
	if exit == 0 {
		exit = 1
	}
	report.Error = &ErrorObject{Code: code, Message: message, Details: details}
	report.Diagnostics = append(report.Diagnostics, Diagnostic{Code: code, Severity: SeverityError, Message: message})
	if exit == 4 || exit == 5 {
		report.Summary.Status = "blocked"
		report.Summary.Blocked = 1
	} else {
		report.Summary.Status = "error"
		report.Summary.Failed = 1
	}
	return report, &Error{Code: code, Message: message, Exit: exit, Details: details}
}

func codeForRepoPlanError(err error) string {
	if parsed, ok := err.(*Error); ok {
		return parsed.Code
	}
	return CodeRepoInvalid
}

func errorCode(err error) string {
	if parsed, ok := err.(*Error); ok {
		return parsed.Code
	}
	return CodeIdentityRequired
}

func errorExit(err error, fallback int) int {
	if parsed, ok := err.(*Error); ok {
		return parsed.ExitCode()
	}
	return fallback
}

func validateIdentityID(kind string, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed != value || !identityIDRegexp.MatchString(trimmed) {
		return &Error{Code: CodeIdentityInvalid, Message: fmt.Sprintf("invalid %s id: %s", kind, value), Exit: 2}
	}
	return nil
}

func localAccountKey(raw string) string {
	if strings.TrimSpace(raw) == "" {
		if current, err := osuser.Current(); err == nil && strings.TrimSpace(current.Username) != "" {
			raw = filepath.Base(current.Username)
		} else if env := strings.TrimSpace(os.Getenv("USER")); env != "" {
			raw = env
		} else {
			raw = "local-user"
		}
	}
	return safeIDCandidate(raw, "local-user")
}

func hostname() string {
	name, err := os.Hostname()
	if err != nil || strings.TrimSpace(name) == "" {
		return "machine-local"
	}
	return name
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func safeIDCandidate(raw string, fallback string) string {
	lower := strings.ToLower(strings.TrimSpace(raw))
	var b strings.Builder
	lastDash := false
	for _, r := range lower {
		safe := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-'
		if safe {
			b.WriteRune(r)
			lastDash = r == '-'
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	candidate := strings.Trim(b.String(), ".-_")
	if candidate == "" || !identityIDRegexp.MatchString(candidate) {
		candidate = fallback
	}
	if !identityIDRegexp.MatchString(candidate) {
		candidate = "local"
	}
	return candidate
}

func disambiguateSubjectID(repoRoot string, kind string, candidate string) string {
	base := candidate
	for suffix := 0; suffix < 100; suffix++ {
		value := base
		if suffix > 0 {
			value = fmt.Sprintf("%s-%d", base, suffix+1)
		}
		if !subjectDirNameExistsCaseFold(repoRoot, kind, value) {
			return value
		}
	}
	return base + "-generated"
}

func subjectDirNameExistsCaseFold(repoRoot string, kind string, candidate string) bool {
	entries, err := os.ReadDir(filepath.Join(repoRoot, "desired", kind))
	if err != nil {
		return false
	}
	candidateFold := strings.ToLower(candidate)
	for _, entry := range entries {
		if strings.ToLower(entry.Name()) == candidateFold {
			return true
		}
	}
	return false
}
