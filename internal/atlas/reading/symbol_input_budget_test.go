package reading

import (
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

type symbolInputProvider struct {
	tableProvider
	symbolRequests [][]byte
}

func (p *symbolInputProvider) Complete(ctx context.Context, prepared llm.Prepared) (llm.Completion, error) {
	var request struct {
		Table string
		Fill  []table.Column
	}
	if err := json.Unmarshal(prepared.Bytes(), &request); err != nil {
		return llm.Completion{}, err
	}
	if request.Table == lines.StageSymbols && len(request.Fill) > 0 && request.Fill[0].Name == "key_symbol" {
		p.mu.Lock()
		p.symbolRequests = append(p.symbolRequests, append([]byte(nil), prepared.Bytes()...))
		p.mu.Unlock()
	}
	return p.tableProvider.Complete(ctx, prepared)
}

func TestSymbolsKeepAnOversizedEvidenceRowThroughExecutionAndCache(t *testing.T) {
	graph := withSymbols(t, twoTargetGraph(t))
	largeName := "Op" + itoa(4)
	largeID := atlas.SymbolID("svc/core/c.go", 14, largeName)
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
	def := lines.SymbolSelection(false)
	input, err := table.Request(def, table.Window{Rows: []table.Row{lines.SymbolRow(large, "File svc/core/c.go does things.")}})
	if err != nil {
		t.Fatal(err)
	}
	if size := len(def.System) + len(input); size <= table.DefaultInputBytes || size >= llm.SemanticRecordByteLimit {
		t.Fatalf("fixture must exceed the default packing budget and fit the real request envelope: %d", size)
	}
	t.Logf("complete symbol input: %d bytes; default packing target: %d", len(def.System)+len(input), table.DefaultInputBytes)
	before, err := json.Marshal(graph)
	if err != nil {
		t.Fatal(err)
	}
	cache := t.TempDir()
	provider := &symbolInputProvider{}
	opts := twoTargetOptions(t, graph, &provider.tableProvider)
	opts.Provider, opts.Executor, opts.Through = provider, readOptions(t, graph, provider, cache).Executor, lines.StageSymbols
	cold := readKnowledge(t, opts)
	if len(provider.symbolRequests) != 3 {
		t.Fatalf("expected ordinary neighbours, one complete oversized symbol, then ordinary neighbours; got %d symbol requests", len(provider.symbolRequests))
	}
	seen := make(map[string]bool)
	for _, raw := range provider.symbolRequests {
		var request struct {
			System string           `json:"_system"`
			Rows   []map[string]any `json:"rows"`
		}
		if err := json.Unmarshal(raw, &request); err != nil {
			t.Fatal(err)
		}
		var originals []table.Row
		for i, row := range request.Rows {
			name, _ := row["name"].(string)
			if seen[name] {
				t.Fatalf("symbol %s was repeated across windows", name)
			}
			seen[name] = true
			var original atlas.Place
			for _, place := range graph.Places {
				if place.Symbol != nil && place.Path == "svc/core/c.go" && place.Symbol.Decl.Name == name {
					original = place
					break
				}
			}
			if original.ID == "" {
				t.Fatalf("provider row has no original declaration: %s", name)
			}
			originals = append(originals, lines.SymbolRow(original, cold[original.Parent].Cells["line"]))
			if row["key"] != table.Key(i) {
				t.Fatalf("window-local ref changed: %v", row["key"])
			}
			if original.ID == largeID && len(request.Rows) != 1 {
				t.Fatal("oversized atomic symbol shares its window with a neighbour")
			}
		}
		want, err := table.Request(def, table.Window{Rows: originals})
		if err != nil {
			t.Fatal(err)
		}
		var expected struct{ Rows []map[string]any }
		if err := json.Unmarshal(want, &expected); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(request.Rows, expected.Rows) {
			t.Fatal("provider execution trimmed or changed a source observation, association, or field")
		}
		if len(request.Rows) > 1 && len(request.System)+len(want) > table.DefaultInputBytes {
			t.Fatal("ordinary neighbours stopped respecting the packing target")
		}
	}
	if len(seen) != 8 || cold["selection:"+largeID].Cells["key_symbol"] == "" || cold["selection:"+largeID].Source != atlas.SourceModel {
		t.Fatalf("not every declaration received an accepted interpretation: %d symbols, large source %q", len(seen), cold["selection:"+largeID].Source)
	}
	warmProvider := &symbolInputProvider{}
	warmOpts := readOptions(t, graph, warmProvider, cache)
	warmOpts.Targets, warmOpts.Through = opts.Targets, lines.StageSymbols
	warm := readKnowledge(t, warmOpts)
	if warmProvider.calls != 0 || len(warm) != len(cold) {
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
