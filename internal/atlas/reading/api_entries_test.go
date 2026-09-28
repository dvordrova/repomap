package reading

import (
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/typesafe/typesafetest"
)

// entriesReader reads the boundaries of main's calls in main.go and of one
// call in main_test.go, dry, with the given roles, each word call outside
// tests that no other fact names answered as enters says for "symbol:line",
// else for its symbol ("" is asked and undecided).
func entriesReader(t *testing.T, roles map[string]apiRole, enters map[string]string, facts []atlas.Place, calls ...atlas.SymbolCall) *reader {
	t.Helper()
	places := []atlas.Place{
		{ID: "f1", Kind: atlas.PlaceFile, Path: "main.go", TargetIDs: []string{"t1"}, File: &atlas.FileFacts{}},
		{ID: "f2", Kind: atlas.PlaceFile, Path: "main_test.go", TargetIDs: []string{"t1"}, File: &atlas.FileFacts{Test: true}},
		{ID: "s1", Kind: atlas.PlaceSymbol, Path: "main.go", LineNo: 1, Parent: "f1", TargetIDs: []string{"t1"},
			Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: "main", Kind: "function", ObjectID: "n1"}, Calls: calls}},
		{ID: "s2", Kind: atlas.PlaceSymbol, Path: "main_test.go", LineNo: 1, Parent: "f2", TargetIDs: []string{"t1"},
			Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: "TestMain", Kind: "function", ObjectID: "n2"}, Calls: []atlas.SymbolCall{wordCall("flag.Bool", 3, 2, "from-a-test")}}},
	}
	places = append(places, facts...)
	r := answerTestReader(t, nil, nil)
	r.dry, r.opts.Through = true, ""
	r.opts.Graph.Places = places
	r.places = map[string]atlas.Place{}
	for _, place := range places {
		r.places[place.ID] = place
	}
	r.api = roles
	r.callEnters, r.entering = map[sourceSite]string{}, map[string]bool{}
	claims := map[sourceSite][]atlas.Place{}
	for _, fact := range facts {
		claims[sourceSite{path: fact.Path, line: fact.LineNo}] = append(claims[sourceSite{path: fact.Path, line: fact.LineNo}], fact)
	}
	for _, place := range places {
		if place.Symbol == nil || r.testFile(place.Parent) {
			continue
		}
		for _, call := range place.Symbol.Calls {
			answer, asked := enters[fmt.Sprintf("%s:%d", call.Name, call.Line)]
			if !asked {
				answer, asked = enters[call.Name]
			}
			if asked && len(call.Values) > 0 && !claimedByOtherFact(claims[sourceSite{path: place.Path, line: call.Line}], call.Name, call.Column) {
				r.callEnters[sourceSite{place.Path, call.Line, call.Column}] = answer
				r.entering[call.Name] = r.entering[call.Name] || answer != "" && answer != lines.APINone
			}
		}
	}
	// As Read counts them: boundaries the reading makes are numbered after
	// the graph's.
	r.nextBoundary = len(facts)
	if err := r.readBoundaries(t.Context()); err != nil {
		t.Fatal(err)
	}
	return r
}

// wordCall is a call of an outside Go symbol at line:column given words.
func wordCall(symbol string, line, column int, values ...string) atlas.SymbolCall {
	dot := strings.LastIndex(symbol, ".")
	return atlas.SymbolCall{Kind: string(programindex.RelationInvokesExternal), Name: symbol, Line: line, Column: column, Values: values,
		API: &atlas.CallAPI{Package: symbol[:dot], Name: symbol[dot+1:]}}
}

// made lists the boundaries a reading made: direction, kind, name, words
// and whether its handler is established.
func made(r *reader) []string {
	var result []string
	for _, id := range sortedKeys(r.boundaries) {
		state := r.boundaries[id]
		b := state.place.Boundary
		handled := "handled"
		if state.handlerUnknown {
			handled = "handler unknown"
		}
		result = append(result, strings.Join([]string{state.place.Path, b.Direction, state.kind, state.name, strings.Join(b.Words, "|"), handled}, " "))
	}
	sort.Strings(result)
	return result
}

