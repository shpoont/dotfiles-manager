package inidriver

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/shpoont/dotfiles-manager/internal/v2/filedriver"
	"github.com/stretchr/testify/require"
)

func TestDriverReadNormalizePreviewApplyAndVerifySelectedKey(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "config.ini")
	before := "# keep top\n" +
		"\n" +
		"[user]\n" +
		"\tname = Leon\n" +
		"    email=old@example.com\n" +
		"\n" +
		"[alias]\n" +
		"\tco = checkout\n" +
		"[credential \"https://example.com\"]\n" +
		"\thelper = supersecret-helper\n"
	require.NoError(t, os.WriteFile(path, []byte(before), 0o644))

	driver := Driver{}
	req := request(root, Selector{Section: "user", Key: "email"})

	detection, err := driver.Detect(req)
	require.NoError(t, err)
	require.True(t, detection.Exists)
	require.True(t, detection.Readable)

	current, err := driver.ReadCurrent(req)
	require.NoError(t, err)
	require.True(t, current.Exists)
	require.Equal(t, "old@example.com", current.Value)
	require.Equal(t, driver.Normalize("  old@example.com  ").SHA256, current.SHA256)
	require.Equal(t, filedriver.ChangeUnchanged, driver.Diff(current, driver.Normalize("old@example.com")).Kind)

	desired := driver.Normalize("new@example.com")
	preview, err := driver.PreviewApply(req, desired)
	require.NoError(t, err)
	require.Equal(t, filedriver.ChangeUpdate, preview.Change.Kind)
	require.Equal(t, IntentSet, preview.Intent)
	requireFile(t, path, before)

	encodedPreview, err := json.Marshal(preview)
	require.NoError(t, err)
	require.NotContains(t, string(encodedPreview), "old@example.com")
	require.NotContains(t, string(encodedPreview), "new@example.com")
	require.NotContains(t, string(encodedPreview), "checkout")
	require.NotContains(t, string(encodedPreview), "supersecret-helper")

	applied, err := driver.Apply(req, desired)
	require.NoError(t, err)
	require.True(t, applied.Mutated)

	after := "# keep top\n" +
		"\n" +
		"[user]\n" +
		"\tname = Leon\n" +
		"\temail = new@example.com\n" +
		"\n" +
		"[alias]\n" +
		"\tco = checkout\n" +
		"[credential \"https://example.com\"]\n" +
		"\thelper = supersecret-helper\n"
	requireFile(t, path, after)

	verify, err := driver.Verify(req, desired)
	require.NoError(t, err)
	require.True(t, verify.Verified)
	require.Equal(t, filedriver.ChangeUnchanged, verify.Change.Kind)
}

func TestDriverCreatesMissingKeyAndSectionOnlyWhenPoliciesAllow(t *testing.T) {
	t.Parallel()

	driver := Driver{}

	t.Run("missing key requires explicit missingKey create", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		path := filepath.Join(root, "config.ini")
		require.NoError(t, os.WriteFile(path, []byte("[user]\n\tname = Leon\n[alias]\n\tco = checkout\n"), 0o644))

		_, err := driver.PreviewApply(request(root, Selector{Section: "user", Key: "email"}), driver.Normalize("leon@example.com"))
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
		requireFile(t, path, "[user]\n\tname = Leon\n[alias]\n\tco = checkout\n")

		req := request(root, Selector{Section: "user", Key: "email", MissingKey: MissingPolicyCreate})
		preview, err := driver.PreviewApply(req, driver.Normalize("leon@example.com"))
		require.NoError(t, err)
		require.Equal(t, filedriver.ChangeCreate, preview.Change.Kind)
		requireFile(t, path, "[user]\n\tname = Leon\n[alias]\n\tco = checkout\n")

		applied, err := driver.Apply(req, driver.Normalize("leon@example.com"))
		require.NoError(t, err)
		require.True(t, applied.Mutated)
		requireFile(t, path, "[user]\n\tname = Leon\n\temail = leon@example.com\n[alias]\n\tco = checkout\n")
	})

	t.Run("missing section and file require explicit missingSection and missingKey create", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		path := filepath.Join(root, "config.ini")
		require.NoError(t, os.WriteFile(path, []byte("# existing\n[alias]\n\tco = checkout\n"), 0o644))

		sectionOnly := request(root, Selector{Section: "user", Key: "email", MissingSection: MissingPolicyCreate})
		_, err := driver.PreviewApply(sectionOnly, driver.Normalize("leon@example.com"))
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())

		req := request(root, Selector{Section: "user", Key: "email", MissingSection: MissingPolicyCreate, MissingKey: MissingPolicyCreate})
		_, err = driver.Apply(req, driver.Normalize("leon@example.com"))
		require.NoError(t, err)
		requireFile(t, path, "# existing\n[alias]\n\tco = checkout\n\n[user]\n\temail = leon@example.com\n")

		missingRoot := filepath.Join(root, "missing-root")
		missingReq := Request{Target: filedriver.Target{LocationID: "config", Root: missingRoot, RelPath: "config.ini", AllowMissingRoot: true}, Selector: Selector{Section: "user", Key: "email", MissingSection: MissingPolicyCreate, MissingKey: MissingPolicyCreate}}
		_, err = driver.Apply(missingReq, driver.Normalize("leon@example.com"))
		require.NoError(t, err)
		requireFile(t, filepath.Join(missingRoot, "config.ini"), "[user]\n\temail = leon@example.com\n")
	})
}

