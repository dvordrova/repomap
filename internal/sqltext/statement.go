package sqltext

import "strings"

// Statement recognizes a supported SQL statement shape in an unbound source
// literal: a leading verb must be followed by its statement structure, so an
// error message or help text that merely starts with an SQL verb is not SQL.
// It does not establish execution or the absence of other SQL: ambiguous bare
// SELECT words and unsupported dialect forms remain ordinary source text.
// Explicit .sql/sqlc inputs bypass this admission check.
func Statement(source string) bool {
	tokens, partial := Tokens(source, 1)
	if len(tokens) == 0 {
		return false
	}
	switch {
	case sqlWord(tokens, 0, "CREATE"):
		i := 1
		if sqlWord(tokens, i, "OR") && sqlWord(tokens, i+1, "REPLACE") {
			i += 2
		}
		for sqlWord(tokens, i, "TEMP", "TEMPORARY", "UNLOGGED", "UNIQUE") {
			i++
		}
		if !sqlWord(tokens, i, "TABLE", "INDEX", "VIEW", "SEQUENCE", "TRIGGER", "FUNCTION", "SCHEMA", "DATABASE") {
			return false
		}
		i++
		if sqlWord(tokens, i, "IF") && sqlWord(tokens, i+1, "NOT") && sqlWord(tokens, i+2, "EXISTS") {
			i += 3
		}
		_, ok := sqlSourceName(tokens, i)
		return ok || partial
	case sqlWord(tokens, 0, "ALTER", "DROP"):
		if !sqlWord(tokens, 1, "TABLE", "INDEX", "VIEW", "SEQUENCE", "TRIGGER", "FUNCTION", "SCHEMA", "DATABASE") {
			return false
		}
		i := 2
		if sqlWord(tokens, i, "IF") && sqlWord(tokens, i+1, "EXISTS") {
			i += 2
		}
		i, ok := sqlSourceName(tokens, i)
		return (ok && (sqlWord(tokens, 0, "DROP") || sqlWord(tokens, i, "ADD", "ALTER", "DROP", "RENAME", "RESTART", "SET", "OWNER"))) || partial
	case sqlWord(tokens, 0, "INSERT"):
		i := 1
		if sqlWord(tokens, i, "OR") && sqlWord(tokens, i+1, "REPLACE", "IGNORE", "ABORT", "FAIL", "ROLLBACK") {
			i += 2
		}
		if !sqlWord(tokens, i, "INTO") {
			return false
		}
		_, ok := sqlSourceName(tokens, i+1)
		return ok || partial
	case sqlWord(tokens, 0, "DELETE"):
		if !sqlWord(tokens, 1, "FROM") {
			return false
		}
		_, ok := sqlSourceName(tokens, 2)
		return ok || partial
	case sqlWord(tokens, 0, "MERGE", "REPLACE"):
		if !sqlWord(tokens, 1, "INTO") {
			return false
		}
		_, ok := sqlSourceName(tokens, 2)
		return ok || partial
	case sqlWord(tokens, 0, "UPDATE"):
		i, ok := sqlSourceName(tokens, 1)
		if !ok || sqlWord(tokens, i, "SET") {
			return ok
		}
		if sqlWord(tokens, i, "AS") {
			i++
		}
		i, ok = sqlSourceName(tokens, i)
		return ok && sqlWord(tokens, i, "SET")
	case sqlWord(tokens, 0, "WITH"):
		i := 1
		if sqlWord(tokens, i, "RECURSIVE") {
			i++
		}
		i, ok := sqlSourceName(tokens, i)
		if !ok {
			return false
		}
		if sqlPunctuation(tokens, i, "(") {
			i = sqlAfterParentheses(tokens, i)
		}
		if !sqlWord(tokens, i, "AS") {
			return false
		}
		i++
		if sqlWord(tokens, i, "NOT") {
			i++
		}
		if sqlWord(tokens, i, "MATERIALIZED") {
			i++
		}
		return sqlPunctuation(tokens, i, "(")
	case sqlWord(tokens, 0, "PRAGMA"):
		i, ok := sqlSourceName(tokens, 1)
		return ok && (i == len(tokens) || sqlPunctuation(tokens, i, "=", "(", ";"))
	case sqlWord(tokens, 0, "SELECT"):
		return embeddedSelect(tokens)
	default:
		return false
	}
}

