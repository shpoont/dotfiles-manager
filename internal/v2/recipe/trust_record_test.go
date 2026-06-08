package recipe

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLocalRecipeTrustRecordLifecycleAndWriteSafetyEvidence(t *testing.T) {
	t.Parallel()

	rec := decodeTrustTestRecipe(t)
	repoRoot := t.TempDir()
	stateRoot := t.TempDir()

	missing, err := EvaluateRecipeTrust(repoRoot, stateRoot, RecipeSourceLocal, rec)
	require.NoError(t, err)
	require.Equal(t, TrustStatusReviewRequired, missing.Status)
	requireTrustDiagnosticCodes(t, missing.Diagnostics, "trust.local.missingRecord")

	recorded, err := RecordLocalRecipeTrust(repoRoot, stateRoot, rec)
	require.NoError(t, err)
	require.Equal(t, TrustStatusTrusted, recorded.Status)
	require.NotEmpty(t, recorded.ContentSHA256)
	require.NotEmpty(t, recorded.WriteSurfaceSHA256)
	require.True(t, strings.HasPrefix(recorded.RecordPath, filepath.Join(stateRoot, "trust")))
	require.False(t, strings.HasPrefix(recorded.RecordPath, repoRoot))

	trusted, err := EvaluateRecipeTrust(repoRoot, stateRoot, RecipeSourceLocal, rec)
	require.NoError(t, err)
	require.Equal(t, TrustStatusTrusted, trusted.Status)
	ctx := trusted.WriteSafetyContext(WriteSafetyContext{})
	require.NoError(t, rec.ValidateWriteSafety(ctx))

	encoded, err := json.Marshal(trusted)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "localTrustEvidence")
	require.NotContains(t, string(encoded), "secret@example.com")
}

func TestLocalRecipeTrustBlocksNakedTrustedContext(t *testing.T) {
	t.Parallel()

	rec := decodeTrustTestRecipe(t)
	err := rec.ValidateWriteSafety(WriteSafetyContext{Source: RecipeSourceLocal, Trusted: true})
	require.Error(t, err)
	requireDiagnosticCodes(t, ValidationDiagnostics(err), "writeSafety.trust.evidenceRequired")
}

func TestLocalRecipeTrustRequiresReviewAfterContentChange(t *testing.T) {
	t.Parallel()

	rec := decodeTrustTestRecipe(t)
	repoRoot := t.TempDir()
	stateRoot := t.TempDir()
	trusted := recordAndEvaluateTrust(t, repoRoot, stateRoot, rec)

	changed := decodeTrustTestRecipeFromBody(t, strings.Replace(trustTestRecipeBody(), "displayName: Selected path test", "displayName: Changed selected path test", 1))
	eval, err := EvaluateRecipeTrust(repoRoot, stateRoot, RecipeSourceLocal, changed)
	require.NoError(t, err)
	require.Equal(t, TrustStatusReviewRequired, eval.Status)
	requireTrustDiagnosticCodes(t, eval.Diagnostics, "trust.local.recipeChanged")

	err = changed.ValidateWriteSafety(trusted.WriteSafetyContext(WriteSafetyContext{}))
	require.Error(t, err)
	requireDiagnosticCodes(t, ValidationDiagnostics(err), "writeSafety.trust.recipeChanged")
}

func TestLocalRecipeTrustRequiresReviewAfterWriteSurfaceBroadening(t *testing.T) {
	t.Parallel()

	rec := decodeTrustTestRecipe(t)
	repoRoot := t.TempDir()
	stateRoot := t.TempDir()
	trusted := recordAndEvaluateTrust(t, repoRoot, stateRoot, rec)

	broadened := decodeTrustTestRecipeFromBody(t, strings.Replace(trustTestRecipeBody(), "path: config.json", "path: broader/config.json", 1))
	eval, err := EvaluateRecipeTrust(repoRoot, stateRoot, RecipeSourceLocal, broadened)
	require.NoError(t, err)
	require.Equal(t, TrustStatusReviewRequired, eval.Status)
	requireTrustDiagnosticCodes(t, eval.Diagnostics, "trust.local.writeSurfaceChanged", "trust.local.writeSurfaceBroadened")

	err = broadened.ValidateWriteSafety(trusted.WriteSafetyContext(WriteSafetyContext{}))
	require.Error(t, err)
	requireDiagnosticCodes(t, ValidationDiagnostics(err), "writeSafety.trust.writeSurfaceChanged")
}

