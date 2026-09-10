package reading

import (
	"reflect"
	"sort"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

func TestDestinationChainsKeepCallArgumentsTogether(t *testing.T) {
	anchor := &sourcevalue.Anchor{Path: "helper.go", Line: 10, Column: 1}
	parameter := func(n int) sourcevalue.Value { return sourcevalue.Value{Kind: "parameter", Position: n, Owner: anchor} }
	literal := func(text string) *sourcevalue.Value { return &sourcevalue.Value{Kind: "literal", Text: text} }
	helper := atlas.Place{ID: "helper", Path: "helper.go", LineNo: 10, TargetIDs: []string{"app"}, Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: "Send", Column: 1}}}
	call := atlas.SymbolCall{Name: "http.Get", Line: 11, Column: 5, API: &atlas.CallAPI{Package: "net/http", Name: "Get"}, SourceArguments: []atlas.SourceArgument{{Position: 1, Origin: &sourcevalue.Value{Kind: "concat", Parts: []sourcevalue.Value{parameter(1), parameter(2)}}}}}
	helper.Symbol.Calls = []atlas.SymbolCall{call}
	app := atlas.Place{ID: "app", Path: "main.go", LineNo: 1, TargetIDs: []string{"app"}, Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: "main"}}}
	app.Symbol.Calls = []atlas.SymbolCall{
		{Name: "Send", Line: 2, Column: 3, CalleeIDs: []string{"helper"}, SourceArguments: []atlas.SourceArgument{{Position: 1, Origin: literal("https://first.example")}, {Position: 2, Origin: literal("/a")}}},
		{Name: "Send", Line: 3, Column: 3, CalleeIDs: []string{"helper"}, SourceArguments: []atlas.SourceArgument{{Position: 1, Origin: literal("https://second.example")}, {Position: 2, Origin: literal("/b")}}},
	}
	sibling := atlas.Place{ID: "other", Path: "other.go", LineNo: 1, TargetIDs: []string{"other"}, Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: "Other"}, Calls: []atlas.SymbolCall{{Name: "Send", Line: 2, Column: 3, CalleeIDs: []string{"helper"}, SourceArguments: []atlas.SourceArgument{{Position: 1, Origin: literal("https://unrelated.example")}, {Position: 2, Origin: literal("/c")}}}}}}
	uses := NewDestinationReader([]atlas.Place{helper, app, sibling}).Read(helper, call)
	var addresses []string
	for _, use := range uses {
		addresses = append(addresses, use.Address)
		if len(use.Steps) != 2 {
			t.Fatalf("chain lost source sites: %+v", use)
		}
	}
	sort.Strings(addresses)
	if !reflect.DeepEqual(addresses, []string{"https://first.example/a", "https://second.example/b"}) {
		t.Fatalf("unobserved cross-product or other target: %v", addresses)
	}
}

func TestDestinationStopsAtUnknownFactoryInsteadOfSelectingNearbyLiteral(t *testing.T) {
	place := atlas.Place{ID: "sender", Path: "pipeline.go", LineNo: 10, TargetIDs: []string{"server"}, Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: "pipeline.post"}}}
	call := atlas.SymbolCall{Name: "http.RoundTripper.RoundTrip", Line: 30, Column: 5, API: &atlas.CallAPI{Package: "net/http", Name: "RoundTrip"}, SourceArguments: []atlas.SourceArgument{{Position: 1, Origin: &sourcevalue.Value{Kind: "call_result", Anchor: &sourcevalue.Anchor{Path: "pipeline.go", Line: 20, Column: 5}}}}}
	place.Symbol.Calls = []atlas.SymbolCall{call, {Name: "picker.pick", Line: 20, Column: 5}, {Name: "log.Printf", Line: 19, Column: 5, Values: []string{"https://unrelated.example"}}}
	uses := NewDestinationReader([]atlas.Place{place}).Read(place, call)
	if len(uses) != 1 || uses[0].Address != "" || uses[0].Frontier != "picker.pick()" || len(uses[0].Steps) != 2 || uses[0].Steps[1].Line != 20 {
		t.Fatalf("frontier changed into guessed destination: %+v", uses)
	}
}

