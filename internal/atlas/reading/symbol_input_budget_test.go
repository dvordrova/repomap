package reading

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/typesafe/typesafetest"
)

// symbolQuestions keeps every request the categorizer answers.
type symbolQuestions struct {
	*typesafetest.Categorizer
	mu       sync.Mutex
	requests [][]byte
}

func (c *symbolQuestions) Complete(ctx context.Context, prepared llm.Prepared) (llm.Completion, error) {
	c.mu.Lock()
	c.requests = append(c.requests, append([]byte(nil), prepared.Bytes()...))
	c.mu.Unlock()
	return c.Categorizer.Complete(ctx, prepared)
}

func TestSymbolsKeepAnOversizedEvidenceRowThroughExecutionAndCache(t *testing.T) {
	graph := withSymbols(t, twoTargetGraph(t))
	largeName := "Op" + itoa(4)
	largeID := graphPlaceID(t, graph, atlas.PlaceSymbol, "svc/core/c.go", 14, largeName)
	var large atlas.Place
	for i := range graph.Places {
		place := &graph.Places[i]
		if place.ID != largeID {
			continue
		}
		// These are distinct observations, not repeated text which the existing
		// evidence catalogue could factor down below the packing target.
		for j := 0; j < 160; j++ {
			evidence := []atlas.EdgeEvidence{{Extractor: "interface_field_assignment", Path: "svc/core/c.go", LineNo: 100 + j,
				Label: fmt.Sprintf("candidate %d: %s", j, strings.Repeat("source-distinct interface witness ", 16))}}
			place.Symbol.Calls = append(place.Symbol.Calls, atlas.SymbolCall{
				Name: fmt.Sprintf("Candidate%d.Compare", j), Line: 300 + j, Resolution: "alternatives", Evidence: evidence,
			})
			place.Symbol.Bindings = append(place.Symbol.Bindings, atlas.SymbolBinding{
				From: largeName, To: fmt.Sprintf("Candidate%d.Compare", j), Path: place.Path, Line: 300 + j,
				Resolution: "alternatives", Evidence: evidence,
			})
		}
		large = *place
	}
	if large.ID == "" {
		t.Fatal("missing large symbol fixture")
	}
	def := table.ForClassifier(lines.SymbolSelection(false))
	alone, err := table.ClassifierCall(closedDecisions(), def, table.Window{Rows: []table.Row{lines.SymbolRow(large, "File svc/core/c.go does things.")}})
	if err != nil {
		t.Fatal(err)
	}
	if size := len(alone.Prompt.User); size <= table.ClassifierBodyBytes || size >= llm.SemanticRecordByteLimit {
		t.Fatalf("fixture must exceed the categorizer's body budget and fit the real request envelope: %d", size)
	}
	before, err := json.Marshal(graph)
	if err != nil {
		t.Fatal(err)
	}
	cache := t.TempDir()
	provider := &tableProvider{}
	questions := &symbolQuestions{Categorizer: closedDecisions()}
	opts := twoTargetOptions(t, graph, provider)
	opts.Categorizer, opts.Executor, opts.Through = questions, readOptions(t, graph, provider, cache).Executor, lines.StageSymbols
	cold := readKnowledge(t, opts)
	if len(questions.requests) < 2 {
		t.Fatalf("the oversized symbol did not get a request of its own: %d requests", len(questions.requests))
	}
	byID := make(map[string]atlas.Place)
	for _, place := range graph.Places {
		byID[place.ID] = place
	}
	seen := make(map[string]bool)
	for _, raw := range questions.requests {
		var request struct {
			Questions map[string]struct {
				Instructions struct {
					Row map[string]any `json:"row"`
				} `json:"instructions"`
			} `json:"questions"`
		}
		if err := json.Unmarshal(raw, &request); err != nil {
			t.Fatal(err)
		}
		for key, question := range request.Questions {
			id := strings.TrimSuffix(key, "|key_symbol")
			if seen[id] {
				t.Fatalf("symbol %s was repeated across requests", id)
			}
			seen[id] = true
			original, known := byID[id]
			if !known || original.Symbol == nil {
				t.Fatalf("question %s has no original declaration", key)
			}
			fields := map[string]any{}
			for _, field := range lines.SymbolRow(original, cold[original.Parent].Cells["line"]).Fields {
				fields[field.Name] = field.Value
			}
			encoded, _ := json.Marshal(fields)
			var want map[string]any
			if err := json.Unmarshal(encoded, &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(question.Instructions.Row, want) {
				t.Fatal("categorizer execution trimmed or changed a source observation, association, or field")
			}
			if id == largeID && len(request.Questions) != 1 {
				t.Fatal("oversized atomic symbol shares its request with a neighbour")
			}
		}
		if len(request.Questions) > 1 && len(raw) > table.ClassifierBodyBytes {
			t.Fatal("ordinary neighbours stopped respecting the body budget")
		}
	}
	if len(seen) != 8 || cold["selection:"+largeID].Cells["key_symbol"] == "" || cold["selection:"+largeID].Source != atlas.SourceModel {
		t.Fatalf("not every declaration received an accepted interpretation: %d symbols, large source %q", len(seen), cold["selection:"+largeID].Source)
	}
	warmProvider := &tableProvider{}
	warmOpts := readOptions(t, graph, warmProvider, cache)
	warmOpts.Targets, warmOpts.Through = opts.Targets, lines.StageSymbols
	warm := readKnowledge(t, warmOpts)
	if warmProvider.calls != 0 || warmOpts.Categorizer.(*typesafetest.Categorizer).Calls() != 0 || len(warm) != len(cold) {
		t.Fatalf("warm reading repeated paid work or lost entities: calls=%d, records=%d/%d", warmProvider.calls, len(warm), len(cold))
	}
	for id, record := range cold {
		reused := warm[id]
		if reused.Source != atlas.SourceCache || reused.ID != record.ID || reused.OriginRequest != record.OriginRequest ||
			reused.OriginResponse != record.OriginResponse || !reflect.DeepEqual(reused.Input, record.Input) || !reflect.DeepEqual(reused.Cells, record.Cells) {
			t.Fatalf("cache changed exact evidence, answer, or source identity for %s", id)
		}
	}
	after, err := json.Marshal(graph)
	if err != nil || string(before) != string(after) {
		t.Fatalf("reading changed original graph evidence: %v", err)
	}
}
