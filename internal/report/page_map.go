package report

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// The map is the first thing on a target page: one picture of what this target
// is made of and how its parts reach each other. It exists because a column of
// equally weighted headings cannot show that one group holds a third of the
// target while another holds three symbols.
//
// The layout is computed here, in Go, and shipped as plain SVG. With no
// scripting the map still reads and every node is a link to the group it
// names; scripting only adds the neighbourhood preview.
const (
	mapNodeWidth      = 196.0
	mapNodeHeight     = 46.0
	mapLaneGap        = 104.0
	mapColumnGap      = 16.0
	mapNodeGap        = 10.0
	mapPadding        = 12.0
	mapLaneLabelSpace = 26.0
	// mapMaxNodesPerColumn wraps a long lane into further columns so the whole
	// map stays inside about one screen instead of becoming a list again.
	mapMaxNodesPerColumn = 7
	// mapTitleBudget is how many characters fit on one node line at the node
	// width above. A longer title is cut on the node and kept whole in its
	// tooltip and in the group card the node links to.
	mapTitleBudget = 26
	// Two drawing constants that keep a nearly horizontal curve from
	// collapsing into a straight line on top of another one.
	mapMinEdgeBend  = 24.0
	mapFlatEdgeBend = 40.0
	// mapBarWidth is how wide a full-width membership bar is drawn.
	mapBarWidth = 174.0
)

type pageMap struct {
	Width      float64
	Height     float64
	Lanes      []pageMapLane
	Nodes      []pageMapNode
	Edges      []pageMapEdge
	LargestPct int
}

type pageMapLane struct {
	Label string
	X     float64
	Width float64
}

type pageMapNode struct {
	ID      string
	Href    string
	Title   string
	Summary string
	Lane    string
	// Members is how many subjects this group holds, and Share how much of the
	// target that is. The node's height carries the same number, so a bucket
	// looks like a bucket before any of it is read.
	Members   int
	Share     int
	X         float64
	Y         float64
	Width     float64
	Height    float64
	BarWidth  float64
	FullTitle string
	// Neighbours lists the node ids one connection away, in both directions,
	// so the preview needs no graph traversal in the browser.
	Neighbours string
	Degree     int
	// Outside counts connections this group has to another target. They are
	// not drawn: a cross-target arrow on this map would claim a geometry that
	// belongs to the other target's page.
	Outside int
}

type pageMapEdge struct {
	Path     string
	From     string
	To       string
	Label    string
	Possible bool
}

