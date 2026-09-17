package places

import (
	"bytes"
	"encoding/json"
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
		{FromID: "caller" + suffix, Kind: programindex.RelationCalls, Dispatch: "interface", Resolution: programindex.ResolutionUnresolved,
			Witnesses: []programindex.Witness{
				{Kind: "interface_dispatch", Detail: "declared RoundTripper.RoundTrip via field Transport", SourceExpression: "transport.RoundTrip(request)", Location: at},
				{Kind: "interface_field_assignment", Detail: "observed Transport assignment", Location: &programindex.Location{Path: at.Path, Line: 6, Column: 4}},
			}},
		{FromID: "caller" + suffix, ToIDs: []string{"external" + suffix}, Kind: programindex.RelationInvokesExternal, Dispatch: "interface_method", Resolution: programindex.ResolutionExact,
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

func TestDeclaredInterfaceDispatchNamesTheAPIWithUnresolvedImplementation(t *testing.T) {
	target := declaredDispatchTarget("a")
	target.Index.Relations = target.Index.Relations[1:]
	calls := collectDispatchCalls(t, target)
	if len(calls) != 1 {
		t.Fatalf("one declared interface call became %d calls: %+v", len(calls), calls)
	}
	call := calls[0]
	if call.Resolution != "unresolved" || call.API == nil || call.API.Package != "net/http" || call.API.Name != "RoundTrip" ||
		len(call.CalleeIDs) != 0 || call.Dispatch != "interface_method" || call.Column != 12 {
		t.Fatalf("declared API became a resolved implementation or lost its site: %+v", call)
	}
}
