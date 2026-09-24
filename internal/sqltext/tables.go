package sqltext

import (
	"sort"
	"strings"
)

// Tables lists, sorted and as written, the names that follow FROM, JOIN,
// INTO, UPDATE or TABLE, except names the statement itself defines with WITH.
func Tables(tokens []Token) []string {
	ctes := map[string]bool{}
	tables := map[string]bool{}
	for i := 0; i+2 < len(tokens); i++ {
		if (i == 0 && strings.EqualFold(tokens[i].Text, "with")) || tokens[i].Text == "," {
			if strings.EqualFold(tokens[i+2].Text, "as") {
				ctes[tokens[i+1].Text] = true
			}
		}
	}
	for i := 0; i+1 < len(tokens); i++ {
		if tokens[i].Quoted {
			continue
		}
		switch strings.ToUpper(tokens[i].Text) {
		case "FROM", "JOIN", "INTO", "UPDATE", "TABLE":
		default:
			continue
		}
		j := i + 1
		if strings.EqualFold(tokens[j].Text, "if") {
			j += 3
		}
		name, _ := Identifier(tokens, j)
		if name != "" && !strings.EqualFold(name, "set") && !ctes[name] {
			tables[name] = true
		}
	}
	out := make([]string, 0, len(tables))
	for name := range tables {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
