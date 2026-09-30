package lines

import (
	"bytes"
	"path"
	"strings"
)

// CallText is the call at a source position as the code wrote it: its
// receiver chain, its name and its arguments through the closing
// parenthesis, with comments dropped and whitespace folded to single
// spaces. A position inside a table row or a Lisp form that is no call
// gives that row or form; one inside an assignment gives its statement.
// Nothing is cut short: the whole call is the evidence, and a question too
// large for its request is the request's to refuse. A file of a language
// with no lexer here gives "" and the question goes without it.
//
// line is 1-based; column is the 1-based byte of the position on its line,
// and 0 when the adapter recorded none (the line's first token).
func CallText(src []byte, filePath string, line, column int) string {
	return NewCallFile(src, filePath).Text(line, column)
}

// CallFile is one file lexed once, for the calls at many of its
// positions: lexing a large file for each call took most of a reading.
type CallFile struct {
	reader *callReader // nil for a language with no lexer here
}

// NewCallFile lexes one file's source for CallText.
func NewCallFile(src []byte, filePath string) *CallFile {
	family, known := callFamilyOf(filePath)
	if !known {
		return &CallFile{}
	}
	c := &callReader{src: src, family: family, breaks: newlineEndsStatement(filePath), goBlocks: strings.EqualFold(path.Ext(filePath), ".go")}
	c.tokens = lexCall(src, family)
	c.matches, c.parents = matchGroups(c.tokens)
	return &CallFile{reader: c}
}

// Text is CallText at a position of the file.
func (f *CallFile) Text(line, column int) string {
	c := f.reader
	if c == nil || line < 1 {
		return ""
	}
	offset, ok := lineOffset(c.src, line, column)
	if !ok {
		return ""
	}
	at := tokenAt(c.tokens, offset)
	if at < 0 {
		return ""
	}
	first, last := c.span(at)
	if first < 0 || last < first {
		return ""
	}
	return renderTokens(c.src, c.tokens, first, last)
}

// RowText is a table's row as the code wrote it, from the position of its
// first word: the widest element around that word (the tokens between two
// commas of one group, or between a comma and the group's edge) holding
// none of the other positions given, the other rows' words as line and
// column. A dict's row is its `"verbosity": Arg(...)`, where CallText at
// its key gives the whole dict; C's `{"get", getCommand, 2}` is the row's
// own braces, as CallText gives it. With no other position it is CallText.
func (f *CallFile) RowText(line, column int, others [][2]int) string {
	c := f.reader
	if c == nil || line < 1 || c.family == lispFamily || len(others) == 0 {
		return f.Text(line, column)
	}
	offset, ok := lineOffset(c.src, line, column)
	if !ok {
		return ""
	}
	at := tokenAt(c.tokens, offset)
	if at < 0 {
		return ""
	}
	outside := map[int]bool{}
	for _, other := range others {
		if offset, ok := lineOffset(c.src, other[0], other[1]); ok {
			if token := tokenAt(c.tokens, offset); token >= 0 {
				outside[token] = true
			}
		}
	}
	first, last := at, at
	for node := at; ; {
		parent := c.parents[node]
		if parent < 0 || c.matches[parent] < 0 || c.codeBlock(parent) {
			break
		}
		low, high := c.element(parent, node)
		held := false
		for token := range outside {
			if token >= low && token <= high {
				held = true
				break
			}
		}
		if held {
			break
		}
		first, last, node = low, high, parent
	}
	return renderTokens(c.src, c.tokens, first, last)
}

// element is the first and last token of the element of a group holding a
// token: between the commas at the group's own depth, line breaks at its
// edges left out.
func (c *callReader) element(open, at int) (int, int) {
	tokens := c.tokens
	low, high := open+1, c.matches[open]-1
	for i := open + 1; i < c.matches[open]; i++ {
		if tokens[i].kind == 'o' && c.matches[i] > i {
			i = c.matches[i]
			continue
		}
		if tokens[i].kind == 'p' && c.text(i) == "," {
			if i < at {
				low = i + 1
			} else {
				high = i - 1
				break
			}
		}
	}
	for low < high && tokens[low].kind == 'n' {
		low++
	}
	for high > low && tokens[high].kind == 'n' {
		high--
	}
	return low, high
}

// callReader holds one file's tokens while CallText finds a call in them.
type callReader struct {
	src              []byte
	family           callFamily
	tokens           []callToken
	matches, parents []int
	// breaks is whether a line break ends a statement; goBlocks whether a
	// brace group's lines ending without a comma make it a block (gofmt
	// ends every line of a composite literal with one).
	breaks, goBlocks bool
}

func (c *callReader) text(i int) string { return c.tokens[i].text(c.src) }

type callFamily int

const (
	cLikeFamily callFamily = iota
	pythonFamily
	lispFamily
)

