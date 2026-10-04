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
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// Which value of a call names what it reaches (lines.APIArgument). Each
// outside symbol whose talks answer says its calls reach something by what
// they are given (lines.ReachesByArgument: an outgoing kind or a file) is
// asked once which argument of its call, or its receiver, names the
// address or the file; the destination walk then follows that one value at
// every call (DestinationReader). When the value a chosen argument holds
// is what another outside call returned and that symbol has no decided
// argument (http.NewRequest's request, handed to Client.Do), that symbol is
// asked the same question, with the calls its result is given to, and the
// walk goes on through it; the rounds end when a walk meets no symbol not
// yet asked. A symbol answered none, or one whose call gives no argument
// and has no receiver, stops the walk at its call. The decisions are
// remembered per symbol (subject apiargument:<symbol>).

// argumentSubject is the knowledge subject of a symbol's argument row: no
// compact place ID holds a colon, so it names no place.
func argumentSubject(symbol string) string { return "apiargument:" + symbol }

// readArguments asks which argument names what each reaching symbol's calls
// reach, then the symbols whose results those arguments carry, round by
// round, and logs what the file symbols' calls reach.
func (r *reader) readArguments(ctx context.Context, symbols []*apiSymbol, talks map[string]string) error {
	r.arguments = map[string]ArgumentChoice{}
	usage := map[string]sourceSite{}
	for _, s := range symbols {
		usage[s.name] = sourceSite{s.usagePath, s.usageLine, s.usageColumn}
	}
	// Every call outside tests of each outside symbol, in graph order.
	calls := map[string][]destinationCall{}
	for _, place := range r.opts.Graph.Places {
		if place.Symbol == nil || r.testFile(place.Parent) {
			continue
		}
		for _, call := range place.Symbol.Calls {
			if call.Kind == string(programindex.RelationInvokesExternal) && call.API != nil && call.Line > 0 {
				name := apiName(*call.API)
				calls[name] = append(calls[name], destinationCall{place, call})
			}
		}
	}
	var reaching, pending []string
	for _, s := range symbols {
		if lines.ReachesByArgument(talks[s.name]) && len(calls[s.name]) > 0 {
			reaching = append(reaching, s.name)
		}
	}
	pending = slices.Clone(reaching)
	owners := argumentOwners(r.opts.Graph.Places)
	options := r.optionNames()
	files := map[string]*lines.CallFile{}
	asked := map[string]bool{}
	// givenTo are, for a symbol whose result a chosen argument carries, the
	// reaching symbols given it; reached is its first call so reached.
	givenTo := map[string][]string{}
	reached := map[string]destinationCall{}
	decided, total := 0, 0
	for round := 7; len(pending) > 0; round++ {
		sort.Strings(pending)
		var rows []table.Row
		var asking []string
		refs := map[string]map[string]ArgumentChoice{}
		subjects := map[string]rowSubject{}
		for _, name := range pending {
			asked[name] = true
			call, ok := reached[name]
			if !ok {
				call = argumentUsage(calls[name], usage[name])
			}
			arguments, choices := argumentOptions(call.call, owners)
			if len(arguments) == 0 {
				// No argument and no receiver: nothing names what it reaches.
				continue
			}
			id := fmt.Sprintf("arg%d", len(rows)+1)
			fields := []table.Field{{Name: "symbol", Value: name}}
			if call.call.API.Signature != "" {
				fields = append(fields, table.Field{Name: "declared", Value: call.call.API.Signature})
			}
			if text := r.sourceText(files, call.place.Path, call.call.Line, call.call.Column); text != "" {
				fields = append(fields, table.Field{Name: "usage", Value: text})
			}
			if answer := talks[name]; answer != "" {
				fields = append(fields, table.Field{Name: "talks", Value: answer})
			}
			if consumers := givenTo[name]; len(consumers) > 0 {
				fields = append(fields, table.Field{Name: "result_given_to", Value: consumers})
			}
			fields = append(fields, table.Field{Name: "arguments", Value: arguments})
			rows = append(rows, table.Row{ID: id, Fields: fields})
			subjects[id] = rowSubject{id: argumentSubject(name), path: call.place.Path, line: call.call.Line}
			refs[id] = choices
			asking = append(asking, name)
		}
		pending = nil
		if len(rows) > 0 {
			total += len(rows)
			r.opts.Stage(lines.StageAPI, fmt.Sprintf("asking which argument names what the calls of %d outside symbols reach", len(rows)))
			previous := r.rowSubjects
			r.rowSubjects = subjects
			answers, err := r.runTable(ctx, lines.APIArgument(), round, rows)
			r.rowSubjects = previous
			if err != nil {
				return err
			}
			for i, name := range asking {
				answer := answers[i].answer
				if answer == nil || answer["argument"] == "" {
					continue
				}
				decided++
				if answer["argument"] == lines.APINone {
					r.arguments[name] = ArgumentChoice{None: true}
					continue
				}
				if choice, ok := refs[rows[i].ID][answer["argument"]]; ok {
					r.arguments[name] = choice
				}
			}
		}
		// The symbols whose results the decided arguments carry, not yet
		// asked, are the next round's.
		consumer := ""
		tracer := NewDestinationReader(r.opts.Graph.Places, DestinationChoices{Arguments: r.arguments, Options: options, FieldWrites: r.opts.Graph.FieldWrites})
		tracer.undecided = func(symbol string, at atlas.Place, call atlas.SymbolCall) {
			if asked[symbol] || r.testFile(at.Parent) {
				return
			}
			if !slices.Contains(pending, symbol) {
				pending = append(pending, symbol)
				reached[symbol] = destinationCall{at, call}
			}
			if !slices.Contains(givenTo[symbol], consumer) {
				givenTo[symbol] = append(givenTo[symbol], consumer)
				slices.Sort(givenTo[symbol])
			}
		}
		for _, name := range reaching {
			consumer = name + " (" + talks[name] + ")"
			for _, call := range calls[name] {
				tracer.discover(call.place, call.call)
			}
		}
	}
	if total == 0 {
		return nil
	}
	fmt.Fprintf(&r.tables, "atlas_api arguments: %d of %d outside symbols decided which argument names what their calls reach\n\n", decided, total)
	// What each call of a file symbol reaches, as the walk reads it; the
	// files each program keeps are gathered from the same walk (files.go).
	tracer := NewDestinationReader(r.opts.Graph.Places, DestinationChoices{Arguments: r.arguments, Options: options, FieldWrites: r.opts.Graph.FieldWrites})
	for _, name := range reaching {
		if talks[name] != lines.APIFile {
			continue
		}
		for _, call := range calls[name] {
			var reach []string
			for _, use := range tracer.Read(call.place, call.call) {
				switch {
				case use.Address != "":
					reach = append(reach, use.Address)
				case use.Frontier != "":
					reach = append(reach, "{"+use.Frontier+"}")
				}
			}
			fmt.Fprintf(&r.tables, "- atlas_files: %s at %s:%d reaches %s\n", name, call.place.Path, call.call.Line, strings.Join(reach, " | "))
		}
	}
	return nil
}

