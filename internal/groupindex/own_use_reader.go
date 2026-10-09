package groupindex

import (
	"iter"
	"strings"

	"github.com/dvordrova/repomap/internal/programindex"
)

// OwnUseReader indexes the original evidence for repeated OwnUses questions.
// It borrows one immutable complete ProgramIndex; the owner must release it
// when moving to another target. Results remain specific to the supplied ids.
type OwnUseReader struct {
	program   programindex.Index
	objects   map[string]int
	relations map[string][]int
}

func NewOwnUseReader(program programindex.Index) *OwnUseReader {
	reader := &OwnUseReader{program: program, objects: make(map[string]int, len(program.Objects)), relations: map[string][]int{}}
	for position, object := range program.Objects {
		reader.objects[object.ID] = position
	}
	for position, relation := range program.Relations {
		if relation.Location != nil && (relation.Kind == programindex.RelationReads || relation.Kind == programindex.RelationCalls || relation.Kind == programindex.RelationInvokesExternal) {
			reader.relations[relation.FromID] = append(reader.relations[relation.FromID], position)
		}
	}
	return reader
}

func (reader *OwnUseReader) object(id string) (programindex.Object, bool) {
	position, found := reader.objects[id]
	if !found {
		return programindex.Object{}, false
	}
	return reader.program.Objects[position], true
}

func (reader *OwnUseReader) OwnUses(ids []string) []OwnUse {
	relations := iter.Seq[programindex.Relation](func(yield func(programindex.Relation) bool) {
		seen := make(map[string]bool, len(ids))
		for _, id := range ids {
			if seen[id] {
				continue
			}
			seen[id] = true
			for _, position := range reader.relations[id] {
				if !yield(reader.program.Relations[position]) {
					return
				}
			}
		}
	})
	named := func(object programindex.Object) string {
		if owner, ok := reader.object(object.OwnerID); ok && object.Kind == programindex.ObjectMethod && owner.Kind == programindex.ObjectType && owner.Name != "" && !strings.Contains(object.Name, ".") {
			return strings.TrimPrefix(owner.Name, "*") + "." + object.Name
		}
		return object.Name
	}
	return ownUsesFrom(relations, ids, reader.object, named, writtenInline)
}
