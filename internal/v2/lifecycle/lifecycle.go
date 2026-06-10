package lifecycle

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
)

const (
	DefaultActionTimeout = 10 * time.Second
	DefaultDetectTimeout = 2 * time.Second
)

const (
	StateRunning    = "running"
	StateNotRunning = "not-running"
	StateUnknown    = "unknown"
	StateAmbiguous  = "ambiguous"

	ModePlanned  = "planned"
	ModeExecuted = "executed"

	PhaseBeforeWrite = "before-write"
	PhaseAfterWrite  = "after-write"

	ActionDetect  = "detect"
	ActionPrompt  = "prompt"
	ActionQuit    = "quit"
	ActionRecheck = "recheck"
	ActionReopen  = "reopen"
	ActionWarn    = "warn"
	ActionBlock   = "block"

	ResultPlanned   = "planned"
	ResultSkipped   = "skipped"
	ResultSucceeded = "succeeded"
	ResultFailed    = "failed"
	ResultBlocked   = "blocked"
	ResultDeclined  = "declined"

	CodePolicyBlocked        = "lifecycle.policy.blocked"
	CodeRunningBlocked       = "lifecycle.running.blocked"
	CodeConfirmationRequired = "lifecycle.confirmation.required"
	CodeUserDeclined         = "lifecycle.user.declined"
	CodeTargetMissing        = "lifecycle.target.missing"
	CodeTargetUnsupported    = "lifecycle.target.unsupported"
	CodeDetectFailed         = "lifecycle.detect.failed"
	CodeDetectAmbiguous      = "lifecycle.detect.ambiguous"
	CodeQuitUnsupported      = "lifecycle.quit.unsupported"
	CodeQuitFailed           = "lifecycle.quit.failed"
	CodeStillRunning         = "lifecycle.recheck.still-running"
	CodeReopenUnsupported    = "lifecycle.reopen.unsupported"
	CodeReopenFailed         = "lifecycle.reopen.failed"
)

type RunningState string

type Detector interface {
	Detect(ctx context.Context, target recipe.LifecycleTarget) (DetectionResult, error)
}

type Controller interface {
	Quit(ctx context.Context, target recipe.LifecycleTarget) error
	Reopen(ctx context.Context, target recipe.LifecycleTarget) error
}

type Prompter interface {
	Prompt(ctx context.Context, prompt Prompt) (bool, error)
}

type Prompt struct {
	TargetID    string
	DisplayName string
	Action      string
	Message     string
}

type DetectionResult struct {
	State RunningState
	Count int
}

type Request struct {
	Recipe            *recipe.Recipe
	SettingID         string
	SettingRef        string
	ResourceID        string
	NativeOperationID string
	Command           string
	DryRun            bool
	Confirmed         bool
	NonInteractive    bool
	ActionTimeout     time.Duration
	Detector          Detector
	Controller        Controller
	Prompter          Prompter
}

type EffectivePolicy struct {
	Lifecycle            string `json:"lifecycle"`
	WriteMode            string `json:"writeMode"`
	BeforeWrite          string `json:"beforeWrite"`
	AfterWrite           string `json:"afterWrite"`
	RequiresConfirmation bool   `json:"requiresConfirmation,omitempty"`
	RequiresRunningState bool   `json:"requiresRunningState,omitempty"`
	LifecycleTargetID    string `json:"lifecycleTargetId,omitempty"`
	NativeOperationID    string `json:"nativeOperationId,omitempty"`
}

