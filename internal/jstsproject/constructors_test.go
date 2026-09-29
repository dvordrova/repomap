package jstsproject

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gitfiles"
	"github.com/dvordrova/repomap/internal/programindex"
)

// src/workers.ts is Python's workers.py in TypeScript: `new Worker(...)` is
// a construct call of Worker's constructor, `new QuietWorker(...)` of the
// BaseWorker constructor it inherits, and the calls on the results are the
// methods of the names' declared types, the inherited run included. A class
// with no constructor anywhere gives the compiler no declaration to call:
// `new Plain()` stays unresolved, where Python calls the class.
func TestCumulativeJSTSConstructorCallsReachTheirConstructor(t *testing.T) {
	root := preparedCompilerProject(t)
	tracked := []string{"package.json", "tsconfig.json", "src/workers.ts"}
	for _, path := range tracked {
		contents, err := os.ReadFile(filepath.Join("..", "..", "testdata", "repositories", "jsts", filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, root, path, string(contents))
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
	objects := map[string]programindex.Object{}
	for _, object := range index.Objects {
		objects[object.ID] = object
	}
	name := func(id string) string { return objects[id].Name } // a method's name is Owner.method
	source, err := os.ReadFile(filepath.Join("..", "..", "testdata", "repositories", "jsts", "src", "workers.ts"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(source), "\n")
	got := map[string][]string{}
	for _, relation := range index.Relations {
		caller := objects[relation.FromID].Name
		if relation.Kind != programindex.RelationCalls || relation.Location == nil || !strings.HasPrefix(caller, "start") && caller != "makePlain" {
			continue
		}
		row := string(relation.Resolution)
		for _, id := range relation.ToIDs {
			row += " " + name(id)
		}
		if relation.Invocation != "" {
			row += " (" + relation.Invocation + ")"
		}
		line := strings.TrimSpace(lines[relation.Location.Line-1])
		got[line] = append(got[line], row)
		sort.Strings(got[line])
	}
	want := map[string][]string{
		"worker = new Worker(name, 2)":        {"exact Worker.constructor (construct)"},
		"worker.run()":                        {"exact BaseWorker.run"},
		"if (worker) worker.step()":           {"exact Worker.step"},
		"const quiet = new QuietWorker(name)": {"exact BaseWorker.constructor (construct)"},
		"return quiet.run()":                  {"exact BaseWorker.run"},
		"return new Plain()":                  {"unresolved (construct)"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("calls in workers.ts:\n have %q\n want %q", got, want)
	}
}
