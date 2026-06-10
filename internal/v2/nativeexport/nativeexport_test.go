package nativeexport

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/shpoont/dotfiles-manager/internal/v2/nativeops"
	"github.com/shpoont/dotfiles-manager/internal/v2/recipe"
	"github.com/shpoont/dotfiles-manager/internal/v2/resolution"
	"github.com/stretchr/testify/require"
)

func TestExportRunsReviewedNativeOperationIntoValidatedStaging(t *testing.T) {
	t.Parallel()

	rec := nativeExportTestRecipe(t)
	executor := &recordingExecutor{body: "opaque-payload-secret"}
	result, err := Export(context.Background(), nativeExportOptions(t, rec, executor))
	require.NoError(t, err)

	require.Equal(t, StatusSucceeded, result.Status)
	require.Equal(t, 1, executor.calls)
	require.DirExists(t, result.StagingRoot)
	require.FileExists(t, filepath.Join(result.StagingRoot, MetadataFile))
	require.Equal(t, "opaque-payload-secret", readFile(t, filepath.Join(result.PayloadRoot, "bundle.txt")))
	require.Equal(t, "native.app", result.Metadata.TargetRef)
	require.Equal(t, "native.app:settings", result.Metadata.SettingRef)
	require.Equal(t, "settings", result.Metadata.ResourceID)
	require.Equal(t, "export-settings", result.Metadata.OperationID)
	require.Equal(t, recipe.RecipeSourceBundled, result.Metadata.Recipe.Source)
	require.Equal(t, "trusted", result.Metadata.Recipe.TrustStatus)
	require.Equal(t, "native-export", result.Metadata.Operation.ArtifactForm)
	require.Equal(t, "metadata-only", result.Metadata.Operation.DiffMode)
	require.Equal(t, []string{"bundle"}, result.Metadata.Operation.OutputIDs)
	require.Equal(t, "user", result.Metadata.Source.Scope)
	require.Equal(t, "alice", result.Metadata.Source.Subject)
	require.Equal(t, "machine-1", result.Metadata.Source.MachineID)
	require.Equal(t, "user-1", result.Metadata.Source.UserID)
	require.Equal(t, "2026-06-09T12:00:00Z", result.Metadata.CapturedAt)
	require.True(t, result.Metadata.Payload.Exists)
	require.Equal(t, 1, result.Metadata.Payload.FileCount)
	require.Equal(t, "succeeded", result.Metadata.Native.Status)
	require.GreaterOrEqual(t, result.Metadata.Native.DurationMillis, int64(0))
	require.Equal(t, int64(5000), result.Metadata.Native.TimeoutMillis)
	require.Equal(t, "reviewed argv command: native-safe-tool", result.Metadata.Native.CommandSummary)
	require.True(t, result.Metadata.Exclusions.Declared)
	require.Equal(t, []string{"settings"}, result.Metadata.Exclusions.CapturedCategories)
	require.Equal(t, []string{"tokens"}, result.Metadata.Exclusions.SecretExclusions)
	require.Equal(t, []string{"sessions"}, result.Metadata.Exclusions.AccountExclusions)
	require.Equal(t, []string{"Internal app settings are not semantically compared"}, result.Metadata.Limitations)
	require.NotContains(t, result.Metadata.Native.CommandSummary, result.PayloadRoot)
}

