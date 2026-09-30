package reading

import (
	"fmt"
	"slices"
	"sort"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// handedTable is a table of words an established entry's handler looks up
// with part of what it was handed: the handler's symbol place and the
// entry's kind.
type handedTable struct {
	handler string
	kind    string
}

// handedTables are, by table place, the tables of words an established
// entry's handler looks up with part of what it was handed (owner's rule
// K3 for a table): a call in the handler given both the table and a field
// or an element of one of its own parameters, or, when the handler hands
// such a part to another declaration of the program by an exact call, a
// call there given both the table and that declaration's parameter at the
// position the part was handed at. othello's key-pressed handler
// host/on-key hands (:key event) to events/on-key, which calls
// (get key->command key): the table's keys n, u, h, 1 and 2 are the keys
// key-pressed takes. A table is the call's argument or receiver where the
// code reads it (places ReadAt). Such a table is no input of its own and is
// not asked what its rows become: each row is a value of the entry, of its
// kind, named by its first word (bindTableRows). A table two handlers
// look up belongs to neither.
func (r *reader) handedTables() map[string]handedTable {
	kinds := r.handlerKinds()
	if len(kinds) == 0 {
		return nil
	}
	// Each table's reads, by the reading declaration and the site, and the
	// tables each declaration reads.
	readAt := map[string]map[[2]int]string{}
	readBy := map[string][]atlas.Place{}
	for _, place := range r.opts.Graph.Places {
		if place.Symbol == nil || len(place.Symbol.Rows) == 0 || r.testFile(place.Parent) {
			continue
		}
		for _, read := range place.Symbol.ReadAt {
			if readAt[read.ReaderID] == nil {
				readAt[read.ReaderID] = map[[2]int]string{}
			}
			readAt[read.ReaderID][[2]int{read.LineNo, read.Column}] = place.ID
			if !slices.ContainsFunc(readBy[read.ReaderID], func(known atlas.Place) bool { return known.ID == place.ID }) {
				readBy[read.ReaderID] = append(readBy[read.ReaderID], place)
			}
		}
	}
	// tableOf is the table a call's argument or receiver is: read there
	// (Clojure's argument at its symbol), or a table the reader reads whose
	// value the adapter traced it to (Python's receiver at the dict written
	// in the table's own declaration).
	tableOf := func(reader string, call atlas.SymbolCall) []string {
		var tables []string
		values := []*sourcevalue.Value{call.ReceiverValue}
		for _, argument := range call.SourceArguments {
			values = append(values, argument.Origin)
		}
		for _, value := range values {
			if value == nil || value.Anchor == nil {
				continue
			}
			found := readAt[reader][[2]int{value.Anchor.Line, value.Anchor.Column}]
			for _, table := range readBy[reader] {
				decl := table.Symbol.Decl
				if found == "" && value.Anchor.Path == table.Path && value.Anchor.Line >= decl.LineNo && value.Anchor.Line <= max(decl.EndLine, decl.LineNo) {
					found = table.ID
				}
			}
			if found != "" && !slices.Contains(tables, found) {
				tables = append(tables, found)
			}
		}
		return tables
	}
	result := map[string]handedTable{}
	shared := map[string]bool{}
	claim := func(table, handler, kind string) {
		if previous, seen := result[table]; seen && previous.handler != handler {
			shared[table] = true
		}
		result[table] = handedTable{handler: handler, kind: kind}
	}
	handlers := make([]string, 0, len(kinds))
	for handler := range kinds {
		handlers = append(handlers, handler)
	}
	sort.Strings(handlers)
	for _, handler := range handlers {
		place := r.places[handler]
		if place.Symbol == nil {
			continue
		}
		for _, call := range place.Symbol.Calls {
			// The handler's own lookup of a table with part of what it was
			// handed.
			if handedPart(place, call.ReceiverValue) || slices.ContainsFunc(call.SourceArguments, func(argument atlas.SourceArgument) bool { return handedPart(place, argument.Origin) }) {
				for _, table := range tableOf(handler, call) {
					claim(table, handler, kinds[handler])
				}
			}
			// A part handed on to another declaration by an exact call.
			if call.Kind != "calls" || call.Resolution != "exact" || len(call.CalleeIDs) != 1 {
				continue
			}
			callee := r.places[call.CalleeIDs[0]]
			if callee.Symbol == nil || callee.ID == handler || r.testFile(callee.Parent) {
				continue
			}
			for _, argument := range call.SourceArguments {
				if argument.Position < 1 || !handedPart(place, argument.Origin) {
					continue
				}
				for _, inner := range callee.Symbol.Calls {
					if !slices.ContainsFunc(inner.SourceArguments, func(given atlas.SourceArgument) bool {
						return ownParameter(callee, given.Origin, argument.Position)
					}) {
						continue
					}
					for _, table := range tableOf(callee.ID, inner) {
						claim(table, handler, kinds[handler])
					}
				}
			}
		}
	}
	for table := range shared {
		delete(result, table)
	}
	return result
}

// ownParameter reports a value that is the declaration's own parameter at
// a position (any, for 0).
func ownParameter(place atlas.Place, value *sourcevalue.Value, position int) bool {
	return value != nil && value.Kind == "parameter" && value.Owner != nil && place.Symbol != nil &&
		value.Owner.Path == place.Path && value.Owner.Line == place.Symbol.Decl.LineNo && (position == 0 || value.Position == position)
}

// handedPart reports a value that is part of what the declaration was
// handed: a field or an element of one of its own parameters, at any depth
// (subArgumentKind's rule). A parameter used whole is not: it is as often
// where the handler writes its answer as what it looks up.
func handedPart(place atlas.Place, value *sourcevalue.Value) bool {
	if value == nil || value.Kind != "field" && value.Kind != "index" || len(value.Parts) == 0 {
		return false
	}
	return ownParameter(place, &value.Parts[0], 0) || handedPart(place, &value.Parts[0])
}

// handedEntry is the entry an established handler handles, for its table's
// values: its boundary among the inputs registering that handler, the first
// by ID of the handler's kind.
func (r *reader) handedEntry(handler, kind string) string {
	var ids []string
	for id, state := range r.boundaries {
		b := state.place.Boundary
		if b != nil && b.Direction == atlas.DirectionIn && b.SubjectID == handler && state.kind == kind && !state.handlerUnknown {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return compactIDLess(ids[i], ids[j]) })
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

// bindHandedRows makes each row of a table an established handler looks up
// with part of what it was handed a value of that handler's entry
// (handedTables): an entry of the entry's kind at its first word, named by
// that word, the key the lookup finds it by, whose handler is not
// established, listed under the entry (ValueOf) and never asked, neither
// what it is nor its name. The row's other words are what the key names
// (Names), and the row stays as written (othello's `:n :new-game`: the key
// n, naming new-game; the model had named it "n new-game"). It reports the
// tables it bound.
func (r *reader) bindHandedRows() map[string]bool {
	bound := map[string]bool{}
	for tableID, handed := range r.handedTables() {
		entry := r.handedEntry(handed.handler, handed.kind)
		place := r.places[tableID]
		if entry == "" || place.Symbol == nil {
			continue
		}
		bound[tableID] = true
		targets := runningTargets(place)
		decl := place.Symbol.Decl
		for position, row := range place.Symbol.Rows {
			var words []string
			for _, literal := range row.Literals {
				words = append(words, literal.Value)
			}
			first := row.Literals[0]
			nameable := lines.NameableWords(words)
			if len(nameable) == 0 {
				continue
			}
			source := fmt.Sprintf("%s\x00row\x00%s\x00%d", atlas.DirectionIn, place.ID, position)
			id := r.boundaryIDs[source]
			if id == "" {
				id = r.compactID("b", &r.nextBoundary)
				r.boundaryIDs[source] = id
			}
			// The neighbouring rows' nearest words bound the row as written.
			var neighbours [][2]int
			if position > 0 {
				previous := place.Symbol.Rows[position-1].Literals
				neighbours = append(neighbours, [2]int{previous[len(previous)-1].LineNo, previous[len(previous)-1].Column})
			}
			if position+1 < len(place.Symbol.Rows) {
				next := place.Symbol.Rows[position+1].Literals[0]
				neighbours = append(neighbours, [2]int{next.LineNo, next.Column})
			}
			// No words to name it by: a row the model made with none is not
			// asked (readBoundaries).
			r.boundaries[id] = &boundaryState{kind: handed.kind, name: nameable[0], names: slices.Clone(nameable[1:]), handlerUnknown: true, tableRow: true, rowNeighbours: neighbours, valueOf: entry, place: atlas.Place{
				ID: id, Kind: atlas.PlaceBoundary, Path: place.Path, LineNo: first.LineNo, Column: first.Column,
				Parent: place.Parent, TargetIDs: slices.Clone(targets), Boundary: &atlas.BoundaryFacts{
					Source: "model", ObjectID: decl.ObjectID, Caller: decl.Name,
					Values: slices.Clone(words), Direction: atlas.DirectionIn, GivenKind: handed.kind}}}
		}
	}
	return bound
}
