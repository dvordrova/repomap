package cubemap

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

func TestBuildEntrypointCandidatesCarriesAcceptedActivityRoots(t *testing.T) {
	index := surfacediscovery.DirectCallIndex{
		Nodes: []surfacediscovery.DirectCallNode{
			{ID: "ordinary", Package: "example.com/app", Symbol: surfacediscovery.Symbol{Name: "ordinary"}, Declaration: surfacediscovery.Location{Path: "ordinary.go", Line: 1}},
			{ID: "activity", Package: "example.com/app", Symbol: surfacediscovery.Symbol{Name: "handler"}, Declaration: surfacediscovery.Location{Path: "handler.go", Line: 3}},
		},
	}

	candidates := buildEntrypointCandidates(index, map[string]int{"activity": 2})
	if len(candidates) != 2 || candidates[0].node.ID != "activity" || candidates[0].activitySurfaces != 2 ||
		!reflect.DeepEqual(candidates[0].signals, []string{"accepted_activity_surface", "no_incoming_exact_edge"}) {
		t.Fatalf("activity entrypoint evidence = %#v", candidates)
	}
}

func TestBuildEntrypointCandidatesNeverTruncatesAuthority(t *testing.T) {
	index := surfacediscovery.DirectCallIndex{Nodes: make([]surfacediscovery.DirectCallNode, maxEntrypointCandidates+1)}
	for position := range index.Nodes {
		index.Nodes[position] = surfacediscovery.DirectCallNode{
			ID: fmt.Sprintf("node-%d", position), Package: "example.com/app",
			Symbol:      surfacediscovery.Symbol{Name: fmt.Sprintf("symbol%d", position)},
			Declaration: surfacediscovery.Location{Path: "app.go", Line: position + 1},
		}
	}
	if candidates := buildEntrypointCandidates(index, nil); len(candidates) != len(index.Nodes) {
		t.Fatalf("entrypoint candidates = %d, want complete %d", len(candidates), len(index.Nodes))
	}
}

func TestDependencyCatalogCoverageRejectsPartialAuthority(t *testing.T) {
	coverage := dependencyCatalogCoverage(dependencies.Coverage{
		State: dependencies.CoveragePartial, ImportsObserved: 7, ImportsRetained: 4,
		Omissions: []dependencies.Omission{
			{Reason: dependencies.OmissionDependencyLoadUnavailable},
			{Reason: dependencies.OmissionModuleAuthorityMissing},
			{Reason: dependencies.OmissionDependencyLoadUnavailable},
		},
	})
	if err := validateDependencyCatalogCoverage(coverage); err == nil ||
		!strings.Contains(err.Error(), "not complete") {
		t.Fatalf("partial dependency coverage error = %v", err)
	}
}

func TestValidateCoverageRejectsPartialSemanticCatalog(t *testing.T) {
	coverage := Coverage{
		DependencyCatalog: DependencyCatalogCoverage{State: dependencies.CoverageComplete},
		Entrypoints:       CandidateCoverage{Observed: 2, Advertised: 1, Omitted: 1, ModelCalled: true},
	}
	if err := validateCoverage(coverage); err == nil ||
		!strings.Contains(err.Error(), "incomplete semantic candidate coverage") {
		t.Fatalf("partial semantic coverage error = %v", err)
	}
}

func TestDiscoverExternalUsagesJoinsExactCallsWithoutRereadingSource(t *testing.T) {
	node := surfacediscovery.DirectCallNode{
		ID: "caller-1", Package: "example.com/app/client", ModuleID: "module-1", ScenarioID: "scenario-1",
		Symbol:      surfacediscovery.Symbol{ID: "symbol-1", Package: "example.com/app/client", Name: "Send"},
		Declaration: surfacediscovery.Location{Path: "client/send.go", Line: 10, Column: 1},
		Body: surfacediscovery.DirectCallBodyRange{
			Start: surfacediscovery.Location{Path: "client/send.go", Line: 10, Column: 1},
			End:   surfacediscovery.Location{Path: "client/send.go", Line: 30, Column: 1},
		},
	}
	family := surfacediscovery.ExternalCallFamily{
		ID: "family-1", CallerID: node.ID,
		Target:       surfacediscovery.ExternalCallTarget{PackagePath: "net/http", Receiver: "*Client", Name: "Do"},
		Dispatch:     surfacediscovery.ExternalCallStatic,
		Invocation:   surfacediscovery.DirectCallSynchronous,
		WitnessCount: 2,
		Callsites: []surfacediscovery.Location{
			{Path: "client/send.go", Line: 20, Column: 4},
			{Path: "client/send.go", Line: 24, Column: 4},
		},
	}
	dependency := dependencyCandidate{
		ref: "d1",
		value: dependencies.Dependency{
			ID: "dependency-1", Kind: dependencies.KindStdlib, Name: "http", PackagePath: "net/http",
		},
	}

	got, matched, err := discoverExternalUsages(
		surfacediscovery.DirectCallIndex{State: surfacediscovery.DirectCallIndexReady, Nodes: []surfacediscovery.DirectCallNode{node}},
		surfacediscovery.ExternalCallIndex{
			Callers: []surfacediscovery.DirectCallNode{node}, Families: []surfacediscovery.ExternalCallFamily{family},
		},
		[]dependencyCandidate{dependency},
	)
	if err != nil {
		t.Fatal(err)
	}
	if matched != 1 || len(got) != 1 || got[0].node.ID != node.ID ||
		!reflect.DeepEqual(got[0].dependencyRefs, []string{"d1"}) ||
		!reflect.DeepEqual(got[0].familiesByDependency["d1"], []surfacediscovery.ExternalCallFamily{family}) {
		t.Fatalf("exact usage join = matched:%d candidates:%#v", matched, got)
	}

	tampered := node
	tampered.Declaration.Line++
	if _, _, err := discoverExternalUsages(
		surfacediscovery.DirectCallIndex{State: surfacediscovery.DirectCallIndexReady, Nodes: []surfacediscovery.DirectCallNode{node}},
		surfacediscovery.ExternalCallIndex{
			Callers: []surfacediscovery.DirectCallNode{tampered}, Families: []surfacediscovery.ExternalCallFamily{family},
		},
		[]dependencyCandidate{dependency},
	); err == nil {
		t.Fatal("mismatched external caller authority was accepted")
	}
}
