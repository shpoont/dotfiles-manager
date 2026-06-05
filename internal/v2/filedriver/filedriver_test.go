package filedriver

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDriverImplementsReadNormalizeDiffPreviewApplyVerify(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := Target{LocationID: "config", Root: root, RelPath: "config.txt"}
	driver := Driver{}

	detection, err := driver.Detect(target)
	require.NoError(t, err)
	require.False(t, detection.Exists)

	desired := driver.Normalize([]byte("hello\n"))
	preview, err := driver.PreviewApply(target, desired)
	require.NoError(t, err)
	require.Equal(t, ChangeCreate, preview.Change.Kind)
	assertMissing(t, filepath.Join(root, "config.txt"))

	applied, err := driver.Apply(target, desired)
	require.NoError(t, err)
	require.True(t, applied.Mutated)
	verify, err := driver.Verify(target, desired)
	require.NoError(t, err)
	require.True(t, verify.Verified)
	requireFile(t, filepath.Join(root, "config.txt"), "hello\n")

	current, err := driver.ReadCurrent(target)
	require.NoError(t, err)
	require.Equal(t, desired.SHA256, current.SHA256)
	require.Equal(t, ChangeUnchanged, driver.Diff(current, desired).Kind)

	updated := driver.Normalize([]byte("updated\n"))
	preview, err = driver.PreviewApply(target, updated)
	require.NoError(t, err)
	require.Equal(t, ChangeUpdate, preview.Change.Kind)
	requireFile(t, filepath.Join(root, "config.txt"), "hello\n")

	_, err = driver.Apply(target, updated)
	require.NoError(t, err)
	verify, err = driver.Verify(target, updated)
	require.NoError(t, err)
	require.True(t, verify.Verified)
	requireFile(t, filepath.Join(root, "config.txt"), "updated\n")

	preview, err = driver.PreviewApply(target, AbsentState())
	require.NoError(t, err)
	require.Equal(t, ChangeDelete, preview.Change.Kind)
	requireFile(t, filepath.Join(root, "config.txt"), "updated\n")

	_, err = driver.Apply(target, AbsentState())
	require.NoError(t, err)
	verify, err = driver.Verify(target, AbsentState())
	require.NoError(t, err)
	require.True(t, verify.Verified)
	assertMissing(t, filepath.Join(root, "config.txt"))
}

func TestBackupAndRestoreHooksAreExplicit(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "config.txt")
	require.NoError(t, os.WriteFile(path, []byte("before"), 0o644))
	target := Target{LocationID: "config", Root: root, RelPath: "config.txt"}
	driver := Driver{}

	called := false
	backup, err := driver.Backup(target, func(req BackupRequest) (BackupResult, error) {
		called = true
		require.True(t, req.Before.Exists)
		require.Equal(t, "before", string(req.Before.Bytes))
		return BackupResult{ID: "memory://backup/config", Before: req.Before.Snapshot()}, nil
	})
	require.NoError(t, err)
	require.True(t, called)
	require.Equal(t, "memory://backup/config", backup.ID)

	err = driver.Restore(target, backup, nil)
	require.Error(t, err)
	require.True(t, IsCode(err, CodeUnsupported))

	restoreCalled := false
	err = driver.Restore(target, backup, func(req RestoreRequest) error {
		restoreCalled = true
		require.Equal(t, backup.ID, req.Backup.ID)
		return nil
	})
	require.NoError(t, err)
	require.True(t, restoreCalled)
}

func TestDriverRejectsTraversalAndUnsafeSymlinkEscape(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "secret.txt")
	require.NoError(t, os.WriteFile(outsideFile, []byte("secret"), 0o644))
	driver := Driver{}

	badPaths := []string{"../escape.txt", "/tmp/escape.txt", `nested\\escape.txt`, "nested/./escape.txt", "nested//escape.txt"}
	for _, rel := range badPaths {
		t.Run(rel, func(t *testing.T) {
			t.Parallel()

			_, err := driver.ReadCurrent(Target{LocationID: "config", Root: root, RelPath: rel})
			require.Error(t, err)
			require.True(t, IsCode(err, CodeUnsafePath), err.Error())
		})
	}

	require.NoError(t, os.Symlink(outside, filepath.Join(root, "outside-link")))
	_, err := driver.ReadCurrent(Target{LocationID: "config", Root: root, RelPath: "outside-link/secret.txt"})
	require.Error(t, err)
	require.True(t, IsCode(err, CodeUnsafePath), err.Error())

	require.NoError(t, os.Symlink(outsideFile, filepath.Join(root, "file-link.txt")))
	_, err = driver.ReadCurrent(Target{LocationID: "config", Root: root, RelPath: "file-link.txt"})
	require.Error(t, err)
	require.True(t, IsCode(err, CodeUnsafePath), err.Error())
}

func requireFile(t *testing.T, path string, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, want, string(got))
}

func assertMissing(t *testing.T, path string) {
	t.Helper()
	_, err := os.Stat(path)
	require.True(t, os.IsNotExist(err), "expected %s to be missing, got %v", path, err)
}

