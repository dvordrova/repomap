package cproject

import (
	"bytes"
	"fmt"
	"path"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// A unit scope says what one translation unit's clang ids mean. clang ids are
// process pointers that differ between dumps, so every identity the program
// shares is a name with external linkage, or a definition location.
type unitScope struct {
	unit *Unit
	// decls are corpus declarations by clang id: top-level ones and the
	// functions and extern variables a function body declares.
	decls map[string]*Node
	// internal are the file-scope names with internal linkage (static on any
	// declaration), statics their definitions in this unit.
	internal map[string]bool
	statics  map[string]*Node
	external map[string]*ExternalDecl
	// platform are functions outside the corpus by name, for a platform
	// function the repository declares again itself.
	platform map[string]*ExternalDecl
	// outside are records and typedefs outside the corpus by the name code
	// spells them with (struct sigaction, FILE).
	outside map[string]*ExternalDecl
	// records are this unit's complete struct, union and enum definitions by
	// the names code spells them with ("struct dict", "dict", "enum color").
	records  map[string]*recordInfo
	byID     map[string]*recordInfo
	fields   map[string]*fieldInfo
	typedefs []*Node
	// typedefRefs are typedefs that are types of their own (typedef char *sds).
	typedefRefs map[string]string
	params      map[string]paramInfo
}

type recordInfo struct {
	// key is the definition's location and name, the record's identity in
	// every unit that sees it.
	key     string
	ref     string // object ref; empty for an anonymous record no typedef names
	name    string
	tag     string
	tagUsed string // struct, union or enum
	node    *Node
	fields  []*fieldInfo // every FieldDecl in order; enum constants for an enum
}

type fieldInfo struct {
	record *recordInfo
	name   string
	ref    string
	node   *Node
}

type paramInfo struct {
	function string // function ref
	position int    // one-based
	name     string
}

func newUnitScope(unit *Unit) *unitScope {
	s := &unitScope{
		unit: unit, decls: map[string]*Node{}, internal: map[string]bool{}, statics: map[string]*Node{},
		external: map[string]*ExternalDecl{}, platform: map[string]*ExternalDecl{}, outside: map[string]*ExternalDecl{},
		records: map[string]*recordInfo{}, byID: map[string]*recordInfo{}, fields: map[string]*fieldInfo{},
		typedefRefs: map[string]string{}, params: map[string]paramInfo{},
	}
	for i := range unit.External {
		declaration := &unit.External[i]
		s.external[declaration.ID] = declaration
		switch declaration.Kind {
		case "FunctionDecl":
			if declaration.Class != FileCorpus && s.platform[declaration.Name] == nil {
				s.platform[declaration.Name] = declaration
			}
		case "RecordDecl", "EnumDecl":
			tag := declaration.TagUsed
			if declaration.Kind == "EnumDecl" {
				tag = "enum"
			}
			if declaration.Name != "" && s.outside[tag+" "+declaration.Name] == nil {
				s.outside[tag+" "+declaration.Name] = declaration
			}
		case "TypedefDecl":
			if s.outside[declaration.Name] == nil {
				s.outside[declaration.Name] = declaration
			}
		}
	}
	// A typedef that defines its record in place names an anonymous one.
	owners := map[string]string{}
	for _, node := range unit.Decls {
		if node.Kind != "TypedefDecl" {
			continue
		}
		for _, child := range node.Inner {
			if child.Kind == "ElaboratedType" && child.OwnedTagDecl != nil {
				if _, taken := owners[child.OwnedTagDecl.ID]; !taken {
					owners[child.OwnedTagDecl.ID] = node.Name
				}
			}
		}
	}
	for _, node := range unit.Decls {
		switch node.Kind {
		case "FunctionDecl", "VarDecl":
			s.decls[node.ID] = node
			if node.StorageClass == "static" {
				s.internal[node.Name] = true
			}
			s.indexScoped(node)
		case "RecordDecl", "EnumDecl":
			s.indexRecord(node, owners)
		case "TypedefDecl":
			s.typedefs = append(s.typedefs, node)
		}
	}
	// A static variable's tentative definitions (static T x;) and its
	// initialized definition are one variable: the initialized one, else the
	// first.
	for _, node := range unit.Decls {
		if !s.internal[node.Name] {
			continue
		}
		previous := s.statics[node.Name]
		switch {
		case node.Kind == "FunctionDecl" && hasBody(node) && previous == nil:
			s.statics[node.Name] = node
		case node.Kind == "VarDecl" && node.StorageClass != "extern" && (previous == nil || previous.Kind == "VarDecl" && previous.Init == "" && node.Init != ""):
			s.statics[node.Name] = node
		}
	}
	return s
}

// indexScoped records the declarations a function body makes of functions and
// external variables: they name file-scope definitions.
func (s *unitScope) indexScoped(node *Node) {
	for _, child := range node.Inner {
		if child.Kind == "FunctionDecl" || child.Kind == "VarDecl" && child.StorageClass == "extern" {
			s.decls[child.ID] = child
		}
		s.indexScoped(child)
	}
}

func (s *unitScope) indexRecord(node *Node, owners map[string]string) {
	for _, child := range node.Inner {
		if child.Kind == "RecordDecl" {
			s.indexRecord(child, owners)
		}
	}
	// clang marks a complete record; an enum is complete with its constants.
	if !node.CompleteDefinition && !(node.Kind == "EnumDecl" && len(node.Inner) > 0) {
		return
	}
	tagUsed := node.TagUsed
	if node.Kind == "EnumDecl" {
		tagUsed = "enum"
	}
	site := node.Loc.Site()
	info := &recordInfo{tag: node.Name, tagUsed: tagUsed, node: node, name: node.Name}
	if owner := owners[node.ID]; owner != "" {
		info.name = owner
	}
	info.key = fmt.Sprintf("%s:%d:%d#%s", site.File, site.Line, site.Col, info.name)
	if info.name != "" && !isAbsolute(site.File) {
		info.ref = "c:type:" + info.key
	}
	s.byID[node.ID] = info
	for _, name := range []string{tagUsed + " " + node.Name, owners[node.ID], tagUsed + " " + owners[node.ID]} {
		if strings.TrimSpace(name) != "" && !strings.HasSuffix(name, " ") && s.records[name] == nil {
			s.records[name] = info
		}
	}
	for _, child := range node.Inner {
		if child.Kind != "FieldDecl" && child.Kind != "EnumConstantDecl" {
			continue
		}
		field := &fieldInfo{record: info, name: child.Name, node: child}
		if info.ref != "" && child.Name != "" {
			field.ref = info.ref + "." + child.Name
		}
		info.fields = append(info.fields, field)
		s.fields[child.ID] = field
	}
}

// record finds the record a type names in this unit: struct dict, dict *,
// const robj *, a typedef of a typedef.
func (s *unitScope) record(t Type) *recordInfo {
	for _, text := range []string{t.QualType, t.Desugared} {
		if base := baseType(text); base != "" {
			if info := s.records[base]; info != nil {
				return info
			}
		}
	}
	return nil
}

// typeRef is the repository type a value of type t carries through pointers,
// arrays and qualifiers: a record or a typedef that is a type of its own.
func (s *unitScope) typeRef(t Type) string {
	if info := s.record(t); info != nil {
		return info.ref
	}
	for _, text := range []string{t.QualType, t.Desugared} {
		if ref := s.typedefRefs[baseType(text)]; ref != "" {
			return ref
		}
	}
	return ""
}

var typeQualifiers = map[string]bool{
	"const": true, "volatile": true, "restrict": true, "__restrict": true, "__restrict__": true,
	"_Nonnull": true, "_Nullable": true, "_Null_unspecified": true, "_Atomic": true, "register": true,
}

// baseType strips qualifiers, pointers and array bounds from a printed C type:
// "const struct dict *" is "struct dict". A function type has none.
func baseType(text string) string {
	if text == "" || strings.Contains(text, "(") {
		return ""
	}
	for {
		trimmed := strings.TrimSpace(text)
		switch {
		case strings.HasSuffix(trimmed, "*"):
			text = trimmed[:len(trimmed)-1]
		case strings.HasSuffix(trimmed, "]"):
			if open := strings.LastIndexByte(trimmed, '['); open >= 0 {
				text = trimmed[:open]
			} else {
				return ""
			}
		default:
			var words []string
			for _, word := range strings.Fields(trimmed) {
				if !typeQualifiers[word] {
					words = append(words, word)
				}
			}
			return strings.Join(words, " ")
		}
	}
}

func isAbsolute(file string) bool {
	return file == "" || strings.HasPrefix(file, "/") || strings.HasPrefix(file, "<") || len(file) > 1 && file[1] == ':'
}

// unwrapValue looks through the casts and parentheses that do not change
// which object an expression names. A pointer cast to an integer is an
// address used as a number, never a callable.
func unwrapValue(n *Node) *Node {
	for n != nil {
		switch n.Kind {
		case "ImplicitCastExpr", "CStyleCastExpr":
			if n.CastKind == "PointerToIntegral" || n.CastKind == "IntegralToPointer" || len(n.Inner) == 0 {
				return n
			}
			n = n.Inner[0]
		case "ParenExpr", "ConstantExpr":
			if len(n.Inner) == 0 {
				return n
			}
			n = n.Inner[0]
		default:
			return n
		}
	}
	return nil
}

// designator is the reference a value makes to a function (f, &f, (void *)f),
// or nil.
func designator(n *Node) *Node {
	n = unwrapValue(n)
	for n != nil && n.Kind == "UnaryOperator" && (n.Opcode == "&" || n.Opcode == "*") && len(n.Inner) == 1 {
		n = unwrapValue(n.Inner[0])
	}
	if n != nil && n.Kind == "DeclRefExpr" && n.ReferencedDecl != nil && n.ReferencedDecl.Kind == "FunctionDecl" {
		return n
	}
	return nil
}

func isNull(n *Node) bool {
	for n != nil {
		switch n.Kind {
		case "ImplicitCastExpr", "CStyleCastExpr":
			if n.CastKind == "NullToPointer" {
				return true
			}
			if len(n.Inner) == 0 {
				return false
			}
			n = n.Inner[0]
		case "ParenExpr", "ConstantExpr":
			if len(n.Inner) == 0 {
				return false
			}
			n = n.Inner[0]
		case "IntegerLiteral":
			return n.Value == "0"
		case "GNUNullExpr":
			return true
		default:
			return false
		}
	}
	return false
}

// parameterRef is the parameter a value names, or nil.
func parameterRef(n *Node) *Node {
	n = unwrapValue(n)
	if n != nil && n.Kind == "DeclRefExpr" && n.ReferencedDecl != nil && n.ReferencedDecl.Kind == "ParmVarDecl" {
		return n
	}
	return nil
}

func stringLiteral(n *Node) (*Node, string, bool) {
	n = unwrapValue(n)
	if n == nil || n.Kind != "StringLiteral" {
		return nil, "", false
	}
	value, ok := cString(n.Value)
	return n, value, ok
}

// cString decodes a C string literal as clang prints it ("a\n", L"x").
func cString(value string) (string, bool) {
	quote := strings.IndexByte(value, '"')
	if quote < 0 || !slices.Contains([]string{"", "L", "u", "U", "u8"}, value[:quote]) || len(value) < quote+2 || value[len(value)-1] != '"' {
		return "", false
	}
	body := value[quote+1 : len(value)-1]
	var out []byte
	for i := 0; i < len(body); i++ {
		c := body[i]
		if c != '\\' || i+1 == len(body) {
			out = append(out, c)
			continue
		}
		i++
		switch e := body[i]; e {
		case 'n':
			out = append(out, '\n')
		case 't':
			out = append(out, '\t')
		case 'r':
			out = append(out, '\r')
		case 'a':
			out = append(out, '\a')
		case 'b':
			out = append(out, '\b')
		case 'f':
			out = append(out, '\f')
		case 'v':
			out = append(out, '\v')
		case 'e':
			out = append(out, 0x1b)
		case 'x':
			v, n := 0, 0
			for i+1 < len(body) && isHex(body[i+1]) {
				i++
				v = v*16 + hexValue(body[i])
				n++
			}
			if n == 0 {
				return "", false
			}
			out = append(out, byte(v))
		case '0', '1', '2', '3', '4', '5', '6', '7':
			v := int(e - '0')
			for n := 1; n < 3 && i+1 < len(body) && body[i+1] >= '0' && body[i+1] <= '7'; n++ {
				i++
				v = v*8 + int(body[i]-'0')
			}
			out = append(out, byte(v))
		default: // \\ \" \' \?
			out = append(out, e)
		}
	}
	text := string(out)
	if !utf8.ValidString(text) {
		return "", false
	}
	return text, true
}

func isHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

func hexValue(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	default:
		return int(c-'A') + 10
	}
}

