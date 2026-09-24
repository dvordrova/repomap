package sqltext

import "strings"

// Statement recognizes a supported SQL statement shape in an unbound source
// literal: a leading verb must be followed by its statement structure, so an
// error message or help text that merely starts with an SQL verb is not SQL.
// An object name may be filled in by the source (DROP TABLE %s, UPDATE {t}
// SET ...); a ':' after it ends a message instead. Supported dialect forms
// include SELECT [NOT] EXISTS (...), MySQL's INSERT IGNORE, UPDATE ... JOIN
// ... SET, multi-table UPDATE and DELETE ... FROM ... JOIN.
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
		for sqlWord(tokens, i, "TEMP", "TEMPORARY", "UNLOGGED", "UNIQUE", "GLOBAL", "LOCAL", "VIRTUAL", "MATERIALIZED", "RECURSIVE") {
			i++
		}
		if !sqlWord(tokens, i, objectKinds...) {
			return false
		}
		i++
		if sqlWord(tokens, i, "IF") && sqlWord(tokens, i+1, "NOT") && sqlWord(tokens, i+2, "EXISTS") {
			i += 3
		}
		return sqlObject(tokens, i, partial)
	case sqlWord(tokens, 0, "ALTER", "DROP"):
		i := 1
		if sqlWord(tokens, i, "MATERIALIZED") {
			i++
		}
		if !sqlWord(tokens, i, objectKinds...) {
			return false
		}
		i++
		if sqlWord(tokens, i, "IF") && sqlWord(tokens, i+1, "EXISTS") {
			i += 2
		}
		if sqlWord(tokens, i, "ONLY") {
			i++
		}
		if sqlWord(tokens, 0, "DROP") {
			return sqlObject(tokens, i, partial)
		}
		i, ok := sqlSourceName(tokens, i)
		return (ok && sqlWord(tokens, i, alterActions...)) || partial
	case sqlWord(tokens, 0, "INSERT"):
		i := 1
		if sqlWord(tokens, i, "OR") && sqlWord(tokens, i+1, "REPLACE", "IGNORE", "ABORT", "FAIL", "ROLLBACK") {
			i += 2
		}
		for sqlWord(tokens, i, "LOW_PRIORITY", "DELAYED", "HIGH_PRIORITY", "IGNORE") {
			i++
		}
		return sqlWord(tokens, i, "INTO") && sqlObject(tokens, i+1, partial)
	case sqlWord(tokens, 0, "DELETE"):
		if sqlWord(tokens, 1, "FROM") {
			i := 2
			if sqlWord(tokens, i, "ONLY") {
				i++
			}
			return sqlObject(tokens, i, partial)
		}
		return multiTableDelete(tokens)
	case sqlWord(tokens, 0, "MERGE", "REPLACE"):
		i := 1
		for sqlWord(tokens, 0, "REPLACE") && sqlWord(tokens, i, "LOW_PRIORITY", "DELAYED") {
			i++
		}
		return sqlWord(tokens, i, "INTO") && sqlObject(tokens, i+1, partial)
	case sqlWord(tokens, 0, "UPDATE"):
		return update(tokens)
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

// The objects CREATE, ALTER and DROP take, and the actions ALTER applies.
var (
	objectKinds  = []string{"TABLE", "INDEX", "VIEW", "SEQUENCE", "TRIGGER", "FUNCTION", "PROCEDURE", "SCHEMA", "DATABASE", "TYPE", "DOMAIN", "EXTENSION"}
	alterActions = []string{"ADD", "ALTER", "DROP", "RENAME", "RESTART", "SET", "RESET", "OWNER", "MODIFY", "CHANGE", "ENABLE", "DISABLE", "ATTACH", "DETACH", "VALIDATE", "INHERIT", "NO", "CLUSTER", "REPLICA", "ENGINE", "CONVERT", "UPDATE"}
	joinWords    = []string{"JOIN", "INNER", "LEFT", "RIGHT", "FULL", "CROSS", "NATURAL", "STRAIGHT_JOIN"}
)

// sqlObject reports that a statement names its object at index. The name may
// be written or supplied at run time, but a ':' right after it ends a message
// prefix ("create table %s: %w"), which SQL never writes after an object name.
func sqlObject(tokens []Token, index int, partial bool) bool {
	end, ok := sqlSourceName(tokens, index)
	if !ok {
		return partial
	}
	return !sqlPunctuation(tokens, end, ":")
}

// update reads UPDATE's targets: one or more comma-separated names with
// optional aliases, then SET directly or after a join (MySQL's
// UPDATE a JOIN b ON ... SET), and SET must start an assignment.
func update(tokens []Token) bool {
	i := 1
	if sqlWord(tokens, i, "OR") && sqlWord(tokens, i+1, "REPLACE", "IGNORE", "ABORT", "FAIL", "ROLLBACK") {
		i += 2
	}
	for sqlWord(tokens, i, "LOW_PRIORITY", "IGNORE", "ONLY") {
		i++
	}
	for {
		end, ok := sqlSourceName(tokens, i)
		if !ok {
			return false
		}
		i = sqlAlias(tokens, end)
		if !sqlPunctuation(tokens, i, ",") {
			break
		}
		i++
	}
	if sqlWord(tokens, i, joinWords...) {
		depth := 0
		for ; i < len(tokens); i++ {
			if sqlPunctuation(tokens, i, "(") {
				depth++
			} else if sqlPunctuation(tokens, i, ")") {
				depth--
			} else if depth == 0 && sqlWord(tokens, i, "SET") {
				break
			}
		}
	}
	return sqlWord(tokens, i, "SET") && sqlAssignment(tokens, i+1)
}

