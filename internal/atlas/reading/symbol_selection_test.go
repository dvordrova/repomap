package reading

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/typesafe/typesafetest"
)

type selectionProvider struct{ tableProvider }

func (p *selectionProvider) Complete(ctx context.Context, prepared llm.Prepared) (llm.Completion, error) {
	response, err := p.tableProvider.Complete(ctx, prepared)
	if err != nil {
		return response, err
	}
	var request struct {
		Table string
		Rows  []map[string]any
	}
	var output struct{ Rows []map[string]string }
	if err := json.Unmarshal(prepared.Bytes(), &request); err != nil {
		return response, err
	}
	if err := json.Unmarshal(response.Response, &output); err != nil {
		return response, err
	}
	for i, row := range output.Rows {
		if name, _ := request.Rows[i]["name"].(string); request.Table == lines.StageSymbols && name == "Op01" {
			delete(row, "line")
		}
	}
	response.Response, err = json.Marshal(output)
	return response, err
}

func TestClosedScopeAndRefusedCaptionKeepIndependentRoles(t *testing.T) {
	graph := withSymbols(t, twoTargetGraph(t))
	op07 := graphPlaceID(t, graph, atlas.PlaceSymbol, "svc/core/c.go", 17, "Op07")
	for i := range graph.Places {
		place := &graph.Places[i]
		if place.Symbol != nil && place.Symbol.Decl.Name == "Op08" {
			place.Symbol.Calls = []atlas.SymbolCall{
				{Name: "Op07", Kind: "calls", Resolution: "exact", CalleeIDs: []string{op07}, Line: 24},
				{Name: "Client.Submit", Kind: "invokes_external", Line: 25, Values: []string{"jobs"}},
			}
		}
	}
	provider := &selectionProvider{tableProvider: tableProvider{openFor: map[string]string{"svc/core": "no"}}}
	opts := twoTargetOptions(t, graph, &provider.tableProvider)
	opts.Provider, opts.Budget = provider, true
	decide := closedDecisions().Decide
	opts.Categorizer = &typesafetest.Categorizer{Decide: func(key string, question llm.Question) (llm.Verdict, bool) {
		if question.Item["name"] == "Op08" {
			return typesafetest.Choose("no"), true
		}
		return decide(key, question)
	}}
	result, err := Read(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete {
		t.Fatal("ordinary reading did not finish")
	}
	raw, err := os.ReadFile(filepath.Join(opts.OwnerRunDir, KnowledgeFilename))
	if err != nil {
		t.Fatal(err)
	}
	var saved struct{ Records []Knowledge }
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	known := make(map[string]Knowledge)
	for _, record := range saved.Records {
		key := record.PlaceID
		if record.Stage == lines.StageSymbols && record.Cells["key_symbol"] != "" {
			key = "selection:" + record.PlaceID
		}
		known[key] = record
	}
	first := graphPlaceID(t, graph, atlas.PlaceSymbol, "svc/core/c.go", 11, "Op01")
	if known["selection:"+first].Cells["key_symbol"] != "yes" || known[first].ID != "" {
		t.Fatal("refused caption changed its accepted selection")
	}
	dropped := op07
	if known["selection:"+dropped].Cells["key_symbol"] != "yes" {
		t.Fatal("a key decision was lost")
	}
}

func TestLearnRetainsUncaptionedKeyWithItsOriginalSelection(t *testing.T) {
	graph := knowledgeGraph(t)
	var symbol atlas.Place
	for _, place := range graph.Places {
		if place.Symbol != nil && place.Symbol.Decl.Name == "Main" {
			symbol = place
			break
		}
	}
	if symbol.ID == "" {
		t.Fatal("missing fixture symbol")
	}
	choice := &Knowledge{ID: "selection-evidence", SubjectID: symbol.Symbol.Decl.ObjectID, Cells: table.Answer{"key_symbol": "yes"}}
	r := reader{opts: Options{Graph: graph}, places: map[string]atlas.Place{},
		knowledgeSubjects: map[string]*Knowledge{choice.SubjectID: choice}, symbolSelections: map[string]*Knowledge{choice.SubjectID: choice}}
	for _, place := range graph.Places {
		r.places[place.ID] = place
	}
	for _, item := range r.learningEvidence() {
		if item.Source.SubjectID != choice.SubjectID {
			continue
		}
		if len(item.Source.KnowledgeIDs) != 1 || item.Source.KnowledgeIDs[0] != choice.ID || item.Source.Path != symbol.Path || item.Source.Evidence == nil {
			t.Fatalf("uncaptioned key lost its original evidence or duplicated knowledge: %+v", item.Source)
		}
		return
	}
	t.Fatal("uncaptioned key disappeared from Learn")
}