// buildMap lays out one target's groups in three columns — what reaches in,
// what the target is, what it reaches out to — and connects them.
func (builder *pageBuilder) buildMap(section *pageSection) *pageMap {
	index := builder.graphIndex(section.programTargetID)
	if index == nil || len(index.Groups) == 0 {
		return nil
	}
	subjects := len(index.Subjects)
	lanes := []struct {
		lane  groupindex.Lane
		label string
	}{
		{groupindex.LaneTriggers, "Reached from"},
		{groupindex.LaneCore, "What it is"},
		{groupindex.LaneDependencies, "Reaches out to"},
	}

	largest := 0
	for _, group := range index.Groups {
		if len(group.MemberSubjectIDs) > largest {
			largest = len(group.MemberSubjectIDs)
		}
	}
	result := &pageMap{}
	if subjects > 0 {
		result.LargestPct = largest * 100 / subjects
	}
	positions := make(map[string]*pageMapNode)
	neighbours := builder.mapNeighbours(*index)

	columnX := mapPadding
	for _, lane := range lanes {
		members := laneGroups(*index, lane.lane)
		if len(members) == 0 {
			continue
		}
		columns := (len(members) + mapMaxNodesPerColumn - 1) / mapMaxNodesPerColumn
		perColumn := (len(members) + columns - 1) / columns
		laneWidth := float64(columns)*mapNodeWidth + float64(columns-1)*mapColumnGap
		result.Lanes = append(result.Lanes, pageMapLane{
			Label: lane.label, X: columnX, Width: laneWidth,
		})
		for position, group := range members {
			column := position / perColumn
			row := position % perColumn
			node := pageMapNode{
				ID: mapNodeID(group.ID), Href: "#" + groupAnchorID(section.ID, group.ID),
				Title: mapTitle(group.Title), FullTitle: group.Title,
				Summary: group.Summary, Lane: string(lane.lane),
				Members: len(group.MemberSubjectIDs),
				X:       columnX + float64(column)*(mapNodeWidth+mapColumnGap),
				Y:       mapPadding + mapLaneLabelSpace + float64(row)*(mapNodeHeight+mapNodeGap),
				Width:   mapNodeWidth, Height: mapNodeHeight,
			}
			if subjects > 0 {
				node.Share = node.Members * 100 / subjects
			}
			node.BarWidth = mapBarWidth * float64(node.Members) / float64(largest)
			local, outside := neighbours[group.ID], 0
			for _, other := range local {
				if other == "" {
					outside++
				}
			}
			node.Outside = outside
			node.Neighbours = strings.Join(mapNodeIDs(local), " ")
			node.Degree = len(local) - outside
			result.Nodes = append(result.Nodes, node)
			bottom := node.Y + node.Height
			if bottom > result.Height {
				result.Height = bottom
			}
		}
		columnX += laneWidth + mapLaneGap
	}
	for position := range result.Nodes {
		positions[result.Nodes[position].ID] = &result.Nodes[position]
	}
	result.Width = columnX - mapLaneGap + mapPadding
	result.Height += mapPadding + 14
	result.Edges = mapEdges(*index, positions)
	return result
}

// mapTitle cuts a title to what fits on one node line. The whole title stays
// in the node's tooltip and on the group card the node links to, so nothing
// is lost, only shortened.
func mapTitle(title string) string {
	runes := []rune(title)
	if len(runes) <= mapTitleBudget {
		return title
	}
	return strings.TrimRight(string(runes[:mapTitleBudget-1]), " ") + "\u2026"
}

func laneGroups(index groupindex.Index, lane groupindex.Lane) []groupindex.Group {
	var result []groupindex.Group
	for _, group := range index.Groups {
		if group.Lane == lane {
			result = append(result, group)
		}
	}
	return result
}

// mapNeighbours returns, per group, the group ids one connection away. An
// empty string stands for a connection whose other end is in another target.
func (builder *pageBuilder) mapNeighbours(index groupindex.Index) map[string][]string {
	result := make(map[string][]string, len(index.Groups))
	add := func(group, other string) {
		for _, existing := range result[group] {
			if existing == other {
				return
			}
		}
		result[group] = append(result[group], other)
	}
	for _, connection := range index.Connections {
		fromLocal := connection.From.TargetID == index.Target.ID
		toLocal := connection.To.TargetID == index.Target.ID
		switch {
		case fromLocal && toLocal:
			add(connection.From.GroupID, connection.To.GroupID)
			add(connection.To.GroupID, connection.From.GroupID)
		case fromLocal:
			add(connection.From.GroupID, "")
		case toLocal:
			add(connection.To.GroupID, "")
		}
	}
	for group := range result {
		sort.Strings(result[group])
	}
	return result
}

func mapNodeIDs(groupIDs []string) []string {
	result := make([]string, 0, len(groupIDs))
	for _, id := range groupIDs {
		if id != "" {
			result = append(result, mapNodeID(id))
		}
	}
	return result
}

// mapEdges draws one curve per local connection. Cross-target connections are
// deliberately absent: the other end has no position on this map.
func mapEdges(index groupindex.Index, nodes map[string]*pageMapNode) []pageMapEdge {
	var result []pageMapEdge
	for _, connection := range index.Connections {
		if connection.From.TargetID != index.Target.ID || connection.To.TargetID != index.Target.ID {
			continue
		}
		from, fromKnown := nodes[mapNodeID(connection.From.GroupID)]
		to, toKnown := nodes[mapNodeID(connection.To.GroupID)]
		if !fromKnown || !toKnown || from == to {
			continue
		}
		result = append(result, pageMapEdge{
			Path: mapEdgePath(from, to), From: from.ID, To: to.ID,
			Label:    connection.Label,
			Possible: connection.SupportResolution == programindex.PatternValuePossible,
		})
	}
	return result
}

