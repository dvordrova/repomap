package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/localization"
	"github.com/dvordrova/repomap/internal/report"
)

func TestPresentationLocalizationHTTPCallMatrix(t *testing.T) {
	clearLLMEnv(t)
	repository := presentationLocalizationHTTPRepository(t)
	orientationJSON := presentationLocalizationHTTPOrientation(t)
	calls := &presentationLocalizationHTTPCalls{}

	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read provider request: %v", err)
			return
		}
		content, kind, err := presentationLocalizationHTTPResponse(
			body,
			orientationJSON,
		)
		if err != nil {
			t.Errorf("build provider response: %v", err)
			return
		}
		calls.add(kind)
		writer.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(writer).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{
				"role": "assistant", "content": string(content),
			}}},
			"usage": map[string]any{
				"prompt_tokens": 101, "completion_tokens": 23,
			},
		}); err != nil {
			t.Errorf("encode provider response: %v", err)
		}
	}))
	defer server.Close()

	t.Setenv("REPOMAP_LLM_ENDPOINT", server.URL)
	t.Setenv("REPOMAP_LLM_MODEL", "matrix-model")
	t.Setenv("REPOMAP_LLM_AUTH", "bearer")
	t.Setenv("REPOMAP_LLM_API_KEY", "matrix-test-key")
	t.Setenv("REPOMAP_LLM_TIMEOUT", "5s")

	missBefore := calls.snapshot()
	missRunDir, missDebugDir := runPresentationLocalizationHTTPMatrix(
		t,
		repository,
		"ru",
		false,
	)
	miss := calls.snapshot().minus(missBefore)
	if miss.orientation != 1 || miss.localization != 1 {
		t.Fatalf(
			"RU cache-miss HTTP calls = %#v, want one orientation and one localization",
			miss,
		)
	}
	assertPresentationLocalizationHTTPSucceeded(t, missRunDir, false)

	// The prompt-only client must be able to identify the cache entry before
	// constructing the live bearer client. A valid hit therefore works with
	// no API key and makes no HTTP request at all.
	t.Setenv("REPOMAP_LLM_API_KEY", "")
	hitBefore := calls.snapshot()
	hit, err := localizePresentationForRun(
		context.Background(),
		missRunDir,
		filepath.Join(missDebugDir, presentationLocalizationCacheDir),
		false,
		io.Discard,
	)
	if err != nil {
		t.Fatalf("cache-hit localizePresentationForRun() error = %v", err)
	}
	if hit.State != report.PresentationLocalizationSucceeded || !hit.CacheHit {
		t.Fatalf("cache-hit outcome without API key = %#v", hit)
	}
	if hitCalls := calls.snapshot().minus(hitBefore); hitCalls != (presentationLocalizationHTTPCallCount{}) {
		t.Fatalf("RU cache-hit HTTP calls = %#v, want zero", hitCalls)
	}

	t.Setenv("REPOMAP_LLM_API_KEY", "matrix-test-key")
	noCacheBefore := calls.snapshot()
	noCacheRunDir, _ := runPresentationLocalizationHTTPMatrix(
		t,
		repository,
		"ru",
		true,
	)
	noCacheCalls := calls.snapshot().minus(noCacheBefore)
	if noCacheCalls.orientation != 1 || noCacheCalls.localization != 1 {
		t.Fatalf(
			"RU --no-cache HTTP calls = %#v, want one orientation and one localization",
			noCacheCalls,
		)
	}
	assertPresentationLocalizationHTTPSucceeded(t, noCacheRunDir, false)

	englishBefore := calls.snapshot()
	englishRunDir, _ := runPresentationLocalizationHTTPMatrix(
		t,
		repository,
		"en",
		true,
	)
	englishCalls := calls.snapshot().minus(englishBefore)
	if englishCalls.orientation != 1 || englishCalls.localization != 0 {
		t.Fatalf(
			"EN HTTP calls = %#v, want one orientation and no localization",
			englishCalls,
		)
	}
	if _, err := os.Stat(filepath.Join(
		englishRunDir,
		report.PresentationLocalizationStatusFile,
	)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("EN run wrote presentation localization status: %v", err)
	}
}

type presentationLocalizationHTTPCallCount struct {
	orientation  int
	localization int
	other        int
}

func (count presentationLocalizationHTTPCallCount) minus(
	before presentationLocalizationHTTPCallCount,
) presentationLocalizationHTTPCallCount {
	return presentationLocalizationHTTPCallCount{
		orientation:  count.orientation - before.orientation,
		localization: count.localization - before.localization,
		other:        count.other - before.other,
	}
}

type presentationLocalizationHTTPCalls struct {
	mu    sync.Mutex
	count presentationLocalizationHTTPCallCount
}

func (calls *presentationLocalizationHTTPCalls) add(kind string) {
	calls.mu.Lock()
	defer calls.mu.Unlock()
	switch kind {
	case "orientation":
		calls.count.orientation++
	case "localization":
		calls.count.localization++
	default:
		calls.count.other++
	}
}

func (calls *presentationLocalizationHTTPCalls) snapshot() presentationLocalizationHTTPCallCount {
	calls.mu.Lock()
	defer calls.mu.Unlock()
	return calls.count
}

