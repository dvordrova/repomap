package surfacediscovery

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/tools/go/types/typeutil"
)

// sameValueCall is, for a call, the earlier call it reads the same value as
// (PROGRAM_INDEX RelationPattern.SameValueAs): the first call of its group,
// by the site of its opening parenthesis, the site SSA gives a call.
//
// A group is the calls of one callee, each written the same but for the
// string literals it is given, that one piece of code reads as spellings of
// one value (litestream's replica URL options):
//
//   - the operands of one || written the same around their call:
//     query.Get("forcePathStyle") != "" || query.Get("force-path-style") != "";
//   - the arms of one if/else-if chain whose headers are written the same
//     but for their call's words and whose bodies are written the same:
//     if v := query.Get("storageClass"); v != "" { storageClass = v } else if
//     v := query.Get("storage-class"); v != "" { storageClass = v }.
//
// && is none: q.Get("user") != "" && q.Get("password") != "" reads two
// values. A code fact of where the joined call is; what the value is, and
// whether each call is an input at all, stays the reading's.
func (a *analyzer) sameValueCall(callsite Location) (Location, bool) {
	if a.sameValues == nil {
		a.sameValues = make(map[Location]Location)
		for name, pkg := range a.packageFacts {
			if !a.admittedPackages[name] || pkg.TypesInfo == nil {
				continue
			}
			for _, file := range pkg.Syntax {
				a.recordSameValueCalls(pkg.TypesInfo, file)
			}
		}
	}
	root, ok := a.sameValues[callsite]
	return root, ok
}

// spelledRead is one operand or arm of a candidate group: its one call
// given string literals, its words, and its text with those literals left
// out, which the group's members share.
type spelledRead struct {
	call   *ast.CallExpr
	callee types.Object
	shape  string
	words  string
}

func (a *analyzer) recordSameValueCalls(info *types.Info, file *ast.File) {
	var source []byte
	sourceRead := false
	text := func(from, to token.Pos) (string, bool) {
		start := a.program.Fset.PositionFor(from, false)
		end := a.program.Fset.PositionFor(to, false)
		if !sourceRead {
			sourceRead = true
			if path, err := filepath.Rel(a.root, start.Filename); err == nil && path != ".." && !strings.HasPrefix(path, ".."+string(filepath.Separator)) {
				source, _ = os.ReadFile(start.Filename)
			}
		}
		if source == nil || start.Filename != end.Filename || start.Offset < 0 || end.Offset < start.Offset || end.Offset > len(source) {
			return "", false
		}
		return string(source[start.Offset:end.Offset]), true
	}
	// read is the one call of a piece of code given a string literal, and
	// the code's text with that call's literals left out.
	read := func(from, to token.Pos, nodes ...ast.Node) (spelledRead, bool) {
		var found []*ast.CallExpr
		closure := false
		for _, node := range nodes {
			ast.Inspect(node, func(child ast.Node) bool {
				switch child := child.(type) {
				case *ast.FuncLit:
					closure = true
					return false
				case *ast.CallExpr:
					if len(stringLiteralArguments(info, child)) > 0 {
						found = append(found, child)
					}
				}
				return true
			})
		}
		if closure || len(found) != 1 {
			return spelledRead{}, false
		}
		call := found[0]
		callee := typeutil.Callee(info, call)
		written, ok := text(from, to)
		if callee == nil || !ok {
			return spelledRead{}, false
		}
		// The literals are left out from the last, so earlier offsets hold.
		base := a.program.Fset.PositionFor(from, false).Offset
		literals := stringLiteralArguments(info, call)
		var words []string
		for i := len(literals) - 1; i >= 0; i-- {
			start := a.program.Fset.PositionFor(literals[i].Pos(), false).Offset - base
			end := a.program.Fset.PositionFor(literals[i].End(), false).Offset - base
			if start < 0 || end < start || end > len(written) {
				return spelledRead{}, false
			}
			words = append(words, written[start:end])
			written = written[:start] + "\x00" + written[end:]
		}
		slices.Reverse(words)
		return spelledRead{call: call, callee: callee, shape: strings.Join(strings.Fields(written), " "), words: strings.Join(words, "\x00")}, true
	}
	record := func(reads []spelledRead) {
		for i, first := range reads {
			if first.call == nil {
				continue
			}
			root := a.location(first.call.Lparen)
			if !validRepositoryDirectCallLocation(root) {
				continue
			}
			if _, joined := a.sameValues[root]; joined {
				continue
			}
			for j := i + 1; j < len(reads); j++ {
				later := reads[j]
				if later.call == nil || later.callee != first.callee || later.shape != first.shape || later.words == first.words {
					continue
				}
				at := a.location(later.call.Lparen)
				if _, joined := a.sameValues[at]; validRepositoryDirectCallLocation(at) && !joined && at != root {
					a.sameValues[at] = root
				}
			}
		}
	}
	chained := make(map[*ast.IfStmt]bool)
	joined := make(map[*ast.BinaryExpr]bool)
	ast.Inspect(file, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.BinaryExpr:
			if n.Op != token.LOR || joined[n] {
				return true
			}
			var operands []ast.Expr
			var flatten func(ast.Expr)
			flatten = func(expression ast.Expr) {
				if or, ok := ast.Unparen(expression).(*ast.BinaryExpr); ok && or.Op == token.LOR {
					joined[or] = true
					flatten(or.X)
					flatten(or.Y)
					return
				}
				operands = append(operands, expression)
			}
			flatten(n)
			reads := make([]spelledRead, len(operands))
			for i, operand := range operands {
				reads[i], _ = read(operand.Pos(), operand.End(), operand)
			}
			record(reads)
		case *ast.IfStmt:
			if chained[n] {
				return true
			}
			var reads []spelledRead
			for arm := n; arm != nil; {
				chained[arm] = true
				from := arm.Cond.Pos()
				nodes := []ast.Node{arm.Cond}
				if arm.Init != nil {
					from = arm.Init.Pos()
					nodes = append(nodes, arm.Init)
				}
				header, ok := read(from, arm.Cond.End(), nodes...)
				body, bodyOK := text(arm.Body.Lbrace, arm.Body.End())
				if ok && bodyOK {
					header.shape += "\x00" + strings.Join(strings.Fields(body), " ")
				} else {
					header = spelledRead{}
				}
				reads = append(reads, header)
				next, _ := arm.Else.(*ast.IfStmt)
				arm = next
			}
			if len(reads) > 1 {
				record(reads)
			}
		}
		return true
	})
}

// stringLiteralArguments are the arguments of a call written as string
// literals, in order.
func stringLiteralArguments(info *types.Info, call *ast.CallExpr) []*ast.BasicLit {
	var result []*ast.BasicLit
	for _, argument := range call.Args {
		literal, ok := ast.Unparen(argument).(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			continue
		}
		if value := info.Types[literal].Value; value == nil || value.Kind() != constant.String {
			continue
		}
		result = append(result, literal)
	}
	return result
}
