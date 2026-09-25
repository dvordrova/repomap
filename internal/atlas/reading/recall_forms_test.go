package reading

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/llm"
)

// cachedResponse stores one exact response in a fresh cache and returns its
// executor and key.
func cachedResponse(t *testing.T, response string) (llm.Executor, string) {
	t.Helper()
	executor := llm.Executor{Enabled: true, RootDir: t.TempDir()}
	outcome, err := llm.ExecuteJSON(t.Context(), executor, &replacementProvider{response: []byte(response)}, llm.Call[json.RawMessage]{
		State: []byte(`{"test":"recall-forms"}`), Prompt: llm.Prompt{User: `{}`},
		Limits: llm.Limits{MaxRequestBytes: 100000, MaxResponseBytes: 100000, MaxOutputTokens: 1000}})
	if err != nil {
		t.Fatal(err)
	}
	return executor, outcome.CacheKey
}

// A remembered response is read with the live envelope rules, and a row
// that lost a cell is recalled without that cell and without authorizing its
// text as glossary prose.
func TestRecallReadsTheLiveEnvelopeAndKeepsARowWithoutItsRefusedCell(t *testing.T) {
	executor, key := cachedResponse(t, `[{"key":"d1","title":"","line":"Starts the analysis."},{"key":"d2","title":"Report","line":"Renders."}]`)
	adapter := &parsedRowAdapter{Provider: &replacementProvider{}}
	recall := &reader{opts: Options{Executor: executor, Provider: adapter}, responseTables: make(map[string]rememberedTable)}
	def := table.Definition{Stage: "atlas_directories", Columns: []table.Column{
		{Name: "title", Kind: table.Text, Alone: true},
		{Name: "line", Kind: table.Text, Alone: true},
	}}
	window := table.Window{Rows: []table.Row{{ID: "current"}}}
	partial, found, err := recall.recallRow(def, window, rememberedRow{RequestKey: key, RowKey: "d1"})
	if err != nil || !found || !reflect.DeepEqual(partial.answer, table.Answer{"line": "Starts the analysis."}) || !partial.partial {
		t.Fatalf("a bare-array memo or a row without its refused cell was lost: %+v / %v", partial, err)
	}
	whole, found, err := recall.recallRow(def, window, rememberedRow{RequestKey: key, RowKey: "d2"})
	if err != nil || !found || whole.partial || whole.answer["title"] != "Report" {
		t.Fatalf("a complete row was not recalled whole: %+v / %v", whole, err)
	}
	if !reflect.DeepEqual(adapter.accepted, [][]string{{"d2"}}) {
		t.Fatalf("a row with a refused cell authorized glossary prose on recall: %v", adapter.accepted)
	}
	executor, key = cachedResponse(t, `{"notes":"no rows here"}`)
	recall = &reader{opts: Options{Executor: executor, Provider: adapter}, responseTables: make(map[string]rememberedTable)}
	if _, found, err := recall.recallRow(def, window, rememberedRow{RequestKey: key, RowKey: "d2"}); found || err == nil {
		t.Fatalf("a remembered response without rows was recalled: %v", err)
	}
}

// A remembered decision-model row the response answered uncertainly is found
// with no answer, not a recall error: the same response would leave it so. A
// row the response never answered is still an error.
func TestClassifierRecallFindsAnUncertainRowWithoutAnAnswer(t *testing.T) {
	executor, key := cachedResponse(t, `{"answers":{"s1|part":{"type":"choice","choice":"c1","probabilities":{"c1":0.4,"none":0.3}},"s2|part":{"type":"choice","choice":"c1","probabilities":{"c1":0.9}}}}`)
	def := table.Definition{Stage: "atlas_core", Classifier: true, Columns: []table.Column{{Name: "part", Kind: table.Choice, Options: []string{"c1", "none"}}}}
	recall := &reader{opts: Options{Executor: executor, Classifier: &replacementProvider{}}, classifierResponses: make(map[string]rememberedClassifier)}
	window := table.Window{Rows: []table.Row{{ID: "current"}}}
	uncertain, found, err := recall.recallRow(def, window, rememberedRow{RequestKey: key, RowKey: "s1"})
	if err != nil || !found || uncertain.answer != nil || uncertain.source != atlas.SourceCache {
		t.Fatalf("an uncertain remembered row was a recall error or gained an answer: %+v / %t / %v", uncertain, found, err)
	}
	decided, found, err := recall.recallRow(def, window, rememberedRow{RequestKey: key, RowKey: "s2"})
	if err != nil || !found || decided.answer["part"] != "c1" {
		t.Fatalf("a decided remembered row was lost: %+v / %v", decided, err)
	}
	if _, found, err := recall.recallRow(def, window, rememberedRow{RequestKey: key, RowKey: "s3"}); found || err == nil {
		t.Fatalf("a row the remembered response never answered was found: %t / %v", found, err)
	}
}
