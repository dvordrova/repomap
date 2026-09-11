package questionbatch

import (
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/llm"
)

// omitOnce answers every question except the named ones the first time each
// of them is asked; a re-asked question is then answered.
func omitOnce(keys ...string) func(modelRequest) (Response, error) {
	var mu sync.Mutex
	dropped := make(map[string]bool)
	once := make(map[string]bool)
	for _, key := range keys {
		once[key] = true
	}
	return func(request modelRequest) (Response, error) {
		response := selectFirst(request, "Inspect the original declaration.")
		mu.Lock()
		defer mu.Unlock()
		kept := []Decision{}
		for _, decision := range response.Questions {
			if once[decision.Key] && !dropped[decision.Key] {
				dropped[decision.Key] = true
				continue
			}
			kept = append(kept, decision)
		}
		response.Questions = kept
		return response, nil
	}
}

// An accepted response that names only some of its questions decided nothing
// about the others: they are asked once more over the same rows, without the
// answered ones, and the journal records the omission, not a loss.
func TestOmittedQuestionsAreReaskedOverTheSameRowsAndRemembered(t *testing.T) {
	input := testInput(3, 20)
	provider := &testProvider{complete: omitOnce("q2", "q5", "q10", "q18")}
	var events []llm.Event
	executor := llm.Executor{Enabled: true, RootDir: t.TempDir(), BatchConcurrency: 4,
		Observer: llm.ObserverFunc(func(event llm.Event) error { events = append(events, event); return nil })}
	var notices []int
	executor.PlanNotice = func(count int) { notices = append(notices, count) }
	result, err := Run(t.Context(), executor, provider, input, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 4 || len(result.Exchanges) != 4 || !reflect.DeepEqual(notices, []int{3, 1}) {
		t.Fatalf("three shared windows and one re-ask expected, got calls=%d exchanges=%d notices=%v", len(provider.requests), len(result.Exchanges), notices)
	}
	reask := result.Exchanges[3]
	if !reask.Reask || reask.Err != nil || !reflect.DeepEqual(reask.QuestionRefs, []string{"q2", "q5", "q10", "q18"}) || !reflect.DeepEqual(reask.ChunkIndexes, []int{0, 1, 2}) || len(reask.Outcome.Value.Rejections) != 0 {
		t.Fatalf("re-ask window = %+v", reask)
	}
	for q, question := range result.Questions {
		for row, chunk := range question.Chunks {
			if !chunk.Inspected || chunk.Source != atlas.SourceModel || (row == 0 && len(chunk.Selections) == 0) {
				t.Fatalf("re-asked question lost coverage at %d/%d: %+v", q, row, chunk)
			}
		}
	}
	omitted := map[int][]string{0: {"q2", "q5"}, 1: {"q10"}, 2: {"q18"}}
	for i := range 3 {
		exchange := result.Exchanges[i]
		var keys []string
		for _, rejection := range exchange.Outcome.Value.Rejections {
			if !rejection.Omitted || !rejection.Recovered || rejection.Chunks != 3 || !strings.Contains(rejection.Reason, "missing question "+rejection.Question) {
				t.Fatalf("first-round omission is not marked recovered: %+v", rejection)
			}
			keys = append(keys, rejection.Question)
		}
		if exchange.Reask || !reflect.DeepEqual(keys, omitted[i]) {
			t.Fatalf("window %d omissions = %v", i, keys)
		}
		for _, rejection := range exchange.Outcome.Value.ResponseRejections() {
			if rejection.Kind != "question_omitted" || rejection.Count != 3 {
				t.Fatalf("omission journaled as a loss: %+v", rejection)
			}
		}
	}
	for _, event := range events {
		for _, rejection := range event.ResponseRejections {
			if rejection.Kind == "question_rejected" {
				t.Fatalf("shared journal counted a recovered question as a loss: %+v", rejection)
			}
		}
	}
	data, err := prepareCatalogue(input, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, question := range data.questions {
		key, err := data.memoIdentity(provider, question)
		if err != nil {
			t.Fatal(err)
		}
		memo, found, err := llm.LoadMemo(executor, key, llm.DecodeJSON[questionMemo](nil))
		if err != nil || !found || len(memo.Windows) != 1 {
			t.Fatalf("memo for %s: found=%t %v", question.Key, found, err)
		}
		if reasked := question.Key == "q2" || question.Key == "q5" || question.Key == "q10" || question.Key == "q18"; memo.Windows[0].Reask != reasked || (reasked && len(memo.Windows[0].Questions) != 4) {
			t.Fatalf("memo for %s does not reference its accepting window: %+v", question.Key, memo.Windows[0])
		}
	}
	provider.complete = nil
	before := len(provider.requests)
	warm, err := Run(t.Context(), executor, provider, input, Options{})
	if err != nil || len(provider.requests) != before || len(warm.Exchanges) != 4 {
		t.Fatalf("recovered questions were not recalled from their re-ask window: calls=%d exchanges=%d err=%v", len(provider.requests)-before, len(warm.Exchanges), err)
	}
	for q, question := range warm.Questions {
		for row, chunk := range question.Chunks {
			if !chunk.Inspected || chunk.Source != atlas.SourceCache {
				t.Fatalf("warm coverage at %d/%d: %+v", q, row, chunk)
			}
		}
	}
}

// A question the model omits when re-asked stays refused as missing; there
// is no third round. Alone in its window that is a refused response; beside
// a sibling the model did answer it is a rejected question.
func TestQuestionOmittedTwiceStaysRefusedAsMissing(t *testing.T) {
	t.Run("alone", func(t *testing.T) {
		provider := &testProvider{complete: func(request modelRequest) (Response, error) {
			response := selectFirst(request, "Inspect the original declaration.")
			kept := []Decision{}
			for _, decision := range response.Questions {
				if decision.Key != "q2" {
					kept = append(kept, decision)
				}
			}
			response.Questions = kept
			return response, nil
		}}
		result, err := Run(t.Context(), llm.Executor{}, provider, testInput(2, 2), Options{})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Exchanges) != 2 || len(provider.requests) != 2 {
			t.Fatalf("exchanges = %d", len(result.Exchanges))
		}
		first, second := result.Exchanges[0], result.Exchanges[1]
		if first.Err != nil || len(first.Outcome.Value.Rejections) != 1 || !first.Outcome.Value.Rejections[0].Omitted || first.Outcome.Value.Rejections[0].Recovered {
			t.Fatalf("first round = %+v", first.Outcome.Value.Rejections)
		}
		if !second.Reask || second.Err == nil || !strings.Contains(second.Err.Error(), "missing question q2") || !reflect.DeepEqual(second.QuestionRefs, []string{"q2"}) || !reflect.DeepEqual(second.ChunkIndexes, []int{0, 1}) {
			t.Fatalf("re-ask = %+v / %v", second, second.Err)
		}
		for row := range 2 {
			if !result.Questions[0].Chunks[row].Inspected || result.Questions[1].Chunks[row].Inspected {
				t.Fatalf("twice-omitted question changed coverage at row %d", row)
			}
		}
	})
	t.Run("beside an answered sibling", func(t *testing.T) {
		// q3 is omitted once and answered when re-asked; q2 is never named.
		once := omitOnce("q3")
		provider := &testProvider{complete: func(request modelRequest) (Response, error) {
			response, err := once(request)
			kept := []Decision{}
			for _, decision := range response.Questions {
				if decision.Key != "q2" {
					kept = append(kept, decision)
				}
			}
			response.Questions = kept
			return response, err
		}}
		result, err := Run(t.Context(), llm.Executor{}, provider, testInput(2, 3), Options{})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Exchanges) != 2 || result.Exchanges[1].Err != nil || !result.Exchanges[1].Reask {
			t.Fatalf("exchanges = %+v", result.Exchanges)
		}
		rejections := result.Exchanges[1].Outcome.Value.Rejections
		if len(rejections) != 1 || rejections[0].Question != "q2" || rejections[0].Omitted || rejections[0].Recovered || !strings.Contains(rejections[0].Reason, "missing question q2") ||
			result.Exchanges[1].Outcome.Value.ResponseRejections()[0].Kind != "question_rejected" {
			t.Fatalf("second omission is not a loss: %+v", rejections)
		}
		byKey := make(map[string]QuestionRejection)
		for _, rejection := range result.Exchanges[0].Outcome.Value.Rejections {
			byKey[rejection.Question] = rejection
		}
		if len(byKey) != 2 || !byKey["q2"].Omitted || byKey["q2"].Recovered || !byKey["q3"].Omitted || !byKey["q3"].Recovered {
			t.Fatalf("first-round omissions = %+v", byKey)
		}
		for row := range 2 {
			if !result.Questions[0].Chunks[row].Inspected || result.Questions[1].Chunks[row].Inspected || !result.Questions[2].Chunks[row].Inspected {
				t.Fatalf("coverage at row %d: %+v", row, result.Questions)
			}
		}
	})
}
