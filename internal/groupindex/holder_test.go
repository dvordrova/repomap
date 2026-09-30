package groupindex

import (
	"reflect"
	"sort"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/programindex"
)

// A word entry whose own call made the object subcommands are declared on
// is no input when the call is given its word only under a parameter's
// name (argparse's add_subparsers(dest="command"), freqtrade's 34
// subcommands, with their handlers or without): it names where the chosen
// one is kept. A command group given its word as its own, by position,
// keeps it: the word a person types before its subcommands (commander's
// program.command("remote"), whose add and rm carry handlers).
func TestADestOfHandledSubcommandsIsNoInputButACommandGroupKeepsItsWord(t *testing.T) {
	at := func(line, column int) *programindex.Location {
		return &programindex.Location{Path: "cli/main.py", Line: line, Column: column}
	}
	call := func(ref string, line int, argument programindex.PatternArgumentInput) programindex.RelationInput {
		return programindex.RelationInput{SourceRef: ref, Kind: programindex.RelationCalls, FromRef: "oa", ToRefs: []string{"ob"},
			Resolution: programindex.ResolutionExact, TargetsObserved: 1, Location: at(line, 5),
			Witnesses: []programindex.Witness{{Kind: "syntax", Location: at(line, 5)}}, WitnessesObserved: 1,
			PatternsObserved: 1, Patterns: []programindex.RelationPatternInput{{SourceRef: ref + "-pattern", Form: programindex.PatternCall, Selector: "command",
				Location: at(line, 10), ArgumentsObserved: 1, Arguments: []programindex.PatternArgumentInput{argument}}}}
	}
	p := atlasTestProgramWith(t, "cli", []programindex.RelationInput{
		call("group", 5, programindex.PatternArgumentInput{Position: 1, Kind: programindex.PatternLiteralString, Value: "remote"}),
		call("dest", 10, programindex.PatternArgumentInput{Keyword: "dest", Kind: programindex.PatternLiteralString, Value: "command"}),
		call("unhandled", 20, programindex.PatternArgumentInput{Keyword: "dest", Kind: programindex.PatternLiteralString, Value: "cmd"}),
	}, "cli/main.py", "cli/handlers.py")
	caller, handler := p.Objects[0].ID, p.Objects[1].ID
	target := atlas.Target{ID: p.Target.ID, Name: p.Target.Name, Language: "python", Kind: "executable", Root: "cli", Zones: []atlas.Zone{}, Arrows: []atlas.Arrow{},
		Boxes: []atlas.Box{{ID: "cli", Dir: "cli", Title: "CLI", Line: "Reads the command line.", Side: atlas.SideIn, MemberIDs: []string{caller, handler},
			Files: []atlas.File{{Path: "cli/main.py", Line: "Builds the parser.", Source: atlas.SourceModel, Symbols: []atlas.Symbol{}}}}}}
	on := func(line int) *atlas.DeclaredOn {
		return &atlas.DeclaredOn{Path: "cli/main.py", LineNo: line, Column: 10}
	}
	word := func(id string, line int, name string, declaredOn *atlas.DeclaredOn) atlas.Boundary {
		return atlas.Boundary{ID: id, ObjectID: caller, BoxID: "cli", Path: "cli/main.py", LineNo: line, Column: 10, Caller: "FA", DeclaredOn: declaredOn,
			Direction: atlas.DirectionIn, Kind: atlas.BoundaryCommand, Values: []string{name}, Name: name, Source: "model", HandlerUnknown: true}
	}
	handOver := func(id string, line int) atlas.Boundary {
		return atlas.Boundary{ID: id, ObjectID: handler, BoxID: "cli", Path: "cli/main.py", LineNo: line, Column: 20, Caller: "FA", DeclaredOn: on(line),
			Direction: atlas.DirectionIn, Kind: atlas.BoundaryCommand, Values: []string{}, Source: "model"}
	}
	target.Boundaries = []atlas.Boundary{
		word("b1", 5, "remote", nil), word("b2", 6, "add", on(5)), handOver("b3", 6), word("b4", 7, "rm", on(5)), handOver("b5", 7),
		word("b6", 10, "command", nil), word("b7", 11, "trade", on(10)), handOver("b8", 11),
		word("b9", 20, "cmd", nil), word("b10", 21, "status", on(20)),
	}
	indexes, err := ProjectAtlas(map[string]programindex.Index{p.Target.ID: p}, atlas.Atlas{Version: atlas.Version, Repository: "test", Targets: []atlas.Target{target}, Joints: []atlas.Joint{}, Diagnostics: []atlas.Diagnostic{}})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, operation := range indexes[0].Operations {
		handled := "handled"
		if operation.HandlerUnknown {
			handled = "no handler"
		}
		got = append(got, operation.Name+" "+handled)
	}
	sort.Strings(got)
	if want := []string{"add handled", "remote no handler", "rm handled", "status no handler", "trade handled"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("inputs %q, want %q", got, want)
	}
}

