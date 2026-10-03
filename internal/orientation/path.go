package orientation

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
)

// flowStage names the categorizer's choices of a Main flow.
const flowStage = "orientation_flow"

// sectionFlowFork journals a split the categorizer did not decide: the path
// ends there, a named fork.
const sectionFlowFork = "flow_fork"

// flowWalk is one walked Main flow: its steps, the splits it asked about
// and the lead of each decided choice (for measurement; the artifact keeps
// the steps).
type flowWalk struct {
	flow     MainFlow
	rejected []RejectedRow
	asked    []flowAsk
	// met are the candidates the walk met, by the index a candidate step
	// carries as its Edge.
	met []flowCandidate
	// failing are, by a step's Edge (-1 the entry), the candidates its work
	// reaches only on failing paths, never a way on; kept the units of the
	// others, before the walk leaves out those already on the path.
	failing map[int][]flowCandidate
	kept    map[int][]string
}

// flowAsk is one split the categorizer was asked about: the one it
// decided, or, under the margin, the ways followed.
type flowAsk struct {
	step       string
	candidates int
	chosen     string
	lead       float64
	decided    bool
	followed   []string
}

// walkFlow is a target's Main flow, walked by code from the target's entry
// over flowGraph (design skeptic, 2026-09-30: a model writing the whole flow
// had given othello 23 to 107 steps on one request and freqtrade's bot loop
// in 1 of 7 answers). A step with no candidate is the flow's result, one is
// followed with no request, and of several the categorizer chooses the one
// the path to the program's work passes through: one closed question per
// split. A lead under table.ClassifierMargin, or no answer, ends the path
// there as a named fork, its candidates kept and the split journaled. No
// step is written by a model: each is its declaration, with the atlas line
// already accepted for it and how the step before reaches it.
func walkFlow(ctx context.Context, executor llm.Executor, categorizer llm.Categorizer, input Input, targetID string) (flowWalk, error) {
	return walkFlowFrom(ctx, executor, categorizer, input, targetID, "")
}

// WalkFlow walks a target's Main flow as Run does, from entry, a subject of
// the target, or from the target's entry when entry is empty: a program's
// flow read from a function of it (a fixture's tool_cli.main in a library).
func WalkFlow(ctx context.Context, executor llm.Executor, categorizer llm.Categorizer, input Input, targetID, entry string) (MainFlow, []RejectedRow, error) {
	walk, err := walkFlowFrom(ctx, executor, categorizer, input, targetID, entry)
	return walk.flow, walk.rejected, err
}

