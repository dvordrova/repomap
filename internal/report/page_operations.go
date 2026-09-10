package report

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// Reuse a declaration's accepted alias only when an operation repeats its
// native name. A distinct action label or command/path has its own meaning.
func (builder *pageBuilder) operationDisplayName(operation groupindex.Operation) string {
	if operation.Source != "model" || operation.Kind == "command" || operation.Kind == "request" {
		return operation.Name
	}
	ref, ok := builder.subjects[operation.SubjectID]
	if !ok || ref.subject.Object == nil || ref.subject.Interpretation == nil ||
		operation.Name != ref.subject.Object.Name {
		return operation.Name
	}
	alias := ref.subject.Interpretation.Alias
	if alias == "" || alias == operation.Name {
		return operation.Name
	}
	return alias + " (" + operation.Name + ")"
}

// Operations are interpretations on existing subjects. Paths use native call
// relations only; a box's imports never become the operation's execution path.
func (builder *pageBuilder) buildOperationMap(section *pageSection, index *groupindex.Index) *pageMap {
	result := &pageMap{MarkerID: "map-arrow-" + section.ID, Subjects: len(index.Subjects), Operations: len(index.Operations) > 0}
	groupOf := make(map[string]string)
	groups := make(map[string]groupindex.Group)
	for _, group := range index.Groups {
		groups[group.ID] = group
		for _, id := range group.MemberSubjectIDs {
			groupOf[id] = group.ID
		}
	}
	result.Grouped = len(groupOf)
	// A native index may include code reached in another module. Join by its
	// exact source location, since object IDs are scoped to their target.
	foreignNodes := make(map[string]pageMapNode)
	owners := make(map[string]string)
	foreign := append([]groupindex.Index(nil), builder.indexes...)
	sort.SliceStable(foreign, func(i, j int) bool { return foreign[i].Target.Kind == "library" && foreign[j].Target.Kind != "library" })
	for _, other := range foreign {
		otherSection := builder.byProgram[other.Target.ID]
		if other.Target.ID == index.Target.ID || otherSection == nil {
			continue
		}
		locations := make(map[string]string)
		for _, subject := range other.Subjects {
			if subject.Object != nil && subject.Object.Location != nil {
				locations[subject.ID] = operationLocationKey(*subject.Object.Location)
			}
		}
		for _, group := range other.Groups {
			foreignNodes[group.ID] = pageMapNode{ID: mapNodeID(section.ID + "-foreign-" + group.ID), Component: other.Target.ID, Href: "#" + groupAnchorID(otherSection.ID, group.ID), FullTitle: group.Title, Title: mapTitle(group.Title), Summary: group.Summary, Concepts: builder.groupConcepts(group), Lane: "dependencies", Members: len(group.MemberSubjectIDs)}
			for _, subjectID := range group.MemberSubjectIDs {
				if key := locations[subjectID]; key != "" && owners[key] == "" {
					owners[key] = group.ID
				}
			}
		}
	}
	nodeOfGroup := func(group string) string {
		if node, ok := foreignNodes[group]; ok {
			return node.ID
		}
		return mapNodeID(group)
	}
	for _, subject := range index.Subjects {
		if groupOf[subject.ID] != "" || subject.Object == nil || subject.Object.Location == nil {
			continue
		}
		if group := owners[operationLocationKey(*subject.Object.Location)]; group != "" {
			groupOf[subject.ID] = group
		}
	}
	adj := make(map[string][]groupindex.StructuralEdge)
	for _, edge := range index.StructuralEdges {
		if edge.Role != groupindex.EdgeRelationTarget {
			continue
		}
		switch edge.RelationKind {
		case programindex.RelationCalls, programindex.RelationExecutes, programindex.RelationInvokesExternal:
			adj[edge.FromSubjectID] = append(adj[edge.FromSubjectID], edge)
		}
	}
	type pathEdge struct {
		from, to string
		possible bool
		label    string
	}
	paths := make(map[string]map[pathEdge]bool)
	usedForeign := make(map[string]bool)
	// Preserve matched integrations even when no classified operation reaches
	// their caller. They belong to the caller's group, not to an unrelated
	// operation merely because it is on the same component page.
	matched := make(map[string]pathEdge)
	// Library and executable views can expose the same source operation. Keep
	// every underlying connection, but give that physical destination one node
	// on this map. Different anchors or differently named components stay distinct.
	peerViews := make(map[string]groupindex.Connection)
	peerViewKey := func(connection groupindex.Connection) string {
		other := builder.graphIndex(connection.To.TargetID)
		if other == nil || connection.ToLocation == nil {
			return ""
		}
		return other.Target.Language + "\x00" + other.Target.Name + "\x00" + operationLocationKey(*connection.ToLocation)
	}
	for _, connection := range index.Connections {
		if connection.SourceKind != "integration" || connection.To.TargetID == index.Target.ID {
			continue
		}
		key := peerViewKey(connection)
		if key == "" {
			continue
		}
		previous, exists := peerViews[key]
		if !exists || builder.graphIndex(connection.To.TargetID).Target.Kind == "executable" && builder.graphIndex(previous.To.TargetID).Target.Kind != "executable" {
			peerViews[key] = connection
		}
	}
	for _, connection := range index.Connections {
		if connection.SourceKind != "integration" || connection.To.TargetID == index.Target.ID {
			continue
		}
		destination := connection
		if selected, ok := peerViews[peerViewKey(connection)]; ok {
			destination = selected
		}
		otherSection := builder.byProgram[destination.To.TargetID]
		otherIndex := builder.graphIndex(destination.To.TargetID)
		if otherSection == nil || otherIndex == nil {
			continue
		}
		key := "peer-" + destination.To.TargetID + "-" + destination.To.GroupID
		if destination.ToLocation != nil {
			key += "-" + operationLocationKey(*destination.ToLocation)
		}
		node := pageMapNode{ID: mapNodeID(section.ID + "-" + key), Component: destination.To.TargetID, Href: "#" + groupAnchorID(otherSection.ID, destination.To.GroupID), FullTitle: otherSection.ShortLabel + " / " + connection.Label, Summary: connection.Summary, Lane: "dependencies"}
		for _, peer := range otherIndex.Operations {
			if destination.ToLocation != nil && operationLocationKey(peer.Location) == operationLocationKey(*destination.ToLocation) {
				node.Href = "#" + operationNodeID(otherSection.ID, peer.ID)
				node.FullTitle = otherSection.ShortLabel + " / " + builder.operationDisplayName(peer)
				node.Summary = peer.Summary
				break
			}
		}
		node.Title = mapTitle(node.FullTitle)
		foreignNodes[key], usedForeign[key] = node, true
		matched[connection.ID] = pathEdge{mapNodeID(connection.From.GroupID), node.ID, true, connection.Label}
	}
	ops := append([]groupindex.Operation(nil), index.Operations...)
	sort.SliceStable(ops, func(i, j int) bool {
		if ops[i].Kind != ops[j].Kind {
			return ops[i].Kind < ops[j].Kind
		}
		if ops[i].Name != ops[j].Name {
			return ops[i].Name < ops[j].Name
		}
		return ops[i].Location.Line < ops[j].Location.Line
	})
	for i, operation := range ops {
		id := operationNodeID(section.ID, operation.ID)
		near := map[string]bool{mapNodeID(operation.GroupID): true}
		path := make(map[pathEdge]bool)
		seen := make(map[string]bool)
		parents := make(map[string]groupindex.StructuralEdge)
		discovered := map[string]bool{operation.SubjectID: true}
		firstInGroup := make(map[string]string)
		queue := []string{operation.SubjectID}
		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			if current == "" || seen[current] {
				continue
			}
			seen[current] = true
			if group := groupOf[current]; group != "" {
				near[nodeOfGroup(group)] = true
				if firstInGroup[nodeOfGroup(group)] == "" {
					firstInGroup[nodeOfGroup(group)] = current
				}
				if _, foreign := foreignNodes[group]; foreign {
					usedForeign[group] = true
				}
			}
			for _, edge := range adj[current] {
				if !discovered[edge.ToSubjectID] {
					discovered[edge.ToSubjectID] = true
					parents[edge.ToSubjectID] = edge
					queue = append(queue, edge.ToSubjectID)
				}
				from, to := groupOf[current], groupOf[edge.ToSubjectID]
				if from != "" && to != "" && from != to {
					label := string(edge.RelationKind)
					path[pathEdge{nodeOfGroup(from), nodeOfGroup(to), edge.Resolution != programindex.ResolutionExact, label}] = true
				}
			}
		}
		// A matched boundary is an integration hypothesis with exact endpoints,
		// never a compiler call. Its other end stays a navigable graph node.
		for _, connection := range index.Connections {
			edge, ok := matched[connection.ID]
			if !ok || !seen[connection.FromSubjectID] {
				continue
			}
			near[edge.to] = true
			path[edge] = true
		}
		paths[id] = path
		nearIDs := make([]string, 0, len(near))
		for nearID := range near {
			nearIDs = append(nearIDs, nearID)
		}
		sort.Strings(nearIDs)
		source := builder.links.anchor(operation.Location.Path, operation.Location.Line, operation.Location.Column)
		subtitle := groups[operation.GroupID].Title
		if runes := []rune(subtitle); len(runes) > 28 {
			subtitle = string(runes[:27]) + "…"
		}
		name := builder.operationDisplayName(operation)
		result.Nodes = append(result.Nodes, pageMapNode{
			ID: id, Href: source.Href, Title: mapTitle(name), FullTitle: name,
			Summary: operation.Summary, Activation: operation.Kind, Source: source, SourceKind: operation.Source,
			OperationGroup: groups[operation.GroupID].Title,
			CallPaths:      builder.operationCallPaths(operation.SubjectID, firstInGroup, parents),
			Subtitle:       subtitle,
			Lane:           "triggers", X: mapPadding, Y: 40 + float64(i)*84, Width: mapNodeWidth, Height: 68,
			Neighbours: strings.Join(nearIDs, " "), Degree: len(near), Members: 1,
		})
	}
	ordered := append([]groupindex.Group(nil), index.Groups...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Title < ordered[j].Title })
	for i, group := range ordered {
		result.Nodes = append(result.Nodes, pageMapNode{
			ID: mapNodeID(group.ID), Href: "#" + groupAnchorID(section.ID, group.ID),
			Title: mapTitle(group.Title), FullTitle: group.Title, Summary: dropEcho(group.Summary, group.Title), Keys: builder.keySymbols(group, maxKeySymbols),
			Lane: string(group.Lane), Members: len(group.MemberSubjectIDs), Concepts: builder.groupConcepts(group),
			X: 246, Y: 40 + float64(i)*84, Width: mapNodeWidth, Height: mapNodeHeight,
		})
	}
	remoteKeys := make([]string, 0, len(usedForeign))
	for id := range usedForeign {
		remoteKeys = append(remoteKeys, id)
	}
	sort.Strings(remoteKeys)
	for i, id := range remoteKeys {
		node := foreignNodes[id]
		node.Remote = true
		node.X = 480
		node.Y = 40 + float64(i)*84
		node.Width = mapNodeWidth
		node.Height = mapNodeHeight
		result.Nodes = append(result.Nodes, node)
	}
	byID := make(map[string]pageMapNode)
	for _, node := range result.Nodes {
		byID[node.ID] = node
		result.Height = max(result.Height, node.Y+node.Height+30)
	}
	usage := make(map[pathEdge][]string)
	for _, edge := range matched {
		usage[edge] = nil
	}
	for i, operation := range ops {
		id := result.Nodes[i].ID
		usage[pathEdge{id, mapNodeID(operation.GroupID), operation.Source == "model", "implemented in"}] = []string{id}
		for edge := range paths[id] {
			usage[edge] = append(usage[edge], id)
		}
	}
	for edge, operations := range usage {
		sort.Strings(operations)
		label := edge.label
		result.Edges = append(result.Edges, pageMapEdge{From: edge.from, To: edge.to, Label: label, Possible: edge.possible, Scope: "operation", Operations: strings.Join(operations, " ")})
	}
	sort.Slice(result.Edges, func(i, j int) bool {
		a, b := result.Edges[i], result.Edges[j]
		if a.From+a.To+a.Label+a.Operations == b.From+b.To+b.Label+b.Operations {
			return !a.Possible && b.Possible
		}
		return a.From+a.To+a.Label+a.Operations < b.From+b.To+b.Label+b.Operations
	})
	result.Width, result.MinWidth = 540, 540
	if len(remoteKeys) > 0 {
		result.Width = 780
		result.MinWidth = 780
		result.Lanes = append(result.Lanes, pageMapLane{Label: "Other components", X: 480})
	}
	routingNodes := make(map[string]*pageMapNode, len(byID))
	for id, node := range byID {
		routingNodes[id] = &node
	}
	router := newMapEdgeRouter(routingNodes, result.Height)
	for i := range result.Edges {
		edge := &result.Edges[i]
		edge.Path, _, _, _, _ = router.route(routingNodes[edge.From], routingNodes[edge.To])
	}
	result.Height += router.extraHeight()
	// The interactive view uses the same outside-column loop geometry.
	// Reserve its full gutter before hovering, so preview cannot rescale nodes.
	for _, node := range result.Nodes {
		result.Width = max(result.Width, node.X+node.Width+mapLoopMaxDepth+2*mapLoopSpread+mapPadding)
	}
	result.Lanes = append(result.Lanes, pageMapLane{Label: "Operations", X: mapPadding}, pageMapLane{Label: "Implementation", X: 246})
	return result
}

