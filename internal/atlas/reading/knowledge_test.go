package reading

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/llm"
)

type replacementProvider struct {
	tableProvider
	response []byte
}

func (p *replacementProvider) Complete(context.Context, llm.Prepared) (llm.Completion, error) {
	return llm.Completion{Response: p.response, ChoiceCount: 1, FinishReason: llm.FinishStop, Metrics: llm.Metrics{Attempts: 1}}, nil
}

type distinctDescriptionProvider struct {
	tableProvider
	symbolRows int
}

func (p *distinctDescriptionProvider) Complete(ctx context.Context, prepared llm.Prepared) (llm.Completion, error) {
	completion, err := p.tableProvider.Complete(ctx, prepared)
	if err != nil {
		return completion, err
	}
	var request struct{ Table string }
	if err := json.Unmarshal(prepared.Bytes(), &request); err != nil || request.Table != lines.StageSymbols {
		return completion, err
	}
	var response struct{ Rows []map[string]string }
	if err := json.Unmarshal(completion.Response, &response); err != nil {
		return completion, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, row := range response.Rows {
		p.symbolRows++
		row["line"] = fmt.Sprintf("Accepted description number %d.", p.symbolRows)
	}
	completion.Response, err = json.Marshal(map[string]any{"rows": response.Rows})
	return completion, err
}

func TestKnowledgeCoalescesExactInputsWithoutLosingSourceBindings(t *testing.T) {
	graph := knowledgeGraph(t)
	firstID := atlas.SymbolID("pkg/a/y.go", 3, "help")
	secondID := atlas.SymbolID("pkg/a/y.go", 8, "help")
	for _, place := range graph.Places {
		if place.ID != firstID {
			continue
		}
		other := place
		facts := *place.Symbol
		other.ID, other.LineNo, other.Symbol = secondID, 8, &facts
		other.Symbol.Decl.LineNo = 8
		other.Symbol.Decl.ObjectID = "different-native-object"
		graph.Places = append(graph.Places, other)
		break
	}
	atlas.SortPlaces(graph.Places)
	cache := t.TempDir()
	provider := &distinctDescriptionProvider{}
	opts := readOptions(t, graph, provider, cache)
	opts.Through, opts.WindowRows = lines.StageSymbols, 1
	first := readKnowledge(t, opts)
	a, b := first[firstID], first[secondID]
	if provider.symbolRows != 3 {
		t.Fatalf("provider saw %d symbol rows, want three distinct inputs for four declarations", provider.symbolRows)
	}
	if a.ID == b.ID || a.SubjectID == b.SubjectID || a.Line != 3 || b.Line != 8 ||
		a.ContextID != atlas.FileID("pkg/a/y.go") || b.ContextID != a.ContextID ||
		a.BasisID != b.BasisID || !reflect.DeepEqual(a.Cells, b.Cells) ||
		a.OriginRequest != b.OriginRequest || a.OriginResponse != b.OriginResponse {
		t.Fatalf("shared answer lost distinct current source bindings: %+v / %+v", a, b)
	}
	journal, err := os.ReadFile(filepath.Join(opts.OwnerRunDir, atlas.TablesFilename))
	if err != nil || !strings.Contains(string(journal), "Shared exact input "+secondID+" · representative "+firstID) {
		t.Fatalf("shared source has no explicit journal binding: %v", err)
	}
	warmProvider := &distinctDescriptionProvider{}
	warmOpts := readOptions(t, graph, warmProvider, cache)
	warmOpts.Through, warmOpts.WindowRows = lines.StageSymbols, 3
	warm := readKnowledge(t, warmOpts)
	if warmProvider.calls != 0 || warmProvider.symbolRows != 0 {
		t.Fatal("unchanged warm reading repeated an exact description input")
	}
	for id, record := range first {
		if record.ID != warm[id].ID || !reflect.DeepEqual(record.Cells, warm[id].Cells) {
			t.Fatalf("warm reading changed a current hint or binding: %s", id)
		}
	}
	replayLine := func(record Knowledge, text string) {
		t.Helper()
		ref, found, err := llm.LoadMemo(opts.Executor, record.BasisID, llm.DecodeJSON[rememberedRow](nil))
		if err != nil || !found {
			t.Fatalf("memo: %v", err)
		}
		exchange, found, err := llm.CachedExchange(cache, ref.RequestKey)
		if err != nil || !found {
			t.Fatalf("exchange: %v", err)
		}
		var response struct{ Rows []map[string]string }
		if err := json.Unmarshal(exchange.Response, &response); err != nil {
			t.Fatal(err)
		}
		for _, row := range response.Rows {
			if row["key"] == ref.RowKey {
				row["line"] = text
			}
		}
		raw, _ := json.Marshal(map[string]any{"rows": response.Rows})
		prepared, _ := llm.NewPrepared(exchange.Request)
		if _, err := llm.ReplayJSON(t.Context(), opts.Executor, &replacementProvider{response: raw}, prepared); err != nil {
			t.Fatal(err)
		}
	}
	replayLine(a, "Updated shared description.")
	updatedProvider := &distinctDescriptionProvider{}
	updatedOpts := readOptions(t, graph, updatedProvider, cache)
	updatedOpts.Through = lines.StageSymbols
	updated := readKnowledge(t, updatedOpts)
	if updatedProvider.calls != 0 {
		t.Fatal("replay required a new description request")
	}
	for _, id := range []string{firstID, secondID} {
		if updated[id].Cells["line"] != "Updated shared description." || updated[id].ID == first[id].ID ||
			updated[id].SubjectID != first[id].SubjectID || updated[id].Line != first[id].Line {
			t.Fatalf("replay did not refresh the original subject: %s", id)
		}
	}
	// A changed parent's actual line is a new input, even when the source
	// name and declaration signature remain identical to their prior values.
	replayLine(first[atlas.FileID("pkg/a/y.go")], "A different parent-file purpose.")
	changedProvider := &distinctDescriptionProvider{}
	changedOpts := readOptions(t, graph, changedProvider, cache)
	changedOpts.Through = lines.StageSymbols
	changed := readKnowledge(t, changedOpts)
	if changedProvider.calls != 1 || changedProvider.symbolRows != 1 {
		t.Fatalf("changed parent input requires one new shared decision: calls=%d rows=%d", changedProvider.calls, changedProvider.symbolRows)
	}
	for _, id := range []string{firstID, secondID} {
		if changed[id].BasisID == updated[id].BasisID || !strings.Contains(string(changed[id].Input), "A different parent-file purpose.") {
			t.Fatal("different parent hypothesis reused an old input")
		}
	}
	if changed[firstID].ID == changed[secondID].ID || !reflect.DeepEqual(changed[firstID].Cells, changed[secondID].Cells) {
		t.Fatal("new shared decision merged native subjects")
	}
	refusedProvider := &tableProvider{refuse: map[string]bool{"pkg/a/y.go": true}}
	refusedOpts := readOptions(t, graph, refusedProvider, t.TempDir())
	refusedOpts.Through, refusedOpts.WindowRows = lines.StageSymbols, 1
	refused, err := Read(t.Context(), refusedOpts)
	if err != nil {
		t.Fatal(err)
	}
	var symbols atlas.StageUse
	for _, use := range refused.Uses {
		if use.Stage == lines.StageSymbols {
			symbols = use
		}
	}
	if symbols.Rows != 4 || symbols.Given != 2 || symbols.Windows != 3 || symbols.Rejected != 1 {
		t.Fatalf("refused shared row lost original source accounting: %+v", symbols)
	}
}

func TestKnowledgeReadsReplayedRowsWithoutRepeatingAnalysis(t *testing.T) {
	cache := t.TempDir()
	graph := knowledgeGraph(t)
	opts := readOptions(t, graph, &tableProvider{}, cache)
	opts.Through, opts.WindowRows = lines.StageSymbols, 2
	first := readKnowledge(t, opts)
	symbolID := atlas.SymbolID("pkg/a/y.go", 3, "help")
	ref, found, err := llm.LoadMemo(opts.Executor, first[symbolID].BasisID, llm.DecodeJSON[rememberedRow](nil))
	if err != nil || !found {
		t.Fatalf("row reference: %v", err)
	}
	exchange, found, err := llm.CachedExchange(cache, ref.RequestKey)
	if err != nil || !found {
		t.Fatalf("original exchange: %v", err)
	}
	var envelope struct {
		Rows []map[string]string `json:"rows"`
	}
	if err := json.Unmarshal(exchange.Response, &envelope); err != nil {
		t.Fatal(err)
	}
	for _, row := range envelope.Rows {
		if row["key"] == ref.RowKey {
			row["line"] = "Decodes the response."
		}
	}
	raw, _ := json.Marshal(envelope)
	prepared, _ := llm.NewPrepared(exchange.Request)
	if _, err := llm.ReplayJSON(t.Context(), opts.Executor, &replacementProvider{response: raw}, prepared); err != nil {
		t.Fatal(err)
	}
	provider := &tableProvider{}
	opts = readOptions(t, graph, provider, cache)
	opts.Through, opts.WindowRows = lines.StageSymbols, 1
	updated := readKnowledge(t, opts)
	if provider.calls != 0 {
		t.Fatalf("replayed response caused %d extra calls", provider.calls)
	}
	if updated[symbolID].Cells["line"] != "Decodes the response." || updated[symbolID].OriginResponse == first[symbolID].OriginResponse || updated[symbolID].ID == first[symbolID].ID {
		t.Fatal("entity description did not follow the current response")
	}
	for id, previous := range first {
		if id != symbolID && updated[id].ID != previous.ID {
			t.Fatalf("replay changed an unrelated entity: %s", id)
		}
	}
	// Invalid row choices are checked by the owning table when resolving the
	// memo, even though replay itself only knows the provider/JSON contract.
	for _, row := range envelope.Rows {
		if row["key"] == ref.RowKey {
			row["key_symbol"] = "invented-choice"
		}
	}
	raw, _ = json.Marshal(envelope)
	if _, err := llm.ReplayJSON(t.Context(), opts.Executor, &replacementProvider{response: raw}, prepared); err != nil {
		t.Fatal(err)
	}
	provider = &tableProvider{}
	opts = readOptions(t, graph, provider, cache)
	opts.Through, opts.WindowRows = lines.StageSymbols, 1
	corrected := readKnowledge(t, opts)
	if provider.calls != 1 || corrected[symbolID].Cells["key_symbol"] == "invented-choice" {
		t.Fatal("invalid replay row bypassed table validation")
	}
}

func knowledgeGraph(t *testing.T) atlas.Graph {
	t.Helper()
	graph := testGraph(t)
	var symbols []atlas.Place
	for i := range graph.Places {
		file := &graph.Places[i]
		if file.File == nil {
			continue
		}
		for j := range file.File.Decls {
			decl := &file.File.Decls[j]
			decl.ObjectID = "object-" + digest([]byte(file.Path+decl.Name))
			symbols = append(symbols, atlas.Place{
				ID: atlas.SymbolID(file.Path, decl.LineNo, decl.Name), Kind: atlas.PlaceSymbol,
				Path: file.Path, LineNo: decl.LineNo, Parent: file.ID, TargetIDs: file.TargetIDs,
				Symbol: &atlas.SymbolFacts{Decl: *decl, Candidate: true, Rank: j + 1},
			})
		}
	}
	graph.Places = append(graph.Places, symbols...)
	atlas.SortPlaces(graph.Places)
	return graph
}

func TestTypeMemberDocumentationInvalidatesOnlyItsDescription(t *testing.T) {
	graph := knowledgeGraph(t)
	id := atlas.SymbolID("pkg/a/y.go", 3, "help")
	var typePlace *atlas.Place
	for i := range graph.Places {
		if graph.Places[i].ID == id {
			typePlace = &graph.Places[i]
		}
	}
	if typePlace == nil {
		t.Fatal("missing fixture declaration")
	}
	typePlace.Symbol.Decl.Kind = "type"
	typePlace.Symbol.Members = []atlas.TypeMember{{Path: "pkg/a/x.go", Decl: atlas.Decl{Name: "Renew", Kind: "method", LineNo: 6, Doc: "Renews validity."}}}
	cache := t.TempDir()
	opts := readOptions(t, graph, &tableProvider{}, cache)
	opts.Through = lines.StageSymbols
	first := readKnowledge(t, opts)
	typePlace.Symbol.Members[0].Decl.Doc = "Renews validity for the supplied interval."
	provider := &tableProvider{}
	opts = readOptions(t, graph, provider, cache)
	opts.Through = lines.StageSymbols
	second := readKnowledge(t, opts)
	if provider.calls != 1 || first[id].BasisID == second[id].BasisID {
		t.Fatal("changed member documentation did not invalidate exactly the type row")
	}
	for key, record := range first {
		if key != id && second[key].ID != record.ID {
			t.Fatalf("unrelated knowledge changed: %s", key)
		}
	}
	typePlace.Symbol.Members = nil
	typePlace.Symbol.Decl.Doc = ""
	provider = &tableProvider{}
	opts = readOptions(t, graph, provider, cache)
	opts.Through = lines.StageSymbols
	third := readKnowledge(t, opts)
	if _, exists := third[id]; exists || provider.calls != 0 {
		t.Fatal("a bare type name reused or requested an unsupported explanation")
	}
}

func readKnowledge(t *testing.T, opts Options) map[string]Knowledge {
	t.Helper()
	if _, err := Read(t.Context(), opts); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(opts.OwnerRunDir, KnowledgeFilename))
	if err != nil {
		t.Fatal(err)
	}
	var artifact struct {
		Version int         `json:"version"`
		Records []Knowledge `json:"records"`
	}
	if err := json.Unmarshal(raw, &artifact); err != nil || artifact.Version != KnowledgeVersion {
		t.Fatalf("knowledge artifact: %v, %s", err, raw)
	}
	result := make(map[string]Knowledge)
	ids := make(map[string]bool)
	for _, record := range artifact.Records {
		result[record.PlaceID], ids[record.ID] = record, true
		if record.SubjectID == "" || record.BasisID == "" || len(record.Input) == 0 || record.Cells["line"] == "" || record.OriginRequest == "" {
			t.Fatalf("unbound interpretation: %+v", record)
		}
	}
	for _, record := range artifact.Records {
		for _, dependency := range record.DependsOn {
			if !ids[dependency] {
				t.Fatalf("%s has a missing model dependency %s", record.PlaceID, dependency)
			}
		}
	}
	return result
}

