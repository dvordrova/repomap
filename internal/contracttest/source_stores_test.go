package contracttest

import (
	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/programindex/goadapter"
	"testing"
)

func TestGoDestinationsSeparateReadAndReturnStoreContexts(t *testing.T) {
	t.Setenv("CGO_ENABLED", "0")
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOWORK", "off")
	root, repository := materializeFixtureRepository(t, "go")
	writePublishedGoFixtureModule(t, root)
	authorities := analyzeGoFixture(t, root, repository, goFixtureAppPackage, "destination-store-review")
	index, err := goadapter.Build(repository, authorities.target, authorities.origins, authorities.direct, authorities.external, authorities.core, authorities.dynamic, authorities.tests)
	if err != nil {
		t.Fatal(err)
	}
	graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: index}}})
	if err != nil {
		t.Fatal(err)
	}
	reader := reading.NewDestinationReader(graph.Places)
	found := map[string]bool{}
	for _, place := range graph.Places {
		if place.Symbol == nil {
			continue
		}
		name := place.Symbol.Decl.Name
		if name != "DestinationStoresAfterCall" && name != "DestinationSingleStoreAfterCall" && name != "DestinationReturnedStore" {
			continue
		}
		for _, call := range place.Symbol.Calls {
			if call.API == nil || call.API.Package != "net/http" || call.API.Name != "Get" {
				continue
			}
			found[name] = true
			uses := reader.Read(place, call)
			if name == "DestinationReturnedStore" {
				if len(uses) != 1 || uses[0].Address != "https://after-only.example" {
					t.Fatalf("write before return lost its distinct return context: %+v", uses)
				}
			} else if len(uses) != 1 || uses[0].Address != "" || uses[0].Frontier == "" {
				t.Fatalf("later/multiple stores became this call's destination: %+v", uses)
			}
		}
	}
	if len(found) != 3 {
		t.Fatal("fixture HTTP call missing")
	}
}
