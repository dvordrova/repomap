package reading

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/llm"
)

type independentResponseProvider struct {
	tableProvider
	response []byte
	calls    int
}

func (p *independentResponseProvider) Complete(context.Context, llm.Prepared) (llm.Completion, error) {
	p.calls++
	return llm.Completion{Response: p.response, ChoiceCount: 1, FinishReason: llm.FinishStop, Metrics: llm.Metrics{Attempts: 1}}, nil
}

func TestIndependentOperationRejectionPreservesNeighboursCacheAndReplay(t *testing.T) {
	cache := t.TempDir()
	def := lines.Operations()
	rows := []table.Row{
		{ID: "first", Fields: []table.Field{{Name: "path", Value: "first.go"}, {Name: "entry_options", Value: []string{"self", "none"}}}},
		{ID: "second", Fields: []table.Field{{Name: "path", Value: "second.go"}, {Name: "entry_options", Value: []string{"self", "none"}}}},
		{ID: "third", Fields: []table.Field{{Name: "path", Value: "third.go"}, {Name: "entry_options", Value: []string{"self", "none"}}}},
	}
	newReader := func(provider llm.Provider) *reader {
		r := answerTestReader(t, nil, provider)
		r.opts.Executor.RootDir = cache
		r.opts.Through = ""
		r.places = make(map[string]atlas.Place)
		r.knowledge = make(map[string]*Knowledge)
		r.knowledgeSubjects = make(map[string]*Knowledge)
		r.responseTables = make(map[string]rememberedTable)
		for _, row := range rows {
			r.places[row.ID] = atlas.Place{ID: row.ID, Path: row.ID + ".go"}
		}
		return r
	}
	response := []byte(`{"extra":{"ignored":true},"rows":[{"key":"r1","entry":"self","activation":"command","name":"First","description":"Runs first.","extra":[1]},{"key":"r2","entry":"u1","activation":"command","name":"Second","description":"Runs second."},{"key":"r3","entry":"none","activation":"none","name":"Third","description":"No operation."}]}`)
	provider := &independentResponseProvider{response: response}
	r := newReader(provider)
	var messages []string
	r.opts.State = func(_, _ string, details ...string) { messages = append(messages, details...) }
	first, err := r.runIndependent(t.Context(), def, 1, nil, rows)
	if err != nil || first[0].answer["name"] != "First" || first[1].answer != nil || first[1].source != atlas.SourceGiven || first[2].answer["entry"] != "none" {
		t.Fatalf("one unsupported u1 removed valid operations: %+v / %v; rejected=%+v", first, err, r.rejected)
	}
	if len(r.rejected) != 1 || r.rejected[0].Kind != "row_rejected" || !strings.Contains(r.rejected[0].Reason, `"u1"`) || r.rejected[0].Samples[0] != "r2" || r.rejected[0].ResponseRef == "" {
		t.Fatalf("refused row lost its reason and original response: %+v", r.rejected)
	}
	if !strings.Contains(strings.Join(messages, "\n"), "1 rows rejected, 2 accepted in this response") || r.use(def.Stage).Given != 1 {
		t.Fatalf("partial impact is not visible: %v / %+v", messages, r.use(def.Stage))
	}
	exchange, found, err := llm.CachedExchange(cache, first[0].requestKey)
	if err != nil || !found || !bytes.Equal(exchange.Response, response) || first[0].requestKey != first[2].requestKey {
		t.Fatalf("accepted neighbours lost their exact shared response: %+v / %v", exchange, err)
	}
	for i, row := range rows {
		input, _, err := r.knowledgeInput(def, nil, row)
		if err != nil {
			t.Fatal(err)
		}
		_, found, err := llm.LoadMemo(r.opts.Executor, input.BasisID, llm.DecodeJSON[rememberedRow](nil))
		if err != nil || found != (i != 1) {
			t.Fatalf("memo availability for %s = %t: %v", row.ID, found, err)
		}
	}
	// The accepted neighbours are recalled using their original row keys after
	// reordering. Only the previously refused row is requested again.
	warmProvider := &independentResponseProvider{response: []byte(`{"rows":[{"key":"r1","entry":"self","activation":"command","name":"Second","description":"Runs second."}]}`)}
	warm := newReader(warmProvider)
	warmed, err := warm.runIndependent(t.Context(), def, 1, nil, []table.Row{rows[2], rows[1], rows[0]})
	if err != nil || warmProvider.calls != 1 || warmed[0].source != atlas.SourceCache || warmed[1].source != atlas.SourceModel || warmed[2].source != atlas.SourceCache {
		t.Fatalf("valid neighbours were requested again: %+v / calls=%d / %v", warmed, warmProvider.calls, err)
	}
	// Replay preserves the original window. A malformed/duplicated neighbouring
	// row must not prevent either accepted memo from reading the new response.
	replayed := []byte(`{"extra":true,"rows":[{"key":"r1","entry":"self","activation":"command","name":"Updated first","description":"Runs first."},{"key":"r2","entry":42},{"key":"r2","entry":"u1"},{"key":"r3","entry":"none","activation":"none","name":"Third","description":"No operation."}]}`)
	prepared, err := llm.NewPrepared(exchange.Request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := llm.ReplayJSON(t.Context(), r.opts.Executor, &independentResponseProvider{response: replayed}, prepared); err != nil {
		t.Fatal(err)
	}
	recallProvider := &independentResponseProvider{}
	recalled := newReader(recallProvider)
	recalled.recallOnly = true
	updated, err := recalled.runIndependent(t.Context(), def, 1, nil, rows)
	if err != nil || recallProvider.calls != 0 || updated[0].answer["name"] != "Updated first" || updated[1].answer["name"] != "Second" || updated[2].answer["entry"] != "none" {
		t.Fatalf("replay lost accepted rows or invoked a provider: %+v / %v", updated, err)
	}
	if updated[0].responseSHA == first[0].responseSHA || updated[2].responseSHA != updated[0].responseSHA {
		t.Fatal("replayed neighbour memos retained a stale response identity")
	}
}
