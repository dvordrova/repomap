package clojureproject

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// forms only splits source expressions inside a native, source-located call.
// Symbol resolution and declaration identity always come from clj-kondo.
// Strings, comments, character literals, collections, metadata and reader
// prefixes retain their original span; no repository code is evaluated.
type form struct {
	start, end int
	children   []form
}

func space(text []rune, at int) int {
	for at < len(text) {
		if unicode.IsSpace(text[at]) || text[at] == ',' {
			at++
			continue
		}
		if text[at] == ';' {
			for at < len(text) && text[at] != '\n' {
				at++
			}
			continue
		}
		break
	}
	return at
}
func forms(text []rune, at int, closing rune) ([]form, int) {
	var out []form
	for {
		at = space(text, at)
		if at >= len(text) {
			return out, at
		}
		if text[at] == closing && closing != 0 {
			return out, at + 1
		}
		discarded := text[at] == '#' && at+1 < len(text) && text[at+1] == '_'
		node, next := readForm(text, at)
		at = next
		if !discarded {
			out = append(out, node)
		}
	}
}
func readForm(text []rune, at int) (form, int) {
	at = space(text, at)
	start := at
	if at >= len(text) {
		return form{start: at, end: at}, at
	}
	var children []form
	switch c := text[at]; c {
	case '(', '[', '{':
		end := map[rune]rune{'(': ')', '[': ']', '{': '}'}[c]
		children, at = forms(text, at+1, end)
	case '"':
		at++
		for at < len(text) {
			if text[at] == '\\' {
				at += 2
				continue
			}
			if text[at] == '"' {
				at++
				break
			}
			at++
		}
	case '\\':
		at++
		if at < len(text) {
			at++
		}
		for at < len(text) && !unicode.IsSpace(text[at]) && !strings.ContainsRune("()[]{}\",;", text[at]) {
			at++
		}
	case '^':
		_, at = readForm(text, at+1)
		_, at = readForm(text, at)
	case '\'', '`', '@', '~':
		at++
		if c == '~' && at < len(text) && text[at] == '@' {
			at++
		}
		_, at = readForm(text, at)
	case '#':
		at++
		if at < len(text) && strings.ContainsRune("({\"", text[at]) {
			_, at = readForm(text, at)
		} else {
			if at < len(text) && strings.ContainsRune("'_=", text[at]) {
				at++
			} else {
				for at < len(text) && !unicode.IsSpace(text[at]) && !strings.ContainsRune("()[]{}\",;", text[at]) {
					at++
				}
			}
			_, at = readForm(text, at)
		}
	default:
		at++
		for at < len(text) && !unicode.IsSpace(text[at]) && !strings.ContainsRune("()[]{}\",;", text[at]) {
			at++
		}
	}
	if at > len(text) {
		at = len(text)
	}
	return form{start: start, end: at, children: children}, at
}

type source struct {
	text  []rune
	lines []int
}

func newSource(value []byte) source {
	s := source{text: []rune(string(value)), lines: []int{0}}
	for i, r := range s.text {
		if r == '\n' {
			s.lines = append(s.lines, i+1)
		}
	}
	return s
}
func (s source) offset(row, col int) int {
	if row < 1 || row > len(s.lines) || col < 1 {
		return -1
	}
	n := s.lines[row-1] + col - 1
	if n > len(s.text) {
		return -1
	}
	return n
}
func (s source) location(file string, offset int) *programindex.Location {
	row := 1
	for row < len(s.lines) && s.lines[row] <= offset {
		row++
	}
	return &programindex.Location{Path: file, Line: row, Column: offset - s.lines[row-1] + 1}
}

