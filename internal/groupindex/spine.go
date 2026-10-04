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
	// Resumes is, for a step the walk reads after the route through an
	// earlier call of the same step ended, that step: the path is back in
	// it, reading its later calls in written order (WalkPathsThen).
	Resumes string `json:"-"`
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
	next := func(step SpineStep) []SpineStep {
		unit := graph.position[step.SubjectID]
		var entered []int
		for _, member := range step.Members {
			entered = append(entered, graph.position[member])
		}
		work := workOf(unit, entered)
		var onward, helpers []SpineStep
		for _, target := range work.units {
			candidate := SpineStep{SubjectID: id(target), Members: ids(work.members[target]), Edge: work.edge[target]}
			if helper(target) {
				candidate.Helper = true
				helpers = append(helpers, candidate)
				continue
			}
			onward = append(onward, candidate)
		}
		// The work is one call only when no helper stands beside it; a
		// step whose one call is into a helper delegates to it too.
		return append(onward, helpers...)
	}
	start := unitOf(root)
	return Walk(SpineStep{SubjectID: id(start), Members: []string{id(root)}, Edge: -1}, next, nil)
}

// Walk follows a path from start, one step at a time: next lists what a
// step's work enters, each a step of its own (a unit, the members of it
// entered, the edge first entering it); a unit already on the path is no
// candidate. A step with no candidate ends the path, one is followed, and
// of several pick chooses the one to follow, or none: the candidates are
// then the path's branches, where it ends. A nil pick ends the path at its
// first split, as an input's spine does. The path never holds a unit twice.
func Walk(start SpineStep, next func(SpineStep) []SpineStep, pick func(SpineStep, []SpineStep) (int, bool)) Spine {
	var result Spine
	visited := map[string]bool{start.SubjectID: true}
	step := start
	for {
		result.Steps = append(result.Steps, step)
		var candidates []SpineStep
		for _, candidate := range next(step) {
			if !visited[candidate.SubjectID] {
				candidates = append(candidates, candidate)
			}
		}
		chosen := -1
		switch {
		case len(candidates) == 0:
			return result
		case len(candidates) == 1:
			chosen = 0
		case pick != nil:
			if at, decided := pick(step, candidates); decided && at >= 0 && at < len(candidates) {
				chosen = at
			}
		}
		if chosen < 0 {
			result.Branches = candidates
			return result
		}
		step = candidates[chosen]
		visited[step.SubjectID] = true
	}
}

// SpinePath is a walked path that may part: its steps, then, where its last
// step's split is followed several ways, each way a path of its own
// (Paths), and the candidates no way follows (Rest). Passed holds, by a
// step's position, the candidates a decided split there did not follow.
type SpinePath struct {
	Steps  []SpineStep
	Paths  []SpinePath
	Rest   []SpineStep
	Passed map[int][]SpineStep
	// Joins are, on a path's last step, each chosen candidate that is where
	// another way of the splits around it starts, with the edge reaching
	// it: the path goes on into it as that way, never walking it twice.
	// Beside ways of its own (Paths) they stand as more ways of the split.
	Joins []SpineStep
	// Then is the route after a split left torn or unanswered at the last
	// step, back in the step whose later calls are not all on the path yet
	// (WalkPathsThen): whichever way runs, it returns there.
	Then *SpinePath `json:"-"`
}

// WalkPaths walks as Walk does, pick naming the candidates to follow at a
// split: one is followed; several are each followed as a path of its own,
// each with its own visited set (the path before the split, the first step
// of every way and its own steps), so no way goes back through the trunk,
// another way's start or its own steps, while two ways may each meet a unit
// further on; none ends the path at a named fork of every candidate. Owner, 2026-09-30: several main paths are allowed where the
// model is torn between them.
func WalkPaths(start SpineStep, next func(SpineStep) []SpineStep, pick func(SpineStep, []SpineStep) []int) SpinePath {
	return walkPath(start, map[string]bool{}, nil, next, pick, nil)
}

// WalkPathsThen walks as WalkPaths does and, where a route ends, goes back
// to the last step that chose among several calls and reads the calls it
// writes after the chosen one, in written order (later): one goes on with
// no question, several are a split of that step among them, asked and
// followed as any split is. "luaD_call: first luaD_precall, then, only if
// its result says so, luaV_execute."
func WalkPathsThen(start SpineStep, next func(SpineStep) []SpineStep, pick func(SpineStep, []SpineStep) []int, later func(step, chosen SpineStep, candidates []SpineStep) []SpineStep) SpinePath {
	return walkPath(start, map[string]bool{}, nil, next, pick, later)
}

// resumeFrame is a step that chose one of several calls, where it stands
// in its path, and the calls it writes after the chosen one.
type resumeFrame struct {
	in    *SpinePath
	step  SpineStep
	at    int
	later []SpineStep
}

