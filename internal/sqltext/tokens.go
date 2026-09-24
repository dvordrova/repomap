// Package sqltext reads SQL text lexically: tokens, identifiers, the tables a
// statement names and whether an unbound literal has SQL statement structure.
// It opens no database and names no driver; every caller that asks whether a
// source string is SQL gets the same answer.
package sqltext

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Token is one lexical SQL unit with its source line and byte offset. Quoted
// is a quoted identifier (its text without the quotes); Literal is a quoted
// value, kept with its quotes.
type Token struct {
	Text    string
	Line    int
	Quoted  bool
	Literal bool
	Offset  int
}

// Tokens splits SQL text, skipping comments. A template hole ({name} or
// ${name}), an unterminated quote or comment makes the result partial.
func Tokens(source string, startLine int) ([]Token, bool) {
	var tokens []Token
	line := startLine
	partial := false
	for i := 0; i < len(source); {
		if source[i] == '\n' {
			line++
			i++
			continue
		}
		if unicode.IsSpace(rune(source[i])) {
			i++
			continue
		}
		if strings.HasPrefix(source[i:], "--") {
			for i < len(source) && source[i] != '\n' {
				i++
			}
			continue
		}
		if strings.HasPrefix(source[i:], "/*") {
			end := strings.Index(source[i+2:], "*/")
			if end < 0 {
				return tokens, true
			}
			end += i + 4
			line += strings.Count(source[i:end], "\n")
			i = end
			continue
		}
		if source[i] == '\'' {
			start, startLine := i, line
			i++
			closed := false
			for i < len(source) {
				if source[i] == '\'' {
					i++
					if i < len(source) && source[i] == '\'' {
						i++
						continue
					}
					closed = true
					break
				}
				if source[i] == '\n' {
					line++
				}
				i++
			}
			partial = partial || !closed
			tokens = append(tokens, Token{Text: source[start:i], Line: startLine, Literal: true, Offset: start})
			continue
		}
		if source[i] == '"' || source[i] == '`' || source[i] == '[' {
			offset := i
			q := source[i]
			close := q
			if q == '[' {
				close = ']'
			}
			start := i + 1
			i++
			for i < len(source) && source[i] != close {
				i++
			}
			if i == len(source) {
				return tokens, true
			}
			tokens = append(tokens, Token{Text: source[start:i], Line: line, Quoted: true, Offset: offset})
			i++
			continue
		}
		if source[i] == '{' || strings.HasPrefix(source[i:], "${") {
			start := i
			for i < len(source) && source[i] != '}' {
				i++
			}
			i = min(i+1, len(source))
			tokens = append(tokens, Token{Text: source[start:i], Line: line, Offset: start})
			partial = true
			continue
		}
		r, width := utf8.DecodeRuneInString(source[i:])
		if unicode.IsLetter(r) || r == '_' {
			start := i
			i += width
			for i < len(source) {
				r, width = utf8.DecodeRuneInString(source[i:])
				if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '$' {
					break
				}
				i += width
			}
			tokens = append(tokens, Token{Text: source[start:i], Line: line, Offset: start})
			continue
		}
		tokens = append(tokens, Token{Text: string(source[i]), Line: line, Offset: i})
		i++
	}
	return tokens, partial
}

// Identifier reads a possibly qualified name at index and returns it with the
// index after it; a value literal or a non-name token yields "".
func Identifier(tokens []Token, index int) (string, int) {
	if index >= len(tokens) || tokens[index].Literal {
		return "", index
	}
	name := tokens[index].Text
	first, _ := utf8.DecodeRuneInString(name)
	if name == "" || !tokens[index].Quoted && !unicode.IsLetter(first) && first != '_' {
		return "", index
	}
	if tokens[index].Quoted && strings.ContainsAny(name, ".\"") {
		name = "\"" + strings.ReplaceAll(name, "\"", "\"\"") + "\""
	}
	index++
	for index+1 < len(tokens) && tokens[index].Text == "." {
		part, next := Identifier(tokens, index+1)
		if part == "" {
			break
		}
		name += "." + part
		index = next
	}
	return name, index
}
