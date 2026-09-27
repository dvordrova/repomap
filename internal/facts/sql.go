package facts

import (
	"strings"

	"github.com/dvordrova/repomap/internal/sqltext"
)

// An SQL statement literal handed to a call the repository does not own is a
// fact about data: which tables the code reads or writes at that line. The
// literal must have SQL statement structure (sqltext.Statement, the same
// admission the database extractor applies to unbound literals); a message or
// help text that merely starts with an SQL verb is not a statement. The tables
// are the written names after FROM, JOIN, INTO, UPDATE or TABLE; a table a
// printf verb or template hole fills in is named only at run time, so it is
// not listed. No driver or ORM is named.
func (b *builder) addSQLQueries(target *targetContext) {
	for _, relation := range target.input.Index.Relations {
		if target.ownsCallee(relation) || target.unreachable(relation) {
			continue
		}
		for _, pattern := range relation.Patterns {
			for _, argument := range pattern.Arguments {
				statement, _, literal := literalValue(argument)
				if !literal || !sqltext.Statement(statement) {
					continue
				}
				anchor := target.patternAnchor(relation, pattern)
				if anchor == nil {
					continue
				}
				tokens, _ := sqltext.Tokens(statement, 1)
				tables := sqltext.Tables(tokens)
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
