package report

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/localization"
)

func TestBuildArchitectureLocalizationRussianPromptIsExactAndReadOnly(t *testing.T) {
	t.Parallel()

	runDir := architectureLocalizationSavedRun(t, false)
	before := architectureLocalizationReplayRunSnapshot(t, runDir)
	first, err := BuildArchitectureLocalizationRussianPrompt(runDir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildArchitectureLocalizationRussianPrompt(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("identical prompt builds differ:\n%s\n%s", first, second)
	}
	after := architectureLocalizationReplayRunSnapshot(t, runDir)
	if !equalArchitectureLocalizationByteMap(before, after) {
		t.Fatalf("prompt preview changed its run directory: before=%v after=%v", before, after)
	}

	var prompt localization.Prompt
	if err := decodeArchitectureLocalizationJSON(first, &prompt); err != nil {
		t.Fatal(err)
	}
	prepared, err := prepareArchitectureLocalizationRussian(runDir)
	if err != nil {
		t.Fatal(err)
	}
	wantPrompt, err := localization.BuildRussianPrompt(prepared.canonical, prepared.input)
	if err != nil {
		t.Fatal(err)
	}
	want, err := localization.MarshalPrompt(wantPrompt)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, want) {
		t.Fatalf("stage prompt does not match localization contract:\n%s\n%s", first, want)
	}
	digest := sha256.Sum256(first)
	// Decision 231 (Archive 9): shared participation + prompt v18 flow
	// into the landscape contract, so the prompt SHA changes (v18).
	// Decision 235 (v11): member-only grammar (prompt v19) — SHA changes.
	// D238 adds bounded primary-scope candidate context and a member-only
	// primary-before-supporting response instruction.
	// D239 makes the embedded Architecture contract and coverage semantics
	// production-aware.
	const wantPromptSHA256 = "ea4c9c74b3b3593c1313ba42a1745e5d565961ea1f87cb01a2eecc638ef51cd0"
	if got := hex.EncodeToString(digest[:]); got != wantPromptSHA256 {
		t.Fatalf("Architecture prompt SHA-256 = %q, want %q", got, wantPromptSHA256)
	}
}

func TestReplayArchitectureLocalizationRussianStageCallsOnceAndMatchesDirectReplay(t *testing.T) {
	t.Parallel()

	runDir := architectureLocalizationSavedRun(t, false)
	before := architectureLocalizationReplayRunSnapshot(t, runDir)
	wantPrompt, err := BuildArchitectureLocalizationRussianPrompt(runDir)
	if err != nil {
		t.Fatal(err)
	}
	_, canonical, input := architectureLocalizationRussianContext(t, runDir)
	projection := architectureLocalizationRussianProjection(t, canonical, input)
	projectionJSON := architectureLocalizationRussianProjectionJSON(t, canonical, input)
	response := architectureLocalizationProviderResponse(t, canonical, input, projection)
	calls := 0
	got, err := replayArchitectureLocalizationRussianStage(
		context.Background(),
		runDir,
		func(_ context.Context, prompt localization.Prompt) ([]byte, error) {
			calls++
			encoded, encodeErr := localization.MarshalPrompt(prompt)
			if encodeErr != nil {
				return nil, encodeErr
			}
			if !bytes.Equal(encoded, wantPrompt) {
				t.Fatalf("provider prompt differs:\n%s\n%s", encoded, wantPrompt)
			}
			return append([]byte(nil), response...), nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("provider calls = %d, want 1", calls)
	}
	want, err := ReplayArchitectureLocalizationRussian(runDir, projectionJSON)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("stage replay differs from direct replay:\n%s\n%s", got, want)
	}
	after := architectureLocalizationReplayRunSnapshot(t, runDir)
	if !equalArchitectureLocalizationByteMap(before, after) {
		t.Fatalf("stage replay changed its run directory: before=%v after=%v", before, after)
	}
}

