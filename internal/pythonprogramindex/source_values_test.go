package pythonprogramindex

import (
	"context"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/pythontarget"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

func TestCumulativePythonSourceValuesRetainSharedHelperAndCapturedOwner(t *testing.T) {
	repository := pythonCorpus(t, cumulativePythonSources(t, "src/fixture_app/destinations.py"))
	index, err := buildOneForTest(context.Background(), repository, targetOfKind(t, repository, pythontarget.KindLibrary))
	if err != nil {
		t.Fatal(err)
	}
	graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: index}}})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	owners := map[sourcevalue.Anchor]string{}
	callSites := map[sourcevalue.Anchor]bool{}
	for _, place := range graph.Places {
		if place.Symbol == nil {
			continue
		}
		owners[sourcevalue.Anchor{Path: place.Path, Line: place.LineNo, Column: place.Symbol.Decl.Column}] = place.Symbol.Decl.Name
		for _, call := range place.Symbol.Calls {
			if call.Line > 0 {
				callSites[sourcevalue.Anchor{Path: place.Path, Line: call.Line, Column: call.Column}] = true
			}
		}
	}
	var visit func(atlas.Place, *sourcevalue.Value)
	visit = func(place atlas.Place, value *sourcevalue.Value) {
		if value == nil {
			return
		}
		if err := sourcevalue.Validate(value); err != nil {
			t.Fatal(err)
		}
		seen[value.Kind] = true
		if value.Initializer != nil {
			seen["field_initializer"] = true
			if strings.Contains(place.Symbol.Decl.Name, "MutableAdapter") && value.Text == "url" {
				if value.Initializer.Kind != "field_value" || value.Initializer.Parts[0].Kind != "unknown" {
					t.Fatalf("later field write kept initial address: %+v", value)
				}
				seen["mutable_field_unknown"] = true
			}
			visit(place, value.Initializer)
		}
		if value.Kind == "parameter" {
			if owners[*value.Owner] == "" {
				t.Fatalf("parameter owner is not a native declaration: %+v", value)
			}
			if place.Symbol.Decl.Name == "refresh" && value.Text == "base" {
				if owners[*value.Owner] != "captured_exchange" {
					t.Fatalf("capture changed owner: %+v", value)
				}
				seen["capture"] = true
			}
		}
		if value.Kind == "call_result" && !callSites[*value.Anchor] {
			t.Fatalf("producer has no original argument row: %+v", value)
		}
		if value.Kind == "literal" && value.Text == "https://시세.example/prices%20latest" {
			seen["raw_unicode_url"] = true
		}
		if value.Kind == "literal" && value.Text == "/시세" {
			seen["suffix"] = true
		}
		for i := range value.Parts {
			visit(place, &value.Parts[i])
		}
	}
	for _, place := range graph.Places {
		if place.Symbol == nil {
			continue
		}
		for _, call := range place.Symbol.Calls {
			visit(place, call.ReceiverValue)
			visit(place, call.ResultValue)
			for _, argument := range call.SourceArguments {
				if place.Symbol.Decl.Name == "reassigned_address" && argument.Position == 2 && call.Name == "dispatch" {
					if argument.Origin.Kind != "unknown" {
						t.Fatalf("mutable value got a final address: %+v", argument.Origin)
					}
					seen["mutation_unknown"] = true
				}
				visit(place, argument.Origin)
			}
		}
	}
	for _, name := range []string{"parameter", "literal", "concat", "call_result", "field", "index", "alternatives", "capture", "suffix", "raw_unicode_url", "mutation_unknown", "receiver", "record", "field_value", "field_initializer", "mutable_field_unknown"} {
		if !seen[name] {
			t.Errorf("source observation missing: %s", name)
		}
	}
	reader := reading.NewDestinationReader(graph.Places)
	var direct, config, constructor bool
	for _, place := range graph.Places {
		if place.Symbol == nil {
			continue
		}
		for _, call := range place.Symbol.Calls {
			if call.API == nil || call.API.Package != "requests" || call.API.Name != "get" {
				continue
			}
			uses := reader.Read(place, call)
			switch place.Symbol.Decl.Name {
			case "direct_price_feed":
				direct = len(uses) == 1 && uses[0].Address == "https://시세.example/prices%20latest" && uses[0].Frontier == ""
			case "dispatch":
				for _, use := range uses {
					config = config || strings.Contains(use.Frontier, "notifications") && strings.Contains(use.Frontier, "url")
				}
			case "LiteralAdapter.refresh":
				constructor = len(uses) == 1 && uses[0].Address == "https://exchange.example/candles" && uses[0].Frontier == "" && len(uses[0].Steps) >= 2
				if !constructor {
					t.Errorf("native Python constructor chain: %+v", uses)
				}
			}
		}
	}
	if !direct || !config || !constructor {
		t.Fatalf("native destination reader: direct=%v config=%v constructor=%v", direct, config, constructor)
	}
}
