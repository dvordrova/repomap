package questionbatch

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/llm"
)

func TestRepeatedRelevancePreservesAllHintsWithoutFirstWins(t *testing.T) {
	data, err := prepareCatalogue(testInput(1, 2), Options{})
	if err != nil {
		t.Fatal(err)
	}
	selections := []Selection{
		{Row: "r1", Anchors: []string{"a2"}, Relevance: "context", Why: "Original setup instructions."},
		{Row: "r1", Anchors: []string{"a1"}, Relevance: "context", Why: "Count field suggests response shape.\n\nIt does not prove runtime values."},
	}
	var previous Response
	for i := range 2 {
		if i == 1 {
			selections[0], selections[1] = selections[1], selections[0]
		}
		raw, _ := json.Marshal(Response{Questions: []Decision{
			{Key: "q1", Selections: append(append([]Selection{}, selections...), selections[0])},
			{Key: "q2", Selections: []Selection{}},
		}})
		result, err := data.decode([]int{0}, data.questions, raw)
		if err != nil || len(result.Questions) != 2 || len(result.Rejections) != 0 {
			t.Fatalf("compatible hints refused a question: %+v / %v", result, err)
		}
		selection := result.Questions[0].Selections[0]
		if !reflect.DeepEqual(selection.Anchors, []string{"a1", "a2"}) || selection.Why != "Count field suggests response shape.\n\nIt does not prove runtime values.\n\nOriginal setup instructions." {
			t.Fatalf("original anchors or qualified hints lost: %+v", selection)
		}
		if i == 1 && !reflect.DeepEqual(previous, result) {
			t.Fatal("response ordering changed the normalized decision")
		}
		previous = result
	}
}

func TestRetrievalReservesSharedAllowanceForReasoningAndSelections(t *testing.T) {
	provider := &testProvider{}
	if _, err := Run(t.Context(), llm.Executor{}, provider, testInput(3, 2), Options{}); err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 1 || provider.requests[0].Limits.MaxOutputTokens != llm.DefaultMaxOutputTokens || !provider.requests[0].Prompt.Reasoning {
		t.Fatalf("retrieval did not reserve the shared allowance for reasoning and selections: %+v", provider.requests)
	}
}

type rawQuestionProvider struct {
	*testProvider
	raw       []byte
	completed int
	accepted  [][]string
}

func (p *rawQuestionProvider) Complete(context.Context, llm.Prepared) (llm.Completion, error) {
	p.completed++
	return llm.Completion{Response: p.raw, FinishReason: llm.FinishStop, ChoiceCount: 1, Metrics: llm.Metrics{Attempts: 1}}, nil
}

func (p *rawQuestionProvider) AdaptResponse(_, raw []byte) (llm.AdaptedResponse, error) {
	return llm.AdaptedResponse{Domain: raw, Accept: func(rows []string) { p.accepted = append(p.accepted, rows) }}, nil
}

