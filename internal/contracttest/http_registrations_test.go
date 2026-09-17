package contracttest

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programindex/adaptertest"
)

func assertGoHTTPRegistrations(t *testing.T, repository *corpus.Corpus, index programindex.Index) {
	t.Helper()
	const source = "internal/storefixture/http_registrations.go"
	result, err := facts.Build(facts.Input{Targets: []facts.TargetInput{{Index: index, Root: "."}}})
	if err != nil {
		t.Fatal(err)
	}
	wanted := map[string]int{"GET /health": 1, "PATCH /tasks/{id}": 1, "HEAD status.example/{$}": 1, "ANY /v1/update": 1, "ANY /v1/metrics": 1, "ANY /direct-field": 1, "ANY /alternative": 1, "ANY /same": 2}
	wanted["ANY /unknown-handler"], wanted["ANY /changed-handler"] = 1, 1
	for _, name := range []string{"/interface-alternative", "/interface-open", "/interface-unknown", "/interface-external"} {
		wanted["ANY "+name] = 1
	}
	wanted["GET /interface-health"] = 1
	counts := make(map[string]int)
	var emptyID, emptyFactID string
	var interfaceEmptyID string
	var sameColumns []int
	for _, fact := range result.OfKind(facts.KindRegistration) {
		if fact.Anchor == nil || fact.Anchor.Path != source {
			continue
		}
		key := firstNonEmptyString(fact.Method, "ANY") + " " + fact.Path
		counts[key]++
		if _, ok := wanted[key]; !ok {
			t.Fatalf("invented route from unknown input/local method: %+v", fact)
		}
		if fact.Anchor.Column <= 0 {
			t.Fatalf("registration lost column: %+v", fact)
		}
		if fact.Path == "/health" {
			emptyID = fact.ObjectID
			emptyFactID = fact.ID
			if !strings.Contains(fact.Symbol, "ReturnedReadyHandler$") || emptyID == "" {
				t.Fatalf("empty returned callback was replaced by its factory or dropped: %+v", fact)
			}
		}
		if fact.Path == "/interface-health" {
			interfaceEmptyID = fact.ObjectID
			if !strings.Contains(fact.Symbol, "health$") || interfaceEmptyID == "" {
				t.Fatalf("named function converted to an interface lost its actual empty callback: %+v", fact)
			}
		} else if strings.HasPrefix(fact.Path, "/interface-") && fact.ObjectID != "" {
			t.Fatalf("ambiguous/unknown interface result became one known handler: %+v", fact)
		}
		if fact.Path == "/alternative" && fact.ObjectID != "" {
			t.Fatalf("alternative callbacks arbitrarily chose one owner: %+v", fact)
		}
		if (fact.Path == "/unknown-handler" || fact.Path == "/changed-handler") && fact.ObjectID != "" {
			t.Fatalf("unresolved/mutated callback borrowed another instance's initializer: %+v", fact)
		}
		if fact.Path == "/direct-field" && !strings.Contains(fact.Symbol, "ReturnedReadyHandler$") {
			t.Fatalf("actual constructor instance lost callable field: %+v", fact)
		}
		if fact.Path == "/same" {
			sameColumns = append(sameColumns, fact.Anchor.Column)
		}
		if strings.HasPrefix(fact.Path, "/v1/") {
			if len(fact.Evidence) < 3 || !strings.Contains(fact.Symbol, "requireRouteToken$") {
				t.Fatalf("constructor/wrapper path lost source chain or actual registered callback: %+v", fact)
			}
		}
	}
	for key, count := range wanted {
		if counts[key] != count {
			t.Fatalf("native route %q count=%d want=%d; all=%v", key, counts[key], count, counts)
		}
	}
	if len(sameColumns) != 2 || sameColumns[0] == sameColumns[1] {
		t.Fatalf("same-line registrations collapsed: %v", sameColumns)
	}
	interfaceArguments := 0
	for _, relation := range index.Relations {
		for _, pattern := range relation.Patterns {
			if pattern.Location == nil || pattern.Location.Path != source || len(pattern.Arguments) != 2 {
				continue
			}
			path, argument := pattern.Arguments[0].Value, pattern.Arguments[1]
			switch path {
			case "GET /interface-health":
				interfaceArguments++
				if len(argument.ObjectIDs) != 1 || argument.ObjectsObserved != 1 || argument.ObjectsOmitted != 0 || argument.ObjectIDs[0] != interfaceEmptyID || argument.Origin == nil || argument.Origin.Kind != "call_result" || argument.Origin.Text != "health" {
					t.Fatalf("interface conversion changed the source callback candidate: %+v", argument)
				}
			case "/interface-alternative", "/interface-open":
				interfaceArguments++
				wantIDs, wantUnknown := 2, 0
				if path == "/interface-open" {
					wantIDs, wantUnknown = 1, 1
				}
				if len(argument.ObjectIDs) != wantIDs || argument.ObjectsObserved != 2 || argument.ObjectsOmitted != wantUnknown {
					t.Fatalf("interface return lost alternatives/open branch: %s %+v", path, argument)
				}
			case "/interface-unknown", "/interface-external":
				interfaceArguments++
				if len(argument.ObjectIDs) != 0 || argument.ObjectsObserved != 0 || argument.ObjectsOmitted != 0 || argument.Origin == nil {
					t.Fatalf("non-callable interface lost its original value or became a callback: %s %+v", path, argument)
				}
			}
		}
	}
	if interfaceArguments != 5 {
		t.Fatalf("interface source controls missing: %d", interfaceArguments)
	}
	graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: index}}, Facts: result})
	if err != nil {
		t.Fatal(err)
	}
	kept, attached, dispatch, interfaceKept := false, false, false, false
	for _, place := range graph.Places {
		if place.Symbol != nil && place.Symbol.Decl.Name == "CallDeclaredHTTPTransport" {
			if len(place.Symbol.Calls) != 1 {
				t.Fatalf("one native interface invocation became multiple calls: %+v", place.Symbol.Calls)
			}
			call := place.Symbol.Calls[0]
			if call.API == nil || call.API.Package != "net/http" || call.API.Name != "RoundTrip" || call.Resolution != "unresolved" ||
				call.Dispatch != "interface_method" || call.Line <= 0 || call.Column <= 0 {
				t.Fatalf("interface call lost declared API, site or unresolved runtime view: %+v", call)
			}
			dispatch = true
		}
		if place.Symbol != nil && place.Symbol.Decl.ObjectID == index.Target.ID+"."+emptyID {
			kept = true
			for _, boundary := range graph.Places {
				if boundary.Boundary == nil || boundary.Boundary.SubjectID != place.ID {
					continue
				}
				for _, origin := range boundary.Boundary.Origins {
					attached = attached || origin.TargetID == index.Target.ID && origin.FactID == emptyFactID && origin.ObjectID == index.Target.ID+"."+emptyID
				}
			}
		}
		if place.Symbol != nil && place.Symbol.Decl.ObjectID == index.Target.ID+"."+interfaceEmptyID {
			for _, boundary := range graph.Places {
				interfaceKept = interfaceKept || boundary.Boundary != nil && boundary.Boundary.ObjectID == index.Target.ID+"."+interfaceEmptyID && boundary.Boundary.SubjectID == place.ID
			}
		}
	}
	if !kept || !attached {
		t.Fatalf("empty callback/fact ownership lost in places: kept=%v attached=%v", kept, attached)
	}
	if !dispatch {
		t.Fatal("native interface transport was absent from the cumulative graph")
	}
	if !interfaceKept {
		t.Fatal("empty callback returned through interface lost its native graph ownership")
	}
	assertGoMethodArgumentExpressions(t, index, graph)
}

