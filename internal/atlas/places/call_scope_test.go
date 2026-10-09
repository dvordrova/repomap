package places

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/programindex"
)

func callScopeRelation(suffix string, resolution programindex.Resolution) programindex.Relation {
	relation := programindex.Relation{
		FromID: "caller" + suffix, Kind: programindex.RelationCalls,
		Dispatch: "interface", Resolution: resolution,
		Witnesses: []programindex.Witness{{Kind: "interface_dispatch", Detail: "dispatch through Send",
			Location: &programindex.Location{Path: "app/client.go", Line: 11, Column: 12}}},
	}
	if resolution != programindex.ResolutionUnresolved {
		relation.ToIDs = []string{"local" + suffix}
	}
	return relation
}

func callScopeTarget(suffix string, resolutions ...programindex.Resolution) TargetInput {
	target := declaredDispatchTarget(suffix)
	target.Index.Relations = nil
	for _, resolution := range resolutions {
		target.Index.Relations = append(target.Index.Relations, callScopeRelation(suffix, resolution))
	}
	return target
}

func callScopeIndexes(targets []TargetInput) []programindex.Index {
	indexes := make([]programindex.Index, len(targets))
	for i, target := range targets {
		indexes[i] = target.Index
	}
	return indexes
}

func collectImmutableCallScopes(t *testing.T, targets ...TargetInput) []atlas.SymbolCall {
	t.Helper()
	before, err := json.Marshal(callScopeIndexes(targets))
	if err != nil {
		t.Fatal(err)
	}
	calls := collectDispatchCalls(t, targets...)
	after, err := json.Marshal(callScopeIndexes(targets))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("call reconciliation mutated native indexes: %v", err)
	}
	return calls
}

func callScopeFind(t *testing.T, calls []atlas.SymbolCall, resolution programindex.Resolution) atlas.SymbolCall {
	t.Helper()
	var found []atlas.SymbolCall
	for _, call := range calls {
		if call.Resolution == string(resolution) {
			found = append(found, call)
		}
	}
	if len(found) != 1 {
		t.Fatalf("want one %s observation, got %+v", resolution, calls)
	}
	return found[0]
}

func TestIdenticalNativeCallsMergeOnlyTheirObserverSets(t *testing.T) {
	a := callScopeTarget("a", programindex.ResolutionAlternatives)
	b := callScopeTarget("b", programindex.ResolutionAlternatives)
	original := collectImmutableCallScopes(t, a)
	for _, targets := range [][]TargetInput{{a, b}, {b, a}} {
		calls := collectImmutableCallScopes(t, targets...)
		if len(calls) != 1 || !slices.Equal(calls[0].TargetIDs, []string{"a", "b"}) {
			t.Fatalf("identical source observation was duplicated or lost a native observer: %+v", calls)
		}
		call := calls[0]
		call.TargetIDs = original[0].TargetIDs
		if !reflect.DeepEqual(call, original[0]) {
			t.Fatalf("merging observers changed the original call evidence: %+v, want %+v", call, original[0])
		}
	}
}

func TestDisjointUnresolvedAndAlternativeCallsKeepTheirNativeScopes(t *testing.T) {
	calls := collectImmutableCallScopes(t,
		callScopeTarget("a", programindex.ResolutionUnresolved),
		callScopeTarget("b", programindex.ResolutionAlternatives))
	if len(calls) != 2 {
		t.Fatalf("one target's possible receiver erased another target's unresolved call: %+v", calls)
	}
	unknown := callScopeFind(t, calls, programindex.ResolutionUnresolved)
	possible := callScopeFind(t, calls, programindex.ResolutionAlternatives)
	if !slices.Equal(unknown.TargetIDs, []string{"a"}) || len(unknown.CalleeIDs) != 0 ||
		!slices.Equal(possible.TargetIDs, []string{"b"}) ||
		!slices.Equal(possible.CalleeIDs, []string{atlas.SymbolID("app/store.go", 20, "Send")}) {
		t.Fatalf("native scopes or receiver certainty were combined: %+v", calls)
	}
}

func TestAlternativeCallRemovesOnlyMatchingUnresolvedObservers(t *testing.T) {
	calls := collectImmutableCallScopes(t,
		callScopeTarget("a", programindex.ResolutionUnresolved),
		callScopeTarget("b", programindex.ResolutionUnresolved, programindex.ResolutionAlternatives))
	if len(calls) != 2 {
		t.Fatalf("partial overlap discarded an independent observation: %+v", calls)
	}
	unknown := callScopeFind(t, calls, programindex.ResolutionUnresolved)
	possible := callScopeFind(t, calls, programindex.ResolutionAlternatives)
	if !slices.Equal(unknown.TargetIDs, []string{"a"}) || !slices.Equal(possible.TargetIDs, []string{"b"}) {
		t.Fatalf("reconciliation removed scopes outside the actual overlap: %+v", calls)
	}
	if unknown.Line != 11 || unknown.Column != 12 || possible.Line != 11 || possible.Column != 12 ||
		unknown.Detail != possible.Detail || unknown.Dispatch != possible.Dispatch {
		t.Fatalf("reconciliation changed the original source site: %+v", calls)
	}
}

func TestNativeCallToAnotherTargetsOwnedSourceRemainsOriginalEvidence(t *testing.T) {
	a := callScopeTarget("a", programindex.ResolutionExact)
	a.Index.Relations[0].Dispatch = ""
	b := callScopeTarget("b")
	b.Root = "app/store"
	b.Index.Objects = b.Index.Objects[1:2]
	for _, target := range []*TargetInput{&a, &b} {
		for i := range target.Index.Objects {
			object := &target.Index.Objects[i]
			if object.Kind == programindex.ObjectFunction && object.Name == "Send" {
				location := *object.Location
				location.Path = "app/store/store.go"
				object.Location = &location
			}
		}
	}
	targets := []TargetInput{a, b}
	before, err := json.Marshal(callScopeIndexes(targets))
	if err != nil {
		t.Fatal(err)
	}
	builder := builder{input: Input{Targets: targets}, files: map[string]*fileState{},
		byID: map[string]programindex.Object{}, fileOf: map[string]string{}, targetOf: map[string]map[string]struct{}{}}
	for _, target := range targets {
		builder.collectObjects(target)
	}
	builder.claimByRoot()
	if !reflect.DeepEqual(builder.files["app/store/store.go"].targets, map[string]struct{}{"b": {}}) {
		t.Fatal("contrast did not establish the callee's independent source owner")
	}
	calls := builder.symbolCalls()[atlas.SymbolID("app/client.go", 10, "Run")]
	if len(calls) != 1 || calls[0].Resolution != string(programindex.ResolutionExact) ||
		!slices.Equal(calls[0].TargetIDs, []string{"a"}) ||
		!slices.Equal(calls[0].CalleeIDs, []string{atlas.SymbolID("app/store/store.go", 20, "Send")}) {
		t.Fatalf("callee source ownership erased or reassigned the original call: %+v", calls)
	}
	after, err := json.Marshal(callScopeIndexes(targets))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("source ownership or call reconciliation mutated native inputs: %v", err)
	}
}