// mapEdgePath leaves the right side of one node and arrives at the left side
// of the other, bending horizontally so parallel edges stay distinguishable.
// A backwards edge leaves and arrives on the same sides, which reads as the
// return direction it is.
func mapEdgePath(from, to *pageMapNode) string {
	startX, startY := from.X+from.Width, from.Y+from.Height/2
	endX, endY := to.X, to.Y+to.Height/2
	if to.X < from.X {
		startX, endX = from.X, to.X+to.Width
	} else if to.X == from.X {
		startX, endX = from.X+from.Width, to.X+to.Width
	}
	bend := (endX - startX) / 2
	if bend < mapMinEdgeBend && bend > -mapMinEdgeBend {
		bend = mapFlatEdgeBend
	}
	return fmt.Sprintf("M%.1f %.1f C%.1f %.1f %.1f %.1f %.1f %.1f",
		startX, startY, startX+bend, startY, endX-bend, endY, endX, endY)
}

func mapNodeID(groupID string) string { return "n-" + safeIDFragment(groupID) }

// groupAnchorID names the group block a map node links to. It is scoped by
// section so two targets can hold groups with the same identity.
func groupAnchorID(sectionID, groupID string) string {
	return sectionID + "-g-" + safeIDFragment(groupID)
}

// safeIDFragment keeps only characters that are safe in an HTML id and a URL
// fragment, so an opaque group identity can address a page element.
func safeIDFragment(value string) string {
	var builder strings.Builder
	for _, symbol := range value {
		switch {
		case symbol >= 'a' && symbol <= 'z', symbol >= 'A' && symbol <= 'Z',
			symbol >= '0' && symbol <= '9', symbol == '-', symbol == '_':
			builder.WriteRune(symbol)
		default:
			builder.WriteByte('-')
		}
	}
	return builder.String()
}

// The repository map answers the first question on the page — what are the
// parts and which one talks to which — before the reader has to join a table
// of portals to a list of target cards by hand.
const (
	repoNodeWidth  = 208.0
	repoNodeHeight = 62.0
	repoNodeGap    = 118.0
	repoRowGap     = 26.0
)

type pageRepoMap struct {
	Width  float64
	Height float64
	Nodes  []pageRepoNode
	Edges  []pageRepoEdge
}

type pageRepoNode struct {
	Href     string
	Name     string
	Language string
	Detail   string
	Analyzed bool
	X        float64
	Y        float64
	Width    float64
	Height   float64
}

type pageRepoEdge struct {
	Path  string
	Label string
	LabelX,
	LabelY float64
	Possible bool
}

// repoOutgoingCounts is how many other targets each target calls.
func (builder *pageBuilder) repoOutgoingCounts() map[string]int {
	counts := make(map[string]int)
	if builder.data.Facts == nil {
		return counts
	}
	seen := make(map[[2]string]struct{})
	for _, portal := range builder.data.Facts.OfKind(facts.KindPortal) {
		if len(portal.Refs) < 2 {
			continue
		}
		call, callKnown := builder.factsByID[portal.Refs[0]]
		route, routeKnown := builder.factsByID[portal.Refs[1]]
		if !callKnown || !routeKnown || call.TargetID == route.TargetID {
			continue
		}
		key := [2]string{call.TargetID, route.TargetID}
		if _, repeated := seen[key]; repeated {
			continue
		}
		seen[key] = struct{}{}
		counts[call.TargetID]++
	}
	return counts
}

