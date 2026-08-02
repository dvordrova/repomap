package deepseek

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/tasklens"
)

func TestDefaultTaskInvestigationConfigIsStableAndEnvironmentIndependent(t *testing.T) {
	t.Setenv("REPOMAP_LLM_MODEL", "must-not-change-skipped-synthesis-identity")
	config := DefaultTaskInvestigationConfig()
	if config.Model != "deepseek-v4-flash" ||
		config.Endpoint != "https://api.deepseek.com/chat/completions" ||
		config.AuthMode != authBearer ||
		config.Timeout != defaultTimeout ||
		config.MaxTokens != defaultMaxTokens {
		t.Fatalf("default Task Investigation config = %#v", config)
	}
}

func TestTaskInvestigationPromptUsesOneJSONThinkingRequestAndHidesDisplayName(t *testing.T) {
	bundle := taskInvestigationFixtureBundle(t)
	client := &Client{
		HTTPClient: &http.Client{}, Endpoint: "https://api.deepseek.com/chat/completions",
		Model: "deepseek-v4-flash", MaxTokens: 7000, Auth: authBearer,
	}
	raw, err := client.TaskInvestigationPromptJSON(bundle)
	if err != nil {
		t.Fatal(err)
	}
	var request chatRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		t.Fatal(err)
	}
	if request.ResponseFormat == nil || request.ResponseFormat.Type != "json_object" ||
		request.Thinking == nil || request.Thinking.Type != "enabled" ||
		request.ReasoningEffort != tasklens.SynthesisThinkingProfile || request.Temperature != nil {
		t.Fatalf("request envelope = %#v", request)
	}
	if request.MaxTokens != client.MaxTokens {
		t.Fatalf("max_tokens = %d", request.MaxTokens)
	}
	if string(raw) == "" || containsJSONText(raw, "task-labelled-checkout") {
		t.Fatalf("provider request leaked display-only checkout name: %s", raw)
	}
}

func TestInvestigateTaskMeasuredRetriesRetryableTransportFailures(t *testing.T) {
	tests := []struct {
		name      string
		newClient func(t *testing.T, calls *int) *Client
	}{
		{
			name: "http 503",
			newClient: func(t *testing.T, calls *int) *Client {
				t.Helper()
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					*calls++
					if *calls == 1 {
						w.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					writeTaskInvestigationResponse(t, w, `{"selected_anchor_ids":[]}`)
				}))
				t.Cleanup(server.Close)
				return taskInvestigationTestClient(server.Client(), server.URL)
			},
		},
		{
			name: "network error",
			newClient: func(t *testing.T, calls *int) *Client {
				t.Helper()
				httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
					*calls++
					if *calls == 1 {
						return nil, errors.New("temporary transport failure")
					}
					return taskInvestigationHTTPResponse(
						request,
						http.StatusOK,
						taskInvestigationResponseBody(`{"selected_anchor_ids":[]}`),
					), nil
				})}
				return taskInvestigationTestClient(httpClient, "https://provider.invalid/chat")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			client := test.newClient(t, &calls)
			result, err := client.InvestigateTaskMeasured(t.Context(), taskInvestigationFixtureBundle(t))
			if err != nil {
				t.Fatal(err)
			}
			if calls != 2 {
				t.Fatalf("transport calls = %d, want 2", calls)
			}
			if result.Attempts != 2 {
				t.Fatalf("Attempts = %d, want 2", result.Attempts)
			}
			if string(result.Content) != `{"selected_anchor_ids":[]}` {
				t.Fatalf("Content = %q", result.Content)
			}
			if result.InputTokens != 11 || result.OutputTokens != 7 ||
				result.PromptCacheHitTokens != 3 || result.PromptCacheMissTokens != 8 ||
				!result.UsageReported {
				t.Fatalf("token usage = %#v", result)
			}
		})
	}
}

