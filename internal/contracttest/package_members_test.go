package contracttest

import (
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/pythontarget"
)

// A call into another target's package is the seam between the two
// targets: levels.py imports fixture_client.rest.RestClient, which only the
// client project's index declares (client/fixture_client/rest.py), and
// calls it, so the two files are joined by a call and an import and each
// stays its own target's. freqtrade's scripts/rest_client.py calling
// freqtrade_client.ft_client.main had drawn no arrow at all. The Go and
// JS/TS adapters index another module's or workspace package's source
// themselves (GO, JSTS); Clojure's and C's fixtures import no other
// target's code (CLOJURE, C).
func TestACallIntoAnotherTargetsPackageJoinsTheirFiles(t *testing.T) {
	_, repository := materializeFixtureRepository(t, "python")
	catalog, err := pythontarget.Discover(t.Context(), repository)
	if err != nil {
		t.Fatal(err)
	}
	var client pythontarget.Target
	for _, candidate := range catalog.Entries {
		if candidate.ProjectDir == "client" && candidate.Kind == pythontarget.KindLibrary {
			client = candidate
		}
	}
	if client.Selector == "" {
		t.Fatalf("the Python fixture has no client library target: %#v", catalog.Entries)
	}
	input, err := sharedPythonFixtureInput(t, repository, client)
	if err != nil {
		t.Fatal(err)
	}
	app := pythonLibraryIndex(t)
	input.Target.ID = "t2"
	clientIndex, err := programindex.New(input)
	if err != nil {
		t.Fatal(err)
	}
	if app.Target.ID == clientIndex.Target.ID {
		t.Fatalf("both targets are %s", app.Target.ID)
	}
	graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: app, Root: "."}, {Index: clientIndex, Root: "client"}}})
	if err != nil {
		t.Fatal(err)
	}
	from, to := atlas.FileID("src/fixture_app/levels.py"), atlas.FileID("client/fixture_client/rest.py")
	kinds := map[string]bool{}
	for _, edge := range graph.Edges {
		if edge.From == from && edge.To == to {
			kinds[edge.Kind] = true
		}
	}
	if !kinds["calls"] || !kinds["imports"] {
		t.Fatalf("levels.py -> client/fixture_client/rest.py edges %v, want calls and imports", kinds)
	}
	for _, place := range graph.Places {
		if place.ID == to && (len(place.TargetIDs) != 1 || place.TargetIDs[0] != clientIndex.Target.ID) {
			t.Fatalf("the client's file is held by %v, want the client target alone", place.TargetIDs)
		}
	}
}