// buildRepoMap places every analyzed target in one row and draws one arrow per
// ordered pair of targets that a portal connects, labelled with how many
// crossings that pair carries. One arrow per portal would redraw the table
// that is already on the page.
func (builder *pageBuilder) buildRepoMap(view *pageView) *pageRepoMap {
	if len(builder.sections) < 2 {
		return nil
	}
	result := &pageRepoMap{Height: repoRowGap + repoNodeHeight + repoRowGap}
	centres := make(map[string]*pageRepoNode, len(builder.sections))
	// A caller reads better to the left of what it calls, so the row is
	// ordered by how many other targets each one reaches, with section order
	// breaking the tie.
	ordered := append([]*pageSection(nil), builder.sections...)
	calls := builder.repoOutgoingCounts()
	place := make(map[*pageSection]int, len(ordered))
	for index, section := range ordered {
		place[section] = index
	}
	sort.SliceStable(ordered, func(left, right int) bool {
		leftCalls := calls[ordered[left].factsTargetID]
		rightCalls := calls[ordered[right].factsTargetID]
		if leftCalls != rightCalls {
			return leftCalls > rightCalls
		}
		return place[ordered[left]] < place[ordered[right]]
	})
	x := mapPadding
	for _, section := range ordered {
		node := pageRepoNode{
			Href: "#" + section.ID, Name: section.Label, Language: section.Language,
			Detail: repoNodeDetail(section), Analyzed: true,
			X: x, Y: repoRowGap, Width: repoNodeWidth, Height: repoNodeHeight,
		}
		result.Nodes = append(result.Nodes, node)
		x += repoNodeWidth + repoNodeGap
	}
	for index := range result.Nodes {
		centres[ordered[index].factsTargetID] = &result.Nodes[index]
	}
	result.Width = x - repoNodeGap + mapPadding
	result.Edges = builder.repoEdges(centres)
	if len(result.Edges) == 0 {
		// Boxes with no arrows say nothing the target cards above have not
		// already said. A picture that adds nothing is not a picture.
		return nil
	}
	return result
}

func repoNodeDetail(section *pageSection) string {
	detail := section.Language
	if section.Kind != "" {
		detail += " · " + section.Kind
	}
	if section.Root != "" {
		detail += " · " + section.Root
	}
	return detail
}

// repoEdges counts the portals between each ordered pair of targets.
func (builder *pageBuilder) repoEdges(nodes map[string]*pageRepoNode) []pageRepoEdge {
	if builder.data.Facts == nil {
		return nil
	}
	type pair struct{ from, to string }
	counts := make(map[pair]int)
	exact := make(map[pair]int)
	var order []pair
	for _, portal := range builder.data.Facts.OfKind(facts.KindPortal) {
		if len(portal.Refs) < 2 {
			continue
		}
		call, callKnown := builder.factsByID[portal.Refs[0]]
		route, routeKnown := builder.factsByID[portal.Refs[1]]
		if !callKnown || !routeKnown || call.TargetID == route.TargetID {
			continue
		}
		key := pair{call.TargetID, route.TargetID}
		if counts[key] == 0 {
			order = append(order, key)
		}
		counts[key]++
		if portal.Resolution != facts.ResolutionPossible {
			exact[key]++
		}
	}
	var result []pageRepoEdge
	for _, key := range order {
		from, fromKnown := nodes[key.from]
		to, toKnown := nodes[key.to]
		if !fromKnown || !toKnown {
			continue
		}
		startX, startY := from.X+from.Width, from.Y+from.Height/2
		endX, endY := to.X, to.Y+to.Height/2
		if to.X < from.X {
			startX, endX = from.X, to.X+to.Width
		}
		result = append(result, pageRepoEdge{
			Path: fmt.Sprintf("M%.1f %.1f L%.1f %.1f", startX, startY, endX, endY),
			Label: fmt.Sprintf("%d HTTP %s", counts[key],
				map[bool]string{true: "calls", false: "call"}[counts[key] != 1]),
			LabelX: (startX + endX) / 2, LabelY: startY - 8,
			// Dashed only when nothing about this pair is exact; one uncertain
			// crossing among several must not make the whole link look uncertain.
			Possible: exact[key] == 0,
		})
	}
	return result
}
