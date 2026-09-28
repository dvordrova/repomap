package cproject

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/dependencies"
	p "github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// Result is one C program as the ordinary pipeline reads it: its ProgramIndex
// input, its dependency catalog, and the platform view it was read in.
// Outside are the included .c files this platform's build does not enter.
type Result struct {
	Program      Program
	Toolchain    Toolchain
	Outside      []string
	Input        p.Input
	Dependencies dependencies.Catalog
}

// Index projects one parsed program into ProgramIndex input. Identities follow
// the linker: a name with external linkage is one object per program, a static
// function, variable or type is its definition location, so a header's static
// inline function or a .c file several units include is one object. Two
// external definitions of one name fail the program.
//
// Direct calls resolve across units; a call of a platform or package
// function is invokes_external. A function passed as an argument is
// passes_callback bound to that argument. A function stored into a field, a
// variable or an array (initializer rows, assignments, and a parameter a
// function stores, joined with what its callers pass) is passes_callback with
// a c_function_pointer_store witness; a row of a module-level table that names
// the function by a string literal is also a construct of the row's record
// with the literal beside the function, the owner's registration shape. A
// call through a field, parameter or variable has the stored functions as its
// targets, unless a store depends on a branch or on a value the index cannot
// name (another pointer, a call's result, a slot whose address is handed on,
// a parameter of a function that is itself used as a value): then it is
// unresolved and the candidates are its witnesses.
func Index(repository *corpus.Corpus, parsed *Parsed) (*Result, error) {
	if repository == nil || parsed == nil {
		return nil, fmt.Errorf("C: repository and parsed program are required")
	}
	if err := parsed.Program.ValidateAgainst(repository); err != nil {
		return nil, err
	}
	b := &builder{
		repository: repository, parsed: parsed, sources: map[string][]byte{}, objects: map[string]*p.ObjectInput{},
		externalFunctions: map[string]string{}, externalVariables: map[string]string{}, functionByRef: map[string]*function{},
		definedAt: map[string]Position{}, slots: map[string]*slot{}, stores: map[string][]*store{}, passedTo: map[string][]passedAt{},
		imported: map[string]bool{}, importers: map[string]dependencies.Importer{}, escaped: map[string]bool{},
		fieldRoles: map[*Node]fieldRole{},
	}
	for _, unit := range parsed.Units {
		b.scopes = append(b.scopes, newUnitScope(unit))
	}
	b.defineTypes()
	if err := b.defineFunctionsAndVariables(); err != nil {
		return nil, fmt.Errorf("C program %s: %w", parsed.Program.Selector, err)
	}
	b.walkAll()
	b.findEscapes()
	b.recordParameterStores()
	b.emitCalls()
	b.emitStores()
	b.emitReads()
	b.emitFieldAccesses()
	b.emitImports()
	b.markUnreachable()
	input, err := b.input()
	if err != nil {
		return nil, err
	}
	catalog, err := dependencies.BuildWithOmissions(b.importerList(), b.deps, nil)
	if err != nil {
		return nil, err
	}
	return &Result{Program: parsed.Program, Toolchain: parsed.Toolchain, Outside: slices.Clone(parsed.Outside), Input: input, Dependencies: catalog}, nil
}

type builder struct {
	repository *corpus.Corpus
	parsed     *Parsed
	sources    map[string][]byte
	// lines are, per source file, the lines holding code (codeLines).
	lines  map[string]map[int]bool
	scopes []*unitScope

	objects   map[string]*p.ObjectInput
	relations []p.RelationInput
	sequence  int

	externalFunctions map[string]string // name -> function ref
	externalVariables map[string]string // name -> variable ref
	definedAt         map[string]Position
	functions         []*function
	functionByRef     map[string]*function
	tables            []table

	slots    map[string]*slot
	slotKeys []string
	stores   map[string][]*store
	passedTo map[string][]passedAt // repository function ref -> arguments its direct callers pass
	escaped  map[string]bool       // repository functions used as values, see findEscapes

	calls      []*call
	rows       []tableRow
	bindings   []binding
	constructs []construct
	reads      []read
	// fieldRoles are the members an enclosing expression has given a role
	// the walk has not reached yet; fieldAccesses the fields bodies read
	// and write.
	fieldRoles    map[*Node]fieldRole
	fieldAccesses []fieldAccess

	imported  map[string]bool
	importers map[string]dependencies.Importer
	deps      []dependencies.Dependency
}

type function struct {
	ref      string
	node     *Node
	scope    *unitScope
	location *p.Location
	// locals indexes the body for following its local variables
	// (locals.go), built at the first read that needs it.
	locals *functionLocals
}

type table struct {
	ref   string
	node  *Node
	scope *unitScope
}

func (b *builder) source(file string) []byte {
	if isAbsolute(file) {
		return nil
	}
	if data, ok := b.sources[file]; ok {
		return data
	}
	var data []byte
	if id, ok := b.repository.ID(file); ok {
		if content, err := b.repository.ReadFileAll(id); err == nil {
			data = content.Bytes
		}
	}
	b.sources[file] = data
	return data
}

