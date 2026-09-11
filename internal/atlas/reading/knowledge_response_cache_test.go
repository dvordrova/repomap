package reading

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/terminology"
)

type rejectedRowAdapter struct {
	llm.Provider
	unwraps, accepted int
	failure           error
}

func (provider *rejectedRowAdapter) AdaptResponse(_, _, _ []byte) (llm.AdaptedResponse, error) {
	provider.unwraps++
	return llm.AdaptedResponse{}, provider.failure
}

func TestKnowledgeCachesResponseAdapterFailureAcrossMemoRows(t *testing.T) {
	base := &replacementProvider{response: []byte(`{"rows":[{"key":"r1","line":"Original first row."},{"key":"r2","line":"Original second row."}]}`)}
	executor := llm.Executor{Enabled: true, RootDir: t.TempDir()}
	outcome, err := llm.ExecuteJSON(t.Context(), executor, base, llm.Call[json.RawMessage]{
		State:  []byte(`{"contract":"knowledge-response-cache-test"}`),
		Prompt: llm.Prompt{System: "Return the saved test rows.", User: `{"table":"test"}`, ResponseFormatJSON: true},
		Limits: llm.Limits{MaxRequestBytes: llm.SemanticRecordByteLimit, MaxResponseBytes: llm.ProviderResponseByteLimit, MaxOutputTokens: llm.DefaultMaxOutputTokens},
	})
	if err != nil || outcome.CacheKey == "" {
		t.Fatalf("cache preparation: %v", err)
	}
	failure := errors.New("invalid result envelope")
	adapter := &rejectedRowAdapter{Provider: base, failure: failure}
	reader := &reader{opts: Options{Executor: executor, Provider: adapter}, responseTables: make(map[string]rememberedTable)}
	for _, key := range []string{"r1", "r2", "r1"} {
		_, found, err := reader.recallRow(table.Definition{}, table.Window{}, rememberedRow{RequestKey: outcome.CacheKey, RowKey: key})
		if found || !errors.Is(err, failure) {
			t.Fatalf("memo row %s acquired authority or changed failure: found=%v err=%v", key, found, err)
		}
	}
	if adapter.unwraps != 1 || adapter.accepted != 0 || len(reader.responseTables) != 1 || !errors.Is(reader.responseTables[outcome.CacheKey].err, failure) {
		t.Fatalf("one failed shared response was decoded repeatedly: unwraps=%d accepted=%d cache=%+v", adapter.unwraps, adapter.accepted, reader.responseTables)
	}
}

type parsedRowAdapter struct {
	llm.Provider
	parses   int
	accepted [][]string
}

func (provider *parsedRowAdapter) AdaptResponse(_, _, response []byte) (llm.AdaptedResponse, error) {
	provider.parses++
	return llm.AdaptedResponse{Domain: response, Rejections: []llm.ResponseRejection{{Kind: "metadata_rejected", Count: 1, Reason: "test metadata"}}, Accept: func(rows []string) { provider.accepted = append(provider.accepted, append([]string(nil), rows...)) }}, nil
}
func TestKnowledgeParsesSharedAdjunctOnceAndAcceptsOnlyValidatedRows(t *testing.T) {
	base := &replacementProvider{response: []byte(`{"extra":true,"rows":[{"key":"r1","line":"First.","extra":{"ignored":true}},{"key":"r2","line":"Second."},{"key":"r3","line":42},{"key":"r4","line":"Duplicate."},{"key":"r4","line":"Duplicate."},{"key":42}]}`)}
	executor := llm.Executor{Enabled: true, RootDir: t.TempDir()}
	outcome, err := llm.ExecuteJSON(t.Context(), executor, base, llm.Call[json.RawMessage]{State: []byte(`{"test":"parsed-memo"}`), Prompt: llm.Prompt{User: `{}`}, Limits: llm.Limits{MaxRequestBytes: 100000, MaxResponseBytes: 100000, MaxOutputTokens: 1000}})
	if err != nil {
		t.Fatal(err)
	}
	var events []llm.Event
	executor.Observer = llm.ObserverFunc(func(event llm.Event) error { events = append(events, event); return nil })
	adapter := &parsedRowAdapter{Provider: base}
	reader := &reader{opts: Options{Executor: executor, Provider: adapter}, responseTables: make(map[string]rememberedTable)}
	def := table.Definition{Stage: "atlas_files", Independent: true, Columns: []table.Column{{Name: "line", Kind: table.Text}}}
	window := table.Window{Rows: []table.Row{{ID: "current-source"}}}
	for _, key := range []string{"r1", "r3", "r4", "r2", "r1"} {
		_, found, err := reader.recallRow(def, window, rememberedRow{RequestKey: outcome.CacheKey, RowKey: key})
		if key == "r3" || key == "r4" {
			if found || err == nil {
				t.Fatal("invalid domain row accepted")
			}
			continue
		}
		if err != nil || !found {
			t.Fatalf("valid memo lost: %s / %v", key, err)
		}
	}
	if adapter.parses != 1 || !reflect.DeepEqual(adapter.accepted, [][]string{{"r1"}, {"r2"}, {"r1"}}) {
		t.Fatalf("adjunct reparsed or wrong rows accepted: %+v", adapter)
	}
	if len(events) != 1 || events[0].Kind != llm.EventCacheHit || events[0].Source != llm.SourceCache || len(events[0].ResponseRejections) != 1 {
		t.Fatalf("duplicate/live metadata event: %+v", events)
	}
}

