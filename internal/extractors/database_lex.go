package extractors

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

type sourceLiteral struct {
	text             string
	line, start, end int
	dynamic          bool
	expression       string
	segments         []sqlSourceSegment
}

type sqlSourceSegment struct {
	text         string
	offset, line int
	multiline    bool
}

// sourceLiterals is a lexical pass, not a second language AST. The mask keeps
// newlines and code positions while removing quoted text and comments, so a
// class/field declaration inside a docstring cannot become schema evidence.
func sourceLiterals(source string, python bool) ([]sourceLiteral, string) {
	mask := []byte(source)
	var out []sourceLiteral
	line := 1
	blank := func(a, b int) {
		for i := a; i < b; i++ {
			if mask[i] != '\n' && mask[i] != '\r' {
				mask[i] = ' '
			}
		}
	}
	for i := 0; i < len(source); {
		if source[i] == '\n' {
			line++
			i++
			continue
		}
		if python && source[i] == '#' || !python && strings.HasPrefix(source[i:], "//") {
			j := i
			for j < len(source) && source[j] != '\n' {
				j++
			}
			blank(i, j)
			i = j
			continue
		}
		if !python && strings.HasPrefix(source[i:], "/*") {
			j := i + 2
			for j < len(source) && !strings.HasPrefix(source[j:], "*/") {
				j++
			}
			j = min(len(source), j+2)
			line += strings.Count(source[i:j], "\n")
			blank(i, j)
			i = j
			continue
		}
		q := source[i]
		if q != '\'' && q != '"' && q != '`' {
			i++
			continue
		}
		start, startLine := i, line
		delimiter := string(q)
		if python && i+2 < len(source) && source[i+1] == q && source[i+2] == q {
			delimiter = strings.Repeat(string(q), 3)
		}
		begin := i + len(delimiter)
		j := begin
		for j < len(source) {
			if strings.HasPrefix(source[j:], delimiter) {
				break
			}
			if source[j] == '\\' && q != '`' {
				j += 2
			} else {
				j++
			}
		}
		if j >= len(source) {
			out = append(out, sourceLiteral{text: source[begin:], line: startLine, start: start, end: len(source), dynamic: true, expression: source[literalSourceStart(source, start, python):]})
			blank(start, len(source))
			break
		}
		end := j + len(delimiter)
		value := source[begin:j]
		dynamic := !python && q == '`' && strings.Contains(value, "${")
		if python {
			dynamic = strings.ContainsAny(source[literalSourceStart(source, start, true):start], "fF")
		}
		if len(delimiter) == 1 && q == '"' && !python {
			if decoded, err := strconv.Unquote(source[start:end]); err == nil {
				value = decoded
			}
		}
		out = append(out, sourceLiteral{text: value, line: startLine, start: start, end: end, dynamic: dynamic})
		line += strings.Count(source[start:end], "\n")
		blank(start, end)
		i = end
	}
	return out, string(mask)
}

var literalAddition = regexp.MustCompile(`^\s*\+\s*(?:([\pL_][\pL\pN_.]*)\s*\+\s*)?$`)

// Join only syntax whose operands remain literal, or a single explicit variable
// hole. Separate SQL arguments/statements and arbitrary calls are never joined.
func joinSQLLiterals(source, mask string, literals []sourceLiteral, python bool) []sourceLiteral {
	var out []sourceLiteral
	for i := 0; i < len(literals); i++ {
		current := literals[i]
		if !sqlStart.MatchString(current.text) {
			continue
		}
		first := literalSourceStart(source, current.start, python)
		current.segments = []sqlSourceSegment{{text: current.text, line: current.line, multiline: strings.Contains(source[current.start:current.end], "\n")}}
		for i+1 < len(literals) {
			next := literals[i+1]
			gap := mask[current.end:next.start]
			if python {
				gap = strings.TrimRight(gap, "fFrRuUbB")
			}
			addition := literalAddition.FindStringSubmatch(gap)
			adjacent := python && strings.TrimSpace(gap) == ""
			if adjacent && strings.Contains(gap, "\n") {
				prefix := mask[:first]
				adjacent = strings.Count(prefix, "(") > strings.Count(prefix, ")")
			}
			if !adjacent && len(addition) == 0 {
				break
			}
			if len(addition) > 1 && addition[1] != "" {
				current.text += "{" + addition[1] + "}"
				current.dynamic = true
			}
			current.segments = append(current.segments, sqlSourceSegment{text: next.text, offset: len(current.text), line: next.line, multiline: strings.Contains(source[next.start:next.end], "\n")})
			current.text += next.text
			current.dynamic = current.dynamic || next.dynamic
			current.end = next.end
			current.expression = source[first:current.end]
			i++
		}
		if current.dynamic && current.expression == "" {
			current.expression = source[first:current.end]
		}
		out = append(out, current)
	}
	return out
}

func literalSourceStart(source string, start int, python bool) int {
	if !python {
		return start
	}
	begin := start
	for begin > 0 && strings.ContainsRune("fFrRuUbB", rune(source[begin-1])) {
		begin--
	}
	if begin > 0 && (unicode.IsLetter(rune(source[begin-1])) || source[begin-1] == '_') {
		return start
	}
	return begin
}

// SQL semicolons are separators only outside comments and quoted values. The
// source text and its line numbers survive unchanged, including sqlc names.
func sqlStatements(source string) []sourceLiteral {
	var out []sourceLiteral
	begin, line, beginLine := 0, 1, 1
	for i := 0; i < len(source); {
		if strings.HasPrefix(source[i:], "--") {
			for i < len(source) && source[i] != '\n' {
				i++
			}
			continue
		}
		if strings.HasPrefix(source[i:], "/*") {
			end := strings.Index(source[i+2:], "*/")
			if end < 0 {
				break
			}
			end += i + 4
			line += strings.Count(source[i:end], "\n")
			i = end
			continue
		}
		if source[i] == '\'' || source[i] == '"' || source[i] == '`' || source[i] == '[' {
			q := source[i]
			if q == '[' {
				q = ']'
			}
			i++
			for i < len(source) {
				if source[i] == '\n' {
					line++
				}
				if source[i] == q {
					i++
					if i < len(source) && source[i] == q {
						i++
						continue
					}
					break
				}
				i++
			}
			continue
		}
		if source[i] == ';' {
			out = append(out, sourceLiteral{text: source[begin:i], line: beginLine})
			begin, beginLine = i+1, line
		}
		if source[i] == '\n' {
			line++
		}
		i++
	}
	if begin < len(source) {
		out = append(out, sourceLiteral{text: source[begin:], line: beginLine})
	}
	return out
}