// A call outside tests whose words its answer makes an entry is that
// entry: its handler is not established, its words are the ones the call
// was given (never its call word or its caller's name), and it is named by
// them as written when nothing chose among them. A single word needs no
// choosing. Words none of which can name an entry (a format ending in a
// line break) make no entry, and that is recorded; a test's call makes
// none, and neither does a call a fact already names. A word given to a
// registration the code found makes the same entry. Each call is its own:
// the call of the same symbol answered none makes nothing.
func TestAWordGivenCallBecomesAnEntryWhoseHandlerIsNotEstablished(t *testing.T) {
	query := atlas.Place{ID: "b1", Kind: atlas.PlaceBoundary, Path: "main.go", LineNo: 9, Column: 12, Parent: "f1", TargetIDs: []string{"t1"}, Given: "users",
		Boundary: &atlas.BoundaryFacts{Source: "fact", Origins: []atlas.BoundaryOrigin{{TargetID: "t1", FactID: "a1"}}, ObjectID: "n1", Caller: "main", Values: []string{"users", "SELECT 1"}, Direction: atlas.DirectionOut, GivenKind: atlas.BoundaryDB}}
	option := atlas.Place{ID: "b2", Kind: atlas.PlaceBoundary, Path: "main.go", LineNo: 11, Column: 3, Parent: "f1", TargetIDs: []string{"t1"}, Given: "vendor/cli.Command.option",
		Boundary: &atlas.BoundaryFacts{Source: "fact", Origins: []atlas.BoundaryOrigin{{TargetID: "t1", FactID: "a2"}}, ObjectID: "n1", Caller: "main", External: "vendor/cli.Command.option",
			Values: []string{"-p, --port <n>"}, Words: []string{"option", "-p, --port <n>"}, Holder: "main.go:10:9", Direction: atlas.DirectionOut}}
	// An address among a call's words leaves the registration only the
	// address as its value; the entry keeps every word the call wrote.
	socket := atlas.Place{ID: "b3", Kind: atlas.PlaceBoundary, Path: "main.go", LineNo: 13, Column: 18, Parent: "f1", TargetIDs: []string{"t1"}, Given: "flag.String",
		Boundary: &atlas.BoundaryFacts{Source: "fact", Origins: []atlas.BoundaryOrigin{{TargetID: "t1", FactID: "a3"}}, ObjectID: "n1", Caller: "main", External: "flag.String",
			Values: []string{"/var/run/x.sock"}, Words: []string{"String", "socket", "/var/run/x.sock", "control socket path"}, Direction: atlas.DirectionOut}}
	r := entriesReader(t, nil, map[string]string{
		"flag.Bool": atlas.BoundaryCommand, "fmt.Printf": atlas.BoundaryCommand,
		"database/sql.DB.QueryRow": atlas.BoundaryCommand, "vendor/cli.Command.option": atlas.BoundaryCommand,
		"flag.String": atlas.BoundaryCommand, "strings.HasPrefix": lines.APINone,
	}, []atlas.Place{query, option, socket},
		wordCall("flag.Bool", 3, 18, "verbose", "log more"),
		wordCall("flag.Bool", 4, 16, "quiet"),
		wordCall("flag.Bool", 5, 16),
		wordCall("fmt.Printf", 7, 2, "%s\n"),
		wordCall("database/sql.DB.QueryRow", 9, 12, "SELECT 1"),
		wordCall("vendor/cli.Command.option", 11, 3, "-p, --port <n>"),
		wordCall("flag.String", 13, 18, "socket", "/var/run/x.sock", "control socket path"),
		wordCall("strings.HasPrefix", 15, 5, "s3://"),
	)
	want := []string{
		"main.go in command -p, --port <n> -p, --port <n> handler unknown",
		"main.go in command quiet quiet handler unknown",
		"main.go in command socket /var/run/x.sock control socket path socket|/var/run/x.sock|control socket path handler unknown",
		"main.go in command verbose log more verbose|log more handler unknown",
		"main.go out db   handled",
	}
	if got := made(r); !reflect.DeepEqual(got, want) {
		t.Fatalf("boundaries = %q\nwant %q", got, want)
	}
	unnamed := 0
	for _, row := range r.rejected {
		if row.Kind == "entry_unnamed" && strings.Contains(row.Reason, "main.go:7") {
			unnamed++
		}
	}
	if unnamed != 1 {
		t.Fatalf("the unnameable words were not recorded: %+v", r.rejected)
	}
	// The entry keeps its caller as where it is declared, and its handler
	// is marked as not established.
	for _, state := range r.boundaries {
		if state.handlerUnknown && (state.place.Boundary.ObjectID != "n1" || state.place.Boundary.CallerDoc != "") {
			t.Fatalf("an entry whose handler is not established lost its declaring caller or carries its documentation: %+v", state.place.Boundary)
		}
	}
}

