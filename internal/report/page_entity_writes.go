package report

import (
	"cmp"
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// pageEntityWrite is one change an input makes to the program's data
// (critic, 2026-09-30: State changes had listed every field write of the
// whole reach, 80 for redis's `set`, listNode.next and dict.used among
// them). An input's work is its handler and what it calls exactly, never
// entering a helper: a declaration the helper question decided serves
// others' work standing in a part most of the program's parts call into,
// other than the handler's own (the flow's helper, page_flow.go). The
// program's data is what outlives one call: the fields reached through a
// file-scope variable (ProgramIndex field_path rooted at a variable:
// `server.dirty`), the fields of a type owning a database table (GroupsIndex
// data record owner: freqtrade's Trade), the database and the files. Its
// changes are, by Kind:
//
//   - "write": such a field the work writes, save a constructor setting up
//     the object its call makes (a call of a class and of its method at one
//     place: RPCException(...) and its __init__);
//
//   - "db": a database call anywhere in the input's reach, with the tables
//     its statement names (GroupsIndex outbound `db` and its data records);
//
//   - "file": a file anywhere in the reach reaches (GroupsIndex data record
//     of kind `file`, each call site's function).
//
//   - "call": a field the work hands to a helper whose own code writes that
//     field's type or a type one of its fields holds, said as those two facts
//     ("redisDb.dict — handed to dictAdd, dictReplace"), Via the helpers:
//     no fact says the helper writes through that parameter rather than a
//     local (ProgramIndex roots such a write at its type), so it is not said
//     to change. It stays for the frozen journey (redis `set` names its
//     keyspace, redisDb.dict).
//
// A helper's own writes are never listed. Each target is one change with
// every function making it
// (Callers), in the order the work first makes it, the database's and the
// files' after. Entity is the record type holding the field or owning the
// table; Destination names a database or a file; Source is where the change
// is first made.
type pageEntityWrite struct {
	Kind        string     `json:"kind"`
	Entity      pageAnchor `json:"entity"`
	EntityName  string     `json:"entity_name,omitempty"`
	Field       string     `json:"field,omitempty"`
	Via         []string   `json:"via,omitempty"`
	Destination string     `json:"destination,omitempty"`
	Tables      []string   `json:"tables,omitempty"`
	Source      pageAnchor `json:"source"`
	Possible    bool       `json:"possible,omitempty"`
	// Integration marks a change of a matched input on another component:
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

// pageDataFacts are one program's facts the changes read, built once: each
// function's field writes, the types owning a table, the file-scope
// variables' names, and the classes a function calls at a place (caller,
// path and line: a construction there).
type pageDataFacts struct {
	writes    map[string][]int
	tables    map[string]bool
	globals   map[string]bool
	makes     map[string][]string
	relations map[string]programindex.Relation
	fieldsOf  map[string][]string
	typesOf   map[string][]string
	reads     map[string][]int
}

func (builder *pageBuilder) dataFacts(index *groupindex.Index) *pageDataFacts {
	if builder.dataByTarget == nil {
		builder.dataByTarget = map[string]*pageDataFacts{}
	}
	targetID := index.Target.ID
	if cached := builder.dataByTarget[targetID]; cached != nil {
		return cached
	}
	facts := &pageDataFacts{writes: map[string][]int{}, tables: map[string]bool{}, globals: map[string]bool{}, makes: map[string][]string{},
		relations: map[string]programindex.Relation{}, fieldsOf: map[string][]string{}, typesOf: map[string][]string{}, reads: map[string][]int{}}
	for _, record := range index.Data {
		if record.OwnerSubjectID != "" && record.Data != nil && record.Data.Kind == "table" {
			facts.tables[record.OwnerSubjectID] = true
		}
	}
	if builder.data != nil && builder.data.ProgramPortfolio != nil {
		for _, entry := range builder.data.ProgramPortfolio.Entries {
			if entry.Target.ID != targetID {
				continue
			}
			kinds := make(map[string]programindex.ObjectKind, len(entry.Objects))
			for _, object := range entry.Objects {
				kinds[object.ID] = object.Kind
			}
			for _, relation := range entry.Relations {
				if relation.Kind == programindex.RelationCalls || relation.Kind == programindex.RelationExecutes {
					facts.relations[relation.ID] = relation
				}
			}
			for _, object := range entry.Objects {
				if object.Kind != programindex.ObjectVariable {
					continue
				}
				if owner := kinds[object.OwnerID]; owner == programindex.ObjectModule || owner == programindex.ObjectPackage {
					facts.globals[object.Name] = true
				}
				if kinds[object.OwnerID] != programindex.ObjectType {
					continue
				}
				facts.fieldsOf[object.OwnerID] = append(facts.fieldsOf[object.OwnerID], object.ID)
				for _, at := range object.Types {
					if typeID := builder.subjectAt[subjectLocationKey(targetID, at.Path, at.Line)]; typeID != "" && !slices.Contains(facts.typesOf[object.ID], typeID) {
						facts.typesOf[object.ID] = append(facts.typesOf[object.ID], typeID)
					}
				}
			}
		}
	}
	for position, edge := range index.StructuralEdges {
		if edge.Role != groupindex.EdgeRelationTarget || edge.Location == nil {
			continue
		}
		switch edge.RelationKind {
		case programindex.RelationWrites:
			facts.writes[edge.FromSubjectID] = append(facts.writes[edge.FromSubjectID], position)
		case programindex.RelationReads:
			facts.reads[edge.FromSubjectID] = append(facts.reads[edge.FromSubjectID], position)
		case programindex.RelationCalls:
			if ref, known := builder.subject(targetID, edge.ToSubjectID); known && ref.subject.Object != nil && ref.subject.Object.Kind == programindex.ObjectType {
				key := constructionKey(edge.FromSubjectID, edge.Location)
				facts.makes[key] = append(facts.makes[key], edge.ToSubjectID)
			}
		}
	}
	builder.dataByTarget[targetID] = facts
	return facts
}

func constructionKey(caller string, at *programindex.Location) string {
	return caller + "\x00" + at.Path + ":" + strconv.Itoa(at.Line)
}

// recordField is a field's record type, when the subject is a variable a
// type owns.
func (builder *pageBuilder) recordField(targetID, subjectID string) (field *groupindex.ObjectFacts, owner groupindex.Subject, ok bool) {
	ref, known := builder.subject(targetID, subjectID)
	if !known || ref.subject.Object == nil || ref.subject.Object.Kind != programindex.ObjectVariable || ref.subject.Object.OwnerID == "" {
		return nil, groupindex.Subject{}, false
	}
	entity, known := builder.subject(targetID, ref.subject.Object.OwnerID)
	if !known || entity.subject.Object == nil || entity.subject.Object.Kind != programindex.ObjectType {
		return nil, groupindex.Subject{}, false
	}
	return ref.subject.Object, entity.subject, true
}

func (builder *pageBuilder) operationWrites(index *groupindex.Index, reach groupindex.Reach) []pageEntityWrite {
	if len(reach.Subjects) == 0 {
		return nil
	}
	targetID := index.Target.ID
	facts := builder.dataFacts(index)
	flow := builder.flowIndex(index)
	groupOf := builder.edgesBetweenGroups(*index).groupOf
	handler := reach.Subjects[0].SubjectID
	helper := func(id string) bool {
		ref, known := builder.subject(targetID, id)
		return known && ref.subject.Interpretation != nil && ref.subject.Interpretation.Helper &&
			flow.shared[groupOf[id]] && groupOf[id] != groupOf[handler]
	}
	// The reach's exact calls by caller, in the order the walk met them: a
	// dispatch's alternatives say which code may run, not what this input
	// changes (redis's call() dispatches to every command).
	calls := map[string][]int{}
	for _, position := range reach.Edges {
		edge := index.StructuralEdges[position]
		if (edge.RelationKind == programindex.RelationCalls || edge.RelationKind == programindex.RelationExecutes) && edge.Resolution == programindex.ResolutionExact {
			calls[edge.FromSubjectID] = append(calls[edge.FromSubjectID], position)
		}
	}
	walk := func(from string, enter func(string) bool) []string {
		order, seen := []string{from}, map[string]bool{from: true}
		for at := 0; at < len(order); at++ {
			for _, position := range calls[order[at]] {
				callee := index.StructuralEdges[position].ToSubjectID
				if !seen[callee] && enter(callee) {
					seen[callee] = true
					order = append(order, callee)
				}
			}
		}
		return order
	}
	work := walk(handler, func(callee string) bool { return !helper(callee) })
	// A constructor the work runs where it makes its class's object sets
	// that object up; it changes nothing the program holds.
	constructs := map[string]string{}
	for _, id := range work {
		for _, position := range calls[id] {
			edge := index.StructuralEdges[position]
			ref, known := builder.subject(targetID, edge.ToSubjectID)
			if known && ref.subject.Object != nil && ref.subject.Object.Kind == programindex.ObjectMethod &&
				slices.Contains(facts.makes[constructionKey(id, edge.Location)], ref.subject.Object.OwnerID) {
				constructs[edge.ToSubjectID] = ref.subject.Object.OwnerID
			}
		}
	}
	// The record types a helper's own code writes, through the calls it
	// makes in this reach.
	written := map[string]map[string]bool{}
	writtenBy := func(helperID string) map[string]bool {
		if types, done := written[helperID]; done {
			return types
		}
		types := map[string]bool{}
		for _, id := range walk(helperID, func(string) bool { return true }) {
			for _, position := range facts.writes[id] {
				if _, owner, ok := builder.recordField(targetID, index.StructuralEdges[position].ToSubjectID); ok {
					types[owner.ID] = true
				}
			}
		}
		written[helperID] = types
		return types
	}
	var result []pageEntityWrite
	seen := map[string]int{}
	// Each target once, with every function making the change.
	add := func(change pageEntityWrite, maker string, at *programindex.Location) {
		key := strings.Join([]string{change.Kind, change.Entity.Href + change.Entity.Open + change.EntityName, change.Field, change.Destination, strings.Join(change.Tables, ",")}, "\x00")
		var step *pageCallStep
		if ref, known := builder.subject(targetID, maker); known {
			name, anchor := builder.subjectDisplay(ref.subject)
			step = &pageCallStep{Name: name}
			if anchor != nil {
				step.Href, step.Open, step.Source, step.NoSource = anchor.Href, anchor.Open, anchor.Text, anchor.NoSource
			}
		}
		if listed, done := seen[key]; done {
			if step != nil && !slices.ContainsFunc(result[listed].Callers, func(other pageCallStep) bool { return other.Name == step.Name && other.Href == step.Href }) {
				result[listed].Callers = append(result[listed].Callers, *step)
			}
			for _, via := range change.Via {
				if !slices.Contains(result[listed].Via, via) {
					result[listed].Via = append(result[listed].Via, via)
				}
			}
			result[listed].Possible = result[listed].Possible && change.Possible
			return
		}
		seen[key] = len(result)
		if at != nil {
			change.Source = builder.links.anchor(at.Path, at.Line, at.Column)
		}
		if step != nil {
			change.Callers = []pageCallStep{*step}
		}
		result = append(result, change)
	}
	// The program's data: a field reached through a file-scope variable, or
	// a field of a type owning a table.
	data := func(edge groupindex.StructuralEdge, owner groupindex.Subject) bool {
		if facts.tables[owner.ID] {
			return true
		}
		root, _, _ := strings.Cut(edge.FieldPath, ".")
		return root != "" && facts.globals[root]
	}
	typed := func(change pageEntityWrite, entity groupindex.Subject) (pageEntityWrite, bool) {
		name, anchor := builder.subjectDisplay(entity)
		if anchor == nil {
			return change, false
		}
		change.Entity, change.EntityName = *anchor, name
		return change, true
	}
	for _, id := range work {
		// What the work writes of the program's data, and the fields it
		// hands to a helper whose code writes their type, in the order its
		// code makes them.
		type made struct {
			change pageEntityWrite
			at     *programindex.Location
		}
		var mine []made
		for _, position := range facts.writes[id] {
			edge := index.StructuralEdges[position]
			field, owner, ok := builder.recordField(targetID, edge.ToSubjectID)
			if !ok || constructs[id] == owner.ID || !data(edge, owner) {
				continue
			}
			if change, ok := typed(pageEntityWrite{Kind: "write", Field: field.Name, Possible: edge.Resolution != programindex.ResolutionExact}, owner); ok {
				mine = append(mine, made{change, edge.Location})
			}
		}
		for _, position := range calls[id] {
			edge := index.StructuralEdges[position]
			if !helper(edge.ToSubjectID) {
				continue
			}
			types := writtenBy(edge.ToSubjectID)
			if len(types) == 0 {
				continue
			}
			ref, known := builder.subject(targetID, edge.ToSubjectID)
			if !known {
				continue
			}
			via, _ := builder.subjectDisplay(ref.subject)
			for _, fieldID := range builder.handedFields(index, facts, edge) {
				field, owner, ok := builder.recordField(targetID, fieldID)
				if !ok || !changesType(facts, types, fieldID) {
					continue
				}
				if change, ok := typed(pageEntityWrite{Kind: "call", Field: field.Name, Via: []string{via}}, owner); ok {
					mine = append(mine, made{change, edge.Location})
				}
			}
		}
		slices.SortStableFunc(mine, func(a, b made) int { return compareSites(a.at, b.at) })
		for _, change := range mine {
			add(change.change, id, change.at)
		}
	}
	// The database and the files anywhere in its reach.
	reached := map[string]bool{}
	for _, subject := range reach.Subjects {
		reached[subject.SubjectID] = true
	}
	records := map[string]groupindex.DataRecord{}
	for _, record := range index.Data {
		records[record.ID] = record
	}
	for _, call := range index.Outbound {
		if call.Kind != "db" || !reached[call.SubjectID] {
			continue
		}
		change := pageEntityWrite{Kind: "db", Destination: cmp.Or(call.Destination, call.External)}
		for _, id := range call.DataIDs {
			record, known := records[id]
			if !known || record.Data == nil || record.Data.Name == "" {
				continue
			}
			if !slices.Contains(change.Tables, record.Data.Name) {
				change.Tables = append(change.Tables, record.Data.Name)
			}
			if record.OwnerSubjectID != "" && change.EntityName == "" {
				if ref, known := builder.subject(targetID, record.OwnerSubjectID); known {
					change, _ = typed(change, ref.subject)
				}
			}
		}
		location := call.Location
		add(change, call.SubjectID, &location)
	}
	for _, record := range index.Data {
		if record.Data == nil || record.Data.Kind != "file" || record.Data.File == nil {
			continue
		}
		name := cmp.Or(record.Data.Name, record.Data.File.Field)
		if name == "" {
			continue
		}
		for i, subject := range record.CallSubjectIDs {
			if subject == "" || !reached[subject] || i >= len(record.Data.File.Calls) {
				continue
			}
			at := record.Data.File.Calls[i].Anchor
			add(pageEntityWrite{Kind: "file", Destination: name}, subject, &programindex.Location{Path: at.Path, Line: at.Line, Column: at.Column})
			break
		}
	}
	return result
}

// changesType says whether a helper writing the given record types changes
// a field handed to it: the field's own type, or a record type one of that
// type's fields holds (a `redisDb *` whose dict dictAdd writes).
func changesType(facts *pageDataFacts, written map[string]bool, fieldID string) bool {
	for _, typeID := range facts.typesOf[fieldID] {
		if written[typeID] {
			return true
		}
		for _, inner := range facts.fieldsOf[typeID] {
			for _, held := range facts.typesOf[inner] {
				if written[held] {
					return true
				}
			}
		}
	}
	return false
}

// handedFields are the record fields a call passes as its arguments: an
// argument naming its object (a Python or Go field), else the field its
// expression ends in, read by the caller within the argument
// (`c->db->dict`: the read of dict there).
func (builder *pageBuilder) handedFields(index *groupindex.Index, facts *pageDataFacts, edge groupindex.StructuralEdge) []string {
	relation, known := facts.relations[edge.RelationID]
	if !known || len(relation.Patterns) == 0 {
		return nil
	}
	arguments := relation.Patterns[0].Arguments
	var fields []string
	for i, argument := range arguments {
		for _, id := range argument.ObjectIDs {
			if _, _, ok := builder.recordField(index.Target.ID, id); ok && !slices.Contains(fields, id) {
				fields = append(fields, id)
			}
		}
		origin := argument.Origin
		if len(argument.ObjectIDs) > 0 || origin == nil || origin.Kind != "field" || origin.Anchor == nil {
			continue
		}
		end := 0
		if i+1 < len(arguments) && arguments[i+1].Origin != nil && arguments[i+1].Origin.Anchor != nil && arguments[i+1].Origin.Anchor.Line == origin.Anchor.Line {
			end = arguments[i+1].Origin.Anchor.Column
		}
		last := ""
		for _, position := range facts.reads[edge.FromSubjectID] {
			read := index.StructuralEdges[position]
			at := read.Location
			if at.Path != origin.Anchor.Path || at.Line != origin.Anchor.Line || at.Column < origin.Anchor.Column || end > 0 && at.Column >= end {
				continue
			}
			if ref, known := builder.subject(index.Target.ID, read.ToSubjectID); known && ref.subject.Object != nil && ref.subject.Object.Name == origin.Text {
				last = read.ToSubjectID
			}
		}
		if last != "" && !slices.Contains(fields, last) {
			fields = append(fields, last)
		}
	}
	return fields
}
