package reading

import (
	"fmt"
	"go/token"
	"slices"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// Settings a structure declares (owner's decision "a", 2026-09-28): a field
// whose tag names a key (`yaml:"dbs"`), the Go adapter's alias facts, may be
// a key a person writes in the program's configuration file. The tag's key
// names no library: `yaml`, `json`, `toml` are data as written. Each tagged
// field outside tests is asked on its own (lines.Inputs("field")): the field
// and its type, its structure, the tag as written and the structure's use as
// the facts show it (a call outside the repository given a value of it: the
// result a repository call returns of it, by the result's position, the
// receiver of its own method, or a record of its type; the tagged field of
// another structure typed with it). A field answered setting is an entry whose
// handler is not established, declared by its structure and named by the
// keys its tag writes; the structures of one program are one catalogue each,
// declared on the one call that decodes them when the facts name exactly
// one.

// taggedField is one field of a repository type whose tag names a key.
type taggedField struct {
	owner  atlas.Place
	member atlas.TypeMember
	// words are the keys the tag names, distinct, in tag order.
	words []string
	tag   string
}

// taggedFields are the fields outside tests whose tags name keys, by type
// and field order.
func (r *reader) taggedFields() []taggedField {
	var result []taggedField
	for _, place := range r.opts.Graph.Places {
		if place.Symbol == nil || place.Symbol.Decl.Kind != string(programindex.ObjectType) || r.testFile(place.Parent) {
			continue
		}
		for _, member := range place.Symbol.Members {
			if member.Decl.Aliases == "" || member.Decl.LineNo < 1 {
				continue
			}
			field := taggedField{owner: place, member: member}
			var tags []string
			for _, alias := range strings.Fields(member.Decl.Aliases) {
				format, name, ok := strings.Cut(alias, ":")
				if !ok || format == "" || name == "" {
					continue
				}
				tags = append(tags, fmt.Sprintf("%s:%q", format, name))
				if !slices.Contains(field.words, name) {
					field.words = append(field.words, name)
				}
			}
			if len(field.words) == 0 {
				continue
			}
			field.tag = strings.Join(tags, " ")
			result = append(result, field)
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if a.member.Path != b.member.Path {
			return a.member.Path < b.member.Path
		}
		return a.member.Decl.LineNo < b.member.Decl.LineNo || a.member.Decl.LineNo == b.member.Decl.LineNo && a.member.Decl.Column < b.member.Decl.Column
	})
	return result
}

// structureUse is what the facts show a structure is used for: the calls
// outside the repository given a value of it, and the tagged fields of
// other structures typed with it.
type structureUse struct {
	decodes []decodeCall
	within  []string
}

// decodeCall is one call outside the repository given a value of the
// structure, in the declaration that makes it.
type decodeCall struct {
	site   sourceSite
	symbol string
	in     string
}

// structureUses reads, by type place ID, what each tagged structure is used
// for.
func (r *reader) structureUses(fields []taggedField) map[string]*structureUse {
	uses := map[string]*structureUse{}
	byName := map[string][]string{}
	for _, field := range fields {
		if uses[field.owner.ID] == nil {
			uses[field.owner.ID] = &structureUse{}
			name := field.owner.Symbol.Decl.Name
			byName[name] = append(byName[name], field.owner.ID)
		}
	}
	if len(uses) == 0 {
		return uses
	}
	// Nested: the tagged field of another structure typed with it.
	for _, field := range fields {
		for _, name := range typeNames(field.member.Decl.Signature, field.member.Decl.Name) {
			for _, id := range byName[name] {
				if id != field.owner.ID {
					line := fmt.Sprintf("the type of field %s (%s) of %s", field.member.Decl.Name, field.tag, field.owner.Symbol.Decl.Name)
					if !slices.Contains(uses[id].within, line) {
						uses[id].within = append(uses[id].within, line)
					}
				}
			}
		}
	}
	sites := r.callSites()
	// returns are the type names of what the repository call at site
	// returns: the result at position, from one, or every result.
	returns := func(site sourceSite, position int) []string {
		call := sites[site]
		if call == nil || call.Kind != string(programindex.RelationCalls) {
			return nil
		}
		var names []string
		for _, id := range call.CalleeIDs {
			if callee := r.places[id]; callee.Symbol != nil {
				results := resultTypes(resultsOf(callee.Symbol.Decl.Signature))
				switch {
				case position == 0:
					for _, result := range results {
						names = append(names, typeNames(result, "")...)
					}
				case position <= len(results):
					names = append(names, typeNames(results[position-1], "")...)
				}
			}
		}
		return names
	}
	for _, place := range r.opts.Graph.Places {
		if place.Symbol == nil || r.testFile(place.Parent) {
			continue
		}
		decl := place.Symbol.Decl
		for _, call := range place.Symbol.Calls {
			if call.Kind != string(programindex.RelationInvokesExternal) || call.API == nil || call.Line < 1 {
				continue
			}
			var named []string
			var visit func(value *sourcevalue.Value)
			visit = func(value *sourcevalue.Value) {
				if value == nil {
					return
				}
				switch value.Kind {
				case "call_result":
					if value.Anchor != nil {
						named = append(named, returns(sourceSite{value.Anchor.Path, value.Anchor.Line, value.Anchor.Column}, value.Position)...)
					}
				case "receiver":
					if owner, _, method := strings.Cut(decl.Name, "."); method {
						named = append(named, owner)
					}
				case "record":
					if value.Type != "" {
						named = append(named, value.Type)
					}
				case "alternatives":
					for i := range value.Parts {
						visit(&value.Parts[i])
					}
				}
			}
			for _, argument := range call.SourceArguments {
				visit(argument.Origin)
			}
			site := sourceSite{place.Path, call.Line, call.Column}
			for _, name := range named {
				for _, id := range byName[name] {
					if !slices.ContainsFunc(uses[id].decodes, func(d decodeCall) bool { return d.site == site }) {
						uses[id].decodes = append(uses[id].decodes, decodeCall{site: site, symbol: apiName(*call.API), in: decl.Name})
					}
				}
			}
		}
	}
	for _, use := range uses {
		sort.Slice(use.decodes, func(i, j int) bool { return use.decodes[i].site.compare(use.decodes[j].site) < 0 })
	}
	return uses
}

// resultsOf is the results part of a callable's signature, "func(r
// io.Reader) (Config, error)" → "(Config, error)"; empty when it has none.
func resultsOf(signature string) string {
	start := strings.Index(signature, "(")
	if start < 0 {
		return ""
	}
	depth := 0
	for i := start; i < len(signature); i++ {
		switch signature[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return signature[i+1:]
			}
		}
	}
	return ""
}

