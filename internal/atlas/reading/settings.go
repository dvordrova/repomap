package reading

import (
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

// Settings a structure declares (owner's decision "a", 2026-09-28): a field
// whose tag names a key (`yaml:"dbs"`), the Go adapter's alias facts, may be
// a key a person writes in the program's configuration file. The tag's key
// names no library: `yaml`, `json`, `toml` are data as written. Each tagged
// field outside tests is asked on its own (lines.Inputs("field")): the field
// and its type, its structure, the tag as written and the structure's use as
// the facts show it (a call outside the repository given a value of it, by
// the static type of the value an argument is given; the tagged field of
// another structure typed with it, with that structure's own use). A field
// answered setting is an entry whose
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
// outside the repository given a value of it, and the fields of other
// structures typed with it.
type structureUse struct {
	decodes []decodeCall
	within  []parentField
}

// decodeCall is one call outside the repository given a value of the
// structure, in the declaration that makes it.
type decodeCall struct {
	site   sourceSite
	symbol string
	in     string
}

// parentField is a field of another structure whose type names this one,
// with its tag as the question writes it when it names a key.
type parentField struct {
	owner  atlas.Place
	member atlas.TypeMember
	tag    string
}

// structureUses reads, by type place ID, what each tagged structure is used
// for. A call gives a value of a structure when the static type of an
// argument's value names it (`&config` of type *Config, `[]DatabaseInfo`);
// a field is typed with it when its declared type names it. Both are the
// adapter's facts, where the named types are declared (Value.Types,
// Decl.Types), never a name matched as text.
func (r *reader) structureUses(fields []taggedField) map[string]*structureUse {
	uses := map[string]*structureUse{}
	tags := map[string]string{}
	for _, field := range fields {
		if uses[field.owner.ID] == nil {
			uses[field.owner.ID] = &structureUse{}
		}
		tags[field.owner.ID+"\x00"+field.member.Decl.Name] = field.tag
	}
	if len(uses) == 0 {
		return uses
	}
	typeAt := map[sourcevalue.Anchor]string{}
	for _, place := range r.opts.Graph.Places {
		if uses[place.ID] != nil {
			typeAt[sourcevalue.Anchor{Path: place.Path, Line: place.LineNo, Column: place.Column}] = place.ID
		}
	}
	for _, place := range r.opts.Graph.Places {
		if uses[place.ID] == nil {
			continue
		}
		for _, member := range place.Symbol.Members {
			for _, declared := range member.Decl.Types {
				if id := typeAt[declared]; id != "" && id != place.ID {
					uses[id].within = append(uses[id].within, parentField{owner: place, member: member, tag: tags[place.ID+"\x00"+member.Decl.Name]})
				}
			}
		}
	}
	for _, place := range r.opts.Graph.Places {
		if place.Symbol == nil || r.testFile(place.Parent) {
			continue
		}
		for _, call := range place.Symbol.Calls {
			if call.Kind != string(programindex.RelationInvokesExternal) || call.API == nil || call.Line < 1 {
				continue
			}
			site := sourceSite{place.Path, call.Line, call.Column}
			for _, argument := range call.SourceArguments {
				if argument.Origin == nil {
					continue
				}
				for _, declared := range argument.Origin.Types {
					id := typeAt[declared]
					if id != "" && !slices.ContainsFunc(uses[id].decodes, func(d decodeCall) bool { return d.site == site }) {
						uses[id].decodes = append(uses[id].decodes, decodeCall{site: site, symbol: apiName(*call.API), in: place.Symbol.Decl.Name})
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

// useLines are a structure's uses as the field question words them: each
// call given a value of it, and each field of another structure typed with
// it followed by that structure's own uses ("the type of field DBs
// (yaml:"dbs") of Config, which is given to …"), each line once. seen holds
// the structures already on the way, so a structure nested in itself ends.
func (r *reader) useLines(uses map[string]*structureUse, id string, seen map[string]bool, files map[string]*lines.CallFile) []string {
	use := uses[id]
	if use == nil || seen[id] {
		return nil
	}
	seen[id] = true
	defer delete(seen, id)
	var result []string
	add := func(line string) {
		if !slices.Contains(result, line) {
			result = append(result, line)
		}
	}
	for _, decode := range use.decodes {
		line := "given to " + decode.symbol + " in " + decode.in
		if call := r.sourceText(files, decode.site.path, decode.site.line, decode.site.column); call != "" {
			line += ": " + call
		}
		add(line)
	}
	for _, parent := range use.within {
		line := "the type of field " + parent.member.Decl.Name
		if parent.tag != "" {
			line += " (" + parent.tag + ")"
		}
		line += " of " + parent.owner.Symbol.Decl.Name
		above := r.useLines(uses, parent.owner.ID, seen, files)
		if len(above) == 0 {
			add(line)
		}
		for _, next := range above {
			add(line + ", which is " + next)
		}
	}
	return result
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
		if used := r.useLines(uses, field.owner.ID, map[string]bool{}, files); len(used) > 0 {
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
	fields := r.taggedFields()
	uses := r.structureUses(fields)
	files := map[string]*lines.CallFile{}
	for _, field := range fields {
		if r.tableKinds[fieldKey(field)] != atlas.BoundarySetting {
			continue
		}
		owner := field.owner
		targets := runningTargets(owner)
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
			state.on = r.declaredOnSite(files, decode.path, decode.line, decode.column)
		}
		r.boundaries[id] = state
	}
}
