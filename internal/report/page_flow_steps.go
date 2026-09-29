package report

import (
	"slices"

	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// pageStepName is a declaration a Main flow step names beside its own:
// read in its part (Part, Key) when one holds it.
type pageStepName struct {
	Name string
	Part string
	Key  string
}

// pageStepRegistration is one place a step's callable is registered: the
// run of calls reaching the function that registers it, that function last,
// and the registering call's own line.
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
//     alternatives, or an open call whose stores name it), after the run of
//     exact calls from the program's entries reaching that function the first
//     time the flow shows it (main → aeMain → aeProcessEvents).
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
		step := pageStepName{Name: name}
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
	own := named(fact.ObjectID)
	row.Label, row.Part, row.Key, row.Anchor = own.Name, own.Part, own.Key, nil
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
	if len(row.Registers) == 1 {
		row.Anchor = row.Registers[0].At
	}
	for _, runner := range builder.runnersOf(section.programTargetID, fact.ObjectID) {
		chain := []string{runner}
		if !path.runners[runner] {
			if reached := fromEntries(runner); reached != nil {
				chain = reached
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
// value, in relation order, each once: a call whose alternatives hold it,
// or an open call whose witnesses name it among the values it may call
// (C's stores into fe->rfileProc).
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
			if relation.Kind != programindex.RelationCalls || relation.Resolution == programindex.ResolutionExact || slices.Contains(runners, relation.FromID) {
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