func embeddedSelect(tokens []Token) bool {
	i := 1
	if sqlWord(tokens, i, "DISTINCT", "ALL") {
		i++
	}
	if i >= len(tokens) {
		return false
	}
	// Written expression forms (CASE, COLLATE, UNION and aliases) can put
	// several tokens before FROM. Preserve that statement evidence without
	// guessing the expression's semantics or interpreting quoted words.
	depth := 0
	for j := i; j < len(tokens); j++ {
		if sqlPunctuation(tokens, j, "(") {
			depth++
		} else if sqlPunctuation(tokens, j, ")") {
			depth--
		} else if j > i && depth == 0 && sqlWord(tokens, j, "FROM") {
			return true
		}
	}
	first := tokens[i]
	if strings.HasPrefix(first.Text, "{") || strings.HasPrefix(first.Text, "${") {
		return true
	}
	if first.Literal || first.Quoted || sqlNumericStart(first.Text) || sqlPunctuation(tokens, i, "*", "(", "?", ":", "$") {
		return true
	}
	if sqlPunctuation(tokens, i, "+", "-") && i+1 < len(tokens) && sqlNumericStart(tokens[i+1].Text) {
		return true
	}
	if sqlWord(tokens, i, "NULL", "TRUE", "FALSE", "CURRENT_DATE", "CURRENT_TIME", "CURRENT_TIMESTAMP") {
		return true
	}
	end, ok := sqlSourceName(tokens, i)
	if !ok {
		return false
	}
	if end > i+1 || sqlPunctuation(tokens, end, "(", ",", "+", "-", "/", "*", "%", "=", "|", "<", ">") || sqlWord(tokens, end, "FROM", "WHERE", "AS") {
		return true
	}
	if sqlWord(tokens, i, "CASE") && sqlWord(tokens, end, "WHEN") {
		return true
	}
	// A bare column and optional alias are ambiguous in a prose literal.
	return false
}

func sqlNumericStart(text string) bool {
	return len(text) > 0 && text[0] >= '0' && text[0] <= '9'
}

func sqlWord(tokens []Token, index int, words ...string) bool {
	if index < 0 || index >= len(tokens) || tokens[index].Quoted || tokens[index].Literal {
		return false
	}
	for _, word := range words {
		if strings.EqualFold(tokens[index].Text, word) {
			return true
		}
	}
	return false
}

func sqlPunctuation(tokens []Token, index int, values ...string) bool {
	if index < 0 || index >= len(tokens) || tokens[index].Quoted || tokens[index].Literal {
		return false
	}
	for _, value := range values {
		if tokens[index].Text == value {
			return true
		}
	}
	return false
}

// A source hole proves only that an identifier is supplied there. It remains
// partial and never becomes a declared table name through Identifier.
func sqlSourceName(tokens []Token, index int) (int, bool) {
	if index < 0 || index >= len(tokens) {
		return index, false
	}
	if strings.HasPrefix(tokens[index].Text, "{") || strings.HasPrefix(tokens[index].Text, "${") {
		return index + 1, true
	}
	if sqlWord(tokens, index, "FROM", "INTO", "AS", "IF", "NOT", "EXISTS", "SET", "SELECT") {
		return index, false
	}
	name, end := Identifier(tokens, index)
	return end, name != ""
}

func sqlAfterParentheses(tokens []Token, index int) int {
	depth := 0
	for ; index < len(tokens); index++ {
		if sqlPunctuation(tokens, index, "(") {
			depth++
		}
		if sqlPunctuation(tokens, index, ")") {
			depth--
			if depth == 0 {
				return index + 1
			}
		}
	}
	return index
}