func TestDriverDeletesSelectedKeyOnlyWhenPolicyAllows(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "config.ini")
	before := "[user]\n\tname = Leon\n\temail = old@example.com\n; keep comment\n[alias]\n\tco = checkout\n"
	require.NoError(t, os.WriteFile(path, []byte(before), 0o644))
	driver := Driver{}

	defaultReq := request(root, Selector{Section: "user", Key: "email"})
	_, err := driver.PreviewApply(defaultReq, DeleteState())
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
	requireFile(t, path, before)

	req := request(root, Selector{Section: "user", Key: "email", DeleteKey: DeletePolicyAllow})
	preview, err := driver.PreviewApply(req, DeleteState())
	require.NoError(t, err)
	require.Equal(t, filedriver.ChangeDelete, preview.Change.Kind)
	require.Equal(t, IntentDelete, preview.Intent)
	requireFile(t, path, before)

	applied, err := driver.Apply(req, DeleteState())
	require.NoError(t, err)
	require.True(t, applied.Mutated)
	requireFile(t, path, "[user]\n\tname = Leon\n; keep comment\n[alias]\n\tco = checkout\n")

	applied, err = driver.Apply(req, DeleteState())
	require.NoError(t, err)
	require.False(t, applied.Mutated)
}

func TestDriverRejectsInvalidSelectorsAndAmbiguousSelectedEntries(t *testing.T) {
	t.Parallel()

	driver := Driver{}

	cases := []struct {
		name     string
		selector Selector
	}{
		{name: "blank section", selector: Selector{Section: "", Key: "email"}},
		{name: "trimmed section", selector: Selector{Section: " user", Key: "email"}},
		{name: "bracketed section", selector: Selector{Section: "[user]", Key: "email"}},
		{name: "blank key", selector: Selector{Section: "user", Key: ""}},
		{name: "key with equals", selector: Selector{Section: "user", Key: "user=email"}},
		{name: "unsupported missing section", selector: Selector{Section: "user", Key: "email", MissingSection: "append"}},
		{name: "unsupported missing key", selector: Selector{Section: "user", Key: "email", MissingKey: "append"}},
		{name: "unsupported duplicate", selector: Selector{Section: "user", Key: "email", DuplicatePolicy: "last"}},
		{name: "unsupported delete", selector: Selector{Section: "user", Key: "email", DeleteKey: "force"}},
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

	duplicateSectionRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(duplicateSectionRoot, "config.ini"), []byte("[user]\n\temail = one@example.com\n[user]\n\tname = Leon\n"), 0o644))
	_, err := driver.ReadCurrent(request(duplicateSectionRoot, Selector{Section: "user", Key: "email"}))
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())

	duplicateKeyRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(duplicateKeyRoot, "config.ini"), []byte("[user]\n\temail = one@example.com\n\temail = two@example.com\n"), 0o644))
	_, err = driver.ReadCurrent(request(duplicateKeyRoot, Selector{Section: "user", Key: "email"}))
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())

	malformedSelectedRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(malformedSelectedRoot, "config.ini"), []byte("[user] # inline comments are unsupported for selected section headers\n\temail = one@example.com\n"), 0o644))
	_, err = driver.ReadCurrent(request(malformedSelectedRoot, Selector{Section: "user", Key: "email"}))
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())

	unrelatedInlineSectionRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(unrelatedInlineSectionRoot, "config.ini"), []byte("[user]\n\temail = one@example.com\n[credential] # preserved unrelated unsupported syntax\n\temail = secret@example.com\n"), 0o644))
	state, err := driver.ReadCurrent(request(unrelatedInlineSectionRoot, Selector{Section: "user", Key: "email"}))
	require.NoError(t, err)
	require.Equal(t, "one@example.com", state.Value)
}

