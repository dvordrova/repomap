package report

import (
	"cmp"
	"slices"
	"strings"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// The reader's question "who changes X, and who reads it?" (owner,
// 2026-09-29) is answered from the program's reads and writes of record
// fields (ProgramIndex relations with a FieldPath, C; a typed receiver's
// field, Python; a declared property, JS/TS): a record type's reading lists
// its fields, a global variable's the fields the code reaches through it
// (server.masterhost), each with the functions writing and reading it by
// part, and a function's reading the fields it writes. No line numbers: the
// names link to their declarations.

// pageReadingFieldUse is one field of a type, or one field as the code
// reaches it through a global variable, with the functions that write it
// and read it, each list by the part they stand in.
type pageReadingFieldUse struct {
	Name    string             `json:"name"`
	Written []pageReadingNames `json:"written,omitempty"`
	Read    []pageReadingNames `json:"read,omitempty"`
}

// pageReadingNames is one part's declarations, by position in the reading's
// declarations and by name.
type pageReadingNames struct {
	Part  string `json:"part,omitempty"`
	Title string `json:"title,omitempty"`
	Decls []int  `json:"decls"`
}

// pageReadingWrite is one field a function writes, as the code reaches it,
// and the type declaring the field, whose reading lists its writers.
type pageReadingWrite struct {
	Path string `json:"path"`
	Decl *int   `json:"decl,omitempty"`
}

// pageFieldAccess is one exact read or write of a field by a declaration:
// the field's subject, its words (the path the code reaches it by, or
// Type.field when the adapter records no path) and its site.
type pageFieldAccess struct {
	from, field, label string
	writes             bool
	path               string
	line, column       int
}

// pageFieldFacts are one program's field accesses by field and by the
// declaration making them, in site order, and the variables each
// declaration reads whole (the root of a path through a global variable).
type pageFieldFacts struct {
	byField, byFrom map[string][]pageFieldAccess
	readsWhole      map[string]map[string]bool
}

// fieldFacts builds a program's field accesses once.
func (builder *pageBuilder) fieldFacts(index *groupindex.Index) *pageFieldFacts {
	if cached := builder.fieldsByTarget[index.Target.ID]; cached != nil {
		return cached
	}
	if builder.fieldsByTarget == nil {
		builder.fieldsByTarget = map[string]*pageFieldFacts{}
	}
	facts := &pageFieldFacts{byField: map[string][]pageFieldAccess{}, byFrom: map[string][]pageFieldAccess{}, readsWhole: map[string]map[string]bool{}}
	builder.fieldsByTarget[index.Target.ID] = facts
	targetID := index.Target.ID
	for _, edge := range index.StructuralEdges {
		if edge.Role != groupindex.EdgeRelationTarget || edge.Resolution != programindex.ResolutionExact ||
			edge.RelationKind != programindex.RelationReads && edge.RelationKind != programindex.RelationWrites {
			continue
		}
		ref, known := builder.subject(targetID, edge.ToSubjectID)
		if !known || ref.subject.Object == nil || ref.subject.Object.Kind != programindex.ObjectVariable {
			continue
		}
		object := ref.subject.Object
		label := edge.FieldPath
		if label == "" {
			owner, known := builder.subject(targetID, object.OwnerID)
			if !known || owner.subject.Object == nil || owner.subject.Object.Kind != programindex.ObjectType {
				// A whole variable read: the root of the paths through it.
				if edge.RelationKind == programindex.RelationReads {
					if facts.readsWhole[edge.FromSubjectID] == nil {
						facts.readsWhole[edge.FromSubjectID] = map[string]bool{}
					}
					facts.readsWhole[edge.FromSubjectID][edge.ToSubjectID] = true
				}
				continue
			}
			label = owner.subject.Object.Name + "." + object.Name
		}
		access := pageFieldAccess{from: edge.FromSubjectID, field: edge.ToSubjectID, label: label, writes: edge.RelationKind == programindex.RelationWrites}
		if edge.Location != nil {
			access.path, access.line, access.column = edge.Location.Path, edge.Location.Line, edge.Location.Column
		}
		facts.byField[access.field] = append(facts.byField[access.field], access)
		facts.byFrom[access.from] = append(facts.byFrom[access.from], access)
	}
	for _, accesses := range facts.byFrom {
		slices.SortStableFunc(accesses, func(a, b pageFieldAccess) int {
			return cmp.Or(cmp.Compare(a.line, b.line), cmp.Compare(a.column, b.column))
		})
	}
	return facts
}

// fieldReadings adds to a part's reading the field uses of its types and
// global variables and the writes of its functions, whose field writes then
// leave the variables it uses. declare names a
// function by its subject (-1 when it cannot be read), partOf the part it
// stands in, and ownerOf the reading of a member.
func (builder *pageBuilder) fieldReadings(index *groupindex.Index, own string, reading *pageGroupReading, types, variables, functions map[string]int,
	fieldsOf map[string][]string, declare func(string) int, partOf func(string, string) (string, string), ownerOf func(int) *pageReadingOwner,
	byName func(a, b int) int) {
	facts := builder.fieldFacts(index)
	if len(facts.byFrom) == 0 {
		return
	}
	targetID := index.Target.ID
	// names groups one side's functions by the part they stand in, own part
	// first, then the part naming most, each by name and each once.
	names := func(accesses []pageFieldAccess, writes bool) []pageReadingNames {
		var groups []pageReadingNames
		for _, access := range accesses {
			if access.writes != writes {
				continue
			}
			if ref, known := builder.subject(targetID, access.from); !known || ref.subject.Object == nil ||
				ref.subject.Object.Kind != programindex.ObjectFunction && ref.subject.Object.Kind != programindex.ObjectMethod {
				continue
			}
			position := declare(access.from)
			if position < 0 {
				continue
			}
			part, title := partOf(targetID, access.from)
			at := slices.IndexFunc(groups, func(group pageReadingNames) bool { return group.Part == part })
			if at < 0 {
				groups = append(groups, pageReadingNames{Part: part, Title: title})
				at = len(groups) - 1
			}
			if !slices.Contains(groups[at].Decls, position) {
				groups[at].Decls = append(groups[at].Decls, position)
			}
		}
		for i := range groups {
			slices.SortFunc(groups[i].Decls, byName)
		}
		slices.SortFunc(groups, func(a, b pageReadingNames) int {
			return cmp.Or(boolFirst(a.Part == own, b.Part == own), cmp.Compare(len(b.Decls), len(a.Decls)), strings.Compare(a.Title, b.Title), strings.Compare(a.Part, b.Part))
		})
		return groups
	}
	use := func(name string, accesses []pageFieldAccess) (pageReadingFieldUse, bool) {
		row := pageReadingFieldUse{Name: name, Written: names(accesses, true), Read: names(accesses, false)}
		return row, len(row.Written)+len(row.Read) > 0
	}
	line := func(id string) (int, int) {
		ref, known := builder.subject(targetID, id)
		if !known || ref.subject.Object == nil || ref.subject.Object.Location == nil {
			return 0, 0
		}
		return ref.subject.Object.Location.Line, ref.subject.Object.Location.Column
	}
	byLine := func(a, b string) int {
		aLine, aColumn := line(a)
		bLine, bColumn := line(b)
		return cmp.Or(cmp.Compare(aLine, bLine), cmp.Compare(aColumn, bColumn), strings.Compare(a, b))
	}
	// A record type: each of its fields in source order, by every path.
	for _, id := range keysInOrder(types) {
		fields := slices.Clone(fieldsOf[id])
		slices.SortFunc(fields, byLine)
		var rows []pageReadingFieldUse
		for _, field := range fields {
			ref, _ := builder.subject(targetID, field)
			if row, used := use(ref.subject.Object.Name, facts.byField[field]); used {
				rows = append(rows, row)
			}
		}
		if len(rows) > 0 {
			ownerOf(types[id]).Fields = rows
		}
	}
	// A global variable: each field as the code reaches it through the
	// variable (a path whose root is its name, written by a function that
	// reads the variable itself), in the source order of the path's first
	// field.
	for _, id := range keysInOrder(variables) {
		ref, known := builder.subject(targetID, id)
		if !known || ref.subject.Object == nil {
			continue
		}
		prefix := ref.subject.Object.Name + "."
		byPath := map[string][]pageFieldAccess{}
		first := map[string]string{}
		var paths []string
		for from, accesses := range facts.byFrom {
			if !facts.readsWhole[from][id] {
				continue
			}
			for _, access := range accesses {
				if !strings.HasPrefix(access.label, prefix) {
					continue
				}
				if _, seen := byPath[access.label]; !seen {
					paths = append(paths, access.label)
				}
				byPath[access.label] = append(byPath[access.label], access)
				if strings.Count(access.label, ".") == 1 {
					first[access.label] = access.field
				}
			}
		}
		// A path's order is its first field's place: server.db.expires
		// stands with server.db when the code reaches server.db itself.
		order := func(path string) string {
			segments := strings.SplitN(path, ".", 3)
			if field := first[segments[0]+"."+segments[1]]; field != "" {
				return field
			}
			return byPath[path][0].field
		}
		slices.SortFunc(paths, func(a, b string) int { return cmp.Or(byLine(order(a), order(b)), strings.Compare(a, b)) })
		var rows []pageReadingFieldUse
		for _, path := range paths {
			accesses := byPath[path]
			slices.SortStableFunc(accesses, func(a, b pageFieldAccess) int { return strings.Compare(a.from, b.from) })
			if row, used := use(path, accesses); used {
				rows = append(rows, row)
			}
		}
		if len(rows) > 0 {
			ownerOf(variables[id]).Fields = rows
		}
	}
	// A function: the fields it writes, each once, in the order the code
	// first writes them, each naming the type that declares the field.
	for _, id := range keysInOrder(functions) {
		var writes []pageReadingWrite
		for _, access := range facts.byFrom[id] {
			if !access.writes || slices.ContainsFunc(writes, func(write pageReadingWrite) bool { return write.Path == access.label }) {
				continue
			}
			write := pageReadingWrite{Path: access.label}
			if field, known := builder.subject(targetID, access.field); known && field.subject.Object != nil {
				if at := declare(field.subject.Object.OwnerID); at >= 0 {
					write.Decl = &at
				}
			}
			writes = append(writes, write)
		}
		if len(writes) > 0 {
			owner := ownerOf(functions[id])
			owner.Writes = writes
			// The fields it writes are said once, on this line, not again
			// among the variables it uses.
			owner.Uses = slices.DeleteFunc(owner.Uses, func(end pageReadingEnd) bool {
				return end.Kind == string(programindex.RelationWrites) && reading.Decls[end.Decl].Kind == "field"
			})
		}
	}
}

// keysInOrder are a map's keys in order, so what a loop declares takes the
// same positions on every render.
func keysInOrder[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
