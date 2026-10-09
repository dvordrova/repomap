package claims

import (
	"fmt"
	"github.com/dvordrova/repomap/internal/programindex"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

var (
	pythonTripleQuote = regexp.MustCompile(`^[rRuU]?("""|''')`)
	pythonDeclaration = regexp.MustCompile(`^(?:async\s+)?def\s|^class\s`)
	jsDeclaration     = regexp.MustCompile(
		`^\s*(?:export\s+)?(?:default\s+)?(?:declare\s+)?(?:abstract\s+)?(?:async\s+)?` +
			`(?:function\b|class\b|const\b|let\b|var\b|interface\b|type\b|enum\b|export\b)`,
	)
)

// pythonDocstrings quotes module and def/class body docstrings, including
// nested declarations. The quote retains its own location and its exact
// declaration header; it does not establish any runtime effect.
func pythonDocstrings(lines []string) []quote {
	var result []quote
	if index := pythonFirstStatement(lines, 0); index >= 0 {
		if item, ok := pythonDocstringAt(lines, index); ok {
			result = append(result, item)
		}
	}
	code := pythonCodeLines(lines)
	for index, line := range code {
		if !pythonDeclaration.MatchString(strings.TrimSpace(line)) {
			continue
		}
		end := pythonSignatureEnd(code, index)
		if end < 0 {
			continue
		}
		body := pythonFirstStatement(lines, end+1)
		if body < 0 || pythonIndent(lines[body]) <= pythonIndent(lines[index]) {
			continue
		}
		if item, ok := pythonDocstringAt(lines, body); ok {
			item.DeclarationLine = index + 1
			result = append(result, item)
		}
	}
	return result
}

// Ignore declaration-looking text inside strings and comments. This is only
// lexical quote extraction; the native adapter remains the declaration owner.
func pythonCodeLines(lines []string) []string {
	code := make([]string, len(lines))
	delimiter := ""
	for i, line := range lines {
		masked := []byte(strings.Repeat(" ", len(line)))
		for pos := 0; pos < len(line); {
			if delimiter != "" {
				if line[pos] == '\\' {
					pos += 2
				} else if strings.HasPrefix(line[pos:], delimiter) {
					pos += len(delimiter)
					delimiter = ""
				} else {
					pos++
				}
				continue
			}
			if line[pos] == '#' {
				break
			}
			if line[pos] == '\'' || line[pos] == '"' {
				// Keep a marker so an inline string body is still a statement.
				masked[pos] = 's'
				delimiter = string(line[pos])
				if triple := strings.Repeat(delimiter, 3); strings.HasPrefix(line[pos:], triple) {
					delimiter = triple
				}
				pos += len(delimiter)
				continue
			}
			masked[pos] = line[pos]
			pos++
		}
		code[i] = string(masked)
	}
	return code
}

func pythonIndent(line string) int {
	indent := 0
	for _, ch := range line {
		switch ch {
		case ' ':
			indent++
		case '\t':
			indent += 8 - indent%8
		default:
			return indent
		}
	}
	return indent
}

func pythonFirstStatement(lines []string, from int) int {
	for index := from; index < len(lines); index++ {
		trimmed := strings.TrimSpace(lines[index])
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			return index
		}
	}
	return -1
}

func pythonSignatureEnd(lines []string, from int) int {
	depth := 0
	for index := from; index < len(lines); index++ {
		line := lines[index]
		for pos, ch := range line {
			switch ch {
			case '(', '[', '{':
				depth++
			case ')', ']', '}':
				depth--
			case ':':
				if depth == 0 {
					if strings.TrimSpace(line[pos+1:]) == "" {
						return index
					}
					// An inline body cannot own the next declaration's string.
					return -1
				}
			}
		}
		if depth == 0 && !strings.HasSuffix(strings.TrimSpace(line), "\\") {
			return -1
		}
	}
	return -1
}

func pythonDocstringAt(lines []string, index int) (quote, bool) {
	trimmed := strings.TrimSpace(lines[index])
	match := pythonTripleQuote.FindStringSubmatch(trimmed)
	if match == nil {
		return quote{}, false
	}
	delimiter := match[1]
	rest := trimmed[len(match[0]):]
	if closing := strings.Index(rest, delimiter); closing >= 0 {
		return docstringQuote(index, rest[:closing])
	}
	collected := []string{rest}
	for next := index + 1; next < len(lines); next++ {
		line := lines[next]
		if closing := strings.Index(line, delimiter); closing >= 0 {
			collected = append(collected, line[:closing])
			return docstringQuote(index, strings.Join(collected, "\n"))
		}
		collected = append(collected, line)
	}
	return quote{}, false
}

func docstringQuote(index int, raw string) (quote, bool) {
	text := quoteWithin(raw)
	if text == "" {
		return quote{}, false
	}
	return quote{Line: index + 1, Text: text}, true
}

