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

	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/localization"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/reportserver"
)

func TestAtlasFirstOrdinaryRunDoesNotCallLegacyPresentationLocalization(t *testing.T) {
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
			t,
			body,
			orientationJSON,
		)
		if err != nil {
			t.Errorf("build provider response: %v", err)
			return
		}
		if kind == "localization" {
			assertNoAuthorizedPublicationArtifacts(
				t,
				calls.currentDebugDir(),
			)
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

	for _, language := range []string{"ru", "en"} {
		before := calls.snapshot()
		debugDir := t.TempDir()
		calls.setCurrentDebugDir(debugDir)
		runDir, _ := runPresentationLocalizationHTTPMatrix(
			t, repository, language, true, debugDir,
		)
		got := calls.snapshot().minus(before)
		if got != (presentationLocalizationHTTPCallCount{other: 1}) {
			t.Fatalf(
				"%s Atlas-first HTTP calls = %#v, want one Architecture request and no raw Orientation/localization",
				language, got,
			)
		}
		if _, err := os.Stat(filepath.Join(runDir, report.PresentationLocalizationStatusFile)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s Atlas-first run wrote legacy presentation localization status: %v", language, err)
		}
	}
}

func TestSemanticResourceLimitStopsBeforeAuthorizedPublication(t *testing.T) {
	clearLLMEnv(t)
	repository := presentationLocalizationHTTPRepository(t)
	orientationJSON := presentationLocalizationHTTPOrientation(t)

	for _, test := range []struct {
		name           string
		terminalStage  string
		wantErrorStage string
	}{
		{
			name:           "representative earlier optional architecture stage",
			terminalStage:  "architecture",
			wantErrorStage: "architecture_synthesis",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
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
					t,
					body,
					orientationJSON,
				)
				if err != nil {
					t.Errorf("build provider response: %v", err)
					return
				}
				if bytes.Contains(
					body,
					[]byte("compact conceptual architecture landscape"),
				) {
					kind = "architecture"
				}
				calls.add(kind)
				finishReason := "stop"
				if kind == test.terminalStage {
					finishReason = "length"
				}
				writer.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(writer).Encode(map[string]any{
					"choices": []any{map[string]any{
						"finish_reason": finishReason,
						"message": map[string]any{
							"role": "assistant", "content": string(content),
						},
					}},
					"usage": map[string]any{
						"prompt_tokens": 101, "completion_tokens": 23,
					},
				}); err != nil {
					t.Errorf("encode provider response: %v", err)
				}
			}))
			defer server.Close()

			t.Setenv("REPOMAP_LLM_ENDPOINT", server.URL)
			t.Setenv("REPOMAP_LLM_MODEL", "resource-proof-model")
			t.Setenv("REPOMAP_LLM_AUTH", "none")
			t.Setenv("REPOMAP_LLM_TIMEOUT", "5s")

			debugDir := t.TempDir()
			var opened, served int
			var stderr bytes.Buffer
			err := runDefaultWithDeps(repository, []string{
				"--debug-dir", debugDir,
				"--discover-surfaces=false",
				"--lang", "ru",
				"--no-cache",
			}, defaultRunDeps{
				ctx: context.Background(), stdout: io.Discard, stderr: &stderr,
				openReport: func(string) error {
					opened++
					return nil
				},
				serveReport: func(context.Context, reportserver.Options) error {
					served++
					return nil
				},
			})
			var limitErr *deepseek.ResourceLimitError
			if !errors.As(err, &limitErr) {
				t.Fatalf(
					"runDefaultWithDeps() error = %v, want ResourceLimitError\nstderr:\n%s",
					err,
					stderr.String(),
				)
			}
			if limitErr.Stage != test.wantErrorStage ||
				limitErr.Kind != deepseek.ResourceLimitOutputTokens ||
				limitErr.FinishReason != "length" {
				t.Fatalf("terminal ResourceLimitError = %#v", limitErr)
			}

			sequence := calls.sequence()
			if len(sequence) == 0 || sequence[len(sequence)-1] != test.terminalStage {
				t.Fatalf("semantic call sequence = %v, want final %q", sequence, test.terminalStage)
			}
			if test.terminalStage == "architecture" {
				if len(sequence) != 1 {
					t.Fatalf(
						"architecture-limit call sequence = %v, want [architecture]",
						sequence,
					)
				}
			}
			if opened != 0 || served != 0 {
				t.Fatalf("publication side effects: open=%d serve=%d", opened, served)
			}

			runDir := presentationLocalizationResourceRunDir(t, debugDir)
			for _, name := range []string{
				"report.json",
				"report.html",
				report.RunManifestFilename,
			} {
				if _, statErr := os.Lstat(filepath.Join(runDir, name)); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("terminal resource outcome published %s: %v", name, statErr)
				}
			}
			if _, statErr := os.Lstat(filepath.Join(debugDir, "latest")); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("terminal resource outcome linked latest: %v", statErr)
			}
		})
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
	mu             sync.Mutex
	count          presentationLocalizationHTTPCallCount
	order          []string
	activeDebugDir string
}