type ActionRecord struct {
	TargetRef           string `json:"targetRef,omitempty"`
	SettingRef          string `json:"settingRef,omitempty"`
	ResourceID          string `json:"resourceId,omitempty"`
	NativeOperationID   string `json:"nativeOperationId,omitempty"`
	LifecycleTargetID   string `json:"lifecycleTargetId,omitempty"`
	LifecycleTargetName string `json:"lifecycleTargetName,omitempty"`
	Phase               string `json:"phase"`
	Action              string `json:"action"`
	Mode                string `json:"mode"`
	Result              string `json:"result"`
	StateBefore         string `json:"stateBefore,omitempty"`
	StateAfter          string `json:"stateAfter,omitempty"`
	ProcessCount        int    `json:"processCount,omitempty"`
	ManagerStopped      bool   `json:"managerStopped,omitempty"`
	ReopenAttempted     bool   `json:"reopenAttempted,omitempty"`
	Code                string `json:"code,omitempty"`
	Message             string `json:"message,omitempty"`
}

type Decision struct {
	Policy         EffectivePolicy
	Target         *recipe.LifecycleTarget
	Actions        []ActionRecord
	Blocked        bool
	ManagerStopped bool
	DiagnosticCode string
	Message        string
}

func EvaluateBefore(ctx context.Context, req Request) Decision {
	policy := PolicyFor(req.Recipe, req.SettingID, req.ResourceID, req.NativeOperationID)
	decision := Decision{Policy: policy}
	base := baseRecord(req, policy, PhaseBeforeWrite)

	switch policy.Lifecycle {
	case "", recipe.LifecycleAllowed:
		return decision
	case recipe.LifecycleWarn:
		decision.Actions = append(decision.Actions, recordWith(base, ActionWarn, mode(req), ResultSucceeded, "", "lifecycle warning is non-blocking"))
		return decision
	case recipe.LifecycleBlocked:
		decision.block(recordWith(base, ActionBlock, mode(req), ResultBlocked, CodePolicyBlocked, "lifecycle policy blocks live writes"))
		return decision
	}

	target, ok := targetFor(req.Recipe, policy.LifecycleTargetID)
	if !ok {
		decision.block(recordWith(base, ActionBlock, mode(req), ResultBlocked, CodeTargetMissing, "lifecycle policy requires an explicit lifecycle target"))
		return decision
	}
	decision.Target = &target
	base = withTarget(base, policy.LifecycleTargetID, target)
	detector := req.Detector
	if detector == nil {
		detector = ProcessNameDetector{}
	}
	detectCtx, cancel := boundedActionContext(ctx, req)
	detected, detectErr := detector.Detect(detectCtx, target)
	cancel()
	detectRecord := recordWith(base, ActionDetect, ModeExecuted, ResultSucceeded, "", "lifecycle target running state detected")
	detectRecord.StateAfter = string(detected.State)
	detectRecord.ProcessCount = detected.Count
	if detectErr != nil {
		detectRecord.Result = ResultFailed
		detectRecord.Code = CodeDetectFailed
		detectRecord.Message = "lifecycle target running-state detection failed"
		decision.Actions = append(decision.Actions, detectRecord)
		decision.block(recordWith(base, ActionBlock, mode(req), ResultBlocked, CodeDetectFailed, "lifecycle detection failed; no write was attempted"))
		return decision
	}
	decision.Actions = append(decision.Actions, detectRecord)
	if detected.State == RunningState(StateAmbiguous) || detected.State == RunningState(StateUnknown) {
		code := CodeDetectAmbiguous
		if detected.State == RunningState(StateUnknown) {
			code = CodeDetectFailed
		}
		decision.block(recordWith(base, ActionBlock, mode(req), ResultBlocked, code, "lifecycle running state is ambiguous; no write was attempted"))
		return decision
	}
	if detected.State == RunningState(StateNotRunning) {
		return decision
	}

	switch policy.Lifecycle {
	case recipe.LifecycleBlockIfRunning:
		decision.block(recordWith(base, ActionBlock, mode(req), ResultBlocked, CodeRunningBlocked, "lifecycle target is running and policy blocks live writes"))
		return decision
	case recipe.LifecycleAskToQuit:
		if req.DryRun {
			decision.Actions = append(decision.Actions, recordWith(base, ActionPrompt, ModePlanned, ResultPlanned, "", "would ask the user to quit the app manually before writing"))
			return decision
		}
		if req.Confirmed {
			decision.block(recordWith(base, ActionPrompt, ModeExecuted, ResultBlocked, CodeConfirmationRequired, "manual quit cannot be satisfied by --yes; quit the app manually and run without --yes to confirm the recheck"))
			return decision
		}
		if req.NonInteractive || req.Prompter == nil {
			decision.block(recordWith(base, ActionPrompt, ModeExecuted, ResultBlocked, CodeConfirmationRequired, "manual quit confirmation is required before writing"))
			return decision
		}
		accepted, err := req.Prompter.Prompt(ctx, Prompt{TargetID: policy.LifecycleTargetID, DisplayName: displayName(policy.LifecycleTargetID, target), Action: ActionPrompt, Message: "Quit the app manually, then confirm to re-check before writing."})
		if err != nil || !accepted {
			decision.block(recordWith(base, ActionPrompt, ModeExecuted, ResultDeclined, CodeUserDeclined, "user did not confirm manual quit; no write was attempted"))
			return decision
		}
		return recheckAfterQuit(ctx, req, decision, base, target, false)
	case recipe.LifecycleQuitIfRunning, recipe.LifecycleReopenIfStoppedByTool:
		if req.DryRun {
			decision.Actions = append(decision.Actions, recordWith(base, ActionQuit, ModePlanned, ResultPlanned, "", "would request managed quit before writing"))
			if policy.Lifecycle == recipe.LifecycleReopenIfStoppedByTool {
				decision.Actions = append(decision.Actions, recordWith(base, ActionReopen, ModePlanned, ResultPlanned, "", "would reopen after writing because the manager stopped the app"))
			}
			return decision
		}
		if !req.Confirmed {
			decision.block(recordWith(base, ActionQuit, ModeExecuted, ResultBlocked, CodeConfirmationRequired, "managed quit requires explicit confirmation"))
			return decision
		}
		if target.Quit.Kind != recipe.LifecycleControlManaged || req.Controller == nil {
			decision.block(recordWith(base, ActionQuit, ModeExecuted, ResultBlocked, CodeQuitUnsupported, "managed quit is not supported for this lifecycle target"))
			return decision
		}
		quitRecord := recordWith(base, ActionQuit, ModeExecuted, ResultSucceeded, "", "managed quit completed")
		controlCtx, cancel := boundedActionContext(ctx, req)
		err := req.Controller.Quit(controlCtx, target)
		cancel()
		if err != nil {
			quitRecord.Result = ResultFailed
			quitRecord.Code = CodeQuitFailed
			quitRecord.Message = "managed quit failed; no write was attempted"
			decision.Actions = append(decision.Actions, quitRecord)
			decision.block(recordWith(base, ActionBlock, ModeExecuted, ResultBlocked, CodeQuitFailed, "managed quit failed; no write was attempted"))
			return decision
		}
		quitRecord.ManagerStopped = true
		decision.Actions = append(decision.Actions, quitRecord)
		return recheckAfterQuit(ctx, req, decision, base, target, true)
	default:
		decision.block(recordWith(base, ActionBlock, mode(req), ResultBlocked, CodeTargetUnsupported, "unsupported lifecycle policy"))
		return decision
	}
}