func operationLocationKey(location programindex.Location) string {
	return fmt.Sprintf("%s:%d:%d", location.Path, location.Line, max(1, location.Column))
}

func operationNodeID(section, id string) string { return section + "-op-" + safeIDFragment(id) }

// Containers and connections come from the same sealed GroupsIndex as the
// operation paths. The browser only folds their visible endpoints; it does
// not classify code or turn semantic connections into native calls.
func (builder *pageBuilder) addMapStructure(result *pageMap, section *pageSection, index *groupindex.Index) {
	result.Explorer = true
	byID := make(map[string]int)
	for i, node := range result.Nodes {
		byID[node.ID] = i
	}
	add := func(node pageMapNode) {
		if _, exists := byID[node.ID]; exists {
			return
		}
		node.X, node.Y, node.Width, node.Height = 12, 40, mapNodeWidth, 68
		byID[node.ID] = len(result.Nodes)
		result.Nodes = append(result.Nodes, node)
	}
	endpoint := func(end groupindex.Endpoint) string {
		if end.TargetID == index.Target.ID {
			return mapNodeID(end.GroupID)
		}
		other := builder.graphIndex(end.TargetID)
		otherSection := builder.byProgram[end.TargetID]
		if other == nil || otherSection == nil {
			return ""
		}
		for _, group := range other.Groups {
			if group.ID != end.GroupID {
				continue
			}
			id := mapNodeID(section.ID + "-foreign-" + group.ID)
			add(pageMapNode{ID: id, Component: end.TargetID, Remote: true, Href: "#" + groupAnchorID(otherSection.ID, group.ID), FullTitle: group.Title, Title: mapTitle(group.Title), Summary: group.Summary, Concepts: builder.groupConcepts(group), Members: len(group.MemberSubjectIDs), Lane: string(group.Lane)})
			return id
		}
		return ""
	}
	// Matched connections are stored by their source target. The destination
	// must read that same saved set to retain its incoming component stubs.
	for _, connection := range builder.allConnections() {
		if connection.From.TargetID != index.Target.ID && connection.To.TargetID != index.Target.ID {
			continue
		}
		from, to := endpoint(connection.From), endpoint(connection.To)
		if connection.SourceKind == "integration" && connection.ToLocation != nil {
			if other := builder.graphIndex(connection.To.TargetID); other != nil {
				if otherSection := builder.byProgram[connection.To.TargetID]; otherSection != nil {
					for _, operation := range other.Operations {
						if operationLocationKey(operation.Location) != operationLocationKey(*connection.ToLocation) {
							continue
						}
						href := "#" + operationNodeID(otherSection.ID, operation.ID)
						for _, node := range result.Nodes {
							if node.Remote && node.Href == href {
								to = node.ID
								break
							}
						}
					}
				}
			}
		}
		if _, exists := byID[from]; !exists {
			continue
		}
		if _, exists := byID[to]; !exists {
			continue
		}
		edge := pageMapEdge{From: from, To: to, Label: connection.Label, Summary: connection.Summary, Scope: "structure", Possible: true}
		if connection.FromLocation != nil {
			l := connection.FromLocation
			edge.FromSource = builder.links.anchor(l.Path, l.Line, l.Column)
		}
		if connection.ToLocation != nil {
			l := connection.ToLocation
			edge.ToSource = builder.links.anchor(l.Path, l.Line, l.Column)
		}
		result.Edges = append(result.Edges, edge)
	}
	addAreas := func(owner *groupindex.Index, remote bool) []string {
		var areaIDs []string
		for _, container := range owner.Containers {
			var children []string
			for _, groupID := range container.GroupIDs {
				id := mapNodeID(groupID)
				if remote {
					id = mapNodeID(section.ID + "-foreign-" + groupID)
				}
				if _, exists := byID[id]; exists {
					children = append(children, id)
				}
			}
			if len(children) == 0 {
				continue
			}
			id := section.ID + "-area-" + safeIDFragment(container.ID)
			unit := "groups"
			if len(children) == 1 {
				unit = "group"
			}
			add(pageMapNode{ID: id, Branch: "area", Children: strings.Join(children, " "), Remote: remote, Component: owner.Target.ID, Href: "#" + id, FullTitle: container.Title, Title: mapTitle(container.Title), Summary: container.Summary, Subtitle: fmt.Sprintf("%d %s · explore →", len(children), unit), Lane: string(container.Lane)})
			areaIDs = append(areaIDs, id)
		}
		return areaIDs
	}
	addAreas(index, false)
	var peerRoots []string
	for _, other := range builder.indexes {
		otherSection := builder.byProgram[other.Target.ID]
		if other.Target.ID == index.Target.ID || otherSection == nil {
			continue
		}
		children := addAreas(&other, true)
		covered := make(map[string]bool)
		for _, id := range children {
			for _, child := range strings.Fields(result.Nodes[byID[id]].Children) {
				covered[child] = true
			}
		}
		for _, node := range result.Nodes {
			if node.Remote && node.Component == other.Target.ID && node.Branch == "" && !covered[node.ID] {
				children = append(children, node.ID)
			}
		}
		if len(children) == 0 {
			continue
		}
		id := section.ID + "-component-" + safeIDFragment(other.Target.ID)
		peerRoots = append(peerRoots, id)
		add(pageMapNode{ID: id, Branch: "component", Children: strings.Join(children, " "), Remote: true, Component: other.Target.ID, Href: "#" + otherSection.ID, FullTitle: otherSection.ShortLabel, Title: mapTitle(otherSection.ShortLabel), Subtitle: other.Target.Kind + " · explore →", Lane: "dependencies"})
	}
	if len(peerRoots) > 1 {
		id := section.ID + "-connected-components"
		add(pageMapNode{ID: id, Branch: "components", Children: strings.Join(peerRoots, " "), Remote: true, Href: "#" + id, FullTitle: "Connected components", Title: mapTitle("Connected components"), Subtitle: fmt.Sprintf("%d components · explore →", len(peerRoots)), Lane: "dependencies"})
	}
	// Every group is retained, including groups with no classified operation.
	// The no-script document uses the compact zone picture; scripted views
	// position all retained nodes from this complete reservoir.
	static := builder.buildZoneMap(section, index)
	result.Width, result.Height, result.MinWidth = static.Width, static.Height, static.MinWidth
	result.Frames, result.Lanes = static.Frames, static.Lanes
	positions := make(map[string]pageMapNode)
	for _, node := range static.Nodes {
		positions[node.ID] = node
	}
	for i := range result.Nodes {
		node := &result.Nodes[i]
		if placed, ok := positions[node.ID]; ok {
			node.X, node.Y, node.Width, node.Height = placed.X, placed.Y, placed.Width, placed.Height
		} else {
			node.InitiallyHidden = true
		}
	}
	for _, edge := range static.Edges {
		edge.Scope = "static"
		result.Edges = append(result.Edges, edge)
	}
}

