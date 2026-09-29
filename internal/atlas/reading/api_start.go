package reading

import (
	"context"
	"fmt"
	"sort"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/programindex"
)

// Statements that start one of the repository's own functions to run on
// its own (a registration fact with a shared invocation word, facts
// started.go: Go's `go f()`, a coroutine handed to asyncio.create_task).
// No outside symbol receives the function, so no symbol's binds answer
// decides it; one such answer would also take its example from the first
// statement and decide every other. Each statement outside tests is asked
// on its own (lines.APIStart). An entry answer is that entry, with the
// started function as its handler; none and an undecided answer make none.

// startSite is the site of a starting statement's boundary.
func startSite(place atlas.Place) sourceSite {
	return sourceSite{place.Path, place.LineNo, place.Column}
}

// readStarts asks what the function each starting statement starts
// becomes and keeps each answer by the statement's site: an entry kind,
// none, or "" for a statement asked and not decided.
func (r *reader) readStarts(ctx context.Context) error {
	r.startKinds = map[sourceSite]string{}
	// The declaration each started call is written in, and the call a
	// started coroutine is handed to, by the started call's site.
	type writtenAt struct {
		in     atlas.Place
		handed *atlas.SymbolCall
	}
	at := map[sourceSite]*writtenAt{}
	for _, place := range r.opts.Graph.Places {
		if place.Symbol == nil {
			continue
		}
		for _, call := range place.Symbol.Calls {
			if call.Line > 0 && (call.Invocation == programindex.InvocationGoroutine || call.Invocation == programindex.InvocationAsyncTask) {
				at[sourceSite{place.Path, call.Line, call.Column}] = &writtenAt{in: place}
			}
		}
	}
	for _, place := range r.opts.Graph.Places {
		if place.Symbol == nil {
			continue
		}
		for i := range place.Symbol.Calls {
			call := &place.Symbol.Calls[i]
			for _, argument := range call.SourceArguments {
				anchor := producedAnchor(argument.Origin)
				if anchor == nil {
					continue
				}
				if written := at[sourceSite{anchor.Path, anchor.Line, anchor.Column}]; written != nil && written.handed == nil {
					written.handed = call
				}
			}
		}
	}
	var ids []string
	for _, place := range r.opts.Graph.Places {
		if b := place.Boundary; b != nil && b.Invocation != "" && !r.testFile(place.Parent) {
			ids = append(ids, place.ID)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return compactIDLess(ids[i], ids[j]) })
	byID := map[string]atlas.Place{}
	for _, place := range r.opts.Graph.Places {
		byID[place.ID] = place
	}
	files := map[string]*lines.CallFile{}
	var rows []table.Row
	var sites []sourceSite
	subjects := map[string]rowSubject{}
	unheld := 0
	for _, id := range ids {
		place := byID[id]
		b := place.Boundary
		site := startSite(place)
		if _, seen := r.startKinds[site]; seen {
			continue
		}
		// A function the graph holds no declaration of (a closure that
		// calls nothing: go func() { <-signals; close(done) }()) has no
		// place an entry could be handled in: it is not asked.
		started := r.places[b.SubjectID]
		if started.Symbol == nil {
			unheld++
			continue
		}
		r.startKinds[site] = ""
		var fields []table.Field
		statement := r.sourceText(files, place.Path, place.LineNo, place.Column)
		written := at[site]
		if written != nil && written.handed != nil {
			if text := r.sourceText(files, place.Path, written.handed.Line, written.handed.Column); text != "" {
				statement = text
			}
		}
		if statement != "" {
			fields = append(fields, table.Field{Name: "statement", Value: statement})
		}
		if written != nil && written.in.Symbol != nil {
			fields = append(fields, table.Field{Name: "in", Value: declarationText(written.in.Symbol.Decl)})
		}
		fields = append(fields, table.Field{Name: "starts", Value: declarationText(started.Symbol.Decl)})
		if calls := calledNames(started.Symbol.Calls); len(calls) > 0 {
			fields = append(fields, table.Field{Name: "calls", Value: calls})
		}
		if len(b.Values) > 0 {
			fields = append(fields, table.Field{Name: "literals", Value: b.Values})
		}
		rowID := fmt.Sprintf("start%d", len(rows)+1)
		subjects[rowID] = rowSubject{id: fmt.Sprintf("start:%s:%d:%d", site.path, site.line, site.column), path: site.path, line: site.line}
		rows = append(rows, table.Row{ID: rowID, Fields: fields})
		sites = append(sites, site)
	}
	if unheld > 0 {
		fmt.Fprintf(&r.tables, "atlas_api starts: %d starting statements start a function the graph holds no declaration of; not asked\n\n", unheld)
	}
	if len(rows) == 0 {
		return nil
	}
	r.opts.Stage(lines.StageAPI, fmt.Sprintf("asking %d statements what the function each starts on its own becomes", len(rows)))
	previous := r.rowSubjects
	r.rowSubjects = subjects
	defer func() { r.rowSubjects = previous }()
	answers, err := r.runTable(ctx, lines.APIStart(), 4, rows)
	if err != nil {
		return err
	}
	decided := 0
	for i, site := range sites {
		if answer := answers[i].answer; answer != nil && answer["starts"] != "" {
			r.startKinds[site] = answer["starts"]
			decided++
		}
	}
	fmt.Fprintf(&r.tables, "atlas_api starts: %d of %d starting statements decided\n\n", decided, len(sites))
	return nil
}

// applyStarts gives each starting statement's boundary the kind its
// question decided; none or undecided is no entry. The entry is named by
// the started function's declaration, wherever it is declared: the
// statement's file may hold only the function starting it.
func (r *reader) applyStarts() {
	for _, id := range sortedKeys(r.boundaries) {
		state := r.boundaries[id]
		b := state.place.Boundary
		if b == nil || b.Invocation == "" {
			continue
		}
		kind := r.startKinds[startSite(state.place)]
		if kind == "" || kind == lines.APINone {
			delete(r.boundaries, id)
			continue
		}
		facts := *b
		facts.GivenKind = kind
		if started := r.places[b.SubjectID]; started.Symbol != nil {
			facts.Caller, facts.CallerDoc = started.Symbol.Decl.Name, started.Symbol.Decl.Doc
		}
		state.place.Boundary = &facts
		state.kind = kind
	}
}

// declarationText is a declaration as a row shows it: its name and its
// signature.
func declarationText(decl atlas.Decl) string {
	if decl.Signature == "" {
		return decl.Name
	}
	return decl.Name + " " + decl.Signature
}

// calledNames are the declarations and outside symbols a function calls,
// each once, in the order it writes them.
func calledNames(calls []atlas.SymbolCall) []string {
	ordered := append([]atlas.SymbolCall(nil), calls...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Line != ordered[j].Line {
			return ordered[i].Line < ordered[j].Line
		}
		return ordered[i].Column < ordered[j].Column
	})
	var names []string
	seen := map[string]bool{}
	for _, call := range ordered {
		name := call.Name
		if call.API != nil {
			name = apiName(*call.API)
		}
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return names
}
