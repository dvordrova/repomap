package report

import (
	"cmp"
	"slices"
	"strings"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// pageFlowCall is one call a declaration makes, as its flow reads it (owner,
// 2026-09-29, the designer's flow v2): its calls stand in the order they
// are written (the first call site's line, then column), each callee once
// with every place it is called; a dispatch site that calls one of several
// declarations is one call. The reading column renders them in this order,
// a call opening in place to its callee's own flow; it sorts nothing.
//
//   - Decl is the callee in the reading's declarations; Name a call into
//     code the report names no declaration for (a library's fork or write),
//     a plain row.
//   - One are, for a dispatch site, the declarations it calls one of.
//   - Helper folds the call behind "Show helper calls": the callee is a
//     declaration the helper question decided serves the work of others,
//     and it stands in a part most of the program's parts call into, or in
//     the caller's own part. A helper into any other part stays: its part
//     says what it is for.
type pageFlowCall struct {
	Decl *int   `json:"decl,omitempty"`
	Name string `json:"name,omitempty"`
	// Lib is the library a named call goes to ("sys/wait.h" for wait3).
	Lib      string            `json:"lib,omitempty"`
	One      []int             `json:"one,omitempty"`
	Kind     string            `json:"kind,omitempty"`
	Possible bool              `json:"possible,omitempty"`
	Helper   bool              `json:"helper,omitempty"`
	Sites    []pageReadingSite `json:"sites,omitempty"`
}

// flowKinds are the relations a flow reads as calls.
var flowKinds = map[programindex.RelationKind]bool{
	programindex.RelationCalls: true, programindex.RelationExecutes: true,
	programindex.RelationPassesCallback: true, programindex.RelationInvokesExternal: true,
}

// pageFlowIndex is one program's calls by caller, in the order they are
// written, and the parts most of its parts call into.
type pageFlowIndex struct {
	byCaller map[string][]int
	shared   map[string]bool
}

// flowIndex builds a program's flow index once.
func (builder *pageBuilder) flowIndex(index *groupindex.Index) *pageFlowIndex {
	if builder.flows == nil {
		builder.flows = map[string]*pageFlowIndex{}
	}
	if cached := builder.flows[index.Target.ID]; cached != nil {
		return cached
	}
	flow := &pageFlowIndex{byCaller: map[string][]int{}, shared: map[string]bool{}}
	groupOf := builder.edgesBetweenGroups(*index).groupOf
	callers := map[string]map[string]bool{}
	for position, edge := range index.StructuralEdges {
		if edge.Role != groupindex.EdgeRelationTarget || !flowKinds[edge.RelationKind] || edge.Resolution == programindex.ResolutionUnresolved {
			continue
		}
		flow.byCaller[edge.FromSubjectID] = append(flow.byCaller[edge.FromSubjectID], position)
		from, to := groupOf[edge.FromSubjectID], groupOf[edge.ToSubjectID]
		if from != "" && to != "" && from != to {
			if callers[to] == nil {
				callers[to] = map[string]bool{}
			}
			callers[to][from] = true
		}
	}
	// "Most of the program" is decided from its parts: a part more than
	// half of the others call into (Redis's Server core state, called from
	// 18 of 21 parts), never by its name.
	for group, from := range callers {
		if 2*len(from) > len(index.Groups)-1 {
			flow.shared[group] = true
		}
	}
	for caller, edges := range flow.byCaller {
		slices.SortStableFunc(edges, func(a, b int) int {
			return compareSites(index.StructuralEdges[a].Location, index.StructuralEdges[b].Location)
		})
		flow.byCaller[caller] = edges
	}
	builder.flows[index.Target.ID] = flow
	return flow
}

// compareSites orders call sites as they are written: file, line, column;
// a call with no place after those with one.
func compareSites(a, b *programindex.Location) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return 1
	case b == nil:
		return -1
	}
	return cmp.Or(strings.Compare(a.Path, b.Path), cmp.Compare(a.Line, b.Line), cmp.Compare(a.Column, b.Column))
}

