package groupindex

import (
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
)

// at is a site of the test's file, as reachGraphTest's inputs are written.
func at(line int) programindex.Location {
	return programindex.Location{Path: "x.c", Line: line, Column: 3}
}

// optionNames are an input's options by name, and nestedNames the inputs
// listed under another input with no tile of their own.
func (g *reachGraphTest) optionNames(name string) []string {
	return g.operationNames(g.reachOf(name).Options)
}

func (g *reachGraphTest) nestedNames() []string {
	var result []string
	for _, operation := range g.index.Operations {
		if g.index.Launch.Nested[operation.ID] {
			result = append(result, operation.Name)
		}
	}
	return result
}

// A flag a helper declares on the parser it is handed is an option of each
// subcommand whose own call made a parser a call hands it, and no tile of
// its own only when every call of the helper hands one: a call handing the
// program's own parser, or a parser the code does not follow, keeps it.
func TestAFlagDeclaredOnAHandedParserIsAnOptionOfTheSubcommandMakingIt(t *testing.T) {
	build := func(third *programindex.Location) *reachGraphTest {
		g := newReachGraphTest()
		g.functions("main", "build", "addCommon")
		g.call("main", "build")
		g.call("build", "addCommon")
		g.seed("main")
		g.declared("-v", "command", "build", 5, 0)
		g.declared("init", "command", "build", 10, 0)
		g.declared("status", "command", "build", 20, 0)
		g.declared("--quiet", "command", "addCommon", 30, 0)
		g.index.Handed = Handed{
			OnParameter: []ParameterCall{{SubjectID: g.ids["addCommon"], Location: at(30), Parameter: 1}},
			Calls: []HandedCall{
				{FromSubjectID: g.ids["build"], ToSubjectID: g.ids["addCommon"], Location: at(11), Exact: true, Made: map[int]programindex.Location{1: at(10)}},
				{FromSubjectID: g.ids["build"], ToSubjectID: g.ids["addCommon"], Location: at(21), Exact: true, Made: map[int]programindex.Location{1: at(20)}},
			},
		}
		if third != nil {
			made := map[int]programindex.Location{}
			if third.Line > 0 {
				made[1] = *third
			}
			g.index.Handed.Calls = append(g.index.Handed.Calls, HandedCall{FromSubjectID: g.ids["build"], ToSubjectID: g.ids["addCommon"], Location: at(40), Exact: true, Made: made})
		}
		g.derive()
		return g
	}
	g := build(nil)
	if !slices.Equal(g.optionNames("init"), []string{"--quiet"}) || !slices.Equal(g.optionNames("status"), []string{"--quiet"}) || !slices.Equal(g.nestedNames(), []string{"--quiet"}) {
		t.Fatalf("init %v, status %v, nested %v", g.optionNames("init"), g.optionNames("status"), g.nestedNames())
	}
	// The program's own parser (made at line 1, no input's call) and a
	// parameter's parser (nothing made) keep --quiet a tile.
	for _, third := range []programindex.Location{at(1), {}} {
		g := build(&third)
		if !slices.Equal(g.optionNames("init"), []string{"--quiet"}) || !slices.Equal(g.optionNames("status"), []string{"--quiet"}) || len(g.nestedNames()) != 0 {
			t.Fatalf("third call at %v: init %v, status %v, nested %v", third, g.optionNames("init"), g.optionNames("status"), g.nestedNames())
		}
	}
	// A helper also reached otherwise than by an exact call keeps it too.
	g = newReachGraphTest()
	g.functions("build", "addCommon")
	g.declared("init", "command", "build", 10, 0)
	g.declared("--quiet", "command", "addCommon", 30, 0)
	g.index.Handed = Handed{
		OnParameter: []ParameterCall{{SubjectID: g.ids["addCommon"], Location: at(30), Parameter: 1}},
		Calls:       []HandedCall{{FromSubjectID: g.ids["build"], ToSubjectID: g.ids["addCommon"], Location: at(11), Exact: true, Made: map[int]programindex.Location{1: at(10)}}},
		Unfollowed:  map[string]bool{g.ids["addCommon"]: true},
	}
	g.derive()
	if !slices.Equal(g.optionNames("init"), []string{"--quiet"}) || len(g.nestedNames()) != 0 {
		t.Fatalf("init %v, nested %v", g.optionNames("init"), g.nestedNames())
	}
}

