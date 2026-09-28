package jstsproject

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/atlas/reading/partstest"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/gitfiles"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/groupindex/flowtest"
	"github.com/dvordrova/repomap/internal/programindex"
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
	// The graph carries the fact layer, as an ordinary run builds it: its
	// registrations are boundary places.
	layer, err := facts.Build(facts.Input{Repository: repository, Targets: []facts.TargetInput{{Index: index, Dependencies: &catalog, Root: "."}}})
	if err != nil {
		t.Fatal(err)
	}
	graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: index, Dependencies: &catalog}}, Facts: layer})
	if err != nil {
		t.Fatal(err)
	}
	// The compiler's tokens hold code lines; the JSDoc, the comment inside
	// and the blank line do not. An overload signature is its one line.
	adaptertest.AssertDeclarationCodeLines(t, graph, "src/type-members.ts", map[string][]int{
		"pick": {1, 1, 3}, "firstOf": {3},
	})
	// The graph records what a declaration reads: recordOrder reads the
	// module's handledOrderIds, and takes an OrderEvent, the interface its
	// parameter is typed with.
	adaptertest.AssertDeclarationUses(t, graph,
		adaptertest.DeclarationUse{FromPath: "src/server.ts", From: "recordOrder", Kind: "takes", ToPath: "src/server.ts", To: "OrderEvent"},
		adaptertest.DeclarationUse{FromPath: "src/server.ts", From: "recordOrder", Kind: "reads", ToPath: "src/server.ts", To: "handledOrderIds"},
	)
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
	// A handler's assignment shows the words of the route that hands it
	// over.
	if registered := split.Registered["src/server.ts"]; !slices.Contains(registered, "get /products/featured") {
		t.Fatalf("server.ts's handlers are asked with registrations %v", registered)
	}
	// The helper question: processPendingJobs, not exported and called by
	// runWorker, and handledOrderIds, not exported and read by recordOrder,
	// are helpers; handledOrderIds goes with its one reader by code.
	// shared/contracts.ts's paintColor, which destinations.ts only re-exports
	// and no file of this program reads, is no helper by code and not asked.
	for _, name := range []string{"processPendingJobs", "handledOrderIds"} {
		if !split.Helpers[[2]string{"src/server.ts", name}] {
			t.Fatalf("%s is no helper: %v", name, split.HelperItems[[2]string{"src/server.ts", name}])
		}
	}
	if got := split.HelperItems[[2]string{"src/server.ts", "handledOrderIds"}]["read_by"]; !reflect.DeepEqual(got, []any{"src/server.ts:recordOrder"}) {
		t.Fatalf("handledOrderIds is asked with read_by %v", got)
	}
	ids, record := split.Symbols[[2]string{"src/server.ts", "handledOrderIds"}], split.Symbols[[2]string{"src/server.ts", "recordOrder"}]
	if split.PartOf[ids] == "" || split.PartOf[ids] != split.PartOf[record] {
		t.Fatalf("handledOrderIds in %q, recordOrder in %q", split.PartOf[ids], split.PartOf[record])
	}
	if item, asked := split.HelperItems[[2]string{"shared/contracts.ts", "paintColor"}]; asked {
		t.Fatalf("paintColor, which nothing reads, was asked: %v", item)
	}
	// GroupsIndex derives the reach on these facts. The fixture has no
	// input of its own: as inputs, recordOrder reaches handledOrderIds by
	// reading it and runWorker reaches processPendingJobs by calling it.
	// Every relation resolved as several alternatives is a dispatch site
	// that dispatches no input. The fixture has none today: a call through
	// a property is never resolved from its stores (JSTS).
	indexes, err := groupindex.ProjectAtlas(map[string]programindex.Index{index.Target.ID: index}, split.Atlas)
	if err != nil {
		t.Fatal(err)
	}
	projected := indexes[0]
	flowtest.Check(t, index, projected)
	if got := flowtest.Reached(projected, flowtest.Probe(t, projected, "src/server.ts", "recordOrder")); got["handledOrderIds"] != "read" {
		t.Fatalf("recordOrder reaches %v", got)
	}
	if got := flowtest.Reached(projected, flowtest.Probe(t, projected, "src/server.ts", "runWorker")); got["processPendingJobs"] != "call" {
		t.Fatalf("runWorker reaches %v", got)
	}
	alternatives := map[string]int{}
	for _, relation := range index.Relations {
		if relation.Resolution == programindex.ResolutionAlternatives && len(relation.ToIDs) > 1 {
			alternatives[relation.ID] = len(relation.ToIDs)
		}
	}
	for _, site := range projected.Dispatch {
		if alternatives[site.RelationID] != len(site.Alternatives) || len(site.OperationIDs) != 0 {
			t.Fatalf("dispatch site %+v", site)
		}
		delete(alternatives, site.RelationID)
	}
	if len(alternatives) != 0 {
		t.Fatalf("relations of several alternatives that are no dispatch site: %v", alternatives)
	}
}
