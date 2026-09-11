package orientation

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// Member evidence carries origins in their request form. Morfeu's request
// spent 90 KB of 280 KB on source_arguments, receiver_value and result_value
// trees up to nine levels deep, an anchor and owner on every node, and the
// answer used five nodes of 118. A synthetic member with one such call must
// reach the wire with the first three levels, no anchors, and well under
// half the bytes of the graph's complete form.
func TestOrientationMemberEvidenceCarriesCompactOrigins(t *testing.T) {
	fixture := newFixture(t)
	anchor := func(line int) *sourcevalue.Anchor {
		return &sourcevalue.Anchor{Path: "alpha/main.go", Line: line, Column: 7}
	}
	origin := &sourcevalue.Value{Kind: "literal", Text: "level9", Anchor: anchor(9)}
	for level := 8; level >= 0; level-- {
		origin = &sourcevalue.Value{Kind: "field", Text: fmt.Sprintf("level%d", level), Anchor: anchor(level + 10), Owner: anchor(1), Parts: []sourcevalue.Value{*origin}}
	}
	call := atlas.SymbolCall{Name: "Apply", Kind: "calls", Line: 5, Column: 9, Invocation: "synchronous", Resolution: "exact",
		ReceiverValue:   &sourcevalue.Value{Kind: "parameter", Text: "client", Position: 1, Owner: anchor(1)},
		SourceArguments: []atlas.SourceArgument{{Position: 1, Origin: origin}},
		ResultValue:     &sourcevalue.Value{Kind: "call_result", Anchor: anchor(5)}}
	main := atlas.Place{ID: "local-place-main", Kind: atlas.PlaceSymbol, Path: "alpha/main.go", LineNo: 1,
		Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{ObjectID: fixture.subjectID("alpha", "inbound"), Name: "Serve", Signature: "func Serve()"}, Calls: []atlas.SymbolCall{call}}}
	fixture.input.Graph = atlas.Graph{Places: []atlas.Place{main}}
	full, err := json.Marshal(main.Symbol.Calls)
	if err != nil {
		t.Fatal(err)
	}
	wire, _, err := buildRequest(fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	if len(wire.MemberEvidence) != 1 {
		t.Fatalf("member evidence count = %d", len(wire.MemberEvidence))
	}
	raw, err := json.Marshal(wire.MemberEvidence[0].Evidence)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"text":"level0"`, `"text":"level1"`, `"text":"level2"`, `"receiver_value":{"kind":"parameter","text":"client"}`, `"result_value":{"kind":"call_result"}`, `"column":9`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("member evidence lost %s: %s", want, raw)
		}
	}
	for _, forbidden := range []string{`"anchor"`, `"owner"`, "level3", "level9"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("member evidence carries %s: %s", forbidden, raw)
		}
	}
	if len(raw) >= len(full)/2 {
		t.Fatalf("member evidence is %d bytes against %d for the graph form", len(raw), len(full))
	}
	if after, _ := json.Marshal(main.Symbol.Calls); string(after) != string(full) {
		t.Fatal("building the request changed the graph's complete origins")
	}
}