func TestDriverErrorAndDetectionBranches(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	file := filepath.Join(root, "config.txt")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o644))
	driver := Driver{}

	detection, err := driver.Detect(Target{LocationID: "config", Root: root, RelPath: "config.txt"})
	require.NoError(t, err)
	require.True(t, detection.Exists)
	require.True(t, detection.Readable)

	_, err = driver.Detect(Target{LocationID: "config", Root: root, RelPath: "missing.txt"})
	require.NoError(t, err)

	_, err = driver.Detect(Target{LocationID: "config", Root: root, RelPath: "../bad"})
	require.Error(t, err)
	require.True(t, IsCode(err, CodeUnsafePath))

	_, err = driver.Detect(Target{LocationID: "config", Root: filepath.Join(root, "missing-root"), RelPath: "config.txt"})
	require.Error(t, err)
	require.True(t, IsCode(err, CodeNotFound))

	_, err = driver.Detect(Target{LocationID: "config", Root: file, RelPath: "config.txt"})
	require.Error(t, err)
	require.True(t, IsCode(err, CodeInvalidSelector))

	dir := filepath.Join(root, "dir")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	_, err = driver.Detect(Target{LocationID: "config", Root: root, RelPath: "dir"})
	require.Error(t, err)
	require.True(t, IsCode(err, CodeInvalidSelector))

	rootLink := filepath.Join(t.TempDir(), "root-link")
	require.NoError(t, os.Symlink(root, rootLink))
	_, err = ResolveTarget(Target{LocationID: "config", Root: rootLink, RelPath: "config.txt", RejectRootSymlink: true})
	require.Error(t, err)
	require.True(t, IsCode(err, CodeUnsafePath))

	typed := &Error{Code: CodeUnsafePath, Op: "op", Path: "path", Err: os.ErrPermission}
	require.Contains(t, typed.Error(), "unsafe-path")
	require.ErrorIs(t, typed.Unwrap(), os.ErrPermission)
	require.True(t, IsCode(fmt.Errorf("wrapped: %w", typed), CodeUnsafePath))
	require.False(t, IsCode(fmt.Errorf("plain"), CodeUnsafePath))
}

