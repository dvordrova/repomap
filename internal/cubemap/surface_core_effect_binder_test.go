package cubemap

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/activitysurface"
	"github.com/dvordrova/repomap/internal/coremap"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

func TestAggregateAnchorCoreRelationUsesExactDirectionAndMinimumHops(t *testing.T) {
	tests := []struct {
		name     string
		anchor   string
		cores    []string
		forward  map[string]int
		reverse  map[string]int
		relation AnchorCoreRelation
		hops     *int
	}{
		{name: "same symbol", anchor: "n1", cores: []string{"n1"}, relation: AnchorCoreSameSymbol, hops: intPointer(0)},
		{name: "anchor reaches core", anchor: "n1", cores: []string{"n2", "n3"}, forward: map[string]int{"n2": 4, "n3": 2}, relation: AnchorReachesCore, hops: intPointer(2)},
		{name: "core reaches anchor", anchor: "n1", cores: []string{"n2"}, reverse: map[string]int{"n2": 3}, relation: CoreReachesAnchor, hops: intPointer(3)},
		{name: "shorter direction wins", anchor: "n1", cores: []string{"n2"}, forward: map[string]int{"n2": 5}, reverse: map[string]int{"n2": 2}, relation: CoreReachesAnchor, hops: intPointer(2)},
		{name: "unconnected", anchor: "n1", cores: []string{"n2"}, relation: AnchorCoreUnconnected},
		{name: "no graph-addressable representative", anchor: "n1", cores: []string{}, relation: AnchorCoreUnconnected},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := aggregateAnchorCoreRelation(test.anchor, test.cores, test.forward, test.reverse)
			if got.relation != test.relation || !equalOptionalInt(got.minHops, test.hops) {
				t.Fatalf("relation = %s/%v, want %s/%v", got.relation, got.minHops, test.relation, test.hops)
			}
		})
	}
}

func TestDirectNodeCoreJoinUsesGraphIdentityAndExactDeclarationFacts(t *testing.T) {
	location := surfacediscovery.Location{Path: "dispatch.go", Line: 10, Column: 3}
	node := surfacediscovery.DirectCallNode{
		ID: "direct-node", Symbol: surfacediscovery.Symbol{
			ID: "direct-node", Package: "example.com/app", Name: "Dispatch", Location: location,
		},
		Package: "example.com/app", Exported: true, Declaration: location,
	}
	symbol := coremap.SymbolFact{
		NodeID: "direct-node", Symbol: surfacediscovery.Symbol{
			ID: "program-object", Package: "example.com/app", Name: "Dispatch", Location: location,
		},
		Package: "example.com/app", Exported: true, Declaration: location,
	}
	if !directNodeMatchesCoreSymbol(node, symbol) {
		t.Fatal("ProgramIndex-local nested symbol identity rejected an exact DirectCall join")
	}
	symbol.Declaration.Line++
	if directNodeMatchesCoreSymbol(node, symbol) {
		t.Fatal("different declaration facts were accepted as an exact DirectCall join")
	}
}

