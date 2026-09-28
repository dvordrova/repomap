package report

import (
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// The sidebar must explain the same original relations as the drawing, even
// when a model supplied no sentence for that pair of parts.
func (builder *pageBuilder) nativeGroupConnections(index groupindex.Index, group groupindex.Group) []pageConnection {
	section := builder.byProgram[index.Target.ID]
	if section == nil {
		return nil
	}
	between := builder.edgesBetweenGroups(index)
	var rows []pageConnection
	for _, position := range between.byGroup[group.ID] {
		edge := index.StructuralEdges[position]
		from, to := between.groups[between.groupOf[edge.FromSubjectID]], between.groups[between.groupOf[edge.ToSubjectID]]
		arrow, peer := "→", to
		if from.ID != group.ID {
			arrow, peer = "←", from
		}
		fromSubject, fromKnown := builder.subject(index.Target.ID, edge.FromSubjectID)
		toSubject, toKnown := builder.subject(index.Target.ID, edge.ToSubjectID)
		if !fromKnown || !toKnown {
			continue
		}
		fromName, fromAnchor := builder.subjectDisplay(fromSubject.subject)
		toName, toAnchor := builder.subjectDisplay(toSubject.subject)
		if fromName == "" || toName == "" {
			continue
		}
		fromDecl := fromAnchor
		if edge.Location != nil {
			fromAnchor = builder.links.anchorPointer(edge.Location.Path, edge.Location.Line, edge.Location.Column)
		}
		rows = append(rows, pageConnection{Native: true, EvidenceID: edge.RelationID + "\x00" + edge.ToSubjectID, Arrow: arrow, Title: peer.Title, Href: "#" + groupAnchorID(section.ID, peer.ID),
			Label:    fromName + " " + strings.ReplaceAll(string(edge.RelationKind), "_", " ") + " " + toName,
			Possible: edge.Resolution != programindex.ResolutionExact, FromSource: fromAnchor, ToSource: toAnchor,
			Kind: relationWord(string(edge.RelationKind), section.Language), FromName: fromName, ToName: toName, FromDecl: fromDecl, ToDecl: toAnchor,
			fromSubject: edge.FromSubjectID, at: edge.Location, fromTarget: index.Target.ID, toTarget: index.Target.ID, toSubject: edge.ToSubjectID})
	}
	return rows
}

// groupEdges are one index's native relations between two different groups
// that no connection already covers, listed under both groups in the index's
// own order. A subject in several groups belongs to the last, as it always
// has here. Every card of the target reads the same list.
type groupEdges struct {
	from    groupEdgesSource
	groups  map[string]groupindex.Group
	groupOf map[string]string
	byGroup map[string][]int
}

// groupEdgesSource is the identity of the lists an index's groupEdges read.
type groupEdgesSource struct {
	edges       *groupindex.StructuralEdge
	connections *groupindex.Connection
	groups      *groupindex.Group
	counts      [3]int
}

func firstOf[T any](values []T) *T {
	if len(values) == 0 {
		return nil
	}
	return &values[0]
}

func (builder *pageBuilder) edgesBetweenGroups(index groupindex.Index) *groupEdges {
	from := groupEdgesSource{edges: firstOf(index.StructuralEdges), connections: firstOf(index.Connections), groups: firstOf(index.Groups),
		counts: [3]int{len(index.StructuralEdges), len(index.Connections), len(index.Groups)}}
	// Keyed by target, and only for the very lists it was built from.
	if cached := builder.groupEdges[index.Target.ID]; cached != nil && cached.from == from {
		return cached
	}
	between := &groupEdges{from: from, groups: make(map[string]groupindex.Group, len(index.Groups)), groupOf: map[string]string{}, byGroup: map[string][]int{}}
	for _, part := range index.Groups {
		between.groups[part.ID] = part
		for _, id := range part.MemberSubjectIDs {
			between.groupOf[id] = part.ID
		}
	}
	covered := map[string]bool{}
	for _, connection := range index.Connections {
		if strings.HasPrefix(connection.SourceKind, "native_") {
			covered[connection.SourceID+"\x00"+connection.ToSubjectID] = true
		}
	}
	for position, edge := range index.StructuralEdges {
		if edge.Role != groupindex.EdgeRelationTarget || covered[edge.RelationID+"\x00"+edge.ToSubjectID] {
			continue
		}
		from, to := between.groupOf[edge.FromSubjectID], between.groupOf[edge.ToSubjectID]
		if from == "" || to == "" || from == to {
			continue
		}
		between.byGroup[from] = append(between.byGroup[from], position)
		between.byGroup[to] = append(between.byGroup[to], position)
	}
	if builder.groupEdges == nil {
		builder.groupEdges = map[string]*groupEdges{}
	}
	builder.groupEdges[index.Target.ID] = between
	return between
}

// A part's native internal relations remain inspectable even though the map
// draws no self-arrow. Contains is represented by the declaration inventory.
func (builder *pageBuilder) internalGroupConnections(index groupindex.Index, group groupindex.Group) []pageConnection {
	members := make(map[string]bool, len(group.MemberSubjectIDs))
	for _, id := range group.MemberSubjectIDs {
		members[id] = true
	}
	var rows []pageConnection
	for _, edge := range index.StructuralEdges {
		if edge.Role != groupindex.EdgeRelationTarget ||
			!members[edge.FromSubjectID] || !members[edge.ToSubjectID] {
			continue
		}
		fromSubject, fromKnown := builder.subject(index.Target.ID, edge.FromSubjectID)
		toSubject, toKnown := builder.subject(index.Target.ID, edge.ToSubjectID)
		if !fromKnown || !toKnown {
			continue
		}
		fromName, fromAnchor := builder.subjectDisplay(fromSubject.subject)
		toName, toAnchor := builder.subjectDisplay(toSubject.subject)
		if fromName == "" || toName == "" {
			continue
		}
		fromDecl := fromAnchor
		if edge.Location != nil {
			fromAnchor = builder.links.anchorPointer(edge.Location.Path, edge.Location.Line, edge.Location.Column)
		}
		rows = append(rows, pageConnection{Native: true, EvidenceID: edge.RelationID + "\x00" + edge.ToSubjectID,
			Label:    fromName + " " + strings.ReplaceAll(string(edge.RelationKind), "_", " ") + " " + toName,
			Possible: edge.Resolution != programindex.ResolutionExact, FromSource: fromAnchor, ToSource: toAnchor,
			Kind: relationWord(string(edge.RelationKind), builder.targetLanguage(index.Target.ID)), FromName: fromName, ToName: toName, FromDecl: fromDecl, ToDecl: toAnchor,
			fromSubject: edge.FromSubjectID, fromTarget: index.Target.ID, toTarget: index.Target.ID, toSubject: edge.ToSubjectID})
	}
	return collapseConnections(rows)
}

// ConnectionGroups is a reading view of the existing connections, computed
// after translation. Equal participant IDs and direction share one heading;
// every original row, source pair and possible-call flag remains inside it.
func (group pageGroup) ConnectionGroups() []pageConnectionGroup {
	var result []pageConnectionGroup
	positions := map[string]int{}
	for _, row := range group.Connections {
		key := row.Arrow + "\x00" + row.Href
		position, found := positions[key]
		if !found || row.Href == "" {
			position = len(result)
			positions[key] = position
			result = append(result, pageConnectionGroup{Arrow: row.Arrow, Title: row.Title, OtherTarget: row.OtherTarget, Href: row.Href})
		}
		item := &result[position]
		item.Rows = append(item.Rows, row)
		if row.Summary != "" {
			found := false
			for _, summary := range item.Summaries {
				if summary.Text == row.Summary {
					found = true
					break
				}
			}
			if !found {
				item.Summaries = append(item.Summaries, pageConnectionSummary{Text: row.Summary, Ref: row.SummaryRef})
			}
		}
	}
	// Who reaches the part comes before what it reaches: Client connections
	// and replies listed nineteen parts it calls, mostly utilities, before
	// the fourteen that call it.
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Arrow != result[j].Arrow {
			return result[i].Arrow == "←"
		}
		return result[i].Title < result[j].Title
	})
	return result
}

type pageConnectionGroup struct {
	Arrow, Title, OtherTarget, Href string
	Summaries                       []pageConnectionSummary
	Rows                            []pageConnection
}

type pageConnectionSummary struct{ Text, Ref string }
