package facts

import (
	"regexp"
	"sort"
	"strings"
)

// An SQL statement literal handed to a call the repository does not own is a
// fact about data: which tables the code reads or writes at that line. The
// statement is recognized by its first keyword, the tables by the keywords
// that precede a table name. No driver or ORM is named.

var sqlStatement = regexp.MustCompile(`(?is)^\s*(?:--[^\n]*\n\s*)*(select|insert|update|delete|with|create|alter|drop|merge|replace)\b`)

var sqlTable = regexp.MustCompile(`(?i)\b(?:from|join|into|update|table(?:\s+if\s+(?:not\s+)?exists)?)\s+` + "`?\"?" + `([A-Za-z_][A-Za-z0-9_.]*)`)

func (b *builder) addSQLQueries(target *targetContext) {
	for _, relation := range target.input.Index.Relations {
		if target.ownsCallee(relation) {
			continue
		}
		for _, pattern := range relation.Patterns {
			for _, argument := range pattern.Arguments {
				statement, _, literal := literalValue(argument)
				if !literal || !sqlStatement.MatchString(statement) {
					continue
				}
				anchor := target.patternAnchor(relation, pattern)
				if anchor == nil {
					continue
				}
				tables := sqlTables(statement)
				if !b.once(strings.Join([]string{string(KindSQLQuery), target.target.ID, anchor.String(), statement}, "\x00")) {
					continue
				}
				symbol, objectID := target.enclosingSymbol(relation.FromID)
				b.add(target.root, Fact{
					Kind:       KindSQLQuery,
					TargetID:   target.target.ID,
					Anchor:     anchor,
					Key:        strings.Join(tables, ", "),
					Value:      clipText(strings.Join(strings.Fields(statement), " ")),
					Symbol:     symbol,
					ObjectID:   objectID,
					Resolution: ResolutionExact,
				}, statement)
			}
		}
	}
}

func sqlTables(statement string) []string {
	seen := make(map[string]bool)
	var tables []string
	for _, match := range sqlTable.FindAllStringSubmatch(statement, -1) {
		name := strings.ToLower(match[1])
		if strings.EqualFold(name, "select") || seen[name] {
			continue
		}
		seen[name] = true
		tables = append(tables, name)
	}
	sort.Strings(tables)
	return tables
}
