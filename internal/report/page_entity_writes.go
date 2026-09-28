package report

import (
	"encoding/json"
	"sort"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// A write belongs to an entity only through the native field's exact owner.
// Its callable must be in this input's reach; shared group/type membership
// is not execution evidence. Callers are the reach's calls into the
// writer (none when the handler writes itself); no route to it is chosen.
// Source and possible dispatch survive both joins.
type pageEntityWrite struct {
	Entity     pageAnchor `json:"entity"`
	EntityName string     `json:"entity_name"`
	Field      string     `json:"field"`
	Source     pageAnchor `json:"source"`
	Possible   bool       `json:"possible"`
	// Integration marks a write of a matched input on another component:
	// an endpoint match, not a native call.
	Integration bool           `json:"integration,omitempty"`
	Callers     []pageCallStep `json:"callers,omitempty"`
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

func (builder *pageBuilder) operationWrites(index *groupindex.Index, reach groupindex.Reach) []pageEntityWrite {
	reached := make(map[string]bool, len(reach.Subjects))
	for _, subject := range reach.Subjects {
		reached[subject.SubjectID] = true
	}
	var result []pageEntityWrite
	for _, edge := range index.StructuralEdges {
		if edge.Role != groupindex.EdgeRelationTarget || edge.RelationKind != programindex.RelationWrites ||
			!reached[edge.FromSubjectID] || edge.Location == nil {
			continue
		}
		fieldRef, known := builder.subject(index.Target.ID, edge.ToSubjectID)
		if !known {
			continue
		}
		field := fieldRef.subject.Object
		if field == nil || field.Kind != programindex.ObjectVariable || field.OwnerID == "" {
			continue
		}
		entityRef, known := builder.subject(index.Target.ID, field.OwnerID)
		if !known {
			continue
		}
		entity := entityRef.subject
		if entity.Object == nil || entity.Object.Kind != programindex.ObjectType {
			continue
		}
		name, anchor := builder.subjectDisplay(entity)
		if anchor == nil {
			continue
		}
		// The write is possible when it is, or when no exact call of the
		// reach enters its writer.
		possible := edge.Resolution != programindex.ResolutionExact
		var callers []pageCallStep
		exact := len(reach.Subjects) > 0 && reach.Subjects[0].SubjectID == edge.FromSubjectID
		for _, position := range reach.Edges {
			call := index.StructuralEdges[position]
			if call.ToSubjectID != edge.FromSubjectID || call.RelationKind == programindex.RelationReads {
				continue
			}
			ref, known := builder.subject(index.Target.ID, call.FromSubjectID)
			if !known {
				continue
			}
			callerName, callerAnchor := builder.subjectDisplay(ref.subject)
			step := pageCallStep{Name: callerName, Possible: call.Resolution != programindex.ResolutionExact}
			if call.Location != nil {
				site := builder.links.anchor(call.Location.Path, call.Location.Line, call.Location.Column)
				step.Href, step.Open, step.Source, step.NoSource = site.Href, site.Open, site.Text, site.NoSource
			} else if callerAnchor != nil {
				step.Href, step.Open, step.Source, step.NoSource = callerAnchor.Href, callerAnchor.Open, callerAnchor.Text, callerAnchor.NoSource
			}
			exact = exact || !step.Possible
			callers = append(callers, step)
		}
		possible = possible || !exact
		result = append(result, pageEntityWrite{Entity: *anchor, EntityName: name, Field: field.Name,
			Source: builder.links.anchor(edge.Location.Path, edge.Location.Line, edge.Location.Column), Possible: possible, Callers: callers})
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
