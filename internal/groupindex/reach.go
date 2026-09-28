package groupindex

import (
	"slices"
	"sort"

	"github.com/dvordrova/repomap/internal/programindex"
)

// Reach is what one input's handler reaches in its program: every
// declaration its calls lead to, and the data its code reads. It follows a
// structural relation-target edge of kind calls, executes or
// invokes_external resolved exactly or as alternatives, from the handler
// and from every declaration it reaches that way. A read from a function,
// method, lambda or module body into a variable or a type adds that
// variable or type as reached; nothing is followed from it, so reading a
// table never runs the callbacks stored in it. Imports, callbacks handed
// over, decorations and writes are never followed, and a call left
// unresolved is not reach: what its stores name may draw a possible arrow,
// but nothing establishes that it runs.
//
// One rule cuts the walk: a relation resolved as alternatives is not
// followed into another input's handler. A dispatch site such as Redis's
// `call`, which calls one of 94 command handlers, is where every one of
// those inputs is dispatched; from there it is that input's own reach. An
// exact call into another input's handler is still followed: a handler used
// as a helper is the caller's own code.
//
// Derived by Derive from the program's structural edges and the saved
// overlay; never persisted.
type Reach struct {
	OperationID string
	// Subjects are the reached declarations, the handler first, in
	// breadth-first order over the structural edges.
	Subjects []ReachedSubject
	// Edges are the positions in Index.StructuralEdges of every followed
	// relation: each call the walk took, and each read of a reached
	// declaration's code, in the order the walk met them.
	Edges []int
	// Groups are the parts holding reached declarations, by depth and then
	// by the first reached declaration in them.
	Groups []ReachedGroup
	// HandsOver are the callbacks-handed-over edges from reached code into
	// another input's handler: that input is registered by this one.
	// HandedOverBy are the inputs whose reach hands this input's handler
	// over, in operation order. Only a declaration that runs hands over:
	// a table the reach only reads registers nothing.
	HandsOver    []int
	HandedOverBy []string
	// SubArguments are the handler-less inputs declared by code only this
	// reach (and no launch) holds, in operation order: words the handler
	// itself checks (launch.go).
	SubArguments []string
}

// ReachedSubject is one reached declaration and its depth: the fewest calls
// from the handler, a read counting as one.
type ReachedSubject struct {
	SubjectID string
	Depth     int
}

// ReachedGroup is one part the reach enters. Depth is the least depth of
// its reached declarations. Entered lists every followed relation into the
// part from a part reached earlier (at a lower depth); Others counts the
// followed relations into it from parts reached no earlier. No witness is
// chosen: every entering call is listed and the rest are counted.
type ReachedGroup struct {
	GroupID string
	Depth   int
	Entered []Witness
	Others  int
}

// Witness is one followed relation into a part. From is the caller's part.
// A caller off the map stands for the parts reached earlier that reach it,
// within this reach, through declarations off the map only; From is empty
// when only the input's own handler, itself off the map, does.
type Witness struct {
	Edge int
	From []string
}

// DispatchSite is a relation resolved as one of several alternatives: the
// declaration FromSubjectID calls one of Alternatives there. OperationIDs
// are the inputs whose handler is one of them: each is dispatched from
// FromSubjectID, one of len(Alternatives). ReachedFrom lists the inputs
// whose reach holds FromSubjectID, computed only for a site that dispatches
// an input. Sites are in source order.
type DispatchSite struct {
	RelationID    string
	FromSubjectID string
	Location      *programindex.Location
	Alternatives  []string
	OperationIDs  []string
	ReachedFrom   []DispatchReach
}

// DispatchReach is one input reaching a dispatch site, with every followed
// call of its reach on any route from its handler to the site's
// declaration. None is chosen, by length or otherwise.
type DispatchReach struct {
	OperationID string
	Edges       []int
}

// Entry is a declaration execution starts from (a target seed) and where
// the map has it: GroupID is the part holding it, the program's entry part.
// A seed is never asked the role split's box question: a split file gives
// it a grouping row of its own, so only a parts answer that leaves that row
// out (and a follow-up that places it nowhere) keeps it OffMap, with the
// reason its file lists it off the map. The code never picks a part for it.
type Entry struct {
	SubjectID string
	GroupID   string
	OffMap    string
}