// callFamilyOf names the lexer of a file by its extension. C statements end
// only at a semicolon; Go, JavaScript and TypeScript also end one at a line
// break outside every group.
func callFamilyOf(filePath string) (callFamily, bool) {
	switch strings.ToLower(path.Ext(filePath)) {
	case ".c", ".h", ".go", ".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs":
		return cLikeFamily, true
	case ".py", ".pyi":
		return pythonFamily, true
	case ".clj", ".cljs", ".cljc", ".edn":
		return lispFamily, true
	default:
		return 0, false
	}
}

func newlineEndsStatement(filePath string) bool {
	switch strings.ToLower(path.Ext(filePath)) {
	case ".c", ".h":
		return false
	default:
		return true
	}
}

// lineOffset is the byte offset of a 1-based line and column; column 0 is
// the line's first non-blank byte.
func lineOffset(src []byte, line, column int) (int, bool) {
	start := 0
	for current := 1; current < line; current++ {
		next := bytes.IndexByte(src[start:], '\n')
		if next < 0 {
			return 0, false
		}
		start += next + 1
	}
	end := len(src)
	if next := bytes.IndexByte(src[start:], '\n'); next >= 0 {
		end = start + next
	}
	if column < 1 {
		offset := start
		for offset < end && (src[offset] == ' ' || src[offset] == '\t' || src[offset] == '\r') {
			offset++
		}
		return offset, true
	}
	return min(start+column-1, end), true
}

type callToken struct {
	kind       byte // 'i' word, 's' string, 'o' opener, 'c' closer, 'n' line break, 'p' punctuation
	start, end int
	// gap reports whitespace or a comment before the token.
	gap, gapNewline bool
}

func (t callToken) text(src []byte) string { return string(src[t.start:t.end]) }

// lexCall splits a file into the tokens CallText needs: words, strings,
// groups, line breaks and punctuation. Comments are dropped and only
// remembered as a gap before the next token.
func lexCall(src []byte, family callFamily) []callToken {
	var tokens []callToken
	gap, gapNewline := false, false
	emit := func(kind byte, start, end int) {
		tokens = append(tokens, callToken{kind: kind, start: start, end: end, gap: gap, gapNewline: gapNewline})
		gap, gapNewline = false, false
	}
	for i := 0; i < len(src); {
		c := src[i]
		switch {
		case c == '\n':
			emit('n', i, i+1)
			gap, gapNewline = true, true
			i++
		case c == ' ' || c == '\t' || c == '\r' || c == '\f' || c == '\v' || family == lispFamily && c == ',':
			gap = true
			i++
		case family == cLikeFamily && c == '/' && i+1 < len(src) && src[i+1] == '/',
			family == pythonFamily && c == '#',
			family == lispFamily && c == ';':
			for i < len(src) && src[i] != '\n' {
				i++
			}
			gap = true
		case family == cLikeFamily && c == '/' && i+1 < len(src) && src[i+1] == '*':
			end := bytes.Index(src[i+2:], []byte("*/"))
			if end < 0 {
				end = len(src)
			} else {
				end += i + 4
			}
			if bytes.IndexByte(src[i:end], '\n') >= 0 {
				gapNewline = true
			}
			gap = true
			i = end
		case c == '"' || family != lispFamily && c == '\'' || family == cLikeFamily && c == '`':
			end := stringEnd(src, i, family)
			emit('s', i, end)
			i = end
		case family == lispFamily && c == '\\':
			// A character literal: \( is a character, never a group.
			end := i + 2
			for end < len(src) && wordByte(src[end], family) {
				end++
			}
			emit('i', i, min(end, len(src)))
			i = min(end, len(src))
		case c == '(' || c == '[' || c == '{':
			emit('o', i, i+1)
			i++
		case c == ')' || c == ']' || c == '}':
			emit('c', i, i+1)
			i++
		case wordByte(c, family):
			end := i + 1
			for end < len(src) && wordByte(src[end], family) {
				end++
			}
			emit('i', i, end)
			i = end
		default:
			end := i + 1
			if i+1 < len(src) {
				switch string(src[i : i+2]) {
				case "->", "::", "?.", "=>":
					end = i + 2
				}
			}
			emit('p', i, end)
			i = end
		}
	}
	return tokens
}

// wordByte is a byte of a name or a number. A Lisp symbol takes almost
// every byte: str/replace, -main, ->, update-in!.
func wordByte(c byte, family callFamily) bool {
	if c >= 0x80 || c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
		return true
	}
	switch family {
	case cLikeFamily:
		return c == '$'
	case lispFamily:
		return !strings.ContainsRune(" \t\r\n\f\v,()[]{}\";`~^@", rune(c)) && c != '\\'
	}
	return false
}

