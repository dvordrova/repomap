package jstsproject

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/atlas/reading/partstest"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gitfiles"
	"github.com/dvordrova/repomap/internal/programindex/adaptertest"
)

// The TypeScript parts request carries code structure only, and every
// declaration takes one part or an entry off the map. SimulationField's
// nested animate function is its lexical child: it is not listed and takes
// SimulationField's part. A class method goes with its class; TypeScript has
// no method declared outside its class body to check.
func TestCumulativeJSTSMapOfParts(t *testing.T) {
	root := preparedCompilerProject(t)
	tracked := []string{"package.json", "tsconfig.json", "shared/contracts.ts", "src/platform.ts", "src/destinations.ts", "src/server.ts", "src/type-members.ts"}
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
	_, index, catalog, err := Build(t.Context(), repository, root)
	if err != nil {
		t.Fatal(err)
	}
	graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: index, Dependencies: &catalog}}})
	if err != nil {
		t.Fatal(err)
	}
	// The compiler's tokens hold code lines; the JSDoc, the comment inside
	// and the blank line do not. An overload signature is its one line.
	adaptertest.AssertDeclarationCodeLines(t, graph, "src/type-members.ts", map[string][]int{
		"pick": {1, 1, 3}, "firstOf": {3},
	})
	checked := partstest.Check(t, graph, reading.TargetMeta{ID: index.Target.ID, Language: "typescript", Kind: "application", Name: index.Target.Name, Root: "."}, root)
	parent := checked.Symbols[[2]string{"src/platform.ts", "SimulationField"}]
	var child string
	for key, id := range checked.Symbols {
		if key[0] == "src/platform.ts" && key[1] != "SimulationField" && (key[1] == "animate" || key[1] == "SimulationField.animate") {
			child = id
		}
	}
	if parent == "" || child == "" || checked.PartOf[child] != checked.PartOf[parent] {
		t.Fatalf("the nested animate function left SimulationField's part: %q %q", child, parent)
	}
	drawer := checked.Symbols[[2]string{"src/platform.ts", "LevelDrawer"}]
	draw := checked.Symbols[[2]string{"src/platform.ts", "LevelDrawer.draw"}]
	if drawer == "" || draw == "" || checked.PartOf[draw] != checked.PartOf[drawer] {
		t.Fatalf("a class method left its class: %q %q", draw, drawer)
	}
	// Split, the method still follows its class, and every file's module
	// body is a row of the assignment.
	split := partstest.CheckSplit(t, graph, reading.TargetMeta{ID: index.Target.ID, Language: "typescript", Kind: "application", Name: index.Target.Name, Root: "."}, root)
	drawer, draw = split.Symbols[[2]string{"src/platform.ts", "LevelDrawer"}], split.Symbols[[2]string{"src/platform.ts", "LevelDrawer.draw"}]
	if !split.Split["src/platform.ts"] || split.PartOf[draw] != split.PartOf[drawer] {
		t.Fatalf("split: a class method in %q, its class in %q", split.PartOf[draw], split.PartOf[drawer])
	}
}
