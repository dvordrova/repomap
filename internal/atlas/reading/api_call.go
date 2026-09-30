package reading

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// What the words of one call become (lines.APICall). Every call outside
// tests that gives an outside symbol a word is asked on its own, after its
// symbol's talks answer: strcmp(argv[i], "-h") checks an option where
// strcasecmp(c->name, "monitor") compares the program's own names, and one
// symbol-level answer read from one example call made both the same. A call
// is not asked when its symbol is handed a callable (binds decides what the
// hand-over becomes), when its symbol talks to other programs or serves
// (its words are that program's: one is not both), or when another fact
// already names the call. The answer is remembered per call, by what the
// call shows, never by its line; an undecided call is unsure (launch.go).

// readCalls asks what the words of each word call become and keeps each
// answer by its site: the entry kind, none, or "" for a call asked and not
// decided. talks holds each symbol's decided talks answer.
func (r *reader) readCalls(ctx context.Context, symbols []*apiSymbol, talks map[string]string) error {
	r.callEnters, r.entering = map[sourceSite]string{}, map[string]bool{}
	handlers := r.handlerKinds()
	handed := map[string]bool{}
	for _, s := range symbols {
		handed[s.name] = s.handsCallable
	}
	claims := map[sourceSite][]atlas.Place{}
	for _, place := range r.opts.Graph.Places {
		if place.Kind == atlas.PlaceBoundary && place.Boundary != nil && place.Boundary.Source != "model" {
			line := sourceSite{path: place.Path, line: place.LineNo}
			claims[line] = append(claims[line], place)
		}
	}
	owners := map[sourcevalue.Anchor]string{}
	for _, place := range r.opts.Graph.Places {
		if place.Symbol != nil {
			owners[sourcevalue.Anchor{Path: place.Path, Line: place.LineNo, Column: place.Symbol.Decl.Column}] = place.Symbol.Decl.Name
			if line := (sourcevalue.Anchor{Path: place.Path, Line: place.LineNo}); owners[line] == "" {
				owners[line] = place.Symbol.Decl.Name
			}
		}
	}
	// waiting are the calls comparing a later element of a value than the
	// key compared before them, by their key's site (key_values.go).
	type waitingCall struct {
		key, site sourceSite
		symbol    string
		call      askedCall
	}
	var waiting []waitingCall
	bySymbol := map[string][]askedCall{}
	files := map[string]*lines.CallFile{}
	tables := r.tableReadSites()
	for _, place := range r.opts.Graph.Places {
		if place.Symbol == nil || r.testFile(place.Parent) {
			continue
		}
		decl := place.Symbol.Decl
		for _, call := range place.Symbol.Calls {
			if call.Kind != string(programindex.RelationInvokesExternal) || call.API == nil || call.Line < 1 || len(call.Values) == 0 {
				continue
			}
			symbol := apiName(*call.API)
			role := r.api[symbol]
			site := sourceSite{place.Path, call.Line, call.Column}
			if _, seen := r.callEnters[site]; seen || handed[symbol] || role.talks != "" || role.publishes || claimedByOtherFact(claims[sourceSite{path: site.path, line: site.line}], symbol, site.column) || r.onOwnTable(call, tables) {
				continue
			}
			// Words its handler compares with what it was handed are an
			// entry's sub-arguments, not asked (sub_arguments.go).
			if kind := subArgumentKind(place, call, handlers); kind != "" {
				r.callEnters[site] = kind
				continue
			}
			r.callEnters[site] = ""
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
			fields = append(fields, table.Field{Name: "in", Value: in}, table.Field{Name: "literals", Value: call.Values})
			var arguments []string
			for _, argument := range call.SourceArguments {
				if argument.Origin == nil {
					continue
				}
				at := strconv.Itoa(argument.Position)
				if argument.Keyword != "" {
					at = argument.Keyword
				}
				arguments = append(arguments, at+": "+originText(argument.Origin, owners))
			}
			if len(arguments) > 0 {
				fields = append(fields, table.Field{Name: "arguments", Value: arguments})
			}
			key, keySite := r.keyCall(files, place, call)
			if key != "" {
				fields = append(fields, table.Field{Name: "compared_after", Value: key})
			}
			if answer := talks[symbol]; answer != "" {
				fields = append(fields, table.Field{Name: "talks", Value: answer})
			}
			if key != "" {
				waiting = append(waiting, waitingCall{key: keySite, site: site, symbol: symbol, call: askedCall{site: site, fields: fields}})
				continue
			}
			bySymbol[symbol] = append(bySymbol[symbol], askedCall{site: site, fields: fields})
		}
	}
	if err := r.askCalls(ctx, 3, bySymbol); err != nil {
		return err
	}
	// A value of a key the code compared just before it is that key's
	// entry's value when the key's answer made it an entry (K3), a step
	// already decided: it is not asked. Any other is asked on its own.
	sort.Slice(waiting, func(i, j int) bool { return waiting[i].site.compare(waiting[j].site) < 0 })
	later := map[string][]askedCall{}
	for _, call := range waiting {
		if kind := r.callEnters[call.key]; kind != "" && kind != lines.APINone {
			r.callEnters[call.site], r.entering[call.symbol] = kind, true
			continue
		}
		later[call.symbol] = append(later[call.symbol], call.call)
	}
	return r.askCalls(ctx, 6, later)
}