// flowOf is a declaration's calls in the order they are written. declare
// names a callee in the reading (its position, or -1 when the report names
// no declaration for it); partOf is the part a declaration stands in.
func (builder *pageBuilder) flowOf(index *groupindex.Index, callerID string, declare func(string) int) []pageFlowCall {
	flow := builder.flowIndex(index)
	groupOf := builder.edgesBetweenGroups(*index).groupOf
	callerPart := groupOf[callerID]
	var calls []pageFlowCall
	at := map[string]int{}
	byRelation := map[string]int{}
	for _, position := range flow.byCaller[callerID] {
		edge := index.StructuralEdges[position]
		var site *pageReadingSite
		if edge.Location != nil {
			anchor := builder.links.anchor(edge.Location.Path, edge.Location.Line, edge.Location.Column)
			site = &pageReadingSite{At: anchor.Text, Href: anchor.Href, Open: anchor.Open}
		}
		kind := string(edge.RelationKind)
		if edge.RelationKind == programindex.RelationCalls {
			kind = ""
		}
		possible := edge.Resolution != programindex.ResolutionExact
		// A dispatch site is one call, one of the declarations it calls.
		if edge.Resolution == programindex.ResolutionAlternatives && edge.RelationID != "" {
			if listed, seen := byRelation[edge.RelationID]; seen {
				if callee := declare(edge.ToSubjectID); callee >= 0 && !slices.Contains(calls[listed].One, callee) {
					calls[listed].One = append(calls[listed].One, callee)
				}
				continue
			}
		}
		callee := declare(edge.ToSubjectID)
		key := edge.ToSubjectID + "\x00" + kind
		if listed, seen := at[key]; seen && calls[listed].One == nil {
			if site != nil && !slices.ContainsFunc(calls[listed].Sites, func(other pageReadingSite) bool { return other.At == site.At }) {
				calls[listed].Sites = append(calls[listed].Sites, *site)
			}
			calls[listed].Possible = calls[listed].Possible && possible
			continue
		}
		call := pageFlowCall{Kind: kind, Possible: possible}
		if site != nil {
			call.Sites = []pageReadingSite{*site}
		}
		if callee >= 0 {
			call.Decl = &callee
		} else if ref, known := builder.subject(index.Target.ID, edge.ToSubjectID); known {
			call.Name, _ = builder.subjectDisplay(ref.subject)
			if object := ref.subject.Object; object != nil && object.External != nil && object.External.Name != "" {
				call.Name, call.Lib = object.External.Name, object.External.PackagePath
				if object.External.Receiver != "" {
					call.Name = object.External.Receiver + "." + call.Name
				}
			}
		}
		if call.Decl == nil && call.Name == "" {
			continue
		}
		if ref, known := builder.subject(index.Target.ID, edge.ToSubjectID); known && ref.subject.Interpretation != nil && ref.subject.Interpretation.Helper {
			part := groupOf[edge.ToSubjectID]
			call.Helper = part != "" && (flow.shared[part] || part == callerPart)
		}
		if edge.Resolution == programindex.ResolutionAlternatives && edge.RelationID != "" {
			byRelation[edge.RelationID] = len(calls)
			if callee >= 0 {
				call.One = []int{callee}
			}
			call.Decl = nil
		}
		at[key] = len(calls)
		calls = append(calls, call)
	}
	// A dispatch site of one declaration is a call of it.
	for i := range calls {
		if len(calls[i].One) == 1 {
			calls[i].Decl, calls[i].One = &calls[i].One[0], nil
		}
	}
	return calls
}