func walkFlowFrom(ctx context.Context, executor llm.Executor, categorizer llm.Categorizer, input Input, targetID, entry string) (flowWalk, error) {
	var index *groupindex.Index
	for position := range input.Groups {
		if input.Groups[position].Target.ID == targetID {
			index = &input.Groups[position]
		}
	}
	if index == nil {
		return flowWalk{}, fmt.Errorf("orientation: flow target %q has no groups index", targetID)
	}
	graph := newFlowGraph(index, input.Facts.OfKind(facts.KindRegistration))
	start := entry
	if start == "" {
		start = flowEntry(index, graph)
	}
	if start == "" || !graph.member(start) {
		return flowWalk{}, nil
	}
	walk := flowWalk{failing: map[int][]flowCandidate{}, kept: map[int][]string{}}
	var failure error
	meet := func(candidate flowCandidate) groupindex.SpineStep {
		walk.met = append(walk.met, candidate)
		return groupindex.SpineStep{SubjectID: candidate.unit, Members: candidate.members, Edge: len(walk.met) - 1}
	}
	next := func(step groupindex.SpineStep) []groupindex.SpineStep {
		var steps []groupindex.SpineStep
		unit := graph.unit(step.Members[0])
		// A class entered through several of its members goes on through
		// one of them: the bot's process loop, not its constructor's
		// schedule (Worker's run, exit and __init__ are one step, whose
		// work the member chosen does).
		if class := graph.subjects[unit]; len(step.Members) > 1 && class != nil && class.Object != nil && class.Object.Kind == programindex.ObjectType {
			for _, member := range step.Members {
				if member == unit || len(graph.candidates(unit, []string{member})) == 0 {
					continue
				}
				steps = append(steps, meet(flowCandidate{unit: member, members: []string{member}, reach: flowEdge{to: member, via: "its member " + graph.name(member)}}))
				walk.kept[step.Edge] = append(walk.kept[step.Edge], member)
			}
			return steps
		}
		// A unit the step's work reaches only on failing paths (an error, a
		// call that never returns) is no way on: it stays beside the step,
		// said so, never asked or followed (control review, 2026-10-03:
		// Lua's forprep reaches the collector only through luaG_runerror,
		// when the step is zero).
		for _, candidate := range graph.candidates(unit, step.Members) {
			if candidate.guard.Fails() {
				walk.failing[step.Edge] = append(walk.failing[step.Edge], candidate)
				continue
			}
			steps = append(steps, meet(candidate))
			walk.kept[step.Edge] = append(walk.kept[step.Edge], candidate.unit)
		}
		return steps
	}
	pick := func(step groupindex.SpineStep, candidates []groupindex.SpineStep) []int {
		if failure != nil || categorizer == nil {
			return nil
		}
		followed, ask, rejected, err := chooseNext(ctx, executor, categorizer, graph, index, step, candidates, walk.met)
		if err != nil {
			failure = err
			return nil
		}
		walk.asked = append(walk.asked, ask)
		walk.rejected = append(walk.rejected, rejected...)
		return followed
	}
	path := groupindex.WalkPaths(groupindex.SpineStep{SubjectID: graph.unit(start), Members: []string{start}, Edge: -1}, next, pick)
	if failure != nil {
		return flowWalk{}, failure
	}
	walk.flow.Steps = walk.flowSteps(path, targetID, graph)
	readRegistrations(walk.flow.Steps, nil, graph, input.Facts.OfKind(facts.KindRegistration))
	walk.flow.Title = flowTitle(walk.flow.Steps, graph)
	return walk, nil
}

// flowSteps are a walked path's steps: each its declaration, its own line
// and how the step before reaches it, and, where the categorizer decided a
// split, the candidates the path passed; the last, where the path parts,
// holds each way followed as a path of its own and the candidates none
// follows.
func (walk *flowWalk) flowSteps(path groupindex.SpinePath, targetID string, graph *flowGraph) []FlowStep {
	var rows []FlowStep
	said := func(met flowCandidate, subject string) FlowBranch {
		reach := met.reach
		return FlowBranch{SubjectID: subject, Via: reach.via, Site: reach.site, Through: slices.Clone(reach.through), Basis: reach.basis,
			Guard: cloneGuard(met.guard), Loop: cloneLocation(reach.loop)}
	}
	branch := func(candidate groupindex.SpineStep) FlowBranch {
		return said(walk.met[candidate.Edge], stepSubject(candidate))
	}
	for position, step := range path.Steps {
		row := FlowStep{TargetID: targetID, SubjectID: stepSubject(step), Explanation: graph.line(stepSubject(step))}
		if step.Edge >= 0 {
			met := walk.met[step.Edge]
			reach := met.reach
			row.Via, row.Site, row.Through, row.Basis = reach.via, reach.site, slices.Clone(reach.through), reach.basis
			row.Guard, row.Loop = cloneGuard(met.guard), cloneLocation(reach.loop)
		}
		for _, candidate := range path.Passed[position] {
			row.Passed = append(row.Passed, branch(candidate))
		}
		for _, failing := range walk.failing[step.Edge] {
			subject := failing.unit
			if len(failing.members) > 0 && graph.subjects[failing.unit] != nil && graph.subjects[failing.unit].Object != nil && graph.subjects[failing.unit].Object.Kind == programindex.ObjectType {
				subject = failing.members[0]
			}
			row.Passed = append(row.Passed, said(failing, subject))
		}
		// A class step going on through one of its members is that
		// member's step, reached as the class was; a way that starts at
		// one of the class's members is reached as its member. The class's
		// other members the path passed are the step before's calls.
		if strings.HasPrefix(row.Via, "its member ") {
			if len(rows) > 0 {
				previous := rows[len(rows)-1]
				row.Via, row.Site, row.Through, row.Basis = previous.Via, previous.Site, previous.Through, previous.Basis
				rows = rows[:len(rows)-1]
				if len(rows) > 0 {
					rows[len(rows)-1].Passed = append(rows[len(rows)-1].Passed, previous.Passed...)
				}
			} else {
				row.Via = "its member"
			}
		}
		if position == len(path.Steps)-1 {
			for _, candidate := range path.Rest {
				row.Branches = append(row.Branches, branch(candidate))
			}
			for _, way := range path.Paths {
				row.Paths = append(row.Paths, FlowPath{Steps: walk.flowSteps(way, targetID, graph)})
			}
			for _, joined := range path.Joins {
				row.Joins = append(row.Joins, branch(joined))
			}
			row.Stop = walk.stop(path, step)
			// A route ending while its step calls through a value the index
			// leaves open says so: the code may go on where no edge says
			// (Lua's luaD_hook calls (*hook)(L, &ar)).
			if row.Stop == StopLeaf || row.Stop == StopFailureOnly || row.Stop == StopRevisits {
				for _, member := range step.Members {
					if at := graph.openAt[member]; at != nil {
						row.OpenAt = cloneLocation(at)
						break
					}
				}
			}
		}
		rows = append(rows, row)
	}
	return rows
}

