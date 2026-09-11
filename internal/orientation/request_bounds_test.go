package orientation

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/groupindex"
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
