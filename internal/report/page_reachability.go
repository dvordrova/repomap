package report

import (
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// Execution adjacency excludes imports, reads and callback transfers. A
// declaration sharing a displayed part is not evidence that it executes.
func executionAdjacency(index *groupindex.Index) map[string][]groupindex.StructuralEdge {
	adj := make(map[string][]groupindex.StructuralEdge)
	for _, edge := range index.StructuralEdges {
		if edge.Role != groupindex.EdgeRelationTarget {
			continue
		}
		switch edge.RelationKind {
		case programindex.RelationCalls, programindex.RelationExecutes, programindex.RelationInvokesExternal:
			adj[edge.FromSubjectID] = append(adj[edge.FromSubjectID], edge)
		}
	}
	return adj
}

// Data reads are terminal dependencies. Reading a value or a type never
// executes its owner, a callback stored in it, or that callback's effects.
func dataReadAdjacency(index *groupindex.Index) map[string][]groupindex.StructuralEdge {
	data := make(map[string]bool)
	readers := make(map[string]bool)
	for _, subject := range index.Subjects {
		if subject.Object != nil && (subject.Object.Kind == programindex.ObjectVariable || subject.Object.Kind == programindex.ObjectType) {
			data[subject.ID] = true
		}
		if subject.Object != nil {
			switch subject.Object.Kind {
			case programindex.ObjectFunction, programindex.ObjectMethod, programindex.ObjectLambda, programindex.ObjectModule:
				readers[subject.ID] = true
			}
		}
	}
	adj := make(map[string][]groupindex.StructuralEdge)
	for _, edge := range index.StructuralEdges {
		if edge.Role == groupindex.EdgeRelationTarget && edge.RelationKind == programindex.RelationReads && readers[edge.FromSubjectID] && data[edge.ToSubjectID] {
			adj[edge.FromSubjectID] = append(adj[edge.FromSubjectID], edge)
		}
	}
	return adj
}

func reachedSubjects(root string, adj map[string][]groupindex.StructuralEdge) map[string]bool {
	seen := map[string]bool{}
	queue := []string{root}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		for _, edge := range adj[id] {
			queue = append(queue, edge.ToSubjectID)
		}
	}
	return seen
}
