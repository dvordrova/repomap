package cproject

import (
	"slices"
	"strings"
)

// Spellings of one read (ProgramIndex RelationPattern.SameValueAs): a call
// of the same function written the same but for the string literals it is
// given, either another operand of one || written the same around its call
// (`strcasecmp(argv[0], "dbfilename") == 0 || strcasecmp(argv[0],
// "dbfile") == 0`), or the condition of another arm of one if/else-if chain
// written the same but for its call's words, whose statement is written the
// same. Each later call names the first. && is none: both are needed.

// joinOperands marks the || operators nested in an unmarked || as one
// chain's and returns the chain's operands, or nil for a nested one.
func (b *builder) joinOperands(n *Node) []*Node {
	if b.joined[n] {
		return nil
	}
	var operands []*Node
	var flatten func(*Node)
	flatten = func(node *Node) {
		inner := node
		for inner != nil && inner.Kind == "ParenExpr" && len(inner.Inner) == 1 {
			inner = inner.Inner[0]
		}
		if inner != nil && inner.Kind == "BinaryOperator" && inner.Opcode == "||" && len(inner.Inner) == 2 {
			b.joined[inner] = true
			flatten(inner.Inner[0])
			flatten(inner.Inner[1])
			return
		}
		operands = append(operands, node)
	}
	flatten(n)
	return operands
}

// chainArms marks the arms of an if/else-if chain its head starts and
// returns them, or nil for an arm another head holds.
func (b *builder) chainArms(n *Node) []*Node {
	if b.joined[n] {
		return nil
	}
	var arms []*Node
	for arm := n; arm != nil && arm.Kind == "IfStmt" && len(arm.Inner) >= 2; {
		b.joined[arm] = true
		arms = append(arms, arm)
		if !arm.HasElse || len(arm.Inner) < 3 {
			break
		}
		arm = arm.Inner[2]
	}
	return arms
}

// spelledRead is one operand or arm: its one call given string literals,
// its text with those literals left out and the literals as written.
type spelledRead struct {
	call         *call
	shape, words string
}

// spelled is the one call of a piece of code given a string literal and
// the code's text with that call's literals left out; statement, when
// given, is written after it.
func (b *builder) spelled(code, statement *Node) (spelledRead, bool) {
	var found []*Node
	var visit func(*Node)
	visit = func(node *Node) {
		if node == nil {
			return
		}
		if node.Kind == "CallExpr" && len(node.Inner) > 1 {
			for _, argument := range node.Inner[1:] {
				if _, _, ok := stringLiteral(argument); ok {
					found = append(found, node)
					break
				}
			}
		}
		for _, child := range node.Inner {
			visit(child)
		}
	}
	visit(code)
	if len(found) != 1 {
		return spelledRead{}, false
	}
	recorded := b.callOf[found[0]]
	begin := code.Begin.Site()
	written := b.text(code)
	if recorded == nil || recorded.direct.ref == "" || recorded.macro != nil || written == "" {
		return spelledRead{}, false
	}
	var literals [][2]int
	for _, argument := range found[0].Inner[1:] {
		literal, _, ok := stringLiteral(argument)
		if !ok {
			continue
		}
		from, to := literal.Begin.Site(), literal.End.Site()
		if from.File != begin.File || to.File != begin.File {
			return spelledRead{}, false
		}
		start, end := from.Offset-begin.Offset, to.Offset+to.TokLen-begin.Offset
		if start < 0 || end < start || end > len(written) {
			return spelledRead{}, false
		}
		literals = append(literals, [2]int{start, end})
	}
	var words []string
	for i := len(literals) - 1; i >= 0; i-- {
		words = append(words, written[literals[i][0]:literals[i][1]])
		written = written[:literals[i][0]] + "\x00" + written[literals[i][1]:]
	}
	slices.Reverse(words)
	shape := strings.Join(strings.Fields(written), " ")
	if statement != nil {
		body := b.text(statement)
		if body == "" {
			return spelledRead{}, false
		}
		shape += "\x00" + strings.Join(strings.Fields(body), " ")
	}
	return spelledRead{call: recorded, shape: shape, words: strings.Join(words, "\x00")}, true
}

// sameTarget says two calls name the same target.
func sameTarget(a, b target) bool {
	return a.ref == b.ref && a.external == b.external && a.unresolved == b.unresolved && slices.Equal(a.alternatives, b.alternatives)
}

// joinSpellings gives each later read of a group the first read's call.
func joinSpellings(reads []spelledRead) {
	for i, first := range reads {
		if first.call == nil || first.call.sameValueAs != nil {
			continue
		}
		for _, later := range reads[i+1:] {
			if later.call == nil || later.call == first.call || later.call.sameValueAs != nil || later.shape != first.shape || later.words == first.words ||
				!sameTarget(later.call.direct, first.call.direct) || later.call.from != first.call.from {
				continue
			}
			later.call.sameValueAs = first.call
		}
	}
}

// joinOr records the spellings among the operands of one || chain.
func (b *builder) joinOr(operands []*Node) {
	if len(operands) < 2 {
		return
	}
	reads := make([]spelledRead, len(operands))
	for i, operand := range operands {
		reads[i], _ = b.spelled(operand, nil)
	}
	joinSpellings(reads)
}

// joinArms records the spellings among the arms of one if/else-if chain.
func (b *builder) joinArms(arms []*Node) {
	if len(arms) < 2 {
		return
	}
	reads := make([]spelledRead, len(arms))
	for i, arm := range arms {
		reads[i], _ = b.spelled(arm.Inner[0], arm.Inner[1])
	}
	joinSpellings(reads)
}
