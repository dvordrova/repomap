package report

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/localization"
	"github.com/dvordrova/repomap/internal/secretscan"
)

func TestArchitectureLocalizationRecordMissDoesNotWrite(t *testing.T) {
	t.Parallel()

	runDir := architectureLocalizationSavedRun(t, false)
	before := architectureLocalizationReplayRunSnapshot(t, runDir)
	got, err := replayArchitectureLocalizationRussianRecord(
		context.Background(),
		runDir,
		architectureLocalizationRecordRequestBuilder(
			"https://gateway.example.test/v1/chat/completions",
			"fixture-model",
			4096,
		),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	result := decodeArchitectureLocalizationRecordResult(t, got)
	if result.Status != ArchitectureLocalizationRecordMiss ||
		result.Replay != nil ||
		result.ProjectionSHA256 != "" ||
		result.Key == "" ||
		result.RecordPath == "" ||
		result.RequestSHA256 == "" {
		t.Fatalf("miss result = %#v", result)
	}
	after := architectureLocalizationReplayRunSnapshot(t, runDir)
	if !equalArchitectureLocalizationByteMap(before, after) {
		t.Fatalf("lookup-only miss changed run: before=%v after=%v", before, after)
	}
}

func TestArchitectureLocalizationRecordStoresThenHitsWithoutResponse(t *testing.T) {
	t.Parallel()

	runDir := architectureLocalizationSavedRun(t, false)
	response := architectureLocalizationRecordFixtureResponse(t)
	builder := architectureLocalizationRecordRequestBuilder(
		"https://gateway.example.test/v1/chat/completions",
		"fixture-model",
		4096,
	)
	var calls atomic.Int32
	storedJSON, err := replayArchitectureLocalizationRussianRecord(
		context.Background(),
		runDir,
		builder,
		func(context.Context) ([]byte, error) {
			calls.Add(1)
			return append([]byte(nil), response...), nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	stored := decodeArchitectureLocalizationRecordResult(t, storedJSON)
	if stored.Status != ArchitectureLocalizationRecordStored ||
		stored.Replay == nil ||
		stored.Replay.Fallback ||
		stored.Replay.Locale != localization.LocaleRussian ||
		calls.Load() != 1 {
		t.Fatalf("stored result = %#v, calls = %d", stored, calls.Load())
	}
	recordPath := filepath.Join(runDir, filepath.FromSlash(stored.RecordPath))
	before, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(recordPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("record permissions = %o, want 600", info.Mode().Perm())
	}

	hitJSON, err := replayArchitectureLocalizationRussianRecord(
		context.Background(),
		runDir,
		builder,
		func(context.Context) ([]byte, error) {
			calls.Add(1)
			return nil, errors.New("response callback must not run on hit")
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	hit := decodeArchitectureLocalizationRecordResult(t, hitJSON)
	if hit.Status != ArchitectureLocalizationRecordHit ||
		hit.Replay == nil ||
		calls.Load() != 1 ||
		hit.Key != stored.Key ||
		hit.RequestSHA256 != stored.RequestSHA256 ||
		hit.ProjectionSHA256 != stored.ProjectionSHA256 ||
		!reflectArchitectureLocalizationReplayEqual(*hit.Replay, *stored.Replay) {
		t.Fatalf("hit = %#v, stored = %#v, calls = %d", hit, stored, calls.Load())
	}
	after, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("exact hit rewrote immutable record bytes")
	}
	architectureLocalizationAssertRecordFiles(t, runDir, 1)
}

func TestArchitectureLocalizationRecordRejectsResponseWithoutRecord(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		response func(*testing.T, string) []byte
		want     string
	}{
		{
			name: "malformed",
			response: func(*testing.T, string) []byte {
				return []byte(`{"version":1`)
			},
			want: "strict JSON",
		},
		{
			name: "field fallback",
			response: func(t *testing.T, runDir string) []byte {
				_, canonical, input := architectureLocalizationRussianContext(t, runDir)
				projection := architectureLocalizationRussianProjection(t, canonical, input)
				delete(projection.Translations, input.Fields[0].ID)
				encoded, err := json.Marshal(projection)
				if err != nil {
					t.Fatal(err)
				}
				return encoded
			},
			want: "not fully accepted",
		},
		{
			name: "provider error",
			response: func(*testing.T, string) []byte {
				return nil
			},
			want: "saved response unavailable",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			runDir := architectureLocalizationSavedRun(t, false)
			before := architectureLocalizationReplayRunSnapshot(t, runDir)
			calls := 0
			_, err := replayArchitectureLocalizationRussianRecord(
				context.Background(),
				runDir,
				architectureLocalizationRecordRequestBuilder(
					"https://gateway.example.test/v1/chat/completions",
					"fixture-model",
					4096,
				),
				func(context.Context) ([]byte, error) {
					calls++
					if test.name == "provider error" {
						return nil, errors.New(`api_key="must-not-leak"`)
					}
					return test.response(t, runDir), nil
				},
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
			if strings.Contains(err.Error(), "must-not-leak") {
				t.Fatalf("provider error leaked response detail: %v", err)
			}
			if calls != 1 {
				t.Fatalf("response calls = %d, want 1", calls)
			}
			after := architectureLocalizationReplayRunSnapshot(t, runDir)
			if !equalArchitectureLocalizationByteMap(before, after) {
				t.Fatalf("rejected response changed run: before=%v after=%v", before, after)
			}
			architectureLocalizationAssertRecordFiles(t, runDir, 0)
		})
	}
}

func TestArchitectureLocalizationRecordRejectsCredentialWhenSecretScanDisabled(t *testing.T) {
	restore := secretscan.SetDisabled(true)
	t.Cleanup(restore)

	runDir := architectureLocalizationSavedRun(t, false)
	before := architectureLocalizationReplayRunSnapshot(t, runDir)
	_, canonical, input := architectureLocalizationRussianContext(t, runDir)
	projection := architectureLocalizationRussianProjection(t, canonical, input)
	const credential = `api_key="company-secret-localization-record"`
	for id := range projection.Translations {
		projection.Translations[id] = credential
		break
	}
	response, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	_, err = replayArchitectureLocalizationRussianRecord(
		context.Background(),
		runDir,
		architectureLocalizationRecordRequestBuilder(
			"https://gateway.example.test/v1/chat/completions",
			"fixture-model",
			4096,
		),
		func(context.Context) ([]byte, error) {
			calls++
			return response, nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "obvious credential") {
		t.Fatalf("error = %v, want obvious credential rejection", err)
	}
	if strings.Contains(err.Error(), credential) {
		t.Fatalf("credential rejection echoed sensitive value: %v", err)
	}
	if calls != 1 {
		t.Fatalf("response calls = %d, want 1", calls)
	}
	after := architectureLocalizationReplayRunSnapshot(t, runDir)
	if !equalArchitectureLocalizationByteMap(before, after) {
		t.Fatalf("credential response changed run: before=%v after=%v", before, after)
	}
	architectureLocalizationAssertRecordFiles(t, runDir, 0)
}

func TestArchitectureLocalizationRecordIdentityVariationMissesCleanly(t *testing.T) {
	t.Parallel()

	runDir := architectureLocalizationSavedRun(t, false)
	response := architectureLocalizationRecordFixtureResponse(t)
	base := architectureLocalizationRecordRequestBuilder(
		"https://gateway.example.test/v1/chat/completions",
		"fixture-model",
		4096,
	)
	storedJSON, err := replayArchitectureLocalizationRussianRecord(
		context.Background(),
		runDir,
		base,
		func(context.Context) ([]byte, error) {
			return append([]byte(nil), response...), nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	stored := decodeArchitectureLocalizationRecordResult(t, storedJSON)

	bodyVariant := func(prompt localization.Prompt) (deepseek.LocalizationRequestEvidence, error) {
		request, buildErr := architectureLocalizationRecordRequestBuilder(
			"https://gateway.example.test/v1/chat/completions",
			"fixture-model",
			4096,
		)(prompt)
		request.Body = append(request.Body, ' ')
		return request, buildErr
	}
	tests := []struct {
		name    string
		builder architectureLocalizationRequestBuilder
	}{
		{
			name: "endpoint",
			builder: architectureLocalizationRecordRequestBuilder(
				"https://other.example.test/v1/chat/completions",
				"fixture-model",
				4096,
			),
		},
		{
			name: "model",
			builder: architectureLocalizationRecordRequestBuilder(
				"https://gateway.example.test/v1/chat/completions",
				"other-model",
				4096,
			),
		},
		{
			name: "generation",
			builder: architectureLocalizationRecordRequestBuilder(
				"https://gateway.example.test/v1/chat/completions",
				"fixture-model",
				8192,
			),
		},
		{name: "request bytes", builder: bodyVariant},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			got, lookupErr := replayArchitectureLocalizationRussianRecord(
				context.Background(),
				runDir,
				test.builder,
				nil,
			)
			if lookupErr != nil {
				t.Fatal(lookupErr)
			}
			result := decodeArchitectureLocalizationRecordResult(t, got)
			if result.Status != ArchitectureLocalizationRecordMiss ||
				result.Key == stored.Key ||
				result.Replay != nil {
				t.Fatalf("identity variation result = %#v, stored = %#v", result, stored)
			}
		})
	}
	architectureLocalizationAssertRecordFiles(t, runDir, 1)
}

func TestArchitectureLocalizationRecordIdentityAcceptsSharedRequestBudget(t *testing.T) {
	t.Parallel()

	runDir := architectureLocalizationSavedRun(t, false)
	prepared, err := prepareArchitectureLocalizationRussian(runDir)
	if err != nil {
		t.Fatal(err)
	}
	prompt, promptJSON := largestArchitectureLocalizationRecordPrompt(t)
	request, err := architectureLocalizationRecordRequestBuilder(
		"https://gateway.example.test/v1/chat/completions",
		"fixture-model",
		4096,
	)(prompt)
	if err != nil {
		t.Fatal(err)
	}
	if len(request.Body) <= maxArchitectureLocalizationArtifactBytes {
		t.Fatalf(
			"request bytes = %d, want above former artifact cap %d",
			len(request.Body),
			maxArchitectureLocalizationArtifactBytes,
		)
	}
	if _, err := buildArchitectureLocalizationProjectionRecordIdentity(
		prepared,
		promptJSON,
		request,
	); err != nil {
		t.Fatalf("shared request budget was rejected: %v", err)
	}
}

func TestArchitectureLocalizationRecordRejectsCorruptSymlinkAndTamper(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*testing.T, string)
		want   string
	}{
		{
			name: "corrupt",
			mutate: func(t *testing.T, path string) {
				if err := os.WriteFile(path, []byte(`{"version":1`), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			want: "not strict JSON",
		},
		{
			name: "tampered projection hash",
			mutate: func(t *testing.T, path string) {
				var record ArchitectureLocalizationProjectionRecord
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(data, &record); err != nil {
					t.Fatal(err)
				}
				record.ProjectionSHA256 = strings.Repeat("0", 64)
				encoded, err := json.Marshal(record)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, append(encoded, '\n'), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			want: "projection hash mismatch",
		},
		{
			name: "alternate signed zero identity",
			mutate: func(t *testing.T, path string) {
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				data = bytes.Replace(
					data,
					[]byte(`"temperature":0`),
					[]byte(`"temperature":-0`),
					1,
				)
				if err := os.WriteFile(path, data, 0o600); err != nil {
					t.Fatal(err)
				}
			},
			want: "identity mismatch",
		},
		{
			name: "duplicate outer field",
			mutate: func(t *testing.T, path string) {
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				data = bytes.Replace(
					data,
					[]byte(`{"version":1,`),
					[]byte(`{"version":1,"version":1,`),
					1,
				)
				if err := os.WriteFile(path, data, 0o600); err != nil {
					t.Fatal(err)
				}
			},
			want: "not canonical JSON",
		},
		{
			name: "symlink",
			mutate: func(t *testing.T, path string) {
				target := filepath.Join(t.TempDir(), "target.json")
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(target, data, 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			},
			want: "not a bounded regular file",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			runDir := architectureLocalizationSavedRun(t, false)
			builder := architectureLocalizationRecordRequestBuilder(
				"https://gateway.example.test/v1/chat/completions",
				"fixture-model",
				4096,
			)
			storedJSON, err := replayArchitectureLocalizationRussianRecord(
				context.Background(),
				runDir,
				builder,
				func(context.Context) ([]byte, error) {
					return architectureLocalizationRecordFixtureResponse(t), nil
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			stored := decodeArchitectureLocalizationRecordResult(t, storedJSON)
			recordPath := filepath.Join(runDir, filepath.FromSlash(stored.RecordPath))
			test.mutate(t, recordPath)

			calls := 0
			_, err = replayArchitectureLocalizationRussianRecord(
				context.Background(),
				runDir,
				builder,
				func(context.Context) ([]byte, error) {
					calls++
					return architectureLocalizationRecordFixtureResponse(t), nil
				},
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
			if calls != 0 {
				t.Fatalf("unsafe record invoked response callback %d time(s)", calls)
			}
		})
	}
}

func TestArchitectureLocalizationRecordRejectsReplacedRunRoot(t *testing.T) {
	t.Parallel()

	runDir := architectureLocalizationSavedRun(t, false)
	builder := architectureLocalizationRecordRequestBuilder(
		"https://gateway.example.test/v1/chat/completions",
		"fixture-model",
		4096,
	)
	prepared, err := prepareArchitectureLocalizationRecord(runDir, builder)
	if err != nil {
		t.Fatal(err)
	}
	moved := runDir + "-moved"
	if err := os.Rename(runDir, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(runDir, 0o700); err != nil {
		t.Fatal(err)
	}

	if _, _, err := loadArchitectureLocalizationProjectionRecord(prepared); err == nil ||
		!strings.Contains(err.Error(), "run root identity mismatch") {
		t.Fatalf("replaced run root error = %v", err)
	}
	architectureLocalizationAssertRecordFiles(t, runDir, 0)
}

func TestArchitectureLocalizationRecordCancellationDoesNotPublish(t *testing.T) {
	t.Parallel()

	builder := architectureLocalizationRecordRequestBuilder(
		"https://gateway.example.test/v1/chat/completions",
		"fixture-model",
		4096,
	)
	t.Run("before lookup", func(t *testing.T) {
		runDir := architectureLocalizationSavedRun(t, false)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		calls := 0
		_, err := replayArchitectureLocalizationRussianRecord(
			ctx,
			runDir,
			builder,
			func(context.Context) ([]byte, error) {
				calls++
				return architectureLocalizationRecordFixtureResponse(t), nil
			},
		)
		if !errors.Is(err, context.Canceled) || calls != 0 {
			t.Fatalf("error = %v, calls = %d", err, calls)
		}
		architectureLocalizationAssertRecordFiles(t, runDir, 0)
	})
	t.Run("after response", func(t *testing.T) {
		runDir := architectureLocalizationSavedRun(t, false)
		ctx, cancel := context.WithCancel(context.Background())
		calls := 0
		_, err := replayArchitectureLocalizationRussianRecord(
			ctx,
			runDir,
			builder,
			func(context.Context) ([]byte, error) {
				calls++
				cancel()
				return architectureLocalizationRecordFixtureResponse(t), nil
			},
		)
		if !errors.Is(err, context.Canceled) || calls != 1 {
			t.Fatalf("error = %v, calls = %d", err, calls)
		}
		architectureLocalizationAssertRecordFiles(t, runDir, 0)
	})
}

func TestArchitectureLocalizationRecordConcurrentSameKeyPublishesOneWinner(t *testing.T) {
	t.Parallel()

	runDir := architectureLocalizationSavedRun(t, false)
	response := architectureLocalizationRecordFixtureResponse(t)
	builder := architectureLocalizationRecordRequestBuilder(
		"https://gateway.example.test/v1/chat/completions",
		"fixture-model",
		4096,
	)
	var ready sync.WaitGroup
	ready.Add(2)
	release := make(chan struct{})
	type outcome struct {
		result ArchitectureLocalizationRecordResult
		err    error
	}
	outcomes := make(chan outcome, 2)
	for index := 0; index < 2; index++ {
		go func() {
			got, err := replayArchitectureLocalizationRussianRecord(
				context.Background(),
				runDir,
				builder,
				func(context.Context) ([]byte, error) {
					ready.Done()
					<-release
					return append([]byte(nil), response...), nil
				},
			)
			if err != nil {
				outcomes <- outcome{err: err}
				return
			}
			var result ArchitectureLocalizationRecordResult
			if decodeErr := json.Unmarshal(got, &result); decodeErr != nil {
				outcomes <- outcome{err: decodeErr}
				return
			}
			outcomes <- outcome{result: result}
		}()
	}
	ready.Wait()
	close(release)

	results := make([]ArchitectureLocalizationRecordResult, 0, 2)
	for index := 0; index < 2; index++ {
		got := <-outcomes
		if got.err != nil {
			t.Fatal(got.err)
		}
		results = append(results, got.result)
	}
	statuses := []string{results[0].Status, results[1].Status}
	sort.Strings(statuses)
	if !equalArchitectureLocalizationStrings(
		statuses,
		[]string{ArchitectureLocalizationRecordHit, ArchitectureLocalizationRecordStored},
	) {
		t.Fatalf("concurrent statuses = %v", statuses)
	}
	if results[0].Key != results[1].Key ||
		results[0].ProjectionSHA256 != results[1].ProjectionSHA256 ||
		results[0].Replay == nil ||
		results[1].Replay == nil ||
		!reflectArchitectureLocalizationReplayEqual(*results[0].Replay, *results[1].Replay) {
		t.Fatalf("concurrent results differ: %#v / %#v", results[0], results[1])
	}
	architectureLocalizationAssertRecordFiles(t, runDir, 1)
}

func architectureLocalizationRecordRequestBuilder(
	endpoint,
	model string,
	maxTokens int,
) architectureLocalizationRequestBuilder {
	client := &deepseek.Client{
		Endpoint:  endpoint,
		Auth:      "none",
		Model:     model,
		MaxTokens: maxTokens,
	}
	return client.BuildLocalizationRequest
}

func architectureLocalizationRecordFixtureResponse(t *testing.T) []byte {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(
		"testdata",
		"architecture-localization",
		"architecture.ru.projection.v1.json",
	))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func decodeArchitectureLocalizationRecordResult(
	t *testing.T,
	data []byte,
) ArchitectureLocalizationRecordResult {
	t.Helper()

	var result ArchitectureLocalizationRecordResult
	if err := decodeArchitectureLocalizationJSON(data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func architectureLocalizationAssertRecordFiles(
	t *testing.T,
	runDir string,
	want int,
) {
	t.Helper()

	root := filepath.Join(
		runDir,
		architectureLocalizationRecordRoot,
		architectureLocalizationRecordVersionDir,
	)
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		if want != 0 {
			t.Fatalf("record directory is absent, want %d record(s)", want)
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	records := 0
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Fatalf("usable temporary record remains: %q", entry.Name())
		}
		if strings.HasSuffix(entry.Name(), ".json") {
			records++
		}
	}
	if records != want {
		t.Fatalf("record files = %d, want %d; entries=%v", records, want, entries)
	}
}

func reflectArchitectureLocalizationReplayEqual(
	left,
	right ArchitectureLocalizationReplay,
) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftJSON, rightJSON)
}

func equalArchitectureLocalizationStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func largestArchitectureLocalizationRecordPrompt(
	t *testing.T,
) (localization.Prompt, []byte) {
	t.Helper()

	low, high := 1, 1<<20
	var best localization.Prompt
	var bestJSON []byte
	for low <= high {
		middle := low + (high-low)/2
		candidate := localization.Prompt{
			Version: localization.PromptVersion,
			System:  "system",
			User:    strings.Repeat("x", middle),
		}
		encoded, err := localization.MarshalPrompt(candidate)
		if err != nil {
			high = middle - 1
			continue
		}
		best = candidate
		bestJSON = encoded
		low = middle + 1
	}
	if len(bestJSON) == 0 {
		t.Fatal("no valid localization prompt found")
	}
	return best, bestJSON
}