// text is the source a node spans where the reader wrote it.
func (b *builder) text(n *Node) string {
	for _, pair := range [][2]Position{{n.Begin.Site(), n.End.Site()}, {n.Begin.Expansion, n.End.Expansion}} {
		begin, end := pair[0], pair[1]
		if begin.File == "" || begin.File != end.File || end.Offset+end.TokLen < begin.Offset {
			continue
		}
		data := b.source(begin.File)
		if end.Offset+end.TokLen > len(data) {
			continue
		}
		return string(data[begin.Offset : end.Offset+end.TokLen])
	}
	return ""
}

func location(position Position) *p.Location {
	if !position.Valid() || isAbsolute(position.File) || position.Col <= 0 {
		return nil
	}
	return &p.Location{Path: position.File, Line: position.Line, Column: position.Col}
}

func sourceAnchor(position Position) *sourcevalue.Anchor {
	if !position.Valid() || isAbsolute(position.File) {
		return nil
	}
	return &sourcevalue.Anchor{Path: position.File, Line: position.Line, Column: max(position.Col, 0)}
}

func positionKey(position Position) string {
	return fmt.Sprintf("%s:%d:%d", position.File, position.Line, position.Col)
}

func (b *builder) add(object p.ObjectInput) {
	if _, exists := b.objects[object.SourceRef]; !exists {
		object.CodeLines = b.codeLinesOf(object)
		b.objects[object.SourceRef] = &object
	}
}

// codeLinesOf counts the code lines of a declaration's source range, or of a
// module's whole file; zero, unknown, without a located range.
func (b *builder) codeLinesOf(object p.ObjectInput) int {
	at := object.Location
	if at == nil || isAbsolute(at.Path) {
		return 0
	}
	lines, ok := b.lines[at.Path]
	if !ok {
		lines = codeLines(b.source(at.Path))
		if b.lines == nil {
			b.lines = map[string]map[int]bool{}
		}
		b.lines[at.Path] = lines
	}
	if object.Kind == p.ObjectModule {
		return len(lines)
	}
	if object.EndLine == 0 {
		return 0
	}
	count := 0
	for line := at.Line; line <= object.EndLine; line++ {
		if lines[line] {
			count++
		}
	}
	return count
}

// codeLines are the lines of a C source holding code: a character outside
// comments that is not blank. String and character literals are code and
// end at an unescaped newline; a backslash-newline continues a // comment
// onto the next line; preprocessor lines are code.
func codeLines(source []byte) map[int]bool {
	lines := map[int]bool{}
	line := 1
	continued := func(at int) bool { // source[at] is '\n'
		back := at - 1
		if back >= 0 && source[back] == '\r' {
			back--
		}
		return back >= 0 && source[back] == '\\'
	}
	for at := 0; at < len(source); at++ {
		switch c := source[at]; {
		case c == '\n':
			line++
		case c == '/' && at+1 < len(source) && source[at+1] == '/':
			for at+1 < len(source) {
				if source[at+1] == '\n' {
					if !continued(at + 1) {
						break
					}
					line++
				}
				at++
			}
		case c == '/' && at+1 < len(source) && source[at+1] == '*':
			for at += 2; at < len(source); at++ {
				if source[at] == '\n' {
					line++
				}
				if source[at] == '*' && at+1 < len(source) && source[at+1] == '/' {
					at++
					break
				}
			}
		case c == '"' || c == '\'':
			lines[line] = true
			for at+1 < len(source) {
				at++
				if source[at] == '\n' {
					line++
					break
				}
				lines[line] = true
				if source[at] == '\\' && at+1 < len(source) {
					at++
					if source[at] == '\n' {
						line++
					}
					continue
				}
				if source[at] == c {
					break
				}
			}
		case c == ' ' || c == '\t' || c == '\r' || c == '\f' || c == '\v':
		default:
			lines[line] = true
		}
	}
	return lines
}

func visibility(internal bool) p.Visibility {
	if internal {
		return p.VisibilityInternal
	}
	return p.VisibilityPublic
}

// module is the file module of a corpus file.
func (b *builder) module(file string) string {
	ref := "c:module:" + file
	b.add(p.ObjectInput{SourceRef: ref, Kind: p.ObjectModule, Name: file, Visibility: p.VisibilityPublic, Directory: moduleDirectory(file), Location: &p.Location{Path: file, Line: 1, Column: 1}})
	return ref
}

func endLine(node *Node, at *p.Location) int {
	end := node.End.Site()
	if at == nil || end.File != at.Path || end.Line < at.Line {
		return 0
	}
	return end.Line
}

