package orientation

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"sort"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/groupindex"
)

func TestQualifiedCompactRefsUseNaturalOrder(t *testing.T) {
	refs := []string{"t10.g1", "t2.g10", "t2.g2", "t1.g20", "t1.g3"}
	sort.Slice(refs, func(i, j int) bool { return compactRefLess(refs[i], refs[j]) })
	want := []string{"t1.g3", "t1.g20", "t2.g2", "t2.g10", "t10.g1"}
	if !reflect.DeepEqual(refs, want) {
		t.Fatalf("qualified refs = %v, want %v", refs, want)
	}
}

// Refs are numbered in an order the graph fixes, not in the order a caller
// listed indexes, groups, members or connections.
func TestRequestBytesDoNotDependOnGroupMemberOrConnectionOrder(t *testing.T) {
	fixture := newFixture(t)
	subject := fixture.objectID("alpha", "inbound")
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

// A presentation change must not invent a second request-local identity for
// an already built group or its members.
func TestGroupLaneChangeKeepsEveryOtherRefInPlace(t *testing.T) {
	fixture := newFixture(t)
	before := decodeRequest(t, fixture.input)
	moved := fixture.input
	moved.Groups = append([]groupindex.Index(nil), fixture.input.Groups...)
	// Both fixture targets hold a group of this title; this test changes the
	// presentation field on the already identified group.
	const title = "Execution triggers"
	changedGroups := 0
	for i := range moved.Groups {
		index := &moved.Groups[i]
		index.Groups = append([]groupindex.Group(nil), index.Groups...)
		for g := range index.Groups {
			group := &index.Groups[g]
			if group.Title != title {
				continue
			}
			group.Lane = groupindex.LaneCore
			changedGroups++
		}
	}
	if changedGroups != len(moved.Groups) {
		t.Fatalf("relaned %d groups across %d indexes", changedGroups, len(moved.Groups))
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
	if changed != changedGroups {
		t.Fatalf("lane changes = %d, want %d", changed, changedGroups)
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