func TestExportBlocksInvalidRunnerOutputAndPayloads(t *testing.T) {
	t.Parallel()

	t.Run("nil context still runs", func(t *testing.T) {
		rec := nativeExportTestRecipe(t)
		var nilContext context.Context
		result, err := Export(nilContext, nativeExportOptions(t, rec, &recordingExecutor{body: "payload"}))
		require.NoError(t, err)
		require.Equal(t, StatusSucceeded, result.Status)
	})

	t.Run("staging create failure", func(t *testing.T) {
		rec := nativeExportTestRecipe(t)
		opts := nativeExportOptions(t, rec, &recordingExecutor{body: "payload"})
		opts.StateRoot = filepath.Join(realTempDir(t), "state-file")
		writeFile(t, opts.StateRoot, "not a directory")
		result, err := Export(context.Background(), opts)
		require.Error(t, err)
		require.Equal(t, StatusBlocked, result.Status)
		require.Equal(t, "nativeexport.staging.create", result.Diagnostic.Code)
	})

	t.Run("invalid operation", func(t *testing.T) {
		rec := nativeExportTestRecipe(t)
		opts := nativeExportOptions(t, rec, &recordingExecutor{body: "payload"})
		opts.Resource.NativeOperation = "missing"
		result, err := Export(context.Background(), opts)
		require.Error(t, err)
		require.Equal(t, StatusBlocked, result.Status)
		require.Equal(t, "nativeexport.operation.invalid", result.Diagnostic.Code)
	})

	t.Run("native runner failure", func(t *testing.T) {
		rec := nativeExportTestRecipe(t)
		result, err := Export(context.Background(), nativeExportOptions(t, rec, &recordingExecutor{result: nativeops.ExecResult{ExitCode: 3, Err: errors.New("boom")}}))
		require.Error(t, err)
		require.Equal(t, StatusBlocked, result.Status)
		require.Equal(t, nativeops.CodeExecutionFailed, result.Diagnostic.Code)
	})

	t.Run("declared output missing", func(t *testing.T) {
		rec := nativeExportTestRecipe(t)
		result, err := Export(context.Background(), nativeExportOptions(t, rec, &recordingExecutor{skipWrite: true}))
		require.Error(t, err)
		require.Equal(t, StatusBlocked, result.Status)
		require.Equal(t, "nativeexport.outputs.missing", result.Diagnostic.Code)
	})

	t.Run("symlink output rejected", func(t *testing.T) {
		rec := nativeExportTestRecipe(t)
		result, err := Export(context.Background(), nativeExportOptions(t, rec, &recordingExecutor{writeSymlink: true}))
		require.Error(t, err)
		require.Equal(t, StatusBlocked, result.Status)
		require.Equal(t, "nativeexport.payload.unsupportedFileType", result.Diagnostic.Code)
	})

	t.Run("size limit rejected", func(t *testing.T) {
		rec := nativeExportTestRecipe(t)
		op := rec.NativeOperations["export-settings"]
		op.Limits.MaxBytes = 4
		rec.NativeOperations["export-settings"] = op
		result, err := Export(context.Background(), nativeExportOptions(t, rec, &recordingExecutor{body: "too-large"}))
		require.Error(t, err)
		require.Equal(t, StatusBlocked, result.Status)
		require.Equal(t, "nativeexport.payload.maxBytes", result.Diagnostic.Code)
	})

	t.Run("metadata write failure", func(t *testing.T) {
		rec := nativeExportTestRecipe(t)
		result, err := Export(context.Background(), nativeExportOptions(t, rec, &recordingExecutor{body: "payload", blockMetadataWrite: true}))
		require.Error(t, err)
		require.Equal(t, StatusBlocked, result.Status)
		require.Equal(t, "nativeexport.metadata.write", result.Diagnostic.Code)
	})
}

func TestReviewExpectedSnapshotAndChangeHelpers(t *testing.T) {
	t.Parallel()

	rec := nativeExportTestRecipe(t)
	op := rec.NativeOperations["export-settings"]
	require.True(t, ReviewRequired(op))
	review := ReviewDiagnostic("native.app:settings", op)
	require.Equal(t, ReviewCode, review.Code)
	require.Contains(t, review.Message, "Review native export before running")
	require.Contains(t, review.Message, "opaque, account-bound")

	limits := EffectiveLimits(op)
	require.Equal(t, int64(1024), limits.MaxBytes)
	require.Equal(t, 10, limits.MaxEntries)
	op.Limits = recipe.NativeExportLimits{}
	limits = EffectiveLimits(op)
	require.Equal(t, int64(recipe.MaxNativeExportBytes), limits.MaxBytes)
	require.Equal(t, recipe.MaxNativeExportEntries, limits.MaxEntries)

	opts := nativeExportOptions(t, rec, &recordingExecutor{body: "payload"})
	expected := Expected(opts)
	require.Equal(t, ExpectedIdentity{TargetRef: "native.app", SettingRef: "native.app:settings", ResourceID: "settings", OperationID: "export-settings", ArtifactForm: "native-export"}, expected)
	require.NoError(t, expected.Matches(Metadata{Schema: MetadataSchema, SchemaVersion: SchemaVersion, TargetRef: expected.TargetRef, SettingRef: expected.SettingRef, ResourceID: expected.ResourceID, OperationID: expected.OperationID, Operation: OperationMetadata{ArtifactForm: expected.ArtifactForm}}))
	require.ErrorContains(t, expected.Matches(Metadata{Schema: MetadataSchema, SchemaVersion: SchemaVersion, TargetRef: "other"}), "target")

	require.Equal(t, PayloadSummary{}, Snapshot(nil))
	payload := PayloadSummary{Exists: true, SHA256: "abc", Normalizer: Normalizer}
	require.Equal(t, payload, Snapshot(&Metadata{Payload: payload}))
	require.Equal(t, "create", ChangeKind(payload, PayloadSummary{}))
	require.Equal(t, "delete", ChangeKind(PayloadSummary{}, payload))
	require.Equal(t, "unchanged", ChangeKind(PayloadSummary{}, PayloadSummary{}))
	require.Equal(t, "unchanged", ChangeKind(payload, payload))
	require.Equal(t, "update", ChangeKind(payload, PayloadSummary{Exists: true, SHA256: "def", Normalizer: Normalizer}))
}

