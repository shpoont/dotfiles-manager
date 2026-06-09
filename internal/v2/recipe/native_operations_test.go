package recipe

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNativeOperationValidationAcceptsReviewedBoundedCommand(t *testing.T) {
	t.Parallel()

	rec := nativeOperationTestRecipe()
	require.NoError(t, rec.Validate())

	surface, _, err := RecipeWriteSurface(rec)
	require.NoError(t, err)
	require.True(t, surface.NativeOperations.Supported)
	require.Equal(t, 1, surface.NativeOperations.Count)
	require.Contains(t, surface.NativeOperations.Summary, "export-settings:export:command:native:metadata-only")
}

func TestNativeOperationValidationRejectsUnsafeShapes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		mut  func(*NativeOperation)
		code string
		id   string
	}{
		{name: "unreviewed", mut: func(op *NativeOperation) { op.Reviewed = false }, code: "nativeOperation.reviewed.required"},
		{name: "invalid id", mut: func(op *NativeOperation) {}, code: "nativeOperation.id.invalid", id: "bad id"},
		{name: "unknown kind", mut: func(op *NativeOperation) { op.Kind = "sync" }, code: "nativeOperation.kind.unsupported"},
		{name: "unsupported runner", mut: func(op *NativeOperation) { op.Runner = "shell" }, code: "nativeOperation.runner.unsupported"},
		{name: "missing platforms", mut: func(op *NativeOperation) { op.Platforms = nil }, code: "nativeOperation.platforms.required"},
		{name: "unsupported platform", mut: func(op *NativeOperation) { op.Platforms = []string{"plan9"} }, code: "nativeOperation.platform.unsupported"},
		{name: "unsupported artifact form", mut: func(op *NativeOperation) { op.ArtifactForm = "file" }, code: "nativeOperation.artifactForm.unsupported"},
		{name: "unsupported diff mode", mut: func(op *NativeOperation) { op.DiffMode = "raw" }, code: "nativeOperation.diffMode.unsupported"},
		{name: "unsupported lifecycle", mut: func(op *NativeOperation) { op.Lifecycle = "sometimes" }, code: "nativeOperation.lifecycle.unsupported"},
		{name: "blank executable", mut: func(op *NativeOperation) { op.Command.Executable = "" }, code: "nativeOperation.command.executable.required"},
		{name: "spaced executable", mut: func(op *NativeOperation) { op.Command.Executable = " /usr/bin/native-safe-tool" }, code: "nativeOperation.command.executable.invalid"},
		{name: "relative executable", mut: func(op *NativeOperation) { op.Command.Executable = "tool" }, code: "nativeOperation.command.executable.notAbsolute"},
		{name: "unclean executable", mut: func(op *NativeOperation) { op.Command.Executable = "/usr/bin/../bin/native-safe-tool" }, code: "nativeOperation.command.executable.invalidPath"},
		{name: "shell executable", mut: func(op *NativeOperation) { op.Command.Executable = "/bin/sh" }, code: "nativeOperation.command.executable.blocked"},
		{name: "windows shell executable", mut: func(op *NativeOperation) {
			op.Command.Executable = "C:/Windows/System32/WindowsPowerShell/v1.0/powershell.exe"
		}, code: "nativeOperation.command.executable.blocked"},
		{name: "osascript inline", mut: func(op *NativeOperation) {
			op.Command.Executable = "/usr/bin/osascript"
			op.Command.Args = []NativeArg{{Literal: "-e"}, {Literal: "display dialog \"x\""}}
		}, code: "nativeOperation.command.executable.blocked"},
		{name: "partial interpolation", mut: func(op *NativeOperation) { op.Command.Args = []NativeArg{{Literal: "--out={{artifact}}"}} }, code: "nativeOperation.command.arg.interpolationUnsupported"},
		{name: "multi choice arg", mut: func(op *NativeOperation) { op.Command.Args = []NativeArg{{Literal: "x", Output: "bundle"}} }, code: "nativeOperation.command.arg.invalid"},
		{name: "nul arg", mut: func(op *NativeOperation) { op.Command.Args = []NativeArg{{Literal: "x\x00"}} }, code: "nativeOperation.command.arg.invalid"},
		{name: "unknown input", mut: func(op *NativeOperation) { op.Command.Args = []NativeArg{{Input: "missing"}} }, code: "nativeOperation.command.arg.inputUnknown"},
		{name: "unknown output", mut: func(op *NativeOperation) { op.Command.Args = []NativeArg{{Output: "missing"}} }, code: "nativeOperation.command.arg.outputUnknown"},
		{name: "unknown temp", mut: func(op *NativeOperation) { op.Command.Args = []NativeArg{{Temp: "missing"}} }, code: "nativeOperation.command.arg.tempUnknown"},
		{name: "missing timeout", mut: func(op *NativeOperation) { op.TimeoutSeconds = 0 }, code: "nativeOperation.timeout.invalid"},
		{name: "huge timeout", mut: func(op *NativeOperation) { op.TimeoutSeconds = MaxNativeOperationTimeoutSeconds + 1 }, code: "nativeOperation.timeout.invalid"},
		{name: "empty exits", mut: func(op *NativeOperation) { op.ExpectedExitCodes = nil }, code: "nativeOperation.expectedExitCodes.invalid"},
		{name: "too many exits", mut: func(op *NativeOperation) {
			op.ExpectedExitCodes = []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
		}, code: "nativeOperation.expectedExitCodes.invalid"},
		{name: "bad exit", mut: func(op *NativeOperation) { op.ExpectedExitCodes = []int{0, 999} }, code: "nativeOperation.expectedExitCode.invalid"},
		{name: "duplicate exit", mut: func(op *NativeOperation) { op.ExpectedExitCodes = []int{0, 0} }, code: "nativeOperation.expectedExitCode.invalid"},
		{name: "stdin not none", mut: func(op *NativeOperation) { op.Stdin = NativeStdinPolicy{Mode: "pipe"} }, code: "nativeOperation.stdin.unsupported"},
		{name: "capture unbounded", mut: func(op *NativeOperation) { op.Stdout = NativeStreamPolicy{Mode: "capture"} }, code: "nativeOperation.stream.maxBytes.invalid"},
		{name: "capture too large", mut: func(op *NativeOperation) {
			op.Stdout = NativeStreamPolicy{Mode: "capture", MaxBytes: MaxNativeCaptureBytes + 1}
		}, code: "nativeOperation.stream.maxBytes.invalid"},
		{name: "discard max bytes", mut: func(op *NativeOperation) { op.Stderr = NativeStreamPolicy{Mode: "discard", MaxBytes: 1} }, code: "nativeOperation.stream.maxBytes.invalid"},
		{name: "unsupported stream mode", mut: func(op *NativeOperation) { op.Stderr = NativeStreamPolicy{Mode: "raw"} }, code: "nativeOperation.stream.mode.unsupported"},
		{name: "implicit cwd", mut: func(op *NativeOperation) { op.WorkingDirectory = "caller" }, code: "nativeOperation.workingDirectory.unsupported"},
		{name: "invalid env key", mut: func(op *NativeOperation) { op.Env["bad-key"] = NativeEnvValue{Literal: "value"} }, code: "nativeOperation.env.key.invalid"},
		{name: "unsupported env key", mut: func(op *NativeOperation) { op.Env["PATH"] = NativeEnvValue{Literal: "/bin"} }, code: "nativeOperation.env.key.unsupported"},
		{name: "execution influencing manager env key", mut: func(op *NativeOperation) { op.Env["DFM_PATH"] = NativeEnvValue{Literal: "/bin"} }, code: "nativeOperation.env.key.unsupported"},
		{name: "inherit env by typo", mut: func(op *NativeOperation) { op.Env["API_TOKEN"] = NativeEnvValue{Literal: "secret"} }, code: "nativeOperation.env.key.sensitive"},
		{name: "multi choice env", mut: func(op *NativeOperation) { op.Env["DFM_BAD"] = NativeEnvValue{Literal: "x", Output: "bundle"} }, code: "nativeOperation.env.value.invalid"},
		{name: "env control char", mut: func(op *NativeOperation) { op.Env["DFM_BAD"] = NativeEnvValue{Literal: "x\n"} }, code: "nativeOperation.env.value.invalid"},
		{name: "env interpolation", mut: func(op *NativeOperation) { op.Env["DFM_BAD"] = NativeEnvValue{Literal: "{{x}}"} }, code: "nativeOperation.env.value.interpolationUnsupported"},
		{name: "env unknown input", mut: func(op *NativeOperation) { op.Env["DFM_BAD"] = NativeEnvValue{Input: "missing"} }, code: "nativeOperation.env.inputUnknown"},
		{name: "env unknown output", mut: func(op *NativeOperation) { op.Env["DFM_BAD"] = NativeEnvValue{Output: "missing"} }, code: "nativeOperation.env.outputUnknown"},
		{name: "env unknown temp", mut: func(op *NativeOperation) { op.Env["DFM_BAD"] = NativeEnvValue{Temp: "missing"} }, code: "nativeOperation.env.tempUnknown"},
		{name: "path invalid id", mut: func(op *NativeOperation) { op.Outputs["bad id"] = NativePathSpec{Root: "artifact", Path: "x"} }, code: "nativeOperation.path.id.invalid"},
		{name: "unsupported path root", mut: func(op *NativeOperation) { op.Outputs["bundle"] = NativePathSpec{Root: "repo", Path: "x"} }, code: "nativeOperation.path.root.unsupported"},
		{name: "location root missing location", mut: func(op *NativeOperation) { op.Outputs["bundle"] = NativePathSpec{Root: "location", Path: "x"} }, code: "nativeOperation.path.location.required"},
		{name: "artifact with location", mut: func(op *NativeOperation) {
			op.Outputs["bundle"] = NativePathSpec{Root: "artifact", Location: "home", Path: "x"}
		}, code: "nativeOperation.path.location.unexpected"},
		{name: "path traversal", mut: func(op *NativeOperation) { op.Outputs["bundle"] = NativePathSpec{Root: "artifact", Path: "../escape"} }, code: "nativeOperation.path.path.invalid"},
		{name: "export output must not be location", mut: func(op *NativeOperation) {
			op.Outputs["bundle"] = NativePathSpec{Root: "location", Location: "home", Path: "x"}
		}, code: "nativeOperation.path.output.exportRootUnsupported"},
		{name: "verify output must be temp", mut: func(op *NativeOperation) { op.Kind = "verify" }, code: "nativeOperation.path.output.verifyRootUnsupported"},
		{name: "import output must be temp", mut: func(op *NativeOperation) { op.Kind = "import" }, code: "nativeOperation.path.output.importRootUnsupported"},
		{name: "import input must not be location", mut: func(op *NativeOperation) {
			op.Kind = "import"
			op.Inputs = map[string]NativePathSpec{"source": {Root: "location", Location: "home", Path: "x"}}
		}, code: "nativeOperation.path.input.importRootUnsupported"},
		{name: "export input desired unsupported", mut: func(op *NativeOperation) {
			op.Inputs = map[string]NativePathSpec{"desired": {Root: "artifact", Path: "x"}}
		}, code: "nativeOperation.path.input.exportArtifactUnsupported"},
		{name: "unsupported redaction", mut: func(op *NativeOperation) { op.Redaction = "raw" }, code: "nativeOperation.redaction.unsupported"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := nativeOperationTestRecipe()
			operationID := "export-settings"
			if tc.id != "" {
				operationID = tc.id
			}
			op := rec.NativeOperations["export-settings"]
			delete(rec.NativeOperations, "export-settings")
			tc.mut(&op)
			rec.NativeOperations[operationID] = op
			diagnostics := ValidationDiagnostics(rec.Validate())
			require.NotEmpty(t, diagnostics)
			codes := make([]string, 0, len(diagnostics))
			for _, diagnostic := range diagnostics {
				codes = append(codes, diagnostic.Code)
			}
			require.Contains(t, codes, tc.code, diagnostics)
		})
	}
}