func TestBackupRestoreHooksAndVerificationErrorsAreExplicit(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "config.ini")
	before := "[user]\n\temail = old@example.com\n[credential]\n\thelper = supersecret\n"
	require.NoError(t, os.WriteFile(path, []byte(before), 0o644))
	driver := Driver{}
	req := request(root, Selector{Section: "user", Key: "email"})

	called := false
	backup, err := driver.Backup(req, func(backupReq BackupRequest) (BackupResult, error) {
		called = true
		require.True(t, backupReq.Before.Exists)
		require.Equal(t, "old@example.com", backupReq.Before.Value)
		require.Equal(t, before, string(backupReq.BeforeFile))
		return BackupResult{ID: "memory://backup/config", Before: backupReq.Before.Snapshot()}, nil
	})
	require.NoError(t, err)
	require.True(t, called)
	require.Equal(t, "memory://backup/config", backup.ID)

	applied, appliedBackup, err := driver.ApplyWithBackup(req, driver.Normalize("new@example.com"), func(backupReq BackupRequest) (BackupResult, error) {
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

	verify, err := driver.Verify(req, driver.Normalize("different@example.com"))
	require.Error(t, err)
	require.False(t, verify.Verified)
	require.True(t, filedriver.IsCode(err, filedriver.CodeVerificationFailed), err.Error())

	_, _, err = driver.ApplyWithBackup(req, driver.Normalize("yet-another@example.com"), func(backupReq BackupRequest) (BackupResult, error) {
		return BackupResult{}, fmt.Errorf("backup failed")
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "backup failed")
}

func TestDriverRejectsUnsafePathsAndSymlinkEscapes(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "config.ini"), []byte("[user]\n\temail = secret@example.com\n"), 0o644))
	driver := Driver{}

	_, err := driver.ReadCurrent(Request{Target: filedriver.Target{LocationID: "config", Root: root, RelPath: "../config.ini"}, Selector: Selector{Section: "user", Key: "email"}})
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())

	require.NoError(t, os.Symlink(outside, filepath.Join(root, "outside-link")))
	_, err = driver.ReadCurrent(Request{Target: filedriver.Target{LocationID: "config", Root: root, RelPath: "outside-link/config.ini"}, Selector: Selector{Section: "user", Key: "email"}})
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())

	inside := filepath.Join(root, "inside")
	require.NoError(t, os.MkdirAll(inside, 0o755))
	require.NoError(t, os.Symlink("inside", filepath.Join(root, "inside-link")))
	insideReq := Request{Target: filedriver.Target{LocationID: "config", Root: root, RelPath: "inside-link/config.ini"}, Selector: Selector{Section: "user", Key: "email", MissingSection: MissingPolicyCreate, MissingKey: MissingPolicyCreate}}
	_, err = driver.Apply(insideReq, driver.Normalize("leon@example.com"))
	require.NoError(t, err)
	requireFile(t, filepath.Join(inside, "config.ini"), "[user]\n\temail = leon@example.com\n")
}