func TestIdentityReadDesiredAndDiagnosticHelpers(t *testing.T) {
	t.Parallel()

	expected := ExpectedIdentity{TargetRef: "native.app", SettingRef: "native.app:settings", ResourceID: "settings", OperationID: "export-settings", ArtifactForm: "native-export"}
	base := Metadata{Schema: MetadataSchema, SchemaVersion: SchemaVersion, TargetRef: expected.TargetRef, SettingRef: expected.SettingRef, ResourceID: expected.ResourceID, OperationID: expected.OperationID, Operation: OperationMetadata{ArtifactForm: expected.ArtifactForm}}
	require.NoError(t, expected.Matches(base))
	for _, tc := range []struct {
		name string
		edit func(*Metadata)
		want string
	}{
		{name: "schema", edit: func(m *Metadata) { m.Schema = "old" }, want: "schema"},
		{name: "version", edit: func(m *Metadata) { m.SchemaVersion = 99 }, want: "schema"},
		{name: "target", edit: func(m *Metadata) { m.TargetRef = "other" }, want: "target"},
		{name: "setting", edit: func(m *Metadata) { m.SettingRef = "native.app:other" }, want: "setting"},
		{name: "resource", edit: func(m *Metadata) { m.ResourceID = "other" }, want: "resource"},
		{name: "operation", edit: func(m *Metadata) { m.OperationID = "other" }, want: "operation"},
		{name: "artifact form", edit: func(m *Metadata) { m.Operation.ArtifactForm = "opaque" }, want: "artifact form"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metadata := base
			tc.edit(&metadata)
			require.ErrorContains(t, expected.Matches(metadata), tc.want)
		})
	}

	require.Equal(t, "invalid", ReadDesired("  ", expected).Status)
	missing := ReadDesired(filepath.Join(realTempDir(t), "missing"), expected)
	require.Equal(t, "missing", missing.Status)
	notDirParent := filepath.Join(realTempDir(t), "not-dir-parent")
	writeFile(t, notDirParent, "not a directory")
	readErr := ReadDesired(filepath.Join(notDirParent, "child"), expected)
	require.Equal(t, "blocked", readErr.Status)
	require.Equal(t, "nativeexport.desired.read", readErr.Diagnostic.Code)

	metadataPath := filepath.Join(realTempDir(t), "artifact", MetadataFile)
	require.NoError(t, os.MkdirAll(filepath.Dir(metadataPath), 0o755))
	require.NoError(t, os.Symlink("/tmp", metadataPath))
	blocked := ReadDesired(filepath.Dir(metadataPath), expected)
	require.Equal(t, "blocked", blocked.Status)
	require.Equal(t, "nativeexport.desired.metadataInvalid", blocked.Diagnostic.Code)

	require.Equal(t, Diagnostic{Code: "nativeops.failed", Message: "bad", Path: "op"}, firstNativeDiagnostic(nativeops.Result{Diagnostics: []nativeops.Diagnostic{{Code: "nativeops.failed", Message: "bad", OperationID: "op"}}}, "fallback"))
	require.Equal(t, "nativeexport.operation.failed", firstNativeDiagnostic(nativeops.Result{}, "fallback").Code)
	require.Equal(t, "nativeexport.payload.maxEntries", payloadDiagnostic(fmt.Errorf("maxEntries"), "ref").Code)
	require.Equal(t, "nativeexport.payload.unsupportedFileType", payloadDiagnostic(fmt.Errorf("unsupported file type"), "ref").Code)
	require.Equal(t, "nativeexport.payload.invalid", payloadDiagnostic(nil, "ref").Code)

	require.Contains(t, ReviewDiagnostic("ref", recipe.NativeOperation{}).Message, "explicit confirmation")
}