func TestDecodeRejectsUnknownNativeOperationFields(t *testing.T) {
	t.Parallel()

	body := `schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: native-test
displayName: Native Test
supportLevel: experimental
capability: read-write
locations:
  home:
    default: "~"
resources:
  marker:
    driver: file
    location: home
    path: .native-test/marker
settings:
  settings:
    scopeDefault: user
    resource: marker
nativeOperations:
  export-settings:
    kind: export
    reviewed: true
    runner: command
    platforms: [darwin]
    artifactForm: native
    diffMode: metadata-only
    lifecycle: allowed
    workingDirectory: temp
    timeoutSeconds: 5
    expectedExitCodes: [0]
    unknownField: nope
    command:
      executable: /usr/bin/native-safe-tool
    stdin:
      mode: none
    stdout:
      mode: discard
    stderr:
      mode: discard
    redaction: metadata-only
`
	_, err := Decode("native.yaml", strings.NewReader(body))
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknownField")
}

func TestExplainFromRecipeSummarizesNativeOperationsWithoutArgv(t *testing.T) {
	t.Parallel()

	explain := explainFromRecipe(nativeOperationTestRecipe(), RecipeSourceLocal, "recipe://local/native-test", TrustStatusReviewRequired)
	require.Len(t, explain.NativeOperations, 1)
	op := explain.NativeOperations[0]
	require.Equal(t, "export-settings", op.ID)
	require.Equal(t, "export", op.Kind)
	require.Equal(t, "reviewed argv command; executable and args are not printed by recipe explain", op.CommandSummary)
	require.Equal(t, []string{"bundle"}, op.OutputIDs)
	require.NotContains(t, op.CommandSummary, "/usr/bin/native-safe-tool")
	require.Equal(t, "unknown", nativeOperationPlatformSupport(nil))
	require.Equal(t, "discard", streamPolicySummary(NativeStreamPolicy{Mode: "discard"}))
	require.Equal(t, "raw", streamPolicySummary(NativeStreamPolicy{Mode: "raw"}))
}