// Words a call passes to another program are that program's, and a
// listener stays the listening side: a word call whose symbol talks to
// other programs or serves is never asked what its words become, so a
// launch stays a launch (exec.CommandContext(ctx, "litestream", "restore",
// …) is not a command of this program). Beside none, or no decided talks
// answer, the call is asked, and the talks answer it was given travels
// with it.
func TestAWordCallBesideWhatItsSymbolDoesWithOtherProgramsIsNotAsked(t *testing.T) {
	calls := []atlas.SymbolCall{
		wordCall("os/exec.CommandContext", 3, 9, "litestream", "restore"),
		wordCall("net.Listen", 4, 9, ":8080"),
		wordCall("flag.Bool", 5, 9, "verbose"),
		wordCall("strings.HasPrefix", 6, 9, "-"),
	}
	places := apiGraph(calls...)
	answers := &asking{verdicts: map[string]llm.Verdict{
		"os/exec.CommandContext": typesafetest.Choose(atlas.BoundaryRunsProgram),
		"net.Listen":             typesafetest.Choose(lines.APIServes),
		"strings.HasPrefix":      {Choice: lines.APINone, Probabilities: map[string]float64{lines.APINone: 0.52, atlas.BoundarySDK: 0.48}},
	}}
	r := apiReader(t, t.TempDir(), places, answers.categorizer())
	if err := r.readAPI(t.Context()); err != nil {
		t.Fatal(err)
	}
	slices.Sort(answers.calls)
	if want := []string{"flag.Bool talks=none", "strings.HasPrefix talks="}; !slices.Equal(answers.calls, want) {
		t.Fatalf("the calls asked what their words become = %v, want %v", answers.calls, want)
	}
}

// Each call is decided on its own, and the launch walk's evidence reads
// those answers: strcmp's "-h" is an option of this program and its
// "monitor" is not, a call asked and not decided is unsure, and a call
// giving no word to a symbol whose words are an entry at another call is
// unsure too. The idiom counts every call of the symbol it recorded.
func TestTheLaunchEvidenceReadsEachCallsOwnAnswer(t *testing.T) {
	r := entriesReader(t, nil, map[string]string{
		"string.h.strcmp:3": atlas.BoundaryCommand, "string.h.strcmp:4": lines.APINone, "string.h.strcmp:5": "",
	}, nil,
		wordCall("string.h.strcmp", 3, 5, "-h"),
		wordCall("string.h.strcmp", 4, 5, "monitor"),
		wordCall("string.h.strcmp", 5, 5, "quit"),
		wordCall("string.h.strcmp", 6, 5),
	)
	if got, want := made(r), []string{"main.go in command -h -h handler unknown"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("boundaries = %q, want %q", got, want)
	}
	unsure, idioms := r.launchEvidence("t1")
	if got := fmt.Sprint(unsure); got != "[{n1 main.go 5 5 string.h.strcmp undecided} {n1 main.go 6 5 string.h.strcmp no_words}]" {
		t.Fatalf("unsure = %s", got)
	}
	if got := fmt.Sprint(idioms); got != "[{string.h.strcmp command 1 4 [n1]}]" {
		t.Fatalf("idioms = %s", got)
	}
}
