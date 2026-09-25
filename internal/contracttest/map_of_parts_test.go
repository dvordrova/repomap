package contracttest

import (
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/atlas/reading/partstest"
	"github.com/dvordrova/repomap/internal/clojureproject"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programindex/goadapter"
	"github.com/dvordrova/repomap/internal/pythonprogramindex"
	"github.com/dvordrova/repomap/internal/pythontarget"
)

// The parts request of each fixture carries code structure only and every
// declaration takes one part or an entry off the map. A Go method declared
// in another file than its type goes with its type: internal/localstore's
// Ledger.Append. Its own file, which declares nothing else, is no row of the
// parts request yet stays on the map in Ledger's part: the card does not
// list it as off the map. Python and TypeScript have no method outside its
// class; Clojure's defmethod, extend-type and extend-protocol are not
// declarations the Clojure adapter projects, so no equivalent exists there
// to check.
func TestCumulativeGoMapOfParts(t *testing.T) {
	t.Setenv("CGO_ENABLED", "0")
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOWORK", "off")
	root, repository := materializeFixtureRepository(t, "go")
	library := analyzeGoFixture(t, root, repository, goFixtureRootPackage, "cumulative-go-map-of-parts")
	index, err := goadapter.Build(repository, library.target, library.origins, library.direct, library.external, library.core, library.dynamic, library.tests)
	if err != nil {
		t.Fatal(err)
	}
	graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: index, Root: "."}}})
	if err != nil {
		t.Fatal(err)
	}
	checked := partstest.Check(t, graph, reading.TargetMeta{ID: index.Target.ID, Language: "go", Kind: "library", Name: index.Target.Name, Root: "."}, root)
	ledger := checked.Symbols[[2]string{"internal/localstore/ledger.go", "Ledger"}]
	appendMethod := checked.Symbols[[2]string{"internal/localstore/ledger_append.go", "Ledger.Append"}]
	if ledger == "" || appendMethod == "" {
		t.Fatalf("the cross-file method fixture is missing: %v %v", ledger, appendMethod)
	}
	if checked.PartOf[appendMethod] == "" || checked.PartOf[appendMethod] != checked.PartOf[ledger] {
		t.Fatalf("Ledger.Append is in %q, Ledger in %q", checked.PartOf[appendMethod], checked.PartOf[ledger])
	}
	for _, entry := range checked.Target.OffMap {
		if entry.File.Path == "internal/localstore/ledger_append.go" {
			t.Fatalf("the file of a method that follows its type is off the map: %+v", entry)
		}
	}
	// The component card lists GroupsIndex's off-map files row for row.
	indexes, err := groupindex.ProjectAtlas(map[string]programindex.Index{index.Target.ID: index}, checked.Atlas)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range indexes[0].OffMap {
		if file.Path == "internal/localstore/ledger_append.go" {
			t.Fatalf("the card lists ledger_append.go off the map: %+v", file)
		}
	}
	// The test declaration of a type declared in the library, root_test.go's
	// testRootReader.expected, follows that type in the same way.
	reader := checked.Symbols[[2]string{"root.go", "testRootReader"}]
	expected := checked.Symbols[[2]string{"root_test.go", "testRootReader.expected"}]
	if reader == "" || expected == "" || checked.PartOf[expected] != checked.PartOf[reader] {
		t.Fatalf("testRootReader.expected left its type: %q %q", expected, reader)
	}
}

func TestCumulativePythonMapOfParts(t *testing.T) {
	root, repository := materializeFixtureRepository(t, "python")
	catalog, err := pythontarget.Discover(t.Context(), repository)
	if err != nil {
		t.Fatal(err)
	}
	var target pythontarget.Target
	for _, candidate := range catalog.Entries {
		if candidate.Kind == pythontarget.KindLibrary && candidate.ProjectDir == "." {
			target = candidate
			break
		}
	}
	input, err := pythonprogramindex.BuildInput(t.Context(), repository, target)
	if err != nil {
		t.Fatal(err)
	}
	index, err := programindex.New(input)
	if err != nil {
		t.Fatal(err)
	}
	graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: index, Root: "."}}})
	if err != nil {
		t.Fatal(err)
	}
	// A module declaring __all__ exports exactly what it lists: the parts
	// request shows the signature of render_level, not of format_score.
	seen := 0
	for _, object := range index.Objects {
		if object.Location == nil || object.Location.Path != "src/fixture_app/exports.py" {
			continue
		}
		want := map[string]programindex.Visibility{"render_level": programindex.VisibilityPublic, "format_score": programindex.VisibilityInternal}[object.Name]
		if want == "" {
			continue
		}
		seen++
		if object.Visibility != want {
			t.Fatalf("%s visibility %s, want %s", object.Name, object.Visibility, want)
		}
	}
	if seen != 2 {
		t.Fatalf("exports.py declarations seen: %d", seen)
	}
	checked := partstest.Check(t, graph, reading.TargetMeta{ID: index.Target.ID, Language: "python", Kind: "library", Name: index.Target.Name, Root: "."}, root)
	// A function nested in another takes its parent's part.
	outer := checked.Symbols[[2]string{"src/fixture_app/http_registrations.py", "register_route"}]
	nested := checked.Symbols[[2]string{"src/fixture_app/http_registrations.py", "empty_registered_handler"}]
	if nested == "" || outer == "" || checked.PartOf[nested] != checked.PartOf[outer] {
		t.Fatalf("a nested function left its parent's part: %q %q", nested, outer)
	}
}

func TestCumulativeClojureMapOfParts(t *testing.T) {
	root, repository := materializeFixtureRepository(t, "clojure")
	targets, err := clojureproject.Scout(repository, "clojure")
	if err != nil || len(targets) != 1 {
		t.Fatalf("Clojure discovery: %v %v", targets, err)
	}
	result, err := clojureproject.Build(t.Context(), root, repository, targets[0])
	if err != nil {
		t.Fatal(err)
	}
	index, err := programindex.New(result.Input)
	if err != nil {
		t.Fatal(err)
	}
	graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: index}}})
	if err != nil {
		t.Fatal(err)
	}
	partstest.Check(t, graph, reading.TargetMeta{ID: index.Target.ID, Language: "clojure", Kind: "executable", Name: index.Target.Name, Root: "."}, root)
}