func TestInternalPathAndOperationGuards(t *testing.T) {
	t.Parallel()

	rec := nativeExportTestRecipe(t)
	opts := nativeExportOptions(t, rec, &recordingExecutor{body: "payload"})
	_, err := exportOperation(Options{})
	require.ErrorContains(t, err, "recipe is required")
	wrongDriver := opts
	wrongDriver.Resource.Driver = recipe.FileDriverID
	_, err = exportOperation(wrongDriver)
	require.ErrorContains(t, err, "resource driver")
	nonExport := nativeExportTestRecipe(t)
	op := nonExport.NativeOperations["export-settings"]
	op.Kind = "import"
	nonExport.NativeOperations["export-settings"] = op
	nonExportOpts := nativeExportOptions(t, nonExport, &recordingExecutor{body: "payload"})
	_, err = exportOperation(nonExportOpts)
	require.ErrorContains(t, err, "kind export")

	_, _, _, err = createStaging(Options{StateRoot: " "})
	require.ErrorContains(t, err, "path is required")
	stateRootFile := filepath.Join(realTempDir(t), "state-root-file")
	writeFile(t, stateRootFile, "not a directory")
	_, _, _, err = createStaging(Options{StateRoot: stateRootFile})
	require.Error(t, err)
	if runtime.GOOS != "windows" {
		stateRoot := realTempDir(t)
		nativeTempRoot := filepath.Join(stateRoot, "temp", "native-export")
		require.NoError(t, os.MkdirAll(nativeTempRoot, 0o755))
		require.NoError(t, os.Chmod(nativeTempRoot, 0o555))
		_, _, _, err = createStaging(Options{StateRoot: stateRoot})
		require.Error(t, err)
		require.NoError(t, os.Chmod(nativeTempRoot, 0o755))
	}
	_, err = cleanAbs(" ")
	require.ErrorContains(t, err, "path is required")
	require.Equal(t, "native-export", safePrefix("...///"))
	require.Len(t, safePrefix(strings.Repeat("a", 100)), 80)
	require.Equal(t, "fallback", defaultString(" ", "fallback"))

	root := realTempDir(t)
	require.ErrorContains(t, validateNoSymlinkParents(root, root), "escapes")
	require.ErrorContains(t, validateNoSymlinkParents(root, filepath.Join(filepath.Dir(root), "outside")), "escapes")
	realParent := filepath.Join(root, "real")
	linkParent := filepath.Join(root, "link")
	require.NoError(t, os.MkdirAll(realParent, 0o755))
	require.NoError(t, os.Symlink(realParent, linkParent))
	require.ErrorContains(t, validateNoSymlinkParents(root, filepath.Join(linkParent, "artifact")), "symlink")
	require.ErrorContains(t, validateNoSymlinkPath(filepath.Join(linkParent, "artifact")), "symlink")

	copySrc := realTempDir(t)
	require.NoError(t, os.Symlink("/tmp", filepath.Join(copySrc, "link")))
	require.ErrorContains(t, copyTree(copySrc, filepath.Join(realTempDir(t), "dst")), "symlink")
	copySrc = realTempDir(t)
	copyDst := filepath.Join(realTempDir(t), "dst")
	writeFile(t, filepath.Join(copySrc, "nested", "file.txt"), "copy-me")
	require.NoError(t, copyTree(copySrc, copyDst))
	require.Equal(t, "copy-me", readFile(t, filepath.Join(copyDst, "nested", "file.txt")))
	require.ErrorContains(t, copyTree(" ", filepath.Join(realTempDir(t), "dst")), "path is required")
	copySrc = realTempDir(t)
	writeFile(t, filepath.Join(copySrc, "file.txt"), "copy-me")
	blockingFile := filepath.Join(realTempDir(t), "blocking-file")
	writeFile(t, blockingFile, "not a directory")
	require.Error(t, copyTree(copySrc, blockingFile))
	copySrc = realTempDir(t)
	writeFile(t, filepath.Join(copySrc, "conflict.txt"), "copy-me")
	copyDst = filepath.Join(realTempDir(t), "dst-conflict")
	require.NoError(t, os.MkdirAll(filepath.Join(copyDst, "conflict.txt"), 0o755))
	require.Error(t, copyTree(copySrc, copyDst))

	atomicParentFile := filepath.Join(realTempDir(t), "parent-file")
	writeFile(t, atomicParentFile, "not a directory")
	require.Error(t, writeFileAtomic(filepath.Join(atomicParentFile, "metadata.json"), []byte("{}"), 0o644))
	if runtime.GOOS != "windows" {
		atomicParent := realTempDir(t)
		require.NoError(t, os.Chmod(atomicParent, 0o555))
		require.Error(t, writeFileAtomic(filepath.Join(atomicParent, "metadata.json"), []byte("{}"), 0o644))
		require.NoError(t, os.Chmod(atomicParent, 0o755))
	}

	require.ErrorContains(t, requireArtifactOutputs(realTempDir(t), recipe.NativeOperation{}), "at least one artifact output")
	require.ErrorContains(t, requireArtifactOutputs(realTempDir(t), recipe.NativeOperation{Outputs: map[string]recipe.NativePathSpec{"logs": {Root: "temp", Path: "log.txt"}}}), "at least one artifact output")
	require.ErrorContains(t, requireArtifactOutputs(realTempDir(t), recipe.NativeOperation{Outputs: map[string]recipe.NativePathSpec{"bundle": {Root: "artifact", Path: "missing.txt"}}}), "was not produced")
}