func TestDestinationConstructorBindsItsOwnKeywordArguments(t *testing.T) {
	formal := &sourcevalue.Anchor{Path: "adapter.py", Line: 3, Column: 5}
	parameter := func(name string, position int) sourcevalue.Value {
		return sourcevalue.Value{Kind: "parameter", Text: name, Position: position, Owner: formal}
	}
	result := &sourcevalue.Value{Kind: "record", Owner: formal, Parts: []sourcevalue.Value{{Kind: "field_value", Text: "endpoint", Parts: []sourcevalue.Value{{Kind: "concat", Parts: []sourcevalue.Value{parameter("base", 1), parameter("suffix", 2)}}}}}}
	app := atlas.Place{ID: "app", Path: "main.py", LineNo: 1, TargetIDs: []string{"app"}, Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: "main"}}}
	for i, values := range [][2]string{{"https://first.example", "/a"}, {"https://second.example", "/b"}} {
		app.Symbol.Calls = append(app.Symbol.Calls, atlas.SymbolCall{Name: "Adapter", Line: 2 + i, Column: 5, CalleeIDs: []string{"native-class"}, ResultValue: result, SourceArguments: []atlas.SourceArgument{
			{Keyword: "base", Origin: &sourcevalue.Value{Kind: "literal", Text: values[0]}},
			{Keyword: "suffix", Origin: &sourcevalue.Value{Kind: "literal", Text: values[1]}},
		}})
	}
	call := atlas.SymbolCall{Name: "requests.get", Line: 5, Column: 5, API: &atlas.CallAPI{Package: "requests.api", Name: "get"}, SourceArguments: []atlas.SourceArgument{{Keyword: "url", Origin: &sourcevalue.Value{Kind: "field", Text: "endpoint", Parts: []sourcevalue.Value{{Kind: "call_result", Anchor: &sourcevalue.Anchor{Path: "main.py", Line: 2, Column: 5}}}}}}}
	app.Symbol.Calls = append(app.Symbol.Calls, call)
	uses := NewDestinationReader([]atlas.Place{app}).Read(app, call)
	if len(uses) != 1 || uses[0].Address != "https://first.example/a" {
		t.Fatalf("constructor lost formal ownership or mixed independent instances: %+v", uses)
	}
}

func TestDestinationAlternativeFieldsKeepTheirSourcePair(t *testing.T) {
	record := func(base, path string) sourcevalue.Value {
		return sourcevalue.Value{Kind: "record", Parts: []sourcevalue.Value{
			{Kind: "field_value", Text: "base", Parts: []sourcevalue.Value{{Kind: "literal", Text: base}}},
			{Kind: "field_value", Text: "path", Parts: []sourcevalue.Value{{Kind: "literal", Text: path}}},
		}}
	}
	choice := sourcevalue.Value{Kind: "alternatives", Anchor: &sourcevalue.Anchor{Path: "app.ts", Line: 3, Column: 1}, Parts: []sourcevalue.Value{record("https://first.example", "/a"), record("https://second.example", "/b")}}
	url := &sourcevalue.Value{Kind: "concat", Parts: []sourcevalue.Value{{Kind: "field", Text: "base", Parts: []sourcevalue.Value{choice}}, {Kind: "field", Text: "path", Parts: []sourcevalue.Value{choice}}}}
	call := atlas.SymbolCall{Name: "fetch", Line: 5, Column: 1, API: &atlas.CallAPI{Package: "platform:javascript", Name: "fetch"}, SourceArguments: []atlas.SourceArgument{{Position: 1, Origin: url}}}
	p := atlas.Place{ID: "main", Path: "app.ts", LineNo: 1, TargetIDs: []string{"app"}, Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: "main"}, Calls: []atlas.SymbolCall{call}}}
	uses := NewDestinationReader([]atlas.Place{p}).Read(p, call)
	if len(uses) != 2 {
		t.Fatalf("lost source alternatives: %+v", uses)
	}
	for _, use := range uses {
		if use.Address == "https://first.example/b" || use.Address == "https://second.example/a" {
			t.Fatalf("unobserved address: %+v", uses)
		}
	}
}
