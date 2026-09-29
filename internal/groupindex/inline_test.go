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
	want := map[string]string{
		"metrics": "ReplicateCommand.Run (inline)",
		"handler": "DatabasesTool (inline)",
		"wrapper": "listDatabases",
		"both":    "ReplicateCommand.Run (inline)",
		"lambda":  "DatabasesTool (inline)",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("inline names = %v\nwant %v", got, want)
	}
}