func TestDriverAdditionalErrorBranches(t *testing.T) {
	t.Parallel()

	driver := Driver{}
	root := t.TempDir()
	req := request(root, Selector{Section: "user", Key: "email"})

	detection, err := driver.Detect(req)
	require.NoError(t, err)
	require.False(t, detection.Exists)

	_, err = driver.Detect(Request{Target: filedriver.Target{LocationID: "config", Root: filepath.Join(root, "missing-root"), RelPath: "config.ini"}, Selector: Selector{Section: "user", Key: "email"}})
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeNotFound), err.Error())

	dir := filepath.Join(root, "dir")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	dirReq := Request{Target: filedriver.Target{LocationID: "config", Root: root, RelPath: "dir"}, Selector: Selector{Section: "user", Key: "email"}}
	_, err = driver.Detect(dirReq)
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
	_, err = driver.ReadCurrent(dirReq)
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
	_, err = driver.Backup(dirReq, nil)
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())

	_, err = driver.PreviewApply(req, AbsentState())
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
	_, err = driver.PreviewApply(req, State{Exists: true, Value: "a\nb", Intent: IntentSet})
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
	_, err = driver.PreviewApply(req, driver.Normalize("safe@example.com\n[alias]\nco = injected"))
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
	_, err = driver.PreviewApply(req, driver.Normalize("safe@example.com\x00injected"))
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
	_, err = driver.Verify(req, AbsentState())
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())

	err = driver.Restore(Request{Target: filedriver.Target{LocationID: "config", Root: root, RelPath: "../bad"}, Selector: Selector{Section: "user", Key: "email"}}, BackupResult{}, func(req RestoreRequest) error { return nil })
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())
	err = driver.Restore(req, BackupResult{ID: "backup"}, func(req RestoreRequest) error { return fmt.Errorf("restore failed") })
	require.Error(t, err)
	require.Contains(t, err.Error(), "restore failed")
}

func request(root string, selector Selector) Request {
	return Request{Target: filedriver.Target{LocationID: "config", Root: root, RelPath: "config.ini"}, Selector: selector}
}

func requireFile(t *testing.T, path string, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, want, string(got))
}

func TestDriverLineEndingsEOFAndSymlinkBranches(t *testing.T) {
	t.Parallel()

	driver := Driver{}

	t.Run("updates preserve selected line CRLF", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		path := filepath.Join(root, "config.ini")
		require.NoError(t, os.WriteFile(path, []byte("[user]\r\n\temail = old@example.com\r\n"), 0o644))
		_, err := driver.Apply(request(root, Selector{Section: "user", Key: "email"}), driver.Normalize("new@example.com"))
		require.NoError(t, err)
		requireFile(t, path, "[user]\r\n\temail = new@example.com\r\n")
	})

	t.Run("selected key at EOF without newline uses default replacement newline", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		path := filepath.Join(root, "config.ini")
		require.NoError(t, os.WriteFile(path, []byte("[user]\n\temail = old@example.com"), 0o644))
		_, err := driver.Apply(request(root, Selector{Section: "user", Key: "email"}), driver.Normalize("new@example.com"))
		require.NoError(t, err)
		requireFile(t, path, "[user]\n\temail = new@example.com\n")
	})

	t.Run("missing key at EOF without newline preserves existing bytes and appends separator", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		path := filepath.Join(root, "config.ini")
		require.NoError(t, os.WriteFile(path, []byte("[user]\n\tname = Leon"), 0o644))
		req := request(root, Selector{Section: "user", Key: "email", MissingKey: MissingPolicyCreate})
		_, err := driver.Apply(req, driver.Normalize("leon@example.com"))
		require.NoError(t, err)
		requireFile(t, path, "[user]\n\tname = Leon\n\temail = leon@example.com\n")
	})

	t.Run("missing section after file without newline preserves existing bytes before append", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		path := filepath.Join(root, "config.ini")
		require.NoError(t, os.WriteFile(path, []byte("[alias]\n\tco = checkout"), 0o644))
		req := request(root, Selector{Section: "user", Key: "email", MissingSection: MissingPolicyCreate, MissingKey: MissingPolicyCreate})
		_, err := driver.Apply(req, driver.Normalize("leon@example.com"))
		require.NoError(t, err)
		requireFile(t, path, "[alias]\n\tco = checkout\n\n[user]\n\temail = leon@example.com\n")
	})

	t.Run("final symlink inside root is written through safely", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		real := filepath.Join(root, "real.ini")
		link := filepath.Join(root, "link.ini")
		require.NoError(t, os.WriteFile(real, []byte("[user]\n\temail = old@example.com\n"), 0o644))
		require.NoError(t, os.Symlink("real.ini", link))
		req := Request{Target: filedriver.Target{LocationID: "config", Root: root, RelPath: "link.ini"}, Selector: Selector{Section: "user", Key: "email"}}
		_, err := driver.Apply(req, driver.Normalize("new@example.com"))
		require.NoError(t, err)
		requireFile(t, real, "[user]\n\temail = new@example.com\n")
	})
}

