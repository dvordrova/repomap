package surfacediscovery

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"sort"
	"strconv"

	"golang.org/x/tools/go/ssa"
)

// comparedWord is one word a function body compares a value with: a switch
// case's constant, or the constant side of an ==.
type comparedWord struct {
	key, text string
	// value is the compared operand's SSA value, nil when SSA wrote no
	// comparison at the site.
	value      ssa.Value
	at         ssa.Instruction
	word       string
	pos        token.Pos
	form       string
	branch     [2]int
	caseSyntax ast.Node
	// exclusive: the branch runs only when the value is one of the case's
	// words (onlyWhenCompared).
	exclusive bool
}

// recordComparisons records each value the body of function compares with
// two or more different string words in two or more cases (GO): the cases of a switch on the
// value and the == comparisons of the same expression with a word, in one
// comparison per expression. A lone comparison is none. The typed syntax
// gives the cases, the words and the lines each case selects; SSA, which
// compares a switch's tag at each case expression and an == at its
// operator, gives where the compared value comes from (sourceValue), so
// `cmd, args = args[0], args[1:]` reads as the first element of args.
func (a *analyzer) recordComparisons(function *ssa.Function, callerID string) {
	if a == nil || a.directCallIndex == nil || function == nil || callerID == "" {
		return
	}
	var body *ast.BlockStmt
	switch syntax := function.Syntax().(type) {
	case *ast.FuncDecl:
		body = syntax.Body
	case *ast.FuncLit:
		body = syntax.Body
	}
	facts := a.packageFacts[functionPackagePath(function)]
	if body == nil || facts == nil || facts.TypesInfo == nil {
		return
	}
	info := facts.TypesInfo
	compares := make(map[token.Pos]*ssa.BinOp)
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if op, ok := instruction.(*ssa.BinOp); ok && op.Op == token.EQL && op.Pos().IsValid() {
				compares[op.Pos()] = op
			}
		}
	}
	var words []comparedWord
	var parents []ast.Node
	ast.Inspect(body, func(node ast.Node) bool {
		if node == nil {
			parents = parents[:len(parents)-1]
			return true
		}
		// A function literal is a closure of its own.
		if _, closure := node.(*ast.FuncLit); closure {
			return false
		}
		switch statement := node.(type) {
		case *ast.SwitchStmt:
			words = append(words, a.switchWords(info, compares, statement)...)
		case *ast.BinaryExpr:
			if word, ok := a.equalsWord(info, compares, parents, statement); ok {
				words = append(words, word)
			}
		}
		parents = append(parents, node)
		return true
	})
	a.recordComparedWords(callerID, words)
}

// stringType reports a value of a string type, named or not.
func stringType(value types.Type) bool {
	if value == nil {
		return false
	}
	basic, ok := value.Underlying().(*types.Basic)
	return ok && basic.Info()&types.IsString != 0
}

// constantWord is the string an expression's constant value holds.
func constantWord(info *types.Info, expression ast.Expr) (string, bool) {
	value := info.Types[expression].Value
	if value == nil || value.Kind() != constant.String {
		return "", false
	}
	return constant.StringVal(value), true
}

// comparedKey names one compared expression within a function: a variable
// by its declaration, anything else by its text.
func comparedKey(info *types.Info, expression ast.Expr) string {
	expression = ast.Unparen(expression)
	if ident, ok := expression.(*ast.Ident); ok {
		if object := info.Uses[ident]; object != nil {
			return ident.Name + "@" + strconv.Itoa(int(object.Pos()))
		}
	}
	return types.ExprString(expression)
}

func (a *analyzer) lines(from, to token.Pos) [2]int {
	first, last := a.location(from), a.location(to)
	if first.Line < 1 || last.Line < first.Line || first.Path != last.Path {
		return [2]int{}
	}
	return [2]int{first.Line, last.Line}
}

// switchWords are a switch's words on a string tag: each case's constant
// expressions, the case clause as the lines it selects.
func (a *analyzer) switchWords(info *types.Info, compares map[token.Pos]*ssa.BinOp, statement *ast.SwitchStmt) []comparedWord {
	if statement.Tag == nil || !stringType(info.TypeOf(statement.Tag)) || info.Types[statement.Tag].Value != nil {
		return nil
	}
	key, text := comparedKey(info, statement.Tag), types.ExprString(ast.Unparen(statement.Tag))
	var result []comparedWord
	for position, clause := range statement.Body.List {
		clause, ok := clause.(*ast.CaseClause)
		if !ok {
			continue
		}
		branch := a.lines(clause.Case, clause.End())
		exclusive := !fallsInto(statement.Body.List, position)
		for _, expression := range clause.List {
			word, ok := constantWord(info, expression)
			if !ok {
				continue
			}
			item := comparedWord{key: key, text: text, word: word, pos: expression.Pos(), form: "case", branch: branch, caseSyntax: clause, exclusive: exclusive}
			if op := compares[expression.Pos()]; op != nil {
				item.value, item.at = op.X, op
			}
			result = append(result, item)
		}
	}
	return result
}

