package facts

import (
	"go/scanner"
	"go/token"
	"path"
	"regexp"
	"slices"
	"strings"
)

var todoMarker = regexp.MustCompile(`\b(TODO|FIXME|XXX|HACK)\b`)

// addTODOs scans every text file of the corpus. Rows under a target root are
// attributed to that target; the rest stay repository-level.
func (b *builder) addTODOs() {
	for _, filePath := range b.source.paths() {
		file, ok := b.source.file(filePath)
		if !ok || file.binary || !slices.ContainsFunc(file.lines, hasTODOWord) {
			continue
		}
		targetID := b.targetForPath(filePath)
		root := b.rootForTarget(targetID)
		for number, line := range todoLines(filePath, file.lines) {
			if !hasTODOWord(line) {
				continue
			}
			match := todoMarker.FindStringSubmatchIndex(line)
			if match == nil {
				continue
			}
			marker := line[match[2]:match[3]]
			text := clipText(strings.TrimLeft(strings.TrimSpace(line[match[3]:]), ":- "))
			text = strings.TrimSpace(strings.TrimSuffix(text, "*/"))
			if text == "" {
				text = marker
			}
			b.add(root, Fact{
				Kind:     KindTODO,
				TargetID: targetID,
				Anchor:   &Anchor{Path: filePath, Line: number + 1},
				Key:      marker,
				Text:     text,
			}, marker, text)
		}
	}
}

// hasTODOWord is whether a line holds a marker's letters at all: a marker
// needs them, and a comment is part of its source lines, so a file without
// them is neither lexed nor matched.
func hasTODOWord(line string) bool {
	return strings.Contains(line, "TODO") || strings.Contains(line, "FIXME") ||
		strings.Contains(line, "XXX") || strings.Contains(line, "HACK")
}

// Go's lexer distinguishes actual comments from context.TODO(), identifiers
// and strings. Keep physical source lines, including inside block comments.
func todoLines(filePath string, lines []string) []string {
	if path.Ext(filePath) != ".go" {
		return lines
	}
	source := []byte(strings.Join(lines, "\n"))
	file := token.NewFileSet().AddFile(filePath, -1, len(source))
	var lexer scanner.Scanner
	lexer.Init(file, source, func(token.Position, string) {}, scanner.ScanComments)
	comments := make([]string, len(lines))
	for {
		pos, kind, text := lexer.Scan()
		if kind == token.EOF {
			break
		}
		if kind != token.COMMENT {
			continue
		}
		start := file.PositionFor(pos, false).Line - 1
		for offset, line := range strings.Split(text, "\n") {
			comments[start+offset] += " " + line
		}
	}
	return comments
}
