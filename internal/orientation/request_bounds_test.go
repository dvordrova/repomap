package orientation

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/llm"
)

func TestOrientationListsAtMostTheAdvertisedMembersPerGroup(t *testing.T) {
	fixture := newFixture(t)
	index := &fixture.input.Groups[0]
	group := &index.Groups[0]
	group.MemberSubjectIDs = nil
	total := MaxAdvertisedGroupMembers + 20
	for i := 0; i < total; i++ {
		id := fmt.Sprintf("local-member-%d", i)
		index.Subjects = append(index.Subjects, groupindex.Subject{ID: id, Object: &groupindex.ObjectFacts{Name: fmt.Sprintf("Handler%d", i)}})
		group.MemberSubjectIDs = append(group.MemberSubjectIDs, id)
		fixture.input.Graph.Places = append(fixture.input.Graph.Places, atlas.Place{ID: "place-" + id, Kind: atlas.PlaceSymbol, Path: "alpha/handlers.go", LineNo: i + 1,
			Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{ObjectID: id, Name: fmt.Sprintf("Handler%d", i)}, Calls: []atlas.SymbolCall{{Name: "Work", Line: i + 1, Column: 2}}}})
	}
	wire, cat, err := buildRequest(fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	listed := wire.Groups[0]
	if len(listed.Members) != MaxAdvertisedGroupMembers || listed.MemberCount != total || listed.Members[0].Name != "Handler0" {
		t.Fatalf("members were not bounded in group order with the real count: %d of %d", len(listed.Members), listed.MemberCount)
	}
	for _, evidence := range wire.MemberEvidence {
		entry, known := cat.subjects[evidence.Ref]
		if !known {
			t.Fatalf("evidence for an uncitable member: %s", evidence.Ref)
		}
		if entry.id == fmt.Sprintf("local-member-%d", total-1) {
			t.Fatal("an unlisted member bought evidence")
		}
	}
}

func TestOrientationBoundsOneMemberObservationLists(t *testing.T) {
	fixture := newFixture(t)
	subject := fixture.subjectID("alpha", "inbound")
	var calls []atlas.SymbolCall
	for i := 0; i < MaxEvidenceCalls+3; i++ {
		calls = append(calls, atlas.SymbolCall{Name: fmt.Sprintf("Step%d", i), Kind: "calls", Line: i + 1, Column: 2})
	}
	var callers []atlas.SymbolCaller
	for i := 0; i < MaxEvidenceCallers+2; i++ {
		callers = append(callers, atlas.SymbolCaller{Path: "alpha/callers.go", Line: i + 1})
	}
	fixture.input.Graph = atlas.Graph{Places: []atlas.Place{{ID: "local-place-main", Kind: atlas.PlaceSymbol, Path: "alpha/main.go", LineNo: 1,
		Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{ObjectID: subject, Name: "Serve"}, Calls: calls, CalledBy: callers}}}}
	wire, _, err := buildRequest(fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	if len(wire.MemberEvidence) != 1 {
		t.Fatalf("member evidence count = %d", len(wire.MemberEvidence))
	}
	raw, _ := json.Marshal(wire.MemberEvidence[0].Evidence)
	var shape struct {
		Calls           []json.RawMessage `json:"calls"`
		CallsOmitted    int               `json:"calls_omitted"`
		CalledBy        []json.RawMessage `json:"called_by"`
		CalledByOmitted int               `json:"called_by_omitted"`
	}
	if err := json.Unmarshal(raw, &shape); err != nil {
		t.Fatal(err)
	}
	if len(shape.Calls) != MaxEvidenceCalls || shape.CallsOmitted != 3 || len(shape.CalledBy) != MaxEvidenceCallers || shape.CalledByOmitted != 2 {
		t.Fatalf("observation lists were not bounded with honest counts: %s", raw)
	}
}

func TestRunWalksThePackingLadderAfterASizeRefusal(t *testing.T) {
	fixture := newFixture(t)
	// Every listed member gets eight observed calls: the first rung keeps six
	// of them, the second three, the last none, so the rungs shrink strictly
	// while the group index itself stays untouched and valid.
	for _, index := range fixture.input.Groups {
		for _, group := range index.Groups {
			for _, id := range group.MemberSubjectIDs {
				var calls []atlas.SymbolCall
				for c := 0; c < 8; c++ {
					calls = append(calls, atlas.SymbolCall{Name: fmt.Sprintf("Step%d", c), Kind: "calls", Line: c + 1, Column: 2, Invocation: "synchronous", Resolution: "exact"})
				}
				fixture.input.Graph.Places = append(fixture.input.Graph.Places, atlas.Place{ID: "place-" + id, Kind: atlas.PlaceSymbol, Path: "alpha/members.go", LineNo: len(fixture.input.Graph.Places) + 1,
					Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{ObjectID: id, Name: "Member"}, Calls: calls}})
			}
		}
	}
	var err error
	if fixture.input.Facts, err = facts.Seal(fixture.input.Facts); err != nil {
		t.Fatal(err)
	}
	sizes := make([]int, len(packingLadder))
	for i, bounds := range packingLadder {
		wire, _, err := encodeRequest(fixture.input, bounds)
		if err != nil {
			t.Fatal(err)
		}
		sizes[i] = len(wire)
	}
	if !(sizes[0] > sizes[1] && sizes[1] > sizes[2]) {
		t.Fatalf("packing rungs are not strictly smaller: %v", sizes)
	}
	// The provider holds only the smallest rung: the first two are refused
	// by size before any transport attempt and the third is answered.
	bounded := &presetProvider{maximumUserBytes: sizes[2], respond: func([]byte) []byte { return []byte(`{}`) }}
	result, rejected, err := Run(t.Context(), llm.Executor{}, bounded, fixture.input)
	if err != nil || bounded.completions != 1 || len(rejected) != 0 || result.RejectedCount != 0 || len(bounded.users[0]) != sizes[2] {
		t.Fatalf("the tighter packing did not reach the provider: %v, rejected=%+v calls=%d sent=%d", err, rejected, bounded.completions, len(bounded.users))
	}
}