// askedCall is one word call to ask: its site and its item.
type askedCall struct {
	site   sourceSite
	fields []table.Field
}

// askCalls asks the word calls of each symbol, one round, and keeps each
// answer by its site.
func (r *reader) askCalls(ctx context.Context, round int, bySymbol map[string][]askedCall) error {
	if len(bySymbol) == 0 {
		return nil
	}
	// The calls of one symbol share windows, in source order; the row IDs
	// are request-local and name no call.
	names := make([]string, 0, len(bySymbol))
	for name := range bySymbol {
		names = append(names, name)
	}
	sort.Strings(names)
	var groups rowGroups
	var sites []sourceSite
	var symbolOf []string
	subjects := map[string]rowSubject{}
	for _, name := range names {
		calls := bySymbol[name]
		sort.Slice(calls, func(i, j int) bool { return calls[i].site.compare(calls[j].site) < 0 })
		var rows []table.Row
		for _, call := range calls {
			id := fmt.Sprintf("call%d", len(sites)+1)
			subjects[id] = rowSubject{id: fmt.Sprintf("apicall:%s@%s:%d:%d", name, call.site.path, call.site.line, call.site.column), path: call.site.path, line: call.site.line}
			rows = append(rows, table.Row{ID: id, Fields: call.fields})
			sites = append(sites, call.site)
			symbolOf = append(symbolOf, name)
		}
		groups = append(groups, rowGroup{rows: rows})
	}
	r.opts.Stage(lines.StageAPI, fmt.Sprintf("asking %d calls of %d outside symbols what the words each is given become", len(sites), len(names)))
	previous := r.rowSubjects
	r.rowSubjects = subjects
	defer func() { r.rowSubjects = previous }()
	answers, err := r.runTableGroups(ctx, lines.APICall(), round, groups, nil)
	if err != nil {
		return err
	}
	decided := 0
	for i, site := range sites {
		if answer := answers[i].answer; answer != nil && answer["enters"] != "" {
			r.callEnters[site] = answer["enters"]
			decided++
			if answer["enters"] != lines.APINone {
				r.entering[symbolOf[i]] = true
			}
		}
	}
	fmt.Fprintf(&r.tables, "atlas_api calls: %d of %d calls decided\n\n", decided, len(sites))
	return nil
}

// claimedByOtherFact reports a call another fact boundary already names at
// its column, or at its whole line when the fact has none: a SQL statement
// a query call sends, a setting read, a handed callable. A registration of
// the symbol itself that hands nothing over and has no kind yet is no
// other fact: its words are this call's, and this call's answer decides
// them.
func claimedByOtherFact(facts []atlas.Place, symbol string, column int) bool {
	for _, fact := range facts {
		b := fact.Boundary
		if fact.Column != 0 && fact.Column != column {
			continue
		}
		if b.Source == "fact" && b.External == symbol && b.GivenKind == "" && b.Registrar == nil && !b.Handed && b.Direction != atlas.DirectionIn {
			continue
		}
		return true
	}
	return false
}

