package reading

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

// Inputs the repository's own code declares (pass 2, C): a callable a
// repository function keeps for later (a registration fact with a
// Registrar), a table of names that stores no callable (a symbol's Rows),
// and a field whose tag names a key (settings.go). None has an outside
// symbol whose role decides it, so each is asked once (lines.Inputs): a
// kept callable per registrar and callable, a table per table, a field per
// field. An accepted kept callable is an entry with its handler;
// an accepted table makes each of its rows an entry whose handler is not
// established, declared by the table, one catalogue. None, middleware and
// an undecided answer make nothing; the classifier's rejected rows and a
// tables.md line record them. A table whose rows are keys of another table,
// or which is only tested for membership, is not asked (tableReferences).

// storedKey is the question a kept callable is asked under: its registrar
// and the callable, so every call keeping the same callable in the same
// function shares one answer.
func storedKey(b *atlas.BoundaryFacts) string {
	return b.Registrar.Path + "\x00" + b.Registrar.Name + "\x00" + b.SubjectID
}

// readInputs asks the stored and table questions, each over its own rows.
func (r *reader) readInputs(ctx context.Context) error {
	r.storedKinds, r.tableKinds = map[string]string{}, map[string]string{}
	if err := r.readComparisons(ctx); err != nil {
		return err
	}
	files := map[string]*lines.CallFile{}
	// Kept callables, grouped by registrar and callable.
	var storedRows []table.Row
	var storedKeys []string
	storedSubjects := map[string]rowSubject{}
	seen := map[string]bool{}
	var ids []string
	byID := map[string]atlas.Place{}
	for _, place := range r.opts.Graph.Places {
		if b := place.Boundary; b != nil && b.Registrar != nil && b.SubjectID != "" && !r.testFile(place.Parent) {
			ids = append(ids, place.ID)
			byID[place.ID] = place
		}
	}
	sort.Slice(ids, func(i, j int) bool { return compactIDLess(ids[i], ids[j]) })
	for _, id := range ids {
		place := byID[id]
		b := place.Boundary
		key := storedKey(b)
		if seen[key] {
			continue
		}
		seen[key] = true
		handed := r.places[b.SubjectID]
		callable := b.Caller
		if handed.Symbol != nil {
			callable = handed.Symbol.Decl.Name
			if signature := handed.Symbol.Decl.Signature; signature != "" {
				callable += " " + signature
			}
		}
		keptBy := b.Registrar.Name
		if b.Registrar.Signature != "" {
			keptBy += " " + b.Registrar.Signature
		}
		keptBy += "; keeps it in " + strings.Join(b.Registrar.Slots, ", ")
		item := []table.Field{{Name: "callable", Value: callable}, {Name: "kept_by", Value: keptBy}}
		if text := r.sourceText(files, place.Path, place.LineNo, place.Column); text != "" {
			item = append(item, table.Field{Name: "call", Value: text})
		}
		if len(b.Values) > 0 {
			item = append(item, table.Field{Name: "literals", Value: b.Values})
		}
		var during []string
		for _, at := range b.Registrar.During {
			line := ""
			switch {
			case at.Seed != "":
				line = "from the program's start at " + at.Seed
			case at.HandedTo != "":
				line = "while " + strings.Join(at.Handlers, ", ") + " run (handed to " + at.HandedTo + ")"
			}
			if len(at.Through) > 0 {
				line += ", through " + strings.Join(at.Through, ", ")
			}
			if line != "" {
				during = append(during, line)
			}
		}
		if len(during) > 0 {
			item = append(item, table.Field{Name: "during", Value: during})
		}
		rowID := fmt.Sprintf("kept%d", len(storedRows)+1)
		storedSubjects[rowID] = rowSubject{id: "stored:" + b.Registrar.Name + "@" + handed.ID, path: place.Path, line: place.LineNo}
		storedRows = append(storedRows, table.Row{ID: rowID, Fields: item})
		storedKeys = append(storedKeys, key)
	}
	// Tables of names.
	readers := map[string][]string{}
	for _, place := range r.opts.Graph.Places {
		if place.Symbol == nil {
			continue
		}
		for _, use := range place.Symbol.Uses {
			if use.Kind == "reads" && use.Resolution == "exact" && !slices.Contains(readers[use.PlaceID], place.ID) {
				readers[use.PlaceID] = append(readers[use.PlaceID], place.ID)
			}
		}
	}
	var tableRows []table.Row
	var tableIDs []string
	tableSubjects := map[string]rowSubject{}
	sourceLines := map[string][]string{}
	references := r.tableReferences()
	for _, place := range r.opts.Graph.Places {
		if place.Symbol == nil || len(place.Symbol.Rows) == 0 || r.testFile(place.Parent) {
			continue
		}
		if reference := references[place.ID]; reference != "" {
			fmt.Fprintf(&r.tables, "- atlas_inputs: table %s at %s:%d %s; not asked\n", place.Symbol.Decl.Name, place.Path, place.LineNo, reference)
			continue
		}
		decl := place.Symbol.Decl
		rows := make([][]string, 0, len(place.Symbol.Rows))
		for _, row := range place.Symbol.Rows {
			words := make([]string, 0, len(row.Literals))
			for _, literal := range row.Literals {
				words = append(words, literal.Value)
			}
			rows = append(rows, words)
		}
		item := []table.Field{{Name: "table", Value: decl.Name}}
		if decl.Signature != "" {
			item = append(item, table.Field{Name: "declared", Value: decl.Signature})
		}
		// The file declaring the table says whose it is: a client's table
		// of the commands a person types is written in the client's files.
		item = append(item, table.Field{Name: "file", Value: place.Path}, table.Field{Name: "rows", Value: rows})
		if reading := r.tableReaders(sourceLines, place, readers[place.ID]); len(reading) > 0 {
			item = append(item, table.Field{Name: "read_by", Value: reading})
		}
		rowID := fmt.Sprintf("table%d", len(tableRows)+1)
		tableSubjects[rowID] = rowSubject{id: "table:" + place.ID, path: place.Path, line: place.LineNo}
		tableRows = append(tableRows, table.Row{ID: rowID, Fields: item})
		tableIDs = append(tableIDs, place.ID)
	}
	// Fields whose tags name keys (settings.go).
	fields := r.taggedFields()
	fieldRows, fieldSubjects := r.fieldRows(fields)
	if len(storedRows)+len(tableRows)+len(fieldRows) == 0 {
		return nil
	}
	r.opts.Stage(lines.StageInputs, fmt.Sprintf("asking what %d callables the repository's own functions keep, %d tables of names and %d tagged fields become", len(storedRows), len(tableRows), len(fieldRows)))
	previous := r.rowSubjects
	defer func() { r.rowSubjects = previous }()
	if len(storedRows) > 0 {
		r.rowSubjects = storedSubjects
		answers, err := r.runTable(ctx, lines.Inputs("stored"), 1, storedRows)
		if err != nil {
			return err
		}
		for i, key := range storedKeys {
			if answer := answers[i].answer; answer != nil && answer["becomes"] != "" {
				r.storedKinds[key] = answer["becomes"]
			}
		}
	}
	// tableKinds already holds the comparisons' kinds (dispatch.go): each
	// count is of its own question's answers.
	tables, fieldsDecided := 0, 0
	if len(tableRows) > 0 {
		r.rowSubjects = tableSubjects
		answers, err := r.runTable(ctx, lines.Inputs("table"), 2, tableRows)
		if err != nil {
			return err
		}
		for i, id := range tableIDs {
			if answer := answers[i].answer; answer != nil && answer["becomes"] != "" {
				r.tableKinds[id] = answer["becomes"]
				tables++
			}
		}
	}
	if len(fieldRows) > 0 {
		r.rowSubjects = fieldSubjects
		answers, err := r.runTable(ctx, lines.Inputs("field"), 3, fieldRows)
		if err != nil {
			return err
		}
		for i, field := range fields {
			if answer := answers[i].answer; answer != nil && answer["becomes"] != "" {
				r.tableKinds[fieldKey(field)] = answer["becomes"]
				fieldsDecided++
			}
		}
	}
	fmt.Fprintf(&r.tables, "atlas_inputs: %d of %d kept callables, %d of %d tables and %d of %d tagged fields decided\n\n", len(r.storedKinds), len(storedRows), tables, len(tableRows), fieldsDecided, len(fieldRows))
	r.reportStage(lines.StageInputs)
	return nil
}