func assertPythonHTTPRegistrations(t *testing.T, repository *corpus.Corpus, index programindex.Index) {
	t.Helper()
	const source = "src/fixture_app/http_registrations.py"
	result, err := facts.Build(facts.Input{Targets: []facts.TargetInput{{Index: index, Root: "."}}})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"/health": "empty_health_handler", "/v1/update": "empty_registered_handler", "/v1/metrics": "empty_registered_handler"}
	owners := make(map[string]bool)
	for _, fact := range result.OfKind(facts.KindRegistration) {
		if fact.Anchor == nil || fact.Anchor.Path != source {
			continue
		}
		name, exists := want[fact.Path]
		if !exists || name != fact.Symbol || fact.Method != "GET" || fact.ObjectID == "" || fact.Anchor.Column <= 0 {
			t.Fatalf("Python constructor/decorator route has wrong source authority: %+v", fact)
		}
		if fact.Path != "/health" && len(fact.Evidence) < 3 {
			t.Fatalf("Python wrapper lost constructor/registration evidence: %+v", fact)
		}
		delete(want, fact.Path)
		owners[index.Target.ID+"."+fact.ObjectID] = true
	}
	if len(want) > 0 {
		t.Fatalf("Python routes omitted: %v", want)
	}
	graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: index}}, Facts: result})
	if err != nil {
		t.Fatal(err)
	}
	for _, place := range graph.Places {
		if place.Symbol != nil {
			delete(owners, place.Symbol.Decl.ObjectID)
		}
	}
	if len(owners) > 0 {
		t.Fatalf("Python empty registered handlers omitted from graph: %v", owners)
	}
	adaptertest.AssertMethodArgumentPositions(t, graph, source, "pass_method_arguments", "receive_method_arguments")
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