// originText says where an argument's value comes from as the code records
// it, in words and without a source position: a word as written, a
// parameter or the receiver of a declaration, a field or an element of
// another value, what a call returns, a join or a choice of values, or the
// code that was not followed.
func originText(value *sourcevalue.Value, owners map[sourcevalue.Anchor]string) string {
	of := func(owner *sourcevalue.Anchor) string {
		if owner == nil {
			return ""
		}
		name := owners[*owner]
		if name == "" {
			name = owners[sourcevalue.Anchor{Path: owner.Path, Line: owner.Line}]
		}
		if name == "" {
			return ""
		}
		return " of " + name
	}
	parts := func(separator string) string {
		texts := make([]string, len(value.Parts))
		for i := range value.Parts {
			texts[i] = originText(&value.Parts[i], owners)
		}
		return strings.Join(texts, separator)
	}
	switch value.Kind {
	case "literal":
		return strconv.Quote(value.Text)
	case "parameter":
		return strings.TrimSpace(fmt.Sprintf("parameter #%d %s", value.Position, value.Text)) + of(value.Owner)
	case "receiver":
		return strings.TrimSpace("receiver "+value.Text) + of(value.Owner)
	case "field":
		return "field " + value.Text + " of " + parts("")
	case "index":
		return "element " + originText(&value.Parts[1], owners) + " of " + originText(&value.Parts[0], owners)
	case "call_result":
		return "result of calling " + value.Text
	case "concat":
		return "joined: " + parts(" + ")
	case "alternatives":
		return "one of: " + parts(" | ")
	case "record":
		return "record {" + parts(", ") + "}"
	case "field_value":
		return value.Text + ": " + parts("")
	}
	if value.Text == "" {
		return "not followed"
	}
	return "not followed: " + value.Text
}

// entersAt is what the words of the call at a site became: the entry kind
// its answer chose, or nothing. undecided says the call was asked and its
// answer was not decided; a call never asked is neither.
func (r *reader) entersAt(path string, line, column int) (kind string, undecided bool) {
	answer, asked := r.callEnters[sourceSite{path, line, column}]
	switch {
	case !asked:
		return "", false
	case answer == "":
		return "", true
	case answer == lines.APINone:
		return "", false
	}
	return answer, false
}

// tableReadSites are where a declaration reads a table the program wrote
// (a variable with rows), by site.
func (r *reader) tableReadSites() map[sourceSite]bool {
	sites := map[sourceSite]bool{}
	for _, place := range r.opts.Graph.Places {
		if place.Symbol == nil || len(place.Symbol.Rows) == 0 {
			continue
		}
		for _, read := range place.Symbol.ReadAt {
			if reader, ok := r.places[read.ReaderID]; ok {
				sites[sourceSite{reader.Path, read.LineNo, read.Column}] = true
			}
		}
	}
	return sites
}

// onOwnTable says a call is made on a table the program wrote: on the
// table read at one of its read sites, an element or a field of it, or
// what an outside call naming nothing made of it (freqtrade's
// options.pop("help") on deepcopy(AVAILABLE_CLI_OPTIONS[val].kwargs)). Its
// words are keys of the code's own data, never what the program is given:
// the call is not asked and makes no entry.
func (r *reader) onOwnTable(call atlas.SymbolCall, tables map[sourceSite]bool) bool {
	sites := r.callSites()
	value := call.ReceiverValue
	seen := map[sourcevalue.Anchor]bool{}
	for value != nil {
		// A value read at a table's read site: the table itself, the base
		// of an element (a table of the same module is anchored at its
		// initializer, the element at the read).
		if value.Anchor != nil && tables[sourceSite{value.Anchor.Path, value.Anchor.Line, value.Anchor.Column}] {
			return true
		}
		switch value.Kind {
		case "field", "index":
			if len(value.Parts) == 0 {
				return false
			}
			value = &value.Parts[0]
			continue
		case "call_result":
			if value.Anchor == nil || seen[*value.Anchor] {
				return false
			}
			seen[*value.Anchor] = true
			producer := sites[sourceSite{value.Anchor.Path, value.Anchor.Line, value.Anchor.Column}]
			if producer == nil || producer.Kind != string(programindex.RelationInvokesExternal) || len(producer.Values) > 0 {
				return false
			}
			// A package's function (copy.deepcopy) makes its value of its
			// first argument; a method of the value it is called on.
			value = producer.ReceiverValue
			if value == nil || producer.API != nil && producer.API.Receiver == "" {
				value = nil
				if len(producer.SourceArguments) > 0 {
					value = producer.SourceArguments[0].Origin
				}
			}
			continue
		}
		return false
	}
	return false
}
