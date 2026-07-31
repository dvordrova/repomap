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
	const wantPromptSHA256 = "da4a36c6cb0f036bffbad49cc082a92c0fe7c05226631342c03287d3eefb28e4"
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
	response, err := os.ReadFile(filepath.Join(
		"testdata",
		"architecture-localization",
		"architecture.ru.projection.v1.json",
	))
	if err != nil {
		t.Fatal(err)
	}
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
	want, err := ReplayArchitectureLocalizationRussian(runDir, response)
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
	responsePath := filepath.Join(
		"testdata",
		"architecture-localization",
		"architecture.ru.projection.v1.json",
	)
	got, err := ReplayArchitectureLocalizationRussianStageFile(
		context.Background(),
		runDir,
		responsePath,
	)
	if err != nil {
		t.Fatal(err)
	}
	want, err := ReplayArchitectureLocalizationRussianFile(runDir, responsePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("file-backed stage differs from direct replay:\n%s\n%s", got, want)
	}

	link := filepath.Join(t.TempDir(), "response.json")
	if err := os.Symlink(filepath.Join("..", responsePath), link); err != nil {
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
	if err == nil || !strings.Contains(err.Error(), "strict JSON") || calls != 1 {
		t.Fatalf("escaped unknown field error = %v, calls = %d", err, calls)
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "api_key") {
		t.Fatalf("escaped unknown field leaked through stage error: %v", err)
	}
}

func TestReplayArchitectureLocalizationRussianStagePreservesFieldFallback(t *testing.T) {
	t.Parallel()

	runDir := architectureLocalizationSavedRun(t, false)
	_, canonical, input := architectureLocalizationRussianContext(t, runDir)
	projection := architectureLocalizationRussianProjection(t, canonical, input)
	delete(projection.Translations, input.Fields[0].ID)
	response, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	got, err := replayArchitectureLocalizationRussianStage(
		context.Background(),
		runDir,
		func(context.Context, localization.Prompt) ([]byte, error) {
			return append([]byte(nil), response...), nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	want, err := ReplayArchitectureLocalizationRussian(runDir, response)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("stage fallback differs from direct replay:\n%s\n%s", got, want)
	}
	var replay ArchitectureLocalizationReplay
	if err := decodeArchitectureLocalizationJSON(got, &replay); err != nil {
		t.Fatal(err)
	}
	if !replay.Fallback ||
		len(replay.Diagnostics) != 1 ||
		replay.Diagnostics[0] != (localization.Diagnostic{
			Code:    "missing_translation",
			FieldID: input.Fields[0].ID,
		}) {
		t.Fatalf("field fallback = %#v", replay)
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

	response, readErr := os.ReadFile(filepath.Join(
		"testdata",
		"architecture-localization",
		"architecture.ru.projection.v1.json",
	))
	if readErr != nil {
		t.Fatal(readErr)
	}
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