// A slot is where a program keeps a function pointer it later calls through:
// a record's field (every value of the record shares it), a parameter of a
// function, or a variable.
type slot struct {
	key  string
	name string // command.proc, parameter cmp of sort, variable handlers
	// function and position are set for a parameter slot.
	function string
	position int
}

// A store is one value a program puts into a slot.
type store struct {
	slot *slot
	// fn is the stored function's ref (a repository function or a platform
	// or package one); param a parameter of in stored as it is, joined at
	// dispatch with what in's callers pass; unknown is any other value. A
	// null store stores nothing callable and is not kept.
	fn      string
	param   int
	unknown bool
	in      string // function or table variable doing the store
	site    Position
	// conditional is a store under a branch (if, switch, ?:, && and ||) of
	// in: the value in the slot depends on a condition the index does not read.
	conditional bool
}

// passed is one argument a direct call of a repository function hands over.
type passed struct {
	fn   string // the function the argument names, the repository's or not
	null bool
	site Position
}

// walker walks one function body or variable initializer.
type walker struct {
	b           *builder
	scope       *unitScope
	owner       string // function or table variable ref
	function    *function
	loops       []programindex.Witness
	conditional bool
	// inList is set inside an initializer list initList already read.
	inList bool
	// unevaluated is set inside an operand the program never evaluates
	// (sizeof, _Alignof): naming a variable there reads nothing.
	unevaluated bool
}

