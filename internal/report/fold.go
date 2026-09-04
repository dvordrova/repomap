package report

import (
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/groupindex"
)

// foldIndexes is the page's view of the group graph: one group per title in a
// target. The graph itself holds one group per title per lane, because a lane
// follows from a member's own categories and a group may not cross one —
// chi's "Client IP middleware" is fifty core symbols, two trigger symbols and
// one dependency, and the index is right to keep them apart. The page is not:
// three boxes with one name are one thing said three times, and nine of the
// fourteen boxes on chi's first screen were such repeats. So the slices are
// folded here, for the picture and the cards alike; the largest slice lends
// its identity, lane and place in a zone, the connections of every slice
// follow it, and nothing below the page is changed.
func foldIndexes(indexes []groupindex.Index) []groupindex.Index {
	canonical := make(map[groupindex.Endpoint]groupindex.Endpoint)
	folded := make([]groupindex.Index, len(indexes))
	for position, index := range indexes {
		folded[position] = index
		folded[position].Groups = foldGroups(index, canonical)
		folded[position].Containers = foldContainers(index, canonical)
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
func foldGroups(index groupindex.Index, canonical map[groupindex.Endpoint]groupindex.Endpoint) []groupindex.Group {
	bySize := append([]groupindex.Group(nil), index.Groups...)
	sort.SliceStable(bySize, func(left, right int) bool {
		return len(bySize[left].MemberSubjectIDs) > len(bySize[right].MemberSubjectIDs)
	})
	at := make(map[string]int, len(bySize))
	result := make([]groupindex.Group, 0, len(bySize))
	for _, group := range bySize {
		key := foldTitle(group.Title)
		here := groupindex.Endpoint{TargetID: index.Target.ID, GroupID: group.ID}
		position, seen := at[key]
		if !seen || key == "" {
			at[key] = len(result)
			canonical[here] = here
			result = append(result, groupindex.Group{
				ID: group.ID, Title: group.Title, Summary: group.Summary, Lane: group.Lane,
				MemberSubjectIDs:   append([]string(nil), group.MemberSubjectIDs...),
				EvidenceSubjectIDs: append([]string(nil), group.EvidenceSubjectIDs...),
			})
			continue
		}
		into := &result[position]
		canonical[here] = groupindex.Endpoint{TargetID: index.Target.ID, GroupID: into.ID}
		into.MemberSubjectIDs = appendAbsent(into.MemberSubjectIDs, group.MemberSubjectIDs)
		into.EvidenceSubjectIDs = appendAbsent(into.EvidenceSubjectIDs, group.EvidenceSubjectIDs)
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
func foldContainers(index groupindex.Index, canonical map[groupindex.Endpoint]groupindex.Endpoint) []groupindex.Container {
	result := make([]groupindex.Container, 0, len(index.Containers))
	for _, container := range index.Containers {
		kept := groupindex.Container{
			ID: container.ID, Title: container.Title, Summary: container.Summary, Lane: container.Lane,
		}
		for _, id := range container.GroupIDs {
			here := groupindex.Endpoint{TargetID: index.Target.ID, GroupID: id}
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
func foldConnections(connections []groupindex.Connection, canonical map[groupindex.Endpoint]groupindex.Endpoint) []groupindex.Connection {
	type said struct {
		from, to    groupindex.Endpoint
		label, kind string
	}
	seen := make(map[said]struct{}, len(connections))
	result := make([]groupindex.Connection, 0, len(connections))
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
		key := said{connection.From, connection.To, connection.Label, connection.SemanticKind}
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
