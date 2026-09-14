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
	groupOf := map[string]groupindex.Group{}
	for _, part := range index.Groups {
		for _, id := range part.MemberSubjectIDs {
			groupOf[id] = part
		}
	}
	covered := map[string]bool{}
	for _, connection := range index.Connections {
		if strings.HasPrefix(connection.SourceKind, "native_") {
			covered[connection.SourceID+"\x00"+connection.ToSubjectID] = true
		}
	}
	var rows []pageConnection
	for _, edge := range index.StructuralEdges {
		if edge.Role != groupindex.EdgeRelationTarget || covered[edge.RelationID+"\x00"+edge.ToSubjectID] {
			continue
		}
		from, to := groupOf[edge.FromSubjectID], groupOf[edge.ToSubjectID]
		if from.ID == "" || to.ID == "" || from.ID == to.ID {
			continue
		}
		arrow, peer := "→", to
		if from.ID != group.ID {
			if to.ID != group.ID {
				continue
			}
			arrow, peer = "←", from
		}
		fromSubject, toSubject := builder.subjects[edge.FromSubjectID], builder.subjects[edge.ToSubjectID]
		fromName, fromAnchor := builder.subjectDisplay(fromSubject.subject)
		toName, toAnchor := builder.subjectDisplay(toSubject.subject)
		if fromName == "" || toName == "" {
			continue
		}
		if edge.Location != nil {
			fromAnchor = builder.links.anchorPointer(edge.Location.Path, edge.Location.Line, edge.Location.Column)
		}
		rows = append(rows, pageConnection{Native: true, EvidenceID: edge.RelationID + "\x00" + edge.ToSubjectID, Arrow: arrow, Title: peer.Title, Href: "#" + groupAnchorID(section.ID, peer.ID),
			Label:    fromName + " " + strings.ReplaceAll(string(edge.RelationKind), "_", " ") + " " + toName,
			Possible: edge.Resolution != programindex.ResolutionExact, FromSource: fromAnchor, ToSource: toAnchor})
	}
	return rows
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
		if edge.Role != groupindex.EdgeRelationTarget || edge.RelationKind == programindex.RelationContains ||
			!members[edge.FromSubjectID] || !members[edge.ToSubjectID] {
			continue
		}
		fromName, fromAnchor := builder.subjectDisplay(builder.subjects[edge.FromSubjectID].subject)
		toName, toAnchor := builder.subjectDisplay(builder.subjects[edge.ToSubjectID].subject)
		if fromName == "" || toName == "" {
			continue
		}
		if edge.Location != nil {
			fromAnchor = builder.links.anchorPointer(edge.Location.Path, edge.Location.Line, edge.Location.Column)
		}
		rows = append(rows, pageConnection{Native: true, EvidenceID: edge.RelationID + "\x00" + edge.ToSubjectID,
			Label:    fromName + " " + strings.ReplaceAll(string(edge.RelationKind), "_", " ") + " " + toName,
			Possible: edge.Resolution != programindex.ResolutionExact, FromSource: fromAnchor, ToSource: toAnchor})
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
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Arrow != result[j].Arrow {
			return result[i].Arrow == "→"
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
