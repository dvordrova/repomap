package orientation

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

func TestOrientationKeepsLateGroupMemberAndItsWorkerEvidence(t *testing.T) {
	fixture := newFixture(t)
	index := &fixture.input.Groups[0]
	var group *groupindex.Group
	for position := range index.Groups {
		if index.Groups[position].Title == "Execution triggers" {
			group = &index.Groups[position]
		}
	}
	if group == nil {
		t.Fatal("trigger group not found")
	}
	group.MemberSubjectIDs = nil
	// Members are listed in subject order, as a sealed index stores them;
	// zero padding keeps the late worker last.
	for i := 0; i < 25; i++ {
		id := fmt.Sprintf("local-member-%02d", i)
		name := fmt.Sprintf("Handler%d", i)
		if i == 24 {
			name = "ConsumeNotifications"
		}
		index.Subjects = append(index.Subjects, groupindex.Subject{ID: id, Object: &groupindex.ObjectFacts{Name: name}})
		group.MemberSubjectIDs = append(group.MemberSubjectIDs, id)
		fixture.input.Graph.Places = append(fixture.input.Graph.Places, atlas.Place{ID: "place-" + id, Kind: atlas.PlaceSymbol,
			Path: "alpha/workers.go", LineNo: i + 1,
			Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{ObjectID: id, Name: name}, Calls: []atlas.SymbolCall{{Name: "ReadQueue", Kind: "invokes_external", Line: i + 1, Column: 2}}}})
	}
	wire, cat, err := buildRequest(fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	listed := groupWire{}
	for _, candidate := range wire.Groups {
		if candidate.MemberCount == 25 {
			listed = candidate
		}
	}
	if len(listed.Members) != 25 || listed.MemberCount != 25 {
		t.Fatalf("incomplete group: %#v", listed)
	}
	last := listed.Members[24]
	if last.Name != "ConsumeNotifications" || cat.subjects[last.Ref].id != "local-member-24" {
		t.Fatalf("late worker cannot be cited: %#v", last)
	}
	for _, evidence := range wire.MemberEvidence {
		if evidence.Ref == last.Ref {
			raw, _ := json.Marshal(evidence.Evidence)
			if !strings.Contains(string(raw), "ReadQueue") {
				t.Fatalf("late worker lost its original calls: %s", raw)
			}
			return
		}
	}
	t.Fatal("late worker has no source evidence")
}

func TestOrientationUsesOriginalMemberCallsWithoutImportingNeighbourBehavior(t *testing.T) {
	fixture := newFixture(t)
	programTargetID := fixture.input.Groups[0].Target.ID
	main := atlas.Place{ID: "local-place-main", Kind: atlas.PlaceSymbol, Path: "alpha/main.go", LineNo: 1, TargetIDs: []string{programTargetID},
		Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{ObjectID: fixture.subjectID("alpha", "inbound"), Name: "Serve", Signature: "func Serve()"}, Calls: []atlas.SymbolCall{
			{Name: "Apply", Kind: "calls", Line: 5, Column: 9, Resolution: "alternatives", CalleeIDs: []string{"local-place-core"},
				SourceArguments: []atlas.SourceArgument{{Position: 1, Origin: &sourcevalue.Value{Kind: "field", Text: "issueTrackerClient", Anchor: &sourcevalue.Anchor{Path: "alpha/main.go", Line: 5, Column: 15}}}}},
			{Name: "Apply", Kind: "calls", Line: 5, Column: 35, Invocation: "goroutine", Resolution: "exact", CalleeIDs: []string{"local-place-core"}},
		}}}
	core := atlas.Place{ID: "local-place-core", Kind: atlas.PlaceSymbol, Path: "alpha/core.go", LineNo: 8, TargetIDs: []string{programTargetID},
		Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{ObjectID: fixture.subjectID("alpha", "core"), Name: "Apply", Signature: "func Apply()"}}}
	unselected := atlas.Place{ID: "local-place-unselected", Kind: atlas.PlaceSymbol, Path: "alpha/other.go", LineNo: 2, TargetIDs: []string{programTargetID},
		Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{ObjectID: "local-object-unselected", Name: "Unrelated"}, Calls: []atlas.SymbolCall{{Name: "unselected-neighbour-exchange"}}}}
	fixture.input.Graph = atlas.Graph{Places: []atlas.Place{main, core, unselected}}
	wire, catalog, err := buildRequest(fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	if len(wire.MemberEvidence) != 2 {
		t.Fatalf("member evidence count = %d", len(wire.MemberEvidence))
	}
	for _, row := range wire.MemberEvidence {
		if _, ok := catalog.subjects[row.Ref]; !ok {
			t.Fatalf("uncitable member evidence: %s", row.Ref)
		}
	}
	raw, _ := json.Marshal(wire.MemberEvidence)
	for _, want := range []string{"issueTrackerClient", `"column":9`, `"column":35`, `"resolution":"alternatives"`, `"invocation":"goroutine"`, `"callee_candidates"`, "alpha/core.go"} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("original observation %q missing: %s", want, raw)
		}
	}
	for _, forbidden := range append(fixture.canonicalIDs(), "local-place-", "local-object-", "unselected-neighbour-exchange", "callee_ids") {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("unadvertised identity/neighbor leaked: %s", forbidden)
		}
	}
}
