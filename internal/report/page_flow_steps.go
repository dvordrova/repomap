package report

import (
	"cmp"
	"slices"

	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/orientation"
	"github.com/dvordrova/repomap/internal/programindex"
)

// pageStepName is a declaration a Main flow step names beside its own:
// read in its part (Part, Key) when one holds it, and a link into its code
// (Code, all of its lines on a static page; Open, a served page's source).
// On a run of calls, Possible and Handed say how the name before reaches
// it, in words: a callable it may call through a value ("may call"), one it
// hands over ("hands over"); neither, an exact call ("→").
type pageStepName struct {
	Name     string
	Part     string
	Key      string
	Code     string
	Open     string
	Possible bool
	Handed   bool
	// Through are, for a split's candidate, the helpers the work passes
	// through on its way to it (orientation FlowBranch.Through).
	Through []pageStepName
	// Implemented is, for a split's candidate, that it is a method of a
	// repository type implementing the interface called, known by method
	// set (orientation FlowBranch.Basis).
	Implemented bool
	// Guard and Loop are, for a split's candidate, what the call reaching
	// it runs under (orientation FlowBranch.Guard, Loop): a failing path's
	// candidate is never a way on, and says so.
	Guard *pageGuard
	Loop  *pageAnchor
}

// pageStepRegistration is one place a step's callable is registered: the
// run of calls reaching the function that registers it, that function last,
// and the registering call's own line, which the words "registers it" link
// (owner, 2026-09-29: the step's own link had landed on that line, inside
// createClient, for readQueryFromClient).
type pageStepRegistration struct {
	By []pageStepName
	At *pageAnchor
}

// flowRegistrations are a Main flow step's saved registrations and runners
// (orientation FlowStep.Registered and RunBy, version 3), each run of calls
// read as a step's names are; the page derives none of them.
func (builder *pageBuilder) flowRegistrations(row *pageFlowStep, step orientation.FlowStep, section *pageSection) {
	chain := func(hops []orientation.FlowHop) []pageStepName {
		names := make([]pageStepName, len(hops))
		for position, hop := range hops {
			names[position] = builder.stepName(section, hop.SubjectID)
			names[position].Possible, names[position].Handed = hop.Possible, hop.Handed
		}
		return names
	}
	for _, registration := range step.Registered {
		said := pageStepRegistration{By: chain(registration.Chain)}
		if fact, known := builder.factsByID[registration.FactID]; known {
			said.At = builder.links.factAnchor(fact)
		}
		row.Registers = append(row.Registers, said)
	}
	for _, runner := range step.RunBy {
		row.RunBy = append(row.RunBy, chain(runner.Chain))
	}
}

