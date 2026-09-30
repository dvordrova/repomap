package groupindex

import (
	"github.com/dvordrova/repomap/internal/programindex"
)

// Spine is an input's flow as a reader follows it from its handler: the
// steps whose work is one call into the next, then where the work splits
// (critic, 2026-09-30: freqtrade's trade had shown its handler's first
// calls and "Reaches 65 more parts deeper", the bot loop hidden behind the
// count). A step is a function, or a class with the methods of it the
// previous step calls: a constructor call and its class are one step
// (Worker(args), worker.run(), worker.exit() are Worker). A step's work is
// its calls, within the reach, into the repository's other steps; a call
// into the outside, a read and a callable handed over (a registration:
// signal.signal(term_handler) belongs under Registers) are no work. While
// a step's work is one other step, the spine follows it; where it is
// several, those are the spine's branches, each named with the members of
// it the step calls. A step into a helper (the helper question: it serves
// others' work) is a branch too, marked so a reading may fold it by name:
// the work is one call only when it is the step's only call. Derived by Derive, never persisted.
type Spine struct {
	Steps    []SpineStep
	Branches []SpineStep
}

// SpineStep is one step of a spine: its declaration (a function or a
// class), the members of it the previous step calls (for a class: its
// methods and itself, the constructor's call; for a function: itself), and
// the edge first entering it from the previous step (-1 for the handler).
type SpineStep struct {
	SubjectID string
	Members   []string
	Edge      int
	// Helper marks a branch into a declaration the helper question decided
	// serves others' work: named beside the branches, never followed.
	Helper bool
}

// spine derives an input's spine from its reach.
func (graph *reachGraph) spine(reach Reach) Spine {
	index := graph.index
	if len(reach.Subjects) == 0 {
		return Spine{}
	}
	reached := make(map[int]bool, len(reach.Subjects))
	for _, subject := range reach.Subjects {
		if position, ok := graph.position[subject.SubjectID]; ok {
			reached[position] = true
		}
	}
	// An input a case declares is handled in its case's lines: the
	// handler's calls outside them are no step of its (branch.go).
	var operation Operation
	for _, candidate := range index.Operations {
		if candidate.ID == reach.OperationID {
			operation = candidate
		}
	}
	rootOf := -1
	// A method's step is its class; any other declaration is its own.
	unitOf := func(position int) int {
		object := index.Subjects[position].Object
		if object != nil && object.Kind == programindex.ObjectMethod && object.OwnerID != "" {
			if owner, ok := graph.position[object.OwnerID]; ok {
				if ownerObject := index.Subjects[owner].Object; ownerObject != nil && ownerObject.Kind == programindex.ObjectType {
					return owner
				}
			}
		}
		return position
	}
	// A helper step serves others' work (the helper question): a class is
	// one when every member of it the reach holds is.
	helper := func(unit int) bool {
		interpretation := index.Subjects[unit].Interpretation
		return interpretation != nil && interpretation.Helper
	}
	outside := func(position int) bool {
		object := index.Subjects[position].Object
		return object == nil || object.Kind == programindex.ObjectExternalSymbol
	}
	// A step's members: the handler alone, or what the previous step's
	// calls enter of it. Its work: every step its members' calls within
	// the reach enter, with the members entered and the first edge, a
	// class's own methods called from its members being its members too.
	type work struct {
		units   []int
		members map[int][]int
		edge    map[int]int
	}
	workOf := func(unit int, entered []int) work {
		result := work{members: map[int][]int{}, edge: map[int]int{}}
		seen := map[int]bool{}
		queue := append([]int(nil), entered...)
		for _, member := range queue {
			seen[member] = true
		}
		for next := 0; next < len(queue); next++ {
			for _, edge := range graph.exec[queue[next]] {
				to := graph.to[edge]
				if !reached[to] || outside(to) || index.StructuralEdges[edge].RelationKind == programindex.RelationPassesCallback || graph.outsideBranch(operation, queue[next], rootOf, edge) {
					continue
				}
				target := unitOf(to)
				if target == unit {
					if !seen[to] {
						seen[to] = true
						queue = append(queue, to)
					}
					continue
				}
				if _, known := result.edge[target]; !known {
					result.edge[target] = edge
					result.units = append(result.units, target)
				}
				if !containsInt(result.members[target], to) {
					result.members[target] = append(result.members[target], to)
				}
			}
		}
		return result
	}
	root, ok := graph.position[reach.Subjects[0].SubjectID]
	if !ok {
		return Spine{}
	}
	rootOf = root
	id := func(position int) string { return index.Subjects[position].ID }
	ids := func(positions []int) []string {
		result := make([]string, 0, len(positions))
		for _, position := range positions {
			result = append(result, id(position))
		}
		return result
	}
	var result Spine
	unit, entered, edge := unitOf(root), []int{root}, -1
	visited := map[int]bool{}
	for {
		visited[unit] = true
		result.Steps = append(result.Steps, SpineStep{SubjectID: id(unit), Members: ids(entered), Edge: edge})
		next := workOf(unit, entered)
		var onward, helpers []int
		for _, target := range next.units {
			switch {
			case visited[target]:
			case helper(target):
				helpers = append(helpers, target)
			default:
				onward = append(onward, target)
			}
		}
		// The work is one call only when no helper stands beside it; a step
		// whose one call is into a helper delegates to it too.
		if len(onward)+len(helpers) == 1 {
			onward, helpers = append(onward, helpers...), nil
		} else if len(onward) == 1 {
			onward = append(onward, -1)
		}
		if len(onward) != 1 {
			onward = slicesDeleteSentinel(onward)
			for _, target := range onward {
				result.Branches = append(result.Branches, SpineStep{SubjectID: id(target), Members: ids(next.members[target]), Edge: next.edge[target]})
			}
			for _, target := range helpers {
				result.Branches = append(result.Branches, SpineStep{SubjectID: id(target), Members: ids(next.members[target]), Edge: next.edge[target], Helper: true})
			}
			return result
		}
		unit, entered, edge = onward[0], next.members[onward[0]], next.edge[onward[0]]
	}
}

func containsInt(values []int, value int) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

// slicesDeleteSentinel drops the -1 marking work that is more than one
// call.
func slicesDeleteSentinel(values []int) []int {
	result := values[:0]
	for _, value := range values {
		if value >= 0 {
			result = append(result, value)
		}
	}
	return result
}