// A handler registered again by a call given its words only under a
// parameter's name (a pattern what it is handed is matched against:
// freqtrade's CallbackQueryHandler(self._profit, pattern="update_profit$")
// beside CommandHandler("profit", self._profit)) is that handler's one
// input; a handler only such a call registers keeps its own.
func TestAHandlerRegisteredAgainByAPatternIsOneInput(t *testing.T) {
	at := func(line int) *programindex.Location {
		return &programindex.Location{Path: "bot/chat.py", Line: line, Column: 9}
	}
	call := func(ref string, line int, to string, argument programindex.PatternArgumentInput) programindex.RelationInput {
		return programindex.RelationInput{SourceRef: ref, Kind: programindex.RelationCalls, FromRef: "oa", ToRefs: []string{to},
			Resolution: programindex.ResolutionExact, TargetsObserved: 1, Location: at(line),
			Witnesses: []programindex.Witness{{Kind: "syntax", Location: at(line)}}, WitnessesObserved: 1,
			PatternsObserved: 1, Patterns: []programindex.RelationPatternInput{{SourceRef: ref + "-pattern", Form: programindex.PatternCall, Selector: "Handler",
				Location: at(line), ArgumentsObserved: 1, Arguments: []programindex.PatternArgumentInput{argument}}}}
	}
	p := atlasTestProgramWith(t, "bot", []programindex.RelationInput{
		call("command", 5, "ob", programindex.PatternArgumentInput{Position: 1, Kind: programindex.PatternLiteralString, Value: "profit"}),
		call("again", 6, "ob", programindex.PatternArgumentInput{Keyword: "pattern", Kind: programindex.PatternLiteralString, Value: "update_profit$"}),
		call("only", 7, "oc", programindex.PatternArgumentInput{Keyword: "pattern", Kind: programindex.PatternLiteralString, Value: `force_exit__\S+`}),
	}, "bot/chat.py", "bot/profit.py", "bot/exit.py")
	target := atlas.Target{ID: p.Target.ID, Name: p.Target.Name, Language: "python", Kind: "executable", Root: "bot", Zones: []atlas.Zone{}, Arrows: []atlas.Arrow{},
		Boxes: []atlas.Box{{ID: "bot", Dir: "bot", Title: "Bot", Line: "Answers chat.", Side: atlas.SideIn, MemberIDs: []string{p.Objects[0].ID, p.Objects[1].ID, p.Objects[2].ID},
			Files: []atlas.File{{Path: "bot/chat.py", Line: "Registers handlers.", Source: atlas.SourceModel, Symbols: []atlas.Symbol{}}}}}}
	registration := func(id string, line int, handler, word string) atlas.Boundary {
		return atlas.Boundary{ID: id, ObjectID: handler, BoxID: "bot", Path: "bot/chat.py", LineNo: line, Column: 9, Caller: "FA",
			Direction: atlas.DirectionIn, Kind: atlas.BoundaryRequest, Values: []string{word}, Name: word, Source: "model"}
	}
	target.Boundaries = []atlas.Boundary{
		registration("b1", 5, p.Objects[1].ID, "profit"), registration("b2", 6, p.Objects[1].ID, "update_profit$"), registration("b3", 7, p.Objects[2].ID, `force_exit__\S+`),
	}
	indexes, err := ProjectAtlas(map[string]programindex.Index{p.Target.ID: p}, atlas.Atlas{Version: atlas.Version, Repository: "test", Targets: []atlas.Target{target}, Joints: []atlas.Joint{}, Diagnostics: []atlas.Diagnostic{}})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, operation := range indexes[0].Operations {
		got = append(got, operation.Name)
	}
	sort.Strings(got)
	if want := []string{`force_exit__\S+`, "profit"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("inputs %q, want %q", got, want)
	}
}
