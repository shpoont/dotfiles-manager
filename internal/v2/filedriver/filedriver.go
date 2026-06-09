package filedriver

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
)

const NormalizerID = "file.bytes.v1"

type ErrorCode string

const (
	CodeNotFound           ErrorCode = "not-found"
	CodePermissionDenied   ErrorCode = "permission-denied"
	CodeInvalidSelector    ErrorCode = "invalid-selector"
	CodeUnsafePath         ErrorCode = "unsafe-path"
	CodeSymlinkUnsupported ErrorCode = "symlink-unsupported"
	CodeSecretDetected     ErrorCode = "secret-detected"
	CodeLifecycleBlocked   ErrorCode = "lifecycle-blocked"
	CodeVerificationFailed ErrorCode = "verification-failed"
	CodeUnsupported        ErrorCode = "unsupported"
	CodeInternal           ErrorCode = "internal-error"
)

type Error struct {
	Code ErrorCode
	Op   string
	Path string
	Err  error
}

func (e *Error) Error() string {
	parts := []string{string(e.Code)}
	if e.Op != "" {
		parts = append(parts, e.Op)
	}
	if e.Path != "" {
		parts = append(parts, e.Path)
	}
	msg := strings.Join(parts, ": ")
	if e.Err != nil {
		msg += ": " + e.Err.Error()
	}
	return msg
}

func (e *Error) Unwrap() error { return e.Err }

func IsCode(err error, code ErrorCode) bool {
	for err != nil {
		if typed, ok := err.(*Error); ok && typed.Code == code {
			return true
		}
		type unwrapper interface{ Unwrap() error }
		unwrapped, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = unwrapped.Unwrap()
	}
	return false
}

type Driver struct{}

type Target struct {
	LocationID        string
	Root              string
	RelPath           string
	AllowMissingRoot  bool
	RejectRootSymlink bool
	RejectLeafSymlink bool
}

type ResolvedPath struct {
	LocationID string
	Root       string
	RootReal   string
	RelPath    string
	AbsPath    string
}

type Detection struct {
	Exists   bool
	Readable bool
	Path     string
}

type State struct {
	Exists     bool
	Bytes      []byte
	SHA256     string
	Normalizer string
}

type Snapshot struct {
	Exists bool
	Size   int
	SHA256 string
}

type ChangeKind string

const (
	ChangeUnchanged ChangeKind = "unchanged"
	ChangeCreate    ChangeKind = "create"
	ChangeUpdate    ChangeKind = "update"
	ChangeDelete    ChangeKind = "delete"
)

type Diff struct {
	Kind   ChangeKind
	Before Snapshot
	After  Snapshot
}

type Preview struct {
	Target     Target
	Path       string
	Change     Diff
	Normalizer string
}

type ApplyResult struct {
	Preview Preview
	Mutated bool
}

type VerifyResult struct {
	Verified bool
	Change   Diff
}

type BackupRequest struct {
	Target Target
	Path   string
	Before State
}

type BackupResult struct {
	ID     string
	Before Snapshot
}

type BackupHook func(BackupRequest) (BackupResult, error)

type RestoreRequest struct {
	Target Target
	Path   string
	Backup BackupResult
}

type RestoreHook func(RestoreRequest) error

func (d Driver) Detect(target Target) (Detection, error) {
	resolved, err := ResolveTarget(target)
	if err != nil {
		return Detection{}, err
	}
	info, err := os.Stat(resolved.AbsPath)
	if os.IsNotExist(err) {
		return Detection{Exists: false, Readable: false, Path: resolved.AbsPath}, nil
	}
	if err != nil {
		return Detection{}, classifyOSError("detect", resolved.AbsPath, err)
	}
	if !info.Mode().IsRegular() {
		return Detection{}, driverError(CodeInvalidSelector, "detect", resolved.AbsPath, fmt.Errorf("path is not a regular file"))
	}
	file, err := os.Open(resolved.AbsPath)
	if err != nil {
		return Detection{}, classifyOSError("detect", resolved.AbsPath, err)
	}
	_ = file.Close()
	return Detection{Exists: true, Readable: true, Path: resolved.AbsPath}, nil
}

