package groupindex

import (
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/programindex"
)

// An entry the model chose no word for is named by its handler, never by
// the function making the registration: othello's `:draw draw/draw-state`
// hand-over, written in start! with its handler in another file, had read
// othello.ui.sketch/start! (the boundary's Caller is the enclosing function
// when the handler lives elsewhere).
func TestAnUnnamedEntryIsNamedByItsHandlerNotItsRegistrar(t *testing.T) {
	p := atlasTestProgram(t, "desktop", "src/sketch.clj", "src/draw.cljc")
	handler := p.Objects[1]
	target := atlas.Target{ID: p.Target.ID, Name: p.Target.Name, Language: "clojure", Kind: "executable", Root: "src", Zones: []atlas.Zone{}, Arrows: []atlas.Arrow{},
		Boxes: []atlas.Box{{ID: "ui", Dir: "src", Title: "UI", Line: "Draws.", Side: atlas.SideIn, MemberIDs: []string{p.Objects[0].ID, handler.ID},
			Files: []atlas.File{{Path: "src/sketch.clj", Line: "Starts.", Source: atlas.SourceModel, Symbols: []atlas.Symbol{}}}}}}
	target.Boundaries = []atlas.Boundary{{ID: "b11", ObjectID: handler.ID, BoxID: "ui", Path: "src/sketch.clj", LineNo: 42, Column: 11, Caller: "FA",
		Direction: atlas.DirectionIn, Kind: atlas.BoundaryExtension, Values: []string{"draw"}, Line: "Draws the board.", FactID: "a123"}}
	indexes, err := ProjectAtlas(map[string]programindex.Index{p.Target.ID: p}, atlas.Atlas{Version: atlas.Version, Repository: "test", Targets: []atlas.Target{target}, Joints: []atlas.Joint{}, Diagnostics: []atlas.Diagnostic{}})
	if err != nil {
		t.Fatal(err)
	}
	if operations := indexes[0].Operations; len(operations) != 1 || operations[0].Name != handler.Name || operations[0].SubjectID != handler.ID {
		t.Fatalf("operations = %+v, want one named by its handler %s", operations, handler.Name)
	}
}

// A relation between parts names a callable written inline by the function
// holding it, never by its number and never by the function it wraps:
// litestream's canvas cards had read "FindSQLiteDatabases$1 calls
// IsSQLiteDatabase", and the wrapping name would read "IsSQLiteDatabase
// calls IsSQLiteDatabase".
func TestAConnectionNamesAnInlineCallableByItsHolder(t *testing.T) {
	at := func(path string, line int) *programindex.Location { return &programindex.Location{Path: path, Line: line, Column: 2} }
	program := programindex.Index{Objects: []programindex.Object{
		{ID: "find", Name: "FindSQLiteDatabases", Kind: programindex.ObjectFunction, Location: at("main.go", 1010), EndLine: 1040},
		{ID: "walk", Name: "FindSQLiteDatabases$1", Kind: programindex.ObjectFunction, Location: at("main.go", 1020), EndLine: 1030},
		{ID: "check", Name: "IsSQLiteDatabase", Kind: programindex.ObjectFunction, Location: at("main.go", 1035), EndLine: 1050},
	}, Relations: []programindex.Relation{{Kind: programindex.RelationCalls, Resolution: programindex.ResolutionExact, FromID: "walk", ToIDs: []string{"check"}}}}
	if got := inlineHolders(program); len(got) != 1 || got["walk"] != "FindSQLiteDatabases (inline)" {
		t.Fatalf("holder names = %v", got)
	}
	if got := inlineNames(program); got["walk"] != "IsSQLiteDatabase" {
		t.Fatalf("a reader still names a wrapper by what it wraps: %v", got)
	}
}

// An input's key is the first word its registration wrote beyond its
// name's, with no space: a table row's key (freqtrade's version_main
// beside -V --version), never its help text.
func TestAnInputsKeyIsItsFirstWordBeyondItsName(t *testing.T) {
	for _, c := range []struct {
		name   string
		values []string
		want   string
	}{
		{"-V --version", []string{"version_main", "-V", "--version", "show program's version number and exit"}, "version_main"},
		{"--erase", []string{"--erase", "Clean all existing data"}, ""},
		{"socket", []string{"socket", "/var/run/litestream.sock", "control socket path"}, "/var/run/litestream.sock"},
	} {
		if got := entryKey(c.name, c.values); got != c.want {
			t.Fatalf("key of %q from %q = %q, want %q", c.name, c.values, got, c.want)
		}
	}
}