func TestLocalRecipeTrustRejectsCorruptRecordAndUnsafeStateRoots(t *testing.T) {
	t.Parallel()

	rec := decodeTrustTestRecipe(t)
	repoRoot := t.TempDir()
	stateRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(stateRoot, "trust"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(stateRoot, "trust", "trust-record.yaml"), []byte("not: [valid"), 0o600))
	eval, err := EvaluateRecipeTrust(repoRoot, stateRoot, RecipeSourceLocal, rec)
	require.NoError(t, err)
	require.Equal(t, TrustStatusBlocked, eval.Status)
	requireTrustDiagnosticCodes(t, eval.Diagnostics, "trust.record.corrupt")

	insideRepo := filepath.Join(repoRoot, ".local-state")
	eval, err = EvaluateRecipeTrust(repoRoot, insideRepo, RecipeSourceLocal, rec)
	require.NoError(t, err)
	require.Equal(t, TrustStatusBlocked, eval.Status)
	requireTrustDiagnosticCodes(t, eval.Diagnostics, "trust.stateRoot.invalid")
}

func TestLocalRecipeTrustRejectsStateSymlinkPaths(t *testing.T) {
	t.Parallel()

	rec := decodeTrustTestRecipe(t)
	repoRoot := t.TempDir()
	actualRoot := t.TempDir()
	linkRoot := filepath.Join(t.TempDir(), "state-root-link")
	require.NoError(t, os.Symlink(actualRoot, linkRoot))
	eval, err := EvaluateRecipeTrust(repoRoot, linkRoot, RecipeSourceLocal, rec)
	require.NoError(t, err)
	require.Equal(t, TrustStatusBlocked, eval.Status)
	requireTrustDiagnosticCodes(t, eval.Diagnostics, "trust.stateRoot.invalid")

	stateRoot := t.TempDir()
	target := t.TempDir()
	require.NoError(t, os.Symlink(target, filepath.Join(stateRoot, "trust")))

	_, err = RecordLocalRecipeTrust(repoRoot, stateRoot, rec)
	require.Error(t, err)
	require.Contains(t, err.Error(), "symlink")

	eval, err = EvaluateRecipeTrust(repoRoot, stateRoot, RecipeSourceLocal, rec)
	require.NoError(t, err)
	require.Equal(t, TrustStatusBlocked, eval.Status)
	requireTrustDiagnosticCodes(t, eval.Diagnostics, "trust.stateRoot.invalid")

	actualState := t.TempDir()
	_, err = RecordLocalRecipeTrust(repoRoot, actualState, rec)
	require.NoError(t, err)
	recordLinkState := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(recordLinkState, "trust"), 0o700))
	require.NoError(t, os.Symlink(filepath.Join(actualState, "trust", "trust-record.yaml"), filepath.Join(recordLinkState, "trust", "trust-record.yaml")))
	eval, err = EvaluateRecipeTrust(repoRoot, recordLinkState, RecipeSourceLocal, rec)
	require.NoError(t, err)
	require.Equal(t, TrustStatusBlocked, eval.Status)
	requireTrustDiagnosticCodes(t, eval.Diagnostics, "trust.stateRoot.invalid")
}

func TestBundledRecipeTrustDoesNotRequireLocalRecord(t *testing.T) {
	t.Parallel()

	rec := decodeTrustTestRecipe(t)
	eval, err := EvaluateRecipeTrust("", "", RecipeSourceBundled, rec)
	require.NoError(t, err)
	require.Equal(t, TrustStatusTrusted, eval.Status)
	require.NoError(t, rec.ValidateWriteSafety(eval.WriteSafetyContext(WriteSafetyContext{})))
}

func TestRecipeTrustEvaluationInputAndSourceBranches(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	stateRoot := t.TempDir()
	rec := decodeTrustTestRecipe(t)

	eval, err := EvaluateRecipeTrust(repoRoot, stateRoot, "", rec)
	require.NoError(t, err)
	require.Equal(t, TrustStatusBlocked, eval.Status)
	requireTrustDiagnosticCodes(t, eval.Diagnostics, "trust.source.required")

	eval, err = EvaluateRecipeTrust(repoRoot, stateRoot, "remote", rec)
	require.NoError(t, err)
	require.Equal(t, TrustStatusBlocked, eval.Status)
	requireTrustDiagnosticCodes(t, eval.Diagnostics, "trust.source.unsupported")

	eval, err = EvaluateRecipeTrust(repoRoot, stateRoot, RecipeSourceLocal, nil)
	require.NoError(t, err)
	require.Equal(t, TrustStatusBlocked, eval.Status)
	requireTrustDiagnosticCodes(t, eval.Diagnostics, "recipe.required")

	invalid := &Recipe{}
	eval, err = EvaluateRecipeTrust(repoRoot, stateRoot, RecipeSourceLocal, invalid)
	require.NoError(t, err)
	require.Equal(t, TrustStatusBlocked, eval.Status)
	require.NotEmpty(t, eval.Diagnostics)

	_, err = RecipeContentSHA256(nil)
	require.Error(t, err)
	_, _, err = RecipeWriteSurface(nil)
	require.Error(t, err)
	_, err = RecipeContentSHA256(invalid)
	require.Error(t, err)
	_, _, err = RecipeWriteSurface(invalid)
	require.Error(t, err)
}

