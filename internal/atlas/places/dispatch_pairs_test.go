package places

import (
	"bytes"
	"encoding/json"
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/programindex"
)

func declaredDispatchTarget(suffix string) TargetInput {
	at := &programindex.Location{Path: "app/client.go", Line: 11, Column: 12}
	return TargetInput{Root: "app", Index: programindex.Index{Target: programindex.Target{ID: suffix, Language: "go"}, Objects: []programindex.Object{
		{ID: "caller" + suffix, Name: "Run", Kind: programindex.ObjectFunction, Location: &programindex.Location{Path: at.Path, Line: 10, Column: 6}},
		{ID: "local" + suffix, Name: "Send", Kind: programindex.ObjectFunction, Location: &programindex.Location{Path: "app/store.go", Line: 20, Column: 6}},
		{ID: "external" + suffix, Name: "RoundTrip", Kind: programindex.ObjectExternalSymbol, External: &programindex.ExternalSymbol{PackagePath: "net/http", Receiver: "RoundTripper", Name: "RoundTrip"}},
	}, Relations: []programindex.Relation{
		{FromID: "caller" + suffix, Kind: programindex.RelationCalls, Invocation: "interface_invoke:synchronous", Resolution: programindex.ResolutionUnresolved,
			Witnesses: []programindex.Witness{
				{Kind: "interface_dispatch", Detail: "declared RoundTripper.RoundTrip via field Transport", SourceExpression: "transport.RoundTrip(request)", Location: at},
				{Kind: "interface_field_assignment", Detail: "observed Transport assignment", Location: &programindex.Location{Path: at.Path, Line: 6, Column: 4}},
			}},
		{FromID: "caller" + suffix, ToIDs: []string{"external" + suffix}, Kind: programindex.RelationInvokesExternal, Invocation: "declared_interface_dispatch:synchronous", Resolution: programindex.ResolutionExact,
			Witnesses: []programindex.Witness{{Kind: "native_external_call", Detail: "declared interface API", SourceExpression: "transport.RoundTrip(request)", Location: at}},
			Patterns:  []programindex.RelationPattern{{Selector: "RoundTrip", Location: at}}},
	}}}
}

func collectDispatchCalls(t *testing.T, targets ...TargetInput) []atlas.SymbolCall {
	t.Helper()
	before, _ := json.Marshal(targets)
	b := builder{input: Input{Targets: targets}, files: map[string]*fileState{}, byID: map[string]programindex.Object{}, fileOf: map[string]string{}, targetOf: map[string]map[string]struct{}{}}
	for _, target := range targets {
		b.collectObjects(target)
	}
	result := b.symbolCalls()[atlas.SymbolID("app/client.go", 10, "Run")]
	after, _ := json.Marshal(targets)
	if !bytes.Equal(before, after) {
		t.Fatal("dispatch reconciliation mutated the original native observations")
	}
	return result
}

func TestPatternedCallsPreserveTheirNativeResolution(t *testing.T) {
	for _, resolution := range []programindex.Resolution{programindex.ResolutionExact, programindex.ResolutionAlternatives, programindex.ResolutionUnresolved} {
		t.Run(string(resolution), func(t *testing.T) {
			target := declaredDispatchTarget("a")
			target.Index.Relations = []programindex.Relation{{FromID: "callera", ToIDs: []string{"locala"}, Kind: programindex.RelationCalls, Resolution: resolution,
				Patterns: []programindex.RelationPattern{{Selector: "Send", Location: &programindex.Location{Path: "app/client.go", Line: 11, Column: 12}}}}}
			calls := collectDispatchCalls(t, target)
			if len(calls) != 1 || calls[0].Resolution != string(resolution) || len(calls[0].CalleeIDs) != 1 || calls[0].API != nil {
				t.Fatalf("native call target and certainty changed: %+v", calls)
			}
		})
	}
}

func TestDeclaredInterfaceViewsKeepOneInvocationAndBothOriginalObservations(t *testing.T) {
	forward := declaredDispatchTarget("a")
	backward := declaredDispatchTarget("b")
	slices.Reverse(backward.Index.Relations[0].Witnesses)
	slices.Reverse(backward.Index.Relations)
	calls := collectDispatchCalls(t, forward, backward)
	reversed := collectDispatchCalls(t, backward, forward)
	encoded, _ := json.Marshal(calls)
	other, _ := json.Marshal(reversed)
	if !bytes.Equal(encoded, other) || len(calls) != 1 {
		t.Fatalf("duplicate source views changed invocation identity or source order:\n%s\n%s", encoded, other)
	}
	call := calls[0]
	if call.Resolution != "unresolved" || call.API == nil || call.API.Package != "net/http" || call.API.Name != "RoundTrip" || len(call.CalleeIDs) != 0 || len(call.DispatchObservations) != 2 {
		t.Fatalf("declared API became a resolved implementation or lost provenance: %+v", call)
	}
	unresolved, declared := call.DispatchObservations[0], call.DispatchObservations[1]
	if unresolved.Kind != "calls" || unresolved.Resolution != "unresolved" || unresolved.Invocation != "interface_invoke:synchronous" || unresolved.Detail == "" || len(unresolved.Witnesses) != 2 {
		t.Fatalf("unresolved source observation was discarded: %+v", unresolved)
	}
	if declared.Kind != "invokes_external" || declared.Resolution != "exact" || declared.Invocation != "declared_interface_dispatch:synchronous" || len(declared.Witnesses) != 1 {
		t.Fatalf("declared API observation was rewritten: %+v", declared)
	}
	for _, witness := range declared.Witnesses {
		if witness.Path != "app/client.go" || witness.Line != 11 || witness.Column != 12 || witness.SourceExpression != "transport.RoundTrip(request)" || witness.Detail != "declared interface API" {
			t.Fatalf("original call witness changed: %+v", witness)
		}
	}
}

