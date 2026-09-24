package extractors

import (
	"fmt"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/sqltext"
	"regexp"
	"strings"
)

var sqlStart = regexp.MustCompile(`(?i)^\s*(SELECT|INSERT|UPDATE|DELETE|WITH|CREATE|ALTER|DROP|PRAGMA|MERGE|REPLACE)\b`)
var sqlName = regexp.MustCompile(`(?m)--\s*name:\s*([A-Za-z_][A-Za-z_0-9]*)\s*:`)

func parseSQLColumns(tokens []sqltext.Token, path string, source ...func(sqltext.Token) facts.Anchor) []facts.DataColumn {
	start := -1
	for i, token := range tokens {
		if token.Text == "(" {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return nil
	}
	var chunks [][]sqltext.Token
	level, begin := 0, start
	for i := start; i < len(tokens); i++ {
		switch tokens[i].Text {
		case "(":
			level++
		case ")":
			if level == 0 {
				chunks = append(chunks, tokens[begin:i])
				i = len(tokens)
				continue
			}
			level--
		case ",":
			if level == 0 {
				chunks = append(chunks, tokens[begin:i])
				begin = i + 1
			}
		}
	}
	var columns []facts.DataColumn
	pk := map[string]bool{}
	fk := map[string]string{}
	for _, chunk := range chunks {
		if len(chunk) == 0 {
			continue
		}
		first := strings.ToUpper(chunk[0].Text)
		if first == "CONSTRAINT" || first == "PRIMARY" || first == "FOREIGN" || first == "UNIQUE" || first == "CHECK" {
			for i, t := range chunk {
				if strings.EqualFold(t.Text, "primary") && i+1 < len(chunk) && strings.EqualFold(chunk[i+1].Text, "key") {
					for _, name := range sqlIdentifierList(chunk, i+2) {
						pk[name] = true
					}
				}
				if strings.EqualFold(t.Text, "foreign") && i+1 < len(chunk) && strings.EqualFold(chunk[i+1].Text, "key") {
					locals := sqlIdentifierList(chunk, i+2)
					for j := i + 2; j < len(chunk); j++ {
						if !strings.EqualFold(chunk[j].Text, "references") {
							continue
						}
						table, end := sqltext.Identifier(chunk, j+1)
						remotes := sqlIdentifierList(chunk, end)
						if table != "" && len(locals) == len(remotes) {
							for k, local := range locals {
								fk[local] = table + "." + remotes[k]
							}
						}
						break
					}
				}
			}
			continue
		}
		name, next := sqltext.Identifier(chunk, 0)
		if name == "" {
			continue
		}
		column := facts.DataColumn{Name: name, Anchor: facts.Anchor{Path: path, Line: chunk[0].Line}}
		if len(source) > 0 && source[0] != nil {
			column.Anchor = source[0](chunk[0])
		}
		if next < len(chunk) {
			column.Type = chunk[next].Text
		}
		for i, token := range chunk {
			switch strings.ToUpper(token.Text) {
			case "PRIMARY":
				column.PrimaryKey = true
			case "REFERENCES":
				ref, j := sqltext.Identifier(chunk, i+1)
				if j+1 < len(chunk) && chunk[j].Text == "(" {
					ref += "." + chunk[j+1].Text
				}
				column.ForeignKey = ref
			}
		}
		columns = append(columns, column)
	}
	for i := range columns {
		columns[i].PrimaryKey = columns[i].PrimaryKey || pk[columns[i].Name]
		if ref := fk[columns[i].Name]; ref != "" {
			columns[i].ForeignKey = ref
		}
	}
	return columns
}

func sqlIdentifierList(tokens []sqltext.Token, start int) []string {
	if start >= len(tokens) || tokens[start].Text != "(" {
		return nil
	}
	var names []string
	for i := start + 1; i < len(tokens); {
		name, end := sqltext.Identifier(tokens, i)
		if name == "" {
			return nil
		}
		names = append(names, name)
		if end < len(tokens) && tokens[end].Text == ")" {
			return names
		}
		if end >= len(tokens) || tokens[end].Text != "," {
			return nil
		}
		i = end + 1
	}
	return nil
}

func (b *databaseExtractor) addSQL(path, scope, source string, line int, dynamic bool, owner *facts.Anchor) string {
	tokens, partial := sqltext.Tokens(source, line)
	if len(tokens) == 0 {
		return ""
	}
	line = tokens[0].Line
	if b.sourceAnchor != nil {
		line = b.sourceAnchor(tokens[0]).Line
	}
	balance := 0
	for _, token := range tokens {
		if token.Text == "(" {
			balance++
		}
		if token.Text == ")" {
			balance--
		}
		if balance < 0 {
			partial = true
		}
	}
	// A table named by a printf verb or template hole is filled in at run time.
	partial = partial || balance != 0 || sqltext.RuntimeTable(tokens)
	statement := strings.ToUpper(tokens[0].Text)
	if !sqlStart.MatchString(statement) {
		return ""
	}
	tables := sqltext.Tables(tokens)
	name := fmt.Sprintf("%s · %s:%d", statement, path, line)
	if named := sqlName.FindStringSubmatch(source); len(named) > 1 {
		name = named[1]
	}
	data := &facts.DataObject{Kind: "query", Origin: "query", Scope: scope, Name: name, Statement: statement, SQL: source, Partial: partial || dynamic, Tables: tables, Owner: owner}
	id := fmt.Sprintf("query:%s:%s:%d:%d", scope, path, line, len(b.response.Nodes))
	b.addNode(facts.ExtractionNode{ID: id, Name: name, Path: path, Line: line, Data: data})
	b.queries = append(b.queries, id)
	if statement == "CREATE" {
		for i := 1; i < len(tokens); i++ {
			if !strings.EqualFold(tokens[i].Text, "table") {
				continue
			}
			j := i + 1
			if j < len(tokens) && strings.EqualFold(tokens[j].Text, "if") {
				j += 3
			}
			table, _, _ := sqltext.ObjectName(tokens, j)
			if table != "" {
				tableLine := tokens[i].Line
				if b.sourceAnchor != nil {
					tableLine = b.sourceAnchor(tokens[i]).Line
				}
				b.addTable(path, tableLine, &facts.DataObject{Kind: "table", Origin: "ddl", Scope: scope, Name: table, Columns: parseSQLColumns(tokens[j:], path, b.sourceAnchor), Partial: partial || dynamic})
			}
			break
		}
	}
	return id
}