// stop is why a walked path ends at its last step (FlowStep.Stop): it parts
// into ways, any it joins among them; the categorizer left its split
// undecided; it goes on only into where other ways of the splits around it
// start, which it goes on as (FlowStep.Joins); its work reaches no further
// unit; it reaches further units only on failing paths; every unit it
// reaches is already on the path.
func (walk *flowWalk) stop(path groupindex.SpinePath, last groupindex.SpineStep) string {
	switch {
	case len(path.Paths) > 0:
		return StopTorn
	case len(path.Rest) > 0:
		return StopUnanswered
	case len(path.Joins) > 0:
		return StopJoins
	case len(walk.kept[last.Edge]) == 0 && len(walk.failing[last.Edge]) > 0:
		return StopFailureOnly
	case len(walk.kept[last.Edge]) == 0:
		return StopLeaf
	}
	return StopRevisits
}

// flowTitle is "From <first> to <last>", naming each end of a flow that
// parts ("From main to DB.Pos or s3.ReplicaClient.LTXFiles"), or, past
// three ends, the step where it parts and how many ways go on.
func flowTitle(steps []FlowStep, graph *flowGraph) string {
	var ends []string
	var collect func([]FlowStep)
	collect = func(steps []FlowStep) {
		last := steps[len(steps)-1]
		// A way that goes on as another way ends nowhere of its own.
		if last.Stop == StopJoins {
			return
		}
		if len(last.Paths) == 0 {
			if name := graph.qualified(last.SubjectID); !slices.Contains(ends, name) {
				ends = append(ends, name)
			}
			return
		}
		for _, path := range last.Paths {
			collect(path.Steps)
		}
	}
	collect(steps)
	first := graph.qualified(steps[0].SubjectID)
	switch {
	case len(ends) == 0:
		return fmt.Sprintf("From %s to %s", first, graph.qualified(steps[len(steps)-1].SubjectID))
	case len(ends) == 1:
		return fmt.Sprintf("From %s to %s", first, ends[0])
	case len(ends) <= 3:
		return fmt.Sprintf("From %s to %s or %s", first, strings.Join(ends[:len(ends)-1], ", "), ends[len(ends)-1])
	}
	return fmt.Sprintf("From %s to %s, then %d ways", first, graph.qualified(steps[len(steps)-1].SubjectID), len(ends))
}

// flowEntry is where a target's Main flow starts: its first seed that runs
// and is no test code, a callable before a module body.
func flowEntry(index *groupindex.Index, graph *flowGraph) string {
	entry := ""
	for _, seed := range index.Target.Seeds {
		if !graph.member(seed.ObjectID) {
			continue
		}
		if seed.Kind == "callable" {
			return seed.ObjectID
		}
		if entry == "" {
			entry = seed.ObjectID
		}
	}
	return entry
}

// stepSubject is the declaration a step names: the one member of its unit
// entered (FreqtradeBot.process, not FreqtradeBot), else the unit.
func stepSubject(step groupindex.SpineStep) string {
	if len(step.Members) == 1 {
		return step.Members[0]
	}
	return step.SubjectID
}

