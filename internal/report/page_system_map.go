package report

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
)

// SystemMap assembles the already prepared, translated component views into
// one drawing. It changes no membership or operation path. Remote views join
// only through their exact existing destination link, never through a label.
// Called by the template after display translation, it adds no model request
// or display-catalogue entry during saved rendering.
func (view *pageView) SystemMap() *pageMap {
	result := &pageMap{System: true, Explorer: true, Operations: true, MarkerID: "system-arrow", Width: 1000, MinWidth: 600}
	positions, destinations, aliases := map[string]int{}, map[string]string{}, map[string]string{}
	add := func(node pageMapNode) {
		if _, exists := positions[node.ID]; exists {
			return
		}
		node.X, node.Y = 20+float64(len(result.Nodes)%3)*300, 40+float64(len(result.Nodes)/3)*100
		node.Width, node.Height, node.InitiallyHidden = 270, 80, false
		node.Title = mapTitle(node.FullTitle)
		positions[node.ID] = len(result.Nodes)
		result.Nodes = append(result.Nodes, node)
	}
	for _, section := range view.Sections {
		if section.Map != nil {
			for _, node := range section.Map.Nodes {
				if node.Remote {
					continue
				}
				node.Owner = section.ID
				add(node)
				destinations["#"+node.ID] = node.ID
				if strings.HasPrefix(node.Href, "#") {
					destinations[node.Href] = node.ID
				}
			}
		}
		for _, row := range append(append([]pageGroupOperation(nil), section.Requests...), section.Activities...) {
			id := strings.TrimPrefix(row.Href, "#")
			if id == "" {
				continue
			}
			add(pageMapNode{ID: id, Owner: section.ID, Activation: row.Kind, FullTitle: row.Name, Summary: row.Summary, SummaryRef: row.SummaryRef, SourceKind: row.Source, Source: row.Anchor, Href: row.Anchor.Href, Lane: "triggers"})
			destinations["#"+id] = id
		}
		id := "system-component-" + section.ID
		add(pageMapNode{ID: id, Owner: section.ID, Branch: "component", ItemKind: "Component", Href: "#" + section.ID, FullTitle: section.ShortLabel, Summary: section.Purpose, SummaryRef: section.PurposeRef, Role: section.Role, RoleRef: section.RoleRef, Language: section.Language, ComponentKind: section.Kind, SourceKind: "model", DetailsID: section.ID})
		destinations["#"+section.ID] = id
	}
	for _, section := range view.Sections {
		if section.Map == nil {
			continue
		}
		for _, node := range section.Map.Nodes {
			if !node.Remote || node.Branch != "" {
				continue
			}
			if canonical := destinations[node.Href]; canonical != "" {
				aliases[node.ID] = canonical
				if node.ID != canonical {
					at := positions[canonical]
					result.Nodes[at].Aliases = append(result.Nodes[at].Aliases, node.ID)
				}
			} else {
				// A peer with no displayed owner still has its original source
				// and explanation. Do not silently turn it into a local part.
				node.ItemKind = "Other components"
				add(node)
			}
		}
	}
	canonical := func(id string) string {
		if to := aliases[id]; to != "" {
			return to
		}
		return id
	}
	remap := func(ids string) string {
		var out []string
		seen := map[string]bool{}
		for _, id := range strings.Fields(ids) {
			id = canonical(id)
			if _, ok := positions[id]; ok && !seen[id] {
				out = append(out, id)
				seen[id] = true
			}
		}
		return strings.Join(out, " ")
	}
	for i := range result.Nodes {
		n := &result.Nodes[i]
		n.Children, n.Neighbours, n.Trace = remap(n.Children), remap(n.Neighbours), remap(n.Trace)
		n.InputOwner = remap(n.InputOwner)
		n.InputPath = remapInputPath(n.InputPath, canonical)
		n.Catalogue = remapCatalogue(n.Catalogue, canonical)
		n.Declares = remap(n.Declares)
		n.Dispatch = remapSiteReadings(n.Dispatch, canonical)
	}
	// A saved integration may already name a participant in this same map.
	// Keep the outbound catalogue intact, but do not draw that known participant
	// again as a third, external system. Ambiguous identities stay separate.
	outboundByConnection := map[string]string{}
	ambiguousConnections := map[string]bool{}
	outboundRows := map[string]pageOutbound{}
	for _, section := range view.Sections {
		for _, row := range section.Outbound {
			id := "system-" + row.ID
			outboundRows[id] = row
			for _, connection := range row.Connections {
				if previous := outboundByConnection[connection]; previous != "" && previous != id {
					ambiguousConnections[connection] = true
				}
				outboundByConnection[connection] = id
			}
		}
	}
	peers := map[string]map[string]bool{}
	for _, section := range view.Sections {
		if section.Map == nil {
			continue
		}
		for _, edge := range section.Map.Edges {
			from, fromKnown := positions[canonical(edge.From)]
			to, toKnown := positions[canonical(edge.To)]
			if edge.ConnectionID == "" || edge.Scope == "static" || !fromKnown || !toKnown ||
				result.Nodes[from].Remote || result.Nodes[to].Remote || result.Nodes[from].Owner == "" ||
				result.Nodes[to].Owner == "" || result.Nodes[from].Owner == result.Nodes[to].Owner {
				continue
			}
			if peers[edge.ConnectionID] == nil {
				peers[edge.ConnectionID] = map[string]bool{}
			}
			peers[edge.ConnectionID][result.Nodes[to].ID] = result.Nodes[to].Activation != ""
		}
	}
	peerByConnection := map[string]string{}
	for connection, candidates := range peers {
		var inputs []string
		owners := map[string]bool{}
		for id, input := range candidates {
			owners[result.Nodes[positions[id]].Owner] = true
			if input {
				inputs = append(inputs, id)
			}
		}
		if len(owners) != 1 {
			continue
		}
		if len(inputs) == 1 {
			peerByConnection[connection] = inputs[0]
		} else if len(inputs) == 0 && len(candidates) == 1 {
			for id := range candidates {
				peerByConnection[connection] = id
			}
		}
	}
	localOutbound := map[string]string{}
	for _, section := range view.Sections {
		for _, row := range section.Outbound {
			peer := ""
			for _, connection := range row.Connections {
				candidate := peerByConnection[connection]
				if candidate == "" || ambiguousConnections[connection] || result.Nodes[positions[candidate]].Owner == section.ID || peer != "" && peer != candidate {
					peer = ""
					break
				}
				peer = candidate
			}
			if peer != "" {
				localOutbound["system-"+row.ID] = peer
			}
		}
	}
	// Reuse the external catalogue's display grouping for unmatched records,
	// per program: one frame per destination its records name, one tile per
	// outside symbol the program calls, each tile and frame the program's own,
	// with its own arrow. Equal destination text across programs proves no
	// identity: one "TCP endpoint" box had taken arrows from all three Redis
	// programs, though for redis-cli that endpoint is redis-server and for
	// redis-server its master. Frames of different programs that name the
	// same destination only stand together in one display group, a frame
	// around them on the map that is no participant and holds no arrow.
	groupFrames := map[string][]int{}
	var groupKeys []string
	foldedTiles := map[string]string{}
	for _, section := range view.Sections {
		for _, group := range groupOutbound(section.Outbound) {
			var children []string
			name := group.Destination
			if name == "" {
				name = group.ProgramLabel
			}
			if name == "" {
				name = group.NativeLabel
			}
			if name == "" {
				name = group.KindLabel
			}
			// One tile per outside symbol: the same call made from several
			// places is one thing the program asks for. Every place it is
			// made from stays a line of the arrow: which function asks.
			tileOf := make(map[string]string)
			for _, row := range group.Rows {
				id := "system-" + row.ID
				if localOutbound[id] != "" {
					continue
				}
				symbol := row.External
				if symbol == "" {
					symbol = row.ID
				}
				tile, folded := tileOf[symbol]
				if !folded {
					tile = id
					tileOf[symbol] = id
					title := row.Line()
					if title == "" {
						title = row.Brief()
					}
					if title == "" {
						title = name
					}
					add(pageMapNode{ID: id, Owner: section.ID, ItemKind: "External communication", FullTitle: title, Summary: row.Summary, SummaryRef: row.SummaryRef, Source: row.Anchor, SourceKind: row.Source, DetailsID: row.ID, Href: "#" + row.ID, Subtitle: row.Address, Lane: "dependencies"})
					children = append(children, id)
				} else if id != tile {
					foldedTiles[id] = tile
					at := positions[tile]
					if !slices.Contains(result.Nodes[at].Aliases, id) {
						result.Nodes[at].Aliases = append(result.Nodes[at].Aliases, id)
					}
				}
				from := targetMapNodeID(section.programTargetID, mapNodeID(row.MapGroup))
				if _, ok := positions[from]; row.MapGroup != "" && ok {
					edge := pageMapEdge{From: from, To: tile, Scope: "structure", Operations: row.Operations, Label: row.KindLabel, Summary: row.Summary, SummaryRef: row.SummaryRef, Possible: row.Source != "fact", FromSource: row.Anchor}
					if row.Caller != "" && row.External != "" && !strings.ContainsAny(row.Caller+row.External, " \t") {
						edge.Calls = []pageEdgeCall{{Label: row.Caller + " calls " + row.External, From: row.CallerAnchor.Href, To: row.Anchor.Href, At: row.Anchor.Text, Caller: declarationKey(&row.CallerAnchor)}}
					}
					result.Edges = append(result.Edges, edge)
				}
			}
			if len(children) > 0 {
				id := "system-" + group.Rows[0].ID + "-destination"
				add(pageMapNode{ID: id, Owner: section.ID, Branch: "communication", ItemKind: "External communication", FullTitle: name, Children: strings.Join(children, " "), Lane: "dependencies"})
				key := strings.ToLower(name)
				if _, seen := groupFrames[key]; !seen {
					groupKeys = append(groupKeys, key)
				}
				groupFrames[key] = append(groupFrames[key], positions[id])
			}
		}
	}
	// Frames in a group that all spell their destination alike leave it to
	// the group to say once: three "DNS resolver" tiles side by side said one
	// thing three times. Different spellings keep their own titles.
	for n, key := range groupKeys {
		frames := groupFrames[key]
		if len(frames) < 2 {
			continue
		}
		title := result.Nodes[frames[0]].FullTitle
		for _, at := range frames {
			if result.Nodes[at].FullTitle != title {
				title = ""
			}
		}
		for _, at := range frames {
			result.Nodes[at].DisplayGroup = fmt.Sprintf("destinations-%d", n+1)
			result.Nodes[at].DisplayGroupTitle = title
		}
	}
	// An input's path into a folded record leads to the tile that stands for
	// it: reading that tile on the input's path keeps "Why it appears".
	// Without it, echo's GET /users/:id and fifteen microblog inputs named
	// tiles no map draws.
	if len(foldedTiles) > 0 {
		for i := range result.Nodes {
			result.Nodes[i].InputPath = remapInputPath(result.Nodes[i].InputPath, func(id string) string {
				if tile := foldedTiles[id]; tile != "" {
					return tile
				}
				return id
			})
		}
	}
	// Exact duplicate display copies (e.g. a cross-component arrow seen from
	// each end) share one line. Its original source endpoints stay intact.
	seenEdges := map[string]bool{}
	for _, section := range view.Sections {
		if section.Map != nil {
			for _, edge := range section.Map.Edges {
				if edge.Scope == "static" {
					continue
				}
				edge.From, edge.To = canonical(edge.From), canonical(edge.To)
				if from := outboundByConnection[edge.ConnectionID]; from != "" && !ambiguousConnections[edge.ConnectionID] {
					if localOutbound[from] == "" {
						edge.From = from
					} else if edge.FromSource == (pageAnchor{}) {
						edge.FromSource = outboundRows[from].Anchor
					}
				}
				if to := peerByConnection[edge.ConnectionID]; to != "" {
					edge.To = to
					if edge.ToSource == (pageAnchor{}) {
						edge.ToSource = result.Nodes[positions[to]].Source
					}
				}
				if _, ok := positions[edge.From]; !ok {
					continue
				}
				if _, ok := positions[edge.To]; !ok {
					continue
				}
				edge.Operations = remap(edge.Operations)
				edge.Path = ""
				edge.Lines = nil
				raw, _ := json.Marshal(edge)
				key := string(raw)
				if !seenEdges[key] {
					result.Edges = append(result.Edges, edge)
					seenEdges[key] = true
				}
			}
		}
		for _, group := range section.RouteGroups {
			for _, row := range group.Rows {
				for i, path := range row.Paths {
					matched := false
					for _, href := range path.OperationHrefs {
						if destinations[href] != "" {
							matched = true
						}
					}
					if matched {
						continue
					}
					id := fmt.Sprintf("system-route-%s-%d", section.ID, len(result.Nodes)+i)
					n := pageMapNode{ID: id, Owner: section.ID, ItemKind: "Incoming requests", Activation: "request", FullTitle: group.Method + " " + path.Path, SourceKind: "fact", DetailsID: section.ID + "-inbound", Lane: "triggers"}
					if path.Anchor != nil {
						n.Source = *path.Anchor
						n.Href = path.Anchor.Href
					}
					add(n)
				}
			}
		}
	}
	// One reading catalogue per component holds the existing inputs outside
	// the component frame. This adds containment, never a runtime connection.
	inputsByOwner := map[string][]string{}
	inputIDs := map[string]bool{}
	for _, node := range result.Nodes {
		if node.Activation != "" {
			inputsByOwner[node.Owner] = append(inputsByOwner[node.Owner], node.ID)
			inputIDs[node.ID] = true
		}
	}
	for i := range result.Nodes {
		var children []string
		for _, id := range strings.Fields(result.Nodes[i].Children) {
			if !inputIDs[id] {
				children = append(children, id)
			}
		}
		result.Nodes[i].Children = strings.Join(children, " ")
	}
	for _, section := range view.Sections {
		if children := inputsByOwner[section.ID]; len(children) > 0 {
			add(pageMapNode{ID: "system-inputs-" + section.ID, Owner: section.ID, Branch: "inputs", ItemKind: "Inputs", FullTitle: section.ShortLabel,
				Children: strings.Join(children, " "), Href: "#" + section.ID + "-inbound", DetailsID: section.ID + "-inbound", Lane: "triggers"})
		}
	}
	// Saved containment supplies the remaining frames; component membership
	// supplies the outer frame without duplicating either reading catalogue.
	contained := map[string]bool{}
	for _, n := range result.Nodes {
		for _, id := range strings.Fields(n.Children) {
			contained[id] = true
		}
	}
	// Areas come first, in the order the model listed them; loose parts follow.
	for _, section := range view.Sections {
		var areas, loose []string
		for _, n := range result.Nodes {
			if n.Owner == section.ID && n.Branch != "component" && n.Branch != "inputs" && !contained[n.ID] && n.ItemKind != "External communication" {
				if n.Branch == "area" {
					areas = append(areas, n.ID)
				} else {
					loose = append(loose, n.ID)
				}
			}
		}
		result.Nodes[positions["system-component-"+section.ID]].Children = strings.Join(append(areas, loose...), " ")
	}
	if view.RepoMap != nil {
		components := map[string]string{}
		for i, node := range view.RepoMap.Nodes {
			id := destinations[node.Href]
			if !node.Analyzed {
				id = fmt.Sprintf("system-unread-%d", i)
				add(pageMapNode{ID: id, ItemKind: "Component", FullTitle: node.FullName, Summary: node.Note, Lane: "dependencies"})
			}
			components[node.ID] = id
		}
		for _, e := range view.RepoMap.Edges {
			from, to := components[e.From], components[e.To]
			if from != "" && to != "" {
				result.Edges = append(result.Edges, pageMapEdge{From: from, To: to, Scope: "component", Label: e.Label, Possible: e.Possible})
			}
		}
	}
	// The component maps retain every exact relation for their cards, but the
	// system map is one drawing. Calls, callback flow and implementation facts
	// between the same two visible nodes share one physical arrow here. Drawing
	// each row separately produces coincident or parallel lines without adding
	// information; the node details still expose every original relation.
	result.Edges = collapseSystemMapEdges(result.Edges)
	completeSystemPaths(result)
	result.Height = 140 + float64(len(result.Nodes)/3)*100
	return result
}