// applyStored gives each kept callable's boundary the kind its question
// decided; one decided none, middleware or undecided is no entry.
func (r *reader) applyStored() {
	for _, id := range sortedKeys(r.boundaries) {
		state := r.boundaries[id]
		b := state.place.Boundary
		if b == nil || b.Registrar == nil {
			continue
		}
		kind := r.storedKinds[storedKey(b)]
		if kind == "" || kind == lines.APINone || kind == lines.APIMiddleware {
			delete(r.boundaries, id)
			continue
		}
		facts := *b
		facts.GivenKind = kind
		state.place.Boundary = &facts
		state.kind = kind
	}
}

// bindTableRows makes each row of an accepted table an entry whose handler
// is not established, at its first word, declared by the table, each case
// of an accepted comparison one (dispatch.go), and each field answered
// setting one, declared by its structure (settings.go).
func (r *reader) bindTableRows() {
	defer r.bindSettingFields()
	r.bindComparisons()
	for _, place := range r.opts.Graph.Places {
		kind := r.tableKinds[place.ID]
		if place.Symbol == nil || kind == "" || kind == lines.APINone {
			continue
		}
		targets := runningTargets(place)
		decl := place.Symbol.Decl
		for position, row := range place.Symbol.Rows {
			var words []string
			for _, literal := range row.Literals {
				words = append(words, literal.Value)
			}
			first := row.Literals[0]
			if len(lines.NameableWords(words)) == 0 {
				r.noEntryWithoutWords(atlas.Place{ID: place.ID, Path: place.Path, LineNo: first.LineNo}, kind)
				continue
			}
			source := fmt.Sprintf("%s\x00row\x00%s\x00%d", atlas.DirectionIn, place.ID, position)
			id := r.boundaryIDs[source]
			if id == "" {
				id = r.compactID("b", &r.nextBoundary)
				r.boundaryIDs[source] = id
			}
			r.boundaries[id] = &boundaryState{kind: kind, handlerUnknown: true, tableRow: true, place: atlas.Place{
				ID: id, Kind: atlas.PlaceBoundary, Path: place.Path, LineNo: first.LineNo, Column: first.Column,
				Parent: place.Parent, TargetIDs: slices.Clone(targets), Boundary: &atlas.BoundaryFacts{
					Source: "model", ObjectID: decl.ObjectID, Caller: decl.Name,
					Values: slices.Clone(words), Words: slices.Clone(words), Direction: atlas.DirectionIn, GivenKind: kind}}}
		}
	}
}