func TestInvestigateTaskMeasuredDoesNotRetryNonRetryableResponse(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"invalid request"}`)
	}))
	defer server.Close()

	client := taskInvestigationTestClient(server.Client(), server.URL)
	result, err := client.InvestigateTaskMeasured(t.Context(), taskInvestigationFixtureBundle(t))
	if err == nil || !strings.Contains(err.Error(), "status 400") {
		t.Fatalf("error = %v, want status 400", err)
	}
	if calls != 1 {
		t.Fatalf("transport calls = %d, want 1", calls)
	}
	if result.Attempts != 1 {
		t.Fatalf("Attempts = %d, want 1", result.Attempts)
	}
}

func TestInvestigateTaskMeasuredReturnsSubstantiveResponseWithoutSemanticRetry(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		writeTaskInvestigationResponse(t, w, "not a task lens proposal")
	}))
	defer server.Close()

	client := taskInvestigationTestClient(server.Client(), server.URL)
	result, err := client.InvestigateTaskMeasured(t.Context(), taskInvestigationFixtureBundle(t))
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || result.Attempts != 1 {
		t.Fatalf("calls = %d, result = %#v", calls, result)
	}
	if string(result.Content) != "not a task lens proposal" {
		t.Fatalf("Content = %q", result.Content)
	}
}

func TestInvestigateTaskMeasuredStopsBeforeRetryWhenContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	calls := 0
	httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		cancel()
		return taskInvestigationHTTPResponse(request, http.StatusServiceUnavailable, ""), nil
	})}
	client := taskInvestigationTestClient(httpClient, "https://provider.invalid/chat")

	result, err := client.InvestigateTaskMeasured(ctx, taskInvestigationFixtureBundle(t))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if calls != 1 {
		t.Fatalf("transport calls = %d, want 1", calls)
	}
	if result.Attempts != 1 {
		t.Fatalf("Attempts = %d, want 1", result.Attempts)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func taskInvestigationTestClient(httpClient *http.Client, endpoint string) *Client {
	return &Client{
		HTTPClient: httpClient,
		Model:      "test-model",
		MaxTokens:  4000,
		Endpoint:   endpoint,
		Auth:       authNone,
	}
}

func writeTaskInvestigationResponse(t *testing.T, writer http.ResponseWriter, content string) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if _, err := io.WriteString(writer, taskInvestigationResponseBody(content)); err != nil {
		t.Errorf("write response: %v", err)
	}
}

func taskInvestigationResponseBody(content string) string {
	encoded, _ := json.Marshal(content)
	return `{"choices":[{"message":{"content":` + string(encoded) +
		`}}],"usage":{"prompt_tokens":11,"completion_tokens":7,` +
		`"prompt_cache_hit_tokens":3,"prompt_cache_miss_tokens":8}}`
}

func taskInvestigationHTTPResponse(request *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}

func taskInvestigationFixtureBundle(t *testing.T) tasklens.Bundle {
	t.Helper()
	const taskText = "Enabled is ignored."
	const stateSHA = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	repository := tasklens.Repository{
		Identity: "example.com/fixture", DisplayName: "task-labelled-checkout",
		Revision:       "0123456789012345678901234567890123456789",
		TreeHash:       "1123456789012345678901234567890123456789",
		StateSHA256:    stateSHA,
		IdentitySource: "remote",
	}
	taskEvidence := tasklens.OpaqueID("evidence", "task", tasklens.SHA256([]byte(taskText)))
	anchors := make([]tasklens.Anchor, 0, 3)
	evidence := []tasklens.Evidence{{
		ID: taskEvidence, Kind: tasklens.EvidenceTaskProvided,
		Summary: "Symptom or requested outcome supplied by the task; not repository truth.",
	}}
	for index, symbol := range []string{"Config", "CopyConfig", "TestCopyConfig"} {
		line := index + 1
		lineText := strconv.Itoa(line)
		filePath := "config.go"
		if index == 2 {
			filePath = "config_test.go"
		}
		excerpt := []tasklens.SourceLine{{Line: line, Text: "func " + symbol + "() {}"}}
		excerptSHA := tasklens.SourceExcerptSHA256(excerpt)
		id := tasklens.OpaqueID("anchor", filePath, symbol, lineText, lineText, excerptSHA)
		evidenceID := tasklens.OpaqueID("evidence", stateSHA, filePath, lineText, lineText, excerptSHA)
		anchors = append(anchors, tasklens.Anchor{
			ID: id, Path: filePath, Symbol: symbol, Package: "fixture",
			StartLine: line, EndLine: line,
			Excerpt: excerpt,
			Scope: tasklens.SourceScope{
				ScopeKind:             tasklens.SourceScopeCompleteEnclosingSymbol,
				ScopeStart:            line,
				ScopeEnd:              line,
				SourceTotalLines:      line,
				NegativeClaimsAllowed: true,
				NegativeEvidenceBasis: tasklens.NegativeEvidenceCompleteScope,
			},
			RoleHints:   []tasklens.AnchorRole{tasklens.RoleRepresentativeImplementation},
			EvidenceIDs: []string{evidenceID},
		})
		evidence = append(evidence, tasklens.Evidence{
			ID: evidenceID, Kind: tasklens.EvidenceRepositoryFact, Path: filePath,
			StartLine: line, EndLine: line, AnchorID: id,
			Summary: "Exact repository source excerpt for " + symbol + " at " + filePath + ":" + lineText + "-" + lineText + ".",
		})
	}
	kindHint, observableHint := tasklens.GroundedTaskClassification(taskText)
	for index := range anchors {
		anchors[index].RoleHints = tasklens.GroundedAnchorRoles(anchors[index], kindHint, taskText)
	}
	bundle := tasklens.Bundle{
		Version: tasklens.BundleVersion,
		ID: tasklens.OpaqueID(
			"task", repository.Identity, repository.Revision, repository.StateSHA256,
			tasklens.SHA256([]byte(taskText)),
		),
		Repository: repository,
		Task:       tasklens.Task{Text: taskText, EvidenceID: taskEvidence},
		KindHint:   kindHint, ObservableHint: observableHint,
		Terms:   tasklens.GroundedTaskTerms(taskText, anchors),
		Anchors: anchors, Evidence: evidence,
		AllowedPaths:  []string{"config.go", "config_test.go"},
		StagesSkipped: tasklens.CanonicalStagesSkipped(),
		Budgets: tasklens.Budgets{
			InitialCandidates: 2, CandidateItemsFound: 2,
			RetainedAnchors: 3, AnchorItemsFound: 3, EvidenceFilesConsidered: 2,
			ReadFiles: 2, ReadBytes: 100,
		},
		Metrics: tasklens.RetrievalMetrics{TrackedFiles: 2, EvidenceFilesRead: 2},
	}
	bundle.Relations = tasklens.GroundedRelations(bundle.Anchors, bundle.Terms)
	bundle.Budgets.RetainedSourceBytes = tasklens.RetainedSourceByteCount(bundle.Anchors)
	bundle.Locality = tasklens.GroundedLocality(
		taskText, bundle.Terms, bundle.Anchors, bundle.Relations,
	)
	bundle.Metrics.RelationsRetained = len(bundle.Relations)
	if err := tasklens.GroundV01Contract(&bundle); err != nil {
		t.Fatal(err)
	}
	trace, err := tasklens.GroundedSelectedRetrievalTrace(bundle)
	if err != nil {
		t.Fatal(err)
	}
	bundle.LocalTrace = trace
	if err := bundle.Validate(); err != nil {
		t.Fatal(err)
	}
	return bundle
}

func containsJSONText(raw []byte, value string) bool {
	var decoded any
	if json.Unmarshal(raw, &decoded) != nil {
		return false
	}
	encoded, _ := json.Marshal(decoded)
	return string(encoded) != "" && stringContains(string(encoded), value)
}

func stringContains(value, needle string) bool {
	for index := 0; index+len(needle) <= len(value); index++ {
		if value[index:index+len(needle)] == needle {
			return true
		}
	}
	return false
}
