package orientation

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/groupindex"
)

// Refs are numbered in an order the graph fixes, not in the order a caller
// listed indexes, groups, members or connections.
func TestRequestBytesDoNotDependOnGroupMemberOrConnectionOrder(t *testing.T) {
	fixture := newFixture(t)
	subject := fixture.subjectID("alpha", "inbound")
	fixture.input.Graph = atlas.Graph{Places: []atlas.Place{{ID: "local-place-main", Kind: atlas.PlaceSymbol, Path: "alpha/main.go", LineNo: 1,
		Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{ObjectID: subject, Name: "Serve"}, Calls: []atlas.SymbolCall{{Name: "Apply", Kind: "calls", Line: 2, Column: 2}}}}}}
	canonical, _, err := encodeRequest(fixture.input, packingLadder[0])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(canonical, []byte(`"member_evidence"`)) {
		t.Fatal("fixture carries no member evidence to order")
	}
	shuffled := fixture.input
	shuffled.Groups = append([]groupindex.Index(nil), fixture.input.Groups...)
	slices.Reverse(shuffled.Groups)
	for i := range shuffled.Groups {
		index := &shuffled.Groups[i]
		index.Groups = append([]groupindex.Group(nil), index.Groups...)
		slices.Reverse(index.Groups)
		for g := range index.Groups {
			members := append([]string(nil), index.Groups[g].MemberSubjectIDs...)
			slices.Reverse(members)
			index.Groups[g].MemberSubjectIDs = members
		}
		index.Connections = append([]groupindex.Connection(nil), index.Connections...)
		slices.Reverse(index.Connections)
	}
	reordered, _, err := encodeRequest(shuffled, packingLadder[0])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(canonical, reordered) {
		t.Fatalf("request bytes follow the caller's order:\n%s\n%s", canonical, reordered)
	}
}

// Morfeu builds 20260911-110335 and 20260911-152759 differ only in the lane
// of "Catalog feature"; the index orders groups by an ID that hashes the
// lane, so g5..g7 named other groups and the same cited members carried
// other s* refs. A lane change may touch its own group's fields only.
func TestGroupLaneChangeKeepsEveryOtherRefInPlace(t *testing.T) {
	fixture := newFixture(t)
	before := decodeRequest(t, fixture.input)
	moved := fixture.input
	moved.Groups = append([]groupindex.Index(nil), fixture.input.Groups...)
	// Both fixture targets hold a group of this title; each is relaned and,
	// as a re-hashed group would, carries a new identity into its
	// connections.
	const title = "Execution triggers"
	relaned := make(map[groupindex.Endpoint]string)
	for i := range moved.Groups {
		index := &moved.Groups[i]
		index.Groups = append([]groupindex.Group(nil), index.Groups...)
		for g := range index.Groups {
			group := &index.Groups[g]
			if group.Title != title {
				continue
			}
			renamed := "program-group-relaned-" + group.ID
			relaned[groupindex.Endpoint{TargetID: index.Target.ID, GroupID: group.ID}] = renamed
			group.Lane, group.ID = groupindex.LaneCore, renamed
		}
	}
	if len(relaned) != len(moved.Groups) {
		t.Fatalf("relaned %d groups across %d indexes", len(relaned), len(moved.Groups))
	}
	for i := range moved.Groups {
		index := &moved.Groups[i]
		index.Connections = append([]groupindex.Connection(nil), index.Connections...)
		for c := range index.Connections {
			connection := &index.Connections[c]
			if renamed, ok := relaned[connection.From]; ok {
				connection.From.GroupID = renamed
			}
			if renamed, ok := relaned[connection.To]; ok {
				connection.To.GroupID = renamed
			}
		}
	}
	after := decodeRequest(t, moved)
	if !reflect.DeepEqual(groupRefsByTitle(before), groupRefsByTitle(after)) {
		t.Fatalf("group refs moved with one lane:\n%v\n%v", groupRefsByTitle(before), groupRefsByTitle(after))
	}
	if !reflect.DeepEqual(memberRefsByLabel(before), memberRefsByLabel(after)) {
		t.Fatalf("member refs moved with one lane:\n%v\n%v", memberRefsByLabel(before), memberRefsByLabel(after))
	}
	changed := 0
	for i := range before.Groups {
		if before.Groups[i].Lane == after.Groups[i].Lane {
			continue
		}
		changed++
		if before.Groups[i].Title != title || after.Groups[i].Lane != string(groupindex.LaneCore) {
			t.Fatalf("another group changed lane: %+v -> %+v", before.Groups[i], after.Groups[i])
		}
	}
	if changed != len(relaned) {
		t.Fatalf("lane changes = %d, want the %d relaned groups", changed, len(relaned))
	}
}

func decodeRequest(t *testing.T, input Input) request {
	t.Helper()
	wire, _, err := encodeRequest(input, packingLadder[0])
	if err != nil {
		t.Fatal(err)
	}
	var decoded request
	if err := json.Unmarshal(wire, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

func groupRefsByTitle(wire request) map[string]string {
	refs := make(map[string]string, len(wire.Groups))
	for _, group := range wire.Groups {
		refs[group.Target+" "+group.Title] = group.Ref
	}
	return refs
}

func memberRefsByLabel(wire request) map[string]string {
	refs := make(map[string]string)
	for _, group := range wire.Groups {
		for _, member := range group.Members {
			refs[group.Target+" "+member.Name+" "+member.Anchor] = member.Ref
		}
	}
	return refs
}
