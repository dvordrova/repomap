package report

import (
	"encoding/json"
	"sort"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// A write belongs to an entity only through the native field's exact owner.
// Its callable must be reached by this input; shared group/type membership is
// not execution evidence. Source and possible dispatch survive both joins.
type pageEntityWrite struct {
	Entity     pageAnchor     `json:"entity"`
	EntityName string         `json:"entity_name"`
	Field      string         `json:"field"`
	Source     pageAnchor     `json:"source"`
	Possible   bool           `json:"possible"`
	Steps      []pageCallStep `json:"steps"`
}

func (step pageCallStep) Anchor() pageAnchor {
	return pageAnchor{Href: step.Href, Open: step.Open, Text: step.Source, NoSource: step.NoSource}
}

func (node pageMapNode) WritesJSON() string {
	if len(node.Writes) == 0 {
		return ""
	}
	raw, _ := json.Marshal(node.Writes)
	return string(raw)
}

func (builder *pageBuilder) operationWrites(index *groupindex.Index, root string, reached map[string]bool, parents map[string]groupindex.StructuralEdge) []pageEntityWrite {
	var result []pageEntityWrite
	for _, edge := range index.StructuralEdges {
		if edge.Role != groupindex.EdgeRelationTarget || edge.RelationKind != programindex.RelationWrites ||
			!reached[edge.FromSubjectID] || edge.Location == nil {
			continue
		}
		field := builder.subjects[edge.ToSubjectID].subject.Object
		if field == nil || field.Kind != programindex.ObjectVariable || field.OwnerID == "" {
			continue
		}
		entity := builder.subjects[field.OwnerID].subject
		if entity.Object == nil || entity.Object.Kind != programindex.ObjectType {
			continue
		}
		name, anchor := builder.subjectDisplay(entity)
		if anchor == nil {
			continue
		}
		steps := builder.callWitness(root, edge.FromSubjectID, parents)
		if len(steps) == 0 {
			continue
		}
		possible := edge.Resolution != programindex.ResolutionExact
		for _, step := range steps {
			possible = possible || step.Possible
		}
		result = append(result, pageEntityWrite{Entity: *anchor, EntityName: name, Field: field.Name,
			Source: builder.links.anchor(edge.Location.Path, edge.Location.Line, edge.Location.Column), Possible: possible, Steps: steps})
	}
	sort.SliceStable(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if a.Entity.Path != b.Entity.Path {
			return a.Entity.Path < b.Entity.Path
		}
		if a.Entity.Line != b.Entity.Line {
			return a.Entity.Line < b.Entity.Line
		}
		if a.Source.Path != b.Source.Path {
			return a.Source.Path < b.Source.Path
		}
		if a.Source.Line != b.Source.Line {
			return a.Source.Line < b.Source.Line
		}
		return a.Field < b.Field
	})
	return result
}
