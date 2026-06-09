// Package ledger persists the v2 local mutation evidence for runs, backups, and
// verified last-applied outcomes.
//
// The package is intentionally internal and CLI-adjacent. It does not choose
// prompts, render previews, implement restore execution, or make v1 commands use
// v2 behavior. Callers provide resolved plans and use the store as the local
// state layer required by the v2 mutation transaction model.
package ledger

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	LedgerEntrySchema    = "dotfiles-manager.v2.ledger-entry"
	RunRecordSchema      = "dotfiles-manager.v2.run-record"
	BackupMetadataSchema = "dotfiles-manager.v2.backup-metadata"
	SchemaVersion        = 1

	StateScheme = "state"
)

var safePathIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

type Store struct {
	root string
	now  func() time.Time
}

type StoreOption func(*Store)

func WithClock(clock func() time.Time) StoreOption {
	return func(store *Store) {
		if clock != nil {
			store.now = clock
		}
	}
}

func NewStore(root string, opts ...StoreOption) (*Store, error) {
	trimmed := strings.TrimSpace(root)
	if trimmed == "" {
		return nil, fmt.Errorf("ledger state root is required")
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return nil, fmt.Errorf("resolve ledger state root %q: %w", root, err)
	}
	store := &Store{root: abs, now: time.Now}
	for _, opt := range opts {
		if opt != nil {
			opt(store)
		}
	}
	return store, nil
}

func (s *Store) Root() string {
	if s == nil {
		return ""
	}
	return s.root
}

func RepoStateID(repoRoot string) (string, error) {
	root := strings.TrimSpace(repoRoot)
	if root == "" {
		return "", fmt.Errorf("repo root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve repo root %q: %w", repoRoot, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("stat repo root %q: %w", abs, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("repo root is not a directory: %s", abs)
	}
	realRoot, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolve real repo root %q: %w", abs, err)
	}
	sum := sha256.Sum256([]byte(realRoot))
	return hex.EncodeToString(sum[:]), nil
}

func DefaultStateRoot(repoRoot string) (string, error) {
	stateID, err := RepoStateID(repoRoot)
	if err != nil {
		return "", err
	}
	return defaultStateRootForOS(runtime.GOOS, stateID)
}

func defaultStateRootForOS(goos string, stateID string) (string, error) {
	switch goos {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory for state root: %w", err)
		}
		return filepath.Join(home, "Library", "Application Support", "dotfiles-manager", "v2", stateID), nil
	case "linux":
		base := strings.TrimSpace(os.Getenv("XDG_STATE_HOME"))
		if base == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("resolve home directory for state root: %w", err)
			}
			base = filepath.Join(home, ".local", "state")
		}
		return filepath.Join(base, "dotfiles-manager", "v2", stateID), nil
	default:
		return "", fmt.Errorf("unsupported OS for v2 local state root: %s", goos)
	}
}

func ValidateStateRoot(repoRoot string, stateRoot string) error {
	repoAbs, err := filepath.Abs(strings.TrimSpace(repoRoot))
	if err != nil {
		return fmt.Errorf("resolve repo root %q: %w", repoRoot, err)
	}
	stateAbs, err := filepath.Abs(strings.TrimSpace(stateRoot))
	if err != nil {
		return fmt.Errorf("resolve state root %q: %w", stateRoot, err)
	}
	if strings.TrimSpace(repoRoot) == "" || strings.TrimSpace(stateRoot) == "" {
		return fmt.Errorf("repo root and state root are required")
	}
	repoReal, err := realOrAbs(repoAbs)
	if err != nil {
		return fmt.Errorf("resolve repo root %q: %w", repoAbs, err)
	}
	stateReal, err := realOrAbs(stateAbs)
	if err != nil {
		return fmt.Errorf("resolve state root %q: %w", stateAbs, err)
	}
	if sameOrInside(repoReal, stateReal) {
		return fmt.Errorf("local state root must not resolve inside repository: %s", stateReal)
	}
	for _, rel := range []string{"desired", "profiles", "recipes"} {
		protected := filepath.Join(repoReal, rel)
		if sameOrInside(protected, stateReal) {
			return fmt.Errorf("local state root must not resolve inside repository %s/: %s", rel, stateReal)
		}
	}
	return nil
}

