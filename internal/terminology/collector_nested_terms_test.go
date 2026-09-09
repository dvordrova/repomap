package terminology

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/llm"
)

func typesCallForNestedTerms(t *testing.T) llm.Call[table.Answers] {
	t.Helper()
	def := lines.Types()
	windows, err := table.Windows(def, 0, []table.Row{
		{ID: "first", Fields: []table.Field{{Name: "path", Value: "api.py"}, {Name: "name", Value: "시세"}}},
		{ID: "second", Fields: []table.Field{{Name: "path", Value: "api.py"}, {Name: "name", Value: "가격"}}},
	})
	if err != nil || len(windows) != 1 {
		t.Fatalf("prepare types window: %+v / %v", windows, err)
	}
	call, err := table.Call(def, windows[0])
	if err != nil {
		t.Fatal(err)
	}
	return call
}

const nestedTermsTypeRows = `[{"key":"r2","line":"Represents a stock price.","alias":"Stock Price","key_symbol":"yes"},{"key":"r1","line":"HTTP carries the quote.","alias":"Stock Quote","key_symbol":"yes"}]`

func TestMisplacedOptionalTermsKeepValidatedTypeRowsAndRawCache(t *testing.T) {
	for _, outerTerms := range []string{"", `,"terms":[]`} {
		t.Run("outer"+outerTerms, func(t *testing.T) {
			response := []byte(`{"result":{"rows":` + nestedTermsTypeRows + `,"terms":[{"name":"HTTP","explanation":"Misplaced metadata must not be accepted.","sources":["g1"]}]}` + outerTerms + `}`)
			base := &testProvider{response: response}
			var events []llm.Event
			executor := llm.Executor{Enabled: true, RootDir: t.TempDir(), Observer: llm.ObserverFunc(func(event llm.Event) error {
				events = append(events, event)
				return nil
			})}
			call := typesCallForNestedTerms(t)
			want := table.Answers{
				{"line": "HTTP carries the quote.", "alias": "Stock Quote", "key_symbol": "yes"},
				{"line": "Represents a stock price.", "alias": "Stock Price", "key_symbol": "yes"},
			}
			var originalRequest []byte
			for attempt := 0; attempt < 2; attempt++ {
				collector := NewCollector([]string{"api.py"})
				value, err := llm.ExecuteJSON(t.Context(), executor, collector.Wrap(base), call)
				if err != nil || !reflect.DeepEqual(value.Value, want) || value.Cached != (attempt == 1) || !bytes.Equal(value.Response, response) {
					t.Fatalf("misplaced metadata changed valid rows, aliases or raw response: %+v / %v", value, err)
				}
				if attempt == 0 {
					originalRequest = value.Request
				} else if !bytes.Equal(originalRequest, value.Request) {
					t.Fatal("metadata filtering changed the exact request")
				}
				if len(collector.Snapshot()) != 0 {
					t.Fatal("misplaced definitions entered the accepted glossary")
				}
				if len(events) != attempt+1 || events[attempt].Failure != llm.FailureNone {
					t.Fatalf("optional metadata failed the main call: %+v", events)
				}
				found := false
				for _, rejection := range events[attempt].ResponseRejections {
					if rejection.Kind == "terminology_metadata_rejected" && rejection.Reason == "misplaced optional terms field" && rejection.Count == 1 && reflect.DeepEqual(rejection.Samples, []string{"result.terms"}) {
						found = true
					}
				}
				if !found {
					t.Fatalf("no precise diagnostic for misplaced metadata: %+v", events[attempt])
				}
				cached, exists, err := llm.CachedExchange(executor.RootDir, value.CacheKey)
				if err != nil || !exists || !bytes.Equal(cached.Response, response) {
					t.Fatal("raw cache was modified")
				}
			}
			if base.calls != 1 {
				t.Fatalf("warm validation made %d calls", base.calls)
			}
		})
	}
}

