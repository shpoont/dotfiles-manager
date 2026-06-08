package plistdriver

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/shpoont/dotfiles-manager/internal/v2/filedriver"
	"github.com/stretchr/testify/require"
	"howett.net/plist"
)

func TestDriverReadPreviewApplyVerifyXMLSelectedScalar(t *testing.T) {
	t.Parallel()

	driver := Driver{}
	root := t.TempDir()
	path := filepath.Join(root, "config.plist")
	writeXMLPlist(t, path, map[string]any{"feature": true, "user": map[string]any{"email": "old@example.com", "name": "Leon"}})

	req := request(root, Selector{Path: []string{"user", "email"}})
	current, err := driver.ReadCurrent(req)
	require.NoError(t, err)
	require.True(t, current.Exists)
	require.Equal(t, []byte(`"old@example.com"`), current.Value)
	require.Equal(t, NormalizerID, current.Normalizer)

	desired := mustNormalize(t, driver, `"new@example.com"`)
	preview, err := driver.PreviewApply(req, desired)
	require.NoError(t, err)
	require.Equal(t, FormatXML, preview.Format)
	require.Equal(t, filedriver.ChangeUpdate, preview.Change.Kind)
	encodedPreview, err := json.Marshal(preview)
	require.NoError(t, err)
	require.NotContains(t, string(encodedPreview), "old@example.com")
	require.NotContains(t, string(encodedPreview), "new@example.com")

	applied, err := driver.Apply(req, desired)
	require.NoError(t, err)
	require.True(t, applied.Mutated)
	require.Equal(t, FormatXML, applied.Preview.Format)
	requirePlistScalar(t, path, []string{"user", "email"}, "new@example.com")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.True(t, bytes.HasPrefix(data, []byte("<?xml")), string(data[:min(len(data), 40)]))

	verify, err := driver.Verify(req, desired)
	require.NoError(t, err)
	require.True(t, verify.Verified)
}

func TestDriverReadPreviewApplyVerifyBinaryPreservesBinary(t *testing.T) {
	t.Parallel()

	driver := Driver{}
	root := t.TempDir()
	path := filepath.Join(root, "config.plist")
	writeBinaryPlist(t, path, map[string]any{"user": map[string]any{"email": "old@example.com"}})

	req := request(root, Selector{Path: []string{"user", "email"}})
	desired := mustNormalize(t, driver, `"new@example.com"`)
	preview, err := driver.PreviewApply(req, desired)
	require.NoError(t, err)
	require.Equal(t, FormatBinary, preview.Format)

	applied, err := driver.Apply(req, desired)
	require.NoError(t, err)
	require.True(t, applied.Mutated)
	require.Equal(t, FormatBinary, applied.Preview.Format)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.True(t, bytes.HasPrefix(data, []byte("bplist")))
	requirePlistScalar(t, path, []string{"user", "email"}, "new@example.com")
}

func TestDriverCreatesDeletesAndDefaultsDeny(t *testing.T) {
	t.Parallel()

	driver := Driver{}
	desired := mustNormalize(t, driver, `"created@example.com"`)

	t.Run("create missing requires explicit policy and defaults to XML", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		_, err := driver.PreviewApply(request(root, Selector{Path: []string{"user", "email"}}), desired)
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())

		req := request(root, Selector{Path: []string{"user", "email"}, CreateMissing: CreatePolicyCreate})
		preview, err := driver.PreviewApply(req, desired)
		require.NoError(t, err)
		require.Equal(t, FormatXML, preview.Format)
		require.Equal(t, filedriver.ChangeCreate, preview.Change.Kind)
		applied, err := driver.Apply(req, desired)
		require.NoError(t, err)
		require.True(t, applied.Mutated)
		requirePlistScalar(t, filepath.Join(root, "config.plist"), []string{"user", "email"}, "created@example.com")
	})

	t.Run("delete requires explicit policy", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		path := filepath.Join(root, "config.plist")
		writeXMLPlist(t, path, map[string]any{"user": map[string]any{"email": "old@example.com", "name": "Leon"}})

		_, err := driver.PreviewApply(request(root, Selector{Path: []string{"user", "email"}}), DeleteState())
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())

		req := request(root, Selector{Path: []string{"user", "email"}, DeleteKey: DeletePolicyAllow})
		preview, err := driver.PreviewApply(req, DeleteState())
		require.NoError(t, err)
		require.Equal(t, filedriver.ChangeDelete, preview.Change.Kind)
		applied, err := driver.Apply(req, DeleteState())
		require.NoError(t, err)
		require.True(t, applied.Mutated)
		state, err := driver.ReadCurrent(req)
		require.NoError(t, err)
		require.False(t, state.Exists)
		requirePlistScalar(t, path, []string{"user", "name"}, "Leon")
	})
}