func TestSummarizePayloadUsesStableMetadataOnlyTreeHash(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "b.txt"), "b\n")
	writeFile(t, filepath.Join(root, "dir", "a.txt"), "a\n")

	first, err := SummarizePayload(root, Limits{MaxBytes: 1024, MaxEntries: 10})
	require.NoError(t, err)
	second, err := SummarizePayload(root, Limits{MaxBytes: 1024, MaxEntries: 10})
	require.NoError(t, err)

	require.True(t, first.Exists)
	require.Equal(t, int64(4), first.Size)
	require.Equal(t, 3, first.EntryCount)
	require.Equal(t, 2, first.FileCount)
	require.Equal(t, 1, first.DirCount)
	require.NotEmpty(t, first.SHA256)
	require.Equal(t, first, second)
}

func TestSummarizePayloadRejectsUnsafePayloadsAndLimits(t *testing.T) {
	t.Parallel()

	t.Run("missing root", func(t *testing.T) {
		_, err := SummarizePayload(filepath.Join(realTempDir(t), "missing"), Limits{MaxBytes: 1024, MaxEntries: 10})
		require.Error(t, err)
	})

	t.Run("blank root", func(t *testing.T) {
		_, err := SummarizePayload(" ", Limits{MaxBytes: 1024, MaxEntries: 10})
		require.ErrorContains(t, err, "path is required")
	})

	t.Run("directory max entries", func(t *testing.T) {
		root := realTempDir(t)
		require.NoError(t, os.Mkdir(filepath.Join(root, "dir"), 0o755))
		_, err := SummarizePayload(root, Limits{MaxBytes: 1024, MaxEntries: 0})
		require.Error(t, err)
		require.Contains(t, err.Error(), "maxEntries")
	})

	t.Run("symlink", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.Symlink("/tmp", filepath.Join(root, "link")))
		_, err := SummarizePayload(root, Limits{MaxBytes: 1024, MaxEntries: 10})
		require.Error(t, err)
		require.Contains(t, err.Error(), "symlink")
	})

	t.Run("max bytes", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, filepath.Join(root, "payload.bin"), "too-large")
		_, err := SummarizePayload(root, Limits{MaxBytes: 4, MaxEntries: 10})
		require.Error(t, err)
		require.Contains(t, err.Error(), "maxBytes")
	})

	t.Run("max entries", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, filepath.Join(root, "one"), "1")
		writeFile(t, filepath.Join(root, "two"), "2")
		_, err := SummarizePayload(root, Limits{MaxBytes: 1024, MaxEntries: 1})
		require.Error(t, err)
		require.Contains(t, err.Error(), "maxEntries")
	})
}

func TestWriteDesiredOnlyReplacesManagerOwnedNativeExportDirectory(t *testing.T) {
	t.Parallel()

	parent := realTempDir(t)
	desiredPath := filepath.Join(parent, "artifact")
	expected := ExpectedIdentity{TargetRef: "native.app", SettingRef: "native.app:settings", ResourceID: "settings", OperationID: "export-settings", ArtifactForm: "native-export"}
	staging := stagingFixture(t, expected, "new")

	require.NoError(t, WriteDesired(desiredPath, staging, expected))
	require.FileExists(t, filepath.Join(desiredPath, MetadataFile))
	require.FileExists(t, filepath.Join(desiredPath, PayloadDir, "bundle.txt"))
	read := ReadDesired(desiredPath, expected)
	require.Equal(t, "present", read.Status)
	require.NotNil(t, read.Metadata)

	replacement := stagingFixture(t, expected, "replacement")
	require.NoError(t, WriteDesired(desiredPath, replacement, expected))
	require.Equal(t, "replacement", readFile(t, filepath.Join(desiredPath, PayloadDir, "bundle.txt")))
}