// equalsWord is the word of an == between a string value and a string
// constant. The lines it selects are the block of the if statement whose
// condition holds it, or the case clause of a tagless switch, through
// parentheses, || and &&; comparisons in one such condition are one case.
func (a *analyzer) equalsWord(info *types.Info, compares map[token.Pos]*ssa.BinOp, parents []ast.Node, expression *ast.BinaryExpr) (comparedWord, bool) {
	if expression.Op != token.EQL {
		return comparedWord{}, false
	}
	compared, literal, constantRight := expression.X, expression.Y, true
	word, ok := constantWord(info, literal)
	if !ok {
		compared, literal, constantRight = expression.Y, expression.X, false
		if word, ok = constantWord(info, literal); !ok {
			return comparedWord{}, false
		}
	}
	if info.Types[compared].Value != nil || !stringType(info.TypeOf(compared)) {
		return comparedWord{}, false
	}
	item := comparedWord{key: comparedKey(info, compared), text: types.ExprString(ast.Unparen(compared)), word: word, pos: literal.Pos(), form: "equals", caseSyntax: expression}
	if op := compares[expression.OpPos]; op != nil {
		item.value, item.at = op.X, op
		if !constantRight {
			item.value = op.Y
		}
	}
	var child ast.Node = expression
walk:
	for position := len(parents) - 1; position >= 0; position-- {
		switch parent := parents[position].(type) {
		case *ast.ParenExpr:
		case *ast.BinaryExpr:
			if parent.Op != token.LOR && parent.Op != token.LAND {
				break walk
			}
		case *ast.IfStmt:
			if parent.Cond == child {
				item.branch, item.caseSyntax = a.lines(parent.Body.Lbrace, parent.Body.Rbrace), parent
				item.exclusive = onlyWhenCompared(info, parent.Cond, item.key)
			}
			break walk
		case *ast.CaseClause:
			for _, listed := range parent.List {
				if listed == child {
					item.branch, item.caseSyntax = a.lines(parent.Case, parent.End()), parent
					item.exclusive = true
					for _, other := range parent.List {
						item.exclusive = item.exclusive && onlyWhenCompared(info, other, item.key)
					}
					if position > 0 {
						if body, ok := parents[position-1].(*ast.BlockStmt); ok {
							for at, clause := range body.List {
								if clause == parent && fallsInto(body.List, at) {
									item.exclusive = false
								}
							}
						}
					}
				}
			}
			break walk
		default:
			break walk
		}
		child = parents[position]
	}
	return item, true
}

// recordComparedWords groups a body's words by compared expression, a case
// per switch clause or if condition, and records each expression compared
// with two or more different non-empty words in two or more cases: one if
// condition naming several words (arg == "-h" || arg == "-help") is one
// comparison, no dispatch.
func (a *analyzer) recordComparedWords(callerID string, words []comparedWord) {
	byKey := make(map[string][]comparedWord)
	for _, word := range words {
		byKey[word.key] = append(byKey[word.key], word)
	}
	for _, group := range byKey {
		sort.SliceStable(group, func(i, j int) bool { return group[i].pos < group[j].pos })
		distinct := make(map[string]bool)
		branches := make(map[ast.Node]bool)
		for _, word := range group {
			if word.word != "" {
				distinct[word.word] = true
				branches[word.caseSyntax] = true
			}
		}
		if len(distinct) < 2 || len(branches) < 2 {
			continue
		}
		site := a.location(group[0].pos)
		if !validRepositoryDirectCallLocation(site) || site.Column <= 0 {
			continue
		}
		comparison := DirectCallComparison{CallerID: callerID, Value: group[0].text, Site: site}
		for _, word := range group {
			if word.value != nil {
				comparison.Origin = a.sourceValue(word.value, make(map[ssa.Value]bool), word.at)
				break
			}
		}
		cases := make(map[ast.Node]int)
		for _, word := range group {
			if at, seen := cases[word.caseSyntax]; seen {
				comparison.Cases[at].Words = append(comparison.Cases[at].Words, word.word)
				continue
			}
			location := a.location(word.pos)
			if !validRepositoryDirectCallLocation(location) || location.Column <= 0 {
				continue
			}
			cases[word.caseSyntax] = len(comparison.Cases)
			comparison.Cases = append(comparison.Cases, DirectCallComparisonCase{
				Form: word.form, Words: []string{word.word}, Site: location, BranchLine: word.branch[0], BranchEnd: word.branch[1], Exclusive: word.exclusive,
			})
		}
		a.directCallIndex.recordComparison(comparison)
	}
}

// onlyWhenCompared says a condition holds only when the value keyed key
// equals one of the words it is compared with: an == of the value with a
// constant, such comparisons joined by ||, or a conjunction one of whose
// sides is one. Anything else (`len(v) != 2 || v == "nu"`, a call) may hold
// for other values.
func onlyWhenCompared(info *types.Info, condition ast.Expr, key string) bool {
	switch expression := ast.Unparen(condition).(type) {
	case *ast.BinaryExpr:
		switch expression.Op {
		case token.LOR:
			return onlyWhenCompared(info, expression.X, key) && onlyWhenCompared(info, expression.Y, key)
		case token.LAND:
			return onlyWhenCompared(info, expression.X, key) || onlyWhenCompared(info, expression.Y, key)
		case token.EQL:
			for _, pair := range [][2]ast.Expr{{expression.X, expression.Y}, {expression.Y, expression.X}} {
				if _, word := constantWord(info, pair[1]); word && info.Types[pair[0]].Value == nil && comparedKey(info, pair[0]) == key {
					return true
				}
			}
		}
	}
	return false
}

// fallsInto says the clause before the one at position ends in fallthrough,
// so that clause's values run this one's body too.
func fallsInto(clauses []ast.Stmt, position int) bool {
	if position == 0 {
		return false
	}
	previous, ok := clauses[position-1].(*ast.CaseClause)
	if !ok || len(previous.Body) == 0 {
		return false
	}
	branch, ok := previous.Body[len(previous.Body)-1].(*ast.BranchStmt)
	return ok && branch.Tok == token.FALLTHROUGH
}
