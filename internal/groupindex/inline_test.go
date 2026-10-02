package groupindex

import (
	"reflect"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
)

// A callable written inline is named as a reader names it, never by the
// number its compiler gives it: by the repository function it only wraps,
// else by the function whose lines hold it, "(inline)", a method with its
// type. A helper called beside outside calls, or beside another repository
// function, names nothing; a callable no function holds keeps its name, and
// so does every other declaration.
func TestInlineCallablesAreNamedByTheirCalleeOrTheirHome(t *testing.T) {
	at := func(line int) *programindex.Location {
		return &programindex.Location{Path: "cmd/app/main.go", Line: line, Column: 2}
	}
	object := func(id, name string, kind programindex.ObjectKind, line, end int) programindex.Object {
		return programindex.Object{ID: id, Name: name, Kind: kind, Location: at(line), EndLine: end}
	}
	calls := func(from string, to ...string) []programindex.Relation {
		var result []programindex.Relation
		for _, id := range to {
			result = append(result, programindex.Relation{Kind: programindex.RelationCalls, Resolution: programindex.ResolutionExact, FromID: from, ToIDs: []string{id}})
		}
		return result
	}
	run := object("run", "Run", programindex.ObjectMethod, 10, 40)
	run.OwnerID = "command"
	program := programindex.Index{Objects: []programindex.Object{
		{ID: "command", Name: "ReplicateCommand", Kind: programindex.ObjectType, Location: at(5), EndLine: 8},
		run,
		object("metrics", "Run$1", programindex.ObjectFunction, 20, 25),
		object("tool", "DatabasesTool", programindex.ObjectFunction, 50, 70),
		object("handler", "DatabasesTool$1", programindex.ObjectFunction, 55, 65),
		object("list", "listDatabases", programindex.ObjectFunction, 80, 90),
		object("both", "Run$2", programindex.ObjectFunction, 30, 35),
		object("lambda", "<lambda>", programindex.ObjectLambda, 60, 60),
		object("loose", "init$1", programindex.ObjectFunction, 100, 101),
		object("wrapper", "Tool$2", programindex.ObjectFunction, 66, 68),
		{ID: "serve", Name: "ListenAndServe", Kind: programindex.ObjectFunction, External: &programindex.ExternalSymbol{PackagePath: "net/http", Name: "ListenAndServe"}},
	}}
	program.Relations = append(program.Relations, calls("metrics", "serve")...)
	program.Relations = append(program.Relations, calls("handler", "list", "serve")...)
	program.Relations = append(program.Relations, calls("both", "list", "tool")...)
	program.Relations = append(program.Relations, calls("wrapper", "list")...)
	got := inlineNames(program)
	// Two a function holds alike are each one of two (the test below).
	want := map[string]string{
		"metrics": "ReplicateCommand.Run (inline, 2)",
		"handler": "DatabasesTool (inline, 2)",
		"wrapper": "listDatabases",
		"both":    "ReplicateCommand.Run (inline, 2)",
		"lambda":  "DatabasesTool (inline, 2)",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("inline names = %v\nwant %v", got, want)
	}
}

// Callables one function writes alike are told apart by the word each
// one's hand-over gives it (headscale's cmd/hi commands, Name = "doctor"),
// else each said as one of how many they are (casdoor's two goroutines of
// Start, whose hand-overs give no word; two handed over under one word):
// never by a number or an ordinal of one's own. One alone keeps its name.
func TestCallablesOneFunctionWritesAlikeAreToldApart(t *testing.T) {
	at := func(path string, line int) *programindex.Location {
		return &programindex.Location{Path: path, Line: line, Column: 2}
	}
	function := func(id, name, path string, line, end int) programindex.Object {
		return programindex.Object{ID: id, Name: name, Kind: programindex.ObjectFunction, Location: at(path, line), EndLine: end}
	}
	field := func(detail string) programindex.Witness {
		return programindex.Witness{Kind: "callable_receiver_field", Detail: detail}
	}
	handed := func(from, to string, witnesses ...programindex.Witness) programindex.Relation {
		return programindex.Relation{Kind: programindex.RelationPassesCallback, Resolution: programindex.ResolutionExact, FromID: from, ToIDs: []string{to}, Witnesses: witnesses}
	}
	program := programindex.Index{Objects: []programindex.Object{
		function("main", "main", "cmd/hi/main.go", 20, 80),
		function("doctor", "main$1", "cmd/hi/main.go", 28, 30),
		function("networks", "main$2", "cmd/hi/main.go", 46, 48),
		function("start", "Start", "service/proxy.go", 306, 372),
		function("http", "Start$1", "service/proxy.go", 321, 327),
		function("https", "Start$2", "service/proxy.go", 329, 370),
		function("run", "Run", "run.go", 1, 40),
		function("first", "Run$1", "run.go", 10, 12),
		function("second", "Run$2", "run.go", 20, 22),
		function("serve", "Serve", "serve.go", 1, 20),
		function("alone", "Serve$1", "serve.go", 5, 9),
	}, Relations: []programindex.Relation{
		handed("main", "doctor", field(`Name = "doctor"`), field(`Help = "Check system requirements"`)),
		handed("main", "networks", field(`Name = "networks"`), field(`Help = "Prune unused Docker networks"`)),
		handed("run", "first", field(`Name = "same"`), field(`Timeout = 10`)),
		handed("run", "second", field(`Name = "same"`), field(`Timeout = 20`)),
	}}
	want := map[string]string{
		"doctor": "main (inline for doctor)", "networks": "main (inline for networks)",
		"http": "Start (inline, 2)", "https": "Start (inline, 2)",
		"first": "Run (inline, 2)", "second": "Run (inline, 2)",
		"alone": "Serve (inline)",
	}
	for name, got := range map[string]map[string]string{"names": inlineNames(program), "holders": inlineHolders(program)} {
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s = %v, want %v", name, got, want)
		}
	}
}