// entries places every seed of the target on the map or off it.
func entries(index *Index) []Entry {
	var result []Entry
	for _, seed := range index.Target.Seeds {
		entry := Entry{SubjectID: seed.ObjectID}
		for _, group := range index.Groups {
			if slices.Contains(group.MemberSubjectIDs, seed.ObjectID) {
				entry.GroupID = group.ID
			}
		}
		if entry.GroupID == "" {
			path := ""
			for _, subject := range index.Subjects {
				if subject.ID == seed.ObjectID && subject.Object != nil && subject.Object.Location != nil {
					path = subject.Object.Location.Path
				}
			}
			for _, file := range index.OffMap {
				if slices.Contains(file.SubjectIDs, seed.ObjectID) || len(file.SubjectIDs) == 0 && file.Path == path && path != "" {
					entry.OffMap = file.Reason
					break
				}
			}
		}
		result = append(result, entry)
	}
	return result
}

// reachGraph is the program's execution structure by subject position.
type reachGraph struct {
	index     *Index
	position  map[string]int
	from, to  []int // by structural edge
	exec      [][]int
	reads     [][]int
	hands     [][]int
	executing []bool
	group     []int // by subject; -1 when the subject is in no part
	handlers  map[int][]int
}

func newReachGraph(index *Index) *reachGraph {
	graph := &reachGraph{
		index:     index,
		position:  make(map[string]int, len(index.Subjects)),
		from:      make([]int, len(index.StructuralEdges)),
		to:        make([]int, len(index.StructuralEdges)),
		exec:      make([][]int, len(index.Subjects)),
		reads:     make([][]int, len(index.Subjects)),
		hands:     make([][]int, len(index.Subjects)),
		executing: make([]bool, len(index.Subjects)),
		group:     make([]int, len(index.Subjects)),
		handlers:  make(map[int][]int),
	}
	data := make([]bool, len(index.Subjects))
	for position, subject := range index.Subjects {
		graph.position[subject.ID] = position
		graph.group[position] = -1
		if subject.Object == nil {
			continue
		}
		switch subject.Object.Kind {
		case programindex.ObjectFunction, programindex.ObjectMethod, programindex.ObjectLambda, programindex.ObjectModule:
			graph.executing[position] = true
		case programindex.ObjectVariable, programindex.ObjectType:
			data[position] = true
		}
	}
	for position, group := range index.Groups {
		for _, id := range group.MemberSubjectIDs {
			if subject, ok := graph.position[id]; ok {
				graph.group[subject] = position
			}
		}
	}
	for position, operation := range index.Operations {
		if subject, ok := graph.position[operation.SubjectID]; ok && operation.SubjectID != "" {
			graph.handlers[subject] = append(graph.handlers[subject], position)
		}
	}
	for position, edge := range index.StructuralEdges {
		from, fromKnown := graph.position[edge.FromSubjectID]
		to, toKnown := graph.position[edge.ToSubjectID]
		graph.from[position], graph.to[position] = from, to
		if !fromKnown || !toKnown || edge.Role != EdgeRelationTarget {
			continue
		}
		switch {
		case executionEdge(edge):
			graph.exec[from] = append(graph.exec[from], position)
		case edge.RelationKind == programindex.RelationReads && graph.executing[from] && data[to]:
			graph.reads[from] = append(graph.reads[from], position)
		case edge.RelationKind == programindex.RelationPassesCallback && graph.executing[from]:
			graph.hands[from] = append(graph.hands[from], position)
		}
	}
	return graph
}

// executionEdge says a structural edge is a call the walk follows: a
// relation target of a call, an execution or an outside invocation,
// resolved exactly or as alternatives.
func executionEdge(edge StructuralEdge) bool {
	if edge.Role != EdgeRelationTarget || edge.Resolution == programindex.ResolutionUnresolved {
		return false
	}
	switch edge.RelationKind {
	case programindex.RelationCalls, programindex.RelationExecutes, programindex.RelationInvokesExternal:
		return true
	}
	return false
}

// Derive computes what GroupsIndex derives from the program's structure and
// the saved overlay: each input's reach, the dispatch sites, where the
// program's entries stand, the phases of subjects and connections, and
// which connections go into helpers. It is
// deterministic and idempotent; ProjectAtlas, Hydrate and Build call it, so
// the ordinary run and a saved rendering derive the same values.
func Derive(index *Index) {
	graph := newReachGraph(index)
	index.Reach = make([]Reach, len(index.Operations))
	for position, operation := range index.Operations {
		index.Reach[position] = graph.reach(position, operation)
	}
	graph.handOvers(index.Reach)
	index.Dispatch = graph.dispatchSites(index.Reach)
	index.Entries = entries(index)
	index.Catalogues = catalogues(index)
	index.Launch = graph.launch(index.Reach)
	graph.phases()
}

