package tomldriver

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shpoont/dotfiles-manager/internal/v2/filedriver"
	"github.com/stretchr/testify/require"
)

func TestDriverReadNormalizePreviewApplyAndVerifySelectedScalar(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "config.toml")
	before := "feature = true\nitems = [{ id = 1 }]\n\n[user]\nname = 'Leon'\nemail = 'old@example.com'\n"
	require.NoError(t, os.WriteFile(path, []byte(before), 0o644))

	driver := Driver{}
	req := request(root, Selector{Path: []string{"user", "email"}})

	detection, err := driver.Detect(req)
	require.NoError(t, err)
	require.True(t, detection.Exists)
	require.True(t, detection.Readable)

	current, err := driver.ReadCurrent(req)
	require.NoError(t, err)
	require.True(t, current.Exists)
	require.Equal(t, []byte(`"old@example.com"`), current.Value)
	require.Equal(t, NormalizerID, current.Normalizer)
	require.NotEmpty(t, current.SHA256)

	oldDesired := mustNormalize(t, driver, `"old@example.com"`)
	require.Equal(t, oldDesired.SHA256, current.SHA256)
	require.Equal(t, filedriver.ChangeUnchanged, driver.Diff(current, oldDesired).Kind)

	desired := mustNormalize(t, driver, `"new@example.com"`)
	preview, err := driver.PreviewApply(req, desired)
	require.NoError(t, err)
	require.Equal(t, filedriver.ChangeUpdate, preview.Change.Kind)
	require.Equal(t, IntentSet, preview.Intent)
	requireFile(t, path, before)

	encodedPreview, err := json.Marshal(preview)
	require.NoError(t, err)
	require.NotContains(t, string(encodedPreview), "old@example.com")
	require.NotContains(t, string(encodedPreview), "new@example.com")
	require.NotContains(t, string(encodedPreview), "Leon")

	applied, err := driver.Apply(req, desired)
	require.NoError(t, err)
	require.True(t, applied.Mutated)
	requireTOMLFile(t, path, "feature = true\n\n[[items]]\nid = 1\n\n[user]\nemail = 'new@example.com'\nname = 'Leon'\n")

	verify, err := driver.Verify(req, desired)
	require.NoError(t, err)
	require.True(t, verify.Verified)
	require.Equal(t, filedriver.ChangeUnchanged, verify.Change.Kind)
}

