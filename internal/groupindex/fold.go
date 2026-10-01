package groupindex

import (
	"fmt"
	"sort"
	"strings"
)

// Fold is the page's view of the group graph: one group per title in a
// target. The graph itself holds one group per title per lane, because a lane
// follows from a member's own categories and a group may not cross one —
// chi's "Client IP middleware" is fifty core symbols, two trigger symbols and
// one dependency, and the index is right to keep them apart. The page is not:
// three boxes with one name are one thing said three times, and nine of the
// fourteen boxes on chi's first screen were such repeats. So the slices are
// folded here, for the picture and the cards alike; the largest slice lends
// its identity, lane and place in a zone, the connections of every slice
// follow it, and nothing below the page is changed. The report folds the
// graph it presents, and analysis folds the same way before deriving the
// overview's test-free views (WithTestFreeViews), which the page reads
// folded.
func Fold(indexes []Index) []Index {
	canonical := make(map[Endpoint]Endpoint)
	folded := make([]Index, len(indexes))
	for position, index := range indexes {
		folded[position] = index
		folded[position].Groups = foldGroups(index, canonical)
		folded[position].Containers = foldContainers(index, canonical)
		folded[position].Operations = append([]Operation(nil), index.Operations...)
		for i := range folded[position].Operations {
			op := &folded[position].Operations[i]
			op.GroupID = canonical[Endpoint{TargetID: index.Target.ID, GroupID: op.GroupID}].GroupID
		}
		folded[position].Outbound = append([]OutboundCall(nil), index.Outbound...)
		for i := range folded[position].Outbound {
			call := &folded[position].Outbound[i]
			call.GroupID = canonical[Endpoint{TargetID: index.Target.ID, GroupID: call.GroupID}].GroupID
			call.ReachedFrom = append([]OutboundCaller(nil), call.ReachedFrom...)
			for j := range call.ReachedFrom {
				caller := &call.ReachedFrom[j]
				caller.GroupID = canonical[Endpoint{TargetID: index.Target.ID, GroupID: caller.GroupID}].GroupID
			}
		}
	}
	for position := range folded {
		folded[position].Connections = foldConnections(folded[position].Connections, canonical)
	}
	return folded
}

func foldTitle(title string) string {
	return strings.ToLower(strings.TrimSpace(title))
}

// foldGroups joins a target's groups by title, largest slice first, and
// records where every slice went.
func foldGroups(index Index, canonical map[Endpoint]Endpoint) []Group {
	bySize := append([]Group(nil), index.Groups...)
	sort.SliceStable(bySize, func(left, right int) bool {
		return len(bySize[left].MemberSubjectIDs) > len(bySize[right].MemberSubjectIDs)
	})
	at := make(map[string]int, len(bySize))
	result := make([]Group, 0, len(bySize))
	for _, group := range bySize {
		key := foldTitle(group.Title)
		here := Endpoint{TargetID: index.Target.ID, GroupID: group.ID}
		position, seen := at[key]
		if !seen || key == "" {
			at[key] = len(result)
			canonical[here] = here
			result = append(result, Group{
				ID: group.ID, Title: group.Title, Summary: group.Summary, Lane: group.Lane, Core: group.Core,
				MemberSubjectIDs:   append([]string(nil), group.MemberSubjectIDs...),
				EvidenceSubjectIDs: append([]string(nil), group.EvidenceSubjectIDs...),
			})
			continue
		}
		into := &result[position]
		canonical[here] = Endpoint{TargetID: index.Target.ID, GroupID: into.ID}
		into.MemberSubjectIDs = appendAbsent(into.MemberSubjectIDs, group.MemberSubjectIDs)
		into.EvidenceSubjectIDs = appendAbsent(into.EvidenceSubjectIDs, group.EvidenceSubjectIDs)
		into.Core = into.Core || group.Core
		if len(group.Summary) > len(into.Summary) {
			into.Summary = group.Summary
		}
	}
	// Back in the order the index had them, so nothing else on the page moves.
	order := make(map[string]int, len(index.Groups))
	for position, group := range index.Groups {
		order[group.ID] = position
	}
	sort.SliceStable(result, func(left, right int) bool {
		return order[result[left].ID] < order[result[right].ID]
	})
	return result
}

// foldContainers keeps a folded group in the part that held its largest
// slice. A part that held only a smaller slice loses it — the box is drawn
// once, where most of it is — and a part left holding nothing goes.
func foldContainers(index Index, canonical map[Endpoint]Endpoint) []Container {
	result := make([]Container, 0, len(index.Containers))
	for _, container := range index.Containers {
		kept := Container{
			ID: container.ID, Title: container.Title, Summary: container.Summary, Lane: container.Lane, Core: container.Core,
		}
		for _, id := range container.GroupIDs {
			here := Endpoint{TargetID: index.Target.ID, GroupID: id}
			if canonical[here] != here {
				continue
			}
			kept.GroupIDs = append(kept.GroupIDs, id)
		}
		if len(kept.GroupIDs) > 0 {
			result = append(result, kept)
		}
	}
	return result
}

// foldConnections points every connection at the folded groups. Two slices of
// one thing talking to each other is that thing talking to itself and goes;
// two slices saying the same thing to the same neighbour say it once.
func foldConnections(connections []Connection, canonical map[Endpoint]Endpoint) []Connection {
	type said struct {
		from, to                                     Endpoint
		label, kind                                  string
		sourceKind, sourceID, fromSubject, toSubject string
		fromLocation, toLocation                     string
	}
	seen := make(map[said]struct{}, len(connections))
	result := make([]Connection, 0, len(connections))
	for _, connection := range connections {
		if to, known := canonical[connection.From]; known {
			connection.From = to
		}
		if to, known := canonical[connection.To]; known {
			connection.To = to
		}
		if connection.From == connection.To {
			continue
		}
		key := said{from: connection.From, to: connection.To, label: connection.Label, kind: connection.SemanticKind, sourceKind: connection.SourceKind, sourceID: connection.SourceID, fromSubject: connection.FromSubjectID, toSubject: connection.ToSubjectID}
		if connection.FromLocation != nil {
			key.fromLocation = fmt.Sprint(*connection.FromLocation)
		}
		if connection.ToLocation != nil {
			key.toLocation = fmt.Sprint(*connection.ToLocation)
		}
		if _, repeated := seen[key]; repeated {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, connection)
	}
	return result
}

func appendAbsent(into, more []string) []string {
	present := make(map[string]struct{}, len(into))
	for _, value := range into {
		present[value] = struct{}{}
	}
	for _, value := range more {
		if _, repeated := present[value]; repeated {
			continue
		}
		present[value] = struct{}{}
		into = append(into, value)
	}
	return into
}
