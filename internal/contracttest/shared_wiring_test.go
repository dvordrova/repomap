package contracttest

import (
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/programindex/goadapter"
)

// Shared code calling through an interface field calls what the linking
// program stores there: storefixture's hookedRun.fire runs cmd/worker's
// workerHook.Run inside the worker alone. The edge is the worker's wiring
// (Edge.Targets), never a static use, so the app, which links storefixture
// too, gains no use of the worker (etcd's server had "used" raftexample's
// Raft node and etcdutl's lessor). C keeps a function pointer stored under
// a branch a witness, which makes no edge; Python, JS/TS and Clojure
// programs of the fixtures share no such field.
func TestSharedCodesInterfaceCallIsTheLinkingProgramsWiring(t *testing.T) {
	t.Setenv("CGO_ENABLED", "0")
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOWORK", "off")
	root, repository := materializeFixtureRepository(t, "go")
	var inputs []places.TargetInput
	ids := map[string]string{}
	for _, pkg := range []string{goFixtureAppPackage, goFixtureRootPackage + "/cmd/worker"} {
		authorities := sharedGoFixtureAuthorities(t, root, repository, pkg, "shared wiring")
		index, err := goadapter.Build(repository, authorities.target, authorities.origins, authorities.direct, authorities.external, authorities.core, authorities.dynamic, authorities.tests)
		if err != nil {
			t.Fatal(err)
		}
		ids[pkg] = index.Target.ID
		inputs = append(inputs, places.TargetInput{Index: index, Root: "."})
	}
	graph, err := places.Build(places.Input{Repository: repository, Targets: inputs})
	if err != nil {
		t.Fatal(err)
	}
	var found *atlas.Edge
	for i, edge := range graph.Edges {
		if edge.From == atlas.FileID("internal/storefixture/command_table.go") && edge.To == atlas.FileID("cmd/worker/main.go") && edge.Kind == "calls" {
			found = &graph.Edges[i]
		}
	}
	if found == nil {
		t.Fatal("no call from the shared hook runner into the worker's hook")
	}
	if found.Static || !slices.Equal(found.Targets, []string{ids[goFixtureRootPackage+"/cmd/worker"]}) {
		t.Fatalf("the shared hook's call: static=%v targets=%v, want the worker's wiring alone", found.Static, found.Targets)
	}
}