func TestKnowledgeMemoCollectsOnlyAcceptedRowFromOriginalLocalContext(t *testing.T) {
	base := &replacementProvider{response: []byte(`{"rows":[{"key":"r1","line":"Alpha is unrelated."},{"key":"r2","line":"Beta is the accepted concept."},{"key":"r3","line":42}]}`)}
	executor := llm.Executor{Enabled: true, RootDir: t.TempDir()}
	def := table.Definition{Stage: "atlas_files", Contract: "local-context-test", System: "Describe each original source.", Independent: true, Columns: []table.Column{{Name: "line", Kind: table.Prose}}}
	rows := []table.Row{
		{ID: "a", Fields: []table.Field{{Name: "path", Value: "a.py"}, {Name: "line", Value: 3}}},
		{ID: "b", Fields: []table.Field{{Name: "path", Value: "b.py"}, {Name: "line", Value: 9}}},
		{ID: "bad", Fields: []table.Field{{Name: "path", Value: "bad.py"}}},
	}
	original := table.Window{Rows: rows}
	var err error
	original.Request, err = table.Request(def, original)
	if err != nil {
		t.Fatal(err)
	}
	call, err := table.Call(def, original)
	if err != nil {
		t.Fatal(err)
	}
	cold := terminology.NewCollector([]string{"a.py", "b.py", "bad.py"})
	outcome, err := llm.ExecuteJSON(t.Context(), executor, cold.Wrap(base), call)
	if err != nil {
		t.Fatal(err)
	}
	// A fresh reader knows only its current isolated row. Memo r2 still belongs
	// to b.py:9 in the original complete window, never to an invented r1 scope.
	current := terminology.NewCollector([]string{"a.py", "b.py", "bad.py"})
	r := &reader{opts: Options{Executor: executor, Provider: current.Wrap(base)}, responseTables: make(map[string]rememberedTable)}
	window := table.Window{Rows: []table.Row{rows[1]}}
	if _, found, err := r.recallRow(def, window, rememberedRow{RequestKey: outcome.CacheKey, RowKey: "r3"}); err == nil || found {
		t.Fatal("refused row became memo prose")
	}
	if _, found, err := r.recallRow(def, window, rememberedRow{RequestKey: outcome.CacheKey, RowKey: "r2"}); err != nil || !found {
		t.Fatalf("valid original row lost: %v", err)
	}
	base.response = []byte(`{"terms":[{"name":"Beta","kind":"domain","explanation":"The concept in the accepted original row.","rows":["p1"]}]}`)
	if err := current.Generate(t.Context(), executor, base); err != nil {
		t.Fatal(err)
	}
	terms := current.Snapshot()
	if len(terms) != 1 || !reflect.DeepEqual(terms[0].Sources, []terminology.Source{{Path: "b.py", Line: 9}}) || !reflect.DeepEqual(terms[0].Origins, []terminology.Origin{{RequestSHA256: outcome.RequestSHA256, Row: "r2"}}) {
		t.Fatalf("memo glossary lost original row, anchor or request: %+v", terms)
	}
}