func TestQuestionMalformedSiblingKeepsExactMemoReplayAndMetadataScope(t *testing.T) {
	input := testInput(2, 2)
	raw := []byte(`{"extra":{"ignored":true},"questions":[{"key":"q1","selections":[{"row":"r1","anchors":42,"relevance":"direct","why":"Refused question metadata."}]},{"key":"q2","extra":true,"selections":[{"row":"r2","anchors":["a1"],"relevance":"direct","why":"Accepted original reason.","extra":[]}]}]}`)
	provider := &rawQuestionProvider{testProvider: &testProvider{}, raw: raw}
	var events []llm.Event
	executor := llm.Executor{Enabled: true, RootDir: t.TempDir(), Observer: llm.ObserverFunc(func(event llm.Event) error { events = append(events, event); return nil })}
	first, err := Run(t.Context(), executor, provider, input, Options{})
	if err != nil || len(first.Exchanges) != 1 || first.Exchanges[0].Err != nil || provider.completed != 1 {
		t.Fatalf("malformed neighbour refused the entire window: %+v / %v", first, err)
	}
	for _, chunk := range first.Questions[0].Chunks {
		if chunk.Inspected || len(chunk.Anchors) != 0 {
			t.Fatal("unavailable question became a negative or positive finding")
		}
	}
	if !first.Questions[1].Chunks[0].Inspected || len(first.Questions[1].Chunks[0].Anchors) != 0 || !first.Questions[1].Chunks[1].Inspected || first.Questions[1].Chunks[1].Why != "Accepted original reason." {
		t.Fatal("valid question lost its complete per-chunk coverage")
	}
	if !reflect.DeepEqual(provider.accepted, [][]string{{"r2"}}) {
		t.Fatalf("refused question authorized glossary metadata: %v", provider.accepted)
	}
	data, err := prepareCatalogue(input, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for i, question := range data.questions {
		key, err := data.memoIdentity(provider, question)
		if err != nil {
			t.Fatal(err)
		}
		_, found, err := llm.LoadMemo(executor, key, llm.DecodeJSON[questionMemo](nil))
		if err != nil || found != (i == 1) {
			t.Fatalf("memo for %s exists=%t: %v", question.Key, found, err)
		}
	}
	retained := input
	retained.Questions = []string{input.Questions[1]}
	warm, err := Run(t.Context(), executor, provider, retained, Options{})
	if err != nil || provider.completed != 1 || len(warm.Exchanges) != 1 || !warm.Exchanges[0].Reused || warm.Questions[0].Chunks[1].Source != atlas.SourceCache {
		t.Fatalf("invalid original sibling defeated retained question memo: %+v / %v", warm, err)
	}
	provider.raw = []byte(strings.ReplaceAll(string(raw), "Accepted original reason.", "Accepted replay reason."))
	prepared, err := llm.NewPrepared(first.Exchanges[0].Outcome.Request)
	if err != nil {
		t.Fatal(err)
	}
	// Replay only resends bytes; the owning decoder reports row diagnostics
	// on the following read, not during the transport-only replay.
	replayExecutor := executor
	replayExecutor.Observer = nil
	if _, err := llm.ReplayJSON(t.Context(), replayExecutor, provider, prepared); err != nil {
		t.Fatal(err)
	}
	replayed, err := Run(t.Context(), executor, provider, retained, Options{})
	if err != nil || provider.completed != 2 || replayed.Questions[0].Chunks[1].Why != "Accepted replay reason." || replayed.Questions[0].Chunks[1].QuestionRef != "q2" {
		t.Fatalf("replay did not update the retained original question: %+v / %v", replayed, err)
	}
	for _, rows := range provider.accepted {
		if !reflect.DeepEqual(rows, []string{"r2"}) {
			t.Fatalf("live/cache/replay broadened rejected metadata: %v", rows)
		}
	}
	for _, event := range events {
		if event.Kind == llm.EventLive || event.Kind == llm.EventCacheHit {
			if len(event.ResponseRejections) != 1 || event.ResponseRejections[0].Kind != "question_rejected" || event.ResponseRejections[0].Count != 2 {
				t.Fatalf("shared diagnostics omitted refused question coverage: %+v", event.ResponseRejections)
			}
		}
	}
}

func TestRetrievalMetadataSharedWithRefusedQuestionIsDiscarded(t *testing.T) {
	data, err := prepareCatalogue(testInput(2, 2), Options{})
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"questions":[{"key":"q1","selections":[{"row":"r1","anchors":["a999"],"relevance":"direct","why":"Invalid interpretation."}]},{"key":"q2","selections":[{"row":"r1","anchors":["a1"],"relevance":"direct","why":"Accepted interpretation."}]}]}`)
	result, err := data.decode([]int{0, 1}, data.questions, raw)
	if err != nil || len(result.Questions) != 1 || result.Questions[0].Key != "q2" || !reflect.DeepEqual(result.AcceptedRowKeys(), []string{"r2"}) {
		t.Fatalf("shared r1 metadata acquired accepted question authority: %+v / %v", result, err)
	}
}