func (graph *reachGraph) reach(position int, operation Operation) Reach {
	result := Reach{OperationID: operation.ID}
	root, known := graph.position[operation.SubjectID]
	if !known || operation.SubjectID == "" {
		return result
	}
	index := graph.index
	depth := map[int]int{root: 0}
	queue := []int{root}
	for next := 0; next < len(queue); next++ {
		current := queue[next]
		for _, edge := range graph.exec[current] {
			to := graph.to[edge]
			if to != root && index.StructuralEdges[edge].Resolution == programindex.ResolutionAlternatives && len(graph.handlers[to]) > 0 {
				continue
			}
			result.Edges = append(result.Edges, edge)
			if _, seen := depth[to]; !seen {
				depth[to] = depth[current] + 1
				queue = append(queue, to)
			}
		}
	}
	// Reads are terminal: what a reached declaration's code reads is
	// reached, one step deeper, and never walked from.
	reached := slices.Clone(queue)
	for _, reader := range queue {
		for _, edge := range graph.reads[reader] {
			to := graph.to[edge]
			result.Edges = append(result.Edges, edge)
			if _, seen := depth[to]; !seen {
				depth[to] = depth[reader] + 1
				reached = append(reached, to)
			}
		}
	}
	sort.SliceStable(reached, func(i, j int) bool { return depth[reached[i]] < depth[reached[j]] })
	result.Subjects = make([]ReachedSubject, len(reached))
	for i, subject := range reached {
		result.Subjects[i] = ReachedSubject{SubjectID: index.Subjects[subject].ID, Depth: depth[subject]}
	}
	result.Groups = graph.reachedGroups(root, reached, depth, result.Edges)
	for _, current := range queue {
		for _, edge := range graph.hands[current] {
			if to := graph.to[edge]; to != root && len(graph.handlers[to]) > 0 {
				result.HandsOver = append(result.HandsOver, edge)
			}
		}
	}
	return result
}

// reachedGroups lists the parts a reach enters and what enters each.
func (graph *reachGraph) reachedGroups(root int, reached []int, depth map[int]int, edges []int) []ReachedGroup {
	groupDepth := map[int]int{}
	var order []int
	for _, subject := range reached {
		group := graph.group[subject]
		if group < 0 {
			continue
		}
		if _, seen := groupDepth[group]; !seen {
			groupDepth[group] = depth[subject]
			order = append(order, group)
		}
	}
	if len(order) == 0 {
		return nil
	}
	// A caller off the map stands for the parts that reach it through code
	// off the map only; handler marks one the off-map handler reaches so.
	var standIns map[int][]int
	var handler map[int]bool
	offMapCallers := func() {
		standIns, handler = map[int][]int{}, map[int]bool{}
		successors := map[int][]int{}
		for _, edge := range edges {
			from, to := graph.from[edge], graph.to[edge]
			if graph.group[to] < 0 && from != to {
				successors[from] = append(successors[from], to)
			}
		}
		spread := func(start int, mark func(int) bool) {
			queue := []int{start}
			for len(queue) > 0 {
				current := queue[0]
				queue = queue[1:]
				for _, next := range successors[current] {
					if mark(next) {
						queue = append(queue, next)
					}
				}
			}
		}
		if graph.group[root] < 0 {
			spread(root, func(next int) bool {
				if handler[next] {
					return false
				}
				handler[next] = true
				return true
			})
		}
		for _, subject := range reached {
			group := graph.group[subject]
			if group < 0 {
				continue
			}
			spread(subject, func(next int) bool {
				if slices.Contains(standIns[next], group) {
					return false
				}
				standIns[next] = append(standIns[next], group)
				return true
			})
		}
	}
	result := make([]ReachedGroup, len(order))
	slot := make(map[int]int, len(order))
	for position, group := range order {
		result[position] = ReachedGroup{GroupID: graph.index.Groups[group].ID, Depth: groupDepth[group]}
		slot[group] = position
	}
	for _, edge := range edges {
		from, to := graph.from[edge], graph.to[edge]
		into := graph.group[to]
		if into < 0 || graph.group[from] == into {
			continue
		}
		entered := &result[slot[into]]
		if caller := graph.group[from]; caller >= 0 {
			if groupDepth[caller] < entered.Depth {
				entered.Entered = append(entered.Entered, Witness{Edge: edge, From: []string{graph.index.Groups[caller].ID}})
			} else {
				entered.Others++
			}
			continue
		}
		if standIns == nil {
			offMapCallers()
		}
		var parts []string
		for _, group := range standIns[from] {
			if groupDepth[group] < entered.Depth {
				parts = append(parts, graph.index.Groups[group].ID)
			}
		}
		if len(parts) > 0 || from == root || handler[from] {
			entered.Entered = append(entered.Entered, Witness{Edge: edge, From: parts})
		} else {
			entered.Others++
		}
	}
	return result
}

