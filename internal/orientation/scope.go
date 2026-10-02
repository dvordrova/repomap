package orientation

import (
	"fmt"
	"slices"
	"strings"

	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// flowGraph is what a program's Main flow walks (path.go): its executing
// declarations outside its test sources, stepped through by unit (a class
// folds its methods, groupindex.Units), with every exact call, every
// alternative of a dispatch site and every hand-over (a callable passed as
// an argument, a registration fact's owner handing its object), each with
// how the code reaches it. The walk trusts earlier stages: a helper the
// helper question decided serves others' work is passed through as part of
// the step calling it, never a step of its own, and a unit whose closure
// (its calls and hand-overs, transitively) enters no part the program exists
// for (Group.Core) is no candidate, when the program has such a part.
type flowGraph struct {
	index    *groupindex.Index
	subjects map[string]*groupindex.Subject
	unit     func(string) string
	out      map[string][]flowEdge
	// core holds the declarations whose closure enters a core part; nil
	// when no part of the program is core.
	core map[string]bool
	// open holds the declarations making a call the index leaves open with
	// no possible target: work no edge says (conduit).
	open map[string]bool
}

// flowEdge is one way a declaration reaches another: via says it as the
// code does ("called", "one of 3", "handed to quil.core.sketch.setup"); an
// alternative of a dispatch site, or a possible end of an open call, is
// said with the declaration holding the site (site) and, to the
// categorizer alone, the site's file and line (at: "server.c:88"). The
// reader's column prints no line. through is the helper the step's work
// passes on its way there (candidates), and said how the categorizer reads
// it ("docall").
type flowEdge struct {
	to      string
	via     string
	site    string
	at      string
	through []string
	said    []string
	// basis is, for a call whose target is a method of a repository type
	// implementing the called interface, no observed flow giving the value,
	// programindex.BasisImplements (GO): the page says so, never a traced
	// call.
	basis string
}

// asked is how a candidate is reached as the categorizer reads it: "one of
// 94 at redis.c:2054", "called through docall".
func (edge flowEdge) asked() string {
	asked := edge.via
	if len(edge.said) > 0 {
		asked += " through " + strings.Join(edge.said, ", ")
	}
	if edge.at != "" {
		asked += " at " + edge.at
	}
	return asked
}

// flowCandidate is one unit a step's work enters: the unit, the members of
// it entered, how the first of them is reached, and the member of the step
// the path passes through to reach it (a class's run, not the class).
type flowCandidate struct {
	unit    string
	members []string
	reach   flowEdge
	through string
	// roots are, by member entered, the member of the step whose work
	// entered it first; by are every member of the step reaching it.
	roots map[string]string
	by    []string
	// ways are, by member entered, how the step's work first reaches it.
	ways map[string]flowEdge
}

func newFlowGraph(index *groupindex.Index, registrations []facts.Fact) *flowGraph {
	graph := &flowGraph{index: index, subjects: make(map[string]*groupindex.Subject, len(index.Subjects)),
		unit: groupindex.Units(index), out: map[string][]flowEdge{}}
	for position := range index.Subjects {
		graph.subjects[index.Subjects[position].ID] = &index.Subjects[position]
	}
	// The call a passed callable is an argument of names what it is handed
	// to: the relation's target, by the argument's relation, else the
	// nearest call the same declaration opens before the argument
	// (aeCreateFileEvent, not the oom its error branch calls after).
	type opened struct {
		line, column int
		name         string
	}
	calledBy := map[string]string{}
	calledAt := map[string][]opened{}
	alternatives := map[string]int{}
	for _, edge := range index.StructuralEdges {
		if edge.Role != groupindex.EdgeRelationTarget || edge.RelationID == "" {
			continue
		}
		switch edge.RelationKind {
		case programindex.RelationCalls, programindex.RelationExecutes, programindex.RelationInvokesExternal:
			if _, named := calledBy[edge.RelationID]; !named {
				calledBy[edge.RelationID] = graph.name(edge.ToSubjectID)
			}
			if edge.Location != nil {
				calledAt[edge.FromSubjectID] = append(calledAt[edge.FromSubjectID], opened{line: edge.Location.Line, column: edge.Location.Column, name: graph.name(edge.ToSubjectID)})
			}
			if edge.Resolution == programindex.ResolutionAlternatives {
				alternatives[edge.RelationID]++
			}
		}
	}
	seen := map[[2]string]bool{}
	add := func(from string, edge flowEdge) {
		if !graph.member(from) || !graph.member(edge.to) || from == edge.to {
			return
		}
		if seen[[2]string{from, edge.to}] {
			// A traced call of a callee the declaration also reaches as an
			// interface's implementation is how it is reached.
			if edge.basis == "" {
				for position, known := range graph.out[from] {
					if known.to == edge.to && known.basis != "" {
						graph.out[from][position] = edge
					}
				}
			}
			return
		}
		seen[[2]string{from, edge.to}] = true
		graph.out[from] = append(graph.out[from], edge)
	}
	// A registration fact says what the callable is registered as
	// (quil.core.sketch.mouse-pressed): it names the hand-over first.
	for _, fact := range registrations {
		if fact.Kind != facts.KindRegistration || fact.TargetID != index.Target.ID || fact.OwnerID == "" || fact.ObjectID == "" {
			continue
		}
		api := fact.Text
		if api == "" {
			api = fact.Key
		}
		add(fact.OwnerID, flowEdge{to: fact.ObjectID, via: "handed to " + api})
	}
	for _, edge := range index.StructuralEdges {
		if edge.Role != groupindex.EdgeRelationTarget || edge.Resolution == programindex.ResolutionUnresolved {
			continue
		}
		switch edge.RelationKind {
		case programindex.RelationCalls, programindex.RelationExecutes:
			reach := flowEdge{to: edge.ToSubjectID, via: "called"}
			if edge.Resolution == programindex.ResolutionAlternatives {
				reach = flowEdge{to: edge.ToSubjectID, via: fmt.Sprintf("one of %d", alternatives[edge.RelationID]), site: edge.FromSubjectID, at: siteOf(edge.Location)}
			}
			reach.basis = edge.Basis
			add(edge.FromSubjectID, reach)
		case programindex.RelationPassesCallback:
			via := "handed over"
			if relation, _, found := strings.Cut(edge.SourceArgumentID, "p"); found && calledBy[relation] != "" {
				via = "handed to " + calledBy[relation]
			} else if edge.Location != nil {
				nearest := opened{line: -1}
				for _, call := range calledAt[edge.FromSubjectID] {
					before := call.line < edge.Location.Line || call.line == edge.Location.Line && call.column <= edge.Location.Column
					later := call.line > nearest.line || call.line == nearest.line && call.column > nearest.column
					if before && later {
						nearest = call
					}
				}
				if nearest.name != "" {
					via = "handed to " + nearest.name
				}
			}
			add(edge.FromSubjectID, flowEdge{to: edge.ToSubjectID, via: via})
		}
	}
	// A call through a function value the index leaves open reaches, as one
	// of its possible targets, each function its stores put there (the
	// map's possible arrows): redis's aeProcessEvents calls fe->rfileProc,
	// which acceptHandler, readQueryFromClient and sendReplyToClient fill.
	graph.open = map[string]bool{}
	for _, call := range index.Unresolved {
		if len(call.Possible) == 0 {
			graph.open[call.FromSubjectID] = true
		}
		for _, end := range call.Possible {
			add(call.FromSubjectID, flowEdge{to: end, via: fmt.Sprintf("one of %d", len(call.Possible)), site: call.FromSubjectID, at: siteOf(call.Location)})
		}
	}
	graph.core = graph.closuresEnteringCore()
	return graph
}

// member says a declaration is one a flow may step through: it runs
// (a function, a method, a lambda or a module body) and is not test code.
func (graph *flowGraph) member(id string) bool {
	subject := graph.subjects[id]
	if subject == nil || subject.Object == nil {
		return false
	}
	switch subject.Object.Kind {
	case programindex.ObjectFunction, programindex.ObjectMethod, programindex.ObjectLambda, programindex.ObjectModule:
	default:
		return false
	}
	return subject.Object.Location == nil || !slices.Contains(graph.index.Target.TestSources, subject.Object.Location.Path)
}

// closuresEnteringCore lists every declaration whose calls and hand-overs,
// transitively, reach a member of a core part, itself included; nil when the
// program has no core part, so the filter keeps every candidate.
func (graph *flowGraph) closuresEnteringCore() map[string]bool {
	into := map[string][]string{}
	for from, edges := range graph.out {
		for _, edge := range edges {
			into[edge.to] = append(into[edge.to], from)
		}
	}
	var queue []string
	result := map[string]bool{}
	for _, group := range graph.index.Groups {
		if !group.Core {
			continue
		}
		for _, id := range group.MemberSubjectIDs {
			if !result[id] {
				result[id] = true
				queue = append(queue, id)
			}
		}
	}
	if len(queue) == 0 {
		return nil
	}
	for next := 0; next < len(queue); next++ {
		for _, from := range into[queue[next]] {
			if !result[from] {
				result[from] = true
				queue = append(queue, from)
			}
		}
	}
	return result
}

// candidates are the units a step's work enters: what its entered members,
// and the unit's own members they call or hand over in turn, reach in other
// units, each once with the members of it entered and how the first is
// reached, in the order the code's relations list them. A public member of
// the step's own class that its work calls is a candidate of its own, a step
// of the class's work (FreqtradeBot.process calls enter_positions and
// exit_positions, which had been folded into it with 46 others); a private
// helper stays folded into the step. A declaration the helper question
// decided serves others' work is never a step and never a dead end: a
// helper the step calls or hands over serves the step, so what it calls or
// hands over, other than further helpers, is the step's work, said through
// it ("called through docall"); a helper's own helpers serve that helper
// (raising an error, growing a stack, allocating) and are not looked
// through, unless it only passes the call on to one of them (conduit), the
// walk then passing through to the first helper doing more ("./lua
// script.lua" runs in the VM: lua_pcallk calls luaV_execute through
// luaD_call, ccall). Lua's handle_script runs the script through docall, a helper
// (0.77-0.81 over 8 draws), and lua_load parses through
// luaD_protectedparser: dropped with all they reach, they had left
// handle_script only luaL_loadfilex and lua_load only the collector's step,
// so the path ran into the collector; looked through at every depth, every
// step reached most of the runtime by its error and allocation helpers.
// One whose closure enters no core part is none.
func (graph *flowGraph) candidates(unit string, entered []string) []flowCandidate {
	var result []flowCandidate
	at := map[string]int{}
	// Each member entered walks its own class's members on its own, so a
	// candidate names every member of the step whose work reaches it: the
	// bot's process and its constructor do different work.
	for _, start := range entered {
		seen := map[string]bool{start: true}
		queue := []string{start}
		// passed are, by a helper the step's work passes through, that
		// helper; the step's own members pass none.
		passed := map[string]flowEdge{}
		for next := 0; next < len(queue); next++ {
			from := queue[next]
			for _, edge := range graph.out[from] {
				target := graph.unit(edge.to)
				helper := graph.decidedHelper(edge.to) || graph.decidedHelper(target)
				if target == unit && start != unit && graph.public(edge.to) && !helper {
					target = edge.to
				}
				if target == unit {
					if !seen[edge.to] {
						seen[edge.to] = true
						queue = append(queue, edge.to)
					}
					continue
				}
				edge.through, edge.said = passed[from].through, passed[from].said
				if helper {
					// A helper's own helpers serve it, not the step, unless it
					// only passes the call on (conduit).
					if (len(edge.through) == 0 || graph.conduit(from)) && !seen[edge.to] {
						seen[edge.to] = true
						passed[edge.to] = flowEdge{through: append(slices.Clone(edge.through), edge.to), said: append(slices.Clone(edge.said), graph.qualified(edge.to))}
						queue = append(queue, edge.to)
					}
					continue
				}
				position, known := at[target]
				if !known {
					position = len(result)
					at[target] = position
					result = append(result, flowCandidate{unit: target, reach: edge, through: start, roots: map[string]string{}, ways: map[string]flowEdge{}})
				}
				candidate := &result[position]
				if _, known := candidate.roots[edge.to]; !known {
					candidate.roots[edge.to] = start
					candidate.ways[edge.to] = edge
				}
				if !slices.Contains(candidate.members, edge.to) {
					candidate.members = append(candidate.members, edge.to)
				}
				if !slices.Contains(candidate.by, start) {
					candidate.by = append(candidate.by, start)
				}
			}
		}
	}
	return slices.DeleteFunc(result, func(candidate flowCandidate) bool {
		return graph.core != nil && !slices.ContainsFunc(candidate.members, func(id string) bool { return graph.core[id] })
	})
}

// public says a declaration is its class's public member: the class's own
// operation, not a helper of it.
func (graph *flowGraph) public(id string) bool {
	subject := graph.subjects[id]
	return subject != nil && subject.Object != nil && subject.Object.Visibility == programindex.VisibilityPublic
}

// coreParts are the core parts a candidate's closure enters, by title, in
// the order the program lists its parts: code's word for where a choice
// leads.
func (graph *flowGraph) coreParts(candidate flowCandidate) []string {
	reached := map[string]bool{}
	queue := append([]string(nil), candidate.members...)
	for _, member := range queue {
		reached[member] = true
	}
	for next := 0; next < len(queue); next++ {
		for _, edge := range graph.out[queue[next]] {
			if !reached[edge.to] {
				reached[edge.to] = true
				queue = append(queue, edge.to)
			}
		}
	}
	var parts []string
	for _, group := range graph.index.Groups {
		if group.Core && slices.ContainsFunc(group.MemberSubjectIDs, func(id string) bool { return reached[id] }) {
			parts = append(parts, group.Title)
		}
	}
	return parts
}

// conduit says a helper only passes the call on: the walk follows exactly
// one edge from it, to a further helper, and it makes no call the index
// leaves open. That helper serves whoever the conduit serves, so the walk
// passes through it too (Lua's luaD_call, whose one call is ccall; f_call,
// handed to luaD_pcall, whose one call is luaD_callnoyield, whose one is
// ccall). A helper doing more than passing on (ccall: the VM, the stack's
// checks, the C stack's error) does its job with its own helpers, which
// serve it, not the step.
func (graph *flowGraph) conduit(id string) bool {
	edges := graph.out[id]
	if len(edges) != 1 || graph.open[id] || !graph.decidedHelper(id) && !graph.decidedHelper(graph.unit(id)) {
		return false
	}
	to := edges[0].to
	return graph.decidedHelper(to) || graph.decidedHelper(graph.unit(to))
}

func (graph *flowGraph) decidedHelper(id string) bool {
	subject := graph.subjects[id]
	return subject != nil && subject.Interpretation != nil && subject.Interpretation.Helper
}

// name is a declaration's name as the program indexes it.
func (graph *flowGraph) name(id string) string {
	if subject := graph.subjects[id]; subject != nil && subject.Object != nil {
		return subject.Object.Name
	}
	return id
}

func siteOf(location *programindex.Location) string {
	if location == nil {
		return "a dispatch site"
	}
	return fmt.Sprintf("%s:%d", location.Path, location.Line)
}