func (w walker) with(conditional bool) walker {
	w.conditional = w.conditional || conditional
	return w
}

func (w walker) loop(kind string, statement *Node) walker {
	w.loops = append(slices.Clone(w.loops), programindex.Witness{Kind: "control_context", Detail: kind, Location: location(statement.Begin.Site())})
	return w
}

func (w walker) walk(n *Node) {
	if n == nil {
		return
	}
	switch n.Kind {
	case "CallExpr":
		if len(n.Inner) == 0 {
			return
		}
		w.b.call(w, n)
		callee := n.Inner[0]
		if designator(callee) == nil {
			w.walk(callee)
		}
		for _, argument := range n.Inner[1:] {
			w.walk(argument)
		}
		return
	case "BinaryOperator":
		if len(n.Inner) != 2 {
			break
		}
		switch n.Opcode {
		case "=":
			w.b.assign(w, n.Inner[0], n.Inner[1])
			// A variable that is itself the destination is written, not
			// read; a member or an element of it is reached through it.
			if !isVariable(n.Inner[0]) {
				w.walk(n.Inner[0])
			}
			if designator(n.Inner[1]) == nil {
				w.walk(n.Inner[1])
			}
			return
		case "&&", "||":
			w.walk(n.Inner[0])
			w.with(true).walk(n.Inner[1])
			return
		}
	case "IfStmt", "ConditionalOperator", "BinaryConditionalOperator":
		for i, child := range n.Inner {
			w.with(i > 0).walk(child)
		}
		return
	case "SwitchStmt":
		for i, child := range n.Inner {
			w.with(i == len(n.Inner)-1).walk(child)
		}
		return
	case "ForStmt":
		if len(n.Inner) == 5 {
			for _, child := range n.Inner[:4] {
				w.walk(child)
			}
			kind := "for body"
			if n.Inner[2].Kind == "" {
				kind = "for body without condition"
			}
			w.loop(kind, n).walk(n.Inner[4])
			return
		}
	case "WhileStmt":
		if len(n.Inner) >= 2 {
			condition := n.Inner[len(n.Inner)-2]
			w.walk(condition)
			kind := "while body"
			if literal := unwrapValue(condition); literal != nil && literal.Kind == "IntegerLiteral" && literal.Value != "0" {
				kind = "while body with constant true condition"
			}
			w.loop(kind, n).walk(n.Inner[len(n.Inner)-1])
			return
		}
	case "DoStmt":
		if len(n.Inner) == 2 {
			w.loop("do-while body", n).walk(n.Inner[0])
			w.walk(n.Inner[1])
			return
		}
	case "VarDecl":
		if value := initializer(n); value != nil {
			w.b.initialize(w, n, value)
			if designator(value) == nil {
				w.walk(value)
			}
		}
		return
	case "UnaryOperator":
		// &fp hands the slot itself over: whoever receives the address may
		// write any function into it.
		if n.Opcode == "&" && len(n.Inner) == 1 {
			w.b.escapeSlot(w, n.Inner[0], n.Begin.Site())
		}
	case "InitListExpr":
		if !w.inList {
			w.b.initList(w, n, "")
		}
		inner := w
		inner.inList = true
		for _, child := range n.Inner {
			if designator(child) == nil {
				inner.walk(child)
			}
		}
		return
	case "UnaryExprOrTypeTraitExpr":
		w.unevaluated = true
	case "DeclRefExpr":
		if w.function != nil && !w.unevaluated {
			w.b.read(w, n)
		}
		return
	case "FunctionDecl", "RecordDecl", "TypedefDecl", "EnumDecl":
		return
	}
	for _, child := range n.Inner {
		w.walk(child)
	}
}

