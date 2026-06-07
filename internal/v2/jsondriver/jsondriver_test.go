package jsondriver

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/shpoont/dotfiles-manager/internal/v2/filedriver"
	"github.com/stretchr/testify/require"
)

func TestDriverReadNormalizePreviewApplyAndVerifySelectedScalar(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "config.json")
	before := `{"user":{"name":"Leon","email":"old@example.com"},"feature":true,"items":[{"id":1}]}`
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
	requireJSONFile(t, path, `{
  "feature": true,
  "items": [
    {
      "id": 1
    }
  ],
  "user": {
    "email": "new@example.com",
    "name": "Leon"
  }
}
`)

	verify, err := driver.Verify(req, desired)
	require.NoError(t, err)
	require.True(t, verify.Verified)
	require.Equal(t, filedriver.ChangeUnchanged, verify.Change.Kind)
}

func TestDriverCreatesMissingPathOnlyWhenPolicyAllows(t *testing.T) {
	t.Parallel()

	driver := Driver{}
	desired := mustNormalize(t, driver, `"leon@example.com"`)

	t.Run("missing leaf requires explicit create", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		path := filepath.Join(root, "config.json")
		require.NoError(t, os.WriteFile(path, []byte(`{"user":{"name":"Leon"},"other":true}`), 0o644))

		_, err := driver.PreviewApply(request(root, Selector{Path: []string{"user", "email"}}), desired)
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
		requireFile(t, path, `{"user":{"name":"Leon"},"other":true}`)

		req := request(root, Selector{Path: []string{"user", "email"}, CreateMissing: CreatePolicyCreate})
		preview, err := driver.PreviewApply(req, desired)
		require.NoError(t, err)
		require.Equal(t, filedriver.ChangeCreate, preview.Change.Kind)
		requireFile(t, path, `{"user":{"name":"Leon"},"other":true}`)

		applied, err := driver.Apply(req, desired)
		require.NoError(t, err)
		require.True(t, applied.Mutated)
		requireJSONFile(t, path, `{
  "other": true,
  "user": {
    "email": "leon@example.com",
    "name": "Leon"
  }
}
`)
	})

	t.Run("missing containers and file require explicit create", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		path := filepath.Join(root, "config.json")
		require.NoError(t, os.WriteFile(path, []byte(`{"other":true}`), 0o644))

		_, err := driver.PreviewApply(request(root, Selector{Path: []string{"user", "identity", "email"}}), desired)
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())

		req := request(root, Selector{Path: []string{"user", "identity", "email"}, CreateMissing: CreatePolicyCreate})
		_, err = driver.Apply(req, desired)
		require.NoError(t, err)
		requireJSONFile(t, path, `{
  "other": true,
  "user": {
    "identity": {
      "email": "leon@example.com"
    }
  }
}
`)

		missingRoot := filepath.Join(root, "missing-root")
		missingReq := Request{Target: filedriver.Target{LocationID: "config", Root: missingRoot, RelPath: "config.json", AllowMissingRoot: true}, Selector: Selector{Path: []string{"user", "email"}, CreateMissing: CreatePolicyCreate}}
		_, err = driver.Apply(missingReq, desired)
		require.NoError(t, err)
		requireJSONFile(t, filepath.Join(missingRoot, "config.json"), `{
  "user": {
    "email": "leon@example.com"
  }
}
`)
	})
}

func TestDriverDeletesSelectedScalarOnlyWhenPolicyAllows(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "config.json")
	before := `{"user":{"name":"Leon","email":"old@example.com"},"other":true}`
	require.NoError(t, os.WriteFile(path, []byte(before), 0o644))
	driver := Driver{}

	defaultReq := request(root, Selector{Path: []string{"user", "email"}})
	_, err := driver.PreviewApply(defaultReq, DeleteState())
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
	requireFile(t, path, before)

	req := request(root, Selector{Path: []string{"user", "email"}, DeleteKey: DeletePolicyAllow})
	preview, err := driver.PreviewApply(req, DeleteState())
	require.NoError(t, err)
	require.Equal(t, filedriver.ChangeDelete, preview.Change.Kind)
	require.Equal(t, IntentDelete, preview.Intent)
	requireFile(t, path, before)

	applied, err := driver.Apply(req, DeleteState())
	require.NoError(t, err)
	require.True(t, applied.Mutated)
	requireJSONFile(t, path, `{
  "other": true,
  "user": {
    "name": "Leon"
  }
}
`)

	applied, err = driver.Apply(req, DeleteState())
	require.NoError(t, err)
	require.False(t, applied.Mutated)

	missingDeleteReq := request(root, Selector{Path: []string{"user", "missing"}})
	_, err = driver.PreviewApply(missingDeleteReq, DeleteState())
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
}

