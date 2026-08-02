package deepseek

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/modelresearch"
)

func atlasStudyPromptFixture() atlasstudy.Prompt {
	return atlasstudy.Prompt{
		Version: atlasstudy.PromptVersion,
		System:  "Use only the supplied short typed refs and return JSON.",
		User:    `Requested prose language: en. Catalog JSON: {"components":[{"ref":"c0001"}]}`,
	}
}

func TestAtlasStudyPromptJSONUsesOneExactJSONRequest(t *testing.T) {
	t.Parallel()
	client := &Client{
		Endpoint: "https://api.deepseek.com/chat/completions",
		Model:    "deepseek-v4-flash", MaxTokens: 64_000,
	}
	body, err := client.AtlasStudyPromptJSON(atlasStudyPromptFixture(), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	var request chatRequest
	if err := json.Unmarshal(body, &request); err != nil {
		t.Fatal(err)
	}
	if request.Model != client.Model || request.MaxTokens != client.MaxTokens ||
		request.ResponseFormat == nil || request.ResponseFormat.Type != "json_object" ||
		request.Thinking == nil || request.Thinking.Type != "disabled" || len(request.Messages) != 2 {
		t.Fatalf("Atlas Study request = %#v", request)
	}
	if !strings.Contains(request.Messages[0].Content, "short typed refs") ||
		!strings.Contains(request.Messages[1].Content, `"c0001"`) ||
		!strings.Contains(request.Messages[1].Content, canonicalEnglishUserContract) {
		t.Fatalf("Atlas Study request lost exact prompt contracts: %#v", request.Messages)
	}
}

func TestAtlasStudyPromptJSONEnforcesFinalProviderBodyBudget(t *testing.T) {
	t.Parallel()
	client := &Client{Model: "fixture-model", MaxTokens: 64_000}
	body, err := client.AtlasStudyPromptJSON(atlasStudyPromptFixture(), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.AtlasStudyPromptJSON(atlasStudyPromptFixture(), len(body)-1)
	var limitErr *modelresearch.ResourceLimitError
	if !errors.As(err, &limitErr) || limitErr.Stage != "atlas_study" ||
		limitErr.Kind != modelresearch.ResourceLimitRequestBytes ||
		limitErr.Limit != len(body)-1 || limitErr.Observed != len(body) ||
		!limitErr.ObservedKnown || limitErr.ConfiguredMaxTokens != client.MaxTokens {
		t.Fatalf("Atlas Study budget error = %#v / %v", limitErr, err)
	}
}

func TestAtlasStudyMeasuredRetriesOnlyImmutableTransport(t *testing.T) {
	prompt := atlasStudyPromptFixture()
	client := &Client{
		Endpoint: "https://provider.example.test/v1/chat/completions", Auth: authNone,
		Model: "fixture-model", MaxTokens: 64_000,
	}
	wantBody, err := client.AtlasStudyPromptJSON(prompt, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	response := `{"repository_type":"library_framework","brief":{},"directions":[]}`
	var bodies [][]byte
	client.HTTPClient = &http.Client{Transport: localizationRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			return nil, err
		}
		bodies = append(bodies, append([]byte(nil), body...))
		if len(bodies) == 1 {
			return localizationHTTPResponse(request, http.StatusServiceUnavailable, `{"error":"busy"}`), nil
		}
		return localizationHTTPResponse(request, http.StatusOK, localizationProviderEnvelope(response)), nil
	})}
	result, err := client.AtlasStudyMeasured(t.Context(), prompt, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if result.Attempts != 2 || string(result.Content) != response || len(bodies) != 2 {
		t.Fatalf("Atlas Study calls/result = %d/%#v", len(bodies), result)
	}
	for index, body := range bodies {
		if !bytes.Equal(body, wantBody) {
			t.Fatalf("transport retry mutated body %d", index)
		}
	}
}

func TestAtlasStudyOutputLengthIsTypedTerminalWithoutSemanticRetry(t *testing.T) {
	t.Parallel()
	var calls int
	client := &Client{
		Endpoint: "https://provider.example.test/v1/chat/completions", Auth: authNone,
		Model: "fixture-model", MaxTokens: 64_000,
		HTTPClient: &http.Client{Transport: localizationRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			calls++
			content, _ := json.Marshal(`{"repository_type":"library_framework"}`)
			body := `{"choices":[{"finish_reason":"length","message":{"content":` + string(content) +
				`}}],"usage":{"prompt_tokens":100,"completion_tokens":64000}}`
			return localizationHTTPResponse(request, http.StatusOK, body), nil
		})},
	}
	result, err := client.AtlasStudyMeasured(t.Context(), atlasStudyPromptFixture(), 1<<20)
	var limitErr *modelresearch.ResourceLimitError
	if !errors.As(err, &limitErr) || calls != 1 || result.Attempts != 1 ||
		limitErr.Stage != "atlas_study" || limitErr.Kind != modelresearch.ResourceLimitOutputTokens ||
		limitErr.Limit != client.MaxTokens || limitErr.FinishReason != "length" {
		t.Fatalf("Atlas Study terminal result/error = calls %d / %#v / %#v / %v", calls, result, limitErr, err)
	}
}
