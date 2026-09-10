package pythonprogramindex

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/pythontarget"
)

func TestCumulativePythonBoundaryOwnersSurviveTargetRelease(t *testing.T) {
	repository := pythonCorpus(t, cumulativePythonSources(t, "src/fixture_app/runtime_registrations.py", "src/fixture_app/route_mounts.py", "src/fixture_app/django_urls.py"))
	input, err := BuildInput(t.Context(), repository, targetOfKind(t, repository, pythontarget.KindLibrary))
	if err != nil {
		t.Fatal(err)
	}
	first, err := programindex.New(input)
	if err != nil {
		t.Fatal(err)
	}
	input.Target.Selector += "/another-native-view"
	second, err := programindex.New(input)
	if err != nil {
		t.Fatal(err)
	}
	original, err := facts.Build(facts.Input{Repository: repository, Targets: []facts.TargetInput{{Index: first}}})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(original)
	var route facts.Fact
	for _, fact := range original.OfKind(facts.KindHTTPRoute) {
		if fact.Path == "/api/v1/ping" {
			route = fact
		}
	}
	if route.ObjectID == "" || route.Anchor == nil {
		t.Fatal("actual native route is missing its handler identity")
	}
	handler := objectByID(t, first, route.ObjectID)
	wantSubject := atlas.SymbolID(handler.Location.Path, handler.Location.Line, handler.Name)
	var localEval facts.Fact
	for _, fact := range original.OfKind(facts.KindDynamicExecution) {
		if fact.Key == "eval" && fact.Anchor != nil && fact.Anchor.Path == "src/fixture_app/runtime_registrations.py" {
			localEval = fact
		}
	}
	if localEval.ID == "" {
		t.Fatal("local eval disappeared from the existing native fact result")
	}
	for _, indexes := range [][]programindex.Index{{first, second}, {second, first}} {
		targets := []places.TargetInput{{Index: indexes[0]}, {Index: indexes[1]}}
		graph, err := places.Build(places.Input{Repository: repository, Targets: targets, Facts: original})
		if err != nil {
			t.Fatal(err)
		}
		var linked, retainedCall bool
		for _, place := range graph.Places {
			if place.Symbol != nil && place.Symbol.Decl.Name == "evaluate_local" {
				for _, call := range place.Symbol.Calls {
					retainedCall = retainedCall || call.Name == "eval" && call.Line == localEval.Anchor.Line
				}
			}
			if place.Boundary == nil {
				continue
			}
			if place.Boundary.FactID == localEval.ID {
				t.Fatalf("local execution was promoted to a runtime boundary: %+v", place)
			}
			if place.Boundary.ObjectID == route.ObjectID && place.Path == route.Anchor.Path && place.LineNo == route.Anchor.Line {
				linked = place.Boundary.SubjectID == wantSubject && place.Boundary.GivenKind == atlas.BoundaryHTTPServer
				if !linked {
					t.Fatalf("native handler ownership lost after target release: %+v", place)
				}
			}
		}
		if !linked || !retainedCall {
			t.Fatalf("native observations missing: route=%v eval call=%v", linked, retainedCall)
		}
		encoded, err := atlas.EncodeGraph(graph)
		if err != nil {
			t.Fatal(err)
		}
		restored, err := atlas.DecodeGraph(encoded)
		if err != nil {
			t.Fatal(err)
		}
		reencoded, err := atlas.EncodeGraph(restored)
		if err != nil || !bytes.Equal(encoded, reencoded) {
			t.Fatalf("native boundary evidence changed across its sealed graph round-trip: %v", err)
		}
	}
	// The same spelling and source line do not authorize an unknown object ID.
	route.ObjectID = "unknown-native-handler"
	unknown, err := facts.Seal(facts.Result{Targets: original.Targets, Facts: []facts.Fact{route}})
	if err != nil {
		t.Fatal(err)
	}
	graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: first}}, Facts: unknown})
	if err != nil {
		t.Fatal(err)
	}
	var keptUnknown bool
	for _, place := range graph.Places {
		if place.Boundary != nil && place.Boundary.FactID == route.ID {
			keptUnknown = true
			if place.Boundary.SubjectID != "" || place.Boundary.ObjectID != route.ObjectID {
				t.Fatalf("unknown handler was guessed from its name/location: %+v", place)
			}
		}
	}
	if !keptUnknown {
		t.Fatal("a fact without resolved ownership lost its original source observation")
	}
	after, _ := json.Marshal(original)
	if err := original.Validate(); err != nil || !bytes.Equal(before, after) {
		t.Fatalf("graph preparation changed original sealed facts: %v", err)
	}
}
