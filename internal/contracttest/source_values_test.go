package contracttest

import (
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

func assertGoSourceValues(t *testing.T, repository *corpus.Corpus, index programindex.Index) {
	t.Helper()
	graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: index}}})
	if err != nil {
		t.Fatal(err)
	}
	assertGoOperationSourceValues(t, graph)
	assertEvidenceVocabulary(t, graph, "interface_invoke:synchronous", "declared_interface_dispatch:synchronous", "callback_transfer:synchronous",
		"callable_binding:field", "go_ssa_dynamic_handoff", "go_declared_interface_dispatch", "callback_registration", "registration_receiver_call",
		"interface_field_assignment", "literal_string", "record", "call_result", "alternatives", "unresolved", "invokes_external")
	seen := map[string]bool{}
	destinations := reading.NewDestinationReader(graph.Places)
	var visit func(*sourcevalue.Value)
	visit = func(value *sourcevalue.Value) {
		if value == nil {
			return
		}
		if err := sourcevalue.Validate(value); err != nil {
			t.Fatal(err)
		}
		seen[value.Kind] = true
		if value.Kind == "literal" && value.Text == "/시세" {
			seen["suffix"] = true
		}
		if value.Kind == "parameter" && value.Text == "base" && value.Owner != nil {
			seen["captured_owner"] = true
		}
		for i := range value.Parts {
			visit(&value.Parts[i])
		}
	}
	for _, place := range graph.Places {
		if place.Symbol == nil || place.Path != "internal/storefixture/destinations.go" {
			continue
		}
		for _, call := range place.Symbol.Calls {
			if call.Name == "http.Client.Do" && call.API != nil && call.API.Package == "net/http" {
				seen["transport_identity"] = true
				addresses := map[string]bool{}
				for _, use := range destinations.Read(place, call) {
					addresses[use.Address] = true
				}
				if place.Symbol.Decl.Name == "DestinationRequest" && (!addresses["{--price-endpoint}/시세"] || !addresses["https://audit.example/events"] || len(addresses) != 2) {
					t.Errorf("actual Go destination chains = %+v", destinations.Read(place, call))
				}
				if place.Symbol.Decl.Name == "DestinationClient.Send" && (!addresses["https://client.example/account"] || len(addresses) != 1) {
					t.Errorf("client field constructor chain = %+v", destinations.Read(place, call))
				}
				if place.Symbol.Decl.Name == "DestinationFactories" && (!addresses["https://factory.example/events"] || len(addresses) != 1) {
					t.Errorf("request factory/WithContext chain = %+v", destinations.Read(place, call))
				}
			}
			for _, argument := range call.SourceArguments {
				visit(argument.Origin)
			}
		}
	}
	for _, want := range []string{"literal", "parameter", "call_result", "concat", "suffix", "captured_owner", "transport_identity"} {
		if !seen[want] {
			t.Errorf("source value %s did not survive Go adapter and places", want)
		}
	}
}