func TestKnowledgeSurvivesBatchChangesAndInvalidatesOnlyChangedBasis(t *testing.T) {
	cache := t.TempDir()
	graph := knowledgeGraph(t)
	provider := &tableProvider{}
	opts := readOptions(t, graph, provider, cache)
	opts.Through, opts.WindowRows = lines.StageSymbols, 2
	first := readKnowledge(t, opts)
	if len(first) != 10 || provider.calls == 0 {
		t.Fatalf("expected four directories, three files and three symbols: %d records, %d calls", len(first), provider.calls)
	}
	// The same entities move from two rows per batch to one, under a different
	// context budget. Their previously accepted interpretations survive intact.
	again := &tableProvider{}
	opts = readOptions(t, graph, again, cache)
	opts.Through, opts.WindowRows, opts.InputBytes = lines.StageSymbols, 1, 4000
	warm := readKnowledge(t, opts)
	if again.calls != 0 {
		t.Fatalf("rebatching reanalysed known entities: %d calls", again.calls)
	}
	for id, record := range first {
		if warm[id].ID != record.ID || warm[id].Source != atlas.SourceCache {
			t.Fatalf("knowledge changed when only its batch changed: %s", id)
		}
	}
	// Edit only one leaf file's author documentation. The file's fake
	// model wording stays identical: the symbol can reuse its answer, while
	// its provenance must point to the newly interpreted file.
	changedFile := atlas.FileID("pkg/a/y.go")
	changedSymbol := atlas.SymbolID("pkg/a/y.go", 3, "help")
	for i := range graph.Places {
		place := &graph.Places[i]
		if place.ID == changedFile {
			place.File.Doc = "This file decodes responses."
		}
	}
	changed := &tableProvider{}
	opts = readOptions(t, graph, changed, cache)
	opts.Through = lines.StageSymbols
	updated := readKnowledge(t, opts)
	if changed.answers["pkg/a/y.go"] != 1 || changed.answers["pkg/a/x.go"] != 0 || changed.answers["pkg/b/z.go"] != 0 {
		t.Fatalf("change did not stay with its affected entities: %v", changed.answers)
	}
	for id, record := range first {
		shouldChange := id == changedFile || id == changedSymbol
		if (updated[id].ID != record.ID) != shouldChange {
			t.Fatalf("unexpected invalidation for %s: changed=%v", id, updated[id].ID != record.ID)
		}
	}
	symbol := updated[changedSymbol]
	if symbol.Source != atlas.SourceCache || symbol.BasisID != first[changedSymbol].BasisID || symbol.OriginRequest != first[changedSymbol].OriginRequest {
		t.Fatal("unchanged symbol input should reuse the original answer")
	}
	if symbol.SubjectID == changedSymbol || symbol.ContextID != changedFile || len(symbol.DependsOn) != 1 || symbol.DependsOn[0] != updated[changedFile].ID {
		t.Fatalf("symbol lost its native subject or file context: %+v", symbol)
	}
	// When the parent's actual text changes, the symbol must be asked again.
	for i := range graph.Places {
		if graph.Places[i].ID == changedFile {
			graph.Places[i].File.Doc = "This file decodes responses and validates their schema."
		}
	}
	newWording := &tableProvider{fileLineFor: map[string]string{"pkg/a/y.go": "Decodes and validates responses."}}
	opts = readOptions(t, graph, newWording, cache)
	opts.Through = lines.StageSymbols
	reworded := readKnowledge(t, opts)
	if newWording.answers["pkg/a/y.go"] != 2 || reworded[changedSymbol].BasisID == symbol.BasisID || reworded[changedSymbol].Source != atlas.SourceModel {
		t.Fatal("changed parent text did not invalidate its dependent symbol answer")
	}
}

