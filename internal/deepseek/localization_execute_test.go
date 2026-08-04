package deepseek

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/dvordrova/repomap/internal/localization"
)

func TestExecuteLocalizationRequestUsesExactEvidenceAndReturnsMetrics(t *testing.T) {
	t.Parallel()

	var receivedBody []byte
	var receivedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		var err error
		receivedBody, err = io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		receivedAuth = request.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"choices":[{"message":{"content":"{\"version\":\"localization-projection-json-v1\",\"fields\":[]}"}}],
			"usage":{
				"prompt_tokens":101,
				"completion_tokens":23,
				"prompt_cache_hit_tokens":80,
				"prompt_cache_miss_tokens":21
			}
		}`)
	}))
	defer server.Close()

	prompt := localization.Prompt{
		Version: localization.PromptVersion,
		System:  "translate exact fields",
		User:    "canonical English payload",
	}
	builder := &Client{
		Endpoint: server.URL, Auth: authBearer,
		Model: "translation-model", MaxTokens: 2048,
	}
	evidence, err := builder.BuildLocalizationRequest(prompt)
	if err != nil {
		t.Fatalf("BuildLocalizationRequest() error = %v", err)
	}
	executor := &Client{
		HTTPClient: server.Client(),
		APIKey:     "local-test-key",
		// Execution must use the provider identity already bound into evidence.
		Endpoint: "https://must-not-be-used.example.test",
		Auth:     authNone,
		Model:    "must-not-be-used",
	}
	result, err := executor.ExecuteLocalizationRequest(
		context.Background(),
		prompt,
		evidence,
	)
	if err != nil {
		t.Fatalf("ExecuteLocalizationRequest() error = %v", err)
	}
	if !bytes.Equal(receivedBody, evidence.Body) {
		t.Fatalf("request body changed\ngot:  %s\nwant: %s", receivedBody, evidence.Body)
	}
	if receivedAuth != "Bearer local-test-key" {
		t.Fatalf("Authorization = %q", receivedAuth)
	}
	if string(result.Content) != `{"version":"localization-projection-json-v1","fields":[]}` {
		t.Fatalf("Content = %s", result.Content)
	}
	if result.Attempts != 1 ||
		result.RequestBytes != len(evidence.Body) ||
		result.InputTokens != 101 ||
		result.OutputTokens != 23 ||
		result.PromptCacheHitTokens != 80 ||
		result.PromptCacheMissTokens != 21 {
		t.Fatalf("metrics = %#v", result)
	}
	body := string(receivedBody)
	if strings.Contains(body, canonicalEnglishSystemContract) ||
		strings.Contains(body, canonicalEnglishUserContract) {
		t.Fatalf("translation request contains semantic-English wrapper: %s", body)
	}
}

func TestExecuteLocalizationRequestRetriesRetryableTransportFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		first func(*http.Request) (*http.Response, error)
	}{
		{
			name: "connection reset",
			first: func(*http.Request) (*http.Response, error) {
				return nil, errors.New("connection reset by peer")
			},
		},
		{
			name: "http 429",
			first: func(request *http.Request) (*http.Response, error) {
				return localizationHTTPResponse(request, http.StatusTooManyRequests, `{"error":"busy"}`), nil
			},
		},
		{
			name: "http 503",
			first: func(request *http.Request) (*http.Response, error) {
				return localizationHTTPResponse(request, http.StatusServiceUnavailable, `{"error":"unavailable"}`), nil
			},
		},
		{
			name: "mid-body read error",
			first: func(request *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body: &localizationReadErrorBody{
						data: []byte(`{"choices":[`), err: io.ErrUnexpectedEOF,
					},
					Request: request,
				}, nil
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			prompt, evidence := localizationExecutionEvidence(t)
			var calls int
			var requestBodies [][]byte
			transport := localizationRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				body, err := io.ReadAll(request.Body)
				if err != nil {
					return nil, err
				}
				requestBodies = append(requestBodies, append([]byte(nil), body...))
				calls++
				if calls == 1 {
					return test.first(request)
				}
				return localizationHTTPResponse(
					request,
					http.StatusOK,
					localizationProviderEnvelope(`{}`),
				), nil
			})

			result, err := (&Client{
				HTTPClient: &http.Client{Transport: transport}, Auth: authNone,
			}).ExecuteLocalizationRequest(t.Context(), prompt, evidence)
			if err != nil {
				t.Fatalf("ExecuteLocalizationRequest() error = %v", err)
			}
			if calls != 2 || len(requestBodies) != 2 {
				t.Fatalf("transport calls/bodies = %d/%d, want 2/2", calls, len(requestBodies))
			}
			for index, body := range requestBodies {
				if !bytes.Equal(body, evidence.Body) {
					t.Fatalf("request body %d changed\ngot:  %s\nwant: %s", index, body, evidence.Body)
				}
			}
			if string(result.Content) != `{}` || result.Attempts != 2 ||
				result.RequestBytes != 2*len(evidence.Body) {
				t.Fatalf("result = %#v", result)
			}
		})
	}
}

func TestExecuteLocalizationRequestStopsBeforeRetryWhenCanceled(t *testing.T) {
	t.Parallel()

	prompt, evidence := localizationExecutionEvidence(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var calls int
	transport := localizationRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		cancel()
		return localizationHTTPResponse(request, http.StatusServiceUnavailable, ""), nil
	})

	result, err := (&Client{
		HTTPClient: &http.Client{Transport: transport}, Auth: authNone,
	}).ExecuteLocalizationRequest(ctx, prompt, evidence)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ExecuteLocalizationRequest() error = %v, want context.Canceled", err)
	}
	if calls != 1 || result.Attempts != 1 || result.RequestBytes != len(evidence.Body) {
		t.Fatalf("calls/result = %d/%#v, want one completed attempt", calls, result)
	}
}

func TestExecuteLocalizationRequestDoesNotRetryMalformedOrSemanticInvalidResponse(t *testing.T) {
	t.Parallel()

	t.Run("malformed response envelope", func(t *testing.T) {
		t.Parallel()
		prompt, evidence := localizationExecutionEvidence(t)
		var calls int
		transport := localizationRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			calls++
			return localizationHTTPResponse(request, http.StatusOK, `{"choices":[`), nil
		})
		result, err := (&Client{
			HTTPClient: &http.Client{Transport: transport}, Auth: authNone,
		}).ExecuteLocalizationRequest(t.Context(), prompt, evidence)
		if err == nil || !errors.Is(err, errResponseEnvelopeMalformed) {
			t.Fatalf("ExecuteLocalizationRequest() error = %v", err)
		}
		if calls != 1 || result.Attempts != 1 {
			t.Fatalf("calls/result = %d/%#v, want one attempt", calls, result)
		}
	})

	t.Run("semantic invalid projection", func(t *testing.T) {
		t.Parallel()
		canonical, err := localization.NewCanonical([]localization.FieldSpec{{
			OwnerKind: localization.OwnerRepository,
			OwnerID:   "repository",
			Name:      localization.FieldProjectGuess,
			Text:      "Repository guide",
		}})
		if err != nil {
			t.Fatal(err)
		}
		input, err := localization.BuildInput(canonical, localization.LocaleRussian)
		if err != nil {
			t.Fatal(err)
		}
		response, err := json.Marshal(localization.ProviderResponse{
			Version:         localization.ProviderResponseVersion + 1,
			CanonicalSHA256: canonical.SHA256,
			Locale:          localization.LocaleRussian,
			Translations: []localization.ProviderTranslation{
				localization.NewProviderTranslation(0, "Руководство по репозиторию"),
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		prompt, evidence := localizationExecutionEvidence(t)
		var calls int
		transport := localizationRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			calls++
			return localizationHTTPResponse(
				request,
				http.StatusOK,
				localizationProviderEnvelope(string(response)),
			), nil
		})
		result, err := (&Client{
			HTTPClient: &http.Client{Transport: transport}, Auth: authNone,
		}).ExecuteLocalizationRequest(t.Context(), prompt, evidence)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := localization.DecodeRussianProviderResponse(canonical, input, result.Content); err == nil {
			t.Fatal("semantic-invalid localization response was accepted locally")
		}
		if calls != 1 || result.Attempts != 1 {
			t.Fatalf("calls/result = %d/%#v, want one attempt", calls, result)
		}
	})
}

func TestExecuteLocalizationRequestRejectsInvalidEvidenceBeforeTransport(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	prompt := localization.Prompt{
		Version: localization.PromptVersion,
		System:  "system",
		User:    "user",
	}
	builder := &Client{
		Endpoint: server.URL, Auth: authNone,
		Model: "translation-model", MaxTokens: 128,
	}
	evidence, err := builder.BuildLocalizationRequest(prompt)
	if err != nil {
		t.Fatalf("BuildLocalizationRequest() error = %v", err)
	}
	evidence.Body = append([]byte(nil), evidence.Body...)
	evidence.Body[len(evidence.Body)-1] = ' '

	result, err := (&Client{
		HTTPClient: server.Client(),
		Auth:       authNone,
	}).ExecuteLocalizationRequest(context.Background(), prompt, evidence)
	if err == nil || !strings.Contains(err.Error(), "invalid localization request evidence") {
		t.Fatalf("ExecuteLocalizationRequest() error = %v", err)
	}
	if len(result.Content) != 0 ||
		result.Attempts != 0 ||
		result.RequestBytes != 0 ||
		result.InputTokens != 0 ||
		result.OutputTokens != 0 ||
		result.PromptCacheHitTokens != 0 ||
		result.PromptCacheMissTokens != 0 {
		t.Fatalf("result = %#v, want zero", result)
	}
	if calls.Load() != 0 {
		t.Fatalf("HTTP calls = %d, want 0", calls.Load())
	}
}

func TestExecuteLocalizationRequestPreservesSafeProviderErrors(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"Bearer company-secret-token-value"}`)
	}))
	defer server.Close()

	prompt := localization.Prompt{
		Version: localization.PromptVersion,
		System:  "system",
		User:    "user",
	}
	builder := &Client{
		Endpoint: server.URL, Auth: authNone,
		Model: "translation-model", MaxTokens: 128,
	}
	evidence, err := builder.BuildLocalizationRequest(prompt)
	if err != nil {
		t.Fatalf("BuildLocalizationRequest() error = %v", err)
	}
	result, err := (&Client{
		HTTPClient: server.Client(),
		Auth:       authNone,
	}).ExecuteLocalizationRequest(context.Background(), prompt, evidence)
	if err == nil ||
		!strings.Contains(err.Error(), "status 400") ||
		!strings.Contains(err.Error(), "[redacted:") ||
		strings.Contains(err.Error(), "company-secret-token-value") {
		t.Fatalf("ExecuteLocalizationRequest() error = %v", err)
	}
	if calls.Load() != 1 ||
		result.Attempts != 1 ||
		result.RequestBytes != len(evidence.Body) {
		t.Fatalf("calls/result = %d/%#v", calls.Load(), result)
	}
}