// line is the atlas line accepted for a declaration itself: a member step
// does not read as its class (FreqtradeBot's process, enter_positions and
// exit_positions had each read "The main class of the bot").
func (graph *flowGraph) line(id string) string {
	if subject := graph.subjects[id]; subject != nil && subject.Interpretation != nil {
		return strings.TrimSpace(subject.Interpretation.Line)
	}
	return ""
}

// class says a declaration is a type: a unit folding its members.
func (graph *flowGraph) class(id string) bool {
	subject := graph.subjects[id]
	return subject != nil && subject.Object != nil && subject.Object.Kind == programindex.ObjectType
}

// typeLine is, for a member of another type with no line of its own, that
// type and its line, said as the type's, never as the member's role
// (litestream's RestoreCommand.Run reads with "type RestoreCommand: a
// command to restore a database from a backup"; etcd's EtcdServer.Stop had
// read as "the main etcd server"). Members of the step's own type take
// none, since it would tell them nothing apart.
func (graph *flowGraph) typeLine(id, stepUnit string) (string, string) {
	if graph.line(id) != "" {
		return "", ""
	}
	if unit := graph.unit(id); unit != id && unit != stepUnit && graph.class(unit) {
		return graph.name(unit), graph.line(unit)
	}
	return "", ""
}

// enteredMembers are, for an option that is a type entered through its
// members, those members as the path enters them: each with its type
// (Etcd.Close), how the step's work reaches it ("handed to
// RegisterInterruptHandler") and its own line when it has one.
func (graph *flowGraph) enteredMembers(candidate groupindex.SpineStep, met flowCandidate) []string {
	if len(candidate.Members) == 0 || !graph.class(candidate.SubjectID) || stepSubject(candidate) != candidate.SubjectID {
		return nil
	}
	var entered []string
	for _, member := range candidate.Members {
		said := graph.qualified(member)
		var about []string
		if way, known := met.ways[member]; known {
			about = append(about, way.asked())
		}
		if line := graph.line(member); line != "" {
			about = append(about, line)
		}
		if len(about) > 0 {
			said += " (" + strings.Join(about, "; ") + ")"
		}
		entered = append(entered, said)
	}
	return entered
}

// part is the title of the part holding a declaration, or "".
func (graph *flowGraph) part(id string) string {
	for _, group := range graph.index.Groups {
		for _, member := range group.MemberSubjectIDs {
			if member == id || member == graph.unit(id) {
				return group.Title
			}
		}
	}
	return ""
}

// handles are the inputs a candidate's members handle, each as its kind and
// name ("interaction mouse-pressed"), each once.
func (graph *flowGraph) handles(candidate flowCandidate) []string {
	var result []string
	for _, operation := range graph.index.Operations {
		if operation.SubjectID == "" || operation.HandlerUnknown || !slices.Contains(candidate.members, operation.SubjectID) && operation.SubjectID != candidate.unit {
			continue
		}
		said := strings.TrimSpace(operation.Kind + " " + operation.Name)
		if said != "" && !slices.Contains(result, said) {
			result = append(result, said)
		}
	}
	if len(result) > 6 {
		result = append(result[:6], fmt.Sprintf("and %d more", len(result)-6))
	}
	return result
}

// qualified is a declaration's name with its class's (FreqtradeBot.process).
func (graph *flowGraph) qualified(id string) string {
	if unit := graph.unit(id); unit != id {
		return graph.name(unit) + "." + graph.name(id)
	}
	return graph.name(id)
}

func (graph *flowGraph) signature(id string) string {
	if subject := graph.subjects[id]; subject != nil && subject.Object != nil {
		return subject.Object.Signature
	}
	return ""
}

// toldApart are a split's candidates by name, those sharing one told apart
// by their type, folder, file or part (groupindex.TellApart): litestream's
// Sync had offered eight options titled ReplicaClient, which the
// categorizer could only read by ref.
func (graph *flowGraph) toldApart(candidates []groupindex.SpineStep) []string {
	names := make([]string, 0, len(candidates))
	spellings := make([][]string, 0, len(candidates))
	for _, candidate := range candidates {
		id := stepSubject(candidate)
		name, file := graph.name(id), ""
		if subject := graph.subjects[id]; subject != nil && subject.Object != nil && subject.Object.Location != nil {
			file = subject.Object.Location.Path
		}
		typed := ""
		if qualified := graph.qualified(id); qualified != name {
			typed = qualified
		}
		names = append(names, name)
		spellings = append(spellings, append([]string{typed}, groupindex.Where(name, file, graph.part(id))...))
	}
	return groupindex.TellApart(names, spellings)
}

