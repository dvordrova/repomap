package reading

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/typesafe/typesafetest"
)

// apiSource is one C file whose functions call outside symbols; each call
// is its own statement, and comments stand beside some of them.
const apiSource = `int main(int argc, char **argv) {
    x = strcmp(a,"b"); /* why */
    fd = accept(s, &sa, &len); // take the next client
    printf("%s\n", name);
    return 0;
}
`

// apiCall is a call of an outside C symbol at the first occurrence of mark
// in apiSource, where the C adapter anchors it.
func apiCall(t *testing.T, header, name, mark string, values ...string) atlas.SymbolCall {
	t.Helper()
	offset := strings.Index(apiSource, mark)
	if offset < 0 {
		t.Fatalf("no %q in the source", mark)
	}
	line := strings.Count(apiSource[:offset], "\n") + 1
	column := offset - strings.LastIndex(apiSource[:offset], "\n")
	return atlas.SymbolCall{Kind: string(programindex.RelationInvokesExternal), Name: header + "." + name, Line: line, Column: column, Values: values,
		API: &atlas.CallAPI{Package: header, Name: name}}
}

// apiGraph is one file with main making the given calls.
func apiGraph(calls ...atlas.SymbolCall) []atlas.Place {
	return []atlas.Place{
		{ID: "f1", Kind: atlas.PlaceFile, Path: "main.c", TargetIDs: []string{"t1"}, File: &atlas.FileFacts{}},
		{ID: "s1", Kind: atlas.PlaceSymbol, Path: "main.c", LineNo: 1, Parent: "f1", TargetIDs: []string{"t1"},
			Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: "main", Kind: "function", ObjectID: "n1"}, Calls: calls}},
	}
}

// apiReader reads the outside symbols of places through the categorizer,
// remembering its answers under cache.
func apiReader(t *testing.T, cache string, places []atlas.Place, categorizer llm.Categorizer) *reader {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, atlas.TablesDir), 0o700); err != nil {
		t.Fatal(err)
	}
	r := &reader{
		opts: Options{
			Graph: atlas.Graph{Version: atlas.GraphVersion, Places: places}, Repository: "x", Revision: "abc",
			Executor: llm.Executor{RootDir: cache, Enabled: true, BatchConcurrency: 1, BatchController: &llm.BatchController{}},
			Provider: &tableProvider{}, Categorizer: categorizer, OwnerRunDir: dir,
			ReadSource: func(string) ([]byte, error) { return []byte(apiSource), nil },
			Stage:      func(string, ...string) {}, State: func(string, string, ...string) {},
		},
		classifierGate: &llm.BatchController{}, places: map[string]atlas.Place{}, api: map[string]apiRole{},
		uses: map[string]*atlas.StageUse{}, started: map[string]time.Time{},
		knowledge: map[string]*Knowledge{}, knowledgeRecords: map[knowledgeRecordKey]*Knowledge{}, knowledgeSubjects: map[string]*Knowledge{},
		responseTables: map[string]rememberedTable{}, classifierResponses: map[string]rememberedClassifier{}, shared: &readerShared{},
		boundaries: map[string]*boundaryState{}, boundaryIDs: map[string]string{},
	}
	for _, place := range places {
		r.places[place.ID] = place
	}
	return r
}

// asking records every api question the categorizer is asked, by symbol,
// and answers a symbol's questions from verdicts (none when a symbol has
// no verdict). A call asked what its words become is recorded in calls as
// "symbol talks=answer" and answered none.
type asking struct {
	mu       sync.Mutex
	usage    map[string]string
	asked    []string
	calls    []string
	verdicts map[string]llm.Verdict
}

func (a *asking) categorizer() *typesafetest.Categorizer {
	return &typesafetest.Categorizer{Decide: func(key string, question llm.Question) (llm.Verdict, bool) {
		symbol, _ := question.Item["symbol"].(string)
		column := key[strings.LastIndex(key, "|")+1:]
		a.mu.Lock()
		defer a.mu.Unlock()
		a.asked = append(a.asked, column+" "+symbol)
		if column == "enters" {
			talks, _ := question.Item["talks"].(string)
			a.calls = append(a.calls, symbol+" talks="+talks)
			return typesafetest.Choose(lines.APINone), true
		}
		if a.usage == nil {
			a.usage = map[string]string{}
		}
		a.usage[symbol], _ = question.Item["usage"].(string)
		if verdict, ok := a.verdicts[symbol]; ok {
			return verdict, true
		}
		return typesafetest.Choose(lines.APINone), true
	}}
}

// An outside symbol is shown by its call as the code wrote it: not the
// whole line, not a comment beside it and not its line number.
func TestAnOutsideSymbolsUsageIsItsCallNotItsLine(t *testing.T) {
	answers := &asking{}
	places := apiGraph(apiCall(t, "string.h", "strcmp", "strcmp", "b"), apiCall(t, "sys/socket.h", "accept", "accept"))
	r := apiReader(t, t.TempDir(), places, answers.categorizer())
	if err := r.readAPI(t.Context()); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"string.h.strcmp": `strcmp(a,"b")`, "sys/socket.h.accept": "accept(s, &sa, &len)"}
	for symbol, usage := range want {
		if answers.usage[symbol] != usage {
			t.Fatalf("%s was shown as %q, want %q", symbol, answers.usage[symbol], usage)
		}
	}
}

