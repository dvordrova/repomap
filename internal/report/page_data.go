package report

import (
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

type pageDataCatalog struct {
	Rows            []pageDataRow
	Tables, Queries int
}

// TableRows and QueryRows split the catalog for display: declared and
// mentioned tables first, SQL texts after, each with its own disclosure.
func (catalog pageDataCatalog) TableRows() []pageDataRow {
	var rows []pageDataRow
	for _, row := range catalog.Rows {
		if row.Origin != "SQL text" {
			rows = append(rows, row)
		}
	}
	return rows
}

func (catalog pageDataCatalog) QueryRows() []pageDataRow {
	var rows []pageDataRow
	for _, row := range catalog.Rows {
		if row.Origin == "SQL text" {
			rows = append(rows, row)
		}
	}
	return rows
}

type pageDataRow struct {
	ID, Name, Origin, Scope, Connection, SQL, Expression, Statement string
	Partial                                                         bool
	Anchor                                                          pageAnchor
	Columns                                                         []pageDataColumn
	References                                                      []pageDataReference
	Queries                                                         []pageDataReference
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
	Name, Href, Via, ViaHref string
	Anchor                   pageAnchor
	Possible                 bool
}

// catalogTable reports a database's own catalog relation (pg_type,
// information_schema.columns): SQL that reads it describes the database
// engine, not this repository's data.
func catalogTable(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasPrefix(lower, "pg_") || strings.HasPrefix(lower, "information_schema.") || lower == "information_schema" || strings.HasPrefix(lower, "sqlite_")
}

// dataRowID is the page-level id of one data record. Record ids carry a
// kind prefix such as "entity:" or "query:"; a colon inside a fragment href
// reads as a URL scheme to html/template, which then replaces the link with
// #ZgotmplZ, so the page id keeps a hyphen there instead.
func dataRowID(sectionID, recordID string) string {
	return sectionID + "-data-" + strings.ReplaceAll(recordID, ":", "-")
}

func (builder *pageBuilder) fillSectionData(section *pageSection) {
	index := builder.graphIndex(section.programTargetID)
	if index == nil {
		return
	}
	refs := map[string]pageDataReference{}
	queries := map[string][]pageDataReference{}
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
		if record.Data != nil {
			name := record.Data.Name
			if record.Data.Schema != "" {
				name = record.Data.Schema + "." + name
			}
			refs[record.ID] = pageDataReference{Name: name, Href: "#" + dataRowID(section.ID, record.ID)}
		}
	}
	for _, record := range index.Data {
		if record.Data != nil && record.Data.Kind == "query" {
			for _, target := range record.References {
				queries[target] = append(queries[target], refs[record.ID])
			}
		}
	}
	operations := dataOperationLinks(index)
	for _, record := range index.Data {
		data := record.Data
		if data == nil || vendoredPath(record.Path) || catalogTable(data.Name) {
			continue
		}
		row := pageDataRow{ID: dataRowID(section.ID, record.ID), Name: data.Name, Scope: data.Scope, Connection: data.Connection, SQL: data.SQL, Expression: data.Expression, Statement: data.Statement, Partial: data.Partial, Anchor: builder.links.anchor(record.Path, record.Line, 0)}
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
		row.Queries = queries[record.ID]
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
	// Reference identities already carry SQL scope. Never link tables by name.
	rows := map[string]*pageDataRow{}
	for i := range section.Data.Rows {
		rows[section.Data.Rows[i].ID] = &section.Data.Rows[i]
	}
	for _, record := range index.Data {
		if record.Data == nil || record.Data.Kind != "query" {
			continue
		}
		query := rows[dataRowID(section.ID, record.ID)]
		for _, ref := range record.References {
			table := rows[dataRowID(section.ID, ref)]
			if table == nil {
				continue
			}
			for _, operation := range query.Operations {
				operation.Via, operation.ViaHref, operation.Anchor = query.Name, "#"+query.ID, query.Anchor
				table.Operations = append(table.Operations, operation)
			}
		}
	}
	byOperation := map[string][]pageDataReference{}
	for _, row := range section.Data.Rows {
		for _, operation := range row.Operations {
			byOperation[operation.Href] = appendDataReference(byOperation[operation.Href], pageDataReference{Name: row.Name, Href: "#" + row.ID})
		}
	}
	for i := range section.Requests {
		section.Requests[i].Data = byOperation[section.Requests[i].Href]
	}
	for i := range section.Activities {
		section.Activities[i].Data = byOperation[section.Activities[i].Href]
	}
	for i := range section.RouteGroups {
		for j := range section.RouteGroups[i].Rows {
			for k := range section.RouteGroups[i].Rows[j].Paths {
				route := &section.RouteGroups[i].Rows[j].Paths[k]
				for _, href := range route.OperationHrefs {
					for _, ref := range byOperation[href] {
						route.Data = appendDataReference(route.Data, ref)
					}
				}
			}
		}
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

func appendDataReference(rows []pageDataReference, ref pageDataReference) []pageDataReference {
	for _, row := range rows {
		if row.Href == ref.Href {
			return rows
		}
	}
	return append(rows, ref)
}

// These are source call paths into model declarations or the exact callable
// containing a SQL occurrence, not an assertion that every method does I/O.
func dataOperationLinks(index *groupindex.Index) map[string][]dataOperationLink {
	result := map[string][]dataOperationLink{}
	owners := map[string]bool{}
	tables := map[string]bool{}
	for _, row := range index.Data {
		if row.OwnerSubjectID != "" {
			owners[row.OwnerSubjectID] = true
			if row.Data != nil && row.Data.Kind == "table" {
				tables[row.OwnerSubjectID] = true
			}
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
			if subject.Object != nil && subject.Object.Location != nil {
				var matches []string
				if owners[current.id] && (subject.Object.Kind == programindex.ObjectFunction || subject.Object.Kind == programindex.ObjectMethod || subject.Object.Kind == programindex.ObjectLambda) {
					matches = append(matches, current.id)
				}
				if tables[subject.Object.OwnerID] {
					matches = append(matches, subject.Object.OwnerID)
				}
				for _, owner := range matches {
					if !matched[owner] {
						matched[owner] = true
						result[owner] = append(result[owner], dataOperationLink{operation: operation, subject: subject, possible: current.possible})
					}
				}
			}
			for _, edge := range calls[current.id] {
				queue = append(queue, step{id: edge.ToSubjectID, possible: current.possible || edge.Resolution != programindex.ResolutionExact})
			}
		}
	}
	return result
}