func (s *Store) WriteRunRecord(record RunRecord) error {
	if s == nil {
		return fmt.Errorf("ledger store is required")
	}
	record = NormalizeRunRecord(record)
	if err := validateRunID(record.RunID); err != nil {
		return err
	}
	path := filepath.Join(s.root, "ledger", "runs", record.RunID+".json")
	payload, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("encode run record %s: %w", record.RunID, err)
	}
	payload = append(payload, '\n')
	if err := writeFileAtomic(path, payload, 0o644); err != nil {
		return fmt.Errorf("write run record %s: %w", path, err)
	}
	return nil
}

func (s *Store) AppendLedgerEntries(entries []LedgerEntry) (err error) {
	if s == nil {
		return fmt.Errorf("ledger store is required")
	}
	entries = NormalizeLedgerEntries(entries)
	if len(entries) == 0 {
		return nil
	}
	path := filepath.Join(s.root, "ledger", "ledger.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create ledger directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open ledger %s: %w", path, err)
	}
	defer func() {
		if closeErr := file.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close ledger %s: %w", path, closeErr)
		}
	}()
	for _, entry := range entries {
		payload, err := json.Marshal(entry)
		if err != nil {
			return fmt.Errorf("encode ledger entry %s/%s: %w", entry.RunID, entry.Item.SettingRef, err)
		}
		if _, err := file.Write(append(payload, '\n')); err != nil {
			return fmt.Errorf("append ledger entry %s/%s: %w", entry.RunID, entry.Item.SettingRef, err)
		}
	}
	return nil
}

