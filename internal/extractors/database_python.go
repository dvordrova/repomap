package extractors

import (
	"regexp"
	"strings"

	"github.com/dvordrova/repomap/internal/facts"
)

type dataOwner struct {
	name, scope string
	line, end   int
}

func dataOwnerAt(owners []dataOwner, line int) *dataOwner {
	var found *dataOwner
	for i := range owners {
		if line >= owners[i].line && line < owners[i].end {
			found = &owners[i]
		}
	}
	return found
}

var pythonClass = regexp.MustCompile(`^class\s+([A-Za-z_][A-Za-z_0-9]*)\s*\(([^)]*)\)\s*:`)
var pythonImport = regexp.MustCompile(`(?m)^from\s+([A-Za-z_.][A-Za-z_0-9.]*)\s+import\s+(\([^)]*\)|[^\n]+)`)
var pythonModuleImport = regexp.MustCompile(`(?m)^import\s+(sqlalchemy(?:\.orm)?)(?:\s+as\s+([A-Za-z_][A-Za-z_0-9]*))?\s*$`)
var pythonTable = regexp.MustCompile(`^\s*__tablename__\s*=`)
var pythonTableArgs = regexp.MustCompile(`^\s*__table_args__\s*=`)
var pythonField = regexp.MustCompile(`^([A-Za-z_][A-Za-z_0-9]*)\s*(?::\s*([^=]+))?(?:=\s*(.*))?$`)
var pythonCall = regexp.MustCompile(`([A-Za-z_][A-Za-z_0-9.]*)\s*\(`)
var primaryKey = regexp.MustCompile(`\bprimary_key\s*=\s*True\b`)