// resultTypes are the types of a results part, one per result: "(_ Config,
// err error)" → "Config", "error"; "(a, b Config)" → "Config", "Config";
// " Config" → "Config".
func resultTypes(results string) []string {
	results = strings.TrimSpace(results)
	if !strings.HasPrefix(results, "(") || !strings.HasSuffix(results, ")") {
		if results == "" {
			return nil
		}
		return []string{results}
	}
	var parts []string
	depth, start := 0, 1
	for i := 1; i < len(results)-1; i++ {
		switch results[i] {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(results[start:i]))
				start = i + 1
			}
		}
	}
	parts = append(parts, strings.TrimSpace(results[start:len(results)-1]))
	// Named results write "name type"; a name alone takes the type of the
	// next result that writes one.
	named := slices.ContainsFunc(parts, func(part string) bool {
		name, _, found := strings.Cut(part, " ")
		return found && token.IsIdentifier(name) && !token.IsKeyword(name)
	})
	if !named {
		return parts
	}
	types := make([]string, len(parts))
	for i := len(parts) - 1; i >= 0; i-- {
		if _, kind, found := strings.Cut(parts[i], " "); found {
			types[i] = strings.TrimSpace(kind)
		} else if i+1 < len(parts) {
			types[i] = types[i+1]
		}
	}
	return types
}

// typeNames are the type names a type expression writes, without package
// qualifiers, pointers or containers: "[]*litestream.DBConfig" names
// DBConfig. skip is a name that is no type (a field's own name leading its
// signature).
func typeNames(text, skip string) []string {
	var names []string
	for _, word := range strings.FieldsFunc(text, func(r rune) bool {
		return !(r == '_' || r == '.' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r > 127)
	}) {
		if at := strings.LastIndex(word, "."); at >= 0 {
			word = word[at+1:]
		}
		if word != "" && word != skip && !slices.Contains(names, word) {
			names = append(names, word)
		}
	}
	return names
}

