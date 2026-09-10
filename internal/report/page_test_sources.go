package report

import (
	"slices"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// overviewBuilder removes known testing material only from the orientation
// view. The saved graph, full ProgramView, question evidence and source checks
// retain every original declaration and connection.
func (builder *pageBuilder) overviewBuilder() *pageBuilder {
	view := *builder
	view.sourceIndexes = builder.indexes
	view.testPaths = make(map[string]bool)
	for _, index := range builder.indexes {
		for _, source := range index.Target.TestSources {
			view.testPaths[source] = true
		}
	}
	// Older current-format Go producers already retained this native witness.
	// It is a build-selected test declaration, never a guessed filename role.
	if builder.data != nil && builder.data.ProgramPortfolio != nil {
		for _, entry := range builder.data.ProgramPortfolio.Entries {
			for _, relation := range entry.View.Relations {
				for _, witness := range relation.Witnesses {
					if witness.Kind == "go_test_declaration" && witness.Location != nil {
						view.testPaths[witness.Location.Path] = true
					}
				}
			}
		}
	}
	testSubjects := make(map[groupindex.SubjectEndpoint]bool)
	testLocation := func(location *programindex.Location) bool { return location != nil && view.testPaths[location.Path] }
	for _, index := range builder.indexes {
		for _, subject := range index.Subjects {
			test := subject.Object != nil && testLocation(subject.Object.Location) || subject.Pattern != nil && testLocation(subject.Pattern.Location)
			if test {
				testSubjects[groupindex.SubjectEndpoint{TargetID: index.Target.ID, SubjectID: subject.ID}] = true
			}
		}
	}
	view.indexes = make([]groupindex.Index, len(builder.indexes))
	visibleGroups := make(map[groupindex.Endpoint]bool)
	for i, index := range builder.indexes {
		projected := index
		testSubject := func(id string) bool {
			return testSubjects[groupindex.SubjectEndpoint{TargetID: index.Target.ID, SubjectID: id}]
		}
		projected.Subjects = slices.DeleteFunc(slices.Clone(index.Subjects), func(subject groupindex.Subject) bool { return testSubject(subject.ID) })
		projected.Groups = nil
		for _, group := range index.Groups {
			members := slices.DeleteFunc(slices.Clone(group.MemberSubjectIDs), testSubject)
			if len(members) == 0 && len(group.MemberSubjectIDs) > 0 {
				continue
			}
			group.MemberSubjectIDs = members
			group.EvidenceSubjectIDs = slices.DeleteFunc(slices.Clone(group.EvidenceSubjectIDs), testSubject)
			projected.Groups = append(projected.Groups, group)
			visibleGroups[groupindex.Endpoint{TargetID: index.Target.ID, GroupID: group.ID}] = true
		}
		projected.Operations = slices.DeleteFunc(slices.Clone(index.Operations), func(operation groupindex.Operation) bool {
			return testSubject(operation.SubjectID) || testLocation(&operation.Location)
		})
		projected.Outbound = nil
		for _, call := range index.Outbound {
			if testSubject(call.SubjectID) || testLocation(&call.Location) {
				continue
			}
			if len(call.Uses) > 0 {
				call.Uses = slices.DeleteFunc(slices.Clone(call.Uses), func(use atlas.DestinationUse) bool {
					return slices.ContainsFunc(use.Steps, func(step atlas.DestinationStep) bool { return view.testPaths[step.Path] })
				})
				if len(call.Uses) == 0 {
					continue
				}
			}
			projected.Outbound = append(projected.Outbound, call)
		}
		projected.Data = slices.DeleteFunc(slices.Clone(index.Data), func(record groupindex.DataRecord) bool {
			return view.testPaths[record.Path] || testSubject(record.OwnerSubjectID) ||
				record.Data != nil && record.Data.Owner != nil && view.testPaths[record.Data.Owner.Path]
		})
		projected.StructuralEdges = slices.DeleteFunc(slices.Clone(index.StructuralEdges), func(edge groupindex.StructuralEdge) bool {
			return testSubject(edge.FromSubjectID) || testSubject(edge.ToSubjectID)
		})
		projected.Containers = nil
		for _, container := range index.Containers {
			container.GroupIDs = slices.DeleteFunc(slices.Clone(container.GroupIDs), func(id string) bool {
				return !visibleGroups[groupindex.Endpoint{TargetID: index.Target.ID, GroupID: id}]
			})
			if len(container.GroupIDs) > 0 {
				projected.Containers = append(projected.Containers, container)
			}
		}
		view.indexes[i] = projected
	}
	for i, index := range view.indexes {
		view.indexes[i].Connections = slices.DeleteFunc(slices.Clone(index.Connections), func(connection groupindex.Connection) bool {
			if !visibleGroups[connection.From] || !visibleGroups[connection.To] || testLocation(connection.FromLocation) || testLocation(connection.ToLocation) {
				return true
			}
			if testSubjects[groupindex.SubjectEndpoint{TargetID: connection.From.TargetID, SubjectID: connection.FromSubjectID}] ||
				testSubjects[groupindex.SubjectEndpoint{TargetID: connection.To.TargetID, SubjectID: connection.ToSubjectID}] {
				return true
			}
			// Mixed production/test support still supports an overview relation.
			// Unknown evidence never becomes a test merely by association.
			for _, evidence := range connection.Evidence {
				if !testSubjects[evidence] {
					return false
				}
			}
			return len(connection.Evidence) > 0
		})
	}
	return &view
}
