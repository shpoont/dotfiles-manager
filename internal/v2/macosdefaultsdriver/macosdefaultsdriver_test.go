package macosdefaultsdriver

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"runtime"
	"testing"
	"time"

	"github.com/shpoont/dotfiles-manager/internal/v2/filedriver"
	"github.com/stretchr/testify/require"
	"howett.net/plist"
)

type fakeDefaultsRunner struct {
	result ExportResult
	err    error
	calls  []string
}

func (r *fakeDefaultsRunner) Export(ctx context.Context, domain string, limits OutputLimits) (ExportResult, error) {
	r.calls = append(r.calls, fmt.Sprintf("%s stdout=%d stderr=%d", domain, limits.StdoutBytes, limits.StderrBytes))
	return r.result, r.err
}

func TestReadCurrentSelectedScalarFromDefaultsExport(t *testing.T) {
	t.Parallel()

	runner := &fakeDefaultsRunner{result: ExportResult{Stdout: defaultsExport(t, map[string]any{"Email": "secret@example.com", "Enabled": true})}}
	state, err := Driver{}.ReadCurrent(Request{Domain: "com.example.app", Key: "Email", Runner: runner})
	require.NoError(t, err)
	require.True(t, state.Exists)
	require.Equal(t, NormalizerID, state.Normalizer)
	require.NotEmpty(t, state.SHA256)
	require.Equal(t, []byte(`"secret@example.com"`), state.Value)
	require.Len(t, runner.calls, 1)
	require.Contains(t, runner.calls[0], "com.example.app")

	payload, err := json.Marshal(state)
	require.NoError(t, err)
	require.NotContains(t, string(payload), "secret@example.com")
}

func TestReadCurrentTreatsMissingKeyAndMissingDomainAsAbsent(t *testing.T) {
	t.Parallel()

	emptyDomain := &fakeDefaultsRunner{result: ExportResult{Stdout: defaultsExport(t, map[string]any{})}}
	state, err := Driver{}.ReadCurrent(Request{Domain: "com.example.app", Key: "Email", Runner: emptyDomain})
	require.NoError(t, err)
	require.False(t, state.Exists)
	require.Equal(t, NormalizerID, state.Normalizer)

	missingDomain := &fakeDefaultsRunner{result: ExportResult{ExitCode: 1, Stderr: []byte("Domain com.example.app does not exist")}}
	state, err = Driver{}.ReadCurrent(Request{Domain: "com.example.app", Key: "Email", Runner: missingDomain})
	require.NoError(t, err)
	require.False(t, state.Exists)
}

func TestReadCurrentBlocksUnsupportedValuesAndMalformedExports(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		result ExportResult
	}{
		{name: "dictionary selected value", result: ExportResult{Stdout: defaultsExport(t, map[string]any{"Email": map[string]any{"nested": true}})}},
		{name: "array selected value", result: ExportResult{Stdout: defaultsExport(t, map[string]any{"Email": []any{"secret@example.com"}})}},
		{name: "malformed plist", result: ExportResult{Stdout: []byte("not plist")}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runner := &fakeDefaultsRunner{result: tc.result}
			_, err := Driver{}.ReadCurrent(Request{Domain: "com.example.app", Key: "Email", Runner: runner})
			require.Error(t, err)
			require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
			require.NotContains(t, err.Error(), "secret@example.com")
		})
	}
}

func TestPreviewDiffIsReadOnlyAndRedactsValues(t *testing.T) {
	t.Parallel()

	runner := &fakeDefaultsRunner{result: ExportResult{Stdout: defaultsExport(t, map[string]any{"Email": "old@example.com"})}}
	desired, err := Driver{}.NormalizeValue("new@example.com")
	require.NoError(t, err)
	preview, err := Driver{}.PreviewDiff(Request{Domain: "com.example.app", Key: "Email", Runner: runner}, desired)
	require.NoError(t, err)
	require.True(t, preview.ReadOnly)
	require.Equal(t, filedriver.ChangeUpdate, preview.Change.Kind)
	require.Equal(t, IntentSet, preview.Intent)
	require.Equal(t, `defaults://current-user/com.example.app/Email`, preview.Path)

	payload, err := json.Marshal(preview)
	require.NoError(t, err)
	require.Contains(t, string(payload), `"readOnly":true`)
	require.NotContains(t, string(payload), "old@example.com")
	require.NotContains(t, string(payload), "new@example.com")
}

func TestRuntimeErrorsAreStableAndDoNotLeakOutput(t *testing.T) {
	t.Parallel()

	runner := &fakeDefaultsRunner{result: ExportResult{ExitCode: 2, Stderr: []byte("raw secret stderr: secret@example.com")}}
	_, err := Driver{}.ReadCurrent(Request{Domain: "com.example.app", Key: "Email", Runner: runner})
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInternal), err.Error())
	require.NotContains(t, err.Error(), "secret@example.com")

	runner = &fakeDefaultsRunner{result: ExportResult{Stdout: []byte("secret@example.com")}}
	_, err = Driver{}.ReadCurrent(Request{Domain: "com.example.app", Key: "Email", Runner: runner, Limits: OutputLimits{StdoutBytes: 4, StderrBytes: 4}})
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInternal), err.Error())
	require.NotContains(t, err.Error(), "secret@example.com")
}