func TestDriverAdditionalSafetyAndErrorBranches(t *testing.T) {
	t.Parallel()

	driver := Driver{}

	t.Run("detect missing and non regular paths", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		_, err := driver.Detect(Request{Target: filedriver.Target{LocationID: "config", Root: root, RelPath: "../bad.toml"}, Selector: Selector{Path: []string{"email"}}})
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())

		detection, err := driver.Detect(request(root, Selector{Path: []string{"email"}}))
		require.NoError(t, err)
		require.False(t, detection.Exists)
		require.False(t, detection.Readable)

		require.NoError(t, os.Mkdir(filepath.Join(root, "config.toml"), 0o755))
		_, err = driver.Detect(request(root, Selector{Path: []string{"email"}}))
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
		_, err = driver.ReadCurrent(request(root, Selector{Path: []string{"email"}}))
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
	})

	t.Run("scalar normalization edge cases", func(t *testing.T) {
		t.Parallel()
		for _, value := range []any{int(1), int8(2), int16(3), int32(4), int64(5), uint(6), uint8(7), uint16(8), uint32(9), float32(1.25), float64(2.5)} {
			state, err := driver.NormalizeValue(value)
			require.NoError(t, err)
			require.True(t, state.Exists)
			require.NotEmpty(t, state.SHA256)
		}

		_, err := driver.NormalizeValue(uint64(math.MaxInt64) + 1)
		require.Error(t, err)
		require.Contains(t, err.Error(), "int64 range")
		_, err = driver.NormalizeValue(math.NaN())
		require.Error(t, err)
		require.Contains(t, err.Error(), "finite")
		_, err = driver.NormalizeValue(math.Inf(1))
		require.Error(t, err)
		require.Contains(t, err.Error(), "finite")
		_, err = driver.NormalizeValue(func() {})
		require.Error(t, err)
		require.Contains(t, err.Error(), "unsupported TOML selected scalar type")
		_, err = driver.Normalize([]byte(`true false`))
		require.Error(t, err)
		require.Contains(t, err.Error(), "trailing JSON data")
		_, err = driver.Normalize([]byte(`42`))
		require.NoError(t, err)
		_, err = driver.Normalize([]byte(`1.25`))
		require.NoError(t, err)
		_, err = driver.NormalizeValue(json.Number("1e1000000"))
		require.Error(t, err)
		require.Contains(t, err.Error(), "valid integer or finite float")
		_, err = parseJSONScalar([]byte(`true {`))
		require.Error(t, err)
	})

	t.Run("desired intent and verification failures", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		path := filepath.Join(root, "config.toml")
		require.NoError(t, os.WriteFile(path, []byte("email = 'old@example.com'\n"), 0o644))

		_, err := driver.PreviewApply(request(root, Selector{Path: []string{"email"}}), State{Exists: true, Value: []byte(`"new@example.com"`), Intent: IntentDelete})
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
		_, err = driver.PreviewApply(request(root, Selector{Path: []string{"email"}}), State{Exists: true, Value: []byte(`{`), Intent: IntentSet})
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())

		_, err = driver.Verify(request(root, Selector{Path: []string{"email"}}), mustNormalize(t, driver, `"different@example.com"`))
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeVerificationFailed), err.Error())
		_, err = driver.Verify(Request{Target: filedriver.Target{LocationID: "config", Root: root, RelPath: "../bad.toml"}, Selector: Selector{Path: []string{"email"}}}, mustNormalize(t, driver, `"new@example.com"`))
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())
		_, err = driver.Verify(request(root, Selector{Path: []string{"email"}}), AbsentState())
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())

		result, backup, err := driver.ApplyWithBackup(request(root, Selector{Path: []string{"email"}}), mustNormalize(t, driver, `"new@example.com"`), func(BackupRequest) (BackupResult, error) {
			return BackupResult{}, errors.New("safe backup failure")
		})
		require.Error(t, err)
		require.False(t, result.Mutated)
		require.Nil(t, backup)
		requireTOMLFile(t, path, "email = 'old@example.com'\n")
	})

	t.Run("restore hook contract", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		req := request(root, Selector{Path: []string{"email"}})

		err := driver.Restore(Request{Target: filedriver.Target{LocationID: "config", Root: root, RelPath: "../bad.toml"}, Selector: Selector{Path: []string{"email"}}}, BackupResult{ID: "backup"}, nil)
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())

		err = driver.Restore(req, BackupResult{ID: "backup"}, nil)
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeUnsupported), err.Error())

		called := false
		err = driver.Restore(req, BackupResult{ID: "backup"}, func(restore RestoreRequest) error {
			called = true
			require.Equal(t, "backup", restore.Backup.ID)
			require.Equal(t, filepath.Join(root, "config.toml"), restore.Path)
			return nil
		})
		require.NoError(t, err)
		require.True(t, called)
	})

	t.Run("low level helpers fail closed", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		inside := filepath.Join(root, "inside.toml")
		require.NoError(t, ensureInside(root, inside))
		require.Error(t, ensureInside(root, filepath.Join(t.TempDir(), "outside.toml")))

		data, err := marshalDocument(nil)
		require.NoError(t, err)
		require.Nil(t, data)

		parsed, err := parseTOMLDocument([]byte(""))
		require.NoError(t, err)
		require.Equal(t, map[string]any{}, parsed)

		missing, err := readRawFile(filepath.Join(root, "missing.toml"))
		require.NoError(t, err)
		require.Nil(t, missing)

		existingFile := filepath.Join(root, "existing.toml")
		require.NoError(t, os.WriteFile(existingFile, []byte("email = 'old@example.com'\n"), 0o600))
		err = writeFileAtomic(existingFile, []byte("email = 'new@example.com'\n"))
		require.NoError(t, err)
		requireTOMLFile(t, existingFile, "email = 'new@example.com'\n")
		info, err := os.Stat(existingFile)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o600), info.Mode().Perm())

		err = writeFileAtomic(filepath.Join(root, "missing-parent", "config.toml"), []byte("email = 'x'\n"))
		require.Error(t, err)
		dirPath := filepath.Join(root, "dir")
		require.NoError(t, os.Mkdir(dirPath, 0o755))
		err = writeFileAtomic(dirPath, []byte("email = 'x'\n"))
		require.Error(t, err)

		require.True(t, filedriver.IsCode(classifyOSError("read", "missing", os.ErrNotExist), filedriver.CodeNotFound))
		require.True(t, filedriver.IsCode(classifyOSError("read", "denied", os.ErrPermission), filedriver.CodePermissionDenied))
		require.True(t, filedriver.IsCode(classifyOSError("read", "internal", errors.New("boom")), filedriver.CodeInternal))
	})

	t.Run("direct selector mutation helpers fail closed", func(t *testing.T) {
		t.Parallel()

		created, err := setSelected(map[string]any{}, Selector{Path: []string{"user", "email"}, CreateMissing: CreatePolicyCreate}, "new@example.com")
		require.NoError(t, err)
		require.Equal(t, map[string]any{"user": map[string]any{"email": "new@example.com"}}, created)

		_, err = setSelected(nil, Selector{Path: []string{"email"}}, "new@example.com")
		require.Error(t, err)
		require.Contains(t, err.Error(), "missing TOML document")
		_, err = setSelected("not-a-table", Selector{Path: []string{"email"}, CreateMissing: CreatePolicyCreate}, "new@example.com")
		require.Error(t, err)
		require.Contains(t, err.Error(), "selector root requires table document")
		_, err = setSelected(map[string]any{}, Selector{Path: []string{"email"}}, "new@example.com")
		require.Error(t, err)
		require.Contains(t, err.Error(), "missing selected path")
		_, err = setSelected(map[string]any{"user": "flat"}, Selector{Path: []string{"user", "email"}, CreateMissing: CreatePolicyCreate}, "new@example.com")
		require.Error(t, err)
		require.Contains(t, err.Error(), "requires table container")
		_, err = setSelected(map[string]any{}, Selector{Path: []string{"user", "email"}}, "new@example.com")
		require.Error(t, err)
		require.Contains(t, err.Error(), "missing selector container")
		_, err = setSelected(map[string]any{"email": []any{"old"}}, Selector{Path: []string{"email"}, CreateMissing: CreatePolicyCreate}, "new@example.com")
		require.Error(t, err)
		require.Contains(t, err.Error(), "existing selected value")
		_, err = setSelected(map[string]any{}, Selector{Path: []string{"email"}, CreateMissing: CreatePolicyCreate}, []any{"new"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "desired selected value")

		deleted, err := deleteSelected(nil, Selector{Path: []string{"email"}, DeleteKey: DeletePolicyAllow})
		require.NoError(t, err)
		require.Nil(t, deleted)
		deleted, err = deleteSelected(map[string]any{"user": map[string]any{}}, Selector{Path: []string{"user", "email"}, DeleteKey: DeletePolicyAllow})
		require.NoError(t, err)
		require.Equal(t, map[string]any{"user": map[string]any{}}, deleted)
		deleted, err = deleteSelected(map[string]any{"missing": "kept"}, Selector{Path: []string{"user", "email"}, DeleteKey: DeletePolicyAllow})
		require.NoError(t, err)
		require.Equal(t, map[string]any{"missing": "kept"}, deleted)

		_, err = deleteSelected(map[string]any{"email": "old@example.com"}, Selector{Path: []string{"email"}})
		require.Error(t, err)
		require.Contains(t, err.Error(), "delete intent requires")
		_, err = deleteSelected("not-a-table", Selector{Path: []string{"email"}, DeleteKey: DeletePolicyAllow})
		require.Error(t, err)
		require.Contains(t, err.Error(), "selector root requires table document")
		_, err = deleteSelected(map[string]any{"user": "flat"}, Selector{Path: []string{"user", "email"}, DeleteKey: DeletePolicyAllow})
		require.Error(t, err)
		require.Contains(t, err.Error(), "requires table container")
		_, err = deleteSelected(map[string]any{"email": []any{"old"}}, Selector{Path: []string{"email"}, DeleteKey: DeletePolicyAllow})
		require.Error(t, err)
		require.Contains(t, err.Error(), "selected value")
	})

	t.Run("render backup restore and write branches", func(t *testing.T) {
		t.Parallel()

		_, err := parseDesiredScalar([]byte(`null`))
		require.Error(t, err)
		require.Contains(t, err.Error(), "null")
		_, err = parseDesiredScalar([]byte(`["array"]`))
		require.Error(t, err)
		require.Contains(t, err.Error(), "arrays are unsupported")
		_, err = parseDesiredScalar([]byte(`true false`))
		require.Error(t, err)
		require.Contains(t, err.Error(), "trailing JSON data")
		_, err = selectScalar("not-a-table", Selector{Path: []string{"email"}})
		require.Error(t, err)
		require.Contains(t, err.Error(), "requires table container")

		missingRoot := t.TempDir()
		createReq := request(missingRoot, Selector{Path: []string{"email"}, CreateMissing: CreatePolicyCreate})
		backup, err := driver.Backup(createReq, nil)
		require.NoError(t, err)
		require.Equal(t, "noop", backup.ID)
		require.False(t, backup.Before.Exists)

		deleteReq := request(missingRoot, Selector{Path: []string{"email"}, DeleteKey: DeletePolicyAllow})
		rendered, err := driver.renderDesired(deleteReq, DeleteState())
		require.NoError(t, err)
		require.Nil(t, rendered)
		_, err = driver.renderDesired(createReq, AbsentState())
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
		_, err = driver.renderDesired(Request{Target: filedriver.Target{LocationID: "config", Root: missingRoot, RelPath: "../bad.toml"}, Selector: Selector{Path: []string{"email"}}}, DeleteState())
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())

		invalidRoot := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(invalidRoot, "config.toml"), []byte("[broken\n"), 0o644))
		_, err = driver.renderDesired(request(invalidRoot, Selector{Path: []string{"email"}, CreateMissing: CreatePolicyCreate}), mustNormalize(t, driver, `"new@example.com"`))
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())

		unchangedRoot := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(unchangedRoot, "config.toml"), []byte("email = 'same@example.com'\n"), 0o644))
		hookCalled := false
		applied, backupPtr, err := driver.ApplyWithBackup(request(unchangedRoot, Selector{Path: []string{"email"}}), mustNormalize(t, driver, `"same@example.com"`), func(BackupRequest) (BackupResult, error) {
			hookCalled = true
			return BackupResult{ID: "unexpected"}, nil
		})
		require.NoError(t, err)
		require.False(t, applied.Mutated)
		require.Nil(t, backupPtr)
		require.False(t, hookCalled)

		backupRoot := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(backupRoot, "config.toml"), []byte("email = 'old@example.com'\n"), 0o644))
		backup, err = driver.Backup(request(backupRoot, Selector{Path: []string{"email"}}), func(BackupRequest) (BackupResult, error) {
			return BackupResult{ID: "filled-by-driver"}, nil
		})
		require.NoError(t, err)
		require.Equal(t, "filled-by-driver", backup.ID)
		require.True(t, backup.Before.Exists)
		require.NotEmpty(t, backup.Before.SHA256)

		err = driver.Restore(request(backupRoot, Selector{Path: []string{"email"}}), BackupResult{ID: "restore"}, func(RestoreRequest) error {
			return errors.New("restore failed")
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "restore failed")

		writeRoot := t.TempDir()
		err = writeTarget(filedriver.Target{LocationID: "config", Root: writeRoot, RelPath: "nested/config.toml"}, []byte("email = 'new@example.com'\n"))
		require.NoError(t, err)
		requireTOMLFile(t, filepath.Join(writeRoot, "nested", "config.toml"), "email = 'new@example.com'\n")
		err = writeTarget(filedriver.Target{LocationID: "config", Root: writeRoot, RelPath: "../bad.toml"}, []byte("email = 'new@example.com'\n"))
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())

		dirRoot := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(dirRoot, "config.toml"), 0o755))
		err = writeTarget(request(dirRoot, Selector{Path: []string{"email"}}).Target, []byte("email = 'new@example.com'\n"))
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())

		brokenSymlinkRoot := t.TempDir()
		require.NoError(t, os.Symlink(filepath.Join(brokenSymlinkRoot, "missing.toml"), filepath.Join(brokenSymlinkRoot, "config.toml")))
		err = writeTarget(request(brokenSymlinkRoot, Selector{Path: []string{"email"}}).Target, []byte("email = 'new@example.com'\n"))
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeNotFound), err.Error())

		outsideRoot := t.TempDir()
		outsidePath := filepath.Join(outsideRoot, "outside.toml")
		require.NoError(t, os.WriteFile(outsidePath, []byte("email = 'old@example.com'\n"), 0o644))
		outsideSymlinkRoot := t.TempDir()
		require.NoError(t, os.Symlink(outsidePath, filepath.Join(outsideSymlinkRoot, "config.toml")))
		err = writeTarget(request(outsideSymlinkRoot, Selector{Path: []string{"email"}}).Target, []byte("email = 'new@example.com'\n"))
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())
	})
}

