package groupindex

import (
	"fmt"
	"slices"
	"sync/atomic"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/programindex"
)

// Derived is what Derive computes from a GroupsIndex's facts: each input's
// reach and spine, the dispatch sites, the entries, the catalogues, the
// launch walk and the phases. It is computed once at analysis and saved
// with the index (Overlay), so a rendering only presents it (owner,
// 2026-10-01: "у html должна быть простая задача — вот данные,
// показываю"). Its positions name the structural edges the bound
// ProgramIndex compiles, in their compiled order.
type Derived struct {
	Reach      []Reach        `json:"reach"`
	Dispatch   []DispatchSite `json:"dispatch,omitempty"`
	Entries    []Entry        `json:"entries,omitempty"`
	Catalogues []Catalogue    `json:"catalogues,omitempty"`
	Launch     Launch         `json:"launch"`
	// SubjectPhases are the subjects' phases by subject ID, those with
	// none left out; Connections each connection's phase and flags, in
	// Connections order.
	SubjectPhases map[string]string `json:"subject_phases,omitempty"`
	Connections   []ConnectionPhase `json:"connections,omitempty"`
}

// derivations counts Derive's runs in this process: a rendering runs none,
// which the report's tests hold it to (Derivations).
var derivations atomic.Int64

// Derivations is how many times this process has run Derive.
func Derivations() int64 { return derivations.Load() }

// ConnectionPhase is what Derive says of one connection (phase.go).
type ConnectionPhase struct {
	Phase    string `json:"phase,omitempty"`
	ToHelper bool   `json:"to_helper,omitempty"`
	Quiet    bool   `json:"quiet,omitempty"`
}

// DerivedOf reads what Derive left on an index, never deriving anything.
func DerivedOf(index Index) Derived {
	result := Derived{Reach: index.Reach, Dispatch: index.Dispatch, Entries: index.Entries, Catalogues: index.Catalogues, Launch: index.Launch}
	for _, subject := range index.Subjects {
		if subject.Phase != "" {
			if result.SubjectPhases == nil {
				result.SubjectPhases = map[string]string{}
			}
			result.SubjectPhases[subject.ID] = subject.Phase
		}
	}
	if len(index.Connections) > 0 {
		result.Connections = make([]ConnectionPhase, len(index.Connections))
		for position, connection := range index.Connections {
			result.Connections[position] = ConnectionPhase{Phase: connection.Phase, ToHelper: connection.ToHelper, Quiet: connection.Quiet}
		}
	}
	return result
}

// ApplyDerived puts saved derivations on an index whose facts they were
// derived from: its operations and connections, in their order. The
// derivations are shared read-only, as Snapshot shares them.
func (index *Index) ApplyDerived(derived Derived) error {
	if len(derived.Reach) != len(index.Operations) || len(derived.Connections) != len(index.Connections) {
		return fmt.Errorf("group index: saved derivations name %d inputs and %d connections, the index %d and %d",
			len(derived.Reach), len(derived.Connections), len(index.Operations), len(index.Connections))
	}
	for _, reach := range derived.Reach {
		for _, edge := range reach.Edges {
			if edge < 0 || edge >= len(index.StructuralEdges) {
				return fmt.Errorf("group index: saved reach names structural edge %d of %d", edge, len(index.StructuralEdges))
			}
		}
	}
	index.Reach, index.Dispatch, index.Entries, index.Catalogues, index.Launch = derived.Reach, derived.Dispatch, derived.Entries, derived.Catalogues, derived.Launch
	for position := range index.Subjects {
		index.Subjects[position].Phase = derived.SubjectPhases[index.Subjects[position].ID]
	}
	for position := range index.Connections {
		phase := derived.Connections[position]
		index.Connections[position].Phase, index.Connections[position].ToHelper, index.Connections[position].Quiet = phase.Phase, phase.ToHelper, phase.Quiet
	}
	return nil
}

// TestPaths are the files a set of programs' known testing material is
// written in: each target's test sources and the files a Go build selected
// as test declarations (go_test_declaration witnesses). A build-selected
// witness, never a guessed filename role.
func TestPaths(indexes []Index, programs []programindex.Index) map[string]bool {
	paths := make(map[string]bool)
	for _, index := range indexes {
		for _, source := range index.Target.TestSources {
			paths[source] = true
		}
	}
	for _, program := range programs {
		addTestWitnessPaths(program, paths)
	}
	return paths
}

// addTestWitnessPaths adds the files a program's Go build selected as test
// declarations.
func addTestWitnessPaths(program programindex.Index, paths map[string]bool) {
	for _, relation := range program.Relations {
		for _, witness := range relation.Witnesses {
			if witness.Kind == "go_test_declaration" && witness.Location != nil {
				paths[witness.Location.Path] = true
			}
		}
	}
}

