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
	assertEvidenceVocabulary(t, graph, "interface", "interface_method", "passes_callback",
		"binds_implementation", "callback_registration", "registration_receiver_call",
		"interface_field_assignment", "literal_string", "alternatives", "unresolved", "invokes_external")
	seen := map[string]bool{}
	// The preset's decisions: Do's request, the URL a request is built
	// with, the request WithContext is called on, and the option
	// DestinationApplication declares with flag.String, whose value
	// flag.String returns.
	options := map[sourcevalue.Anchor]string{}
	for _, place := range graph.Places {
		if place.Symbol == nil {
			continue
		}
		for _, call := range place.Symbol.Calls {
			if call.API != nil && call.API.Package == "flag" && call.API.Name == "String" && len(call.Values) > 0 {
				options[sourcevalue.Anchor{Path: place.Path, Line: call.Line, Column: call.Column}] = call.Values[0]
			}
		}
	}
	destinations := reading.NewDestinationReader(graph.Places, reading.DestinationChoices{Arguments: map[string]reading.ArgumentChoice{
		"net/http.Client.Do": {Position: 1}, "net/http.NewRequestWithContext": {Position: 3}, "net/http.NewRequest": {Position: 2},
		"net/http.Request.WithContext": {Receiver: true}, "net/http.Get": {Position: 1},
	}, Options: options})
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
			// A helper's `return "", err` is its failure: the address
			// walks only its other returns.
			if place.Symbol.Decl.Name == "DestinationThroughAFailingHelper" && call.API != nil && call.API.Package == "net/http" && call.API.Name == "Get" {
				seen["failing_helper"] = true
				uses := destinations.Read(place, call)
				if len(uses) != 1 || uses[0].Address != "https://versioned.example/v1/items" {
					t.Errorf("a helper's failing return reached the address: %+v", uses)
				}
			}
			for _, argument := range call.SourceArguments {
				visit(argument.Origin)
			}
		}
	}
	for _, want := range []string{"literal", "parameter", "call_result", "concat", "suffix", "captured_owner", "transport_identity", "failing_helper"} {
		if !seen[want] {
			t.Errorf("source value %s did not survive Go adapter and places", want)
		}
	}
}