// tableReferences are the tables whose rows are no inputs of their own, each
// with why, by place. A table every read of which outside tests looks
// another table up with each of its one-word rows (a keys read) holds that
// table's rows: ARGS_TRADE's option keys, which _build_args looks up in
// AVAILABLE_CLI_OPTIONS. The other table must be asked, or hold in turn the
// rows of one that is; a cycle of such tables is asked. A table every read
// of which outside tests only tests a value against its rows holds the
// words of one condition, as a list written in the condition is one case and
// no comparison: NO_CONF_REQURIED, which _parse_args tests the parsed
// subcommand against. Either is used by its readers, not asked.
func (r *reader) tableReferences() map[string]string {
	keysOf := map[string][]string{}
	usedBy := map[string][]string{}
	result := map[string]string{}
	for _, place := range r.opts.Graph.Places {
		if place.Symbol == nil || len(place.Symbol.Rows) == 0 || r.testFile(place.Parent) {
			continue
		}
		var keys, readers []string
		membership, plain := false, false
		for _, read := range place.Symbol.ReadAt {
			reader := r.places[read.ReaderID]
			if reader.Symbol == nil || r.testFile(reader.Parent) {
				continue
			}
			if name := reader.Symbol.Decl.Name; !slices.Contains(readers, name) {
				readers = append(readers, name)
			}
			switch read.Form {
			case atlas.TableReadKeys:
				if !slices.Contains(keys, read.KeysOf) {
					keys = append(keys, read.KeysOf)
				}
			case atlas.TableReadMembership:
				membership = true
			default:
				plain = true
			}
		}
		switch {
		case plain || len(readers) == 0:
		case membership && len(keys) == 0:
			result[place.ID] = "is only tested for a value's membership, used by " + strings.Join(readers, ", ")
		case !membership && oneWordRows(place.Symbol.Rows):
			keysOf[place.ID], usedBy[place.ID] = keys, readers
		}
	}
	// A table holds another's rows when that one is asked, or holds in turn
	// the rows of one that is.
	const (
		visiting = iota + 1
		holds
		asked
	)
	state := map[string]int{}
	var resolve func(id string) bool
	resolve = func(id string) bool {
		keys, ok := keysOf[id]
		if !ok {
			return false
		}
		switch state[id] {
		case visiting, asked:
			return false
		case holds:
			return true
		}
		state[id] = visiting
		for _, of := range keys {
			table := r.places[of]
			if r.testFile(table.Parent) || result[of] != "" || keysOf[of] != nil && !resolve(of) {
				state[id] = asked
				return false
			}
		}
		state[id] = holds
		return true
	}
	for _, place := range r.opts.Graph.Places {
		if keysOf[place.ID] == nil || !resolve(place.ID) {
			continue
		}
		var names []string
		for _, of := range keysOf[place.ID] {
			names = append(names, r.places[of].Symbol.Decl.Name)
		}
		result[place.ID] = "holds keys of " + strings.Join(names, ", ") + ", used by " + strings.Join(usedBy[place.ID], ", ")
	}
	return result
}