func TestReduceSurfaceCoreEffectBindingsRestoresOnlyLocalAuthority(t *testing.T) {
	compilation := testSurfaceCoreEffectCompilation(t)
	result, err := reduceSurfaceCoreEffectBindings(compilation, []byte(`{
  "surface_core":[{"surface_ref":"s1","core_ref":"c1"}],
  "effect_core":[{"effect_ref":"e1","core_ref":"c1"}]
}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.SurfaceCore) != 1 || result.SurfaceCore[0].SurfaceID != "surface-exact" ||
		result.SurfaceCore[0].CoreBlockID != "core-exact" || result.SurfaceCore[0].Relation != AnchorCoreSameSymbol ||
		len(result.EffectCore) != 1 || result.EffectCore[0].ExternalCallFamilyID != "family-exact" ||
		result.EffectCore[0].CallerNodeID != "caller-exact" || result.EffectCore[0].CoreBlockID != "core-exact" ||
		result.EffectCore[0].Relation != AnchorReachesCore {
		t.Fatalf("restored bindings = %#v", result)
	}
	if result.Coverage.SelectedSurfaceCore != 1 || result.Coverage.SelectedEffectCore != 1 || !result.Coverage.ModelCalled {
		t.Fatalf("coverage = %#v", result.Coverage)
	}
}

func TestReduceSurfaceCoreEffectBindingsRejectsUnknownAndDuplicatePairs(t *testing.T) {
	compilation := testSurfaceCoreEffectCompilation(t)
	tests := map[string]string{
		"unknown":     `{"surface_core":[{"surface_ref":"s9","core_ref":"c1"}],"effect_core":[]}`,
		"duplicate":   `{"surface_core":[],"effect_core":[{"effect_ref":"e1","core_ref":"c1"},{"effect_ref":"e1","core_ref":"c1"}]}`,
		"extra field": `{"surface_core":[],"effect_core":[],"explanation":"guess"}`,
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := reduceSurfaceCoreEffectBindings(compilation, []byte(raw)); err == nil {
				t.Fatal("invalid refs-only response was accepted")
			}
		})
	}
}

func testSurfaceCoreEffectCompilation(t *testing.T) surfaceCoreEffectCompilation {
	t.Helper()
	zero := 0
	one := 1
	request := surfaceCoreEffectRequest{
		Target: binderTargetRow{Kind: "executable_package", ModulePath: "example.com/app", PackagePath: "example.com/app"},
		Symbols: []binderSymbolRow{
			{Ref: "n1", Path: "dispatch.go", Line: 10, Package: "example.com/app", Symbol: "Dispatch"},
			{Ref: "n2", Path: "effect.go", Line: 20, Package: "example.com/app", Symbol: "Send"},
		},
		Cores: []binderCoreRow{{
			Ref: "c1", Name: "Dispatch", Purpose: "Dispatches accepted work.",
			RepresentativeRefs: []string{"n1"},
			Representatives: []binderCoreRepresentativeRow{{
				GraphRef: "n1", Package: "example.com/app", Name: "Dispatch",
				Path: "dispatch.go", Line: 10,
			}},
			Objects: []binderCoreObjectRow{},
		}},
		Surfaces: []binderSurfaceRow{{
			Ref: "s1", AnchorRef: "n1", Kind: "http_route", Role: "entry_surface", Form: "direct_call",
			CoreRelations: []binderCoreRelationRow{{CoreRef: "c1", Relation: AnchorCoreSameSymbol, MinHops: &zero}},
		}},
		Effects: []binderEffectRow{{
			Ref: "e1", AnchorRef: "n2", DependencyName: "client", DependencyPackage: "example.com/client",
			Operation: "Do", CoreRelations: []binderCoreRelationRow{{CoreRef: "c1", Relation: AnchorReachesCore, MinHops: &one}},
		}},
	}
	wire, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	compilation := surfaceCoreEffectCompilation{
		request: request, requestWire: wire, targetRef: "target-exact", directCallSHA256: strings.Repeat("a", 64),
		surfaces: map[string]activitysurface.Surface{"s1": {ID: "surface-exact"}},
		cores:    map[string]coremap.Block{"c1": {ID: "core-exact"}},
		effects: map[string]binderEffectAuthority{"e1": {
			caller:    Symbol{NodeID: "caller-exact"},
			operation: IntegrationOperation{ExternalCallFamilyID: "family-exact"},
		}},
		surfacePairs: map[string]binderPairAuthority{
			binderPairKey("s1", "c1"): {relation: AnchorCoreSameSymbol, minHops: &zero},
		},
		effectPairs: map[string]binderPairAuthority{
			binderPairKey("e1", "c1"): {relation: AnchorReachesCore, minHops: &one},
		},
	}
	compilation.authoritySHA256, err = surfaceCoreEffectAuthoritySHA(
		compilation.targetRef, compilation.directCallSHA256, compilation.requestWire,
		compilation.surfaces, compilation.cores, compilation.effects,
	)
	if err != nil {
		t.Fatal(err)
	}
	return compilation
}
