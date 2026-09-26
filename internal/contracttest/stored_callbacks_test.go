package contracttest

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/programindex"
)

// storedCallbackRelation is one relation of a fixture file as the checks below
// read it: who, what, where and how certain.
type storedCallbackRelation struct {
	relation programindex.Relation
	from     string
	to       []string
	line     int
}

func storedCallbackRelations(index programindex.Index, path string) []storedCallbackRelation {
	names := make(map[string]string, len(index.Objects))
	for _, object := range index.Objects {
		names[object.ID] = object.Name
	}
	var result []storedCallbackRelation
	for _, relation := range index.Relations {
		if relation.Location == nil || relation.Location.Path != path {
			continue
		}
		view := storedCallbackRelation{relation: relation, from: names[relation.FromID], line: relation.Location.Line}
		for _, id := range relation.ToIDs {
			view.to = append(view.to, names[id])
		}
		sort.Strings(view.to)
		result = append(result, view)
	}
	return result
}

// storedCallbackExpectation is what one source line of an equivalents example
// must hold: a relation kind from a named caller to exact targets, or to none.
type storedCallbackExpectation struct {
	kind       programindex.RelationKind
	from       string
	to         string
	resolution programindex.Resolution
}

func assertStoredCallbackLines(t *testing.T, relations []storedCallbackRelation, kinds map[programindex.RelationKind]bool, want map[int]storedCallbackExpectation) {
	t.Helper()
	seen := make(map[int]bool)
	for _, view := range relations {
		if !kinds[view.relation.Kind] {
			continue
		}
		expected, ok := want[view.line]
		if !ok || expected.kind != view.relation.Kind {
			continue
		}
		if view.from != expected.from || strings.Join(view.to, ",") != expected.to || view.relation.Resolution != expected.resolution {
			t.Fatalf("line %d: %s %s -> %v %s, want %s -> %q %s", view.line, view.relation.Kind, view.from, view.to,
				view.relation.Resolution, expected.from, expected.to, expected.resolution)
		}
		if seen[view.line] {
			t.Fatalf("line %d holds two %s relations", view.line, view.relation.Kind)
		}
		seen[view.line] = true
	}
	for line, expected := range want {
		if !seen[line] {
			t.Fatalf("line %d lost its %s relation from %s", line, expected.kind, expected.from)
		}
	}
}

// The Python equivalents of a callback stored under a branch and a call
// through a stored function value (src/fixture_app/stored_callbacks.py).
// Python keeps no binding for a dict of handlers; PYTHON.md records it.
func assertPythonStoredCallbacks(t *testing.T, index programindex.Index) {
	t.Helper()
	const path = "src/fixture_app/stored_callbacks.py"
	relations := storedCallbackRelations(index, path)
	kinds := map[programindex.RelationKind]bool{
		programindex.RelationCalls: true, programindex.RelationWrites: true, programindex.RelationPassesCallback: true,
	}
	unresolved, exact := programindex.ResolutionUnresolved, programindex.ResolutionExact
	assertStoredCallbackLines(t, relations, kinds, map[int]storedCallbackExpectation{
		// Each store stays a write of its own attribute where it is written.
		11: {programindex.RelationWrites, "register", "on_read", exact},
		13: {programindex.RelationWrites, "register", "on_write", exact},
		// The calls through the attributes gain no handler.
		16: {programindex.RelationCalls, "fire", "", unresolved},
		17: {programindex.RelationCalls, "fire", "", unresolved},
		// Each handler keeps its registration at its own call.
		30: {programindex.RelationPassesCallback, "run_event_loop", "accept_client", exact},
		31: {programindex.RelationPassesCallback, "run_event_loop", "flush_replies", exact},
		38: {programindex.RelationCalls, "run_single_handler", "accept_client", exact},
	})
	for _, view := range relations {
		if view.from == "fire" && view.relation.Kind == programindex.RelationCalls && len(view.to) != 0 {
			t.Fatalf("a call through a stored attribute borrowed a registered handler: %+v", view)
		}
		if view.relation.Kind == programindex.RelationPassesCallback && view.relation.SourceArgumentID == "" {
			t.Fatalf("registration lost its source argument: %+v", view)
		}
	}
}