func TestValidateRequestAndLogicalSelectorMetadata(t *testing.T) {
	t.Parallel()

	require.NoError(t, ValidateRequest(Request{Domain: "com.example.app", Key: "A/B"}))
	require.Equal(t, "defaults://current-user/com.example.app/A%2FB", LogicalPath("com.example.app", "A/B"))
	require.Equal(t, `domain="com.example.app" key="A/B"`, SelectorSummary("com.example.app", "A/B"))

	for _, req := range []Request{
		{Domain: "", Key: "Email"},
		{Domain: " com.example.app", Key: "Email"},
		{Domain: "com/example", Key: "Email"},
		{Domain: "-domain", Key: "Email"},
		{Domain: "com.example.app", Key: ""},
		{Domain: "com.example.app", Key: " Email"},
		{Domain: "com.example.app", Key: "-Email"},
		{Domain: "com.example.app", Key: "Line\nBreak"},
	} {
		require.Error(t, ValidateRequest(req), "request: %+v", req)
	}
}

func TestReadOnlyMutationAndBackupAPIsFailBeforeHooks(t *testing.T) {
	t.Parallel()

	req := Request{Domain: "com.example.app", Key: "Email", Runner: &fakeDefaultsRunner{}}
	desired, err := Driver{}.NormalizeValue("new@example.com")
	require.NoError(t, err)

	called := false
	_, err = Driver{}.Backup(req, func(req BackupRequest) (BackupResult, error) {
		called = true
		return BackupResult{}, nil
	})
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsupported), err.Error())
	require.False(t, called)

	_, backup, err := Driver{}.ApplyWithBackup(req, desired, func(req BackupRequest) (BackupResult, error) {
		called = true
		return BackupResult{}, nil
	})
	require.Error(t, err)
	require.Nil(t, backup)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsupported), err.Error())
	require.False(t, called)

	called = false
	err = Driver{}.Restore(req, BackupResult{ID: "backup"}, func(req RestoreRequest) error {
		called = true
		return nil
	})
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsupported), err.Error())
	require.False(t, called)
}

func defaultsExport(t *testing.T, value map[string]any) []byte {
	t.Helper()
	data, err := plist.Marshal(value, plist.XMLFormat)
	require.NoError(t, err)
	return data
}

func TestDetectNormalizeDiffAndApplyWrapperBranches(t *testing.T) {
	t.Parallel()

	runner := &fakeDefaultsRunner{result: ExportResult{Stdout: defaultsExport(t, map[string]any{"Flag": true})}}
	detection, err := Driver{}.Detect(Request{Domain: "com.example.app", Key: "Flag", Runner: runner})
	require.NoError(t, err)
	require.True(t, detection.Exists)
	require.True(t, detection.Readable)
	require.Equal(t, "defaults://current-user/com.example.app/Flag", detection.Path)

	set, err := Driver{}.Normalize([]byte(`"value"`))
	require.NoError(t, err)
	same, err := Driver{}.NormalizeValue("value")
	require.NoError(t, err)
	updated, err := Driver{}.NormalizeValue("other")
	require.NoError(t, err)
	deleted := DeleteState()

	require.Equal(t, filedriver.ChangeUnchanged, Driver{}.Diff(AbsentState(), AbsentState()).Kind)
	require.Equal(t, filedriver.ChangeCreate, Driver{}.Diff(AbsentState(), set).Kind)
	require.Equal(t, filedriver.ChangeDelete, Driver{}.Diff(set, deleted).Kind)
	require.Equal(t, filedriver.ChangeUnchanged, Driver{}.Diff(set, same).Kind)
	require.Equal(t, filedriver.ChangeUpdate, Driver{}.Diff(set, updated).Kind)
	require.False(t, deleted.Snapshot().Exists)

	_, err = Driver{}.Normalize([]byte(`"unterminated`))
	require.Error(t, err)
	_, err = Driver{}.Normalize([]byte(`"value" true`))
	require.Error(t, err)

	_, err = Driver{}.Apply(Request{Domain: "com.example.app", Key: "Flag"}, set)
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeUnsupported), err.Error())
}