func nativeOperationTestRecipe() *Recipe {
	return &Recipe{
		Schema:        Schema,
		SchemaVersion: SupportedVersion,
		Target:        "native-test",
		DisplayName:   "Native Test",
		SupportLevel:  "experimental",
		Capability:    "read-write",
		Locations: map[string]Location{
			"home": {Default: "~"},
		},
		Resources: map[string]Resource{
			"marker": {Driver: FileDriverID, Location: "home", Path: ".native-test/marker", Capability: "read-write"},
		},
		Settings: map[string]Setting{
			"settings": {Label: "Settings", ScopeDefault: "user", Resource: "marker", Capability: "read-write", ArtifactForm: "native", Lifecycle: LifecycleAllowed},
		},
		NativeOperations: map[string]NativeOperation{
			"export-settings": {
				Kind:              "export",
				Reviewed:          true,
				Runner:            "command",
				Platforms:         []string{"darwin"},
				ArtifactForm:      "native",
				DiffMode:          "metadata-only",
				Lifecycle:         LifecycleAllowed,
				WorkingDirectory:  "temp",
				TimeoutSeconds:    5,
				ExpectedExitCodes: []int{0},
				Command: NativeCommand{Executable: "/usr/bin/native-safe-tool", Args: []NativeArg{
					{Literal: "--out"},
					{Output: "bundle"},
				}},
				Stdin:     NativeStdinPolicy{Mode: "none"},
				Stdout:    NativeStreamPolicy{Mode: "capture", MaxBytes: 4096},
				Stderr:    NativeStreamPolicy{Mode: "discard"},
				Env:       map[string]NativeEnvValue{"DFM_OUTPUT": {Output: "bundle"}},
				Outputs:   map[string]NativePathSpec{"bundle": {Root: "artifact", Path: "exports/settings.bundle"}},
				Redaction: "metadata-only",
			},
		},
	}
}