func TestDriverRejectsUnsafeOrAmbiguousPlistDocuments(t *testing.T) {
	t.Parallel()

	driver := Driver{}

	for _, tc := range []struct {
		name string
		data []byte
		want string
	}{
		{name: "duplicate XML keys", data: []byte(`<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict><key>email</key><string>one</string><key>email</key><string>two</string></dict></plist>`), want: "duplicate"},
		{name: "duplicate binary keys", data: duplicateKeyBinaryPlist(), want: "duplicate"},
		{name: "binary 128 bit integer", data: binaryRoot128BitInteger(), want: "only up to 8-byte integers"},
		{name: "binary UID", data: binaryPlistWithUID(t), want: "UID"},
		{name: "OpenStep", data: []byte(`{ email = old; }`), want: "unsupported plist format"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(root, "config.plist"), tc.data, 0o644))
			_, err := driver.ReadCurrent(request(root, Selector{Path: []string{"email"}}))
			require.Error(t, err)
			require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
			require.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestDriverRejectsUnsupportedSelectedLeavesAndDesiredValues(t *testing.T) {
	t.Parallel()

	driver := Driver{}
	root := t.TempDir()
	path := filepath.Join(root, "config.plist")
	writeXMLPlist(t, path, map[string]any{
		"array": []any{"x"},
		"dict":  map[string]any{"nested": true},
		"data":  []byte("secret"),
		"date":  time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC),
	})

	for _, tc := range []struct {
		key     string
		message string
	}{
		{key: "array", message: "arrays are unsupported"},
		{key: "dict", message: "dictionaries are unsupported"},
		{key: "data", message: "data values are unsupported"},
		{key: "date", message: "date values are unsupported"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			_, err := driver.ReadCurrent(request(root, Selector{Path: []string{tc.key}}))
			require.Error(t, err)
			require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
			require.Contains(t, err.Error(), tc.message)
		})
	}

	_, err := driver.Normalize([]byte(`null`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "null")
	_, err = driver.NormalizeValue(uint64(math.MaxInt64) + 1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "int64 range")
	_, err = driver.NormalizeValue(math.Inf(1))
	require.Error(t, err)
	require.Contains(t, err.Error(), "finite")
	_, err = driver.NormalizeValue(json.Number("1e1000000"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "valid integer or finite float")
}

func TestDriverRejectsSelectorExpressionsButAllowsDottedKeys(t *testing.T) {
	t.Parallel()

	driver := Driver{}
	root := t.TempDir()
	path := filepath.Join(root, "config.plist")
	writeXMLPlist(t, path, map[string]any{"com.example": map[string]any{"enabled": true}})

	state, err := driver.ReadCurrent(request(root, Selector{Path: []string{"com.example", "enabled"}}))
	require.NoError(t, err)
	require.True(t, state.Exists)
	require.Equal(t, []byte(`true`), state.Value)

	for _, selector := range []Selector{{}, {Path: []string{""}}, {Path: []string{"*"}}, {Path: []string{"$"}}, {Path: []string{"."}}, {Path: []string{".."}}, {Path: []string{"items[0]"}}, {Path: []string{"line\nbreak"}}} {
		_, err := driver.ReadCurrent(request(root, selector))
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
	}
}

func TestDriverPathTraversalAndSymlinkSafety(t *testing.T) {
	t.Parallel()

	driver := Driver{}
	desired := mustNormalize(t, driver, `"safe@example.com"`)
	root := t.TempDir()

	unsafeReq := Request{Target: filedriver.Target{LocationID: "config", Root: root, RelPath: "../bad.plist"}, Selector: Selector{Path: []string{"email"}, CreateMissing: CreatePolicyCreate}}
	_, err := driver.PreviewApply(unsafeReq, desired)
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())

	realPath := filepath.Join(root, "real.plist")
	writeXMLPlist(t, realPath, map[string]any{"email": "old@example.com"})
	require.NoError(t, os.Symlink(realPath, filepath.Join(root, "inside-link.plist")))
	insideReq := Request{Target: filedriver.Target{LocationID: "config", Root: root, RelPath: "inside-link.plist"}, Selector: Selector{Path: []string{"email"}}}
	_, err = driver.Apply(insideReq, desired)
	require.NoError(t, err)
	requirePlistScalar(t, realPath, []string{"email"}, "safe@example.com")

	outsideRoot := t.TempDir()
	outsidePath := filepath.Join(outsideRoot, "outside.plist")
	writeXMLPlist(t, outsidePath, map[string]any{"email": "old@example.com"})
	require.NoError(t, os.Symlink(outsidePath, filepath.Join(root, "outside-link.plist")))
	outsideReq := Request{Target: filedriver.Target{LocationID: "config", Root: root, RelPath: "outside-link.plist"}, Selector: Selector{Path: []string{"email"}}}
	_, err = driver.Apply(outsideReq, desired)
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())
	requirePlistScalar(t, outsidePath, []string{"email"}, "old@example.com")
}

