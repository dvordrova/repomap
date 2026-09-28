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
// Registrar), and a table of names that stores no callable (a symbol's
// Rows). Neither has an outside symbol whose role decides it, so each is
// asked once (lines.Inputs): a kept callable per registrar and callable, a
// table per table. An accepted kept callable is an entry with its handler;
// an accepted table makes each of its rows an entry whose handler is not
// established, declared by the table, one catalogue. None, middleware and
// an undecided answer make nothing; the classifier's rejected rows and a
// tables.md line record them.

// storedKey is the question a kept callable is asked under: its registrar
// and the callable, so every call keeping the same callable in the same
// function shares one answer.
func storedKey(b *atlas.BoundaryFacts) string {
	return b.Registrar.Path + "\x00" + b.Registrar.Name + "\x00" + b.SubjectID
}

// readInputs asks the stored and table questions, each over its own rows.
func (r *reader) readInputs(ctx context.Context) error {
	r.storedKinds, r.tableKinds = map[string]string{}, map[string]string{}
	files := map[string][]byte{}
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
			if use.Kind == "reads" && use.Resolution == "exact" && !slices.Contains(readers[use.PlaceID], place.Symbol.Decl.Name) {
				readers[use.PlaceID] = append(readers[use.PlaceID], place.Symbol.Decl.Name)
			}
		}
	}
	var tableRows []table.Row
	var tableIDs []string
	tableSubjects := map[string]rowSubject{}
	for _, place := range r.opts.Graph.Places {
		if place.Symbol == nil || len(place.Symbol.Rows) == 0 || r.testFile(place.Parent) {
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
		item = append(item, table.Field{Name: "rows", Value: rows})
		if reading := readers[place.ID]; len(reading) > 0 {
			sort.Strings(reading)
			item = append(item, table.Field{Name: "read_by", Value: reading})
		}
		rowID := fmt.Sprintf("table%d", len(tableRows)+1)
		tableSubjects[rowID] = rowSubject{id: "table:" + place.ID, path: place.Path, line: place.LineNo}
		tableRows = append(tableRows, table.Row{ID: rowID, Fields: item})
		tableIDs = append(tableIDs, place.ID)
	}
	if len(storedRows)+len(tableRows) == 0 {
		return nil
	}
	r.opts.Stage(lines.StageInputs, fmt.Sprintf("asking what %d callables the repository's own functions keep and %d tables of names become", len(storedRows), len(tableRows)))
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
	if len(tableRows) > 0 {
		r.rowSubjects = tableSubjects
		answers, err := r.runTable(ctx, lines.Inputs("table"), 2, tableRows)
		if err != nil {
			return err
		}
		for i, id := range tableIDs {
			if answer := answers[i].answer; answer != nil && answer["becomes"] != "" {
				r.tableKinds[id] = answer["becomes"]
			}
		}
	}
	fmt.Fprintf(&r.tables, "atlas_inputs: %d of %d kept callables and %d of %d tables decided\n\n", len(r.storedKinds), len(storedRows), len(r.tableKinds), len(tableRows))
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
		if kind == "" || kind == lines.APINone || kind == lines.APIMiddleware || r.testFile(state.place.Parent) {
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
// is not established, at its first word, declared by the table.
func (r *reader) bindTableRows() {
	if r.boundaryIDs == nil {
		r.boundaryIDs = make(map[string]string)
	}
	for _, place := range r.opts.Graph.Places {
		kind := r.tableKinds[place.ID]
		if place.Symbol == nil || kind == "" || kind == lines.APINone {
			continue
		}
		targets := runningTargets(place)
		if len(targets) == 0 {
			targets = place.TargetIDs
		}
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
			r.boundaries[id] = &boundaryState{kind: kind, handlerUnknown: true, place: atlas.Place{
				ID: id, Kind: atlas.PlaceBoundary, Path: place.Path, LineNo: first.LineNo, Column: first.Column,
				Parent: place.Parent, TargetIDs: slices.Clone(targets), Boundary: &atlas.BoundaryFacts{
					Source: "model", ObjectID: decl.ObjectID, Caller: decl.Name,
					Values: slices.Clone(words), Words: slices.Clone(words), Direction: atlas.DirectionIn, GivenKind: kind}}}
		}
	}
}