func TestDriverInternalHelperBranches(t *testing.T) {
	t.Parallel()

	require.True(t, filedriver.IsCode(classifyOSError("op", "path", os.ErrPermission), filedriver.CodePermissionDenied))
	require.True(t, filedriver.IsCode(classifyOSError("op", "path", os.ErrNotExist), filedriver.CodeNotFound))
	require.True(t, filedriver.IsCode(classifyOSError("op", "path", fmt.Errorf("boom")), filedriver.CodeInternal))
	require.Error(t, ensureInside("/tmp/base", "/tmp/elsewhere"))

	root := t.TempDir()
	driver := Driver{}
	req := request(root, Selector{Section: "user", Key: "email", MissingSection: MissingPolicyCreate, MissingKey: MissingPolicyCreate})

	backup, err := driver.Backup(req, nil)
	require.NoError(t, err)
	require.Equal(t, "noop", backup.ID)
	require.False(t, backup.Before.Exists)

	applied, appliedBackup, err := driver.ApplyWithBackup(req, driver.Normalize("leon@example.com"), nil)
	require.NoError(t, err)
	require.True(t, applied.Mutated)
	require.NotNil(t, appliedBackup)
	require.Equal(t, "noop", appliedBackup.ID)

	applied, appliedBackup, err = driver.ApplyWithBackup(req, driver.Normalize("leon@example.com"), func(req BackupRequest) (BackupResult, error) {
		t.Fatalf("backup hook must not be called for unchanged apply")
		return BackupResult{}, nil
	})
	require.NoError(t, err)
	require.False(t, applied.Mutated)
	require.Nil(t, appliedBackup)

	dirReq := Request{Target: filedriver.Target{LocationID: "config", Root: root, RelPath: "dir-as-file"}, Selector: Selector{Section: "user", Key: "email"}}
	require.NoError(t, os.Mkdir(filepath.Join(root, "dir-as-file"), 0o755))
	err = writeTarget(dirReq.Target, []byte("x"))
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())

	err = writeFileAtomic(filepath.Join(root, "missing-parent", "config.ini"), []byte("x"))
	require.Error(t, err)

	name, boundary, malformed := parseSectionBoundary("[unterminated", "user")
	require.Empty(t, name)
	require.False(t, boundary)
	require.False(t, malformed)
	key, value, ok := parseAssignment("not an assignment")
	require.Empty(t, key)
	require.Empty(t, value)
	require.False(t, ok)
	require.Equal(t, "\temail = x\n", canonicalAssignment("", "email", "x"))

	unsafeReq := Request{Target: filedriver.Target{LocationID: "config", Root: root, RelPath: "../bad.ini"}, Selector: Selector{Section: "user", Key: "email"}}
	_, err = driver.PreviewApply(unsafeReq, driver.Normalize("x"))
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())
	_, err = driver.Backup(unsafeReq, nil)
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())
	_, err = driver.Verify(unsafeReq, driver.Normalize("x"))
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())
	_, err = driver.renderDesired(unsafeReq, driver.Normalize("x"))
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())
	err = writeTarget(unsafeReq.Target, []byte("x"))
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsafePath), err.Error())

	duplicateRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(duplicateRoot, "config.ini"), []byte("[user]\n\temail = one@example.com\n\temail = two@example.com\n"), 0o644))
	_, err = driver.renderDesired(request(duplicateRoot, Selector{Section: "user", Key: "email"}), driver.Normalize("three@example.com"))
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())

	_, err = readRawFile(filepath.Join(root, "dir-as-file"))
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
}