// The Go equivalents of a C command table, a callback stored under a branch
// and a call through a stored function value
// (internal/storefixture/command_table.go).
func assertGoCommandTableAndStoredCallbacks(t *testing.T, repository *corpus.Corpus, index programindex.Index) {
	t.Helper()
	const path = "internal/storefixture/command_table.go"
	relations := storedCallbackRelations(index, path)

	// {Name: "get", Arity: 2, Run: getCommand} and the "set" row: each row
	// is its own binding, carrying its own literals and no other row's.
	rows := map[string]struct {
		line   int
		fields []string
	}{
		"getCommand": {13, []string{`Arity = 2`, `Name = "get"`}},
		"setCommand": {14, []string{`Arity = 3`, `Name = "set"`}},
	}
	seenRows := 0
	for _, view := range relations {
		if view.from != "commandTable" {
			continue
		}
		relation := view.relation
		want, ok := rows[strings.Join(view.to, ",")]
		if !ok || relation.Kind != programindex.RelationPassesCallback || relation.Resolution != programindex.ResolutionExact ||
			relation.Invocation != "" || view.line != want.line {
			t.Fatalf("command table row lost its exact handler or became an execution: %+v", view)
		}
		primary := false
		var fields []string
		for _, witness := range relation.Witnesses {
			switch witness.Kind {
			case "go_ssa_dynamic_handoff":
				primary = true
			case "callable_receiver_field":
				if witness.Location == nil || witness.Location.Path != path || witness.Location.Line != want.line {
					t.Fatalf("row literal lost its own row: %+v", witness)
				}
				fields = append(fields, witness.Detail)
			}
		}
		sort.Strings(fields)
		if !primary || strings.Join(fields, "|") != strings.Join(want.fields, "|") {
			t.Fatalf("row %v fields = %v primary=%v, want %v", view.to, fields, primary, want.fields)
		}
		seenRows++
	}
	if seenRows != len(rows) {
		t.Fatalf("command table rows = %d, want %d", seenRows, len(rows))
	}

	// The same rows reach the reading as bindings of each handler, each with
	// its own name.
	graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: index}}})
	if err != nil {
		t.Fatal(err)
	}
	for handler, name := range map[string]string{"getCommand": `Name = "get"`, "setCommand": `Name = "set"`} {
		object := programIndexObjectNamed(t, index, programindex.ObjectFunction, handler, path)
		labels := map[string]bool{}
		for _, place := range graph.Places {
			if place.Symbol == nil || place.Symbol.Decl.ObjectID != index.Target.ID+"."+object.ID {
				continue
			}
			for _, binding := range place.Symbol.Bindings {
				if !strings.HasSuffix(binding.From, "commandTable") || binding.Resolution != "exact" || binding.Path != path {
					continue
				}
				for _, evidence := range binding.Evidence {
					if evidence.Extractor == "callable_receiver_field" {
						labels[evidence.Label] = true
					}
				}
			}
		}
		if !labels[name] || len(labels) != 2 {
			t.Fatalf("%s binding carries %v, want its own row with %s", handler, labels, name)
		}
	}

	// Calls through stored function values: one store before the call is
	// exact; a looked-up row, a field filled through a parameter and a store
	// under a branch stay unresolved, never alternatives. The stores stay
	// bindings where they are written, and each handler keeps its
	// registration at its own call.
	unresolved, exact := programindex.ResolutionUnresolved, programindex.ResolutionExact
	calls, callbacks := programindex.RelationCalls, programindex.RelationPassesCallback
	assertStoredCallbackLines(t, relations, map[programindex.RelationKind]bool{calls: true, callbacks: true}, map[int]storedCallbackExpectation{
		38: {calls, "DispatchCommand", "", unresolved},
		51: {callbacks, "register", "", unresolved},
		53: {callbacks, "register", "", unresolved},
		58: {calls, "fire", "", unresolved},
		59: {calls, "fire", "", unresolved},
		67: {callbacks, "RunEventLoop", "acceptClient", exact},
		68: {callbacks, "RunEventLoop", "flushReplies", exact},
		74: {callbacks, "RunSingleHandler", "acceptClient", exact},
		75: {calls, "RunSingleHandler", "acceptClient", exact},
		82: {callbacks, "RunChosenHandler", "acceptClient", exact},
		84: {calls, "RunChosenHandler", "", unresolved},
		// The same loop with interface-typed fields: readyLoop.register
		// stores its handler into read or write under a branch, and
		// RunChosenReady stores one under a branch. No call through a field
		// gains a handler, not even the one registered for the other field.
		112: {calls, "fire", "", unresolved},
		113: {calls, "fire", "", unresolved},
		129: {calls, "RunChosenReady", "", unresolved},
	})
	throughFields := map[int]bool{38: true, 58: true, 59: true, 75: true, 84: true}
	for _, view := range relations {
		functionValue := view.relation.Dispatch == programindex.DispatchFunctionValue
		if functionValue != (view.relation.Kind == calls && throughFields[view.line]) {
			t.Fatalf("function-value dispatch on the wrong relation: %+v", view)
		}
		if view.relation.Kind == callbacks && (view.line == 67 || view.line == 68) && view.relation.SourceArgumentID == "" {
			t.Fatalf("registration lost its source argument: %+v", view)
		}
	}

	// What the stores put into an open interface field stays with the call as
	// its witnesses, each at its store, as the C adapter names them.
	const fixturePackage = "example.com/repomap/cumulative-go-fixture/internal/storefixture."
	stored := func(handler, field string, line int) string {
		return fmt.Sprintf("(%s).Handle stored in readyLoop.%s under a condition@%d", handler, field, line)
	}
	read := []string{stored("acceptReady", "read", 105), stored("acceptReady", "read", 127), stored("flushReady", "read", 105)}
	openCalls := map[int][]string{
		112: read,
		113: {stored("acceptReady", "write", 107), stored("flushReady", "write", 107)},
		129: read,
	}
	// Each handler keeps its exact registration at its own call: call line
	// to the line of the Handle method it binds.
	registrations := map[int]int{118: 95, 119: 96}
	objects := make(map[string]programindex.Object, len(index.Objects))
	for _, object := range index.Objects {
		objects[object.ID] = object
	}
	seenOpen, seenRegistrations := 0, 0
	for _, view := range relations {
		relation := view.relation
		if want, ok := openCalls[view.line]; ok && relation.Kind == calls {
			var witnesses []string
			for _, witness := range relation.Witnesses {
				if witness.Kind == "interface_field_assignment" && witness.Location != nil && witness.Location.Path == path {
					witnesses = append(witnesses, strings.ReplaceAll(witness.Detail, fixturePackage, "")+"@"+strconv.Itoa(witness.Location.Line))
				}
			}
			sort.Strings(witnesses)
			if relation.Dispatch != programindex.DispatchInterface || strings.Join(witnesses, "|") != strings.Join(want, "|") {
				t.Fatalf("line %d: open field call witnesses = %v dispatch %s, want %v", view.line, witnesses, relation.Dispatch, want)
			}
			seenOpen++
		}
		if handler, ok := registrations[view.line]; ok && relation.Kind == programindex.RelationBindsImplementation {
			if view.from != "RunReadyLoop" || relation.Resolution != exact || len(relation.ToIDs) != 1 ||
				objects[relation.ToIDs[0]].Location == nil || objects[relation.ToIDs[0]].Location.Line != handler {
				t.Fatalf("line %d: handler lost its exact registration: %+v", view.line, view)
			}
			seenRegistrations++
		}
	}
	if seenOpen != len(openCalls) || seenRegistrations != len(registrations) {
		t.Fatalf("open field calls = %d, registrations = %d, want %d and %d", seenOpen, seenRegistrations, len(openCalls), len(registrations))
	}
}
