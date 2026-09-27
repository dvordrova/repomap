package cproject

import (
	"strings"
	"unicode"
)

// markUnreachable proves, for a program with a main, which of its functions
// nothing it runs can reach, and marks them Unreachable.
//
// A C function runs only when code that runs names it: calls it directly, or
// uses its address (stores, passes, returns, compares it, or casts it to an
// integer that may be cast back). So the functions that can run are the ones
// named from where running starts, closed over what their own definitions
// name. Running starts at main; at functions the runtime calls without a
// name in the code (constructor and destructor attributes); at the program's
// file-scope initializers, which exist before main; at an external function
// whose name a platform or package header also declares, which that library
// may call instead of its own; and at a name the implementation reserves
// (__x, _X), which only the implementation calls. A cleanup attribute names
// its function where the variable is declared. Assembly and alias attributes
// name functions by text, and every repository function their text names is
// named there.
//
// A program that looks symbols up by name (dlsym) can reach any function,
// and one whose assembly text cannot be read proves nothing: neither marks
// anything. A library (no main) is called from outside and marks nothing.
func (b *builder) markUnreachable() {
	main := b.mainFunction()
	if main == nil {
		return
	}
	byName := map[string][]string{} // function name -> repository function refs
	for _, fn := range b.functions {
		byName[fn.node.Name] = append(byName[fn.node.Name], fn.ref)
	}
	edges := map[string][]string{}
	roots := []string{main.ref}
	provable := true
	// name visits n and hands every repository function it names to named.
	var visit func(scope *unitScope, n *Node, named func(string))
	visit = func(scope *unitScope, n *Node, named func(string)) {
		if n == nil || !provable {
			return
		}
		switch n.Kind {
		case "DeclRefExpr":
			if ref := n.ReferencedDecl; ref != nil && ref.Kind == "FunctionDecl" {
				if fn := b.repositoryFunction(scope, ref); fn != "" {
					named(fn)
				} else if lookupByName[ref.Name] {
					provable = false
				}
			}
		case "CleanupAttr":
			if ref := n.CleanupFunction; ref != nil {
				if fn := b.repositoryFunction(scope, ref); fn != "" {
					named(fn)
				}
			}
		case "GCCAsmStmt", "MSAsmStmt", "FileScopeAsmDecl", "AliasAttr", "IFuncAttr":
			texts := b.writtenTexts(n)
			if len(texts) == 0 {
				provable = false
				return
			}
			for _, word := range identifiers(strings.Join(texts, " ")) {
				for _, name := range []string{word, strings.TrimPrefix(word, "_")} {
					for _, fn := range byName[name] {
						named(fn)
					}
				}
			}
		}
		for _, child := range n.Inner {
			visit(scope, child, named)
		}
		visit(scope, n.ArrayFiller, named)
	}
	root := func(fn string) { roots = append(roots, fn) }
	for _, scope := range b.scopes {
		for _, node := range scope.unit.Decls {
			if node.Kind != "FunctionDecl" {
				visit(scope, node, root)
				continue
			}
			fn := b.repositoryFunction(scope, &DeclRef{ID: node.ID, Kind: node.Kind, Name: node.Name})
			if fn != "" && !scope.internal[node.Name] && (scope.platform[node.Name] != nil || reservedName(node.Name)) {
				root(fn)
			}
			for _, child := range node.Inner {
				switch child.Kind {
				case "ConstructorAttr", "DestructorAttr":
					if fn != "" {
						root(fn)
					}
				case "CompoundStmt":
					if fn != "" {
						visit(scope, child, func(to string) { edges[fn] = append(edges[fn], to) })
					}
				default:
					visit(scope, child, root)
				}
			}
		}
	}
	if !provable {
		return
	}
	reached := map[string]bool{}
	for len(roots) > 0 {
		fn := roots[len(roots)-1]
		roots = roots[:len(roots)-1]
		if reached[fn] {
			continue
		}
		reached[fn] = true
		roots = append(roots, edges[fn]...)
	}
	for _, fn := range b.functions {
		if !reached[fn.ref] {
			b.objects[fn.ref].Unreachable = true
		}
	}
}

// writtenTexts are the source a node spans where it is spelled and where the
// reader sees it: for an asm statement a macro writes, the macro's body and
// the arguments its use passes.
func (b *builder) writtenTexts(n *Node) []string {
	var texts []string
	for _, pair := range [][2]Position{{n.Begin.Spelling, n.End.Spelling}, {n.Begin.Expansion, n.End.Expansion}} {
		begin, end := pair[0], pair[1]
		if begin.File == "" || begin.File != end.File || end.Offset+end.TokLen < begin.Offset {
			continue
		}
		if data := b.source(begin.File); end.Offset+end.TokLen <= len(data) {
			texts = append(texts, string(data[begin.Offset:end.Offset+end.TokLen]))
		}
	}
	return texts
}

// lookupByName are the platform functions that find a function by its name
// at run time.
var lookupByName = map[string]bool{"dlsym": true, "dlvsym": true, "dlfunc": true}

// reservedName is a name the C standard reserves for the implementation.
func reservedName(name string) bool {
	return strings.HasPrefix(name, "__") || len(name) > 1 && name[0] == '_' && unicode.IsUpper(rune(name[1]))
}

// identifiers are the C identifiers text spells.
func identifiers(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		return !(r == '_' || r == '$' || unicode.IsLetter(r) || unicode.IsDigit(r))
	})
}

// mainFunction is the program's main, or nil for a library.
func (b *builder) mainFunction() *function {
	main := b.parsed.Main
	if main == nil {
		return nil
	}
	for _, fn := range b.functions {
		if fn.node == main.Node || fn.ref == "c:function:@main" && fn.location != nil && fn.location.Path == main.At.File && fn.location.Line == main.At.Line {
			return fn
		}
	}
	return nil
}