// argumentUsage is the call a symbol's argument row shows: the one its
// talks row showed, else its first call outside tests.
func argumentUsage(calls []destinationCall, usage sourceSite) destinationCall {
	for _, call := range calls {
		if call.place.Path == usage.path && call.call.Line == usage.line && call.call.Column == usage.column {
			return call
		}
	}
	return calls[0]
}

// argumentOptions are the entries a call's argument row offers, by
// request-local ref: each argument by its position, with the parameter's
// name when the declared type gives it, or by its keyword, and the receiver
// the call is made on, each with where the call's value comes from. An
// argument whose origin the index did not record is not offered.
func argumentOptions(call atlas.SymbolCall, owners map[sourcevalue.Anchor]string) ([]map[string]any, map[string]ArgumentChoice) {
	var names []string
	if call.API != nil {
		names = parameterNames(call.API.Signature)
	}
	arguments := slices.Clone(call.SourceArguments)
	sort.SliceStable(arguments, func(i, j int) bool {
		a, b := arguments[i], arguments[j]
		if (a.Keyword == "") != (b.Keyword == "") {
			return a.Keyword == ""
		}
		if a.Position != b.Position {
			return a.Position < b.Position
		}
		return a.Keyword < b.Keyword
	})
	var entries []map[string]any
	choices := map[string]ArgumentChoice{}
	add := func(ref, title string, origin *sourcevalue.Value, choice ArgumentChoice) {
		if _, seen := choices[ref]; seen {
			return
		}
		choices[ref] = choice
		entries = append(entries, map[string]any{"ref": ref, "title": title, "given": originText(origin, owners)})
	}
	if call.ReceiverValue != nil {
		add("r", "receiver", call.ReceiverValue, ArgumentChoice{Receiver: true})
	}
	keywords := 0
	for _, argument := range arguments {
		if argument.Origin == nil {
			continue
		}
		switch {
		case argument.Keyword != "":
			keywords++
			add(fmt.Sprintf("k%d", keywords), "keyword "+argument.Keyword, argument.Origin, ArgumentChoice{Keyword: argument.Keyword})
		case argument.Position > 0:
			title := fmt.Sprintf("argument %d", argument.Position)
			if argument.Position <= len(names) {
				title += ": " + names[argument.Position-1]
			}
			add(fmt.Sprintf("a%d", argument.Position), title, argument.Origin, ArgumentChoice{Position: argument.Position})
		}
	}
	return entries, choices
}

// argumentOwners names a parameter's or receiver's owner, by the
// declaration's anchor, for originText.
func argumentOwners(places []atlas.Place) map[sourcevalue.Anchor]string {
	owners := map[sourcevalue.Anchor]string{}
	for _, place := range places {
		if place.Symbol == nil {
			continue
		}
		owners[sourcevalue.Anchor{Path: place.Path, Line: place.LineNo, Column: place.Symbol.Decl.Column}] = place.Symbol.Decl.Name
		if line := (sourcevalue.Anchor{Path: place.Path, Line: place.LineNo}); owners[line] == "" {
			owners[line] = place.Symbol.Decl.Name
		}
	}
	return owners
}

// optionNames are, by the site of each call whose words an answer made a
// command-line option, the option's name: its first word that can name
// one. What such a call returns is that option's value.
func (r *reader) optionNames() map[sourcevalue.Anchor]string {
	options := map[sourcevalue.Anchor]string{}
	for _, place := range r.opts.Graph.Places {
		if place.Symbol == nil {
			continue
		}
		for _, call := range place.Symbol.Calls {
			if r.callEnters[sourceSite{place.Path, call.Line, call.Column}] != atlas.BoundaryCommand {
				continue
			}
			if words := lines.NameableWords(call.Values); len(words) > 0 {
				options[sourcevalue.Anchor{Path: place.Path, Line: call.Line, Column: call.Column}] = words[0]
			}
		}
	}
	return options
}