// goDocComments quotes the // block directly above a top-level func or type.
// Compiler directives (//go:...) are not prose and are left out.
func goDocComments(lines []string) []quote {
	var result []quote
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", strings.Join(lines, "\n"), parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return result
	}
	add := func(doc *ast.CommentGroup, name *ast.Ident) {
		if doc == nil {
			return
		}
		if item, ok := docstringQuote(fset.PositionFor(doc.Pos(), false).Line-1, doc.Text()); ok {
			if name != nil {
				site := fset.PositionFor(name.Pos(), false)
				item.DeclarationLine, item.DeclarationColumn = site.Line, site.Column
			}
			result = append(result, item)
		}
	}
	for _, decl := range file.Decls {
		switch decl := decl.(type) {
		case *ast.FuncDecl:
			add(decl.Doc, decl.Name)
		case *ast.GenDecl:
			if decl.Tok != token.TYPE {
				continue
			}
			if len(decl.Specs) == 1 {
				spec := decl.Specs[0].(*ast.TypeSpec)
				if spec.Doc == nil {
					add(decl.Doc, spec.Name)
				}
			} else {
				add(decl.Doc, nil)
			}
			for _, raw := range decl.Specs {
				spec := raw.(*ast.TypeSpec)
				add(spec.Doc, spec.Name)
				iface, ok := spec.Type.(*ast.InterfaceType)
				if !ok {
					continue
				}
				for _, field := range iface.Methods.List {
					if _, ok := field.Type.(*ast.FuncType); ok && len(field.Names) == 1 {
						add(field.Doc, field.Names[0])
					}
				}
			}
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Line < result[j].Line })
	return result
}

// jsDocBlocks quotes /** ... */ blocks that sit directly above a declaration.
// Tag lines (@param, @returns, ...) describe structure, not intent, and are
// left out of the quote.
func jsDocBlocks(lines []string) []quote {
	result, _ := jsDocBlocksWithOwners(lines, nil)
	return result
}

func jsDocBlocksWithOwners(lines []string, owners map[programindex.LineRange]declarationSite) ([]quote, error) {
	var result []quote
	for span, owner := range owners {
		text, err := nativeCommentText(lines, span)
		if err != nil {
			return nil, err
		}
		if item, ok := docstringQuote(span.Line-1, jsDocText(strings.Split(text, "\n"))); ok {
			item.Column = span.Column
			item.DeclarationLine, item.DeclarationColumn = owner.Line, owner.Column
			result = append(result, item)
		}
	}
	// Conventional unbound comments remain author claims. Native-known ranges
	// already supplied their complete quote and never pass through this fallback.
	for index := 0; index < len(lines); index++ {
		if !strings.HasPrefix(strings.TrimSpace(lines[index]), "/**") {
			continue
		}
		end := index
		for end < len(lines) && !strings.Contains(lines[end], "*/") {
			end++
		}
		if end >= len(lines) {
			break
		}
		startByte := strings.Index(lines[index], "/**")
		endByte := strings.Index(lines[end], "*/") + 2
		native := false
		for span := range owners {
			if span.Line == index+1 && span.EndLine == end+1 {
				first, err1 := utf16ByteOffset(lines[index], span.Column)
				last, err2 := utf16ByteOffset(lines[end], span.EndColumn+1)
				if err1 == nil && err2 == nil && first == startByte && last == endByte {
					native = true
					break
				}
			}
		}
		if !native && end+1 < len(lines) && jsDeclaration.MatchString(lines[end+1]) {
			block := append([]string(nil), lines[index:end+1]...)
			block[len(block)-1] = block[len(block)-1][:endByte]
			if item, ok := docstringQuote(index, jsDocText(block)); ok {
				result = append(result, item)
			}
		}
		index = end
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Line != result[j].Line {
			return result[i].Line < result[j].Line
		}
		return result[i].Column < result[j].Column
	})
	return result, nil
}

// TypeScript coordinates are one-based UTF-16 units, with an inclusive end.
// Original corpus strings use UTF-8; a surrogate-pair interior is no boundary.
func utf16ByteOffset(line string, column int) (int, error) {
	if column < 1 {
		return 0, fmt.Errorf("invalid UTF-16 column %d", column)
	}
	units := 1
	for offset, r := range line {
		if units == column {
			return offset, nil
		}
		width := 1
		if r > 0xffff {
			width = 2
		}
		units += width
		if units > column {
			return 0, fmt.Errorf("UTF-16 column splits a source character")
		}
	}
	if units == column {
		return len(line), nil
	}
	return 0, fmt.Errorf("UTF-16 column exceeds source line")
}
func nativeCommentText(lines []string, span programindex.LineRange) (string, error) {
	if span.Line < 1 || span.EndLine < span.Line || span.EndLine > len(lines) {
		return "", fmt.Errorf("native comment range outside source")
	}
	first, err := utf16ByteOffset(lines[span.Line-1], span.Column)
	if err != nil {
		return "", err
	}
	last, err := utf16ByteOffset(lines[span.EndLine-1], span.EndColumn+1)
	if err != nil {
		return "", err
	}
	if span.Line == span.EndLine && last < first {
		return "", fmt.Errorf("reversed native comment range")
	}
	block := append([]string(nil), lines[span.Line-1:span.EndLine]...)
	block[len(block)-1] = block[len(block)-1][:last]
	block[0] = block[0][first:]
	text := strings.Join(block, "\n")
	if !utf8.ValidString(text) || !strings.HasPrefix(text, "/**") || !strings.HasSuffix(text, "*/") || strings.Index(text, "*/") != len(text)-2 {
		return "", fmt.Errorf("native range does not name a complete JSDoc comment")
	}
	return text, nil
}

func jsDocText(block []string) string {
	var prose []string
	for _, line := range block {
		text := strings.TrimSpace(line)
		text = strings.TrimPrefix(text, "/**")
		text = strings.TrimSuffix(text, "*/")
		text = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text), "*"))
		if strings.HasPrefix(text, "@") {
			continue
		}
		prose = append(prose, text)
	}
	return strings.Join(prose, "\n")
}
