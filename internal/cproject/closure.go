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

// linkClosure takes the units a linker would take for main from pool: each
// name main's units use and do not define goes to the one unit defining it.
// Two units defining a needed name, or a unit whose definition clashes with
// one already taken, fail the program.
//
// It also returns the names the repository declares that no taken or pool
// unit defines, sorted.
func linkClosure(main *Unit, pool []*Unit) ([]*Unit, []string, error) {
	table := map[*Unit]symbols{main: unitSymbols(main)}
	for _, unit := range pool {
		table[unit] = unitSymbols(unit)
	}
	taken := []*Unit{}
	inProgram := map[*Unit]bool{}
	defined := map[string]struct {
		definition
		unit *Unit
	}{}
	var queue []string
	take := func(unit *Unit) error {
		inProgram[unit] = true
		taken = append(taken, unit)
		names := make([]string, 0, len(table[unit].defs))
		for name := range table[unit].defs {
			names = append(names, name)
		}
		slices.Sort(names)
		for _, name := range names {
			def := table[unit].defs[name]
			if previous, ok := defined[name]; ok {
				if !previous.tentative && !def.tentative && previous.at != def.at {
					return fmt.Errorf("%s is defined in both %s and %s", name, previous.unit.Path, unit.Path)
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
		queue = append(queue, table[unit].refs...)
		return nil
	}
	if err := take(main); err != nil {
		return nil, nil, err
	}
	unresolved := map[string]bool{}
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		if _, ok := defined[name]; ok {
			continue
		}
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
		switch len(candidates) {
		case 0:
			// A platform or package function, or a repository declaration
			// no parsed unit defines.
			for _, unit := range taken {
				if table[unit].own[name] {
					unresolved[name] = true
				}
			}
		case 1:
			if err := take(candidates[0]); err != nil {
				return nil, nil, err
			}
		default:
			var paths []string
			for _, unit := range candidates {
				paths = append(paths, unit.Path)
			}
			return nil, nil, fmt.Errorf("%s is defined in several units: %s", name, strings.Join(paths, ", "))
		}
	}
	names := make([]string, 0, len(unresolved))
	for name := range unresolved {
		names = append(names, name)
	}
	slices.Sort(names)
	return taken, names, nil
}
