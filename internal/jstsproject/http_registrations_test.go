package jstsproject

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/gitfiles"
	"github.com/dvordrova/repomap/internal/programindex/adaptertest"
)

func TestCumulativeJSTSHTTPConstructorPathsAndEmptyCallbacks(t *testing.T) {
	root := preparedCompilerProject(t)
	const source = "src/http-registrations.ts"
	tracked := []string{"package.json", "tsconfig.json", source}
	for _, path := range tracked {
		contents, err := os.ReadFile(filepath.Join("..", "..", "testdata", "repositories", "jsts", filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, root, path, string(contents))
	}
	materializeCumulativeJSTSDependencyTypes(t, root)
	repository, err := corpus.New(t.Context(), root, gitfiles.Listing{Paths: tracked, RegularPaths: tracked})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	_, index, _, err := Build(t.Context(), repository, root)
	if err != nil {
		t.Fatal(err)
	}
	result, err := facts.Build(facts.Input{Targets: []facts.TargetInput{{Index: index, Root: "."}}})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"/health": true, "/returned": true, "/v1/update": true, "/v1/metrics": true}
	// installRouteTree: the leaf's app is the express() passed on through two
	// parameters; the branch hands its own app to itself, a cycle with no
	// holder.
	holders := map[string]int{"/tree/leaf": 56, "/tree/branch": 0, "/tree/branch/nested": 0}
	adaptertest.AssertParameterHolders(t, result, source, holders)
	var emptyID string
	for _, fact := range result.OfKind(facts.KindRegistration) {
		if _, tree := holders[fact.Path]; fact.Anchor == nil || fact.Anchor.Path != source || tree {
			continue
		}
		if !want[fact.Path] || fact.Method != "GET" || fact.Anchor.Column <= 0 {
			t.Fatalf("TS unknown/local path became a route or duplicated it: %+v", fact)
		}
		delete(want, fact.Path)
		if fact.Path == "/returned" {
			// TypeScript currently exposes this as an unresolved callable
			// result. The factory itself must never become the HTTP handler.
			if fact.ObjectID != "" {
				t.Fatalf("TS unresolved function return acquired a handler: %+v", fact)
			}
		} else {
			if !strings.HasSuffix(fact.Symbol, "emptyHealthHandler") || fact.ObjectID == "" {
				t.Fatalf("TS empty callback lost native declaration: %+v", fact)
			}
			emptyID = fact.ObjectID
		}
		if strings.HasPrefix(fact.Path, "/v1/") && len(fact.Evidence) < 3 {
			t.Fatalf("TS wrapper lost constructor/registration source chain: %+v", fact)
		}
	}
	if len(want) != 0 {
		t.Fatalf("TS routes omitted: %v", want)
	}
	graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: index}}, Facts: result})
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.AssertMethodArgumentPositions(t, graph, source, "passMethodArguments", "receiveMethodArguments")
	for _, place := range graph.Places {
		// Places scope native objects by target: t*.n*.
		if place.Symbol != nil && place.Symbol.Decl.ObjectID == index.Target.ID+"."+emptyID {
			return
		}
	}
	t.Fatal("empty TypeScript handler disappeared from places")
}