func TestDriverSupportsScalarTypesAndRejectsUnsupportedLeaves(t *testing.T) {
	t.Parallel()

	driver := Driver{}
	root := t.TempDir()
	path := filepath.Join(root, "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("s = 'text'\nb = true\ni = 42\nf = 1.25\na = [1, 2]\nd = 2024-01-02\nt = 12:34:56\ndt = 2024-01-02T03:04:05\nodt = 2024-01-02T03:04:05Z\n\n[table]\nvalue = 'nested'\n"), 0o644))

	for _, tc := range []struct {
		name string
		key  string
		want string
	}{
		{name: "string", key: "s", want: `"text"`},
		{name: "bool", key: "b", want: `true`},
		{name: "integer", key: "i", want: `42`},
		{name: "float", key: "f", want: `1.25`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			state, err := driver.ReadCurrent(request(root, Selector{Path: []string{tc.key}}))
			require.NoError(t, err)
			require.True(t, state.Exists)
			require.Equal(t, []byte(tc.want), state.Value)
		})
	}

	for _, tc := range []struct {
		key     string
		message string
	}{
		{key: "a", message: "arrays are unsupported"},
		{key: "d", message: "date/time values are unsupported"},
		{key: "t", message: "date/time values are unsupported"},
		{key: "dt", message: "date/time values are unsupported"},
		{key: "odt", message: "date/time values are unsupported"},
		{key: "table", message: "tables are unsupported"},
	} {
		t.Run("reject "+tc.key, func(t *testing.T) {
			_, err := driver.ReadCurrent(request(root, Selector{Path: []string{tc.key}}))
			require.Error(t, err)
			require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
			require.Contains(t, err.Error(), tc.message)
		})
	}

	_, err := driver.Normalize([]byte(`null`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "null")
	_, err = driver.Normalize([]byte(`{"object":true}`))
	require.Error(t, err)
	_, err = driver.Normalize([]byte(`["array"]`))
	require.Error(t, err)
	_, err = driver.NormalizeValue(json.Number("01"))
	require.Error(t, err)
}

func TestDriverCreatesDeletesAndKeepsBackupRedacted(t *testing.T) {
	t.Parallel()

	driver := Driver{}
	desired := mustNormalize(t, driver, `"leon@example.com"`)

	t.Run("missing paths require explicit create", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		path := filepath.Join(root, "config.toml")
		require.NoError(t, os.WriteFile(path, []byte("[user]\nname = 'Leon'\n"), 0o644))

		_, err := driver.PreviewApply(request(root, Selector{Path: []string{"user", "email"}}), desired)
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
		requireFile(t, path, "[user]\nname = 'Leon'\n")

		req := request(root, Selector{Path: []string{"user", "email"}, CreateMissing: CreatePolicyCreate})
		preview, err := driver.PreviewApply(req, desired)
		require.NoError(t, err)
		require.Equal(t, filedriver.ChangeCreate, preview.Change.Kind)

		var backupReqs []BackupRequest
		applied, backup, err := driver.ApplyWithBackup(req, desired, func(req BackupRequest) (BackupResult, error) {
			backupReqs = append(backupReqs, req)
			return BackupResult{ID: "backup", Before: req.Before.Snapshot()}, nil
		})
		require.NoError(t, err)
		require.True(t, applied.Mutated)
		require.NotNil(t, backup)
		require.Len(t, backupReqs, 1)
		require.Contains(t, string(backupReqs[0].BeforeFile), "Leon")
		requireTOMLFile(t, path, "[user]\nemail = 'leon@example.com'\nname = 'Leon'\n")

		encoded, marshalErr := json.Marshal(backupReqs[0])
		require.NoError(t, marshalErr)
		require.NotContains(t, string(encoded), "Leon")
		require.NotContains(t, string(encoded), "leon@example.com")
	})

	t.Run("missing file create backs up absent file", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		req := request(root, Selector{Path: []string{"user", "email"}, CreateMissing: CreatePolicyCreate})

		var backupReq BackupRequest
		_, backup, err := driver.ApplyWithBackup(req, desired, func(req BackupRequest) (BackupResult, error) {
			backupReq = req
			return BackupResult{ID: "absent", Before: req.Before.Snapshot()}, nil
		})
		require.NoError(t, err)
		require.NotNil(t, backup)
		require.False(t, backupReq.Before.Exists)
		require.Nil(t, backupReq.BeforeFile)
		requireTOMLFile(t, filepath.Join(root, "config.toml"), "[user]\nemail = 'leon@example.com'\n")
	})

	t.Run("delete requires explicit policy", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		path := filepath.Join(root, "config.toml")
		before := "other = true\n\n[user]\nname = 'Leon'\nemail = 'old@example.com'\n"
		require.NoError(t, os.WriteFile(path, []byte(before), 0o644))

		_, err := driver.PreviewApply(request(root, Selector{Path: []string{"user", "email"}}), DeleteState())
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
		requireFile(t, path, before)

		req := request(root, Selector{Path: []string{"user", "email"}, DeleteKey: DeletePolicyAllow})
		preview, err := driver.PreviewApply(req, DeleteState())
		require.NoError(t, err)
		require.Equal(t, filedriver.ChangeDelete, preview.Change.Kind)
		require.Equal(t, IntentDelete, preview.Intent)

		applied, err := driver.Apply(req, DeleteState())
		require.NoError(t, err)
		require.True(t, applied.Mutated)
		requireTOMLFile(t, path, "other = true\n\n[user]\nname = 'Leon'\n")
	})
}

