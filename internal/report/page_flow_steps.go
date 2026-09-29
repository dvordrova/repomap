package report

import (
	"cmp"
	"slices"
	"strings"

	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// pageStepName is a declaration a Main flow step names beside its own:
// read in its part (Part, Key) when one holds it, and a link into its code
// (Code, all of its lines on a static page; Open, a served page's source).
type pageStepName struct {
	Name string
	Part string
	Key  string
	Code string
	Open string
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

// pageStepPath is what a Main flow's earlier steps named, in order: the
// declarations a later registration is reached from, and the functions
// already shown running registered callables with the calls reaching them.
type pageStepPath struct {
	shown   []string
	runners map[string]bool
}

// registeredStep reads a step citing a registration (owner, 2026-09-29:
// three of redis-server's six steps had read the registrar,
// aeCreateFileEvent, and aeMain, aeProcessEvents and createClient were
// gone) as the callable it registers, a name reading it, with how it comes
// to run, all from the program's own facts and calls:
//
//   - where it is registered: of the registrations of that callable in this
//     program, those whose registering function the most recent earlier step
//     reaches by exact calls, each with that run of calls (acceptHandler →
//     createClient); none reached, every one (from an entry when one reaches
//     it). Never an arbitrary first one: readQueryFromClient's step had linked
//     the one in beforeSleep's resume path;
//   - what runs it: each function calling it through a value (a dispatch's
//     alternatives, a call through a function value, or an open call whose
//     stores name it), after the run of exact calls from the program's
//     entries reaching that function the first time the flow shows it (main
//     → aeMain → aeProcessEvents), from the last runner already shown on
//     that run when there is one.
func (builder *pageBuilder) registeredStep(row *pageFlowStep, fact facts.Fact, section *pageSection, path *pageStepPath) bool {
	index := builder.graphIndex(section.programTargetID)
	if index == nil || builder.data.Facts == nil {
		return false
	}
	ref, known := builder.subject(section.programTargetID, fact.ObjectID)
	if !known || ref.subject.Object == nil || ref.subject.Object.Location == nil {
		return false
	}
	groupOf := builder.edgesBetweenGroups(*index).groupOf
	named := func(subjectID string) pageStepName {
		ref, known := builder.subject(section.programTargetID, subjectID)
		if !known {
			return pageStepName{Name: subjectID}
		}
		name, anchor := builder.subjectDisplay(ref.subject)
		step := pageStepName{Name: builder.withType(section.programTargetID, ref.subject, name)}
		if anchor != nil {
			step.Code, step.Open = cmp.Or(anchor.Code, anchor.Href), anchor.Open
		}
		if group := groupOf[subjectID]; group != "" && anchor != nil {
			step.Part, step.Key = "#"+groupAnchorID(section.ID, group), declarationKey(anchor)
		}
		return step
	}
	names := func(chain []string) []pageStepName {
		out := make([]pageStepName, len(chain))
		for i, id := range chain {
			out[i] = named(id)
		}
		return out
	}
	// The step's name is the callable's, its link all of the callable's
	// lines; the registering call's line is its registration's.
	own := named(fact.ObjectID)
	_, ownAnchor := builder.subjectDisplay(ref.subject)
	row.Label, row.Part, row.Key, row.Anchor = own.Name, own.Part, own.Key, ownAnchor
	var calls []int
	for position, edge := range index.StructuralEdges {
		if edge.Role == groupindex.EdgeRelationTarget && edge.RelationKind == programindex.RelationCalls && edge.Resolution == programindex.ResolutionExact {
			calls = append(calls, position)
		}
	}
	var entries []string
	for _, entry := range index.Entries {
		entries = append(entries, entry.SubjectID)
	}
	fromEntries := func(to string) []string {
		for _, entry := range entries {
			if chain := chainOf(index, calls, entry, to); chain != nil {
				return chain
			}
		}
		return nil
	}
	var sites []facts.Fact
	for _, other := range builder.data.Facts.OfKind(facts.KindRegistration) {
		if other.TargetID == fact.TargetID && other.ObjectID == fact.ObjectID && other.OwnerID != "" {
			sites = append(sites, other)
		}
	}
	registration := func(site facts.Fact, chain []string) pageStepRegistration {
		if chain == nil {
			chain = []string{site.OwnerID}
		}
		return pageStepRegistration{By: names(chain), At: builder.links.factAnchor(site)}
	}
	starts := path.shown
	if len(starts) == 0 {
		starts = entries
	}
	for at := len(starts) - 1; at >= 0 && row.Registers == nil; at-- {
		for _, site := range sites {
			if chain := chainOf(index, calls, starts[at], site.OwnerID); chain != nil {
				row.Registers = append(row.Registers, registration(site, chain))
			}
		}
	}
	if row.Registers == nil {
		for _, site := range sites {
			row.Registers = append(row.Registers, registration(site, fromEntries(site.OwnerID)))
		}
	}
	// Registrations by the same run of calls read once, linking the first
	// registering call: sizeWorkspace had read "canvas registers it" four
	// times, one function's four calls.
	byChain := map[string]bool{}
	row.Registers = slices.DeleteFunc(row.Registers, func(site pageStepRegistration) bool {
		var key strings.Builder
		for _, name := range site.By {
			key.WriteString(name.Name + "\x00" + name.Key + "\x00")
		}
		seen := byChain[key.String()]
		byChain[key.String()] = true
		return seen
	})
	for _, runner := range builder.runnersOf(section.programTargetID, fact.ObjectID) {
		chain := []string{runner}
		if !path.runners[runner] {
			// From the entries, or from the last runner already shown on
			// the way: "aeProcessEvents → processTimeEvents runs it" after
			// "main → aeMain → aeProcessEvents runs it".
			if reached := fromEntries(runner); reached != nil {
				chain = reached
				for at := len(reached) - 2; at > 0; at-- {
					if path.runners[reached[at]] {
						chain = reached[at:]
						break
					}
				}
			}
			path.runners[runner] = true
		}
		row.RunBy = append(row.RunBy, names(chain))
		path.shown = append(path.shown, chain...)
	}
	path.shown = append(path.shown, fact.ObjectID)
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
// registration of it reads as registeredStep reads it, from the program's
// entries, a runner the flow has already reached by its entries named
// alone ("serverCron — main → initServer registers it; aeProcessEvents runs
// it"); a callable no saved registration hands over is its name alone.
// Nothing is looked for beyond the saved kinds and facts.
func (builder *pageBuilder) ownWork(section *pageSection, flow *pageFlow, path *pageStepPath) []pageOwnWork {
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
	if path == nil {
		path = &pageStepPath{runners: map[string]bool{}}
	}
	groupOf := builder.edgesBetweenGroups(*index).groupOf
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
		name = builder.withType(section.programTargetID, ref.subject, name)
		row := pageOwnWork{Input: operationNodeID(section.ID, operation.ID)}
		fact, registered := builder.factsByID[operation.FactID]
		if !registered || fact.Kind != facts.KindRegistration || fact.ObjectID != operation.SubjectID || fact.OwnerID == "" ||
			!builder.registeredStep(&row.pageFlowStep, fact, section, &pageStepPath{runners: path.runners}) {
			row.pageFlowStep = pageFlowStep{Label: name, Anchor: anchor}
			if group := groupOf[operation.SubjectID]; group != "" && anchor != nil {
				row.Part, row.Key = "#"+groupAnchorID(section.ID, group), declarationKey(anchor)
			}
		}
		work = append(work, row)
	}
	return work
}