func (d Driver) ReadCurrent(target Target) (State, error) {
	resolved, err := ResolveTarget(target)
	if err != nil {
		return State{}, err
	}
	info, err := os.Stat(resolved.AbsPath)
	if os.IsNotExist(err) {
		return AbsentState(), nil
	}
	if err != nil {
		return State{}, classifyOSError("readCurrent", resolved.AbsPath, err)
	}
	if !info.Mode().IsRegular() {
		return State{}, driverError(CodeInvalidSelector, "readCurrent", resolved.AbsPath, fmt.Errorf("path is not a regular file"))
	}
	data, err := os.ReadFile(resolved.AbsPath)
	if err != nil {
		return State{}, classifyOSError("readCurrent", resolved.AbsPath, err)
	}
	return d.Normalize(data), nil
}

func (d Driver) Normalize(raw []byte) State {
	copyBytes := append([]byte(nil), raw...)
	sum := sha256.Sum256(copyBytes)
	return State{
		Exists:     true,
		Bytes:      copyBytes,
		SHA256:     hex.EncodeToString(sum[:]),
		Normalizer: NormalizerID,
	}
}

func AbsentState() State {
	return State{Exists: false, Normalizer: NormalizerID}
}

func (s State) Snapshot() Snapshot {
	if !s.Exists {
		return Snapshot{Exists: false}
	}
	return Snapshot{Exists: true, Size: len(s.Bytes), SHA256: s.SHA256}
}

func (d Driver) Diff(current State, desired State) Diff {
	before := current.Snapshot()
	after := desired.Snapshot()
	switch {
	case !current.Exists && !desired.Exists:
		return Diff{Kind: ChangeUnchanged, Before: before, After: after}
	case !current.Exists && desired.Exists:
		return Diff{Kind: ChangeCreate, Before: before, After: after}
	case current.Exists && !desired.Exists:
		return Diff{Kind: ChangeDelete, Before: before, After: after}
	case current.SHA256 == desired.SHA256 && bytes.Equal(current.Bytes, desired.Bytes):
		return Diff{Kind: ChangeUnchanged, Before: before, After: after}
	default:
		return Diff{Kind: ChangeUpdate, Before: before, After: after}
	}
}

func (d Driver) PreviewApply(target Target, desired State) (Preview, error) {
	resolved, err := ResolveTarget(target)
	if err != nil {
		return Preview{}, err
	}
	current, err := d.ReadCurrent(target)
	if err != nil {
		return Preview{}, err
	}
	return Preview{
		Target:     target,
		Path:       resolved.AbsPath,
		Change:     d.Diff(current, desired),
		Normalizer: NormalizerID,
	}, nil
}

func (d Driver) Backup(target Target, hook BackupHook) (BackupResult, error) {
	resolved, err := ResolveTarget(target)
	if err != nil {
		return BackupResult{}, err
	}
	before, err := d.ReadCurrent(target)
	if err != nil {
		return BackupResult{}, err
	}
	if hook == nil {
		return BackupResult{ID: "noop", Before: before.Snapshot()}, nil
	}
	result, err := hook(BackupRequest{Target: target, Path: resolved.AbsPath, Before: before})
	if err != nil {
		return BackupResult{}, fmt.Errorf("backup hook for %s: %w", resolved.AbsPath, err)
	}
	if result.Before == (Snapshot{}) {
		result.Before = before.Snapshot()
	}
	return result, nil
}

func (d Driver) Apply(target Target, desired State) (ApplyResult, error) {
	result, _, err := d.ApplyWithBackup(target, desired, nil)
	return result, err
}

func (d Driver) ApplyWithBackup(target Target, desired State, hook BackupHook) (ApplyResult, *BackupResult, error) {
	preview, err := d.PreviewApply(target, desired)
	if err != nil {
		return ApplyResult{}, nil, err
	}
	if preview.Change.Kind == ChangeUnchanged {
		return ApplyResult{Preview: preview, Mutated: false}, nil, nil
	}
	backup, err := d.Backup(target, hook)
	if err != nil {
		return ApplyResult{}, nil, err
	}
	if desired.Exists {
		if err := writeTarget(target, desired.Bytes); err != nil {
			return ApplyResult{}, nil, err
		}
	} else {
		if err := removeTarget(target); err != nil {
			return ApplyResult{}, nil, err
		}
	}
	return ApplyResult{Preview: preview, Mutated: true}, &backup, nil
}

func (d Driver) Verify(target Target, desired State) (VerifyResult, error) {
	current, err := d.ReadCurrent(target)
	if err != nil {
		return VerifyResult{}, err
	}
	change := d.Diff(current, desired)
	if change.Kind != ChangeUnchanged {
		resolved, resolveErr := ResolveTarget(target)
		path := ""
		if resolveErr == nil {
			path = resolved.AbsPath
		}
		return VerifyResult{Verified: false, Change: change}, driverError(CodeVerificationFailed, "verify", path, fmt.Errorf("post-apply state differs: %s", change.Kind))
	}
	return VerifyResult{Verified: true, Change: change}, nil
}