func TestDriverDetectBackupRestoreAndVerificationBranches(t *testing.T) {
	t.Parallel()

	driver := Driver{}

	t.Run("detect missing existing and non regular", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		req := request(root, Selector{Path: []string{"email"}})

		detection, err := driver.Detect(req)
		require.NoError(t, err)
		require.False(t, detection.Exists)
		require.False(t, detection.Readable)
		require.Equal(t, filepath.Join(root, "config.plist"), detection.Path)

		writeXMLPlist(t, filepath.Join(root, "config.plist"), map[string]any{"email": "old@example.com"})
		detection, err = driver.Detect(req)
		require.NoError(t, err)
		require.True(t, detection.Exists)
		require.True(t, detection.Readable)

		require.NoError(t, os.Remove(filepath.Join(root, "config.plist")))
		require.NoError(t, os.Mkdir(filepath.Join(root, "config.plist"), 0o755))
		_, err = driver.Detect(req)
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
	})

	t.Run("backup nil hook custom hook fallback and hook error", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeXMLPlist(t, filepath.Join(root, "config.plist"), map[string]any{"email": "old@example.com"})
		req := request(root, Selector{Path: []string{"email"}})

		noop, err := driver.Backup(req, nil)
		require.NoError(t, err)
		require.Equal(t, "noop", noop.ID)
		require.True(t, noop.Before.Exists)

		var captured BackupRequest
		custom, err := driver.Backup(req, func(req BackupRequest) (BackupResult, error) {
			captured = req
			return BackupResult{ID: "custom"}, nil
		})
		require.NoError(t, err)
		require.Equal(t, "custom", custom.ID)
		require.True(t, custom.Before.Exists)
		require.Equal(t, filepath.Join(root, "config.plist"), captured.Path)
		require.NotEmpty(t, captured.BeforeFile)

		_, err = driver.Backup(req, func(BackupRequest) (BackupResult, error) {
			return BackupResult{}, fmt.Errorf("safe backup failure")
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "safe backup failure")
	})

	t.Run("apply unchanged verify mismatch and restore hooks", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeXMLPlist(t, filepath.Join(root, "config.plist"), map[string]any{"email": "old@example.com"})
		req := request(root, Selector{Path: []string{"email"}})

		same := mustNormalize(t, driver, `"old@example.com"`)
		applied, backup, err := driver.ApplyWithBackup(req, same, func(BackupRequest) (BackupResult, error) {
			require.Fail(t, "unchanged apply must not run backup hook")
			return BackupResult{}, nil
		})
		require.NoError(t, err)
		require.False(t, applied.Mutated)
		require.Nil(t, backup)

		verify, err := driver.Verify(req, mustNormalize(t, driver, `"different@example.com"`))
		require.Error(t, err)
		require.False(t, verify.Verified)
		require.True(t, filedriver.IsCode(err, filedriver.CodeVerificationFailed), err.Error())

		_, err = driver.Verify(req, State{Exists: false})
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())

		err = driver.Restore(req, BackupResult{ID: "backup"}, nil)
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeUnsupported), err.Error())

		var restored RestoreRequest
		err = driver.Restore(req, BackupResult{ID: "backup"}, func(req RestoreRequest) error {
			restored = req
			return nil
		})
		require.NoError(t, err)
		require.Equal(t, "backup", restored.Backup.ID)
		require.Equal(t, filepath.Join(root, "config.plist"), restored.Path)

		err = driver.Restore(req, BackupResult{ID: "backup"}, func(RestoreRequest) error {
			return fmt.Errorf("safe restore failure")
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "safe restore failure")
	})
}

