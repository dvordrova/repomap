package cproject

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/dvordrova/repomap/internal/corpus"
)

// firstMentioned returns the first of names that the unit's source spells as
// an identifier.
func firstMentioned(repository *corpus.Corpus, spec UnitSpec, names []string) string {
	if len(names) == 0 {
		return ""
	}
	content, err := repository.ReadFileAll(corpus.FileID(spec.FileRef))
	if err != nil {
		return names[0]
	}
	words := map[string]bool{}
	for _, word := range identifier.FindAll(content.Bytes, -1) {
		words[string(word)] = true
	}
	for _, name := range names {
		if words[name] {
			return name
		}
	}
	return ""
}

var identifier = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)

// symbols are a unit's externally visible definitions and the external names
// it uses without defining them; own are those the repository declares.
type symbols struct {
	defs map[string]definition
	refs []string
	own  map[string]bool
}

type definition struct {
	at        Position
	tentative bool
}

func unitSymbols(unit *Unit) symbols {
	internal := map[string]bool{}
	declared := map[string]string{} // clang id -> name of a file-scope function or variable
	own := map[string]bool{}        // clang ids the repository declares
	for _, node := range unit.Decls {
		if node.Kind == "FunctionDecl" || node.Kind == "VarDecl" {
			declared[node.ID], own[node.ID] = node.Name, true
			if node.StorageClass == "static" {
				internal[node.Name] = true
			}
		}
	}
	for _, external := range unit.External {
		if external.Kind == "FunctionDecl" || external.Kind == "VarDecl" {
			declared[external.ID] = external.Name
		}
	}
	result := symbols{defs: map[string]definition{}, own: map[string]bool{}}
	for _, node := range unit.Decls {
		if internal[node.Name] {
			continue
		}
		switch {
		case node.Kind == "FunctionDecl" && hasBody(node):
			result.defs[node.Name] = definition{at: node.Loc.Expansion}
		case node.Kind == "VarDecl" && node.StorageClass == "":
			if previous, ok := result.defs[node.Name]; !ok || previous.tentative {
				result.defs[node.Name] = definition{at: node.Loc.Expansion, tentative: node.Init == ""}
			}
		}
	}
	// A function or extern variable declared inside a function body names an
	// external definition as a file-scope declaration does.
	var scoped func(node *Node, nested bool)
	scoped = func(node *Node, nested bool) {
		if nested && (node.Kind == "FunctionDecl" || node.Kind == "VarDecl" && node.StorageClass == "extern") {
			declared[node.ID], own[node.ID] = node.Name, true
		}
		for _, child := range node.Inner {
			scoped(child, true)
		}
	}
	for _, node := range unit.Decls {
		scoped(node, false)
	}
	refs := map[string]bool{}
	var walk func(node *Node)
	walk = func(node *Node) {
		if node.Kind == "DeclRefExpr" && node.ReferencedDecl != nil {
			name, ok := declared[node.ReferencedDecl.ID]
			if !ok && node.ReferencedDecl.Kind == "FunctionDecl" {
				// An implicitly declared function (a call with no prototype
				// in scope): clang does not dump its declaration.
				name, ok = node.ReferencedDecl.Name, true
			}
			if ok && !internal[name] {
				if _, defined := result.defs[name]; !defined {
					refs[name] = true
					if own[node.ReferencedDecl.ID] {
						result.own[name] = true
					}
				}
			}
		}
		for _, child := range node.Inner {
			walk(child)
		}
		if node.ArrayFiller != nil {
			walk(node.ArrayFiller)
		}
	}
	for _, node := range unit.Decls {
		walk(node)
	}
	for name := range refs {
		result.refs = append(result.refs, name)
	}
	slices.Sort(result.refs)
	return result
}

// Alternative is a name several units of a program define where the build
// does not say which one it links (C.md "Build description and targets"):
// every definer stays in the program, and a call of the name resolves to
// them as alternatives, as an interface's implementations do (owner,
// 2026-10-02: alternatives are never a reason to fail, never chosen by
// where they are written).
type Alternative struct {
	Name  string   `json:"name"`
	Units []string `json:"units"` // the units defining it, by path, sorted
}

// closure is what linkClosure takes for one program.
type closure struct {
	// units are every unit the program holds, the main unit first;
	// alternative are those it holds only as a name's alternative
	// definition, or for what such a unit needs: no link line of the build
	// says they are linked.
	units       []*Unit
	alternative []*Unit
	// alternatives are the names several of its units define, by name.
	alternatives []Alternative
	// unresolved are the names the repository declares that no taken or
	// pool unit defines, sorted.
	unresolved []string
}

