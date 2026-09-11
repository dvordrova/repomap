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
)

type selectionProvider struct{ tableProvider }

func (p *selectionProvider) Complete(ctx context.Context, prepared llm.Prepared) (llm.Completion, error) {
	response, err := p.tableProvider.Complete(ctx, prepared)
	if err != nil {
		return response, err
	}
	var request struct {
		Table string
		Fill  []table.Column
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
		name, _ := request.Rows[i]["name"].(string)
		if request.Table == lines.StageSymbols {
			if request.Fill[0].Name == "key_symbol" {
				if name == "Op08" {
					row["key_symbol"], row["activation"], row["outbound"] = "no", "request", "c1 c2"
				}
				if name == "Op07" {
					row["activation"] = "unassessed"
				}
			} else if name == "Op01" {
				delete(row, "line")
			}
		}
		if request.Table == lines.StageOperations && name == "Op08" {
			row["activation"], row["entry"], row["name"], row["description"] = "request", "self", "submit job", "Accepts a job submission."
		}
	}
	response.Response, err = json.Marshal(output)
	return response, err
}

func TestClosedScopeAndRefusedCaptionKeepIndependentRoles(t *testing.T) {
	graph := withSymbols(t, twoTargetGraph(t))
	for i := range graph.Places {
		place := &graph.Places[i]
		if place.Symbol != nil && place.Symbol.Decl.Name == "Op08" {
			place.Symbol.Calls = []atlas.SymbolCall{
				{Name: "Op07", Kind: "calls", Resolution: "exact", CalleeIDs: []string{atlas.SymbolID("svc/core/c.go", 17, "Op07")}, Line: 24},
				{Name: "Client.Submit", Kind: "invokes_external", Line: 25, Values: []string{"jobs"}},
			}
		}
	}
	provider := &selectionProvider{tableProvider: tableProvider{openFor: map[string]string{"svc/core": "no"}}}
	opts := twoTargetOptions(t, graph, &provider.tableProvider)
	opts.Provider, opts.Budget = provider, true
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
		known[record.PlaceID] = record
	}
	first := atlas.SymbolID("svc/core/c.go", 11, "Op01")
	if known["selection:"+first].Cells["key_symbol"] != "yes" || known[first].ID != "" {
		t.Fatal("refused caption changed its accepted selection")
	}
	unassessed := atlas.SymbolID("svc/core/c.go", 17, "Op07")
	if known["selection:"+unassessed].Cells["activation"] != "unassessed" {
		t.Fatal("missing role became a negative finding")
	}
	operation := atlas.SymbolID("svc/core/c.go", 18, "Op08")
	if known["selection:"+operation].Cells["outbound"] != "c2" || known[operation].ID != "" {
		t.Fatal("non-key operation lost its independent outgoing call")
	}
	found := false
	for _, target := range result.Atlas.Targets {
		for _, box := range target.Boxes {
			for _, file := range box.Files {
				for _, symbol := range file.Symbols {
					if symbol.ID == operation {
						found = symbol.Activation == "request" && symbol.OperationSummary != "" && symbol.Line == ""
					}
				}
			}
		}
	}
	if !found {
		t.Fatal("closed non-key operation disappeared from the atlas")
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