// handOvers fills each reach's HandedOverBy from every reach's HandsOver.
func (graph *reachGraph) handOvers(reaches []Reach) {
	for position := range reaches {
		for _, edge := range reaches[position].HandsOver {
			for _, handled := range graph.handlers[graph.to[edge]] {
				if !slices.Contains(reaches[handled].HandedOverBy, reaches[position].OperationID) {
					reaches[handled].HandedOverBy = append(reaches[handled].HandedOverBy, reaches[position].OperationID)
				}
			}
		}
	}
	for position := range reaches {
		slices.SortFunc(reaches[position].HandedOverBy, func(a, b string) int {
			switch {
			case compactIDLess(a, b, "o"):
				return -1
			case compactIDLess(b, a, "o"):
				return 1
			}
			return 0
		})
	}
}

// dispatchSites lists every relation resolved as alternatives with at
// least two targets, in source order, and who is dispatched there.
func (graph *reachGraph) dispatchSites(reaches []Reach) []DispatchSite {
	index := graph.index
	var sites []DispatchSite
	byRelation := map[string]int{}
	for _, edge := range index.StructuralEdges {
		if edge.Role != EdgeRelationTarget || edge.RelationID == "" || edge.Resolution != programindex.ResolutionAlternatives {
			continue
		}
		position, seen := byRelation[edge.RelationID]
		if !seen {
			position = len(sites)
			byRelation[edge.RelationID] = position
			sites = append(sites, DispatchSite{RelationID: edge.RelationID, FromSubjectID: edge.FromSubjectID, Location: cloneLocation(edge.Location)})
		}
		if !slices.Contains(sites[position].Alternatives, edge.ToSubjectID) {
			sites[position].Alternatives = append(sites[position].Alternatives, edge.ToSubjectID)
		}
	}
	sites = slices.DeleteFunc(sites, func(site DispatchSite) bool { return len(site.Alternatives) < 2 })
	sort.SliceStable(sites, func(i, j int) bool { return locationBefore(sites[i].Location, sites[j].Location) })
	for position := range sites {
		site := &sites[position]
		for operation := range index.Operations {
			if index.Operations[operation].SubjectID != "" && slices.Contains(site.Alternatives, index.Operations[operation].SubjectID) {
				site.OperationIDs = append(site.OperationIDs, index.Operations[operation].ID)
			}
		}
		if len(site.OperationIDs) == 0 {
			continue
		}
		declaration, known := graph.position[site.FromSubjectID]
		if !known {
			continue
		}
		for operation := range reaches {
			if edges, reached := graph.routesTo(reaches[operation], declaration); reached {
				site.ReachedFrom = append(site.ReachedFrom, DispatchReach{OperationID: reaches[operation].OperationID, Edges: edges})
			}
		}
	}
	return sites
}

// routesTo is every followed edge of a reach on a route from its handler to
// the declaration, found by a backward pass inside the reach. The
// declaration is not walked from: a route ends where it is reached.
func (graph *reachGraph) routesTo(reach Reach, declaration int) ([]int, bool) {
	if len(reach.Subjects) == 0 {
		return nil, false
	}
	id := graph.index.Subjects[declaration].ID
	held := false
	for _, subject := range reach.Subjects {
		if subject.SubjectID == id {
			held = true
			break
		}
	}
	if !held {
		return nil, false
	}
	predecessors := map[int][]int{}
	for _, edge := range reach.Edges {
		if graph.from[edge] != declaration {
			predecessors[graph.to[edge]] = append(predecessors[graph.to[edge]], graph.from[edge])
		}
	}
	leads := map[int]bool{declaration: true}
	queue := []int{declaration}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, previous := range predecessors[current] {
			if !leads[previous] {
				leads[previous] = true
				queue = append(queue, previous)
			}
		}
	}
	var edges []int
	for _, edge := range reach.Edges {
		if graph.from[edge] != declaration && leads[graph.to[edge]] {
			edges = append(edges, edge)
		}
	}
	return edges, true
}
