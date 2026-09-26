package table

import (
	"context"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
)

// One malformed answer leaves only its question unanswered; every other
// row keeps its decision. A response without answers decided nothing.
func TestDecodeClassifierReadsEachAnswerAlone(t *testing.T) {
	window := closedWindow()
	raw := []byte(`{"answers":{"s1|part":{"type":"choice","choice":"Serving","probabilities":{"Serving":0.9}},"s2|part":"Serving"}}`)
	result, err := DecodeClassifier(closedDefinition(), window, raw)
	if err != nil || result.Answers[0]["part"] != "c1" || result.Answers[1] != nil || len(result.Rejections) != 1 || !strings.Contains(result.Rejections[0].Reason, "not answered") {
		t.Fatalf("one malformed answer refused its neighbours: %+v / %v", result, err)
	}
	for _, raw := range []string{`{}`, `{"answers":null}`, `{"answers":[]}`, `not json`} {
		if _, err := DecodeClassifier(closedDefinition(), window, []byte(raw)); err == nil {
			t.Fatalf("a response without answers was accepted: %s", raw)
		}
	}
}

// A window whose every row the model answered, if uncertainly, is an
// explicit answer: it is accepted and cached with no row decided, so the
// same request is not bought again. A window with rows left unanswered, or
// answered outside their options, and nothing accepted is still refused.
func TestAnUncertainWindowIsAnExplicitAnswer(t *testing.T) {
	def, window := closedDefinition(), closedWindow()
	uncertain := []byte(`{"answers":{"s1|part":{"type":"choice","choice":"Serving","probabilities":{"Serving":0.4,"none":0.35}},"s2|part":{"type":"choice","choice":"none","probabilities":{"Serving":0.45,"none":0.5}}}}`)
	result, err := DecodeClassifier(def, window, uncertain)
	if err != nil || result.Answers[0] != nil || result.Answers[1] != nil || len(result.Rejections) != 2 || len(result.AcceptedRowKeys()) != 0 {
		t.Fatalf("an all-uncertain window was refused or decided: %+v / %v", result, err)
	}
	for name, raw := range map[string]string{
		"every row missing":        `{"answers":{}}`,
		"one row missing":          `{"answers":{"s1|part":{"type":"choice","choice":"Serving","probabilities":{"Serving":0.4,"none":0.35}}}}`,
		"one unlisted, one unsure": `{"answers":{"s1|part":{"type":"choice","choice":"c9","probabilities":{"c9":1}},"s2|part":{"type":"choice","choice":"none","probabilities":{"none":0.3,"Serving":0.28}}}}`,
	} {
		if _, err := DecodeClassifier(def, window, []byte(raw)); err == nil || !strings.Contains(err.Error(), "no rows accepted") {
			t.Fatalf("%s: a window without an explicit answer was accepted: %v", name, err)
		}
	}
	provider := &fixedClassifierProvider{response: uncertain}
	call, err := ClassifierCall(def, window)
	if err != nil {
		t.Fatal(err)
	}
	executor := llm.Executor{Enabled: true, RootDir: t.TempDir()}
	if _, err := llm.ExecuteJSON(t.Context(), executor, provider, call); err != nil {
		t.Fatal(err)
	}
	recalled, err := llm.RecallJSON(t.Context(), executor, provider, call)
	if err != nil || !recalled.Cached || provider.calls != 1 {
		t.Fatalf("the uncertain window was not cached: %+v / %v", recalled, err)
	}
}

type fixedClassifierProvider struct {
	response []byte
	calls    int
}

func (*fixedClassifierProvider) State() []byte { return []byte(`{"model":"classifier-test"}`) }
func (*fixedClassifierProvider) Prepare(prompt llm.Prompt, _ llm.Limits) (llm.Prepared, error) {
	return llm.NewPrepared([]byte(prompt.User))
}
func (provider *fixedClassifierProvider) Complete(context.Context, llm.Prepared) (llm.Completion, error) {
	provider.calls++
	return llm.Completion{Response: provider.response, ChoiceCount: 1, FinishReason: llm.FinishStop, Metrics: llm.Metrics{Attempts: 1}}, nil
}