// waysIn are the ways a request reaches one dispatch site, one per outer
// input in the order of their kinds (requests first), each by its shortest
// route: through a callable it hands over when it hands one over (the
// callable's shortest run to the site), else by its own calls. A site no
// outer input is established for is one way, the site alone.
func (builder *pageBuilder) waysIn(index *groupindex.Index, site groupindex.DispatchSite, decls *pathDecls, inputNode func(string) string, kindOf map[string]string) []pageWay {
	handler := map[string]string{}
	for _, operation := range index.Operations {
		handler[operation.ID] = operation.SubjectID
	}
	best := map[string]pageWay{}
	length := map[string]int{}
	var order []string
	for _, outer := range site.Outer {
		node, root := inputNode(outer.OperationID), handler[outer.OperationID]
		if root == "" {
			continue
		}
		var way pageWay
		score := 0
		if outer.Registered == "" {
			chain := chainOf(index, outer.Edges, root, site.FromSubjectID)
			if chain == nil {
				continue
			}
			way = pageWay{Input: node, Chain: decls.all(chain)}
			// A way by calls alone is taken only when no callable is
			// handed over.
			score = 1_000_000 + len(chain)
		} else {
			passing := index.StructuralEdges[outer.HandOver].FromSubjectID
			by := chainOf(index, outer.Registering, root, passing)
			calls := chainOf(index, outer.Edges, outer.Registered, site.FromSubjectID)
			if by == nil || calls == nil {
				continue
			}
			way = pageWay{Input: node, Chain: decls.all(append([]string{root}, calls...)), Hop: 1, By: decls.all(by)}
			score = len(calls)
		}
		if previous, seen := length[node]; seen && previous <= score {
			continue
		}
		if _, seen := length[node]; !seen {
			order = append(order, node)
		}
		best[node], length[node] = way, score
	}
	slices.SortStableFunc(order, func(a, b string) int { return outerKindRank(kindOf[a]) - outerKindRank(kindOf[b]) })
	var ways []pageWay
	for _, node := range order {
		ways = append(ways, best[node])
	}
	if len(ways) == 0 {
		ways = append(ways, pageWay{Chain: []int{decls.of(site.FromSubjectID)}})
	}
	return ways
}

// all names declarations by their positions in the reading.
func (decls *pathDecls) all(subjects []string) []int {
	positions := make([]int, len(subjects))
	for i, subject := range subjects {
		positions[i] = decls.of(subject)
	}
	return positions
}

// chainOf is the shortest run of the given calls from one declaration to
// another, both included, in call order; nil when they do not join them.
// Calls are taken in the order they are given.
func chainOf(index *groupindex.Index, edges []int, from, to string) []string {
	if from == to {
		return []string{from}
	}
	next := map[string][]string{}
	for _, position := range edges {
		edge := index.StructuralEdges[position]
		next[edge.FromSubjectID] = append(next[edge.FromSubjectID], edge.ToSubjectID)
	}
	previous := map[string]string{from: ""}
	queue := []string{from}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, callee := range next[current] {
			if _, seen := previous[callee]; seen {
				continue
			}
			previous[callee] = current
			if callee == to {
				chain := []string{to}
				for at := current; at != ""; at = previous[at] {
					chain = append([]string{at}, chain...)
				}
				return chain
			}
			queue = append(queue, callee)
		}
	}
	return nil
}

// markWaysFrom names, for each way but the first, the declaration where it
// leaves the first: the one before the first declaration both run
// (syncWithMaster, before createClient), or the way's first when they
// share none.
func markWaysFrom(ways []pageWay) {
	if len(ways) < 2 {
		return
	}
	full := func(way pageWay) []int {
		if way.Hop > 0 {
			return append(slices.Clone(way.By), way.Chain[way.Hop:]...)
		}
		return way.Chain
	}
	first := full(ways[0])
	for i := 1; i < len(ways); i++ {
		route := full(ways[i])
		from := route[0]
		for at, decl := range route {
			if slices.Contains(first, decl) {
				if at > 0 {
					from = route[at-1]
				}
				break
			}
			from = decl
		}
		ways[i].From = &from
	}
}
