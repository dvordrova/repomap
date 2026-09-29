package groupindex

import (
	"fmt"
	"slices"
	"strings"

	"github.com/dvordrova/repomap/internal/programindex"
)

// OutboundCaller is one way the program's code reaches an outgoing call
// from outside the part making it (OutboundCall.ReachedFrom), a code fact:
// Redis's connect is written in anet.c's anetTcpGenericConnect, and
// redis-server reaches it from syncWithMaster, redis-cli from cliConnect.
// It is the first caller in another part, with Location where it calls
// into the call's part, or, where the callers run out inside that part, the
// seed or input handler the path starts at, with no Location. GroupID is
// the caller's part; empty for a caller no drawn part holds.
type OutboundCaller struct {
	SubjectID string                 `json:"subject_id"`
	GroupID   string                 `json:"group_id,omitempty"`
	Location  *programindex.Location `json:"location,omitempty"`
}

// reachedFrom walks, per program, the exact calls into an outgoing call's
// function backwards (READING § Outside systems). It passes only through
// callers in the function's own part; each path ends at the first caller
// in another part, recorded with its call site. A path whose callers run
// out inside the part is kept only when it ends at a seed or an input's
// handler; otherwise it is dropped, so a helper nothing uses (anet.c's
// anetTcpNonBlockConnect) names nobody. Callers in the program's test
// sources, and callers its adapter proved it never runs, call nothing
// here. A cycle stops where it closes; there is no depth cap.
type reachedFrom struct {
	callers map[string][]reachedCall
	groupOf func(string) string
	entries map[string]bool
}

type reachedCall struct {
	from     string
	location *programindex.Location
}

func newReachedFrom(program programindex.Index, groupOf func(string) string, entries map[string]bool) *reachedFrom {
	tests := make(map[string]bool, len(program.Target.TestSources))
	for _, source := range program.Target.TestSources {
		tests[atlasPath(source)] = true
	}
	objects := make(map[string]programindex.Object, len(program.Objects))
	for _, object := range program.Objects {
		objects[object.ID] = object
	}
	callers := make(map[string][]reachedCall)
	for _, relation := range program.Relations {
		if relation.Kind != programindex.RelationCalls || relation.Resolution != programindex.ResolutionExact {
			continue
		}
		caller, known := objects[relation.FromID]
		if !known || caller.Unreachable || caller.Location != nil && tests[atlasPath(caller.Location.Path)] ||
			relation.Location != nil && tests[atlasPath(relation.Location.Path)] {
			continue
		}
		for _, to := range relation.ToIDs {
			if to != relation.FromID {
				callers[to] = append(callers[to], reachedCall{from: relation.FromID, location: relation.Location})
			}
		}
	}
	return &reachedFrom{callers: callers, groupOf: groupOf, entries: entries}
}

// of is the callers an outgoing call made in subjectID is reached from, in
// canonical order; nil for a call no part holds.
func (walk *reachedFrom) of(subjectID string) []OutboundCaller {
	own := walk.groupOf(subjectID)
	if subjectID == "" || own == "" {
		return nil
	}
	var result []OutboundCaller
	seen := map[string]bool{subjectID: true}
	stack := []string{subjectID}
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		calls := walk.callers[current]
		if len(calls) == 0 && current != subjectID && walk.entries[current] {
			result = append(result, OutboundCaller{SubjectID: current, GroupID: own})
		}
		for _, call := range calls {
			if group := walk.groupOf(call.from); group != own {
				result = append(result, OutboundCaller{SubjectID: call.from, GroupID: group, Location: cloneLocation(call.location)})
				continue
			}
			if !seen[call.from] {
				seen[call.from] = true
				stack = append(stack, call.from)
			}
		}
	}
	slices.SortFunc(result, compareOutboundCallers)
	return slices.CompactFunc(result, func(a, b OutboundCaller) bool { return compareOutboundCallers(a, b) == 0 })
}

// compareOutboundCallers orders callers by part, then call site (a path's
// start with no site after the sites), then subject.
func compareOutboundCallers(a, b OutboundCaller) int {
	switch {
	case a.GroupID != b.GroupID:
		if a.GroupID == "" || b.GroupID == "" {
			return strings.Compare(b.GroupID, a.GroupID)
		}
		if compactIDLess(a.GroupID, b.GroupID, "g") {
			return -1
		}
		return 1
	case !sameLocation(a.Location, b.Location):
		if locationBefore(a.Location, b.Location) {
			return -1
		}
		return 1
	case a.SubjectID != b.SubjectID:
		if subjectIDLess(a.SubjectID, b.SubjectID) {
			return -1
		}
		return 1
	}
	return 0
}

func sameLocation(a, b *programindex.Location) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

func cloneOutboundCallers(callers []OutboundCaller) []OutboundCaller {
	if callers == nil {
		return nil
	}
	result := slices.Clone(callers)
	for i := range result {
		result[i].Location = cloneLocation(callers[i].Location)
	}
	return result
}

// cloneOutbound is a consumer-owned copy of outbound calls.
func cloneOutbound(calls []OutboundCall) []OutboundCall {
	if calls == nil {
		return nil
	}
	result := append([]OutboundCall(nil), calls...)
	for i := range result {
		result[i].Values = cloneStrings(calls[i].Values)
		result[i].Uses = cloneDestinationUses(calls[i].Uses)
		result[i].ReachedFrom = cloneOutboundCallers(calls[i].ReachedFrom)
	}
	return result
}

func validateOutboundCallers(call OutboundCall, subjects map[string]Subject, groups map[string]struct{}) error {
	if len(call.ReachedFrom) > 0 && call.SubjectID == "" {
		return fmt.Errorf("group index: outbound call %q is reached from callers but made by no subject", call.ID)
	}
	for i, caller := range call.ReachedFrom {
		_, subjectExists := subjects[caller.SubjectID]
		_, groupExists := groups[caller.GroupID]
		if !subjectExists || caller.GroupID != "" && !groupExists || !validOptionalLocation(caller.Location) ||
			i > 0 && compareOutboundCallers(call.ReachedFrom[i-1], caller) >= 0 {
			return fmt.Errorf("group index: outbound call %q has an invalid or noncanonical caller %q", call.ID, caller.SubjectID)
		}
	}
	return nil
}