func TestRecordLocalRecipeTrustExistingRecordBranches(t *testing.T) {
	t.Parallel()

	rec := decodeTrustTestRecipe(t)
	repoRoot := t.TempDir()
	stateRoot := t.TempDir()
	restrictive := 0o500
	if os.Getuid() == 0 {
		restrictive = 0o555
	}

	require.NoError(t, os.MkdirAll(filepath.Join(stateRoot, "trust"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(stateRoot, "trust", "trust-record.yaml"), []byte("not: [valid"), 0o600))
	eval, err := RecordLocalRecipeTrust(repoRoot, stateRoot, rec)
	require.Error(t, err)
	require.Equal(t, TrustStatusBlocked, eval.Status)
	requireTrustDiagnosticCodes(t, eval.Diagnostics, "trust.record.corrupt")

	stateRoot = t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(stateRoot, "trust"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(stateRoot, "trust", "trust-record.yaml"), []byte("schema: dotfiles-manager.v2.trust-record\nschemaVersion: 1\n"), 0o600))
	eval, err = RecordLocalRecipeTrust(repoRoot, stateRoot, rec)
	require.NoError(t, err)
	require.Equal(t, TrustStatusTrusted, eval.Status)

	blockedState := filepath.Join(t.TempDir(), "state-file")
	require.NoError(t, os.WriteFile(blockedState, []byte("x"), 0o600))
	eval, err = RecordLocalRecipeTrust(repoRoot, blockedState, rec)
	require.Error(t, err)
	require.Equal(t, TrustStatusBlocked, eval.Status)

	noWriteState := t.TempDir()
	require.NoError(t, os.Chmod(noWriteState, os.FileMode(restrictive)))
	t.Cleanup(func() { _ = os.Chmod(noWriteState, 0o700) })
	_, _ = RecordLocalRecipeTrust(repoRoot, noWriteState, rec)
}

func TestLocalRecipeTrustMissingRecordForDifferentTarget(t *testing.T) {
	t.Parallel()

	rec := decodeTrustTestRecipe(t)
	other := decodeTrustTestRecipeFromBody(t, strings.Replace(trustTestRecipeBody(), "target: test.json", "target: other.test", 1))
	repoRoot := t.TempDir()
	stateRoot := t.TempDir()
	_, err := RecordLocalRecipeTrust(repoRoot, stateRoot, other)
	require.NoError(t, err)

	eval, err := EvaluateRecipeTrust(repoRoot, stateRoot, RecipeSourceLocal, rec)
	require.NoError(t, err)
	require.Equal(t, TrustStatusReviewRequired, eval.Status)
	requireTrustDiagnosticCodes(t, eval.Diagnostics, "trust.local.missingRecord")
}

func TestLocalTrustEvidencePrivateMismatchBranches(t *testing.T) {
	t.Parallel()

	rec := decodeTrustTestRecipe(t)
	baseEvidence := &localTrustEvidence{
		status:             TrustStatusTrusted,
		source:             RecipeSourceLocal,
		target:             rec.Target,
		schemaVersion:      rec.SchemaVersion,
		contentSHA256:      "wrong",
		writeSurfaceSHA256: "wrong",
	}
	err := rec.ValidateWriteSafety(WriteSafetyContext{Source: RecipeSourceLocal, Trusted: true, localTrustEvidence: baseEvidence})
	require.Error(t, err)
	requireDiagnosticCodes(t, ValidationDiagnostics(err), "writeSafety.trust.recipeChanged", "writeSafety.trust.writeSurfaceChanged")

	wrongIdentity := *baseEvidence
	wrongIdentity.status = TrustStatusReviewRequired
	wrongIdentity.source = RecipeSourceBundled
	wrongIdentity.target = "other"
	wrongIdentity.schemaVersion = 999
	err = rec.ValidateWriteSafety(WriteSafetyContext{Source: RecipeSourceLocal, Trusted: true, localTrustEvidence: &wrongIdentity})
	require.Error(t, err)
	requireDiagnosticCodes(t, ValidationDiagnostics(err), "writeSafety.trust.evidenceInvalid", "writeSafety.trust.evidenceMismatch")
}

