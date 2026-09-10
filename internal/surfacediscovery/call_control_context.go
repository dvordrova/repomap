package surfacediscovery

import (
	"go/ast"
	"go/types"
)

// ControlContext is lexical source context, not a claim that a function is a
// worker or that a loop runs forever. Only calls in the statement body inherit
// it; a range expression, for initializer, or nested callable does not.
type ControlContext struct {
	Kind     string   `json:"kind"`
	Location Location `json:"location"`
}

func (a *analyzer) callControlContext(callsite Location) []ControlContext {
	if a.callControls == nil {
		a.callControls = make(map[Location][]ControlContext)
		for name, pkg := range a.packageFacts {
			if !a.admittedPackages[name] {
				continue
			}
			var walk func(ast.Node, []ControlContext)
			walk = func(node ast.Node, context []ControlContext) {
				if node == nil {
					return
				}
				body := func(node ast.Node, kind string, statements ast.Node) {
					next := append(append([]ControlContext(nil), context...), ControlContext{Kind: kind, Location: a.location(node.Pos())})
					walk(statements, next)
				}
				switch n := node.(type) {
				case *ast.FuncDecl:
					if n.Body != nil {
						walk(n.Body, nil)
					}
					return
				case *ast.FuncLit:
					if n.Body != nil {
						walk(n.Body, nil)
					}
					return
				case *ast.ForStmt:
					walk(n.Init, context)
					walk(n.Cond, context)
					walk(n.Post, context)
					kind := "for body"
					if n.Cond == nil {
						kind = "for body without condition"
					}
					body(n, kind, n.Body)
					return
				case *ast.RangeStmt:
					walk(n.X, context)
					kind := "range body"
					if t := pkg.TypesInfo.TypeOf(n.X); t != nil {
						if _, ok := t.Underlying().(*types.Chan); ok {
							kind = "range body over channel"
						}
					}
					body(n, kind, n.Body)
					return
				case *ast.SelectStmt:
					kind := "select without default"
					for _, statement := range n.Body.List {
						if statement.(*ast.CommClause).Comm == nil {
							kind = "select with default"
						}
					}
					body(n, kind, n.Body)
					return
				case *ast.CallExpr:
					if len(context) > 0 {
						for _, position := range []Location{a.location(n.Pos()), a.location(n.Lparen)} {
							if validRepositoryDirectCallLocation(position) {
								a.callControls[position] = append([]ControlContext(nil), context...)
							}
						}
					}
				}
				ast.Inspect(node, func(child ast.Node) bool {
					if child == node {
						return true
					}
					if child != nil {
						walk(child, context)
					}
					return false
				})
			}
			for _, file := range pkg.Syntax {
				walk(file, nil)
			}
		}
	}
	return append([]ControlContext(nil), a.callControls[callsite]...)
}