func presentationLocalizationHTTPResponse(
	body,
	orientationJSON []byte,
) ([]byte, string, error) {
	var request struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		return nil, "", err
	}
	if len(request.Messages) != 2 {
		return []byte(`{}`), "other", nil
	}
	system := request.Messages[0].Content
	switch {
	case strings.Contains(
		system,
		"You translate a complete bounded presentation inventory",
	):
		const marker = "localization_input:\n"
		user := request.Messages[1].Content
		offset := strings.Index(user, marker)
		if offset < 0 {
			return nil, "", io.ErrUnexpectedEOF
		}
		var input localization.Input
		if err := json.Unmarshal([]byte(user[offset+len(marker):]), &input); err != nil {
			return nil, "", err
		}
		if input.SourceLocale != localization.LocaleEnglish ||
			input.TargetLocale != localization.LocaleRussian {
			return nil, "", errors.New("unexpected localization direction")
		}
		translations := make(map[string]string, len(input.Fields))
		for _, field := range input.Fields {
			translated := "Полностью переведённое описание"
			for _, placeholder := range field.Placeholders {
				for count := 0; count < placeholder.Count; count++ {
					translated += " " + placeholder.Token
				}
			}
			translations[field.ID] = translated
		}
		encoded, err := json.Marshal(localization.Projection{
			Version:         localization.ProjectionVersion,
			CanonicalSHA256: input.CanonicalSHA256,
			Locale:          localization.LocaleRussian,
			Translations:    translations,
		})
		return encoded, "localization", err
	case strings.Contains(system, "senior software engineer helping orient"):
		return bytes.Clone(orientationJSON), "orientation", nil
	default:
		// Optional architecture/Study stages are intentionally irrelevant to
		// this matrix. Valid JSON lets their normal fail-soft paths finish
		// while the test counts semantic and localization transport separately.
		return []byte(`{}`), "other", nil
	}
}

func presentationLocalizationHTTPRepository(t *testing.T) string {
	t.Helper()
	repository := t.TempDir()
	writeFile(
		t,
		filepath.Join(repository, "go.mod"),
		"module example.com/localization-matrix\n\ngo 1.24\n",
	)
	writeFile(
		t,
		filepath.Join(repository, "main.go"),
		"package main\n\nfunc main() {}\n",
	)
	runGit(t, repository, "init", "--quiet")
	runGit(t, repository, "add", "--", "go.mod", "main.go")
	commitTestRepository(t, repository)
	return repository
}

func presentationLocalizationHTTPOrientation(t *testing.T) []byte {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{
		"project_guess": "tiny localization matrix command",
		"confidence":    0.9,
		"high_level_map": []any{map[string]any{
			"name":           "command",
			"evidence":       []string{"main.go"},
			"why_it_matters": "it owns process startup",
		}},
		"first_files_to_open": []any{map[string]any{
			"path":   "main.go",
			"reason": "process entrypoint",
		}},
		"candidate_flows": []any{map[string]any{
			"name":              "Process startup",
			"trigger":           "the executable starts",
			"likely_entrypoint": "main.go",
			"likely_files":      []string{"main.go"},
			"why_interesting":   "shows the complete command behavior",
			"evidence":          []string{"main.go"},
			"confidence":        0.9,
		}},
		"important_domain_words": []any{},
		"questions_for_human":    []any{},
		"unverified_paths":       []any{},
		"warnings":               []any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func runPresentationLocalizationHTTPMatrix(
	t *testing.T,
	repository,
	language string,
	noCache bool,
) (string, string) {
	t.Helper()
	debugDir := t.TempDir()
	args := []string{
		"--debug-dir", debugDir,
		"--discover-surfaces=false",
		"--guided-tour=false",
		"--lang", language,
		"--no-open",
		"--no-serve",
	}
	if noCache {
		args = append(args, "--no-cache")
	}
	var stderr bytes.Buffer
	if err := runDefaultWithDeps(repository, args, defaultRunDeps{
		ctx: context.Background(), stdout: io.Discard, stderr: &stderr,
	}); err != nil {
		t.Fatalf(
			"runDefaultWithDeps(--lang %s, no-cache=%v) error = %v\nstderr:\n%s",
			language,
			noCache,
			err,
			stderr.String(),
		)
	}
	runDir, err := filepath.EvalSymlinks(filepath.Join(debugDir, "latest"))
	if err != nil {
		t.Fatal(err)
	}
	return runDir, debugDir
}

func assertPresentationLocalizationHTTPSucceeded(
	t *testing.T,
	runDir string,
	wantCacheHit bool,
) {
	t.Helper()
	canonical, err := report.ReadRunDir(runDir)
	if err != nil {
		t.Fatal(err)
	}
	projected, status := report.LoadPresentationLocalization(
		runDir,
		canonical,
		localization.LocaleRussian,
	)
	if status.State != report.PresentationLocalizationSucceeded ||
		status.CacheHit != wantCacheHit ||
		projected.ReportLanguage != localization.LocaleRussian {
		t.Fatalf("localized presentation/status = %#v/%#v", projected, status)
	}
}
