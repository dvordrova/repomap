package report

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/targetoutcome"
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
	mapNodeWidth  = 196.0
	mapNodeHeight = 58.0
	// mapLaneGap is wide enough to write on. An arrow with no words on it is
	// a line between two boxes and a reader has to guess what it means, so
	// the gutter between lanes carries the connection's own words.
	mapLaneGap = 136.0
	// A lane too long for one column is broken into further columns, and the
	// gap between them has to fit an arrow and its words too — narrower than
	// a lane boundary, wide enough to draw in.
	mapColumnGap      = 104.0
	mapNodeGap        = 10.0
	mapPadding        = 12.0
	mapLaneLabelSpace = 26.0
	// mapMaxNodesPerColumn wraps a long lane into further columns so the whole
	// map stays inside about one screen instead of becoming a list again.
	mapMaxNodesPerColumn = 7
	// mapTitleBudget is how many characters fit on one node line at the node
	// width above, and a title may take two of them. A group's name is what
	// the map is for, so it wraps rather than being cut; only a name too long
	// for both lines is cut, and the whole of it stays in the node's tooltip
	// and on the group card the node links to.
	mapTitleBudget = 26
	mapTitleLines  = 2
	// mapBarWidth is how wide a full-width membership bar is drawn.
	mapBarWidth = 174.0
	// An edge between two groups in one column loops back into the side it
	// left from, so its head points at the box it arrives at. Depth grows
	// with the vertical distance it covers, up to a bound, and repeats in one
	// gutter are pushed apart so two loops are two loops.
	mapLoopMinDepth = 26.0
	mapLoopMaxDepth = 74.0
	mapLoopPerRow   = 0.22
	mapLoopSpread   = 9.0
	mapLoopVariants = 3
	// An edge that skips over a column travels along a band under the map
	// instead of passing behind the boxes in between, where it used to
	// disappear and re-emerge as two unrelated stubs.
	mapBandTop  = 16.0
	mapBandStep = 9.0
	mapBandTurn = 14.0
	// mapEdgeLabelBudget is how many characters fit on one label line inside
	// a lane gutter, and a label may take two of them.
	mapEdgeLabelBudget = 22
	mapEdgeLabelLines  = 2
	mapEdgeLabelEm     = 5.3
	mapEdgeLabelLine   = 11.0
	// mapEdgeLabelMinRoom and mapEdgeLabelMinBudget are the least horizontal
	// space, and the least characters in it, worth writing a label in. Below
	// them the words would be cut to nothing, and the whole label is on the
	// arrow's tooltip anyway.
	mapEdgeLabelMinRoom   = 40.0
	mapEdgeLabelMinBudget = 6
)

// mapLabelOffsets are the vertical nudges a label tries, in order, when the
// place it wants is already written on.
var mapLabelOffsets = []float64{0, -14, 14, -27, 27, -40, 40}

type pageMap struct {
	Width      float64
	Height     float64
	Lanes      []pageMapLane
	Nodes      []pageMapNode
	Edges      []pageMapEdge
	LargestPct int
	// Subjects and Grouped say how much of the target the map accounts for.
	// Grouping is a sparse cover by design, so a map of four boxes over a
	// thousand symbols must not read as the whole target.
	Subjects int
	Grouped  int
}

type pageMapLane struct {
	Label string
	X     float64
	Width float64
}

type pageMapNode struct {
	ID      string
	Href    string
	Title   []string
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
	// Steps names the main-flow step numbers that pass through this group, so
	// the map and the flow below it describe the same journey. StepX and StepY
	// are absolute because a text anchored at its end ignores dx.
	Steps string
	StepX float64
	StepY float64
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
	// Lines is the label written beside the edge. It is empty when there is
	// no room for it without covering another one; the whole label is on the
	// edge's tooltip either way.
	Lines  []string
	LabelX float64
	LabelY float64
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
	result := &pageMap{Subjects: subjects}
	if subjects > 0 {
		result.LargestPct = largest * 100 / subjects
	}
	grouped := make(map[string]struct{})
	for _, group := range index.Groups {
		for _, member := range group.MemberSubjectIDs {
			grouped[member] = struct{}{}
		}
	}
	result.Grouped = len(grouped)
	positions := make(map[string]*pageMapNode)
	neighbours := builder.mapNeighbours(*index)
	steps := builder.flowStepsByGroup(section, *index)

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
			node.Steps = steps[group.ID]
			node.StepX = node.X + node.Width - 10
			node.StepY = node.Y + mapNodeHeight - 9
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
	edges, band, rightmost := mapEdges(*index, positions, result.Height)
	result.Edges = edges
	result.Height += band + mapPadding + 14
	if reach := rightmost + mapPadding; reach > result.Width {
		result.Width = reach
	}
	return result
}

