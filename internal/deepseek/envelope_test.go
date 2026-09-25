package deepseek

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
)

func completeRawEnvelope(t *testing.T, body string) (llm.Completion, error) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(body))
	}))
	defer server.Close()
	prepared, err := llm.NewPrepared([]byte(`{"request":true}`))
	if err != nil {
		t.Fatal(err)
	}
	return llmProviderTestClient(server).Complete(context.Background(), prepared)
}

// Token counts are metrics. A count the adapter cannot read leaves the
// answer beside it intact and is reported as unavailable; an envelope whose
// choices or content cannot be read is still no answer.
func TestEnvelopeWithUnreadableUsageKeepsItsAnswer(t *testing.T) {
	const choices = `"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"{\"ok\":true}"}}]`
	for name, usage := range map[string]string{
		"fractional count": `{"prompt_tokens":12.0,"completion_tokens":3}`,
		"quoted count":     `{"prompt_tokens":"12","completion_tokens":3}`,
		"not an object":    `"12 tokens"`,
	} {
		t.Run(name, func(t *testing.T) {
			completion, err := completeRawEnvelope(t, `{`+choices+`,"usage":`+usage+`}`)
			if err != nil || string(completion.Response) != `{"ok":true}` || completion.FinishReason != llm.FinishStop ||
				completion.Metrics.UsageReported || completion.Metrics.InputTokens != 0 || completion.Metrics.OutputTokens != 0 {
				t.Fatalf("unreadable usage changed the answer or invented counts: %#v / %v", completion, err)
			}
		})
	}
	completion, err := completeRawEnvelope(t, `{`+choices+`,"usage":{"prompt_tokens":12,"completion_tokens":3}}`)
	if err != nil || !completion.Metrics.UsageReported || completion.Metrics.InputTokens != 12 || completion.Metrics.OutputTokens != 3 {
		t.Fatalf("readable usage = %#v / %v", completion, err)
	}
	for name, body := range map[string]string{
		"missing choices":    `{"usage":{"prompt_tokens":12}}`,
		"unreadable choices": `{"choices":"stop"}`,
		"unreadable content": `{"choices":[{"finish_reason":"stop","message":{"content":12}}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if completion, err := completeRawEnvelope(t, body); err == nil {
				t.Fatalf("accepted an envelope without a readable answer: %#v", completion)
			}
		})
	}
}

// Letter case is a harmless form difference of a closed finish reason. A
// missing or unknown reason cannot prove the answer complete.
func TestFinishReasonIsReadInAnyLetterCase(t *testing.T) {
	envelope := func(finish string) string {
		return `{"choices":[{` + finish + `"message":{"role":"assistant","content":"{\"ok\":true}"}}],"usage":{"completion_tokens":100}}`
	}
	for _, reason := range []string{"STOP", "Stop"} {
		completion, err := completeRawEnvelope(t, envelope(`"finish_reason":"`+reason+`",`))
		if err != nil || completion.FinishReason != llm.FinishStop || string(completion.Response) != `{"ok":true}` {
			t.Fatalf("finish_reason %q = %#v / %v", reason, completion, err)
		}
	}
	completion, err := completeRawEnvelope(t, envelope(`"finish_reason":"LENGTH",`))
	var limitErr *ResourceLimitError
	if !errors.As(err, &limitErr) || limitErr.Kind != ResourceLimitOutputTokens || completion.FinishReason != llm.FinishLength {
		t.Fatalf("an upper-case output cut was not the output-token refusal: %#v / %v", completion, err)
	}
	for name, finish := range map[string]string{
		"unknown": `"finish_reason":"eos",`,
		"missing": ``,
		"null":    `"finish_reason":null,`,
	} {
		t.Run(name, func(t *testing.T) {
			completion, err := completeRawEnvelope(t, envelope(finish))
			var incomplete *IncompleteCompletionError
			if !errors.As(err, &incomplete) || completion.FinishReason != llm.FinishUnknown {
				t.Fatalf("accepted a completion with no known finish reason: %#v / %v", completion, err)
			}
		})
	}
}
