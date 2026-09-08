package contracttest

import (
	"testing"

	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/programindex"
)

func assertGoLocalHTTPNameFacts(t *testing.T, index programindex.Index) {
	t.Helper()
	caller := programIndexObjectNamed(t, index, programindex.ObjectFunction, "ReadLocalHTTPName", "internal/storefixture/local_http_names.go")
	callee := programIndexObjectNamed(t, index, programindex.ObjectFunction, "Get", "internal/localstore/store.go")
	assertNativeHTTPNameFacts(t, index, caller, callee, 6, programindex.ResolutionExact,
		facts.Fact{Anchor: &facts.Anchor{Path: "cmd/app/main.go", Line: 17}, Symbol: "fetchLevels", Path: "/api/levels", Resolution: facts.ResolutionExact})
}

func assertPythonLocalHTTPNameFacts(t *testing.T, index programindex.Index) {
	t.Helper()
	caller := programIndexObjectNamed(t, index, programindex.ObjectFunction, "read_local_http_name", "src/fixture_app/levels.py")
	callee := programIndexObjectNamed(t, index, programindex.ObjectFunction, "get", "src/requests.py")
	assertNativeHTTPNameFacts(t, index, caller, callee, 16, programindex.ResolutionAlternatives,
		facts.Fact{Anchor: &facts.Anchor{Path: "src/fixture_app/levels.py", Line: 5}, Symbol: "fetch_level", Path: "https://catalog.example/levels/{param}", Resolution: facts.ResolutionPossible})
}

// Absence of HTTP facts is meaningful only if the native call and its precise
// local declaration survived the adapter. Real HTTP calls are positive controls.
func assertNativeHTTPNameFacts(t *testing.T, index programindex.Index, caller, callee programindex.Object, line int, resolution programindex.Resolution, control facts.Fact) {
	t.Helper()
	if caller.External != nil || callee.External != nil {
		t.Fatalf("local HTTP-like names became external objects: caller=%+v callee=%+v", caller, callee)
	}
	found := false
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationCalls || relation.FromID != caller.ID || !sameSingleID(relation.ToIDs, callee.ID) {
			continue
		}
		if relation.Resolution != resolution || relation.TargetsOmitted != 0 {
			t.Fatalf("native call lost its resolved local target: %+v", relation)
		}
		locations := []*programindex.Location{relation.Location}
		for _, witness := range relation.Witnesses {
			locations = append(locations, witness.Location)
		}
		for _, location := range locations {
			if location != nil && location.Path == caller.Location.Path && location.Line == line && location.Column > 0 {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("native call %s -> %s at %s:%d is missing", caller.Name, callee.Name, caller.Location.Path, line)
	}
	result, err := facts.Build(facts.Input{Targets: []facts.TargetInput{{Index: index, Root: "."}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	controls := 0
	for _, fact := range result.Facts {
		if fact.Kind != facts.KindHTTPCall && fact.Kind != facts.KindHTTPRoute && fact.Kind != facts.KindPortal {
			continue
		}
		if fact.ObjectID == caller.ID || (fact.Anchor != nil && fact.Anchor.Path == caller.Location.Path && fact.Anchor.Line == line) {
			t.Fatalf("local %s call acquired an HTTP fact: %+v", index.Target.Language, fact)
		}
		if fact.Kind != facts.KindHTTPCall || fact.Anchor == nil || fact.Anchor.Path != control.Anchor.Path || fact.Anchor.Line != control.Anchor.Line {
			continue
		}
		if fact.Method != "GET" || fact.Path != control.Path || fact.Symbol != control.Symbol || fact.Resolution != control.Resolution || fact.Anchor.Column <= 0 {
			t.Fatalf("real outbound HTTP control lost method, path or source: %+v", fact)
		}
		controls++
	}
	if controls != 1 {
		t.Fatalf("real outbound HTTP control at %s:%d appeared %d times, want once", control.Anchor.Path, control.Anchor.Line, controls)
	}
}