// defineTypes creates the records, enums and typedefs of every unit, merged by
// definition location. A typedef of a repository record or enum is that
// type's name, not a second type; a typedef defining an anonymous record names
// it.
func (b *builder) defineTypes() {
	byTag := map[string]map[string]*recordInfo{}
	for _, scope := range b.scopes {
		for _, info := range scope.byID {
			for _, name := range []string{info.tagUsed + " " + info.tag, info.name} {
				if info.ref == "" || strings.TrimSpace(name) == "" || strings.HasSuffix(name, " ") {
					continue
				}
				if byTag[name] == nil {
					byTag[name] = map[string]*recordInfo{}
				}
				byTag[name][info.key] = info
			}
		}
	}
	for _, scope := range b.scopes {
		// Records in dump order: byID has none.
		var visit func(nodes []*Node)
		visit = func(nodes []*Node) {
			for _, node := range nodes {
				if node.Kind != "RecordDecl" && node.Kind != "EnumDecl" {
					continue
				}
				visit(node.Inner)
				info := scope.byID[node.ID]
				if info == nil || info.ref == "" {
					continue
				}
				b.defineRecord(info)
			}
		}
		visit(scope.unit.Decls)
	}
	for _, scope := range b.scopes {
		for _, node := range scope.typedefs {
			underlying := baseType(node.Type.QualType)
			if strings.Contains(node.Type.QualType, "*") || strings.Contains(node.Type.QualType, "[") {
				underlying = ""
			}
			if info := scope.records[node.Name]; info != nil {
				continue // it names a record or enum it defines
			}
			if underlying != "" {
				if info := scope.records[underlying]; info != nil {
					scope.records[node.Name] = info
					continue
				}
				if candidates := byTag[underlying]; len(candidates) == 1 {
					for _, info := range candidates {
						scope.records[node.Name] = info
					}
					continue
				}
			}
			site := node.Loc.Site()
			if isAbsolute(site.File) {
				continue
			}
			ref := fmt.Sprintf("c:type:%s#%s", positionKey(site), node.Name)
			scope.typedefRefs[node.Name] = ref
			at := location(site)
			b.add(p.ObjectInput{SourceRef: ref, Kind: p.ObjectType, Name: node.Name, Visibility: visibility(!isHeader(site.File)),
				Signature: clean(node.Type.QualType), OwnerRef: b.module(site.File), ContainerRef: b.module(site.File), Location: at, EndLine: endLine(node, at)})
		}
	}
	// A record only another unit defines (an opaque struct behind a
	// typedef) is still the program's one record of that name.
	for _, scope := range b.scopes {
		for name, candidates := range byTag {
			if scope.records[name] == nil && len(candidates) == 1 {
				for _, info := range candidates {
					scope.records[name] = info
				}
			}
		}
	}
}

func isHeader(file string) bool { return strings.HasSuffix(file, ".h") }

func (b *builder) defineRecord(info *recordInfo) {
	site := info.node.Loc.Site()
	if _, exists := b.objects[info.ref]; exists {
		return
	}
	signature := info.tagUsed
	if info.tag != "" && info.tag != info.name {
		signature += " " + info.tag
	}
	at := location(site)
	internal := !isHeader(site.File)
	module := b.module(site.File)
	b.add(p.ObjectInput{SourceRef: info.ref, Kind: p.ObjectType, Name: info.name, Visibility: visibility(internal), Signature: signature,
		OwnerRef: module, ContainerRef: module, Location: at, EndLine: endLine(info.node, at)})
	for _, field := range info.fields {
		if field.ref == "" {
			continue
		}
		fieldAt := location(field.node.Loc.Site())
		signature := clean(field.node.Type.QualType)
		if field.node.Kind == "EnumConstantDecl" {
			signature = ""
		}
		b.add(p.ObjectInput{SourceRef: field.ref, Kind: p.ObjectVariable, Name: field.name, Visibility: visibility(internal), Signature: signature,
			OwnerRef: info.ref, ContainerRef: info.ref, Location: fieldAt, EndLine: endLine(field.node, fieldAt)})
	}
}

// functionRef is a function definition's identity: its name when it has
// external linkage, else its definition location.
func (b *builder) functionRef(s *unitScope, node *Node) string {
	if s.internal[node.Name] {
		return fmt.Sprintf("c:function:%s#%s", positionKey(node.Loc.Site()), node.Name)
	}
	return "c:function:@" + node.Name
}

func (b *builder) variableKey(s *unitScope, node *Node) string {
	if s.internal[node.Name] {
		return fmt.Sprintf("c:variable:%s#%s", positionKey(node.Loc.Site()), node.Name)
	}
	return "c:variable:@" + node.Name
}

// defineFunctionsAndVariables creates every function definition and
// file-scope variable definition of the program's units.
func (b *builder) defineFunctionsAndVariables() error {
	// External names first, so a unit can resolve a call into a unit after it.
	for _, scope := range b.scopes {
		for _, node := range scope.unit.Decls {
			site := node.Loc.Site()
			if scope.internal[node.Name] || isAbsolute(site.File) {
				continue
			}
			switch {
			case node.Kind == "FunctionDecl" && hasBody(node):
				ref := b.functionRef(scope, node)
				if err := b.defineOnce("function", node.Name, site); err != nil {
					return err
				}
				b.externalFunctions[node.Name] = ref
			case node.Kind == "VarDecl" && node.StorageClass != "extern":
				ref := b.variableKey(scope, node)
				if node.Init != "" {
					if err := b.defineOnce("variable", node.Name, site); err != nil {
						return err
					}
				}
				b.externalVariables[node.Name] = ref
			}
		}
	}
	for _, scope := range b.scopes {
		for _, node := range scope.unit.Decls {
			site := node.Loc.Site()
			if isAbsolute(site.File) {
				continue
			}
			switch {
			case node.Kind == "FunctionDecl" && hasBody(node):
				ref := b.functionRef(scope, node)
				if !scope.internal[node.Name] && b.definedAt["function "+node.Name] != site {
					continue
				}
				fn := b.functionByRef[ref]
				if fn == nil {
					fn = &function{ref: ref, node: node, scope: scope, location: location(site)}
					b.functionByRef[ref] = fn
					b.functions = append(b.functions, fn)
					b.defineFunction(fn)
				}
				// Parameters of every unit's copy name the same function.
				position := 0
				for _, child := range node.Inner {
					if child.Kind == "ParmVarDecl" {
						position++
						scope.params[child.ID] = paramInfo{function: ref, position: position, name: child.Name}
					}
				}
			case node.Kind == "VarDecl" && node.StorageClass != "extern":
				ref := b.variableKey(scope, node)
				if !scope.internal[node.Name] && node.Init == "" && b.definedAt["variable "+node.Name].File != "" && b.definedAt["variable "+node.Name] != site {
					continue // a tentative definition of a variable another unit initializes
				}
				if scope.internal[node.Name] && scope.statics[node.Name] != node {
					continue // a tentative definition of a static the unit defines again
				}
				if _, exists := b.objects[ref]; exists {
					continue
				}
				at := location(site)
				module := b.module(site.File)
				b.add(p.ObjectInput{SourceRef: ref, Kind: p.ObjectVariable, Name: node.Name, Visibility: visibility(scope.internal[node.Name]),
					Signature: clean(node.Type.QualType), OwnerRef: module, ContainerRef: module, Location: at, EndLine: endLine(node, at)})
				if value := initializer(node); value != nil {
					b.tables = append(b.tables, table{ref: ref, node: node, scope: scope})
				}
			}
		}
	}
	return nil
}

