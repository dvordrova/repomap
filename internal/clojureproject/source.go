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
	result := make([]programindex.PatternArgumentInput, 0, len(children))
	for i, node := range children {
		value := string(s.text[start+node.start : start+node.end])
		loc := s.location(at.Filename, start+node.start)
		arg := programindex.PatternArgumentInput{Position: i + 1, Kind: programindex.PatternDynamic,
			Origin: &sourcevalue.Value{Kind: "unknown", Text: value, Anchor: &sourcevalue.Anchor{Path: loc.Path, Line: loc.Line, Column: loc.Column}}}
		if literal, err := clojureString(value); err == nil && strings.HasPrefix(value, "\"") {
			arg.Kind = programindex.PatternLiteralString
			arg.Value = literal
			arg.Origin.Kind = "literal"
			arg.Origin.Text = literal
		}
		result = append(result, arg)
	}
	return result
}

func clojureString(value string) (string, error) {
	// Clojure strings can span physical lines; Go string escapes otherwise
	// provide the same spelling for the ordinary quoted string literals here.
	return strconv.Unquote(strings.ReplaceAll(strings.ReplaceAll(value, "\n", `\n`), "\r", `\r`))
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
