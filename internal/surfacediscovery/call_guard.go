package surfacediscovery

import (
	"go/ast"
	"go/token"
	"go/types"
)

// CallGuard is the strongest construct of its function a call runs under
// (ProgramIndex Guard): CallGuardBranch an if or else arm, a case or select
// clause, or the right operand of && or ||; CallGuardError a failing path,
// what builtin panic is handed, an arm ending in panic, or the arm taken when
// a value of the predeclared error type is not nil (`if err != nil`, the else
// of `if err == nil`, a conjunct of the condition). A call in a condition, a
// switch's tag or a case's list is in no arm; a function literal's body
// starts afresh. Location is the construct's.
type CallGuard struct {
	Kind     string   `json:"kind"`
	Location Location `json:"location"`
}

// The Go call guard kinds.
const (
	CallGuardBranch = "branch"
	CallGuardError  = "error"
)

func callGuardStrength(guard *CallGuard) int {
	switch {
	case guard == nil:
		return 0
	case guard.Kind == CallGuardBranch:
		return 1
	default:
		return 2
	}
}

// weakerCallGuard folds two sites of one call edge: an unguarded site
// leaves none, else the weaker stands.
func weakerCallGuard(a, b *CallGuard) *CallGuard {
	if a == nil || b == nil {
		return nil
	}
	if callGuardStrength(b) < callGuardStrength(a) {
		return b
	}
	return a
}

// callGuard is the guard a call site runs under, or nil.
func (a *analyzer) callGuard(callsite Location) *CallGuard {
	if a.callGuards == nil {
		a.buildCallGuards()
	}
	if guard, ok := a.callGuards[callsite]; ok {
		copied := guard
		return &copied
	}
	return nil
}

// buildCallGuards walks the target's packages once, as callControlContext
// does, keeping each call's guard by its position and its parenthesis.
func (a *analyzer) buildCallGuards() {
	a.callGuards = make(map[Location]CallGuard)
	panicObject, errorType := types.Universe.Lookup("panic"), types.Universe.Lookup("error").Type()
	for name, pkg := range a.packageFacts {
		if !a.admittedPackages[name] || pkg.TypesInfo == nil {
			continue
		}
		info := pkg.TypesInfo
		isPanic := func(call *ast.CallExpr) bool {
			ident, ok := ast.Unparen(call.Fun).(*ast.Ident)
			return ok && info.Uses[ident] == panicObject
		}
		// An arm ends in panic when its last statement panics and no
		// return before it leaves the function normally.
		endsInPanic := func(block *ast.BlockStmt) bool {
			if block == nil || len(block.List) == 0 {
				return false
			}
			statement, ok := block.List[len(block.List)-1].(*ast.ExprStmt)
			if !ok {
				return false
			}
			call, ok := ast.Unparen(statement.X).(*ast.CallExpr)
			if !ok || !isPanic(call) {
				return false
			}
			returns := false
			ast.Inspect(block, func(node ast.Node) bool {
				switch node.(type) {
				case *ast.FuncLit:
					return false
				case *ast.ReturnStmt:
					returns = true
				}
				return !returns
			})
			return !returns
		}
		isError := func(expression ast.Expr) bool {
			value := info.TypeOf(expression)
			return value != nil && types.Identical(value, errorType)
		}
		isNil := func(expression ast.Expr) bool {
			ident, ok := ast.Unparen(expression).(*ast.Ident)
			return ok && info.Uses[ident] == types.Universe.Lookup("nil")
		}
		// errorTest says a condition, or one of its && conjuncts, compares a
		// value of the error type with nil by op.
		var errorTest func(condition ast.Expr, op token.Token) bool
		errorTest = func(condition ast.Expr, op token.Token) bool {
			binary, ok := ast.Unparen(condition).(*ast.BinaryExpr)
			if !ok {
				return false
			}
			if binary.Op == token.LAND && op == token.NEQ {
				return errorTest(binary.X, op) || errorTest(binary.Y, op)
			}
			return binary.Op == op && (isError(binary.X) && isNil(binary.Y) || isError(binary.Y) && isNil(binary.X))
		}
		stronger := func(outer *CallGuard, kind string, at token.Pos) *CallGuard {
			inner := &CallGuard{Kind: kind, Location: a.location(at)}
			if callGuardStrength(outer) > callGuardStrength(inner) {
				return outer
			}
			return inner
		}
		var walk func(node ast.Node, guard *CallGuard)
		walk = func(node ast.Node, guard *CallGuard) {
			if node == nil {
				return
			}
			switch n := node.(type) {
			case *ast.FuncDecl:
				if n.Body != nil {
					walk(n.Body, nil)
				}
				return
			case *ast.FuncLit:
				walk(n.Body, nil)
				return
			case *ast.IfStmt:
				walk(n.Init, guard)
				walk(n.Cond, guard)
				kind := CallGuardBranch
				if errorTest(n.Cond, token.NEQ) || endsInPanic(n.Body) {
					kind = CallGuardError
				}
				walk(n.Body, stronger(guard, kind, n.Pos()))
				if n.Else != nil {
					kind = CallGuardBranch
					if block, ok := n.Else.(*ast.BlockStmt); errorTest(n.Cond, token.EQL) || ok && endsInPanic(block) {
						kind = CallGuardError
					}
					walk(n.Else, stronger(guard, kind, n.Else.Pos()))
				}
				return
			case *ast.CaseClause:
				for _, expression := range n.List {
					walk(expression, guard)
				}
				kind := CallGuardBranch
				if endsInPanic(&ast.BlockStmt{List: n.Body}) {
					kind = CallGuardError
				}
				for _, statement := range n.Body {
					walk(statement, stronger(guard, kind, n.Pos()))
				}
				return
			case *ast.CommClause:
				walk(n.Comm, guard)
				for _, statement := range n.Body {
					walk(statement, stronger(guard, CallGuardBranch, n.Pos()))
				}
				return
			case *ast.BinaryExpr:
				if n.Op == token.LAND || n.Op == token.LOR {
					walk(n.X, guard)
					walk(n.Y, stronger(guard, CallGuardBranch, n.Pos()))
					return
				}
			case *ast.CallExpr:
				if guard != nil {
					for _, position := range []Location{a.location(n.Pos()), a.location(n.Lparen)} {
						if validRepositoryDirectCallLocation(position) {
							a.callGuards[position] = *guard
						}
					}
				}
				if isPanic(n) {
					inner := stronger(guard, CallGuardError, n.Pos())
					for _, argument := range n.Args {
						walk(argument, inner)
					}
					return
				}
			}
			ast.Inspect(node, func(child ast.Node) bool {
				if child == node {
					return true
				}
				if child != nil {
					walk(child, guard)
				}
				return false
			})
		}
		for _, file := range pkg.Syntax {
			walk(file, nil)
		}
	}
}
