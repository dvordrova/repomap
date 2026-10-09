// Package flowtest checks what GroupsIndex derives on a language fixture's
// real facts: each input's reach, the dispatch sites and the phases.
// Check holds an index to the rules of reach.go; Probe reads the reach a
// declaration would have as an input, for fixtures that have no input of
// their own. Language fixture tests of every adapter share it.
package flowtest

import (
	"reflect"
	"slices"
	"strconv"
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// Check holds every derived value of index to its definition, on the facts
// it was derived from, and checks that the index hydrated from its saved
// overlay derives the same values.
func Check(t testing.TB, program programindex.Index, index groupindex.Index) {
	t.Helper()
	subjects := map[string]groupindex.Subject{}
	for _, subject := range index.Subjects {
		subjects[subject.ID] = subject
	}
	kind := func(id string) programindex.ObjectKind {
		if object := subjects[id].Object; object != nil {
			return object.Kind
		}
		return ""
	}
	executing := func(id string) bool {
		switch kind(id) {
		case programindex.ObjectFunction, programindex.ObjectMethod, programindex.ObjectLambda, programindex.ObjectModule:
			return true
		}
		return false
	}
	handlers := map[string][]string{}
	serves := false
	for _, operation := range index.Operations {
		if operation.SubjectID != "" {
			handlers[operation.SubjectID] = append(handlers[operation.SubjectID], operation.ID)
			serves = true
		}
	}
	if len(index.Reach) != len(index.Operations) {
		t.Fatalf("%d reaches for %d inputs", len(index.Reach), len(index.Operations))
	}
	group := map[string]string{}
	for _, g := range index.Groups {
		for _, id := range g.MemberSubjectIDs {
			group[id] = g.ID
		}
	}
	working := map[[2]string]bool{}
	for _, connection := range index.Connections {
		if connection.Phase == groupindex.PhaseRuntime || connection.Phase == groupindex.PhaseBoth {
			working[[2]string{connection.From.GroupID, connection.To.GroupID}] = true
		}
	}
	for position, reach := range index.Reach {
		operation := index.Operations[position]
		if reach.OperationID != operation.ID {
			t.Fatalf("reach %d is %s's, not %s's", position, reach.OperationID, operation.ID)
		}
		reached := map[string]bool{}
		for _, subject := range reach.Subjects {
			reached[subject.SubjectID] = true
		}
		if operation.SubjectID == "" {
			if len(reach.Subjects) != 0 {
				t.Fatalf("%s has no handler and reaches %d declarations", operation.Name, len(reach.Subjects))
			}
			continue
		}
		// What the walk expands: the handler and every call's target. A
		// declaration reached only by a read is never walked from.
		walked := map[string]bool{operation.SubjectID: true}
		for _, position := range reach.Edges {
			if edge := index.StructuralEdges[position]; edge.RelationKind != programindex.RelationReads {
				walked[edge.ToSubjectID] = true
			}
		}
		for _, position := range reach.Edges {
			edge := index.StructuralEdges[position]
			if edge.Role != groupindex.EdgeRelationTarget || !reached[edge.FromSubjectID] || !reached[edge.ToSubjectID] || !walked[edge.FromSubjectID] {
				t.Fatalf("%s follows %s→%s (%s), which does not start at a walked declaration", operation.Name, edge.FromSubjectID, edge.ToSubjectID, edge.RelationKind)
			}
			switch edge.RelationKind {
			case programindex.RelationCalls, programindex.RelationExecutes, programindex.RelationInvokesExternal:
				if edge.Resolution == programindex.ResolutionUnresolved {
					t.Fatalf("%s follows an unresolved call %s→%s", operation.Name, edge.FromSubjectID, edge.ToSubjectID)
				}
				if edge.Resolution == programindex.ResolutionAlternatives && edge.ToSubjectID != operation.SubjectID && len(handlers[edge.ToSubjectID]) > 0 {
					t.Fatalf("%s follows the dispatch %s into another input's handler %s", operation.Name, edge.FromSubjectID, edge.ToSubjectID)
				}
			case programindex.RelationReads:
				if to := kind(edge.ToSubjectID); !executing(edge.FromSubjectID) || to != programindex.ObjectVariable && to != programindex.ObjectType {
					t.Fatalf("%s follows a read %s→%s that is not code reading data", operation.Name, edge.FromSubjectID, edge.ToSubjectID)
				}
			default:
				t.Fatalf("%s follows %s %s→%s", operation.Name, edge.RelationKind, edge.FromSubjectID, edge.ToSubjectID)
			}
			if from, to := group[edge.FromSubjectID], group[edge.ToSubjectID]; from != "" && to != "" && from != to && !working[[2]string{from, to}] {
				t.Fatalf("%s crosses from %s to %s, which no runtime connection joins", operation.Name, from, to)
			}
		}
		for _, position := range reach.HandsOver {
			edge := index.StructuralEdges[position]
			if edge.RelationKind != programindex.RelationPassesCallback || !walked[edge.FromSubjectID] || !executing(edge.FromSubjectID) ||
				edge.ToSubjectID == operation.SubjectID || len(handlers[edge.ToSubjectID]) == 0 {
				t.Fatalf("%s hands over %s→%s, which is not running code handing another input's handler over", operation.Name, edge.FromSubjectID, edge.ToSubjectID)
			}
		}
	}
	if !serves {
		for _, subject := range index.Subjects {
			if subject.Phase != "" {
				t.Fatalf("%s has phase %q in a program with no handled input", subject.ID, subject.Phase)
			}
		}
	}
	reachOf := map[string]groupindex.Reach{}
	for _, reach := range index.Reach {
		reachOf[reach.OperationID] = reach
	}
	for _, site := range index.Dispatch {
		if len(site.Alternatives) < 2 {
			t.Fatalf("dispatch site %s has %d alternatives", site.RelationID, len(site.Alternatives))
		}
		var dispatched []string
		for _, operation := range index.Operations {
			if operation.SubjectID != "" && slices.Contains(site.Alternatives, operation.SubjectID) {
				dispatched = append(dispatched, operation.ID)
			}
		}
		if !slices.Equal(site.OperationIDs, dispatched) {
			t.Fatalf("dispatch site %s dispatches %v, want %v", site.RelationID, site.OperationIDs, dispatched)
		}
		for _, reached := range site.ReachedFrom {
			reach := reachOf[reached.OperationID]
			ends := false
			for _, position := range reached.Edges {
				edge := index.StructuralEdges[position]
				if !slices.Contains(reach.Edges, position) || edge.FromSubjectID == site.FromSubjectID {
					t.Fatalf("site %s: %s's route edge %s→%s is not in its reach or leaves the site", site.RelationID, reached.OperationID, edge.FromSubjectID, edge.ToSubjectID)
				}
				ends = ends || edge.ToSubjectID == site.FromSubjectID
			}
			if !ends && index.Operations[slices.IndexFunc(index.Operations, func(o groupindex.Operation) bool { return o.ID == reached.OperationID })].SubjectID != site.FromSubjectID {
				t.Fatalf("site %s: %s's routes do not end at it", site.RelationID, reached.OperationID)
			}
		}
	}
	// Only a part holding a seed, or a library's export, is the entry, and
	// only an area holding such a part.
	seeds := map[string]bool{}
	for _, seed := range index.Target.Seeds {
		seeds[seed.ObjectID] = true
	}
	for _, export := range index.Target.Exports {
		seeds[export.ObjectID] = true
	}
	lanes := map[string]groupindex.Lane{}
	for _, g := range index.Groups {
		lanes[g.ID] = g.Lane
		if (g.Lane == groupindex.LaneTriggers) != slices.ContainsFunc(g.MemberSubjectIDs, func(id string) bool { return seeds[id] }) {
			t.Fatalf("part %q is %s, and holding a seed or an export is its only entry", g.Title, g.Lane)
		}
	}
	for _, container := range index.Containers {
		if (container.Lane == groupindex.LaneTriggers) != slices.ContainsFunc(groupindex.ContainerGroups(index.Containers, container.ID), func(id string) bool { return lanes[id] == groupindex.LaneTriggers }) {
			t.Fatalf("area %q is %s, and holding the entry's part is its only entry", container.Title, container.Lane)
		}
	}
	hydrated, err := groupindex.OverlayFromIndex(index).Hydrate(program)
	if err != nil {
		t.Fatalf("hydrate: %v", err)
	}
	if !reflect.DeepEqual(index.Reach, hydrated.Reach) || !reflect.DeepEqual(index.Dispatch, hydrated.Dispatch) || !reflect.DeepEqual(index.Entries, hydrated.Entries) {
		t.Fatal("the hydrated index derives another reach or other dispatch sites")
	}
	for position, subject := range index.Subjects {
		if subject.Phase != hydrated.Subjects[position].Phase {
			t.Fatalf("%s is %q projected and %q hydrated", subject.ID, subject.Phase, hydrated.Subjects[position].Phase)
		}
	}
	for position, connection := range index.Connections {
		other := hydrated.Connections[position]
		if connection.Phase != other.Phase || connection.ToHelper != other.ToHelper || connection.Quiet != other.Quiet {
			t.Fatalf("connection %s is %q/%v/%v projected and %q/%v/%v hydrated", connection.ID, connection.Phase, connection.ToHelper, connection.Quiet, other.Phase, other.ToHelper, other.Quiet)
		}
	}
}

// Probe is the reach the declaration named name in the file path would have
// as an input of index: the same derivation with one more input, handled by
// it.
func Probe(t testing.TB, index groupindex.Index, path, name string) groupindex.Reach {
	t.Helper()
	var subject *groupindex.Subject
	for position := range index.Subjects {
		if object := index.Subjects[position].Object; object != nil && object.Name == name && object.Location != nil && object.Location.Path == path {
			if subject != nil {
				t.Fatalf("two declarations of %s are named %s", path, name)
			}
			subject = &index.Subjects[position]
		}
	}
	if subject == nil {
		t.Fatalf("no declaration of %s is named %s", path, name)
	}
	probe := index.Snapshot()
	probe.Operations = append(slices.Clone(index.Operations), groupindex.Operation{
		ID: "o" + strconv.Itoa(len(index.Operations)+1), SubjectID: subject.ID, Kind: "entry", Name: name, Source: "model", Location: *subject.Object.Location,
	})
	groupindex.Derive(&probe)
	return probe.Reach[len(probe.Reach)-1]
}

// Reached names what a reach holds, by declaration name, with how it got
// there: "read" for a declaration only a read reached, "call" otherwise.
func Reached(index groupindex.Index, reach groupindex.Reach) map[string]string {
	names := map[string]string{}
	for _, subject := range index.Subjects {
		if subject.Object != nil {
			names[subject.ID] = subject.Object.Name
		}
	}
	how := map[string]string{}
	for _, subject := range reach.Subjects {
		how[subject.SubjectID] = "read"
	}
	if len(reach.Subjects) > 0 {
		how[reach.Subjects[0].SubjectID] = "call"
	}
	for _, position := range reach.Edges {
		if edge := index.StructuralEdges[position]; edge.RelationKind != programindex.RelationReads {
			how[edge.ToSubjectID] = "call"
		}
	}
	result := map[string]string{}
	for id, way := range how {
		result[names[id]] = way
	}
	return result
}