// TestFreeViews are copies of a set of indexes without their known testing
// material: its subjects, groups emptied by it, its inputs, outbound calls,
// data, structural edges and the connections touching it or a group it
// emptied. The overview reads them; the saved index keeps everything. They
// carry no derivations: analysis derives them once (WithTestFreeViews) and
// a rendering applies those (Index.TestFree).
func TestFreeViews(indexes []Index, testPaths map[string]bool) []Index {
	testSubjects := make(map[SubjectEndpoint]bool)
	testLocation := func(location *programindex.Location) bool { return location != nil && testPaths[location.Path] }
	for _, index := range indexes {
		for _, subject := range index.Subjects {
			test := subject.Object != nil && testLocation(subject.Object.Location) || subject.Pattern != nil && testLocation(subject.Pattern.Location)
			if test {
				testSubjects[SubjectEndpoint{TargetID: index.Target.ID, SubjectID: subject.ID}] = true
			}
		}
	}
	views := make([]Index, len(indexes))
	visibleGroups := make(map[Endpoint]bool)
	for i, index := range indexes {
		projected := index
		testSubject := func(id string) bool {
			return testSubjects[SubjectEndpoint{TargetID: index.Target.ID, SubjectID: id}]
		}
		projected.Subjects = slices.DeleteFunc(slices.Clone(index.Subjects), func(subject Subject) bool { return testSubject(subject.ID) })
		projected.Groups = nil
		for _, group := range index.Groups {
			members := slices.DeleteFunc(slices.Clone(group.MemberSubjectIDs), testSubject)
			if len(members) == 0 && len(group.MemberSubjectIDs) > 0 {
				continue
			}
			group.MemberSubjectIDs = members
			group.EvidenceSubjectIDs = slices.DeleteFunc(slices.Clone(group.EvidenceSubjectIDs), testSubject)
			projected.Groups = append(projected.Groups, group)
			visibleGroups[Endpoint{TargetID: index.Target.ID, GroupID: group.ID}] = true
		}
		projected.Operations = slices.DeleteFunc(slices.Clone(index.Operations), func(operation Operation) bool {
			return testSubject(operation.SubjectID) || testLocation(&operation.Location)
		})
		projected.Outbound = nil
		for _, call := range index.Outbound {
			if testSubject(call.SubjectID) || testLocation(&call.Location) {
				continue
			}
			if len(call.Uses) > 0 {
				call.Uses = slices.DeleteFunc(slices.Clone(call.Uses), func(use atlas.DestinationUse) bool {
					return slices.ContainsFunc(use.Steps, func(step atlas.DestinationStep) bool { return testPaths[step.Path] })
				})
				if len(call.Uses) == 0 {
					continue
				}
			}
			projected.Outbound = append(projected.Outbound, call)
		}
		projected.Data = slices.DeleteFunc(slices.Clone(index.Data), func(record DataRecord) bool {
			return testPaths[record.Path] || testSubject(record.OwnerSubjectID) ||
				record.Data != nil && record.Data.Owner != nil && testPaths[record.Data.Owner.Path]
		})
		projected.StructuralEdges = slices.DeleteFunc(slices.Clone(index.StructuralEdges), func(edge StructuralEdge) bool {
			return testSubject(edge.FromSubjectID) || testSubject(edge.ToSubjectID)
		})
		projected.Containers = nil
		for _, container := range index.Containers {
			container.GroupIDs = slices.DeleteFunc(slices.Clone(container.GroupIDs), func(id string) bool {
				return !visibleGroups[Endpoint{TargetID: index.Target.ID, GroupID: id}]
			})
			projected.Containers = append(projected.Containers, container)
		}
		projected.Containers = pruneEmptyContainers(projected.Containers)
		views[i] = projected
	}
	for i, view := range views {
		views[i].Connections = slices.DeleteFunc(slices.Clone(view.Connections), func(connection Connection) bool {
			if !visibleGroups[connection.From] || !visibleGroups[connection.To] || testLocation(connection.FromLocation) || testLocation(connection.ToLocation) {
				return true
			}
			if testSubjects[SubjectEndpoint{TargetID: connection.From.TargetID, SubjectID: connection.FromSubjectID}] ||
				testSubjects[SubjectEndpoint{TargetID: connection.To.TargetID, SubjectID: connection.ToSubjectID}] {
				return true
			}
			// Mixed production/test support still supports an overview
			// relation. Unknown evidence never becomes a test merely by
			// association.
			for _, evidence := range connection.Evidence {
				if !testSubjects[evidence] {
					return false
				}
			}
			return len(connection.Evidence) > 0
		})
		views[i].TestFree = nil
	}
	return views
}

// WithTestFreeViews derives each index's test-free view once (Fold,
// TestFreeViews, Derive) and keeps it on the index (TestFree), before the indexes are
// sealed: the overview presents it, a rendering never derives it. witnessed
// are the files the programs' builds selected as tests (TestPaths).
func WithTestFreeViews(indexes []Index, witnessed map[string]bool) {
	paths := TestPaths(indexes, nil)
	for path := range witnessed {
		paths[path] = true
	}
	// The overview reads the folded graph (Fold), as the page does.
	views := TestFreeViews(Fold(indexes), paths)
	for i := range views {
		Derive(&views[i])
		derived := DerivedOf(views[i])
		indexes[i].TestFree = &derived
	}
}