// Each outside symbol's answer, and each word call's, is remembered on its
// own, an uncertain one among them: a warm reading of the same symbols
// asks the categorizer nothing, and the symbol it could not decide stays
// undecided rather than being drawn again.
func TestAWarmAtlasAPIRereadMakesNoLiveCall(t *testing.T) {
	cache := t.TempDir()
	uncertain := make(map[string]float64)
	for _, option := range lines.API(false, true).Columns[0].Options {
		uncertain[option] = 0
	}
	uncertain[lines.APINone], uncertain[atlas.BoundaryClientRequest] = 0.52, 0.48
	answers := &asking{verdicts: map[string]llm.Verdict{
		"sys/socket.h.accept": typesafetest.Choose(lines.APIServes),
		"string.h.strcmp":     {Choice: lines.APINone, Probabilities: uncertain},
	}}
	places := apiGraph(apiCall(t, "string.h", "strcmp", "strcmp", "b"), apiCall(t, "sys/socket.h", "accept", "accept"), apiCall(t, "stdio.h", "printf", "printf", "%s\n"))
	categorizer := answers.categorizer()
	first := apiReader(t, cache, places, categorizer)
	if err := first.readAPI(t.Context()); err != nil {
		t.Fatal(err)
	}
	calls := categorizer.Calls()
	if calls == 0 || !first.api["sys/socket.h.accept"].publishes || first.api["string.h.strcmp"] != (apiRole{}) {
		t.Fatalf("first reading: %d calls, roles %+v", calls, first.api)
	}
	warm := apiReader(t, cache, places, categorizer)
	if err := warm.readAPI(t.Context()); err != nil {
		t.Fatal(err)
	}
	if categorizer.Calls() != calls {
		t.Fatalf("a warm reading asked the categorizer %d more times", categorizer.Calls()-calls)
	}
	if fmt.Sprint(warm.apiRoles()) != fmt.Sprint(first.apiRoles()) {
		t.Fatalf("the warm reading changed the roles: %+v, then %+v", first.apiRoles(), warm.apiRoles())
	}
	// Three symbols and the two word calls beside no talking: accept serves.
	if use := warm.uses[lines.StageAPI]; use == nil || use.Reused != 5 || use.Live != 0 {
		t.Fatalf("the warm reading did not reuse all three symbols and two calls: %+v", use)
	}
}

// A new call of a symbol not asked before asks that symbol and that call
// alone: every other symbol's and call's answer is remembered by its own
// evidence, whatever row it takes in the next request.
func TestAtlasAPIAsksOnlyTheSymbolANewCallAdds(t *testing.T) {
	cache := t.TempDir()
	answers := &asking{}
	before := apiGraph(apiCall(t, "string.h", "strcmp", "strcmp", "b"), apiCall(t, "sys/socket.h", "accept", "accept"))
	if err := apiReader(t, cache, before, answers.categorizer()).readAPI(t.Context()); err != nil {
		t.Fatal(err)
	}
	answers.asked = nil
	// printf sorts between the two: every row after it moves.
	after := apiGraph(apiCall(t, "string.h", "strcmp", "strcmp", "b"), apiCall(t, "sys/socket.h", "accept", "accept"), apiCall(t, "stdio.h", "printf", "printf", "%s\n"))
	if err := apiReader(t, cache, after, answers.categorizer()).readAPI(t.Context()); err != nil {
		t.Fatal(err)
	}
	slices.Sort(answers.asked)
	if !slices.Equal(answers.asked, []string{"enters stdio.h.printf", "talks stdio.h.printf"}) {
		t.Fatalf("a new call asked %v, want only its symbol", answers.asked)
	}
}

// A call's answer is remembered by what the call shows, never by its line:
// the same calls moved down by an edit above them ask nothing again.
func TestAWordCallMovedByAnUnrelatedEditIsNotAskedAgain(t *testing.T) {
	cache := t.TempDir()
	answers := &asking{}
	calls := []atlas.SymbolCall{apiCall(t, "string.h", "strcmp", "strcmp", "b"), apiCall(t, "stdio.h", "printf", "printf", "%s\n")}
	if err := apiReader(t, cache, apiGraph(calls...), answers.categorizer()).readAPI(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(answers.calls) != 2 {
		t.Fatalf("the word calls asked = %v, want both", answers.calls)
	}
	answers.asked = nil
	for i := range calls {
		calls[i].Line++
	}
	moved := apiReader(t, cache, apiGraph(calls...), answers.categorizer())
	moved.opts.ReadSource = func(string) ([]byte, error) { return []byte("\n" + apiSource), nil }
	if err := moved.readAPI(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(answers.asked) != 0 || len(moved.callEnters) != 2 {
		t.Fatalf("moved calls asked %v, answered %v", answers.asked, moved.callEnters)
	}
}

// A reading may stop after the outside symbols (`read --through api`), so
// their questions can be read again on a saved reading input alone.
func TestAReadingCanStopAfterTheOutsideSymbols(t *testing.T) {
	stage, err := StageName("api")
	if err != nil || stage != lines.StageAPI {
		t.Fatalf("StageName(api) = %q, %v", stage, err)
	}
	if err := validateControls(Options{Through: stage}); err != nil {
		t.Fatal(err)
	}
}
