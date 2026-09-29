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
// fields (ProgramIndex relations with a FieldPath, C and Go; a typed
// receiver's field, Python; a declared property, JS/TS): a record type's reading lists
// its fields, a global variable's the fields the code reaches through it
// (server.masterhost), each with the functions writing and reading it by
// part, and a function's reading the fields it writes and the fields and
// global variables it reads. No line numbers: the names link to their
// declarations.

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

// pageReadingPath is one field or global variable a declaration writes or
// reads, in the words the code reaches it by (server.dirty,
// redisClient.argv, shared), and the declaration its name reads: the type
// declaring the field, whose reading lists its writers and readers, or the
// variable itself.
type pageReadingPath struct {
	Path string `json:"path"`
	Decl *int   `json:"decl,omitempty"`
}

// pageFieldAccess is one exact read or write of a field, or one exact read
// of a whole module variable, by a declaration: the field's or the
// variable's subject, its words (the path the code reaches the field by,
// Type.field when the adapter records no path, or the variable's name) and
// its site.
type pageFieldAccess struct {
	from, field, label string
	writes             bool
	path               string
	line, column       int
}

// pageFieldFacts are one program's field accesses by field and by the
// declaration making them, in site order; the variables each declaration
// reads whole (the root of a path through a global variable); and each
// declaration's reads of a whole module variable, in site order.
type pageFieldFacts struct {
	byField, byFrom map[string][]pageFieldAccess
	readsWhole      map[string]map[string]bool
	wholeByFrom     map[string][]pageFieldAccess
}

// fieldFacts builds a program's field accesses once.
func (builder *pageBuilder) fieldFacts(index *groupindex.Index) *pageFieldFacts {
	if cached := builder.fieldsByTarget[index.Target.ID]; cached != nil {
		return cached
	}
	if builder.fieldsByTarget == nil {
		builder.fieldsByTarget = map[string]*pageFieldFacts{}
	}
	facts := &pageFieldFacts{byField: map[string][]pageFieldAccess{}, byFrom: map[string][]pageFieldAccess{}, readsWhole: map[string]map[string]bool{},
		wholeByFrom: map[string][]pageFieldAccess{}}
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
				// A whole variable read: the root of the paths through it,
				// and, of a module's variable, a read the reader's Reads
				// line says (a local or a parameter is never listed).
				if edge.RelationKind == programindex.RelationReads {
					if facts.readsWhole[edge.FromSubjectID] == nil {
						facts.readsWhole[edge.FromSubjectID] = map[string]bool{}
					}
					facts.readsWhole[edge.FromSubjectID][edge.ToSubjectID] = true
					if builder.moduleVariable(targetID, object) {
						whole := pageFieldAccess{from: edge.FromSubjectID, field: edge.ToSubjectID, label: object.Name}
						if edge.Location != nil {
							whole.path, whole.line, whole.column = edge.Location.Path, edge.Location.Line, edge.Location.Column
						}
						facts.wholeByFrom[whole.from] = append(facts.wholeByFrom[whole.from], whole)
					}
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
		slices.SortStableFunc(accesses, bySite)
	}
	for _, accesses := range facts.wholeByFrom {
		slices.SortStableFunc(accesses, bySite)
	}
	return facts
}

// bySite orders one declaration's accesses as they are written.
func bySite(a, b pageFieldAccess) int {
	return cmp.Or(cmp.Compare(a.line, b.line), cmp.Compare(a.column, b.column))
}

// fieldReadings adds to a part's reading the field uses of its types and
// global variables, the writes of its functions and the reads of its
// declarations. declare names a declaration by its subject (-1 when it
// cannot be read), partOf the part it stands in, and ownerOf the reading of
// a member.
func (builder *pageBuilder) fieldReadings(index *groupindex.Index, own string, reading *pageGroupReading, types, variables, functions map[string]int,
	fieldsOf map[string][]string, declare func(string) int, partOf func(string, string) (string, string), ownerOf func(int) *pageReadingOwner,
	byName func(a, b int) int) {
	facts := builder.fieldFacts(index)
	if len(facts.byFrom) == 0 && len(facts.wholeByFrom) == 0 {
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
	// The declaration a path's name reads: the type declaring the field,
	// or the variable itself.
	holder := func(access pageFieldAccess, whole bool) *int {
		id := access.field
		if !whole {
			field, known := builder.subject(targetID, access.field)
			if !known || field.subject.Object == nil {
				return nil
			}
			id = field.subject.Object.OwnerID
		}
		if at := declare(id); at >= 0 {
			return &at
		}
		return nil
	}
	// A function: the fields it writes, each once, in the order the code
	// first writes them.
	for _, id := range keysInOrder(functions) {
		var writes []pageReadingPath
		for _, access := range facts.byFrom[id] {
			if !access.writes || slices.ContainsFunc(writes, func(write pageReadingPath) bool { return write.Path == access.label }) {
				continue
			}
			writes = append(writes, pageReadingPath{Path: access.label, Decl: holder(access, false)})
		}
		if len(writes) > 0 {
			ownerOf(functions[id]).Writes = writes
		}
	}
	// A declaration: the fields and module variables it reads, each once,
	// in the order the code first uses them (owner, 2026-09-29: "Uses
	// variables" had printed c->argv as argv with every line using it). A
	// field it also writes stays on its Writes line, and a name leading to a
	// longer path it reads or writes is said by that path (server by
	// server.dirty, server.db by server.db.dict.size).
	for _, members := range []map[string]int{functions, types, variables} {
		for _, id := range keysInOrder(members) {
			type use struct {
				access pageFieldAccess
				whole  bool
			}
			var uses []use
			for _, access := range facts.byFrom[id] {
				uses = append(uses, use{access: access})
			}
			for _, access := range facts.wholeByFrom[id] {
				uses = append(uses, use{access: access, whole: true})
			}
			slices.SortStableFunc(uses, func(a, b use) int { return bySite(a.access, b.access) })
			var writes []string
			if found := slices.IndexFunc(reading.Own, func(owner pageReadingOwner) bool { return owner.Decl == members[id] }); found >= 0 {
				for _, write := range reading.Own[found].Writes {
					writes = append(writes, write.Path)
				}
			}
			paths := map[string]bool{}
			for _, write := range writes {
				paths[write] = true
			}
			for _, use := range uses {
				paths[use.access.label] = true
			}
			leads := func(label string) bool {
				for path := range paths {
					if strings.HasPrefix(path, label+".") {
						return true
					}
				}
				return false
			}
			var reads []pageReadingPath
			for _, use := range uses {
				access := use.access
				if access.writes || slices.Contains(writes, access.label) || leads(access.label) ||
					slices.ContainsFunc(reads, func(read pageReadingPath) bool { return read.Path == access.label }) {
					continue
				}
				reads = append(reads, pageReadingPath{Path: access.label, Decl: holder(access, use.whole)})
			}
			if len(reads) > 0 {
				ownerOf(members[id]).Reads = reads
			}
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
