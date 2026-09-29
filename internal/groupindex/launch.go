package groupindex

import (
	"slices"

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
// (could not look inside: its unresolved calls), or nothing, counted only. It is how the inputs
// were found, never a gate: no input outside it is hidden.
//
// Nested are the handler-less inputs declared only inside an input's
// handler reach (its sub-arguments, as SORT's asc) or only by its own
// command's code (its options, as litestream's databases -json): each is
// listed in that input's reading (Reach.SubArguments, Reach.Options) and is
// no tile of its own.
//
// Derived by Derive, never persisted.
type Launch struct {
	Roots     []string
	Functions []LaunchFunction
	Nested    map[string]bool
}

// LaunchFunction is one function the launch walk reaches: its depth from a
// root, Via the structural edge that first reached it (-1 for a root), and
// what it holds: Found its inputs, Unsure positions in Index.Unsure and
// Closed positions in Index.Unresolved.
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
		case subject.Object.Kind == programindex.ObjectModule && index.Target.Language != "c":
			// A C file scope runs nothing at load; its static tables are data.
			root(position)
		case index.Target.Language == "go" && subject.Object.Kind == programindex.ObjectFunction && subject.Object.Name == "init":
			root(position)
		case index.Target.Language == "go" && subject.Object.Kind == programindex.ObjectVariable && len(graph.exec[position]) > 0:
			// A Go package-level variable whose initializer calls is run at
			// load. A C table's rows are constructions, not load-time code.
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
	// A table's inputs are found where the launch looks them up: the first
	// launch function that reads the table.
	readBy := map[string]int{}
	for _, edge := range index.StructuralEdges {
		if edge.Role != EdgeRelationTarget || edge.RelationKind != programindex.RelationReads || edge.Resolution != programindex.ResolutionExact {
			continue
		}
		function, ok := at[edge.FromSubjectID]
		if !ok {
			continue
		}
		if previous, seen := readBy[edge.ToSubjectID]; !seen || function < previous {
			readBy[edge.ToSubjectID] = function
		}
	}
	for _, operation := range index.Operations {
		if operation.DeclaredBy == "" {
			continue
		}
		position, ok := at[operation.DeclaredBy]
		if !ok {
			position, ok = readBy[operation.DeclaredBy]
		}
		if ok {
			result.Functions[position].Found = append(result.Functions[position].Found, operation.ID)
		}
	}
	for position, call := range index.Unsure {
		if function, ok := at[call.SubjectID]; ok {
			result.Functions[function].Unsure = append(result.Functions[function].Unsure, position)
		}
	}
	for position, call := range index.Unresolved {
		if function, ok := at[call.FromSubjectID]; ok {
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
	// A value of another input's words (appendfsync's always) is that
	// input's sub-argument too.
	for _, operation := range index.Operations {
		if operation.ValueOf == "" || result.Nested[operation.ID] {
			continue
		}
		for position := range reaches {
			if reaches[position].OperationID == operation.ValueOf {
				reaches[position].SubArguments = append(reaches[position].SubArguments, operation.ID)
				result.Nested[operation.ID] = true
			}
		}
	}
	var roots []int
	for _, position := range queue {
		if depth[position] == 0 {
			roots = append(roots, position)
		}
	}
	graph.options(reaches, roots, result.Nested)
	return result
}

// options nests a command's own options under it (owner, 2026-09-29: a
// flag belongs to its subcommand), by two code facts:
//
//   - declared on the object an input's own call made: argparse's
//     init.add_argument("--force") on commands.add_parser("init"), unless
//     an input with its own handler is declared on that object too;
//   - declared by code only a case's branch runs: litestream's Main.Run
//     runs (&DatabasesCommand{}).Run(ctx, args) in case "databases", whose
//     own flag set declares -json; or declared in that branch itself.
//
// An option has its input's kind: a setting a subcommand's code reads (a
// replica URL's endpoint) is no flag of it, and a value of another input
// (ValueOf) is that input's already. Code only a branch runs is code the
// walk from the launch's roots reaches only through a call written in a
// case's branch. A line belongs to the branch starting last before it, so a
// branch nested in another is its own, and the line `} else if (…) {`
// closing one guarded block and opening the next is the next's.
func (graph *reachGraph) options(reaches []Reach, roots []int, nested map[string]bool) {
	index := graph.index
	if len(index.Operations) == 0 {
		return
	}
	// Each declaration's branches, and the input at each branch's word.
	branches := map[int][]InputBranch{}
	for _, branch := range index.Branches {
		if position, known := graph.position[branch.SubjectID]; known {
			branches[position] = append(branches[position], branch)
		}
	}
	inputAt := map[string]int{}
	for position, operation := range index.Operations {
		inputAt[operationLocationKey(operation.Location)] = position
	}
	// owner is the input whose branch holds a line of a declaration's code,
	// or -1: the branch starting last at or before the line, the narrower
	// of two starting on it.
	owner := func(subject int, at *programindex.Location) int {
		if at == nil {
			return -1
		}
		var holding *InputBranch
		for i := range branches[subject] {
			branch := &branches[subject][i]
			if branch.Location.Path != at.Path || at.Line < branch.Branch.Line || at.Line > branch.Branch.EndLine {
				continue
			}
			if holding == nil || branch.Branch.Line > holding.Branch.Line ||
				branch.Branch.Line == holding.Branch.Line && branch.Branch.EndLine < holding.Branch.EndLine {
				holding = branch
			}
		}
		if holding == nil {
			return -1
		}
		position, ok := inputAt[operationLocationKey(holding.Location)]
		if !ok || !index.Operations[position].HandlerUnknown {
			return -1
		}
		return position
	}
	branchOf := map[int]int{}
	for from := range graph.exec {
		if len(branches[from]) == 0 {
			continue
		}
		for _, edge := range graph.exec[from] {
			if input := owner(from, index.StructuralEdges[edge].Location); input >= 0 {
				branchOf[edge] = input
			}
		}
	}
	// The launch without the cases' branches.
	launched := map[int]bool{}
	queue := slices.Clone(roots)
	for _, position := range roots {
		launched[position] = true
	}
	for next := 0; next < len(queue); next++ {
		for _, edge := range graph.exec[queue[next]] {
			to := graph.to[edge]
			if _, inBranch := branchOf[edge]; inBranch || launched[to] ||
				index.StructuralEdges[edge].Resolution == programindex.ResolutionAlternatives && len(graph.handlers[to]) > 0 {
				continue
			}
			launched[to] = true
			queue = append(queue, to)
		}
	}
	// What each case's branch runs: from its calls, through calls written in
	// no other case's branch, never into another input's handler through
	// alternatives.
	runs := map[int]map[int]bool{}
	for edge, input := range branchOf {
		if runs[input] == nil {
			runs[input] = map[int]bool{}
		}
		runs[input][graph.to[edge]] = true
	}
	for input, reached := range runs {
		queue := make([]int, 0, len(reached))
		for position := range reached {
			queue = append(queue, position)
		}
		for next := 0; next < len(queue); next++ {
			for _, edge := range graph.exec[queue[next]] {
				to := graph.to[edge]
				if other, inBranch := branchOf[edge]; inBranch && other != input || reached[to] ||
					index.StructuralEdges[edge].Resolution == programindex.ResolutionAlternatives && len(graph.handlers[to]) > 0 {
					continue
				}
				reached[to] = true
				queue = append(queue, to)
			}
		}
	}
	// An object something handled is declared on holds entries of their
	// own: freqtrade's subparsers, on which 33 subcommands are joined to
	// their handlers, are no options of the "command" dest the model named.
	handledOn := map[string]bool{}
	for _, operation := range index.Operations {
		if on := operation.DeclaredOn; on != nil && !operation.HandlerUnknown {
			handledOn[operationLocationKey(on.Location)] = true
		}
	}
	nest := func(input, option int) {
		id := index.Operations[option].ID
		if slices.Contains(reaches[input].Options, id) {
			return
		}
		reaches[input].Options = append(reaches[input].Options, id)
		nested[id] = true
	}
	for position, operation := range index.Operations {
		if !operation.HandlerUnknown || operation.ValueOf != "" {
			continue
		}
		// Declared on the object an input's own call made.
		if on := operation.DeclaredOn; on != nil && !handledOn[operationLocationKey(on.Location)] {
			if input, ok := inputAt[operationLocationKey(on.Location)]; ok && input != position && index.Operations[input].Kind == operation.Kind {
				nest(input, position)
			}
		}
		declaredBy, known := graph.position[operation.DeclaredBy]
		if operation.DeclaredBy == "" || !known {
			continue
		}
		// Declared in a case's branch itself.
		location := operation.Location
		if input := owner(declaredBy, &location); input >= 0 && input != position && index.Operations[input].Kind == operation.Kind {
			nest(input, position)
		}
		// Declared by code only a case's branch runs.
		if launched[declaredBy] {
			continue
		}
		for input := range index.Operations {
			if input != position && runs[input][declaredBy] && index.Operations[input].Kind == operation.Kind {
				nest(input, position)
			}
		}
	}
}