func TestWriteDesiredRejectsUntrustedExistingPath(t *testing.T) {
	t.Parallel()

	expected := ExpectedIdentity{TargetRef: "native.app", SettingRef: "native.app:settings", ResourceID: "settings", OperationID: "export-settings", ArtifactForm: "native-export"}

	t.Run("existing file", func(t *testing.T) {
		parent := realTempDir(t)
		desiredPath := filepath.Join(parent, "artifact")
		writeFile(t, desiredPath, "not a manager artifact")
		err := WriteDesired(desiredPath, stagingFixture(t, expected, "new"), expected)
		require.Error(t, err)
		require.Contains(t, err.Error(), "manager-owned")
	})

	t.Run("symlink", func(t *testing.T) {
		parent := realTempDir(t)
		desiredPath := filepath.Join(parent, "artifact")
		require.NoError(t, os.Symlink(t.TempDir(), desiredPath))
		err := WriteDesired(desiredPath, stagingFixture(t, expected, "new"), expected)
		require.Error(t, err)
		require.Contains(t, err.Error(), "manager-owned")
	})

	t.Run("corrupt metadata", func(t *testing.T) {
		parent := realTempDir(t)
		desiredPath := filepath.Join(parent, "artifact")
		writeFile(t, filepath.Join(desiredPath, MetadataFile), "{")
		err := WriteDesired(desiredPath, stagingFixture(t, expected, "new"), expected)
		require.Error(t, err)
		require.Contains(t, err.Error(), "metadata")
	})

	t.Run("mismatched metadata", func(t *testing.T) {
		parent := realTempDir(t)
		desiredPath := filepath.Join(parent, "artifact")
		mismatch := expected
		mismatch.OperationID = "other-export"
		_ = stagingFixtureAt(t, desiredPath, mismatch, "old")
		err := WriteDesired(desiredPath, stagingFixture(t, expected, "new"), expected)
		require.Error(t, err)
		require.Contains(t, err.Error(), "operation")
	})

	t.Run("symlink parent", func(t *testing.T) {
		parent := realTempDir(t)
		realParent := filepath.Join(parent, "real")
		linkParent := filepath.Join(parent, "link")
		require.NoError(t, os.MkdirAll(realParent, 0o755))
		require.NoError(t, os.Symlink(realParent, linkParent))
		err := WriteDesired(filepath.Join(linkParent, "artifact"), stagingFixture(t, expected, "new"), expected)
		require.Error(t, err)
		require.Contains(t, err.Error(), "symlink")
	})

	t.Run("staged metadata mismatch", func(t *testing.T) {
		parent := realTempDir(t)
		mismatch := expected
		mismatch.ResourceID = "other"
		err := WriteDesired(filepath.Join(parent, "artifact"), stagingFixture(t, mismatch, "new"), expected)
		require.Error(t, err)
		require.Contains(t, err.Error(), "resource")
	})
}

func TestWriteDesiredRejectsInvalidInputsAndStaging(t *testing.T) {
	t.Parallel()

	expected := ExpectedIdentity{TargetRef: "native.app", SettingRef: "native.app:settings", ResourceID: "settings", OperationID: "export-settings", ArtifactForm: "native-export"}
	require.ErrorContains(t, WriteDesired(" ", stagingFixture(t, expected, "new"), expected), "path is required")

	t.Run("missing staged metadata", func(t *testing.T) {
		err := WriteDesired(filepath.Join(realTempDir(t), "artifact"), realTempDir(t), expected)
		require.ErrorContains(t, err, "staged native export metadata is invalid")
	})

	t.Run("unsafe staged payload", func(t *testing.T) {
		staging := stagingFixture(t, expected, "new")
		payloadFile := filepath.Join(staging, PayloadDir, "bundle.txt")
		require.NoError(t, os.Remove(payloadFile))
		require.NoError(t, os.Symlink("/tmp", payloadFile))
		err := WriteDesired(filepath.Join(realTempDir(t), "artifact"), staging, expected)
		require.ErrorContains(t, err, "symlink")
	})

	t.Run("unsafe staged sibling blocks copy", func(t *testing.T) {
		staging := stagingFixture(t, expected, "new")
		require.NoError(t, os.Symlink("/tmp", filepath.Join(staging, "extra-link")))
		err := WriteDesired(filepath.Join(realTempDir(t), "artifact"), staging, expected)
		require.ErrorContains(t, err, "symlink")
	})
}

