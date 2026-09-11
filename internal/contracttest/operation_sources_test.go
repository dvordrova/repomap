package contracttest

import (
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// These observations distinguish two uses of net/http.Header.Set and identify
// continuation of the same request. They establish source context, not roles.
// The reading package separately checks this context in its prepared request.
func assertGoOperationSourceValues(t *testing.T, graph atlas.Graph) {
	t.Helper()
	const path = "internal/storefixture/http_registrations.go"
	seen := map[string]bool{}
	parameter := func(value *sourcevalue.Value, name string, position, ownerLine int) bool {
		return value != nil && value.Kind == "parameter" && value.Text == name && value.Position == position &&
			value.Owner != nil && value.Owner.Path == path && value.Owner.Line == ownerLine
	}
	for _, place := range graph.Places {
		if place.Symbol == nil || place.Path != path {
			continue
		}
		for _, call := range place.Symbol.Calls {
			if call.API == nil || call.API.Package != "net/http" {
				continue
			}
			switch {
			case call.Line == 127 && call.API.Name == "Set":
				v := call.ReceiverValue
				if v == nil || v.Kind != "field" || v.Text != "Header" || len(v.Parts) != 1 || !parameter(&v.Parts[0], "r", 2, 126) || call.Column <= 0 {
					t.Fatalf("request header lost its original receiver: %+v receiver=%+v", call, v)
				}
				if len(call.SourceArguments) != 2 || call.SourceArguments[1].Origin == nil || call.SourceArguments[1].Origin.Text != "fixture-request" {
					t.Fatalf("request mutation lost source arguments: %+v", call)
				}
				seen["request"] = true
			case call.Line == 128 && call.API.Name == "ServeHTTP":
				if !parameter(call.ReceiverValue, "next", 1, 125) || len(call.SourceArguments) != 2 ||
					!parameter(call.SourceArguments[0].Origin, "w", 1, 126) || !parameter(call.SourceArguments[1].Origin, "r", 2, 126) ||
					call.Resolution != "unresolved" || len(call.DispatchObservations) != 2 {
					t.Fatalf("middleware lost captured continuation, same-request arguments or runtime uncertainty: %+v", call)
				}
				seen["continuation"] = true
			case call.Line == 133 && call.API.Name == "Header":
				if !parameter(call.ReceiverValue, "w", 1, 132) {
					t.Fatalf("response header lookup lost writer receiver: %+v", call)
				}
				seen["response_lookup"] = true
			case call.Line == 133 && call.API.Name == "Set":
				v := call.ReceiverValue
				if v == nil || v.Kind != "call_result" || v.Anchor == nil || v.Anchor.Path != path || v.Anchor.Line != 133 || v.Anchor.Column <= 0 {
					t.Fatalf("response mutation lost its original header lookup: %+v", call)
				}
				found := false
				for _, lookup := range place.Symbol.Calls {
					found = found || lookup.Line == v.Anchor.Line && lookup.Column == v.Anchor.Column && lookup.API != nil && lookup.API.Name == "Header"
				}
				if !found {
					t.Fatalf("response header call-result points outside its actual lookup: %+v", call)
				}
				seen["response_mutation"] = true
			}
		}
	}
	for _, name := range []string{"request", "continuation", "response_lookup", "response_mutation"} {
		if !seen[name] {
			t.Errorf("actual cumulative operation evidence missing: %s", name)
		}
	}
}
