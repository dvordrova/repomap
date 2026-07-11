package deepseek

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/sourcecard"
	"github.com/dvordrova/repomap/internal/sourceexplain"
)

func TestSourcePromptContainsGroundingContract(t *testing.T) {
	t.Parallel()

	client := &Client{Model: "deepseek-v4-flash", MaxTokens: 1000}
	requestJSON, err := client.SourcePromptJSON(sourceBundleJSON(t))
	if err != nil {
		t.Fatalf("SourcePromptJSON() error = %v", err)
	}
	var request chatRequest
	if err := json.Unmarshal(requestJSON, &request); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	if request.ResponseFormat == nil || request.ResponseFormat.Type != "json_object" {
		t.Fatalf("response format = %#v", request.ResponseFormat)
	}
	if len(request.Messages) != 2 {
		t.Fatalf("messages = %d", len(request.Messages))
	}
	prompt := request.Messages[0].Content + request.Messages[1].Content
	for _, fragment := range []string{
		"question's candidate_source_evidence_ids",
		"anchor_source_evidence_id",
		"lexical window",
		"Always include both",
		"one next_action_id",
		"source-4",
		"checkInput",
	} {
		if !strings.Contains(prompt, fragment) {
			t.Fatalf("prompt does not contain %q:\n%s", fragment, prompt)
		}
	}
}

func TestSourcePromptCanonicalizesUnknownFields(t *testing.T) {
	t.Parallel()

	bundleJSON := sourceBundleJSON(t)
	bundleJSON = []byte(strings.TrimSuffix(string(bundleJSON), "}") + `,"api_key":"must-not-survive"}`)
	requestJSON, err := (&Client{Model: "test"}).SourcePromptJSON(bundleJSON)
	if err != nil {
		t.Fatalf("SourcePromptJSON() error = %v", err)
	}
	if strings.Contains(string(requestJSON), "must-not-survive") || strings.Contains(string(requestJSON), "api_key") {
		t.Fatalf("request retained unknown sensitive field: %s", requestJSON)
	}
}

func TestSourcePromptRejectsInvalidOrUnsafeBundle(t *testing.T) {
	t.Parallel()

	client := &Client{Model: "test"}
	if _, err := client.SourcePromptJSON([]byte("not json")); err == nil {
		t.Fatal("invalid JSON error = nil")
	}

	bundle := sourceBundleFixture()
	bundle.Source.Lines[1].Text = `apiKey := "abcdefghijklmnop"`
	data, _ := json.Marshal(bundle)
	if _, err := client.SourcePromptJSON(data); err == nil {
		t.Fatal("unsafe source error = nil")
	}
}

func TestAssessSourceUsesTolerantResponseBoundary(t *testing.T) {
	t.Parallel()

	var received chatRequest
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer fixture-key" {
			t.Errorf("Authorization = %q", request.Header.Get("Authorization"))
		}
		body, _ := io.ReadAll(request.Body)
		if err := json.Unmarshal(body, &received); err != nil {
			t.Errorf("request body: %v", err)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message": map[string]any{"role": "assistant", "content": "weak non-json response"},
			}},
		})
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: server.Client(),
		APIKey:     "fixture-key",
		Model:      "deepseek-v4-flash",
		MaxTokens:  1000,
		Endpoint:   server.URL,
	}
	raw, err := client.AssessSource(context.Background(), sourceBundleJSON(t))
	if err != nil {
		t.Fatalf("AssessSource() error = %v", err)
	}
	if string(raw) != "weak non-json response" {
		t.Fatalf("raw = %q", raw)
	}
	if received.ResponseFormat == nil || received.ResponseFormat.Type != "json_object" {
		t.Fatalf("response format = %#v", received.ResponseFormat)
	}
}

func sourceBundleJSON(t *testing.T) []byte {
	t.Helper()
	data, err := json.Marshal(sourceBundleFixture())
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func sourceBundleFixture() sourceexplain.Bundle {
	target := sourcecard.Target{
		EvidenceID: "resolution-001",
		EntityID:   "target",
		Name:       "server.Work",
		Kind:       evidence.EntityMethod,
		Path:       "pkg/work.go",
		Line:       3,
	}
	return sourceexplain.Bundle{
		Version:  sourceexplain.BundleVersion,
		RepoName: "repo",
		Target:   target,
		Source: sourceexplain.Source{
			FileSHA256: strings.Repeat("a", 64),
			Window: sourcecard.Window{
				StartLine:     3,
				EndLine:       5,
				IncludedBytes: 82,
				StopReason:    sourcecard.StopNextTopLevelFunc,
			},
			Complete:   false,
			StopReason: sourcecard.StopNextTopLevelFunc,
			Lines: []sourcecard.Line{
				{EvidenceID: "source-3", Line: 3, Text: "func (s *server) Work() error {"},
				{EvidenceID: "source-4", Line: 4, Text: "\tif err := checkInput(); err != nil {"},
				{EvidenceID: "source-5", Line: 5, Text: "\t\treturn err"},
			},
		},
		Questions: []sourceexplain.Question{{
			ID:                         "question-call-out-001",
			Predicate:                  sourceexplain.PredicateValidatesInput,
			AnchorEvidenceID:           "call-out-001",
			AnchorSourceEvidenceID:     "source-4",
			CalleeName:                 "checkInput",
			CandidateSourceEvidenceIDs: []string{"source-4", "source-5"},
		}},
		AllowedActions: []sourceexplain.AllowedAction{{
			ID:               "action-find-tests",
			Operation:        sourceexplain.OperationFindTests,
			AnchorEvidenceID: target.EvidenceID,
		}},
		Warnings: []string{},
	}
}
