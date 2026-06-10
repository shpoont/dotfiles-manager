package ledger

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/shpoont/dotfiles-manager/internal/v2/nativeexport"
	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
)

type NativeExportBackupRequest struct {
	RepoRoot     string
	TargetRef    string
	SettingRef   string
	ResourceID   string
	StagingRoot  string
	Expected     nativeexport.ExpectedIdentity
	Before       NormalizedState
	OperationID  string
	ArtifactForm string
}

func (s *Store) WriteNativeExportBackup(runID string, createdAt time.Time, req NativeExportBackupRequest) (BackupItem, error) {
	if s == nil {
		return BackupItem{}, fmt.Errorf("ledger store is required")
	}
	if err := ValidateStateRoot(req.RepoRoot, s.root); err != nil {
		return BackupItem{}, err
	}
	if err := validateRunID(runID); err != nil {
		return BackupItem{}, err
	}
	if req.StagingRoot == "" {
		return BackupItem{}, fmt.Errorf("native export backup staging root is required")
	}
	stamp := formatTime(createdAt)
	if stamp == "" {
		stamp = formatTime(s.now().UTC())
	}
	key := selectedValueItemKey(req.SettingRef, req.ResourceID)
	ref := stateURI("backups", runID, key)
	payloadRel := filepath.ToSlash(filepath.Join("payloads", key, "native-export"))
	payloadAbs := filepath.Join(s.root, "backups", runID, payloadRel)
	if err := nativeexport.WriteArtifact(payloadAbs, req.StagingRoot, req.Expected); err != nil {
		return BackupItem{}, fmt.Errorf("write native export backup artifact: %w", err)
	}
	before := req.Before
	before.DriverVersion = nativeexport.DriverVersion
	if before.Normalizer == "" {
		before.Normalizer = nativeexport.Normalizer
	}
	item := NormalizeBackupItem(BackupItem{
		Ref:            ref,
		TargetRef:      req.TargetRef,
		SettingRef:     req.SettingRef,
		ResourceID:     req.ResourceID,
		Driver:         recipe.NativeExportDriverID,
		DriverVersion:  nativeexport.DriverVersion,
		LivePath:       req.OperationID,
		PayloadRelPath: payloadRel,
		CreatedAt:      stamp,
		Before:         before,
		Restore: RestoreCompatibility{
			Compatible:    false,
			Driver:        recipe.NativeExportDriverID,
			DriverVersion: nativeexport.DriverVersion,
			Normalizer:    nativeexport.Normalizer,
			Message:       "Native export backup was recorded before native apply; automatic native restore is not implemented in this tranche.",
		},
	})
	if err := s.upsertBackupItem(runID, stamp, item); err != nil {
		return BackupItem{}, err
	}
	return item, nil
}
