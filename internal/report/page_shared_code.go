package report

import (
	"slices"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// sharedCode joins the programs of this report by the declarations they
// hold in common, by exact identity (groupindex.DeclarationKey: path, line,
// column, kind and name; never a name): anet.c's anetTcpConnect is one
// declaration that redis-cli's cliConnect and redis-server's syncWithMaster
// call, each in its own program. Built once.
type sharedCode struct {
	// holders are, by declaration key, the programs holding it and its
	// object there, in page order.
	holders map[string][]sharedHolder
	// keys are each program's objects' declaration keys, by
	// target-qualified object ID.
	keys map[string]string
	// unreachable marks the target-qualified objects their program's
	// adapter proved it never runs (ProgramIndex unreachable).
	unreachable map[string]bool
}

type sharedHolder struct {
	targetID, objectID string
}

func (builder *pageBuilder) sharedJoin() *sharedCode {
	if builder.shared != nil {
		return builder.shared
	}
	join := &sharedCode{holders: map[string][]sharedHolder{}, keys: map[string]string{}, unreachable: map[string]bool{}}
	builder.shared = join
	if builder.data == nil || builder.data.ProgramPortfolio == nil {
		return join
	}
	for _, section := range builder.sections {
		for _, index := range builder.data.ProgramPortfolio.Entries {
			if index.Target.ID != section.programTargetID {
				continue
			}
			for _, object := range index.Objects {
				key := groupindex.DeclarationKey(object)
				if key == "" {
					continue
				}
				qualified := subjectKey(index.Target.ID, object.ID)
				join.keys[qualified] = key
				join.unreachable[qualified] = object.Unreachable
				join.holders[key] = append(join.holders[key], sharedHolder{targetID: index.Target.ID, objectID: object.ID})
			}
		}
	}
	return join
}

// neverRun says whether a declaration's program never runs it (its adapter
// proved it unreachable there).
func (builder *pageBuilder) neverRun(targetID, subjectID string) bool {
	return builder.sharedJoin().unreachable[subjectKey(targetID, subjectID)]
}

// callerElsewhere is one call into a shared declaration made by another
// program's own code: the program, the caller and the call's site.
type callerElsewhere struct {
	targetID string
	caller   string
	edge     groupindex.StructuralEdge
}

// callersElsewhere are the calls other programs of this report make into
// the same declaration, each program's callers it runs, in page order
// (owner, 2026-09-28: "Called by" of anetTcpConnect had listed only one
// program's callers).
func (builder *pageBuilder) callersElsewhere(targetID, subjectID string) []callerElsewhere {
	join := builder.sharedJoin()
	key := join.keys[subjectKey(targetID, subjectID)]
	if key == "" {
		return nil
	}
	var result []callerElsewhere
	for _, holder := range join.holders[key] {
		if holder.targetID == targetID || join.unreachable[subjectKey(holder.targetID, holder.objectID)] {
			continue
		}
		index := builder.graphIndex(holder.targetID)
		if index == nil {
			continue
		}
		for _, edge := range index.StructuralEdges {
			if edge.Role != groupindex.EdgeRelationTarget || edge.ToSubjectID != holder.objectID || !callsInto(edge.RelationKind) ||
				join.unreachable[subjectKey(holder.targetID, edge.FromSubjectID)] {
				continue
			}
			if slices.ContainsFunc(result, func(listed callerElsewhere) bool {
				return listed.targetID == holder.targetID && listed.caller == edge.FromSubjectID
			}) {
				continue
			}
			result = append(result, callerElsewhere{targetID: holder.targetID, caller: edge.FromSubjectID, edge: edge})
		}
	}
	return result
}

func callsInto(kind programindex.RelationKind) bool {
	return kind == programindex.RelationCalls || kind == programindex.RelationExecutes
}