// callee is the source text of the form a call at site calls: `%` in
// `#(% 1)`, whose local clj-kondo reports with no name.
func (s source) callee(at site) string {
	start, end := s.offset(at.Row, at.Col), s.offset(at.EndRow, at.EndCol)
	if start < 0 || end <= start || s.text[start] != '(' {
		return ""
	}
	nodes, _ := forms(s.text[start:end], 0, 0)
	if len(nodes) != 1 || len(nodes[0].children) == 0 {
		return ""
	}
	first := nodes[0].children[0]
	return string(s.text[start+first.start : start+first.end])
}
func (s source) arguments(at site) []programindex.PatternArgumentInput {
	start, end := s.offset(at.Row, at.Col), s.offset(at.EndRow, at.EndCol)
	if start < 0 || end <= start || s.text[start] != '(' {
		return nil
	}
	nodes, _ := forms(s.text[start:end], 0, 0)
	if len(nodes) != 1 || len(nodes[0].children) == 0 {
		return nil
	}
	children := nodes[0].children[1:]
	text := func(node form) string { return string(s.text[start+node.start : start+node.end]) }
	positional, keywords := keywordArguments(children, text)
	result := make([]programindex.PatternArgumentInput, 0, len(children))
	argument := func(node form) programindex.PatternArgumentInput {
		value := text(node)
		loc := s.location(at.Filename, start+node.start)
		arg := programindex.PatternArgumentInput{Kind: programindex.PatternDynamic,
			Origin: &sourcevalue.Value{Kind: "unknown", Text: value, Anchor: &sourcevalue.Anchor{Path: loc.Path, Line: loc.Line, Column: loc.Column}}}
		if literal, err := clojureString(value); err == nil && strings.HasPrefix(value, "\"") {
			arg.Kind = programindex.PatternLiteralString
			arg.Value = literal
			arg.Origin.Kind = "literal"
			arg.Origin.Text = literal
		}
		return arg
	}
	for i, node := range positional {
		arg := argument(node)
		arg.Position = i + 1
		result = append(result, arg)
	}
	for _, pair := range keywords {
		arg := argument(pair[1])
		arg.Keyword = text(pair[0])[1:]
		result = append(result, arg)
	}
	return result
}

// keywordArguments splits a call's argument forms into its positional
// arguments and its keyword arguments, as Clojure passes them to a function
// taking `& {:keys [...]}`: the trailing keyword/value pairs
// (`(q/sketch :title "Othello" :key-pressed on-key)`) or, since Clojure
// 1.11 the same arguments, a trailing map literal (`(serve app {:port 8080})`).
// Each keyword is a plain keyword written once; a repeated or
// auto-resolved (`::k`) keyword leaves every argument positional.
func keywordArguments(children []form, text func(form) string) ([]form, [][2]form) {
	keyword := func(node form) bool {
		value := text(node)
		return len(node.children) == 0 && len(value) > 1 && value[0] == ':' && value[1] != ':' && programindex.ValidName(value[1:])
	}
	pairs := func(nodes []form) ([][2]form, bool) {
		seen := map[string]bool{}
		var result [][2]form
		for i := 0; i+1 < len(nodes); i += 2 {
			if !keyword(nodes[i]) || seen[text(nodes[i])] {
				return nil, false
			}
			seen[text(nodes[i])] = true
			result = append(result, [2]form{nodes[i], nodes[i+1]})
		}
		return result, len(result) > 0 && len(nodes)%2 == 0
	}
	// The pairs run back from the last argument while each pair starts with
	// a keyword.
	first := len(children)
	for first >= 2 && keyword(children[first-2]) {
		first -= 2
	}
	if first < len(children) {
		if found, ok := pairs(children[first:]); ok {
			return children[:first], found
		}
		return children, nil
	}
	if last := len(children) - 1; last >= 0 && strings.HasPrefix(text(children[last]), "{") {
		if found, ok := pairs(children[last].children); ok {
			return children[:last], found
		}
	}
	return children, nil
}

// loadedHeaders are the rune spans of a definition form that its namespace
// evaluates once, when it loads: the reader metadata written on the defined
// name and, with attrMaps, the attr-map before the parameters or arities and
// the one after a list of arities. A :pre/:post map follows a parameter
// vector, runs on each call and is no attr-map.
func (s source) loadedHeaders(at site, attrMaps bool) [][2]int {
	start := s.offset(at.Row, at.Col)
	if start < 0 || start >= len(s.text) || s.text[start] != '(' {
		return nil
	}
	definition, _ := readForm(s.text, start)
	if len(definition.children) < 2 {
		return nil
	}
	var spans [][2]int
	for next := definition.children[1].start; next < len(s.text) && s.text[next] == '^'; {
		var meta form
		meta, next = readForm(s.text, next+1)
		spans = append(spans, [2]int{meta.start, meta.end})
		next = space(s.text, next)
	}
	if !attrMaps {
		return spans
	}
	rest := definition.children[2:]
	if len(rest) > 0 && s.text[rest[0].start] == '"' {
		rest = rest[1:]
	}
	if len(rest) > 0 && s.text[rest[0].start] == '{' {
		spans = append(spans, [2]int{rest[0].start, rest[0].end})
		rest = rest[1:]
	}
	if last := len(rest) - 1; last > 0 && s.text[rest[0].start] == '(' && s.text[rest[last].start] == '{' {
		spans = append(spans, [2]int{rest[last].start, rest[last].end})
	}
	return spans
}

func clojureString(value string) (string, error) {
	// Clojure strings can span physical lines; Go string escapes otherwise
	// provide the same spelling for the ordinary quoted string literals here.
	return strconv.Unquote(strings.ReplaceAll(strings.ReplaceAll(value, "\n", `\n`), "\r", `\r`))
}

