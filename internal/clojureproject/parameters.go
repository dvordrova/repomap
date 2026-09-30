package clojureproject

import (
	"strings"

	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// localBinding is the binding a local's use names: where its symbol is
// bound, in clj-kondo's locals, linked by the binding's id.
type localBinding struct {
	filename, name string
	row, col       int
}

// localBindings are, by the site a local is used at, where it is bound.
// clj-kondo's binding ids are process-local and canonicalAnalysis drops
// them, so the links are read first; a split analysis numbers each half on
// its own, and a file is in one half, so an id is read with its file.
func localBindings(a analysis) map[site]localBinding {
	type key struct {
		filename, lang string
		id             int
	}
	bound := make(map[key]local, len(a.Locals))
	for _, binding := range a.Locals {
		bound[key{binding.Filename, binding.Lang, binding.ID}] = binding
	}
	result := make(map[site]localBinding, len(a.LocalUsages))
	for _, use := range a.LocalUsages {
		binding, ok := bound[key{use.Filename, use.Lang, use.ID}]
		if !ok {
			continue
		}
		result[site{Filename: use.Filename, Row: use.Row, Col: use.Col}] = localBinding{filename: binding.Filename, name: binding.Name, row: binding.Row, col: binding.Col}
	}
	return result
}

// parameters are, by where each is written, the plain symbols of a
// definition's parameter vectors with their positions from one: the vector
// of `(defn f [a b] …)` and each arity's of `(defn f ([a] …) ([a b] …))`.
// A destructuring form holds no plain parameter, and a symbol after & is
// no position a call counts.
func (s source) parameters(file string, at site) map[[2]int]int {
	start, end := s.offset(at.Row, at.Col), s.offset(at.EndRow, at.EndCol)
	if start < 0 || end <= start || s.text[start] != '(' {
		return nil
	}
	nodes, _ := forms(s.text[start:end], 0, 0)
	if len(nodes) != 1 || len(nodes[0].children) < 3 {
		return nil
	}
	text := func(node form) string { return string(s.text[start+node.start : start+node.end]) }
	result := map[[2]int]int{}
	vector := func(node form) {
		for i, parameter := range node.children {
			name := text(parameter)
			if name == "&" {
				return
			}
			if len(parameter.children) > 0 || strings.ContainsAny(name, "{[(^#") {
				continue
			}
			at := s.location(file, start+parameter.start)
			result[[2]int{at.Line, at.Column}] = i + 1
		}
	}
	for _, child := range nodes[0].children[2:] {
		switch {
		case strings.HasPrefix(text(child), "["):
			vector(child)
			return result
		case strings.HasPrefix(text(child), "(") && len(child.children) > 0 && strings.HasPrefix(text(child.children[0]), "["):
			vector(child.children[0])
		}
	}
	return result
}

// parameterOrigin is what an argument written as a function's own
// parameter, or as a keyword looked up in one (`(:key event)`), carries:
// that parameter, or that field of it. Other arguments keep their origin.
func parameterOrigin(origin *sourcevalue.Value, parameterAt func(row, col int) (string, int, *sourcevalue.Anchor, bool)) *sourcevalue.Value {
	if origin == nil || origin.Anchor == nil || origin.Kind != "unknown" {
		return origin
	}
	value := origin.Text
	parameter := func(name string, column int) (*sourcevalue.Value, bool) {
		bound, position, owner, ok := parameterAt(origin.Anchor.Line, column)
		if !ok || bound != name {
			return nil, false
		}
		return &sourcevalue.Value{Kind: "parameter", Text: name, Position: position, Owner: owner,
			Anchor: &sourcevalue.Anchor{Path: origin.Anchor.Path, Line: origin.Anchor.Line, Column: column}}, true
	}
	if symbol(value) {
		if found, ok := parameter(value, origin.Anchor.Column); ok {
			return found
		}
		return origin
	}
	// (:key event): the keyword and the symbol, on one line.
	inner, ok := strings.CutPrefix(value, "(:")
	if !ok || !strings.HasSuffix(inner, ")") {
		return origin
	}
	fields := strings.Fields(strings.TrimSuffix(inner, ")"))
	if len(fields) != 2 || !symbol(fields[1]) || !symbol(fields[0]) {
		return origin
	}
	column := origin.Anchor.Column + strings.LastIndex(value, fields[1])
	found, ok := parameter(fields[1], column)
	if !ok {
		return origin
	}
	return &sourcevalue.Value{Kind: "field", Text: fields[0], Parts: []sourcevalue.Value{*found}, Anchor: origin.Anchor}
}

// symbol reports a plain Clojure symbol: no namespace, keyword, literal
// or form.
func symbol(value string) bool {
	if value == "" || strings.ContainsAny(value, " \t\n()[]{}\"';@^`~#:/\\") {
		return false
	}
	first := value[0]
	return !(first >= '0' && first <= '9') && value != "nil" && value != "true" && value != "false"
}

// mapRows are the rows of a table a def names: `(def key->command {:n
// :new-game :u :undo})`, a map literal of two or more entries whose keys
// and values are keywords or strings, each entry one row of its key's word
// and its value's, as written (a keyword without its colon). A map holding
// a form, a number or a symbol is no table of words.
func (s source) mapRows(file string, at site) []programindex.TableRow {
	start, end := s.offset(at.Row, at.Col), s.offset(at.EndRow, at.EndCol)
	if start < 0 || end <= start || s.text[start] != '(' {
		return nil
	}
	nodes, _ := forms(s.text[start:end], 0, 0)
	if len(nodes) != 1 || len(nodes[0].children) < 3 {
		return nil
	}
	text := func(node form) string { return string(s.text[start+node.start : start+node.end]) }
	value := nodes[0].children[len(nodes[0].children)-1]
	if !strings.HasPrefix(text(value), "{") || len(value.children) < 4 || len(value.children)%2 != 0 {
		return nil
	}
	word := func(node form) (programindex.RowLiteral, bool) {
		written := text(node)
		literal := programindex.RowLiteral{Location: s.location(file, start+node.start)}
		switch {
		case len(node.children) > 0:
			return literal, false
		case strings.HasPrefix(written, ":") && !strings.HasPrefix(written, "::") && len(written) > 1:
			literal.Value = written[1:]
		case strings.HasPrefix(written, "\""):
			decoded, err := clojureString(written)
			if err != nil {
				return literal, false
			}
			literal.Value = decoded
		default:
			return literal, false
		}
		return literal, true
	}
	var rows []programindex.TableRow
	for i := 0; i+1 < len(value.children); i += 2 {
		key, ok := word(value.children[i])
		if !ok {
			return nil
		}
		mapped, ok := word(value.children[i+1])
		if !ok {
			return nil
		}
		rows = append(rows, programindex.TableRow{Literals: []programindex.RowLiteral{key, mapped}})
	}
	return rows
}
