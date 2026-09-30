package reading

import (
	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// The object an entry is declared on (decision 2026-09-28 §4): the value a
// call giving words or handing a callable acts on, followed back from its
// receiver (else the first argument a call produced) through the outside
// calls that name nothing (Flags(), PersistentFlags()), stopping at the
// first producing call that names something, by words, or at a composite
// literal the code built. It is a code fact: two entries declared on one
// object belong together (one catalogue), and a hand-over made on an
// entry's own result joins that entry. A parameter, a variable, or no
// produced value gives none: the entry is then keyed by the function that
// declares it. The object is never followed to the holder root, which
// would merge a parser's subparsers into one.

// callSites indexes every call of the graph's declarations by its site.
func (r *reader) callSites() map[sourceSite]*atlas.SymbolCall {
	if r.sites != nil {
		return r.sites
	}
	r.sites = make(map[sourceSite]*atlas.SymbolCall)
	for _, place := range r.opts.Graph.Places {
		if place.Symbol == nil {
			continue
		}
		for i := range place.Symbol.Calls {
			call := &place.Symbol.Calls[i]
			if call.Line < 1 {
				continue
			}
			site := sourceSite{place.Path, call.Line, call.Column}
			if _, seen := r.sites[site]; !seen {
				r.sites[site] = call
			}
		}
	}
	return r.sites
}

// declaredOn is the object the call at a site is made on, or nil.
func (r *reader) declaredOn(path string, line, column int) *sourcevalue.Anchor {
	sites := r.callSites()
	call := sites[sourceSite{path, line, column}]
	if call == nil {
		return nil
	}
	start := producedValue(call)
	if start == nil {
		return nil
	}
	at := *start
	for seen := map[sourcevalue.Anchor]bool{}; !seen[at]; {
		seen[at] = true
		producer := sites[sourceSite{at.Path, at.Line, at.Column}]
		// A call the graph does not hold, a repository call, or one that
		// names something is the object.
		if producer == nil || producer.Kind != string(programindex.RelationInvokesExternal) || len(producer.Values) > 0 {
			break
		}
		next := producedValue(producer)
		if next == nil {
			break
		}
		at = *next
	}
	return &at
}

// producedValue is where the value a call acts on was produced: its
// receiver's call or composite literal, else the first argument's.
func producedValue(call *atlas.SymbolCall) *sourcevalue.Anchor {
	if anchor := producedAnchor(call.ReceiverValue); anchor != nil {
		return anchor
	}
	if call.ReceiverValue != nil {
		return nil
	}
	for _, argument := range call.SourceArguments {
		if anchor := producedAnchor(argument.Origin); anchor != nil {
			return anchor
		}
	}
	return nil
}

func producedAnchor(value *sourcevalue.Value) *sourcevalue.Anchor {
	if value == nil || value.Anchor == nil {
		return nil
	}
	switch value.Kind {
	case "call_result", "record":
		return value.Anchor
	}
	return nil
}

// markDeclaredOn records, for every incoming boundary at a call, the object
// its call is made on, and the call that made the object as written; and
// the boundary's own registration as written, a table's row with its arity
// and flags (owner, 2026-09-28: RPUSH's arity 3 and REDIS_CMD_BULK were in
// no reading).
func (r *reader) markDeclaredOn() {
	files := map[string]*lines.CallFile{}
	for _, state := range r.boundaries {
		facts := state.place.Boundary
		if facts == nil || facts.Direction != atlas.DirectionIn || state.place.LineNo < 1 {
			continue
		}
		var written string
		if state.tableRow {
			// A row is its own element of the table, never the whole table.
			written = r.rowText(files, state.place.Path, state.place.LineNo, state.place.Column, state.rowNeighbours)
		} else {
			written = r.sourceText(files, state.place.Path, state.place.LineNo, state.place.Column)
		}
		if atlas.ValidName(written) {
			state.asWritten = written
		}
		on := r.declaredOn(state.place.Path, state.place.LineNo, state.place.Column)
		if on == nil {
			continue
		}
		state.on = r.declaredOnSite(files, on.Path, on.Line, on.Column)
	}
}

// declaredOnSite is the call at a site that made an object, with the call as
// written, folded to one line; a call whose text holds a character no line
// can show is named by its site alone.
func (r *reader) declaredOnSite(files map[string]*lines.CallFile, path string, line, column int) *atlas.DeclaredOn {
	text := r.sourceText(files, path, line, column)
	if !atlas.ValidName(text) {
		text = ""
	}
	return &atlas.DeclaredOn{Path: path, LineNo: line, Column: column, Text: text}
}