func TestCopyPayloadAndValidatePayload(t *testing.T) {
	t.Parallel()

	src := realTempDir(t)
	writeFile(t, filepath.Join(src, "bundle.txt"), "payload")
	writeFile(t, filepath.Join(src, "nested", "settings.json"), "{}")
	expected, err := SummarizePayload(src, Limits{MaxBytes: 1024, MaxEntries: 10})
	require.NoError(t, err)

	dst := filepath.Join(realTempDir(t), "copy", PayloadDir)
	require.NoError(t, CopyPayload(src, dst))
	require.Equal(t, "payload", readFile(t, filepath.Join(dst, "bundle.txt")))
	require.Equal(t, "{}", readFile(t, filepath.Join(dst, "nested", "settings.json")))
	summary, err := ValidatePayload(dst, expected, Limits{MaxBytes: 1024, MaxEntries: 10})
	require.NoError(t, err)
	require.Equal(t, expected.SHA256, summary.SHA256)

	wrongHash := expected
	wrongHash.SHA256 = strings.Repeat("0", 64)
	_, err = ValidatePayload(dst, wrongHash, Limits{MaxBytes: 1024, MaxEntries: 10})
	require.ErrorContains(t, err, "hash")

	wrongNormalizer := expected
	wrongNormalizer.Normalizer = "other"
	_, err = ValidatePayload(dst, wrongNormalizer, Limits{MaxBytes: 1024, MaxEntries: 10})
	require.ErrorContains(t, err, "normalizer")

	require.NoError(t, os.Symlink("/tmp", filepath.Join(src, "link")))
	require.ErrorContains(t, CopyPayload(src, filepath.Join(realTempDir(t), "blocked")), "symlink")
	require.ErrorContains(t, CopyPayload(" ", filepath.Join(realTempDir(t), "blocked")), "path is required")
}

func TestLowLevelMetadataPayloadAndHashValidationErrors(t *testing.T) {
	t.Parallel()

	root := realTempDir(t)
	_, err := fileSHA256(filepath.Join(root, "missing"))
	require.Error(t, err)
	_, err = fileSHA256(root)
	require.Error(t, err)

	notDir := filepath.Join(root, "not-dir")
	writeFile(t, notDir, "payload")
	_, err = SummarizePayload(notDir, Limits{MaxBytes: 1024, MaxEntries: 10})
	require.ErrorContains(t, err, "payload root")

	metadataPath := filepath.Join(root, "metadata.json")
	writeFile(t, metadataPath, `{"schema":"x","unknown":true}`)
	_, err = readMetadata(metadataPath)
	require.Error(t, err)
	if runtime.GOOS != "windows" {
		unreadableMetadataPath := filepath.Join(root, "unreadable-metadata.json")
		writeFile(t, unreadableMetadataPath, `{"schema":"x"}`)
		require.NoError(t, os.Chmod(unreadableMetadataPath, 0o000))
		_, err = readMetadata(unreadableMetadataPath)
		require.Error(t, err)
		require.NoError(t, os.Chmod(unreadableMetadataPath, 0o644))
	}

	require.NoError(t, os.Remove(metadataPath))
	require.NoError(t, os.Symlink("/tmp", metadataPath))
	_, err = readMetadata(metadataPath)
	require.ErrorContains(t, err, "regular file")

	require.Error(t, copyTree(filepath.Join(root, "missing"), filepath.Join(root, "dst")))
}

type recordingExecutor struct {
	body               string
	result             nativeops.ExecResult
	skipWrite          bool
	writeSymlink       bool
	blockMetadataWrite bool
	calls              int
}

func (e *recordingExecutor) Run(ctx context.Context, spec nativeops.ExecSpec) nativeops.ExecResult {
	e.calls++
	if e.result.Err != nil || e.result.ExitCode != 0 || e.result.TimedOut || e.result.Stdout.LimitExceeded || e.result.Stderr.LimitExceeded {
		return e.result
	}
	if !e.skipWrite {
		if len(spec.Args) < 2 {
			return nativeops.ExecResult{ExitCode: 2, Err: errors.New("missing output arg")}
		}
		outputPath := spec.Args[1]
		if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
			return nativeops.ExecResult{ExitCode: 1, Err: err}
		}
		if e.writeSymlink {
			if err := os.Symlink("/tmp", outputPath); err != nil {
				return nativeops.ExecResult{ExitCode: 1, Err: err}
			}
		} else if err := os.WriteFile(outputPath, []byte(e.body), 0o644); err != nil {
			return nativeops.ExecResult{ExitCode: 1, Err: err}
		}
		if e.blockMetadataWrite {
			stagingRoot := filepath.Dir(filepath.Dir(outputPath))
			if err := os.Mkdir(filepath.Join(stagingRoot, MetadataFile), 0o755); err != nil {
				return nativeops.ExecResult{ExitCode: 1, Err: err}
			}
		}
	}
	stdout := e.result.Stdout
	if stdout.Mode == "" {
		stdout.Mode = spec.Stdout.Mode
	}
	stderr := e.result.Stderr
	if stderr.Mode == "" {
		stderr.Mode = spec.Stderr.Mode
	}
	return nativeops.ExecResult{ExitCode: 0, Stdout: stdout, Stderr: stderr}
}