// codeLines are the lines of the source that hold code: a character that is
// not blank, not a comma and not in a `;` comment, outside the docstrings
// Docstrings reads. A string spanning lines holds each of them; a `\;`
// character literal and a `;` inside a string are code.
func (s source) codeLines() map[int]bool {
	documented := map[int]bool{}
	for _, span := range s.docstringSpans() {
		for at := span[0]; at < span[1]; at++ {
			documented[at] = true
		}
	}
	lines := map[int]bool{}
	line := 1
	mark := func(at int) {
		if !documented[at] {
			lines[line] = true
		}
	}
	text := s.text
	for at := 0; at < len(text); at++ {
		switch c := text[at]; {
		case c == '\n':
			line++
		case c == ';':
			for at+1 < len(text) && text[at+1] != '\n' {
				at++
			}
		case c == '"':
			mark(at)
			for at+1 < len(text) {
				at++
				if text[at] == '\n' {
					line++
				}
				mark(at)
				if text[at] == '\\' && at+1 < len(text) {
					at++
					if text[at] == '\n' {
						line++
					}
					continue
				}
				if text[at] == '"' {
					break
				}
			}
		case c == '\\':
			mark(at)
			if at+1 < len(text) && text[at+1] != '\n' {
				at++
			}
		case unicode.IsSpace(c) || c == ',':
		default:
			mark(at)
		}
	}
	return lines
}

// docstringSpans are the rune spans of the docstrings of the top-level
// definition forms Docstrings reads.
func (s source) docstringSpans() [][2]int {
	nodes, _ := forms(s.text, 0, 0)
	var spans [][2]int
	for _, node := range nodes {
		if len(node.children) < 3 || s.text[node.start] != '(' {
			continue
		}
		head := node.children[0]
		switch string(s.text[head.start:head.end]) {
		case "ns", "defn", "defn-", "defmacro", "defmulti", "defprotocol", "clojure.core/defn":
		default:
			continue
		}
		if quote := node.children[2]; s.text[quote.start] == '"' {
			spans = append(spans, [2]int{quote.start, quote.end})
		}
	}
	return spans
}

// countLines counts the lines from first to last that hold code.
func countLines(lines map[int]bool, first, last int) int {
	count := 0
	for line := first; line <= last; line++ {
		if lines[line] {
			count++
		}
	}
	return count
}

// Documentation is a literal author quote attached to its declaration line.
type Documentation struct {
	Line int
	Text string
}

func Docstrings(data []byte) []Documentation {
	source := newSource(data)
	nodes, _ := forms(source.text, 0, 0)
	var docs []Documentation
	for _, node := range nodes {
		if len(node.children) < 3 || source.text[node.start] != '(' {
			continue
		}
		head := node.children[0]
		name := string(source.text[head.start:head.end])
		switch name {
		case "ns", "defn", "defn-", "defmacro", "defmulti", "defprotocol", "clojure.core/defn":
		default:
			continue
		}
		quote := node.children[2]
		text, err := clojureString(string(source.text[quote.start:quote.end]))
		if err != nil {
			continue
		}
		docs = append(docs, Documentation{Line: source.location("source.clj", quote.start).Line, Text: text})
	}
	return docs
}