func TestDriverNormalizationSelectionAndPathBranches(t *testing.T) {
	t.Parallel()

	driver := Driver{}
	for _, value := range []any{
		int(1), int8(2), int16(3), int32(4), int64(5),
		uint(6), uint8(7), uint16(8), uint32(9),
		float32(1.25), json.Number("10.5"),
	} {
		_, err := driver.NormalizeValue(value)
		require.NoError(t, err, "%T", value)
	}

	for _, tc := range []struct {
		name  string
		value any
		want  string
	}{
		{name: "bad json number", value: json.Number("not-a-number"), want: "valid JSON number"},
		{name: "nan", value: math.NaN(), want: "finite"},
		{name: "time", value: time.Now(), want: "date values are unsupported"},
		{name: "uid", value: plist.UID(1), want: "UID"},
		{name: "data", value: []byte("secret"), want: "data values are unsupported"},
		{name: "array", value: []any{"secret"}, want: "arrays are unsupported"},
		{name: "map", value: map[string]any{"secret": true}, want: "dictionaries are unsupported"},
		{name: "unknown", value: func() {}, want: "unsupported plist selected scalar type"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := driver.NormalizeValue(tc.value)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.want)
		})
	}

	for _, raw := range [][]byte{[]byte(``), []byte(`true false`), []byte(`null`), []byte(`[]`), []byte(`{}`)} {
		_, err := driver.Normalize(raw)
		require.Error(t, err, string(raw))
		_, err = parseDesiredScalar(raw)
		require.Error(t, err, string(raw))
	}

	selector := Selector{Path: []string{"user", "email"}, CreateMissing: CreatePolicyCreate, DeleteKey: DeletePolicyAllow}
	_, err := setSelected("root", selector, "new@example.com")
	require.Error(t, err)
	require.Contains(t, err.Error(), "root requires dictionary")
	_, err = setSelected(map[string]any{"user": []any{"not-dict"}}, selector, "new@example.com")
	require.Error(t, err)
	require.Contains(t, err.Error(), "requires dictionary container")
	_, err = setSelected(map[string]any{"user": map[string]any{"email": []any{"old"}}}, selector, "new@example.com")
	require.Error(t, err)
	require.Contains(t, err.Error(), "existing selected value")

	_, err = deleteSelected("root", selector)
	require.Error(t, err)
	require.Contains(t, err.Error(), "root requires dictionary")
	_, err = deleteSelected(map[string]any{"user": []any{"not-dict"}}, selector)
	require.Error(t, err)
	require.Contains(t, err.Error(), "requires dictionary container")
	_, err = deleteSelected(map[string]any{"user": map[string]any{"email": []any{"old"}}}, selector)
	require.Error(t, err)
	require.Contains(t, err.Error(), "selected value at path")
	root, err := deleteSelected(map[string]any{"other": true}, selector)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"other": true}, root)
	root, err = deleteSelected(nil, selector)
	require.NoError(t, err)
	require.Nil(t, root)

	err = rejectUIDAnywhere(map[string]any{"nested": []any{plist.UID(7)}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "array index 0")
	require.Equal(t, "unknown(999)", plistFormatDisplay(999))
	_, err = marshalDocument(map[string]any{"email": "old@example.com"}, 999)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown(999)")

	t.Run("raw and write target non regular paths", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(root, "config.plist"), 0o755))
		_, err := driver.ReadCurrent(request(root, Selector{Path: []string{"email"}}))
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())

		err = writeTarget(request(root, Selector{Path: []string{"email"}}).Target, []byte("not plist"))
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
	})

	t.Run("write target direct path errors", func(t *testing.T) {
		t.Parallel()
		err := writeTarget(filedriver.Target{}, []byte("not plist"))
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())

		root := t.TempDir()
		require.NoError(t, os.Symlink(filepath.Join(root, "missing-target.plist"), filepath.Join(root, "config.plist")))
		err = writeTarget(request(root, Selector{Path: []string{"email"}}).Target, []byte("not plist"))
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeNotFound), err.Error())
	})

	t.Run("atomic write preserves existing permissions", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "config.plist")
		require.NoError(t, os.WriteFile(path, []byte("old"), 0o600))
		require.NoError(t, writeFileAtomic(path, []byte("new")))
		info, err := os.Stat(path)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, []byte("new"), data)

		err = writeFileAtomic(filepath.Join(t.TempDir(), "missing-parent", "config.plist"), []byte("new"))
		require.Error(t, err)
	})

	require.Error(t, ensureInside(t.TempDir(), filepath.Join(t.TempDir(), "outside.plist")))
	require.True(t, filedriver.IsCode(classifyOSError("test", "/tmp/missing", os.ErrNotExist), filedriver.CodeNotFound))
	require.True(t, filedriver.IsCode(classifyOSError("test", "/tmp/denied", os.ErrPermission), filedriver.CodePermissionDenied))
	require.True(t, filedriver.IsCode(classifyOSError("test", "/tmp/internal", fmt.Errorf("boom")), filedriver.CodeInternal))
}