func TestDriverSupportsJSONScalarTypesAndRejectsObjectArrayLeafValues(t *testing.T) {
	t.Parallel()

	driver := Driver{}
	root := t.TempDir()
	path := filepath.Join(root, "config.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"s":"text","n":1.25,"b":true,"z":null,"o":{},"a":[]}`), 0o644))

	for _, tc := range []struct {
		name string
		key  string
		want string
	}{
		{name: "string", key: "s", want: `"text"`},
		{name: "number", key: "n", want: `1.25`},
		{name: "bool", key: "b", want: `true`},
		{name: "null", key: "z", want: `null`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			state, err := driver.ReadCurrent(request(root, Selector{Path: []string{tc.key}}))
			require.NoError(t, err)
			require.True(t, state.Exists)
			require.Equal(t, []byte(tc.want), state.Value)
		})
	}

	for _, key := range []string{"o", "a"} {
		_, err := driver.ReadCurrent(request(root, Selector{Path: []string{key}}))
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
	}

	_, err := driver.Normalize([]byte(`{"object":true}`))
	require.Error(t, err)
	_, err = driver.Normalize([]byte(`["array"]`))
	require.Error(t, err)
}

func TestDriverRejectsInvalidSelectorsDuplicateKeysAndAmbiguousDocuments(t *testing.T) {
	t.Parallel()

	driver := Driver{}

	cases := []struct {
		name     string
		selector Selector
	}{
		{name: "empty path", selector: Selector{}},
		{name: "empty segment", selector: Selector{Path: []string{"user", ""}}},
		{name: "wildcard", selector: Selector{Path: []string{"user", "*"}}},
		{name: "jsonpath root", selector: Selector{Path: []string{"$"}}},
		{name: "bracket expression", selector: Selector{Path: []string{"users[0]"}}},
		{name: "unsupported create policy", selector: Selector{Path: []string{"user"}, CreateMissing: "force"}},
		{name: "unsupported delete policy", selector: Selector{Path: []string{"user"}, DeleteKey: "force"}},
		{name: "unsupported duplicate policy", selector: Selector{Path: []string{"user"}, DuplicatePolicy: "last"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			_, err := driver.ReadCurrent(request(root, tc.selector))
			require.Error(t, err)
			require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
		})
	}

	duplicateRoot := t.TempDir()
	duplicatePath := filepath.Join(duplicateRoot, "config.json")
	require.NoError(t, os.WriteFile(duplicatePath, []byte(`{"user":{"email":"one@example.com","email":"two@example.com"}}`), 0o644))
	_, err := driver.ReadCurrent(request(duplicateRoot, Selector{Path: []string{"user", "email"}}))
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
	requireFile(t, duplicatePath, `{"user":{"email":"one@example.com","email":"two@example.com"}}`)

	unrelatedDuplicateRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(unrelatedDuplicateRoot, "config.json"), []byte(`{"user":{"email":"one@example.com"},"other":{"dup":1,"dup":2}}`), 0o644))
	_, err = driver.PreviewApply(request(unrelatedDuplicateRoot, Selector{Path: []string{"user", "email"}}), mustNormalize(t, driver, `"two@example.com"`))
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())

	arrayTraversalRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(arrayTraversalRoot, "config.json"), []byte(`{"users":[{"email":"one@example.com"}]}`), 0o644))
	_, err = driver.ReadCurrent(request(arrayTraversalRoot, Selector{Path: []string{"users", "email"}}))
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())

	nonObjectRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(nonObjectRoot, "config.json"), []byte(`[]`), 0o644))
	_, err = driver.ReadCurrent(request(nonObjectRoot, Selector{Path: []string{"user"}}))
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
}