// sqlAssignment is what follows SET: a column (or a column list in
// parentheses) and '=', or a hole that supplies the assignments.
func sqlAssignment(tokens []Token, index int) bool {
	if _, ok := sourceHole(tokens, index); ok || sqlPunctuation(tokens, index, "(") {
		return true
	}
	if sqlWord(tokens, index, "FROM", "WHERE", "TO") {
		return false
	}
	name, end, runtime := ObjectName(tokens, index)
	return (name != "" || runtime) && sqlPunctuation(tokens, end, "=")
}

// multiTableDelete is MySQL's DELETE t[.*][, u[.*]] FROM t JOIN u ...: the
// deleted names, FROM, and a source joined to another; a single source with no
// join ("Delete files from cache") is prose.
func multiTableDelete(tokens []Token) bool {
	i := 1
	for {
		end, ok := sqlSourceName(tokens, i)
		if !ok {
			return false
		}
		i = end
		if sqlPunctuation(tokens, i, ".") && sqlPunctuation(tokens, i+1, "*") {
			i += 2
		}
		if !sqlPunctuation(tokens, i, ",") {
			break
		}
		i++
	}
	if !sqlWord(tokens, i, "FROM") {
		return false
	}
	end, ok := sqlSourceName(tokens, i+1)
	if !ok {
		return false
	}
	i = sqlAlias(tokens, end)
	return sqlWord(tokens, i, joinWords...) || sqlPunctuation(tokens, i, ",")
}

// sqlAlias skips AS alias or a bare alias that is not the next clause's word.
func sqlAlias(tokens []Token, index int) int {
	if sqlWord(tokens, index, "AS") {
		if _, ok := sqlName(tokens, index+1); ok {
			return index + 2
		}
		return index
	}
	if sqlWord(tokens, index, "SET", "FROM", "WHERE", "ON", "USING", "OUTER") || sqlWord(tokens, index, joinWords...) {
		return index
	}
	if end, ok := sqlName(tokens, index); ok && end == index+1 {
		return end
	}
	return index
}

func embeddedSelect(tokens []Token) bool {
	i := 1
	if sqlWord(tokens, i, "DISTINCT", "ALL") {
		i++
	}
	if i >= len(tokens) {
		return false
	}
	if sqlWord(tokens, i, "NOT") && sqlWord(tokens, i+1, "EXISTS") {
		i++
	}
	if sqlWord(tokens, i, "EXISTS") && sqlPunctuation(tokens, i+1, "(") {
		return true
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
	if first.Literal || first.Quoted || sqlNumericStart(first.Text) || sqlPunctuation(tokens, i, "*", "(", "?", "$") {
		return true
	}
	// A named placeholder (:name) or a server variable (@@name) is written
	// against its name; "select: empty target list" is a message.
	if sqlPunctuation(tokens, i, ":") && adjacent(tokens, i+1) && !tokens[i+1].Quoted && !tokens[i+1].Literal {
		return true
	}
	if sqlPunctuation(tokens, i, "@") && sqlPunctuation(tokens, i+1, "@") && adjacent(tokens, i+1) && adjacent(tokens, i+2) {
		return true
	}
	if sqlPunctuation(tokens, i, "+", "-") && i+1 < len(tokens) && sqlNumericStart(tokens[i+1].Text) {
		return true
	}
	if sqlWord(tokens, i, "NULL", "TRUE", "FALSE", "CURRENT_DATE", "CURRENT_TIME", "CURRENT_TIMESTAMP") {
		return true
	}
	end, ok := sqlName(tokens, i)
	if !ok {
		return false
	}
	if end > i+1 || sqlPunctuation(tokens, end, "(", ",", "+", "-", "/", "*", "=", "|", "<", ">") || sqlWord(tokens, end, "FROM", "WHERE", "AS") {
		return true
	}
	// '%' is modulo unless it is a printf verb ("select user %d name").
	if _, verb := sourceHole(tokens, end); sqlPunctuation(tokens, end, "%") && !verb {
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

// sqlSourceName reads a statement's object name. A template hole or printf
// verb proves only that a name is supplied there; it never becomes a declared
// table name.
func sqlSourceName(tokens []Token, index int) (int, bool) {
	if sqlWord(tokens, index, reservedNames...) {
		return index, false
	}
	name, end, runtime := ObjectName(tokens, index)
	return end, name != "" || runtime
}

// sqlName reads a written, possibly qualified name that is not a clause word.
func sqlName(tokens []Token, index int) (int, bool) {
	if index < 0 || index >= len(tokens) || sqlWord(tokens, index, reservedNames...) {
		return index, false
	}
	name, end := Identifier(tokens, index)
	return end, name != ""
}

var reservedNames = []string{"FROM", "INTO", "AS", "IF", "NOT", "EXISTS", "SET", "SELECT", "WITH", "WHERE", "VALUES"}

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