func TestKnowledgeRebindsChangedOwnersAndNativeSubjectsWithoutModelCalls(t *testing.T) {
	cache := t.TempDir()
	graph := knowledgeGraph(t)
	opts := readOptions(t, graph, &tableProvider{}, cache)
	opts.Through = lines.StageSymbols
	first := readKnowledge(t, opts)
	for i := range graph.Places {
		place := &graph.Places[i]
		place.TargetIDs = []string{"t2", "t1"}
		if place.Symbol != nil {
			place.Symbol.Decl.ObjectID = "new-scope-" + place.Symbol.Decl.ObjectID
		}
	}
	provider := &tableProvider{}
	opts = readOptions(t, graph, provider, cache)
	opts.Through, opts.WindowRows = lines.StageSymbols, 1
	opts.Targets = append(opts.Targets, TargetMeta{ID: "t2", Language: "go", Kind: "library", Name: "example.com/x", Root: "pkg/a"})
	updated := readKnowledge(t, opts)
	if provider.calls != 0 {
		t.Fatalf("ownership-only change made %d model calls", provider.calls)
	}
	for id, old := range first {
		current := updated[id]
		if current.Source != atlas.SourceCache || current.BasisID != old.BasisID || current.OriginRequest != old.OriginRequest || !reflect.DeepEqual(current.Cells, old.Cells) {
			t.Fatalf("ownership change discarded an unchanged answer: %s", id)
		}
		if current.ID == old.ID || !reflect.DeepEqual(current.TargetIDs, []string{"t1", "t2"}) {
			t.Fatalf("reused answer kept the old ownership binding: %s", id)
		}
		if strings.HasPrefix(id, "sym:") && current.SubjectID != "new-scope-"+old.SubjectID {
			t.Fatalf("reused symbol lost its current native subject: %s", id)
		}
		if len(current.DependsOn) > 0 && current.DependsOn[0] != updated[current.ContextID].ID {
			t.Fatalf("reused answer kept an obsolete parent: %s", id)
		}
	}
}