func TestDeclaredInterfaceCounterpartsRequireSameNativeScopeAndUnambiguousSite(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*programindex.Index)
		count  int
	}{
		{"different column", func(i *programindex.Index) {
			i.Relations[1].Patterns[0].Location = &programindex.Location{Path: "app/client.go", Line: 11, Column: 30}
		}, 2},
		{"unknown column", func(i *programindex.Index) {
			i.Relations[1].Patterns[0].Location = &programindex.Location{Path: "app/client.go", Line: 11}
		}, 2},
		{"different dispatch", func(i *programindex.Index) { i.Relations[1].Invocation = "declared_interface_dispatch:goroutine" }, 2},
		{"local possible implementation", func(i *programindex.Index) {
			i.Relations[0].ToIDs = []string{"locala"}
			i.Relations[0].Resolution = programindex.ResolutionAlternatives
		}, 2},
		{"another API at same site", func(i *programindex.Index) {
			i.Objects = append(i.Objects, programindex.Object{ID: "other-api", Name: "RoundTrip", Kind: programindex.ObjectExternalSymbol, External: &programindex.ExternalSymbol{PackagePath: "example/transport", Receiver: "RoundTripper", Name: "RoundTrip"}})
			other := i.Relations[1]
			other.ToIDs = []string{"other-api"}
			i.Relations = append(i.Relations, other)
		}, 3},
		{"independent unresolved witness", func(i *programindex.Index) {
			other := i.Relations[0]
			other.Witnesses = []programindex.Witness{{Kind: "interface_dispatch", Detail: "different receiver evidence", Location: &programindex.Location{Path: "app/client.go", Line: 11, Column: 12}}}
			i.Relations = append(i.Relations, other)
		}, 3},
		{"different native caller", func(i *programindex.Index) {
			other := i.Objects[0]
			other.ID = "another-caller"
			i.Objects = append(i.Objects, other)
			i.Relations[1].FromID = other.ID
		}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target := declaredDispatchTarget("a")
			tc.change(&target.Index)
			calls := collectDispatchCalls(t, target)
			if len(calls) != tc.count {
				t.Fatalf("independent observations collapsed: %+v", calls)
			}
			for _, call := range calls {
				if len(call.DispatchObservations) != 0 {
					t.Fatalf("unproved pair acquired a merged observation: %+v", call)
				}
			}
		})
	}
	// Matching compiler-located places across targets do not prove that one
	// native target observed the two complementary views of a call.
	a, b := declaredDispatchTarget("a"), declaredDispatchTarget("b")
	a.Index.Relations = a.Index.Relations[:1]
	b.Index.Relations = b.Index.Relations[1:]
	if calls := collectDispatchCalls(t, a, b); len(calls) != 2 {
		t.Fatalf("cross-target observations became a paired native invocation: %+v", calls)
	}
}

func TestDeclaredInterfaceFamilyKeepsDistinctColumnsAndTheirWitnesses(t *testing.T) {
	target := declaredDispatchTarget("a")
	for i := range target.Index.Relations {
		second := target.Index.Relations[i].Witnesses[0]
		second.Location = &programindex.Location{Path: "app/client.go", Line: 11, Column: 30}
		target.Index.Relations[i].Witnesses = append(target.Index.Relations[i].Witnesses, second)
	}
	second := target.Index.Relations[1].Patterns[0]
	second.Location = &programindex.Location{Path: "app/client.go", Line: 11, Column: 30}
	target.Index.Relations[1].Patterns = append(target.Index.Relations[1].Patterns, second)
	calls := collectDispatchCalls(t, target)
	if len(calls) != 2 || calls[0].Column == calls[1].Column {
		t.Fatalf("two actual calls on one source line collapsed: %+v", calls)
	}
	for _, call := range calls {
		if len(call.DispatchObservations) != 2 {
			t.Fatalf("exact call pair was not recognized: %+v", call)
		}
		for _, view := range call.DispatchObservations {
			for _, witness := range view.Witnesses {
				if witness.Kind != "interface_field_assignment" && (witness.Line != call.Line || witness.Column != call.Column) {
					t.Fatalf("another call's family witness leaked into this invocation: %+v", call)
				}
			}
		}
	}
}
