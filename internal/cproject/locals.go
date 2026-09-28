package cproject

import (
	"encoding/json"
	"strings"

	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// A local variable's value at a read, the way the Go (SSA), Python and JS
// adapters follow a local: the writes that reach the read in the same
// function. The nearest write before the read that every path to the read
// passes through (its dominating write) stands, with every other write that
// can come between it and the read: one in a branch after it, or one later
// in a loop that holds the read and not that write. One value is that
// value; several are alternatives, each once, in source order, as the Go
// adapter records a join (φ). Without a dominating write the writes before
// the read are alternatives beside the value the variable holds on a path
// that writes none, which is not followed. A static local (it keeps its
// value between calls), an array (its contents are written through its
// address) and a variable whose address the function takes are not
// followed. This reads the syntax tree; it evaluates nothing.

// localWrite is one write of a local: its declaration's initializer, an
// assignment, or a write computed from the old value (x += 1, x++), whose
// value is nil.
type localWrite struct {
	node  *Node
	value *Node
}

// functionLocals indexes one function body for following its locals.
type functionLocals struct {
	parent map[*Node]*Node
	order  map[*Node]int
	locals map[string]*Node // VarDecl by clang ID
	writes map[string][]localWrite
	taken  map[string]bool // address taken: written through a pointer
	// labels are the positions of the statements a jump can reach from
	// elsewhere (goto labels, case and default labels).
	labels []int
}

// locals indexes a function's body once.
func (f *function) localsIndex() *functionLocals {
	if f.locals != nil {
		return f.locals
	}
	l := &functionLocals{parent: map[*Node]*Node{}, order: map[*Node]int{}, locals: map[string]*Node{}, writes: map[string][]localWrite{}, taken: map[string]bool{}}
	var visit func(parent, n *Node)
	visit = func(parent, n *Node) {
		if n == nil {
			return
		}
		l.parent[n] = parent
		l.order[n] = len(l.order)
		switch n.Kind {
		case "VarDecl":
			if n != f.node && n.StorageClass == "" && !arrayType(n.Type.QualType) && !arrayType(n.Type.Desugared) {
				l.locals[n.ID] = n
				if value := initializer(n); value != nil {
					l.writes[n.ID] = append(l.writes[n.ID], localWrite{node: n, value: value})
				}
			}
		case "BinaryOperator":
			if n.Opcode == "=" && len(n.Inner) == 2 {
				if id := localRef(n.Inner[0]); id != "" {
					l.writes[id] = append(l.writes[id], localWrite{node: n, value: n.Inner[1]})
				}
			}
		case "CompoundAssignOperator":
			if len(n.Inner) > 0 {
				if id := localRef(n.Inner[0]); id != "" {
					l.writes[id] = append(l.writes[id], localWrite{node: n})
				}
			}
		case "UnaryOperator":
			if len(n.Inner) == 1 {
				if id := localRef(n.Inner[0]); id != "" {
					switch n.Opcode {
					case "++", "--":
						l.writes[id] = append(l.writes[id], localWrite{node: n})
					case "&":
						l.taken[id] = true
					}
				}
			}
		case "LabelStmt", "CaseStmt", "DefaultStmt":
			l.labels = append(l.labels, l.order[n])
		case "FunctionDecl", "RecordDecl", "TypedefDecl", "EnumDecl":
			if n != f.node {
				return
			}
		}
		for _, child := range n.Inner {
			visit(n, child)
		}
	}
	visit(nil, f.node)
	f.locals = l
	return l
}

// localRef is the clang ID of the variable an expression names itself (x or
// (x)), or "".
func localRef(n *Node) string {
	n = unwrapValue(n)
	if n == nil || n.Kind != "DeclRefExpr" || n.ReferencedDecl == nil || n.ReferencedDecl.Kind != "VarDecl" {
		return ""
	}
	return n.ReferencedDecl.ID
}

// followLocal is the value the local a read names holds there, or nil when
// the read names no local of the function the index can follow.
func (b *builder) followLocal(w walker, read *Node, active map[string]bool) *sourcevalue.Value {
	if w.function == nil {
		return nil
	}
	ref := unwrapValue(read)
	id := localRef(ref)
	if id == "" {
		return nil
	}
	l := w.function.localsIndex()
	if _, indexed := l.order[ref]; !indexed || l.locals[id] == nil || l.taken[id] || active[id] {
		return nil
	}
	writes := l.writes[id]
	if len(writes) == 0 {
		return nil
	}
	at := l.order[ref]
	holds := map[*Node]bool{}
	for n := ref; n != nil; n = l.parent[n] {
		holds[n] = true
	}
	// A write holding the read (x = x + 1) reads before it writes.
	dominating := -1
	for i, write := range writes {
		if l.order[write.node] < at && !holds[write.node] && l.dominates(write.node, ref, holds) {
			dominating = i
		}
	}
	var reaching []localWrite
	for i, write := range writes {
		position := l.order[write.node]
		switch {
		case holds[write.node]:
			continue
		case i == dominating:
		case position < at && (dominating < 0 || position > l.order[writes[dominating].node]):
		case position > at && l.backEdge(write.node, ref, holds, writes, dominating):
		default:
			continue
		}
		reaching = append(reaching, write)
	}
	active[id] = true
	defer delete(active, id)
	var values []*sourcevalue.Value
	for _, write := range reaching {
		if write.value == nil {
			values = append(values, &sourcevalue.Value{Kind: "unknown", Text: b.written(write.node), Anchor: sourceAnchor(write.node.Begin.Site())})
			continue
		}
		values = append(values, b.originOf(w, write.value, active))
	}
	if dominating < 0 {
		// A path that writes nothing on the way holds the value the
		// variable had before, which is not followed.
		values = append(values, &sourcevalue.Value{Kind: "unknown", Text: b.written(ref), Anchor: sourceAnchor(ref.Begin.Site())})
	}
	return alternativesOf(values, sourceAnchor(ref.Begin.Site()))
}

// dominates reports a write every path to the read passes through: the
// declaration's own initializer, or a write that its enclosing code always
// evaluates up to the first statement or expression holding both, where it
// comes first, with no label between them a jump could enter by.
func (l *functionLocals) dominates(write, read *Node, holds map[*Node]bool) bool {
	if write.Kind == "VarDecl" {
		return true
	}
	for _, label := range l.labels {
		if label > l.order[write] && label < l.order[read] {
			return false
		}
	}
	child := write
	for {
		parent := l.parent[child]
		if parent == nil {
			return false
		}
		if holds[parent] {
			return evaluatedBefore(parent, child, childHolding(parent, holds))
		}
		if !alwaysEvaluates(parent, child) {
			return false
		}
		child = parent
	}
}

// backEdge reports a write after the read that reaches it again: it stands
// in the repeated part of a loop holding the read whose repeated part does
// not hold the dominating write.
func (l *functionLocals) backEdge(write, read *Node, holds map[*Node]bool, writes []localWrite, dominating int) bool {
	for loop := l.parent[read]; loop != nil; loop = l.parent[loop] {
		repeated := repeatedPart(loop)
		if repeated == nil {
			continue
		}
		inside := func(n *Node) bool {
			for ; n != nil; n = l.parent[n] {
				if l.parent[n] == loop {
					return repeated[n]
				}
			}
			return false
		}
		if !inside(read) || !inside(write) {
			continue
		}
		if dominating < 0 || !inside(writes[dominating].node) {
			return true
		}
	}
	return false
}

// repeatedPart is, for a loop, the children it runs again: a for loop's
// condition, increment and body, a while or do loop's whole.
func repeatedPart(loop *Node) map[*Node]bool {
	switch loop.Kind {
	case "ForStmt":
		if len(loop.Inner) != 5 {
			return nil
		}
		return map[*Node]bool{loop.Inner[2]: true, loop.Inner[3]: true, loop.Inner[4]: true}
	case "WhileStmt", "DoStmt":
		repeated := map[*Node]bool{}
		for _, child := range loop.Inner {
			repeated[child] = true
		}
		return repeated
	}
	return nil
}

// childHolding is the child of parent on the way to the read.
func childHolding(parent *Node, holds map[*Node]bool) *Node {
	for _, child := range parent.Inner {
		if holds[child] {
			return child
		}
	}
	return nil
}

// alwaysEvaluates reports a child its parent evaluates whenever the parent
// itself is evaluated: an operand, a statement of a block, a condition, a
// for loop's initializer, a do loop's body. A branch, a loop body, the right
// operand of && and || and a switch's body are not.
func alwaysEvaluates(parent, child *Node) bool {
	index := -1
	for i, c := range parent.Inner {
		if c == child {
			index = i
		}
	}
	switch parent.Kind {
	case "CompoundStmt", "DeclStmt", "ReturnStmt", "ParenExpr", "ImplicitCastExpr", "CStyleCastExpr", "ConstantExpr", "ExprWithCleanups",
		"UnaryOperator", "MemberExpr", "ArraySubscriptExpr", "CallExpr", "CompoundAssignOperator", "VarDecl", "LabelStmt", "CaseStmt", "DefaultStmt":
		return true
	case "BinaryOperator":
		return index == 0 || parent.Opcode != "&&" && parent.Opcode != "||"
	case "ConditionalOperator", "BinaryConditionalOperator", "IfStmt", "SwitchStmt":
		return index == 0
	case "WhileStmt":
		return index == len(parent.Inner)-2
	case "DoStmt":
		return index == 0
	case "ForStmt":
		return index == 0 || index == 2
	}
	return false
}

// evaluatedBefore reports that parent always evaluates first, the child
// holding the write before the child holding the read.
func evaluatedBefore(parent, write, read *Node) bool {
	if write == nil || read == nil || write == read {
		return false
	}
	index := func(n *Node) int {
		for i, c := range parent.Inner {
			if c == n {
				return i
			}
		}
		return -1
	}
	w, r := index(write), index(read)
	switch parent.Kind {
	case "CompoundStmt":
		return w < r
	case "BinaryOperator":
		return w == 0 && r == 1 && (parent.Opcode == "&&" || parent.Opcode == "||" || parent.Opcode == ",")
	case "ConditionalOperator", "BinaryConditionalOperator", "IfStmt", "SwitchStmt":
		return w == 0 && r > 0
	case "WhileStmt":
		return w == len(parent.Inner)-2 && r == len(parent.Inner)-1
	case "DoStmt":
		return w == 0 && r == 1
	case "ForStmt":
		return w == 0 && r > 0 || w == 2 && r > 2
	}
	return false
}

// written is an expression as the source writes it, on one line.
func (b *builder) written(n *Node) string {
	return strings.Join(strings.Fields(strings.ToValidUTF8(b.text(n), "�")), " ")
}

// alternativesOf is one value, or the alternatives of several, each once
// by what it says (two increments of a counter are one), in order.
func alternativesOf(values []*sourcevalue.Value, anchor *sourcevalue.Anchor) *sourcevalue.Value {
	seen := map[string]bool{}
	result := &sourcevalue.Value{Kind: "alternatives", Anchor: anchor}
	for _, value := range values {
		key := contentKey(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		result.Parts = append(result.Parts, *value)
	}
	if len(result.Parts) == 1 {
		return &result.Parts[0]
	}
	return result
}

// contentKey is what a value says, without where it is written.
func contentKey(value *sourcevalue.Value) string {
	var strip func(v *sourcevalue.Value)
	strip = func(v *sourcevalue.Value) {
		v.Anchor = nil
		v.Owner = nil
		for i := range v.Parts {
			strip(&v.Parts[i])
		}
		if v.Initializer != nil {
			strip(v.Initializer)
		}
	}
	copied := sourcevalue.Clone(value)
	strip(copied)
	raw, _ := json.Marshal(copied)
	return string(raw)
}