func (b *databaseExtractor) addPythonModels(file, source, mask string, literals []sourceLiteral) []dataOwner {
	lines := strings.Split(source, "\n")
	masked := strings.Split(mask, "\n")
	imports := map[string]string{}
	for _, match := range pythonModuleImport.FindAllStringSubmatch(mask, -1) {
		local := match[2]
		if local == "" {
			local = strings.Split(match[1], ".")[0]
		}
		qualified := match[1]
		if match[2] == "" {
			qualified = local
		}
		imports[local] = qualified
	}
	for _, match := range pythonImport.FindAllStringSubmatch(mask, -1) {
		module := match[1]
		for _, part := range strings.Split(match[2], ",") {
			fields := strings.Fields(strings.TrimSpace(strings.Trim(part, " ()\n\r\t")))
			if len(fields) == 0 {
				continue
			}
			local := fields[0]
			if len(fields) == 3 && fields[1] == "as" {
				local = fields[2]
			}
			imports[local] = module + "." + fields[0]
		}
	}
	orm := false
	for _, name := range imports {
		orm = orm || name == "sqlalchemy" || strings.HasPrefix(name, "sqlalchemy.")
	}
	if !orm {
		return nil
	}
	var owners []dataOwner
	for i, line := range masked {
		match := pythonClass.FindStringSubmatch(line)
		if len(match) == 0 {
			continue
		}
		end := len(masked)
		for j := i + 1; j < len(masked); j++ {
			if strings.TrimSpace(masked[j]) != "" && masked[j][0] != ' ' && masked[j][0] != '\t' {
				end = j
				break
			}
		}
		var bases []string
		for _, base := range strings.Split(match[2], ",") {
			base = strings.TrimSpace(base)
			qualified := pythonDataName(base, imports)
			if qualified == "" {
				qualified = file + ":" + base
			}
			bases = append(bases, qualified)
		}
		scope := "orm:" + strings.Join(bases, ",")
		owner := dataOwner{name: match[1], scope: scope, line: i + 1, end: end + 1}
		owners = append(owners, owner)
		name, tableLine := "", 0
		for j := i + 1; j < end; j++ {
			if !pythonTable.MatchString(masked[j]) {
				continue
			}
			for _, literal := range literals {
				if literal.line == j+1 && !literal.dynamic {
					name = literal.text
					tableLine = j + 1
					break
				}
			}
		}
		if name == "" {
			continue
		}
		data := &facts.DataObject{Kind: "table", Origin: "orm", Scope: scope, Name: name, Owner: &facts.Anchor{Path: file, Line: i + 1}}
		for j := i + 1; j < end; j++ {
			if !pythonTableArgs.MatchString(masked[j]) {
				continue
			}
			complete := lines[j]
			balance := pythonBracketBalance(masked[j])
			for k := j + 1; k < end && balance > 0; k++ {
				complete += "\n" + lines[k]
				balance += pythonBracketBalance(masked[k])
			}
			values, code := sourceLiterals(complete, true)
			for k := 0; k+1 < len(values); k++ {
				if values[k].text == "schema" && strings.TrimSpace(code[values[k].end:values[k+1].start]) == ":" && !values[k+1].dynamic {
					if data.Schema != "" && data.Schema != values[k+1].text {
						data.Partial = true
						data.Schema = ""
						break
					}
					data.Schema = values[k+1].text
				}
			}
			data.Partial = data.Partial || balance != 0
		}
		for j := i + 1; j < end; j++ {
			// Direct class fields only: method locals and nested blocks are not columns.
			if len(masked[j])-len(strings.TrimLeft(masked[j], " ")) != 4 {
				continue
			}
			field := pythonField.FindStringSubmatch(strings.TrimSpace(lines[j]))
			if len(field) == 0 || strings.HasPrefix(field[1], "__") {
				continue
			}
			annotation, expression := strings.TrimSpace(field[2]), strings.TrimSpace(field[3])
			callee := strings.TrimSpace(strings.Split(expression, "(")[0])
			qualified := pythonDataName(callee, imports)
			columnCall := strings.Contains(expression, "(") && (qualified == "sqlalchemy.Column" || qualified == "sqlalchemy.orm.mapped_column")
			mapped := strings.Contains(annotation, "[") && pythonDataName(strings.TrimSpace(strings.Split(annotation, "[")[0]), imports) == "sqlalchemy.orm.Mapped"
			if !columnCall && !(mapped && expression == "") {
				continue
			}
			complete := lines[j]
			completeMask := masked[j]
			balance := strings.Count(masked[j], "(") - strings.Count(masked[j], ")")
			for k := j + 1; k < end && balance > 0; k++ {
				complete += "\n" + lines[k]
				completeMask += "\n" + masked[k]
				balance += strings.Count(masked[k], "(") - strings.Count(masked[k], ")")
			}
			column := facts.DataColumn{Name: field[1], Type: annotation, PrimaryKey: primaryKey.MatchString(completeMask), Anchor: facts.Anchor{Path: file, Line: j + 1}}
			for _, call := range pythonCall.FindAllStringSubmatchIndex(completeMask, -1) {
				name := complete[call[2]:call[3]]
				value := pythonFirstString(complete[call[1]:])
				if value == "" {
					continue
				}
				switch pythonDataName(name, imports) {
				case "sqlalchemy.ForeignKey":
					column.ForeignKey = value
				case "sqlalchemy.Column", "sqlalchemy.orm.mapped_column":
					column.Name = value
				}
			}
			if balance != 0 {
				data.Partial = true
			}
			data.Columns = append(data.Columns, column)
		}
		b.addTable(file, tableLine, data)
	}
	return owners
}

func pythonDataName(name string, imports map[string]string) string {
	first, rest, _ := strings.Cut(name, ".")
	qualified := imports[first]
	if qualified == "" {
		return ""
	}
	if rest != "" {
		qualified += "." + rest
	}
	return qualified
}

func pythonFirstString(source string) string {
	source = strings.TrimSpace(source)
	if len(source) < 2 || source[0] != '\'' && source[0] != '"' {
		return ""
	}
	end := strings.IndexByte(source[1:], source[0])
	if end < 0 {
		return ""
	}
	return source[1 : end+1]
}

func pythonBracketBalance(source string) int {
	return strings.Count(source, "(") + strings.Count(source, "{") + strings.Count(source, "[") - strings.Count(source, ")") - strings.Count(source, "}") - strings.Count(source, "]")
}