func TestDriverRejectsInvalidSelectorsDuplicateKeysAndInvalidDocuments(t *testing.T) {
	t.Parallel()

	driver := Driver{}
	for _, tc := range []struct {
		name     string
		selector Selector
	}{
		{name: "empty path", selector: Selector{}},
		{name: "empty segment", selector: Selector{Path: []string{"user", ""}}},
		{name: "wildcard", selector: Selector{Path: []string{"user", "*"}}},
		{name: "tomlpath root", selector: Selector{Path: []string{"$"}}},
		{name: "array expression", selector: Selector{Path: []string{"users[0]"}}},
		{name: "unsupported create policy", selector: Selector{Path: []string{"user"}, CreateMissing: "force"}},
		{name: "unsupported delete policy", selector: Selector{Path: []string{"user"}, DeleteKey: "force"}},
		{name: "unsupported duplicate policy", selector: Selector{Path: []string{"user"}, DuplicatePolicy: "last"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := driver.ReadCurrent(request(t.TempDir(), tc.selector))
			require.Error(t, err)
			require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
		})
	}

	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "duplicate key", body: "email = 'one@example.com'\nemail = 'two@example.com'\n"},
		{name: "dotted key table ambiguity", body: "user.email = 'one@example.com'\n\n[user]\nemail = 'two@example.com'\n"},
		{name: "invalid syntax", body: "[user\nemail = 'broken'\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(root, "config.toml"), []byte(tc.body), 0o644))
			_, err := driver.ReadCurrent(request(root, Selector{Path: []string{"user", "email"}}))
			require.Error(t, err)
			require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
		})
	}
}

