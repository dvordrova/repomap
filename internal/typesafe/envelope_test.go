package typesafe

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
)

// A direct executor call has the same guard as a table. Both independently
// enforce the single-question and complete-request bounds before any HTTP call.
func TestEveryEvaluationPreparesAgainstItsProviderEnvelope(t *testing.T) {
	for _, test := range []struct {
		name    string
		context string
		asks    map[string]llm.Question
		limit   int
	}{
		{"shared state", strings.Repeat("я", QuestionTokenLimit/2), map[string]llm.Question{"q": {Ask: "Choose?"}}, QuestionTokenLimit},
		{"criteria", "", map[string]llm.Question{"q": {Ask: "Choose?", Options: []llm.Option{{Name: "one", Meaning: strings.Repeat("x", QuestionTokenLimit)}}}}, QuestionTokenLimit},
		{"total", "", map[string]llm.Question{"a": {Ask: strings.Repeat("x", 22_000)}, "b": {Ask: strings.Repeat("y", 22_000)}, "c": {Ask: strings.Repeat("z", 22_000)}}, RequestTokenLimit},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			client := handlerClient(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
			prompt, err := client.Prompt("task", map[string]any{"complete": test.context}, test.asks)
			if err != nil {
				t.Fatal(err)
			}
			_, err = llm.ExecuteJSON(t.Context(), llm.Executor{}, client, llm.Call[any]{Prompt: prompt,
				Limits:         llm.Limits{MaxRequestBytes: 1 << 20, MaxResponseBytes: 1 << 20, MaxOutputTokens: 1},
				DecodeValidate: func(raw []byte) (any, error) { var v any; err := json.Unmarshal(raw, &v); return v, err }})
			var refused *llm.ResourceLimitError
			if !errors.As(err, &refused) || refused.Kind != llm.ResourceLimitContextTokens || refused.Limit != test.limit || calls != 0 {
				t.Fatalf("guard: calls=%d err=%v", calls, err)
			}
		})
	}
}

// Exact prepared bytes keep their existing cache identity. JSON escaping,
// non-ASCII bytes and model framing all participate in the envelope reservation.
func TestEnvelopeIncludesModelAndExactFramingAtTheBoundary(t *testing.T) {
	client := &Client{Model: "jev-test"}
	prompt := llm.Prompt{User: `{"state":"","questions":{"q":"?"}}`}
	prepared, err := client.Prepare(prompt, llm.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	padding := QuestionTokenLimit - 1 - prepared.Len()
	prompt.User = `{"state":"` + strings.Repeat("x", padding) + `","questions":{"q":"?"}}`
	prepared, err = client.Prepare(prompt, llm.Limits{})
	if err != nil || prepared.Len() != QuestionTokenLimit-1 {
		t.Fatalf("exact boundary: size=%d err=%v", prepared.Len(), err)
	}
	prompt.User = strings.Replace(prompt.User, `"state":"`, `"state":"x`, 1)
	if _, err := client.Prepare(prompt, llm.Limits{}); err == nil {
		t.Fatal("one byte beyond question plus output allowance prepared")
	}
}