func TestReplayArchitectureLocalizationRussianStageFileIsProviderFree(t *testing.T) {
	t.Parallel()

	runDir := architectureLocalizationSavedRun(t, false)
	_, canonical, input := architectureLocalizationRussianContext(t, runDir)
	projectionJSON := architectureLocalizationRussianProjectionJSON(t, canonical, input)
	projectionPath := filepath.Join(t.TempDir(), "architecture.ru.projection.v1.json")
	if err := os.WriteFile(projectionPath, projectionJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	projection := architectureLocalizationRussianProjection(t, canonical, input)
	responsePath := filepath.Join(t.TempDir(), "provider-response.json")
	if err := os.WriteFile(
		responsePath,
		architectureLocalizationProviderResponse(t, canonical, input, projection),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	got, err := ReplayArchitectureLocalizationRussianStageFile(
		context.Background(),
		runDir,
		responsePath,
	)
	if err != nil {
		t.Fatal(err)
	}
	want, err := ReplayArchitectureLocalizationRussianFile(runDir, projectionPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("file-backed stage differs from direct replay:\n%s\n%s", got, want)
	}

	link := filepath.Join(t.TempDir(), "response.json")
	if err := os.Symlink(responsePath, link); err != nil {
		t.Fatal(err)
	}
	if _, err := ReplayArchitectureLocalizationRussianStageFile(
		context.Background(),
		runDir,
		link,
	); err == nil {
		t.Fatal("symlinked saved response unexpectedly reached the stage")
	}
}

func TestReplayArchitectureLocalizationRussianStageDoesNotRetryRejectedResponse(t *testing.T) {
	t.Parallel()

	runDir := architectureLocalizationSavedRun(t, false)
	calls := 0
	_, err := replayArchitectureLocalizationRussianStage(
		context.Background(),
		runDir,
		func(context.Context, localization.Prompt) ([]byte, error) {
			calls++
			return []byte(`{"version":1`), nil
		},
	)
	if err == nil || calls != 1 {
		t.Fatalf("malformed response error = %v, calls = %d", err, calls)
	}

	calls = 0
	_, err = replayArchitectureLocalizationRussianStage(
		context.Background(),
		runDir,
		func(context.Context, localization.Prompt) ([]byte, error) {
			calls++
			return bytes.Repeat(
				[]byte("x"),
				maxArchitectureLocalizationArtifactBytes+1,
			), nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "byte limit") || calls != 1 {
		t.Fatalf("oversize response error = %v, calls = %d", err, calls)
	}
}

func TestReplayArchitectureLocalizationRussianStageDoesNotEchoEscapedUnknownField(t *testing.T) {
	t.Parallel()

	runDir := architectureLocalizationSavedRun(t, false)
	const secret = `company-secret-value-123456789`
	response := []byte(
		`{"version":1,"canonical_sha256":"x","locale":"ru","translations":{},` +
			`"\u0061pi_key=\u0022` + secret + `\u0022":true}`,
	)
	calls := 0
	_, err := replayArchitectureLocalizationRussianStage(
		context.Background(),
		runDir,
		func(context.Context, localization.Prompt) ([]byte, error) {
			calls++
			return append([]byte(nil), response...), nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "invalid provider response") || calls != 1 {
		t.Fatalf("escaped unknown field error = %v, calls = %d", err, calls)
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "api_key") {
		t.Fatalf("escaped unknown field leaked through stage error: %v", err)
	}
}

func TestReplayArchitectureLocalizationRussianStageRejectsIncompleteProviderResponse(t *testing.T) {
	t.Parallel()

	runDir := architectureLocalizationSavedRun(t, false)
	_, canonical, input := architectureLocalizationRussianContext(t, runDir)
	projection := architectureLocalizationRussianProjection(t, canonical, input)
	response := architectureLocalizationProviderResponse(t, canonical, input, projection)
	var providerResponse localization.ProviderResponse
	if err := json.Unmarshal(response, &providerResponse); err != nil {
		t.Fatal(err)
	}
	providerResponse.Translations = providerResponse.Translations[:len(providerResponse.Translations)-1]
	response, err := json.Marshal(providerResponse)
	if err != nil {
		t.Fatal(err)
	}
	_, err = replayArchitectureLocalizationRussianStage(
		context.Background(),
		runDir,
		func(context.Context, localization.Prompt) ([]byte, error) {
			return append([]byte(nil), response...), nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "invalid provider response") {
		t.Fatalf("incomplete provider response error = %v", err)
	}
}

func TestReplayArchitectureLocalizationRussianStageCancellationAndSafeError(t *testing.T) {
	t.Parallel()

	runDir := architectureLocalizationSavedRun(t, false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	_, err := replayArchitectureLocalizationRussianStage(
		ctx,
		runDir,
		func(context.Context, localization.Prompt) ([]byte, error) {
			calls++
			return nil, nil
		},
	)
	if !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatalf("pre-canceled stage error = %v, calls = %d", err, calls)
	}

	_, canonical, input := architectureLocalizationRussianContext(t, runDir)
	projection := architectureLocalizationRussianProjection(t, canonical, input)
	response := architectureLocalizationProviderResponse(t, canonical, input, projection)
	ctx, cancel = context.WithCancel(context.Background())
	_, err = replayArchitectureLocalizationRussianStage(
		ctx,
		runDir,
		func(context.Context, localization.Prompt) ([]byte, error) {
			cancel()
			return response, nil
		},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled successful response error = %v", err)
	}

	const secret = `api_key="company-secret-localization-stage"`
	_, err = replayArchitectureLocalizationRussianStage(
		context.Background(),
		runDir,
		func(context.Context, localization.Prompt) ([]byte, error) {
			return nil, errors.New(secret)
		},
	)
	if err == nil ||
		strings.Contains(err.Error(), secret) ||
		!strings.Contains(err.Error(), "provider response unavailable") {
		t.Fatalf("unsafe provider error = %v", err)
	}
}