func (calls *presentationLocalizationHTTPCalls) add(kind string) {
	calls.mu.Lock()
	defer calls.mu.Unlock()
	calls.order = append(calls.order, kind)
	switch kind {
	case "orientation":
		calls.count.orientation++
	case "localization":
		calls.count.localization++
	default:
		calls.count.other++
	}
}

func (calls *presentationLocalizationHTTPCalls) sequence() []string {
	calls.mu.Lock()
	defer calls.mu.Unlock()
	return append([]string(nil), calls.order...)
}

func (calls *presentationLocalizationHTTPCalls) setCurrentDebugDir(debugDir string) {
	calls.mu.Lock()
	defer calls.mu.Unlock()
	calls.activeDebugDir = debugDir
}

func (calls *presentationLocalizationHTTPCalls) currentDebugDir() string {
	calls.mu.Lock()
	defer calls.mu.Unlock()
	return calls.activeDebugDir
}

func (calls *presentationLocalizationHTTPCalls) snapshot() presentationLocalizationHTTPCallCount {
	calls.mu.Lock()
	defer calls.mu.Unlock()
	return calls.count
}

func presentationLocalizationHTTPResponse(
	t *testing.T,
	body []byte,
	orientationFixture orientationResponseFixture,
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
		translations := make([]localization.ProviderTranslation, len(input.Fields))
		for index, field := range input.Fields {
			translated := "Полностью переведённое описание"
			for _, placeholder := range field.Placeholders {
				for count := 0; count < placeholder.Count; count++ {
					translated += " " + placeholder.Token
				}
			}
			translations[index] = localization.NewProviderTranslation(
				index,
				translated,
			)
		}
		encoded, err := json.Marshal(localization.ProviderResponse{
			Version:         localization.ProviderResponseVersion,
			CanonicalSHA256: input.CanonicalSHA256,
			Locale:          localization.LocaleRussian,
			Translations:    translations,
		})
		return encoded, "localization", err
	case strings.Contains(system, "senior software engineer helping orient"):
		return orientationResponseForRequest(t, body, orientationFixture), "orientation", nil
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

func presentationLocalizationResourceRunDir(t *testing.T, debugDir string) string {
	t.Helper()
	entries, err := os.ReadDir(debugDir)
	if err != nil {
		t.Fatal(err)
	}
	var runDirs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidate := filepath.Join(debugDir, entry.Name())
		if _, err := os.Stat(filepath.Join(candidate, "metadata.json")); err == nil {
			runDirs = append(runDirs, candidate)
		}
	}
	if len(runDirs) != 1 {
		t.Fatalf("artifact run directories = %v, want exactly one", runDirs)
	}
	return runDirs[0]
}

func presentationLocalizationHTTPOrientation(t *testing.T) orientationResponseFixture {
	t.Helper()
	return orientationResponseFixture{
		ProjectGuess: "tiny localization matrix command", Confidence: 0.9,
		Map:        []orientationMapFixture{{Name: "command", Role: "entry", EvidencePath: "main.go", WhyItMatters: "it owns process startup"}},
		FirstFiles: []orientationFileFixture{{Path: "main.go", Reason: "process entrypoint"}},
		Flows: []orientationFlowFixture{{
			Name: "Process startup", Trigger: "the executable starts", EntrypointPath: "main.go",
			LikelyPaths: []string{"main.go"}, EvidencePaths: []string{"main.go"},
			WhyInteresting: "shows the complete command behavior", Confidence: 0.9,
		}},
		Warnings: []string{},
	}
}

func runPresentationLocalizationHTTPMatrix(
	t *testing.T,
	repository,
	language string,
	noCache bool,
	debugDir string,
) (string, string) {
	t.Helper()
	args := []string{
		"--debug-dir", debugDir,
		"--discover-surfaces=false",
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

func assertNoAuthorizedPublicationArtifacts(t *testing.T, debugDir string) {
	t.Helper()
	if strings.TrimSpace(debugDir) == "" {
		t.Fatal("missing active debug directory for publication boundary assertion")
	}
	runDir := presentationLocalizationResourceRunDir(t, debugDir)
	for _, name := range []string{
		"report.json",
		"report.html",
		report.RunManifestFilename,
	} {
		if _, err := os.Lstat(filepath.Join(runDir, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("authorized publication artifact %s exists before localization: %v", name, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(debugDir, "latest")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("latest exists before localization: %v", err)
	}
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
	prepared, err := report.PrepareRunPresentation(runDir, canonical, nil)
	if err != nil {
		t.Fatal(err)
	}
	projected, status := report.LoadPresentationLocalization(
		runDir,
		prepared,
		localization.LocaleRussian,
	)
	if status.State != report.PresentationLocalizationSucceeded ||
		status.CacheHit != wantCacheHit ||
		projected.ReportLanguage != localization.LocaleRussian {
		t.Fatalf("localized presentation/status = %#v/%#v", projected, status)
	}
}

func assertPresentationLocalizationRunMaxTokens(
	t *testing.T,
	runDir string,
	want int,
) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(runDir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	var metadata struct {
		MaxTokens int `json:"max_tokens"`
	}
	if err := json.Unmarshal(raw, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.MaxTokens != want {
		t.Fatalf("run metadata max_tokens = %d, want exact %d", metadata.MaxTokens, want)
	}
}
