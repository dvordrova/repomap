package report

import (
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// dispatchFold is one set of declarations a dispatch site can call: a
// relation whose adapter retained several alternatives calls one of them
// (Redis's call reaches `c->cmd->proc`, one of 94 command functions). Every
// site with the same set shares the fold, and a declaration that hands
// every member of it over by another relation (cmdTable passes all 94 as
// callbacks) folds with it too. A card lists such rows as one line: they
// are one fact about one set, not ninety-four calls to read.
type dispatchFold struct {
	id      string
	members map[string]bool
	// kinds are the relation kinds of the dispatch sites themselves.
	kinds map[string]bool
	// through is the declaration at the first dispatch site, by source
	// order: "the same 94 as call".
	through string
}

// dispatchFacts are one target's folds: by dispatch relation, and by the
// (caller, relation kind, callee) row of a declaration handing a whole set over.
type dispatchFacts struct {
	site   map[string]*dispatchFold
	handed map[[3]string]*dispatchFold
}

type dispatchRelation struct {
	id, from, kind string
	location       *programindex.Location
	targets        []string
	alternatives   bool
}

// dispatchSites is every relation of a target with its retained targets,
// in source order.
func dispatchRelations(index *groupindex.Index) []dispatchRelation {
	byID := map[string]*dispatchRelation{}
	var order []string
	for _, edge := range index.StructuralEdges {
		if edge.Role != groupindex.EdgeRelationTarget || edge.RelationID == "" {
			continue
		}
		relation := byID[edge.RelationID]
		if relation == nil {
			relation = &dispatchRelation{id: edge.RelationID, from: edge.FromSubjectID, kind: string(edge.RelationKind), location: edge.Location,
				alternatives: edge.Resolution == programindex.ResolutionAlternatives}
			byID[edge.RelationID] = relation
			order = append(order, edge.RelationID)
		}
		if !slices.Contains(relation.targets, edge.ToSubjectID) {
			relation.targets = append(relation.targets, edge.ToSubjectID)
		}
	}
	relations := make([]dispatchRelation, 0, len(order))
	for _, id := range order {
		relations = append(relations, *byID[id])
	}
	sort.SliceStable(relations, func(i, j int) bool {
		return locationBeforeInFile(relations[i].location, relations[j].location)
	})
	return relations
}

func locationBeforeInFile(left, right *programindex.Location) bool {
	switch {
	case left == nil || right == nil:
		return left != nil && right == nil
	case left.Path != right.Path:
		return left.Path < right.Path
	case left.Line != right.Line:
		return left.Line < right.Line
	default:
		return left.Column < right.Column
	}
}

// dispatch is the folds of a target, built once.
func (builder *pageBuilder) dispatch(targetID string) *dispatchFacts {
	if facts, built := builder.dispatchByTarget[targetID]; built {
		return facts
	}
	if builder.dispatchByTarget == nil {
		builder.dispatchByTarget = map[string]*dispatchFacts{}
	}
	facts := &dispatchFacts{site: map[string]*dispatchFold{}, handed: map[[3]string]*dispatchFold{}}
	builder.dispatchByTarget[targetID] = facts
	index := builder.graphIndex(targetID)
	if index == nil {
		return facts
	}
	relations := dispatchRelations(index)
	bySet := map[string]*dispatchFold{}
	var folds []*dispatchFold
	for _, relation := range relations {
		if !relation.alternatives || len(relation.targets) < 2 {
			continue
		}
		members := slices.Clone(relation.targets)
		sort.Strings(members)
		key := strings.Join(members, "\x00")
		fold := bySet[key]
		if fold == nil {
			fold = &dispatchFold{id: targetID + "/f" + strconv.Itoa(len(folds)+1), members: map[string]bool{}, kinds: map[string]bool{}}
			for _, member := range members {
				fold.members[member] = true
			}
			if ref, known := builder.subject(targetID, relation.from); known {
				fold.through, _ = builder.subjectDisplay(ref.subject)
			}
			bySet[key] = fold
			folds = append(folds, fold)
		}
		fold.kinds[relation.kind] = true
		facts.site[relation.id] = fold
	}
	if len(folds) == 0 {
		return facts
	}
	// What each declaration hands over by each relation kind.
	handed := map[[2]string]map[string]bool{}
	for _, relation := range relations {
		key := [2]string{relation.from, relation.kind}
		if handed[key] == nil {
			handed[key] = map[string]bool{}
		}
		for _, target := range relation.targets {
			handed[key][target] = true
		}
	}
	keys := make([][2]string, 0, len(handed))
	for key := range handed {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i][0]+"\x00"+keys[i][1] < keys[j][0]+"\x00"+keys[j][1] })
	for _, fold := range folds {
		for _, key := range keys {
			targets := handed[key]
			// Another kind of relation only: a caller calling every member
			// itself is its own rows, not the dispatch said again.
			if fold.kinds[key[1]] || len(targets) < len(fold.members) {
				continue
			}
			covers := true
			for member := range fold.members {
				if !targets[member] {
					covers = false
					break
				}
			}
			if !covers {
				continue
			}
			for member := range fold.members {
				row := [3]string{key[0], key[1], member}
				if facts.handed[row] == nil {
					facts.handed[row] = fold
				}
			}
		}
	}
	return facts
}

// foldCall marks a card's call with the dispatch fold it belongs to, if any.
func (builder *pageBuilder) foldCall(call *pageEdgeCall, connection groupindex.Connection) {
	if call == nil || !strings.HasPrefix(connection.SourceKind, "native_") || connection.From.TargetID != connection.To.TargetID {
		return
	}
	facts := builder.dispatch(connection.From.TargetID)
	if fold := facts.site[connection.SourceID]; fold != nil && fold.members[connection.ToSubjectID] {
		call.Fold, call.Of, call.One = fold.id, len(fold.members), true
		return
	}
	kind := strings.TrimPrefix(connection.SourceKind, "native_")
	if fold := facts.handed[[3]string{connection.FromSubjectID, kind, connection.ToSubjectID}]; fold != nil {
		call.Fold, call.Of, call.Same = fold.id, len(fold.members), fold.through
	}
}
