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
	// object there, in page order: the programs whose map claims the file
	// declaring it (heldFiles). A project index several programs share
	// (Python's) declares every file for each of them; a script program
	// holds its own file and what it imports there, never the package its
	// siblings are built from.
	holders map[string][]sharedHolder
	// held are, by target, the files its program holds (heldFiles); nil
	// for a program whose index draws no map and names no file off it,
	// which holds what its ProgramIndex declares.
	held map[string]map[string]bool
	// keys are each program's objects' declaration keys, by
	// target-qualified object ID.
	keys map[string]string
	// unreachable marks the target-qualified objects their program's
	// adapter proved it never runs (ProgramIndex unreachable).
	unreachable map[string]bool
	// into are, by target, its program's calls by the declaration they
	// call (callsIntoOf), built once per program: beets' render had
	// scanned each sharing program's every edge once per member of every
	// part (callersElsewhere, 30 of its 64 s).
	into map[string]map[string][]groupindex.StructuralEdge
}

type sharedHolder struct {
	targetID, objectID string
}

func (builder *pageBuilder) sharedJoin() *sharedCode {
	if builder.shared != nil {
		return builder.shared
	}
	join := &sharedCode{holders: map[string][]sharedHolder{}, keys: map[string]string{}, unreachable: map[string]bool{}, held: map[string]map[string]bool{},
		into: map[string]map[string][]groupindex.StructuralEdge{}}
	builder.shared = join
	if builder.data == nil || builder.data.ProgramPortfolio == nil {
		return join
	}
	for _, section := range builder.sections {
		// Ownership and declaration identity read the same complete target.
		// Two portfolio-wide passes would evict every target before the
		// second pass and decode every file again.
		entries := builder.nativeTargets(section.programTargetID)
		join.held[section.programTargetID] = heldFiles(builder.graphIndex(section.programTargetID), entries)
		for _, index := range entries {
			if index.Target.ID != section.programTargetID {
				continue
			}
			for _, object := range index.Objects {
				key := groupindex.DeclarationKey(object)
				if key == "" || !join.holds(index.Target.ID, object.Location) {
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

// heldFiles are the files a program's map claims: those of the
// declarations its parts hold and those it names off the map (GroupsIndex
// OffMap), by path; nil when its index draws no map and names no file off
// it.
func heldFiles(index *groupindex.Index, entries []programindex.Index) map[string]bool {
	if index == nil || len(index.Groups) == 0 && len(index.OffMap) == 0 {
		return nil
	}
	files := map[string]bool{}
	for _, file := range index.OffMap {
		files[file.Path] = true
	}
	members := map[string]bool{}
	for _, group := range index.Groups {
		for _, id := range group.MemberSubjectIDs {
			members[id] = true
		}
	}
	for _, entry := range entries {
		if entry.Target.ID != index.Target.ID {
			continue
		}
		for _, object := range entry.Objects {
			if members[object.ID] && object.Location != nil {
				files[object.Location.Path] = true
			}
		}
	}
	return files
}

// holds says whether a program holds the declaration written at a place:
// its map claims the file (heldFiles).
func (join *sharedCode) holds(targetID string, at *programindex.Location) bool {
	files, drawn := join.held[targetID]
	if !drawn || files == nil {
		return true
	}
	return at != nil && files[at.Path]
}

// holdsSubject says whether a program holds one of its index's subjects.
func (builder *pageBuilder) holdsSubject(targetID, subjectID string) bool {
	join := builder.sharedJoin()
	if files, drawn := join.held[targetID]; !drawn || files == nil {
		return true
	}
	ref, known := builder.subject(targetID, subjectID)
	if !known || ref.subject.Object == nil {
		return false
	}
	return join.holds(targetID, ref.subject.Object.Location)
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
// program's callers). A caller is listed under a program only when that
// program holds the caller: freqtrade's FreqtradeBot.process had listed
// Worker._process_running under each of five script programs sharing the
// project's index, none of which holds worker.py.
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
		for _, edge := range builder.callsIntoOf(holder.targetID)[holder.objectID] {
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

// callsIntoOf are a program's calls by the declaration they call, each
// list in the order its index holds them: a call or execution its own
// code makes, never a declaration's call of itself (othello.ai/move's
// two-argument form calling its three-argument form had been listed again
// under the app program), a caller the program never runs or a caller it
// does not hold.
func (builder *pageBuilder) callsIntoOf(targetID string) map[string][]groupindex.StructuralEdge {
	join := builder.sharedJoin()
	if calls, built := join.into[targetID]; built {
		return calls
	}
	calls := map[string][]groupindex.StructuralEdge{}
	if index := builder.graphIndex(targetID); index != nil {
		for _, edge := range index.StructuralEdges {
			if edge.Role != groupindex.EdgeRelationTarget || !callsInto(edge.RelationKind) || edge.FromSubjectID == edge.ToSubjectID ||
				join.unreachable[subjectKey(targetID, edge.FromSubjectID)] || !builder.holdsSubject(targetID, edge.FromSubjectID) {
				continue
			}
			calls[edge.ToSubjectID] = append(calls[edge.ToSubjectID], edge)
		}
	}
	join.into[targetID] = calls
	return calls
}

// own says whether a declaration is its program's own code: no other
// program of the report holds it.
func (join *sharedCode) own(targetID, subjectID string) bool {
	key := join.keys[subjectKey(targetID, subjectID)]
	return key == "" || len(join.holders[key]) < 2
}

// ownPath is the shortest run of calls, in one program, from the nearest
// declaration of its own code to subjectID, each declaration once: in
// redis-cli the connect written in shared anet.c is reached from
// cliConnect → anetTcpConnect → anetTcpGenericConnect. A caller its
// program never runs is on no path, and calls are taken in the order
// the program's index holds them. A declaration of the program's own code
// is its own path; nil when no own code of the program calls it.
func (builder *pageBuilder) ownPath(targetID, subjectID string) []string {
	join := builder.sharedJoin()
	if subjectID == "" || join.own(targetID, subjectID) {
		return []string{subjectID}
	}
	if builder.graphIndex(targetID) == nil {
		return nil
	}
	calls := builder.callsIntoOf(targetID)
	next := map[string]string{subjectID: ""}
	queue := []string{subjectID}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, edge := range calls[current] {
			caller := edge.FromSubjectID
			if _, seen := next[caller]; seen {
				continue
			}
			next[caller] = current
			if join.own(targetID, caller) {
				path := []string{caller}
				for at := current; at != ""; at = next[at] {
					path = append(path, at)
				}
				return path
			}
			queue = append(queue, caller)
		}
	}
	return nil
}

// pageCallSide is one program's side of a call that leaves it: the
// program, and the run of calls from the nearest function of its own code
// to the declaration the call is written in or lands in, each with the
// part it is read in. A call between two programs has two sides
// ("redis-cli: cliConnect → anetTcpConnect → anetTcpGenericConnect ⇢
// redis-server: acceptHandler → anetAccept"), a call to an outside
// endpoint one: the shared anet pair alone had named neither program.
type pageCallSide struct {
	Program string         `json:"program"`
	Path    []pageSideStep `json:"path"`
}

type pageSideStep struct {
	Name string `json:"name"`
	// Key is the declaration as the page's script keys it, Part the map
	// node of the part it is read in; either is empty when the report has
	// none.
	Key  string `json:"key,omitempty"`
	Part string `json:"part,omitempty"`
}

// callSide is a program's side of a call made or taken by subjectID; nil
// when the declaration is none the report names.
func (builder *pageBuilder) callSide(targetID, subjectID string) *pageCallSide {
	section := builder.byProgram[targetID]
	if section == nil {
		return nil
	}
	path := builder.ownPath(targetID, subjectID)
	if path == nil {
		// No own code of the program calls it: the declaration stands alone.
		path = []string{subjectID}
	}
	side := &pageCallSide{Program: componentTitle(section, builder.sections)}
	var groupOf map[string]string
	if index := builder.graphIndex(targetID); index != nil {
		groupOf = builder.edgesBetweenGroups(*index).groupOf
	}
	for _, id := range path {
		ref, known := builder.subject(targetID, id)
		if !known {
			return nil
		}
		name, anchor := builder.subjectDisplay(ref.subject)
		if name == "" {
			return nil
		}
		step := pageSideStep{Name: name, Key: declarationKey(anchor)}
		if group := groupOf[id]; group != "" {
			step.Part = targetMapNodeID(targetID, mapNodeID(group))
		}
		side.Path = append(side.Path, step)
	}
	return side
}
