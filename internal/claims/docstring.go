package claims

import (
	"regexp"
	"strings"
)

var (
	pythonTripleQuote = regexp.MustCompile(`^[rRuUbBfF]{0,2}("""|''')`)
	pythonTopLevelDef = regexp.MustCompile(`^(?:async\s+)?def\s|^class\s`)
	jsDeclaration     = regexp.MustCompile(
		`^\s*(?:export\s+)?(?:default\s+)?(?:declare\s+)?(?:abstract\s+)?(?:async\s+)?` +
			`(?:function\b|class\b|const\b|let\b|var\b|interface\b|type\b|enum\b|export\b)`,
	)
)

// pythonDocstrings quotes the module docstring and the docstring of every
// top-level def/class: the first statement of a body when it is a
// triple-quoted string. No AST; a line scanner is enough for a quote.
func pythonDocstrings(lines []string) []quote {
	var result []quote
	if index := pythonFirstStatement(lines, 0); index >= 0 {
		if item, ok := pythonDocstringAt(lines, index); ok {
			result = append(result, item)
		}
	}
	for index, line := range lines {
		if !pythonTopLevelDef.MatchString(line) {
			continue
		}
		end := pythonSignatureEnd(lines, index)
		if end < 0 {
			continue
		}
		body := pythonFirstStatement(lines, end+1)
		if body < 0 || !isIndented(lines[body]) {
			continue
		}
		if item, ok := pythonDocstringAt(lines, body); ok {
			result = append(result, item)
		}
	}
	return result
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
	for index := from; index < len(lines); index++ {
		trimmed := strings.TrimSpace(lines[index])
		if comment := strings.Index(trimmed, "#"); comment >= 0 {
			trimmed = strings.TrimSpace(trimmed[:comment])
		}
		if strings.HasSuffix(trimmed, ":") {
			return index
		}
	}
	return -1
}

func isIndented(line string) bool {
	return strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
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
	for index, line := range lines {
		if !strings.HasPrefix(line, "func ") && !strings.HasPrefix(line, "type ") {
			continue
		}
		start := index
		for start > 0 && strings.HasPrefix(lines[start-1], "//") {
			start--
		}
		if start == index {
			continue
		}
		var prose []string
		for _, comment := range lines[start:index] {
			if strings.HasPrefix(comment, "//go:") {
				continue
			}
			prose = append(prose, strings.TrimPrefix(comment, "//"))
		}
		if item, ok := docstringQuote(start, strings.Join(prose, "\n")); ok {
			result = append(result, item)
		}
	}
	return result
}

// jsDocBlocks quotes /** ... */ blocks that sit directly above a declaration.
// Tag lines (@param, @returns, ...) describe structure, not intent, and are
// left out of the quote.
func jsDocBlocks(lines []string) []quote {
	var result []quote
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
		next := end + 1
		if next < len(lines) && jsDeclaration.MatchString(lines[next]) {
			if item, ok := docstringQuote(index, jsDocText(lines[index:end+1])); ok {
				result = append(result, item)
			}
		}
		index = end
	}
	return result
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
