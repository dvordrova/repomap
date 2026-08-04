package deepseek

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/dvordrova/repomap/internal/modelresearch"
)

func TestResearchKeepsBoundedRetrySemanticsThroughSharedTransport(t *testing.T) {
	t.Parallel()

	prompt := modelresearch.Prompt{
		Version: modelresearch.PromptVersion,
		System:  "return valid json only",
		User:    "return one json object",
	}
	client := &Client{
		Endpoint:  "https://provider.example.test/v1/chat/completions",
		Auth:      authNone,
		Model:     "research-model",
		MaxTokens: 128,
	}
	wantBody, err := client.BuildResearchRequest(prompt)
	if err != nil {
		t.Fatal(err)
	}

	var calls int
	var requestBodies [][]byte
	client.HTTPClient = &http.Client{Transport: localizationRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			return nil, err
		}
		requestBodies = append(requestBodies, append([]byte(nil), body...))
		calls++
		if calls == 1 {
			return localizationHTTPResponse(request, http.StatusServiceUnavailable, ""), nil
		}
		return localizationHTTPResponse(
			request,
			http.StatusOK,
			localizationProviderEnvelope(`{}`),
		), nil
	})}

	result, err := client.Research(t.Context(), prompt)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || result.Attempts != 2 || string(result.Content) != `{}` {
		t.Fatalf("calls/result = %d/%#v", calls, result)
	}
	for index, body := range requestBodies {
		if !bytes.Equal(body, wantBody) {
			t.Fatalf("request body %d changed\ngot:  %s\nwant: %s", index, body, wantBody)
		}
	}
}