func (s *Store) CommitRun(record RunRecord) ([]LedgerEntry, error) {
	record = NormalizeRunRecord(record)
	if err := s.WriteRunRecord(record); err != nil {
		return nil, err
	}
	entries := LedgerEntriesForRun(record)
	if err := s.AppendLedgerEntries(entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func (s *Store) ListBackups() ([]BackupMetadata, error) {
	if s == nil {
		return nil, fmt.Errorf("ledger store is required")
	}
	root := filepath.Join(s.root, "backups")
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read backups root %s: %w", root, err)
	}
	backups := make([]BackupMetadata, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name(), "backup.yaml")
		metadata, err := readBackupMetadata(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		backups = append(backups, metadata)
	}
	sort.SliceStable(backups, func(i, j int) bool {
		left := backups[i].CreatedAt + "\x00" + backups[i].RunID
		right := backups[j].CreatedAt + "\x00" + backups[j].RunID
		return left < right
	})
	return backups, nil
}

func NormalizeRunRecord(record RunRecord) RunRecord {
	if record.Schema == "" {
		record.Schema = RunRecordSchema
	}
	if record.SchemaVersion == 0 {
		record.SchemaVersion = SchemaVersion
	}
	record.RunID = strings.TrimSpace(record.RunID)
	record.Command = strings.TrimSpace(record.Command)
	record.ProfileStack = trimStrings(record.ProfileStack)
	record.Items = append([]ItemRecord(nil), record.Items...)
	for i := range record.Items {
		record.Items[i] = NormalizeItemRecord(record.Items[i])
	}
	sort.SliceStable(record.Items, func(i, j int) bool {
		return itemRecordSortKey(record.Items[i]) < itemRecordSortKey(record.Items[j])
	})
	record.Summary = summarizeItems(record.Items)
	if record.Status == "" {
		record.Status = record.Summary.Status
	}
	return record
}

func NormalizeItemRecord(item ItemRecord) ItemRecord {
	item.TargetRef = strings.TrimSpace(item.TargetRef)
	item.SettingRef = strings.TrimSpace(item.SettingRef)
	item.Operation = strings.TrimSpace(item.Operation)
	item.Driver = strings.TrimSpace(item.Driver)
	item.DriverVersion = strings.TrimSpace(item.DriverVersion)
	item.ResourceID = strings.TrimSpace(item.ResourceID)
	item.DesiredURI = strings.TrimSpace(item.DesiredURI)
	item.DesiredRelPath = strings.TrimSpace(item.DesiredRelPath)
	item.LivePath = strings.TrimSpace(item.LivePath)
	item.DesiredPath = strings.TrimSpace(item.DesiredPath)
	item.ArtifactRefs = NormalizeArtifactRefs(item.ArtifactRefs)
	item.SourceBackupRefs = trimStrings(item.SourceBackupRefs)
	sort.Strings(item.SourceBackupRefs)
	item.BackupRefs = trimStrings(item.BackupRefs)
	sort.Strings(item.BackupRefs)
	item.Diagnostics = append([]Diagnostic(nil), item.Diagnostics...)
	sort.SliceStable(item.Diagnostics, func(i, j int) bool {
		return item.Diagnostics[i].Code+"\x00"+item.Diagnostics[i].Message < item.Diagnostics[j].Code+"\x00"+item.Diagnostics[j].Message
	})
	if item.Result == "" {
		if item.Verification.Verified {
			item.Result = ItemResultVerified
		} else {
			item.Result = ItemResultFailed
		}
	}
	return item
}

func NormalizeArtifactRefs(refs ArtifactRefs) ArtifactRefs {
	refs.Desired = strings.TrimSpace(refs.Desired)
	refs.DesiredURI = strings.TrimSpace(refs.DesiredURI)
	refs.DesiredPath = strings.TrimSpace(refs.DesiredPath)
	refs.LivePath = strings.TrimSpace(refs.LivePath)
	refs.SourceBackup = strings.TrimSpace(refs.SourceBackup)
	refs.SourceBackupPayload = strings.TrimSpace(refs.SourceBackupPayload)
	refs.Backup = strings.TrimSpace(refs.Backup)
	refs.BackupPayload = strings.TrimSpace(refs.BackupPayload)
	refs.RunRecord = strings.TrimSpace(refs.RunRecord)
	refs.Ledger = strings.TrimSpace(refs.Ledger)
	return refs
}

func NormalizeLedgerEntries(entries []LedgerEntry) []LedgerEntry {
	normalized := append([]LedgerEntry(nil), entries...)
	for i := range normalized {
		entry := &normalized[i]
		if entry.Schema == "" {
			entry.Schema = LedgerEntrySchema
		}
		if entry.SchemaVersion == 0 {
			entry.SchemaVersion = SchemaVersion
		}
		entry.RunID = strings.TrimSpace(entry.RunID)
		entry.Command = strings.TrimSpace(entry.Command)
		entry.ProfileStack = trimStrings(entry.ProfileStack)
		entry.Item = NormalizeItemRecord(entry.Item)
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		return normalized[i].RunID+"\x00"+itemRecordSortKey(normalized[i].Item) < normalized[j].RunID+"\x00"+itemRecordSortKey(normalized[j].Item)
	})
	return normalized
}

func LedgerEntriesForRun(record RunRecord) []LedgerEntry {
	record = NormalizeRunRecord(record)
	entries := make([]LedgerEntry, 0, len(record.Items))
	for _, item := range record.Items {
		if !item.Verification.Verified || item.Result != ItemResultVerified {
			continue
		}
		entries = append(entries, LedgerEntry{
			Schema:        LedgerEntrySchema,
			SchemaVersion: SchemaVersion,
			RunID:         record.RunID,
			Timestamp:     record.FinishedAt,
			Command:       record.Command,
			ProfileStack:  append([]string(nil), record.ProfileStack...),
			Item:          item,
		})
	}
	return NormalizeLedgerEntries(entries)
}

func stateURI(parts ...string) string {
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.Trim(strings.TrimSpace(part), "/")
		if trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return StateScheme + "://" + strings.Join(cleaned, "/")
}