// chooseNext asks the categorizer which candidate the path continues
// through, one closed question (table.ClassifierCall): the task names the
// program and its core parts, the item is the step, and each option is a
// candidate with its name, signature, part, atlas line and how it is
// reached. No docstring enters.
func chooseNext(ctx context.Context, executor llm.Executor, categorizer llm.Categorizer, graph *flowGraph, index *groupindex.Index,
	step groupindex.SpineStep, candidates []groupindex.SpineStep, met []flowCandidate) ([]int, flowAsk, []RejectedRow, error) {
	var core []string
	for _, group := range index.Groups {
		if group.Core {
			core = append(core, group.Title)
		}
	}
	domain := "none named"
	if len(core) > 0 {
		domain = strings.Join(core, "; ")
	}
	system := fmt.Sprintf("We trace the one path a newcomer follows from where %s starts to the work it exists for, once. Its domain parts: %s.", index.Target.Name, domain)
	subject := stepSubject(step)
	options := make([]map[string]any, 0, len(candidates))
	names := graph.toldApart(candidates)
	// The inputs a candidate handles, as the reading decided them (a user's
	// mouse press is an interaction, a sketch's setup an extension), are a
	// criterion of every option or of none: said of some, it reads as "no"
	// on the rest, while the catalogue holds only inputs written as literals
	// (lua's pmain had offered "handles: command W, command e l" on runargs
	// alone, the script being a positional argument, and the walk took the
	// -l option 5 of 5; handle_script wins 5 of 5 without it).
	handled := make([][]string, len(candidates))
	every := len(candidates) > 0
	for position, candidate := range candidates {
		handled[position] = graph.handles(met[candidate.Edge])
		every = every && len(handled[position]) > 0
	}
	for position, candidate := range candidates {
		id := stepSubject(candidate)
		ref := fmt.Sprintf("c%d", position+1)
		terms := []string{names[position]}
		// An option is what the path enters: a type entered through some
		// of its members is said by those members, never by the type's own
		// line or signature, and a type's line beside a member of it is
		// said as the type's (etcd's startEtcd had offered "Etcd ... serves
		// peers, clients and metrics", entered only through Close, handed
		// to the interrupt handler, and Err: the walk took the shutdown
		// path 5 of 5).
		if entered := graph.enteredMembers(candidate, met[candidate.Edge]); len(entered) > 0 {
			terms = append(terms, "enters "+strings.Join(entered, ", "))
		} else if signature := graph.signature(id); signature != "" {
			terms = append(terms, "signature "+signature)
		}
		if part := graph.part(id); part != "" {
			terms = append(terms, "in part "+part)
		}
		if line := graph.line(id); line != "" && !graph.class(id) {
			terms = append(terms, "role: "+line)
		} else if typ, line := graph.typeLine(id, graph.unit(subject)); line != "" {
			terms = append(terms, "type "+typ+": "+line)
		}
		reached := met[candidate.Edge].reach.asked()
		if len(step.Members) > 1 {
			var by []string
			for _, member := range met[candidate.Edge].by {
				by = append(by, graph.qualified(member))
			}
			reached += " by " + strings.Join(by, ", ")
		}
		terms = append(terms, "reached: "+reached)
		if every {
			terms = append(terms, "handles: "+strings.Join(handled[position], ", "))
		}

		options = append(options, map[string]any{"ref": ref, "title": names[position], "criteria": strings.Join(terms, "; ")})
	}
	item := []table.Field{{Name: "step", Value: graph.name(subject)}}
	if signature := graph.signature(subject); signature != "" {
		item = append(item, table.Field{Name: "signature", Value: signature})
	}
	if part := graph.part(subject); part != "" {
		item = append(item, table.Field{Name: "part", Value: part})
	}
	item = append(item, table.Field{Name: "candidates", Value: options})
	def := table.Definition{Stage: flowStage, Contract: "repomap.orientation.flow.v1", System: system, Classifier: true,
		Columns: []table.Column{{Name: "next", Kind: table.Choice, OptionsFrom: "candidates", CriteriaFrom: "criteria", Item: "step",
			Ask: "Which of `candidates` does the path from `step` continue through to do the program's core work once: one run of a command, one request or message a server handles, or one user action carried to its visible result?"}}}
	window := table.Window{Stage: flowStage, Rows: []table.Row{{ID: index.Target.ID + "." + subject, Fields: item}}}
	call, err := table.ClassifierCall(categorizer, def, window)
	if err != nil {
		return nil, flowAsk{}, nil, err
	}
	var verdicts map[string]llm.Verdict
	decode := call.DecodeValidate
	call.DecodeValidate = func(raw []byte) (table.Result, error) {
		verdicts, _ = categorizer.Verdicts(raw)
		return decode(raw)
	}
	ask := flowAsk{step: graph.name(subject), candidates: len(candidates)}
	key := index.Target.ID + "." + subject + "|next"
	outcome, err := llm.ExecuteJSON(ctx, executor, categorizer, call)
	if err == nil && len(outcome.Value.Answers) == 1 && outcome.Value.Answers[0] != nil {
		chosen := outcome.Value.Answers[0]["next"]
		for position := range candidates {
			if fmt.Sprintf("c%d", position+1) == chosen {
				ask.chosen, ask.decided = names[position], true
				ask.lead = leadOf(verdicts, key)
				return []int{position}, ask, nil, nil
			}
		}
	}
	if ctx.Err() != nil {
		return nil, flowAsk{}, nil, ctx.Err()
	}
	// Under the margin the path parts: each candidate the categorizer
	// holds within the margin of its leader is a way of its own, the margin
	// bounding how many (owner, 2026-09-30); with no verdict the path ends
	// here, a named fork.
	reason := "the categorizer did not answer"
	if err != nil {
		reason = err.Error()
	} else if len(outcome.Value.Rejections) > 0 {
		reason = outcome.Value.Rejections[0].Reason
	}
	ask.lead = leadOf(verdicts, key)
	followed := withinMargin(verdicts[key], names)
	for _, position := range followed {
		ask.followed = append(ask.followed, names[position])
	}
	raw, _ := json.Marshal(struct {
		Step       string   `json:"step"`
		Candidates []string `json:"candidates"`
		Lead       float64  `json:"lead"`
		Followed   []string `json:"followed,omitempty"`
	}{graph.name(subject), names, ask.lead, ask.followed})
	return followed, ask, []RejectedRow{{Stage: StageName, Section: sectionFlowFork, Raw: raw, Reason: reason}}, nil
}

