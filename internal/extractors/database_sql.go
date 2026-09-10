package extractors

import (
	"fmt"
	"github.com/dvordrova/repomap/internal/facts"
	"regexp"
	"sort"
	"strings"
)

var sqlStart = regexp.MustCompile(`(?i)^\s*(SELECT|INSERT|UPDATE|DELETE|WITH|CREATE|ALTER|DROP|PRAGMA)\b`)
var sqlName = regexp.MustCompile(`(?m)--\s*name:\s*([A-Za-z_][A-Za-z_0-9]*)\s*:`)

func sqlTables(tokens []sqlToken) []string {
	ctes := map[string]bool{}
	tables := map[string]bool{}
	for i := 0; i+2 < len(tokens); i++ {
		if (i == 0 && strings.EqualFold(tokens[i].text, "with")) || tokens[i].text == "," {
			if strings.EqualFold(tokens[i+2].text, "as") {
				ctes[tokens[i+1].text] = true
			}
		}
	}
	for i := 0; i+1 < len(tokens); i++ {
		if tokens[i].quoted {
			continue
		}
		switch strings.ToUpper(tokens[i].text) {
		case "FROM", "JOIN", "INTO", "UPDATE", "TABLE":
		default:
			continue
		}
		j := i + 1
		if strings.EqualFold(tokens[j].text, "if") {
			j += 3
		}
		name, _ := sqlIdentifier(tokens, j)
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
func parseSQLColumns(tokens []sqlToken, path string, source ...func(sqlToken) facts.Anchor) []facts.DataColumn {
	start := -1
	for i, token := range tokens {
		if token.text == "(" {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return nil
	}
	var chunks [][]sqlToken
	level, begin := 0, start
	for i := start; i < len(tokens); i++ {
		switch tokens[i].text {
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
		first := strings.ToUpper(chunk[0].text)
		if first == "CONSTRAINT" || first == "PRIMARY" || first == "FOREIGN" || first == "UNIQUE" || first == "CHECK" {
			for i, t := range chunk {
				if strings.EqualFold(t.text, "primary") && i+1 < len(chunk) && strings.EqualFold(chunk[i+1].text, "key") {
					for _, name := range sqlIdentifierList(chunk, i+2) {
						pk[name] = true
					}
				}
				if strings.EqualFold(t.text, "foreign") && i+1 < len(chunk) && strings.EqualFold(chunk[i+1].text, "key") {
					locals := sqlIdentifierList(chunk, i+2)
					for j := i + 2; j < len(chunk); j++ {
						if !strings.EqualFold(chunk[j].text, "references") {
							continue
						}
						table, end := sqlIdentifier(chunk, j+1)
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
		name, next := sqlIdentifier(chunk, 0)
		if name == "" {
			continue
		}
		column := facts.DataColumn{Name: name, Anchor: facts.Anchor{Path: path, Line: chunk[0].line}}
		if len(source) > 0 && source[0] != nil {
			column.Anchor = source[0](chunk[0])
		}
		if next < len(chunk) {
			column.Type = chunk[next].text
		}
		for i, token := range chunk {
			switch strings.ToUpper(token.text) {
			case "PRIMARY":
				column.PrimaryKey = true
			case "REFERENCES":
				ref, j := sqlIdentifier(chunk, i+1)
				if j+1 < len(chunk) && chunk[j].text == "(" {
					ref += "." + chunk[j+1].text
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

func sqlIdentifierList(tokens []sqlToken, start int) []string {
	if start >= len(tokens) || tokens[start].text != "(" {
		return nil
	}
	var names []string
	for i := start + 1; i < len(tokens); {
		name, end := sqlIdentifier(tokens, i)
		if name == "" {
			return nil
		}
		names = append(names, name)
		if end < len(tokens) && tokens[end].text == ")" {
			return names
		}
		if end >= len(tokens) || tokens[end].text != "," {
			return nil
		}
		i = end + 1
	}
	return nil
}

func (b *databaseExtractor) addSQL(path, scope, source string, line int, dynamic bool, owner *facts.Anchor) string {
	tokens, partial := sqlTokens(source, line)
	if len(tokens) == 0 {
		return ""
	}
	line = tokens[0].line
	if b.sourceAnchor != nil {
		line = b.sourceAnchor(tokens[0]).Line
	}
	balance := 0
	for _, token := range tokens {
		if token.text == "(" {
			balance++
		}
		if token.text == ")" {
			balance--
		}
		if balance < 0 {
			partial = true
		}
	}
	partial = partial || balance != 0
	statement := strings.ToUpper(tokens[0].text)
	if !sqlStart.MatchString(statement) {
		return ""
	}
	tables := sqlTables(tokens)
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
			if !strings.EqualFold(tokens[i].text, "table") {
				continue
			}
			j := i + 1
			if j < len(tokens) && strings.EqualFold(tokens[j].text, "if") {
				j += 3
			}
			table, _ := sqlIdentifier(tokens, j)
			if table != "" {
				tableLine := tokens[i].line
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