// ownWorkReading reads a callable a program runs on its own from its saved
// facts: each registration of it by its registering function alone (no run
// of calls from the entries is chosen: litestream's Replica.monitor had read
// "main → Main.Run → ReplicateCommand.Run → Store.Close → … → Replica.Start
// registers it", the shortest of the program's exact calls, through its
// shutdown), and each function calling it through a value, after the run of
// exact calls from the program's entries reaching that function, from the
// last declaration the Main flow already shows on that run.
func (builder *pageBuilder) ownWorkReading(row *pageFlowStep, fact facts.Fact, section *pageSection, shown map[string]bool) bool {
	index := builder.graphIndex(section.programTargetID)
	if index == nil || builder.data.Facts == nil {
		return false
	}
	ref, known := builder.subject(section.programTargetID, fact.ObjectID)
	if !known || ref.subject.Object == nil || ref.subject.Object.Location == nil {
		return false
	}
	own := builder.stepName(section, fact.ObjectID)
	_, ownAnchor := builder.subjectDisplay(ref.subject)
	row.Label, row.Part, row.Key, row.Anchor = own.Name, own.Part, own.Key, ownAnchor
	var calls []int
	for position, edge := range index.StructuralEdges {
		if edge.Role == groupindex.EdgeRelationTarget && edge.RelationKind == programindex.RelationCalls && edge.Resolution == programindex.ResolutionExact {
			calls = append(calls, position)
		}
	}
	byChain := map[string]bool{}
	for _, other := range builder.data.Facts.OfKind(facts.KindRegistration) {
		if other.TargetID != fact.TargetID || other.ObjectID != fact.ObjectID || other.OwnerID == "" {
			continue
		}
		owner := builder.stepName(section, other.OwnerID)
		// Registrations by one function read once, linking the first
		// registering call: sizeWorkspace had read "canvas registers it"
		// four times, one function's four calls.
		if key := owner.Name + "\x00" + owner.Key; !byChain[key] {
			byChain[key] = true
			row.Registers = append(row.Registers, pageStepRegistration{By: []pageStepName{owner}, At: builder.links.factAnchor(other)})
		}
	}
	for _, runner := range builder.runnersOf(section.programTargetID, fact.ObjectID) {
		chain := []string{runner}
		if !shown[runner] {
			for _, entry := range index.Entries {
				if reached := chainOf(index, calls, entry.SubjectID, runner); reached != nil {
					chain = reached
					for at := len(reached) - 2; at > 0; at-- {
						if shown[reached[at]] {
							chain = reached[at:]
							break
						}
					}
					break
				}
			}
		}
		// A runner once read is named alone after.
		shown[runner] = true
		names := make([]pageStepName, len(chain))
		for position, id := range chain {
			names[position] = builder.stepName(section, id)
		}
		row.RunBy = append(row.RunBy, names)
	}
	return true
}

// runnersOf are the functions of a program that call a callable through a
// value, in relation order, each once: a call whose alternatives hold it, a
// call through a function value resolved to it alone (processTimeEvents'
// te->timeProc, which only serverCron is stored in), or an open call whose
// witnesses name it among the values it may call (C's stores into
// fe->rfileProc).
func (builder *pageBuilder) runnersOf(programTargetID, objectID string) []string {
	if builder.data == nil || builder.data.ProgramPortfolio == nil {
		return nil
	}
	var runners []string
	for _, entry := range builder.data.ProgramPortfolio.Entries {
		if entry.Target.ID != programTargetID {
			continue
		}
		for _, relation := range entry.Relations {
			if relation.Kind != programindex.RelationCalls || relation.Resolution == programindex.ResolutionExact && relation.Dispatch != programindex.DispatchFunctionValue ||
				slices.Contains(runners, relation.FromID) {
				continue
			}
			runs := slices.Contains(relation.ToIDs, objectID)
			for _, witness := range relation.Witnesses {
				runs = runs || relation.Resolution == programindex.ResolutionUnresolved && witness.ObjectID == objectID
			}
			if runs {
				runners = append(runners, relation.FromID)
			}
		}
	}
	return runners
}

// fieldTypes are where the repository types a field's declared type names
// are declared (ProgramIndex Object.Types), from the program's index.
func (builder *pageBuilder) fieldTypes(programTargetID, objectID string) []programindex.Location {
	if builder.data == nil || builder.data.ProgramPortfolio == nil {
		return nil
	}
	if builder.typesOf == nil {
		builder.typesOf = map[string]map[string][]programindex.Location{}
	}
	byObject, done := builder.typesOf[programTargetID]
	if !done {
		byObject = map[string][]programindex.Location{}
		for _, entry := range builder.data.ProgramPortfolio.Entries {
			if entry.Target.ID != programTargetID {
				continue
			}
			for _, object := range entry.Objects {
				if len(object.Types) > 0 {
					byObject[object.ID] = object.Types
				}
			}
		}
		builder.typesOf[programTargetID] = byObject
	}
	return byObject[objectID]
}

// pageOwnWork is one piece of work a program runs on its own, read as a
// Main flow step citing its registration is: the callable, where it is
// registered and what runs it. Input is its input's node on the map.
type pageOwnWork struct {
	pageFlowStep
	Input string
}