// mapTitle breaks a title over the node's title lines, on word boundaries
// where it can. Only a name too long for every line is cut.
func mapTitle(title string) []string {
	words := strings.Fields(title)
	lines := make([]string, 0, mapTitleLines)
	current := ""
	for _, word := range words {
		candidate := word
		if current != "" {
			candidate = current + " " + word
		}
		if len([]rune(candidate)) <= mapTitleBudget {
			current = candidate
			continue
		}
		if current != "" {
			lines = append(lines, current)
			current = ""
		}
		if len(lines) == mapTitleLines {
			break
		}
		current = word
	}
	if current != "" && len(lines) < mapTitleLines {
		lines = append(lines, current)
	}
	if len(lines) == 0 {
		return []string{cutTitle(title)}
	}
	if last := len(lines) - 1; len([]rune(lines[last])) > mapTitleBudget {
		lines[last] = cutTitle(lines[last])
	}
	if joined := strings.Join(lines, " "); joined != strings.Join(words, " ") {
		lines[len(lines)-1] = cutTitle(lines[len(lines)-1] + " " + "\u2026")
	}
	return lines
}

// wrapToLines fills at most lines lines of at most budget characters each,
// breaking on words where it can. Text that does not fit ends in an ellipsis
// rather than being dropped silently.
func wrapToLines(text string, budget, lines int) []string {
	if budget < mapEdgeLabelMinBudget || lines < 1 {
		return nil
	}
	words := strings.Fields(text)
	result := make([]string, 0, lines)
	current := ""
	for position, word := range words {
		candidate := word
		if current != "" {
			candidate = current + " " + word
		}
		if len([]rune(candidate)) <= budget {
			current = candidate
			continue
		}
		if current != "" {
			result = append(result, current)
			current = ""
		}
		if len(result) == lines {
			return withEllipsis(result, budget)
		}
		if len([]rune(word)) > budget {
			word = string([]rune(word)[:budget-1]) + "…"
			result = append(result, word)
			if len(result) == lines || position < len(words)-1 {
				return withEllipsis(result, budget)
			}
			continue
		}
		current = word
	}
	if current != "" && len(result) < lines {
		result = append(result, current)
		current = ""
	}
	if current != "" {
		return withEllipsis(result, budget)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func withEllipsis(lines []string, budget int) []string {
	if len(lines) == 0 {
		return nil
	}
	last := len(lines) - 1
	if strings.HasSuffix(lines[last], "…") {
		return lines
	}
	runes := []rune(lines[last])
	if len(runes)+1 > budget {
		runes = runes[:budget-1]
	}
	lines[last] = strings.TrimRight(string(runes), " ") + "…"
	return lines
}

func cutTitle(value string) string {
	runes := []rune(value)
	if len(runes) <= mapTitleBudget {
		return value
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

// mapEdges draws one arrow per pair of groups that are connected, and writes
// what the connection is beside it where there is room. Cross-target
// connections are deliberately absent: the other end has no position on this
// map. Two connections between the same pair are one arrow carrying both
// labels, because two identical curves drawn on top of each other are one
// curve that reads as a thicker line.
func mapEdges(index groupindex.Index, nodes map[string]*pageMapNode, bottom float64) ([]pageMapEdge, float64, float64) {
	type pair struct{ from, to string }
	labels := make(map[pair][]string)
	possible := make(map[pair]bool)
	exact := make(map[pair]bool)
	var order []pair
	for _, connection := range index.Connections {
		if connection.From.TargetID != index.Target.ID || connection.To.TargetID != index.Target.ID {
			continue
		}
		from, fromKnown := nodes[mapNodeID(connection.From.GroupID)]
		to, toKnown := nodes[mapNodeID(connection.To.GroupID)]
		if !fromKnown || !toKnown || from == to {
			continue
		}
		key := pair{from.ID, to.ID}
		if _, seen := labels[key]; !seen {
			order = append(order, key)
			labels[key] = nil
		}
		if !containsString(labels[key], connection.Label) && connection.Label != "" {
			labels[key] = append(labels[key], connection.Label)
		}
		if connection.SupportResolution == programindex.PatternValuePossible {
			possible[key] = true
		} else {
			exact[key] = true
		}
	}
	router := newMapEdgeRouter(nodes, bottom)
	result := make([]pageMapEdge, 0, len(order))
	for _, key := range order {
		from, to := nodes[key.from], nodes[key.to]
		path, labelX, labelY, room, minLeft := router.route(from, to)
		edge := pageMapEdge{
			Path: path, From: key.from, To: key.to,
			Label: strings.Join(labels[key], " · "),
			// Dashed only when nothing about this pair is exact, so one
			// uncertain call among several cannot make the whole arrow
			// look uncertain.
			Possible: possible[key] && !exact[key],
		}
		if lines, atX, atY := router.placeLabel(edge.Label, labelX, labelY, room, minLeft); lines != nil {
			edge.Lines, edge.LabelX, edge.LabelY = lines, atX, atY
		}
		result = append(result, edge)
	}
	return result, router.extraHeight(), router.rightmost
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

// mapEdgeRouter keeps an arrow out of the boxes. Three shapes exist, and each
// was drawn wrong before: an edge inside one column left and arrived on the
// same side with its head pointing away from the box it arrived at; an edge
// that skipped a column passed behind the boxes in between and read as two
// unrelated stubs; and neither carried a word of what it meant.
type mapEdgeRouter struct {
	columns []float64
	bandY   float64
	// bandEnd is the rightmost x each band row is already occupied to, so two
	// detours share a row only when their spans do not overlap.
	bandEnd []float64
	loops   map[float64]int
	placed  []mapLabelBox
	// rightmost is how far right anything drawn here reaches. A loop beside
	// the last column, and the words on it, live outside the columns, and a
	// picture that ends at the last box cuts them off.
	rightmost float64
}

type mapLabelBox struct{ left, right, top, bottom float64 }

func newMapEdgeRouter(nodes map[string]*pageMapNode, bottom float64) *mapEdgeRouter {
	seen := make(map[float64]struct{}, len(nodes))
	router := &mapEdgeRouter{bandY: bottom + mapBandTop, loops: make(map[float64]int)}
	for _, node := range nodes {
		if _, repeated := seen[node.X]; repeated {
			continue
		}
		seen[node.X] = struct{}{}
		router.columns = append(router.columns, node.X)
	}
	sort.Float64s(router.columns)
	return router
}

func (router *mapEdgeRouter) extraHeight() float64 {
	if len(router.bandEnd) == 0 {
		return 0
	}
	return mapBandTop + float64(len(router.bandEnd))*mapBandStep
}

func (router *mapEdgeRouter) column(x float64) int {
	for index, candidate := range router.columns {
		if candidate == x {
			return index
		}
	}
	return -1
}

// gutter is the empty vertical strip after column index, as a centre and a
// width. The strip after the last column is as wide as a lane gap so an edge
// leaving the right-hand lane still has somewhere to go.
func (router *mapEdgeRouter) gutter(index int) (centre, width float64) {
	right := router.columns[index] + mapNodeWidth
	if index+1 >= len(router.columns) {
		return right + mapLaneGap/2, mapLaneGap
	}
	next := router.columns[index+1]
	return (right + next) / 2, next - right
}

// route returns the path, where a label for it would sit, and how much
// horizontal room that label has.
func (router *mapEdgeRouter) route(from, to *pageMapNode) (path string, labelX, labelY, room, minLeft float64) {
	fromColumn, toColumn := router.column(from.X), router.column(to.X)
	startY, endY := from.Y+from.Height/2, to.Y+to.Height/2
	switch {
	case fromColumn == toColumn:
		// A loop's words start clear of the box it left, or the box is drawn
		// over the first half of them.
		x, y, width := router.loopLabel(from, startY, endY)
		return router.loopPath(from, fromColumn, startY, endY), x, y, width, from.X + from.Width + 3
	case abs(toColumn-fromColumn) >= 2:
		path, x, y, width := router.detour(from, to, fromColumn, toColumn, startY, endY)
		return path, x, y, width, 0
	default:
		path, x, y, width := router.direct(from, to, fromColumn, toColumn, startY, endY)
		return path, x, y, width, 0
	}
}

// direct joins two neighbouring columns. Forward it leaves the right side and
// arrives at the left; backwards it leaves the left side and arrives at the
// right, which is the same curve read the other way round.
func (router *mapEdgeRouter) direct(
	from, to *pageMapNode,
	fromColumn, toColumn int,
	startY, endY float64,
) (string, float64, float64, float64) {
	startX, endX := from.X+from.Width, to.X
	gutterIndex := fromColumn
	if toColumn < fromColumn {
		startX, endX = from.X, to.X+to.Width
		gutterIndex = toColumn
	}
	// The curve bends by half the horizontal distance it covers and no more:
	// a fixed bend across a narrow gutter overshoots the box it is arriving
	// at and comes back, which reads as a mistake.
	bend := (endX - startX) / 2
	path := fmt.Sprintf("M%.1f %.1f C%.1f %.1f %.1f %.1f %.1f %.1f",
		startX, startY, startX+bend, startY, endX-bend, endY, endX, endY)
	centre, width := router.gutter(gutterIndex)
	return path, centre, (startY + endY) / 2, width
}

// loopPath connects two groups in one column with an arc in the gutter beside
// it, arriving back on the side it left so its head points into the box.
func (router *mapEdgeRouter) loopPath(
	from *pageMapNode,
	column int,
	startY, endY float64,
) string {
	side := from.X + from.Width
	depth := router.loopDepth(from, startY, endY)
	router.loops[from.X]++
	router.reach(side + depth)
	return fmt.Sprintf("M%.1f %.1f C%.1f %.1f %.1f %.1f %.1f %.1f",
		side, startY, side+depth, startY, side+depth, endY, side, endY)
}

func (router *mapEdgeRouter) loopDepth(from *pageMapNode, startY, endY float64) float64 {
	depth := mapLoopMinDepth + abs(int(endY-startY))*mapLoopPerRow
	if depth > mapLoopMaxDepth {
		depth = mapLoopMaxDepth
	}
	return depth + float64(router.loops[from.X]%mapLoopVariants)*mapLoopSpread
}

// loopLabel puts a loop's words at its own apex rather than in the middle of
// the gutter, so they do not queue up behind the labels of the edges that
// cross that gutter on their way to the next column.
func (router *mapEdgeRouter) loopLabel(
	from *pageMapNode,
	startY, endY float64,
) (x, y, room float64) {
	side := from.X + from.Width
	depth := router.loopDepth(from, startY, endY)
	_, width := router.gutter(router.column(from.X))
	return side + depth*0.6, (startY + endY) / 2, width
}

// detour sends an edge that skips a column along a band under the map. Rows in
// the band are reused whenever two detours cover different spans.
func (router *mapEdgeRouter) detour(
	from, to *pageMapNode,
	fromColumn, toColumn int,
	startY, endY float64,
) (string, float64, float64, float64) {
	startX, endX := from.X+from.Width, to.X
	firstGutter, lastGutter := fromColumn, toColumn-1
	if toColumn < fromColumn {
		startX, endX = from.X, to.X+to.Width
		firstGutter, lastGutter = fromColumn-1, toColumn
	}
	first, _ := router.gutter(firstGutter)
	last, _ := router.gutter(lastGutter)
	left, right := first, last
	if left > right {
		left, right = right, left
	}
	row := router.bandRow(left, right)
	bandY := router.bandY + float64(row)*mapBandStep
	turn := mapBandTurn
	if first > last {
		turn = -turn
	}
	path := fmt.Sprintf(
		"M%.1f %.1f C%.1f %.1f %.1f %.1f %.1f %.1f L%.1f %.1f C%.1f %.1f %.1f %.1f %.1f %.1f",
		startX, startY,
		first, startY, first, bandY, first+turn, bandY,
		last-turn, bandY,
		last, bandY, last, endY, endX, endY,
	)
	return path, (first + last) / 2, bandY - 6, right - left
}

func (router *mapEdgeRouter) bandRow(left, right float64) int {
	for row, end := range router.bandEnd {
		if left > end {
			router.bandEnd[row] = right
			return row
		}
	}
	router.bandEnd = append(router.bandEnd, right)
	return len(router.bandEnd) - 1
}

// placeLabel writes the connection's own words beside the arrow when they fit
// in the room it has and do not cover a label already written. A label that
// cannot be placed is not shrunk to nothing: it stays on the tooltip whole.
func (router *mapEdgeRouter) placeLabel(label string, x, y, room, minLeft float64) ([]string, float64, float64) {
	if label == "" || room < mapEdgeLabelMinRoom {
		return nil, x, y
	}
	budget := int((room - 8) / mapEdgeLabelEm)
	if budget > mapEdgeLabelBudget {
		budget = mapEdgeLabelBudget
	}
	lines := wrapToLines(label, budget, mapEdgeLabelLines)
	if len(lines) == 0 {
		return nil, x, y
	}
	widest := 0
	for _, line := range lines {
		if count := len([]rune(line)); count > widest {
			widest = count
		}
	}
	width := float64(widest)*mapEdgeLabelEm + 6
	height := float64(len(lines))*mapEdgeLabelLine + 2
	if left := x - width/2; minLeft > 0 && left < minLeft {
		x = minLeft + width/2
	}
	// A label that would land on one already written moves off its edge a
	// little rather than disappearing. Only when every offset is taken does
	// the label give up and stay on the tooltip alone.
	for _, offset := range mapLabelOffsets {
		box := mapLabelBox{
			left: x - width/2, right: x + width/2,
			top: y + offset - height/2, bottom: y + offset + height/2,
		}
		if router.occupied(box) {
			continue
		}
		router.placed = append(router.placed, box)
		router.reach(box.right)
		return lines, x, y + offset
	}
	return nil, x, y
}

func (router *mapEdgeRouter) reach(x float64) {
	if x > router.rightmost {
		router.rightmost = x
	}
}

func (router *mapEdgeRouter) occupied(box mapLabelBox) bool {
	for _, taken := range router.placed {
		if box.left < taken.right && taken.left < box.right &&
			box.top < taken.bottom && taken.top < box.bottom {
			return true
		}
	}
	return false
}

func abs(value int) float64 {
	if value < 0 {
		return float64(-value)
	}
	return float64(value)
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
// parts, how big is each one, and which one talks to which — before the reader
// has to join a table of portals to a list of target cards by hand. It draws
// every target the run knew about, including the ones it could not read, so
// "seventeen of twenty" is a picture and not a footnote.
const (
	repoNodeWidth  = 184.0
	repoNodeHeight = 74.0
	repoNodeGapX   = 30.0
	// A repository whose targets call each other needs room between the boxes
	// for the arrow and what it carries; one whose targets do not can pack
	// them closer and fit more on a row.
	repoCalledGapX = 122.0
	repoNodeGapY   = 22.0
	repoRowGap     = 20.0
	// repoPerRow wraps the row so twenty targets are a block a screen wide
	// rather than a strip nobody scrolls to the end of.
	repoPerRow = 5
	// With arrows to draw the row is shorter, because each gap is four times
	// as wide.
	repoCalledPerRow = 4
	repoBarWidth     = 158.0
	repoNameBudget   = 24
	repoNameLines    = 2
	// repoDetailBudget is what fits on the line under the name at the node
	// width above.
	repoDetailBudget = 30
)

type pageRepoMap struct {
	Width  float64
	Height float64
	Nodes  []pageRepoNode
	Edges  []pageRepoEdge
	// Caption says what the picture is and, when nothing calls anything, why
	// there are no arrows in it.
	Caption string
}

type pageRepoNode struct {
	Href     string
	Name     []string
	FullName string
	Detail   string
	// Note carries why a target has no page of its own. A target the run
	// could not read is still one of the repository's parts.
	Note     string
	Analyzed bool
	Members  int
	BarWidth float64
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

// repoSymbolCounts is how many symbols each target holds, which is what makes
// one box wider-barred than another. A repository where one target is ten
// times the size of the rest should look like one.
func (builder *pageBuilder) repoSymbolCounts() map[string]int {
	counts := make(map[string]int)
	for _, section := range builder.sections {
		index := builder.graphIndex(section.programTargetID)
		if index == nil {
			continue
		}
		symbols := 0
		for _, subject := range index.Subjects {
			if subject.Kind == groupindex.SubjectObject {
				symbols++
			}
		}
		counts[section.factsTargetID] = symbols
	}
	return counts
}

// buildRepoMap places every target of the repository in a wrapped grid, sized
// by how many symbols it holds, and draws one arrow per ordered pair that a
// portal connects, labelled with how many crossings that pair carries. One
// arrow per portal would redraw the table that is already on the page.
func (builder *pageBuilder) buildRepoMap(view *pageView) *pageRepoMap {
	unread := builder.unreadTargets()
	if len(builder.sections)+len(unread) < 2 {
		return nil
	}
	result := &pageRepoMap{}
	centres := make(map[string]*pageRepoNode, len(builder.sections))
	// A caller reads better to the left of what it calls, and a big part
	// before a small one, so the grid is ordered by how many other targets
	// each one reaches and then by size, with section order breaking the tie.
	ordered := append([]*pageSection(nil), builder.sections...)
	calls := builder.repoOutgoingCounts()
	symbols := builder.repoSymbolCounts()
	place := make(map[*pageSection]int, len(ordered))
	for index, section := range ordered {
		place[section] = index
	}
	sort.SliceStable(ordered, func(left, right int) bool {
		leftSection, rightSection := ordered[left], ordered[right]
		leftCalls, rightCalls := calls[leftSection.factsTargetID], calls[rightSection.factsTargetID]
		if leftCalls != rightCalls {
			return leftCalls > rightCalls
		}
		leftSize, rightSize := symbols[leftSection.factsTargetID], symbols[rightSection.factsTargetID]
		if leftSize != rightSize {
			return leftSize > rightSize
		}
		return place[leftSection] < place[rightSection]
	})
	largest := 1
	for _, count := range symbols {
		if count > largest {
			largest = count
		}
	}
	gapX, perRow := repoNodeGapX, repoPerRow
	if len(calls) > 0 {
		gapX, perRow = repoCalledGapX, repoCalledPerRow
	}
	position := 0
	appendNode := func(node pageRepoNode) {
		node.X = mapPadding + float64(position%perRow)*(repoNodeWidth+gapX)
		node.Y = repoRowGap + float64(position/perRow)*(repoNodeHeight+repoNodeGapY)
		node.Width, node.Height = repoNodeWidth, repoNodeHeight
		result.Nodes = append(result.Nodes, node)
		position++
	}
	for _, section := range ordered {
		members := symbols[section.factsTargetID]
		appendNode(pageRepoNode{
			Href: "#" + section.ID, Name: wrapToLines(section.Label, repoNameBudget, repoNameLines),
			FullName: section.Label, Detail: repoNodeDetail(section, members), Analyzed: true,
			Members: members, BarWidth: repoBarWidth * float64(members) / float64(largest),
		})
	}
	for index := range result.Nodes {
		centres[ordered[index].factsTargetID] = &result.Nodes[index]
	}
	for _, target := range unread {
		appendNode(pageRepoNode{
			Name: wrapToLines(target.name, repoNameBudget, repoNameLines), FullName: target.name,
			Detail: target.language, Note: cutToBudget(target.note, repoDetailBudget),
		})
	}
	rows := (position + perRow - 1) / perRow
	columns := position
	if columns > perRow {
		columns = perRow
	}
	result.Width = mapPadding*2 + float64(columns)*repoNodeWidth + float64(columns-1)*gapX
	result.Height = repoRowGap*2 + float64(rows)*repoNodeHeight + float64(rows-1)*repoNodeGapY
	result.Edges = builder.repoEdges(centres)
	result.Caption = repoMapCaption(len(builder.sections), len(unread), len(result.Edges))
	return result
}

func repoMapCaption(analyzed, unread, edges int) string {
	caption := "Every part of this repository, sized by how many symbols it holds."
	switch {
	case edges > 0:
		caption += " An arrow is one target calling another over HTTP."
	default:
		caption += " No target calls another over HTTP, so there are no arrows."
	}
	if unread > 0 {
		caption += fmt.Sprintf(
			" %d of %d were read; the pale ones were not, and each says why.",
			analyzed, analyzed+unread,
		)
	}
	return caption
}

// unreadTarget is a target the run knew about and could not read.
type unreadTarget struct{ name, language, note string }

func (builder *pageBuilder) unreadTargets() []unreadTarget {
	var result []unreadTarget
	for _, outcome := range builder.data.TargetOutcomePortfolio.Outcomes {
		if outcome.State == targetoutcome.StateAnalyzed {
			continue
		}
		result = append(result, unreadTarget{
			name:     outcome.DisplayName,
			language: string(outcome.Language),
			note:     "not read · " + strings.ReplaceAll(string(outcome.FailureReason), "_", " "),
		})
	}
	return result
}

// repoNodeDetail is the one line under a target's name. It has room for
// about thirty characters, so it says what the target is and how big it is,
// and leaves the root to the card below, which has a whole column for it.
func repoNodeDetail(section *pageSection, symbols int) string {
	size := fmt.Sprintf("%d %s", symbols, pluralWord(symbols, "symbol", "symbols"))
	detail := size
	if section.Language != "" {
		detail += " · " + section.Language
	}
	// The kind is the first thing to go when the line is too long: a word cut
	// to "ap…" tells the reader less than leaving it out and letting the card
	// below say "application".
	withKind := detail
	if section.Kind != "" {
		withKind += " · " + section.Kind
	}
	if len([]rune(withKind)) <= repoDetailBudget {
		return withKind
	}
	return cutToBudget(detail, repoDetailBudget)
}

func cutToBudget(value string, budget int) string {
	runes := []rune(value)
	if len(runes) <= budget {
		return value
	}
	return strings.TrimRight(string(runes[:budget-1]), " ·") + "…"
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
		bend := (endX - startX) / 2
		result = append(result, pageRepoEdge{
			Path: fmt.Sprintf("M%.1f %.1f C%.1f %.1f %.1f %.1f %.1f %.1f",
				startX, startY, startX+bend, startY, endX-bend, endY, endX, endY),
			Label: fmt.Sprintf("%d HTTP %s", counts[key],
				map[bool]string{true: "calls", false: "call"}[counts[key] != 1]),
			LabelX: (startX + endX) / 2, LabelY: (startY+endY)/2 - 7,
			// Dashed only when nothing about this pair is exact; one uncertain
			// crossing among several must not make the whole link look uncertain.
			Possible: exact[key] == 0,
		})
	}
	return result
}

// flowStepsByGroup labels each group with the main-flow step numbers that pass
// through it. The flow below the map and the map itself then describe one
// journey rather than two, and a reader can see where it enters and leaves
// before reading a word of it.
func (builder *pageBuilder) flowStepsByGroup(
	section *pageSection,
	index groupindex.Index,
) map[string]string {
	orient := builder.data.Orientation
	if orient == nil || len(orient.MainFlow.Steps) == 0 {
		return nil
	}
	owner := make(map[string][]string, len(index.Subjects))
	for _, group := range index.Groups {
		for _, member := range group.MemberSubjectIDs {
			owner[member] = append(owner[member], group.ID)
		}
	}
	result := make(map[string]string)
	seen := make(map[string]map[string]struct{})
	for position, step := range orient.MainFlow.Steps {
		subjectID := step.SubjectID
		if subjectID == "" && step.FactID != "" {
			if fact, known := builder.factsByID[step.FactID]; known {
				subjectID = fact.ObjectID
			}
		}
		if subjectID == "" {
			continue
		}
		ordinal := strconv.Itoa(position + 1)
		for _, groupID := range owner[subjectID] {
			if seen[groupID] == nil {
				seen[groupID] = make(map[string]struct{})
			}
			if _, repeated := seen[groupID][ordinal]; repeated {
				continue
			}
			seen[groupID][ordinal] = struct{}{}
			if result[groupID] == "" {
				result[groupID] = ordinal
				continue
			}
			result[groupID] += "," + ordinal
		}
	}
	return result
}
