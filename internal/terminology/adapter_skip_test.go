package terminology

import (
	"errors"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
)

// corruptContextProvider supplies local glossary context the collector cannot
// read, as a damaged or foreign cache record would.
type corruptContextProvider struct{ *provider }

func (corruptContextProvider) ResponseContext(llm.Prompt) ([]byte, error) {
	return []byte(`{"version":"repomap.terminology.context.v0","sources":[{"ref":"g1","path":"a.py","line":1}]}`), nil
}

// The glossary is optional: a defect in its own local context skips its prose
// for this exchange and never refuses the owner's accepted main answer.
func TestGlossaryContextErrorKeepsTheMainAnswer(t *testing.T) {
	collector := NewCollector([]string{"a.py"})
	answer := `{"rows":[{"key":"r1","text":"The accepted answer."}]}`
	base := &testProvider{complete: func(llm.Prepared) (llm.Completion, error) { return completed(answer) }}
	wrapped := corruptContextProvider{collector.Wrap(base).(*provider)}
	call := llm.Call[acceptedRows]{
		State:    []byte(`{"stage":"owner"}`),
		Prompt:   llm.Prompt{System: "Owner task.", User: `{"rows":[{"key":"r1","path":"a.py","line":1}]}`, ResponseFormatJSON: true},
		Limits:   llm.Limits{MaxRequestBytes: 1 << 20, MaxResponseBytes: 1 << 20, MaxOutputTokens: 1000},
		Validate: func(value acceptedRows) error { return nil },
	}
	outcome, err := llm.ExecuteJSON(t.Context(), llm.Executor{}, wrapped, call)
	if err != nil || len(outcome.Value.Rows) != 1 || outcome.Value.Rows[0]["text"] != "The accepted answer." {
		t.Fatalf("a glossary context error refused the main answer: %+v, %v", outcome.Value, err)
	}
	skipped := false
	for _, rejection := range outcome.ResponseRejections {
		skipped = skipped || rejection.Kind == "glossary_prose_skipped" && rejection.Reason != ""
	}
	if !skipped || len(collector.pending) != 0 {
		t.Fatalf("the skipped glossary prose was not recorded, or was collected anyway: %+v, pending=%d", outcome.ResponseRejections, len(collector.pending))
	}

	// The owner still refuses a main answer that is actually wrong.
	call.Validate = func(acceptedRows) error { return errors.New("owner refuses the row") }
	if _, err := llm.ExecuteJSON(t.Context(), llm.Executor{}, wrapped, call); err == nil {
		t.Fatal("the skipped glossary made a refused main answer acceptable")
	}
}