// ownWork is what a program runs without a request arriving (owner,
// 2026-09-29, benchmark v4: serverCron, a timer, was not findable from a
// Main flow of client commands): its inputs of the scheduled kind, then of
// the continuous kind (the ways-in order, outerKindRank), each in its saved
// order and each callable once, save one the Main flow already names. A
// registration of it reads as ownWorkReading reads it, a runner the flow
// already shows named alone ("serverCron — initServer registers it;
// aeProcessEvents → processTimeEvents runs it"); a callable no saved
// registration hands over is its name alone.
// Nothing is looked for beyond the saved kinds and facts.
func (builder *pageBuilder) ownWork(section *pageSection, flow *pageFlow, shownSubjects map[string]bool) []pageOwnWork {
	index := builder.graphIndex(section.programTargetID)
	if index == nil {
		return nil
	}
	var operations []groupindex.Operation
	for _, operation := range index.Operations {
		if (operation.Kind == "scheduled" || operation.Kind == "continuous") && operation.SubjectID != "" && !operation.HandlerUnknown {
			operations = append(operations, operation)
		}
	}
	slices.SortStableFunc(operations, func(a, b groupindex.Operation) int {
		return outerKindRank(a.Kind) - outerKindRank(b.Kind)
	})
	shown := map[string]bool{}
	if flow != nil {
		for _, step := range flow.Steps {
			if step.Key != "" {
				shown[step.Key] = true
			}
		}
	}
	seen := map[string]bool{}
	var work []pageOwnWork
	for _, operation := range operations {
		ref, known := builder.subject(section.programTargetID, operation.SubjectID)
		if !known || seen[operation.SubjectID] {
			continue
		}
		seen[operation.SubjectID] = true
		name, anchor := builder.subjectDisplay(ref.subject)
		if name == "" || shown[declarationKey(anchor)] {
			continue
		}
		row := pageOwnWork{Input: operationNodeID(section.ID, operation.ID)}
		fact, registered := builder.factsByID[operation.FactID]
		if !registered || fact.Kind != facts.KindRegistration || fact.ObjectID != operation.SubjectID || fact.OwnerID == "" ||
			!builder.ownWorkReading(&row.pageFlowStep, fact, section, shownSubjects) {
			own := builder.stepName(section, operation.SubjectID)
			row.pageFlowStep = pageFlowStep{Label: own.Name, Anchor: anchor, Part: own.Part, Key: own.Key}
		}
		work = append(work, row)
	}
	return work
}

// stepName names a declaration a Main flow step reads: its name, a method
// with its type, a callable written inline as a reader names it
// ("ReplicateCommand.Run (inline)", subjectDisplay), read in its part, its
// link all of its lines.
func (builder *pageBuilder) stepName(section *pageSection, subjectID string) pageStepName {
	ref, known := builder.subject(section.programTargetID, subjectID)
	if !known {
		return pageStepName{Name: subjectID}
	}
	name, anchor := builder.subjectDisplay(ref.subject)
	step := pageStepName{Name: builder.withType(section.programTargetID, ref.subject, name)}
	if anchor != nil {
		step.Code, step.Open = cmp.Or(anchor.Code, anchor.Href), anchor.Open
	}
	index := builder.graphIndex(section.programTargetID)
	if index == nil || anchor == nil {
		return step
	}
	if group := builder.edgesBetweenGroups(*index).groupOf[subjectID]; group != "" {
		step.Part, step.Key = "#"+groupAnchorID(section.ID, group), declarationKey(anchor)
	}
	return step
}

// inline says a subject is a callable written inline in another (a Go
// function literal, a lambda), as GroupsIndex says (ObjectFacts.Anonymous):
// nobody's declaration to look for, so it is no tile, no key and no member
// of a part's reading, whatever name it is read by. A dollar sign in a name
// says nothing: JavaScript's `export function price$()` is a declaration
// like any other (external review, 2026-10-03).
func (builder *pageBuilder) inline(subject groupindex.Subject) bool {
	return subject.Object != nil && subject.Object.Anonymous
}