func localizationExecutionEvidence(t *testing.T) (localization.Prompt, LocalizationRequestEvidence) {
	t.Helper()
	prompt := localization.Prompt{
		Version: localization.PromptVersion,
		System:  "system",
		User:    "user",
	}
	evidence, err := (&Client{
		Endpoint:  "https://provider.example.test/v1/chat/completions",
		Auth:      authNone,
		Model:     "translation-model",
		MaxTokens: 128,
	}).BuildLocalizationRequest(prompt)
	if err != nil {
		t.Fatalf("BuildLocalizationRequest() error = %v", err)
	}
	return prompt, evidence
}

func localizationProviderEnvelope(content string) string {
	encoded, _ := json.Marshal(content)
	return `{"choices":[{"message":{"content":` + string(encoded) +
		`}}],"usage":{"prompt_tokens":7,"completion_tokens":3}}`
}

func localizationHTTPResponse(
	request *http.Request,
	status int,
	body string,
) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}

type localizationRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn localizationRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

type localizationReadErrorBody struct {
	data []byte
	err  error
}

func (body *localizationReadErrorBody) Read(buffer []byte) (int, error) {
	if len(body.data) == 0 {
		return 0, body.err
	}
	read := copy(buffer, body.data)
	body.data = body.data[read:]
	return read, nil
}

func (*localizationReadErrorBody) Close() error {
	return nil
}