// oneWordRows says every row of a table is one word with no field: a list,
// tuple or set of strings, whose elements a loop reads whole.
func oneWordRows(rows []atlas.TableRow) bool {
	for _, row := range rows {
		if len(row.Literals) != 1 || row.Literals[0].Field != "" {
			return false
		}
	}
	return true
}

// tableReaders is how a table is read, for its question: each declaration
// reading it, by name and signature, with each line reading it as the code
// wrote it (places ReadAt) and the declarations outside tests calling it,
// by name and signature, each with its lines calling the reader as
// written. redis-cli's cmdTable is read by lookupCommand, which compares a
// row's name with the name it is handed, and cliSendCommand calls it with
// the first word of its argument vector: what a person typed is looked up
// there. A reader the graph holds no read site of shows its name and
// callers.
func (r *reader) tableReaders(sourceLines map[string][]string, table atlas.Place, readerIDs []string) []map[string]any {
	sites := map[string][]int{}
	for _, read := range table.Symbol.ReadAt {
		if !slices.Contains(readerIDs, read.ReaderID) {
			readerIDs = append(readerIDs, read.ReaderID)
		}
		if !slices.Contains(sites[read.ReaderID], read.LineNo) {
			sites[read.ReaderID] = append(sites[read.ReaderID], read.LineNo)
		}
	}
	type reader struct {
		name   string
		fields map[string]any
	}
	var result []reader
	for _, id := range readerIDs {
		place := r.places[id]
		if place.Symbol == nil || r.testFile(place.Parent) {
			continue
		}
		decl := place.Symbol.Decl
		name := decl.Name
		if decl.Signature != "" {
			name += " " + decl.Signature
		}
		fields := map[string]any{"reader": name}
		var reads []string
		at := sites[id]
		slices.Sort(at)
		for _, line := range at {
			if text := r.sourceLine(sourceLines, place.Path, line); text != "" && !slices.Contains(reads, text) {
				reads = append(reads, text)
			}
		}
		if len(reads) > 0 {
			fields["reads"] = reads
		}
		// Each caller with the lines calling the reader as written: what
		// it hands the reader says where the word looked up comes from.
		var names []string
		calls := map[string][]string{}
		for _, caller := range place.Symbol.CalledBy {
			if caller.Kind != "calls" || caller.Resolution != "exact" || r.testPath(caller.Path) {
				continue
			}
			text := caller.Name
			if caller.Signature != "" {
				text += " " + caller.Signature
			}
			if _, seen := calls[text]; !seen {
				names = append(names, text)
				calls[text] = nil
			}
			if line := r.sourceLine(sourceLines, caller.Path, caller.Line); line != "" && !slices.Contains(calls[text], line) {
				calls[text] = append(calls[text], line)
			}
		}
		if len(names) > 0 {
			slices.Sort(names)
			var callers []map[string]any
			for _, name := range names {
				caller := map[string]any{"caller": name}
				if len(calls[name]) > 0 {
					caller["calls"] = calls[name]
				}
				callers = append(callers, caller)
			}
			fields["called_by"] = callers
		}
		result = append(result, reader{name: name, fields: fields})
	}
	slices.SortFunc(result, func(a, b reader) int { return strings.Compare(a.name, b.name) })
	entries := make([]map[string]any, 0, len(result))
	for _, item := range result {
		entries = append(entries, item.fields)
	}
	return entries
}

// sourceLine is one source line as the code wrote it, its space folded,
// reading each file once into sourceLines; "" for a line the file does not
// hold.
func (r *reader) sourceLine(sourceLines map[string][]string, path string, line int) string {
	if r.opts.ReadSource == nil || line < 1 {
		return ""
	}
	text, read := sourceLines[path]
	if !read {
		if content, err := r.opts.ReadSource(path); err == nil {
			text = strings.Split(string(content), "\n")
		}
		sourceLines[path] = text
	}
	if line > len(text) {
		return ""
	}
	return strings.Join(strings.Fields(text[line-1]), " ")
}