// A row of a table a helper looks up with the keys it is handed, while it
// declares on the parser it is handed, is an option of each subcommand
// whose parser a call handing that row's key hands it (freqtrade's
// _build_args(optionlist=ARGS_TRADE, parser=trade_cmd)). A row is no tile
// of its own only when every call of the helper is followed and each call
// looking it up hands a subcommand's parser, and the helper alone reads
// the table: a list the code wrote no rows for (a spread) keeps every row,
// and the program's parser keeps the rows its keys name.
func TestARowLookedUpWithHandedKeysIsAnOptionOfTheSubcommandMakingTheParser(t *testing.T) {
	build := func(extra *HandedCall, otherReader bool) *reachGraphTest {
		g := newReachGraphTest()
		g.functions("main", "build", "buildArgs", "listFlags")
		g.declare(programindex.ObjectVariable, "OPTIONS")
		g.call("main", "build")
		g.call("build", "buildArgs")
		g.relate(programindex.RelationReads, programindex.ResolutionExact, "buildArgs", "OPTIONS")
		if otherReader {
			g.relate(programindex.RelationReads, programindex.ResolutionExact, "listFlags", "OPTIONS")
		}
		g.seed("main")
		g.declared("trade", "command", "build", 10, 0)
		g.declared("backtest", "command", "build", 20, 0)
		g.declared("--verbose", "command", "OPTIONS", 50, 0)
		g.declared("--fee", "command", "OPTIONS", 51, 0)
		g.declared("--db-url", "command", "OPTIONS", 52, 0)
		table := g.ids["OPTIONS"]
		g.index.Handed = Handed{
			OnParameter: []ParameterCall{{SubjectID: g.ids["buildArgs"], Location: at(60), Parameter: 2}},
			Calls: []HandedCall{
				{FromSubjectID: g.ids["build"], ToSubjectID: g.ids["buildArgs"], Location: at(11), Exact: true, Made: map[int]programindex.Location{2: at(10)}, Keys: map[string][]string{table: {"db_url", "fee"}}},
				{FromSubjectID: g.ids["build"], ToSubjectID: g.ids["buildArgs"], Location: at(21), Exact: true, Made: map[int]programindex.Location{2: at(20)}, Keys: map[string][]string{table: {"fee"}}},
			},
			Rows: []TableRowKey{{TableID: table, Key: "verbosity", Location: at(50)}, {TableID: table, Key: "fee", Location: at(51)}, {TableID: table, Key: "db_url", Location: at(52)}},
		}
		if extra != nil {
			call := *extra
			call.FromSubjectID, call.ToSubjectID = g.ids["build"], g.ids["buildArgs"]
			if call.Keys != nil {
				call.Keys = map[string][]string{table: call.Keys["OPTIONS"]}
			}
			g.index.Handed.Calls = append(g.index.Handed.Calls, call)
		}
		g.derive()
		return g
	}
	g := build(nil, false)
	if !slices.Equal(g.optionNames("trade"), []string{"--fee", "--db-url"}) || !slices.Equal(g.optionNames("backtest"), []string{"--fee"}) || !slices.Equal(g.nestedNames(), []string{"--fee", "--db-url"}) {
		t.Fatalf("trade %v, backtest %v, nested %v", g.optionNames("trade"), g.optionNames("backtest"), g.nestedNames())
	}
	// The program's own parser, looked up with verbosity: --verbose stays a
	// tile and is no subcommand's; the others are still only theirs.
	g = build(&HandedCall{Location: at(31), Exact: true, Made: map[int]programindex.Location{2: at(1)}, Keys: map[string][]string{"OPTIONS": {"verbosity"}}}, false)
	if len(g.optionNames("trade")) != 2 || !slices.Equal(g.nestedNames(), []string{"--fee", "--db-url"}) {
		t.Fatalf("trade %v, nested %v", g.optionNames("trade"), g.nestedNames())
	}
	// A list with no rows (a spread of another) and no keys read at all
	// each keep every row a tile; so does another reader of the table.
	for name, g := range map[string]*reachGraphTest{
		"spread":       build(&HandedCall{Location: at(31), Exact: true, Made: map[int]programindex.Location{2: at(20)}, Keys: map[string][]string{"OPTIONS": nil}}, false),
		"no keys read": build(&HandedCall{Location: at(31), Exact: true, Made: map[int]programindex.Location{2: at(20)}}, false),
		"other reader": build(nil, true),
	} {
		if !slices.Equal(g.optionNames("trade"), []string{"--fee", "--db-url"}) || len(g.nestedNames()) != 0 {
			t.Fatalf("%s: trade %v, nested %v", name, g.optionNames("trade"), g.nestedNames())
		}
	}
}