// isVariable reports an expression that names a variable itself: x or (x),
// not x.field, x[i] or *x.
func isVariable(n *Node) bool {
	n = unwrapValue(n)
	return n != nil && n.Kind == "DeclRefExpr" && n.ReferencedDecl != nil && n.ReferencedDecl.Kind == "VarDecl"
}

// A read is a function body naming a file-scope variable of the program:
// its value, a member or an element of it, or its address.
type read struct {
	from, variable string
	site           Position
	macro          *programindex.Witness
}

// read records a function's use of a file-scope variable. A parameter, a
// local (static or not) and a platform variable are no program variable.
func (b *builder) read(w walker, n *Node) {
	ref := n.ReferencedDecl
	if ref == nil || ref.Kind != "VarDecl" {
		return
	}
	variable := b.variableRef(w.scope, ref)
	if variable == "" || b.objects[variable] == nil {
		return
	}
	r := read{from: w.owner, variable: variable, site: n.Begin.Site()}
	if n.Begin.InMacroBody() {
		if macro := TokenText(b.source(n.Begin.Expansion.File), n.Begin.Expansion); macro != "" {
			r.macro = &programindex.Witness{Kind: "macro_expansion", Detail: fmt.Sprintf("%s expands to a read of %s", macro, ref.Name), Location: location(n.Begin.Spelling)}
		}
	}
	b.reads = append(b.reads, r)
}

// initializer is a variable's initializing expression.
func initializer(n *Node) *Node {
	if n.Init == "" {
		return nil
	}
	for i := len(n.Inner) - 1; i >= 0; i-- {
		child := n.Inner[i]
		if child.Kind != "" && !strings.HasSuffix(child.Kind, "Attr") {
			return child
		}
	}
	return nil
}

// target is what a direct call or a designator names.
type target struct {
	ref        string // repository object, or external symbol object
	external   bool
	unresolved string // why nothing is known, for a repository declaration no unit defines
}

// resolveFunction finds the function a reference names, the way the linker
// would: an internal name is this unit's definition, an external one the
// program's definition, else the platform or package declaration clang read.
func (b *builder) resolveFunction(s *unitScope, ref *DeclRef) target {
	name := ref.Name
	if s.internal[name] {
		if node := s.statics[name]; node != nil && node.Kind == "FunctionDecl" {
			return target{ref: b.functionRef(s, node)}
		}
		return target{unresolved: fmt.Sprintf("%s is declared static and defined in no unit of %s", name, b.parsed.Program.Name)}
	}
	if definition, ok := b.externalFunctions[name]; ok {
		return target{ref: definition}
	}
	if declaration := s.external[ref.ID]; declaration != nil {
		return target{ref: b.externalSymbol("function", declaration.Name, declaration), external: true}
	}
	if declaration := s.platform[name]; declaration != nil {
		return target{ref: b.externalSymbol("function", declaration.Name, declaration), external: true}
	}
	if declaration := s.decls[ref.ID]; declaration != nil {
		at := declaration.Loc.Site()
		return target{unresolved: fmt.Sprintf("%s is declared at %s:%d and defined in no unit of %s", name, at.File, at.Line, b.parsed.Program.Name)}
	}
	if builtinName(name) {
		return target{ref: b.externalSymbol("function", name, nil), external: true}
	}
	return target{unresolved: fmt.Sprintf("%s is declared implicitly and defined in no unit of %s", name, b.parsed.Program.Name)}
}

