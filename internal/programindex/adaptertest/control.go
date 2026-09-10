package adaptertest

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/programindex"
)

// Control names a source statement enclosing one fixture call's execution.
type Control struct {
	Line int
	Kind string
}

// AssertRegistrationArgument checks that each callback receives the literal
// of its own registration, without borrowing another call site's route/topic.
func AssertRegistrationArgument(t testing.TB, graph atlas.Graph, source, callback string, want map[int]string) {
	t.Helper()
	seen := make(map[int]bool)
	for _, place := range graph.Places {
		if place.Symbol == nil || place.Symbol.Decl.Name != callback {
			continue
		}
		for _, binding := range place.Symbol.Bindings {
			value, ok := want[binding.Line]
			if !ok || binding.Path != source || binding.To != callback {
				continue
			}
			if len(binding.Arguments) != 1 || binding.Arguments[0].Value != value ||
				binding.Arguments[0].Path != source || binding.Arguments[0].Line != binding.Line {
				t.Fatalf("%s registration at %s:%d lost or borrowed arguments: %#v", callback, source, binding.Line, binding.Arguments)
			}
			row, err := json.Marshal(lines.SymbolRow(place, "").Fields)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(row), value) {
				t.Fatalf("callback row lost registered value %q", value)
			}
			seen[binding.Line] = true
		}
	}
	if len(seen) != len(want) {
		t.Fatalf("%s registrations = %v, want %v", callback, seen, want)
	}
}

// AssertCallControls checks native call-site observations and the evidence sent
// to symbol selection. Anonymous Go closures remain native-only declarations.
func AssertCallControls(t testing.TB, index programindex.Index, graph atlas.Graph, path, selector string, want map[int][]Control) {
	t.Helper()
	seen := make(map[int]bool)
	for _, relation := range index.Relations {
		for _, pattern := range relation.Patterns {
			if pattern.Selector != selector || pattern.Location == nil || pattern.Location.Path != path {
				continue
			}
			expected, ok := want[pattern.Location.Line]
			if !ok {
				t.Fatalf("unexpected %s call at %s:%d", selector, path, pattern.Location.Line)
			}
			seen[pattern.Location.Line] = true
			var got []Control
			for _, witness := range pattern.Context {
				if witness.Kind != "control_context" || witness.Location == nil || witness.Location.Path != path {
					t.Fatalf("control observation lost its source: %#v", witness)
				}
				got = append(got, Control{witness.Location.Line, witness.Detail})
			}
			if !reflect.DeepEqual(got, expected) {
				t.Fatalf("call at %s:%d: control = %#v, want %#v", path, pattern.Location.Line, got, expected)
			}
		}
	}
	if len(seen) != len(want) {
		t.Fatalf("%s call sites = %v, want %v", path, seen, want)
	}
	projected := make(map[int]bool)
	for _, place := range graph.Places {
		if place.Symbol == nil || place.Path != path {
			continue
		}
		for _, call := range place.Symbol.Calls {
			if call.Name != selector {
				continue
			}
			expected, ok := want[call.Line]
			if !ok {
				t.Fatalf("unexpected projected %s at %d", selector, call.Line)
			}
			projected[call.Line] = true
			var got []Control
			for _, evidence := range call.Evidence {
				if evidence.Extractor == "control_context" {
					if evidence.Path != path {
						t.Fatalf("control context changed source: %#v", evidence)
					}
					got = append(got, Control{evidence.LineNo, evidence.Label})
				}
			}
			if !reflect.DeepEqual(got, expected) {
				t.Fatalf("atlas call at %d: control = %#v, want %#v", call.Line, got, expected)
			}
			row, err := json.Marshal(lines.SymbolRow(place, "").Fields)
			if err != nil {
				t.Fatal(err)
			}
			for _, control := range expected {
				if !strings.Contains(string(row), control.Kind) || !strings.Contains(string(row), "source_evidence") {
					t.Fatalf("provider row omitted anchored control context at %d", call.Line)
				}
			}
		}
	}
	for line, expected := range want {
		if len(expected) > 0 && !projected[line] {
			t.Fatalf("control-bearing call at %s:%d disappeared before the provider row", path, line)
		}
	}
}