// caseComparison is a `(case value "a" … "b" … default)` form at site as a
// comparison of value with its words (PROGRAM_INDEX Comparison): a test
// that is a string literal, or a list of them (`("a" "b")`), and its result
// form are one case, whose branch runs from the test to the result's last
// line. Any other test (a number, a keyword, a symbol) is no word, and a form
// comparing fewer than two words in two cases is none. The value's origin is
// its text: the adapter follows no Clojure value.
func (s source) caseComparison(at site) *programindex.Comparison {
	start, end := s.offset(at.Row, at.Col), s.offset(at.EndRow, at.EndCol)
	if start < 0 || end <= start || s.text[start] != '(' {
		return nil
	}
	nodes, _ := forms(s.text[start:end], 0, 0)
	if len(nodes) != 1 || len(nodes[0].children) < 4 {
		return nil
	}
	children := nodes[0].children
	written := func(node form) string {
		return strings.Join(strings.Fields(string(s.text[start+node.start:start+node.end])), " ")
	}
	word := func(node form) (string, bool) {
		value := string(s.text[start+node.start : start+node.end])
		if !strings.HasPrefix(value, "\"") {
			return "", false
		}
		literal, err := clojureString(value)
		return literal, err == nil
	}
	compared := children[1]
	anchor := s.location(at.Filename, start+compared.start)
	comparison := &programindex.Comparison{Value: written(compared), Origin: &sourcevalue.Value{Kind: "unknown", Text: written(compared),
		Anchor: &sourcevalue.Anchor{Path: anchor.Path, Line: anchor.Line, Column: anchor.Column}}}
	distinct := map[string]bool{}
	for i := 2; i+1 < len(children); i += 2 {
		test, result := children[i], children[i+1]
		candidates := []form{test}
		if s.text[start+test.start] == '(' {
			candidates = test.children
		}
		item := programindex.ComparisonCase{Form: programindex.ComparisonCaseForm}
		for _, candidate := range candidates {
			value, ok := word(candidate)
			if !ok {
				item.Words = nil
				break
			}
			if item.Location == nil {
				item.Location = s.location(at.Filename, start+candidate.start)
			}
			item.Words = append(item.Words, value)
		}
		if len(item.Words) == 0 {
			continue
		}
		first, last := s.location(at.Filename, start+test.start), s.location(at.Filename, start+result.end-1)
		item.Branch = &programindex.LineRange{Line: first.Line, EndLine: last.Line}
		for _, value := range item.Words {
			if value != "" {
				distinct[value] = true
			}
		}
		comparison.Cases = append(comparison.Cases, item)
	}
	if len(comparison.Cases) < 2 || len(distinct) < 2 {
		return nil
	}
	comparison.Location = comparison.Cases[0].Location
	return comparison
}

// joinedCalls are, for an `(or …)` form at site, the calls its operands
// read the same value with (PROGRAM_INDEX SameValueAs): operands written
// the same but for the string literals their one call is given, each call
// of the same form, `(or (get params "storageClass") (get params
// "storage-class"))`. Each later call's opening parenthesis maps to the
// first's. `and` is none: both are needed.
func (s source) joinedCalls(at site) map[[2]int][2]int {
	start, end := s.offset(at.Row, at.Col), s.offset(at.EndRow, at.EndCol)
	if start < 0 || end <= start || end > len(s.text) || s.text[start] != '(' {
		return nil
	}
	nodes, _ := forms(s.text[start:end], 0, 0)
	if len(nodes) != 1 || len(nodes[0].children) < 3 {
		return nil
	}
	type read struct {
		call         [2]int
		shape, words string
	}
	spelled := func(operand form) (read, bool) {
		var calls []form
		closure := false
		var visit func(form)
		visit = func(node form) {
			text := s.text[start+node.start:]
			if len(text) > 3 && string(text[:3]) == "(fn" && (unicode.IsSpace(text[3]) || text[3] == '[') || text[0] == '#' && len(text) > 1 && text[1] == '(' {
				closure = true
				return
			}
			if text[0] == '(' {
				for _, child := range node.children {
					if s.text[start+child.start] == '"' {
						calls = append(calls, node)
						break
					}
				}
			}
			for _, child := range node.children {
				visit(child)
			}
		}
		visit(operand)
		if closure || len(calls) != 1 || len(calls[0].children) == 0 {
			return read{}, false
		}
		call := calls[0]
		written := string(s.text[start+operand.start : start+operand.end])
		base := operand.start
		var words []string
		for i := len(call.children) - 1; i >= 0; i-- {
			child := call.children[i]
			if s.text[start+child.start] != '"' {
				continue
			}
			from, to := child.start-base, child.end-base
			words = append([]string{written[from:to]}, words...)
			written = written[:from] + "\x00" + written[to:]
		}
		at := s.location("", start+call.start)
		return read{call: [2]int{at.Line, at.Column}, shape: strings.Join(strings.Fields(written), " "), words: strings.Join(words, "\x00")}, true
	}
	// Offsets index runes; a byte slice of the written text holds them only
	// for ASCII text, so a form holding any other character joins nothing.
	for _, r := range s.text[start:end] {
		if r > unicode.MaxASCII {
			return nil
		}
	}
	reads := make([]*read, 0, len(nodes[0].children)-1)
	for _, operand := range nodes[0].children[1:] {
		if value, ok := spelled(operand); ok {
			reads = append(reads, &value)
		} else {
			reads = append(reads, nil)
		}
	}
	joined := map[[2]int][2]int{}
	for i, first := range reads {
		if first == nil {
			continue
		}
		if _, later := joined[first.call]; later {
			continue
		}
		for _, later := range reads[i+1:] {
			if later == nil || later.shape != first.shape || later.words == first.words || later.call == first.call {
				continue
			}
			if _, seen := joined[later.call]; !seen {
				joined[later.call] = first.call
			}
		}
	}
	return joined
}