// Keep one shortest native call witness for each reached group. This is an
// explanation of membership in the view, not an assertion that all calls run.
func (builder *pageBuilder) operationCallPaths(root string, destinations map[string]string, parents map[string]groupindex.StructuralEdge) string {
	type step struct {
		Name     string `json:"name"`
		Href     string `json:"href,omitempty"`
		Open     string `json:"open,omitempty"`
		Source   string `json:"source"`
		Possible bool   `json:"possible,omitempty"`
		NoSource bool   `json:"no_source,omitempty"`
	}
	paths := make(map[string][]step)
	for node, destination := range destinations {
		var reversed []step
		for current := destination; current != ""; {
			ref, known := builder.subjects[current]
			if !known {
				reversed = nil
				break
			}
			name, anchor := builder.subjectDisplay(ref.subject)
			if anchor == nil {
				reversed = nil
				break
			}
			edge, hasParent := parents[current]
			reversed = append(reversed, step{Name: name, Href: anchor.Href, Open: anchor.Open, Source: anchor.Text, NoSource: anchor.NoSource, Possible: hasParent && edge.Resolution != programindex.ResolutionExact})
			if current == root {
				break
			}
			if !hasParent {
				reversed = nil
				break
			}
			current = edge.FromSubjectID
		}
		if len(reversed) == 0 {
			continue
		}
		for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
			reversed[left], reversed[right] = reversed[right], reversed[left]
		}
		paths[node] = reversed
	}
	data, _ := json.Marshal(paths) // strings and booleans only
	return string(data)
}
