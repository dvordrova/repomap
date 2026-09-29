package cproject

import (
	"strconv"
	"unicode"
	"unicode/utf8"

	p "github.com/dvordrova/repomap/internal/programindex"
)

// switchComparison records a switch whose cases compare the value with
// character literals as the value's comparison (PROGRAM_INDEX Comparison):
// `switch (c) { case 'h': … case 'v': … }`. Each case label written in the
// switch's own block is a case, labels stacked on one statement are one
// case with several words, and a case's branch runs from its label to the
// line before the next label, or to the block's end. Other labels (a number,
// an enum, a range) and a character the index cannot hold ('\0') are no
// words; C compares strings by calls (strcmp), which are call facts.
func (b *builder) switchComparison(w walker, n *Node) {
	if w.function == nil || len(n.Inner) < 2 {
		return
	}
	body, condition := n.Inner[len(n.Inner)-1], n.Inner[len(n.Inner)-2]
	fn := b.objects[w.owner]
	if body.Kind != "CompoundStmt" || fn == nil || !fn.Kind.Callable() {
		return
	}
	at := location(condition.Begin.Site())
	value := b.written(condition)
	if at == nil || value == "" {
		return
	}
	comparison := p.Comparison{Value: value, Origin: b.origin(w, condition), Location: nil}
	labelLine := func(label *Node) int { return label.Begin.Site().Line }
	var labels []*Node
	for _, child := range body.Inner {
		if child.Kind == "CaseStmt" || child.Kind == "DefaultStmt" {
			labels = append(labels, child)
		}
	}
	for position, label := range labels {
		if label.Kind != "CaseStmt" {
			continue
		}
		item := p.ComparisonCase{Form: p.ComparisonCaseForm}
		for current := label; current != nil && current.Kind == "CaseStmt"; {
			if len(current.Inner) == 2 {
				if literal := unwrapValue(current.Inner[0]); literal != nil && literal.Kind == "CharacterLiteral" {
					if code, err := strconv.Atoi(literal.Value); err == nil && code > 0 && code <= unicode.MaxRune && utf8.ValidRune(rune(code)) {
						if where := location(literal.Begin.Site()); where != nil {
							if item.Location == nil {
								item.Location = where
							}
							item.Words = append(item.Words, string(rune(code)))
						}
					}
				}
			}
			if len(current.Inner) == 0 {
				break
			}
			current = current.Inner[len(current.Inner)-1]
		}
		if len(item.Words) == 0 {
			continue
		}
		first, last := labelLine(label), body.End.Site().Line
		if position+1 < len(labels) {
			last = labelLine(labels[position+1]) - 1
		}
		if first > 0 && last >= first {
			item.Branch = &p.LineRange{Line: first, EndLine: last}
		}
		comparison.Cases = append(comparison.Cases, item)
	}
	if len(comparison.Cases) == 0 {
		return
	}
	comparison.Location = comparison.Cases[0].Location
	distinct := map[string]bool{}
	for _, item := range comparison.Cases {
		for _, word := range item.Words {
			distinct[word] = true
		}
	}
	if len(distinct) < 2 || len(comparison.Cases) < 2 {
		return
	}
	// A function a header defines is walked once per unit including it.
	for _, recorded := range fn.Comparisons {
		if *recorded.Location == *comparison.Location {
			return
		}
	}
	fn.Comparisons = append(fn.Comparisons, comparison)
}