func TestDriverApplyBackupAndVerificationErrorBranches(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := Target{LocationID: "config", Root: root, RelPath: "config.txt"}
	driver := Driver{}
	desired := driver.Normalize([]byte("same"))
	require.NoError(t, os.WriteFile(filepath.Join(root, "config.txt"), desired.Bytes, 0o644))

	applied, backup, err := driver.ApplyWithBackup(target, desired, func(req BackupRequest) (BackupResult, error) {
		t.Fatalf("backup hook must not be called for unchanged apply")
		return BackupResult{}, nil
	})
	require.NoError(t, err)
	require.False(t, applied.Mutated)
	require.Nil(t, backup)

	_, _, err = driver.ApplyWithBackup(target, driver.Normalize([]byte("new")), func(req BackupRequest) (BackupResult, error) {
		return BackupResult{}, fmt.Errorf("backup failed")
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "backup failed")
	requireFile(t, filepath.Join(root, "config.txt"), "same")

	verify, err := driver.Verify(target, driver.Normalize([]byte("different")))
	require.Error(t, err)
	require.False(t, verify.Verified)
	require.True(t, IsCode(err, CodeVerificationFailed))

	err = driver.Restore(target, BackupResult{ID: "backup"}, func(req RestoreRequest) error {
		return fmt.Errorf("restore failed")
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "restore failed")
}

func TestDriverPathCreationAndSymlinkBranches(t *testing.T) {
	t.Parallel()

	driver := Driver{}
	missingRoot := filepath.Join(t.TempDir(), "missing-root")
	createdTarget := Target{LocationID: "desired", Root: missingRoot, RelPath: "config.txt", AllowMissingRoot: true}
	_, err := driver.Apply(createdTarget, driver.Normalize([]byte("created")))
	require.NoError(t, err)
	requireFile(t, filepath.Join(missingRoot, "config.txt"), "created")

	root := t.TempDir()
	inside := filepath.Join(root, "inside")
	require.NoError(t, os.MkdirAll(inside, 0o755))
	require.NoError(t, os.Symlink("inside", filepath.Join(root, "link-dir")))
	_, err = driver.Apply(Target{LocationID: "config", Root: root, RelPath: "link-dir/config.txt"}, driver.Normalize([]byte("via ancestor symlink")))
	require.NoError(t, err)
	requireFile(t, filepath.Join(inside, "config.txt"), "via ancestor symlink")

	realFile := filepath.Join(root, "real.txt")
	require.NoError(t, os.WriteFile(realFile, []byte("old"), 0o644))
	require.NoError(t, os.Symlink("real.txt", filepath.Join(root, "file-link-inside.txt")))
	_, err = driver.Apply(Target{LocationID: "config", Root: root, RelPath: "file-link-inside.txt"}, driver.Normalize([]byte("through file symlink")))
	require.NoError(t, err)
	requireFile(t, realFile, "through file symlink")

	parentFile := filepath.Join(root, "parent-file")
	require.NoError(t, os.WriteFile(parentFile, []byte("x"), 0o644))
	err = writeTarget(Target{LocationID: "config", Root: root, RelPath: "parent-file/child.txt"}, []byte("x"))
	require.Error(t, err)
	require.True(t, IsCode(err, CodeInvalidSelector))

	err = removeTarget(Target{LocationID: "config", Root: root, RelPath: "missing-delete.txt"})
	require.NoError(t, err)
	err = removeTarget(Target{LocationID: "config", Root: root, RelPath: "inside"})
	require.Error(t, err)
	require.True(t, IsCode(err, CodeInvalidSelector))
}

func TestDriverInternalErrorClassificationBranches(t *testing.T) {
	t.Parallel()

	require.True(t, IsCode(classifyOSError("op", "path", os.ErrPermission), CodePermissionDenied))
	require.True(t, IsCode(classifyOSError("op", "path", os.ErrNotExist), CodeNotFound))
	require.True(t, IsCode(classifyOSError("op", "path", fmt.Errorf("boom")), CodeInternal))

	require.Error(t, ensureLexicallyInside("/tmp/base", "/tmp/elsewhere"))
	require.Error(t, ensureRealInside("/tmp/base", "/tmp/elsewhere"))
}

func TestDriverAdditionalErrorBranches(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	driver := Driver{}

	_, err := ResolveTarget(Target{LocationID: "config", Root: "", RelPath: "config.txt"})
	require.Error(t, err)
	require.True(t, IsCode(err, CodeInvalidSelector))

	_, err = driver.ReadCurrent(Target{LocationID: "config", Root: root, RelPath: "../bad"})
	require.Error(t, err)
	require.True(t, IsCode(err, CodeUnsafePath))

	dir := filepath.Join(root, "dir")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	_, err = driver.ReadCurrent(Target{LocationID: "config", Root: root, RelPath: "dir"})
	require.Error(t, err)
	require.True(t, IsCode(err, CodeInvalidSelector))

	_, err = driver.PreviewApply(Target{LocationID: "config", Root: root, RelPath: "../bad"}, driver.Normalize([]byte("x")))
	require.Error(t, err)
	require.True(t, IsCode(err, CodeUnsafePath))
	_, err = driver.PreviewApply(Target{LocationID: "config", Root: root, RelPath: "dir"}, driver.Normalize([]byte("x")))
	require.Error(t, err)
	require.True(t, IsCode(err, CodeInvalidSelector))

	_, err = driver.Backup(Target{LocationID: "config", Root: root, RelPath: "../bad"}, nil)
	require.Error(t, err)
	require.True(t, IsCode(err, CodeUnsafePath))
	_, err = driver.Backup(Target{LocationID: "config", Root: root, RelPath: "dir"}, nil)
	require.Error(t, err)
	require.True(t, IsCode(err, CodeInvalidSelector))

	_, _, err = driver.ApplyWithBackup(Target{LocationID: "config", Root: root, RelPath: "../bad"}, driver.Normalize([]byte("x")), nil)
	require.Error(t, err)
	require.True(t, IsCode(err, CodeUnsafePath))
	_, _, err = driver.ApplyWithBackup(Target{LocationID: "config", Root: root, RelPath: "dir"}, driver.Normalize([]byte("x")), nil)
	require.Error(t, err)
	require.True(t, IsCode(err, CodeInvalidSelector))
	_, _, err = driver.ApplyWithBackup(Target{LocationID: "config", Root: root, RelPath: "dir"}, AbsentState(), nil)
	require.Error(t, err)
	require.True(t, IsCode(err, CodeInvalidSelector))

	_, err = driver.Verify(Target{LocationID: "config", Root: root, RelPath: "dir"}, AbsentState())
	require.Error(t, err)
	require.True(t, IsCode(err, CodeInvalidSelector))
	err = driver.Restore(Target{LocationID: "config", Root: root, RelPath: "../bad"}, BackupResult{}, func(req RestoreRequest) error { return nil })
	require.Error(t, err)
	require.True(t, IsCode(err, CodeUnsafePath))

	err = writeTarget(Target{LocationID: "config", Root: root, RelPath: "dir"}, []byte("x"))
	require.Error(t, err)
	require.True(t, IsCode(err, CodeInvalidSelector))
	err = writeFileAtomic(filepath.Join(root, "missing-parent", "file.txt"), []byte("x"))
	require.Error(t, err)

	broken := filepath.Join(root, "broken")
	require.NoError(t, os.Symlink("missing-target", broken))
	_, err = ResolveTarget(Target{LocationID: "config", Root: root, RelPath: "broken/file.txt"})
	require.Error(t, err)
	require.True(t, IsCode(err, CodeInternal) || IsCode(err, CodeNotFound), err.Error())

	for _, rel := range []string{"", " space", "/absolute", "a/../b"} {
		_, err := ValidateRelativePath(rel)
		require.Error(t, err)
	}
}
