package cproject

import (
	"strings"

	p "github.com/dvordrova/repomap/internal/programindex"
)

// arm is a construct of a function whose code runs only when its condition
// decides so (an if or else arm, a ?: arm, the right operand of && or ||, a
// switch's body), at the construct's own place; node is the arm's code, or
// nil when no single statement holds it (a switch's cases); condition is
// the code deciding it and when the outcome it runs on (Guard.When), none
// for an a ?: b's arms.
type arm struct {
	site      Position
	node      *Node
	condition *Node
	when      string
}

// findEnding finds the corpus functions that never return: every path of
// the body ends in a call that never returns, a function outside the corpus
// whose type says noreturn (longjmp, abort, exit as the platform declares
// them) or another such corpus function. A function its author declares
// noreturn is taken at its word while its body agrees, so a throw that
// re-throws through itself stays one (Lua 5.4's luaD_throw, l_noret, ends in
// longjmp, a re-throw or abort); another is found from the calls it ends in
// (Lua 5.1.5's luaD_throw ends in longjmp or exit, its luaG_runerror in
// luaG_errormsg, which ends in luaD_throw). A body looping forever ends in
// no call, so it is none, nor is a function only calling itself unless its
// author declares it noreturn.
func (b *builder) findEnding() {
	b.ending = map[string]bool{}
	declared := map[string]bool{}
	for _, fn := range b.functions {
		if declaredNoReturn(fn.node) {
			declared[fn.ref], b.ending[fn.ref] = true, true
		}
	}
	bodyEnds := func(fn *function) bool {
		for _, child := range fn.node.Inner {
			if child.Kind == "CompoundStmt" && b.ends(fn.scope, child) {
				return true
			}
		}
		return false
	}
	for round, changed := 0, true; changed && round <= len(b.functions); round++ {
		changed = false
		for _, fn := range b.functions {
			ends := bodyEnds(fn)
			switch {
			case b.ending[fn.ref] && !ends:
				delete(b.ending, fn.ref)
				changed = true
			case !b.ending[fn.ref] && ends && !declared[fn.ref]:
				b.ending[fn.ref] = true
				changed = true
			}
		}
	}
}

// declaredNoReturn says a function's declaration says it never returns: its
// type carries the GNU attribute, or the declaration C11's _Noreturn.
func declaredNoReturn(n *Node) bool {
	if noreturnType(n) {
		return true
	}
	for _, child := range n.Inner {
		if child.Kind == "C11NoReturnAttr" {
			return true
		}
	}
	return false
}

// ends says a statement never completes: a call that never returns, a block
// reaching one at its top level before any statement that may leave
// normally, an if whose both arms end, or a do { } while (0) whose body ends
// (a macro's statement). A statement that may return, or jump away, before
// it leaves the function normally: `if (x) return; abort();` returns when x
// holds (control review, 2026-10-03), so only what no path leaves normally
// ends. A loop is never said to end.
func (b *builder) ends(scope *unitScope, n *Node) bool {
	if n == nil {
		return false
	}
	switch n.Kind {
	case "CompoundStmt":
		for _, child := range n.Inner {
			if b.mayLeave(child) {
				return false
			}
			if b.ends(scope, child) {
				return true
			}
		}
		return false
	case "IfStmt":
		return n.HasElse && len(n.Inner) >= 3 && b.ends(scope, n.Inner[len(n.Inner)-2]) && b.ends(scope, n.Inner[len(n.Inner)-1])
	case "DoStmt":
		if len(n.Inner) == 2 {
			if literal := unwrapValue(n.Inner[1]); literal != nil && literal.Kind == "IntegerLiteral" && literal.Value == "0" {
				return !b.mayLeave(n.Inner[0]) && b.ends(scope, n.Inner[0])
			}
		}
		return false
	}
	call := unwrapValue(n)
	return call != nil && call.Kind == "CallExpr" && b.callEnds(scope, call)
}

// mayLeave says a statement holds a way out of the function other than
// ending: a return, or a goto whose label the walk does not follow. It is
// a fact of the statement, kept once.
func (b *builder) mayLeave(n *Node) bool {
	if n == nil {
		return false
	}
	if known, ok := b.leaves[n]; ok {
		return known
	}
	leaves := n.Kind == "ReturnStmt" || n.Kind == "GotoStmt" || n.Kind == "IndirectGotoStmt"
	for _, child := range n.Inner {
		if leaves {
			break
		}
		leaves = b.mayLeave(child)
	}
	if b.leaves == nil {
		b.leaves = map[*Node]bool{}
	}
	b.leaves[n] = leaves
	return leaves
}

// callEnds says a call never returns: its callee is a corpus function that
// ends (findEnding) or a function outside the corpus whose type says
// noreturn.
func (b *builder) callEnds(scope *unitScope, call *Node) bool {
	if len(call.Inner) == 0 {
		return false
	}
	named := designator(call.Inner[0])
	if named == nil {
		return false
	}
	resolved := b.resolveFunction(scope, named.ReferencedDecl)
	switch {
	case resolved.ref == "":
		return false
	case resolved.external:
		return noreturnType(call.Inner[0]) || noreturnType(named)
	default:
		return b.ending[resolved.ref]
	}
}

// noreturnType says a function's type, as clang prints it, never returns
// (`__attribute__((noreturn))`, which the platform's __dead2 spells).
func noreturnType(n *Node) bool {
	return n != nil && (strings.Contains(n.Type.QualType, "noreturn") || strings.Contains(n.Type.Desugared, "noreturn"))
}

// guard is the strongest construct a call runs under (programindex.Guard): a
// call of a function that never returns, at the call; else the innermost
// arm ending in one, at that arm's construct; else the innermost arm, a
// branch. A call in a condition itself is in no arm.
func (b *builder) guard(w walker, call *Node) *p.Guard {
	if b.callEnds(w.scope, call) {
		return &p.Guard{Kind: p.GuardNoReturn, Location: location(call.Begin.Site())}
	}
	for i := len(w.arms) - 1; i >= 0; i-- {
		if w.arms[i].node != nil && b.armEnds(w.scope, w.arms[i].node) {
			return b.armGuard(p.GuardNoReturn, w.arms[i])
		}
	}
	if len(w.arms) > 0 {
		return b.armGuard(p.GuardBranch, w.arms[len(w.arms)-1])
	}
	return nil
}

// armGuard is a guard at an arm's construct, with its condition's code as
// the reader wrote it.
func (b *builder) armGuard(kind string, a arm) *p.Guard {
	guard := &p.Guard{Kind: kind, Location: location(a.site)}
	if a.condition != nil {
		guard.Condition, guard.When = p.GuardCondition(b.text(a.condition), a.when)
	}
	return guard
}

// armEnds is ends for an arm once the corpus's ending functions are known,
// kept per arm: every call in an arm asks it.
func (b *builder) armEnds(scope *unitScope, n *Node) bool {
	if known, ok := b.armEnding[n]; ok {
		return known
	}
	if b.armEnding == nil {
		b.armEnding = map[*Node]bool{}
	}
	b.armEnding[n] = b.ends(scope, n)
	return b.armEnding[n]
}