func TestDriverAbsentInvalidJSONAndNonRegularPaths(t *testing.T) {
	t.Parallel()

	driver := Driver{}
	root := t.TempDir()
	req := request(root, Selector{Path: []string{"user", "email"}})

	detection, err := driver.Detect(req)
	require.NoError(t, err)
	require.False(t, detection.Exists)
	state, err := driver.ReadCurrent(req)
	require.NoError(t, err)
	require.False(t, state.Exists)

	missingPathRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(missingPathRoot, "config.json"), []byte(`{"user":{"name":"Leon"}}`), 0o644))
	state, err = driver.ReadCurrent(request(missingPathRoot, Selector{Path: []string{"user", "email"}}))
	require.NoError(t, err)
	require.False(t, state.Exists)

	invalidRoot := t.TempDir()
	invalidPath := filepath.Join(invalidRoot, "config.json")
	require.NoError(t, os.WriteFile(invalidPath, []byte(`{"user":`), 0o644))
	_, err = driver.ReadCurrent(request(invalidRoot, Selector{Path: []string{"user"}}))
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
	requireFile(t, invalidPath, `{"user":`)

	emptyRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(emptyRoot, "config.json"), nil, 0o644))
	_, err = driver.PreviewApply(request(emptyRoot, Selector{Path: []string{"user"}, CreateMissing: CreatePolicyCreate}), mustNormalize(t, driver, `"value"`))
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())

	dir := filepath.Join(root, "dir")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	dirReq := Request{Target: filedriver.Target{LocationID: "config", Root: root, RelPath: "dir"}, Selector: Selector{Path: []string{"user"}}}
	_, err = driver.Detect(dirReq)
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
	_, err = driver.ReadCurrent(dirReq)
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
}

func TestBackupRestoreHooksAndVerificationErrorsAreExplicit(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "config.json")
	before := `{"user":{"email":"old@example.com"},"secret":"keep-me"}`
	require.NoError(t, os.WriteFile(path, []byte(before), 0o644))
	driver := Driver{}
	req := request(root, Selector{Path: []string{"user", "email"}})

	called := false
	backup, err := driver.Backup(req, func(backupReq BackupRequest) (BackupResult, error) {
		called = true
		require.True(t, backupReq.Before.Exists)
		require.Equal(t, []byte(`"old@example.com"`), backupReq.Before.Value)
		require.Equal(t, before, string(backupReq.BeforeFile))
		encoded, err := json.Marshal(backupReq)
		require.NoError(t, err)
		require.NotContains(t, string(encoded), "old@example.com")
		require.NotContains(t, string(encoded), "keep-me")
		return BackupResult{ID: "memory://backup/config", Before: backupReq.Before.Snapshot()}, nil
	})
	require.NoError(t, err)
	require.True(t, called)
	require.Equal(t, "memory://backup/config", backup.ID)

	applied, appliedBackup, err := driver.ApplyWithBackup(req, mustNormalize(t, driver, `"new@example.com"`), func(backupReq BackupRequest) (BackupResult, error) {
		return BackupResult{ID: "memory://backup/apply", Before: backupReq.Before.Snapshot()}, nil
	})
	require.NoError(t, err)
	require.True(t, applied.Mutated)
	require.NotNil(t, appliedBackup)
	require.Equal(t, "memory://backup/apply", appliedBackup.ID)

	err = driver.Restore(req, backup, nil)
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsupported), err.Error())

	restoreCalled := false
	err = driver.Restore(req, backup, func(restoreReq RestoreRequest) error {
		restoreCalled = true
		require.Equal(t, backup.ID, restoreReq.Backup.ID)
		return nil
	})
	require.NoError(t, err)
	require.True(t, restoreCalled)

	verify, err := driver.Verify(req, mustNormalize(t, driver, `"different@example.com"`))
	require.Error(t, err)
	require.False(t, verify.Verified)
	require.True(t, filedriver.IsCode(err, filedriver.CodeVerificationFailed), err.Error())

	_, _, err = driver.ApplyWithBackup(req, mustNormalize(t, driver, `"yet-another@example.com"`), func(backupReq BackupRequest) (BackupResult, error) {
		return BackupResult{}, fmt.Errorf("backup failed")
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "backup failed")
}

