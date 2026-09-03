package programgrouping

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
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
		"joins two": {Assign: []consolidateAssign{
			{Ref: "c1", Cluster: "mw"}, {Ref: "c2", Cluster: "mw"},
			{Ref: "c3", Cluster: "router"}, {Ref: "c4", Cluster: "entry"},
		}},
		"forgets one": {Assign: []consolidateAssign{
			{Ref: "c1", Cluster: "mw"}, {Ref: "c2", Cluster: "mw"},
		}},
		"names one twice": {Assign: []consolidateAssign{
			{Ref: "c1", Cluster: "mw"}, {Ref: "c2", Cluster: "mw"},
			{Ref: "c2", Cluster: "other"}, {Ref: "c3", Cluster: "other"},
		}},
		"puts two lanes under one label": {Assign: []consolidateAssign{
			{Ref: "c1", Cluster: "all"}, {Ref: "c4", Cluster: "all"},
		}},
		"invents a candidate": {Assign: []consolidateAssign{
			{Ref: "c1", Cluster: "mw"}, {Ref: "c99", Cluster: "mw"},
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

// A part of a target is naturally several lanes at once, but a lane follows
// from a member's own categories and consolidation may not move one. A group
// naming candidates of two lanes becomes one group per lane under the same
// name — not one group and a reject that falls back out on its own.
func TestConsolidationSplitsACrossLaneGroupByLane(t *testing.T) {
	t.Parallel()

	merged, _ := applyConsolidation(consolidationCandidates(), consolidateResponse{
		Assign: []consolidateAssign{
			{Ref: "c1", Cluster: "all"}, {Ref: "c4", Cluster: "all"},
		},
	})
	// c1 (core) and c4 (triggers) shared one label, so they are two clusters,
	// one per lane, and no member crossed.
	var core, triggers *groupProposal
	for position := range merged.groups {
		group := &merged.groups[position]
		if !strings.HasPrefix(group.Key, "k") {
			continue
		}
		switch group.Lane {
		case groupindex.LaneCore:
			core = group
		case groupindex.LaneTriggers:
			triggers = group
		}
	}
	if core == nil || triggers == nil {
		t.Fatalf("one label over two lanes did not stay two clusters: %#v", merged.groups)
	}
	if !reflect.DeepEqual(core.MemberSubjectIDs, []string{"s1", "s2"}) ||
		!reflect.DeepEqual(triggers.MemberSubjectIDs, []string{"s5"}) {
		t.Fatalf("members moved lane: core=%#v triggers=%#v", core, triggers)
	}
}

// Two candidates that became one group had a connection between them that is
// now a group talking to itself, and a group does not talk to itself.
func TestConsolidationMovesConnectionsAndDropsSelfLoops(t *testing.T) {
	t.Parallel()

	merged, _ := applyConsolidation(consolidationCandidates(), consolidateResponse{
		Assign: []consolidateAssign{
			{Ref: "c1", Cluster: "mw"}, {Ref: "c2", Cluster: "mw"},
			{Ref: "c3", Cluster: "router"}, {Ref: "c4", Cluster: "entry"},
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
		Assign: []consolidateAssign{
			{Ref: "c1", Cluster: "mw"}, {Ref: "c2", Cluster: "mw"},
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

// A window numbers its candidates from one. Translating those refs back onto
// the whole list is what lets a later level resolve them; without it a part
// gathered whichever groups happened to sit at those positions.
func TestWindowCandidateRefsBecomeGlobal(t *testing.T) {
	t.Parallel()

	if got := globalCandidateRefs([]string{"c1", "c3"}, 40); !reflect.DeepEqual(got, []string{"c41", "c43"}) {
		t.Fatalf("second window refs = %#v", got)
	}
	if got := globalCandidateRefs([]string{"c1"}, 0); !reflect.DeepEqual(got, []string{"c1"}) {
		t.Fatalf("first window refs = %#v", got)
	}
}

// A consolidation question carries the count it should answer with, because
// the rule it replaces was written in units of a target while the question is
// asked of one window, and a model reading it against the wrong denominator
// squeezed forty candidates to fourteen in one draw and joined nothing in the
// next.
func TestWantedLabelsHalvesAWindow(t *testing.T) {
	for _, c := range []struct{ candidates, want int }{
		{2, 2}, {4, 4}, {8, 4}, {20, 10}, {40, 20}, {41, 21},
	} {
		if got := wantedLabels(phaseConsolidate, c.candidates); got != c.want {
			t.Errorf("wantedLabels(%d) = %d, want %d", c.candidates, got, c.want)
		}
	}
}

// A window of forty candidates came back as a single label in one draw: an
// answer of the right shape saying that forty unrelated things are one thing.
// The window keeps its candidates unjoined instead.
func TestFlatConsolidationIsRefused(t *testing.T) {
	request := consolidateRequest{Phase: phaseConsolidate, Labels: 20}
	for position := range 40 {
		request.Candidates = append(request.Candidates, consolidateCandidate{Ref: candidateRef(position)})
	}
	flat := consolidateResponse{}
	spread := consolidateResponse{}
	for position := range 40 {
		ref := candidateRef(position)
		flat.Assign = append(flat.Assign, consolidateAssign{Ref: ref, Cluster: "everything"})
		spread.Assign = append(spread.Assign, consolidateAssign{
			Ref: ref, Cluster: fmt.Sprintf("cluster-%d", position/3),
		})
	}
	// The parts question is answered well by a small number, and refusing it
	// there left three cold draws of chi with no zones at all.
	if err := refuseFlatConsolidation(consolidateRequest{Phase: phaseContainers, Labels: 13}, consolidateResponse{
		Assign: []consolidateAssign{{Ref: "c1", Cluster: "router"}, {Ref: "c2", Cluster: "router"}},
	}); err != nil {
		t.Errorf("two parts for a target were refused: %v", err)
	}
	if got := wantedLabels(phaseContainers, 40); got != 13 {
		t.Errorf("wantedLabels(containers, 40) = %d, want 13", got)
	}
	if err := refuseFlatConsolidation(request, flat); err == nil {
		t.Error("forty candidates gathered into one label were accepted")
	}
	if err := refuseFlatConsolidation(request, spread); err != nil {
		t.Errorf("fourteen labels for forty candidates were refused: %v", err)
	}
	// Half of what was asked for still says something true.
	if err := refuseFlatConsolidation(consolidateRequest{Phase: phaseConsolidate, Labels: 4}, consolidateResponse{
		Assign: []consolidateAssign{{Ref: "c1", Cluster: "a"}, {Ref: "c2", Cluster: "b"}},
	}); err != nil {
		t.Errorf("two labels for four asked were refused: %v", err)
	}
}

// The parts a target is divided into are named one call earlier, so the
// question after it is a choice from a closed list and not an invented
// partition — which three cold draws answered with nineteen parts, thirty-six
// and two. A name outside the list is the invention coming back, and the group
// carrying it belongs to no part instead.
func TestOnlyNamedPartsSurvive(t *testing.T) {
	request := consolidateRequest{
		Phase: phaseContainers, Parts: []string{"Request routing", "Middleware chain"},
	}
	kept, err := keepNamedParts(request, consolidateResponse{Assign: []consolidateAssign{
		{Ref: "c1", Cluster: "request routing"},
		{Ref: "c2", Cluster: "Middleware chain"},
		{Ref: "c3", Cluster: "something it made up"},
	}})
	if err != nil {
		t.Fatalf("keepNamedParts: %v", err)
	}
	if len(kept.Assign) != 2 {
		t.Fatalf("kept = %#v", kept.Assign)
	}
	// A part keeps the spelling it was named with, not the one it came back as.
	if kept.Assign[0].Cluster != "Request routing" {
		t.Errorf("cluster = %q, want the spelling from parts", kept.Assign[0].Cluster)
	}
	if _, err := keepNamedParts(request, consolidateResponse{Assign: []consolidateAssign{
		{Ref: "c1", Cluster: "invented"},
	}}); err == nil {
		t.Error("an answer naming no part at all was accepted")
	}
}

// The field an answer arrives under follows the wording of the question: asked
// to choose from `parts`, the model answers with "part". Insisting on
// "cluster" threw away three targets' worth of entirely correct answers.
func TestAssignmentIsReadUnderAnyOfItsNames(t *testing.T) {
	var decoded consolidateResponse
	if err := json.Unmarshal([]byte(
		`{"assign":[{"ref":"c1","part":"route tree"},{"ref":"c2","group":"middleware chain"},`+
			`{"ref":"c3","cluster":"request routing"}]}`), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := []string{"route tree", "middleware chain", "request routing"}
	for position, assign := range decoded.Assign {
		if assign.cluster() != want[position] {
			t.Errorf("cluster() = %q, want %q", assign.cluster(), want[position])
		}
	}
}