// defineOnce records where an external name is defined and refuses a second
// definition elsewhere.
func (b *builder) defineOnce(kind, name string, site Position) error {
	key := kind + " " + name
	if previous, ok := b.definedAt[key]; ok && previous != site {
		return fmt.Errorf("%s %s is defined twice: %s:%d and %s:%d", kind, name, previous.File, previous.Line, site.File, site.Line)
	}
	b.definedAt[key] = site
	return nil
}

var storageWords = regexp.MustCompile(`\b(static|extern|inline|__inline|__inline__|register)\b`)
var attributes = regexp.MustCompile(`__attribute__\s*\(\(.*?\)\)`)

// clean is text on one line: comments dropped, white space collapsed.
func clean(text string) string {
	var out strings.Builder
	for i := 0; i < len(text); i++ {
		switch {
		case strings.HasPrefix(text[i:], "/*"):
			end := strings.Index(text[i+2:], "*/")
			if end < 0 {
				i = len(text)
			} else {
				i += end + 3
			}
			out.WriteByte(' ')
		case strings.HasPrefix(text[i:], "//"):
			end := strings.IndexByte(text[i:], '\n')
			if end < 0 {
				i = len(text)
			} else {
				i += end
			}
			out.WriteByte(' ')
		default:
			out.WriteByte(text[i])
		}
	}
	return strings.ToValidUTF8(strings.Join(strings.Fields(out.String()), " "), "�")
}

// defineFunction creates a function object: its signature as written up to
// the body, its parameters and result with the repository types they carry.
func (b *builder) defineFunction(fn *function) {
	node, scope := fn.node, fn.scope
	site := node.Loc.Site()
	module := b.module(site.File)
	var parameters []p.TypedNameInput
	for _, child := range node.Inner {
		if child.Kind == "ParmVarDecl" {
			value := p.TypedNameInput{Name: child.Name, Type: clean(child.Type.QualType), TypeRef: scope.typeRef(child.Type)}
			if value.Name == "" && value.Type == "" {
				continue
			}
			parameters = append(parameters, value)
		}
	}
	var results []p.TypedNameInput
	if result := b.resultType(node); result != "" && result != "void" {
		results = []p.TypedNameInput{{Type: result, TypeRef: scope.typeRef(Type{QualType: result})}}
	}
	b.add(p.ObjectInput{SourceRef: fn.ref, Kind: p.ObjectFunction, Name: node.Name, Visibility: visibility(scope.internal[node.Name]),
		Signature: b.signature(node, parameters, results), OwnerRef: module, ContainerRef: module, Location: fn.location,
		EndLine: endLine(node, fn.location), Parameters: parameters, Results: results})
}

// resultType is the type a function returns as written before its name, else
// as clang prints it.
func (b *builder) resultType(node *Node) string {
	begin, name := node.Begin.Site(), node.Loc.Site()
	if !node.Begin.FromMacro() && !node.Loc.FromMacro() && begin.File == name.File && begin.Offset < name.Offset {
		if data := b.source(begin.File); name.Offset <= len(data) {
			text := clean(attributes.ReplaceAllString(storageWords.ReplaceAllString(string(data[begin.Offset:name.Offset]), " "), " "))
			if text != "" && !strings.ContainsAny(text, "(){};#") {
				return text
			}
		}
	}
	text := node.Type.QualType
	depth := 0
	for i, c := range text {
		switch c {
		case '(':
			if depth == 0 {
				return strings.TrimSpace(text[:i])
			}
			depth++
		case ')':
			depth--
		}
	}
	return ""
}