func EvaluateAfter(ctx context.Context, req Request, managerStopped bool) Decision {
	policy := PolicyFor(req.Recipe, req.SettingID, req.ResourceID, req.NativeOperationID)
	decision := Decision{Policy: policy, ManagerStopped: managerStopped}
	if policy.Lifecycle != recipe.LifecycleReopenIfStoppedByTool || !managerStopped {
		return decision
	}
	base := baseRecord(req, policy, PhaseAfterWrite)
	target, ok := targetFor(req.Recipe, policy.LifecycleTargetID)
	if !ok {
		decision.block(recordWith(base, ActionReopen, ModeExecuted, ResultBlocked, CodeTargetMissing, "lifecycle reopen target is missing"))
		return decision
	}
	decision.Target = &target
	base = withTarget(base, policy.LifecycleTargetID, target)
	record := recordWith(base, ActionReopen, ModeExecuted, ResultSucceeded, "", "managed reopen completed")
	record.ManagerStopped = true
	record.ReopenAttempted = true
	if target.Reopen.Kind != recipe.LifecycleControlManaged || req.Controller == nil {
		record.Result = ResultBlocked
		record.Code = CodeReopenUnsupported
		record.Message = "managed reopen is not supported for this lifecycle target"
		decision.block(record)
		return decision
	}
	controlCtx, cancel := boundedActionContext(ctx, req)
	err := req.Controller.Reopen(controlCtx, target)
	cancel()
	if err != nil {
		record.Result = ResultFailed
		record.Code = CodeReopenFailed
		record.Message = "managed reopen failed after live write"
		decision.block(record)
		return decision
	}
	decision.Actions = append(decision.Actions, record)
	return decision
}