func TestDriverDeterministicRenderingAndNormalizedHashes(t *testing.T) {
	t.Parallel()

	driver := Driver{}
	desired := mustNormalize(t, driver, `"new@example.com"`)

	rootA := t.TempDir()
	rootB := t.TempDir()
	bodyA := "z = 1\na = 2\n\n[user]\nzeta = 'z'\nemail = 'old@example.com'\nalpha = 'a'\n"
	bodyB := "a = 2\nz = 1\n\n[user]\nalpha = 'a'\nemail = 'old@example.com'\nzeta = 'z'\n"
	require.NoError(t, os.WriteFile(filepath.Join(rootA, "config.toml"), []byte(bodyA), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(rootB, "config.toml"), []byte(bodyB), 0o644))

	renderedA1, err := driver.renderDesired(request(rootA, Selector{Path: []string{"user", "email"}}), desired)
	require.NoError(t, err)
	renderedA2, err := driver.renderDesired(request(rootA, Selector{Path: []string{"user", "email"}}), desired)
	require.NoError(t, err)
	renderedB, err := driver.renderDesired(request(rootB, Selector{Path: []string{"user", "email"}}), desired)
	require.NoError(t, err)
	require.Equal(t, renderedA1, renderedA2)
	require.Equal(t, renderedA1, renderedB)
	require.Equal(t, "a = 2\nz = 1\n\n[user]\nalpha = 'a'\nemail = 'new@example.com'\nzeta = 'z'\n", string(renderedA1))

	quoteA := t.TempDir()
	quoteB := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(quoteA, "config.toml"), []byte("email = 'same@example.com'\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(quoteB, "config.toml"), []byte("email = \"same@example.com\"\n"), 0o644))
	stateA, err := driver.ReadCurrent(request(quoteA, Selector{Path: []string{"email"}}))
	require.NoError(t, err)
	stateB, err := driver.ReadCurrent(request(quoteB, Selector{Path: []string{"email"}}))
	require.NoError(t, err)
	require.Equal(t, stateA.SHA256, stateB.SHA256)
	require.Equal(t, stateA.Value, stateB.Value)
}