func collapseSystemMapEdges(edges []pageMapEdge) []pageMapEdge {
	type endpoints struct{ from, to string }
	positions := make(map[endpoints]int, len(edges))
	result := make([]pageMapEdge, 0, len(edges))
	for _, edge := range edges {
		key := endpoints{edge.From, edge.To}
		position, exists := positions[key]
		if !exists {
			positions[key] = len(result)
			edge.Calls = edgeCalls(edge)
			result = append(result, edge)
			continue
		}
		merged := &result[position]
		for _, call := range edgeCalls(edge) {
			if !slices.Contains(merged.Calls, call) {
				merged.Calls = append(merged.Calls, call)
			}
		}
		merged.Operations = joinUniqueFields(merged.Operations, edge.Operations)
		merged.Label = joinUniqueText(merged.Label, edge.Label, " · ")
		merged.Summary = joinUniqueText(merged.Summary, edge.Summary, " ")
		merged.Possible = merged.Possible && edge.Possible
		// One arrow stands quiet only when every relation it draws does, as
		// the canvas groups them: the first relation's flag had decided it.
		merged.Init = merged.Init && edge.Init
		if edgeScopeRank(edge.Scope) < edgeScopeRank(merged.Scope) {
			merged.Scope = edge.Scope
		}
		if merged.ConnectionID == "" {
			merged.ConnectionID = edge.ConnectionID
		} else if edge.ConnectionID != "" && merged.ConnectionID != edge.ConnectionID {
			merged.ConnectionID = ""
		}
		if merged.LabelRef != edge.LabelRef {
			merged.LabelRef = ""
		}
		if merged.SummaryRef != edge.SummaryRef {
			merged.SummaryRef = ""
		}
		if merged.FromSource != edge.FromSource {
			merged.FromSource = pageAnchor{}
		}
		if merged.ToSource != edge.ToSource {
			merged.ToSource = pageAnchor{}
		}
	}
	return result
}