// walkPath walks one path; starts are the first steps of the other ways of
// the splits around it, which it may go on into, as a candidate of its own,
// never walking them twice. Chosen alone, the path ends there joining that
// way (5.1.5's script way reaching lua_pcall, the REPL way's start, through
// docall); chosen beside a way of its own, the split is torn, the join one
// of its ways.
func walkPath(step SpineStep, seen map[string]bool, starts map[string]bool, next func(SpineStep) []SpineStep, pick func(SpineStep, []SpineStep) []int, later func(step, chosen SpineStep, candidates []SpineStep) []SpineStep) SpinePath {
	visited := make(map[string]bool, len(seen)+1)
	for id := range seen {
		visited[id] = true
	}
	visited[step.SubjectID] = true
	own := step.SubjectID
	joinable := func(id string) bool { return starts[id] && id != own }
	var result SpinePath
	cur := &result
	var frames []resumeFrame
	// resumed, while not nil, are the later calls of the frame the path is
	// back in, standing for the step's own candidates.
	var resumed *resumeFrame
	// backTo is the last frame whose later calls are not all on the path.
	backTo := func() *resumeFrame {
		for len(frames) > 0 {
			frame := frames[len(frames)-1]
			frames = frames[:len(frames)-1]
			for _, candidate := range frame.later {
				if !visited[candidate.SubjectID] {
					return &frame
				}
			}
		}
		return nil
	}
	for {
		asker := step
		var candidates []SpineStep
		if resumed != nil {
			asker = resumed.step
			for _, candidate := range resumed.later {
				if !visited[candidate.SubjectID] {
					candidates = append(candidates, candidate)
				}
			}
		} else {
			cur.Steps = append(cur.Steps, step)
			for _, candidate := range next(step) {
				if !visited[candidate.SubjectID] || joinable(candidate.SubjectID) {
					candidates = append(candidates, candidate)
				}
			}
		}
		if len(candidates) == 0 {
			// The route ends; the path goes back to the last step whose
			// later calls are not all on it yet.
			if resumed = backTo(); resumed == nil {
				return result
			}
			continue
		}
		var chosen []int
		if len(candidates) == 1 {
			chosen = []int{0}
		} else if pick != nil {
			for _, at := range pick(asker, candidates) {
				if at >= 0 && at < len(candidates) && !containsInt(chosen, at) {
					chosen = append(chosen, at)
				}
			}
		}
		// What the path goes on into where another way starts is joined,
		// never walked again; every such choice is kept.
		var walked []int
		for _, at := range chosen {
			if joinable(candidates[at].SubjectID) {
				cur.Joins = append(cur.Joins, candidates[at])
				continue
			}
			walked = append(walked, at)
		}
		back := resumed
		resumed = nil
		switch {
		case len(chosen) == 0:
			if back != nil {
				// A step's later calls left undecided stay its passed
				// calls; the path ends where it was.
				return result
			}
			cur.Rest = candidates
			if resumed = backTo(); resumed == nil {
				return result
			}
			cur.Then = &SpinePath{}
			cur = cur.Then
			continue
		case len(walked) == 0:
			if back == nil {
				cur.keepPassed(candidates, chosen)
			}
			return result
		case len(walked) == 1 && len(cur.Joins) == 0:
			// A decided split keeps what it did not follow.
			if back == nil {
				cur.keepPassed(candidates, chosen)
			}
			chosenStep := candidates[walked[0]]
			parent, parentAt, in := step, len(cur.Steps)-1, cur
			if back != nil {
				parent, parentAt, in = back.step, back.at, back.in
				chosenStep.Resumes = back.step.SubjectID
				in.unpass(parentAt, chosenStep.SubjectID)
			}
			if later != nil && len(candidates) > 1 {
				if after := later(parent, candidates[walked[0]], candidates); len(after) > 0 {
					frames = append(frames, resumeFrame{in: in, step: parent, at: parentAt, later: after})
				}
			}
			step = chosenStep
			visited[step.SubjectID] = true
			continue
		}
		if back != nil {
			// A step's later calls the categorizer is torn between stay its
			// passed calls: the path ends where it was.
			return result
		}
		// The split is torn: each way of its own is walked apart, and none
		// walks into another's first step, though each may go on into it,
		// joining that way; the joins chosen here stand beside them.
		ways := make(map[string]bool, len(starts)+len(walked))
		for id := range starts {
			ways[id] = true
		}
		for _, at := range walked {
			visited[candidates[at].SubjectID] = true
			ways[candidates[at].SubjectID] = true
		}
		for _, at := range walked {
			cur.Paths = append(cur.Paths, walkPath(candidates[at], visited, ways, next, pick, later))
		}
		for position, candidate := range candidates {
			if !containsInt(chosen, position) {
				cur.Rest = append(cur.Rest, candidate)
			}
		}
		// After the ways, the path goes back to the last step whose later
		// calls are not all on it yet.
		if resumed = backTo(); resumed == nil {
			return result
		}
		cur.Then = &SpinePath{}
		cur = cur.Then
	}
}

// unpass takes a step read after its parent's route back out of the
// parent's passed calls.
func (result *SpinePath) unpass(at int, subjectID string) {
	if result.Passed == nil {
		return
	}
	kept := result.Passed[at][:0]
	for _, candidate := range result.Passed[at] {
		if candidate.SubjectID != subjectID {
			kept = append(kept, candidate)
		}
	}
	result.Passed[at] = kept
}

// keepPassed keeps, at a decided split, the candidates the path did not
// follow, by the position of the step deciding it.
func (result *SpinePath) keepPassed(candidates []SpineStep, chosen []int) {
	if len(candidates) < 2 {
		return
	}
	if result.Passed == nil {
		result.Passed = map[int][]SpineStep{}
	}
	at := len(result.Steps) - 1
	for position, candidate := range candidates {
		if !containsInt(chosen, position) {
			result.Passed[at] = append(result.Passed[at], candidate)
		}
	}
}

// Units maps a declaration to the unit a walk steps through: a method whose
// owner is a class is that class, which folds its methods (Worker(args),
// worker.run() and worker.exit() are one step); any other declaration is
// its own unit.
func Units(index *Index) func(subjectID string) string {
	objects := make(map[string]*ObjectFacts, len(index.Subjects))
	for _, subject := range index.Subjects {
		objects[subject.ID] = subject.Object
	}
	return func(subjectID string) string {
		object := objects[subjectID]
		if object != nil && object.Kind == programindex.ObjectMethod && object.OwnerID != "" {
			if owner := objects[object.OwnerID]; owner != nil && owner.Kind == programindex.ObjectType {
				return object.OwnerID
			}
		}
		return subjectID
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