// fieldRows are the field question's rows: one per tagged field.
func (r *reader) fieldRows(fields []taggedField) ([]table.Row, map[string]rowSubject) {
	uses := r.structureUses(fields)
	files := map[string]*lines.CallFile{}
	rows := make([]table.Row, 0, len(fields))
	subjects := map[string]rowSubject{}
	for _, field := range fields {
		decl := field.member.Decl
		text := decl.Signature
		if text == "" {
			text = decl.Name
		}
		owner := field.owner.Symbol.Decl
		structure := owner.Name + ", declared in " + field.owner.Path
		if owner.Signature != "" && owner.Signature != "struct" {
			structure += " as " + owner.Signature
		}
		item := []table.Field{{Name: "field", Value: text}, {Name: "structure", Value: structure}, {Name: "tag", Value: field.tag}}
		var used []string
		if use := uses[field.owner.ID]; use != nil {
			for _, decode := range use.decodes {
				line := "given to " + decode.symbol + " in " + decode.in
				if call := r.sourceText(files, decode.site.path, decode.site.line, decode.site.column); call != "" {
					line += ": " + call
				}
				used = append(used, line)
			}
			used = append(used, use.within...)
		}
		if len(used) > 0 {
			item = append(item, table.Field{Name: "structure_use", Value: used})
		}
		id := fmt.Sprintf("field%d", len(rows)+1)
		subjects[id] = rowSubject{id: "field:" + field.owner.ID + "." + decl.Name, path: field.member.Path, line: decl.LineNo}
		rows = append(rows, table.Row{ID: id, Fields: item})
	}
	return rows, subjects
}

// fieldKey is the field question's answer key. The answers are kept beside
// the tables' in tableKinds, whose keys are place IDs and never hold a NUL.
func fieldKey(field taggedField) string {
	return "field\x00" + field.owner.ID + "\x00" + field.member.Decl.Name
}

// bindSettingFields makes each field answered setting an entry whose
// handler is not established, at the field, declared by its structure,
// named by its tag's keys, on the one call that decodes its structure when
// the facts name exactly one.
func (r *reader) bindSettingFields() {
	if len(r.tableKinds) == 0 {
		return
	}
	if r.boundaryIDs == nil {
		r.boundaryIDs = make(map[string]string)
	}
	fields := r.taggedFields()
	uses := r.structureUses(fields)
	files := map[string]*lines.CallFile{}
	for _, field := range fields {
		if r.tableKinds[fieldKey(field)] != atlas.BoundarySetting {
			continue
		}
		owner := field.owner
		targets := runningTargets(owner)
		if len(targets) == 0 {
			targets = owner.TargetIDs
		}
		parent := owner.Parent
		if field.member.Path != owner.Path {
			parent = ""
			for _, place := range r.opts.Graph.Places {
				if place.Kind == atlas.PlaceFile && place.Path == field.member.Path {
					parent = place.ID
					break
				}
			}
		}
		decl := field.member.Decl
		source := fmt.Sprintf("%s\x00field\x00%s\x00%s", atlas.DirectionIn, owner.ID, decl.Name)
		id := r.boundaryIDs[source]
		if id == "" {
			id = r.compactID("b", &r.nextBoundary)
			r.boundaryIDs[source] = id
		}
		state := &boundaryState{kind: atlas.BoundarySetting, handlerUnknown: true, place: atlas.Place{
			ID: id, Kind: atlas.PlaceBoundary, Path: field.member.Path, LineNo: decl.LineNo, Column: decl.Column,
			Parent: parent, TargetIDs: slices.Clone(targets), Boundary: &atlas.BoundaryFacts{
				Source: "model", ObjectID: owner.Symbol.Decl.ObjectID, Caller: owner.Symbol.Decl.Name,
				Values: slices.Clone(field.words), Words: slices.Clone(field.words), Direction: atlas.DirectionIn, GivenKind: atlas.BoundarySetting}}}
		if use := uses[owner.ID]; use != nil && len(use.decodes) == 1 {
			decode := use.decodes[0].site
			state.on = &atlas.DeclaredOn{Path: decode.path, LineNo: decode.line, Column: decode.column, Text: r.sourceText(files, decode.path, decode.line, decode.column)}
		}
		r.boundaries[id] = state
	}
}