func TestDriverErrorBranchesAroundPreviewApplyAndRender(t *testing.T) {
	t.Parallel()

	driver := Driver{}
	desired := mustNormalize(t, driver, `"new@example.com"`)

	t.Run("selector and desired validation errors", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeXMLPlist(t, filepath.Join(root, "config.plist"), map[string]any{"email": "old@example.com"})

		for _, selector := range []Selector{
			{Path: []string{"email"}, CreateMissing: "sometimes"},
			{Path: []string{"email"}, DeleteKey: "sometimes"},
			{Path: []string{"email"}, DuplicatePolicy: "allow"},
		} {
			_, err := driver.PreviewApply(request(root, selector), desired)
			require.Error(t, err)
			require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
		}

		_, err := driver.PreviewApply(request(root, Selector{}), desired)
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())

		_, err = driver.PreviewApply(request(root, Selector{Path: []string{"email"}}), State{Exists: true, Intent: IntentDelete, Value: []byte(`"x"`)})
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
	})

	t.Run("apply surfaces backup render and unsafe write errors", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		path := filepath.Join(root, "config.plist")
		writeXMLPlist(t, path, map[string]any{"email": "old@example.com"})
		req := request(root, Selector{Path: []string{"email"}})

		_, _, err := driver.ApplyWithBackup(req, desired, func(BackupRequest) (BackupResult, error) {
			return BackupResult{}, fmt.Errorf("safe backup apply failure")
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "safe backup apply failure")

		writeXMLPlist(t, path, map[string]any{"email": "old@example.com"})
		_, _, err = driver.ApplyWithBackup(req, desired, func(req BackupRequest) (BackupResult, error) {
			require.NoError(t, os.WriteFile(req.Path, []byte(`not plist`), 0o644))
			return BackupResult{ID: "backup", Before: req.Before.Snapshot()}, nil
		})
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())

		outsideRoot := t.TempDir()
		outsidePath := filepath.Join(outsideRoot, "outside.plist")
		writeXMLPlist(t, outsidePath, map[string]any{"email": "old@example.com"})
		writeXMLPlist(t, path, map[string]any{"email": "old@example.com"})
		_, _, err = driver.ApplyWithBackup(req, desired, func(req BackupRequest) (BackupResult, error) {
			require.NoError(t, os.Remove(req.Path))
			require.NoError(t, os.Symlink(outsidePath, req.Path))
			return BackupResult{ID: "backup", Before: req.Before.Snapshot()}, nil
		})
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())
	})

	t.Run("render direct errors", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		req := request(root, Selector{Path: []string{"email"}, CreateMissing: CreatePolicyCreate, DeleteKey: DeletePolicyAllow})

		_, err := driver.renderDesired(Request{Target: req.Target, Selector: Selector{}}, desired)
		require.Error(t, err)

		_, err = driver.renderDesired(req, State{Exists: false})
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())

		require.NoError(t, os.WriteFile(filepath.Join(root, "config.plist"), []byte(`not plist`), 0o644))
		_, err = driver.renderDesired(req, desired)
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())

		writeXMLPlist(t, filepath.Join(root, "config.plist"), map[string]any{"email": "old@example.com"})
		_, err = driver.renderDesired(req, State{Exists: true, Intent: IntentSet, Value: []byte(`{"not":"scalar"}`)})
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())

		require.NoError(t, os.Remove(filepath.Join(root, "config.plist")))
		data, err := driver.renderDesired(req, DeleteState())
		require.NoError(t, err)
		require.Nil(t, data)
	})

	t.Run("direct selection helper edges", func(t *testing.T) {
		t.Parallel()
		selector := Selector{Path: []string{"user", "email"}, CreateMissing: CreatePolicyCreate}

		_, err := selectScalar("root", selector)
		require.Error(t, err)
		absent, err := selectScalar(map[string]any{}, selector)
		require.NoError(t, err)
		require.False(t, absent.exists)
		absent, err = selectScalar(map[string]any{}, Selector{})
		require.NoError(t, err)
		require.False(t, absent.exists)

		_, err = setSelected(nil, Selector{Path: []string{"email"}}, "new@example.com")
		require.Error(t, err)
		require.Contains(t, err.Error(), "missing plist document")
		_, err = setSelected(map[string]any{}, Selector{Path: []string{"email"}}, "new@example.com")
		require.Error(t, err)
		require.Contains(t, err.Error(), "missing selected path")
		_, err = setSelected(nil, selector, []any{"unsupported"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "desired selected value")
	})
}

