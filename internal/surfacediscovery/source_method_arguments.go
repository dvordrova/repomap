package surfacediscovery

import (
	"go/ast"
	"go/types"
	"os"
	"path/filepath"
	"strings"

	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// sourceMethodArguments retains the written method values passed at an exact
// call site. SSA may represent these as synthetic bound-method wrappers with
// no indexed callable identity. Their spelling is still source evidence; it
// does not resolve the wrapper or assert that the receiving call invokes it.
func (a *analyzer) sourceMethodArguments(callsite Location, count int) []*sourcevalue.Value {
	if a.methodArguments == nil {
		a.methodArguments = make(map[Location][]*sourcevalue.Value)
		for name, pkg := range a.packageFacts {
			if !a.admittedPackages[name] || pkg.TypesInfo == nil {
				continue
			}
			for _, file := range pkg.Syntax {
				var source []byte
				ast.Inspect(file, func(node ast.Node) bool {
					call, ok := node.(*ast.CallExpr)
					// A written spread is one source expression, while SSA may
					// expand its stored array. Do not match those slots by index.
					if !ok || call.Ellipsis.IsValid() {
						return true
					}
					at := a.location(call.Lparen)
					if !validRepositoryDirectCallLocation(at) {
						return true
					}
					var arguments []*sourcevalue.Value
					for i, expr := range call.Args {
						selector, ok := ast.Unparen(expr).(*ast.SelectorExpr)
						if !ok {
							continue
						}
						selection := pkg.TypesInfo.Selections[selector]
						if selection == nil || selection.Kind() != types.MethodVal || containsFunctionLiteral(expr) {
							continue
						}
						start := a.program.Fset.PositionFor(expr.Pos(), false)
						end := a.program.Fset.PositionFor(expr.End(), false)
						path, err := filepath.Rel(a.root, start.Filename)
						if err != nil || path == ".." || strings.HasPrefix(path, ".."+string(filepath.Separator)) || end.Filename != start.Filename {
							continue // compiler-generated source is not an authored expression
						}
						if source == nil {
							source, err = os.ReadFile(start.Filename)
							if err != nil {
								// The typed call remains available; unavailable
								// source text supplies no expression evidence.
								return false
							}
						}
						anchor := a.valueAnchor(expr.Pos())
						if anchor == nil || start.Offset < 0 || end.Offset < start.Offset || end.Offset > len(source) {
							continue
						}
						if arguments == nil {
							arguments = make([]*sourcevalue.Value, len(call.Args))
						}
						arguments[i] = &sourcevalue.Value{Kind: "unknown", Text: string(source[start.Offset:end.Offset]), Anchor: anchor}
					}
					if arguments != nil {
						a.methodArguments[at] = arguments
					}
					return true
				})
			}
		}
	}
	if arguments := a.methodArguments[callsite]; len(arguments) == count {
		return arguments
	}
	return nil
}

func containsFunctionLiteral(expr ast.Expr) bool {
	found := false
	ast.Inspect(expr, func(node ast.Node) bool {
		if _, ok := node.(*ast.FuncLit); ok {
			found = true
		}
		return !found
	})
	return found
}
