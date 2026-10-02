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
// may call instead of its own; and at a name the C standard reserves for the
// implementation (any file-scope name that begins with an underscore). A
// cleanup attribute names its function where the variable is declared.
// Assembly, asm labels and alias attributes name functions by text, and every
// repository function their text names is named there; a function whose own
// symbol an asm label renames is called by that name, so it is a root.
//
// Code the adapter does not read can call any function with external linkage
// by its name: an input of the link line no compile line produced (a
// response file too), a library other than the C runtime's own, other
// programs calling a shared object, and code the program loads or looks up
// at run time (dlopen, dlsym). Then every external function is a root and
// only static functions can be proven. A link line that names another entry
// (-e, --entry, -init, -fini, a linker script) or rewrites symbols (--defsym,
// --wrap, -alias), or drops the runtime's start files, and assembly text the
// adapter cannot read, prove nothing. A library (no main) is called from
// outside and marks nothing.
func (b *builder) markUnreachable() {
	main := b.mainFunction()
	if main == nil {
		return
	}
	outside, provable := linkedOutside(b.parsed.Program)
	byName := map[string][]string{} // function name -> repository function refs
	for _, fn := range b.functions {
		byName[fn.node.Name] = append(byName[fn.node.Name], fn.ref)
	}
	edges := map[string][]string{}
	roots := []string{main.ref}
	// visit hands every repository function n names to named.
	var visit func(scope *unitScope, n *Node, named func(string))
	visit = func(scope *unitScope, n *Node, named func(string)) {
		if n == nil || !provable {
			return
		}
		switch n.Kind {
		case "DeclRefExpr":
			if ref := n.ReferencedDecl; ref != nil && ref.Kind == "FunctionDecl" {
				if fns := b.repositoryFunctions(scope, ref); len(fns) > 0 {
					for _, fn := range fns {
						named(fn)
					}
				} else if loadsCode[ref.Name] {
					outside = true
				}
			}
		case "CleanupAttr":
			if ref := n.CleanupFunction; ref != nil {
				for _, fn := range b.repositoryFunctions(scope, ref) {
					named(fn)
				}
			}
		case "GCCAsmStmt", "MSAsmStmt", "FileScopeAsmDecl", "AliasAttr", "IFuncAttr", "AsmLabelAttr":
			texts := b.writtenTexts(n)
			if len(texts) == 0 {
				// A label a platform header writes (and a redeclaration
				// inherits) names a platform symbol, never a repository
				// function; any other unread text proves nothing.
				if n.Kind != "AsmLabelAttr" {
					provable = false
				}
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
	var external []string
	for _, scope := range b.scopes {
		for _, node := range scope.unit.Decls {
			if node.Kind != "FunctionDecl" {
				visit(scope, node, root)
				continue
			}
			fn := b.repositoryFunction(scope, &DeclRef{ID: node.ID, Kind: node.Kind, Name: node.Name})
			if b.alternatives[node.Name] && hasBody(node) {
				// An alternative definition is its own function.
				fn = b.functionRef(scope, node)
			}
			if fn != "" && !scope.internal[node.Name] {
				external = append(external, fn)
				if scope.platform[node.Name] != nil || strings.HasPrefix(node.Name, "_") {
					root(fn)
				}
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
				case "AsmLabelAttr":
					// Code outside the repository calls this function by
					// the symbol its label writes, which no call here names.
					if fn != "" && len(b.writtenTexts(child)) > 0 {
						root(fn)
					}
					visit(scope, child, root)
				default:
					visit(scope, child, root)
				}
			}
		}
	}
	if !provable {
		return
	}
	if outside {
		roots = append(roots, external...)
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

// loadsCode are the platform functions that bring in code at run time, or
// find a function by its name.
var loadsCode = map[string]bool{"dlopen": true, "dlmopen": true, "dlsym": true, "dlvsym": true, "dlfunc": true}

// runtimeLibraries are the C runtime's own libraries. They call a program's
// function only through a pointer it hands them, a name their headers declare
// (malloc) or a name the standard reserves.
var runtimeLibraries = map[string]bool{"c": true, "m": true, "pthread": true, "dl": true, "rt": true}

// linkedOutside reads a program's link line: whether it links code the
// adapter does not read, and whether main is where running starts. Other
// programs call a shared object's external functions by name.
func linkedOutside(program Program) (outside, provable bool) {
	outside, provable = len(program.Missing) > 0 || program.Kind == ProgramShared, true
	args := program.LinkArgs
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-e", arg == "-nostartfiles", arg == "-nostdlib", strings.HasPrefix(arg, "-T"):
			provable = false
		case arg == "-framework":
			outside = true
			i++
		case arg == "-l" && i+1 < len(args):
			outside = outside || !runtimeLibraries[args[i+1]]
			i++
		case strings.HasPrefix(arg, "-l"):
			outside = outside || !runtimeLibraries[strings.TrimPrefix(arg, "-l")]
		case strings.HasPrefix(arg, "-Wl,"):
			for _, word := range strings.Split(strings.TrimPrefix(arg, "-Wl,"), ",") {
				switch {
				case linkerRedirects(word):
					provable = false
				case strings.HasPrefix(word, "-l"):
					outside = outside || !runtimeLibraries[strings.TrimPrefix(word, "-l")]
				}
			}
		}
	}
	return outside, provable
}

// linkerRedirects reports a linker option that makes a function run without
// a name in the code: another entry or initializer (-e, --entry, -init,
// -fini, a linker script's ENTRY), a symbol defined as another (--defsym,
// --wrap, Apple's -alias), or options read from a file.
func linkerRedirects(word string) bool {
	for _, option := range []string{"-e", "--entry", "-init", "-fini", "--defsym", "--wrap", "-alias", "-alias_list", "--script", "--default-script", "-dT"} {
		if word == option || strings.HasPrefix(word, option+"=") {
			return true
		}
	}
	return strings.HasPrefix(word, "-T") || strings.HasPrefix(word, "@")
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
