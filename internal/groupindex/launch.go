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
// Nested are the handler-less inputs an input's handler's own body declares
// (its sub-arguments, as SORT's asc) or only by its own
// command's code (its options, as litestream's databases -json): each is
// listed in that input's reading (Reach.SubArguments, Reach.Options) and is
// no tile of its own. An option the program takes as well (a helper also
// handed the program's own parser) is listed and stays a tile.
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
	// Sub-arguments: a handler-less input the handler's own body declares
	// (its own comparisons and lookups: SORT's asc in sortCommand), and
	// the launch walk does not reach, is that input's, each word once.
	// Code deeper in the reach checks its own words: a key the reach's
	// configuration reads is a setting, a word a helper compares is an
	// input of its own (critic, 2026-09-30: freqtrade's trade had listed
	// every word of its 65 parts as "Words its handler checks").
	for position := range reaches {
		reach := &reaches[position]
		operation := index.Operations[position]
		if operation.SubjectID == "" || operation.HandlerUnknown {
			continue
		}
		if _, launched := at[operation.SubjectID]; launched {
			continue
		}
		words := map[string]bool{}
		for _, word := range index.Operations {
			if !word.HandlerUnknown || word.DeclaredBy != operation.SubjectID || word.ID == reach.OperationID {
				continue
			}
			result.Nested[word.ID] = true
			if words[word.Kind+"\x00"+word.Name] {
				continue
			}
			words[word.Kind+"\x00"+word.Name] = true
			reach.SubArguments = append(reach.SubArguments, word.ID)
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
// flag belongs to its subcommand), by three code facts (the third, handed
// to a parameter, in handedOptions):
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
		if !ok || !index.Operations[position].HandlerUnknown && index.Operations[position].Branch == nil {
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
	// nest lists an option under an input (none when input is -1), and
	// hides its tile when the rule establishing it says the option is only
	// its inputs'.
	nest := func(input, option int, only bool) {
		id := index.Operations[option].ID
		if only {
			nested[id] = true
		}
		if input < 0 || slices.Contains(reaches[input].Options, id) {
			return
		}
		reaches[input].Options = append(reaches[input].Options, id)
	}
	for position, operation := range index.Operations {
		if !operation.HandlerUnknown || operation.ValueOf != "" {
			continue
		}
		// Declared on the object an input's own call made.
		if on := operation.DeclaredOn; on != nil && !handledOn[operationLocationKey(on.Location)] {
			if input, ok := inputAt[operationLocationKey(on.Location)]; ok && input != position && index.Operations[input].Kind == operation.Kind {
				nest(input, position, true)
			}
		}
		declaredBy, known := graph.position[operation.DeclaredBy]
		if operation.DeclaredBy == "" || !known {
			continue
		}
		// Declared in a case's branch itself.
		location := operation.Location
		if input := owner(declaredBy, &location); input >= 0 && input != position && index.Operations[input].Kind == operation.Kind {
			nest(input, position, true)
		}
		// Declared by code only a case's branch runs.
		if launched[declaredBy] {
			continue
		}
		for input := range index.Operations {
			if input != position && runs[input][declaredBy] && index.Operations[input].Kind == operation.Kind {
				nest(input, position, true)
			}
		}
	}
	graph.handedOptions(inputAt, handledOn, nest)
}

// handedOptions nests what a declaration declares on the object it is
// handed (owner's verdict, 2026-09-30) by a third code fact: an input a
// declaration F declares on its own parameter P (a helper's
// command.add_argument("--quiet")), or a row of a table F looks up with
// keys it is handed while it makes calls on P (freqtrade's _build_args
// adds AVAILABLE_CLI_OPTIONS[val] for each val of the optionlist it is
// handed to the parser it is handed), is an option of each input whose
// own call made the object a call of F hands P
// (_build_args(optionlist=ARGS_TRADE, parser=trade_cmd), trade_cmd the
// result of subparsers.add_parser("trade")); a row only under the calls
// whose keys name it. It is no tile of its own only when every call of F
// is followed: F reached by exact calls alone, each handing P an input's
// object and, for a row, a list of keys the code wrote; and, for a row, F
// alone reads the table. A call handing a parameter, a spread or a list
// with no rows keeps the tile.
func (graph *reachGraph) handedOptions(inputAt map[string]int, handledOn map[string]bool, nest func(input, option int, only bool)) {
	index := graph.index
	handed := index.Handed
	if len(handed.Calls) == 0 {
		return
	}
	callsOf := map[string][]HandedCall{}
	for _, call := range handed.Calls {
		callsOf[call.ToSubjectID] = append(callsOf[call.ToSubjectID], call)
	}
	onParameter := map[string]ParameterCall{}
	parametersOf := map[string][]int{}
	for _, call := range handed.OnParameter {
		onParameter[operationLocationKey(call.Location)] = call
		if !slices.Contains(parametersOf[call.SubjectID], call.Parameter) {
			parametersOf[call.SubjectID] = append(parametersOf[call.SubjectID], call.Parameter)
		}
	}
	// objectInput is the input of a kind whose own call made the object at
	// a site, or -1: none, the option itself, or an object something
	// handled is declared on.
	objectInput := func(at programindex.Location, kind string, option int) int {
		key := operationLocationKey(at)
		input, ok := inputAt[key]
		if !ok || handledOn[key] || input == option || index.Operations[input].Kind != kind {
			return -1
		}
		return input
	}
	// Declared on a parameter.
	for position, operation := range index.Operations {
		if !operation.HandlerUnknown || operation.ValueOf != "" {
			continue
		}
		call, ok := onParameter[operationLocationKey(operation.Location)]
		if !ok || call.SubjectID != operation.DeclaredBy {
			continue
		}
		calls := callsOf[call.SubjectID]
		only := len(calls) > 0 && !handed.Unfollowed[call.SubjectID]
		for _, into := range calls {
			input := -1
			if at, made := into.Made[call.Parameter]; made {
				input = objectInput(at, operation.Kind, position)
			}
			if input < 0 {
				only = false
				continue
			}
			nest(input, position, false)
		}
		if only {
			nest(-1, position, true)
		}
	}
	// A table's rows, looked up with keys a declaration is handed.
	readers := map[string][]string{}
	for _, call := range handed.Calls {
		for table := range call.Keys {
			if !slices.Contains(readers[table], call.ToSubjectID) {
				readers[table] = append(readers[table], call.ToSubjectID)
			}
		}
	}
	if len(readers) == 0 {
		return
	}
	readAlso := map[string]bool{}
	for _, edge := range index.StructuralEdges {
		if edge.Role == EdgeRelationTarget && edge.RelationKind == programindex.RelationReads && len(readers[edge.ToSubjectID]) > 0 && !slices.Contains(readers[edge.ToSubjectID], edge.FromSubjectID) {
			readAlso[edge.ToSubjectID] = true
		}
	}
	rowAt := map[string]TableRowKey{}
	for _, row := range handed.Rows {
		rowAt[operationLocationKey(row.Location)] = row
	}
	for position, operation := range index.Operations {
		row, ok := rowAt[operationLocationKey(operation.Location)]
		if !ok || !operation.HandlerUnknown || operation.ValueOf != "" || row.TableID != operation.DeclaredBy {
			continue
		}
		table := row.TableID
		only, listed := !readAlso[table], false
		for _, reader := range slices.Sorted(slices.Values(readers[table])) {
			parameters := parametersOf[reader]
			if len(parameters) != 1 || handed.Unfollowed[reader] {
				only = false
				continue
			}
			for _, into := range callsOf[reader] {
				keys, known := into.Keys[table]
				if !known || keys == nil {
					only = false
					continue
				}
				if !slices.Contains(keys, row.Key) {
					continue
				}
				input := -1
				if at, made := into.Made[parameters[0]]; made {
					input = objectInput(at, operation.Kind, position)
				}
				if input < 0 {
					only = false
					continue
				}
				nest(input, position, false)
				listed = true
			}
		}
		if only && listed {
			nest(-1, position, true)
		}
	}
}