func TestQuestionReusesEntityKnowledgeWithoutDescriptionCalls(t *testing.T) {
	cache := t.TempDir()
	graph := knowledgeGraph(t)
	first := readOptions(t, graph, &tableProvider{}, cache)
	first.Through = lines.StageSymbols
	records := readKnowledge(t, first)
	provider := &tableProvider{questionFor: map[string]table.Answer{
		"pkg/a/x.go": {"relevance": "direct", "anchors": "a1", "why": "Inspect Main and its author documentation."},
	}}
	opts := readOptions(t, graph, provider, cache)
	opts.Through, opts.Questions = lines.StageQuestion, []string{"Where is the entry point?"}
	result, route := readQuestionResult(t, opts)
	if provider.calls != 1 || len(route.Stops) == 0 {
		t.Fatalf("question made extra description calls: %d; stops %d", provider.calls, len(route.Stops))
	}
	reused := 0
	for _, use := range result.Uses {
		if use.Stage != lines.StageQuestion {
			if use.Live != 0 || use.Windows != 0 {
				t.Fatalf("question ran a description window: %+v", use)
			}
			reused += use.Reused
		}
	}
	if reused != len(records) {
		t.Fatalf("question reused %d of %d entity descriptions", reused, len(records))
	}
	requests, _ := filepath.Glob(filepath.Join(opts.OwnerRunDir, atlas.TablesDir, "atlas_question-*.input.ref.json"))
	raw, _ := readWindowPayload(requests[0])
	if !strings.Contains(string(raw), "prior_model_hypothesis") || !strings.Contains(string(raw), "file_model_hypothesis") || hexID.Match(raw) {
		t.Fatalf("request lost labelled knowledge or leaked internal IDs: %s", raw)
	}
	for _, stop := range route.Stops {
		if stop.Path == "pkg/a/x.go" && len(stop.KnowledgeIDs) != 2 {
			t.Fatalf("question lost function and file knowledge bindings: %+v", stop)
		}
	}
}

