package report

import (
	"encoding/json"
	"fmt"
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
		n.Children, n.Neighbours = remap(n.Children), remap(n.Neighbours)
		n.InputOwner = remap(n.InputOwner)
		if n.CallPaths != "" {
			var paths map[string]json.RawMessage
			if json.Unmarshal([]byte(n.CallPaths), &paths) == nil {
				joined := map[string]json.RawMessage{}
				for id, path := range paths {
					joined[canonical(id)] = path
				}
				raw, _ := json.Marshal(joined)
				n.CallPaths = string(raw)
			}
		}
	}
	// Reuse the external catalogue's display grouping. Each original record
	// remains a separate selectable endpoint inside a communication frame.
	outboundByConnection := map[string]string{}
	ambiguousConnections := map[string]bool{}
	for _, section := range view.Sections {
		for _, group := range groupOutbound(section.Outbound) {
			var children []string
			name := group.Destination
			if name == "" {
				name = group.NativeLabel
			}
			if name == "" {
				name = group.KindLabel
			}
			for _, row := range group.Rows {
				id := "system-" + row.ID
				title := row.Line()
				if title == "" {
					title = row.Brief()
				}
				if title == "" {
					title = name
				}
				add(pageMapNode{ID: id, Owner: section.ID, ItemKind: "External communication", FullTitle: title, Summary: row.Summary, SummaryRef: row.SummaryRef, Source: row.Anchor, SourceKind: row.Source, DetailsID: row.ID, Href: "#" + row.ID, Subtitle: row.Address, Lane: "dependencies"})
				children = append(children, id)
				for _, connection := range row.Connections {
					if previous := outboundByConnection[connection]; previous != "" && previous != id {
						ambiguousConnections[connection] = true
					}
					outboundByConnection[connection] = id
				}
				from := mapNodeID(row.MapGroup)
				if _, ok := positions[from]; row.MapGroup != "" && ok {
					result.Edges = append(result.Edges, pageMapEdge{From: from, To: id, Scope: "structure", Operations: row.Operations, Label: row.KindLabel, Summary: row.Summary, SummaryRef: row.SummaryRef, Possible: row.Source != "fact", FromSource: row.Anchor})
				}
			}
			if len(children) > 0 {
				add(pageMapNode{ID: children[0] + "-destination", Owner: section.ID, Branch: "communication", ItemKind: "External communication", FullTitle: name, Children: strings.Join(children, " "), Lane: "dependencies"})
			}
		}
	}
	// Exact duplicate display copies (e.g. a cross-component arrow seen from
	// each end) share one line. Its original source endpoints stay intact.
	inputByConnection := map[string]string{}
	for _, section := range view.Sections {
		if section.Map == nil {
			continue
		}
		for _, edge := range section.Map.Edges {
			to := canonical(edge.To)
			if at, ok := positions[to]; ok && edge.ConnectionID != "" && result.Nodes[at].Activation != "" {
				inputByConnection[edge.ConnectionID] = to
			}
		}
	}
	seenEdges := map[string]bool{}
	for _, section := range view.Sections {
		if section.Map != nil {
			for _, edge := range section.Map.Edges {
				if edge.Scope == "static" {
					continue
				}
				edge.From, edge.To = canonical(edge.From), canonical(edge.To)
				if from := outboundByConnection[edge.ConnectionID]; from != "" && !ambiguousConnections[edge.ConnectionID] {
					edge.From = from
				}
				if to := inputByConnection[edge.ConnectionID]; to != "" {
					edge.To = to
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
	for _, section := range view.Sections {
		var members []string
		for _, n := range result.Nodes {
			if n.Owner == section.ID && n.Branch != "component" && n.Branch != "inputs" && !contained[n.ID] && n.ItemKind != "External communication" {
				members = append(members, n.ID)
			}
		}
		result.Nodes[positions["system-component-"+section.ID]].Children = strings.Join(members, " ")
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
	completeSystemPaths(result)
	result.Height = 140 + float64(len(result.Nodes)/3)*100
	return result
}

// Compose only already established paths whose endpoint is an exact input.
// Never traverse a neighbouring part merely because it shares a group or name.
func completeSystemPaths(view *pageMap) {
	inputs := map[string]*pageMapNode{}
	paths := map[string][]int{}
	witnesses := map[string]map[string][]pageCallStep{}
	writes := map[string][]pageEntityWrite{}
	uses := make([]map[string]bool, len(view.Edges))
	for i := range view.Nodes {
		if n := &view.Nodes[i]; n.Activation != "" {
			inputs[n.ID] = n
			var paths map[string][]pageCallStep
			_ = json.Unmarshal([]byte(n.CallPaths), &paths)
			witnesses[n.ID] = paths
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
		joined := map[string][]pageCallStep{}
		for id, steps := range witnesses[root] {
			joined[id] = steps
		}
		prefixes := map[string][]pageCallStep{}
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
				write.Steps = append([]pageCallStep(nil), write.Steps...)
				if id != root {
					write.Possible = true // The remote input is an integration match.
					if len(write.Steps) > 0 {
						write.Steps[0].Possible, write.Steps[0].Integration = true, true
					}
					write.Steps = append(append([]pageCallStep(nil), prefixes[id]...), write.Steps...)
				}
				joinedWrites = append(joinedWrites, write)
			}
			if id != root && len(prefixes[id]) > 0 {
				for destination, steps := range witnesses[id] {
					if len(joined[destination]) > 0 || len(steps) == 0 {
						continue
					}
					continuation := append([]pageCallStep(nil), steps...)
					continuation[0].Possible = true
					continuation[0].Integration = true // An endpoint match, not a native call.
					joined[destination] = append(append([]pageCallStep(nil), prefixes[id]...), continuation...)
				}
			}
			for _, at := range paths[id] {
				edge := view.Edges[at]
				uses[at][root] = true
				near[edge.From], near[edge.To] = true, true
				if inputs[edge.To] != nil && !seen[edge.To] {
					if len(prefixes[edge.To]) == 0 {
						prefixes[edge.To] = joined[edge.To]
					}
					queue = append(queue, edge.To)
				}
			}
		}
		delete(near, root)
		node.Writes = joinedWrites
		node.Neighbours = sortedKeys(near)
		if len(joined) > 0 {
			raw, _ := json.Marshal(joined)
			node.CallPaths = string(raw)
		}
	}
	for i := range view.Edges {
		view.Edges[i].Operations = sortedKeys(uses[i])
	}
}

func sortedKeys(values map[string]bool) string {
	keys := make([]string, 0, len(values))
	for id := range values {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	return strings.Join(keys, " ")
}
