package reading

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
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
	def := lines.Layers()
	rows := []table.Row{
		{ID: "first", Fields: []table.Field{{Name: "path", Value: "first.go"}}},
		{ID: "second", Fields: []table.Field{{Name: "path", Value: "second.go"}}},
		{ID: "third", Fields: []table.Field{{Name: "path", Value: "third.go"}}},
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
	response := []byte(`{"extra":{"ignored":true},"rows":[{"key":"first","role":"adapter","extra":[1]},{"key":"second","role":"u1"},{"key":"third","role":"passthrough"}]}`)
	provider := &independentResponseProvider{response: response}
	r := newReader(provider)
	var messages []string
	r.opts.State = func(_, _ string, details ...string) { messages = append(messages, details...) }
	first, err := r.runIndependent(t.Context(), def, 1, rowGroups{{rows: rows}})
	if err != nil || first[0].answer["role"] != "adapter" || first[1].answer != nil || first[1].source != atlas.SourceGiven || first[2].answer["role"] != "passthrough" {
		t.Fatalf("one unsupported u1 removed valid operations: %+v / %v; rejected=%+v", first, err, r.rejected)
	}
	if len(r.rejected) != 1 || r.rejected[0].Kind != "row_rejected" || !strings.Contains(r.rejected[0].Reason, `"u1"`) || r.rejected[0].Samples[0] != "second" || r.rejected[0].ResponseRef == "" {
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
	warmProvider := &independentResponseProvider{response: []byte(`{"rows":[{"key":"second","role":"logic"}]}`)}
	warm := newReader(warmProvider)
	warmed, err := warm.runIndependent(t.Context(), def, 1, rowGroups{{rows: []table.Row{rows[2], rows[1], rows[0]}}})
	if err != nil || warmProvider.calls != 1 || warmed[0].source != atlas.SourceCache || warmed[1].source != atlas.SourceModel || warmed[2].source != atlas.SourceCache {
		t.Fatalf("valid neighbours were requested again: %+v / calls=%d / %v", warmed, warmProvider.calls, err)
	}
	// Replay preserves the original window. A malformed/duplicated neighbouring
	// row must not prevent either accepted memo from reading the new response.
	replayed := []byte(`{"extra":true,"rows":[{"key":"first","role":"logic"},{"key":"second","role":42},{"key":"second","role":"u1"},{"key":"third","role":"passthrough"}]}`)
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
	updated, err := recalled.runIndependent(t.Context(), def, 1, rowGroups{{rows: rows}})
	if err != nil || recallProvider.calls != 0 || updated[0].answer["role"] != "logic" || updated[1].answer["role"] != "logic" || updated[2].answer["role"] != "passthrough" {
		t.Fatalf("replay lost accepted rows or invoked a provider: %+v / %v", updated, err)
	}
	if updated[0].responseSHA == first[0].responseSHA || updated[2].responseSHA != updated[0].responseSHA {
		t.Fatal("replayed neighbour memos retained a stale response identity")
	}
}

// refusingRoleProvider answers every layer row, but with an unknown role for
// keys ending in 7, so those rows are refused and never remembered.
type refusingRoleProvider struct {
	tableProvider
	asked []string
}

func (p *refusingRoleProvider) Complete(_ context.Context, prepared llm.Prepared) (llm.Completion, error) {
	var request struct {
		Rows []struct {
			Key string `json:"key"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(prepared.Bytes(), &request); err != nil {
		return llm.Completion{}, err
	}
	var rows []map[string]string
	p.mu.Lock()
	for _, row := range request.Rows {
		p.asked = append(p.asked, row.Key)
		role := "logic"
		if strings.HasSuffix(row.Key, "7") {
			role = "u1"
		}
		rows = append(rows, map[string]string{"key": row.Key, "role": role})
	}
	p.mu.Unlock()
	raw, err := json.Marshal(map[string]any{"rows": rows})
	return llm.Completion{Response: raw, ChoiceCount: 1, FinishReason: llm.FinishStop, Metrics: llm.Metrics{Attempts: 1}}, err
}

// Remembered rows are recalled at once but taken in row order: a reading over
// the same memory lists its reused rows in row order, counts each once and
// asks only the rows that were refused before.
func TestRecalledRowsAreTakenInRowOrder(t *testing.T) {
	cache := t.TempDir()
	def := lines.Layers()
	var rows []table.Row
	var remembered, refused []string
	for i := 1; i <= 60; i++ {
		id := fmt.Sprintf("s%d", i)
		rows = append(rows, table.Row{ID: id, Fields: []table.Field{{Name: "path", Value: id + ".go"}}})
		if strings.HasSuffix(id, "7") {
			refused = append(refused, id)
		} else {
			remembered = append(remembered, id)
		}
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
	if _, err := newReader(&refusingRoleProvider{}).runIndependent(t.Context(), def, 1, rowGroups{{rows: rows}}); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 3; attempt++ {
		provider := &refusingRoleProvider{}
		warm := newReader(provider)
		answers, err := warm.runIndependent(t.Context(), def, 1, rowGroups{{rows: rows}})
		if err != nil {
			t.Fatal(err)
		}
		var reused []string
		for _, line := range strings.Split(warm.tables.String(), "\n") {
			if strings.HasPrefix(line, "- Reused ") {
				reused = append(reused, strings.Fields(line)[2])
			}
		}
		if !reflect.DeepEqual(reused, remembered) {
			t.Fatalf("attempt %d: reused rows out of row order: %v", attempt, reused)
		}
		if !reflect.DeepEqual(provider.asked, refused) {
			t.Fatalf("attempt %d: asked %v, want only the refused rows %v", attempt, provider.asked, refused)
		}
		use := warm.use(def.Stage)
		if use.Reused != len(remembered) || use.Rows != len(rows) {
			t.Fatalf("attempt %d: use %+v", attempt, *use)
		}
		for i, row := range rows {
			if (answers[i].answer != nil) == strings.HasSuffix(row.ID, "7") || answers[i].answer != nil && answers[i].source != atlas.SourceCache {
				t.Fatalf("attempt %d: row %s answered %+v", attempt, row.ID, answers[i])
			}
		}
	}
}
