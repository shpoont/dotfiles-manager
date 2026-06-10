package ledger

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/shpoont/dotfiles-manager/internal/v2/filedriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/inidriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/jsondriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/macosdefaultsdriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/plistdriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/shpoont/dotfiles-manager/internal/v2/selectedvalue"
	"github.com/shpoont/dotfiles-manager/internal/v2/tomldriver"
	"github.com/shpoont/dotfiles-manager/internal/v2/yamldriver"
)

const (
	IniFileSelectedDriverVersion               = "ini-file.driver.v1"
	JSONFileSelectedDriverVersion              = "json-file.driver.v1"
	YAMLFileSelectedDriverVersion              = "yaml-file.driver.v1"
	TOMLFileSelectedDriverVersion              = "toml-file.driver.v1"
	PlistFileSelectedDriverVersion             = "plist-file.driver.v1"
	MacOSDefaultsReadOnlySelectedDriverVersion = "macos-defaults-readonly.driver.v1"
)

type SelectedValueBackupRequest struct {
	TargetRef  string
	SettingRef string
	ResourceID string
	Driver     string
	LivePath   string
	Before     NormalizedState
	BeforeFile []byte
}

func (s *Store) WriteSelectedValueBackup(runID string, createdAt time.Time, req SelectedValueBackupRequest) (BackupItem, error) {
	if s == nil {
		return BackupItem{}, fmt.Errorf("ledger store is required")
	}
	if err := validateRunID(runID); err != nil {
		return BackupItem{}, err
	}
	stamp := formatTime(createdAt)
	if stamp == "" {
		stamp = formatTime(s.now().UTC())
	}
	driverVersion := selectedValueDriverVersion(req.Driver)
	key := selectedValueItemKey(req.SettingRef, req.ResourceID)
	ref := stateURI("backups", runID, key)
	payloadRel := ""
	before := fileStateFromSelectedValueBackup(req.BeforeFile)
	if before.Exists {
		payloadRel = filepath.ToSlash(filepath.Join("payloads", key, "before"))
		if err := writeFileAtomic(filepath.Join(s.root, "backups", runID, payloadRel), append([]byte(nil), req.BeforeFile...), 0o600); err != nil {
			return BackupItem{}, fmt.Errorf("write selected-value backup payload: %w", err)
		}
	}
	before.DriverVersion = driverVersion
	item := NormalizeBackupItem(BackupItem{
		Ref:            ref,
		TargetRef:      req.TargetRef,
		SettingRef:     req.SettingRef,
		ResourceID:     req.ResourceID,
		Driver:         req.Driver,
		DriverVersion:  driverVersion,
		LivePath:       req.LivePath,
		PayloadRelPath: payloadRel,
		CreatedAt:      stamp,
		Before:         before,
		Restore: RestoreCompatibility{
			Compatible:    true,
			Driver:        req.Driver,
			DriverVersion: driverVersion,
			Normalizer:    filedriver.NormalizerID,
			Message:       "Restore payload compatibility is recorded for the whole pre-mutation file; selected-value restore execution is handled by the restore flow.",
		},
	})
	if err := s.upsertBackupItem(runID, stamp, item); err != nil {
		return BackupItem{}, err
	}
	return item, nil
}

func fileStateFromSelectedValueBackup(beforeFile []byte) NormalizedState {
	if beforeFile == nil {
		return NormalizedState{Exists: false, Normalizer: filedriver.NormalizerID}
	}
	state := filedriver.Driver{}.Normalize(beforeFile)
	return NormalizedState{Exists: true, Hash: state.SHA256, Normalizer: state.Normalizer, Size: len(state.Bytes)}
}

func SelectedValueDriverVersion(driver string) string {
	return selectedValueDriverVersion(driver)
}

func SelectedValueNormalizer(driver string) string {
	return selectedValueNormalizer(driver)
}

func SelectedValueState(snapshot selectedvalue.Snapshot, driver string) NormalizedState {
	return NormalizedState{Exists: snapshot.Exists, Hash: snapshot.SHA256, Normalizer: snapshot.Normalizer, DriverVersion: selectedValueDriverVersion(driver)}
}

func selectedValueDriverVersion(driver string) string {
	switch driver {
	case recipe.IniFileDriverID:
		return IniFileSelectedDriverVersion
	case recipe.JSONFileDriverID:
		return JSONFileSelectedDriverVersion
	case recipe.YAMLFileDriverID:
		return YAMLFileSelectedDriverVersion
	case recipe.TOMLFileDriverID:
		return TOMLFileSelectedDriverVersion
	case recipe.PlistFileDriverID:
		return PlistFileSelectedDriverVersion
	case recipe.MacOSDefaultsReadOnlyDriverID:
		return MacOSDefaultsReadOnlySelectedDriverVersion
	default:
		return strings.TrimSpace(driver)
	}
}

func selectedValueNormalizer(driver string) string {
	switch driver {
	case recipe.IniFileDriverID:
		return inidriver.NormalizerID
	case recipe.JSONFileDriverID:
		return jsondriver.NormalizerID
	case recipe.YAMLFileDriverID:
		return yamldriver.NormalizerID
	case recipe.TOMLFileDriverID:
		return tomldriver.NormalizerID
	case recipe.PlistFileDriverID:
		return plistdriver.NormalizerID
	case recipe.MacOSDefaultsReadOnlyDriverID:
		return macosdefaultsdriver.NormalizerID
	default:
		return ""
	}
}

func selectedValueItemKey(settingRef string, resourceID string) string {
	base := strings.Trim(itemKeyRegexp.ReplaceAllString(settingRef+"-"+resourceID, "_"), "_")
	if base == "" {
		base = "item"
	}
	if !safePathIDPattern.MatchString(base) {
		base = "item-" + base
	}
	return base
}
