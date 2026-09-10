package report

import (
	"sort"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

type pageDataCatalog struct {
	Rows            []pageDataRow
	Tables, Queries int
}
type pageDataRow struct {
	ID, Name, Origin, Scope, Connection, SQL, Expression, Statement string
	Partial                                                         bool
	Anchor                                                          pageAnchor
	Columns                                                         []pageDataColumn
	References                                                      []pageDataReference
	Operations                                                      []pageDataOperation
}
type pageDataColumn struct {
	Name, Type, ForeignKey string
	PrimaryKey             bool
	Anchor                 pageAnchor
}
type pageDataReference struct {
	Name, Href string
	Source     *pageAnchor
}
type pageDataOperation struct {
	Name, Href, Via string
	Anchor          pageAnchor
	Possible        bool
}

func (builder *pageBuilder) fillSectionData(section *pageSection) {
	index := builder.graphIndex(section.programTargetID)
	if index == nil {
		return
	}
	refs := map[string]pageDataReference{}
	for _, original := range builder.sourceIndexes {
		if original.Target.ID != index.Target.ID {
			continue
		}
		for _, record := range original.Data {
			if record.Data != nil {
				refs[record.ID] = pageDataReference{Name: record.Data.Name, Source: builder.links.anchorPointer(record.Path, record.Line, 0)}
			}
		}
	}
	for _, record := range index.Data {
		refs[record.ID] = pageDataReference{Name: record.Data.Name, Href: "#" + section.ID + "-data-" + record.ID}
	}
	operations := dataOperationLinks(index)
	for _, record := range index.Data {
		data := record.Data
		if data == nil {
			continue
		}
		row := pageDataRow{ID: section.ID + "-data-" + record.ID, Name: data.Name, Scope: data.Scope, Connection: data.Connection, SQL: data.SQL, Expression: data.Expression, Statement: data.Statement, Partial: data.Partial, Anchor: builder.links.anchor(record.Path, record.Line, 0)}
		if data.Schema != "" {
			row.Name = data.Schema + "." + data.Name
		}
		if data.Kind == "query" {
			section.Data.Queries++
			row.Origin = "SQL text"
		} else {
			section.Data.Tables++
			switch data.Origin {
			case "orm":
				row.Origin = "Declared in model"
			case "ddl":
				row.Origin = "Declared in SQL"
			default:
				row.Origin = "Mentioned in SQL"
			}
		}
		for _, column := range data.Columns {
			row.Columns = append(row.Columns, pageDataColumn{Name: column.Name, Type: column.Type, ForeignKey: column.ForeignKey, PrimaryKey: column.PrimaryKey, Anchor: builder.links.anchor(column.Anchor.Path, column.Anchor.Line, column.Anchor.Column)})
		}
		for _, ref := range record.References {
			if linked, ok := refs[ref]; ok {
				row.References = append(row.References, linked)
			}
		}
		for _, link := range operations[record.OwnerSubjectID] {
			row.Operations = append(row.Operations, pageDataOperation{Name: builder.operationDisplayName(link.operation), Href: "#" + operationNodeID(section.ID, link.operation.ID), Via: link.subject.Object.Name, Possible: link.possible, Anchor: builder.links.anchor(link.subject.Object.Location.Path, link.subject.Object.Location.Line, link.subject.Object.Location.Column)})
		}
		section.Data.Rows = append(section.Data.Rows, row)
	}
	sort.SliceStable(section.Data.Rows, func(i, j int) bool {
		a, b := section.Data.Rows[i], section.Data.Rows[j]
		if (a.SQL == "") != (b.SQL == "") {
			return a.SQL == ""
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.ID < b.ID
	})
}

type dataOperationLink struct {
	operation groupindex.Operation
	subject   groupindex.Subject
	possible  bool
}

// These are source call paths into declarations owned by a model. They are
// explicitly code associations, not an assertion that every method does I/O.
func dataOperationLinks(index *groupindex.Index) map[string][]dataOperationLink {
	result := map[string][]dataOperationLink{}
	owners := map[string]bool{}
	for _, row := range index.Data {
		if row.OwnerSubjectID != "" {
			owners[row.OwnerSubjectID] = true
		}
	}
	if len(owners) == 0 {
		return result
	}
	subjects := map[string]groupindex.Subject{}
	for _, subject := range index.Subjects {
		subjects[subject.ID] = subject
	}
	calls := map[string][]groupindex.StructuralEdge{}
	for _, edge := range index.StructuralEdges {
		if edge.Role == groupindex.EdgeRelationTarget && edge.RelationKind == programindex.RelationCalls {
			calls[edge.FromSubjectID] = append(calls[edge.FromSubjectID], edge)
		}
	}
	for _, operation := range index.Operations {
		type step struct {
			id       string
			possible bool
		}
		queue := []step{{id: operation.SubjectID}}
		seen := map[string]bool{}
		matched := map[string]bool{}
		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			if current.id == "" || seen[current.id] {
				continue
			}
			seen[current.id] = true
			subject := subjects[current.id]
			if subject.Object != nil && subject.Object.Location != nil && owners[subject.Object.OwnerID] && !matched[subject.Object.OwnerID] {
				owner := subject.Object.OwnerID
				matched[owner] = true
				result[owner] = append(result[owner], dataOperationLink{operation: operation, subject: subject, possible: current.possible})
			}
			for _, edge := range calls[current.id] {
				queue = append(queue, step{id: edge.ToSubjectID, possible: current.possible || edge.Resolution != programindex.ResolutionExact})
			}
		}
	}
	return result
}