func TestDriverRejectsUnsafePathsAndSymlinkEscapes(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "config.json"), []byte(`{"user":{"email":"secret@example.com"}}`), 0o644))
	driver := Driver{}

	_, err := driver.ReadCurrent(Request{Target: filedriver.Target{LocationID: "config", Root: root, RelPath: "../config.json"}, Selector: Selector{Path: []string{"user", "email"}}})
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())

	require.NoError(t, os.Symlink(outside, filepath.Join(root, "outside-link")))
	_, err = driver.ReadCurrent(Request{Target: filedriver.Target{LocationID: "config", Root: root, RelPath: "outside-link/config.json"}, Selector: Selector{Path: []string{"user", "email"}}})
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())

	inside := filepath.Join(root, "inside")
	require.NoError(t, os.MkdirAll(inside, 0o755))
	require.NoError(t, os.Symlink("inside", filepath.Join(root, "inside-link")))
	insideReq := Request{Target: filedriver.Target{LocationID: "config", Root: root, RelPath: "inside-link/config.json"}, Selector: Selector{Path: []string{"user", "email"}, CreateMissing: CreatePolicyCreate}}
	_, err = driver.Apply(insideReq, mustNormalize(t, driver, `"leon@example.com"`))
	require.NoError(t, err)
	requireJSONFile(t, filepath.Join(inside, "config.json"), `{
  "user": {
    "email": "leon@example.com"
  }
}
`)
}

func TestDriverAdditionalErrorBranches(t *testing.T) {
	t.Parallel()

	driver := Driver{}
	root := t.TempDir()
	req := request(root, Selector{Path: []string{"user", "email"}})

	_, err := driver.Detect(Request{Target: filedriver.Target{LocationID: "config", Root: filepath.Join(root, "missing-root"), RelPath: "config.json"}, Selector: Selector{Path: []string{"user", "email"}}})
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeNotFound), err.Error())

	_, err = driver.PreviewApply(req, AbsentState())
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
	_, err = driver.PreviewApply(req, State{Exists: true, Value: []byte(`"safe"`), Intent: IntentDelete})
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
	_, err = driver.Verify(req, AbsentState())
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())

	err = driver.Restore(Request{Target: filedriver.Target{LocationID: "config", Root: root, RelPath: "../bad"}, Selector: Selector{Path: []string{"user", "email"}}}, BackupResult{}, func(req RestoreRequest) error { return nil })
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())
	err = driver.Restore(req, BackupResult{ID: "backup"}, func(req RestoreRequest) error { return fmt.Errorf("restore failed") })
	require.Error(t, err)
	require.Contains(t, err.Error(), "restore failed")

	_, err = driver.Normalize([]byte(`invalid`))
	require.Error(t, err)
	_, err = driver.Normalize([]byte(`true false`))
	require.Error(t, err)
}

func request(root string, selector Selector) Request {
	return Request{Target: filedriver.Target{LocationID: "config", Root: root, RelPath: "config.json"}, Selector: selector}
}

func mustNormalize(t *testing.T, driver Driver, raw string) State {
	t.Helper()
	state, err := driver.Normalize([]byte(raw))
	require.NoError(t, err)
	return state
}

func requireFile(t *testing.T, path string, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, want, string(got))
}

func requireJSONFile(t *testing.T, path string, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, want, string(got))
}

