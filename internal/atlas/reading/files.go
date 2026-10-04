package reading

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/programindex"
)

// The files a program keeps are its data (owner, 2026-09-29): each file its
// own code reaches by a path, with the calls reaching it. Which outside
// symbols reach a file (`talks` file) and which of a call's values names the
// path (the argument decision) are the model's answers, already given; the
// rest is code. Every call outside tests of a file symbol is walked along
// its decided argument (DestinationReader) to where the path ends, and the
// calls are grouped by that end:
//
//   - a literal or a template as the walk writes it, each unresolved part by its plainest word (`dump.kv`,
//     `{path}-wal`, `{--config}`, `{env:KVD_CONFIG}`) is one file;
//   - a field the walk cannot follow further (`server.dbfile`, a field of a
//     file-scope variable whose accesses the program records by that path)
//     is one file, whose values are the field's writes in the program, each
//     walked from the value it stores (ProgramIndex Relation.Value): kvd's
//     main stores "dump.kv", its configuration reader what strdup returns,
//     which is not established;
//   - anything else is a path not established, one per function making the
//     calls: it is never invented.
//
// No role is asked or given (skeptic, 2026-09-29): the page shows the path
// as written and the functions reaching it.

// FileReader derives the files of each program from one walk of the graph.
type FileReader struct {
	places  []atlas.Place
	tracer  *DestinationReader
	symbols map[string]bool
	skip    func(atlas.Place) bool
	// fields are, by the path the code reaches a field by, every recorded
	// access of it: the walk ending at such a path ends at the field, and
	// its writes are its values.
	fields map[string][]fileFieldAccess
}

// fileFieldAccess is one read or write of a field by a declaration.
type fileFieldAccess struct {
	place atlas.Place
	field atlas.SymbolField
}

// NewFileReader reads the calls of the file symbols (by outside symbol,
// package.Receiver.Name) with the reading's decided arguments and options.
// skip leaves out a declaration's calls and writes: the reading's test code.
func NewFileReader(places []atlas.Place, symbols []string, choices DestinationChoices, skip func(atlas.Place) bool) *FileReader {
	reader := &FileReader{places: places, tracer: NewDestinationReader(places, choices), symbols: map[string]bool{}, skip: skip, fields: map[string][]fileFieldAccess{}}
	for _, symbol := range symbols {
		reader.symbols[symbol] = true
	}
	for _, place := range places {
		if place.Symbol == nil || skip != nil && skip(place) {
			continue
		}
		for _, field := range place.Symbol.Fields {
			reader.fields[field.Path] = append(reader.fields[field.Path], fileFieldAccess{place, field})
		}
	}
	return reader
}

// file is one file being gathered: its key (where its path ends) and what
// the record says.
type dataFile struct {
	key   string
	data  facts.DataObject
	calls map[facts.DataCall]bool
}