func TestDriverXMLAndBinarySafetyBranches(t *testing.T) {
	t.Parallel()

	nestedDuplicateXML := []byte(`<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><array><dict><key>dup</key><true/><key>dup</key><false/></dict></array></plist>`)
	err := validateXMLDuplicateKeys(nestedDuplicateXML)
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate")

	nestedKeyXML := []byte(`<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict><key><string>bad</string></key><true/></dict></plist>`)
	err = validateXMLDuplicateKeys(nestedKeyXML)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unexpected nested element")

	err = validateXMLDuplicateKeys([]byte(`<plist><dict><key>unterminated</key>`))
	require.Error(t, err)

	for _, xmlInput := range []string{
		`<array><string>ignored</string></array>`,
		`<array><array><string>ignored</string></array></array>`,
	} {
		decoder := xml.NewDecoder(strings.NewReader(xmlInput))
		token, err := decoder.Token()
		require.NoError(t, err)
		require.IsType(t, xml.StartElement{}, token)
		require.NoError(t, validateXMLContainer(decoder, "array"))
	}
	decoder := xml.NewDecoder(strings.NewReader(`<array><string>unterminated`))
	token, err := decoder.Token()
	require.NoError(t, err)
	require.IsType(t, xml.StartElement{}, token)
	err = validateXMLContainer(decoder, "array")
	require.Error(t, err)

	for _, tc := range []struct {
		name string
		data []byte
		want string
	}{
		{name: "invalid header", data: []byte("not-a-bplist"), want: "invalid binary plist header"},
		{name: "invalid offset size", data: mutateBinaryTrailer(buildBinaryPlist([][]byte{{0x10, 0x01}}, 0), func(trailer []byte) { trailer[6] = 0 }), want: "invalid binary plist offset/ref size"},
		{name: "unreasonable count", data: mutateBinaryTrailer(buildBinaryPlist([][]byte{{0x10, 0x01}}, 0), func(trailer []byte) { binary.BigEndian.PutUint64(trailer[8:16], 500) }), want: "object count is unreasonable"},
		{name: "offset table escapes", data: mutateBinaryTrailer(buildBinaryPlist([][]byte{{0x10, 0x01}}, 0), func(trailer []byte) { binary.BigEndian.PutUint64(trailer[24:32], uint64(1<<63)) }), want: "offset table escapes"},
		{name: "object offset escapes", data: binaryWithEscapingObjectOffset(), want: "object 0 offset escapes"},
		{name: "reference out of range", data: buildBinaryPlist([][]byte{{0xa1, 0x07}}, 0), want: "out of range"},
		{name: "dict key is not string", data: buildBinaryPlist([][]byte{{0x10, 0x01}, {0x10, 0x02}, {0xd1, 0x00, 0x01}}, 2), want: "not a string"},
		{name: "extended count non integer", data: buildBinaryPlist([][]byte{{0x5f, 0x00}}, 0), want: "extended count does not use an integer"},
		{name: "extended count too wide", data: buildBinaryPlist([][]byte{append([]byte{0x5f, 0x14}, make([]byte, 16)...)}, 0), want: "extended count uses 16 bytes"},
		{name: "duplicate UTF16 keys", data: duplicateUTF16KeyBinaryPlist(), want: "duplicate plist dictionary key"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateBinaryPlistSafety(tc.data)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.want)
		})
	}

	parser := &binaryPlistSafety{data: []byte{0x55}, numObjects: 1, offsets: []uint64{0}, visited: map[uint64]bool{}}
	_, err = parser.stringObject(0)
	require.Error(t, err)
	require.Contains(t, err.Error(), "ASCII string escapes")

	_, err = parser.stringObject(9)
	require.Error(t, err)
	require.Contains(t, err.Error(), "out of range")

	_, err = parser.readUint(0, 0)
	require.Error(t, err)
	_, err = parser.readUint(0, 9)
	require.Error(t, err)
	_, err = parser.readUint(1, 1)
	require.Error(t, err)

	_, err = parser.byteAt(99)
	require.Error(t, err)
	_, _, err = parser.objectCount(99, 0x0f)
	require.Error(t, err)

	escapingUTF16 := &binaryPlistSafety{data: []byte{0x61, 0x00}, numObjects: 1, offsets: []uint64{0}, visited: map[uint64]bool{}}
	_, err = escapingUTF16.stringObject(0)
	require.Error(t, err)
	require.Contains(t, err.Error(), "UTF-16 string escapes")

	visited := &binaryPlistSafety{data: []byte{0x10}, numObjects: 1, offsets: []uint64{0}, visited: map[uint64]bool{0: true}}
	require.NoError(t, visited.validateObject(0))
}

