package reading

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/programindex"
)

// Per-call entry questions (K4). A symbol answered per_call takes whatever
// words it is given, and they mean different things at different calls:
// each of its word calls outside tests is asked on its own (lines.APICall),
// once per call site, and remembered per site. A symbol whose own answer
// was undecided is never re-asked per call; its calls stay unsure. The
// answer of a call is what an entry at that call is; a call left undecided
// is unsure (launch.go).

// callLevels says, by symbol place, whether its code runs at launch (the
// walk from the seeds and the load-time roots over exact and alternative
// calls, never into a handler through alternatives) or inside an entry's
// handler already found (a callable a registration hands to a symbol that
// binds an entry), as the per-call item states it.
func (r *reader) callLevels() map[string]string {
	calls := map[string][]atlas.SymbolCall{}
	for _, place := range r.opts.Graph.Places {
		if place.Symbol != nil {
			calls[place.ID] = place.Symbol.Calls
		}
	}
	handlers := map[string]bool{}
	for _, place := range r.opts.Graph.Places {
		b := place.Boundary
		if b == nil || b.Source != "fact" || b.Direction != atlas.DirectionIn || b.SubjectID == "" || r.api[b.External].binds == "" {
			continue
		}
		handlers[b.SubjectID] = true
	}
	walk := func(roots []string) map[string]bool {
		seen := map[string]bool{}
		queue := []string{}
		for _, root := range roots {
			if !seen[root] {
				seen[root] = true
				queue = append(queue, root)
			}
		}
		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			for _, call := range calls[current] {
				if call.Kind != string(programindex.RelationCalls) || call.Resolution != "exact" && call.Resolution != "alternatives" {
					continue
				}
				for _, callee := range call.CalleeIDs {
					if call.Resolution == "alternatives" && handlers[callee] || seen[callee] {
						continue
					}
					seen[callee] = true
					queue = append(queue, callee)
				}
			}
		}
		return seen
	}
	roots := append([]string(nil), r.opts.Graph.SeedDecls...)
	for _, place := range r.opts.Graph.Places {
		if place.Symbol == nil {
			continue
		}
		decl := place.Symbol.Decl
		language := strings.ToLower(pathLanguage(place.Path))
		if decl.Kind == "module" && language != "c" || language == "go" && decl.Kind == "function" && decl.Name == "init" {
			roots = append(roots, place.ID)
		}
	}
	launch := walk(roots)
	var handlerRoots []string
	for id := range handlers {
		handlerRoots = append(handlerRoots, id)
	}
	sort.Strings(handlerRoots)
	inside := walk(handlerRoots)
	levels := map[string]string{}
	for id := range calls {
		switch {
		case launch[id]:
			levels[id] = "runs while the program starts, reached from its entry"
		case inside[id]:
			levels[id] = "runs inside the code of an entry already found"
		default:
			levels[id] = "neither"
		}
	}
	return levels
}

// pathLanguage is a source path's language family by its extension.
func pathLanguage(path string) string {
	switch {
	case strings.HasSuffix(path, ".c") || strings.HasSuffix(path, ".h"):
		return "c"
	case strings.HasSuffix(path, ".go"):
		return "go"
	}
	return ""
}

// readCalls asks every word call of the per_call symbols what its words
// become, and keeps each decided answer by its site.
func (r *reader) readCalls(ctx context.Context, symbols map[string]bool) error {
	r.callEnters = map[sourceSite]string{}
	if len(symbols) == 0 {
		return nil
	}
	levels := r.callLevels()
	files := map[string][]byte{}
	var rows []table.Row
	var sites []sourceSite
	subjects := map[string]rowSubject{}
	seen := map[sourceSite]bool{}
	for _, place := range r.opts.Graph.Places {
		if place.Symbol == nil || r.testFile(place.Parent) {
			continue
		}
		decl := place.Symbol.Decl
		var callers []string
		for _, caller := range place.Symbol.CalledBy {
			callers = appendUnique(callers, caller.Name)
		}
		sort.Strings(callers)
		for _, call := range place.Symbol.Calls {
			if call.Kind != string(programindex.RelationInvokesExternal) || call.API == nil || call.Line < 1 || len(call.Values) == 0 {
				continue
			}
			symbol := apiName(*call.API)
			site := sourceSite{place.Path, call.Line, call.Column}
			if !symbols[symbol] || seen[site] {
				continue
			}
			seen[site] = true
			in := decl.Name
			if decl.Signature != "" {
				in += " " + decl.Signature
			}
			fields := []table.Field{{Name: "symbol", Value: symbol}}
			if call.API.Signature != "" {
				fields = append(fields, table.Field{Name: "declared", Value: call.API.Signature})
			}
			if text := r.sourceText(files, place.Path, call.Line, call.Column); text != "" {
				fields = append(fields, table.Field{Name: "call", Value: text})
			}
			fields = append(fields, table.Field{Name: "in", Value: in})
			if len(callers) > 0 {
				fields = append(fields, table.Field{Name: "called_by", Value: callers})
			}
			fields = append(fields, table.Field{Name: "literals", Value: call.Values})
			var arguments []string
			for _, argument := range call.SourceArguments {
				if argument.Origin != nil {
					arguments = append(arguments, argument.Origin.Kind+": "+argument.Origin.Text)
				}
			}
			if len(arguments) > 0 {
				fields = append(fields, table.Field{Name: "arguments", Value: arguments})
			}
			fields = append(fields, table.Field{Name: "level", Value: levels[place.ID]})
			id := fmt.Sprintf("call%d", len(rows)+1)
			subjects[id] = rowSubject{id: fmt.Sprintf("apicall:%s@%s:%d:%d", symbol, place.Path, call.Line, call.Column), path: place.Path, line: call.Line}
			rows = append(rows, table.Row{ID: id, Fields: fields})
			sites = append(sites, site)
		}
	}
	if len(rows) == 0 {
		return nil
	}
	r.opts.Stage(lines.StageAPI, fmt.Sprintf("asking %d calls of %d symbols whose words differ by call what their words become", len(rows), len(symbols)))
	previous := r.rowSubjects
	r.rowSubjects = subjects
	defer func() { r.rowSubjects = previous }()
	answers, err := r.runTable(ctx, lines.APICall(), 4, rows)
	if err != nil {
		return err
	}
	decided := 0
	for i, site := range sites {
		answer := answers[i].answer
		if answer == nil || answer["enters"] == "" {
			continue
		}
		r.callEnters[site] = answer["enters"]
		decided++
	}
	fmt.Fprintf(&r.tables, "atlas_api per call: %d of %d calls decided\n\n", decided, len(rows))
	return nil
}

// entersAt is what the words of the call at a site are for a symbol's role:
// the symbol's entry kind, or for a per_call symbol the call's own answer
// (none is no entry); undecided says a per_call symbol's call had no
// decided answer.
func (r *reader) entersAt(role apiRole, path string, line, column int) (kind string, undecided bool) {
	if !role.perCall {
		return role.enters, false
	}
	answer, ok := r.callEnters[sourceSite{path, line, column}]
	switch {
	case !ok:
		return "", true
	case answer == lines.APINone:
		return "", false
	}
	return answer, false
}
