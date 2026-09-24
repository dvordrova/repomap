package sqltext

import (
	"sort"
	"strings"
)

// Tables lists, sorted and as written, the names that follow FROM, JOIN,
// INTO, UPDATE or TABLE, except names the statement itself defines with WITH.
// IF [NOT] EXISTS, ONLY and LATERAL are skipped, and ON DUPLICATE KEY UPDATE
// names columns, not a table. A name written with a template hole or printf
// verb is supplied at run time and is not listed.
func Tables(tokens []Token) []string {
	tables := map[string]bool{}
	tableMentions(tokens, func(name string, runtime bool) {
		if !runtime {
			tables[name] = true
		}
	})
	out := make([]string, 0, len(tables))
	for name := range tables {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// RuntimeTable reports whether a table position holds a template hole or a
// printf verb, so the text names its table only at run time.
func RuntimeTable(tokens []Token) bool {
	found := false
	tableMentions(tokens, func(_ string, runtime bool) { found = found || runtime })
	return found
}

func tableMentions(tokens []Token, visit func(name string, runtime bool)) {
	ctes := map[string]bool{}
	for i := 0; i+2 < len(tokens); i++ {
		if (i == 0 && strings.EqualFold(tokens[i].Text, "with")) || tokens[i].Text == "," {
			if strings.EqualFold(tokens[i+2].Text, "as") {
				ctes[tokens[i+1].Text] = true
			}
		}
	}
	for i := 0; i+1 < len(tokens); i++ {
		if !sqlWord(tokens, i, "FROM", "JOIN", "INTO", "UPDATE", "TABLE") {
			continue
		}
		// ON DUPLICATE KEY UPDATE assigns columns of the INSERT's table.
		if sqlWord(tokens, i, "UPDATE") && sqlWord(tokens, i-1, "KEY") {
			continue
		}
		j := i + 1
		switch {
		case sqlWord(tokens, j, "IF") && sqlWord(tokens, j+1, "NOT") && sqlWord(tokens, j+2, "EXISTS"):
			j += 3
		case sqlWord(tokens, j, "IF") && sqlWord(tokens, j+1, "EXISTS"):
			j += 2
		}
		for sqlWord(tokens, j, "ONLY", "LATERAL", "LOW_PRIORITY", "IGNORE") {
			j++
		}
		if sqlWord(tokens, j, reservedNames...) {
			continue
		}
		name, _, runtime := ObjectName(tokens, j)
		switch {
		case runtime:
			visit("", true)
		case name != "" && !ctes[name]:
			visit(name, false)
		}
	}
}