func PolicyFor(rec *recipe.Recipe, settingID string, resourceID string, nativeOperationID ...string) EffectivePolicy {
	policy := EffectivePolicy{Lifecycle: recipe.LifecycleAllowed, WriteMode: "allowed", BeforeWrite: "none", AfterWrite: "none"}
	if rec == nil {
		return policy
	}
	resource := recipe.Resource{}
	if resourceID != "" {
		resource = rec.Resources[resourceID]
	}
	setting := recipe.Setting{}
	if settingID != "" {
		setting = rec.Settings[settingID]
	}
	lifecycle := resource.Lifecycle
	if setting.Lifecycle != "" {
		lifecycle = setting.Lifecycle
	}
	if lifecycle == "" {
		lifecycle = recipe.LifecycleAllowed
	}
	targetID := setting.LifecycleTarget
	if targetID == "" {
		targetID = resource.LifecycleTarget
	}
	operationID := firstNativeOperationID(nativeOperationID...)
	if operationID != "" {
		if operation, ok := rec.NativeOperations[operationID]; ok && operation.Lifecycle != "" {
			lifecycle = operation.Lifecycle
			targetID = operation.LifecycleTarget
		}
	}
	policy.Lifecycle = lifecycle
	policy.LifecycleTargetID = targetID
	policy.NativeOperationID = operationID
	switch lifecycle {
	case recipe.LifecycleWarn:
		policy.WriteMode = "warn"
	case recipe.LifecycleBlocked:
		policy.WriteMode = "blocked"
	case recipe.LifecycleAskToQuit:
		policy.WriteMode = "blocked-while-running"
		policy.BeforeWrite = "ask-to-quit"
		policy.RequiresRunningState = true
	case recipe.LifecycleQuitIfRunning:
		policy.WriteMode = "blocked-while-running"
		policy.BeforeWrite = "quit-if-running"
		policy.RequiresConfirmation = true
		policy.RequiresRunningState = true
	case recipe.LifecycleBlockIfRunning:
		policy.WriteMode = "blocked-while-running"
		policy.BeforeWrite = "block-if-running"
		policy.RequiresRunningState = true
	case recipe.LifecycleReopenIfStoppedByTool:
		policy.WriteMode = "blocked-while-running"
		policy.BeforeWrite = "quit-if-running"
		policy.AfterWrite = "reopen-if-stopped-by-tool"
		policy.RequiresConfirmation = true
		policy.RequiresRunningState = true
	}
	return policy
}