func TestIndependentTypeRowsKeepOnlyAcceptedRowTerminology(t *testing.T) {
	for name, domain := range map[string]string{
		"other result field": `{"rows":` + nestedTermsTypeRows + `,"terms":[],"unexpected":true}`,
		"row extra field":    `{"rows":[{"key":"r1","line":"HTTP carries the quote.","alias":"Stock Quote","key_symbol":"yes","extra":{}},{"key":"r2","line":"Represents a stock price.","alias":"Stock Price","key_symbol":"yes"}],"terms":[]}`,
		"invalid alias cell": `{"rows":[{"key":"r1","line":"HTTP carries the quote.","alias":42,"key_symbol":"yes"},{"key":"r2","line":"Represents a stock price.","alias":"Stock Price","key_symbol":"yes"}],"terms":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			base := &testProvider{}
			executor := llm.Executor{Enabled: true, RootDir: t.TempDir()}
			for attempt := 0; attempt < 2; attempt++ {
				collector := NewCollector([]string{"api.py"})
				wrapped := collector.Wrap(base)
				call := typesCallForNestedTerms(t)
				prepared, err := llm.Prepare(wrapped, call.Prompt, call.Limits)
				if err != nil {
					t.Fatal(err)
				}
				base.response = responseJSON(json.RawMessage(domain),
					termJSON("HTTP", "A transfer protocol.", sourceRef(t, collector, prepared.Bytes(), "api.py", "r1")),
					termJSON("stock price", "The amount paid for one share.", sourceRef(t, collector, prepared.Bytes(), "api.py", "r2")),
				)
				outcome, err := llm.ExecuteJSON(t.Context(), executor, wrapped, call)
				if err != nil || outcome.Cached != (attempt == 1) || outcome.Value[1]["alias"] != "Stock Price" {
					t.Fatalf("valid neighbouring type lost: %+v / %v", outcome, err)
				}
				wantTerms := 2
				if name == "invalid alias cell" {
					wantTerms = 1
					if outcome.Value[0] != nil {
						t.Fatal("invalid alias acquired accepted cells")
					}
				} else if outcome.Value[0]["alias"] != "Stock Quote" {
					t.Fatal("extra field rejected a valid type")
				}
				terms := collector.Snapshot()
				if len(terms) != wantTerms || wantTerms == 1 && (terms[0].Name != "stock price" || !reflect.DeepEqual(terms[0].Origins, []Origin{{RequestSHA256: outcome.RequestSHA256, Row: "r2"}})) {
					t.Fatalf("rejected row metadata entered the glossary or valid neighbour was lost: %+v", terms)
				}
			}
			if base.calls != 1 {
				t.Fatal("accepted independent neighbours did not reuse their original response")
			}
		})
	}
}

func TestMisplacedTermsCannotSupplyComputedOccurrences(t *testing.T) {
	collector := NewCollector([]string{"api.py"})
	wrapped, request := prepareForTest(t, collector, `{"path":"api.py"}`)
	response := []byte(`{"result":{"answer":"The source is loaded.","terms":[{"name":"MetadataOnly"}]},"terms":[{"name":"MetadataOnly","explanation":"This term occurs only in rejected metadata.","sources":["g1"]}]}`)
	adapted, err := llm.AdaptResponse(wrapped, request, response)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(adapted.Domain, []byte(`{"answer":"The source is loaded."}`)) {
		t.Fatalf("misplaced metadata retained main authority: %s", adapted.Domain)
	}
	adapted.Accepted(nil)
	if len(collector.Snapshot()) != 0 {
		t.Fatal("an occurrence from rejected metadata authorized another term")
	}
}

func TestOwningResultTermsFieldKeepsItsAuthority(t *testing.T) {
	type result struct {
		Answer string `json:"answer"`
		Terms  string `json:"terms"`
	}
	want := result{Answer: "HTTP carries the quote.", Terms: "Original computed terms"}
	base := &testProvider{response: responseJSON(want)}
	collector := NewCollector([]string{"api.py"})
	call := llm.Call[result]{State: []byte(`{"test":"owning-terms"}`), Prompt: llm.Prompt{
		User: `{"path":"api.py"}`, ResponseExample: `{"answer":"<answer>","terms":"<computed terms>"}`,
	}, Limits: llm.Limits{MaxRequestBytes: 100000, MaxResponseBytes: 100000, MaxOutputTokens: 1000}}
	value, err := llm.ExecuteJSON(t.Context(), llm.Executor{}, collector.Wrap(base), call)
	if err != nil || value.Value != want {
		t.Fatalf("adjunct removed legitimate owning terms: %+v / %v", value.Value, err)
	}
	adapted, err := llm.AdaptResponse(collector.Wrap(base), value.Request, value.Response)
	if err != nil || len(adapted.Rejections) != 0 {
		t.Fatalf("owning terms became optional metadata: %+v / %v", adapted.Rejections, err)
	}
	raw, _ := json.Marshal(want)
	if !bytes.Equal(adapted.Domain, raw) {
		t.Fatalf("original result bytes changed: %s", adapted.Domain)
	}
}