func TestKnowledgeScopeAndNoCache(t *testing.T) {
	cache := t.TempDir()
	graph := knowledgeGraph(t)
	initial := readOptions(t, graph, &tableProvider{}, cache)
	initial.Through = lines.StageSymbols
	original := readKnowledge(t, initial)
	for _, change := range []string{"repository", "no-cache"} {
		t.Run(change, func(t *testing.T) {
			provider := &tableProvider{}
			opts := readOptions(t, graph, provider, cache)
			opts.Through = lines.StageSymbols
			if change == "repository" {
				opts.Repository = "another repository"
			} else {
				opts.Executor.Enabled = false
			}
			records := readKnowledge(t, opts)
			if change == "repository" && provider.calls != 0 {
				t.Fatal("identical model input was reanalysed only because the repository label changed")
			}
			for _, record := range records {
				// Exact whole-request cache reuse across repositories is permitted;
				// entity interpretations must nevertheless have distinct identity.
				if change == "no-cache" && record.Source != atlas.SourceModel {
					t.Fatal("--no-cache recalled an entity interpretation")
				}
				if change == "repository" && original[record.PlaceID].ID == record.ID {
					t.Fatal("another repository reused the same knowledge identity")
				}
			}
			if change == "no-cache" && provider.calls == 0 {
				t.Fatal("--no-cache made no live requests")
			}
		})
	}
}