// Files are the files one program's code reaches, as data records w1, w2, …
// in the order of their first calls.
func (reader *FileReader) Files(targetID string) []atlas.DataRecord {
	if len(reader.symbols) == 0 {
		return nil
	}
	files := map[string]*dataFile{}
	gather := func(key, name, field string) *dataFile {
		if files[key] == nil {
			files[key] = &dataFile{key: key, data: facts.DataObject{Kind: "file", Origin: "call", Scope: targetID, Name: name, File: &facts.DataFile{Field: field}}, calls: map[facts.DataCall]bool{}}
			if field != "" {
				files[key].data.File.Values = reader.values(targetID, field)
			}
		}
		return files[key]
	}
	for _, place := range reader.places {
		if place.Symbol == nil || !contains(place.TargetIDs, targetID) || reader.skip != nil && reader.skip(place) {
			continue
		}
		for _, call := range place.Symbol.Calls {
			if call.Kind != string(programindex.RelationInvokesExternal) || call.API == nil || call.Line < 1 || !reader.symbols[apiName(*call.API)] {
				continue
			}
			site := facts.DataCall{Symbol: apiName(*call.API), ObjectID: place.Symbol.Decl.ObjectID, Anchor: facts.Anchor{Path: place.Path, Line: call.Line, Column: call.Column}}
			for _, use := range reader.tracer.Read(place, call) {
				if !contains(use.TargetIDs, targetID) {
					continue
				}
				var found *dataFile
				switch path, stored := strings.CutPrefix(use.Frontier, "initializer: "); {
				case nullPath(use.Address):
					// None, nil or null names no file.
					continue
				case use.Address != "":
					found = gather("path\x00"+use.Address, use.Address, "")
				case stored && path != "" && plainTemplate(path) != "":
					// A field whose receiver the walk cannot follow holds
					// what its one store, or each store, puts in it
					// (DestinationReader "initializer").
					found = gather("path\x00"+plainTemplate(path), plainTemplate(path), "")
				case use.Frontier != "" && len(reader.fields[use.Frontier]) > 0:
					found = gather("field\x00"+use.Frontier, "{"+use.Frontier+"}", use.Frontier)
				case writtenTemplate(use.Frontier) && plainTemplate(use.Frontier) != "":
					// A template the walk wrote around a part it could not
					// resolve is one file, the part read by its plainest
					// word: `{path}-wal`. Templates alike once read so are
					// one file, each site one of its calls.
					found = gather("path\x00"+plainTemplate(use.Frontier), plainTemplate(use.Frontier), "")
				default:
					found = gather("unknown\x00"+place.ID, "", "")
				}
				found.calls[site] = true
			}
		}
	}
	gathered := make([]*dataFile, 0, len(files))
	for _, found := range files {
		for call := range found.calls {
			found.data.File.Calls = append(found.data.File.Calls, call)
		}
		slices.SortFunc(found.data.File.Calls, func(a, b facts.DataCall) int {
			return cmp.Or(anchorCompare(a.Anchor, b.Anchor), strings.Compare(a.Symbol, b.Symbol))
		})
		gathered = append(gathered, found)
	}
	slices.SortFunc(gathered, func(a, b *dataFile) int {
		return cmp.Or(anchorCompare(a.data.File.Calls[0].Anchor, b.data.File.Calls[0].Anchor), strings.Compare(a.key, b.key))
	})
	records := make([]atlas.DataRecord, 0, len(gathered))
	for i, found := range gathered {
		first := found.data.File.Calls[0].Anchor
		data := found.data
		records = append(records, atlas.DataRecord{ID: fmt.Sprintf("w%d", i+1), Path: first.Path, Line: first.Line, Data: &data})
	}
	return records
}

// writtenTemplate says a walk's end is a template it wrote (concat): literal
// text beside a part it could not resolve, in braces.
func writtenTemplate(frontier string) bool {
	open := strings.IndexByte(frontier, '{')
	closing := strings.LastIndexByte(frontier, '}')
	return open >= 0 && closing > open && (open > 0 || closing < len(frontier)-1)
}

// nullPath reports a path whose value is the language's null: Python's
// None, Go's and Clojure's nil, JS's null or undefined. It names no file
// (freqtrade's create_datadir's datadir=None had stood as a file "None").
func nullPath(address string) bool {
	switch address {
	case "None", "nil", "null", "undefined":
		return true
	}
	return false
}

// plainTemplate is a path template with each part the walk could not
// resolve read by the plainest word it gives (templateWord): its last field
// or variable, or a key it is looked up by; a setting's part (`{--config}`,
// `{env:API}`) stays. Never an internal expression, a type path or a
// diagnostic phrase: litestream's
// `{Clone[[]*…/litestream.DB …]()[?].metaPath}.tmp`, `{write not
// established before read.metaPath}.tmp` and `{db.metaPath}.tmp` are all
// `{metaPath}.tmp`. "" when a part gives no word: the path is not
// established.
func plainTemplate(template string) string {
	var out strings.Builder
	for len(template) > 0 {
		open := strings.IndexByte(template, '{')
		if open < 0 {
			out.WriteString(template)
			break
		}
		out.WriteString(template[:open])
		depth, closing := 0, -1
		for i := open; i < len(template) && closing < 0; i++ {
			switch template[i] {
			case '{':
				depth++
			case '}':
				if depth--; depth == 0 {
					closing = i
				}
			}
		}
		if closing < 0 {
			out.WriteString(template[open:])
			break
		}
		part := template[open+1 : closing]
		if !strings.HasPrefix(part, "--") && !strings.HasPrefix(part, "env:") {
			if part = templateWord(part); part == "" {
				return ""
			}
		}
		out.WriteString("{" + part + "}")
		template = template[closing+1:]
	}
	return out.String()
}

