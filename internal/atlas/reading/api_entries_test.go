package reading

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/programindex"
)

// entriesReader reads the boundaries of main's calls in main.go and of one
// call in main_test.go, dry, with the given roles.
func entriesReader(t *testing.T, roles map[string]apiRole, facts []atlas.Place, calls ...atlas.SymbolCall) *reader {
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

// A call outside tests that gives words to a symbol whose words are an
// entry is that entry: its handler is not established, its words are the ones
// the call was given (never its call word or its caller's name), and it is
// named by them as written when nothing chose among them. A single word
// needs no choosing. Words none of which can name an entry (a format
// ending in a line break) make no entry, and that is recorded; a test's
// call makes none, and neither does a call a fact already names. A word
// given to a registration the code found makes the same entry.
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
	r := entriesReader(t, map[string]apiRole{
		"flag.Bool": {enters: atlas.BoundaryCommand}, "fmt.Printf": {enters: atlas.BoundaryCommand},
		"database/sql.DB.QueryRow": {enters: atlas.BoundaryCommand}, "vendor/cli.Command.option": {enters: atlas.BoundaryCommand},
		"flag.String": {enters: atlas.BoundaryCommand},
	}, []atlas.Place{query, option, socket},
		wordCall("flag.Bool", 3, 18, "verbose", "log more"),
		wordCall("flag.Bool", 4, 16, "quiet"),
		wordCall("flag.Bool", 5, 16),
		wordCall("fmt.Printf", 7, 2, "%s\n"),
		wordCall("database/sql.DB.QueryRow", 9, 12, "SELECT 1"),
		wordCall("flag.String", 13, 18, "socket", "/var/run/x.sock", "control socket path"),
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

// Words a call passes to another program are that program's: an entry
// kind answered beside anything a call does with other programs but none
// is refused, alone, and the talks answer stands. A listener stays the
// listening side, a launch stays a launch (exec.CommandContext(ctx,
// "litestream", "restore", …) is not a command of this program), and a
// database call stays a database call. Beside none the entry stands.
func TestAnEntryBesideWhatACallDoesWithOtherProgramsIsRefused(t *testing.T) {
	for _, talks := range []string{lines.APIServes, atlas.BoundaryDB, atlas.BoundaryRunsProgram} {
		stands := answerTestReader(t, nil, nil)
		role := apiRoleOf(stands.talksStands("sym1", "os/exec.CommandContext", map[string]string{"talks": talks, "enters": atlas.BoundaryCommand}))
		if role.enters != "" || len(stands.rejected) != 1 || stands.rejected[0].Kind != "cell_rejected" {
			t.Fatalf("an entry beside %s: role %+v, rejected %+v", talks, role, stands.rejected)
		}
		if talks == lines.APIServes && !role.publishes || talks != lines.APIServes && role.talks != talks {
			t.Fatalf("the %s answer did not stand: %+v", talks, role)
		}
	}
	kept := answerTestReader(t, nil, nil)
	if role := apiRoleOf(kept.talksStands("sym1", "flag.Bool", map[string]string{"talks": lines.APINone, "enters": atlas.BoundaryCommand})); role.enters != atlas.BoundaryCommand || len(kept.rejected) != 0 {
		t.Fatalf("an entry beside none: role %+v, rejected %+v", role, kept.rejected)
	}
}