func joinUniqueFields(left, right string) string {
	values := strings.Fields(left)
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		seen[value] = true
	}
	for _, value := range strings.Fields(right) {
		if !seen[value] {
			values = append(values, value)
			seen[value] = true
		}
	}
	return strings.Join(values, " ")
}

func joinUniqueText(left, right, separator string) string {
	if left == "" {
		return right
	}
	if right == "" || left == right {
		return left
	}
	return left + separator + right
}

func edgeScopeRank(scope string) int {
	switch scope {
	case "operation":
		return 0
	case "structure":
		return 1
	case "component":
		return 2
	default:
		return 3
	}
}

// Compose only already established paths whose endpoint is an exact input.
// Never traverse a neighbouring part merely because it shares a group or
// name. A matched input's parts and writes continue the root's path as a
// possible integration; no chain is prefixed to them.
func completeSystemPaths(view *pageMap) {
	inputs := map[string]*pageMapNode{}
	readings := map[string]pageInputPath{}
	paths := map[string][]int{}
	writes := map[string][]pageEntityWrite{}
	uses := make([]map[string]bool, len(view.Edges))
	for i := range view.Nodes {
		if n := &view.Nodes[i]; n.Activation != "" {
			inputs[n.ID] = n
			var reading pageInputPath
			_ = json.Unmarshal([]byte(n.InputPath), &reading)
			readings[n.ID] = reading
			writes[n.ID] = n.Writes
		}
	}
	for i, edge := range view.Edges {
		uses[i] = map[string]bool{}
		for _, id := range strings.Fields(edge.Operations) {
			paths[id] = append(paths[id], i)
			uses[i][id] = true
		}
	}
	for root, node := range inputs {
		var joinedWrites []pageEntityWrite
		seen := map[string]bool{}
		own := readings[root]
		own.Parts = slices.Clone(own.Parts)
		own.Decls = slices.Clone(own.Decls)
		parts := map[string]bool{}
		for _, part := range own.Parts {
			parts[part.Part] = true
		}
		joined := false
		near := map[string]bool{}
		for _, id := range strings.Fields(node.Neighbours) {
			near[id] = true
		}
		queue := []string{root}
		for len(queue) > 0 {
			id := queue[0]
			queue = queue[1:]
			if seen[id] {
				continue
			}
			seen[id] = true
			for _, write := range writes[id] {
				write.Callers = slices.Clone(write.Callers)
				if id != root {
					write.Possible, write.Integration = true, true // The remote input is an integration match.
				}
				joinedWrites = append(joinedWrites, write)
			}
			if id != root {
				// A matched input's own trace and parts continue the root's,
				// after it.
				node.Trace = joinUniqueFields(node.Trace, inputs[id].Trace)
				other := readings[id]
				for _, part := range other.Parts {
					if parts[part.Part] {
						continue
					}
					parts[part.Part] = true
					joined = true
					copied := pageInputPart{Part: part.Part, Title: part.Title, Depth: part.Depth, Others: part.Others}
					for _, call := range part.Entered {
						copied.Entered = append(copied.Entered, pageCall{own.adopt(other.Decls[call[0]]), own.adopt(other.Decls[call[1]]), call[2] | callPossible | callIntegration})
					}
					own.Parts = append(own.Parts, copied)
				}
			}
			for _, at := range paths[id] {
				edge := view.Edges[at]
				uses[at][root] = true
				near[edge.From], near[edge.To] = true, true
				if inputs[edge.To] != nil && !seen[edge.To] {
					queue = append(queue, edge.To)
				}
			}
		}
		delete(near, root)
		node.Writes = joinedWrites
		node.Neighbours = sortedKeys(near)
		if joined {
			raw, _ := json.Marshal(own)
			node.InputPath = string(raw)
		}
	}
	for i := range view.Edges {
		view.Edges[i].Operations = sortedKeys(uses[i])
	}
}

// adopt adds another reading's declaration to this one's, once.
func (path *pageInputPath) adopt(decl pageDecl) int {
	for position, known := range path.Decls {
		if known == decl {
			return position
		}
	}
	path.Decls = append(path.Decls, decl)
	return len(path.Decls) - 1
}

func sortedKeys(values map[string]bool) string {
	keys := make([]string, 0, len(values))
	for id := range values {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	return strings.Join(keys, " ")
}
