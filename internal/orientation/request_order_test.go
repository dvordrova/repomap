package orientation

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"sort"
	"testing"

	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
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
	canonical := encodeOverview(t, fixture.input)
	canonicalFlow, _ := encodeFlow(t, fixture.input, fixture.targetID("alpha"))
	if !bytes.Contains(canonical, []byte(`"Apply@10`)) || !bytes.Contains(canonicalFlow, []byte(`"Apply@10`)) {
		t.Fatal("fixture carries no member calls to order")
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
	if reordered := encodeOverview(t, shuffled); !bytes.Equal(canonical, reordered) {
		t.Fatalf("request bytes follow the caller's order:\n%s\n%s", canonical, reordered)
	}
	if reordered, _ := encodeFlow(t, shuffled, fixture.targetID("alpha")); !bytes.Equal(canonicalFlow, reordered) {
		t.Fatalf("flow bytes follow the caller's order:\n%s\n%s", canonicalFlow, reordered)
	}
}

// A presentation change must not invent a second request-local identity for
// an already built group.
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

func decodeRequest(t *testing.T, input Input) overviewRequest {
	t.Helper()
	var decoded overviewRequest
	if err := json.Unmarshal(encodeOverview(t, input), &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

func groupRefsByTitle(wire overviewRequest) map[string]string {
	refs := make(map[string]string, len(wire.Groups))
	for _, group := range wire.Groups {
		refs[group.Target+" "+group.Title] = group.Ref
	}
	return refs
}

// A callable a member hands over follows it in the flow scope even when an
// input's reach holds it later, and test code is no member (othello's
// start! handed setup, update-state and draw-state to quil, which stood at
// 192, 194 and 174 of 197 members; its specs were 38 members and 57% of the
// member bytes, their registrations facts of the request).
func TestTheFlowScopePlacesAHandOverAfterItsGiverAndHoldsNoTestCode(t *testing.T) {
	subject := func(id, path string, kind programindex.ObjectKind) groupindex.Subject {
		return groupindex.Subject{ID: id, Kind: groupindex.SubjectObject, Object: &groupindex.ObjectFacts{Name: id, Kind: kind, Location: &programindex.Location{Path: path, Line: 1, Column: 1}}}
	}
	edge := func(from, to string, kind programindex.RelationKind) groupindex.StructuralEdge {
		return groupindex.StructuralEdge{FromSubjectID: from, ToSubjectID: to, Role: groupindex.EdgeRelationTarget, RelationKind: kind, Resolution: programindex.ResolutionExact}
	}
	fn := programindex.ObjectFunction
	index := groupindex.Index{
		Target: programindex.Target{ID: "t1", TestSources: []string{"spec/sketch_spec.clj"}, Seeds: []programindex.TargetSeed{{ObjectID: "main"}}},
		Subjects: []groupindex.Subject{subject("main", "src/core.clj", fn), subject("start", "src/sketch.clj", fn), subject("helper", "src/sketch.clj", fn),
			subject("setup", "src/sketch.clj", fn), subject("update", "src/sketch.clj", fn), subject("tick", "src/events.clj", fn),
			subject("spec", "spec/sketch_spec.clj", programindex.ObjectModule), subject("probe", "spec/sketch_spec.clj", fn)},
		StructuralEdges: []groupindex.StructuralEdge{edge("main", "start", programindex.RelationCalls), edge("main", "helper", programindex.RelationCalls),
			edge("start", "setup", programindex.RelationPassesCallback), edge("start", "update", programindex.RelationPassesCallback),
			edge("update", "tick", programindex.RelationCalls), edge("spec", "start", programindex.RelationCalls)},
		Launch: groupindex.Launch{Functions: []groupindex.LaunchFunction{{SubjectID: "spec"}}},
		Reach:  []groupindex.Reach{{Subjects: []groupindex.ReachedSubject{{SubjectID: "update"}, {SubjectID: "tick"}}}},
	}
	registrations := []facts.Fact{{Kind: facts.KindRegistration, TargetID: "t1", OwnerID: "spec", ObjectID: "probe"}}
	scope := scopeOf(index, registrations)
	if want := []string{"main", "start", "setup", "update", "helper", "tick"}; !slices.Equal(scope.Members, want) {
		t.Fatalf("scope = %v, want %v", scope.Members, want)
	}
}