func request(root string, selector Selector) Request {
	return Request{Target: filedriver.Target{LocationID: "config", Root: root, RelPath: "config.plist"}, Selector: selector}
}

func mustNormalize(t *testing.T, driver Driver, raw string) State {
	t.Helper()
	state, err := driver.Normalize([]byte(raw))
	require.NoError(t, err)
	return state
}

func writeXMLPlist(t *testing.T, path string, value any) {
	t.Helper()
	data, err := plist.MarshalIndent(value, plist.XMLFormat, "\t")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

func writeBinaryPlist(t *testing.T, path string, value any) {
	t.Helper()
	data, err := plist.Marshal(value, plist.BinaryFormat)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

func requirePlistScalar(t *testing.T, path string, selector []string, want any) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	doc, err := parsePlistDocument(data)
	require.NoError(t, err)
	current := doc.root
	for _, segment := range selector {
		object, ok := current.(map[string]any)
		require.True(t, ok, "segment %s requires map", segment)
		current = object[segment]
	}
	require.Equal(t, want, current)
}

func duplicateKeyBinaryPlist() []byte {
	objects := [][]byte{
		append([]byte{0x55}, []byte("email")...),
		append([]byte{0x53}, []byte("one")...),
		append([]byte{0x55}, []byte("email")...),
		append([]byte{0x53}, []byte("two")...),
		{0xd2, 0x00, 0x02, 0x01, 0x03},
	}
	return buildBinaryPlist(objects, 4)
}

func binaryRoot128BitInteger() []byte {
	object := append([]byte{0x14}, make([]byte, 16)...)
	object[16] = 1
	return buildBinaryPlist([][]byte{object}, 0)
}

func binaryPlistWithUID(t *testing.T) []byte {
	t.Helper()
	data, err := plist.Marshal(map[string]any{"uid": plist.UID(7)}, plist.BinaryFormat)
	require.NoError(t, err)
	return data
}

func buildBinaryPlist(objects [][]byte, topObject uint64) []byte {
	data := []byte("bplist00")
	offsets := make([]byte, 0, len(objects))
	for _, object := range objects {
		offsets = append(offsets, byte(len(data)))
		data = append(data, object...)
	}
	offsetTableOffset := len(data)
	data = append(data, offsets...)
	trailer := make([]byte, 32)
	trailer[6] = 1
	trailer[7] = 1
	binary.BigEndian.PutUint64(trailer[8:16], uint64(len(objects)))
	binary.BigEndian.PutUint64(trailer[16:24], topObject)
	binary.BigEndian.PutUint64(trailer[24:32], uint64(offsetTableOffset))
	return append(data, trailer...)
}

func mutateBinaryTrailer(data []byte, mutate func([]byte)) []byte {
	copyData := append([]byte(nil), data...)
	mutate(copyData[len(copyData)-32:])
	return copyData
}

func binaryWithEscapingObjectOffset() []byte {
	data := buildBinaryPlist([][]byte{{0x10, 0x01}}, 0)
	offsetTableOffset := int(binary.BigEndian.Uint64(data[len(data)-8:]))
	data[offsetTableOffset] = 0xff
	return data
}

func duplicateUTF16KeyBinaryPlist() []byte {
	objects := [][]byte{
		utf16StringObject("dup"),
		append([]byte{0x53}, []byte("one")...),
		utf16StringObject("dup"),
		append([]byte{0x53}, []byte("two")...),
		{0xd2, 0x00, 0x02, 0x01, 0x03},
	}
	return buildBinaryPlist(objects, 4)
}

func utf16StringObject(value string) []byte {
	units := utf16.Encode([]rune(value))
	if len(units) > 0x0e {
		panic("test helper only supports short UTF16 strings")
	}
	out := []byte{0x60 | byte(len(units))}
	for _, unit := range units {
		out = binary.BigEndian.AppendUint16(out, unit)
	}
	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