func builtinName(name string) bool {
	for _, prefix := range []string{"__builtin_", "__sync_", "__atomic_", "__c11_atomic_"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// variableRef is the program variable a reference names, or "".
func (b *builder) variableRef(s *unitScope, ref *DeclRef) string {
	declaration := s.decls[ref.ID]
	if declaration == nil || declaration.Kind != "VarDecl" {
		return ""
	}
	if s.internal[ref.Name] {
		if node := s.statics[ref.Name]; node != nil && node.Kind == "VarDecl" {
			return b.variableKey(s, node)
		}
		return ""
	}
	return b.externalVariables[ref.Name]
}

// slotOf is the slot an expression reads or writes a function pointer
// through: a field, a parameter of the function being walked, a variable, or
// an element of an array variable.
func (b *builder) slotOf(w walker, n *Node) *slot {
	n = unwrapValue(n)
	for n != nil && n.Kind == "UnaryOperator" && n.Opcode == "*" && len(n.Inner) == 1 {
		n = unwrapValue(n.Inner[0])
	}
	if n == nil {
		return nil
	}
	switch n.Kind {
	case "MemberExpr":
		if field := w.scope.fields[n.ReferencedMemberDecl]; field != nil {
			record := field.record
			name := record.name
			if name == "" {
				name = record.tagUsed
			}
			return b.slot("field:"+record.key+"."+field.name, name+"."+field.name, "", 0)
		}
	case "ArraySubscriptExpr":
		if len(n.Inner) > 0 {
			if base := unwrapValue(n.Inner[0]); base != nil && base.Kind == "DeclRefExpr" && base.ReferencedDecl != nil && base.ReferencedDecl.Kind == "VarDecl" {
				return b.slotOf(w, base)
			}
		}
	case "DeclRefExpr":
		ref := n.ReferencedDecl
		if ref == nil {
			return nil
		}
		switch ref.Kind {
		case "ParmVarDecl":
			if param, ok := w.scope.params[ref.ID]; ok {
				return b.slot(fmt.Sprintf("param:%s:%d", param.function, param.position), fmt.Sprintf("parameter %s of %s", param.name, b.objects[param.function].Name), param.function, param.position)
			}
		case "VarDecl":
			if variable := b.variableRef(w.scope, ref); variable != "" {
				return b.slot("var:"+variable, "variable "+ref.Name, "", 0)
			}
			if w.function != nil {
				return b.slot("local:"+w.function.ref+":"+ref.ID, "local variable "+ref.Name, "", 0)
			}
		}
	}
	return nil
}

func (b *builder) slot(key, name, function string, position int) *slot {
	if existing := b.slots[key]; existing != nil {
		return existing
	}
	s := &slot{key: key, name: name, function: function, position: position}
	b.slots[key] = s
	return s
}

// store records what a value puts into a slot.
func (b *builder) store(w walker, into *slot, value *Node, site Position) *store {
	if into == nil || value == nil || isNull(value) {
		return nil
	}
	// copy->free = orig->free keeps what the slot already holds.
	if from := unwrapValue(value); from != nil && from.Kind != "DeclRefExpr" && b.slotOf(w, from) == into {
		return nil
	}
	st := &store{slot: into, in: w.owner, conditional: w.conditional, site: site}
	switch {
	case designator(value) != nil:
		d := designator(value)
		if resolved := b.resolveFunction(w.scope, d.ReferencedDecl); resolved.ref != "" {
			st.fn = resolved.ref
		} else {
			st.unknown = true
		}
		st.site = d.Begin.Site()
	case parameterRef(value) != nil && w.function != nil:
		param, ok := w.scope.params[parameterRef(value).ReferencedDecl.ID]
		if ok && param.function == w.function.ref {
			st.param = param.position
		} else {
			st.unknown = true
		}
	default:
		st.unknown = true
	}
	b.stores[into.key] = append(b.stores[into.key], st)
	return st
}

// escapeSlot records that the address of a slot leaves the code the index
// reads (&fp, or an array handed to a call): whatever is written through it
// is a value the index cannot name. clang prints a pointer to a function
// typedef (handler *) without its function type, so any slot counts; one
// never called through changes nothing.
func (b *builder) escapeSlot(w walker, value *Node, site Position) {
	value = unwrapValue(value)
	if value == nil {
		return
	}
	for _, text := range []string{value.Type.QualType, value.Type.Desugared} {
		if arrayType(text) && (strings.Contains(text, "*const") || strings.Contains(text, "* const")) {
			return // an array of constant pointers cannot be written through
		}
	}
	if into := b.slotOf(w, value); into != nil {
		b.stores[into.key] = append(b.stores[into.key], &store{slot: into, unknown: true, in: w.owner, site: site})
	}
}

// assign handles lhs = rhs: a function stored into a field or a variable.
func (b *builder) assign(w walker, lhs, rhs *Node) {
	into := b.slotOf(w, lhs)
	if into == nil {
		b.externalStore(w, lhs, rhs)
		return
	}
	if into.function != "" {
		// A parameter overwritten inside its own function: its callers no
		// longer say what it holds.
		if !isNull(rhs) {
			b.stores[into.key] = append(b.stores[into.key], &store{slot: into, unknown: true, in: w.owner, site: lhs.Begin.Site()})
		}
		return
	}
	st := b.store(w, into, rhs, rhs.Begin.Site())
	if st != nil && b.functionByRef[st.fn] != nil && !strings.HasPrefix(into.key, "local:") {
		b.bindings = append(b.bindings, binding{from: w.owner, fn: st.fn, site: st.site, detail: fmt.Sprintf("%s stored in %s", b.objects[st.fn].Name, into.name)})
	}
}

// initialize handles a variable's initializer: a function pointer variable
// set to a function, or a table.
func (b *builder) initialize(w walker, variable, value *Node) {
	if unwrapValue(value) != nil && unwrapValue(value).Kind == "InitListExpr" {
		return // walked as an initializer list
	}
	ref := &DeclRef{ID: variable.ID, Kind: "VarDecl", Name: variable.Name}
	var into *slot
	if key := b.variableRef(w.scope, ref); key != "" {
		into = b.slot("var:"+key, "variable "+variable.Name, "", 0)
	} else if w.function != nil {
		into = b.slot("local:"+w.function.ref+":"+variable.ID, "local variable "+variable.Name, "", 0)
	}
	if into != nil {
		b.store(w, into, value, value.Begin.Site())
	}
}

// initList reads an initializer list: each record row stores its function
// designators into the record's fields, and an array of function pointers
// stores its elements into the array variable. table is the variable a
// module-level initializer belongs to.
func (b *builder) initList(w walker, list *Node, table string) {
	if list == nil || list.Kind != "InitListExpr" {
		return
	}
	if arrayType(list.Type.QualType) {
		for _, child := range list.Inner {
			if inner := unwrapValue(child); inner != nil && inner.Kind == "InitListExpr" {
				b.initList(w, inner, table)
			} else if d := designator(child); d != nil && table != "" {
				into := b.slot("var:"+table, "variable "+b.objects[table].Name, "", 0)
				if st := b.store(w, into, child, d.Begin.Site()); st != nil && b.functionByRef[st.fn] != nil {
					b.bindings = append(b.bindings, binding{from: table, fn: st.fn, site: st.site, detail: fmt.Sprintf("%s stored in %s", b.objects[st.fn].Name, into.name)})
				}
			}
		}
		return
	}
	record := w.scope.record(list.Type)
	if record == nil || record.tagUsed != "struct" {
		return
	}
	row := tableRow{table: table, owner: w.owner, record: record, begin: list.Begin.Site()}
	// clang's initializer list has one element per member; an unnamed
	// bit-field (int :3) is padding, not a member.
	var members []*fieldInfo
	for _, field := range record.fields {
		if field.name != "" || !field.node.IsBitfield {
			members = append(members, field)
		}
	}
	for i, child := range list.Inner {
		if i >= len(members) {
			break
		}
		field := members[i]
		if inner := unwrapValue(child); inner != nil && inner.Kind == "InitListExpr" {
			b.initList(w, inner, table)
			continue
		}
		if literal, value, ok := stringLiteral(child); ok && field.name != "" {
			row.literals = append(row.literals, rowLiteral{field: field.name, value: value, site: literal.Begin.Site()})
			continue
		}
		d := designator(child)
		if d == nil || field.name == "" {
			continue
		}
		name := record.name
		if name == "" {
			name = record.tagUsed
		}
		into := b.slot("field:"+record.key+"."+field.name, name+"."+field.name, "", 0)
		if st := b.store(w, into, child, d.Begin.Site()); st != nil && b.functionByRef[st.fn] != nil {
			row.stored = append(row.stored, rowStore{field: field.name, fn: st.fn, site: st.site, slot: into})
		}
	}
	if len(row.stored) > 0 {
		b.rows = append(b.rows, row)
	}
}

// arrayType reports a printed array type: struct cmd[3], or void (*[2])(void)
// for an array of function pointers.
func arrayType(text string) bool {
	text = strings.TrimSpace(text)
	return strings.HasSuffix(text, "]") || strings.Contains(text, "(*[") || strings.Contains(text, "(*const [")
}

type tableRow struct {
	table    string // module-level variable, or "" for a row a function builds
	owner    string // the table, or the function building the row
	record   *recordInfo
	begin    Position
	literals []rowLiteral
	stored   []rowStore
}

type rowLiteral struct {
	field, value string
	site         Position
}

type rowStore struct {
	field string
	fn    string
	site  Position
	slot  *slot
}

// externalStore handles a function stored into a field of a record the
// platform or a package declares (act.sa_sigaction = handler): the record is
// named by the type of the value written before the field, the field as the
// source spells it (sa_sigaction is a macro for a member of a union).
func (b *builder) externalStore(w walker, lhs, rhs *Node) {
	member := unwrapValue(lhs)
	d := designator(rhs)
	if member == nil || member.Kind != "MemberExpr" || d == nil || len(member.Inner) == 0 {
		return
	}
	resolved := b.resolveFunction(w.scope, d.ReferencedDecl)
	if resolved.ref == "" || resolved.external {
		return
	}
	written := member.End.Expansion
	base := member.Inner[0]
	for {
		inner := unwrapValue(base)
		if inner == nil || inner.Kind != "MemberExpr" || inner.End.Expansion != written || len(inner.Inner) == 0 {
			break
		}
		base = inner.Inner[0]
	}
	container := baseType(unwrapValue(base).Type.QualType)
	declaration := w.scope.outside[container]
	if declaration == nil && unwrapValue(base).Type.Desugared != "" {
		container = baseType(unwrapValue(base).Type.Desugared)
		declaration = w.scope.outside[container]
	}
	field := b.writtenField(member)
	if declaration == nil || field == "" || location(written) == nil {
		return
	}
	b.constructs = append(b.constructs, construct{
		from: w.owner, container: container, declaration: declaration, field: field, fn: resolved.ref,
		site: written, fnSite: d.Begin.Site(),
	})
}

// writtenField is a member's field as the source spells it after its . or
// ->: at the expansion (act.sa_sigaction, where sa_sigaction is the platform's
// macro for a union member), else in the body of the repository macro that
// wrote the store. When neither place shows it, it is clang's member name.
func (b *builder) writtenField(member *Node) string {
	for _, at := range []Position{member.End.Expansion, member.End.Spelling} {
		if isAbsolute(at.File) {
			continue
		}
		data := b.source(at.File)
		token := TokenText(data, at)
		before := bytes.TrimRight(data[:min(at.Offset, len(data))], " \t\r\n\\")
		if token != "" && (bytes.HasSuffix(before, []byte(".")) || bytes.HasSuffix(before, []byte("->"))) {
			return token
		}
	}
	return member.Name
}

type construct struct {
	from        string
	container   string // struct sigaction
	declaration *ExternalDecl
	field       string
	fn          string
	site        Position // the field as written
	fnSite      Position
}

// binding is a function stored into a field or variable by a function body or
// a module-level initializer.
type binding struct {
	from, fn string
	site     Position
	detail   string
}

// call records one call site. Its relation is emitted after every body is
// walked, when function-pointer stores are all known.
type call struct {
	relationRef, patternRef string
	from                    string
	site                    Position
	selector                string
	macro                   *programindex.Witness
	context                 []programindex.Witness
	arguments               []programindex.PatternArgumentInput
	designators             map[int]designated // argument position -> function it names
	// direct is a callee the call names; otherwise slot, when known, is
	// where the called pointer is kept.
	direct     target
	slot       *slot
	expression string // the callee as written, for a call through a pointer
}

type designated struct {
	fn   string
	site Position
}

// call records a call site: its callee, selector, macro, loop context and
// arguments.
func (b *builder) call(w walker, n *Node) {
	if len(n.Inner) == 0 || w.owner == "" {
		return
	}
	b.sequence++
	c := &call{relationRef: fmt.Sprintf("c:call:%d", b.sequence), patternRef: fmt.Sprintf("c:call:%d:pattern", b.sequence),
		from: w.owner, context: slices.Clone(w.loops), designators: map[int]designated{}}
	callee := n.Inner[0]
	named := designator(callee)
	start := callee.Begin
	if named != nil {
		start = named.Begin
	}
	c.site = start.Site()
	if start.InMacroBody() {
		macro := TokenText(b.source(start.Expansion.File), start.Expansion)
		if macro != "" {
			c.selector = macro
			target := "a call"
			if named != nil {
				target = "a call of " + named.ReferencedDecl.Name
			}
			c.macro = &programindex.Witness{Kind: "macro_expansion", Detail: fmt.Sprintf("%s expands to %s", macro, target), Location: location(start.Spelling)}
		}
	}
	if named != nil {
		c.direct = b.resolveFunction(w.scope, named.ReferencedDecl)
		if c.selector == "" {
			c.selector = named.ReferencedDecl.Name
		}
	} else {
		c.slot = b.slotOf(w, callee)
		c.expression = strings.Join(strings.Fields(b.text(callee)), " ")
		if c.selector == "" {
			c.selector = writtenName(unwrapValue(callee))
		}
		if c.selector == "" {
			c.selector = c.expression
		}
	}
	if c.selector == "" {
		c.selector = "(call)"
	}
	for i, argument := range n.Inner[1:] {
		position := i + 1
		value := programindex.PatternArgumentInput{Position: position, Kind: programindex.PatternDynamic, Origin: b.origin(w, argument)}
		if _, text, ok := stringLiteral(argument); ok {
			value.Kind, value.Value = programindex.PatternLiteralString, text
		} else if d := designator(argument); d != nil {
			resolved := b.resolveFunction(w.scope, d.ReferencedDecl)
			if resolved.ref != "" {
				value.ObjectRefs, value.Resolution, value.ObjectsObserved = []string{resolved.ref}, programindex.ResolutionExact, 1
				if !resolved.external {
					c.designators[position] = designated{fn: resolved.ref, site: d.Begin.Site()}
				}
			}
		}
		c.arguments = append(c.arguments, value)
		// An array handed to a call is its address.
		if array := unwrapValue(argument); array != nil && (arrayType(array.Type.QualType) || arrayType(array.Type.Desugared)) {
			b.escapeSlot(w, array, argument.Begin.Site())
		}
		if c.direct.ref != "" && !c.direct.external {
			b.passedTo[c.direct.ref] = append(b.passedTo[c.direct.ref], passedAt{position: position, value: b.passedValue(w, argument)})
		}
	}
	b.calls = append(b.calls, c)
}

type passedAt struct {
	position int
	value    passed
}

// passedValue is what an argument hands a repository function's parameter.
func (b *builder) passedValue(w walker, argument *Node) passed {
	value := passed{site: argument.Begin.Site(), null: isNull(argument)}
	if d := designator(argument); d != nil {
		if resolved := b.resolveFunction(w.scope, d.ReferencedDecl); resolved.ref != "" {
			value.fn, value.site = resolved.ref, d.Begin.Site()
		}
	}
	return value
}

// writtenName is the name a callee expression ends in: the field of
// cmd->proc, the parameter cmp.
func writtenName(n *Node) string {
	for n != nil {
		switch n.Kind {
		case "MemberExpr":
			return n.Name
		case "DeclRefExpr":
			if n.ReferencedDecl != nil {
				return n.ReferencedDecl.Name
			}
			return ""
		case "ArraySubscriptExpr", "UnaryOperator":
			if len(n.Inner) == 0 {
				return ""
			}
			n = unwrapValue(n.Inner[0])
		default:
			return ""
		}
	}
	return ""
}

// origin is an argument's source expression: a literal, a parameter of the
// function, the result of a call written there, or its text.
func (b *builder) origin(w walker, argument *Node) *sourcevalue.Value {
	site := argument.Begin.Site()
	anchor := sourceAnchor(site)
	if anchor == nil {
		return nil
	}
	text := strings.ToValidUTF8(b.text(argument), "�")
	if _, value, ok := stringLiteral(argument); ok {
		return &sourcevalue.Value{Kind: "literal", Text: value, Anchor: anchor}
	}
	if p := parameterRef(argument); p != nil && w.function != nil {
		if param, ok := w.scope.params[p.ReferencedDecl.ID]; ok && param.function == w.function.ref && w.function.location != nil {
			owner := sourceAnchor(Position{File: w.function.location.Path, Line: w.function.location.Line, Col: w.function.location.Column})
			return &sourcevalue.Value{Kind: "parameter", Text: param.name, Position: param.position, Owner: owner, Anchor: anchor}
		}
	}
	if inner := unwrapValue(argument); inner != nil && inner.Kind == "CallExpr" && len(inner.Inner) > 0 {
		callee := inner.Inner[0]
		start := callee.Begin
		if named := designator(callee); named != nil {
			start = named.Begin
		}
		if at := sourceAnchor(start.Site()); at != nil {
			return &sourcevalue.Value{Kind: "call_result", Text: text, Anchor: at}
		}
	}
	return &sourcevalue.Value{Kind: "unknown", Text: text, Anchor: anchor}
}

// findEscapes records the repository functions used as values anywhere in the
// program: stored, passed, returned or put in a table. Such a function may be
// called through a pointer with arguments the index does not see, so what
// its direct callers pass is not all its parameters can hold. Calling a
// function, comparing it with a pointer or keeping its address as a number
// hands nothing callable over.
func (b *builder) findEscapes() {
	var visit func(scope *unitScope, n *Node)
	visit = func(scope *unitScope, n *Node) {
		if n == nil {
			return
		}
		skipDesignators := func(children []*Node) {
			for _, child := range children {
				if designator(child) == nil {
					visit(scope, child)
				}
			}
		}
		switch n.Kind {
		case "CallExpr":
			if len(n.Inner) > 0 {
				skipDesignators(n.Inner[:1])
				for _, argument := range n.Inner[1:] {
					visit(scope, argument)
				}
			}
			return
		case "BinaryOperator":
			switch n.Opcode {
			case "==", "!=", "<", ">", "<=", ">=":
				skipDesignators(n.Inner)
				return
			}
		case "ImplicitCastExpr", "CStyleCastExpr":
			if n.CastKind == "PointerToIntegral" {
				skipDesignators(n.Inner)
				return
			}
		case "DeclRefExpr":
			if ref := n.ReferencedDecl; ref != nil && ref.Kind == "FunctionDecl" {
				if fn := b.repositoryFunction(scope, ref); fn != "" {
					b.escaped[fn] = true
				}
			}
			return
		case "FunctionDecl", "RecordDecl", "TypedefDecl", "EnumDecl":
			return
		}
		for _, child := range n.Inner {
			visit(scope, child)
		}
	}
	for _, fn := range b.functions {
		for _, child := range fn.node.Inner {
			if child.Kind == "CompoundStmt" {
				visit(fn.scope, child)
			}
		}
	}
	for _, t := range b.tables {
		visit(t.scope, initializer(t.node))
	}
}

// repositoryFunction is the repository function a reference names, or "",
// without creating an object for a platform one.
func (b *builder) repositoryFunction(s *unitScope, ref *DeclRef) string {
	if s.internal[ref.Name] {
		if node := s.statics[ref.Name]; node != nil && node.Kind == "FunctionDecl" {
			return b.functionRef(s, node)
		}
		return ""
	}
	return b.externalFunctions[ref.Name]
}

// candidate is one function a slot may hold, with where it enters the slot.
type candidate struct {
	fn          string
	site        Position
	detail      string
	conditional bool
}

// candidates are the functions a slot can hold when it is called: the
// functions stored into it, and for a parameter, or a store of a parameter,
// the functions each direct caller passes there. uncertain is a value the
// index cannot name (another pointer, a call's result, a parameter a caller
// passes on); conditional a store under a branch.
func (b *builder) candidates(s *slot) (result []candidate, uncertain, conditional bool) {
	seen := map[string]bool{}
	add := func(c candidate) {
		key := fmt.Sprintf("%s\x00%s:%d:%d\x00%t", c.fn, c.site.File, c.site.Line, c.site.Col, c.conditional)
		if !seen[key] {
			seen[key] = true
			result = append(result, c)
		}
		conditional = conditional || c.conditional
	}
	join := func(function string, position int, via string, isConditional bool) {
		// A function used as a value is also called through pointers, with
		// arguments no direct call shows.
		if b.escaped[function] {
			uncertain = true
		}
		for _, at := range b.passedTo[function] {
			if at.position != position || at.value.null {
				continue
			}
			if at.value.fn == "" {
				uncertain = true
				continue
			}
			detail := fmt.Sprintf("%s passed to %s", b.objects[at.value.fn].Name, b.objects[function].Name)
			if via != "" {
				detail = fmt.Sprintf("%s stored in %s by %s", b.objects[at.value.fn].Name, via, b.objects[function].Name)
			}
			if isConditional {
				detail += " under a condition"
			}
			add(candidate{fn: at.value.fn, site: at.value.site, detail: detail, conditional: isConditional})
		}
	}
	if s.function != "" {
		join(s.function, s.position, "", false)
	}
	for _, st := range b.stores[s.key] {
		switch {
		case st.unknown:
			uncertain = true
		case st.fn != "":
			detail := fmt.Sprintf("%s stored in %s", b.objects[st.fn].Name, s.name)
			if st.conditional {
				detail += " under a condition"
			}
			add(candidate{fn: st.fn, site: st.site, detail: detail, conditional: st.conditional})
		case st.param > 0:
			join(st.in, st.param, s.name, st.conditional)
		}
	}
	slices.SortStableFunc(result, func(x, y candidate) int {
		if c := strings.Compare(x.site.File, y.site.File); c != 0 {
			return c
		}
		if x.site.Line != y.site.Line {
			return x.site.Line - y.site.Line
		}
		return x.site.Col - y.site.Col
	})
	return result, uncertain, conditional
}

func (b *builder) slotOrder() []string {
	if len(b.slotKeys) != len(b.slots) {
		b.slotKeys = b.slotKeys[:0]
		for key := range b.slots {
			b.slotKeys = append(b.slotKeys, key)
		}
		slices.Sort(b.slotKeys)
	}
	return b.slotKeys
}

// moduleDirectory is the directory a file module lives in.
func moduleDirectory(file string) string { return path.Dir(file) }
