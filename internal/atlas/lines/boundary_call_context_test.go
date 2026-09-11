package lines

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

func TestBoundaryCallerContextKeepsOnlyNativeCallsToThisOwner(t *testing.T) {
	owner := atlas.Place{ID: "private-owner", Path: "client.go", LineNo: 10, Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{ObjectID: "private-owner-object", Name: "newClient"}}}
	caller := atlas.Place{ID: "private-caller", Path: "main.go", LineNo: 1, Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{ObjectID: "private-caller-object", Name: "main"}}}
	caller.Symbol.Calls = []atlas.SymbolCall{
		{Name: "newClient", Kind: "calls", Line: 7, Column: 3, Resolution: "exact", CalleeIDs: []string{owner.ID},
			SourceArguments: []atlas.SourceArgument{{Position: 1, Origin: &sourcevalue.Value{Kind: "unknown", Text: "pipelineServerURL", Anchor: &sourcevalue.Anchor{Path: "main.go", Line: 7, Column: 13}}}},
			ResultValue:     &sourcevalue.Value{Kind: "alternatives", Text: "result-tree-marker", Parts: []sourcevalue.Value{{Kind: "literal", Text: "result-tree-marker"}}}},
		{Name: "newClient", Kind: "calls", Line: 7, Column: 35, Resolution: "exact", CalleeIDs: []string{owner.ID},
			SourceArguments: []atlas.SourceArgument{{Position: 1, Origin: &sourcevalue.Value{Kind: "unknown", Text: "issueTrackerURL", Anchor: &sourcevalue.Anchor{Path: "main.go", Line: 7, Column: 45}}}}},
		{Name: "unrelatedSameLineCall", Kind: "calls", Line: 7, Column: 70, Resolution: "exact", CalleeIDs: []string{"private-neighbour"}},
	}
	owner.Symbol.CalledBy = []atlas.SymbolCaller{{ObjectID: caller.Symbol.Decl.ObjectID, PlaceID: caller.ID, Name: "main", Path: "main.go", Line: 7, Kind: "calls", Resolution: "exact"}}
	fields := BoundarySourceContext(atlas.Place{}, owner, nil, map[string]atlas.Place{caller.ID: caller})
	raw, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"pipelineServerURL", "issueTrackerURL", `"column":3`, `"column":35`, `"source_arguments"`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("source call context lost %s: %s", want, raw)
		}
	}
	for _, forbidden := range []string{"unrelatedSameLineCall", "private-", "callee_ids", "result_value", "result-tree-marker"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("caller context imported %s", forbidden)
		}
	}
}
