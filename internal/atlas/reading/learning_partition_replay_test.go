package reading

import (
	"context"
	"fmt"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/llm"
)

// fixedAnswerProvider answers every request with one response, the way a
// replay stores whatever the provider returned.
type fixedAnswerProvider struct {
	llm.Provider
	response []byte
}

func (p fixedAnswerProvider) Complete(context.Context, llm.Prepared) (llm.Completion, error) {
	return llm.Completion{Response: p.response, FinishReason: llm.FinishStop, ChoiceCount: 1, Metrics: llm.Metrics{Attempts: 1}}, nil
}

// A replay of the original Learn request that the current decoder refuses is
// kept on disk, but it is no answer: the partition memo still stands and the
// original is not asked live again. A replay the decoder accepts supersedes
// the partition.
func TestLearningPartitionStandsOverARefusedOriginalReplay(t *testing.T) {
	var evidence []learningEvidence
	for i := 0; i < 4; i++ {
		evidence = append(evidence, learningEvidence{Context: map[string]any{"ordinal": i, "original": "Original declaration."},
			Source: atlas.QuestionStop{SubjectID: fmt.Sprintf("subject-%d", i), Path: fmt.Sprintf("file%d.py", i), Line: i + 1}})
	}
	pool := func() learningRequest { return newLearningPool(evidence, false) }
	tooLarge := func(pool learningRequest) error {
		if len(pool.Evidence) > 2 {
			return &llm.ResourceLimitError{Kind: llm.ResourceLimitOutputTokens}
		}
		return nil
	}
	provider := &learningResourceProvider{refuse: tooLarge}
	cache := t.TempDir()
	run := func() *reader {
		r := isolatedLearningReader(t, cache, provider)
		r.learning = &atlas.LearningPlan{Version: 1, State: "ready"}
		if err := r.executeLearning(t.Context(), []learningRequest{pool()}, learningPrompt); err != nil {
			t.Fatal(err)
		}
		return r
	}
	r := run()
	if len(provider.requests) != 3 {
		t.Fatalf("fixture: %d requests, want the refused original and two children", len(provider.requests))
	}
	original, err := learningCall(pool(), learningPrompt)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := llm.Prepare(provider, original.Prompt, original.Limits)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := llm.ReplayJSON(t.Context(), r.opts.Executor, fixedAnswerProvider{Provider: provider, response: []byte(`{"reviews":7}`)}, prepared); err != nil {
		t.Fatal(err)
	}
	if !llm.AcceptsCachedAnswer(r.opts.Executor, provider, llm.Call[any]{Prompt: original.Prompt, Limits: original.Limits}) {
		t.Fatal("fixture: the replayed original is not on disk")
	}
	if llm.AcceptsCachedAnswer(r.opts.Executor, provider, original) {
		t.Fatal("the Learn decoder accepted a replay without a reviews list")
	}
	before := len(provider.requests)
	warm := run()
	if _, windows := warm.planLearningPartitions([]learningRequest{pool()}, learningPrompt); len(windows) != 2 || len(provider.requests) != before {
		t.Fatalf("a refused original replay displaced the partition: %d windows, %d new requests", len(windows), len(provider.requests)-before)
	}

	// A replay the Learn decoder accepts is the original's answer.
	provider.refuse = nil
	if _, err := llm.ReplayJSON(t.Context(), r.opts.Executor, provider, prepared); err != nil {
		t.Fatal(err)
	}
	provider.refuse = tooLarge
	before = len(provider.requests)
	warm = run()
	if _, windows := warm.planLearningPartitions([]learningRequest{pool()}, learningPrompt); len(windows) != 1 || len(provider.requests) != before {
		t.Fatalf("an accepted original replay did not supersede the partition: %d windows, %d new requests", len(windows), len(provider.requests)-before)
	}
}