func nativeExportOptions(t *testing.T, rec *recipe.Recipe, executor nativeops.Executor) Options {
	t.Helper()
	return Options{
		RepoRoot:        t.TempDir(),
		StateRoot:       t.TempDir(),
		Recipe:          rec,
		RecipeSource:    recipe.RecipeSourceBundled,
		TrustEvaluation: &recipe.TrustEvaluation{Status: "trusted"},
		Setting:         resolution.ResolvedSetting{TargetID: "native.app", SettingID: "settings", Scope: "user", Subject: "alice"},
		ResourceID:      "settings",
		Resource:        rec.Resources["settings"],
		MachineID:       "machine-1",
		UserID:          "user-1",
		RunID:           "run:with spaces",
		Now: func() time.Time {
			return time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
		},
		Executor: executor,
	}
}

func nativeExportTestRecipe(t *testing.T) *recipe.Recipe {
	t.Helper()
	rec := &recipe.Recipe{
		Schema:        recipe.Schema,
		SchemaVersion: recipe.SupportedVersion,
		Target:        "native.app",
		DisplayName:   "Native App",
		SupportLevel:  "experimental",
		Capability:    "export-only",
		Settings: map[string]recipe.Setting{
			"settings": {Label: "Settings bundle", SupportLevel: "experimental", Capability: "export-only", ArtifactForm: "native-export", Sensitivity: "personal", Redaction: "redacted-for-display", Lifecycle: recipe.LifecycleAllowed, ScopeDefault: "user", Resource: "settings"},
		},
		Resources: map[string]recipe.Resource{
			"settings": {Driver: recipe.NativeExportDriverID, NativeOperation: "export-settings", Capability: "export-only", Sensitivity: "personal", Redaction: "redacted-for-display", Lifecycle: recipe.LifecycleAllowed},
		},
		NativeOperations: map[string]recipe.NativeOperation{
			"export-settings": {
				Kind:              "export",
				Reviewed:          true,
				Runner:            "command",
				Platforms:         []string{runtime.GOOS},
				ArtifactForm:      "native-export",
				DiffMode:          "metadata-only",
				Lifecycle:         recipe.LifecycleAllowed,
				WorkingDirectory:  "temp",
				TimeoutSeconds:    5,
				ExpectedExitCodes: []int{0},
				Command: recipe.NativeCommand{Executable: "/usr/bin/native-safe-tool", Args: []recipe.NativeArg{
					{Literal: "export"},
					{Output: "bundle"},
				}},
				Stdin:     recipe.NativeStdinPolicy{Mode: "none"},
				Stdout:    recipe.NativeStreamPolicy{Mode: "discard"},
				Stderr:    recipe.NativeStreamPolicy{Mode: "discard"},
				Outputs:   map[string]recipe.NativePathSpec{"bundle": {Root: "artifact", Path: "bundle.txt"}},
				Redaction: "metadata-only",
				Review:    recipe.NativeReviewPolicy{Required: true, Reasons: []string{"opaque", "account-bound"}, Message: "Review native export before running"},
				Limits:    recipe.NativeExportLimits{MaxBytes: 1024, MaxEntries: 10},
				ExportMetadata: recipe.NativeExportMetadataPolicy{
					CapturedCategories: []string{"settings"},
					SecretExclusions:   []string{"tokens"},
					AccountExclusions:  []string{"sessions"},
					Limitations:        []string{"Internal app settings are not semantically compared"},
				},
			},
		},
	}
	require.NoError(t, rec.Validate())
	return rec
}

func stagingFixture(t *testing.T, expected ExpectedIdentity, body string) string {
	t.Helper()
	return stagingFixtureAt(t, t.TempDir(), expected, body)
}

func stagingFixtureAt(t *testing.T, root string, expected ExpectedIdentity, body string) string {
	t.Helper()
	payload := filepath.Join(root, PayloadDir)
	writeFile(t, filepath.Join(payload, "bundle.txt"), body)
	summary, err := SummarizePayload(payload, Limits{MaxBytes: 1024, MaxEntries: 10})
	require.NoError(t, err)
	err = WriteMetadata(root, Metadata{
		Schema:        MetadataSchema,
		SchemaVersion: SchemaVersion,
		TargetRef:     expected.TargetRef,
		SettingRef:    expected.SettingRef,
		ResourceID:    expected.ResourceID,
		OperationID:   expected.OperationID,
		Operation:     OperationMetadata{ArtifactForm: expected.ArtifactForm},
		Payload:       summary,
	})
	require.NoError(t, err)
	return root
}

func writeFile(t *testing.T, path string, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

func realTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	return real
}
