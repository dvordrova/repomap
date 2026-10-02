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

// Two of eight declarations are over the categorizer's envelope. Op3
// registers 400 routes through one outside API, as casdoor's InitAPI does:
// it is asked once, alone, in its packed form, and answered. Op4 holds 200
// distinct observations that no packing shortens and cannot fit even at
// the sparsest density: preparation refuses it with its measured size and
// the envelope, and it is never sent. Every other row is asked byte for
// byte as built, and a warm reading asks nothing.
func TestSymbolsPackARowOverTheEnvelopeAndRefuseOneThatCannotFit(t *testing.T) {
	graph := withSymbols(t, twoTargetGraph(t))
	routerID := graphPlaceID(t, graph, atlas.PlaceSymbol, "svc/core/c.go", 13, "Op"+itoa(3))
	largeName := "Op" + itoa(4)
	largeID := graphPlaceID(t, graph, atlas.PlaceSymbol, "svc/core/c.go", 14, largeName)
	api := &atlas.CallAPI{Package: "github.com/beego/beego/v2/server/web", Name: "Router", Signature: "func(rootpath string, c web.ControllerInterface, mappingMethods ...string) *web.HttpServer"}
	var router, large atlas.Place
	for i := range graph.Places {
		place := &graph.Places[i]
		switch place.ID {
		case routerID:
			for j := range 400 {
				place.Symbol.Calls = append(place.Symbol.Calls, atlas.SymbolCall{Kind: "invokes_external", Name: "web.Router", Line: 100 + j, API: api,
					Values: []string{fmt.Sprintf("/api/get-thing-%d", j), fmt.Sprintf("GET:GetThing%d", j)}})
				place.Symbol.Bindings = append(place.Symbol.Bindings, atlas.SymbolBinding{From: "Op3", To: "ApiController.Finish", Kind: "binds_implementation",
					Detail: "parameter 2 -> github.com/beego/beego/v2/server/web.Router; interface github.com/beego/beego/v2/server/web.ControllerInterface method Finish func()",
					Path:   place.Path, Line: 100 + j, Resolution: "exact"})
			}
			router = *place
		case largeID:
			// Distinct observations, not repeated text which the evidence
			// catalogue or the packing could factor down.
			for j := range 200 {
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
	}
	if router.ID == "" || large.ID == "" {
		t.Fatal("missing large symbol fixtures")
	}
	def := table.ForClassifier(lines.SymbolSelection(false))
	for _, fixture := range []struct {
		place atlas.Place
		over  int
	}{{router, table.ClassifierQuestionBytes}, {large, table.ClassifierQuestionCeiling}} {
		row := lines.SymbolRow(fixture.place, "File svc/core/c.go does things.")
		alone, err := table.ClassifierCall(closedDecisions(), def, table.Window{Rows: []table.Row{row}})
		if err != nil {
			t.Fatal(err)
		}
		if size := len(alone.Prompt.User); size <= fixture.over {
			t.Fatalf("fixture %s must be over %d bytes: %d", fixture.place.ID, fixture.over, size)
		}
		packed, err := table.ClassifierCall(closedDecisions(), def, table.Window{Rows: []table.Row{lines.PackSymbolRow(row)}})
		if err != nil {
			t.Fatal(err)
		}
		if fits := len(packed.Prompt.User) <= table.ClassifierQuestionBytes; fits != (fixture.place.ID == routerID) {
			t.Fatalf("fixture %s packed is %d bytes", fixture.place.ID, len(packed.Prompt.User))
		}
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
	result, err := Read(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	cold := knowledgeIn(t, opts.OwnerRunDir)
	byID := make(map[string]atlas.Place)
	for _, place := range graph.Places {
		byID[place.ID] = place
	}
	seen := make(map[string]bool)
	for _, raw := range questions.requests {
		var request struct {
			Questions map[string]struct {
				Instructions struct {
					Row json.RawMessage `json:"row"`
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
			if id == largeID {
				t.Fatal("a row that cannot fit was sent for the model to refuse")
			}
			row := lines.SymbolRow(original, cold[original.Parent].Cells["line"])
			if id == routerID {
				row = lines.PackSymbolRow(row)
				if len(request.Questions) != 1 {
					t.Fatal("the packed row shares its request with a neighbour")
				}
			}
			fields := map[string]any{}
			for _, field := range row.Fields {
				fields[field.Name] = field.Value
			}
			want, err := json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
			if string(question.Instructions.Row) != string(want) {
				t.Fatalf("categorizer request for %s is not the row as built (packed only over the envelope)", id)
			}
			if id != routerID && strings.Contains(string(want), `"lines":[`) && strings.Contains(string(want), `"shared":{"api"`) {
				t.Fatalf("a row that fits was packed: %s", id)
			}
		}
		if len(request.Questions) > 1 && len(raw) > table.ClassifierBodyBytes {
			t.Fatal("ordinary neighbours stopped respecting the body budget")
		}
	}
	if len(seen) != 7 || cold["selection:"+routerID].Cells["key_symbol"] == "" || cold["selection:"+routerID].Source != atlas.SourceModel {
		t.Fatalf("every declaration that fits, packed or not, must be answered: %d asked, packed row source %q", len(seen), cold["selection:"+routerID].Source)
	}
	if _, answered := cold["selection:"+largeID]; answered {
		t.Fatal("a refused row has an answer")
	}
	refused := 0
	for _, row := range result.Rejected {
		if row.Kind != "over_envelope" {
			continue
		}
		refused++
		if row.Stage != lines.StageSymbols || len(row.Samples) != 1 || row.Samples[0] != largeID ||
			!strings.Contains(row.Reason, "was not sent: even packed") || !strings.Contains(row.Reason, fmt.Sprintf("over Jev's envelope of %d and %d", table.ClassifierQuestionTokens, table.ClassifierRequestTokens)) {
			t.Fatalf("refusal does not record the row, its measured size and the envelope: %+v", row)
		}
	}
	if refused != 1 {
		t.Fatalf("refusals at preparation: %d, want 1: %+v", refused, result.Rejected)
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
