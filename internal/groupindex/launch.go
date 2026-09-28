package groupindex

import (
	"github.com/dvordrova/repomap/internal/programindex"
)

// Launch is the walk from where the program starts: each seed, and the
// code the language runs at load (a Go init function and package-level
// variable initializers, a Python, JS or Clojure module body), as roots
// only, never as seeds. It follows the Reach rule: exact and alternative
// calls, never a hand-over, never into another input's handler through
// alternatives; a read is not followed. Structure stops it, not a depth.
//
// For each function it reaches it says what the function holds: the
// inputs it declares or registers (found), the calls that may declare one
// the reading could not decide (unsure), the calls the code cannot follow
// (could not look inside), or nothing, counted only. It is how the inputs
// were found, never a gate: no input outside it is hidden.
//
// Nested are the handler-less inputs declared only inside an input's
// handler reach (its sub-arguments, as SORT's asc): each is listed in that
// input's reading (Reach.SubArguments) and is no tile of its own.
//
// Derived by Derive, never persisted.
type Launch struct {
	Roots     []string
	Functions []LaunchFunction
	Nested    map[string]bool
}

// LaunchFunction is one function the launch walk reaches: its depth from a
// root, Via the structural edge that first reached it (-1 for a root), and
// what it holds.
type LaunchFunction struct {
	SubjectID string
	Depth     int
	Via       int
	Found     []string
	Unsure    []int
	Closed    []int
}

// Outcome is found, unsure, closed or nothing, the first that holds.
func (function LaunchFunction) Outcome() string {
	switch {
	case len(function.Found) > 0:
		return "found"
	case len(function.Unsure) > 0:
		return "unsure"
	case len(function.Closed) > 0:
		return "closed"
	}
	return "nothing"
}

func (graph *reachGraph) launch(reaches []Reach) Launch {
	index := graph.index
	result := Launch{Nested: map[string]bool{}}
	depth := map[int]int{}
	var queue []int
	via := map[int]int{}
	root := func(position int) {
		if _, seen := depth[position]; seen || !graph.executing[position] && len(graph.exec[position]) == 0 {
			return
		}
		depth[position], via[position] = 0, -1
		queue = append(queue, position)
		result.Roots = append(result.Roots, index.Subjects[position].ID)
	}
	for _, seed := range index.Target.Seeds {
		if position, ok := graph.position[seed.ObjectID]; ok {
			root(position)
		}
	}
	for position, subject := range index.Subjects {
		if subject.Object == nil {
			continue
		}
		switch {
		case subject.Object.Kind == programindex.ObjectModule:
			root(position)
		case index.Target.Language == "go" && subject.Object.Kind == programindex.ObjectFunction && subject.Object.Name == "init":
			root(position)
		case subject.Object.Kind == programindex.ObjectVariable && len(graph.exec[position]) > 0:
			// A package-level variable whose initializer calls is run at load.
			root(position)
		}
	}
	for next := 0; next < len(queue); next++ {
		current := queue[next]
		for _, edge := range graph.exec[current] {
			to := graph.to[edge]
			if index.StructuralEdges[edge].Resolution == programindex.ResolutionAlternatives && len(graph.handlers[to]) > 0 {
				continue
			}
			if _, seen := depth[to]; seen {
				continue
			}
			depth[to], via[to] = depth[current]+1, edge
			queue = append(queue, to)
		}
	}
	at := map[string]int{}
	for _, position := range queue {
		at[index.Subjects[position].ID] = len(result.Functions)
		result.Functions = append(result.Functions, LaunchFunction{SubjectID: index.Subjects[position].ID, Depth: depth[position], Via: via[position]})
	}
	for _, operation := range index.Operations {
		if position, ok := at[operation.DeclaredBy]; ok && operation.DeclaredBy != "" {
			result.Functions[position].Found = append(result.Functions[position].Found, operation.ID)
		}
	}
	for position, call := range index.Unsure {
		if function, ok := at[call.SubjectID]; ok && call.SubjectID != "" {
			result.Functions[function].Unsure = append(result.Functions[function].Unsure, position)
		}
	}
	for position, edge := range index.StructuralEdges {
		if edge.Role != EdgeRelationPattern || edge.Resolution != programindex.ResolutionUnresolved || edge.RelationKind != programindex.RelationCalls {
			continue
		}
		if function, ok := at[edge.FromSubjectID]; ok {
			result.Functions[function].Closed = append(result.Functions[function].Closed, position)
		}
	}
	// Sub-arguments: a handler-less input whose declaring code only an
	// input's handler reaches, and the launch walk does not, is that
	// input's.
	for position := range reaches {
		reach := &reaches[position]
		reached := map[string]bool{}
		for _, subject := range reach.Subjects {
			reached[subject.SubjectID] = true
		}
		if len(reached) == 0 {
			continue
		}
		for _, operation := range index.Operations {
			if !operation.HandlerUnknown || operation.DeclaredBy == "" || operation.ID == reach.OperationID || !reached[operation.DeclaredBy] {
				continue
			}
			if _, launched := at[operation.DeclaredBy]; launched {
				continue
			}
			reach.SubArguments = append(reach.SubArguments, operation.ID)
			result.Nested[operation.ID] = true
		}
	}
	return result
}
