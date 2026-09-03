package programgrouping

import (
	"reflect"
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
)

func consolidationCandidates() proposalSet {
	return proposalSet{
		groups: []groupProposal{
			{Key: "a", Title: "Middleware", Lane: groupindex.LaneCore, MemberSubjectIDs: []string{"s1", "s2"}},
			{Key: "b", Title: "Middleware common", Lane: groupindex.LaneCore, MemberSubjectIDs: []string{"s2", "s3"}},
			{Key: "c", Title: "Router core", Lane: groupindex.LaneCore, MemberSubjectIDs: []string{"s4"}},
			{Key: "d", Title: "HTTP entry", Lane: groupindex.LaneTriggers, MemberSubjectIDs: []string{"s5"}},
		},
		connections: []connectionProposal{
			{FromGroupKey: "d", ToGroupKey: "a", SemanticKind: "dispatches_to", Label: "dispatches"},
			{FromGroupKey: "a", ToGroupKey: "b", SemanticKind: "invokes", Label: "invokes"},
			{FromGroupKey: "d", ToGroupKey: "c", SemanticKind: "dispatches_to", Label: "dispatches"},
		},
	}
}

// A consolidation joins members. Whatever the response says, every member of
// every candidate has to come out the other side.
func TestConsolidationNeverLosesAMember(t *testing.T) {
	t.Parallel()

	candidates := consolidationCandidates()
	for name, response := range map[string]consolidateResponse{
		"joins two": {Groups: []consolidateGroup{
			{Title: "Middleware", Lane: groupindex.LaneCore, CandidateRefs: []string{"c1", "c2"}},
			{Title: "Router core", Lane: groupindex.LaneCore, CandidateRefs: []string{"c3"}},
			{Title: "HTTP entry", Lane: groupindex.LaneTriggers, CandidateRefs: []string{"c4"}},
		}},
		"forgets one": {Groups: []consolidateGroup{
			{Title: "Middleware", Lane: groupindex.LaneCore, CandidateRefs: []string{"c1", "c2"}},
		}},
		"names one twice": {Groups: []consolidateGroup{
			{Title: "Middleware", Lane: groupindex.LaneCore, CandidateRefs: []string{"c1", "c2"}},
			{Title: "Middleware again", Lane: groupindex.LaneCore, CandidateRefs: []string{"c2", "c3"}},
		}},
		"moves a lane": {Groups: []consolidateGroup{
			{Title: "Everything", Lane: groupindex.LaneCore, CandidateRefs: []string{"c1", "c4"}},
		}},
		"invents a candidate": {Groups: []consolidateGroup{
			{Title: "Middleware", Lane: groupindex.LaneCore, CandidateRefs: []string{"c1", "c99"}},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			merged, _ := applyConsolidation(candidates, response)
			seen := make(map[string]struct{})
			for _, group := range merged.groups {
				for _, member := range group.MemberSubjectIDs {
					seen[member] = struct{}{}
				}
			}
			for _, want := range []string{"s1", "s2", "s3", "s4", "s5"} {
				if _, kept := seen[want]; !kept {
					t.Fatalf("member %s was lost: %#v", want, merged.groups)
				}
			}
		})
	}
}

// A lane follows from a member's own categories, so consolidation may not move
// one; the candidate whose lane does not match stays as it was.
func TestConsolidationRefusesToMoveALane(t *testing.T) {
	t.Parallel()

	merged, diagnostics := applyConsolidation(consolidationCandidates(), consolidateResponse{
		Groups: []consolidateGroup{
			{Title: "Everything", Lane: groupindex.LaneCore, CandidateRefs: []string{"c1", "c4"}},
		},
	})
	for _, group := range merged.groups {
		for _, member := range group.MemberSubjectIDs {
			if member == "s5" && group.Lane != groupindex.LaneTriggers {
				t.Fatalf("a triggers member was moved to %s: %#v", group.Lane, group)
			}
		}
	}
	if !hasDiagnostic(diagnostics, diagnosticConsolidationLaneMismatch) {
		t.Fatalf("the refused lane move was not recorded: %#v", diagnostics)
	}
}

// Two candidates that became one group had a connection between them that is
// now a group talking to itself, and a group does not talk to itself.
func TestConsolidationMovesConnectionsAndDropsSelfLoops(t *testing.T) {
	t.Parallel()

	merged, _ := applyConsolidation(consolidationCandidates(), consolidateResponse{
		Groups: []consolidateGroup{
			{Title: "Middleware", Lane: groupindex.LaneCore, CandidateRefs: []string{"c1", "c2"}},
			{Title: "Router core", Lane: groupindex.LaneCore, CandidateRefs: []string{"c3"}},
			{Title: "HTTP entry", Lane: groupindex.LaneTriggers, CandidateRefs: []string{"c4"}},
		},
	})
	keyOf := make(map[string]string, len(merged.groups))
	for _, group := range merged.groups {
		keyOf[group.Title] = group.Key
	}
	want := [][2]string{
		{keyOf["HTTP entry"], keyOf["Middleware"]},
		{keyOf["HTTP entry"], keyOf["Router core"]},
	}
	got := make([][2]string, 0, len(merged.connections))
	for _, connection := range merged.connections {
		got = append(got, [2]string{connection.FromGroupKey, connection.ToGroupKey})
	}
	if len(got) != len(want) {
		t.Fatalf("connections = %#v, want %#v", got, want)
	}
	for _, pair := range want {
		if !containsPair(got, pair) {
			t.Fatalf("connection %v is missing from %#v", pair, got)
		}
	}
}

// A candidate the response never names is not an error and not a loss: it
// comes through exactly as its own shard proposed it.
func TestConsolidationKeepsWhatTheResponseIgnored(t *testing.T) {
	t.Parallel()

	merged, diagnostics := applyConsolidation(consolidationCandidates(), consolidateResponse{
		Groups: []consolidateGroup{
			{Title: "Middleware", Lane: groupindex.LaneCore, CandidateRefs: []string{"c1", "c2"}},
		},
	})
	var kept *groupProposal
	for position := range merged.groups {
		if merged.groups[position].Title == "HTTP entry" {
			kept = &merged.groups[position]
		}
	}
	if kept == nil {
		t.Fatalf("the ignored candidate is gone: %#v", merged.groups)
	}
	if kept.Lane != groupindex.LaneTriggers ||
		!reflect.DeepEqual(kept.MemberSubjectIDs, []string{"s5"}) {
		t.Fatalf("the ignored candidate changed: %#v", kept)
	}
	if !hasDiagnostic(diagnostics, diagnosticConsolidationUnclaimed) {
		t.Fatalf("the ignored candidate was not recorded: %#v", diagnostics)
	}
}

func hasDiagnostic(diagnostics []groupindex.Diagnostic, kind string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Kind == kind {
			return true
		}
	}
	return false
}

func containsPair(values [][2]string, wanted [2]string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