func recheckAfterQuit(ctx context.Context, req Request, decision Decision, base ActionRecord, target recipe.LifecycleTarget, managerStopped bool) Decision {
	detector := req.Detector
	if detector == nil {
		detector = ProcessNameDetector{}
	}
	detectCtx, cancel := boundedActionContext(ctx, req)
	result, err := detector.Detect(detectCtx, target)
	cancel()
	recheck := recordWith(base, ActionRecheck, ModeExecuted, ResultSucceeded, "", "lifecycle target is no longer running")
	recheck.StateAfter = string(result.State)
	recheck.ProcessCount = result.Count
	recheck.ManagerStopped = managerStopped
	if err != nil {
		recheck.Result = ResultFailed
		recheck.Code = CodeDetectFailed
		recheck.Message = "lifecycle recheck failed; no write was attempted"
		decision.Actions = append(decision.Actions, recheck)
		decision.block(recordWith(base, ActionBlock, ModeExecuted, ResultBlocked, CodeDetectFailed, "lifecycle recheck failed; no write was attempted"))
		return decision
	}
	decision.Actions = append(decision.Actions, recheck)
	if result.State != RunningState(StateNotRunning) {
		decision.block(recordWith(base, ActionBlock, ModeExecuted, ResultBlocked, CodeStillRunning, "lifecycle target is still running; no write was attempted"))
		return decision
	}
	decision.ManagerStopped = managerStopped
	return decision
}

func (d *Decision) block(record ActionRecord) {
	d.Blocked = true
	if record.Code != "" {
		d.DiagnosticCode = record.Code
	}
	if record.Message != "" {
		d.Message = record.Message
	}
	d.Actions = append(d.Actions, record)
}

func baseRecord(req Request, policy EffectivePolicy, phase string) ActionRecord {
	return ActionRecord{TargetRef: targetRef(req.SettingRef), SettingRef: req.SettingRef, ResourceID: req.ResourceID, NativeOperationID: policy.NativeOperationID, LifecycleTargetID: policy.LifecycleTargetID, Phase: phase}
}

func firstNativeOperationID(ids ...string) string {
	for _, id := range ids {
		if strings.TrimSpace(id) != "" {
			return strings.TrimSpace(id)
		}
	}
	return ""
}

func boundedActionContext(ctx context.Context, req Request) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}
	timeout := req.ActionTimeout
	if timeout <= 0 {
		timeout = DefaultActionTimeout
	}
	return context.WithTimeout(ctx, timeout)
}

func withTarget(record ActionRecord, id string, target recipe.LifecycleTarget) ActionRecord {
	record.LifecycleTargetID = id
	record.LifecycleTargetName = displayName(id, target)
	return record
}

func recordWith(base ActionRecord, action string, mode string, result string, code string, message string) ActionRecord {
	record := base
	record.Action = action
	record.Mode = mode
	record.Result = result
	record.Code = code
	record.Message = message
	return record
}

func mode(req Request) string {
	if req.DryRun {
		return ModePlanned
	}
	return ModeExecuted
}

func targetRef(settingRef string) string {
	if idx := strings.Index(settingRef, ":"); idx > 0 {
		return settingRef[:idx]
	}
	return ""
}

func targetFor(rec *recipe.Recipe, id string) (recipe.LifecycleTarget, bool) {
	if rec == nil || strings.TrimSpace(id) == "" {
		return recipe.LifecycleTarget{}, false
	}
	target, ok := rec.LifecycleTargets[id]
	return target, ok
}

func displayName(id string, target recipe.LifecycleTarget) string {
	if strings.TrimSpace(target.DisplayName) != "" {
		return strings.TrimSpace(target.DisplayName)
	}
	return id
}

func RecordsToDiagnostics(actions []ActionRecord) []Diagnostic {
	var diagnostics []Diagnostic
	for _, action := range actions {
		if action.Code == "" {
			continue
		}
		diagnostics = append(diagnostics, Diagnostic{Code: action.Code, Message: action.Message})
	}
	return diagnostics
}

type Diagnostic struct {
	Code    string
	Message string
}

type ProcessNameDetector struct {
	PSPath  string
	Timeout time.Duration
}