func (d Driver) Restore(target Target, backup BackupResult, hook RestoreHook) error {
	resolved, err := ResolveTarget(target)
	if err != nil {
		return err
	}
	if hook == nil {
		return driverError(CodeUnsupported, "restore", resolved.AbsPath, fmt.Errorf("restore requires a caller-provided restore hook in this slice"))
	}
	if err := hook(RestoreRequest{Target: target, Path: resolved.AbsPath, Backup: backup}); err != nil {
		return fmt.Errorf("restore hook for %s: %w", resolved.AbsPath, err)
	}
	return nil
}

func ResolveTarget(target Target) (ResolvedPath, error) {
	root := strings.TrimSpace(target.Root)
	if root == "" {
		return ResolvedPath{}, driverError(CodeInvalidSelector, "resolve", "", fmt.Errorf("target root is required"))
	}
	rel, err := ValidateRelativePath(target.RelPath)
	if err != nil {
		return ResolvedPath{}, driverError(CodeUnsafePath, "resolve", target.RelPath, err)
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return ResolvedPath{}, driverError(CodeInvalidSelector, "resolve", root, err)
	}
	rootLstat, err := os.Lstat(rootAbs)
	if os.IsNotExist(err) {
		if !target.AllowMissingRoot {
			return ResolvedPath{}, driverError(CodeNotFound, "resolve", rootAbs, fmt.Errorf("location root does not exist"))
		}
		candidate := filepath.Join(rootAbs, filepath.FromSlash(rel))
		if err := ensureLexicallyInside(rootAbs, candidate); err != nil {
			return ResolvedPath{}, driverError(CodeUnsafePath, "resolve", candidate, err)
		}
		return ResolvedPath{LocationID: target.LocationID, Root: rootAbs, RootReal: rootAbs, RelPath: rel, AbsPath: candidate}, nil
	}
	if err != nil {
		return ResolvedPath{}, classifyOSError("resolve", rootAbs, err)
	}
	if target.RejectRootSymlink && rootLstat.Mode()&os.ModeSymlink != 0 {
		return ResolvedPath{}, driverError(CodeUnsafePath, "resolve", rootAbs, fmt.Errorf("location root must not be a symlink"))
	}
	rootInfo, err := os.Stat(rootAbs)
	if err != nil {
		return ResolvedPath{}, classifyOSError("resolve", rootAbs, err)
	}
	if !rootInfo.IsDir() {
		return ResolvedPath{}, driverError(CodeInvalidSelector, "resolve", rootAbs, fmt.Errorf("location root is not a directory"))
	}
	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return ResolvedPath{}, classifyOSError("resolve", rootAbs, err)
	}
	candidate := filepath.Join(rootAbs, filepath.FromSlash(rel))
	if err := ensureLexicallyInside(rootAbs, candidate); err != nil {
		return ResolvedPath{}, driverError(CodeUnsafePath, "resolve", candidate, err)
	}
	if err := ensureExistingPathStaysInside(rootAbs, rootReal, rel); err != nil {
		return ResolvedPath{}, err
	}
	if target.RejectLeafSymlink {
		if info, err := os.Lstat(candidate); err == nil && info.Mode()&os.ModeSymlink != 0 {
			return ResolvedPath{}, driverError(CodeSymlinkUnsupported, "resolve", candidate, fmt.Errorf("leaf symlink is unsupported for this resource"))
		} else if err != nil && !os.IsNotExist(err) {
			return ResolvedPath{}, classifyOSError("resolve", candidate, err)
		}
	}
	return ResolvedPath{LocationID: target.LocationID, Root: rootAbs, RootReal: rootReal, RelPath: rel, AbsPath: candidate}, nil
}

func ValidateRelativePath(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("relative path is required")
	}
	if trimmed != value {
		return "", fmt.Errorf("relative path must not have surrounding whitespace: %s", value)
	}
	if strings.Contains(trimmed, "\\") {
		return "", fmt.Errorf("relative path must not contain backslashes: %s", value)
	}
	if filepath.IsAbs(trimmed) || strings.HasPrefix(trimmed, "/") {
		return "", fmt.Errorf("relative path must be relative: %s", value)
	}
	slashed := filepath.ToSlash(trimmed)
	parts := strings.Split(slashed, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("relative path contains unsafe segment: %s", value)
		}
	}
	cleaned := pathpkg.Clean(slashed)
	if cleaned != slashed || strings.HasPrefix(cleaned, "../") || cleaned == ".." {
		return "", fmt.Errorf("relative path escapes root: %s", value)
	}
	return slashed, nil
}

