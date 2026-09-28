package report

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// Reuse a declaration's accepted alias only when an operation repeats its
// native name. A distinct action label or command/path has its own meaning.
func (builder *pageBuilder) operationDisplayName(targetID string, operation groupindex.Operation) string {
	if operation.Source != "model" || operation.Kind == "command" || operation.Kind == "request" {
		return operation.Name
	}
	ref, ok := builder.subject(targetID, operation.SubjectID)
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

// Operations are interpretations on existing subjects. An input's path is
// its saved reach (GroupsIndex): the parts its handler's calls and reads
// enter, each joined from every earlier part a call enters it from.
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
	// Foreign nodes exist only for an explicit cross-target connection. Equal
	// source locations are common when two executables share a library; a path
	// is evidence, not authority to move one target's operation into its sibling.
	foreignNodes := make(map[string]pageMapNode)
	type pathEdge struct {
		from, to     string
		possible     bool
		label        string
		connectionID string
	}
	paths := make(map[string]map[pathEdge]bool)
	usedForeign := make(map[string]bool)
	// Preserve matched integrations even when no classified operation reaches
	// their caller. They belong to the caller's group, not to an unrelated
	// operation merely because it is on the same component page.
	matched := make(map[string]pathEdge)
	matchedCalls := make(map[string]*pageEdgeCall)
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
				node.FullTitle = otherSection.ShortLabel + " / " + builder.operationDisplayName(otherIndex.Target.ID, peer)
				node.Summary = peer.Summary
				break
			}
		}
		node.Title = mapTitle(node.FullTitle)
		foreignNodes[key], usedForeign[key] = node, true
		matched[connection.ID] = pathEdge{mapNodeID(connection.From.GroupID), node.ID, true, connection.Label, connectionKey(index.Target.ID, connection.ID)}
		matchedCalls[connectionKey(index.Target.ID, connection.ID)] = builder.connectionCall(connection)
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
	reachOf := make(map[string]groupindex.Reach, len(index.Reach))
	for _, reach := range index.Reach {
		reachOf[reach.OperationID] = reach
	}
	inputNode := func(id string) string { return operationNodeID(section.ID, id) }
	partOf := func(subject string) string {
		if group := groupOf[subject]; group != "" {
			return mapNodeID(group)
		}
		return ""
	}
	for i, operation := range ops {
		id := operationNodeID(section.ID, operation.ID)
		reach := reachOf[operation.ID]
		// An operation in a file off the map has no part to stand beside,
		// and one whose handler is not established is claimed to be
		// implemented in no part: the part its call is written in declares
		// it.
		owner := ""
		if operation.GroupID != "" && !operation.HandlerUnknown {
			owner = mapNodeID(operation.GroupID)
		}
		near := map[string]bool{}
		if owner != "" {
			near[owner] = true
		}
		reached := make(map[string]int, len(reach.Subjects))
		for _, subject := range reach.Subjects {
			reached[subject.SubjectID] = subject.Depth
		}
		// The trace lists the parts in the order the reach enters them:
		// by depth, then by the first declaration reached in each.
		var trace []string
		// Every call into a part from a part reached earlier draws one arrow
		// per pair; it is dashed only when none of its calls is exact.
		type pair struct{ from, to string }
		possible := map[pair]bool{}
		kinds := map[pair][]string{}
		var order []pair
		for _, group := range reach.Groups {
			part := mapNodeID(group.GroupID)
			near[part] = true
			trace = append(trace, part)
			for _, witness := range group.Entered {
				edge := index.StructuralEdges[witness.Edge]
				for _, from := range witness.From {
					key := pair{mapNodeID(from), part}
					if _, seen := kinds[key]; !seen {
						order = append(order, key)
						possible[key] = true
					}
					possible[key] = possible[key] && edge.Resolution != programindex.ResolutionExact
					if !slices.Contains(kinds[key], string(edge.RelationKind)) {
						kinds[key] = append(kinds[key], string(edge.RelationKind))
					}
				}
			}
		}
		path := make(map[pathEdge]bool)
		for _, key := range order {
			path[pathEdge{key.from, key.to, possible[key], strings.Join(kinds[key], " · "), ""}] = true
		}
		decls := builder.pathDecls(index.Target.ID, partOf)
		var extra []pageInputPart
		// A matched boundary is an integration hypothesis with exact endpoints,
		// never a compiler call. Its other end stays a navigable graph node.
		for _, connection := range index.Connections {
			edge, ok := matched[connection.ID]
			if _, reaches := reached[connection.FromSubjectID]; !ok || !reaches {
				continue
			}
			near[edge.to] = true
			path[edge] = true
			if !slices.Contains(trace, edge.to) {
				trace = append(trace, edge.to)
			}
			peer := pageDecl{Name: edge.label}
			for _, node := range foreignNodes {
				if node.ID == edge.to {
					peer = pageDecl{Name: node.FullTitle, Href: node.Href, Part: node.ID}
				}
			}
			extra = append(extra, pageInputPart{Part: edge.to, Title: peer.Name, Depth: reached[connection.FromSubjectID] + 1,
				Entered: []pageCall{{decls.of(connection.FromSubjectID), decls.add("peer "+edge.to, peer), callPossible | callIntegration}}})
		}
		// An outside call the reach makes is entered at the declaration
		// that makes it.
		for _, call := range index.Outbound {
			depth, reaches := reached[call.SubjectID]
			if !reaches || call.SubjectID == "" {
				continue
			}
			name := call.External
			if name == "" {
				name = outboundKindLabel(call.Kind)
			}
			anchor := builder.links.anchor(call.Location.Path, call.Location.Line, call.Location.Column)
			tile := "system-" + section.ID + "-out-" + call.ID
			outside := pageDecl{Name: name, Href: anchor.Href, Open: anchor.Open, Source: anchor.Text, NoSource: anchor.NoSource, Part: tile}
			extra = append(extra, pageInputPart{Part: tile, Title: name, Depth: depth + 1, Entered: []pageCall{{decls.of(call.SubjectID), decls.add("outside "+call.ID, outside), 0}}})
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
		name := builder.operationDisplayName(index.Target.ID, operation)
		var handler string
		var handlerSource pageAnchor
		if ref, known := builder.subject(index.Target.ID, operation.SubjectID); known {
			if display, anchor := builder.subjectDisplay(ref.subject); display != "" {
				handler = display
				if anchor != nil {
					handlerSource = *anchor
				}
			}
		}
		result.Nodes = append(result.Nodes, pageMapNode{
			ID: id, Href: source.Href, Title: mapTitle(name), FullTitle: name,
			InputOwner: owner,
			Summary:    operation.Summary, Activation: operation.Kind, Source: source, SourceKind: operation.Source,
			OperationGroup: groups[operation.GroupID].Title,
			InputPath:      builder.inputPath(index, operation, reach, decls, mapNodeID, inputNode, extra),
			Writes:         builder.operationWrites(index, reach),
			Subtitle:       subtitle,
			Lane:           "triggers", X: mapPadding, Y: 40 + float64(i)*84, Width: mapNodeWidth, Height: 68,
			Neighbours: strings.Join(nearIDs, " "), Degree: len(near), Members: 1,
			Trace:   strings.Join(trace, " "),
			Handler: handler, HandlerSource: handlerSource, HandlerUnknown: operation.HandlerUnknown,
		})
	}
	ordered := append([]groupindex.Group(nil), index.Groups...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Title < ordered[j].Title })
	for i, group := range ordered {
		symbols, symbolCalls := builder.groupSymbols(index.Target.ID, group)
		result.Nodes = append(result.Nodes, pageMapNode{
			ID: mapNodeID(group.ID), Href: "#" + groupAnchorID(section.ID, group.ID),
			Dispatch: builder.siteReadings(index, group, builder.pathDecls(index.Target.ID, partOf), inputNode),
			Title:    mapTitle(group.Title), FullTitle: group.Title, Summary: dropEcho(group.Summary, group.Title), Keys: builder.keySymbols(index.Target.ID, group, maxKeySymbols),
			Lane: pageLane(group.Lane, group.Core), Symbols: symbols, SymbolCalls: symbolCalls, Members: len(group.MemberSubjectIDs), Concepts: builder.groupConcepts(index.Target.ID, group),
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
	handledBy := make(map[string]pageEdgeCall)
	for i, operation := range ops {
		id := result.Nodes[i].ID
		if ref, known := builder.subject(index.Target.ID, operation.SubjectID); known {
			if name, anchor := builder.subjectDisplay(ref.subject); name != "" {
				call := pageEdgeCall{Label: "implemented in", Name: name, Callee: declarationKey(anchor)}
				if anchor != nil {
					call.To = anchor.Href
				}
				handledBy[id] = call
			}
		}
		if operation.GroupID != "" && !operation.HandlerUnknown {
			usage[pathEdge{id, mapNodeID(operation.GroupID), operation.Source == "model", "implemented in", ""}] = []string{id}
		}
		for edge := range paths[id] {
			usage[edge] = append(usage[edge], id)
		}
	}
	for edge, operations := range usage {
		sort.Strings(operations)
		label := edge.label
		drawn := pageMapEdge{ConnectionID: edge.connectionID, From: edge.from, To: edge.to, Label: label, Possible: edge.possible, Scope: "operation", Operations: strings.Join(operations, " ")}
		if call, ok := handledBy[edge.from]; ok && label == "implemented in" {
			drawn.Calls = []pageEdgeCall{call}
		}
		if call := matchedCalls[edge.connectionID]; call != nil && edge.connectionID != "" {
			drawn.Calls = []pageEdgeCall{*call}
		}
		result.Edges = append(result.Edges, drawn)
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

func operationNodeID(section, id string) string { return section + "-" + safeIDFragment(id) }

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
			id := foreignNodeID(section.ID, end.TargetID, group.ID)
			add(pageMapNode{ID: id, Component: end.TargetID, Remote: true, Href: "#" + groupAnchorID(otherSection.ID, group.ID), FullTitle: group.Title, Title: mapTitle(group.Title), Summary: group.Summary, Concepts: builder.groupConcepts(end.TargetID, group), Members: len(group.MemberSubjectIDs), Lane: pageLane(group.Lane, group.Core)})
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
		// GroupsIndex says which arrows stand quiet until an end is looked
		// at (Connection.Quiet): wiring and calls into helpers, with its
		// exceptions.
		edge := pageMapEdge{ConnectionID: connectionKey(connection.From.TargetID, connection.ID), From: from, To: to, Label: connection.Label, Summary: connection.Summary, Scope: "structure", Possible: !strings.HasPrefix(connection.SourceKind, "native_") || connection.SupportResolution != programindex.PatternValueExact, Init: connection.Quiet}
		if connection.FromLocation != nil {
			l := connection.FromLocation
			edge.FromSource = builder.links.anchor(l.Path, l.Line, l.Column)
		}
		if connection.ToLocation != nil {
			l := connection.ToLocation
			edge.ToSource = builder.links.anchor(l.Path, l.Line, l.Column)
		}
		if call := builder.connectionCall(connection); call != nil {
			edge.Calls = []pageEdgeCall{*call}
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
					id = foreignNodeID(section.ID, owner.Target.ID, groupID)
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
			add(pageMapNode{ID: id, Branch: "area", Children: strings.Join(children, " "), Remote: remote, Component: owner.Target.ID, Href: "#" + id, FullTitle: container.Title, Title: mapTitle(container.Title), Summary: container.Summary, Subtitle: fmt.Sprintf("%d %s · explore →", len(children), unit), Lane: pageLane(container.Lane, container.Core)})
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

// pageCallStep is one declaration calling a writer on an input's path:
// its name and where the call is written.
type pageCallStep struct {
	Name     string `json:"name"`
	Href     string `json:"href,omitempty"`
	Open     string `json:"open,omitempty"`
	Source   string `json:"source"`
	Possible bool   `json:"possible,omitempty"`
	NoSource bool   `json:"no_source,omitempty"`
}

// foreignNodeID names another target's group on this section's map. Group
// IDs are per target, so two peers both have a g15: without the owner the
// second one's arrows landed on the first's node (redis-benchmark's "Linked
// list calls Memory allocation" drawn into redis-server's Networking).
func foreignNodeID(sectionID, targetID, groupID string) string {
	return mapNodeID(sectionID + "-foreign-" + safeIDFragment(targetID) + "-" + groupID)
}

// connectionKey names a connection across the page. A connection's x* ID is
// its source target's ordinal, so redis-benchmark's x17 (listRelease calls
// zfree) and another program's x17 are different connections; keyed by the
// bare ID, the system map moved one's arrow onto the other's peer.
func connectionKey(targetID, id string) string {
	return targetID + "/" + id
}