func (d ProcessNameDetector) Detect(ctx context.Context, target recipe.LifecycleTarget) (DetectionResult, error) {
	if target.Detect.Kind != recipe.LifecycleDetectProcessName {
		return DetectionResult{State: RunningState(StateUnknown)}, fmt.Errorf("unsupported lifecycle detector %q", target.Detect.Kind)
	}
	wanted := map[string]bool{}
	for _, name := range target.Detect.Names {
		trimmed := strings.TrimSpace(name)
		if trimmed != "" {
			wanted[trimmed] = true
		}
	}
	if len(wanted) == 0 {
		return DetectionResult{State: RunningState(StateUnknown)}, fmt.Errorf("process-name detector has no names")
	}
	psPath, err := d.psPath()
	if err != nil {
		return DetectionResult{State: RunningState(StateUnknown)}, err
	}
	detectCtx := ctx
	cancel := func() {}
	if _, ok := ctx.Deadline(); !ok {
		timeout := d.Timeout
		if timeout <= 0 {
			timeout = DefaultDetectTimeout
		}
		detectCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()
	cmd := exec.CommandContext(detectCtx, psPath, "-axo", "comm=")
	cmd.Env = []string{"LC_ALL=C", "LANG=C"}
	output, err := cmd.Output()
	if err != nil {
		return DetectionResult{State: RunningState(StateUnknown)}, err
	}
	count := 0
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		name := strings.TrimSpace(scanner.Text())
		if name == "" {
			continue
		}
		base := filepath.Base(name)
		if wanted[base] || wanted[name] {
			count++
		}
	}
	if err := scanner.Err(); err != nil {
		return DetectionResult{State: RunningState(StateUnknown)}, err
	}
	if count > 0 {
		return DetectionResult{State: RunningState(StateRunning), Count: count}, nil
	}
	return DetectionResult{State: RunningState(StateNotRunning)}, nil
}

func (d ProcessNameDetector) psPath() (string, error) {
	if strings.TrimSpace(d.PSPath) != "" {
		if !filepath.IsAbs(d.PSPath) {
			return "", fmt.Errorf("ps path must be absolute")
		}
		info, err := os.Stat(d.PSPath)
		if err != nil {
			return "", err
		}
		if info.IsDir() {
			return "", fmt.Errorf("ps path is a directory")
		}
		return d.PSPath, nil
	}
	for _, candidate := range []string{"/bin/ps", "/usr/bin/ps"} {
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no supported absolute ps path found")
}

type UnsupportedController struct{}

func (UnsupportedController) Quit(context.Context, recipe.LifecycleTarget) error {
	return errors.New("managed lifecycle quit is unsupported")
}

func (UnsupportedController) Reopen(context.Context, recipe.LifecycleTarget) error {
	return errors.New("managed lifecycle reopen is unsupported")
}

type TextPrompter struct {
	In  io.Reader
	Out io.Writer
}

func (p TextPrompter) Prompt(ctx context.Context, prompt Prompt) (bool, error) {
	if p.In == nil || p.Out == nil {
		return false, errors.New("lifecycle prompter is not configured")
	}
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}
	label := strings.TrimSpace(prompt.DisplayName)
	if label == "" {
		label = prompt.TargetID
	}
	_, _ = fmt.Fprintf(p.Out, "%s\nConfirm once %s is no longer running [y/N]: ", prompt.Message, label)
	reader := bufio.NewReader(p.In)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}

func SortRecords(records []ActionRecord) []ActionRecord {
	out := append([]ActionRecord(nil), records...)
	sort.SliceStable(out, func(i, j int) bool {
		return recordSortKey(out[i]) < recordSortKey(out[j])
	})
	return out
}

func recordSortKey(record ActionRecord) string {
	return record.TargetRef + "\x00" + record.SettingRef + "\x00" + record.ResourceID + "\x00" + record.Phase + "\x00" + record.Action + "\x00" + record.Mode + "\x00" + record.Result + "\x00" + record.Code
}