func TestDriverNoOpHelpersMissingDeleteAndFinalSymlink(t *testing.T) {
	t.Parallel()

	driver := Driver{}

	valueState, err := driver.NormalizeValue("same@example.com")
	require.NoError(t, err)
	require.Equal(t, []byte(`"same@example.com"`), valueState.Value)
	boolState, err := driver.NormalizeValue(true)
	require.NoError(t, err)
	require.Equal(t, []byte(`true`), boolState.Value)
	nullState, err := driver.NormalizeValue(nil)
	require.NoError(t, err)
	require.Equal(t, []byte(`null`), nullState.Value)

	t.Run("unchanged selected scalar does not backup or mutate", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		path := filepath.Join(root, "config.json")
		before := `{"user":{"email":"same@example.com"}}`
		require.NoError(t, os.WriteFile(path, []byte(before), 0o644))
		req := request(root, Selector{Path: []string{"user", "email"}})

		preview, err := driver.PreviewApply(req, valueState)
		require.NoError(t, err)
		require.Equal(t, filedriver.ChangeUnchanged, preview.Change.Kind)

		applied, backup, err := driver.ApplyWithBackup(req, valueState, func(BackupRequest) (BackupResult, error) {
			require.FailNow(t, "backup must not run for unchanged apply")
			return BackupResult{}, nil
		})
		require.NoError(t, err)
		require.False(t, applied.Mutated)
		require.Nil(t, backup)
		requireFile(t, path, before)
	})

	t.Run("nil backup hook returns redacted noop metadata", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		path := filepath.Join(root, "config.json")
		require.NoError(t, os.WriteFile(path, []byte(`{"user":{"email":"same@example.com"}}`), 0o644))
		backup, err := driver.Backup(request(root, Selector{Path: []string{"user", "email"}}), nil)
		require.NoError(t, err)
		require.Equal(t, "noop", backup.ID)
		require.True(t, backup.Before.Exists)
	})

	t.Run("delete missing file requires allow and remains no-op", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		req := request(root, Selector{Path: []string{"user", "email"}, DeleteKey: DeletePolicyAllow})
		preview, err := driver.PreviewApply(req, DeleteState())
		require.NoError(t, err)
		require.Equal(t, filedriver.ChangeUnchanged, preview.Change.Kind)
		applied, err := driver.Apply(req, DeleteState())
		require.NoError(t, err)
		require.False(t, applied.Mutated)
		_, err = os.Stat(filepath.Join(root, "config.json"))
		require.True(t, os.IsNotExist(err))
	})

	t.Run("selected object delete is rejected as broad mutation", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		path := filepath.Join(root, "config.json")
		before := `{"user":{"email":"same@example.com"},"other":true}`
		require.NoError(t, os.WriteFile(path, []byte(before), 0o644))
		_, err := driver.PreviewApply(request(root, Selector{Path: []string{"user"}, DeleteKey: DeletePolicyAllow}), DeleteState())
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
		requireFile(t, path, before)
	})

	t.Run("final symlink inside root is written through safely", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		realPath := filepath.Join(root, "real.json")
		linkPath := filepath.Join(root, "link.json")
		require.NoError(t, os.WriteFile(realPath, []byte(`{"user":{"email":"old@example.com"}}`), 0o644))
		require.NoError(t, os.Symlink("real.json", linkPath))

		req := Request{Target: filedriver.Target{LocationID: "config", Root: root, RelPath: "link.json"}, Selector: Selector{Path: []string{"user", "email"}}}
		_, err := driver.Apply(req, mustNormalize(t, driver, `"same@example.com"`))
		require.NoError(t, err)
		requireJSONFile(t, realPath, `{
  "user": {
    "email": "same@example.com"
  }
}
`)
	})
}

func TestInternalHelperErrorBranches(t *testing.T) {
	t.Parallel()

	permissionErr := classifyOSError("op", "path", os.ErrPermission)
	require.True(t, filedriver.IsCode(permissionErr, filedriver.CodePermissionDenied), permissionErr.Error())
	notFoundErr := classifyOSError("op", "path", os.ErrNotExist)
	require.True(t, filedriver.IsCode(notFoundErr, filedriver.CodeNotFound), notFoundErr.Error())
	internalErr := classifyOSError("op", "path", fmt.Errorf("boom"))
	require.True(t, filedriver.IsCode(internalErr, filedriver.CodeInternal), internalErr.Error())

	driver := Driver{}
	_, err := driver.NormalizeValue(func() {})
	require.Error(t, err)
	_, err = canonicalScalar(json.Number("not-a-number"))
	require.Error(t, err)
	_, err = marshalDocument(map[string]any{"bad": func() {}})
	require.Error(t, err)
	_, err = parseDesiredScalar([]byte(`{"object":true}`))
	require.Error(t, err)

	_, err = setSelected(true, Selector{Path: []string{"user"}, CreateMissing: CreatePolicyCreate}, "value")
	require.Error(t, err)
	_, err = setSelected(map[string]any{"user": []any{}}, Selector{Path: []string{"user", "email"}, CreateMissing: CreatePolicyCreate}, "value")
	require.Error(t, err)
	_, err = deleteSelected(true, Selector{Path: []string{"user"}, DeleteKey: DeletePolicyAllow})
	require.Error(t, err)
	root, err := deleteSelected(map[string]any{"profile": map[string]any{"name": "Leon"}}, Selector{Path: []string{"user", "email"}, DeleteKey: DeletePolicyAllow})
	require.NoError(t, err)
	require.NotNil(t, root)
	root, err = deleteSelected(nil, Selector{Path: []string{"user"}, DeleteKey: DeletePolicyAllow})
	require.NoError(t, err)
	require.Nil(t, root)

	rootDir := t.TempDir()
	missingParent := filepath.Join(rootDir, "missing", "file.json")
	err = writeFileAtomic(missingParent, []byte(`{}`))
	require.Error(t, err)
	dirPath := filepath.Join(rootDir, "dir-target")
	require.NoError(t, os.MkdirAll(dirPath, 0o755))
	err = writeFileAtomic(dirPath, []byte(`{}`))
	require.Error(t, err)
}

