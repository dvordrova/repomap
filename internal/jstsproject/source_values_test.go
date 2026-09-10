package jstsproject

import (
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

func TestNestedProjectRebasesCompleteSourceValueChain(t *testing.T) {
	anchor := sourcevalue.Anchor{Path: "src/destinations.ts", Line: 7, Column: 3}
	origin := &sourcevalue.Value{Kind: "field", Text: "url", Anchor: &anchor, Parts: []sourcevalue.Value{{Kind: "receiver", Owner: &anchor}},
		Initializer: &sourcevalue.Value{Kind: "field_value", Text: "url", Anchor: &anchor, Parts: []sourcevalue.Value{{Kind: "parameter", Text: "url", Position: 1, Owner: &anchor}}}}
	original := sourcevalue.Clone(origin)
	output := helperOutput{Calls: []Call{{Location: Location{Path: anchor.Path, Line: anchor.Line, Column: anchor.Column}, Pattern: &CallPattern{
		ReceiverValue: origin, ResultValue: origin, Arguments: []CallPatternArgument{{Position: 1, Origin: origin}},
	}}}}
	rebaseHelperOutput("front", &output)
	if !reflect.DeepEqual(origin, original) {
		t.Fatal("rebasing mutated a shared source value")
	}
	var check func(*sourcevalue.Value)
	check = func(value *sourcevalue.Value) {
		if value == nil {
			return
		}
		for _, got := range []*sourcevalue.Anchor{value.Anchor, value.Owner} {
			if got != nil && (got.Path != "front/src/destinations.ts" || got.Line != 7 || got.Column != 3) {
				t.Fatalf("source chain no longer addresses its repository call/declaration: %+v", got)
			}
		}
		check(value.Initializer)
		for i := range value.Parts {
			check(&value.Parts[i])
		}
	}
	call := output.Calls[0]
	if call.Location.Path != "front/src/destinations.ts" {
		t.Fatalf("call location: %+v", call.Location)
	}
	check(call.Pattern.ReceiverValue)
	check(call.Pattern.ResultValue)
	check(call.Pattern.Arguments[0].Origin)
}

func assertCumulativeJSTSSourceValues(t *testing.T, repository *corpus.Corpus, index programindex.Index) {
	t.Helper()
	const path = "src/destinations.ts"
	seen := map[string]bool{}
	owners := map[sourcevalue.Anchor]string{}
	objects := map[string]programindex.Object{}
	callSites := map[sourcevalue.Anchor]bool{}
	for _, object := range index.Objects {
		objects[object.ID] = object
		if object.Location != nil {
			owners[sourcevalue.Anchor{Path: object.Location.Path, Line: object.Location.Line, Column: object.Location.Column}] = object.Name
		}
	}
	for _, relation := range index.Relations {
		for _, pattern := range relation.Patterns {
			if pattern.Location != nil {
				callSites[sourcevalue.Anchor{Path: pattern.Location.Path, Line: pattern.Location.Line, Column: pattern.Location.Column}] = true
			}
		}
	}
	var visit func(*sourcevalue.Value, string)
	visit = func(value *sourcevalue.Value, caller string) {
		if value == nil {
			return
		}
		if err := sourcevalue.Validate(value); err != nil {
			t.Fatal(err)
		}
		seen[value.Kind] = true
		if value.Initializer != nil {
			seen["field_initializer"] = true
			if strings.Contains(caller, "MutableAdapter") && value.Text == "url" {
				if value.Initializer.Kind != "field_value" || value.Initializer.Parts[0].Kind != "unknown" {
					t.Fatalf("later field write kept initial address: %+v", value)
				}
				seen["mutable_field_unknown"] = true
			}
			visit(value.Initializer, caller)
		}
		if value.Kind == "parameter" {
			if owners[*value.Owner] == "" {
				t.Fatalf("parameter owner is not native: %+v", value)
			}
			if caller == "capturedExchange.refresh" && value.Text == "base" {
				if owners[*value.Owner] != "capturedExchange" {
					t.Fatalf("captured parameter changed owner: %+v", value)
				}
				seen["capture"] = true
			}
		}
		if value.Kind == "call_result" && !callSites[*value.Anchor] {
			t.Fatalf("producer has no native argument row: %+v", value)
		}
		if value.Kind == "literal" && value.Text == "https://시세.example/prices%20latest" {
			seen["raw_unicode_url"] = true
		}
		if value.Kind == "literal" && value.Text == "/시세" {
			seen["suffix"] = true
		}
		for i := range value.Parts {
			visit(&value.Parts[i], caller)
		}
	}
	for _, relation := range index.Relations {
		caller, ok := objects[relation.FromID]
		if !ok || caller.Location == nil || caller.Location.Path != path {
			continue
		}
		for _, pattern := range relation.Patterns {
			visit(pattern.ReceiverValue, caller.Name)
			visit(pattern.ResultValue, caller.Name)
			for _, argument := range pattern.Arguments {
				if caller.Name == "reassignedAddress" && pattern.Selector == "dispatch" {
					if argument.Origin == nil || argument.Origin.Kind != "unknown" {
						t.Fatalf("mutation got an exact address: %+v", argument.Origin)
					}
					seen["mutation_unknown"] = true
				}
				visit(argument.Origin, caller.Name)
			}
		}
	}
	for _, name := range []string{"parameter", "literal", "concat", "call_result", "field", "index", "alternatives", "capture", "suffix", "raw_unicode_url", "mutation_unknown", "receiver", "record", "field_value", "field_initializer", "mutable_field_unknown"} {
		if !seen[name] {
			t.Errorf("source observation missing: %s", name)
		}
	}
	// The ordinary places projection must preserve the same anchored source
	// expression, not only its former dynamic/literal classification.
	graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: index}}})
	if err != nil {
		t.Fatal(err)
	}
	reader := reading.NewDestinationReader(graph.Places)
	var direct, config, constructor, projected bool
	for _, place := range graph.Places {
		if place.Symbol == nil || place.Path != path {
			continue
		}
		for _, call := range place.Symbol.Calls {
			if len(call.SourceArguments) > 0 {
				projected = true
			}
			if call.API == nil || call.API.Package != javascriptPlatform || call.API.Name != "fetch" {
				continue
			}
			uses := reader.Read(place, call)
			switch place.Symbol.Decl.Name {
			case "directPriceFeed":
				direct = len(uses) == 1 && uses[0].Address == "https://시세.example/prices%20latest" && uses[0].Frontier == ""
			case "dispatch":
				for _, use := range uses {
					config = config || strings.Contains(use.Frontier, "notifications") && strings.Contains(use.Frontier, "url")
				}
			case "LiteralAdapter.refresh":
				constructor = len(uses) == 1 && uses[0].Address == "https://exchange.example/candles" && uses[0].Frontier == "" && len(uses[0].Steps) >= 2
				if !constructor {
					t.Errorf("native JSTS constructor chain: %+v", uses)
				}
			}
		}
	}
	if !projected || !direct || !config || !constructor {
		t.Fatalf("native destination reader: projected=%v direct=%v config=%v constructor=%v", projected, direct, config, constructor)
	}
}