// linkClosure takes the units a linker would take for main from pool: each
// name main's units use and do not define goes to the one unit defining it.
// Of several units defining it, the build decides: the one that shares a
// build output (outputs: a link line's output or an archive taking the unit)
// with the unit needing it, when exactly one does; a unit the build takes is
// never preferred over one it does not. Where the build does not say, and no
// unit the program needs for another name defines it, every definer is kept
// as an alternative: Lua 5.1.5's src/llex.c and etc/noparser.c both define
// luaX_init. Two units the program links (not alternatives) defining one name
// differently fail it.
func linkClosure(main *Unit, pool []*Unit, outputs map[*Unit][]string) (closure, error) {
	table := map[*Unit]symbols{main: unitSymbols(main)}
	for _, unit := range pool {
		table[unit] = unitSymbols(unit)
	}
	var result closure
	inProgram := map[*Unit]bool{}
	alternativeOnly := map[*Unit]bool{}
	defined := map[string]struct {
		definition
		unit *Unit
	}{}
	definers := map[string][]string{}
	alternativeName := func(name string, units ...*Unit) {
		for _, unit := range units {
			if !slices.Contains(definers[name], unit.Path) {
				definers[name] = append(definers[name], unit.Path)
			}
		}
	}
	// A needed name with the unit that needs it: of several units defining
	// it, the one sharing a build output with that unit takes it.
	type need struct {
		name string
		from *Unit
	}
	var queue, deferred []need
	take := func(unit *Unit, alternative bool) error {
		inProgram[unit] = true
		result.units = append(result.units, unit)
		if alternative {
			alternativeOnly[unit] = true
		}
		names := make([]string, 0, len(table[unit].defs))
		for name := range table[unit].defs {
			names = append(names, name)
		}
		slices.Sort(names)
		for _, name := range names {
			def := table[unit].defs[name]
			if previous, ok := defined[name]; ok {
				if !previous.tentative && !def.tentative && previous.at != def.at {
					// An alternative's definition beside another stays an
					// alternative of it; two linked units clashing fail.
					if !alternative && !alternativeOnly[previous.unit] {
						return fmt.Errorf("%s is defined in both %s and %s", name, previous.unit.Path, unit.Path)
					}
					alternativeName(name, previous.unit, unit)
					continue
				}
				if !previous.tentative || def.tentative {
					continue
				}
			}
			defined[name] = struct {
				definition
				unit *Unit
			}{def, unit}
		}
		for _, name := range table[unit].refs {
			queue = append(queue, need{name, unit})
		}
		return nil
	}
	if err := take(main, false); err != nil {
		return closure{}, err
	}
	definersOf := func(name string) []*Unit {
		var candidates, tentative []*Unit
		for _, unit := range pool {
			if def, ok := table[unit].defs[name]; ok && !inProgram[unit] {
				if def.tentative {
					tentative = append(tentative, unit)
				} else {
					candidates = append(candidates, unit)
				}
			}
		}
		if len(candidates) == 0 && len(tentative) == 1 {
			candidates = tentative
		}
		return candidates
	}
	unresolved := map[string]bool{}
	for {
		for len(queue) > 0 {
			name, from := queue[0].name, queue[0].from
			queue = queue[1:]
			if _, ok := defined[name]; ok {
				continue
			}
			candidates := definersOf(name)
			// Units defining one name: the build output that takes the unit
			// needing it and exactly one of them decides (Lua's liblua.a
			// archives src/lstate.c and src/llex.c, not etc/noparser.c).
			if len(candidates) > 1 {
				var linked []*Unit
				for _, unit := range candidates {
					if slices.ContainsFunc(outputs[unit], func(output string) bool { return slices.Contains(outputs[from], output) }) {
						linked = append(linked, unit)
					}
				}
				if len(linked) == 1 {
					candidates = linked
				}
			}
			switch len(candidates) {
			case 0:
				// A platform or package function, or a repository
				// declaration no parsed unit defines.
				for _, unit := range result.units {
					if table[unit].own[name] {
						unresolved[name] = true
					}
				}
			case 1:
				if err := take(candidates[0], alternativeOnly[from]); err != nil {
					return closure{}, err
				}
			default:
				// Undecided until what the program needs for other names
				// is taken: a unit it needs anyway may define it.
				deferred = append(deferred, need{name, from})
			}
		}
		if len(deferred) == 0 {
			break
		}
		pending := deferred
		deferred = nil
		for _, n := range pending {
			if _, ok := defined[n.name]; ok {
				continue
			}
			candidates := definersOf(n.name)
			alternativeName(n.name, candidates...)
			for _, unit := range candidates {
				if err := take(unit, true); err != nil {
					return closure{}, err
				}
			}
		}
	}
	for _, unit := range result.units {
		if alternativeOnly[unit] {
			result.alternative = append(result.alternative, unit)
		}
	}
	for name, paths := range definers {
		slices.Sort(paths)
		result.alternatives = append(result.alternatives, Alternative{Name: name, Units: paths})
	}
	slices.SortFunc(result.alternatives, func(a, b Alternative) int { return strings.Compare(a.Name, b.Name) })
	for name := range unresolved {
		result.unresolved = append(result.unresolved, name)
	}
	slices.Sort(result.unresolved)
	return result, nil
}