// withinMargin are the candidates a verdict holds within the classifier
// margin of its leader, leader first, by the name or ref each was offered
// under; none when there is no verdict or only its leader.
func withinMargin(verdict llm.Verdict, names []string) []int {
	probability := func(position int) (float64, bool) {
		for _, label := range []string{names[position], fmt.Sprintf("c%d", position+1)} {
			if value, ok := verdict.Probabilities[label]; ok {
				return value, true
			}
		}
		return 0, false
	}
	top := 0.0
	for position := range names {
		if value, ok := probability(position); ok && value > top {
			top = value
		}
	}
	var followed []int
	for position := range names {
		if value, ok := probability(position); ok && top > 0 && value > top-table.ClassifierMargin {
			followed = append(followed, position)
		}
	}
	sort.SliceStable(followed, func(i, j int) bool {
		a, _ := probability(followed[i])
		b, _ := probability(followed[j])
		return a > b
	})
	if len(followed) < 2 {
		return nil
	}
	return followed
}

// leadOf is how far a verdict's choice leads the runner-up.
func leadOf(verdicts map[string]llm.Verdict, key string) float64 {
	verdict, ok := verdicts[key]
	if !ok {
		return 0
	}
	top, rival := verdict.Probabilities[verdict.Choice], 0.0
	for option, probability := range verdict.Probabilities {
		if option != verdict.Choice && probability > rival {
			rival = probability
		}
	}
	return top - rival
}
