package claims

import (
	"regexp"
	"strings"
)

// commentMarker names the markers that state intent rather than work left to
// do; TODO/FIXME are facts and belong to the facts layer.
var commentMarker = regexp.MustCompile(`(?i)\b(?:NOTE|WARNING|IMPORTANT):|\bDEPRECATED\b`)

// markerComments quotes each comment line carrying a marker. Only the comment
// portion of a line is inspected so a marker inside code is never quoted.
func markerComments(lines []string, lineComment string) []quote {
	var result []quote
	for index, line := range lines {
		portion, ok := commentPortion(line, lineComment)
		if !ok || !commentMarker.MatchString(portion) {
			continue
		}
		if text := quoteWithin(portion); text != "" {
			result = append(result, quote{Line: index + 1, Text: text})
		}
	}
	return result
}

// commentPortion returns the prose of a line comment or of one line inside a
// block comment, without its marker characters.
func commentPortion(line, lineComment string) (string, bool) {
	if start := strings.Index(line, lineComment); start >= 0 {
		return strings.TrimLeft(line[start+len(lineComment):], "/*#! "), true
	}
	trimmed := strings.TrimSpace(line)
	if lineComment == "//" && (strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "/*")) {
		return strings.TrimSpace(strings.TrimSuffix(strings.TrimLeft(trimmed, "/* "), "*/")), true
	}
	return "", false
}
