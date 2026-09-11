package deepseek

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestContextLimitRetainsActualProviderRefusalWithoutTransportRetry(t *testing.T) {
	const body = `{"error":{"message":"This model's maximum context length is 1048576 tokens. However, you requested 1784470 tokens (1656470 in the messages, 128000 in the completion). Please reduce the length of the messages or completion.","type":"invalid_request_error","param":null,"code":"invalid_request_error"}}`
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(400)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()
	completion, retryable, err := doChatMeasured(t.Context(), server.Client(), server.URL, "", "none", []byte(`{}`))
	resource, ok := err.(*ResourceLimitError)
	if !ok || retryable || requests != 1 {
		t.Fatalf("context refusal: %v, retry=%t, calls=%d", err, retryable, requests)
	}
	if resource.Kind != ResourceLimitContextTokens || resource.HTTPStatus != 400 || resource.Limit != 1048576 || resource.Observed != 1784470 || !resource.ObservedKnown || resource.InputTokens != 1656470 || resource.ConfiguredMaxTokens != 128000 || resource.OutputTokens != 0 {
		t.Fatalf("resource = %#v", resource)
	}
	if string(completion.Content) != body || completion.ResponseBytes != len(body) {
		t.Fatal("original provider response was lost")
	}
}

func TestContextLimitRequiresAnExplicitContextRefusal(t *testing.T) {
	for _, test := range []struct {
		name, body string
		status     int
		want       bool
	}{
		{"closed code", `{"error":{"code":"context_length_exceeded","message":"Input does not fit"}}`, 400, true},
		{"input longer than context", `{"error":{"message":"The input (703005 tokens) is longer than the model's context length (524288 tokens).","type":"invalid_request_error"}}`, 400, true},
		{"gateway total wording", `{"error":{"code":"400","message":"Requested token count exceeds the model's maximum context length of 524288 tokens. You requested a total of 573542 tokens: 445542 tokens from the input messages and 128000 tokens for the completion. Please reduce the number of tokens in the input messages or the completion to fit within the limit.","type":"BadRequestError"}}`, 400, true},
		{"gateway total wording within limit", `{"error":{"code":"400","message":"maximum context length of 524288 tokens. You requested a total of 500000 tokens: 372000 tokens from the input messages and 128000 tokens for the completion."}}`, 400, false},
		{"input within context", `{"error":{"message":"The input (500000 tokens) is longer than the model's context length (524288 tokens)."}}`, 400, false},
		{"DeepSeek terse", `{"error":{"code":"quota_limit_reached","message":"Input token exceed the limit (request id: example)"}}`, 400, true},
		{"top-level numeric code", `{"code":400,"message":"This model's maximum context length is 1024 tokens. However, you requested 1200 tokens (1000 in the messages, 200 in the completion)."}`, 400, true},
		{"quota alone", `{"error":{"code":"quota_limit_reached","message":"Balance too low"}}`, 400, false},
		{"output maximum", `{"error":{"message":"max_tokens must be at most 128000"}}`, 400, false},
		{"unknown parameter", `{"error":{"message":"context_length is not a supported parameter"}}`, 400, false},
		{"setting mention", `{"error":{"message":"maximum context length is 1024 tokens"}}`, 400, false},
		{"not over limit", `{"message":"maximum context length is 1024 tokens. However, you requested 1000 tokens (800 in the messages, 200 in the completion)"}`, 400, false},
		{"inconsistent counts", `{"message":"maximum context length is 1024 tokens. However, you requested 1200 tokens (1200 in the messages, 200 in the completion)"}`, 400, false},
		{"different status", `{"error":{"code":"context_length_exceeded"}}`, 429, false},
		{"plain error", `context_length_exceeded`, 400, false},
		{"quoted request", `{"request":{"code":"context_length_exceeded"},"message":"invalid schema"}`, 400, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := providerContextLimit(test.status, []byte(test.body))
			if (got != nil) != test.want {
				t.Fatalf("context limit=%#v, want %t", got, test.want)
			}
		})
	}
	overflow, _ := json.Marshal(map[string]string{"message": "maximum context length is " + strings.Repeat("9", 40) + " tokens. However, you requested 1200 tokens (1000 in the messages, 200 in the completion)"})
	if providerContextLimit(400, overflow) != nil {
		t.Fatal("overflowing counts accepted")
	}
}