// templateWord is the plainest word an expression gives: from its last
// segment back, a field or variable name, or a key it indexes by as a
// quoted word; a call's name and an unknown ("?", a diagnostic phrase)
// give none of their own. "" when none does.
func templateWord(expression string) string {
	identifier := func(text string) bool {
		if text == "" {
			return false
		}
		for i, r := range text {
			if !(r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || i > 0 && r >= '0' && r <= '9') {
				return false
			}
		}
		return true
	}
	// The top-level segments, split at the dots outside brackets, quotes
	// and parentheses.
	var segments []string
	depth, quote, start := 0, byte(0), 0
	for i := 0; i < len(expression); i++ {
		c := expression[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '[' || c == '(':
			depth++
		case c == ']' || c == ')':
			depth--
		case c == '.' && depth == 0:
			segments = append(segments, expression[start:i])
			start = i + 1
		}
	}
	segments = append(segments, expression[start:])
	for i := len(segments) - 1; i >= 0; i-- {
		segment, called := strings.TrimSpace(segments[i]), false
		for strings.HasSuffix(segment, "]") || strings.HasSuffix(segment, ")") {
			closing := segment[len(segment)-1]
			opening := byte('[')
			if closing == ')' {
				opening, called = '(', true
			}
			depth, at := 0, -1
			for j := len(segment) - 1; j >= 0 && at < 0; j-- {
				switch segment[j] {
				case closing:
					depth++
				case opening:
					if depth--; depth == 0 {
						at = j
					}
				}
			}
			if at < 0 {
				break
			}
			inner := segment[at+1 : len(segment)-1]
			if closing == ']' && len(inner) > 2 && (inner[0] == '"' || inner[0] == '\'') && inner[len(inner)-1] == inner[0] {
				if key := inner[1 : len(inner)-1]; identifier(key) {
					return key
				}
			}
			segment = segment[:at]
		}
		if identifier(segment) && !called {
			return segment
		}
	}
	return ""
}

// values are the writes of a field in one program, in site order, each with
// the path its stored value walks to: a literal or a template, the value a
// field's store holds, or nothing established. A write the adapter recorded
// no value for (Go's, a compound assignment) establishes none.
func (reader *FileReader) values(targetID, field string) []facts.DataValue {
	var values []facts.DataValue
	seen := map[facts.DataValue]bool{}
	add := func(value facts.DataValue) {
		if !seen[value] {
			seen[value] = true
			values = append(values, value)
		}
	}
	for _, access := range reader.fields[field] {
		if access.field.Kind != "writes" || !contains(runningTargets(access.place), targetID) {
			continue
		}
		at := facts.DataValue{ObjectID: access.place.Symbol.Decl.ObjectID, Anchor: facts.Anchor{Path: access.place.Path, Line: access.field.LineNo, Column: access.field.Column}}
		if access.field.Value == nil {
			add(at)
			continue
		}
		start := destinationPath{DestinationUse: atlas.DestinationUse{TargetIDs: runningTargets(access.place)}, choices: map[string]int{}}
		walked := false
		for _, use := range reader.tracer.value(access.field.Value, access.place, start, map[string]bool{}) {
			if !contains(use.TargetIDs, targetID) {
				continue
			}
			value := at
			if path, stored := strings.CutPrefix(use.Frontier, "initializer: "); use.Address != "" && !nullPath(use.Address) {
				value.Value = use.Address
			} else if stored {
				value.Value = plainTemplate(path)
			}
			add(value)
			walked = true
		}
		// A write whose value no walk of this program reads is still one.
		if !walked {
			add(at)
		}
	}
	slices.SortStableFunc(values, func(a, b facts.DataValue) int {
		return cmp.Or(anchorCompare(a.Anchor, b.Anchor), strings.Compare(a.Value, b.Value))
	})
	return values
}

func anchorCompare(a, b facts.Anchor) int {
	return cmp.Or(strings.Compare(a.Path, b.Path), cmp.Compare(a.Line, b.Line), cmp.Compare(a.Column, b.Column))
}

// fileSymbols are the outside symbols whose talks answer is file.
func (r *reader) fileSymbols() []string {
	var symbols []string
	for name, role := range r.api {
		if role.talks == lines.APIFile {
			symbols = append(symbols, name)
		}
	}
	slices.Sort(symbols)
	return symbols
}

// filesForTarget are one program's files (FileReader), read with the
// reading's decisions; the walk is built once for every program.
func (r *reader) filesForTarget(id string) []atlas.DataRecord {
	if r.fileReader == nil {
		r.fileReader = NewFileReader(r.opts.Graph.Places, r.fileSymbols(), DestinationChoices{Arguments: r.arguments, Options: r.optionNames(), FieldWrites: r.opts.Graph.FieldWrites},
			func(place atlas.Place) bool { return r.testFile(place.Parent) })
	}
	return r.fileReader.Files(id)
}