// signature is the declaration as written up to the body, without storage
// words; a declaration a macro wrote is rebuilt from its parts.
func (b *builder) signature(node *Node, parameters, results []p.TypedNameInput) string {
	begin := node.Begin.Site()
	var body *Node
	for _, child := range node.Inner {
		if child.Kind == "CompoundStmt" {
			body = child
		}
	}
	if body != nil && !node.Begin.FromMacro() && !body.Begin.FromMacro() && begin.File == body.Begin.Site().File && begin.Offset < body.Begin.Site().Offset {
		if data := b.source(begin.File); body.Begin.Site().Offset <= len(data) {
			text := clean(storageWords.ReplaceAllString(string(data[begin.Offset:body.Begin.Site().Offset]), " "))
			if text != "" && utf8.ValidString(text) && !strings.ContainsAny(text, "{};#") {
				return text
			}
		}
	}
	result := "void"
	if len(results) > 0 {
		result = results[0].Type
	}
	var values []string
	for _, parameter := range parameters {
		values = append(values, strings.TrimSpace(parameter.Type+" "+parameter.Name))
	}
	if node.Variadic {
		values = append(values, "...")
	}
	return fmt.Sprintf("%s %s(%s)", result, node.Name, strings.Join(values, ", "))
}

// externalSymbol is the object of a declaration outside the corpus: a function
// or variable (kind "function"), a record or typedef (kind "type"). Its
// package is the header the repository includes on the way to it; its
// authority is the platform unless the header is a package's.
func (b *builder) externalSymbol(kind, name string, declaration *ExternalDecl) string {
	pkg, authority := "builtin", p.ExternalAuthorityPlatform
	// clang declares its builtins (and library functions used without a
	// prototype) implicitly, located at their first use in the corpus.
	if declaration != nil && declaration.Class != FileCorpus && !builtinName(name) {
		pkg = declaration.Package
		if pkg == "" {
			pkg = baseName(declaration.Position.File)
		}
		if declaration.Class == FilePackage || declaration.Class == FileRepository {
			authority = p.ExternalAuthorityPackage
		}
	}
	ref := fmt.Sprintf("c:external:%s:%s:%s", kind, pkg, name)
	b.add(p.ObjectInput{SourceRef: ref, Kind: p.ObjectExternalSymbol, Name: name, Visibility: p.VisibilityPublic,
		External: &p.ExternalSymbol{AuthorityKind: authority, PackagePath: pkg, Name: name}})
	return ref
}