func writeTarget(target Target, data []byte) error {
	resolved, err := ResolveTarget(target)
	if err != nil {
		return err
	}
	writePath := resolved.AbsPath
	if linkInfo, err := os.Lstat(resolved.AbsPath); err == nil && linkInfo.Mode()&os.ModeSymlink != 0 {
		real, err := filepath.EvalSymlinks(resolved.AbsPath)
		if err != nil {
			return classifyOSError("apply", resolved.AbsPath, err)
		}
		if err := ensureRealInside(resolved.RootReal, real); err != nil {
			return driverError(CodeUnsafePath, "apply", resolved.AbsPath, err)
		}
		writePath = real
	}
	if info, err := os.Stat(writePath); err == nil && !info.Mode().IsRegular() {
		return driverError(CodeInvalidSelector, "apply", writePath, fmt.Errorf("path is not a regular file"))
	} else if err != nil && !os.IsNotExist(err) {
		return classifyOSError("apply", writePath, err)
	}
	if err := os.MkdirAll(filepath.Dir(writePath), 0o755); err != nil {
		return classifyOSError("apply", filepath.Dir(writePath), err)
	}
	if err := writeFileAtomic(writePath, data); err != nil {
		return classifyOSError("apply", writePath, err)
	}
	return nil
}

func removeTarget(target Target) error {
	resolved, err := ResolveTarget(target)
	if err != nil {
		return err
	}
	info, err := os.Lstat(resolved.AbsPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return classifyOSError("apply", resolved.AbsPath, err)
	}
	if info.IsDir() {
		return driverError(CodeInvalidSelector, "apply", resolved.AbsPath, fmt.Errorf("path is a directory"))
	}
	if err := os.Remove(resolved.AbsPath); err != nil {
		return classifyOSError("apply", resolved.AbsPath, err)
	}
	return nil
}

func writeFileAtomic(path string, data []byte) error {
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	parent := filepath.Dir(path)
	tmp, err := os.CreateTemp(parent, ".dfm-file-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func ensureExistingPathStaysInside(rootAbs string, rootReal string, rel string) error {
	current := rootAbs
	parts := strings.Split(rel, "/")
	for idx, part := range parts {
		current = filepath.Join(current, filepath.FromSlash(part))
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return classifyOSError("resolve", current, err)
		}
		pathForChildren := current
		if info.Mode()&os.ModeSymlink != 0 {
			real, err := filepath.EvalSymlinks(current)
			if err != nil {
				return classifyOSError("resolve", current, err)
			}
			if err := ensureRealInside(rootReal, real); err != nil {
				return driverError(CodeUnsafePath, "resolve", current, err)
			}
			pathForChildren = real
		}
		if idx < len(parts)-1 {
			statInfo, err := os.Stat(current)
			if err != nil {
				return classifyOSError("resolve", current, err)
			}
			if !statInfo.IsDir() {
				return driverError(CodeInvalidSelector, "resolve", current, fmt.Errorf("ancestor is not a directory"))
			}
			current = pathForChildren
		}
	}
	return nil
}

func ensureLexicallyInside(base string, candidate string) error {
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return err
	}
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return err
	}
	return ensureRealInside(baseAbs, candidateAbs)
}

func ensureRealInside(root string, candidate string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(rootAbs, candidateAbs)
	if err != nil {
		return err
	}
	relSlash := filepath.ToSlash(rel)
	if relSlash == ".." || strings.HasPrefix(relSlash, "../") || filepath.IsAbs(rel) {
		return fmt.Errorf("path escapes root: %s", candidate)
	}
	return nil
}

func classifyOSError(op string, path string, err error) error {
	if os.IsPermission(err) {
		return driverError(CodePermissionDenied, op, path, err)
	}
	if os.IsNotExist(err) {
		return driverError(CodeNotFound, op, path, err)
	}
	return driverError(CodeInternal, op, path, err)
}

func driverError(code ErrorCode, op string, path string, err error) error {
	return &Error{Code: code, Op: op, Path: path, Err: err}
}
