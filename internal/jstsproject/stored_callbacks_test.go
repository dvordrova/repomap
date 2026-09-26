package jstsproject

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gitfiles"
	"github.com/dvordrova/repomap/internal/programindex"
)

// The TypeScript equivalent of a C callback stored in one of two
// function-pointer fields under a branch (src/stored-callbacks.ts). The calls
// through the properties gain no handler, and each handler keeps its
// registration at its own call. A table of handlers keeps no binding and a
// call through a local constant names the constant; JSTS.md records both.
func TestCumulativeJSTSStoredCallbacksStayUnresolved(t *testing.T) {
	root := preparedCompilerProject(t)
	const path = "src/stored-callbacks.ts"
	tracked := []string{"package.json", "tsconfig.json", path}
	for _, file := range tracked {
		contents, err := os.ReadFile(filepath.Join("..", "..", "testdata", "repositories", "jsts", filepath.FromSlash(file)))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, root, file, string(contents))
	}
	repository, err := corpus.New(t.Context(), root, gitfiles.Listing{Paths: tracked, RegularPaths: tracked})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	_, index, _, err := Build(t.Context(), repository, root)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]string{}
	for _, object := range index.Objects {
		names[object.ID] = object.Name
	}
	type want struct {
		kind programindex.RelationKind
		from string
		to   string
	}
	lines := map[int]want{
		17: {programindex.RelationCalls, "EventLoop.fire", ""},
		18: {programindex.RelationCalls, "EventLoop.fire", ""},
		27: {programindex.RelationPassesCallback, "runEventLoop", "acceptClient"},
		28: {programindex.RelationPassesCallback, "runEventLoop", "flushReplies"},
	}
	seen := map[int]bool{}
	for _, relation := range index.Relations {
		if relation.Location == nil || relation.Location.Path != path {
			continue
		}
		from := names[relation.FromID]
		if from == "EventLoop.fire" && relation.Kind == programindex.RelationCalls && len(relation.ToIDs) != 0 {
			t.Fatalf("a call through a stored property borrowed a registered handler: %+v", relation)
		}
		expected, ok := lines[relation.Location.Line]
		if !ok || expected.kind != relation.Kind {
			continue
		}
		if seen[relation.Location.Line] || from != expected.from {
			t.Fatalf("line %d: unexpected %s from %s", relation.Location.Line, relation.Kind, from)
		}
		seen[relation.Location.Line] = true
		if expected.to == "" {
			if relation.Resolution != programindex.ResolutionUnresolved || len(relation.ToIDs) != 0 {
				t.Fatalf("call through a property stored under a branch was resolved: %+v", relation)
			}
			continue
		}
		if relation.Resolution != programindex.ResolutionExact || len(relation.ToIDs) != 1 ||
			names[relation.ToIDs[0]] != expected.to || relation.SourceArgumentID == "" {
			t.Fatalf("registration of %s lost its exact handler or argument: %+v", expected.to, relation)
		}
	}
	if len(seen) != len(lines) {
		t.Fatalf("stored callback lines seen %v, want %d", seen, len(lines))
	}
}