func baseName(file string) string {
	if file == "" || strings.HasPrefix(file, "<") {
		return "builtin"
	}
	if slash := strings.LastIndexAny(file, `/\`); slash >= 0 {
		return file[slash+1:]
	}
	return file
}

// walkAll walks every function body once, and every module-level
// initializer.
func (b *builder) walkAll() {
	for _, fn := range b.functions {
		for _, child := range fn.node.Inner {
			if child.Kind == "CompoundStmt" {
				walker{b: b, scope: fn.scope, owner: fn.ref, function: fn}.walk(child)
			}
		}
	}
	for _, t := range b.tables {
		value := initializer(t.node)
		w := walker{b: b, scope: t.scope, owner: t.ref}
		if list := unwrapValue(value); list != nil && list.Kind == "InitListExpr" {
			b.initList(w, list, t.ref)
			continue
		}
		b.initialize(w, t.node, value)
		if d := designator(value); d != nil {
			if resolved := b.resolveFunction(t.scope, d.ReferencedDecl); resolved.ref != "" && !resolved.external {
				b.bindings = append(b.bindings, binding{from: t.ref, fn: resolved.ref, site: d.Begin.Site(), detail: fmt.Sprintf("%s stored in variable %s", b.objects[resolved.ref].Name, t.node.Name)})
			}
		}
	}
}

func (b *builder) relation(r p.RelationInput) {
	if r.TargetsObserved == 0 {
		r.TargetsObserved = max(1, len(r.ToRefs))
	}
	r.WitnessesObserved = len(r.Witnesses)
	r.PatternsObserved = len(r.Patterns)
	b.relations = append(b.relations, r)
}

func resolution(targets []string) p.Resolution {
	switch len(targets) {
	case 0:
		return p.ResolutionUnresolved
	case 1:
		return p.ResolutionExact
	default:
		return p.ResolutionAlternatives
	}
}

// recordParameterStores gives each function the stores of its own
// parameters, as it received them, into a field (an array of records
// included) or a module-level variable: what a call handing that function a
// callable joins to (PROGRAM_INDEX). A local variable holds nothing beyond
// the call, and a parameter handed on to another function's parameter is not
// stored.
func (b *builder) recordParameterStores() {
	for _, key := range b.slotOrder() {
		if !strings.HasPrefix(key, "field:") && !strings.HasPrefix(key, "var:") {
			continue
		}
		for _, st := range b.stores[key] {
			fn := b.objects[st.in]
			at := location(st.site)
			if st.param == 0 || fn == nil || !fn.Kind.Callable() || at == nil {
				continue
			}
			name := ""
			if st.param <= len(fn.Parameters) {
				name = fn.Parameters[st.param-1].Name
			}
			fn.ParameterStores = append(fn.ParameterStores, p.ParameterStore{Parameter: st.param, Name: name, Slot: strings.TrimPrefix(st.slot.name, "variable "), Location: at})
		}
	}
}

// emitCalls turns every recorded call into its relation, and every function
// an argument names into a callback bound to that argument.
func (b *builder) emitCalls() {
	// The slots a function stores each parameter into; conditional when a
	// branch decides whether, or into which of them, it is stored.
	type storedIn struct {
		slots       []*slot
		conditional bool
	}
	stored := map[string]map[int]*storedIn{}
	for _, key := range b.slotOrder() {
		for _, st := range b.stores[key] {
			if st.param == 0 {
				continue
			}
			if stored[st.in] == nil {
				stored[st.in] = map[int]*storedIn{}
			}
			into := stored[st.in][st.param]
			if into == nil {
				into = &storedIn{}
				stored[st.in][st.param] = into
			}
			if !slices.Contains(into.slots, st.slot) {
				into.slots = append(into.slots, st.slot)
			}
			into.conditional = into.conditional || st.conditional
		}
	}
	for _, c := range b.calls {
		at := location(c.site)
		pattern := p.RelationPatternInput{SourceRef: c.patternRef, Form: p.PatternCall, Selector: c.selector, Location: at,
			Context: c.context, Arguments: c.arguments, ArgumentsObserved: len(c.arguments)}
		r := p.RelationInput{SourceRef: c.relationRef, Kind: p.RelationCalls, FromRef: c.from, Location: at, Patterns: []p.RelationPatternInput{pattern}}
		switch {
		case c.direct.ref != "":
			r.ToRefs, r.Resolution = []string{c.direct.ref}, p.ResolutionExact
			if c.direct.external {
				r.Kind = p.RelationInvokesExternal
			}
			r.Witnesses = []p.Witness{{Kind: "c_call", Detail: "call of " + b.objects[c.direct.ref].Name, Location: at}}
		case c.slot == nil && c.direct.unresolved != "":
			r.Resolution = p.ResolutionUnresolved
			r.Witnesses = []p.Witness{{Kind: "c_call", Detail: c.direct.unresolved, Location: at}}
		default:
			r.Dispatch = p.DispatchFunctionValue
			through := "call through " + c.expression
			if c.slot != nil {
				through = "call through " + c.slot.name
			}
			r.Witnesses = []p.Witness{{Kind: "c_function_value_call", Detail: clean(through), Location: at}}
			if c.slot != nil {
				found, uncertain, conditional := b.candidates(c.slot)
				var targets []string
				for _, candidate := range found {
					if !slices.Contains(targets, candidate.fn) {
						targets = append(targets, candidate.fn)
					}
					// The witness names what its store put there, so a call left
					// open keeps its candidates as identities, never targets.
					r.Witnesses = append(r.Witnesses, p.Witness{Kind: "c_function_pointer_store", Detail: candidate.detail, Location: location(candidate.site), ObjectRef: candidate.fn})
				}
				if !uncertain && !conditional {
					r.ToRefs = targets
				}
			}
			r.Resolution = resolution(r.ToRefs)
			if len(r.ToRefs) == 1 && b.objects[r.ToRefs[0]].Kind == p.ObjectExternalSymbol {
				r.Kind = p.RelationInvokesExternal
			}
		}
		if c.macro != nil {
			r.Witnesses = append(r.Witnesses, *c.macro)
		}
		b.relation(r)
		positions := make([]int, 0, len(c.designators))
		for position := range c.designators {
			positions = append(positions, position)
		}
		slices.Sort(positions)
		for _, position := range positions {
			d := c.designators[position]
			witness := p.Witness{Kind: "c_callback_argument", Detail: fmt.Sprintf("%s passed to %s as argument %d", b.objects[d.fn].Name, c.selector, position), Location: location(d.site)}
			if c.direct.ref != "" && !c.direct.external {
				if into := stored[c.direct.ref][position]; into != nil {
					var names []string
					for _, s := range into.slots {
						names = append(names, s.name)
					}
					detail := fmt.Sprintf("%s stored in %s by %s", b.objects[d.fn].Name, strings.Join(names, " and "), b.objects[c.direct.ref].Name)
					if into.conditional {
						detail = fmt.Sprintf("%s stored in %s by %s under a condition", b.objects[d.fn].Name, strings.Join(names, " or "), b.objects[c.direct.ref].Name)
					}
					witness = p.Witness{Kind: "c_function_pointer_store", Detail: detail, Location: location(d.site)}
				}
			}
			b.sequence++
			b.relation(p.RelationInput{SourceRef: fmt.Sprintf("c:callback:%d", b.sequence), Kind: p.RelationPassesCallback, FromRef: c.from,
				ToRefs: []string{d.fn}, Resolution: p.ResolutionExact, Location: location(d.site), Witnesses: []p.Witness{witness},
				SourceArgument: &p.PatternArgumentRefInput{RelationSourceRef: c.relationRef, PatternSourceRef: c.patternRef, Position: position}})
		}
	}
}

// emitStores turns the functions stored by assignments, initializers and
// table rows into callbacks. A row of a module-level table that names its
// function by a string literal is also the construction of its record, with
// the literals beside the function: the owner's registration shape for a
// table the repository owns.
func (b *builder) emitStores() {
	for _, bound := range b.bindings {
		b.sequence++
		b.relation(p.RelationInput{SourceRef: fmt.Sprintf("c:store:%d", b.sequence), Kind: p.RelationPassesCallback, FromRef: bound.from,
			ToRefs: []string{bound.fn}, Resolution: p.ResolutionExact, Location: location(bound.site),
			Witnesses: []p.Witness{{Kind: "c_function_pointer_store", Detail: bound.detail, Location: location(bound.site)}}})
	}
	for _, row := range b.rows {
		owner := row.owner
		tableName := b.objects[owner].Name
		var fieldWitness []p.Witness
		var literals []p.PatternArgumentInput
		for _, literal := range row.literals {
			literals = append(literals, p.PatternArgumentInput{Keyword: literal.field, Kind: p.PatternLiteralString, Value: literal.value,
				Origin: &sourcevalue.Value{Kind: "literal", Text: literal.value, Anchor: sourceAnchor(literal.site)}})
		}
		if len(row.literals) > 0 {
			first := row.literals[0]
			fieldWitness = []p.Witness{{Kind: "callable_receiver_field", Detail: first.field + " = " + strconv.Quote(first.value), Location: location(first.site)}}
		}
		for _, st := range row.stored {
			b.sequence++
			detail := fmt.Sprintf("%s stored in %s by a row of %s", b.objects[st.fn].Name, st.slot.name, tableName)
			if row.table == "" {
				detail = fmt.Sprintf("%s stored in %s by %s", b.objects[st.fn].Name, st.slot.name, tableName)
			}
			callback := p.RelationInput{SourceRef: fmt.Sprintf("c:store:%d", b.sequence), Kind: p.RelationPassesCallback, FromRef: owner,
				ToRefs: []string{st.fn}, Resolution: p.ResolutionExact, Location: location(st.site),
				Witnesses: append([]p.Witness{{Kind: "c_function_pointer_store", Detail: detail, Location: location(st.site)}}, fieldWitness...)}
			if row.table != "" && len(literals) > 0 && row.record.ref != "" {
				relationRef, patternRef := fmt.Sprintf("c:row:%d", b.sequence), fmt.Sprintf("c:row:%d:pattern", b.sequence)
				arguments := append(slices.Clone(literals), p.PatternArgumentInput{Keyword: st.field, Kind: p.PatternDynamic, ObjectRefs: []string{st.fn},
					Resolution: p.ResolutionExact, ObjectsObserved: 1, Origin: &sourcevalue.Value{Kind: "unknown", Text: b.objects[st.fn].Name, Anchor: sourceAnchor(st.site)}})
				at := location(st.site)
				b.relation(p.RelationInput{SourceRef: relationRef, Kind: p.RelationCalls, FromRef: owner, ToRefs: []string{row.record.ref}, Resolution: p.ResolutionExact,
					Invocation: p.InvocationConstruct, Location: at,
					Witnesses: []p.Witness{{Kind: "c_initializer_row", Detail: fmt.Sprintf("row of %s building %s", tableName, row.record.name), Location: location(row.begin)}},
					Patterns:  []p.RelationPatternInput{{SourceRef: patternRef, Form: p.PatternCall, Selector: row.record.name, Location: at, Arguments: arguments, ArgumentsObserved: len(arguments)}}})
				callback.SourceArgument = &p.PatternArgumentRefInput{RelationSourceRef: relationRef, PatternSourceRef: patternRef, Keyword: st.field}
			}
			b.relation(callback)
		}
	}
	for _, c := range b.constructs {
		b.sequence++
		typeRef := b.externalSymbol("type", c.container, c.declaration)
		relationRef, patternRef := fmt.Sprintf("c:construct:%d", b.sequence), fmt.Sprintf("c:construct:%d:pattern", b.sequence)
		at := location(c.site)
		argument := p.PatternArgumentInput{Keyword: c.field, Kind: p.PatternDynamic, ObjectRefs: []string{c.fn}, Resolution: p.ResolutionExact, ObjectsObserved: 1,
			Origin: &sourcevalue.Value{Kind: "unknown", Text: b.objects[c.fn].Name, Anchor: sourceAnchor(c.fnSite)}}
		b.relation(p.RelationInput{SourceRef: relationRef, Kind: p.RelationInvokesExternal, FromRef: c.from, ToRefs: []string{typeRef}, Resolution: p.ResolutionExact,
			Invocation: p.InvocationConstruct, Location: at,
			Witnesses: []p.Witness{{Kind: "c_field_store", Detail: fmt.Sprintf("%s stored in %s.%s", b.objects[c.fn].Name, c.container, c.field), Location: at}},
			Patterns:  []p.RelationPatternInput{{SourceRef: patternRef, Form: p.PatternCall, Selector: c.container, Location: at, Arguments: []p.PatternArgumentInput{argument}, ArgumentsObserved: 1}}})
		b.relation(p.RelationInput{SourceRef: fmt.Sprintf("c:construct:%d:callback", b.sequence), Kind: p.RelationPassesCallback, FromRef: c.from,
			ToRefs: []string{c.fn}, Resolution: p.ResolutionExact, Location: location(c.fnSite),
			Witnesses:      []p.Witness{{Kind: "c_function_pointer_store", Detail: fmt.Sprintf("%s stored in %s.%s", b.objects[c.fn].Name, c.container, c.field), Location: location(c.fnSite)}},
			SourceArgument: &p.PatternArgumentRefInput{RelationSourceRef: relationRef, PatternSourceRef: patternRef, Keyword: c.field}})
	}
}

// emitReads turns each place a function body names a file-scope variable into
// a reads relation, one per site, exact: the linker's identity of the
// variable is known.
func (b *builder) emitReads() {
	seen := map[string]bool{}
	for _, r := range b.reads {
		key := r.from + "\x00" + r.variable + "\x00" + positionKey(r.site)
		if seen[key] {
			continue
		}
		seen[key] = true
		at := location(r.site)
		witnesses := []p.Witness{{Kind: "c_variable_read", Detail: "read of " + b.objects[r.variable].Name, Location: at}}
		if r.macro != nil {
			witnesses = append(witnesses, *r.macro)
		}
		b.sequence++
		b.relation(p.RelationInput{SourceRef: fmt.Sprintf("c:read:%d", b.sequence), Kind: p.RelationReads, FromRef: r.from,
			ToRefs: []string{r.variable}, Resolution: p.ResolutionExact, Location: at, Witnesses: witnesses})
	}
}

// emitFieldAccesses turns each place a function body reads or writes a field
// of a repository record into a reads or writes relation, one per site,
// exact: the field is the record's own, whatever value it is reached from.
func (b *builder) emitFieldAccesses() {
	seen := map[string]bool{}
	for _, access := range b.fieldAccesses {
		kind, witness, verb := p.RelationReads, "c_field_read", "read of "
		if access.write {
			kind, witness, verb = p.RelationWrites, "c_field_write", "write of "
		}
		key := access.from + "\x00" + access.field + "\x00" + string(kind) + "\x00" + positionKey(access.site)
		if seen[key] {
			continue
		}
		seen[key] = true
		at := location(access.site)
		witnesses := []p.Witness{{Kind: witness, Detail: verb + access.path, Location: at}}
		if access.macro != nil {
			witnesses = append(witnesses, *access.macro)
		}
		b.sequence++
		b.relation(p.RelationInput{SourceRef: fmt.Sprintf("c:field:%d", b.sequence), Kind: kind, FromRef: access.from,
			ToRefs: []string{access.field}, Resolution: p.ResolutionExact, Location: at, Witnesses: witnesses, FieldPath: access.path})
	}
}

// input assembles the target and its sealed-to-be graph.
func (b *builder) input() (p.Input, error) {
	program := b.parsed.Program
	sources := map[string]string{program.Anchor.Path: program.AnchorFileRef}
	addSource := func(file string) error {
		if _, ok := sources[file]; ok {
			return nil
		}
		id, ok := b.repository.ID(file)
		if !ok {
			return fmt.Errorf("C program %s: %s is not in the corpus", program.Selector, file)
		}
		sources[file] = string(id)
		return nil
	}
	for _, unit := range b.parsed.Units {
		if err := addSource(unit.Path); err != nil {
			return p.Input{}, err
		}
	}
	target := p.TargetInput{Language: "c", Kind: string(program.Kind), Name: program.Name, Selector: program.Selector, AnchorFileRef: program.AnchorFileRef}
	// The executable a link line writes is named by its output (Makefile:49
	// links redis-server); a program built by hand from its main unit has
	// no name the build gives it.
	for _, observation := range program.Evidence {
		if observation.Kind == "c_link" && program.Kind == ProgramExecutable && observation.Fields["output"] != "" {
			target.Executables = append(target.Executables, path.Base(observation.Fields["output"]))
		}
	}
	if main := b.parsed.Main; main != nil {
		fn := b.mainFunction()
		if fn == nil {
			return p.Input{}, fmt.Errorf("C program %s: main at %s:%d has no function object", program.Selector, main.At.File, main.At.Line)
		}
		if err := addSource(fn.location.Path); err != nil {
			return p.Input{}, err
		}
		target.Seeds = append(target.Seeds, p.TargetSeedInput{ObjectRef: fn.ref, Kind: p.SeedCallable, Location: fn.location})
	}
	for file, ref := range sources {
		target.Sources = append(target.Sources, p.TargetSource{FileRef: ref, Path: file})
	}
	slices.SortFunc(target.Sources, func(a, b p.TargetSource) int { return strings.Compare(a.Path, b.Path) })
	input := p.Input{ScenarioSHA256: program.CorpusSHA256, SourceSHA256: b.sourceDigest(), Target: target, Relations: b.relations}
	for _, object := range b.objects {
		input.Objects = append(input.Objects, *object)
	}
	slices.SortFunc(input.Objects, func(a, b p.ObjectInput) int { return strings.Compare(a.SourceRef, b.SourceRef) })
	input.Coverage = p.CoverageInput{Measured: true, ObjectsObserved: len(input.Objects), RelationsObserved: len(input.Relations)}
	return input, nil
}

// sourceDigest binds the input to what clang read: the program, the platform
// view and each unit's command and include tree.
func (b *builder) sourceDigest() string {
	type unitView struct {
		Path     string    `json:"path"`
		Command  []string  `json:"command"`
		Includes []Include `json:"includes"`
		Bytes    int64     `json:"json_bytes"`
	}
	view := struct {
		Program   string     `json:"program"`
		Version   string     `json:"version"`
		Target    string     `json:"target"`
		Sysroot   string     `json:"sysroot"`
		Overrides []string   `json:"overrides"`
		Units     []unitView `json:"units"`
	}{Program: b.parsed.Program.Ref, Version: b.parsed.Toolchain.Version, Target: b.parsed.Toolchain.Target, Sysroot: b.parsed.Toolchain.Sysroot, Overrides: b.parsed.Toolchain.Overrides}
	for _, unit := range b.parsed.Units {
		view.Units = append(view.Units, unitView{Path: unit.Path, Command: unit.Command, Includes: unit.Includes, Bytes: unit.JSONBytes})
	}
	raw, _ := json.Marshal(view)
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}