func TestScalarNormalizationCoversSupportedAndUnsupportedDefaultsTypes(t *testing.T) {
	t.Parallel()

	supported := []any{
		"value",
		true,
		json.Number("42"),
		json.Number("1.25"),
		int(1), int8(2), int16(3), int32(4), int64(5),
		uint(1), uint8(2), uint16(3), uint32(4), uint64(5),
		float32(1.5), float64(2.5),
	}
	for _, value := range supported {
		state, err := Driver{}.NormalizeValue(value)
		require.NoError(t, err, "%T", value)
		require.True(t, state.Exists)
		require.NotEmpty(t, state.SHA256)
	}

	unsupported := []any{
		nil,
		[]byte("data"),
		time.Now(),
		plist.UID(7),
		[]any{"value"},
		map[string]any{"nested": true},
		uint64(math.MaxInt64 + 1),
		math.NaN(),
		math.Inf(1),
	}
	for _, value := range unsupported {
		_, err := Driver{}.NormalizeValue(value)
		require.Error(t, err, "%T", value)
	}

	_, err := Driver{}.Normalize([]byte(`1e1000000000`))
	require.Error(t, err)
	_, err = Driver{}.NormalizeValue(struct{ Name string }{"value"})
	require.Error(t, err)
}

func TestRunnerClassificationAndBoundedBuffers(t *testing.T) {
	t.Parallel()

	stdout := &boundedBuffer{limit: 4, stream: "stdout"}
	n, err := stdout.Write([]byte("secret"))
	require.NoError(t, err)
	require.Equal(t, 6, n)
	require.True(t, stdout.truncated)
	require.Equal(t, []byte("secr"), stdout.Bytes())
	n, err = stdout.Write([]byte("again"))
	require.NoError(t, err)
	require.Equal(t, 5, n)

	unlimited := &boundedBuffer{limit: 0, stream: "stdout"}
	n, err = unlimited.Write([]byte("secret"))
	require.NoError(t, err)
	require.Equal(t, 6, n)
	require.Empty(t, unlimited.Bytes())

	req := Request{Domain: "com.example.app", Key: "Flag"}
	for _, err := range []error{timeoutError{}, outputLimitError{stream: "stdout"}, unsupportedRuntimeError{}, fmt.Errorf("boom")} {
		classified := classifyRunnerError(req, err)
		require.Error(t, classified)
		require.NotContains(t, classified.Error(), "secret")
	}
	driverErr := driverError(filedriver.CodeUnsupported, "read", LogicalPath(req.Domain, req.Key), fmt.Errorf("safe"))
	require.Same(t, driverErr, classifyRunnerError(req, driverErr))

	require.Equal(t, "missing-domain", stableStderrCategory([]byte("Domain com.example.app does not exist")))
	require.Equal(t, "command-failed", stableStderrCategory([]byte{}))
	require.Equal(t, "command-failed", stableStderrCategory([]byte("secret stderr")))

	_, err = unsupportedRunner{}.Export(context.Background(), "com.example.app", OutputLimits{})
	require.Error(t, err)
	if runtime.GOOS != "darwin" {
		_, err = Driver{}.ReadCurrent(Request{Domain: "com.example.app", Key: "Flag"})
		require.Error(t, err)
		require.True(t, filedriver.IsCode(err, filedriver.CodeUnsupported), err.Error())
	}
}

func TestPreviewDiffRejectsInvalidDesiredState(t *testing.T) {
	t.Parallel()

	runner := &fakeDefaultsRunner{result: ExportResult{Stdout: defaultsExport(t, map[string]any{"Flag": true})}}
	_, err := Driver{}.PreviewDiff(Request{Domain: "com.example.app", Key: "Flag", Runner: runner}, State{Exists: true, Intent: IntentDelete, Value: []byte(`true`)})
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())

	_, err = Driver{}.PreviewDiff(Request{Domain: "com.example.app", Key: "Flag", Runner: runner}, State{Exists: false})
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())

	deletePreview, err := Driver{}.PreviewDiff(Request{Domain: "com.example.app", Key: "Flag", Runner: runner}, DeleteState())
	require.NoError(t, err)
	require.True(t, deletePreview.ReadOnly)
	require.Equal(t, filedriver.ChangeDelete, deletePreview.Change.Kind)
}

func TestDefaultAndSystemRunnerBranchesAreStable(t *testing.T) {
	t.Parallel()

	runner := defaultRunner()
	if runtime.GOOS == "darwin" {
		require.IsType(t, systemRunner{}, runner)
	} else {
		require.IsType(t, unsupportedRunner{}, runner)
	}

	result, err := systemRunner{}.Export(context.Background(), "com.shpoont.dotfiles-manager.nonexistent-test-domain", OutputLimits{StdoutBytes: 256 * 1024, StderrBytes: 16 * 1024})
	if err != nil {
		require.NotContains(t, err.Error(), "secret")
	}
	require.LessOrEqual(t, len(result.Stdout), 256*1024)
	require.LessOrEqual(t, len(result.Stderr), 16*1024)
}

func TestReadOnlyAPIsValidateRequestBeforeUnsupported(t *testing.T) {
	t.Parallel()

	_, err := Driver{}.Backup(Request{}, nil)
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())

	_, _, err = Driver{}.ApplyWithBackup(Request{}, State{}, nil)
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())

	err = Driver{}.Restore(Request{}, BackupResult{}, nil)
	require.Error(t, err)
	require.True(t, filedriver.IsCode(err, filedriver.CodeInvalidSelector), err.Error())
}