func TestAdditionalPublicBranchCoverage(t *testing.T) {
	t.Parallel()

	driver := Driver{}
	root := t.TempDir()
	path := filepath.Join(root, "config.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"user":{"email":"old@example.com"}}`), 0o644))
	req := request(root, Selector{Path: []string{"user", "email"}})

	backup, err := driver.Backup(req, func(BackupRequest) (BackupResult, error) {
		return BackupResult{ID: "memory://backup/no-before"}, nil
	})
	require.NoError(t, err)
	require.Equal(t, "memory://backup/no-before", backup.ID)
	require.True(t, backup.Before.Exists)

	_, _, err = driver.ApplyWithBackup(req, mustNormalize(t, driver, `"new@example.com"`), func(BackupRequest) (BackupResult, error) {
		require.NoError(t, os.WriteFile(path, []byte(`{"user":`), 0o644))
		return BackupResult{ID: "memory://backup/corrupt"}, nil
	})
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())

	unsafeReq := Request{Target: filedriver.Target{LocationID: "config", Root: root, RelPath: "../bad.json"}, Selector: Selector{Path: []string{"user"}}}
	_, err = driver.Detect(unsafeReq)
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())
	_, err = driver.PreviewApply(unsafeReq, mustNormalize(t, driver, `"value"`))
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector) || filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())
	_, err = driver.Verify(unsafeReq, mustNormalize(t, driver, `"value"`))
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())
}

func TestAdditionalWriteTargetBranches(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dir := filepath.Join(root, "dir")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	err := writeTarget(filedriver.Target{LocationID: "config", Root: root, RelPath: "dir"}, []byte(`{}`))
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())

	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "outside.json")
	require.NoError(t, os.WriteFile(outsideFile, []byte(`{}`), 0o644))
	link := filepath.Join(root, "outside.json")
	require.NoError(t, os.Symlink(outsideFile, link))
	err = writeTarget(filedriver.Target{LocationID: "config", Root: root, RelPath: "outside.json"}, []byte(`{}`))
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())

	err = ensureInside(root, outsideFile)
	require.Error(t, err)
}

func TestAdditionalSelectorMutationHelperBranches(t *testing.T) {
	t.Parallel()

	_, err := selectScalar(map[string]any{"n": json.Number("not-a-number")}, Selector{Path: []string{"n"}})
	require.Error(t, err)
	_, err = setSelected(map[string]any{}, Selector{Path: []string{"user"}, CreateMissing: CreatePolicyCreate}, []any{"not-scalar"})
	require.Error(t, err)
	_, err = setSelected(nil, Selector{Path: []string{"user"}}, "value")
	require.Error(t, err)
	_, err = setSelected(map[string]any{}, Selector{Path: []string{"user", "email"}}, "value")
	require.Error(t, err)
	_, err = setSelected(map[string]any{"user": map[string]any{"email": map[string]any{"nested": true}}}, Selector{Path: []string{"user", "email"}, CreateMissing: CreatePolicyCreate}, "value")
	require.Error(t, err)
	_, err = deleteSelected(map[string]any{"user": []any{}}, Selector{Path: []string{"user", "email"}, DeleteKey: DeletePolicyAllow})
	require.Error(t, err)
	_, err = deleteSelected(map[string]any{"user": map[string]any{"email": map[string]any{"nested": true}}}, Selector{Path: []string{"user", "email"}, DeleteKey: DeletePolicyAllow})
	require.Error(t, err)
	_, err = parseDesiredScalar([]byte(`invalid`))
	require.Error(t, err)
}