func TestDriverPathTraversalAndSymlinkSafety(t *testing.T) {
	t.Parallel()

	driver := Driver{}
	desired := mustNormalize(t, driver, `"safe@example.com"`)

	root := t.TempDir()
	unsafeReq := Request{Target: filedriver.Target{LocationID: "config", Root: root, RelPath: "../bad.toml"}, Selector: Selector{Path: []string{"email"}, CreateMissing: CreatePolicyCreate}}
	_, err := driver.PreviewApply(unsafeReq, desired)
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())

	realPath := filepath.Join(root, "real.toml")
	require.NoError(t, os.WriteFile(realPath, []byte("email = 'old@example.com'\n"), 0o644))
	require.NoError(t, os.Symlink(realPath, filepath.Join(root, "inside-link.toml")))
	insideReq := Request{Target: filedriver.Target{LocationID: "config", Root: root, RelPath: "inside-link.toml"}, Selector: Selector{Path: []string{"email"}}}
	_, err = driver.Apply(insideReq, desired)
	require.NoError(t, err)
	requireTOMLFile(t, realPath, "email = 'safe@example.com'\n")

	outsideRoot := t.TempDir()
	outsidePath := filepath.Join(outsideRoot, "outside.toml")
	require.NoError(t, os.WriteFile(outsidePath, []byte("email = 'old@example.com'\n"), 0o644))
	require.NoError(t, os.Symlink(outsidePath, filepath.Join(root, "outside-link.toml")))
	outsideReq := Request{Target: filedriver.Target{LocationID: "config", Root: root, RelPath: "outside-link.toml"}, Selector: Selector{Path: []string{"email"}}}
	_, err = driver.Apply(outsideReq, desired)
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())
	requireTOMLFile(t, outsidePath, "email = 'old@example.com'\n")
}

func request(root string, selector Selector) Request {
	return Request{Target: filedriver.Target{LocationID: "config", Root: root, RelPath: "config.toml"}, Selector: selector}
}

func mustNormalize(t *testing.T, driver Driver, raw string) State {
	t.Helper()
	state, err := driver.Normalize([]byte(raw))
	require.NoError(t, err)
	return state
}

func requireFile(t *testing.T, path string, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, want, string(data))
}

func requireTOMLFile(t *testing.T, path string, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, strings.ReplaceAll(want, "\r\n", "\n"), strings.ReplaceAll(string(data), "\r\n", "\n"))
}