// stringEnd is the byte after a string that opens at start: a backslash
// escapes the next byte, except in a Go raw string; Python's triple quotes
// run to their closing triple.
func stringEnd(src []byte, start int, family callFamily) int {
	quote := src[start]
	if family == pythonFamily && start+2 < len(src) && src[start+1] == quote && src[start+2] == quote {
		closing := strings.Repeat(string(quote), 3)
		for i := start + 3; i < len(src); i++ {
			if src[i] == '\\' {
				i++
				continue
			}
			if bytes.HasPrefix(src[i:], []byte(closing)) {
				return i + 3
			}
		}
		return len(src)
	}
	for i := start + 1; i < len(src); i++ {
		switch {
		case src[i] == '\\' && quote != '`':
			i++
		case src[i] == quote:
			return i + 1
		case src[i] == '\n' && quote != '`' && family != lispFamily:
			// An unterminated one-line string ends at its line.
			return i
		}
	}
	return len(src)
}

// matchGroups pairs every opener with its closer and gives each token its
// innermost enclosing opener (-1 at the top level).
func matchGroups(tokens []callToken) (matches, parents []int) {
	matches = make([]int, len(tokens))
	parents = make([]int, len(tokens))
	var stack []int
	for i, token := range tokens {
		matches[i] = -1
		parents[i] = -1
		if len(stack) > 0 {
			parents[i] = stack[len(stack)-1]
		}
		switch token.kind {
		case 'o':
			stack = append(stack, i)
		case 'c':
			if len(stack) > 0 {
				open := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				matches[open], matches[i] = i, open
				parents[i] = parents[open]
			}
		}
	}
	return matches, parents
}

// tokenAt is the token holding a byte offset, else the first one after it
// on the same line.
func tokenAt(tokens []callToken, offset int) int {
	for i, token := range tokens {
		if token.kind == 'n' && token.start >= offset {
			return -1
		}
		if token.kind != 'n' && token.end > offset {
			return i
		}
	}
	return -1
}

func (c *callReader) chainPunct(i int) bool {
	if c.tokens[i].kind != 'p' {
		return false
	}
	switch c.text(i) {
	case ".", "->", "::", "?.":
		return true
	}
	return false
}

// span is the first and last token of the call at a token.
func (c *callReader) span(at int) (int, int) {
	tokens, matches, parents := c.tokens, c.matches, c.parents
	if c.family == lispFamily {
		if tokens[at].kind == 'o' && matches[at] >= 0 {
			return at, matches[at]
		}
		if parent := parents[at]; parent >= 0 && matches[parent] >= 0 {
			return parent, matches[parent]
		}
		return at, at
	}
	// The call's own name: the anchor, or the name before the parenthesis
	// the anchor stands on.
	name, open := at, -1
	if tokens[at].kind == 'o' && c.text(at) == "(" {
		open, name = at, at-1
		for name >= 0 && tokens[name].kind == 'n' {
			name--
		}
		if name < 0 || tokens[name].kind != 'i' && tokens[name].kind != 'c' {
			return at, matches[at]
		}
	}
	first := c.chainStart(name)
	if open < 0 && tokens[name].kind == 'i' {
		// Forward over the rest of a selector chain to its parenthesis.
		next := name + 1
		for next+1 < len(tokens) && c.chainPunct(next) && tokens[next+1].kind == 'i' {
			next += 2
		}
		next = c.skipTypeArguments(next)
		if next < len(tokens) && tokens[next].kind == 'o' && c.text(next) == "(" {
			open = next
		}
	}
	if open >= 0 && matches[open] >= 0 {
		return first, matches[open]
	}
	// No call here: a row of a table, or a statement such as an assignment.
	if parent := parents[at]; parent >= 0 && matches[parent] >= 0 && !c.codeBlock(parent) {
		return parent, matches[parent]
	}
	return c.statement(at)
}