func TestTrustRecordValidationBranches(t *testing.T) {
	t.Parallel()

	rec := decodeTrustTestRecipe(t)
	surface, surfaceHash, err := RecipeWriteSurface(rec)
	require.NoError(t, err)
	record := TrustRecord{
		Schema:        "wrong",
		SchemaVersion: 999,
		LocalRecipes: map[string]LocalRecipeTrustRecord{
			"Bad Target": {
				Source:             "remote",
				Target:             "mismatch",
				SchemaVersion:      999,
				WriteSurfaceSHA256: "wrong",
				WriteSurface:       surface,
			},
			rec.Target: {
				Source:             RecipeSourceLocal,
				Target:             rec.Target,
				SchemaVersion:      SupportedVersion,
				ContentSHA256:      "hash",
				WriteSurfaceSHA256: surfaceHash,
				WriteSurface:       surface,
			},
		},
	}
	err = validateTrustRecord(record)
	require.Error(t, err)
	requireDiagnosticCodes(t, ValidationDiagnostics(err),
		"trust.record.schema.invalid",
		"trust.record.schemaVersion.invalid",
		"trust.record.target.invalid",
		"trust.record.source.invalid",
		"trust.record.target.mismatch",
		"trust.record.recipeSchemaVersion.invalid",
		"trust.record.contentSHA256.required",
		"trust.record.writeSurfaceSHA256.mismatch",
	)
}

func TestTrustPathHelperBranches(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	stateRoot := t.TempDir()
	fileStateRoot := filepath.Join(t.TempDir(), "state-file")
	require.NoError(t, os.WriteFile(fileStateRoot, []byte("x"), 0o600))
	_, err := validateTrustRoots(repoRoot, fileStateRoot)
	require.Error(t, err)

	linkRoot := filepath.Join(t.TempDir(), "state-link")
	require.NoError(t, os.Symlink(t.TempDir(), linkRoot))
	_, err = validateTrustRoots(repoRoot, linkRoot)
	require.Error(t, err)

	require.Error(t, rejectSymlinksUnder(stateRoot, filepath.Join(repoRoot, "outside")))
	require.NoError(t, rejectSymlinksUnder(stateRoot, filepath.Join(stateRoot, "missing", "path")))
	require.True(t, isPathWithin(stateRoot, stateRoot))
	require.False(t, isPathWithin(stateRoot, repoRoot))
	require.NotPanics(t, func() { _ = mustCanonicalSHA256(TrustLocationSurface{ID: "config"}) })
	require.Nil(t, copySelector(nil))
}

func TestTrustRecordAtomicDefaultsAndHashHelperBranches(t *testing.T) {
	t.Parallel()

	paths, err := validateTrustRoots(t.TempDir(), t.TempDir())
	require.NoError(t, err)
	require.NoError(t, writeTrustRecordAtomic(paths, TrustRecord{}))
	record, err := readTrustRecord(paths.recordPath)
	require.NoError(t, err)
	require.Equal(t, TrustRecordSchema, record.Schema)
	require.Equal(t, TrustRecordVersion, record.SchemaVersion)
	require.Empty(t, record.LocalRecipes)

	surface := TrustWriteSurface{NativeOperations: TrustNativeOperationsSurface{Count: 1, Summary: "test"}}
	require.NotEmpty(t, writeSurfaceKeys(surface))
	require.False(t, writeSurfaceBroadened(surface, surface))

	_, err = canonicalSHA256(func() {})
	require.Error(t, err)
	require.Panics(t, func() { _ = mustCanonicalSHA256(func() {}) })
}

func decodeTrustTestRecipe(t *testing.T) *Recipe {
	t.Helper()
	return decodeTrustTestRecipeFromBody(t, trustTestRecipeBody())
}

func decodeTrustTestRecipeFromBody(t *testing.T, body string) *Recipe {
	t.Helper()
	rec, err := Decode("recipe.yaml", strings.NewReader(body))
	require.NoError(t, err)
	return rec
}

func recordAndEvaluateTrust(t *testing.T, repoRoot string, stateRoot string, rec *Recipe) TrustEvaluation {
	t.Helper()
	_, err := RecordLocalRecipeTrust(repoRoot, stateRoot, rec)
	require.NoError(t, err)
	eval, err := EvaluateRecipeTrust(repoRoot, stateRoot, RecipeSourceLocal, rec)
	require.NoError(t, err)
	require.Equalf(t, TrustStatusTrusted, eval.Status, "diagnostics: %#v", eval.Diagnostics)
	return eval
}

func trustTestRecipeBody() string {
	return writeSafeSelectedPathRecipe("test.json", JSONFileDriverID, "config.json")
}

func requireTrustDiagnosticCodes(t *testing.T, diagnostics []ValidationDiagnostic, want ...string) {
	t.Helper()
	seen := map[string]bool{}
	for _, diagnostic := range diagnostics {
		seen[diagnostic.Code] = true
	}
	for _, code := range want {
		require.Truef(t, seen[code], "expected diagnostic code %q in %#v", code, diagnostics)
	}
}