func validateRunID(runID string) error {
	trimmed := strings.TrimSpace(runID)
	if !safePathIDPattern.MatchString(trimmed) {
		return fmt.Errorf("run ID must be a safe path id, got %q", runID)
	}
	return nil
}

func readBackupMetadata(path string) (BackupMetadata, error) {
	file, err := os.Open(path)
	if err != nil {
		return BackupMetadata{}, err
	}
	defer func() { _ = file.Close() }()
	var metadata BackupMetadata
	dec := yaml.NewDecoder(file)
	if err := dec.Decode(&metadata); err != nil {
		return BackupMetadata{}, fmt.Errorf("parse backup metadata %s: %w", path, err)
	}
	return NormalizeBackupMetadata(metadata), nil
}

func writeBackupMetadata(path string, metadata BackupMetadata) error {
	metadata = NormalizeBackupMetadata(metadata)
	payload, err := yaml.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("encode backup metadata %s: %w", metadata.RunID, err)
	}
	if err := writeFileAtomic(path, payload, 0o644); err != nil {
		return fmt.Errorf("write backup metadata %s: %w", path, err)
	}
	return nil
}

func NormalizeBackupMetadata(metadata BackupMetadata) BackupMetadata {
	if metadata.Schema == "" {
		metadata.Schema = BackupMetadataSchema
	}
	if metadata.SchemaVersion == 0 {
		metadata.SchemaVersion = SchemaVersion
	}
	metadata.RunID = strings.TrimSpace(metadata.RunID)
	metadata.Items = append([]BackupItem(nil), metadata.Items...)
	for i := range metadata.Items {
		metadata.Items[i] = NormalizeBackupItem(metadata.Items[i])
	}
	sort.SliceStable(metadata.Items, func(i, j int) bool {
		return metadata.Items[i].Ref < metadata.Items[j].Ref
	})
	return metadata
}

func NormalizeBackupItem(item BackupItem) BackupItem {
	item.Ref = strings.TrimSpace(item.Ref)
	item.TargetRef = strings.TrimSpace(item.TargetRef)
	item.SettingRef = strings.TrimSpace(item.SettingRef)
	item.ResourceID = strings.TrimSpace(item.ResourceID)
	item.Driver = strings.TrimSpace(item.Driver)
	item.DriverVersion = strings.TrimSpace(item.DriverVersion)
	item.LivePath = strings.TrimSpace(item.LivePath)
	item.PayloadRelPath = filepath.ToSlash(strings.TrimSpace(item.PayloadRelPath))
	item.Restore.Driver = strings.TrimSpace(item.Restore.Driver)
	item.Restore.DriverVersion = strings.TrimSpace(item.Restore.DriverVersion)
	item.Restore.Normalizer = strings.TrimSpace(item.Restore.Normalizer)
	item.Restore.Message = strings.TrimSpace(item.Restore.Message)
	return item
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".dfm-ledger-*")
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

func realOrAbs(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return real, nil
	}
	parts := []string{}
	current := abs
	for {
		if real, err := filepath.EvalSymlinks(current); err == nil {
			for i := len(parts) - 1; i >= 0; i-- {
				real = filepath.Join(real, parts[i])
			}
			return real, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return abs, nil
		}
		parts = append(parts, filepath.Base(current))
		current = parent
	}
}

func sameOrInside(parent string, child string) bool {
	parentAbs, err := filepath.Abs(parent)
	if err != nil {
		return false
	}
	childAbs, err := filepath.Abs(child)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(parentAbs, childAbs)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	return rel == "." || rel == "" || (!strings.HasPrefix(rel, "../") && rel != ".." && !filepath.IsAbs(rel))
}

func trimStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		out = append(out, trimmed)
	}
	return out
}

func itemRecordSortKey(item ItemRecord) string {
	return item.TargetRef + "\x00" + item.SettingRef + "\x00" + item.Operation + "\x00" + item.ResourceID
}