// skipTypeArguments steps over the type arguments of a generic call,
// f<T>(x) or f[T](x), to its parenthesis; anything else stays where it is.
func (c *callReader) skipTypeArguments(at int) int {
	tokens := c.tokens
	if at >= len(tokens) {
		return at
	}
	if tokens[at].kind == 'o' && c.text(at) == "[" && c.matches[at] > at && c.matches[at]+1 < len(tokens) && c.text(c.matches[at]+1) == "(" {
		return c.matches[at] + 1
	}
	if tokens[at].kind != 'p' || c.text(at) != "<" {
		return at
	}
	depth := 0
	for i := at; i < len(tokens); i++ {
		switch {
		case tokens[i].kind == 'o' && c.text(i) == "(", tokens[i].kind == 'n', tokens[i].kind == 'p' && c.text(i) == ";":
			return at
		case tokens[i].kind == 'o' && c.matches[i] > i:
			i = c.matches[i]
		case tokens[i].kind == 'p' && c.text(i) == "<":
			depth++
		case tokens[i].kind == 'p' && c.text(i) == ">":
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return at
}

// chainStart walks back from a name over the selector chain it ends:
// a.b().c, p->q, pkg::f, a[0].f.
func (c *callReader) chainStart(at int) int {
	tokens, matches := c.tokens, c.matches
	// back is where a group ending at a closer starts, with the groups and
	// the name written against it: f(x)[0] from its ], a(b)(c) from its ).
	back := func(closer int) int {
		open := matches[closer]
		for open > 0 && !tokens[open].gap {
			switch before := open - 1; {
			case tokens[before].kind == 'i':
				return before
			case tokens[before].kind == 'c' && matches[before] >= 0:
				open = matches[before]
			default:
				return open
			}
		}
		return open
	}
	// previous is the token before i, across line breaks: a chain may go on
	// on the next line (.action(run)).
	previous := func(i int) int {
		i--
		for i >= 0 && tokens[i].kind == 'n' {
			i--
		}
		return i
	}
	first := at
	if tokens[first].kind == 'c' {
		if matches[first] < 0 {
			return first
		}
		first = back(first)
	}
	for {
		dot := previous(first)
		if dot >= 0 && c.family == cLikeFamily && tokens[dot].kind == 'i' && c.text(dot) == "new" {
			return dot
		}
		if dot < 0 || !c.chainPunct(dot) {
			return first
		}
		before := previous(dot)
		switch {
		case before < 0:
			return first
		case tokens[before].kind == 'i':
			first = before
		case tokens[before].kind == 'c' && matches[before] >= 0:
			first = back(before)
		default:
			return first
		}
	}
}

// codeBlock reports a brace group of statements rather than a row of
// values: one after a closing parenthesis, an arrow or a block keyword, one
// holding a semicolon, or in Go one whose lines do not end in commas.
func (c *callReader) codeBlock(open int) bool {
	tokens, matches := c.tokens, c.matches
	if c.family == pythonFamily || c.text(open) != "{" {
		return false
	}
	previous := open - 1
	for previous >= 0 && tokens[previous].kind == 'n' {
		previous--
	}
	if previous >= 0 {
		switch text := c.text(previous); text {
		case ")", "=>", "else", "do", "try", "finally":
			return true
		}
	}
	lastSignificant := open
	for i := open + 1; i < matches[open]; i++ {
		switch tokens[i].kind {
		case 'o':
			if matches[i] < 0 {
				return false
			}
			i = matches[i]
			lastSignificant = i
			continue
		case 'n':
			if c.goBlocks && lastSignificant != open && c.text(lastSignificant) != "," {
				return true
			}
			continue
		case 'p':
			if c.text(i) == ";" {
				return true
			}
		}
		lastSignificant = i
	}
	return false
}

// statement is the statement holding a token, between the separators at
// its depth: a semicolon, a line break where one ends a statement, a block
// before it, or the enclosing group.
func (c *callReader) statement(at int) (int, int) {
	tokens, matches := c.tokens, c.matches
	ends := func(i int) bool {
		switch tokens[i].kind {
		case 'n':
			return c.breaks
		case 'p':
			return c.text(i) == ";"
		}
		return false
	}
	parent := c.parents[at]
	low, high := 0, len(tokens)-1
	if parent >= 0 {
		low = parent + 1
		if matches[parent] >= 0 {
			high = matches[parent] - 1
		}
	}
	first := at
	for i := at - 1; i >= low; i-- {
		if tokens[i].kind == 'c' {
			if c.text(i) == "}" || matches[i] < 0 {
				break
			}
			i = matches[i]
			first = i
			continue
		}
		if ends(i) {
			break
		}
		first = i
	}
	last := at
	for i := at + 1; i <= high; i++ {
		if tokens[i].kind == 'o' && matches[i] >= 0 {
			i = matches[i]
			last = i
			continue
		}
		if ends(i) {
			break
		}
		last = i
	}
	for first < last && tokens[first].kind == 'n' {
		first++
	}
	return first, last
}

// renderTokens writes the tokens from first to last as written, one space
// where the source had whitespace or a comment, none after an opener or
// before a closer when that whitespace broke a line.
func renderTokens(src []byte, tokens []callToken, first, last int) string {
	var text strings.Builder
	previous := byte(0)
	for i := first; i <= last; i++ {
		token := tokens[i]
		if token.kind == 'n' {
			continue
		}
		if text.Len() > 0 && token.gap {
			if !(token.gapNewline && (previous == 'o' || token.kind == 'c')) {
				text.WriteByte(' ')
			}
		}
		text.Write(src[token.start:token.end])
		previous = token.kind
	}
	return text.String()
}
